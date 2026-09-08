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
	"strings"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// fakeProxyController drives the proxy endpoints without the proxy
// package (the webserver only sees the narrow interface).
type fakeProxyController struct {
	selected string
	sessions []string
	clients  int
}

func (f *fakeProxyController) SelectBot(id string) { f.selected = id }

func (f *fakeProxyController) SelectedBot() string { return f.selected }

func (f *fakeProxyController) SessionIDs() []string { return f.sessions }

func (f *fakeProxyController) ClientCount() int { return f.clients }

func (f *fakeProxyController) LoginAddr() string { return "127.0.0.1:2107" }

func (f *fakeProxyController) GameAddr() string { return "127.0.0.1:7778" }

func TestProxyEndpointsReportAndSelect(t *testing.T) {
	controller := &fakeProxyController{selected: "", sessions: []string{"bot1", "bot2"}, clients: 1}
	server := NewServer(state.NewRegistry(), "127.0.0.1:0", log.New(io.Discard, "", 0))
	server.SetProxy(controller)

	// The status endpoint reports the proxy state.
	status := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(status, httptest.NewRequest(
		http.MethodGet, "/api/proxy", nil))
	require.Equal(t, http.StatusOK, status.Code)
	var payload proxyStatus
	require.NoError(t, json.Unmarshal(status.Body.Bytes(), &payload))
	require.True(t, payload.Enabled)
	require.Empty(t, payload.SelectedBot)
	require.Equal(t, []string{"bot1", "bot2"}, payload.Sessions)
	require.Equal(t, 1, payload.Clients)
	require.Equal(t, "127.0.0.1:2107", payload.Login)
	require.Equal(t, "127.0.0.1:7778", payload.Game)

	// The select endpoint switches the proxy target.
	selectBody := strings.NewReader(`{"botId":"bot2"}`)
	selected := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(selected, httptest.NewRequest(
		http.MethodPost, "/api/proxy/select", selectBody))
	require.Equal(t, http.StatusOK, selected.Code)
	require.Equal(t, "bot2", controller.selected)
	require.NoError(t, json.Unmarshal(selected.Body.Bytes(), &payload))
	require.Equal(t, "bot2", payload.SelectedBot)

	// An empty bot id is rejected.
	bad := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(bad, httptest.NewRequest(
		http.MethodPost, "/api/proxy/select", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusBadRequest, bad.Code)
}

func TestProxyEndpointsAbsentWithoutProxy(t *testing.T) {
	server := NewServer(state.NewRegistry(), "127.0.0.1:0", log.New(io.Discard, "", 0))

	status := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(status, httptest.NewRequest(
		http.MethodGet, "/api/proxy", nil))
	require.Equal(t, http.StatusNotFound, status.Code)
}
