// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "errors"
    "fmt"
    "sort"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// Options are the builder tunables. The defaults mirror the research
// round (docs/recast_pathfinding.md): the 40 unit climb of the server
// HEIGHT_INCREASE_LIMIT, the 32 unit dedup delta of the l2j duplicate
// noise (the measured region noise sits at 16/24/32 unit deltas -
// both-open pairs of the same surface; the elven region of the
// research round showed none of it, the 22_xx column shows 54k pairs
// per region) and the 4 layer island filter. The rectangle polygons
// carry no height tunable: they merge only the cells of one exact
// geodata height (the faithful square port of the owner directive).
type Options struct {
    Climb          int32
    DedupDelta     int32
    MinSheetLayers int32
    // Workers caps the pack build parallelism (0: runtime.NumCPU()).
    Workers int
    // RepairFake replaces the l2j uninitialized filler cells (one
    // layer, height 0, fully open) with the nearest real surface
    // blend (fakerepair.go): the bay filler turns into the sea floor
    // water, the land gaps into the connecting ground.
    RepairFake bool
}

// DefaultOptions returns the production tunables.
func DefaultOptions() Options {
    return Options{
        Climb:          40,
        DedupDelta:     32,
        MinSheetLayers: 4,
    }
}

// BuildStats is the audit summary of one region build.
type BuildStats struct {
    Layers           int
    StackedColumns   int
    UnderwaterLayers int
    Sheets           int
    DroppedSheets    int
    DroppedLayers    int
    IslandSheets     int
    IslandLayers     int
    Polys            int
    WaterPolys       int
    Links            int
    NSWEBlockedPairs int
    FakeFilled       int
    BuildTime        time.Duration
}

// RegionBuild is the phase A result of one region: the tile with the
// polygons, the internal links and the spatial index (no external
// links yet), plus the border strips the phase B stitching pairs
// against the neighbours.
type RegionBuild struct {
    Tile   *navmesh.Tile
    Strips borderStrips
    Stats  BuildStats
}

// BuildRegion runs the offline build of one region: the parse and
// dedup, the sheet decomposition, the maximal same-height rectangle
// polygons, the internal links with the NSWE portal spans and the
// bounding volume tree.
func BuildRegion(
    data []byte, col, row int16, opts Options,
) (*RegionBuild, error) {
    started := time.Now()
    rl, err := extractRegion(data, col, row, opts.DedupDelta)
    if err != nil {
        return nil, err
    }
    fakeFilled := 0
    if opts.RepairFake {
        fakeFilled = repairFakeCells(rl, opts.Climb)
    }
    sh := assignSheets(rl, opts.Climb, opts.MinSheetLayers)
    rects, polyAt := buildRects(rl, sh, opts.Climb)
    acc, strips := buildInternalLinks(rl, sh, polyAt, opts.Climb)
    specs := acc.emit()

    tile := assembleTile(col, row, opts.Climb, rects, specs)

    stats := collectStats(rl, sh, acc, tile)
    stats.FakeFilled = fakeFilled
    stats.BuildTime = time.Since(started)

    return &RegionBuild{Tile: tile, Strips: strips, Stats: stats}, nil
}

// assembleTile converts the rectangle polygons into the wire tile and
// chains the link specs.
func assembleTile(col, row int16, climb int32, rects []rectPoly,
    specs []linkSpec,
) *navmesh.Tile {
    tile := &navmesh.Tile{
        Col:      col,
        Row:      row,
        Climb:    climb,
        Polys:    make([]navmesh.Poly, len(rects)),
        Links:    make([]navmesh.Link, 0, len(specs)),
        ExtLinks: nil,
        BVTree:   nil,
    }
    for i, rect := range rects {
        tile.Polys[i] = navmesh.Poly{
            X0: rect.x0, Y0: rect.y0, X1: rect.x1, Y1: rect.y1,
            H00: rect.h00, H10: rect.h10, H01: rect.h01,
            H11:       rect.h11,
            FirstLink: -1,
            Area:      rect.area,
        }
    }
    for _, spec := range specs {
        appendLink(tile, spec)
    }

    return tile
}

// StitchRegion runs the phase B of one region: the external links
// against the neighbour strips are appended to the decoded tile. The
// neighbours array is indexed by the own side (see stitchBorders).
func StitchRegion(tile *navmesh.Tile, own borderStrips,
    neighbors [4]*borderStrips, opts Options,
) (int, error) {
    if tile == nil {
        return 0, errors.New("stitch region: the tile is nil")
    }

    return stitchBorders(tile, own, neighbors, opts.Climb), nil
}

// collectStats summarizes the build for the audits.
func collectStats(rl *regionLayers, sh *sheets, acc *linkAccumulator,
    tile *navmesh.Tile,
) BuildStats {
    stats := BuildStats{
        Layers:           len(rl.layers),
        StackedColumns:   0,
        UnderwaterLayers: 0,
        Sheets:           sh.count,
        DroppedSheets:    droppedSheetCount(sh),
        DroppedLayers:    droppedLayerCount(sh),
        IslandSheets:     islandSheetCount(sh),
        IslandLayers:     islandLayerCount(sh),
        Polys:            len(tile.Polys),
        WaterPolys:       0,
        Links:            len(tile.Links),
        NSWEBlockedPairs: acc.blockedPairs,
        BuildTime:        0,
    }
    for idx := range rl.cellCnt {
        if rl.cellCnt[idx] >= 2 {
            stats.StackedColumns++
        }
    }
    for _, layer := range rl.layers {
        if layer.h < waterLevel {
            stats.UnderwaterLayers++
        }
    }
    for i := range tile.Polys {
        if tile.Polys[i].Area == navmesh.AreaWater {
            stats.WaterPolys++
        }
    }

    return stats
}

// droppedSheetCount counts the island sheets.
func droppedSheetCount(sh *sheets) int {
    count := 0
    for s := range sh.count {
        if sh.dropped[s] {
            count++
        }
    }

    return count
}

// islandSheetCount counts the floating component drops.
func islandSheetCount(sh *sheets) int {
    count := 0
    for s := range sh.count {
        if sh.island[s] {
            count++
        }
    }

    return count
}

// islandLayerCount counts the layers of the floating components.
func islandLayerCount(sh *sheets) int {
    count := 0
    for _, sheet := range sh.sheetOf {
        if sheet >= 0 && sh.island[sheet] {
            count++
        }
    }

    return count
}

// droppedLayerCount counts the layers of the dropped sheets.
func droppedLayerCount(sh *sheets) int {
    count := 0
    for _, sheet := range sh.sheetOf {
        if sheet >= 0 && sh.dropped[sheet] {
            count++
        }
    }

    return count
}

// StitchAll stitches every build of the map against its present
// neighbours (the phase B of the offline cmd): every region pairs
// with the west/east/north/south builds that exist, a missing
// neighbour stays a wall the runtime re-stitches lazily when its tile
// appears. The answer is the total count of the external links added.
func StitchAll(builds map[navmesh.RegionKey]*RegionBuild,
    opts Options,
) (int, error) {
    keys := make([]navmesh.RegionKey, 0, len(builds))
    for key := range builds {
        keys = append(keys, key)
    }
    sort.Slice(keys, func(i, j int) bool {
        if keys[i].Col != keys[j].Col {
            return keys[i].Col < keys[j].Col
        }

        return keys[i].Row < keys[j].Row
    })

    added := 0
    for _, key := range keys {
        build := builds[key]
        var neighbors [4]*borderStrips
        west := builds[navmesh.RegionKey{Col: key.Col - 1, Row: key.Row}]
        if west != nil {
            neighbors[stripWest] = &west.Strips
        }
        east := builds[navmesh.RegionKey{Col: key.Col + 1, Row: key.Row}]
        if east != nil {
            neighbors[stripEast] = &east.Strips
        }
        north := builds[navmesh.RegionKey{Col: key.Col, Row: key.Row - 1}]
        if north != nil {
            neighbors[stripNorth] = &north.Strips
        }
        south := builds[navmesh.RegionKey{Col: key.Col, Row: key.Row + 1}]
        if south != nil {
            neighbors[stripSouth] = &south.Strips
        }
        count, err := StitchRegion(build.Tile, build.Strips, neighbors,
            opts)
        if err != nil {
            return added, fmt.Errorf("region %d_%d: %w", key.Col, key.Row,
                err)
        }
        added += count
    }

    return added, nil
}
