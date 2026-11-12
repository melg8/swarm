// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestLazyDevScripts pins the payload policy of the web UI: the
// shared index page never references a dev gallery, the statistics
// tab or the pathfind UI eagerly, while main.js loads each on demand
// and the files stay served.
func TestLazyDevScripts(t *testing.T) {
    server, _ := newTestServer(t)
    handler := server.httpServer.Handler

    recorder := httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    page := recorder.Body.String()

    for _, file := range []string{
        "fight.js", "fighttest.js", "pathtest.js", "pathcurve.js",
        "pathflow.js", "stats.js", "pathfind.js",
    } {
        require.NotContains(t, page,
            `<script src="`+file+`"></script>`)

        recorder = httptest.NewRecorder()
        handler.ServeHTTP(recorder, httptest.NewRequest(
            http.MethodGet, "/"+file, nil))
        require.Equal(t, http.StatusOK, recorder.Code, file)
    }

    recorder = httptest.NewRecorder()
    handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/main.js", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    mainScript := recorder.Body.String()
    for _, marker := range []string{
        `"fight": "fight.js"`,
        `"test-fight": "fighttest.js"`,
        `"test-path": "pathtest.js"`,
        `"test-path-v2": "pathcurve.js"`,
        `"test-path-v3": "pathflow.js"`,
        `loadScript("stats.js")`,
        `loadScript("pathfind.js")`,
    } {
        require.Contains(t, mainScript, marker)
    }
}
