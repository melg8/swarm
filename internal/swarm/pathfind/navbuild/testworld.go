// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "encoding/binary"
    "testing"
)

// layerSpec is one synthetic cell layer.
type layerSpec struct {
    h    int16
    nswe uint8
}

// encodeCellWord packs a layer into the l2j cell word: the height in
// bits 4..15 (the decode is int16(word&0xFFF0)>>1, so the height must
// be a multiple of 8 - the l2j granularity), the NSWE walls in the
// low four bits.
func encodeCellWord(layer layerSpec) uint16 {
    if layer.h%8 != 0 {
        panic("the synthetic l2j height must be a multiple of 8")
    }

    return ((uint16(layer.h) << 1) & 0xFFF0) | uint16(layer.nswe)
}

// writeRegionFile serializes a synthetic region: the grid answers the
// layer stack of every cell (nil answers a fully blocked void layer).
// The writer picks the cheapest block kind per block: flat for
// uniform open cells, complex for single layer cells, multilayer
// otherwise.
func writeRegionFile(t *testing.T,
    grid func(cx, cy int) []layerSpec,
) []byte {
    t.Helper()
    buffer := make([]byte, 0, 1<<20)
    for bx := range 256 {
        for by := range 256 {
            buffer = appendBlock(t, buffer, bx, by, grid)
        }
    }

    return buffer
}

// appendBlock writes one 8x8 block of the region.
func appendBlock(t *testing.T, buffer []byte, bx, by int,
    grid func(cx, cy int) []layerSpec,
) []byte {
    t.Helper()
    stacks := make([][]layerSpec, 64)
    uniform := true
    var first layerSpec
    for lx := range 8 {
        for ly := range 8 {
            stack := grid(bx*8+lx, by*8+ly)
            if stack == nil {
                stack = []layerSpec{{h: -3504, nswe: 0}}
            }
            stacks[lx*8+ly] = stack
            if len(stack) == 0 {
                // The true void cell (zero layers): it forces the
                // multilayer block kind, the body writes the zero
                // layer count for it.
                uniform = false

                continue
            }
            if lx == 0 && ly == 0 {
                first = stack[0]
            }
            if len(stack) != 1 || stack[0] != first {
                uniform = false
            }
        }
    }
    if uniform && first.nswe == 0x0F {
        // The flat block stores the raw height (no cell word
        // encoding) and implies open walls for all 64 cells.
        buffer = append(buffer, 0)
        var word [2]byte
        binary.LittleEndian.PutUint16(word[:], uint16(first.h))
        buffer = append(buffer, word[:]...)

        return buffer
    }
    if uniform {
        // A uniform blocked stack still fits the complex block.
        buffer = append(buffer, 1)
        for cell := range 64 {
            buffer = appendCellWord(buffer, stacks[cell][0])
        }

        return buffer
    }
    buffer = append(buffer, 2)
    for cell := range 64 {
        buffer = append(buffer, byte(len(stacks[cell])))
        for _, layer := range stacks[cell] {
            buffer = appendCellWord(buffer, layer)
        }
    }

    return buffer
}

// appendCellWord appends one encoded cell word.
func appendCellWord(buffer []byte, layer layerSpec) []byte {
    var word [2]byte
    binary.LittleEndian.PutUint16(word[:], encodeCellWord(layer))

    return append(buffer, word[:]...)
}

// miniWorld is the synthetic floating village of the builder tests:
// a dry mainland plateau, a six step shore ramp descending toward the
// water level, the water itself and a floating deck stacked over the
// water with no connection. The shore wall closes the first ten cells
// of the ramp edge, so the only dry crossing into the ramp sits at
// x 10..20 (the NSWE portal span case).
//
// The heights (multiples of 8, the l2j granularity): the plateau
// rides at -3504, the ramp steps -40 per row down to -3744, the water
// floor sits at -3784 (below the C1 water level -3780) and the deck
// floats at -3504 over the water.
func miniWorld(t *testing.T) []byte {
    t.Helper()
    rampHeights := []int16{-3544, -3584, -3624, -3664, -3704, -3744}

    return writeRegionFile(t, func(cx, cy int) []layerSpec {
        switch {
        case cx < 20 && cy < 10:
            return []layerSpec{{h: -3504, nswe: wallSouthOfRamp(cx, cy)}}
        case cx < 20 && cy >= 10 && cy < 16:
            return []layerSpec{{h: rampHeights[cy-10], nswe: 0x0F}}
        case cx < 20 && cy >= 16 && cy < 20:
            // The water column with the floating deck above it.
            return []layerSpec{
                {h: -3784, nswe: 0x0F},
                {h: -3504, nswe: 0x0F},
            }
        default:
            return nil
        }
    })
}

// wallSouthOfRamp closes the south wall of the plateau cells west of
// x 10: the ramp edge is passable only through the eastern half.
func wallSouthOfRamp(cx, cy int) uint8 {
    if cy == 9 && cx < 10 {
        return 0x0F &^ nsweSouth
    }

    return 0x0F
}
