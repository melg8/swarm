// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "io"
    "log"
    "math"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The reproduction of the 2026-09-19 16:02 state dump report (build
// 07ccb0e, bot unittest1, phase townReturn): the character spawned at the
// village cell 43032 50408 -2992 and NEVER moved - "не исправил либо
// не залил? опять прямая линия до зоны - ее НЕ должно быть, ТОЛЬКО
// идти по маршрутам и НИКОГДА не идти напрямую".
//
// The dump's own numbers pin the mechanism:
//
//   - every walk click of the session died on the LOCAL click port
//     ("the server would refuse the walk click to 43032 50416" - the
//     8 unit neighbor of the spawn, re-pathing 1 of 3 at 16:00:07);
//     the clicks were never sent, so no server answer ever arrived
//     and the cursor key escape never armed;
//   - the freeze ladder climbed: the corridor ban at 43032 50416,
//     the detour re-plan (whose first funnel waypoint stays the same
//     8 unit step the ban cannot move), the second freeze - and the
//     rung two answered "the detour route froze as well, walking to
//     25500 59756 by the server routing": the walk plan collapsed
//     into the SINGLE FAR WAYPOINT (the dump's walk plan view: 1
//     waypoints, wp 0: 25500 59756 -3544 <-- TARGET);
//   - the direct segment's own hops were refused locally too - its
//     validation gate stands BEFORE the escape arming branches - so
//     the walk sat on the forbidden plan for the whole 45 s window,
//     the trip aborted and the cycle repeated three times (the
//     widening bans 48 -> 96 -> 192 in the event log).
//
// The offline probes against the real pack + the real mesh tiles
// confirm both halves: the mesh plans the 64 waypoint route from the
// spawn (its wp 0 IS the refused 43032 50416), while the grid click
// port refuses every direction from the spawn cell but south - the
// mesh/grid pocket the session froze in.
//
// The contract the owner pinned (verbatim): ТОЛЬКО идти по маршрутам
// и НИКОГДА не идти напрямую - the walk plan is ALWAYS a real route,
// the direct server routed walk does not exist, and when the walk
// does not move toward the current point the cursor key escape arms
// ALONG THE ROUTE (the claims follow the planner's bends) and at the
// moment the point is reached the normal routed clicks resume.
const (
    // spawnDumpX/Y/Z is the character position of the dump (the
    // village spawn cell the session never left).
    spawnDumpX = int32(43032)
    spawnDumpY = int32(50408)
    spawnDumpZ = int32(-2992)
    // zoneDumpX/Y is the hunting zone center of the dump (leash half
    // 633, the deck -3544 the zone return resolved).
    zoneDumpX = int32(25500)
    zoneDumpY = int32(59756)
)

// spawnDumpNavmeshDir finds the navmesh tile directory the way the
// bot mode detects it: data/navmesh above the package directory.
func spawnDumpNavmeshDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return ""
    }
    for range 6 {
        candidate := filepath.Join(dir, "data", "navmesh")
        if info, err := os.Stat(candidate); err == nil && info.IsDir() {
            return candidate
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            break
        }
        dir = parent
    }

    return ""
}

// spawnDumpMesh loads the real navmesh mesh of the live integration
// (the tiles cover the village and the hunting zone regions). The
// tiles are a local runtime artifact (cmd/navmesh-build, gitignored)
// the same way the geodata pack is: a tree without them skips the
// dump reproduction instead of failing it.
func spawnDumpMesh(t *testing.T) *navmesh.Mesh {
    t.Helper()
    dir := spawnDumpNavmeshDir()
    if dir == "" {
        t.Skip("no local navmesh tiles, the dump reproduction needs them")
    }
    mesh := navmesh.NewMesh(dir)
    require.Positive(t, mesh.Stats().TileFiles,
        "the mesh must hold tiles")

    return mesh
}

// spawnDumpLoop builds the loop of the 16:02 dump on the real pack
// and the real mesh tiles (the live hybrid navigator: the mesh plans,
// the grid validates the clicks). The server model is the honest one
// of the report: no server refusal answers exist in the session (the
// refused clicks never left the bot), so the sim refuses nothing and
// follows the claims - the whole freeze lived inside the bot.
func spawnDumpLoop(
    t *testing.T,
) (*Loop, *fakeGame, *state.Bot, *cursorKeyServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, spawnDumpX, spawnDumpY, spawnDumpZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(zoneDumpX, zoneDumpY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &cursorKeyServer{
        engine: engine, refuseRadius: 0, keyboard: true,
    }

    return loop, game, bot, sim, sink
}

// driveSpawnWalk mirrors the production dispatch of
// walkTownWaypoints for one simulated tick after the direct walk
// elimination: the armed escape drives the claims, the routed segments
// follow their waypoints, and a finished plan ends the trip the way
// tickTownTrip ends the return segment. The consumeAt call answers the
// requests the tick sent the way an honest server does.
func driveSpawnWalk(
    t *testing.T, loop *Loop, sim *cursorKeyServer, game *fakeGame,
    bot *state.Bot, now time.Time,
) {
    t.Helper()
    x, y, z, ok := bot.SelfPosition()
    require.True(t, ok, "the character position must be known")
    if loop.cursorEscape.armed {
        loop.driveCursorKeyEscape(now, x, y)
    } else if loop.followWaypoints(x, y, z, now) {
        loop.endTownTrip("back at the farm spot")
    }
    sim.consumeAt(game, bot, now)
}

// TestReproRefusedSpawnNeverWalksTheDirectLine pins the whole 16:02
// contract end to end: the spawn pocket refuses every click locally,
// the freeze ladder hands the walk to the cursor key escape ALONG THE
// PLANNED ROUTE, the claims walk the character off the poisoned cell
// following the planner's bends, the settle returns the walk to the
// normal routed clicks on the same plan - and the walk reaches the
// hunting zone. The walk plan never collapses into the single far
// waypoint: no line of the session ever names the server routed walk.
func TestReproRefusedSpawnNeverWalksTheDirectLine(t *testing.T) {
    loop, game, bot, sim, sink := spawnDumpLoop(t)

    // The pathfound return plans a real route over the mesh (the 64
    // waypoint answer of the offline probe).
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase,
        "the zone return plans the walk")
    require.Greater(t, len(loop.waypoints), 1,
        "the pathfound return plans a real multi waypoint route")
    require.NotNil(t, loop.segmentSearch,
        "the plan carries its mesh search contract")

    arrived := false
    escapes := make([]escapeSnapshot, 0, 4)
    now := time.Now()
    for i := 0; i < 400 && !arrived; i++ {
        now = now.Add(2 * time.Second)
        // The live invariant of the owner contract: the walk plan
        // is ALWAYS a real route - every plan of this walk carries
        // its mesh search contract, the search-less plan view was
        // the direct segment's single far waypoint.
        if loop.tripActive() && len(loop.waypoints) > 0 {
            require.NotNil(t, loop.segmentSearch,
                "the walk plan lost its search contract - the "+
                    "plan view fell back to the search-less direct "+
                    "line the owner forbade")
        }
        if loop.cursorEscape.armed &&
            (len(escapes) == 0 ||
                loop.cursorEscapes > escapes[len(escapes)-1].attempt) {
            escapes = append(escapes, escapeSnapshot{
                attempt: loop.cursorEscapes,
                origin: pathfind.Vec3{
                    X: float64(loop.cursorEscape.originX),
                    Y: float64(loop.cursorEscape.originY),
                },
                plan:   append([]pathfind.Vec3(nil), loop.waypoints...),
                steps:  append([][3]int32(nil), loop.cursorEscape.steps...),
                claims: len(game.claims),
            })
        }
        driveSpawnWalk(t, loop, sim, game, bot, now)
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, zoneDumpX, zoneDumpY, 633) &&
            !loop.tripActive() {
            arrived = true
        }
    }

    require.True(t, arrived,
        "the walk must reach the hunting zone along the routes")
    require.NotEmpty(t, escapes,
        "the frozen ladder must hand the walk to the cursor key "+
            "escape - the mode switch the owner contract demands")
    for i, snap := range escapes {
        from, to := escapeClaimRange(snap, escapes, i,
            len(game.claims))
        escapeSnapshotClaims(t, loop, game, snap, i, from, to)
    }
    require.NotEmpty(t, game.cursorWalks,
        "the cursor key arm (the movement mode 0 request) was sent")
    require.NotEmpty(t, game.claims,
        "the claimed ValidatePosition stream walked the route")
    require.Positive(t, sim.accepted,
        "the clicks resumed from the escaped ground - the normal "+
            "mode returned at the point")
    require.NotContains(t, sink.String(), "by the server routing",
        "the direct server routed walk must never arm")
    require.NotContains(t, sink.String(), "the server routed walk",
        "no direct walk line belongs in the session log")
    require.Contains(t, sink.String(), "walking along the planned route",
        "the escape line names the route following recovery")
    require.Contains(t, sink.String(),
        "the cursor key escape walked to",
        "the settle line names the escaped ground")
}

// escapeSnapshot captures one armed escape of the walk: the attempt
// number, the origin it armed at, the plan it walks, the claimed
// steps it built and the claim index its stream starts at.
type escapeSnapshot struct {
    attempt int
    origin  pathfind.Vec3
    plan    []pathfind.Vec3
    steps   [][3]int32
    claims  int
}

// escapeClaimRange returns the claim slice of one escape: from its
// own arm index to the next escape's arm (or the stream end).
func escapeClaimRange(
    snap escapeSnapshot, escapes []escapeSnapshot, i, total int,
) (int, int) {
    end := total
    if i+1 < len(escapes) {
        end = escapes[i+1].claims
    }

    return snap.claims, end
}

// escapeSnapshotClaims pins one armed escape against its own plan:
// the plan holds more than the single far waypoint, every claim sits
// on the route corridor, the step ladder stays dry and the settle
// returns the walk to the normal clicks (the WASD along the route
// contract).
func escapeSnapshotClaims(
    t *testing.T, loop *Loop, game *fakeGame,
    snap escapeSnapshot, index int, claimFrom, claimTo int,
) {
    t.Helper()
    require.Greater(t, len(snap.plan), 1,
        "escape %d must arm over a real route, never over a "+
            "single waypoint plan", index)
    for i, claim := range game.claims[claimFrom:claimTo] {
        dist := routeStepsCorridor(snap.plan, snap.origin,
            [3]int32{claim[0], claim[1], claim[2]})
        require.LessOrEqual(t, dist, waypointCorridor,
            "escape %d claim %d sits %.0f off the planned route - "+
                "the escape marched something else", index, i, dist)
    }
    // The water guard of the step ladder the escape built: every
    // claimed step's line stays dry.
    prev := snap.origin
    for i, step := range snap.steps {
        to := pathfind.Vec3{
            X: float64(step[0]), Y: float64(step[1]),
            Z: float64(step[2]),
        }
        crossed, err := loop.navigator.WaterCrossed(prev, to)
        require.NoError(t, err)
        require.False(t, crossed,
            "escape %d step %d never crosses water: the step %v",
            index, i, step)
        prev = to
    }
}

// TestReproRefusedSpawnLadderArmsTheEscapeWithoutServerAnswers pins
// the 16:02 refusal family itself: the refusals lived in the LOCAL
// click port (the clicks never left the bot), so no server answer
// ever arrived - and the frozen ladder must STILL switch the walk to
// the cursor key escape (the owner rule: the recovery switches modes
// as soon as the current one proves useless). The escape arms over a
// real route whose far end leads to the segment destination.
func TestReproRefusedSpawnLadderArmsTheEscapeWithoutServerAnswers(
    t *testing.T,
) {
    loop, game, bot, sim, sink := spawnDumpLoop(t)

    loop.lastHit = time.Now().Add(-time.Minute)
    loop.returnToZone()
    require.Equal(t, phaseTownReturn, loop.phase)

    armed := false
    var armPlan []pathfind.Vec3
    now := time.Now()
    for i := 0; i < 60 && !armed; i++ {
        now = now.Add(2 * time.Second)
        driveSpawnWalk(t, loop, sim, game, bot, now)
        if loop.cursorEscape.armed && len(game.claims) >= 3 {
            armed = true
            armPlan = append([]pathfind.Vec3(nil), loop.waypoints...)
        }
    }

    require.True(t, armed,
        "the frozen ladder arms the cursor key escape without any "+
            "server refusal answer")
    require.Zero(t, sim.refusals,
        "the server never refused anything - the refusals of this "+
            "dump family live in the local click port")
    require.NotEmpty(t, game.cursorWalks,
        "the cursor key arm was sent")
    require.NotEmpty(t, game.claims,
        "the claimed steps walked the route")
    require.Greater(t, len(armPlan), 1,
        "the escape arms over the real route")
    dest := loop.segmentDest
    last := armPlan[len(armPlan)-1]
    require.LessOrEqual(t,
        math.Hypot(last.X-dest.X, last.Y-dest.Y), tripApproachRadius,
        "the route the escape walks leads to the segment destination")
    require.Contains(t, sink.String(), "walking along the planned route",
        "the escape line names the route following recovery")
}

// TestCursorEscapePlanlessAimStaysPocketSized pins the planless
// fallback of the escape: without a plan (no route the planner could
// answer) the escape arms over the straight ladder toward the aim -
// and the aim is CLAMPED to the pocket radius, so a far target can
// never pull a straight march out of the planless escape (the owner
// rule: НИКОГДА не идти напрямую).
func TestCursorEscapePlanlessAimStaysPocketSized(t *testing.T) {
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.SetNavigator(&fakeNavigator{})
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(io.MultiWriter(sink), "", 0))
    dest := pathfind.Vec3{
        X: float64(zoneDumpX), Y: float64(zoneDumpY), Z: -3544,
    }

    // No plan at all (the planner owns no route from here), the aim
    // is the far zone - the old degenerate plan target.
    selfX, selfY := spawnDumpX, spawnDumpY
    require.Nil(t, loop.waypoints)

    require.True(t, loop.beginCursorKeyEscape(
        selfX, selfY, spawnDumpZ,
        int32(dest.X), int32(dest.Y), int32(dest.Z)),
        "the escape arms on the planless fallback")
    steps := loop.cursorEscape.steps
    require.NotEmpty(t, steps, "the straight ladder armed")
    last := steps[len(steps)-1]
    walked := math.Hypot(
        float64(last[0]-selfX), float64(last[1]-selfY))
    require.LessOrEqual(t, walked, cursorEscapeRouteMax+cursorEscapeStep,
        "the planless ladder ends inside the pocket (%.0f walked)",
        walked)
    for i, step := range steps {
        stepWalked := math.Hypot(
            float64(step[0]-selfX), float64(step[1]-selfY))
        require.LessOrEqual(t, stepWalked,
            cursorEscapeRouteMax+cursorEscapeStep,
            "step %d marches past the pocket toward the far "+
                "target (%.0f walked)", i, stepWalked)
    }
    require.NotContains(t, sink.String(),
        "walking along the planned route",
        "the planless fallback never names a route walk")
}
