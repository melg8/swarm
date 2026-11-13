// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The plaza Herbiel walk round: the micro reproduction of the plaza ping-pong
// report (the follower alternated two clicks 63 units apart while the server
// delivered both, then the frozen corridor ban sealed the plaza for the
// session). The character wakes ON the report's plaza cell in the farm
// readiness state, so the weapon run trip arms at once and walks the report
// legs - the sells at the stand, the gear buys, then the walk to Herbiel.
// Pass: the character leaves the plaza band, crosses the western corridor and
// arrives within the interaction distance of Herbiel's spawn.
const (
    // plazaHerbielX/Y/Z is the report's plaza cell: the Unoren customer stand
    // in the live server frame of the village deck.
    plazaHerbielX = 44584
    plazaHerbielY = 46944
    plazaHerbielZ = -2920
    // herbielNpcX/Y is the spawn of the grocery trader Herbiel (the
    // npc the interaction distance check reads): the walk's aim.
    herbielNpcX = 42766
    herbielNpcY = 50037
    // corridorHerbielX/Y is the western corridor sample the planned route
    // crosses; the mid-route progress check reads the distance to it.
    corridorHerbielX = 43520
    corridorHerbielY = 47312
    // plazaHerbielLeft is the distance from the plaza cell beyond
    // which the character counts as having left the ping-pong band.
    plazaHerbielLeft = 200.0
    // corridorHerbielSeen is the distance from the corridor sample
    // within which the walk counts as crossing the western corridor.
    corridorHerbielSeen = 300.0
    // herbielInteraction is the interaction distance ring the arrival check
    // accepts (the stand sits across the counter from the spawn).
    herbielInteraction = 250.0
    // plazaHerbielTimeout bounds the round: the walk covers ~3 km of
    // village corridor plus the buy transaction windows of the two
    // stops before it, the bound keeps the slow live pacing inside.
    plazaHerbielTimeout = 8 * time.Minute
)

// plazaHerbielAccount is the temp account of the scenario (the first
// free number past the sixteen the other scenarios own).
const (
    plazaHerbielAccount  = "temp17"
    plazaHerbielPassword = "temp17"
)

// The check ids of the scenario.
const (
    checkPlazaLeft     = "left-plaza"
    checkCorridorCross = "corridor"
    checkHerbielArrive = "herbiel"
)

// plazaHerbielReset returns the start state: the farm readiness shape
// positioned ON the report's plaza cell, no walk-in needed.
func plazaHerbielReset(account string) characterReset {
    reset := farmReadinessReset(account)
    reset.X = plazaHerbielX
    reset.Y = plazaHerbielY
    reset.Z = plazaHerbielZ

    return reset
}

func plazaHerbielChecks() []Check {
    return []Check{
        {
            ID:     checkPlazaLeft,
            Label:  "the walk left the plaza ping-pong band",
            Done:   false,
            Detail: "standing at the plaza cell",
        },
        {
            ID:     checkCorridorCross,
            Label:  "the walk crosses the western village corridor",
            Done:   false,
            Detail: "the corridor ahead",
        },
        {
            ID:     checkHerbielArrive,
            Label:  "the character stands by the trader Herbiel",
            Done:   false,
            Detail: "far from the trader Herbiel",
        },
    }
}

// plazaHerbielScenario runs the round: the temp character wakes on the plaza
// cell in the farm readiness state, the weapon run trip arms at once and the
// stop ladder walks the report's legs; the pass contract is the check list of
// plazaHerbielChecks.
func plazaHerbielScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(plazaHerbielChecks())

    if err := m.ensureCharacter(plazaHerbielAccount,
        plazaHerbielPassword, plazaHerbielAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(plazaHerbielReset(plazaHerbielAccount),
        test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, plazaHerbielAccount,
            plazaHerbielPassword, plazaHerbielAccount, m.proxy,
            test.appendLog)
        sessionDone <- nil
    }()
    defer func() {
        cancelSession()
        joinSessionDone(sessionDone)
    }()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        return err
    }

    onlineSince := time.Now()
    test.appendLog("acceptance: the bot is in the world at the plaza " +
        "cell of the report, watching the walk to Herbiel")

    for {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        done, detail := evaluatePlazaHerbiel(tracker, test)
        if done {
            test.appendLog("acceptance: the walk reached the Herbiel " +
                "interaction distance in " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + ")")

            return nil
        }
        if time.Since(onlineSince) > plazaHerbielTimeout {
            return fmt.Errorf("the walk to Herbiel did not complete in %s"+
                " (%s) - the ping-pong freeze family",
                time.Since(onlineSince).Round(time.Second), detail)
        }
        select {
        case <-ctx.Done():
            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(monitorPeriod):
        }
    }
}

// evaluatePlazaHerbiel rewrites the check list from the live tracker state and
// reports whether every check holds.
func evaluatePlazaHerbiel(
    tracker *state.Bot, test *Test,
) (bool, string) {
    if tracker.Status() != state.StatusOnline {
        return false, detailOffline
    }
    x, y, _, ok := tracker.SelfPosition()
    if !ok {
        return false, detailNoPosition
    }
    plazaDist := math.Hypot(
        float64(x-plazaHerbielX), float64(y-plazaHerbielY))
    test.updateCheck(checkPlazaLeft, plazaDist > plazaHerbielLeft,
        fmt.Sprintf("%.0f units from the plaza cell", plazaDist))

    corridorDist := math.Hypot(
        float64(x-corridorHerbielX), float64(y-corridorHerbielY))
    test.updateCheck(checkCorridorCross,
        corridorDist <= corridorHerbielSeen,
        fmt.Sprintf("%.0f units from the corridor crossing",
            corridorDist))

    herbielDist := math.Hypot(
        float64(x-herbielNpcX), float64(y-herbielNpcY))
    test.updateCheck(checkHerbielArrive,
        herbielDist <= herbielInteraction,
        fmt.Sprintf("%.0f units from the trader Herbiel", herbielDist))
    if allChecksDone(test) {
        return true, fmt.Sprintf("%.0f units from Herbiel",
            herbielDist)
    }

    return false, fmt.Sprintf("%.0f units from the plaza, %.0f from "+
        "Herbiel", plazaDist, herbielDist)
}
