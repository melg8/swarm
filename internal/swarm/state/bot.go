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
    // phase is the activity banner of the hunt loop; a manual-only session
    // leaves it empty and the UI shows the status text.
    phase string
    // kind tags the role: empty reads as a fleet bot, KindAcceptance splits
    // the manager's temp bots out of the fleet list (set once).
    kind string
    // nowFn is the decision clock (see clock.go); nil reads the system one.
    nowFn func() time.Time
    // phaseAt ages the phase; a long age is the top stuck signature.
    phaseAt time.Time
    selfID  int32
    char    CharacterState
    // world is the dense object storage (see objectStore): applies mutate in
    // place, scans walk it sequentially.
    world     objectStore
    inventory inventoryStore
    // inventoryVersion counts the inventory and paperdoll mutations; the
    // equip managers key their cached scans on it (see hunt equipManager).
    inventoryVersion uint64
    paperdoll        [PaperdollSlots]int32
    // log, chat and combat are the event log, the chat ring and the web
    // view animation feed.
    log       eventLog
    chat      chatLog
    combat    combatFeed
    zone      *Zone
    zoneViews []ZoneView
    // huntMeshVersion and huntMeshJSON cache the encoded Voronoi hunt mesh
    // (see SetHuntingCells); huntCell is the live cell the loop holds. Both
    // survive session resets, and the fresh loop republishes the record.
    huntMeshVersion string
    huntMeshJSON    []byte
    huntCell        CellLiveView
    // killMarks carries the recent kill positions: the fleet cross layer
    // survives the bot switches of the web view.
    killMarks []KillMarkView
    packets   int64
    version   uint64
    started   time.Time
    // sessionAt is the moment of the current login: ResetSession refreshes
    // it, so the bounded waits re-arm on every reconnect (the enter world
    // burst races the first ticks of every session).
    sessionAt time.Time
    updated   time.Time
    // hunt and huntPublishedAt are the diagnostics the loop publishes every
    // tick, stamped for the heartbeat age of the encoders.
    hunt            HuntDiagnostics
    huntPublishedAt time.Time
    // huntLastAction is the last decision line of the loop (NoteAction).
    huntLastAction   string
    huntLastActionAt time.Time
    // huntErrand is the blocking errand status; the acceptance runner fails
    // fast on an errand that already stopped.
    huntErrand HuntErrand
    // actionFailedAt is the arrival of the last ActionFailed (0x35): the
    // only channel that names a server side refusal, which the offline click
    // port cannot see (see hunt.Loop.refusalEvidence).
    actionFailedAt time.Time
    // moveRequestAt and otherRequestAt are the sends of the last walk click
    // (0x01) and of the last other request: ActionFailed carries no identity,
    // so the refusal channel compares them (see OtherRequestBetween).
    moveRequestAt  time.Time
    otherRequestAt time.Time
    // packetWindow feeds the packet rate of the diagnostics view.
    packetWindow packetRateWindow
    // eventSink mirrors every event into the session journal (SetEventSink).
    eventSink    EventSink
    commandQueue chan Command
    // walkPlan is the published walk of the web UI and the dump (see
    // SetWalkPlan): the segment waypoints, the current cursor and the
    // destination. walkPlanStart is the zero point of the timing, and
    // walkWpAt holds the arrival of every waypoint (see
    // publishWalkPlanLocked).
    walkPlan      *WalkPlan
    walkPlanAt    time.Time
    walkPlanStart time.Time
    walkWpAt      []time.Time
    // lastWalkPlan keeps the last walk after it ended: a stuck segment report
    // needs the whole plan (see rememberWalkPlanLocked). The next published
    // plan overwrites it, and the lastWalk* fields carry its timing.
    lastWalkPlan   *WalkPlan
    lastWalkPlanAt time.Time
    lastWalkStart  time.Time
    lastWalkWpAt   []time.Time
    // shopping is the published purchase queue (see SetShoppingPlan), nil
    // while nothing is published; shoppingAt bounds its freshness
    // (shoppingPlanTTL).
    shopping   *ShoppingPlanView
    shoppingAt time.Time
    // dialog is the open NPC_HTML page (see dialog.go): one page, replaced on
    // every NpcHtmlMessage and cleared with the session.
    dialog openDialog
    // quests is the quest journal (id -> state) and questItems the repeated
    // stacks (id -> count, the sell filter set): the server sends the whole
    // journal on every change, so both replace (see quests.go).
    quests     map[int32]int32
    questItems map[int32]int32
    // skills holds the learned list (id -> level + passive).
    // skillsRevision counts SetSkills and weapon priority changes;
    // skillQueueCache rebuilds when the class or revision moves (see
    // skillQueueLocked). skillWeapons holds the preferred weapon families.
    skills          map[int32]learnedSkill
    skillsRevision  uint64
    skillQueueCache atomic.Pointer[skillQueueCache]
    skillWeapons    []string
    // bookKeepCache caches the demanded spellbook ids (see
    // demandedBooksLocked) between skill revisions: the sell trips read it
    // under the read lock, so the atomic pointer keeps those readers
    // read-only.
    bookKeepCache atomic.Pointer[bookKeepCache]
    // buffs holds the AbnormalStatusUpdate effects (skill -> level + seconds
    // left at arrival); buffsAt anchors the countdown (see SetBuffs).
    buffs   map[int32]buffRecord
    buffsAt time.Time
    // skillCasts holds the cast and reuse windows the self MagicSkillUse
    // opened: the web view reads them through the skillStates section (see
    // skill_cast.go).
    skillCasts map[int32]skillCastWindow
    // critVictim, critVictimAt and critVictimHits carry the critical hint of
    // the last Attack broadcast: the next HP drops of that victim inside
    // critHintWindow consume the count and label the damage critical (the
    // StatusUpdate drops carry no flag, see recordCharDamageLocked). One
    // slot: an overlapping mislabel is cosmetic.
    critVictim     int32
    critVictimAt   time.Time
    critVictimHits int
    // loginCooldownUntil is the reconnect pause after an emergency logout;
    // the tracker outlives the sessions (see SetLoginCooldown).
    loginCooldownUntil time.Time
    // emergencyReason is the cause of the last emergency logout, consumed by
    // the supervisor: the lost-session record names it instead of the closed
    // socket.
    emergencyReason string
    // dangerSpots is the capped ring of recent emergency logout positions
    // (see NoteDangerSpot); the fresh loop folds them into the zone
    // regression, which lost its count with the previous loop.
    dangerSpots []DangerSpot
    // deathCell and deathCellAt name the cell the last death regression left.
    // The heat lives in the per-session loop, so the next loop refuses the
    // deadly ground the login landed on until it cools (see runBot).
    deathCell   string
    deathCellAt time.Time
    // metrics holds the lifetime counters (see metrics.go), surviving the
    // session resets like every other lifetime field.
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
