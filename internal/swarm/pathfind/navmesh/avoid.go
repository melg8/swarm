// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

// The avoid areas of the corridor search: the recovery bans of the
// hunt loop ported onto the rectangle granularity of the mesh. The
// grid engine walls every covered CELL of a ban; the mesh walls every
// polygon whose footprint the ban disk TOUCHES - the over-walling
// direction, deliberately: a funnelled route crosses only corridor
// polygons, and no corridor polygon touches the disk, so no point of
// any walked leg ever enters the banned ground (the grid only keeps
// the cell centers of the smoothed legs out - the mesh guarantee is
// strictly stronger). A big rectangle grazed by a ban at its corner is
// walled whole; the detours err long, never through the freeze the
// ban exists to detour.

import "math"

// AvoidCircle is one banned world disk of a search: the recovery ban
// the hunt loop derives from its freeze reports (ground the live
// server refused to walk although the geodata pack modeled it as
// open). A local leaf-package type, kept beside Pos.
type AvoidCircle struct {
    CenterX float64
    CenterY float64
    Radius  float64
}

// Avoid search constants, mirroring the grid engine (search.go: the
// avoidEscapeMultiplier and avoidEscapeRadius of the cell search).
const (
    // avoidEscapeMultiplier prices the escape polygons of the ban
    // that holds the search start: the only honest route out of the
    // own ban crosses its own ground, expensively.
    avoidEscapeMultiplier = 6.0
    // avoidEscapeRadius bounds the escape polygons of the own ban:
    // the polygons whose footprint reaches within this distance of
    // the start stay passable at the multiplier, the own ground
    // beyond keeps its wall (the sealed goal contract).
    avoidEscapeRadius = 256.0
)

// avoidState classifies one polygon against the avoid circles of a
// search.
type avoidState uint8

const (
    avoidFree   avoidState = iota // no circle touches the footprint
    avoidWall                     // a wall under the ban rules below
    avoidEscape                   // own ban near the start: priced
)

// avoidCtx is the per-search avoid context, derived once from the
// filter circles and the raw start position: the escape index of the
// ban holding the start (the way-out rule) and the start itself for
// the escape ring test.
type avoidCtx struct {
    areas     []AvoidCircle
    escapeIdx int
    start     Pos
    active    bool
}

// newAvoidCtx derives the avoid context. The escape index is the
// first circle containing the start position - the walker standing
// inside a ban must be able to plan its way OUT of it, exactly like
// the grid startAvoidIndex rule.
func newAvoidCtx(areas []AvoidCircle, start Pos) avoidCtx {
    ctx := avoidCtx{
        areas:     areas,
        escapeIdx: -1,
        start:     start,
        active:    len(areas) > 0,
    }
    if !ctx.active {
        return ctx
    }
    for i, area := range areas {
        if math.Hypot(start.X-area.CenterX, start.Y-area.CenterY) <=
            area.Radius {
            ctx.escapeIdx = i

            break
        }
    }

    return ctx
}

// noAvoid is the avoid context of the searches without bans (the
// water escape).
func noAvoid() avoidCtx {
    return avoidCtx{
        areas:     nil,
        escapeIdx: -1,
        start:     Pos{X: 0, Y: 0, Z: 0},
        active:    false,
    }
}

// state classifies one polygon of one tile against the circles of
// the context:
//
//   - a polygon touched by a FOREIGN ban is a wall, even when the own
//     escape ring overlaps it too (the route out of the own ban must
//     not thread a foreign ban - the grid cellAvoidedEscape rule),
//   - a polygon touched only by the own ban is an escape polygon while
//     its footprint reaches within avoidEscapeRadius of the start,
//     a wall beyond it,
//   - a polygon no circle touches is free.
//
// The start polygon itself carries whatever state it carries: the
// corridor search creates its root node before any avoid check (the
// grid start cell is likewise passable by construction), the walls
// gate only the steps ONTO polygons.
func (a avoidCtx) state(tile *Tile, poly *Poly) avoidState {
    if !a.active || tile == nil || poly == nil {
        return avoidFree
    }
    x0, y0, x1, y1 := tile.WorldRect(poly)
    own := false
    for i, area := range a.areas {
        if !circleOverlapsRect(area, x0, y0, x1, y1) {
            continue
        }
        if i != a.escapeIdx {
            return avoidWall
        }
        own = true
    }
    if !own {
        return avoidFree
    }
    if rectPointDist(x0, y0, x1, y1, a.start.X, a.start.Y) <=
        avoidEscapeRadius {
        return avoidEscape
    }

    return avoidWall
}

// circleOverlapsRect reports whether a circle and an axis aligned
// rectangle share any point (the standard clamp test).
func circleOverlapsRect(
    c AvoidCircle, x0, y0, x1, y1 float64,
) bool {
    px := math.Max(x0, math.Min(x1, c.CenterX))
    py := math.Max(y0, math.Min(y1, c.CenterY))
    dx := c.CenterX - px
    dy := c.CenterY - py

    return dx*dx+dy*dy <= c.Radius*c.Radius
}

// rectPointDist returns the planar distance from a point to an axis
// aligned rectangle (zero inside).
func rectPointDist(x0, y0, x1, y1, px, py float64) float64 {
    dx := math.Max(math.Max(x0-px, px-x1), 0)
    dy := math.Max(math.Max(y0-py, py-y1), 0)

    return math.Hypot(dx, dy)
}
