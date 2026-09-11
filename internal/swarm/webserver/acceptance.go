// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"net/http"

	"github.com/melg8/swarm/internal/swarm/acceptance"
)

// SetAcceptance attaches the acceptance test manager and registers
// its endpoints. Call it before ListenAndServe; without it the test
// panel stays hidden (the endpoints answer 404).
func (s *Server) SetAcceptance(manager *acceptance.Manager) {
	s.acceptance = manager
	mux := s.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/acceptance/tests", s.handleAcceptanceTests)
	mux.HandleFunc("POST /api/acceptance/tests/{id}/run",
		s.handleAcceptanceRun)
	mux.HandleFunc("POST /api/acceptance/run", s.handleAcceptanceRunAll)
}

// handleAcceptanceTests answers the scenario list with the live
// status, the checks and the log tail of every test.
func (s *Server) handleAcceptanceTests(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, s.acceptance.Tests())
}

// handleAcceptanceRun starts (or restarts) one scenario: the button
// semantics - pressing run during or after a run recreates the bot
// with the same name and the same scenario.
func (s *Server) handleAcceptanceRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.acceptance.Start(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)

		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// runAllRequest is the payload of POST /api/acceptance/run.
type runAllRequest struct {
	Mode string `json:"mode"`
}

// handleAcceptanceRunAll starts every scenario the requested way:
// sequential runs them one after another, parallel starts them all
// at once (each on its own temp account).
func (s *Server) handleAcceptanceRunAll(
	w http.ResponseWriter, r *http.Request,
) {
	var request runAllRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)

		return
	}
	if err := s.acceptance.StartAll(request.Mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	w.WriteHeader(http.StatusAccepted)
}
