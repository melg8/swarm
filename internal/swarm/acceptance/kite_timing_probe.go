// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "math"
    "strconv"
    "sync"
    "time"

    "github.com/melg8/swarm/internal/swarm/connection"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The kite timing probe (issue #70 research vehicle): a raw wire
// experiment that measures how the Mobius C1 server answers a retreat
// click issued at a controlled delay after the own bow shot started.
// The bow cycle on the server has three phases (the windup before the
// hit task lands, the hit itself and the reuse tail), and the question
// of the improved kite is which phase admits a MoveToLocation without
// deferring it - the answer paces the retreat click of the archer.
// The probe runs one temp character with the bow kit on the Kaboo Orc
// cell, walks a delay ladder of retreat clicks and reports per delay
// whether the movement started immediately, was deferred to the cycle
// end or never ran, plus a cursor key (WASD) round that asks whether
// the claims stream moves the character through the windup while the
// arrow still lands.

// kiteProbeScenarioID is the scenario registry id.
const kiteProbeScenarioID = "kite-timing-probe"

// kiteProbeAccount is the temp account of the probe: temp19 keeps it
// clear of the fleet spread of the archer fleet scenario (temp20+).
const (
    kiteProbeAccount  = "temp19"
    kiteProbePassword = "temp19"
)

// kiteProbePotions is the injected Lesser Healing Potion count: the
// raw session has no hunt loop to drink them, the probe drains one
// whenever the health dips under the floor (the Kaboo Orc Grunt of the
// cell hits a level 7 character hard enough to matter over the probe
// window).
const kiteProbePotions = 20

// kiteProbeHealFloor is the health percentage under which the probe
// drinks a potion between the ladder rounds.
const kiteProbeHealFloor = 60.0

// kiteProbeEquipPace separates the UseItem calls of the manual dress:
// the server flood protector gates the item use at roughly one per
// second, so a faster pace silently drops equips.
const kiteProbeEquipPace = 1200 * time.Millisecond

// kiteProbeLadder is the delay grid of the retreat click: from the
// same instant as the shot (0 ms) past the expected windup end
// (~1.5 s for the Short Bow) to past the full disable window (~3 s),
// the boundary the probe hunts sits inside the grid.
var kiteProbeLadder = []time.Duration{
    0, 300 * time.Millisecond, 600 * time.Millisecond,
    900 * time.Millisecond, 1200 * time.Millisecond,
    1500 * time.Millisecond, 1800 * time.Millisecond,
    2200 * time.Millisecond, 2600 * time.Millisecond,
    3000 * time.Millisecond,
}

// The probe observation constants: the window one round watches for
// the own movement broadcast after the retreat click, the round trip
// the anchor wait allows for the own Attack broadcast after a forced
// attack request (the AI engage walks the bow range first when the
// target sits near the outer band, so the window tolerates the walk),
// and the retreat click length (the hunt kite step).
const (
    kiteProbeObserveWindow = 5 * time.Second
    kiteProbeAnswerWindow  = 3500 * time.Millisecond
    kiteProbeRetreatStep   = 400.0
    kiteProbeImmediateCap  = 500 * time.Millisecond
    kiteProbeTargetMaxDist = 800.0
    kiteProbeMaxMobLevel   = 12
)

// The cursor key probe constants: the claim stream cadence of the
// official client walk simulation (five claims a second), the run
// speed unit step one claim advances (the ~170 units per second of
// the level 7 elven fighter) and the windup length the probe assumes
// when the live pAtkSpd answer is missing (the Short Bow value).
const (
    kiteProbeClaimPeriod = 200 * time.Millisecond
    kiteProbeClaimStep   = 34.0
    kiteProbeClaimSpan   = 3 * time.Second
    kiteProbeWindupFall  = 1500 * time.Millisecond
)

// kiteProbeTimeout bounds the whole probe: the dress, the ladder of
// ten rounds and the cursor key round land well inside four minutes
// on the live stack; the budget keeps the session prologue and the
// teardown comfortable while staying under the 6 minute process cap.
const kiteProbeTimeout = 5 * time.Minute

// The probe check ids: the target engagement, the delay ladder, the
// measured retreat window, the cursor key motion and the shot that
// lands while the character moves.
const (
    checkKiteProbeTarget = "target"
    checkKiteProbeLadder = "ladder"
    checkKiteProbeWindow = "retreat-window"
    checkKiteProbeWASD   = "wasd-motion"
    checkKiteProbeShot   = "wasd-shot"
)

// kiteProbeChecks is the initial check list of the probe.
func kiteProbeChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID:    checkKiteProbeTarget,
            Label: "engages a mob with the bow",
            Done:  false, Detail: "",
        },
        {
            ID:    checkKiteProbeLadder,
            Label: "walks the retreat delay ladder",
            Done:  false, Detail: "",
        },
        {
            ID:    checkKiteProbeWindow,
            Label: "measures the earliest immediate retreat delay",
            Done:  false, Detail: "",
        },
        {
            ID:    checkKiteProbeWASD,
            Label: "cursor key claims move the character",
            Done:  false, Detail: "",
        },
        {
            ID:    checkKiteProbeShot,
            Label: "the arrow lands while the character moves",
            Done:  false, Detail: "",
        },
    }
}

// The wire opcodes the probe recorder watches (the from-game-server
// ids of the connection dispatch): the Attack broadcast, the
// MoveToLocation and StopMove broadcasts, the ValidateLocation
// adoption broadcast and the ActionFailed refusal.
const (
    probeOpMove     = 0x01
    probeOpAttack   = 0x06
    probeOpFailed   = 0x35
    probeOpStopMove = 0x59
    probeOpValidate = 0x76
    probeIDOffset   = 1
    probeIDWidth    = 4
)

// probeRecorder timestamps the self-relevant wire events of the probe
// session: every own Attack broadcast, every own movement-family
// broadcast and every ActionFailed. The recorder hangs on the receive
// tap of the raw session next to the run log mirror, so the rounds
// read exact arrival times without polling the tracker.
type probeRecorder struct {
    mu      sync.Mutex
    selfID  func() int32
    attacks []time.Time
    moves   []time.Time
    claims  []time.Time
    fails   []time.Time
}

// newProbeRecorder arms the recorder with the self object id getter
// (the id only settles after the world entry, so the getter defers
// the read to every event).
func newProbeRecorder(selfID func() int32) *probeRecorder {
    return &probeRecorder{selfID: selfID}
}

// recv is the receive tap callback: one raw payload per server packet.
func (r *probeRecorder) recv(payload []byte) {
    if len(payload) < probeIDOffset+probeIDWidth {
        return
    }
    var who int32
    for i := range probeIDWidth {
        who |= int32(payload[probeIDOffset+i]) << (8 * i)
    }
    now := time.Now()
    r.mu.Lock()
    defer r.mu.Unlock()
    switch payload[0] {
    case probeOpAttack:
        if r.selfID() != 0 && who == r.selfID() {
            r.attacks = append(r.attacks, now)
        }
    case probeOpMove:
        if r.selfID() != 0 && who == r.selfID() {
            r.moves = append(r.moves, now)
        }
    case probeOpValidate:
        if r.selfID() != 0 && who == r.selfID() {
            r.claims = append(r.claims, now)
        }
    case probeOpFailed:
        r.fails = append(r.fails, now)
    }
}

// lastAttackAfter answers the newest own Attack broadcast timestamp
// after the mark (the zero time when none arrived).
func (r *probeRecorder) lastAttackAfter(mark time.Time) time.Time {
    r.mu.Lock()
    defer r.mu.Unlock()
    for i := len(r.attacks) - 1; i >= 0; i-- {
        if r.attacks[i].After(mark) {
            return r.attacks[i]
        }
    }

    return time.Time{}
}

// firstMoveAfter answers the first own MoveToLocation broadcast after
// the mark and how many ActionFailed answers arrived in between.
func (r *probeRecorder) firstMoveAfter(
    mark time.Time,
) (time.Time, int) {
    r.mu.Lock()
    defer r.mu.Unlock()
    first := time.Time{}
    for _, at := range r.moves {
        if at.After(mark) && (first.IsZero() || at.Before(first)) {
            first = at
        }
    }
    fails := 0
    for _, at := range r.fails {
        if at.After(mark) && (first.IsZero() || at.Before(first)) {
            fails++
        }
    }

    return first, fails
}

// claimCountAfter answers how many own ValidateLocation broadcasts
// (the server adopting the claimed placements) arrived after the mark.
func (r *probeRecorder) claimCountAfter(mark time.Time) int {
    r.mu.Lock()
    defer r.mu.Unlock()
    count := 0
    for _, at := range r.claims {
        if at.After(mark) {
            count++
        }
    }

    return count
}

// probeRound is one measured ladder round: the delay the retreat
// click waited after the own Attack broadcast, the timestamps of the
// anchor shot, the click and the first movement broadcast, the
// refusal count before the movement and the classified verdict.
type probeRound struct {
    delay     time.Duration
    shotAt    time.Time
    walkSent  time.Time
    movedAt   time.Time
    refusals  int
    verdict   string
    lagFromShot time.Duration
}

// classifyProbeRound reads the round timing into the verdict name:
// the movement broadcast inside the immediate cap after the click is
// an immediate acceptance, a later broadcast is a deferred one (the
// server held the click back to the cycle end) and no broadcast at
// all inside the observation window means the click never ran.
func classifyProbeRound(r probeRound) string {
    if r.movedAt.IsZero() {
        return "no-move"
    }
    if r.movedAt.Sub(r.walkSent) <= kiteProbeImmediateCap {
        return "immediate"
    }

    return "deferred"
}

// probeLadderSummary folds the ladder rounds into the headline: the
// minimal delay whose click the server accepted immediately (the
// boundary of the move window) and the per-round verdict list.
type probeLadderSummary struct {
    Rounds      int
    ImmediateAt time.Duration
    HasBoundary bool
    Verdicts    []string
}

// summarizeLadder folds the classified rounds: the boundary is the
// smallest delay with an immediate verdict among the rounds that got
// a shot at all.
func summarizeLadder(rounds []probeRound) probeLadderSummary {
    summary := probeLadderSummary{
        Rounds:      len(rounds),
        ImmediateAt: 0,
    }
    for _, round := range rounds {
        summary.Verdicts = append(summary.Verdicts, round.verdict)
        if round.verdict == "immediate" &&
            (!summary.HasBoundary || round.delay < summary.ImmediateAt) {
            summary.HasBoundary = true
            summary.ImmediateAt = round.delay
        }
    }

    return summary
}

// kiteProbeReset returns the start state of the probe: the level 7
// archer kit of the archer scenario plus the potion belt, waking on
// the same Kaboo Orc cell focus (the proven spawn).
func kiteProbeReset(account string) characterReset {
    items := append([]ResetItem{}, archerKiteItems...)
    items = append(items, ResetItem{ItemID: 1060, Count: kiteProbePotions})
    reset := archerKiteReset(account)
    reset.Items = items

    return reset
}

// kiteProbeScenario runs the raw wire probe: dress the kit, walk the
// retreat delay ladder against a live mob, run the cursor key round
// and fold the measurements into the checks. The scenario passes when
// every measurement landed - the values themselves are the data the
// improved kite paces itself with, not a pass or fail contract.
func kiteProbeScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(kiteProbeChecks())

    if err := m.ensureCharacter(kiteProbeAccount, kiteProbePassword,
        kiteProbeAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(kiteProbeReset(kiteProbeAccount), test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    probe, err := newKiteProbeSession(ctx, m, test)
    if err != nil {
        return err
    }
    defer probe.close()

    if err := probe.dress(); err != nil {
        return fmt.Errorf("dress the kit: %w", err)
    }
    target, err := probe.findTarget()
    if err != nil {
        return fmt.Errorf("find target: %w", err)
    }
    test.updateCheck(checkKiteProbeTarget, true,
        "the mob "+strconv.FormatInt(int64(target), 10)+" is engaged")

    rounds := probe.walkLadder()
    summarizeAndMarkLadder(test, rounds)

    wasd := probe.cursorKeyRound()
    markWASD(test, wasd)

    return probeVerdict(rounds, wasd)
}

// summarizeAndMarkLadder folds the ladder rounds into the checks and
// the test log: one line per round (the delay, the verdict, the
// refusal count and the observed movement lag after the shot) plus
// the boundary headline.
func summarizeAndMarkLadder(test *Test, rounds []probeRound) {
    if len(rounds) == 0 {
        test.updateCheck(checkKiteProbeLadder, false,
            "no ladder round completed")

        return
    }
    for _, round := range rounds {
        test.appendLog(fmt.Sprintf(
            "probe: delay %4d ms after the shot -> %-9s (refusals %d,"+
                " move lag from shot %s)",
            round.delay.Milliseconds(), round.verdict, round.refusals,
            round.lagFromShot.Round(time.Millisecond*10)))
    }
    summary := summarizeLadder(rounds)
    detail := fmt.Sprintf("%d rounds, verdicts %v",
        summary.Rounds, summary.Verdicts)
    if summary.HasBoundary {
        detail = fmt.Sprintf(
            "earliest immediate retreat at %d ms after the shot",
            summary.ImmediateAt.Milliseconds())
    }
    test.updateCheck(checkKiteProbeLadder, true,
        fmt.Sprintf("%d rounds walked", summary.Rounds))
    test.updateCheck(checkKiteProbeWindow, summary.HasBoundary, detail)
    test.appendLog("probe: " + detail)
}

// probeWASDResult is the cursor key round measurement: whether the
// claims moved the character inside the windup, how far, how many
// claim adoptions the server broadcast and whether the target lost
// health while the character moved (the arrow landed).
type probeWASDResult struct {
    anchor       time.Time
    movedDuringWindup bool
    gainUnits    float64
    adoptions    int
    hpBefore     float64
    hpAfter      float64
    damageLanded bool
}

// markWASD writes the cursor key round into the checks and the log.
func markWASD(test *Test, result probeWASDResult) {
    test.updateCheck(checkKiteProbeWASD, result.movedDuringWindup,
        fmt.Sprintf("moved %.0f units during the windup, %d claim"+
            " adoptions", result.gainUnits, result.adoptions))
    test.updateCheck(checkKiteProbeShot, result.damageLanded,
        fmt.Sprintf("target HP %.0f%% -> %.0f%% while moving",
            result.hpBefore, result.hpAfter))
    test.appendLog(fmt.Sprintf(
        "probe: wasd round - moved during windup %t (gain %.0f"+
            " units, adoptions %d), damage while moving %t (HP %.0f%%"+
            " -> %.0f%%)",
        result.movedDuringWindup, result.gainUnits, result.adoptions,
        result.damageLanded, result.hpBefore, result.hpAfter))
}

// probeVerdict reads the measurements into the scenario answer: the
// infrastructure failures (no round, no anchor) are errors, the
// measured values never are - the probe reports what the server does.
func probeVerdict(rounds []probeRound, wasd probeWASDResult) error {
    if len(rounds) == 0 {
        return errors.New("the probe completed no ladder round")
    }
    if wasd.anchor.IsZero() {
        return errors.New("the cursor key round got no shot anchor")
    }

    return nil
}

// kiteProbeSession is the raw wire session of the probe: the login,
// the world entry and the game client driven directly (no hunt loop),
// with the probe recorder tapped into the receive stream next to the
// run log mirror.
type kiteProbeSession struct {
    ctx      context.Context
    manager  *Manager
    game     *connection.GameClient
    tracker  *state.Bot
    recorder *probeRecorder
    runDone  chan error
    cancel   context.CancelFunc
}

// newKiteProbeSession logs the temp character in, enters the world
// and starts the read loop of the game client. The session owns the
// world from here on: the dress, the ladder and the cursor key round
// drive the client directly.
func newKiteProbeSession(
    ctx context.Context, m *Manager, test *Test,
) (*kiteProbeSession, error) {
    tracker := m.registryTracker(kiteProbeAccount)
    tracker.ResetSession()
    auth, game, err := m.openGame(kiteProbeAccount, kiteProbePassword,
        m.proxy)
    if err != nil {
        return nil, err
    }
    session := &kiteProbeSession{
        ctx:     ctx,
        manager: m,
        game:    game,
        tracker: tracker,
    }
    game.SetTracker(tracker)
    session.recorder = newProbeRecorder(tracker.SelfObjectID)
    session.wireTaps()
    charList, err := game.Authenticate(gameSessionParams(auth))
    if err != nil {
        _ = game.Close()

        return nil, fmt.Errorf("game authentication: %w", err)
    }
    slot, _, found := charList.FindCharacterByName(kiteProbeAccount)
    if !found {
        _ = game.Close()

        return nil, fmt.Errorf("character %s not found", kiteProbeAccount)
    }
    if err := game.EnterWorld(int32(slot)); err != nil {
        _ = game.Close()

        return nil, fmt.Errorf("enter world: %w", err)
    }
    runCtx, cancel := context.WithCancel(ctx)
    session.cancel = cancel
    session.runDone = make(chan error, 1)
    go func() {
        session.runDone <- game.Run(runCtx, kiteProbeAccount)
    }()
    if err := session.waitWorld(); err != nil {
        cancel()
        _ = game.Close()

        return nil, err
    }
    test.appendLog("probe: the raw session is in the world")

    return session, nil
}

// wireTaps wires the recorder next to the run log mirror on the
// receive stream and the run log on the send stream.
func (s *kiteProbeSession) wireTaps() {
    runLog := s.manager.runLogFor(kiteProbeAccount)
    s.game.SetTap(func(payload []byte) {
        s.recorder.recv(payload)
        if runLog != nil {
            runLog.Recv(payload)
        }
    })
    if runLog != nil {
        s.game.SetSendTap(runLog.Send)
    }
}

// waitWorld blocks until the tracker knows the own position and the
// inventory (the world entry burst) or the budget lapses.
func (s *kiteProbeSession) waitWorld() error {
    deadline := time.Now().Add(onlineWait)
    for time.Now().Before(deadline) {
        if s.ctx.Err() != nil {
            return fmt.Errorf("cancelled: %w", s.ctx.Err())
        }
        if _, _, _, ok := s.tracker.SelfPosition(); ok {
            if len(s.tracker.Snapshot().Inventory) > 0 {
                return nil
            }
        }
        time.Sleep(monitorPeriod / 4)
    }

    return errors.New("the world entry burst never settled")
}

// close tears the session down: the read loop unwinds with the
// context and the socket drops.
func (s *kiteProbeSession) close() {
    if s.cancel != nil {
        s.cancel()
    }
    if s.runDone != nil {
        select {
        case <-s.runDone:
        case <-time.After(10 * time.Second):
        }
    }
    _ = s.game.Close()
}

// probeItemIDs maps the dress order to the item ids: the bow first,
// the quiver second (the left hand belongs to the arrows), then the
// armor and the jewels.
var probeDressItems = []int32{13, 17, 23, 31, 44, 50, 1121}

// dress equips the bow kit manually: the raw session has no hunt
// loop, so the UseItem calls of the official client paperdoll the
// gear one slot at a time, paced by the item use flood protector.
func (s *kiteProbeSession) dress() error {
    for _, itemID := range probeDressItems {
        objectID, ok := s.inventoryObject(itemID)
        if !ok {
            return fmt.Errorf("item %d missing from the inventory",
                itemID)
        }
        if err := s.game.UseItem(objectID); err != nil {
            return fmt.Errorf("equip item %d: %w", itemID, err)
        }
        time.Sleep(kiteProbeEquipPace)
    }

    return nil
}

// inventoryObject resolves an item id to its inventory object id.
func (s *kiteProbeSession) inventoryObject(itemID int32,
) (int32, bool) {
    for _, item := range s.tracker.Snapshot().Inventory {
        if item.ItemID == itemID {
            return item.ObjectID, true
        }
    }

    return 0, false
}

// findTarget waits for a live attackable mob inside the bow band: the
// Kaboo Orc ground respawns every 15-20 s, so the wait tolerates a
// freshly cleared cell.
func (s *kiteProbeSession) findTarget() (int32, error) {
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        if s.ctx.Err() != nil {
            return 0, fmt.Errorf("cancelled: %w", s.ctx.Err())
        }
        if id, ok := s.nearestMob(); ok {
            return id, nil
        }
        time.Sleep(time.Second)
    }

    return 0, errors.New("no attackable mob inside the band in 30 s")
}

// nearestMob scans the known objects for the preferred target: the
// weakest live attackable mob inside the approach band (the level
// order keeps the probe away from the level 10 Kaboo Orc Fighter
// when the level 7 Grunts are around), skipping the guards (the
// level cap).
func (s *kiteProbeSession) nearestMob() (int32, bool) {
    selfX, selfY, _, ok := s.tracker.SelfPosition()
    if !ok {
        return 0, false
    }
    best := int32(0)
    bestLevel := int32(kiteProbeMaxMobLevel + 1)
    bestDist := kiteProbeTargetMaxDist
    for _, id := range s.tracker.KnownObjectIDs() {
        if !s.tracker.ObjectAttackable(id) ||
            !s.tracker.ObjectAlive(id) {
            continue
        }
        level, hasLevel := s.tracker.ObjectLevel(id)
        if hasLevel && level > kiteProbeMaxMobLevel {
            continue
        }
        mobX, mobY, _, ok := s.tracker.ObjectPosition(id)
        if !ok {
            continue
        }
        dist := math.Hypot(float64(mobX-selfX), float64(mobY-selfY))
        if dist > bestDist {
            continue
        }
        if !hasLevel {
            level = kiteProbeMaxMobLevel
        }
        if level < bestLevel ||
            (level == bestLevel && dist < bestDist) {
            best = id
            bestLevel = level
            bestDist = dist
        }
    }

    return best, best != 0
}

// kiteProbeLadderBudget bounds the ladder wall time: the rounds
// pause for the cell respawns when the auto attack chain spent the
// in-band mobs, so the budget keeps the whole probe inside its
// timeout no matter how the respawns pace.
const kiteProbeLadderBudget = 150 * time.Second

// kiteProbeTargetWait bounds the respawn wait of one round: the cell
// respawns its mobs every 15-20 s, so a round whose band emptied
// waits one respawn window before it skips.
const kiteProbeTargetWait = 25 * time.Second

// walkLadder walks the delay ladder: every round anchors on the next
// own Attack broadcast of a forced attack, sleeps the round delay and
// sends the retreat click, then observes the movement answer. The
// rounds that lose the anchor (the mob died, the flood gate ate the
// request) are skipped, not measured; the ladder stops at its wall
// budget whatever rounds landed.
func (s *kiteProbeSession) walkLadder() []probeRound {
    var rounds []probeRound
    started := time.Now()
    for _, delay := range kiteProbeLadder {
        if s.ctx.Err() != nil ||
            time.Since(started) > kiteProbeLadderBudget {
            break
        }
        s.settle()
        s.drinkIfNeeded()
        target, ok := s.waitForTarget()
        if !ok {
            continue
        }
        anchor, ok := s.anchorShot(target)
        if !ok {
            continue
        }
        round := s.retreatClickRound(anchor, target, delay)
        rounds = append(rounds, round)
        s.stopWalk()
        // The attack stance keeps shooting (and spending) the cell
        // between the rounds - the cancel keeps the mobs alive for
        // the rounds that follow.
        _ = s.game.ClearTarget()
    }

    return rounds
}

// waitForTarget waits one respawn window for a live target in band
// (the auto attack chain of the earlier rounds may have spent the
// in-band mobs).
func (s *kiteProbeSession) waitForTarget() (int32, bool) {
    deadline := time.Now().Add(kiteProbeTargetWait)
    for {
        if id, ok := s.nearestMob(); ok {
            return id, true
        }
        if time.Now().After(deadline) || s.ctx.Err() != nil {
            return 0, false
        }
        time.Sleep(2 * time.Second)
    }
}

// settle waits for the character to stand still (the previous round
// may still be walking) and lets the broadcast dust settle.
func (s *kiteProbeSession) settle() {
    deadline := time.Now().Add(3 * time.Second)
    for time.Now().Before(deadline) && s.tracker.SelfWalking() {
        time.Sleep(100 * time.Millisecond)
    }
    time.Sleep(400 * time.Millisecond)
}

// drinkIfNeeded drains a healing potion when the health dipped under
// the floor (one per call at most: the item use flood gate).
func (s *kiteProbeSession) drinkIfNeeded() {
    if s.tracker.SelfHealthPercent() >= kiteProbeHealFloor {
        return
    }
    if objectID, ok := s.inventoryObject(1060); ok {
        if err := s.game.UseItem(objectID); err == nil {
            time.Sleep(kiteProbeEquipPace)
        }
    }
}

// anchorShot forces the attack on the target and waits for the own
// Attack broadcast (the server commit of the shot): the first request
// may only select the target and the AI may walk the bow range before
// the first swing, so the wait re-requests inside the flood gate until
// the broadcast lands or the attempts run out.
func (s *kiteProbeSession) anchorShot(
    target int32,
) (time.Time, bool) {
    for attempt := 0; attempt < 3; attempt++ {
        mark := time.Now()
        if err := s.game.AttackTarget(target); err != nil {
            return time.Time{}, false
        }
        deadline := time.Now().Add(kiteProbeAnswerWindow)
        for time.Now().Before(deadline) {
            if at := s.recorder.lastAttackAfter(mark); !at.IsZero() {
                return at, true
            }
            time.Sleep(20 * time.Millisecond)
        }
        // The attempts ride the PlayerAction flood protector (one
        // request per second); the answer window already spaces the
        // pair far past it, the nap covers the tail.
        time.Sleep(200 * time.Millisecond)
    }

    return time.Time{}, false
}

// retreatClickRound runs one measured round: the retreat click at the
// ladder delay after the anchor, the observation of the movement
// answer and the classification.
func (s *kiteProbeSession) retreatClickRound(
    anchor time.Time, target int32, delay time.Duration,
) probeRound {
    round := probeRound{delay: delay, shotAt: anchor}
    selfX, selfY, selfZ, ok := s.tracker.SelfPosition()
    mobX, mobY, _, ok2 := s.tracker.ObjectPosition(target)
    if !ok || !ok2 {
        return round
    }
    time.Sleep(delay)
    endX, endY := retreatEndpoint(selfX, selfY, mobX, mobY)
    walkSent := time.Now()
    if err := s.game.WalkTo(endX, endY, selfZ); err != nil {
        return round
    }
    round.walkSent = walkSent
    movedAt, refusals := s.observeMove(walkSent)
    round.movedAt = movedAt
    round.refusals = refusals
    if !movedAt.IsZero() {
        round.lagFromShot = movedAt.Sub(anchor)
    }
    round.verdict = classifyProbeRound(round)

    return round
}

// observeMove waits for the first own movement broadcast after the
// click and counts the ActionFailed answers that arrived first.
func (s *kiteProbeSession) observeMove(
    walkSent time.Time,
) (time.Time, int) {
    deadline := walkSent.Add(kiteProbeObserveWindow)
    for time.Now().Before(deadline) {
        if s.ctx.Err() != nil {
            break
        }
        movedAt, refusals := s.recorder.firstMoveAfter(walkSent)
        if !movedAt.IsZero() {
            return movedAt, refusals
        }
        time.Sleep(20 * time.Millisecond)
    }
    movedAt, refusals := s.recorder.firstMoveAfter(walkSent)

    return movedAt, refusals
}

// stopWalk cancels the current walk the way the official client does:
// a click onto the own position collapses to a StopMove.
func (s *kiteProbeSession) stopWalk() {
    if x, y, z, ok := s.tracker.SelfPosition(); ok {
        if err := s.game.WalkTo(x, y, z); err == nil {
            time.Sleep(300 * time.Millisecond)
        }
    }
}

// retreatEndpoint computes the retreat click target: the kite step
// length along the ray away from the mob.
func retreatEndpoint(
    selfX, selfY, mobX, mobY int32,
) (int32, int32) {
    dx := float64(selfX - mobX)
    dy := float64(selfY - mobY)
    length := math.Hypot(dx, dy)
    if length < 1 {
        return selfX + int32(kiteProbeRetreatStep), selfY
    }

    return int32(float64(selfX) + dx/length*kiteProbeRetreatStep),
        int32(float64(selfY) + dy/length*kiteProbeRetreatStep)
}

// cursorKeyRound measures the WASD shot-while-moving window: the
// cursor key latch arms right after the shot anchor, the claim stream
// walks the character through the windup, and the round watches the
// own movement, the claim adoptions and the target health (the arrow
// of the anchored shot lands at the windup end no matter where the
// archer walked - the question is whether the claims really walk).
func (s *kiteProbeSession) cursorKeyRound() probeWASDResult {
    result := probeWASDResult{hpBefore: -1, hpAfter: -1}
    s.settle()
    s.drinkIfNeeded()
    target, ok := s.waitForTarget()
    if !ok {
        return result
    }
    anchor, ok := s.anchorShot(target)
    if !ok {
        return result
    }
    result.anchor = anchor
    selfX, selfY, selfZ, ok := s.tracker.SelfPosition()
    mobX, mobY, _, ok2 := s.tracker.ObjectPosition(target)
    if !ok || !ok2 {
        return result
    }
    endX, endY := retreatEndpoint(selfX, selfY, mobX, mobY)
    if err := s.game.CursorKeyWalkTo(endX, endY, selfZ); err != nil {
        return result
    }
    result.hpBefore = s.tracker.ObjectHealthPercent(target)
    windup := kiteProbeWindupFall
    if pAtkSpd := s.tracker.SelfPAtkSpd(); pAtkSpd > 0 {
        windup = time.Duration(500000 / pAtkSpd) * time.Millisecond
    }
    gain := s.streamClaims(anchor, windup, selfZ,
        float64(selfX), float64(selfY), float64(endX), float64(endY))
    result.gainUnits = gain
    result.movedDuringWindup = gain >= 50
    result.adoptions = s.recorder.claimCountAfter(anchor)
    time.Sleep(time.Second)
    result.hpAfter = s.tracker.ObjectHealthPercent(target)
    result.damageLanded = result.hpAfter >= 0 &&
        result.hpBefore >= 0 && result.hpAfter < result.hpBefore
    s.stopWalk()

    return result
}

// streamClaims walks the claimed positions of the cursor key movement
// for the windup window: one claim every claim period, each step the
// run speed unit advance along the retreat ray. The answer is how far
// the own position (the tracker view fed by the broadcasts) moved
// from the anchor by the windup end.
func (s *kiteProbeSession) streamClaims(
    anchor time.Time, windup time.Duration, selfZ int32,
    fromX, fromY, toX, toY float64,
) float64 {
    startX, startY, _, ok := s.tracker.SelfPosition()
    if !ok {
        return 0
    }
    rayX, rayY := toX-fromX, toY-fromY
    rayLen := math.Hypot(rayX, rayY)
    if rayLen < 1 {
        return 0
    }
    heading := int32(0)
    ticker := time.NewTicker(kiteProbeClaimPeriod)
    defer ticker.Stop()
    deadline := anchor.Add(windup)
    for now := range ticker.C {
        if now.After(deadline) || s.ctx.Err() != nil {
            break
        }
        step := float64(now.Sub(anchor)) / float64(time.Millisecond) *
            kiteProbeClaimStep / float64(kiteProbeClaimPeriod)
        if step > rayLen {
            step = rayLen
        }
        claimX := int32(fromX + rayX/rayLen*step)
        claimY := int32(fromY + rayY/rayLen*step)
        if err := s.game.ClaimValidatePosition(claimX, claimY, selfZ,
            heading); err != nil {
            break
        }
    }
    endX, endY, _, ok := s.tracker.SelfPosition()
    if !ok {
        return 0
    }

    return math.Hypot(float64(endX-startX), float64(endY-startY))
}
