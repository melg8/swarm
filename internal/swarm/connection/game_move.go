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

// positionValidation is the send state of the client position validation:
// the placement of the last report and its moment, so the ticker answers a
// changed placement and a sparse heartbeat while the character stands still
// (see validatePositionIdleGap).
type positionValidation struct {
    x, y, z  int32
    heading  int32
    reported bool
    at       time.Time
}

// validatePosition sends the client position validation of the official
// client (ValidatePosition 0x48) when the observed placement changed or the
// idle heartbeat elapsed. The server builds half of its session view from
// this stream (the clientZ of the z adoption gate, the last position of the
// door logout check, the heading), and a silent session leaves that view at
// its defaults - the C1 adoption branch Math.abs(_z - player.getClientZ()) <
// 800 needs a live client z. The report carries exactly the placement the
// server itself broadcast (docs/protocol_description.md).
func (gc *GameClient) validatePosition(validation *positionValidation) error {
    if gc.tracker == nil {
        return nil
    }
    // The cursor key claims own the stream while the escape runs: the echo
    // of the broadcast position would lag them by one broadcast and snap
    // the character back a step each tick (see ClaimValidatePosition).
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

// WalkTo makes the character walk to a world point like a ground click of
// the official client (MoveToLocation 0x01 in mouse mode); this build of
// the Mobius C1 server has no click handler for ground items, so a drop
// never moves the character by itself.
func (gc *GameClient) WalkTo(x int32, y int32, z int32) error {
    selfX, selfY, selfZ, ok := gc.tracker.SelfPosition()
    if !ok {
        return errors.New("failed to walk: own position is unknown")
    }
    // The mouse click returns the session to the mouse movement mode: the
    // server clears the cursor key flag the escape armed
    // (MoveToLocation.runImpl), so the echo ticker owns the stream again.
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

// CursorKeyWalkTo sends the keyboard-mode move request (MoveToLocation 0x01
// in cursor key mode, the arrow keys of the official client): the server
// arms the cursor key movement of the session (the origin is adopted within
// the thousand unit window, the flag latches) and every later
// ValidatePosition moves the character without any click validation. A
// click-refusing cell answers no mouse click at all, so this stream is the
// movement that still walks it off; the hunt loop arms it only for the
// cursor key escape (hunt.Loop.beginCursorKeyEscape, docs/hunting.md).
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

// ClaimValidatePosition reports a claimed client position
// (ValidatePosition 0x48): while the cursor key movement is armed the server
// syncs the claimed placement straight into the world and broadcasts it, so
// the claims walk the character without any click validation
// (ValidatePosition.runImpl, the cursor key branch). The claims gate the
// echo ticker: the echo of the broadcast position would lag them by one
// broadcast and snap the character back a step each tick. The first
// mouse-mode walk clears the gate (see WalkTo).
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
