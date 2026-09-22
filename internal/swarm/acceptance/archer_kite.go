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
    "strings"
    "sync"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The archer kite acceptance scenario (issue #28, the codeable
// acceptance half of #20): a temp account launches as the archer
// type of the fleet, wakes on a Kaboo Orc hunting cell with the bow
// kit in the bag and farms the window under the kiting contract -
// the fight happens at the bow radii, the retreats step when the
// mobs close, the bot survives and the quiver never runs dry.

// archerKiteScenarioID is the scenario registry id.
const archerKiteScenarioID = "archer-kite"

// archerKiteAccount is the temp account of the archer scenario:
// the next free slot after the stuck point spread (the ladder
// holds temp1 through temp17; temp18 keeps the parallel run all
// collision free). The password equals the account (the Mobius
// login auto-creates missing accounts).
const (
    archerKiteAccount  = "temp18"
    archerKitePassword = "temp18"
)

// archerKiteDefaultMinutes is the smoke window of the scenario: the
// kiting contract needs enough fights to observe the steps (the
// mass 4 Kaboo cell clears in ~2 minutes per round with the
// 15-20 s respawns pacing the re-engages), short enough to fit the
// 2 h session dev cycle. A longer measured round sets
// SWARM_ARCHER_KITE_MINUTES the way the soak smoke sets
// SWARM_SOAK_MINUTES; the scenario itself is duration-agnostic.
const archerKiteDefaultMinutes = 10

// archerKiteShutdownGrace is the budget the scenario gives the
// supervised session to unwind after the window: the logout
// announcement, the combat stance delay and the socket flush (the
// manager's restartWait covers the worst case on top).
const archerKiteShutdownGrace = 90 * time.Second

// The start ground of the archer scenario: the Kaboo Orc Grunt SW-5
// cell (elven-hex-104) of the elven registry - the aggressive melee
// orcs close on the archer the moment they see the bow shots, which
// is exactly the threat the kiting contract answers; the mass 4
// ground (a Kaboo Orc Fighter, a Spore Fungus and two Kaboo Orc
// Grunts) keeps the fight cadence honest through the respawns. The
// level 7 start sits inside the cell band (7-10, the ladder has no
// reason to relocate), below the guide window (8-24: the unbuffed
// start never walks the 26 km guide trip first) and miles away from
// the delevel scenario's ground (the temp14 guard death cycle at
// 43500 54560) so the parallel run all never pits two bots against
// one cell. The Z is the geodata height of the focus (the same
// oracle the walk planner trusts).
const (
    archerKiteSpawnX = 36000
    archerKiteSpawnY = 50229
    archerKiteSpawnZ = -3456
)

// The start state of the archer: the level 7 elven fighter of the
// vitals table, the bottom of the level 7 experience span, no
// wallet (nothing affordable, no shopping trip arms), no skill
// points (no lesson prefix clears the 300 sp trip gate) and the bow
// kit in the bag - the short bow, a full quiver (600 arrows, the
// restock target the shop strategy buys to; the floor sits at 150
// and the smoke window cannot spend the difference) and the wooden
// outfit of the zone return dump (the same wearable set the proven
// reset injects).
const (
    archerKiteStartLevel = 7
    archerKiteStartHP    = 167
    archerKiteStartMP    = 63
    archerKiteStartCP    = 67
    // archerKiteQuiver is the injected arrow count: the restock
    // target of the shop strategy, so the quiver starts full and
    // the window observes the consumption against the floor.
    archerKiteQuiver = 600
)

// archerKiteItems is the injected start kit: the wooden armor set
// and the starter jewels of the zone return dump (the proven
// wearable ids), the short bow instead of the melee weapon and the
// full quiver. No shield: the left hand belongs to the arrows (the
// archer profile lets nothing else stay there).
var archerKiteItems = []ResetItem{
    {ItemID: 13, Count: 1},   // Short Bow
    {ItemID: 17, Count: 600}, // Wooden Arrow (the quiver)
    {ItemID: 23, Count: 1},   // Wooden Breastplate
    {ItemID: 31, Count: 1},   // Bone Gaiters
    {ItemID: 44, Count: 1},   // Leather Helmet
    {ItemID: 50, Count: 1},   // Leather Gloves
    {ItemID: 1121, Count: 1}, // Apprentice's Shoes
    {ItemID: 114, Count: 1},  // Earring of Strength
    {ItemID: 115, Count: 1},  // Earring of Wisdom
    {ItemID: 876, Count: 2},  // Ring of Anguish
    {ItemID: 907, Count: 1},  // Necklace of Anguish
    {ItemID: 1060, Count: 3}, // Lesser Healing Potion
}

// archerKiteTypeHook wires the hunt loop of the session to the
// archer bot type: the same one-line wiring the config launch of
// cmd/swarm applies to an archer slot of the fleet (the type
// registry maps the "archer" type to the bow gear profile; the
// profile owns the weapon preference, the quiver keeping and the
// bow-aware fight radii of the loop).
func archerKiteTypeHook(loop *hunt.Loop) {
    loop.SetGearProfile(gear.Archer{})
}

// archerKiteDuration reads the SWARM_ARCHER_KITE_MINUTES env and
// returns the scenario window. A missing or invalid value falls
// back to the smoke default; a zero or negative value also falls
// back so an accidental empty string never produces a zero-length
// run.
func archerKiteDuration() time.Duration {
    minutes := archerKiteDefaultMinutes
    if raw := os.Getenv("SWARM_ARCHER_KITE_MINUTES"); raw != "" {
        if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
            minutes = parsed
        }
    }

    return time.Duration(minutes) * time.Minute
}

// archerKiteTimeout computes the test timeout from the window: the
// window itself plus the startup (the character ensure, the state
// injection, the login dial and the world entry) and the shutdown
// grace. The manager adds its own restartWait on top, so this bound
// only needs to cover the honest run.
func archerKiteTimeout() time.Duration {
    return archerKiteDuration() + 2*ensurePause + onlineWait +
        archerKiteShutdownGrace
}

// The archer kite check ids: the narrative of the contract - the
// type armed the bow, the fight stays at the bow radii, the kite
// steps answer the closing mobs, the bot survives the window and
// the quiver never runs dry.
const (
    checkArcherKiteArmed    = "armed"
    checkArcherKiteRange    = "range"
    checkArcherKiteRetreats = "retreats"
    checkArcherKiteSurvives = "survives"
    checkArcherKiteQuiver   = "quiver"
)

// archerKiteChecks is the initial check list of the archer
// scenario.
func archerKiteChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID:    checkArcherKiteArmed,
            Label: "wears the bow and carries the quiver",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteRange,
            Label: "fights at the bow radii",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteRetreats,
            Label: "retreats when the mobs close",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteSurvives,
            Label: "stays online the window, the HP never bottoms",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteQuiver,
            Label: "the arrow stock never empties",
            Done:  false, Detail: "",
        },
    }
}

// The kite telemetry mirrors of the hunt loop constants (the
// originals are unexported in the hunt package; the values are the
// contract the scenario observes, the comments name their source).
const (
    // archerKiteMeleeRadius is the melee engage radius (hunt
    // userEngageRadius): a fight sample beyond it is a ranged
    // sample.
    archerKiteMeleeRadius = 150.0
    // archerKiteBowBand is the bow stall radius (hunt
    // userBowStallRadius): a healthy standing bow fight stays
    // inside it.
    archerKiteBowBand = 650.0
    // archerKiteRangedMargin lifts the ranged sample boundary over
    // the melee radius: the broadcast lag of the last-known
    // positions moves the observed distance by tens of units.
    archerKiteRangedMargin = 50.0
    // archerKiteBandSlack widens the bow band ceiling for the same
    // broadcast lag (a sample one hop behind a running mob).
    archerKiteBandSlack = 130.0
    // archerKiteMinFightSamples is the evidence floor: the range
    // verdict only forms once the window observed enough fights.
    archerKiteMinFightSamples = 5
    // archerKiteRangedShare is the ranged half of the verdict: at
    // least half the fight samples must sit beyond the melee
    // radius (a cornered hold is the honest edge, not the norm).
    archerKiteRangedShare = 2
    // archerKiteMinKiteSteps is the retreat evidence floor: a
    // repeated pattern, not a single blip.
    archerKiteMinKiteSteps = 2
    // archerKiteHPFloor is the "bottomed" boundary of the survival
    // check: below it the kiting failed to keep the character out
    // of the killing range.
    archerKiteHPFloor = 10.0
)

// The kite log line markers of the hunt loop (kite.go): the step
// line narrates every retreat hop, the hold lines narrate the
// cornered and the surrounded standing fights.
const (
    kiteStepMark = "kiting clear (step"
    kiteHoldMark = "holding ground and shooting"
)

// archerKiteLog wraps the test log line callback with the kite
// telemetry counter: every hunt log line still flows to the test
// log (and through it to the botlog run file), while the wrapper
// counts the kite step and hold lines the session goroutine emits.
// The counter is mutex-guarded because the monitor goroutine reads
// it while the session goroutine writes it.
type archerKiteLog struct {
    mu    sync.Mutex
    next  func(string)
    steps int
    holds int
}

// newArcherKiteLog arms the wrapper around the base log line
// callback.
func newArcherKiteLog(next func(string)) *archerKiteLog {
    return &archerKiteLog{next: next}
}

// line is the logLine callback of the session: it counts the kite
// markers and forwards the line unchanged.
func (l *archerKiteLog) line(line string) {
    l.mu.Lock()
    if strings.Contains(line, kiteStepMark) {
        l.steps++
    }
    if strings.Contains(line, kiteHoldMark) {
        l.holds++
    }
    l.mu.Unlock()
    l.next(line)
}

// counts reads the kite telemetry counters.
func (l *archerKiteLog) counts() (steps int, holds int) {
    l.mu.Lock()
    defer l.mu.Unlock()

    return l.steps, l.holds
}

// archerKiteSample is one monitor observation of the live tracker:
// the pure half of the evaluation (the fold and the verdicts run
// without the tracker, the unit tests pin them against synthetic
// samples).
type archerKiteSample struct {
    // online is the session status at the sample moment.
    online bool
    // dead is the death flag of the character.
    dead bool
    // bowWorn and quiverWorn are the paperdoll state: the right
    // hand holds the bow (the weapon kind answer) and the left
    // hand holds the arrow stack.
    bowWorn    bool
    quiverWorn bool
    // fighting is true when the auto attack runs against a live
    // fight target (the swings are in flight).
    fighting bool
    // fightDist is the distance to the fight target, -1 when the
    // sample carries no fight.
    fightDist float64
    // hpPercent is the health percentage, -1 before the first
    // vitals answer.
    hpPercent float64
    // arrows is the total arrow count of the inventory (the worn
    // quiver stack plus the bagged spare piles), -1 before the
    // first inventory answer.
    arrows int32
}

// archerKiteWatch is the latched observation state of the window:
// the monitor folds one sample per period into it and the verdicts
// read the accumulated evidence.
type archerKiteWatch struct {
    bowSeen       bool
    quiverSeen    bool
    fightSamples  int
    rangedSamples int
    maxFightDist  float64
    kiteSteps     int
    kiteHolds     int
    minHP         float64
    minArrows     int32
    sawOffline    bool
    died          bool
}

// newArcherKiteWatch arms the watch with the neutral start state:
// the minimums latch down from the full values, the arrow floor
// only forms once the first inventory answer lands.
func newArcherKiteWatch() archerKiteWatch {
    return archerKiteWatch{minHP: 100, minArrows: -1}
}

// fold latches one sample into the watch. The pure core of the
// evaluation: no tracker, no clock, no side effects.
func (w archerKiteWatch) fold(s archerKiteSample) archerKiteWatch {
    if !s.online {
        w.sawOffline = true
    }
    if s.dead {
        w.died = true
    }
    if s.bowWorn {
        w.bowSeen = true
    }
    if s.quiverWorn {
        w.quiverSeen = true
    }
    if s.fighting && s.fightDist >= 0 {
        w.fightSamples++
        if s.fightDist >= archerKiteMeleeRadius+archerKiteRangedMargin {
            w.rangedSamples++
        }
        if s.fightDist > w.maxFightDist {
            w.maxFightDist = s.fightDist
        }
    }
    if s.hpPercent >= 0 && s.hpPercent < w.minHP {
        w.minHP = s.hpPercent
    }
    if s.arrows >= 0 {
        if w.minArrows < 0 || s.arrows < w.minArrows {
            w.minArrows = s.arrows
        }
    }

    return w
}

// archerKiteVerdicts is the per-check outcome of the watch: done
// carries the pass state, detail narrates the evidence.
type archerKiteVerdicts struct {
    armed    bool
    armedD   string
    ranged   bool
    rangedD  string
    retreats bool
    retreatD string
    survives bool
    surviveD string
    quiver   bool
    quiverD  string
}

// verdicts reads the latched evidence into the check outcomes. The
// range verdict only forms on the evidence floor: a window without
// fights holds the check open (the detail says why), a window of
// fights inside the band with the ranged majority passes it, a
// window of melee samples (the archer tanking instead of kiting)
// fails it. The quiver verdict holds from the first inventory
// answer: the stock never emptied. The survival verdict is the
// conjunction of the online hold, the death flag and the HP floor.
func (w archerKiteWatch) verdicts() archerKiteVerdicts {
    v := archerKiteVerdicts{
        armedD: "no bow in hand yet",
        rangedD: fmt.Sprintf("%d fight samples so far",
            w.fightSamples),
        retreatD: fmt.Sprintf("%d kite steps, %d holds observed",
            w.kiteSteps, w.kiteHolds),
        surviveD: "the window is running",
        quiverD:  "the inventory is not known yet",
    }

    if w.bowSeen && w.quiverSeen {
        v.armed = true
        v.armedD = "the bow is worn, the quiver is on"
    }

    if w.fightSamples >= archerKiteMinFightSamples {
        inBand := w.maxFightDist <= archerKiteBowBand+archerKiteBandSlack
        ranged := w.rangedSamples*archerKiteRangedShare >=
            w.fightSamples
        v.ranged = inBand && ranged
        share := 0
        if w.fightSamples > 0 {
            share = w.rangedSamples * 100 / w.fightSamples
        }
        v.rangedD = fmt.Sprintf(
            "%d samples, %d%% ranged, max %.0f units",
            w.fightSamples, share, w.maxFightDist)
    }

    v.retreats = w.kiteSteps >= archerKiteMinKiteSteps

    if w.sawOffline || w.died {
        reason := "the character died"
        if w.sawOffline {
            reason = "the online status dropped"
        }
        v.surviveD = reason + " inside the window"
    } else {
        v.survives = w.minHP >= archerKiteHPFloor
        v.surviveD = fmt.Sprintf("min HP %.0f%%",
            math.Round(w.minHP))
    }

    if w.minArrows >= 0 {
        v.quiver = w.minArrows > 0
        v.quiverD = fmt.Sprintf("min %d arrows in stock",
            w.minArrows)
    }

    return v
}

// archerKiteReset returns the start state of the archer scenario:
// the level 7 elven fighter with the vitals of the gain table, the
// bottom of the level experience span, the empty wallet and skill
// points and the bow kit in the bag, waking on the Kaboo Orc Grunt
// SW-5 cell focus.
func archerKiteReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   archerKiteStartLevel,
        Exp:     soakExperienceTable[archerKiteStartLevel-1],
        SP:      0,
        Adena:   0,
        Items:   archerKiteItems,
        X:       archerKiteSpawnX,
        Y:       archerKiteSpawnY,
        Z:       archerKiteSpawnZ,
        MaxHP:   archerKiteStartHP,
        MaxMP:   archerKiteStartMP,
        MaxCP:   archerKiteStartCP,
    }
}

// archerKiteSampleOf extracts one sample from the live tracker: the
// session status, the death flag, the paperdoll state (the bow kind
// of the right hand, the left hand occupancy), the fight distance
// (the self position against the fight target position while the
// swings run), the health percentage and the total arrow count of
// the inventory (the worn quiver stack plus the bagged piles).
func archerKiteSampleOf(tracker *state.Bot) archerKiteSample {
    sample := archerKiteSample{
        online:    tracker.Status() == state.StatusOnline,
        dead:      tracker.SelfDead(),
        fightDist: -1,
        hpPercent: -1,
        arrows:    -1,
    }
    if kind, ok := tracker.SelfWeaponKind(); ok && kind == "BOW" {
        sample.bowWorn = true
    }
    paperdoll := tracker.PaperdollSlotObjectIDs()
    if paperdoll[state.PaperdollLHand] != 0 {
        sample.quiverWorn = true
    }
    if percent := tracker.SelfHealthPercent(); percent >= 0 {
        sample.hpPercent = percent
    }
    snapshot := tracker.Snapshot()
    for _, item := range snapshot.Inventory {
        if item.ItemID == woodenArrowItemID {
            if sample.arrows < 0 {
                sample.arrows = 0
            }
            sample.arrows += item.Count
        }
    }
    target := snapshot.Diagnostics.FightingTargetID
    if target != 0 && snapshot.Diagnostics.AutoAttacking {
        sample.fighting = true
        x, y, _, ok := tracker.SelfPosition()
        tx, ty, _, tok := tracker.ObjectPosition(target)
        if ok && tok {
            sample.fightDist = math.Hypot(
                float64(tx-x), float64(ty-y))
        }
    }

    return sample
}

// woodenArrowItemID is the ammo the archer scenarios count: the
// Wooden Arrow of the elven shops (the quiver standard the gear
// package's arrowItemID resolves on the live catalog; the id is a
// constant of the data pack).
const woodenArrowItemID = int32(17)

// archerKiteMonitor runs the check evaluation until the deadline or
// the context end. The monitor owns the watch (single goroutine):
// every period it folds one tracker sample and the kite log counter
// into it and rewrites the check list from the verdicts. Every exit
// path first sends the accumulated watch on the buffered channel
// (the send never blocks) so the awaiter always reads the real
// evidence, never the zero value of a bare close.
func archerKiteMonitor(
    ctx context.Context, tracker *state.Bot, test *Test,
    kiteLog *archerKiteLog, deadline time.Time,
    done chan<- archerKiteWatch,
) {
    watch := newArcherKiteWatch()
    defer func() {
        done <- watch
        close(done)
    }()
    for {
        if ctx.Err() != nil {
            return
        }
        watch = watch.fold(archerKiteSampleOf(tracker))
        watch.kiteSteps, watch.kiteHolds = kiteLog.counts()
        applyArcherKiteVerdicts(test, watch.verdicts())
        if time.Now().After(deadline) {
            test.appendLog("acceptance: the archer kite window " +
                "ended, stopping the bot")

            return
        }
        select {
        case <-ctx.Done():
            return
        case <-time.After(monitorPeriod):
        }
    }
}

// applyArcherKiteVerdicts rewrites the check list of the archer
// scenario from the verdicts of the watch.
func applyArcherKiteVerdicts(test *Test, v archerKiteVerdicts) {
    test.updateCheck(checkArcherKiteArmed, v.armed, v.armedD)
    test.updateCheck(checkArcherKiteRange, v.ranged, v.rangedD)
    test.updateCheck(checkArcherKiteRetreats, v.retreats, v.retreatD)
    test.updateCheck(checkArcherKiteSurvives, v.survives,
        v.surviveD)
    test.updateCheck(checkArcherKiteQuiver, v.quiver, v.quiverD)
}

// archerKiteScenario runs the archer acceptance vehicle: the temp
// character resets onto the Kaboo Orc hunting cell with the bow kit
// in the bag, the session launches as the archer type of the fleet
// (the gear profile hook of the session runner), the auto equipment
// dresses the kit, the quiver arms and the bot farms the window
// under the kiting contract while the monitor folds the telemetry
// into the checks. The final verdict is the conjunction of the
// contract checks: a failure names the broken half.
func archerKiteScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(archerKiteChecks())

    if err := m.ensureCharacter(archerKiteAccount,
        archerKitePassword, archerKiteAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(archerKiteReset(archerKiteAccount),
        test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    defer cancelSession()
    sessionDone := make(chan error, 1)
    kiteLog := newArcherKiteLog(test.appendLog)
    go func() {
        m.runSessionSupervised(sessionCtx, archerKiteAccount,
            archerKitePassword, archerKiteAccount, m.proxy,
            kiteLog.line, archerKiteTypeHook)
        sessionDone <- nil
    }()
    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        cancelSession()
        <-sessionDone

        return fmt.Errorf("world entry: %w", err)
    }
    test.appendLog("acceptance: the archer is online, watching " +
        "the kite window of " + archerKiteDuration().String())

    deadline := time.Now().Add(archerKiteDuration())
    monitorCtx, cancelMonitor := context.WithCancel(ctx)
    defer cancelMonitor()
    monitorDone := make(chan archerKiteWatch, 1)
    go archerKiteMonitor(monitorCtx, tracker, test, kiteLog,
        deadline, monitorDone)

    return archerKiteAwaitOutcome(ctx, test, cancelSession,
        sessionDone, cancelMonitor, monitorDone)
}

// archerKiteAwaitOutcome waits for the monitor verdict or the
// context end, then tears the session down and answers with the
// contract verdict: nil when every check holds, an error naming the
// broken halves otherwise.
func archerKiteAwaitOutcome(
    ctx context.Context, test *Test, cancelSession context.CancelFunc,
    sessionDone chan error, cancelMonitor context.CancelFunc,
    monitorDone chan archerKiteWatch,
) error {
    var watch archerKiteWatch
    select {
    case <-ctx.Done():
        cancelMonitor()
        <-monitorDone
        cancelSession()
        <-sessionDone

        return fmt.Errorf("cancelled: %w", ctx.Err())
    case watch = <-monitorDone:
    }
    if ctx.Err() != nil {
        cancelSession()
        <-sessionDone

        return fmt.Errorf("cancelled: %w", ctx.Err())
    }
    cancelMonitor()
    <-monitorDone
    cancelSession()
    <-sessionDone

    verdicts := watch.verdicts()
    applyArcherKiteVerdicts(test, verdicts)
    test.appendLog("acceptance: the kite telemetry - " +
        fmt.Sprintf("%d kite steps, %d cornered holds, %d fight "+
            "samples (%d ranged), min HP %.0f%%, min arrows %d",
            watch.kiteSteps, watch.kiteHolds, watch.fightSamples,
            watch.rangedSamples, math.Round(watch.minHP),
            watch.minArrows))

    var broken []string
    if !verdicts.armed {
        broken = append(broken, "the bow never armed")
    }
    if !verdicts.ranged {
        broken = append(broken, "the fight left the bow radii")
    }
    if !verdicts.retreats {
        broken = append(broken, "the kite never stepped")
    }
    if !verdicts.survives {
        broken = append(broken, "the survival broke: "+
            verdicts.surviveD)
    }
    if !verdicts.quiver {
        broken = append(broken, "the quiver ran dry")
    }
    if len(broken) > 0 {
        return errors.New("the archer kite contract failed: " +
            strings.Join(broken, "; "))
    }

    return nil
}
