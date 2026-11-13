// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "encoding/binary"
    "math"
    "time"
)

// answerNetPingLocally serves the client keepalive without a server
// round trip: the synthesized NetPing answer carries the game time the
// relay harvested from the last real answer of the bot session (the
// C1 client uses it for its clock cosmetics; before the first harvest
// it is zero, which only delays the day-night rendering). A send
// failure is silent: the connection teardown owns the reporting, and
// the answer rate of an answer driven client forbids per packet logs.
func (gc *gameConn) answerNetPingLocally() {
    gc.mu.Lock()
    gameTime := gc.netPingTime
    gc.mu.Unlock()

    answer := make([]byte, 5)
    answer[0] = serverOpNetPing
    binary.LittleEndian.PutUint32(answer[1:], uint32(gameTime))
    _ = gc.sendToClient(answer)
}

// clientViewDiverged reports whether one client position report
// contradicts the place the server holds the played character at. The
// Mobius ValidatePosition handler snaps the character to the reported
// place whenever the 3D distance exceeds one move speed, and it only
// answers a correction to a report 500 or more units away - so a stale
// spectator view in the band between the two drags the character back
// with no correction at all, and a view stuck on client side geometry
// keeps reporting the same stale place even when the correction
// arrives (the railing case: the frozen pawn of the stair railing held
// the character off the gatekeeper and the teleport bypass failed the
// interaction range silently). The compared place is the projected
// live position, not the last broadcast origin, because the origin
// sits a whole walk segment behind a moving character.
func (gc *gameConn) clientViewDiverged(payload []byte) bool {
    if len(payload) < 13 {
        return false
    }
    reportX := int32(binary.LittleEndian.Uint32(payload[1:5]))
    reportY := int32(binary.LittleEndian.Uint32(payload[5:9]))
    if reportX == 0 && reportY == 0 {
        // The login edge report the server ignores itself.
        return false
    }
    session := gc.currentSession()
    if session == nil {
        return false
    }
    selfX, selfY, selfZ, ok := session.tracker.SelfProjectedPosition()
    if !ok {
        return false
    }
    reportZ := int32(binary.LittleEndian.Uint32(payload[9:13]))
    dx := float64(reportX) - selfX
    dy := float64(reportY) - selfY
    dz := float64(reportZ) - selfZ
    limit := session.tracker.SelfRunSpeed() + validateDivergenceSlack

    return math.Sqrt(dx*dx+dy*dy+dz*dz) > limit
}

// correctClientView drops one diverged position report and answers it
// the way the Mobius server answers an out of sync client: a
// ValidateLocation carrying the live place of the played character.
// The answer heals the client's stale pawn view (the next reports come
// back in sync and transit again), and the diverged report itself
// never reaches the server, severing the position feedback loop that
// snapped the character between the bot's walk requests and the
// client's stale view. The correction may repeat with every stale
// report (the client heals after one, a stubborn view keeps getting
// corrected once per report), the log line is rate limited instead.
func (gc *gameConn) correctClientView(payload []byte) {
    reportX := int32(binary.LittleEndian.Uint32(payload[1:5]))
    reportY := int32(binary.LittleEndian.Uint32(payload[5:9]))
    x, y, ok := gc.sendViewCorrection()
    if !ok {
        return
    }
    if gc.viewLogAt.IsZero() || time.Since(gc.viewLogAt) > validateLogPeriod {
        gc.viewLogAt = time.Now()
        gc.server.logger.Printf(
            "game#%d: client position report %d %d is stale "+
                "against the bot at "+
                "%d %d, correcting the client view",
            gc.id, reportX, reportY, x, y)
    }
}

// discardClientVeto drops one client CannotMoveAnymore (0x36) and
// answers it with the same correction the server sends an out of sync
// client. The Mobius handler throws the reported place away and
// cancels the player's move intention (CannotMoveAnymore.runImpl ->
// notifyActionArrivedBlocked -> setIntentionIdle), which stops the
// bot's server side walk at the server place - the frozen pawn of an
// attached client vetoed every click the bot sent and the character
// never walked. The veto never transits: the local collision of a
// spectator client has no authority over the bot session, and the
// correction ends the client's local walk too, so the following
// reports ride the bot again.
func (gc *gameConn) discardClientVeto(payload []byte) {
    if len(payload) < 13 {
        return
    }
    reportX := int32(binary.LittleEndian.Uint32(payload[1:5]))
    reportY := int32(binary.LittleEndian.Uint32(payload[5:9]))
    x, y, ok := gc.sendViewCorrection()
    if !ok {
        return
    }
    if gc.vetoLogAt.IsZero() || time.Since(gc.vetoLogAt) > validateLogPeriod {
        gc.vetoLogAt = time.Now()
        gc.server.logger.Printf(
            "game#%d: client move veto at %d %d dropped, the bot "+
                "walks at %d %d",
            gc.id, reportX, reportY, x, y)
    }
}

// sendViewCorrection hands the client a ValidateLocation carrying the
// live place of the played character, the same answer the Mobius
// server sends an out of sync client. It reports the place it sent and
// false when the current session carries no usable one.
func (gc *gameConn) sendViewCorrection() (int32, int32, bool) {
    session := gc.currentSession()
    if session == nil {
        return 0, 0, false
    }
    objectID := session.tracker.SelfObjectID()
    if objectID == 0 {
        return 0, 0, false
    }
    selfX, selfY, selfZ, ok := session.tracker.SelfProjectedPosition()
    if !ok {
        return 0, 0, false
    }
    x := int32(selfX)
    y := int32(selfY)
    z := int32(selfZ)
    answer := make([]byte, 21)
    answer[0] = serverOpValidateLocation
    binary.LittleEndian.PutUint32(answer[1:5], uint32(objectID))
    binary.LittleEndian.PutUint32(answer[5:9], uint32(x))
    binary.LittleEndian.PutUint32(answer[9:13], uint32(y))
    binary.LittleEndian.PutUint32(answer[13:17], uint32(z))
    binary.LittleEndian.PutUint32(answer[17:21],
        uint32(session.tracker.SelfHeading()))
    _ = gc.sendToClient(answer)

    return x, y, true
}

// noteNetPingGameTime harvests the game time of one suppressed NetPing
// answer of the bot session so the locally synthesized client answers
// carry the value the real server would have sent.
func (gc *gameConn) noteNetPingGameTime(payload []byte) {
    if len(payload) < 5 {
        return
    }
    gameTime := int32(binary.LittleEndian.Uint32(payload[1:5]))
    gc.mu.Lock()
    gc.netPingTime = gameTime
    gc.mu.Unlock()
}
