// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/require"
)

// galleryCase is the per mode expectation of the gallery boot chain:
// the reported mode, the section markup, the script tag and file, the
// mode handshake of main.js and the exported identifiers of the
// gallery script.
type galleryCase struct {
    server        *Server
    mode          string
    section       string
    grid          string
    scroll        string
    pause         string
    picked        string
    scriptTag     string
    scriptPath    string
    scriptMarkers []string
    modeExpr      string
    initCall      string
}

// assertGallery verifies the config endpoint and the static assets of
// one gallery mode.
func assertGallery(t *testing.T, gallery galleryCase) {
    t.Helper()
    assertGalleryConfig(t, gallery)
    assertGalleryBootChain(t, gallery)
}

// assertGalleryConfig verifies GET /api/config reports the gallery
// mode and none of the bot data.
func assertGalleryConfig(t *testing.T, gallery galleryCase) {
    t.Helper()

    recorder := httptest.NewRecorder()
    gallery.server.httpServer.Handler.ServeHTTP(recorder,
        httptest.NewRequest(http.MethodGet, "/api/config", nil))
    require.Equal(t, http.StatusOK, recorder.Code)

    var config configResponse
    require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &config))
    require.Equal(t, gallery.mode, config.Mode)
    require.Nil(t, config.Geodata)
    require.Nil(t, config.Defaults)
}

// assertGalleryBootChain verifies the static asset chain of one
// gallery mode end to end at the file level: the index page references
// the section and the script, the script is served, the mode dispatch
// of main.js boots it and the map tile of the cells resolves.
func assertGalleryBootChain(t *testing.T, gallery galleryCase) {
    t.Helper()

    handler := gallery.server.httpServer.Handler

    recorder := httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    page := recorder.Body.String()
    for _, marker := range []string{gallery.section, gallery.grid,
        gallery.scroll, gallery.pause, gallery.picked,
        gallery.scriptTag} {
        require.Contains(t, page, marker)
    }

    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        gallery.scriptPath, nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    script := recorder.Body.String()
    for _, marker := range gallery.scriptMarkers {
        require.Contains(t, script, marker)
    }

    // The mode handshake of main.js must dispatch the gallery mode to
    // the gallery boot.
    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/main.js", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    mainScript := recorder.Body.String()
    require.Contains(t, mainScript, gallery.modeExpr)
    require.Contains(t, mainScript, gallery.initCall)

    // The map tile background of the gallery cells: the starter keltir
    // meadow tile of the world pyramid.
    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
        "/maps/0/21_19.jpg", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, "image/jpeg", recorder.Header().Get("Content-Type"))
    require.NotEmpty(t, recorder.Body.Bytes())
}
