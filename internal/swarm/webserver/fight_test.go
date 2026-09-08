// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// newFightTestServer builds the fight showcase server with a test
// logger.
func newFightTestServer(t *testing.T) *Server {
	t.Helper()

	return NewFightServer("127.0.0.1:0", log.New(io.Discard, "", 0))
}

// TestFightServerConfig pins the mode handshake of the fight showcase:
// the page boots its variant grid only when /api/config reports the
// fight mode, exactly the way the pathfind test detects its own mode.
func TestFightServerConfig(t *testing.T) {
	server := newFightTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/config", nil))

	require.Equal(t, http.StatusOK, recorder.Code)

	var config configResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
	require.Equal(t, modeFight, config.Mode)
}

// TestFightServerServesTheShowcasePage checks the static shell: the
// index page and the fight script must answer so the browser can
// boot the showcase after the mode handshake.
func TestFightServerServesTheShowcasePage(t *testing.T) {
	server := newFightTestServer(t)
	handler := server.httpServer.Handler

	t.Run("index page", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), "fight-strip")
	})

	t.Run("fight script", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/fight.js", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		body := recorder.Body.String()
		require.Contains(t, body, "FightUI")
		require.Contains(t, body, "VARIANTS")
	})

	t.Run("map tile for the background", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, "/maps/2/21_19.jpg", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "image/jpeg", recorder.Header().Get("Content-Type"))
	})
}
