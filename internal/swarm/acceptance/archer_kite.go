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
// type armed the bow, the shots follow the bow cooldown, the
// standing fight stays inside the bow band, the kite steps answer
// the closing mobs, the fights resolve, the bot survives the window
// and the quiver never runs dry.
const (
    checkArcherKiteArmed    = "armed"
    checkArcherKiteCadence  = "cadence"
    checkArcherKiteRange    = "range"
    checkArcherKiteRetreats = "retreats"
    checkArcherKiteKills    = "kills"
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
            ID:    checkArcherKiteCadence,
            Label: "shoots as soon as the bow is ready",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteRange,
            Label: "fights stay inside the bow band",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteRetreats,
            Label: "retreats when the mobs close",
            Done:  false, Detail: "",
        },
        {
            ID:    checkArcherKiteKills,
            Label: "the fights resolve",
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
    // archerKiteBowBand is the bow stall radius (hunt
    // userBowStallRadius): a healthy standing bow fight stays
    // inside it. The band is the drift fence of the range check,
    // NOT a ranged-majority demand: the shot-cycle contract of the
    // 2026-11-09 round deliberately fights at melee between the
    // shots (the windup standstill lets the chaser close), so the
    // range verdict only rejects a fight that ran past the stall
    // radius.
    archerKiteBowBand = 650.0
    // archerKiteBandSlack widens the bow band ceiling for the same
    // broadcast lag (a sample one hop behind a running mob).
    archerKiteBandSlack = 130.0
    // archerKiteMinFightSamples is the evidence floor: the range
    // verdict only forms once the window observed enough fights.
    archerKiteMinFightSamples = 5
    // archerKiteShotGap is the healthy same-fight gap between two
    // self shot broadcasts: the C1 bow disable window on the live
    // pAtkSpd (~2.97 s at 337) plus the tick and broadcast slack.
    // The old safe-distance gate measured 20-30 s gaps against the
    // same-speed chasers; the shot-cycle contract fires the next
    // shot at the disable end whatever range the chase reached.
    archerKiteShotGap = 4 * time.Second
    // archerKiteMinShotGaps is the evidence floor of the cadence
    // verdict: a 5-minute window of a running fight measured ~35
    // same-fight gaps, the floor keeps the verdict open on a window
    // that barely fought.
    archerKiteMinShotGaps = 6
    // archerKiteFastGapPermille is the majority share of the
    // same-fight gaps that must sit inside archerKiteShotGap: three
    // quarters leaves room for the first shot of a re-engage while
    // the treadmill (20-30 s) fails every gap.
    archerKiteFastGapPermille = 750
    // archerKiteMinKiteSteps is the retreat evidence floor: a
    // repeated pattern, not a single blip.
    archerKiteMinKiteSteps = 2
    // archerKiteHPFloor is the "bottomed" boundary of the survival
    // check: below it the kiting failed to keep the character out
    // of the killing range.
    archerKiteHPFloor = 10.0
)

// archerKiteMinKills is the kill floor of the window: the shot
// cycle exists to RESOLVE fights (the safe-distance treadmill fired
// arrows while the mob regenerated and killed nothing), so a window
// that resolves no fight fails regardless of the movement telemetry.
// The floor scales at one kill per two minutes, never under one.
func archerKiteMinKills(window time.Duration) int {
    kills := int(window / (2 * time.Minute))
    if kills < 1 {
        kills = 1
    }

    return kills
}

// The kite log line markers of the hunt loop (kite.go): the step
// lines narrate every retreat hop - the proximity step ("kiting
// clear") and the shot-paced rhythm step ("kiting the reload", the
// issue #60 layer that fires at the own Attack broadcast and owns
// the majority of the retreats since it merged) - and the hold
// lines narrate the cornered and the surrounded standing fights.
const (
    kiteStepMark = "kiting clear (step"
    kiteShotMark = "kiting the reload"
    kiteHoldMark = "holding ground and shooting"
)

// archerKiteLog wraps the test log line callback with the kite
// telemetry counter: every hunt log line still flows to the test
// log (and through it to the botlog run file), while the wrapper
// counts the kite step, hold, shot-cycle retreat and kill lines the
// session goroutine emits. The counter is mutex-guarded because the
// monitor goroutine reads it while the session goroutine writes it.
type archerKiteLog struct {
    mu     sync.Mutex
    next   func(string)
    steps  int
    holds  int
    reload int
    kills  int
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
    if strings.Contains(line, kiteStepMark) ||
        strings.Contains(line, kiteShotMark) {
        l.steps++
    }
    if strings.Contains(line, kiteShotMark) {
        // The shot-cycle retreat line: the deferred click at the
        // windup end of the just-released shot.
        l.reload++
    }
    if strings.Contains(line, kiteHoldMark) {
        l.holds++
    }
    if strings.Contains(line, "Hunt: target") &&
        strings.Contains(line, "died, looting") {
        l.kills++
    }
    l.mu.Unlock()
    l.next(line)
}

// counts reads the kite telemetry counters.
func (l *archerKiteLog) counts() (
    steps, holds, reloads, kills int,
) {
    l.mu.Lock()
    defer l.mu.Unlock()

    return l.steps, l.holds, l.reload, l.kills
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
    // walking is true when the character is in motion at the
    // sample moment. A retreat leg legitimately opens the distance
    // past the bow band while the chaser lags behind, so a walking
    // sample cannot judge the standing fight radius.
    walking bool
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
    // shotAt is the timestamp of the last self Attack broadcast,
    // zero before the first shot of the session.
    shotAt time.Time
    // fightTarget is the auto-attack target at the sample moment,
    // zero when no fight runs.
    fightTarget int32
}

// archerKiteWatch is the latched observation state of the window:
// the monitor folds one sample per period into it and the verdicts
// read the accumulated evidence.
type archerKiteWatch struct {
    bowSeen      bool
    quiverSeen   bool
    fightSamples int
    maxFightDist float64
    shotCount    int
    shotGaps     int
    fastShotGaps int
    lastShot     time.Time
    lastShotFor  int32
    reloadTails  int
    kills        int
    minKills     int
    kiteSteps    int
    kiteHolds    int
    minHP        float64
    minArrows    int32
    sawOffline   bool
    died         bool
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
        // Only a standing sample judges the bow band: while the
        // character walks a retreat leg the gap to the lagging
        // chaser grows past the band by design, and latching that
        // distance would fail the contract on the kite's own
        // working distance (the 2026-11-09 5-minute run measured
        // 796 units on a 780 ceiling while every retreat step was
        // healthy).
        if !s.walking && s.fightDist > w.maxFightDist {
            w.maxFightDist = s.fightDist
        }
    }
    // The shot cadence of the fight: a fresh self Attack broadcast
    // inside the same fight target records the gap from the previous
    // one. A target switch starts a new chain (the walk to the next
    // mob is not a slow shot), and the freshly killed target reads
    // zero before the next pick lands - both break the chain, never
    // fake a gap.
    w = w.foldShot(s)
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

// foldShot latches one fresh self Attack broadcast into the cadence
// chain of the watch: the same-fight gap counts between the previous
// shot and the fresh one, a target switch or a vanished target starts
// a new chain. The pure half of the shot evidence.
func (w archerKiteWatch) foldShot(
    s archerKiteSample,
) archerKiteWatch {
    if s.shotAt.IsZero() || !s.shotAt.After(w.lastShot) {
        return w
    }
    w.shotCount++
    if !w.lastShot.IsZero() && s.fightTarget != 0 &&
        s.fightTarget == w.lastShotFor {
        gap := s.shotAt.Sub(w.lastShot)
        w.shotGaps++
        if gap <= archerKiteShotGap {
            w.fastShotGaps++
        }
    }
    w.lastShot = s.shotAt
    if s.fightTarget != 0 {
        w.lastShotFor = s.fightTarget
    }

    return w
}

// archerKiteVerdicts is the per-check outcome of the watch: done
// carries the pass state, detail narrates the evidence.
type archerKiteVerdicts struct {
    armed    bool
    armedD   string
    cadence  bool
    cadenceD string
    band     bool
    bandD    string
    retreats bool
    retreatD string
    kills    bool
    killsD   string
    survives bool
    surviveD string
    quiver   bool
    quiverD  string
}

// verdicts reads the latched evidence into the check outcomes. The
// cadence verdict only forms past its evidence floor: the majority
// of the same-fight shot gaps must sit inside the C1 bow cooldown
// (a treadmill window fires every 20-30 s and fails every gap). The
// band verdict only judges STANDING fight samples and only forms on
// its own floor - it is a drift fence, not a ranged-majority
// demand. The retreat and kill verdicts read the log counters and
// the scaled kill floor. The quiver verdict holds from the first
// inventory answer: the stock never emptied. The survival verdict
// is the conjunction of the online hold, the death flag and the HP
// floor.
func (w archerKiteWatch) verdicts() archerKiteVerdicts {
    floor := w.minKills
    if floor < 1 {
        floor = 1
    }
    v := archerKiteVerdicts{
        armedD: "no bow in hand yet",
        cadenceD: fmt.Sprintf("%d shot gaps so far",
            w.shotGaps),
        bandD: fmt.Sprintf("%d fight samples so far",
            w.fightSamples),
        retreatD: fmt.Sprintf("%d kite steps, %d holds observed",
            w.kiteSteps, w.kiteHolds),
        killsD:   fmt.Sprintf("%d kills of %d wanted", w.kills, floor),
        surviveD: "the window is running",
        quiverD:  "the inventory is not known yet",
    }

    if w.bowSeen && w.quiverSeen {
        v.armed = true
        v.armedD = "the bow is worn, the quiver is on"
    }

    if w.shotGaps >= archerKiteMinShotGaps {
        share := w.fastShotGaps * 100 / w.shotGaps
        v.cadence = w.fastShotGaps*1000 >=
            w.shotGaps*archerKiteFastGapPermille
        v.cadenceD = fmt.Sprintf(
            "%d shots, %d%% of the gaps inside the %.0fs cooldown",
            w.shotCount, share, archerKiteShotGap.Seconds())
    }

    if w.fightSamples >= archerKiteMinFightSamples {
        v.band = w.maxFightDist <= archerKiteBowBand+archerKiteBandSlack
        v.bandD = fmt.Sprintf("%d standing samples, max %.0f units",
            w.fightSamples, w.maxFightDist)
    }

    v.retreats = w.kiteSteps >= archerKiteMinKiteSteps
    v.retreatD = fmt.Sprintf(
        "%d kite steps (%d shot-cycle), %d holds observed",
        w.kiteSteps, w.reloadTails, w.kiteHolds)

    v.kills = w.kills >= floor

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
        walking:   tracker.SelfWalking(),
        fightDist: -1,
        hpPercent: -1,
        arrows:    -1,
        shotAt:    tracker.SelfLastShotAt(),
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
    sample.fightTarget = target
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
    watch.minKills = archerKiteMinKills(time.Until(deadline))
    defer func() {
        done <- watch
        close(done)
    }()
    for {
        if ctx.Err() != nil {
            return
        }
        watch = watch.fold(archerKiteSampleOf(tracker))
        steps, holds, reloads, kills := kiteLog.counts()
        watch.kiteSteps, watch.kiteHolds = steps, holds
        watch.reloadTails, watch.kills = reloads, kills
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
    test.updateCheck(checkArcherKiteCadence, v.cadence, v.cadenceD)
    test.updateCheck(checkArcherKiteRange, v.band, v.bandD)
    test.updateCheck(checkArcherKiteRetreats, v.retreats, v.retreatD)
    test.updateCheck(checkArcherKiteKills, v.kills, v.killsD)
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
    sessionDone := make(chan struct{}, 1)
    kiteLog := newArcherKiteLog(test.appendLog)
    go func() {
        m.runSessionSupervised(sessionCtx, archerKiteAccount,
            archerKitePassword, archerKiteAccount, m.proxy,
            kiteLog.line, archerKiteTypeHook)
        sessionDone <- struct{}{}
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
    sessionDone chan struct{}, cancelMonitor context.CancelFunc,
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
        fmt.Sprintf("%d shots, %d shot gaps (%d inside the "+
            "cooldown), %d kite steps (%d shot-cycle), %d holds, "+
            "%d kills, %d fight samples, min HP %.0f%%, min "+
            "arrows %d",
            watch.shotCount, watch.shotGaps, watch.fastShotGaps,
            watch.kiteSteps, watch.reloadTails, watch.kiteHolds,
            watch.kills, watch.fightSamples,
            math.Round(watch.minHP), watch.minArrows))

    broken := archerKiteBroken(verdicts)
    if len(broken) > 0 {
        return errors.New("the archer kite contract failed: " +
            strings.Join(broken, "; "))
    }

    return nil
}

// archerKiteBroken names every failed half of the archer contract in
// the narrative order of the checks. The split keeps the scenario
// teardown readable and the verdict mapping in one place.
func archerKiteBroken(v archerKiteVerdicts) []string {
    var broken []string
    if !v.armed {
        broken = append(broken, "the bow never armed")
    }
    if !v.cadence {
        broken = append(broken, "the shots left the bow cooldown")
    }
    if !v.band {
        broken = append(broken, "the fight left the bow band")
    }
    if !v.retreats {
        broken = append(broken, "the kite never stepped")
    }
    if !v.kills {
        broken = append(broken, "the fights never resolved")
    }
    if !v.survives {
        broken = append(broken, "the survival broke: "+v.surviveD)
    }
    if !v.quiver {
        broken = append(broken, "the quiver ran dry")
    }

    return broken
}
