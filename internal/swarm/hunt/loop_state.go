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
    target  int32
    lastHit time.Time
    // fightStartAt and fightStartFor hold the start of the CURRENT
    // fight: set when the server confirms the running fight of a
    // new target, keyed by the target id (a stale pair is ignored).
    // engageAt re-anchors on every live fight activity for the
    // stuck timeout, so it cannot measure the fight length.
    fightStartAt  time.Time
    fightStartFor int32
    engageAt      time.Time
    targetSkip    map[int32]time.Time
    // The lure is the ranged luring state of the current pick (nil when
    // no lure runs): the melee answer to a covered target, see
    // lure.go.
    lure *lureState
    // The corpse record of the last kill: the ranged kills (the bow
    // lure, the caster spells) die far from the character, the drops
    // land at the corpse and their broadcast can trail the death
    // packet - the loot phase walks to the corpse and waits the grace
    // window out instead of leaving the loot on the ground (see
    // loop_actions.go).
    killX        int32
    killY        int32
    killZ        int32
    killAt       time.Time
    killPosKnown bool
    killWalked   bool
    killLooted   bool
    // The blind engage recovery state (see loop_los.go): the armed
    // reposition walk, its planned geodata waypoints and the attempt
    // budget for the current target.
    losAt        time.Time
    losWaypoints []pathfind.Vec3
    losWpIndex   int
    losMoveAt    time.Time
    losTried     int
    // skipScratch is the reused dense skip list of the target
    // searches: activeSkips rebuilds it in place every call, so
    // the per tick search filter costs no allocation (the old
    // per call map allocation ran on every engage tick).
    skipScratch []int32
    // avoidScratch is the reused threat buffer of the aggro-aware
    // walk steering (see loop_avoid.go): the scan refills it in
    // place, so the per segment danger pass costs no allocation.
    avoidScratch []state.AggroThreat
    // avoidLogAt paces the detour diagnostic of the walk steering:
    // a busy corridor bends every segment, the log names it once per
    // period instead of every request.
    avoidLogAt time.Time
    // combatAvoidScanAt paces the impending-add threat scan of the
    // running fight (see avoidImpendingAdd): the engage ticks four
    // times a second, the scan pass over the hot records runs at the
    // target search cadence.
    combatAvoidScanAt time.Time
    // combatAvoidAt paces the reposition steps away from an impending
    // aggressive add: one step per period, the walk costs the fight a
    // second of swings.
    combatAvoidAt time.Time
    // combatAvoidUntil marks the movement window the impending-add
    // step owns: a forced attack request interrupts a running walk
    // server-side, so the engage holds its re-requests until the step
    // finished. The kite step of the archer (see kite.go) shares the
    // window: one movement owner per fight step.
    combatAvoidUntil time.Time
    // combatAvoidHoldAt paces the walled-pocket diagnostic of the
    // impending-add step (see avoidImpendingAdd): a fight whose every
    // sidestep lane is walled, over water or outside the leash holds
    // the ground, and the log names it once per period.
    combatAvoidHoldAt time.Time
    // The combat casting state (see combat_skills.go): the pacing
    // timestamps of the strike and spell requests and of the self
    // buff casts, plus the locally tracked reuse windows of the
    // cast skills (the server clock of the real windows starts at
    // the cast start, the local one carries a jitter margin).
    castAt        time.Time
    buffAt        time.Time
    skillReuse    map[int32]time.Time
    profilePicked bool
    // healFlightUntil bounds the in-flight window of the recovery
    // self heal (see recovery_heal.go): the sit request waits it
    // out because the server refuses the sit of a casting
    // character.
    healFlightUntil time.Time
    engLastDist     float64
    engDistAt       time.Time
}

// lootState is the loot phase state: the in-flight pickup and the
// skipped drops.
type lootState struct {
    lootID     int32
    lootAt     time.Time
    lootMoveAt time.Time
    skipped    map[int32]time.Time
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
    // segment plan (see click_frame.go): the difference between the
    // server vouched standing z and the mesh frame height of the same
    // cell, calibrated when a fresh plan is accepted and re-measured
    // by every re-path (each plan starts at the cell the character
    // actually stands on). Every plan derived click z rides it into
    // the server frame before the click guards and the wire see it
    // (clickWaypoint, the route samples, the escape hops, the varied
    // aims); a plan without a measurable offset keeps it zero and
    // rides the raw mesh z.
    segmentFrameOffset float64
    moveAt             time.Time
    // flowScanAt paces the corner pre-click scans: at most one scan
    // per flowPreClickGap whatever its answer, so a route point no
    // far target serves cannot turn the ring into a per tick probe.
    flowScanAt time.Time
    stuckAt    time.Time
    stuckX     int32
    stuckY     int32
    // stuckWP is the waypoint cursor the stuck window opened at: a
    // cursor advance is net progress whatever the geometry (a
    // passed waypoint is ground covered).
    stuckWP int
    // stuckBest is the best distance to the current waypoint seen
    // inside the stuck window: only an improvement beyond the
    // progress margin resets the window, so an oscillation that
    // moves without closing on its target stays stuck.
    stuckBest float64
    // stuckFast arms after the first stuck skip of a trip: subsequent
    // stuck detections use the shorter stuckFastTimeout so the walker
    // cycles through the remaining waypoints quickly instead of waiting
    // the full stuckTimeout for each one. resetTownTrip and
    // startWalkSegment clear it.
    stuckFast bool
    // skipX/skipY/skipArmed record the cell the last waypoint skip of
    // the frozen episode left the character on (the stuck skip of
    // stuckTownWalk and the aggro circle skip of clickWaypoint both
    // run through skipMoveFresh): a repeat skip on the same cell is
    // denied, because a skip that moved the character nothing proved
    // the click transport dead on this ground - repeating it only
    // marches the cursor (and the eventual chord walk) away from the
    // plan while the character stands still (the 2026-09-21 town walk
    // report: 49 stuck skips advanced the cursor 15 -> 63 of 87, the
    // aim 18,000 units out, through 3.5 frozen minutes without a unit
    // of movement or a single re-path) and starves the recovery ladder
    // that owns the dead click transport.
    skipArmed bool
    skipX     int32
    skipY     int32
    // moveStartAt/X/Y are the move start watchdog of the walker (see
    // noteMoveStart): the deadline arms when a walk click goes out,
    // a position change or the server movement broadcast clears it
    // and a deadline that passes with the character still standing
    // on the baseline cell names the click dead - the recovery
    // ladder fast forwards instead of waiting the full stuck window
    // (the owner rule of the 2026-09-19 round: a movement command
    // that did not start the movement switches the recovery mode at
    // once).
    moveStartAt time.Time
    moveStartX  int32
    moveStartY  int32
    // forceStuck arms when the move start watchdog names a fresh
    // click dead: the next walkStuck verdict runs the recovery at
    // once (the stuck window check is skipped for that one verdict)
    // instead of standing out the remaining window.
    forceStuck bool
    rePaths    int
    // extendArmed arms the short click extension of the town walk
    // follower: it flips on the first stuck that finds no clear
    // successor waypoint (the pinned cursor - the plain clicks of
    // the segment proved they do not move the character) and clears at
    // the trip boundaries. While armed, the clicks whose primary
    // target sits under the server rescue floor (minWalkClick) or
    // behind the character on the route re-aim at the forward
    // route samples (the 2026-09-11 11:34 village return dump: the
    // 22 unit first waypoint click froze the character through two
    // whole trip cycles - a collapsed click under the findPath
    // rescue threshold is silently canceled by the server).
    extendArmed   bool
    repathX       int32
    repathY       int32
    frozenRepaths int
    // cursorEscape carries the cursor key escape of a click-refusing
    // cell (see beginCursorKeyEscape): the armed state, the claimed
    // dry steps toward the escape aim, the claim cursor and the
    // follow bookkeeping. The escape emulates the arrow keys of the
    // official client - the movement mode 0 arm plus the claimed
    // ValidatePosition stream the server follows without any click
    // validation - because the 2026-09-14 15:10 report proved the
    // user's server answers NO mouse click from the dump cell at
    // all: even the official client stood frozen on the plaza cell
    // until the player walked it out with the arrows.
    cursorEscape cursorEscapeState
    // cursorEscapes counts the cursor key escape attempts spent by
    // the current town trip: a server that ignores the claims too
    // (keyboard movement disabled, a build without the cursor key
    // branch) burns the attempts and the trip aborts with the
    // honest reason instead of grinding the escape ladder forever.
    cursorEscapes int
    // segmentRefused latches the online refusal evidence of the current
    // segment: the server answered a click of this segment with
    // ActionFailed while the character stood still (see
    // refusalEvidence). The varied aim machinery reads it - a
    // segment the server refused needs its targets varied, not the
    // same aim re-clicked (the 2026-09-14 10:18 dump: the server
    // refused every click for its own reasons). It clears on the
    // segment boundaries with the other segment state.
    segmentRefused bool
    // segmentRefusedX/Y remember the cell where the segment's first refusal
    // verdict latched (see latchSegmentRefused): a stuck verdict with
    // refusal evidence ON THAT VERY CELL names a refusing pocket -
    // the server answers no click from the ground the character
    // stands on (the 2026-09-14 15:10 report: even the official
    // client's mouse clicks died on the plaza cell) - and the cursor
    // key escape owns the recovery at once, the varied aims only
    // grind more refused clicks from the same ground. It clears on
    // the trip boundaries with the segmentRefused latch.
    segmentRefusedX int32
    segmentRefusedY int32
    // refusalVariants counts the varied aim attempts spent on the
    // current segment (see stuckTownWalk): the refusal answer of the
    // server is target specific - a shorter prefix or a sideways
    // offset of the same waypoint often walks where the plain click
    // was refused - so the stuck verdict varies the aim before it
    // re-paths the whole segment.
    refusalVariants int
    // zoneRefusalX/Y remembers the cell where the server's refusal
    // answers last stalled the direct zone segments: a later refusal
    // stall on the very same cell (no ground covered in between)
    // holds the return backoff instead of re-arming the cycle the
    // server keeps refusing (see noteZoneSegmentStall).
    zoneRefusalX int32
    zoneRefusalY int32
    // segmentRadius is the approach radius the current town segment searches
    // its route within: the npc search radius (npcApproachRadius) for
    // the npc destinations (the merchant stops, the teacher stops, the
    // delevel guard walks - the plan ends at the npc's own point, the
    // user rule of the 2026-09-20 report: a wide ring ends the plan at
    // the first walkable surface inside its ball - the shop edge - and
    // the character never enters the shop), the wide trip ring
    // (tripApproachRadius) for the returns. The re-paths of the segment
    // inherit it.
    segmentRadius float64
    // segmentSearch is the mesh search contract the current segment's plan
    // answered (the approach radius startWalkSegmentSearch ran with):
    // the walk plan view publishes it,
    // so the 3D pathfind link rebuilds the very search the walk
    // follows instead of a lookalike (the 2026-09-19 route mismatch:
    // the viewer rebuilt a dry zone return with the swim filter and
    // folded the answer into a straight water blind chord). Nil for
    // the direct segments no mesh search produced.
    segmentSearch *state.WalkSearch
    farmX         int32
    farmY         int32
    farmZ         int32
    zoneReturn    bool
    zoneFails     int
    // zoneSegmentAt/zoneSegmentX/zoneSegmentY hold the no-movement
    // window of the direct zone segments (see noteZoneSegmentStall):
    // the baseline arms on
    // the first segment send, a cell change re-baselines it and a hold
    // past the stuck timeout re-arms the pathfound zone return.
    zoneSegmentAt time.Time
    zoneSegmentX  int32
    zoneSegmentY  int32
    roadFights    int
    // zoneSegmentLogAt paces the walled direct segment diagnostic of the
    // zone return escalation (see guardZoneSegmentClick): the refusal
    // repeats every second while the character stands in the
    // pocket, the log names the wall once per period.
    zoneSegmentLogAt time.Time
}

// townState is the town trip: the merchant stops and the sell pacing.
type townState struct {
    // tripAbortRun counts the consecutive aborted trips of this
    // session: past the escalation threshold the trip cooldown
    // doubles per abort (capped), so a deployment whose paths never
    // validate stops burning a tick every five minutes (the observed
    // six hour run: 47 identical "no walkable path" aborts). A trip
    // that ends any other way resets the streak.
    tripAbortRun      int
    sellAt            time.Time
    sellPhaseAt       time.Time
    merchantID        int32
    merchantPick      time.Time
    merchantDeckUntil time.Time
    // merchantPull paces the attack analog pull ladder of the deck
    // wait (one click per select period, see approachMerchant).
    merchantPull time.Time
    // merchantPulled holds the object id the post selection pull
    // already fired for (the practice fires once per merchant, a
    // re-pick to a different object id arms it again).
    merchantPulled int32
    sold           map[int32]bool
    tripStart      time.Time
    tripEndedAt    time.Time
}

// shopState is the town shopping: the frozen purchase plan, the buy
// confirmations and the replacement sales.
type shopState struct {
    // cityShoppingTriedAt stamps the last city shopping errand
    // attempt for the retry cooldown (see city_shopping.go).
    cityShoppingTriedAt time.Time
    // The town trip shopping state (see shopping.go): the frozen
    // purchase plan of the running trip, the merchant stops it
    // distributes into, the request pacing of the buys, the in-flight
    // buy batch awaiting its inventory confirmation and the cached
    // plan of the shopping trigger.
    tripPlan     []gear.Purchase
    tripStops    []tripStop
    buysPlanned  bool
    buyAt        time.Time
    buyRequested []gear.Purchase
    // buyBaseline holds the owned stack count of every item id of the
    // in-flight batch at the moment the request went out: the
    // stackable orders arrive as a count growth (the server merges
    // the delivery into the carried stack), so the arrival gate
    // compares the live count against the baseline plus the order.
    buyBaseline       map[int32]int32
    buyConfirmAt      time.Time
    buyRetries        int
    shoppingPlanAt    time.Time
    shoppingPlanCache []gear.Purchase
    // shoppingPlanAdena holds the adena the cached queue was planned
    // against: the widget view shows the planning wallet, not the
    // live one drifting with the loot of the same tick.
    shoppingPlanAdena int64
    // shoppingViewCache holds the built ShoppingPlanView that
    // corresponds to shoppingPlanCache. The view is rebuilt from the
    // cached plan every tick (200ms) without this cache, which on the
    // 100 bot fleet is the dominant allocation source (44 percent of
    // all heap allocations under live profiling): each rebuild
    // allocates a []ShoppingEntryView slice plus runs six npcdata
    // dictionary lookups per entry. Caching the built view alongside
    // the plan collapses the per tick view cost to a pointer copy
    // for the 25 ticks between plan recomputes.
    shoppingViewCache state.ShoppingPlanView
    // The sell first step of the replacement purchases (see
    // stepReplacementSales): the planned purchases that displace
    // equipped gear sell the displaced pieces before buying, so the
    // queue holds the object ids to unequip, replaceSelling the
    // collected sale batch and the flags pace the unequip requests
    // and mark the step done.
    replacePlanned   bool
    replaceDone      bool
    replaceQueue     []int32
    replaceSelling   []state.InventoryItem
    replaceSellSent  bool
    replaceUnequipAt time.Time
    replaceWaitAt    time.Time
    replaceTried     int
    // tripGearStart snapshots the paperdoll object ids the trip
    // started with (and their item ids) so the trip exits can arm
    // the gear debt: a slot the trip left empty whose piece is
    // gone from the inventory was sold for a replacement that
    // never landed (see gearDebtCheck, the 2026-09-12 pantsless
    // dump report).
    tripGearStart    [state.PaperdollSlots]int32
    tripGearStartIDs [state.PaperdollSlots]int32
    // gearDebt holds the paperdoll slot indexes the town trip
    // machinery stranded (the map value is the item id of the lost
    // piece for the logs): the debt survives the trip end and runs
    // the refill trip on the gear run cooldown until the slot is
    // dressed again (gearGapRunWanted clears the refilled entries).
    gearDebt map[int]int32
}

// learnState is the skill learning: the teacher stops, the lesson
// confirmations and the guide buff stops.
type learnState struct {
    // The skill learning state (see learning.go): the cached learning
    // queue of the trip trigger, the selected village teacher with
    // its click pacing and deck retry window, and the in-flight
    // lesson awaiting its SkillList confirmation with the retry
    // budget.
    learnPlanCache    []state.SkillPlanEntry
    learnPlanAt       time.Time
    learnPlanRevision uint64
    teacherID         int32
    teacherPick       time.Time
    teacherDeckUntil  time.Time
    // teacherWalkUntil bounds the whole close approach of the teach
    // stop: the ring walk clicks the npc approach point until the
    // character stands right by the teacher, and the window answers
    // the two dead ends - the ring the server refuses to walk (the
    // talk then fires from wherever within the interaction distance)
    // and the teacher on a deck the ring cannot reach (the stop
    // skips, the trip continues).
    teacherWalkUntil time.Time
    learnRequested   *lessonTarget
    learnConfirmAt   time.Time
    learnRetries     int
    learnAt          time.Time
    learnRevision    uint64
    // The Newbie Guide support magic state (see guide_buffs.go): the
    // found guide npc with its pick pacing, the dialog anchor and
    // the close approach window, plus the refusal cooldown the stop
    // arms when the expected buffs never land (the cooldown outlives
    // the trip - the gate of the server stays refused for a while).
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
    // delevelMedian is the live median mob level of the held ground
    // at the deleveling start: the trigger evidence the webui
    // message renders next to the target level (the character level
    // is too high for exactly this ground).
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
    // delevelAborts counts the consecutive ABORTED delevelings (no
    // completed one in between): every abort arms the escalating
    // wait through delevelAbortWait, only a finished deleveling
    // resets the streak - the systemic guard against the farm and
    // village commute of never completing attempts.
    delevelAborts int
}

// groundState is the hunting ground registry: the zone list, the held
// cell and the per-zone death bookkeeping.
type groundState struct {
    // The multi zone hunting state (see zones.go): the registry of the
    // deployment, the picked and the manually overridden zone. The
    // cell mode (see cell_policy.go) replaces the registry with the
    // Voronoi partition alternative - the zone fields then mirror
    // the patrol square of the held cell.
    zones []HuntingZone
    cell  *cellHunter
    // cellPin is the pending ground lock of SetCellPin: the pin the
    // acceptance fleet installs BEFORE the registry exists lands
    // here and SetHuntingCells re-applies it when the registry
    // arrives (the hunter itself carries the live copy).
    cellPin      string
    zoneRegion   string
    zonePickedID string
    zoneCheckAt  time.Time
    // The death regression bookkeeping of the multi zone hunting (see
    // zones.go): the per zone death counts of the session, the band
    // cap a demoted zone installs (negative = no cap) and the character
    // level the bookkeeping last reset at.
    zoneDeaths     map[string]int32
    zoneDeathCap   int32
    zoneDeathLevel int32
    // zoneEmptySince arms the rotation of a cleared-out square (see
    // zones.go): the timestamp the square turned mob-less, zero while
    // mobs remain or the character fights, rests or walks.
    zoneEmptySince time.Time
    // zoneEmptyUntil holds the empty cooldown of the rotated-away
    // squares (see zones.go): the zone stays out of the rotation
    // contest until its expiry so the sweep moves forward through
    // the band instead of ping ponging between two squares.
    zoneEmptyUntil map[string]time.Time
    // zoneMobPriority holds the engage bias of the picked zone (see
    // zones.go): the template id to priority map built from the mob
    // list of the active ground, nil for zones without mob data.
    zoneMobPriority map[int32]int32
}

// safetyState is the defensive layer: the flee episode, the escape
// watchdogs, the pile up run and the post relogin settle.
type safetyState struct {
    // fleeAt paces the escape walk requests: the escape must not
    // wait out the attack request pacing of the engage (the last
    // forced attack fired moments before the threshold crossed).
    fleeAt time.Time
    // fleeSince tracks the start of the running flee episode: the
    // escape segments never stop the chase by themselves when the
    // pursuing pack is fast, and past the fleeLogoutAfter budget
    // the session logs out to reset the aggro instead of running
    // forever. A recovered health or a fresh fight clears it.
    fleeSince time.Time
    // fleeGapAt and fleeGap anchor the closure-rate watchdog of the
    // escape: the moment the probe opened and the threat distance
    // then. A chaser that keeps pace holds the gap flat while the
    // character walks the whole budget (the 14:03:09 dump: 21 s at
    // ~19 units), so a window that fails to widen the gap ends the
    // session early instead (see escapeNotOpening).
    fleeGapAt time.Time
    fleeGap   float64
    // escapeAnchorAt/X/Y anchor the escape no-progress watchdog: the
    // spot and the moment the current observation window opened. A
    // window that lapses with less than escapeNoProgressDistance
    // covered names a dead escape (the H-006 silent drop, a walk
    // with no accepted segment) and logs the session out instead of
    // standing in the blows. Every accepted window re-anchors.
    escapeAnchorAt time.Time
    escapeAnchorX  int32
    escapeAnchorY  int32
    // escapeWalkAt stamps the last escape walk issued in the current
    // episode: the no-progress window only opens with an issued walk,
    // so the stand-up settle of a sitting character is never
    // mistaken for a stalled escape.
    escapeWalkAt time.Time
    // panicAt marks the armed pile up run (the loop run between
    // the social pile up and its deferred logout): zero while no
    // pile up forced one, otherwise the moment the pack was
    // spotted. The run is committed once armed - the logout
    // happens at panicRunDistance from the panic point no matter
    // how the pack thins out on the way (see panicPileUpRun).
    panicAt time.Time
    // panicX and panicY anchor the pile up run: the aggro point
    // the character was standing on when the pack piled up. The
    // logout fires only after the run opened panicRunDistance
    // units between the character and this point.
    panicX int32
    panicY int32
    // logoutDone marks the one shot emergency logout: the session
    // unwinds within a second of the request, the flag keeps the
    // dying ticks quiet.
    logoutDone bool
    // spawnSettle arms the post relogin settle (see settle.go): the
    // supervisor enables it on every fresh session, the tests and
    // the manual sessions keep it off.
    spawnSettle bool
    // settled marks the one shot spawn settle: the hold, the
    // regeneration and the first strike run once per session, the
    // flag ends them for good.
    settled bool
    // settleHoldAt stamps the start of the settle hold: the entry
    // log line fires once, the field keeps it from repeating.
    settleHoldAt time.Time
}

// diagState is the diagnostics: the stagnation watch and the paced log gates.
type diagState struct {
    // noTargetSince tracks when the target search last came up
    // empty: the patrol toward the zone center waits out the
    // patience before it walks.
    noTargetSince time.Time
    // noPickLogAt paces the targetless diagnostic log (see
    // logNoPickableTargets): zero means the next empty pick logs
    // at once, a successful pick re-arms it.
    noPickLogAt time.Time
    // weaponWaitLogAt paces the bare-handed hold log of the engage
    // gate (see logWeaponWait): the hold spans the weapon run
    // cooldown window, one line per period keeps the log readable.
    weaponWaitLogAt time.Time
    // The stagnation watch state (see stagnation.go): the last
    // observed experience with the moment it last changed, and
    // the last observed position with the moment the character
    // last stood elsewhere. A value that stops moving for its
    // window logs the livelock line (the web UI event feed and
    // the bot log both carry it, the state dump diagnostics show
    // the stall ages) and drives the recovery escalation: the
    // fire counters climb while a window keeps re-firing and a
    // movement or an experience change resets them, the soft
    // reset clears the frozen loop state on the first position
    // fire and the hard reset rebuilds the session through the
    // emergency logout once the soft reset proved not enough.
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

// userState is the manual web UI command: the walk plan and its follower state.
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
    // userDeadline is the priced budget deadline of the current
    // manual mesh route (see userMoveBudget): zero for the direct
    // server routed walk, which keeps the base window.
    userDeadline  time.Time
    userMoveAt    time.Time
    userWaypoints []pathfind.Vec3
    userWpIndex   int
    userPathTried bool
    // userSearch is the mesh search contract of the manual walk plan
    // (the swim filter, the user approach radius - see planUserWalk):
    // the walk plan view publishes it, the 3D pathfind link rebuilds
    // the very search. Nil for the direct manual walks no mesh search
    // produced.
    userSearch *state.WalkSearch
    // userFrameOffset is the measured z frame offset of the manual
    // walk plan (the segmentFrameOffset of the user follower, see
    // click_frame.go): calibrated when planUserWalk accepts a fresh
    // mesh route and ridden by the follower's waypoint clicks.
    userFrameOffset float64
    // userRedirect marks a manual command that replaced a walk
    // still running on the server: the next walk request fires at
    // once instead of waiting for the old walk to finish.
    userRedirect bool
    // userRefusalVariants counts the varied aim attempts the manual
    // walk follower spent on the current waypoint (the user walk
    // answer to a refused click, see sendUserVariedAim): the refusal
    // of the server is target specific, a shorter prefix or a
    // sideways offset of the same waypoint often validates where the
    // plain aim bounced (the temple entrance round: the first click
    // walked the character onto the porch, the server stopped its own
    // walk at the door frame and answered every straight re-click
    // from the porch with ActionFailed - the east sideways aim
    // crosses the door opening and walks). It resets on the plan
    // boundaries and on every waypoint the follower advances past.
    userRefusalVariants int
    userDeferred        []state.Command
    userLastDist        float64
    userDistAt          time.Time
}

// gearState is the auto equipment: the scorer and the pending
// inventory confirmations.
type gearState struct {
    // equip drives the auto equipment: it equips inventory gear that
    // beats the paperdoll of the character (see equip.go).
    equip *equipManager
    // The pending server confirmations of the in flight inventory
    // actions, keyed by object id (see markInventoryAction,
    // inventoryItemAllowed and gateInventoryCommand): the burst plan
    // of the auto equipment and the manual commands share the map,
    // each action holds only its own item and write set back.
    pendingActions map[int32]pendingInventory
}

// kiteState is the archer kite: the retreat cycle, the deferred
// click, the motion ledger and the tunable block.
type kiteState struct {
    // kiteAt paces the archer kite steps (see kite.go): one retreat
    // per period, the rest of the cycle stands and shoots.
    kiteAt time.Time
    // kiteClickAt schedules the deferred retreat click of the live
    // shot cycle (see kiteClickWalk, kite.go - the issue #70
    // findings): the server saves a MoveToLocation issued inside
    // the bow windup and replays it only at the disable end, where
    // the re-shot cancels it, so the retreat click waits the windup
    // end plus the safety lead before it goes out. The zero value
    // disarms the schedule.
    kiteClickAt time.Time
    // kiteClickFor is the fight the deferred click belongs to (see
    // kite.go): a target switch - the old target died, a fresh pick
    // - disarms the schedule, the retreat serves the fight that
    // armed it.
    kiteClickFor int32
    // kiteClickUntil bounds the walk window of the deferred click:
    // the disable end of the arming shot plus the re-engage delay -
    // the walk owns the reuse tail (the windup end to the disable
    // end) and the forced attack re-request fires the moment the
    // window lapses.
    kiteClickUntil time.Time
    // kiteFor is the fight the kite streak counts for (the hunt
    // target): a fresh target resets the streak (kiteStreak). The
    // armed threat may be a train member - the streak still counts
    // the steps of the running fight.
    kiteFor int32
    // kiteStreak counts the consecutive kite steps of the kiteFor
    // target: crossing kiteStreakLimit names the unwinnable race
    // (a chaser at least as fast as the character) in the log - a
    // DIAGNOSTIC since the round-14 always-run redesign, the
    // retreat keeps running straight (the standing "fight it out"
    // answer is gone).
    kiteStreak int
    // kiteHeldFor is the target the cornered hold belongs to (see
    // kite.go): the hold of one target never paces another.
    kiteHeldFor int32
    // kiteHeldAt stamps the last cornered-hold probe (see kite.go):
    // the hold re-probes at the kite period, not every tick, and the
    // diagnostic line lands once per hold episode.
    kiteHeldAt time.Time
    // kiteWalkX/Y/Z carry the endpoint of the in-flight kite retreat
    // walk: the re-click ladder (see kiteReclickWalk, kite.go, issue
    // #60 the second round) re-issues the click toward exactly this
    // cell while the movement window runs and the character stands
    // still on the issue cell - the single click of the first round
    // died silently too often (the stance transition swallow, the
    // endpoint cell refusal, the H-006 silent drop) and the
    // character stood through the whole bow reload the owner's dump
    // reports. The fields arm through kiteIssueWalk and clear with
    // the ladder (a fresh step re-arms them whole).
    kiteWalkX int32
    kiteWalkY int32
    kiteWalkZ int32
    // kiteWalkUntil bounds the re-click ladder with the same window
    // the kite step set (kiteStepWindow plus the re-engage delay):
    // past it the forced attack re-request owns the tick, and a
    // re-click would only fight the re-engage for the movement. The
    // zero value disarms the ladder.
    kiteWalkUntil time.Time
    // kiteFightFor/X/Y anchor the current fight's own ground (the
    // round-14 location-general redesign): the position of the
    // fight target at the fight's FIRST retreat resolution (see
    // kiteResolveAndClick, kite.go). The retreat leash
    // (kiteTerrainLane) reads the anchor INSTEAD of the
    // hunting-square registry, so the kite works on any map, any
    // ground, with or without a zone registry - the generated
    // elven/dion tables own the target economy alone, never the
    // fight. A fresh target re-anchors.
    kiteFightFor int32
    kiteFightX   int32
    kiteFightY   int32
    // The kite motion ledger of the current fight (see
    // kite_ledger.go, the round-14 feedback ask): the per-fight
    // standing/moving accounting that answers "does the archer run
    // all the time" directly in the live log at the fight end -
    // every standing second is named by its reason (the windup the
    // server forces, the hold verdict, the unattributed idle).
    kiteMotionFor     int32
    kiteMotionAt      time.Time
    kiteMotionMoving  time.Duration
    kiteMotionWindup  time.Duration
    kiteMotionHold    time.Duration
    kiteMotionIdle    time.Duration
    kiteMotionHPStart float64
    kiteMotionHPMin   float64
    // kiteWalkBaseX/Y is the cell the kite walk was issued from -
    // the ladder's dead-click oracle: a character still standing on
    // this cell past the probe pace never moved whatever the
    // next dump says about the clicks, and a character that left it
    // tells the ladder the click moved something after all.
    kiteWalkBaseX int32
    kiteWalkBaseY int32
    // kiteReclickAt stamps the last click of the ladder (the initial
    // click included): the kiteReclickPeriod pacing rides it, and the
    // refusal correlation (kiteClickRefused) reads it as the send
    // baseline of the ActionFailed attribution.
    kiteReclickAt time.Time
    // kiteWalkIssuedAt stamps the moment the kite walk went out: the
    // silent probe of the ladder measures the walk age from it (a
    // walk unanswered past kiteProbeElapsed with the character still
    // on the issue cell names the endpoint dead without any refusal
    // evidence - the H-006 silent drop signature).
    kiteWalkIssuedAt time.Time
    // kiteReclicks counts the re-clicks spent on the current kite
    // walk: the click bound reads it (the packet budget of a dead
    // transport).
    kiteReclicks int
    // kiteWalkDead latches the dead-endpoint verdict of the current
    // walk (the refusal evidence or the silent probe set it): one
    // diagnostic line per walk, the hold follows the latch.
    kiteWalkDead bool
    // kiteBearings is the scratch of the retreat direction: the
    // planar bearings of the chaser train the last resolution
    // collected (kiteTrainDirection fills it every call - the
    // centroid sum and the encircled gap read the same enumeration).
    // A train wider than the array keeps its farthest members out of
    // the gap geometry only (the sum stays whole) - a train that
    // wide is the flee machinery's logout case, not a kite shape.
    kiteBearings [kiteMaxTrainMembers]float64
    // kite carries the tunable block of the kite fight (see
    // kite.go): the loop starts at DefaultKiteParams - the shipped
    // tuning the acceptance scenario pins - and the launch config
    // overrides it per bot spec (SetKiteParams, the tuning knobs of
    // owner issue #29). The zero value never leaks: NewLoop fills
    // the defaults before the first tick.
    kite KiteParams
}
