// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"log"
	"net/http"
)

// NewFightServer creates the web server of the fight animation test
// mode: a bot less showcase page that loops a hero versus enemy demo
// fight through every damage visualization variant, so the user can
// compare the ideas side by side in the browser and pick the one to
// implement for the live map. The page needs no game connection and
// no API beyond the mode handshake - the fight loop, the variants
// and the map background run entirely in the browser.
func NewFightServer(address string, logger *log.Logger) *Server {
	server := newServer(address, logger)

	mux := server.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/config", server.handleFightConfig)

	return server
}

// handleFightConfig reports the fight showcase mode of the web UI.
func (s *Server) handleFightConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, configResponse{
		Mode: modeFight,
	})
}
