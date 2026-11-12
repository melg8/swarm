// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "io"
    "log"
    "testing"
)

// newPathGalleryServer builds the walk plan gallery server.
func newPathGalleryServer() *Server {
    return NewTestPathServer("127.0.0.1:0",
        log.New(io.Discard, "", 0))
}

// TestPathGallery covers the walk plan gallery mode: the config
// endpoint, the gallery section markup, the script and the mode
// dispatch down to the shared map tile.
func TestPathGallery(t *testing.T) {
    assertGallery(t, galleryCase{
        server:     newPathGalleryServer(),
        mode:       modeTestPath,
        section:    `id="path-section"`,
        grid:       `id="path-grid"`,
        scroll:     `id="path-scroll"`,
        pause:      `id="path-pause"`,
        picked:     `id="path-picked"`,
        scriptFile: "pathtest.js",
        scriptPath: "/pathtest.js",
        scriptMarkers: []string{
            "const PATH_VARIANTS = [",
            "const PathTest = {",
        },
        modeExpr: `config.mode === "test-path"`,
        initCall: "PathTest.init()",
    })
}
