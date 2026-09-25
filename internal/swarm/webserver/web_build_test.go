// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "testing/fstest"

    "github.com/stretchr/testify/require"
)

// The build id is the content identity of the embedded UI: stable
// across reads (the client compares it every two seconds) and
// sensitive to the content (a new deployment must read differently).
func TestWebBuildIDStableAndContentBound(t *testing.T) {
    tree := fstest.MapFS{
        "web/index.html":         {Data: []byte("<html><head></head></html>")},
        "web/app.js":             {Data: []byte("const a = 1;")},
        "web/maps/21_19/0_0.jpg": {Data: []byte("tile")},
    }
    first := webBuildID(tree)
    require.NotEmpty(t, first)
    require.Equal(t, first, webBuildID(tree),
        "the same tree hashes identically")

    tree["web/app.js"] = &fstest.MapFile{Data: []byte("const a = 2;")}
    require.NotEqual(t, first, webBuildID(tree),
        "a changed asset changes the build id")
}

// The meta injection targets the head of the index page exactly once.
func TestInjectBuildMeta(t *testing.T) {
    page := "<html>\n<head>\n<title>swarm</title>\n</head>\n</html>"
    injected := injectBuildMeta(page, "abc123")
    require.Equal(t, 1, strings.Count(injected, "swarm-build"))
    require.Contains(t, injected,
        "<head>\n  <meta name=\"swarm-build\" content=\"abc123\">")
    // Idempotent: injecting the same id again changes nothing.
    require.Equal(t, injected, injectBuildMeta(injected, "abc123"))

    // A headless page and an empty build pass through unchanged.
    require.Equal(t, "no head", injectBuildMeta("no head", "abc123"))
    require.Equal(t, page, injectBuildMeta(page, ""))
}

// The served index page carries the build meta and the no-cache
// header, so a reload after a deployment picks the binary's own page
// (issue #48).
func TestIndexServesBuildMeta(t *testing.T) {
    server, _ := newTestServer(t)
    require.NotEmpty(t, server.webBuild)

    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, "no-cache",
        recorder.Header().Get("Cache-Control"))
    body, err := io.ReadAll(recorder.Body)
    require.NoError(t, err)
    require.Contains(t, string(body),
        "<meta name=\"swarm-build\" content=\""+server.webBuild+"\">")

    // The direct index.html request answers the same injected page.
    recorder = httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/index.html", nil))
    body, err = io.ReadAll(recorder.Body)
    require.NoError(t, err)
    require.Contains(t, string(body), "swarm-build")
}

// The bots poll names the build of the running binary: the web app
// compares it against its own meta and reloads on a mismatch
// (issue #48).
func TestBotListCarriesBuildHeader(t *testing.T) {
    server, _ := newTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/bots", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, server.webBuild,
        recorder.Header().Get("X-Swarm-Build"))
}

// The scripts and styles answer with no-cache: the embedded files
// carry no validator, a heuristic browser cache could otherwise keep
// the OLD asset after the reload and the page would still run stale
// code (issue #48). The tiles and icons keep their default caching.
func TestStaticScriptsAnswerNoCache(t *testing.T) {
    server, _ := newTestServer(t)

    for path, want := range map[string]string{
        "/app.js":             "no-cache",
        "/style.css":          "no-cache",
        "/maps/21_19/0_0.jpg": "",
    } {
        recorder := httptest.NewRecorder()
        server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
            http.MethodGet, path, nil))
        require.Equal(t, want, recorder.Header().Get("Cache-Control"),
            "cache header of %s", path)
    }
}
