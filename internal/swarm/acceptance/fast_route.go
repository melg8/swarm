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

// The fast route round: the same elven forest to Gludio corridor the
// two route rounds ride, covered the fastest way the packet channels
// allow. The desync branch of ValidatePosition.runImpl adopts every
// claim beyond the move speed band with NO distance cap and the
// server holds no flood protector over the ValidatePosition packet,
// so the sprint dumps the whole guide ladder at full queue rate -
// batches of claims limited only by the hunt loop tick that drains
// the command queue (24 claims per batch under the 32 slot drop
// bound, one tick per batch) - and the server adopts the entire
// corridor in a couple of seconds: a hundred thousand units of a
// twelve minute honest run in the time a real character crosses one
// screen. The probe click at the far end reads the adopted Gludio
// placement back and the logout store keeps it.
//
//nolint:funlen // the scenario walks its phases in order
func fastRouteScenario(
    ctx context.Context, m *Manager, t *Test,
) error {
    test := t
    test.setChecks(routeChecks(
        "the sprint dumped the corridor ladder at full rate"))

    if err := prepareAbuseCharacter(m, test, routeReset(
        fastRouteAccount)); err != nil {
        return err
    }

    cancelSession, sessionDone := startAbuseSession(ctx, m, test,
        fastRouteAccount)
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
        " units, the sprint starts")

    moved, lastX, lastY, lastAt, startedAt := runFastRouteSprint(
        ctx, tracker, test, guides)
    speed := routeEffectiveSpeed(lastX, lastY, startedAt, lastAt)
    evaluateRouteRide(test,
        "the whole ladder left in "+strconv.Itoa(
            fastRouteBatches(guides))+" batches of up to "+
            strconv.Itoa(routeBatchClaims)+" claims",
        moved, speed, ladderDistance, runSpeed)
    evaluateRouteArrival(test, lastX, lastY)
    test.appendLog("acceptance: the corridor sprint finished - " +
        strconv.FormatFloat(speed, 'f', 0, 64) +
        " units per second effective, " +
        strconv.FormatFloat(ladderDistance, 'f', 0, 64) +
        " units of corridor, " +
        lastAt.Sub(startedAt).Round(time.Millisecond).String() +
        " of riding")

    if err := finishAbuseSession(cancelSession, sessionDone,
        test); err != nil {
        return err
    }
    storedX, storedY, storedZ, err := readRouteStoredRow(m, test,
        fastRouteAccount)
    if err != nil {
        return err
    }
    evaluateRouteStored(test, storedX, storedY, storedZ)

    if !allChecksDone(test) {
        return routeScenarioFailed("fast route", speed,
            ladderDistance, runSpeed, storedX, storedY, storedZ)
    }

    return nil
}

// fastRouteBatches is the batch count of a sprint: the ladder rides
// routeBatchClaims claims per hunt loop tick.
func fastRouteBatches(guides []guidePoint) int {
    return (len(guides) + routeBatchClaims - 1) / routeBatchClaims
}

// runFastRouteSprint dumps the whole guide ladder at full queue
// rate: every batch queues up to routeBatchClaims claims (under the
// 32 slot drop bound of the command queue) and the batch pause
// covers one hunt loop tick that drains them, so the server adopts
// the entire corridor a stride per packet with no pacing between the
// claims. The probe click at the far end reads the adopted placement
// back. It returns whether the sprint moved at all and the last
// sampled placement with its moment and the moment the first claim
// left.
func runFastRouteSprint(
    ctx context.Context, tracker *state.Bot, test *Test,
    guides []guidePoint,
) (moved bool, lastX, lastY int32, lastAt, startedAt time.Time) {
    for batch := range fastRouteBatches(guides) {
        if ctx.Err() != nil {
            return moved, lastX, lastY, lastAt, startedAt
        }
        first := batch * routeBatchClaims
        last := first + routeBatchClaims
        if last > len(guides) {
            last = len(guides)
        }
        for _, guide := range guides[first:last] {
            pushRouteClaim(tracker, guide)
        }
        if startedAt.IsZero() {
            startedAt = time.Now()
            moved = true
        }
        select {
        case <-ctx.Done():
            return moved, lastX, lastY, lastAt, startedAt
        case <-time.After(routeBatchPause):
        }
        test.appendLog("acceptance: batch " + strconv.Itoa(batch+1) +
            " of " + strconv.Itoa(fastRouteBatches(guides)) +
            " left, " + strconv.Itoa(last) + " of " +
            strconv.Itoa(len(guides)) + " guides queued")
    }

    // The arrival probe: one raw click past the final claim whose
    // echo reports the adopted Gludio placement back.
    final := guides[len(guides)-1]
    pushRouteProbe(tracker, final)
    for attempt := 1; attempt <= 4; attempt++ {
        select {
        case <-ctx.Done():
            return moved, lastX, lastY, lastAt, startedAt
        case <-time.After(routeProbeWait):
        }
        x, y, _, ok := tracker.SelfPosition()
        if !ok {
            continue
        }
        echo := math.Hypot(float64(x-final.X), float64(y-final.Y))
        test.appendLog("acceptance: the arrival echo landed " +
            strconv.Itoa(int(x)) + " " + strconv.Itoa(int(y)) +
            " (" + strconv.Itoa(int(echo)) + " units from the " +
            "final claim)")
        if echo <= routeArriveTolerance {
            lastX, lastY = x, y
            lastAt = time.Now()
        }
        if lastAt.After(startedAt) {
            break
        }
    }

    return moved, lastX, lastY, lastAt, startedAt
}
