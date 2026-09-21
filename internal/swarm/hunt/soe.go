// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The Scroll of Escape economy (the owner rule of 2026-09-21): the
// bot always carries at least one scroll (the shopping queue buys it
// the moment the last one is gone), and the trip start uses the
// scroll instead of the long walk home whenever the walking time is
// worth more than the scroll costs at the character's measured farm
// income. The scroll teleports to the nearest village (skill 2013, a
// 20 second stationary cast the server answers with TeleportToLocation
// and the connection layer confirms with Appearing), so it replaces
// the farm spot to village leg only - the shopping stops and the
// return leg run unchanged behind it.

import (
    "math"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

const (
    // soeItemID is the Scroll of Escape display id (the Herbiel
    // grocery buylist 3015000 carries it, reference price 400).
    soeItemID = int32(736)
    // soeCastSeconds is the skill 2013 cast time (hitTime 20000 of
    // the server skill data): the stationary seconds the scroll
    // always costs.
    soeCastSeconds = 20.0
    // soeArrivalWait bounds the escape wait: the cast, the packet
    // round trips and the teleport landing (the UseItem to the self
    // TeleportToLocation window) plus a healthy margin.
    soeArrivalWait = 35 * time.Second
    // soeArrivalPoll is the position check period of the arrival
    // wait.
    soeArrivalPoll = 250 * time.Millisecond
    // soeArrivalDistance is the displacement that marks the landing:
    // the village respawn sits far from every farm spot the trip
    // starts from.
    soeArrivalDistance = 500.0
    // soeWalkFactor widens the straight line distance to the merchant
    // into a walk length estimate (the geodata path bends around the
    // buildings and the coast; 1.4 keeps the estimate honest without
    // overpaying the scroll on a nearly straight road).
    soeWalkFactor = 1.4
    // soeRunSpeedFallback is the run speed used while the server has
    // not told the character speed yet (the elven fighter base run
    // speed of the deployed stack).
    soeRunSpeedFallback = 120.0
)

// soeCount returns the number of scrolls in the inventory.
func (l *Loop) soeCount() int {
    count := 0
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID == soeItemID {
            count += int(item.Count)
        }
    }

    return count
}

// soeObjectID resolves the first scroll stack of the bag (the empty
// answer means the bag holds none).
func (l *Loop) soeObjectID() (int32, bool) {
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID == soeItemID && item.Count > 0 && !item.Equipped {
            return item.ObjectID, true
        }
    }

    return 0, false
}

// currentAdenaPerMin returns the trusted measured income of the held
// ground, zero while no ground is held or the rate has not earned its
// trust horizon yet (an untrusted rate must not promise income the
// ground never paid - the walk would be priced with a guess).
func (h *cellHunter) currentAdenaPerMin() float64 {
    if h.picked < 0 || h.picked >= len(h.metrics) {
        return 0
    }
    metric := &h.metrics[h.picked]
    if !metric.trusted() {
        return 0
    }

    return metric.adenaPerMin()
}

// soeEscapeProfitable prices the walk home against the scroll: the
// walking seconds saved beyond the scroll's own cast, valued at the
// character's measured adena per second, must beat the scroll price
// (the 400 reference plus the town tax the buylist charges).
func (l *Loop) soeEscapeProfitable(
    walkDist float64, adenaPerMin float64,
) bool {
    if adenaPerMin <= 0 {
        return false
    }
    speed := l.tracker.SelfRunSpeed()
    if speed <= 0 {
        speed = soeRunSpeedFallback
    }
    walkSeconds := walkDist * soeWalkFactor / speed
    saved := walkSeconds - soeCastSeconds
    if saved <= 0 {
        return false
    }
    adenaPerSecond := adenaPerMin / 60.0
    price := l.soePrice()

    return saved*adenaPerSecond > float64(price)
}

// soePrice returns the scroll buy price with the active region tax
// (the reference price of the generated item data times the town tax
// rate of the shopping flow).
func (l *Loop) soePrice() int64 {
    price := npcdata.ItemPrice(soeItemID)

    return price + int64(float64(price)*townTaxRate)
}

// escapeWithSOE uses the scroll and waits for the teleport landing:
// the scroll vanishes from the inventory (the skill consume of the
// cast) and the character reappears at the village respawn. It
// reports whether the landing happened - a failed escape leaves the
// trip on the walking path.
func (l *Loop) escapeWithSOE() bool {
    objectID, ok := l.soeObjectID()
    if !ok {
        return false
    }
    startX, startY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    if err := l.game.UseItem(objectID); err != nil {
        l.logf("Hunt: the scroll of escape use failed: %v", err)

        return false
    }
    deadline := time.Now().Add(soeArrivalWait)
    for time.Now().Before(deadline) {
        pace(soeArrivalPoll)
        x, y, _, ok := l.tracker.SelfPosition()
        if ok && math.Hypot(float64(x-startX), float64(y-startY)) >
            soeArrivalDistance {
            l.logf("Hunt: the scroll of escape lands at the village")

            return true
        }
    }
    l.logf("Hunt: the scroll of escape never landed, walking on")

    return false
}

// maybeEscapeWithSOE arms the scroll leg of the trip start: the
// scroll runs only when the bag holds one, the ground pays a trusted
// income and the walk home is worth more than the scroll. The scroll
// replaces the walk only on success - every other path keeps the
// planned walking trip.
func (l *Loop) maybeEscapeWithSOE(merchant townNpc) {
    if l.soeCount() < 1 {
        return
    }
    income := float64(0)
    if l.cell != nil {
        income = l.cell.currentAdenaPerMin()
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    walkDist := math.Hypot(
        float64(merchant.X-selfX), float64(merchant.Y-selfY))
    if !l.soeEscapeProfitable(walkDist, income) {
        return
    }
    if l.escapeWithSOE() {
        l.tracker.RecordEvent("the scroll of escape replaced the walk " +
            "home (" + strconv.FormatInt(l.soePrice(), 10) + " adena)")
    }
}

// soePurchaseLine builds the keep one scroll purchase of the shopping
// queue: the grocery merchant of the active region sells the scroll,
// the line lands only when the bag holds none (the dropOwnedPurchases
// filter would drop a carried line anyway).
func (l *Loop) soePurchaseLine() (gear.Purchase, bool) {
    if l.soeCount() > 0 {
        return gear.Purchase{}, false
    }
    listID, merchantID, ok := soeMerchantOf(l.zoneRegion)
    if !ok {
        return gear.Purchase{}, false
    }

    return gear.Purchase{
        ItemID:             soeItemID,
        ListID:             listID,
        MerchantTemplateID: merchantID,
        Count:              1,
        Price:              l.soePrice(),
        Reason:             "keep one Scroll of Escape",
        Affordable:         true,
    }, true
}

// soeMerchantOf resolves the grocery merchant of the region that
// sells the scroll (the elven Herbiel list 3015000 and the Dion Lara
// list 3006300 carry the item 736; the generated buylists are the
// authority).
func soeMerchantOf(region string) (int32, int32, bool) {
    switch region {
    case regionDion:
        return 3006300, 7063, true
    default:
        return 3015000, 7150, true
    }
}

// keepSOE is the junk sell guard: the scroll stack the economy buys
// never sells back (the 200 adena sale of a 460 adena scroll would
// burn the keep one rule every trip).
func keepSOE(item state.InventoryItem) bool {
    return item.ItemID == soeItemID
}
