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
  NOT DONE YET (the interrupted session): the tile rebuild and the
  visual before/after verification of the owner's two views.

- 2026-09-17: the exact square port replaced the bilinear vertex
  field (the owner's changed position: the original l2j geometry IS
  squares, so the detour version must port it as close as possible
  WITHOUT changing its visual or actual representation): the
  rectangle growth now spans only the cells of one exact geodata
  height, every polygon is flat at that height (all four corners
  equal), the vertexHeight averaging, the bilinear tolerance split
  and the Options.HeightTolerance tunable are gone. The quantization
  staircase the interpolation used to smooth away is the honest l2j
  answer the mesh now carries - the mesh variant and the original
  cells render of the viewer draw the same geometry, the comparison
  toggle turned into the port audit. The NSWE wall fidelity, the
  sheet decomposition, the island drops and the 32 unit dedup are
  untouched (the port concerns the surface representation, not the
  reachability filtering). The height step walls close exactly the
  genuine steps now: same-height neighbors join byte for byte, the
  filler's job shrank to the real staircase. 21_19 measured:
  372 846 polys (was 245k wall honest bilinear, 390 596 orig rects),
  1 013 866 links (was 259 332 - every height run edge is a polygon
  boundary with its portal now), 44.2 MB tile (was 12.9), 26.4 ms
  decode, the hard pair 14.9 ms warm over 219 polys (was 7.7 ms
  over 171 - the honest price of the faithful squares, still 350x
  under the grid engine). Tests: the vertex round's seam pinning
  became the exact contract (exact_test.go: the flat corners, the
  per-height strips of the slope, the sharp shore step), the
  RectExactHeights build test pins the parabolic valley port,
  buildRects lost the tolerance argument. The four dense tiles are
  rebuilt (data/navmesh -force) and the owner's two views verified
  against the exact mesh. The lint etiquette: the gci formatter
  wants to re-tab the whole tree (its gofmt passthrough vs the
  spaces only policy - the pre-existing systemic conflict, the
  formatter of record stays gofmt-spaces); the new code adds zero
  non gci issues.

- 2026-09-17: the geometry variant toggle landed in the viewer (the
  owner comparison request, the client half): a "geometry" select
  switches live between the detour mesh and the original l2j cells
  render. The tile entries cache BOTH variants (built/loading per
  variant, the active variant's scene object in mesh), so a switch
  back is instant and a first switch loads the other variant on
  demand (the status row shows "loading orig" while the server
  renders the region, seconds for a real tile). The camera, the
  route pair, the drawn route and the tile selection live ABOVE the
  variant - they survive every switch unchanged, which is the point:
  the same funnel waypoints draw over the raw cells. The chosen
  variant rides the view link as geom=mesh|orig, the boot restores
  it BEFORE the tile loads (a pasted orig link opens on the original
  geometry) and the stale variant fetch that finishes after a fast
  switch stays cached without entering the scene.
  TestNavmeshViewScriptContract pins the toggle contract (the
  select, the per variant cache, the original endpoint, the URL
  parameter). Round etiquette: the lint --fix pass had re-tabbed the
  geometry files - the spaces only policy returned in a follow up
  commit (c57e3a0).

- 2026-09-17: the original geometry endpoint landed (the server half
  of the owner's comparison toggle): GET
  /api/navmesh/original/{key} renders one region straight from the
  raw l2j geodata cells into the same NMV2 payload the mesh endpoint
  serves - no sheet decomposition, no rectangle smoothing, no vertex
  field. Every cell layer with an open wall direction draws at its
  exact height; the only compression is the exact height merge
  (neighboring cells of the SAME layer height become maximal
  rectangles, pixel identical to the per cell quads): 21_19 measures
  390 596 rectangles over 4.4M walkable layers (22.3 MB with the step
  walls - the mesh tile scale), the worst audited region 21_22 lands
  at 1.2M rects. The height step walls follow the mesh payload rules
  (the MaxX/MaxY emission, the 0.5 unit floor, the 80 unit open air
  cap) and resolve the region borders through the east and north
  neighbor regions (the border strip pass, 2048 cells). The link
  portal block stays empty - passability in the original render is
  the cell geometry itself. The engine less viewer answers 501.
  Tests: the flat region collapse (one rect, ETag 304 roundtrip),
  the checkerboard merge (65536 rects, the water area, the ordering
  by height), the step walls (2*255*256 records, the first record
  content) and the real 21_19 scale pin (390 596 rects).

- 2026-09-17: the viewer waypoint coordinates checkbox default off
  landed (the owner request): the state starts false and the
  checkbox ships unchecked - one click brings the coordinate wall
  back. TestNavmeshViewScriptContract pins the default. Remaining
  from the interrupted session: the geometry variant toggle (the
  detour mesh vs the original l2j cells render plus the geom URL
  parameter with the camera and the route preserved), the tile
  rebuild and the visual before/after verification of the owner's
  two views.

- 2026-09-17: the route shortcut pass (the smoothing) landed: the
  funnel on the exact square mesh pivots at every clearance pinhole
  and the walker micro steers (the owner walk plan stuck at wp 25);
  Filter.Smooth arms the greedy farthest visible merge in
  navmesh/smooth.go - every merged chord crosses the intermediate
  portals inside their open spans and keeps the capsule radius from
  the wall spans (the link complement per polygon side), the raw
  funnel answer rides in Route.RawWaypoints. The viewer route variant
  toggle (smoothed vs raw, the path= URL parameter, one search serves
  both) draws the two answers; the bot navigator and the viewer arm
  the pass with the engine capsule radius. The owner repro route
  (21_19 swim) answers 63 smoothed waypoints of 81 raw, the granular
  legs (< 16 units) drop from 7 to 1. Tests: the corner under the
  capsule stays (the chord 5.66 off the inner wall is refused), the
  open boundary merge (the funnel pivot at a soft span end folds),
  the void detour keeps every pivot, the portal index chain, the
  unarmed contract. KNOWN NEXT: the grid per cell walls (the
  diagonal anti corner cut) are finer than the mesh side level link
  spans - 26 of 62 smoothed legs still hold a grid wall sample under
  the radius (worst 1.6 units near the owner stuck area), the grid
  capsule pass re-fragments the answer with micro anchors; the next
  round binds the shortcut pass to the grid wall oracle (the
  LegGuard seam) so the merged chords clear the server accurate
  raster directly.

- 2026-09-17: the wall oracle round: the grid per cell walls (the
  diagonal anti corner cut) are finer than the mesh side level link
  spans - the funnel pivots one radius off a mesh span end sit up to
  6 units off a real grid wall on the staircase (the owner stuck at
  wp 25), and the capsule pass re fragmentation (129 wps, 79 tiny
  legs) defeated the mesh smoothing end to end. Filter.Guard arms
  the LegGuard seam: the shortcut pass answers every chord to the
  grid capsule (pathfind.Capsule.LegClear - the movement line of
  sight plus the 4 unit clearance sampling), and the walker answer
  composes ApplyPath with the new ShortenPath fold (the greedy
  farthest visible over the post pass points, every surviving leg
  LegClear). The owner repro walks 14 waypoints of the legacy 148
  (path 4947 -> 4852, max leg 1070), the raw toggle variant keeps
  the legacy pipeline as the before picture. Tests: the guard
  refusal keeps the pivots, the guard merge folds (navmesh), the
  LegClear corridor center through wall answers, the ShortenPath
  open collapse and the sealed detour keep (pathfind). Visual
  verification on 127.0.0.1:8082: the staircase closeup draws the
  granular raw dots vs the clean smoothed line, the toggle and the
  path= link parameter restore both ways, screenshots in the agent
  download archive.

- 2026-09-17: the server move limit round: the owner asked to check
  the game server sources for the movement distance restriction the
  smoothing could trip. The Mobius C1 sources answer twice
  (network/clientpackets/MoveToLocation.java,
  entity/actor/Creature.java moveToLocation): the 9900 unit packet
  refusal (the walker's own maxMoveLeg = 1000 split already covers
  it) and the WATER clamp - the destination of every swimming move
  request scales onto the 700 unit sphere around the current
  position (the isInWater divider), and a target beyond it never
  answers. The smoothed open water legs (the owner repro runs the
  swim filter, the measured max leg 1070) trip exactly that clamp:
  the server stops the character short of every such waypoint and
  the follower never sees the arrival. Capsule.ShortenPath now
  answers the server clamp per anchor (the new legLimit probe over
  the engine water raster, the same OverWater oracle the water
  escape uses): a leg that leaves a water position splits at the
  clamp distance, the fold resumes from the split over the same
  horizon, the dry anchored legs keep their unclamped merge. Tests:
  the water legs cap (the split preserves the walk length and the
  endpoints), the dry flip (the long clear chord survives). The
  viewer smoothed variant rides the same fold.

- 2026-09-17: the double click pathfind round: the owner asked for
  the webui double click to run the pathfind and store the points
  into the dump state as if the bot itself planned them - the manual
  test loop of the stuck reports (move the bot by hand, call the
  pathfind with the map clicks, watch where it sticks, report the
  problem area). The manual move planner no longer waits for the
  2000 unit threshold: every manual move plans through the
  navigator (the mesh corridor, the capsule clearance, the wall
  guard - one pipeline with the bot's own walks), a failed search
  still falls through to the direct server routed walk. The dump
  keeps the most recent plan after its walk ends (the state last
  walk record, the "last walk plan (...)" dump section - the live
  plan expires with the walk, the stuck report needs the whole
  planned walk after it too); the dump parser reads the new section
  and the ApplyDump replay restores it as the walk plan of the repro
  bot. Tests: the near click plans and walks the plan, the
  replacement plans fresh, the record survives the expired plan and
  the clear (state), the dump round trip (webserver).

- 2026-09-18: the hierarchy hardening and the whole map trip
  measurements round. The environment survived the session timeout
  (the repo, the branch, the pack intact); the resumed work pushed
  the component gate commit first, then answered the owner's four
  asks. (1) The within tile benchmark (TestHierarchyVersusFlat21x19,
  the real 21_19): the flat search wins the reachable same tile
  pairs (4.9 ms vs 9.1 ms at 16k, 15 ms vs 5 ms warm at 21.5k) and
  draws the shorter corridors - the hierarchy now serves the cross
  tile queries and the flat capped escalations only (hierWorthy is
  cross tile, RouteApproach escalates on the budget cap). The
  abstract cache rides the LRU (32 regions - the whole map coarse
  walks accumulated every region graph into the OOM territory, the
  whole map tests gate behind SWARM_WHOLEMAP with the chunked
  stats). (2) The town routes (TestNavmeshTownRoutes, the
  teleporter coordinates of the C1 data): Elven Village ->
  Gludio plans one shot (10.8 s cold, 6.1 s warm, 121 794 units,
  found); Gludio -> Gludin and Gludin -> Giran strand one shot -
  the probes (TestBayNorthShoreProbe, TestSidecarPairProbe,
  TestWaterBorderStitch) name the cause: the shipped geodata leaves
  the open water of the Gludio bay and the inner bay unmapped (the
  straight line aims do not bind) and the 18_21/17_21 border steps
  land heights over the climb (land against flat sea - the stitch
  refuses honestly). The re-path walk (the bot's real pattern) covers
  58 852 of 73 199 units in 3 replans on the Gludio Gludin leg.
  The fix belongs to the data: a water filled geodata refresh or the
  operator waypoint graph. (3) The map size arithmetic: the pack is
  3.5 GB of gzip tiles against the 543 MB l2j geodata (6.4x) - the
  price of the explicit topology (every height run edge a polygon
  with its portal links, the query structure the funnel and the
  corridor searches need); the quantization headroom (uint16 cell
  rects, uint16 portal spans) sits at ~35% of the decoded bytes,
  documented in the report. (4) The bot command and the load answer:
  the navigator switch is the presence of the tile directory
  (-navmesh data/navmesh or the autodetect) - no separate flag; the
  maps load lazily per query (the tile LRU 4, the abstract LRU 32,
  the hop cache 4096), the whole pack loads only in the viewer mode
  (the bare -show-navmesh stitches every tile).

### Progress (2026-09-19, the agents.md restoration round)

- the owner session limits land in AGENTS.md as the mandatory first
  section (the 2 hour life from the owner prompt, the mandatory stop
  at 1h45m with everything pushed, the timer reset on every fresh
  prompt, the /home/z/my-project/.session_start_ts stamp protocol) -
  the rule previously lived only in the agents.md run notes.
- the sandbox subprocess study of this session lands: both detached
  variants (setsid own session, double fork env -i renamed binary)
  survive 8+ minutes across tool calls with heartbeat probes
  (scripts/detach_probe_a.sh, detach_probe_b.sh in the sandbox, not
  in the repo) - the 09-18 verdicts do not reproduce today; the
  single-tool-call pattern stays the always-correct option, the
  detached server stays the verified option with a ps re-check.
- the pre-split operational nuggets return from the c7786c9 split in
  the condensed form: the Mobius stack operational notes are whole
  again (flood protector mechanics, re-registration wait, account in
  use self heal, slow SIGTERM, single invocation e2e, auto
  registration), the client proxy short form is back (emulated
  login, replay+relay contract, shared cipher chain, proxy.log,
  proxy_e2e, port layout), the archer guard retaliation fact joins
  the protocol notes.
- verified against the collected versions (the file history
  v01..v09): the c7786c9 split dropped nothing load-bearing - the
  ~163 absent lines of the 1733 are reflowed, superseded or
  rephrased in docs/; the restoration adds the operational layer
  back, not the 112 KB blob.
## Active task: the webui debugging surface - the walk plan timings, the pathfind link and the hover coordinates (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`. Commits
as melg8. Other agents may push to the same branch concurrently -
rebase before every push. Owner request (three pieces, each its own
atomic commit):

1. **The dump state walk plan timings** - the state dump of a walking
   bot carries not only which waypoint it aims at but how long every
   leg took: a stuck point shows its cost, not just its name.
   - `state/bot.go`: the timing view of the published walk plan - the
     zero point (`walkPlanStart`, the first publish of the route) and
     the observed arrival of every waypoint (`walkWpAt`, the entry i
     fills when the follower cursor moves past i on the same route
     republish). A fresh route (or a cursor that moved back) restarts
     the view; a mid walk publish with the cursor already ahead
     pre-fills the passed prefix. `publishWalkPlanLocked` +
     `walkPlansSameRoute` (the cursor blind route compare). The last
     walk record copies the timing view (lastWalkStart, lastWalkWpAt)
     so a finished walk keeps its leg durations. The Snapshot carries
     the Go side dump fields only (`json:"-"`, the wire stays byte
     identical): WalkStart, WalkWpAt, WalkAt, LastWalkStart,
     LastWalkWpAt.
   - `webserver/dump.go`: the walk plan section prints the `started`
     line (the zero point, the last seen moment, the time on the
     walk), the passed waypoints carry `(passed, t+10.4s, leg 5.2s)`
     and the aimed one ` <-- TARGET (walking 45.2s)` - the stuck leg
     number. The last walk plan measures the aimed leg to the moment
     the plan ended. Sub minute durations keep the tenth of a second,
     the longer ones fold into the minute shape. The dump parser
     needs no change (the wp lines keep the leading `x y z` triple).
   - Tests: the state timing tracking (the advance, the equal
     republish, the fresh route, the record copy) and the section
     formatting (the suffixes, the minute fold, the untimed shape).

### Progress

- The walk plan timing view and the dump suffixes - this commit.
- the pathfind link button (webui): the map toolbar gains the
  `pathfind link` button beside the session dump - one click freezes
  the published walk of the selected bot into the 3D navmesh viewer
  URL and copies it (the clipboard fallbacks mirror the dump button).
  The link carries the from/to pair off the walk plan (the planning
  origin, the final destination), the tiles around the pair (the
  bounding box grown by half a tile - the owner example link's
  20_19..21_20 block reproduces exactly), the swim filter, the
  scale/geom/path defaults and the camera pose computed with the
  viewer framing math (the three quarter orbit south east of the
  route midpoint, the analytic yaw/pitch of frameInitialTiles) - the
  pasted link answers itself in the -show-navmesh viewer, ready for
  the own experiments or for attaching to an agent report. The shift
  click asks for the viewer base address and remembers it in the
  localStorage (the default stays http://127.0.0.1:8082/). The
  repro_hud harness pins the URL structure (the pair, the camera
  orientation, the tiles, the defaults, the bare snapshot without a
  plan).
- the webui cursor surface: the walk plan coordinate labels draw on
  hover only now (the constant per waypoint (x, y) labels littered
  every planned walk), the hovered waypoint prints its full x y z
  triple in the dump walk line format; the status bar gains the
  cursor chip (the world point under the mouse, the full triple while
  a waypoint is held) and the ctrl+c (meta+c) of the map copies the
  chip values (the selection aware fall through keeps the browser
  copy intact when the pointer is off the map or a text selection is
  active); the viewer coordinate inputs accept the bare x y pair now
  (the z inherits from the other line) and keep the wp prefixed dump
  lines parsed (the first three numbers after the wp marker bind -
  the new timing suffix carries numbers of its own, the last three
  contract broke for the dump walk lines). The repro_map_render
  harness drives the whole surface (the label absence, the hover
  label, the chip, the copy shortcut, the flash, the leave reset).

## Active task: the village escape round - the route following cursor escape, the move start watchdog and the refused click root cause (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Rebase before every push.

### Goal (the owner prompt of 2026-09-19)

1. The cursor key escape (the WASD-like recovery of a click refusing
   cell) walks STRAIGHT lines toward the aim today - the owner rule:
   the claimed steps must follow the PATHFIND ROUTE and must never
   try to cross the whole map in a straight line.
2. The walker must switch to the next recovery mode AS FAST AS
   POSSIBLE when the current one did nothing - a movement command
   that never started the movement must not wait a full stuck window
   (15 s / 4 s today).
3. The owner asks WHY the server refuses the ordinary ground clicks
   in the village plaza zone, and the same at the shop and the
   teacher hall entries - a systemic question, with an elegant
   solution wanted. Acceptance: the `village-escape` scenario
   (the refused-click dump cell) must keep passing.

### Progress

- 2026-09-19: the route following cursor escape (commit 1). The
  escape arms `cursorEscapeRouteSteps` when the leg holds a plan:
  the claimed ValidatePosition steps march the plan polyline from
  the current cursor (the pathfind route the leg already walks),
  interpolated into run-speed strides, water guarded per stride and
  capped at `cursorEscapeRouteMax` (2500 units, one direct hop of
  ground) so an escape stays a pocket recovery - the claims never
  carry the character across the map. The straight line ladder
  stays as the planless fallback. The bend closure keeps the route
  bend points among the steps so the next segment leaves from the
  bend and not from a cut corner. Tests: the bend corridor (every
  step sits on the planned line, the bend lands among the steps,
  the stride spacing, the cap), the cap on a 12000 unit route, the
  wet stride stop, the planless nil fallback; the plaza repro tests
  re-verified green with the route following ladder.

- 2026-09-19: the move start watchdog and the refusing pocket fast
  path (commit 2). The new `noteMoveStart` watchdog arms a deadline
  (`moveStartWindow`, 3 s) when a walk click goes out while the
  character stands still, clears it on a position change or the
  server's own movement broadcast and names the click dead when the
  deadline passes on the baseline cell - a dead click forces the
  stuck verdict (the `forceStuck` gate of walkStuck skips the window
  check for one verdict), so the recovery ladder runs at ~3 s per
  rung instead of the 15 s first window. Wired into the three walk
  machineries: followWaypoints (the planned legs, force), the direct
  routed leg (a silent hop arms the cursor key escape instead of
  re-hopping into the 45 s window) and the direct zone legs (the
  stall backdate fires noteZoneLegStall this tick). The refusing
  pocket verdict: a stuck verdict with refusal evidence ON the cell
  where the leg's first refusal latched, AFTER the varied aims of
  the leg are spent - the varied aims keep their chance to cure the
  target specific refusals (the round 82 order), the escape takes
  over the moment they prove useless from the same ground. The
  escape arms from the follower path now too: walkTownWaypoints
  drives the claims while the escape holds the leg (symmetric with
  walkDirectLeg), followWaypoints stands its clicks down while
  armed, the settle message names the walk that resumes (the routed
  hops or the planned clicks), and the mode 0 arm aims the ladder's
  far end (a self cell arm answers the stopMove refusal before the
  flag latches). Tests: the silent click recovery inside the move
  start window, the pocket sequence (the variants one per verdict,
  the escape once they are spent, the claims owning the leg, no
  further mouse clicks), the direct leg silent hop escape; the
  varied aim and plaza repros re-verified green.

- 2026-09-19: the refused click root cause documented (commit 3).
  The WHY: the village geodata is a layer sandwich (the deck over
  the water floor 872 apart; the teacher hall roof/interior/water
  328/1136 apart; the shop interior partially walled; the deck not
  flat even inside one pack) and the server resolves every click
  target and line step layer by the NEAREST z - a pack vintage
  disagreement flips the layer, the line check refuses, the click
  collapses and the deployed build answers ActionFailed (the
  official client's mouse clicks met the same wall). The evidence
  pins in `pathfind/village_layer_sandwich_test.go` (the four
  measured stacks), the full analysis lives in Round 85 of
  `docs/development_log.md` with the owner side cure (the geodata
  vintage alignment + the master's direct-movement fallback). The
  bot side answer is the claim transport of commits 1-2.

### Progress (2026-09-19, the feedback loops audit)

- the audit of the feedback an autonomous agent receives lands as
  docs/agent_feedback_loops.md (the inventory by layer: static, test,
  live stack, runtime observability, process memory; what is good,
  what needs improvement, what is missing, the priority order) - the
  doc joins the AGENTS.md documentation map.
- PROGRESS.md regenerated through tools/progress_report.sh: the
  ladder now reads M1 red (last run FAIL - the class transfer runs of
  2026-09-13), the stale 2026-09-12 page is gone; the dashboard
  staleness is recorded in the audit as an improvement item (the
  regeneration rule exists, it just was not applied).
- the headline findings of the audit: task check:all misses the
  build and vet gates the verify-loop skill mandates; coverage is
  collected but never archived; benchmarks have no committed
  baseline to diff; CI is absent while the go-verify-loop skill
  references it; the acceptance scenarios run serially against a
  parallel-ready account partition; the session start ritual is
  prose, not a command.

### Progress (2026-09-19, the feedback audit remediation: all twelve items)

- every improvement and every missing item of
  docs/agent_feedback_loops.md landed in one tooling round (the
  implementation status section of the audit maps each item to its
  landing place):
  - the gate: `task verify` (build + vet + lint + test + fmt:check,
    check:all is the alias) - check:all no longer skips the two
    gates the verify-loop skill mandates; `docs/ci_workflow.yml`
    (the owner copies it to `docs/ci_workflow.yml` - the push of
    a workflow file needs the workflow-scoped token) runs the same
    order on every push plus the race slice of
    connection/pathfind and the logfmt scan (the CI that did not
    exist now exists); `task prepush` (tools/prepush.sh, the 20-40 s
    touched-packages gate) is documented as mandatory in the git
    conventions and installable as a git hook
    (`tools/install_dev_tools.sh hook`).
  - the numbers: `task test:cover` archives
    runs/coverage-latest.txt (committed, 27 packages seeded) and
    fails on a per package drop beyond COVER_DROP_LIMIT (2.0 pp);
    `task bench:save` + `task bench:diff` (cmd/benchdiff, the
    offline benchstat twin with its own tests) give the benchmark
    rule its committed baseline; `task progress` is the one-word
    PROGRESS.md regeneration.
  - the wall time: `-acceptance all-parallel` (Manager.RunAllParallel)
    launches every scenario at once on the temp account partition
    with the flood protector stagger and collects every failure
    instead of stopping at the first - the headless run pays the
    slowest scenario, not the sum (the barrier test pins the
    overlap).
  - the conventions: internal/logfmt parses the module and asserts
    capital-first (component tags count) and no trailing period at
    every production log call site - the six existing violations it
    found (main.go, proxy/server.go, webserver/navmesh.go) are
    fixed, the rule starts from zero debt; docs/flake_ledger.md
    opened with the 8034403 poll-test pins as the seed rows and the
    verify-loop skill names it mandatory after every flake fix.
  - the ritual: tools/session_start.sh prints the bootstrap
    checklist (the stamp age against the 2h/1h45m budget, the
    fetch/rebase verdict, the ss 2106 probe, the open H-001..H-004
    ids, the active task headline); the hypotheses registry carries
    the advance-or-close-one-per-session rule; progress_report.sh
    gained the trailing-window trends (per scenario pass rate, XP/h
    and stuck deltas).
- the formatting source of truth is pinned: go.mod carries
  `toolchain go1.26.8` (the tree is gofmt-spaces clean under the
  1.26 gofmt; the 1.24-line gofmt disagrees on the struct field
  comment layout) - `task fmt:check` now agrees with the committed
  tree on every host, and CI (setup-go + the toolchain directive)
  would have caught the mismatch instead of silently going red on
  the whitespace gate.

- 2026-09-19: the server frame click transport (the point 3 bot side
  cure). The WHY: the round 85 refusal mechanism names the clicked z
  the layer selector - the server resolves the click's destination
  layer by the nearest height to the z the request carries, the plan
  waypoints carry the bot pack's mesh z, and wherever the pack
  vintages disagree about a surface's absolute height the raw mesh z
  names the wrong layer (the village sandwich flips) and the click
  refuses. The HOW: the walk layer measures the vintage shift at the
  one pair both frames vouch for - the character's server vouched
  standing z against the plan's first waypoint z (the same cell on
  the pack) - and rides every plan derived click z into the server
  frame: the town leg clicks and their long leg splits, the forward
  route samples, the escape hops, the varied aims, the manual walk
  follower and the quest segment follower (each machinery measures
  its own plan start). The server vouched clicks (drops, mobs, npc
  approach points, self position aims) stay untouched. The guards:
  an offset beyond 500 units is a layer snap and measures zero (the
  872 unit deck over water gap must never anchor), a swimming
  character measures no shift (the swim z against the mesh floor is
  geometry), and the arrival re-measurement died in the teacher walk
  repro (the 16-35 unit ramp steps made the 150 unit arrive radius
  measure the NEXT step's rise into the offset) - the offset
  calibrates at the plan starts only, each re-path re-measures on the
  surface the character actually stands on. The Gludio lesson rides
  unchanged: the anchored z is never the bare self z, it carries the
  plan's own relative geometry in the server frame. Tests:
  `hunt/click_frame_test.go` pins the measurement, the calibration,
  the systemic click transport, the long leg split, the layer snap
  discard, the manual walk and the quest segment; the existing suites
  stay byte identical green (26 packages ok, lint --new clean).

### Progress (2026-09-19, lint debt clearance round)

- The full lint debt (~200 tracked findings from the ungated parallel
  week, plus the tail the default `max-same-issues: 3` cap kept
  hidden) is paid: `golangci-lint run ./...` answers **0 issues** with
  `max-issues-per-linter: 0` and `max-same-issues: 0` now pinned in
  `.golangci.yml` so the gate always sees the complete list.
- Real code fixes: the `eh-eh` tautology and the dead `end` clamp in
  the seam scan and the region chord, the unused `autonomous` param
  and the always-nil error of `runSessionSupervised` (six call sites
  simplified), five dead declarations deleted (soakMaxLevel,
  zoneOverrideSlack, patrolZone, dstComps, runHopBudget), five
  always-constant test helper params folded, the G109 bounds guard
  now sits next to the conversion, errorlint switched to errors.Is,
  the union-find `var find` pairs merged, the concat loops rebuilt on
  strings.Builder, the v1/v2 wire twins and the packet clone carry
  documented dupl relief, the water zone table and the two replay
  probes got the analysis-driver exclusion, `tools/prepush.sh` and
  the CI lint step now run the full uncapped lint.
- The suppression policy is uniform: every complexity finding carries
  one `//nolint:a,b // note` above the func (36 sites), the
  deliberate partial inits live in the `exhaustruct_v5`
  `ignore-patterns` list with per-group reasons, stale directives
  removed (nolintlint is the auditor).
- AGENTS.md gains the "Tree cleanliness discipline" section (caps off,
  fmt before commit, suppression policy, relief lives in config,
  dead code is deleted) and the Code conventions section now matches
  the real config.
- CI workflow copied to `.github/workflows/ci.yml` (lint step flipped
  from `--new` to the full run); the push needs the workflow-scoped
  token, otherwise the owner copy stays the fallback.

## Active task: the pathfind link repro contract - the viewer rebuilds the very search the bot walks (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner report ("Выясни почему не совпал маршрут у бота в реальном
мире и при построении через веб", the 2026-09-19 14:26 temp11 dump):
the bot walked its planned 17 waypoint dry zone return from the elven
village plaza to the hunting square center, the pathfind link opened
the 3D viewer at the same from/to pair, and the drawn route had
nothing in common with the walk. Find the cause and make the link a
reproduction.

### The diagnosis (the full analysis is Round 87 of the development log)

1. The viewer's capsule post pass folded the whole mesh route into
   ONE straight chord: the grid oracle of the fold is water blind
   (LegClear=true while 5 of 65 sampled chord points sit over the
   elven lake). The bot's plan is the search answer as produced; the
   fold drew a route the bot never walks.
2. The link hardcoded `filter=swim`; the zone return plans the DRY
   search - two different corridors (13.3 km dry detour vs 11.9 km
   swim cut).
3. The viewer answered the exact destination; the bot plans the
   approach search (radius 200) - the plan legitimately ends 185
   units short of the destination.
4. The frozen area bans never rode the link (silent in the clean
   session, a real gap in general).

### The fix (this commit)

The search contract rides the plan: `state.WalkPlan.Search`
(the `walkSearch` wire field: dry, approach, avoid circles) stamped
by `startWalkLegSearch` / `planUserWalk`, cleared by
`armDirectLeg`, published by `geodataWalkPlan` / `userWalkPlan`;
the viewer POST gains `approach` / `avoid` / `fold`, the viewer URL
and the HUD link round-trip the same (`fold=0` = the plan repro mode
serving the answer as the bot publishes it); the dump names the
filter word in the walk plan header and carries the full contract on
its `search` line for the paste-a-dump flow.

### Progress

- Round 87 lands as one commit: the state contract, the hunt
  stamps, the viewer handler/URL/POST, the HUD link serialization,
  the dump header word + search line, the tests (state JSON, the
  webserver handler pins, the hunt stamps, the HUD harness, the
  dump round trip) and the docs (webui.md, navmesh.md, this file,
  the development log round).

- 2026-09-19: the escape walks the route, not the chord (the 14:46
  dump round). The WHY: the 14:46 dump proved the cursor key escape of
  a DIRECT leg marched the straight chord to the far zone target -
  armDirectLeg replaces the waypoints with the single destination
  spec, the route following ladder over it interpolates the chord
  (both escape aims of the dump sat on it byte for byte), the claims
  dragged the character through the village geometry and the water
  guard stranded it. The HOW: replanDirectEscapeRoute runs the leg
  start search for the phase (the session bans respected) before the
  claims build, installs the fresh route as the leg plan and stands
  the direct leg down - the WASD escape walks along the planner's
  bends and the settle returns the walk to the normal routed clicks
  on the same plan (the owner contract: WASD along the route, the
  normal mode at the point); the planless fallback keeps the pocket
  contract toward the validated hop aim. The existing repro drivers
  now mirror the production dispatch (the armed escape drives before
  the direct leg check). Tests:
  `hunt/cursor_escape_direct_leg_repro_test.go` (the dump repro on
  the real pack, the planless fallback unit pin, the village escape
  end to end over the refusal pocket), the hunt suite green, 28
  packages ok, lint --new clean.
## Active task: the unit test gate red - the race slice budget and the coverage lift (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner reported the unit tests red. The diagnosis split into
three independent defects, all fixed in one round:

1. the whitespace gate was red tree-wide: `runs/.cover-run.log`
   (a runtime artifact of `tools/coverage_delta.sh`) was committed
   in 56adde9 and its `go test` output carries tabs, so
   `task fmt:check` / `task verify` / prepush fail on any machine;
2. the CI race slice (`connection` + `pathfind` under `-race`) was
   red: `TestFindPathFromDeckToFarWestZoneStaysOnRamps` asserts the
   production hunt tick budget (`< 5s`) on a wall clock the race
   detector inflates 6-10x (8.15 s measured against the plain
   0.84 s run) - the assertion reported instrumentation cost as a
   route failure;
3. the per package coverage sat low in the packages whose logic is
   reachable but untested (cmd/swarm 7.4%, huntaudit 19.1%).

### Progress

- `runs/.cover-run.log` and `runs/.coverage-current.txt` untracked,
  both gitignored (the committed baseline stays
  `runs/coverage-latest.txt`); the whitespace gate is green again.
- The race budget pinned, not the tolerance: `raceDetectorBudget`
  (a build tag pair in pathfind, 10 under `-race`, 1 plain) scales
  the two zone route wall time budgets; the assertion still fails
  on a real 10x planning regression. Plain mode keeps the exact
  production figure. The flake ledger row added (the detector
  overhead is instrumentation, not scheduler nondeterminism).
- Coverage lifted with reachable-surface tests: `cmd/swarm`
  7.4 -> 11.4 (the fleet account ladder, the address split),
  `huntaudit` 19.1 -> 23.0 (the spot filter, the game endpoint
  renderer, the draft mode no-op), the packet wire layouts pinned
  per file (the sell multi-entry body, the creation request full
  field order, the cursor key move mode, the cast modifiers).
- Real bug found by the new pins: `fleetAccountName` collided a
  numbered base with its first follower (temp2 fleet ran
  temp2, temp2, temp4) - the ladder now continues the base number
  (temp2, temp3, temp4); both call sites (the per bot account and
  the startup log line) share the fixed function.
- `to_game_server` stays at 65.3% honestly: the remaining branches
  are the defensive `Writer` error plumbing that a `bytes.Buffer`
  backend can never fire - reachable ceiling reached without
  touching production code.
- Baseline: `runs/coverage-latest.txt` re-committed, total
  statement coverage 75.3%; verify (build, vet, lint, test,
  fmt:check) green, the two fixed zone route tests green under
  `-race` directly.

- 2026-09-19: the direct walk dies (the 16:02 dump round). The WHY:
  the 16:02 dump (build 07ccb0e, bot test1) walked the path round 88
  did not cover - the spawn cell 43032 50408 is a LOCAL refusal
  pocket on the real pack (every direction refuses but south), the
  clicks never left the bot so no server answer ever arrived, the
  escape arming branches never ran, the ladder collapsed the plan
  into the single far waypoint through armDirectLeg and the walk sat
  on the forbidden direct line for three trip cycles while the
  widened bans (48 -> 96 -> 192) could not move the mesh's first
  funnel waypoint off the pocket. The offline probes (the real pack +
  the built real mesh tiles) pin both halves: the mesh plans the 64
  waypoint route whose wp 0 IS the refused 8 unit click, the grid
  refuses every first leg. The HOW: the direct server routed walk is
  ELIMINATED - escalateFrozenLeg rung 2 arms the cursor key escape
  ALONG THE CURRENT PLAN (the claims follow the planner's bends, the
  settle returns the normal routed clicks on the same plan, the
  re-arm repeats while the attempts last), the budget-burned zone
  return and the failed planning hold with a paced log instead of
  marching walkZoneLeg (which stays only for the in-zone patrol and
  the no-navigator deployments), and the planless escape aim clamps
  into the pocket radius. Every walk plan of the loop now carries its
  mesh search contract - the search-less plan view WAS the direct
  leg's fingerprint. Tests:
  `hunt/direct_walk_elimination_repro_test.go` (the 16:02 dump end to
  end on the real pack + mesh, the no-server-answer ladder pin, the
  planless clamp) + the reworked contract pins; the direct leg test
  set retires with the machinery. The live acceptance village-escape
  answers PASS on the built binary, tools/mobius_e2e.sh answers
  E2E_OK, the hunt suite green, every package ok, no new lint
  findings, the whitespace gate green.

### Progress (2026-09-19, the red unit tests and the coverage lift round)

- The owner report ("исправь тесты чтоб все проходили") verified
  against the full gate surface: build, vet, the full uncapped lint
  and the plain `go test ./...` (28 packages, `-count=1`) were green
  already - the red tests live in the race slice and in the
  per package timeout.
- Three fixes, two real races found by `go test -race -count=1
  ./...` (first full-repo race run on record):
  1. `memwatch` `TestWatchLogsFootprintLines`: the watch goroutine
     logged into a plain `bytes.Buffer` while the test polled
     `out.String()` from the test goroutine - the log.Logger mutex
     covers only its own writes, never the reader. The test now
     wraps the buffer in a `syncBuffer` (a mutex-guarded pair of
     Write/String), the race detector stays silent across 5
     consecutive runs.
  2. `hunt` `TestDriveGatekeeperTeleportHappyPath` /
     `TestDriveGatekeeperTeleportStaleHtmlIgnored`: the simulated
     server goroutine wrote `fakeGame.htmlNPC`/`htmlBody` directly
     while the production `awaitDialog` loop polled
     `LastHTMLDialog()` from the loop goroutine. `fakeGame` gains
     the `htmlMu` mutex; the poll reads and the simulation writes
     go through `setHTMLDialog`; verified with `-race -count=3`.
  3. `pathfind` (600.065 s) and `pathfind/navbuild` (600.049 s)
     both died in the 10m default `go test` timeout with the search
     tests mid-flight at 4 s and 24 s - the detector's 6-10x hot
     loop overhead on a two core box, no race found. `task
     test:race` now carries `-timeout=30m` with the measured
     reasoning in the comment.
- Coverage lifted where the reachable surface was untested:
  `cmd/gofmt-spaces` 34.5 -> 84.5 (the CLI layer: -l/-w/stdout
  modes, the dot and underscore dir skip rules, the explicit dot
  root entry, the single file and non-go arguments, the broken
  source failure that does not stop the batch, the default cwd
  walk), `cmd/benchdiff` 60.7 -> 75.3 (the metric block rendering
  and the empty-unit skip, the missing file error, the zero base
  delta guard, the vanished-benchmarks report, the one-sided
  metric join, the GOMAXPROCS strip boundary), `huntaudit`
  23.0 -> 24.5 (the broken JSON resume refusal, the directory read
  error path, the atomic temp+rename write with the summary stamp,
  the missing anchors file refusal). `memwatch` holds 100.
- Total statement coverage 75.3 -> 75.6; the baseline
  `runs/coverage-latest.txt` re-committed with the deltas.

### Progress (2026-09-19, the post-rebase verification round)

- The full plain `go test ./...` after the rebase onto 7a9544a (the
  direct walk elimination) surfaced the repro tests the round left
  hard-failing on any tree without the local artifacts:
  `TestReproRefusedSpawnNeverWalksTheDirectLine` and
  `TestReproRefusedSpawnLadderArmsTheEscapeWithoutServerAnswers`
  died in `spawnDumpMesh` on `require.NotEmpty(dir)` while the
  navmesh tiles are a gitignored runtime artifact (the same class
  as the geodata pack, which `reproEngine` skips correctly).
  `spawnDumpMesh` now skips with the honest message when the tiles
  are absent (the repo skip pattern of navbuild/real_test.go) - the
  dump reproduction still runs to the end where the tiles exist
  (verified: the tiles rebuilt for 20_18..21_20 with
  `cmd/navmesh-build -regions ...`, both tests green against them).
- Final verification after the fixes: `go test -count=1 ./...`
  answers 28 packages ok, zero failures, the whitespace gate and
  the uncapped lint green.

## Active task: the priced water - the walled form retires, the bot plans through the water objects (2026-09-19)
## Active task: the wasd ground progress - the escape claims mark the walked waypoints and carry the server heading (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner directive ("в коде кажется остались устаревшие способы - не
должно сохраняться filter=swim - бот должен рассчитывать путь и через
водные объекты, просто с корректными замедлениями - в воде плыть
медленней чем бежать"): the dry/swim filter dichotomy is the
outdated way. One search remains - the water is a price (the
run/swim ratio 2.3), never a wall - and the walker walks the wet
legs the plan carries.

### The round (the full analysis is Round 90 of the development log)
- `navmesh`: `AllowWater`, `DryFilter`, `RouteDry` deleted;
  `DefaultFilter` is the one priced search (the escape keeps its 8x
  water price).
- The grid engine: `search.dry`, `FindPathApproachDry(+-Avoiding)`,
  `DryLine` deleted; the pricing, the direct line dry gate and the
  `legDry` smoothing stay (they ARE the pricing honesty).
- The hunt loop: one priced search everywhere (`startWalkLegSearch`
  without the non-dry switch, the shop escalation and the zone
  return fallback ladder deleted); the click water guard retired
  (`clickWouldEnterWater`, `shortenWetHop`, the wet variant skip,
  the extension water gate); the escape gate reads the plan's intent
  (the aim waypoint below `pathfind.WaterLevel` and ahead of the
  character keeps the swim walking).
- The web: the viewer filter select, the `filter` URL param, the
  POST field and the response word gone; the links carry
  `approach`/`avoid`/`fold=0` only; the dump header names `mesh`;
  `ApplyDump` restores the search contract into the replayed plan.
- The state wire: `WalkSearch.Dry` deleted
  (`{"approach":...,"avoid":[...]}`).

### Progress
- Round 90 lands as one commit: the two engines, the hunt loop, the
  webserver handler, the viewer and the HUD, the dump writer and
  parser, the state wire, the pricing pins (the wide band swims, the
  narrow band detours - whole flat block worlds, the per cell setCell
  pillar artifact documented), the escape gate pin, the docs rounds.
The owner reported two defects of the round 89 escape on the 17:18
session (build 94ec5e3, bot test1): the wasd walked waypoints did not
mark passed and the resumed clicks walked BACK to them (two minutes
of backtrack legs in the dump), and the web UI showed the character
facing a direction it never walked during the wasd walk.

### Outcome

- The claim ladder carries a waypoint map now: every completing claim
  marks its route waypoint passed while the escape streams (gated on
  the escapeFollows verdict - a server that ignores the claims never
  fakes progress), the settle advances the cursor at the position the
  character actually reached (never the ladder's aim) and
  re-baselines the stuck window, so the resumed clicks aim forward.
- The claim heading follows the mobius convention
  (LocationUtil.calculateHeadingFrom: atan2(deltaY, deltaX) - the
  swapped atan2 the old code carried mirrored the facing; the mirror
  also pointed the mobius cursor-key obstacle probe behind the
  character's back), the claims set the session facing optimistically
  (state.Bot.ApplySelfFacing), and the claim echoes keep that facing
  while the claims own the stream (GameClient.placementHeading gates
  the ValidateLocation/StopMove headings).
- Reproductions: cursor_escape_wp_sync_repro_test.go (the walked
  waypoints marked passed mid-escape and after the settle, the
  forward-only resumed clicks, the no-skip window after the settle,
  the heading convention pins with the dump's own 33472 sample) and
  TestGameClientClaimEchoKeepsTheClaimFacing in the connection suite.
- Live gates: village-escape PASS on the deployed stack,
  tools/mobius_e2e.sh E2E_OK, `go test ./...` every package ok,
  golangci-lint 0 issues, the whitespace gate green. The mobius
  server stays untouched.
