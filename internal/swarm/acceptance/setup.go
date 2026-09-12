// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"fmt"
	"strconv"
	"time"
)

// The injected elven fighter start state of the farm readiness
// scenario: the first creation node of the ElvenFighter player
// template (dist/game/data/stats/players/templates/StartingClass/
// ElvenFighter.xml creationPoints) so every run starts exactly where
// a freshly created elven fighter would, with the vitals of the level
// table (280 hp / 111 mp / 112 cp at level 15) and the level start
// experience of the C1 table (254327, see state/experience.go).
const (
	elvenSpawnX = 46045
	elvenSpawnY = 41251
	elvenSpawnZ = -3440
	level15Exp  = 254327
	level15HP   = 280
	level15MP   = 111
	level15CP   = 112
)

// adenaItemID is the adena item of the C1 item table.
const adenaItemID = 57

// ResetItem is one injected inventory stack of the start state: the
// template id and the count of the pile the character wakes up with.
type ResetItem struct {
	ItemID int32
	Count  int32
}

// adenaObjectIDBase is the object id of the injected adena stack. It
// stays below the FIRST_OBJECT_ID (268435456) of the server IdManager
// on purpose: the running server allocates every new world object id
// from that range, so the injected rows can never collide with it,
// and the IdManager restart scan skips the below range ids anyway.
const adenaObjectIDBase = 100000000

// onlinePollPeriod and onlinePollTimeout bound the wait for the
// server side flush after a session stops: the logout store runs
// immediately for a calm character, but a character that left in
// combat is stored fifteen seconds after the combat ends, and the
// reset must not race that flush (it would overwrite the injection).
const (
	onlinePollPeriod  = time.Second
	onlinePollTimeout = 45 * time.Second
)

// characterReset is the injected start state of one temp character.
type characterReset struct {
	Account string
	Char    string
	Level   int32
	Exp     int64
	SP      int64
	Adena   int64
	// Items are the injected inventory stacks (no object ids of their
	// own: the reset derives stable ones below the server range). The
	// character equips the wearable pieces through the auto equipment
	// of the hunt loop - the injection lands everything in the bag.
	Items []ResetItem
	X     int32
	Y     int32
	Z     int32
	MaxHP int32
	MaxMP int32
	MaxCP int32
}

// farmReadinessReset returns the level 15 start state of the user
// scenario: 20,000 SP, 100,000 adena, the creation spawn point.
func farmReadinessReset(account string) characterReset {
	return characterReset{
		Account: account,
		Char:    account,
		Level:   15,
		Exp:     level15Exp,
		SP:      20000,
		Adena:   100000,
		X:       elvenSpawnX,
		Y:       elvenSpawnY,
		Z:       elvenSpawnZ,
		MaxHP:   level15HP,
		MaxMP:   level15MP,
		MaxCP:   level15CP,
	}
}

// The dump start state of the zone return scenario (the 2026-09-12
// 01:50 stuck report, build 2149ad1): the character woke at the elven
// village main street cell next to Herbiel - the position the frozen
// return left it standing on for over an hour - as the level 14
// fighter of the report with the exact inventory the dump carried.
const (
	// zoneReturnSpawnX/Y/Z is the reported stuck position (the dump of
	// 2026-09-12 01:50, the village respawn cell of test1).
	zoneReturnSpawnX = 43048
	zoneReturnSpawnY = 50312
	zoneReturnSpawnZ = -2992
	// The vitals and the wallet of the dump character.
	zoneReturnExp   = 192206
	zoneReturnSP    = 7549
	zoneReturnAdena = 31857
	zoneReturnHP    = 339
	zoneReturnMP    = 137
	zoneReturnCP    = 105
)

// zoneReturnItems is the exact item set of the dump: the equipped
// paperdoll (the Brandish two hander, the wooden armor set, the
// starter jewels) and the bag of the report (the arrows, the potions,
// the recipes and the crafting materials). The injection lands every
// stack in the bag - the auto equipment of the hunt loop dresses the
// character from it, the wearable pieces are the ones the dump wore.
var zoneReturnItems = []ResetItem{
	{ItemID: 1333, Count: 1}, // Brandish (the two hand sword of the dump)
	{ItemID: 23, Count: 1},   // Wooden Breastplate
	{ItemID: 31, Count: 1},   // Bone Gaiters
	{ItemID: 44, Count: 1},   // Leather Helmet
	{ItemID: 50, Count: 1},   // Leather Gloves
	{ItemID: 1121, Count: 1}, // Apprentice's Shoes
	{ItemID: 114, Count: 1},  // Earring of Strength
	{ItemID: 115, Count: 1},  // Earring of Wisdom
	{ItemID: 876, Count: 2},  // Ring of Anguish
	{ItemID: 907, Count: 1},  // Necklace of Anguish
	{ItemID: 17, Count: 65},  // Wooden Arrow
	{ItemID: 1060, Count: 3}, // Lesser Healing Potion
	{ItemID: 1103, Count: 1}, // Cotton Stockings
	{ItemID: 1795, Count: 8}, // Recipe: Leather Shoes
	{ItemID: 1798, Count: 2}, // Recipe: Leather Helmet
	{ItemID: 1831, Count: 4}, // Antidote
	{ItemID: 1833, Count: 2}, // Bandage
	{ItemID: 1864, Count: 8}, // Stem
	{ItemID: 1866, Count: 4}, // Suede
	{ItemID: 1867, Count: 9}, // Animal Skin
	{ItemID: 1869, Count: 1}, // Iron Ore
	{ItemID: 1870, Count: 5}, // Coal
	{ItemID: 2005, Count: 1}, // Broadsword Blade
	{ItemID: 2008, Count: 1}, // Cedar Staff Head
	{ItemID: 2010, Count: 1}, // Brandish Blade
}

// zoneReturnReset returns the dump start state of the zone return
// scenario: the reported stuck cell, the reported level and vitals,
// the reported wallet and the reported item set.
func zoneReturnReset(account string) characterReset {
	return characterReset{
		Account: account,
		Char:    account,
		Level:   14,
		Exp:     zoneReturnExp,
		SP:      zoneReturnSP,
		Adena:   zoneReturnAdena,
		Items:   zoneReturnItems,
		X:       zoneReturnSpawnX,
		Y:       zoneReturnSpawnY,
		Z:       zoneReturnSpawnZ,
		MaxHP:   zoneReturnHP,
		MaxMP:   zoneReturnMP,
		MaxCP:   zoneReturnCP,
	}
}

// The dump start state of the gear gap scenario (the 2026-09-12
// 04:58 pantsless report, build 4deb888): the level 14 fighter test2
// stood at its Spore Fungus SW farm spot wearing every slot EXCEPT
// the legs - the town trip had sold the piece for a replacement that
// never landed - with the 13162 adena of the report and nothing in
// the bag. The scenario verifies the recovery invariant: the hunt
// loop detects the hole, shops and dresses it.
const (
	// gearGapSpawnX/Y/Z is the reported farm spot position (the dump
	// of 2026-09-12 04:58, inside the Spore Fungus SW zone).
	gearGapSpawnX = 38344
	gearGapSpawnY = 46248
	gearGapSpawnZ = -3592
	// The vitals, the wallet and the experience of the dump character.
	gearGapExp   = 247995
	gearGapSP    = 2977
	gearGapAdena = 13162
	gearGapHP    = 339
	gearGapMP    = 137
	gearGapCP    = 105
)

// gearGapItems is the exact item set of the dump: the equipped
// paperdoll minus the legs piece the trip stranded (the injection
// lands every stack in the bag - the auto equipment of the hunt loop
// dresses the character from it, the wearable pieces are the ones the
// dump wore).
var gearGapItems = []ResetItem{
	{ItemID: 20, Count: 1},  // Buckler
	{ItemID: 22, Count: 1},  // Leather Shirt
	{ItemID: 37, Count: 1},  // Leather Shoes
	{ItemID: 43, Count: 1},  // Wooden Helmet
	{ItemID: 49, Count: 1},  // Gloves
	{ItemID: 112, Count: 1}, // Apprentice's Earring
	{ItemID: 112, Count: 1}, // Apprentice's Earring
	{ItemID: 116, Count: 1}, // Magic Ring
	{ItemID: 116, Count: 1}, // Magic Ring
	{ItemID: 118, Count: 1}, // Necklace of Magic
	{ItemID: 153, Count: 1}, // Sickle
}

// gearGapReset returns the dump start state of the gear gap
// scenario: the reported farm spot, the reported level and vitals,
// the reported wallet and the reported pantsless item set.
func gearGapReset(account string) characterReset {
	return characterReset{
		Account: account,
		Char:    account,
		Level:   14,
		Exp:     gearGapExp,
		SP:      gearGapSP,
		Adena:   gearGapAdena,
		Items:   gearGapItems,
		X:       gearGapSpawnX,
		Y:       gearGapSpawnY,
		Z:       gearGapSpawnZ,
		MaxHP:   gearGapHP,
		MaxMP:   gearGapMP,
		MaxCP:   gearGapCP,
	}
}

// The start state of the building entry scenario (the character of
// the 2026-09-12 03:56 trainer hall freeze dump, standing at its walk
// plan origin): the level 15 elven fighter wakes at the west aisle
// entrance of the trainer hall with the wallet and the empty bag the
// dump character carried when its town visit began - the weapon run
// trip arms at once (the dump's own first trip: "no weapon in hand,
// the weapon run comes first, 33 lessons worth 18690 sp wait at the
// teacher"), buys the gear and the spellbooks across the village
// merchants and then walks the teacher leg from the last stop into
// the trainer hall - the exact leg the dump froze on.
const (
	// entrySpawnX/Y/Z is the dump walk plan origin: the aisle
	// entrance of the elven village trainer hall.
	entrySpawnX = 44744
	entrySpawnY = 51992
	entrySpawnZ = -2792
	// The wallet and the vitals of the dump character at its town
	// visit start: the shopping budget of the weapon run round.
	entrySP    = 20000
	entryAdena = 100000
	entryHP    = 358
	entryMP    = 145
	entryCP    = 178
)

// buildingEntryReset returns the dump start state of the building
// entry scenario: the aisle entrance position, the reported level and
// vitals, the reported wallet and the reported item set.
func buildingEntryReset(account string) characterReset {
	return characterReset{
		Account: account,
		Char:    account,
		Level:   15,
		Exp:     level15Exp,
		SP:      entrySP,
		Adena:   entryAdena,
		Items:   nil,
		X:       entrySpawnX,
		Y:       entrySpawnY,
		Z:       entrySpawnZ,
		MaxHP:   entryHP,
		MaxMP:   entryMP,
		MaxCP:   entryCP,
	}
}

// waitCharacterOffline polls the characters row until the server
// flushed the session state (the online flag drops with the store on
// logout). The timeout case continues with a warning instead of
// failing the run: a crashed server leaves the flag set, and the
// reset UPDATE itself clears it.
func waitCharacterOffline(db *DB, charID int64, log func(string)) {
	deadline := time.Now().Add(onlinePollTimeout)
	query := "SELECT online FROM characters WHERE charId=" +
		strconv.FormatInt(charID, 10)
	for time.Now().Before(deadline) {
		rows, err := db.Query(query)
		if err == nil && len(rows) == 1 && rows[0][0] == "0" {
			return
		}
		if err != nil {
			log("acceptance: the offline poll failed: " + err.Error())

			return
		}
		time.Sleep(onlinePollPeriod)
	}
	log("acceptance: the character stayed marked online, resetting anyway")
}

// resetCharacter wipes the temp character back to the injected start
// state. The character must exist and be offline: the caller creates
// it through the game protocol first (a level 1 elven fighter with
// the starter gear) and waits for the server flush. The reset clears
// the inventory, the learned skills, the saved effects, the shortcuts
// and the reuse stamps, injects the adena stack and rewrites the
// vitals, the level, the experience, the SP and the spawn position.
func resetCharacter(db *DB, reset characterReset, log func(string)) error {
	charID, err := characterID(db, reset)
	if err != nil {
		return err
	}
	waitCharacterOffline(db, charID, log)

	// The owned rows: the inventory (with the adena restack), the
	// learned skills, the saved effects, the shortcuts and the item
	// reuse stamps. A character that never entered the world has none
	// of them, the deletes stay idempotent.
	if _, err := db.Exec("DELETE FROM items WHERE owner_id=" +
		strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("wipe items: %w", err)
	}
	if _, err := db.Exec("DELETE FROM character_skills WHERE charId=" +
		strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("wipe skills: %w", err)
	}
	if _, err := db.Exec("DELETE FROM character_skills_save WHERE charId=" +
		strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("wipe saved effects: %w", err)
	}
	if _, err := db.Exec("DELETE FROM character_shortcuts WHERE charId=" +
		strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("wipe shortcuts: %w", err)
	}
	if _, err := db.Exec(
		"DELETE FROM character_item_reuse_save WHERE charId=" +
			strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("wipe item reuse: %w", err)
	}

	// The injected adena stack: the fixed object id keeps the insert
	// idempotent (a re-run deletes the rows above first).
	adena := strconv.FormatInt(reset.Adena, 10)
	objectID := strconv.FormatInt(charID+adenaObjectIDBase, 10)
	if _, err := db.Exec(
		"INSERT INTO items (owner_id, object_id, item_id, count, loc, " +
			"loc_data) VALUES (" + strconv.FormatInt(charID, 10) + ", " +
			objectID + ", " + strconv.Itoa(adenaItemID) + ", " + adena +
			", 'INVENTORY', 0)"); err != nil {
		return fmt.Errorf("inject adena: %w", err)
	}

	// The injected inventory stacks: the derived object ids stay
	// below the FIRST_OBJECT_ID range like the adena stack (the
	// wipe above keeps the numbering idempotent on a re-run).
	for i, item := range reset.Items {
		itemObjectID := strconv.FormatInt(
			charID+adenaObjectIDBase+int64(i)+1, 10)
		if _, err := db.Exec(
			"INSERT INTO items (owner_id, object_id, item_id, count, " +
				"loc, loc_data) VALUES (" +
				strconv.FormatInt(charID, 10) + ", " + itemObjectID +
				", " + strconv.Itoa(int(item.ItemID)) + ", " +
				strconv.Itoa(int(item.Count)) +
				", 'INVENTORY', 0)"); err != nil {
			return fmt.Errorf("inject item %d: %w", item.ItemID, err)
		}
	}

	if _, err := db.Exec(
		"UPDATE characters SET level=" + strconv.Itoa(int(reset.Level)) +
			", exp=" + strconv.FormatInt(reset.Exp, 10) +
			", sp=" + strconv.FormatInt(reset.SP, 10) +
			", x=" + strconv.Itoa(int(reset.X)) +
			", y=" + strconv.Itoa(int(reset.Y)) +
			", z=" + strconv.Itoa(int(reset.Z)) +
			", heading=0, karma=0, online=0" +
			", maxHp=" + strconv.Itoa(int(reset.MaxHP)) +
			", curHp=" + strconv.Itoa(int(reset.MaxHP)) +
			", maxMp=" + strconv.Itoa(int(reset.MaxMP)) +
			", curMp=" + strconv.Itoa(int(reset.MaxMP)) +
			", maxCp=" + strconv.Itoa(int(reset.MaxCP)) +
			", curCp=" + strconv.Itoa(int(reset.MaxCP)) +
			" WHERE charId=" + strconv.FormatInt(charID, 10)); err != nil {
		return fmt.Errorf("inject vitals: %w", err)
	}

	return nil
}

// characterID resolves the character row id of the temp account.
func characterID(db *DB, reset characterReset) (int64, error) {
	rows, err := db.Query(
		"SELECT charId FROM characters WHERE account_name='" +
			reset.Account + "' AND char_name='" + reset.Char + "'")
	if err != nil {
		return 0, fmt.Errorf("resolve character: %w", err)
	}
	if len(rows) != 1 {
		return 0, fmt.Errorf("character %s not found on account %s",
			reset.Char, reset.Account)
	}
	id, err := strconv.ParseInt(rows[0][0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse character id %q: %w", rows[0][0], err)
	}

	return id, nil
}
