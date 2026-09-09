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

// NPCClanHelpRange resolves the ai clanHelpRange of an npc by the raw
// NpcInfo template id: the distance within the attacked npc calls its
// clan mates to help. It returns zero when the template is unknown or
// the npc hunts alone.
func NPCClanHelpRange(templateID int32) int32 {
	if templateID <= npcTemplateOffset {
		return 0
	}

	return npcClanHelpRanges[templateID-npcTemplateOffset]
}

// NPCClans resolves the clan names of an npc by the raw NpcInfo template
// id. The Mobius AttackableAI answers a player attack with a clan call:
// every nearby npc whose clans intersect the clans of the attacked npc
// (the special clan ALL matches every clan) joins the fight. It returns
// nil when the template is unknown or the npc belongs to no clan. The
// returned list is the shared pre-split dictionary entry (see clans.go)
// and must be treated as read-only.
func NPCClans(templateID int32) []string {
	if templateID <= npcTemplateOffset {
		return nil
	}

	return npcClanSets[templateID-npcTemplateOffset]
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

// ItemIcon resolves the icon file name (without extension) of an item
// by its display id, served by the web interface as
// /icons/<name>.png. It returns an empty string when the item is
// unknown.
func ItemIcon(displayID int32) string {
	return itemIcons[displayID]
}

// GearStats are the combat stats of an equippable item from the item
// stats xml: the bodypart of the template (rhand, lhand, lrhand, chest,
// legs, onepiece, head, gloves, feet, back, underwear, neck,
// rear;lear, rfinger;lfinger), the weapon type for weapons (SWORD,
// BLUNT, DAGGER, POLE, BOW, ...) and the stats block values. The gear
// scoring profiles of the bot derive the slot and the value of an item
// from them.
//
// Type, ArmorType, SoulShots and SpiritShots power the item status
// tooltip of the equipment widget: Type is the XML category (Weapon,
// Armor, EtcItem), ArmorType the LIGHT/HEAVY/ROBE family of an armor
// piece, SoulShots/SpiritShots the per swing consumption count.
type GearStats struct {
	BodyPart    string
	WeaponType  string
	ArmorType   string
	Type        string
	PAtk        int32
	MAtk        int32
	PDef        int32
	MDef        int32
	SDef        int32
	RShld       int32
	PAtkSpd     int32
	SoulShots   int32
	SpiritShots int32
}

// ItemGearStats resolves the combat stats of an equippable item by its
// display id. It reports false when the item is unknown or carries no
// bodypart (a consumable, a material: nothing the bot can equip).
func ItemGearStats(displayID int32) (GearStats, bool) {
	stats, ok := itemGearStats[displayID]

	return stats, ok
}

// ItemType resolves the XML category of an item by its display id
// ("Weapon", "Armor", "Shield", "EtcItem", "Asset", "Lure", "Arrow",
// "QuestItem"). It returns an empty string when the item is unknown.
// The category drives the family specific layout of the item status
// tooltip.
func ItemType(displayID int32) string {
	if stats, ok := itemGearStats[displayID]; ok {
		return stats.Type
	}

	return itemTypes[displayID]
}

// BuyListsOfNPC resolves the buylist ids a merchant sells by its
// packet template id (the display id of the NpcInfo packets, for
// example 7147 for the elven weapon trader Unoren). It returns nil
// when the npc sells nothing.
func BuyListsOfNPC(templateID int32) []int32 {
	return npcBuyLists[templateID]
}

// ItemsOfBuyList resolves the item ids of a buylist. It returns nil
// when the list is unknown.
func ItemsOfBuyList(listID int32) []int32 {
	return buyListItems[listID]
}
