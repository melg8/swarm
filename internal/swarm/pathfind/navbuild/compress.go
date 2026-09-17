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
)

// CompressStats summarizes a tile directory compression pass.
type CompressStats struct {
    Count int
    Bytes int64
}

// maybeDecompressTile undoes the gzip wrapping of a tile file when
// the gzip magic word is present (the raw tile bytes otherwise).
func maybeDecompressTile(raw []byte) ([]byte, bool, error) {
    if len(raw) < 2 || raw[0] != 0x1F || raw[1] != 0x8B {
        return raw, false, nil
    }
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

// maybeCompressTile wraps the encoded tile bytes into gzip when the
// source file was compressed (the rewrite keeps the file's format).
func maybeCompressTile(data []byte, compress bool) ([]byte, error) {
    if !compress {
        return data, nil
    }
    var buf bytes.Buffer
    zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
    if err != nil {
        return nil, fmt.Errorf("gzip writer: %w", err)
    }
    if _, err := zw.Write(data); err != nil {
        return nil, fmt.Errorf("gzip write: %w", err)
    }
    if err := zw.Close(); err != nil {
        return nil, fmt.Errorf("gzip close: %w", err)
    }

    return buf.Bytes(), nil
}

// CompressTileDir gzips every plain tile file of the directory in
// place (the runtime detects the gzip magic word and decompresses on
// load). The dense exact square tiles shrink to roughly half; the
// already compressed files are skipped, so the pass is idempotent.
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
        if len(raw) >= 2 && raw[0] == 0x1F && raw[1] == 0x8B {
            // Already compressed.
            continue
        }
        var buf bytes.Buffer
        zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
        if err != nil {
            return stats, fmt.Errorf("gzip writer: %w", err)
        }
        if _, err := zw.Write(raw); err != nil {
            return stats, fmt.Errorf("gzip %s: %w", name, err)
        }
        if err := zw.Close(); err != nil {
            return stats, fmt.Errorf("gzip close %s: %w", name, err)
        }
        compressed := buf.Bytes()
        tmp := path + ".gz.tmp"
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
