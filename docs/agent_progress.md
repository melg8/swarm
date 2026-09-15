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
- The live integration round opened. The corridor search gained the
  two contracts the Navigator seam needs: the approach radius goal
  (the FindPathApproach stop condition - the first polygon whose
  closest surface point lies within the 3D radius of the end, the
  polygon-granularity form of the grid nodeReached) and the avoid
  areas (the recovery bans: Filter.Avoid carries AvoidCircle disks,
  a polygon whose footprint a ban touches walls the search - the
  over-walling direction, so no funnelled leg ever enters the banned
  ground; the ban holding the start opens its escape ring within 256
  units at the 6x multiplier, the foreign ban wins over the escape
  ring - the rectangle granularity port of the grid
  cellAvoidedEscape rules). `RouteApproach` is the new facade entry,
  `Route` degenerates to it at radius zero. Six tests over a fresh
  two-lane synthetic world pin the semantics: the early stop, the
  start-poly radius, the lane detour with waypoint-outside-ban, the
  sealed goal, the escape way-out and the foreign ban precedence.
- `hunt.NewNavmeshNavigator` - the hybrid behind the Navigator seam:
  the five route queries (FindPathApproach, the avoiding/dry
  avoiding forms with the bans converted into mesh disks, FindPath,
  FindWaterEscape) serve from the mesh corridor search and fall back
  to the grid engine on EVERY answer the mesh cannot serve (a
  missing tile, a dropped island sheet, a sealed goal, a partial
  closest-reachable corridor - round one keeps the grid engine the
  reachability authority, so the hybrid can only add routes, never
  lose them); the validation layer (ValidateClick, the sight lines,
  the water rasters, the deck heights) never leaves the engine. Five
  tests pin the seam: the synthetic corridor route + escape served
  from the mesh with an engine-less fallback proving the mesh
  answered, the no-mesh fallback surfacing the engine error, the ban
  flow sealing the mesh route, the REAL hard pair (the elven village
  deck to the water under the bridge, 21_19 built straight from the
  shipped geodata through navbuild.BuildRegion - the 5.17 s grid
  walk answers from the mesh in one call) and the validation answers
  identical through the hybrid and the engine on the real pack.
- The runtime wiring landed (cmd/swarm/main.go): the -navmesh flag
  names the tile directory explicitly, the empty value autodetects
  data/navmesh (the build command's documented output), and the mesh
  installs `hunt.NewNavmeshNavigator` behind SetNavigator for every
  bot of the fleet - a directory without tiles keeps the plain
  engine navigator silently. data/navmesh joined .gitignore (the
  tiles are derived from the tracked geodata, 1.9 GB for the full
  pack, rebuildable in ~4.5 min). The smoke run against the four
  elven region tiles (21_19, 22_19, 21_20, 22_20 - 62 MB, 8 s build)
  logs the readiness line in both the autodetect and the explicit
  flag mode. docs/navmesh.md gained the live integration section
  (the seam, the fallback rule, the ban granularity note) and the
  honest not-wired-yet residue (the acceptance stack stays on the
  pure engine navigator, the partial waypoints stay unexposed);
  AGENTS.md and docs/hunting.md follow the wiring.
- The partial round landed. `pathfind.Result.Partial` extends the
  seam contract (Found=false with Partial set and waypoints ending
  at the closest reachable point; the grid engine itself never sets
  it - its searches answer the bare not found). The hybrid's two
  avoiding forms (FindPathApproachAvoiding,
  FindPathApproachDryAvoiding) serve the mesh partial corridors
  through it: the engine run comes FIRST on every mesh partial (a
  full engine route the mesh missed still wins, an engine error
  still surfaces - the can-only-add rule holds), and only after the
  engine's own clean not found does meshPartial serve the mesh funnel
  waypoints (Aborted carried from the engine verdict, Explored the
  sum of both searches). The strict forms (FindPathApproach,
  FindPath) never surface partials - the blind engage recovery and
  the user walks keep their verdict semantics. The consumers:
  startWalkLegSearch arms the follower on the partial waypoints (the
  town legs and zone returns walk toward the closest reachable point
  instead of aborting; the bare not found still refuses to plan),
  followPlannedSegment walks the partial quest segments (the
  re-plan loop and the no-progress guard stay the safety net). Four
  hybrid tests (the synthetic shore world with the engine geodata
  built as flat blocks in the hunt tests: the partial served after
  the engine confirmation, the engine route winning over the mesh
  partial of a broken chain, the engine error surfacing without
  geodata; the REAL hard pair dry - the elven village deck to the
  water under the bridge answers the dry partial with every waypoint
  above the water level) plus three consumer pins (the town leg
  arming on the partial, the quest segment walking it through the
  moving game simulator, the bare not found still aborting).

- 2026-09-15: the mesh viewer round landed (the owner request: an
  interactive 3D viewer over the built meshes, a double click pair
  builds a route with a construction timer, launched by a
  `-show-navmesh` flag, loading either one named mesh or all meshes
  of the directory stitched together). `webserver.NewNavmeshServer`
  adds the fourth bot less mode behind the embedded web interface:
  `GET /api/navmesh/tiles` (the listing with the derived world
  footprints, no decode - through the new `Mesh.TileFiles`), `GET
  /api/navmesh/geometry/{col}_{row}` (the binary NMV1 payload: a 24
  byte header, the contiguous int16 corner block, the area tail;
  ~25 B per polygon, ETag revalidation, a process lifetime cache)
  and `POST /api/navmesh/path` (the measured `Route` call with the
  swim/dry filter select of the hunt loop's two search profiles).
  The page is `web/navmesh_view.js` over the vendored three.js r160
  module build (`web/vendor/`, offline like the rest of the
  interface): the polygon quads tessellate client side with the
  height ramp terrain colors and the flat water blue, a minimal
  orbit rig (drag rotate, wheel dolly, right drag pan), the double
  click raycast arms the start then asks the route, and the result
  panel carries the status badge, the construction timer
  (microsecond resolution under a millisecond), the waypoint count,
  the corridor/explored poly counts and the path length. The
  `-show-navmesh` flag is a custom `flag.Value` with `IsBoolFlag`:
  the bare form opens every tile stitched, `-show-navmesh=21_19`
  (comma separated) opens the named tiles, the route queries always
  run over the full directory mesh. Verified live in a headless
  browser over the four elven tiles: the REAL hard pair (village
  deck -> water under the bridge) draws its 49 waypoint funnel with
  a 8.8 ms timer against the grid engine's 5.17 s flood; the VLM
  review of the screenshots confirms the watertight terrain, the
  cliff geometry and the marker/path rendering. Five webserver API
  tests (config/tiles/geometry binary layout/etag 304/path with the
  swim found and dry partial answers) plus the flag parse pin; the
  lint stays at the pre-existing 68 findings, the full suite green.

### Next

The viewer round closes the tooling side of the port. The
follow-up candidates (NOT started): the acceptance stack switch to
the hybrid once the live sessions prove it (which may also skip
the engine confirmation flood on mesh partials - the ~13 s the real
hard pair dry search pays today), the Detour-parity optimization if
the fleet benchmark asks for it.
