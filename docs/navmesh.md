# The navmesh pathfinding: the Detour-style runtime and the Go mesh builder

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The implementation round of the `feature/new-pathfind` branch: the
production port of the Detour **query** model the research round
recommended (`docs/recast_pathfinding.md`), fed by a custom Go mesh
builder that works straight from the l2j geodata. The grid engine of
`internal/swarm/pathfind` stays the click validation and local walk
layer it already is; this subsystem answers the long routes.

## The architecture in one paragraph

The offline `cmd/navmesh-build` converts every `X_Y.l2j` geodata
region into an `X_Y.nm` navigation mesh tile: the walkable cell
layers are partitioned into 2D manifold **sheets**, every sheet
decomposes into maximal rectangles of one exact cell height (the
faithful square port - the mesh represents the raw l2j squares as
they are), and the polygon links carry the open NSWE portal spans of
the shared edges. The runtime (`internal/swarm/pathfind/navmesh`)
loads the tiles lazily and answers the queries over them: the 3D
nearest polygon resolution (the stacked-layer disambiguation), the
A* corridor search with the water area pricing, the funnel string
pulling and the water escape. No C++ anywhere: the Recast build
pipeline is replaced by the sheet decomposition the research round
proved.

## The tile format

One tile is one region: a 40 byte header (the magic `SWN1`, the
version, the region key, the climb, the section counts) and the
sections - every section 4 byte aligned, little endian. The version
3 wire is the current one (the columnar layout, the bucket grid
index; docs/fastpath_research.md section 11), the decode reads the
version 1 (the int32 records and the bounding volume tree) and the
version 2 (the uint16 quantization) packs the same way.

- **The polygons are rectangles in region local cell bounds**
  (`X0 <= cx < X1`, `Y0 <= cy < Y1`, half open). The world rectangle
  spans the grid vertices `X0*16 .. X1*16` anchored at the region
  origin `((col-20)*32768, (row-18)*32768)`.
- **Every polygon is flat at the exact geodata height of its cells**:
  the rectangle growth only ever spans cells of one height, so the
  four corners carry that height and the surface is the square the
  raw l2j geometry holds - the port changes nothing about where the
  ground sits, the visual and the actual representation alike (the
  owner's porting directive). The bilinear vertex field of the
  earlier rounds (the average of the sheet's cells around each grid
  vertex, split-bounded by the 24 unit tolerance) smoothed the
  quantization staircase into interpolated surfaces; the staircase is
  the honest l2j answer and the mesh keeps it now. The same-height
  neighbors join seamlessly (identical edge heights on both sides),
  the height changes render as the genuine steps the geodata
  encodes.
- **The links are Detour-style chains** (`FirstLink` into a flat
  store, `Next` splicing): every link leaves through one rectangle
  side and carries `T0..T1` - the **inclusive cell range along the
  crossing axis where the geodata walls are open**. The funnel may
  only cross a shared edge inside this span: the NSWE wall fidelity
  the height-only import cannot express lands at cell-pair
  granularity, the improvement over both the research mesh (413k
  walled pairs it would connect) and the naive Detour import.
- **The external links** name their target by region key and polygon
  index; a missing neighbour tile resolves to a wall at runtime and
  re-stitches nothing (the link simply stays unused until the tile
  appears).
- **The BVTree** is the Detour `dtCreateBVTree` port: quantized
  bounds (cell units on the horizontal axes, 16 unit steps above
  -8192 on the height axis), median split along the longest axis,
  negative node index as the leaf escape. The tree holds exactly
  `2N-1` nodes.

The water semantics follow the grid engine: the polygons of the
sheets below the C1 water level (-3780) carry the water area; the
swim pricing is a 3x area cost (the `waterCostMultiplier`), the dry
searches exclude them and the escape prices them 8x. H-001 (the
breath gauge question) stays a hypothesis either way - the registry
entry is unchanged.

## The build pipeline

`internal/swarm/pathfind/navbuild` runs five steps per region:

1. **Parse and dedup** - the region file goes through the exported
   pathfind parser (`ParseRegionData`), the cell stacks flatten into
   the builder layout, and the within-32-unit duplicate layers merge
   (the l2j generator noise; the higher surface wins). The measured
   regions carry the same surface twice with a 0..32 unit jitter -
   both layers open, the lower copy often wall restricted (the 22_xx
   column counts ~54k such pairs per region; the elven region of the
   research round showed none, which is why the first round shipped
   the 16 unit rule that missed the 24/32 noise entirely). A real
   stacked floor never sits within 32 units of its ceiling, so the
   merge keeps every genuine deck: unmerged, the duplicates render as
   z-fighting polygon layers over the same ground.
2. **The sheet decomposition** - the walkable layers flood into 2D
   manifolds top down: the fill moves to neighbour column layers
   within the 40 unit climb of the same wetness class and never
   occupies a column twice. A sheet never stacks over itself, so the
   rectangle footprints never self-overlap - the property whose loss
   crashed the naive Recast import (the 137k vertex contour). Sheets
   smaller than 4 layers are unreachable islands (no neighbour
   within the climb range anywhere); the grid engine cannot reach
   them either. **The floating component drop** then rejects the
   islands no walk can EVER reach: the sheet graph joins two sheets
   when a layer of one admits an engine step into a neighbour layer
   of the other (the paired NSWE walls plus the climb rule - the
   exact `canStep` the grid search walks; the wetness split is
   ignored because the engine steps between the shore and the water
   freely), and a component survives only when one of its sheets
   touches the region border - the seam where the neighbour region
   may continue the walk. Everything else is the geodata structural
   encoding: the giant trees of the elven forest carry their canopies
   as stacked layers hundreds of units over the ground, the trunk
   helixes step within the climb range but wall every step of the
   way up, and the floating village decks skirt down towards the
   lake bed. No character can stand on any of it - the grid engine
   confirms it (a ground-to-canopy search arrives at the terrain
   under the tree, never at the branch height) - so the mesh must
   not pretend otherwise: keeping the canopy rendered the mother
   tree as a walkable ramp fused into the ground (the measured round
   dropped 1386 floating sheets / 60 118 layers from 21_19 alone,
   `navbuild/floating_test.go` pins the drop and the survival of the
   bridge deck, the village deck and the water).
3. **The rectangle decomposition** - every sheet splits into maximal
   rectangles of one exact cell height (extend right, then down).
   The growth respects the NSWE walls: a rectangle only spans cells
   whose mutual steps are open (the paired walls of both sides plus
   the climb height rule - `hStepOpen`/`vStepOpen` check every
   horizontal and vertical pair inside the growing rectangle), so
   the interior of every polygon is walkable by construction. The
   wall-blind growth of the first rounds let a single polygon
   swallow walled cell pairs (the town regions measured them by the
   million) and the corridor search tunnelled straight through the
   buildings; `navbuild/wall_test.go` audits every same-polygon
   neighbour pair of the real regions for the open-step contract.
   The growth respects the exact height the same way: two cells of
   a different height never share a rectangle, which is what pins
   the faithful square port (the staircase of the slopes decomposes
   into the per-height strips the geodata encodes). The honest
   growth multiplies the polygon count (21_19: 91k -> 373k) - the
   price of routes that respect the walls and represent the squares
   as they are.
4. **The links** - the adjacent cell layer pairs of the whole region
   accumulate the open portal spans: the pair needs the height
   difference within the climb AND the NSWE walls open in both
   directions (the `wallsOpen` rule of the grid search - the source
   wall in the step direction plus the target wall in the reverse
   direction). The cell pairs inside one polygon need no link. The
   span coordinates of every link key merge into maximal runs, every
   run is one portal. The region borders collect their kept layers
   into **strips** for the cross-region stitching.
5. **The tree and the wire format** - the BVTree builds over the
   polygon bounds and `EncodeTile` serializes everything.

The **phase B** stitches the region borders: the strip of one region
edge pairs against the opposite strip of its neighbour (the same
open + height + within-climb rules), the matches accumulate portal
spans into external links. `BuildPack` orchestrates both phases over
the whole pack with flat memory: phase A writes every tile
immediately and retains only the border strips (a value copy - the
field pointer would pin the whole build), phase B re-decodes only
the tiles whose neighbours exist.

## The measured reality

All numbers from the real elven village region `21_19` (4.4M cell
layers, 186k stacked columns) on the sandbox:

| Metric | grid A* (current) | Detour C++ (research) | this port |
|---|---|---|---|
| the hard bridge pair (village -> water under the bridge) | 5.17 s, 500k nodes | 169 us | 219 polys + funnel, 14.9 ms warm, 24.8 ms cold (the decode included) |
| 200 random region pairs | 129/200, avg 2.94 s | "200/200"*, avg 339 us | 128 full + 61 partial = 189/200 (the pre-exact-port replay; the exact mesh answers the same corridors over more polygons) |
| the region data at runtime | ~20 MB parsed, 140 ms load | 2.89 MB tile | 44.2 MB tile, 26.4 ms decode |
| the offline build | n/a | 5.1 s per region | 2.7 s per region, 372 846 polys |
| the pack build (165 regions) | n/a | ~14 min estimated | 4m25s, 1.9 GB of tiles pre-exact-port; the exact square port multiplies the tile bytes ~3.4x (the four dense measured regions hold 287.7 MB) |

\* the research number counted `DT_PARTIAL_RESULT` as success - the
honest split is 128 full corridors + 61 closest-reachable partials +
11 isolated starts (island surfaces no walk leaves; the C++ audit
reported the first 8 as separate components, the wall-honest
rectangle growth of 2026-09-16 unmasked the last 3 whose corridors
used to tunnel through the walls a single polygon swallowed - the
grid engine answers every one of the 11 with its own clean not
found). The 61 partials are the correct Detour behavior for
unreachable targets under the filter, not misses.

The 14.9 ms warm hard-pair number sits ~90x over the C++ Detour: the
Go runtime resolves every link target through the tile map (a
mutex-guarded LRU) where Detour dereferences flat arrays, and the
nearest-poly query allocates its candidate buffer. The exact square
port multiplied the polygon and link counts (every height run edge
of the quantized staircase is a polygon boundary with its portal),
which roughly doubled the warm route time of the bilinear rounds -
the honest price of representing the l2j squares as they are. The
pooled query state (one node array + one heap per call) keeps the
allocations low, the cold route adds the lazy tile decode (~26 ms
per 44 MB tile), exactly what a cold bot pays. The corridor itself
is 219 polygons. This is still two orders of magnitude under
the grid engine's 5.17 s on the same pair, and the fleet-scale plan
(~2.9 s of route queries per bot-minute during hunts) fits the 100
bot budget comfortably; the Detour-parity optimization (the flat
tile array, the per-tile poly index) is future headroom, not a
blocker.

The `Route`/`RouteDry`/`WaterEscape` contracts mirror the grid
engine's `FindPathApproach` family: the stacked-layer
disambiguation test (the same x/y, the deck vs the water under it),
the dry partial with the closest reachable dry point, and the
priced water escape all pass on the real mesh
(`navbuild/real_test.go`).

## The corrupt regions of the pack

Seven region files of the shipped pack fail the l2j parse. The
2026-09-18 round diagnosed and fixed six of them: `17_20..17_25`
ship as **multi region concatenations** - the named region stream
followed by appended neighbouring sea regions and a truncated
fragment (the block walk of `scripts/geo_diag` proves the first
stream parses clean). `scripts/geo_repair` truncated each file to
its first valid region (the originals ride beside as
`X_Y.l2j.orig`, git ignored) and the Gludin coast pack builds:
1 517 579 polys, 1 744 external links. `16_10` stays corrupt mid
file (invalid layer count 0 at offset 6 291 459 - the far south
west ocean, no gameplay); the pack covers 164 of 165 regions.

The pack border audit (`TestPackBorderAudit`) walks every ordered
region pair and reports the asymmetric or all dead borders: 484
pairs, 0 asymmetric after the repair round. The audit only counts
the pairs WITH links - the zero link borders are the geodata seams
and holes below.

### The geodata holes and seams (the town route blocker)

The 2026-09-18 town route round found the pack's geodata **leaves
the open water unmapped** in patches: the Gludio bay and the inner
bay between Gludio and Dion hold no layers in the shipped l2j
files, the mesh build answers with holes, and every straight line
aim into them fails to bind (`TestBayNorthShoreProbe` names the
coordinates). The tile borders step where the coverage differs:
the 18_21/17_21 border pairs land heights 291 distinct (land,
-3700..-1200) against one flat sea level - over the 40 unit climb
the stitch refuses them honestly (TestSidecarPairProbe). The
consequences for the trip planning:

- the one shot routes whose straight line crosses the holes strand
  (Gludio->Gludin, Gludin->Giran, Gludio->Giran answer partial);
- the honest shoreline detours exist on the mesh but the coarse
  guide's sampled crossings and the confined fallback miss them
  (the re-path walk of `TestNavmeshTownRoutes` covers 58 852 of
  73 199 units in 3 replans before stranding);
- the fix belongs to the data: a geodata refresh with the water
  blocks filled (the far sea regions 17_23..17_25 prove the format
  carries them - full flat sheets at sea level), or an operator
  waypoint graph for the bay crossings.

## The hierarchy (the HNA* port)

The cluster graph of docs/pathfinding.md runs the two phase query
(docs in code: hierarchy.go): the coarse A* over the cluster
blocks answers the portal chain, the refinement threads the real
mesh hop by hop through it (the hop corridors cache - the HNA*
intra-edge answers), and the confined fallback threads the chain
clusters on the real mesh when the sampled crossings strand.

- **The coarse guide** prices the edges uniformly (no water
  multiplier): the guide only points the direction and the uniform
  price keeps the plain dist3 heuristic consistent - the swim
  priced guide flooded half the map (the honest water route costs
  3x the straight line and the plain A* cannot prune the land wave).
- **The component gate**: the first (gated) attempt verifies the
  cluster connectivity through the per tile link components (the
  union find of abstract.go) - the chain cannot fake a passage
  between two island components of one cluster. The gate rides the
  4096 pop cap and the f spread bound; a stranded gated attempt
  hands over to the **gate free plain HPA\*** chain whose honesty
  the refinement hops and the confined fallback restore (the
  measured pairs where the sampled crossings strand: the bay legs).
- **The caches**: the tile LRU (4 tiles default), the abstract LRU
  (32 regions - the comps array costs 4 bytes per polygon and the
  whole pack sums into the hundreds of megabytes), the hop corridor
  cache (4096 portal pairs). The abstract rebuilds are
  deterministic (the edge indices stable, the dstComps and the ban
  bookkeeping survive).
- **The escalation**: the same tile queries stay flat (the within
  tile measurements answer the flat search faster and with the
  shorter corridors); a flat search that caps at maxQueryNodes
  escalates to the hierarchy (RouteApproach).

The measured answers (the sandbox, the real pack):

| query | flat | hierarchical cold | warm |
|---|---|---|---|
| 21_19 short 1.5k | 0.85 ms, 1951 nodes | 0.97 ms | 0.97 ms |
| 21_19 medium 16k | 4.9 ms, 8983 nodes | 9.1 ms | 4.4 ms |
| 21_19 diagonal 21.5k | 15 ms, 19392 nodes | 1.61 s | 5.1 ms |
| the owner diagonal 20_19..21_20 swim | caps into partial | 3.27 s, full | - |
| Elven Village -> Gludio 93k | caps into partial | 10.8 s, full | 6.1 s |

## The tooling

```bash
# build the whole pack (idempotent, mtime based):
go run ./cmd/navmesh-build -geodata data/geodata -out data/navmesh

# build selected regions:
go run ./cmd/navmesh-build -regions 21_19,22_19 -out /tmp/navmesh

# the runtime and builder suites (the real-region tests skip
# themselves without the geodata pack):
go test ./internal/swarm/pathfind/navmesh/ ./internal/swarm/pathfind/navbuild/ -v

# the benchmarks (build, hard pair, nearest poly, decode, escape):
go test ./internal/swarm/pathfind/navbuild/ -run '^$' -bench BenchmarkReal -benchmem
```

## The mesh viewer

The `-show-navmesh` launch mode serves a bot less 3D inspection
surface for the built tiles (the same embedded web interface the
bot control uses, no game connection):

```bash
# every tile of the directory stitched together:
go run ./cmd/swarm -show-navmesh

# one named tile (comma separated keys also work):
go run ./cmd/swarm -show-navmesh=21_19

# an explicit tile directory and port:
go run ./cmd/swarm -show-navmesh -navmesh data/navmesh -web 127.0.0.1:8080
```

The viewer renders the rectangle polygons as an exaggerated terrain
(height ramp for the ground, depth ramp for the water - the geodata
water class is everything below the C1 water level, so the blue
deepens with the riverbed instead of faking a flat surface at a
height the ground never held; the height scale selector lifts the
subtle geodata relief to 2x/4x). The render carries the honesty
rules the rounds taught it: the logarithmic depth buffer keeps the
32768 unit tiles from z fighting at the viewing distances of the
stitched world, the balanced light rig (an ambient floor plus a
hemisphere, a sun and a counter fill - the floor keeps the steep
cascade quads of the l2j slope smoothing cells readable instead of
black, because a steep quad tessellates into two triangles whose
flat normals face apart and the away-facing half would fall to
black without it) keeps every face above the darkness, the quad
tessellation splits every surface polygon along the corner 1-2
anti-diagonal (the wire corners are row-major, so the two triangles
are (0,2,1) and (1,2,3) - the original (0,2,1)+(0,2,3) pair
anchored both triangles on the shared 0-2 edge and left the right
quarter of every polygon see-through, the recurring black-triangles
report: 12.11 percent of the reported view fell to the clear color
before the fix, 0.03 after), and the
merged duplicate layers of the 32 unit dedup leave no stacked
surfaces to flicker. The voids that remain are honest: the island
sheets under the four-layer minimum (4301 of them in 21_19, 5624
layer instances) and the fully blocked cells render as the clear
color - the coverage audit (navbuild coverage_test.go) proves every
walkable layer of every kept sheet carries a polygon. A double
click on the mesh arms the green start marker, the second double
click picks the destination and asks the server for the real
corridor search - the same `Route` call the hunt loop's hybrid
navigator issues - and the answer draws the funnel polyline with its
waypoints plus the measured construction time. The timer is the
server side `time.Since` around the query:
the first route over a cold region honestly includes the lazy tile
decode (~1.45 ms per tile), exactly what a cold bot pays. The elven
hard pair (the village deck at (45768, 49848, -3056) to the water
under the bridge at (44920, 50792, -3928)) answers in single digit
milliseconds where the grid engine floods for 5.17 s - the viewer is
the fastest way to see the stacked-layer walk the port bought.

The inspection surface answers the three questions a route debug
session asks. Which square am I looking at: every loaded tile draws
its region grid outline above the geometry, the outline under the
cursor lights up amber, and the readout bar at the top names the tile
with its region local cell (a progressive raycast sweep - one tile
per frame, the nearest bounding sphere first - so the answer tracks
the pointer without ever blocking the flight). Where does the route
run: the result panel carries the from/to rows with their tile keys
and world coordinates, and every waypoint wears a label with its
index and coordinates. Where may a route cross between polygons:
the `edge connections` toggle draws the real link portals of the
mesh - the open spans the NSWE walls leave - as small colored
segments riding the surface, green for the field to field
connections, blue for the water to water ones, teal for the shore
pairs and gray for the links into tiles that did not resolve (the
legend carries the four swatches). A long shared edge between two
rectangles shows its gates, not its full length: the portal span
is the honest answer the funnel respects.

The camera is a flight rig (the owner request replacing the orbit):
WASD flies along the full view vector - W follows the pitch like an
airplane - Q and E descend and climb, the pointer drag yaws and
pitches, Shift boosts 4x and the wheel retunes the cruise speed (the
panel carries the speed readout; the keys sit on the window so the
canvas focus never matters, and the form fields keep their own
typing). The framing and the restore write the euler angles
directly under the YXZ order with the roll pinned at zero: the
original `lookAt` framing left a z angle behind under the default
XYZ order and the first frames read it as a rolled horizon - the
owner's tilted-view report of the solid surface round.

The feedback channel closes the loop between the owner session and
the agent session (the owner request: reproduce the exact view
locally and let the route answer itself). The `copy view link`
button freezes the whole view state into one URL - the camera pose
(world x, y, height, yaw, pitch), the armed or answered route pair,
the visible tile selection, the route filter and the height scale -
and copies it to the clipboard (the link field itself always holds
the URL for manual selection where the clipboard API is
unavailable). A paste of that URL boots the viewer into the exact
view: the query parameters restore the camera, the tiles, the filter
and the scale, and the route pair re-runs automatically - the result
panel, the polyline and the markers rebuild on their own. The
parameters live in `navmesh_view.js` (`parseViewParams` is the boot
half, `buildViewStateUrl` the copy half) and
`TestNavmeshViewScriptContract` pins them against drift.

The flag selection bounds the initially VISIBLE tiles only: the
route queries always run over the full directory mesh, so a path may
leave the visible tiles (the checkbox list loads more tiles on
demand, the polyline draws wherever it walks). The swim/dry filter
select mirrors the hunt loop's two search profiles (water priced 3x
versus walled), and the `tiles=` parameter of a shared link
overrides the flag selection the same way.

The endpoints behind the page (the mode of `GET /api/config` is
`navmesh`):

- `GET /api/navmesh/tiles` - the tile file listing with the derived
  world footprints, no tile decoded;
- `GET /api/navmesh/geometry/{col}_{row}` - the binary NMV2 payload
  of one tile (the encoder lives in navmesh_geometry.go): a 32 byte
  header (magic, region key, world anchors, the height range, the
  poly, link and wall counts), then a contiguous int16 corner block
  (four corners per polygon, the corner order X0Y0 X1Y0 X0Y1 X1Y1,
  each a cellX/cellY/height triple - the world position is
  worldMin + cell*16), the area bytes (0 ground, 1 water), the link
  portal records and the height step wall records. The link portals
  carry the world span of every connection with the area class of
  the pair (the edge connections overlay); the walls carry the
  vertical filler quads between the flat surfaces of adjacent
  rectangles. Every polygon is flat at its exact cell height, so the
  same-height neighbors agree about the shared edge byte for byte
  (no filler there, the surfaces join seamlessly) and the filler
  closes exactly the genuine height steps of the geodata - the
  quantized staircase of the slopes renders as the steps it is, the
  same picture the original cells render draws. Before the exact
  square port the corners came from the bilinear vertex field: the
  interpolation smoothed the staircase (the shingled roof sheets the
  owner first reported) and the filler closed the surfaces the
  interpolation left disagreeing. The owner's porting directive
  pinned the faithful squares and both the smoothing and the
  disagreement went away with it. The filler is height capped: the
  honest crack scale is the 40 unit climb of the linked surfaces,
  and a wall may never close more than 80 units -
  a taller step between two surfaces is not a crack but the open air
  between two separate worlds (the floating deck over the lake, the
  tree canopy over the ground), and the void is the honest answer
  there. The uncapped wall of the first rounds curtained the
  floating village down to the water and grew grey stalagmites
  under every branch. The
  walls emit through the MaxX/MaxY sides only, so every shared edge
  is walled once, and the region borders resolve their targets in
  the east and north neighbor tiles through the mesh. The triangles
  never ride the wire: every polygon is its own quad of four
  consecutive corners and the viewer tessellates. The payload is
  immutable per tile, so an ETag revalidates for free and the
  server caches the encoded bytes;
- `POST /api/navmesh/path` - `{start, end, filter}` positions and
  the reply `{found, partial, waypoints, durationMs, explored,
  corridor, filter}` of the measured `Route` call. When the capsule
  clearance arms the shortcut pass the reply carries the two variants
  of the walk: `waypoints` is the smoothed answer (the merged chords)
  and `rawWaypoints` is the raw funnel answer it merged - the viewer
  route variant toggle draws one or the other from one search (the
  `path=smooth|raw` view link parameter restores the pick).

## The shortcut pass (the smoothing)

The funnel on the exact square mesh pivots at every portal the
clearance shrinks into a pinhole: a 16 unit portal loses 7.5 off both
span ends, one unit of window stays, and a long walk turns at every
height run edge - the walker micro steers through a hundred plus
waypoints and hooks on the dense turns (the owner report with the
walk plan aiming at wp 25). `Filter.Smooth` arms the shortcut pass
(smooth.go): the greedy farthest visible merge walks the corridor and
folds the funnel waypoints into the longest chords that (a) cross
every intermediate portal inside its open span - the corridor
fidelity, no chord leaves the polygon chain - and (b) keep the
WaypointClearance radius from every wall edge of the crossed
polygons - the walls are the side portions without a link, the same
closed edges the capsule clearance pass pushes away from. The merged
answer keeps the pivots no safe chord skips, so it never adds a turn;
the raw funnel answer rides along in `Route.RawWaypoints` for the
comparison. The open spans of the mesh are whole geodata cells
(16 units), so every span holds a crossing a 7.5 capsule clears and
the pass never dead ends.

## The wall oracle (the LegGuard seam)

The mesh wall spans are the side level approximation of the walls:
the grid movement validation sees the per cell walls (the paired
NSWE walls and the diagonal anti corner cut) that a rectangle side
lumps together, and a funnel pivot one radius off a mesh span end
can still sit a few units off a grid wall on the staircase terrain.
`Filter.Guard` arms the wall oracle (the `LegGuard` interface - a
`LegClear(ax, ay, az, bx, by, bz, radius)` answer for one straight
leg): the shortcut pass then asks the guard about every chord before
the mesh spans, and the grid capsule of the caller
(`pathfind.Capsule.LegClear` - the line of sight the movement channel
applies plus the 4 unit clearance sampling of the bend pass) is the
authority. The composition closes at the caller: the walker answer
runs the capsule push and bend pass (`ApplyPath`) and folds into the
longest grid clear legs (`ShortenPath`) - every leg the bot consumes
answers the server movement rules with the capsule clearance. The
owner repro route (21_19 swim) walks 14 waypoints of the legacy 148,
the path length drops 4947 to 4852, and the raw variant of the
viewer toggle keeps the legacy pipeline as the before picture.

Three.js itself is vendored (`web/vendor/three.module.min.js`, the
r160 module build, MIT) so the viewer works offline like the rest of
the interface; the page boots through the dynamic import of
`main.js` on the `navmesh` mode. The geometry of the full stitched
pack is a few hundred megabytes of GPU buffers - the viewer targets
the working set of a handful of regions, not the whole 165 tile
pack at once.

## The live integration

The hunt loop consumes the mesh since the live integration round:
`hunt.NewNavmeshNavigator` (internal/swarm/hunt/navmesh_navigator.go)
installs the hybrid behind the same `Navigator` seam the pure grid
engine navigator used - the town trips, the zone returns, the quest
trips and the user walks plan their routes without knowing which
engine answered.

- **The route queries serve from the mesh**: FindPathApproach, both
  avoiding forms (the frozen corridor bans of the session convert
  into mesh ban disks), FindPath and FindWaterEscape run the corridor
  search with the approach radius goal (the polygon-granularity form
  of the grid nodeReached) and answer the funnel waypoints as the
  pathfind.Result contract of the seam.
- **Every answer the mesh cannot serve falls back to the grid
  engine**: a missing tile under an endpoint, ground the sheet
  decomposition dropped, a sealed goal under the bans. The grid
  engine stays the reachability authority - the hybrid can only ADD
  routes (the milliseconds of the mesh corridor instead of the
  seconds of the grid flood), never lose them.
- **The partial corridors surface through Result.Partial (the
  partial round)**: when the mesh answers a closest-reachable
  corridor (the destination unreachable under the filter) and the
  engine CONFIRMS it with its own clean not found, the avoiding
  forms serve the mesh funnel waypoints with Found=false and
  Partial set - the town legs and the quest segments walk toward the
  closest reachable point instead of aborting at the start position
  (the shore of a swim-only destination, the border of the sealed
  corridor). The engine run comes FIRST on every mesh partial, so a
  full route the mesh missed still wins, and an engine error still
  surfaces as the honest verdict; the strict forms
  (FindPathApproach, FindPath) never surface partials - the blind
  engage recovery and the user walks keep their verdict semantics.
  The production profile of an unreachable destination is unchanged
  (the mesh milliseconds plus the one engine flood the fallback
  already paid); trusting the mesh verdict without the engine
  confirmation is the acceptance-switch round's headroom.
- **The validation layer never leaves the grid engine**:
  ValidateClick (the click guard of every walked leg), the sight
  lines, the water rasters and the deck heights stay on the raster
  the server itself walks. The mesh plans, the raster validates -
  the division the acceptance stack of the town trips already
  enforces leg by leg.
- **The recovery bans wall the mesh at rectangle granularity**: a
  polygon whose footprint a ban disk touches walls the corridor
  search (the over-walling direction - no funnelled leg ever enters
  the banned ground, where the grid only keeps the cell centers of
  the smoothed legs out), the ban holding the start opens its escape
  ring within 256 units of the start at the 6x multiplier, a foreign
  ban wins over the escape ring. The ban disks come from the same
  freeze reports the grid bans come from - one report, two engines,
  the same detour.

The runtime wiring (cmd/swarm/main.go): the `-navmesh` flag names the
tile directory explicitly, the empty value autodetects
`data/navmesh` (the documented output of the build command); a
directory without tiles keeps the plain engine navigator without a
word of noise. The geodata engine and the mesh share the process and
survive the reconnects; the mesh LRU holds 32 tiles (~a route
neighborhood) by default.

## What is NOT wired yet

The acceptance stack still installs the pure engine navigator (its
regression scenarios pin the click validation machinery of the grid
engine); the mesh serves the live hunt loops only. The partial round
serves the closest-reachable waypoints through the Navigator
contract, but the hybrid still runs the engine confirmation before
every partial - skipping that flood (trusting the mesh verdict) is
the acceptance-switch round once the live sessions prove the hybrid.
The Detour-parity optimization headroom (the flat tile array, the
per-tile poly index) stays future work until the fleet benchmark
asks for it.
