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

// navmeshGeoMagic is the magic word of the binary geometry payload
// ('NMV1' little endian). The fixed header of navmeshGeoHeaderSize
// bytes follows: magic u32, col i16, row i16, worldMinX i32,
// worldMinY i32, minH i16, maxH i16, polyCount u32; then the
// contiguous corner block and the area tail block described on
// navmeshGeoRecordSize (0 ground, 1 water).
const navmeshGeoMagic = 0x31564D4E

// navmeshGeoHeaderSize is the fixed prefix of the geometry payload.
const navmeshGeoHeaderSize = 24

// navmeshGeoCornerStride is the wire size of one polygon corner
// triple (cellX, cellY, height as int16).
const navmeshGeoCornerStride = 6

// navmeshGeoRecordSize is the wire size of one polygon: four corner
// triples plus the area byte. The layout keeps the corner block
// contiguous (every polygon contributes twelve int16 values in a
// row, the corner order X0Y0 X1Y0 X0Y1 X1Y1 - the world position of
// a corner is worldMin + cell*16) with the area bytes as one tail
// block, so the viewer reads the corners as a single Int16Array
// view. The triangles themselves never ride the wire: every polygon
// is its own quad of four consecutive corners, the viewer
// tessellates (0, 2, 1) (0, 2, 3).
const navmeshGeoRecordSize = navmeshGeoCornerStride*4 + 1

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
    payload, err := encodeNavmeshGeometry(tile)
    if err != nil {
        return nil, "", err
    }
    s.navmeshGeo[key] = payload

    return payload, navmeshGeoETag(key, payload), nil
}

// navmeshGeoETag derives the immutable geometry tag of a tile.
func navmeshGeoETag(key navmesh.RegionKey, payload []byte) string {
    return fmt.Sprintf(`"nmv1-%d_%d-%d"`, key.Col, key.Row,
        binary.LittleEndian.Uint32(payload[20:24]))
}

// encodeNavmeshGeometry renders one tile into the binary NMV1 payload
// the viewer tessellates: the quantized corner quads of every
// rectangle polygon plus the area bytes.
func encodeNavmeshGeometry(tile *navmesh.Tile) ([]byte, error) {
    if len(tile.Polys) == 0 {
        return nil, fmt.Errorf("tile %d_%d holds no polygons", tile.Col,
            tile.Row)
    }
    if int64(len(tile.Polys))*navmeshGeoRecordSize > 1<<31 {
        return nil, fmt.Errorf("tile %d_%d too large: %d polys", tile.Col,
            tile.Row, len(tile.Polys))
    }

    minX := tile.WorldMinX()
    minY := tile.WorldMinY()
    minH, maxH := int16(32767), int16(-32768)
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        for _, h := range []int16{poly.H00, poly.H10, poly.H01, poly.H11} {
            if h < minH {
                minH = h
            }
            if h > maxH {
                maxH = h
            }
        }
    }

    size := navmeshGeoHeaderSize + len(tile.Polys)*navmeshGeoRecordSize
    payload := make([]byte, size)
    binary.LittleEndian.PutUint32(payload[0:], navmeshGeoMagic)
    binary.LittleEndian.PutUint16(payload[4:], uint16(tile.Col))
    binary.LittleEndian.PutUint16(payload[6:], uint16(tile.Row))
    binary.LittleEndian.PutUint32(payload[8:], uint32(int32(minX)))
    binary.LittleEndian.PutUint32(payload[12:], uint32(int32(minY)))
    binary.LittleEndian.PutUint16(payload[16:], uint16(minH))
    binary.LittleEndian.PutUint16(payload[18:], uint16(maxH))
    binary.LittleEndian.PutUint32(payload[20:], uint32(len(tile.Polys)))

    offset := navmeshGeoHeaderSize
    for i := range tile.Polys {
        poly := &tile.Polys[i]
        writeGeoCorner(payload, offset, poly.X0, poly.Y0, int32(poly.H00))
        writeGeoCorner(payload, offset+6, poly.X1, poly.Y0, int32(poly.H10))
        writeGeoCorner(payload, offset+12, poly.X0, poly.Y1, int32(poly.H01))
        writeGeoCorner(payload, offset+18, poly.X1, poly.Y1, int32(poly.H11))
        offset += navmeshGeoCornerStride * 4
    }
    areas := navmeshGeoHeaderSize + len(tile.Polys)*navmeshGeoCornerStride*4
    for i := range tile.Polys {
        payload[areas+i] = tile.Polys[i].Area
    }

    return payload, nil
}

// writeGeoCorner stores one quantized polygon corner (the cell
// coordinates and the exact geodata height).
func writeGeoCorner(payload []byte, offset int, cellX, cellY, height int32) {
    binary.LittleEndian.PutUint16(payload[offset:], uint16(cellX))
    binary.LittleEndian.PutUint16(payload[offset+2:], uint16(cellY))
    binary.LittleEndian.PutUint16(payload[offset+4:], uint16(height))
}
