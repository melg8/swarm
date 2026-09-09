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
	// modeTestFight is the fight FX comparison gallery mode
	// (NewTestFightServer): the static cells of the -test-fight-ui flag.
	modeTestFight = "test-fight"
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

// ssePingComment is the constant keepalive comment of the stream.
var ssePingComment = []byte(": ping\n\n")

// sseStream writes the SSE events of one open connection. It owns
// the reusable frame and payload buffers, so the steady state of a
// long lived stream costs zero allocations per event: the frame
// assembly and the JSON encoding both write into buffers sized by
// the first (largest) snapshot and reused by every later one (a
// watched 100 npc bot used to pay a fresh 64 KB frame plus a fresh
// payload buffer on every version change - with a fleet of
// streams that is megabytes of garbage per second).
type sseStream struct {
	frame   []byte
	payload []byte
}

// snapshotEvent encodes the current bot state into the payload
// buffer, assembles the SSE frame over it and writes both out. The
// encode walks the live state under the read lock and allocates
// nothing: the view structs of the elements stay on the call stack
// (see Bot.AppendSnapshotJSON).
func (s *sseStream) snapshotEvent(
	w http.ResponseWriter, flusher http.Flusher, bot *state.Bot,
) {
	s.payload = bot.AppendSnapshotJSON(s.payload[:0])
	s.frame = appendEventFrame(s.frame[:0], s.payload)
	if _, err := w.Write(s.frame); err != nil {
		return
	}
	flusher.Flush()
}

// appendEventFrame assembles the SSE event frame for a payload into
// dst: the event header, the data bytes and the blank line that
// terminates the frame.
func appendEventFrame(dst, data []byte) []byte {
	dst = append(dst, "event: snapshot\ndata: "...)
	dst = append(dst, data...)

	return append(dst, '\n', '\n')
}

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
	proxy        ProxyController
	logger       *log.Logger
	httpServer   *http.Server
	eventsDone   chan struct{}
	shutdown     func()
}

// ProxyController drives the client proxy from the web UI: which bot a
// connecting game client attaches to. The proxy server of the swarm
// process implements it; without a proxy the endpoints stay absent and
// the UI hides the selection.
type ProxyController interface {
	// SelectBot marks the bot the next connecting client attaches to.
	SelectBot(id string)
	// SelectedBot returns the marked bot id ("" when nothing selected).
	SelectedBot() string
	// SessionIDs lists the registered bot sessions in order.
	SessionIDs() []string
	// ClientCount returns the connected game client count.
	ClientCount() int
	// LoginAddr returns the primary login listen address.
	LoginAddr() string
	// GameAddr returns the primary game listen address.
	GameAddr() string
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
	mux.HandleFunc("GET /api/bots/{id}/dump", server.handleBotDump)
	mux.HandleFunc("GET /api/bots/{id}/events", server.handleBotEvents)
	mux.HandleFunc("POST /api/bots/{id}/commands", server.handleBotCommand)
	mux.HandleFunc("GET /api/config", server.handleBotConfig)

	return server
}

// SetProxy attaches the client proxy controller and registers its
// endpoints. Call it before ListenAndServe.
func (s *Server) SetProxy(controller ProxyController) {
	s.proxy = controller
	mux := s.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/proxy", s.handleProxyStatus)
	mux.HandleFunc("POST /api/proxy/select", s.handleProxySelect)
}

// proxyStatus is the payload of GET /api/proxy.
type proxyStatus struct {
	Enabled     bool     `json:"enabled"`
	SelectedBot string   `json:"selectedBot"`
	Sessions    []string `json:"sessions"`
	Clients     int      `json:"clients"`
	Login       string   `json:"login"`
	Game        string   `json:"game"`
}

// proxySelectRequest is the payload of POST /api/proxy/select.
type proxySelectRequest struct {
	BotID string `json:"botId"`
}

// handleProxyStatus reports the proxy state for the UI selection.
func (s *Server) handleProxyStatus(w http.ResponseWriter, _ *http.Request) {
	if s.proxy == nil {
		http.Error(w, "no proxy", http.StatusNotFound)

		return
	}
	writeJSON(w, s.logger, proxyStatus{
		Enabled:     true,
		SelectedBot: s.proxy.SelectedBot(),
		Sessions:    s.proxy.SessionIDs(),
		Clients:     s.proxy.ClientCount(),
		Login:       s.proxy.LoginAddr(),
		Game:        s.proxy.GameAddr(),
	})
}

// handleProxySelect switches the bot a connecting client attaches to.
func (s *Server) handleProxySelect(w http.ResponseWriter, r *http.Request) {
	if s.proxy == nil {
		http.Error(w, "no proxy", http.StatusNotFound)

		return
	}
	var request proxySelectRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}
	if request.BotID == "" {
		http.Error(w, "botId is required", http.StatusBadRequest)

		return
	}
	s.proxy.SelectBot(request.BotID)
	writeJSON(w, s.logger, proxyStatus{
		Enabled:     true,
		SelectedBot: request.BotID,
		Sessions:    s.proxy.SessionIDs(),
		Clients:     s.proxy.ClientCount(),
		Login:       s.proxy.LoginAddr(),
		Game:        s.proxy.GameAddr(),
	})
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
		proxy:        nil,
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
	s.logger.Println("Web interface listening on http://" +
		s.httpServer.Addr)

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
	data := bot.AppendSnapshotJSON(nil)
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
		http.Error(w, "streaming unsupported",
			http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	poll := time.NewTicker(eventPollPeriod)
	defer poll.Stop()
	ping := time.NewTicker(eventPingPeriod)
	defer ping.Stop()

	stream := &sseStream{frame: nil, payload: nil}
	stream.snapshotEvent(w, flusher, bot)
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
			writeSnapshotEvent(w, flusher, bot,
				&lastVersion, stream)
		}
	}
}

// writePing sends the keepalive comment. It reports a write failure.
func (s *Server) writePing(w http.ResponseWriter, flusher http.Flusher) bool {
	if _, err := w.Write(ssePingComment); err != nil {
		return true
	}
	flusher.Flush()

	return false
}

// writeSnapshotEvent streams the bot state when its version changed.
// The buffers of the stream are reused across the events.
func writeSnapshotEvent(
	w http.ResponseWriter, flusher http.Flusher,
	bot *state.Bot, lastVersion *uint64, stream *sseStream,
) {
	version := bot.Version()
	if version == *lastVersion {
		return
	}
	stream.snapshotEvent(w, flusher, bot)
	*lastVersion = version
}

// writeEvent writes one SSE event and flushes it. Test seam for
// the frame layout; the stream path reuses its buffers through
// sseStream.snapshotEvent instead.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, data []byte) {
	event := appendEventFrame(make([]byte, 0, len(data)+24), data)
	if _, err := w.Write(event); err != nil {
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
