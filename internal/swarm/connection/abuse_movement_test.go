// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// The abuse movement mode of the -abuse launch flag: every WalkTo of
// the session rides the movement abuse channel instead of the server
// side run. The channel contract pinned here mirrors the live verified
// route rounds - the keyboard mode arm latches the cursor key flag of
// the session once (a move request in mode 0), every walk after it is
// a single ValidatePosition claim the armed session adopts with no
// speed and no distance validation, and the arm repeats only when the
// client has evidence the flag dropped (a mouse mode request cleared
// it, or the claim echoes went silent past the grace window).

// quietMoves asserts that no move request leaves the session within
// the settle window.
func quietMoves(t *testing.T, moves <-chan []byte) {
    t.Helper()
    select {
    case payload := <-moves:
        t.Fatalf("the abuse walk leaked a move request: %v", payload)
    case <-time.After(300 * time.Millisecond):
    }
}

// quietValidations asserts that no position claim leaves the session
// within the settle window.
func quietValidations(t *testing.T, validations <-chan []byte) {
    t.Helper()
    select {
    case payload := <-validations:
        t.Fatalf("the walk leaked a position claim: %v", payload)
    case <-time.After(300 * time.Millisecond):
    }
}

// TestAbuseWalkArmsOnceThenClaimsOnly pins the packet shape of the
// abuse channel: the first walk latches the cursor key flag with a
// keyboard mode move request carrying the destination, then claims
// the destination; every later walk is a bare claim - one packet per
// destination, no run left on the wire.
func TestAbuseWalkArmsOnceThenClaimsOnly(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, tracker := newValidatingSession(t, server)
    client.EnableAbuseMovement()
    require.True(t, client.AbuseMovementEnabled())

    // The first walk: the keyboard mode arm aimed past the walk
    // request cap on the destination line, then the claim of the
    // destination itself.
    require.NoError(t, client.WalkTo(45000, 49500, -3000))
    target, origin, mode := awaitMove(t, moves)
    armX, armY := abuseArmTarget(45768, 49848, 45000, 49500)
    require.Equal(t, [3]int32{armX, armY, -3000}, target,
        "the arm aims past the 9900 unit walk cap on the destination line")
    armAim := math.Hypot(float64(armX-45768), float64(armY-49848))
    require.Greater(t, armAim, 9900.0,
        "the arm aim clears the walk request cap, so the arm never "+
            "starts the server side run it replaces")
    require.Equal(t, [3]int32{45768, 49848, -3056}, origin,
        "the arm carries the standing point as its origin")
    require.Equal(t, int32(0), mode,
        "the arm walks in the cursor key movement mode")
    x, y, z, heading := awaitValidation(t, validations)
    require.Equal(t, int32(45000), x)
    require.Equal(t, int32(49500), y)
    require.Equal(t, int32(-3000), z)
    require.Equal(t, tracker.SelfHeading(), heading,
        "the claim carries the tracked heading")

    // The server echo of the adopted claim: the armed session
    // broadcasts every claim back as the ValidateLocation of the own
    // character, which keeps the echo evidence of the re-arm check
    // fresh (the fake server never answers by itself).
    client.applyValidateLocation(
        buildValidateLocationAt(100, 45000, 49500, -3000, 0))

    // The second walk: the flag is latched and the echo is fresh, so
    // nothing re-arms - the walk is one bare claim.
    require.NoError(t, client.WalkTo(44000, 49000, -3100))
    quietMoves(t, moves)
    x, y, z, _ = awaitValidation(t, validations)
    require.Equal(t, int32(44000), x)
    require.Equal(t, int32(49000), y)
    require.Equal(t, int32(-3100), z)

    require.True(t, client.cursorKeyArmed.Load(),
        "the arm mirror holds after the walks")
    require.Equal(t, int64(2), client.abuseClaims.Load(),
        "one claim left per walk")
    require.True(t, client.claimsOwnStream.Load(),
        "the claims own the position stream")
}

// TestAbuseArmTargetClearsTheWalkCap pins the arm geometry: the aim
// sits abuseArmAim units from the standing point on the destination
// line - past the 9900 unit walk request cap, so the server refuses
// the arm's walk AFTER the mode 0 branch latched the cursor key flag
// and no run ever starts - and a degenerate line aims north.
func TestAbuseArmTargetClearsTheWalkCap(t *testing.T) {
    selfX, selfY := int32(45768), int32(49848)

    // The aim sits on the destination line, past the cap.
    x, y := abuseArmTarget(selfX, selfY, 45000, 49500)
    aim := math.Hypot(float64(x-selfX), float64(y-selfY))
    require.InDelta(t, abuseArmAim, aim, 2.0,
        "the aim distance is abuseArmAim units")
    require.Greater(t, aim, 9900.0)
    dx := float64(45000 - selfX)
    dy := float64(49500 - selfY)
    length := math.Hypot(dx, dy)
    require.InDelta(t, dx/length, float64(x-selfX)/aim, 0.001,
        "the aim keeps the destination direction")
    require.InDelta(t, dy/length, float64(y-selfY)/aim, 0.001)

    // A short destination still arms past the cap (the claim that
    // follows carries the short hop - the cursor key branch adopts
    // any distance).
    x, y = abuseArmTarget(selfX, selfY, 45800, 49900)
    aim = math.Hypot(float64(x-selfX), float64(y-selfY))
    require.Greater(t, aim, 9900.0)

    // The degenerate line (the destination on the standing point)
    // aims north.
    x, y = abuseArmTarget(selfX, selfY, selfX, selfY)
    require.Equal(t, selfX, x)
    require.Equal(t, selfY+int32(abuseArmAim), y)
}

// TestAbuseWalkSkipsTheStandingPoint pins the no-op guard: a walk to
// the standing point never leaves the wire (the server refuses the
// arm whose target matches the origin outright, and the claim of the
// current placement moves nothing).
func TestAbuseWalkSkipsTheStandingPoint(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, _ := newValidatingSession(t, server)
    client.EnableAbuseMovement()

    require.NoError(t, client.WalkTo(45768, 49848, -3000))
    quietMoves(t, moves)
    quietValidations(t, validations)
    require.Equal(t, int64(0), client.abuseClaims.Load())
}

// TestAbuseWalkRearmsAfterTheRawClickDisarm pins the disarm healing:
// a raw mouse mode click clears the server side cursor key flag (the
// MoveToLocation mode 1 branch), so the next abuse walk latches it
// again before its claim instead of streaming claims the server would
// only adopt through the silent desync branch.
func TestAbuseWalkRearmsAfterTheRawClickDisarm(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, _ := newValidatingSession(t, server)
    client.EnableAbuseMovement()

    // The arm plus the claim of the first walk, with the server echo
    // keeping the channel evidence fresh (the disarm below is then the
    // only reason the next walk re-arms).
    require.NoError(t, client.WalkTo(45000, 49500, -3000))
    _, _, mode := awaitMove(t, moves)
    require.Equal(t, int32(0), mode)
    _, _, _, _ = awaitValidation(t, validations)
    client.applyValidateLocation(
        buildValidateLocationAt(100, 45000, 49500, -3000, 0))

    // The raw click disarms the cursor key flag.
    require.NoError(t, client.ClickWalkTo(45700, 49800, -3040))
    _, _, mode = awaitMove(t, moves)
    require.Equal(t, int32(1), mode)
    require.False(t, client.cursorKeyArmed.Load(),
        "the raw click clears the arm mirror")

    // The next abuse walk re-arms before its claim.
    require.NoError(t, client.WalkTo(44500, 49200, -3020))
    _, _, mode = awaitMove(t, moves)
    require.Equal(t, int32(0), mode,
        "the walk after the disarm latches the flag again")
    _, _, _, _ = awaitValidation(t, validations)
}

// TestAbuseWalkRearmsWhenTheEchoesGoSilent pins the echo healing: an
// armed session echoes every adopted claim back as a ValidateLocation
// broadcast of the own character, so a claim stream whose echo went
// silent past the grace window rides a dropped flag and the next walk
// re-arms.
func TestAbuseWalkRearmsWhenTheEchoesGoSilent(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, _ := newValidatingSession(t, server)
    client.EnableAbuseMovement()

    // The first walk arms and claims, and the server echo keeps the
    // channel evidence fresh before the test silences it.
    require.NoError(t, client.WalkTo(45000, 49500, -3000))
    _, _, mode := awaitMove(t, moves)
    require.Equal(t, int32(0), mode)
    _, _, _, _ = awaitValidation(t, validations)
    client.applyValidateLocation(
        buildValidateLocationAt(100, 45000, 49500, -3000, 0))

    // The echo silence: the last self broadcast sits past the grace
    // window (the flag dropped or never latched - the claims would
    // degrade to the silent desync adoption without the readback).
    client.selfValidateAt.Store(
        time.Now().Add(-abuseEchoGrace - time.Second).UnixNano())

    // The next walk re-arms before its claim.
    require.NoError(t, client.WalkTo(44500, 49200, -3020))
    _, _, mode = awaitMove(t, moves)
    require.Equal(t, int32(0), mode,
        "the silent echo re-arms the cursor key flag")
    _, _, _, _ = awaitValidation(t, validations)
}

// TestWalkWithoutTheFlagKeepsTheMouseMode pins the default: without
// the -abuse flag every walk stays the plain mouse mode ground click
// the hunt loop always ran - no arm, no claim.
func TestWalkWithoutTheFlagKeepsTheMouseMode(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, _ := newValidatingSession(t, server)
    require.False(t, client.AbuseMovementEnabled())

    require.NoError(t, client.WalkTo(45000, 49500, -3000))
    _, _, mode := awaitMove(t, moves)
    require.Equal(t, int32(1), mode,
        "the plain walk stays the mouse mode ground click")
    quietValidations(t, validations)
    require.False(t, client.cursorKeyArmed.Load())
    require.Equal(t, int64(0), client.abuseClaims.Load())
}

// TestApplyValidateLocationRecordsTheSelfEcho pins the echo evidence
// bookkeeping: the ValidateLocation broadcast of the own character
// refreshes the timestamp the re-arm check of the abuse channel
// reads, while the broadcasts of other objects never do.
func TestApplyValidateLocationRecordsTheSelfEcho(t *testing.T) {
    validations := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(validations)

    client, tracker := newValidatingSession(t, server)
    require.Zero(t, client.selfValidateAt.Load())

    // The broadcast of another object leaves the echo stamp alone.
    client.applyValidateLocation(
        buildValidateLocationAt(2, 44900, 49900, -3050, 4096))
    require.Zero(t, client.selfValidateAt.Load())

    // The self broadcast records the echo moment.
    before := time.Now()
    client.applyValidateLocation(
        buildValidateLocationAt(100, 46000, 50000, -3050, 32768))
    echoAt := time.Unix(0, client.selfValidateAt.Load())
    require.False(t, echoAt.Before(before),
        "the self broadcast refreshes the echo timestamp")
    require.LessOrEqual(t, echoAt, time.Now().Add(time.Second))

    // The tracker placement followed the broadcast.
    x, y, _, ok := tracker.SelfPosition()
    require.True(t, ok)
    require.Equal(t, int32(46000), x)
    require.Equal(t, int32(50000), y)
}
