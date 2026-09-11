// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// monitorPeriod paces the condition evaluation of the running
// scenarios: every tick reads the tracker state and rewrites the
// check list.
const monitorPeriod = 2 * time.Second

// onlineWait bounds the wait for the world entry of a session: past
// it the scenario fails instead of hanging until the timeout.
const onlineWait = 90 * time.Second

// The farm readiness condition ids.
const (
	checkOnline   = "online"
	checkEquipped = "equipped"
	checkSkills   = "skills"
	checkZone     = "zone"
	checkBuffs    = "buffs"
	checkKill     = "kill"
)

// farmChecks is the initial check list of the farm readiness
// scenario: the narrative order of the user story.
func farmChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: "entered the world", Done: false,
			Detail: "",
		},
		{
			ID: checkEquipped, Label: "wears weapon, armor and jewels",
			Done: false, Detail: "",
		},
		{
			ID: checkSkills, Label: "learned every affordable lesson",
			Done: false, Detail: "",
		},
		{
			ID: checkZone, Label: "walked to the farm zone",
			Done: false, Detail: "",
		},
		{
			ID: checkBuffs, Label: "fights under attack and defence auras",
			Done: false, Detail: "",
		},
		{
			ID: checkKill, Label: "killed a mob on the farm zone",
			Done: false, Detail: "",
		},
	}
}

// lifetimeChecks is the check list of the bot lifetime scenario.
func lifetimeChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: "entered the world", Done: false,
			Detail: "",
		},
		{
			ID: "stable", Label: "stayed online 30 seconds", Done: false,
			Detail: "",
		},
		{
			ID: "graceful", Label: "shut down gracefully", Done: false,
			Detail: "",
		},
	}
}

// relayChecks is the check list of the proxy relay scenario.
func relayChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: "bot session entered the world",
			Done: false, Detail: "",
		},
		{
			ID: "login", Label: "client logged into the emulated login",
			Done: false, Detail: "",
		},
		{
			ID: "replay", Label: "client entered the world through the replay",
			Done: false, Detail: "",
		},
		{
			ID: "relay", Label: "client walk echoed through the live relay",
			Done: false, Detail: "",
		},
		{
			ID: "ping", Label: "net pings answered locally",
			Done: false, Detail: "",
		},
	}
}

// equipCounts summarizes the paperdoll of the character.
type equipCounts struct {
	weapon int
	chest  int
	legs   int
	head   int
	gloves int
	feet   int
	back   int
	jewels int
	shield int
}

// total sums the filled slots.
func (c equipCounts) armor() int {
	return c.chest + c.legs + c.head + c.gloves + c.feet + c.back
}

// evaluateEquipment reads the paperdoll and reports whether the
// character wears a proper gear set: a weapon in the right hand, the
// chest covered and at least four of the six armor slots plus one
// jewel filled - the outcome the shop strategy of a 100k adena wallet
// reaches in the elven village shops.
func evaluateEquipment(tracker *state.Bot) (equipCounts, bool) {
	doll := tracker.PaperdollSlotObjectIDs()
	counts := equipCounts{
		weapon: filled(doll[state.PaperdollRHand]),
		chest:  filled(doll[state.PaperdollChest]),
		legs:   filled(doll[state.PaperdollLegs]),
		head:   filled(doll[state.PaperdollHead]),
		gloves: filled(doll[state.PaperdollGloves]),
		feet:   filled(doll[state.PaperdollFeet]),
		back:   filled(doll[state.PaperdollBack]),
		shield: filled(doll[state.PaperdollLHand]),
		jewels: filled(doll[state.PaperdollREar]) +
			filled(doll[state.PaperdollLEar]) +
			filled(doll[state.PaperdollNeck]) +
			filled(doll[state.PaperdollRFinger]) +
			filled(doll[state.PaperdollLFinger]),
	}
	proper := counts.weapon == 1 && counts.chest == 1 &&
		counts.armor() >= 4 && counts.jewels >= 1

	return counts, proper
}

// filled reports a filled paperdoll slot.
func filled(objectID int32) int {
	if objectID != 0 {
		return 1
	}

	return 0
}

// evaluateSkills reports whether the character learned its key
// skills and has no affordable lesson left: the auras of the final
// state plus the emptied affordable prefix of the class tree.
func evaluateSkills(tracker *state.Bot) (detail string, done bool) {
	level := tracker.SelfLevel()
	plan := tracker.SkillPlan()
	affordableLeft := false
	if plan != nil {
		for i := range plan.Entries {
			entry := &plan.Entries[i]
			if entry.ReqLevel <= level &&
				int64(entry.SpCost) <= plan.Sp {
				affordableLeft = true

				break
			}
		}
	}
	auras := 0
	for _, skill := range tracker.ActiveSkills() {
		if skill.SkillID == attackAuraSkillID ||
			skill.SkillID == defenseAuraSkillID {
			auras++
		}
	}
	total := len(tracker.ActiveSkills())
	if auras < 2 {
		return fmt.Sprintf("%d skills, the auras missing", total), false
	}
	if affordableLeft {
		return fmt.Sprintf("%d skills, affordable lessons left", total),
			false
	}
	if plan == nil {
		return fmt.Sprintf("%d skills, the queue empty", total), true
	}

	return fmt.Sprintf("%d skills, %d sp left", total, plan.Sp), true
}

// evaluateZone reports whether the character stands inside its
// current hunting zone (the spot square the hunt loop leashes itself
// to) and returns the zone bounds for the kill placement check.
func evaluateZone(tracker *state.Bot) (detail string, inside bool,
	zone *state.Zone,
) {
	snapshot := tracker.Snapshot()
	zone = snapshot.HuntingZone
	if zone == nil {
		return "no hunting zone yet", false, nil
	}
	x, y, _, ok := tracker.SelfPosition()
	if !ok {
		return "no position yet", false, zone
	}
	if !insideZone(zone, x, y) {
		return fmt.Sprintf("at %d %d, the zone is %d %d %+d", x, y,
			zone.CX, zone.CY, zone.Half), false, zone
	}

	return fmt.Sprintf("inside %d %d %+d", zone.CX, zone.CY, zone.Half),
		true, zone
}

// insideZone reports whether the point lies inside the zone square.
func insideZone(zone *state.Zone, x int32, y int32) bool {
	return abs32(x-zone.CX) <= zone.Half && abs32(y-zone.CY) <= zone.Half
}

// abs32 is the absolute value of an int32.
func abs32(value int32) int32 {
	if value < 0 {
		return -value
	}

	return value
}

// evaluateKill reports whether one kill mark of this run landed
// inside the hunting zone.
func evaluateKill(
	tracker *state.Bot, zone *state.Zone, runStartMs int64,
) (detail string, done bool) {
	marks := tracker.KillMarks()
	for i := range marks {
		if marks[i].AtMs < runStartMs {
			// A kill of a previous run of the same character: the
			// kill ring survives the session resets.
			continue
		}
		if zone != nil && !insideZone(zone, marks[i].X, marks[i].Y) {
			continue
		}
		at := time.UnixMilli(marks[i].AtMs).Format("15:04:05")

		return "killed at " + itoa(int(marks[i].X)) + "," +
			itoa(int(marks[i].Y)) + " " + at, true
	}

	return "no kills yet", false
}

// waitOnline blocks until the tracker reports the session online (or
// the context ends): the precondition of every check evaluation.
func waitOnline(
	ctx context.Context, tracker *state.Bot, test *Test,
) error {
	deadline := time.Now().Add(onlineWait)
	for {
		if ctx.Err() != nil {
			return fmt.Errorf("cancelled while entering the world: %w",
				ctx.Err())
		}
		if tracker.Status() == state.StatusOnline {
			test.updateCheck(checkOnline, true,
				"online as "+tracker.Info().Name)

			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the bot never entered the world within " +
				itoa(int(onlineWait/time.Second)) + "s (status " +
				string(tracker.Status()) + ")")
		}
		test.appendLog("acceptance: waiting for the world entry, status " +
			string(tracker.Status()))
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(monitorPeriod):
		}
	}
}
