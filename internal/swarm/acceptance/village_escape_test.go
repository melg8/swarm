// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The village escape scenario tests: the 2026-09-14 12:31 stuck dump
// (build 73fcfa6, bot test3, phase townReturn) - the character stood
// at the village plaza cell through a whole day of refused walks and
// must get out of the city within two minutes of the world entry.

// newVillageEscapeBot builds an online tracker at the dump start
// state: the level 15 fighter on the reported plaza cell.
func newVillageEscapeBot() *state.Bot {
    bot := state.NewBot(escapeAccount)
    bot.SetCharacter(escapeAccount, 100, 18, villageEscapeSpawnX,
        villageEscapeSpawnY, villageEscapeSpawnZ, villageEscapeHP,
        villageEscapeMP)
    bot.SetOnline(escapeAccount)
    bot.ApplyUserInfo(state.UserInfo{
        Name:    escapeAccount,
        Level:   15,
        ClassID: 18,
        X:       villageEscapeSpawnX,
        Y:       villageEscapeSpawnY,
        Z:       villageEscapeSpawnZ,
        MaxHP:   villageEscapeHP,
        CurHP:   villageEscapeHP,
        MaxMP:   villageEscapeMP,
        CurMP:   villageEscapeMP,
    })

    return bot
}

// TestVillageEscapeLeadsTheWebUIList pins the head of the scenario
// list: the user contract of the 2026-09-14 report - the acceptance
// test of the refused-click dump shows FIRST in the list the web UI
// serves, ahead of every other scenario.
func TestVillageEscapeLeadsTheWebUIList(t *testing.T) {
    defs := Definitions()
    require.NotEmpty(t, defs)
    require.Equal(t, "village-escape", defs[0].ID,
        "the village escape scenario owns the first list slot")
    require.Equal(t, escapeAccount, defs[0].Account)
    require.Equal(t, villageEscapeTimeout, defs[0].Timeout)
    require.NotNil(t, defs[0].Scenario)
    for _, needle := range []string{
        "45768 49848 -3056", "level 15", "Brandish",
        "1312 adena", "two minutes", "3000+ units",
    } {
        require.Contains(t, defs[0].Description, needle)
    }
}

// TestVillageEscapeResetMatchesTheDump pins the injected start state
// of the report: the plaza cell position, the level, the experience,
// the wallet and the vitals of the dump character.
func TestVillageEscapeResetMatchesTheDump(t *testing.T) {
    reset := villageEscapeReset(escapeAccount)
    require.Equal(t, int32(15), reset.Level)
    require.Equal(t, int64(villageEscapeExp), reset.Exp)
    require.Equal(t, int64(villageEscapeSP), reset.SP)
    require.Equal(t, int64(villageEscapeAdena), reset.Adena)
    require.Equal(t, int32(45768), reset.X)
    require.Equal(t, int32(49848), reset.Y)
    require.Equal(t, int32(-3056), reset.Z)
    require.Equal(t, int32(358), reset.MaxHP)
    require.Equal(t, int32(145), reset.MaxMP)
    require.NotEmpty(t, reset.Items,
        "the dump inventory rides along the reset")
}

// TestVillageEscapeItemsMatchTheDump pins the exact inventory of the
// report: the wearables of the paperdoll (the two hand sword, the
// bone armor set, the leather helmet and gloves, the starter jewels)
// and the bag of the dump (the arrows, the hunting bow, the two
// crafting leftovers). The adena stack is injected by the dedicated
// column of the reset, never duplicated in the item list.
func TestVillageEscapeItemsMatchTheDump(t *testing.T) {
    require.Len(t, villageEscapeItems, 15)
    counts := map[int32]int32{}
    for _, item := range villageEscapeItems {
        counts[item.ItemID] += item.Count
        require.Positive(t, item.Count)
    }
    // The equipped paperdoll of the dump: the Brandish two hander,
    // the bone armor set, the leather helmet and gloves, the starter
    // jewels (both magic rings).
    require.Equal(t, int32(1), counts[1333], "Brandish")
    require.Equal(t, int32(1), counts[24], "Bone Breastplate")
    require.Equal(t, int32(1), counts[31], "Bone Gaiters")
    require.Equal(t, int32(1), counts[38], "Low Boots")
    require.Equal(t, int32(1), counts[44], "Leather Helmet")
    require.Equal(t, int32(1), counts[50], "Leather Gloves")
    require.Equal(t, int32(2), counts[112], "the Apprentice's Earring pair")
    require.Equal(t, int32(2), counts[116], "the Magic Ring pair")
    require.Equal(t, int32(1), counts[118], "Necklace of Magic")
    // The bag of the dump: the arrows, the hunting bow and the two
    // crafting leftovers.
    require.Equal(t, int32(589), counts[17], "Wooden Arrow")
    require.Equal(t, int32(1), counts[271], "Hunting Bow")
    require.Equal(t, int32(1), counts[1866], "Suede")
    require.Equal(t, int32(1), counts[1867], "Animal Skin")
    // The adena stack never rides the item list: the reset injects it
    // through its own column with its fixed object id.
    require.NotContains(t, counts, adenaItemID)
}

// TestVillageEscapeChecksCoverTheStory pins the check list of the
// escape scenario: the village escape within the two minute window.
func TestVillageEscapeChecksCoverTheStory(t *testing.T) {
    checks := villageEscapeChecks()
    require.Len(t, checks, 1)
    require.Equal(t, checkEscape, checks[0].ID)
    require.False(t, checks[0].Done)
    require.NotEmpty(t, checks[0].Label)
}

// TestEvaluateVillageEscapeConditions pins the pass condition of the
// scenario: a character beyond the village radius passes the escape
// check, a character inside the city (the dump signature: frozen on
// the plaza cell) keeps it open. The radius covers every village
// structure of the geodata - the farthest dump npc stood 2734 units
// from the plaza - so a character 3000 units out stands on the open
// road past every wall, gate and deck of the city.
func TestEvaluateVillageEscapeConditions(t *testing.T) {
    bot := newVillageEscapeBot()
    test := &Test{}
    test.setChecks(villageEscapeChecks())

    escaped, detail := evaluateVillageEscape(bot, test)
    require.False(t, escaped)
    require.False(t, test.checks[0].Done)
    require.Contains(t, detail, "0 units")

    // The dump walk plan exits through the southwest gate corridor
    // (43512 50504, 2348 units out): still inside the city.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 43512, Y: 50504, Z: -2992,
    })
    escaped, _ = evaluateVillageEscape(bot, test)
    require.False(t, escaped, "the gate corridor is still the city")

    // The plan's fourth waypoint (40648 52680, 5100 units out) is the
    // open ground past the village: the escape flips.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 40648, Y: 52680, Z: -3224,
    })
    escaped, _ = evaluateVillageEscape(bot, test)
    require.True(t, escaped, "the open road past the village escapes")
    require.True(t, test.checks[0].Done)

    // The offline tracker never evaluates: the early session and the
    // reconnect windows keep the check untouched.
    offline := state.NewBot(escapeAccount)
    offline.ApplyUserInfo(state.UserInfo{
        Name: escapeAccount, Level: 15, ClassID: 18,
        X: 40648, Y: 52680, Z: -3224,
    })
    test2 := &Test{}
    test2.setChecks(villageEscapeChecks())
    escaped, _ = evaluateVillageEscape(offline, test2)
    require.False(t, escaped)
    require.False(t, test2.checks[0].Done)
}
