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

// The reproduction of the 2026-09-19 14:46 state dump report (build
// 0331dd1, bot temp10, phase townReturn): two routes were refused and
// the cursor key escape walked the character STRAIGHT toward the
// zone - the walk the user forbade ("wasd пошел на прямую к зоне").
//
// The dump's own numbers pin the chord march: the last walk plan held
// a SINGLE waypoint (the zone center 28500 54560 -3264, the direct
// leg the frozen leg escalation armed at 14:45:13 - "the detour route
// froze as well, walking to 28500 54560 by the server routing"), and
// the escape's aims of the two refused routes sit EXACTLY on the
// straight chord from the plan origin 45768 49848 to that zone
// center:
//
//    14:45:16 the escape toward 43267 50530 (18 claimed steps):
//             the chord point at t = 0.14489 - the stride 18 x 144
//             units from the origin (2593 of the 17899 unit chord);
//    14:45:38 the escape toward 42711 50682 (4 claimed steps):
//             the chord point at t = 0.17703 - the stride 22 x 144
//             units from the origin (3169 along the same chord).
//
// The route following ladder of the escape (cursorEscapeRouteSteps)
// marched the "planned route" it names - but the plan it was handed
// IS the chord: the direct leg's waypoint plan is the single
// destination spec (armDirectLeg), not a route, so the ladder over it
// interpolates the straight line to the far zone target and the
// claims drag the character through the village geometry the planner
// would route around. The second escape's chord died on the water
// guard four strides in (the village water the chord crosses) and the
// character stranded at 42850 50644 - moving no.
//
// The contract the user pinned for the fix: the WASD escape walks
// ALONG THE ROUTE - when the walk does not move toward the current
// point, the mode switches to the WASD claims over a REAL planned
// route, and at the moment the point is reached the walk returns to
// the normal routed clicks. A degenerate single waypoint plan is
// never a route: the escape re-plans the town route first (the same
// search the leg start runs, the session bans respected) and the
// claims follow the fresh plan; without a route at all the escape
// stays a pocket sized straight ladder toward the validated hop aim -
// never a march toward the far target.
const (
    // directLegDumpX/Y/Z is the walk plan origin of the dump (the
    // village stand the direct leg armed at).
    directLegDumpX = int32(45768)
    directLegDumpY = int32(49848)
    directLegDumpZ = int32(-3056)
    // directLegDumpZoneX/Y is the hunting zone center of the dump
    // (the Kaboo Orc Fighter Leader SW-10 cell, leash half 633).
    directLegDumpZoneX = int32(28500)
    directLegDumpZoneY = int32(54560)
)

// directLegDumpLoop builds the loop of the 14:46 dump scenario on the
// real geodata pack: the character stands on the village plan origin,
// the far hunting zone waits, and the server model carries the cursor
// key semantics with the refusal pocket of the report (the dump
// proves the refusals rode at least the first 3169 units of the walk:
// the resumed clicks of 14:45:38 died at once).
func directLegDumpLoop(
    t *testing.T, keyboard bool, refuseRadius float64,
) (*Loop, *fakeGame, *state.Bot, *cursorKeyServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, directLegDumpX, directLegDumpY, directLegDumpZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(
        directLegDumpZoneX, directLegDumpZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &cursorKeyServer{
        engine: engine, refuseRadius: refuseRadius, keyboard: keyboard,
    }

    return loop, game, bot, sim, sink
}

// driveTownWalk mirrors the production dispatch of walkTownWaypoints
// for one simulated tick: the armed escape drives first, the direct
// leg walks its server routed hops, the routed legs follow their
// waypoints. The consumeAt call answers the requests the tick sent
// the way the reported server does.
func driveTownWalk(
    t *testing.T, loop *Loop, sim *cursorKeyServer, game *fakeGame,
    bot *state.Bot, now time.Time,
) {
    t.Helper()
    x, y, z, ok := bot.SelfPosition()
    require.True(t, ok, "the character position must be known")
    if loop.cursorEscape.armed || loop.directLeg {
        loop.walkDirectLeg(now, x, y, z)
    } else if loop.followWaypoints(x, y, z, now, true) {
        loop.endTownTrip("back at the farm spot")
    }
    sim.consumeAt(game, bot, now)
}

// TestReproDirectLegEscapeWalksTheRouteNotTheChord pins the fix
// contract on the dump's exact moment: the direct leg owns the walk
// (the single far waypoint plan), the hop click is refused, the
// cursor key escape arms - and the escape must NOT march the chord of
// the degenerate plan. It re-plans the town route (the real planner
// route the leg start search produces), installs it as the leg plan
// and the claims walk along it; the walk returns to the normal routed
// mode the moment the claimed point is reached.
func TestReproDirectLegEscapeWalksTheRouteNotTheChord(t *testing.T) {
    loop, game, bot, sim, sink := directLegDumpLoop(t, true, 20000)

    // The pathfound return plans a real route (the dump's 10
    // waypoint plan).
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase,
        "the zone return plans the walk")
    require.Greater(t, len(loop.waypoints), 1,
        "the pathfound return plans a real route")

    // The dump state of 14:45:13: the detour froze as well, the
    // direct leg owns the walk - the plan is the single far
    // waypoint.
    loop.armDirectLeg("the detour route froze as well")
    require.True(t, loop.directLeg)
    require.Len(t, loop.waypoints, 1,
        "the direct leg plan is the single destination spec")
    dest := loop.legDest

    // Drive the refused hop cycle until the escape arms and the
    // claims march.
    armed := false
    now := time.Now()
    for range 30 {
        now = now.Add(2 * time.Second)
        driveTownWalk(t, loop, sim, game, bot, now)
        if loop.cursorEscape.armed && len(game.claims) >= 3 {
            armed = true

            break
        }
    }
    require.True(t, armed,
        "the refused hop hands the walk to the cursor key escape")
    require.NotEmpty(t, game.cursorWalks,
        "the cursor key arm (the movement mode 0 request) was sent")
    require.NotEmpty(t, game.claims,
        "the claimed ValidatePosition stream was sent")

    // THE CONTRACT: the escape walks a real route, not the chord
    // of the degenerate plan.
    require.Greater(t, len(loop.waypoints), 1,
        "the escape must re-plan the degenerate direct leg plan "+
            "into a real route before the claims walk")
    require.False(t, loop.directLeg,
        "the walk returns to the routed mode on the fresh plan")
    last := loop.waypoints[len(loop.waypoints)-1]
    require.LessOrEqual(t,
        math.Hypot(last.X-dest.X, last.Y-dest.Y), tripApproachRadius,
        "the re-planned route still leads to the leg destination")

    // The claims follow the installed route polyline (the WASD
    // along the route contract) - the ladder runs from the escape
    // origin through the fresh plan's waypoints.
    self := pathfind.Vec3{
        X: float64(loop.cursorEscape.originX),
        Y: float64(loop.cursorEscape.originY),
    }
    for i, claim := range game.claims {
        dist := routeStepsCorridor(loop.waypoints, self,
            [3]int32{claim[0], claim[1], claim[2]})
        require.LessOrEqual(t, dist, waypointCorridor,
            "claim %d sits %.0f off the re-planned route - the "+
                "escape marched something else", i, dist)
    }

    // The escape line names the route following recovery, the
    // water guard holds for every claimed step.
    require.Contains(t, sink.String(), "walking along the planned route",
        "the escape line names the route following recovery")
    prev := pathfind.Vec3{
        X: float64(directLegDumpX), Y: float64(directLegDumpY),
        Z: float64(directLegDumpZ),
    }
    for _, claim := range game.claims {
        to := pathfind.Vec3{
            X: float64(claim[0]), Y: float64(claim[1]),
            Z: float64(claim[2]),
        }
        crossed, err := loop.navigator.WaterCrossed(prev, to)
        require.NoError(t, err)
        require.False(t, crossed,
            "a claimed step never crosses water: the claim %v", claim)
        prev = to
    }
}

// TestReproRefusedTownWalkReachesTheZoneOnTheRouteClaims pins the
// village escape acceptance over the whole walk: the direct leg state
// of the dump, the refusal pocket covering the town leg, the escape
// cycles walking the character along the routes - normal mode, WASD
// along the route at the refusal, normal mode again at the point -
// until the walk reaches the zone. Every armed escape of the walk
// holds a real route: no escape of the session ever marches a
// single waypoint plan.
func TestReproRefusedTownWalkReachesTheZoneOnTheRouteClaims(t *testing.T) {
    loop, game, bot, sim, sink := directLegDumpLoop(t, true, 4000)

    arrived := false
    for range 6 {
        if arrived {
            break
        }
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            continue
        }
        // The dump state: the detour froze as well, the direct
        // leg owns the walk.
        loop.armDirectLeg("the detour route froze as well")
        now := time.Now()
        for i := 0; i < 900 && loop.phase == phaseTownReturn; i++ {
            now = now.Add(2 * time.Second)
            // The live invariant of the contract: an armed
            // escape always walks a real route.
            if loop.cursorEscape.armed {
                require.Greater(t, len(loop.waypoints), 1,
                    "an armed escape must never walk the "+
                        "single waypoint chord plan")
            }
            driveTownWalk(t, loop, sim, game, bot, now)
        }
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, directLegDumpZoneX,
                directLegDumpZoneY, 633) && !loop.tripActive() {
            arrived = true
        }
    }

    require.True(t, arrived,
        "the escape cycles must walk the character to the zone "+
            "along the routes")
    require.Positive(t, sim.refusals,
        "the refusal pocket bounced the clicks first")
    require.NotEmpty(t, game.cursorWalks,
        "the cursor key arms were sent")
    require.NotEmpty(t, game.claims,
        "the claimed ValidatePosition stream walked the routes")
    require.Positive(t, sim.accepted,
        "the clicks resumed from the escaped ground - the normal "+
            "mode returned at the point")
    require.Contains(t, sink.String(), "walking along the planned route",
        "the escape lines name the route following recovery")
    require.Contains(t, sink.String(),
        "the cursor key escape walked to",
        "the settle lines name the escaped ground")
    require.NotContains(t, sink.String(),
        "the cursor key escape made no progress",
        "the server follows the claims of this model")
}

// TestCursorEscapeDirectLegWithoutRouteWalksTheHopAim pins the
// planless fallback of the degenerate leg: when the escape re-plan
// finds no route (the planner owns no path from the standing cell),
// the escape falls back to the straight ladder toward the VALIDATED
// HOP AIM - the pocket sized walk the planless contract holds - and
// never interpolates the straight line toward the far target past it.
func TestCursorEscapeDirectLegWithoutRouteWalksTheHopAim(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    nav := &fakeNavigator{fail: true}
    loop.SetNavigator(nav)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(io.MultiWriter(sink), "", 0))
    dest := pathfind.Vec3{X: 28500, Y: 54560, Z: -3264}
    loop.legDest = dest
    loop.waypoints = []pathfind.Vec3{dest}
    loop.wpIndex = 0
    loop.directLeg = true

    // The validated hop aim of the direct leg: 2000 units along the
    // chord toward the destination (under the directHopMax bound).
    selfX, selfY := directLegDumpX, directLegDumpY
    dx := dest.X - float64(selfX)
    dy := dest.Y - float64(selfY)
    aimX := int32(float64(selfX) + dx*2000/math.Hypot(dx, dy))
    aimY := int32(float64(selfY) + dy*2000/math.Hypot(dx, dy))

    require.True(t, loop.beginCursorKeyEscape(
        selfX, selfY, directLegDumpZ, aimX, aimY, directLegDumpZ),
        "the escape arms on the planless fallback")
    require.True(t, loop.directLeg,
        "the failed re-plan leaves the direct leg honest")
    require.Len(t, loop.waypoints, 1,
        "the failed re-plan leaves the single waypoint plan")

    steps := loop.cursorEscape.steps
    require.NotEmpty(t, steps, "the straight ladder armed")
    last := steps[len(steps)-1]
    require.Equal(t, aimX, last[0],
        "the ladder ends at the hop aim, not past it toward the "+
            "far target")
    require.Equal(t, aimY, last[1])
    for i, step := range steps {
        walked := math.Hypot(
            float64(step[0]-selfX), float64(step[1]-selfY))
        require.LessOrEqual(t, walked, 2000.0+cursorEscapeStep,
            "step %d marches past the hop aim toward the far "+
                "target (%.0f walked)", i, walked)
    }
    require.NotContains(t, sink.String(),
        "walking along the planned route",
        "the planless fallback never names a route walk")
}
