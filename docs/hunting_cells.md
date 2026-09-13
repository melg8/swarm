# The hexagon cell hunting (hunt/cell*.go, hunt/cells_elven.go)

The cell mode is the current hunting system of the elven lands (the
user orders of 2026-09-13: the Voronoi partition first, the uniform
hexagon grid, the enemy-first roam and the ranged-kill loot after;
the circle geometry of `docs/hunting_system_redesign.md` went before
both). `main.go` wires it through `SetHuntingZoneRegion("elven")` ->
`SetHuntingCellRegion` -> `SetHuntingCells(ElvenHuntingCells())`.

## Why the circles had to go

The spot geometry failed structurally, not parametrically: a circle
that covers HALF of a respawn ground (the Dryad case of the live
sessions) farms that half to exhaustion while the other half
accumulates an unfarmed mob mass; the followup circle that covers the
second half then faces an oversaturated ground it cannot clear. No
radius, no anchor tuning fixes a cover that is partial by shape - the
cover must OWN every spawn point exactly once. That is the definition
of a partition, and the Voronoi diagram is the natural one for a
spot-seeded ground. The hexagon grid of the second order keeps the
partition property and adds the UNIFORMITY: one hexagon shape at one
circumradius over the whole map, the same patrol square everywhere -
no slivers, no fat blocks, the mesh reads as a grid an operator can
reason about.

The second structural failure was the loading boundary: the Mobius
world grid broadcasts the objects of the own region plus the 8
adjacent ones (~2048 units guaranteed, `SHIFT_BY 11`), so a ground
whose far side sits beyond that circle from the bot's position reads
EMPTY while it is full - the emptiness reading, the base fact of the
wait-or-switch economy, lies. The cells bound their extent from the
focus so the whole cell stays inside the knownlist circle of a bot
standing anywhere in its patrol square.

## The partition (tools/generate_hunt_cells.py)

- **The grid**: flat-top hexagons of ONE circumradius (1000 units)
  tile the plane - the column pitch 1.5*R, the row pitch sqrt(3)*R,
  the odd columns shifted half a row. The grid is INFINITE by
  construction: every world point falls into exactly one hexagon
  through the analytic axial cube rounding (`hex_of_point`), no
  clipping, no nearest-seed search, no densification loop. The
  registry holds the hexagons that own at least one spawn sample
  point (540 hexagons for the elven ground).
- **The ownership**: every spawn sample point maps analytically onto
  exactly ONE hexagon - the half-covered respawn failure stays
  impossible by construction (the Voronoi partition solved it with
  the nearest-seed assignment; the hex grid solves it with the
  tessellation itself). The live audit cross-check pins it: 98.6
  percent of the 1629 observed mobs of the 2026-09-13 audit belong
  to exactly one hexagon (the rest sits in the boundary rounding
  gaps and past the audited ground).
- **The visibility budget**: the circumradius is sized so the
  axis-aligned square inscribed in the hexagon (half 0.634*R = 633)
  plus the hexagon extent from the focus (R = 1000) stays inside the
  knownlist circle: `633 * sqrt(2) + 1000 = 1896 <= 2048` - the
  whole hexagon is inside the knownlist circle of a bot standing
  anywhere in its patrol square, with 152 units of headroom for the
  wander of the border mobs. The generator asserts the invariant
  and the registry tests pin it.
- **The mob composition**: the counts of every territory species
  distribute over the hexagons by the sample share (largest
  remainder, the territory totals preserved exactly - the elven
  ground carries 812 spawned mobs over 540 hexagons).
- **The patrol square**: the axis-aligned square inscribed in the
  hexagon at the center (the inward-normal distance of every edge
  scaled by its |nx|+|ny|) - UNIFORM (633 everywhere, the inscribed
  square of the identical shape). The MOVEMENT machinery of the loop
  stays square-based (the patrol walk, the zone return, the flee
  steps, the in-zone checks) - every point of the square is inside
  the hexagon, so the movement leash never leaves the cell.
- **The ground**: the exact convex hexagon polygon
  (`state.CellZone`, the int64 cross-product containment). The
  emptiness reading of the economy and the delevel median fence on
  the WHOLE hexagon (`Loop.targetZone()`) - the farm ground is the
  complete hexagon, never a square approximation of it. The TARGET
  SEARCH itself runs unfenced in the cell mode (see the enemy-first
  roam below).
- **The adjacency**: the six grid ring neighbors present in the
  registry (symmetric by construction, mean degree 5.3) - the
  rotation graph of the hunt policy.

The registry (`hunt/cells_elven.go`, 540 hexagons) is generated
code: re-run `tools/generate_hunt_cells.py` after any spawn data
change (the JSON twin `docs/hunt_analysis/cells_elven.json` and the
report `docs/hunt_analysis/cells_report.txt` regenerate with it).
The `-hunt-audit` CLI (the `huntaudit` package, the port of the spot
audit) measures the live geometry of the registry: the probe
character visits every cell focus through the database position
injection, dumps the attackable population it sees and the evidence
JSON feeds the next regeneration.

## The rotation (hunt/cell_policy.go, hunt/cell_metrics.go)

The hunter holds ONE hexagon - but the hunt roams FREELY: the bot
never rigidly binds its fights to the held ground (the enemy-first
roam below). Between the fights the wait-or-rotate economy runs (the
port of the spot economy, the geometry replaced):

- **The patience windows**: a predicted respawn within 20 s holds the
  ground (the drift walks toward the corpse position); no data at all
  gets 40 s; a full minute of emptiness marks the ground starved.
  The emptiness reading is UNFENCED - a mob of ANY hexagon the
  character currently sees keeps the ground occupied (the engage
  picks it); a ground counts as zero-enemy only when nothing
  pickable stands visible in the whole knownlist.
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
- **The follow switch**: the free-roam hunt crosses the hexagon
  boundaries chasing the nearest visible enemy; the held ground
  FOLLOWS the actual fight (`followGround`, paced at 10 s) - the map
  highlight, the metrics and the occupancy track where the fight
  really runs, the left ground starts its ripeness clock, and the
  kill records attribute to the hexagon the CORPSE lies in
  (`cellNoteKill` through `groundOf`). A boundary fight never
  ping-pongs the registry (the pacing floor, the window and the
  starve-cooldown guards).

## The enemy-first roam (hunt/loop.go)

The user order of 2026-09-13: before moving, the bot picks the zone
that holds the enemies it already SEES from the current position; a
zero-enemy zone is the last resort; the bot never rigidly binds to
the current zone - it hunts the nearest ACTUAL enemy, even one that
left the zone.

- **The unfenced pick** (`pickZone`): the target search of the cell
  mode carries NO fence - the pick takes the nearest visible windowed
  enemy wherever it stands (the knownlist bounds it). The legacy
  zone mode keeps the square leash.
- **The far walk** (`walkToFarTarget`): a targetless hunter walks
  toward the nearest visible enemy ANYWHERE in sight - toward the
  zone that holds the enemies, never into an enemy-less one. One
  paced leg at a time; the per-second pick takes any enemy the leg
  comes past.
- **The out-of-ground gate** (`onHeldGround`, `cellEnemiesVisible`):
  a character outside its held hexagon with a visible enemy keeps
  hunting it; the walk home fires ONLY when nothing pickable is
  visible at all (the zero-enemy last resort). The running fights
  outside the ground still finish where they stand
  (`adoptOutZoneFight`), the hurt-under-attack flee stays first.
- **The ranged-kill loot** (`noteKillPosition`, `killApproachWalk`):
  a mob killed at range (the bow lure, the caster spells) drops at
  its corpse; the loot phase records the corpse position, walks
  there and holds a short grace window for the trailing drop
  broadcast - the drops (the adena included) are approached and
  picked up, never left on the ground. A melee kill (the corpse at
  the feet) never holds the phase; a kill whose drops were already
  seen never walks the corpse again.

## The views (hunt/cell_view.go, the web map)

- **The mesh**: the static registry payload (the polygons, the
  bands, the foci) is served ONCE per version through
  `GET /api/hunt-mesh` (the ETag is the version string, the client
  caches it) - the mesh bytes never ride the per-second snapshot
  stream. The live snapshot carries only the version marker plus the
  live record of the held cell (the identity, the farming/moving
  state, the respawn clock, the measured income, the occupancy - a
  couple hundred bytes).
- **The map layer** (`web/map.js`): EXACTLY TWO hexagons draw - the
  one the bot fights in (the active cell of the live record: the
  light amber fill, the bright stroke, the focus dot and the live
  label - "which zone is the bot going to" answers through this
  element alone) and the one under the map cursor (the hover hit
  test of the mesh: the dashed outline, the light fill and the name
  label). Every other hexagon of the registry stays INVISIBLE - the
  full-partition edge raster retired with the Voronoi layer (a 1k+
  hexagon partition would drown the map, and the partition carries
  no information the operator needs while the fights run); the kill
  crosses layer is unchanged.
- **The manual control is gone** (the user order): the zone list
  panel, the hunt buttons and the `CommandZone` path retired - the
  registry of a full project grows past a thousand cells, the economy
  owns the rotation.

## The registry invariants (hunt/cells_test.go)

The committed registry is pinned against the generator drift: the
UNIFORM hexagon geometry (six corners, one circumradius, one area,
one patrol half everywhere), the convex CCW polygons, the symmetric
in-range adjacency, the patrol square and the focus inside the
polygon, the mass arithmetic (812 mobs preserved), the band ordering
(the village-nearest ground of the lowest band leads - the starter
fallback). A regeneration that breaks one of these is a generator
bug.
