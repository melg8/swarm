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
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The archer fleet acceptance scenario (issue #70, the mass kite
// audit): five archer bots wake at five different cells of the elven
// Kaboo ground and farm a short window under the improved kite
// contract. The scenario answers WHICH of the five kite behaviors the
// fleet does NOT implement today - holding the maximum distance,
// shooting, regaining the distance before the reload ends, re-shooting
// the moment the distance is back and retreating on a curve that stays
// near the farm point - per bot and for the fleet as a whole.

// archerFleetScenarioID is the scenario registry id.
const archerFleetScenarioID = "archer-fleet"

// archerFleetDefaultMinutes is the live window of the fleet audit:
// the whole scenario (five ensures, five resets, the staggered logins,
// the window, the teardown) must land under five minutes on the live
// stack, and the window itself is the observed fight time. The
// measured overhead of the launch and the teardown is ~25 seconds,
// so a 3 minute window rides ~3.4 minutes wall-clock - the extra
// half minute over the old 2.5 buys the per-fight corner evidence
// the curving verdict needs (the sparse-fight slots of the 2.5
// minute rounds closed with a single corner against the floor of
// two - more fights, more corners, the same honest attribution).
const archerFleetDefaultMinutes = 3.0

// archerFleetShutdownGrace bounds the teardown of the five sessions.
const archerFleetShutdownGrace = 45 * time.Second

// fleetOnlineBudget bounds the staggered five-slot launch inside the
// fleet timeout: the measured launch lands the fifth slot ~13
// seconds after the scenario start, so the budget rides three times
// the measurement (the entry latch keeps a slot that enters late
// harmless - it samples less, the verdict names it). The shared
// onlineWait (90 s, the single-bot pathological-stack bound) would
// push the three minute window past the five minute ask.
const fleetOnlineBudget = 40 * time.Second

// archerFleetSlotDesc is one fleet slot: the temp account and the
// spawn cell focus the archer wakes on.
type archerFleetSlotDesc struct {
    Account string
    Cell    string
    X       int32
    Y       int32
}

// archerFleetSlots is the five-cell spread of the fleet: the mass 3+
// Kaboo Orc cells of the south-west elven ground, far enough apart
// that five hunt loops never share a mob pool, close enough to share
// one terrain sheet (the spawn Z oracle stays honest).
var archerFleetSlots = []archerFleetSlotDesc{
    {Account: "temp20", Cell: "elven-hex-104", X: 36000, Y: 50229},
    {Account: "temp21", Cell: "elven-hex-106", X: 36000, Y: 51962},
    {Account: "temp22", Cell: "elven-hex-105", X: 34500, Y: 49363},
    {Account: "temp23", Cell: "elven-hex-086", X: 39000, Y: 50229},
    {Account: "temp24", Cell: "elven-hex-087", X: 37500, Y: 49363},
}

// archerFleetPassword answers the temp account password (the account
// equals the password, the Mobius login auto-creates missing ones).
func archerFleetPassword(account string) string {
    return account
}

// The fleet telemetry thresholds (the mirrors of the hunt kite
// constants and the server bow cycle): the fight distance floor of the
// "maximum distance" behavior, the retreat lag ceiling of the "regain
// the distance early" behavior (the server defers a click inside the
// windup to the cycle end, so a lag under the ceiling means the click
// rode the accepted window), the re-shot gap ceiling, the per-fight
// drift leash of the curving retreat and the evidence floors.
const (
    fleetFightDistFloor  = 250.0
    fleetRetreatLagCeil  = 2200 * time.Millisecond
    fleetReshotGapCeil   = 1500 * time.Millisecond
    fleetFightDriftLeash = 1500.0
    fleetMinShots        = 3
    fleetMinRetreats     = 2
    fleetMinFightSamples = 4
    fleetTurnAngle       = 40.0
    fleetMinTurns        = 2
    fleetSamplePeriod    = 250 * time.Millisecond
)

// The fleet check ids: one per audited kite behavior plus the online
// gate - the matrix the scenario prints names the missing ones.
const (
    checkFleetMaxRange = "max-range"
    checkFleetShoots   = "shoots"
    checkFleetEarly    = "early-retreat"
    checkFleetReshot   = "quick-reshot"
    checkFleetCurve    = "curved-retreat"
)

// archerFleetChecks is the initial check list of the fleet scenario.
func archerFleetChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID:    checkFleetMaxRange,
            Label: "holds the fight at the maximum bow distance",
            Done:  false, Detail: "",
        },
        {
            ID:    checkFleetShoots,
            Label: "shoots the bow through the window",
            Done:  false, Detail: "",
        },
        {
            ID:    checkFleetEarly,
            Label: "regains distance before the reload ends",
            Done:  false, Detail: "",
        },
        {
            ID:    checkFleetReshot,
            Label: "re-shoots once the distance is back",
            Done:  false, Detail: "",
        },
        {
            ID:    checkFleetCurve,
            Label: "retreats on a curve, stays near the farm point",
            Done:  false, Detail: "",
        },
    }
}

// archerFleetDuration reads the SWARM_ARCHER_FLEET_MINUTES env and
// returns the fleet window.
func archerFleetDuration() time.Duration {
    minutes := archerFleetDefaultMinutes
    if raw := os.Getenv("SWARM_ARCHER_FLEET_MINUTES"); raw != "" {
        if parsed, err := strconv.ParseFloat(raw, 64); err == nil &&
            parsed > 0 {
            minutes = parsed
        }
    }

    return time.Duration(minutes * float64(time.Minute))
}

// archerFleetTimeout computes the scenario timeout: the window plus
// the startup and the teardown, kept under the five minute ask (the
// three minute window rides the fleet's own measured launch budget -
// the shared 90 s online wait belongs to the single-bot scenarios).
func archerFleetTimeout() time.Duration {
    return archerFleetDuration() + 2*ensurePause +
        fleetOnlineBudget + archerFleetShutdownGrace
}

// The fleet vitals (measured on the live stack): the server
// recomputes the level 7 elven fighter maxima at login (the class
// formula answers 214 HP / 82 MP whatever the reset row says), and a
// curHp below that lands as a WOUNDED start - the fleet round of
// 2026-09-23 measured the cost (a slot entering at 167 of 214 spent
// the window on potions, an emergency logout and a 60 second sit:
// every kite check of the slot read as not passing with zero
// evidence). The audit measures the five kite behaviors, not
// wounded survival - the slots wake at the full server-computed
// vitals so every check reads real behavior.
const (
    fleetLevel7MaxHP = 214
    fleetLevel7MaxMP = 82
)

// archerFleetReset returns the start state of one fleet slot: the
// proven archer kit of the archer scenario woken on the slot focus
// at the full level 7 vitals (the single-bot archer scenario keeps
// its own gain-table start).
func archerFleetReset(slot archerFleetSlotDesc, z int32,
) characterReset {
    reset := archerKiteReset(slot.Account)
    reset.X = slot.X
    reset.Y = slot.Y
    reset.Z = z
    reset.MaxHP = fleetLevel7MaxHP
    reset.MaxMP = fleetLevel7MaxMP

    return reset
}

// fleetSpawnZ answers the walkable height of a spawn focus through
// the geodata click oracle: a short approach click onto the focus
// snaps to the layer the server would walk. A missing engine or an
// out-of-band answer falls back to the proven archer cell height.
func fleetSpawnZ(engine *pathfind.Engine, x, y int32) int32 {
    if engine != nil {
        from := pathfind.Vec3{
            X: float64(x) + 250, Y: float64(y), Z: 0,
        }
        to := pathfind.Vec3{X: float64(x), Y: float64(y), Z: 0}
        if snapped, ok := engine.ValidateClick(from, to); ok &&
            snapped.Z > -4200 && snapped.Z < -2200 {
            return int32(snapped.Z)
        }
    }

    return archerKiteSpawnZ
}

// fleetSample is one observation of one fleet bot: the 250 ms monitor
// tick latches the movement state, the last own shot stamp, the
// position, the fight target and its ground position, and the health.
type fleetSample struct {
    at        time.Time
    walking   bool
    shotAt    time.Time
    hasPos    bool
    x, y      int32
    fighting  bool
    targetID  int32
    fightDist float64
    hasTarget bool
    tx, ty    int32
    hpPct     float64
}

// fleetWatch folds one bot's samples into the behavior evidence: the
// shot count, the retreat lags (the movement start after a shot), the
// re-shot gaps (the shot after a retreat walk ends), the fight
// distance distribution, the per-fight drift of the curving retreat
// and the retreat walk corners. The retreat metrics ride the
// DIRECTION gate: only a walk that moved away from the fight target
// it started in counts as a kite retreat - the loot pickups, the
// cell-rotation approaches and the chase-stall walks toward the
// target polluted the plain walk population of the first live round
// (a 3.1 s walk-end-to-shot median measured on walks the kite never
// issued).
type fleetWatch struct {
    shots       int
    retreatLags []time.Duration
    reshotGaps  []time.Duration
    fightDists  []float64
    // The per-fight curve attribution (the leash fix of the curve
    // round: the spawn-anchor displacement measured the hunt's own
    // cell rotation into the leash). The drift anchor is the fight
    // target's stand at the fight start - the farm point the circle
    // must hold - and only the fights with a confirmed retreat feed
    // the leash, so the approach walks and the cell hops between
    // the fights never reach the metric.
    fightTarget   int32
    fightAnchorX  int32
    fightAnchorY  int32
    fightMaxDrift float64
    fightRetreats int
    fightDrifts   []float64
    // The corner evidence of the curve: the signed angle between the
    // displacement vectors of two CONSECUTIVE confirmed retreats of
    // one fight. A server walk is a straight segment, so the curve
    // is exactly this polyline corner - the straight-line runaway
    // repeats its heading (a zero corner), the curving circle bends
    // every corner one way (the hunt's 70 degree bearing).
    lastRetreatVecX  float64
    lastRetreatVecY  float64
    lastRetreatFight int32
    haveLastRetreat  bool
    cornersCW        int
    cornersCCW       int
    // fold state
    lastShot    time.Time
    walkStarted time.Time
    walkEndedAt time.Time
    // the pending retreat candidate of the running walk: armed at
    // the walk start (the lag from the last shot), confirmed or
    // dropped at the walk end by the away-direction check.
    pendingLag     time.Duration
    havePendingLag bool
    walkFighting   bool
    walkTargetID   int32
    haveWalkFrom   bool
    walkFromX      int32
    walkFromY      int32
    walkTargetX    int32
    walkTargetY    int32
    walkShotAt     time.Time
}

// newFleetWatch arms the fold.
func newFleetWatch() fleetWatch {
    return fleetWatch{}
}

// fold latches one sample into the watch (the pure evaluation core).
func (w fleetWatch) fold(s fleetSample) fleetWatch {
    if !s.shotAt.IsZero() && s.shotAt != w.lastShot {
        w.shots++
        w.lastShot = s.shotAt
        if !w.walkEndedAt.IsZero() && s.shotAt.After(w.walkEndedAt) {
            gap := s.shotAt.Sub(w.walkEndedAt)
            if gap >= 0 {
                w.reshotGaps = append(w.reshotGaps, gap)
            }
            w.walkEndedAt = time.Time{}
        }
    }
    if s.walking && w.walkStarted.IsZero() {
        // The retreat candidate: the walk starts inside a live fight
        // (the deferred click of the shot cycle). The lag is pending
        // until the walk end confirms the direction - a chase-stall
        // walk toward the target starts in a live fight too.
        w.walkStarted = s.at
        w.walkFighting = s.fighting
        w.walkTargetID = s.targetID
        w.haveWalkFrom = s.hasPos
        w.walkFromX, w.walkFromY = s.x, s.y
        w.walkTargetX, w.walkTargetY = s.tx, s.ty
        w.walkShotAt = w.lastShot
        if !w.lastShot.IsZero() && s.at.After(w.lastShot) {
            w.pendingLag = s.at.Sub(w.lastShot)
            w.havePendingLag = true
        }
    }
    if !s.walking && !w.walkStarted.IsZero() {
        // The walk ended: the away-direction verdict gates the
        // pending retreat metrics. A walk whose displacement leans
        // away from the target it started against is the kite
        // retreat; anything else (a loot pickup, an approach of the
        // next pick, a chase walk toward the target) is fleet noise.
        if w.walkFighting && w.haveWalkFrom && s.hasPos {
            dx := float64(s.x - w.walkFromX)
            dy := float64(s.y - w.walkFromY)
            awayX := float64(w.walkFromX - w.walkTargetX)
            awayY := float64(w.walkFromY - w.walkTargetY)
            if dx*awayX+dy*awayY > 0 {
                if w.havePendingLag {
                    w.retreatLags = append(w.retreatLags, w.pendingLag)
                }
                if !s.shotAt.IsZero() && !w.walkShotAt.IsZero() &&
                    s.shotAt != w.walkShotAt {
                    // The re-shot itself ended the walk (the server
                    // stops the movement on the attack): the gap is
                    // the walk end to the interrupt shot, ~zero. The
                    // walkShotAt guard keeps a mid-walk attach (the
                    // fold never saw the owning shot) from minting a
                    // fake zero.
                    w.reshotGaps = append(w.reshotGaps, 0)
                } else {
                    w.walkEndedAt = s.at
                }
                w = w.foldRetreat(dx, dy)
            }
        }
        w.walkStarted = time.Time{}
        w.havePendingLag = false
        w.walkFighting = false
        w.walkTargetID = 0
        w.haveWalkFrom = false
    }
    if s.fighting && s.fightDist >= 0 {
        w.fightDists = append(w.fightDists, s.fightDist)
    }
    if s.fighting && s.hasTarget && s.targetID != 0 {
        w = w.foldFight(s)
    }

    return w
}

// foldRetreat books one confirmed retreat walk (the away-direction
// gate already passed): the walk's fight gains its retreat count, and
// the walk's unit displacement extends the corner chain of THAT
// fight - two consecutive confirmed retreats of one fight measure
// the polyline corner between their straight segments, the exact
// geometry of the curving retreat (a fixed-side bearing bends every
// corner one way; a straight-line runaway repeats its heading).
func (w fleetWatch) foldRetreat(dx, dy float64) fleetWatch {
    if w.walkTargetID != 0 {
        if w.walkTargetID == w.fightTarget {
            w.fightRetreats++
        }
        length := math.Hypot(dx, dy)
        if length >= 1 {
            unitX, unitY := dx/length, dy/length
            if w.haveLastRetreat &&
                w.lastRetreatFight == w.walkTargetID {
                // The signed rotation from the previous retreat
                // heading to this one: positive bends
                // counterclockwise, negative clockwise.
                dot := unitX*w.lastRetreatVecX +
                    unitY*w.lastRetreatVecY
                cross := w.lastRetreatVecX*unitY -
                    w.lastRetreatVecY*unitX
                angle := math.Atan2(cross, dot) * 180 / math.Pi
                if angle >= fleetTurnAngle {
                    w.cornersCCW++
                } else if angle <= -fleetTurnAngle {
                    w.cornersCW++
                }
            }
            w.lastRetreatVecX, w.lastRetreatVecY = unitX, unitY
            w.lastRetreatFight = w.walkTargetID
            w.haveLastRetreat = true
        }
    }

    return w
}

// foldFight tracks the per-fight drift of the curving retreat: the
// first fighting sample of a target latches the anchor on the MOB'S
// STAND (the farm point the fight opened on - not the spawn, whose
// displacement carries the hunt's own cell rotation), and every
// fighting sample of the same fight measures the character's
// distance from it. A fight switch (a new target id) closes the
// tracked fight, recording its max drift when the kite really
// retreated in it - the fights without a retreat (a one-shot kill,
// a chase the engage never opened the range on) carry no curve
// evidence and stay out of the leash.
func (w fleetWatch) foldFight(s fleetSample) fleetWatch {
    if w.fightTarget == 0 || s.targetID != w.fightTarget {
        w = w.closeFight()
        w.fightTarget = s.targetID
        w.fightAnchorX, w.fightAnchorY = s.tx, s.ty
        w.fightMaxDrift = 0
        w.fightRetreats = 0
    }
    if !s.hasPos {
        return w
    }
    drift := math.Hypot(float64(s.x-w.fightAnchorX),
        float64(s.y-w.fightAnchorY))
    if drift > w.fightMaxDrift {
        w.fightMaxDrift = drift
    }

    return w
}

// closeFight records the tracked fight's max drift when the kite
// retreated in it (the leash measures fights, not the whole window).
func (w fleetWatch) closeFight() fleetWatch {
    if w.fightTarget != 0 && w.fightRetreats > 0 {
        w.fightDrifts = append(w.fightDrifts, w.fightMaxDrift)
    }
    w.fightTarget = 0
    w.fightRetreats = 0
    w.fightMaxDrift = 0

    return w
}

// kiteFightDrifts answers the drifts of every kited fight, the
// closed ones plus the fight still open at the window's end.
func (w fleetWatch) kiteFightDrifts() []float64 {
    drifts := append([]float64{}, w.fightDrifts...)
    if w.fightTarget != 0 && w.fightRetreats > 0 {
        drifts = append(drifts, w.fightMaxDrift)
    }

    return drifts
}

// curveCorners answers the big corners that agree on one turn side:
// the curving retreat bends every corner the same way, so the
// majority side carries the evidence (a wall-bounce zigzag racks
// both sides and stays evidence).
func (w fleetWatch) curveCorners() int {
    if w.cornersCW > w.cornersCCW {
        return w.cornersCW
    }

    return w.cornersCCW
}

// medianDuration answers the median of a duration slice.
func medianDuration(values []time.Duration) time.Duration {
    if len(values) == 0 {
        return 0
    }
    sorted := append([]time.Duration{}, values...)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i] < sorted[j]
    })

    return sorted[len(sorted)/2]
}

// medianFloat answers the median of a float slice.
func medianFloat(values []float64) float64 {
    if len(values) == 0 {
        return 0
    }
    sorted := append([]float64{}, values...)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i] < sorted[j]
    })

    return sorted[len(sorted)/2]
}

// fleetVerdicts is the per-behavior outcome of one bot's watch.
type fleetVerdicts struct {
    maxRange  bool
    maxRangeD string
    shoots    bool
    shootsD   string
    early     bool
    earlyD    string
    reshot    bool
    reshotD   string
    curve     bool
    curveD    string
}

// verdicts reads the folded evidence into the behavior outcomes: each
// verdict holds only on its evidence floor (a bot that never fought
// keeps the checks open, the detail says why).
func (w fleetWatch) verdicts() fleetVerdicts {
    v := fleetVerdicts{
        maxRangeD: "no fight samples yet",
        shootsD:   fmt.Sprintf("%d shots observed", w.shots),
        earlyD: fmt.Sprintf("%d retreats observed",
            len(w.retreatLags)),
        reshotD: fmt.Sprintf("%d walk-end re-shots observed",
            len(w.reshotGaps)),
        curveD: fmt.Sprintf("%d kite fights, %d retreat corners"+
            " observed", len(w.fightDrifts), w.curveCorners()),
    }
    if w.shots >= fleetMinShots {
        v.shoots = true
        v.shootsD = fmt.Sprintf("%d shots in the window", w.shots)
    }
    if len(w.fightDists) >= fleetMinFightSamples {
        median := medianFloat(w.fightDists)
        v.maxRange = median >= fleetFightDistFloor
        v.maxRangeD = fmt.Sprintf("median fight distance %.0f"+
            " units over %d samples", median, len(w.fightDists))
        if v.maxRange && w.shots < fleetMinShots {
            // The distance evidence without the shots is not a kite
            // held at range but a slot that never fought (the
            // approach stretches of the pursuit-hold round measured
            // a walking slot's 1434 unit 'fight' medians over 62
            // samples with zero shots - the attack stance lingered
            // while the character circled its cell). The max-range
            // verdict rides the shooting evidence the same way the
            // distance evidence rides the samples.
            v.maxRange = false
            v.maxRangeD = fmt.Sprintf("median fight distance %.0f"+
                " units over %d samples, but only %d shots - the"+
                " slot never fought",
                median, len(w.fightDists), w.shots)
        }
    }
    if len(w.retreatLags) >= fleetMinRetreats {
        median := medianDuration(w.retreatLags)
        v.early = median <= fleetRetreatLagCeil
        v.earlyD = fmt.Sprintf("median shot-to-retreat lag %s"+
            " over %d retreats", median.Round(100*time.Millisecond),
            len(w.retreatLags))
    }
    if len(w.reshotGaps) >= fleetMinRetreats {
        median := medianDuration(w.reshotGaps)
        v.reshot = median <= fleetReshotGapCeil
        v.reshotD = fmt.Sprintf("median walk-end-to-shot gap %s"+
            " over %d walks", median.Round(100*time.Millisecond),
            len(w.reshotGaps))
    }
    // The curving retreat holds on two attributions the leash fix of
    // the curve round pinned: the per-fight drift (the median max
    // displacement from the mob's stand across the kited fights -
    // the cell rotation between the fights never reaches it) and
    // the same-side polyline corners between the consecutive
    // confirmed retreats of one fight.
    drifts := w.kiteFightDrifts()
    corners := w.curveCorners()
    if len(drifts) > 0 && corners >= fleetMinTurns {
        median := medianFloat(drifts)
        leashed := median <= fleetFightDriftLeash
        v.curve = leashed
        if leashed {
            v.curveD = fmt.Sprintf("median fight drift %.0f units"+
                " over %d kite fights, %d same-side corners",
                median, len(drifts), corners)
        } else {
            v.curveD = fmt.Sprintf("the fight drift broke at %.0f"+
                " units (median over %d kite fights)",
                median, len(drifts))
        }
    } else {
        v.curveD = fmt.Sprintf("%d kite fights and %d same-side"+
            " retreat corners observed (need %d corners)",
            len(drifts), corners, fleetMinTurns)
    }

    return v
}

// fleetBotRun is the live state of one fleet bot: the slot, the
// wrapped session log counter, the folded watch and the manager the
// tracker resolves through.
type fleetBotRun struct {
    slot    archerFleetSlotDesc
    manager *Manager
    log     *archerKiteLog
    watch   fleetWatch
}

// tracker resolves the live tracker of the bot's account.
func (b *fleetBotRun) tracker() *state.Bot {
    return b.manager.registryTracker(b.slot.Account)
}

// archerFleetScenario runs the mass audit: reset the five slots,
// launch the five archer sessions, sample the fleet through the
// window and answer with the behavior matrix - the scenario fails
// naming the behaviors the fleet does NOT implement.
func archerFleetScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(archerFleetChecks())

    resets := make([]characterReset, 0, len(archerFleetSlots))
    for _, slot := range archerFleetSlots {
        if err := m.ensureCharacter(slot.Account,
            archerFleetPassword(slot.Account), slot.Account,
            test.appendLog); err != nil {
            return fmt.Errorf("ensure character %s: %w", slot.Account,
                err)
        }
        time.Sleep(ensurePause)
        z := fleetSpawnZ(m.engine, slot.X, slot.Y)
        test.appendLog(fmt.Sprintf("fleet: slot %s wakes on %s at"+
            " (%d, %d, %d)", slot.Account, slot.Cell, slot.X, slot.Y,
            z))
        resets = append(resets, archerFleetReset(slot, z))
    }
    for _, reset := range resets {
        if err := m.injectReset(reset, test); err != nil {
            return fmt.Errorf("inject start state %s: %w",
                reset.Account, err)
        }
    }
    time.Sleep(ensurePause)

    sessionCtx, cancelSession := context.WithCancel(ctx)
    defer cancelSession()
    bots := launchFleet(sessionCtx, m, test)

    if err := waitFleetOnline(ctx, bots); err != nil {
        cancelSession()
        joinFleet(bots)

        return fmt.Errorf("world entry: %w", err)
    }
    test.appendLog("fleet: all five archers are online, the audit" +
        " window is running")

    window := archerFleetDuration()
    deadline := time.Now().Add(window)
    windowStart := time.Now()
    sampleFleet(ctx, bots, deadline)
    ran := time.Since(windowStart)
    cancelSession()
    joinFleet(bots)

    return fleetVerdict(test, bots, window, ran)
}

// launchFleet starts the five supervised archer sessions with the
// staggered logins of the parallel runner.
func launchFleet(
    ctx context.Context, m *Manager, test *Test,
) []*fleetBotRun {
    bots := make([]*fleetBotRun, 0, len(archerFleetSlots))
    for _, slot := range archerFleetSlots {
        m.ensureFleetTracker(slot.Account)
        bot := &fleetBotRun{
            slot:    slot,
            manager: m,
            log:     newArcherKiteLog(test.appendLog),
            watch:   newFleetWatch(),
        }
        bots = append(bots, bot)
        go m.runSessionSupervised(ctx, slot.Account,
            archerFleetPassword(slot.Account), slot.Account, m.proxy,
            bot.log.line, archerKiteTypeHook)
        time.Sleep(2 * time.Second)
    }

    return bots
}

// ensureFleetTracker registers the slot's tracker in the bot registry
// BEFORE the session launches. NewManager registers one tracker per
// scenario definition (the single-account scenarios), but the fleet
// runs five accounts under one definition - without the registration
// every registryTracker resolution of a slot account mints a fresh
// offline twin (the session updates its own private copy, the entry
// wait and the sampler poll another, the web UI shows neither) and
// the audit cannot see its own bots: the first live rounds measured
// a full entry-budget lapse on five bots that were all in the world
// and fighting. The registration mirrors the NewManager seeding (the
// acceptance kind, the offline stance until the first login) so the
// fleet slots appear in the web UI bot list like every scenario
// tracker.
func (m *Manager) ensureFleetTracker(account string) *state.Bot {
    if bot, ok := m.registry.Get(account); ok {
        return bot
    }
    bot := state.NewBot(account)
    bot.SetKind(state.KindAcceptance)
    bot.SetOffline()
    m.registry.Add(bot)

    return bot
}

// waitFleetOnline waits until every fleet tracker has entered the
// world at least once or the budget lapses. The entry LATCH
// tolerates the death-restart cycle of the starter-kit archers (the
// level 7 fleet on the mass 3+ Kaboo cells dies sometimes: the first
// live round measured a bot dying inside the launch minute, and its
// village restart - the designed recovery with the cell regression -
// outlasted the whole entry budget while the other four fought on):
// a bot that entered and died mid-launch still entered, and the
// audit window measures the kite rhythm of whatever the fleet does,
// with the per-bot evidence floors naming the slots that spent the
// window recovering instead of fighting.
func waitFleetOnline(ctx context.Context, bots []*fleetBotRun) error {
    deadline := time.Now().Add(onlineWait)
    entered := make([]bool, len(bots))
    for time.Now().Before(deadline) {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        allIn := true
        for i, bot := range bots {
            if entered[i] {
                continue
            }
            if bot.tracker().Status() != state.StatusOnline {
                allIn = false

                break
            }
            entered[i] = true
            bot.log.line("fleet: " + bot.slot.Account +
                " entered the world")
        }
        if allIn {
            return nil
        }
        time.Sleep(time.Second)
    }

    return errors.New("a fleet bot never entered the world")
}

// sampleFleet samples every fleet bot through the window at the
// quarter-second cadence and folds the samples into the watches.
func sampleFleet(ctx context.Context, bots []*fleetBotRun,
    deadline time.Time,
) {
    for time.Now().Before(deadline) {
        if ctx.Err() != nil {
            return
        }
        for _, bot := range bots {
            bot.foldSample()
        }
        time.Sleep(fleetSamplePeriod)
    }
}

// foldSample latches one sample of the bot's tracker into its watch.
func (b *fleetBotRun) foldSample() {
    tracker := b.tracker()
    sample := fleetSample{
        at:        time.Now(),
        hpPct:     tracker.SelfHealthPercent(),
        shotAt:    tracker.SelfLastShotAt(),
        walking:   tracker.SelfWalking(),
        fightDist: -1,
    }
    if x, y, _, ok := tracker.SelfPosition(); ok {
        sample.hasPos = true
        sample.x, sample.y = x, y
    }
    snapshot := tracker.Snapshot()
    target := snapshot.Diagnostics.FightingTargetID
    if target != 0 && snapshot.Diagnostics.AutoAttacking {
        if x, y, _, ok := tracker.SelfPosition(); ok {
            if tx, ty, _, tok := tracker.ObjectPosition(target); tok {
                sample.fighting = true
                sample.targetID = target
                sample.hasTarget = true
                sample.tx, sample.ty = tx, ty
                sample.fightDist = math.Hypot(float64(tx-x),
                    float64(ty-y))
            }
        }
    }
    b.watch = b.watch.fold(sample)
}

// joinFleet waits for the session goroutines to unwind (bounded by
// the shutdown grace).
func joinFleet(bots []*fleetBotRun) {
    deadline := time.Now().Add(archerFleetShutdownGrace)
    for time.Now().Before(deadline) {
        allIdle := true
        for _, bot := range bots {
            if bot.tracker().Status() == state.StatusOnline {
                allIdle = false

                break
            }
        }
        if allIdle {
            return
        }
        time.Sleep(time.Second)
    }
}

// fleetVerdict folds the fleet watches into the behavior matrix and
// the scenario answer: the per-bot table lands in the test log, the
// checks carry the fleet-wide verdicts and the error names the
// behaviors the fleet does not implement.
func fleetVerdict(
    test *Test, bots []*fleetBotRun, window, ran time.Duration,
) error {
    if ran < window*9/10 {
        test.appendLog(fmt.Sprintf("fleet: the audit window truncated"+
            " - %s of %s, the verdicts ride partial data",
            ran.Round(time.Second), window.Round(time.Second)))

        return fmt.Errorf("the audit window truncated at %s of %s",
            ran.Round(time.Second), window.Round(time.Second))
    }
    counts := map[string]int{}
    behind := map[string][]string{}
    for _, bot := range bots {
        verdicts := bot.watch.verdicts()
        test.appendLog(fmt.Sprintf("fleet: %s on %s - max-range %t"+
            " (%s), shoots %t (%s), early-retreat %t (%s),"+
            " quick-reshot %t (%s), curved-retreat %t (%s)",
            bot.slot.Account, bot.slot.Cell,
            verdicts.maxRange, verdicts.maxRangeD,
            verdicts.shoots, verdicts.shootsD,
            verdicts.early, verdicts.earlyD,
            verdicts.reshot, verdicts.reshotD,
            verdicts.curve, verdicts.curveD))
        for id, ok := range map[string]bool{
            checkFleetMaxRange: verdicts.maxRange,
            checkFleetShoots:   verdicts.shoots,
            checkFleetEarly:    verdicts.early,
            checkFleetReshot:   verdicts.reshot,
            checkFleetCurve:    verdicts.curve,
        } {
            if ok {
                counts[id]++
            } else {
                // The fleet line names the slots the behavior did
                // not hold on - a true fail and a slot that spent
                // the window recovering (the evidence floors never
                // armed) both belong here: the per-bot line above
                // says which of the two it was.
                behind[id] = append(behind[id], bot.slot.Account)
            }
        }
    }
    total := len(bots)
    var broken []string
    for _, id := range []string{checkFleetMaxRange,
        checkFleetShoots, checkFleetEarly, checkFleetReshot,
        checkFleetCurve} {
        detail := fmt.Sprintf("%d of %d bots", counts[id], total)
        if len(behind[id]) > 0 {
            detail += " (not passing: " + strings.Join(behind[id],
                ", ") + ")"
        }
        test.updateCheck(id, counts[id] == total, detail)
        if counts[id] < total {
            broken = append(broken, id+" ("+detail+")")
        }
    }
    steps, holds := 0, 0
    for _, bot := range bots {
        botSteps, botHolds := bot.log.counts()
        steps += botSteps
        holds += botHolds
    }
    test.appendLog(fmt.Sprintf("fleet: total kite steps %d, holds"+
        " %d across the window", steps, holds))
    if len(broken) > 0 {
        return errors.New("the fleet kite audit found unimplemented" +
            " behaviors: " + strings.Join(broken, "; "))
    }

    return nil
}
