// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/session"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// TestSessionReportDisabled verifies the 503 answer without a journal
// (the -session-dir "" mode).
func TestSessionReportDisabled(t *testing.T) {
	server, _ := newTestServer(t)
	server.SetSessionJournal(nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/session-report", nil)
	server.httpServer.Handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "session journal is disabled")
}

// TestSessionReportNotFound verifies the 404 answer of an unknown bot.
func TestSessionReportNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	journal, err := session.NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	defer journal.Close()
	server.SetSessionJournal(journal)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet, "/api/bots/nobody/session-report", nil)
	server.httpServer.Handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code)
}

// TestSessionReportRenders verifies the report of a bot with journal
// events: the live header block and the sections render. The report
// endpoint reads the aggregate the flusher builds from the queue, so
// the assertions poll until the kill lands.
func TestSessionReportRenders(t *testing.T) {
	server, bot := newTestServer(t)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, Race: 1, ClassID: 18,
		X: 45000, Y: 50000, Z: -3500,
		Exp: 2500, MaxHP: 100, CurHP: 90, MaxMP: 50, CurMP: 40,
		CurrentLoad: 10, MaxLoad: 100,
	})
	journal, err := session.NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	defer journal.Close()
	server.SetSessionJournal(journal)
	journal.Kill("test1", "Kaboo Orc", 4, 8.2, 87.5)
	journal.Story("test1", "Hunt: the hunt decision",
		time.Unix(1700000000, 0))

	require.Eventually(t, func() bool {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodGet, "/api/bots/test1/session-report", nil)
		server.httpServer.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			return false
		}
		body := response.Body.String()
		if !strings.Contains(body, "Kaboo Orc") {
			return false
		}
		require.Contains(t, body, "swarm session report")
		require.Contains(t, body, "bot: test1")
		require.Contains(t, body, "live now: level 5")
		require.Contains(t, body, "Hunt: the hunt decision")

		return true
	}, 5*time.Second, 20*time.Millisecond, "the report must render")
}
