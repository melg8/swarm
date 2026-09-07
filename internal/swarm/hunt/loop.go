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
	phase   phase
	// autonomous enables the hunting phases of the loop: the engage,
	// loot, town trip and delevel logic. A manual only session (started
	// without -hunt) keeps it off, the loop then drains the manual web
	// commands and otherwise stays idle.
	autonomous        bool
	target            int32
	lastHit           time.Time
	lootID            int32
	lootAt            time.Time
	lootMoveAt        time.Time
	skipped           map[int32]time.Time
	restActionAt      time.Time
	restActionSit     bool
	restartAt         time.Time
	zoneCX            int32
	zoneCY            int32
	zoneHalf          int32
	navigator         Navigator
	waypoints         []pathfind.Vec3
	wpIndex           int
	legDest           pathfind.Vec3
	moveAt            time.Time
	stuckAt           time.Time
	stuckX            int32
	stuckY            int32
	rePaths           int
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
	userKind          string
	userX             int32
	userY             int32
	userZ             int32
	userTarget        int32
	userStart         time.Time
	userMoveAt        time.Time
	userWaypoints     []pathfind.Vec3
	userWpIndex       int
	userPathTried     bool
	// userRedirect marks a manual command that replaced a walk
	// still running on the server: the next walk request fires at
	// once instead of waiting for the old walk to finish.
	userRedirect bool
	// equip drives the auto equipment: it equips inventory gear that
	// beats the paperdoll of the character (see equip.go).
	equip *equipManager
	// The town trip shopping state (see shopping.go): the merchant
	// stops of the running trip, the request pacing of the buys, the
	// in-flight buy batch awaiting its inventory confirmation and
	// the cached plan of the shopping trigger.
	tripStops         []tripStop
	buysPlanned       bool
	buyAt             time.Time
	buyRequested      []gear.Purchase
	buyConfirmAt      time.Time
	buyRetries        int
	shoppingPlanAt    time.Time
	shoppingPlanCache []gear.Purchase
	// The multi zone hunting state (see zones.go): the registry of the
	// deployment, the picked and the manually overridden zone.
	zones        []HuntingZone
	zonePickedID string
	zoneOverride int
	zoneCheckAt  time.Time
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
func NewLoop(game GameAPI, tracker *state.Bot) *Loop {
	return &Loop{
		game:              game,
		tracker:           tracker,
		logger:            log.Default(),
		autonomous:        true,
		phase:             phaseEngage,
		equip:             newEquipManager(gear.MeleeFighter{}),
		target:            0,
		lastHit:           time.Time{},
		lootID:            0,
		lootAt:            time.Time{},
		lootMoveAt:        time.Time{},
		skipped:           make(map[int32]time.Time),
		restActionAt:      time.Time{},
		restActionSit:     false,
		restartAt:         time.Time{},
		zoneCX:            0,
		zoneCY:            0,
		zoneHalf:          0,
		navigator:         nil,
		waypoints:         nil,
		wpIndex:           0,
		legDest:           pathfind.Vec3{},
		moveAt:            time.Time{},
		stuckAt:           time.Time{},
		stuckX:            0,
		stuckY:            0,
		rePaths:           0,
		farmX:             0,
		farmY:             0,
		farmZ:             0,
		sellAt:            time.Time{},
		sellPhaseAt:       time.Time{},
		merchantID:        0,
		merchantPick:      time.Time{},
		merchantDeckUntil: time.Time{},
		sold:              make(map[int32]bool),
		tripStops:         nil,
		buysPlanned:       false,
		buyAt:             time.Time{},
		buyRequested:      nil,
		buyConfirmAt:      time.Time{},
		buyRetries:        0,
		shoppingPlanAt:    time.Time{},
		shoppingPlanCache: nil,
		tripStart:         time.Time{},
		tripEndedAt:       time.Time{},
		zones:             nil,
		zonePickedID:      "",
		zoneOverride:      -1,
		zoneCheckAt:       time.Time{},
		zoneReturn:        false,
		zoneFails:         0,
		delevelTarget:     0,
		delevelGuard:      0,
		delevelTried:      nil,
		delevelFight:      time.Time{},
		delevelEnd:        time.Time{},
		userKind:          "",
		userX:             0,
		userY:             0,
		userZ:             0,
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.tick()
		}
	}
}

// tick advances the hunt state machine by one decision. The phase
// transitions fall through, so a kill switches into looting and the
// first pickup happens on the same tick.
func (l *Loop) tick() {
	if l.tracker.SelfDead() {
		l.recoverFromDeath()

		return
	}
	// The manual commands of the web UI arrive asynchronously on the
	// bot tracker: drain them before the phase dispatch so a command
	// can interrupt the current phase.
	l.consumeUserCommands()
	if l.phase == phaseDelevel {
		l.tickDelevel()
		l.tracker.ClearWalkPlan()

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
	l.tracker.ClearWalkPlan()
	if !l.autonomous {
		// A manual only session never hunts on its own: the loop
		// waits in the idle phase for the next web command.
		l.phase = phaseIdle

		return
	}
	// The auto equipment runs in every phase of the hunt: the paperdoll
	// stays current while a town trip buys its gear and while the loot
	// drops arrive, so the combat stats never lag behind the inventory.
	l.maybeEquipGear()
	if l.tripActive() {
		l.tickTownTrip()

		return
	}
	// A town trip never abandons a running fight: the loot of the kill
	// is the point of the fight, so the trip start waits for the last
	// corpse to be looted and the character to stand between the
	// targets (see fightBusy).
	if !l.fightBusy() {
		l.maybeStartTownTrip()
		if l.tripActive() {
			l.tickTownTrip()

			return
		}
	}
	// The destroy cleanup runs outside the trips: everything the
	// merchant refuses is still better sold at the next shop than
	// destroyed on the way.
	l.cleanupInventory()
	if l.delevelWanted() {
		l.startDelevel()
		l.tickDelevel()

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
	l.lootID = 0
	if l.phase == phaseDelevel {
		// Count the death against the experience it removed
		// before the replan: the free death counter may abort
		// the deleveling once the character is alive again.
		l.noteDelevelDeath()
		l.waypoints = nil
	} else {
		if l.autonomous {
			l.phase = phaseEngage
		} else {
			l.phase = phaseIdle
		}
		l.resetTownTrip()
		// The village restart lands next to the shops and the cooldown
		// a recent finished trip armed must not hold the sale back:
		// otherwise the revived character walks to the farm spot with
		// the full bag first and returns to sell later.
		l.tripEndedAt = time.Time{}
	}
	l.logger.Printf("Hunt: character died, restarting at the nearest village")
	if err := l.game.RestartAtVillage(); err != nil {
		l.logger.Printf("Hunt: village restart failed: %v", err)
	}
}

// engage attacks the nearest attackable npc while the current target
// lives, then switches to the loot phase. The Mobius AttackRequest has
// double click semantics: the first request selects the target (the
// MyTargetSelected answer), the repeated request for the already
// selected target triggers the forced attack. The loop therefore keeps
// re-requesting the target until the character is actually engaged in
// the fight, which the MoveToPawn/Attack/AutoAttackStart broadcasts
// confirm.
func (l *Loop) engage() {
	// The hunting zone leash: attacks happen inside the square only,
	// and a character outside of it (a long chase, a village respawn)
	// walks back instead of hunting.
	if !l.inZoneSelf() {
		l.returnToZone()

		return
	}
	if l.zoneReturn {
		l.zoneReturn = false
		l.zoneFails = 0
		l.logger.Printf("Hunt: back in the hunting zone, resuming the hunt")
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
	// while its skip delay lasts.
	now := time.Now()
	serverTarget := l.tracker.SelfTargetID()
	if serverTarget != 0 && l.tracker.ObjectAlive(serverTarget) &&
		!l.targetSkipped(serverTarget, now) {
		l.target = serverTarget
	}
	if l.target != 0 && !l.tracker.ObjectAlive(l.target) {
		l.logger.Printf("Hunt: target %d died, looting", l.target)
		l.target = 0
		l.phase = phaseLoot
		l.lootID = 0

		return
	}
	if l.target != 0 && !l.tracker.SelfEngaged(l.target) &&
		!l.engageAt.IsZero() && now.Sub(l.engageAt) > engageStuckTimeout {
		// The repeated attack requests never started the fight:
		// the selection on the server points at an object that
		// refuses the forced attack (typically the corpse of an
		// abruptly disconnected session, which the server keeps
		// selected). Only the selection of a DIFFERENT object id
		// replaces the stale one, so the target is dropped and
		// skipped for a while.
		l.logger.Printf("Hunt: target %d does not engage, "+
			"switching to another", l.target)
		if l.targetSkip == nil {
			l.targetSkip = make(map[int32]time.Time)
		}
		l.targetSkip[l.target] = now
		l.target = 0
		l.engageAt = time.Time{}

		return
	}
	if l.target == 0 {
		// Rest while the character is hurt: the regeneration is
		// faster out of combat and engaging with low HP risks
		// death. A sitting character stands up through the rest
		// logic once recovered. A character that is being hit right
		// now keeps fighting instead of sitting into the blows.
		hurt := l.tracker.SelfHealthPercent() < reengageHealthPercent
		if l.tracker.SelfSitting() || (hurt && !l.tracker.SelfUnderAttack()) {
			l.rest()

			return
		}
		if now.Sub(l.lastHit) < selectPeriod {
			return
		}
		pick, ok := l.tracker.NearestAttackableExcept(
			attackNearestRange, l.zone(), l.skippedTargets(now))
		if !ok {
			return
		}
		l.target = pick.ObjectID
		l.engageAt = now
	}
	if l.tracker.SelfFighting(l.target) {
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
					l.logger.Printf("Hunt: chase on %d stalled at "+
						"%d units, walking to the target",
						l.target, int(dist))
					if err := l.game.WalkTo(x, y, z); err != nil {
						l.logger.Printf("Hunt: chase walk failed: %v",
							err)
					}
				}
			}
		}

		return
	}
	if now.Sub(l.lastHit) < engageRetryPeriod {
		return
	}
	if err := l.game.AttackTarget(l.target); err != nil {
		l.logger.Printf("Hunt: attack failed: %v", err)

		return
	}
	l.lastHit = now
}

// targetSkipped reports whether the object id is currently held out of
// the engage target search after it refused to start a fight.
func (l *Loop) targetSkipped(objectID int32, now time.Time) bool {
	skippedAt, ok := l.targetSkip[objectID]

	return ok && now.Sub(skippedAt) < engageSkipDelay
}

// skippedTargets collects the object ids currently held out of the
// target search after they refused to start a fight.
func (l *Loop) skippedTargets(now time.Time) map[int32]bool {
	if len(l.targetSkip) == 0 {
		return nil
	}
	ids := make(map[int32]bool, len(l.targetSkip))
	for objectID, skippedAt := range l.targetSkip {
		if now.Sub(skippedAt) < engageSkipDelay {
			ids[objectID] = true
		}
	}

	return ids
}

// returnToZone walks the character back into the hunting square over the
// geodata waypoints: a village respawn after death or a deleveling guard
// post sits behind the village walls, and a direct walk bumps into them,
// so the return is planned with the pathfinder and followed by the town
// trip waypoint machinery (leg splitting, passed waypoint skipping, stuck
// re-pathing) through phaseTownReturn. The remembered farm spot is the
// destination when one exists, the zone center otherwise. The failures of
// the pathfound legs (a missing geodata region, an unreachable deck) fall
// back to the legacy direct legs, so a character without a walkable path
// still moves home.
func (l *Loop) returnToZone() {
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	zone := l.zone()
	if zone == nil {
		return
	}
	selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	l.target = 0
	l.lootID = 0
	if l.zoneReturn && l.phase == phaseEngage {
		// The previous pathfound return leg ended without reaching
		// the zone (a stuck walk aborts the leg): count the failure
		// and stop planning past the budget.
		l.zoneFails++
	}
	if !l.zoneReturn {
		l.zoneReturn = true
		l.logger.Printf("Hunt: outside the hunting zone, pathfinding back")
	}
	if (l.navigator == nil) || l.zoneFails >= zoneReturnFailBudget {
		l.phase = phaseEngage
		l.walkZoneLeg(zone, selfX, selfY, selfZ)

		return
	}
	dest := pathfind.Vec3{
		X: float64(zone.CX),
		Y: float64(zone.CY),
		Z: float64(selfZ),
	}
	if (l.farmX != 0) || (l.farmY != 0) {
		dest = pathfind.Vec3{
			X: float64(l.farmX),
			Y: float64(l.farmY),
			Z: float64(l.farmZ),
		}
	}
	l.tripStart = time.Now()
	l.rePaths = 0
	l.phase = phaseTownReturn
	if !l.startWalkLeg(dest) {
		// No geodata path: direct legs toward the zone, the server
		// stops them at obstacles and the next second plans again.
		l.phase = phaseEngage
		l.walkZoneLeg(zone, selfX, selfY, selfZ)

		return
	}
}

// walkZoneLeg walks one direct short leg toward the zone center: the
// emergency fallback of the pathfinding zone return. The leg length
// respects the server move request limit (9900 units) and the walk rate
// limits itself through the select pacing of the return.
func (l *Loop) walkZoneLeg(zone *state.Zone, selfX int32, selfY int32, selfZ int32) {
	moveX, moveY := zone.CX, zone.CY
	dx := float64(zone.CX - selfX)
	dy := float64(zone.CY - selfY)
	if dist := math.Hypot(dx, dy); dist > returnWalkLeg {
		frac := returnWalkLeg / dist
		moveX = int32(float64(selfX) + dx*frac)
		moveY = int32(float64(selfY) + dy*frac)
	}
	if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
		l.logger.Printf("Hunt: walk back failed: %v", err)
	}
}

// rest brings the resting character back to full health. Sitting
// accelerates the regeneration, so the character sits down below the
// sit threshold and stands up again once recovered. The sit/stand
// action is a server side toggle, so every transition is confirmed by
// the ChangeWaitType broadcast before the opposite one is ever sent.
func (l *Loop) rest() {
	now := time.Now()
	if now.Sub(l.lastHit) < selectPeriod {
		return
	}
	l.lastHit = now
	hp := l.tracker.SelfHealthPercent()
	wantSit := false
	switch {
	case l.tracker.SelfSitting() && hp < standUpHealthPercent:
		// The sit is confirmed and the regeneration is running.
		return
	case l.tracker.SelfSitting():
		wantSit = false
	case hp < sitDownHealthPercent:
		wantSit = true
	default:
		l.logger.Printf("Hunt: resting, HP %.0f%% below %.0f%%",
			hp, reengageHealthPercent)

		return
	}
	if l.tracker.SelfSitting() == wantSit {
		return
	}
	if !l.restActionAt.IsZero() {
		if l.restActionSit == l.tracker.SelfSitting() {
			// The previous transition is confirmed, consume it.
			l.restActionAt = time.Time{}
		} else if now.Sub(l.restActionAt) < restRetryPeriod {
			// Confirmation still pending, never double toggle.
			return
		}
	}
	if wantSit {
		l.logger.Printf("Hunt: HP %.0f%% below %.0f%%, sitting down to regenerate",
			hp, sitDownHealthPercent)
	} else {
		l.logger.Printf("Hunt: HP %.0f%% recovered, standing up", hp)
	}
	if err := l.game.ActionSitStand(); err != nil {
		l.logger.Printf("Hunt: sit/stand action failed: %v", err)

		return
	}
	l.restActionAt = now
	l.restActionSit = wantSit
}

// loot picks up the ground items around the character until none is left
// within the loot radius, then hunts the next target. Farther items are
// approached with an explicit walk first so the character visibly runs
// toward the loot instead of trusting the click to start the whole
// approach.
func (l *Loop) loot() {
	item, ok := l.tracker.NearestGroundItemExcluding(lootRadius, l.skipped,
		l.zone())
	if !ok {
		l.phase = phaseEngage
		l.target = 0

		return
	}
	now := time.Now()
	if item.ObjectID != l.lootID {
		l.lootID = item.ObjectID
		l.lootAt = now
		l.lootMoveAt = time.Time{}
	}
	if now.Sub(l.lootAt) > pickupTimeout {
		// The pickup did not finish: the item is protected or
		// unreachable. Skip it for a while and try the next one.
		l.skipped[item.ObjectID] = now.Add(pickupRetryDelay)
		l.lootID = 0
		l.logger.Printf("Hunt: pickup of %d timed out, skipping", item.ObjectID)

		return
	}
	if now.Sub(l.lootMoveAt) < selectPeriod {
		return
	}
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return
	}
	dist := math.Hypot(float64(item.X-selfX), float64(item.Y-selfY))
	l.lootMoveAt = now
	if dist > lootApproachRadius {
		if err := l.game.WalkTo(item.X, item.Y, item.Z); err != nil {
			l.logger.Printf("Hunt: walk to loot failed: %v", err)
		}

		return
	}
	if err := l.game.PickupItem(item); err != nil {
		l.logger.Printf("Hunt: pickup failed: %v", err)
	}
}

// cleanupInventory destroys junk items when the slots or the weight of
// the character approach the server limits.
func (l *Loop) cleanupInventory() {
	stats := l.tracker.InventoryStats()
	if stats.SlotPercent < cleanupSlotPercent &&
		stats.WeightPercent < cleanupWeightPercent {
		return
	}
	junk := l.tracker.DestroyableItems(destroyBatch)
	if len(junk) == 0 {
		return
	}
	l.logger.Printf("Hunt: inventory at %d slots and %.0f%% weight, "+
		"destroying %d items", stats.Slots, stats.WeightPercent, len(junk))
	for _, item := range junk {
		err := l.game.DestroyItem(item.ObjectID, item.Count)
		if err != nil {
			l.logger.Printf("Hunt: destroy failed: %v", err)

			return
		}
	}
}
