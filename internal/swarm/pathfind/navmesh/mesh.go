// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "sync"
)

// DefaultMeshCapacity bounds how many tiles stay loaded. One tile of
// a dense region costs a few megabytes, so the default keeps the
// working set of any long route (a handful of regions) resident with
// headroom; the loads are cheap (the whole tile decodes in about a
// millisecond) so an eviction is not a stall. A capacity of zero or
// less means unlimited (the full 165 region pack is feasible at
// roughly half a gigabyte).
const DefaultMeshCapacity = 32

// tileFileExt is the tile file extension under the mesh directory.
const tileFileExt = ".nm"

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
