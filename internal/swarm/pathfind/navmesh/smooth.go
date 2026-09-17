// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"math"
	"sort"
)

// The shortcut pass of the route answer (the smoothing): the funnel
// on the exact square mesh pivots at every portal the clearance
// shrinks into a pinhole, so a long walk turns at every height run
// edge and the walker micro steers through hundreds of waypoints (the
// owner report: the bot hooks and sticks on the dense turns). The
// pass merges the funnel waypoints into the longest chords the
// corridor geometry allows:
//
//  1. the chord from waypoint i to waypoint k must cross every
//     intermediate portal inside its open span - a crossing outside
//     the span is a wall the server movement validation refuses (the
//     paired NSWE walls and the anti corner cut);
//  2. the chord must keep the clearance radius away from every wall
//     edge of the polygons it passes: the walls are the side portions
//     without a link (the complement of the open spans), the same
//     closed edges the capsule clearance pass pushes away from. A
//     straight leg closer than the radius to a wall edge sweeps the
//     capsule into it - the hook the pivot offset removed from the
//     turns comes back through the merged legs.
//
// The open spans of this mesh are whole geodata cells (16 units), so
// every span holds a crossing a 7.5 capsule clears (16 > 2 * 7.5) and
// the pass never dead ends. The merged answer keeps the funnel
// waypoints it could not merge, so it never adds a turn and never
// leaves the corridor.

// smoothEpsilon relaxes the wall distance rule by a hair: the funnel
// pivots sit exactly one radius off the wall edges (offsetPortal) and
// the chords through them must survive the float rounding of the
// distance comparison.
const smoothEpsilon = 1e-6

// smoothScanWindow caps how far the greedy scan walks back per anchor
// before it accepts the next waypoint: a bound on the worst case
// work of one merge attempt round (the deep far-end probes that fail
// early dominate the cost, the cap keeps the pass inside the search
// budget of the caller).
const smoothScanWindow = 256

// smoothPath merges the funnel waypoints into the longest safe chords
// (greedy farthest visible): the answer holds a subset of the input
// positions - the first, the last and every pivot no safe chord
// skips.
func (m *Mesh) smoothPath(corridor []PolyRef, wps []funnelWp,
	filter Filter,
) []Pos {
	clearance := filter.WaypointClearance
	if len(wps) == 0 {
		return nil
	}
	if len(wps) < 3 || clearance <= 0 || len(corridor) < 2 {
		return funnelPositions(wps)
	}

	walls := make(map[PolyRef]*[4][]wallSpan, len(corridor))
	merged := make([]Pos, 0, len(wps))
	merged = append(merged, wps[0].pos)
	anchor := 0
	for anchor < len(wps)-1 {
		far := anchor + 1
		if last := len(wps) - 1; far+smoothScanWindow < last {
			far += smoothScanWindow
		} else {
			far = last
		}
		chosen := anchor + 1
		for k := far; k > anchor+1; k-- {
			if m.chordClear(corridor, wps[anchor], wps[k], filter,
				walls) {
				chosen = k

				break
			}
		}
		merged = append(merged, wps[chosen].pos)
		anchor = chosen
	}

	return merged
}

// chordClear answers whether the straight chord from the waypoint
// "from" to the waypoint "to" walks the corridor safely: it crosses
// every intermediate portal inside the open span, and the walls keep
// the clearance - the armed guard (the server accurate raster)
// answers the whole chord, the mesh wall spans answer per polygon.
func (m *Mesh) chordClear(corridor []PolyRef, from, to funnelWp,
	filter Filter, walls map[PolyRef]*[4][]wallSpan,
) bool {
	clearance := filter.WaypointClearance
	start := int(from.portal) + 1
	if start < 0 {
		start = 0
	}
	end := int(to.portal)
	if end > len(corridor)-1 {
		end = len(corridor) - 1
	}
	if start > end {
		return false
	}
	if filter.Guard != nil && !filter.Guard.LegClear(
		from.pos.X, from.pos.Y, from.pos.Z,
		to.pos.X, to.pos.Y, to.pos.Z, clearance) {
		return false
	}

	current := from.pos
	for q := start; q < end; q++ {
		tile, poly, link, ok := m.linkBetween(corridor[q], corridor[q+1])
		if !ok {
			return false
		}
		ax, ay, bx, by := tile.Portal(poly, link)
		x, y, crossed := segmentCrossing(current.X, current.Y,
			to.pos.X, to.pos.Y, ax, ay, bx, by)
		if !crossed {
			return false
		}
		if filter.Guard == nil {
			spans := wallSpansOf(m, corridor[q], walls)
			if spans == nil {
				return false
			}
			// The pass is 2D: the crossing height rides on the chord
			// endpoints, the exit carries none.
			exit := Pos{X: x, Y: y, Z: 0}
			if !polyWallClear(spans, current, exit, clearance) {
				return false
			}
		}
		current = Pos{X: x, Y: y, Z: current.Z}
	}
	if filter.Guard != nil {
		return true
	}
	spans := wallSpansOf(m, corridor[end], walls)

	return spans != nil && polyWallClear(spans, current, to.pos,
		clearance)
}

// wallSpan is one closed wall portion of a polygon side in world
// coordinates (the segment form the distance check consumes).
type wallSpan struct {
	ax, ay, bx, by float64
}

// wallSpansOf returns the wall spans of one polygon, computing them
// once per shortcut pass (the cache holds every corridor polygon).
func wallSpansOf(m *Mesh, ref PolyRef,
	walls map[PolyRef]*[4][]wallSpan,
) *[4][]wallSpan {
	if cached, ok := walls[ref]; ok {
		return cached
	}
	spans := m.computeWallSpans(ref)
	walls[ref] = spans

	return spans
}

// computeWallSpans derives the closed wall portions of the polygon
// sides: every side spans the whole rectangle edge; the link chain
// carves the open spans out of it; the complement is the wall. An
// unlinked portion is an edge the server movement validation blocks -
// the NSWE wall or the height step beyond the climb limit - the same
// closed edges the capsule clearance pass treats as obstacles.
func (m *Mesh) computeWallSpans(ref PolyRef) *[4][]wallSpan {
	tile, poly := m.polyOfRef(ref)
	if tile == nil || poly == nil {
		return nil
	}

	var open [4][][2]int32
	for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
		link := &tile.Links[li]
		li = link.Next
		open[link.Side] = append(open[link.Side],
			[2]int32{link.T0, link.T1 + 1})
	}

	spans := &[4][]wallSpan{}
	for _, side := range []uint8{SideMinX, SideMaxX, SideMinY, SideMaxY} {
		lo, hi := poly.X0, poly.X1
		if side == SideMinX || side == SideMaxX {
			lo, hi = poly.Y0, poly.Y1
		}
		for _, gap := range complementSpans(lo, hi, open[side]) {
			spans[side] = append(spans[side],
				tile.sideSpanSegment(side, poly, gap[0], gap[1]))
		}
	}

	return spans
}

// sideSpanSegment maps a cell range of one polygon side onto the
// world segment of the rectangle edge (the side fixes the axis and
// the coordinate, the cell range walks the other axis).
func (t *Tile) sideSpanSegment(side uint8, poly *Poly, c0, c1 int32,
) wallSpan {
	wx := func(c int32) float64 {
		return t.worldMinX + float64(c)*cellSizeWorld
	}
	wy := func(c int32) float64 {
		return t.worldMinY + float64(c)*cellSizeWorld
	}

	switch side {
	case SideMinX:
		return wallSpan{wx(poly.X0), wy(c0), wx(poly.X0), wy(c1)}
	case SideMaxX:
		return wallSpan{wx(poly.X1), wy(c0), wx(poly.X1), wy(c1)}
	case SideMinY:
		return wallSpan{wx(c0), wy(poly.Y0), wx(c1), wy(poly.Y0)}
	default: // SideMaxY
		return wallSpan{wx(c0), wy(poly.Y1), wx(c1), wy(poly.Y1)}
	}
}

// complementSpans returns the gaps the open spans leave inside the
// cell range [lo, hi): the closed portions of the side.
func complementSpans(lo, hi int32, open [][2]int32) [][2]int32 {
	if hi <= lo {
		return nil
	}
	sorted := make([][2]int32, len(open))
	copy(sorted, open)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i][0] < sorted[j][0]
	})

	gaps := make([][2]int32, 0, len(sorted)+1)
	cursor := lo
	for _, span := range sorted {
		if span[1] <= cursor {
			continue
		}
		if span[0] > cursor {
			end := span[0]
			if end > hi {
				end = hi
			}
			gaps = append(gaps, [2]int32{cursor, end})
		}
		if span[1] > cursor {
			cursor = span[1]
		}
		if cursor >= hi {
			break
		}
	}
	if cursor < hi {
		gaps = append(gaps, [2]int32{cursor, hi})
	}

	return gaps
}

// polyWallClear answers whether the segment keeps the clearance from
// every wall span of the polygon (2D: the capsule sweeps the leg, a
// wall edge closer than the radius eats the capsule side).
func polyWallClear(spans *[4][]wallSpan, a, b Pos, clearance float64,
) bool {
	limit := clearance - smoothEpsilon
	if limit <= 0 {
		return true
	}
	limitSq := limit * limit
	for side := range spans {
		for _, span := range spans[side] {
			if segSegDistSqr2D(a.X, a.Y, b.X, b.Y,
				span.ax, span.ay, span.bx, span.by) < limitSq {
				return false
			}
		}
	}

	return true
}

// segmentCrossing returns the intersection point of the segments ab
// and cd when they properly cross (the chord through the portal
// span). Parallel and collinear pairs answer false: a chord running
// along the portal edge is no crossing.
func segmentCrossing(ax, ay, bx, by, cx, cy, dx, dy float64,
) (float64, float64, bool) {
	rx, ry := bx-ax, by-ay
	sx, sy := dx-cx, dy-cy
	denom := rx*sy - ry*sx
	if math.Abs(denom) < 1e-12 {
		return 0, 0, false
	}
	t := ((cx-ax)*sy - (cy-ay)*sx) / denom
	s := ((cx-ax)*ry - (cy-ay)*rx) / denom
	const slack = 1e-9
	if t < -slack || t > 1+slack || s < -slack || s > 1+slack {
		return 0, 0, false
	}

	return ax + t*rx, ay + t*ry, true
}

// segSegDistSqr2D is the squared distance between two 2D segments
// (the clamped closest points of Ericson's Real-Time Collision
// Detection, the degenerate cases included).
func segSegDistSqr2D(ax, ay, bx, by, cx, cy, dx, dy float64) float64 {
	d1x, d1y := bx-ax, by-ay
	d2x, d2y := dx-cx, dy-cy
	rx, ry := ax-cx, ay-cy
	a := d1x*d1x + d1y*d1y
	e := d2x*d2x + d2y*d2y
	f := d2x*rx + d2y*ry

	const tiny = 1e-12
	var s, t float64
	switch {
	case a <= tiny && e <= tiny:
		s, t = 0, 0
	case a <= tiny:
		s = 0
		t = clamp01(f / e)
	case e <= tiny:
		t = 0
		c := d1x*rx + d1y*ry
		s = clamp01(-c / a)
	default:
		b := d1x*d2x + d1y*d2y
		denom := a*e - b*b
		c := d1x*rx + d1y*ry
		if denom > tiny {
			s = clamp01((b*f - c*e) / denom)
		}
		t = (b*s + f) / e
		if t < 0 {
			t = 0
			s = clamp01(-c / a)
		} else if t > 1 {
			t = 1
			s = clamp01((b - c) / a)
		}
	}

	px := ax + d1x*s - (cx + d2x*t)
	py := ay + d1y*s - (cy + d2y*t)

	return px*px + py*py
}

// clamp01 clamps the value into [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}

	return v
}
