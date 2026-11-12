// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package hunt drives the automatic hunting behavior of a bot: attack the
// nearest attackable npc, pick up the loot it drops around the corpse and
// keep the inventory of the long living session working by destroying
// junk items when the slots or the weight run out.
package hunt

import (
    "log"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Loop is the hunt state machine of one bot session. The fields live in
// the embedded per-concern blocks declared in loop_state.go; a block is
// promoted, so the methods of the package read l.target, not
// l.combatState.target.
type Loop struct {
    game       GameAPI
    tracker    *state.Bot
    logger     *log.Logger
    journal    *session.Journal
    phase      phase
    autonomous bool
    combatState
    lootState
    restState
    questState
    travelState
    townState
    shopState
    learnState
    delevelState
    groundState
    safetyState
    diagState
    userState
    gearState
    kiteState
}

// NewLoop creates the hunt loop for a connected game client. Every
// field left at its zero value carries the documented zero start; the
// maps and the tuned defaults below are the only non-zero starts.
func NewLoop(game GameAPI, tracker *state.Bot) *Loop {
    loop := &Loop{
        game:       game,
        tracker:    tracker,
        logger:     log.Default(),
        phase:      phaseEngage,
        autonomous: true,
    }
    loop.equip = newEquipManager(gear.MeleeFighter{})
    loop.kite = DefaultKiteParams()
    loop.segmentRadius = tripApproachRadius
    loop.zoneDeathCap = -1
    loop.skipped = make(map[int32]time.Time)
    loop.sold = make(map[int32]bool)
    loop.skillReuse = make(map[int32]time.Time)
    loop.gearDebt = make(map[int]int32)
    loop.pendingActions = make(map[int32]pendingInventory)

    return loop
}

// SetNavigator installs the geodata path finder behind the town trips.
// Without a navigator the trips never trigger and the loop hunts as
// before.
func (l *Loop) SetNavigator(navigator Navigator) {
    l.navigator = navigator
}

// SetLogger replaces the log target of the loop. The default is
// log.Default; the deployment wires a logger that mirrors the hunt
// decision lines into the bot tracker event log (they then show up
// in the web UI log tab and the state dump - the debugging material
// of the live sessions).
func (l *Loop) SetLogger(logger *log.Logger) {
    if logger == nil {
        return
    }
    l.logger = logger
}

// SetJournal installs the session journal of the loop: the kill,
// death, trip and zone events of the structured session trail land in
// it. A nil journal (the default, and the -session-dir "" mode) turns
// every journal call into a no-op, so the tests and the acceptance
// scenarios run unchanged.
func (l *Loop) SetJournal(journal *session.Journal) {
    l.journal = journal
}

// SetGearProfile replaces the gear scoring profile of the auto
// equipment and the shop strategy (the melee fighter is the default;
// a mage profile swaps weapon and armor preferences).
func (l *Loop) SetGearProfile(profile gear.Profile) {
    if l.equip == nil {
        l.equip = newEquipManager(profile)

        return
    }
    l.equip.profile = profile
}

// SetAutonomy toggles the autonomous hunting of the loop: a manual only
// session (the bot started without -hunt) keeps the loop in the idle
// phase and only executes the manual commands of the web UI (move,
// attack, pickup, useItem, drop, destroy), including the village
// restart after a death.
func (l *Loop) SetAutonomy(autonomous bool) {
    l.autonomous = autonomous
    if !autonomous {
        l.phase = phaseIdle
    }
}
