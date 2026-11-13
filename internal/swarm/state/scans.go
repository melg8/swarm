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

// Zone is a square world area: the hunting policy (attack and loot inside,
// never wander out). A nil zone means no limit.
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

// ZoneArea is the geometric leash contract: the target searches, the loot scans
// and the emptiness readings ask whether a point belongs to the hunted ground
// (the square *Zone or the convex *CellZone). A nil area means no limit.
type ZoneArea interface {
    Contains(x int32, y int32) bool
}

// areaNil reports whether a ZoneArea carries no leash: a nil interface or a
// typed nil *Zone, which is not a nil interface.
func areaNil(zone ZoneArea) bool {
    if zone == nil {
        return true
    }
    square, ok := zone.(*Zone)

    return ok && square == nil
}

// NearestAttacker returns the closest living attackable npc that targets the
// character at its projected position (the mob whose blows land, the chase of
// the flight).
func (b *Bot) NearestAttacker() (AttackTarget, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    //nolint:exhaustruct_v5 // zero value grows inside the loop
    best := AttackTarget{}
    bestDist := math.MaxFloat64
    found := false
    selfX := float64(b.char.X)
    selfY := float64(b.char.Y)
    nowNano := b.now().UnixNano()
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

// SelfAttackers returns every living attackable npc that targets the character
// (the full aggro load) at the raw last known positions; a caller measuring a
// moving chase wants NearestAttacker.
func (b *Bot) SelfAttackers() []AttackTarget {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return nil
    }
    attackers := make([]AttackTarget, 0, 4)
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if obj.Kind != kindNPC || !obj.Attackable || obj.Dead ||
            obj.TargetID != b.selfID {
            continue
        }
        attackers = append(attackers, AttackTarget{
            ObjectID: obj.ObjectID,
            Name:     b.world.cold[i].Name,
            X:        obj.X,
            Y:        obj.Y,
            Z:        obj.Z,
        })
    }

    return attackers
}

// HasBowAttackerBesides reports whether a living attackable npc other than the
// given object targets the character with a bow in the right hand: the
// losing-fight verdict never tries to out-kite a ranged enemy. The scan is
// allocation free (every engage tick).
func (b *Bot) HasBowAttackerBesides(objectID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return false
    }
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if obj.Kind == kindNPC && obj.Attackable && !obj.Dead &&
            obj.TargetID == b.selfID && obj.ObjectID != objectID &&
            obj.BowWielding {
            return true
        }
    }

    return false
}

// NearestAttackable returns the closest living attackable npc within
// maxDistance, at the projected position: the once per second movement
// broadcast leaves a moving mob far from its last packet position.
func (b *Bot) NearestAttackable(
    maxDistance float64, zone ZoneArea,
) (AttackTarget, bool) {
    return b.NearestAttackableExcept(maxDistance, zone, nil)
}

// NearestAttackableExcept returns the closest living attackable npc of the
// zone, skipping the given object ids: the engage marks a target that never
// starts the fight as stuck and searches elsewhere for a while. The dense id
// slice keeps the tick path allocation free.
func (b *Bot) NearestAttackableExcept(
    maxDistance float64, zone ZoneArea, skip []int32,
) (AttackTarget, bool) {
    return b.nearestAttackable(
        maxDistance, zone, skip, 0, 0, false, false, nil)
}

// ZoneHasAttackable reports whether a living attackable npc stands inside the
// zone at its projected position (the emptiness reading of the rotation). A nil
// zone never holds mobs.
func (b *Bot) ZoneHasAttackable(zone ZoneArea) bool {
    return b.ZoneHasAttackableBelow(zone, 0)
}

// ZoneHasAttackableBelow adds the level ceiling of the engage: a square whose
// survivors all sit above maxLevel is as good as empty (zero disables the
// filter, an unresolved template passes). A nil zone never holds mobs.
func (b *Bot) ZoneHasAttackableBelow(zone ZoneArea, maxLevel int32) bool {
    if areaNil(zone) {
        return false
    }
    b.mu.RLock()
    defer b.mu.RUnlock()
    nowNano := b.now().UnixNano()
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

// ZoneHasPickable reports whether the engage search could take any npc of the
// zone now (the level window, the skip list and the clan fence apply, the
// distance does not). A square whose only survivors sit in fenced packs reads
// empty, so the rotation never stalls on it.
func (b *Bot) ZoneHasPickable(
    zone ZoneArea, maxLevel int32, skip []int32,
) bool {
    return b.ZoneHasPickableWindowed(zone, 0, maxLevel, skip, false)
}

// ZoneHasPickableWindowed adds the level window floor: mobs below minLevel pay
// no adena and read as empty (zero disables the floor, an unresolved template
// passes). avoidBows fences the archers out like the pick itself.
func (b *Bot) ZoneHasPickableWindowed(
    zone ZoneArea, minLevel int32, maxLevel int32, skip []int32,
    avoidBows bool,
) bool {
    _, ok := b.nearestAttackable(
        math.MaxFloat64, zone, skip, minLevel, maxLevel, true,
        avoidBows, nil)

    return ok
}

// AggroThreat describes one aggressive npc an autonomous walk must not pass
// close to: the projected position, the effective on-sight aggro range and the
// identity for the logs.
type AggroThreat struct {
    ObjectID   int32
    TemplateID int32
    Name       string
    X          float64
    Y          float64
    Z          int32
    // AggroRange is the effective trigger radius (the template value clamped at
    // MaxAggroRange for the Monster type).
    AggroRange float64
}

// AppendAggroThreats appends the living idle aggressive mobs within maxDistance
// at their projected positions: a mob that already holds a target is no
// steering problem, and excludeID drops the current fight target. The callers
// hand reused buffers in.
func (b *Bot) AppendAggroThreats(
    dst []AggroThreat, excludeID int32, maxDistance float64,
) []AggroThreat {
    b.mu.RLock()
    defer b.mu.RUnlock()
    selfX := float64(b.char.X)
    selfY := float64(b.char.Y)
    nowNano := b.now().UnixNano()
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if obj.Kind != kindNPC || !obj.Attackable || obj.Dead ||
            !obj.Aggressive || obj.TargetID != 0 ||
            obj.ObjectID == excludeID {
            continue
        }
        x, y := projectedPosition(obj, nowNano)
        if math.Hypot(x-selfX, y-selfY) >= maxDistance {
            continue
        }
        dst = append(dst, AggroThreat{
            ObjectID:   obj.ObjectID,
            TemplateID: b.world.cold[i].TemplateID,
            Name:       b.world.cold[i].Name,
            X:          x,
            Y:          y,
            Z:          obj.Z,
            AggroRange: float64(b.world.cold[i].AggroRange),
        })
    }

    return dst
}

// BlockedTarget describes one living attackable npc the engage search rejected:
// the projected position and the reason the targetless diagnostic logs.
type BlockedTarget struct {
    ObjectID int32
    Name     string
    X        int32
    Y        int32
    Z        int32
    Reason   string
}

// NearestBlockedTargets classifies the living attackable npcs the way the
// engage pick does and returns the nearest rejected ones with the reason, in
// the pick's own order. Mobs that pass every filter stay out (the far target
// walk reaches them). The limit bounds the diagnostic list.
func (b *Bot) NearestBlockedTargets(
    zone ZoneArea, maxLevel int32, skip []int32, limit int,
) []BlockedTarget {
    return b.NearestBlockedTargetsWindowed(
        zone, 0, maxLevel, skip, limit, false)
}

// NearestBlockedTargetsWindowed adds the level window floor and the bow fence,
// so the diagnostic explains the archer the hunt passed over.
func (b *Bot) NearestBlockedTargetsWindowed(
    zone ZoneArea, minLevel int32, maxLevel int32, skip []int32, limit int,
    avoidBows bool,
) []BlockedTarget {
    if limit <= 0 {
        return nil
    }
    b.mu.RLock()
    defer b.mu.RUnlock()
    selfX := float64(b.char.X)
    selfY := float64(b.char.Y)
    nowNano := b.now().UnixNano()
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
            scans, cand, b.world.cold, zone, minLevel, maxLevel, skip,
            avoidBows)
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

// blockedReason mirrors the rejection chain of the constrained search for one
// scan record and returns the reason (empty when the pick could take the mob):
// the skip list, the level window, the bow fence, the zone square, the clan
// fence. The caller holds the read lock.
func blockedReason(
    scans []npcScan, cand *npcScan, cold []objectCold,
    zone ZoneArea, minLevel int32, maxLevel int32, skip []int32,
    avoidBows bool,
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
    if avoidBows && cand.bowWielding {
        return "bow wielder (the archer trade is avoided)"
    }
    if zone != nil && !zone.Contains(int32(math.Round(cand.x)),
        int32(math.Round(cand.y))) {
        return "outside the hunting zone"
    }
    if helper := socialHelperRecord(scans, cand); helper != nil {
        return fmt.Sprintf("clan pack with %s (%d)",
            cold[helper.slot].Name, helper.objectID)
    }

    return ""
}

// targetPriorityBias converts one priority point into the distance discount of
// the search: small next to the 1500 unit engage radius, so a mob at the
// doorstep still beats a far preferred one.
const targetPriorityBias = 200.0

// NearestAttackableConstrained returns the closest living attackable npc with
// the safety constraints: mobs above maxLevel never enter a fight (a level gap
// fight is a death risk, zero disables), and mobs whose clan mates stand within
// their help range are skipped while avoidSocial is set - attacking them pulls
// the whole camp (the Mobius AttackableAI clan call). An unknown template
// passes.
func (b *Bot) NearestAttackableConstrained(
    maxDistance float64, zone ZoneArea, skip []int32,
    maxLevel int32, avoidSocial bool,
) (AttackTarget, bool) {
    return b.nearestAttackable(
        maxDistance, zone, skip, 0, maxLevel, avoidSocial, false, nil)
}

// NearestAttackablePreferred adds the zone mob priorities: every priority point
// biases the pick by targetPriorityBias units, so the engage tilts toward the
// exp rich species. A nil map keeps the nearest-first pick, and avoidBows keeps
// a bow profile from initiating on an archer (the defensive answer runs through
// the attacker scan).
func (b *Bot) NearestAttackablePreferred(
    maxDistance float64, zone ZoneArea, skip []int32,
    maxLevel int32, avoidSocial bool, priority map[int32]int32,
    avoidBows bool,
) (AttackTarget, bool) {
    return b.nearestAttackable(
        maxDistance, zone, skip, 0, maxLevel, avoidSocial, avoidBows,
        priority)
}

// NearestAttackablePreferredWindowed adds the level window floor: mobs below
// minLevel pay no adena (zero disables the floor, level 0 passes both bounds),
// and avoidBows applies the bow fence of the preferred pick.
func (b *Bot) NearestAttackablePreferredWindowed(
    maxDistance float64, zone ZoneArea, skip []int32,
    minLevel int32, maxLevel int32, avoidSocial bool,
    priority map[int32]int32, avoidBows bool,
) (AttackTarget, bool) {
    return b.nearestAttackable(
        maxDistance, zone, skip, minLevel, maxLevel,
        avoidSocial, avoidBows, priority)
}

// nearestAttackable is the shared search core: the plain variant walks the hot
// storage directly, the constrained one flattens the attackable npcs into scan
// records first.
//
//nolint:cyclop // the zone, level, social and priority filters are one scan
func (b *Bot) nearestAttackable(
    maxDistance float64, zone ZoneArea, skip []int32,
    minLevel int32, maxLevel int32, avoidSocial bool, avoidBows bool,
    priority map[int32]int32,
) (AttackTarget, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if avoidSocial {
        return b.nearestAttackableSocial(
            maxDistance, zone, skip, minLevel, maxLevel, avoidBows,
            priority)
    }

    //nolint:exhaustruct_v5 // zero value grows inside the loop
    best := AttackTarget{}
    bestScore := math.MaxFloat64
    found := false
    selfX := float64(b.char.X)
    selfY := float64(b.char.Y)
    nowNano := b.now().UnixNano()
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
        if avoidBows && obj.BowWielding {
            continue
        }
        x, y := projectedPosition(obj, nowNano)
        if zone != nil && !zone.Contains(
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

// nearestAttackableSocial is the constrained variant: one pass flattens the
// npcs into compact scan records (one projection each, clans as bitmasks) and
// the pull check walks that flat array. The caller holds the read lock.
func (b *Bot) nearestAttackableSocial(
    maxDistance float64, zone ZoneArea, skip []int32,
    minLevel int32, maxLevel int32, avoidBows bool,
    priority map[int32]int32,
) (AttackTarget, bool) {
    //nolint:exhaustruct_v5 // zero value grows inside the loop
    best := AttackTarget{}
    bestScore := math.MaxFloat64
    found := false
    selfX := float64(b.char.X)
    selfY := float64(b.char.Y)
    nowNano := b.now().UnixNano()
    // The records come from a pool: the search runs under the read lock, so a
    // per bot scratch would race, and the pool shares the memory.
    scanPtr := npcScanPool.Get().(*[]npcScan)
    scans := (*scanPtr)[:0]
    defer func() {
        *scanPtr = scans[:0]
        npcScanPool.Put(scanPtr)
    }()
    scans = b.appendAttackableScans(scans, nowNano)
    for i := range scans {
        cand := &scans[i]
        if !socialCandidateKeep(scans, cand, zone, skip,
            minLevel, maxLevel, avoidBows) {
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

// socialCandidateKeep reports whether one flattened candidate passes the
// filters of the constrained search: the skip list, the level window, the bow
// fence, the zone box and the pull.
func socialCandidateKeep(
    scans []npcScan, cand *npcScan, zone ZoneArea, skip []int32,
    minLevel int32, maxLevel int32, avoidBows bool,
) bool {
    if skipContains(skip, cand.objectID) {
        return false
    }
    if maxLevel > 0 && cand.level > maxLevel && cand.level > 0 {
        return false
    }
    if minLevel > 0 && cand.level < minLevel && cand.level > 0 {
        return false
    }
    if avoidBows && cand.bowWielding {
        return false
    }
    if zone != nil && !zone.Contains(
        int32(math.Round(cand.x)), int32(math.Round(cand.y))) {
        return false
    }

    return !socialHelpersNear(scans, cand)
}

// appendAttackableScans flattens every living attackable npc into the pooled
// array (one projected position each, the clans as bitmasks, the template id
// for the priority bias). The caller holds the read lock.
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
            bowWielding:   obj.BowWielding,
        })
    }

    return scans
}

// npcScanPool recycles the scan arrays (pointer to slice, so the grown capacity
// travels back into the pool).
var npcScanPool = sync.Pool{
    New: func() any {
        scans := make([]npcScan, 0, 64)

        return &scans
    },
}

// skipContains reports whether the dense skip list holds the id: the lists stay
// tiny, so the linear walk beats a map lookup.
func skipContains(skip []int32, objectID int32) bool {
    for _, id := range skip {
        if id == objectID {
            return true
        }
    }

    return false
}

// socialHelpMargin widens the clan help radius: a pack mate wandering in while
// the fight runs would join it.
const socialHelpMargin = 200.0

// socialHelpZLimit mirrors the Mobius AttackableAI guard: clan mates more than
// 600 units apart in height never answer the call.
const socialHelpZLimit = 600.0

// npcScan is the compact record of one living attackable npc: the projected
// position, the z level, the clan bitmask and the world slot in one cache
// friendly block.
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
    bowWielding   bool
}

// socialHelpersNear reports whether attacking the candidate would pull its clan
// mates: the Mobius AttackableAI lets the attacked npc call every nearby
// attackable sharing one of its clans (the ALL clan matches everything) within
// its clanHelpRange.
func socialHelpersNear(scans []npcScan, cand *npcScan) bool {
    return socialHelperRecord(scans, cand) != nil
}

// socialHelperRecord finds the clan mate that fences the candidate out, nil for
// a loner: socialHelpersNear and the blocked diagnostic share it.
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

// clanMaskAssists mirrors the Mobius assist check: a shared clan bit calls the
// helper, and the ALL clan of the attacked npc matches every clan (single
// sided - a lone ALL mob next to a clanned one does not pull it).
func clanMaskAssists(attacked uint64, helper uint64) bool {
    if attacked == 0 || helper == 0 {
        return false
    }
    if attacked&npcdata.ClanMaskAll != 0 {
        return true
    }

    return attacked&helper&^npcdata.ClanMaskAll != 0
}

// NearestNpcByTemplates returns the closest living npc of the given template
// ids within maxDistance: the town trip finds its merchant with it, and
// merchants never move, so the raw packet position is honest.
func (b *Bot) NearestNpcByTemplates(
    templates []int32, maxDistance float64,
) (AttackTarget, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    //nolint:exhaustruct_v5 // zero value grows inside the loop
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

// templateWanted reports whether the template id is in the wanted set: the
// merchant lists carry a handful of ids, so the linear scan beats a per call
// map allocation.
func templateWanted(templateID int32, templates []int32) bool {
    for _, id := range templates {
        if id == templateID {
            return true
        }
    }

    return false
}

// MedianZoneMobLevel returns the median level of the living attackable npcs
// inside the zone, zero when none is known: the delevel policy compares the
// character level against it. The scan runs every hunt tick, so the median
// comes from a 256 bucket histogram (one pass, zero allocations) with a sort
// fallback past the bucket bound, which the C1 data never reaches.
func (b *Bot) MedianZoneMobLevel(zone ZoneArea) int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if areaNil(zone) {
        return 0
    }
    var buckets [256]int32
    count, overflow := b.countZoneMobLevels(zone, &buckets)
    if count == 0 {
        return 0
    }
    if !overflow {
        return bucketMedian(&buckets, count)
    }

    return b.sortedZoneMedian(zone, count)
}

// zoneMobLevelCandidate reports whether the record counts toward the zone
// median: a living, attackable npc of a positive level.
func zoneMobLevelCandidate(obj *objectHot) bool {
    return obj.Kind == kindNPC && obj.Attackable && !obj.Dead &&
        obj.Level > 0
}

// countZoneMobLevels runs the histogram pass over the hot records and reports a
// level past the bucket bound (the sort fallback re-scans then). The caller
// holds the bot lock.
func (b *Bot) countZoneMobLevels(
    zone ZoneArea, buckets *[256]int32,
) (int32, bool) {
    count := int32(0)
    overflow := false
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if !zoneMobLevelCandidate(obj) ||
            !zone.Contains(obj.X, obj.Y) {
            continue
        }
        count++
        if obj.Level < int32(len(buckets)) {
            buckets[obj.Level]++
        } else {
            overflow = true
        }
    }

    return count, overflow
}

// bucketMedian walks the ascending level axis to the median rank: the first
// level whose cumulative count passes count/2 (zero when it never does).
func bucketMedian(buckets *[256]int32, count int32) int32 {
    seen := int32(0)
    for level := 1; level < len(buckets); level++ {
        seen += buckets[level]
        if seen > count/2 {
            return int32(level)
        }
    }

    return 0
}

// sortedZoneMedian is the overflow fallback of the median scan: the C1 data
// carries no level past the 256 bucket bound, so the allocation never arms in
// practice. The caller holds the bot lock.
func (b *Bot) sortedZoneMedian(zone ZoneArea, count int32) int32 {
    levels := make([]int32, 0, count)
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if !zoneMobLevelCandidate(obj) ||
            !zone.Contains(obj.X, obj.Y) {
            continue
        }
        levels = append(levels, obj.Level)
    }
    slices.Sort(levels)

    return levels[len(levels)/2]
}

// projectedPosition estimates where an object is now: a standing object keeps
// its packet position, a moving one advances from the segment start at its
// effective speed (the Mobius Creature.updatePosition loop steps every 100 ms).
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
