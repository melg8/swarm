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
decomposes into rectangle polygons with exact corner heights, and the
polygon links carry the open NSWE portal spans of the shared edges.
The runtime (`internal/swarm/pathfind/navmesh`) loads the tiles
lazily and answers the queries over them: the 3D nearest polygon
resolution (the stacked-layer disambiguation), the A* corridor
search with the water area pricing, the funnel string pulling and
the water escape. No C++ anywhere: the Recast build pipeline is
replaced by the sheet decomposition the research round proved.

## The tile format

One tile is one region: a 40 byte header (the magic `SWN1`, the
version, the region key, the climb, the section counts), the polygon
section, the link section, the external link section and the
bounding volume tree - every section 4 byte aligned, little endian.

- **The polygons are rectangles in region local cell bounds**
  (`X0 <= cx < X1`, `Y0 <= cy < Y1`, half open). The world rectangle
  spans the grid vertices `X0*16 .. X1*16` anchored at the region
  origin `((col-20)*32768, (row-18)*32768)`.
- **The corner heights are exact**: `H00` is the geodata height of
  the cell `(X0, Y0)`, `H10` of `(X1-1, Y0)` and so on. The interior
  height is the bilinear interpolation of the four - the build splits
  every rectangle until the interpolated surface stays within 24
  units of every covered cell height, so no detail mesh exists or is
  needed.
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
   the builder layout, and the within-16-unit duplicate layers merge
   (the l2j generator noise; the higher surface wins).
2. **The sheet decomposition** - the walkable layers flood into 2D
   manifolds top down: the fill moves to neighbour column layers
   within the 40 unit climb of the same wetness class and never
   occupies a column twice. A sheet never stacks over itself, so the
   rectangle footprints never self-overlap - the property whose loss
   crashed the naive Recast import (the 137k vertex contour). Sheets
   smaller than 4 layers are unreachable islands (no neighbour
   within the climb range anywhere); the grid engine cannot reach
   them either.
3. **The rectangle decomposition** - every sheet splits into maximal
   rectangles (extend right, then down), and every rectangle splits
   recursively along the axis that carries the height variation
   until the bilinear corner surface stays within the 24 unit
   tolerance of every covered cell. The split axis rule matters: a
   curved valley must cut across the curvature, not along it.
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
| the hard bridge pair (village -> water under the bridge) | 5.17 s, 500k nodes | 169 us | 7.7 ms full Route (A* 171 polys + funnel) |
| 200 random region pairs | 129/200, avg 2.94 s | "200/200"*, avg 339 us | 133 full + 59 partial = 192/200, avg ~10 ms |
| the region data at runtime | ~20 MB parsed, 140 ms load | 2.89 MB tile | 12.9 MB tile, 7.6 ms decode |
| the offline build | n/a | 5.1 s per region | 1.7 s per region |
| the pack build (165 regions) | n/a | ~14 min estimated | 4m25s, 1.9 GB of tiles, 604 MB peak RSS |

\* the research number counted `DT_PARTIAL_RESULT` as success - the
honest split is 133 full corridors + 59 closest-reachable partials +
8 isolated starts (island surfaces no walk leaves; the C++ audit
reported them as separate components). The 59 partials are the
correct Detour behavior for unreachable targets under the filter,
not misses.

The 7.7 ms hard-pair number sits ~45x over the C++ Detour: the
Go runtime resolves every link target through the tile map (a
mutex-guarded LRU) where Detour dereferences flat arrays, and the
nearest-poly query allocates its candidate buffer. The pooled query
state (one node array + one heap per call) keeps the allocations at
22 per route. The corridor itself is 171 polygons - the A* expands
14k nodes over them. This is still three orders of magnitude under
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

Seven region files of the shipped pack fail the l2j parse: `16_10`
(invalid layer count), `17_20..17_25` (trailing bytes - the files
are shorter or longer than the block layout consumes). The failures
are pre-existing: the grid engine's parser (the same format code
path via `ParseRegionData`) rejects them identically, so the current
pathfinding never covered them either. The pack build reports them
as FAILED and continues; a future pack refresh fixes them or not -
the runtime treats a missing tile as a wall, never as an error.

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

## What is NOT wired yet

The hunt loop and the walker still navigate on the grid engine; the
navmesh subsystem ships the builder, the runtime, the CLI and the
replay evidence. The live integration (the hunt loop consuming the
navmesh corridors, the grid engine reduced to the click validation
of every smoothed leg) is the follow-up round on the acceptance
stack of `feature/proxy-server`.
