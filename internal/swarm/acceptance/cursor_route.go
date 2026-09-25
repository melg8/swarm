// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "fmt"
    "math"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The cursor route round: the level 19 character with the million
// adena wallet wakes at the elven forest creation spawn, the mesh
// navigator plans the corridor to the Gludio arrival point and the
// cursor key claim stream rides the PATHFINDING guides - the keyboard
// mode arm latches the session cursor key flag (MoveToLocation mode
// 0 of the official client arrow keys) and every following
// ValidatePosition claim is synced straight into the world and
// broadcast back (setSyncedXYZ, no speed and no distance validation),
// so the interpolated 94 unit steps - every step UNDER the move speed
// band the desync branch of the handler would need, only the cursor
// key branch can adopt them - carry the character the whole corridor
// at several times the run speed while the echoed placements stream
// back from the server itself.
//
//nolint:funlen // the scenario walks its phases in order
func cursorRouteScenario(
    ctx context.Context, m *Manager, t *Test,
) error {
    test := t
    test.setChecks(routeChecks(
        "the cursor stream rode the corridor guides"))

    if err := prepareAbuseCharacter(m, test, routeReset(
        cursorRouteAccount)); err != nil {
        return err
    }

    cancelSession, sessionDone := startAbuseSession(ctx, m, test,
        cursorRouteAccount)
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
    steps := cursorRouteSteps(route)
    streamDistance := routeDistanceXY(steps)
    test.updateCheck(checkRoutePlan, true,
        strconv.Itoa(len(steps))+" stream claims over "+
            strconv.FormatFloat(streamDistance, 'f', 0, 64)+
            " units of corridor")
    test.appendLog("acceptance: the corridor stream holds " +
        strconv.Itoa(len(steps)) + " claims over " +
        strconv.FormatFloat(streamDistance, 'f', 0, 64) +
        " units, the keyboard mode arm follows")

    if err := armCursorRouteMode(ctx, tracker); err != nil {
        cancelSession()
        <-sessionDone

        return err
    }

    moved, confirmed, lastX, lastY, lastAt, startedAt :=
        runCursorRouteStream(ctx, tracker, test, steps)
    speed := routeEffectiveSpeed(lastX, lastY, startedAt, lastAt)
    evaluateRouteRide(test,
        strconv.Itoa(confirmed)+" of "+
            strconv.Itoa(cursorRouteCycles(steps))+
            " stream cycles confirmed by their echoes",
        moved, speed, streamDistance, runSpeed)
    evaluateRouteArrival(test, lastX, lastY)
    test.appendLog("acceptance: the corridor stream finished - " +
        strconv.Itoa(confirmed) + " of " +
        strconv.Itoa(cursorRouteCycles(steps)) + " cycles confirmed, " +
        strconv.FormatFloat(speed, 'f', 0, 64) +
        " units per second effective, " +
        strconv.FormatFloat(streamDistance, 'f', 0, 64) +
        " units of corridor")

    pushRouteProbe(tracker, steps[len(steps)-1])
    select {
    case <-ctx.Done():
    case <-time.After(routeProbeWait):
    }
    if err := finishAbuseSession(cancelSession, sessionDone,
        test); err != nil {
        return err
    }
    storedX, storedY, storedZ, err := readRouteStoredRow(m, test,
        cursorRouteAccount)
    if err != nil {
        return err
    }
    evaluateRouteStored(test, storedX, storedY, storedZ)

    if !allChecksDone(test) {
        return routeScenarioFailed("cursor route", speed,
            streamDistance, runSpeed, storedX, storedY, storedZ)
    }

    return nil
}

// cursorRouteCycles is the cycle count of a route stream: the claims
// bundle in cursorCycleClaims per cycle the way the short round
// paces them.
func cursorRouteCycles(steps []guidePoint) int {
    return (len(steps) + cursorCycleClaims - 1) / cursorCycleClaims
}

// armCursorRouteMode queues the keyboard mode arm of the route ride
// (the cursor key walk to the Gludio arrival point) and waits out its
// settle window.
func armCursorRouteMode(
    ctx context.Context, tracker *state.Bot,
) error {
    pushRouteArm(tracker)
    select {
    case <-ctx.Done():
        return fmt.Errorf("cancelled: %w", ctx.Err())
    case <-time.After(cursorArmWait):
    }

    return nil
}

// runCursorRouteStream streams the interpolated corridor claims in
// cycles: every cycle queues its bundle of claims, the cycle pause
// covers the hunt loop tick and the server adoption, and the echoed
// placement (the server broadcasts every adopted claim of an armed
// cursor key session back to the session itself) is sampled against
// the latest claim. It returns whether the stream moved at all, the
// confirmed cycle count and the last sampled placement with its
// moment and the moment the first claim left.
func runCursorRouteStream(
    ctx context.Context, tracker *state.Bot, test *Test,
    steps []guidePoint,
) (moved bool, confirmed int, lastX, lastY int32,
    lastAt, startedAt time.Time,
) {
    for cycle := 1; cycle <= cursorRouteCycles(steps); cycle++ {
        if ctx.Err() != nil {
            return moved, confirmed, lastX, lastY, lastAt, startedAt
        }
        first := (cycle - 1) * cursorCycleClaims
        last := cycle * cursorCycleClaims
        if last > len(steps) {
            last = len(steps)
        }
        for _, step := range steps[first:last] {
            pushRouteClaim(tracker, step)
        }
        if startedAt.IsZero() {
            startedAt = time.Now()
            moved = true
        }
        select {
        case <-ctx.Done():
            return moved, confirmed, lastX, lastY, lastAt, startedAt
        case <-time.After(routeClaimPause):
        }
        claim := steps[last-1]
        x, y, _, ok := tracker.SelfPosition()
        if !ok {
            continue
        }
        echo := math.Hypot(float64(x-claim.X), float64(y-claim.Y))
        if echo <= cursorEchoTolerance {
            confirmed++
            lastX, lastY = x, y
            lastAt = time.Now()
        }
        test.updateCheck(checkRouteMove,
            confirmed > 0,
            "cycle "+strconv.Itoa(cycle)+" of "+
                strconv.Itoa(cursorRouteCycles(steps))+
                ": the echo landed "+
                strconv.Itoa(int(echo))+" units from claim "+
                strconv.Itoa(last)+" ("+
                strconv.Itoa(confirmed)+" confirmed)")
    }
    test.appendLog("acceptance: the stream rode " +
        strconv.Itoa(cursorRouteCycles(steps)) + " cycles of " +
        strconv.Itoa(len(steps)) + " claims, " +
        strconv.Itoa(confirmed) + " confirmed by their echoes")

    return moved, confirmed, lastX, lastY, lastAt, startedAt
}
