# Recast/Detour research: what a navmesh gives the geodata pathfinder

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The research round of the `feature/new-pathfind` branch: an
evaluation of
[recastnavigation](https://github.com/recastnavigation/recastnavigation)
(Recast builds navigation meshes from voxelized geometry, Detour
answers queries over them) as the replacement for the grid A* of
`internal/swarm/pathfind`. Everything below was measured with the
runnable experiments of `research/recast/` (upstream pinned at
9f4ce64, built with the plain sandbox g++, upstream tests 33 cases /
5000 assertions green) and the Go prototype of
`internal/swarm/pathfind/navmesh`. The hard case that motivated the
round: a route from the elven village to a point under the bridge -
on the water below the floating village - where the x and y match a
walkable deck column but the z sits hundreds of units below it.

## The answer in one paragraph

The Detour **query** model is exactly what the swarm pathfinder
needs: stacked layers at the same (x, y) are first class citizens
(nearest polygon resolves by 3D distance, the A* walks polygons
across surfaces), water becomes a priced area instead of a wall of
crutches, and the measured query cost on the real elven village
region is **microseconds against seconds** (169 us vs 5.2 s on the
hard bridge pair, 339 us vs 2.9 s average over 200 random regional
pairs - the grid engine aborted 39 of those 200 at the expansion
cap). The Recast **build** pipeline however does not fit the l2j
geodata as-is: it structurally assumes one walkable surface per
region, and the elven lands stack surfaces that additionally connect
within the climb range (terraces, interior floors, ramps under
decks) - the naive import crashes with a 137k vertex contour. The
working converter needs a custom sheet decomposition (implemented
and measured here). The recommendation: port the Detour **runtime**
into Go (the prototype already answers real queries on the real
mesh) fed by a custom Go mesh builder that starts from the geodata
cells, and keep the grid engine as the click validation and local
walk layer it already is.

## The measured comparison

| Metric | grid A* (current) | Detour C++ | Go prototype |
|---|---|---|---|
| hard pair: village to water under the bridge | 5.17 s, 500 458 nodes | 169 us, 38 polys | 571 us |
| 200 random pairs across region 21_19 | 129/200 found, 39 aborted, avg 2.94 s, 284k nodes | 200/200, avg 339 us | 163/200, avg 915 us |
| data held per region at runtime | ~20 MB parsed region | 2.89 MB tile | 2.89 MB tile |
| load cost per region | ~140 ms parse | - (built offline) | 1.45 ms tile parse |
| region cache pressure | LRU of 4 thrashes on long routes | all 166 tiles resident is possible (~500 MB upper bound) | same |
| world scale (Gludin to Aden) | 25.2 M nodes, 106 s, only with the cap raised | the corridor is hundreds of polys | same |

The grid engine numbers come from `TestNavmeshPairReplay`
(`internal/swarm/pathfind/navmesh_replay_test.go`) replaying the
exact pairs the C++ experiment captured
(`research/recast/results/query_pairs_21_19.txt`); the Detour
numbers from `research/recast/results/geodata_navmesh.txt`; the Go
prototype numbers from the tests and benchmarks of
`internal/swarm/pathfind/navmesh`.

## Experiment A+B: the synthetic floating village

`research/recast/experiments/bridge_water.cpp` builds the elven
village topology in miniature: a mainland plateau, a shore ramp
descending under the water level, a lake bed, an island deck riding
288 units above the bed, and a bridge strip connecting the mainland
to the island over the water. The results
(`results/bridge_water.txt`):

- **Same (x, y), different z resolves by 3D distance**: the query
  point under the bridge binds to the WATER polygon, the identical
  x/y at deck height binds to the bridge GROUND polygon, a mid
  height point picks the nearer surface. This is the core of the
  hard case: the server-side "which floor did you mean" question the
  grid engine answers with the target layer resolution
  (`getHeight(tx, ty, tz)` semantics) and the layer poisoning fixes.
- **The full route exists and is honest**: village deck -> bridge ->
  mainland -> shore ramp -> water -> under the bridge target, 19
  polygons, the water legs priced 3x exactly like
  `waterCostMultiplier`. The same route with the swim area excluded
  answers the partial path to the closest dry polygon - the honest
  "you cannot get there dry" the hunt loop needs (the current engine
  implements it as `FindPathApproachDry` returning not found).
- **The water escape is an ordinary query**: from the water under
  the bridge back onto the deck with the water priced 8x - the
  `FindWaterEscape` breadth first flood of the current engine
  becomes a plain findPath with an area cost.
- **The line of sight survives**: the raycast from the island center
  toward the mainland hits the cliff edge and reports the wall
  normal - the `LineOfSight`/`DryLine` equivalents keep their
  semantics.
- Query cost on the 88 polygon synthetic mesh: 3.7 us per
  findPath+findStraightPath, build 10.8 ms.

**The porting hazard this experiment surfaced (the most valuable
finding of the round)**: when two stacked walkable surfaces are
additionally within `walkableClimb` of each other at a lateral edge
(the first draft had the bridge deck 2..4 voxels above its own
ramp), the Recast region flood fill merges them into ONE region
whose 2D footprint self overlaps. The traced contours come out
mangled ("multiple outlines"), the polygon link graph breaks with
silent one way links, and the water becomes unreachable although the
span graph is connected. The fix in the synthetic world was a bridge
abutment (the real village keeps decks and beds hundreds of units
apart); the real geodata still contains the pattern - see the sheet
decomposition below.

## Experiment C: the real geodata converter

`research/recast/experiments/geodata_navmesh.cpp` converts the real
elven village region `21_19.l2j` (2.9 MB file, 4 400 272 cell
layers, 186 723 stacked columns, 436 561 layers below the water
level) into a Detour tile. The naive "every layer becomes a span,
water below -3780 becomes a water area" import **fails** on the real
data with exactly the hazard above: `rcBuildRegions` logs 7335
overlapping regions, `rcBuildContours` logs "multiple outlines",
`mergeHoles` fails and `rcBuildPolyMesh` dies on a 137 069 vertex
contour. The working pipeline:

1. **Parse** the l2j format exactly like `region.go` does (flat,
   complex, multilayer blocks; height = sign extended
   `(short)(word & 0xFFF0) >> 1`). One trap found the hard way: the
   block order is x strip major (all y blocks of one x strip first),
   so neighbouring cells sit far apart in the layer array - a per
   cell (offset, count) layout is mandatory, a prefix range layout
   silently scrambles the data (the Go engine stores self contained
   packed spans for the same reason).
2. **Dedup** near identical layers within one cell (within 16
   units). The shipped pack turned out clean (1 duplicate in the
   whole region) but the guard stays: the known l2j generator noise
   would otherwise poison the sheet decomposition.
3. **Sheet decomposition**: partition the walkable layers into 2D
   manifold sheets - a flood fill from the highest unassigned layer
   that moves to neighbour column layers within the 40 unit climb of
   the same wetness class and never occupies a column twice. Sheets
   are the surfaces; a sheet is never stacked over itself, so the
   region flood fill never sees a self overlapping footprint again.
   The region: 5 778 sheets (12 water class), the largest holding
   3.4 M layers. Sheets smaller than 4 layers are dropped - they are
   islands no walk can reach (no neighbour layer within the climb
   range anywhere), so the grid engine cannot reach them either:
   4 303 islands, 5 626 layers.
4. **Color the sheets** with area ids through greedy graph coloring
   of the sheet adjacency (ground 47..62, water 31..46): column
   adjacent sheets never share an area, so the Recast region build
   keeps them apart and their shared cell borders become real
   polygon links.
5. **Build** the Recast pipeline (compact heightfield with
   walkableHeight of 1 voxel - the L2 movement validation has no
   ceiling rule - and the 40 unit climb, distance field, watershed
   regions, contours with the demo simplification error 1.3,
   poly mesh, detail mesh) and validate the produced links against
   the height rule (links between vertically stacked surfaces die).
6. The result: **14 062 polygons (525 water), 23 149 vertices, a
   2.89 MB Detour tile, built in 5.1 s** (offline, once per region;
   166 region files ≈ 14 minutes single threaded). The link graph audit:
   0 one way links, the largest connected component holds 10 630 of
   the polygons, the rest are genuine cliff terraces.

### The queries on the real mesh

The bridge finder scanned 1 560 deck over water columns and picked
the one closest to the village dump cell (45768 49848 -3056): deck
z -3048 over water z -3928 at (44920, 50792), a clean 880 unit stack.

- The disambiguation holds on real data: the under bridge point
  binds to the WATER polygon (nearest z -3920), the same x/y at deck
  height to the GROUND polygon (nearest z -3036).
- **Village -> under the bridge, swimming allowed (water 3x)**: full
  38 polygon corridor, 13 straight path corners - the route descends
  the western shore into the lake and swims to the target; 169 us.
- The dry filter: partial answer with the closest reachable dry
  point (44916, -3048, 50817) - the bridge deck right above the
  target.
- The reverse water escape: full 39 polygon route back onto the
  deck.
- 200 random region wide pairs: 200/200 routable at 339 us average.

### The fidelity audit (what the height only mesh cannot see)

The NSWE walls are not geometry: the mesh import connects two
neighbour layers whenever their heights are within the climb range,
and the audit counted **413 692 neighbour pairs in this one region
where the NSWE flags block a passage the mesh would allow**. That is
the invisible wall gap of the pure navmesh route - the plans can
cross walls the server refuses. Three mitigations, all compatible
with the architecture the bot already has:

1. The plan level guard stays: every smoothed leg is already
   verified against the ported server click validation
   (`Engine.ValidateClick`) before the walker sends it - a navmesh
   plan crossing an invisible wall dies at the same gate a bad grid
   plan dies at today.
2. The runtime ban memory stays: the frozen corridor avoid areas of
   the hunt loop are exactly the mechanism that learns server side
   walls the data does not model.
3. A production builder can filter the poly links by the NSWE rule
   of their cells (the same cell pair recovery the link height
   validation of the experiment uses) - the wall fidelity then
   survives at the link level and the 413k gap closes to the walls
   interior to single polygons.

## The Go port: prototype and effort

`internal/swarm/pathfind/navmesh` is a pure Go Detour **runtime**
prototype: it parses the exported tile
(`navmesh_21_19.bin`, the exact dtCreateNavMeshData layout with the
links rebuilt from the polygon neighbor fields like
`dtNavMesh::connectIntLinks`) and answers the queries on the real
mesh:

- `ParseTile`: 2 891 246 bytes in 1.45 ms (vs the ~140 ms l2j region
  parse of the grid engine - the runtime data is 96x cheaper to
  load).
- `FindNearestPoly`: the BVTree walk plus the 3D closest point - the
  stacked layer disambiguation test passes on the real bridge
  column.
- `FindPath`: the Detour A* (edge midpoint node positions, averaged
  area costs, 3D heuristic) - **the hard bridge pair answers in
  571 us** (5 allocations: the per query node array is the known
  headroom; a pooled version sits under the C++ number).
- `Corners`: the simplified string pulling (edge midpoints; the
  production port carries the full funnel algorithm).
- The 200 pair replay: 163/200 routable at 915 us average. The 37
  misses are the height approximation of the prototype's
  `closestPointOnPoly` (the polygon fan interpolation instead of the
  detail mesh): the nearest poly resolution occasionally binds a
  stacked point to the other layer. The production port needs the
  detail mesh heights (or exact per vertex heights from the geodata
  cells - a custom builder gets them for free, see below).

The effort estimate for the production port, grounded in the
measured source sizes (DetourNavMeshQuery.cpp 2 600 lines,
DetourNavMesh.cpp 1 600, DetourNavMeshBuilder.cpp 800,
DetourCommon.cpp 250 - the C++ port surface) and in what the
prototype already does:

| Piece | Est. Go LOC | Notes |
|---|---|---|
| tile parse + links + BVTree | ~700 | done in the prototype (470), production flattens the adjacency |
| query: nearest poly, A*, straight path (funnel), filters | ~1 500 | the prototype has the A* and the nearest poly; the funnel and the sliced query remain |
| multi region tiles + external links | ~600 | dtNavMesh::addTile/connectExtLogs equivalent, 165 region tiles |
| **the custom mesh builder** (sheets, quad merge, NSWE link filter) | ~1 500 - 2 000 | replaces the whole Recast build: the sheets are computed from the geodata directly (the C++ experiment proves the semantics), merged into polygons per surface |
| tooling: offline builder cmd + audits + regression tests | ~800 | the bridge/water/escape suites as Go tests |
| total | **~5 000 - 5 600** | one focused week for the runtime, one to two for the builder and the audits |

The custom Go builder is the better builder for this data anyway:
the geodata is already an exact voxel surface, so the
rasterization/filtering half of Recast (the part that turns messy
triangle soups into spans) has nothing to contribute - while the
region/contour half actively fights the stacked surfaces. A Go
builder that partitions layers into sheets (the proven algorithm)
and emits polygons per sheet keeps the NSWE walls as link
attributes from the start, gets exact per vertex heights (no detail
mesh), and removes the last C++ dependency of the round.

## What the transition gives, crutch by crutch

| Current crutch | Navmesh equivalent |
|---|---|
| `waterCostMultiplier` 3x per underwater cell | water polys carry a swim area cost (the same 3x, per area not per step) |
| `FindPathApproachDry` walling the water | a filter that excludes the swim areas; unreachable targets answer the partial path with the closest dry point |
| `FindWaterEscape` breadth first flood | an ordinary findPath with a high water area cost |
| `legDry` smoothing rule (the ford regression) | the funnel never leaves the poly corridor; water legs stay water legs |
| layer poisoning fix (`nodeKey` = cell + height) | stacked layers are separate polygons by construction |
| the 1M expansion cap and its aborts | the corridor is hundreds of polys; world routes stop costing millions of nodes |
| region LRU thrash (4 regions, 140 ms misses) | 2.89 MB tiles, 1.45 ms loads, all resident feasible |

What stays: the click validation port (the plan level guard), the
avoid area memory, the walker itself, and the geodata files (the
builder consumes them, the runtime does not).

## Verdict and migration path

1. **Do not port Recast.** The build pipeline mismatches the data
   structurally (the self overlapping region disease), and the
   geodata is already voxelized - the value of Recast is its query
   model, not its rasterizer.
2. **Port the Detour runtime to Go and build the mesh in Go** from
   the geodata with the sheet decomposition (the algorithm is proven
   by the C++ experiment; the prototype proves the runtime). The
   production shape: an offline `cmd` builds region tiles into
   `data/navmesh/`, the bot loads tiles lazily at 1.45 ms each and
   answers every long route query in microseconds.
3. **Keep the grid engine as the validation and local layer**: the
   click validation, the avoid bans and the short range walks stay
   on the cell grid - the navmesh feeds them the corridor.
4. The water semantics of H-001 stay a hypothesis either way: the
   navmesh prices swimming exactly like the grid engine does today
   (a constant multiplier); the live swim experiment the registry
   plans will calibrate both engines equally.

## Reproducing

```bash
cd research/recast
bash build.sh        # clones upstream at 9f4ce64, g++ only
./build/bridge_water | tee results/bridge_water.txt
./build/geodata_navmesh | tee results/geodata_navmesh.txt
# the Go prototype on the exported tile:
go test ./internal/swarm/pathfind/navmesh/ -v
# the grid engine replay:
go test ./internal/swarm/pathfind/ -run TestNavmeshPairReplay -v
```
