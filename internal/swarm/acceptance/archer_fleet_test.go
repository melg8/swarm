// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "math"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestArcherFleetRegistered pins the registration of the fleet audit:
// the id, the first account and the five minute budget.
func TestArcherFleetRegistered(t *testing.T) {
    defs := Definitions()
    var found bool
    for _, def := range defs {
        if def.ID == archerFleetScenarioID {
            found = true
            require.Equal(t, archerFleetSlots[0].Account, def.Account)
            require.Less(t, def.Timeout, 5*time.Minute,
                "the fleet audit stays under the five minute ask")
        }
    }
    require.True(t, found, "the fleet audit must be registered")
}

// TestArcherFleetSlots pins the five cell spread: five temp accounts,
// five distinct cells and five distinct foci.
func TestArcherFleetSlots(t *testing.T) {
    require.Len(t, archerFleetSlots, 5)
    cells := map[string]bool{}
    foci := map[[2]int32]bool{}
    accounts := map[string]bool{}
    for _, slot := range archerFleetSlots {
        cells[slot.Cell] = true
        foci[[2]int32{slot.X, slot.Y}] = true
        accounts[slot.Account] = true
        require.Equal(t, slot.Account, archerFleetPassword(slot.Account))
    }
    require.Len(t, cells, 5, "the slots sit on distinct cells")
    require.Len(t, foci, 5, "the slots sit on distinct foci")
    require.Len(t, accounts, 5, "the slots ride distinct accounts")
}

// TestArcherFleetReset pins the reset of one slot: the archer kit on
// the slot focus with the oracle z.
func TestArcherFleetReset(t *testing.T) {
    slot := archerFleetSlots[1]
    reset := archerFleetReset(slot, -3500)
    require.Equal(t, slot.Account, reset.Account)
    require.Equal(t, slot.X, reset.X)
    require.Equal(t, slot.Y, reset.Y)
    require.Equal(t, int32(-3500), reset.Z)
    require.NotEmpty(t, reset.Items)
}

// TestArcherFleetTimeoutStaysUnderTheAsk pins the scenario bound: the
// three minute window plus the fleet's own launch budget and the
// teardown grace must stay under the five minute ask (the arithmetic
// lives in the constants - the pin keeps a future window bump from
// silently breaking the contract).
func TestArcherFleetTimeoutStaysUnderTheAsk(t *testing.T) {
    require.LessOrEqual(t, archerFleetTimeout(), 5*time.Minute,
        "the whole fleet scenario stays under the five minute ask")
}

// TestArcherFleetResetWakesAtFullVitals pins the wounded-start fix:
// the fleet slots wake at the full server-computed level 7 vitals -
// the round of 2026-09-23 measured a slot entering at 167 of 214 HP
// (the gain-table start the server maxima outrank) spending the
// window on potions, an emergency logout and a 60 second sit, every
// kite check reading as not passing with zero evidence. The
// single-bot archer scenario keeps its own gain-table start.
func TestArcherFleetResetWakesAtFullVitals(t *testing.T) {
    reset := archerFleetReset(archerFleetSlots[0], -3456)
    require.Equal(t, int32(fleetLevel7MaxHP), reset.MaxHP,
        "the fleet slots wake at the full server-computed HP")
    require.Equal(t, int32(fleetLevel7MaxMP), reset.MaxMP,
        "the fleet slots wake at the full server-computed MP")

    single := archerKiteReset("probe")
    require.Equal(t, int32(archerKiteStartHP), single.MaxHP,
        "the single-bot archer scenario keeps its gain-table start")
}

// TestFleetSpawnZFallback pins the oracle fallback: a nil engine and
// an out-of-band answer both fall back to the proven archer height.
func TestFleetSpawnZFallback(t *testing.T) {
    require.EqualValues(t, archerKiteSpawnZ,
        fleetSpawnZ(nil, 36000, 50229))
}

// The geometry knobs of the synthetic kite streams: the retreat
// length (the hunt's kiteStep), the mob's trailing gap at the fight
// start and the curving bend of the improved stream (the hunt's
// kiteCurveStep).
const (
    fleetStreamStep  = 400.0
    fleetStreamTrail = 250.0
    fleetStreamBend  = 70.0 * math.Pi / 180
)

// fleetKiteCycles emits the samples of one synthetic kite fight: a
// shot every cycle, a retreat walk that starts retreatLag after the
// shot and runs a second and a half, the fight samples filling the
// rest, and a standing tail that closes the last walk. The fight
// target TRAILS the character one retreat behind - the chasing mob
// of a real kite fight, whose stand the fold's drift anchor latches
// - so the away-ray of every retreat points from the mob's position
// and the direction gate confirms the orbit chords the way the live
// fight produces them. The heading bends -bend every cycle (the
// curving circle of the improved kite) or stays fixed at bend 0
// (the straight runaway that marches off the farm point). The
// answer carries the character's position after the last retreat so
// the caller chains the next fight or a between-fights cell hop
// onto it.
func fleetKiteCycles(
    base time.Time, cycle, retreatLag time.Duration,
    fightDist float64, fight int32, start [2]int32,
    heading, bend float64, cycles int,
) ([]fleetSample, [2]int32) {
    const walkDur = 1500 * time.Millisecond
    px, py := float64(start[0]), float64(start[1])
    // The mob's stand at the fight start: the trailing gap behind
    // the opening retreat heading (the chase the fight opened on).
    mobX := px - fleetStreamTrail*math.Cos(heading)
    mobY := py - fleetStreamTrail*math.Sin(heading)
    var samples []fleetSample
    for shot := range cycles {
        shotAt := base.Add(time.Duration(shot) * cycle)
        walkAt := shotAt.Add(retreatLag)
        fromX, fromY := px, py
        toX := px + fleetStreamStep*math.Cos(heading)
        toY := py + fleetStreamStep*math.Sin(heading)
        for step := 0; step < 8; step++ {
            at := shotAt.Add(time.Duration(step) * cycle / 8)
            phase := 0.0
            if at.After(walkAt) {
                phase = at.Sub(walkAt).Seconds() /
                    walkDur.Seconds()
                if phase > 1 {
                    phase = 1
                }
            }
            samples = append(samples, fleetSample{
                at: at, shotAt: shotAt, fighting: true,
                targetID: fight, hasTarget: true,
                tx: int32(mobX), ty: int32(mobY),
                fightDist: fightDist, hasPos: true, hpPct: 90,
                walking: at.After(walkAt) &&
                    at.Before(walkAt.Add(walkDur)),
                x: int32(fromX + (toX-fromX)*phase),
                y: int32(fromY + (toY-fromY)*phase),
            })
        }
        // The mob trails to the stand this cycle fought from once
        // the retreat leaves it (the chase of the next cycle).
        mobX, mobY = fromX, fromY
        px, py = toX, toY
        heading -= bend
    }
    // The standing tail: the last retreat walk needs a non-walking
    // sample after its end to confirm.
    tail := base.Add(time.Duration(cycles) * cycle)
    for i := range 2 {
        samples = append(samples, fleetSample{
            at:       tail.Add(time.Duration(i) * cycle / 8),
            shotAt:   base.Add(time.Duration(cycles-1) * cycle),
            fighting: true, targetID: fight, hasTarget: true,
            tx: int32(mobX), ty: int32(mobY),
            fightDist: fightDist, hasPos: true, hpPct: 90,
            x: int32(px), y: int32(py),
        })
    }

    return samples, [2]int32{int32(px), int32(py)}
}

// fleetKiteStream answers the one-fight timeline of a whole window:
// the improved kite (the 70 degree bend) or the legacy runaway.
func fleetKiteStream(
    cycle time.Duration, retreatLag time.Duration,
    fightDist float64, start [2]int32, curved bool,
) []fleetSample {
    bend := 0.0
    if curved {
        bend = fleetStreamBend
    }
    samples, _ := fleetKiteCycles(time.Now().Add(-time.Minute),
        cycle, retreatLag, fightDist, 1, start, math.Pi, bend, 5)

    return samples
}

// TestFleetFoldImprovedKite pins the fold against an improved
// stream: the retreat lags ride the accepted window, the re-shots
// land fast, the fights stay beyond the floor, the retreat corners
// bend one way and the fight drift stays inside the leash.
func TestFleetFoldImprovedKite(t *testing.T) {
    watch := newFleetWatch()
    for _, sample := range fleetKiteStream(
        3*time.Second, 1600*time.Millisecond, 380.0,
        [2]int32{36000, 50229}, true,
    ) {
        watch = watch.fold(sample)
    }
    verdicts := watch.verdicts()
    require.True(t, verdicts.maxRange, verdicts.maxRangeD)
    require.True(t, verdicts.shoots, verdicts.shootsD)
    require.True(t, verdicts.early, verdicts.earlyD)
    require.True(t, verdicts.reshot, verdicts.reshotD)
    require.True(t, verdicts.curve, verdicts.curveD)
    require.Len(t, watch.kiteFightDrifts(), 1,
        "one fight ran through the stream")
}

// TestFleetFoldLegacyKite pins the fold against the legacy stream:
// the retreat waits the deferred replay (the full reload), so the
// early-retreat behavior fails exactly the way the audit must name,
// the fights sit at melee range and the straight runaway never
// bends a corner.
func TestFleetFoldLegacyKite(t *testing.T) {
    watch := newFleetWatch()
    for _, sample := range fleetKiteStream(
        6*time.Second, 3200*time.Millisecond, 200.0,
        [2]int32{36000, 50229}, false,
    ) {
        watch = watch.fold(sample)
    }
    verdicts := watch.verdicts()
    require.False(t, verdicts.early,
        "the deferred replay must fail the early-retreat behavior")
    require.False(t, verdicts.maxRange,
        "the melee-range stream must fail the max-range behavior")
    require.False(t, verdicts.curve,
        "the straight runaway must fail the curved-retreat behavior")
    require.True(t, verdicts.shoots)
}

// TestMedianDuration pins the median helpers on even and odd slices.
func TestMedianDuration(t *testing.T) {
    require.Zero(t, medianDuration(nil))
    require.Equal(t, 2*time.Second,
        medianDuration([]time.Duration{time.Second, 2 * time.Second}))
    require.Equal(t, 2*time.Second, medianDuration([]time.Duration{
        time.Second, 3 * time.Second, 2 * time.Second,
    }))
    require.Zero(t, medianFloat(nil))
    require.InDelta(t, 400.0, medianFloat([]float64{100, 400}), 0.01)
}

// TestFleetFoldDropsTheFleetNoiseWalks pins the reject path of the
// direction gate: walks that move toward the fight target (the
// chase-stall steps) or start outside a fight (the loot pickups,
// the cell-rotation approaches) never feed the retreat medians -
// the exact pollution the first live round measured into a 3.1 s
// reshot median on walks the kite never issued.
func TestFleetFoldDropsTheFleetNoiseWalks(t *testing.T) {
    watch := newFleetWatch()
    base := time.Now().Add(-time.Minute)
    target := [2]int32{36600, 50229}
    sample := func(at time.Time, walking bool, x, y int32,
        fighting bool, shotAt time.Time,
    ) fleetSample {
        targetID := int32(0)
        if fighting {
            targetID = 1
        }

        return fleetSample{
            at: at, walking: walking, fighting: fighting,
            targetID: targetID, hasPos: true, hasTarget: fighting,
            fightDist: 300,
            tx:        target[0], ty: target[1], shotAt: shotAt, hpPct: 90,
            x: x, y: y,
        }
    }
    // The chase walk of the first fight: fighting, but the
    // displacement runs TOWARD the target.
    walk := base.Add(time.Second)
    for i := range 4 {
        at := walk.Add(time.Duration(i) * 250 * time.Millisecond)
        watch = watch.fold(sample(at, true, 36000+int32(i)*80,
            50229, true, base))
    }
    watch = watch.fold(sample(walk.Add(time.Second), false, 36320,
        50229, true, base))
    // The shot after the chase walk (the would-be reshot gap).
    shot2 := base.Add(3 * time.Second)
    watch = watch.fold(sample(shot2, false, 36320, 50229, true, shot2))
    // The loot walk: no fight, any direction.
    loot := base.Add(4 * time.Second)
    for i := range 4 {
        at := loot.Add(time.Duration(i) * 250 * time.Millisecond)
        watch = watch.fold(sample(at, true, 36320-int32(i)*80,
            50229, false, shot2))
    }
    watch = watch.fold(sample(loot.Add(time.Second), false, 36000,
        50229, false, shot2))
    shot3 := base.Add(6 * time.Second)
    watch = watch.fold(sample(shot3, false, 36000, 50229, true, shot3))

    require.Equal(t, 3, watch.shots)
    require.Empty(t, watch.retreatLags,
        "the toward-target chase walk must not feed the retreat lags")
    require.Empty(t, watch.reshotGaps,
        "the noise walks must not feed the reshot gaps")
}

// TestFleetFoldLeashIgnoresTheCellRotation pins the leash
// attribution fix of the curve round: the drift measures one fight
// at a time against the mob's stand that fight opened on, so a
// 2000 unit cell hop BETWEEN two kite fights (the hunt's own cell
// rotation) never reaches the leash. The spawn-anchor metric of
// the first live rounds measured exactly that hop into a broken
// leash at 1644-2367 units on four slots.
func TestFleetFoldLeashIgnoresTheCellRotation(t *testing.T) {
    watch := newFleetWatch()
    base := time.Now().Add(-time.Minute)
    const cycle = 3 * time.Second
    const lag = 1600 * time.Millisecond
    start := [2]int32{36000, 50229}
    // The first kite fight: three curving retreats around its own
    // opening stand.
    first, end := fleetKiteCycles(base, cycle, lag, 380.0, 1,
        start, math.Pi, fleetStreamBend, 3)
    for _, sample := range first {
        watch = watch.fold(sample)
    }
    // The cell rotation: the hunt's approach walk to the next
    // cell, 2000 units east, outside any fight.
    hopAt := base.Add(3*cycle + time.Second)
    for i := range 6 {
        at := hopAt.Add(time.Duration(i) * 250 * time.Millisecond)
        watch = watch.fold(fleetSample{
            at: at, walking: i < 5, hasPos: true, hpPct: 90,
            x: end[0] + int32(i)*400, y: end[1],
            shotAt: base.Add(2 * cycle),
        })
    }
    // The second kite fight opens on the new cell's stand: the
    // same curving circle, a fresh drift anchor.
    secondStart := [2]int32{end[0] + 2000, end[1]}
    second, _ := fleetKiteCycles(hopAt.Add(2*time.Second), cycle,
        lag, 380.0, 2, secondStart, math.Pi, fleetStreamBend, 3)
    for _, sample := range second {
        watch = watch.fold(sample)
    }

    verdicts := watch.verdicts()
    require.True(t, verdicts.curve, verdicts.curveD,
        "the cell hop between the fights must not break the leash")
    drifts := watch.kiteFightDrifts()
    require.Len(t, drifts, 2,
        "both fights feed the leash, one drift each")
    for _, drift := range drifts {
        require.Less(t, drift, fleetFightDriftLeash,
            "each fight's drift measures its own stand")
    }
}

// TestFleetFoldCornerNeedsTheSameFight pins the corner chain's
// fight gate: retreats of two different fights never measure a
// corner between them - the geometry across a fight switch is the
// pick walk of the hunt, not the kite curve.
func TestFleetFoldCornerNeedsTheSameFight(t *testing.T) {
    watch := newFleetWatch()
    base := time.Now().Add(-time.Minute)
    const cycle = 3 * time.Second
    const lag = 1600 * time.Millisecond
    start := [2]int32{36000, 50229}
    // Two one-retreat fights at right angles: the corner between
    // the two retreat vectors would be 90 degrees if the chain
    // ignored the fight switch - the gate must not count it.
    first, end := fleetKiteCycles(base, cycle, lag, 380.0, 1,
        start, math.Pi, 0, 1)
    for _, sample := range first {
        watch = watch.fold(sample)
    }
    secondStart := [2]int32{end[0] + 2000, end[1]}
    second, _ := fleetKiteCycles(base.Add(2*cycle), cycle, lag,
        380.0, 2, secondStart, math.Pi/2, 0, 1)
    for _, sample := range second {
        watch = watch.fold(sample)
    }

    require.Zero(t, watch.curveCorners(),
        "the cross-fight corner must not count")
    require.Len(t, watch.kiteFightDrifts(), 2)
}
