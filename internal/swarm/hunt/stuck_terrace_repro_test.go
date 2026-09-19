// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The 2026-09-20 stuck point round: the owner found the fleet frozen
// on two elven village terrace spots - 43632 50560 -2960 and
// 41920 52128 -3000. Both stand on link components the strict edge
// only mesh link graph cannot leave (52 and 29 polygons, the 640x752
// and the 592x432 boxes) while the grid engine plans out of the very
// same cells through the diagonal squeezes the server movement
// channels allow. The components outgrew the old 320 pocket side, so
// the pocket escape declined and the route attempts answered the bare
// not found (or the partial that walks to the component's inner
// boundary and strands it there) - the bots stood frozen. The
// widened escape bound plus the horizontal displacement floor of the
// exit scan answer the walk out.

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
// reported point: the bot plans the walk home, the escape rides the
// walk-what-you-can partial contract, the follower clicks deliver it
// and the followup plan cycles route from the connected ground - the
// character leaves the terrace. Pre fix the return held forever.
func testReproStuckTerraceWalksOut(
    t *testing.T, x, y, z int32,
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
    require.Contains(t, sink.String(),
        "walking the closest reachable point",
        "the escape must ride the walk-what-you-can partial contract")
}

// TestReproStuckTerrace43632WalksOut pins the first reported point.
func TestReproStuckTerrace43632WalksOut(t *testing.T) {
    testReproStuckTerraceWalksOut(t, stuckTerraceAX, stuckTerraceAY,
        stuckTerraceAZ)
}

// TestReproStuckTerrace41920WalksOut pins the second reported point.
func TestReproStuckTerrace41920WalksOut(t *testing.T) {
    testReproStuckTerraceWalksOut(t, stuckTerraceBX, stuckTerraceBY,
        stuckTerraceBZ)
}
