# The death mark variant gallery: one page, 35 candidates (status: in progress)

Started 2026-11-10, branch main, issue0 (owner instruction: the flat
gray corpse disc of the map is dull, paint many candidate death marks
in situ at the real map scale and let the owner pick one by code).

## Goal

A single offline HTML page that lets the owner choose the death mark
that replaces the current corpse face of `drawKillMarks` in
`internal/swarm/webserver/web/map.js`. Done means:

- every candidate is visible alone and in a camp, at the real marker
  size (10 px radius, alpha 0.85) over the real committed map tiles;
- the art is varied (skulls, graves, bones, the fallen, omens, magic),
  not 35 recolors of one shape;
- the page runs from the repository checkout without network access,
  a build step or a dependency;
- the owner picks one code and the winner is small enough to port as a
  pre-rendered sprite into `map.js`.

## Context

- The current mark: `drawKillMarks` (map.js, the `show-kills` layer)
  paints one shared path of gray discs `#80868b` with two X eyes
  `#39424e`, radius 10, alpha 0.85, fed by `/api/fleet/kills`. The
  marks are permanent session statistics, not fading effects.
- The gallery lives in `docs/design/`, not under the embedded
  `internal/swarm/webserver/web/` tree: it is a throwaway design page
  and must not ship inside the bot binary.
- Terrain reuses `internal/swarm/webserver/web/maps/<level>/<bx>_<by>`
  through the same level rule as `drawMapBackground`: the finest
  pyramid level whose native resolution stays within a factor of two
  of the drawn tile size.
- Art convention: one draw function per candidate, painted in a unit
  box (one unit = marker radius, origin = mark centre), so the same
  function renders at the 10 px map scale and at the x7 detail zoom.
  A production port pre-renders the winner once into a sprite.
- Variants V00..V35: V00 is the current corpse face (the baseline),
  the rest are grouped by family in `KILL_FAMILIES`.

## Progress

### 2026-11-10 - the gallery, the art library and the terrain

- `docs/design/killmarks_art.js`: the palette, the shared helpers
  (skull, bones, grass, gradients) and 36 draw functions; the file
  passes `node --check`.
- `docs/design/killmarks.html`: the controls (terrain, zoom, mark
  size, kills in view, jitter, animation), the hero scene, the family
  sections, the per-card solo/pack panes and the dark/paper/terrain
  readability plates, the `?family`, `?ids`, `?hero=0`, `?sel=`,
  `?zoom=`, `?kills=`, `?terrain=` and `?sheet=1` review modes.
- The terrain switched from a pre-stitched 3x3 block to direct
  per-tile drawing with a debounced repaint on tile arrival, so the
  zoom slider reaches the map default 0.12 without running past the
  stitched canvas; the cameras sit inside the covered tile rectangle.
- The layout is in world units: one map tile is 1024 gallery units, a
  camp spreads over ~150 units, strays reach the tile corners.

## Status

The page and the art library are written and reviewed through headless
Chrome screenshots. Next: hand the owner the variant list and take the
pick, then port the chosen draw function into `map.js` as a sprite.
