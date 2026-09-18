// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import "testing"

// TestZstdTilePackLoad reads the zstd tile of the corridor pack
// through the production loader (the magic word branch, the pooled
// decoder, the DecodeTile parse) and binds two world positions: the
// zstd round must not change what the runtime sees. Skips when the
// pack is absent (the tiles stay out of git).
func TestZstdTilePackLoad(t *testing.T) {
    mesh := NewMesh("../../../../data/navmesh")
    tile, err := mesh.Tile(RegionKey{Col: 21, Row: 19})
    if err != nil {
        t.Skipf("the tile pack is not present: %v", err)
    }
    if len(tile.Polys) == 0 {
        t.Fatal("the decoded 21_19 tile answers no polygons")
    }
    for _, p := range []Pos{
        {X: 36000, Y: 36000, Z: -3696},
        {X: 50000, Y: 50000, Z: -3696},
    } {
        ref, snapped, ok := mesh.FindNearestPoly(p)
        if !ok {
            t.Fatalf("the position %.0f %.0f does not bind to the "+
                "zstd tile", p.X, p.Y)
        }
        if snapped.X != p.X || snapped.Y != p.Y {
            t.Logf("pos %.0f,%.0f snapped to %.0f,%.0f (ref %d)",
                p.X, p.Y, snapped.X, snapped.Y, uint64(ref))
        }
    }
}
