// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "errors"
    "fmt"
    "time"

    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// positionValidation is the send state of the client position
// validation loop: the placement of the last report and its moment,
// so the ticker sends only what the official client sends - a
// validation whenever the observed placement changed (the movement
// rate) and a sparse heartbeat while the character stands still (see
// validatePositionIdleGap).
type positionValidation struct {
    x, y, z  int32
    heading  int32
    reported bool
    at       time.Time
}

// validatePosition sends the client position validation of the
// official client (ValidatePosition 0x48) when the observed placement
// of the character changed since the last report, or when the idle
// heartbeat elapsed while the character stood still. The official
// client streams this packet from its own movement simulation and the
// server builds half of its session view from it: the clientZ the z
// adoption gate reads, the last server position the door logout
// exploit check compares, the client heading. A bot that never
// validates leaves that view frozen at its defaults - the C1
// clientZ adoption branch (Math.abs(_z - player.getClientZ()) < 800)
// can never run for a session whose client z the server still reads
// as zero - and every deployment whose movement validation expects
// the client liveness signal answers the silence its own way. The
// report carries exactly the tracker placement the server itself
// broadcast: nothing is claimed the server did not say.
func (gc *GameClient) validatePosition(validation *positionValidation) error {
    if gc.tracker == nil {
        return nil
    }
    // The cursor key claims own the stream while the escape runs:
    // the echo of the broadcast position would lag the claims by one
    // broadcast and snap the character back a step each tick while
    // the server's cursor key flag holds (the claimed placements
    // move the character unconditionally, see
    // ClaimValidatePosition).
    if gc.claimsOwnStream.Load() {
        return nil
    }
    x, y, z, ok := gc.tracker.SelfPosition()
    if !ok {
        return nil
    }
    heading := gc.tracker.SelfHeading()
    now := time.Now()
    changed := !validation.reported || x != validation.x || y != validation.y ||
        z != validation.z || heading != validation.heading
    idle := now.Sub(validation.at) >= validatePositionIdleGap
    if !changed && !idle {
        return nil
    }
    request := togameserver.NewValidatePositionPacket()
    request.X = x
    request.Y = y
    request.Z = z
    request.Heading = heading
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send validate position: %w", err)
    }
    validation.x, validation.y, validation.z = x, y, z
    validation.heading = heading
    validation.reported = true
    validation.at = now

    return nil
}

// WalkTo makes the character walk to a world point, exactly like a
// ground click of the official client (client MoveToLocation 0x01 in
// mouse mode). The hunt loop uses it to run toward a drop: this build of
// the Mobius C1 server has no click handler for ground items, so the
// character would otherwise never move toward the loot by itself.
func (gc *GameClient) WalkTo(x int32, y int32, z int32) error {
    selfX, selfY, selfZ, ok := gc.tracker.SelfPosition()
    if !ok {
        return errors.New("failed to walk: own position is unknown")
    }
    // The mouse click returns the session to the mouse movement
    // mode: the server clears the cursor key flag the cursor key
    // escape armed (MoveToLocation.runImpl), so the echo ticker of
    // the client position validation owns the stream again (see
    // claimsOwnStream).
    gc.claimsOwnStream.Store(false)
    request := togameserver.NewMoveToLocationRequestPacket()
    request.TargetX = x
    request.TargetY = y
    request.TargetZ = z
    request.OriginX = selfX
    request.OriginY = selfY
    request.OriginZ = selfZ
    request.Mode = togameserver.MoveModeMouse
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send move to location: %w", err)
    }

    return nil
}

// CursorKeyWalkTo sends the keyboard-mode move request (MoveToLocation
// 0x01 in cursor key mode, the arrow keys of the official client): the
// server arms the cursor key movement of the session - the packet's
// origin is adopted within the thousand unit window and the cursor key
// flag latches - and every ValidatePosition the session streams
// afterwards moves the character server-side without any click
// validation (ValidatePosition.runImpl syncs the claimed placement
// into the world and broadcasts it). The ground a click-refusing cell
// owns answers no mouse click at all; the cursor key stream is the
// movement that still walks it off (the 2026-09-14 15:10 report: even
// the official client stood frozen on the village plaza cell until
// the player walked the arrows). The hunt loop arms it only for the
// cursor key escape of a click-refusing stuck (see
// hunt.Loop.beginCursorKeyEscape).
func (gc *GameClient) CursorKeyWalkTo(
    x int32, y int32, z int32,
) error {
    selfX, selfY, selfZ, ok := gc.tracker.SelfPosition()
    if !ok {
        return errors.New(
            "failed to walk with the cursor keys: own position is unknown")
    }
    request := togameserver.NewMoveToLocationRequestPacket()
    request.TargetX = x
    request.TargetY = y
    request.TargetZ = z
    request.OriginX = selfX
    request.OriginY = selfY
    request.OriginZ = selfZ
    request.Mode = togameserver.MoveModeCursorKeys
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf(
            "failed to send the cursor key move: %w", err)
    }

    return nil
}

// ClaimValidatePosition reports a CLAIMED client position of the
// character (ValidatePosition 0x48): while the server's cursor key
// movement is armed, the claimed placement is synced straight into
// the world and broadcast (ValidatePosition.runImpl - the cursor key
// branch), so the claims walk the character without any click
// validation. This is the packet stream the official client drives
// from its own movement simulation while the player walks with the
// arrows - the arrow walk being the only movement a click-refusing
// cell answers (the 2026-09-14 15:10 report). The claims gate the
// echo ticker (see claimsOwnStream): the echo of the broadcast
// position would otherwise lag the claims by one broadcast and snap
// the character back a step each tick while the cursor key flag
// holds. The first mouse-mode walk clears the gate (see WalkTo).
func (gc *GameClient) ClaimValidatePosition(
    x int32, y int32, z int32, heading int32,
) error {
    request := togameserver.NewValidatePositionPacket()
    request.X = x
    request.Y = y
    request.Z = z
    request.Heading = heading
    gc.claimsOwnStream.Store(true)
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf(
            "failed to send the claimed position: %w", err)
    }

    return nil
}
