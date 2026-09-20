// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The 2026-09-20 stuck point round: the owner found the fleet frozen
// on two elven village terrace spots - 43632 50560 -2960 and
// 41920 52128 -3000. The first stands on a cell whose honest surface
// is the -2992 ground under the terrace edge: the pure 3D nearest
// polygon of the old FindNearestPoly snapped one cell north onto the
// sealed terrace deck (the reported z sits 32 units over the ground)
// and stranded the route inside its link component, while the
// column-first binding of the guard stairs round lands on the
// connected ground and the return plans a normal found route. The
// second spot is genuinely sealed terrace (29 polygons, the 592x432
// box, every link direction walled while the diagonal squeezes the
// server movement channels allow carry the only way out) - the
// pocket escape rides the walk-what-you-can partial contract there.

import (
    "bytes"
    "io"
    "log"
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The reported stuck positions and the hunting zone of the session.
const (
    stuckTerraceAX = int32(43632)
    stuckTerraceAY = int32(50560)
    stuckTerraceAZ = int32(-2960)
    stuckTerraceBX = int32(41920)
    stuckTerraceBY = int32(52128)
    stuckTerraceBZ = int32(-3000)
    stuckZoneX     = int32(25500)
    stuckZoneY     = int32(51095)
)

// stuckTerraceLoop builds the loop of a stuck terrace session on the
// real pack and the real mesh tiles: the character stands on the
// reported terrace spot, the far hunting zone waits, and the server
// model is the honest one (no refusal answers - the whole freeze
// lived in the route graph, the exits validate through the
// permissive transport).
func stuckTerraceLoop(
    t *testing.T, x, y, z int32,
) (*Loop, *fakeGame, *state.Bot, *cursorKeyServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, x, y, z)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(stuckZoneX, stuckZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &cursorKeyServer{
        engine: engine, refuseRadius: 0, keyboard: true,
    }

    return loop, game, bot, sim, sink
}

// testReproStuckTerraceWalksOut drives the terrace session of one
// reported point and pins the per spot contract: the bot walks out of
// the reported spot either through the normal found route (the cured
// first spot, no escape log) or through the walk-what-you-can partial
// escape contract (the genuinely sealed second spot). Pre fix the
// return held forever on both.
func testReproStuckTerraceWalksOut(
    t *testing.T, x, y, z int32, wantEscapeLog bool,
) {
    t.Helper()
    loop, game, bot, sim, sink := stuckTerraceLoop(t, x, y, z)

    escaped := false
    for range 4 {
        if escaped {
            break
        }
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            continue
        }
        now := time.Now()
        for range 300 {
            if loop.phase != phaseTownReturn {
                break
            }
            now = now.Add(2 * time.Second)
            driveTownWalkTick(t, loop, game, bot, now, true, sim.consumeAt)
            if px, py, _, ok := bot.SelfPosition(); ok &&
                math.Hypot(float64(px-x), float64(py-y)) >= 96 {
                escaped = true

                break
            }
        }
    }

    require.True(t, escaped,
        "the bot must walk out of the stuck terrace spot")
    if wantEscapeLog {
        require.Contains(t, sink.String(),
            "walking the closest reachable point",
            "the escape must ride the walk-what-you-can partial contract")
    } else {
        require.NotContains(t, sink.String(),
            "walking the closest reachable point",
            "the cured spot plans the normal found route - the escape "+
                "must stay silent")
    }
}

// TestReproStuckTerrace43632WalksOut pins the first reported point:
// the column-first binding lands on the connected ground under the
// terrace edge and the return walks the normal found route.
func TestReproStuckTerrace43632WalksOut(t *testing.T) {
    testReproStuckTerraceWalksOut(t, stuckTerraceAX, stuckTerraceAY,
        stuckTerraceAZ, false)
}

// TestReproStuckTerrace41920WalksOut pins the second reported point:
// the sealed terrace component keeps the pocket escape walk out.
func TestReproStuckTerrace41920WalksOut(t *testing.T) {
    testReproStuckTerraceWalksOut(t, stuckTerraceBX, stuckTerraceBY,
        stuckTerraceBZ, true)
}
