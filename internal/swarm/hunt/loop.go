// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package hunt drives the automatic hunting behavior of a bot: attack the
// nearest attackable npc, pick up the loot it drops around the corpse and
// keep the inventory of the long living session working by destroying
// junk items when the slots or the weight run out.
package hunt

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/session"
	"github.com/melg8/swarm/internal/swarm/state"
)

// GameAPI abstracts the game actions the hunt loop needs. The
// connection.GameClient implements it.
type GameAPI interface {
	// AttackTarget sends the attack request for a target: the Mobius
	// server answers the first request for a new target with
	// MyTargetSelected (the request only selects it) and resolves the
	// repeated request to a forced attack.
	AttackTarget(objectID int32) error
	// WalkTo makes the character walk to a world point, like a ground
	// click of the official client.
	WalkTo(x int32, y int32, z int32) error
	// PickupItem clicks a ground item to walk to it and pick it up.
	PickupItem(item state.LootItem) error
	// ActionSitStand toggles between sitting and standing.
	ActionSitStand() error
	// RestartAtVillage revives a dead character at the nearest village.
	RestartAtVillage() error
	// DestroyItem destroys inventory items.
	DestroyItem(objectID int32, count int32) error
	// SellItems sells inventory items to the shop merchant.
	SellItems(items []state.InventoryItem) error
	// BuyItems buys items from the buylist of the selected merchant
	// (one list per request, paced by the transaction flood
	// protector).
	BuyItems(listID int32, items []gear.Purchase) error
	// UseItem uses an inventory item: equippable items toggle their
	// equipped state, the same packet equips and unequips.
	UseItem(objectID int32) error
	// DropItem drops an inventory item on the ground at the given
	// position (the server accepts drops at the feet only).
	DropItem(objectID int32, count int32, x int32, y int32, z int32) error
	// ClickObject selects a world object through the plain client
	// click: the server NpcClick handler selects the npc and
	// remembers it as the last folk the character talked to -
	// the teacher the lesson requests resolve their trainer
	// through.
	ClickObject(objectID int32) error
	// ClearTarget drops the npc selection a conversation left
	// behind: the self click replaces the server side selection
	// (see GameClient.ClearTarget), so the leftover villager
	// never reads as a hunt target of the next engage.
	ClearTarget() error
	// AcquireSkill learns one lesson of the class skill tree at
	// the last folk teacher: the SP is charged and the demanded
	// spellbook consumed.
	AcquireSkill(skillID int32, level int32) error
	// UseMagicSkill casts a learned active skill - a strike at
	// the selected target or a self buff.
	UseMagicSkill(skillID int32) error
	// RequestLogout ends the session: the logout packet goes out
	// first (the server answers it while the combat stance lapsed)
	// and the connection closes either way (the server stores a
	// character that left mid combat fifteen seconds after the
	// combat ends).
	RequestLogout() error
	// SendBypass sends a RequestBypassToServer command to the server
	// (the gatekeeper teleport flow, the H-002 verification). The
	// command is the raw bypass string (the client strips the
	// "bypass -h " prefix the html button carries).
	SendBypass(command string) error
	// LastHTMLDialog returns the npc object id and the html body of
	// the last NpcHTMLMessage the server sent (the gatekeeper dialog
	// the bot reads the bypass buttons from). A zero npcObjID means
	// no dialog arrived yet.
	LastHTMLDialog() (npcObjID int32, html string)
}

// Timing and threshold constants of the hunt loop.
const (
	// tickPeriod is the decision cadence of the loop. The Mobius
	// PlayerActionFloodProtector accepts one player action per second,
	// so the actual requests stay rate limited by the engage periods
	// below; the short cadence only makes the state transitions (target
	// died, loot finished, health recovered) act within a quarter
	// second instead of a full one.
	tickPeriod = 250 * time.Millisecond
	// lootRadius is the distance around the character within drops are
	// picked up after a kill.
	lootRadius = 900.0
	// attackNearestRange bounds the target search radius.
	attackNearestRange = 1500.0
	// lootApproachRadius is the distance within which a ground item is
	// clicked instead of walked to: the server AI covers the last
	// stretch and executes the pickup.
	lootApproachRadius = 60.0
	// selectPeriod is the minimum pause between two nearest target
	// selections. The server accepts one player action per second
	// (PlayerActionFloodProtector), so selecting faster is pointless.
	selectPeriod = 1 * time.Second
	// reengageHealthPercent is the HP level above which the next target
	// is engaged immediately after a kill instead of resting. Below it
	// the loop waits for the regeneration - the same threshold as the
	// sit down, so a hurt character sits right away instead of standing
	// around: 30 percent may not be enough to kill the next mob and
	// turns every fight into a death risk.
	reengageHealthPercent = 60.0
	// sitDownHealthPercent is the HP level below which the resting
	// character sits down: the sitting regeneration is much faster.
	sitDownHealthPercent = 60.0
	// standUpHealthPercent is the HP level at which a sitting character
	// stands up again and resumes the hunt.
	standUpHealthPercent = 90.0
	// restRetryPeriod guards the sit/stand toggle against confirmation
	// lag: the ChangeWaitType broadcast confirms each transition and a
	// repeat is only sent when the flip never happened (lost packet),
	// so a slow confirmation can never toggle the character back.
	restRetryPeriod = 3 * time.Second
	// standSettlePeriod holds the movement through the server side
	// stand animation: the ChangeWaitType broadcast confirms the
	// standing at once, but the server keeps the character
	// paralyzed on the REST intention for the 2.5 s StandUpTask
	// window after it - move requests of that window bounce off
	// ActionFailed. Waiting the window out (plus a margin) makes
	// the first walk request land on a movable character.
	standSettlePeriod = 3 * time.Second
	// deathRestartPeriod is the pause between village restart requests
	// of a dead character: the server keeps a short death delay and
	// refuses early revives, so the request retries until it lands.
	deathRestartPeriod = 5 * time.Second
	// engageRetryPeriod is the pause between repeated forced attack
	// requests for the selected target. The server accepts one player
	// action per second (PlayerActionFloodProtector), so one second is
	// the fastest useful cadence.
	engageRetryPeriod = 1 * time.Second
	// pickupTimeout is how long one pickup attempt may take before the
	// item is skipped. It must cover the walk to the farthest item of
	// the loot radius plus the server side pickup.
	pickupTimeout = 20 * time.Second
	// pickupRetryDelay is how long a failed item stays skipped.
	pickupRetryDelay = 30 * time.Second
	// cleanupSlotPercent destroys junk at this inventory fill level.
	cleanupSlotPercent = 70.0
	// cleanupWeightPercent destroys junk at this weight level.
	cleanupWeightPercent = 75.0
	// destroyBatch is the number of junk items destroyed per cleanup.
	destroyBatch = 4
	// zoneReturnFailBudget bounds the consecutive pathfound zone return
	// legs that end without reaching the zone (a stuck walk aborts the
	// leg): past the budget the return falls back to the direct legacy
	// legs, which at least keep the character moving home.
	zoneReturnFailBudget = 3
	// roadFightBudget bounds the consecutive fights the out of zone
	// adoption may start during one walk home: the aggressive
	// territory on the road feeds a fresh attacker every respawn
	// window, so without a cap the adoption holds the character on
	// the road forever (the 2026-09-11 08:04 parallel round: the
	// walk home never resumed, the farm leg timed out on the road
	// fights). Past the budget the walk home continues through the
	// blows - the flee flow owns the hurt case, the mobs leash back
	// once the character leaves the aggro radius. The budget resets
	// on the zone entry.
	roadFightBudget = 3
	// engageStuckTimeout bounds how long the engage keeps re-requesting
	// a target that never actually starts the fight: the usual cause
	// is a stale server side selection (an abrupt disconnect left the
	// auto attack running, the target died and the server keeps the
	// corpse selected - it never clears the selection, only the next
	// selection of a DIFFERENT object replaces it), so every forced
	// attack on the same object id comes back refused forever.
	engageStuckTimeout = 12 * time.Second
	// userEngageRadius is the melee approach distance of an attack
	// command: inside it the forced attack request starts the swings,
	// outside it the chase (or the fallback walk) closes the distance
	// first.
	userEngageRadius = 150.0
	// chaseProgressWindow bounds one progress sample of a chase: the
	// distance to the target is measured once per window.
	chaseProgressWindow = 3 * time.Second
	// chaseProgressStep is the distance a healthy chase closes within
	// one progress window; less than that counts as stalled.
	chaseProgressStep = 50.0
	// engageSkipDelay keeps a stuck target out of the target search:
	// the next selection of a different object already breaks the
	// stale state, the delay only stops the immediate re-pick of the
	// very same nearest npc.
	engageSkipDelay = 30 * time.Second
	// targetMaxLevelSlack bounds the mob level the engage initiates
	// on above the character level: two levels keep the experience
	// flow without turning every pull into a death risk (the
	// observed death ran at half health into a level 10 mob).
	targetMaxLevelSlack = 2
	// escapeHealthPercent is the HP level below which a running
	// fight is dropped and the character runs: pressing on below a
	// quarter of the health bar dies far more often than it kills.
	escapeHealthPercent = 25.0
	// losingFightGap is the health lead a target may hold over the
	// hurt character before the fight counts as lost (the level
	// gap pulls, the adds that joined a social pack).
	losingFightGap = 25.0
	// escapeTargetBeatenPercent is the target health below which a
	// hurt character finishes the fight instead of fleeing: the
	// kill is one swing away.
	escapeTargetBeatenPercent = 15.0
	// fleeSkipDelay keeps a fled target out of the search: walking
	// straight back into the mob the character just ran from
	// re-creates the same death risk at lower health.
	fleeSkipDelay = 2 * time.Minute
	// escapeWalkDistance is one escape leg of the flee flow.
	escapeWalkDistance = 700.0
	// noTargetPatience is the idle time before a targetless hunter
	// patrols toward the zone center: entering a zone engages the
	// first mob in reach, and only an empty radius keeps the
	// character walking toward the middle.
	noTargetPatience = 6 * time.Second
	// patrolCenterMinDist suppresses the center patrol when the
	// character already stands central: the respawns come to it.
	patrolCenterMinDist = 700.0
	// farTargetRange bounds the far target lookup of a targetless
	// hunter: the granular zone squares reach past the engage radius
	// (a 1300 half corner sits 1800+ units from the center), so the
	// nearest in-zone mob can stand far outside the pick radius. The
	// tracker only knows the mobs the server showed the character,
	// so this stays inside the loaded region block anyway.
	farTargetRange = 6000.0
	// noPickLogPeriod paces the targetless diagnostic of the
	// engage: the nearest rejected mobs with their positions and
	// the rejection reasons print at most once per period while
	// the pick comes up empty, so a standing hunter explains
	// itself in the log without spamming every tick.
	noPickLogPeriod = 5 * time.Second
	// noPickLogLimit bounds the targetless diagnostic: the log
	// lists the nearest rejected mobs, not the whole knownlist.
	noPickLogLimit = 3
	// panicLogoutHealthPercent is the HP level below which a
	// character under attack logs out for a pause: the escape
	// could not shake the chase, staying means dying (the
	// experience loss of a death at level 10+ costs hours of
	// farming).
	panicLogoutHealthPercent = 12.0
	// panicLogoutPause is the login cooldown of the emergency
	// logout: the supervisor waits it out before the next session.
	// The aggro resets the moment the character leaves the world -
	// the chasing mobs drop the target and start their walk home -
	// so a short pause that covers the logout round trip is all
	// the reset needs (observed live: a two second relogin lands
	// clean), and the character regenerates sitting through the
	// sessions that follow. A failed early login (the server still
	// holds the combat stance body) only costs one retry: the
	// supervisor backoff doubles it.
	panicLogoutPause = 2 * time.Second
	// panicLogoutAttackers is the aggro count that starts the
	// pile up run: two swinging mobs outdamage anything a lone
	// farmer can answer, and a social pack only grows while the
	// fight lasts - the fight is dropped and the logout happens
	// at a distance instead (see panicRunDistance).
	panicLogoutAttackers = 2
	// panicRunDistance is the escape distance the pile up run
	// must open from the aggro point before the logout fires:
	// logged out there, the character leaves the chasing pack
	// behind, the mobs walk home while the character is offline,
	// and the short relogin lands far outside their aggro range
	// instead of on top of the same pack.
	panicRunDistance = 600.0
	// fleeLogoutAfter bounds one flee episode: an escape that has
	// not shaken the chase within this budget ends the session
	// instead - the mobs keep the character running forever
	// otherwise, and the relogin after the pause resets their
	// aggro while the character regenerates sitting. The pile up
	// run shares the budget: a run that cannot open the escape
	// distance within it (a cornered or blocked escape) still
	// ends the session rather than running forever.
	fleeLogoutAfter = 20 * time.Second
)

// phase is the coarse activity of the hunt loop.
type phase string

const (
	// phaseEngage walks to and attacks the current or nearest target.
	phaseEngage phase = "engage"
	// phaseLoot picks up the drops around the last kill.
	phaseLoot phase = "loot"
	// phaseTownWalk follows the geodata waypoints to the town shop.
	phaseTownWalk phase = "townWalk"
	// phaseTownSell sells the inventory junk at the shop merchant.
	phaseTownSell phase = "townSell"
	// phaseTownReturn follows the geodata waypoints back to the farm
	// spot.
	phaseTownReturn phase = "townReturn"
	// phaseDelevel dies at the town guards to lose the excess levels.
	phaseDelevel phase = "delevel"
	// phaseUser executes a manual command of the web UI (a map click
	// for a walk, an attack or a pickup) until it completes, then the
	// autonomous hunting resumes.
	phaseUser phase = "user"
	// phaseIdle waits for the next manual command of the web UI: the
	// manual only mode of a session started without -hunt never leaves
	// this phase, the loop exists purely to turn the queued commands
	// into world actions.
	phaseIdle phase = "idle"
)

// Loop is the hunt state machine of one bot session.
type Loop struct {
	game    GameAPI
	tracker *state.Bot
	logger  *log.Logger
	journal *session.Journal
	phase   phase
	// autonomous enables the hunting phases of the loop: the engage,
	// loot, town trip and delevel logic. A manual only session (started
	// without -hunt) keeps it off, the loop then drains the manual web
	// commands and otherwise stays idle.
	autonomous bool
	target     int32
	lastHit    time.Time
	// fightStartAt and fightStartFor hold the start of the CURRENT
	// fight: set when the server confirms the running fight of a
	// new target, keyed by the target id (a stale pair is ignored).
	// engageAt re-anchors on every live fight activity for the
	// stuck timeout, so it cannot measure the fight length.
	fightStartAt  time.Time
	fightStartFor int32
	lootID        int32
	lootAt        time.Time
	lootMoveAt    time.Time
	skipped       map[int32]time.Time
	restActionAt  time.Time
	restActionSit bool
	restartAt     time.Time
	zoneCX        int32
	zoneCY        int32
	zoneHalf      int32
	navigator     Navigator
	waypoints     []pathfind.Vec3
	wpIndex       int
	legDest       pathfind.Vec3
	legStart      pathfind.Vec3
	waterEscape   bool
	moveAt        time.Time
	stuckAt       time.Time
	stuckX        int32
	stuckY        int32
	// stuckFast arms after the first stuck skip of a trip: subsequent
	// stuck detections use the shorter stuckFastTimeout so the walker
	// cycles through the remaining waypoints quickly instead of waiting
	// the full stuckTimeout for each one. resetTownTrip and
	// startWalkLeg clear it.
	stuckFast bool
	rePaths   int
	// extendArmed arms the short click extension of the town walk
	// follower: it flips on the first stuck that finds no clear
	// successor waypoint (the pinned cursor - the plain clicks of
	// the leg proved they do not move the character) and clears at
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
	// frozenAreas is the session memory of the ground the live
	// server refused to walk although the geodata pack modeled it
	// as open: every dry town leg search routes around them (see
	// banFrozenCorridor). The 2026-09-12 trainer hall aisle dump:
	// the plan entered the building through the west aisle column,
	// the server walled it, the deterministic re-plan reproduced
	// the identical route and the character stood frozen through
	// the whole re-path budget - the ban turns that freeze into a
	// detour.
	frozenAreas []pathfind.AvoidArea
	// frozenStage counts the escalation rungs of the frozen town
	// leg (0: none, 1: the banned detour re-plan, 2: the direct
	// server routed walk): see escalateFrozenLeg. It resets on the
	// stop boundaries, not on the re-plans of the same leg.
	frozenStage int
	// directLeg marks the town leg that walks by the server's own
	// routing (see armDirectLeg): the geodata plan proved unable to
	// move the character, so the follower drops it and clicks the
	// stop target directly - bounded by directLegUntil.
	directLeg      bool
	directLegUntil time.Time
	// legRadius is the approach radius the current town leg searches
	// its route within: the wide trip ring (tripApproachRadius) for
	// the merchant stops and the returns, the close ring
	// (npcApproachOffset) for the teacher stops - the user rule of
	// the 2026-09-12 report: the character walks right up to the
	// training npc, and the geodata search is the one that knows the
	// walkable ring cells (the trainer hall interior is walkable only
	// along its rows, the straight line offset ring lands on the
	// roof-only band). The re-paths of the leg inherit it.
	legRadius         float64
	farmX             int32
	farmY             int32
	farmZ             int32
	sellAt            time.Time
	sellPhaseAt       time.Time
	merchantID        int32
	merchantPick      time.Time
	merchantDeckUntil time.Time
	sold              map[int32]bool
	tripStart         time.Time
	tripEndedAt       time.Time
	zoneReturn        bool
	zoneFails         int
	roadFights        int
	delevelTarget     int32
	delevelGuard      int32
	delevelTried      map[string]bool
	delevelFight      time.Time
	delevelEnd        time.Time
	delevelExp        int32
	delevelLevel      int32
	delevelFree       int
	delevelWait       time.Time
	delevelCounted    bool
	engageAt          time.Time
	targetSkip        map[int32]time.Time
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
	// the stall ages).
	stagXP     int32
	stagXPAt   time.Time
	stagPosX   int32
	stagPosY   int32
	stagPosZ   int32
	stagPosSet bool
	stagPosAt  time.Time
	// avoidScratch is the reused threat buffer of the aggro-aware
	// walk steering (see loop_avoid.go): the scan refills it in
	// place, so the per leg danger pass costs no allocation.
	avoidScratch []state.AggroThreat
	// avoidLogAt paces the detour diagnostic of the walk steering:
	// a busy corridor bends every leg, the log names it once per
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
	// finished.
	combatAvoidUntil time.Time
	// zoneLegLogAt paces the walled direct leg diagnostic of the
	// zone return escalation (see guardZoneLegClick): the refusal
	// repeats every second while the character stands in the
	// pocket, the log names the wall once per period.
	zoneLegLogAt time.Time
	// fleeAt paces the escape walk requests: the escape must not
	// wait out the attack request pacing of the engage (the last
	// forced attack fired moments before the threshold crossed).
	fleeAt time.Time
	// fleeSince tracks the start of the running flee episode: the
	// escape legs never stop the chase by themselves when the
	// pursuing pack is fast, and past the fleeLogoutAfter budget
	// the session logs out to reset the aggro instead of running
	// forever. A recovered health or a fresh fight clears it.
	fleeSince time.Time
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
	logoutDone    bool
	userKind      string
	userX         int32
	userY         int32
	userZ         int32
	userPlanX     int32
	userPlanY     int32
	userPlanZ     int32
	userTarget    int32
	userStart     time.Time
	userMoveAt    time.Time
	userWaypoints []pathfind.Vec3
	userWpIndex   int
	userPathTried bool
	// userRedirect marks a manual command that replaced a walk
	// still running on the server: the next walk request fires at
	// once instead of waiting for the old walk to finish.
	userRedirect bool
	// equip drives the auto equipment: it equips inventory gear that
	// beats the paperdoll of the character (see equip.go).
	equip *equipManager
	// The town trip shopping state (see shopping.go): the frozen
	// purchase plan of the running trip, the merchant stops it
	// distributes into, the request pacing of the buys, the in-flight
	// buy batch awaiting its inventory confirmation and the cached
	// plan of the shopping trigger.
	tripPlan          []gear.Purchase
	tripStops         []tripStop
	buysPlanned       bool
	buyAt             time.Time
	buyRequested      []gear.Purchase
	buyConfirmAt      time.Time
	buyRetries        int
	shoppingPlanAt    time.Time
	shoppingPlanCache []gear.Purchase
	// shoppingPlanAdena holds the adena the cached queue was planned
	// against: the widget view shows the planning wallet, not the
	// live one drifting with the loot of the same tick.
	shoppingPlanAdena int64
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
	// The combat casting state (see combat_skills.go): the pacing
	// timestamps of the strike and spell requests and of the self
	// buff casts, plus the locally tracked reuse windows of the
	// cast skills (the server clock of the real windows starts at
	// the cast start, the local one carries a jitter margin).
	castAt        time.Time
	buffAt        time.Time
	skillReuse    map[int32]time.Time
	profilePicked bool
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
	// The multi zone hunting state (see zones.go): the registry of the
	// deployment, the picked and the manually overridden zone. The
	// spot mode (see spot_policy.go) replaces the registry with the
	// spot anchored alternative - the zone fields then mirror the
	// leash square of the active spot.
	zones        []HuntingZone
	spot         *spotHunter
	zoneRegion   string
	zonePickedID string
	zoneOverride int
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
	// The pending server confirmation of the last inventory
	// action (see markInventoryAction and gateInventoryCommand).
	userPendingItem  int32
	userPendingEquip bool
	userPendingCount int32
	userPendingAt    time.Time
	userDeferred     []state.Command
	userLastDist     float64
	userDistAt       time.Time
	engLastDist      float64
	engDistAt        time.Time
}

// NewLoop creates the hunt loop for a connected game client.
// Every Loop field is initialized explicitly (the exhaustruct
// convention), which exceeds the line budget.
func NewLoop(game GameAPI, tracker *state.Bot) *Loop { //nolint:funlen
	return &Loop{
		game:              game,
		tracker:           tracker,
		logger:            log.Default(),
		journal:           nil,
		autonomous:        true,
		phase:             phaseEngage,
		equip:             newEquipManager(gear.MeleeFighter{}),
		target:            0,
		lastHit:           time.Time{},
		fightStartAt:      time.Time{},
		fightStartFor:     0,
		lootID:            0,
		lootAt:            time.Time{},
		lootMoveAt:        time.Time{},
		skipped:           make(map[int32]time.Time),
		restActionAt:      time.Time{},
		restActionSit:     false,
		restartAt:         time.Time{},
		zoneCX:            0,
		zoneCY:            0,
		zoneRegion:        "",
		zoneHalf:          0,
		navigator:         nil,
		waypoints:         nil,
		wpIndex:           0,
		legDest:           pathfind.Vec3{X: 0, Y: 0, Z: 0},
		legStart:          pathfind.Vec3{X: 0, Y: 0, Z: 0},
		waterEscape:       false,
		moveAt:            time.Time{},
		stuckAt:           time.Time{},
		stuckX:            0,
		stuckY:            0,
		stuckFast:         false,
		rePaths:           0,
		extendArmed:       false,
		legRadius:         tripApproachRadius,
		repathX:           0,
		repathY:           0,
		frozenRepaths:     0,
		frozenAreas:       nil,
		frozenStage:       0,
		directLeg:         false,
		directLegUntil:    time.Time{},
		farmX:             0,
		farmY:             0,
		farmZ:             0,
		sellAt:            time.Time{},
		sellPhaseAt:       time.Time{},
		merchantID:        0,
		merchantPick:      time.Time{},
		merchantDeckUntil: time.Time{},
		sold:              make(map[int32]bool),
		tripPlan:          nil,
		tripStops:         nil,
		buysPlanned:       false,
		buyAt:             time.Time{},
		buyRequested:      nil,
		buyConfirmAt:      time.Time{},
		buyRetries:        0,
		shoppingPlanAt:    time.Time{},
		shoppingPlanCache: nil,
		shoppingPlanAdena: 0,
		learnPlanCache:    nil,
		learnPlanAt:       time.Time{},
		learnPlanRevision: 0,
		teacherID:         0,
		teacherPick:       time.Time{},
		teacherDeckUntil:  time.Time{},
		teacherWalkUntil:  time.Time{},
		learnRequested:    nil,
		learnConfirmAt:    time.Time{},
		learnRetries:      0,
		learnAt:           time.Time{},
		learnRevision:     0,
		castAt:            time.Time{},
		buffAt:            time.Time{},
		skillReuse:        make(map[int32]time.Time),
		profilePicked:     false,
		weaponWaitLogAt:   time.Time{},
		stagXP:            0,
		stagXPAt:          time.Time{},
		stagPosX:          0,
		stagPosY:          0,
		stagPosZ:          0,
		stagPosSet:        false,
		stagPosAt:         time.Time{},
		zoneLegLogAt:      time.Time{},
		shoppingViewCache: state.ShoppingPlanView{
			Entries: nil,
			Adena:   0,
			Total:   0,
			Trip:    false,
		},

		replacePlanned:    false,
		replaceDone:       false,
		replaceQueue:      nil,
		replaceSelling:    nil,
		replaceSellSent:   false,
		replaceUnequipAt:  time.Time{},
		replaceWaitAt:     time.Time{},
		replaceTried:      0,
		tripGearStart:     [state.PaperdollSlots]int32{},
		tripGearStartIDs:  [state.PaperdollSlots]int32{},
		gearDebt:          make(map[int]int32),
		tripStart:         time.Time{},
		tripEndedAt:       time.Time{},
		zones:             nil,
		spot:              nil,
		zonePickedID:      "",
		zoneOverride:      -1,
		zoneCheckAt:       time.Time{},
		zoneDeaths:        nil,
		zoneDeathCap:      -1,
		zoneDeathLevel:    0,
		zoneEmptySince:    time.Time{},
		zoneEmptyUntil:    nil,
		zoneMobPriority:   nil,
		zoneReturn:        false,
		zoneFails:         0,
		roadFights:        0,
		delevelTarget:     0,
		delevelGuard:      0,
		delevelTried:      nil,
		delevelFight:      time.Time{},
		delevelEnd:        time.Time{},
		delevelExp:        0,
		delevelLevel:      0,
		delevelFree:       0,
		delevelWait:       time.Time{},
		delevelCounted:    false,
		engageAt:          time.Time{},
		targetSkip:        nil,
		losAt:             time.Time{},
		losWaypoints:      nil,
		losWpIndex:        0,
		losMoveAt:         time.Time{},
		losTried:          0,
		skipScratch:       nil,
		noTargetSince:     time.Time{},
		noPickLogAt:       time.Time{},
		avoidScratch:      nil,
		avoidLogAt:        time.Time{},
		combatAvoidScanAt: time.Time{},
		combatAvoidAt:     time.Time{},
		combatAvoidUntil:  time.Time{},
		fleeAt:            time.Time{},
		fleeSince:         time.Time{},
		panicAt:           time.Time{},
		panicX:            0,
		panicY:            0,
		logoutDone:        false,
		userKind:          "",
		userX:             0,
		userY:             0,
		userZ:             0,
		userPlanX:         0,
		userPlanY:         0,
		userPlanZ:         0,
		userTarget:        0,
		userStart:         time.Time{},
		userMoveAt:        time.Time{},
		userWaypoints:     nil,
		userWpIndex:       0,
		userPathTried:     false,
		userRedirect:      false,
		userPendingItem:   0,
		userPendingEquip:  false,
		userPendingCount:  0,
		userPendingAt:     time.Time{},
		userDeferred:      nil,
		userLastDist:      0,
		userDistAt:        time.Time{},
		engLastDist:       0,
		engDistAt:         time.Time{},
	}
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

// journalKill emits the structured kill record: the fight duration
// and the health the character ended with ride along for the combat
// efficiency aggregates of the session report.
func (l *Loop) journalKill(objectID int32, now time.Time) {
	if l.journal == nil {
		return
	}
	mob := l.tracker.ObjectName(objectID)
	mobLevel, _ := l.tracker.ObjectLevel(objectID)
	fight := 0.0
	if l.fightStartFor == objectID && !l.fightStartAt.IsZero() &&
		now.After(l.fightStartAt) {
		fight = now.Sub(l.fightStartAt).Seconds()
	}
	l.journal.Kill(l.tracker.ID(), mob, mobLevel, fight,
		l.tracker.SelfHealthPercent())
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

// SetHuntingZone configures the hunting square: the bot attacks the
// monsters inside it only and walks back as soon as it leaves. A zero
// half disables the zone.
func (l *Loop) SetHuntingZone(cx int32, cy int32, half int32) {
	l.zoneCX, l.zoneCY, l.zoneHalf = cx, cy, half
	l.tracker.SetHuntingZone(cx, cy, half)
}

// zone returns the hunting zone of the loop, nil when disabled.
func (l *Loop) zone() *state.Zone {
	if l.zoneHalf == 0 {
		return nil
	}

	return &state.Zone{CX: l.zoneCX, CY: l.zoneCY, Half: l.zoneHalf}
}

// inZoneSelf reports whether the character stands inside the hunting
// zone (always true when the zone is disabled).
func (l *Loop) inZoneSelf() bool {
	zone := l.zone()
	if zone == nil {
		return true
	}
	x, y, _, ok := l.tracker.SelfPosition()

	return !ok || zone.Contains(x, y)
}

// DefaultHuntingZone is the configured hunting square of this
// deployment: 3300x3300 units centered just below the Newbie Helper of
// the Elven village. Wide enough to keep the gremlin pack on the east
// edge (up to x ~47460) and the keltir field south of it (up to
// y ~42980) inside the square.
func DefaultHuntingZone() (cx int32, cy int32, half int32) {
	return 46112, 41500, 1650
}

// Run drives the hunt loop until the context is done.
func (l *Loop) Run(ctx context.Context) {
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	// The fleet claim of the hunted spot dies with this goroutine: a
	// session end (the emergency logout of the safety layer, a server
	// restart, a lost connection) unwinds the whole loop together
	// with its spotHunter, and the claim must not outlive it (see
	// spotHunter.releaseClaim).
	defer l.releaseSpotClaim()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			started := time.Now()
			l.tick()
			// The tick duration feeds the statistics view
			// of the web UI: the loop cadence health of a
			// loaded process (NoteHuntTick averages and
			// bounds the window max, see state metrics).
			l.tracker.NoteHuntTick(time.Since(started))
		}
	}
}

// releaseSpotClaim hands the fleet occupancy claim of the hunted spot
// back to the hub when the loop stops. Idempotent: a second call (a
// stop after a spot release) is a no-op.
func (l *Loop) releaseSpotClaim() {
	if l.spot != nil {
		l.spot.releaseClaim()
	}
}

// tick advances the hunt state machine by one decision. The phase
// transitions fall through, so a kill switches into looting and the
// first pickup happens on the same tick.
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
func (l *Loop) tick() { //nolint:cyclop,funlen
	// Publish the hunt loop phase to the bot tracker so the web UI
	// can show the human readable activity banner. SetPhase is a
	// no-op when the phase has not changed, so the per tick call
	// never churns the event stream. Runs on every return path
	// through the defer. The closure captures l.phase by reference
	// so the value at return time is published (a plain defer call
	// evaluates its arguments at registration time). The shopping
	// view of the web UI widget rides the same defer: every tick
	// republishes the queue (SetShoppingPlan no-ops on an identical
	// view) so the widget never shows a stale plan whatever phase
	// the tick ended in. The same defer publishes the loop internals
	// for the diagnostics section of the state dump
	// (SetHuntDiagnostics stamps the heartbeat the encoders age
	// without a version bump). The stagnation watch rides the same
	// defer so it observes every tick whatever path the state
	// machine took (see stagnation.go).
	defer func() {
		now := time.Now()
		l.tracker.SetPhase(string(l.phase))
		l.publishShoppingView()
		l.tracker.SetHuntDiagnostics(l.diagnostics(now))
		l.observeStagnation(now)
	}()
	if l.tracker.SelfDead() {
		l.recoverFromDeath()

		return
	}
	// The emergency logout: critical health with the blows still
	// landing ends the session at once (one hit from death, the
	// run has nothing left to protect), while a social pile up -
	// several mobs already hold the character as their target -
	// starts the pile up run instead: the logout waits until the
	// character opened the escape distance from the aggro point.
	// The deleveling wants the deaths, a manual only session
	// never decides on its own, and a request already sent stays
	// one shot while the session unwinds. The armed run keeps
	// driving the logout even after the pack thins out on the
	// way: the escape distance, not the live mob count, ends it.
	if !l.logoutDone && l.autonomous && l.phase != phaseDelevel {
		if l.tracker.SelfHealthPercent() < panicLogoutHealthPercent &&
			l.tracker.SelfUnderAttack() {
			l.emergencyLogout()

			return
		}
		if l.tracker.SelfAttackerCount() >= panicLogoutAttackers ||
			!l.panicAt.IsZero() {
			l.panicPileUpRun(time.Now())

			return
		}
	}
	if l.logoutDone {
		return
	}
	// The manual commands of the web UI arrive asynchronously on the
	// bot tracker: drain them before the phase dispatch so a command
	// can interrupt the current phase.
	l.consumeUserCommands()
	if l.phase == phaseDelevel {
		l.tickDelevel()
		l.publishWalkPlan()

		return
	}
	if l.phase == phaseUser {
		l.tickUser()
		// The walk plan view follows the manual phase: the plan
		// publishes while a manual move runs, anything else clears
		// it (ClearWalkPlan is a no-op without a plan).
		l.publishWalkPlan()

		return
	}
	if !l.autonomous {
		// A manual only session never hunts on its own: the loop
		// waits in the idle phase for the next web command.
		l.tracker.ClearWalkPlan()
		l.phase = phaseIdle

		return
	}
	// The auto equipment runs in every phase of the hunt: the paperdoll
	// stays current while a town trip buys its gear and while the loot
	// drops arrive, so the combat stats never lag behind the inventory.
	// The class profile pick runs before it: a mystic class scores the
	// caster gear from its very first equip decision.
	l.maybePickGearProfile()
	l.maybeEquipGear()
	// The replaced starter kit follows the equips: the unsellable,
	// undroppable Squire's pieces leave the bag through the destroy
	// request as soon as their replacement is worn.
	l.maybeDestroyReplacedStarters()
	if l.handleTownTrip() {
		return
	}
	// The non walking hunt phases clear the walk plan view: the
	// engage and loot phases have no planned path to draw.
	l.tracker.ClearWalkPlan()
	// The destroy cleanup runs outside the trips: everything the
	// merchant refuses is still better sold at the next shop than
	// destroyed on the way.
	l.cleanupInventory()
	if l.delevelWanted() {
		l.startDelevel()
		l.tickDelevel()
		l.publishWalkPlan()

		return
	}
	l.maybeSwitchZone()
	if l.phase == phaseEngage {
		l.engage()
	}
	if l.phase == phaseLoot {
		l.loot()
	}
}

// handleTownTrip drives the town trip start and the running trip tick.
// It reports true when the tick was consumed by a town trip (already
// running or just started), so the caller skips the engage/loot logic.
// The town walk plan view publishes here too: while the trip walks to
// or from the merchant, the remaining geodata waypoints publish so the
// map draws the planned path. A town trip never abandons a running
// fight (the loot of the kill is the point of the fight), so the start
// waits for the last corpse to be looted and the character to stand
// between the targets (see fightBusy).
func (l *Loop) handleTownTrip() bool {
	if l.tripActive() {
		l.tickTownTrip()
		l.publishWalkPlan()

		return true
	}
	if l.fightBusy() {
		return false
	}
	l.maybeStartTownTrip()
	if !l.tripActive() {
		return false
	}
	l.tickTownTrip()
	l.publishWalkPlan()

	return true
}

// fightBusy reports whether the character is still bound to the fight
// it started: a selected living target, a pending loot pickup or a hit
// landing right now. The town trip start waits it out - walking to a
// vendor mid-combat leaves the mob alive (it heals back up) and its
// drops on the ground.
func (l *Loop) fightBusy() bool {
	if l.phase == phaseLoot {
		return true
	}
	if l.phase != phaseEngage {
		return false
	}
	if l.target != 0 {
		return true
	}

	return l.tracker.SelfUnderAttack()
}

// recoverFromDeath returns a dead character to the hunt: the village
// restart request is the death dialog choice of the official client and
// revives the character with restored vitals. The request retries until
// the revival lands (the server refuses it during the death delay) and
// the stale target and loot references are dropped; the zone leash of
// the engage phase walks the revived character back afterwards. A town
// trip interrupted by the death is dropped without a cooldown so a full
// inventory sells right after the revival (the restart lands in the
// village, next to the shops). A death during the deleveling is the
// point of the exercise: the phase keeps running and replans the walk
// to the guard from the restart point.
func (l *Loop) recoverFromDeath() {
	now := time.Now()
	if !l.restartAt.IsZero() && now.Sub(l.restartAt) < deathRestartPeriod {
		return
	}
	l.tracker.ClearWalkPlan()
	l.restartAt = now
	l.target = 0
	l.clearBlindRecovery()
	l.lootID = 0
	if l.phase == phaseDelevel {
		// Count the death against the experience it removed
		// before the replan: the free death counter may abort
		// the deleveling once the character is alive again.
		l.noteDelevelDeath()
		l.waypoints = nil
		l.legStart = pathfind.Vec3{X: 0, Y: 0, Z: 0}
		l.waterEscape = false
	} else {
		if l.autonomous {
			l.phase = phaseEngage
		} else {
			l.phase = phaseIdle
		}
		l.resetTownTrip()
		l.noTargetSince = time.Time{}
		l.fleeAt = time.Time{}
		l.fleeSince = time.Time{}
		// The village restart lands next to the shops and the cooldown
		// a recent finished trip armed must not hold the sale back:
		// otherwise the revived character walks to the farm spot with
		// the full bag first and returns to sell later.
		l.tripEndedAt = time.Time{}
		// The death counts against the active zone: past the limit
		// the band ladder regresses onto easier grounds (the
		// deleveling branch above never counts, its deaths are the
		// point of the phase).
		l.noteZoneDeath()
	}
	l.logf("Hunt: character died, restarting at the nearest village")
	if l.journal != nil {
		level := l.tracker.SelfLevel()
		x, y, _, _ := l.tracker.SelfPosition()
		l.journal.Death(l.tracker.ID(), level, x, y)
	}
	if err := l.game.RestartAtVillage(); err != nil {
		l.logf("Hunt: village restart failed: %v", err)
	}
}

// engage attacks the nearest attackable npc while the current target
// lives, then switches to the loot phase. The Mobius AttackRequest has
// double click semantics: the first request selects the target (the
// MyTargetSelected answer), the repeated request for the already
// selected target triggers the forced attack. The loop therefore keeps
// re-requesting the target until the character is actually engaged in
// the fight, which the MoveToPawn/Attack/AutoAttackStart broadcasts
// confirm. The safety gates run before every attack decision: a
// Pre-consolidation phase debt; the hunt loop cleanup is planned
// (docs/quality_review_and_agent_prompts.md P07).
// losing fight is fled instead of fought to the death, a hurt
// character under attack keeps running instead of standing in the
// blows, and the target search never initiates on mobs above the
// character level slack or on social pulls whose clan mates stand
// within the help range.
// The engage decision tree: the target search, the attack start,
// the loot, the rest and the safety gates form one state machine.
// Refactor candidate: split into the phase handlers.
//
//nolint:cyclop,funlen,gocognit,maintidx
func (l *Loop) engage() {
	// The hunting zone leash: new fights start inside the square only,
	// and a character outside of it (a village respawn, the walk home
	// after a finished fight) walks back instead of hunting. Two
	// exceptions: a hurt character under attack flees even outside the
	// square (the leash walk home would drag it through the chasing
	// pack), and a fight that crossed the square line is finished
	// where it stands - the mob that dragged the character out (a
	// chase, an aggressive pull) dies before the walk home, dropping
	// the fight here would walk home through the blows and leave the
	// loot on the ground (see adoptOutZoneFight).
	now := time.Now()
	if !l.inZoneSelf() {
		if l.tracker.SelfUnderAttack() &&
			l.tracker.SelfHealthPercent() < reengageHealthPercent {
			l.fleeFromThreat(now)

			return
		}
		if !l.adoptOutZoneFight(now) {
			if l.weaponlessRunWanted() {
				// The bare-handed errand outranks the walk
				// home: the return walk through the
				// aggressive packs never arrives unarmed -
				// the panic logout saves the character on
				// the ground it flees, the relogin handoff
				// resumes that ground and the packs pile on
				// again (the 2026-09-11 farm round: the
				// reset bot walked from the village to the
				// Spore Fungus SW ground bare handed and
				// livelocked in the logout cycle). The trip
				// machinery starts the weapon run on the
				// next tick and its return leg walks home
				// armed; the wallet that cannot afford any
				// weapon keeps the ordinary return (the
				// punches are all it has).
				if !l.zoneReturn {
					l.zoneReturn = true
					l.logger.Printf("Hunt: no weapon in " +
						"hand, the weapon run outranks " +
						"the walk home")
				}

				return
			}
			l.returnToZone()

			return
		}
	}
	if l.zoneReturn {
		l.zoneReturn = false
		l.zoneFails = 0
		l.roadFights = 0
		l.logf("Hunt: back in the hunting zone, resuming the hunt")
	}
	// The mana rest of the caster runs before the server target
	// re-adopt: a caster whose mana sits under the re-engage
	// threshold owns its fights until the bar recovers (the dry
	// one drops the running fight, the standing one holds the new
	// picks), and the re-adopt below would otherwise hand the
	// dropped target straight back. Under attack the hold lifts:
	// the caster answers the blows instead of sitting into them,
	// and the safety nets further down stay armed whatever the
	// mana says (see combat_skills.go).
	manaHeld := l.mageManaLow() && !l.tracker.SelfUnderAttack()
	if manaHeld && l.target != 0 {
		l.dropFightForMana()
	}
	// Prefer the server view of the target while it lives: the
	// MyTargetSelected answer of the last attack request arrives
	// asynchronously, so the fresh value is read every tick. A stale
	// id of a dead or removed target must not be re-adopted: the
	// server never clears the selection of a corpse (only the next
	// selection replaces it), so blindly trusting it locked the
	// loop into an engage/loot ping-pong where the next target was
	// never selected. A target marked stuck (its repeated attack
	// requests never started the fight) is not re-adopted either
	// while its skip delay lasts. The mana held caster re-adopts
	// nothing - the rest owns the selection until the mana stands
	// back up. The adoption also requires an attackable npc: the
	// server side selection a town trip leaves behind (the talked
	// teacher or merchant) points at a friendly villager, and the
	// forced attack requests on it only burn the engage stuck
	// timeout before the skip - the trip end clears the selection
	// (see GameClient.ClearTarget), this gate keeps the engage
	// clean even when a selection lingers (a talk without the trip
	// end, a self selection of the clear click itself).
	serverTarget := l.tracker.SelfTargetID()
	if serverTarget != 0 && !manaHeld &&
		l.tracker.ObjectAlive(serverTarget) &&
		l.tracker.ObjectAttackable(serverTarget) &&
		!l.targetSkipped(serverTarget, now) {
		l.target = serverTarget
	}
	if l.target != 0 && !l.tracker.ObjectAlive(l.target) {
		if l.spot != nil {
			l.spotNoteKill(l.target, now)
		}
		l.journalKill(l.target, now)
		l.logf("Hunt: target %d died, looting", l.target)
		l.target = 0
		l.clearBlindRecovery()
		l.phase = phaseLoot
		l.lootID = 0

		return
	}
	if l.target != 0 && !l.tracker.SelfFighting(l.target) &&
		!l.engageAt.IsZero() && now.Sub(l.engageAt) > engageStuckTimeout &&
		!l.blindRecoveryArmed(now) {
		// The repeated attack requests never started a real
		// fight: the selection on the server points at an
		// object that refuses the forced attack (typically the
		// corpse of an abruptly disconnected session, which
		// the server keeps selected), or the armed attack
		// stance went stale behind an obstacle. The gate is
		// the FRESH fight view: the looser SelfEngaged stays
		// true forever behind an obstacle, because the server
		// arms the attack stance without a line of sight
		// check (Creature.onForcedAttack) and no swing ever
		// lands to disarm it - the observed 53 minute stall
		// held the loop exactly there. Only the selection of a
		// DIFFERENT object id replaces the stale one, so the
		// target is dropped and skipped for a while; the
		// blind recovery owns the obstructed case with its
		// own budgets instead.
		l.logf("Hunt: target %d does not engage, "+
			"switching to another", l.target)
		if l.targetSkip == nil {
			l.targetSkip = make(map[int32]time.Time)
		}
		l.targetSkip[l.target] = now.Add(engageSkipDelay)
		l.target = 0
		l.engageAt = time.Time{}
		l.clearBlindRecovery()

		return
	}
	if l.target != 0 && l.losingFight() {
		// The fight turned into a death risk: the health fell under
		// the escape threshold or the target keeps a clear health
		// lead over a hurt character (the level gap pull of the
		// observed death, an add that joined a social pack).
		// Pressing on below that line dies far more often than it
		// kills, so the fight is dropped and the character runs.
		// It stays ahead of the blind recovery: an add grinding a
		// repositioning character must escalate into the flee, not
		// into another detour leg.
		l.fleeFromTarget(l.target, now)

		return
	}
	if l.target != 0 && l.blindRecoveryArmed(now) {
		// The server refuses the swings with "Cannot see
		// target." (a fresh refusal was seen) or the planned
		// reposition walk is still running: an obstacle stands
		// between the character and the selected target. Walk
		// around it over the geodata first, switch the target only
		// when no route clears the block (see loop_los.go).
		l.recoverBlindEngage(now)

		return
	}
	if l.target == 0 {
		// Rest while the character is hurt: the regeneration is
		// faster out of combat and engaging with low HP risks
		// death. A sitting character stands up through the rest
		// logic once recovered. A hurt character under attack ran
		// out of targets it can win: it keeps fleeing instead of
		// standing in the blows or sitting into them.
		hurt := l.tracker.SelfHealthPercent() < reengageHealthPercent
		if !hurt && !l.tracker.SelfUnderAttack() {
			// The health recovered and the blows stopped: the flee
			// episode is over, a future escape gets a fresh budget.
			l.fleeSince = time.Time{}
		}
		if hurt && l.tracker.SelfUnderAttack() {
			l.fleeFromThreat(now)

			return
		}
		if l.tracker.SelfSitting() && l.tracker.SelfUnderAttack() {
			// A mob reached a resting character above the hurt gate:
			// stand up - the pick below selects the attacker and
			// the fight answers itself (the hurt branch above runs
			// at the low health instead).
			l.standUpGuarded(now)

			return
		}
		if l.tracker.SelfSitting() || hurt {
			l.rest()

			return
		}
		if manaHeld {
			// The mana rest of the caster: the dry one sits
			// down (the rest toggles it), the one above the
			// sit threshold stands and regenerates. The self
			// buffs wait for the recovered bar - a cast now
			// would spend the mana the rest is rebuilding.
			if l.tracker.SelfManaPercent() < manaSitPercent {
				l.rest()
			}

			return
		}
		if l.maybeSelfBuff(now) {
			// The missing self buff just fired: the tick is
			// spent on it, the next pick happens on the next
			// tick (the select pacing gates it anyway).
			return
		}
		if now.Sub(l.lastHit) < selectPeriod {
			return
		}
		// The aggro answer of the targetless pick: a mob
		// already holds the character as its target (its swings
		// or its chase - both carry the character as the target
		// id), so walking to a fresh mob only drags the chase
		// into a second opponent and the fights pile up (the
		// reported deaths). A healthy character with a winnable
		// attacker engages it at once - the fight answers the
		// aggro where it stands; anything else (hurt, the
		// attacker above the level ceiling) stays with the
		// defensive flow: the standard escape walk that logs
		// out when it never shakes the chase.
		if attacker, ok := l.tracker.NearestAttacker(); ok {
			if !l.attackerEngageable(attacker.ObjectID) {
				l.fleeFromThreat(now)

				return
			}
			l.target = attacker.ObjectID
			l.engageAt = now
			l.clearBlindRecovery()
			l.logger.Printf("Hunt: %s is on us, fighting it "+
				"instead of picking a new target",
				attacker.Name)
			// Falls through: the forced attack request below
			// fires on this very tick, the chase stops where
			// it stands.
		} else {
			if l.weaponlessRunWanted() {
				// Bare-handed with an affordable weapon
				// in the plan: no fresh targets - the
				// fists land 2 damage while the plan
				// offers a sword, and the weapon run
				// (the town trip trigger of this tick)
				// owns the next moves. The attacker
				// answer above stays armed: a mob
				// already on the character is fought
				// whatever the weapon state is.
				l.logWeaponWait(now)

				return
			}
			pick, ok := l.tracker.NearestAttackablePreferredWindowed(
				attackNearestRange, l.zone(), l.activeSkips(now),
				l.minTargetLevel(), l.maxTargetLevel(), true,
				l.zoneMobPriority)
			if !ok {
				if l.walkToFarTarget(now) {
					return
				}
				l.patrolToCenter(now)

				return
			}
			l.noTargetSince = time.Time{}
			l.noPickLogAt = time.Time{}
			l.target = pick.ObjectID
			l.engageAt = now
			l.clearBlindRecovery()
		}
	}
	if l.tracker.SelfFighting(l.target) {
		// The confirmed running fight stamps its start once: the
		// kill journal record measures the real fight length
		// against it (engageAt re-anchors and cannot).
		if l.fightStartAt.IsZero() || l.fightStartFor != l.target {
			l.fightStartAt = now
			l.fightStartFor = l.target
		}
		// A running fight ends the flee episode: the character
		// answered instead of running, the escape budget resets.
		l.fleeSince = time.Time{}
		// The fresh fight also re-anchors the engage clock: the
		// stuck timeout measures from the last real fight
		// activity, not from the original pick - a slow but
		// living fight must never trip it. The re-anchor requires
		// the fight to have progressed past the last "Cannot see
		// target." refusal: the server keeps broadcasting the chase
		// steps of an attack it refuses to execute (the phantom
		// chase), and a clock re-anchored past every refusal holds
		// both the stuck timeout and the blind recovery away from
		// the obstructed engage forever (the 2026-09-12 03:25 dump:
		// 80 units from the target, over a minute of refusals, no
		// walk, no switch).
		if l.fightClearedRefusal() {
			l.engageAt = now
		}
		// The impending add: an aggressive neighbor about to
		// join the fight gets a step of clearance BEFORE its
		// on-sight trigger fires (see loop_avoid.go) - backing
		// off a potential second opponent beats the pile up
		// run and the emergency relogin of the real one.
		if l.avoidImpendingAdd(now) {
			return
		}
		// The combat casting of the running fight: the warrior
		// strike of the weapon in hand, the mystic attack spell
		// (see combat_skills.go). The casts share the human pacing
		// of the loop, the auto attack keeps swinging whatever the
		// cast request does.
		l.maybeCastCombatSkill(now)
		// The swings land right now: nothing to re-request. A stale
		// engagement (the fight was interrupted, the auto attack flag
		// and the combat window linger) falls through and keeps
		// re-requesting the forced attack instead of standing still
		// until the stuck timeout switches the target. A running chase
		// also needs the progress watchdog: the Mobius path search
		// stalls on some routes while the stuck chase packets keep the
		// engagement fresh - then the loop walks the stretch itself.
		if x, y, z, ok := l.tracker.ObjectPosition(l.target); ok {
			if selfX, selfY, _, selfOK := l.tracker.SelfPosition(); selfOK {
				dist := math.Hypot(
					float64(x-selfX), float64(y-selfY))
				if dist > userEngageRadius &&
					!l.chaseProgress(
						&l.engLastDist, &l.engDistAt, dist, now) &&
					!l.tracker.SelfWalking() &&
					now.Sub(l.lastHit) >= engageRetryPeriod {
					l.lastHit = now
					l.logf("Hunt: chase on %d stalled at "+
						"%d units, walking to the target",
						l.target, int(dist))
					if err := l.game.WalkTo(x, y, z); err != nil {
						l.logf("Hunt: chase walk failed: %v",
							err)
					}
				}
			}
		}

		return
	}
	if now.Before(l.combatAvoidUntil) {
		// The impending-add step owns the movement for its
		// short window: a forced attack request interrupts a
		// running walk server-side, and the add would meet the
		// character right back where it stood.
		return
	}
	if now.Sub(l.lastHit) < engageRetryPeriod {
		return
	}
	if err := l.game.AttackTarget(l.target); err != nil {
		l.logf("Hunt: attack failed: %v", err)

		return
	}
	l.lastHit = now
}

// losingFight reports whether the running fight turned into a death
// risk: the character fell under the escape threshold, or it dropped
// below the re-engage health while the target keeps a clear health
// lead (a level gap pull, an add that joined a social pack). A
// target that is nearly finished does not count as losing - the
// kill is one swing away and fleeing drops its loot.
func (l *Loop) losingFight() bool {
	selfHP := l.tracker.SelfHealthPercent()
	if selfHP >= reengageHealthPercent {
		return false
	}
	targetHP := l.tracker.ObjectHealthPercent(l.target)
	if targetHP >= 0 && targetHP < escapeTargetBeatenPercent {
		return false
	}
	if selfHP < escapeHealthPercent {
		return true
	}

	return targetHP >= 0 && targetHP-selfHP > losingFightGap
}

// maxTargetLevel bounds the mob level the engage initiates on: the
// character level plus the slack. A fresh spawn whose UserInfo has
// not arrived yet (level 0) disables the filter instead of fencing
// every pick out.
func (l *Loop) maxTargetLevel() int32 {
	level := l.tracker.SelfLevel()
	if level <= 0 {
		return 0
	}

	return level + targetMaxLevelSlack
}
