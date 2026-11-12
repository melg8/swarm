// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "io"
    "log"
    "testing"
)

// newPathFlowServer builds the flow and snake speed lab server.
func newPathFlowServer() *Server {
    return NewTestPathFlowServer("127.0.0.1:0",
        log.New(io.Discard, "", 0))
}

// TestPathFlowGallery covers the flow and snake speed lab mode: the
// config endpoint, the flow section markup, the script and the mode
// dispatch down to the shared map tile.
func TestPathFlowGallery(t *testing.T) {
    assertGallery(t, galleryCase{
        server:     newPathFlowServer(),
        mode:       modeTestPathV3,
        section:    `id="flow-section"`,
        grid:       `id="flow-grid"`,
        scroll:     `id="flow-scroll"`,
        pause:      `id="flow-pause"`,
        picked:     `id="flow-picked"`,
        scriptFile: "pathflow.js",
        scriptPath: "/pathflow.js",
        scriptMarkers: []string{
            "const FLOW_VARIANTS = [",
            "const PathFlow = {",
        },
        modeExpr: `config.mode === "test-path-v3"`,
        initCall: "PathFlow.init()",
    })
}
