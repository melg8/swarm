// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "io"
    "log"
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The reproduction of the 2026-09-14 10:18 state dump report (build
// c22d529, bot test3, phase townReturn, uptime 7m24s): the level 15
// character stood at x 45768 y 49848 z -3056 (the elven village
// center) and never moved a single cell through NINE aborted trips
// while the corridor bans grew (43512 50504 widened to r768, then the
// fresh bans 44104 49544, 44200 49560, 44296 49576, 44392 49592,
// 44488 49608 - the whole northern exit sealed for the session), the
// direct zone legs ground ("moved nothing for 16s, re-arming the
// pathfound return") and every server routed walk aborted at once on
// "the server routed walk would swim".
//
// The root cause (three parts, pinned by the tests below):
//
//  1. The bot had no channel for the server's refusal answer. The
//     server answers every refused MoveToLocation with ActionFailed
//     (0x35); the bot parsed the packet and discarded it, so every
//     stuck verdict read as a "frozen corridor" and the ladder banned
//     innocent ground. The live reproduction on the reference stack
//     (the full geodata pack, PathFinding = 2) walks the identical
//     dump scenario cleanly - the user's server build (its unknown
//     packet fingerprints 0x57/53 bytes and 0xe7/21 bytes do not
//     exist on the reference master) refuses the village exit clicks
//     its own way, and other fleet bots in open ground keep moving.
//
//  2. The server routed fallback clicked the zone center ~10200
//     units away: beyond the 9900 request cap of MoveToLocation
//     (refused outright) and over the 3000 unit boundary where
//     Mobius walks a player click as a straight line (the water
//     guard correctly refused the wet line - the "anti water law"
//     was right in spirit but it guarded an impossible geometry).
//
//  3. The zone leg grind stall re-armed the pathfound return on
//     refusal evidence too: the cycle the server kept refusing ran
//     another seven minutes in the dump.
//
// The fix: the tracker records the ActionFailed arrivals
// (ApplyActionFailed), the hunt walk machinery reads them as the
// online refusal evidence (refusalEvidence), a refused stuck varies
// the aim before anything else (sendVariedAim), a leg with refusal
// evidence never bans a corridor (escalateFrozenLeg) and its routed
// walk aborts early with the honest reason, the routed walk hops
// under the straight line boundary (directHopMax) with the water
// guard on the hop line, and the zone leg stall holds the return
// backoff instead of re-arming.
const (
    // refusalDumpX/Y/Z is the reported stuck position (the dump of
    // 2026-09-14 10:18, the village center stand of test3).
    refusalDumpX = int32(45768)
    refusalDumpY = int32(49848)
    refusalDumpZ = int32(-3056)
    // refusalDumpZoneX/Y is the held cell center of the dump (the
    // Kaboo Orc Fighter SW-7 rotation of the report, leash half 633).
    refusalDumpZoneX = int32(34500)
    refusalDumpZoneY = int32(51095)
)

// refusingClickServer simulates the server side of the dump: every
// walk request validates through the ported offline rules (the bot's
// own geodata passes them - that is the dump's premise) but the
// server refuses the click its own way and answers ActionFailed -
// the character never moves a cell. The maxAccept switch models the
// target specific refusal: a click longer than the bound is refused
// (the old server build whose pathfinder gives up on the far
// targets), a shorter one walks its validated destination at once.
type refusingClickServer struct {
    engine *pathfind.Engine
    // refuseAll answers ActionFailed for every click (the dump's
    // server: nothing it is asked to walk moves the character).
    refuseAll bool
    // maxAccept answers ActionFailed for clicks longer than the
    // bound (0: no length limit) - the target specific refusal.
    maxAccept float64
    requests  int
    refused   int
    accepted  int
}

// consumeAt takes the newest walk request of the fake game, answers
// it the way the reported server did at the tick's time and applies
// the destination the server would walk to. A refused click records
// the ActionFailed arrival one tick-millisecond after the request -
// the correlation window of refusalEvidence sees the answer of the
// click the loop just sent.
func (s *refusingClickServer) consumeAt(
    game *fakeGame, bot *state.Bot, now time.Time,
) {
    if len(game.walks) <= s.requests {
        return
    }
    s.requests = len(game.walks)
    target := game.walks[len(game.walks)-1]
    selfX, selfY, selfZ, ok := bot.SelfPosition()
    if !ok {
        return
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    to := pathfind.Vec3{
        X: float64(target[0]), Y: float64(target[1]),
        Z: float64(target[2]),
    }
    validated, ok := s.engine.ValidateClick(from, to)
    if !ok || s.refuseAll || (s.maxAccept > 0 &&
        math.Hypot(to.X-from.X, to.Y-from.Y) > s.maxAccept) {
        s.refused++
        bot.ApplyActionFailed(now.Add(time.Millisecond))

        return
    }
    s.accepted++
    bot.ApplyMovement(state.Movement{
        ObjectID: 100,
        X:        int32(validated.X), Y: int32(validated.Y),
        Z:     int32(validated.Z),
        DestX: int32(validated.X), DestY: int32(validated.Y),
        DestZ: int32(validated.Z),
    })
}

// refusalDumpLoop builds the loop of the dump scenario on the real
// geodata pack: the village center stand, the far western hunting
// zone, the event-mirroring logger.
func refusalDumpLoop(
    t *testing.T, refuseAll bool, maxAccept float64,
) (*Loop, *fakeGame, *state.Bot, *refusingClickServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, refusalDumpX, refusalDumpY, refusalDumpZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(refusalDumpZoneX, refusalDumpZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &refusingClickServer{
        engine: engine, refuseAll: refuseAll, maxAccept: maxAccept,
    }

    return loop, game, bot, sim, sink
}

// TestReproRefusedClickSkipsTheCorridorBan pins the inversion of the
// dump's session poisoning: a server that refuses every click (the
// ActionFailed answer arrives for each one, the character never
// moves) must not ban a single corridor - the corridors are innocent,
// the refusal evidence names the server - the trips abort with the
// honest reasons and the zone leg grind holds the return backoff
// instead of re-arming the cycle the server keeps refusing.
func TestReproRefusedClickSkipsTheCorridorBan(t *testing.T) {
    loop, game, bot, sim, sink := refusalDumpLoop(t, true, 0)

    for range 4 {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            // The post-abort grind of the dump: the direct zone legs
            // against the refusing server - the stall must hold the
            // return backoff, not re-arm it.
            zone := loop.zone()
            require.NotNil(t, zone)
            grind := time.Now()
            // Two stall windows: the first refusal stall re-arms
            // (the progress probe has nothing yet), the second on
            // the same unmoved cell holds the backoff.
            for range 40 {
                grind = grind.Add(time.Second)
                x, y, z, ok := bot.SelfPosition()
                require.True(t, ok)
                loop.walkZoneLeg(zone, x, y, z, grind)
                sim.consumeAt(game, bot, grind)
            }

            continue
        }
        now := time.Now()
        for i := 0; i < 400 && loop.phase == phaseTownReturn; i++ {
            now = now.Add(2 * time.Second)
            x, y, z, ok := bot.SelfPosition()
            require.True(t, ok, "the character position must be known")
            if loop.directLeg {
                // armDirectLeg stamps its window with the real
                // clock while this driver walks on the synthetic
                // one: re-arm the window so the leg lives inside
                // the driven timeline.
                loop.directLegUntil = now.Add(directLegWindow)
                loop.walkDirectLeg(now, x, y, z)
                sim.consumeAt(game, bot, now)

                continue
            }
            if loop.followWaypoints(x, y, z, now, true) {
                break
            }
            sim.consumeAt(game, bot, now)
        }
    }

    require.Empty(t, loop.frozenAreas,
        "a server that refuses the clicks does not name frozen "+
            "corridors - the session must not poison the planner")
    require.Positive(t, sim.refused,
        "the refusal model is live: the clicks of the trips bounced")
    require.Zero(t, sim.accepted,
        "the refuse-all premise: nothing ever moved")
    x, y, _, _ := bot.SelfPosition()
    require.InDelta(t, float64(refusalDumpX), float64(x), 1,
        "the character never moved a cell")
    require.InDelta(t, float64(refusalDumpY), float64(y), 1)
    require.Contains(t, sink.String(),
        "the server refused the walk click",
        "the refusal evidence is visible to the operator")
    require.Contains(t, sink.String(), "varying the aim",
        "the varied aim ladder runs before the re-path machinery")
    require.Contains(t, sink.String(), "skipping the corridor ban",
        "the escalation gate reads the refusal latch")
    require.Contains(t, sink.String(),
        "the server refused the routed walk clicks",
        "the direct leg aborts early with the honest reason")
    require.Contains(t, sink.String(), "holding the return backoff",
        "the zone leg grind stops re-arming the refused cycle")
    require.Equal(t, zoneReturnFailBudget, loop.zoneFails,
        "the refused grind keeps the return backoff armed")
}

// TestReproRefusedClickVariedAimWalksOut pins the target specific
// refusal recovery: a server that refuses the long clicks (its
// pathfinder gives up past the 600 unit bound) still accepts the
// varied aims - the half leg prefix of the same waypoint - and the
// walk inches out of the village and into the zone without a single
// corridor ban, re-path or abort.
func TestReproRefusedClickVariedAimWalksOut(t *testing.T) {
    loop, game, bot, sim, sink := refusalDumpLoop(t, false, 600)

    arrived := false
    for range 10 {
        if arrived {
            break
        }
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            continue
        }
        now := time.Now()
        for range 600 {
            if loop.phase != phaseTownReturn {
                break
            }

            now = now.Add(2 * time.Second)
            x, y, z, ok := bot.SelfPosition()
            require.True(t, ok, "the character position must be known")
            if loop.directLeg {
                loop.walkDirectLeg(now, x, y, z)
                sim.consumeAt(game, bot, now)

                continue
            }
            if loop.followWaypoints(x, y, z, now, true) {
                loop.endTownTrip("back at the farm spot")

                break
            }
            sim.consumeAt(game, bot, now)
        }
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, refusalDumpZoneX, refusalDumpZoneY, 633) &&
            !loop.tripActive() {
            arrived = true
        }
    }

    require.True(t, arrived,
        "the varied aim must walk the character out of the refused "+
            "village approach and into the zone")
    require.Empty(t, loop.frozenAreas,
        "no corridor ban armed: the refusal was target specific, not "+
            "a frozen corridor")
    require.Positive(t, sim.refused,
        "the plain long clicks are refused by the model")
    require.Positive(t, sim.accepted,
        "the varied aims are accepted by the model")
    require.Contains(t, sink.String(), "varying the aim",
        "the varied aim line names the recovery")
    require.NotContains(t, sink.String(), "banning the frozen corridor",
        "the corridor ban never runs on refusal evidence")
    require.NotContains(t, sink.String(),
        "the server refused the routed walk clicks",
        "the routed fallback never runs dry: the varied aims keep "+
            "the walk moving")
}

// TestReproDirectLegHopsUnderTheCap pins the hop geometry of the
// server routed walk: the old fallback clicked the far leg target
// once (a 10200 unit request the server refuses beyond its 9900
// cap, or walks as a wet straight line past the 3000 boundary -
// the dump aborted on it instantly), the hop fallback sends clicks
// capped at directHopMax, every hop passes the offline click port
// before it leaves (no blind refused clicks) and the walk carries
// the character toward its target - progress first, the honest
// abort last.
func TestReproDirectLegHopsUnderTheCap(t *testing.T) {
    loop, game, bot, sim, sink := refusalDumpLoop(t, false, 0)
    loop.legDest = pathfind.Vec3{
        X: float64(refusalDumpZoneX), Y: float64(refusalDumpZoneY),
        Z: -3560,
    }
    loop.armDirectLeg("test hop walk")

    startX, startY, _, _ := bot.SelfPosition()
    startDist := math.Hypot(
        float64(refusalDumpZoneX-startX),
        float64(refusalDumpZoneY-startY))
    now := time.Now()
    for i := 0; i < 30 && loop.directLeg; i++ {
        now = now.Add(2 * time.Second)
        x, y, z, ok := bot.SelfPosition()
        require.True(t, ok)
        loop.walkDirectLeg(now, x, y, z)
        if len(game.walks) > sim.requests {
            target := game.walks[len(game.walks)-1]
            hop := math.Hypot(
                float64(target[0]-x), float64(target[1]-y))
            require.LessOrEqual(t, hop, directHopMax+1,
                "every routed hop stays under the straight line "+
                    "boundary the server routing owns")
        }
        sim.consumeAt(game, bot, now)
    }

    require.False(t, loop.directLeg,
        "the hop walk must finish within the window")
    require.Zero(t, sim.refused,
        "every sent hop passed the offline click port - the routed "+
            "walk never sends a click the reference server collapses")
    require.Positive(t, sim.accepted,
        "the hops moved the character")
    x, y, _, _ := bot.SelfPosition()
    endDist := math.Hypot(
        float64(refusalDumpZoneX-x), float64(refusalDumpZoneY-y))
    require.Less(t, endDist, startDist,
        "the hop walk carried the character toward its target "+
            "instead of aborting at the start cell the way the dump "+
            "did")

    // A dry nearby target walks the whole hop chain to arrival.
    game2 := &fakeGame{}
    bot2 := newTestBot()
    moveSelfTo(bot2, refusalDumpX, refusalDumpY, refusalDumpZ)
    loop2 := NewLoop(game2, bot2)
    engine := reproEngine(t)
    loop2.SetNavigator(NewNavigator(engine))
    loop2.SetHuntingZone(refusalDumpZoneX, refusalDumpZoneY, 633)
    sink2 := &bytes.Buffer{}
    loop2.SetLogger(log.New(
        io.MultiWriter(sink2, eventMirror{bot: bot2}), "", 0))
    sim2 := &refusingClickServer{engine: engine}
    loop2.legDest = pathfind.Vec3{X: 43512, Y: 50504, Z: -2992}
    loop2.armDirectLeg("test dry hop walk")
    now = time.Now()
    for range 10 {
        if !loop2.directLeg {
            break
        }

        now = now.Add(2 * time.Second)
        x2, y2, z2, ok := bot2.SelfPosition()
        require.True(t, ok)
        loop2.walkDirectLeg(now, x2, y2, z2)
        sim2.consumeAt(game2, bot2, now)
    }
    require.False(t, loop2.directLeg, "the dry hop walk finishes")
    require.Contains(t, sink2.String(), "the server routed walk reached",
        "the dry direction arrives at the hop target")
    require.Zero(t, sim2.refused,
        "the dry hops pass the offline port as well")
    _ = sink
}

// TestShortenWetHopHalvesToTheDryPrefix pins the water guard of the
// routed hops: a hop whose line crosses the lake shortens toward its
// dry prefix (the walkable shore prefix carries the character to the
// waterline) and a hop that stays wet down to the floor reports no
// dry prefix at all - the direction is water blocked and the trip
// ends with the honest abort.
func TestShortenWetHopHalvesToTheDryPrefix(t *testing.T) {
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, refusalDumpX, refusalDumpY, refusalDumpZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)

    from := pathfind.Vec3{
        X: float64(refusalDumpX), Y: float64(refusalDumpY),
        Z: float64(refusalDumpZ),
    }
    zone := pathfind.Vec3{
        X: float64(refusalDumpZoneX), Y: float64(refusalDumpZoneY),
        Z: -3560,
    }
    // Walk the dump line west until the first wet sample: the stand
    // one step before it is the shore cell of the test.
    dx := zone.X - from.X
    dy := zone.Y - from.Y
    wetAt := 1.0
    for frac := 0.05; frac < 1.0; frac += 0.05 {
        probe := pathfind.Vec3{
            X: from.X + dx*frac, Y: from.Y + dy*frac, Z: from.Z,
        }
        crossed, err := nav.WaterCrossed(from, probe)
        if err == nil && crossed {
            wetAt = frac

            break
        }
    }
    require.Negative(t, wetAt-1.0+0.05,
        "the dump line crosses water: the wet sample must exist")
    shore := pathfind.Vec3{
        X: from.X + dx*(wetAt-0.05), Y: from.Y + dy*(wetAt-0.05),
        Z: from.Z,
    }

    // The full hop from the shore crosses the water.
    hop := pathfind.Vec3{
        X: shore.X + dx*0.3, Y: shore.Y + dy*0.3, Z: shore.Z,
    }
    crossed, err := nav.WaterCrossed(shore, hop)
    require.NoError(t, err)
    require.True(t, crossed,
        "the hop from the shore toward the zone crosses the lake")

    hopX, hopY, hopZ, dry := loop.shortenWetHop(
        int32(shore.X), int32(shore.Y), int32(shore.Z),
        int32(hop.X), int32(hop.Y), int32(hop.Z))
    require.True(t, dry,
        "a hop over the lake has a dry shore prefix")
    short := pathfind.Vec3{
        X: float64(hopX), Y: float64(hopY), Z: float64(hopZ),
    }
    crossed, err = nav.WaterCrossed(shore, short)
    require.NoError(t, err)
    require.False(t, crossed,
        "the shortened hop line stays dry")
    require.Less(t, math.Hypot(short.X-shore.X, short.Y-shore.Y),
        math.Hypot(hop.X-shore.X, hop.Y-shore.Y),
        "the shortened hop is a prefix of the refused one")
}

// TestReproZoneLegStallHoldsBackoffOnRefusal pins the grind gate: the
// direct zone legs against a refusing server (ActionFailed per
// click, zero movement) stall into the honest line that HOLDS the
// return backoff - the fail budget stays armed, the cycle the server
// keeps refusing does not restart.
func TestReproZoneLegStallHoldsBackoffOnRefusal(t *testing.T) {
    loop, game, bot, sim, sink := refusalDumpLoop(t, true, 0)
    // The post-abort state of the dump: the budget is armed and the
    // engage phase answers direct legs only.
    loop.zoneFails = zoneReturnFailBudget
    loop.zoneReturn = true
    loop.phase = phaseEngage
    zone := loop.zone()
    require.NotNil(t, zone)

    now := time.Now()
    // Two stall windows on the same unmoved cell: the first
    // refusal stall re-arms the pathfound return (the progress
    // probe is empty but unproven), the second holds the backoff.
    for range 40 {
        now = now.Add(time.Second)
        x, y, z, ok := bot.SelfPosition()
        require.True(t, ok)
        loop.walkZoneLeg(zone, x, y, z, now)
        sim.consumeAt(game, bot, now)
    }
    require.Equal(t, zoneReturnFailBudget, loop.zoneFails,
        "the refusal stall on the unmoved cell holds the return "+
            "backoff armed")
    require.Contains(t, sink.String(), "re-arming",
        "the first refusal stall re-arms the pathfound return")
    require.Contains(t, sink.String(), "holding the return backoff",
        "the second refusal stall on the same cell holds the "+
            "backoff: the cycle the server keeps refusing does not "+
            "restart")
}

// TestRefusalEvidenceWindow pins the correlation window itself: the
// answer counts only inside refusalAnswerWindow after the click -
// an old refusal is not the answer to the click just sent, and the
// tracker round trip carries the arrival time.
func TestRefusalEvidenceWindow(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    base := time.Now()
    loop.moveAt = base

    bot.ApplyActionFailed(base.Add(time.Second))
    require.True(t, loop.refusalEvidence(),
        "the fresh answer of the click counts")

    bot.ApplyActionFailed(base.Add(refusalAnswerWindow + time.Second))
    require.False(t, loop.refusalEvidence(),
        "a stale refusal is not the answer to the click just sent")

    bot.ApplyActionFailed(base.Add(-time.Second))
    require.False(t, loop.refusalEvidence(),
        "an answer that predates the click does not count")

    loop.moveAt = time.Time{}
    require.False(t, loop.refusalEvidence(),
        "without a sent click there is nothing to correlate")

    failedAt := base.Add(2 * time.Second)
    bot.ApplyActionFailed(failedAt)
    require.Equal(t, failedAt, bot.LastActionFailed(),
        "the tracker carries the arrival time of the last refusal")
}
