// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "bytes"
    "compress/gzip"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "sync"
)

// DefaultMeshCapacity bounds how many tiles stay loaded. The exact
// square port inflated the dense region tiles into the hundreds of
// megabytes of heap (the 2.6M polygon region holds 230 MB of polys
// and links, whole map 100M polys), so the default keeps the working
// set of a route (the current region and its neighbours) resident
// without risking the memory; the loads are cheap (the whole tile
// decodes in well under a second, the hop corridors cache across
// evictions) so an eviction is not a stall. A capacity of zero or
// less means unlimited (fine for the sparse tiles, a memory hazard on
// the dense whole map pack).
const DefaultMeshCapacity = 4

// tileFileExt is the tile file extension under the mesh directory.
const tileFileExt = ".nm"

// abstractFileExt is the persistent coarse layer sidecar extension
// (docs/fastpath_research.md): the pack build writes the region
// cluster graph next to the tile, the runtime loads it instead of
// re-scanning the decoded tile.
const abstractFileExt = ".ab"

// Mesh loads navigation mesh tiles lazily from a directory and
// answers the navigation queries over them. It is safe for concurrent
// use: the tiles are immutable after decode and the query state is
// pooled per call.
type Mesh struct {
    dir      string
    capacity int

    mu     sync.Mutex
    tiles  map[RegionKey]*tileEntry
    lru    []*tileEntry
    files  map[RegionKey]struct{}
    states sync.Pool

    // The hierarchy layer: the abstract cluster graphs (LRU bounded,
    // the rebuilds are deterministic), the coarse search state pool
    // and the hop corridor cache (the HNA* intra-edges).
    abstractMu       sync.Mutex
    abstracts        map[RegionKey]*regionAbstract
    abstractOrder    []RegionKey
    abstractCapacity int
    coarsePool       sync.Pool
    hierMu           sync.Mutex
    hops             map[hopKey][]PolyRef
    hopOrder         []hopKey
    dstComps         map[abstractEdgeRef]uint32
}

// tileEntry is one loaded, failed or missing tile in the cache.
type tileEntry struct {
    key   RegionKey
    tile  *Tile
    err   error
    state tileState
}

type tileState uint8

const (
    tileLoaded tileState = iota
    tileFailed
    tileMissing
)

// RegionKey identifies a tile by its region coordinates (the X_Y of
// the X_Y.nm file name).
type RegionKey struct {
    Col, Row int16
}

// ErrTileAbsent reports a region without a tile file: the link
// resolution treats it as a wall, the callers that need a tile check
// it with errors.Is.
var ErrTileAbsent = errors.New("navmesh tile absent")

// NewMesh creates a mesh over a tile directory. The directory is
// scanned once for X_Y.nm files; an empty or missing directory yields
// a working mesh whose every query answers the NoNavmeshError.
func NewMesh(dir string) *Mesh {
    mesh := &Mesh{
        dir:      dir,
        capacity: DefaultMeshCapacity,
        mu:       sync.Mutex{},
        tiles:    make(map[RegionKey]*tileEntry),
        lru:      make([]*tileEntry, 0, DefaultMeshCapacity),
        files:    make(map[RegionKey]struct{}),
        states: sync.Pool{New: func() any {
            return &queryState{
                nodes:  nil,
                index:  make(map[PolyRef]uint32, 1024),
                open:   nil,
                best:   0,
                bestH:  0,
                escape: false,
            }
        }},
        abstracts:        make(map[RegionKey]*regionAbstract),
        abstractOrder:    make([]RegionKey, 0, abstractCacheCapacity),
        abstractCapacity: abstractCacheCapacity,
        coarsePool: sync.Pool{New: func() any {
            return &coarseState{
                nodes:   nil,
                index:   make(map[coarseNodeKey]int32, 1024),
                settled: make(map[coarseCompKey]bool, 1024),
                open:    nil,
                best:    -1,
                bestH:   0,
            }
        }},
        hops:     make(map[hopKey][]PolyRef),
        hopOrder: make([]hopKey, 0, hopCacheCapacity),
    }
    mesh.scanFiles()

    return mesh
}

// SetCacheCapacity overrides the loaded tile bound (a value of zero
// or less keeps every tile loaded).
func (m *Mesh) SetCacheCapacity(capacity int) {
    m.mu.Lock()
    m.capacity = capacity
    m.mu.Unlock()
}

// SetAbstractCapacity overrides the abstract (cluster graph) cache
// bound (a value of zero or less keeps every abstract of the pack
// resident - the whole map diagnostic arithmetic, a memory hazard on
// the dense packs).
func (m *Mesh) SetAbstractCapacity(capacity int) {
    m.abstractMu.Lock()
    m.abstractCapacity = capacity
    m.abstractMu.Unlock()
}

// Stats is the mesh summary for diagnostics.
type Stats struct {
    Dir       string `json:"dir"`
    TileFiles int    `json:"tileFiles"`
    Loaded    int    `json:"loadedTiles"`
}

// Stats returns the current mesh summary.
func (m *Mesh) Stats() Stats {
    m.mu.Lock()
    defer m.mu.Unlock()

    return Stats{Dir: m.dir, TileFiles: len(m.files), Loaded: len(m.tiles)}
}

// TileFiles lists the region keys of the tile files found in the mesh
// directory, ordered col first then row: the directory scan without
// a single tile decode (the viewer listing and the flag validation).
func (m *Mesh) TileFiles() []RegionKey {
    m.mu.Lock()
    defer m.mu.Unlock()

    keys := make([]RegionKey, 0, len(m.files))
    for key := range m.files {
        keys = append(keys, key)
    }
    sort.Slice(keys, func(i, j int) bool {
        if keys[i].Col != keys[j].Col {
            return keys[i].Col < keys[j].Col
        }

        return keys[i].Row < keys[j].Row
    })

    return keys
}

// scanFiles counts the tile files of the directory.
func (m *Mesh) scanFiles() {
    entries, err := os.ReadDir(m.dir)
    if err != nil {
        return
    }
    for _, entry := range entries {
        base := strings.TrimSuffix(entry.Name(), tileFileExt)
        if base == entry.Name() {
            continue
        }
        colText, rowText, found := strings.Cut(base, "_")
        if !found {
            continue
        }
        col, err := strconv.Atoi(colText)
        if err != nil || col < -32768 || col > 32767 {
            continue
        }
        row, err := strconv.Atoi(rowText)
        if err != nil || row < -32768 || row > 32767 {
            continue
        }
        // The bounds checks above pin the int16 conversion range.
        key := RegionKey{Col: int16(col), Row: int16(row)} //nolint:gosec // guarded
        m.files[key] = struct{}{}
    }
}

// Tile returns the loaded tile of a region, loading it on demand. A
// missing tile file is a cached miss (not an error): the link
// resolution treats it as a wall.
func (m *Mesh) Tile(key RegionKey) (*Tile, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if entry, ok := m.tiles[key]; ok {
        m.touch(entry)
        m.evict()

        switch entry.state {
        case tileLoaded:
            return entry.tile, nil
        case tileFailed:
            return nil, entry.err
        default:
            return nil, ErrTileAbsent
        }
    }
    entry := &tileEntry{key: key, tile: nil, err: nil, state: tileLoaded}
    if _, ok := m.files[key]; !ok {
        entry.state = tileMissing
    } else {
        data, err := os.ReadFile(filepath.Join(
            m.dir, fmt.Sprintf("%d_%d%s", key.Col, key.Row, tileFileExt)))
        if err != nil {
            entry.state = tileFailed
            entry.err = fmt.Errorf("read navmesh tile %d_%d: %w", key.Col,
                key.Row, err)
        } else {
            // The tile file may be gzip compressed (the
            // navmesh-build -compress output): the gzip magic word
            // decides, both formats decode the same way.
            if len(data) >= 2 && data[0] == 0x1F && data[1] == 0x8B {
                zr, gzErr := gzip.NewReader(bytes.NewReader(data))
                if gzErr == nil {
                    var raw []byte
                    raw, gzErr = io.ReadAll(zr)
                    _ = zr.Close()
                    if gzErr == nil {
                        data = raw
                    }
                }
                if gzErr != nil {
                    entry.state = tileFailed
                    entry.err = fmt.Errorf(
                        "decompress navmesh tile %d_%d: %w", key.Col,
                        key.Row, gzErr)
                    m.tiles[key] = entry
                    m.lru = append(m.lru, entry)
                    m.touch(entry)
                    m.evict()

                    return nil, entry.err
                }
            }
            entry.tile, entry.err = DecodeTile(data)
            if entry.err != nil {
                entry.state = tileFailed
                entry.err = fmt.Errorf("decode navmesh tile %d_%d: %w",
                    key.Col, key.Row, entry.err)
            } else {
                entry.state = tileLoaded
            }
        }
    }
    m.tiles[key] = entry
    m.lru = append(m.lru, entry)
    m.touch(entry)
    m.evict()

    switch entry.state {
    case tileLoaded:
        return entry.tile, nil
    case tileFailed:
        return nil, entry.err
    default:
        return nil, ErrTileAbsent
    }
}

// evict drops the least recently used entries beyond the capacity.
func (m *Mesh) evict() {
    for len(m.lru) > m.capacity && m.capacity > 0 {
        oldest := m.lru[0]
        m.lru = m.lru[1:]
        delete(m.tiles, oldest.key)
    }
}

// touch moves an entry to the back of the LRU queue.
func (m *Mesh) touch(entry *tileEntry) {
    for i, candidate := range m.lru {
        if candidate == entry {
            m.lru = append(m.lru[:i], m.lru[i+1:]...)
            m.lru = append(m.lru, entry)

            return
        }
    }
}

// tileOfRef returns the loaded tile holding a reference (loading it
// on demand). A missing or failed tile answers nil: the caller treats
// the link as a wall.
func (m *Mesh) tileOfRef(ref PolyRef) *Tile {
    col, row := TileOf(ref)
    tile, err := m.Tile(RegionKey{Col: col, Row: row})
    if err != nil || tile == nil {
        return nil
    }

    return tile
}

// polyOfRef returns the polygon of a reference together with its
// tile, or nil when the tile is unavailable or the index is stale.
func (m *Mesh) polyOfRef(ref PolyRef) (*Tile, *Poly) {
    poly := PolyOf(ref)
    if poly < 0 {
        return nil, nil
    }
    tile := m.tileOfRef(ref)
    if tile == nil || int(poly) >= len(tile.Polys) {
        return nil, nil
    }

    return tile, &tile.Polys[poly]
}

// acquireState takes a pooled query state.
func (m *Mesh) acquireState() *queryState {
    return m.states.Get().(*queryState)
}

// releaseState returns a query state to the pool (the buffers keep
// their capacity for the next query).
func (m *Mesh) releaseState(state *queryState) {
    m.states.Put(state)
}

// acquireCoarse takes a pooled coarse (cluster level) search state.
func (m *Mesh) acquireCoarse() *coarseState {
    return m.coarsePool.Get().(*coarseState)
}

// releaseCoarse returns a coarse search state to the pool.
func (m *Mesh) releaseCoarse(state *coarseState) {
    m.coarsePool.Put(state)
}

// resolveLink names the neighbor a link leads to: an internal link
// resolves inside the same tile, an external link loads the neighbor
// region tile and validates its polygon index.
func (m *Mesh) resolveLink(tile *Tile, link *Link) PolyRef {
    if link.To >= 0 {
        if int(link.To) >= len(tile.Polys) {
            return 0
        }

        return RefOf(tile.Col, tile.Row, uint32(link.To))
    }
    extIdx := -link.To - 1
    if int(extIdx) >= len(tile.ExtLinks) {
        return 0
    }
    ext := &tile.ExtLinks[extIdx]
    if ext.Poly == 0xFFFFFFFF {
        return 0
    }
    neighbor, err := m.Tile(
        RegionKey{Col: int16(ext.Col), Row: int16(ext.Row)})
    if err != nil || neighbor == nil ||
        ext.Poly >= uint32(len(neighbor.Polys)) {
        return 0
    }

    return RefOf(int16(ext.Col), int16(ext.Row), ext.Poly)
}

// PolyOf returns the tile and polygon of a reference (nil when the
// tile is unavailable or the index is stale).
func (m *Mesh) PolyOf(ref PolyRef) (*Tile, *Poly) {
    return m.polyOfRef(ref)
}
