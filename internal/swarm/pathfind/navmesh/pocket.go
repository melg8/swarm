// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// The pocket escape: the route answer for a start standing on ground the link
// graph cannot leave. The mesh links polygons across shared edges only, while
// the server movement channels also walk the diagonal squeezes and the one
// sided wall pairs of the click transport, so a railing pocket seals into a
// linkless island the corridor search never leaves. A stranded component (the
// flood stays inside pocketMaxSide) answers the closest connected ground
// outside it, carried past the walker arrival slack (docs/navmesh.md).

const (
    // pocketMaxSide bounds the link component the escape serves: the railing
    // pockets measure 16..320 units and the reported terraces 640x752 and
    // 592x432, while the mainland flood aborts once the box would stretch past
    // the side.
    pocketMaxSide = 2048.0
    // pocketExitRadius bounds the world ring the exit search scans around the
    // pocket box: a real pocket is adjacent to the ground it cannot link.
    pocketExitRadius = 768.0
    // pocketExitWaterPenalty prices the water polygons of the exit: the deck
    // sandwich bed lies 936 units under the elven village decks.
    pocketExitWaterPenalty = 8.0
    // pocketExitBanPenalty prices the exit ground under a foreign recovery ban:
    // the own ban ring keeps its way out, a dry unbanned exit wins the aim.
    pocketExitBanPenalty = 4.0
    // pocketExitZWindow bounds the height window of the exit candidates: the
    // deck sandwiches stack a few hundred units apart, the cliffs under a
    // thousand.
    pocketExitZWindow = 1024.0
    // pocketExitHorizontalFloorSq is the squared horizontal displacement the
    // exit boundary must carry from the standing point: the ground directly
    // under it (the stacked layers, the diagonal corner neighbors) is not a
    // walkable exit, and a boundary without displacement collapses the aim.
    pocketExitHorizontalFloorSq = 1.0
    // pocketExitMarchSteps carry the exit aim past the pocket boundary
    // into the connected ground (see the var definition below).
)

// pocketExitMarchSteps are the depths of the aim march past the pocket
// boundary, all beyond the 150 unit arrival slack of the walk follower (an aim
// inside it arrives without a click and the plan spins in place); the
// composition layer may still refine the direction through its click validation
// port.
var pocketExitMarchSteps = [3]float64{288.0, 256.0, 224.0}

// pocketComponent is the flooded link component of a pocket start: the seen
// polygons, their bounding box and the start polygon's own rect (the final aim
// guard).
type pocketComponent struct {
    seen       map[PolyRef]struct{}
    minX, minY float64
    maxX, maxY float64
    // sx0..sy1 is the start polygon's rect: the final aim guard checks the
    // answer against it.
    sx0, sy0, sx1, sy1 float64
}

// pocketEscape answers the walk out of a stranded start, nil when the start is
// not a pocket (the component outgrew the box or no connected exit exists
// within the radius) and the caller keeps the plain search answer. The plan is
// the single exit waypoint carried deep into the connected ground - not the
// boundary point (it sits under the server rescue click floor and inside the
// arrival slack) and not the start position.
func (m *Mesh) pocketEscape(
    startRef PolyRef, startPos Pos, avoid avoidCtx,
) *Route {
    component := m.floodPocketComponent(startRef)
    if component == nil {
        return nil
    }
    exit, ok := m.pocketExit(component, startPos, avoid)
    if !ok {
        return nil
    }
    if exit.X < component.sx1 && exit.X > component.sx0 &&
        exit.Y < component.sy1 && exit.Y > component.sy0 {
        // The aim collapsed back into the start footprint: the plan would click
        // the character's own position, and the honest no exit answer keeps the
        // plain search verdict.
        return nil
    }

    return &Route{
        Found:        false,
        Partial:      true,
        Waypoints:    []Pos{exit},
        Corridor:     nil,
        Explored:     len(component.seen),
        PocketEscape: true,
    }
}

// floodPocketComponent floods the link component of startRef and returns it
// with its bounding box, nil when the component outgrows the pocket box: the
// flood aborts the moment a polygon would stretch the box past the side.
func (m *Mesh) floodPocketComponent(startRef PolyRef) *pocketComponent {
    col, row := TileOf(startRef)
    tile, err := m.Tile(RegionKey{Col: col, Row: row})
    if err != nil || tile == nil {
        return nil
    }
    poly := &tile.Polys[PolyOf(startRef)]
    x0, y0, x1, y1 := tile.WorldRect(poly)
    if x1-x0 > pocketMaxSide || y1-y0 > pocketMaxSide {
        return nil
    }
    component := &pocketComponent{
        seen: map[PolyRef]struct{}{startRef: {}},
        minX: x0,
        minY: y0,
        maxX: x1,
        maxY: y1,
        sx0:  x0,
        sy0:  y0,
        sx1:  x1,
        sy1:  y1,
    }
    frontier := []PolyRef{startRef}
    for len(frontier) > 0 {
        var next []PolyRef
        for _, ref := range frontier {
            grown, honest := m.floodStep(component, ref)
            if !honest {
                return nil
            }
            next = append(next, grown...)
        }
        frontier = next
    }

    return component
}

// floodStep visits one frontier polygon: every unvisited link target that keeps
// the component inside the pocket box joins it and the next frontier. The
// second answer reports whether the flood may continue.
func (m *Mesh) floodStep(
    component *pocketComponent, ref PolyRef,
) (next []PolyRef, honest bool) {
    col, row := TileOf(ref)
    tile, err := m.Tile(RegionKey{Col: col, Row: row})
    if err != nil || tile == nil {
        return nil, true
    }
    poly := &tile.Polys[PolyOf(ref)]
    for li := poly.FirstLink; li >= 0 &&
        int(li) < len(tile.Links); li = tile.Links[li].Next {
        target, ttile, tpoly := m.linkTarget(tile, &tile.Links[li])
        if ttile == nil {
            continue
        }
        if _, seen := component.seen[target]; seen {
            continue
        }
        if !component.admit(ttile, tpoly) {
            return nil, false
        }
        component.seen[target] = struct{}{}
        next = append(next, target)
    }

    return next, true
}

// admit grows the component box over the polygon's rect: false when it would
// stretch the box past the pocket side. The box commits only on admit.
func (c *pocketComponent) admit(tile *Tile, poly *Poly) bool {
    x0, y0, x1, y1 := tile.WorldRect(poly)
    nMinX := math.Min(c.minX, x0)
    nMinY := math.Min(c.minY, y0)
    nMaxX := math.Max(c.maxX, x1)
    nMaxY := math.Max(c.maxY, y1)
    if nMaxX-nMinX > pocketMaxSide || nMaxY-nMinY > pocketMaxSide {
        return false
    }
    c.minX, c.minY, c.maxX, c.maxY = nMinX, nMinY, nMaxX, nMaxY

    return true
}

// pocketExit finds the walkable exit aim of the pocket: the closest connected
// ground outside the component seeds the boundary, and the aim marches deep
// into that ground along the horizontal exit direction.
func (m *Mesh) pocketExit(
    component *pocketComponent, startPos Pos, avoid avoidCtx,
) (Pos, bool) {
    boundary, ok := m.pocketBoundary(component, startPos, avoid)
    if !ok {
        return Pos{}, false
    }

    return m.pocketAim(component, startPos, boundary), true
}

// pocketBoundary scans the world ring around the pocket box for the closest
// point of the connected ground outside the component, never another linkless
// island and never the vertical stack in the start footprint; the water and the
// banned ground are priced so the honest dry exit wins.
func (m *Mesh) pocketBoundary(
    component *pocketComponent, startPos Pos, avoid avoidCtx,
) (Pos, bool) {
    minX := component.minX - pocketExitRadius
    minY := component.minY - pocketExitRadius
    maxX := component.maxX + pocketExitRadius
    maxY := component.maxY + pocketExitRadius
    minZ := startPos.Z - pocketExitZWindow
    maxZ := startPos.Z + pocketExitZWindow

    bestDist := math.MaxFloat64
    boundary := Pos{}
    found := false
    var candidates []int32
    for _, key := range m.regionKeysOfBox(minX, minY, maxX, maxY) {
        tile, err := m.Tile(key)
        if err != nil || tile == nil {
            continue
        }
        candidates = tileQueryPolys(tile, minX, minY, maxX, maxY, minZ,
            maxZ, candidates[:0])
        for _, pi := range candidates {
            ref := RefOf(key.Col, key.Row, uint32(pi))
            if _, seen := component.seen[ref]; seen {
                continue
            }
            poly := &tile.Polys[pi]
            if poly.FirstLink < 0 {
                // Never exit into another island: the escape must land on
                // ground the next plan cycle routes from.
                continue
            }
            cx, cy, cz := tile.ClosestPoint(poly, startPos.X, startPos.Y,
                startPos.Z)
            dx := startPos.X - cx
            dy := startPos.Y - cy
            if dx*dx+dy*dy < pocketExitHorizontalFloorSq {
                // The ground directly under the standing point: the stacked
                // layers and the diagonal corner neighbors answer the closest
                // 3D points of the whole scan (a deck 32 units down beats the
                // honest edge at 36), but the character cannot walk down onto
                // them and their boundary carries no horizontal displacement.
                continue
            }
            dz := startPos.Z - cz
            dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
            if poly.Area == AreaWater {
                dist *= pocketExitWaterPenalty
            }
            if avoid.state(tile, poly) == avoidWall {
                dist *= pocketExitBanPenalty
            }
            if dist < bestDist {
                bestDist = dist
                boundary = Pos{X: cx, Y: cy, Z: cz}
                found = true
            }
        }
    }

    return boundary, found
}

// pocketAim carries the boundary point deep into the exit ground along the
// horizontal exit direction and snaps it onto real ground that is neither the
// pocket nor another island. The march stays horizontal (a ground walk, the
// surface snap owns the height); the full depth first, the shorter steps when
// the exit strip ends sooner, the boundary point as the last resort.
func (m *Mesh) pocketAim(
    component *pocketComponent, startPos, boundary Pos,
) Pos {
    dirX := boundary.X - startPos.X
    dirY := boundary.Y - startPos.Y
    length := math.Hypot(dirX, dirY)
    if length < 1 {
        return boundary
    }
    for _, march := range pocketExitMarchSteps {
        frac := march / length
        aim := Pos{
            X: boundary.X + dirX*frac,
            Y: boundary.Y + dirY*frac,
            Z: boundary.Z,
        }
        ref, pos, ok := m.FindNearestPoly(aim)
        if !ok {
            continue
        }
        if _, seen := component.seen[ref]; seen {
            continue
        }
        _, poly := m.polyOfRef(ref)
        if poly.FirstLink < 0 {
            continue
        }

        return pos
    }

    return boundary
}
