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
    dressAccount   = "temp11"
    dressPassword  = "temp11"
    escapeAccount  = "temp10"
    escapePassword = "temp10"
    pocketAccount  = "temp12"
    pocketPassword = "temp12"
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

// villageEscapeTimeout bounds the refused-click escape scenario:
// the escape window itself is the two minutes of the user contract
// (villageEscapeWindow, measured from the world entry), the scenario
// budget adds the character injection, the login handshake and the
// graceful shutdown around it.
const villageEscapeTimeout = 5 * time.Minute

// fullDressTimeout bounds the world entry burst scenario: the run is
// the character injection, the login handshake, one burst of the auto
// equipment and the graceful shutdown - the dress itself lands within
// the ten second window of the world entry, the budget keeps the
// session prologue comfortable.
const fullDressTimeout = 5 * time.Minute

// Definitions returns the acceptance scenario list: the world entry
// burst round of the auto equipment first (the newest round owns the
// head of the list the web UI serves), then the village escape of the
// refused-click dump, the farm readiness round of the user, plus the
// in-process mirrors of the two existing e2e harnesses
// (tools/mobius_e2e.sh and tools/proxy_e2e.sh) so the whole
// acceptance suite runs from one place.
//
//nolint:funlen,maintidx // the scenario registry is one linear table by design
func Definitions() []TestDef {
    return []TestDef{
        {
            ID:      delevelScenarioID,
            Title:   "delevel · the guard death cycle",
            Account: delevelAccount,
            Timeout: delevelTimeout,
            Description: "Start: the elven fighter temp14 wakes ON the " +
                "Green Dryad S-16 hunting cell (43500 54560 -3664, the " +
                "cell focus on its single geodata layer, 3200 units " +
                "south of the Starden guard post) as level 15 with the " +
                "BOTTOM of level experience and the wearable outfit in " +
                "the bag. The cell median is 8 (the Green Dryad and " +
                "the Kaboo Orc Grunt of the ground), so the trigger " +
                "gap holds for the live and the static median alike " +
                "and the target (median + 5) lands at 13 - two guard " +
                "deaths from the level bottom (the Mobius death " +
                "penalty removes a percentage of the level span, every " +
                "death from the bottom crosses one level boundary). " +
                "Flow: the deleveling announces its target level and " +
                "its reason in the webui message (the diagnostics " +
                "carry the target, the start level and the zone " +
                "median), the bot walks to the archer guard, provokes " +
                "it, pays the death penalty, repeats until the target " +
                "and walks home. Pass: the walk made progress (no " +
                "freeze), at least one death paid the penalty, the " +
                "character reached the announced target, the bot " +
                "returned onto the farm ground and NO new deleveling " +
                "started within the no retry window after the finish " +
                "(the anti ping-pong contract - the escalating abort " +
                "wait of the deleveling, the unit ladder lives in " +
                "hunt/delevel_oscillation_test.go).",
            Scenario: delevelScenario,
        },
        {
            ID:      "church-entry",
            Title:   "church entry · the temple NPC walk",
            Account: churchAccount,
            Timeout: churchEntryTimeout,
            Description: "Start: the elven fighter temp13 wakes at the " +
                "village plaza cell in front of the temple (44694 51921 " +
                "-2808, the walk start of the 2026-09-19 owner report) " +
                "as the level 15 character with the starter outfit. " +
                "Flow: the manual walk command aims the temple interior " +
                "cell (44718 52291 -2792) 40 units from the hierarch " +
                "Asterios, the hunt loop (manual only mode) plans the " +
                "route and clicks the legs, the way the owner drives " +
                "the map walks. Pass: the character leaves the plaza, " +
                "crosses the temple entrance and stands on the interior " +
                "cell within the NPC ring (the report held the character " +
                "at the entrance forever - the same click from the real " +
                "client through the proxy refused to enter too).",
            Scenario: churchEntryScenario,
        },
        {
            ID:      "full-dress",
            Title:   "full dress · the world entry burst",
            Account: dressAccount,
            Timeout: fullDressTimeout,
            Description: "Start: the elven fighter temp11 spawns at " +
                "the character creation point of the elven village " +
                "as level 15 with an empty paperdoll and the complete " +
                "outfit in the bag - the Brandish two hander, the " +
                "wooden breastplate, the bone gaiters, the leather " +
                "helmet, the gloves, the apprentice's shoes, both " +
                "earrings, both anguish rings and the necklace " +
                "(eleven wearable pieces, nothing worn). Flow: the " +
                "world entry arms the auto equipment and the burst " +
                "planner sends every independent use item request in " +
                "one tick - the deployed build disables the UseItem " +
                "flood protector (FloodProtectorUseItemInterval = 0) " +
                "and the independent slots never share server state, " +
                "the pair swaps and the displaced pieces wait for " +
                "their confirmations. Pass: every injected piece " +
                "sits on its paperdoll slot within the ten second " +
                "window of the world entry (the retired fixed pause " +
                "between the requests needed over twenty seconds " +
                "for the same bag).",
            Scenario: fullDressScenario,
        },
        {
            ID:      "village-escape",
            Title:   "village escape · the refused-click dump cell",
            Account: escapeAccount,
            Timeout: villageEscapeTimeout,
            Description: "Start: the elven fighter temp10 wakes at the " +
                "reported stuck cell of the 2026-09-14 12:31 dump " +
                "(45768 49848 -3056, the elven village plaza - the " +
                "position test3 held through a whole day of three dumps) " +
                "as the level 15 character of the report with the exact " +
                "state it carried - 87.09 percent of level 15, 1760 sp, " +
                "1312 adena, the Brandish sword, the bone armor set, the " +
                "leather helmet and gloves, the starter jewels, 589 " +
                "arrows and the hunting bow in the bag. Flow: the bot " +
                "walks itself out of the village - the walk plans the " +
                "geodata route through the village streets, the follower " +
                "clicks its legs and the stuck recovery (the varied aim, " +
                "the corridor detours, the server routed hops) owns every " +
                "refused click along the way. Pass: the character stands " +
                "outside the city - 3000+ units from the village plaza, " +
                "past every wall, gate and deck of the geodata - within " +
                "two minutes of the world entry (the dump day held " +
                "characters on that cell forever; the reproductions of " +
                "the corridor and refusal rounds live in " +
                "hunt/corridor_widen_repro_test.go and " +
                "hunt/refusal_signal_repro_test.go).",
            Scenario: villageEscapeScenario,
        },
        {
            ID:      "railing-pocket",
            Title:   "railing pocket · the mesh island cell",
            Account: pocketAccount,
            Timeout: railingPocketTimeout,
            Description: "Start: the elven fighter temp13 wakes at the " +
                "reported route cell of the 2026-09-19 report (43736 " +
                "47048 -2992, the elven village deck cell just off the " +
                "railings) as a level 15 fighter with the standard " +
                "dump outfit. The cell itself is honest ground - the " +
                "server walks clicks in and out of it and the grid " +
                "engine plans out of it - but every axis neighbor " +
                "carries a railing wall bit, so the mesh link graph " +
                "(edge-only connections) holds it as a linkless one " +
                "polygon island whose only way out is the diagonal " +
                "squeeze the server's anti corner cut rule allows. " +
                "Before the pocket escape every plan attempt from " +
                "this cell answered the bare not found and the bot " +
                "stood there forever. Flow: the walk plans the mesh " +
                "pocket escape (the closest reachable route out of " +
                "the stranded component), the follower clicks at the " +
                "refined exit aim and the followup plan cycles route " +
                "from the connected deck. Pass: the character stands " +
                "256+ units from the pocket cell within two minutes " +
                "of the world entry - on connected ground the " +
                "planner routes from (the loop level reproduction " +
                "lives in hunt/railing_pocket_repro_test.go, the " +
                "mesh level one in " +
                "pathfind/navmesh/pocket_test.go).",
            Scenario: railingPocketScenario,
        },
        {
            ID:      "stuck-point-43632",
            Title:   "stuck point 43632 · the frozen terrace spot",
            Account: stuckAccountA,
            Timeout: stuckPointTimeout,
            Description: "Start: the elven fighter temp15 wakes at the " +
                "first stuck position of the 2026-09-20 fleet freeze " +
                "report (43632 50560 -2960, heading 21963 - the elven " +
                "village terrace spot) as a level 15 fighter with the " +
                "standard dump outfit. The spot stands on a link " +
                "component the strict edge only mesh link graph " +
                "cannot leave (52 polygons, the 640x752 box) while " +
                "the grid engine plans out of the very same cell " +
                "through the diagonal squeezes the server movement " +
                "channels allow. The component outgrew the old 320 " +
                "pocket side, so the escape declined and the bots " +
                "stood frozen there (the bare not found, or the " +
                "partial that walks to the component's inner " +
                "boundary and strands it). Flow: the walk plans the " +
                "widened mesh pocket escape (the closest reachable " +
                "route out of the stranded component, the exit aim " +
                "horizontally displaced from the standing point), " +
                "the follower clicks at the refined exit aim and the " +
                "followup plan cycles route from the connected " +
                "ground. Pass: the character stands 256+ units from " +
                "the reported spot within two minutes of the world " +
                "entry (the mesh level reproduction lives in " +
                "pathfind/navmesh/pocket_test.go, the loop level one " +
                "in hunt/stuck_terrace_repro_test.go).",
            Scenario: stuckPointAScenario,
        },
        {
            ID:      "stuck-point-41920",
            Title:   "stuck point 41920 · the frozen terrace spot",
            Account: stuckAccountB,
            Timeout: stuckPointTimeout,
            Description: "Start: the elven fighter temp16 wakes at the " +
                "second stuck position of the 2026-09-20 fleet " +
                "freeze report (41920 52128 -3000, heading 26712 - " +
                "the elven village terrace spot by the north dock " +
                "walk) as a level 15 fighter with the standard dump " +
                "outfit. The spot stands on a link component the " +
                "strict edge only mesh link graph cannot leave (29 " +
                "polygons, the 592x432 box) while the grid engine " +
                "plans out of the very same cell through the " +
                "diagonal squeezes the server movement channels " +
                "allow (the spot also sits 24 units above the " +
                "surrounding ground - the layers stacked directly " +
                "under it answered the closest 3D exit candidates " +
                "and the escape aim degenerated into the standing " +
                "cell before the horizontal displacement floor). " +
                "Flow: the walk plans the widened mesh pocket " +
                "escape, the follower clicks at the refined exit " +
                "aim and the followup plan cycles route from the " +
                "connected ground. Pass: the character stands 256+ " +
                "units from the reported spot within two minutes of " +
                "the world entry (the mesh level reproduction lives " +
                "in pathfind/navmesh/pocket_test.go, the loop level " +
                "one in hunt/stuck_terrace_repro_test.go).",
            Scenario: stuckPointBScenario,
        },
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
