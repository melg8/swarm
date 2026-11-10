# The map void base - the glitchy background without tiles (status: in review)

Started 2026-09-22, branch feature/map-bg-fallback, issue #53.

## Goal

The issue (with the screenshot): disabling the map background or
scrolling to the sides of the map where no tiles exist shows a
strange glitchy background instead of a sane empty view. Done means:
the areas without tiles read as a calm, theme matched backdrop in
every state - the bg toggle, the out-of-tile scroll, the loading
gaps.

## Context

- Issue: https://github.com/melg8/swarm/issues/53 (created 2026-09-22).
- The frame path: draw() composites the OFFSCREEN background cache
  with one blit and never clears the main canvas - by design the
  cache is expected to overwrite every pixel (the tiles normally
  cover the viewport).
- The cache raster started with a clearRect only: every area the
  static layers did not paint stayed TRANSPARENT, and the blit
  composited the PREVIOUS frame's pixels through those holes - the
  glitchy smear (stale units, labels, tile remnants) of the
  screenshot. Two repro paths: the `show-map` toggle (the layer flag
  is in the cache key, the re-raster skips the tiles entirely) and
  the scroll past the tile pack (no tile entries, the draw loop
  continues past them).
- The `--bg-map` CSS variable already existed for both themes
  (#e9edf2 light, #0c0f14 dark) - the canvas void the fix fills in.

## Progress

### 2026-09-22 17:30 UTC - the opaque void base

- `internal/swarm/webserver/web/map.js`: `refreshColors` reads
  `--bg-map` into `this.colors.void` (theme aware - `colorsRev`
  already invalidates the cache on a theme flip); `renderBackground`
  fills the whole cache with the void color right after the
  clearRect, before the tiles, the grid and the zone frame raster.
  The cache blit now overwrites every pixel with either tiles or the
  calm backdrop - no stale frame remnants anywhere.
- `tools/repro_map_render.js`: the background cache scenario pins the
  void base (the first rect of the re-raster covers the full cache
  with `--bg-map`) and the bg-toggle re-render (the void returns on
  the no-tiles raster); the THEME stub gains `--bg-map`.
- All nine web harnesses green; `node --check` clean.

## Status

In review: PR opened with "Fixes #53", CI watched green.
