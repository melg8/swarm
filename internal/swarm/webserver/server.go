// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package webserver serves the embedded web interface of the swarm bot: static
// files, JSON snapshot endpoints, a server sent event stream and the botless
// pathfind test mode (NewPathfindServer).
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

    "github.com/melg8/swarm/internal/swarm/acceptance"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
)

//go:embed web
var webContent embed.FS

// Server modes reported by GET /api/config.
const (
    modeBot      = "bot"
    modePathfind = "pathfind"
    // modeTestFight is the fight FX gallery of -test-fight-ui.
    modeTestFight = "test-fight"
    // modeTestPath is the walk plan gallery of -test-path-ui.
    modeTestPath = "test-path"
    // modeTestPathV2 is the smooth curve lab of -test-path-ui-v2 (thirty
    // animated variants).
    modeTestPathV2 = "test-path-v2"
    // modeTestPathV3 is the flow and snake speed lab of -test-path-ui-v3
    // (forty rainbow variants).
    modeTestPathV3 = "test-path-v3"
    // modeFight is the combat variant showcase of -test-fight-ui-v1.
    modeFight = "fight"
    // modeNavmesh is the 3D mesh viewer of -show-navmesh.
    modeNavmesh = "navmesh"

    // defaultPathfindScale is the initial map zoom of the pathfind
    // test, a bit closer than the bot map default.
    defaultPathfindScale = 0.06

    // fleetKillMarkLimit bounds the fleet kill ring of /api/fleet/kills: the
    // marks are session death statistics (nothing expires by time), so the
    // oldest drop only when the fleet outgrows the cap.
    fleetKillMarkLimit = 2000
)

// Poll intervals of the event stream.
const (
    eventPollPeriod = 300 * time.Millisecond
    eventPingPeriod = 15 * time.Second
)

// ssePingComment is the constant keepalive comment of the stream.
var ssePingComment = []byte(": ping\n\n")

// sseStream writes the SSE events of one connection. The reusable frame and
// payload buffers keep the steady state allocation free: both grow to the first
// (largest) snapshot and serve every later one (a fresh 64 KB frame and payload
// per version change would be megabytes of garbage per second).
type sseStream struct {
    frame   []byte
    payload []byte
}

// snapshotEvent encodes the bot state into the payload buffer, assembles the
// frame over it and writes both out. The encode allocates nothing: the view
// structs stay on the stack (see Bot.AppendSnapshotJSON).
func (s *sseStream) snapshotEvent(
    w http.ResponseWriter, flusher http.Flusher, bot *state.Bot,
) {
    s.payload = bot.AppendSnapshotJSON(s.payload[:0])
    s.frame = appendEventFrame(s.frame[:0], s.payload)
    armSSEWrite(w)
    if _, err := w.Write(s.frame); err != nil {
        return
    }
    flusher.Flush()
}

// sseWriteTimeout bounds one SSE frame write: a half-open client otherwise
// parks the handler in a blocked Write forever, and eventsDone cannot interrupt
// it. The deadline refreshes per frame, so a live slow reader keeps streaming.
const sseWriteTimeout = 15 * time.Second

// armSSEWrite sets the per frame write deadline of the SSE streams.
// Writers without deadline support (the wrapped recorders of the
// tests) keep the unbounded behavior.
func armSSEWrite(w http.ResponseWriter) {
    _ = http.NewResponseController(w).SetWriteDeadline(
        time.Now().Add(sseWriteTimeout))
}

// appendEventFrame assembles the SSE event frame for a payload into dst: the
// event header, the data bytes and the blank line that terminates the frame.
func appendEventFrame(dst, data []byte) []byte {
    dst = append(dst, "event: snapshot\ndata: "...)
    dst = append(dst, data...)

    return append(dst, '\n', '\n')
}

// httpReadHeaderTimeout bounds the header read of the web server.
const httpReadHeaderTimeout = 5 * time.Second

// Server serves the web interface for a bot registry; in pathfind test mode the
// registry is empty and the pathfinder answers the map requests.
type Server struct {
    registry     *state.Registry
    pathfinder   *pathfind.Engine
    pathfindView *pathfind.Vec3
    geodataTiles *geodataTileCache
    iconsDir     atomic.Value // string, the icon pack directory or ""
    proxy        ProxyController
    acceptance   *acceptance.Manager
    journal      *session.Journal
    logger       *log.Logger
    httpServer   *http.Server
    eventsDone   chan struct{}
    shutdown     func()
    // stats is the bot statistics collector (nil in the botless modes).
    stats *statsCollector
    // navmeshMesh is the tile mesh of the -show-navmesh run.
    navmeshMesh *navmesh.Mesh
    // navmeshTiles are the flag selected tiles (nil or empty means every tile,
    // the stitched world).
    navmeshTiles []navmesh.RegionKey
    // navmeshEngine is the geodata engine (nil in the tests): the original
    // geometry source and the clearance radius holder.
    navmeshEngine *pathfind.Engine
    // navmeshCapsule answers the wall clearance of the route answers.
    navmeshCapsule *pathfind.Capsule
    // navmeshGeo caches the encoded geometry; navmeshGeoMu guards the fills.
    navmeshGeo   map[navmesh.RegionKey][]byte
    navmeshGeoMu sync.Mutex
    // navmeshCompare is the second mesh of the dual pack view (nil for the
    // single pack viewer); navmeshCompareGeo caches its geometry.
    navmeshCompare    *navmesh.Mesh
    navmeshCompareGeo map[navmesh.RegionKey][]byte
    // navmeshOriginal caches the raw l2j cell render of the comparison toggle;
    // navmeshRegions loads one geodata region on demand.
    navmeshOriginal map[navmesh.RegionKey][]byte
    navmeshRegions  func(navmesh.RegionKey) (*pathfind.Region, error)
    // webBuild is the content hash of the embedded UI and indexPage the
    // index.html with the build meta injected: the page and the /api/bots poll
    // carry the id, so a stale tab reloads itself when the UI moved on.
    webBuild  string
    indexPage []byte
}

// ProxyController drives the client proxy from the web UI: which bot a
// connecting client attaches to. Without a proxy the endpoints stay absent and
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
    mux.HandleFunc("GET /api/fleet/kills", server.handleFleetKills)
    mux.HandleFunc("GET /api/bots/{id}/state", server.handleBotState)
    mux.HandleFunc("GET /api/bots/{id}/dump", server.handleBotDump)
    mux.HandleFunc("GET /api/bots/{id}/events", server.handleBotEvents)
    mux.HandleFunc("POST /api/bots/{id}/commands", server.handleBotCommand)
    mux.HandleFunc("GET /api/config", server.handleBotConfig)
    mux.HandleFunc("GET /api/stats", server.handleStats)
    mux.HandleFunc("GET /api/stats/{id}", server.handleBotStats)

    // The sampler starts with the server, so the history collects from the
    // first minute of the process, not from the first visit of the tab.
    server.stats = newStatsCollector(registry, logger)
    go server.stats.run()

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
    // The build identity and the index page with the meta injected: both are
    // constants of the binary.
    build := webBuildID(webContent)
    var indexPage []byte
    raw, readErr := fs.ReadFile(webContent, "web/index.html")
    if readErr == nil {
        indexPage = []byte(injectBuildMeta(string(raw), build))
    }
    server := &Server{
        registry:          nil,
        pathfinder:        nil,
        pathfindView:      nil,
        geodataTiles:      newGeodataTileCache(),
        iconsDir:          atomic.Value{},
        proxy:             nil,
        acceptance:        nil,
        journal:           nil,
        logger:            logger,
        httpServer:        nil,
        eventsDone:        make(chan struct{}),
        shutdown:          nil,
        stats:             nil,
        navmeshMesh:       nil,
        navmeshTiles:      nil,
        navmeshEngine:     nil,
        navmeshCapsule:    nil,
        navmeshGeo:        nil,
        navmeshGeoMu:      sync.Mutex{},
        navmeshCompare:    nil,
        navmeshCompareGeo: nil,
        navmeshOriginal:   nil,
        navmeshRegions:    nil,
        webBuild:          build,
        indexPage:         indexPage,
    }
    // The geometry caches start ready: the dual view endpoints share the
    // mutex and the cached contract.
    server.navmeshGeo = make(map[navmesh.RegionKey][]byte)
    server.navmeshCompareGeo = make(map[navmesh.RegionKey][]byte)
    //nolint:exhaustruct_v5 // the zero defaults of http.Server are intended
    server.httpServer = &http.Server{
        Addr:              address,
        Handler:           mux,
        ReadHeaderTimeout: httpReadHeaderTimeout,
    }
    server.shutdown = sync.OnceFunc(func() { close(server.eventsDone) })
    server.initIconsDir(logger)

    mux.HandleFunc("GET /icons/{name}", server.serveIcons)
    mux.HandleFunc("GET /api/hunt-mesh", server.handleHuntMesh)

    staticFS, err := fs.Sub(webContent, "web")
    if err != nil {
        logger.Printf("Error web content unavailable: %v", err)
    }
    mux.Handle("GET /", server.staticHandler(http.FileServerFS(staticFS)))

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
    if s.stats != nil {
        s.stats.stop()
    }

    return s.httpServer.Shutdown(ctx)
}

// handleBotList responds with the compact info of all bots and carries the
// build id (X-Swarm-Build): the web app reloads the page when the UI build
// moved on.
func (s *Server) handleBotList(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("X-Swarm-Build", s.webBuild)
    writeJSON(w, s.logger, s.registry.List())
}

// handleFleetKills responds with the recent kill marks of every bot: the web
// map polls it for the fleet cross layer, so the kills stay across the bot
// switches. An empty registry answers an empty array.
func (s *Server) handleFleetKills(w http.ResponseWriter, _ *http.Request) {
    marks := s.registry.FleetKillMarks(fleetKillMarkLimit)
    if marks == nil {
        // An idle fleet serves an empty array: the map layer feeds it directly.
        marks = []state.KillMarkView{}
    }
    writeJSON(w, s.logger, marks)
}

// handleBotState responds with the full snapshot of one bot through the direct
// append writer: json.Marshal costs 6x the direct write.
func (s *Server) handleBotState(w http.ResponseWriter, r *http.Request) {
    bot, ok := s.lookupBot(w, r)
    if !ok {
        return
    }
    w.Header().Set("Content-Type", "application/json")
    data := bot.AppendSnapshotJSON(nil)
    data = append(data, '\n')
    // The direct writer HTML escapes like json.Marshal, and the JSON content
    // type never executes in a browser.
    //nolint:gosec // escaped json, see above
    if _, err := w.Write(data); err != nil {
        s.logger.Printf("Error writing json response: %v", err)
    }
}

// handleBotEvents streams the snapshot events of one bot over SSE.
func (s *Server) handleBotEvents(w http.ResponseWriter, r *http.Request) {
    bot, ok := s.lookupBot(w, r)
    if !ok {
        return
    }

    s.streamEvents(r.Context(), w, bot)
}

// streamEvents writes snapshot events until the request or the server ends.
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
    armSSEWrite(w)
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
