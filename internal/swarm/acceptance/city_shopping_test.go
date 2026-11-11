// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The city shopping scenario tests: the registration, the start
// state and the paperdoll table the watch checks against - the
// offline half of the errand verification, no live stack needed.

// TestCityShoppingDefinition pins the scenario registration: the
// temp account, the run budget and the story of the errand.
func TestCityShoppingDefinition(t *testing.T) {
    var found bool
    for _, def := range Definitions() {
        if def.ID != cityShoppingScenarioID {
            continue
        }
        found = true
        require.Equal(t, cityShoppingAccount, def.Account)
        require.Equal(t, cityShoppingTimeout, def.Timeout)
        require.NotNil(t, def.Scenario)
        for _, needle := range []string{
            "36000 50229 -3456", "level 15", "500 000 adena",
            "Mirabel", "Gludio", "Sydnia", "Simplon", "Bella",
        } {
            require.Contains(t, def.Description, needle)
        }
    }
    require.True(t, found, "the scenario must be registered")
}

// TestCityShoppingSlotsMatchTheKit pins the acceptance check table to
// the hunt kit: the watch cannot drift from the driver.
func TestCityShoppingSlotsMatchTheKit(t *testing.T) {
    itemIDs := make([]int32, 0, len(cityShoppingSlots))
    for _, piece := range cityShoppingSlots {
        itemIDs = append(itemIDs, piece.itemID)
    }
    require.Equal(t, hunt.CityShoppingKit(), itemIDs)
    require.Len(t, cityShoppingSlots, 7)
    for index, piece := range cityShoppingSlots {
        require.NotZero(t, piece.slot, "piece %d has a slot", index)
    }
}

// TestCityShoppingResetShape pins the injected start state: the
// level 15 vitals, the 500 000 adena wallet and the village dress in
// the bag on the Kaboo ground.
func TestCityShoppingResetShape(t *testing.T) {
    reset := cityShoppingReset(cityShoppingAccount)
    require.Equal(t, cityShoppingAccount, reset.Account)
    require.Equal(t, cityShoppingAccount, reset.Char)
    require.Equal(t, int32(cityShoppingLevel), reset.Level)
    require.Equal(t, int64(cityShoppingAdena), reset.Adena)
    require.Equal(t, reset.Exp, int64(level15Exp))
    require.Equal(t, int64(0), reset.SP)
    require.Equal(t, int32(cityShoppingSpawnX), reset.X)
    require.Equal(t, int32(cityShoppingSpawnY), reset.Y)
    require.Equal(t, int32(cityShoppingSpawnZ), reset.Z)
    require.Equal(t, int32(level15HP), reset.MaxHP)
    require.Equal(t, int32(level15MP), reset.MaxMP)
    require.Equal(t, int32(level15CP), reset.MaxCP)
    requiresBagItem(t, reset.Items, 2, "Long Sword")
    requiresBagItem(t, reset.Items, 102, "Round Shield")
}

// requiresBagItem asserts one item of the injected bag.
func requiresBagItem(
    t *testing.T, items []ResetItem, itemID int32, name string,
) {
    t.Helper()
    for _, item := range items {
        if item.ItemID == itemID {
            require.Equal(t, int32(1), item.Count, name)

            return
        }
    }
    require.Failf(t, "the bag misses an item", "%s (%d)", name, itemID)
}

// TestCityShoppingCheckStory pins the check list: every stage of the
// errand appears exactly once.
func TestCityShoppingCheckStory(t *testing.T) {
    checks := cityShoppingChecks()
    ids := make([]string, 0, len(checks))
    for _, check := range checks {
        ids = append(ids, check.ID)
        require.False(t, check.Done, "the check starts open")
        require.NotEmpty(t, check.Label)
    }
    require.Equal(t, []string{
        checkOnline,
        checkCityShopGludio,
        checkCityShopWeapon,
        checkCityShopArmor,
        checkCityShopSold,
        checkCityShopWorn,
        checkCityShopHome,
        checkCityShopFarm,
    }, ids)
}

// TestCityShoppingWornDetail pins the paperdoll map: a fully worn
// kit counts every slot, a village dress counts none.
func TestCityShoppingWornDetail(t *testing.T) {
    bot := state.NewBot("worn")
    doll := [state.PaperdollSlots]int32{}
    items := make([]state.InventoryItem, 0, len(cityShoppingSlots))
    for index, piece := range cityShoppingSlots {
        objectID := int32(1000 + index)
        doll[piece.slot] = objectID
        items = append(items, state.InventoryItem{
            ObjectID: objectID, ItemID: piece.itemID, Count: 1,
            Equipped: true,
        })
    }
    bot.ApplyItemList(items)
    bot.ApplyPaperdoll(doll)
    count, detail := cityShoppingWornDetail(bot)
    require.Equal(t, len(cityShoppingSlots), count)
    require.Equal(t, "7 of 7 slots", detail)

    village := state.NewBot("village")
    village.ApplyItemList([]state.InventoryItem{
        {ObjectID: 7, ItemID: 2, Count: 1, Equipped: true},
    })
    village.ApplyPaperdoll([state.PaperdollSlots]int32{})
    count, detail = cityShoppingWornDetail(village)
    require.Zero(t, count)
    require.Equal(t, "0 of 7 slots", detail)
}

// TestCityShoppingWatchAbort pins the fail fast matrix of the watch:
// a stopped errand, an errand past its wall and a silent return
// ground end the run at once, while an errand in flight inside the
// wall, a finished one and a paid return ground keep it alive.
func TestCityShoppingWatchAbort(t *testing.T) {
    now := time.Now()
    finished := state.HuntErrand{
        Name:      hunt.CityShoppingErrand,
        StartedAt: now.Add(-5 * time.Minute),
        EndedAt:   now.Add(-time.Minute),
        Err:       "",
    }
    stopped := state.HuntErrand{
        Name:      hunt.CityShoppingErrand,
        StartedAt: now.Add(-5 * time.Minute),
        EndedAt:   now.Add(-time.Minute),
        Err: "the ride home: the teleport to Elven Village " +
            "never landed",
    }
    cases := []struct {
        name       string
        errand     state.HuntErrand
        homeAgo    time.Duration
        farmDone   bool
        wantErr    string
        wantDetail string
    }{
        {
            name:   "no errand ever ran",
            errand: state.HuntErrand{},
        },
        {
            name: "another errand ran",
            errand: state.HuntErrand{
                Name:      "town-trip",
                StartedAt: now.Add(-time.Hour),
                EndedAt:   now.Add(-59 * time.Minute),
                Err:       "another errand failed",
            },
        },
        {
            name:   "the errand finished",
            errand: finished,
        },
        {
            name:    "the errand stopped",
            errand:  stopped,
            wantErr: "never landed",
        },
        {
            name: "the errand overran its wall",
            errand: state.HuntErrand{
                Name: hunt.CityShoppingErrand,
                StartedAt: now.Add(
                    -cityShoppingErrandWall - time.Minute),
            },
            wantErr: "without an answer",
        },
        {
            name: "the errand is inside its wall",
            errand: state.HuntErrand{
                Name:      hunt.CityShoppingErrand,
                StartedAt: now.Add(-time.Minute),
            },
        },
        {
            name:       "a silent return ground",
            errand:     finished,
            homeAgo:    cityShoppingFarmWindow + time.Minute,
            wantErr:    "did not resume",
            wantDetail: "no kill landed",
        },
        {
            name:     "a paid return ground",
            errand:   finished,
            homeAgo:  cityShoppingFarmWindow + time.Minute,
            farmDone: true,
        },
    }
    for _, testCase := range cases {
        t.Run(testCase.name, func(t *testing.T) {
            bot := state.NewBot(cityShoppingAccount)
            bot.SetHuntErrand(testCase.errand)
            watch := &cityShoppingWatch{farmDone: testCase.farmDone}
            if testCase.homeAgo > 0 {
                watch.homeAtMs = now.Add(
                    -testCase.homeAgo).UnixMilli()
            }
            test := &Test{}
            test.setChecks(cityShoppingChecks())

            err := watch.abort(bot, test)
            if testCase.wantErr == "" {
                require.NoError(t, err)
            } else {
                require.ErrorContains(t, err, testCase.wantErr)
            }
            if testCase.wantDetail == "" {
                return
            }
            require.Contains(t, test.checks[7].Detail,
                testCase.wantDetail)
            require.Equal(t, checkCityShopFarm, test.checks[7].ID)
            require.False(t, test.checks[7].Done)
        })
    }
}

// TestCityShoppingWatchLatchesTheStory pins the latch of the visit
// and dress checks: the bot must leave Gludio and swaps gear on the
// later town trips, so a live position or paperdoll predicate would
// reopen facts the run already proved and the scenario could never
// reach all checks done.
func TestCityShoppingWatchLatchesTheStory(t *testing.T) {
    bot := state.NewBot(cityShoppingAccount)
    bot.SetCharacter(cityShoppingAccount, 100, 18,
        cityShoppingSpawnX, cityShoppingSpawnY, -3456,
        level15HP, level15MP)
    bot.SetOnline(cityShoppingAccount)
    items := make([]state.InventoryItem, 0, len(cityShoppingSlots))
    var doll [state.PaperdollSlots]int32
    for index, piece := range cityShoppingSlots {
        objectID := int32(2000 + index)
        doll[piece.slot] = objectID
        items = append(items, state.InventoryItem{
            ObjectID: objectID, ItemID: piece.itemID, Count: 1,
            Equipped: true,
        })
    }
    bot.ApplyItemList(items)

    watch := &cityShoppingWatch{}
    test := &Test{}
    test.setChecks(cityShoppingChecks())

    // In Gludio with the whole kit worn: both facts latch.
    bot.ApplyUserInfo(state.UserInfo{
        Name: cityShoppingAccount, Level: 15, ClassID: 18,
        X: cityShoppingGludioX, Y: cityShoppingGludioY, Z: -3114,
        MaxHP: level15HP, CurHP: level15HP,
        MaxMP: level15MP, CurMP: level15MP,
    })
    bot.ApplyPaperdoll(doll)
    watch.evaluate(bot, test)
    require.True(t, test.checks[1].Done, "the Gludio arrival latches")
    require.True(t, test.checks[5].Done, "the worn kit latches")

    // Back home with the paperdoll emptied: the live predicates are
    // false, the latched checks stay done.
    bot.ApplyUserInfo(state.UserInfo{
        Name: cityShoppingAccount, Level: 15, ClassID: 18,
        X: cityShoppingSpawnX, Y: cityShoppingSpawnY, Z: -3456,
        MaxHP: level15HP, CurHP: level15HP,
        MaxMP: level15MP, CurMP: level15MP,
    })
    bot.ApplyPaperdoll([state.PaperdollSlots]int32{})
    watch.evaluate(bot, test)
    require.True(t, test.checks[1].Done,
        "the Gludio arrival stays proven after the return")
    require.True(t, test.checks[5].Done,
        "the worn kit stays proven after a gear swap")
}

// TestCityShoppingWatchSellsTheOldKit pins the sale latch: the old
// village gear must leave the bag while the errand is open, and the
// restock trip after the errand cannot turn the check green.
func TestCityShoppingWatchSellsTheOldKit(t *testing.T) {
    newBot := func(oldGear bool) *state.Bot {
        bot := state.NewBot(cityShoppingAccount)
        bot.SetCharacter(cityShoppingAccount, 100, 18,
            cityShoppingGludioX, cityShoppingGludioY, -3114,
            level15HP, level15MP)
        bot.SetOnline(cityShoppingAccount)
        items := make([]state.InventoryItem, 0, len(cityShoppingSlots)+1)
        for index, piece := range cityShoppingSlots {
            items = append(items, state.InventoryItem{
                ObjectID: int32(3000 + index), ItemID: piece.itemID,
                Count: 1,
            })
        }
        if oldGear {
            items = append(items, state.InventoryItem{
                ObjectID: 4000, ItemID: 2, Count: 1,
            })
        }
        bot.ApplyItemList(items)
        bot.ApplyUserInfo(state.UserInfo{
            Name: cityShoppingAccount, Level: 15, ClassID: 18,
            X: cityShoppingGludioX, Y: cityShoppingGludioY, Z: -3114,
            MaxHP: level15HP, CurHP: level15HP,
            MaxMP: level15MP, CurMP: level15MP,
        })

        return bot
    }
    openErrand := state.HuntErrand{
        Name:      hunt.CityShoppingErrand,
        StartedAt: time.Now(),
        EndedAt:   time.Time{},
        Err:       "",
    }

    // The old gear left the bag while the errand runs: the check
    // latches.
    sold := newBot(false)
    sold.SetHuntErrand(openErrand)
    watch := &cityShoppingWatch{}
    test := &Test{}
    test.setChecks(cityShoppingChecks())
    watch.evaluate(sold, test)
    require.True(t, test.checks[4].Done, "the sale latches")
    require.Equal(t, checkCityShopSold, test.checks[4].ID)

    // A leftover old piece keeps the check open with its count.
    leftover := newBot(true)
    leftover.SetHuntErrand(openErrand)
    watch = &cityShoppingWatch{}
    test = &Test{}
    test.setChecks(cityShoppingChecks())
    watch.evaluate(leftover, test)
    require.False(t, test.checks[4].Done)
    require.Contains(t, test.checks[4].Detail, "1 of 7 old pieces")

    // The restock trip after the errand sells the leftover: the
    // ended errand never turns the check green.
    leftover.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 4000, ItemID: 2, Count: 0, Change: 3},
    })
    leftover.SetHuntErrand(state.HuntErrand{
        Name:      hunt.CityShoppingErrand,
        StartedAt: openErrand.StartedAt,
        EndedAt:   time.Now(),
        Err:       "",
    })
    watch.evaluate(leftover, test)
    require.False(t, test.checks[4].Done,
        "the post errand junk sale never masks the defect")
}
