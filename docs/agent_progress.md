# Agent progress log

Crash-safe task tracking: the current task, its full context and
per-commit progress live here (see the "Work protocol" section in
AGENTS.md). Entries are append-only; a new agent resumes the newest
unfinished entry.

This file carries ONLY the active and the most recent context:
finished task entries and older progress streams move to
`agent_progress_archive.md` (append-only, same order). The permanent
root-cause history of every round lives in `docs/development_log.md`;
check the archive when the recent context references an older task.

## Active task: the recastnavigation port - the production Detour runtime and the Go mesh builder (2026-09-15)

Started: 2026-09-15. Branch: `feature/new-pathfind` (on the research
tip cf000f5). Commits as melg8. Other agents may push to the same
branch concurrently - rebase before every push.

### Goal

The owner approved the migration path of
`docs/recast_pathfinding.md` ("Daju dobro, portiruj"): port the
Detour RUNTIME into production Go and build the navigation mesh in
Go from the geodata with the sheet decomposition - the grid engine
stays as the click validation and local walk layer. The concrete
pieces from the research verdict:

1. the offline tile builder (the sheet decomposition -> polygon
   merge -> NSWE-filtered links -> BVTree -> tile file, one cmd),
2. the multi region tile loading (lazy, ~1.45 ms per tile),
3. the funnel string pulling of findStraightPath,
4. the NSWE link filter (improved: portal spans keep the crossing
   inside the open cell pairs of every shared edge),
5. the pooled query allocations.

### Progress

- `pathfind.ParseRegionData` + `Region.LayerStack` (the exported
  region access the builder walks without the engine cache, with the
  reusable stack buffer) - commit 25a463d.
- the research prototype isolated as
  `pathfind/navmesh/prototype` (unchanged behavior, its own suite) -
  commit 1ee48e7.
- the production runtime `pathfind/navmesh`: the native rectangle
  tile format (cell bounds, four exact corner heights, Detour-style
  link chains with the NSWE portal spans, the quantized BVTree; the
  full encode/decode roundtrip and the corruption guards), the Mesh
  (lazy multi region tile loading with the LRU), the A* corridor
  search (pooled state, the area-cost water pricing, the partial
  closest-reachable answer), the funnel string pulling (the
  findStraightPath port with the portal span restriction) and the
  Route/RouteDry/WaterEscape facades - commit c5c32b9. 17 unit tests
  over the synthetic corridor world (the stacked disambiguation, the
  dry partial, the escape, the portal span restriction, the cross
  region routes, the LRU).
- the Go mesh builder `pathfind/navbuild`: the geodata flatten +
  dedup, the sheet decomposition (the top-down flood of the 2D
  manifolds with the one-layer-per-column rule and the island
  filter), the maximal rectangle decomposition with the
  height-variation-aligned splits bounded by the 24 unit bilinear
  tolerance, the NSWE portal span link walk (the wallsOpen rule) and
  the BVTree port - commit d3bb9ac. The synthetic world suite plus
  the real region suite: 21_19 builds in 1.7 s into 91 453 polygons
  (7 045 water) with 5 778 sheets / 4 303 islands - the SAME sheet
  numbers the C++ research experiment produced - zero one-way links,
  the hard bridge pair answers (the full swim corridor 171 polys,
  the dry partial, the reverse escape), the 200 pair replay classifies
  133 full + 59 partial + 8 isolated (the C++ "200/200" counted the
  partials as success through dtStatusSucceed - the honest split is
  documented in docs/navmesh.md).
- `cmd/navmesh-build` + `navbuild.BuildPack`: the two-phase pack
  build with flat memory (phase A writes the tiles immediately and
  retains value-copied border strips - the field pointer would pin
  the whole 200 MB build, the first version OOMed at 2.4 GB; phase B
  re-decodes only the stitched tiles) - commit 2117ef0. The full
  165-region pack builds in 4m25s at 604 MB peak RSS, 1.9 GB of
  tiles, 193 784 external links. Seven pack regions fail the l2j
  parse (16_10, 17_20..17_25) - pre-existing corrupt files the grid
  engine parser rejects identically, documented in docs/navmesh.md.
- `docs/navmesh.md`: the subsystem reference (the tile format, the
  build pipeline, the measured comparison table, the corrupt-region
  note, the not-wired-yet scope).

### Next

The live integration round: the hunt loop consuming the navmesh
corridors while the grid engine stays the click validation of every
smoothed leg (on the acceptance stack of feature/proxy-server), then
the runtime optimization headroom (the flat tile array, the per-tile
poly index) if the fleet benchmark asks for it.
