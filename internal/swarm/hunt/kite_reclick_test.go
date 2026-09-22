// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The re-click ladder of the kite walk (issue #60, the second
// round): the owner's follow-up dump showed the rhythm trigger
// working with the character still standing through every reload -
// the single retreat click barely executed ("moving: no", the mob
// closing ~380 units per cycle), while the manual clicking behind
// the character "runs 2 seconds no problem, covering large
// distance". The tests pin the ladder: the dead click re-issued at
// the pacing, the same endpoint first, the probe verdict and the
// fan rotation at the probe ordinal, the click bound, and the
// stand-down paths (the walk running, the character moved, the
// window expired).

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// ageKiteReclick backdates the ladder pacing so the next tick's
// re-click passes the kiteReclickPeriod gate - the test twin of the
// 600ms the live loop spends between the clicks.
func ageKiteReclick(loop *Loop) {
    loop.kiteReclickAt = time.Now().Add(-kiteReclickPeriod - time.Millisecond)
}

// TestKiteReclickLadderReissuesTheDeadClick pins the core behavior
// of the ladder: a kite walk the server never answered (no movement
// broadcast, the character standing on the issue cell) gets its
// click re-issued at the pacing - toward exactly the same endpoint,
// because at the first re-click the broadcast of a healthy accepted
// click may legitimately not have arrived yet.
func TestKiteReclickLadderReissuesTheDeadClick(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1, "the kite step clicked once")

    // The click died silently: nothing moves, the pacing ages out.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the ladder re-clicked the dead retreat walk")
    require.Equal(t, game.walks[0], game.walks[1],
        "the first re-click aims the same endpoint")

    // The second dead click ages out too - still the same endpoint
    // before the probe ordinal, then the rotation below owns it.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 3,
        "the ladder keeps re-clicking while the character stands")
}

// TestKiteReclickLadderRotatesTheDeadEndpoint pins the probe and
// the rotation: the second re-click (the probe ordinal) latches the
// dead-click verdict and rotates the endpoint onto the next fan
// candidate - a cell the server refuses never starts a walk, so
// clicking it again changes nothing. The third re-click retries the
// rotated lane.
func TestKiteReclickLadderRotatesTheDeadEndpoint(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)

    // The first re-click: the same endpoint, no verdict yet.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2)
    require.Equal(t, game.walks[0], game.walks[1])
    require.False(t, loop.kiteWalkDead,
        "the first re-click is inside the broadcast gate - no verdict")

    // The second re-click: the probe latches, the endpoint rotates.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 3)
    require.True(t, loop.kiteWalkDead,
        "the probe latched the dead click at the probe ordinal")
    require.NotEqual(t, game.walks[1], game.walks[2],
        "the dead endpoint rotated onto the fan lane")
    // The straight lane west died; the +45 degree fan candidate
    // serves the retreat instead.
    require.Equal(t, [3]int32{44717, 49717, -3500}, game.walks[2],
        "the rotation takes the next fan candidate")

    // The third re-click: the rotated lane retried as-is.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 4)
    require.Equal(t, game.walks[2], game.walks[3],
        "the last re-click retries the rotated lane")
}

// TestKiteReclickLadderBoundsTheClicks pins the packet budget of
// the ladder: one initial click plus kiteReclickLimit re-issues
// inside the movement window - a dead transport spends the bound
// and stops, the next shot cycle owns the retry.
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
// ladder the click moved something after all (the ground truth
// moved before the broadcast, or the walk already ran and finished
// short) - the ladder stands down, the next shot cycle re-arms it.
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
// the tick (the re-engage restarts the shooting from the opened
// distance), and a re-click there would only fight the re-engage.
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

// TestKiteReclickLadderKeepsTheDeadEndpointWhenCornered pins the
// rotation fallback: a lane battery that finds no alternative (the
// fan candidates all blocked behind) keeps the dead endpoint - the
// last re-click still retries the transport, the hold answer
// belongs to the next shot cycle.
func TestKiteReclickLadderKeepsTheDeadEndpointWhenCornered(t *testing.T) {
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

    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2)

    // The probe ordinal: the rotation finds no alternative lane,
    // the dead endpoint keeps serving the re-click.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 3)
    require.True(t, loop.kiteWalkDead, "the probe latched")
    require.Equal(t, game.walks[1], game.walks[2],
        "a cornered rotation keeps the dead endpoint")
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
// pacing gate itself: a ladder tick inside the 600ms window sends
// nothing - the repeated clicks are paced, not sprayed.
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
