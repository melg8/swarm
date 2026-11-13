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

// clientViewDiverged reports whether one client position report contradicts the
// projected live place by more than one move speed plus
// validateDivergenceSlack. The band under the server's 500 unit correction
// threshold is the silent snap window this check closes; the projected place is
// compared, not the last broadcast origin, which sits a walk segment behind a
// moving character (docs/proxy.md, "The client position reports").
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

// correctClientView drops one diverged report and answers it with the
// ValidateLocation the Mobius server sends an out of sync client: the
// correction heals the stale pawn view and the report never reaches
// the server, severing the position feedback loop. The answer may
// repeat per report; the log line is rate limited.
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
// answers it with the same correction: the Mobius handler throws the
// reported place away and cancels the move intention, which would stop
// the bot's server side walk. The local collision of a spectator
// client has no authority over the bot session, and the correction
// ends the client's local walk (docs/proxy.md, "The client move veto").
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
