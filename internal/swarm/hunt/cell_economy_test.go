// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The ground economy tests: the adena gap scale of the server tables, the
// window economy of a real registry ground and the damage rate meter that
// prices the fight time (cell_economy.go).

// economyGround builds one test ground of the named template ids with the
// uniform respawn window of the registry.
func economyGround(id string, mobs []CellMob) Cell {
    return Cell{
        ID: id, Name: id, Region: "elven",
        RespawnMin: 15, RespawnMax: 20, Mobs: mobs,
    }
}

// coldCombatRate empties the process wide damage meter around a test.
func coldCombatRate(t *testing.T) {
    t.Helper()
    globalCombatRate.reset()
    t.Cleanup(globalCombatRate.reset)
}

// warmCombatRate fills the meter with an exact rate: the decay cancels for
// identical samples.
func warmCombatRate(t *testing.T, rate float64) {
    t.Helper()
    coldCombatRate(t)
    for range combatRateMinSamples {
        globalCombatRate.add(100, 100, 100/rate)
    }
    require.InDelta(t, rate, globalCombatRate.rate(), 1e-6)
}

// TestAdenaGapChanceScale pins the DropAdena* scale of Rates.ini: the full
// drop inside the gap of eight levels, the linear fall to a tenth at fifteen.
func TestAdenaGapChanceScale(t *testing.T) {
    cases := []struct {
        mobLevel int32
        want     float64
    }{
        {24, 1},
        {15, 1},
        {14, 0.8714285714285714},
        {12, 0.6142857142857143},
        {8, 0.10},
        {1, 0.10},
    }
    for _, test := range cases {
        require.InDelta(t, test.want, adenaGapChance(23, test.mobLevel),
            1e-9, "mob level %d", test.mobLevel)
    }
}

// TestCellWindowEconomyPricesTheGroundMobs pins the window mean of a real
// ground: the level 23 window holds only the level 15 Crimson Spider, the
// level 22 window adds the level 14 Dryad Elder at the full drop.
func TestCellWindowEconomyPricesTheGroundMobs(t *testing.T) {
    ground := economyGround("test-dryad-elder", []CellMob{
        {TemplateID: 20460, Name: "Crimson Spider", Level: 15, Count: 1},
        {TemplateID: 20019, Name: "Dryad Elder", Level: 14, Count: 1},
    })
    economy, ok := cellWindowEconomy(ground, 23)
    require.True(t, ok)
    require.InDelta(t, 1.0, economy.mass, 1e-9)
    require.InDelta(t, 246.9542, economy.hp, 1e-4)
    require.InDelta(t, 73.0, economy.pdef, 1e-9)
    require.InDelta(t, 32.0, economy.patk, 1e-9)
    require.InDelta(t, 84.70, economy.adena, 1e-9)

    economy, ok = cellWindowEconomy(ground, 22)
    require.True(t, ok)
    require.InDelta(t, 2.0, economy.mass, 1e-9)
    require.InDelta(t, 238.66985, economy.hp, 1e-4)
    require.InDelta(t, 75.5, economy.pdef, 1e-9)
    require.InDelta(t, 29.5, economy.patk, 1e-9)
    require.InDelta(t, 83.125, economy.adena, 1e-9)
}

// TestCellWindowEconomySkipsUnpricedMobs covers the empty answers: an
// unknown template, a zero count and a window outside every mob.
func TestCellWindowEconomySkipsUnpricedMobs(t *testing.T) {
    unknown := economyGround("test-unknown", []CellMob{
        {TemplateID: 999999999, Name: "Ghost", Level: 15, Count: 2},
        {TemplateID: 20460, Name: "Crimson Spider", Level: 15, Count: 0},
    })
    _, ok := cellWindowEconomy(unknown, 23)
    require.False(t, ok)

    below := economyGround("test-below", []CellMob{
        {TemplateID: 20534, Name: "Red Keltir", Level: 2, Count: 4},
    })
    _, ok = cellWindowEconomy(below, 23)
    require.False(t, ok)
}

// TestGroundKillCapacityPricesTheFight pins the capacity of a ground before
// and after the meter warms: the flat solo rate first, the fight at the
// measured damage rate plus the loot and risk overhead after.
func TestGroundKillCapacityPricesTheFight(t *testing.T) {
    coldCombatRate(t)
    require.InDelta(t, cellKillCapacity, groundKillCapacity(cellEconomy{}),
        1e-9)

    warmCombatRate(t, 19)
    ground := cellEconomy{hp: 246.9542, pdef: 73, patk: 32}
    // The fight 246.9542 * 73 / 100 / 19 = 9.4882 s, the overhead 6 s and
    // the risk 0.3 * 32 = 9.6 s.
    require.InDelta(t, 60.0/25.088240, groundKillCapacity(ground), 1e-4)
}

// TestCellValuePrefersTheFinalAdenaBand is the run6 correction: at level 23
// the level 15 ground pays more per minute than the Lirein 18/19 ground the
// level priced prior chose, because the heavier fight eats the richer drop.
func TestCellValuePrefersTheFinalAdenaBand(t *testing.T) {
    warmCombatRate(t, 19)
    dryad := economyGround("test-dryad-elder", []CellMob{
        {TemplateID: 20460, Name: "Crimson Spider", Level: 15, Count: 1},
        {TemplateID: 20019, Name: "Dryad Elder", Level: 14, Count: 1},
    })
    lirein := economyGround("test-lirein", []CellMob{
        {TemplateID: 20044, Name: "Lirein Elder", Level: 19, Count: 1},
        {TemplateID: 20036, Name: "Lirein", Level: 18, Count: 1},
        {TemplateID: 20466, Name: "Pincer Spider", Level: 17, Count: 1},
    })
    hunter := &cellHunter{cells: []Cell{dryad, lirein}, level: 23}
    dryadValue := hunter.cellValue(&dryad, &cellMetric{})
    lireinValue := hunter.cellValue(&lirein, &cellMetric{})
    require.Greater(t, dryadValue, lireinValue)
    require.Greater(t, dryadValue, lireinValue*1.05)
    require.Less(t, dryadValue, lireinValue*1.15)
}

// TestNoteCellCombatFeedsTheMeter pins the kill hook: the held ground prices
// the fight and a missing ground or a zero fight feeds nothing.
func TestNoteCellCombatFeedsTheMeter(t *testing.T) {
    coldCombatRate(t)
    ground := economyGround("test-dryad-elder", []CellMob{
        {TemplateID: 20460, Name: "Crimson Spider", Level: 15, Count: 1},
    })
    loop := NewLoop(&fakeGame{}, newTestBot())
    loop.cell = &cellHunter{cells: []Cell{ground}, picked: 0, level: 23}
    loop.noteCellCombat(0)
    samples, _, _ := globalCombatRate.snapshot()
    require.Equal(t, 0, samples)

    loop.noteCellCombat(4)
    samples, hp, seconds := globalCombatRate.snapshot()
    require.Equal(t, 1, samples)
    require.InDelta(t, 246.9542*0.73, hp, 1e-4)
    require.InDelta(t, 4.0, seconds, 1e-9)

    loop.cell.picked = -1
    loop.noteCellCombat(4)
    samples, _, _ = globalCombatRate.snapshot()
    require.Equal(t, 1, samples)
}
