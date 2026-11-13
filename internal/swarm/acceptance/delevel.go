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

// The delevel acceptance scenario runs the guard death cycle end to end on the
// live stack: the outleveled character walks to the archer guards, provokes
// them, pays the death experience penalty down to the announced target level
// and returns to the farm spot without a second deleveling, while the webui
// message carries the target level and the trigger evidence. The start state
// forces the trigger honestly: level 15 at the bottom of its level experience
// on the Green Dryad S-16 cell (median 8, target 13), so two deaths cross the
// boundary and the finished level sits below the gap (docs/hunting.md).
const (
    delevelScenarioID = "delevel"
    delevelAccount    = "temp14"
    delevelPassword   = "temp14"
)

// delevelStartLevel is the injected level: above the dryad median by the
// trigger gap 7, above the Lucky protection and two deaths short of the target
// level 13.
const delevelStartLevel = 15

// The start ground: the Green Dryad S-16 focus cell (43500 54560) on its
// single geodata layer (z -3664), 3200 units south of the Starden guard post -
// the shortest honest guard walk of the low ground registry.
const (
    delevelGroundX = 43500
    delevelGroundY = 54560
    delevelGroundZ = -3664
)

// delevelProgressUnits is the movement the delevel walk must cover: the guard
// post sits ~3200 units away, so a frozen walk never leaves the start
// neighborhood.
const delevelProgressUnits = 2000.0

// delevelReturnRadius is the arrival ring of the walk home at the remembered
// farm spot (the injection point).
const delevelReturnRadius = 1500.0

// delevelNoRetryDefault is the watch after the finished deleveling: the phase
// must stay out of delevel while the bot walks home and resumes the hunt, or
// it would run back to the guards (the anti ping-pong contract).
const delevelNoRetryDefault = 6 * time.Minute

// delevelNoRetryWindow resolves the watch duration: the
// SWARM_DELEVEL_NORETRY_SECONDS knob shortens it for sandbox runs, the
// deployment default stays the six minute contract.
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

// delevelTimeout bounds the whole scenario: both guard death cycles, the
// return walk and the no retry watch.
const delevelTimeout = 25 * time.Minute

// The delevel condition ids.
const (
    // checkDelevelStart holds once the message carried the target, the start
    // level and the median mob level.
    checkDelevelStart = "delevel-start"
    // checkDelevelProgress holds once the walk covered the stuck detector
    // distance.
    checkDelevelProgress = "delevel-progress"
    // checkDelevelDeath holds once a death paid the experience penalty.
    checkDelevelDeath = "delevel-death"
    // checkDelevelTarget holds once the level reached the announcement and the
    // phase left the deleveling.
    checkDelevelTarget = "delevel-target"
    // checkDelevelReturn holds once the bot walked back onto the farm ground.
    checkDelevelReturn = "delevel-return"
    // checkDelevelNoRetry holds once the no retry window passed without a
    // re-entry.
    checkDelevelNoRetry = "delevel-no-retry"
)

// delevelItems is the wearable outfit of the start state, dressed from the bag
// by the world entry auto equipment; nothing sellable, so no junk trip delays
// the deleveling.
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

// delevelReset returns the start state: the level 15 fighter at the bottom of
// the level experience (every death crosses one boundary), standing on the
// dryad cell.
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

// delevelScenario runs the guard death cycle: the outleveled character
// delevels at the guard to the announced target and returns to the farm spot.
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
    sessionDone := make(chan struct{}, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, delevelAccount,
            delevelPassword, delevelAccount, m.proxy, test.appendLog)
        sessionDone <- struct{}{}
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

// delevelObserver accumulates the watch state: the announcement, the walk
// progress, the death count and the post finish return and no retry gates, one
// snapshot per monitor tick.
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

// observeDelevel refreshes the announcement and progress checks from one
// snapshot: the announcement holds once the diagnostics carried the target, the
// start level and the zone median, the progress once the walk crossed the
// stuck detector distance.
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

// observeOutcome refreshes the death, target, return and no retry checks from
// one snapshot, reporting true once every gate latched and failing on a
// deleveling that re-entered after the finished cycle.
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

// delevelWatch observes the session through the guard death cycle: the
// announcement, the progress, the deaths, the target, the return and the no
// retry window; the pass or the context end hands over, a re-entry fails.
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
