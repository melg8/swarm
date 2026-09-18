// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "encoding/binary"
    "fmt"
    "os"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The legacy tile upgrade: the pack builds before the v3 round left
// the gzip wrapped v1/v2 tiles (and the plain tiles of the
// -compress=false runs) behind, and the mtime freshness skip kept
// them in place forever - the runtime reads every version, so the
// pack worked, but the cold route paid the legacy decode (the wide
// gzip tables, the per link Next chains, the BV tree walk) and the
// disk carried the legacy size. The build pass rewrites such tiles
// in place: the decode reads the legacy wire, the encode writes the
// v3 columnar shape and the file lands in one zstd frame - the same
// bytes a fresh build would have written, without the geodata parse
// (the multi gigabyte pack upgrades in minutes, not hours).

// tileUpgrade reports what the legacy pass did with one tile.
type tileUpgrade struct {
    // upgraded answers whether the file was rewritten (false keeps
    // the current shape or reports the err below).
    upgraded bool
    // fromBytes and toBytes are the file sizes before and after the
    // rewrite (fromBytes probes every tile, toBytes fills on a
    // rewrite only).
    fromBytes int64
    toBytes   int64
    // err carries the failure of a kept tile (the corrupt or the
    // foreign file): the pack pass logs it and moves on, the tile
    // stays servable through the legacy decode.
    err error
}

// upgradeLegacyTile rewrites one tile file into the current pack
// shape (the v3 wire in one zstd frame) unless the file already sits
// in that shape. The rewrite is atomic (the tmp file and the
// rename), so a killed build never leaves a half written tile
// behind, and the caller logs the failure without failing the pack.
func upgradeLegacyTile(tilePath string) (tileUpgrade, error) {
    up := tileUpgrade{}
    data, err := os.ReadFile(tilePath) //nolint:gosec // the fixed dir
    if err != nil {
        return up, fmt.Errorf("read the tile: %w", err)
    }
    up.fromBytes = int64(len(data))
    raw, _, err := maybeDecompressTile(data)
    if err != nil {
        return up, fmt.Errorf("decompress: %w", err)
    }
    if len(raw) < 8 {
        return up, fmt.Errorf("%w: %d bytes is too short",
            navmesh.ErrBadTile, len(raw))
    }
    version := binary.LittleEndian.Uint32(raw[4:])
    if version == navmesh.TileWireVersionCurrent && isZstd(data) {
        return up, nil // the current shape: nothing to do
    }
    tile, err := navmesh.DecodeTile(raw)
    if err != nil {
        return up, fmt.Errorf("decode: %w", err)
    }
    encoded, err := navmesh.EncodeTile(tile)
    if err != nil {
        return up, fmt.Errorf("encode: %w", err)
    }
    packed, err := encodeZstd(encoded)
    if err != nil {
        return up, fmt.Errorf("zstd: %w", err)
    }
    tmp := tilePath + ".tmp"
    if err := os.WriteFile(tmp, packed, 0o600); err != nil {
        return up, fmt.Errorf("write the tile: %w", err)
    }
    if err := os.Rename(tmp, tilePath); err != nil {
        return up, fmt.Errorf("replace the tile: %w", err)
    }
    up.upgraded = true
    up.toBytes = int64(len(packed))

    return up, nil
}
