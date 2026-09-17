// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

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
    // hierRouteThreshold is the straight line distance above which a
    // route goes through the hierarchy: four cluster diagonals - the
    // flat search stays the honest answer under it.
    hierRouteThreshold = float64(4 * clusterCellsSide * cellSizeWorld)
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
    mid     Pos
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
// target cluster plus the area pair of the crossing.
type abstractClassKey struct {
    to      clusterKey
    srcArea uint8
    dstArea uint8
}

// regionAbstract is the level 1 graph of one region: the clusters
// with polygons as nodes, the boundary crossing portals as edges.
type regionAbstract struct {
    key   RegionKey
    nodes []abstractNode
    index map[clusterID]int32
    edges []abstractEdge
    // polys is the polygon count of the source tile (the level 0
    // size the doc arithmetic quotes).
    polys int
}

// abstractOf returns the abstract graph of a region, building it from
// the decoded tile on first use. A tile that fails to load answers
// nil (the coarse search treats the region as a wall, the same
// semantics the fine search applies to a missing tile).
func (m *Mesh) abstractOf(key RegionKey) *regionAbstract {
    m.abstractMu.Lock()
    abstract, ok := m.abstracts[key]
    m.abstractMu.Unlock()
    if ok {
        return abstract
    }

    tile, err := m.Tile(key)
    if err != nil || tile == nil {
        m.abstractMu.Lock()
        m.abstracts[key] = nil
        m.abstractMu.Unlock()

        return nil
    }
    abstract = buildAbstract(tile)

    m.abstractMu.Lock()
    // Last writer wins: the build is a deterministic function of the
    // immutable tile, the concurrent duplicate is discarded.
    m.abstracts[key] = abstract
    m.abstractMu.Unlock()

    return abstract
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
        polys: len(tile.Polys),
    }
    markClusters(tile, abstract)
    self := clusterKey{Col: tile.Col, Row: tile.Row, ID: 0}
    for pi := range tile.Polys {
        poly := &tile.Polys[pi]
        for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
            link := &tile.Links[li]
            li = link.Next
            sourceID := clusterOfEdgeSource(tile, poly, link)
            edge, ok := abstractEdgeOf(tile, int32(pi), link)
            if !ok {
                continue
            }
            if edge.to == self && edge.to.ID == sourceID {
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
// resolve at refinement time).
func abstractEdgeOf(tile *Tile, poly int32, link *Link,
) (abstractEdge, bool) {
    p := &tile.Polys[poly]
    ax, ay, bx, by := tile.Portal(p, link)
    edge := abstractEdge{
        to:    clusterKey{Col: 0, Row: 0, ID: 0},
        toRef: 0,
        link:  0,
        mid: Pos{
            X: (ax + bx) * 0.5,
            Y: (ay + by) * 0.5,
            Z: tile.HeightAt(p, (ax+bx)*0.5, (ay+by)*0.5),
        },
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
        return abstractInternalEdge(tile, link, edge, cx, cy)
    }

    return abstractExternalEdge(tile, link, edge, cx, cy)
}

// abstractInternalEdge finishes the coarse edge of an internal link:
// the target polygon of the same tile names the target cluster, the
// polygon reference and the surface area (false on a corrupt link).
func abstractInternalEdge(tile *Tile, link *Link, edge abstractEdge,
    cx, cy int32,
) (abstractEdge, bool) {
    if int(link.To) >= len(tile.Polys) {
        return abstractEdge{to: clusterKey{Col: 0, Row: 0, ID: 0},
            toRef: 0, link: 0, mid: Pos{X: 0, Y: 0, Z: 0},
            srcArea: 0, dstArea: 0}, false
    }
    target := &tile.Polys[link.To]
    edge.to = clusterKey{
        Col: tile.Col, Row: tile.Row,
        ID: clusterOfCell(clampCell(cx), clampCell(cy)),
    }
    edge.toRef = RefOf(tile.Col, tile.Row, uint32(link.To))
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
            srcArea: 0, dstArea: 0}, false
    }
    ext := &tile.ExtLinks[extIdx]
    if ext.Poly == 0xFFFFFFFF {
        return abstractEdge{to: clusterKey{Col: 0, Row: 0, ID: 0},
            toRef: 0, link: 0, mid: Pos{X: 0, Y: 0, Z: 0},
            srcArea: 0, dstArea: 0}, false
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
