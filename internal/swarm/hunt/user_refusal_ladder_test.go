package hunt

import (
    "bytes"
    "io"
    "log"
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The temple entrance refusal round: the owner dump of 2026-09-19
// 20:32 held the character on the temple porch (44696 51992 -2792,
// 71 units into a 371 unit walk) for the whole walk window while the
// server answered every straight re-click with ActionFailed - the
// plain follower re-issued the same aim every walk request period
// and the same refusal bounced forever. The server class behind the
// dump is the 15:10 class: the deployment validates the click lines
// with its own geodata and answers NO mouse click from the ground
// its model seals (the official client met the same refusals). The
// recovery is the refusal ladder of the town walk carried to the
// manual walk: the varied aims first, the cursor key escape (the
// claimed ValidatePosition stream the server follows without any
// click validation) once the variants spent.

// porchServer models the reported deployment for the manual walk:
// the mouse clicks whose origin stands within the porch radius
// answer ActionFailed and never move the character (the geodata the
// user's server validates the porch lines with), the clicks from the
// clear ground validate against the real pack and walk their
// destination, and the cursor key arm plus the claimed
// ValidatePosition stream sync the claimed placements straight into
// the world (the reference master's cursor key branch of
// ValidatePosition.runImpl - no click validation at all).
type porchServer struct {
    engine         *pathfind.Engine
    porchX, porchY int32
    refuseRadius   float64
    cursorArmed    bool
    walks          int
    cursorWalks    int
    claims         int
    refusals       int
    accepted       int
}

// consume answers the requests the tick sent the way the reported
// server does: the arm latches, the claims move the character, the
// clicks from the porch radius bounce.
func (s *porchServer) consume(
    game *fakeGame, bot *state.Bot, now time.Time,
) {
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
    if len(game.walks) > s.walks {
        s.walks = len(game.walks)
        selfX, selfY, _, ok := bot.SelfPosition()
        if !ok {
            return
        }
        target := game.walks[len(game.walks)-1]
        from := pathfind.Vec3{
            X: float64(selfX), Y: float64(selfY),
        }
        to := pathfind.Vec3{
            X: float64(target[0]), Y: float64(target[1]),
            Z: float64(target[2]),
        }
        cellDist := math.Hypot(
            float64(selfX-s.porchX), float64(selfY-s.porchY))
        validated, valid := s.engine.ValidateClick(from, to)
        if cellDist <= s.refuseRadius || !valid {
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

// TestUserWalkRefusalLadderWalksTheRefusingPorch pins the manual
// walk refusal ladder end to end on the real pack and the real mesh
// tiles: the character wakes on the temple porch, the walk command
// aims the interior cell, every click the porch answers bounces with
// ActionFailed, the varied aims bounce with it (the refusal is
// origin owned on this deployment), and the cursor key escape walks
// the character out of the refusing ground - the plan completes
// instead of burning the whole walk window on the same refused aim.
// Pre fix the follower re-issued the plain aim forever and the walk
// timed out.
func TestUserWalkRefusalLadderWalksTheRefusingPorch(t *testing.T) {
    engine := reproEngine(t)
    mesh := spawnDumpMesh(t)
    nav := NewNavmeshNavigator(engine, mesh)
    bot := newTestBot()
    moveSelfTo(bot, 44696, 51992, -2792)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetNavigator(nav)
    sink := &bytes.Buffer{}
    loop.SetLogger(log.New(io.MultiWriter(sink, eventMirror{bot: bot}),
        "", 0))
    sim := &porchServer{
        engine: engine, porchX: 44696, porchY: 51992,
        refuseRadius: 100,
    }

    loop.userMovement(state.Command{
        Kind: state.CommandMove, X: 44718, Y: 52291, Z: -2792,
    })

    base := time.Now()
    done := false
    for i := range 240 {
        now := base.Add(time.Duration(i*2) * time.Second)
        loop.tickUserMove(now)
        sim.consume(game, bot, now)
        if loop.userWpIndex >= len(loop.userWaypoints) &&
            len(loop.userWaypoints) > 0 {
            done = true

            break
        }
    }
    require.True(t, done, "the manual walk must complete the plan, "+
        "the log: %s", sink.String())
    require.Positive(t, sim.refusals,
        "the porch must have refused the straight clicks")
    require.GreaterOrEqual(t, sim.cursorWalks, 1,
        "the cursor key escape must have armed")
    require.Contains(t, sink.String(), "varying the aim")
    require.Contains(t, sink.String(), "cursor key escape")
    _, _, _, ok := bot.SelfPosition()
    require.True(t, ok)
    x, y, _, _ := bot.SelfPosition()
    inside := math.Hypot(float64(x-44718), float64(y-52291))
    require.LessOrEqual(t, inside, 150.0,
        "the character must stand on the interior cell, got %d %d",
        x, y)
}
