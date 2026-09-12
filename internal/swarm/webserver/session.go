// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"net/http"

	"github.com/melg8/swarm/internal/swarm/session"
	"github.com/melg8/swarm/internal/swarm/state"
)

// The session report endpoint of the web UI: the compact plain text
// report of everything the persistent session journal collected about
// one bot since the application start (the hourly curves, the kill and
// death statistics, the money trail, the stalls, the story tail).
// The "session dump" button of the HUD copies it to the clipboard - a
// long-lived run on the user machine hands its whole history to the
// developer agent in one click, no file digging on the user side.

// SetSessionJournal attaches the session journal and registers the
// report endpoint. Call it before ListenAndServe; without a journal
// the endpoint answers 503 (the -session-dir "" mode).
func (s *Server) SetSessionJournal(journal *session.Journal) {
	s.journal = journal
	mux := s.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc(
		"GET /api/bots/{id}/session-report", s.handleSessionReport)
}

// handleSessionReport serves the session report of one bot as plain
// text.
func (s *Server) handleSessionReport(w http.ResponseWriter, r *http.Request) {
	if s.journal == nil {
		http.Error(w, "session journal is disabled",
			http.StatusServiceUnavailable)

		return
	}
	bot, ok := s.lookupBot(w, r)
	if !ok {
		return
	}
	report, err := s.journal.Report(bot.ID(), liveView(bot))
	if err != nil {
		http.Error(w, "no session data: "+err.Error(), http.StatusNotFound)

		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// The report is plain text assembled in process from the journal
	// aggregates (no user input reaches the response body), the same
	// false positive the dump endpoint carries.
	//nolint:gosec // in-process plain text, see above
	if _, err := w.Write([]byte(report)); err != nil {
		s.logger.Printf("Error writing session report: %v", err)
	}
}

// liveView builds the point-in-time block of the report header from
// the tracker.
func liveView(bot *state.Bot) *session.LiveView {
	x, y, _, ok := bot.SelfPosition()
	if !ok {
		x, y = 0, 0
	}

	return &session.LiveView{
		ID:     bot.ID(),
		Status: string(bot.Status()),
		Phase:  bot.Phase(),
		Level:  bot.SelfLevel(),
		ExpPercent: state.ExpPercent(bot.SelfLevel(),
			int64(bot.SelfExp())),
		Health:      bot.SelfHealthPercent(),
		Adena:       int64(bot.InventoryStats().Adena),
		X:           x,
		Y:           y,
		StartedUnix: bot.SessionStartedAt().Unix(),
	}
}
