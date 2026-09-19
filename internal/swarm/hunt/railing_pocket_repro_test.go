// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The 2026-09-19 railing pocket round: the bot stood at 43736 47048
// -2992 (an elven village deck cell just off the railings) and could
// not build a route anywhere - the standing polygon is a linkless one
// cell island the strict edge-only mesh link graph cannot connect
// (every axis neighbor walled by the railing segments, the diagonal
// squeeze the server movement allows carries the only way out), so
// every plan attempt answered the bare not found and the return held
// forever. The mesh pocket escape answers the walk out and the
// followup plan cycle routes from the connected ground.

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

// The dump cell (the reported pocket) and the hunting zone of the
// session (the elven village zone the 17:18 dump walked home to).
const (
    pocketDumpX = int32(43736)
    pocketDumpY = int32(47048)
    pocketDumpZ = int32(-2992)
    pocketZoneX = int32(25500)
    pocketZoneY = int32(51095)
)

// railingPocketLoop builds the loop of the railing pocket session on
// the real pack and the real mesh tiles: the character stands in the
// linkless deck cell, the far hunting zone waits, and the server model
// is the honest one (no refusal answers - the whole freeze lived in
// the route graph).
func railingPocketLoop(
    t *testing.T,
) (*Loop, *fakeGame, *state.Bot, *cursorKeyServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, pocketDumpX, pocketDumpY, pocketDumpZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.SetHuntingZone(pocketZoneX, pocketZoneY, 633)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &cursorKeyServer{
        engine: engine, refuseRadius: 0, keyboard: true,
    }

    return loop, game, bot, sim, sink
}

// TestReproRailingPocketWalksOut pins the escape contract of the
// report: from the railing pocket cell the bot plans the walk out
// (the mesh pocket escape rides the walk-what-you-can partial
// contract), the follower clicks deliver it (the click transport
// validates the one sided wall pairs the link builder refuses), and
// the followup plan cycles route from the connected ground - the
// character leaves the pocket cell. Pre escape the return held
// forever: every plan attempt answered the bare not found.
func TestReproRailingPocketWalksOut(t *testing.T) {
    loop, game, bot, sim, sink := railingPocketLoop(t)

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
            if x, y, _, ok := bot.SelfPosition(); ok &&
                math.Hypot(float64(x-pocketDumpX),
                    float64(y-pocketDumpY)) >= 96 {
                escaped = true

                break
            }
        }
    }

    require.True(t, escaped,
        "the bot must walk out of the railing pocket cell")
    require.Contains(t, sink.String(),
        "walking the closest reachable point",
        "the escape must ride the walk-what-you-can partial contract")
}
