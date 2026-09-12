// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"strings"
	"time"
)

// The temp accounts of the scenarios: temp1 through temp6 with the
// matching passwords and character names. They never collide with the
// -bots fleet (which derives test1, test2, ... from the base
// account).
const (
	farmAccount    = "temp1"
	farmPassword   = "temp1"
	lifeAccount    = "temp2"
	lifePassword   = "temp2"
	relayAccount   = "temp3"
	relayPassword  = "temp3"
	returnAccount  = "temp4"
	returnPassword = "temp4"
	gearAccount    = "temp5"
	gearPassword   = "temp5"
	entryAccount   = "temp6"
	entryPassword  = "temp6"
)

// Scenario timeouts: the farm readiness runs the one town visit
// round - the weapon, the pdef maximizing armor set, the basic jewel
// set, the spellbooks and the lessons all land in a single village
// walk, then the bot crosses to its farm zone and kills its first
// mob under the auras. The measured duration of the fixed flow is
// ~8 minutes (472 s and 481 s on the local stack, the lessons pacing
// dominates); the bound holds two and a half times that so a live
// run whose mob positions, walk retries or road fights drift slower
// still fits - the aggressive mobs on the village road interrupt the
// trips (the emergency logout reconnects and retries) and the zone
// kill may wait out a respawn. The lifetime and relay scenarios are
// short hops.
const (
	farmTimeout = 20 * time.Minute
	lifeTimeout = 4 * time.Minute
)

// zoneReturnTimeout bounds the stuck cell scenario: the walk from the
// village street to the far hunting grounds covers several thousand
// units of geodata route (the dump zone sat 7900 units away), and a
// shopping detour or two may ride along - a quarter of an hour keeps
// the slowest honest run inside the bound.
const zoneReturnTimeout = 15 * time.Minute

// gearGapTimeout bounds the pantsless dump scenario: the recovery is
// one town trip (the walk to the village, the filler buy, the walk
// back) that a road fight or a relogin may stretch - the aggressive
// Kaboo packs of the dump surroundings interrupt the walks the same
// way they interrupted the trip that caused the report. A quarter of
// an hour keeps the slowest honest run inside the bound.
const gearGapTimeout = 15 * time.Minute

// buildingEntryTimeout bounds the trainer hall entry scenario: the
// weapon run town round - the weapon, the armor, the jewels, the
// spellbooks and the teacher leg into the hall - measured ~2.2
// minutes on the live stack run (the dump's own town visit), the
// bound leaves room for the walk retries and the aggressive road
// mobs of the village surroundings.
const buildingEntryTimeout = 9 * time.Minute

// lifeOnlineTime is how long the lifetime scenario keeps the bot in
// the world: enough to observe a stable session (the mirrors of
// tools/mobius_e2e.sh stay 45 seconds), short enough to keep the
// sequential run of all tests snappy.
const lifeOnlineTime = 30 * time.Second

// The buff skills of the elven fighter: the auras the final state of
// the farm scenario demands (Attack Aura 77, Defence Aura 91).
const (
	attackAuraSkillID  = 77
	defenseAuraSkillID = 91
)

// Definitions returns the acceptance scenario list: the farm
// readiness round of the user, plus the in-process mirrors of the two
// existing e2e harnesses (tools/mobius_e2e.sh and tools/proxy_e2e.sh)
// so the whole acceptance suite runs from one place.
func Definitions() []TestDef {
	return []TestDef{
		{
			ID:      classTransferScenarioID,
			Title:   "class transfer · the M2 accept stage",
			Account: classTransferAccount,
			Timeout: classTransferTimeout,
			Description: "Start: the elven fighter temp9 is injected at " +
				"level 19 (the Q00406 start gate) with the milestone " +
				"wallet shape, standing on the approach ring of Master " +
				"Sorius in Gludio (-13440 122493 -3103, 150 units off " +
				"his trainer hall cell). Flow: the manual session " +
				"enters the world, finds Sorius through the world " +
				"store and the dialog walker drives the two-link " +
				"accept route of the chain data (the quest pages of " +
				"the CREATED state - the challenge link runs the " +
				"startQuest event). Pass (the staged gate of the M2 " +
				"vehicle): the quest journal flips to Q00406 cond 1 " +
				"within the budget; the kill stages (the quest trip " +
				"phase) and the Rains class change join in the " +
				"follow-up rounds, the SelfClassID flip closes M2.",
			Scenario: classTransferScenario,
		},
		{
			ID:      milestoneScenarioID,
			Title:   "level milestone · N to N+1",
			Account: milestoneAccount,
			Timeout: milestoneTimeout,
			Description: "Start: the elven fighter temp8 is injected at " +
				"level N (SWARM_LEVEL_MILESTONE_LEVEL, default 10) " +
				"with the near-threshold experience (one twentieth of " +
				"the level span below the N+1 threshold), the template " +
				"vitals of the level, 20,000 SP, 100,000 adena and an " +
				"empty bag, standing at the creation spawn point of the " +
				"elven village. Flow: the bot runs the weapon run town " +
				"round (the gear, the spellbooks, the lessons), walks " +
				"to its band hunting ground through the zone ladder " +
				"and farms the last kills to the level-up. Pass: the " +
				"observed level reaches N+1 within the time budget " +
				"(the UserInfo the server broadcasts on the level-up " +
				"refreshes the tracker); the metrics trail in " +
				"runs/metrics.jsonl receives one row either way.",
			Scenario: levelMilestoneScenario,
		},
		{
			ID:      soakScenarioID,
			Title:   "soak · the M1 metrics trail",
			Account: soakAccount,
			Timeout: soakTimeout(),
			Description: "Start: a fresh level 1 elven fighter temp7 " +
				"enters the world (no database injection, the login " +
				"auto-creates the account). Flow: the supervised hunt " +
				"loop farms the elven lands for the configured window " +
				"(SWARM_SOAK_MINUTES, default the 10 minute smoke; the " +
				"real M1 run sets 480 for the 8 hour proof), the " +
				"stagnation guard watches for no XP gain for M minutes " +
				"and no position change for K minutes, the lost " +
				"sessions reconnect the way the 24/7 supervisor does. " +
				"Pass: the bot stayed online the whole window, the " +
				"stagnation guard never fired and the session ended " +
				"gracefully; one JSON line lands in runs/metrics.jsonl " +
				"either way (date, scenario, duration, start/end level, " +
				"XP per hour, deaths, adena, stuck events, PASS/FAIL).",
			Scenario: soakScenario,
		},
		{
			ID:      "farm-readiness",
			Title:   "farm readiness · level 15",
			Account: farmAccount,
			Timeout: farmTimeout,
			Description: "Start: the elven fighter temp1 spawns at the " +
				"character creation point of the elven village (46045 " +
				"41251 -3440) as level 15 with 20,000 SP, 100,000 adena, " +
				"an empty inventory and no learned skills. Flow: the bot " +
				"walks the village, buys a proper gear set and the " +
				"demanded spellbooks, learns every lesson its SP " +
				"affords (Attack Aura and Defence Aura included), then " +
				"leaves the town for its hunting zone. Pass: the bot " +
				"wears a weapon with armor and jewels, has no " +
				"affordable lesson left, stands inside the farm zone " +
				"under the attack and defence auras and has killed at " +
				"least one mob there after this run started.",
			Scenario: farmReadinessScenario,
		},
		{
			ID:      "building-entry",
			Title:   "building entry · the teacher hall walk",
			Account: entryAccount,
			Timeout: buildingEntryTimeout,
			Description: "Start: the elven fighter temp5 wakes at the " +
				"trainer hall west aisle entrance (44744 51992 -2792, the " +
				"freeze cell of the 2026-09-12 03:56 dump) as the level 15 " +
				"character of the report - 20,000 SP, 100,000 adena and an " +
				"empty inventory, exactly the state the dump's town visit " +
				"began from. Flow: the weapon run trip arms at once, buys " +
				"the gear and the spellbooks across the village merchants " +
				"and then walks the teacher leg from the last stop through " +
				"the building entrance right up to the class master Ellenia " +
				"inside the hall, and the lessons begin. Pass: the character " +
				"stands within the interaction distance of Ellenia and at " +
				"least one lesson consumed SP (the dump freeze held the " +
				"character at the entrance forever - the frozen corridor " +
				"ban, the detour re-plan and the direct server routed walk " +
				"own the recovery, the reproduction lives in " +
				"hunt/building_entry_test.go).",
			Scenario: buildingEntryScenario,
		},
		{
			ID:      "zone-return",
			Title:   "zone return · stuck dump cell",
			Account: returnAccount,
			Timeout: zoneReturnTimeout,
			Description: "Start: the elven fighter temp4 wakes at the " +
				"reported stuck cell of the 2026-09-12 freeze dump " +
				"(43048 50312 -2992, the elven village street next to " +
				"Herbiel) as the level 14 character of the report with " +
				"the exact inventory it carried - the Brandish sword, " +
				"the wooden armor set, the starter jewels, the arrows, " +
				"the potions, the recipes and the crafting pile, " +
				"31,857 adena and 7,549 sp. Flow: the bot dresses " +
				"itself, picks its hunting zone and walks there " +
				"through the village streets and the geodata route. " +
				"Pass: the bot stands inside its selected hunting " +
				"zone (the freeze of the report left it standing on " +
				"the village cell forever - the round 58 fix and its " +
				"reproduction live in hunt/round58_repro_test.go).",
			Scenario: zoneReturnScenario,
		},
		{
			ID:      "gear-gap",
			Title:   "gear gap refill · pantsless dump",
			Account: gearAccount,
			Timeout: gearGapTimeout,
			Description: "Start: the elven fighter temp5 wakes at the " +
				"reported farm spot of the 2026-09-12 04:58 pantsless " +
				"dump (38344 46248 -3592, the Spore Fungus SW ground) " +
				"as the level 14 character test2 of the report with " +
				"the exact paperdoll it carried - every slot filled " +
				"EXCEPT the legs (the town trip had sold the piece " +
				"for a replacement that never landed) - and the " +
				"13,162 adena of the report, nothing in the bag. " +
				"Flow: the bot picks its hunting zone, the shop " +
				"strategy plans the legs filler against the empty " +
				"slot and the town trip buys it. Pass: the legs slot " +
				"of the paperdoll is dressed again (the report's bot " +
				"farmed on without it - the round 60 gear debt fix " +
				"and its reproduction live in hunt/round60_repro_test.go).",
			Scenario: gearGapScenario,
		},
		{
			ID:      "bot-lifetime",
			Title:   "bot lifetime · world session",
			Account: lifeAccount,
			Timeout: lifeTimeout,
			Description: "Start: the elven fighter temp2 enters the world " +
				"as a fresh level 1 character (no database injection). " +
				"Flow: the in-process mirror of tools/mobius_e2e.sh - the " +
				"session logs in, enters the world, stays online for 30 " +
				"seconds and then shuts down the way the SIGINT path of " +
				"the e2e script does. Pass: the bot was online for the " +
				"whole 30 second window and the session ended gracefully " +
				"(the logout announced, no transport error).",
			Scenario: botLifetimeScenario,
		},
		{
			ID:      "proxy-relay",
			Title:   "proxy relay · C1 client path",
			Account: relayAccount,
			Timeout: relayTimeout,
			Description: "Start: the elven fighter temp3 enters the world " +
				"behind a dedicated client proxy bound to ephemeral ports " +
				"(the main -proxy listeners stay untouched). Flow: the " +
				"in-process mirror of tools/proxy_e2e.sh - a fake C1 " +
				"client logs into the emulated login server with arbitrary " +
				"credentials, sees the one character of the bot, enters " +
				"the world through the recorded replay, walks the bot " +
				"through the live relay and reads the locally answered net " +
				"pings. Pass: every step of the client path completed - " +
				"the login, the char list, the world replay, the movement " +
				"echo and the ping answers.",
			Scenario: proxyRelayScenario,
		},
	}
}

// AccountList renders the comma separated temp account list of the
// startup log line.
func AccountList() string {
	accounts := Definitions()
	if len(accounts) == 0 {
		return ""
	}
	var list strings.Builder
	list.WriteString(accounts[0].Account)
	for _, def := range accounts[1:] {
		list.WriteString(", ")
		list.WriteString(def.Account)
	}

	return list.String()
}

// DefinitionsIDs returns the ids of the registered scenarios in
// definition order. The CLI uses this for `-acceptance list` so an
// agent discovers the scenarios without a Manager instance (and
// without the temp bots landing in a registry).
func DefinitionsIDs() []string {
	defs := Definitions()
	ids := make([]string, 0, len(defs))
	for _, def := range defs {
		ids = append(ids, def.ID)
	}

	return ids
}
