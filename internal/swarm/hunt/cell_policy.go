// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The cell policy: the decision core of the hexagon cell hunting.
// The hunter holds ONE hexagon (the convex polygon is the ground
// leash, the inscribed patrol square keeps the character anchored),
// reads the emptiness of the KNOWNLIST through the level window (the
// free-roam hunt fights the nearest visible enemy wherever it
// stands - a ground holds nothing worth staying for only when
// nothing at all is pickable in sight), predicts the respawns of
// the kills it made and decides between waiting (the respawn comes
// back before any walk could reach an equivalent ground), drifting
// to the predicted respawn position (corpse camping) and rotating
// to a NEIGHBOR hexagon (the hex grid ring: the moves stay local,
// the walks short). The rotation is paced by the respawn RIPENESS:
// a ground the hunter cleared at T is unripe until T + its respawn
// window, so the picker never walks back into a ground that holds
// nothing - the depletion/oversaturation cycle of the circle
// geometry cannot build (every spawn point of the ground belongs to
// exactly one hexagon, the uniform grid owns the whole respawn
// area). The far relocation fires only when the level window
// empties the whole neighborhood. The manual ground selection of
// the web UI is gone: the registry of a full project grows past a
// thousand cells, a hand-switched list is meaningless - the economy
// owns the rotation.

// The decision constants of the cell policy.
const (
    // cellWaitPatience is the respawn window worth waiting out at
    // the ground: the elven respawn runs 15-20 s per mob, so a
    // predicted respawn within this window beats every walk (the
    // walk to a neighbor costs the same time).
    cellWaitPatience = 20 * time.Second
    // cellNoDataPatience bounds the wait of a cell with NO respawn
    // predictions (a fresh session, kills of other hunters): the
    // window gets two chances before the economy takes over.
    cellNoDataPatience = 40 * time.Second
    // cellExhaustedPatience is the emptiness duration that marks a
    // cell as starved: nothing pickable for a full minute means the
    // ground cannot feed the character.
    cellExhaustedPatience = 60 * time.Second
    // cellMinStay bounds the voluntary economic switch: a better
    // cell only wins after the current ground got a fair trial.
    cellMinStay = 5 * time.Minute
    // cellExhaustedMinStay bounds the starved switch: a dead ground
    // may be left early, but not instantly (the emptiness reading
    // needs the patrol to settle first).
    cellExhaustedMinStay = 90 * time.Second
    // cellSwitchMargin is the hysteresis of the voluntary switch:
    // the alternative must score 25 percent above the current
    // ground, so the hunter never ping-pongs between comparable
    // cells.
    cellSwitchMargin = 1.25
    // cellStarveCooldown holds a starved ground out of the contest:
    // the starved sweep moves forward through the mesh instead of
    // walking back into the empty ground. The ripeness clock paces
    // the healthy rotation (seconds); this cooldown is the anomaly
    // net for a ground that stays empty even when ripe (another
    // hunter's farm, a wiped spawn).
    cellStarveCooldown = 5 * time.Minute
    // cellWindowFloorSlack is the level distance below the character
    // that still pays full adena (mob level + 8, the C1 drop penalty
    // edge): mobs below the floor never enter a fight.
    cellWindowFloorSlack = 8
    // cellWaitWalkPeriod paces the drift toward a predicted respawn
    // position, cellWaitWalkMinDist suppresses it when the character
    // already stands on the respawn point.
    cellWaitWalkPeriod  = 2 * time.Second
    cellWaitWalkMinDist = 500.0
    // cellViewPeriod paces the live view republication: the respawn
    // ETA label ticks once per second.
    cellViewPeriod = time.Second
    // cellKillLogCap bounds the respawn overlay records, cellKillTTL
    // prunes the predictions the world already invalidated.
    cellKillLogCap = 96
    cellKillTTL    = 5 * time.Minute
    // cellMapKillCap bounds the persistent map kill log (the death
    // statistics layer of the web map): the marks never expire by
    // time - the oldest ones drop only when the log outgrows the cap
    // (issue #6: the kill read of the map is a long term collection
    // of the death places, not a five minute melt).
    cellMapKillCap = 400
    // cellMaxLevelSlack mirrors the engage ceiling: the hard guard
    // above the character level (targetMaxLevelSlack).
    cellMaxLevelSlack = targetMaxLevelSlack
    // huntMeshVersion identifies the registry payload the web client
    // caches: the version changes only when the registry changes (a
    // regeneration or a region switch), the client refetches then.
    huntMeshVersion = "elven-hexes-1"
    // cellFollowPeriod paces the follow-ground switch: the free-roam
    // hunt crosses the hexagon boundaries chasing the nearest visible
    // enemy, and the held hexagon tracks the actual fight ground - a
    // boundary fight must not ping-pong the registry, the switch
    // fires at most once per period.
    cellFollowPeriod = 10 * time.Second
)

// cellHunter is the cell mode state of one hunt loop: the held cell,
// the polygon leash, the respawn overlay of the kills, the per cell
// metrics of the session and the wait-or-rotate bookkeeping.
type cellHunter struct {
    // cells is the registry of the deployment, picked the index of
    // the held cell (-1 before the first pick).
    cells  []Cell
    picked int
    // pin names a cell the economy may never leave (the acceptance
    // fleet's per-slot ground lock, see SetCellPin): a pinned hunter
    // picks the pinned cell first and never rotates off it - the
    // followGround drift and the wait-or-rotate economy both stand
    // down while the pin holds. Empty means the free economy.
    pin string
    // leashes caches the convex polygon of every cell of the
    // registry (the ground scans resolve the containing hexagon
    // without rebuilding the vertex slices per lookup).
    leashes []*state.CellZone
    // hub shares the cell occupancy with the fleet, leave releases
    // the claim of the current cell.
    hub   *cellHub
    leave func()
    // leash is the convex polygon target zone of the held cell
    // (built once per switch, the scans filter through it).
    leash *state.CellZone
    // metrics carries the session experience per cell, aggro the
    // precomputed static danger share per cell.
    metrics []cellMetric
    aggro   []float64
    // followAt paces the follow-ground switch (the held hexagon
    // tracks the ground the fight actually runs on).
    followAt time.Time
    // enteredAt marks when the current stay began (the minimum stay
    // floors of the switch decisions).
    enteredAt time.Time
    // level is the character level the window state was built at.
    level int32
    // priority is the window biased engage map of the current cell
    // (mirrored into loop.zoneMobPriority).
    priority map[int32]int32
    // kills is the respawn overlay: the recent kill records with
    // their predicted respawn times.
    kills []killRecord
    // mapKills is the persistent kill log the web map draws (the
    // fleet wide death statistics): every kill appends and nothing
    // expires by time - the respawn predictions above keep their own
    // short lifecycle, the map marks outlive them (issue #6).
    mapKills []state.KillMarkView
    // lastAdena and adenaKnown baseline the income attribution,
    // lastAccumAt paces the active time accumulation.
    lastAdena   int32
    adenaKnown  bool
    lastAccumAt time.Time
    // selfX, selfY and selfKnown cache the character position of
    // the tick (the proximity discount of the scores).
    selfX     int32
    selfY     int32
    selfKnown bool
    // emptySince arms the wait-or-rotate economy: the moment the
    // cell ground last read empty through the level window.
    emptySince time.Time
    // lastWaitWalkAt paces the drift toward the predicted respawn
    // positions.
    lastWaitWalkAt time.Time
    // viewAt paces the live view republication, meshPublished marks
    // the one time mesh install of the registry.
    viewAt        time.Time
    meshPublished bool
    // starvedUntil holds the anomaly cooldown of every ground the
    // hunter starved on: cell id -> the moment it re-opens for the
    // switch contest.
    starvedUntil map[string]time.Time
    // clearedAt holds the ripeness clock of every ground the hunter
    // emptied: cell id -> the moment the ground read empty. A cell
    // is ripe again at clearedAt + its respawn window - the picker
    // never walks into an unripe cell, so the rotation follows the
    // respawn refill instead of outrunning it.
    clearedAt map[string]time.Time
}

// newCellHunter creates the cell mode state for a registry.
func newCellHunter(cells []Cell, hub *cellHub) *cellHunter {
    hunter := &cellHunter{ //nolint:exhaustruct_v5 // session fields start zero
        cells:   cells,
        picked:  -1,
        hub:     hub,
        leave:   nil,
        leashes: make([]*state.CellZone, len(cells)),
    }
    for index := range cells {
        hunter.leashes[index] = cells[index].targetZone()
    }
    hunter.metrics = make([]cellMetric, len(cells))
    hunter.aggro = make([]float64, len(cells))
    for index := range cells {
        hunter.aggro[index] = cellAggroMass(cells[index])
    }

    return hunter
}

// SetHuntingCells installs the cell registry of the deployment: the
// loop hunts the cell mode (the neighbor rotation paced by the
// respawn ripeness replaces the spot economy). The first pick
// happens on the next tick. The mesh view publishes IMMEDIATELY: a
// bot that starts a town trip, a delevel or a manual walk before its
// first pick would otherwise never run the picker (those phases
// consume every tick ahead of maybeSwitchZone) and the map of the
// web UI showed no mesh at all until the first hunt resumed.
func (l *Loop) SetHuntingCells(cells []Cell) {
    if len(cells) == 0 {
        return
    }
    l.cell = newCellHunter(cells, globalCellHub)
    if l.cellPin != "" {
        // A pin installed before the registry (see SetCellPin):
        // the fresh hunter inherits it.
        l.cell.pin = l.cellPin
    }
    // The legacy zone state stands down: the zone bookkeeping of
    // the loop (the picked id, the overrides, the death caps)
    // mirrors the cell state instead.
    l.zones = nil
    l.zonePickedID = ""
    l.zoneDeaths = nil
    l.zoneDeathCap = -1
    l.zoneEmptySince = time.Time{}
    l.zoneEmptyUntil = nil
    l.zoneMobPriority = nil
    l.cell.publishMesh(l)
    l.cell.publishView(l, time.Now())
}

// SetCellPin locks the hunt to one named cell (the acceptance
// fleet's per-slot ground lock): a pinned loop picks the pinned
// cell on its first evaluation and never rotates off it - the
// five fleet slots stay on their five different grounds instead
// of converging onto the shared ripe cells (the round-12/13 crowd:
// three bots on one ground, the medians swinging with the mob
// contest). The pin outranks the standing handoff and the economy
// pick; an unknown or level-ineligible pin degrades to the free
// economy with the log naming it.
func (l *Loop) SetCellPin(cellID string) {
    l.cellPin = cellID
    if l.cell != nil {
        l.cell.pin = cellID
    }
}

// SetHuntingCellRegion installs the cell registry of one region by
// name ("elven"): the deployment wiring of the future regions.
func (l *Loop) SetHuntingCellRegion(region string) {
    switch region {
    case regionElven, "":
        l.SetHuntingCells(ElvenHuntingCells())
    default:
        l.logger.Printf("Hunt: no cell registry for region %q, "+
            "hunting without cells", region)
    }
}

// cellEvaluate is the tick entry of the cell mode (the branch of
// maybeSwitchZone): it tracks the level changes, accumulates the
// metrics, republishes the live view and runs the wait-or-rotate
// economy between the fights.
func (l *Loop) cellEvaluate(now time.Time) {
    hunter := l.cell
    if hunter == nil {
        return
    }
    level := l.tracker.SelfLevel()
    if level > 0 && level != hunter.level {
        hunter.level = level
        if hunter.picked >= 0 {
            hunter.priority = cellMobPriorities(
                hunter.cells[hunter.picked], level)
            l.zoneMobPriority = hunter.priority
        }
    }
    if hunter.picked < 0 {
        if level <= 0 {
            // The character stats have not arrived yet: the first
            // pick waits for the level.
            return
        }
        if hunter.applyPinnedCell(l, level, now) {
            return
        }
        if x, y, _, ok := l.tracker.SelfPosition(); ok {
            hunter.selfX, hunter.selfY, hunter.selfKnown = x, y, true
            if ground, onGround := hunter.standingGround(x, y); onGround {
                // The relogin handoff: a character that enters the
                // world ON a hunting cell resumes it instead of
                // re-contesting the economy - the position IS the
                // intent (the emergency logout left it where the
                // panic run ended, on the ground it had just walked
                // to).
                hunter.apply(l, ground, now)
                cell := hunter.cells[ground]
                l.logger.Printf("Hunt: level %d: resuming the "+
                    "cell %s - the login landed on its ground",
                    level, cell.Name)

                return
            }
        }
        best, _ := hunter.pickGlobal(-1, now)
        if best >= 0 {
            hunter.apply(l, best, now)
            cell := hunter.cells[best]
            l.logger.Printf("Hunt: level %d: holding the cell %s "+
                "(levels %d-%d, %d mobs, respawn %d-%ds, %d "+
                "neighbors)", level, cell.Name, cell.MinLevel,
                cell.MaxLevel, int(cell.Mass), cell.RespawnMin,
                cell.RespawnMax, len(cell.Neighbors))
        }

        return
    }
    hunter.readSelf(l)
    hunter.followGround(l, now)
    hunter.accumulate(l, now)
    hunter.publishView(l, now)
    hunter.waitOrRotate(l, now)
}

// applyPinnedCell serves the ground lock of SetCellPin on the
// first evaluation: the pin outranks both the standing handoff and
// the economy pick - the pin IS the intent. An unknown or
// level-ineligible pin degrades to the free economy with the log
// naming it. Reports whether the pin settled the pick.
func (h *cellHunter) applyPinnedCell(
    l *Loop, level int32, now time.Time,
) bool {
    if h.pin == "" {
        return false
    }
    index := h.indexOf(h.pin)
    if index < 0 {
        l.logger.Printf("Hunt: the pinned cell %s is not "+
            "in the registry, hunting the free economy", h.pin)

        return false
    }
    if !cellEligible(h.cells[index], level) {
        l.logger.Printf("Hunt: the pinned cell %s is "+
            "outside the level %d window, hunting the "+
            "free economy", h.pin, level)

        return false
    }
    h.apply(l, index, now)
    l.logger.Printf("Hunt: level %d: holding the "+
        "pinned cell %s (the ground lock)",
        level, h.cells[index].Name)

    return true
}

// indexOf resolves the registry index of a cell id (-1 unknown).
func (h *cellHunter) indexOf(cellID string) int {
    for index := range h.cells {
        if h.cells[index].ID == cellID {
            return index
        }
    }

    return -1
}

// readSelf caches the character position of the tick.
func (h *cellHunter) readSelf(l *Loop) {
    if x, y, _, ok := l.tracker.SelfPosition(); ok {
        h.selfX, h.selfY, h.selfKnown = x, y, true
    }
}

// groundOf resolves the registry index of the hexagon a world
// position falls into (-1 outside every ground): the free-roam hunt
// attributes the kills and the held ground by WHERE the action
// happens, not by the stale held index. The scan over the convex
// hexagons costs one containment test per cell - cheap at the call
// cadence (a kill note, a paced follow check).
func (h *cellHunter) groundOf(x int32, y int32) int {
    for index := range h.leashes {
        if h.leashes[index].Contains(x, y) {
            return index
        }
    }

    return -1
}

// followGround moves the held hexagon onto the ground the character
// actually fights on: the free-roam hunt crosses the hexagon
// boundaries chasing the nearest visible enemy, and the map
// highlight, the metrics and the occupancy must track where the
// fight really runs - not the ground the picker held when the roam
// started. The fight target position drives the switch while a
// fight runs (the mob stands in the hexagon the fight belongs to,
// even when the character itself still straddles the boundary), the
// character position otherwise. The switch respects the level
// window (an outgrown ground never becomes the held one), the
// starvation cooldown and the pacing floor - a boundary fight never
// ping-pongs the registry. Every departure starts the ripeness
// clock of the left ground, so the rotation returns exactly when
// its respawn refilled it.
func (h *cellHunter) followGround(l *Loop, now time.Time) {
    if h.pin != "" {
        // The ground lock (see SetCellPin): the held cell never
        // follows the free-roam fight off the pinned ground.
        return
    }
    if h.picked < 0 || !h.selfKnown {
        return
    }
    if !h.followAt.IsZero() && now.Sub(h.followAt) < cellFollowPeriod {
        return
    }
    x, y := h.selfX, h.selfY
    if l.target != 0 {
        if tx, ty, _, ok := l.tracker.ObjectPosition(l.target); ok {
            x, y = tx, ty
        }
    }
    ground := h.groundOf(x, y)
    if ground < 0 || ground == h.picked {
        return
    }
    if !cellEligible(h.cells[ground], h.level) ||
        h.starveCoolingDown(h.cells[ground].ID, now) ||
        !cellDelevelSafe(h.cells[ground], h.level) {
        return
    }
    h.followAt = now
    left := h.cells[h.picked].Name
    h.foldVisit(h.picked)
    h.markCleared(h.cells[h.picked].ID, now)
    h.apply(l, ground, now)
    l.logger.Printf("Hunt: the fight crossed into %s (was %s), "+
        "following the ground", h.cells[ground].Name, left)
}

// standingGround resolves the hunting cell the character stands on
// after a fresh login: the relogin handoff of the cell economy. The
// ground counts as standing ground while the position falls inside
// the cell polygon (the exact grid assignment - the cell OWNS the
// ground the character stands on) and the cell stays inside the
// level window of the character (an outgrown ground never resumes).
// A character that logs in between the cells (a relogin mid-walk)
// falls through to the scored pick.
func (h *cellHunter) standingGround(x int32, y int32) (int, bool) {
    ground := h.groundOf(x, y)
    if ground < 0 {
        return -1, false
    }
    if !cellEligible(h.cells[ground], h.level) {
        return -1, false
    }

    return ground, true
}

// releaseClaim drops the fleet occupancy claim of the current cell:
// the loop goroutine owns the claim, and the session end (the
// emergency logout, a server restart, a lost connection) must hand
// it back - the 24/7 supervisor builds a fresh cellHunter on every
// relogin, and a claim that dies with the old loop would stay in the
// process wide hub forever. Every ghost hunter halved the score of
// its ground in the occupancy division, so a bot that relogged on
// the same cell over and over watched the picker walk its fresh
// sessions onto the neighbor grounds.
func (h *cellHunter) releaseClaim() {
    if h.leave != nil {
        h.leave()
        h.leave = nil
    }
}

// windowFloor returns the engage level floor of the cell mode: the
// character level minus the adena edge slack (zero while the level is
// unknown - the floor disables itself).
func (h *cellHunter) windowFloor() int32 {
    if h.level <= 0 {
        return 0
    }
    floor := h.level - cellWindowFloorSlack
    if floor < 1 {
        floor = 1
    }

    return floor
}

// markCleared starts the ripeness clock of a ground the hunter
// leaves empty: the respawn window refills it, the picker holds off
// until then. The expired entries of the map leave on the way (the
// registry holds hundreds of cells, the map must not grow forever).
func (h *cellHunter) markCleared(cellID string, now time.Time) {
    if h.clearedAt == nil {
        h.clearedAt = make(map[string]time.Time)
    }
    for id, at := range h.clearedAt {
        if now.Sub(at) > cellKillTTL {
            delete(h.clearedAt, id)
        }
    }
    h.clearedAt[cellID] = now
}

// ripe reports whether the ground holds its expected population at
// the given time: a cell never cleared is ripe (its static mass
// stands), a cell cleared at T ripens at T + its respawn window.
func (h *cellHunter) ripe(cell Cell, now time.Time) bool {
    cleared, ok := h.clearedAt[cell.ID]
    if !ok {
        return true
    }

    return now.Sub(cleared) >= respawnDuration(cell.RespawnMax)
}

// ripenessFactor returns the score fraction of the cell at the given
// time: the full value while ripe, the unripe discount while the
// respawn window refills the ground (the unripe gate of the rotation
// keeps the hunter away anyway - the discount prices the residual
// chance of the global contest).
func (h *cellHunter) ripenessFactor(index int, now time.Time) float64 {
    if h.ripe(h.cells[index], now) {
        return 1.0
    }

    return cellUnripeDiscount
}

// respawnDuration converts the respawn seconds into the duration.
func respawnDuration(seconds int32) time.Duration {
    return time.Duration(seconds) * time.Second
}

// markStarved starts the anomaly cooldown of a ground the hunter
// leaves empty even though it should have ripened: the sweep never
// walks straight back into it. The expired entries of the map leave
// on the way.
func (h *cellHunter) markStarved(cellID string, now time.Time) {
    if h.starvedUntil == nil {
        h.starvedUntil = make(map[string]time.Time)
    }
    for id, until := range h.starvedUntil {
        if now.After(until) {
            delete(h.starvedUntil, id)
        }
    }
    h.starvedUntil[cellID] = now.Add(cellStarveCooldown)
}

// starveCoolingDown reports whether the ground sits in its
// post-starvation cooldown at the given time.
func (h *cellHunter) starveCoolingDown(cellID string, now time.Time) bool {
    until, ok := h.starvedUntil[cellID]

    return ok && now.Before(until)
}

// pickGlobal resolves the best scoring eligible cell of the WHOLE
// registry (the relocation contest: the level window left the
// neighborhood, a death regression moves the hunter away). The
// exclude index keeps a ground out of the contest. Grounds cooling
// down from a starvation stay out too.
func (h *cellHunter) pickGlobal(exclude int, now time.Time) (int, float64) {
    best := -1
    bestScore := 0.0
    for index := range h.cells {
        if index == exclude {
            continue
        }
        if !cellEligible(h.cells[index], h.level) {
            continue
        }
        if h.starveCoolingDown(h.cells[index].ID, now) {
            continue
        }
        if !cellDelevelSafe(h.cells[index], h.level) {
            continue
        }
        if score := h.cellScore(index, now); score > bestScore {
            best, bestScore = index, score
        }
    }
    if best < 0 && exclude < 0 {
        // The level window holds no ground at all (or every ground
        // of the window cools down from a starvation): the closest
        // mob level distance wins - a character above or below every
        // window still hunts something.
        return h.pickClosestLevel(now), 0
    }

    return best, bestScore
}

// pickClosestLevel resolves the level-distance fallback of the
// global contest: the cell whose mob levels sit nearest the
// character level, the starve cooldowns respected while any ground
// stays open, ignored when everything cools down (a starved session
// still needs a ground to stand on).
func (h *cellHunter) pickClosestLevel(now time.Time) int {
    best := -1
    bestGap := int32(1 << 30)
    for index := range h.cells {
        if h.starveCoolingDown(h.cells[index].ID, now) {
            continue
        }
        gap := cellLevelDistance(h.cells[index], h.level)
        if gap < bestGap {
            bestGap, best = gap, index
        }
    }
    if best >= 0 {
        return best
    }
    for index := range h.cells {
        gap := cellLevelDistance(h.cells[index], h.level)
        if gap < bestGap {
            bestGap, best = gap, index
        }
    }

    return best
}

// pickRing resolves the best RIPE eligible candidate of the neighbor
// ring of the held cell: ring 1 is the direct Voronoi adjacency, ring
// 2 the neighbors of the neighbors. The local sweep: the rotation
// moves through the mesh, the walks stay inside the neighborhood
// (the mean Voronoi edge is a few hundred units, the whole ring sits
// inside a couple of thousand).
func (h *cellHunter) pickRing(ring int, now time.Time) (int, float64) {
    if h.picked < 0 {
        return -1, 0
    }
    candidates := h.ringCells(ring)
    best := -1
    bestScore := 0.0
    for _, candidate := range candidates {
        index := int(candidate)
        if !cellEligible(h.cells[index], h.level) {
            continue
        }
        if h.starveCoolingDown(h.cells[index].ID, now) {
            continue
        }
        if !cellDelevelSafe(h.cells[index], h.level) {
            continue
        }
        if !h.ripe(h.cells[index], now) {
            // The unripe gate: the rotation never walks into a
            // ground the respawn has not refilled yet.
            continue
        }
        if score := h.cellScore(index, now); score > bestScore {
            best, bestScore = index, score
        }
    }

    return best, bestScore
}

// ringCells collects the registry indices of the neighbor ring of
// the held cell: ring 1 the direct adjacency, ring 2 the neighbors
// of the neighbors minus the inner ring minus the held cell. The
// 1-hop adjacency is symmetric (the generator pins it), the 2-hop
// set deduplicates through the map.
func (h *cellHunter) ringCells(ring int) []int32 {
    if h.picked < 0 {
        return nil
    }
    held := h.cells[h.picked]
    if ring <= 1 {
        return held.Neighbors
    }
    seen := make(map[int32]bool, len(held.Neighbors)*4)
    for _, neighbor := range held.Neighbors {
        seen[neighbor] = true
    }
    seen[int32(h.picked)] = true
    var outer []int32
    for _, neighbor := range held.Neighbors {
        for _, second := range h.cells[neighbor].Neighbors {
            if seen[second] {
                continue
            }
            seen[second] = true
            outer = append(outer, second)
        }
    }

    return outer
}

// ringEligible reports whether the neighbor ring of the held cell
// holds any level-eligible ground at all (the ripeness plays no
// role): an eligible but unripe ring means the neighborhood refills
// on its own clock and the hunter WAITS instead of relocating far.
func (h *cellHunter) ringEligible(now time.Time) bool {
    if h.picked < 0 {
        return false
    }
    for ring := 1; ring <= 2; ring++ {
        for _, index := range h.ringCells(ring) {
            if !cellEligible(h.cells[index], h.level) {
                continue
            }
            if h.starveCoolingDown(h.cells[index].ID, now) {
                continue
            }

            return true
        }
    }

    return false
}

// apply switches the hunter to a cell: the patrol square and the
// polygon leash of the loop retarget, the fleet claim moves, the
// visit metrics of the old ground fold into the totals and the new
// visit begins. The route change owns the target register: the held
// fight of the old ground drops with its server selection (the owner
// rule of the travel target reset, see dropAttackTarget) - a no-op
// while the walk answers nothing.
func (h *cellHunter) apply(l *Loop, index int, now time.Time) {
    l.dropAttackTarget("the ground rotation owns the way")
    if h.picked >= 0 && h.picked < len(h.metrics) {
        h.foldVisit(h.picked)
    }
    if h.leave != nil {
        h.leave()
    }
    h.picked = index
    h.enteredAt = now
    cell := h.cells[index]
    h.leave = h.hub.enter(cell.ID)
    h.leash = cell.targetZone()
    l.zonePickedID = cell.ID
    l.zoneCX, l.zoneCY, l.zoneHalf = cell.FocusX, cell.FocusY, cell.PatrolHalf
    l.farmX, l.farmY, l.farmZ = 0, 0, 0
    l.zoneEmptySince = time.Time{}
    h.emptySince = time.Time{}
    h.priority = cellMobPriorities(cell, h.level)
    l.zoneMobPriority = h.priority
    l.tracker.SetHuntingZone(cell.FocusX, cell.FocusY, cell.PatrolHalf)
    // The income attribution re-baselines at the new ground.
    h.adenaKnown = false
    h.lastAccumAt = time.Time{}
    h.publishView(l, now)
}

// foldVisit closes the running stay of a cell: the live visit
// numbers fold into the totals of the ground.
func (h *cellHunter) foldVisit(index int) {
    metric := &h.metrics[index]
    metric.activeMin += metric.visitActiveMin
    metric.adena += metric.visitAdena
    metric.kills += metric.visitKills
    metric.visitActiveMin = 0
    metric.visitAdena = 0
    metric.visitKills = 0
}

// cellNoteKill records a kill of the overlay: the corpse position
// predicts where the mob returns (the server default respawns at the
// spawn point, EnableRandomMonsterSpawns = false keeps it near the
// death place), the respawn window of the species bounds WHEN. The
// record feeds the wait-or-rotate economy and the kill centroid EMA
// of the map view (the dynamic focus: the patrol drifts toward the
// ground the character actually farms). The kill attributes to the
// hexagon the CORPSE lies on - the free-roam hunt fights across the
// boundaries and the held index is not necessarily the ground the
// mob died on.
func (l *Loop) cellNoteKill(objectID int32, now time.Time) {
    h := l.cell
    if h == nil || h.picked < 0 {
        return
    }
    x, y, _, ok := l.tracker.ObjectPosition(objectID)
    if !ok {
        // The corpse vanished from the knownlist before the record:
        // it died at the character's feet.
        x, y = h.selfX, h.selfY
    }
    ground := h.groundOf(x, y)
    if ground < 0 {
        ground = h.picked
    }
    wire := l.tracker.ObjectTemplateID(objectID)
    rmin, rmax, found := cellMobRespawn(h.cells, wire)
    if !found {
        cell := h.cells[ground]
        rmin, rmax = cell.RespawnMin, cell.RespawnMax
    }
    mid := time.Duration(rmin+rmax) * time.Second / 2
    cell := h.cells[ground]
    name := l.tracker.ObjectName(objectID)
    if name == "" && wire != 0 {
        // The corpse vanished from the knownlist before the record:
        // the generated npc dictionary resolves the display name of
        // the species from the template id.
        name = npcdata.NPCName(wire + 1000000)
    }
    level, _ := l.tracker.ObjectLevel(objectID)
    h.kills = append(h.kills, killRecord{
        cellID: cell.ID, x: x, y: y, at: now,
        respawnAt: now.Add(mid),
        name:      name, level: level,
    })
    h.pruneKills(now)
    if len(h.kills) > cellKillLogCap {
        h.kills = h.kills[len(h.kills)-cellKillLogCap:]
    }
    // The persistent map kill log rides along: the same record the
    // respawn overlay holds, minus the lifecycle - the mark never
    // expires by time, only the cap drops the oldest ones (issue #6:
    // the map answers "where did the fleet kill everything" for the
    // whole session, not for the last five minutes).
    h.recordMapKill(
        //nolint:exhaustruct_v5 // BotID stays 0: the registry stamps
        // the owning bot at the merge (Registry.FleetKillMarks).
        state.KillMarkView{
            X: x, Y: y, AtMs: now.UnixMilli(),
            Name: name, Level: level,
        })
    metric := &h.metrics[ground]
    metric.visitKills++
    if !metric.killPosKnown {
        metric.killX, metric.killY = float64(x), float64(y)
        metric.killPosKnown = true
    } else {
        metric.killX = metric.killX*0.8 + float64(x)*0.2
        metric.killY = metric.killY*0.8 + float64(y)*0.2
    }
}

// cellNoteDeath counts a hunting death against the current cell: the
// death heat rises (the safety score of the ground decays with it,
// per cell - never the whole band), and the regression re-pick fires
// right away: a fresh death usually means the ground outguns the
// character, the walk back to the village spawn should aim at an
// easier cell instead of the corpse ground. The regression contest
// is global (the escape from the danger): a deadly neighbor of a
// deadly cell must not win by adjacency.
func (l *Loop) cellNoteDeath(now time.Time) {
    h := l.cell
    if h == nil || h.picked < 0 {
        return
    }
    cell := h.cells[h.picked]
    metric := &h.metrics[h.picked]
    metric.deaths++
    metric.deathCount++
    metric.lastDeathAt = now
    l.logger.Printf("Hunt: death at the cell %s (heat %.1f)", cell.Name,
        metric.deaths)
    // The fresh heat crushed the score of the ground: re-pick when
    // any eligible alternative now outranks it.
    current := h.cellScore(h.picked, now)
    best, bestScore := h.pickGlobal(h.picked, now)
    if best >= 0 && best != h.picked && bestScore > current {
        h.foldVisit(h.picked)
        h.apply(l, best, now)
        l.logger.Printf("Hunt: regressing from %s to %s after the "+
            "death", cell.Name, h.cells[best].Name)
    }
    h.publishView(l, now)
}

// accumulate advances the visit metrics: the active time of the stay
// (town trips excluded - the walks to the vendor are not the
// ground's cost) and the adena delta of the inventory attributed as
// the income of the ground (rest included: hard fights internalize
// their own cost, the rate is the honest income per hunting minute).
func (h *cellHunter) accumulate(l *Loop, now time.Time) {
    if l.tripActive() {
        h.lastAccumAt = time.Time{}

        return
    }
    if h.lastAccumAt.IsZero() {
        h.lastAccumAt = now
        h.lastAdena = l.tracker.InventoryStats().Adena
        h.adenaKnown = true

        return
    }
    deltaMin := now.Sub(h.lastAccumAt).Minutes()
    if deltaMin <= 0 {
        return
    }
    h.lastAccumAt = now
    metric := &h.metrics[h.picked]
    metric.visitActiveMin += deltaMin
    adena := l.tracker.InventoryStats().Adena
    if h.adenaKnown {
        if gained := int64(adena - h.lastAdena); gained > 0 {
            metric.visitAdena += gained
        }
    }
    h.lastAdena = adena
    h.adenaKnown = true
}

// waitOrRotate is the economy between the fights: a pickable mob
// visible ANYWHERE in the knownlist always wins (the engage takes
// it - the free-roam hunt does not fence the pick on the held
// hexagon), a predicted respawn within the patience window beats
// every walk (the hunter drifts to the corpse position), and only a
// knownlist that holds NOTHING pickable at all lets the economy
// decide between waiting out the respawn and rotating to the
// neighbor. The guards mirror the zone rotation: only a healthy,
// standing engage phase reads the emptiness at all - resting walks
// and town trips never rotate. A merely held target no longer resets
// the timer (the owner rule of the travel target reset): a stuck
// fight the ground cannot convert into a kill lets the window run,
// and the apply of the rotation drops it. The reading runs UNFENCED
// (the whole knownlist through the level window): a mob of the
// neighbor hexagon the character already sees is not emptiness, the
// bot walks and fights it - a ground counts as zero-enemy only when
// nothing at all is in sight, and moving there is the last resort of
// the economy.
func (h *cellHunter) waitOrRotate(l *Loop, now time.Time) {
    if h.pin != "" {
        // The ground lock (see SetCellPin): the rotation economy
        // stands down - the pinned cell is held through its empty
        // stretches (the respawn refills it) and the starve switch
        // never fires.
        return
    }
    if l.phase != phaseEngage || l.tripActive() ||
        l.tracker.SelfUnderAttack() || l.tracker.SelfSitting() {
        h.emptySince = time.Time{}

        return
    }
    cell := h.cells[h.picked]
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    if l.tracker.ZoneHasPickableWindowed(nil,
        h.windowFloor(), l.maxTargetLevel(), l.activeSkips(now)) {
        h.emptySince = time.Time{}

        return
    }
    if h.emptySince.IsZero() {
        h.emptySince = now

        return
    }
    waited := now.Sub(h.emptySince)
    eta, predX, predY, pending := h.earliestPendingRespawn(cell.ID, now)
    if pending && eta <= cellWaitPatience {
        // The respawn comes back before any walk could reach an
        // equivalent ground: hold the ground, drift toward the
        // predicted corpse position (one paced segment).
        h.waitWalk(l, predX, predY, selfX, selfY, selfZ, now)

        return
    }
    if !pending && waited < cellNoDataPatience {
        // No kill records of the ground: give the respawn window two
        // chances before the economy decides.
        return
    }
    h.evaluateRotation(l, &cell, now, waited, pending, eta)
}

// waitWalk drifts the waiting hunter toward a predicted respawn
// position: one paced short segment (the per second target search of the
// engage picks up any mob the segment comes past), suppressed while the
// character already stands on the respawn point.
func (h *cellHunter) waitWalk(
    l *Loop, x int32, y int32,
    selfX int32, selfY int32, selfZ int32, now time.Time,
) {
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    if dist < cellWaitWalkMinDist {
        return
    }
    if !h.lastWaitWalkAt.IsZero() &&
        now.Sub(h.lastWaitWalkAt) < cellWaitWalkPeriod {
        return
    }
    h.lastWaitWalkAt = now
    moveX, moveY := x, y
    dx := float64(x - selfX)
    dy := float64(y - selfY)
    if dist > returnWalkSegment {
        frac := returnWalkSegment / dist
        moveX = int32(float64(selfX) + dx*frac)
        moveY = int32(float64(selfY) + dy*frac)
    }
    if err := l.game.WalkTo(moveX, moveY, selfZ); err != nil {
        l.logger.Printf("Hunt: respawn drift walk failed: %v", err)
    }
}

// evaluateRotation decides the ground change of an emptied cell: the
// starved path (nothing pickable for a full minute - the ground
// cannot feed the character, any ripe neighbor wins after the short
// stay floor, the global contest follows when the whole neighborhood
// offers nothing) and the economic path (a ripe neighbor scores
// beyond the hysteresis margin after the fair trial of the minimum
// stay). A hunter without any alternative keeps waiting - the ground
// refills on its own clock. Every departure from an emptied ground
// starts its ripeness clock: the rotation returns exactly when the
// respawn refilled it.
func (h *cellHunter) evaluateRotation(
    l *Loop, cell *Cell, now time.Time,
    waited time.Duration, pending bool, eta time.Duration,
) {
    stayed := now.Sub(h.enteredAt)
    current := h.cellScore(h.picked, now)
    best, bestScore, ring := h.rotationCandidate(now)
    starved := waited >= cellExhaustedPatience &&
        stayed >= cellExhaustedMinStay &&
        h.expectedSupply(cell.ID, now, time.Minute) == 0
    if best < 0 && !starved {
        // No ripe neighbor and no starvation: the ground refills on
        // its own clock, the wait continues (an eligible unripe ring
        // ripens within the respawn window).
        return
    }
    if best < 0 {
        // The local sweep found nothing: either the neighborhood is
        // all unripe (the wait continues - it ripens within the
        // window) or the level window left the ground (the far
        // relocation).
        if !starved || h.ringEligible(now) {
            return
        }
        best, bestScore = h.pickGlobal(h.picked, now)
        if best < 0 {
            return
        }
        ring = 0
    }
    better := bestScore > current*cellSwitchMargin && stayed >= cellMinStay
    if !starved && !better {
        return
    }
    h.logRotation(l, cell, best, ring, current, bestScore,
        starved && !better, waited, pending, eta)
    // The emptied ground starts its ripeness clock: the rotation
    // paces itself on the respawn refill.
    h.markCleared(cell.ID, now)
    if starved {
        // The left ground keeps its starvation cooldown too: the
        // next starved sweep moves forward past it instead of
        // walking straight back into the empty ground.
        h.markStarved(cell.ID, now)
    }
    h.apply(l, best, now)
}

// rotationCandidate resolves the best ripe neighbor of the held
// cell: the 1-hop ring first (the local sweep), the 2-hop ring when
// the direct adjacency offers nothing. The ring marker (1, 2, 0)
// rides along for the log line; 0 marks the global relocation the
// caller falls back to.
func (h *cellHunter) rotationCandidate(
    now time.Time,
) (int, float64, int) {
    best, bestScore := h.pickRing(1, now)
    if best >= 0 {
        return best, bestScore, 1
    }
    best, bestScore = h.pickRing(2, now)
    if best >= 0 {
        return best, bestScore, 2
    }

    return -1, 0, 1
}

// logRotation prints the decision of the ground change: the starved
// paths name the waited emptiness and the respawn ETA, the economic
// path names the score race.
func (h *cellHunter) logRotation(
    l *Loop, cell *Cell, best int, ring int,
    current float64, bestScore float64, starvedOnly bool,
    waited time.Duration, pending bool, eta time.Duration,
) {
    if starvedOnly {
        target := h.cells[best]
        if ring > 0 {
            l.logger.Printf("Hunt: %s starved (waited %s, respawn eta "+
                "%s): rotating to the neighbor %s", cell.Name,
                waited.Round(time.Second),
                etaRound(pending, eta), target.Name)
        } else {
            l.logger.Printf("Hunt: %s starved (waited %s, respawn eta "+
                "%s): relocating to %s", cell.Name,
                waited.Round(time.Second),
                etaRound(pending, eta), target.Name)
        }

        return
    }
    l.logger.Printf("Hunt: %s outscored (%.0f vs %.0f adena/min "+
        "score): rotating to %s", cell.Name, bestScore,
        current, h.cells[best].Name)
}

// etaRound renders the pending respawn ETA for the logs (never for
// the starved message when no prediction exists).
func etaRound(pending bool, eta time.Duration) string {
    if !pending {
        return "unknown"
    }

    return eta.Round(time.Second).String()
}

// minTargetLevel returns the engage level floor of the loop: the
// cell window floor in the cell mode, zero (disabled) in the legacy
// zone mode.
func (l *Loop) minTargetLevel() int32 {
    if l.cell == nil {
        return 0
    }

    return l.cell.windowFloor()
}

// The respawn overlay of the cell hunting: the kill records with
// their predicted respawn times.

// killRecord is one kill of the overlay: the corpse position (the
// server default respawns the mob at its spawn point, the elven
// windows are short), the victim the map tooltip shows (the name and
// the level, captured while the corpse is still in the knownlist)
// and the species respawn window midpoint as the predicted time.
type killRecord struct {
    cellID    string
    x         int32
    y         int32
    at        time.Time
    respawnAt time.Time
    name      string
    level     int32
}

// pruneKills drops the overlay records the world already invalidated
// (the TTL passed long ago - the mob either respawned and died again
// or never came back where predicted).
func (h *cellHunter) pruneKills(now time.Time) {
    kept := h.kills[:0]
    for index := range h.kills {
        if now.Sub(h.kills[index].at) <= cellKillTTL {
            kept = append(kept, h.kills[index])
        }
    }
    h.kills = kept
}

// recordMapKill appends one kill to the persistent map log and holds
// the count cap: the marks never expire by time, the oldest ones drop
// only when the log outgrows cellMapKillCap (issue #6).
func (h *cellHunter) recordMapKill(mark state.KillMarkView) {
    h.mapKills = append(h.mapKills, mark)
    if len(h.mapKills) > cellMapKillCap {
        h.mapKills = h.mapKills[len(h.mapKills)-cellMapKillCap:]
    }
}

// earliestPendingRespawn resolves the earliest UNEXPIRED prediction
// of a cell: the kill records whose respawn still lies in the
// future. The ETA, the predicted position, and whether any
// prediction exists at all.
func (h *cellHunter) earliestPendingRespawn(
    cellID string, now time.Time,
) (time.Duration, int32, int32, bool) {
    var bestETA time.Duration
    bestX, bestY := int32(0), int32(0)
    found := false
    for index := range h.kills {
        kill := &h.kills[index]
        if kill.cellID != cellID || !kill.respawnAt.After(now) {
            continue
        }
        eta := kill.respawnAt.Sub(now)
        if !found || eta < bestETA {
            bestETA, bestX, bestY, found = eta, kill.x, kill.y, true
        }
    }

    return bestETA, bestX, bestY, found
}

// expectedSupply counts the mob supply of a cell within a horizon:
// the pending respawn predictions that land inside it. The live
// pickable population is the emptiness reading itself (the caller
// reached here with an empty ground), so the predictions are the
// only remaining supply.
func (h *cellHunter) expectedSupply(
    cellID string, now time.Time, horizon time.Duration,
) int {
    count := 0
    for index := range h.kills {
        kill := &h.kills[index]
        if kill.cellID != cellID || !kill.respawnAt.After(now) {
            continue
        }
        if kill.respawnAt.Sub(now) <= horizon {
            count++
        }
    }

    return count
}
