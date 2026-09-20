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

// The reproduction of the 2026-09-14 08:42 state dump report (build
// 6e45624, bot unittest3, phase engage, uptime 1m33s): the level 15
// character stood at x 45768 y 49848 z -3056 (the elven village south
// terrace deck) with the held cell "Kaboo Orc Fighter SW-7" 10200
// units away at 36000 46765, the walk plan empty, the character
// outside the zone and NOTHING moving it. The event log carried three
// identical trip cycles - "town walk stuck, re-pathing (1 of 3)",
// "banning the frozen corridor at 43512 50504", "the detour route
// froze as well, walking to 36000 46765 by the server routing",
// "town trip ended: aborted, the server routed walk would swim" -
// and after the third abort ("3 aborted trips in a row, the next trip
// waits 10m0s") ten seconds of total silence: no Hunt line, no walk
// plan, a frozen character.
//
// The root cause (two halves, both pinned here):
//
//  1. The deterministic planner reproduced the same walled southwest
//     corridor each trip (the geodata pack models it open, the user's
//     server walls it) and the recovery of the era - the frozen
//     corridor ban with its widening - only ever pushed the modeled
//     wall outward. The 2026-09-20 plaza round removed that whole
//     system (its rectangle granularity sealed whole mesh sheets and
//     its trigger read the follower's own cursor pinning as a
//     freeze): the cursor key escape now owns the frozen walk - the
//     claims transport walks the character ALONG THE PLANNED ROUTE
//     through the walled ground (the arrow key movement of the
//     2026-09-14 15:10 report, the only movement a click refusing
//     server answers), the settle returns the walk to the normal
//     routed clicks and the trip budget bounds itself.
//
//  2. The budget-gated direct zone segments ground silently. After the
//     third abort zoneFails reached zoneReturnFailBudget and the
//     return fell back to walkZoneSegment: the 1000 unit hops toward the
//     zone center pass the offline click validation (the pack models
//     the southwest line open) but the server silently cancels every
//     one of them - and walkZoneSegment had no movement watcher, no abort
//     and no log line: the bot stood clicking the collapsed southwest
//     segment once a second forever (the regression of the 2026-09-12
//     01:50 report through the new abort path).
//
// The fix (pinned by the tests below):
//
//  1. The frozen trip escalation hands the walk to the cursor key
//     escape (see escalateFrozenSegment): the claims walk the plan
//     through the walled approach, no corridor ban ever seals a
//     search of the session and the escape budget bounds the
//     recovery.
//
//  2. walkZoneSegment runs the no-movement stall of noteZoneSegmentStall: a
//     position that holds past the stuck timeout while the direct
//     segments go out re-arms the pathfound zone return (the same
//     recovery the offline refusal of guardZoneSegmentClick arms), so the
//     grind becomes a bounded, loud window that hands the walk back
//     to the routed segments.
const (
    // reproWidenX/Y/Z is the reported stuck position (the dump of
    // 2026-09-14 08:42, the village south terrace deck of unittest3).
    reproWidenX = int32(45768)
    reproWidenY = int32(49848)
    reproWidenZ = int32(-3056)
    // reproWidenZoneX/Y is the held cell center of the dump (the
    // Kaboo Orc Fighter SW-7 hexagon, the leash half 633).
    reproWidenZoneX = int32(36000)
    reproWidenZoneY = int32(46765)
)

// reproWidenWalls models the server side disagreement of the dump:
// the walled patches the user's server refuses to walk although the
// bot's geodata pack models the ground open. The band covers the
// whole southwest approach from the terrace (the first segments of the
// r48/r96/r192 plans cross it, the 1000 unit grind hop lands inside
// it) while the shop deck route - the r384 re-plan - threads below it
// and the west side stairs clear it (verified against the real pack:
// the sweep of the probe run, 0 walled segments on the escape route).
func reproWidenWalls() []pathfind.AvoidArea {
    return []pathfind.AvoidArea{
        {Center: pathfind.Vec3{X: 44900, Y: 50050}, Radius: 550},
        {Center: pathfind.Vec3{X: 44100, Y: 50290}, Radius: 500},
        {Center: pathfind.Vec3{X: 43550, Y: 50460}, Radius: 420},
    }
}

// walledVillageServer simulates the server side of the walk against
// the walled patches: every walk request validates through the ported
// offline rules first (the bot's own geodata - the dump's premise is
// that this validation PASSES on the walled corridor), and the
// server's own geodata then refuses the line whose steps cross a
// walled patch - the move is silently canceled, the character never
// moves a cell. A clear line walks its validated destination at once
// (the walk time collapses into the arrival broadcast).
type walledVillageServer struct {
    engine   *pathfind.Engine
    walled   []pathfind.AvoidArea
    requests int
    refused  int
    // cursorWalks/claims count the mode 0 arms and the claimed
    // positions the sim already answered; cursorArmed mirrors the
    // session's cursor key flag - the claims sync their placements
    // straight into the world (the arrow key walk the frozen ladder
    // hands the movement to), the mouse clicks that validate clear
    // the flag.
    cursorWalks int
    claims      int
    cursorArmed bool
}

// consume takes the newest requests of the fake game: the mode 0 arm
// latches the cursor key flag, the claims sync their placements and
// the newest walk request validates the way the reported server did -
// the server would walk the validated destination or silently cancel
// the line whose steps cross a walled patch.
func (s *walledVillageServer) consume(game *fakeGame, bot *state.Bot) {
    if len(game.cursorWalks) > s.cursorWalks {
        s.cursorWalks = len(game.cursorWalks)
        s.cursorArmed = true
    }
    if len(game.claims) > s.claims {
        for _, claim := range game.claims[s.claims:] {
            if s.cursorArmed {
                moveSelfTo(bot, claim[0], claim[1], claim[2])
            }
            s.claims++
        }
    }
    if len(game.walks) <= s.requests {
        return
    }
    s.requests = len(game.walks)
    s.cursorArmed = false
    target := game.walks[len(game.walks)-1]
    selfX, selfY, selfZ, ok := bot.SelfPosition()
    if !ok {
        return
    }
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    to := pathfind.Vec3{
        X: float64(target[0]), Y: float64(target[1]), Z: float64(target[2]),
    }
    validated, ok := s.engine.ValidateClick(from, to)
    if !ok {
        s.refused++

        return
    }
    if s.lineWalled(from, validated) {
        s.refused++

        return
    }
    bot.ApplyMovement(state.Movement{
        ObjectID: 100,
        X:        int32(validated.X), Y: int32(validated.Y),
        Z:     int32(validated.Z),
        DestX: int32(validated.X), DestY: int32(validated.Y),
        DestZ: int32(validated.Z),
    })
}

// lineWalled steps the straight line of a click cell by cell and
// reports whether any step lands on a walled patch: the server move
// validation collapses such a click onto the walker and the move is
// canceled without a single cell of movement.
func (s *walledVillageServer) lineWalled(from, to pathfind.Vec3) bool {
    dist := math.Hypot(to.X-from.X, to.Y-from.Y)
    steps := int(dist/reproSimStep) + 1
    for i := 0; i <= steps; i++ {
        frac := float64(i) / float64(steps)
        x := from.X + (to.X-from.X)*frac
        y := from.Y + (to.Y-from.Y)*frac
        for _, area := range s.walled {
            if math.Hypot(area.Center.X-x, area.Center.Y-y) <= area.Radius {
                return true
            }
        }
    }

    return false
}

// TestReproWalledApproachEscapesThroughTheClaims replays the whole
// dump standoff against the real geodata pack: the character on the
// terrace cell, the held cell 10200 units west, the server walls the
// whole southwest approach the deterministic planner keeps routing
// through. The frozen trip escalation must hand the walk to the
// cursor key escape - the claims walk the character ALONG THE PLANNED
// ROUTE through the walled ground (the arrow key movement of the
// 2026-09-14 15:10 report), the settle returns the walk to the normal
// routed clicks past the wall and the walk arrives inside the zone -
// the inversion of the dump signature (three identical aborted trips,
// then the silent grind, a character that never moved a cell), and
// NO corridor ban ever seals a search of the session.
func TestReproWalledApproachEscapesThroughTheClaims(t *testing.T) {
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, reproWidenX, reproWidenY, reproWidenZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(reproWidenZoneX, reproWidenZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &walledVillageServer{engine: engine, walled: reproWidenWalls()}

    arrived := false
    for trip := 0; trip < 30 && !arrived; trip++ {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            // The budget hold of the eliminated direct zone segments: the
            // returnToZone call paces its own retry (the hold resets
            // the fail counter once per backoff window), the next call
            // plans a fresh route.
            require.Zero(t, loop.zoneFails,
                "the budget hold resets the fail counter for the retry")

            continue
        }
        now := time.Now()
        for i := 0; i < 400 && loop.phase == phaseTownReturn; i++ {
            now = now.Add(2 * time.Second)
            loop.moveAt = time.Time{}
            x, y, z, ok := bot.SelfPosition()
            require.True(t, ok, "the character position must be known")
            if loop.cursorEscape.armed {
                // The frozen segment escalation handed the walk to the
                // cursor key escape: the claims walk the plan while
                // they hold.
                loop.driveCursorKeyEscape(now, x, y)
                sim.consume(game, bot)

                continue
            }
            if loop.followWaypoints(x, y, z, now) {
                loop.endTownTrip("back at the farm spot")

                break
            }
            sim.consume(game, bot)
        }
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, reproWidenZoneX, reproWidenZoneY, 633) &&
            !loop.tripActive() {
            arrived = true
        }
    }

    require.True(t, arrived,
        "the cursor key escape must walk the return through the "+
            "walled approach and into the zone")
    require.Positive(t, sim.refused,
        "the wall is modeled: the corridor clicks of the frozen trips "+
            "are refused without movement")
    require.NotEmpty(t, game.cursorWalks,
        "the frozen trips handed the walk to the cursor key escape")
    require.NotEmpty(t, game.claims,
        "the claimed ValidatePosition stream walked the walled route")
    require.NotContains(t, sink.String(), "frozen corridor",
        "no corridor ban ever seals a search of the session - the "+
            "system is gone")
}

// TestReproZoneSegmentGrindStallReArmsThePathfoundReturn pins the grind
// stall of the dump's terminal state: the direct zone segments pass the
// offline validation, the server silently cancels them and the
// character holds its cell - the window fires past the stuck timeout,
// logs one honest line and clears the fail budget so the next
// returnToZone tick plans a fresh geodata route instead of grinding
// the collapsed segment forever.
func TestReproZoneSegmentGrindStallReArmsThePathfoundReturn(t *testing.T) {
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, reproWidenX, reproWidenY, reproWidenZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(reproWidenZoneX, reproWidenZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    // The post-abort state of the dump: the third trip ended, the
    // budget is armed and the engage phase answers direct segments only.
    loop.zoneFails = zoneReturnFailBudget
    loop.zoneReturn = true
    loop.phase = phaseEngage
    zone := loop.zone()
    require.NotNil(t, zone)

    now := time.Now()
    // The grind: one segment per second, the character never moves (the
    // sim of the escape test refuses the line; the stall window alone
    // is under test here, so the clicks simply go out).
    fired := false
    for i := 0; i < 40 && !fired; i++ {
        now = now.Add(time.Second)
        clicks := len(game.walks)
        loop.walkZoneSegment(zone, reproWidenX, reproWidenY, reproWidenZ, now)
        if len(game.walks) == clicks && i > 0 {
            // The held click of the firing tick: the stall answered.
            fired = true
        }
    }
    require.True(t, fired,
        "the no-movement stall must fire within the stuck timeout")
    require.Zero(t, loop.zoneFails,
        "the stall re-arms the pathfound zone return")
    require.True(t, loop.zoneSegmentAt.IsZero(),
        "the stall window clears after the fire")
    require.Contains(t, sink.String(),
        "the direct zone segments moved nothing",
        "the grind is no longer silent")

    // The re-armed return plans a geodata route on the next tick.
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase,
        "the pathfound zone return owns the walk again")
    require.NotEmpty(t, loop.waypoints,
        "the fresh geodata route is planned")
}

// TestReproZoneSegmentStallRebaselinesOnMovement pins the honest-flow
// guard: a character that MOVES between the direct segments (the open
// ground walk the fallback exists for) re-baselines the window - no
// stall fires, the segments keep going out and the escalation state
// stays armed.
func TestReproZoneSegmentStallRebaselinesOnMovement(t *testing.T) {
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, reproWidenX, reproWidenY, reproWidenZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(reproWidenZoneX, reproWidenZoneY, 633)
    loop.zoneFails = zoneReturnFailBudget
    loop.zoneReturn = true
    zone := loop.zone()
    require.NotNil(t, zone)

    now := time.Now()
    loop.walkZoneSegment(zone, reproWidenX, reproWidenY, reproWidenZ, now)
    require.Len(t, game.walks, 1, "the first segment goes out")

    // The character walks its segment: the window re-baselines on the new
    // cell, however long the later ticks take.
    moveSelfTo(bot, 45000, 49600, -3056)
    now = now.Add(time.Minute)
    loop.walkZoneSegment(zone, 45000, 49600, -3056, now)
    require.Len(t, game.walks, 2,
        "a moved character keeps its direct segments - no stall fired")
    require.Equal(t, zoneReturnFailBudget, loop.zoneFails,
        "the escalation state survives the moving grind")
    require.False(t, loop.zoneSegmentAt.IsZero(),
        "the window re-baselined on the new cell")
}

// TestReproZoneSegmentStallStandsDownOnPlannedSegment pins the reset: a
// planned geodata segment owns the movement, so the stall watcher stands
// down - its window would otherwise read a trip's frozen standstill
// as its own and fire early on the segments the budget gate resumes when
// the trip ends.
func TestReproZoneSegmentStallStandsDownOnPlannedSegment(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    moveSelfTo(bot, reproWidenX, reproWidenY, reproWidenZ)
    loop.zoneCX = reproWidenZoneX
    loop.zoneCY = reproWidenZoneY
    loop.zoneHalf = 633
    zone := loop.zone()
    require.NotNil(t, zone)

    now := time.Now()
    loop.walkZoneSegment(zone, reproWidenX, reproWidenY, reproWidenZ, now)
    require.Len(t, game.walks, 1)
    require.False(t, loop.zoneSegmentAt.IsZero(),
        "the direct segment armed the stall watcher")

    loop.waypoints = []pathfind.Vec3{
        {X: float64(reproWidenX), Y: float64(reproWidenY),
            Z: float64(reproWidenZ)},
        {X: 45464, Y: 49208, Z: -3064},
    }
    loop.wpIndex = 0
    require.True(t, loop.startWalkSegment(pathfind.Vec3{
        X: 36000, Y: 46765, Z: -3712,
    }))
    require.True(t, loop.zoneSegmentAt.IsZero(),
        "the planned segment stands the stall watcher down")
}
