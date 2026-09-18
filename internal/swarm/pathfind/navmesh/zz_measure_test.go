package navmesh

import (
    "os"
    "testing"
)

func TestCluster192Probe(t *testing.T) {
    dir := "../../../../data/navmesh"
    if _, err := os.Stat(dir); err != nil {
        t.Skip("no pack")
    }
    mesh := NewMesh(dir)
    mesh.SetCacheCapacity(4)

    // The probe answers against the 24_14 and 25_11 tiles: the
    // corridor packs without them skip.
    for _, key := range []RegionKey{{24, 14}, {25, 11}} {
        if _, err := mesh.Tile(key); err != nil {
            t.Skipf("the probe tile %d_%d is not in the pack", key.Col,
                key.Row)
        }
    }

    target := Pos{X: 180192, Y: -213024, Z: -3744}
    ref, pos, ok := mesh.FindNearestPoly(target)
    if !ok {
        t.Fatal("target does not bind")
    }
    col, row := TileOf(ref)
    abstract := mesh.abstractOf(RegionKey{Col: col, Row: row})
    if abstract == nil {
        t.Fatal("no target abstract")
    }
    idx := PolyOf(ref)
    comp := abstract.comps[idx]
    count := 0
    for _, c := range abstract.comps {
        if c == comp {
            count++
        }
    }
    t.Logf("target binds to %d_%d poly %d at %.0f %.0f, comp %d holds"+
        " %d polys", col, row, idx, pos.X, pos.Y, comp, count)

    // The 24_14 -> 25_11 border: the east side external links of
    // 24_14, their aliveness and the target comps in 25_11.
    west := mesh.abstractOf(RegionKey{Col: 24, Row: 14})
    if west == nil {
        t.Fatal("no 24_14 abstract")
    }
    tile, err := mesh.Tile(RegionKey{Col: 24, Row: 14})
    if err != nil {
        t.Fatal(err)
    }
    east := 0
    dead := 0
    compCounts := map[uint32]int{}
    for i := range tile.ExtLinks {
        ext := &tile.ExtLinks[i]
        if ext.Col != 25 || ext.Row != 11 {
            continue
        }
        east++
        if ext.Poly == 0xFFFFFFFF || int(ext.Poly) >= len(abstract.comps) {
            dead++

            continue
        }
        compCounts[abstract.comps[ext.Poly]]++
    }
    t.Logf("24_14 -> 25_11: %d ext links, %d dead, target comps: %v",
        east, dead, compCounts)
}
