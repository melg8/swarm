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

// The church entry round: the owner report of 2026-09-19 - the walk
// from the village plaza cell in front of the temple (44694 51921
// -2808) toward the temple interior cell next to the hierarch
// Asterios (44718 52291 -2792) stops at the entrance, and the same
// click from the real client attached through the swarm proxy refuses
// to enter too, so the refusal is a systemic one. The scenario pins
// the owner contract: the plain manual walk into the temple reaches
// the interior cell at the NPC.
const (
    // churchSpawnX/Y/Z is the plaza cell in front of the temple
    // entrance (the walk start of the owner report).
    churchSpawnX = 44694
    churchSpawnY = 51921
    churchSpawnZ = -2808
    // churchTargetX/Y/Z is the interior cell of the report (the walk
    // destination, 40 units from the temple NPC).
    churchTargetX = 44718
    churchTargetY = 52291
    churchTargetZ = -2792
    // The spawn position of the hierarch Asterios (npc 30154, the
    // temple spawn of the datapack at 44692 52261 -2792): the "to the
    // NPC" half of the contract.
    churchNpcX = 44692
    churchNpcY = 52261

    churchEntryTimeout = 5 * time.Minute
    // The arrival radius of the interior cell check: a couple of
    // collision radii around the destination the walk plans with.
    churchArriveRadius = 60.0
    // The NPC interaction distance of the temple contract: the class
    // master walks land the lessons from well within this ring.
    churchNpcRadius = 200.0
)

// The temp account of the church entry scenario (the first free
// number past the eleven accounts the other scenarios own).
const (
    churchAccount  = "temp13"
    churchPassword = "temp13"
)

// The check ids of the scenario.
const (
    checkChurchWalk   = "walk"
    checkChurchInside = "inside"
    checkChurchNpc    = "npc"
)

// churchEntryReset returns the start state of the scenario: the level
// 15 fighter of the standard starter outfit wakes on the plaza cell
// the report walks from.
func churchEntryReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   15,
        Exp:     level15Exp,
        SP:      20000,
        Adena:   100000,
        Items:   villageEscapeItems,
        X:       churchSpawnX,
        Y:       churchSpawnY,
        Z:       churchSpawnZ,
        MaxHP:   level15HP,
        MaxMP:   level15MP,
        MaxCP:   level15CP,
    }
}

// churchEntryChecks builds the initial check list of the scenario.
func churchEntryChecks() []Check {
    return []Check{
        {
            ID:     checkChurchWalk,
            Label:  "the walk command moved the character off the plaza",
            Done:   false,
            Detail: "standing at the plaza cell",
        },
        {
            ID:     checkChurchInside,
            Label:  "the character stands on the temple interior cell",
            Done:   false,
            Detail: "outside the temple",
        },
        {
            ID:     checkChurchNpc,
            Label:  "the character stands next to Asterios inside",
            Done:   false,
            Detail: "far from the temple NPC",
        },
    }
}

// startChurchSession launches the manual only bot session of the
// temple scenario: the hunt loop consumes the web commands and never
// arms its own trips, so the walk of this scenario is the exact
// manual walk the owner drives. The goroutine reports the session
// end on the channel, the caller owns the cancel.
func startChurchSession(
    ctx context.Context, m *Manager, test *Test,
) (context.CancelFunc, chan error) {
    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        sessionDone <- m.runSession(sessionCtx, churchAccount,
            churchPassword, churchAccount, false, m.proxy,
            test.appendLog)
    }()

    return cancelSession, sessionDone
}

// churchEntryScenario runs the temple entrance round: the temp
// character wakes on the plaza cell of the report, the manual walk
// command aims the interior cell next to the hierarch Asterios and
// the hunt loop (manual only mode) plans and clicks the walk. Pass:
// the character left the plaza, crossed the temple entrance and
// stands on the interior cell within the NPC ring.
func churchEntryScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(churchEntryChecks())

    if err := m.ensureCharacter(churchAccount, churchPassword,
        churchAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(churchEntryReset(churchAccount),
        test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    cancelSession, sessionDone := startChurchSession(ctx, m, test)
    defer cancelSession()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        cancelSession()
        <-sessionDone

        return err
    }

    tracker.PushCommand(state.Command{
        Kind:     state.CommandMove,
        ObjectID: 0,
        Count:    0,
        X:        churchTargetX,
        Y:        churchTargetY,
        Z:        churchTargetZ,
    })
    test.appendLog("acceptance: the walk command to the temple " +
        "interior (44718 52291 -2792) is queued, watching the walk")

    for {
        if ctx.Err() != nil {
            cancelSession()
            <-sessionDone

            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        evaluateChurchEntry(tracker, test)
        if allChecksDone(test) {
            test.appendLog("acceptance: the character entered the " +
                "temple and stands at the NPC, stopping the bot")
            cancelSession()
            if err := <-sessionDone; err != nil {
                return fmt.Errorf("session end: %w", err)
            }
            test.appendLog("acceptance: the bot left the world gracefully")

            return nil
        }
        select {
        case <-ctx.Done():
            cancelSession()
            <-sessionDone

            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(monitorPeriod):
        }
    }
}

// evaluateChurchEntry rewrites the check list of the temple scenario
// from the live tracker state: the character moved off the plaza,
// stands on the interior cell and stands next to the temple NPC.
func evaluateChurchEntry(tracker *state.Bot, test *Test) {
    if tracker.Status() != state.StatusOnline {
        return
    }
    x, y, _, ok := tracker.SelfPosition()
    if !ok {
        return
    }

    startDist := math.Hypot(
        float64(x-churchSpawnX), float64(y-churchSpawnY))
    test.updateCheck(checkChurchWalk, startDist > churchArriveRadius,
        fmt.Sprintf("%.0f units from the plaza cell", startDist))

    targetDist := math.Hypot(
        float64(x-churchTargetX), float64(y-churchTargetY))
    test.updateCheck(checkChurchInside,
        targetDist <= churchArriveRadius,
        fmt.Sprintf("%.0f units from the interior cell", targetDist))

    npcDist := math.Hypot(
        float64(x-churchNpcX), float64(y-churchNpcY))
    test.updateCheck(checkChurchNpc, npcDist <= churchNpcRadius,
        fmt.Sprintf("%.0f units from Asterios", npcDist))
}
