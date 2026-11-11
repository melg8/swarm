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
            require.Contains(t, test.checks[6].Detail,
                testCase.wantDetail)
            require.Equal(t, checkCityShopFarm, test.checks[6].ID)
            require.False(t, test.checks[6].Done)
        })
    }
}
