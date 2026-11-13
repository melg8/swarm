// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "log"
    "net/http"
)

// NewTestPathServer creates the web server of the walk plan visualization
// gallery: the animated comparison page of the walk plan drawing ideas, no bot
// and no geodata behind it (the map background of the cells is the static tile
// pyramid that ships with the web content).
func NewTestPathServer(address string, logger *log.Logger) *Server {
    server := newServer(address, logger)

    mux := server.httpServer.Handler.(*http.ServeMux)
    mux.HandleFunc("GET /api/config", server.handlePathGalleryConfig)

    return server
}

// handlePathGalleryConfig reports the walk plan gallery mode of the web UI.
func (s *Server) handlePathGalleryConfig(
    w http.ResponseWriter, _ *http.Request,
) {
    writeJSON(w, s.logger, configResponse{
        Mode:     modeTestPath,
        Geodata:  nil,
        MaxSteps: 0,
        Defaults: nil,
    })
}
