// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The healing potion economy: a running fight drinks a potion the moment the
// health drops under the drink line, and the town shopping queue keeps the
// stock above the floor. The 60 potion drinks of the 51 hour run7 against zero
// bought potions are the bug this file closes: the plan never ordered one, so
// the fight gate found an empty bag and the flee budget ran out instead.

import (
    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

const (
    // lesserHealingPotionID is the 1060 potion (heal over time 8 x 3, reuse
    // 10 s), sold by the elven Herbiel and the Dion Lara.
    lesserHealingPotionID = int32(1060)
    // healingPotionID is the 1061 potion (heal over time 24 x 3, reuse 10 s),
    // sold by the Dion Lara only.
    healingPotionID = int32(1061)
    // potionRestockFloor is the carried count below which the shopping queue
    // plans a restock: the last potions stay for the fight that spends them.
    potionRestockFloor = int32(8)
    // potionRestockTarget is the carried count a restock buys to: 40 potions
    // cover hours of fights at the 10 s reuse window.
    potionRestockTarget = int32(40)
)

// potionIDs is the preference order of the healing potions: the stronger 1061
// first where the region merchant sells it.
var potionIDs = [...]int32{healingPotionID, lesserHealingPotionID}

// isHealingPotion reports whether the item id is a healing potion of the
// economy.
func isHealingPotion(itemID int32) bool {
    return itemID == lesserHealingPotionID || itemID == healingPotionID
}

// potionCount sums the carried healing potion stacks.
func (l *Loop) potionCount() int32 {
    count := int32(0)
    for _, item := range l.tracker.InventoryItems() {
        if isHealingPotion(item.ItemID) {
            count += item.Count
        }
    }

    return count
}

// healingPotionObject resolves the strongest carried potion stack, the empty
// answer meaning none.
func (l *Loop) healingPotionObject() (int32, bool) {
    for _, potionID := range potionIDs {
        for _, item := range l.tracker.InventoryItems() {
            if item.ItemID == potionID && item.Count > 0 && !item.Equipped {
                return item.ObjectID, true
            }
        }
    }

    return 0, false
}

// potionMerchantOf resolves the potion line of the region: the Dion Lara list
// 3006300 (1061 and 1060) and the elven Herbiel list 3015000 (1060 only).
func potionMerchantOf(region string) (
    listID int32, merchantID int32, itemID int32,
) {
    if region == regionDion {
        return 3006300, 7063, healingPotionID
    }

    return 3015000, 7150, lesserHealingPotionID
}

// potionUnitPrice prices one potion of the region with its town tax.
func potionUnitPrice(region string, itemID int32) int64 {
    price := npcdata.ItemPrice(itemID)
    tax := townTaxRate
    if region == regionDion {
        tax = dionTownTaxRate
    }

    return price + int64(float64(price)*tax)
}

// potionPurchaseLine plans the restock order against the adena the affordable
// gear plan leaves. Below the floor it buys the strongest potion the region
// sells up to the target count, capped by the wallet; a wallet that cannot
// cover one potion plans nothing so the gear lines still run.
func (l *Loop) potionPurchaseLine(adena int64) (gear.Purchase, bool) {
    var zero gear.Purchase
    carried := l.potionCount()
    if carried >= potionRestockFloor {
        return zero, false
    }
    listID, merchantID, itemID := potionMerchantOf(l.zoneRegion)
    unit := potionUnitPrice(l.zoneRegion, itemID)
    if unit <= 0 || adena < unit {
        return zero, false
    }
    count := potionRestockTarget - carried
    if capped := adena / unit; int64(count) > capped {
        count = int32(capped)
    }
    if count <= 0 {
        return zero, false
    }

    return gear.Purchase{
        ItemID:             itemID,
        ListID:             listID,
        MerchantTemplateID: merchantID,
        Count:              count,
        OwnedStack:         carried,
        Price:              unit * int64(count),
        Reason: "keep " + npcdata.ItemName(itemID) +
            " for the fights",
        SellFirst:  nil,
        SellCredit: 0,
        Gain:       0,
        Affordable: true,
        Missing:    0,
    }, true
}

// keepPotion is the junk sell guard: the healing stock never sells back, the
// sale of a 90 adena potion would undo a whole restock line.
func keepPotion(item state.InventoryItem) bool {
    return isHealingPotion(item.ItemID)
}
