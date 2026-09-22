// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the pure helpers of the counter probe
// - the id parse, the region path arithmetic and the region key
// readback of the file names.

func TestParseID(t *testing.T) {
    t.Run("a decimal id parses", func(t *testing.T) {
        require.Equal(t, int64(7155), parseID("7155"))
        require.Equal(t, int64(-7), parseID("-7"))
    })

    t.Run("a non numeric value answers zero", func(t *testing.T) {
        require.Zero(t, parseID("abc"))
        require.Zero(t, parseID(""))
    })
}

func TestAbs(t *testing.T) {
    t.Run("negative values flip", func(t *testing.T) {
        require.Equal(t, 5, abs(-5))
        require.Equal(t, 5, abs(5))
        require.Zero(t, abs(0))
    })
}

func TestRegionPathAndKeyOf(t *testing.T) {
    t.Run("the path lands in the region of the world position", func(t *testing.T) {
        path := regionPath("data/geodata", 46112, 41500)
        key := pathfind.CellToRegion(
            pathfind.WorldToCell(46112, 41500))
        require.Equal(t,
            "data/geodata/"+itoa(int(key.Col))+"_"+itoa(int(key.Row))+".l2j",
            path)
    })

    t.Run("the key reads back out of the path", func(t *testing.T) {
        key := regionKeyOf("data/geodata/24_19.l2j")
        require.Equal(t, pathfind.RegionKey{Col: 24, Row: 19}, key)
    })

    t.Run("a broken tail answers the zero key", func(t *testing.T) {
        key := regionKeyOf("data/geodata/broken")
        require.Equal(t, pathfind.RegionKey{Col: 0, Row: 0}, key)
    })
}

// itoa keeps the test free of the fmt dependency in the assertion.
func itoa(v int) string {
    if v == 0 {
        return "0"
    }
    neg := v < 0
    if neg {
        v = -v
    }
    digits := ""
    for v > 0 {
        digits = string(rune('0'+v%10)) + digits
        v /= 10
    }
    if neg {
        return "-" + digits
    }

    return digits
}
