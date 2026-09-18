package navmesh

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

// TestColdProbe scratches the cold route cost apart: the tile decodes,
// the sidecar loads and the searches (the diagnostic the prefetch and
// the sidecar guard rounds measured with, docs/fastpath_research.md
// sections 9 and 10).
func TestColdProbe(t *testing.T) {
    dir := "../../../../data/navmesh"
    if _, err := os.Stat(filepath.Join(dir, "20_19.nm")); err != nil {
        t.Skipf("the tile pack is not present: %v", err)
    }
    mesh := NewMesh(dir)
    keys := []RegionKey{
        {Col: 20, Row: 19}, {Col: 20, Row: 20},
        {Col: 21, Row: 19}, {Col: 21, Row: 20},
    }

    tileCost := time.Duration(0)
    for _, key := range keys {
        began := time.Now()
        if _, err := mesh.Tile(key); err != nil {
            t.Fatalf("tile %d_%d: %v", key.Col, key.Row, err)
        }
        tileCost += time.Since(began)
    }
    t.Logf("tile decode total: %s", tileCost)

    absCost := time.Duration(0)
    fresh := NewMesh(dir)
    for _, key := range keys {
        began := time.Now()
        if a := fresh.abstractOf(key); a == nil {
            t.Fatalf("abstract %d_%d missing", key.Col, key.Row)
        }
        absCost += time.Since(began)
    }
    t.Logf("sidecar+abstract total (4): %s", absCost)

    start := Pos{X: 12338, Y: 42444, Z: -3640}
    end := Pos{X: 58630, Y: 91061, Z: -3696}

    // The cold answer: everything lazy.
    cold := NewMesh(dir)
    began := time.Now()
    route, err := cold.Route(start, end, DefaultFilter())
    if err != nil {
        t.Fatalf("route: %v", err)
    }
    t.Logf("cold route: found=%v waypoints=%d explored=%d in %s",
        route.Found, len(route.Waypoints), route.Explored, time.Since(began))

    // The tiles warm, the sidecars cold.
    tilesWarm := NewMesh(dir)
    for _, key := range keys {
        _, _ = tilesWarm.Tile(key)
    }
    began = time.Now()
    _, err = tilesWarm.Route(start, end, DefaultFilter())
    if err != nil {
        t.Fatalf("route: %v", err)
    }
    t.Logf("route with tiles warm: %s", time.Since(began))

    // The sidecars warm, the tiles cold.
    sidesWarm := NewMesh(dir)
    for _, key := range keys {
        if a := sidesWarm.abstractOf(key); a == nil {
            t.Fatalf("abstract missing")
        }
    }
    began = time.Now()
    _, err = sidesWarm.Route(start, end, DefaultFilter())
    if err != nil {
        t.Fatalf("route: %v", err)
    }
    t.Logf("route with sidecars warm (tiles cold): %s", time.Since(began))
}
