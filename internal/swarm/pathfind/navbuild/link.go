// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "sort"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// NSWE wall flags (the Mobius Cell layout, pathfind layer.go): a set
// bit is an open direction.
const (
    nsweEast  uint8 = 1 << 0
    nsweWest  uint8 = 1 << 1
    nsweSouth uint8 = 1 << 2
    nsweNorth uint8 = 1 << 3
)

// linkKey identifies one accumulated link: the source polygon, its
// side and the encoded target (the polygon index for internal links,
// -(ext index + 1) for external ones).
type linkKey struct {
    poly int32
    side uint8
    to   int32
}

// linkAccumulator collects the open portal spans of the polygon
// boundaries: for every link key the cell coordinates along the
// crossing axis where the NSWE walls are open and the height rule
// passes.
type linkAccumulator struct {
    spans map[linkKey][]int32
    // blockedPairs counts the height-compatible neighbour pairs the
    // NSWE walls block - the invisible-wall audit of the research
    // round (the pairs the height-only import would connect).
    blockedPairs int
}

func newLinkAccumulator() *linkAccumulator {
    return &linkAccumulator{
        spans:        make(map[linkKey][]int32),
        blockedPairs: 0,
    }
}

// add records one open cell crossing of one link direction.
func (a *linkAccumulator) add(poly int32, side uint8, to int32,
    span int32,
) {
    key := linkKey{poly: poly, side: side, to: to}
    a.spans[key] = append(a.spans[key], span)
}

// emit turns the accumulated spans into links: per key the sorted
// coordinates merge into maximal runs, every run is one portal. The
// answer is sorted by (poly, side, t0) so the chain assembly is
// deterministic.
func (a *linkAccumulator) emit() []linkSpec {
    specs := make([]linkSpec, 0, len(a.spans))
    for key, coords := range a.spans {
        sort.Slice(coords, func(i, j int) bool {
            return coords[i] < coords[j]
        })
        runStart := coords[0]
        prev := coords[0]
        flush := func(end int32) {
            specs = append(specs, linkSpec{
                poly: key.poly, side: key.side, to: key.to,
                t0: runStart, t1: end,
            })
        }
        for _, c := range coords[1:] {
            if c == prev+1 {
                prev = c

                continue
            }
            flush(prev)
            runStart = c
            prev = c
        }
        flush(prev)
    }
    sort.Slice(specs, func(i, j int) bool {
        if specs[i].poly != specs[j].poly {
            return specs[i].poly < specs[j].poly
        }
        if specs[i].side != specs[j].side {
            return specs[i].side < specs[j].side
        }

        return specs[i].t0 < specs[j].t0
    })

    return specs
}

// linkSpec is one emitted portal: the link of the wire format before
// the chain assembly.
type linkSpec struct {
    poly   int32
    side   uint8
    to     int32
    t0, t1 int32
}

// nsweOpen reports whether the passage from a cell layer to the
// neighbour in (dx, dy) is open: the source wall in the step
// direction AND the target wall in the reverse direction, the
// wallsOpen rule of the grid search.
func nsweOpen(a, b cellLayer, dx, dy int32) bool {
    switch {
    case dy < 0:
        if a.nswe&nsweNorth == 0 || b.nswe&nsweSouth == 0 {
            return false
        }
    case dy > 0:
        if a.nswe&nsweSouth == 0 || b.nswe&nsweNorth == 0 {
            return false
        }
    case dx > 0:
        if a.nswe&nsweEast == 0 || b.nswe&nsweWest == 0 {
            return false
        }
    case dx < 0:
        if a.nswe&nsweWest == 0 || b.nswe&nsweEast == 0 {
            return false
        }
    }

    return true
}

// borderStrip is one region edge: per border position (cy on the
// west/east edges, cx on the north/south) the kept layer instances
// with their heights, walls and polygon indices. The border stitch
// pairs the strips of neighbouring regions.
type borderStrip struct {
    offsets [regionCellsSide + 1]uint32
    entries []borderEntry
}

// borderEntry is one kept layer of one border cell.
type borderEntry struct {
    h    int16
    nswe uint8
    poly uint32
}

// borderStrips holds the four region edges.
type borderStrips struct {
    strips [4]borderStrip
}

// The strip side indices: the west and east strips run over cy, the
// north and south strips over cx.
const (
    stripWest  = 0
    stripEast  = 1
    stripNorth = 2
    stripSouth = 3
)

// stripSide maps an outward step direction to the strip it leaves.
func stripSide(dx, dy int32) int {
    switch {
    case dx < 0:
        return stripWest
    case dx > 0:
        return stripEast
    case dy < 0:
        return stripNorth
    default:
        return stripSouth
    }
}

// buildInternalLinks walks the adjacent cell layer pairs of the whole
// region and accumulates the open portal spans between the polygons;
// the region border cells land in the border strips for the external
// stitching. The walk visits the east and south neighbours only (the
// reverse link directions accumulate from the same visit), the west
// and north borders collect their strips explicitly.
func buildInternalLinks(rl *regionLayers, sh *sheets,
    polyAt []int32, climb int32,
) (*linkAccumulator, borderStrips) {
    acc := newLinkAccumulator()
    var strips borderStrips
    for cx := range regionCellsSide {
        for cy := range regionCellsSide {
            if cx == 0 {
                strips.collect(rl, sh, polyAt, cx, cy, -1, 0)
            }
            if cy == 0 {
                strips.collect(rl, sh, polyAt, cx, cy, 0, -1)
            }
            walk := &linkWalker{
                acc: acc, sh: sh, polyAt: polyAt, climb: climb,
            }
            if cx+1 < regionCellsSide {
                walk.pair(rl, cx, cy, 1, 0)
            } else {
                strips.collect(rl, sh, polyAt, cx, cy, 1, 0)
            }
            if cy+1 < regionCellsSide {
                walk.pair(rl, cx, cy, 0, 1)
            } else {
                strips.collect(rl, sh, polyAt, cx, cy, 0, 1)
            }
        }
    }
    strips.finalize()

    return acc, strips
}

// linkWalker relaxes the layer pairs of two adjacent cells.
type linkWalker struct {
    acc    *linkAccumulator
    sh     *sheets
    polyAt []int32
    climb  int32
}

// pair walks the layer cross product of the cell and its neighbour in
// the step direction, accumulating the open crossings.
func (w *linkWalker) pair(rl *regionLayers, cx, cy int, dx, dy int) {
    idx := cx*regionCellsSide + cy
    off := int(rl.cellOff[idx])
    cnt := int(rl.cellCnt[idx])
    nIdx := (cx+dx)*regionCellsSide + cy + dy
    nOff := int(rl.cellOff[nIdx])
    nCnt := int(rl.cellCnt[nIdx])
    span := int32(cy)
    sideA := navmesh.SideMaxX
    sideB := navmesh.SideMinX
    if dy != 0 {
        span = int32(cx)
        sideA = navmesh.SideMaxY
        sideB = navmesh.SideMinY
    }
    for k := off; k < off+cnt; k++ {
        if !w.kept(k) {
            continue
        }
        for m := nOff; m < nOff+nCnt; m++ {
            if !w.kept(m) {
                continue
            }
            // The cell pairs inside one polygon need no link: the
            // interior is walkable by construction.
            if w.polyAt[k] == w.polyAt[m] {
                continue
            }
            a := rl.layers[k]
            b := rl.layers[m]
            if abs16(a.h-b.h) > w.climb {
                continue
            }
            if !nsweOpen(a, b, int32(dx), int32(dy)) {
                w.acc.blockedPairs++

                continue
            }
            w.acc.add(w.polyAt[k], sideA, w.polyAt[m], span)
            w.acc.add(w.polyAt[m], sideB, w.polyAt[k], span)
        }
    }
}

// kept reports whether the layer instance belongs to a kept sheet.
func (w *linkWalker) kept(k int) bool {
    sheet := w.sh.sheetOf[k]

    return sheet >= 0 && !w.sh.dropped[sheet]
}

// collect appends the kept layers of one border cell to its strip.
func (s *borderStrips) collect(rl *regionLayers, sh *sheets,
    polyAt []int32, cx, cy int, dx, dy int,
) {
    side := stripSide(int32(dx), int32(dy))
    strip := &s.strips[side]
    pos := cy
    if dy != 0 {
        pos = cx
    }
    idx := cx*regionCellsSide + cy
    off := int(rl.cellOff[idx])
    cnt := int(rl.cellCnt[idx])
    for k := off; k < off+cnt; k++ {
        sheet := sh.sheetOf[k]
        if sheet < 0 || sh.dropped[sheet] || polyAt[k] < 0 {
            continue
        }
        strip.entries = append(strip.entries, borderEntry{
            h:    rl.layers[k].h,
            nswe: rl.layers[k].nswe,
            poly: uint32(polyAt[k]),
        })
        strip.offsets[pos+1]++
    }
}

// finalize turns the per-position entry counts into prefix offsets.
func (s *borderStrips) finalize() {
    for side := range s.strips {
        strip := &s.strips[side]
        for pos := 1; pos <= regionCellsSide; pos++ {
            strip.offsets[pos] += strip.offsets[pos-1]
        }
    }
}

// stitchBorders computes the external links of one region against the
// border strips of its neighbours: every position of a shared edge
// pairs the kept layers of both sides, the open and height-compatible
// pairs accumulate portal spans into links that target the neighbour
// region polygons. The neighbours array is indexed by the own side:
// neighbors[stripWest] holds the strips of the region to the west and
// its EAST strip pairs with the own west strip. The own links are
// appended to the tile and the ExtLinks array grows accordingly; the
// caller re-encodes the tile.
func stitchBorders(tile *navmesh.Tile, own borderStrips,
    neighbors [4]*borderStrips, climb int32,
) int {
    added := 0
    added += stitchSide(tile, &own.strips[stripWest],
        neighborStrip(neighbors, stripWest, stripEast),
        tile.Col-1, tile.Row, navmesh.SideMinX, -1, 0, climb)
    added += stitchSide(tile, &own.strips[stripEast],
        neighborStrip(neighbors, stripEast, stripWest),
        tile.Col+1, tile.Row, navmesh.SideMaxX, 1, 0, climb)
    added += stitchSide(tile, &own.strips[stripNorth],
        neighborStrip(neighbors, stripNorth, stripSouth),
        tile.Col, tile.Row-1, navmesh.SideMinY, 0, -1, climb)
    added += stitchSide(tile, &own.strips[stripSouth],
        neighborStrip(neighbors, stripSouth, stripNorth),
        tile.Col, tile.Row+1, navmesh.SideMaxY, 0, 1, climb)

    return added
}

// neighborStrip picks the opposite strip of one neighbour.
func neighborStrip(neighbors [4]*borderStrips, ownSide, theirSide int,
) *borderStrip {
    if neighbors[ownSide] == nil {
        return nil
    }

    return &neighbors[ownSide].strips[theirSide]
}

// stitchSide pairs one own strip against the neighbour strip of the
// opposite edge and appends the external links to the tile.
func stitchSide(tile *navmesh.Tile, own, neighbor *borderStrip,
    col, row int16, side uint8,
    dx, dy int32, climb int32,
) int {
    if neighbor == nil {
        return 0
    }
    acc := newLinkAccumulator()
    for pos := range regionCellsSide {
        ownFrom := int(own.offsets[pos])
        ownTo := int(own.offsets[pos+1])
        nbFrom := int(neighbor.offsets[pos])
        nbTo := int(neighbor.offsets[pos+1])
        for i := ownFrom; i < ownTo; i++ {
            a := own.entries[i]
            for j := nbFrom; j < nbTo; j++ {
                b := neighbor.entries[j]
                if abs16(a.h-b.h) > climb {
                    continue
                }
                if !nsweOpen(cellLayer{h: a.h, nswe: a.nswe},
                    cellLayer{h: b.h, nswe: b.nswe}, dx, dy) {
                    continue
                }
                target := extTargetIndex(tile, col, row, b.poly)
                acc.add(int32(a.poly), side, target, int32(pos))
            }
        }
    }
    specs := acc.emit()
    for _, spec := range specs {
        appendLink(tile, spec)
    }

    return len(specs)
}

// extTargetIndex returns the encoded link target of one neighbor
// polygon, adding it to the tile ExtLinks on first use.
func extTargetIndex(tile *navmesh.Tile, col, row int16,
    poly uint32,
) int32 {
    for i, ext := range tile.ExtLinks {
        if ext.Col == int32(col) && ext.Row == int32(row) &&
            ext.Poly == poly {
            return -(int32(i) + 1)
        }
    }
    tile.ExtLinks = append(tile.ExtLinks, navmesh.ExtLink{
        Col: int32(col), Row: int32(row), Poly: poly,
    })

    return -int32(len(tile.ExtLinks))
}

// appendLink chains one link spec into the tile (walking the source
// polygon chain to its tail).
func appendLink(tile *navmesh.Tile, spec linkSpec) {
    link := navmesh.Link{
        Side: spec.side, To: spec.to, Next: -1,
        T0: spec.t0, T1: spec.t1,
    }
    tile.Links = append(tile.Links, link)
    idx := int32(len(tile.Links) - 1)
    poly := &tile.Polys[spec.poly]
    if poly.FirstLink < 0 {
        poly.FirstLink = idx

        return
    }
    chain := poly.FirstLink
    for tile.Links[chain].Next >= 0 {
        chain = tile.Links[chain].Next
    }
    tile.Links[chain].Next = idx
}
