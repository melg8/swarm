// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"sort"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// starterSet lists the item ids of the newbie kits the server never
// takes back: the Mobius item xml flags them is_sellable=false and
// is_dropable=false, so no shop buys them and the ground refuses
// them - the destroy request is the only way they ever leave the
// character. The elven fighter starts with the Dagger (item 10, the
// weapon of the creation kit) plus the Squire's armor set; every kit
// piece the paperdoll no longer needs is dead weight (the shirt alone
// weighs 3300, the dagger 1160), the hunt loop destroys them through
// ReplacedStarterItems once a better weapon or armor piece is worn.
var starterSet = map[int32]bool{
	10:   true, // Dagger (the elven fighter's starter weapon)
	1146: true, // Squire's Shirt
	1147: true, // Squire's Pants
	2369: true, // Squire's Sword
}

// IsStarterItem reports whether the item id belongs to the newbie kit
// the shops refuse to buy and the ground refuses to take: the planner
// must never count its sell value and the junk flows must never offer
// it - the destroy request is its only way out (see
// ReplacedStarterItems).
func IsStarterItem(itemID int32) bool {
	return starterSet[itemID]
}

// StarterDrop is one replaced starter item scheduled for destruction.
type StarterDrop struct {
	// Item is the inventory entry to destroy.
	Item state.InventoryItem
	// Weight is the unit weight of the item: the dead load the
	// destruction frees.
	Weight int32
	// Reason is the human readable log line.
	Reason string
}

// ReplacedStarterItems returns the unequipped starter kit items whose
// paperdoll slot a replacement already occupies with an equal or
// better profile score: the equip planner would never pick them again
// (a swap needs a strictly better candidate), so they are dead weight
// the shops refuse to buy and the ground refuses to take. An empty
// slot keeps its starter item - the auto equipment still wears it -
// and a starter item with no replacement in sight is never returned.
// The order is deterministic (object ids ascending).
func ReplacedStarterItems(
	profile Profile, equipment Equipment,
) []StarterDrop {
	paperdoll := equipment.Paperdoll(profile)
	replaced := make([]StarterDrop, 0, len(starterSet))
	for _, item := range equipment.Items {
		if item.Equipped || !starterSet[item.ItemID] {
			continue
		}
		stats, ok := npcdata.ItemGearStats(item.ItemID)
		if !ok {
			continue
		}
		score := scoreStats(profile, stats)
		if score <= 0 {
			continue
		}
		current := starterReplacement(paperdoll, stats)
		if paperdollEmpty(current) {
			continue
		}
		if current.Score >= score {
			replaced = append(replaced, StarterDrop{
				Item:   item,
				Weight: npcdata.ItemWeight(item.ItemID),
				Reason: "destroying " + describeItem(ScoredItem{
					Item:  item,
					Stats: stats,
					Score: score,
					Slot:  slotInvalid,
				}) + ": the equipped " + describeItem(current) +
					" replaced it (unsellable, undroppable)",
			})
		}
	}
	sort.Slice(replaced, func(i int, j int) bool {
		return replaced[i].Item.ObjectID < replaced[j].Item.ObjectID
	})

	return replaced
}

// starterReplacement resolves the paperdoll entry a starter item of
// the bodypart competes with: the chest and right hand items their own
// slot, the legs item the legs slot unless a one-piece armor occupies
// the chest (the one-piece blocks the legs slot and displaces the
// starter pants with itself).
func starterReplacement(
	paperdoll [slotCount]ScoredItem, stats npcdata.GearStats,
) ScoredItem {
	if stats.BodyPart == partLegs &&
		paperdoll[SlotChest].Stats.BodyPart == partOnepiece {
		return paperdoll[SlotChest]
	}
	slots := SlotsForBodyPart(stats.BodyPart)
	if len(slots) == 0 {
		return clearedScoredItem
	}

	return paperdoll[slots[0]]
}

// paperdollEmpty reports whether the paperdoll entry holds no item
// (the zero entry Paperdoll leaves in slots without usable gear).
func paperdollEmpty(entry ScoredItem) bool {
	return entry.Item.ObjectID == 0 && entry.Stats.BodyPart == ""
}
