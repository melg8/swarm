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
// stack, and the window itself is the observed fight time.
const archerFleetDefaultMinutes = 2.5

// archerFleetShutdownGrace bounds the teardown of the five sessions.
const archerFleetShutdownGrace = 45 * time.Second

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
// rode the accepted window), the re-shot gap ceiling, the anchor
// leash of the farm point and the evidence floors.
const (
    fleetFightDistFloor   = 250.0
    fleetRetreatLagCeil   = 2200 * time.Millisecond
    fleetReshotGapCeil    = 1500 * time.Millisecond
    fleetAnchorLeash      = 1500.0
    fleetMinShots         = 3
    fleetMinRetreats      = 2
    fleetMinFightSamples  = 4
    fleetTurnAngle        = 40.0
    fleetMinTurns         = 2
    fleetSamplePeriod     = 250 * time.Millisecond
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
// the startup and the teardown, kept under the five minute ask.
func archerFleetTimeout() time.Duration {
    return archerFleetDuration() + 2*ensurePause + onlineWait +
        archerFleetShutdownGrace
}

// archerFleetReset returns the start state of one fleet slot: the
// proven archer kit of the archer scenario woken on the slot focus.
func archerFleetReset(slot archerFleetSlotDesc, z int32,
) characterReset {
    reset := archerKiteReset(slot.Account)
    reset.X = slot.X
    reset.Y = slot.Y
    reset.Z = z

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
// position, the fight target and the health.
type fleetSample struct {
    at        time.Time
    walking   bool
    shotAt    time.Time
    hasPos    bool
    x, y      int32
    fighting  bool
    fightDist float64
    hpPct     float64
}

// fleetWatch folds one bot's samples into the behavior evidence: the
// shot count, the retreat lags (the movement start after a shot), the
// re-shot gaps (the shot after a walk ends), the fight distance
// distribution, the anchor leash and the walk direction turns.
type fleetWatch struct {
    anchorX     int32
    anchorY     int32
    shots       int
    retreatLags []time.Duration
    reshotGaps  []time.Duration
    fightDists  []float64
    maxAnchor   float64
    turns       int
    // fold state
    lastShot     time.Time
    walkStarted  time.Time
    walkEndedAt  time.Time
    lastWalkVecX float64
    lastWalkVecY float64
    haveWalkVec  bool
    lastX        int32
    lastY        int32
    haveLast     bool
}

// newFleetWatch arms the fold with the spawn anchor.
func newFleetWatch(x, y int32) fleetWatch {
    return fleetWatch{
        anchorX: x, anchorY: y,
    }
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
        w.walkStarted = s.at
        if !w.lastShot.IsZero() && s.at.After(w.lastShot) {
            if lag := s.at.Sub(w.lastShot); lag >= 0 {
                w.retreatLags = append(w.retreatLags, lag)
            }
        }
    }
    if !s.walking && !w.walkStarted.IsZero() {
        w.walkStarted = time.Time{}
        w.walkEndedAt = s.at
    }
    if s.fighting && s.fightDist >= 0 {
        w.fightDists = append(w.fightDists, s.fightDist)
    }
    if s.hasPos {
        anchorDist := math.Hypot(float64(s.x-w.anchorX),
            float64(s.y-w.anchorY))
        if anchorDist > w.maxAnchor {
            w.maxAnchor = anchorDist
        }
        if w.haveLast {
            vecX := float64(s.x - w.lastX)
            vecY := float64(s.y - w.lastY)
            stepLen := math.Hypot(vecX, vecY)
            if stepLen >= 20 {
                if w.haveWalkVec {
                    dot := vecX*w.lastWalkVecX + vecY*w.lastWalkVecY
                    cross := vecX*w.lastWalkVecY - vecY*w.lastWalkVecX
                    angle := math.Abs(math.Atan2(cross, dot) * 180 /
                        math.Pi)
                    if angle >= fleetTurnAngle {
                        w.turns++
                    }
                }
                w.lastWalkVecX = vecX / stepLen
                w.lastWalkVecY = vecY / stepLen
                w.haveWalkVec = true
            }
        }
        w.lastX, w.lastY = s.x, s.y
        w.haveLast = true
    }

    return w
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
    maxRange     bool
    maxRangeD    string
    shoots       bool
    shootsD      string
    early        bool
    earlyD       string
    reshot       bool
    reshotD      string
    curve        bool
    curveD       string
}

// verdicts reads the folded evidence into the behavior outcomes: each
// verdict holds only on its evidence floor (a bot that never fought
// keeps the checks open, the detail says why).
func (w fleetWatch) verdicts() fleetVerdicts {
    v := fleetVerdicts{
        maxRangeD: "no fight samples yet",
        shootsD:   fmt.Sprintf("%d shots observed", w.shots),
        earlyD:    fmt.Sprintf("%d retreats observed",
            len(w.retreatLags)),
        reshotD:   fmt.Sprintf("%d walk-end re-shots observed",
            len(w.reshotGaps)),
        curveD: fmt.Sprintf("max anchor distance %.0f units, %d"+
            " walk turns", w.maxAnchor, w.turns),
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
    leashed := w.maxAnchor <= fleetAnchorLeash
    curved := w.turns >= fleetMinTurns
    v.curve = leashed && curved
    if !leashed {
        v.curveD = fmt.Sprintf("the anchor leash broke at %.0f"+
            " units", w.maxAnchor)
    } else if !curved {
        v.curveD = fmt.Sprintf("leashed at %.0f units but only %d"+
            " walk turns", w.maxAnchor, w.turns)
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
        bot := &fleetBotRun{
            slot:    slot,
            manager: m,
            log:     newArcherKiteLog(test.appendLog),
            watch:   newFleetWatch(slot.X, slot.Y),
        }
        bots = append(bots, bot)
        go m.runSessionSupervised(ctx, slot.Account,
            archerFleetPassword(slot.Account), slot.Account, m.proxy,
            bot.log.line, archerKiteTypeHook)
        time.Sleep(2 * time.Second)
    }

    return bots
}

// waitFleetOnline waits until every fleet tracker reports online or
// the budget lapses.
func waitFleetOnline(ctx context.Context, bots []*fleetBotRun) error {
    deadline := time.Now().Add(onlineWait)
    for time.Now().Before(deadline) {
        if ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        allOnline := true
        for _, bot := range bots {
            if bot.tracker().Status() != state.StatusOnline {
                allOnline = false

                break
            }
        }
        if allOnline {
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
            }
        }
    }
    total := len(bots)
    var broken []string
    for _, id := range []string{checkFleetMaxRange,
        checkFleetShoots, checkFleetEarly, checkFleetReshot,
        checkFleetCurve} {
        detail := fmt.Sprintf("%d of %d bots", counts[id], total)
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
