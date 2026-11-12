// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "sync"
    "sync/atomic"
    "time"
)

// Bot tracks the observed state of a single bot session.
type Bot struct {
    mu     sync.RWMutex
    id     string
    status Status
    // phase mirrors the hunt loop phase so the web UI can show a
    // human readable activity banner (hunting, walking to town,
    // selling, deleveling). Empty until the loop publishes its
    // first phase; the manual only sessions stay empty (the loop
    // never sets it) and the UI falls back to the status text.
    phase string
    // kind tags the role of the bot in the process: the empty
    // default reads as long-running, the acceptance manager tags
    // its temp bots with KindAcceptance so the web UI can split
    // them out of the fleet bot list. Set once at construction;
    // the sidebar reads it through Info().
    kind string
    // nowFn is the time source of the decision logic (see clock.go):
    // nil reads the system clock, the hunt loop and the tests inject
    // their own.
    nowFn func() time.Time
    // phaseAt records when the phase last changed: the
    // diagnostics report the age of the current phase (a bot
    // stuck in one phase for minutes is the top stuck
    // signature).
    phaseAt time.Time
    selfID  int32
    char    CharacterState
    // world is the dense object storage (see objectStore): the
    // packet apply paths mutate the records in place and the
    // scans walk the memory sequentially.
    world     objectStore
    inventory inventoryStore
    // inventoryVersion counts the inventory and paperdoll
    // mutations: the equip managers of the hunt loop key their
    // cached scans on it, so an unchanged bag costs no per tick
    // re-scan (see hunt equipManager).
    inventoryVersion uint64
    paperdoll        [PaperdollSlots]int32
    // log is the rolling packet event log, chat the chat window
    // ring, combat the web view animation feed (one component
    // value each, see their types for the layout contracts).
    log       eventLog
    chat      chatLog
    combat    combatFeed
    zone      *Zone
    zoneViews []ZoneView
    // huntMeshVersion and huntMeshJSON cache the encoded Voronoi
    // hunt mesh of the cell hunting (the static registry payload the
    // version keyed endpoint serves - see SetHuntingCells), huntCell
    // is the live record of the cell the loop holds. Both survive the
    // session resets like the zone policy: the fresh loop republishes
    // the live record after its first pick.
    huntMeshVersion string
    huntMeshJSON    []byte
    huntCell        CellLiveView
    // killMarks carries the recent kills of the hunt loop (the kill
    // ring of the cell hunter): the positions feed the fleet wide
    // cross layer of the map, so the crosses survive the bot switches
    // of the web view.
    killMarks []KillMarkView
    packets   int64
    version   uint64
    started   time.Time
    // sessionAt is the moment of the current login: ResetSession
    // refreshes it, so the bounded waits of the hunt loop (the skill
    // list gate of the first town trip) re-arm on every reconnect -
    // the enter world packet burst races the first ticks of every
    // session, not only the first one of the tracker.
    sessionAt time.Time
    updated   time.Time
    // hunt, huntPublishedAt: the internals the hunt loop
    // publishes every tick (SetHuntDiagnostics) with their
    // publication stamp - the encoders derive the loop
    // heartbeat age from it.
    hunt            HuntDiagnostics
    huntPublishedAt time.Time
    // huntLastAction, huntLastActionAt: the last decision line
    // of the loop (NoteAction), the same text the event log
    // carries.
    huntLastAction   string
    huntLastActionAt time.Time
    // huntErrand is the status of the blocking errand of the loop
    // (the city shopping round): the acceptance runner reads it to
    // fail fast on an errand that already stopped.
    huntErrand HuntErrand
    // actionFailedAt records the arrival time of the last
    // ActionFailed (0x35) answer of the server: the server refuses
    // a request - a move click its geodata collapses, an action
    // while the character is held - and answers with the one byte
    // packet. The hunt walk machinery reads it as the online
    // refusal evidence: the offline click validation port cannot
    // see a server side refusal (a different server build or
    // geodata validates the same click differently), the
    // ActionFailed answer is the only channel that names it (see
    // hunt.Loop.refusalEvidence).
    actionFailedAt time.Time
    // moveRequestAt records the send time of the last walk click
    // (MoveToLocation 0x01) and otherRequestAt the send time of the
    // last non walk request: the ActionFailed answer carries no
    // request identity, so the refusal channel compares the two to
    // decide whether an arrival may be attributed to the click at
    // all (an equip or transaction refusal riding the click would
    // otherwise latch a walk refusal that never happened, see
    // OtherRequestBetween).
    moveRequestAt  time.Time
    otherRequestAt time.Time
    // packetWindow feeds the packet rate of the diagnostics
    // view (see packetRateWindow).
    packetWindow packetRateWindow
    // eventSink mirrors every recorded event into the session
    // journal of cmd/swarm (see SetEventSink): nil unless the
    // journal wiring installed it.
    eventSink    EventSink
    commandQueue chan Command
    // The published walk plan of the web UI and the state dump
    // (see SetWalkPlan): the planning origin, the full waypoint
    // list of the segment, the waypoint the follower currently heads
    // to and the final destination - the whole walk reads at a
    // glance in the dump. walkPlanStart pins the moment the walk
    // was first published (the zero point of the waypoint timing
    // line of the dump) and walkWpAt holds the observed arrival
    // time of every waypoint (the entry i fills when the follower
    // cursor moves past i, see publishWalkPlanLocked).
    walkPlan      *WalkPlan
    walkPlanAt    time.Time
    walkPlanStart time.Time
    walkWpAt      []time.Time
    // lastWalkPlan keeps the most recent published walk plan after
    // its walk ended: the live plan expires with the walk (the TTL,
    // the arrive, the timeout), the report of a stuck segment needs the
    // whole planned walk even when the walk is already over (see
    // rememberWalkPlanLocked). The next published plan overwrites
    // the record; nothing clears it. lastWalkStart and lastWalkWpAt
    // carry the timing view of the record (the same zero point and
    // arrivals the live plan held, deep copied).
    lastWalkPlan   *WalkPlan
    lastWalkPlanAt time.Time
    lastWalkStart  time.Time
    lastWalkWpAt   []time.Time
    // shopping holds the published purchase queue of the shop
    // strategy (see SetShoppingPlan): what the bot plans to buy next
    // with the prices and the missing adena, nil while nothing is
    // published. shoppingAt bounds its freshness (shoppingPlanTTL).
    shopping   *ShoppingPlanView
    shoppingAt time.Time
    // dialog holds the open dialog page (the NPC_HTML scope mirror
    // of docs/quest_protocol.md): one page, replaced on every
    // NpcHtmlMessage arrival, cleared with the session (see
    // dialog.go).
    dialog openDialog
    // quests holds the quest journal of the QuestList packets
    // (quest id -> the state int of the cond or completion flags);
    // questItems holds the quest item stacks the packet repeats
    // (item id -> count, the quest-item id set the sell filter
    // needs). The server sends the whole journal on every change, so
    // both maps are replaced, not merged (see quests.go).
    quests     map[int32]int32
    questItems map[int32]int32
    // skills holds the learned skill list of the server packet
    // (id -> level + passive). skillsRevision counts the SetSkills
    // calls and the weapon priority changes; skillQueueCache caches
    // the ordered learning queue behind an atomic pointer and
    // rebuilds when the class or the revision moves (see
    // skillQueueLocked). skillWeapons holds the weapon families the
    // queue prefers (the weapon in hand and the next weapon of the
    // purchase plan).
    skills          map[int32]learnedSkill
    skillsRevision  uint64
    skillQueueCache atomic.Pointer[skillQueueCache]
    skillWeapons    []string
    // bookKeepCache caches the demanded spellbook item ids of the
    // unlocked queued lessons (see demandedBooksLocked): the junk
    // flows of the sell trips and the overflow destroy read it under
    // the read lock, so the per call walk of the queue stays cached
    // between the skill list revisions. A learn bumps the revision, a
    // level up shifts the unlock window, a class swap changes the
    // tree - the key fields answer all three. The atomic pointer
    // keeps the read lock readers read-only (the plain fields here
    // raced two concurrent snapshot encoders, see the 2026-09-20
    // review).
    bookKeepCache atomic.Pointer[bookKeepCache]
    // buffs holds the active effect list of the server
    // AbnormalStatusUpdate packets (skillId -> level + seconds left
    // at the arrival); buffsAt anchors the remaining seconds the
    // views count down from (see SetBuffs).
    buffs   map[int32]buffRecord
    buffsAt time.Time
    // skillCasts holds the cast and reuse windows the self
    // MagicSkillUse broadcasts opened (skill id -> the window): the
    // cast fill and the cooldown countdown of the web view read it
    // through the skillStates snapshot section (see skill_cast.go).
    skillCasts map[int32]skillCastWindow
    // critVictim/critVictimAt/critVictimHits carry the critical hit
    // hint of the last Attack broadcast: the victim of the landed
    // critical blows, the moment they landed and how many of them
    // await their HP drop (a dual weapon can crit twice in one
    // broadcast). The next HP drops of that victim within
    // critHintWindow consume the count and label their damage float
    // as critical (the StatusUpdate HP drops carry no crit flag -
    // see recordCharDamageLocked). One slot: the bursts that overlap
    // two crits on different victims inside the window are rare and
    // the wrong label is cosmetic only.
    critVictim     int32
    critVictimAt   time.Time
    critVictimHits int
    // loginCooldownUntil holds the reconnect pause the supervisor
    // honors after an emergency logout. The tracker outlives the
    // sessions, so the cooldown spans them (see SetLoginCooldown).
    loginCooldownUntil time.Time
    // emergencyReason carries the honest cause of the last intentional
    // emergency logout across the session boundary: the supervisor
    // consumes it once when the session unwinds, so the lost-session
    // record names the pile up instead of the misleading "use of
    // closed network connection" of the socket close.
    emergencyReason string
    // dangerSpots is the capped ring of the recent emergency logout
    // positions (see NoteDangerSpot): the zone regression of the hunt
    // loop folds them back in on every fresh session, because an
    // emergency logout ends the loop instance that counted it.
    dangerSpots []DangerSpot
    // deathCell names the hunting cell the last death regression
    // left, deathCellAt the moment it happened. The cell heat lives
    // in the per-session hunt loop, which the supervisor rebuilds on
    // every relogin (see runBot); the tracker outlives the sessions,
    // so the resume handoff of the next loop can refuse the deadly
    // ground the login landed on until the heat cools.
    deathCell   string
    deathCellAt time.Time
    // metrics holds the lifetime statistics counters of the tracker
    // (see metrics.go): the kills, deaths, sessions, swings and
    // damage of the whole deployment history of the bot, surviving
    // the session resets like every other lifetime field.
    metrics botMetrics
}

// NewBot creates a bot tracker for the given session id (account name).
func NewBot(id string) *Bot {
    return &Bot{ //nolint:exhaustruct_v5 // killMarks starts nil
        mu:                 sync.RWMutex{},
        id:                 id,
        status:             StatusConnecting,
        phase:              "",
        phaseAt:            time.Time{},
        loginCooldownUntil: time.Time{},
        emergencyReason:    "",
        dangerSpots:        nil,
        selfID:             0,
        char:               newCharacterState(),
        world:              newObjectStore(),
        inventory:          newInventoryStore(),
        inventoryVersion:   0,
        paperdoll:          [PaperdollSlots]int32{},
        log:                newEventLog(),
        chat:               newChatLog(),
        combat:             newCombatFeed(),
        zone:               nil,
        zoneViews:          nil,
        huntMeshVersion:    "",
        huntMeshJSON:       nil,
        huntCell:           CellLiveView{}, //nolint:exhaustruct_v5 // zero
        packets:            0,
        version:            0,
        started:            time.Now(),
        sessionAt:          time.Now(),
        updated:            time.Time{},
        hunt:               HuntDiagnostics{}, //nolint:exhaustruct_v5 // zero
        huntPublishedAt:    time.Time{},
        huntLastAction:     "",
        huntLastActionAt:   time.Time{},
        huntErrand:         HuntErrand{}, //nolint:exhaustruct_v5 // zero
        packetWindow: packetRateWindow{
            second: 0, filled: 0, counts: [packetRateSeconds]int32{},
        },
        eventSink:      nil,
        commandQueue:   make(chan Command, commandQueueCapacity),
        walkPlan:       nil,
        walkPlanAt:     time.Time{},
        shopping:       nil,
        shoppingAt:     time.Time{},
        skills:         nil,
        skillsRevision: 0,
        quests:         nil,
        questItems:     nil,
        skillWeapons:   nil,
        buffs:          nil,
        buffsAt:        time.Time{},
        metrics:        newBotMetrics(),
    }
}
