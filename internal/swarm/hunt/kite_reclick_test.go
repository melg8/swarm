// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The re-click ladder of the kite walk (issue #60, the rounds two
// and three; the straight-lane hold of the 2026-11-09 owner round):
// the acceptance dump of the third round named the dominant dead-click
// mechanism - the server answers specific kite destination cells with
// an instant bare ActionFailed. The tests pin the ladder: the paced
// re-issue, the refusal-evidence verdict, the silent probe, the click
// bound, the cycle-long walk window (the C1 disable window of the live
// pAtkSpd) and the stand-down paths (the arrival that stops the
// re-clicks but keeps the movement hold, the window expired, the dead
// endpoint holds the ground instead of rotating).

import (
    "testing"
    "time"

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
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1, "the kite step clicked once")

    // The click stayed silent: nothing moves, the pacing ages out.
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the ladder re-clicked the silent retreat walk")
    require.Equal(t, game.walks[0], game.walks[1],
        "a re-click inside the broadcast gate aims the same endpoint")
}

// TestKiteRefusalHoldsAtTheProbeAge pins the refusal-evidence
// verdict: the server answered the kite click with ActionFailed (the
// instant refusal the third-round acceptance dump shows - the
// MoveToLocation handler refuses specific destination cells). The
// verdict latches at once, but the spam keeps hitting the endpoint
// while the walk is young - the age gate separates a genuine click
// refusal from the straggler refusals of the server AI's own attack
// retries (the bow disable path answers ActionFailed at a one-second
// cadence while the character stands) and the movement broadcast of
// a healthy click still gets its chance. At the probe age the
// straight design has no second lane: the ladder stands down and the
// hold keeps the bow shooting.
func TestKiteRefusalHoldsAtTheProbeAge(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
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
        "no hold before the probe age")
    require.True(t, loop.kiteWalkDead,
        "the refusal evidence latched the dead verdict")

    // The walk ages past the probe: the hold lands. The straight
    // design has no fan to rotate onto (the owner rule of the
    // 2026-11-09 round), so the ladder clears and the archer keeps
    // shooting from where it stands.
    ageKiteReclick(loop)
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 2,
        "the refused endpoint is not rotated onto a fan lane")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down at the probe age")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the dead endpoint arms the hold for the target")
}

// TestKiteBowRetryRefusalIsNotAttributedToTheClick pins the bow
// retry guard: an attack request the server bounced through the bow
// disable schedules notifyActionReadyToAct one second ahead
// (Creature.canUseRangeWeapon), so its ActionFailed lands ~1.25 s
// later with no client request inside the tight answer window - the
// 14:06:50 dump of the round-19 run: request at 49.013, refusal at
// 50.263, the healthy retreat click of that moment latched as
// "refused by the server" and burned a fan candidate.
func TestKiteBowRetryRefusalIsNotAttributedToTheClick(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    clickAt := loop.kiteReclickAt
    bot.ApplyOtherRequestSent(
        clickAt.Add(-kiteBowRetryLead + 100*time.Millisecond))
    bot.ApplyActionFailed(clickAt.Add(10 * time.Millisecond))

    require.False(t, loop.kiteClickRefused(),
        "the bow retry chain owns the failure, not the click")
    loop.tick()
    require.False(t, loop.kiteWalkDead,
        "the healthy endpoint keeps its verdict")
}

// TestKiteRetreatClickRefusalStillLatches pins the other side of the
// guard: a refusal with no action request in the retry band belongs
// to the click and latches the dead verdict exactly as before.
func TestKiteRetreatClickRefusalStillLatches(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    clickAt := loop.kiteReclickAt
    // The last action request predates the retry band: the failure
    // answers the click.
    bot.ApplyOtherRequestSent(clickAt.Add(-5 * time.Second))
    bot.ApplyActionFailed(clickAt.Add(10 * time.Millisecond))

    require.True(t, loop.kiteClickRefused(),
        "a click refusal with no retry in flight still latches")
}

// TestKiteSilentProbeHoldsTheGround pins the silent probe: a walk
// unanswered past the probe age (no ActionFailed, no movement
// broadcast - the H-006 silent drop signature) names the endpoint
// dead, stands the ladder down and holds the ground for the bow.
func TestKiteSilentProbeHoldsTheGround(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The click stayed silent through the broadcast gate window. The
    // straight design has no fan to rotate onto, so the ladder
    // clears and the hold owns the cycle.
    ageKiteReclick(loop)
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the silent endpoint is not rotated onto a fan lane")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down at the probe age")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the silent endpoint arms the hold for the target")
}

// TestKiteRefusedDeadEndHoldsTheGround pins the cornered answer of
// the dead click: once the straight endpoint is refused and the walk
// ages past the probe, the ladder stands down - the last re-click
// does not grind a dead transport, and the hold answer keeps the bow
// shooting because there is no second lane to rotate onto.
func TestKiteRefusedDeadEndHoldsTheGround(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    // The straight lane passes the server click validation (the fake
    // navigator accepts every click); the refusal below is the LIVE
    // answer - the server takes the click and answers ActionFailed.
    loop.SetNavigator(&fakeNavigator{})
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the straight lane carried the step")
    require.Equal(t, [3]int32{44600, 50000, -3500}, game.walks[0])

    // The straight cell got refused and the walk aged past the
    // probe: the verdict is old enough to trust - the ladder stands
    // down and the hold owns the cycle.
    loop.tracker.ApplyActionFailed(time.Now())
    ageKiteWalkIssue(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a dead straight endpoint sends no fan re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the hold owns the cycle after the dead click")
}

// TestKiteYoungWalkSurvivesAStragglerRefusal pins the straggler
// guard: the server AI's own attack retries answer ActionFailed at
// a one second cadence through the bow disable, and a straggler of
// that cadence on a YOUNG walk (the movement broadcast still in
// flight) rotates the lane but never stands the ladder down - the
// broadcast gate must get its chance, the walk keeps running.
func TestKiteYoungWalkSurvivesAStragglerRefusal(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
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
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    for range kiteReclickLimit + 2 {
        ageKiteReclick(loop)
        loop.tick()
    }
    require.Len(t, game.walks, 1+kiteReclickLimit,
        "the ladder spends the click bound and stops")
}

// TestKiteReclickLadderSurvivesTheWalkAdoption pins the cycle-long
// ladder: the movement broadcast of the walk (the character is
// moving, SelfWalking true) does not stand the ladder down - a silent
// drop mid-window later needs the re-click machinery alive. The
// adoption refreshes the episode budget instead (a mid-window drop
// gets its own re-clicks), and a LATER standing drop re-bases the
// ladder onto the current cell and keeps pushing the SAME endpoint.
func TestKiteReclickLadderSurvivesTheWalkAdoption(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The click stays silent for two paced re-clicks: the episode
    // budget runs down while the character stands on the issue
    // cell.
    for range 2 {
        ageKiteReclick(loop)
        loop.tick()
    }
    require.Len(t, game.walks, 3)
    require.Equal(t, 2, loop.kiteReclicks,
        "the silent clicks spent the episode budget")

    // The server answers with the movement start broadcast: the
    // character walks toward the endpoint. The adoption keeps the
    // ladder armed and resets the episode budget - no re-click
    // fights the running walk.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 44600, DestY: 50000, DestZ: -3500,
    })
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 3,
        "a running walk needs no re-click")
    require.Zero(t, loop.kiteReclicks,
        "the adoption resets the episode budget")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stays armed through the running walk")

    // The walk drops mid-window (no more movement broadcasts, the
    // character stands 200 into the 400 leg): the ladder re-bases
    // onto the drop cell and re-clicks the SAME endpoint.
    moveSelfTo(bot, 44800, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 4,
        "the mid-window drop re-clicks the endpoint")
    require.Equal(t, game.walks[0], game.walks[3],
        "the recovery aims the same endpoint")
    require.Equal(t, int32(44800), loop.kiteWalkBaseX,
        "the ladder re-based onto the drop cell")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the recovery keeps the ladder alive")
}

// TestKiteReclickLadderCompletesAtTheEndpoint pins the arrival rule
// of the cycle-long ladder: a stand WITHIN kiteArrivalEpsilon of the
// endpoint completes the leg (the server's cell granularity and a
// partial route's last walkable cell never land the character
// exactly on the clicked cell) - the ladder stops re-clicking, but
// the walk stays armed so the movement hold survives until the bow
// disable end (the forced attack must not fire at the collapsed
// arrival range).
func TestKiteReclickLadderCompletesAtTheEndpoint(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The character arrives 50 units short of the clicked endpoint
    // (44600): inside the 100 unit arrival radius.
    moveSelfTo(bot, 44650, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "an arrived leg needs no re-click")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the movement hold survives the early arrival")
}

// TestKiteReclickLadderExpiresWithTheWindow pins the window bound:
// past the kite movement window the forced attack re-request owns
// the tick, and a re-click there would only fight the re-engage.
func TestKiteReclickLadderExpiresWithTheWindow(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
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
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The pacing is fresh (the initial click just went out): the
    // ladder waits.
    loop.tick()
    require.Len(t, game.walks, 1,
        "a fresh pacing sends no re-click")
}

// TestKiteBowWindowSpendsTheCooldown pins the bow-speed window: the
// live pAtkSpd the StatusUpdate broadcasts drives the C1 disable
// formula (500000/pAtkSpd + reuseDelay*333/pAtkSpd, the reuse 1500 of
// the bow family) - the walk spends the whole cooldown the server
// enforces between the shots ("its like 3 seconds to draw shot" - the
// owner's own measurement of the Short Bow kit), so the forced attack
// fires exactly when the bow is ready.
func TestKiteBowWindowSpendsTheCooldown(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The equipping broadcast of the bow: pAtkSpd 337 (the value
    // the acceptance dump carries for the Short Bow kit).
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrAtkSpd, Value: 337},
    })
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The C1 disable formula at the live attack speed. A variable
    // (not a constant) keeps the formula a runtime conversion - a
    // constant one is not representable as the integer Duration.
    pAtkSpd := 337.0
    want := time.Duration(
        ((500000.0 + kiteBowReuseDelay*333.0) / pAtkSpd) *
            float64(time.Millisecond))
    aged := loop.kiteWalkUntil.Sub(loop.kiteWalkIssuedAt)
    require.InDelta(t, float64(want), float64(aged),
        float64(50*time.Millisecond),
        "the walk spends the whole C1 disable window")
}

// TestKiteBowWindowFallsBackWithoutTheSpeed pins the fallback: no
// pAtkSpd broadcast (the packet never arrived) keeps the shipped
// fixed window - the walk contract never depends on the packet being
// parsed.
func TestKiteBowWindowFallsBackWithoutTheSpeed(t *testing.T) {
    bot, _, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Zero(t, bot.SelfPAtkSpd(),
        "the scene never broadcast the attack speed")
    aged := loop.kiteWalkUntil.Sub(loop.kiteWalkIssuedAt)
    require.InDelta(t, float64(kiteStepWindow), float64(aged),
        float64(50*time.Millisecond),
        "the missing speed falls back to the shipped window")
}
