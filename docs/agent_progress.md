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

- 2026-09-15: the viewer defect round landed (the owner report: a
  mesh full of black triangles and elements at wrong heights -
  detached fragments). The diagnosis ran through the scratch
  `cmd/navanalyze` (kept as the diagnostic companion): the black
  triangles were the steep cascade quads of the l2j slope smoothing
  cells (a fifth of the polygons) falling to black under the single
  hard sun, the detached fragments were the unmerged duplicate
  layers of the geodata noise - the measured regions carry the same
  surface twice with a 0..32 unit jitter (both layers open, the
  lower copy often wall restricted, ~54k pairs per region of the
  22_xx column; the elven region of the research round showed none,
  which is why the 16 unit rule shipped), and the flat water planes
  sat at heights the riverbed never held. The fixes: the builder
  dedup delta lifts to 32 units (a real stacked floor never sits
  within 32 units of its ceiling - a genuine deck keeps both
  layers, pinned by TestExtractRegionDedupNoise), the light rig
  rebalances to a dominant hemisphere with the sun and the counter
  fill, the logarithmic depth buffer keeps the stitched world from
  z fighting at every viewing distance, and the water renders as
  the submerged terrain it really is - a depth ramp toward the dark
  navy of the deepest riverbeds. The inspection surface the owner
  asked for: the top readout bar names the tile square under the
  cursor with its region local cell and world coordinates (a
  progressive raycast sweep - one tile per frame, the nearest
  bounding sphere first), every loaded tile draws its region grid
  outline with the hovered one lit amber, the result panel carries
  the from/to rows with their tile keys and world coordinates, and
  every route waypoint wears a label with its index and coordinates
  (toggleable, next to the polygon edge overlay). Verified live in
  a headless browser over the 11 tile stitched world: the full pack
  loads (11/11 tiles), the terrain reads clean at every zoom (the
  VLM review of the overview and the route closeups confirms no
  black triangles, no floating fragments, no seams between the
  tiles, the water following the riverbed contours), the cursor
  readout tracks the pointer (tile 22_16, cell 883 883, world
  coordinates verified against the region anchors), and a 129
  waypoint route answers in 79.6 ms with its from/to rows and
  readable waypoint labels.

### Next

- 2026-09-16: the wall-honest rectangle decomposition landed (the
  owner report: the bots walk out of the town through the buildings,
  the viewer routes cross the walls). The root cause: the maximal
  rectangle growth of `navbuild/rect.go` ignored the NSWE walls - a
  rectangle spanned any cells of one sheet, and because the link walk
  skips the cell pairs inside one polygon ("the interior is walkable
  by construction"), a single polygon swallowed whole wall segments:
  the corridor search then funnelled straight through (the audit of
  the shipped pack counted 1.24M swallowed pairs in 21_22, 1.43M in
  22_22, 1.02M in 22_19, 0.41M in 21_19). The fix: `hStepOpen`/
  `vStepOpen` gate every horizontal and vertical cell pair inside
  the growing rectangle (the paired NSWE walls of both sides plus
  the climb height rule - the same `canStep` the grid search walks
  on), `buildRects` takes the climb. The honest decomposition
  multiplies the polygon count (21_19: 91k -> 245k, 22_22: 127k ->
  567k) - the price of routes that respect the walls; the pack must
  be rebuilt (`cmd/navmesh-build`, data/navmesh is gitignored). The
  regression pin: `TestBuildRegionInteriorWall` (a wall segment
  INSIDE the would-be maximal rectangle splits the decomposition -
  the case a border-only wall check misses) and
  `TestBuildRegionInteriorWalls` (the full audit of the real 21_19
  and 21_22: 6.06M + 4.79M same-polygon neighbour pairs, zero walls
  swallowed). The honest side effect: 3 of the 200 research replay
  pairs lost their (wall-tunnelling) corridors - the grid engine
  confirms all 11 isolated pairs are genuinely unreachable, the
  replay pin and docs/navmesh.md carry the new split (128 full +
  61 partial + 11 isolated).

- 2026-09-16: the viewer feedback round landed (the owner request:
  a camera that flies like a plane, a button that copies the camera
  position with the view direction and the route A/B pair, and a
  launch mechanism that restores the position from that data). The
  orbit rig is replaced by the flight rig: WASD flies along the full
  view vector (W follows the pitch), Q/E descend and climb, the
  pointer drag yaws and pitches, Shift boosts 4x, the wheel retunes
  the cruise speed (the panel carries the speed readout; the frame
  delta drives the movement so the speed is frame-rate honest). The
  feedback channel: the `copy view link` button freezes the whole
  view state - the camera pose over the game world axes (x, y,
  height, yaw, pitch), the armed or answered route pair, the visible
  tile selection, the filter, the height scale - into one URL (the
  clipboard write falls back to field selection for the non-secure
  contexts). The launch mechanism: the boot parses the same query
  parameters, restores the camera, the tiles, the filter and the
  scale, and the restored route pair re-runs on its own - a pasted
  link reproduces the exact view AND its answer. Verified live in a
  headless browser over the rebuilt six-region world: the flight
  controls move the camera along the view vector (the W probe moved
  it 1760 units into the pitch), the drag changes the yaw/pitch, the
  wheel the speed, the copy button builds the full link, and the
  link roundtrip (move the camera, copy, reopen) restores the pose
  to the unit and re-runs the hard bridge pair (34 waypoints, 5.7
  ms); the VLM review of the screenshots confirms the terrain, the
  route polyline with its markers and the panels.
  `TestNavmeshViewScriptContract` pins the boot contract (the rig
  controls, the parameter names, the copy wiring) against drift.

- 2026-09-16: the solid surface round landed (the owner report over
  two pasted view links: the view is tilted sideways, the render
  still has black triangles, and the request to show the small edge
  connections between the polygons colored green for fields and blue
  for water). All three answers, each verified live in the headless
  browser over the real 21_19: (1) the tilt was a real roll the boot
  framing leaked - `lookAt` under the default XYZ euler order left a
  z angle behind, the YXZ rig reinterpreted it and never zeroed it
  (a measured rotation.z of 0.503, a 28.8 degree horizon roll); the
  framing now computes the yaw and the pitch analytically and the
  rig writes the full euler every frame with the roll pinned at
  zero (rotation.z measures exactly 0). (2) the black triangles had
  two roots. The big one: adjacent rectangles sample their own
  inside cells for the shared edge heights, so 536 724 of the
  604 191 adjacency pairs of 21_19 disagree about the edge height -
  every geodata step rendered as a see-through black wedge (the
  pixel-exact clear color behind the crack). The NMV2 geometry
  payload now carries the height step walls: the vertical filler
  quads between the two bilinear surfaces, emitted through the
  MaxX/MaxY sides only (every shared edge walled exactly once), the
  crossing split keeps the quads simple when the profiles meet
  inside the span, and the region borders resolve their targets in
  the east and north neighbor tiles through the mesh. The smaller
  root: a steep quad tessellates into two triangles whose flat
  normals face apart and the away-facing half fell to black under
  the old light rig - the rig now carries an ambient floor
  (ambient 0.42, hemisphere 0.6, sun 0.45, fill 0.25) so no face
  drops into the darkness; the dark-face pixels of the two reported
  views measure 0.4 percent of the screen and the wedges are gone
  (what remains black is the honest void of the unwalkable cells -
  pixel-exact clear color with no geometry). (3) the edge
  connections overlay replaces the white polygon perimeter overlay:
  every real link portal of the tile rides the surface as its open
  world span (the NSWE gates, not the full shared edge), colored
  green for the field-field pairs, blue for the water-water ones,
  teal for the shore pairs and gray for the unresolved external
  targets - the legend carries the four swatches and
  `TestNavmeshViewScriptContract` pins the decode, the classes and
  the toggle. The payload: 244 837 polygons + 552 168 walls +
  275 830 links = 18.5 MB for 21_19, the layout check and the
  endpoint pin (header counts, the portal records, the wall
  records, the crossing split, the flat skip) live in
  TestNavmeshGeometryEndpoint and TestEncodeNavmeshGeometryWalls.

- 2026-09-16: the tessellation round landed (the owner report over
  the view link cam=51366,46831,-2966: the grass is still not fully
  filled and there are black triangles). One root cause behind both
  symptoms, present since the first viewer commit: the surface quad
  tessellation read the row-major wire corners (0 = min corner,
  1 = +x, 2 = +y, 3 = the opposite) but drew the two triangles as
  (0,2,1)+(0,2,3) - both anchored on the shared 0-2 edge, so the
  right quarter of EVERY polygon (the triangle between the east
  edge and the two diagonals) never rendered: the clear color shone
  through as one see-through wedge per rectangle, scaling with the
  rectangle size - the big maximal rectangles read as the large
  black triangles, the dense small-rectangle fields read as the
  unfilled grass with zebra-stripe gaps. The wall filler quads were
  immune (their corners ride the wire in cyclic order, where the
  same index pair is a correct 0-2 diagonal split), which is why
  the earlier rounds kept chasing lighting and height-step ghosts.
  The fix: the surface tessellation now splits along the 1-2
  anti-diagonal - (0,2,1) covers the lower-left half, (1,2,3) the
  upper-right one - while the walls keep their cyclic 0-2 split,
  and TestNavmeshViewScriptContract pins both index runs against a
  silent regression. Measured over the reported view (before ->
  after): clear-color pixels 174 401 of 1.44 M (12.11 percent, 515
  components, the largest a 596x165 slab) -> 440 (0.03 percent, 2
  small components); the top-down check of the same area: 8.39 ->
  0.03 percent; the elven village view: 0.00 percent with no
  zebra stripes (the VLM review of all three screenshots confirms
  the continuous surface). The leftover specks are honest voids,
  proven twice: the cursor raycast over the largest one answers
  "no tile" (no polygon exists there), and the new navbuild
  coverage audit TestRealRegionWalkableCoverage proves every
  walkable layer of every kept sheet carries a polygon (21_19:
  4 400 066 walkable layers, 4 394 442 covered, 5 624 dropped
  island layers, 0 holes) - the specks are the island sheets the
  four-layer minimum drops by design (a scratch probe of the
  largest oblique-view void found only DROPPED sheet verdicts in
  its cells). The route search and the double click picking
  re-verified live on the fixed geometry (a 254 unit pair answered
  in 53 us with 2 waypoints).

- 2026-09-17: the floating worlds round landed (the owner report:
  the bridge and the elven village must hang in the air above the
  water but connect into it instead, and the mother tree must not
  fuse its branches into the ground - "eto kasaetsya ne tolko etogo
  mesta", it is map wide). Two root causes, both fixed:

  1. The height step walls had no height cap: the NMV2 encoder
     walled EVERY XY adjacent polygon pair, so a floating deck over
     the lake got a 700 unit green curtain down to the water and
     every branch a grey stalagmite to the ground (21_19 carried
     ~22k fabricated walls over 80 units). The honest crack scale
     of the build is the 40 climb + 24 bilinear tolerance; the
     filler now refuses anything taller than 80 units - a taller
     step is the open air between two separate worlds and the void
     is the honest answer (the cap dropped the 21_19 wall block
     from 445 889 to 423 984 records, the worst surviving step is
     exactly 80, TestEncodeNavmeshGeometryWalls pins the over-cap
     rejection next to the crossing split and the flat skip).

  2. The geodata structural encoding walked straight into the mesh:
     the giant trees of the elven forest carry their canopies as
     stacked walkable-flagged layers (the trunk helixes step within
     the climb range but wall every step of the way up - the sheet
     flood crosses them because it ignores the NSWE walls by
     design, so a "ramp" of polygons fused the tree into the
     ground). The new dropIslandComponents builds the sheet graph
     over engine steps (the paired NSWE walls plus the climb rule,
     the exact canStep of the grid search, the wetness split
     ignored) and rejects every component that touches no region
     border - the seam where the neighbour region may continue the
     walk. The measured 21_19: 1386 floating sheets / 60 118 layers
     dropped (polys 244 837 -> 217 904), while the bridge deck, the
     village decks, the shore and the water survive - the grid
     engine confirms the verdicts (a ground-to-canopy search
     arrives at the terrain under the tree, never at the branch
     height; TestRealRegionFloatingIslands pins the drop and the
     survivors, TestBuildRegionIslandFilter pins the synthetic
     border/separation semantics).

  The full pack rebuilt (158 regions, 57.3 M polys, 4.3 GB, the 7
  known corrupt edge regions aside) and the viewer re-verified over
  the two reported views: the bridge and the village now float with
  open air gaps to the lake (the VLM before/after review answers
  PASS on both), the mother tree lost its fusion and the distant
  terrain walls that remain are the honest climb-step fillers. The
  user route of the report (33 680,56 807 -> 47 028,50 833, swim)
  still answers found in ~4 ms over the rebuilt mesh.

The follow-up candidates (NOT started): the acceptance stack switch to
the hybrid once the live sessions prove it (which may also skip
the engine confirmation flood on mesh partials - the ~13 s the real
hard pair dry search pays today), the Detour-parity optimization if
the fleet benchmark asks for it, the route-vs-engine replay harness
over the Dion hunting grounds.

- 2026-09-17: the capsule clearance round landed (the owner report: the
  pathfinding ignores the character collision capsule and puts the
  waypoints too close to the wall edges and corners - the character
  sticks in the passage and clips every bend). The server movement
  validation is cell level and never checks the capsule (the elven
  fighter template radius 7.5), so the planner owns the clearance:
  `pathfind.Capsule` (pathfind/capsule.go) answers the wall clearance
  of a point (the exact distance to the nearest closed NSWE wall edge
  of the layer nearest the reference z, exact within one cell) and
  `ApplyPath` enforces the radius over a planned waypoint path (the
  interior waypoints pushed off the walls by the damped projection,
  every leg sampled and bent around the walls through pushed-in
  anchor chains, every move validated by the engine line of sight,
  the first and the last waypoints never move, the honest fallback
  keeps the original geometry). The mesh funnel pulls its pivots
  inward from the portal span ends by the same radius
  (`Filter.WaypointClearance`, offsetPortal; a span narrower than
  twice the radius pivots at its middle). The engine post pass arms
  through `Engine.SetCapsuleClearance` - the bot wiring, the pathfind
  test UI and the navmesh viewer arm it with the template radius, the
  hunt hybrid serves the mesh answers through the same radius (the
  cleared filter plus the cleared waypoints). The viewer route
  endpoint clears the mesh answers through the viewer engine. Tests:
  the clearance pins (push off the wall, the pillar bend chain, the
  no-churn contract, the solid block fallback, the engine
  integration), the funnel pivot pins (the L corridor raw pivot on
  the wall boundary vs the cleared pivot at the radius). The same
  round fixed the shingled slope rendering (the owner report: the
  terrain steps down like roof sheets instead of joining edge to
  edge - hills, not stairs): the rectangle corners read the sheet's
  own VERTEX field now (the average of the sheet's cells around the
  grid vertex), so the adjacent rectangles of one sheet agree about
  every shared edge (the inside-cell corners disagreed by the full 8
  unit quantization step on EVERY slope adjacency) while different
  sheets keep their own levels and genuine cliffs stay sharp
  (navbuild/rect.go vertexHeight, the vertex tests pin the seam free
  join and the sharp shore step). The mesh pack must be rebuilt for
  the new geometry (data/navmesh is gitignored, cmd/navmesh-build).
  NOT DONE YET (the interrupted session): the viewer waypoint
  coordinates checkbox default off, the geometry variant toggle (the
  detour mesh vs the original l2j cells render plus the geom URL
  parameter with the camera and the route preserved), the tile
  rebuild and the visual before/after verification of the owner's
  two views.
