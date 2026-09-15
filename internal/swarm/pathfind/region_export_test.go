// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestParseRegionDataAndLayerStack pins the exported region access the
// navmesh builder uses: parsing the real region file of the shipped
// pack through ParseRegionData (outside the engine cache) and reading
// cell stacks through the reusable LayerStack buffer.
func TestParseRegionDataAndLayerStack(t *testing.T) {
    data, err := os.ReadFile(filepath.Join(
        "testdata", "22_22.l2j"))
    require.NoError(t, err)

    region, err := ParseRegionData(data, RegionKey{Col: 22, Row: 22})
    require.NoError(t, err)
    require.NotNil(t, region)

    // The stacks of LayerStack must match the Layers copy for a sample
    // of cells across the region, and the buffer must be reused when
    // it has room.
    buf := make([]Layer, 0, 8)
    reused := true
    for _, local := range []Point{
        {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 17, Y: 33},
        {X: 2047, Y: 2047}, {X: 1024, Y: 1024}, {X: 511, Y: 900},
    } {
        stack := region.LayerStack(local, buf)
        layers := region.Layers(local)
        require.Equal(t, layers, stack, "cell %d %d", local.X, local.Y)
        if cap(stack) != cap(buf) {
            reused = false
        }
        buf = stack
    }
    require.True(t, reused, "the stack buffer must survive the sample")

    // The multilayer cells of the region prove the stacked case: at
    // least one sampled column holds two or more layers.
    stacked := 0
    for localX := int32(0); localX < cellsPerRegionSide; localX += 97 {
        for localY := int32(0); localY < cellsPerRegionSide; localY += 131 {
            stack := region.LayerStack(Point{X: localX, Y: localY}, buf)
            buf = stack
            if len(stack) >= 2 {
                stacked++
            }
        }
    }
    require.Positive(t, stacked, "the region must hold stacked columns")
}
