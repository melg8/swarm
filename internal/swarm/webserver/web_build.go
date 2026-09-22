// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "crypto/sha256"
    "encoding/hex"
    "io/fs"
    "net/http"
    "strings"
)

// The build identity of the embedded web interface (issue #48): the
// server hashes the embedded web content and the page carries the
// same id through a meta tag the server injects into index.html. The
// /api/bots poll reports the id of the RUNNING binary through the
// X-Swarm-Build header, so a tab that outlived a deployment notices
// the mismatch and reloads itself into the new version instead of
// silently driving the map with the old assets.

// webBuildID hashes the whole file tree of the embedded web content:
// the hex sha256 over the sorted file paths and the file bytes. The
// id changes exactly when the served content changes - no git, no
// ldflags, no build plumbing, the embed itself is the source of
// truth.
func webBuildID(fsys fs.FS) string {
    hash := sha256.New()
    entries := make([]string, 0, 64)
    walk := func(path string, d fs.DirEntry, walkErr error) error {
        if walkErr != nil {
            return walkErr
        }
        if !d.IsDir() {
            entries = append(entries, path)
        }

        return nil
    }
    err := fs.WalkDir(fsys, ".", walk)
    if err != nil || len(entries) == 0 {
        // An unreadable or empty tree: the constant id disables the
        // reload check (the client reloads only on a REAL mismatch
        // against a non empty meta).
        return ""
    }
    for _, path := range entries {
        content, readErr := fs.ReadFile(fsys, path)
        if readErr != nil {
            return ""
        }
        hash.Write([]byte(path))
        hash.Write([]byte{0})
        hash.Write(content)
        hash.Write([]byte{0})
    }

    return hex.EncodeToString(hash.Sum(nil))[:16]
}

// injectBuildMeta inserts the build meta tag into the head of the
// index page: the page reads its own build id from the DOM at boot
// (the script needs no extra request to learn it). A page without a
// head marker passes through unchanged.
func injectBuildMeta(indexHTML string, build string) string {
    if build == "" {
        return indexHTML
    }
    meta := "<meta name=\"swarm-build\" content=\"" + build + "\">"
    if strings.Contains(indexHTML, meta) {
        return indexHTML
    }
    const headMarker = "<head>"
    index := strings.Index(indexHTML, headMarker)
    if index < 0 {
        return indexHTML
    }
    at := index + len(headMarker)

    return indexHTML[:at] + "\n  " + meta + indexHTML[at:]
}

// staticHandler wraps the embedded file server with the build
// plumbing of the reload check (issue #48):
//
//   - "/" and "/index.html" serve the prebuilt index page (the
//     injected meta) with no-cache, so a reload always picks the
//     binary's own page;
//   - every script and style sheet answers with no-cache too - the
//     embedded files carry no validator (a zero modtime), so without
//     the header a browser could heuristically keep the OLD asset
//     even after the reload and the page would still run the stale
//     code;
//   - everything else (the map tiles, the icons, the meshes) passes
//     through untouched - they are content named, they change on
//     their own schedule, and caching them is what keeps the map
//     fast.
func (s *Server) staticHandler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        isIndex := r.URL.Path == "/" ||
            r.URL.Path == "/index.html"
        if s.webBuild != "" && isIndex {
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            w.Header().Set("Cache-Control", "no-cache")
            w.Header().Set("X-Swarm-Build", s.webBuild)
            _, _ = w.Write(s.indexPage)

            return
        }
        if strings.HasSuffix(r.URL.Path, ".js") ||
            strings.HasSuffix(r.URL.Path, ".css") ||
            strings.HasSuffix(r.URL.Path, ".html") {
            w.Header().Set("Cache-Control", "no-cache")
        }
        next.ServeHTTP(w, r)
    })
}
