// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "math"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The long route movement abuse round: the elven forest creation
// spawn sits 100,000+ world units from the Gludio castle town across
// the Neutral Zone corridor - a walk the server would move the
// character at the run speed of the template (144 units per second,
// a twelve minute hike through the aggressive spawns of the corridor).
// The two movement abuse channels of the short rounds cover the same
// trip in a fraction of the time: the desync teleport drift hops the
// server side placement along the PATHFINDING guides (the navmesh
// route the fleet bot itself plans - the funnel waypoints of the
// corridor search), and the cursor key stream rides the same guides
// in fixed under-the-band hops. The scenarios pin the abuse contract
// at the world scale: the level 19 character with the million adena
// wallet starts at the forest spawn, the mesh navigator plans the
// route to the Gludio teleporter arrival point, the claims carry the
// server position along the guide ladder and the arrival check reads
// the Gludio placement back - the same packet channels the short
// rounds proved, now over a route measured in minutes of honest
// walking.
const (
    // The three temp accounts of the route rounds (the next free
    // numbers past the accounts the other scenarios own).
    desyncRouteAccount = "temp21"
    cursorRouteAccount = "temp22"
    fastRouteAccount   = "temp23"
    // The Gludio arrival point: the teleporter arrival coordinate of
    // the Mobius C1 data (dist/game/data/teleporters/town, the same
    // point the navmesh town route corpus binds its Gludio segments
    // to).
    gludioX = -12787
    gludioY = 122779
    gludioZ = -3114
    // The wallet of the route rounds: the user's one million adena.
    routeAdena = 1_000_000
    // The level of the route rounds and its start experience, SP and
    // vitals (the C1 experience table level 19 bottom, the elven
    // fighter lvlUpgainData 339 hp / 137 mp / 135 cp).
    routeLevel = 19
    level19Exp = 675590
    level19HP  = 339
    level19MP  = 137
    level19CP  = 135
    // routeStraightDistance is the direct spawn to Gludio distance
    // (about 100.5k units); the planned corridor runs longer.
    routeStraightDistance = 100_538.64
    // routeMinDistance is the pass gate of the covered distance: the
    // corridor answer must beat the straight line by its detours but
    // never fall short of it.
    routeMinDistance = 100_000.0
    // routeSpeedFactor is the pass gate of the effective speed
    // against the server reported run speed.
    routeSpeedFactor = 2.0
    // routeArriveTolerance accepts the final placement this far from
    // the Gludio arrival point: the final claim lands on the point
    // itself, the arrival probe walk may advance the character its
    // collision offset past it.
    routeArriveTolerance = 400.0
    // routeStoredTolerance accepts the stored database placement this
    // far from the Gludio arrival point in x and y.
    routeStoredTolerance = 500.0
    // routeStoredZTolerance accepts the stored z this far from the
    // claimed one: the desync branch of ValidatePosition.runImpl
    // keeps the higher server z whenever the claim descends (no
    // geodata loaded - GeoEngine.getHeight echoes the reference z),
    // so a downhill arrival stores the highest z the ladder ever
    // claimed.
    routeStoredZTolerance = 1500.0
    // routePlanTimeout bounds one mesh route query and the whole
    // segmented planning loop of the scenario prologue.
    routePlanTimeout = 4 * time.Minute
    // routePlanIterations caps the segmented planning: one full
    // answer of the hierarchical search or a dozen partial corridors.
    routePlanIterations = 12
    // routeMeshCapacity is the tile cache capacity of the planning
    // window: the corridor crosses nine regions and the production
    // default cache (four tiles) would evict the regions the search
    // still needs, so the prologue lifts the bound for the planning
    // and restores the production default after (the measured
    // corridor answers in under a second at this capacity, about
    // eleven seconds cold at the default).
    routeMeshCapacity = 12
    // routeGuideStride is the minimum distance between two desync
    // guide claims: the stride clears the move speed band of the
    // desync branch (the run speed 144) and the 500..600 unit
    // correction band of ValidatePosition.runImpl (diffSq < 360000)
    // whose ValidateLocation answer would only confuse the echo
    // picture - a claim inside the band is skipped, the guide beyond
    // it re-syncs the ladder.
    routeGuideStride = 700.0
    // routeProbeEvery probes the server position every this many
    // desync guide claims (the probe click costs its settle window,
    // a mid route ladder probes a dozen times, not once per guide).
    routeProbeEvery = 8
    // The command pacing of the rounds: the hunt loop tick drains the
    // queue every 250 ms and the queue itself holds 32 commands
    // before it drops the oldest, so a batch stays small enough that
    // even a missed tick never overflows it (two queued batches of
    // 16 fill the queue exactly) and the pause covers a tick.
    routeClaimPause  = 350 * time.Millisecond
    routeProbeWait   = 950 * time.Millisecond
    routeBatchClaims = 16
    routeBatchPause  = 350 * time.Millisecond
    // The shared check labels and initial details of the route
    // rounds (the goconst pin of the shared check list).
    labelRouteSpeed  = "moved faster than twice the run speed"
    labelRouteDist   = "covered the long corridor distance"
    labelRouteStored = "the logout store kept the Gludio placement"
    detailPlanIdle   = "the planning has not started"
    detailRideIdle   = "the ride has not started"
    detailDistIdle   = "standing at the forest spawn"
    detailStoredLive = "the session is still running"
    // The shared check ids of the route rounds.
    checkRoutePlan   = "plan"
    checkRouteMove   = "move"
    checkRouteSpeed  = "speed"
    checkRouteDist   = "distance"
    checkRouteArrive = "arrival"
    checkRouteStored = "stored"
)

// The scenario ids and titles of the route rounds.
const (
    desyncRouteID    = "desync-route"
    desyncRouteTitle = "desync route · the elf forest to Gludio drift"
    cursorRouteID    = "cursor-route"
    cursorRouteTitle = "cursor route · the elf forest to Gludio stream"
    fastRouteID      = "fast-route"
    fastRouteTitle   = "fast route · the elf forest to Gludio sprint"
)

// The scenario timeouts: the planning prologue (a cold hierarchical
// search answers the corridor in about eleven seconds, a thrashing
// tile cache stretches the segmented round), the movement itself (the
// desync ladder about a minute of paced claims, the cursor stream two
// minutes, the sprint a couple of seconds) and the session epilogue.
const (
    desyncRouteTimeout = 10 * time.Minute
    cursorRouteTimeout = 12 * time.Minute
    fastRouteTimeout   = 8 * time.Minute
)

// routeReset returns the start state of a route round: the level 19
// elven fighter with the one million adena wallet wakes at the elven
// forest creation spawn point - the same cell a freshly created
// character would stand on, three levels and a fortune richer.
func routeReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   routeLevel,
        Exp:     level19Exp,
        SP:      0,
        Adena:   routeAdena,
        Items:   nil,
        X:       elvenSpawnX,
        Y:       elvenSpawnY,
        Z:       elvenSpawnZ,
        MaxHP:   level19HP,
        MaxMP:   level19MP,
        MaxCP:   level19CP,
    }
}

// routeChecks builds the initial check list of a route round.
func routeChecks(moveLabel string) []Check {
    return []Check{
        {
            ID:     checkOnline,
            Label:  labelEnteredWorld,
            Done:   false,
            Detail: "",
        },
        {
            ID:     checkRoutePlan,
            Label:  "the mesh navigator planned the route to Gludio",
            Done:   false,
            Detail: detailPlanIdle,
        },
        {
            ID:     checkRouteMove,
            Label:  moveLabel,
            Done:   false,
            Detail: detailRideIdle,
        },
        {
            ID:     checkRouteSpeed,
            Label:  "moved faster than twice the run speed",
            Done:   false,
            Detail: detailRideIdle,
        },
        {
            ID:     checkRouteDist,
            Label:  labelRouteDist,
            Done:   false,
            Detail: detailDistIdle,
        },
        {
            ID:     checkRouteArrive,
            Label:  "arrived at the Gludio arrival point",
            Done:   false,
            Detail: "the corridor still lies ahead",
        },
        {
            ID:     checkRouteStored,
            Label:  labelRouteStored,
            Done:   false,
            Detail: detailStoredLive,
        },
    }
}

// guidePoint is one claimed placement of a route ladder.
type guidePoint struct {
    X int32
    Y int32
    Z int32
}

// planAbuseRoute plans the walk from the elven forest spawn to the
// Gludio arrival point through the mesh navigator the fleet bot
// serves (the sole route planner of the live integration): one
// hierarchical corridor answer when the search binds end to end, the
// segmented round otherwise - walk the partial corridor the mesh
// hands back, re-plan from its end, repeat. The planning runs inside
// the scenario prologue (a cold corridor search answers in about
// eleven seconds, the segmented round stays under the bound) and
// returns the guide waypoints the claims ride.
//
//nolint:funlen // the segmented planning walks its iterations in order
func planAbuseRoute(
    ctx context.Context, m *Manager, test *Test,
) ([]guidePoint, error) {
    if m.engine == nil || m.mesh == nil {
        return nil, errors.New("the route rounds need the navmesh " +
            "tiles (build them with cmd/navmesh-build, " +
            "docs/navmesh.md)")
    }
    navigator := hunt.NewNavmeshNavigator(m.engine, m.mesh)
    start := pathfind.Vec3{
        X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ,
    }
    end := pathfind.Vec3{X: gludioX, Y: gludioY, Z: gludioZ}

    // The planning window lifts the tile cache bound over the nine
    // regions of the corridor (see routeMeshCapacity) and restores
    // the production default after - no other planning runs between
    // the two calls.
    m.mesh.SetCacheCapacity(routeMeshCapacity)
    defer m.mesh.SetCacheCapacity(navmesh.DefaultMeshCapacity)

    planCtx, cancel := context.WithTimeout(ctx, routePlanTimeout)
    defer cancel()

    guides := make([]guidePoint, 0, 64)
    current := start
    began := time.Now()
    for iteration := 1; iteration <= routePlanIterations; iteration++ {
        if planCtx.Err() != nil {
            return nil, fmt.Errorf("the route planning window ended "+
                "after %d iterations: %w", iteration-1, planCtx.Err())
        }
        result, err := navigator.FindPathApproach(current, end, 0)
        if err != nil {
            return nil, fmt.Errorf("the mesh route query failed: %w",
                err)
        }
        if result.Found && len(result.Waypoints) > 0 {
            guides = appendRouteWaypoints(guides, result.Waypoints)
            test.appendLog("acceptance: the mesh navigator planned " +
                "the corridor - iteration " + strconv.Itoa(iteration) +
                ", " + strconv.Itoa(len(result.Waypoints)) +
                " waypoints, " +
                strconv.FormatFloat(result.Length, 'f', 0, 64) +
                " units, " + time.Since(began).Round(
                time.Millisecond).String() + " of planning")

            return guides, nil
        }
        if !result.Partial || len(result.Waypoints) < 2 {
            return nil, fmt.Errorf("the mesh navigator stranded at "+
                "%d %d %d after %d iterations (no corridor, no "+
                "partial)", int32(current.X), int32(current.Y),
                int32(current.Z), iteration)
        }
        waypoints := result.Waypoints
        last := waypoints[len(waypoints)-1]
        step := math.Hypot(last.X-current.X, last.Y-current.Y)
        if step < 1000 {
            return nil, fmt.Errorf("the mesh corridor stalled %.0f "+
                "units short of Gludio after %d iterations", step,
                iteration)
        }
        guides = appendRouteWaypoints(guides, waypoints)
        test.appendLog("acceptance: the corridor search answered a " +
            "partial - iteration " + strconv.Itoa(iteration) + ", " +
            strconv.Itoa(len(waypoints)) + " waypoints, the replan " +
            "continues from " + strconv.Itoa(int(last.X)) + " " +
            strconv.Itoa(int(last.Y)))
        current = last
    }

    return nil, fmt.Errorf("the segmented planning never reached "+
        "Gludio within %d iterations", routePlanIterations)
}

// appendRouteWaypoints appends the funnel waypoints of one planning
// answer onto the guide ladder, skipping the waypoints that sit on
// the ladder end already (the segmented round re-plans from the last
// waypoint of the previous answer).
func appendRouteWaypoints(
    guides []guidePoint, waypoints []pathfind.Vec3,
) []guidePoint {
    for _, wp := range waypoints {
        if len(guides) > 0 {
            last := guides[len(guides)-1]
            if int32(wp.X) == last.X && int32(wp.Y) == last.Y {
                continue
            }
        }
        guides = append(guides, guidePoint{
            X: int32(wp.X), Y: int32(wp.Y), Z: int32(wp.Z),
        })
    }

    return guides
}

// desyncRouteGuides collapses the planned corridor into the desync
// guide ladder: the claims hop guide to guide, but a claim inside the
// stride of the previous one never fires the desync branch of the
// server handler (it lands in the correction band instead), so the
// ladder keeps one claim per stride and always closes on the Gludio
// arrival point.
func desyncRouteGuides(route []guidePoint) []guidePoint {
    guides := make([]guidePoint, 0, len(route)/2+2)
    var lastX, lastY float64
    seeded := false
    for _, point := range route {
        if seeded {
            step := math.Hypot(float64(point.X)-lastX,
                float64(point.Y)-lastY)
            if step < routeGuideStride && point != route[len(route)-1] {
                continue
            }
        }
        guides = append(guides, point)
        lastX, lastY = float64(point.X), float64(point.Y)
        seeded = true
    }
    if len(guides) == 0 || guides[len(guides)-1] != route[len(route)-1] {
        guides = append(guides, route[len(route)-1])
    }

    return guides
}

// cursorRouteSteps interpolates the planned corridor into the cursor
// claim stream: fixed hops under the move speed band of the server
// handler (the run speed 144, the hop 94) along the corridor polyline
// - only the cursor key branch of ValidatePosition.runImpl can adopt
// them, the same claim without the armed cursor flag would be ignored
// outright. The final step lands exactly on the Gludio arrival point.
func cursorRouteSteps(route []guidePoint) []guidePoint {
    steps := make([]guidePoint, 0, 1100)
    from := guidePoint{X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ}
    for _, to := range route {
        segmentX := float64(to.X - from.X)
        segmentY := float64(to.Y - from.Y)
        length := math.Hypot(segmentX, segmentY)
        if length <= cursorHopDistance {
            steps = append(steps, to)
            from = to

            continue
        }
        hops := int(length / cursorHopDistance)
        for hop := 1; hop <= hops; hop++ {
            fraction := float64(hop*cursorHopDistance) / length
            steps = append(steps, guidePoint{
                X: from.X + int32(segmentX*fraction),
                Y: from.Y + int32(segmentY*fraction),
                Z: from.Z + int32(float64(to.Z-from.Z)*fraction),
            })
        }
        steps = append(steps, to)
        from = to
    }

    return steps
}

// pushRouteClaim queues one claimed placement of a route ladder.
func pushRouteClaim(tracker *state.Bot, point guidePoint) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandClaimPosition,
        ObjectID: 0,
        Count:    0,
        X:        point.X,
        Y:        point.Y,
        Z:        point.Z,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// pushRouteArm queues the keyboard mode arm of a cursor route round:
// the cursor key walk whose target is the Gludio arrival point.
func pushRouteArm(tracker *state.Bot) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandCursorWalk,
        ObjectID: 0,
        Count:    0,
        X:        gludioX,
        Y:        gludioY,
        Z:        gludioZ,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// pushRouteProbe queues the probe click of a route round: a short raw
// mouse-mode walk toward the Gludio point whose movement echo reports
// the server position back to the session.
func pushRouteProbe(tracker *state.Bot, at guidePoint) {
    dx := float64(gludioX - at.X)
    dy := float64(gludioY - at.Y)
    length := math.Hypot(dx, dy)
    if length < 1 {
        dx, dy, length = -1, 0, 1
    }
    tracker.PushCommand(state.Command{
        Kind:     state.CommandClickWalk,
        ObjectID: 0,
        Count:    0,
        X:        at.X + int32(dx/length*desyncProbeOffset),
        Y:        at.Y + int32(dy/length*desyncProbeOffset),
        Z:        at.Z,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// routeDistanceXY measures the walked distance of a guide ladder in
// the x/y plane (the z of the desync channel drifts monotonically -
// the server keeps the higher z of a descending claim - so the plane
// carries the geometry).
func routeDistanceXY(guides []guidePoint) float64 {
    total := 0.0
    from := guidePoint{X: elvenSpawnX, Y: elvenSpawnY, Z: elvenSpawnZ}
    for _, point := range guides {
        total += math.Hypot(float64(point.X-from.X),
            float64(point.Y-from.Y))
        from = point
    }

    return total
}

// routeEffectiveSpeed computes the effective speed of a ride: the
// displacement from the spawn to the last sampled placement over the
// elapsed claim window.
func routeEffectiveSpeed(
    lastX, lastY int32, startedAt, lastAt time.Time,
) float64 {
    if startedAt.IsZero() || lastAt.IsZero() ||
        lastAt.Before(startedAt) {
        return 0
    }
    elapsed := lastAt.Sub(startedAt).Seconds()
    if elapsed <= 0 {
        return 0
    }
    displacement := math.Hypot(
        float64(lastX-elvenSpawnX), float64(lastY-elvenSpawnY))

    return displacement / elapsed
}

// evaluateRouteRide rewrites the movement, speed and distance checks
// from the ride results.
func evaluateRouteRide(
    test *Test, moveDetail string, moved bool,
    speed, distance, runSpeed float64,
) {
    test.updateCheck(checkRouteMove, moved, moveDetail)
    test.updateCheck(checkRouteSpeed, speed >= routeSpeedFactor*runSpeed,
        strconv.FormatFloat(speed, 'f', 0, 64)+
            " units per second against the run speed "+
            strconv.FormatFloat(runSpeed, 'f', 0, 64))
    test.updateCheck(checkRouteDist, distance >= routeMinDistance,
        strconv.FormatFloat(distance, 'f', 0, 64)+
            " units of corridor covered")
}

// evaluateRouteArrival rewrites the arrival check: the last sampled
// placement sits within the arrival tolerance of the Gludio point.
func evaluateRouteArrival(test *Test, x, y int32) {
    drift := math.Hypot(float64(x-gludioX), float64(y-gludioY))
    test.updateCheck(checkRouteArrive, drift <= routeArriveTolerance,
        "the placement "+strconv.Itoa(int(x))+" "+
            strconv.Itoa(int(y))+" sits "+
            strconv.Itoa(int(drift))+
            " units from the Gludio arrival point")
}

// evaluateRouteStored rewrites the store check: the character row
// keeps the Gludio placement of the final claim (the z band tolerates
// the monotonic z drift of the desync branch).
func evaluateRouteStored(test *Test, storedX, storedY, storedZ int32) {
    drift := math.Hypot(
        float64(storedX-gludioX), float64(storedY-gludioY))
    ok := drift <= routeStoredTolerance &&
        abs32(storedZ-gludioZ) <= routeStoredZTolerance
    test.updateCheck(checkRouteStored, ok,
        "stored "+strconv.Itoa(int(storedX))+" "+
            strconv.Itoa(int(storedY))+" "+
            strconv.Itoa(int(storedZ))+", "+
            strconv.Itoa(int(drift))+
            " units from the Gludio arrival point")
}

// readRouteStoredRow reads the stored placement of a route round and
// narrates it against the Gludio arrival point.
func readRouteStoredRow(
    m *Manager, test *Test, account string,
) (int32, int32, int32, error) {
    storedX, storedY, storedZ, err := m.readStoredPosition(
        routeReset(account), test)
    if err != nil {
        return 0, 0, 0, fmt.Errorf("read the stored position: %w", err)
    }
    test.appendLog("acceptance: the character row stores " +
        strconv.Itoa(int(storedX)) + " " + strconv.Itoa(int(storedY)) +
        " " + strconv.Itoa(int(storedZ)) + " (the Gludio arrival " +
        "point is " + strconv.Itoa(gludioX) + " " +
        strconv.Itoa(gludioY) + " " + strconv.Itoa(gludioZ) + ")")

    return storedX, storedY, storedZ, nil
}

// routeScenarioFailed reports the failed route ride with its numbers.
func routeScenarioFailed(
    kind string, speed, distance, runSpeed float64,
    storedX, storedY, storedZ int32,
) error {
    return fmt.Errorf("the %s ride failed its checks: %.0f units "+
        "per second effective against the %.0f run speed, %.0f "+
        "units covered, stored at %d %d %d",
        kind, speed, runSpeed, distance, storedX, storedY, storedZ)
}
