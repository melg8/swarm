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
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
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
	// modeFight is the combat animation variant showcase of the
	// -test-fight-ui-v1 run: a looping demo fight on every variant.
	modeFight = "fight"

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
	iconsDir     atomic.Value // string, the icon pack directory or ""
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
	mux.HandleFunc("POST /api/bots/{id}/commands", server.handleBotCommand)
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
		iconsDir:     atomic.Value{},
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
	server.initIconsDir(logger)

	mux.HandleFunc("GET /icons/{name}", server.serveIcons)

	staticFS, err := fs.Sub(webContent, "web")
	if err != nil {
		logger.Printf("Error web content unavailable: %v", err)
	}
	mux.Handle("GET /", http.FileServerFS(staticFS))

	return server
}

// handleBotConfig reports the bot mode of the web UI.
func (s *Server) handleBotConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.logger, configResponse{
		Mode:     modeBot,
		Geodata:  nil,
		MaxSteps: 0,
		Defaults: nil,
	})
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

// handleBotState responds with the full snapshot of one bot. The
// snapshot encodes through the direct append writer - the reflection
// and compacting walk of json.Marshal costs 6x the direct write.
func (s *Server) handleBotState(w http.ResponseWriter, r *http.Request) {
	bot, ok := s.lookupBot(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	data := bot.Snapshot().AppendJSON(nil)
	data = append(data, '\n')
	// The direct writer HTML escapes the payload exactly like
	// json.Marshal, and the JSON content type never executes in
	// a browser - the taint report is a false positive.
	//nolint:gosec // escaped json, see above
	if _, err := w.Write(data); err != nil {
		s.logger.Printf("Error writing json response: %v", err)
	}
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

	writeEvent(w, flusher, encodeSnapshotJSON(bot.Snapshot()))
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
			writeSnapshotEvent(w, flusher, bot, &lastVersion)
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

// writeSnapshotEvent streams the bot state when its version changed.
func writeSnapshotEvent(
	w http.ResponseWriter, flusher http.Flusher,
	bot *state.Bot, lastVersion *uint64,
) {
	version := bot.Version()
	if version == *lastVersion {
		return
	}
	writeEvent(w, flusher, encodeSnapshotJSON(bot.Snapshot()))
	*lastVersion = version
}

// encodeSnapshotJSON marshals the snapshot through the direct append
// writer of the state package (see Snapshot.AppendJSON): the event
// stream serializes on every version change, so the reflection walk
// of json.Marshal is the wrong tool here.
func encodeSnapshotJSON(snap state.Snapshot) []byte {
	return snap.AppendJSON(nil)
}

// writeEvent writes one SSE event and flushes it. The frame and the
// payload share one buffer allocation sized up front.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, data []byte) {
	event := make([]byte, 0, len(data)+24)
	event = append(event, "event: snapshot\ndata: "...)
	event = append(event, data...)
	event = append(event, '\n', '\n')
	// The payload is the HTML escaped JSON document of the
	// snapshot (see Snapshot.AppendJSON) and the SSE stream is
	// never executed by a browser - the taint report is a false
	// positive.
	if _, err := w.Write(event); err != nil { //nolint:gosec // escaped
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
