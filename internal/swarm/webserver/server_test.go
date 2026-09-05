// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// syncRecorder is a thread safe response writer: the event stream test
// reads the body while the stream goroutine keeps writing.
type syncRecorder struct {
	mu   sync.Mutex
	code int
	buf  bytes.Buffer
	hdr  http.Header
}

func newSyncRecorder() *syncRecorder {
	//nolint:exhaustruct // the zero values are the point
	return &syncRecorder{hdr: http.Header{}}
}

// Header implements http.ResponseWriter.
func (r *syncRecorder) Header() http.Header { return r.hdr }

// Write implements http.ResponseWriter.
func (r *syncRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.buf.Write(p) //nolint:wrapcheck // test helper
}

// WriteHeader implements http.ResponseWriter.
func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = code
}

// Flush implements http.Flusher.
func (r *syncRecorder) Flush() {}

// String returns the accumulated body under the lock.
func (r *syncRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.buf.String()
}

// newTestServer builds a server with one bot in a known state.
func newTestServer(t *testing.T) (*Server, *state.Bot) {
	t.Helper()

	registry := state.NewRegistry()
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.SetOnline("test1")
	//nolint:exhaustruct // spawn fields only
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, X: 45100, Y: 50100, Name: "Keltir", Attackable: true,
	})
	registry.Add(bot)

	return NewServer(registry, "127.0.0.1:0", log.New(io.Discard, "", 0)), bot
}

func TestBotListEndpoint(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t,
		"application/json", recorder.Header().Get("Content-Type"))

	var bots []state.BotInfo
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &bots))
	require.Len(t, bots, 1)
	require.Equal(t, "test1", bots[0].ID)
	require.Equal(t, state.StatusOnline, bots[0].Status)
}

func TestBotStateEndpoint(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/state", nil))

	require.Equal(t, http.StatusOK, recorder.Code)

	var snapshot state.Snapshot
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &snapshot))
	require.Equal(t, "test1", snapshot.ID)
	require.Equal(t, state.StatusOnline, snapshot.Status)
	require.Equal(t, "test1", snapshot.Character.Name)
	require.Equal(t, int32(45000), snapshot.Character.X)
	require.Len(t, snapshot.Objects, 1)
	require.Equal(t, "Keltir", snapshot.Objects[0].Name)
}

func TestBotStateUnknownBot(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/nobody/state", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestStaticIndexServed(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, "SWARM")
	require.Contains(t, body, "map-canvas")
}

func TestStaticAssetServed(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/app.js", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "refreshBots")
}

func TestEventsStreamDeliversSnapshots(t *testing.T) {
	server, bot := newTestServer(t)

	recorder := newSyncRecorder()
	request := httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/events", nil)
	done := make(chan struct{})

	go func() {
		defer close(done)
		server.httpServer.Handler.ServeHTTP(recorder, request)
	}()
	t.Cleanup(func() {
		server.shutdown()
		<-done
	})

	// Wait for the initial snapshot, then bump the state and read again.
	require.Eventually(t, func() bool {
		return recorder.String() != ""
	}, 2*time.Second, 20*time.Millisecond)

	//nolint:exhaustruct // spawn fields only
	bot.ApplyNpcInfo(state.NpcInfo{ObjectID: 8, Name: "Orc"})

	require.Eventually(t, func() bool {
		return len(recorder.String()) > 100
	}, 2*time.Second, 50*time.Millisecond)

	body := recorder.String()
	require.Contains(t, body, "event: snapshot")
	require.Contains(t, body, "data: ")
	require.Contains(t, body, `"id":"test1"`)
}

func TestEventsStreamUnknownBot(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/nobody/events", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestListenAndServeOnFreePort(t *testing.T) {
	server, _ := newTestServer(t)
	server.httpServer.Addr = "127.0.0.1:0"

	listener, err := listenFree()
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	server.httpServer.Addr = address
	go func() {
		_ = server.ListenAndServe()
	}()

	require.Eventually(t, func() bool {
		//nolint:noctx // test request
		response, err := http.Get("http://" + address + "/api/bots")
		if err != nil {
			return false
		}
		defer response.Body.Close()

		return response.StatusCode == http.StatusOK
	}, 2*time.Second, 50*time.Millisecond)

	ctx, cancel := contextWithTimeout()
	defer cancel()
	require.NoError(t, server.Shutdown(ctx))
}

// listenFree opens a loopback listener to reserve a free port.
func listenFree() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

// contextWithTimeout builds a short shutdown context.
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}
