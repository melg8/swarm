// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "log"
    "net/http"
)

// NewTestPathFlowServer creates the web server of the flow and snake speed lab:
// the comparison page of the rainbow flow and the pulse snake variants, no bot
// and no geodata behind it (the map background of the cells is the static tile
// pyramid that ships with the web content).
func NewTestPathFlowServer(address string, logger *log.Logger) *Server {
    server := newServer(address, logger)

    mux := server.httpServer.Handler.(*http.ServeMux)
    mux.HandleFunc("GET /api/config", server.handlePathFlowConfig)

    return server
}

// handlePathFlowConfig reports the flow and snake lab mode of the web UI.
func (s *Server) handlePathFlowConfig(
    w http.ResponseWriter, _ *http.Request,
) {
    writeJSON(w, s.logger, configResponse{
        Mode:     modeTestPathV3,
        Geodata:  nil,
        MaxSteps: 0,
        Defaults: nil,
    })
}
