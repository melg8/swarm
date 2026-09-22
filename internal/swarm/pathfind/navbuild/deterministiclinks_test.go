// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// TestLinkEmitTotalOrder pins the deterministic link emission: the
// emit sort must be a total order over the emitted specs. The stack
// tie (two keys sharing poly, side and t0 with a different target -
// the stacked layer column pairs one polygon with two neighbours on
// the same span) flipped the relative order with the map iteration
// under the partial (poly, side, t0) key, two rebuilds of the same
// region produced differently ordered link arrays and the corpus
// replay jittered on the A* tie-breaking (issue #63).
func TestLinkEmitTotalOrder(t *testing.T) {
    acc := newLinkAccumulator()
    // Enough keys that sort.Slice takes the unstable pdqsort path;
    // every second key ties on (poly, side, t0) with its neighbour
    // through a different target and a shorter run.
    for i := range 64 {
        poly := int32(i / 2)
        span := int32(i % 2 * 8)
        acc.add(poly, navmesh.SideMaxY, int32(100+i), span)
        acc.add(poly, navmesh.SideMaxY, int32(200+i), span)
        acc.add(poly, navmesh.SideMinY, int32(300+i), span)
    }

    want := acc.emit()
    require.NotEmpty(t, want)
    for attempt := range 200 {
        got := acc.emit()
        require.Equal(t, want, got,
            "emit attempt %d reordered the specs: the sort key is not total", attempt)
    }
    // The exact tie-break: the specs sharing (poly, side, t0) rise by
    // t1 then by the target index.
    for i := 1; i < len(want); i++ {
        a, b := want[i-1], want[i]
        if a.poly == b.poly && a.side == b.side && a.t0 == b.t0 {
            require.True(t, a.t1 < b.t1 ||
                (a.t1 == b.t1 && a.to < b.to),
                "the tie (%d,%d,%d) must rise by t1 then to: %+v vs %+v",
                a.poly, a.side, a.t0, a, b)
        }
    }
}

// TestBuildRegionRebuildByteIdentical pins the reproducible build:
// building the same region twice produces byte identical tile wires.
// The stacked layer columns (the pair within the climb of the source
// but beyond the dedup delta) force the link tie the unstable sort
// flipped; the deterministic emission keeps the rebuilds identical.
func TestBuildRegionRebuildByteIdentical(t *testing.T) {
    // Row A rides at -3504; row B stacks -3504 and -3464 (40 apart:
    // beyond the 32 dedup delta, both within the 40 climb of row A),
    // so the row A polygon links both row B polygons on the same
    // spans - the tie that needs the total order.
    data := writeRegionFile(t, func(cx, cy int) []layerSpec {
        if cx < 32 && cy < 1 {
            return []layerSpec{{h: -3504, nswe: 0x0F}}
        }
        if cx < 32 && cy >= 1 && cy < 3 {
            return []layerSpec{
                {h: -3504, nswe: 0x0F},
                {h: -3464, nswe: 0x0F},
            }
        }

        return nil
    })

    var reference []byte
    for attempt := range 8 {
        build, err := BuildRegion(data, 21, 19, DefaultOptions())
        require.NoError(t, err)
        encoded, err := navmesh.EncodeTile(build.Tile)
        require.NoError(t, err)
        if attempt == 0 {
            reference = encoded

            continue
        }
        require.Equal(t, reference, encoded,
            "rebuild attempt %d produced different tile bytes", attempt)
    }
}
