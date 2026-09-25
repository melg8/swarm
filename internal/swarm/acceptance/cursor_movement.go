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

// The cursor movement round: the keyboard movement channel of the
// Mobius C1 server (MoveToLocation mode 0, the arrow keys of the
// official client) turns the session into a cursor key session
// (MoveToLocation.runImpl latches player.setCursorKeyMovement) and
// from that moment EVERY ValidatePosition claim is synced straight
// into the world (ValidatePosition.runImpl calls setSyncedXYZ and
// broadcasts the placement) - no speed validation, no distance
// validation, the client position stream IS the server position. The
// official client drives the channel at the honest run speed of its
// own movement simulation; a bot that streams the claims faster
// moves faster, and the server follows. The scenario pins the abuse
// contract: the claims ride fixed 94 unit hops (UNDER the 125 unit
// move speed band the desync branch of the handler would need, so
// only the cursor key branch can adopt them - the drift proves the
// cursor channel, not the desync correction), the echoed placements
// (the server broadcasts every adopted claim back to the session
// itself) carry the character across a long distance at several
// times the run speed and the logout store keeps the final placement.
const (
    // cursorAccount is the temp account of the scenario.
    cursorAccount = "temp20"
    // cursorHopDistance is the distance between two consecutive
    // claims: 94 units, deliberately under the 125 move speed band
    // (the run speed of the elven fighter) so the desync branch of
    // ValidatePosition.runImpl never fires - every adopted claim is
    // the cursor key branch alone (the same claim without the cursor
    // flag would be ignored outright).
    cursorHopDistance = 94
    // cursorHopCount is the total claim count of the stream: 60 hops
    // cover 5640 units, a long distance ride.
    cursorHopCount = 60
    // cursorCycleClaims streams this many claims per cycle (the
    // command queue drains once per hunt loop tick, 250 ms, so the
    // cycle bundles a few claims and keeps the queue shallow).
    cursorCycleClaims = 3
    // cursorCyclePause paces one stream cycle: the claims leave on
    // the next loop tick, the echoes arrive within the pause.
    cursorCyclePause = 350 * time.Millisecond
    // cursorArmWait lets the keyboard mode arm settle before the
    // first claim (the origin adoption and the AI move intention of
    // the server side handler).
    cursorArmWait = 500 * time.Millisecond
    // cursorEchoTolerance accepts an echoed placement this far from
    // the latest claim of a cycle: the server side AI keeps walking
    // the armed target between the claims (the run speed advance of
    // one cycle) and a sample may land one claim behind.
    cursorEchoTolerance = 150.0
    // The pass gates: a majority of the stream cycles confirmed by
    // their echoes, an effective speed of at least twice the server
    // reported run speed, a long distance displacement and the store
    // keeping the final placement.
    cursorMinCycles     = 14
    cursorSpeedFactor   = 2.0
    cursorMinDistance   = 3000.0
    cursorStoredTol     = 300.0
    cursorScenarioID    = "cursor-movement"
    cursorScenarioTitle = "cursor movement · the keyboard stream ride"
    cursorTimeout       = 5 * time.Minute
)

// The check ids of the cursor movement scenario.
const (
    checkCursorStream = "stream"
    checkCursorSpeed  = "speed"
    checkCursorDist   = "distance"
    checkCursorStored = "stored"
)

// cursorHopX returns the claimed X of hop index i (1 based): the
// creation spawn shifted i hop lengths west.
func cursorHopX(i int) int32 {
    return elvenSpawnX - int32(i)*cursorHopDistance
}

// cursorFinalX is the last claim of the stream and the walk target of
// the keyboard mode arm.
func cursorFinalX() int32 {
    return cursorHopX(cursorHopCount)
}

// cursorCycleCount is the number of stream cycles.
func cursorCycleCount() int {
    return cursorHopCount / cursorCycleClaims
}

// cursorReset returns the start state of the cursor movement
// scenario: the level 15 fighter wakes at the creation spawn point
// with the standard vitals and an empty bag.
func cursorReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   15,
        Exp:     level15Exp,
        SP:      0,
        Adena:   1000,
        Items:   nil,
        X:       elvenSpawnX,
        Y:       elvenSpawnY,
        Z:       elvenSpawnZ,
        MaxHP:   level15HP,
        MaxMP:   level15MP,
        MaxCP:   level15CP,
    }
}

// cursorChecks builds the initial check list of the cursor movement
// scenario.
func cursorChecks() []Check {
    return []Check{
        {
            ID:     checkOnline,
            Label:  labelEnteredWorld,
            Done:   false,
            Detail: "",
        },
        {
            ID:     checkCursorStream,
            Label:  "the cursor stream moved the server position",
            Done:   false,
            Detail: "the stream has not started",
        },
        {
            ID:     checkCursorSpeed,
            Label:  labelRouteSpeed,
            Done:   false,
            Detail: "the ride has not started",
        },
        {
            ID:     checkCursorDist,
            Label:  "covered a long distance",
            Done:   false,
            Detail: "standing at the spawn",
        },
        {
            ID:     checkCursorStored,
            Label:  "the logout store kept the ridden placement",
            Done:   false,
            Detail: "the session is still running",
        },
    }
}

// startCursorSession launches the manual only bot session of the
// cursor movement scenario.
func startCursorSession(
    ctx context.Context, m *Manager, test *Test,
) (context.CancelFunc, chan error) {
    return startAbuseSession(ctx, m, test, cursorAccount)
}

// pushCursorArm queues the keyboard mode arm: the cursor key walk to
// the end of the ride line.
func pushCursorArm(tracker *state.Bot) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandCursorWalk,
        ObjectID: 0,
        Count:    0,
        X:        cursorFinalX(),
        Y:        elvenSpawnY,
        Z:        elvenSpawnZ,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// pushCursorClaims queues one stream cycle: the claims of the hops
// from..to (1 based, inclusive).
func pushCursorClaims(tracker *state.Bot, from, to int) {
    for hop := from; hop <= to; hop++ {
        tracker.PushCommand(state.Command{
            Kind:     state.CommandClaimPosition,
            ObjectID: 0,
            Count:    0,
            X:        cursorHopX(hop),
            Y:        elvenSpawnY,
            Z:        elvenSpawnZ,
            Text:     "",
            Target:   "",
            Channel:  0,
        })
    }
}

// pushCursorDisarm queues the disarm click: a raw mouse-mode walk
// past the final claim that returns the session to the click
// movement (a mouse-mode MoveToLocation clears the server side
// cursor key flag).
func pushCursorDisarm(tracker *state.Bot) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandClickWalk,
        ObjectID: 0,
        Count:    0,
        X:        cursorFinalX() - desyncProbeOffset,
        Y:        elvenSpawnY,
        Z:        elvenSpawnZ,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// armCursorMode queues the keyboard mode arm and waits out its
// settle window: the server side origin adoption and the AI move
// intention of the arm.
func armCursorMode(ctx context.Context, tracker *state.Bot) error {
    pushCursorArm(tracker)
    select {
    case <-ctx.Done():
        return fmt.Errorf("cancelled: %w", ctx.Err())
    case <-time.After(cursorArmWait):
    }

    return nil
}

// runCursorRide streams the claim cycles and evaluates the ride
// checks: the movement half of the scenario between the keyboard mode
// arm and the disarm click.
func runCursorRide(
    ctx context.Context, tracker *state.Bot, test *Test,
) (confirmed int, lastX int32, speed, distance float64) {
    streamConfirmed, streamX, lastAt, startedAt := runCursorStream(
        ctx, tracker, test)
    confirmed = streamConfirmed
    lastX = streamX
    speed = desyncEffectiveSpeed(lastX, startedAt, lastAt)
    distance = math.Abs(float64(elvenSpawnX - lastX))
    test.appendLog("acceptance: the claim stream finished - " +
        strconv.Itoa(confirmed) + " of " +
        strconv.Itoa(cursorCycleCount()) + " cycles confirmed, " +
        strconv.FormatFloat(speed, 'f', 0, 64) +
        " units per second effective, " +
        strconv.FormatFloat(distance, 'f', 0, 64) + " units covered")

    return confirmed, lastX, speed, distance
}

// cursorScenario runs the cursor movement round: the temp character
// wakes at the creation spawn, the keyboard mode arm turns the
// session into a cursor key session and the claim stream rides the
// server position along the line west at several times the run speed
// - every claim is under the move speed band, so only the cursor key
// branch of the server handler can adopt it. Pass: a majority of the
// stream cycles confirmed by their echoed placements, the effective
// speed at least twice the run speed, a displacement of 3000+ units
// and the logout store keeping the ridden placement.
func cursorScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(cursorChecks())

    if err := prepareAbuseCharacter(m, test, cursorReset(
        cursorAccount)); err != nil {
        return err
    }

    cancelSession, sessionDone := startCursorSession(ctx, m, test)
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
        " units per second, the keyboard mode arm follows")

    if err := armCursorMode(ctx, tracker); err != nil {
        cancelSession()
        <-sessionDone

        return err
    }

    confirmed, _, speed, distance := runCursorRide(
        ctx, tracker, test)
    evaluateCursorChecks(test, confirmed, speed, distance, runSpeed)

    pushCursorDisarm(tracker)
    select {
    case <-ctx.Done():
    case <-time.After(desyncProbeWait):
    }
    if err := finishAbuseSession(cancelSession, sessionDone,
        test); err != nil {
        return err
    }
    storedX, storedY, storedZ, err := readCursorStoredRow(m, test)
    if err != nil {
        return err
    }
    evaluateCursorStored(test, storedX, storedY, storedZ)

    if !allChecksDone(test) {
        return fmt.Errorf("the cursor ride failed its checks: "+
            "%d of %d cycles confirmed, %.0f units per second "+
            "effective against the %.0f run speed, %.0f units "+
            "covered, stored at %d %d %d",
            confirmed, cursorCycleCount(), speed, runSpeed, distance,
            storedX, storedY, storedZ)
    }

    return nil
}

// readCursorStoredRow reads the stored placement of the cursor ride
// and narrates it against the final claim.
func readCursorStoredRow(
    m *Manager, test *Test,
) (int32, int32, int32, error) {
    storedX, storedY, storedZ, err := m.readStoredPosition(
        cursorReset(cursorAccount), test)
    if err != nil {
        return 0, 0, 0, fmt.Errorf("read the stored position: %w", err)
    }
    test.appendLog("acceptance: the character row stores " +
        strconv.Itoa(int(storedX)) + " " + strconv.Itoa(int(storedY)) +
        " " + strconv.Itoa(int(storedZ)) + " (the final claim was " +
        strconv.Itoa(int(cursorFinalX())) + " " +
        strconv.Itoa(int(elvenSpawnY)) + " " +
        strconv.Itoa(int(elvenSpawnZ)) + ")")

    return storedX, storedY, storedZ, nil
}

// runCursorStream streams the claim cycles: every cycle queues its
// bundle of claims, waits the cycle pause (the hunt loop tick drains
// the queue, the server adopts and broadcasts every claim) and
// samples the echoed placement against the latest claim. It returns
// the confirmed cycle count, the last sampled X, its moment and the
// moment the first claim left.
func runCursorStream(
    ctx context.Context, tracker *state.Bot, test *Test,
) (confirmed int, lastX int32, lastAt, startedAt time.Time) {
    lastX = elvenSpawnX
    for cycle := 1; cycle <= cursorCycleCount(); cycle++ {
        if ctx.Err() != nil {
            return confirmed, lastX, lastAt, startedAt
        }
        first := (cycle-1)*cursorCycleClaims + 1
        last := cycle * cursorCycleClaims
        pushCursorClaims(tracker, first, last)
        if startedAt.IsZero() {
            startedAt = time.Now()
        }
        select {
        case <-ctx.Done():
            return confirmed, lastX, lastAt, startedAt
        case <-time.After(cursorCyclePause):
        }
        x, _, _, ok := tracker.SelfPosition()
        if !ok {
            continue
        }
        claimX := cursorHopX(last)
        echo := math.Abs(float64(x - claimX))
        if echo <= cursorEchoTolerance {
            confirmed++
            lastX = x
            lastAt = time.Now()
        }
        test.updateCheck(checkCursorStream,
            confirmed >= cursorMinCycles,
            "cycle "+strconv.Itoa(cycle)+" of "+
                strconv.Itoa(cursorCycleCount())+
                ": the echo landed "+strconv.Itoa(int(echo))+
                " units from claim "+strconv.Itoa(last)+" ("+
                strconv.Itoa(confirmed)+" confirmed)")
    }
    test.appendLog("acceptance: the stream rode " +
        strconv.Itoa(cursorCycleCount()) + " cycles of " +
        strconv.Itoa(cursorCycleClaims) + " claims, " +
        strconv.Itoa(confirmed) + " confirmed by their echoes")

    return confirmed, lastX, lastAt, startedAt
}

// evaluateCursorChecks rewrites the stream, speed and distance checks
// from the ride results.
func evaluateCursorChecks(
    test *Test, confirmed int, speed, distance, runSpeed float64,
) {
    test.updateCheck(checkCursorStream, confirmed >= cursorMinCycles,
        strconv.Itoa(confirmed)+" of "+
            strconv.Itoa(cursorCycleCount())+
            " cycles confirmed by their echoes")
    test.updateCheck(checkCursorSpeed, speed >= cursorSpeedFactor*runSpeed,
        strconv.FormatFloat(speed, 'f', 0, 64)+
            " units per second against the run speed "+
            strconv.FormatFloat(runSpeed, 'f', 0, 64))
    test.updateCheck(checkCursorDist, distance >= cursorMinDistance,
        strconv.FormatFloat(distance, 'f', 0, 64)+
            " units from the spawn")
}

// evaluateCursorStored rewrites the store check: the character row
// keeps the ridden placement of the final claim.
func evaluateCursorStored(test *Test, storedX, storedY, storedZ int32) {
    drift := math.Hypot(
        float64(storedX-cursorFinalX()),
        float64(storedY-elvenSpawnY))
    ok := drift <= cursorStoredTol &&
        abs32(storedZ-elvenSpawnZ) <= cursorStoredTol
    test.updateCheck(checkCursorStored, ok,
        "stored "+strconv.Itoa(int(storedX))+" "+
            strconv.Itoa(int(storedY))+" "+
            strconv.Itoa(int(storedZ))+", "+
            strconv.Itoa(int(drift))+" units from the final claim")
}
