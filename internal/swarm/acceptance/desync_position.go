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

// The desync validate position round: the "out of sync" correction of
// the Mobius C1 ValidatePosition handler (ValidatePosition.runImpl)
// resolves every claimed placement whose 3D distance to the server
// position exceeds the move speed (the run speed of the character,
// 125 units per second for the elven fighter) by TRUSTING the client
// - player.setXYZ(claim) - with no upper distance bound, no geodata
// check and no door check on the jump. A session that never walks a
// single click hops its server side placement anywhere on the map at
// the pace of its position claims: eight hops of 700 units, one hop
// per claim, cover 5600 units in about ten seconds - more than four
// times the run speed the server itself would move the character at.
// The scenario pins the abuse contract: from the creation spawn the
// claimed hops carry the server position across a long distance, the
// probe clicks read the adopted placements back through the self
// MoveToLocation echo and the logout store keeps the final placement
// in the character row.
const (
    // desyncAccount is the temp account of the scenario (the first
    // free number past the accounts the other scenarios own).
    desyncAccount = "temp19"
    // The claim ladder: every hop claims the spawn cell shifted one
    // hop length west (the X axis, the creation spawn line). A hop
    // of 700 units clears the move speed band with room (the desync
    // branch needs more than 125) and skips the 500..600 unit
    // correction band of the handler (diffSq < 360000) whose
    // ValidateLocation answer would only confuse the echo picture.
    desyncHopDistance = 700
    desyncHopCount    = 8
    // The probe click rides desyncProbeOffset units past the claim
    // (further west): the fresh server side walk broadcasts its
    // MoveToLocation echo back to the clicking client itself, and the
    // echo origin reports the server position of the moment - the
    // adopted claim, when the desync branch took it.
    desyncProbeOffset = 60
    // The hop pacing: the claim lands, the probe follows a quarter
    // second later (the command queue latency of the hunt loop tick
    // plus the server task), the echo arrives within the probe
    // window (the first movement broadcast of a fresh walk is
    // immediate, the throttled ones run once per second).
    desyncClaimPause = 350 * time.Millisecond
    desyncProbeWait  = 950 * time.Millisecond
    // desyncEchoTolerance accepts an echo origin this far from the
    // claimed hop: the probe walk itself advances the character up to
    // its collision offset while the echo travels, and a walk in
    // progress snaps the origin a few units past the claim.
    desyncEchoTolerance = 250.0
    // desyncStoredTolerance accepts the stored database placement
    // this far from the final claim: the logout store rounds nothing,
    // but the last probe walk may advance the character its offset
    // past the final claim before the session ends.
    desyncStoredTolerance = 300.0
    // The pass gates: a majority of the hops confirmed by their echo
    // origins, an effective speed of at least twice the server
    // reported run speed and a long distance displacement.
    desyncMinHops       = 7
    desyncSpeedFactor   = 2.0
    desyncMinDistance   = 3000.0
    desyncScenarioID    = "desync-position"
    desyncScenarioTitle = "desync position · the validation teleport drift"
    desyncTimeout       = 5 * time.Minute
)

// The check ids of the desync scenario.
const (
    checkDesyncHops     = "hops"
    checkDesyncSpeed    = "speed"
    checkDesyncDistance = "distance"
    checkDesyncStored   = "stored"
)

// desyncHopX returns the claimed X of hop index i (1 based): the
// creation spawn shifted i hop lengths west.
func desyncHopX(i int) int32 {
    return elvenSpawnX - int32(i)*desyncHopDistance
}

// desyncFinalX is the last claim of the ladder.
func desyncFinalX() int32 {
    return desyncHopX(desyncHopCount)
}

// desyncReset returns the start state of the desync scenario: the
// level 15 fighter wakes at the creation spawn point of the elven
// village with the standard vitals and an empty bag - the scenario
// needs no gear, the claims never walk a step.
func desyncReset(account string) characterReset {
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

// desyncChecks builds the initial check list of the desync scenario.
func desyncChecks() []Check {
    return []Check{
        {
            ID:     checkOnline,
            Label:  labelEnteredWorld,
            Done:   false,
            Detail: "",
        },
        {
            ID:     checkDesyncHops,
            Label:  "the server adopted the claimed hops",
            Done:   false,
            Detail: "no hop confirmed yet",
        },
        {
            ID:     checkDesyncSpeed,
            Label:  labelRouteSpeed,
            Done:   false,
            Detail: "the drift has not started",
        },
        {
            ID:     checkDesyncDistance,
            Label:  "covered a long distance",
            Done:   false,
            Detail: "standing at the spawn",
        },
        {
            ID:     checkDesyncStored,
            Label:  "the logout store kept the drifted placement",
            Done:   false,
            Detail: "the session is still running",
        },
    }
}

// startDesyncSession launches the manual only bot session of the
// desync scenario: the hunt loop consumes the claim and probe
// commands and never arms its own trips, so the position claims are
// the only movement the session speaks.
func startDesyncSession(
    ctx context.Context, m *Manager, test *Test,
) (context.CancelFunc, chan error) {
    return startAbuseSession(ctx, m, test, desyncAccount)
}

// desyncRunSpeed reads the server reported run speed of the character
// (the UserInfo of the world entry): the reference the speed check
// multiplies.
func desyncRunSpeed(tracker *state.Bot) float64 {
    speed := tracker.SelfRunSpeed()
    if speed <= 0 {
        return 1
    }

    return speed
}

// pushDesyncClaim queues one claimed placement hop.
func pushDesyncClaim(tracker *state.Bot, x int32) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandClaimPosition,
        ObjectID: 0,
        Count:    0,
        X:        x,
        Y:        elvenSpawnY,
        Z:        elvenSpawnZ,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// pushDesyncProbe queues the probe click of one hop: a short raw
// mouse-mode walk past the claim whose movement echo reports the
// server position back to the session.
func pushDesyncProbe(tracker *state.Bot, claimX int32) {
    tracker.PushCommand(state.Command{
        Kind:     state.CommandClickWalk,
        ObjectID: 0,
        Count:    0,
        X:        claimX - desyncProbeOffset,
        Y:        elvenSpawnY,
        Z:        elvenSpawnZ,
        Text:     "",
        Target:   "",
        Channel:  0,
    })
}

// prepareAbuseCharacter ensures the temp character of a movement
// abuse scenario exists and lands the injected start state: the
// shared prologue of the two rounds (the character create, the
// server flush pause, the database reset, the settle pause).
func prepareAbuseCharacter(
    m *Manager, test *Test, reset characterReset,
) error {
    if err := m.ensureCharacter(reset.Account, reset.Account,
        reset.Account, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(reset, test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    return nil
}

// startAbuseSession launches the manual only bot session of a
// movement abuse scenario: the hunt loop consumes the claim, cursor
// and click commands and never arms its own trips, so the position
// claims are the only movement the session speaks.
func startAbuseSession(
    ctx context.Context, m *Manager, test *Test, account string,
) (context.CancelFunc, chan error) {
    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        sessionDone <- m.runSession(sessionCtx, account,
            account, account, false, m.proxy, test.appendLog)
    }()

    return cancelSession, sessionDone
}

// finishAbuseSession stops the abuse session and drains its end: the
// graceful logout path every abuse scenario shares.
func finishAbuseSession(
    cancelSession context.CancelFunc, sessionDone chan error,
    test *Test,
) error {
    test.appendLog("acceptance: stopping the bot, the store check " +
        "follows")
    cancelSession()
    if err := <-sessionDone; err != nil {
        return fmt.Errorf("session end: %w", err)
    }
    test.appendLog("acceptance: the bot left the world gracefully")

    return nil
}

// desyncScenario runs the desync validate position round: the temp
// character wakes at the creation spawn, the claim ladder hops the
// server side position 700 units west per claim (every claim beyond
// the move speed band of the server handler is adopted by the desync
// correction with no distance cap) and the probe clicks read the
// adopted placements back. Pass: a majority of the hops confirmed by
// their echo origins, the effective speed at least twice the run
// speed, a displacement of 3000+ units and the logout store keeping
// the final placement.
func desyncScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(desyncChecks())

    if err := prepareAbuseCharacter(m, test, desyncReset(
        desyncAccount)); err != nil {
        return err
    }

    cancelSession, sessionDone := startDesyncSession(ctx, m, test)
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
        " units per second, the drift ladder starts")

    confirmed, lastX, lastAt, startedAt := runDesyncLadder(
        ctx, tracker, test)

    speed := desyncEffectiveSpeed(lastX, startedAt, lastAt)
    distance := math.Abs(float64(elvenSpawnX - lastX))
    evaluateDesyncChecks(test, confirmed, speed, distance, runSpeed)
    test.appendLog("acceptance: the drift ladder finished - " +
        strconv.Itoa(confirmed) + " of " +
        strconv.Itoa(desyncHopCount) + " hops confirmed, " +
        strconv.FormatFloat(speed, 'f', 0, 64) +
        " units per second effective, " +
        strconv.FormatFloat(distance, 'f', 0, 64) + " units covered")

    if err := finishAbuseSession(cancelSession, sessionDone,
        test); err != nil {
        return err
    }
    storedX, storedY, storedZ, err := m.readStoredPosition(
        desyncReset(desyncAccount), test)
    if err != nil {
        return fmt.Errorf("read the stored position: %w", err)
    }
    test.appendLog("acceptance: the character row stores " +
        strconv.Itoa(int(storedX)) + " " + strconv.Itoa(int(storedY)) +
        " " + strconv.Itoa(int(storedZ)) + " (the final claim was " +
        strconv.Itoa(int(desyncFinalX())) + " " +
        strconv.Itoa(int(elvenSpawnY)) + " " +
        strconv.Itoa(int(elvenSpawnZ)) + ")")
    evaluateDesyncStored(test, storedX, storedY, storedZ)

    if !allChecksDone(test) {
        return fmt.Errorf("the desync drift failed its checks: "+
            "%d of %d hops confirmed, %.0f units per second "+
            "effective against the %.0f run speed, %.0f units "+
            "covered, stored at %d %d %d",
            confirmed, desyncHopCount, speed, runSpeed, distance,
            storedX, storedY, storedZ)
    }

    return nil
}

// runDesyncLadder walks the claim ladder: every hop claims its
// placement, waits the claim pause (the command queue latency of the
// hunt loop tick plus the server task), probes the server position
// with a raw click and samples the echoed placement. It returns the
// confirmed hop count, the last sampled X, its moment and the moment
// the first claim left.
func runDesyncLadder(
    ctx context.Context, tracker *state.Bot, test *Test,
) (confirmed int, lastX int32, lastAt, startedAt time.Time) {
    lastX = elvenSpawnX
    for hop := 1; hop <= desyncHopCount; hop++ {
        if ctx.Err() != nil {
            return confirmed, lastX, lastAt, startedAt
        }
        claimX := desyncHopX(hop)
        pushDesyncClaim(tracker, claimX)
        if startedAt.IsZero() {
            startedAt = time.Now()
        }
        select {
        case <-ctx.Done():
            return confirmed, lastX, lastAt, startedAt
        case <-time.After(desyncClaimPause):
        }
        pushDesyncProbe(tracker, claimX)
        select {
        case <-ctx.Done():
            return confirmed, lastX, lastAt, startedAt
        case <-time.After(desyncProbeWait):
        }
        x, _, _, ok := tracker.SelfPosition()
        if !ok {
            continue
        }
        echo := math.Abs(float64(x - claimX))
        if echo <= desyncEchoTolerance {
            confirmed++
            lastX = x
            lastAt = time.Now()
        }
        test.updateCheck(checkDesyncHops,
            confirmed >= desyncMinHops,
            "hop "+strconv.Itoa(hop)+" of "+strconv.Itoa(desyncHopCount)+
                ": the echo landed "+strconv.Itoa(int(echo))+
                " units from the claim ("+
                strconv.Itoa(confirmed)+" confirmed)")
        test.appendLog("acceptance: hop " + strconv.Itoa(hop) +
            " claimed " + strconv.Itoa(int(claimX)) + ", the echo " +
            strconv.Itoa(int(x)) + " (" +
            strconv.Itoa(int(echo)) + " off, " +
            strconv.Itoa(confirmed) + " of " +
            strconv.Itoa(hop) + " confirmed)")
    }

    return confirmed, lastX, lastAt, startedAt
}

// desyncEffectiveSpeed computes the effective drift speed: the
// displacement from the spawn to the last confirmed echo over the
// elapsed claim window.
func desyncEffectiveSpeed(
    lastX int32, startedAt, lastAt time.Time,
) float64 {
    if startedAt.IsZero() || lastAt.IsZero() ||
        lastAt.Before(startedAt) {
        return 0
    }
    elapsed := lastAt.Sub(startedAt).Seconds()
    if elapsed <= 0 {
        return 0
    }

    return math.Abs(float64(elvenSpawnX-lastX)) / elapsed
}

// evaluateDesyncChecks rewrites the hop, speed and distance checks
// from the ladder results.
func evaluateDesyncChecks(
    test *Test, confirmed int, speed, distance, runSpeed float64,
) {
    test.updateCheck(checkDesyncHops, confirmed >= desyncMinHops,
        strconv.Itoa(confirmed)+" of "+strconv.Itoa(desyncHopCount)+
            " hops confirmed by their echoes")
    test.updateCheck(checkDesyncSpeed, speed >= desyncSpeedFactor*runSpeed,
        strconv.FormatFloat(speed, 'f', 0, 64)+
            " units per second against the run speed "+
            strconv.FormatFloat(runSpeed, 'f', 0, 64))
    test.updateCheck(checkDesyncDistance, distance >= desyncMinDistance,
        strconv.FormatFloat(distance, 'f', 0, 64)+
            " units from the spawn")
}

// evaluateDesyncStored rewrites the store check: the character row
// keeps the drifted placement of the final claim.
func evaluateDesyncStored(test *Test, storedX, storedY, storedZ int32) {
    drift := math.Hypot(
        float64(storedX-desyncFinalX()),
        float64(storedY-elvenSpawnY))
    ok := drift <= desyncStoredTolerance &&
        abs32(storedZ-elvenSpawnZ) <= desyncStoredTolerance
    test.updateCheck(checkDesyncStored, ok,
        "stored "+strconv.Itoa(int(storedX))+" "+
            strconv.Itoa(int(storedY))+" "+
            strconv.Itoa(int(storedZ))+", "+
            strconv.Itoa(int(drift))+" units from the final claim")
}
