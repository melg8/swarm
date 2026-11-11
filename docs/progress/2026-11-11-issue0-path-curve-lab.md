# Smooth curve animation lab (status: in progress)

Started 2026-11-11, branch feature/path-viz-gallery, issue none.
The follow-up of `docs/progress/2026-11-11-issue0-path-viz-gallery.md`:
the owner picked the smooth curve idea (variant 11 of the first
gallery) and asked for 30 more variants based on it, animations
first, then colors and the drawing approach.

## Goal

A second comparison page (`-test-path-ui-v2`, mode `test-path-v2`)
with 30 numbered variants that all draw the same Catmull-Rom walk and
animate it. The owner picks the motion first; the color and the
stroke approach are the second and third axes.

Acceptance criteria:

- every variant uses the smooth curve as its base (no polyline-only
  cards) and the same route, stop dwells, walker and clock as the
  first gallery, so the animations compare one to one;
- the 30 ideas spread over three groups: motion (comet, dash drift,
  draw-on, scan band, pulse, beads, fireflies, sparks, meteors,
  ripples, heartbeat, wave, stop ping), color (hue ramp, temperature
  split, rainbow flow, split duotone) and stroke approach (tapered
  ribbon, double strand, emboss, neon tube, elastic band);
- clicking a head marks the card and the toolbar chip lists the
  picked numbers;
- `tools/repro_path_curve.js` gates the structure, every variant's
  engagement and animation, the curve timeline (the curve passes
  through the stops), the map tile, the walker, the scroll window
  and the picks;
- the first gallery stays untouched on its own mode/flag;
- docs (`docs/webui.md`), the `webui-harness` skill and the AGENTS.md
  mode list name the new page.

## Context

- The first page is `web/pathtest.js` (mode `test-path`), landed on
  this branch: 20 variants, the shared 7 point route with 5 stops.
- The smooth curve is the Catmull-Rom resampling of the route
  (`PATH_SMOOTH` in that file), 16 samples per segment.
- The map background is the open meadow crop of `maps/0/21_19.jpg`
  (world center 43328/43648, 1900 units wide).
- All 30 variants must share one animation timeline: travel 4200 ms,
  240 ms dwell at every stop, 900 ms arrival hold, 300 ms fade.
- Classic scripts share the global scope: every new top level name
  is prefixed `curve`/`CURVE` to avoid a `const` collision with the
  first gallery.

## Progress

### 2026-11-11 - start the round

Recorded the brief and the naming constraints, started the page.

### 2026-11-11 - implement the curve lab

- `web/pathcurve.js`: thirty numbered variants on the Catmull-Rom
  curve - 01-21 motion, 22-25 color, 26-30 stroke approach. The walker
  rides the curve itself (not the polyline), dwells at the same five
  stops and shares the same clock and palette as the first gallery.
- The server side: `pathcurve.go` (mode `test-path-v2`), the
  `-test-path-ui-v2` flag and `runTestPathV2UI`, the `curve-section`
  markup, the `main.js` dispatch and the `.curve-*` CSS block.
- The tests: `pathgallery_test.go` now holds the shared config and
  boot-chain assertions, the two gallery test files pass their mode
  data; `pathcurve_test.go` covers the new mode.
- `tools/repro_path_curve.js`: ALL PASS (30 cards, 21/4/5 group split,
  the curve through every stop, the off-polyline bow, every variant's
  engagement and animation, the curve timeline, the tile, the walker,
  the scroll window and the picks).
- All eleven Node harnesses pass, `go test ./internal/swarm/webserver/`
  passes, `task lint:new` is 0 issues, `task fmt:check` is clean.
- The rebuilt gallery binary serves the lab at
  `http://127.0.0.1:8097/`; the headless screenshots confirm all 30
  cards render.
- Docs updated: `docs/webui.md` (mode 6 and the harness section), the
  `webui-harness` skill (eleven harnesses) and the AGENTS.md botless
  flag list.

## Status

in progress: the page is live and verified; waiting for the owner to
pick the animation numbers (or a combination) from 01-30.
