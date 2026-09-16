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

    mux := server.httpServer.Handler.(*http.ServeMux)
    mux.HandleFunc("GET /api/config", server.handleNavmeshConfig)
    mux.HandleFunc("GET /api/navmesh/tiles", server.handleNavmeshTiles)
    mux.HandleFunc("GET /api/navmesh/geometry/{key}",
        server.handleNavmeshGeometry)
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
}

// navmeshPathResponse is the reply of POST /api/navmesh/path: the
// funnel waypoints of the corridor search with the measured
// construction time (the first query of a region may include the lazy
// tile decode, roughly a millisecond per tile - the number stays
// honest, it is what a cold bot pays too).
type navmeshPathResponse struct {
    Found      bool           `json:"found"`
    Partial    bool           `json:"partial"`
    Error      string         `json:"error,omitempty"`
    Waypoints  []navmeshPoint `json:"waypoints"`
    DurationMs float64        `json:"durationMs"`
    Explored   int            `json:"explored"`
    Corridor   int            `json:"corridor"`
    Filter     string         `json:"filter"`
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

// handleNavmeshPath runs one corridor search of the mesh between the
// two double clicked points and measures the construction time.
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
    filter, filterName := navmesh.DryFilter(), "dry"
    if request.Filter != "dry" {
        filter, filterName = navmesh.DefaultFilter(), "swim"
    }

    start := navmesh.Pos{X: request.Start.X, Y: request.Start.Y,
        Z: request.Start.Z}
    end := navmesh.Pos{X: request.End.X, Y: request.End.Y,
        Z: request.End.Z}
    began := time.Now()
    route, err := s.navmeshMesh.Route(start, end, filter)
    duration := time.Since(began)

    response := navmeshPathResponse{
        Found:      false,
        Partial:    false,
        Error:      "",
        Waypoints:  []navmeshPoint{},
        DurationMs: float64(duration.Nanoseconds()) / 1e6,
        Explored:   0,
        Corridor:   0,
        Filter:     filterName,
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
        response.Waypoints = toNavmeshPoints(route.Waypoints)
    }
    writeJSON(w, s.logger, response)
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
    payload, err := encodeNavmeshGeometry(s.navmeshMesh, tile)
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
