// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package state

import (
	"math"
	"slices"
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
	now := time.Now()
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead ||
			obj.TargetID != b.selfID {
			continue
		}
		x, y := projectedPosition(obj, now)
		dist := math.Hypot(x-selfX, y-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     obj.Name,
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
// replaces it) and searches for a different target for a while.
func (b *Bot) NearestAttackableExcept(
	maxDistance float64, zone *Zone, skip map[int32]bool,
) (AttackTarget, bool) {
	return b.nearestAttackable(maxDistance, zone, skip, 0, false)
}

// ZoneHasAttackable reports whether at least one living attackable
// npc stands inside the zone square (the projected position, so a
// walking mob counts where it actually is). The zone rotation uses it
// as the emptiness reading of a hunting ground: the constrained
// search of the engage fences the socially packed camps out, while
// this check answers the plain question of whether the square still
// holds anything to kill at all. A nil zone never holds mobs.
func (b *Bot) ZoneHasAttackable(zone *Zone) bool {
	if zone == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	now := time.Now()
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		x, y := projectedPosition(obj, now)
		if zone.Contains(int32(math.Round(x)), int32(math.Round(y))) {
			return true
		}
	}

	return false
}

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
	maxDistance float64, zone *Zone, skip map[int32]bool,
	maxLevel int32, avoidSocial bool,
) (AttackTarget, bool) {
	return b.nearestAttackable(maxDistance, zone, skip, maxLevel, avoidSocial)
}

// nearestAttackable is the shared target search core of the two public
// pickers. The plain variant walks the dense storage directly; the
// socially constrained variant flattens the living attackable npcs
// into compact scan records first (see nearestAttackableSocial).
func (b *Bot) nearestAttackable(
	maxDistance float64, zone *Zone, skip map[int32]bool,
	maxLevel int32, avoidSocial bool,
) (AttackTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if avoidSocial {
		return b.nearestAttackableSocial(
			maxDistance, zone, skip, maxLevel)
	}

	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestDist := maxDistance
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	now := time.Now()
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		if skip[obj.ObjectID] {
			continue
		}
		if maxLevel > 0 && obj.Level > maxLevel && obj.Level > 0 {
			continue
		}
		x, y := projectedPosition(obj, now)
		if !zone.Contains(
			int32(math.Round(x)), int32(math.Round(y))) {
			continue
		}
		dist := math.Hypot(x-selfX, y-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     obj.Name,
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
	maxDistance float64, zone *Zone, skip map[int32]bool, maxLevel int32,
) (AttackTarget, bool) {
	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestDist := maxDistance
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	now := time.Now()
	scans := make([]npcScan, 0, len(b.world.objects))
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		x, y := projectedPosition(obj, now)
		scans = append(scans, npcScan{
			x:             x,
			y:             y,
			z:             obj.Z,
			objectID:      obj.ObjectID,
			slot:          int32(i),
			level:         obj.Level,
			clanHelpRange: obj.ClanHelpRange,
			clanMask:      obj.ClanMask,
		})
	}
	for i := range scans {
		cand := &scans[i]
		if skip[cand.objectID] {
			continue
		}
		if maxLevel > 0 && cand.level > maxLevel && cand.level > 0 {
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
		if dist < bestDist {
			bestDist = dist
			found = true
			obj := &b.world.objects[cand.slot]
			best = AttackTarget{
				ObjectID: cand.objectID,
				Name:     obj.Name,
				X:        int32(math.Round(cand.x)),
				Y:        int32(math.Round(cand.y)),
				Z:        cand.z,
			}
		}
	}

	return best, found
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
	if cand.clanHelpRange <= 0 || cand.clanMask == 0 {
		return false
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
			return true
		}
	}

	return false
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
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || obj.Dead {
			continue
		}
		if !templateWanted(obj.TemplateID, templates) {
			continue
		}
		dist := math.Hypot(
			float64(obj.X)-selfX, float64(obj.Y)-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     obj.Name,
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
	levels := make([]int32, 0, len(b.world.objects))
	for i := range b.world.objects {
		obj := &b.world.objects[i]
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead ||
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
func projectedPosition(obj *WorldObject, now time.Time) (float64, float64) {
	if !obj.Moving || obj.MoveAt.IsZero() {
		return float64(obj.X), float64(obj.Y)
	}
	dx := float64(obj.DestX - obj.X)
	dy := float64(obj.DestY - obj.Y)
	dist := math.Hypot(dx, dy)
	speed := obj.EffectiveSpeed()
	if dist < 1 || speed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	elapsed := now.Sub(obj.MoveAt).Seconds()
	if elapsed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	traveled := math.Min(speed*elapsed, dist)
	frac := traveled / dist

	return float64(obj.X) + dx*frac, float64(obj.Y) + dy*frac
}
