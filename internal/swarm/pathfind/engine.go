// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultMaxPassableHeight is the default maximum height difference the
// search accepts between two neighbouring cells. It follows the
// L2Bot2.0 default (its combat AI moves with 30).
const DefaultMaxPassableHeight = uint16(30)

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
		cache:    make(map[RegionKey]*cacheEntry),
		lru:      make([]*cacheEntry, 0, DefaultCacheCapacity),
		pool:     newLayerPool(),
		files:    0,
		center:   Vec3{},
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
	e.center = Vec3{X: sumX / float64(e.files), Y: sumY / float64(e.files)}
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

// regionFileNames lists the region files of the directory sorted by
// name, for tests and diagnostics.
func (e *Engine) regionFileNames() []string {
	entries, err := os.ReadDir(e.dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, e.files)
	for _, entry := range entries {
		if _, _, ok := parseRegionFileName(entry.Name()); ok {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	return names
}

// cellLayers returns the layer stack of a global cell, loading its
// region on demand. The second result is false when the cell has no
// geodata (missing or failed region).
func (e *Engine) cellLayers(p Point) ([]Layer, bool) {
	key := CellToRegion(p)
	entry, err := e.entry(key)
	if err != nil {
		return nil, false
	}

	return entry.region.Layers(LocalCell(p)), true
}

// closestLayer returns the layer of the cell closest to z.
func (e *Engine) closestLayer(p Point, z int16) (Layer, bool) {
	key := CellToRegion(p)
	entry, err := e.entry(key)
	if err != nil {
		return Layer{}, false
	}

	return entry.region.ClosestLayer(LocalCell(p), z)
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

	entry := &cacheEntry{key: key}
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
// between neighbouring cells. The target layer is selected as the layer
// of the target cell closest to the start height, like the original
// pathfinder does: a target coordinate with several floors resolves to
// the floor the walker can actually reach from where it stands. A search
// that exhausts the grid returns Found=false with a nil error; hard
// failures (no geodata at the start or target, corrupt regions) return
// an error.
func (e *Engine) FindPath(
	start, end Vec3, maxPassableHeight uint16,
) (*Result, error) {
	search := newSearch(e, maxPassableHeight)

	return search.run(start, end)
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
