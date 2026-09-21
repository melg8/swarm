<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Dead mob icon style research (issue #6)

The live map draws a corpse as the dead threat color (`#80868b`) circle
with the look direction tick at 0.45 alpha - `drawUnitTick` of
`internal/swarm/webserver/web/map.js` with `threatOf() === "dead"`.
The result reads as "a faded alive mob": the marker keeps the exact
geometry of a living unit, only washed out, so a kill pile under the
character looks like a smudge of half transparent mobs instead of a
readable "these are dead" state.

This research page answers the issue #6 brief: 30 different icon
ideas, one temporary page showing the same scene (multiple killed
mobs, the character, some alive mobs nearby) with the ability to swap
between the variants.

## Opening the page

The page is a standalone artifact - no server, no build step:

- open `research/dead-mob-icons/index.html` directly in a browser
  (the background falls back to a procedural meadow when the map tile
  is unreachable from `file://`), or
- serve the repository root with any static server
  (`python3 -m http.server` from the repo root, then open
  `/research/dead-mob-icons/`) - this loads the real 21_19 keltir
  meadow tile the bot hunts on, the same crop the fight FX gallery
  uses.

Everything the scene paints mirrors the live map exactly: the
`mapColors` palette, the `drawUnitTick` marker geometry, the label
halo, the fleet kill skull (`traceKillSkull`), the world grid, the
active hunting cell hexagon, the dead-first draw order and the
`computeUnitScale` zoom clamp. A chosen variant ports into
`drawObjects` of `map.js` almost line for line.

## The judgment scene

The fixed scene (world coordinates of the real hunt, character at
46112/41500):

- the character (blue, breathing ring) with its red target link to
  the mob it fights (combat red, dashed "attacking me" ring),
- an aggressive (orange) and two passive (green) mobs, a friendly
  npc (gray), another fleet bot (violet),
- seven corpses: the fresh kill at melee distance, a two-corpse
  cluster, one under the feet of the current target, four scattered,
- two ground item drops (gold diamonds) on the fresh kills,
- three fleet kill skulls in three ages (6 s, 100 s, 240 s) - two of
  them sit exactly on their corpses, the overlap the live map shows,
- the gold hunting cell hexagon with its focus dot and label.

While the variant swaps, everything else stays pixel identical -
that is the point: the icons are judged against the same scene.

## The controls

- variant buttons grouped by family (or `←`/`→` keys, digits `0-9`),
- `g` - the side by side gallery: all 31 icons at 2.5x over the same
  tile crop, clickable,
- `k` - toggle the fleet kill skulls (judge the variant alone and in
  the kill mark company),
- labels / cell / grid toggles and the zoom slider (the marker sizes
  follow the `computeUnitScale` clamp exactly like the live map).

## The variant catalog

Variant 0 is the current live map style - the baseline to beat. The
30 new ideas, grouped in seven families:

| # | name | family | the idea |
| --- | --- | --- | --- |
| 1 | skull-gray | skulls | the fleet kill skull geometry in the corpse gray: one death glyph, neutral color |
| 2 | skull-orange | skulls | the corpse joins the kill mark family: the exact orange skull |
| 3 | skull-bone | skulls | ivory skull, dark outline: the loudest, unmissable zoomed out |
| 4 | skull-in-ring | skulls | hybrid: the current circle footprint with a small skull inside |
| 5 | skull-bones | skulls | the jolly roger: skull over two crossed bones |
| 6 | x-bold | crosses | a bold slate X: the minimal "done" mark |
| 7 | x-in-circle | crosses | X inside a hollow circle: the "closed" sign read |
| 8 | x-soft | crosses | gray X at half alpha: the quietest cross |
| 9 | cross-tilt | crosses | a tilted dried-blood cross: the battlefield grave marker |
| 10 | x-double | crosses | two small X marks: the farming density read |
| 11 | tombstone | graves | a slate slab with the rounded top |
| 12 | tombstone-cross | graves | the slab with the engraved cross |
| 13 | tombstone-tilt | graves | the fallen-over slab, playful |
| 14 | grave-mound | graves | a dark mound of ground with the upright cross |
| 15 | ghost-circle | ghosts | the current marker with the fill removed: a hollow absence |
| 16 | ghost-blob | ghosts | the little bobbing ghost: the friendliest read |
| 17 | dissolve | ghosts | the circle breaking into drifting dots, animated |
| 18 | dashed-ring | ghosts | a dashed outline: "dropped from the world" |
| 19 | smudge | ghosts | a soft radial stain, the quietest option |
| 20 | splat-dark | ground | a dark ink splat on the terrain |
| 21 | splat-blood | ground | the dried blood splat with droplets |
| 22 | ash-pile | ground | the ash pile with rising smoke wisps, animated |
| 23 | puddle | ground | a flat dark puddle: melted into the ground |
| 24 | capsule | fallen | the mob lying flat: a rounded capsule bar on the ground |
| 25 | fallen-figure | fallen | a tiny stick figure lying down, sprawled limbs |
| 26 | x-eyes | fallen | the current gray circle with cartoon X eyes |
| 27 | no-symbol | symbols | the prohibition sign: "out of play" |
| 28 | deflated | symbols | the squashed marker with a drooping tick: it leaked out |
| 29 | broken-swords | symbols | two crossed broken swords: died in combat |
| 30 | soul-wisp | symbols | a faint ground dot and an ivory wisp floating above, animated |

## The self test

The page carries a built-in self test ("run the self test" toggle):
every variant paints on a fresh 120x120 canvas and the test counts
the covered pixels, so a variant that throws or paints nothing fails
loudly. The 2026-09-21 run passed all 31 variants.

## How to pick (the review protocol)

1. Open the gallery (`g`), scan the 31 icons side by side on the
   tile crop.
2. Click the interesting numbers (or flip with `←`/`→` in the scene
   view) and judge them inside the full scene: next to the alive
   mobs, under the labels, over the kill skulls, zoomed out.
3. Toggle the kill skulls (`k`) off and on - a variant that fights
   the orange skull (like 2, which becomes it) or hides behind it is
   a real integration concern, not a taste question.
4. Post the numbers you like to issue #6 - the winner gets ported
   into `drawObjects` of `map.js` as a follow-up task.

## Files

- `index.html` - the page shell (dark chrome, controls, gallery),
- `deadmob.js` - the scene, the 31 variants, the view, the self test,
- `screenshots/` - the captured evidence (the baseline, two
  candidate scenes, the full gallery strip).
