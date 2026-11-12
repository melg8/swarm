// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// combatState is the target and fight state of the loop: the pick,
// the fight clocks, the skip lists, the lure and the blind engage
// recovery.
type combatState struct {
    target int32
    // lastActionAt paces every action the loop initiates: each gate
    // reads it against its own period, so two actions never share one
    // window.
    lastActionAt time.Time
    // fightStartAt and fightStartFor hold the start of the CURRENT
    // fight, keyed by target id; engageAt re-anchors on live fight
    // activity for the stuck timeout and cannot measure its length.
    fightStartAt  time.Time
    fightStartFor int32
    engageAt      time.Time
    // targetSkip holds the mobs the target search holds out, each on
    // its own expiry deadline.
    targetSkip map[int32]time.Time
    // The ranged lure of the current pick, nil when no lure runs
    // (lure.go).
    lure *lureState
    // The corpse record of the last kill: the ranged kills drop far
    // from the character, so the loot phase walks to the corpse and
    // waits the drop broadcast out (loop_actions.go).
    killX        int32
    killY        int32
    killZ        int32
    killAt       time.Time
    killPosKnown bool
    killWalked   bool
    killLooted   bool
    // The blind engage recovery (loop_los.go): the armed reposition
    // walk, its waypoints and the target attempt budget.
    losAt        time.Time
    losWaypoints []pathfind.Vec3
    losWpIndex   int
    losMoveAt    time.Time
    losTried     int
    // skipScratch is the reused dense skip list of the target search:
    // activeSkips rebuilds it in place, so the tick filter allocates
    // nothing.
    skipScratch []int32
    // avoidScratch is the reused threat buffer of the aggro-aware
    // walk steer (loop_avoid.go), refilled in place.
    avoidScratch []state.AggroThreat
    // avoidLogAt paces the detour diagnostic of the walk steer: one
    // line per period.
    avoidLogAt time.Time
    // combatAvoidScanAt paces the impending-add threat scan at the
    // target search cadence.
    combatAvoidScanAt time.Time
    // combatAvoidAt paces the impending-add reposition steps: one per
    // period.
    combatAvoidAt time.Time
    // combatAvoidUntil is the movement window the impending-add step
    // or the kite step owns; one movement owner per fight step.
    combatAvoidUntil time.Time
    // combatAvoidHoldAt paces the walled-pocket diagnostic of the
    // impending-add step.
    combatAvoidHoldAt time.Time
    // The combat casting state (combat_skills.go): the strike, spell
    // and self buff pacing plus the local reuse windows (the server
    // clock starts at the cast start, the local one keeps a jitter
    // margin).
    castAt        time.Time
    buffAt        time.Time
    skillReuse    map[int32]time.Time
    profilePicked bool
    // healFlightUntil bounds the in-flight recovery self heal: the
    // sit request waits it out (recovery_heal.go).
    healFlightUntil time.Time
    engLastDist     float64
    engDistAt       time.Time
}

// lootState is the loot phase state: the in-flight pickup and the
// drops standing on their pickup retry deadline.
type lootState struct {
    lootID     int32
    lootAt     time.Time
    lootMoveAt time.Time
    itemSkip   map[int32]time.Time
}

// restState is the rest phase state: the sit/stand toggle and the
// death restart pacing.
type restState struct {
    restActionAt  time.Time
    restActionSit bool
    restartAt     time.Time
}

// questState is the quest trip pacing: the walk, potion and fight log
// periods of the class transfer stops.
type questState struct {
    // questWalkAt paces the quest trip walk requests (the flood
    // protector mute of the 2026-09-12 class transfer run).
    questWalkAt time.Time
    // questPotionAt paces the quest trip potion uses (the C1 reuse
    // delay of the healing potions).
    questPotionAt time.Time
    // questFightLogAt paces the quest trip fight diagnostics.
    questFightLogAt time.Time
}

// travelState is the waypoint walker: the plan, the stuck and refusal
// watchdogs, the cursor escape.
type travelState struct {
    zoneCX       int32
    zoneCY       int32
    zoneHalf     int32
    navigator    Navigator
    waypoints    []pathfind.Vec3
    wpIndex      int
    segmentDest  pathfind.Vec3
    segmentStart pathfind.Vec3
    // segmentFrameOffset is the measured z frame offset of the current
    // plan (click_frame.go): every plan derived click z rides it into
    // the server frame, a plan without a measurable offset keeps zero.
    segmentFrameOffset float64
    moveAt             time.Time
    // flowScanAt paces the corner pre-click scans: at most one per
    // flowPreClickGap whatever the answer.
    flowScanAt time.Time
    stuckAt    time.Time
    stuckX     int32
    stuckY     int32
    // stuckWP is the cursor the stuck window opened at: a cursor
    // advance is net progress.
    stuckWP int
    // stuckBest is the best distance to the waypoint inside the
    // window; only an improvement past the margin resets it.
    stuckBest float64
    // stuckFast arms after the first stuck skip of a trip: later
    // detections use stuckFastTimeout.
    stuckFast bool
    // skipX/skipY record the cell the last waypoint skip left the
    // character on: a repeat skip on the same cell is denied, because
    // it proved the click transport dead there.
    skipArmed bool
    skipX     int32
    skipY     int32
    // moveStartAt/X/Y are the move start watchdog (noteMoveStart): a
    // click that leaves the character on the baseline cell names the
    // transport dead and fast forwards the recovery ladder.
    moveStartAt time.Time
    moveStartX  int32
    moveStartY  int32
    // forceStuck runs the next walkStuck verdict's recovery at once.
    forceStuck bool
    rePaths    int
    // extendArmed arms the short click extension: a stuck with no
    // clear successor re-aims under-floor and backward targets at the
    // forward route samples (docs/hunting.md).
    extendArmed   bool
    repathX       int32
    repathY       int32
    frozenRepaths int
    // cursorEscape is the cursor key escape of a click-refusing cell
    // (beginCursorKeyEscape): the arrow key emulation the server
    // follows without any click validation.
    cursorEscape cursorEscapeState
    // cursorEscapes counts the escape attempts of the trip: a server
    // that ignores the claims too aborts the trip instead of grinding.
    cursorEscapes int
    // segmentRefused latches the online refusal evidence of the
    // current segment: the server answered ActionFailed while the
    // character stood still (refusalEvidence).
    segmentRefused bool
    // segmentRefusedX/Y remember the cell the first refusal latched
    // at: a stuck verdict there names a refusing pocket, and the
    // cursor escape owns it before the varied aims.
    segmentRefusedX int32
    segmentRefusedY int32
    // refusalVariants counts the varied aims spent on the segment: the
    // refusal is target specific.
    refusalVariants int
    // zoneRefusalX/Y remember where the refusal answers last stalled
    // the direct zone segments (noteZoneSegmentStall).
    zoneRefusalX int32
    zoneRefusalY int32
    // segmentRadius is the approach radius of the current segment's
    // search: the npc radius for npc destinations, the trip ring for
    // the returns.
    segmentRadius float64
    // segmentSearch is the mesh search contract of the current
    // segment's plan, published by the walk plan view; nil for the
    // direct segments.
    segmentSearch *state.WalkSearch
    farmX         int32
    farmY         int32
    farmZ         int32
    zoneReturn    bool
    zoneFails     int
    // zoneSegmentAt/X/Y hold the no-movement window of the direct zone
    // segments (noteZoneSegmentStall).
    zoneSegmentAt time.Time
    zoneSegmentX  int32
    zoneSegmentY  int32
    roadFights    int
    // zoneSegmentLogAt paces the walled direct segment diagnostic.
    zoneSegmentLogAt time.Time
}

// townState is the town trip: the merchant stops and the sell pacing.
type townState struct {
    // tripAbortRun counts the consecutive aborted trips: past the
    // escalation threshold the cooldown doubles per abort (capped).
    tripAbortRun      int
    sellAt            time.Time
    sellPhaseAt       time.Time
    merchantID        int32
    merchantPick      time.Time
    merchantDeckUntil time.Time
    // merchantPull paces the attack analog pull ladder of the deck
    // wait (approachMerchant).
    merchantPull time.Time
    // merchantPulled holds the object id the post selection pull
    // already fired for.
    merchantPulled int32
    sold           map[int32]bool
    tripStart      time.Time
    tripEndedAt    time.Time
}

// shopState is the town shopping: the frozen purchase plan, the buy
// confirmations and the replacement sales.
type shopState struct {
    // cityShoppingTriedAt stamps the last city shopping errand attempt
    // for the retry cooldown (city_shopping.go).
    cityShoppingTriedAt time.Time
    // The town trip shopping (shopping.go): the frozen purchase plan,
    // its merchant stops, the buy pacing and the in-flight batch.
    tripPlan     []gear.Purchase
    tripStops    []tripStop
    buysPlanned  bool
    buyAt        time.Time
    buyRequested []gear.Purchase
    // buyBaseline holds the owned stack count of every in-flight item
    // at the request: the confirmation compares the live count against
    // baseline plus order.
    buyBaseline       map[int32]int32
    buyConfirmAt      time.Time
    buyRetries        int
    shoppingPlanAt    time.Time
    shoppingPlanCache []gear.Purchase
    // shoppingPlanAdena is the adena the cached queue was planned
    // against: the widget shows the planning wallet, not the live one.
    shoppingPlanAdena int64
    // shoppingViewCache is the built view of shoppingPlanCache:
    // rebuilding it every 200 ms tick was 44 percent of the fleet heap
    // allocations, a pointer copy now serves the 25 ticks between
    // recomputes.
    shoppingViewCache state.ShoppingPlanView
    // The sell first step of the replacement purchases
    // (stepReplacementSales): the object ids to unequip, the collected
    // sale batch and the pacing flags.
    replacePlanned   bool
    replaceDone      bool
    replaceQueue     []int32
    replaceSelling   []state.InventoryItem
    replaceSellSent  bool
    replaceUnequipAt time.Time
    replaceWaitAt    time.Time
    replaceTried     int
    // tripGearStart snapshots the paperdoll object ids and item ids the
    // trip started with: a slot the trip left empty whose piece is gone
    // arms the gear debt (gearDebtCheck).
    tripGearStart    [state.PaperdollSlots]int32
    tripGearStartIDs [state.PaperdollSlots]int32
    // gearDebt holds the paperdoll slot indexes the trip stranded with
    // the lost item id; the debt survives the trip and runs the refill
    // trip until the slot is dressed again.
    gearDebt map[int]int32
}

// learnState is the skill learning: the teacher stops, the lesson
// confirmations and the guide buff stops.
type learnState struct {
    // The skill learning (learning.go): the cached queue, the selected
    // teacher with its pacing and the in-flight lesson.
    learnPlanCache    []state.SkillPlanEntry
    learnPlanAt       time.Time
    learnPlanRevision uint64
    teacherID         int32
    teacherPick       time.Time
    teacherDeckUntil  time.Time
    // teacherWalkUntil bounds the close approach of the teach stop:
    // the ring walk until standing by the teacher, or the stop skips.
    teacherWalkUntil time.Time
    learnRequested   *lessonTarget
    learnConfirmAt   time.Time
    learnRetries     int
    learnAt          time.Time
    learnRevision    uint64
    // The Newbie Guide support magic (guide_buffs.go): the found npc,
    // its pacing, the approach window and the refusal cooldown that
    // outlives the trip.
    guideID        int32
    guidePick      time.Time
    guideAt        time.Time
    guideWalkUntil time.Time
    guideRefusedAt time.Time
}

// delevelState is the deleveling: the guard target, the free death
// budget and the abort backoff.
type delevelState struct {
    delevelTarget int32
    // delevelMedian is the live median mob level of the held ground at
    // the deleveling start, the trigger evidence.
    delevelMedian  int32
    delevelGuard   int32
    delevelTried   map[string]bool
    delevelFight   time.Time
    delevelEnd     time.Time
    delevelExp     int32
    delevelLevel   int32
    delevelFree    int
    delevelWait    time.Time
    delevelCounted bool
    // delevelAborts counts the consecutive aborted delevelings: each
    // abort escalates the wait through delevelAbortWait, only a
    // finished one resets the streak.
    delevelAborts int
}

// groundState is the hunting ground registry: the zone list, the held
// cell and the per-zone death bookkeeping.
type groundState struct {
    // The multi zone hunting (zones.go): the registry, the cell mode
    // partition (cell_policy.go) and the picked zone.
    zones []HuntingZone
    cell  *cellHunter
    // cellPin is the pending ground lock of SetCellPin: it lands
    // before the registry exists and SetHuntingCells re-applies it.
    cellPin      string
    zoneRegion   string
    zonePickedID string
    zoneCheckAt  time.Time
    // The death regression bookkeeping (zones.go): the per zone death
    // counts, the band cap a demoted zone installs (negative = no cap)
    // and the level it last reset at.
    zoneDeaths     map[string]int32
    zoneDeathCap   int32
    zoneDeathLevel int32
    // zoneEmptySince arms the rotation of a cleared-out square: the
    // moment it turned mob-less, zero while mobs remain or the
    // character is busy.
    zoneEmptySince time.Time
    // zoneEmptyUntil is the empty cooldown of the rotated-away squares:
    // they stay out of the rotation until it expires.
    zoneEmptyUntil map[string]time.Time
    // zoneMobPriority is the template id to priority engage bias of the
    // picked zone, nil without mob data.
    zoneMobPriority map[int32]int32
}

// safetyState is the defensive layer: the flee episode, the escape
// watchdogs, the pile up run and the post relogin settle.
type safetyState struct {
    // fleeAt paces the escape walk requests, independent of the engage
    // pacing.
    fleeAt time.Time
    // fleeSince is the start of the running flee episode: past
    // fleeLogoutAfter the session logs out to reset the aggro.
    fleeSince time.Time
    // fleeGapAt and fleeGap are the closure-rate watchdog of the
    // escape: the probe moment and the threat distance then; a window
    // that fails to widen the gap ends the session early
    // (escapeNotOpening).
    fleeGapAt time.Time
    fleeGap   float64
    // escapeAnchorAt/X/Y are the no-progress watchdog: a window that
    // covers less than escapeNoProgressDistance names a dead escape
    // (the H-006 silent drop) and logs out.
    escapeAnchorAt time.Time
    escapeAnchorX  int32
    escapeAnchorY  int32
    // escapeWalkAt stamps the last escape walk: the no-progress window
    // only opens with an issued walk.
    escapeWalkAt time.Time
    // panicAt marks the armed pile up run (panicPileUpRun): zero while
    // none forced one, else the moment the pack was spotted.
    panicAt time.Time
    // panicX and panicY anchor the pile up run: the logout fires past
    // panicRunDistance from the aggro point.
    panicX int32
    panicY int32
    // logoutDone marks the one shot emergency logout so the dying ticks
    // stay quiet.
    logoutDone bool
    // spawnSettle arms the post relogin settle (settle.go): the
    // supervisor sets it on every fresh session.
    spawnSettle bool
    // settled ends the one shot spawn settle: the hold, the
    // regeneration and the first strike run once per session.
    settled bool
    // settleHoldAt stamps the start of the settle hold; the entry log
    // fires once.
    settleHoldAt time.Time
}

// diagState is the diagnostics: the stagnation watch and the paced log
// gates.
type diagState struct {
    // noTargetSince is the moment the target search last came up empty:
    // the patrol waits out the patience before it walks.
    noTargetSince time.Time
    // noPickLogAt paces the targetless diagnostic; zero logs at once, a
    // successful pick re-arms it.
    noPickLogAt time.Time
    // weaponWaitLogAt paces the bare-handed hold log of the engage gate
    // (logWeaponWait).
    weaponWaitLogAt time.Time
    // The stagnation watch (stagnation.go): the last experience and
    // position with their change moments; a stalled value logs the
    // livelock line and drives the soft and hard reset escalation.
    stagXP       int32
    stagXPAt     time.Time
    stagPosX     int32
    stagPosY     int32
    stagPosZ     int32
    stagPosSet   bool
    stagPosAt    time.Time
    stagPosFires int
    stagXPFires  int
    stagHardAt   time.Time
}

// userState is the manual web UI command: the walk plan and its
// follower state.
type userState struct {
    userKind   string
    userX      int32
    userY      int32
    userZ      int32
    userPlanX  int32
    userPlanY  int32
    userPlanZ  int32
    userTarget int32
    userStart  time.Time
    // userDeadline is the priced budget deadline of the manual mesh
    // route (userMoveBudget); zero for the direct walk.
    userDeadline  time.Time
    userMoveAt    time.Time
    userWaypoints []pathfind.Vec3
    userWpIndex   int
    userPathTried bool
    // userSearch is the mesh search contract of the manual plan
    // (planUserWalk), published by the walk plan view; nil for the
    // direct walk.
    userSearch *state.WalkSearch
    // userFrameOffset is the measured z frame offset of the manual plan
    // (click_frame.go).
    userFrameOffset float64
    // userRedirect marks a command that replaced a walk still running
    // server-side: the next request fires at once.
    userRedirect bool
    // userRefusalVariants counts the varied aims the manual follower
    // spent on the waypoint: the refusal is target specific, a sideways
    // aim often crosses where the straight one bounces.
    userRefusalVariants int
    userDeferred        []state.Command
    userLastDist        float64
    userDistAt          time.Time
}

// gearState is the auto equipment: the scorer and the pending
// inventory confirmations.
type gearState struct {
    // equip drives the auto equipment: it equips inventory gear that
    // beats the paperdoll (equip.go).
    equip *equipManager
    // pendingActions holds the pending server confirmations of the
    // in-flight inventory actions, keyed by object id.
    pendingActions map[int32]pendingInventory
}

// kiteState is the archer kite: the retreat cycle, the deferred
// click, the motion ledger and the tunable block.
type kiteState struct {
    // kiteAt paces the kite steps: one retreat per period.
    kiteAt time.Time
    // kiteClickAt schedules the deferred retreat click of the shot
    // cycle (kiteClickWalk): the server replays a MoveToLocation
    // issued inside the bow windup only at the disable end; zero
    // disarms.
    kiteClickAt time.Time
    // kiteClickFor is the fight the deferred click belongs to; a
    // target switch disarms it.
    kiteClickFor int32
    // kiteClickUntil bounds the walk window of the deferred click: the
    // disable end plus the re-engage delay.
    kiteClickUntil time.Time
    // kiteFor is the fight the kite streak counts for; a fresh target
    // resets it.
    kiteFor int32
    // kiteStreak counts the consecutive kite steps: crossing
    // kiteStreakLimit names the unwinnable race in the log, a
    // diagnostic only.
    kiteStreak int
    // kiteHeldFor is the target the cornered hold belongs to.
    kiteHeldFor int32
    // kiteHeldAt stamps the last cornered-hold probe: it re-probes at
    // the kite period.
    kiteHeldAt time.Time
    // kiteWalkX/Y/Z are the endpoint of the in-flight retreat walk:
    // the re-click ladder re-issues the click toward exactly this cell
    // while the character stands still (kiteReclickWalk).
    kiteWalkX int32
    kiteWalkY int32
    kiteWalkZ int32
    // kiteWalkUntil bounds the re-click ladder; past it the forced
    // attack re-request owns the tick.
    kiteWalkUntil time.Time
    // kiteFightFor/X/Y anchor the fight's own ground: the target
    // position at the first retreat resolution. The retreat leash
    // reads this anchor instead of the zone registry, so the kite works
    // on any ground.
    kiteFightFor int32
    kiteFightX   int32
    kiteFightY   int32
    // The motion ledger of the fight (kite_ledger.go): the standing
    // and moving accounting that names every standing second by its
    // reason.
    kiteMotionFor     int32
    kiteMotionAt      time.Time
    kiteMotionMoving  time.Duration
    kiteMotionWindup  time.Duration
    kiteMotionHold    time.Duration
    kiteMotionIdle    time.Duration
    kiteMotionHPStart float64
    kiteMotionHPMin   float64
    // kiteWalkBaseX/Y is the cell the kite walk was issued from - the
    // ladder's dead-click oracle.
    kiteWalkBaseX int32
    kiteWalkBaseY int32
    // kiteReclickAt stamps the last click of the ladder: the re-click
    // pacing and the refusal correlation ride it.
    kiteReclickAt time.Time
    // kiteWalkIssuedAt stamps the moment the walk went out: the silent
    // probe measures the walk age from it.
    kiteWalkIssuedAt time.Time
    // kiteReclicks counts the re-clicks of the current walk: the click
    // bound reads it.
    kiteReclicks int
    // kiteWalkDead latches the dead-endpoint verdict of the walk: one
    // diagnostic line, then the hold follows.
    kiteWalkDead bool
    // kiteBearings is the scratch of the retreat direction: the planar
    // bearings of the chaser train; a train wider than the array keeps
    // its farthest members out of the gap geometry only.
    kiteBearings [kiteMaxTrainMembers]float64
    // kite is the tunable block of the kite fight: DefaultKiteParams,
    // overridden per bot spec (SetKiteParams); NewLoop fills it before
    // the first tick.
    kite KiteParams
}
