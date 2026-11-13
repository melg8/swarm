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

// The cell policy: the decision core of the hexagon cell hunting. The hunter
// holds one hexagon, reads the knownlist emptiness through the level window,
// predicts the respawns of its kills and chooses between waiting, drifting to
// the predicted respawn and rotating to a neighbor ring. The far relocation
// fires only when the level window empties the neighborhood; the partition and
// its economy: docs/hunting_cells.md.

// The decision constants of the cell policy.
const (
    // cellWaitPatience is the respawn window worth waiting out: the elven
    // respawn runs 15-20 s per mob, so a prediction inside it beats every walk.
    cellWaitPatience = 20 * time.Second
    // cellNoDataPatience bounds the wait of a cell with no respawn predictions
    // (a fresh session, kills of other hunters): two respawn windows.
    cellNoDataPatience = 40 * time.Second
    // cellExhaustedPatience is the emptiness duration that marks a cell as
    // starved: nothing pickable for a full minute.
    cellExhaustedPatience = 60 * time.Second
    // cellMinStay bounds the voluntary economic switch: a better
    // cell only wins after the current ground got a fair trial.
    cellMinStay = 5 * time.Minute
    // cellExhaustedMinStay bounds the starved switch: a dead ground may be
    // left early, but not instantly (the patrol must settle first).
    cellExhaustedMinStay = 90 * time.Second
    // cellSwitchMargin is the hysteresis of the voluntary switch: the
    // alternative must score 25 percent above the current ground.
    cellSwitchMargin = 1.25
    // cellStarveCooldown holds a starved ground out of the contest:
    // the sweep moves forward instead of walking back into it.
    cellStarveCooldown = 5 * time.Minute
    // cellDeathRegressionWindow is how long the recorded death
    // regression refuses the ground the login landed on.
    cellDeathRegressionWindow = 15 * time.Minute
    // cellWindowFloorSlack is the level distance below the character that
    // still pays full adena (mob level + 8, the C1 drop penalty edge).
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
    // cellMapKillCap bounds the persistent map kill log: the marks
    // never expire by time, only the oldest drop past the cap.
    cellMapKillCap = 400
    // cellMaxLevelSlack mirrors the engage ceiling: the hard guard
    // above the character level (targetMaxLevelSlack).
    cellMaxLevelSlack = targetMaxLevelSlack
    // huntMeshVersion identifies the registry payload the web client caches:
    // the version changes only when the registry does (a region switch).
    huntMeshVersion = "elven-hexes-1"
    // cellFollowPeriod paces the follow-ground switch, so a boundary
    // fight never ping-pongs the registry.
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
    // pin names a cell the economy may never leave (SetCellPin): a
    // pinned hunter never rotates off it. Empty means the free
    // economy.
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
    // mapKills is the persistent kill log the web map draws (the fleet
    // wide death statistics): every kill appends, nothing expires by
    // time.
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
    // clearedAt is the ripeness clock of every ground the hunter
    // emptied: a cell is ripe again at clearedAt + its respawn
    // window.
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
        hunter.leashes[index] = cells[index].TargetZone()
    }
    hunter.metrics = make([]cellMetric, len(cells))
    hunter.aggro = make([]float64, len(cells))
    for index := range cells {
        hunter.aggro[index] = cellAggroMass(cells[index])
    }

    return hunter
}

// SetHuntingCells installs the cell registry: the loop hunts the cell mode
// from the next tick. The mesh view publishes at once, so a bot that starts a
// trip before its first pick still shows the map.
func (l *Loop) SetHuntingCells(cells []Cell) {
    if len(cells) == 0 {
        return
    }
    l.cell = newCellHunter(cells, globalCellHub)
    if l.cellPin != "" {
        // A pin installed before the registry (SetCellPin) is inherited.
        l.cell.pin = l.cellPin
    }
    // The legacy zone state stands down: the loop's zone bookkeeping (the
    // picked id, the overrides, the death caps) mirrors the cell state.
    l.zones = nil
    l.zonePickedID = ""
    l.zoneDeaths = nil
    l.zoneDeathCap = -1
    l.zoneEmptySince = time.Time{}
    l.zoneEmptyUntil = nil
    l.zoneMobPriority = nil
    l.cell.publishMesh(l)
    l.cell.publishView(l, l.now())
}

// SetCellPin locks the hunt to one named cell (the acceptance fleet's per-slot
// ground lock): the pin outranks the standing handoff and the economy pick; an
// unknown or level-ineligible pin degrades to the free economy.
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
// maybeSwitchZone): it tracks the level changes, accumulates the metrics,
// republishes the live view and runs the wait-or-rotate economy.
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
            // The stats have not arrived yet: the first pick waits for them.
            return
        }
        if hunter.applyPinnedCell(l, level, now) {
            return
        }
        skip := -1
        if x, y, _, ok := l.tracker.SelfPosition(); ok {
            hunter.selfX, hunter.selfY, hunter.selfKnown = x, y, true
            if ground, onGround := hunter.standingGround(x, y); onGround {
                // The relogin handoff: a character that enters the world ON a
                // hunting cell resumes it unless the death regression refuses.
                cell := hunter.cells[ground]
                if age, deadly := l.deathRegression(cell.ID, now); deadly {
                    l.logger.Printf("Hunt: level %d: not resuming "+
                        "the cell %s - the death regression stands "+
                        "(%.0fs ago)", level, cell.Name,
                        age.Seconds())
                    skip = ground
                } else {
                    hunter.apply(l, ground, now)
                    l.logger.Printf("Hunt: level %d: resuming the "+
                        "cell %s - the login landed on its ground",
                        level, cell.Name)

                    return
                }
            }
        }
        best, _ := hunter.pickGlobal(skip, now)
        if best < 0 {
            // The registry holds no alternative (a single cell or a window
            // that left only the deadly ground): standing beats nowhere.
            best, _ = hunter.pickGlobal(-1, now)
        }
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

// applyPinnedCell serves the ground lock on the first evaluation: the pin
// outranks both the standing handoff and the economy pick.
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
// position falls into (-1 outside every ground).
func (h *cellHunter) groundOf(x int32, y int32) int {
    for index := range h.leashes {
        if h.leashes[index].Contains(x, y) {
            return index
        }
    }

    return -1
}

// followGround moves the held hexagon onto the ground the character actually
// fights on (the target position while a fight runs, the character position
// otherwise), within the level window, the starvation cooldown and the pacing.
func (h *cellHunter) followGround(l *Loop, now time.Time) {
    if h.pin != "" {
        // The ground lock: the held cell never follows a fight off the pin.
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
    if _, deadly := l.deathRegression(h.cells[ground].ID, now); deadly {
        // The character stands on the ground its last death regressed
        // away from: following it would undo the regression.
        h.followAt = now
        l.logger.Printf("Hunt: not following the standing ground "+
            "%s - the death regression stands", h.cells[ground].Name)

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
    h.applyFollowing(l, ground, now)
    l.logger.Printf("Hunt: the fight crossed into %s (was %s), "+
        "following the ground", h.cells[ground].Name, left)
}

// standingGround resolves the hunting cell the character stands on after
// a fresh login: the exact grid assignment inside the level window (a
// login between the cells falls through to the scored pick).
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
// the claim dies with the loop otherwise, and every ghost hunter
// halved the score of its ground.
func (h *cellHunter) releaseClaim() {
    if h.leave != nil {
        h.leave()
        h.leave = nil
    }
}

// windowFloor returns the engage level floor of the cell mode: the character
// level minus the adena edge slack (zero while the level is unknown).
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

// markCleared starts the ripeness clock of a ground the hunter leaves empty:
// the picker holds off until the respawn window refills it. Expired entries
// leave on the way (the map must not grow forever).
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

// ripe reports whether the ground holds its expected population: a never
// cleared cell is ripe, a cell cleared at T ripens at T + its respawn window.
func (h *cellHunter) ripe(cell Cell, now time.Time) bool {
    cleared, ok := h.clearedAt[cell.ID]
    if !ok {
        return true
    }

    return now.Sub(cleared) >= respawnDuration(cell.RespawnMax)
}

// ripenessFactor returns the score fraction of the cell: the full value while
// ripe, the unripe discount while the respawn window refills the ground.
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

// markStarved starts the anomaly cooldown of a ground the hunter leaves empty
// even though it should have ripened: the sweep never walks straight back into
// it. Expired entries leave on the way.
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
// registry; the exclude index and the starved grounds stay out.
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
        // The level window holds no ground (or every ground cools down): the
        // closest mob level distance wins, so any level still hunts something.
        return h.pickClosestLevel(now), 0
    }

    return best, bestScore
}

// pickClosestLevel is the level-distance fallback of the global
// contest: the cell whose mob levels sit nearest the character.
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

// pickRing resolves the best RIPE candidate of the neighbor ring of the held
// cell: ring 1 is the direct Voronoi adjacency, ring 2 the second hop.
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
            // The unripe gate: the rotation never enters a refilling ground.
            continue
        }
        if score := h.cellScore(index, now); score > bestScore {
            best, bestScore = index, score
        }
    }

    return best, bestScore
}

// ringCells collects the registry indices of the neighbor ring: ring 1 the
// direct adjacency, ring 2 the second hop minus the inner ring and the held.
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

// ringEligible reports whether the neighbor ring holds any level eligible
// ground at all (the ripeness plays no role): an eligible but unripe ring
// refills on its own clock, so the hunter waits instead of relocating far.
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

// apply switches the hunter to a cell: the patrol square and the polygon leash
// retarget, the fleet claim moves, the visit metrics of the old ground fold in.
func (h *cellHunter) apply(l *Loop, index int, now time.Time) {
    l.dropAttackTarget("the ground rotation owns the way")
    h.applyGround(l, index, now)
}

// applyFollowing switches onto the ground a RUNNING fight stands on:
// bookkeeping only, the fight target stays selected.
func (h *cellHunter) applyFollowing(l *Loop, index int, now time.Time) {
    h.applyGround(l, index, now)
}

// applyGround moves the cell economy state to the given ground: the shared body
// of apply and applyFollowing, which differ only in the fight ownership.
func (h *cellHunter) applyGround(l *Loop, index int, now time.Time) {
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
    h.leash = cell.TargetZone()
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

// cellNoteKill records a kill of the overlay: the corpse position predicts
// where the mob returns and the species respawn window bounds when. The kill
// attributes to the hexagon the corpse lies on.
func (l *Loop) cellNoteKill(objectID int32, now time.Time) {
    h := l.cell
    if h == nil || h.picked < 0 {
        return
    }
    x, y, _, ok := l.tracker.ObjectPosition(objectID)
    if !ok {
        // The corpse vanished from the knownlist: it died at the feet.
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
        // The generated npc dictionary resolves the display name of the
        // species from the template id.
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
    // The persistent map kill log rides along: the mark never expires
    // by time, only the cap drops the oldest.
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

// deathRegression reports whether the cell carries a fresh death regression
// recorded by the tracker; a lapsed record reopens the ground. The age feeds
// the caller's log.
func (l *Loop) deathRegression(
    cellID string, now time.Time,
) (time.Duration, bool) {
    deadly, at := l.tracker.DeathRegression()
    if deadly == "" || deadly != cellID {
        return 0, false
    }
    age := now.Sub(at)
    if age >= cellDeathRegressionWindow {
        return 0, false
    }

    return age, true
}

// cellNoteDeath counts a hunting death against the current cell: the
// heat rises and the regression re-pick fires right away.
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
    // The record survives the session: a fresh loop would resume the ground.
    l.tracker.SetDeathRegression(cell.ID)
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

// accumulate advances the visit metrics: the active time of the stay (town
// trips excluded) and the adena delta as the income.
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

// waitOrRotate is the economy between the fights: a pickable mob anywhere in
// the knownlist wins, a predicted respawn inside the patience window beats
// every walk, and only an empty knownlist lets the economy wait or rotate. The
// reading runs unfenced and only in a healthy standing engage phase.
func (h *cellHunter) waitOrRotate(l *Loop, now time.Time) {
    if h.pin != "" {
        // The ground lock: the pinned cell is held through its empty
        // stretches and the starve switch never fires.
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
        h.windowFloor(), l.maxTargetLevel(), l.activeSkips(now),
        l.avoidBowTargets()) {
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
        // The respawn comes back before any walk could reach an equivalent
        // ground: hold and drift toward the predicted corpse position.
        h.waitWalk(l, predX, predY, selfX, selfY, selfZ, now)

        return
    }
    if !pending && waited < cellNoDataPatience {
        // No kill records: give the respawn window two chances.
        return
    }
    h.evaluateRotation(l, &cell, now, waited, pending, eta)
}

// waitWalk drifts the waiting hunter toward a predicted respawn position: one
// paced short segment (the target search picks up any mob it passes),
// suppressed while the character already stands on the point.
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
    moveX, moveY, _ := walkClickSegment(selfX, selfY, selfZ, float64(x),
        float64(y), float64(selfZ), returnWalkSegment)
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logger.Printf("Hunt: respawn drift walk failed: %v", err)
    }
}

// evaluateRotation decides the ground change of an emptied cell: the starved
// path after a full minute without a pickable mob, the economic path on a ripe
// neighbor beyond the hysteresis margin after the minimum stay.
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
        // No ripe neighbor and no starvation: the ground refills on its own
        // clock, the wait continues (an unripe ring ripens within the window).
        return
    }
    if best < 0 {
        // The local sweep found nothing: either the neighborhood is all unripe
        // (the wait continues) or the level window left the ground (relocate).
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
    // The emptied ground starts its ripeness clock.
    h.markCleared(cell.ID, now)
    if starved {
        // The left ground keeps its starvation cooldown: the next starved sweep
        // moves past it instead of walking straight back.
        h.markStarved(cell.ID, now)
    }
    h.apply(l, best, now)
}

// rotationCandidate resolves the best ripe neighbor of the held cell: the
// 1-hop ring first, the 2-hop ring next; ring 0 marks the global relocation.
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

// logRotation prints the decision of the ground change: the starved paths name
// the waited emptiness and the respawn ETA, the economic path the score race.
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

// etaRound renders the pending respawn ETA for the starved logs.
func etaRound(pending bool, eta time.Duration) string {
    if !pending {
        return "unknown"
    }

    return eta.Round(time.Second).String()
}

// minTargetLevel returns the engage level floor of the loop: the cell window
// floor in the cell mode, zero (disabled) in the legacy zone mode.
func (l *Loop) minTargetLevel() int32 {
    if l.cell == nil {
        return 0
    }

    return l.cell.windowFloor()
}

// The respawn overlay: the kill records with predicted respawn times.

// killRecord is one kill of the overlay: the corpse position, the
// victim the map tooltip shows and the predicted respawn time.
type killRecord struct {
    cellID    string
    x         int32
    y         int32
    at        time.Time
    respawnAt time.Time
    name      string
    level     int32
}

// pruneKills drops the overlay records the world already invalidated (the TTL
// passed: the mob either respawned and died again or never came back).
func (h *cellHunter) pruneKills(now time.Time) {
    kept := h.kills[:0]
    for index := range h.kills {
        if now.Sub(h.kills[index].at) <= cellKillTTL {
            kept = append(kept, h.kills[index])
        }
    }
    h.kills = kept
}

// recordMapKill appends one kill to the persistent map log and holds the cap:
// the marks never expire by time, only the oldest drop past cellMapKillCap.
func (h *cellHunter) recordMapKill(mark state.KillMarkView) {
    h.mapKills = append(h.mapKills, mark)
    if len(h.mapKills) > cellMapKillCap {
        h.mapKills = h.mapKills[len(h.mapKills)-cellMapKillCap:]
    }
}

// earliestPendingRespawn resolves the earliest unexpired prediction of a cell
// (a record whose respawn still lies in the future): the ETA, the predicted
// position and whether any prediction exists at all.
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
// the pending respawn predictions that land inside it.
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
