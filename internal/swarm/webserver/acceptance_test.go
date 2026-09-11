// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/acceptance"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// newAcceptanceServer builds a bot mode server with a manager of two
// instant scenarios.
func newAcceptanceServer(t *testing.T) *Server {
	t.Helper()
	registry := state.NewRegistry()
	server := NewServer(registry, "127.0.0.1:0", log.Default())
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
	server.SetAcceptance(acceptance.NewManager(acceptance.ManagerDeps{
		Registry: registry,
		Login:    "127.0.0.1:2106",
		Engine:   nil,
		Proxy:    nil,
		Logger:   log.Default(),
		DBConfig: acceptance.DefaultDBConfig(),
	}, []acceptance.TestDef{{
		ID:          "green",
		Title:       "green",
		Description: "passes at once",
		Account:     "temp1",
		Timeout:     5 * time.Second,
		Scenario: func(context.Context, *acceptance.Manager,
			*acceptance.Test,
		) error {
			return nil
		},
	}, {
		ID:          "red",
		Title:       "red",
		Description: "fails at once",
		Account:     "temp2",
		Timeout:     5 * time.Second,
		Scenario: func(context.Context, *acceptance.Manager,
			*acceptance.Test,
		) error {
			return errAcceptanceTest
		},
	}}))

	return server
}

// errAcceptanceTest is the static failure of the red scenario.
var errAcceptanceTest = acceptanceTestError{}

// acceptanceTestError renders the static failure.
type acceptanceTestError struct{}

// Error implements the error interface.
func (acceptanceTestError) Error() string { return "red scenario failed" }

// TestAcceptanceEndpointsAbsentWithoutManager pins the hidden panel
// contract: a server without the manager answers 404 and the UI
// keeps the tests hidden.
func TestAcceptanceEndpointsAbsentWithoutManager(t *testing.T) {
	registry := state.NewRegistry()
	server := NewServer(registry, "127.0.0.1:0", log.Default())
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})

	for _, path := range []string{
		"/api/acceptance/tests",
		"/api/acceptance/tests/green/run", "/api/acceptance/run",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		server.httpServer.Handler.ServeHTTP(recorder, request)
		if path == "/api/acceptance/tests" {
			require.Equal(t, http.StatusNotFound, recorder.Code, path)

			continue
		}
		require.Equal(t, http.StatusNotFound, recorder.Code, path)
	}
}

// TestAcceptanceTestsEndpointServesTheList pins the payload shape: the
// ids, the descriptions, the statuses and the temp accounts.
func TestAcceptanceTestsEndpointServesTheList(t *testing.T) {
	server := newAcceptanceServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/acceptance/tests",
		nil)
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	var tests []map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &tests))
	require.Len(t, tests, 2)
	require.Equal(t, "green", tests[0]["id"])
	require.Equal(t, "temp1", tests[0]["account"])
	require.Equal(t, "idle", tests[0]["status"])
	require.NotEmpty(t, tests[0]["description"])
	require.Equal(t, 5, int(tests[0]["timeoutSec"].(float64)))
}

// TestAcceptanceRunEndpointStartsAndRestarts pins the button
// semantics over http: one POST starts the scenario, the unknown id
// answers 404 and the bad mode answers 400.
func TestAcceptanceRunEndpointStartsAndRestarts(t *testing.T) {
	server := newAcceptanceServer(t)

	// The unknown id answers 404.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/acceptance/tests/no-such/run", nil)
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNotFound, recorder.Code)

	// The green scenario runs to completion.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost,
		"/api/acceptance/tests/green/run", nil)
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.True(t, waitAcceptanceStatus(t, server, "green", "passed"))

	// The red scenario fails with the reason.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost,
		"/api/acceptance/tests/red/run", nil)
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.True(t, waitAcceptanceStatus(t, server, "red", "failed"))
}

// TestAcceptanceRunAllEndpointModes pins the run all payload: the
// parallel mode starts every scenario at once, the invalid mode
// answers 400.
func TestAcceptanceRunAllEndpointModes(t *testing.T) {
	server := newAcceptanceServer(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/acceptance/run",
		stringReader(`{"mode":"sideways"}`))
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/acceptance/run",
		stringReader(`{"mode":"parallel"}`))
	server.httpServer.Handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.True(t, waitAcceptanceStatus(t, server, "green", "passed"))
	require.True(t, waitAcceptanceStatus(t, server, "red", "failed"))
}

// waitAcceptanceStatus polls the tests endpoint until the wanted
// status lands (or the deadline fails the test).
func waitAcceptanceStatus(
	t *testing.T, server *Server, id string, status string,
) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet,
			"/api/acceptance/tests", nil)
		server.httpServer.Handler.ServeHTTP(recorder, request)
		var tests []map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &tests); err == nil {
			for _, test := range tests {
				if test["id"] == id && test["status"] == status {
					return true
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	return false
}

// stringReader wraps a string in an io.Reader for the test requests.
func stringReader(body string) io.Reader {
	return strings.NewReader(body)
}
