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
    "testing"

    "github.com/stretchr/testify/require"
)

// newPathGalleryServer builds the walk plan gallery server.
func newPathGalleryServer() *Server {
    return NewTestPathServer("127.0.0.1:0",
        log.New(io.Discard, "", 0))
}

func TestPathConfigEndpoint(t *testing.T) {
    server := newPathGalleryServer()
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/config", nil))

    require.Equal(t, http.StatusOK, recorder.Code)

    var config configResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
    require.Equal(t, modeTestPath, config.Mode)
    require.Nil(t, config.Geodata)
    require.Nil(t, config.Defaults)
}

// TestPathStaticAssets verifies the gallery boot chain end to end at
// the file level: the index page references the gallery script and the
// gallery section markup, the script itself is served and the map tile
// the cells paint their background from resolves.
func TestPathStaticAssets(t *testing.T) {
    server := newPathGalleryServer()
    handler := server.httpServer.Handler

    recorder := httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    page := recorder.Body.String()
    require.Contains(t, page, `id="path-section"`)
    require.Contains(t, page, `id="path-grid"`)
    require.Contains(t, page, `id="path-scroll"`)
    require.Contains(t, page, `id="path-pause"`)
    require.Contains(t, page, `id="path-picked"`)
    require.Contains(t, page, `<script src="pathtest.js"></script>`)

    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/pathtest.js", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    script := recorder.Body.String()
    require.Contains(t, script, "const PATH_VARIANTS = [")
    require.Contains(t, script, "const PathTest = {")

    // The mode handshake of main.js must dispatch "test-path" to the
    // gallery boot.
    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/main.js", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    mainScript := recorder.Body.String()
    require.Contains(t, mainScript, `config.mode === "test-path"`)
    require.Contains(t, mainScript, "PathTest.init()")

    // The map tile background of the gallery cells: the starter
    // keltir meadow tile of the world pyramid.
    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/maps/0/21_19.jpg", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, "image/jpeg", recorder.Header().Get("Content-Type"))
    require.NotEmpty(t, recorder.Body.Bytes())
}
