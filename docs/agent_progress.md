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

### Design decisions (locked at the start)

- The mesh builder replaces the whole Recast pipeline: geodata
  layers -> dedup (16 unit bands) -> sheets (2D manifolds, 40 unit
  climb, one layer per column per sheet, island filter < 4 layers)
  -> rectangle polygons per sheet (maximal rectangle decomposition,
  split until the bilinear corner height error stays within 24 units
  of every cell height - exact per vertex heights, no detail mesh).
- The NSWE walls become portal spans: a link between two polygons
  carries the maximal open cell span of the shared edge, so the
  funnel can only cross where the geodata walls are open. This
  closes the 413k invisible-wall gap of the height-only import at
  cell pair granularity.
- The native tile format (rectangle polys: cell bounds + 4 exact
  corner heights + Detour-style link chains + quantized BVTree)
  replaces the Detour binary of the research round; the research
  parser moves to navmesh/prototype unchanged.
- The water semantics stay the research model: water polys carry a
  3x area cost (the dry searches exclude them, the escape prices
  them high) - H-001 stays a hypothesis either way.
- The hunt loop wiring is NOT part of this round: the port ships
  the builder, the runtime, the CLI and the replay evidence; the
  live integration follows on the acceptance stack of
  feature/proxy-server.

### Acceptance criteria

- `go build ./...`, the new suites and `task lint:new` green;
  `task fmt:check` clean.
- The builder builds the real 21_19 region from `data/geodata` in
  one command and the tile passes the health audit (zero one-way
  links).
- The hard bridge pair (village dump cell 45768 49848 -3056 ->
  water under the bridge 44920 -3928 50792) answers: the full swim
  corridor, the dry partial with the closest dry point, the reverse
  escape.
- The 200 pair replay reaches >= 195/200 routable (the C++ Detour
  answered 200/200, the prototype 163/200).
- The benchmarks answer the microsecond claim (findPath+funnel on
  the hard pair, tile decode, region build).
