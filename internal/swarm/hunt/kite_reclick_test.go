// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The re-click ladder of the kite walk (issue #60, the rounds two
// and three, reworked leg-long in round 17; the straight-lane hold of
// the 2026-11-09 owner round): the acceptance dump of the third
// round named the dominant dead-click mechanism - the server answers
// specific kite destination cells with an instant bare ActionFailed.
// The tests pin the ladder: the paced re-issue, the refusal-evidence
// verdict, the silent probe, the click bound, the leg-scaled walk
// window (the C1 disable formula on the live pAtkSpd at the
// per-unit pace of the race leg) and the stand-down paths (the leg
// completed at the endpoint, the window expired, the dead endpoint
// holds the ground instead of rotating).

import (
    "math"
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
    // Only the straight west lane of the race leg passes the line
    // of sight.
    nav := &fakeNavigator{}
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return int32(to.X) == 44278, nil
    }
    loop.SetNavigator(nav)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the straight lane carried the step")
    require.Equal(t, [3]int32{44278, 50000, -3500}, game.walks[0])

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

// TestKiteReclickLadderSurvivesTheWalkAdoption pins the leg-long
// ladder of round 17: the movement broadcast of the walk (the
// character is moving, SelfWalking true) no longer stands the ladder
// down - the race legs run many times the ordinary step's walk time
// and a silent drop later needs the re-click machinery alive. The
// adoption refreshes the episode budget instead (a mid-leg drop gets
// its own re-clicks), and a LATER standing drop mid-route re-bases
// the ladder onto the current cell and keeps pushing the SAME
// endpoint - the recovery the window-sized ladder never had (a
// silent drop mid-leg used to stand the character through the rest
// of the window while the chaser collected the difference).
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
        DestX: 44278, DestY: 50000, DestZ: -3500,
    })
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 3,
        "a running walk needs no re-click")
    require.Zero(t, loop.kiteReclicks,
        "the adoption resets the episode budget")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stays armed through the running walk")

    // The walk drops mid-route (no more movement broadcasts, the
    // character stands 400 into the 722 leg): the ladder re-bases
    // onto the drop cell and re-clicks the SAME endpoint.
    moveSelfTo(bot, 44600, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 4,
        "the mid-route drop re-clicks the endpoint")
    require.Equal(t, game.walks[0], game.walks[3],
        "the recovery aims the same endpoint")
    require.Equal(t, int32(44600), loop.kiteWalkBaseX,
        "the ladder re-based onto the drop cell")
    require.False(t, loop.kiteWalkUntil.IsZero(),
        "the recovery keeps the ladder alive")
}

// TestKiteReclickLadderCompletesAtTheEndpoint pins the arrival
// stand-down of the leg-long ladder: a stand WITHIN
// kiteArrivalEpsilon of the endpoint completes the leg (the server's
// cell granularity and a partial route's last walkable cell never
// land the character exactly on the clicked cell) - the ladder
// stands down, the window's remainder belongs to the arrival shot's
// own cycle.
func TestKiteReclickLadderCompletesAtTheEndpoint(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The character arrives 50 units short of the clicked endpoint
    // (44278): inside the 100 unit arrival radius. The mob chases to
    // 500 units: inside the bow stall radius (the chase watchdog owns
    // only the stretches beyond it) and beyond the re-shot floor (the
    // arrival chain preserves the stand of a safe shot).
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 44828, Y: 50000, Name: "Keltir",
    })
    moveSelfTo(bot, 44328, 50000, -3500)
    ageKiteReclick(loop)
    loop.tick()
    require.Len(t, game.walks, 1,
        "an arrived leg needs no re-click")
    require.True(t, loop.kiteWalkUntil.IsZero(),
        "the ladder stood down at the arrival")
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

// TestKiteBowWindowSpendsTheCooldown pins the leg-scaled
// bow-speed window: the live pAtkSpd the StatusUpdate broadcasts
// drives the C1 disable formula (500000/pAtkSpd +
// reuseDelay*333/pAtkSpd, the reuse 1500 of the bow family) at the
// PER-UNIT pace of the race leg - the walk spends the whole
// cooldown the server enforces between the shots ("its like 3
// seconds to draw shot" - the owner's own measurement of the Short
// Bow kit) for every kiteStep of the leg, so the 722 opening leg of
// the closed mob (the ring budget around its stand) holds the
// movement for ~5.36s from the issue: nothing interrupts the leg
// before it reaches its endpoint.
func TestKiteBowWindowSpendsTheCooldown(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    // The equipping broadcast of the bow: pAtkSpd 337 (the value
    // the acceptance dump carries for the Short Bow kit).
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrAtkSpd, Value: 337},
    })
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1)

    // The C1 disable formula per kiteStep of the walked leg:
    // (500000 + reuse*333)/pAtkSpd * leg/400.
    const pAtkSpd = 337.0
    leg := math.Hypot(
        float64(game.walks[0][0]-45000), float64(game.walks[0][1]-50000))
    wantMS := (500000.0 + kiteBowReuseDelay*333.0) / pAtkSpd
    want := time.Duration(
        wantMS * float64(time.Millisecond) * leg / kiteStep)
    aged := loop.kiteWalkUntil.Sub(loop.kiteWalkIssuedAt)
    require.InDelta(t, float64(want), float64(aged),
        float64(50*time.Millisecond),
        "the walk window scales with the leg at the C1 disable pace")
    require.Greater(t, aged,
        time.Duration(wantMS*float64(time.Millisecond)),
        "the race leg window outlasts the plain cooldown window")
}

// TestKiteBowWindowFallsBackWithoutTheSpeed pins the fallback: no
// pAtkSpd broadcast (the packet never arrived) keeps the shipped
// fixed window pace - the walk contract never depends on the packet
// being parsed - and the leg scaling still applies on top of it.
func TestKiteBowWindowFallsBackWithoutTheSpeed(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    tickPastTheWindup(loop)
    require.Zero(t, bot.SelfPAtkSpd(),
        "the scene never broadcast the attack speed")
    // The shipped fixed window at the leg pace: 722/400 * 2s.
    leg := math.Hypot(
        float64(game.walks[0][0]-45000), float64(game.walks[0][1]-50000))
    aged := loop.kiteWalkUntil.Sub(loop.kiteWalkIssuedAt)
    require.InDelta(t, leg/kiteStep*float64(kiteStepWindow),
        float64(aged), float64(50*time.Millisecond),
        "the missing speed falls back to the shipped window at the "+
            "leg pace")
}
