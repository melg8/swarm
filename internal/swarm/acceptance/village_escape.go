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

// The dump start state of the village escape scenario (the
// 2026-09-14 12:31 stuck report, build 73fcfa6, bot test3): the
// level 15 fighter stood at the elven village center cell
// (the Mother Tree plaza where the village streets converge)
// through a whole day of refused walks - three dumps of the same
// position (08:42, 10:18 and 12:31) show the character never
// moving a single cell while the corridor bans, the detour
// re-plans and the server routed hops all failed. The scenario
// pins the exact recovery contract the user demanded: from this
// position the bot finds a path and gets out of the city within
// two minutes at most.
const (
    // villageEscapeSpawnX/Y/Z is the reported stuck position (the
    // dump of 2026-09-14 12:31, the village plaza cell of test3).
    villageEscapeSpawnX = 45768
    villageEscapeSpawnY = 49848
    villageEscapeSpawnZ = -3056
    // The vitals, the wallet and the experience of the dump
    // character: level 15 at 87.09 percent of the level span.
    villageEscapeExp   = 321857
    villageEscapeSP    = 1760
    villageEscapeAdena = 1312
    villageEscapeHP    = 358
    villageEscapeMP    = 145
    villageEscapeCP    = 178
)

// villageEscapeItems is the exact item set of the dump: the equipped
// paperdoll (the Brandish two hander, the bone armor set, the leather
// helmet and gloves, the starter jewels) and the bag of the report
// (the arrows, the hunting bow and the two crafting leftovers). The
// injection lands every stack in the bag - the auto equipment of the
// hunt loop dresses the character from it.
var villageEscapeItems = []ResetItem{
    {ItemID: 24, Count: 1},   // Bone Breastplate
    {ItemID: 31, Count: 1},   // Bone Gaiters
    {ItemID: 38, Count: 1},   // Low Boots
    {ItemID: 44, Count: 1},   // Leather Helmet
    {ItemID: 50, Count: 1},   // Leather Gloves
    {ItemID: 112, Count: 1},  // Apprentice's Earring
    {ItemID: 112, Count: 1},  // Apprentice's Earring
    {ItemID: 116, Count: 1},  // Magic Ring
    {ItemID: 116, Count: 1},  // Magic Ring
    {ItemID: 118, Count: 1},  // Necklace of Magic
    {ItemID: 1333, Count: 1}, // Brandish (the two hand sword of the dump)
    {ItemID: 17, Count: 589}, // Wooden Arrow
    {ItemID: 271, Count: 1},  // Hunting Bow
    {ItemID: 1866, Count: 1}, // Suede
    {ItemID: 1867, Count: 1}, // Animal Skin
}

// villageEscapeReset returns the dump start state of the village
// escape scenario: the reported plaza cell, the reported level and
// vitals, the reported wallet and the reported item set.
func villageEscapeReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   15,
        Exp:     villageEscapeExp,
        SP:      villageEscapeSP,
        Adena:   villageEscapeAdena,
        Items:   villageEscapeItems,
        X:       villageEscapeSpawnX,
        Y:       villageEscapeSpawnY,
        Z:       villageEscapeSpawnZ,
        MaxHP:   villageEscapeHP,
        MaxMP:   villageEscapeMP,
        MaxCP:   villageEscapeCP,
    }
}

// The escape contract of the scenario: the character is out of the
// city when its distance from the village center passes the escape
// radius, and the whole escape (from the world entry at the dump
// cell to the open ground past the village) fits the escape window.
//
// The radius covers every village structure of the geodata: the
// farthest village npc of the dump stood 2734 units from the plaza
// (Cobendell at the trainer hall), the southwest gate corridor the
// walk plans exit through sits 2348 units out, and the village
// streets the zone return scenario starts from reach 2760 units -
// a character 3000 units from the plaza stands on the open road to
// the hunting grounds past every wall, gate and deck of the city.
const (
    villageEscapeRadius = 3000.0
    villageEscapeWindow = 2 * time.Minute
    villageEscapeBudget = 5 * time.Minute
)

// villageEscapeScenario runs the refused-click dump round: the temp
// character wakes at the village plaza cell of the 12:31 dump with
// the dump state and must get out of the city on its own within the
// two minute window - the day of dumps held characters on that very
// cell forever (the corridor bans, the detour re-plans and the
// server routed hops of the report all failed to move them).
func villageEscapeScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(villageEscapeChecks())

    tracker, cancelSession, sessionDone, err :=
        launchEscapeSession(ctx, m, test)
    if err != nil {
        return err
    }
    defer cancelSession()

    onlineSince := time.Now()
    test.appendLog("acceptance: the bot is in the world at the village " +
        "plaza cell, watching the escape window")

    for {
        if ctx.Err() != nil {
            cancelSession()
            <-sessionDone

            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        escaped, detail := evaluateVillageEscape(tracker, test)
        if escaped {
            test.appendLog("acceptance: the character left the city in " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + "), stopping the bot")
            cancelSession()
            if err := <-sessionDone; err != nil {
                return fmt.Errorf("session end: %w", err)
            }
            test.appendLog("acceptance: the bot left the world gracefully")

            return nil
        }
        if time.Since(onlineSince) > villageEscapeWindow {
            cancelSession()
            <-sessionDone

            return errors.New("the character stood inside the city for " +
                time.Since(onlineSince).Round(time.Second).String() +
                " (" + detail + ") - the escape window closed")
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

// launchEscapeSession prepares the dump character and starts the
// supervised bot session of the escape scenario: the character
// ensure, the dump state injection, the session goroutine and the
// world entry wait. The returned cancel stops the session goroutine;
// the caller drains the done channel on every exit path.
func launchEscapeSession(
    ctx context.Context, m *Manager, test *Test,
) (*state.Bot, context.CancelFunc, <-chan error, error) {
    if err := m.ensureCharacter(escapeAccount, escapePassword,
        escapeAccount, test.appendLog); err != nil {
        return nil, nil, nil, fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(
        villageEscapeReset(escapeAccount), test); err != nil {
        return nil, nil, nil, fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        sessionDone <- m.runSessionSupervised(sessionCtx, escapeAccount,
            escapePassword, escapeAccount, true, m.proxy, test.appendLog)
    }()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        cancelSession()
        <-sessionDone

        return nil, nil, nil, err
    }

    return tracker, cancelSession, sessionDone, nil
}

// evaluateVillageEscape rewrites the check list of the escape
// scenario from the live tracker state: the escape check holds once
// the character stands beyond the village radius from the dump plaza
// cell. It returns the verdict and the detail line of the moment.
func evaluateVillageEscape(
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
        float64(x-villageEscapeSpawnX),
        float64(y-villageEscapeSpawnY))
    detail := fmt.Sprintf("%.0f units from the village plaza",
        dist)
    test.updateCheck(checkEscape, dist >= villageEscapeRadius, detail)

    return dist >= villageEscapeRadius, detail
}

// villageEscapeChecks builds the initial check list of the escape
// scenario.
func villageEscapeChecks() []Check {
    return []Check{{
        ID:     checkEscape,
        Label:  "left the city within the two minute window",
        Done:   false,
        Detail: "standing at the village plaza cell",
    }}
}
