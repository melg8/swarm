// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRotationPackets(t *testing.T) {
	t.Run("begin rotation", func(t *testing.T) {
		data := []byte{0x77}
		data = putInt32(data, 268473919)
		data = putInt32(data, 12345)
		data = putInt32(data, 1)   // side
		data = putInt32(data, 500) // speed

		p := NewBeginRotationPacket()
		err := ParseBeginRotationPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, int32(12345), p.Heading)
	})

	t.Run("stop rotation", func(t *testing.T) {
		data := []byte{0x78}
		data = putInt32(data, 268473919)
		data = putInt32(data, 45678)
		data = putInt32(data, 500) // speed
		data = append(data, 0)     // unknown byte

		p := NewStopRotationPacket()
		err := ParseStopRotationPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, int32(45678), p.Heading)
	})
}

func TestParseMoveTypeAndTeleportPackets(t *testing.T) {
	t.Run("change move type run", func(t *testing.T) {
		data := []byte{0x3E}
		data = putInt32(data, 42)
		data = putInt32(data, 1)

		p := NewChangeMoveTypePacket()
		err := ParseChangeMoveTypePacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(42), p.ObjectID)
		require.True(t, p.Running)
	})

	t.Run("change move type walk", func(t *testing.T) {
		data := []byte{0x3E}
		data = putInt32(data, 42)
		data = putInt32(data, 0)

		p := NewChangeMoveTypePacket()
		err := ParseChangeMoveTypePacket(p, data)
		require.NoError(t, err)
		require.False(t, p.Running)
	})

	t.Run("teleport to location", func(t *testing.T) {
		data := []byte{0x38}
		data = putInt32(data, 7)
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 1) // fade
		data = putInt32(data, 9000)

		p := NewTeleportToLocationPacket()
		err := ParseTeleportToLocationPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(7), p.ObjectID)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50000), p.Y)
		require.Equal(t, int32(-3500), p.Z)
		require.Equal(t, int32(9000), p.Heading)
	})
}

func TestParseTargetPackets(t *testing.T) {
	t.Run("my target selected", func(t *testing.T) {
		data := []byte{0xBF}
		data = putInt32(data, 1234)
		data = putInt16(data, 3) // color

		p := NewMyTargetSelectedPacket()
		err := ParseMyTargetSelectedPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(1234), p.ObjectID)
	})

	t.Run("target selected", func(t *testing.T) {
		data := []byte{0x39}
		data = putInt32(data, 1001)
		data = putInt32(data, 2002)
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 0)

		p := NewTargetSelectedPacket()
		err := ParseTargetSelectedPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(1001), p.ObjectID)
		require.Equal(t, int32(2002), p.TargetID)
		require.Equal(t, int32(45000), p.X)
	})

	t.Run("target unselected", func(t *testing.T) {
		data := []byte{0x3A}
		data = putInt32(data, 1001)
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 0)

		p := NewTargetUnselectedPacket()
		err := ParseTargetUnselectedPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(1001), p.ObjectID)
	})
}

func TestParseItemWorldPackets(t *testing.T) {
	t.Run("spawn item", func(t *testing.T) {
		data := []byte{0x15}
		data = putInt32(data, 9001)
		data = putInt32(data, 1000057)
		data = putInt32(data, 45100)
		data = putInt32(data, 50100)
		data = putInt32(data, -3500)
		data = putInt32(data, 1) // stackable
		data = putInt32(data, 57)

		p := NewSpawnItemPacket()
		err := ParseSpawnItemPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(9001), p.ObjectID)
		require.Equal(t, int32(1000057), p.TemplateID)
		require.True(t, p.Stackable)
		require.Equal(t, int32(57), p.Count)
		require.Equal(t, int32(45100), p.X)
	})

	t.Run("get item", func(t *testing.T) {
		data := []byte{0x17}
		data = putInt32(data, 77) // player
		data = putInt32(data, 9001)
		data = putInt32(data, 45100)
		data = putInt32(data, 50100)
		data = putInt32(data, -3500)

		p := NewGetItemPacket()
		err := ParseGetItemPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(77), p.PlayerID)
		require.Equal(t, int32(9001), p.ObjectID)
		require.Equal(t, int32(45100), p.X)
	})
}

// inventoryItemEntry appends one inventory item entry to the buffer.
func inventoryItemEntry(
	dst []byte, objectID int32, itemID int32, count int32,
	type2 int16, equipped int16,
) []byte {
	dst = putInt16(dst, 0) // type1
	dst = putInt32(dst, objectID)
	dst = putInt32(dst, itemID)
	dst = putInt32(dst, count)
	dst = putInt16(dst, type2)
	dst = putInt16(dst, 0) // custom type1
	dst = putInt16(dst, equipped)
	dst = putInt32(dst, 0) // body part
	dst = putInt16(dst, 0) // enchant
	dst = putInt16(dst, 0) // custom type2

	return dst
}

func TestParseInventoryPackets(t *testing.T) {
	itemEntry := inventoryItemEntry

	t.Run("item list", func(t *testing.T) {
		data := []byte{0x27}
		data = putInt16(data, 0) // show window
		data = putInt16(data, 2)
		data = itemEntry(data, 1, 57, 500, 4, 0)
		data = itemEntry(data, 2, 10, 1, 0, 1)

		p := NewItemListPacket()
		err := ParseItemListPacket(p, data)
		require.NoError(t, err)
		require.Len(t, p.Items, 2)
		require.Equal(t, int32(57), p.Items[0].ItemID)
		require.Equal(t, int16(4), p.Items[0].Type2)
		require.True(t, p.Items[1].Equipped)
	})

	t.Run("item list reuses the slice", func(t *testing.T) {
		p := NewItemListPacket()
		first := []byte{0x27}
		first = putInt16(first, 0)
		first = putInt16(first, 1)
		first = itemEntry(first, 1, 57, 500, 4, 0)
		require.NoError(t, ParseItemListPacket(p, first))
		require.Len(t, p.Items, 1)

		second := []byte{0x27}
		second = putInt16(second, 0)
		second = putInt16(second, 2)
		second = itemEntry(second, 2, 10, 1, 0, 1)
		second = itemEntry(second, 3, 11, 1, 0, 0)
		require.NoError(t, ParseItemListPacket(p, second))
		require.Len(t, p.Items, 2)
		require.Equal(t, int32(2), p.Items[0].ObjectID)
	})
}

func TestParseInventoryUpdatePacket(t *testing.T) {
	itemEntry := inventoryItemEntry

	t.Run("inventory update", func(t *testing.T) {
		data := []byte{0x37}
		data = putInt16(data, 2)
		data = putInt16(data, 1) // add
		data = itemEntry(data, 5, 57, 100, 4, 0)
		data = putInt16(data, 3) // remove
		data = itemEntry(data, 6, 10, 1, 0, 0)

		p := NewInventoryUpdatePacket()
		err := ParseInventoryUpdatePacket(p, data)
		require.NoError(t, err)
		require.Len(t, p.Items, 2)
		require.Equal(t, int16(1), p.Items[0].Change)
		require.Equal(t, int16(3), p.Items[1].Change)
		require.Equal(t, int32(57), p.Items[0].ItemID)
	})

	t.Run("implausible counts fail", func(t *testing.T) {
		data := []byte{0x27}
		data = putInt16(data, 0)
		data = putInt16(data, 500)

		p := NewItemListPacket()
		err := ParseItemListPacket(p, data)
		require.Error(t, err)
	})
}
