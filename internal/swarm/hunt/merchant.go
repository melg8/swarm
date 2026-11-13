// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "strings"
    "sync"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// The merchant wares knowledge of the town trips: the server resolves
// RequestBuyItem through the targeted folk npc and refuses every line of a
// list the npc does not trade, silently (a chat message for the flood, a
// bare ActionFailed for the range and foreign list refusals), so a buy aimed
// at the wrong trader burns the retry budget. Every purchase therefore rides
// the merchant whose buylist carries the item, while the sells stay universal
// through the standard inventory sell list.

// MerchantWares is the set of goods families a town merchant trades, derived
// from the generated buylist catalogs at runtime.
type MerchantWares uint8

const (
    // WaresNone marks an npc the catalogs sell nothing through.
    WaresNone MerchantWares = 0
    // WaresWeapon covers the hand weapons (the weapon trader).
    WaresWeapon MerchantWares = 1 << 0
    // WaresArmor covers the armor pieces and the shields (the armor
    // trader: the shields share the armor shop lists).
    WaresArmor MerchantWares = 1 << 1
    // WaresJewel covers the earrings, the rings and the necklaces.
    WaresJewel MerchantWares = 1 << 2
    // WaresMagic covers the spellbooks (the magic goods of the jewel
    // trader).
    WaresMagic MerchantWares = 1 << 3
    // WaresConsumable covers the arrows, the potions, the scrolls and
    // the rest of the grocer stock.
    WaresConsumable MerchantWares = 1 << 4
)

// magicNamePrefixes are the display name prefixes of the magic goods
// the town traders stock (the item types carry no dedicated family
// for them: the spellbooks, the mystic amulets and the summon
// blueprints answer the plain EtcItem category).
var magicNamePrefixes = []string{
    "Spellbook:", "Amulet:", "Blueprint:",
}

// The gear families of the generated item tables (npcdata.ItemType).
const (
    itemTypeWeapon = "Weapon"
    itemTypeArmor  = "Armor"
    itemTypeShield = "Shield"
)

// merchantWaresCache holds the wares of the resolved merchants: the
// classification walks the merchant's whole buylist stock and the
// trip planning asks for it per frozen plan line.
var merchantWaresCache sync.Map

// String renders the wares set for the log lines ("weapons",
// "armor, jewels").
func (w MerchantWares) String() string {
    if w == WaresNone {
        return "nothing"
    }
    families := []struct {
        flag MerchantWares
        name string
    }{
        {WaresWeapon, "weapons"},
        {WaresArmor, "armor"},
        {WaresJewel, "jewels"},
        {WaresMagic, "spellbooks"},
        {WaresConsumable, "consumables"},
    }
    names := make([]string, 0, len(families))
    for _, family := range families {
        if w&family.flag != 0 {
            names = append(names, family.name)
        }
    }

    return strings.Join(names, ", ")
}

// merchantWares classifies the goods of the merchant template id (the
// display id of the NpcInfo packets) from its generated buylists:
// every family the lists carry sets its flag.
func merchantWares(templateID int32) MerchantWares {
    if cached, ok := merchantWaresCache.Load(templateID); ok {
        return cached.(MerchantWares)
    }
    var wares MerchantWares
    for _, listID := range npcdata.BuyListsOfNPC(templateID) {
        for _, itemID := range npcdata.ItemsOfBuyList(listID) {
            wares |= itemWares(itemID)
        }
    }
    merchantWaresCache.Store(templateID, wares)

    return wares
}

// itemWares classifies one catalog item into its goods family: the
// equippables split by their gear stats (the shields trade at the
// armor shop with the armor pieces, the jewels sit on the pair and
// the neck bodyparts - the arrows carry a left hand bodypart too and
// must not read as shields, the item type gate catches them first),
// the magic goods carry the display name prefixes and the rest of
// the stock belongs to the grocer.
func itemWares(itemID int32) MerchantWares {
    stats, hasStats := npcdata.ItemGearStats(itemID)
    if !hasStats || stats.Type != itemTypeArmor {
        if hasStats && stats.Type == itemTypeWeapon {
            return WaresWeapon
        }
        name := npcdata.ItemName(itemID)
        for _, prefix := range magicNamePrefixes {
            if strings.HasPrefix(name, prefix) {
                return WaresMagic
            }
        }

        return WaresConsumable
    }
    if gear.CategoryOf(stats) == gear.CategoryJewel {
        return WaresJewel
    }

    return WaresArmor
}

// merchantSellsItem reports whether one of the merchant's buylists
// carries the item: the exact check the buy enforcement rides (the
// family classification above is the human knowledge, the lists are
// the authority).
func merchantSellsItem(templateID int32, itemID int32) bool {
    for _, listID := range npcdata.BuyListsOfNPC(templateID) {
        for _, product := range npcdata.ItemsOfBuyList(listID) {
            if product == itemID {
                return true
            }
        }
    }

    return false
}

// merchantTemplateWanted reports whether the tracker template id of
// the selected npc answers one of the wanted merchant templates.
func merchantTemplateWanted(templateID int32, templates []int32) bool {
    for _, id := range templates {
        if id == templateID {
            return true
        }
    }

    return false
}
