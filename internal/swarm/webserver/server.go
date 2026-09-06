// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package webserver serves the embedded web interface of the swarm bot:
// static files, JSON snapshot endpoints and a server sent event stream
// of the observed bot states. It also hosts the bot less pathfind test
// mode (NewPathfindServer) that exposes the map path search of the
// pathfind package.
package webserver

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
)

//go:embed web
var webContent embed.FS

// Server modes reported by GET /api/config.
const (
	modeBot      = "bot"
	modePathfind = "pathfind"

	// defaultPathfindScale is the initial map zoom of the pathfind
	// test, a bit closer than the bot map default.
	defaultPathfindScale = 0.06
)

// Poll intervals of the event stream.
const (
	eventPollPeriod = 300 * time.Millisecond
	eventPingPeriod = 15 * time.Second
)

// httpReadHeaderTimeout bounds the header read of the web server.
const httpReadHeaderTimeout = 5 * time.Second

// Server serves the web interface for a bot registry. In pathfind test
// mode the registry is empty and the pathfinder answers the map
// requests instead.
type Server struct {
	registry     *state.Registry
	pathfinder   *pathfind.Engine
	pathfindView *pathfind.Vec3
	geodataTiles *geodataTileCache
	logger       *log.Logger
	httpServer   *http.Server
	eventsDone   chan struct{}
	shutdown     func()
}

// NewServer creates the web server bound to the given address.
func NewServer(
	registry *state.Registry, address string, logger *log.Logger,
) *Server {
	server := newServer(address, logger)
	server.registry = registry

	mux := server.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/bots", server.handleBotList)
	mux.HandleFunc("GET /api/bots/{id}/state", server.handleBotState)
	mux.HandleFunc("GET /api/bots/{id}/events", server.handleBotEvents)
	mux.HandleFunc("GET /api/config", server.handleBotConfig)

	return server
}

// newServer builds the shared server shell with the static files.
func newServer(address string, logger *log.Logger) *Server {
	mux := http.NewServeMux()
	server := &Server{
		registry:     nil,
		pathfinder:   nil,
		pathfindView: nil,
		geodataTiles: newGeodataTileCache(),
		logger:       logger,
		httpServer:   nil,
		eventsDone:   make(chan struct{}),
		shutdown:     nil,
	}
	//nolint:exhaustruct // the zero defaults of http.Server are intended
	server.httpServer = &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}
	server.shutdown = sync.OnceFunc(func() { close(server.eventsDone) })

	staticFS, err := fs.Sub(webContent, "web")
	if err != nil {
		logger.Printf("Error web content unavailable: %v", err)
	}
	mux.Handle("GET /", http.FileServerFS(staticFS))

	return server
}

// handleBotConfig reports the bot mode of the web UI.
func (s *Server) handleBotConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, configResponse{Mode: modeBot})
}

// Address returns the address the server listens on.
func (s *Server) Address() string {
	return s.httpServer.Addr
}

// ListenAndServe runs the web server until shutdown.
func (s *Server) ListenAndServe() error {
	s.logger.Println("Web interface listening on http://" + s.httpServer.Addr)

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the web server. Active event streams are
// cancelled first so the shutdown does not wait for them.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdown()

	return s.httpServer.Shutdown(ctx)
}

// handleBotList responds with the compact info of all bots.
func (s *Server) handleBotList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, s.registry.List())
}

// handleBotState responds with the full snapshot of one bot.
func (s *Server) handleBotState(w http.ResponseWriter, r *http.Request) {
	bot, ok := s.lookupBot(w, r)
	if !ok {
		return
	}
	writeJSON(w, s.logger, bot.Snapshot())
}

// handleBotEvents streams snapshot events of one bot over SSE whenever the
// bot state version changes.
func (s *Server) handleBotEvents(w http.ResponseWriter, r *http.Request) {
	bot, ok := s.lookupBot(w, r)
	if !ok {
		return
	}

	s.streamEvents(r.Context(), w, bot)
}

// streamEvents writes snapshot events until the request or the server
// ends.
func (s *Server) streamEvents(
	ctx context.Context, w http.ResponseWriter, bot *state.Bot,
) {
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	poll := time.NewTicker(eventPollPeriod)
	defer poll.Stop()
	ping := time.NewTicker(eventPingPeriod)
	defer ping.Stop()

	if snapshot, err := json.Marshal(bot.Snapshot()); err == nil {
		writeEvent(w, flusher, snapshot)
	}
	lastVersion := bot.Version()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.eventsDone:
			return
		case <-ping.C:
			if s.writePing(w, flusher) {
				return
			}
		case <-poll.C:
			if writeSnapshotEvent(w, flusher, bot, &lastVersion) {
				return
			}
		}
	}
}

// writePing sends the keepalive comment. It reports a write failure.
func (s *Server) writePing(w http.ResponseWriter, flusher http.Flusher) bool {
	if _, err := w.Write([]byte(": ping\n\n")); err != nil {
		return true
	}
	flusher.Flush()

	return false
}

// writeSnapshotEvent streams the bot state when its version changed. It
// reports whether the stream must stop.
func writeSnapshotEvent(
	w http.ResponseWriter, flusher http.Flusher,
	bot *state.Bot, lastVersion *uint64,
) bool {
	version := bot.Version()
	if version == *lastVersion {
		return false
	}
	snapshot, err := json.Marshal(bot.Snapshot())
	if err != nil {
		return true
	}
	writeEvent(w, flusher, snapshot)
	*lastVersion = version

	return false
}

// writeEvent writes one SSE event and flushes it.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, data []byte) {
	var buf bytes.Buffer
	buf.WriteString("event: snapshot\ndata: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	if _, err := w.Write(buf.Bytes()); err != nil {
		return
	}
	flusher.Flush()
}

// lookupBot resolves the bot id of the request path.
func (s *Server) lookupBot(
	w http.ResponseWriter, r *http.Request,
) (*state.Bot, bool) {
	bot, ok := s.registry.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "bot not found", http.StatusNotFound)

		return nil, false
	}

	return bot, true
}

// writeJSON responds with a JSON document.
func writeJSON(w http.ResponseWriter, logger *log.Logger, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		logger.Printf("Error encoding json response: %v", err)
	}
}
