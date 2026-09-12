// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The zone return scenario tests: the round 58 acceptance round of the
// 2026-09-12 freeze dump (build 2149ad1, bot test1, phase engage) - the
// character woke on the reported stuck village cell with the reported
// item set and must walk to its selected hunting zone on its own.

// newZoneReturnBot builds an online tracker at the dump start state:
// the level 14 fighter on the reported stuck cell.
func newZoneReturnBot() *state.Bot {
	bot := state.NewBot(returnAccount)
	bot.SetCharacter(returnAccount, 100, 18, zoneReturnSpawnX,
		zoneReturnSpawnY, zoneReturnSpawnZ, zoneReturnHP, zoneReturnMP)
	bot.SetOnline(returnAccount)
	bot.ApplyUserInfo(state.UserInfo{
		Name:    returnAccount,
		Level:   14,
		ClassID: 18,
		X:       zoneReturnSpawnX,
		Y:       zoneReturnSpawnY,
		Z:       zoneReturnSpawnZ,
		MaxHP:   zoneReturnHP,
		CurHP:   zoneReturnHP,
		MaxMP:   zoneReturnMP,
		CurMP:   zoneReturnMP,
	})

	return bot
}

// TestZoneReturnDefinition pins the scenario registration: the stuck
// cell round owns temp4, runs under the quarter hour bound and carries
// the dump story.
func TestZoneReturnDefinition(t *testing.T) {
	var found bool
	for _, def := range Definitions() {
		if def.ID != "zone-return" {
			continue
		}
		found = true
		require.Equal(t, returnAccount, def.Account)
		require.Equal(t, zoneReturnTimeout, def.Timeout)
		require.NotNil(t, def.Scenario)
		for _, needle := range []string{
			"43048 50312 -2992", "level 14", "Brandish",
			"31,857 adena", "hunting zone",
		} {
			require.Contains(t, def.Description, needle)
		}
	}
	require.True(t, found, "the zone-return scenario is registered")
}

// TestZoneReturnResetMatchesTheDump pins the injected start state of
// the report: the stuck cell position, the level, the experience, the
// wallet and the vitals of the dump character.
func TestZoneReturnResetMatchesTheDump(t *testing.T) {
	reset := zoneReturnReset(returnAccount)
	require.Equal(t, int32(14), reset.Level)
	require.Equal(t, int64(192206), reset.Exp)
	require.Equal(t, int64(7549), reset.SP)
	require.Equal(t, int64(31857), reset.Adena)
	require.Equal(t, int32(43048), reset.X)
	require.Equal(t, int32(50312), reset.Y)
	require.Equal(t, int32(-2992), reset.Z)
	require.Equal(t, int32(339), reset.MaxHP)
	require.Equal(t, int32(137), reset.MaxMP)
	require.NotEmpty(t, reset.Items,
		"the dump inventory rides along the reset")
}

// TestZoneReturnItemsMatchTheDump pins the exact inventory of the
// report: the wearables of the paperdoll (the two hand sword, the
// wooden armor set, the starter jewels - the two rings as one stack)
// and the bag of the freeze dump. The adena stack is injected by the
// dedicated column of the reset, never duplicated in the item list.
func TestZoneReturnItemsMatchTheDump(t *testing.T) {
	require.Len(t, zoneReturnItems, 25)
	counts := map[int32]int32{}
	for _, item := range zoneReturnItems {
		counts[item.ItemID] += item.Count
		require.Positive(t, item.Count)
	}
	// The equipped paperdoll of the dump: the Brandish two hander, the
	// wooden armor set, the starter jewels (both anguish rings).
	require.Equal(t, int32(1), counts[1333], "Brandish")
	require.Equal(t, int32(1), counts[23], "Wooden Breastplate")
	require.Equal(t, int32(1), counts[31], "Bone Gaiters")
	require.Equal(t, int32(1), counts[44], "Leather Helmet")
	require.Equal(t, int32(1), counts[50], "Leather Gloves")
	require.Equal(t, int32(1), counts[1121], "Apprentice's Shoes")
	require.Equal(t, int32(1), counts[114], "Earring of Strength")
	require.Equal(t, int32(1), counts[115], "Earring of Wisdom")
	require.Equal(t, int32(2), counts[876], "the Ring of Anguish pair")
	require.Equal(t, int32(1), counts[907], "Necklace of Anguish")
	// The bag of the dump: the arrows, the potions, the recipes and the
	// crafting pile.
	require.Equal(t, int32(65), counts[17], "Wooden Arrow")
	require.Equal(t, int32(3), counts[1060], "Lesser Healing Potion")
	require.Equal(t, int32(1), counts[1103], "Cotton Stockings")
	require.Equal(t, int32(8), counts[1795], "Recipe: Leather Shoes")
	require.Equal(t, int32(2), counts[1798], "Recipe: Leather Helmet")
	require.Equal(t, int32(4), counts[1831], "Antidote")
	require.Equal(t, int32(2), counts[1833], "Bandage")
	require.Equal(t, int32(8), counts[1864], "Stem")
	require.Equal(t, int32(4), counts[1866], "Suede")
	require.Equal(t, int32(9), counts[1867], "Animal Skin")
	require.Equal(t, int32(1), counts[1869], "Iron Ore")
	require.Equal(t, int32(5), counts[1870], "Coal")
	require.Equal(t, int32(1), counts[2005], "Broadsword Blade")
	require.Equal(t, int32(1), counts[2008], "Cedar Staff Head")
	require.Equal(t, int32(1), counts[2010], "Brandish Blade")
	// The adena stack never rides the item list: the reset injects it
	// through its own column with its fixed object id.
	require.NotContains(t, counts, adenaItemID)
}

// TestZoneReturnChecksCoverTheStory pins the check list of the stuck
// cell scenario: the world entry plus the walk into the selected
// hunting zone.
func TestZoneReturnChecksCoverTheStory(t *testing.T) {
	checks := zoneReturnChecks()
	require.Len(t, checks, 2)
	require.Equal(t, checkOnline, checks[0].ID)
	require.Equal(t, checkZone, checks[1].ID)
	for _, check := range checks {
		require.False(t, check.Done)
		require.NotEmpty(t, check.Label)
	}
}

// TestEvaluateZoneReturnConditions pins the pass condition of the
// scenario: the character standing inside the hunting zone its own
// spot economy selected flips the zone check, a character anywhere
// else (the dump signature: frozen on the village cell) keeps it open.
func TestEvaluateZoneReturnConditions(t *testing.T) {
	bot := newZoneReturnBot()
	// The dump zone: the Kaboo Orc Fighter SW leash (the round 58
	// report), the character 7900 units away from it.
	bot.SetHuntingZone(35214, 51358, 1448)
	test := &Test{}
	test.setChecks(zoneReturnChecks())

	evaluateZoneReturnConditions(bot, test)
	require.False(t, test.checks[1].Done,
		"the stuck cell is far outside the zone")

	bot.ApplyUserInfo(state.UserInfo{
		Name: returnAccount, Level: 14, ClassID: 18,
		X: 35214, Y: 51358, Z: zoneReturnSpawnZ,
	})
	evaluateZoneReturnConditions(bot, test)
	require.True(t, test.checks[1].Done,
		"standing on the zone anchor passes the scenario")

	// The offline tracker never evaluates: the early session and the
	// reconnect windows keep the check untouched.
	offline := state.NewBot(returnAccount)
	offline.SetHuntingZone(35214, 51358, 1448)
	offline.ApplyUserInfo(state.UserInfo{
		Name: returnAccount, Level: 14, ClassID: 18,
		X: 35214, Y: 51358, Z: zoneReturnSpawnZ,
	})
	test2 := &Test{}
	test2.setChecks(zoneReturnChecks())
	evaluateZoneReturnConditions(offline, test2)
	require.False(t, test2.checks[1].Done)
}

// TestResetCharacterInjectsDumpItems pins the database half of the
// dump state injection: every stack of the report lands as its own
// items row with a derived object id below the server range, the adena
// stack keeps its fixed slot and the wearables land in the bag (the
// auto equipment of the hunt loop dresses the character from it).
func TestResetCharacterInjectsDumpItems(t *testing.T) {
	server := startFakeDBServer(t)
	const charID = 7
	server.answers["SELECT charId FROM characters WHERE "+
		"account_name='temp4' AND char_name='temp4'"] = [][]string{
		{strconv.FormatInt(charID, 10)},
	}
	server.answers["SELECT online FROM characters WHERE charId=7"] = [][]string{{"0"}}
	db := connectFake(t, server)

	// The served queries land in a buffered channel of 16: the reset
	// sends far more (the wipes, the adena and the 25 item inserts), so
	// a drain goroutine collects them for the assertions.
	served := make([]string, 0, 40)
	drained := make(chan struct{})
	go func() {
		for query := range server.queries {
			served = append(served, query)
		}
		close(drained)
	}()

	require.NoError(t, resetCharacter(db, zoneReturnReset("temp4"),
		func(string) {}))
	close(server.queries)
	<-drained

	// Every stack of the dump inventory is its own insert with its own
	// derived object id, the adena included.
	adena := "INSERT INTO items (owner_id, object_id, item_id, count, " +
		"loc, loc_data) VALUES (7, " +
		strconv.FormatInt(charID+adenaObjectIDBase, 10) + ", 57, 31857, " +
		"'INVENTORY', 0)"
	require.Contains(t, served, adena, "the adena stack of the dump")
	for i, item := range zoneReturnItems {
		insert := "INSERT INTO items (owner_id, object_id, item_id, " +
			"count, loc, loc_data) VALUES (7, " +
			strconv.FormatInt(charID+adenaObjectIDBase+int64(i)+1, 10) +
			", " + strconv.Itoa(int(item.ItemID)) + ", " +
			strconv.Itoa(int(item.Count)) + ", 'INVENTORY', 0)"
		require.Contains(t, served, insert,
			"the dump stack "+strconv.Itoa(int(item.ItemID)))
	}
	// The derived object ids never reach the server range.
	require.Less(t, charID+adenaObjectIDBase+int64(len(zoneReturnItems))+1,
		int64(268435456))
	// The character row rewrite carries the dump position and vitals.
	var vitals bool
	for _, query := range served {
		if strings.HasPrefix(query, "UPDATE characters SET level=14") &&
			strings.Contains(query, "x=43048, y=50312, z=-2992") &&
			strings.Contains(query, "maxHp=339") &&
			strings.Contains(query, "maxMp=137") {
			vitals = true
		}
	}
	require.True(t, vitals, "the characters row carries the dump state")
	// The wipe runs before the injection: the items delete precedes the
	// first insert.
	var wipe, firstInsert int
	for i, query := range served {
		if strings.HasPrefix(query, "DELETE FROM items WHERE owner_id=7") {
			wipe = i
		}
		if strings.HasPrefix(query, "INSERT INTO items") && firstInsert == 0 {
			firstInsert = i
		}
	}
	require.Less(t, wipe, firstInsert,
		"the inventory wipe precedes the injection")
}

// TestZoneReturnTimeoutBoundsTheSlowWalk pins the timeout constant:
// the dump zone sat 7900 units away and the delevel detour of the
// report burned its own minutes - a quarter of an hour bounds the
// slowest honest walk with room to spare.
func TestZoneReturnTimeoutBoundsTheSlowWalk(t *testing.T) {
	require.Equal(t, 15*time.Minute, zoneReturnTimeout)
	require.LessOrEqual(t, zoneReturnTimeout, farmTimeout,
		"the stuck cell walk stays inside the farm readiness bound")
	require.GreaterOrEqual(t, zoneReturnTimeout, 10*time.Minute,
		"the far walk with its detours needs the headroom")
}
