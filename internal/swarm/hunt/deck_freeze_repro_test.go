// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The reproduction of the 2026-09-14 07:53 state dump (build 140aa42,
// bot unittest2, phase engage, uptime 1m57s): the level 14 character
// stood at x 42712 y 49128 z -2992 (the Elven Village terrace deck)
// while its held hunting cell "Kaboo Orc Fighter SW-7" sat 7000 units
// west at center 36000 46765 on the field deck (z ~-3576). The dump
// listed pickable mobs in the knownlist (Kaboo Orc Grunt level 7 at
// 3157 units, Green Dryad level 8 at 3799, Kaboo Orc Archer level 8
// at 3830, Spore Fungus level 9 at 4108) - all on the field deck
// below the terrace. The hunt log carried a single "level 14:
// holding the cell Kaboo Orc Fighter SW-7" line and nothing else
// for the whole uptime: no "outside the hunting zone, pathfinding
// back", no "engaging", no "far target walk", no "no pickable
// target". The bot neither moved nor engaged.
//
// Root cause: the engage phase gates the zone return on
// cellEnemiesVisible, and cellEnemiesVisible answered true (the
// knownlist held pickable mobs on the field deck). The pick flow
// that followed tried to walk straight toward the nearest visible
// mob (walkToFarTarget -> game.WalkTo), but the straight line from
// the village terrace to the field deck crosses the village railing
// and the deck edge - the server's geodata correction collapses the
// click target onto the walker cell, the move is silently canceled
// and the character never moves. The pathfound zone return (the
// only path that walks the character down the terrace ramp) never
// armed because the visible-mob reading held its gate shut.
//
// The fix: cellEnemiesVisible answers true only for the mobs the
// character can actually reach by a direct walk - the ones on the
// SAME deck (a small z gap). A character on a different deck than
// every visible mob reads empty (the visible mobs are on a deck the
// direct walk cannot reach), so the engage falls through to
// returnToZone and the pathfinder walks the character down the
// terrace ramp to the field deck.

const (
    // reproDeckVillageX/Y/Z is the reported stuck position (the dump
    // of 2026-09-14 07:53:33, the character unittest2 on the Elven
    // Village terrace deck).
    reproDeckVillageX = int32(42712)
    reproDeckVillageY = int32(49128)
    reproDeckVillageZ = int32(-2992)
    // reproDeckFieldZ is the z of the field deck the hunting cell
    // and the visible mobs sit on (the Elven lands field deck below
    // the village terrace).
    reproDeckFieldZ = int32(-3576)
    // reproDeckCellX/Y is the held cell center (the Kaboo Orc Fighter
    // SW-7 hexagon focus of the dump).
    reproDeckCellX = int32(36000)
    reproDeckCellY = int32(46765)
    // reproDeckCellPatrolHalf is the patrol square half of the held
    // cell of the dump (the hexagon circumradius 1000 -> the inscribed
    // square half 633).
    reproDeckCellPatrolHalf = int32(633)
    // reproDeckMobObject is the object id of the nearest visible mob
    // of the dump (a Kaboo Orc Grunt level 7 at 3157 units, on the
    // field deck).
    reproDeckMobObject = int32(268439337)
    // reproDeckMobX/Y is the position of the dump's nearest mob.
    reproDeckMobX = int32(39557)
    reproDeckMobY = int32(49023)
)

// reproDeckScene builds the dump standoff: the level 14 character
// unittest2 on the village terrace deck, the held cell "Kaboo Orc
// Fighter SW-7" 7000 units west on the field deck, and the nearest
// visible mob (a Kaboo Orc Grunt level 7) on the field deck at 3157
// units. The loop is in the engage phase with no target. The cell
// registry carries a single hexagon matching the dump's held cell
// geometry (the focus at the dump coordinates, the same patrol half,
// the level 8-10 mob window of the dump).
func reproDeckScene(t *testing.T) (*Loop, *fakeGame, *state.Bot) {
    t.Helper()
    bot := state.NewBot("acc1")
    bot.SetCharacter("unittest2", 268451289, 18,
        reproDeckVillageX, reproDeckVillageY, reproDeckVillageZ, 339, 137)
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest2", Level: 14, Race: 1, ClassID: 18,
        X: reproDeckVillageX, Y: reproDeckVillageY, Z: reproDeckVillageZ,
        MaxHP: 339, CurHP: 339, MaxMP: 137, CurMP: 137, Exp: 238925,
    })
    bot.SetOnline("unittest2")
    // The nearest visible mob of the dump: a Kaboo Orc Grunt level 7
    // on the field deck at 3157 units. The cell window floor of a
    // level 14 character is 14-8 = 6, so the level 7 mob passes the
    // level filter - it would be pickable if the character stood on
    // the same deck. The wire template id carries the npcdata offset
    // (1000000 + 20470 = 1000470) so NPCLevel resolves to 7.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: reproDeckMobObject, TemplateID: 1000470,
        Attackable: true,
        X:          reproDeckMobX, Y: reproDeckMobY, Z: reproDeckFieldZ,
        Name: "Kaboo Orc Grunt",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.SetAutonomy(true)
    // The cell registry of the dump: one hexagon at the dump cell
    // focus with the dump cell mobs. The character stands outside it
    // (the village terrace sits 7000 units east of the cell focus).
    loop.SetHuntingCells([]Cell{
        {
            ID: "elven-hex-113", Name: "Kaboo Orc Fighter SW-7",
            Region: regionElven, MinLevel: 8, MaxLevel: 10,
            FocusX: reproDeckCellX, FocusY: reproDeckCellY,
            PatrolHalf: reproDeckCellPatrolHalf,
            RespawnMin: 15, RespawnMax: 20, Mass: 5.0,
            Mobs: []CellMob{
                {TemplateID: 20471, Name: "Kaboo Orc Fighter",
                    Level: 10, Count: 3, RespawnMin: 15, RespawnMax: 20},
                {TemplateID: 20509, Name: "Spore Fungus",
                    Level: 9, Count: 1, RespawnMin: 15, RespawnMax: 20},
                {TemplateID: 20469, Name: "Kaboo Orc Archer",
                    Level: 8, Count: 1, RespawnMin: 15, RespawnMax: 20},
            },
            Vertices: []CellVertex{
                {X: 37000, Y: 46765},
                {X: 36500, Y: 47631},
                {X: 35500, Y: 47631},
                {X: 35000, Y: 46765},
                {X: 35500, Y: 45899},
                {X: 36500, Y: 45899},
            },
            Neighbors: []int32{},
        },
    })
    // The first tick picks the cell (the only one of the registry).
    loop.tick()
    require.Equal(t, "elven-hex-113", loop.zonePickedID,
        "the cell hunter picked the SW-7 cell of the dump")
    loop.phase = phaseEngage

    return loop, game, bot
}

// TestReproDeckVillageReturnToZoneOnDifferentDeck pins the fix: the
// cell mode treats the visible mobs on a different deck than the
// character as unreachable by a direct walk, so the engage falls
// through to returnToZone and the pathfinder plans the ramp walk
// down to the field deck. The dump signature inverts: instead of a
// single direct walk toward the mob (on the village z), the loop
// plans the pathfound zone return and walks the first waypoint.
func TestReproDeckVillageReturnToZoneOnDifferentDeck(t *testing.T) {
    loop, game, _ := reproDeckScene(t)
    nav := &fakeNavigator{found: true, route: []pathfind.Vec3{
        {X: 41000, Y: 49000, Z: -3000},
        {X: 38000, Y: 48000, Z: -3576},
        {X: 36000, Y: 46765, Z: -3576},
    }}
    loop.SetNavigator(nav)
    // The patience window of walkToFarTarget holds the first walk
    // back, so it never interferes with the zone return path.
    loop.noTargetSince = time.Now().Add(-2 * noTargetPatience)
    loop.lastHit = time.Time{}

    loop.tick()

    // The zone return armed: the pathfinder planned the ramp walk
    // down to the field deck. The engage phase moved to the town
    // return phase (the zone return follower).
    require.True(t, loop.zoneReturn,
        "the zone return armed - the field deck mobs read unreachable")
    require.Equal(t, phaseTownReturn, loop.phase,
        "the loop entered the pathfound zone return phase")
    require.Positive(t, nav.calls,
        "the pathfinder planned the ramp walk down to the field deck")
    require.NotEmpty(t, game.walks,
        "the first waypoint of the pathfound return walked")
    // The first walk of the pathfound return is the first waypoint
    // of the planned route (capped at maxMoveDistance = 1000 units along
    // the line to the first waypoint). The route heads west toward
    // the cell center on the field deck, NOT directly toward the mob.
    firstWalk := game.walks[0]
    dx := float64(firstWalk[0] - reproDeckVillageX)
    dy := float64(firstWalk[1] - reproDeckVillageY)
    require.InDelta(t, -1.0, dx/math.Hypot(dx, dy), 0.1,
        "the pathfound return heads west toward the cell center")
    // The walk stays on the village terrace z of the character (the
    // first segment walks the terrace toward the ramp, not the field
    // deck directly - the segment interpolation keeps the starting z).
    require.InDelta(t, reproDeckVillageZ, firstWalk[2], 1,
        "the first segment keeps the village terrace z")
}

// TestReproDeckVillageNoDirectWalkTowardCrossDeckMob pins the freeze
// signature of the dump inverted by the fix: the bot stands on the
// village terrace deck with pickable mobs visible on the field deck
// below. Before the fix the engage fired a direct walk toward the
// nearest mob (on the village z, the straight line the server
// cancels at the railing) and the pathfound zone return never
// armed. After the fix the direct walk toward the cross-deck mob
// never fires - the zone return owns the movement instead.
func TestReproDeckVillageNoDirectWalkTowardCrossDeckMob(t *testing.T) {
    loop, game, _ := reproDeckScene(t)
    nav := &fakeNavigator{found: true, route: []pathfind.Vec3{
        {X: 41000, Y: 49000, Z: -3000},
        {X: 38000, Y: 48000, Z: -3576},
        {X: 36000, Y: 46765, Z: -3576},
    }}
    loop.SetNavigator(nav)
    loop.noTargetSince = time.Now().Add(-2 * noTargetPatience)
    loop.lastHit = time.Time{}

    loop.tick()

    // No direct walk toward the cross-deck mob fired: every walk
    // request of the tick belongs to the pathfound zone return (the
    // first waypoint of the planned route, on the village terrace
    // heading west toward the ramp). A direct walk toward the mob
    // would head along the line to (39557, 49023) - the walks of
    // the zone return head toward the cell center (36000, 46765)
    // instead, and the first waypoint (41000, 49000) sits on the
    // same heading line.
    for _, w := range game.walks {
        dx := float64(w[0] - reproDeckVillageX)
        // The direct walk toward the mob would head roughly along
        // (-1, 0) (the mob sits 3157 units west, 105 south). The
        // zone return heads along the planned route - the first
        // waypoint (41000, 49000) sits (-1712, -128) from the
        // character, a heading of (-0.997, -0.075) - close to the
        // direct line but the segment is the pathfound one, not the
        // direct mob walk. The test pins the absence of the direct
        // mob walk by asserting the walk targets the waypoint, not
        // the mob: the waypoint sits ~1700 units west, the mob
        // ~3157 units west, so the walk distance stays under 2000.
        require.Less(t, math.Abs(dx), 2000.0,
            "the walk stays on the pathfound route (the first "+
                "waypoint sits under 2000 units west), not the "+
                "direct mob walk (3157 units west)")
    }
}

// TestReproDeckVillageEngagesSameDeckMob pins the regression guard:
// the deck gate of cellEnemiesVisible must NOT suppress the free-roam
// engage when the visible mob sits on the SAME deck as the character.
// The dump's held cell sits on the field deck; a character standing
// on the field deck next to a visible mob engages it directly - the
// free-roam hunt keeps its design.
func TestReproDeckVillageEngagesSameDeckMob(t *testing.T) {
    loop, game, bot := reproDeckScene(t)
    // Move the character onto the field deck next to the nearest mob
    // (same deck, same z - the free-roam engage path).
    bot.SetCharacter("unittest2", 268451289, 18,
        reproDeckMobX-200, reproDeckMobY, reproDeckFieldZ, 339, 137)
    bot.ApplyUserInfo(state.UserInfo{
        Name: "unittest2", Level: 14, Race: 1, ClassID: 18,
        X: reproDeckMobX - 200, Y: reproDeckMobY, Z: reproDeckFieldZ,
        MaxHP: 339, CurHP: 339, MaxMP: 137, CurMP: 137, Exp: 238925,
    })
    loop.SetNavigator(&fakeNavigator{found: true})
    loop.noTargetSince = time.Now().Add(-2 * noTargetPatience)
    loop.lastHit = time.Time{}

    loop.tick()

    // The engage picked the mob on the same deck (the free-roam
    // design): a forced attack request fired, no zone return armed.
    require.NotEmpty(t, game.forces,
        "the engage attacked the same-deck mob directly")
    require.False(t, loop.zoneReturn,
        "the zone return never armed for the same-deck mob")
    require.Equal(t, reproDeckMobObject, loop.target,
        "the target is the same-deck mob of the knownlist")
}
