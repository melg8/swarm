// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// AttackTarget describes a target the bot can attack.
type AttackTarget struct {
	ObjectID int32
	Name     string
	X        int32
	Y        int32
	Z        int32
}

// Zone is a square world area: the hunting policy of the bot (attack
// and loot inside it only, never wander out). Nil zones mean no limit.
type Zone struct {
	CX   int32 `json:"cx"`
	CY   int32 `json:"cy"`
	Half int32 `json:"half"`
}

// Contains reports whether the world point lies inside the zone.
func (z *Zone) Contains(x int32, y int32) bool {
	if z == nil {
		return true
	}

	return x >= z.CX-z.Half && x <= z.CX+z.Half &&
		y >= z.CY-z.Half && y <= z.CY+z.Half
}

// NearestAttacker returns the closest living attackable npc that
// currently targets the character: the mob whose blows land, the
// chase of the flee flow. The projected position of every attacker
// candidate measures the moving chase.
func (b *Bot) NearestAttacker() (AttackTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestDist := math.MaxFloat64
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	nowNano := time.Now().UnixNano()
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || !obj.Attackable || obj.Dead ||
			obj.TargetID != b.selfID {
			continue
		}
		x, y := projectedPosition(obj, nowNano)
		dist := math.Hypot(x-selfX, y-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     b.world.cold[i].Name,
				X:        int32(math.Round(x)),
				Y:        int32(math.Round(y)),
				Z:        obj.Z,
			}
		}
	}

	return best, found
}

// NearestAttackable returns the closest living attackable npc within the
// given distance of the character. The distance uses the projected
// current position of every npc (see projectedPosition), not the raw
// packet position: the server broadcasts movement at most once per
// second, so a moving mob is typically tens or hundreds of units away
// from its last packet start position and a stale "nearest" choice
// would send the character to a mob that is no longer the closest one.
func (b *Bot) NearestAttackable(
	maxDistance float64, zone *Zone,
) (AttackTarget, bool) {
	return b.NearestAttackableExcept(maxDistance, zone, nil)
}

// NearestAttackableExcept returns the closest living attackable npc of
// the zone like NearestAttackable, skipping the given object ids: the
// engage marks a target that never starts the fight as stuck (a stale
// server side selection of a corpse keeps refusing every forced attack
// on the same object id - only the next selection of a different object
// replaces it) and searches for a different target for a while. The
// skip list is a dense id slice the caller rebuilds into a reused
// buffer - no per call map allocation on the tick path.
func (b *Bot) NearestAttackableExcept(
	maxDistance float64, zone *Zone, skip []int32,
) (AttackTarget, bool) {
	return b.nearestAttackable(maxDistance, zone, skip, 0, 0, false, nil)
}

// ZoneHasAttackable reports whether at least one living attackable
// npc stands inside the zone square (the projected position, so a
// walking mob counts where it actually is). The zone rotation uses it
// as the emptiness reading of a hunting ground: the constrained
// search of the engage fences the socially packed camps out, while
// this check answers the plain question of whether the square still
// holds anything to kill at all. A nil zone never holds mobs.
func (b *Bot) ZoneHasAttackable(zone *Zone) bool {
	return b.ZoneHasAttackableBelow(zone, 0)
}

// ZoneHasAttackableBelow reports whether at least one living
// attackable npc stands inside the zone square AND passes the level
// ceiling of the engage (a mob above maxLevel never enters a fight
// this character can win, so a square whose survivors all sit above
// the ceiling is as good as empty for the rotation; level 0 disables
// the filter like everywhere in the target search, an unresolved
// template counts as passable). A nil zone never holds mobs.
func (b *Bot) ZoneHasAttackableBelow(zone *Zone, maxLevel int32) bool {
	if zone == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	nowNano := time.Now().UnixNano()
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		if maxLevel > 0 && obj.Level > maxLevel && obj.Level > 0 {
			continue
		}
		x, y := projectedPosition(obj, nowNano)
		if zone.Contains(int32(math.Round(x)), int32(math.Round(y))) {
			return true
		}
	}

	return false
}

// ZoneHasPickable reports whether the engage target search could take
// ANY npc of the zone right now: the pick's own filters apply - the
// level window, the skip list and the social clan fence - while the
// distance plays no role (the far target walk covers the whole
// square, a mob at any in-zone distance is reachable). The zone
// rotation uses this as its emptiness reading: a square whose only
// survivors stand in mutually fenced social packs reads EMPTY to the
// picker even though mobs live in it, and a hunter that waits in such
// a square forever - no pick, no far walk, no patrol leg off the
// center - stalls the whole session. A nil zone never holds pickable
// targets.
func (b *Bot) ZoneHasPickable(
	zone *Zone, maxLevel int32, skip []int32,
) bool {
	return b.ZoneHasPickableWindowed(zone, 0, maxLevel, skip)
}

// ZoneHasPickableWindowed extends ZoneHasPickable with the level
// window floor of the spot hunting: mobs below minLevel pay no adena
// at the character's level (the level gap penalty edge), so a ground
// whose only survivors sit below the floor reads empty for the
// wait-or-move economy - the same way the ceiling reads the too-high
// survivors. Zero disables the floor (an unresolved template passes
// both bounds, matching the ceiling semantics).
func (b *Bot) ZoneHasPickableWindowed(
	zone *Zone, minLevel int32, maxLevel int32, skip []int32,
) bool {
	_, ok := b.nearestAttackable(
		math.MaxFloat64, zone, skip, minLevel, maxLevel, true, nil)

	return ok
}

// BlockedTarget describes one living attackable npc the engage target
// search rejected: the projected position and the human readable
// rejection reason. The targetless diagnostic of the hunt loop logs
// them, so a standing hunter shows WHICH mobs it sees around itself
// and WHY it does not attack them.
type BlockedTarget struct {
	ObjectID int32
	Name     string
	X        int32
	Y        int32
	Z        int32
	Reason   string
}

// NearestBlockedTargets classifies the living attackable npcs around
// the character exactly the way the engage pick does and returns the
// nearest ones the pick rejected, with the projected position and the
// reason: the skip list, the level window, the zone square or the
// social clan fence (in the pick's own check order). Mobs that pass
// every filter stay out of the list - the far target walk reaches
// them, they are simply not engaged yet. The limit bounds the list:
// the diagnostic logs a handful of the nearest mobs, not the whole
// knownlist.
func (b *Bot) NearestBlockedTargets(
	zone *Zone, maxLevel int32, skip []int32, limit int,
) []BlockedTarget {
	return b.NearestBlockedTargetsWindowed(zone, 0, maxLevel, skip, limit)
}

// NearestBlockedTargetsWindowed extends NearestBlockedTargets with the
// level window floor of the spot hunting: a mob below the floor shows
// its reason like the ones above the ceiling.
func (b *Bot) NearestBlockedTargetsWindowed(
	zone *Zone, minLevel int32, maxLevel int32, skip []int32, limit int,
) []BlockedTarget {
	if limit <= 0 {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	nowNano := time.Now().UnixNano()
	scanPtr := npcScanPool.Get().(*[]npcScan)
	scans := (*scanPtr)[:0]
	defer func() {
		*scanPtr = scans[:0]
		npcScanPool.Put(scanPtr)
	}()
	scans = b.appendAttackableScans(scans, nowNano)
	type rejectedEntry struct {
		target BlockedTarget
		dist   float64
	}
	entries := make([]rejectedEntry, 0, len(scans))
	for i := range scans {
		cand := &scans[i]
		reason := blockedReason(
			scans, cand, b.world.cold, zone, minLevel, maxLevel, skip)
		if reason == "" {
			continue
		}
		entries = append(entries, rejectedEntry{
			target: BlockedTarget{
				ObjectID: cand.objectID,
				Name:     b.world.cold[cand.slot].Name,
				X:        int32(math.Round(cand.x)),
				Y:        int32(math.Round(cand.y)),
				Z:        cand.z,
				Reason:   reason,
			},
			dist: math.Hypot(cand.x-selfX, cand.y-selfY),
		})
	}
	slices.SortStableFunc(entries, func(a, b rejectedEntry) int {
		return cmp.Compare(a.dist, b.dist)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	blocked := make([]BlockedTarget, 0, len(entries))
	for i := range entries {
		blocked = append(blocked, entries[i].target)
	}

	return blocked
}

// blockedReason mirrors the rejection chain of the constrained target
// search for one scan record and returns the human readable reason
// (empty when the pick could take the mob). The order matches the
// pick's own checks: the skip list, the level window, the zone
// square, the social clan fence. The world cold half resolves the
// names the reasons print; the caller must hold the read lock.
func blockedReason(
	scans []npcScan, cand *npcScan, cold []objectCold,
	zone *Zone, minLevel int32, maxLevel int32, skip []int32,
) string {
	if skipContains(skip, cand.objectID) {
		return "skipped by the engage"
	}
	if minLevel > 0 && cand.level < minLevel && cand.level > 0 {
		return fmt.Sprintf("level %d below the window floor %d",
			cand.level, minLevel)
	}
	if maxLevel > 0 && cand.level > maxLevel && cand.level > 0 {
		return fmt.Sprintf("level %d above the ceiling %d",
			cand.level, maxLevel)
	}
	if !zone.Contains(int32(math.Round(cand.x)), int32(math.Round(cand.y))) {
		return "outside the hunting zone"
	}
	if helper := socialHelperRecord(scans, cand); helper != nil {
		return fmt.Sprintf("clan pack with %s (%d)",
			cold[helper.slot].Name, helper.objectID)
	}

	return ""
}

// targetPriorityBias converts one priority point of the zone mob
// list into the distance discount of the target search: a priority 2
// mob reads as 2 x bias units closer than it stands, so the pick
// prefers it among comparably near candidates while a far stronger
// preference still loses to a mob at the doorstep (the bias stays
// small next to the 1500 unit engage radius on purpose).
const targetPriorityBias = 200.0

// NearestAttackableConstrained returns the closest living attackable
// npc of the zone with the hunt safety constraints applied on top of
// the skip list: mobs above maxLevel are never initiated on (a level
// gap fight is a death risk, zero disables the filter) and mobs whose
// clan mates stand within their clan help range are skipped while
// avoidSocial is set - attacking them pulls the whole camp (the Mobius
// AttackableAI clan call). The level of an unknown template stays
// pass the filter: the data of the C1 dictionary is complete, an
// unknown level means the bot never resolved the template and should
// not be fenced by it.
func (b *Bot) NearestAttackableConstrained(
	maxDistance float64, zone *Zone, skip []int32,
	maxLevel int32, avoidSocial bool,
) (AttackTarget, bool) {
	return b.nearestAttackable(
		maxDistance, zone, skip, 0, maxLevel, avoidSocial, nil)
}

// NearestAttackablePreferred extends NearestAttackableConstrained
// with the zone mob priorities: every priority point of the template
// id biases the pick by targetPriorityBias units of distance, so the
// engage farms every species of the ground while tilting toward the
// exp rich mobs when several candidates sit at a comparable range. A
// nil (or empty) priority map keeps the plain nearest-first pick.
func (b *Bot) NearestAttackablePreferred(
	maxDistance float64, zone *Zone, skip []int32,
	maxLevel int32, avoidSocial bool, priority map[int32]int32,
) (AttackTarget, bool) {
	return b.nearestAttackable(
		maxDistance, zone, skip, 0, maxLevel, avoidSocial, priority)
}

// NearestAttackablePreferredWindowed extends the preferred pick with
// the level window floor of the spot hunting: mobs below minLevel pay
// no adena at the character's level and never enter a fight the spot
// economy would count as income. Zero disables the floor; an
// unresolved template (level 0) passes both bounds like everywhere in
// the target search.
func (b *Bot) NearestAttackablePreferredWindowed(
	maxDistance float64, zone *Zone, skip []int32,
	minLevel int32, maxLevel int32, avoidSocial bool,
	priority map[int32]int32,
) (AttackTarget, bool) {
	return b.nearestAttackable(
		maxDistance, zone, skip, minLevel, maxLevel,
		avoidSocial, priority)
}

// nearestAttackable is the shared target search core of the public
// pickers. The plain variant walks the dense hot storage directly; the
// socially constrained variant flattens the living attackable npcs
// into compact scan records first (see nearestAttackableSocial). The
// level window [minLevel, maxLevel] fences the worthless and the
// deadly mobs out (zero bound = disabled, an unresolved template
// passes).
func (b *Bot) nearestAttackable(
	maxDistance float64, zone *Zone, skip []int32,
	minLevel int32, maxLevel int32, avoidSocial bool,
	priority map[int32]int32,
) (AttackTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if avoidSocial {
		return b.nearestAttackableSocial(
			maxDistance, zone, skip, minLevel, maxLevel, priority)
	}

	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestScore := math.MaxFloat64
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	nowNano := time.Now().UnixNano()
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		if skipContains(skip, obj.ObjectID) {
			continue
		}
		if maxLevel > 0 && obj.Level > maxLevel && obj.Level > 0 {
			continue
		}
		if minLevel > 0 && obj.Level < minLevel && obj.Level > 0 {
			continue
		}
		x, y := projectedPosition(obj, nowNano)
		if !zone.Contains(
			int32(math.Round(x)), int32(math.Round(y))) {
			continue
		}
		dist := math.Hypot(x-selfX, y-selfY)
		if dist >= maxDistance {
			continue
		}
		score := dist - targetPriorityBias*
			float64(priority[b.world.cold[i].TemplateID])
		if score < bestScore {
			bestScore = score
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     b.world.cold[i].Name,
				X:        int32(math.Round(x)),
				Y:        int32(math.Round(y)),
				Z:        obj.Z,
			}
		}
	}

	return best, found
}

// nearestAttackableSocial is the constrained variant of the target
// search: one pass flattens the living attackable npcs into compact
// scan records (one projection per npc, clans as bitmasks), the pair
// check of the social pull walks that flat array - the old
// implementation rescanned the whole world storage per candidate. The
// caller must hold the read lock.
func (b *Bot) nearestAttackableSocial(
	maxDistance float64, zone *Zone, skip []int32,
	minLevel int32, maxLevel int32, priority map[int32]int32,
) (AttackTarget, bool) {
	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestScore := math.MaxFloat64
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	nowNano := time.Now().UnixNano()
	// The flat scan records come from a pool: the search runs under
	// the read lock (concurrent readers), so a per bot scratch would
	// race - the pool hands every caller its own array and the fleet
	// of searches shares the memory instead of allocating a fresh
	// block per tick. The flattening walk streams the compact hot
	// records only; the name and template of a candidate resolve
	// through the cold half afterwards.
	scanPtr := npcScanPool.Get().(*[]npcScan)
	scans := (*scanPtr)[:0]
	defer func() {
		*scanPtr = scans[:0]
		npcScanPool.Put(scanPtr)
	}()
	scans = b.appendAttackableScans(scans, nowNano)
	for i := range scans {
		cand := &scans[i]
		if skipContains(skip, cand.objectID) {
			continue
		}
		if maxLevel > 0 && cand.level > maxLevel && cand.level > 0 {
			continue
		}
		if minLevel > 0 && cand.level < minLevel && cand.level > 0 {
			continue
		}
		if !zone.Contains(
			int32(math.Round(cand.x)), int32(math.Round(cand.y))) {
			continue
		}
		if socialHelpersNear(scans, cand) {
			continue
		}
		dist := math.Hypot(cand.x-selfX, cand.y-selfY)
		if dist >= maxDistance {
			continue
		}
		score := dist - targetPriorityBias*
			float64(priority[cand.templateID])
		if score < bestScore {
			bestScore = score
			found = true
			best = AttackTarget{
				ObjectID: cand.objectID,
				Name:     b.world.cold[cand.slot].Name,
				X:        int32(math.Round(cand.x)),
				Y:        int32(math.Round(cand.y)),
				Z:        cand.z,
			}
		}
	}

	return best, found
}

// appendAttackableScans flattens every living attackable npc of the
// dense hot storage into the pooled scan array (one projected
// position per npc, clans as bitmasks, the template id for the
// priority bias). The template read touches the cold half, the filter
// runs on the hot half first so non npcs cost nothing. The caller
// must hold the read lock.
func (b *Bot) appendAttackableScans(
	scans []npcScan, nowNano int64,
) []npcScan {
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		x, y := projectedPosition(obj, nowNano)
		scans = append(scans, npcScan{
			x:             x,
			y:             y,
			z:             obj.Z,
			objectID:      obj.ObjectID,
			slot:          int32(i),
			level:         obj.Level,
			templateID:    b.world.cold[i].TemplateID,
			clanHelpRange: obj.ClanHelpRange,
			clanMask:      obj.ClanMask,
		})
	}

	return scans
}

// npcScanPool recycles the flat scan arrays of the constrained target
// search across the calls (pointer to slice, so the grown capacity
// travels back into the pool).
var npcScanPool = sync.Pool{
	New: func() any {
		scans := make([]npcScan, 0, 64)

		return &scans
	},
}

// skipContains reports whether the dense skip list holds the id. The
// lists stay tiny (the handful of targets the engage or the flee flow
// held out), so the linear walk beats a map hash lookup and keeps the
// tick path allocation free.
func skipContains(skip []int32, objectID int32) bool {
	for _, id := range skip {
		if id == objectID {
			return true
		}
	}

	return false
}

// socialHelpMargin widens the clan help radius of the target search: a
// pack mate that wanders into the radius while the fight runs would
// join it, so the pick keeps a spare margin instead of trusting the
// frozen positions of the last packets.
const socialHelpMargin = 200.0

// socialHelpZLimit mirrors the Mobius AttackableAI guard: clan mates
// more than 600 units apart in height never answer the call.
const socialHelpZLimit = 600.0

// npcScan is the compact scan record of one living attackable npc:
// the projected position, the z level, the clan bitmask and the
// slot of the world record packed into one cache friendly block.
// The constrained target search builds one array of them per call
// and the social pull check walks it pairwise - no struct copies
// out of the world storage, no repeated projections, no string
// work in the pair loop.
type npcScan struct {
	x             float64
	y             float64
	z             int32
	objectID      int32
	slot          int32
	level         int32
	templateID    int32
	clanHelpRange int32
	clanMask      uint64
}

// socialHelpersNear reports whether attacking the candidate would
// pull its clan mates: the Mobius AttackableAI lets the attacked npc
// call every nearby attackable that shares one of its clans (the
// special ALL clan matches everything) within its clanHelpRange. The
// positions of the flat scan array are the projected ones, so moving
// pack mates are measured where they actually stand.
func socialHelpersNear(scans []npcScan, cand *npcScan) bool {
	return socialHelperRecord(scans, cand) != nil
}

// socialHelperRecord finds the clan mate that fences the candidate out
// of the target search (the pack mate whose assistance the attack
// would pull), nil for a loner. socialHelpersNear and the blocked
// target diagnostic share it, so the fence semantics live in exactly
// one place.
func socialHelperRecord(scans []npcScan, cand *npcScan) *npcScan {
	if cand.clanHelpRange <= 0 || cand.clanMask == 0 {
		return nil
	}
	reach := float64(cand.clanHelpRange) + socialHelpMargin
	reachSq := reach * reach
	for i := range scans {
		other := &scans[i]
		if other.objectID == cand.objectID {
			continue
		}
		if !clanMaskAssists(cand.clanMask, other.clanMask) {
			continue
		}
		zDiff := other.z - cand.z
		if zDiff < 0 {
			zDiff = -zDiff
		}
		if float64(zDiff) > socialHelpZLimit {
			continue
		}
		dx := other.x - cand.x
		dy := other.y - cand.y
		if dx*dx+dy*dy <= reachSq {
			return other
		}
	}

	return nil
}

// clanMaskAssists mirrors the Mobius clan check of the assist call on
// the precomputed bitmasks: the attacked npc calls the nearby npc when
// their clans share a bit, or when the attacked npc itself belongs to
// the ALL clan (ALL matches every clan). The single sided ALL keeps
// the server semantics: a lone ALL mob next to a clanned mob does not
// pull it.
func clanMaskAssists(attacked uint64, helper uint64) bool {
	if attacked == 0 || helper == 0 {
		return false
	}
	if attacked&npcdata.ClanMaskAll != 0 {
		return true
	}

	return attacked&helper&^npcdata.ClanMaskAll != 0
}

// NearestNpcByTemplates returns the closest living npc whose template id
// is in the given set, within the distance of the character. The town
// trip uses it to find the shop merchant spawned at the destination
// coordinates. Merchants never move, so the raw packet position is used.
func (b *Bot) NearestNpcByTemplates(
	templates []int32, maxDistance float64,
) (AttackTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestDist := maxDistance
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || obj.Dead {
			continue
		}
		if !templateWanted(b.world.cold[i].TemplateID, templates) {
			continue
		}
		dist := math.Hypot(
			float64(obj.X)-selfX, float64(obj.Y)-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     b.world.cold[i].Name,
				X:        obj.X,
				Y:        obj.Y,
				Z:        obj.Z,
			}
		}
	}

	return best, found
}

// templateWanted reports whether the template id is in the wanted set.
// The merchant lists of the town trips carry a handful of ids, so the
// linear scan beats a per call map allocation.
func templateWanted(templateID int32, templates []int32) bool {
	for _, id := range templates {
		if id == templateID {
			return true
		}
	}

	return false
}

// MedianZoneMobLevel returns the median level of the living attackable
// npcs inside the zone, zero when none of them is visible or known: the
// delevel policy compares the character level against it to detect a
// hunting ground whose monsters are too low for the character. Nil zones
// mean no limit and return zero.
func (b *Bot) MedianZoneMobLevel(zone *Zone) int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if zone == nil {
		return 0
	}
	levels := make([]int32, 0, len(b.world.hot))
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind != kindNPC || !obj.Attackable || obj.Dead ||
			obj.Level <= 0 {
			continue
		}
		if !zone.Contains(obj.X, obj.Y) {
			continue
		}
		levels = append(levels, obj.Level)
	}
	if len(levels) == 0 {
		return 0
	}
	slices.Sort(levels)

	return levels[len(levels)/2]
}

// projectedPosition estimates where an object is right now: standing
// objects keep their packet position, moving ones advance from the
// segment start toward the destination at their effective speed. It is
// the server side counterpart of the web map interpolation (the Mobius
// Creature.updatePosition loop steps creatures toward the destination
// every 100 ms game tick from the last broadcast position).
func projectedPosition(obj *objectHot, nowNano int64) (float64, float64) {
	if !obj.Moving || obj.MoveAt == 0 {
		return float64(obj.X), float64(obj.Y)
	}
	dx := float64(obj.DestX - obj.X)
	dy := float64(obj.DestY - obj.Y)
	dist := math.Hypot(dx, dy)
	speed := obj.effectiveSpeed()
	if dist < 1 || speed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	elapsed := float64(nowNano-obj.MoveAt) / 1e9
	if elapsed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	traveled := math.Min(speed*elapsed, dist)
	frac := traveled / dist

	return float64(obj.X) + dx*frac, float64(obj.Y) + dy*frac
}
