# The hunt data extraction: the generated registries into huntdata (status: complete)

Started 2026-11-13, branch main, issue0 (continuation of the
package-wide refactor after the proxy game connection split).

## Goal

The `internal/swarm/hunt` package carries three generated registries
with ~17.3k lines of spawn data between the hand-written hunt logic
(`cells_elven.go` 11836, `zones_elven.go` 4242, `zones_dion.go` 1269
of 72133 total). Done means:

- the generated registries move to a leaf package
  `internal/swarm/huntdata`, together with the value types they
  describe (`Cell`, `CellMob`, `CellVertex`, `HuntingZone`, `ZoneMob`)
  and the two methods on them (`TargetZone`, `ContainsPoint`);
- `hunt` keeps type aliases and the exported accessors
  (`ElvenHuntingCells`, `ElvenHuntingZones`, `DionHuntingZones`), so
  every hunt file and test compiles with only the method renames at
  the call sites;
- the generators (`tools/generate_hunt_cells.py`,
  `tools/generate_hunt_zones.py`) emit the new package and paths;
- the data bytes are unchanged: the registry tests pin the same
  tables and `git diff` on the moved data files shows the package
  line only;
- no behavior change.

## Constraints

- `huntdata` is a leaf: it imports `state` only (and the generators
  emit the data); `hunt` imports `huntdata`, never the reverse.
- The region keys stay one concept: `hunt` keeps the `regionElven`
  and `regionDion` constants as aliases of the exported
  `huntdata.RegionElven`/`RegionDion`, the generated files keep using
  the unexported aliases they already reference.
- The frozen logs (`docs/development_log.md` and the archives) stay
  untouched; the live docs (`docs/hunting.md`,
  `docs/hunting_cells.md`, `docs/band_20_25_survey.md`,
  `docs/hunting_system_redesign.md`, the AGENTS.md layout) follow the
  paths.

## Context

Measured 2026-11-13 before the move:

- `cells_elven.go` has one var (`elvenHuntingCells []Cell`) and one
  accessor; `zones_elven.go` and `zones_dion.go` the same
  (`[]HuntingZone`); the only methods on the moved types are
  `Cell.targetZone` (4 call sites) and `HuntingZone.containsPoint`
  (3 call sites);
- the type definitions live in `hunt/cell.go` (CellVertex 30,
  CellMob 39, Cell 63) and `hunt/zones.go` (ZoneMob 30, HuntingZone
  46); the big model comments precede them and move with the types;
- the Mobius spawn tree is absent on this host, so the generators
  cannot be re-run: the move is verified by `py_compile` and by the
  byte comparison of the data, and the generator emission lines are
  updated by hand;
- the `merchant_stands_world.go` table stays in `hunt` (it is emitted
  by `cmd/counterprobe -mode world-emit` and is small; the counterprobe
  path is out of scope for this round).

## Progress

### 2026-11-13 - the plan
The declaration inventory, the type/accessor map above; next the
package skeleton and the data move.

### 2026-11-13 - the move
The new leaf package `internal/swarm/huntdata`:

| File | Content |
| --- | --- |
| `doc.go` | the package doc |
| `cell.go` | `CellVertex`, `CellMob`, `Cell`, the `TargetZone` method |
| `zone.go` | `ZoneMob`, `HuntingZone`, the `ContainsPoint` method, `RegionElven`/`RegionDion` and the unqualified keys the generated files emit |
| `cells_elven.go` | the generated 540 hexagon elven partition (moved, package line only) |
| `zones_elven.go` | the generated elven zone ladder (moved, package line only) |
| `zones_dion.go` | the generated Dion 20-25 ladder (moved, package line only) |

`hunt` keeps the aliases (`Cell`, `CellMob`, `CellVertex`,
`HuntingZone`, `ZoneMob`), the `regionElven`/`regionDion` constants
and the `ElvenHuntingCells`, `ElvenHuntingZones`, `DionHuntingZones`
wrappers, so the hunt files and tests only changed at the four
`TargetZone` and three `ContainsPoint` call sites. The generators
`tools/generate_hunt_cells.py` and `tools/generate_hunt_zones.py`
emit the new package and paths; the live docs and the AGENTS.md
layout follow. The Mobius spawn tree is absent on this host, so both
generators were checked with `py_compile` only.

Verification: `go build ./...`, `go vet`, the hunt suite (28.2 s),
`huntaudit`, `golangci-lint` 0 issues; `task test:cover` inside the
2.0 pp budget (hunt 84.4 to 85.7, the generated data left the
package) and `task verify` green.

## Status

Complete: the package and the baseline land on `main` as `538bc33c`
and `826cb640`, `task verify` green, the tree pushed.
