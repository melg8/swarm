// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "net/http"
    "strconv"
)

// handleHuntMesh serves the static Voronoi hunt mesh of the cell
// hunting: the partition payload every cell-mode bot of the process
// published through the tracker (the registry is the same for the
// whole fleet, the first bot installs it). The response carries the
// mesh version as the ETag: the client fetches once per version,
// the per second snapshots only tell it WHICH cell the bot holds -
// the mesh bytes never ride the snapshot stream. A registry without
// a cell-mode bot answers 404 (the legacy zone mode draws no mesh).
func (s *Server) handleHuntMesh(w http.ResponseWriter, r *http.Request) {
    version, payload := "", []byte(nil)
    for _, bot := range s.registry.Bots() {
        version, payload = bot.HuntMesh()
        if version != "" {
            break
        }
    }
    if version == "" {
        http.Error(w, "no hunting mesh installed", http.StatusNotFound)

        return
    }
    etag := `"` + version + `"`
    if r.Header.Get("If-None-Match") == etag {
        w.WriteHeader(http.StatusNotModified)

        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("ETag", etag)
    w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
    if r.Method == http.MethodHead {
        w.WriteHeader(http.StatusOK)

        return
    }
    _, _ = w.Write(payload)
}
