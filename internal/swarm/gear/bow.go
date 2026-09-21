// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "sort"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// The ranged luring tool of the melee hunter: a bow plus a quiver of
// arrows. The melee profile scores bows zero (the profile's Weapon
// ladder ranks the close combat damage), so the bow never competes
// with the weapon milestone - the shop strategy plans it in its own
// phase behind the weapon: a melee bot that owns a sword buys the
// next bow rung the wallet affords and keeps its quiver stocked, so
// the hunt loop can pull fenced mobs from range (see hunt/lure.go).
// The mystic profiles never lure: they already cast from range.

// weaponTypeBow is the gear weapon family of the bows.
const weaponTypeBow = "BOW"

// The quiver economics of the luring.
const (
    // arrowRestockFloor is the arrow count the restock triggers at:
    // below it the next town trip tops the quiver back up.
    arrowRestockFloor = 150
    // arrowRestockTarget is the quiver size the restock buys to: one
    // batch lasts a hundred lures (one to three shots each) plus the
    // bow skill casts between the town trips.
    arrowRestockTarget = 600
)

// bowLurer reports whether the profile fights in melee and therefore
// wants the ranged luring tool: the melee fighter profile only (the
// mystic attacks from range already, a custom test profile takes the
// safe default of no bow).
func bowLurer(profile Profile) bool {
    _, ok := profile.(MeleeFighter)

    return ok
}

// BowLurer is the exported form of bowLurer: the hunt loop's junk
// keep set consults it to protect the luring tool from the sell and
// destroy flows (the bow scores zero under the melee profile, so the
// plain planned equips never hold it).
func BowLurer(profile Profile) bool {
    return bowLurer(profile)
}

// ownedBow scans the inventory for the best bow the character owns
// (equipped or bagged) and reports its attack stat. A character
// without any bow answers zero.
func ownedBow(equipment Equipment) int32 {
    var best int32
    for _, item := range equipment.Items {
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        if !ok || stats.WeaponType != weaponTypeBow {
            continue
        }
        if stats.PAtk > best {
            best = stats.PAtk
        }
    }

    return best
}

// bowOffers resolves the bows the catalog sells with their cheapest
// offers: the highest attack first. The bows score zero under the
// melee profile, so the generic candidate build never lists them -
// this scan reads the buylists directly.
func bowOffers(catalog Catalog) []purchaseCandidate {
    type offer struct {
        listID   int32
        merchant int32
        price    int64
    }
    offers := make(map[int32]offer)
    for _, shop := range catalog.Shops {
        if shop.TaxRate < 0 || shop.TaxRate > shopTaxLimit {
            continue
        }
        for _, listID := range shop.Lists {
            for _, itemID := range npcdata.ItemsOfBuyList(listID) {
                stats, ok := npcdata.ItemGearStats(itemID)
                if !ok || stats.WeaponType != weaponTypeBow {
                    continue
                }
                price := int64(float64(npcdata.ItemPrice(itemID)) *
                    (1 + shop.TaxRate))
                current, seen := offers[itemID]
                if !seen || price < current.price ||
                    (price == current.price && listID < current.listID) {
                    offers[itemID] = offer{
                        listID:   listID,
                        merchant: shop.MerchantTemplateID,
                        price:    price,
                    }
                }
            }
        }
    }
    bows := make([]purchaseCandidate, 0, len(offers))
    for itemID, offer := range offers {
        stats, _ := npcdata.ItemGearStats(itemID)
        bows = append(bows, purchaseCandidate{
            itemID:   itemID,
            listID:   offer.listID,
            merchant: offer.merchant,
            price:    offer.price,
            stats:    stats,
            score:    float64(stats.PAtk),
        })
    }
    sort.Slice(bows, func(i int, j int) bool {
        if bows[i].score != bows[j].score {
            return bows[i].score > bows[j].score
        }

        return bows[i].itemID < bows[j].itemID
    })

    return bows
}

// arrowItemID resolves the ammo the catalog sells: the cheapest arrow
// item of the buylists (the quiver standard of the deployment - the
// Wooden Arrow of the elven shops). Zero when the merchants sell no
// arrows at all.
func arrowItemID(catalog Catalog) int32 {
    type offer struct {
        listID   int32
        merchant int32
        price    int64
    }
    best := int32(0)
    var bestOffer offer
    for _, shop := range catalog.Shops {
        if shop.TaxRate < 0 || shop.TaxRate > shopTaxLimit {
            continue
        }
        for _, listID := range shop.Lists {
            for _, itemID := range npcdata.ItemsOfBuyList(listID) {
                stats, gear := npcdata.ItemGearStats(itemID)
                if !gear || stats.Type != "EtcItem" ||
                    stats.BodyPart != "lhand" || stats.WeaponType != "" {
                    // The ammo discriminator: an etc item that rides
                    // the left hand is a quiver item (the Mobius
                    // arrow xml: etcitem_type ARROW, bodypart lhand).
                    continue
                }
                price := int64(float64(npcdata.ItemPrice(itemID)) *
                    (1 + shop.TaxRate))
                if best == 0 || price < bestOffer.price ||
                    (price == bestOffer.price && itemID < best) {
                    best = itemID
                    bestOffer = offer{
                        listID:   listID,
                        merchant: shop.MerchantTemplateID,
                        price:    price,
                    }
                }
            }
        }
    }

    return best
}

// arrowOffer resolves the cheapest catalog offer of the given arrow
// item: the list and the merchant of the restock purchase.
func arrowOffer(
    catalog Catalog, itemID int32,
) (int32, int32, int64, bool) {
    bestList, bestMerchant, bestPrice := int32(0), int32(0), int64(0)
    found := false
    for _, shop := range catalog.Shops {
        if shop.TaxRate < 0 || shop.TaxRate > shopTaxLimit {
            continue
        }
        for _, listID := range shop.Lists {
            for _, candidate := range npcdata.ItemsOfBuyList(listID) {
                if candidate != itemID {
                    continue
                }
                price := int64(float64(npcdata.ItemPrice(itemID)) *
                    (1 + shop.TaxRate))
                if !found || price < bestPrice ||
                    (price == bestPrice && listID < bestList) {
                    bestList, bestMerchant, bestPrice, found = listID,
                        shop.MerchantTemplateID, price, true
                }
            }
        }
    }

    return bestList, bestMerchant, bestPrice, found
}

// ownedArrowCount sums the arrow stacks of the inventory (the
// equipped quiver included).
func ownedArrowCount(equipment Equipment, itemID int32) int32 {
    var count int32
    for _, item := range equipment.Items {
        if item.ItemID == itemID {
            count += item.Count
        }
    }

    return count
}

// bowPhase plans the ranged luring tool of the melee hunter: the best
// affordable bow strictly stronger than the owned one (the gradual
// upgrade - the wallet buys the next rung once the weapon milestone
// spent its share), then the arrow restock when the quiver runs low.
// The phase runs behind the weapon milestone: the virtual paperdoll
// carries the worn or the just planned melee weapon, a bare-handed
// hunter spends the wallet on the sword before the luring tool (the
// weapon run flow of the hunt loop).
func (w *planWalk) bowPhase() {
    if w.tailDone() || !bowLurer(w.profile) {
        return
    }
    if w.virtual[SlotRHand].Score <= 0 {
        // No melee weapon worn or planned: the weapon milestone owns
        // the wallet first, the tool follows on the next trip.
        return
    }
    w.planBowUpgrade()
    if w.tailDone() {
        return
    }
    w.planArrowRestock()
}

// planBowUpgrade emits the best affordable bow strictly stronger than
// the owned one: one bow per trip.
func (w *planWalk) planBowUpgrade() {
    owned := ownedBow(w.equipment)
    // The bows this walk already planned count as owned: the tail
    // walk continues the phases past the wallet and must not
    // re-purchase the rung the affordable walk just bought.
    for _, purchase := range w.purchases {
        stats, ok := npcdata.ItemGearStats(purchase.ItemID)
        if ok && stats.WeaponType == "BOW" && stats.PAtk > owned {
            owned = stats.PAtk
        }
    }
    bows := bowOffers(w.catalog)
    for _, bow := range bows {
        if bow.stats.PAtk <= owned || w.planned[bow.itemID] > 0 {
            continue
        }
        if bow.price > w.budget {
            continue
        }
        gain := float64(bow.stats.PAtk - owned)
        w.planned[bow.itemID]++
        w.budget -= bow.price
        w.spent += bow.price
        missing := w.spent - w.adena - w.credited
        if missing < 0 {
            missing = 0
        }
        //nolint:exhaustruct_v5 // no displaced gear on the tool
        w.purchases = append(w.purchases, Purchase{
            ItemID:             bow.itemID,
            ListID:             bow.listID,
            MerchantTemplateID: bow.merchant,
            Count:              1,
            Price:              bow.price,
            Reason:             "buying the luring bow " + bow.describe(gain),
            Gain:               gain,
            Affordable:         w.affordable,
            Missing:            missing,
        })
        if !w.affordable {
            w.tailLeft--
        }

        return
    }
}

// planArrowRestock emits the quiver top-up: the arrow batch that
// refills the inventory to the restock target when the owned count
// fell under the floor.
func (w *planWalk) planArrowRestock() {
    itemID := arrowItemID(w.catalog)
    if itemID == 0 {
        return
    }
    owned := ownedArrowCount(w.equipment, itemID)
    if owned >= arrowRestockFloor {
        return
    }
    listID, merchant, unit, ok := arrowOffer(w.catalog, itemID)
    if !ok {
        return
    }
    count := arrowRestockTarget - owned
    price := unit * int64(count)
    if price > w.budget {
        // The affordable part of the batch still flies: a partial
        // restock beats an empty quiver.
        count = int32(w.budget / unit)
        if count <= 0 {
            return
        }
        price = unit * int64(count)
    }
    w.planned[itemID]++
    w.budget -= price
    w.spent += price
    missing := w.spent - w.adena - w.credited
    if missing < 0 {
        missing = 0
    }
    //nolint:exhaustruct_v5 // the sell paths never touch the ammo
    w.purchases = append(w.purchases, Purchase{
        ItemID:             itemID,
        ListID:             listID,
        MerchantTemplateID: merchant,
        Count:              count,
        OwnedStack:         owned,
        Price:              price,
        Reason:             "restocking the quiver with arrows",
        Affordable:         w.affordable,
        Missing:            missing,
    })
    if !w.affordable {
        w.tailLeft--
    }
}
