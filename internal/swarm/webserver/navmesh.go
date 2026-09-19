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

// The navmesh viewer mode (the -show-navmesh flag of cmd/swarm): the
// bot less 3D inspection surface of docs/navmesh.md. The mesh tiles
// render through the embedded three.js viewer, a double click pair
// asks the real corridor search for the route and the answer carries
// the measured construction time. The flag selects the tiles: a bare
// -show-navmesh loads every tile of the directory stitched together
// (the links cross the region borders through the external targets),
// -show-navmesh=21_19 opens the named tiles only - the route queries
// always run over the full directory mesh, so a path may leave the
// visible tiles (the polyline draws wherever it walks).
//
// The binary geometry contract of the tile endpoint lives in
// navmesh_geometry.go: the NMV2 payload of the surface quads, the
// link portals of the edge connections overlay and the height step
// walls that close the cracks between adjacent bilinear surfaces.

// navmeshPathBodyLimit caps the JSON request of a route query.
const navmeshPathBodyLimit = 4096

// NavmeshOptions tunes the navmesh viewer server.
type NavmeshOptions struct {
    // InitialTiles are the tile keys the viewer opens with; an empty
    // selection opens every tile of the directory (the stitched
    // world). The -show-navmesh=21_19 form of the flag fills it.
    InitialTiles []navmesh.RegionKey
    // Engine is the geodata engine behind the original geometry and
    // the capsule clearance of the route answers. A nil engine keeps
    // the viewer mesh only with the raw funnel pivots (the tests).
    Engine *pathfind.Engine
}

// NewNavmeshServer creates the web server of the navmesh viewer mode:
// no game connection and no bot, the 3D mesh inspection surface with
// the interactive route search of the real corridor query.
func NewNavmeshServer(
    mesh *navmesh.Mesh, address string, logger *log.Logger,
    options NavmeshOptions,
) *Server {
    server := newServer(address, logger)
    server.navmeshMesh = mesh
    server.navmeshTiles = options.InitialTiles
    server.navmeshEngine = options.Engine
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

    return server
}

// navmeshConfigResponse is the boot payload of the viewer page.
type navmeshConfigResponse struct {
    Mode    string        `json:"mode"`
    Navmesh navmeshConfig `json:"navmesh"`
}

// navmeshConfig tells the viewer which tiles exist and which of them
// the flag selected.
type navmeshConfig struct {
    Dir       string           `json:"dir"`
    TileFiles int              `json:"tileFiles"`
    Tiles     []navmeshTileKey `json:"tiles"`
    // InitialTiles lists the flag selected keys; nil means every
    // tile (the bare -show-navmesh of the stitched world).
    InitialTiles []navmeshTileKey `json:"initialTiles"`
}

// navmeshTileKey names one tile with its world footprint (the tile
// rectangle spans MinX..MaxX, MinY..MaxY of the world plane).
type navmeshTileKey struct {
    Col  int16   `json:"col"`
    Row  int16   `json:"row"`
    MinX float64 `json:"minX"`
    MinY float64 `json:"minY"`
    MaxX float64 `json:"maxX"`
    MaxY float64 `json:"maxY"`
}

// navmeshPoint is one world position of the navmesh API: X and Y are
// the horizontal world axes, Z the height.
type navmeshPoint struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z"`
}

// navmeshPathRequest is the body of POST /api/navmesh/path.
type navmeshPathRequest struct {
    Start  navmeshPoint `json:"start"`
    End    navmeshPoint `json:"end"`
    Filter string       `json:"filter"`
    // Approach is the approach radius of the search (the plan repro
    // contract): a positive value succeeds on the first polygon whose
    // surface sits within the radius of the end (the bot trip
    // searches plan with 200), the zero default keeps the exact
    // destination contract of the double click pair.
    Approach float64 `json:"approach,omitempty"`
    // Avoid lists the ban circles the search seals (the frozen areas
    // the plan carried). Empty for the clean searches.
    Avoid []navmeshAvoidCircle `json:"avoid,omitempty"`
    // Fold runs the capsule post pass over the answer (the pushes and
    // the bend pass, then the fold into the longest grid clear legs -
    // the double click default). The plan repro links send false: the
    // answer then IS the search answer the bot publishes as its walk
    // plan, no grid pass bends it (the grid oracle is water blind -
    // the 2026-09-19 route mismatch round: the fold drew a straight
    // chord over the lake the mesh route detours).
    Fold *bool `json:"fold,omitempty"`
}

// navmeshAvoidCircle is one ban circle of the route request: the
// center on the world plane and the radius to seal.
type navmeshAvoidCircle struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    R float64 `json:"r"`
}

// navmeshPathResponse is the reply of POST /api/navmesh/path: the
// waypoints of the corridor search with the measured construction
// time (the first query of a region may include the lazy tile
// decode, roughly a millisecond per tile - the number stays honest,
// it is what a cold bot pays too). The smoothed answer rides in
// waypoints when the capsule clearance arms the shortcut pass; the
// raw funnel answer rides in rawWaypoints for the comparison toggle.
type navmeshPathResponse struct {
    Found        bool           `json:"found"`
    Partial      bool           `json:"partial"`
    Error        string         `json:"error,omitempty"`
    Waypoints    []navmeshPoint `json:"waypoints"`
    RawWaypoints []navmeshPoint `json:"rawWaypoints,omitempty"`
    DurationMs   float64        `json:"durationMs"`
    Explored     int            `json:"explored"`
    Corridor     int            `json:"corridor"`
    Filter       string         `json:"filter"`
}

// handleNavmeshConfig answers the mode and the tile listing of the
// viewer boot.
func (s *Server) handleNavmeshConfig(w http.ResponseWriter, _ *http.Request) {
    stats := s.navmeshMesh.Stats()
    tiles := meshTileKeys(s.navmeshMesh)
    initial := []navmeshTileKey(nil)
    if len(s.navmeshTiles) > 0 {
        initial = tileKeysOf(s.navmeshTiles)
    }
    writeJSON(w, s.logger, navmeshConfigResponse{
        Mode: modeNavmesh,
        Navmesh: navmeshConfig{
            Dir:          stats.Dir,
            TileFiles:    stats.TileFiles,
            Tiles:        tiles,
            InitialTiles: initial,
        },
    })
}

// handleNavmeshTiles lists the tile files of the mesh directory with
// their world footprints. No tile is decoded: the listing reads the
// directory scan, the geometry endpoint loads on demand.
func (s *Server) handleNavmeshTiles(w http.ResponseWriter, _ *http.Request) {
    writeJSON(w, s.logger, navmeshTilesResponse{
        Tiles: meshTileKeys(s.navmeshMesh),
    })
}

// navmeshTilesResponse is the payload of GET /api/navmesh/tiles.
type navmeshTilesResponse struct {
    Tiles []navmeshTileKey `json:"tiles"`
}

// handleNavmeshGeometry serves the binary polygon soup of one tile.
// The payload is immutable for a given tile file, so the ETag lets the
// browser revalidate for free and the server caches the encoded bytes
// for the lifetime of the process.
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

// handleNavmeshOriginal serves the binary polygon soup of one region
// rendered from the RAW l2j geodata cells (the comparison variant of
// the geometry toggle). The payload contract is the NMV2 of the mesh
// endpoint with an empty link block; the ETag lets the browser
// revalidate for free and the server caches the encoded bytes for the
// lifetime of the process.
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

// filterNameDry is the wire word the dump header and the pathfind
// link carry for the dry (water sealed) mesh search.
const filterNameDry = "dry"

// handleNavmeshPath runs one corridor search of the mesh between the
// two double clicked points (the exact destination, the fold pipeline)
// or one plan repro search of a pathfind link (the approach radius,
// the ban circles, the raw answer), and measures the construction
// time.
//
//nolint:funlen,cyclop // the handler mirrors the validation steps in order
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
    filter, filterName := navmesh.DryFilter(), filterNameDry
    if request.Filter != filterNameDry {
        filter, filterName = navmesh.DefaultFilter(), "swim"
        // The swim pricing follows the server water zone data: the
        // water polygons a C1 WaterZone cuboid covers swim at the
        // run/swim speed ratio, the river beds the zone data omits
        // walk at the plain land rate (the zone data, not the depth,
        // prices the swim).
        filter.WaterZones = navmesh.C1WaterZones()
    }
    clearance := 0.0
    if s.navmeshEngine != nil {
        clearance = s.navmeshEngine.CapsuleRadius()
    }
    filter.WaypointClearance = clearance
    filter.Smooth = clearance > 0
    if s.navmeshCapsule != nil {
        filter.Guard = s.navmeshCapsule
    }

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
    if request.Approach > 0 {
        route, err = s.navmeshMesh.RouteApproach(start, end,
            request.Approach, filter)
    } else {
        route, err = s.navmeshMesh.Route(start, end, filter)
    }
    duration := time.Since(began)
    // The console line keeps the construction cost observable from
    // the server window (the viewer answer carries the same number
    // in the duration field): the cold request pays the tile decode,
    // the repeat answers from the resident mesh.
    if err != nil {
        s.logger.Printf("Navmesh route (%s): failed in %.1f ms: %v",
            filterName, float64(duration.Nanoseconds())/1e6, err)
    } else {
        if route == nil {
            s.logger.Printf("Navmesh route (%s): no path in %.1f ms",
                filterName, float64(duration.Nanoseconds())/1e6)
        } else {
            status := "no path"
            switch {
            case route.Found:
                status = "found"
            case route.Partial:
                status = "partial"
            }
            s.logger.Printf("Navmesh route (%s): %s in %.1f ms - "+
                "%d waypoints, %d regions", filterName, status,
                float64(duration.Nanoseconds())/1e6,
                len(route.Waypoints), len(route.Corridor))
        }
    }

    response := navmeshPathResponse{
        Found:        false,
        Partial:      false,
        Error:        "",
        Waypoints:    []navmeshPoint{},
        RawWaypoints: nil,
        DurationMs:   float64(duration.Nanoseconds()) / 1e6,
        Explored:     0,
        Corridor:     0,
        Filter:       filterName,
    }
    if err != nil {
        response.Error = err.Error()
        writeJSON(w, s.logger, response)

        return
    }
    if route != nil {
        response.Found = route.Found
        response.Partial = route.Partial
        response.Explored = route.Explored
        response.Corridor = len(route.Corridor)
        // The fold switch: the smoothed answer walks the full
        // pipeline (the pushes and the bends of the capsule pass,
        // then the fold into the longest grid clear legs), the plan
        // repro answer (fold=false) serves the search waypoints as
        // the bot publishes them. The raw funnel answer keeps the
        // legacy post pass only - it is the before picture of the
        // comparison toggle.
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
    writeJSON(w, s.logger, response)
}

// clearedWaypoints runs the answer waypoints through the capsule
// clearance post pass when the viewer engine arms it (the funnel
// pivot clearance covers the turns, the smoothing covers the merged
// legs, the post pass covers whatever still grazes a wall) and folds
// the result into the longest grid clear legs - the walker consumes
// legs that answer the server movement rules with the capsule
// clearance.
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

// legacyWaypoints runs the raw funnel answer through the capsule
// push and bend pass only - the pre smoothing pipeline the owner
// compares against (the granular pivots and the anchor chains stay
// visible in the raw variant of the route toggle).
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

// tileKeyOf derives the wire tile key of a region: the world footprint
// is a pure function of the region anchors (one region spans 32768
// world units from its min corner).
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
    s.navmeshGeoMu.Lock()
    defer s.navmeshGeoMu.Unlock()
    if s.navmeshGeo == nil {
        s.navmeshGeo = make(map[navmesh.RegionKey][]byte)
    }
    if payload, ok := s.navmeshGeo[key]; ok {
        return payload, navmeshGeoETag(key, payload), nil
    }

    tile, err := s.navmeshMesh.Tile(key)
    if err != nil {
        return nil, "", fmt.Errorf("load navmesh tile %d_%d: %w", key.Col,
            key.Row, err)
    }
    began := time.Now()
    payload, err := encodeNavmeshGeometry(s.navmeshMesh, tile)
    duration := time.Since(began)
    // The first open of a tile pays the polygon soup encode (the
    // process cache answers the repeats); the console line keeps
    // that cost observable.
    s.logger.Printf("Navmesh geometry %d_%d: %.1f ms, %.1f MB",
        key.Col, key.Row, float64(duration.Nanoseconds())/1e6,
        float64(len(payload))/(1024*1024))
    if err != nil {
        return nil, "", err
    }
    s.navmeshGeo[key] = payload

    return payload, navmeshGeoETag(key, payload), nil
}

// navmeshGeoETag derives the immutable geometry tag of a tile: the
// region key with the three block counts of the NMV2 payload.
func navmeshGeoETag(key navmesh.RegionKey, payload []byte) string {
    return fmt.Sprintf(`"nmv2-%d_%d-%d-%d-%d"`, key.Col, key.Row,
        binary.LittleEndian.Uint32(payload[20:24]),
        binary.LittleEndian.Uint32(payload[24:28]),
        binary.LittleEndian.Uint32(payload[28:32]))
}

// encodeNavmeshGeometry, the NMV2 payload encoder of the tile
// endpoint, lives in navmesh_geometry.go together with the link
// portal and height step wall emission.
