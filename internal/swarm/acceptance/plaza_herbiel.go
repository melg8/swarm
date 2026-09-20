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

// The plaza Herbiel walk round (the 2026-09-20 17:09 farm readiness
// ping-pong report): the shopping trip of the report walked from the
// shop plaza to the grocery trader Herbiel and the follower
// ping-ponged between the plaza cell A (44584 46944, the Unoren
// customer stand the character traded at) and the first route sample
// 63 units out - nine alternating MoveToLocation clicks while the
// server accepted and delivered every one of them, then the frozen
// corridor verdict banned the plaza for the session and every later
// route search of the run answered from the sealed plaza. The round
// of the fix made the follower's advance gate answer through the same
// server click transport the clicks obey (the follower keeps one
// oracle end to end) and removed the corridor ban system.
//
// The scenario is the micro reproduction of the report: the character
// wakes ON the plaza cell the ping-pong lived on, in the exact start
// state of the farm readiness round (the level 15 elven fighter with
// the 20,000 SP, the 100,000 adena and the empty bag of the fresh
// character), so the weapon run trip arms at once and the stop ladder
// walks the very legs of the report - the sells at the Unoren stand
// the character already stands on, the gear buys, then the walk to
// Herbiel through the village corridor the ping-pong froze in. The
// pass contract pins the three observable halves of the fixed walk:
// the character leaves the plaza band (the ping-pong never left it),
// the walk makes real progress through the western corridor, and the
// character arrives within the interaction distance of Herbiel's
// spawn - the stand the trip aimed at.
const (
    // plazaHerbielX/Y/Z is the plaza cell A of the report: the Unoren
    // customer stand the report's character traded at (the tracker z
    // is the live server frame of the village deck).
    plazaHerbielX = 44584
    plazaHerbielY = 46944
    plazaHerbielZ = -2920
    // herbielNpcX/Y is the spawn of the grocery trader Herbiel (the
    // npc the interaction distance check reads): the walk's aim.
    herbielNpcX = 42766
    herbielNpcY = 50037
    // corridorHerbielX/Y is the western corridor sample the planned
    // route crosses (the third waypoint of the offline repro): the
    // mid-route progress check reads the character's distance to it.
    corridorHerbielX = 43520
    corridorHerbielY = 47312
    // plazaHerbielLeft is the distance from the plaza cell beyond
    // which the character counts as having left the ping-pong band.
    plazaHerbielLeft = 200.0
    // corridorHerbielSeen is the distance from the corridor sample
    // within which the walk counts as crossing the western corridor.
    corridorHerbielSeen = 300.0
    // herbielInteraction is the server interaction distance ring the
    // arrival check accepts (the stand sits across the counter from
    // the spawn).
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

// plazaHerbielReset returns the start state of the scenario: the farm
// readiness shape (the level 15 fighter with the 20,000 SP, the
// 100,000 adena and the empty bag) positioned ON the plaza cell of
// the report - the character is already at the place the ping-pong
// lived on, no walk-in needed.
func plazaHerbielReset(account string) characterReset {
    reset := farmReadinessReset(account)
    reset.X = plazaHerbielX
    reset.Y = plazaHerbielY
    reset.Z = plazaHerbielZ

    return reset
}

// plazaHerbielChecks builds the initial check list of the scenario.
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

// plazaHerbielScenario runs the micro round of the report: the temp
// character wakes on the plaza cell in the farm readiness start
// state, the weapon run trip arms at once (the empty bag, no weapon)
// and the stop ladder walks the legs of the report - the sells at the
// stand the character stands on, the gear buys, then the walk to
// Herbiel through the corridor the ping-pong froze in. Pass: the
// character left the plaza band, crossed the western corridor and
// stands within the interaction distance of Herbiel's spawn within
// the budget.
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
        <-sessionDone
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

// evaluatePlazaHerbiel rewrites the check list of the scenario from
// the live tracker state: the character left the plaza band, crossed
// the western corridor and stands within the Herbiel interaction
// distance. It reports whether every check holds.
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
