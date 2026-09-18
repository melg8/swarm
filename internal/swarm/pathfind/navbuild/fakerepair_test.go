// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// fakeSplitWorld builds the two real halves with the fake filler
// stripe between them: the west land plateau, the east water floor,
// the middle columns the l2j uninitialized filler (1 layer, height 0,
// fully open). The stripe width rides the parameter.
func fakeSplitWorld(stripe int) func(cx, cy int) []layerSpec {
    return func(cx, cy int) []layerSpec {
        if cx >= 1000 && cx < 1000+stripe {
            return []layerSpec{{h: 0, nswe: 0x0F}}
        }
        if cx < 1000 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }

        return []layerSpec{{h: -4736, nswe: 0x0F}}
    }
}

// TestFakeRepairConnectsLand builds the split world without and with
// the fake repair: without, the filler builds the floating plateau
// and the land never reaches the water side; with the repair the
// filler blends into the connecting surface and the route crosses.
func TestFakeRepairConnectsLand(t *testing.T) {
    const stripe = 64
    region := writeRegionFile(t, fakeSplitWorld(stripe))

    plain, err := BuildRegion(region, 18, 21,
        Options{Climb: 40, DedupDelta: 32, MinSheetLayers: 4})
    if err != nil {
        t.Fatal(err)
    }
    if plain.Stats.FakeFilled != 0 {
        t.Fatalf("plain build filled %d fake cells", plain.Stats.FakeFilled)
    }
    // The plain build: the land polys and the water polys live on
    // disconnected components (the island filter keeps both - both
    // touch the region border), the filler plateau sits between.
    linksLandToWater := countAreaLinks(plain.Tile)
    if linksLandToWater {
        t.Fatal("the plain build links land to water through the" +
            " filler: the world lost the fake cliff")
    }

    repaired, err := BuildRegion(region, 18, 21, Options{
        Climb: 40, DedupDelta: 32, MinSheetLayers: 4, RepairFake: true})
    if err != nil {
        t.Fatal(err)
    }
    if repaired.Stats.FakeFilled != stripe*regionCellsSide {
        t.Fatalf("filled %d fake cells, want %d",
            repaired.Stats.FakeFilled, stripe*regionCellsSide)
    }
    if !countAreaLinks(repaired.Tile) {
        t.Fatal("the repaired build still does not link the land to" +
            " the water: the fill left the cliff")
    }
}

// countAreaLinks answers whether the tile carries an internal link
// that joins a ground polygon to a water polygon (the walkable land
// to water transition).
func countAreaLinks(tile *navmesh.Tile) bool {
    seen := make([]bool, len(tile.Links))
    for pi := range tile.Polys {
        for li := tile.Polys[pi].FirstLink; li >= 0 &&
            int(li) < len(tile.Links) && !seen[li]; {
            seen[li] = true
            link := &tile.Links[li]
            li = link.Next
            if link.To < 0 {
                continue
            }
            a, b := tile.Polys[pi].Area, tile.Polys[link.To].Area
            if (a == navmesh.AreaGround && b == navmesh.AreaWater) ||
                (a == navmesh.AreaWater && b == navmesh.AreaGround) {
                return true
            }
        }
    }

    return false
}
