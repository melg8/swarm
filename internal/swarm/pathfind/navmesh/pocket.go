// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "math"

// The pocket escape: the route answer for a start standing on ground
// the link graph cannot leave. The mesh links polygons across shared
// EDGES only (the open NSWE wall pair of both cells, the strict form
// the server pathfinder applies), while the server movement channels
// also walk DIAGONAL squeezes (the anti corner cut rule of
// GeoEngine.checkNearestNsweAntiCornerCut the grid engine mirrors) and
// its click transport validates one sided wall pairs the link builder
// refuses. A cell whose every axis neighbor is walled - a railing
// segment closing each side - but which touches open ground across a
// corner is a linkless one polygon island: the corridor search can
// never leave it and every route from the standing position answers
// the bare not found, while the grid engine plans out of the very
// same cell without effort. The character reaches such cells through
// the permissive click transport (or a server side displacement) and
// the strict route graph strands it there.
//
// The escape restores the class invariant instead of patching one
// cell: when the corridor search cannot leave the start's link
// component AND that component is a small spot (the flood stays
// inside the pocket box), the search answers the closest reachable
// route OUT of the spot - the nearest connected ground of another
// component, water priced like every escape (the bed under a deck
// must never win the exit), the bot's own bans priced like the
// foreign ground they name. The walk-what-you-can partial contract
// serves the answer end to end: the follower clicks at the exit aim,
// the click validation judges every step honestly (the permissive
// transport that got the character into the pocket delivers it back
// out), and the next plan cycle routes from connected ground.

const (
    // pocketMaxSide bounds the link component the escape serves: a
    // component whose bounding box fits the side is a stranded spot
    // (the railing pockets measure one to a few cells, 16..320 units);
    // a wider one is honest ground - a sealed yard or an island the
    // walk-what-you-can partial already serves - and keeps the plain
    // answers.
    pocketMaxSide = 320.0
    // pocketExitRadius bounds the world ring the exit search scans
    // around the pocket box: a pocket is by construction adjacent to
    // the ground it cannot link (the diagonal or one sided neighbor
    // cells are polys within a few cells), so a connected exit within
    // the radius exists for every real pocket.
    pocketExitRadius = 768.0
    // pocketExitWaterPenalty prices the water polygons of the exit
    // search: the escape pricing of the water crossings (the deck
    // sandwich bed lies 936 units under the elven village decks - the
    // raw 3D distance would pick it against nothing).
    pocketExitWaterPenalty = 8.0
    // pocketExitBanPenalty prices the exit ground under a foreign
    // recovery ban (the session ground the live server refused): the
    // own ban ring keeps its way out contract - the escape from the
    // ban that holds the start crosses its own ground - but a dry
    // unbanned exit next door wins the aim when one exists.
    pocketExitBanPenalty = 4.0
    // pocketExitZWindow bounds the height window of the exit
    // candidates: the deck sandwiches stack layers a few hundred
    // units apart, the region cliffs measure under a thousand - the
    // window keeps the absurd stacks out of the scan.
    pocketExitZWindow = 1024.0
    // pocketExitMarchSteps carry the exit aim past the pocket boundary
    // into the connected ground (see the var definition below).
)

// pocketExitMarchSteps are the depths of the aim march past the
// pocket boundary, measured from the boundary along the exit
// direction: the walk follower serves the plan's last waypoint with a
// wide arrival slack (150 units in the live hunt layer), so an aim
// closer than that "arrives" without a single click and the plan
// spins without moving the character. The depths keep the snapped aim
// beyond that slack even in the worst case (the boundary at the
// pocket edge plus the nearest poly snap window pull); the composition
// layer may still refine the direction through its click validation
// port - the mesh answer stays the seed of the walk out, never a hard
// promise.
var pocketExitMarchSteps = [3]float64{288.0, 256.0, 224.0}

// pocketComponent is the flooded link component of a pocket start:
// the seen polygons and their bounding box in world units.
type pocketComponent struct {
    seen       map[PolyRef]struct{}
    minX, minY float64
    maxX, maxY float64
}

// pocketEscape answers the walk out of a stranded start, nil when the
// start is not a pocket (the component left the pocket box or no
// connected exit exists within the radius): the caller keeps the
// plain search answer in that case. The plan is the single exit
// waypoint - the aim carried deep into the connected ground, not the
// boundary point (a boundary aim sits under the server rescue click
// floor, behind the strict sight lines the follower gates the
// waypoint advance on, and inside the wide arrival slack the follower
// serves the last waypoint with) and not the start position (the
// character stands on it, the follower would click its own cell). The
// next plan cycle routes from the exit ground the arrival lands on.
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

    return &Route{
        Found:        false,
        Partial:      true,
        Waypoints:    []Pos{exit},
        Corridor:     nil,
        Explored:     len(component.seen),
        PocketEscape: true,
    }
}

// floodPocketComponent floods the link component of startRef and
// returns it with its bounding box, nil when the component outgrows
// the pocket box (the honest ground case): the flood aborts the
// moment a polygon would stretch the box past pocketMaxSide, so the
// cost stays bounded by the polygons of a pocket sized area.
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

// floodStep visits one frontier polygon: every unvisited link target
// that keeps the component inside the pocket box joins the component
// and the next frontier. The second answer reports whether the flood
// may continue - false when a target would stretch the box past the
// pocket side (the honest ground verdict of the flood).
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

// admit grows the component box over the polygon's world rect: false
// when the rect would stretch the box past the pocket side - the
// honest ground verdict. The box commits only on admit.
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

// pocketExit finds the walkable exit aim of the pocket: the closest
// connected ground outside the component seeded as the boundary, the
// aim marched deep into that ground along the horizontal exit
// direction and snapped onto the real surface.
func (m *Mesh) pocketExit(
    component *pocketComponent, startPos Pos, avoid avoidCtx,
) (Pos, bool) {
    boundary, ok := m.pocketBoundary(component, startPos, avoid)
    if !ok {
        return Pos{}, false
    }

    return m.pocketAim(component, startPos, boundary), true
}

// pocketBoundary scans the world ring around the pocket box for the
// closest point of the connected ground outside the component (the
// tile spatial index answers each tile of the ring), never another
// linkless island, the water and the banned ground priced so the
// honest dry exit wins.
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
                // Never exit into another island: the escape must
                // land on ground the next plan cycle routes from.
                continue
            }
            cx, cy, cz := tile.ClosestPoint(poly, startPos.X, startPos.Y,
                startPos.Z)
            dx := startPos.X - cx
            dy := startPos.Y - cy
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

// pocketAim carries the boundary point deep into the exit ground
// along the horizontal exit direction and snaps it onto the real
// surface of ground that is neither the pocket nor another island.
// The march stays horizontal - the walk is a ground walk, the surface
// snap owns the height - so a boundary poly a deck step above or
// below the start cannot tilt the aim into the air. The full depth
// first, the shorter steps when the exit strip ends sooner; the
// boundary point itself stays the last resort (the composition layer
// refines the deliverable aim through its click validation port).
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
