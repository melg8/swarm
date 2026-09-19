// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "math"
    "os"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The delevel scenario (the owner demand): the guard death cycle must
// run end to end on the live stack - the outleveled character walks
// to the archer guards, provokes them, pays the death experience
// penalty down to the announced target level and returns to the farm
// spot WITHOUT re-entering the deleveling - and the webui message of
// the running deleveling must carry the target level and the trigger
// evidence (the start level against the median mob level of the held
// ground).
//
// The start state forces the trigger honestly: the character wakes ON
// the Green Dryad S-16 hunting cell (the standing ground handoff of
// the cell policy - the position IS the intent) as level 15 with the
// bottom of level experience. The cell median is 8 (the Green Dryad
// and the Kaboo Orc Grunt of the ground), so the trigger gap (level -
// median >= 7) holds for the live and the static median alike, and
// the target (median + 5) lands at 13 - two guard deaths from the
// level bottom (the Mobius death penalty removes a percentage of the
// LEVEL SPAN, so every death from the bottom crosses one level
// boundary). The finished cycle holds the hysteresis: level 13 over
// the median 8 sits below the trigger gap, so the hunt resumes
// instead of a second deleveling.
const (
    delevelScenarioID = "delevel"
    delevelAccount    = "temp14"
    delevelPassword   = "temp14"
)

// delevelStartLevel is the injected level: 15 sits above the dryad
// ground median by exactly the trigger gap 7, keeps the character
// above the Lucky protection (the deaths pay the penalty from 10 up)
// and two deaths short of the target level 13.
const delevelStartLevel = 15

// The start ground: the Green Dryad S-16 focus cell (43500 54560) on
// its single geodata layer (the probe of the pack answers z -3664
// for every reference height) - 3200 units south of the Starden
// guard post, the shortest honest guard walk of the low ground
// registry.
const (
    delevelGroundX = 43500
    delevelGroundY = 54560
    delevelGroundZ = -3664
)

// delevelProgressUnits is the movement the delevel walk must cover
// while the phase runs: the guard post sits ~3200 units from the
// dryad ground, the honest attempt crosses thousands of units, a
// frozen walk (the stuck report class the check owns) never leaves
// the start neighborhood.
const delevelProgressUnits = 2000.0

// delevelReturnRadius is the arrival ring of the walk home: the
// return leg aims at the remembered farm spot (the injection point),
// the arrival lands the character back on the dryad ground.
const delevelReturnRadius = 1500.0

// delevelNoRetryDefault is the watch after the finished deleveling:
// the phase must stay out of delevel while the bot walks home and
// resumes the hunt (the anti ping-pong contract - the bots must not
// return to the farm spot only to run back to the guards).
const delevelNoRetryDefault = 6 * time.Minute

// delevelNoRetryWindow resolves the watch duration: the
// SWARM_DELEVEL_NORETRY_SECONDS environment variable shapes it the
// way the soak duration and milestone level knobs do (the sandbox
// verification runs with a bounded tool window shorten it; the
// deployment default stays the six minute contract).
func delevelNoRetryWindow() time.Duration {
    raw := os.Getenv("SWARM_DELEVEL_NORETRY_SECONDS")
    if raw != "" {
        if secs, err := strconv.Atoi(raw); err == nil &&
            secs > 0 && secs <= 3600 {
            wait := time.Duration(secs) * time.Second

            return wait
        }
    }

    return delevelNoRetryDefault
}

// delevelTimeout bounds the whole scenario: the two guard death
// cycles (the walk to the guard, the provoke, the revive, the second
// walk), the return walk and the no retry watch fit well inside a
// quarter and a half of the farm readiness budget.
const delevelTimeout = 25 * time.Minute

// The delevel condition ids.
const (
    // checkDelevelStart holds once the diagnostics carried the full
    // deleveling message: the target level, the start level and the
    // median mob level of the held ground.
    checkDelevelStart = "delevel-start"
    // checkDelevelProgress holds once the delevel walk covered the
    // stuck detector distance (the frozen walk never does).
    checkDelevelProgress = "delevel-progress"
    // checkDelevelDeath holds once a death paid the experience
    // penalty (the level dropped).
    checkDelevelDeath = "delevel-death"
    // checkDelevelTarget holds once the character reached the
    // announced target level and the phase left the deleveling.
    checkDelevelTarget = "delevel-target"
    // checkDelevelReturn holds once the bot walked back onto the farm
    // ground after the finish.
    checkDelevelReturn = "delevel-return"
    // checkDelevelNoRetry holds once the no retry window passed
    // without a deleveling re-entry.
    checkDelevelNoRetry = "delevel-no-retry"
)

// delevelItems is the wearable outfit of the start state: every slot
// dressed from the bag by the world entry auto equipment, nothing
// sellable (the junk town trip would delay the deleveling the
// scenario watches).
var delevelItems = []ResetItem{
    {ItemID: 1333, Count: 1}, // Brandish (the two hand sword)
    {ItemID: 23, Count: 1},   // Wooden Breastplate
    {ItemID: 31, Count: 1},   // Bone Gaiters
    {ItemID: 44, Count: 1},   // Leather Helmet
    {ItemID: 50, Count: 1},   // Leather Gloves
    {ItemID: 1121, Count: 1}, // Apprentice's Shoes
    {ItemID: 114, Count: 1},  // Earring of Strength
    {ItemID: 115, Count: 1},  // Earring of Wisdom
    {ItemID: 876, Count: 2},  // Ring of Anguish
    {ItemID: 907, Count: 1},  // Necklace of Anguish
}

// delevelReset returns the start state of the delevel scenario: the
// level 15 fighter at the bottom of the level experience (every
// guard death crosses one level boundary), standing on the dryad
// ground cell.
func delevelReset(account string) characterReset {
    gain := elvenFighterVitals[delevelStartLevel-1]

    return characterReset{
        Account: account,
        Char:    account,
        Level:   delevelStartLevel,
        // The bottom of level 15: the threshold itself - the first
        // penalty removes enough experience to cross the boundary.
        Exp:   soakExperienceTable[delevelStartLevel-1],
        SP:    milestoneSP,
        Adena: milestoneWallet,
        Items: delevelItems,
        X:     delevelGroundX,
        Y:     delevelGroundY,
        Z:     delevelGroundZ,
        MaxHP: gain.HP,
        MaxMP: gain.MP,
        MaxCP: gain.CP,
    }
}

// delevelChecks builds the initial check list of the delevel
// scenario: the narrative order of the guard death cycle.
func delevelChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID: checkDelevelStart,
            Label: "the deleveling announced its target level and " +
                "reason",
            Done: false, Detail: "",
        },
        {
            ID:    checkDelevelProgress,
            Label: "the delevel walk made progress (no freeze)",
            Done:  false, Detail: "",
        },
        {
            ID:    checkDelevelDeath,
            Label: "a guard death paid the experience penalty",
            Done:  false, Detail: "",
        },
        {
            ID: checkDelevelTarget,
            Label: "the deaths dropped the character to the announced " +
                "target",
            Done: false, Detail: "",
        },
        {
            ID:    checkDelevelReturn,
            Label: "the bot walked back onto the farm ground",
            Done:  false, Detail: "",
        },
        {
            ID:    checkDelevelNoRetry,
            Label: "no new deleveling after the return (no ping-pong)",
            Done:  false, Detail: "",
        },
    }
}

// delevelScenario runs the guard death cycle round: the outleveled
// character delevels at the town guards to the announced target and
// returns to the farm spot without re-entering the deleveling.
func delevelScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(delevelChecks())

    if err := m.ensureCharacter(delevelAccount, delevelPassword,
        delevelAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(delevelReset(delevelAccount), test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, delevelAccount,
            delevelPassword, delevelAccount, m.proxy, test.appendLog)
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
    test.appendLog(fmt.Sprintf("acceptance: the bot is in the world on "+
        "the dryad ground as level %d with the bottom of level "+
        "experience, watching the deleveling", tracker.SelfLevel()))

    return delevelWatch(ctx, test, tracker)
}

// delevelObserver accumulates the watch state of the delevel
// scenario: the announcement capture, the walk progress, the death
// count and the post finish gates (the return and the no retry
// window), one tracker snapshot per monitor tick.
type delevelObserver struct {
    deaths        *deathEdgeTracker
    announced     int32
    progressUnits float64
    lastX, lastY  int32
    havePos       bool
    startedAt     time.Time
    startLevel    int32
    finished      bool
    noRetryUntil  time.Time
    returned      bool
    noRetryHeld   bool
}

// newDelevelObserver arms the observer at the world entry: the start
// level anchors the completion log line.
func newDelevelObserver(tracker *state.Bot) *delevelObserver {
    return &delevelObserver{
        deaths:        newDeathEdgeTracker(),
        announced:     0,
        progressUnits: 0,
        lastX:         0,
        lastY:         0,
        havePos:       false,
        startedAt:     time.Now(),
        startLevel:    tracker.SelfLevel(),
        finished:      false,
        noRetryUntil:  time.Time{},
        returned:      false,
        noRetryHeld:   false,
    }
}

// observeDelevel refreshes the announcement and progress checks from
// one snapshot: the announcement check holds once the diagnostics
// carried the full deleveling message (the target level, the start
// level and the zone median), the progress check holds once the
// delevel walk crossed the stuck detector distance (the frozen walk
// of the stuck reports never leaves the start neighborhood).
func (o *delevelObserver) observeDelevel(
    test *Test, diag state.HuntDiagnostics, x, y int32, posOK bool,
) {
    if diag.DelevelActive {
        if o.announced == 0 && diag.DelevelTarget > 0 &&
            diag.DelevelFromLevel > 0 && diag.DelevelZoneMedian > 0 {
            o.announced = diag.DelevelTarget
            test.appendLog(fmt.Sprintf("acceptance: the deleveling "+
                "announced the target level %d (start level %d over "+
                "the median %d)", o.announced, diag.DelevelFromLevel,
                diag.DelevelZoneMedian))
        }
        if posOK {
            if o.havePos {
                o.progressUnits += math.Hypot(
                    float64(x-o.lastX), float64(y-o.lastY))
            }
            o.lastX, o.lastY, o.havePos = x, y, true
        }
    }
    test.updateCheck(checkDelevelStart, o.announced > 0,
        delevelStartDetail(o.announced, diag))
    test.updateCheck(checkDelevelProgress,
        o.progressUnits >= delevelProgressUnits,
        fmt.Sprintf("%.0f of %.0f units of the guard walk",
            o.progressUnits, delevelProgressUnits))
}

// observeOutcome refreshes the death, target, return and no retry
// checks from one snapshot. It reports true once every gate of the
// cycle latched (the pass) and fails with the ping-pong error when
// the deleveling re-entered after the finished cycle.
func (o *delevelObserver) observeOutcome(
    test *Test, tracker *state.Bot, diag state.HuntDiagnostics,
    x, y int32, posOK bool,
) (bool, error) {
    test.updateCheck(checkDelevelDeath, o.deaths.deaths() > 0,
        fmt.Sprintf("%d death(s) since the world entry",
            o.deaths.deaths()))
    level := tracker.SelfLevel()
    if !o.finished && o.announced > 0 && !diag.DelevelActive &&
        level > 0 && level <= o.announced {
        o.finished = true
        o.noRetryUntil = time.Now().Add(delevelNoRetryWindow())
        test.appendLog(fmt.Sprintf("acceptance: the deleveling "+
            "finished at level %d (target %d), watching the return "+
            "and the no retry window", level, o.announced))
    }
    if o.announced > 0 {
        test.updateCheck(checkDelevelTarget, o.finished,
            fmt.Sprintf("level %d of the target %d", level,
                o.announced))
    }
    if !o.finished {
        return false, nil
    }
    if posOK {
        dist := math.Hypot(
            float64(x-delevelGroundX), float64(y-delevelGroundY))
        if dist <= delevelReturnRadius {
            o.returned = true
        }
        test.updateCheck(checkDelevelReturn, o.returned,
            fmt.Sprintf("%.0f units from the farm ground", dist))
    }
    if diag.DelevelActive {
        // The ping-pong the contract forbids: a fresh deleveling
        // after the finished one.
        test.updateCheck(checkDelevelNoRetry, false,
            "the deleveling re-entered after the finish")

        return false, errors.New("the bot re-entered the deleveling " +
            "after the finished cycle - the farm and village commute " +
            "the contract forbids")
    }
    if !o.noRetryUntil.IsZero() && time.Now().After(o.noRetryUntil) {
        o.noRetryHeld = true
        test.updateCheck(checkDelevelNoRetry, true,
            "the no retry window passed with the phase out of delevel")
    }

    return o.returned && o.noRetryHeld, nil
}

// delevelWatch observes the session through the guard death cycle:
// the announcement, the walk progress, the deaths, the target, the
// return and the no retry window. The pass hands over to the
// teardown, the context end (the manager timeout among others) or a
// deleveling re-entry inside the window fails the run.
func delevelWatch(
    ctx context.Context, test *Test, tracker *state.Bot,
) error {
    o := newDelevelObserver(tracker)
    for {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        o.deaths.update(tracker)
        snap := tracker.Snapshot()
        diag := snap.Diagnostics.Hunt
        x, y, _, posOK := tracker.SelfPosition()
        o.observeDelevel(test, diag, x, y, posOK)
        complete, err := o.observeOutcome(test, tracker, diag, x, y, posOK)
        if err != nil {
            return err
        }
        if complete {
            test.appendLog(fmt.Sprintf("acceptance: the delevel cycle "+
                "completed: level %d -> %d in %s", o.startLevel,
                tracker.SelfLevel(),
                time.Since(o.startedAt).Round(time.Second).String()))

            return nil
        }
        select {
        case <-ctx.Done():
            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(monitorPeriod):
        }
    }
}

// delevelStartDetail renders the announcement check detail from the
// captured target and the live diagnostics.
func delevelStartDetail(announced int32, diag state.HuntDiagnostics) string {
    if announced <= 0 {
        return "waiting for the deleveling announcement"
    }

    return fmt.Sprintf("deleveling to level %d (start level %d over the "+
        "median %d)", announced, diag.DelevelFromLevel,
        diag.DelevelZoneMedian)
}
