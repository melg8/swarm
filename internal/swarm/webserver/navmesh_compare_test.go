// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
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

// writeNavmeshCompareTile serializes the compare pack tile of the
// dual pack view tests: the same 20_18 region with the water polygon
// merged away - two ground polygons where the primary pack carries
// three (the reduced decomposition the diff highlights).
func writeNavmeshCompareTile(t *testing.T) string {
    t.Helper()
    tile := &navmesh.Tile{
        Col:   20,
        Row:   18,
        Climb: 40,
        Polys: []navmesh.Poly{
            {X0: 0, Y0: 0, X1: 1, Y1: 1, H00: 0, H10: 0, H01: 0,
                H11: 0, FirstLink: 0, Area: navmesh.AreaGround},
            {X0: 1, Y0: 0, X1: 2, Y1: 1, H00: 0, H10: 0, H01: 0,
                H11: 0, FirstLink: -1, Area: navmesh.AreaGround},
        },
        Links: []navmesh.Link{
            {Side: navmesh.SideMaxX, To: 1, Next: -1, T0: 0, T1: 0},
        },
        ExtLinks: nil,
        BVTree:   nil,
    }
    data, err := navmesh.EncodeTile(tile)
    require.NoError(t, err)
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "20_18.nm"), data, 0o600))

    // A tile the primary pack does not carry: the diff answers it as
    // fully added (the reduced pack covers a region the old one
    // lacks in this synthetic pair).
    extra := &navmesh.Tile{
        Col:   21,
        Row:   19,
        Climb: 40,
        Polys: []navmesh.Poly{
            {X0: 0, Y0: 0, X1: 1, Y1: 1, H00: 0, H10: 0, H01: 0,
                H11: 0, FirstLink: -1, Area: navmesh.AreaGround},
        },
    }
    data, err = navmesh.EncodeTile(extra)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(
        filepath.Join(dir, "21_19.nm"), data, 0o600))

    return dir
}

// newNavmeshDualTestServer builds the dual pack viewer: the primary
// pack of the house fixture and the reduced compare pack.
func newNavmeshDualTestServer(t *testing.T) *Server {
    t.Helper()

    return NewNavmeshServer(navmesh.NewMesh(writeNavmeshTestTile(t)),
        "127.0.0.1:0", log.New(io.Discard, "", 0),
        NavmeshOptions{
            CompareMesh: navmesh.NewMesh(writeNavmeshCompareTile(t)),
        })
}

// The boot payload names the compare pack: the viewer learns the
// second tile list from the same config fetch.
func TestNavmeshConfigCarriesCompare(t *testing.T) {
    server := newNavmeshDualTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/config", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    var answer navmeshConfigResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
    require.NotNil(t, answer.Compare)
    require.Equal(t, 2, answer.Compare.TileFiles)
    require.Len(t, answer.Compare.Tiles, 2)
    require.Equal(t, int16(20), answer.Compare.Tiles[0].Col)
}

// The single pack viewer answers no compare section.
func TestNavmeshConfigWithoutCompare(t *testing.T) {
    server := newNavmeshTestServer(t, nil)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/config", nil))

    var answer navmeshConfigResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
    require.Nil(t, answer.Compare)
}

// The compare geometry endpoint serves the second pack's payload: the
// same tile key, the reduced polygon count readable in the header.
func TestNavmeshCompareGeometry(t *testing.T) {
    server := newNavmeshDualTestServer(t)

    primary := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(primary, httptest.NewRequest(
        http.MethodGet, "/api/navmesh/geometry/20_18", nil))
    require.Equal(t, http.StatusOK, primary.Code)

    compare := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(compare, httptest.NewRequest(
        http.MethodGet, "/api/navmesh/compare/geometry/20_18", nil))
    require.Equal(t, http.StatusOK, compare.Code)
    require.NotEqual(t, primary.Body.Bytes(), compare.Body.Bytes(),
        "the reduced pack geometry differs from the primary")
}

// The diff endpoint names the vanished polygons: the water rectangle
// of the primary pack is absent from the reduced one.
func TestNavmeshCompareDiff(t *testing.T) {
    server := newNavmeshDualTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/navmesh/compare/diff/20_18", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    var answer navmeshCompareDiffResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
    require.Equal(t, int16(20), answer.Col)
    require.Equal(t, int16(18), answer.Row)
    require.Equal(t, 3, answer.PolysA)
    require.Equal(t, 2, answer.PolysB)
    require.Len(t, answer.Vanished, 1)
    require.Equal(t, int32(2), answer.Vanished[0].X0)
    require.Equal(t, int32(3), answer.Vanished[0].X1)
    require.Equal(t, int16(-48), answer.Vanished[0].H00)
    require.Empty(t, answer.Added)
}

// A tile the primary pack lacks diffs as fully added: the compare
// side carries it alone.
func TestNavmeshCompareDiffAddedTile(t *testing.T) {
    server := newNavmeshDualTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/navmesh/compare/diff/21_19", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    var answer navmeshCompareDiffResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
    require.Equal(t, 0, answer.PolysA)
    require.Equal(t, 1, answer.PolysB)
    require.Len(t, answer.Added, 1)
    require.Empty(t, answer.Vanished)
}

// A tile neither pack carries answers not found.
func TestNavmeshCompareDiffUnknownTile(t *testing.T) {
    server := newNavmeshDualTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/navmesh/compare/diff/22_20", nil))

    require.Equal(t, http.StatusNotFound, recorder.Code)
}

// The dual pack viewer without the compare mesh refuses the compare
// endpoints instead of answering an empty diff.
func TestNavmeshCompareEndpointsRefuseSinglePack(t *testing.T) {
    server := newNavmeshTestServer(t, nil)
    for _, path := range []string{
        "/api/navmesh/compare/geometry/20_18",
        "/api/navmesh/compare/diff/20_18",
    } {
        recorder := httptest.NewRecorder()
        server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
            http.MethodGet, path, nil))
        require.Equal(t, http.StatusNotImplemented, recorder.Code, path)
    }
}

// The compare pathfind: the same click pair answers from both meshes
// in one response (the viewer draws both routes).
func TestNavmeshPathCarriesCompareAnswer(t *testing.T) {
    server := newNavmeshDualTestServer(t)
    body := `{"start":{"x":100,"y":8,"z":0},"end":{"x":230,"y":8,"z":0}}`
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodPost, "/api/navmesh/path", strings.NewReader(body)))

    require.Equal(t, http.StatusOK, recorder.Code)
    var answer navmeshPathResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
    require.NotNil(t, answer.Compare,
        "the armed compare pack answers the same query")
    require.GreaterOrEqual(t, answer.Compare.DurationMs, 0.0)
}
