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
	"time"
)

// DefaultMaxPassableHeight is the default maximum height the search
// accepts climbing between two neighbouring cells. It mirrors the
// Mobius GeoEngine HEIGHT_INCREASE_LIMIT (40) - the upward step gate
// of the server movement validation the planned walks must pass.
const DefaultMaxPassableHeight = uint16(40)

// DefaultCacheCapacity bounds how many parsed regions stay in memory.
// One multilayer region costs roughly 20 MB parsed, so the default keeps
// the working set of a pathfind session comfortably small; the cache
// evicts the least recently used regions when it overflows.
const DefaultCacheCapacity = 4

// MaxSearchExpansions caps the node expansions of one search. An
// unreachable target in open terrain would otherwise explore the whole
// region grid (4.2M cells, tens of seconds); the cap stops the search
// early and reports it through Result.Aborted. Long real world paths
// stay two orders of magnitude below the cap.
const MaxSearchExpansions = 1000000

// ErrMissingCell reports that a requested start or target position has
// no geodata underneath (outside the shipped regions or no geodata
// directory).
var ErrMissingCell = errors.New("cell has no geodata")

// Stats is the engine state summary for diagnostics and the web UI.
type Stats struct {
	Dir           string `json:"dir"`
	RegionFiles   int    `json:"regionFiles"`
	LoadedRegions int    `json:"loadedRegions"`
	HasData       bool   `json:"hasData"`
	Center        Vec3   `json:"center"`
}

// Engine reads geodata region files lazily and answers cell layer
// queries for the search. It is safe for concurrent use.
type Engine struct {
	dir      string
	capacity int
	maxPass  uint16

	mu       sync.Mutex
	cache    map[RegionKey]*cacheEntry
	lru      []*cacheEntry
	pool     *layerPool
	files    int
	center   Vec3
	hasFiles bool
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
		dir:      dir,
		capacity: DefaultCacheCapacity,
		maxPass:  DefaultMaxPassableHeight,
		mu:       sync.Mutex{},
		cache:    make(map[RegionKey]*cacheEntry),
		lru:      make([]*cacheEntry, 0, DefaultCacheCapacity),
		pool:     newLayerPool(),
		files:    0,
		center:   Vec3{X: 0, Y: 0, Z: 0},
		hasFiles: false,
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

// scanFiles counts the region files in the directory and computes the
// world center of the covered area (the mean of the region centers),
// which the web UI uses as the initial camera position.
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

// entry returns the cache entry of a region, loading it on demand and
// evicting the least recently used entry when the cache is full.
func (e *Engine) entry(key RegionKey) (*cacheEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if entry, ok := e.cache[key]; ok {
		e.touch(entry)

		// Failed entries stay cached: every hit must repeat the error,
		// otherwise a missing region would surface as a nil region.
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
	e.touch(entry)

	return entry, entry.err
}

// touch moves an entry to the back of the LRU queue.
func (e *Engine) touch(entry *cacheEntry) {
	for i, candidate := range e.lru {
		if candidate == entry {
			e.lru = append(e.lru[:i], e.lru[i+1:]...)
			e.lru = append(e.lru, entry)

			return
		}
	}
}

// Result of a path search: the smoothed waypoints the walker follows,
// the raw cell path for debugging and the search statistics.
type Result struct {
	Found     bool
	Aborted   bool
	Waypoints []Vec3
	RawPath   []Vec3
	Duration  time.Duration
	Explored  int
	OpenLeft  int
	Length    float64
}

// FindPath searches the walkable path from start to end. The max
// passable height bounds the height difference the walker can step
// between neighbouring cells in BOTH directions: the climb gate is
// the Mobius HEIGHT_INCREASE_LIMIT (40) and the drop uses the same
// bound, because a walkable surface connects its cells gradually -
// ramps, bridges and shores step 8..24 units per cell - while a
// height jump of hundreds of units is a terrace boundary (the lake
// bed under the floating elven city deck, a cliff behind a railing
// the geodata does not model), not a walkable connection. The target
// layer is selected as the layer of the target cell closest to the
// target z, like the server's own pathfinder does
// (getHeight(tx, ty, tz)), and the search succeeds on the first
// arrival on the target cell at any layer. A search that exhausts
// the grid returns Found=false with a nil error; hard failures (no
// geodata at the start or target, corrupt regions) return an error.
func (e *Engine) FindPath(
	start, end Vec3, maxPassableHeight uint16,
) (*Result, error) {
	search := newSearch(e, maxPassableHeight)

	return search.run(start, end, 0)
}

// FindPathApproach searches the walkable path from start to end and
// succeeds as soon as the walk reaches a cell within the approach
// radius (the 3D distance) of the end point - preferring the exact
// target whenever it is reachable. The town trips navigate with it:
// a merchant standing behind a counter or on a floor layer the
// geodata does not model (the elven village shops hold no deck layer
// at their real floor z) is still reached correctly, because the walk
// ends on the deck ring within the merchant interaction distance
// while the water deck below the shop - close in x and y but far in
// z - never satisfies the radius.
func (e *Engine) FindPathApproach(
	start, end Vec3, approachRadius float64, maxPassableHeight uint16,
) (*Result, error) {
	search := newSearch(e, maxPassableHeight)

	return search.run(start, end, approachRadius)
}

// FindPathApproachDry is the water walled form of FindPathApproach:
// every step of the search onto an underwater cell costs impassable,
// so the waypoints of a found route all stand above the water level (a
// start below it exits to the shore first). The shore walks of the
// hunt loop navigate with it: a planned swim is a plan the click guard
// refuses leg by leg, and the walker burned its whole re-path budget
// re-planning the identical wet route before it aborted (the delevel
// water loop of the 2026-09-10 state dump, stuck at the elven village
// shore). A target only swimming reaches answers Found=false: the
// caller aborts the leg and arms its cooldown instead of walking into
// the water.
func (e *Engine) FindPathApproachDry(
	start, end Vec3, approachRadius float64, maxPassableHeight uint16,
) (*Result, error) {
	search := newSearch(e, maxPassableHeight)
	search.dry = true

	return search.run(start, end, approachRadius)
}

// AvoidArea names one world patch the recovery searches route around:
// a circle over the ground the live server refused to walk although
// the geodata pack modeled it as open. The hunt loop derives the
// patches from its freeze reports (the aimed waypoint of a leg whose
// re-path produced no movement at all) and keeps them for the session,
// so every later plan detours around the frozen corridor instead of
// re-planning the identical deterministic route into it.
type AvoidArea struct {
	Center Vec3
	Radius float64
}

// FindPathApproachDryAvoiding is FindPathApproachDry with the avoid
// areas of the caller: the search treats every cell inside an area as
// impassable, the direct line shortcut refuses lines crossing one and
// the smoothing keeps its collapsed legs outside them, so the returned
// route detours around the banned ground. The start cell itself stays
// allowed wherever it sits - the walker standing inside a patch must
// be able to plan its way OUT of it. A route that only exists through
// the banned ground answers Found=false.
func (e *Engine) FindPathApproachDryAvoiding(
	start, end Vec3, approachRadius float64, maxPassableHeight uint16,
	avoid []AvoidArea,
) (*Result, error) {
	search := newSearch(e, maxPassableHeight)
	search.dry = true
	search.avoid = avoid

	return search.run(start, end, approachRadius)
}

// FindWaterEscape plans the way out of the water for a position whose
// geodata surface lies below the C1 water level: the walk to the
// nearest shore cell standing above the water surface. The hunt loop
// arms it when the character stands over a lake or sea bed (the
// town trip stuck under the elven village plateau swam there) - the
// ordinary destination searches are meaningless until the character
// is back ashore, because the server refuses move requests from a
// floating character onto decks the water has no walkable connection
// to. A start already on dry ground answers Found=false without an
// error: no escape is needed.
func (e *Engine) FindWaterEscape(start Vec3) (*Result, error) {
	search := newSearch(e, e.maxPass)

	return search.runEscape(start)
}

// DryLine reports whether the straight segment between two world
// positions is a clean dry walk: walkable by the surface rules (the
// same raster the line of sight uses) and never dipping under the
// water level anywhere along the line. The town walker checks every
// click target with it before sending the move request: the server
// moves characters into water without any hesitation (its own
// pathfinding carries no water cost, and swimming move requests skip
// the geodata validation entirely), so keeping the character ashore
// is the walker's own job.
func (e *Engine) DryLine(start, end Vec3) (bool, error) {
	search := newSearch(e, e.maxPass)
	from, err := search.nodeAtWorld(start)
	if err != nil {
		return false, err
	}
	to, err := search.nodeAtWorld(end)
	if err != nil {
		return false, err
	}

	return search.lineOfSight(from, to) && search.dryLine(from, to), nil
}

// WaterCrossed reports whether the straight line between two world
// positions crosses cells whose resolved surface lies below the
// water level: the pure water raster of DryLine without its line of
// sight gate. The water guard of the town trips needs exactly this -
// the sight gate exists to validate a walkable leg (the height steps
// of the smoothing), but a click line that merely crosses a height
// step (the village deck ramps, the plaza over the shops) routes
// fine through the server pathfinder and must not read as water.
func (e *Engine) WaterCrossed(start, end Vec3) (bool, error) {
	search := newSearch(e, e.maxPass)
	from, err := search.nodeAtWorld(start)
	if err != nil {
		return false, err
	}
	to, err := search.nodeAtWorld(end)
	if err != nil {
		return false, err
	}

	return !search.dryLine(from, to), nil
}

// OverWater reports whether the walkable surface under a world
// position lies below the C1 water level: the character stands (or
// swims) over a lake or sea bed. The layer is the one closest to the
// reference z - the deck the character itself is on. A position
// without geodata answers false (never over water) so a broken pack
// cannot trap the walker in an endless escape.
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

	return layer.Height < waterLevel
}

// ClosestHeight resolves the height of the geodata layer at the world
// position that is closest to refZ: the deck the server itself would
// pick for a destination at (x, y) named with z = refZ (its own
// pathfinder resolves the target cell by the destination z, not by
// the deck the walker stands on). Planning code uses it to put a
// search goal on real ground before FindPathApproach - a fabricated
// height at the target puts the 3D approach goal mid air whenever the
// destination sits on another deck than the walker, and the search
// then can never match the goal. A position with no region file
// under it answers the region load error, a cell without layers
// ErrMissingCell; either way the caller stays on its fallback height.
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
	search := newSearch(e, maxPassableHeight)
	from, err := search.nodeAtWorld(start)
	if err != nil {
		return false, err
	}
	to, err := search.nodeAtWorld(end)
	if err != nil {
		return false, err
	}

	return search.lineOfSight(from, to), nil
}
