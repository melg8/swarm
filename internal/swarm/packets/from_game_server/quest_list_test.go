// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// buildQuestListPayload writes a QuestList packet body: [opcode]
// [questCount: 2] then per quest [questId: 4][state: 4], then
// [itemCount: 2] and per item [objectId: 4][itemId: 4][count: 4]
// [bodyPart: 4].
func buildQuestListPayload(
	quests [][2]int32, items [][4]int32,
) []byte {
	writer := packet.NewWriter()
	writer.WriteByte(questListPacketID)
	writer.WriteInt16(int16(len(quests)))
	for _, quest := range quests {
		writer.WriteInt32(quest[0]) // quest id
		writer.WriteInt32(quest[1]) // state (cond or completion flags)
	}
	writer.WriteInt16(int16(len(items)))
	for _, item := range items {
		writer.WriteInt32(item[0]) // object id
		writer.WriteInt32(item[1]) // item id
		writer.WriteInt32(item[2]) // count
		writer.WriteInt32(item[3]) // body part mask
	}

	return writer.Bytes()
}

// TestParseQuestListPacketEmptyLiveForm pins the empty journal the
// live stack pushes at world entry: 5 bytes, both counts zero (the
// 2026-09-12 trace round of docs/quest_protocol.md).
func TestParseQuestListPacketEmptyLiveForm(t *testing.T) {
	p := NewQuestListPacket()
	// [0x98][00 00][00 00]
	data := []byte{0x98, 0x00, 0x00, 0x00, 0x00}

	require.NoError(t, ParseQuestListPacket(p, data))
	require.Empty(t, p.Quests)
	require.Empty(t, p.Items)
}

// TestParseQuestListPacket pins the populated journal: the two class
// transfer quests at their conds plus the quest item stacks of the
// Q00406 run (the topaz pieces).
func TestParseQuestListPacket(t *testing.T) {
	p := NewQuestListPacket()
	data := buildQuestListPayload(
		[][2]int32{
			{406, 1}, // Q00406 Path to an Elven Knight, cond 1
			{407, 3}, // Q00407 Path to an Elven Scout, cond 3
		},
		[][4]int32{
			{268502145, 1205, 20, 0}, // Topaz Piece stack
			{268502146, 1206, 7, 0},  // Emerald Piece stack
		},
	)

	require.NoError(t, ParseQuestListPacket(p, data))
	require.Len(t, p.Quests, 2)
	require.Equal(t, int32(406), p.Quests[0].QuestID)
	require.Equal(t, int32(1), p.Quests[0].State)
	require.Equal(t, int32(407), p.Quests[1].QuestID)
	require.Equal(t, int32(3), p.Quests[1].State)

	require.Len(t, p.Items, 2)
	require.Equal(t, int32(268502145), p.Items[0].ObjectID)
	require.Equal(t, int32(1205), p.Items[0].ItemID)
	require.Equal(t, int32(20), p.Items[0].Count)
	require.Equal(t, int32(0), p.Items[0].BodyPart)
	require.Equal(t, int32(1206), p.Items[1].ItemID)
	require.Equal(t, int32(7), p.Items[1].Count)
}

// TestParseQuestListPacketReusesBuffer proves the second parse
// resets the entry buffers instead of growing them (the parse
// struct is a reusable field of the game client).
func TestParseQuestListPacketReusesBuffer(t *testing.T) {
	p := NewQuestListPacket()
	first := buildQuestListPayload(
		[][2]int32{{406, 1}, {407, 3}},
		[][4]int32{{268502145, 1205, 20, 0}},
	)
	require.NoError(t, ParseQuestListPacket(p, first))
	require.Len(t, p.Quests, 2)
	require.Len(t, p.Items, 1)

	second := buildQuestListPayload([][2]int32{{219, 5}}, nil)
	require.NoError(t, ParseQuestListPacket(p, second))
	require.Len(t, p.Quests, 1)
	require.Equal(t, int32(219), p.Quests[0].QuestID)
	require.Empty(t, p.Items)
}

// TestParseQuestListPacketRejectsBadID pins the opcode check.
func TestParseQuestListPacketRejectsBadID(t *testing.T) {
	p := NewQuestListPacket()
	data := []byte{0x97, 0x00, 0x00, 0x00, 0x00}

	require.Error(t, ParseQuestListPacket(p, data))
}

// TestParseQuestListPacketTruncatedQuests proves a quest entry cut
// mid-stream fails instead of returning half a journal.
func TestParseQuestListPacketTruncatedQuests(t *testing.T) {
	p := NewQuestListPacket()
	// One quest announced, only the id present.
	data := []byte{0x98, 0x01, 0x00, 0xF6, 0x01, 0x00, 0x00}

	require.Error(t, ParseQuestListPacket(p, data))
	require.Empty(t, p.Quests)
}

// TestParseQuestListPacketTruncatedItems proves an item entry cut
// mid-stream fails.
func TestParseQuestListPacketTruncatedItems(t *testing.T) {
	p := NewQuestListPacket()
	// No quests, one item announced, two of the four fields present.
	data := []byte{
		0x98, 0x00, 0x00, 0x01, 0x00,
		0x01, 0x00, 0x00, 0x00, // object id
		0xB5, 0x04, 0x00, 0x00, // item id
	}

	require.Error(t, ParseQuestListPacket(p, data))
}

// TestParseQuestListPacketImplausibleCounts proves the count caps
// fail fast on corrupted counts.
func TestParseQuestListPacketImplausibleCounts(t *testing.T) {
	p := NewQuestListPacket()
	// Quest count 65 (cap 64).
	questCount := buildRawQuestListCount(int16(65), 0)
	require.Error(t, ParseQuestListPacket(p, questCount))

	// Item count 257 (cap 256).
	itemCount := buildRawQuestListCount(0, int16(257))
	require.Error(t, ParseQuestListPacket(p, itemCount))
}

// buildRawQuestListCount writes the packet with raw count fields so
// the caps can be tested without building the entries.
func buildRawQuestListCount(quests int16, items int16) []byte {
	writer := packet.NewWriter()
	writer.WriteByte(questListPacketID)
	writer.WriteInt16(quests)
	writer.WriteInt16(items)

	return writer.Bytes()
}
