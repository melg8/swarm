// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
)

// enterSellPhase switches into the selling and shopping state at the shop;
// the log names the stop honestly (the first sells, the buy stops trade).
func (l *Loop) enterSellPhase() {
    l.phase = phaseTownSell
    l.sellPhaseAt = l.now()
    l.sellAt = time.Time{}
    l.buyAt = time.Time{}
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
    if l.sellableStop() {
        l.logf("Hunt: shop reached, selling the junk")

        return
    }
    if len(l.tripStops) > 0 {
        l.logf("Hunt: shop reached at %s",
            l.tripStops[0].merchant.Name)

        return
    }
    l.logf("Hunt: shop reached")
}

// tickTownSell runs the sell stop (the first trip stop) and the buy stops.
// Every vendor trip sells the whole accumulated junk - the selling ends when
// nothing sellable is left, not at the trip trigger, or a bot would farm with
// 30 percent junk and walk back for the sale. The sell banks the credits the
// plan counted on, the stop planning distributes the purchases, and the
// return segment starts when no stop is left.
//
//nolint:cyclop,gocognit // the town sell trip segments
func (l *Loop) tickTownSell() {
    now := l.now()
    if l.teachStop() {
        // The teacher stop approaches the class master and learns the queued
        // lessons (learning.go); it carries no buys and never sells. A
        // teacher that never shows up skips the lessons - the requests
        // resolve their trainer through the last folk npc.
        if !l.handleTeacher(now) {
            return
        }
        if l.teacherID > 0 && !l.tickTeacherLessons(now) {
            return
        }
        l.advanceTripStop()

        return
    }
    if l.guideStop() {
        // The Newbie Guide stop receives the support magic through the
        // dialog (guide_buffs.go); no buys, no sells, and a missing guide or
        // a refusal (the cooldown arms) skips the stop.
        if !l.handleGuideStop(now) {
            return
        }
        l.advanceTripStop()

        return
    }
    if l.sellableStop() {
        if l.junkRemaining() {
            if !l.handleMerchant(now, l.merchantTemplates()) {
                return
            }
            l.sellJunk()

            return
        }
        if !l.replaceDone {
            // The displaced pieces sell first: the credit the plan counted
            // on must be banked before the buys spend it.
            if !l.stepReplacementSales(now) {
                return
            }
            l.replaceDone = true
        }
        if !l.buysPlanned {
            stats := l.tracker.InventoryStats()
            l.logf("Hunt: shop: junk sold (%d slots left, "+
                "%.0f%% weight), distributing the trip plan",
                stats.Slots, stats.WeightPercent)
            l.planShoppingStops()
            // The learning stops close the trip: the books and the teacher ride
            // behind the gear stops, so one town visit buys everything (the one
            // town visit rule; planLearnStops merges a matching book stop into
            // the gear stop of its merchant). The weapon stop ran first, so an
            // abort on the teacher never strands a bare-handed character.
            l.planLearnStops()
            // The Newbie Guide support magic closes the trip behind the
            // lessons (guide_buffs.go); a refused character keeps the later
            // trips away through the cooldown.
            l.planGuideStop()
        }
    }
    if l.stopBuysPending() {
        if !l.handleMerchant(now, l.stopMerchantTemplates()) {
            return
        }
        // The buys need the selected merchant in reach; without it the sells
        // still work and the buys are skipped.
        if l.merchantID > 0 {
            l.tickStopShopping(now)
        } else {
            l.resetStopBuys("no merchant in reach for the buys")
        }

        return
    }
    if len(l.tripStops) > 1 || (len(l.tripStops) == 1 &&
        !l.tripStops[0].sell && !l.stopBuysPending()) ||
        l.buysPlanned {
        l.advanceTripStop()

        return
    }
    l.startReturnSegment()
}

// handleMerchant approaches the shop merchant and selects it like the official
// client before a transaction, reporting false while the character walks or
// waits for one to appear. The sale works without a merchant (the inventory
// sell list), so a no-show only delays it; the buys need one and skip instead.
// The selected npc must trade what the wanted templates name: the sell passes
// every town merchant, the buy stops their own trader, and a selection left
// over from the sell phase re-picks here instead of aiming the buy at the
// armor trader next door (the server silently refuses a foreign list).
func (l *Loop) handleMerchant(now time.Time, templates []int32) bool {
    if l.merchantID < 0 {
        return true
    }
    if l.merchantID > 0 {
        if merchantTemplateWanted(
            l.tracker.ObjectTemplateID(l.merchantID), templates) {
            return l.approachMerchant(now)
        }
        l.logf("Hunt: shop: %s does not sell this stop's goods, "+
            "re-picking the merchant", l.tracker.ObjectName(l.merchantID))
        l.merchantID = 0
    }
    if now.Sub(l.merchantPick) < selectPeriod {
        return false
    }
    l.merchantPick = now
    l.merchantDeckUntil = time.Time{}
    merchant, ok := l.tracker.NearestNpcByTemplates(
        templates, merchantFindRadius)
    if ok {
        l.merchantID = merchant.ObjectID
        l.logf("Hunt: trading with %s", merchant.Name)

        return false
    }
    if now.Sub(l.sellPhaseAt) < merchantWaitTimeout {
        return false
    }
    l.merchantID = -1
    if l.stopBuysPending() {
        // The buys cannot run without the selected merchant: skip them.
        l.resetStopBuys("merchant never showed up")
    }

    return true
}

// approachMerchant walks to the merchant, selects it inside the interaction
// distance and reports when the sale may start. The gate is 3D: the server
// INTERACTION_DISTANCE of 250 checks x, y and z together, so separate 2D and
// z limits would let a diagonal stand-off slip past. The selection re-requests
// once per second until MyTargetSelected confirms it. A merchant on another
// deck hands the close walk to the select + pull ladder of the bounded deck
// window, then the stop gives up. The walk clicks the npc approach point,
// never the merchant cell (the roof layer step-over, docs/hunting.md).
func (l *Loop) approachMerchant(now time.Time) bool {
    x, y, z, ok := l.tracker.ObjectPosition(l.merchantID)
    if !ok {
        l.merchantID = -1

        return true
    }
    selfX, selfY, selfZ, _ := l.tracker.SelfPosition()
    dist2D := math.Hypot(float64(x-selfX), float64(y-selfY))
    dz := float64(z - selfZ)
    dist3D := math.Sqrt(dist2D*dist2D + dz*dz)
    // The select fires within the 250 interaction distance even when the 200
    // approach gate is not met: the offset ring lands ~150 units 2D out and a
    // small z gap keeps dist3D above 200. Without this early return the bot
    // looped on the ring forever (the 05:45 dump).
    if dist3D <= npcInteractionDist {
        return l.selectMerchant(now)
    }
    if dist3D > merchantApproachDist {
        ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
        approachDist2D := math.Hypot(
            float64(ax-selfX), float64(ay-selfY))
        if dist2D <= merchantApproachDist {
            // The pack misses some village ramps: the character stands under
            // the merchant deck with the 2D distance met. Ground clicks cannot
            // close a z gap (they teleported the bot onto the roof), so the
            // deck window hands the walk to the select + pull ladder.
            if l.merchantDeckUntil.IsZero() {
                l.merchantDeckUntil = now.Add(merchantDeckWindow)
                l.logf("Hunt: %s stands on another deck (z %d vs "+
                    "%d), the attack analog pull walks the straight "+
                    "line",
                    l.tracker.ObjectName(l.merchantID), selfZ, z)
            }
            if now.Before(l.merchantDeckUntil) {
                // The select + pull ladder hands the walk to the server: the
                // interaction distance is met whatever the deck geometry.
                l.merchantDeckPullLadder(now)

                return false
            }
            l.logf("Hunt: %s stays out of reach, the sells work "+
                "without it and its buys are skipped",
                l.tracker.ObjectName(l.merchantID))
            l.merchantID = -1

            return true
        }

        // Far away on the same level: walk to the offset point, not the
        // merchant cell; the interaction early return takes over at the ring
        // even with a small z gap.
        if approachDist2D > hopCoincideDist {
            l.walkToward(ax, ay, az, now)
        }

        return false
    }
    // The bot stands on the offset ring with the select distance met.
    return l.selectMerchant(now)
}

// merchantDeckPullLadder runs the select + pull ladder of the merchant deck
// wait: the plain click selects, the pull on the selected npc takes the
// interact intention and the server walks the straight line (one click per
// select period, the flood protector).
func (l *Loop) merchantDeckPullLadder(now time.Time) {
    if now.Sub(l.merchantPull) < selectPeriod {
        return
    }
    l.merchantPull = now
    var err error
    if l.tracker.SelfTargetID() != l.merchantID {
        err = l.game.ClickObject(l.merchantID)
    } else {
        err = l.game.InteractPull(l.merchantID)
    }
    if err != nil {
        l.logf("Hunt: merchant pull failed: %v", err)
    }
}

// selectMerchant re-requests the selection once per select period until the
// tracker confirms it, then fires the attack analog pull once and reports
// ready. The transactions need the merchant as the selected target
// (RequestBuyItem checks it server side); the pull is the second plain click
// of the client double click - the interact intention walks the last stretch
// straight, so the transaction distance is met with certainty (in reach the
// click only opens the dialog, and the transactions pace past it).
func (l *Loop) selectMerchant(now time.Time) bool {
    if l.tracker.SelfTargetID() != l.merchantID {
        // RequestBuyItem checks the selected target server side: re-request
        // the selection until the tracker confirms it, then report ready.
        if now.Sub(l.merchantPick) >= selectPeriod {
            l.merchantPick = now
            if err := l.game.AttackTarget(l.merchantID); err != nil {
                l.logf("Hunt: merchant select failed: %v", err)
            }
        }

        return false
    }
    // The pull runs once per merchant (a re-pick re-arms the object id).
    if l.merchantPulled != l.merchantID {
        l.merchantPulled = l.merchantID
        if err := l.game.InteractPull(l.merchantID); err != nil {
            l.logf("Hunt: merchant pull failed: %v", err)
        }
    }

    return true
}

// sellableJunk lists the inventory junk without the newbie kit and the escape
// scroll: the starter items are unsellable server side (is_sellable=false),
// so offering them wastes a flood protector window - the destroy flow of the
// replaced starters owns them. The Scroll of Escape a keep one rule buys never
// sells back (the 200 adena sale of a 460 adena scroll would burn the rule).
func (l *Loop) sellableJunk() []state.InventoryItem {
    junk := make([]state.InventoryItem, 0, 8)
    for _, entry := range l.tracker.SellableItemsExcluding(
        l.plannedEquipKeeps()) {
        if gear.IsStarterItem(entry.ItemID) || keepSOE(entry) {
            continue
        }
        junk = append(junk, entry)
    }

    return junk
}

// junkRemaining reports whether sellable items are left the trip has not
// offered: every vendor visit sells the junk completely, batch after batch.
// The planned equips stay out (a looted or bought upgrade waits for its use
// item request) and so does the unsellable newbie kit.
func (l *Loop) junkRemaining() bool {
    for _, item := range l.sellableJunk() {
        if !l.sold[item.ObjectID] {
            return true
        }
    }

    return false
}

// sellJunk sells the next batch of junk, most junky items first. Every item
// is offered once per trip: the server silently skips a refused item, so
// re-offering it would stall the trip. An empty batch ends the selling.
func (l *Loop) sellJunk() {
    now := l.now()
    if !l.transactionWindowFree(now) {
        return
    }
    l.sellAt = now
    junk := l.sellableJunk()
    batch := make([]state.InventoryItem, 0, sellBatchSize)
    for _, item := range junk {
        if l.sold[item.ObjectID] {
            continue
        }
        batch = append(batch, item)
        if len(batch) >= sellBatchSize {
            break
        }
    }
    if len(batch) == 0 {
        l.startReturnSegment()

        return
    }
    if err := l.game.SellItems(batch); err != nil {
        l.logf("Hunt: sell request failed: %v", err)

        return
    }
    for _, item := range batch {
        l.sold[item.ObjectID] = true
    }
    if l.journal != nil {
        l.journal.Sell(l.tracker.ID(), len(batch))
    }
    l.logf("Hunt: offered %d items for sale", len(batch))
}
