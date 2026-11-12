// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// Navigator plans walkable paths through the world geodata. The
// pathfind engine is wrapped into one through NewNavigator; tests fake
// the interface.
type Navigator interface {
    // FindPathApproach plans a walk that must end within the
    // approach radius (3D) of the target point: the merchant stops
    // use the interaction distance, the exact target is preferred
    // whenever it is reachable.
    FindPathApproach(start, end pathfind.Vec3, approachRadius float64) (
        *pathfind.Result, error,
    )
    // FindPathApproachAvoiding plans the walk around the given avoid
    // areas: the composition layer's avoid seam (the mesh ban walls
    // with the escape ring of the own ban). The hunt loop's frozen
    // corridor bans were its historical consumer - the plaza round of
    // 2026-09-20 removed them (the rectangle granularity sealed whole
    // mesh sheets), the mesh level capability stays for the library
    // contract.
    FindPathApproachAvoiding(
        start, end pathfind.Vec3, approachRadius float64,
        avoid []pathfind.AvoidArea,
    ) (*pathfind.Result, error)
    // FindPath plans a walk to the target cell arriving on whatever
    // deck of it the walk reaches first.
    FindPath(start, end pathfind.Vec3) (*pathfind.Result, error)
    // ClosestHeight resolves the height of the layer at the world
    // position closest to refZ - the deck the server itself picks
    // for a destination named with that z. The zone return resolves
    // its goal height through it before the approach search.
    ClosestHeight(x, y float64, refZ int16) (int16, error)
    // LineOfSight reports whether the geodata holds a clear straight
    // line between two world positions: the blind engage recovery uses
    // it to find a standing point that sees the obstructed target.
    LineOfSight(start, end pathfind.Vec3) (bool, error)
    // OverWater reports whether the walkable surface under the world
    // position lies below the C1 water level: the character stands
    // over a lake or sea bed (swimming or floating on it). The frame
    // measurement arms on it - a swimming character measures no
    // vintage shift (see click_frame.go).
    OverWater(x, y float64, refZ int16) bool
    // ValidateClick mirrors the server-side validation of a
    // mouse-mode move request: it answers the destination the
    // server would actually walk to and whether the click runs at
    // all. A refused click (the geodata correction collapses the
    // target onto the walker) never moves the character - the
    // follower reacts to it instead of sending it.
    ValidateClick(from, to pathfind.Vec3) (pathfind.Vec3, bool)
}

// engineNavigator adapts a geodata engine to the Navigator interface,
// applying the configured maximum passable height of the engine.
type engineNavigator struct {
    engine *pathfind.Engine
}

// NewNavigator wraps a geodata engine into the town trip navigator.
// Returning the Navigator interface is the deliberate seam of the
// hunt package (see the AGENTS.md interface map).
func NewNavigator(engine *pathfind.Engine) Navigator { //nolint:ireturn
    return engineNavigator{engine: engine}
}

// FindPathApproach searches the walkable path with the engine settings
// and the approach radius goal.
func (e engineNavigator) FindPathApproach(
    start, end pathfind.Vec3, approachRadius float64,
) (*pathfind.Result, error) {
    return e.engine.FindPathApproach(
        start, end, approachRadius, e.engine.MaxPassableHeight())
}

// FindPathApproachAvoiding searches the path around the avoid areas
// with the engine settings and the approach radius goal.
func (e engineNavigator) FindPathApproachAvoiding(
    start, end pathfind.Vec3, approachRadius float64,
    avoid []pathfind.AvoidArea,
) (*pathfind.Result, error) {
    return e.engine.FindPathApproachAvoiding(
        start, end, approachRadius, e.engine.MaxPassableHeight(), avoid)
}

// FindPath searches the walkable path with the engine settings.
func (e engineNavigator) FindPath(
    start, end pathfind.Vec3,
) (*pathfind.Result, error) {
    return e.engine.FindPath(start, end, e.engine.MaxPassableHeight())
}

// ClosestHeight resolves the destination deck height with the engine.
func (e engineNavigator) ClosestHeight(
    x, y float64, refZ int16,
) (int16, error) {
    return e.engine.ClosestHeight(x, y, refZ)
}

// LineOfSight answers the geodata sight line with the engine settings.
func (e engineNavigator) LineOfSight(
    start, end pathfind.Vec3,
) (bool, error) {
    return e.engine.LineOfSight(start, end, e.engine.MaxPassableHeight())
}

// OverWater answers the geodata water surface check with the engine.
func (e engineNavigator) OverWater(x, y float64, refZ int16) bool {
    return e.engine.OverWater(x, y, refZ)
}

// ValidateClick mirrors the server move validation with the engine.
func (e engineNavigator) ValidateClick(
    from, to pathfind.Vec3,
) (pathfind.Vec3, bool) {
    return e.engine.ValidateClick(from, to)
}
