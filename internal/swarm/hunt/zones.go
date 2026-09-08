// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// Multi zone hunting: the hunting grounds of a region are small
// squares anchored on the real spawn territories of the Mobius spawn
// data (every square centers on a cluster of ElvenStarting.xml
// territories and only reaches as far as their mobs walk), a ladder
// of level bands gates them on the mob band and the equipped gear,
// and the loop rotates between the grounds of one band when a square
// is cleared out faster than the respawn refills it. The death
// regression demotes a band the character died in too often: three
// deaths in one square mean the character does not pull the zone, so
// the ladder caps below that band until the next level change. The
// zone registry is data driven: the elven lands are configured here,
// the other regions (orcs, dwarves, dark elves, humans) join as their
// own lists, and a mage profile changes only the gear scoring, never
// the zone model.

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
}

// regionElven is the region key of the elven lands zone registry.
const regionElven = "elven"

// elvenHuntingZones ladders the elven lands in ten mob level bands
// over thirty granular squares. Every square anchors on the spawn
// territories of ElvenStarting.xml: the keltir ring around the
// village (ore n02 territories 2119_01..08), the wolf and raider
// fields around it (2119_03..13 and the south oren04 ring), the
// kaboo and goblin camps of the southeast (2119_14..21), the grunt
// and fighter woods of the southwest (ore n04 2119_22..34), the
// lieutenant and leader camps of the far west (ore n04 2019_01..24)
// and the dryad, spider and lirein forests of the deep southwest
// (ore n04 2019_02..22 and the 2020 block). The halves stay at
// 1000-1300 units: the squares of one band rotate into each other
// when a pack dies out, and the respawn (15-20 s) refills a small
// square far faster than the wide squares of the old four zone
// layout, whose halves reached past the spawn points into empty
// ground. The first zone of the list is the starter fallback of the
// picker (levels below every band hunt there).
var elvenHuntingZones = []HuntingZone{
	// Band 1-3: the keltir ring of the village surroundings.
	{
		ID: "elven-keltir-village", Name: "Keltir Meadow by the Village",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 46112, CY: 41500, Half: 1000,
	},
	{
		ID: "elven-keltir-east", Name: "Keltir Field East",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 49100, CY: 42300, Half: 1100,
	},
	{
		ID: "elven-keltir-far-east", Name: "Keltir Plain Far East",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 51300, CY: 41100, Half: 1100,
	},
	{
		ID: "elven-keltir-north", Name: "Keltir Slope North",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 47900, CY: 37500, Half: 1000,
	},
	{
		ID: "elven-keltir-west", Name: "Keltir Woods West",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 42700, CY: 42400, Half: 1200,
	},
	{
		ID: "elven-keltir-hills-west", Name: "Keltir Hills West",
		Region: regionElven, MinLevel: 1, MaxLevel: 3, MinGear: 0,
		CX: 41280, CY: 39000, Half: 1000,
	},
	// Band 3-4: the wolf downs past the keltir ring.
	{
		ID: "elven-wolf-west", Name: "Wolf Ridge West",
		Region: regionElven, MinLevel: 3, MaxLevel: 4, MinGear: 10,
		CX: 38500, CY: 40200, Half: 1200,
	},
	{
		ID: "elven-wolf-northwest", Name: "Wolf Hills Northwest",
		Region: regionElven, MinLevel: 3, MaxLevel: 4, MinGear: 10,
		CX: 41000, CY: 35200, Half: 1200,
	},
	{
		ID: "elven-wolf-north", Name: "Wolf Downs North",
		Region: regionElven, MinLevel: 3, MaxLevel: 4, MinGear: 10,
		CX: 46655, CY: 35342, Half: 1100,
	},
	// Band 4-6: the wolf-raider fields and the first goblin contact.
	{
		ID: "elven-raider-southeast", Name: "Raider Field Southeast",
		Region: regionElven, MinLevel: 4, MaxLevel: 6, MinGear: 30,
		CX: 50556, CY: 46156, Half: 1000,
	},
	{
		ID: "elven-raider-east", Name: "Raider Camp East",
		Region: regionElven, MinLevel: 4, MaxLevel: 6, MinGear: 30,
		CX: 53600, CY: 45300, Half: 1100,
	},
	{
		ID: "elven-raider-south", Name: "Raider Trail South",
		Region: regionElven, MinLevel: 4, MaxLevel: 6, MinGear: 30,
		CX: 39500, CY: 53600, Half: 1100,
	},
	{
		ID: "elven-raider-vale", Name: "Raider Vale South",
		Region: regionElven, MinLevel: 4, MaxLevel: 6, MinGear: 30,
		CX: 48788, CY: 55700, Half: 1100,
	},
	// Band 5-7: the goblin and kaboo camps of the southeast.
	{
		ID: "elven-goblin-camp", Name: "Goblin Camp Southeast",
		Region: regionElven, MinLevel: 5, MaxLevel: 7, MinGear: 40,
		CX: 51707, CY: 50504, Half: 1100,
	},
	{
		ID: "elven-kaboo-camp-east", Name: "Kaboo Camp East",
		Region: regionElven, MinLevel: 5, MaxLevel: 7, MinGear: 40,
		CX: 54400, CY: 51200, Half: 1100,
	},
	// Band 7-8: the kaboo grunt woods of the south.
	{
		ID: "elven-grunt-forest", Name: "Kaboo Grunt Forest",
		Region: regionElven, MinLevel: 7, MaxLevel: 8, MinGear: 80,
		CX: 43074, CY: 56632, Half: 1200,
	},
	{
		ID: "elven-grunt-west", Name: "Kaboo Grunt Woods West",
		Region: regionElven, MinLevel: 7, MaxLevel: 8, MinGear: 80,
		CX: 38324, CY: 50214, Half: 1100,
	},
	// Band 8-10: the kaboo fighter and spore fungus woods.
	{
		ID: "elven-kaboo-fighter-woods", Name: "Kaboo Fighter Woods",
		Region: regionElven, MinLevel: 8, MaxLevel: 10, MinGear: 110,
		CX: 34769, CY: 51063, Half: 1100,
	},
	{
		ID: "elven-fighter-ridge", Name: "Kaboo Fighter Ridge",
		Region: regionElven, MinLevel: 8, MaxLevel: 10, MinGear: 110,
		CX: 38499, CY: 46233, Half: 1100,
	},
	{
		ID: "elven-fungus-woods", Name: "Fungus Woods Southwest",
		Region: regionElven, MinLevel: 8, MaxLevel: 10, MinGear: 110,
		CX: 35471, CY: 55288, Half: 1100,
	},
	// Band 9-12: the kaboo fighter lieutenant camps of the far west.
	{
		ID: "elven-lieutenant-woods", Name: "Lieutenant Woods West",
		Region: regionElven, MinLevel: 9, MaxLevel: 12, MinGear: 130,
		CX: 30917, CY: 48818, Half: 1100,
	},
	{
		ID: "elven-lieutenant-camp", Name: "Lieutenant Camp West",
		Region: regionElven, MinLevel: 9, MaxLevel: 12, MinGear: 130,
		CX: 25442, CY: 41213, Half: 1100,
	},
	// Band 11-13: the fighter leader camps.
	{
		ID: "elven-leader-forest", Name: "Leader Forest West",
		Region: regionElven, MinLevel: 11, MaxLevel: 13, MinGear: 160,
		CX: 26115, CY: 50735, Half: 1100,
	},
	{
		ID: "elven-leader-south", Name: "Leader Downs South",
		Region: regionElven, MinLevel: 11, MaxLevel: 13, MinGear: 160,
		CX: 26697, CY: 63398, Half: 1100,
	},
	// Band 12-14: the dryad elder woods.
	{
		ID: "elven-elder-forest", Name: "Elder Forest West",
		Region: regionElven, MinLevel: 12, MaxLevel: 14, MinGear: 200,
		CX: 17134, CY: 45435, Half: 1200,
	},
	{
		ID: "elven-elder-woods", Name: "Elder Woods Deep West",
		Region: regionElven, MinLevel: 12, MaxLevel: 14, MinGear: 200,
		CX: 16232, CY: 52625, Half: 1200,
	},
	// Band 13-16: the dryad elder and spider forest.
	{
		ID: "elven-spider-forest", Name: "Spider Forest West",
		Region: regionElven, MinLevel: 13, MaxLevel: 16, MinGear: 230,
		CX: 9730, CY: 49233, Half: 1100,
	},
	{
		ID: "elven-spider-hills", Name: "Spider Hills West",
		Region: regionElven, MinLevel: 13, MaxLevel: 16, MinGear: 230,
		CX: 11150, CY: 56050, Half: 1100,
	},
	// Band 16-19: the lirein and pincer spider grounds of the deep
	// southwest.
	{
		ID: "elven-lirein-woods", Name: "Lirein Woods Deep West",
		Region: regionElven, MinLevel: 16, MaxLevel: 19, MinGear: 300,
		CX: 6566, CY: 59847, Half: 1100,
	},
	{
		ID: "elven-pincer-forest", Name: "Pincer Forest South",
		Region: regionElven, MinLevel: 16, MaxLevel: 19, MinGear: 300,
		CX: 9618, CY: 63158, Half: 1100,
	},
}

// ElvenHuntingZones returns the hunting grounds of the elven lands.
func ElvenHuntingZones() []HuntingZone {
	zones := make([]HuntingZone, len(elvenHuntingZones))
	copy(zones, elvenHuntingZones)

	return zones
}

// zoneSwitchPeriod bounds the automatic zone re-evaluation: the level
// changes and the gear upgrades (the town trips and the auto
// equipment) move the character up the ladder within this period.
const zoneSwitchPeriod = 30 * time.Second

// zoneRotateAfter bounds the patience of a hunter in a cleared-out
// square: the square holds no attackable mob this long after the
// character reached its middle, and the nearest ground of the same
// band is closer than waiting out the respawn of this one (the
// respawn of the elven lands runs at 15-20 s per mob).
const zoneRotateAfter = 10 * time.Second

// zoneDeathLimit is the death count that demotes a hunting ground: a
// character that died this often in one square does not pull the
// zone, and the band ladder caps below its band (the regression onto
// an easier ground) until the next level change re-opens it.
const zoneDeathLimit = 3

// zoneOverrideSlack is the level slack a manual zone selection keeps
// its override: the automatic picker resumes once the character
// outgrows the band.
const zoneOverrideSlack = 3

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
// level, the gear points and the position: the highest level band
// both gates allow, so the gear gate holds the character back until
// its equipment pays for the stronger mobs. Among the zones of the
// winning band the current one keeps its post (a periodic re-pick
// never bounces the character between the grounds of one band), the
// nearest one to the character wins an open contest. maxMinLevel
// caps the ladder from above for the death regression (zones whose
// band passes the cap are too hard for now, a negative value
// disables the cap). The starter zone (the first of the list) is the
// fallback for levels below every band.
func PickHuntingZone(
	zones []HuntingZone, level int32, gearPoints int32,
	currentID string, fromX int32, fromY int32, maxMinLevel int32,
) (HuntingZone, bool) {
	best := -1
	for index := range zones {
		candidate := zones[index]
		if level < candidate.MinLevel ||
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
		// Below every band or the cap closed the ladder: the first
		// zone of the region (the starter ground).
		if len(zones) > 0 {
			return zones[0], true
		}

		return HuntingZone{
			ID: "", Name: "", Region: "", MinLevel: 0, MaxLevel: 0,
			MinGear: 0, CX: 0, CY: 0, Half: 0,
		}, false
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

// SetHuntingZones installs the zone registry of the deployment: the
// loop picks the zone for the character state on the next tick and
// re-evaluates it as the level and the gear grow. Without a registry
// the loop hunts with the plain SetHuntingZone square (or without a
// zone at all).
func (l *Loop) SetHuntingZones(zones []HuntingZone) {
	l.zones = zones
	l.zoneOverride = -1
	l.zonePickedID = ""
	l.zoneCheckAt = time.Time{}
	l.zoneDeaths = nil
	l.zoneDeathCap = -1
	l.zoneDeathLevel = 0
	l.zoneEmptySince = time.Time{}
	l.publishZoneView()
}

// SetHuntingZoneRegion installs the registry of one region by name
// ("elven"): the map of the future deployments.
func (l *Loop) SetHuntingZoneRegion(region string) {
	switch region {
	case regionElven, "":
		l.SetHuntingZones(ElvenHuntingZones())
	default:
		l.logger.Printf("Hunt: no zone registry for region %q, hunting "+
			"without zones", region)
	}
}

// userZoneSelect applies the manual zone selection of the web UI: the
// index refers to the zone registry order. The selection overrides
// the automatic picker until the character outgrows the band.
func (l *Loop) userZoneSelect(index int32) {
	if len(l.zones) == 0 {
		return
	}
	if index < 0 || int(index) >= len(l.zones) {
		l.logger.Printf("Hunt: zone index %d out of range", index)

		return
	}
	zone := l.zones[index]
	l.zoneOverride = int(index)
	l.logger.Printf("Hunt: user selected the hunting zone %s", zone.Name)
	l.applyHuntingZone(zone)
}

// maybeSwitchZone re-evaluates the automatic zone pick: a level gain
// or a gear upgrade moves the character up the ladder, a cleared-out
// square rotates to the next ground of its band, and the death
// bookkeeping resets when the level changes. The ladder re-pick only
// happens between the fights (no target, nobody attacks), so a
// running fight always finishes in the old square.
func (l *Loop) maybeSwitchZone() {
	if len(l.zones) == 0 {
		return
	}
	now := time.Now()
	l.resetZoneDeathState()
	if l.zoneOverride < 0 {
		l.maybeRotateEmptyZone(now)
	}
	if l.zonePickedID != "" && now.Sub(l.zoneCheckAt) < zoneSwitchPeriod {
		return
	}
	l.zoneCheckAt = now
	level := l.tracker.SelfLevel()
	if l.zoneOverride >= 0 {
		zone := l.zones[l.zoneOverride]
		if level <= zone.MaxLevel+zoneOverrideSlack {
			return
		}
		l.zoneOverride = -1
		l.logger.Printf("Hunt: outgrew the manual zone %s, resuming the "+
			"automatic picker", zone.Name)
	}
	fromX, fromY := l.selfZoneAnchor()
	zone, ok := PickHuntingZone(l.zones, level, l.gearPoints(),
		l.zonePickedID, fromX, fromY, l.zoneDeathCap)
	if !ok || zone.ID == l.zonePickedID {
		return
	}
	l.applyHuntingZone(zone)
	l.logger.Printf("Hunt: level %d with gear %d: hunting %s (levels "+
		"%d-%d)", level, l.gearPoints(), zone.Name, zone.MinLevel,
		zone.MaxLevel)
}

// selfZoneAnchor returns the anchor of the nearest zone tie break:
// the character position when known, the center of the current
// square otherwise (the zone of the last hunt is a better guess of
// the whereabouts than the origin of the world).
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
// square: the pack of a small square dies out faster than the respawn
// refills it when the kills outpace the spawner, so a hunter that
// stands in the middle of a mob-less square for the rotate window
// moves on to the nearest ground of the same band. The emptiness
// reading only counts while the character stands central (the tracker
// only knows the mobs the server showed it, and the patrol walk
// brings the character to the middle first); running fights, resting
// walks and town trips reset the timer - a rotation never abandons
// any of them.
// The linear guard chain is the emptiness protocol itself: every
// guard either resets or holds the empty timer, and splitting it
// would scatter that contract over helpers.
func (l *Loop) maybeRotateEmptyZone(now time.Time) { //nolint:cyclop
	zone := l.zone()
	if zone == nil || l.zonePickedID == "" {
		l.zoneEmptySince = time.Time{}

		return
	}
	if l.phase != phaseEngage || l.target != 0 || l.tripActive() ||
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
	if l.tracker.ZoneHasAttackable(zone) {
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
	next, ok := l.rotationZone(selfX, selfY)
	if !ok {
		return
	}
	current, currentOK := l.zoneByID(l.zonePickedID)
	l.applyHuntingZone(next)
	l.zoneCheckAt = now
	if currentOK {
		l.logger.Printf("Hunt: %s is cleared out, rotating to %s",
			current.Name, next.Name)

		return
	}
	l.logger.Printf("Hunt: the zone is cleared out, rotating to %s",
		next.Name)
}

// rotationZone picks the rotation target of a cleared-out square: the
// nearest zone of the same band (the same mob level window, so the
// rotation never changes the difficulty), never the current one. The
// death cap holds even here: a demoted band is done for now.
func (l *Loop) rotationZone(selfX int32, selfY int32) (HuntingZone, bool) {
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

// noteZoneDeath counts a hunting death against the zone the death
// happened in: the square that contains the death spot first (an
// emergency logout death lands on a fresh session before any zone
// pick - the death spot is the only truthful attribution), the picked
// zone of the loop as the fallback (a flight that ended between the
// squares still belongs to the ground it fled from). Past the death
// limit the zone outguns the character, the band ladder caps below
// the zone band (the regression onto an easier ground) and the next
// tick re-picks with the cap. The counting happens in
// recoverFromDeath; the deleveling deaths are the point of that phase
// and never count.
func (l *Loop) noteZoneDeath() {
	zone, ok := l.deathZone()
	if !ok {
		return
	}
	if l.zoneDeaths == nil {
		l.zoneDeaths = make(map[string]int32)
	}
	l.zoneDeaths[zone.ID]++
	deaths := l.zoneDeaths[zone.ID]
	if deaths < zoneDeathLimit {
		l.logger.Printf("Hunt: death %d of %d in %s",
			deaths, zoneDeathLimit, zone.Name)
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
	l.logger.Printf("Hunt: %d deaths in %s, the zone outguns the "+
		"character: regressing to an easier band (capped below level "+
		"%d) until the level grows", deaths, zone.Name, zone.MinLevel)
}

// deathZone resolves the zone a death counts against: the square
// that contains the death spot, the picked zone when the spot lies
// between the squares (a flight that died outside every ground).
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

// resetZoneDeathState clears the zone death bookkeeping when the
// character level changes (a level up the ladder or a level down the
// deleveling): the level is the measure of what the character pulls,
// so every level change re-opens the demoted bands for a retry.
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
		l.logger.Printf("Hunt: level %d: the zone death bookkeeping "+
			"resets", level)
	}
}

// applyHuntingZone switches the hunting square of the loop and the
// tracker and publishes the zone view of the map. The remembered
// farm spot belongs to the previous square: the switch drops it, so
// the returns aim at the new center until the hunt remembers a
// fresh spot inside it. The rotation timer starts over: the new
// square gets its own empty window before any rotation away from it.
func (l *Loop) applyHuntingZone(zone HuntingZone) {
	l.zonePickedID = zone.ID
	l.zoneCX, l.zoneCY, l.zoneHalf = zone.CX, zone.CY, zone.Half
	l.farmX, l.farmY, l.farmZ = 0, 0, 0
	l.zoneEmptySince = time.Time{}
	l.tracker.SetHuntingZone(zone.CX, zone.CY, zone.Half)
	l.publishZoneView()
}

// publishZoneView pushes the zone registry with the active marker to
// the tracker snapshot: the map draws every zone and highlights the
// one the bot hunts in (or walks to), the death counts and the
// demoted bands of the regression travel along for the zone panel.
func (l *Loop) publishZoneView() {
	if l.tracker == nil {
		return
	}
	views := make([]state.ZoneView, 0, len(l.zones))
	for _, zone := range l.zones {
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
