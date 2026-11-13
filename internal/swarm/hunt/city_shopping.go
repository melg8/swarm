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

// The city shopping errand: a dressed character with enough adena walks to the
// Elven Village gatekeeper, teleports to Gludio, sells the village pieces its
// new kit replaces, buys the top no-grade weapon, armor set and (wallet
// permitting) bow, rides back and resumes the hunt on its own ground. The
// policy, the gates and the live route - the Mirabel and Bella pairs, the
// Sydnia and Simplon lists, the 455 760 adena kit and 292 800 bow at the 20
// percent Gludio tax - live in docs/hunting.md.

// CityShoppingErrand is the errand name the tracker status publishes; the
// acceptance watch compares against it.
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
    // cityShopBowID is the Composition Bow of Sydnia (64 pAtk), carried
    // beside the melee dress once the wallet covers the whole plan.
    cityShopBowID = int32(273)
)

// cityShopTaxPercent is the Gludio buy tax markup, in percent.
const cityShopTaxPercent = int64(20)

// The pacing and bounds of the errand.
const (
    // cityShopPoll re-reads the tracker; the shopping decisions pace
    // themselves on the transaction window.
    cityShopPoll = 500 * time.Millisecond
    // cityShopStopTimeout bounds one merchant stop with its retries.
    cityShopStopTimeout = 4 * time.Minute
    // cityShopCooldown spaces two errands of one session.
    cityShopCooldown = 10 * time.Minute
    // cityShopHomeRadius is the arrival distance of the walk home.
    cityShopHomeRadius = 300.0
    // cityShopPendingWait bounds the wait for the in flight equips: a second
    // use request for an equipped item toggles it right back off.
    cityShopPendingWait = 5 * time.Second
    // cityShopBudget bounds one whole errand. The far elven cells sit up to
    // 53 000 units from the village, so a walking segment can run near the
    // 10 minute quest walk timeout; every stage keeps its own shorter timeout.
    cityShopBudget = 30 * time.Minute
)

// cityShoppingOutbound is the Elven Village to Gludio hop through Mirabel.
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

// cityShoppingHomebound is the Gludio to Elven Village hop through Bella.
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

// cityShopStop is one merchant visit: the town npc, the buylist the whole
// stop rides (one transaction request) and the item ids it buys.
type cityShopStop struct {
    merchant townNpc
    listID   int32
    itemIDs  []int32
}

// cityShoppingWeaponStop is the Sydnia stop: the weapon runs first, so the
// old weapon sells and the Falchion lands before any armor walk.
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

// cityShoppingArmorStop is the Simplon stop: the armor set and the shield in
// one transaction of list 3025300.
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

// cityShoppingStops is the fixed melee kit plan, the weapon first. The ordinary
// planner scores by value per adena and would pick D-grade pieces or jewels.
var cityShoppingStops = []cityShopStop{
    cityShoppingWeaponStop,
    cityShoppingArmorStop,
}

// cityShopBowStop is the bow visit: the Sydnia counter and weapon list, walked
// after the kit stops. The purchase keeps the melee dress on and sells the
// weaker owned bows it replaces.
var cityShopBowStop = cityShopStop{
    merchant: cityShoppingWeaponStop.merchant,
    listID:   cityShoppingWeaponStop.listID,
    itemIDs:  []int32{cityShopBowID},
}

// CityShoppingKit returns the top no-grade melee kit ids in purchase order
// (the weapon first); the acceptance scenario verifies the worn set.
func CityShoppingKit() []int32 {
    kit := make([]int32, 0, 7)
    for _, stop := range cityShoppingStops {
        kit = append(kit, stop.itemIDs...)
    }

    return kit
}

// CityShoppingBowID returns the bow the errand buys behind the kit; the
// acceptance scenario verifies the bagged tool.
func CityShoppingBowID() int32 {
    return cityShopBowID
}

// cityShoppingProfileAccepts reports whether the profile fights with the fixed
// plan: the Falchion is a melee sword and the kit ends in a shield, and the
// caster profile scores the C1 sword by its magic attack, so the score alone
// does not separate the classes.
func (l *Loop) cityShoppingProfileAccepts() bool {
    if l.equip == nil {
        return false
    }
    _, melee := l.equip.profile.(gear.MeleeFighter)

    return melee
}

// cityShopPrice is the Gludio counter price of one kit piece: the reference
// price plus the town buy tax. The server prices the request itself.
func cityShopPrice(itemID int32) int64 {
    price := npcdata.ItemPrice(itemID)

    return price + price*cityShopTaxPercent/100
}

// cityShoppingMissingCost sums the Gludio prices of the kit pieces the
// character does not carry yet (worn or bagged).
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

// cityShoppingBowUpgrade reports whether the bow joins the errand: a character
// that already owns the bow or a stronger one never buys a second copy.
func (l *Loop) cityShoppingBowUpgrade() bool {
    stats, ok := npcdata.ItemGearStats(cityShopBowID)
    if !ok {
        return false
    }

    return l.ownedBowAttack() < stats.PAtk
}

// cityShoppingPlan resolves what one errand would buy now: the Gludio cost of
// the missing kit pieces plus the bow when the wallet covers the whole plan.
// An unaffordable plan answers zero, so the gate reads a single number.
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

// cityShoppingPlanStops builds the merchant route: the kit stops carry only the
// missing pieces (an owned line would be dropped at buy time, an empty stop
// costs a walk), and the bow stop rides last when the plan covers it.
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

// cityShoppingArmed reports whether the character already owns a village
// weapon: the errand upgrades a dressed character only.
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

// cityShoppingWanted reports whether the errand may start now: the character
// hunts the elven lands with a melee profile, owns a village weapon, misses a
// piece the wallet covers, and no trip or cooldown holds the session.
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
        l.now().Sub(l.cityShoppingTriedAt) < cityShopCooldown {
        return false
    }
    cost, _ := l.cityShoppingPlan()
    if cost <= 0 {
        return false
    }
    // The armed scan copies the bag for a bare paperdoll, so it runs last; the
    // steady state of a poor bot never reaches it.
    return l.cityShoppingArmed()
}

// maybeCityShopping runs the errand when the character qualifies. The errand
// blocks the tick - it owns every move and transaction from the village walk
// to the return - and reports true when it ran, so the tick skips the hunt
// phases. A pending loot or the blows landing now hold the start; a merely
// held target drops before the walk.
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

// startCityShopping runs one errand: the phase switch, the blocking drive under
// the errand budget and the state restore. The attempt stamp arms the cooldown
// whatever the outcome; the tracker status carries it to the acceptance watch.
func (l *Loop) startCityShopping(ctx context.Context) {
    l.cityShoppingTriedAt = l.now()
    l.phase = phaseCityShop
    started := l.now()
    l.tracker.SetHuntErrand(state.HuntErrand{
        Name:      CityShoppingErrand,
        StartedAt: started,
        EndedAt:   time.Time{},
        Err:       "",
    })
    // The blocking errand outlives the tick, so the phase is published here;
    // the deferred publish of the tick runs only after the drive returns.
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
        EndedAt:   l.now(),
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

// driveCityShopping is the blocking flow: the village dress, the ride to
// Gludio, the frozen merchant stops, the new kit dress, the ride home and the
// walk back to the start spot.
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
    // The auto equipment dresses one piece per tick and the errand blocks the
    // ticks, so it dresses the bag itself; the in flight equips settle first.
    l.awaitPendingEquips(cityShopPendingWait)
    if err := l.EquipBaggedGear(); err != nil {
        return fmt.Errorf("the village dress: %w", err)
    }
    // The scroll leg replaces the walk to Mirabel whenever the ground pays the
    // price; the landing is the village respawn beside her.
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

// driveCityShopTransfer rides one gatekeeper hop from a distance: the walk to
// the teleporter (the approach brings the npc into the tracker) and the dialog
// route of the quest engine.
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

// cityShopOpen installs the plan of one city stop: the frozen purchases, a
// fresh sold map and replacement state, and a single merchant stop.
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
    l.sellPhaseAt = l.now()
    l.resetBuyRequest()
}

// cityShopClose clears the stop state of the visit.
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

// cityShopAt runs one merchant stop: the walk to the customer stand, the sell
// first step of the displaced pieces, the stop shopping pipeline of the
// ordinary town trip and the inventory verification. The single trip stop
// rides the same selection and confirmation gates the town trips use.
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
    deadline := l.now().Add(cityShopStopTimeout)
    for {
        if err := ctx.Err(); err != nil {
            return fmt.Errorf("cancelled: %w", err)
        }
        now := l.now()
        if l.cityShopTrade(now) {
            break
        }
        if l.now().After(deadline) {
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

// cityShopTrade runs one tick of a merchant stop: the displaced pieces sell
// before the buys, so a replacement never lands while its piece still sits in
// the paperdoll. It reports true when the stop is done.
func (l *Loop) cityShopTrade(now time.Time) bool {
    if !l.replaceDone && l.stepReplacementSales(now) {
        l.replaceDone = true
    }

    return l.replaceDone && l.tickStopShopping(now)
}

// cityShopBuys builds the frozen purchase lines of one stop: one entry per
// item id on the stop buylist at the Gludio price, each naming the equipped
// piece of its slot as the sell first target.
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

// cityShopBuyReason names the plan line of one purchase in the trip log.
func cityShopBuyReason(itemID int32) string {
    if itemID == cityShopBowID {
        return "the luring bow of the city shop"
    }

    return "the top grade kit of the city shop"
}

// cityShopDisplaced prices the pieces one stop purchase retires. A kit purchase
// displaces the worn village piece of its slot; the bow displaces nothing on
// the paperdoll (the two hand slot belongs to the Falchion and the shield) but
// retires the weaker owned bows, because the lurer keeps only the strongest
// one (addLureToolKeeps).
func cityShopDisplaced(
    equipment gear.Equipment, itemID int32,
) (int64, []int32) {
    if itemID == cityShopBowID {
        return weakerBowSale(equipment, itemID)
    }

    return gear.DisplacedByItem(equipment, itemID)
}

// weakerBowSale collects the owned bows of a lower attack than the bought one,
// equipped or bagged, each at its referencePrice/2 sell value.
func weakerBowSale(equipment gear.Equipment, itemID int32) (int64, []int32) {
    bought, ok := npcdata.ItemGearStats(itemID)
    if !ok {
        return 0, nil
    }
    var credit int64
    ids := make([]int32, 0, 1)
    for _, item := range equipment.Items {
        stats, statsOK := npcdata.ItemGearStats(item.ItemID)
        if !statsOK || stats.WeaponType != weaponTypeBow ||
            stats.PAtk >= bought.PAtk {
            continue
        }
        credit += npcdata.ItemPrice(item.ItemID) / 2
        ids = append(ids, item.ObjectID)
    }

    return credit, ids
}

// walkCityShopHome walks the character back to the spot the errand started
// from: the teleport lands on the village square, the hunt ground lies beyond.
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

// awaitPendingEquips waits until the in flight inventory actions settle, so the
// blocking equip loop re-plans against the settled paperdoll.
func (l *Loop) awaitPendingEquips(wait time.Duration) {
    deadline := l.now().Add(wait)
    for {
        l.prunePendingActions(l.now())
        if len(l.pendingActions) == 0 || l.now().After(deadline) {
            return
        }
        pace(cityShopPoll)
    }
}
