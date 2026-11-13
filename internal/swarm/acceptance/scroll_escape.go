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

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The scroll escape acceptance scenario: the level 15 fighter stands on the
// Keltir glade 5 000 units south of the elven village with one Scroll of Escape
// and uses it. The scenario verifies the whole scroll leg of the town trip: the
// skill 2013 cast window with the blue-white vortex icon the C1 client draws
// for it (skill0000), the item consumption and the landing on the elven town
// respawn ring.

// scrollEscapeScenarioID is the scenario registry id.
const scrollEscapeScenarioID = "scroll-escape"

// The temp account of the scenario, the next free slot after the city shopping
// errand (temp25); the password equals the account (the login auto-creates
// missing accounts).
const (
    scrollEscapeAccount  = "temp26"
    scrollEscapePassword = "temp26"
)

// scrollEscapeTimeout bounds the whole run: the reset, the login handshake, the
// 20 second cast, the landing and the graceful shutdown.
const scrollEscapeTimeout = 5 * time.Minute

// The start ground: the Keltir glade of the elven starting grounds (the
// oren02_2119_08 territory), far enough from the town ring to prove the travel.
// Every Keltir carries isAggressive="false" in the datapack (a missing
// attribute would default to aggressive), so the 20 second stationary cast is
// never interrupted, and no race respawn zone covers the cell, so the scroll
// lands on the town ring instead of a starting ground respawn.
const (
    scrollEscapeSpawnX = 45496
    scrollEscapeSpawnY = 44544
    scrollEscapeSpawnZ = -3704
    scrollEscapeLevel  = 15
)

// The teleport landing: the twelve non-chaotic respawn points of elf_town
// (dist/game/data/mapregion/elf_town.xml) spread a 1 114 by 1 072 box, and the
// server picks one at random because Player.ini keeps
// RandomRespawnInTownEnabled on. The radius around the box center covers every
// point plus the 50 unit offset of OffsetOnTeleportEnabled.
const (
    scrollEscapeTownX      = 45355
    scrollEscapeTownY      = 49561
    scrollEscapeTownRadius = 950.0
)

// The skill of the scroll and the icon the C1 client draws for it: the
// decrypted skillgrp.txt points skill 2013 at icon.skill0000, the blue-white
// vortex of the unstuck art.
const (
    scrollEscapeSkillID = 2013
    scrollEscapeIcon    = "skill0000"
)

// scrollEscapePoll paces the cast, the inventory and the position
// watch: the 20 second cast window must never slip between two reads.
const scrollEscapePoll = 250 * time.Millisecond

// scrollEscapeInventoryWait bounds the wait for the injected scroll
// to reach the live inventory after the world entry.
const scrollEscapeInventoryWait = 30 * time.Second

// scrollEscapeDrainWait bounds the wait for the escape goroutine once every
// check holds: its landing poll runs at the same quarter second, so it answers
// almost at once.
const scrollEscapeDrainWait = 5 * time.Second

// The condition ids of the scenario.
const (
    // checkScrollCast holds once the skill 2013 cast window reached
    // the tracker (the server broadcast the MagicSkillUse back).
    checkScrollCast = "cast"
    // checkScrollIcon holds once the cast state carries the vortex
    // art of the skill dictionary.
    checkScrollIcon = "icon"
    // checkScrollConsumed holds once the scroll left the bag.
    checkScrollConsumed = "consumed"
    // checkScrollLanded holds once the character stands on the town
    // respawn ring.
    checkScrollLanded = "landed"
)

// scrollEscapeReset returns the start state: the level 15 vitals on the Keltir
// glade with one Scroll of Escape in the bag (the display id the hunt economy
// injects).
func scrollEscapeReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   scrollEscapeLevel,
        Exp:     level15Exp,
        SP:      0,
        Adena:   0,
        Items:   []ResetItem{{ItemID: hunt.SOEItemID(), Count: 1}},
        X:       scrollEscapeSpawnX,
        Y:       scrollEscapeSpawnY,
        Z:       scrollEscapeSpawnZ,
        MaxHP:   level15HP,
        MaxMP:   level15MP,
        MaxCP:   level15CP,
    }
}

// scrollEscapeChecks builds the initial check list in its narrative order.
func scrollEscapeChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID:    checkScrollCast,
            Label: "the Scroll of Escape cast window opened (skill 2013)",
            Done:  false, Detail: "",
        },
        {
            ID:    checkScrollIcon,
            Label: "the cast published the blue-white vortex art",
            Done:  false, Detail: "",
        },
        {
            ID:    checkScrollConsumed,
            Label: "the server consumed the scroll from the bag",
            Done:  false, Detail: "",
        },
        {
            ID:    checkScrollLanded,
            Label: "the character landed on the elven town respawn ring",
            Done:  false, Detail: "",
        },
    }
}

// scrollEscapeScenario runs the scroll round: the temp character wakes with the
// scroll in the bag, the hunt loop stays manual so no hunt or trip competes
// with the cast, and the watch verifies the cast, the icon, the consumption and
// the landing.
func scrollEscapeScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(scrollEscapeChecks())
    if err := prepareScrollEscapeState(m, test); err != nil {
        return err
    }

    cancelSession, sessionDone, loopCh :=
        startScrollEscapeSession(ctx, m, test)
    defer cancelSession()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        return stopScrollEscapeSession(cancelSession, sessionDone, err)
    }
    test.appendLog("acceptance: the bot is in the world on the " +
        "Keltir glade with one Scroll of Escape, watching the cast")

    loop, err := awaitScrollEscapeLoop(ctx, loopCh)
    if err != nil {
        return stopScrollEscapeSession(cancelSession, sessionDone, err)
    }
    if err := waitScrollPresent(ctx, tracker); err != nil {
        return stopScrollEscapeSession(cancelSession, sessionDone, err)
    }

    return watchScrollEscape(ctx, tracker, test, loop,
        cancelSession, sessionDone)
}

// prepareScrollEscapeState creates the temp character and injects the start
// state: the level 15 fighter with one scroll in the bag on the Keltir glade.
func prepareScrollEscapeState(m *Manager, test *Test) error {
    if err := m.ensureCharacter(scrollEscapeAccount,
        scrollEscapePassword, scrollEscapeAccount,
        test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(scrollEscapeReset(scrollEscapeAccount),
        test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    return nil
}

// watchScrollEscape drives the scroll and the watch: the escape seam runs in
// its own goroutine (the arrival wait is a blocking poll) while the scenario
// reads the cast, the bag and the position every quarter second.
func watchScrollEscape(
    ctx context.Context, tracker *state.Bot, test *Test,
    loop *hunt.Loop, cancelSession context.CancelFunc,
    sessionDone chan error,
) error {
    escapeDone := make(chan bool, 1)
    go func() {
        escapeDone <- loop.EscapeWithSOE()
    }()

    watch := &scrollEscapeWatch{}
    for {
        if ctx.Err() != nil {
            return stopScrollEscapeSession(cancelSession, sessionDone,
                fmt.Errorf("cancelled: %w", ctx.Err()))
        }
        watch.evaluate(tracker, test)
        if allChecksDone(test) {
            return finishScrollEscapeSession(test, escapeDone,
                cancelSession, sessionDone, watch.landedDetail)
        }
        select {
        case <-ctx.Done():
            return stopScrollEscapeSession(cancelSession, sessionDone,
                fmt.Errorf("cancelled: %w", ctx.Err()))
        case landed := <-escapeDone:
            escapeDone = nil
            if !landed {
                return stopScrollEscapeSession(cancelSession, sessionDone,
                    errors.New("the scroll of escape never landed the "+
                        "character - the cast or the teleport failed"))
            }
            watch.evaluate(tracker, test)
        case <-time.After(scrollEscapePoll):
        }
    }
}

// stopScrollEscapeSession stops the bot after an early failure and
// answers the cause.
func stopScrollEscapeSession(
    cancelSession context.CancelFunc, sessionDone chan error, cause error,
) error {
    cancelSession()
    joinSessionDone(sessionDone)

    return cause
}

// startScrollEscapeSession launches the supervised session with the loop hook
// that keeps the loop manual and hands it to the scenario: the scroll use needs
// the live loop, and the autonomy would start a town trip of its own.
func startScrollEscapeSession(
    ctx context.Context, m *Manager, test *Test,
) (context.CancelFunc, chan error, <-chan *hunt.Loop) {
    loopCh := make(chan *hunt.Loop, 1)
    hook := func(loop *hunt.Loop) {
        loop.SetAutonomy(false)
        select {
        case loopCh <- loop:
        default:
        }
    }

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, scrollEscapeAccount,
            scrollEscapePassword, scrollEscapeAccount, m.proxy,
            test.appendLog, hook)
        sessionDone <- nil
    }()

    return cancelSession, sessionDone, loopCh
}

// awaitScrollEscapeLoop waits for the hook delivery of the session loop, which
// runs right after the world entry, so the answer is ready by the time the
// online wait returns.
func awaitScrollEscapeLoop(
    ctx context.Context, loopCh <-chan *hunt.Loop,
) (*hunt.Loop, error) {
    select {
    case loop := <-loopCh:
        return loop, nil
    case <-ctx.Done():
        return nil, fmt.Errorf("cancelled: %w", ctx.Err())
    case <-time.After(scrollEscapeInventoryWait):
        return nil, errors.New("the session loop never reached the " +
            "acceptance hook")
    }
}

// waitScrollPresent waits for the injected scroll to reach the live
// inventory: the world entry carries the item list asynchronously.
func waitScrollPresent(ctx context.Context, tracker *state.Bot) error {
    deadline := time.Now().Add(scrollEscapeInventoryWait)
    for {
        if tracker.InventoryHasItem(hunt.SOEItemID()) {
            return nil
        }
        if time.Now().After(deadline) {
            return errors.New("the injected Scroll of Escape never " +
                "reached the bag")
        }
        select {
        case <-ctx.Done():
            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(scrollEscapePoll):
        }
    }
}

// finishScrollEscapeSession stops the bot after the pass and drains the escape
// goroutine, whose landing poll ends within the quarter second of the
// successful check.
func finishScrollEscapeSession(
    test *Test, escapeDone <-chan bool, cancelSession context.CancelFunc,
    sessionDone chan error, landing string,
) error {
    if escapeDone != nil {
        select {
        case <-escapeDone:
        case <-time.After(scrollEscapeDrainWait):
        }
    }
    test.appendLog("acceptance: the scroll landed the character on the " +
        "town ring (" + landing + "), stopping the bot")
    cancelSession()
    if err := <-sessionDone; err != nil {
        return fmt.Errorf("session end: %w", err)
    }
    test.appendLog("acceptance: the bot left the world gracefully")

    return nil
}

// scrollEscapeWatch accumulates the latches: the cast, the icon and the
// consumption retire their checks once seen, the landing latches the ring
// position.
type scrollEscapeWatch struct {
    castSeen       bool
    castDetail     string
    iconSeen       bool
    iconDetail     string
    presentSeen    bool
    consumedSeen   bool
    consumedDetail string
    landedSeen     bool
    landedDetail   string
}

// evaluate rewrites the check list from one live tracker read.
func (w *scrollEscapeWatch) evaluate(tracker *state.Bot, test *Test) {
    if tracker.Status() != state.StatusOnline {
        return
    }
    w.evaluateScroll(tracker, test)
    w.evaluateLanding(tracker, test)
}

// evaluateScroll latches the cast window, its icon and the item consumption.
func (w *scrollEscapeWatch) evaluateScroll(
    tracker *state.Bot, test *Test,
) {
    if tracker.InventoryHasItem(hunt.SOEItemID()) {
        w.presentSeen = true
    } else if w.presentSeen {
        w.consumedSeen = true
        w.consumedDetail = "the scroll left the bag"
    }
    for _, view := range tracker.SkillStates() {
        if view.SkillID != scrollEscapeSkillID {
            continue
        }
        w.castSeen = true
        w.castDetail = fmt.Sprintf("skill %d cast window of %d ms",
            view.SkillID, view.CastTotalMs)
        if view.Icon == scrollEscapeIcon {
            w.iconSeen = true
            w.iconDetail = "the cast carries the " + view.Icon + " art"
        }
    }
    test.updateCheck(checkScrollCast, w.castSeen, w.castDetail)
    test.updateCheck(checkScrollIcon, w.iconSeen, w.iconDetail)
    test.updateCheck(checkScrollConsumed, w.consumedSeen,
        w.consumedDetail)
}

// evaluateLanding latches the town ring landing.
func (w *scrollEscapeWatch) evaluateLanding(
    tracker *state.Bot, test *Test,
) {
    x, y, _, ok := tracker.SelfPosition()
    if !ok {
        return
    }
    dist := math.Hypot(
        float64(x-scrollEscapeTownX),
        float64(y-scrollEscapeTownY))
    detail := fmt.Sprintf("%.0f units from the town ring center", dist)
    if dist <= scrollEscapeTownRadius {
        w.landedSeen = true
        w.landedDetail = fmt.Sprintf("standing at %d,%d", x, y)
        detail = w.landedDetail
    }
    test.updateCheck(checkScrollLanded, w.landedSeen, detail)
}
