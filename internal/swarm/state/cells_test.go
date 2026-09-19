// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// squareZone builds a counter-clockwise square polygon leash for the
// containment tests.
func squareZone(cx int32, cy int32, half int32) *CellZone {
    return NewCellZone([]ZoneVertex{
        {X: cx - half, Y: cy - half},
        {X: cx + half, Y: cy - half},
        {X: cx + half, Y: cy + half},
        {X: cx - half, Y: cy + half},
    })
}

func TestCellZoneContains(t *testing.T) {
    zone := squareZone(50000, 42000, 1000)

    require.True(t, zone.Contains(50000, 42000))
    require.True(t, zone.Contains(49000, 41500))
    require.True(t, zone.Contains(51000, 42500))
    require.True(t, zone.Contains(49000, 41000))
    // The edge itself belongs to the closed polygon.
    require.True(t, zone.Contains(51000, 42000))

    require.False(t, zone.Contains(49000, 40999))
    require.False(t, zone.Contains(51001, 42000))
    require.False(t, zone.Contains(45000, 42000))
}

func TestCellZoneContainsLargeCoordinates(t *testing.T) {
    // The cross products of the far map regions overflow int32: the
    // containment must run on int64 intermediates.
    zone := NewCellZone([]ZoneVertex{
        {X: 100000, Y: 100000},
        {X: 160000, Y: 100000},
        {X: 160000, Y: 160000},
        {X: 100000, Y: 160000},
    })

    require.True(t, zone.Contains(130000, 130000))
    require.False(t, zone.Contains(30000, 30000))
    require.False(t, zone.Contains(230000, 230000))
}

func TestCellZoneNilAndDegenerate(t *testing.T) {
    var zone *CellZone
    require.True(t, zone.Contains(0, 0))

    empty := NewCellZone(nil)
    require.True(t, empty.Contains(0, 0))

    line := NewCellZone([]ZoneVertex{{X: 0, Y: 0}, {X: 100, Y: 100}})
    require.True(t, line.Contains(5000, 5000))
}

func TestCellZoneConcaveRejected(t *testing.T) {
    // A concave vertex order (a dart) reports the inside of the dart
    // body as outside: the leash is defined for CONVEX counter-
    // clockwise polygons, the concave input is a generator bug the
    // registry tests pin there.
    dart := NewCellZone([]ZoneVertex{
        {X: 0, Y: 0},
        {X: 2000, Y: 2000},
        {X: 0, Y: 4000},
        {X: 1000, Y: 2000},
    })
    require.False(t, dart.Contains(100, 2000))
}

func TestCellZoneFromPairs(t *testing.T) {
    zone := CellZoneFromPairs([]int32{0, 0, 100, 0, 100, 100, 0, 100})
    require.NotNil(t, zone)
    require.Len(t, zone.Vertices, 4)
    require.True(t, zone.Contains(50, 50))
    require.False(t, zone.Contains(150, 50))

    require.Nil(t, CellZoneFromPairs(nil))
    require.Nil(t, CellZoneFromPairs([]int32{1, 2, 3}))
    // An odd pair count is malformed: no leash at all.
    require.Nil(t, CellZoneFromPairs([]int32{0, 0, 100, 0, 100}))
}

func TestZoneAreaNilSemantics(t *testing.T) {
    // The legacy nil *Zone handed into the interface area keeps its
    // "no limit" meaning through areaNil, and the plain nil
    // interface collapses to the same.
    var square *Zone
    require.True(t, areaNil(nil))
    require.True(t, areaNil(square))
    require.False(t, areaNil(&Zone{CX: 1, CY: 1, Half: 1}))
    require.False(t, areaNil(NewCellZone([]ZoneVertex{
        {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1},
    })))

    // A nil area through the pickable reading: no leash means every
    // attackable npc counts (the legacy NearestAttackable(nil)
    // behavior of the tests).
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 0, 0, 0, 50, 30)
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 7, X: 100, Y: 0, Name: "Gremlin", Attackable: true,
    })
    target, ok := bot.NearestAttackable(1500, nil)
    require.True(t, ok)
    require.Equal(t, int32(7), target.ObjectID)
}
