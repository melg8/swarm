# The Voronoi cell hunting (hunt/cell*.go, hunt/cells_elven.go)

The cell mode is the current hunting system of the elven lands (the
user order of 2026-09-13; it replaces the spot-anchored circles of
`docs/hunting_system_redesign.md`). `main.go` wires it through
`SetHuntingZoneRegion("elven")` -> `SetHuntingCellRegion` ->
`SetHuntingCells(ElvenHuntingCells())`.

## Why the circles had to go

The spot geometry failed structurally, not parametrically: a circle
that covers HALF of a respawn ground (the Dryad case of the live
sessions) farms that half to exhaustion while the other half
accumulates an unfarmed mob mass; the followup circle that covers the
second half then faces an oversaturated ground it cannot clear. No
radius, no anchor tuning fixes a cover that is partial by shape - the
cover must OWN every spawn point exactly once. That is the definition
of a partition, and the Voronoi diagram is the natural one for a
point-seeded ground.

The second structural failure was the loading boundary: the Mobius
world grid broadcasts the objects of the own region plus the 8
adjacent ones (~2048 units guaranteed, `SHIFT_BY 11`), so a ground
whose far side sits beyond that circle from the bot's position reads
EMPTY while it is full - the emptiness reading, the base fact of the
wait-or-switch economy, lies. The cells bound their extent from the
focus so the whole cell stays inside the knownlist circle of a bot
standing anywhere in its patrol square.

## The partition (tools/generate_hunt_cells.py)

- **The seeds**: the spawn territory polygons of the Mobius XML are
  sampled on a 128-unit grid; the sample cloud of every grid
  adjacency cluster is cut (median cut on the widest axis) and packed
  into visibility-sized pieces - the piece centroids are the seeds
  (the same placement the live audited spot geometry validated on
  2026-09-13).
- **The cells**: the Voronoi cell of a seed is the half-plane
  intersection (the points nearer to the seed than to any other),
  clipped to the ground envelope by Sutherland-Hodgman - the convex
  CCW polygon is exact, not a square approximation. Every spawn
  sample point is assigned to its NEAREST seed: the partition owns
  every spawn point of the ground exactly once, the half-covered
  respawn failure is impossible by construction. The live audit
  cross-check pins it: 99.9 percent of the 1629 observed mobs of the
  2026-09-13 audit belong to exactly one cell.
- **The visibility budget**: a cell whose sample radius from its
  focus exceeds 1600 units splits (a fresh seed at the far half
  centroid, the diagram recomputes) until stable; the patrol square
  half is capped so `patrolHalf * sqrt(2) + radius <= 2048` - the
  whole cell is inside the knownlist circle from ANY corner of the
  patrol square. The generator asserts the invariant and the registry
  tests pin it.
- **The mob composition**: the counts of every territory species
  distribute over the cells by the sample share (largest remainder,
  the territory totals preserved exactly - the elven ground carries
  812 spawned mobs over 349 cells).
- **The patrol square**: the axis-aligned square inscribed in the
  cell polygon at the focus (the inward-normal distance of every edge
  scaled by its |nx|+|ny|), floored at 256. The MOVEMENT machinery of
  the loop stays square-based (the patrol walk, the zone return, the
  flee steps, the in-zone checks) - every point of the square is
  inside the polygon, so the movement leash never leaves the cell.
- **The target leash**: the exact convex polygon
  (`state.CellZone`, the int64 cross-product containment). The
  engage, the far target search, the emptiness reading and the
  delevel median fence on the WHOLE cell - the farm ground is the
  complete cell, never a square approximation of it
  (`Loop.targetZone()`).
- **The adjacency**: the seeds whose bisector bounds the cell are its
  neighbors (symmetric by construction, mean degree 5.7) - the
  rotation graph of the hunt policy.

The registry (`hunt/cells_elven.go`, 349 cells) is generated code:
re-run `tools/generate_hunt_cells.py` after any spawn data change
(the JSON twin `docs/hunt_analysis/cells_elven.json` and the report
`docs/hunt_analysis/cells_report.txt` regenerate with it). The
`-hunt-audit` CLI (the `huntaudit` package, the port of the spot
audit) measures the live geometry of the registry: the probe
character visits every cell focus through the database position
injection, dumps the attackable population it sees and the evidence
JSON feeds the next regeneration.

## The rotation (hunt/cell_policy.go, hunt/cell_metrics.go)

The hunter holds ONE cell. Between the fights the wait-or-rotate
economy runs (the port of the spot economy, the geometry replaced):

- **The patience windows**: a predicted respawn within 20 s holds the
  ground (the drift walks toward the corpse position); no data at all
  gets 40 s; a full minute of emptiness marks the ground starved.
- **The ripeness clock** (the core of the anti-depletion design): a
  ground the hunter leaves empty starts its clock at `clearedAt`; it
  is UNRIPE until `clearedAt + its respawn window` (15-20 s elven)
  and the rotation never walks into an unripe cell. The bot farms its
  neighborhood at the respawn pace - the crop rotation: cell A
  cleared, the neighbor B farmed meanwhile, A ripe again on return.
  No ground starves while another saturates, the cycle closes at the
  respawn rate and the walks stay inside the adjacency ring (a few
  hundred units per hop).
- **The neighbor-first sweep**: the starved and the economic switches
  pick the best RIPE candidate of the 1-hop ring, then the 2-hop
  ring; the far relocation (the global contest with the proximity
  discount) fires only when the level window emptied the whole
  neighborhood - the bots travel the map freely but never run far
  for farm.
- **The starvation net**: a ground that reads empty even when ripe
  (another hunter's farm, a wiped spawn) keeps a 5-minute starve
  cooldown - the sweep moves forward through the mesh, the two-ground
  ping-pong livelock of the 2026-09-13 session cannot return.
- **The economy**: `cellScore = value x safety x proximity x
  ripeness / (1 + occupancy)`. The value is the bootstrap prior
  (window mass x respawn turnover x level-priced kills) until 5
  minutes of measured income replace it (a trusted zero stays zero);
  the safety folds the static aggressive share and the decayed death
  heat (half-life 30 minutes, per cell); the proximity discounts the
  walk (half score at 8000 units); the ripeness discounts an unripe
  cell; the occupancy (the process-wide `cellHub`) divides the score
  so the fleet spreads over different neighborhoods. A voluntary
  switch needs the alternative 25 percent above after a 5-minute
  fair trial; a death re-picks at once through the global contest
  (the escape from the deadly neighborhood).
- **The window**: the target search filters on `[max(1, L-8), L+2]`
  (the C1 full-adena edge, the hard engage ceiling), the priorities
  bias the white-green subrange `[L-4, L-1]` strongest.
- **The dynamic focus**: the kill centroid EMA per cell feeds the
  map label and the walk targets drift to the live mass (the
  wait-walk legs to the predicted corpse positions). The PARTITION
  stays static (the fleet shares one registry - the cells, the
  neighbors and the ripeness clocks of one bot never re-map the
  ground of another).

## The views (hunt/cell_view.go, the web map)

- **The mesh**: the static registry payload (the polygons, the
  bands, the foci) is served ONCE per version through
  `GET /api/hunt-mesh` (the ETag is the version string, the client
  caches it) - the mesh bytes never ride the per-second snapshot
  stream. The live snapshot carries only the version marker plus the
  live record of the held cell (the identity, the farming/moving
  state, the respawn clock, the measured income, the occupancy - a
  couple hundred bytes).
- **The map layer** (`web/map.js`): the partition edges stroke as
  ONE cached raster (one thin line per boundary, no fills, no
  shading of the inactive cells - the render load of a 1k+ cell
  registry stays a single blit), and exactly ONE highlighted element:
  the cell the bot holds or walks to (the light amber fill, the
  bright stroke, the focus dot and the live label - "which zone is
  the bot going to" answers through this element alone). The hovered
  cell of the pointer reads its name; the kill crosses layer is
  unchanged.
- **The manual control is gone** (the user order): the zone list
  panel, the hunt buttons and the `CommandZone` path retired - the
  registry of a full project grows past a thousand cells, the economy
  owns the rotation.

## The registry invariants (hunt/cells_test.go)

The committed registry is pinned against the generator drift: the
convex CCW polygons, the symmetric in-range adjacency, the patrol
square and the focus inside the polygon, the mass arithmetic (812
mobs preserved), the band ordering (the village-nearest ground of
the lowest band leads - the starter fallback). A regeneration that
breaks one of these is a generator bug.
