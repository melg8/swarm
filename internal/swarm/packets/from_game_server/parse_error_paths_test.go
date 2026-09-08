// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// requireTruncatedPrefixesError asserts that parsing every strict prefix
// of a valid packet fails: a truncated packet must return an error and
// must never panic, whatever the truncation point.
func requireTruncatedPrefixesError(
	t *testing.T, data []byte, parse func(data []byte) error,
) {
	t.Helper()

	for size := range data {
		require.Error(t, parse(data[:size]), "prefix of len %d", size)
	}
}

func TestTruncatedCompactPacketPrefixesError(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		parse func(data []byte) error
	}{
		{
			"my target selected", buildMyTargetSelectedData(),
			func(data []byte) error {
				return ParseMyTargetSelectedPacket(
					NewMyTargetSelectedPacket(), data)
			},
		},
		{
			"target selected", buildTargetSelectedData(),
			func(data []byte) error {
				return ParseTargetSelectedPacket(
					NewTargetSelectedPacket(), data)
			},
		},
		{
			"target unselected", buildTargetUnselectedData(),
			func(data []byte) error {
				return ParseTargetUnselectedPacket(
					NewTargetUnselectedPacket(), data)
			},
		},
		{
			"begin rotation", buildBeginRotationData(),
			func(data []byte) error {
				return ParseBeginRotationPacket(
					NewBeginRotationPacket(), data)
			},
		},
		{
			"stop rotation", buildStopRotationData(),
			func(data []byte) error {
				return ParseStopRotationPacket(
					NewStopRotationPacket(), data)
			},
		},
		{
			"change move type", buildChangeMoveTypeData(),
			func(data []byte) error {
				return ParseChangeMoveTypePacket(
					NewChangeMoveTypePacket(), data)
			},
		},
		{
			"teleport to location", buildTeleportToLocationData(),
			func(data []byte) error {
				return ParseTeleportToLocationPacket(
					NewTeleportToLocationPacket(), data)
			},
		},
		{
			"social action", buildSocialActionData(),
			func(data []byte) error {
				return ParseSocialActionPacket(
					NewSocialActionPacket(), data)
			},
		},
		{
			"auto attack start", buildAutoAttackStartData(),
			func(data []byte) error {
				return ParseAutoAttackStartPacket(
					NewAutoAttackStartPacket(), data)
			},
		},
		{
			"auto attack stop", buildAutoAttackStopData(),
			func(data []byte) error {
				return ParseAutoAttackStopPacket(
					NewAutoAttackStopPacket(), data)
			},
		},
		{
			"delete object", buildDeleteObjectData(),
			func(data []byte) error {
				return ParseDeleteObjectPacket(
					NewDeleteObjectPacket(), data)
			},
		},
		{
			"move to location", buildMoveToLocationData(),
			func(data []byte) error {
				return ParseMoveToLocationPacket(
					NewMoveToLocationPacket(), data)
			},
		},
		{
			"move to pawn", buildMoveToPawnData(),
			func(data []byte) error {
				return ParseMoveToPawnPacket(
					NewMoveToPawnPacket(), data)
			},
		},
		{
			"stop move", buildStopMoveData(),
			func(data []byte) error {
				return ParseStopMovePacket(NewStopMovePacket(), data)
			},
		},
		{
			"validate location", buildValidateLocationData(),
			func(data []byte) error {
				return ParseValidateLocationPacket(
					NewValidateLocationPacket(), data)
			},
		},
		{
			"drop item", buildDropItemData(),
			func(data []byte) error {
				return ParseDropItemPacket(NewDropItemPacket(), data)
			},
		},
		{
			"spawn item", buildSpawnItemData(),
			func(data []byte) error {
				return ParseSpawnItemPacket(NewSpawnItemPacket(), data)
			},
		},
		{
			"get item", buildGetItemData(),
			func(data []byte) error {
				return ParseGetItemPacket(NewGetItemPacket(), data)
			},
		},
		{
			"status update", buildStatusUpdateData(),
			func(data []byte) error {
				return ParseStatusUpdatePacket(
					NewStatusUpdatePacket(), data)
			},
		},
		{
			"net ping", buildNetPingData(),
			func(data []byte) error {
				return ParseNetPingPacket(NewNetPingPacket(), data)
			},
		},
		{
			"attack", buildAttackData(),
			func(data []byte) error {
				return ParseAttackPacket(NewAttackPacket(), data)
			},
		},
		{
			"char selected", buildCharSelectedData(),
			func(data []byte) error {
				return ParseCharSelectedPacket(
					NewCharSelectedPacket(), data)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.parse(tc.data))
			requireTruncatedPrefixesError(t, tc.data, tc.parse)
		})
	}
}

func buildMyTargetSelectedData() []byte {
	data := []byte{myTargetSelectedPacketID}
	data = putInt32(data, 333333)
	data = append(data, 1, 0) // target color

	return data
}

func buildTargetSelectedData() []byte {
	data := []byte{targetSelectedPacketID}
	for _, value := range []int32{333333, 444444, 46000, 51000, -3400, 0} {
		data = putInt32(data, value)
	}

	return data
}

func buildTargetUnselectedData() []byte {
	data := []byte{targetUnselectedPacketID}
	for _, value := range []int32{333333, 46000, 51000, -3400, 0} {
		data = putInt32(data, value)
	}

	return data
}

func buildBeginRotationData() []byte {
	data := []byte{beginRotationPacketID}
	for _, value := range []int32{333333, 23456, 1, 400} {
		data = putInt32(data, value)
	}

	return data
}

func buildStopRotationData() []byte {
	data := []byte{stopRotationPacketID}
	for _, value := range []int32{333333, 34567, 400} {
		data = putInt32(data, value)
	}
	data = append(data, 0)

	return data
}

func buildChangeMoveTypeData() []byte {
	data := []byte{changeMoveTypePacketID}
	for _, value := range []int32{55, 1} {
		data = putInt32(data, value)
	}

	return data
}

func buildTeleportToLocationData() []byte {
	data := []byte{teleportToLocationPacketID}
	for _, value := range []int32{66, 46000, 51000, -3400, 1, 9000} {
		data = putInt32(data, value)
	}

	return data
}

func buildSocialActionData() []byte {
	data := []byte{socialActionPacketID}
	for _, value := range []int32{77, 15} {
		data = putInt32(data, value)
	}

	return data
}

func buildAutoAttackStartData() []byte {
	data := []byte{autoAttackStartPacketID}
	data = putInt32(data, 88)

	return data
}

func buildAutoAttackStopData() []byte {
	data := []byte{autoAttackStopPacketID}
	data = putInt32(data, 88)

	return data
}

func buildDeleteObjectData() []byte {
	data := []byte{deleteObjectPacketID}
	data = putInt32(data, 99)

	return data
}

func buildMoveToLocationData() []byte {
	data := []byte{moveToLocationPacketID}
	for _, value := range []int32{11, 220, 330, 440, 120, 130, 140} {
		data = putInt32(data, value)
	}

	return data
}

func buildMoveToPawnData() []byte {
	data := []byte{moveToPawnPacketID}
	for _, value := range []int32{111111, 222222, 50, 46000, 51000, -3400, 46200, 51200, -3300} {
		data = putInt32(data, value)
	}

	return data
}

func buildStopMoveData() []byte {
	data := []byte{stopMovePacketID}
	for _, value := range []int32{11, 220, 330, 440, 8192} {
		data = putInt32(data, value)
	}

	return data
}

func buildValidateLocationData() []byte {
	data := []byte{validateLocationPacketID}
	for _, value := range []int32{222222, 46000, 51000, -3400, 49152} {
		data = putInt32(data, value)
	}

	return data
}

func buildDropItemData() []byte {
	data := []byte{dropItemPacketID}
	// Dropper, object, template, x, y, z, stackable and count.
	for _, value := range []int32{101, 202, 5720, 46000, 51000, -3400, 1, 24} {
		data = putInt32(data, value)
	}

	return data
}

func buildSpawnItemData() []byte {
	data := []byte{spawnItemPacketID}
	for _, value := range []int32{202, 5720, 46000, 51000, -3400, 1, 24} {
		data = putInt32(data, value)
	}

	return data
}

func buildGetItemData() []byte {
	data := []byte{getItemPacketID}
	for _, value := range []int32{101, 202, 46000, 51000, -3400} {
		data = putInt32(data, value)
	}

	return data
}

func buildStatusUpdateData() []byte {
	data := []byte{statusUpdatePacketID}
	for _, value := range []int32{222222, 2, 0x09, 60, 0x0A, 130} {
		data = putInt32(data, value)
	}

	return data
}

func buildNetPingData() []byte {
	data := []byte{netPingPacketID}
	data = putInt32(data, 470)

	return data
}

func buildAttackData() []byte {
	data := []byte{attackPacketID}
	data = putInt32(data, 111111)
	data = putInt32(data, 222222)
	data = putInt32(data, 33)
	data = append(data, 0x01)
	for _, value := range []int32{46000, 51000, -3400} {
		data = putInt32(data, value)
	}
	data = putInt16(data, 1) // one more hit
	data = putInt32(data, 222222)
	data = putInt32(data, 8)
	data = append(data, 0x00)
	for _, value := range []int32{46200, 51200, -3300} {
		data = putInt32(data, value)
	}

	return data
}

func buildCharSelectedData() []byte {
	data := []byte{charSelectedPacketID}
	data = append(data, utf16("sweep")...)
	data = putInt32(data, 555555)
	data = append(data, utf16("Sweeper")...)
	// Session id, clan id, unknown, sex, race, class, active, x, y, z.
	for _, value := range []int32{9, 0, 0, 0, 2, 18, 1, 46000, 51000, -3400} {
		data = putInt32(data, value)
	}
	data = putFloat64(data, 96)
	data = putFloat64(data, 33)

	return data
}

func TestUserInfoTruncatedPrefixesError(t *testing.T) {
	parse := func(data []byte) error {
		return ParseUserInfoPacket(NewUserInfoPacket(), data)
	}

	requireTruncatedPrefixesError(t, buildUserInfoData(), parse)
}

func buildUserInfoData() []byte {
	data := []byte{userInfoPacketID}
	// X, y, z and the vehicle id.
	for _, value := range []int32{46000, 51000, -3400, 0} {
		data = putInt32(data, value)
	}
	data = putInt32(data, 111111)
	data = append(data, utf16("sweep")...)
	// Race, female, class, level and exp.
	for _, value := range []int32{2, 0, 25, 6, 1000} {
		data = putInt32(data, value)
	}
	// Six stats, four vitals, sp, load and max load.
	for _, value := range []int32{
		30, 31, 32, 33, 34, 35, 130, 90, 42, 35, 12, 700, 80000,
	} {
		data = putInt32(data, value)
	}
	data = append(data, make([]byte, userInfoWeaponFlagSkip)...)
	for index := range userInfoPaperdollSlots {
		data = putInt32(data, int32(268473900+index))
	}
	data = append(data, make([]byte, userInfoStatsSkip)...)
	data = putInt32(data, 170) // run speed
	data = putInt32(data, 85)  // walk speed
	data = append(data, make([]byte, userInfoSpeedTrail)...)
	data = putFloat64(data, 1.2)

	return data
}

func TestNpcInfoTruncatedPrefixesError(t *testing.T) {
	parse := func(data []byte) error {
		return ParseNpcInfoPacket(NewNpcInfoPacket(), data)
	}

	requireTruncatedPrefixesError(t, buildNpcInfoData(), parse)
}

func buildNpcInfoData() []byte {
	data := []byte{npcInfoPacketID}
	// Object id, template id and the attackable flag.
	for _, value := range []int32{222222, 1001278, 1} {
		data = putInt32(data, value)
	}
	// X, y, z and the heading.
	for _, value := range []int32{46000, 51000, -3400, 16384} {
		data = putInt32(data, value)
	}
	data = append(data, make([]byte, npcInfoSpeedLead)...)
	data = putInt32(data, 160) // run speed
	data = putInt32(data, 60)  // walk speed
	data = append(data, make([]byte, npcInfoSpeedTrail)...)
	data = putFloat64(data, 1.1)
	data = append(data, make([]byte, npcInfoAtkSpeedTail)...)
	data = putFloat64(data, 12)
	data = append(data, make([]byte, npcInfoBodyTail)...)
	data = append(data, 1, 0, 1, 0, 0) // running, dead flags
	data = append(data, utf16("Gremlin")...)
	data = append(data, utf16("")...)

	return data
}

func TestCharInfoTruncatedPrefixes(t *testing.T) {
	full := buildCharInfoData()
	parse := func(data []byte) error {
		return ParseCharInfoPacket(NewCharInfoPacket(), data)
	}
	require.NoError(t, parse(full))

	// The clan tail and the seven state flag bytes behind the title are
	// optional: a truncated tail keeps the flag defaults. Everything
	// before the title is mandatory.
	optional := charInfoClanTail + 7
	mandatory := len(full) - optional
	for size := range mandatory {
		require.Error(t, parse(full[:size]), "prefix of len %d", size)
	}

	t.Run("missing tail keeps defaults", func(t *testing.T) {
		p := NewCharInfoPacket()
		require.NoError(t, ParseCharInfoPacket(p, full[:mandatory]))
		require.False(t, p.Running)
	})

	t.Run("clan tail cut keeps defaults", func(t *testing.T) {
		p := NewCharInfoPacket()
		require.NoError(t, ParseCharInfoPacket(p, full[:mandatory+5]))
		require.False(t, p.Running)
	})

	t.Run("flags cut in the middle keep defaults", func(t *testing.T) {
		p := NewCharInfoPacket()
		require.NoError(t, ParseCharInfoPacket(p, full[:len(full)-3]))
		require.False(t, p.Running)
	})
}

func buildCharInfoData() []byte {
	data := []byte{charInfoPacketID}
	// X, y, z and the vehicle id.
	for _, value := range []int32{46000, 51000, -3400, 0} {
		data = putInt32(data, value)
	}
	data = putInt32(data, 222222)
	data = append(data, utf16("Sweeper")...)
	// Race, female and the base class.
	for _, value := range []int32{2, 0, 25} {
		data = putInt32(data, value)
	}
	data = append(data, make([]byte, charInfoSpeedLead)...)
	data = putInt32(data, 140) // run speed
	data = putInt32(data, 70)  // walk speed
	data = append(data, make([]byte, charInfoSpeedTrail)...)
	data = putFloat64(data, 1.05)
	data = append(data, make([]byte, charInfoAtkSpeedTail)...)
	data = putFloat64(data, 8)
	data = append(data, make([]byte, charInfoBodyTail)...)
	data = append(data, utf16("SweepTitle")...)
	data = append(data, make([]byte, charInfoClanTail)...)
	data = append(data, 1, 1, 0, 0, 0, 0, 0)

	return data
}

func TestCharSelectInfoTruncatedPrefixesError(t *testing.T) {
	data := append([]byte{charSelectInfoPacketID}, putInt32(nil, 1)...)
	data = append(data, buildCharacterEntry("sweep", "sweep")...)
	parse := func(data []byte) error {
		return ParseCharSelectInfoPacket(NewCharSelectInfoPacket(), data)
	}

	require.NoError(t, parse(data))
	requireTruncatedPrefixesError(t, data, parse)
}

func TestParseSystemMessageParamTypes(t *testing.T) {
	t.Run("typed parameters", func(t *testing.T) {
		data := []byte{systemMessagePacketID}
		data = putInt32(data, 42) // message id
		data = putInt32(data, 4)  // param count
		// Skill name parameter: id and level ints.
		data = putInt32(data, sysParamSkillName)
		data = putInt32(data, 194)
		data = putInt32(data, 1)
		// Zone name parameter: id, y and z ints.
		data = putInt32(data, sysParamZoneName)
		data = putInt32(data, 7)
		data = putInt32(data, 100)
		data = putInt32(data, 200)
		// Player name parameter: a utf16 string.
		data = putInt32(data, sysParamPlayerName)
		data = append(data, utf16("Sweeper")...)
		// Npc name parameter: one int.
		data = putInt32(data, sysParamNpcName)
		data = putInt32(data, 7147)

		p := NewSystemMessagePacket()
		require.NoError(t, ParseSystemMessagePacket(p, data))
		require.Equal(t, int32(42), p.MessageID)
		require.Len(t, p.Params, 4)
		require.Equal(t, int32(sysParamSkillName), p.Params[0].Type)
		require.Equal(t, int32(194), p.Params[0].Int)
		require.Equal(t, int32(sysParamZoneName), p.Params[1].Type)
		require.Equal(t, int32(7), p.Params[1].Int)
		require.Equal(t, int32(sysParamPlayerName), p.Params[2].Type)
		require.Equal(t, "Sweeper", p.Params[2].Text)
		require.Equal(t, int32(sysParamNpcName), p.Params[3].Type)
	})

	t.Run("implausible param counts", func(t *testing.T) {
		for _, count := range []int32{systemParamsCap + 1, -1} {
			data := []byte{systemMessagePacketID}
			data = putInt32(data, 1)
			data = putInt32(data, count)

			p := NewSystemMessagePacket()
			require.Error(t, ParseSystemMessagePacket(p, data), "count %d", count)
		}
	})

	t.Run("truncated prefixes error", func(t *testing.T) {
		data := []byte{systemMessagePacketID}
		data = putInt32(data, 42)
		data = putInt32(data, 2)
		// One text parameter and one truncated int parameter.
		data = putInt32(data, sysParamText)
		data = append(data, utf16("sweep")...)
		data = putInt32(data, sysParamIntNumber)
		data = putInt32(data, 5)

		parse := func(data []byte) error {
			return ParseSystemMessagePacket(NewSystemMessagePacket(), data)
		}

		require.NoError(t, parse(data))
		requireTruncatedPrefixesError(t, data, parse)
	})
}

func TestStatusUpdateForEachCapsStoredAttributes(t *testing.T) {
	data := []byte{statusUpdatePacketID}
	data = putInt32(data, 222222)
	data = putInt32(data, 12)
	for i := range 12 {
		data = putInt32(data, int32(i))
		data = putInt32(data, int32(i*10))
	}

	p := NewStatusUpdatePacket()
	require.NoError(t, ParseStatusUpdatePacket(p, data))

	visited := 0
	p.ForEach(func(id int32, _ int32) {
		visited++
		require.Equal(t, int32(visited-1), id)
	})
	require.Equal(t, statusUpdateMaxAttrs, visited)
}

func TestItemListTruncatedEntryStops(t *testing.T) {
	entry := inventoryItemEntry(nil, 3, 57, 25, 4, 0)

	// A truncated item entry stops the list without a parse error: the
	// entries parsed so far stay and the rest is dropped.
	for size := range entry {
		data := []byte{itemListPacketID}
		data = putInt16(data, 0)
		data = putInt16(data, 1)
		data = append(data, entry[:size]...)

		p := NewItemListPacket()
		require.NoError(t, ParseItemListPacket(p, data))
		require.Empty(t, p.Items)
	}
}

func TestInventoryUpdateTruncatedPrefixesError(t *testing.T) {
	data := []byte{inventoryUpdatePacketID}
	data = putInt16(data, 1)
	data = putInt16(data, 1) // change code: add
	data = inventoryItemEntry(data, 3, 57, 25, 4, 0)

	parse := func(data []byte) error {
		return ParseInventoryUpdatePacket(NewInventoryUpdatePacket(), data)
	}

	require.NoError(t, parse(data))
	requireTruncatedPrefixesError(t, data, parse)
}
