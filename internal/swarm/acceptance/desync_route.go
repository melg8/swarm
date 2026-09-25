// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "math"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The desync route round: the level 19 character with the million
// adena wallet wakes at the elven forest creation spawn, the mesh
// navigator plans the corridor to the Gludio arrival point and the
// desync claim ladder hops the server side placement along the
// PATHFINDING guides - every claim beyond the move speed band of the
// server handler is adopted by the out of sync correction of
// ValidatePosition.runImpl (player.setXYZ, no distance cap, no
// geodata and no door check), so the paced claims carry the character
// the whole corridor (a twelve minute honest run) in about a minute
// while the probe clicks read the adopted placements back through the
// self MoveToLocation echoes.
//
//nolint:funlen // the scenario walks its phases in order
func desyncRouteScenario(
    ctx context.Context, m *Manager, t *Test,
) error {
    test := t
    test.setChecks(routeChecks(
        "the desync ladder hopped the corridor guides"))

    if err := prepareAbuseCharacter(m, test, routeReset(
        desyncRouteAccount)); err != nil {
        return err
    }

    cancelSession, sessionDone := startAbuseSession(ctx, m, test,
        desyncRouteAccount)
    defer cancelSession()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        cancelSession()
        <-sessionDone

        return err
    }
    runSpeed := desyncRunSpeed(tracker)
    test.appendLog("acceptance: the run speed of the character is " +
        strconv.FormatFloat(runSpeed, 'f', 0, 64) +
        " units per second, the corridor planning starts")

    route, err := planAbuseRoute(ctx, m, test)
    if err != nil {
        cancelSession()
        <-sessionDone

        return err
    }
    guides := desyncRouteGuides(route)
    ladderDistance := routeDistanceXY(guides)
    test.updateCheck(checkRoutePlan, true,
        strconv.Itoa(len(guides))+" guide claims over "+
            strconv.FormatFloat(ladderDistance, 'f', 0, 64)+
            " units of corridor")
    test.appendLog("acceptance: the corridor ladder holds " +
        strconv.Itoa(len(guides)) + " guide claims over " +
        strconv.FormatFloat(ladderDistance, 'f', 0, 64) +
        " units, the drift starts")

    moved, confirmed, lastX, lastY, lastAt, startedAt :=
        runDesyncRouteLadder(ctx, tracker, test, guides)
    speed := routeEffectiveSpeed(lastX, lastY, startedAt, lastAt)
    evaluateRouteRide(test,
        strconv.Itoa(confirmed)+" of "+strconv.Itoa(len(guides))+
            " guide claims confirmed by their echoes",
        moved, speed, ladderDistance, runSpeed)
    evaluateRouteArrival(test, lastX, lastY)
    test.appendLog("acceptance: the corridor drift finished - " +
        strconv.Itoa(confirmed) + " of " +
        strconv.Itoa(len(guides)) + " guides confirmed, " +
        strconv.FormatFloat(speed, 'f', 0, 64) +
        " units per second effective, " +
        strconv.FormatFloat(ladderDistance, 'f', 0, 64) +
        " units of corridor")

    if err := finishAbuseSession(cancelSession, sessionDone,
        test); err != nil {
        return err
    }
    storedX, storedY, storedZ, err := readRouteStoredRow(m, test,
        desyncRouteAccount)
    if err != nil {
        return err
    }
    evaluateRouteStored(test, storedX, storedY, storedZ)

    if !allChecksDone(test) {
        return routeScenarioFailed("desync route", speed,
            ladderDistance, runSpeed, storedX, storedY, storedZ)
    }

    return nil
}

// runDesyncRouteLadder walks the desync guide ladder of the corridor:
// every guide claims its placement, the claim pause covers the hunt
// loop tick and the server task, and every routeProbeEvery guides a
// probe click reads the adopted placement back through its echo. It
// returns whether the ladder moved at all, the confirmed probe count
// and the last sampled placement with its moment and the moment the
// first claim left.
func runDesyncRouteLadder(
    ctx context.Context, tracker *state.Bot, test *Test,
    guides []guidePoint,
) (moved bool, confirmed int, lastX, lastY int32,
    lastAt, startedAt time.Time,
) {
    for i, guide := range guides {
        if ctx.Err() != nil {
            return moved, confirmed, lastX, lastY, lastAt, startedAt
        }
        pushRouteClaim(tracker, guide)
        if startedAt.IsZero() {
            startedAt = time.Now()
            moved = true
        }
        select {
        case <-ctx.Done():
            return moved, confirmed, lastX, lastY, lastAt, startedAt
        case <-time.After(routeClaimPause):
        }
        if (i+1)%routeProbeEvery != 0 && i != len(guides)-1 {
            continue
        }
        pushRouteProbe(tracker, guide)
        select {
        case <-ctx.Done():
            return moved, confirmed, lastX, lastY, lastAt, startedAt
        case <-time.After(routeProbeWait):
        }
        x, y, _, ok := tracker.SelfPosition()
        if !ok {
            continue
        }
        echo := math.Hypot(float64(x-guide.X), float64(y-guide.Y))
        if echo <= desyncEchoTolerance {
            confirmed++
            lastX, lastY = x, y
            lastAt = time.Now()
        }
        test.updateCheck(checkRouteMove,
            confirmed > 0,
            "guide "+strconv.Itoa(i+1)+" of "+
                strconv.Itoa(len(guides))+": the echo landed "+
                strconv.Itoa(int(echo))+" units from the claim ("+
                strconv.Itoa(confirmed)+" confirmed)")
        test.appendLog("acceptance: guide " + strconv.Itoa(i+1) +
            " claimed " + strconv.Itoa(int(guide.X)) + " " +
            strconv.Itoa(int(guide.Y)) + ", the echo " +
            strconv.Itoa(int(x)) + " " + strconv.Itoa(int(y)) +
            " (" + strconv.Itoa(int(echo)) + " off, " +
            strconv.Itoa(confirmed) + " confirmed)")
    }

    return moved, confirmed, lastX, lastY, lastAt, startedAt
}
