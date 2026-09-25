// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The long route movement abuse scenario tests: the desync route
// drift, the cursor route stream and the fast route sprint - the
// three world scale rounds that ride the pathfinding corridor from
// the elven forest spawn to Gludio.

// syntheticRoute builds a deterministic corridor of guides for the
// ladder tests: a straight west line of 400 unit hops from the spawn
// toward the Gludio longitude.
func syntheticRoute(hops int, hop float64) []guidePoint {
    route := make([]guidePoint, 0, hops+1)
    for i := 1; i <= hops; i++ {
        route = append(route, guidePoint{
            X: int32(float64(elvenSpawnX) - hop*float64(i)),
            Y: elvenSpawnY,
            Z: elvenSpawnZ,
        })
    }

    return route
}

// TestRouteScenarioDescriptionsPinTheContract pins the descriptions
// of the three route rounds: the level 19 start state, the million
// adena wallet, the Gludio arrival point and the pathfinding guides.
func TestRouteScenarioDescriptionsPinTheContract(t *testing.T) {
    defs := Definitions()
    byID := map[string]TestDef{}
    for _, def := range defs {
        byID[def.ID] = def
    }

    desync := byID[desyncRouteID]
    require.Equal(t, desyncRouteAccount, desync.Account)
    require.Equal(t, desyncRouteTimeout, desync.Timeout)
    require.NotNil(t, desync.Scenario)
    for _, needle := range []string{
        "LEVEL 19", "ONE MILLION ADENA", "46045 41251 -3440",
        "-12787 122779 -3114", "PATHFINDING", "no distance cap",
        "700 units", "twice", "100,000+ units",
    } {
        require.Contains(t, desync.Description, needle)
    }

    cursor := byID[cursorRouteID]
    require.Equal(t, cursorRouteAccount, cursor.Account)
    require.Equal(t, cursorRouteTimeout, cursor.Timeout)
    require.NotNil(t, cursor.Scenario)
    for _, needle := range []string{
        "LEVEL 19", "ONE MILLION ADENA", "-12787 122779 -3114",
        "PATHFINDING", "94 unit steps", "setSyncedXYZ",
        "twice", "100,000+ units",
    } {
        require.Contains(t, cursor.Description, needle)
    }

    fast := byID[fastRouteID]
    require.Equal(t, fastRouteAccount, fast.Account)
    require.Equal(t, fastRouteTimeout, fast.Timeout)
    require.NotNil(t, fast.Scenario)
    for _, needle := range []string{
        "LEVEL 19", "ONE MILLION ADENA", "-12787 122779 -3114",
        "FULL QUEUE RATE", "NO distance cap", "flood protector",
        "100,000+ units",
    } {
        require.Contains(t, fast.Description, needle)
    }
}

// TestRouteResetIsTheLevel19Wallet pins the start state of the route
// rounds: the level 19 character with the million adena wallet at
// the elven forest creation spawn.
func TestRouteResetIsTheLevel19Wallet(t *testing.T) {
    for _, account := range []string{
        desyncRouteAccount, cursorRouteAccount, fastRouteAccount,
    } {
        reset := routeReset(account)
        require.Equal(t, account, reset.Account)
        require.Equal(t, account, reset.Char)
        require.Equal(t, int32(routeLevel), reset.Level)
        require.Equal(t, reset.Exp, int64(level19Exp))
        require.Equal(t, reset.Adena, int64(routeAdena))
        require.Empty(t, reset.Items)
        require.Equal(t, int32(elvenSpawnX), reset.X)
        require.Equal(t, int32(elvenSpawnY), reset.Y)
        require.Equal(t, int32(elvenSpawnZ), reset.Z)
        require.Equal(t, int32(level19HP), reset.MaxHP)
        require.Equal(t, int32(level19MP), reset.MaxMP)
        require.Equal(t, int32(level19CP), reset.MaxCP)
    }
}

// TestRouteArrivalPointGeometry pins the Gludio arrival point and the
// corridor premise: the straight spawn to Gludio distance clears the
// pass gate of the covered distance.
func TestRouteArrivalPointGeometry(t *testing.T) {
    straight := math.Hypot(float64(gludioX-elvenSpawnX),
        float64(gludioY-elvenSpawnY))
    require.InDelta(t, routeStraightDistance, straight, 1.0)
    require.Greater(t, straight, routeMinDistance,
        "the corridor gate must sit under the straight distance")
    require.Equal(t, -12787, gludioX)
    require.Equal(t, 122779, gludioY)
    require.Equal(t, -3114, gludioZ)
}

// TestDesyncRouteGuidesStride pins the desync guide ladder: every
// guide sits at least the stride away from the previous one (the
// claims must clear the correction band of the handler), the ladder
// starts at the first route waypoint and always closes on the last.
func TestDesyncRouteGuidesStride(t *testing.T) {
    route := syntheticRoute(300, 400)
    route = append(route, guidePoint{X: gludioX, Y: gludioY, Z: gludioZ})

    guides := desyncRouteGuides(route)
    require.NotEmpty(t, guides)
    require.Equal(t, route[0], guides[0],
        "the ladder starts at the first route waypoint")
    require.Equal(t, route[len(route)-1], guides[len(guides)-1],
        "the ladder closes on the Gludio arrival point")
    for i := 1; i < len(guides); i++ {
        step := math.Hypot(
            float64(guides[i].X-guides[i-1].X),
            float64(guides[i].Y-guides[i-1].Y))
        require.GreaterOrEqual(t, step, routeGuideStride,
            "guide %d sits closer than the stride", i)
    }

    sparse := desyncRouteGuides([]guidePoint{
        {X: elvenSpawnX - 5000, Y: elvenSpawnY, Z: elvenSpawnZ},
        {X: gludioX, Y: gludioY, Z: gludioZ},
    })
    require.Len(t, sparse, 2,
        "a sparse corridor keeps every guide - the stride only "+
            "collapses the dense ones")
}

// TestCursorRouteStepsUnderTheBand pins the cursor stream
// interpolation: every step stays under the move speed band (only
// the cursor key branch can adopt it) and the stream closes exactly
// on the corridor end.
func TestCursorRouteStepsUnderTheBand(t *testing.T) {
    route := []guidePoint{
        {X: elvenSpawnX - 300, Y: elvenSpawnY, Z: elvenSpawnZ},
        {X: elvenSpawnX - 1300, Y: elvenSpawnY + 400, Z: elvenSpawnZ},
        {X: elvenSpawnX - 2600, Y: elvenSpawnY + 400, Z: elvenSpawnZ},
    }
    steps := cursorRouteSteps(route)
    require.NotEmpty(t, steps)
    from := guidePoint{X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ}
    for i, step := range steps {
        hop := math.Hypot(float64(step.X-from.X), float64(step.Y-from.Y))
        require.LessOrEqual(t, hop, float64(cursorHopDistance)+1.0,
            "step %d exceeds the hop bound", i)
        from = step
    }
    require.Equal(t, route[len(route)-1], steps[len(steps)-1],
        "the stream closes exactly on the corridor end")
}

// TestCursorRouteCyclesCoverTheStream pins the cycle math of the
// cursor stream: the cycles cover every claim.
func TestCursorRouteCyclesCoverTheStream(t *testing.T) {
    require.Equal(t, 7, cursorRouteCycles(make([]guidePoint, 20)))
    require.Equal(t, 1, cursorRouteCycles(make([]guidePoint, 1)))
    steps := cursorRouteSteps(syntheticRoute(300, 400))
    covered := cursorRouteCycles(steps) * cursorCycleClaims
    require.GreaterOrEqual(t, covered, len(steps),
        "the cycles cover the whole stream")
}

// TestFastRouteBatchesStayUnderTheQueueBound pins the sprint
// batching: every batch stays under the command queue drop bound and
// the batches cover the whole ladder.
func TestFastRouteBatchesStayUnderTheQueueBound(t *testing.T) {
    require.Less(t, routeBatchClaims, 32,
        "the batch must stay under the 32 slot queue drop bound")
    guides := syntheticRoute(300, 700)
    batches := fastRouteBatches(guides)
    require.Equal(t, (300+routeBatchClaims-1)/routeBatchClaims, batches)
    require.GreaterOrEqual(t, batches*routeBatchClaims, len(guides),
        "the batches cover the whole ladder")
    require.LessOrEqual(t, len(guides)-(batches-1)*routeBatchClaims,
        routeBatchClaims, "the last batch stays under the bound")
}

// TestRouteDistanceXY pins the ladder distance math: the collapsed
// desync ladder keeps the corridor length of the full route.
func TestRouteDistanceXY(t *testing.T) {
    route := syntheticRoute(250, 400)
    full := routeDistanceXY(route)
    collapsed := routeDistanceXY(desyncRouteGuides(route))
    require.InDelta(t, full, collapsed, 700.0,
        "the stride collapse shortens the corridor by at most one "+
            "stride")
    require.Greater(t, full, 99_000.0,
        "the synthetic corridor clears the distance gate")
}

// TestRouteEffectiveSpeed pins the speed math of the route rounds:
// the spawn displacement over the claim window, zero for a degenerate
// window.
func TestRouteEffectiveSpeed(t *testing.T) {
    started := time.Now()
    lastAt := started.Add(60 * time.Second)
    speed := routeEffectiveSpeed(gludioX, gludioY, started, lastAt)
    require.InDelta(t, routeStraightDistance/60.0, speed, 0.001)

    require.Zero(t, routeEffectiveSpeed(
        gludioX, gludioY, time.Time{}, lastAt))
    require.Zero(t, routeEffectiveSpeed(
        gludioX, gludioY, started, time.Time{}))
    require.Zero(t, routeEffectiveSpeed(
        gludioX, gludioY, lastAt, started),
        "a negative window reports no speed")
}

// TestEvaluateRouteRide pins the pass gates of the route rounds: the
// movement confirmation, the speed factor and the corridor distance
// thresholds decide the checks.
func TestEvaluateRouteRide(t *testing.T) {
    test := &Test{def: TestDef{ID: desyncRouteID}}
    test.setChecks(routeChecks("the test ladder"))

    evaluateRouteRide(test, "nothing confirmed", false, 100, 1000,
        144)
    for _, id := range []string{
        checkRouteMove, checkRouteSpeed, checkRouteDist,
    } {
        require.False(t, checkDone(test, id), "%s must fail short", id)
    }

    evaluateRouteRide(test, "the guides confirmed", true,
        routeStraightDistance/60.0, 110_000, 144)
    require.True(t, checkDone(test, checkRouteMove))
    require.True(t, checkDone(test, checkRouteSpeed),
        "1675 clears twice the 144 run speed")
    require.True(t, checkDone(test, checkRouteDist))
}

// TestEvaluateRouteArrivalStored pins the arrival and store checks:
// a placement at the Gludio point passes, the forest spawn fails.
func TestEvaluateRouteArrivalStored(t *testing.T) {
    test := &Test{def: TestDef{ID: desyncRouteID}}
    test.setChecks(routeChecks("the test ladder"))

    evaluateRouteArrival(test, gludioX-60, gludioY+40)
    require.True(t, checkDone(test, checkRouteArrive))

    evaluateRouteArrival(test, elvenSpawnX, elvenSpawnY)
    require.False(t, checkDone(test, checkRouteArrive),
        "the forest spawn fails the arrival check")

    evaluateRouteStored(test, gludioX-60, gludioY+40, gludioZ+900)
    require.True(t, checkDone(test, checkRouteStored),
        "the z band tolerates the monotonic drift of the desync "+
            "branch")

    evaluateRouteStored(test, elvenSpawnX, elvenSpawnY, elvenSpawnZ)
    require.False(t, checkDone(test, checkRouteStored),
        "the forest spawn fails the store check")
}

// TestRouteScenarioCommandsPushClean pins the command payloads of the
// route rounds: the sprint batches queue the guide claims in ladder
// order and the cursor arm targets the Gludio arrival point.
func TestRouteScenarioCommandsPushClean(t *testing.T) {
    bot := state.NewBot(fastRouteAccount)
    guides := []guidePoint{
        {X: elvenSpawnX - 900, Y: elvenSpawnY, Z: elvenSpawnZ},
        {X: elvenSpawnX - 1800, Y: elvenSpawnY, Z: elvenSpawnZ},
        {X: gludioX, Y: gludioY, Z: gludioZ},
    }
    for _, guide := range guides {
        pushRouteClaim(bot, guide)
    }
    pushRouteArm(bot)
    pushRouteProbe(bot, guides[len(guides)-1])

    commands := drainAllCommands(bot)
    require.Len(t, commands, len(guides)+2)
    for i, guide := range guides {
        require.Equal(t, state.CommandClaimPosition, commands[i].Kind)
        require.Equal(t, guide.X, commands[i].X)
        require.Equal(t, guide.Y, commands[i].Y)
        require.Equal(t, guide.Z, commands[i].Z)
    }
    arm := commands[len(guides)]
    require.Equal(t, state.CommandCursorWalk, arm.Kind)
    require.Equal(t, int32(gludioX), arm.X)
    require.Equal(t, int32(gludioY), arm.Y)
    require.Equal(t, int32(gludioZ), arm.Z)
    probe := commands[len(guides)+1]
    require.Equal(t, state.CommandClickWalk, probe.Kind)
    require.Less(t, math.Abs(float64(probe.X-gludioX)),
        float64(desyncProbeOffset)+1.0,
        "the probe aim sits one probe offset from the arrival point")
}
