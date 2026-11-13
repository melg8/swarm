// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package npcdata carries the generated name dictionaries of the npc and item
// display ids. The Mobius server sends NpcInfo packets with empty names for
// npcs that resolve them client side (classic NPCName-e.dat) and DropItem
// packets with only the item display id, so the bot resolves the names itself.
package npcdata

// NPCDisplayOffset is added to the npc display id inside NpcInfo packets.
const NPCDisplayOffset = 1000000

// NPCName resolves the npc name by the raw NpcInfo template id (display id +
// 1000000), empty for an unknown template.
func NPCName(templateID int32) string {
    if templateID <= NPCDisplayOffset {
        return ""
    }

    return npcNames[templateID-NPCDisplayOffset]
}

// NPCLevel resolves the npc level by the raw NpcInfo template id, zero for an
// unknown template.
func NPCLevel(templateID int32) int32 {
    if templateID <= NPCDisplayOffset {
        return 0
    }

    return npcLevels[templateID-NPCDisplayOffset]
}

// NPCAggroRange resolves the effective aggro range the server builds into the
// template: the xml ai aggroRange capped at the deployed MaxAggroRange
// (NPC.ini) for the Monster type only and the raw xml value for every other
// type. It is the radius AttackableAI tests with isInsideRadius3D before it
// consults the aggression flag, not a promise that the npc attacks on sight;
// zero for an unknown template or no range.
func NPCAggroRange(templateID int32) int32 {
    if templateID <= NPCDisplayOffset {
        return 0
    }

    return npcAggroRanges[templateID-NPCDisplayOffset]
}

// NPCIsAggressive resolves the effective on-sight aggression: a monster class
// template (Monster, RaidBoss, GrandBoss) with a true ai isAggressive flag and
// a positive effective aggro range attacks a neutral player on sight. Guards
// and friendly mobs answer only players with karma and folk never attack, so
// they resolve false even when their xml flags suggest otherwise.
func NPCIsAggressive(templateID int32) bool {
    if templateID <= NPCDisplayOffset {
        return false
    }

    return npcAggressives[templateID-NPCDisplayOffset]
}

// NPCClanHelpRange resolves the ai clanHelpRange: the distance within which the
// attacked npc calls its clan mates. Zero for an unknown template or a lone
// hunter.
func NPCClanHelpRange(templateID int32) int32 {
    if templateID <= NPCDisplayOffset {
        return 0
    }

    return npcClanHelpRanges[templateID-NPCDisplayOffset]
}

// NPCClans resolves the clan names of an npc: AttackableAI answers a player
// attack with a clan call, where every nearby npc whose clans intersect the
// clans of the attacked npc joins the fight (the special clan ALL matches every
// clan). Nil for an unknown template or a clan less npc; the returned list is
// the shared pre-split dictionary entry (see clans.go), read-only.
func NPCClans(templateID int32) []string {
    if templateID <= NPCDisplayOffset {
        return nil
    }

    return npcClanSets[templateID-NPCDisplayOffset]
}

// NPCWireTemplateID maps the Mobius internal (CT0 xml) template id onto the
// NpcInfo wire template id: NpcIdConverter translates the xml stats id onto the
// C4 display id and the wire adds the 1000000 offset (the Kaboo Orc Fighter
// 20471 arrives as 1000471). The hunt zone registries carry the xml ids of the
// spawn data, so their mob priorities resolve through this map. An id outside
// the converter table passes through unchanged (a wire id or a hand written
// test id stays itself, an unmapped internal id keeps its value and the bias
// never matches, so the plain nearest-first pick applies).
func NPCWireTemplateID(internalID int32) int32 {
    if wire, ok := npcInternalWireIDs[internalID]; ok {
        return wire
    }

    return internalID
}

// ItemName resolves the item name by its display id, empty when unknown.
func ItemName(displayID int32) string {
    return itemNames[displayID]
}

// ItemPrice resolves the reference price of an item by its display id; the
// Mobius server sells at referencePrice/2. Zero when the item is unknown.
func ItemPrice(displayID int32) int64 {
    return itemPrices[displayID]
}

// ItemWeight resolves the unit item weight by its display id, zero when
// unknown.
func ItemWeight(displayID int32) int32 {
    return itemWeights[displayID]
}

// ItemIcon resolves the icon file name (without extension) by the display id,
// served by the web interface as /icons/<name>.png. Empty when the item is
// unknown.
func ItemIcon(displayID int32) string {
    return itemIcons[displayID]
}

// GearStats are the combat stats of an equippable item from the item stats xml:
// the bodypart (rhand, lhand, lrhand, chest, legs, onepiece, head, gloves,
// feet, back, underwear, neck, rear;lear, rfinger;lfinger), the weapon type
// (SWORD, BLUNT, DAGGER, POLE, BOW, ...) and the stat block values, from which
// the gear scoring profiles derive the slot and the value of an item. Type is
// the XML category (Weapon, Armor, EtcItem), ArmorType the LIGHT/HEAVY/ROBE
// family, SoulShots and SpiritShots the per swing consumption count of the item
// status tooltip.
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

// ItemGearStats resolves the combat stats of an equippable item by its display
// id; false for an unknown item or one without a bodypart (a consumable, a
// material).
func ItemGearStats(displayID int32) (GearStats, bool) {
    stats, ok := itemGearStats[displayID]

    return stats, ok
}

// ItemType resolves the XML category of an item by its display id ("Weapon",
// "Armor", "Shield", "EtcItem", "Asset", "Lure", "Arrow", "QuestItem"), empty
// when unknown. The category drives the family specific layout of the item
// status tooltip.
func ItemType(displayID int32) string {
    if stats, ok := itemGearStats[displayID]; ok {
        return stats.Type
    }

    return itemTypes[displayID]
}

// BuyListsOfNPC resolves the buylist ids a merchant sells by its packet
// template id (the NpcInfo display id, for example 7147 for the elven weapon
// trader Unoren). Nil when the npc sells nothing.
func BuyListsOfNPC(templateID int32) []int32 {
    return npcBuyLists[templateID]
}

// ItemsOfBuyList resolves the item ids of a buylist, nil when the list is
// unknown.
func ItemsOfBuyList(listID int32) []int32 {
    return buyListItems[listID]
}
