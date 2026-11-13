// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "encoding/binary"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The navmesh viewer mode (the -show-navmesh flag of cmd/swarm) is the
// botless 3D inspection surface of docs/navmesh.md: the embedded three.js
// viewer renders the tiles, a double click pair asks the real corridor
// search and the answer carries the construction time. A bare
// -show-navmesh loads every tile stitched together (the links cross the
// region borders), -show-navmesh=21_19 opens the named tiles only, while
// the queries always run over the full directory mesh. The binary geometry
// contract (NMV2) lives in navmesh_geometry.go.

// navmeshPathBodyLimit caps the JSON request of a route query.
const navmeshPathBodyLimit = 4096

// NavmeshOptions tunes the navmesh viewer server.
type NavmeshOptions struct {
    // InitialTiles are the tile keys the viewer opens with; empty opens
    // every tile of the directory (the stitched world).
    InitialTiles []navmesh.RegionKey
    // Engine backs the original geometry and the capsule clearance. A nil
    // engine keeps the raw funnel pivots (the tests).
    Engine *pathfind.Engine
    // CompareMesh arms the dual pack view: the second pack renders side by
    // side with the primary one, with the per tile poly diff and one click
    // routing both meshes. Nil keeps the single pack viewer.
    CompareMesh *navmesh.Mesh
}

// NewNavmeshServer creates the botless viewer server with the real corridor
// search.
func NewNavmeshServer(
    mesh *navmesh.Mesh, address string, logger *log.Logger,
    options NavmeshOptions,
) *Server {
    server := newServer(address, logger)
    server.navmeshMesh = mesh
    server.navmeshTiles = options.InitialTiles
    server.navmeshEngine = options.Engine
    server.navmeshCompare = options.CompareMesh
    if options.Engine != nil && options.Engine.CapsuleRadius() > 0 {
        server.navmeshCapsule = pathfind.NewCapsule(options.Engine)
    }
    if options.Engine != nil && options.Engine.Dir() != "" {
        dir := options.Engine.Dir()
        server.navmeshRegions = func(key navmesh.RegionKey) (
            *pathfind.Region, error) {
            return loadGeodataRegion(dir, key)
        }
    }

    mux := server.httpServer.Handler.(*http.ServeMux)
    mux.HandleFunc("GET /api/config", server.handleNavmeshConfig)
    mux.HandleFunc("GET /api/navmesh/tiles", server.handleNavmeshTiles)
    mux.HandleFunc("GET /api/navmesh/geometry/{key}",
        server.handleNavmeshGeometry)
    mux.HandleFunc("GET /api/navmesh/original/{key}",
        server.handleNavmeshOriginal)
    mux.HandleFunc("POST /api/navmesh/path", server.handleNavmeshPath)
    mux.HandleFunc("GET /api/navmesh/compare/geometry/{key}",
        server.handleNavmeshCompareGeometry)
    mux.HandleFunc("GET /api/navmesh/compare/diff/{key}",
        server.handleNavmeshCompareDiff)

    return server
}

// navmeshConfigResponse is the boot payload; Compare is nil for the single
// pack viewer.
type navmeshConfigResponse struct {
    Mode    string         `json:"mode"`
    Navmesh navmeshConfig  `json:"navmesh"`
    Compare *navmeshConfig `json:"compare,omitempty"`
}

// navmeshConfig tells the viewer which tiles exist.
type navmeshConfig struct {
    Dir       string           `json:"dir"`
    TileFiles int              `json:"tileFiles"`
    Tiles     []navmeshTileKey `json:"tiles"`
    // InitialTiles lists the flag selected keys; nil means every tile.
    InitialTiles []navmeshTileKey `json:"initialTiles"`
}

// navmeshTileKey names one tile with its world footprint.
type navmeshTileKey struct {
    Col  int16   `json:"col"`
    Row  int16   `json:"row"`
    MinX float64 `json:"minX"`
    MinY float64 `json:"minY"`
    MaxX float64 `json:"maxX"`
    MaxY float64 `json:"maxY"`
}

// navmeshPoint is one world position: X and Y horizontal, Z the height.
type navmeshPoint struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z"`
}

// navmeshPathRequest is the body of POST /api/navmesh/path.
type navmeshPathRequest struct {
    Start navmeshPoint `json:"start"`
    End   navmeshPoint `json:"end"`
    // Approach is the approach radius (the plan repro contract): positive
    // succeeds on the first polygon within the radius of the end (the bot
    // trip plans with 200), zero keeps the exact destination.
    Approach float64 `json:"approach,omitempty"`
    // Avoid lists the ban circles the search seals.
    Avoid []navmeshAvoidCircle `json:"avoid,omitempty"`
    // Fold runs the capsule post pass and the fold into the longest grid
    // clear segments (the double click default). The plan repro links send
    // false, so the answer IS the published search plan; the grid oracle is
    // water blind and once drew a chord over the lake the mesh detours.
    Fold *bool `json:"fold,omitempty"`
}

// navmeshAvoidCircle is one ban circle: the center and the radius.
type navmeshAvoidCircle struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    R float64 `json:"r"`
}

// navmeshPathResponse is the corridor answer with the measured
// construction time (a cold query pays the lazy tile decode, roughly a
// millisecond per tile). Waypoints is the smoothed answer when the capsule
// pass arms, RawWaypoints the raw funnel answer for the comparison toggle.
type navmeshPathResponse struct {
    Found        bool           `json:"found"`
    Partial      bool           `json:"partial"`
    Error        string         `json:"error,omitempty"`
    Waypoints    []navmeshPoint `json:"waypoints"`
    RawWaypoints []navmeshPoint `json:"rawWaypoints,omitempty"`
    DurationMs   float64        `json:"durationMs"`
    Explored     int            `json:"explored"`
    Corridor     int            `json:"corridor"`
    // Compare carries the same query over the second pack of the dual view
    // (nil for the single pack viewer).
    Compare *navmeshPathResponse `json:"compare,omitempty"`
}

// handleNavmeshConfig answers the mode and the tile listing.
func (s *Server) handleNavmeshConfig(w http.ResponseWriter, _ *http.Request) {
    stats := s.navmeshMesh.Stats()
    tiles := meshTileKeys(s.navmeshMesh)
    initial := []navmeshTileKey(nil)
    if len(s.navmeshTiles) > 0 {
        initial = tileKeysOf(s.navmeshTiles)
    }
    answer := navmeshConfigResponse{
        Mode: modeNavmesh,
        Navmesh: navmeshConfig{
            Dir:          stats.Dir,
            TileFiles:    stats.TileFiles,
            Tiles:        tiles,
            InitialTiles: initial,
        },
        Compare: nil,
    }
    if s.navmeshCompare != nil {
        compareStats := s.navmeshCompare.Stats()
        answer.Compare = &navmeshConfig{
            Dir:          compareStats.Dir,
            TileFiles:    compareStats.TileFiles,
            Tiles:        meshTileKeys(s.navmeshCompare),
            InitialTiles: nil,
        }
    }
    writeJSON(w, s.logger, answer)
}

// handleNavmeshTiles lists the tile files with their world footprints; no
// tile is decoded (the geometry endpoint loads on demand).
func (s *Server) handleNavmeshTiles(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, s.logger, navmeshTilesResponse{
        Tiles: meshTileKeys(s.navmeshMesh),
    })
}

// navmeshTilesResponse is the payload of GET /api/navmesh/tiles.
type navmeshTilesResponse struct {
    Tiles []navmeshTileKey `json:"tiles"`
}

// handleNavmeshGeometry serves the binary polygon soup of one tile. The
// payload is immutable, so the ETag revalidates for free and the server
// caches the encoded bytes for the process lifetime.
func (s *Server) handleNavmeshGeometry(w http.ResponseWriter, r *http.Request) {
    key, ok := navmeshKeyOf(r.PathValue("key"))
    if !ok {
        http.Error(w, "bad tile key, expected col_row like 21_19",
            http.StatusBadRequest)

        return
    }
    payload, etag, err := s.navmeshGeometry(key)
    if err != nil {
        if errors.Is(err, navmesh.ErrTileAbsent) {
            http.Error(w, "no such navmesh tile",
                http.StatusNotFound)

            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)

        return
    }
    if r.Header.Get("If-None-Match") == etag {
        w.WriteHeader(http.StatusNotModified)

        return
    }
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("ETag", etag)
    w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
    if r.Method == http.MethodHead {
        w.WriteHeader(http.StatusOK)

        return
    }
    //nolint:gosec // the binary geometry of the local tile pack, an
    // application/octet-stream answer without any html context.
    _, _ = w.Write(payload)
}

// handleNavmeshOriginal serves the geometry of one region from the raw l2j
// cells (the comparison toggle): the NMV2 contract with an empty link block,
// cached like the mesh geometry.
func (s *Server) handleNavmeshOriginal(w http.ResponseWriter,
    r *http.Request,
) {
    key, ok := navmeshKeyOf(r.PathValue("key"))
    if !ok {
        http.Error(w, "bad tile key, expected col_row like 21_19",
            http.StatusBadRequest)

        return
    }
    payload, etag, err := s.navmeshOriginalGeometry(key)
    if err != nil {
        if errors.Is(err, errOriginalUnavailable) {
            http.Error(w, err.Error(), http.StatusNotImplemented)

            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)

        return
    }
    if r.Header.Get("If-None-Match") == etag {
        w.WriteHeader(http.StatusNotModified)

        return
    }
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("ETag", etag)
    w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
    if r.Method == http.MethodHead {
        w.WriteHeader(http.StatusOK)

        return
    }
    //nolint:gosec // the binary geometry of the local geodata, an
    // application/octet-stream answer without any html context.
    _, _ = w.Write(payload)
}

// handleNavmeshPath runs one corridor search between the double clicked
// points (or one plan repro search of a pathfind link) and measures the
// time.
func (s *Server) handleNavmeshPath(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(io.LimitReader(r.Body, navmeshPathBodyLimit))
    if err != nil {
        http.Error(w, "failed to read the request body", http.StatusBadRequest)

        return
    }
    var request navmeshPathRequest
    if err := json.Unmarshal(body, &request); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)

        return
    }
    response := s.navmeshSearchAnswer(s.navmeshMesh, &request, "")
    if s.navmeshCompare != nil {
        // The same click pair routes the second mesh under the identical
        // filter, so the viewer compares both routes by eye.
        compare := s.navmeshSearchAnswer(
            s.navmeshCompare, &request, "compare")
        response.Compare = &compare
    }
    writeJSON(w, s.logger, response)
}

// navmeshSearchAnswer runs one corridor search of the given mesh and measures
// the construction time. The filter contract is identical for every mesh of
// the view, so two packs over the same geodata differ by the decomposition
// only. The label distinguishes the console lines of the two packs.
func (s *Server) navmeshSearchAnswer(
    mesh *navmesh.Mesh, request *navmeshPathRequest, label string,
) navmeshPathResponse {
    filter, clearance := s.navmeshRouteFilter()

    start := navmesh.Pos{X: request.Start.X, Y: request.Start.Y,
        Z: request.Start.Z}
    end := navmesh.Pos{X: request.End.X, Y: request.End.Y,
        Z: request.End.Z}
    for _, circle := range request.Avoid {
        filter.Avoid = append(filter.Avoid, navmesh.AvoidCircle{
            CenterX: circle.X,
            CenterY: circle.Y,
            Radius:  circle.R,
        })
    }
    began := time.Now()
    var route *navmesh.Route
    var err error
    if request.Approach > 0 {
        route, err = mesh.RouteApproach(start, end,
            request.Approach, filter)
    } else {
        route, err = mesh.Route(start, end, filter)
    }
    duration := time.Since(began)
    s.logNavmeshRoute(label, route, err, duration)

    response := navmeshPathResponse{
        Found:        false,
        Partial:      false,
        Error:        "",
        Waypoints:    []navmeshPoint{},
        RawWaypoints: nil,
        DurationMs:   float64(duration.Nanoseconds()) / 1e6,
        Explored:     0,
        Corridor:     0,
        Compare:      nil,
    }
    if err != nil {
        response.Error = err.Error()

        return response
    }
    if route != nil {
        response.Found = route.Found
        response.Partial = route.Partial
        response.Explored = route.Explored
        response.Corridor = len(route.Corridor)
        // The smoothed answer walks the full pipeline; the plan repro answer
        // serves the search waypoints as the bot publishes them. The raw
        // funnel keeps the push and bend pass only (the before picture).
        fold := true
        if request.Fold != nil {
            fold = *request.Fold
        }
        waypoints := route.Waypoints
        if fold {
            waypoints = s.clearedWaypoints(route.Waypoints, clearance)
        }
        response.Waypoints = toNavmeshPoints(waypoints)
        response.RawWaypoints = toNavmeshPoints(
            s.legacyWaypoints(route.RawWaypoints, clearance))
    }

    return response
}

// navmeshRouteFilter builds the search filter: the priced search (water
// polygons stay walkable at the swim rate, so a crossing competes with the
// land detour on travel time), the capsule clearance of the viewer engine and
// the guard of the armed capsule pass.
func (s *Server) navmeshRouteFilter() (navmesh.Filter, float64) {
    filter := navmesh.DefaultFilter()
    filter.WaterZones = navmesh.C1WaterZones()
    clearance := 0.0
    if s.navmeshEngine != nil {
        clearance = s.navmeshEngine.CapsuleRadius()
    }
    filter.WaypointClearance = clearance
    filter.Smooth = clearance > 0
    if s.navmeshCapsule != nil {
        filter.Guard = s.navmeshCapsule
    }

    return filter, clearance
}

// logNavmeshRoute keeps the construction cost observable: a cold request
// pays the tile decode, a repeat answers from the resident mesh. The label
// distinguishes the two packs of the dual view.
func (s *Server) logNavmeshRoute(label string, route *navmesh.Route,
    err error, duration time.Duration,
) {
    logLabel := "Navmesh route"
    if label != "" {
        logLabel = "Navmesh route (" + label + ")"
    }
    if err != nil {
        s.logger.Printf("%s: failed in %.1f ms: %v", logLabel,
            float64(duration.Nanoseconds())/1e6, err)

        return
    }
    if route == nil {
        s.logger.Printf("%s: no path in %.1f ms", logLabel,
            float64(duration.Nanoseconds())/1e6)

        return
    }
    status := "no path"
    switch {
    case route.Found:
        status = "found"
    case route.Partial:
        status = "partial"
    }
    s.logger.Printf("%s: %s in %.1f ms - "+
        "%d waypoints, %d regions", logLabel, status,
        float64(duration.Nanoseconds())/1e6,
        len(route.Waypoints), len(route.Corridor))
}

// clearedWaypoints runs the answer through the capsule clearance post pass
// (the funnel pivots and the smoothing cover the turns and the merges, the
// post pass whatever still grazes a wall) and folds it into the longest grid
// clear segments.
func (s *Server) clearedWaypoints(waypoints []navmesh.Pos,
    clearance float64,
) []navmesh.Pos {
    if s.navmeshCapsule == nil || len(waypoints) == 0 {
        return waypoints
    }
    vecs := make([]pathfind.Vec3, len(waypoints))
    for i, wp := range waypoints {
        vecs[i] = pathfind.Vec3{X: wp.X, Y: wp.Y, Z: wp.Z}
    }
    vecs = s.navmeshCapsule.ApplyPath(vecs, clearance)
    vecs = s.navmeshCapsule.ShortenPath(vecs, clearance)
    positions := make([]navmesh.Pos, len(vecs))
    for i, vec := range vecs {
        positions[i] = navmesh.Pos{X: vec.X, Y: vec.Y, Z: vec.Z}
    }

    return positions
}

// legacyWaypoints runs the raw funnel answer through the capsule push and
// bend pass only - the before picture of the comparison toggle.
func (s *Server) legacyWaypoints(waypoints []navmesh.Pos,
    clearance float64,
) []navmesh.Pos {
    if s.navmeshCapsule == nil || len(waypoints) == 0 {
        return waypoints
    }
    vecs := make([]pathfind.Vec3, len(waypoints))
    for i, wp := range waypoints {
        vecs[i] = pathfind.Vec3{X: wp.X, Y: wp.Y, Z: wp.Z}
    }
    vecs = s.navmeshCapsule.ApplyPath(vecs, clearance)
    positions := make([]navmesh.Pos, len(vecs))
    for i, vec := range vecs {
        positions[i] = navmesh.Pos{X: vec.X, Y: vec.Y, Z: vec.Z}
    }

    return positions
}

// toNavmeshPoints converts the funnel waypoints to the JSON shape.
func toNavmeshPoints(waypoints []navmesh.Pos) []navmeshPoint {
    points := make([]navmeshPoint, len(waypoints))
    for i, waypoint := range waypoints {
        points[i] = navmeshPoint{
            X: waypoint.X,
            Y: waypoint.Y,
            Z: waypoint.Z,
        }
    }

    return points
}

// navmeshKeyOf parses a col_row path value into a region key.
func navmeshKeyOf(text string) (navmesh.RegionKey, bool) {
    colText, rowText, found := strings.Cut(text, "_")
    if !found {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }
    col, err := strconv.Atoi(colText)
    if err != nil || col < -32768 || col > 32767 {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }
    row, err := strconv.Atoi(rowText)
    if err != nil || row < -32768 || row > 32767 {
        return navmesh.RegionKey{Col: 0, Row: 0}, false
    }
    // The bounds checks above pin the int16 conversion range.
    col16, row16 := int16(col), int16(row) //nolint:gosec // guarded

    return navmesh.RegionKey{Col: col16, Row: row16}, true
}

// meshTileKeys lists the tile files of the mesh directory in the
// col-major order with their world footprints (the region rectangle
// of the tile key, no decode).
func meshTileKeys(mesh *navmesh.Mesh) []navmeshTileKey {
    files := mesh.TileFiles()
    keys := make([]navmeshTileKey, 0, len(files))
    for _, file := range files {
        keys = append(keys, tileKeyOf(file))
    }

    return keys
}

// tileKeysOf converts region keys to the wire tile keys.
func tileKeysOf(keys []navmesh.RegionKey) []navmeshTileKey {
    tiles := make([]navmeshTileKey, 0, len(keys))
    for _, key := range keys {
        tiles = append(tiles, tileKeyOf(key))
    }

    return tiles
}

// tileKeyOf derives the wire tile key: the footprint is a pure function of
// the region anchors (one region spans 32768 world units).
func tileKeyOf(key navmesh.RegionKey) navmeshTileKey {
    region := navmesh.TileWorldSize()
    minX := float64(key.Col-navmesh.TileZeroCol()) * region
    minY := float64(key.Row-navmesh.TileZeroRow()) * region

    return navmeshTileKey{
        Col:  key.Col,
        Row:  key.Row,
        MinX: minX,
        MinY: minY,
        MaxX: minX + region,
        MaxY: minY + region,
    }
}

// navmeshGeometry returns the cached binary geometry payload of one
// tile together with its ETag.
func (s *Server) navmeshGeometry(key navmesh.RegionKey,
) ([]byte, string, error) {
    return s.navmeshGeometryCached(s.navmeshMesh, s.navmeshGeo,
        key, "")
}

// navmeshGeometryCached is the encode-and-cache core of the pack geometry
// endpoints: a tile rasterizes once per process and the ETag revalidates for
// free. The label names the pack in the console line and the load error.
func (s *Server) navmeshGeometryCached(
    mesh *navmesh.Mesh, cache map[navmesh.RegionKey][]byte,
    key navmesh.RegionKey, label string,
) ([]byte, string, error) {
    s.navmeshGeoMu.Lock()
    defer s.navmeshGeoMu.Unlock()
    if payload, ok := cache[key]; ok {
        return payload, navmeshGeoETag(key, payload), nil
    }

    tile, err := mesh.Tile(key)
    if err != nil {
        return nil, "", fmt.Errorf("load %snavmesh tile %d_%d: %w",
            label, key.Col, key.Row, err)
    }
    began := time.Now()
    payload, err := encodeNavmeshGeometry(mesh, tile)
    duration := time.Since(began)
    // A cold tile pays the polygon soup encode; the process cache answers
    // the repeats.
    s.logger.Printf("Navmesh %sgeometry %d_%d: %.1f ms, %.1f MB",
        label, key.Col, key.Row, float64(duration.Nanoseconds())/1e6,
        float64(len(payload))/(1024*1024))
    if err != nil {
        return nil, "", err
    }
    cache[key] = payload

    return payload, navmeshGeoETag(key, payload), nil
}

// navmeshGeoETag derives the immutable tag: the key and the three block
// counts.
func navmeshGeoETag(key navmesh.RegionKey, payload []byte) string {
    return fmt.Sprintf(`"nmv2-%d_%d-%d-%d-%d"`, key.Col, key.Row,
        binary.LittleEndian.Uint32(payload[20:24]),
        binary.LittleEndian.Uint32(payload[24:28]),
        binary.LittleEndian.Uint32(payload[28:32]))
}

// encodeNavmeshGeometry, the NMV2 payload encoder of the tile
// endpoint, lives in navmesh_geometry.go together with the link
// portal and height step wall emission.
