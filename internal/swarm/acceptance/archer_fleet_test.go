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

// TestFleetSpawnZFallback pins the oracle fallback: a nil engine and
// an out-of-band answer both fall back to the proven archer height.
func TestFleetSpawnZFallback(t *testing.T) {
    require.EqualValues(t, archerKiteSpawnZ,
        fleetSpawnZ(nil, 36000, 50229))
}

// fleetSampleStream builds the sample timeline of one fleet slot: a
// shot every cycle, a retreat walk that starts retreatLag after the
// shot and runs a second and a half, the fight samples filling the
// rest. The fight target stands east of the anchor and every retreat
// walk moves away from it (the direction gate of the fold): straight
// west for the legacy stream, bending around the anchor - the curved
// retreat the audit wants to see - for the improved one.
func fleetSampleStream(
    cycle time.Duration, retreatLag time.Duration,
    fightDist float64, anchor [2]int32, curved bool,
) []fleetSample {
    base := time.Now().Add(-time.Minute)
    target := [2]int32{anchor[0] + 600, anchor[1]}
    const walkDur = 1500 * time.Millisecond
    var samples []fleetSample
    // The persistent walk state: the character stands where its last
    // retreat left it - the arc angle around the anchor (the curved
    // stream circles it) or the straight west offset.
    angle, offsetX := 0.0, 300.0
    for shot := 0; shot < 4; shot++ {
        shotAt := base.Add(time.Duration(shot) * cycle)
        walkAt := shotAt.Add(retreatLag)
        for step := 0; step < 8; step++ {
            at := shotAt.Add(time.Duration(step) * cycle / 8)
            sample := fleetSample{
                at: at, shotAt: shotAt, fighting: true,
                fightDist: fightDist, hasPos: true, hasTarget: true,
                tx: target[0], ty: target[1], hpPct: 90,
            }
            // The retreat displacement: the away half-plane of the
            // target, straight west or the anchor-bending arc.
            phase := 0.0
            if at.After(walkAt) {
                phase = at.Sub(walkAt).Seconds() /
                    walkDur.Seconds()
                if phase > 1 {
                    phase = 1
                }
            }
            if curved {
                cur := angle - 160.0*phase*math.Pi/180
                sample.x = anchor[0] + int32(300*math.Cos(cur))
                sample.y = anchor[1] + int32(300*math.Sin(cur))
            } else {
                sample.x = anchor[0] + int32(offsetX-400*phase)
                sample.y = anchor[1]
            }
            sample.walking = at.After(walkAt) && at.Before(walkAt.Add(walkDur))
            samples = append(samples, sample)
        }
        angle -= 160.0 * math.Pi / 180
        offsetX -= 400
    }

    return samples
}

// TestFleetFoldImprovedKite pins the fold against an improved stream:
// the retreat lags ride the accepted window, the re-shots land fast,
// the fights stay beyond the floor and the turns show a curve.
func TestFleetFoldImprovedKite(t *testing.T) {
    watch := newFleetWatch(36000, 50229)
    for _, sample := range fleetSampleStream(
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
}

// TestFleetFoldLegacyKite pins the fold against the legacy stream:
// the retreat waits the deferred replay (the full reload), so the
// early-retreat behavior fails exactly the way the audit must name.
func TestFleetFoldLegacyKite(t *testing.T) {
    watch := newFleetWatch(36000, 50229)
    for _, sample := range fleetSampleStream(
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
