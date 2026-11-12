# The shop planner split: six files by responsibility (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the loop, town, walker, state and launcher rounds).

## Goal

`internal/swarm/gear/shopping.go` (1553 lines) is the last hand-written
file above the 1500 line god-file bar of AGENTS.md that has no active
work around it. Done means:

- `shopping.go` keeps the data model (`Shop`, `Catalog`, `Purchase`)
  and the public plan/query API (`PlanPurchases`,
  `PlanPurchaseQueue`);
- the phase walker (the four phases, the tail walk, the emission)
  moves to `shopping_phases.go`;
- the exhaustive armor set enumeration moves to `shopping_armor.go`;
- the candidate generation, the two caches and the scoring helpers
  move to `shopping_candidates.go`;
- the slot scoring helpers move to `shopping_slots.go`;
- the equipment simulation and the public queries over it
  (`SimulateInventory`, `PlannedEquips`, the credits and the displaced
  pieces) move to `shopping_equip.go`;
- byte preserving move, no behavior change.

## Constraints

- `internal/swarm/gear` only; the hunt and webserver call sites are
  untouched.
- The package tests (`shopping_test.go`, `plan_test.go`, the bow and
  starter tests) are the safety net and stay green unchanged.
- `task fmt` (gofmt-spaces) after the move, then the full gate.

## Context

Measured 2026-11-12 before the split:

- 64 top-level declarations, the largest being `PlanPurchases` (28
  lines), `buildCatalogCandidates` (59), `viableCandidates` (73) and
  `purchaseGain` (57); no single function is the problem, the file is;
- the declaration order already groups the planner into the model, the
  phases, the armor enumeration, the candidates and the simulation, so
  the split follows it;
- the shared constants (`shopTaxLimit`, `shoppingQueueTail`,
  `unboundedBudget`, `enumerateArmorSetCap`) travel with their only
  consumer.

## Progress

### 2026-11-12 - the plan
The declaration inventory and the six-file grouping above; the split
script (the AST range lister plus a verbatim copy, scratch) is next.

## Status

Complete: the six files land on `main` as `4133669d`; `task
verify` green and the live run clean.

### 2026-11-12 - the move
The split script (the AST range lister plus a verbatim copy, scratch,
not committed) produced the six files:

| File | Lines | Content |
| --- | --- | --- |
| `shopping.go` | 148 | `Shop`, `Catalog`, `Purchase`, `PlanPurchases`, `PlanPurchaseQueue`, the tax and queue tail constants |
| `shopping_phases.go` | 263 | the phase constants, `planWalk`, `runPhases`, the four phase emitters and the tail walk |
| `shopping_armor.go` | 201 | `armorOption`, `armorFamilies`, `planArmorSet` and the frontier pruning |
| `shopping_candidates.go` | 674 | the offer candidates, the two caches, the catalog hash and the scoring helpers |
| `shopping_slots.go` | 67 | the slot bit buffer and the slot scores |
| `shopping_equip.go` | 246 | the virtual paperdoll simulation and the public queries over it |

A move, not a rewrite: no declaration renamed, no signature touched,
no constant changed. The import block of each file is derived from the
identifiers its code actually selects (the comments and the string
literals are dropped first, so a doc sentence never drags a package
in).

Content check: the sorted non-blank line multiset of
`HEAD:internal/swarm/gear/shopping.go` against the six files reports 0
lost lines; the 31 extra lines are exactly the five added SPDX headers
plus the five `package gear` / `import (...)` blocks and the import
specs.

Verified: `go build ./...`, `go vet ./internal/swarm/gear/...`,
`golangci-lint run ./internal/swarm/gear/...` 0 issues,
`go test ./internal/swarm/gear/... -count=1` ok in 0.4 s.

Next: `task verify`, the live run, then the fragment archive.

### 2026-11-12 - the gate and the live run
`task verify` exit 0 (build, vet, the full tree lint, the whole test
suite, `fmt:check`). The live run on the Windows stack: the binary
carries the `-ldflags` identity of `4133669d`, `-login
127.0.0.3:2106 -hunt -web ""`, 65 s in world, then Ctrl+Break - the
character hunted (the kite hold line), `Leaving the game world with
test1` / `Bot finished`, exit code 0.
