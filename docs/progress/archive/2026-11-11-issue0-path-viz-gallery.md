# Walk plan visualization gallery (status: in progress)

Started 2026-11-11, branch feature/path-viz-gallery, issue none.

## Goal

The owner reads the current walk plan drawing of the live map as
boring: static (no animation), the stop points are too large and the
line does not carry the direction or the progress of the walk. Build a
separate comparison page with 20 numbered walk plan visualizations,
all animating the same route on the same map tile with one shared
clock, so the owner picks the best variant or a combination.

Acceptance criteria:

- the page is its own server mode (`-test-path-ui`, mode `test-path`)
  that needs no game connection, exactly like the fight FX gallery;
- 20 variants, every one animated (the paint changes between two
  moments of the loop), numbered and described;
- one shared route: 5 stops plus the destination, the walker dwells at
  every stop, so the stop marker and the line style are comparable;
- the stop markers stay small (the complaint: the current 3.5 px disc
  plus a 2 px white ring draws as a fat blob at the live zoom);
- clicking a variant marks it and the toolbar lists the picked
  numbers, so a combination is easy to name;
- the Node harness `tools/repro_path_gallery.js` gates the structure,
  every variant's engagement and animation, the shared timeline,
  the map tile background and the controls;
- `docs/webui.md` and the `webui-harness` skill list the new mode and
  harness.

## Context

- The live path drawing is `drawWalkPlan` in `web/map.js`: a magenta
  dashed polyline, a 3.5 px disc per waypoint and the pulsing
  destination marker.
- The gallery follows the `-test-fight-ui` pattern:
  `webserver.NewTestFightServer`, `tools/repro_fight_ui.js`, the
  `body.mode-test-fight` CSS switch.
- The map background tile is `maps/0/21_19.jpg` (the starter meadow),
  world origin 32768/32768, tile center 46112/41500.
- The Mobius stack runs on this host (2106, 7777, 3306 listening);
  the gallery itself needs none of it.

## Progress

### 2026-11-11 - start the round

Created the branch, recorded the brief and the verified context.

### 2026-11-11 - the gallery page

Landed the `-test-path-ui` mode: `webserver.NewTestPathServer`
(mode `test-path`), the CLI flag, the `path-section` markup, the
`mode-test-path` CSS switch and `web/pathtest.js` with 20 numbered
variants. Every card walks the same seven point route (origin,
5 stops, destination) with one shared walker and clock: the walker
dwells 240 ms at every stop, holds 900 ms at the destination and the
loop restarts after a 300 ms fade. The variants separate the line
axis (dash flow, comet, chevrons, beads, casing, road median,
railway, energy beam...), the stop axis (2-3 px specks, ticks,
diamonds, numbers, rings) and the destination axis (ring, pin,
diamond, bullseye). Clicking a head marks the card and the toolbar
chip lists the picked numbers.

The map background moved to the open meadow crop (tile pixel
330/340, world 43328/43648) after a screenshot review: the starter
meadow center of the fight gallery is dark forest there and buried
the thin lines.

Verified: `go build ./...`, `go test ./internal/swarm/webserver/`,
`task lint:new`, `task fmt:check`, all ten Node harnesses, and a
headless Chrome screenshot of the running page.

### 2026-11-11 - the harness and the docs

Added `tools/repro_path_gallery.js`: structure (20 cards, numbering,
4 column template), per variant engagement against the engine
baseline at four loop moments, per variant animation (the recorded
paint trace at two moments must differ), the shared timeline (origin,
stop dwells, arrival, passed stop count, loop wrap), the meadow tile
background of every card, the shared walker, the scroll window and
the pick controls. Documented the mode in `docs/webui.md` and the
`webui-harness` skill.

## Status

in progress: the gallery is ready for the owner's pick; the chosen
variant (or combination) becomes the live `drawWalkPlan` of map.js.
