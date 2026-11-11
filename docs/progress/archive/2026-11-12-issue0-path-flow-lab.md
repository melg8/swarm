# Flow and snake speed lab (status: complete)

Started 2026-11-12, branch feature/path-viz-gallery, issue none.
The follow-up of `docs/progress/2026-11-11-issue0-path-curve-lab.md`:
the owner picked the rainbow flow and the pulse snake ideas and asked
for 40 more variants (20 each) in different color schemes and at
different movement speeds. The rainbow flow must travel from the
start to the destination, not the opposite way.

## Goal

A third comparison page (`-test-path-ui-v3`, mode `test-path-v3`)
with 40 numbered cards on the same Catmull-Rom walk: 01-20 the
rainbow flow (five palettes x four speeds) and 21-40 the pulse snake
(five crest schemes x four wave periods). The flow bands move toward
the destination; the snake crests travel the same way.

Acceptance criteria:

- every flow variant moves its hue bands from the planning origin to
  the destination, and the four speed classes differ;
- every snake variant moves its width crest from the origin to the
  destination, and the four wave periods differ;
- the v2 lab's own `rainbow flow` variant (#24) is fixed to the same
  forward direction;
- the 40 cards share the route, the stop dwells, the walker and the
  clock of the first two labs, so the variants compare one to one;
- `tools/repro_path_flow.js` gates the structure (20/20 split, five
  palettes and four speeds per family), the forward direction, every
  variant's engagement and animation, the curve timeline, the tile,
  the walker, the scroll window and the picks;
- `tools/repro_path_curve.js` gains the forward-direction check for
  the fixed v2 rainbow flow;
- docs (`docs/webui.md`), the `webui-harness` skill and the AGENTS.md
  mode list name the new page.

## Context

- The base page is `web/pathcurve.js` (mode `test-path-v2`): 30
  variants on the Catmull-Rom curve, the same 7 point route with five
  stops and the same walk budget.
- The flow direction is a pure phase function: a hue at position `k`
  at time `t` equals `(k * bands - t / period) mod 1`, so a fixed hue
  moves to a larger `k` as the clock runs.
- The snake wave keeps the v2 shape `sin(dist / wavelength - t /
  period)`, which already travels toward the destination.
- Global scope is shared by classic scripts: every new top level name
  is prefixed `flow`/`FLOW`; the engine is `PathFlow`.

## Progress

### 2026-11-12 - start the round

Recorded the brief and the direction requirement, started the page.

### 2026-11-12 - the page, the harness and the v2 fix

- `web/pathflow.js`: the 40 card lab on the shared Catmull-Rom route.
  The flow cards color 36 curve segments from
  `flowColorAt(k, t, palette, period)` (the phase subtracts the clock,
  so a fixed band moves toward the destination); the snake cards pulse
  the width and the crest color with `flowWave`. Five palettes x four
  periods each, five crest schemes x four periods each.
- The v2 `rainbow flow` (#24) now colors through `curveRainbowHue`,
  the same forward formula; `tools/repro_path_curve.js` checks it.
- Server: `pathflow.go` (mode `test-path-v3`), the `-test-path-ui-v3`
  flag, `runTestPathV3UI`, the `flow-section` markup, the `main.js`
  dispatch, the `.flow-*` CSS and the new `pathflow_test.go`.
- `tools/repro_path_flow.js`: ALL PASS (40 cards, 20/20 split, five
  schemes x four speeds per family, forward flow/snake direction,
  every variant engaged and animated, the curve timeline, the tile,
  the walker, the scroll window at 8/8 cards, the pick controls).
- All twelve Node harnesses pass; `go test
  ./internal/swarm/webserver/`, `task lint:new` (0 issues) and
  `task fmt:check` are clean.
- Live: v3 on `127.0.0.1:8098` (`/api/config` reports
  `test-path-v3`), v2 restarted on `127.0.0.1:8097` from the same
  build; the screenshots are `runs/shots/pathflow-top.png`,
  `runs/shots/pathflow-all.png` and `runs/shots/pathcurve-new.png`.
- Docs updated: `docs/webui.md` (mode 7 and the harness entry), the
  `webui-harness` skill (twelve harnesses) and the AGENTS.md botless
  mode list.

### 2026-11-12 - variant 12 into the live map

- `web/map.js`: the walk plan line now draws as the pastel flow of
  variant 12 - `walkFlowHue` runs the lab phase
  (`frac(fraction * 2 - now / 2800)`) over the hue range
  `190 + u * 260` at 58% saturation and 78% lightness, and
  `drawWalkFlow` paints 10 px screen steps over a dark casing
  (`rgba(27, 36, 48, 0.35)`, 4.5 px) so the light map imagery keeps
  the line readable. The waypoint dots, the aimed waypoint ring, the
  hover coordinate label and the destination marker keep the magenta
  (`mapColors.userPath`).
- `tools/repro_map_render.js`: the `walk flow` scenario pins the dark
  casing, the pastel steps, the retired `[7, 4]` dashed magenta line,
  the kept magenta dots, the forward band direction (`hueAt(0.2, 0)`
  equals `hueAt(0.325, 700)`), the repaint under `advanceClock(700)`
  and `needsMoreFrames()`; ALL PASS.
- All twelve Node harnesses and `go test
  ./internal/swarm/webserver/ ./cmd/swarm/` are green.
- `docs/webui.md`: the walk path view and the map harness entry name
  the flow, the marker reads magenta instead of the stale blue.

### 2026-11-12 - the bead dots and the aim pulse

- `web/map.js`: the waypoint dots lose the white outline and take the
  flow hue of their own route fraction (`hsl(hue, 72%, 66%)`, a step
  deeper than the ribbon's 58/78), so every dot reads as a bead of the
  line palette; the aimed waypoint (`walkIndex`) pulses a soft ring
  (radius 5.5 to 9 px, fading as it swells) instead of the old white
  target ring.
- `tools/repro_map_render.js`: the `walk flow` scenario pins the
  fraction hues, the retired white outlines, the magenta destination
  marker, the ring hue and its in/out radius change; ALL PASS.
- `docs/webui.md`: the walk path view and the harness entry describe
  the bead dots and the aim pulse.

## Status

complete: the owner picked the pastel flow (variant 12, 2.8 s); it is
the live walk plan line of map.js with the bead waypoint dots and the
aim ring, committed on `feature/path-viz-gallery` and merged into
main. The lab mode and its harness stay in the tree as the reference
page.

