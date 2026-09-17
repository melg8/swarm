// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// The capsule clearance tests run over a synthetic corridor region:
// a flat plane at height 0 with a two cells wide horizontal corridor
// between the wall rows (the north wall of row 10 and the south wall
// of row 11), sealed at both ends, plus optional extra obstacles. All
// tests address the world through cell fraction helpers, so the
// assertions read in game geometry, not in wire offsets.

// capsuleWallWorld builds the corridor region in a fresh geodata dir
// and returns the engine over it together with the world origin of
// the region.
func capsuleWallWorld(t *testing.T) (*Engine, func(float64, float64) Vec3) {
    t.Helper()
    spec := &regionSpec{}
    spec.setFlat(0)
    closed := map[[2]int]uint8{}
    add := func(x, y int, dirs uint8) {
        key := [2]int{x, y}
        closed[key] |= dirs
    }
    // The corridor rows 10..11, the walls along both sides.
    for x := 0; x < 32; x++ {
        add(x, 9, nsweSouth)
        add(x, 10, nsweNorth)
        add(x, 11, nsweSouth)
        add(x, 12, nsweNorth)
    }
    // The end seals at x 3..4 and x 21..22.
    for y := 10; y <= 11; y++ {
        add(3, y, nsweEast)
        add(4, y, nsweWest)
        add(21, y, nsweEast)
        add(22, y, nsweWest)
    }
    for cell, dirs := range closed {
        spec.setCell(cell[0], cell[1],
            Layer{Height: 0, NSWE: nsweAll &^ dirs})
    }
    dir := t.TempDir()
    spec.writeRegion(t, dir)
    engine := NewEngine(dir)

    return engine, func(cellX, cellY float64) Vec3 {
        baseX := float64((testRegionCol - tileZeroCol) * tileSize)
        baseY := float64((testRegionRow - tileZeroRow) * tileSize)

        return Vec3{
            X: baseX + cellX*cellSize,
            Y: baseY + cellY*cellSize,
            Z: 0,
        }
    }
}

// pathMinClearance walks the sampled legs of the path and returns the
// smallest wall clearance along them.
func pathMinClearance(capsule *Capsule, path []Vec3) float64 {
    worst := math.MaxFloat64
    for i := 1; i < len(path); i++ {
        a, b := path[i-1], path[i]
        length := math.Hypot(b.X-a.X, b.Y-a.Y)
        samples := int(length/capsuleSampleStep) + 1
        if samples < 2 {
            samples = 2
        }
        for k := 0; k <= samples; k++ {
            t := float64(k) / float64(samples)
            point := legPoint(a, b, t)
            worst = math.Min(worst,
                capsule.Clearance(point.X, point.Y, int16(point.Z)))
        }
    }

    return worst
}

// TestCapsuleClearance pins the probe: a cell center of the corridor
// keeps 8 units to its adjacent wall, the mid corridor line 16, a
// point on the wall line 0 and an open field point the cap.
func TestCapsuleClearance(t *testing.T) {
    engine, world := capsuleWallWorld(t)
    capsule := NewCapsule(engine)

    center := world(10.5, 10.5)
    require.InDelta(t, 8, capsule.Clearance(center.X, center.Y, 0), 1e-6)

    middle := world(10.5, 11)
    require.InDelta(t, 16, capsule.Clearance(middle.X, middle.Y, 0), 1e-6)

    onWall := world(10.5, 10)
    require.InDelta(t, 0, capsule.Clearance(onWall.X, onWall.Y, 0), 1e-6)

    field := world(100.5, 100.5)
    require.InDelta(t, capsuleMaxDistance,
        capsule.Clearance(field.X, field.Y, 0), 1e-6)
}

// TestCapsulePushesWaypointOffWall runs the post pass over a path
// whose middle waypoint sits on the corridor wall line: the waypoint
// moves into the corridor until the capsule clears the wall, the
// endpoints stay and every leg of the answer clears the radius.
func TestCapsulePushesWaypointOffWall(t *testing.T) {
    engine, world := capsuleWallWorld(t)
    capsule := NewCapsule(engine)

    start := world(5.5, 10.5)
    end := world(15.5, 10.5)
    onWall := world(10.5, 10)
    path := capsule.ApplyPath([]Vec3{start, onWall, end},
        DefaultCollisionRadius)

    require.Len(t, path, 3)
    require.Equal(t, start, path[0])
    require.Equal(t, end, path[2])
    pushed := path[1]
    require.GreaterOrEqual(t, capsule.Clearance(pushed.X, pushed.Y,
        int16(pushed.Z)), DefaultCollisionRadius)
    // The push went into the corridor (south of the wall line).
    require.Greater(t, pushed.Y, onWall.Y)
    require.GreaterOrEqual(t, pathMinClearance(capsule, path),
        DefaultCollisionRadius-1e-6)
}

// TestCapsuleBendsGrazingLeg bends a straight leg that runs into a
// pillar: one fully blocked corridor cell protrudes into the row and
// the leg skims its faces at sub radius distances. The answer keeps
// the endpoints, adds the bend anchors skirtting the pillar and every
// leg of the answer clears the radius.
func TestCapsuleBendsGrazingLeg(t *testing.T) {
    _, world := capsuleWallWorld(t)
    // Rebuild the region with one blocked pillar cell inside the
    // corridor (the row 10 cell at x 10).
    spec := &regionSpec{}
    spec.setFlat(0)
    closed := map[[2]int]uint8{}
    add := func(x, y int, dirs uint8) {
        key := [2]int{x, y}
        closed[key] |= dirs
    }
    for x := 0; x < 32; x++ {
        add(x, 9, nsweSouth)
        add(x, 10, nsweNorth)
        add(x, 11, nsweSouth)
        add(x, 12, nsweNorth)
    }
    for cell, dirs := range closed {
        spec.setCell(cell[0], cell[1],
            Layer{Height: 0, NSWE: nsweAll &^ dirs})
    }
    spec.setCell(10, 10, Layer{Height: 0, NSWE: 0})
    dir := t.TempDir()
    spec.writeRegion(t, dir)
    capsule := NewCapsule(NewEngine(dir))

    start := world(5.5, 10.5)
    end := world(15.5, 10.5)
    path := capsule.ApplyPath([]Vec3{start, end}, DefaultCollisionRadius)
    require.Greater(t, len(path), 2, "the leg must gain bend anchors")
    require.Equal(t, start, path[0])
    require.Equal(t, end, path[len(path)-1])
    require.GreaterOrEqual(t, pathMinClearance(capsule, path),
        DefaultCollisionRadius-1e-6, "every leg must clear the capsule")
}

// TestCapsuleKeepsClearLegUntouched pins the no-churn contract: a
// corridor path whose waypoints and legs already clear the capsule
// passes through the post pass unchanged.
func TestCapsuleKeepsClearLegUntouched(t *testing.T) {
    engine, world := capsuleWallWorld(t)
    capsule := NewCapsule(engine)

    start := world(5.5, 10.5)
    middle := world(10.5, 10.5)
    end := world(15.5, 10.5)
    before := []Vec3{start, middle, end}
    after := capsule.ApplyPath(before, DefaultCollisionRadius)
    require.Equal(t, before, after)
}

// TestCapsuleFallbackKeepsImpossibleWaypoint pins the honest
// fallback: a waypoint inside a solid wall block cannot be pushed
// anywhere walkable, so the pass keeps it and the legs unchanged.
func TestCapsuleFallbackKeepsImpossibleWaypoint(t *testing.T) {
    _, world := capsuleWallWorld(t)
    // Solid block over the cells 8..12 x 10..11: the corridor sealed
    // mid way with a thick wall.
    spec := &regionSpec{}
    spec.setFlat(0)
    for x := 8; x <= 12; x++ {
        for y := 10; y <= 11; y++ {
            spec.setCell(x, y, Layer{Height: 0, NSWE: 0})
        }
    }
    // The rest of the corridor world's walls are irrelevant here; the
    // block alone defines the clearance question. Write it over a
    // fresh copy of the corridor region: rebuild the whole region.
    closed := map[[2]int]uint8{}
    add := func(x, y int, dirs uint8) {
        key := [2]int{x, y}
        closed[key] |= dirs
    }
    for x := 0; x < 32; x++ {
        add(x, 9, nsweSouth)
        add(x, 10, nsweNorth)
        add(x, 11, nsweSouth)
        add(x, 12, nsweNorth)
    }
    for cell, dirs := range closed {
        if cell[0] >= 8 && cell[0] <= 12 &&
            (cell[1] == 10 || cell[1] == 11) {
            continue
        }
        spec.setCell(cell[0], cell[1],
            Layer{Height: 0, NSWE: nsweAll &^ dirs})
    }
    dir := t.TempDir()
    spec.writeRegion(t, dir)
    blocked := NewEngine(dir)

    capsule := NewCapsule(blocked)
    start := world(5.5, 10.5)
    inside := world(10.5, 10.1)
    end := world(15.5, 10.5)
    before := []Vec3{start, inside, end}
    after := capsule.ApplyPath(before, DefaultCollisionRadius)
    require.Equal(t, before, after)
}

// TestEngineCapsuleClearanceIntegration arms the pass on the engine
// and plans a corridor route: the waypoints keep the clearance, and
// the disabled engine answers the raw smoothing (the waypoints of a
// clean corridor route are cell centers, clearance 8 - untouched by
// the pass either way).
func TestEngineCapsuleClearanceIntegration(t *testing.T) {
    engine, world := capsuleWallWorld(t)
    start := world(5.5, 10.5)
    end := world(15.5, 10.5)

    raw, err := engine.FindPath(start, end, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, raw.Found)
    require.GreaterOrEqual(t,
        pathMinClearance(NewCapsule(engine), raw.Waypoints),
        DefaultCollisionRadius-1e-6,
        "cell center smoothing already clears the capsule here")

    engine.SetCapsuleClearance(DefaultCollisionRadius)
    require.Equal(t, DefaultCollisionRadius, engine.CapsuleRadius())
    cleared, err := engine.FindPath(start, end, DefaultMaxPassableHeight)
    require.NoError(t, err)
    require.True(t, cleared.Found)
    require.Equal(t, raw.Waypoints, cleared.Waypoints,
        "the pass must not churn a path that already clears")
    require.GreaterOrEqual(t,
        pathMinClearance(NewCapsule(engine), cleared.Waypoints),
        DefaultCollisionRadius-1e-6)
}
