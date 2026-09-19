// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "bytes"
    "io"
    "log"
    "math"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The reproduction of the 2026-09-14 15:10 state dump report (build
// 70d49c5, bot temp10, phase townReturn, uptime 3m46s): the level 15
// character stood at the village plaza cell 45768 49848 -3056 through
// three session restarts and the whole escalation ladder - the stuck
// re-paths, the corridor ban at 43512 50504, the detour freeze and
// the server routed walk that aborted on "the server refused the
// routed walk clicks" - never moving a single cell. The user then
// proved the refusal server side with the OFFICIAL CLIENT: from that
// exact point the client's own ground clicks die too ("из этой точки
// даже через клиент не двигается") - only the ARROW KEYS moved the
// character, and after the arrow walk the clicks worked again
// ("удалось только движение по стрелкам - после этого отлипло").
//
// The Mobius source explains both halves of the user's observation:
//
//   - MoveToLocation carries a movement mode field ("is 0 if cursor
//     keys are used 1 if mouse is used"): the mode 0 branch latches
//     the player's cursor key flag and adopts the packet origin, and
//     while that flag holds, ValidatePosition.runImpl syncs every
//     claimed placement straight into the world and broadcasts it -
//     the character follows the client's own movement simulation
//     with NO click validation at all. That is the arrow walk: the
//     only movement a click-refusing cell answers.
//   - The mode 1 clicks of the same cell run the server's geodata
//     path check from the standing cell, and the user's server
//     refuses them (its geodata seals the plaza cell its own way) -
//     the same refusal the official client met.
//
// The tests below pin the full contract: the refusing cell with the
// cursor key semantics modeled honestly (the cursorKeyServer), the
// bot's cursor key escape walking the character out of the refusing
// cell and the clicks resuming from the escaped ground (the exact
// arrow-key recovery of the user), and the honest abort when the
// server ignores the claims too (the keyboard movement disabled -
// the frozen client reproduction).

// cursorKeyServer simulates the server side of the 15:10 report with
// the cursor key semantics of the reference Mobius master: the mouse
// clicks (movement mode 1) whose origin stands within the refusal
// radius of the dump cell answer ActionFailed and never move the
// character - the user's server refusing its own plaza cell, the
// refusal the official client met live - while the cursor key arm
// (movement mode 0) latches the session's cursor key flag and every
// claimed ValidatePosition syncs the claimed placement straight into
// the world (the cursor key branch of ValidatePosition.runImpl: no
// click validation, the character just follows the stream). A mouse
// click the server accepts clears the cursor key flag - the session
// returns to the mouse movement exactly the way the server does.
type cursorKeyServer struct {
    engine *pathfind.Engine
    // refuseRadius is the radius around the dump cell whose mouse
    // clicks the server refuses: the plaza cell and its immediate
    // ground (the geodata anomaly of the user's deployment).
    refuseRadius float64
    // keyboard mirrors ENABLE_KEYBOARD_MOVEMENT of the reference
    // server (default true): false drops the movement mode 0
    // requests silently - the arrow keys themselves would freeze.
    keyboard bool
    // cursorArmed mirrors the session's cursor key flag
    // (Creature.setCursorKeyMovement).
    cursorArmed bool
    walks       int
    cursorWalks int
    claims      int
    refusals    int
    accepted    int
}

// consumeAt takes the newest requests of the fake game and answers
// them the way the reported server did at the tick's time: the
// refusing cell bounces the mouse clicks with ActionFailed (the
// answer lands one tick-millisecond after the request, inside the
// correlation window of refusalEvidence), the cursor key arm latches
// the flag, and the claims move the character to the claimed
// placement (the zero distance move broadcast of the server's
// ValidateLocation answer).
func (s *cursorKeyServer) consumeAt(
    game *fakeGame, bot *state.Bot, now time.Time,
) {
    // The cursor key arm: the movement mode 0 request latches the
    // session's cursor key flag (MoveToLocation.runImpl), or drops
    // silently when the keyboard movement is disabled.
    if len(game.cursorWalks) > s.cursorWalks {
        s.cursorWalks = len(game.cursorWalks)
        if s.keyboard {
            s.cursorArmed = true
        }
    }
    // The claimed positions: the cursor key branch of
    // ValidatePosition.runImpl - setSyncedXYZ plus the
    // ValidateLocation broadcast, the character follows the stream.
    if len(game.claims) > s.claims {
        for _, claim := range game.claims[s.claims:] {
            if s.cursorArmed {
                moveSelfTo(bot, claim[0], claim[1], claim[2])
            }
            s.claims++
        }
    }
    // The mouse clicks: the refusing ground answers ActionFailed, the
    // clear ground walks its validated destination and returns the
    // session to the mouse movement (the cursor key flag clears).
    if len(game.walks) > s.walks {
        s.walks = len(game.walks)
        selfX, selfY, selfZ, ok := bot.SelfPosition()
        if !ok {
            return
        }
        target := game.walks[len(game.walks)-1]
        from := pathfind.Vec3{
            X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
        }
        to := pathfind.Vec3{
            X: float64(target[0]), Y: float64(target[1]),
            Z: float64(target[2]),
        }
        cellDist := math.Hypot(
            float64(selfX-refusalDumpX),
            float64(selfY-refusalDumpY))
        validated, ok := s.engine.ValidateClick(from, to)
        if cellDist <= s.refuseRadius || !ok {
            s.refusals++
            bot.ApplyActionFailed(now.Add(time.Millisecond))

            return
        }
        s.accepted++
        s.cursorArmed = false
        bot.ApplyMovement(state.Movement{
            ObjectID: 100,
            X:        int32(validated.X), Y: int32(validated.Y),
            Z:     int32(validated.Z),
            DestX: int32(validated.X), DestY: int32(validated.Y),
            DestZ: int32(validated.Z),
        })
    }
}

// cursorEscapeDumpLoop builds the loop of the 15:10 dump scenario on
// the real geodata pack: the character stands on the refusing plaza
// cell, the far hunting zone waits, and the server model carries the
// refusal radius and the cursor key semantics.
func cursorEscapeDumpLoop(
    t *testing.T, keyboard bool,
) (*Loop, *fakeGame, *state.Bot, *cursorKeyServer, *bytes.Buffer) {
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
    sim := &cursorKeyServer{
        engine: engine, refuseRadius: 200, keyboard: keyboard,
    }

    return loop, game, bot, sim, sink
}

// driveTownWalkTick mirrors the production dispatch of
// walkTownWaypoints for one simulated tick after the direct walk
// elimination: the armed escape drives the claims first, the routed
// legs follow their waypoints, and a finished plan optionally ends
// the trip (the tickTownTrip side effect of the return leg). The
// consume call answers the requests the tick sent the way the modeled
// server does. It reports whether the walk plan finished.
func driveTownWalkTick(
    t *testing.T, loop *Loop, game *fakeGame, bot *state.Bot,
    now time.Time, endOnFinish bool,
    consume func(*fakeGame, *state.Bot, time.Time),
) bool {
    t.Helper()
    x, y, z, ok := bot.SelfPosition()
    require.True(t, ok, "the character position must be known")
    if loop.cursorEscape.armed {
        loop.driveCursorKeyEscape(now, x, y)
        consume(game, bot, now)

        return false
    }
    finished := loop.followWaypoints(x, y, z, now)
    if finished && endOnFinish {
        loop.endTownTrip("back at the farm spot")
    }
    consume(game, bot, now)

    return finished
}

// TestReproCursorKeyEscapeWalksOutOfTheRefusingCell pins the
// arrow-key recovery of the 15:10 report: the plaza cell refuses
// every mouse click (the official client's clicks die on it too),
// the cursor key escape arms the movement mode 0 stream and walks
// the character off the refusing ground claim by claim, and the
// server routed clicks resume from the escaped ground - the walk
// reaches the hunting zone, the two minute contract of the report
// holds. The claimed steps never name a wet cell: the water guard
// holds for the claims exactly the way it holds for the clicks.
func TestReproCursorKeyEscapeWalksOutOfTheRefusingCell(t *testing.T) {
    loop, game, bot, sim, sink := cursorEscapeDumpLoop(t, true)

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
            // The production dispatch mirror drives the armed escape
            // first; the routed legs follow the waypoints.
            if driveTownWalkTick(t, loop, game, bot, now, true,
                sim.consumeAt) {
                break
            }
        }
        if x, y, _, ok := bot.SelfPosition(); ok &&
            inZoneSquare(x, y, refusalDumpZoneX, refusalDumpZoneY, 633) &&
            !loop.tripActive() {
            arrived = true
        }
    }

    require.True(t, arrived,
        "the cursor key escape must walk the character out of the "+
            "refusing plaza cell and the walk must reach the zone")
    require.NotEmpty(t, game.cursorWalks,
        "the cursor key arm (the movement mode 0 request) was sent")
    require.NotEmpty(t, game.claims,
        "the claimed ValidatePosition stream was sent")
    require.Positive(t, sim.refusals,
        "the refusing cell bounced the mouse clicks first")
    require.Positive(t, sim.accepted,
        "the clicks resumed from the escaped ground")
    require.Empty(t, loop.frozenAreas,
        "a server that refuses the clicks does not name frozen "+
            "corridors - the session must not poison the planner")
    require.Contains(t, sink.String(), "walking along the planned route",
        "the escape line names the route following recovery")
    require.Regexp(t,
        `resuming the planned walk clicks`,
        sink.String(),
        "the clicks resume after the escape")
    require.NotContains(t, sink.String(),
        "the cursor key escape made no progress",
        "the server follows the claims of this model")
    // The water guard holds for the claimed steps: every claim line
    // from the standing cell to the claimed placement stays dry.
    prev := pathfind.Vec3{
        X: float64(refusalDumpX), Y: float64(refusalDumpY),
        Z: float64(refusalDumpZ),
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

// TestReproCursorKeyEscapeAbortsWhenTheServerIgnoresTheClaims pins
// the frozen-client reproduction: a server that refuses the mouse
// clicks AND drops the movement mode 0 requests silently (the
// keyboard movement disabled) freezes every movement channel - the
// exact situation the user met with the official client before the
// arrows worked ("из этой точки даже через клиент не двигается").
// The escape burns its attempts against the ignoring server, the
// corridor bans stay off (the refusal evidence owns the verdict) and
// the trips abort with the honest reason - the character never moves
// a single cell.
func TestReproCursorKeyEscapeAbortsWhenTheServerIgnoresTheClaims(
    t *testing.T,
) {
    loop, game, bot, sim, sink := cursorEscapeDumpLoop(t, false)

    for range 4 {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.returnToZone()
        if loop.phase != phaseTownReturn {
            continue
        }
        now := time.Now()
        for i := 0; i < 400 && loop.phase == phaseTownReturn; i++ {
            now = now.Add(2 * time.Second)
            // The production dispatch mirror drives the armed escape
            // first; the routed legs follow the waypoints.
            driveTownWalkTick(t, loop, game, bot, now, false,
                sim.consumeAt)
        }
    }

    require.Zero(t, sim.accepted,
        "the refusing cell never walks a mouse click")
    require.Positive(t, sim.refusals,
        "the refusing cell bounced the clicks")
    x, y, _, _ := bot.SelfPosition()
    require.InDelta(t, float64(refusalDumpX), float64(x), 1,
        "the character never moved a cell - the frozen client")
    require.InDelta(t, float64(refusalDumpY), float64(y), 1)
    require.Empty(t, loop.frozenAreas,
        "the refusal evidence keeps the corridor bans off")
    require.Contains(t, sink.String(),
        "the cursor key escape made no progress",
        "the ignoring server is named honestly")
    require.Contains(t, sink.String(),
        "walk stuck, no movement since the re-path",
        "the attempts burn out and the trip aborts with the honest "+
            "reason")
    require.NotContains(t, sink.String(), "by the server routing",
        "the direct server routed walk never arms")
    require.GreaterOrEqual(t,
        strings.Count(sink.String(), "the cursor key escape made no "+
            "progress"),
        cursorEscapeAttemptsMax,
        "the escape attempts burned out before the honest abort")
}
