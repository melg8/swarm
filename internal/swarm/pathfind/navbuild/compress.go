// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navbuild

import (
    "bytes"
    "compress/gzip"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "sync"

    "github.com/klauspost/compress/zstd"
)

// CompressStats summarizes a tile directory compression pass.
type CompressStats struct {
    Count int
    Bytes int64
}

// The zstd frame magic word (little endian 0xFD2FB528): the tile
// files carry it since the zstd round, the gzip magic (1F 8B) marks
// the legacy packs the runtime still reads.
var zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

// isZstd reports whether the buffer starts with the zstd frame magic.
func isZstd(raw []byte) bool {
    return len(raw) >= 4 && raw[0] == zstdMagic[0] && raw[1] == zstdMagic[1] &&
        raw[2] == zstdMagic[2] && raw[3] == zstdMagic[3]
}

// zstdDecoders pools the decoders: the zstd reader carries decode
// tables the reuse keeps warm across the tile loads of one process.
var zstdDecoders = sync.Pool{New: func() any {
    reader, _ := zstd.NewReader(nil,
        zstd.WithDecoderConcurrency(1),
        zstd.WithDecoderMaxMemory(1<<30))

    return reader
}}

// decodeZstd unwraps one zstd frame.
func decodeZstd(raw []byte) ([]byte, error) {
    reader := zstdDecoders.Get().(*zstd.Decoder)
    defer zstdDecoders.Put(reader)
    data, err := reader.DecodeAll(raw, nil)

    return data, err
}

// encodeZstd wraps the bytes into one zstd frame. The best
// compression level: the decode cost the cold queries pay does not
// move with the encode level (a zstd property), so the ratio is free
// at runtime and only the pack build time pays it - the measured
// headroom over the old default level was 12.6% of the tile pack and
// 82% of the plain abstract sidecars (issue #11: the default level
// left 111 MB on the tiles, the unwrapped sidecars carried 333 MB of
// compressible zero fields).
var zstdEncoder, zstdEncoderErr = zstd.NewWriter(nil,
    zstd.WithEncoderConcurrency(1),
    zstd.WithEncoderLevel(zstd.SpeedBestCompression))

// encodeZstdOnce encodes through the shared encoder (the encoder is
// not concurrency safe, the callers hold - the pack pass runs one
// compress call at a time inside the per worker file scope; the
// mutex keeps that true if the phases ever overlap).
var zstdEncoderMu sync.Mutex

func encodeZstd(data []byte) ([]byte, error) {
    zstdEncoderMu.Lock()
    defer zstdEncoderMu.Unlock()
    if zstdEncoderErr != nil {
        return nil, fmt.Errorf("zstd encoder: %w", zstdEncoderErr)
    }

    return zstdEncoder.EncodeAll(data, nil), nil
}

// maybeDecompressTile undoes the compression wrapping of a tile file
// when a known magic word is present (the raw tile bytes otherwise).
// The zstd frames are the current format, the gzip frames the legacy
// packs carry.
func maybeDecompressTile(raw []byte) ([]byte, bool, error) {
    if isZstd(raw) {
        data, err := decodeZstd(raw)
        if err != nil {
            return nil, false, fmt.Errorf("zstd decode: %w", err)
        }

        return data, true, nil
    }
    if len(raw) >= 2 && raw[0] == 0x1F && raw[1] == 0x8B {
        zr, err := gzip.NewReader(bytes.NewReader(raw))
        if err != nil {
            return nil, false, fmt.Errorf("gzip reader: %w", err)
        }
        data, err := io.ReadAll(zr)
        if err != nil {
            _ = zr.Close()

            return nil, false, fmt.Errorf("gzip read: %w", err)
        }
        if err := zr.Close(); err != nil {
            return nil, false, fmt.Errorf("gzip close: %w", err)
        }

        return data, true, nil
    }

    return raw, false, nil
}

// maybeCompressTile wraps the encoded tile bytes into zstd when the
// source file was compressed (the rewrite keeps the file's format).
func maybeCompressTile(data []byte, compress bool) ([]byte, error) {
    if !compress {
        return data, nil
    }

    return encodeZstd(data)
}

// CompressTileDir compresses every plain tile file of the directory
// in place (the runtime detects the magic word and decompresses on
// load). The dense exact square tiles shrink to roughly half; the
// already compressed files (zstd or the legacy gzip) are skipped, so
// the pass is idempotent.
func CompressTileDir(outDir string) (CompressStats, error) {
    stats := CompressStats{Count: 0, Bytes: 0}
    entries, err := os.ReadDir(outDir)
    if err != nil {
        return stats, fmt.Errorf("read the tile directory: %w", err)
    }
    for _, entry := range entries {
        name := entry.Name()
        if entry.IsDir() || !strings.HasSuffix(name, ".nm") {
            continue
        }
        path := filepath.Join(outDir, name)
        raw, err := os.ReadFile(path)
        if err != nil {
            return stats, fmt.Errorf("read %s: %w", name, err)
        }
        if isZstd(raw) || (len(raw) >= 2 && raw[0] == 0x1F && raw[1] == 0x8B) {
            // Already compressed.
            continue
        }
        compressed, err := encodeZstd(raw)
        if err != nil {
            return stats, fmt.Errorf("zstd %s: %w", name, err)
        }
        tmp := path + ".zs.tmp"
        //nolint:gosec // the path is the data dir the operator passed
        // to the compress pass, the walk owns the tree.
        if err := os.WriteFile(tmp, compressed, 0o600); err != nil {
            return stats, fmt.Errorf("write %s: %w", tmp, err)
        }
        if err := os.Rename(tmp, path); err != nil {
            return stats, fmt.Errorf("rename %s: %w", tmp, err)
        }
        stats.Count++
        stats.Bytes += int64(len(raw) - len(compressed))
    }

    return stats, nil
}
