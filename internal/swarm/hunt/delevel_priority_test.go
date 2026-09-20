// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The delevel priority tests (the owner report of the delevel
// acceptance round): the outleveled character on the spawn spot must
// start the deleveling at once - the mobs of the spot never pull the
// engage into a farming detour first, the shopping plans never send
// the character to the shops instead of the guards, and a spot whose
// living mobs all sit inside the respawn window still triggers on the
// static median of the held ground.

// delevelCell builds the test stand-in of the Green Dryad ground: one
// low median cell (a level 8 and a level 7 mob) whose gap to a level
// 15 character holds the delevel trigger exactly.
func delevelCell(fx, fy int32) Cell {
    cell := quadCell("delevel-home", "Dryad Ground", fx, fy, 800,
        []int32{}, []CellMob{
            {TemplateID: 20336, Name: "Green Dryad", Level: 8, Count: 1,
                RespawnMin: 15, RespawnMax: 20},
            {TemplateID: 20470, Name: "Kaboo Orc Grunt", Level: 7,
                Count: 1, RespawnMin: 15, RespawnMax: 20},
        })
    cell.Mass = 2

    return cell
}

// newDelevelCellLoop prepares the spawn spot scene: a level 15
// character standing on the focus of a single low median cell in the
// cell mode (no legacy zone), the navigator armed. The weapons in the
// bag keep the weapon run out of the scene - only the delevel and the
// farming compete here.
func newDelevelCellLoop(t *testing.T) (*Loop, *fakeGame, *state.Bot) {
    t.Helper()
    bot := newTestBot()
    setZoneTestLevel(bot, 15)
    moveSelfTo(bot, 46112, 41500, -3500)
    giveTestWeapon(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetHuntingCells([]Cell{delevelCell(46112, 41500)})
    loop.SetNavigator(&fakeNavigator{found: true})
    loop.lastHit = time.Now().Add(-time.Minute)
    t.Cleanup(func() {
        if loop.cell != nil && loop.cell.leave != nil {
            loop.cell.leave()
        }
    })

    return loop, game, bot
}

// giveTestWeapon puts a plain sword in the bag: the weapon run of the
// shopping strategy stays out of the scene, only the delevel and the
// farming compete here. The auto equipment wears it on the tick.
func giveTestWeapon(bot *state.Bot) {
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 601, ItemID: 1, Count: 1},
    })
}

// fillInventoryForTripTest fills the bag past the trip slot trigger
// (half of the 80 slot limit): the strongest town trip reason of the
// loop, the one the shopping detour report named.
func fillInventoryForTripTest(bot *state.Bot) {
    items := make([]state.InventoryItem, 0, 48)
    for i := range 45 {
        items = append(items, state.InventoryItem{
            ObjectID: int32(700 + i), ItemID: 1060, Count: 1,
        })
    }
    bot.ApplyItemList(items)
}

// spawnSpotMobs puts living pickable mobs on the cell ground: the
// random circumstance of the report - the spot holds mobs the engage
// would love to farm. The wire template ids resolve to the real
// levels (8 and 7) the live median reads.
func spawnSpotMobs(bot *state.Bot) {
    points := [][2]int32{{46000, 41400}, {46200, 41600}}
    templates := [2]int32{1000336, 1000470}
    for i, p := range points {
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID: int32(300 + i), TemplateID: templates[i],
            Attackable: true, X: p[0], Y: p[1],
            Name: "Dryad", Z: -3500,
        })
    }
}

// delevelGuardWalkTarget is the first walk segment of the guard walk
// from the test spot (46112 41500): the nearest archer guard of the
// scene is Kendell (47595 51569, the eastern post), the segment caps
// at the server move request limit along the straight line to it.
func delevelGuardWalkTarget() [3]int32 {
    return segmentWalkTarget([3]int32{46112, 41500, -3500},
        pathfind.Vec3{
            X: float64(kendellPos[0]), Y: float64(kendellPos[1]),
            Z: float64(kendellPos[2]),
        })
}

// TestDelevelOutranksTheEngageOnTheSpawnSpot pins the engage
// preemption: the first tick of an outleveled character on its spot
// starts the deleveling - the living mobs of the ground never win one
// attack request, the walk to the guard replaces the farming.
func TestDelevelOutranksTheEngageOnTheSpawnSpot(t *testing.T) {
    loop, game, bot := newDelevelCellLoop(t)
    spawnSpotMobs(bot)

    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase,
        "the outleveled spot starts the deleveling on the first tick")
    require.Empty(t, game.forces,
        "the mobs of the spot never receive an attack request")
    require.Equal(t, int32(13), loop.delevelTarget,
        "the target is the median 8 plus the full drop gap 5")
    require.Equal(t, []([3]int32){delevelGuardWalkTarget()}, game.walks,
        "the first walk is the guard segment")

    // The trigger holds across the following ticks: the phase never
    // falls back to the engage while the walk runs.
    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase)
    require.Empty(t, game.forces)
}

// TestDelevelTriggersOnStaticMedianWhileSpotEmpty pins the respawn
// window answer: the spot the character just sees empty (every living
// mob inside the respawn window) still triggers the deleveling on the
// static median of the held ground - the designed mix of the ground,
// not the flickering live read, decides when the living read is
// empty. The old behavior farmed the respawn waves one kill at a time
// and only de-leveled when a tick happened to land on a living mob.
func TestDelevelTriggersOnStaticMedianWhileSpotEmpty(t *testing.T) {
    loop, game, _ := newDelevelCellLoop(t)

    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase,
        "the empty live read falls back to the static median")
    require.Empty(t, game.forces)
    require.Equal(t, []([3]int32){delevelGuardWalkTarget()}, game.walks,
        "the guard walk replaces the empty spot patrol")
    require.Equal(t, int32(13), loop.delevelTarget,
        "the static median 8 computes the target, the Lucky clamp "+
            "never fires")
}

// TestDelevelOutranksTheTownTrip pins the shopping preemption: a full
// inventory (the strongest trip trigger) never sends the outleveled
// character to the shops - the deleveling starts instead, the sale
// waits for the walk home after the deaths.
func TestDelevelOutranksTheTownTrip(t *testing.T) {
    loop, game, bot := newDelevelCellLoop(t)
    spawnSpotMobs(bot)
    fillInventoryForTripTest(bot)

    loop.tick()
    require.Equal(t, phaseDelevel, loop.phase,
        "the deleveling outranks the town trip")
    require.Equal(t, []([3]int32){delevelGuardWalkTarget()}, game.walks,
        "the only walk is the guard segment, the shop walk never ran")
    require.Empty(t, game.sells)
    require.Nil(t, loop.tripStops,
        "no trip stops were planned behind the guard walk")
}
