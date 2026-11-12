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

// enterSellPhase switches into the selling and shopping state at the
// shop. The log names the stop honestly: the first stop sells the
// junk, the buy stops of the frozen plan only trade.
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

// tickTownSell runs the sell stop (the first trip stop) and the buy
// stops. EVERY vendor trip sells the whole accumulated junk - the
// selling ends when nothing sellable is left, not when the inventory
// drops below the trip trigger (a bag of 30 percent junk on a buy
// trip still sells, or the bot would farm with it and walk back for
// the sale later). The sell first step banks the credits the frozen
// trip plan counted on, the stop planning distributes its purchases,
// every buy stop completes when its purchases were requested, and
// the return segment starts when no stop is left.
//
//nolint:cyclop,gocognit // the town sell trip segments
func (l *Loop) tickTownSell() {
    now := l.now()
    if l.teachStop() {
        // The teacher stop: approach the class master, click it and
        // learn the queued lessons (see learning.go). The stop carries
        // no buys and never sells - the junk selling belongs to the
        // first stop of the trip. A teacher that never showed up (or
        // stands out of reach) skips the lessons - the requests
        // resolve their trainer through the last folk npc and cannot
        // run without it.
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
        // The Newbie Guide stop: approach the guide and receive the
        // support magic through the dialog (see guide_buffs.go). The
        // stop carries no buys and never sells; a guide that never
        // showed up or a refusal (the cooldown arms) skips the stop
        // - the trip continues either way.
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
            // The replacement purchases sell their displaced pieces
            // first: the credit the plan counted on must be banked
            // before the buys spend it.
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
            // The learning stops close the trip: the books and
            // the teacher ride BEHIND the gear stops, so one
            // town visit buys the weapon, the armor, the jewels,
            // the books and teaches the lessons (the user rule
            // of the one town visit - the books stop merges with
            // the gear stop of its merchant when they match, see
            // planLearnStops). The weapon stop already ran (the
            // sell stop routes to the weapon merchant), so an
            // abort on the teacher segment never strands a
            // bare-handed character.
            l.planLearnStops()
            // The Newbie Guide support magic closes the trip behind
            // the lessons (see guide_buffs.go): the stop plans once,
            // the refusal cooldown of a refused character keeps the
            // later trips away.
            l.planGuideStop()
        }
    }
    if l.stopBuysPending() {
        if !l.handleMerchant(now, l.stopMerchantTemplates()) {
            return
        }
        // The buys need the selected merchant within the interaction
        // distance: without it the sells still work, the buys are
        // skipped.
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

// handleMerchant approaches the shop merchant and selects it like the
// official client does before a transaction. It reports false while the
// character still walks toward the merchant or waits for one to appear.
// The sale itself works without a merchant (the standard inventory sell
// list), so a merchant that never shows up only delays it; the buys
// need the merchant, their stops skip the purchases instead.
//
// The selected npc must trade what the wanted templates name: the sell
// phase passes every town merchant (the junk sells to any vendor), the
// buy stops pass their own trader - the server resolves the buy
// through the targeted folk npc and silently refuses the list the
// targeted npc does not trade, so a selection left over from the sell
// phase (the nearest vendor, often the armor trader standing next to
// the weapon shop) re-picks here instead of aiming the weapon buy at
// the armor trader.
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
        // The buys cannot run without the selected merchant: skip
        // them instead of waiting forever.
        l.resetStopBuys("merchant never showed up")
    }

    return true
}

// approachMerchant walks to the merchant, selects it inside the
// interaction distance and reports when the sale may start. The
// distance gate is 3D (the server INTERACTION_DISTANCE of 250 checks
// x, y and z together - separate 2D and z limits would let a diagonal
// stand-off slip past 250 and refuse every transaction). The selection
// re-requests itself once per second until the MyTargetSelected
// answer confirms it. A merchant standing on another deck of the
// geodata (the disconnected village decks) hands the close walk to
// the server - the select + attack analog pull ladder of the deck
// window walks the straight line the pack cannot; only when the
// window burns out is the merchant skipped (the sale does not need
// the merchant, the buys of its stop do).
//
// The approach walk clicks the ground at the npc approach point, not
// at the merchant's exact cell: the server's getValidLocation walks a
// Bresenham line that can "step over" onto a roof layer when the
// click targets an interior cell (the 2026-09-11 roof teleport
// report), so the offset keeps the click line on the surrounding deck.
//
// The merchant select fires as soon as the bot is within the server
// interaction distance (npcInteractionDist = 250 in 3D), even when
// the z gap keeps the dist3D above the approach gate (200) - see
// approachTeacher for the same fix and the 2026-09-11 05:45 dump.
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
    // The merchant select fires within the server interaction distance
    // (250 in 3D) even when the approach gate (200) is not met: the
    // offset ring lands the bot at ~150 units 2D from the npc, and a
    // small z gap keeps dist3D above 200 but within 250. Without this
    // early return the bot looped on the offset ring forever (the
    // 2026-09-11 05:45 dump).
    if dist3D <= npcInteractionDist {
        return l.selectMerchant(now)
    }
    if dist3D > merchantApproachDist {
        ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
        approachDist2D := math.Hypot(
            float64(ax-selfX), float64(ay-selfY))
        if dist2D <= merchantApproachDist {
            // The geodata pack misses some village ramps: the character
            // stands under the merchant deck (the 2D distance is met,
            // the z is not). The ground clicks cannot close a z gap
            // (clicking the merchant's exact cell here teleported the
            // bot onto the roof, the 2026-09-11 report), so the deck
            // window bounds the select + attack analog pull ladder
            // that hands the walk to the server before the merchant
            // is given up.
            if l.merchantDeckUntil.IsZero() {
                l.merchantDeckUntil = now.Add(merchantDeckWindow)
                l.logf("Hunt: %s stands on another deck (z %d vs "+
                    "%d), the attack analog pull walks the straight "+
                    "line",
                    l.tracker.ObjectName(l.merchantID), selfZ, z)
            }
            if now.Before(l.merchantDeckUntil) {
                // The deck walk the ground clicks cannot close: the
                // select + attack analog pull ladder hands the walk
                // to the server - the interaction distance is met
                // however the deck geometry sits between them.
                l.merchantDeckPullLadder(now)

                return false
            }
            l.logf("Hunt: %s stays out of reach, the sells work "+
                "without it and its buys are skipped",
                l.tracker.ObjectName(l.merchantID))
            l.merchantID = -1

            return true
        }

        // Far away on the same level: a plain approach walk to the
        // offset point, not the merchant's exact cell. The dist3D <=
        // npcInteractionDist early return above takes over once the bot
        // arrives at the offset ring (the talk click lands from the ring
        // even with a small z gap).
        if approachDist2D > hopCoincideDist {
            l.walkToward(ax, ay, az, now)
        }

        return false
    }
    // dist3D in (merchantApproachDist, npcInteractionDist]: the bot is
    // on the offset ring, the merchant select proceeds.
    return l.selectMerchant(now)
}

// merchantDeckPullLadder runs the select + attack analog pull ladder
// of the merchant deck wait. The plain click selects the npc at any
// distance, the pull click on the selected npc takes the interact
// intention and the server walks the character to the npc along the
// straight line (one click per select period - the player action
// flood protector).
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

// selectMerchant re-requests the merchant selection once per select
// period until the tracker confirms it, then fires the attack analog
// pull once and reports ready. The transactions need the merchant as
// the selected target (RequestBuyItem checks it server side); the
// pull is the practice of every npc interaction - the second plain
// click on the selected merchant (the client double click) hands the
// last stretch to the server: the interact intention walks the
// character to the merchant along the straight line, so the
// transaction distance is met with certainty. Within the interaction
// distance the same click only opens the merchant dialog - harmless,
// the transactions pace past it.
func (l *Loop) selectMerchant(now time.Time) bool {
    if l.tracker.SelfTargetID() != l.merchantID {
        // The transactions need the merchant as the selected target
        // (RequestBuyItem checks it server side): re-request the
        // selection once per second until the tracker confirmed it
        // and only then report ready.
        if now.Sub(l.merchantPick) >= selectPeriod {
            l.merchantPick = now
            if err := l.game.AttackTarget(l.merchantID); err != nil {
                l.logf("Hunt: merchant select failed: %v", err)
            }
        }

        return false
    }
    // The attack analog pull of the interaction practice: once per
    // merchant (the field compares object ids, a re-pick re-arms it).
    if l.merchantPulled != l.merchantID {
        l.merchantPulled = l.merchantID
        if err := l.game.InteractPull(l.merchantID); err != nil {
            l.logf("Hunt: merchant pull failed: %v", err)
        }
    }

    return true
}

// sellableJunk lists the inventory junk of the sell trips without
// the newbie kit and the escape scroll: the starter items are
// unsellable on the server (is_sellable=false - every offer of them
// is silently skipped), so offering them only wastes a transaction
// window of the flood protector once per trip - the destroy flow of
// the replaced starters owns them instead. The Scroll of Escape the
// keep one rule buys never sells back (the 200 adena sale of a
// 460 adena scroll would burn the rule every trip).
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

// junkRemaining reports whether sellable inventory items are left the
// trip has not offered yet: every vendor visit sells the accumulated
// junk completely, batch after batch, whatever started the trip. The
// planned equips of the auto equipment stay out of the junk (a looted
// or bought upgrade waits for its use item request, the sell stop
// must not eat it) and so does the unsellable newbie kit (the destroy
// flow owns it).
func (l *Loop) junkRemaining() bool {
    for _, item := range l.sellableJunk() {
        if !l.sold[item.ObjectID] {
            return true
        }
    }

    return false
}

// sellJunk sells the next batch of inventory junk, most junky items
// first. Every item is offered once per trip: the server silently skips
// what it refuses to sell, so re-offering it forever would stall the
// trip. An empty batch (nothing left to sell) ends the selling.
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
