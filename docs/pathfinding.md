# Pathfinding

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The long distance movement of the bot is served by the standalone
pathfinding module `internal/swarm/pathfind`. It is a Go port of the
pathfinder behind L2Bot2.0
([L2jGeodataPathFinder](https://github.com/k0t9i/L2jGeodataPathFinder),
MIT): an A* search over the geodata cell grid with wall (NSWE) and
height checks, a supercover line rasterization for line of sight and a
string pulling post smoothing that collapses the raw cell path into
turning points. The module only reads geodata files, never talks to
the game server, so it works without a bot and can later be used as a
movement service or a hunt helper (return to the farm spot after
death, walk to town and back) as soon as the benchmarks stay
acceptable - which they currently do (see below).

## Geodata format (Mobius C1 `GeoEngine.java` is the reference)

Headerless little endian region files `X_Y.l2j` with the same
`World.TILE_ZERO_COORD` anchors as the map tiles (X = floor(x / 32768)
+ 20, Y = floor(y / 32768) + 18), 65536 blocks of 8x8 cells per
region, block kinds flat (raw 2 byte height), complex (64 cell words:
low nibble NSWE, height = (word & 0xFFF0) >> 1) and multilayer (per
cell layer count byte + layer words). The wire heights are quantized
to multiples of 8, flat blocks store the raw height.

## Engine

`pathfind.NewEngine(dir)` scans the directory, parses regions lazily
and keeps an LRU cache of `DefaultCacheCapacity` (4) parsed regions
(~20 MB each). `Engine.FindPath(start, end, maxPassableHeight)` returns
the smoothed waypoints plus the raw cell path, the search duration,
the explored node count and the path length; `ErrMissingCell` marks a
start or target without geodata and `Result.Aborted` a search that hit
`MaxSearchExpansions` (1M) - an unreachable target in open terrain
would otherwise sweep the whole region grid. The target z drives the
target layer resolution (like the server's own `PathFinding.findPath`
getHeight(tx, ty, tz)), and the plain search terminates on the target
cell with whatever layer the walk arrived on.
`Engine.FindPathApproach(start, end, approachRadius,
maxPassableHeight)` terminates on the first node within the 3D approach
radius of the end point instead - the town trips navigate with it (a
merchant behind a counter or on a floor layer the geodata does not
model is reached through the deck ring within the interaction
distance, the water deck below the shop never satisfies the radius).

The A* step rules mirror the Mobius movement validation: upward steps
are gated at `DefaultMaxPassableHeight` (40, the Mobius
HEIGHT_INCREASE_LIMIT), drops of any height are walkable, and layers
below the C1 water surface (waterLevel -3780, the maxZ of the water
zones) cost `waterCostMultiplier` (3x) per step so bridges and shores
beat swimming whenever they exist. The line of sight raster keeps the
strict symmetric height rule, so the smoothing never collapses a
detour into a straight drop. The smoothing is also water aware
(`legDry`): a leg between two dry points must stay above the water
surface - the string pulling only asks the line of sight, and the
sight lines across the gradual lake beds stay open, so without the
rule the smoothed path would ford the very bays the cost aware search
paid to route around (the 2026-09-10 elven lake regression,
`TestSmoothedLegsStayDry`). Legs that start or end in the water are
exempt: they belong to the swim escape below.

The engine answers three water queries besides the searches:
`OverWater(x, y, refZ)` tells whether the walkable surface under a
position lies below the water level (the layer closest to the
reference z decides, so a swimmer above a lake bed reports over water
while a character on the deck above the same cell does not);
`DryLine(start, end)` verifies that the straight segment between two
world points is a clean dry walk (walkable and never below the water
level - the town walker checks every click line with it before sending
the move request, because the server walks characters into water
without any hesitation); `FindWaterEscape(start)` plans the way out of
the water for a position standing over a lake or sea bed: a breadth
first flood over the walkable surface (the same canStep rules) that
stops on the first node above the water level - the nearest shore. A
start already on dry ground answers Found=false.

## Deliberate deviations from the original

- Wall hits are skipped instead of being pushed into the open set with
  an astronomic cost (the original can return a wall crossing path for
  a sealed target).
- The smoothing anchor stays on the committed waypoint, so every leg
  of the smoothed path is line of sight verified (the original jumps
  the anchor past the commit and can cut wall corners near gaps).
- Search nodes are keyed by (cell, layer height) instead of one node
  per cell - a cell first touched from the water must not lose its
  bridge deck layer, that poisoning cut the Elven village bridge in
  half until the fix (regression test `TestFindPathBridgeOverWater`).
- The open set is a binary heap instead of the linear scan, ties
  broken by remaining distance and insertion order for deterministic
  paths.

## Pathfind test UI

`go run ./cmd/swarm -pathfind-test` serves the map without any bot
behind it (`-geodata` points at a geodata directory, auto detected at
`./data/geodata` and the reference deployment otherwise;
`-max-passable` overrides the default 40). The UI opens on the hunting
zone, shows draggable A and B markers (or arm the set A/set B buttons
and click the map), draws the found path as a red dashed line with a
small circle at every turning point, the raw A* cell path as a faint
line on demand (`raw path` toggle) and the search statistics (time,
nodes explored, waypoints, path length, loaded regions) in the
sidebar. Endpoints: `GET /api/config` (mode, geodata summary) and
`POST /api/pathfind` (start/end world points).

## Tests and benchmarks

Tests and benchmarks run without a bot or server: the synthetic region
builder in `region_test.go` writes hand built l2j files, the real
Giran region of the original example ships as
`pathfind/testdata/22_22.l2j` (from the L2jGeodataPathFinder usage
example, MIT) and anchors the parser plus the cross city benchmark
(the original example path near the Giran weapon shop to the north
bridge). Numbers on the dev machine: cross city path ~15 ms / 13k
allocs, line of sight ~1.4 ms, region parse ~140 ms (one time, 21 MB
resident) - fast enough for occasional hunt usage; the extreme
synthetic zigzag maze stays at ~2.3 s and is bounded by the expansion
cap. Benchmarks: `go test ./internal/swarm/pathfind/ -bench .
-benchmem`.

## Geodata visualization

The pathfind test map can replace the photo imagery with rendered
geodata (`geodata` checkbox in the toolbar, mode select: height /
walls / layers). The bot process renders each region tile on demand
(`GET /api/geodata/tile/{level}/{bx}_{by}.png?mode=...`, pyramid
level 0 renders one cell per pixel, 4 more levels halve the
resolution) and caches the encoded PNGs in a small LRU; the browser
caches them aggressively. The height mode paints the walk surface
grayscale with a blue tint below the typical sea level, red over cells
with closed walls and green on multilayer cells - the walls and layers
modes isolate the connectivity and the multilayer structure. The view
follows exactly what the search sees at the default height: the layer
closest to zero.

## Geodata deployment

The empty `data/geodata` folder of the reference deployment was filled
with a 215 region C1 compatible pack (all files verified against the
block walk rule above; sources documented in the session log). The
game server picks the files up on its next restart; the pathfinder
reads them directly and needs no server restart.

The geodata region files the bot navigates with live in `data/geodata`
of this repository: the complete old-world pack (165 regions, grid
16_10..26_26, l2j headerless format, sha1-verified at download time
from the LGK-Games/Geodata mirror of the upstream pack; 21_19.l2j is
byte identical with the region the running server already used) so
the bot never depends on the server tree for its pathfinding and
future hunting grounds beyond the elven lands are covered too;
refresh the pack the same way when a deployment upgrade changes it.

## World navigation gaps

`docs/navigation_analysis.md` holds the measured analysis of what
separates the current system from universal A to B navigation: scale
limits of the grid search (a half world land route costs 25M node
expansions, some village pairs have no geodata connection at all), the
missing meta transport graph (352 gatekeeper destinations and 3 boat
routes in the server data), water movement as a separate cost
dimension (swim speed vs run speed, breath and drowning on deep
crossings, unverified server swimming semantics), doors, the danger
layer over the spawn data and the missing bot side waypoint walker.
Continue any navigation work from that document.
