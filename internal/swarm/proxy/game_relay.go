// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "errors"
    "time"
)

// transitToServer forwards one decrypted client packet to the real game
// server through the bot session. The keepalive is answered locally and
// never transits; a held client keeps the keepalive and the logout
// (the synthesized LeaveWorld) and swallows the rest. A send failure
// means the session link died: the recorder close owns the hold, so
// the client connection survives (docs/proxy.md).
func (gc *gameConn) transitToServer(payload []byte) error {
    if payload[0] == clientOpLogout {
        gc.markUserLogout()
        if gc.isHolding() {
            // The user wants out of the frozen world: the character is
            // offline, so the server cannot answer the logout - the
            // proxy hands the client the login screen itself.
            gc.relayToClient(buildLeaveWorldPacket())
            gc.shutdown("the user logged out while held for the bot relogin")

            return nil
        }
    }
    if payload[0] == clientOpNetPing {
        // The client keepalive never reaches the real server: the bot
        // session already keeps that connection alive with its own ping
        // cycle, and a transit would close the ping feedback loop (the
        // server answer would be recorded and relayed, an answer driven
        // client would re-ping per answer, and the loop would accelerate
        // without bound on the attached bot session). The proxy answers
        // the keepalive itself, exactly like the server would.
        gc.answerNetPingLocally()

        return nil
    }
    if gc.isHolding() {
        gc.server.logger.Printf(
            "game#%d: client packet 0x%02x swallowed while held "+
                "for the bot relogin",
            gc.id, payload[0])

        return nil
    }
    if payload[0] == clientOpValidatePosition &&
        gc.clientViewDiverged(payload) {
        // The client reports a place the bot's world contradicts: the
        // report never reaches the server (the Mobius out of sync
        // branch would snap the server side character to the client's
        // stale view and fight every bot walk request), and the client
        // gets the correction the server itself would have sent.
        gc.correctClientView(payload)

        return nil
    }
    if payload[0] == clientOpCannotMoveAnymore {
        // The client vetoes its local walk: the Mobius handler throws
        // the reported place away and cancels the player's move
        // intention, which stops the bot's own server side walk at
        // the server place. A spectator client drives no movement, so
        // its local collision has no authority over the bot session.
        gc.discardClientVeto(payload)

        return nil
    }
    transformed, ok := gc.server.transformer.ClientToServer(payload)
    if !ok {
        gc.server.logger.Printf("game#%d: dropped client packet 0x%02x",
            gc.id, payload[0])

        return nil
    }
    if err := gc.currentSession().client.SendRaw(transformed); err != nil {
        gc.server.logger.Printf(
            "game#%d: client packet 0x%02x not sent, the bot "+
                "session link is failing: %v",
            gc.id, payload[0], err)

        return nil
    }
    gc.server.logger.Printf("game#%d: client -> server 0x%02x (%d bytes)",
        gc.id, payload[0], len(payload))

    return nil
}

// runSender is the single writer of the client socket: it encrypts and
// sends every outbound payload in order, so the outbound cipher chain
// stays consistent no matter which goroutine produced the packet. On
// the connection close it flushes the packets already queued (a
// shutdown right behind a queued packet - the LeaveWorld of the held
// logout - must still reach the client) and then closes the socket
// itself: the reader is released by the close, not by the done flag.
func (gc *gameConn) runSender() {
    gc.senderLive.Store(true)
    defer func() {
        close(gc.senderDone)
        _ = gc.conn.Close()
    }()

    buf := make([]byte, 0, 1024)
    for {
        select {
        case <-gc.done:
            gc.drainOutbound(buf)

            return
        case payload := <-gc.outCh:
            if !gc.writeOutbound(buf, payload) {
                return
            }
        }
    }
}

// drainOutbound flushes the packets queued at the moment of the close;
// a failed write aborts the drain (the socket is gone).
func (gc *gameConn) drainOutbound(buf []byte) {
    for {
        select {
        case payload := <-gc.outCh:
            if !gc.writeOutbound(buf, payload) {
                return
            }
        default:
            return
        }
    }
}

// writeOutbound encrypts one payload on the scratch buffer and writes it to the
// client socket. It reports false when the write failed: the socket is closed
// right away (the reader must be released), the remaining queue is dropped.
func (gc *gameConn) writeOutbound(buf []byte, payload []byte) bool {
    // The payload may be shared with the recorder history: the
    // encryption transforms in place, so it runs on the copy.
    buf = append(buf[:0], payload...)
    gc.crypt.Encrypt(buf)
    if err := gc.conn.SetWriteDeadline(
        time.Now().Add(gameWriteTimeout)); err != nil {
        gc.server.logger.Printf("game#%d: write deadline failed: %v",
            gc.id, err)
        _ = gc.conn.Close()

        return false
    }
    if err := writeWirePacket(gc.conn, buf); err != nil {
        gc.server.logger.Printf("game#%d: client write failed: %v",
            gc.id, err)
        _ = gc.conn.Close()

        return false
    }

    return true
}

// runRelay streams the sessions of the client's bot until the
// connection ends: each cycle replays one session history and follows
// it with the live feed, then performs the relogin handoff or the WebUI
// switch through the same restart dance. The relay goroutine stays the
// single owner of the stream for the life of the connection.
func (gc *gameConn) runRelay() {
    defer gc.setRelayLive(false)
    fromSeq := gc.charSelectedSeq
    for {
        next, ok := gc.streamSession(fromSeq)
        if !ok {
            return
        }
        fromSeq = next
    }
}

// streamSession serves one bot session cycle: the replay of everything recorded
// after the given sequence, followed by the live feed. It returns the replay
// start sequence of the next cycle, or ok=false when the connection is done.
//
// The LeaveWorld of a bot logout is suppressed in both phases (see
// handoff.go); a client that asked for the logout itself receives it.
func (gc *gameConn) streamSession(fromSeq int64) (int64, bool) {
    session := gc.currentSession()
    recorder := session.recorder
    entries, sub := recorder.Attach(fromSeq)
    defer recorder.removeSubscriber(sub)

    // Snapshot the selection channel at the start of the cycle: when
    // SelectBot fires it (closes it), the live feed select wakes up
    // and the client switches onto the newly selected bot.
    selectionCh := gc.server.selectionChannel()

    // A selection whose target bot is not online yet (registered but still
    // connecting, or not registered at all) arms a pending switch: the live
    // feed keeps flowing while the poll ticker below retries the switch, so the
    // client lands on the target the moment it enters the world. Without the
    // pending the relay would stay on the current bot forever: SelectBot of the
    // same id is a no-op that never refires the channel.
    pendingSwitch := ""
    switchPoll := time.NewTicker(botSwitchPollPeriod)
    defer switchPoll.Stop()

    selfID := session.tracker.SelfObjectID()
    lastSelfMoveSeq := lastSelfMovementSeq(entries, selfID)

    replayBytes := 0
    for i := range entries {
        replayBytes += len(entries[i].payload)
    }
    gc.server.logger.Printf(
        "game#%d: replaying %d recorded packets (%d bytes) with "+
            "the live self state (self id %d), then live",
        gc.id, len(entries), replayBytes, selfID)

    gc.replayHistory(entries, selfID, lastSelfMoveSeq, session)
    gc.server.logger.Printf("game#%d: replay done, live relay active", gc.id)

    for {
        select {
        case <-gc.done:
            return 0, false
        case <-sub.poison:
            gc.shutdown("the client fell behind the live feed")

            return 0, false
        case <-recorder.CloseSignal():
            return gc.serveRelogin(session)
        case <-selectionCh:
            target, next := gc.serveBotSwitch(session)
            if next != nil {
                return gc.parkAtCharSelect()
            }
            // The selection did not resolve to a different online session: stay
            // on the current live feed, and arm the pending switch when the
            // target is not online yet ("" when the selection names the current
            // bot or nothing).
            pendingSwitch = target
            selectionCh = gc.server.selectionChannel()
        case <-switchPoll.C:
            next, ok := gc.retryPendingSwitch(session, pendingSwitch)
            if !ok {
                return 0, false
            }
            if next != nil {
                return gc.parkAtCharSelect()
            }
        case update := <-sub.ch:
            if !gc.deliverLiveUpdate(update.payload) {
                return 0, false
            }
        }
    }
}

// deliverLiveUpdate applies the live feed policy of one recorded
// packet: the LeaveWorld of a bot logout is suppressed (the client
// stays for the relogin hold), the NetPing answers of the bot session
// are filtered out with their game time harvested (the keepalive round
// trips carry no world state and relaying them arms the ping feedback
// loop, see isNetPingAnswerPayload), and everything else is relayed to
// the client. It reports false when the connection is done.
func (gc *gameConn) deliverLiveUpdate(payload []byte) bool {
    if isLeaveWorldPayload(payload) && !gc.clientLoggedOut() {
        gc.server.logger.Printf(
            "game#%d: leave world suppressed, the client stays for "+
                "the bot relogin",
            gc.id)

        return true
    }
    if isNetPingAnswerPayload(payload) {
        gc.noteNetPingGameTime(payload)

        return true
    }

    return gc.relayToClient(payload)
}

// replayHistory streams the recorded history of one session cycle: the
// LeaveWorld of a bot logout is skipped (the hold depends on the client
// staying in the world), the NetPing answers of the bot session are
// skipped with their game time harvested (see isNetPingAnswerPayload),
// the UserInfo of the played character carries the live state and the
// stale self movement is dropped except the newest packet.
func (gc *gameConn) replayHistory(
    entries []RecorderEntry, selfID int32, lastSelfMoveSeq int64,
    session *botSession,
) {
    droppedMoves := 0
    for i := range entries {
        payload := entries[i].payload
        if isLeaveWorldPayload(payload) && !gc.clientLoggedOut() {
            continue
        }
        if isNetPingAnswerPayload(payload) {
            gc.noteNetPingGameTime(payload)

            continue
        }
        if isSelfMovementPayload(payload, selfID) &&
            entries[i].seq != lastSelfMoveSeq {
            droppedMoves++

            continue
        }
        if payload[0] == replayOpUserInfo {
            payload = patchUserInfoSelfLive(
                payload, selfID, session.tracker.SelfSnapshot())
        }
        if !gc.relayToClient(payload) {
            return
        }
    }
    if droppedMoves > 0 {
        gc.server.logger.Printf(
            "game#%d: replay dropped %d stale self movement packets",
            gc.id, droppedMoves)
    }
}

// relayToClient transforms one server packet and hands it to the
// sender; it reports false when the connection is done.
func (gc *gameConn) relayToClient(payload []byte) bool {
    transformed, ok := gc.server.transformer.ServerToClient(payload)
    if !ok {
        return true
    }
    select {
    case gc.outCh <- transformed:
        return true
    case <-gc.done:
        return false
    }
}

func (gc *gameConn) sendToClient(payload []byte) error {
    select {
    case gc.outCh <- payload:
        return nil
    case <-gc.done:
        return errors.New("the client connection is closing")
    }
}
