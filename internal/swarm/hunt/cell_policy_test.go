// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The cell policy tests: a three cell mesh (a home ground at the
// character position, a rich direct neighbor a walk away, a farther
// alternative behind it) drives the wait-or-rotate economy - the
// ripeness pacing, the neighbor-first sweep, the starvation
// livelock net, the death regression, the income attribution and
// the relogin handoff.

// quadCell builds one convex test cell: a counter-clockwise square
// polygon of the given extent around the focus, the mob list and
// the neighbor indices.
func quadCell(
    id string, name string, fx int32, fy int32, extent int32,
    neighbors []int32, mobs []CellMob,
) Cell {
    return Cell{
        ID: id, Name: name, Region: "elven",
        MinLevel: mobs[0].Level, MaxLevel: mobs[0].Level,
        FocusX: fx, FocusY: fy, PatrolHalf: extent / 2,
        RespawnMin: 15, RespawnMax: 20, Mass: 0,
        Mobs: mobs, Vertices: []CellVertex{
            {X: fx - extent, Y: fy - extent},
            {X: fx + extent, Y: fy - extent},
            {X: fx + extent, Y: fy + extent},
            {X: fx - extent, Y: fy + extent},
        },
        Neighbors: neighbors,
    }
}

// homeMobs are the village keltirs: the level 6 window floor admits
// them, the white-green preference biases them.
func homeMobs() []CellMob {
    return []CellMob{
        {TemplateID: 20534, Name: "Red Keltir", Level: 2, Count: 4,
            RespawnMin: 15, RespawnMax: 20},
        {TemplateID: 20537, Name: "Elder Red Keltir", Level: 4, Count: 2,
            RespawnMin: 15, RespawnMax: 20},
    }
}

// richMobs are the wolves of the neighbor cell: the same window,
// the double mass.
func richMobs() []CellMob {
    return []CellMob{
        {TemplateID: 20014, Name: "Gray Wolf", Level: 6, Count: 8,
            RespawnMin: 15, RespawnMax: 20},
        {TemplateID: 20017, Name: "Elder Wolf", Level: 8, Count: 4,
            RespawnMin: 15, RespawnMax: 20},
    }
}

// poorMobs are the sparse wolves of the far cell: the escape valve
// of the starved sweep, never the economic winner.
func poorMobs() []CellMob {
    return []CellMob{
        {TemplateID: 20014, Name: "Gray Wolf", Level: 6, Count: 2,
            RespawnMin: 15, RespawnMax: 20},
    }
}

// testCells builds the three cell mesh of the policy tests: the
// home ground at the character position, the rich direct neighbor
// 1600 units east, a poor far alternative 5000 units north behind
// the neighbor (the escape valve of the sweeps, never the economic
// winner while a ripe neighbor exists).
func testCells() []Cell {
    cells := []Cell{
        quadCell("test-home", "Home Keltirs", 46112, 41500, 800,
            []int32{1}, homeMobs()),
        quadCell("test-rich", "Rich Wolves", 47712, 41500, 900,
            []int32{0, 2}, richMobs()),
        quadCell("test-far", "Far Wolves", 46112, 46500, 900,
            []int32{1}, poorMobs()),
    }
    for index := range cells {
        mass := 0.0
        for mob := range cells[index].Mobs {
            mass += float64(cells[index].Mobs[mob].Count)
        }
        cells[index].Mass = mass
        cells[index].MinLevel = cells[index].Mobs[0].Level
        cells[index].MaxLevel = cells[index].Mobs[0].Level
        for _, mob := range cells[index].Mobs {
            cells[index].MinLevel = min(cells[index].MinLevel, mob.Level)
            cells[index].MaxLevel = max(cells[index].MaxLevel, mob.Level)
        }
    }

    return cells
}

// cellTestBot prepares a level 6 character standing on the home
// focus.
func cellTestBot(t *testing.T) *state.Bot {
    t.Helper()
    bot := newTestBot()
    setZoneTestLevel(bot, 6)

    return bot
}

// pickedCellLoop prepares a hunting loop in the cell mode with the
// home ground held. The fleet claim releases when the test ends.
func pickedCellLoop(t *testing.T) (*Loop, *fakeGame, *cellHunter) {
    t.Helper()
    bot := cellTestBot(t)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells(testCells())
    loop.tick()
    require.Equal(t, "test-home", loop.zonePickedID)
    require.NotNil(t, loop.cell)
    require.GreaterOrEqual(t, loop.cell.picked, 0)
    hunter := loop.cell
    t.Cleanup(func() {
        if hunter.leave != nil {
            hunter.leave()
        }
    })

    return loop, game, hunter
}

func TestCellPickAppliesLeashAndWindow(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    // The patrol square of the loop mirrors the held cell, the
    // tracker zone follows it, the polygon leash fences the ground.
    cell := hunter.cells[hunter.picked]
    require.Equal(t, cell.FocusX, loop.zoneCX)
    require.Equal(t, cell.FocusY, loop.zoneCY)
    require.Equal(t, cell.PatrolHalf, loop.zoneHalf)
    require.NotNil(t, hunter.leash)
    require.True(t, hunter.leash.Contains(46112, 41500))
    require.False(t, hunter.leash.Contains(46112+801, 41500))
    // The window biased priorities of the ground mirror into the
    // engage: the level 2 keltirs read priority 3 (the white-green
    // subrange of a level 6 character).
    require.Equal(t, map[int32]int32{
        npcdata.NPCWireTemplateID(20534): 3,
        npcdata.NPCWireTemplateID(20537): 3,
    }, hunter.priority)
    require.Equal(t, 0, globalCellHub.occupancy("test-rich"))
    require.Equal(t, 1, globalCellHub.occupancy("test-home"))
}

func TestCellWaitsForPredictedRespawn(t *testing.T) {
    loop, game, hunter := pickedCellLoop(t)
    now := time.Now()
    // A kill of the home ground predicts the respawn in 17.5 s: the
    // emptiness holds the ground (the patience window), the drift
    // walk paces toward the corpse position.
    hunter.kills = append(hunter.kills, killRecord{
        cellID: "test-home", x: 46800, y: 41800, at: now,
        respawnAt: now.Add(17 * time.Second),
    })
    hunter.emptySince = now.Add(-2 * time.Second)
    loop.cellEvaluate(now)
    require.Equal(t, "test-home", loop.zonePickedID)
    // The drift walk issued one leg toward the corpse position.
    require.Len(t, game.walks, 1)
}

func TestCellStarvedRotatesToNeighbor(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    // The ground reads empty, the overlay holds nothing, the stay
    // and the emptiness outran the starved thresholds: the rotation
    // moves to the RICH NEIGHBOR (the 1-hop ring), not the far
    // alternative - the sweep stays local.
    hunter.emptySince = now.Add(-70 * time.Second)
    hunter.enteredAt = now.Add(-2 * time.Minute)
    loop.cellEvaluate(now)
    require.Equal(t, "test-rich", loop.zonePickedID)
    require.Equal(t, 1, globalCellHub.occupancy("test-rich"))
    require.Equal(t, 0, globalCellHub.occupancy("test-home"))
}

func TestCellHysteresisHoldsComparableGround(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    // A starved ground but a short stay: the ground keeps its post
    // until the exhaustion floor passes.
    hunter.emptySince = now.Add(-70 * time.Second)
    hunter.enteredAt = now.Add(-10 * time.Second)
    loop.cellEvaluate(now)
    require.Equal(t, "test-home", loop.zonePickedID)
    // A longer stay but a fresh emptiness: also no switch.
    hunter.enteredAt = now.Add(-2 * time.Minute)
    hunter.emptySince = now.Add(-5 * time.Second)
    loop.cellEvaluate(now)
    require.Equal(t, "test-home", loop.zonePickedID)
}

func TestCellRotationPacesByRipeness(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    // The home ground starves: the rotation moves to the rich
    // neighbor and the left ground starts its ripeness clock (the
    // respawn window refills it 20 s later).
    hunter.emptySince = now.Add(-70 * time.Second)
    hunter.enteredAt = now.Add(-2 * time.Minute)
    loop.cellEvaluate(now)
    require.Equal(t, "test-rich", loop.zonePickedID)
    require.False(t, hunter.ripe(hunter.cells[0], now.Add(10*time.Second)))
    require.True(t, hunter.ripe(hunter.cells[0], now.Add(21*time.Second)))
    // The character stands on the rich ground; it starves 10 s
    // later. The home neighbor sits INSIDE its unripe window: the
    // rotation takes the farther neighbor instead of walking back
    // into the ground the respawn has not refilled yet - the
    // depletion cycle of the circle geometry cannot build.
    loop.tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 47712, Y: 41500, Z: -3400,
    })
    hunter.readSelf(loop)
    early := now.Add(10 * time.Second)
    hunter.emptySince = early.Add(-70 * time.Second)
    hunter.enteredAt = early.Add(-2 * time.Minute)
    loop.cellEvaluate(early)
    require.Equal(t, "test-far", loop.zonePickedID,
        "the rotation skips the unripe home ground")
}

func TestCellRipeReturnResumesClearedGround(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    // The ECONOMIC rotation leaves the home ground after the fair
    // trial: the rich neighbor outscored it by the margin (45 s of
    // emptiness - past the no-data patience, short of the
    // starvation threshold - so the left ground ripens WITHOUT the
    // starve cooldown).
    hunter.emptySince = now.Add(-45 * time.Second)
    hunter.enteredAt = now.Add(-6 * time.Minute)
    loop.cellEvaluate(now)
    require.Equal(t, "test-rich", loop.zonePickedID)
    require.True(t, hunter.ripe(hunter.cells[0], now.Add(21*time.Second)))
    require.False(t, hunter.starveCoolingDown("test-home",
        now.Add(30*time.Second)))
    // The rich ground starves half a minute later: the home neighbor
    // RIPENED (its respawn window passed) and carries no starve
    // cooldown - the rotation takes it back. The crop rotation
    // closes the loop at the respawn pace, no ground starves while
    // another saturates.
    ripe := now.Add(30 * time.Second)
    loop.tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 47712, Y: 41500, Z: -3400,
    })
    hunter.readSelf(loop)
    hunter.emptySince = ripe.Add(-70 * time.Second)
    hunter.enteredAt = ripe.Add(-2 * time.Minute)
    loop.cellEvaluate(ripe)
    require.Equal(t, "test-home", loop.zonePickedID)
}

func TestCellStarvedCooldownBreaksTheLivelock(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    bot := loop.tracker
    now := time.Now()
    // The home ground starves: the rotation moves to the rich
    // neighbor and the left ground starts its starvation cooldown.
    hunter.emptySince = now.Add(-70 * time.Second)
    hunter.enteredAt = now.Add(-2 * time.Minute)
    loop.cellEvaluate(now)
    require.Equal(t, "test-rich", loop.zonePickedID)
    require.True(t, hunter.starveCoolingDown("test-home", now))
    require.False(t, hunter.starveCoolingDown("test-rich", now))
    // The walk to the rich ground lands the character on its focus:
    // the second starve reading happens there. The rich ground
    // starves identically two minutes later: the only ripe
    // 1-hop alternative (the home ground) sits in its five minute
    // starve cooldown, so the sweep widens to the 2-hop ring (the
    // far ground) instead of bouncing back - the two nearest
    // grounds can no longer ping-pong the hunter.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 47712, Y: 41500, Z: -3400,
    })
    later := now.Add(2 * time.Minute)
    hunter.emptySince = later.Add(-70 * time.Second)
    hunter.enteredAt = later.Add(-2 * time.Minute)
    loop.cellEvaluate(later)
    require.Equal(t, "test-far", loop.zonePickedID)
    // The cooldown expires: the home ground re-opens for the
    // contest and the next starved sweep may walk back to it.
    expired := now.Add(6 * time.Minute)
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 46112, Y: 46500, Z: -3400,
    })
    hunter.readSelf(loop)
    require.False(t, hunter.starveCoolingDown("test-home", expired))
    hunter.emptySince = expired.Add(-70 * time.Second)
    hunter.enteredAt = expired.Add(-2 * time.Minute)
    loop.cellEvaluate(expired)
    require.Equal(t, "test-home", loop.zonePickedID)
}

func TestCellDeathHeatRegresses(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    loop.cellNoteDeath(now)
    require.GreaterOrEqual(t, hunter.metrics[0].deaths, 1.0)
    // The death regression escapes the deadly neighborhood through
    // the global contest: the proximity discount still prefers the
    // near grounds, the rich neighbor wins.
    require.Equal(t, "test-rich", loop.zonePickedID)
    // The heat decays with time: a half-life later it halves.
    heatNow := hunter.deathHeat(&hunter.metrics[0], now)
    heatLater := hunter.deathHeat(&hunter.metrics[0],
        now.Add(30*time.Minute))
    require.InDelta(t, heatNow*0.5, heatLater, 1e-6)
}

func TestCellTrustedZeroIncomeScoresZero(t *testing.T) {
    _, _, hunter := pickedCellLoop(t)
    // A starved ground (minutes of active time, no adena, no kills)
    // must not promise its window-mass prior forever: the trusted
    // zero stays zero - the 2026-09-13 two-ground livelock.
    metric := &hunter.metrics[hunter.picked]
    metric.activeMin = 6
    metric.adena = 0
    require.InDelta(t, 0.0, hunter.cellValue(
        &hunter.cells[hunter.picked], metric), 1e-9)
}

func TestCellAccumulateAttributesAdena(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    metric := &hunter.metrics[hunter.picked]
    hunter.accumulate(loop, now)
    // A loot gain lands in the inventory: the next accumulation
    // attributes it to the ground (the adena item, type 2 class 4).
    loop.tracker.ApplyItemList([]state.InventoryItem{
        {ObjectID: 999, ItemID: 57, Count: 900, Type2: 4, Change: 1},
    })
    hunter.accumulate(loop, now.Add(2*time.Second))
    require.InDelta(t, 2.0/60.0, metric.visitActiveMin, 0.001)
    require.Equal(t, int64(900), metric.visitAdena)
    // The fold closes the visit into the totals.
    hunter.foldVisit(hunter.picked)
    require.Equal(t, int64(900), metric.adena)
    require.InDelta(t, 2.0/60.0, metric.activeMin, 0.0001)
}

func TestCellViewCarriesTheLiveRecord(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    hunter.viewAt = time.Time{}
    hunter.publishView(loop, now)
    view := loop.tracker.HuntCell()
    require.Equal(t, "test-home", view.ID)
    require.Equal(t, "Home Keltirs", view.Name)
    require.Equal(t, cellLiveStateFarming, view.State)
    require.Equal(t, int32(6), view.SpawnMass)
    require.Equal(t, int32(15), view.RespawnMinSec)
    require.Equal(t, int32(20), view.RespawnMaxSec)
    require.Equal(t, int32(1), view.Occupancy)
    // A pending respawn of the overlay rides along.
    hunter.kills = append(hunter.kills, killRecord{
        cellID: "test-home", x: 46400, y: 41600, at: now,
        respawnAt: now.Add(17 * time.Second),
    })
    hunter.viewAt = time.Time{}
    hunter.publishView(loop, now.Add(time.Second))
    view = loop.tracker.HuntCell()
    require.Equal(t, int32(16), view.NextRespawnSec)
}

func TestCellViewMovingStateOutsideGround(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    // The character walks off the polygon: the live record flips to
    // the moving state - the map answers "which zone is the bot
    // going to" through exactly this marker.
    loop.tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 47712, Y: 41500, Z: -3400,
    })
    hunter.readSelf(loop)
    hunter.viewAt = time.Time{}
    hunter.publishView(loop, now)
    view := loop.tracker.HuntCell()
    require.Equal(t, cellLiveStateMoving, view.State)
}

func TestSetHuntingCellsPublishesMeshAtInstall(t *testing.T) {
    bot := cellTestBot(t)
    loop := NewLoop(&fakeGame{}, bot)
    loop.SetHuntingCells(testCells())
    // The mesh installs with the registry even BEFORE the first
    // pick (the phases that consume every tick ahead of the picker
    // must not hide the map).
    version, payload := bot.HuntMesh()
    require.Equal(t, huntMeshVersion, version)
    require.Contains(t, string(payload), `"id":"test-home"`)
    require.Contains(t, string(payload), `"verts":[`)
}

func TestCellViewPublishesKillMarks(t *testing.T) {
    loop, _, hunter := pickedCellLoop(t)
    now := time.Now()
    hunter.kills = append(hunter.kills, killRecord{
        cellID: "test-home", x: 46400, y: 41600, at: now,
        respawnAt: now.Add(17 * time.Second),
    })
    hunter.viewAt = time.Time{}
    hunter.publishView(loop, now)
    marks := loop.tracker.KillMarks()
    require.Len(t, marks, 1)
    require.Equal(t, int32(46400), marks[0].X)
}

func TestCellMobRespawnLookup(t *testing.T) {
    cells := testCells()
    rmin, rmax, found := cellMobRespawn(cells,
        npcdata.NPCWireTemplateID(20014))

    require.True(t, found)
    require.Equal(t, int32(15), rmin)
    require.Equal(t, int32(20), rmax)
    _, _, found = cellMobRespawn(cells, npcdata.NPCWireTemplateID(9999))
    require.False(t, found)
}

func TestCellWindowHelpers(t *testing.T) {
    cells := testCells()
    // The level 6 window: the home ground holds the full mass, the
    // rich ground holds the level 6 wolves only.
    require.InDelta(t, 6.0, cellWindowMass(cells[0], 6), 1e-9)
    require.InDelta(t, 8.0, cellWindowMass(cells[1], 6), 1e-9)
    require.True(t, cellEligible(cells[0], 6))
    require.True(t, cellEligible(cells[1], 6))
    // The level 20 character outgrew the home ground entirely: the
    // adena floor (level - 8) sits above every mob.
    require.False(t, cellEligible(cells[0], 20))
    // The median of the home mix (4 keltirs of 2, 2 elders of 4)
    // reads 2, the delevel safety holds while the gap stays under
    // the trigger.
    require.Equal(t, int32(2), cellMedianLevel(cells[0]))
    require.True(t, cellDelevelSafe(cells[0], 6))
    require.False(t, cellDelevelSafe(cells[0], 10))
    require.Equal(t, int32(2), cellLevelDistance(cells[0], 6))
}

func TestCellMobPrioritiesBiasWindow(t *testing.T) {
    cells := testCells()
    // A level 6 character: the level 2 keltirs sit in the
    // white-green subrange [2, 5] (priority 3), the level 4 elders
    // too; the level 6 wolves of the rich neighbor read priority 2
    // (the [level-5, level] window proper).
    priorities := cellMobPriorities(cells[0], 6)
    require.Equal(t, int32(3), priorities[npcdata.NPCWireTemplateID(20534)])
    priorities = cellMobPriorities(cells[1], 6)
    require.Equal(t, int32(2), priorities[npcdata.NPCWireTemplateID(20014)])
    // A level 13 character: the home ground falls out of every
    // window (the floor is 5) - no priority at all.
    require.Empty(t, cellMobPriorities(cells[0], 13))
}
