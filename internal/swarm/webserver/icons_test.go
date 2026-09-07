// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// newIconsTestServer builds a server with the icon pack pointed at a
// temporary directory that holds one tiny PNG.
func newIconsTestServer(t *testing.T, dir string) *Server {
	t.Helper()

	registry := state.NewRegistry()
	server := NewServer(registry, "127.0.0.1:0",
		log.New(os.Stderr, "test: ", 0))
	t.Setenv("SWARM_ICONS", dir)
	server.initIconsDir(log.New(os.Stderr, "test: ", 0))

	return server
}

// TestIconServed verifies the icon route: an existing icon file is
// served as an image with cache headers, the .png suffix of the URL
// maps onto the extension-less pack names.
func TestIconServed(t *testing.T) {
	dir := t.TempDir()
	// A minimal valid PNG (1x1 transparent) is enough for the file
	// server; the handler only stats the file.
	png := []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
		0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "weapon_small_sword_i00.png"), png, 0o600))

	server := newIconsTestServer(t, dir)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/icons/weapon_small_sword_i00.png", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t,
		"image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, iconsCacheMaxAge,
		recorder.Header().Get("Cache-Control"))
	require.True(t, bytes.Equal(png, recorder.Body.Bytes()))
}

// TestIconNotFound covers the refusals: unknown names, path escapes
// and missing packs must not leak the file system.
func TestIconNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "armor_t02_u_i00.png"),
		[]byte{0x89, 'P', 'N', 'G'}, 0o600))
	server := newIconsTestServer(t, dir)

	for _, name := range []string{
		"/icons/no_such_icon_i00.png",
		"/icons/..%2Fsecret.png",
		"/icons/weapon_small_sword_i00.txt",
		"/icons/weapon.name.png",
	} {
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodGet, name, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code, "url: "+name)
	}
}

// TestIconDisabledWithoutPack verifies the empty pack behavior: every
// icon request answers 404 so the widget falls back to its glyphs.
func TestIconDisabledWithoutPack(t *testing.T) {
	server := newIconsTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/icons/armor_t02_u_i00.png", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

// chdir switches the working directory for the duration of the test.
// A manual os.Chdir instead of testing.T.Chdir: the module builds with
// the go 1.23 language level where T.Chdir is not available yet, and
// the test must keep compiling on 1.23 and 1.24 toolchains alike.
func chdir(t *testing.T, dir string) {
	t.Helper()

	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore working directory %q: %v", orig, err)
		}
	})
}

// TestDetectIconsDirWalkUp verifies the candidate walk: the pack is
// found in a parent of the working directory, not only next to it.
func TestDetectIconsDirWalkUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	pack := filepath.Join(root, "data", "icons")
	require.NoError(t, os.MkdirAll(pack, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pack, "etc_adena_i00.png"),
		[]byte{0x89, 'P', 'N', 'G'}, 0o600))

	chdir(t, nested)

	require.Equal(t, pack, detectIconsDir())
}
