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

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The archer fleet acceptance scenario: five archer bots wake at five cells of
// the elven Kaboo ground and farm a short window under the kite contract; it
// answers per bot and for the fleet which behaviors the fleet lacks.

// archerFleetScenarioID is the scenario registry id.
const archerFleetScenarioID = "archer-fleet"

// archerFleetDefaultMinutes is the live window of the audit: the scenario must
// land under five minutes, and the measured launch and teardown overhead is
// ~25 s, so three minutes ride ~3.4 minutes of wall clock.
const archerFleetDefaultMinutes = 3.0

// archerFleetShutdownGrace bounds the teardown of the five sessions.
const archerFleetShutdownGrace = 45 * time.Second

// fleetOnlineBudget bounds the staggered five-slot launch: the fifth slot lands
// ~13 s in, and the shared 90 s onlineWait would push the window past five
// minutes.
const fleetOnlineBudget = 40 * time.Second

// archerFleetSlotDesc is one fleet slot: the temp account and the spawn cell
// focus.
type archerFleetSlotDesc struct {
    Account string
    Cell    string
    X       int32
    Y       int32
}

// archerFleetSlots is the five-cell spread: the mass 3+ Kaboo Orc cells of the
// south-west elven ground, far enough apart that the loops never share a mob
// pool, close enough to share one terrain sheet (the spawn Z oracle).
var archerFleetSlots = []archerFleetSlotDesc{
    {Account: "temp20", Cell: "elven-hex-104", X: 36000, Y: 50229},
    {Account: "temp21", Cell: "elven-hex-106", X: 36000, Y: 51962},
    {Account: "temp22", Cell: "elven-hex-105", X: 34500, Y: 49363},
    {Account: "temp23", Cell: "elven-hex-086", X: 39000, Y: 50229},
    {Account: "temp24", Cell: "elven-hex-087", X: 37500, Y: 49363},
}

// archerFleetPassword answers the temp account password (the account equals the
// password, the Mobius login auto-creates missing ones).
func archerFleetPassword(account string) string {
    return account
}

// The fleet telemetry thresholds mirror the kite constants and the server bow
// cycle (docs/kite_timing_findings.md).
const (
    fleetFightDistFloor  = 250.0
    fleetRetreatLagCeil  = 2200 * time.Millisecond
    fleetReshotGapCeil   = 1500 * time.Millisecond
    fleetReshotDistFloor = 400.0
    fleetFightDriftLeash = 1500.0
    fleetMinShots        = 3
    fleetMinRetreats     = 2
    fleetMinFightSamples = 4
    fleetTurnAngle       = 40.0
    fleetMinTurns        = 2
    fleetSamplePeriod    = 250 * time.Millisecond
    // fleetWindupWindow mirrors the accepted-move boundary (the measured windup
    // end plus the lead, ~1.65 s at pAtkSpd 337): the one allowed standing.
    fleetWindupWindow = 1650 * time.Millisecond
    // fleetStandShareCeil bounds the avoidable standing share of a fight: the
    // healthy rhythm books near zero, a standing fight reads above it.
    fleetStandShareCeil = 0.35
)

// The fleet check ids: one per audited kite behavior plus the online gate.
const (
    checkFleetMaxRange = "max-range"
    checkFleetShoots   = "shoots"
    checkFleetEarly    = "early-retreat"
    checkFleetReshot   = "quick-reshot"
    checkFleetCurve    = "curved-retreat"
    checkFleetRuns     = "always-run"
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
        {
            ID:    checkFleetRuns,
            Label: "runs through the fight, stands only the windup",
            Done:  false, Detail: "",
        },
    }
}

// archerFleetDuration reads the SWARM_ARCHER_FLEET_MINUTES env and returns the
// fleet window.
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

// archerFleetTimeout computes the scenario timeout: the window plus startup and
// teardown, under the five minute ask (the fleet rides fleetOnlineBudget, not
// the shared 90 s onlineWait of the single-bot scenarios).
func archerFleetTimeout() time.Duration {
    return archerFleetDuration() + 2*ensurePause +
        fleetOnlineBudget + archerFleetShutdownGrace
}

// The fleet vitals: the server recomputes the level 7 elven fighter maxima at
// login (214 HP / 82 MP whatever the reset row says), and a lower curHp lands
// as a wounded start that spends the window recovering.
const (
    fleetLevel7MaxHP = 214
    fleetLevel7MaxMP = 82
)

// archerFleetReset returns the start state of one slot: the archer kit woken on
// the slot focus at the full level 7 vitals (the single-bot scenario keeps its
// own gain-table start).
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

// fleetSpawnZ answers the walkable height of a spawn focus through the geodata
// click oracle: an approach click onto the focus snaps to the layer the server
// would walk. A missing engine or out-of-band answer falls back to
// archerKiteSpawnZ.
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

// fleetSample is one observation: the movement state, the last own shot stamp,
// the position, the fight target ground position and the health.
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

// fleetWatch folds one bot's samples into the behavior evidence: the shots, the
// retreat lags and re-shot gaps, the fight distances, the per-fight drift and
// the retreat corners. Only the first away-walk of a shot counts.
type fleetWatch struct {
    shots       int
    retreatLags []time.Duration
    reshotGaps  []time.Duration
    reshotDists []float64
    fightDists  []float64
    // The curve attribution: the drift anchor is the target's stand at the
    // fight start, and only fights with a confirmed retreat feed the leash.
    fightTarget   int32
    fightAnchorX  int32
    fightAnchorY  int32
    fightMaxDrift float64
    fightRetreats int
    fightDrifts   []float64
    // The always-run book: every fighting sample lands in one bucket -
    // walking, the forced windup or the avoidable standing.
    fightMoving   int
    fightWindup   int
    fightStanding int
    fightSamplesN int
    // The corner evidence: the signed angle between consecutive confirmed
    // retreats of one fight - a straight runaway repeats its heading, the
    // circle bends every corner one way.
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
    // The pending retreat candidate of the running walk: armed at the walk
    // start with the lag from the last shot, settled at the walk end by the
    // away-direction check.
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
    // lastRetreatShotAt names the shot whose first confirmed retreat is booked:
    // the continuation walks of one cycle pair to the same shot, and only the
    // first measures the answer speed.
    lastRetreatShotAt time.Time
}

// newFleetWatch arms the fold.
func newFleetWatch() fleetWatch {
    return fleetWatch{}
}

// foldMotion books one fighting sample: walking, the server-forced windup
// (inside fleetWindupWindow of the own shot) or the avoidable standing.
func (w fleetWatch) foldMotion(s fleetSample) fleetWatch {
    w.fightSamplesN++
    switch {
    case s.walking:
        w.fightMoving++
    case !s.shotAt.IsZero() &&
        s.at.Sub(s.shotAt) < fleetWindupWindow:
        w.fightWindup++
    default:
        w.fightStanding++
    }

    return w
}

// reshotDist answers the fight distance of a re-shot sample, -1 without a known
// range: a missing distance must never read as a passed floor.
func reshotDist(s fleetSample) float64 {
    if s.fighting && s.fightDist >= 0 {
        return s.fightDist
    }

    return -1
}

// foldWalkEnd settles the walk candidate: the away-direction verdict gates the
// pending retreat metrics, the first confirmed retreat of its shot books the
// lag, and the walk state clears.
func (w fleetWatch) foldWalkEnd(s fleetSample) fleetWatch {
    if w.walkFighting && w.haveWalkFrom && s.hasPos {
        dx := float64(s.x - w.walkFromX)
        dy := float64(s.y - w.walkFromY)
        awayX := float64(w.walkFromX - w.walkTargetX)
        awayY := float64(w.walkFromY - w.walkTargetY)
        if dx*awayX+dy*awayY > 0 {
            if w.havePendingLag &&
                w.walkShotAt != w.lastRetreatShotAt {
                // The first confirmed retreat of its shot: the continuation
                // walks of the cycle pair to it and never re-feed the
                // median (lastRetreatShotAt).
                w.retreatLags = append(w.retreatLags, w.pendingLag)
                w.lastRetreatShotAt = w.walkShotAt
            }
            if !s.shotAt.IsZero() && !w.walkShotAt.IsZero() &&
                s.shotAt != w.walkShotAt {
                // The re-shot itself ended the walk: the gap is zero and the
                // settling sample carries the range the re-shot landed at.
                w.reshotGaps = append(w.reshotGaps, 0)
                w.reshotDists = append(w.reshotDists, reshotDist(s))
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

    return w
}

// foldWalkStart arms the walk candidate: a live fight arms the lag, and the
// walk end confirms the away direction (a chase-stall walk starts in a fight
// too).
func (w fleetWatch) foldWalkStart(s fleetSample) fleetWatch {
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

    return w
}

// foldShot latches a fresh own shot: the shot counts, and a walk that ended
// before it books the re-shot gap with the distance it landed at.
func (w fleetWatch) foldShot(s fleetSample) fleetWatch {
    w.shots++
    w.lastShot = s.shotAt
    if !w.walkEndedAt.IsZero() && s.shotAt.After(w.walkEndedAt) {
        if gap := s.shotAt.Sub(w.walkEndedAt); gap >= 0 {
            w.reshotGaps = append(w.reshotGaps, gap)
            w.reshotDists = append(w.reshotDists, reshotDist(s))
        }
        w.walkEndedAt = time.Time{}
    }

    return w
}

// fold latches one sample into the watch (the pure evaluation core).
func (w fleetWatch) fold(s fleetSample) fleetWatch {
    if !s.shotAt.IsZero() && s.shotAt != w.lastShot {
        w = w.foldShot(s)
    }
    if s.walking && w.walkStarted.IsZero() {
        w = w.foldWalkStart(s)
    }
    if !s.walking && !w.walkStarted.IsZero() {
        // The walk ended: foldWalkEnd settles the candidate.
        w = w.foldWalkEnd(s)
    }
    if s.fighting && s.fightDist >= 0 {
        w.fightDists = append(w.fightDists, s.fightDist)
    }
    if s.fighting {
        w = w.foldMotion(s)
    }
    if s.fighting && s.hasTarget && s.targetID != 0 {
        w = w.foldFight(s)
    }

    return w
}

// foldRetreat books one confirmed retreat: the walk's fight gains its retreat
// count and its unit displacement extends the fight's corner chain.
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
                // The signed rotation from the previous heading: positive
                // bends counterclockwise, negative clockwise.
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

// foldFight tracks the per-fight drift: the first fighting sample of a target
// anchors on the mob's stand (the farm point, not the spawn), and a fight
// switch closes the track. Fights without a retreat carry no curve evidence.
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

// closeFight records the max drift of a fight the kite retreated in (the leash
// measures fights, not the window).
func (w fleetWatch) closeFight() fleetWatch {
    if w.fightTarget != 0 && w.fightRetreats > 0 {
        w.fightDrifts = append(w.fightDrifts, w.fightMaxDrift)
    }
    w.fightTarget = 0
    w.fightRetreats = 0
    w.fightMaxDrift = 0

    return w
}

// kiteFightDrifts answers the drifts of every kited fight, closed and open.
func (w fleetWatch) kiteFightDrifts() []float64 {
    drifts := append([]float64{}, w.fightDrifts...)
    if w.fightTarget != 0 && w.fightRetreats > 0 {
        drifts = append(drifts, w.fightMaxDrift)
    }

    return drifts
}

// curveCorners answers the big corners of one turn side: the curving retreat
// bends every corner the same way, so the majority side carries the evidence
// (a wall-bounce zigzag racks both sides).
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

// fleetVerdicts is the per-behavior outcome: the verdict, its detail and
// whether the evidence floor engaged (an unarmed floor is "no evidence").
type fleetVerdicts struct {
    maxRange  bool
    maxRangeD string
    maxRangeE bool
    shoots    bool
    shootsD   string
    shootsE   bool
    early     bool
    earlyD    string
    earlyE    bool
    reshot    bool
    reshotD   string
    reshotE   bool
    curve     bool
    curveD    string
    curveE    bool
    runs      bool
    runsD     string
    runsE     bool
}

// runsVerdict reads the motion book into the always-run outcome: the avoidable
// standing share of the fight samples, everything outside the forced windup.
func (w fleetWatch) runsVerdict() (ok bool, detail string,
    evidenced bool,
) {
    if w.fightSamplesN < fleetMinFightSamples ||
        w.shots < fleetMinShots {
        // The floor rides the shooting evidence: a slot that never shot never
        // fought a kite fight.
        return false, fmt.Sprintf("%d fight samples, %d shots "+
            "observed", w.fightSamplesN, w.shots), false
    }
    share := float64(w.fightStanding) / float64(w.fightSamplesN)

    return share <= fleetStandShareCeil,
        fmt.Sprintf("standing %d of %d fight samples"+
            " (%.0f%% avoidable, %d%% in the windup)",
            w.fightStanding, w.fightSamplesN, share*100,
            100*w.fightWindup/w.fightSamplesN), true
}

// curveVerdict reads the drifts and corners into the curving-retreat outcome:
// the per-fight drift from the mob's stand (the cell rotation never reaches
// it) and the same-side corners between the consecutive retreats.
func (w fleetWatch) curveVerdict() (ok bool, detail string,
    evidenced bool,
) {
    drifts := w.kiteFightDrifts()
    corners := w.curveCorners()
    if len(drifts) == 0 || corners < fleetMinTurns {
        return false, fmt.Sprintf("%d kite fights and %d same-side"+
            " retreat corners observed (need %d corners)",
            len(drifts), corners, fleetMinTurns), false
    }
    median := medianFloat(drifts)
    if median > fleetFightDriftLeash {
        return false, fmt.Sprintf("the fight drift broke at %.0f"+
            " units (median over %d kite fights)",
            median, len(drifts)), true
    }

    return true, fmt.Sprintf("median fight drift %.0f units over"+
        " %d kite fights, %d same-side corners",
        median, len(drifts), corners), true
}

// reshotVerdict folds the quick-reshot behavior: the kite runs long race legs
// before the single re-shot, so the verdict reads the distance the re-shot
// lands at. The gap ceiling is the fallback when no distance was seen.
func (w fleetWatch) reshotVerdict(v *fleetVerdicts) {
    v.reshotE = true
    medianGap := medianDuration(w.reshotGaps)
    dists := make([]float64, 0, len(w.reshotDists))
    for _, dist := range w.reshotDists {
        if dist >= 0 {
            dists = append(dists, dist)
        }
    }
    if len(dists) > 0 {
        medianDist := medianFloat(dists)
        v.reshot = medianDist >= fleetReshotDistFloor
        v.reshotD = fmt.Sprintf("median re-shot distance %.0f"+
            " units over %d re-shots, median walk-end-to-shot"+
            " gap %s", medianDist, len(dists),
            medianGap.Round(100*time.Millisecond))
    } else {
        v.reshot = medianGap <= fleetReshotGapCeil
        v.reshotD = fmt.Sprintf("median walk-end-to-shot gap %s"+
            " over %d walks, no re-shot distance observed"+
            " - the gap ceiling fallback",
            medianGap.Round(100*time.Millisecond),
            len(w.reshotGaps))
    }
}

// verdicts reads the folded evidence into the outcomes: each verdict holds only
// on its evidence floor (a bot that never fought keeps the check open).
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
    v.runs, v.runsD, v.runsE = w.runsVerdict()
    if w.shots >= fleetMinShots {
        v.shoots = true
        v.shootsE = true
        v.shootsD = fmt.Sprintf("%d shots in the window", w.shots)
    }
    if len(w.fightDists) >= fleetMinFightSamples {
        median := medianFloat(w.fightDists)
        v.maxRange = median >= fleetFightDistFloor
        v.maxRangeD = fmt.Sprintf("median fight distance %.0f"+
            " units over %d samples", median, len(w.fightDists))
        if v.maxRange && w.shots < fleetMinShots {
            // The distance without the shots is not a kite held at range but
            // a lingering attack stance (1434 unit medians with zero shots).
            v.maxRange = false
            v.maxRangeD = fmt.Sprintf("median fight distance %.0f"+
                " units over %d samples, but only %d shots - the"+
                " slot never fought",
                median, len(w.fightDists), w.shots)
        }
        // The floor arms on the shooting evidence too: a slot that never shot
        // never fought a kite fight, whatever the stance sampled.
        v.maxRangeE = w.shots >= fleetMinShots
    }
    if len(w.retreatLags) >= fleetMinRetreats {
        v.earlyE = true
        median := medianDuration(w.retreatLags)
        v.early = median <= fleetRetreatLagCeil
        v.earlyD = fmt.Sprintf("median shot-to-retreat lag %s"+
            " over %d retreats", median.Round(100*time.Millisecond),
            len(w.retreatLags))
    }
    if len(w.reshotGaps) >= fleetMinRetreats {
        w.reshotVerdict(&v)
    }
    v.curve, v.curveD, v.curveE = w.curveVerdict()

    return v
}

// fleetBotRun is the live state of one fleet bot: the slot, the log counter,
// the folded watch and the manager the tracker resolves through.
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

// archerFleetScenario runs the mass audit: reset the five slots, launch the
// sessions, sample the fleet through the window and answer with the behavior
// matrix; the scenario fails naming the missing behaviors.
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

// launchFleet starts the five supervised archer sessions with staggered logins.
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
        // The per-slot ground lock: the slot pins its own cell, so the archers
        // farm five points instead of converging onto the ripe cells.
        hook := func(loop *hunt.Loop) {
            archerKiteTypeHook(loop)
            loop.SetCellPin(slot.Cell)
        }
        go m.runSessionSupervised(ctx, slot.Account,
            archerFleetPassword(slot.Account), slot.Account, m.proxy,
            bot.log.line, hook)
        time.Sleep(2 * time.Second)
    }

    return bots
}

// ensureFleetTracker registers the slot's tracker before the session launches:
// without it every resolution mints a fresh offline twin the audit cannot see.
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

// waitFleetOnline waits until every tracker entered the world once or the
// budget lapses. The latch tolerates the death-restart cycle: a bot that
// entered and died still entered, and the evidence floors name the recovering
// slots.
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

// sampleFleet samples every bot through the window at the quarter-second
// cadence and folds the samples into the watches.
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

// joinFleet waits for the session goroutines to unwind (bounded by the
// shutdown grace).
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

// fleetCheckFold books one bot's verdicts: a pass counts, an evidenced fail
// names the slot behind, an unarmed floor lands under "no evidence".
func fleetCheckFold(
    counts map[string]int,
    behind, unevidenced map[string][]string,
    bot *fleetBotRun, verdicts fleetVerdicts,
) {
    for id, verdict := range map[string]struct {
        ok        bool
        evidenced bool
    }{
        checkFleetMaxRange: {verdicts.maxRange, verdicts.maxRangeE},
        checkFleetShoots:   {verdicts.shoots, verdicts.shootsE},
        checkFleetEarly:    {verdicts.early, verdicts.earlyE},
        checkFleetReshot:   {verdicts.reshot, verdicts.reshotE},
        checkFleetCurve:    {verdicts.curve, verdicts.curveE},
        checkFleetRuns:     {verdicts.runs, verdicts.runsE},
    } {
        switch {
        case verdict.ok:
            counts[id]++
        case verdict.evidenced:
            behind[id] = append(behind[id], bot.slot.Account)
        default:
            unevidenced[id] = append(unevidenced[id],
                bot.slot.Account)
        }
    }
}

// fleetBotLine prints the per-bot verdict table row.
func fleetBotLine(test *Test, bot *fleetBotRun, v fleetVerdicts) {
    test.appendLog(fmt.Sprintf("fleet: %s on %s - max-range %t"+
        " (%s), shoots %t (%s), early-retreat %t (%s),"+
        " quick-reshot %t (%s), curved-retreat %t (%s),"+
        " always-run %t (%s)",
        bot.slot.Account, bot.slot.Cell,
        v.maxRange, v.maxRangeD,
        v.shoots, v.shootsD,
        v.early, v.earlyD,
        v.reshot, v.reshotD,
        v.curve, v.curveD,
        v.runs, v.runsD))
}

// fleetVerdict folds the watches into the behavior matrix: the per-bot table
// lands in the log, the checks carry the fleet-wide verdicts and the error
// names the missing behaviors.
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
    unevidenced := map[string][]string{}
    for _, bot := range bots {
        verdicts := bot.watch.verdicts()
        fleetBotLine(test, bot, verdicts)
        fleetCheckFold(counts, behind, unevidenced, bot, verdicts)
    }
    total := len(bots)
    var broken []string
    for _, id := range []string{checkFleetMaxRange,
        checkFleetShoots, checkFleetEarly, checkFleetReshot,
        checkFleetCurve, checkFleetRuns} {
        detail := fmt.Sprintf("%d of %d bots", counts[id], total)
        if len(behind[id]) > 0 {
            detail += " (not passing: " + strings.Join(behind[id],
                ", ") + ")"
        }
        if len(unevidenced[id]) > 0 {
            detail += " (no evidence: " + strings.Join(unevidenced[id],
                ", ") + ")"
        }
        test.updateCheck(id, counts[id] == total, detail)
        if counts[id] < total {
            broken = append(broken, id+" ("+detail+")")
        }
    }
    steps, holds := 0, 0
    for _, bot := range bots {
        botSteps, botHolds, _, _ := bot.log.counts()
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
