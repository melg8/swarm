// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"log"
	"net/http"
)

// NewTestFightServer creates the web server of the fight FX test mode:
// the static comparison gallery of the combat damage visualizations, no
// bot and no geodata behind it (the map background of the cells is the
// static tile pyramid that ships with the web content).
func NewTestFightServer(address string, logger *log.Logger) *Server {
	server := newServer(address, logger)

	mux := server.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/config", server.handleFightGalleryConfig)

	return server
}

// handleFightConfig reports the fight test mode of the web UI.
func (s *Server) handleFightGalleryConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, configResponse{
		Mode:     modeTestFight,
		Geodata:  nil,
		MaxSteps: 0,
		Defaults: nil,
	})
}
