// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The stuck points scenario (the 2026-09-20 fleet freeze report): the
// characters wake on the two terrace spots the owner reported the
// fleet frozen on - 43632 50560 -2960 and 41920 52128 -3000 - and
// cannot build a route anywhere from them. Both spots stand on link
// components the strict edge only mesh link graph cannot leave (52
// and 29 polygons, the 640x752 and the 592x432 boxes) while the grid
// engine plans out of the very same cells through the diagonal
// squeezes the server movement channels allow. The components
// outgrew the old 320 pocket side, so the pocket escape declined and
// the route attempts answered the bare not found (or the partial
// that walks to the component's inner boundary and strands it there)
// - the bots stood frozen. The widened escape bound plus the
// horizontal displacement floor of the exit scan answer the walk out;
// the scenarios pin the contract: from each reported position the bot
// builds a route and leaves the strand on its own.
const (
    // stuckPointAX/Y/Z is the first reported spot (heading 21963).
    stuckPointAX = 43632
    stuckPointAY = 50560
    stuckPointAZ = -2960
    // stuckPointBX/Y/Z is the second reported spot (heading 26712).
    stuckPointBX = 41920
    stuckPointBY = 52128
    stuckPointBZ = -3000
    // stuckEscapeRadius is the escape ring both scenarios accept: the
    // mesh escape aims measure 276 and 311 units out (the march past
    // the arrival slack), a character 256+ units from the spawn has
    // walked the escape ray onto the connected ground the followup
    // plans route from.
    stuckEscapeRadius = 256.0
    stuckPointWindow  = 2 * time.Minute
    // stuckPointTimeout bounds one stuck point scenario: the escape
    // window plus the character injection, the login handshake and
    // the graceful shutdown (the railing pocket budget shape).
    stuckPointTimeout = 5 * time.Minute
)

// The stuck point scenario accounts (the temp ladder tail).
const (
    stuckAccountA  = "temp15"
    stuckPasswordA = "temp15"
    stuckAccountB  = "temp16"
    stuckPasswordB = "temp16"
)

// stuckPointReset returns the start state of a stuck point scenario:
// the reported terrace spot and the same level 15 fighter shape the
// village escape scenario injects (the strands catch any character
// the same way, the dump state keeps the scenarios comparable).
func stuckPointReset(account string, x, y, z int32) characterReset {
    reset := villageEscapeReset(account)
    reset.X = x
    reset.Y = y
    reset.Z = z

    return reset
}

// runStuckPointScenario drives one stuck point round: the temp
// character wakes on the reported terrace spot and must walk out of
// it on its own within the two minute window - the report held the
// fleet on these spots forever (every route attempt answered the
// bare not found or the inner boundary partial before the fix).
func runStuckPointScenario(
    ctx context.Context, m *Manager, t *Test,
    account, password string, x, y, z int32,
) error {
    test := t
    test.setChecks(stuckPointChecks())

    if err := m.ensureCharacter(account, password, account,
        test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    reset := stuckPointReset(account, x, y, z)
    if err := m.injectReset(reset, test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, account, password,
            account, m.proxy, test.appendLog)
        sessionDone <- nil
    }()
    defer func() {
        cancelSession()
        <-sessionDone
    }()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        return err
    }

    onlineSince := time.Now()
    test.appendLog("acceptance: the bot is in the world at the stuck " +
        "terrace spot, watching the escape window")

    for {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        escaped, detail := evaluateStuckPoint(tracker, test, x, y)
        if escaped {
            test.appendLog("acceptance: the character left the stuck " +
                "terrace in " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + ")")

            return nil
        }
        if time.Since(onlineSince) > stuckPointWindow {
            return errors.New("the character stood inside the stuck " +
                "terrace for " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + ") - the escape window closed")
        }
        select {
        case <-ctx.Done():
            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(monitorPeriod):
        }
    }
}

// evaluateStuckPoint rewrites the check list of the stuck point
// scenario from the live tracker state: the escape check holds once
// the character stands beyond the escape radius from the reported
// spot.
func evaluateStuckPoint(
    tracker *state.Bot, test *Test, x, y int32,
) (bool, string) {
    if tracker.Status() != state.StatusOnline {
        return false, detailOffline
    }
    px, py, _, ok := tracker.SelfPosition()
    if !ok {
        return false, detailNoPosition
    }
    dist := math.Hypot(
        float64(px-x), float64(py-y))
    detail := fmt.Sprintf("%.0f units from the stuck terrace", dist)
    test.updateCheck(checkStuckEscape, dist >= stuckEscapeRadius, detail)

    return dist >= stuckEscapeRadius, detail
}

// stuckPointChecks builds the initial check list of the stuck point
// scenarios.
func stuckPointChecks() []Check {
    return []Check{{
        ID:     checkStuckEscape,
        Label:  "left the stuck terrace within the two minute window",
        Done:   false,
        Detail: "standing at the stuck terrace spot",
    }}
}

// stuckPointAScenario runs the first reported spot round.
func stuckPointAScenario(ctx context.Context, m *Manager, t *Test) error {
    return runStuckPointScenario(ctx, m, t, stuckAccountA, stuckPasswordA,
        stuckPointAX, stuckPointAY, stuckPointAZ)
}

// stuckPointBScenario runs the second reported spot round.
func stuckPointBScenario(ctx context.Context, m *Manager, t *Test) error {
    return runStuckPointScenario(ctx, m, t, stuckAccountB, stuckPasswordB,
        stuckPointBX, stuckPointBY, stuckPointBZ)
}
