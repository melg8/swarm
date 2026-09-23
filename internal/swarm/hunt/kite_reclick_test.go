// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The re-click ladder of the kite walk (issue #60, the rounds two
// and three): the acceptance dump of the third round named the
// dominant dead-click mechanism - the server answers specific kite
// destination cells with an instant bare ActionFailed while the
// rotated fan lanes walk fine. The tests pin the ladder: the paced
// re-issue, the refusal-evidence rotation (at once, no probe wait),
// the silent probe rotation, the refused-cell memory (a refused
// cell is never re-clicked), the click bound, the bow-speed window
// (the C1 disable formula on the live pAtkSpd) and the stand-down
// paths (the walk running, the character moved, the window
// expired, nothing left to rotate onto).

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// ageKiteReclick backdates the ladder pacing so the next tick's
// re-click passes the kiteReclickPeriod gate - the test twin of the
// 250ms the live loop spends between the clicks.
func ageKiteReclick(loop *Loop) {
    loop.kiteReclickAt = time.Now().Add(-kiteReclickPeriod - time.Millisecond)
}

// ageKiteWalkIssue backdates the walk issue stamp so the next
// re-click passes the silent probe age gate - the test twin of the
// 1.2s the live walk may legitimately spend waiting for the
// movement broadcast.
func ageKiteWalkIssue(loop *Loop) {
    loop.kiteWalkIssuedAt = time.Now().Add(-kiteProbeElapsed - time.Millisecond)
}

// TestKiteReclickLadderReissuesTheDeadClick pins the core behavior
// of the ladder: a kite walk the server never answered (no movement
// broadcast, no refusal, the character standing on the issue cell)
// gets its click re-issued at the pacing - toward exactly the same
// endpoint, because inside the once-per-second broadcast gate a
// healthy accepted click may legitimately not have answered yet.
func TestKiteReclickLadderReissuesTheDeadClick(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1, "the kite step clicked once")

    // The click stayed silent: nothing moves, the pacing ages out.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the ladder re-clicked the silent retreat walk")
    require.Equal(t, game.walks[0], game.walks[1],
        "a re-click inside the broadcast gate aims the same endpoint")
}

// TestKiteRefusalRotatesAtTheProbeAge pins the refusal-evidence
// rotation: the server answered the kite click with ActionFailed
// (the instant refusal the third-round acceptance dump shows - the
// MoveToLocation handler refuses specific destination cells). The
// verdict latches at once, the ROTATION lands at the probe age -
// the age gate separates a genuine click refusal from the straggler
// refusals of the server AI's own attack retries (the bow disable
// path answers ActionFailed at a one-second cadence while the
// character stands), and a young walk keeps its endpoint so the
// movement broadcast of a healthy click still clears the ladder.
func TestKiteRefusalRotatesAtTheProbeAge(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The server's refusal answer lands right after the click: the
    // verdict latches, the young walk keeps its endpoint (the age
    // gate separates the straggler refusals of the server AI's own
    // attack retries from a genuine click refusal).
    loop.tracker.ApplyActionFailed(time.Now())
    loop.tick()
    require.Len(t, game.walks, 2,
        "the young walk re-clicks its endpoint")
    require.Equal(t, game.walks[0], game.walks[1],
        "no rotation before the probe age")
    require.True(t, loop.kiteWalkDead,
        "the refusal evidence latched the dead verdict")

    // The walk ages past the probe: the rotation lands.
    ageKiteReclick(loop)
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 3)
    require.NotEqual(t, game.walks[1], game.walks[2],
        "the refused endpoint rotated onto the fan lane")
    require.Equal(t, [3]int32{44717, 49717, -3500}, game.walks[2],
        "the rotation takes the next fan candidate")
}

// TestKiteRefusalNeverReClicksTheRefusedCell pins the refused-cell
// memory: consecutive refusals rotate through the fan candidates and
// never fold back onto a cell the server already refused.
func TestKiteRefusalNeverReClicksTheRefusedCell(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    refused := map[[2]int32]bool{}
    for i := range 4 {
        loop.tracker.ApplyActionFailed(time.Now())
        ageKiteReclick(loop)
        ageKiteWalkIssue(loop)
        loop.tick()
        require.Len(t, game.walks, 2+i,
            "each aged refusal rotates and re-clicks")
        endpoint := [2]int32{game.walks[1+i][0], game.walks[1+i][1]}
        require.False(t, refused[endpoint],
            "the rotation never re-clicks a refused cell: %v", endpoint)
        refused[[2]int32{game.walks[i][0], game.walks[i][1]}] = true
    }
}

// TestKiteSilentProbeRotatesTheDeadEndpoint pins the silent probe:
// a walk unanswered past the probe age (no ActionFailed, no movement
// broadcast - the H-006 silent drop signature) names the endpoint
// dead and rotates the same way the refusal evidence does.
func TestKiteSilentProbeRotatesTheDeadEndpoint(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The click stayed silent through the broadcast gate window.
    ageKiteReclick(loop)
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 2)
    require.True(t, loop.kiteWalkDead, "the silent probe latched")
    require.NotEqual(t, game.walks[0], game.walks[1],
        "the silent endpoint rotated onto the fan lane")
    require.Equal(t, [3]int32{44717, 49717, -3500}, game.walks[1])
}

// TestKiteRefusalWithNoLaneLeftStandsDown pins the cornered answer
// of the rotation: when every candidate of the away hemisphere is
// refused or blocked, the ladder stands down - the last re-click
// does not grind a dead transport, the hold answer owns the cycle.
func TestKiteRefusalWithNoLaneLeftStandsDown(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    // Only the straight west lane passes the line of sight; every
    // fan candidate is walled.
    nav := &fakeNavigator{}
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return int32(to.X) == 44600, nil
    }
    loop.SetNavigator(nav)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the straight lane carried the step")
    require.Equal(t, [3]int32{44600, 50000, -3500}, game.walks[0])

    // The straight cell got refused and the walk aged past the
    // probe: the rotation has no alternative lane (the fans are
    // walled) and the verdict is old enough to trust - the ladder
    // stands down.
    loop.tracker.ApplyActionFailed(time.Now())
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a rotation with no lane left sends no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down")
}

// TestKiteYoungWalkSurvivesAStragglerRefusal pins the straggler
// guard: the server AI's own attack retries answer ActionFailed at
// a one second cadence through the bow disable, and a straggler of
// that cadence on a YOUNG walk (the movement broadcast still in
// flight) rotates the lane but never stands the ladder down - the
// broadcast gate must get its chance, the walk keeps running.
func TestKiteYoungWalkSurvivesAStragglerRefusal(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // A straggler refusal answer lands right after the initial
    // click (the walk is young - the broadcast gate still open).
    loop.tracker.ApplyActionFailed(time.Now())
    loop.tick()
    require.Len(t, game.walks, 2,
        "the young walk re-clicks through the straggler")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "a young walk survives the straggler refusal")
}

// TestKiteReclickLadderBoundsTheClicks pins the packet budget of
// the ladder: one initial click plus kiteReclickLimit re-issues - a
// dead silent transport spends the bound and stops.
func TestKiteReclickLadderBoundsTheClicks(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    for range kiteReclickLimit + 2 {
        ageKiteReclick(loop)
        loop.tick()
    }
    require.Len(t, game.walks, 1+kiteReclickLimit,
        "the ladder spends the click bound and stops")
}

// TestKiteReclickLadderStandsDownWhenTheWalkRuns pins the
// SelfWalking oracle: the movement broadcast of the walk (the
// character is moving) clears the ladder - the click landed, the
// manual-clicking replication stops, no re-click fights the running
// walk.
func TestKiteReclickLadderStandsDownWhenTheWalkRuns(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The server answered the click with the movement start
    // broadcast: the character walks toward the endpoint.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 44600, DestY: 50000, DestZ: -3500,
    })
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a running walk needs no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down on the movement broadcast")
}

// TestKiteReclickLadderQuietsWhenTheCharacterMoved pins the
// base-cell oracle: a character that left the issue cell tells the
// ladder the click moved something after all - the ladder stands
// down, the next shot cycle re-arms it.
func TestKiteReclickLadderQuietsWhenTheCharacterMoved(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The character arrived one cell off the issue point (a short
    // wobble walk, standing again).
    moveSelfTo(bot, 45020, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a character off the issue cell needs no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down on the base-cell change")
}

// TestKiteReclickLadderExpiresWithTheWindow pins the window bound:
// past the kite movement window the forced attack re-request owns
// the tick, and a re-click there would only fight the re-engage.
func TestKiteReclickLadderExpiresWithTheWindow(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The window burned with the character standing.
    loop.kiteWalkUntil = time.Now().Add(-time.Millisecond)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "past the window the re-engage owns the tick")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down at the window end")
}

// TestKiteProximityStepArmsTheReclickLadder pins the shared seam:
// the proximity kite (the fight view refreshed by a chase that
// ended, no shot committed) issues its walk through the same
// ladder - the dead-click recovery belongs to the walk, not to the
// trigger that armed it.
func TestKiteProximityStepArmsTheReclickLadder(t *testing.T) {
    bot, game, loop := kiteBowBotChaseFresh(t, 45200)
    // The chase ended (the arrival broadcast): the character
    // stands again while the fight view stays fresh - the proximity
    // trigger owns the step, not the shot.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45000, DestY: 50000, DestZ: -3500,
    })
    loop.tick()
    require.Len(t, game.walks, 1,
        "the proximity step fired on the closed mob")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the ladder is armed on the proximity walk")

    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the proximity walk re-clicks through the same ladder")
    require.Equal(t, game.walks[0], game.walks[1])
}

// TestKiteReclickLadderDoesNotFirePastTheFreshPacing pins the
// pacing gate itself: a ladder tick inside the re-click window with
// no refusal evidence sends nothing - the repeated clicks are
// paced, not sprayed.
func TestKiteReclickLadderDoesNotFirePastTheFreshPacing(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The pacing is fresh (the initial click just went out): the
    // ladder waits.
    loop.tick()
    require.Len(t, game.walks, 1,
        "a fresh pacing sends no re-click")
}

// TestKiteBowWindowSpendsTheCooldown pins the bow-speed window: the
// live pAtkSpd the StatusUpdate broadcasts drives the C1 disable
// formula (500000/pAtkSpd + reuseDelay*333/pAtkSpd, the reuse 1500
// of the bow family) - the walk spends the whole cooldown the
// server enforces between the shots ("its like 3 seconds to draw
// shot" - the owner's own measurement of the Short Bow kit).
func TestKiteBowWindowSpendsTheCooldown(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The equipping broadcast of the bow: pAtkSpd 337 (the value
    // the acceptance dump carries for the Short Bow kit).
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrAtkSpd, Value: 337},
    })
    before := time.Now()
    loop.tick()
    require.Len(t, game.walks, 1)

    // The C1 disable formula: (500000 + reuse*333)/pAtkSpd.
    const pAtkSpd = 337.0
    wantMS := (500000.0 + kiteBowReuseDelay*333.0) / pAtkSpd
    want := time.Duration(wantMS * float64(time.Millisecond))
    aged := time.Until(loop.kiteWalkUntil)
    require.InDelta(t, want, aged, float64(250*time.Millisecond),
        "the walk window matches the C1 disable formula")
    require.Greater(t, aged, kiteStepWindow,
        "the bow window outlasts the shipped fixed window")
    _ = before
    _ = game
}

// TestKiteBowWindowFallsBackWithoutTheSpeed pins the fallback: no
// pAtkSpd broadcast (the packet never arrived) keeps the shipped
// fixed window - the walk contract never depends on the packet
// being parsed.
func TestKiteBowWindowFallsBackWithoutTheSpeed(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Zero(t, bot.SelfPAtkSpd(),
        "the scene never broadcast the attack speed")
    aged := time.Until(loop.kiteWalkUntil)
    require.InDelta(t, kiteStepWindow, aged,
        float64(250*time.Millisecond),
        "the missing speed falls back to the shipped window")
}
