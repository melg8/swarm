// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"strings"
	"time"
)

// The temp accounts of the scenarios: temp1, temp2 and temp3 with the
// matching passwords and character names. They never collide with the
// -bots fleet (which derives test1, test2, ... from the base
// account).
const (
	farmAccount   = "temp1"
	farmPassword  = "temp1"
	lifeAccount   = "temp2"
	lifePassword  = "temp2"
	relayAccount  = "temp3"
	relayPassword = "temp3"
)

// Scenario timeouts: the farm readiness needs the shopping, the
// lessons, the walk and the kill - the aggressive mobs on the village
// road interrupt the trips (the emergency logout reconnects and
// retries), a quarter of an hour measured too tight for the full
// cycle, half an hour bounds a slow economy with room to spare. The
// lifetime and relay scenarios are short hops.
const (
	farmTimeout = 30 * time.Minute
	lifeTimeout = 4 * time.Minute
)

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
