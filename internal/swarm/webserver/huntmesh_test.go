// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestHuntMeshEndpoint serves the static Voronoi hunt mesh of the
// cell hunting: the registry payload answers with the version ETag
// (the conditional request short-circuits with 304), a registry
// without a cell-mode bot answers 404 (the legacy zone mode draws no
// mesh).
func TestHuntMeshEndpoint(t *testing.T) {
    server, bot := newTestServer(t)

    // Without an installed mesh the endpoint answers not found.
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/hunt-mesh", nil))
    require.Equal(t, http.StatusNotFound, recorder.Code)

    bot.SetHuntingCells("test-mesh-1", []state.CellMeshView{{
        ID: "c1", Name: "Home Cell", MinLevel: 1, MaxLevel: 4,
        FocusX: 45000, FocusY: 50000, Mass: 6,
        Verts: []int32{44500, 49500, 45500, 49500, 45500, 50500,
            44500, 50500},
    }})

    recorder = httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/hunt-mesh", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, `"test-mesh-1"`, recorder.Header().Get("ETag"))
    require.Contains(t, recorder.Body.String(), `"id":"c1"`)
    require.Contains(t, recorder.Body.String(), `"verts":[44500,49500`)

    // The conditional request against the current ETag short-circuits.
    request := httptest.NewRequest(http.MethodGet, "/api/hunt-mesh", nil)
    request.Header.Set("If-None-Match", `"test-mesh-1"`)
    recorder = httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, request)
    require.Equal(t, http.StatusNotModified, recorder.Code)
    require.Empty(t, recorder.Body.String())

    // The live snapshot carries the mesh version and the live record
    // of the held cell (the per second payload stays slim).
    bot.SetHuntingCellLive(state.CellLiveView{
        ID: "c1", Name: "Home Cell", State: "farming",
        MinLevel: 1, MaxLevel: 4, SpawnMass: 6,
        RespawnMinSec: 15, RespawnMaxSec: 20, NextRespawnSec: 7,
    })
    recorder = httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/bots/unittest1/state", nil))
    body := recorder.Body.String()
    require.Contains(t, body, `"huntMesh":"test-mesh-1"`,
        "the mesh version rides the snapshot")
    require.Contains(t, body, `"huntCell":{"id":"c1"`,
        "the live cell record rides the snapshot")
}
