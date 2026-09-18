// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// TestSidecarPairProbe walks one neighbour pair's border strips and
// reports the entry counts, the height and nswe histograms and the
// pairing filter results - the diagnostic round for the missing water
// border links of the pack.
func TestSidecarPairProbe(t *testing.T) {
    outDir := "../../../../data/navmesh"
    ownKey := navmesh.RegionKey{Col: 18, Row: 21}
    nbKey := navmesh.RegionKey{Col: 17, Row: 21}
    own, ok := readStripsSidecar(outDir, ownKey)
    if !ok {
        // The diagnostic answers against the pack sidecars: the
        // pack builds in data/navmesh (gitignored), a fresh
        // checkout skips the probe.
        t.Skip("the pack sidecar is not present (the tile pack " +
            "builds into data/navmesh)")
    }
    nb, ok := readStripsSidecar(outDir, nbKey)
    if !ok {
        t.Skip("the neighbour sidecar is not present")
    }
    west := &own.strips[stripWest]
    east := &nb.strips[stripEast]
    t.Logf("18_21 west strip entries: %d (offset tail %d)",
        len(west.entries), west.offsets[regionCellsSide])
    t.Logf("17_21 east strip entries: %d (offset tail %d)",
        len(east.entries), east.offsets[regionCellsSide])

    hist := func(entries []borderEntry) (map[int16]int, map[uint8]int) {
        hs := map[int16]int{}
        ns := map[uint8]int{}
        for _, e := range entries {
            hs[e.h]++
            ns[e.nswe]++
        }

        return hs, ns
    }
    ownH, ownN := hist(west.entries)
    nbH, nbN := hist(east.entries)
    if len(ownH) > 8 {
        t.Logf("own heights: %d distinct, top sample: %v", len(ownH),
            ownH)
    } else {
        t.Logf("own heights: %v", ownH)
    }
    t.Logf("own nswe: %v", ownN)
    t.Logf("nb heights: %d distinct", len(nbH))
    t.Logf("nb nswe: %v", nbN)

    climb := int32(DefaultOptions().Climb)
    paired, heightSkip, nsweSkip := 0, 0, 0
    for pos := 0; pos < regionCellsSide; pos++ {
        for i := west.offsets[pos]; i < west.offsets[pos+1]; i++ {
            a := west.entries[i]
            for j := east.offsets[pos]; j < east.offsets[pos+1]; j++ {
                b := east.entries[j]
                if abs16(a.h-b.h) > climb {
                    heightSkip++

                    continue
                }
                if !nsweOpen(cellLayer{h: a.h, nswe: a.nswe},
                    cellLayer{h: b.h, nswe: b.nswe}, -1, 0) {
                    nsweSkip++

                    continue
                }
                paired++
            }
        }
    }
    t.Logf("pairing over the border: paired %d, height skipped %d,"+
        " nswe skipped %d, climb %d", paired, heightSkip, nsweSkip,
        climb)
}
