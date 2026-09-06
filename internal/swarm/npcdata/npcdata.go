// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package npcdata carries the generated name dictionaries of the npc and
// item display ids. The Mobius server sends NpcInfo packets with empty
// names for npcs that resolve their names on the client side (classic
// NPCName-e.dat) and DropItem packets only carry the item display id, so
// the bot resolves the names itself.
package npcdata

// npcTemplateOffset is added to the display id inside NpcInfo packets.
const npcTemplateOffset = 1000000

// NPCName resolves the name of an npc by the raw NpcInfo template id
// (display id + 1000000). It returns an empty string when the template is
// unknown.
func NPCName(templateID int32) string {
	if templateID <= npcTemplateOffset {
		return ""
	}

	return npcNames[templateID-npcTemplateOffset]
}

// NPCLevel resolves the level of an npc by the raw NpcInfo template id.
// It returns zero when the template is unknown.
func NPCLevel(templateID int32) int32 {
	if templateID <= npcTemplateOffset {
		return 0
	}

	return npcLevels[templateID-npcTemplateOffset]
}

// NPCAggroRange resolves the ai aggroRange of an npc by the raw NpcInfo
// template id. It returns zero when the template is unknown or passive.
func NPCAggroRange(templateID int32) int32 {
	if templateID <= npcTemplateOffset {
		return 0
	}

	return npcAggroRanges[templateID-npcTemplateOffset]
}

// NPCIsAggressive resolves the ai isAggressive flag of an npc by the raw
// NpcInfo template id: aggressive npcs attack players on sight.
func NPCIsAggressive(templateID int32) bool {
	if templateID <= npcTemplateOffset {
		return false
	}

	return npcAggressives[templateID-npcTemplateOffset]
}

// ItemName resolves the name of a ground item by its display id. It
// returns an empty string when the item is unknown.
func ItemName(displayID int32) string {
	return itemNames[displayID]
}

// ItemPrice resolves the reference price of an item by its display id.
// The Mobius server sells items at referencePrice/2. It returns zero
// when the item is unknown.
func ItemPrice(displayID int32) int64 {
	return itemPrices[displayID]
}

// ItemWeight resolves the unit weight of an item by its display id. It
// returns zero when the item is unknown.
func ItemWeight(displayID int32) int32 {
	return itemWeights[displayID]
}
