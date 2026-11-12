// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The city shopping acceptance scenario: a character that already
// wears the village dress and has saved 800 000 adena walks to the
// Elven Village gatekeeper, teleports to Gludio, buys the top
// no-grade weapon and armor set at the town shops, carries the top
// no-grade bow beside them, rides back to the Elven Village and
// resumes the hunt on the start ground. The errand itself lives in
// hunt/city_shopping.go and starts from the production trigger; the
// scenario only sets the start state and watches the story.

// cityShoppingScenarioID is the scenario registry id.
const cityShoppingScenarioID = "city-shopping"

// cityShoppingAccount is the temp account of the scenario: the next
// free slot after the archer fleet spread (temp20 through temp24).
// The password equals the account (the Mobius login auto-creates
// missing accounts).
const (
    cityShoppingAccount  = "temp25"
    cityShoppingPassword = "temp25"
)

// cityShoppingTimeout bounds the whole run: the errand stages (the
// walk to Mirabel, the outbound ride, both merchant stops, the dress,
// the ride home and the walk back) complete in under four minutes on
// the deployed stack, the standard restock trip that follows spends
// about five, and the resumed farm adds one kill. The watch fails
// fast on a stopped errand or a silent return ground, so the budget
// only catches a session that never reaches either gate.
const cityShoppingTimeout = 20 * time.Minute

// cityShoppingErrandWall bounds the errand runtime the watch accepts:
// the fixture's ground sits 11 000 units from Mirabel and the errand
// completes in about three minutes on the deployed stack, so the wall
// catches a hung errand long before the 30 minute driver budget of a
// far cell would.
const cityShoppingErrandWall = 12 * time.Minute

// cityShoppingFarmWindow bounds the resumed farm wait after the
// return: the ordinary hunt opens the standard restock trip right
// after the errand (the pull bow, arrows, the escape scroll and the
// Newbie Guide buffs - a walk across the town and back), so the
// window covers that trip plus the first kill on the Kaboo ground. A
// bot that never comes back to the ground stays silent past it.
const cityShoppingFarmWindow = 10 * time.Minute

// The start state of the errand: the level 15 vitals on the Kaboo
// Orc Grunt SW-5 cell (elven-hex-104), where the cell median of 9
// stays under the delevel trigger and the hunt resumes on the
// return. The 800 000 adena cover the 748 560 Gludio counter price of
// the top no-grade kit and the bow behind it.
const (
    cityShoppingSpawnX = 36000
    cityShoppingSpawnY = 50229
    cityShoppingSpawnZ = -3456
    cityShoppingLevel  = 15
    cityShoppingAdena  = 800000
)

// The arrival and return gates of the watch: the Gludio teleport
// square and the start ground.
const (
    cityShoppingGludioX      = -12694
    cityShoppingGludioY      = 122776
    cityShoppingGludioRadius = 3500.0
    cityShoppingHomeRadius   = 3000.0
)

// The condition ids of the scenario.
const (
    // checkCityShopGludio holds once the teleport landed the
    // character on the Gludio square.
    checkCityShopGludio = "gludio"
    // checkCityShopWeapon holds once the Falchion is in the bag.
    checkCityShopWeapon = "weapon"
    // checkCityShopArmor holds once every armor piece of the kit is
    // in the bag.
    checkCityShopArmor = "armor"
    // checkCityShopBow holds once the top no-grade bow is in the bag
    // beside the melee kit.
    checkCityShopBow = "bow"
    // checkCityShopSold holds once the replaced village gear left the
    // bag during the errand.
    checkCityShopSold = "sold"
    // checkCityShopWorn holds once the whole kit fills its paperdoll
    // slots.
    checkCityShopWorn = "worn"
    // checkCityShopHome holds once the character walked back to the
    // start ground after the purchases.
    checkCityShopHome = "home"
    // checkCityShopFarm holds once a kill or experience growth
    // followed the return.
    checkCityShopFarm = "farming"
)

// cityShoppingVillageKit is the village dress in the bag: the level
// 15 no-grade set the auto equipment puts on, the weapon the errand
// upgrades and the starter jewels. The two rings ride separate rows
// so both fingers receive their own object id.
var cityShoppingVillageKit = []ResetItem{
    {ItemID: 2, Count: 1},   // Long Sword
    {ItemID: 24, Count: 1},  // Bone Breastplate
    {ItemID: 31, Count: 1},  // Bone Gaiters
    {ItemID: 44, Count: 1},  // Leather Helmet
    {ItemID: 50, Count: 1},  // Leather Gloves
    {ItemID: 38, Count: 1},  // Low Boots
    {ItemID: 102, Count: 1}, // Round Shield
    {ItemID: 114, Count: 1}, // Earring of Strength
    {ItemID: 115, Count: 1}, // Earring of Wisdom
    {ItemID: 876, Count: 1}, // Ring of Anguish (right finger)
    {ItemID: 876, Count: 1}, // Ring of Anguish (left finger)
    {ItemID: 907, Count: 1}, // Necklace of Anguish
}

// cityShoppingOldGear is the village weapon, armor set and shield the
// errand replaces and must sell at the Gludio merchants (the starter
// jewels are kept).
var cityShoppingOldGear = []int32{2, 24, 31, 44, 50, 38, 102}

// cityShoppingSlot is one piece of the top kit and the paperdoll
// slot it must fill: the worn check maps every slot object id back
// to its item id and compares against this table.
type cityShoppingSlot struct {
    itemID int32
    slot   int
}

// cityShoppingSlots is the paperdoll layout of the top no-grade kit
// in purchase order (the weapon first, the same order the hunt
// driver buys in).
var cityShoppingSlots = []cityShoppingSlot{
    {itemID: 68, slot: state.PaperdollRHand},  // Falchion
    {itemID: 26, slot: state.PaperdollChest},  // Bronze Breastplate
    {itemID: 34, slot: state.PaperdollLegs},   // Bronze Gaiters
    {itemID: 1148, slot: state.PaperdollHead}, // Hard Leather Helmet
    {itemID: 51, slot: state.PaperdollGloves}, // Bracer
    {itemID: 39, slot: state.PaperdollFeet},   // Boots
    {itemID: 625, slot: state.PaperdollLHand}, // Bone Shield
}

// cityShoppingReset returns the start state of the errand: the level
// 15 vitals and experience of the checkpoint table, the 800 000
// adena wallet and the village dress in the bag on the Kaboo ground.
func cityShoppingReset(account string) characterReset {
    return characterReset{
        Account: account,
        Char:    account,
        Level:   cityShoppingLevel,
        Exp:     level15Exp,
        SP:      0,
        Adena:   cityShoppingAdena,
        Items:   cityShoppingVillageKit,
        X:       cityShoppingSpawnX,
        Y:       cityShoppingSpawnY,
        Z:       cityShoppingSpawnZ,
        MaxHP:   level15HP,
        MaxMP:   level15MP,
        MaxCP:   level15CP,
    }
}

// cityShoppingChecks builds the initial check list of the scenario:
// the errand story in its narrative order.
func cityShoppingChecks() []Check {
    return []Check{
        {
            ID: checkOnline, Label: labelEnteredWorld, Done: false,
            Detail: "",
        },
        {
            ID:    checkCityShopGludio,
            Label: "the character teleported to Gludio",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopWeapon,
            Label: "Sydnia sold the top no-grade weapon",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopArmor,
            Label: "Simplon sold the whole no-grade armor set",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopBow,
            Label: "Sydnia sold the top no-grade bow",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopSold,
            Label: "the replaced weapon and armor were sold",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopWorn,
            Label: "the whole top no-grade kit is worn",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopHome,
            Label: "the bot walked back to the start ground",
            Done:  false, Detail: "",
        },
        {
            ID:    checkCityShopFarm,
            Label: "the hunt resumed after the return",
            Done:  false, Detail: "",
        },
    }
}

// startCitySession launches the supervised scenario session: the
// production errand trigger finds the start state and runs the story.
func startCitySession(
    ctx context.Context, m *Manager, test *Test,
) (context.CancelFunc, chan error) {
    sessionCtx, cancelSession := context.WithCancel(ctx)
    sessionDone := make(chan error, 1)
    go func() {
        m.runSessionSupervised(sessionCtx, cityShoppingAccount,
            cityShoppingPassword, cityShoppingAccount, m.proxy,
            test.appendLog)
        sessionDone <- nil
    }()

    return cancelSession, sessionDone
}

// cityShoppingScenario runs the errand round: the temp character
// wakes with the village dress in the bag and the 800 000 adena
// wallet, the armed hunt loop walks it through the city purchase and
// the watch verifies every stage of the story.
func cityShoppingScenario(ctx context.Context, m *Manager, t *Test) error {
    test := t
    test.setChecks(cityShoppingChecks())

    if err := m.ensureCharacter(cityShoppingAccount, cityShoppingPassword,
        cityShoppingAccount, test.appendLog); err != nil {
        return fmt.Errorf("ensure character: %w", err)
    }
    time.Sleep(ensurePause)
    if err := m.injectReset(cityShoppingReset(cityShoppingAccount),
        test); err != nil {
        return fmt.Errorf("inject start state: %w", err)
    }
    time.Sleep(ensurePause)

    cancelSession, sessionDone := startCitySession(ctx, m, test)
    defer cancelSession()

    tracker := m.tracker(test)
    if err := waitOnline(ctx, tracker, test); err != nil {
        cancelSession()
        joinSessionDone(sessionDone)

        return err
    }
    test.appendLog("acceptance: the bot is in the world with the " +
        "village dress and 800 000 adena, watching the city shopping " +
        "errand")

    watch := &cityShoppingWatch{}
    for {
        if ctx.Err() != nil {
            cancelSession()
            joinSessionDone(sessionDone)

            return fmt.Errorf("cancelled: %w", ctx.Err())
        }
        watch.evaluate(tracker, test)
        if err := watch.abort(tracker, test); err != nil {
            cancelSession()
            joinSessionDone(sessionDone)

            return err
        }
        if allChecksDone(test) {
            test.appendLog("acceptance: the kit and the bow are in " +
                "and the hunt resumed, stopping the bot")
            cancelSession()
            if err := <-sessionDone; err != nil {
                return fmt.Errorf("session end: %w", err)
            }
            test.appendLog("acceptance: the bot left the world gracefully")

            return nil
        }
        select {
        case <-ctx.Done():
            cancelSession()
            joinSessionDone(sessionDone)

            return fmt.Errorf("cancelled: %w", ctx.Err())
        case <-time.After(monitorPeriod):
        }
    }
}

// cityShoppingWatch accumulates the latches of the scenario: the
// purchase flags gate the return watch, the home stamp gates the
// resumed farm check, the visit, dress and farm flags retire their
// checks once the story reached them - the bot leaves Gludio and
// swaps gear on the later town trips, so a live position or paperdoll
// predicate would reopen a fact the run already proved.
type cityShoppingWatch struct {
    weaponBought bool
    armorBought  bool
    bowBought    bool
    bowSeen      bool
    bowDetail    string
    gludioSeen   bool
    gludioDetail string
    soldSeen     bool
    soldDetail   string
    wornSeen     bool
    wornDetail   string
    homeAtMs     int64
    homeExp      int32
    farmDone     bool
}

// evaluate rewrites the check list from one live tracker snapshot.
func (w *cityShoppingWatch) evaluate(tracker *state.Bot, test *Test) {
    if tracker.Status() != state.StatusOnline {
        return
    }
    selfX, selfY, _, posOK := tracker.SelfPosition()
    if posOK {
        arrived := math.Hypot(
            float64(selfX-cityShoppingGludioX),
            float64(selfY-cityShoppingGludioY),
        ) <= cityShoppingGludioRadius
        if arrived {
            w.gludioSeen = true
            w.gludioDetail = fmt.Sprintf(
                "standing at %d,%d", selfX, selfY)
        }
        detail := w.gludioDetail
        if detail == "" {
            detail = fmt.Sprintf("standing at %d,%d", selfX, selfY)
        }
        test.updateCheck(checkCityShopGludio, w.gludioSeen, detail)
    }
    w.weaponBought = tracker.InventoryHasItem(
        cityShoppingSlots[0].itemID)
    _, weaponDetail := cityShoppingBagDetail(
        tracker, cityShoppingSlots[:1])
    test.updateCheck(checkCityShopWeapon, w.weaponBought, weaponDetail)
    armorBought, armorDetail := cityShoppingBagDetail(
        tracker, cityShoppingSlots[1:])
    w.armorBought = armorBought == len(cityShoppingSlots)-1
    test.updateCheck(checkCityShopArmor, w.armorBought, armorDetail)
    w.evaluateBow(tracker, test)
    w.evaluateSales(tracker, test)
    worn, wornDetail := cityShoppingWornDetail(tracker)
    if worn == len(cityShoppingSlots) {
        w.wornSeen = true
        w.wornDetail = wornDetail
    }
    if w.wornDetail != "" {
        wornDetail = w.wornDetail
    }
    test.updateCheck(checkCityShopWorn, w.wornSeen, wornDetail)
    if !w.weaponBought || !w.armorBought || !w.bowBought || !posOK {
        return
    }
    atHome := math.Hypot(
        float64(selfX-cityShoppingSpawnX),
        float64(selfY-cityShoppingSpawnY),
    ) <= cityShoppingHomeRadius
    if !atHome {
        return
    }
    if w.homeAtMs == 0 {
        w.homeAtMs = time.Now().UnixMilli()
        w.homeExp = tracker.SelfExp()
    }
    test.updateCheck(checkCityShopHome, true,
        fmt.Sprintf("back at %d,%d after the city run", selfX, selfY))
    w.evaluateFarming(tracker, test)
}

// evaluateBow latches the bow purchase: the tool rides the last
// Gludio stop, behind the melee kit. The latch retires the live bag
// predicate - the junk flows may sell a duplicate later and would
// reopen the proven fact.
func (w *cityShoppingWatch) evaluateBow(
    tracker *state.Bot, test *Test,
) {
    if w.bowSeen {
        test.updateCheck(checkCityShopBow, true, w.bowDetail)

        return
    }
    w.bowBought = tracker.InventoryHasItem(hunt.CityShoppingBowID())
    if w.bowBought {
        w.bowSeen = true
        w.bowDetail = "the Composition Bow is in the bag"
    }
    detail := w.bowDetail
    if detail == "" {
        detail = "waiting for the bow purchase"
    }
    test.updateCheck(checkCityShopBow, w.bowSeen, detail)
}

// evaluateSales latches the sale of the replaced village gear: the
// old pieces must leave the bag while the errand still runs, because
// the later restock trip sells the leftovers as junk and would mask
// the missing sell first step.
func (w *cityShoppingWatch) evaluateSales(
    tracker *state.Bot, test *Test,
) {
    if w.soldSeen {
        test.updateCheck(checkCityShopSold, true, w.soldDetail)

        return
    }
    errand := tracker.HuntErrand()
    if errand.Name != hunt.CityShoppingErrand || !errand.EndedAt.IsZero() {
        return
    }
    if !w.weaponBought || !w.armorBought {
        return
    }
    carried := 0
    for _, itemID := range cityShoppingOldGear {
        if tracker.InventoryHasItem(itemID) {
            carried++
        }
    }
    if carried > 0 {
        test.updateCheck(checkCityShopSold, false, fmt.Sprintf(
            "%d of %d old pieces still in the bag", carried,
            len(cityShoppingOldGear)))

        return
    }
    w.soldSeen = true
    w.soldDetail = "the old village gear left the bag at Gludio"
    test.updateCheck(checkCityShopSold, true, w.soldDetail)
}

// evaluateFarming reports whether the hunt of the return ground paid
// after the home stamp: a kill mark of the stamp or later, or the
// experience growth of the resumed farm.
func (w *cityShoppingWatch) evaluateFarming(
    tracker *state.Bot, test *Test,
) {
    if w.homeAtMs == 0 {
        return
    }
    grown := tracker.SelfExp() > w.homeExp
    killed := false
    for _, mark := range tracker.KillMarks() {
        if mark.AtMs >= w.homeAtMs {
            killed = true

            break
        }
    }
    done := killed || grown
    detail := "waiting for the first kill after the return"
    switch {
    case killed && grown:
        detail = "a kill landed and the experience grew after the return"
    case killed:
        detail = "a kill landed after the return"
    case grown:
        detail = "the experience grew after the return"
    }
    w.farmDone = done
    test.updateCheck(checkCityShopFarm, done, detail)
}

// abort reports the terminal failure of the run: the errand the watch
// follows stopped with an error, the errand overran its driver budget
// without an answer, or the return ground produced no farm inside the
// window. A stopped errand never fixes itself, so the scenario ends at
// once instead of idling out its own budget.
func (w *cityShoppingWatch) abort(
    tracker *state.Bot, test *Test,
) error {
    errand := tracker.HuntErrand()
    if errand.Name == hunt.CityShoppingErrand &&
        !errand.StartedAt.IsZero() {
        if !errand.EndedAt.IsZero() && errand.Err != "" {
            return fmt.Errorf("the city shopping errand stopped: %s",
                errand.Err)
        }
        if errand.EndedAt.IsZero() &&
            time.Since(errand.StartedAt) > cityShoppingErrandWall {
            return fmt.Errorf(
                "the city shopping errand runs for %s without an "+
                    "answer, past the %s wall",
                time.Since(errand.StartedAt).Round(time.Second),
                cityShoppingErrandWall)
        }
    }
    if w.homeAtMs == 0 || w.farmDone {
        return nil
    }
    since := time.Since(time.UnixMilli(w.homeAtMs))
    if since > cityShoppingFarmWindow {
        test.updateCheck(checkCityShopFarm, false,
            "no kill landed inside the farm window of the return")

        return fmt.Errorf(
            "the hunt did not resume within %s of the return",
            cityShoppingFarmWindow)
    }

    return nil
}

// cityShoppingBagDetail counts the pieces of the slot range the bag
// carries and renders the count.
func cityShoppingBagDetail(
    tracker *state.Bot, pieces []cityShoppingSlot,
) (int, string) {
    count := 0
    for _, piece := range pieces {
        if tracker.InventoryHasItem(piece.itemID) {
            count++
        }
    }

    return count, fmt.Sprintf("%d of %d pieces", count, len(pieces))
}

// cityShoppingWornDetail maps the paperdoll object ids back to the
// item ids and counts the kit pieces the worn slots hold.
func cityShoppingWornDetail(tracker *state.Bot) (int, string) {
    itemByObject := make(map[int32]int32)
    for _, item := range tracker.InventoryItems() {
        itemByObject[item.ObjectID] = item.ItemID
    }
    doll := tracker.PaperdollSlotObjectIDs()
    count := 0
    for _, piece := range cityShoppingSlots {
        if itemByObject[doll[piece.slot]] == piece.itemID {
            count++
        }
    }

    return count, fmt.Sprintf("%d of %d slots", count,
        len(cityShoppingSlots))
}
