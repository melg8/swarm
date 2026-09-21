// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "errors"
    "fmt"
    "hash/crc32"
    "io"
    "math"
    "os"
    "path/filepath"
    "time"
)

// The hierarchical route layer (the HNA* adaptation of
// https://github.com/educharlie/HNA-Algorithm for the region tile
// world): the polygon mesh stays the level 0 geometry, the cluster
// graph is the level 1 abstraction above it. One abstract node is a
// square block of clusterCellsSide x clusterCellsSide geodata cells
// (2048 x 2048 world units); one abstract edge is a mesh link whose
// portal crosses a cluster boundary, carrying the crossing midpoint
// the refinement hops search through. The abstract graph of a region
// builds lazily from the decoded tile (one pass over the links),
// caches forever (a few megabytes for the densest region) and lets
// the tile itself leave under the LRU. The HNA* paper builds the
// same shape of hierarchy with a graph partitioner (METIS) over one
// Recast tile; the region grid of this world already partitions the
// map, so the clusters are the deterministic spatial blocks between
// the region borders and the partitioner step disappears.
//
// The whole map arithmetic (the layer count answer): the 165 region
// pack holds ~50M polygons at level 0, 165 x 16 x 16 = 42240 cluster
// nodes at level 1 and 165 region nodes at level 2. Level 1 fits one
// search budget whole map wide, level 2 is the trivial shortcut the
// region keys already give; three levels cover the world.
const (
    // clusterCellsSide is the cell count of one cluster side (128
    // cells = 2048 world units).
    clusterCellsSide = int32(128)
    // clustersPerSide is the cluster count of one region side (16).
    clustersPerSide = regionCellsSide / clusterCellsSide
    // unknownArea marks the target area an external link cannot name
    // without decoding the neighbour tile (the coarse search prices
    // it as ground, the dry search lets the refinement decide).
    unknownArea = uint8(0xFF)
)

// clusterKey names one abstract node: the cluster block of a region.
type clusterKey struct {
    Col, Row int16
    ID       clusterID
}

// clusterID packs the cluster indices inside a region (CX<<4|CY).
type clusterID uint8

// clusterOfCell returns the cluster id of a region local cell.
func clusterOfCell(cx, cy int32) clusterID {
    return clusterID(uint8(cx/clusterCellsSide)<<4 |
        uint8(cy/clusterCellsSide))
}

// clusterOfPos returns the cluster key of a world position (the
// position clamps into the region - a border position belongs to the
// region the query named).
func clusterOfPos(col, row int16, x, y float64) clusterKey {
    minX := (float64(col) - tileZeroCol) * tileWorldSize
    minY := (float64(row) - tileZeroRow) * tileWorldSize
    cx := int32(math.Floor((x - minX) / cellSizeWorld))
    cy := int32(math.Floor((y - minY) / cellSizeWorld))
    cx = clampCell(cx)
    cy = clampCell(cy)

    return clusterKey{Col: col, Row: row, ID: clusterOfCell(cx, cy)}
}

// clampCell folds a region local cell coordinate into the region.
func clampCell(c int32) int32 {
    if c < 0 {
        return 0
    }
    if c >= regionCellsSide {
        return regionCellsSide - 1
    }

    return c
}

// abstractEdgeRef identifies one coarse edge of the whole world: the
// region that owns it plus the local edge index (the hop cache and
// the query bans name edges through it).
type abstractEdgeRef struct {
    Region RegionKey
    Index  int32
}

// abstractEdge is one cluster to cluster portal: the mesh link whose
// portal span crosses a cluster boundary. The refinement hop resolves
// the target polygon through the source link (the neighbour tile
// decodes on demand) and searches from the crossing midpoint.
type abstractEdge struct {
    to clusterKey
    // toRef is the target polygon of the crossing when the link is
    // internal; zero defers to the link resolution (external target).
    toRef PolyRef
    // link is the index into the source tile's link store: the
    // refinement resolves the target through it (the tile may need a
    // decode at that point).
    link int32
    // mid is the portal midpoint: the world position the hop searches
    // through (on the shared edge, at the surface height).
    mid Pos
    // srcComp is the link component of the source polygon; dstComp is
    // the component of the target polygon (known at build time for
    // the internal links, zero until the coarse search resolves it
    // through the neighbour abstract for the external ones).
    srcComp uint32
    dstComp uint32
    // extTarget names the neighbour side of an external crossing (the
    // record the source tile already carries): the coarse search
    // resolves the target component through it without a neighbour
    // tile decode. Zero poly marks the internal edges.
    extCol  int16
    extRow  int16
    extPoly uint32
    srcArea uint8
    dstArea uint8
}

// abstractNode is one cluster of the region abstract graph.
type abstractNode struct {
    id    clusterID
    edges []int32
    // classes dedupes the parallel crossings: one representative edge
    // per target cluster and area pair (the crossing closest to the
    // shared side center). The exact square port produces millions of
    // tiny polygons, a cluster border holds thousands of open cell
    // pairs - the coarse search needs the connectivity and one honest
    // crossing point per class, not every cell pair of the border.
    classes map[abstractClassKey]int32
}

// abstractClassKey dedupes the parallel edges of one node: the
// target cluster, the area pair of the crossing and the source
// component (the crossings of different components serve different
// parts of the cluster and must all survive the dedupe).
type abstractClassKey struct {
    to      clusterKey
    srcArea uint8
    dstArea uint8
    srcComp uint32
}

// regionAbstract is the level 1 graph of one region: the clusters
// with polygons as nodes, the boundary crossing portals as edges.
type regionAbstract struct {
    key   RegionKey
    nodes []abstractNode
    index map[clusterID]int32
    edges []abstractEdge
    // comps is the link component id per polygon of the source tile
    // (the whole tile link graph is bidirectional, the union find
    // over the links labels it): the coarse search chains two edges
    // through one cluster only when the crossing they stand on and
    // the crossing they leave through share the component - the
    // intra cluster connectivity the HPA* entrance analysis gives.
    comps []uint32
    // polys is the polygon count of the source tile (the level 0
    // size the doc arithmetic quotes).
    polys int
    // TileSize/TileModTime are the tile file stat the sidecar was
    // built against (the staleness guard of the sidecar load; the
    // in memory rebuild leaves them zero). The v2 sidecars also
    // carry the head and the tail CRC32 of the tile file bytes
    // (TileChecksums): the mtime loss tolerant guard.
    TileSize      int64
    TileModTime   time.Time
    TileChecksums bool
    TileHeadCRC   uint32
    TileTailCRC   uint32
}

// BuildAbstract scans one decoded tile into its cluster graph (the
// exported path of the pack sidecar pass and the tests). The answer
// shares the buildAbstract semantics: one deterministic pass over the
// link chains.
//
// sidecar pass and the tests forward the answer opaquely (encode,
// diff), the exported shape keeps the call sites uniform.
//
//nolint:revive // the graph type stays internal by design; the
func BuildAbstract(tile *Tile) *regionAbstract {
    return buildAbstract(tile)
}

// The abstract cache bound. The abstracts were cached forever and the
// whole map coarse walks accumulated every region's graph (the comps
// array alone is 4 bytes per polygon - the 100M polygon pack sums
// into the hundreds of megabytes); the LRU keeps the working set of
// the running queries (the current coarse frontier and the hop
// refinement tiles) and rebuilds the evicted ones on demand (one
// deterministic pass over the immutable tile, the edge indices stay
// stable across the rebuilds - the ban bookkeeping survives).
const abstractCacheCapacity = 32

// abstractOf returns the cached cluster graph of a region, building it
// over the decoded tile on demand. A tile that fails to load answers
// nil (the coarse search treats the region as a wall, the same
// semantics the fine search applies to a missing tile). The LRU bound
// keeps the whole map walks from accumulating every region graph; the
// rebuilds are deterministic and cheap against the route costs.
func (m *Mesh) abstractOf(key RegionKey) *regionAbstract {
    m.abstractMu.Lock()
    abstract, ok := m.abstracts[key]
    if ok {
        m.touchAbstract(key)
        m.abstractMu.Unlock()

        return abstract
    }
    m.abstractMu.Unlock()

    // The persistent coarse layer first: the sidecar answers without
    // decoding the tile (the stat guard inside ties it to the exact
    // tile file). The tile decode stays for the rebuild fallback, the
    // refinement hops and the nearest poly queries.
    if sidecar := m.abstractSidecarOf(key); sidecar != nil {
        m.cacheAbstract(key, sidecar)

        return sidecar
    }

    tile, err := m.Tile(key)
    if err != nil || tile == nil {
        m.cacheAbstract(key, nil)

        return nil
    }
    built := buildAbstract(tile)
    m.cacheAbstract(key, built)

    return built
}

// cacheAbstract stores the abstract in the LRU (nil caches the miss).
// The last writer wins: the build is a deterministic function of the
// immutable tile, the concurrent duplicate is discarded.
func (m *Mesh) cacheAbstract(key RegionKey, abstract *regionAbstract) {
    m.abstractMu.Lock()
    m.abstracts[key] = abstract
    m.abstractOrder = append(m.abstractOrder, key)
    m.evictAbstracts()
    m.abstractMu.Unlock()
}

// abstractSidecarOf loads the region abstract from the X_Y.ab sidecar
// without decoding the tile (a nil answer means the sidecar is
// absent, stale or unreadable and the caller decodes and rebuilds).
// The sidecar carries the tile file size and mtime it was built
// against; the mismatch rejects it, so a stale sidecar never serves.
// The v2 sidecars carry the tile head and tail checksums instead:
// the deployment copies that lose the file mtimes keep serving (the
// same size and the same end checksums mean the same tile file). A
// tile rewrite that changes the content almost surely changes the
// header counts or the tail tree, so a rebuilt tile never passes a
// stale sidecar.
func (m *Mesh) abstractSidecarOf(key RegionKey) *regionAbstract {
    sidecarPath := filepath.Join(m.dir,
        fmt.Sprintf("%d_%d%s", key.Col, key.Row, abstractFileExt))
    data, err := os.ReadFile(sidecarPath)
    if err != nil {
        return nil
    }
    // The sidecar may carry the zstd wrapping (the pack compression
    // round of issue #11): the magic word decides, the plain sidecars
    // of the older packs keep decoding as they were.
    if isZstdFrame(data) {
        raw, zsErr := decodeZstdFrame(data)
        if zsErr != nil {
            return nil
        }
        data = raw
    }
    abstract, err := DecodeAbstract(data)
    if err != nil || abstract == nil {
        return nil
    }
    tilePath := filepath.Join(m.dir,
        fmt.Sprintf("%d_%d%s", key.Col, key.Row, tileFileExt))
    info, err := os.Stat(tilePath)
    if err != nil {
        return nil
    }
    if info.Size() != abstract.TileSize {
        return nil
    }
    if abstract.TileChecksums {
        if tileChecksumsMatch(tilePath, abstract) {
            return abstract
        }

        return nil
    }
    if !info.ModTime().Equal(abstract.TileModTime) {
        return nil
    }

    return abstract
}

// tileChecksumWindow is the byte count of each file end the sidecar
// checksum covers.
const tileChecksumWindow = 512

// TileChecksumsOf computes the tile head and tail CRC32 over the
// byte windows the sidecar guard checks (the shared definition the
// pack build and the runtime probe compile against).
func TileChecksumsOf(data []byte) (head, tail uint32) {
    headEnd := tileChecksumWindow
    if len(data) < headEnd {
        headEnd = len(data)
    }
    head = crc32.ChecksumIEEE(data[:headEnd])
    tailStart := len(data) - tileChecksumWindow
    if tailStart < 0 {
        tailStart = 0
    }

    return head, crc32.ChecksumIEEE(data[tailStart:])
}

// tileChecksumsMatch reads the two file ends and compares them with
// the sidecar recorded checksums (the ~1 KB read against a multi
// megabyte decode of a wrongly rejected sidecar).
func tileChecksumsMatch(path string, abstract *regionAbstract) bool {
    file, err := os.Open(path)
    if err != nil {
        return false
    }
    defer file.Close()

    head := make([]byte, tileChecksumWindow)
    headN, err := io.ReadFull(file, head)
    if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
        return false
    }
    if crc32.ChecksumIEEE(head[:headN]) != abstract.TileHeadCRC {
        return false
    }
    info, err := file.Stat()
    if err != nil {
        return false
    }
    tailStart := info.Size() - tileChecksumWindow
    if tailStart < 0 {
        tailStart = 0
    }
    if _, err := file.Seek(tailStart, io.SeekStart); err != nil {
        return false
    }
    tail := make([]byte, tileChecksumWindow)
    tailN, err := io.ReadFull(file, tail)
    if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
        return false
    }

    return crc32.ChecksumIEEE(tail[:tailN]) == abstract.TileTailCRC
}

// touchAbstract moves a region to the back of the abstract LRU.
// Callers hold abstractMu.
func (m *Mesh) touchAbstract(key RegionKey) {
    for i, candidate := range m.abstractOrder {
        if candidate == key {
            m.abstractOrder = append(m.abstractOrder[:i],
                m.abstractOrder[i+1:]...)
            m.abstractOrder = append(m.abstractOrder, key)

            return
        }
    }
}

// evictAbstracts drops the oldest abstracts beyond the cache bound.
// Callers hold abstractMu.
func (m *Mesh) evictAbstracts() {
    for m.abstractCapacity > 0 && len(m.abstractOrder) > m.abstractCapacity {
        oldest := m.abstractOrder[0]
        m.abstractOrder = m.abstractOrder[1:]
        delete(m.abstracts, oldest)
    }
}

// buildAbstract scans one decoded tile into its cluster graph: one
// pass over the polygon link chains, every link whose portal crosses
// a cluster boundary becomes an abstract edge. The node set is the
// cluster blocks the polygon footprints touch.
func buildAbstract(tile *Tile) *regionAbstract {
    abstract := &regionAbstract{
        key:   RegionKey{Col: tile.Col, Row: tile.Row},
        nodes: nil,
        index: make(map[clusterID]int32, clustersPerSide*clustersPerSide),
        edges: nil,
        comps: linkComponents(tile),
        polys: len(tile.Polys),
    }
    markClusters(tile, abstract)
    for pi := range tile.Polys {
        poly := &tile.Polys[pi]
        for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
            link := &tile.Links[li]
            current := li
            li = link.Next
            sourceID := clusterOfEdgeSource(tile, poly, link)
            edge, ok := abstractEdgeOf(tile, abstract.comps, int32(pi),
                current, link)
            if !ok {
                continue
            }
            if edge.to.Col == tile.Col && edge.to.Row == tile.Row &&
                edge.to.ID == sourceID {
                // The link stays inside one cluster: the coarse graph
                // does not see it (the refinement searches the real
                // mesh).
                continue
            }
            node := abstract.nodeOf(sourceID)
            if node.classes == nil {
                node.classes = make(map[abstractClassKey]int32)
            }
            class := abstractClassKey{
                to:      edge.to,
                srcArea: edge.srcArea,
                dstArea: edge.dstArea,
                srcComp: edge.srcComp,
            }
            if idx, seen := node.classes[class]; seen {
                // Keep the crossing closest to the side center: the
                // representative of the open span the funnel routes
                // through.
                if edgeCrossingDistance(tile, sourceID, edge) <
                    edgeCrossingDistance(tile, sourceID,
                        abstract.edges[idx]) {
                    abstract.edges[idx] = edge
                }

                continue
            }
            node.classes[class] = int32(len(abstract.edges))
            node.edges = append(node.edges, int32(len(abstract.edges)))
            abstract.edges = append(abstract.edges, edge)
        }
    }

    return abstract
}

// linkComponents labels every polygon of the tile with its link
// component (union find over the internal links; the tile link graph
// is bidirectional by construction, the labels are the strongly
// connected pieces the coarse chain verification needs).
func linkComponents(tile *Tile) []uint32 {
    parent := make([]uint32, len(tile.Polys))
    for i := range parent {
        parent[i] = uint32(i)
    }
    find := func(x uint32) uint32 {
        for parent[x] != x {
            parent[x] = parent[parent[x]]
            x = parent[x]
        }

        return x
    }
    union := func(a, b uint32) {
        ra, rb := find(a), find(b)
        if ra != rb {
            parent[rb] = ra
        }
    }
    for pi := range tile.Polys {
        poly := &tile.Polys[pi]
        for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
            link := &tile.Links[li]
            li = link.Next
            if link.To >= 0 && int(link.To) < len(tile.Polys) {
                union(uint32(pi), uint32(link.To))
            }
        }
    }
    comps := make([]uint32, len(tile.Polys))
    ids := make(map[uint32]uint32)
    for pi := range parent {
        root := find(uint32(pi))
        id, ok := ids[root]
        if !ok {
            id = uint32(len(ids) + 1)
            ids[root] = id
        }
        comps[pi] = id
    }

    return comps
}

// edgeCrossingDistance measures how far a crossing sits from the
// center of the cluster side it leaves through (the dedupe metric:
// the central crossing represents the open span best).
func edgeCrossingDistance(tile *Tile, sourceID clusterID,
    edge abstractEdge,
) float64 {
    cx := int32(sourceID>>4) * clusterCellsSide
    cy := int32(sourceID&0x0F) * clusterCellsSide
    half := clusterCellsSide / 2
    x := float64(cx+half)*cellSizeWorld + tile.WorldMinX()
    y := float64(cy+half)*cellSizeWorld + tile.WorldMinY()
    dx := edge.mid.X - x
    dy := edge.mid.Y - y

    return dx*dx + dy*dy
}

// markClusters creates a node for every cluster block a polygon
// footprint touches (a region wide sea rectangle touches all of them
// - the node set stays complete without a per cell walk).
func markClusters(tile *Tile, abstract *regionAbstract) {
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        x0, x1 := poly.X0/clusterCellsSide,
            (poly.X1-1)/clusterCellsSide
        y0, y1 := poly.Y0/clusterCellsSide,
            (poly.Y1-1)/clusterCellsSide
        for cx := x0; cx <= x1; cx++ {
            for cy := y0; cy <= y1; cy++ {
                abstract.nodeOf(clusterOfCell(cx, cy))
            }
        }
    }
}

// nodeOf returns the node of a cluster id, creating it on first use.
func (a *regionAbstract) nodeOf(id clusterID) *abstractNode {
    if ni, ok := a.index[id]; ok {
        return &a.nodes[ni]
    }
    a.nodes = append(a.nodes, abstractNode{id: id, edges: nil,
        classes: nil})
    a.index[id] = int32(len(a.nodes) - 1)

    return &a.nodes[len(a.nodes)-1]
}

// abstractEdgeOf derives the coarse edge of one mesh link: the
// crossing midpoint, the source and target clusters and the target
// polygon reference (internal links name it directly, external ones
// resolve at refinement time). The link index rides the edge: the
// external target and its component resolve through the source tile's
// link store.
func abstractEdgeOf(tile *Tile, comps []uint32, poly int32,
    linkIdx int32, link *Link,
) (abstractEdge, bool) {
    p := &tile.Polys[poly]
    ax, ay, bx, by := tile.Portal(p, link)
    edge := abstractEdge{
        to:    clusterKey{Col: 0, Row: 0, ID: 0},
        toRef: 0,
        link:  linkIdx,
        mid: Pos{
            X: (ax + bx) * 0.5,
            Y: (ay + by) * 0.5,
            Z: tile.HeightAt(p, (ax+bx)*0.5, (ay+by)*0.5),
        },
        srcComp: comps[poly],
        dstComp: 0,
        srcArea: p.Area,
        dstArea: unknownArea,
    }
    // The cell across the shared edge decides the target cluster.
    cx, cy := edgeSourceCell(p, link)
    switch link.Side {
    case SideMinX:
        cx--
    case SideMaxX:
        cx++
    case SideMinY:
        cy--
    default: // SideMaxY
        cy++
    }
    if link.To >= 0 {
        return abstractInternalEdge(tile, comps, link, edge, cx, cy)
    }

    return abstractExternalEdge(tile, link, edge, cx, cy)
}

// abstractInternalEdge finishes the coarse edge of an internal link:
// the target polygon of the same tile names the target cluster, the
// polygon reference and the surface area (false on a corrupt link).
func abstractInternalEdge(tile *Tile, comps []uint32, link *Link,
    edge abstractEdge, cx, cy int32,
) (abstractEdge, bool) {
    if int(link.To) >= len(tile.Polys) {
        return abstractEdge{to: clusterKey{Col: 0, Row: 0, ID: 0},
            toRef: 0, link: 0, mid: Pos{X: 0, Y: 0, Z: 0},
            srcComp: 0, dstComp: 0, srcArea: 0, dstArea: 0}, false
    }
    target := &tile.Polys[link.To]
    edge.to = clusterKey{
        Col: tile.Col, Row: tile.Row,
        ID: clusterOfCell(clampCell(cx), clampCell(cy)),
    }
    edge.toRef = RefOf(tile.Col, tile.Row, uint32(link.To))
    edge.dstComp = comps[link.To]
    edge.dstArea = target.Area

    return edge, true
}

// abstractExternalEdge finishes the coarse edge of an external link:
// the neighbour cell mirrors onto the opposite border of the
// neighbour region, its cluster folds the border coordinate (false on
// a corrupt or absent link).
func abstractExternalEdge(tile *Tile, link *Link, edge abstractEdge,
    cx, cy int32,
) (abstractEdge, bool) {
    extIdx := -link.To - 1
    if int(extIdx) >= len(tile.ExtLinks) {
        return abstractEdge{to: clusterKey{Col: 0, Row: 0, ID: 0},
            toRef: 0, link: 0, mid: Pos{X: 0, Y: 0, Z: 0},
            srcComp: 0, dstComp: 0, srcArea: 0, dstArea: 0}, false
    }
    ext := &tile.ExtLinks[extIdx]
    if ext.Poly == 0xFFFFFFFF {
        return abstractEdge{to: clusterKey{Col: 0, Row: 0, ID: 0},
            toRef: 0, link: 0, mid: Pos{X: 0, Y: 0, Z: 0},
            srcComp: 0, dstComp: 0, srcArea: 0, dstArea: 0}, false
    }
    ncx, ncy := cx, cy
    switch link.Side {
    case SideMinX:
        ncx = regionCellsSide - 1
    case SideMaxX:
        ncx = 0
    case SideMinY:
        ncy = regionCellsSide - 1
    default: // SideMaxY
        ncy = 0
    }
    edge.to = clusterKey{
        Col: int16(ext.Col), Row: int16(ext.Row),
        ID: clusterOfCell(clampCell(ncx), clampCell(ncy)),
    }
    edge.extCol = int16(ext.Col)
    edge.extRow = int16(ext.Row)
    edge.extPoly = ext.Poly

    return edge, true
}

// clusterOfEdgeSource returns the cluster of the cell just inside the
// source polygon at the crossing (the source node of the edge).
func clusterOfEdgeSource(_ *Tile, p *Poly, link *Link) clusterID {
    cx, cy := edgeSourceCell(p, link)

    return clusterOfCell(clampCell(cx), clampCell(cy))
}

// edgeSourceCell returns the region local cell of the polygon just
// inside the crossing portal (before the step across the side).
func edgeSourceCell(p *Poly, link *Link) (cx, cy int32) {
    span := (link.T0 + link.T1) / 2
    switch link.Side {
    case SideMinX:
        cx, cy = p.X0, span
    case SideMaxX:
        cx, cy = p.X1-1, span
    case SideMinY:
        cx, cy = span, p.Y0
    default: // SideMaxY
        cx, cy = span, p.Y1-1
    }

    return cx, cy
}
