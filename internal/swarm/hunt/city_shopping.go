// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "context"
    "errors"
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The city shopping errand: a character that already owns a village
// dress and has saved enough adena walks to the Elven Village
// gatekeeper, teleports to Gludio, sells the village weapon and armor
// the trip replaces, buys the top no-grade weapon and armor set at
// the town shops, carries the top no-grade bow when the wallet covers
// it, rides back to the Elven Village and resumes the hunt on its own
// ground. The live bots run it whenever the wallet covers the missing
// kit; the ordinary town trip keeps the rest of the shopping. The
// policy and the gates: docs/hunting.md.
//
// The route and the stock are live Mobius C1 facts: Mirabel of the
// Elven Village teleports to Gludio (the only direct link, 9 200
// adena - free below MaxFreeTeleportLevel 40), Bella of Gludio
// teleports back to the Elven Village, Sydnia (30321, display 7321)
// sells the Falchion 68 and the Composition Bow 273 on list 3032100
// and Simplon (30253, display 7253) sells the whole no-grade armor
// set, the shield and the boots on list 3025300. The 20 percent
// Gludio buy tax (the MerchantPriceConfig of the castle) prices the
// kit at 455 760 adena (292 800 for the weapon and 162 960 for the
// armor) and the bow at another 292 800.

// CityShoppingErrand is the name the errand publishes in the tracker
// errand status: the acceptance watch compares against it to tell the
// errand it follows from a later one.
const CityShoppingErrand = "city-shopping"

// The item ids of the top no-grade kit the errand buys.
const (
    // cityShopWeaponID is the Falchion of Sydnia.
    cityShopWeaponID = int32(68)
    // cityShopChestID is the Bronze Breastplate of Simplon.
    cityShopChestID = int32(26)
    // cityShopLegsID is the Bronze Gaiters of Simplon.
    cityShopLegsID = int32(34)
    // cityShopHeadID is the Hard Leather Helmet of Simplon.
    cityShopHeadID = int32(1148)
    // cityShopGlovesID is the Bracer of Simplon.
    cityShopGlovesID = int32(51)
    // cityShopFeetID is the Boots of Simplon.
    cityShopFeetID = int32(39)
    // cityShopShieldID is the Bone Shield of Simplon.
    cityShopShieldID = int32(625)
    // cityShopBowID is the Composition Bow of Sydnia: the top
    // no-grade bow (64 pAtk, no crystal grade). The errand carries it
    // beside the melee dress once the wallet covers the whole plan.
    cityShopBowID = int32(273)
)

// cityShopTaxPercent is the Gludio buy tax markup of the merchant
// price config, in percent.
const cityShopTaxPercent = int64(20)

// The pacing and bounds of the errand.
const (
    // cityShopPoll is the poll period of the blocking buy loop: the
    // stop shopping decisions pace themselves on the transaction
    // window, the poll only re-reads the tracker.
    cityShopPoll = 500 * time.Millisecond
    // cityShopStopTimeout bounds one merchant stop: the selection
    // ladder, the buy confirmations and their retries.
    cityShopStopTimeout = 4 * time.Minute
    // cityShopCooldown spaces two errands of one session: an errand
    // that failed (a refused buy, a lost session) retries after it.
    cityShopCooldown = 10 * time.Minute
    // cityShopHomeRadius is the arrival distance of the walk home:
    // the errand ends when the character stands this close to the
    // spot it started from.
    cityShopHomeRadius = 300.0
    // cityShopPendingWait bounds the wait for the in flight equip
    // requests of the tick before the blocking dress re-plans: a
    // second use request for an equipped item toggles it right back
    // off.
    cityShopPendingWait = 5 * time.Second
    // cityShopBudget bounds one whole errand: the walk to the
    // gatekeeper, both rides, the two merchant stops, the dress and
    // the walk back. The far elven cells sit up to 53 000 units from
    // the village, so the two walking segments can each run near the
    // ten minute quest walk timeout; the scroll leg cheapens the
    // outbound one whenever the ground income justifies it (see
    // maybeEscapeTo). The budget only bounds a hung errand - every
    // stage keeps its own shorter timeout.
    cityShopBudget = 30 * time.Minute
)

// cityShoppingOutbound is the Elven Village to Gludio hop: the
// teleporter Mirabel and the Gludio square the list button lands on.
var cityShoppingOutbound = QuestTransfer{
    Gatekeeper: QuestNpc{
        TemplateID: 7146,
        Name:       "Mirabel",
        X:          46926,
        Y:          51511,
        Z:          -2976,
    },
    DestLabel: "The Town of Gludio",
    ArriveX:   -12694,
    ArriveY:   122776,
    ArriveZ:   -3114,
}

// cityShoppingHomebound is the Gludio to Elven Village hop: the
// teleporter Bella and the village square the list button lands on.
var cityShoppingHomebound = QuestTransfer{
    Gatekeeper: QuestNpc{
        TemplateID: 7256,
        Name:       "Bella",
        X:          -12736,
        Y:          122816,
        Z:          -3114,
    },
    DestLabel: "Elven Village",
    ArriveX:   46890,
    ArriveY:   51531,
    ArriveZ:   -2976,
}

// cityShopStop is one merchant visit of the errand: the town npc, the
// buylist the whole stop rides (one transaction request) and the item
// ids the stop buys.
type cityShopStop struct {
    merchant townNpc
    listID   int32
    itemIDs  []int32
}

// cityShoppingWeaponStop is the Sydnia stop: the melee weapon
// milestone the errand runs first, so the old weapon sells and the
// Falchion lands before any armor walk.
var cityShoppingWeaponStop = cityShopStop{
    merchant: townNpc{
        TemplateID: 7321,
        Name:       "Sydnia",
        X:          -15238,
        Y:          123282,
        Z:          -3107,
    },
    listID:  3032100,
    itemIDs: []int32{cityShopWeaponID},
}

// cityShoppingArmorStop is the Simplon stop: the whole no-grade armor
// set and the shield in one buy transaction of list 3025300.
var cityShoppingArmorStop = cityShopStop{
    merchant: townNpc{
        TemplateID: 7253,
        Name:       "Simplon",
        X:          -15132,
        Y:          123282,
        Z:          -3107,
    },
    listID: 3025300,
    itemIDs: []int32{
        cityShopChestID,
        cityShopLegsID,
        cityShopHeadID,
        cityShopGlovesID,
        cityShopFeetID,
        cityShopShieldID,
    },
}

// cityShoppingStops is the fixed melee kit plan of the errand, the
// weapon first. The planner of the ordinary town trip is not used:
// it scores by value per adena and would pick D-grade pieces or
// jewels, while the errand buys exactly the top no-grade set.
var cityShoppingStops = []cityShopStop{
    cityShoppingWeaponStop,
    cityShoppingArmorStop,
}

// cityShopBowStop is the bow visit of the errand: the same Sydnia
// counter and weapon list as the melee weapon, walked after the kit
// stops so the other kit pieces are in the bag first. The purchase
// keeps the melee dress on (the two hand tool would drop the
// Falchion and the shield), so the stop displaces nothing.
var cityShopBowStop = cityShopStop{
    merchant: cityShoppingWeaponStop.merchant,
    listID:   cityShoppingWeaponStop.listID,
    itemIDs:  []int32{cityShopBowID},
}

// CityShoppingKit returns the item ids of the top no-grade melee kit
// in purchase order (the weapon first): the acceptance scenario
// verifies the bought and worn set against it.
func CityShoppingKit() []int32 {
    kit := make([]int32, 0, 7)
    for _, stop := range cityShoppingStops {
        kit = append(kit, stop.itemIDs...)
    }

    return kit
}

// CityShoppingBowID returns the top no-grade bow the errand buys
// behind the kit when the wallet covers it: the acceptance scenario
// verifies the bagged tool against it.
func CityShoppingBowID() int32 {
    return cityShopBowID
}

// cityShoppingProfileAccepts reports whether the gear profile fights
// with the fixed plan: the Falchion is a melee sword and the kit ends
// in a shield, so only the melee fighter buys it. The caster profile
// scores the C1 sword by its magic attack, so the score alone does
// not separate the classes.
func (l *Loop) cityShoppingProfileAccepts() bool {
    if l.equip == nil {
        return false
    }
    _, melee := l.equip.profile.(gear.MeleeFighter)

    return melee
}

// cityShopPrice is the Gludio counter price of one kit piece: the
// reference price of the item table plus the town buy tax. The server
// prices the request itself; the errand only budgets with it.
func cityShopPrice(itemID int32) int64 {
    price := npcdata.ItemPrice(itemID)

    return price + price*cityShopTaxPercent/100
}

// cityShoppingMissingCost sums the Gludio prices of the kit pieces the
// character does not carry yet (worn or bagged, both count).
func (l *Loop) cityShoppingMissingCost() int64 {
    cost := int64(0)
    for _, stop := range cityShoppingStops {
        for _, itemID := range stop.itemIDs {
            if l.tracker.InventoryHasItem(itemID) {
                continue
            }
            cost += cityShopPrice(itemID)
        }
    }

    return cost
}

// cityShoppingBowUpgrade reports whether the bow joins the errand: the
// melee lurer climbs from a tool weaker than the Composition Bow, so a
// character that already owns the bow (or a stronger one) never buys a
// second copy.
func (l *Loop) cityShoppingBowUpgrade() bool {
    stats, ok := npcdata.ItemGearStats(cityShopBowID)
    if !ok {
        return false
    }

    return l.ownedBowAttack() < stats.PAtk
}

// cityShoppingPlan resolves what one errand would buy now: the Gludio
// cost of the kit pieces the character still misses, plus the bow when
// the wallet covers the whole plan beside it. An unaffordable plan
// answers zero, so the gate reads a single number.
func (l *Loop) cityShoppingPlan() (int64, bool) {
    cost := l.cityShoppingMissingCost()
    adena := int64(l.tracker.InventoryStats().Adena)
    if adena < cost {
        return 0, false
    }
    bowCost := cityShopPrice(cityShopBowID)
    if adena >= cost+bowCost && l.cityShoppingBowUpgrade() {
        return cost + bowCost, true
    }

    return cost, false
}

// cityShoppingPlanStops builds the merchant route of one errand: the
// kit stops carry only the pieces the inventory still misses (an owned
// line would be dropped at buy time, and an empty stop costs a walk),
// and the bow stop rides last when the plan covers it.
func (l *Loop) cityShoppingPlanStops(withBow bool) []cityShopStop {
    stops := make([]cityShopStop, 0, len(cityShoppingStops)+1)
    for _, stop := range cityShoppingStops {
        items := make([]int32, 0, len(stop.itemIDs))
        for _, itemID := range stop.itemIDs {
            if l.tracker.InventoryHasItem(itemID) {
                continue
            }
            items = append(items, itemID)
        }
        if len(items) == 0 {
            continue
        }
        stops = append(stops, cityShopStop{
            merchant: stop.merchant,
            listID:   stop.listID,
            itemIDs:  items,
        })
    }
    if withBow {
        stops = append(stops, cityShopBowStop)
    }

    return stops
}

// cityShoppingArmed reports whether the character already owns a
// village weapon: the errand upgrades a dressed character, a
// bare-handed one keeps the ordinary weapon run.
func (l *Loop) cityShoppingArmed() bool {
    doll := l.tracker.PaperdollSlotObjectIDs()
    if doll[state.PaperdollRHand] != 0 {
        return true
    }
    for _, item := range l.tracker.InventoryItems() {
        if !item.Equipped && npcdata.ItemType(item.ItemID) == itemTypeWeapon {
            return true
        }
    }

    return false
}

// cityShoppingWanted reports whether the errand may start now: the
// character hunts the elven lands with a melee profile, owns a
// village weapon, some piece of the plan is still missing and the
// wallet covers it, and no trip or cooldown holds the session.
func (l *Loop) cityShoppingWanted() bool {
    if l.navigator == nil || l.tripActive() {
        return false
    }
    if l.zoneRegion != "" && l.zoneRegion != regionElven {
        return false
    }
    if !l.cityShoppingProfileAccepts() {
        return false
    }
    if _, _, _, ok := l.tracker.SelfPosition(); !ok {
        return false
    }
    if !l.cityShoppingTriedAt.IsZero() &&
        time.Since(l.cityShoppingTriedAt) < cityShopCooldown {
        return false
    }
    cost, _ := l.cityShoppingPlan()
    if cost <= 0 {
        return false
    }
    // The armed scan copies the bag for a bare paperdoll, so it runs
    // last: the steady state of a poor bot never reaches it, and the
    // bow question above is answered by the version cache.
    return l.cityShoppingArmed()
}

// maybeCityShopping runs the errand when the character qualifies. The
// errand blocks the tick: it owns every move and transaction from the
// village walk to the return, the way the quest trip engine owns a
// manual loop. It reports true when it ran, so the tick returns
// without dispatching the hunt phases. A real fight holds the start
// (a pending loot, the blows landing now); a merely held target drops
// before the walk, the errand owns the target register.
func (l *Loop) maybeCityShopping(ctx context.Context) bool {
    if !l.cityShoppingWanted() {
        return false
    }
    if l.fightBusy() {
        if l.phase == phaseLoot || l.tracker.SelfUnderAttack() {
            return false
        }
        l.dropAttackTarget("the city shopping errand owns the way")
    }
    l.startCityShopping(ctx)

    return true
}

// startCityShopping runs one errand: the phase switch, the blocking
// drive under the errand budget and the state restore. The attempt
// stamp arms the cooldown whatever the outcome was; the next ordinary
// tick hunts from the spot the errand ended on. The tracker errand
// status carries the outcome to the acceptance watch.
func (l *Loop) startCityShopping(ctx context.Context) {
    l.cityShoppingTriedAt = time.Now()
    l.phase = phaseCityShop
    started := time.Now()
    l.tracker.SetHuntErrand(state.HuntErrand{
        Name:      CityShoppingErrand,
        StartedAt: started,
        EndedAt:   time.Time{},
        Err:       "",
    })
    // The blocking errand outlives the tick, so the tracker phase is
    // published here as well: the deferred publish of the tick runs
    // only after the drive returned.
    l.tracker.SetPhase(string(phaseCityShop))
    l.clearServerSelection()
    l.target = 0
    l.lootID = 0
    cost, withBow := l.cityShoppingPlan()
    stops := l.cityShoppingPlanStops(withBow)
    l.logf("Hunt: city shop: the %d adena wallet covers the %d adena "+
        "plan, walking to Gludio with %d stops",
        l.tracker.InventoryStats().Adena, cost, len(stops))
    if withBow {
        l.logf("Hunt: city shop: the plan carries the %s",
            npcdata.ItemName(cityShopBowID))
    }
    driveCtx, cancel := context.WithTimeout(ctx, cityShopBudget)
    err := l.driveCityShopping(driveCtx, stops)
    cancel()
    if errors.Is(err, context.DeadlineExceeded) {
        err = fmt.Errorf("the errand ran past its %s budget",
            cityShopBudget)
    }
    errText := ""
    if err != nil {
        errText = err.Error()
        l.logf("Hunt: city shop: the errand stopped: %v", err)
    } else {
        l.logf("Hunt: city shop: the errand finished, the hunt resumes")
    }
    l.tracker.SetHuntErrand(state.HuntErrand{
        Name:      CityShoppingErrand,
        StartedAt: started,
        EndedAt:   time.Now(),
        Err:       errText,
    })
    l.phase = phaseEngage
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
    l.tripStops = nil
    l.resetBuyRequest()
    l.clearServerSelection()
    l.resetStagnationWatch()
}

// driveCityShopping is the blocking flow: the dress of the bagged
// village kit, the ride to Gludio, the frozen merchant stops, the
// dress of the new kit, the ride home and the walk back to the spot
// the errand started on.
func (l *Loop) driveCityShopping(
    ctx context.Context, stops []cityShopStop,
) error {
    if err := ctx.Err(); err != nil {
        return fmt.Errorf("cancelled: %w", err)
    }
    homeX, homeY, homeZ, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the errand")
    }
    // The dress the auto equipment of the tick would reach one piece
    // per tick: the errand blocks the ticks, so it dresses the bag
    // itself. The in flight equips of the last tick settle first (a
    // second use request for an equipped item toggles it off again).
    l.awaitPendingEquips(cityShopPendingWait)
    if err := l.EquipBaggedGear(); err != nil {
        return fmt.Errorf("the village dress: %w", err)
    }
    // The scroll leg replaces the walk to Mirabel whenever the ground
    // pays the price; the landing is the village respawn beside her.
    l.maybeEscapeTo(
        cityShoppingOutbound.Gatekeeper.X,
        cityShoppingOutbound.Gatekeeper.Y)
    if err := l.driveCityShopTransfer(ctx, cityShoppingOutbound); err != nil {
        return fmt.Errorf("the ride to Gludio: %w", err)
    }
    for _, stop := range stops {
        if err := l.cityShopAt(ctx, stop); err != nil {
            return err
        }
    }
    if err := l.EquipBaggedGear(); err != nil {
        return fmt.Errorf("the new kit dress: %w", err)
    }
    if err := l.driveCityShopTransfer(ctx, cityShoppingHomebound); err != nil {
        return fmt.Errorf("the ride home: %w", err)
    }

    return l.walkCityShopHome(homeX, homeY, homeZ)
}

// driveCityShopTransfer rides one gatekeeper hop from a distance: the
// walk to the teleporter station first (the knownlist push of the
// approach brings the npc into the tracker), then the dialog route of
// the quest engine.
func (l *Loop) driveCityShopTransfer(
    ctx context.Context, transfer QuestTransfer,
) error {
    if err := ctx.Err(); err != nil {
        return fmt.Errorf("cancelled: %w", err)
    }
    if err := l.walkToQuestPoint(
        transfer.Gatekeeper.X, transfer.Gatekeeper.Y,
        transfer.Gatekeeper.Z, questWalkTimeout); err != nil {
        return fmt.Errorf("the walk to %s: %w",
            transfer.Gatekeeper.Name, err)
    }

    return l.driveQuestTransfer(ctx, transfer)
}

// cityShopOpen installs the plan of one city stop for the visit: the
// purchases ride the frozen plan, the sold map records the sell
// offers, the replacement state starts clean and the stop list holds
// the single merchant of the visit.
func (l *Loop) cityShopOpen(stop cityShopStop) {
    buys := l.cityShopBuys(stop)
    l.tripPlan = buys
    l.sold = make(map[int32]bool)
    l.resetReplacementSales()
    l.tripStops = []tripStop{{
        merchant: stop.merchant,
        buys:     buys,
        sell:     false,
        teach:    false,
        guide:    false,
    }}
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
    l.sellPhaseAt = time.Now()
    l.resetBuyRequest()
}

// cityShopClose clears the stop state of the visit whatever the
// outcome.
func (l *Loop) cityShopClose() {
    l.tripStops = nil
    l.tripPlan = nil
    l.resetReplacementSales()
    l.resetBuyRequest()
    l.clearServerSelection()
    l.merchantID = 0
    l.merchantPick = time.Time{}
    l.merchantDeckUntil = time.Time{}
}

// cityShopAt runs one merchant stop: the walk to the customer stand,
// the sell first step of the pieces the stop displaces, the stop
// shopping pipeline of the ordinary town trip and the inventory
// verification of the stop items. The stop installs a single trip
// stop for the duration of the visit, so the buys ride the same
// merchant selection and buy confirmation gates the town trips use;
// the stop and the in flight batch are cleared whatever the outcome.
func (l *Loop) cityShopAt(
    ctx context.Context, stop cityShopStop,
) error {
    if err := ctx.Err(); err != nil {
        return fmt.Errorf("cancelled: %w", err)
    }
    stand := merchantStandPoint(stop.merchant)
    l.logf("Hunt: city shop: walking to %s at (%.0f, %.0f)",
        stop.merchant.Name, stand.X, stand.Y)
    if err := l.walkToQuestPoint(
        int32(stand.X), int32(stand.Y), int32(stand.Z),
        questWalkTimeout); err != nil {
        return fmt.Errorf("the walk to %s: %w", stop.merchant.Name, err)
    }
    l.cityShopOpen(stop)
    defer l.cityShopClose()
    deadline := time.Now().Add(cityShopStopTimeout)
    for {
        if err := ctx.Err(); err != nil {
            return fmt.Errorf("cancelled: %w", err)
        }
        now := time.Now()
        if l.cityShopTrade(now) {
            break
        }
        if time.Now().After(deadline) {
            return fmt.Errorf(
                "the stop at %s timed out", stop.merchant.Name)
        }
        pace(cityShopPoll)
    }
    for _, itemID := range stop.itemIDs {
        if l.tracker.InventoryHasItem(itemID) {
            continue
        }

        return fmt.Errorf("the %s from %s never arrived",
            npcdata.ItemName(itemID), stop.merchant.Name)
    }
    l.logf("Hunt: city shop: %s delivered %d pieces",
        stop.merchant.Name, len(stop.itemIDs))

    return nil
}

// cityShopTrade runs one tick of a merchant stop: the sell first step
// of the displaced pieces before the buys, so a replacement never
// lands while the piece it replaces still sits in the paperdoll. It
// reports true when the stop is done.
func (l *Loop) cityShopTrade(now time.Time) bool {
    if !l.replaceDone && l.stepReplacementSales(now) {
        l.replaceDone = true
    }

    return l.replaceDone && l.tickStopShopping(now)
}

// cityShopBuys builds the frozen purchase lines of one stop: one
// entry per item id, all riding the stop buylist (one transaction
// request) at the Gludio price, each carrying the equipped piece of
// its slot as the sell first target.
func (l *Loop) cityShopBuys(stop cityShopStop) []gear.Purchase {
    equipment := l.equipment()
    buys := make([]gear.Purchase, 0, len(stop.itemIDs))
    for _, itemID := range stop.itemIDs {
        credit, sellFirst := cityShopDisplaced(equipment, itemID)
        buys = append(buys, gear.Purchase{
            ItemID:             itemID,
            ListID:             stop.listID,
            MerchantTemplateID: stop.merchant.TemplateID,
            Count:              1,
            OwnedStack:         0,
            Price:              cityShopPrice(itemID),
            Reason:             cityShopBuyReason(itemID),
            SellFirst:          sellFirst,
            SellCredit:         credit,
            Gain:               0,
            Affordable:         true,
            Missing:            0,
        })
    }

    return buys
}

// cityShopBuyReason names the plan line of one purchase in the trip
// log.
func cityShopBuyReason(itemID int32) string {
    if itemID == cityShopBowID {
        return "the luring bow of the city shop"
    }

    return "the top grade kit of the city shop"
}

// cityShopDisplaced prices the equipped pieces one stop purchase
// replaces. The bow displaces nothing: the two hand slot it would take
// belongs to the Falchion and the shield the hunt keeps wearing, and
// the errand carries the bow as the bagged tool of the melee dress.
func cityShopDisplaced(
    equipment gear.Equipment, itemID int32,
) (int64, []int32) {
    if itemID == cityShopBowID {
        return 0, nil
    }

    return gear.DisplacedByItem(equipment, itemID)
}

// walkCityShopHome walks the character back to the spot the errand
// started from: the teleport lands on the village square, the ground
// it hunts lies beyond it.
func (l *Loop) walkCityShopHome(x int32, y int32, z int32) error {
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the walk home")
    }
    if math.Hypot(float64(selfX-x), float64(selfY-y)) <=
        cityShopHomeRadius {
        return nil
    }
    if err := l.walkToQuestPoint(x, y, z, questWalkTimeout); err != nil {
        return fmt.Errorf("the walk home: %w", err)
    }

    return nil
}

// awaitPendingEquips waits until the in flight inventory actions of
// the tick settle (confirmed or timed out), so the blocking equip
// loop re-plans against the settled paperdoll.
func (l *Loop) awaitPendingEquips(wait time.Duration) {
    deadline := time.Now().Add(wait)
    for {
        l.prunePendingActions(time.Now())
        if len(l.pendingActions) == 0 || time.Now().After(deadline) {
            return
        }
        pace(cityShopPoll)
    }
}
