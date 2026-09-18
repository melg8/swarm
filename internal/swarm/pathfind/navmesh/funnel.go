// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// funnelEpsilon is the 2D coincidence threshold of the funnel: the
// portal advance guard and the waypoint deduplication.
const funnelEpsilon = 1e-4

// funnelWp is one waypoint of the raw funnel answer: the position and
// the corridor index of the portal the waypoint sits on (-1 for the
// start projection before the first portal, len(corridor)-1 for the
// end projection). The portal index is the corridor walk address the
// shortcut pass replays: a chord leaving the waypoint crosses the
// portals after it, a chord arriving at it ends on its portal edge.
type funnelWp struct {
    pos    Pos
    portal int32
}

// straightPath turns a polygon corridor into walk waypoints through
// the funnel algorithm of dtNavMeshQuery::findStraightPath: the
// portals of the corridor (the open spans of the shared edges) narrow
// a left/right cone from the start until it inverts, every inversion
// appends the funnel apex as a turning point. The waypoints never
// leave the corridor polygons and only cross shared edges inside
// their open portal spans - the NSWE wall fidelity of the mesh.
// The clearance radius pulls every pivot inward from the span ends
// (offsetPortal) so a turning waypoint keeps the character capsule
// away from the walls the span end sits on.
//
// The corridor must be a connected polygon chain (the A* answer). The
// start and end positions snap onto the corridor surface like
// closestPointOnPolyBoundary does in Detour: the end of a partial
// corridor projects onto its last polygon, which is exactly the
// closest-reachable-point semantics of the dry searches.
func (m *Mesh) straightPath(corridor []PolyRef, startPos, endPos Pos,
    clearance float64,
) []Pos {
    return funnelPositions(m.straightPathWps(corridor, startPos, endPos,
        clearance))
}

// funnelPositions strips the funnel waypoints down to their positions.
func funnelPositions(wps []funnelWp) []Pos {
    positions := make([]Pos, len(wps))
    for i, wp := range wps {
        positions[i] = wp.pos
    }

    return positions
}

// straightPathWps is the funnel answer with the corridor walk
// addresses (the portal index of every waypoint) the shortcut pass
// replays.
func (m *Mesh) straightPathWps(corridor []PolyRef, startPos, endPos Pos,
    clearance float64,
) []funnelWp {
    if len(corridor) == 0 {
        return nil
    }
    firstTile, firstPoly := m.polyOfRef(corridor[0])
    if firstTile == nil {
        return nil
    }
    sx, sy, sz := firstTile.ClosestPoint(firstPoly, startPos.X,
        startPos.Y, startPos.Z)
    closestStart := Pos{X: sx, Y: sy, Z: sz}
    lastTile, lastPoly := m.polyOfRef(corridor[len(corridor)-1])
    if lastTile == nil {
        return nil
    }
    ex, ey, ez := lastTile.ClosestPoint(lastPoly, endPos.X,
        endPos.Y, endPos.Z)
    closestEnd := Pos{X: ex, Y: ey, Z: ez}

    waypoints := make([]funnelWp, 0, len(corridor)+1)
    waypoints = appendWp(waypoints, closestStart, -1)

    if len(corridor) > 1 {
        m.runFunnel(corridor, closestStart, closestEnd, clearance,
            &waypoints)
    }

    waypoints = appendWp(waypoints, closestEnd,
        int32(len(corridor)-1))

    return waypoints
}

// runFunnel is the funnel state machine over the corridor portals.
//
// The cone side labels follow the local travel direction (the
// left/right of the corridor as it crosses each portal); the signed
// area convention here is the standard cross product (positive = the
// third point lies counter-clockwise left of the first-two line),
// which is the negative of the Detour dtTriArea2D - every comparison
// of the upstream algorithm flips with it.
func (m *Mesh) runFunnel(corridor []PolyRef, closestStart, closestEnd Pos,
    clearance float64, waypoints *[]funnelWp,
) {
    cone := funnelCone{
        apex:       closestStart,
        left:       closestStart,
        right:      closestStart,
        apexIndex:  0,
        leftIndex:  0,
        rightIndex: 0,
    }

    i := 0
    for i < len(corridor) {
        left, right, ok := m.corridorPortal(corridor, i, closestEnd,
            clearance)
        if !ok {
            // A broken chain ends the walk at the last connected
            // polygon: the corridor is inconsistent (should not
            // happen on a built mesh).
            return
        }
        if i == 0 &&
            distPtSegSqr2D(cone.apex, left, right) < funnelEpsilon {
            i++

            continue
        }
        if apex, inverted := cone.narrowRight(right, i); inverted {
            *waypoints = appendWp(*waypoints, apex, int32(cone.apexIndex))
            i = cone.apexIndex + 1

            continue
        }
        if apex, inverted := cone.narrowLeft(left, i); inverted {
            *waypoints = appendWp(*waypoints, apex, int32(cone.apexIndex))
            i = cone.apexIndex + 1

            continue
        }
        i++
    }
}

// corridorPortal returns the left and right portal points of the
// corridor transition at index i (the shared edge between the
// polygons i and i+1, or the end position at the corridor end),
// pulled inward by the clearance radius (offsetPortal).
func (m *Mesh) corridorPortal(corridor []PolyRef, i int, closestEnd Pos,
    clearance float64,
) (Pos, Pos, bool) {
    if i+1 >= len(corridor) {
        return closestEnd, closestEnd, true
    }
    tile, poly, link, ok := m.linkBetween(corridor[i], corridor[i+1])
    if !ok {
        return Pos{}, Pos{}, false
    }
    nextTile, nextPoly := m.polyOfRef(corridor[i+1])
    if nextTile == nil {
        return Pos{}, Pos{}, false
    }
    left, right := m.portalLeftRight(tile, poly, link, nextTile, nextPoly)
    left, right = offsetPortal(left, right, clearance)

    return left, right, true
}

// funnelCone is the narrowing cone of the funnel algorithm: the apex
// and the two portal points it advanced through, with the corridor
// indices the restarts jump back to.
type funnelCone struct {
    apex                             Pos
    left, right                      Pos
    apexIndex, leftIndex, rightIndex int
}

// narrowRight advances the right cone side through the right point of
// portal i. It reports the new apex when the cone inverts on the left
// side (the path bends at the left portal point and the funnel
// restarts from it).
func (c *funnelCone) narrowRight(right Pos, i int) (Pos, bool) {
    if triArea2D(c.apex, c.right, right) < 0 {
        return Pos{}, false
    }
    if same2D(c.apex, c.right) || triArea2D(c.apex, c.left, right) < 0 {
        c.right = right
        c.rightIndex = i

        return Pos{}, false
    }
    c.apex = c.left
    c.apexIndex = c.leftIndex
    apex := c.apex
    c.left = apex
    c.right = apex
    c.leftIndex = c.apexIndex
    c.rightIndex = c.apexIndex

    return apex, true
}

// narrowLeft advances the left cone side through the left point of
// portal i. It reports the new apex when the cone inverts on the
// right side.
func (c *funnelCone) narrowLeft(left Pos, i int) (Pos, bool) {
    if triArea2D(c.apex, c.left, left) > 0 {
        return Pos{}, false
    }
    if same2D(c.apex, c.left) || triArea2D(c.apex, c.right, left) > 0 {
        c.left = left
        c.leftIndex = i

        return Pos{}, false
    }
    c.apex = c.right
    c.apexIndex = c.rightIndex
    apex := c.apex
    c.left = apex
    c.right = apex
    c.leftIndex = c.apexIndex
    c.rightIndex = c.apexIndex

    return apex, true
}

// linkBetween finds the link of one polygon that leads to the next
// polygon of the corridor.
func (m *Mesh) linkBetween(ref, next PolyRef,
) (*Tile, *Poly, *Link, bool) {
    tile, poly := m.polyOfRef(ref)
    if tile == nil {
        return nil, nil, nil, false
    }
    for li := poly.FirstLink; li >= 0 && int(li) < len(tile.Links); {
        link := &tile.Links[li]
        li = link.Next
        if m.resolveLink(tile, link) == next {
            return tile, poly, link, true
        }
    }

    return nil, nil, nil, false
}

// portalLeftRight returns the portal endpoints labeled by the local
// travel direction: the endpoint on the left of the corridor crossing
// (the from-polygon center toward the neighbor center) is the left
// cone side, the other one the right. The heights come from the
// surface of the polygon the link leaves.
func (m *Mesh) portalLeftRight(tile *Tile, poly *Poly, link *Link,
    nextTile *Tile, nextPoly *Poly,
) (Pos, Pos) {
    ax, ay, bx, by := tile.Portal(poly, link)
    midX, midY := (ax+bx)*0.5, (ay+by)*0.5
    cx0, cy0 := rectCenter(tile, poly)
    cx1, cy1 := rectCenter(nextTile, nextPoly)
    dx, dy := cx1-cx0, cy1-cy0
    // The cross product of the travel direction with the endpoint
    // offset picks the left endpoint (positive = counter-clockwise
    // left of the travel).
    crossA := dx*(ay-midY) - dy*(ax-midX)
    var left, right Pos
    if crossA > 0 {
        left = Pos{X: ax, Y: ay, Z: tile.HeightAt(poly, ax, ay)}
        right = Pos{X: bx, Y: by, Z: tile.HeightAt(poly, bx, by)}
    } else {
        left = Pos{X: bx, Y: by, Z: tile.HeightAt(poly, bx, by)}
        right = Pos{X: ax, Y: ay, Z: tile.HeightAt(poly, ax, ay)}
    }

    return left, right
}

// rectCenter returns the world center of a rectangle polygon.
func rectCenter(tile *Tile, poly *Poly) (float64, float64) {
    x0, y0, x1, y1 := tile.WorldRect(poly)

    return (x0 + x1) * 0.5, (y0 + y1) * 0.5
}

// offsetPortal pulls the portal endpoints inward along the shared
// edge by the clearance radius: the exact Detour pivot is the span
// end - the point where the open span meets the wall - and a
// character capsule turning exactly there clips the wall corner. A
// span narrower than twice the radius pivots at its middle: the
// deepest point of a narrow doorway is the best the capsule gets.
func offsetPortal(left, right Pos, radius float64) (Pos, Pos) {
    if radius <= 0 {
        return left, right
    }
    dx, dy := right.X-left.X, right.Y-left.Y
    length := math.Hypot(dx, dy)
    if length < 1e-6 {
        return left, right
    }
    if length <= 2*radius {
        mid := Pos{
            X: (left.X + right.X) / 2,
            Y: (left.Y + right.Y) / 2,
            Z: (left.Z + right.Z) / 2,
        }

        return mid, mid
    }
    ux, uy := dx/length, dy/length
    t := radius / length
    pulled := func(from Pos, sign float64) Pos {
        return Pos{
            X: from.X + sign*ux*radius,
            Y: from.Y + sign*uy*radius,
            Z: from.Z + sign*(right.Z-left.Z)*t,
        }
    }

    return pulled(left, 1), pulled(right, -1)
}

// appendWp appends a funnel corner with its corridor portal unless it
// duplicates the previous one in 2D (the duplicate keeps the earlier
// portal: the positions coincide, the walk address only tightens).
func appendWp(waypoints []funnelWp, point Pos, portal int32) []funnelWp {
    if len(waypoints) > 0 &&
        same2D(waypoints[len(waypoints)-1].pos, point) {
        return waypoints
    }

    return append(waypoints, funnelWp{pos: point, portal: portal})
}

// triArea2D is the signed 2D triangle area with the standard cross
// product convention: positive when c lies counter-clockwise left of
// the a-b line (the negative of the Detour dtTriArea2D).
func triArea2D(a, b, c Pos) float64 {
    return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// same2D reports whether two positions coincide in 2D.
func same2D(a, b Pos) bool {
    return math.Abs(a.X-b.X) < funnelEpsilon &&
        math.Abs(a.Y-b.Y) < funnelEpsilon
}

// distPtSegSqr2D is the squared 2D distance of a point to a segment
// (the Detour dtDistancePtSegSqr2D).
func distPtSegSqr2D(pt, a, b Pos) float64 {
    dx := b.X - a.X
    dy := b.Y - a.Y
    if dx == 0 && dy == 0 {
        ex := pt.X - a.X
        ey := pt.Y - a.Y

        return ex*ex + ey*ey
    }
    t := ((pt.X-a.X)*dx + (pt.Y-a.Y)*dy) / (dx*dx + dy*dy)
    t = math.Max(0, math.Min(1, t))
    ex := a.X + t*dx - pt.X
    ey := a.Y + t*dy - pt.Y

    return ex*ex + ey*ey
}
