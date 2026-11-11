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

### 2026-11-11 - the owner picked V01, the skull ships as a sprite

- The owner chose V01 (the plain skull, `kkShadow` + `kkSkull` at
  k=1).
- `map.js`: `paintKillMark` ports the V01 geometry verbatim into the
  unit box; `killMarkSprite` renders it once per device pixel ratio
  into a canvas sized `ceil(2 * killMarkSize * dpr)`;
  `drawKillMarks` blits the sprite per visible mark at the constant
  alpha 0.85; `drawKillMarkFlat` keeps the gray disc and X eyes as
  the fallback of a document without a 2d canvas. The sprite is
  cached in `killSprite`/`killSpriteDpr` and rebuilt only when the
  ratio changes. The stale "the kill skulls fade with age" comment
  on the frame cache is corrected: the ring changes with every poll.
- The port is pixel identical to the gallery V01 (a headless Chrome
  comparison of the two sprites over a real map tile reports the same
  bytes).
- `tools/repro_zone_hover.js`: the recording context grew the
  ellipse/curve/clip/gradient surface the art needs and a
  `drawImage` record; the kill scenario now asserts one 20 px blit
  per mark at alpha 0.85 for every age, the sprite art (a linear
  gradient bone fill, three ink fills, the ink strokes), the sprite
  device size, no per frame repaint, the toggle, and the flat face
  fallback of a canvasless document. The context records fills and
  strokes of the same path like the canvas does.
- `docs/webui.md`: the corpse marker paragraph, the fleet kill marks
  bullet and the cache/paint-cost note read the skull sprite now.

### 2026-11-11 - the mark takes the mob size and follows the zoom

- Owner feedback: the skulls were too small and ignored the map zoom.
- `map.js`: `killMarkRadius()` sizes a mark from the mob marker of the
  kill - the skull bone spans `radiusOf` of an in combat npc (radius
  6) times `killMarkSizeFactor` (2, the thin bone art reads lighter
  than the solid circle) times the shared `unitScale` - so the marks
  grow and shrink with the camera exactly like the mobs.
  `unitScaleMin`/`unitScaleMax` became named constants
  (computeUnitScale clamps with them), and `killMarkMaxRadius` derives
  the sprite resolution from the cap.
- `killMarkSprite` renders at the largest blit of the zoom range, so
  the zoom never rebuilds the raster (a pixel epsilon keeps the exact
  integer size from rounding up). `killMarkAt` picks within the
  larger of the fixed 13 px and the drawn radius.
- `tools/repro_zone_hover.js`: the scenario measures the live in
  combat mob circle of the same frame and asserts the skull bone
  equals its radius, at the default zoom and at the zoom cap.
- `docs/webui.md`: the size rule and the sprite resolution note.

## Status

V01 ships in the map and awaits the owner's live verdict (keep it or
switch to another variant of the gallery). The harnesses pass, the
port is pixel identical to the gallery art, the mark is the mob size
and follows the zoom.
