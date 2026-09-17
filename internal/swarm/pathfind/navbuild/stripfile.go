// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "encoding/binary"
    "fmt"
    "os"
    "path/filepath"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The border strip sidecar: the phase A build persists the border
// strips of every region next to its tile file (X_Y.nsb). The phase B
// stitching pairs strips of neighbouring regions - the sidecar keeps
// that possible across pack build passes (the chunked builds of the
// 165 region pack): a tile built in an earlier pass contributes its
// sidecar strip instead of the in-pass build. The strips describe the
// border cells of the region - appending the external links to the
// tile never changes them, so the sidecar stays valid for the tile
// lifetime.
const (
    stripsMagic   = 0x534E5753 // 'SWNS' little endian
    stripsVersion = 1
    // sidecarExt is the sidecar file extension under the tile dir.
    sidecarExt = ".nsb"
    // stripEntryWireSize packs h (2), nswe (1), poly (4).
    stripEntryWireSize = 7
)

// sidecarPath returns the strip sidecar path of a region key.
func sidecarPath(outDir string, key navmesh.RegionKey) string {
    return filepath.Join(outDir,
        fmt.Sprintf("%d_%d%s", key.Col, key.Row, sidecarExt))
}

// writeStripsSidecar encodes the border strips of a region into the
// sidecar file.
func writeStripsSidecar(outDir string, key navmesh.RegionKey,
    strips borderStrips,
) error {
    // Header 16, per side 2049*4 offsets plus the packed entries.
    exact := 16
    for side := range strips.strips {
        exact += 4 * (regionCellsSide + 1)
        exact += stripEntryWireSize * len(strips.strips[side].entries)
    }
    data := make([]byte, exact)

    binary.LittleEndian.PutUint32(data[0:], stripsMagic)
    binary.LittleEndian.PutUint32(data[4:], stripsVersion)
    offset := 16
    for side := range strips.strips {
        strip := &strips.strips[side]
        for _, off := range strip.offsets {
            binary.LittleEndian.PutUint32(data[offset:], off)
            offset += 4
        }
        for _, entry := range strip.entries {
            binary.LittleEndian.PutUint16(data[offset:], uint16(entry.h))
            data[offset+2] = entry.nswe
            binary.LittleEndian.PutUint32(data[offset+3:], entry.poly)
            offset += stripEntryWireSize
        }
    }
    if err := os.WriteFile(sidecarPath(outDir, key), data[:offset],
        0o600); err != nil {
        return fmt.Errorf("write the strip sidecar: %w", err)
    }

    return nil
}

// readStripsSidecar decodes the border strips of a region from the
// sidecar file (false when the file is absent or malformed).
func readStripsSidecar(outDir string, key navmesh.RegionKey,
) (borderStrips, bool) {
    data, err := os.ReadFile(sidecarPath(outDir, key))
    if err != nil || len(data) < 16 {
        return borderStrips{strips: [4]borderStrip{}}, false
    }
    if binary.LittleEndian.Uint32(data[0:]) != stripsMagic ||
        binary.LittleEndian.Uint32(data[4:]) != stripsVersion {
        return borderStrips{strips: [4]borderStrip{}}, false
    }
    var strips borderStrips
    offset := 16
    for side := range strips.strips {
        strip := &strips.strips[side]
        for pos := range strip.offsets {
            if offset+4 > len(data) {
                return borderStrips{strips: [4]borderStrip{}}, false
            }
            strip.offsets[pos] = binary.LittleEndian.Uint32(data[offset:])
            offset += 4
        }
        count := int(strip.offsets[regionCellsSide])
        strip.entries = make([]borderEntry, count)
        for i := range strip.entries {
            if offset+stripEntryWireSize > len(data) {
                return borderStrips{strips: [4]borderStrip{}}, false
            }
            strip.entries[i] = borderEntry{
                h:    int16(binary.LittleEndian.Uint16(data[offset:])),
                nswe: data[offset+2],
                poly: binary.LittleEndian.Uint32(data[offset+3:]),
            }
            offset += stripEntryWireSize
        }
    }

    return strips, true
}
