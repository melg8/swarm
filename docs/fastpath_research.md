# The fast pathfind research: the scales, the layers and the flow fields

The owner question this doc answers: the pathfind algorithm that
works at the whole map scale without the repeated re-paths, an order
of magnitude faster than the current answer - the simplified maps
with the many elements merged into one simpler surface for the
preliminary route, the full maps for the exact route inside it, or
the flow field alternative. The goal: the longest route plans in 10
seconds at the worst, ideally under 1 second.

## 1. The measured baseline (the corridor pack, this round)

The three town legs (the teleporter arrival points, the hierarchical
route, the corridor pack of 31 regions, the fake cell repair on):

| leg | straight | cold | warm | one shot |
| --- | --- | --- | --- | --- |
| Elven Village -> Gludio | 92939 | 1.77 s | 545 ms | found |
| Gludio -> Gludin | 73199 | 2.61 s | 3.4 ms | found |
| Gludin -> Giran | 164172 | 2.16 s | 130 ms | partial (the strand, section 4) |

The previous session measured the same Elven leg at 10.8 s cold on
the full pack. The corridor pack, the uint16 tile format and the
fake repair together bought the 6x. The warm answers are already sub
1 second everywhere the one shot works - the remaining gap is the
cold start and the strand.

## 2. The simplified map idea: already the architecture, needs the persistence

The owner proposal - the simplified map for the preliminary route,
the full maps inside it - is the architecture the port already runs:
the on the fly HPA* abstraction (navmesh/abstract.go) merges every
region's polygons into the 16x16 cell cluster graph (the nodes are
the clusters, the edges are the boundary links, the link components
gate the fake crossings), the coarse search plans over it and the
refinement hops re-search the real mesh inside the corridor clusters.

What it lacks is persistence. The cold query pays the tile decode
plus the cluster graph build for every region it touches; the
abstract LRU (32 graphs) evicts them again. The fix is the
**persistent coarse layer**:

- `navmesh-build` serializes the built cluster graph of every region
  into an `X_Y.ab` sidecar (the nodes, the edges and the run length
  encoded components - a few hundred kilobytes per dense region,
  against the tens of megabytes of the tile).
- The runtime loads the `.ab` files eagerly for the whole world
  (165 regions x the small file = the tens of megabytes of RAM) and
  the coarse search runs over the complete world graph with zero
  tile decodes. The tile decode stays only inside the refinement
  hops, along the coarse corridor - the working set the LRU already
  bounds.

That removes most of the cold/warm gap and the section 1 table of
this round already carries the measured answer: on the compressed
corridor pack the Gludio Gludin cold start drops 9.17 s -> 5.25 s
(-43 percent) with the sidecars, the warm answer stays 3.7 ms, the
route answer stays identical (the same 110986 explored, the same
corridor). The sidecar pass writes 31 sidecars of 56 MB total for
the corridor (1.8 MB average against the 27 MB average tile). The
remaining cold cost is the gunzip tile decode of the refinement hops
- the price of the 2.6x disk saving.

## 3. The flow field verdict: not the tool for this query shape

The flow field (the Dijkstra/BFS flood from the target over every
cell, the agents then read the field downhill) amortizes over the
agents per target. The arithmetic at this scale:

- One region grid cell stack is 2048 x 2048 = 4.2M cells; the whole
  continent is 165 regions = 690M cells. One field build walks every
  reachable cell - the cost of ~1000 warm A* queries spent to answer
  ONE query.
- The mesh A* explores 11k-43k polygons for the town legs (the
  fraction of a percent of the 15M polygon corridor). The flow field
  explores 100 percent by construction. It loses the single query by
  3 to 4 orders of magnitude, exactly the opposite of the goal.
- The amortization case (a fleet of bots walking to ONE hunt spot)
  does not pay here either: the fleet is dozens, the field serves
  the fleet only after the build pays for itself at thousands of
  agents, and the LRU cached coarse graph already answers the fleet's
  coarse guidance in milliseconds.

The known implementations (the Recast/Detour crowd local steering,
the Supreme Commander 2 style hierarchical flow fields, the Red Blob
game demos) confirm the shape: flow fields win in the dense crowds
to the shared target, not in the sparse agents over the continent.
The verdict: keep HPA*, skip the flow field.

## 4. The Gludin -> Giran strand: the anatomy and the plan

The strand the town route measurement reports is now mapped to the
cell (giran_strand_test.go, pocket_reach_test.go,
eastseam_test.go):

- The segmented walk advances 93044 of 164172 units through 18_22 and
  19_22, crosses the 19_22/20_22 seam and stalls at the inner bay
  shore cell (20_22 cell 0_1024, world (0, 147456)).
- From the stall cell the east targets answer partial=false (the
  confined search exhausts the pocket component, 64k explored, no
  exit), the north detour answers partial=true (the pocket opens
  north), the seam crossing back west works.
- The raw geodata seam: the 19_22 east edge is the flat -3720 shelf,
  the 20_22 west edge rides -3520..-3456 - the 200 unit step the
  climb rule refuses. The full border scan: 1110 of 2048 cells pair
  legally (the spans y 1282..1743 and y 0..350 are the widest), the
  strand sits in the y 912..1281 gap.
- The geometry is honest (the mountain ridge along the straight
  line): the route has to detour through the crossable spans or
  around the bay. The one shot search dies because the confined
  allowed set (the coarse corridor plus one cluster dilation) has no
  exit inside its budget - the coarse guide picked a corridor that
  dead ends inside the pocket.

The plan: (a) the persistent coarse layer of section 2 lets the
coarse search see the whole world graph and pick the detour corridor
that actually reaches Giran before any confined budget starts; (b)
the re-path walk should re-target the last bindable waypoint toward
the crossable spans (the practical bot answer today); (c) the
operator waypoint graph over the towns remains the fleet grade
fallback.

## 5. The size arithmetic after the uint16 round

The tile format v2 quantizes the polygon bounds and the link spans
to uint16 (the poly wire 32 -> 24 bytes, the link 20 -> 16, the
external link 12 -> 8). The corridor pack of 31 regions builds
2.2 GB raw and compresses to 0.85 GB (2.6x). Against the source
geodata of the same regions (~130 MB) the mesh stays 6.5x: the
links (33.7M over the corridor) and the repaired fake surface (the
bay areas became walkable geometry) own the mass. The next size
lever is the link delta encoding if the disk mass ever matters more
than the decode speed.

## 6. The recommendation

1. Implement the persistent coarse layer (section 2) - the biggest
   remaining lever, it serves the cold start, the fleet queries and
   the Giran detour at once.
2. Keep the hierarchical A* and the LRU; skip the flow fields
   (section 3).
3. Land the fake cell repair as the default for the pack builds (the
   bay legs already answer one shot with it; the flag flip is the
   release decision).
4. The Giran leg rides the re-targeted re-path until (1) lands, then
   re-measure (section 4).

## 7. The external engine benchmarks: the raasta and the condor measurements

The owner question this section answers: how much time the algorithms
of https://github.com/MacCracken/raasta (the Rust navigation engine,
the navmesh A* of src/mesh.rs) and
https://github.com/bnomei/condor (the Rust comparison library, the
navmesh family: ChannelSearch, TA*, TRA*) take on our problem.
`cmd/navmesh-export` dumps the closed subgraph of the requested
regions (the polygons, the resolved links and the portal segments)
into a flat binary; the bridge harness now ships in this repository
(`tools/l2bridge`, a cargo project pinning raasta
af87051a9478f48a555057ed64ce87e03a6e887e and condor
011da4cbea32f3f49938985dc1f15a608d45a316), `tools/l2bridge/run_bench.sh`
reproduces the whole round: the pack build, the production probe,
the cargo build and the per engine query runs (one algorithm per
process, the condor family under a 600 s wall clock cap). The
measurement machine: 2 cores, ~4 GB RAM. The raasta side prices the
water polys 3x through the NavPoly cost field (the swim filter
semantics of the production Route, the plain find_path applies the
poly cost).

### 7.1 The owner diagonal across the 4 tile corridor

The dump of the 20_19, 20_20, 21_19, 21_20 corridor: 2,541,901
polygons, 7,452,342 links. The query: the owner's URL route
from=12338,42444,-3640 to=58630,91061,-3696, the swim filter
semantics of the production Route.

| engine | build | query | answer |
| --- | --- | --- | --- |
| swarm production Route (tiles decoded, sidecars warm) | - | 0.58 ms first, 0.45 ms warm | found, 287 waypoints |
| swarm production Route (the user visible cold: zstd decode + sidecar load + search) | - | 0.58 s | found, same corridor |
| raasta navmesh A* | 370 ms | 154 ms first, 149 ms best | found, 935 poly path |
| condor ChannelSearch | 1.9 s | >550 s, killed | no answer |
| condor TAStar | 2.0 s | >550 s, killed | no answer |
| condor TRAStar (preprocess) | - | preprocess does not finish at this scale | no answer |

### 7.2 The single tile 21_19

The dump of 21_19 alone: 372,846 polygons, 1,013,866 links (781
external targets outside the region skipped). The query: an interior
pair the production engine connects (36000,36000 to 50000,50000),
found route.

| engine | build | query | answer |
| --- | --- | --- | --- |
| swarm flat search warm (BenchmarkFlatFound21x19) | - | 12.1 ms | found |
| swarm flat search, the unreachable pair (BenchmarkFlatExhaustive21x19) | - | 782 ms | no path |
| raasta navmesh A* | 56 ms | 5.5 ms first, 4.9 ms best | found, 298 poly path |
| condor ChannelSearch | 281 ms | 482.7 s, found | 3 point polyline, 60805 visited |
| condor TRAStar | 247 ms + 342 ms preprocess | search >550 s, killed | no answer |

The raasta flat engine beats the swarm flat search on the single
tile (4.9 vs 12.1 ms warm) and loses to the production hierarchy on
the cross tile route (149 ms vs 0.58 s cold / 0.58 ms warm - the
raasta number is every query, no warm path exists). The exhaustive
pair names the node state constant: the raasta typed arena answers
the whole component sweep orders ahead of the boxed heap round
(the typed arena node index of section 8 closed most of that gap;
the 782 ms answer still carries the heap ops and the per link
height bilinear).

### 7.3 The native suites (each engine on its own synthetic data)

The raasta criterion suite (51 benches, this machine): grid A*
100x100 15.1 us, JPS 100x100 35.8 us, Theta* 50x50 39 us, Lazy
Theta* 9.9 us, bidirectional 8.9 us, fringe 104 us, flow field
50x50 206 us, HPA* 200x200 batch of 20 queries 29.7 ms, D* Lite
compute/replan 1.7 ms, navmesh A* 1000 polygons 24.3 us. Their
navmesh scenes are the 10..1000 polygon toys; the per polygon cost
scales cleanly but nothing there exercises the million polygon
graph.

The condor bench lane (`dev/condor-bench`, the navmesh_direct and
navmesh_prepared targets) runs Polyanya, ChannelSearch and TA* over
its own scenario pack - the scenes there are the same small order.
The published suite never leaves the design scale of its library.

### 7.4 The verdict

1. The condor navmesh family is not a candidate at this scale: its
   per call `Vec` allocations (neighbors(), portals_from() return
   owned vectors) and the linear point location cannot carry a 2.5M
   cell query. The single tile ChannelSearch answer landed at
   482.7 s (five orders of magnitude behind the flat searches), the
   prepared TRA* preprocess finishes on the single tile (342 ms)
   and its search still does not return inside 550 s, and at the
   corridor scale neither the searches nor the preprocess finish
   at all.
2. The raasta navmesh A* is a well built flat engine: on the single
   tile it beats the swarm flat search 4.9 vs 12.1 ms warm, and its
   exhaustive behavior on the unreachable pair shows how much the
   typed arena open list buys. But it has no hierarchy: on the
   cross tile diagonal it lands at ~150 ms against the swarm
   production 0.58 ms warm (the coarse layer + the hop refinement),
   and it would hold that per query forever - no warm path exists.
3. The architecture verdict of sections 2 and 3 stands: the
   persistent coarse layer + the budgeted refinement hops is the
   right shape for this mesh. What the external engines contribute
   is the implementation lesson (section 8): the flat search inside
   the hops deserves the typed arena node state.

## 8. The flat search node state: the measured optimization target

The 7.3 gap (the 494 ms exhaustive flat answer against the raasta
35 ms on the identical unreachable query) decomposes into the node
index (the `map[PolyRef]uint32` in queryState - a hash lookup with
the 64 bit key per expansion), the `map[PolyRef]uint32` in the
escape guard and the boxed heap expansions. The replacement: the
open addressing table keyed by the packed PolyRef (the 64 bit key,
the linear probing, the power of two capacity sized to the tile poly
count) with the typed slice based open list. The expected win is
the 3..5x on the flat search constant, which moves the refinement
hops and the confined searches of the hierarchy with it. The warm
hierarchy answers do not wait for this (they are already 0.5 ms),
the cold corridor answer does.

## 9. The zstd tile round: the cold path the profile named

The section 8 profile put the flate decoder next to the search on
the cold queries: the huffman stages own a third of the first route
samples. The tile compression moves to zstd (the klauspost pure Go
implementation): the pack build writes zstd frames, the runtime
loader picks the format by the magic word (the legacy gzip packs
stay readable forever, the zstd round adds no migration step).

The same 31 region corridor pack, rebuilt end to end (the tiles and
the sidecars, the fake repair on, 2 workers, 2m35s):

| measure | gzip (BestCompression) | zstd (SpeedDefault) |
| --- | --- | --- |
| tile bytes on disk | 0.84 GB | 0.85 GB |
| owner diagonal cold (4 tiles decoded) | 1.45 s | 0.55 s |
| single tile cold (21_19) | 324 ms | 156 ms |
| warm answers | unchanged | unchanged |

The ratio is a wash (the link tables are the incompressible mass),
the decode is 2.5x faster, and that is the trade the cold queries
want. The user visible answer of the owner diagonal lands at
~0.6 s on this 2 core sandbox - an order of magnitude under the
>10 s report that opened this research, with the warm steady state
untouched (0.5 ms). The remaining cold cost splits between the
zstd decode itself, the tile parse and the coarse sidecar loads;
the next lever would be the parallel decode of the corridor tiles
per hop, not worth it while the answer stays under a second.

## 10. The parallel cold path and the mtime loss guard

The cold route cost decomposed on the 4 tile corridor (the
TestColdProbe diagnostic, the mtime stripped pack copy): the 4 tile
decodes 840 ms sequential, the 4 sidecar loads 1.43 s (!), the
searches ~30 ms. Two rounds answer the decomposition.

**The sidecar staleness guard was a deployment trap.** The v1
sidecar records the tile file size and mtime it was built against;
a pack copied without preserving the mtimes (a plain `cp -r`, an
archive extraction, some sync tools) rejected every sidecar and the
coarse search fell back to the tile decode per frontier region -
the silent multi second degradation on exactly the machines the
route is already cold on. The v2 sidecar adds the head and the tail
CRC32 of the tile file bytes (the 56 byte header had the room):
the runtime guard reads ~1 KB and accepts a same size same checksum
tile regardless of the mtime. A rebuilt tile almost surely changes
the header counts or the tail tree, so a stale sidecar never
serves; the legacy v1 sidecars keep the mtime only guard. The
measured sidecar load after the fix: 1.43 s -> 10.5 ms per 4
regions.

**The cold loads run in parallel now.** The Tile decode moved
outside the mesh lock behind a per key singleflight (the
concurrent callers of one region share one decode, the different
regions decode side by side), and the route path prefetches: the
endpoint tiles before the nearest poly resolution, the line
corridor region abstracts (capped at 24, the abstract LRU stays
the RAM bound) before the coarse search, the first chain regions
before the hop refinement plus a rolling two cluster look ahead
during the hops (the LRU window the walk keeps live).

The owner diagonal, the mtime stripped copy, this machine: cold
1.48 s -> 0.45 s, the warm answers unchanged (0.4..0.6 ms), the
route answer identical (found, 287 waypoints, 49825 explored). On
a many region world pack the guard round is the bigger lever: a
rejected sidecar cost a tile decode per coarse frontier region,
the decode count grows with the route length instead of the tile
count the route touches.
