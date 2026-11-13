// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "sync/atomic"
    "time"
)

// DefaultMaxPassableHeight is the default climb between two neighbouring cells:
// the Mobius GeoEngine HEIGHT_INCREASE_LIMIT (40), the upward step gate the
// planned walks must pass.
const DefaultMaxPassableHeight = uint16(40)

// DefaultCacheCapacity bounds the parsed regions in memory: one multilayer
// region costs roughly 20 MB parsed, and the cache evicts the least recently
// used entry when it overflows.
const DefaultCacheCapacity = 4

// MaxSearchExpansions caps the node expansions of one search: an unreachable
// target in open terrain would otherwise explore the whole region grid (4.2M
// cells, tens of seconds). The cap stops the search and reports Result.Aborted;
// long real world paths stay two orders of magnitude below it.
const MaxSearchExpansions = 1000000

// ErrMissingCell reports that a requested start or target position has no
// geodata underneath (outside the shipped regions or no geodata directory).
var ErrMissingCell = errors.New("cell has no geodata")

// Stats is the engine state summary for diagnostics and the web UI.
type Stats struct {
    Dir           string `json:"dir"`
    RegionFiles   int    `json:"regionFiles"`
    LoadedRegions int    `json:"loadedRegions"`
    HasData       bool   `json:"hasData"`
    Center        Vec3   `json:"center"`
}

// Engine reads geodata region files lazily and answers cell layer queries; it
// is safe for concurrent use.
type Engine struct {
    dir      string
    capacity int
    maxPass  uint16
    // capsuleRadius arms the capsule clearance post-pass of the smoothed
    // waypoints (capsule.go); zero keeps the raw smoothing. It is set once.
    capsuleRadius float64

    mu    sync.RWMutex
    cache map[RegionKey]*cacheEntry
    lru   []*cacheEntry
    // last is the single region fast path: the queries are highly local (a walk
    // probes the same region thousands of cells in a row), so one pointer and
    // key check replaces the lock and the map hash on nearly every call.
    last     atomic.Pointer[regionSlotEntry]
    pool     *layerPool
    files    int
    center   Vec3
    hasFiles bool
}

// regionSlotEntry is the published pair of the entry fast path.
type regionSlotEntry struct {
    key   RegionKey
    entry *cacheEntry
}

// cacheEntry is one loaded or failed region in the cache.
type cacheEntry struct {
    key    RegionKey
    region *Region
    err    error
}

// NewEngine creates an engine over a geodata directory. The directory is
// scanned once for X_Y.l2j files; a missing or empty directory yields a
// working engine without data (every cell lookup fails).
func NewEngine(dir string) *Engine {
    engine := &Engine{
        dir:           dir,
        capacity:      DefaultCacheCapacity,
        maxPass:       DefaultMaxPassableHeight,
        capsuleRadius: 0,
        mu:            sync.RWMutex{},
        cache:         make(map[RegionKey]*cacheEntry),
        lru:           make([]*cacheEntry, 0, DefaultCacheCapacity),
        pool:          newLayerPool(),
        files:         0,
        center:        Vec3{X: 0, Y: 0, Z: 0},
        hasFiles:      false,
    }
    engine.scanFiles()

    return engine
}

// MaxPassableHeight returns the default height step limit of the engine.
func (e *Engine) MaxPassableHeight() uint16 {
    return e.maxPass
}

// SetMaxPassableHeight overrides the default height step limit.
func (e *Engine) SetMaxPassableHeight(maxPassableHeight uint16) {
    e.maxPass = maxPassableHeight
}

// Dir returns the geodata directory of the engine.
func (e *Engine) Dir() string {
    return e.dir
}

// Stats returns the current engine summary.
func (e *Engine) Stats() Stats {
    e.mu.Lock()
    defer e.mu.Unlock()

    return Stats{
        Dir:           e.dir,
        RegionFiles:   e.files,
        LoadedRegions: len(e.cache),
        HasData:       e.hasFiles,
        Center:        e.center,
    }
}

// scanFiles counts the region files and computes the world center of the
// covered area (the mean of the region centers), the initial camera of the web
// UI.
func (e *Engine) scanFiles() {
    entries, err := os.ReadDir(e.dir)
    if err != nil {
        return
    }
    sumX, sumY := 0.0, 0.0
    for _, entry := range entries {
        col, row, ok := parseRegionFileName(entry.Name())
        if !ok {
            continue
        }
        e.files++
        sumX += (float64(col) - tileZeroCol + 0.5) * tileSize
        sumY += (float64(row) - tileZeroRow + 0.5) * tileSize
    }
    if e.files == 0 {
        return
    }
    e.hasFiles = true
    e.center = Vec3{
        X: sumX / float64(e.files),
        Y: sumY / float64(e.files),
        Z: 0,
    }
}

// parseRegionFileName accepts names like "22_22.l2j".
func parseRegionFileName(name string) (int, int, bool) {
    base := strings.TrimSuffix(name, ".l2j")
    if base == name {
        return 0, 0, false
    }
    colText, rowText, found := strings.Cut(base, "_")
    if !found {
        return 0, 0, false
    }
    col, err := strconv.Atoi(colText)
    if err != nil {
        return 0, 0, false
    }
    row, err := strconv.Atoi(rowText)
    if err != nil {
        return 0, 0, false
    }

    return col, row, true
}

// entry returns the cache entry of a region, loading it on demand and evicting
// the least recently inserted one when the cache is full. The hit path shares
// the read lock (the parallel guard probes hit the same four region keys
// millions of times) and the parse path takes the exclusive lock and re-checks.
// The eviction order is the insert order: the parsed regions are immutable and
// a route working set fits the capacity.
func (e *Engine) entry(key RegionKey) (*cacheEntry, error) {
    if slot := e.last.Load(); slot != nil && slot.key == key {
        return slot.entry, slot.entry.err
    }
    e.mu.RLock()
    if entry, ok := e.cache[key]; ok {
        e.mu.RUnlock()
        e.last.Store(&regionSlotEntry{key: key, entry: entry})

        // Failed entries stay cached: every hit must repeat the error,
        // otherwise a missing region would surface as a nil region.
        return entry, entry.err
    }
    e.mu.RUnlock()

    e.mu.Lock()
    defer e.mu.Unlock()

    if entry, ok := e.cache[key]; ok {
        return entry, entry.err
    }

    entry := &cacheEntry{key: key, region: nil, err: nil}
    data, err := os.ReadFile(filepath.Join(
        e.dir, fmt.Sprintf("%d_%d.l2j", key.Col, key.Row)))
    if err == nil {
        entry.region, err = parseRegion(data, key, e.pool)
    }
    if err != nil {
        entry.err = fmt.Errorf("failed to load geodata region %d_%d: %w",
            key.Col, key.Row, err)
    }
    e.cache[key] = entry
    e.lru = append(e.lru, entry)
    for len(e.lru) > e.capacity {
        oldest := e.lru[0]
        e.lru = e.lru[1:]
        delete(e.cache, oldest.key)
    }
    e.last.Store(&regionSlotEntry{key: key, entry: entry})

    return entry, entry.err
}

// Result of a path search: the smoothed waypoints the walker follows,
// the raw cell path for debugging and the search statistics.
type Result struct {
    Found bool
    // Partial reports the closest-reachable answer of a search whose
    // destination is unreachable under its filter: the waypoints end at the
    // closest reachable point. The grid engine never answers one; the navmesh
    // hybrid serves the mesh partial corridors once the engine confirmed the
    // destination truly unreachable, so the consumer walks toward that point
    // instead of aborting at the start position.
    Partial   bool
    Aborted   bool
    Waypoints []Vec3
    RawPath   []Vec3
    Duration  time.Duration
    Explored  int
    OpenLeft  int
    Length    float64
}

// FindPath searches the walkable path from start to end. The passable height
// bounds the step in both directions: ramps, bridges and shores connect their
// cells gradually (8..24 units per cell), while a jump of hundreds is a terrace
// boundary (the lake bed under the floating elven city deck, a cliff behind a
// railing), not a walkable connection. The target layer is the one closest to
// the target z, like the server's own pathfinder (getHeight(tx, ty, tz)), and
// the search succeeds on the first arrival at any layer. An exhausted grid
// returns Found=false with a nil error; hard failures return an error.
func (e *Engine) FindPath(
    start, end Vec3, maxPassableHeight uint16,
) (*Result, error) {
    search := newSearch(e, maxPassableHeight)

    return search.run(start, end, 0)
}

// FindPathApproach searches the walkable path and succeeds as soon as the walk
// reaches a cell within the 3D approach radius of the end, preferring the exact
// target whenever it is reachable. The town trips navigate with it: a merchant
// behind a counter or on a floor layer the geodata does not model (the elven
// village shops hold no deck layer at their real floor z) is still reached on
// the deck ring inside the interaction distance, while the water deck below the
// shop - close in x and y but far in z - never satisfies the radius.
func (e *Engine) FindPathApproach(
    start, end Vec3, approachRadius float64, maxPassableHeight uint16,
) (*Result, error) {
    search := newSearch(e, maxPassableHeight)

    return search.run(start, end, approachRadius)
}

// FindPathApproachAvoiding plans the walk around the caller avoid areas: every
// cell inside one is impassable, so the A* detours around the patch. The start
// cell stays allowed, because the walker standing inside a patch must plan its
// way out (its escape ring cells cost avoidEscapeMultiplier instead). The
// ban-less fallback once re-planned the very corridor the bans exist to detour,
// so the zone return and the quest segments always search with the bans.
func (e *Engine) FindPathApproachAvoiding(
    start, end Vec3, approachRadius float64, maxPassableHeight uint16,
    avoid []AvoidArea,
) (*Result, error) {
    search := newSearch(e, maxPassableHeight)
    search.avoid = avoid

    return search.run(start, end, approachRadius)
}

// AvoidArea names one world patch the recovery searches route around: a circle
// over ground the live server refused although the geodata pack models it as
// open. The hunt derives the patches from its freeze reports (the aimed
// waypoint of a segment whose re-path produced no movement) and keeps them for
// the session, so every later plan detours instead of re-planning the identical
// deterministic route into it.
type AvoidArea struct {
    Center Vec3
    Radius float64
}

// OverWater reports whether the walkable surface lies below the C1 water level:
// the character stands on (or swims over) a lake or sea bed. The layer is the
// one closest to the reference z, the deck the character is on. A position
// without geodata answers false, so a broken pack cannot trap the walker in an
// endless escape.
func (e *Engine) OverWater(x, y float64, refZ int16) bool {
    coords := WorldToCell(x, y)
    entry, err := e.entry(CellToRegion(coords))
    if err != nil || entry.region == nil {
        return false
    }
    layer, ok := entry.region.ClosestLayer(LocalCell(coords), refZ)
    if !ok {
        return false
    }

    return layer.Height < WaterLevel
}

// CellBlockedAtZ mirrors the packet level isCompletelyBlocked check of the
// Mobius MoveToLocation handler: the click validation port (ValidateClick)
// walks the terrain-following z along its Bresenham line like the server AI
// correction, but the packet handler asks the destination cell at the click z -
// the nearest layer decides, and a closed upper layer (a tree canopy, a cliff
// band) refuses the click with an instant ActionFailed although the ground
// below is walkable (the kite endpoints on the hill deck bounce while the
// rotated fan lanes walk fine). The straight line fight steps read this gate.
func (e *Engine) CellBlockedAtZ(x, y float64, z int16) bool {
    return e.cellBlocked(WorldToCell(x, y), z)
}

// ClosestHeight resolves the height of the layer closest to refZ: the deck the
// server itself would pick for a destination at (x, y) named with z = refZ (its
// pathfinder resolves the target cell by the destination z, not by the deck the
// walker stands on). Planning uses it to put a search goal on real ground
// before FindPathApproach, because a fabricated height puts the 3D approach
// goal mid air and the search can never match it. A missing region answers the
// load error, a cell without layers ErrMissingCell; the caller stays on its
// fallback height.
func (e *Engine) ClosestHeight(
    x, y float64, refZ int16,
) (int16, error) {
    coords := WorldToCell(x, y)
    entry, err := e.entry(CellToRegion(coords))
    if err != nil {
        return 0, err
    }
    layer, ok := entry.region.ClosestLayer(LocalCell(coords), refZ)
    if !ok {
        return 0, fmt.Errorf("%w at %.0f %.0f", ErrMissingCell, x, y)
    }

    return layer.Height, nil
}

// LineOfSight reports whether a straight line between two world
// positions crosses only open cells with compatible heights.
func (e *Engine) LineOfSight(
    start, end Vec3, maxPassableHeight uint16,
) (bool, error) {
    return e.lineOfSightCells(start, end, maxPassableHeight)
}
