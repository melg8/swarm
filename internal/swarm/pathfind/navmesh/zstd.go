// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
    "fmt"
    "sync"

    "github.com/klauspost/compress/zstd"
)

// tileZstdDecoders pools the per process zstd decoders: the decode
// tables stay warm across the tile loads (the cold route decodes a
// handful of tiles, the fleet replans decode hundreds).
var tileZstdDecoders = sync.Pool{New: func() any {
    reader, _ := zstd.NewReader(nil,
        zstd.WithDecoderConcurrency(1),
        zstd.WithDecoderMaxMemory(1<<30))

    return reader
}}

// decodeTileZstd unwraps one zstd tile frame (the navmesh-build
// -compress output of the zstd round; the gzip tiles decode through
// the legacy branch of the loader).
func decodeTileZstd(raw []byte) ([]byte, error) {
    reader := tileZstdDecoders.Get().(*zstd.Decoder)
    defer tileZstdDecoders.Put(reader)
    data, err := reader.DecodeAll(raw, nil)
    if err != nil {
        return nil, fmt.Errorf("zstd decode: %w", err)
    }

    return data, nil
}
