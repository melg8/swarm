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

// The railing pocket scenario (the 2026-09-19 route report): the
// character wakes at 43736 47048 -2992 - the elven village deck cell
// just off the railings - and cannot build a route anywhere from it.
// The cell itself is honest ground: the server walks clicks in and
// out of it, the grid engine plans out of it, but the mesh link graph
// connects polygons across shared edges only and every axis neighbor
// of the cell carries a railing wall bit - the cell is a linkless one
// polygon island whose only way out is the diagonal squeeze the
// server's anti corner cut rule allows. The mesh pocket escape
// answers the walk out and the plan cycles route from the connected
// ground; the scenario pins the contract the user demanded: from this
// position the bot builds a route and leaves the pocket on its own.
const (
    // railingPocketSpawnX/Y/Z is the reported pocket cell (the deck
    // cell off the village railings the report named).
    railingPocketSpawnX = 43736
    railingPocketSpawnY = 47048
    railingPocketSpawnZ = -2992
    // The escape radius: the pocket cell plus its railing ring
    // measure under 100 units across, the mesh escape aim lands 200+
    // units out, the followup plans route from there - a character
    // 256 units from the spawn stands on the connected deck ground
    // the planner routes from.
    railingPocketRadius = 256.0
    railingPocketWindow = 2 * time.Minute
)

// railingPocketTimeout bounds the pocket scenario: the escape window
// itself is the two minutes of the village escape contract (the walk
// out of the pocket rides the same machinery), the budget adds the
// character injection, the login handshake and the graceful shutdown.
const railingPocketTimeout = 5 * time.Minute

// railingPocketReset returns the start state of the pocket scenario:
// the reported pocket cell and the same level 15 fighter shape the
// village escape scenario injects (the pocket strands any character
// the same way, the dump state keeps the two scenarios comparable).
func railingPocketReset(account string) characterReset {
    reset := villageEscapeReset(account)
    reset.X = railingPocketSpawnX
    reset.Y = railingPocketSpawnY
    reset.Z = railingPocketSpawnZ

    return reset
}

// railingPocketScenario runs the railing pocket round: the temp
// character wakes inside the linkless deck cell and must walk out of
// it on its own within the two minute window - the report held the
// character there forever (every plan attempt answered the bare not
// found before the pocket escape, the return held without a single
// walk request).
func railingPocketScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(railingPocketChecks())

    if err := m.ensureCharacter(pocketAccount, pocketPassword,
        pocketAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    reset := railingPocketReset(pocketAccount)
    if err := m.injectReset(reset, test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, pocketAccount, pocketPassword,
            pocketAccount, m.proxy, test.appendLog)
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
    test.appendLog("acceptance: the bot is in the world at the railing " +
        "pocket cell, watching the escape window")

    for {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        escaped, detail := evaluateRailingPocket(tracker, test)
        if escaped {
            test.appendLog("acceptance: the character left the railing " +
                "pocket in " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + ")")

            return nil
        }
        if time.Since(onlineSince) > railingPocketWindow {
            return errors.New("the character stood inside the railing " +
                "pocket for " +
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

// evaluateRailingPocket rewrites the check list of the pocket
// scenario from the live tracker state: the escape check holds once
// the character stands beyond the pocket radius from the spawn cell.
func evaluateRailingPocket(
    tracker *state.Bot, test *Test,
) (bool, string) {
    if tracker.Status() != state.StatusOnline {
        return false, "offline"
    }
    x, y, _, ok := tracker.SelfPosition()
    if !ok {
        return false, "the position is unknown"
    }
    dist := math.Hypot(
        float64(x-railingPocketSpawnX),
        float64(y-railingPocketSpawnY))
    detail := fmt.Sprintf("%.0f units from the railing pocket", dist)
    test.updateCheck(checkPocketEscape, dist >= railingPocketRadius, detail)

    return dist >= railingPocketRadius, detail
}

// railingPocketChecks builds the initial check list of the pocket
// scenario.
func railingPocketChecks() []Check {
    return []Check{{
        ID:     checkPocketEscape,
        Label:  "left the railing pocket within the two minute window",
        Done:   false,
        Detail: "standing at the railing pocket cell",
    }}
}
