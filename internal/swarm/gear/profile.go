// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
)

// Profile scores equipment for one combat class. A profile decides
// which weapons the class fights with and which stats matter: the
// melee fighter ranks weapons by their physical damage output, a
// future mage profile would rank staves by magical damage and value
// robes over leather. Implementing the interface for a new class is
// the extension point of the whole gear stack: the equip planner, the
// shop strategy and the zone gear gates all dispatch through it.
type Profile interface {
	// Name is the log name of the profile.
	Name() string
	// WeaponScore ranks a weapon for the class; zero means the class
	// cannot fight with it (bows for a melee fighter, swords for a
	// mage).
	WeaponScore(stats npcdata.GearStats) float64
	// ArmorScore ranks an armor piece for the class.
	ArmorScore(stats npcdata.GearStats) float64
	// JewelScore ranks a jewel for the class.
	JewelScore(stats npcdata.GearStats) float64
	// ShieldScore ranks a shield for the class.
	ShieldScore(stats npcdata.GearStats) float64
	// WeaponPoints is the zone gating value of a weapon: the damage
	// per hit for a melee class, the magical damage for a caster.
	WeaponPoints(stats npcdata.GearStats) int32
}

// MeleeFighter is the profile of a melee damage class: swords,
// blunts, daggers, poles and fist weapons ranked by their physical
// damage output, armor by pDef, jewels by mDef and shields by the
// expected block value.
type MeleeFighter struct{}

// Name of the melee fighter profile.
func (MeleeFighter) Name() string {
	return "melee fighter"
}

// meleeWeaponTypes lists the weapon families a melee fighter swings
// in close combat; bows and the odd types (fishing rods, magic
// implements without physical damage) are excluded and score zero.
var meleeWeaponTypes = map[string]bool{
	"SWORD":      true,
	"BLUNT":      true,
	"DAGGER":     true,
	"POLE":       true,
	"DUAL":       true,
	"DUALFIST":   true,
	"FIST":       true,
	"ETC":        false,
	"BOW":        false,
	"FISHINGROD": false,
}

// WeaponScore ranks melee weapons by their damage output proxy: the
// damage per hit scales with pAtk and the hit rate with the attack
// speed.
func (MeleeFighter) WeaponScore(stats npcdata.GearStats) float64 {
	if !meleeWeaponTypes[stats.WeaponType] {
		return 0
	}

	return float64(stats.PAtk) * float64(stats.PAtkSpd)
}

// ArmorScore ranks armor by its physical defense.
func (MeleeFighter) ArmorScore(stats npcdata.GearStats) float64 {
	return float64(stats.PDef)
}

// JewelScore ranks jewels by their magical defense.
func (MeleeFighter) JewelScore(stats npcdata.GearStats) float64 {
	return float64(stats.MDef)
}

// ShieldScore ranks shields by their expected block value: the block
// power hits on the block rate share of the incoming swings.
func (MeleeFighter) ShieldScore(stats npcdata.GearStats) float64 {
	return float64(stats.SDef) * float64(stats.RShld) / 100
}

// WeaponPoints is the damage per hit of the melee weapon.
func (MeleeFighter) WeaponPoints(stats npcdata.GearStats) int32 {
	if !meleeWeaponTypes[stats.WeaponType] {
		return 0
	}

	return stats.PAtk
}

// MysticFighter is the profile of a caster class: magic implements
// (the staves) ranked by their magical damage output, armor by pDef,
// jewels by mDef and shields by the expected block value. The melee
// weapons of the fighter families score zero - a caster swings the
// staff only when the mana runs dry.
type MysticFighter struct{}

// Name of the mystic fighter profile.
func (MysticFighter) Name() string {
	return "mystic fighter"
}

// WeaponScore ranks caster weapons by their magical damage output:
// the magic attack of the implement scaled by its attack speed (the
// casting itself is instant, the number only orders the purchases).
func (MysticFighter) WeaponScore(stats npcdata.GearStats) float64 {
	return float64(stats.MAtk) * float64(stats.PAtkSpd)
}

// ArmorScore ranks armor by its physical defense.
func (MysticFighter) ArmorScore(stats npcdata.GearStats) float64 {
	return float64(stats.PDef)
}

// JewelScore ranks jewels by their magical defense.
func (MysticFighter) JewelScore(stats npcdata.GearStats) float64 {
	return float64(stats.MDef)
}

// ShieldScore ranks shields by their expected block value.
func (MysticFighter) ShieldScore(stats npcdata.GearStats) float64 {
	return float64(stats.SDef) * float64(stats.RShld) / 100
}

// WeaponPoints is the zone gating value of a caster weapon: the
// magical damage.
func (MysticFighter) WeaponPoints(stats npcdata.GearStats) int32 {
	return stats.MAtk
}

// scoreStats dispatches the profile scoring on the gear family of the
// stats.
func scoreStats(profile Profile, stats npcdata.GearStats) float64 {
	switch CategoryOf(stats) {
	case CategoryWeapon:
		return profile.WeaponScore(stats)
	case CategoryShield:
		return profile.ShieldScore(stats)
	case CategoryArmor:
		return profile.ArmorScore(stats)
	case CategoryJewel:
		return profile.JewelScore(stats)
	case CategoryUnusable:
		return 0
	default:
		return 0
	}
}

// Score returns the profile value of the inventory item for its gear
// family; zero for items the profile cannot use and items without
// gear stats (consumables, materials, quest items).
func Score(profile Profile, item state.InventoryItem) float64 {
	stats, ok := npcdata.ItemGearStats(item.ItemID)
	if !ok {
		return 0
	}

	return scoreStats(profile, stats)
}

// GearPoints returns the zone gating points of the item: the weapon
// value on the character stat scale plus the defenses. A zero score
// (an unusable item) always returns zero points.
func gearPoints(profile Profile, item state.InventoryItem) int32 {
	stats, ok := npcdata.ItemGearStats(item.ItemID)
	if !ok || scoreStats(profile, stats) <= 0 {
		return 0
	}
	switch CategoryOf(stats) {
	case CategoryWeapon:
		return profile.WeaponPoints(stats)
	case CategoryShield:
		return int32(float64(stats.SDef) * float64(stats.RShld) / 100)
	case CategoryArmor:
		return stats.PDef
	case CategoryJewel:
		return stats.MDef
	case CategoryUnusable:
		return 0
	default:
		return 0
	}
}

// TotalGearPoints summarizes the equipped gear in intuitive points
// for the hunting zone gates: the weapon damage per hit plus the
// defenses of the armor, jewels and shield. A gate reads like "the
// kaboo woods need 120 points: a broadsword (11) and the wooden set
// (108)".
func TotalGearPoints(profile Profile, equipment Equipment) int32 {
	paperdoll := equipment.Paperdoll(profile)
	total := int32(0)
	for slot := Slot(0); slot < slotCount; slot++ {
		entry := paperdoll[slot]
		if entry.Item.ObjectID == 0 {
			continue
		}
		total += gearPoints(profile, entry.Item)
	}

	return total
}
