// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const questListPacketID byte = 0x98

// questCountCap bounds the plausible quest count of the packet: the
// engine refuses to start more than 25 quests per character
// (ScriptLink.showQuestWindow), so 64 leaves generous headroom while
// a corrupted count fails fast instead of walking garbage.
const questCountCap = 64

// questItemCountCap bounds the plausible quest item stack count: the
// quest scripts hand out a few dozen distinct items at most, so 256
// is far past every honest packet and still cheap to fail on.
const questItemCountCap = 256

// QuestListEntry is one active quest of the journal: the quest id
// (the numeric prefix of the script name, e.g. 406 of
// Q00406_PathOfTheElvenKnight) and the state int - the cond step of
// a started quest, or the completion flags mask when the quest has
// completed paths (see docs/quest_protocol.md).
type QuestListEntry struct {
	QuestID int32
	State   int32
}

// QuestItemEntry is one quest item stack the packet repeats: the
// quest items of the inventory (they still arrive through the
// ordinary inventory packets first; this section names which
// inventory items are quest bound - the sell filter fact).
type QuestItemEntry struct {
	ObjectID int32
	ItemID   int32
	Count    int32
	BodyPart int32
}

// QuestListPacket is the quest journal snapshot the server pushes at
// world entry and after every quest state change (a push protocol -
// the client never polls; see docs/quest_protocol.md).
// Wire format (see QuestList.writeImpl): [opcode 0x98]
// [questCount: 2] then per quest [questId: 4][state: 4], then
// [itemCount: 2] and per item [objectId: 4][itemId: 4][count: 4]
// [bodyPart: 4]. The empty live form is 5 bytes: one opcode, two
// zero counts.
type QuestListPacket struct {
	Quests []QuestListEntry
	Items  []QuestItemEntry
}

// NewQuestListPacket creates a packet ready for parsing with
// reusable entry buffers.
func NewQuestListPacket() *QuestListPacket {
	return &QuestListPacket{
		Quests: make([]QuestListEntry, 0, 4),
		Items:  make([]QuestItemEntry, 0, 4),
	}
}

// ParseQuestListPacket reads the packet from payload bytes.
func ParseQuestListPacket(p *QuestListPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, questListPacketID); err != nil {
		return err
	}
	if err := readQuestEntries(reader, p); err != nil {
		return err
	}

	return readQuestItemEntries(reader, p)
}

// readQuestEntries reads the quest count and the quest entries of
// the journal into the reusable buffer.
func readQuestEntries(reader *packet.Reader, p *QuestListPacket) error {
	questCount, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read quest count: %w", err)
	}
	if questCount < 0 || int(questCount) > questCountCap {
		return fmt.Errorf("implausible quest count %d", questCount)
	}
	p.Quests = p.Quests[:0]
	for range questCount {
		questID, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest id: %w", err)
		}
		state, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest state: %w", err)
		}
		p.Quests = append(p.Quests, QuestListEntry{
			QuestID: questID,
			State:   state,
		})
	}

	return nil
}

// readQuestItemEntries reads the quest item count and the item
// stacks of the journal into the reusable buffer.
func readQuestItemEntries(reader *packet.Reader, p *QuestListPacket) error {
	itemCount, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read quest item count: %w", err)
	}
	if itemCount < 0 || int(itemCount) > questItemCountCap {
		return fmt.Errorf("implausible quest item count %d", itemCount)
	}
	p.Items = p.Items[:0]
	for range itemCount {
		objectID, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest item object id: %w", err)
		}
		itemID, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest item id: %w", err)
		}
		count, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest item count: %w", err)
		}
		bodyPart, err := reader.ReadInt32()
		if err != nil {
			return fmt.Errorf("failed to read quest item body part: %w", err)
		}
		p.Items = append(p.Items, QuestItemEntry{
			ObjectID: objectID,
			ItemID:   itemID,
			Count:    count,
			BodyPart: bodyPart,
		})
	}

	return nil
}
