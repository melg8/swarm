// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "time"
)

// serveBotSwitch handles a WebUI selection change while a client is
// connected: a different ready bot (online with its CharSelected
// recorded) starts the restart dance onto it. It returns the target id
// with the new session when the dance started, the target id with a
// nil session while the target is not ready (the caller arms the
// pending switch), and "" with a nil session when the selection names
// the current bot or nothing (docs/proxy.md, "Switching bots").
func (gc *gameConn) serveBotSwitch(current *botSession) (string, *botSession) {
    target := gc.server.SelectedBot()
    if target == "" || target == current.id {
        return "", nil
    }
    next := gc.server.sessionByID(target)
    if next == nil || next == current || !switchReady(next) {
        gc.server.logger.Printf(
            "game#%d: selection switched to %q but the bot is not online "+
                "yet, serving %q until it enters the world",
            gc.id, target, current.id)

        return target, nil
    }
    gc.server.logger.Printf(
        "game#%d: selection switched from %q to %q, starting the restart dance",
        gc.id, current.id, next.id)
    if !gc.beginRestartSwitch(next) {
        return "", nil
    }

    return target, next
}

// retryPendingSwitch resolves one poll tick of the pending bot switch:
// the WebUI selection named a bot that was not online yet when it
// fired, and the tick checks whether the target entered the world in
// the meantime. It starts the restart dance onto it and returns the
// new session with ok=true, returns nil with ok=true while the target
// is still not online (the live feed keeps flowing), and ok=false when
// the connection died mid dance.
func (gc *gameConn) retryPendingSwitch(
    session *botSession, pending string,
) (*botSession, bool) {
    if pending == "" || pending == session.id {
        return nil, true
    }
    next := gc.server.sessionByID(pending)
    if next == nil || next == session || !switchReady(next) {
        return nil, true
    }
    gc.server.logger.Printf(
        "game#%d: the pending switch target %q entered the world, "+
            "starting the restart dance",
        gc.id, pending)
    if !gc.beginRestartSwitch(next) {
        return nil, false
    }

    return next, true
}

// beginRestartSwitch starts the restart dance for a live in-world
// client: RestartResponse plus the char list move it to its char select
// screen (the exact Mobius RequestRestart.handlePacket answer), the
// session swaps to the target and the auto select follows. The C1
// client tears its world down on the RestartResponse, which is what
// makes the restart the only safe identity change: the teleport +
// DeleteObject sweep crashes it (docs/proxy.md, "Switching bots").
func (gc *gameConn) beginRestartSwitch(next *botSession) bool {
    gc.setSession(next)
    gc.setState(gameStateChars)

    if err := gc.sendToClient(buildRestartResponsePacket()); err != nil {
        gc.server.logger.Printf(
            "game#%d: the restart answer did not reach the client: %v",
            gc.id, err)

        return false
    }
    list, err := gc.buildCharacterList()
    if err != nil {
        gc.server.logger.Printf(
            "game#%d: the char list of %q could not be built (%v), "+
                "continuing with the dance",
            gc.id, next.id, err)
    } else if err := gc.sendToClient(list); err != nil {
        gc.server.logger.Printf(
            "game#%d: the char list of %q did not reach the client: %v",
            gc.id, next.id, err)

        return false
    }
    gc.armAutoSelect()
    gc.server.logger.Printf(
        "game#%d: restart dance onto %q offered (auto select in %s)",
        gc.id, next.id, switchTimings())

    return true
}

// offerCharList re-points a parked client at the given session: the
// client already sits on its char select screen, so the dance only
// swaps the session, pushes the char list of the newly selected bot and
// re-arms the auto select - the screen re-renders the model of the new
// character and the unsolicited answer follows as before.
func (gc *gameConn) offerCharList(next *botSession) bool {
    gc.setSession(next)
    list, err := gc.buildCharacterList()
    if err != nil {
        gc.server.logger.Printf(
            "game#%d: the char list of %q could not be built: %v",
            gc.id, next.id, err)

        return true // the auto select still lands on the new session
    }
    if err := gc.sendToClient(list); err != nil {
        gc.server.logger.Printf(
            "game#%d: the char list of %q did not reach the client: %v",
            gc.id, next.id, err)

        return false
    }
    gc.armAutoSelect()
    gc.server.logger.Printf(
        "game#%d: the parked char select screen re-offered onto %q",
        gc.id, next.id)

    return true
}

// parkAtCharSelect parks the relay while the client sits on the char
// select screen of a restart dance: it waits for the read loop to
// report the re-entry (the EnterWorld after the served char selected
// answer), tracks newer WebUI selections (the screen re-offers the
// newest char list) and polls for a pending target that was offline at
// the selection time. It returns the replay sequence of the entered
// session, or ok=false when the connection ended.
func (gc *gameConn) parkAtCharSelect() (int64, bool) {
    // Drain any stale re-entry report before parking: a duplicate
    // EnterWorld of the previous cycle must not leak into this one.
    for len(gc.enterWorldCh) > 0 {
        <-gc.enterWorldCh
    }

    pending := ""
    poll := time.NewTicker(botSwitchPollPeriod)
    defer poll.Stop()
    for {
        selectionCh := gc.server.selectionChannel()
        select {
        case <-gc.done:
            return 0, false
        case seq := <-gc.enterWorldCh:
            return seq, true
        case <-selectionCh:
            target, next := parkSelectionTarget(
                gc.server, gc.currentSession().id)
            if next != nil {
                if !gc.offerCharList(next) {
                    return 0, false
                }
                pending = ""
            } else {
                pending = target
            }
        case <-poll.C:
            var ok bool
            pending, ok = gc.parkPendingOffer(pending)
            if !ok {
                return 0, false
            }
        }
    }
}

// parkSelectionTarget resolves what a selection change means for a
// parked client: the ready session of the newest selection to
// re-offer, or the pending target id when the selection is not ready
// yet ("" when the selection names the current bot or nothing - the
// park keeps offering what it has).
func parkSelectionTarget(s *Server, currentID string) (string, *botSession) {
    target := s.SelectedBot()
    if target == "" || target == currentID {
        return "", nil
    }
    if next := s.sessionByID(target); next != nil && switchReady(next) {
        return "", next
    }

    return target, nil
}

// parkPendingOffer completes one poll tick of a pending target armed
// while the client is parked: when the target became ready, the char
// list is re-offered onto it and the pending clears. It reports ok=false
// only when the re-offer failed (the client connection is gone).
func (gc *gameConn) parkPendingOffer(pending string) (string, bool) {
    if pending == "" || pending == gc.currentSession().id {
        return pending, true
    }
    next := gc.server.sessionByID(pending)
    if next == nil || !switchReady(next) {
        return pending, true
    }
    if !gc.offerCharList(next) {
        return "", false
    }

    return "", true
}

// armAutoSelect schedules the unsolicited char selected answer of the
// dance: after the delay the proxy plays the double click of the only
// listed character itself, so the WebUI selection alone carries the
// client into the world of the new bot (a hands-off switch). The user's
// own double click cancels the pending answer (see cancelAutoSelect);
// whichever fires first wins and the other becomes a no-op.
func (gc *gameConn) armAutoSelect() {
    delay := switchTimings()
    gc.mu.Lock()
    if gc.autoSelect != nil {
        gc.autoSelect.Stop()
    }
    gc.autoSelect = time.AfterFunc(delay, gc.autoSelectCharSelected)
    gc.mu.Unlock()
}

// cancelAutoSelect drops the pending unsolicited answer: the user's own
// double click arrived first.
func (gc *gameConn) cancelAutoSelect() {
    gc.mu.Lock()
    if gc.autoSelect != nil {
        gc.autoSelect.Stop()
        gc.autoSelect = nil
    }
    gc.mu.Unlock()
}

// autoSelectCharSelected is the proxy's own double click: it serves the
// char selected answer of the offered character while the client still
// sits on the char select screen. A client that moved on (the state
// left the chars screen - the user clicked first or the connection is
// gone) makes the timer a no-op.
func (gc *gameConn) autoSelectCharSelected() {
    gc.mu.Lock()
    gc.autoSelect = nil
    if gc.state != gameStateChars {
        gc.mu.Unlock()

        return
    }
    gc.mu.Unlock()

    gc.server.logger.Printf(
        "game#%d: auto selecting the offered character, serving the char "+
            "selected answer",
        gc.id)
    if err := gc.serveCharSelectedLive(); err != nil {
        gc.shutdown("the auto char select failed: " + err.Error())
    }
}

// serveRelogin handles the end of the streamed bot session: a client
// that asked for the logout itself is closed with the session, any
// other client is held through the bot relogin and taken through the
// restart dance onto the replacement session (the same character
// re-enters its fresh world). It returns the replay start sequence of
// the replacement session (ok=true) or the end of the connection.
func (gc *gameConn) serveRelogin(session *botSession) (int64, bool) {
    if gc.clientLoggedOut() {
        gc.shutdown("the bot session ended after the client logout")

        return 0, false
    }
    next := gc.holdForRelogin(session)
    if next == nil {
        return 0, false
    }
    if !gc.beginRestartSwitch(next) {
        return 0, false
    }

    return gc.parkAtCharSelect()
}
