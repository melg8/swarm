// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The corner turn reproduction (the owner report of the delevel
// acceptance round: the bot stuck on the way to the eastern guard,
// the systematic ask: the sticking on corners and turns). The
// mechanism, probed against the real elven pack (cmd/cornerprobe):
//
//  1. The destination correction of a leg click stops the character
//     8..50 units short of the turn pivot - the click transport
//     cannot reach the funnel pivot over the corner approach cells
//     (the real pack: the dryad -> Starden guard walk, the leg click
//     toward the turn at 40968 53392 delivers only to 40968 53404).
//  2. The pass radius (50) counts the turn reached - correct - and
//     the follower clicks the NEXT waypoint. The chord from the
//     stopped position cuts the corner: the server collapses it (the
//     anti corner cut of the corner approach cells), the shorten
//     ladder halves into the same corner, and the back hop's pivot
//     click refuses with it (the probe: hop:N from the 8 unit band).
//  3. The old ladder fell to the re-path: the deterministic planner
//     reproduced the identical route, the second identical re-path
//     aborted the trip - and the frozen corridor ban sealed the
//     corner ground for the rest of the session. The stick.
//
// The fix: clickForwardJump - the first later plan waypoint whose
// straight line the server transport validates from the stuck cell.
// The cursor jumps onto it and the click walks the character around
// the corner before the stuck window ever opens. The tests below
// plan the live guard route and scan it for its own refusal band -
// the exact funnel floats name cells the rounded print values miss,
// so nothing is hardcoded.

// cornerBandPosition walks the incoming leg back from the turn by
// the given distance with a lateral offset (the arrival band
// positions the destination correction produces: the character stops
// on or beside the leg).
func cornerBandPosition(wps []pathfind.Vec3, turn int, back float64,
    side float64,
) pathfind.Vec3 {
    prev, wp := wps[turn-1], wps[turn]
    dx, dy := wp.X-prev.X, wp.Y-prev.Y
    leg := math.Hypot(dx, dy)
    frac := back / leg
    ux, uy := dx/leg, dy/leg

    return pathfind.Vec3{
        X: wp.X - ux*back - uy*side,
        Y: wp.Y - uy*back + ux*side,
        Z: wp.Z + (prev.Z-wp.Z)*frac,
    }
}

// correctionServer simulates the Mobius click transport honestly: a
// walk request walks the character to the CORRECTED destination of
// the server validation (getValidLocation's partial answer) - the
// mechanism that stops the character short of the corner pivot in
// the first place, and the mechanism that delivers the recovery
// jump's chord around it.
type correctionServer struct {
    engine *pathfind.Engine
    walks  int
}

// consume applies the newest walk request of the fake game the way
// the server resolves it: the character ends the walk at the
// validated destination, which may sit short of the clicked target.
func (s *correctionServer) consume(game *fakeGame, bot *state.Bot) {
    if len(game.walks) <= s.walks {
        return
    }
    s.walks = len(game.walks)
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
    if !ok {
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

// cornerTurnScene plans the real guard walk of the delevel scenario
// (the dryad ground to Starden over the mesh) and scans its turns
// for the refusal band: an arrival position of a turn whose outgoing
// chord the click transport refuses while some farther plan
// waypoint's chord validates - the corner the recovery jump serves.
// The scene skips the test when the pack or such a band is absent.
func cornerTurnScene(t *testing.T) (
    nav navmeshNavigator, engine *pathfind.Engine, wps []pathfind.Vec3,
    bandTurn, bandNext int, bandPos pathfind.Vec3,
) {
    t.Helper()
    nav, engine = realNavmeshNavigator(t)
    // The guard walks of the delevel scenario: the dryad ground to
    // either archer post, and the second walk of a death (the
    // village restart to either post). The audited corner bands sit
    // on the eastern guard route (cmd/cornerprobe).
    starts := []pathfind.Vec3{
        {X: 43500, Y: 54560, Z: -3664},
        {X: 44962, Y: 51105, Z: -3024},
    }
    goals := []pathfind.Vec3{
        {X: 42971, Y: 51372, Z: -2992},
        {X: 47595, Y: 51569, Z: -2992},
    }
    for _, start := range starts {
        for _, goal := range goals {
            result, err := nav.FindPathApproach(start, goal, 10)
            if err != nil || result == nil || !result.Found {
                continue
            }
            if turn, next, pos, ok := cornerRefusalBand(
                engine, result.Waypoints); ok {
                return nav, engine, result.Waypoints, turn, next, pos
            }
        }
    }
    t.Skip("no corner refusal band on these plans")
    t.Helper()

    return nav, engine, nil, 0, 0, pathfind.Vec3{}
}

// cornerRefusalBand scans the plan turns for the refusal band: an
// arrival position of a turn whose outgoing chord the click
// transport refuses while some farther plan waypoint's chord
// validates - the corner the recovery jump serves. The scan
// validates from the INT position the tracker holds: the server
// broadcasts integer coordinates, and the click answers flip on the
// sub-unit raster edges (the jump probe lesson).
func cornerRefusalBand(engine *pathfind.Engine,
    wps []pathfind.Vec3,
) (bandTurn, bandNext int, bandPos pathfind.Vec3, found bool) {
    for i := 1; i+1 < len(wps); i++ {
        for _, back := range []float64{8, 16, 24, 32, 40, 48} {
            for _, side := range []float64{0, -8, 8} {
                raw := cornerBandPosition(wps, i, back, side)
                pos := pathfind.Vec3{
                    X: float64(int32(math.Round(raw.X))),
                    Y: float64(int32(math.Round(raw.Y))),
                    Z: float64(int32(math.Round(raw.Z))),
                }
                if _, ok := engine.ValidateClick(pos, wps[i+1]); ok {
                    continue
                }
                for next := i + 2; next < len(wps); next++ {
                    if _, ok := engine.ValidateClick(pos, wps[next]); ok {
                        return i, next, pos, true
                    }
                }
            }
        }
    }

    return 0, 0, pathfind.Vec3{}, false
}

// TestCornerTurnBandJumpsTheCursorForward pins the click time
// recovery: the character standing in the corner turn band (the turn
// chord refused offline) gets its cursor jumped onto the first later
// waypoint whose line validates and the recovery click goes out -
// without a re-path, without a stuck window and without the frozen
// corridor ban that used to seal the corner for the session.
func TestCornerTurnBandJumpsTheCursorForward(t *testing.T) {
    nav, _, wps, bandTurn, bandNext, bandPos := cornerTurnScene(t)

    bot := newTestBot()
    moveSelfTo(bot, int32(bandPos.X), int32(bandPos.Y), int32(bandPos.Z))
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.waypoints = wps
    loop.wpIndex = bandTurn + 1
    loop.segmentDest = wps[len(wps)-1]

    now := time.Now()
    loop.followWaypoints(int32(bandPos.X), int32(bandPos.Y),
        int32(bandPos.Z), now)

    t.Logf("bandTurn=%d bandNext=%d cursor=%d walks=%v rePaths=%d",
        bandTurn, bandNext, loop.wpIndex, game.walks, loop.rePaths)
    for j := bandTurn - 1; j <= bandNext+1 && j < len(wps); j++ {
        t.Logf("wps[%d] = (%.2f %.2f %.2f)", j, wps[j].X, wps[j].Y,
            wps[j].Z)
    }
    require.Equal(t, bandNext, loop.wpIndex,
        "the cursor jumped onto the validating farther waypoint")
    require.NotEmpty(t, game.walks, "the recovery click went out")
    require.Zero(t, loop.rePaths, "the re-path ladder never burned")
}

// TestCornerWalkRoundsTheTurnWithoutRepath walks the plan fragment
// over the real corner against the honest click transport: the
// correction stops the character short of the pivot, the pass radius
// hands the walk to the next leg, the turn band jump carries the
// character around the corner - the walk crosses the audited corner
// with the re-path budget untouched and no corridor bans.
func TestCornerWalkRoundsTheTurnWithoutRepath(t *testing.T) {
    nav, engine, wps, bandTurn, _, _ := cornerTurnScene(t)
    bot := newTestBot()
    start := wps[bandTurn-1]
    moveSelfTo(bot, int32(start.X), int32(start.Y), int32(start.Z))
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.waypoints = wps
    loop.wpIndex = bandTurn - 1
    loop.segmentDest = wps[len(wps)-1]
    sim := &correctionServer{engine: engine}

    now := time.Now()
    passed := false
    for i := 0; i < 80 && !passed; i++ {
        now = now.Add(2 * time.Second)
        loop.moveAt = time.Time{}
        x, y, z, ok := bot.SelfPosition()
        require.True(t, ok, "the character position must be known")
        if loop.wpIndex > bandTurn {
            passed = true

            break
        }
        loop.followWaypoints(x, y, z, now)
        sim.consume(game, bot)
    }

    require.True(t, passed,
        "the walk crossed the audited corner (the cursor passed it)")
    require.Zero(t, loop.rePaths, "the corner recovery burned no re-path")
}

// TestCornerTurnJumpScanSendsNothingWhenNothingValidates pins the
// honesty guard of the jump: when every later waypoint's line
// refuses from the stuck cell (a pocket the plan cannot jump out
// of), the jump sends nothing and keeps the cursor - no blind cursor
// jump ahead of a line the server would cancel. The re-path ladder
// keeps its ownership of such a segment.
func TestCornerTurnJumpScanSendsNothingWhenNothingValidates(t *testing.T) {
    engine := reproEngine(t)
    nav := NewNavigator(engine)
    bot := newTestBot()
    moveSelfTo(bot, 40968, 53400, -3313)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    // The plan fragment without any validating farther waypoint from
    // the band: the click to the remaining waypoint refuses and the
    // scan behind it is empty.
    loop.waypoints = []pathfind.Vec3{
        {X: 40968, Y: 53504, Z: -3328},
        {X: 40968, Y: 53392, Z: -3312},
        {X: 40832, Y: 53216, Z: -3296},
    }
    loop.wpIndex = 2

    moveX, moveY, moveZ := 0.0, 0.0, 0.0
    fired := loop.clickForwardJump(40968, 53400, -3313,
        &moveX, &moveY, &moveZ, time.Now())

    require.False(t, fired, "no jump without a validated line")
    require.Equal(t, 2, loop.wpIndex, "the cursor stayed put")
    require.Empty(t, game.walks,
        "no recovery click went out from the refusing cell")
}

// TestCornerTurnJumpCarriesTheValidatedTarget pins the pointer
// contract of the jump: the cursor lands on the validating waypoint
// and the move pointers carry its corrected destination (the caller
// sends exactly one walk for the ladder rung).
func TestCornerTurnJumpCarriesTheValidatedTarget(t *testing.T) {
    nav, _, wps, bandTurn, bandNext, bandPos := cornerTurnScene(t)

    bot := newTestBot()
    moveSelfTo(bot, int32(bandPos.X), int32(bandPos.Y), int32(bandPos.Z))
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.waypoints = wps
    loop.wpIndex = bandTurn + 1
    loop.segmentDest = wps[len(wps)-1]

    moveX, moveY, moveZ := 0.0, 0.0, 0.0
    fired := loop.clickForwardJump(int32(bandPos.X), int32(bandPos.Y),
        int32(bandPos.Z), &moveX, &moveY, &moveZ, time.Now())

    require.True(t, fired, "the scene's band has a validating jump")
    require.Equal(t, bandNext, loop.wpIndex,
        "the cursor jumped onto the validating farther waypoint")
    // The pointers carry the corrected destination of the jumped
    // waypoint: a real step from the band position.
    step := math.Hypot(moveX-bandPos.X, moveY-bandPos.Y)
    require.GreaterOrEqual(t, step, extendMarchStep,
        "the corrected target moves the character a real step")
    require.Empty(t, game.walks,
        "the jump itself sends nothing - the caller sends one walk")
}
