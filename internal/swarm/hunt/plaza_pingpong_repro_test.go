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

// The reproduction of the 2026-09-20 17:09 farm readiness ping-pong
// report (build 2524196, bot temp1): after the weapon and armor stops
// the trip walked to the grocery trader Herbiel and the follower
// ping-ponged between the plaza cell A (44584 46944 -2920, the Unoren
// customer stand the character traded at) and the first route sample
// 63 units out (44528 46972) - nine alternating MoveToLocation clicks
// while the server accepted and delivered every one of them. The
// alternating clicks made no net progress, the frozen trip verdict
// banned the "frozen corridor" at the plaza cell for the session and
// every later route search answered from a sealed plaza (the partial
// to 44295 46944, then "no path at all" for the rest of the session).
//
// The root cause the dump named and the probes confirmed: the waypoint
// advance gate (segmentAdvanceClear) asked the grid engine's symmetric
// line of sight oracle - the A* node rule (a step must climb no more
// than 40 units up AND down, both cells' walls open in both
// directions) - while the clicks the follower actually sends pass the
// server click transport port (ValidateClick: climbs limited, drops
// free, the source wall plus the anti corner cut). On ordinary
// terrain the two oracles disagree: the advance gate refused lines
// the server happily walks, the cursor pinned on the plan's own start
// waypoint, the short click extension escaped ~63 units out and the
// pinned cursor clicked the character right back - the ping-pong.
const (
    // plazaPingPongX/Y/Z is the plaza cell A of the report: the
    // character stands on the Unoren customer stand (the tracker z is
    // the live server frame; the pack models the same cell 64 units
    // lower, the vintage shift the frame offset measures).
    plazaPingPongX = int32(44584)
    plazaPingPongY = int32(46944)
    plazaPingPongZ = int32(-2920)
    // herbielStandX/Y/Z is the grocery trader's customer stand point
    // of the stand table (the npc segment destination of the report).
    herbielStandX = int32(42798)
    herbielStandY = int32(50101)
    herbielStandZ = int32(-2984)
    // plazaPingPongArrive is the acceptance radius of the reproduced
    // walk: the character must reach the stand neighborhood.
    plazaPingPongArrive = 150.0
    // plazaPingPongBackClick is the backward click detector radius: a
    // click aimed back inside the plaza spawn neighborhood while the
    // character already left it is the ping-pong signature.
    plazaPingPongBackClick = 100.0
    // plazaPingPongLeft is the distance from the spawn beyond which
    // the character counts as having left the plaza cell.
    plazaPingPongLeft = 200.0
)

// plazaServer models the live server of the report: an honest click
// transport with no refusing ground at all - every mouse click the
// geodata validation accepts moves the character to the validated
// destination (the server moved the character on every click of the
// dump - the freeze verdict the old corridor ban answered was never a
// server freeze), a click the validation cancels (the self click of
// the pinned cursor: the correction collapses onto the walker) answers
// ActionFailed within the same tick the way the dump's answers landed.
// The cursor key semantics ride along (the reference Mobius master):
// the movement mode 0 arm latches the session's cursor key flag and
// every claimed ValidatePosition syncs the claimed placement straight
// into the world - the arrow key walk the frozen ladder hands the
// movement to (see beginCursorKeyEscape).
type plazaServer struct {
    engine      *pathfind.Engine
    walks       int
    cursorWalks int
    claims      int
    cursorArmed bool
    accepted    int
    refusals    int
}

// consumeAt answers the requests of one tick: the cursor key arm
// latches the flag, the claims sync the claimed placements (the
// server frame z rides the pack's height delta - the sim keeps the
// character on the surface the live server kept it on, the plaza is
// flat and the z holds), and the newest walk click walks to its
// validated destination.
func (s *plazaServer) consumeAt(
    game *fakeGame, bot *state.Bot, now time.Time,
) {
    // The cursor key arm: the movement mode 0 request latches the
    // session's cursor key flag (MoveToLocation.runImpl).
    if len(game.cursorWalks) > s.cursorWalks {
        s.cursorWalks = len(game.cursorWalks)
        s.cursorArmed = true
    }
    // The claimed positions: the cursor key branch of
    // ValidatePosition.runImpl - the character follows the stream.
    if len(game.claims) > s.claims {
        for _, claim := range game.claims[s.claims:] {
            if s.cursorArmed {
                moveSelfTo(bot, claim[0], claim[1], claim[2])
            }
            s.claims++
        }
    }
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
        s.refusals++
        bot.ApplyActionFailed(now)

        return
    }
    s.accepted++
    s.cursorArmed = false
    bot.ApplyMovement(state.Movement{
        ObjectID: 100,
        X:        int32(validated.X), Y: int32(validated.Y),
        Z:     selfZ,
        DestX: int32(validated.X), DestY: int32(validated.Y),
        DestZ: selfZ,
    })
}

// plazaPingPongLoop builds the loop of the report on the real geodata
// pack and the real mesh tiles: the character stands on the plaza cell
// A, the trip holds the Herbiel merchant stop and the npc segment to
// the stand point is armed exactly the way advanceTripStop armed it
// (the npc rung search with its close radius).
func plazaPingPongLoop(
    t *testing.T,
) (*Loop, *fakeGame, *state.Bot, *plazaServer, *bytes.Buffer) {
    t.Helper()
    engine := reproEngine(t)
    nav := NewNavmeshNavigator(engine, spawnDumpMesh(t))
    bot := newTestBot()
    moveSelfTo(bot, plazaPingPongX, plazaPingPongY, plazaPingPongZ)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    loop.lastHit = time.Now().Add(-time.Minute)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(
        io.MultiWriter(sink, eventMirror{bot: bot}), "", 0))
    sim := &plazaServer{engine: engine}

    loop.phase = phaseTownWalk
    loop.tripStart = time.Now()
    loop.tripStops = []tripStop{{
        merchant: townNpc{
            TemplateID: 7150, Name: "Herbiel",
            X: 42766, Y: 50037, Z: -2984,
        },
    }}
    loop.segmentRadius = tripApproachRadius
    require.True(t, loop.startWalkNpcSegment(pathfind.Vec3{
        X: float64(herbielStandX), Y: float64(herbielStandY),
        Z: float64(herbielStandZ),
    }), "the Herbiel npc segment must plan on the real pack")

    return loop, game, bot, sim, sink
}

// TestReproPlazaPingPongWalksToHerbiel pins the whole report contract:
// from the plaza cell the follower walks the planned route to the
// Herbiel stand - the cursor advances off the plan's own start onto
// the first real waypoint (the advance gate follows the same server
// click transport the clicks pass), no click ever aims the character
// back into the plaza cell it already left and no frozen corridor ban
// seals the plaza (the ban of the report poisoned every later route
// search of the session into "no path at all").
func TestReproPlazaPingPongWalksToHerbiel(t *testing.T) {
    disablePace(t)
    loop, game, bot, sim, sink := plazaPingPongLoop(t)

    var backClicks int
    walkCount := 0
    leftPlaza := false
    deadline := time.Now().Add(180 * time.Second)
    for time.Now().Before(deadline) {
        loop.tick()
        sim.consumeAt(game, bot, time.Now())
        selfX, selfY, _, ok := bot.SelfPosition()
        if !ok {
            continue
        }
        if len(game.walks) > walkCount {
            for ; walkCount < len(game.walks); walkCount++ {
                w := game.walks[walkCount]
                t.Logf("walk #%d -> %d %d %d (self %d %d, wpIndex %d of %d, rePaths %d)",
                    walkCount+1, w[0], w[1], w[2], selfX, selfY,
                    loop.wpIndex, len(loop.waypoints), loop.rePaths)
            }
        }
        distSpawn := math.Hypot(
            float64(selfX-plazaPingPongX),
            float64(selfY-plazaPingPongY))
        if distSpawn > plazaPingPongLeft {
            leftPlaza = true
        }
        // The ping-pong detector: once the character left the plaza,
        // every click target must leave it too (a target back inside
        // the spawn neighborhood walks the character backward - the
        // alternating click signature of the report).
        if leftPlaza && len(game.walks) > 0 {
            w := game.walks[len(game.walks)-1]
            targetDist := math.Hypot(
                float64(w[0]-plazaPingPongX),
                float64(w[1]-plazaPingPongY))
            if targetDist <= plazaPingPongBackClick {
                backClicks++
            }
        }
        if dist := math.Hypot(
            float64(selfX-herbielStandX),
            float64(selfY-herbielStandY)); dist <= plazaPingPongArrive {
            t.Logf("reached the Herbiel stand neighborhood: "+
                "%d %d (walks %d, accepted %d, refusals %d, "+
                "rePaths %d)",
                selfX, selfY, len(game.walks), sim.accepted,
                sim.refusals, loop.rePaths)

            break
        }
        time.Sleep(10 * time.Millisecond)
    }

    selfX, selfY, _, ok := bot.SelfPosition()
    require.True(t, ok)
    dist := math.Hypot(
        float64(selfX-herbielStandX),
        float64(selfY-herbielStandY))
    t.Logf("end state: %d %d, dist to the stand %.0f, walks %d, "+
        "back clicks %d, rePaths %d",
        selfX, selfY, dist, len(game.walks), backClicks, loop.rePaths)
    t.Logf("log tail:\n%s", sink.String())

    require.LessOrEqual(t, dist, plazaPingPongArrive,
        "the walk must reach the Herbiel stand neighborhood - the "+
            "report held the character ping-ponging on the plaza "+
            "forever")
    require.Zero(t, backClicks,
        "no click may aim the character back into the plaza cell it "+
            "already left - the alternating click signature of the "+
            "ping-pong")
    require.NotContains(t, sink.String(), "frozen corridor",
        "no frozen corridor ban may seal the walkable plaza - the "+
            "report's ban poisoned every later route search of the "+
            "session")
}

// TestSegmentAdvanceClearFollowsTheClickTransport pins the root cause
// itself on the real pack: the advance gate of the follower must
// answer with the same oracle the click transport answers with. The
// plaza cell to the first real waypoint of the Herbiel plan is the
// exact line of the report - the grid line of sight oracle refuses it
// (the symmetric node rule) while the server click transport accepts
// it (the server walked the click on every hop of the dump).
func TestSegmentAdvanceClearFollowsTheClickTransport(t *testing.T) {
    disablePace(t)
    loop, _, bot, _, _ := plazaPingPongLoop(t)

    selfX, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    require.Equal(t, plazaPingPongX, selfX)
    require.Greater(t, len(loop.waypoints), 2,
        "the Herbiel plan carries the multi waypoint route of the "+
            "report")

    // The first successor of the plan's own start cell: the gate the
    // cursor advance runs on the very first follow tick.
    require.True(t, loop.segmentAdvanceClear(selfX, selfY, selfZ, 1),
        "the advance gate must clear the first real waypoint - the "+
            "server click transport accepts the line the follower "+
            "clicks (the dump's server walked it on every hop) while "+
            "the grid line of sight oracle of the report refused it")

    // The same line through the click transport port: the contract
    // the gate must mirror.
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    wp := loop.waypoints[1]
    to := pathfind.Vec3{
        X: wp.X, Y: wp.Y,
        Z: anchorZToServerFrame(wp.Z, loop.segmentFrameOffset),
    }
    corrected, ok := loop.navigator.ValidateClick(from, to)
    require.True(t, ok,
        "the server click transport accepts the first leg line")
    require.LessOrEqual(t,
        math.Hypot(corrected.X-to.X, corrected.Y-to.Y),
        waypointPassDist,
        "the correction lands within the pass radius of the target")
}
