// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "encoding/binary"
    "encoding/json"
    "io"
    "log"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// newNavmeshTestServer builds a navmesh viewer server over a temp
// tile directory holding one synthetic region: three flat ground
// polygons in a row, the last one water, linked through the maxx
// portals. The world anchors sit at the origin (region 20_18), so
// the polygons span x 0..48 of the y strip 0..16.
func newNavmeshTestServer(
    t *testing.T, initial []navmesh.RegionKey,
) *Server {
    t.Helper()
    tile := &navmesh.Tile{
        Col:   20,
        Row:   18,
        Climb: 40,
        Polys: []navmesh.Poly{
            {X0: 0, Y0: 0, X1: 1, Y1: 1, H00: 0, H10: 0, H01: 0,
                H11: 0, FirstLink: 0, Area: navmesh.AreaGround},
            {X0: 1, Y0: 0, X1: 2, Y1: 1, H00: 0, H10: 0, H01: 0,
                H11: 0, FirstLink: 1, Area: navmesh.AreaGround},
            {X0: 2, Y0: 0, X1: 3, Y1: 1, H00: -300, H10: -300,
                H01: -300, H11: -300, FirstLink: -1,
                Area: navmesh.AreaWater},
        },
        Links: []navmesh.Link{
            {Side: navmesh.SideMaxX, To: 1, Next: -1, T0: 0, T1: 0},
            {Side: navmesh.SideMaxX, To: 2, Next: -1, T0: 0, T1: 0},
        },
        ExtLinks: nil,
        BVTree:   nil,
    }
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "20_18.nm"), data, 0o600))

    return NewNavmeshServer(navmesh.NewMesh(dir), "127.0.0.1:0",
        log.New(io.Discard, "", 0), NavmeshOptions{InitialTiles: initial})
}

// navmeshGet exercises one handler with a fresh recorder.
func navmeshGet(t *testing.T, server *Server, path string) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder,
        httptest.NewRequest(http.MethodGet, path, nil))

    return recorder
}

// navmeshPost exercises one handler with a JSON body.
func navmeshPost(t *testing.T, server *Server, path, body string,
) *httptest.ResponseRecorder {
    t.Helper()
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodPost, path, strings.NewReader(body)))

    return recorder
}

// TestNavmeshConfigEndpoint pins the boot payload: the mode is the
// navmesh viewer, the tile listing carries the derived world
// footprint and the initial selection mirrors the flag selection
// (an empty selection answers null - every tile stitched).
func TestNavmeshConfigEndpoint(t *testing.T) {
    t.Run("empty selection is the stitched world", func(t *testing.T) {
        server := newNavmeshTestServer(t, nil)
        recorder := navmeshGet(t, server, "/api/config")

        require.Equal(t, http.StatusOK, recorder.Code)

        var config navmeshConfigResponse
        require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
        require.Equal(t, modeNavmesh, config.Mode)
        require.Equal(t, 1, config.Navmesh.TileFiles)
        require.Nil(t, config.Navmesh.InitialTiles)
        require.Len(t, config.Navmesh.Tiles, 1)
        require.EqualValues(t, 20, config.Navmesh.Tiles[0].Col)
        require.EqualValues(t, 18, config.Navmesh.Tiles[0].Row)
        require.InDelta(t, 0, config.Navmesh.Tiles[0].MinX, 0.01)
        require.InDelta(t, 0, config.Navmesh.Tiles[0].MinY, 0.01)
        require.InDelta(t, 32768, config.Navmesh.Tiles[0].MaxX, 0.01)
        require.InDelta(t, 32768, config.Navmesh.Tiles[0].MaxY, 0.01)
    })

    t.Run("the flag selection lands in the initial tiles", func(t *testing.T) {
        server := newNavmeshTestServer(t, []navmesh.RegionKey{
            {Col: 20, Row: 18},
        })
        recorder := navmeshGet(t, server, "/api/config")

        require.Equal(t, http.StatusOK, recorder.Code)

        var config navmeshConfigResponse
        require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
        require.Len(t, config.Navmesh.InitialTiles, 1)
        require.EqualValues(t, 20, config.Navmesh.InitialTiles[0].Col)
        require.EqualValues(t, 18, config.Navmesh.InitialTiles[0].Row)
    })
}

// TestNavmeshTilesEndpoint pins the tile listing without a decode.
func TestNavmeshTilesEndpoint(t *testing.T) {
    server := newNavmeshTestServer(t, nil)
    recorder := navmeshGet(t, server, "/api/navmesh/tiles")

    require.Equal(t, http.StatusOK, recorder.Code)

    var tiles navmeshTilesResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &tiles))
    require.Len(t, tiles.Tiles, 1)
    require.EqualValues(t, 20, tiles.Tiles[0].Col)
}

// TestNavmeshGeometryEndpoint pins the binary NMV1 payload: the
// header derivation (the world anchor of the region, the exact
// corner heights), the quantized corner layout and the area bytes,
// plus the revalidation dance of the immutable ETag.
func TestNavmeshGeometryEndpoint(t *testing.T) {
    server := newNavmeshTestServer(t, nil)
    recorder := navmeshGet(t, server, "/api/navmesh/geometry/20_18")

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, "application/octet-stream",
        recorder.Header().Get("Content-Type"))
    require.NotEmpty(t, recorder.Header().Get("ETag"))

    payload := recorder.Body.Bytes()
    require.Len(t, payload, navmeshGeoHeaderSize+3*navmeshGeoRecordSize)
    require.EqualValues(t, navmeshGeoMagic,
        binary.LittleEndian.Uint32(payload[0:4]))
    require.EqualValues(t, 20, int16(binary.LittleEndian.Uint16(payload[4:6])))
    require.EqualValues(t, 18, int16(binary.LittleEndian.Uint16(payload[6:8])))
    require.EqualValues(t, 0, int32(binary.LittleEndian.Uint32(payload[8:12])))
    require.EqualValues(t, 0, int32(binary.LittleEndian.Uint32(payload[12:16])))
    require.EqualValues(t, -300, int16(binary.LittleEndian.Uint16(payload[16:18])))
    require.EqualValues(t, 0, int16(binary.LittleEndian.Uint16(payload[18:20])))
    require.EqualValues(t, 3, binary.LittleEndian.Uint32(payload[20:24]))

    // The first polygon spans cells (0,0)-(1,1) flat at height 0:
    // the four corners land in the documented order X0Y0 X1Y0 X0Y1
    // X1Y1 and the area byte closes the record.
    corner := func(index, part int) int16 {
        base := navmeshGeoHeaderSize + index*6 + part*2

        return int16(binary.LittleEndian.Uint16(payload[base:]))
    }
    require.EqualValues(t, 0, corner(0, 0))
    require.EqualValues(t, 0, corner(0, 1))
    require.EqualValues(t, 0, corner(0, 2))
    require.EqualValues(t, 1, corner(1, 0))
    require.EqualValues(t, 0, corner(1, 1))
    require.EqualValues(t, 0, corner(3, 2))
    require.EqualValues(t, 1, corner(4, 0))
    require.EqualValues(t, 0, corner(4, 1))
    require.Equal(t, navmesh.AreaGround,
        payload[navmeshGeoHeaderSize+3*navmeshGeoCornerStride*4])
    require.Equal(t, navmesh.AreaWater,
        payload[navmeshGeoHeaderSize+3*navmeshGeoCornerStride*4+2])

    t.Run("the etag revalidates for free", func(t *testing.T) {
        etag := recorder.Header().Get("ETag")
        request := httptest.NewRequest(http.MethodGet,
            "/api/navmesh/geometry/20_18", nil)
        request.Header.Set("If-None-Match", etag)
        revalidate := httptest.NewRecorder()
        server.httpServer.Handler.ServeHTTP(revalidate, request)

        require.Equal(t, http.StatusNotModified, revalidate.Code)
        require.Empty(t, revalidate.Body.Bytes())
    })

    t.Run("a missing tile answers 404", func(t *testing.T) {
        recorder := navmeshGet(t, server, "/api/navmesh/geometry/21_19")
        require.Equal(t, http.StatusNotFound, recorder.Code)
    })

    t.Run("a malformed key answers 400", func(t *testing.T) {
        recorder := navmeshGet(t, server, "/api/navmesh/geometry/ab")
        require.Equal(t, http.StatusBadRequest, recorder.Code)
    })
}

// TestNavmeshPathEndpoint pins the route search of the viewer: the
// swim filter walks into the water polygon and answers found, the
// dry filter walls it and answers the partial corridor, and both
// carry the measured construction time.
func TestNavmeshPathEndpoint(t *testing.T) {
    server := newNavmeshTestServer(t, nil)

    t.Run("the swim route reaches the water polygon", func(t *testing.T) {
        recorder := navmeshPost(t, server, "/api/navmesh/path",
            `{"start":{"x":8,"y":8,"z":0},"end":{"x":40,"y":8,"z":-300},`+
                `"filter":"swim"}`)

        require.Equal(t, http.StatusOK, recorder.Code)

        var response navmeshPathResponse
        require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
        require.True(t, response.Found)
        require.False(t, response.Partial)
        require.Equal(t, "swim", response.Filter)
        require.NotEmpty(t, response.Waypoints)
        require.InDelta(t, 8, response.Waypoints[0].X, 0.01)
        require.InDelta(t, 8, response.Waypoints[0].Y, 0.01)
        last := response.Waypoints[len(response.Waypoints)-1]
        require.InDelta(t, 40, last.X, 0.01)
        require.InDelta(t, 8, last.Y, 0.01)
        require.InDelta(t, -300, last.Z, 0.01)
        require.Greater(t, response.DurationMs, float64(0))
        require.GreaterOrEqual(t, response.Explored, 1)
        require.GreaterOrEqual(t, response.Corridor, 2)
    })

    t.Run("the dry filter answers the partial corridor", func(t *testing.T) {
        recorder := navmeshPost(t, server, "/api/navmesh/path",
            `{"start":{"x":8,"y":8,"z":0},"end":{"x":40,"y":8,"z":-300},`+
                `"filter":"dry"}`)

        require.Equal(t, http.StatusOK, recorder.Code)

        var response navmeshPathResponse
        require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
        require.False(t, response.Found)
        require.True(t, response.Partial)
        require.Equal(t, "dry", response.Filter)
        require.NotEmpty(t, response.Waypoints)
        // The partial corridor ends on the last dry polygon (the
        // ground strip x 16..32), never inside the water.
        last := response.Waypoints[len(response.Waypoints)-1]
        require.InDelta(t, 32, last.X, 0.01)
        require.InDelta(t, 8, last.Y, 0.01)
        require.InDelta(t, 0, last.Z, 0.01)
    })

    t.Run("a bad body answers 400", func(t *testing.T) {
        recorder := navmeshPost(t, server, "/api/navmesh/path", "{")
        require.Equal(t, http.StatusBadRequest, recorder.Code)
    })
}

// TestEncodeNavmeshGeometryRejectsEmptyTiles pins the guard: a tile
// without polygons has no viewer payload.
func TestEncodeNavmeshGeometryRejectsEmptyTiles(t *testing.T) {
    _, err := encodeNavmeshGeometry(&navmesh.Tile{Col: 20, Row: 18})
    require.Error(t, err)
}

// TestNavmeshViewScriptContract pins the feedback channel of the
// viewer script (navmesh_view.js): the view state link parameters the
// boot parses, the flight rig controls and the copy button wiring.
// The script itself runs in the browser - the live verification of
// the round drove it headlessly - this pin keeps the boot contract
// from drifting silently (a renamed parameter or a dropped button
// would break every link already shared).
func TestNavmeshViewScriptContract(t *testing.T) {
    script, err := os.ReadFile(filepath.Join("web", "navmesh_view.js"))
    require.NoError(t, err)
    source := string(script)

    // The flight rig: wasd flies, q/e climb and descend, the wheel
    // retunes the speed, the drag yaws and pitches.
    for _, control := range []string{
        `createFlyRig(viewer.camera, canvas)`,
        `"KeyW"`, `"KeyA"`, `"KeyS"`, `"KeyD"`, `"KeyQ"`, `"KeyE"`,
        `rig.pitch = Math.min(1.55, Math.max(-1.55, rig.pitch - dy`,
    } {
        require.Contains(t, source, control,
            "the flight rig control is missing from the viewer script")
    }

    // The view state link: the boot parses the camera pose, the route
    // pair, the tile selection, the filter and the scale; the copy
    // button builds the URL back.
    for _, part := range []string{
        `search.get("cam")`, `search.get("from")`, `search.get("to")`,
        `search.get("tiles")`, `search.get("filter")`,
        `search.get("scale")`,
        `params.set("cam"`, `params.set("from"`, `params.set("to"`,
        `params.set("tiles"`, `params.set("filter"`, `params.set("scale"`,
        `copyViewState)`, `id="nmv-copy"`, `id="nmv-link"`,
    } {
        require.Contains(t, source, part,
            "the view state link contract is missing from the viewer"+
                " script")
    }

    // The boot restores the route pair and runs the search on its own.
    require.Contains(t, source, `void requestRoute(view.from, view.to)`,
        "the restored route pair must run automatically")
    // The camera restores through the world axes mapping (three y is
    // the height, scaled).
    require.Contains(t, source, `cam.x, cam.z * viewer.heightScale, cam.y`)
}
