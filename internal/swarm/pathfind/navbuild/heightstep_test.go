// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestSnapHeight pins the height quantization rounding: the nearest
// multiple of the step, the half step ties round away from zero (the
// negative heights of the world keep the symmetric tie rule).
func TestSnapHeight(t *testing.T) {
    cases := []struct {
        in   int16
        step int32
        want int16
    }{
        {in: -2000, step: 16, want: -2000}, // already on the step
        {in: -2008, step: 16, want: -2016}, // the tie rounds away
        {in: -2004, step: 16, want: -2000}, // nearest wins
        {in: 2000, step: 16, want: 2000},
        {in: 2008, step: 16, want: 2016}, // the positive tie
        {in: -2000, step: 32, want: -2016},
        {in: -2008, step: 32, want: -2016}, // the pair collapses
        {in: 0, step: 40, want: 0},
        {in: -3504, step: 40, want: -3520},
    }
    for _, c := range cases {
        require.Equal(t, c.want, snapHeight(c.in, c.step),
            "snapHeight(%d, %d)", c.in, c.step)
    }
}

// TestExtractRegionHeightStepSnapsLayers pins the snap point of the
// quantization: the cell layers carry the snapped heights before the
// dedup and the decomposition run, so the whole downstream sees the
// coarser grid as the geodata truth.
func TestExtractRegionHeightStepSnapsLayers(t *testing.T) {
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 32 && cy < 32 {
            return []layerSpec{{h: -2000, nswe: 0x0F}}
        }

        return nil
    })

    // Without the step the exact geodata height survives.
    rl, err := extractRegion(data, 21, 19,
        DefaultOptions().DedupDelta, 0)
    require.NoError(t, err)
    stack := stackOf(t, rl, 0, 0)
    require.Len(t, stack, 1)
    require.Equal(t, int16(-2000), stack[0].h)

    // The step 40 snaps the surface onto the coarser grid: -2024
    // sits 16 under the -2040 multiple and 24 over the -2000 one.
    data2024 := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 32 && cy < 32 {
            return []layerSpec{{h: -2024, nswe: 0x0F}}
        }

        return nil
    })
    rl, err = extractRegion(data2024, 21, 19,
        DefaultOptions().DedupDelta, 40)
    require.NoError(t, err)
    stack = stackOf(t, rl, 0, 0)
    require.Len(t, stack, 1)
    require.Equal(t, int16(-2040), stack[0].h,
        "-2024 snaps to the nearest multiple of 40")
}

// TestBuildRegionHeightStepMergesNearEqual pins the size effect the
// evaluation measures: the near equal surfaces the exact height rule
// fragments into per cell rectangles become one flat rectangle under
// the coarser step - the polygon count the pack wire carries drops.
func TestBuildRegionHeightStepMergesNearEqual(t *testing.T) {
    // The open floor of two interleaved heights 8 units apart: the
    // exact height decomposition keeps every cell its own rectangle,
    // the step 32 snaps both surfaces onto -2016 and the floor
    // becomes a single rectangle.
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 32 && cy < 32 {
            if (cx+cy)%2 == 0 {
                return []layerSpec{{h: -2000, nswe: 0x0F}}
            }

            return []layerSpec{{h: -2008, nswe: 0x0F}}
        }

        return nil
    })

    exact, err := BuildRegion(data, 21, 19, DefaultOptions())
    require.NoError(t, err)

    quantized, err := BuildRegion(data, 21, 19, Options{
        Climb:      40,
        DedupDelta: 32,
        HeightStep: 32,
    })
    require.NoError(t, err)

    groundPolys := func(build *RegionBuild) int {
        n := 0
        for i := range build.Tile.Polys {
            if build.Tile.Polys[i].Area == 0 {
                n++
            }
        }

        return n
    }
    require.Equal(t, 32*32, groundPolys(exact),
        "the checkerboard fragments into per cell rectangles")
    require.Equal(t, 1, groundPolys(quantized),
        "the snapped checkerboard is one flat rectangle")

    // The merged rectangle spans the whole patch at the snapped
    // height: the walkable surface is the same ground the geodata
    // carried, the wire just stores it once.
    require.Len(t, quantized.Tile.Polys, 1)
    poly := &quantized.Tile.Polys[0]
    require.Equal(t, int32(0), poly.X0)
    require.Equal(t, int32(32), poly.X1)
    require.Equal(t, int32(32), poly.Y1)
    require.Equal(t, int16(-2016), poly.H00)
    require.Equal(t, int16(-2016), poly.H11)
}
