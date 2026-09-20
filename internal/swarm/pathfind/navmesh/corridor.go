// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// corridorSlack is the priced deviation the shortcut region grows
// beyond the search corridor: the slope strips of the merged mesh
// staircase (the height run edges step the strips one cell aside),
// the corridor chain threads them one strip wide and the funnel
// through the chain pivots at the span ends the sliding portals
// force - the walk the terrain allows cuts the staircase diagonally
// through the strips the chain left out. The flood prices the
// deviation like the search step and caps it at the slack, the
// shortcut chords thread the grown region through the real link
// spans only (the owner zigzag report of the raw answer).
const corridorSlack = 96

// smoothScanWindow caps how far the greedy scan walks back per anchor
// before it accepts the next waypoint: a bound on the worst case
// work of one merge attempt round (the deep far-end probes that fail
// early dominate the cost, the cap keeps the pass inside the search
// budget of the caller).
const smoothScanWindow = 256

// corridorRegion grows the search corridor into the walk region the
// shortcut chords may thread: a multi source flood from the chain
// polygons over the open links, priced like the search step (the
// entry distance times the averaged polygon prices, polyCost) and
// capped at the slack over the free chain. The avoid queries keep
// the bare chain (the bans wall the search, the region must not
// reopen the banned ground the chain already avoided).
//
//nolint:cyclop,gocognit,funlen // the portal and wall checks
func (m *Mesh) corridorRegion(corridor []PolyRef, filter Filter,
) map[PolyRef]struct{} {
    region := make(map[PolyRef]struct{}, len(corridor)*2)
    for _, ref := range corridor {
        region[ref] = struct{}{}
    }
    if len(filter.Avoid) > 0 || len(corridor) < 2 {
        return region
    }
    var zones *zoneIndex
    if len(filter.WaterZones) > 0 {
        zones = newZoneIndex(filter.WaterZones)
    }
    type entry struct {
        ref PolyRef
        acc float64
    }
    frontier := make([]entry, 0, len(corridor)*4)
    for _, ref := range corridor {
        frontier = append(frontier, entry{ref, 0})
    }
    for len(frontier) > 0 {
        // The linear minimum pops the cheapest frontier entry (the
        // frontier of one region growth stays small, the heap is
        // not worth the code).
        best := 0
        for i := 1; i < len(frontier); i++ {
            if frontier[i].acc < frontier[best].acc {
                best = i
            }
        }
        current := frontier[best]
        frontier[best] = frontier[len(frontier)-1]
        frontier = frontier[:len(frontier)-1]
        if current.acc > corridorSlack {
            continue
        }
        tile, poly := m.polyOfRef(current.ref)
        if tile == nil || poly == nil {
            continue
        }
        fromCost := polyCost(tile, poly, zones, filter)
        for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
            link := &tile.Links[li]
            li = link.Next
            targetRef, targetTile, targetPoly := m.linkTarget(tile,
                link)
            if targetTile == nil {
                continue
            }
            if _, seen := region[targetRef]; seen {
                continue
            }
            targetCost := polyCost(targetTile, targetPoly, zones,
                filter)
            ax, ay, bx, by := tile.Portal(poly, link)
            mid := Pos{X: (ax + bx) * 0.5, Y: (ay + by) * 0.5,
                Z: tile.HeightAt(poly, (ax+bx)*0.5, (ay+by)*0.5)}
            cx0, cy0 := rectCenter(tile, poly)
            step := math.Hypot(mid.X-cx0, mid.Y-cy0) *
                (fromCost + targetCost) * 0.5
            acc := current.acc + step
            if acc > corridorSlack {
                continue
            }
            region[targetRef] = struct{}{}
            frontier = append(frontier, entry{targetRef, acc})
        }
    }

    return region
}

// regionChord answers whether the straight segment walks the region:
// from the start polygon it exits every rectangle through an open
// link span whose span holds the crossing, every polygon it enters
// belongs to the region, and the visited polygon list rides along
// for the wall clearance check of the caller. The walk crosses only
// the links of the polygon it leaves, so the stacked layers stay on
// the connected surface.
//
//nolint:cyclop,gocognit,funlen // the wall and guard branches
func (m *Mesh) regionChord(startRef PolyRef, a, b Pos,
    region map[PolyRef]struct{}, visited *[]PolyRef,
) bool {
    current := startRef
    cx, cy := a.X, a.Y
    *visited = (*visited)[:0]
    const eps = 1e-9
    for range 1024 {
        if _, ok := region[current]; !ok {
            return false
        }
        tile, poly := m.polyOfRef(current)
        if tile == nil || poly == nil {
            return false
        }
        *visited = append(*visited, current)
        x0, y0, x1, y1 := tile.WorldRect(poly)
        if b.X >= x0-eps && b.X <= x1+eps && b.Y >= y0-eps && b.Y <= y1+eps {
            return true
        }
        dx, dy := b.X-cx, b.Y-cy
        exitT := math.MaxFloat64
        var exitSide uint8
        var exitX, exitY float64
        for _, side := range [4]uint8{SideMinX, SideMaxX, SideMinY,
            SideMaxY} {
            var t float64
            switch side {
            case SideMinX:
                if math.Abs(dx) < 1e-12 {
                    continue
                }
                t = (x0 - cx) / dx
            case SideMaxX:
                if math.Abs(dx) < 1e-12 {
                    continue
                }
                t = (x1 - cx) / dx
            case SideMinY:
                if math.Abs(dy) < 1e-12 {
                    continue
                }
                t = (y0 - cy) / dy
            default: // SideMaxY
                if math.Abs(dy) < 1e-12 {
                    continue
                }
                t = (y1 - cy) / dy
            }
            if t <= eps || t >= exitT {
                continue
            }
            px, py := cx+t*dx, cy+t*dy
            if px < x0-eps || px > x1+eps || py < y0-eps || py > y1+eps {
                continue
            }
            exitT = t
            exitSide = side
            exitX, exitY = px, py
        }
        if exitT == math.MaxFloat64 {
            return false
        }
        next := m.regionCross(tile, poly, exitSide, exitX, exitY)
        if next == 0 {
            return false
        }
        current = next
        cx, cy = exitX, exitY
    }

    return false
}

// regionCross resolves the polygon across the open link whose span
// holds the crossing point on the given side (zero: the crossing is
// a wall or outside every span - the chord may not pass).
func (m *Mesh) regionCross(tile *Tile, poly *Poly, side uint8,
    x, y float64,
) PolyRef {
    const eps = 1e-7
    for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
        link := &tile.Links[li]
        li = link.Next
        if link.Side != side {
            continue
        }
        lo, hi := tile.worldMinY+float64(link.T0)*cellSizeWorld,
            tile.worldMinY+float64(link.T1+1)*cellSizeWorld
        if side == SideMinY || side == SideMaxY {
            lo = tile.worldMinX + float64(link.T0)*cellSizeWorld
            hi = tile.worldMinX + float64(link.T1+1)*cellSizeWorld
        }
        along := y
        if side == SideMinY || side == SideMaxY {
            along = x
        }
        if along < lo-eps || along > hi+eps {
            continue
        }
        targetRef, _, targetPoly := m.linkTarget(tile, link)
        if targetPoly == nil {
            continue
        }

        return targetRef
    }

    return 0
}

// shortenCorridorWaypoints folds the funnel waypoints into the
// longest chords the corridor region allows (the greedy farthest
// visible walk of the shortcut pass): a chord must walk the region
// (regionChord: every crossed span open, every entered polygon in
// the region) and keep the clearance from the walls of the polygons
// it visits - the armed guard answers the walls instead of the mesh
// spans (the server accurate raster). The answer holds a subset of
// the funnel positions: the first, the last and every pivot no safe
// chord skips.
func (m *Mesh) shortenCorridorWaypoints(corridor []PolyRef,
    wps []funnelWp, filter Filter,
) []Pos {
    if len(wps) == 0 {
        return nil
    }
    clearance := filter.WaypointClearance
    if len(wps) < 3 || clearance <= 0 || len(corridor) < 2 {
        return funnelPositions(wps)
    }
    region := m.corridorRegion(corridor, filter)
    walls := make(map[PolyRef]*[4][]wallSpan, len(region))
    merged := make([]Pos, 0, len(wps))
    merged = append(merged, wps[0].pos)
    anchor := 0
    visited := make([]PolyRef, 0, 32)
    for anchor < len(wps)-1 {
        far := anchor + 1
        if last := len(wps) - 1; far+smoothScanWindow < last {
            far += smoothScanWindow
        } else {
            far = last
        }
        chosen := anchor + 1
        for k := far; k > anchor+1; k-- {
            if !m.chordWalksRegion(corridor, wps[anchor], wps[k],
                region, walls, &visited, filter) {
                continue
            }
            chosen = k

            break
        }
        merged = append(merged, wps[chosen].pos)
        anchor = chosen
    }

    return merged
}

// chordWalksRegion answers whether the straight chord from the
// waypoint "from" to the waypoint "to" walks the corridor region
// safely: the region chord crosses only open spans of region
// polygons, and the walls keep the clearance - the armed guard (the
// server accurate raster) answers the whole chord instead of the
// mesh wall spans.
func (m *Mesh) chordWalksRegion(corridor []PolyRef, from, to funnelWp,
    region map[PolyRef]struct{}, walls map[PolyRef]*[4][]wallSpan,
    visited *[]PolyRef, filter Filter,
) bool {
    start := int(from.portal)
    if start < 0 {
        start = 0
    }
    if !m.regionChord(corridor[start], from.pos, to.pos, region,
        visited) {
        return false
    }
    if filter.Guard != nil {
        return filter.Guard.SegmentClear(from.pos.X, from.pos.Y,
            from.pos.Z, to.pos.X, to.pos.Y, to.pos.Z,
            filter.WaypointClearance)
    }
    for _, ref := range *visited {
        spans := wallSpansOf(m, ref, walls)
        if spans == nil {
            return false
        }
        if !polyWallClear(spans, from.pos, to.pos,
            filter.WaypointClearance) {
            return false
        }
    }

    return true
}
