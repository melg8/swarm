// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "io"
    "log"
    "testing"
)

// newPathCurveServer builds the smooth curve animation lab server.
func newPathCurveServer() *Server {
    return NewTestPathCurveServer("127.0.0.1:0",
        log.New(io.Discard, "", 0))
}

// TestPathCurveGallery covers the smooth curve animation lab mode:
// the config endpoint, the curve section markup, the script and the
// mode dispatch down to the shared map tile.
func TestPathCurveGallery(t *testing.T) {
    assertGallery(t, galleryCase{
        server:     newPathCurveServer(),
        mode:       modeTestPathV2,
        section:    `id="curve-section"`,
        grid:       `id="curve-grid"`,
        scroll:     `id="curve-scroll"`,
        pause:      `id="curve-pause"`,
        picked:     `id="curve-picked"`,
        scriptTag:  `<script src="pathcurve.js"></script>`,
        scriptPath: "/pathcurve.js",
        scriptMarkers: []string{
            "const CURVE_VARIANTS = [",
            "const PathCurve = {",
        },
        modeExpr: `config.mode === "test-path-v2"`,
        initCall: "PathCurve.init()",
    })
}
