// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "sort"

// QuestEntryView is one active quest of the journal as the
// connection layer hands it over: the quest id and the state int
// (the cond step of a started quest or the completion flags mask -
// see docs/quest_protocol.md).
type QuestEntryView struct {
	QuestID int32
	State   int32
}

// QuestItemView is one quest item stack of the journal: the item id
// and its count. The object id of the packet is inventory detail the
// tracker already owns through the ordinary item packets; what the
// journal adds is WHICH items are quest bound - the fact the sell
// filter of the shop strategy will need.
type QuestItemView struct {
	ItemID int32
	Count  int32
}

// ApplyQuestList replaces the quest journal: the server always sends
// the whole journal (at world entry and after every state change),
// so the maps are replaced, not merged - the same whole-list
// semantics the skill list applies.
func (b *Bot) ApplyQuestList(quests []QuestEntryView, items []QuestItemView) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.quests = make(map[int32]int32, len(quests))
	for _, quest := range quests {
		b.quests[quest.QuestID] = quest.State
	}
	b.questItems = make(map[int32]int32, len(items))
	for _, item := range items {
		b.questItems[item.ItemID] = item.Count
	}
	b.touch()
}

// QuestCond returns the journal state int of the quest (the cond
// step or the completion flags mask) and whether the quest is active
// at all. A started Q00406 at cond 2 answers (2, true); a quest the
// journal does not carry answers (0, false).
func (b *Bot) QuestCond(questID int32) (int32, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cond, active := b.quests[questID]

	return cond, active
}

// QuestCount returns the number of active quests of the journal.
func (b *Bot) QuestCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.quests)
}

// QuestIDs returns the sorted ids of the active quests (sorted for
// the deterministic snapshot and test output).
func (b *Bot) QuestIDs() []int32 {
	b.mu.RLock()
	ids := make([]int32, 0, len(b.quests))
	for questID := range b.quests {
		ids = append(ids, questID)
	}
	b.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	return ids
}

// IsQuestItem reports whether the item id is a quest item of the
// current journal - the sell filter answer for the shop strategy
// (a quest item must never enter the junk batch; the quest engine
// removes them itself on quest exit).
func (b *Bot) IsQuestItem(itemID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, quest := b.questItems[itemID]

	return quest
}

// QuestItemCount returns the journal count of the quest item stack
// and whether the journal carries the item at all.
func (b *Bot) QuestItemCount(itemID int32) (int32, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	count, present := b.questItems[itemID]

	return count, present
}
