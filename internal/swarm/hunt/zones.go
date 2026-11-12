// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "strconv"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Multi zone hunting: the hunting grounds of a region are compact
// squares derived from the real Mobius spawn polygons (a territory
// spawns at uniformly random points and wanders inside it), a level band
// ladder gates them on the mob band and the equipped gear, and the loop
// rotates between the grounds of one band when a square clears faster
// than the respawn refills it. A rotated-away square keeps an empty
// cooldown, so the sweep moves forward A to B to C; the death regression
// demotes a band, and tools/generate_hunt_zones.py builds the registry.

// ZoneMob describes one mob species of a hunting zone: the whole mob
// list of the spawn territory the square covers. Every species is
// farmed; Priority adds the soft engage preference of targetPriorityBonus.
type ZoneMob struct {
    // TemplateID is the npc template id of the spawn data.
    TemplateID int32
    // Name is the display name of the mob.
    Name string
    // Level is the mob level of the npc stats.
    Level int32
    // Count is the spawned count of the parent territory.
    Count int32
    // Priority biases the engage toward this species: every point
    // makes the mob read as closer than it stands (soft tie break,
    // distance still dominates far picks).
    Priority int32
}

// HuntingZone describes one hunting ground: the level band of its
// mobs, the gear gate in zone gating points and the square the bot
// leashes itself to (the engage zone of the hunt loop).
type HuntingZone struct {
    // ID is the stable identifier of the zone (region prefixed).
    ID string
    // Name is the display name of the map view.
    Name string
    // Region groups the zones of one territory (elven, orc, ...).
    Region string
    // MinLevel and MaxLevel are the mob level band of the zone.
    MinLevel int32
    MaxLevel int32
    // MinGear is the zone gating points the equipped gear must reach
    // before the bot hunts here (see gear.TotalGearPoints).
    MinGear int32
    // CX, CY and Half describe the hunting square.
    CX   int32
    CY   int32
    Half int32
    // Mobs lists every mob species of the spawn territory the square
    // covers (all of them are farmed inside the square, the
    // priorities bias the engage; empty for hand placed zones).
    Mobs []ZoneMob
}

// regionElven is the region key of the elven lands zone registry.
const regionElven = "elven"

// regionDion is the region key of the Dion 20-25 band (the survey
// grounds of docs/band_20_25_survey.md, the zones_dion.go registry and
// the gear.DionCatalog of T-010).
const regionDion = "dion"

// zoneLevelLead is the level lead the character must hold over the top
// mob of a band before the picker opens it (the observed deaths came
// from characters engaging mobs above their own level). The starter band
// of the registry is the floor.
const zoneLevelLead = 1

// zoneSwitchPeriod bounds the automatic zone re-evaluation: the level
// changes and the gear upgrades (the town trips and the auto
// equipment) move the character up the ladder within this period.
const zoneSwitchPeriod = 30 * time.Second

// zoneRotateAfter bounds the patience of a hunter in a cleared-out
// square: the elven lands respawn a mob in 15-20 s, and the nearest
// ground of the same band is closer than waiting it out.
const zoneRotateAfter = 10 * time.Second

// zoneEmptyCooldown holds a rotated-away square out of the rotation
// contest, so the sweep moves A to B to C instead of ping ponging
// between the two nearest squares. It spans two respawn cycles.
const zoneEmptyCooldown = 40 * time.Second

// zoneDeathLimit is the death count that demotes a hunting ground: the
// band ladder caps below its band until the next level change.
const zoneDeathLimit = 3

// zoneDistance measures the anchor distance between a zone center and
// a world position.
func zoneDistance(zone HuntingZone, x int32, y int32) float64 {
    return math.Hypot(float64(zone.CX-x), float64(zone.CY-y))
}

// zoneBandRank compares the band of two zones for the picker ladder:
// a positive rank means the first zone sits higher (the picker wants
// it), zero means the same band (the tie break rules apply).
func zoneBandRank(zone HuntingZone, other HuntingZone) int {
    if zone.MaxLevel != other.MaxLevel {
        if zone.MaxLevel > other.MaxLevel {
            return 1
        }

        return -1
    }
    if zone.MinLevel != other.MinLevel {
        if zone.MinLevel > other.MinLevel {
            return 1
        }

        return -1
    }

    return 0
}

// sameBand reports whether two zones belong to one rotation group:
// the same mob level window (the rotation never changes difficulty,
// only the square).
func sameBand(zone HuntingZone, other HuntingZone) bool {
    return zone.MinLevel == other.MinLevel &&
        zone.MaxLevel == other.MaxLevel
}

// PickHuntingZone returns the best zone of the list for the character
// level, the gear points and the position: the highest band both gates
// allow (the level gate opens a band above its top mob plus the level
// lead, the gear gate waits for the equipment). Among the zones of the
// winning band the current one keeps its post, the nearest one wins an
// open contest, and maxMinLevel caps the ladder for the death regression
// (negative disables it). A character below every band hunts the starter
// band through the starter fallback contest.
func PickHuntingZone(
    zones []HuntingZone, level int32, gearPoints int32,
    currentID string, fromX int32, fromY int32, maxMinLevel int32,
) (HuntingZone, bool) {
    best := -1
    for index := range zones {
        candidate := zones[index]
        if level < candidate.MaxLevel+zoneLevelLead ||
            gearPoints < candidate.MinGear {
            continue
        }
        if maxMinLevel >= 0 && candidate.MinLevel > maxMinLevel {
            continue
        }
        if best < 0 {
            best = index

            continue
        }
        rank := zoneBandRank(candidate, zones[best])
        if rank > 0 {
            best = index

            continue
        }
        if rank < 0 {
            continue
        }
        // The same band: the current zone keeps its post, otherwise
        // the nearest center wins.
        if candidate.ID == currentID {
            if zones[best].ID != currentID {
                best = index
            }

            continue
        }
        if zones[best].ID == currentID {
            continue
        }
        if zoneDistance(candidate, fromX, fromY) <
            zoneDistance(zones[best], fromX, fromY) {
            best = index
        }
    }
    if best < 0 {
        // Below every band or the cap closed the ladder: the
        // starter band runs its own fallback contest.
        return starterZone(zones, currentID, fromX, fromY)
    }

    return zones[best], true
}

// starterZone resolves the fallback of the picker: the starter band (the
// band of the first zone of the registry) runs its own contest when no
// ladder band opens. The current ground keeps its post, an open contest
// takes the nearest one.
func starterZone(
    zones []HuntingZone, currentID string, fromX int32, fromY int32,
) (HuntingZone, bool) {
    if len(zones) == 0 {
        return noZone, false
    }
    best := 0
    for index := 1; index < len(zones); index++ {
        candidate := zones[index]
        if !sameBand(candidate, zones[0]) {
            continue
        }
        if candidate.ID == currentID {
            if zones[best].ID != currentID {
                best = index
            }

            continue
        }
        if zones[best].ID == currentID {
            continue
        }
        if zoneDistance(candidate, fromX, fromY) <
            zoneDistance(zones[best], fromX, fromY) {
            best = index
        }
    }

    return zones[best], true
}

// containsPoint reports whether the world point lies inside the
// hunting square of the zone.
func (z HuntingZone) containsPoint(x int32, y int32) bool {
    return z.Half > 0 && x >= z.CX-z.Half && x <= z.CX+z.Half &&
        y >= z.CY-z.Half && y <= z.CY+z.Half
}

// noZone is the zero zone of the not-found returns (exhaustruct
// wants the plain var instead of a partial literal).
var noZone HuntingZone

// zoneByID resolves a zone of the registry by its id.
func (l *Loop) zoneByID(id string) (HuntingZone, bool) {
    if id == "" {
        return noZone, false
    }
    for index := range l.zones {
        if l.zones[index].ID == id {
            return l.zones[index], true
        }
    }

    return noZone, false
}

// SetHuntingZones installs the zone registry of the deployment: the loop
// picks the zone on the next tick and re-evaluates it as the level and
// the gear grow. The legacy call stands the cell mode down.
func (l *Loop) SetHuntingZones(zones []HuntingZone) {
    l.cell = nil
    l.zones = zones
    l.zonePickedID = ""
    l.zoneCheckAt = time.Time{}
    l.zoneDeaths = nil
    l.zoneDeathCap = -1
    l.zoneDeathLevel = 0
    l.zoneEmptySince = time.Time{}
    l.zoneEmptyUntil = nil
    l.publishZoneView()
}

// SetHuntingZoneRegion installs the registry of one region by name: the
// elven lands run the Voronoi cell registry, the Dion band its square
// list, and an unknown region hunts without zones.
func (l *Loop) SetHuntingZoneRegion(region string) {
    l.zoneRegion = region
    switch region {
    case regionElven, "":
        l.SetHuntingCellRegion(regionElven)
    case regionDion:
        // The T-009 survey grounds: a deployment selects the region
        // explicitly, the Dion gear catalog follows it.
        l.SetHuntingZones(DionHuntingZones())
    default:
        l.logf("Hunt: no zone registry for region %q, hunting "+
            "without zones", region)
    }
}

// stopForZoneSwitch halts the walks a ground switch makes pointless (a
// manual move, a town trip, a zone return): the server keeps a walk
// running until the next move request replaces its destination, so one
// request to the current spot stops it. The selling stop and the
// deleveling keep their walks.
func (l *Loop) stopForZoneSwitch() {
    if l.phase == phaseDelevel {
        return
    }
    if l.phase == phaseUser && l.userKind == state.CommandMove {
        l.resumeAuto()
    }
    // The route change owns the target register: the held fight of the
    // old square drops with its server selection.
    l.dropAttackTarget("the zone switch owns the way")
    if l.phase == phaseTownWalk || l.phase == phaseTownReturn {
        l.resetTownTrip()
        l.zoneReturn = false
        l.zoneFails = 0
    }
    if x, y, z, ok := l.tracker.SelfPosition(); ok &&
        l.tracker.SelfWalking() {
        if err := l.game.WalkTo(x, y, z); err != nil {
            l.logf("Hunt: zone switch stop walk failed: %v", err)
        }
    }
}

// maybeSwitchZone re-evaluates the automatic zone pick: a level gain or
// a gear upgrade moves the character up the ladder, a cleared-out square
// rotates to the next ground of its band, and the death bookkeeping
// resets on a level change. The re-pick only runs between the fights.
func (l *Loop) maybeSwitchZone() {
    if l.cell != nil {
        l.cellEvaluate(l.now())

        return
    }
    if len(l.zones) == 0 {
        return
    }
    now := l.now()
    l.resetZoneDeathState()
    l.maybeRotateEmptyZone(now)
    if l.zonePickedID != "" && now.Sub(l.zoneCheckAt) < zoneSwitchPeriod {
        return
    }
    l.zoneCheckAt = now
    level := l.tracker.SelfLevel()
    fromX, fromY := l.selfZoneAnchor()
    zone, ok := PickHuntingZone(l.zones, level, l.gearPoints(),
        l.zonePickedID, fromX, fromY, l.zoneDeathCap)
    if !ok || zone.ID == l.zonePickedID {
        return
    }
    l.applyHuntingZone(zone)
    if l.journal != nil {
        l.journal.Zone(l.tracker.ID(), zone.Name,
            "the ladder re-pick at level "+strconv.Itoa(int(level)))
    }
    l.logf("Hunt: level %d with gear %d: hunting %s (levels "+
        "%d-%d)", level, l.gearPoints(), zone.Name, zone.MinLevel,
        zone.MaxLevel)
}

// selfZoneAnchor returns the anchor of the nearest zone tie break: the
// character position when known, the center of the current square
// otherwise.
func (l *Loop) selfZoneAnchor() (int32, int32) {
    if x, y, _, ok := l.tracker.SelfPosition(); ok {
        return x, y
    }
    if l.zoneHalf > 0 {
        return l.zoneCX, l.zoneCY
    }

    return 0, 0
}

// maybeRotateEmptyZone rotates the hunting ground of a cleared-out
// square: a hunter that stands central in a mob-less square for the
// rotate window moves on to the nearest ground of the same band that is
// not cooling down. The emptiness reading counts the mobs the character
// can actually PICK (the level filter, the skip list, the social clan
// fence), so a square whose only survivors sit in mutually fenced packs
// reads empty. The loot, the trip, the landing blows and the rest reset
// the timer, a held fight of a stuck engage does not.
func (l *Loop) maybeRotateEmptyZone(now time.Time) { //nolint:cyclop
    zone := l.zone()
    if zone == nil || l.zonePickedID == "" {
        l.zoneEmptySince = time.Time{}

        return
    }
    if l.phase != phaseEngage || l.tripActive() ||
        l.tracker.SelfUnderAttack() || l.tracker.SelfSitting() ||
        !l.inZoneSelf() {
        l.zoneEmptySince = time.Time{}

        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    if math.Hypot(float64(zone.CX-selfX), float64(zone.CY-selfY)) >
        float64(zone.Half) {
        // The patrol walk has not brought the character to the middle
        // yet: the emptiness reading of the far corners is not
        // trustworthy.
        l.zoneEmptySince = time.Time{}

        return
    }
    if l.tracker.ZoneHasPickable(
        zone, l.maxTargetLevel(), l.activeSkips(now)) {
        l.zoneEmptySince = time.Time{}

        return
    }
    if l.zoneEmptySince.IsZero() {
        l.zoneEmptySince = now

        return
    }
    if now.Sub(l.zoneEmptySince) < zoneRotateAfter {
        return
    }
    // The window is up: re-arm the timer either way, so a square
    // without a rotation target waits out another full window instead
    // of spinning the check every tick.
    l.zoneEmptySince = now
    next, ok := l.rotationZone(selfX, selfY, now)
    if !ok {
        return
    }
    current, currentOK := l.zoneByID(l.zonePickedID)
    if currentOK {
        // The rotated-away square keeps its cooldown: the next
        // rotations of the band sweep forward past it instead of
        // walking straight back into the cleared ground.
        l.markZoneEmpty(current.ID, now)
    }
    l.applyHuntingZone(next)
    l.zoneCheckAt = now
    if l.journal != nil {
        l.journal.Zone(l.tracker.ID(), next.Name,
            "the square cleared out")
    }
    if currentOK {
        l.logf("Hunt: %s is cleared out, rotating to %s",
            current.Name, next.Name)

        return
    }
    l.logf("Hunt: the zone is cleared out, rotating to %s",
        next.Name)
}

// markZoneEmpty starts the empty cooldown of a rotated-away zone and
// prunes the expired entries on the way (the registry holds a few
// hundred squares, the map must not grow forever).
func (l *Loop) markZoneEmpty(id string, now time.Time) {
    if l.zoneEmptyUntil == nil {
        l.zoneEmptyUntil = make(map[string]time.Time)
    }
    for zoneID, until := range l.zoneEmptyUntil {
        if now.After(until) {
            delete(l.zoneEmptyUntil, zoneID)
        }
    }
    l.zoneEmptyUntil[id] = now.Add(zoneEmptyCooldown)
}

// zoneCoolingDown reports whether the zone sits in its post-rotation
// empty cooldown at the given time.
func (l *Loop) zoneCoolingDown(id string, now time.Time) bool {
    until, ok := l.zoneEmptyUntil[id]

    return ok && now.Before(until)
}

// rotationZone picks the rotation target: the nearest zone of the same
// band (the difficulty never changes), never the current one and never a
// cooling square, so the sweep moves forward. The death cap holds even
// here.
func (l *Loop) rotationZone(
    selfX int32, selfY int32, now time.Time,
) (HuntingZone, bool) {
    current, ok := l.zoneByID(l.zonePickedID)
    if !ok {
        return noZone, false
    }
    best := -1
    bestDist := math.MaxFloat64
    for index := range l.zones {
        candidate := l.zones[index]
        if candidate.ID == current.ID ||
            !sameBand(candidate, current) {
            continue
        }
        if l.zoneDeathCap >= 0 && candidate.MinLevel > l.zoneDeathCap {
            continue
        }
        if l.zoneCoolingDown(candidate.ID, now) {
            continue
        }
        if dist := zoneDistance(candidate, selfX, selfY); dist < bestDist {
            best = index
            bestDist = dist
        }
    }
    if best < 0 {
        return noZone, false
    }

    return l.zones[best], true
}

// noteZoneDeath counts a hunting death against the zone it happened in:
// the square containing the death spot first (an emergency logout death
// lands on a fresh session before any zone pick), the picked zone as the
// fallback. The deleveling deaths never count.
func (l *Loop) noteZoneDeath() {
    l.noteZoneRegression("death", "deaths")
}

// noteZoneDanger counts one emergency logout against the zone it fired
// in: a ground that keeps piling mobs is as hostile as one that keeps
// killing. The logout spot also lands in the tracker danger ring, which
// the next session seeds back (seedZoneDanger).
func (l *Loop) noteZoneDanger() {
    if x, y, _, ok := l.tracker.SelfPosition(); ok {
        l.tracker.NoteDangerSpot(x, y)
    }
    l.noteZoneRegression("pile up", "pile ups")
}

// noteZoneRegression is the shared counting core of the zone losses:
// deaths and emergency logouts both count against the zone they
// happened in, and past the limit the ladder caps below its band.
func (l *Loop) noteZoneRegression(kind string, kinds string) {
    if l.cell != nil {
        l.cellNoteDeath(l.now())

        return
    }
    zone, ok := l.deathZone()
    if !ok {
        return
    }
    if l.zoneDeaths == nil {
        l.zoneDeaths = make(map[string]int32)
    }
    l.zoneDeaths[zone.ID]++
    count := l.zoneDeaths[zone.ID]
    if count < zoneDeathLimit {
        l.logf("Hunt: %s %d of %d in %s",
            kind, count, zoneDeathLimit, zone.Name)
        l.publishZoneView()

        return
    }
    if l.zoneDeathCap < 0 || zone.MinLevel-1 < l.zoneDeathCap {
        l.zoneDeathCap = zone.MinLevel - 1
    }
    // Force the ladder re-pick on the next living tick: the gate of
    // maybeSwitchZone passes with a zero evaluation time.
    l.zoneCheckAt = time.Time{}
    l.zoneEmptySince = time.Time{}
    l.publishZoneView()
    l.logf("Hunt: %d %s in %s, the zone outguns the "+
        "character: regressing to an easier band (capped below level "+
        "%d) until the level grows", count, kinds, zone.Name, zone.MinLevel)
}

// zoneDangerWindow bounds the emergency logout spots the seeding folds
// in: an older spot describes a ground the character outgrew.
const zoneDangerWindow = time.Hour

// seedZoneDanger folds the emergency logout spots the tracker carried
// over from the previous sessions into the fresh loop: the counters live
// in the loop instance, and an emergency logout ends its own loop, so
// without the seeding a too-hot ground never reaches the limit.
func (l *Loop) seedZoneDanger() {
    for _, spot := range l.tracker.DangerSpots(zoneDangerWindow) {
        for index := range l.zones {
            if !l.zones[index].containsPoint(spot.X, spot.Y) {
                continue
            }
            if l.zoneDeaths == nil {
                l.zoneDeaths = make(map[string]int32)
            }
            l.zoneDeaths[l.zones[index].ID]++
            l.logf("Hunt: carrying over an emergency logout at %d %d "+
                "into the zone regression of %s (count %d)",
                spot.X, spot.Y, l.zones[index].Name,
                l.zoneDeaths[l.zones[index].ID])
        }
    }
}

// deathZone resolves the zone a death counts against: the square
// containing the death spot, the picked zone for a death between squares.
func (l *Loop) deathZone() (HuntingZone, bool) {
    if x, y, _, ok := l.tracker.SelfPosition(); ok {
        for index := range l.zones {
            if l.zones[index].containsPoint(x, y) {
                return l.zones[index], true
            }
        }
    }

    return l.zoneByID(l.zonePickedID)
}

// resetZoneDeathState clears the zone death bookkeeping on a level
// change (the level is the measure of what the character pulls): every
// change re-opens the demoted bands.
func (l *Loop) resetZoneDeathState() {
    level := l.tracker.SelfLevel()
    if level == 0 || level == l.zoneDeathLevel {
        return
    }
    first := l.zoneDeathLevel == 0
    l.zoneDeathLevel = level
    if len(l.zoneDeaths) == 0 && l.zoneDeathCap < 0 {
        return
    }
    l.zoneDeaths = nil
    l.zoneDeathCap = -1
    l.publishZoneView()
    if !first {
        l.logf("Hunt: level %d: the zone death bookkeeping "+
            "resets", level)
    }
}

// applyHuntingZone switches the hunting square of the loop and the
// tracker and publishes the zone view: the remembered farm spot belongs
// to the previous square, the rotation timer starts over, the mob list
// becomes the engage bias and the held fight of the old square drops
// with its server selection.
func (l *Loop) applyHuntingZone(zone HuntingZone) {
    l.dropAttackTarget("the zone change owns the way")
    l.zonePickedID = zone.ID
    l.zoneCX, l.zoneCY, l.zoneHalf = zone.CX, zone.CY, zone.Half
    l.farmX, l.farmY, l.farmZ = 0, 0, 0
    l.zoneEmptySince = time.Time{}
    l.zoneMobPriority = zoneMobPriority(zone)
    l.tracker.SetHuntingZone(zone.CX, zone.CY, zone.Half)
    l.publishZoneView()
}

// zoneMobPriority builds the template id to priority map of a zone (nil
// when no mob carries a priority). The generated registries carry the
// Mobius CT0 xml template ids (20471 for the Kaboo Orc Fighter) while
// the NpcInfo packets identify the npc by the C4 display id plus the
// 1000000 offset (1000471), so every id translates onto its wire key.
func zoneMobPriority(zone HuntingZone) map[int32]int32 {
    var priorities map[int32]int32
    for index := range zone.Mobs {
        mob := &zone.Mobs[index]
        if mob.Priority <= 0 {
            continue
        }
        if priorities == nil {
            priorities = make(map[int32]int32, len(zone.Mobs))
        }
        priorities[npcdata.NPCWireTemplateID(mob.TemplateID)] = mob.Priority
    }

    return priorities
}

// publishZoneView pushes the zone registry with the active marker to the
// tracker snapshot: the map draws every zone and the zone panel reads
// the death counts and the demoted bands.
func (l *Loop) publishZoneView() {
    if l.tracker == nil {
        return
    }
    views := make([]state.ZoneView, 0, len(l.zones))
    for _, zone := range l.zones {
        //nolint:exhaustruct_v5 // the legacy zone view stays partial
        views = append(views, state.ZoneView{
            ID:       zone.ID,
            Name:     zone.Name,
            Region:   zone.Region,
            MinLevel: zone.MinLevel,
            MaxLevel: zone.MaxLevel,
            MinGear:  zone.MinGear,
            CX:       zone.CX,
            CY:       zone.CY,
            Half:     zone.Half,
            Active:   zone.ID == l.zonePickedID,
            Deaths:   l.zoneDeaths[zone.ID],
            Demoted:  l.zoneDeathCap >= 0 && zone.MinLevel > l.zoneDeathCap,
        })
    }
    l.tracker.SetHuntingZones(views)
}
