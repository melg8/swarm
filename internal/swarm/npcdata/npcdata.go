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

// ItemName resolves the name of a ground item by its display id. It
// returns an empty string when the item is unknown.
func ItemName(displayID int32) string {
	return itemNames[displayID]
}
