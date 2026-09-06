// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/stretchr/testify/require"
)

// newGeodataTestServer builds a pathfind test server over a temp
// geodata directory with one flat region file.
func newGeodataTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	block := []byte{0, 0, 0} // flat block at height 0
	data := make([]byte, 0, 65536*len(block))
	for i := 0; i < 65536; i++ {
		data = append(data, block...)
	}
	name := filepath.Join(dir, "22_22.l2j")
	require.NoError(t, os.WriteFile(name, data, 0o644))

	engine := pathfind.NewEngine(dir)

	return NewPathfindServer(engine, "127.0.0.1:0", log.New(io.Discard, "", 0),
		PathfindOptions{})
}

// TestHandleGeodataTile checks the happy path of the tile endpoint: a
// decodable PNG of the level size with caching headers.
func TestHandleGeodataTile(t *testing.T) {
	server := newGeodataTestServer(t)

	for _, tc := range []struct {
		level string
		size  int
	}{
		{level: "0", size: 2048},
		{level: "2", size: 512},
		{level: "4", size: 128},
	} {
		url := "/api/geodata/tile/" + tc.level + "/22_22.png?mode=height"
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusOK, recorder.Code, tc.level)
		require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
		require.NotEmpty(t, recorder.Header().Get("Cache-Control"))

		img, err := png.Decode(recorder.Body)
		require.NoError(t, err, tc.level)
		require.Equal(t, tc.size, img.Bounds().Dx(), tc.level)
		require.Equal(t, tc.size, img.Bounds().Dy(), tc.level)
	}
}

// TestHandleGeodataTileErrors checks the bad request and not found
// paths of the tile endpoint.
func TestHandleGeodataTileErrors(t *testing.T) {
	server := newGeodataTestServer(t)
	handler := server.httpServer.Handler

	cases := []struct {
		url  string
		code int
	}{
		{url: "/api/geodata/tile/9/22_22.png", code: http.StatusBadRequest},
		{url: "/api/geodata/tile/x/22_22.png", code: http.StatusBadRequest},
		{url: "/api/geodata/tile/0/22_22.jpg", code: http.StatusBadRequest},
		{url: "/api/geodata/tile/0/22_22", code: http.StatusBadRequest},
		{url: "/api/geodata/tile/0/xx_yy.png", code: http.StatusBadRequest},
		{url: "/api/geodata/tile/0/30_30.png", code: http.StatusNotFound},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, tc.url, nil))
		require.Equal(t, tc.code, recorder.Code, tc.url)
	}
}

// TestHandleGeodataTileModes checks that the mode parameter changes the
// rendered tile (the walls view paints walled cells red).
func TestHandleGeodataTileModes(t *testing.T) {
	server := newGeodataTestServer(t)
	handler := server.httpServer.Handler

	fetch := func(mode string) image.Image {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
			"/api/geodata/tile/4/22_22.png?mode="+mode, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		img, err := png.Decode(recorder.Body)
		require.NoError(t, err)

		return img
	}
	height := fetch("height")
	walls := fetch("walls")
	require.NotEqual(t, height.At(0, 0), walls.At(0, 0))
}
