// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "log"
    "net/http"
)

// NewTestPathCurveServer creates the web server of the smooth curve animation
// lab: the comparison page of the Catmull-Rom walk motion and drawing ideas, no
// bot and no geodata behind it (the map background of the cells is the static
// tile pyramid that ships with the web content).
func NewTestPathCurveServer(address string, logger *log.Logger) *Server {
    server := newServer(address, logger)

    mux := server.httpServer.Handler.(*http.ServeMux)
    mux.HandleFunc("GET /api/config", server.handlePathCurveConfig)

    return server
}

// handlePathCurveConfig reports the smooth curve lab mode of the web UI.
func (s *Server) handlePathCurveConfig(
    w http.ResponseWriter, _ *http.Request,
) {
    writeJSON(w, s.logger, configResponse{
        Mode:     modeTestPathV2,
        Geodata:  nil,
        MaxSteps: 0,
        Defaults: nil,
    })
}
