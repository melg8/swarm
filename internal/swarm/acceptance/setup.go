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
	X       int32
	Y       int32
	Z       int32
	MaxHP   int32
	MaxMP   int32
	MaxCP   int32
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
