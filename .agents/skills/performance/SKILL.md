---
name: performance
description: >-
  The data-oriented design and zero-allocation discipline of the swarm
  repository: SoA split of the world store, hot vs cold records, the
  snapshot encoder contract, the benchmark gate and the fleet E2E
  stretch goal. Use when touching a hot path (state apply, packet
  parse, snapshot encode, hunt tick, pathfinder), when an allocation
  shows up in a benchmark, when a new field lands on a record that
  every tick reads, or when the user says "медленно", "тормозит",
  "100 ботов" or "optimise".
---

# Performance and data-oriented design (swarm)

The stretch goal is 100 concurrent bots on one process against a live
Mobius server. The fleet benchmark (`internal/swarm/fleete2e`)
verifies that goal end-to-end; every hot-path change must keep it
reachable.

## The four hot paths

1. **Packet apply** - every received packet mutates the tracker under
   the bot mutex. The packet parsers (`internal/swarm/packets/*`) are
   pure; the connection layer (`internal/swarm/connection`) calls the
   public `state.Bot` Apply API. A new packet handler pays one mutex
   acquisition and the field copies; avoid per-packet allocations.
2. **Snapshot encode** - the SSE stream fires `AppendSnapshotJSON` on
   every version change. The encoder walks the live state under the
   read lock, reuses the caller's `dst` buffer, and the per-element
   view structs stay on the stack. Steady state allocates nothing per
   event. See `state/snapshot_live.go`.
3. **Hunt tick** - target search (`NearestAttackableConstrained`),
   loot scan, movement decision. The scans walk the dense hot array
   by index so the CPU prefetcher sees a contiguous block.
4. **Pathfinder** - 99 s of the test suite is here. Benchmarks live
   in `internal/swarm/pathfind/*_bench_test.go`.

## The SoA split (state/objects_store.go)

```go
type objectStore struct {
    hot  []objectHot   // position, speed, target, dead, in-combat
    cold []objectCold // name, title, template id, max HP/MP
    index map[int32]int32
}
```

Both halves share the same slot index; the store keeps them
length-locked. Removals swap the last record of each half into the
freed slot so the arrays stay dense (no holes). A new field lands in
the half its reader visits - a per-tick field read in `cold` defeats
the split.

The `index` map is the only random-access path; a linear scan by
object id is a performance bug.

## Zero-allocation encode contract

`AppendSnapshotJSON` and `snapshotJSONSizeLocked` together pre-size
the one allocation the encode pays (the `dst` buffer growth) and
allocate nothing per element. The historical snapshot-copy path paid
~26 KB of garbage per event on a 100-npc bot - the comment on
`AppendSnapshotJSON` records that regression. Match the bar:

- Reusable `dst []byte` parameter, return grown slice.
- View structs (`ObjectSnapshot`, `CharacterSnapshot`, ...) stay on
  the stack of the encode call, never escape.
- `strconv.AppendInt` / `strconv.AppendUint` for the integer fields,
  not `fmt.Sprintf` (the `perfsprint` linter enforces this).
- The size estimate (`snapshotJSONSizeLocked`) over-allocates
  slightly; the caller passes `dst[:0]` so the capacity is reused
  across versions.

## The benchmark gate

```bash
go test -bench=. -benchmem -count=5 ./internal/swarm/state/...
go test -bench=. -benchmem -count=5 ./internal/swarm/pathfind/...
SWARM_FLEET_E2E=1 go test ./internal/swarm/fleete2e/ \
    -bench . -benchtime 30x -timeout 30m
```

- Every `*_bench_test.go` reports `-benchmem`.
- A change to a hot path compares allocations before/after; a
  regression of more than 10% on a fleet benchmark blocks the change
  until explained in the commit message.
- The `prealloc`, `perfsprint`, `gocritic` linters enforce the easy
  wins (slice preallocation, `strconv` over `fmt`, the obvious
  rewrites). Do not silence them with `//nolint` for allocation
  reasons - fix the allocation.

## What is NOT a hot path

The web UI handlers (one per SSE client, human-paced), the bot
startup (one per session), the gear planner (one per shopping trip),
the snapshot copy used by the periodic endpoint (one per HTTP
request). Optimize the four hot paths before touching these. Do not
micro-optimize code that runs once per minute.

## When to profile

When a benchmark regresses and the cause is not obvious, profile:

```bash
go test -benchmem -cpuprofile=cpu_out -memprofile=mem_out \
    -run='^$' -bench '^BenchmarkEncryptor_Write$' \
    github.com/melg8/swarm/internal/swarm/crypt
go tool pprof -http=localhost:8080 mem_out
```

Read the flame graph for the allocation hot spot; the encoder pattern
above is the usual fix.

## When to add a benchmark

A new code path in a hot area (a new snapshot field, a new packet
handler, a new hunt scan) arrives with a `*_bench_test.go` that
exercises it with `-benchmem`. The benchmark is the regression gate
the next change in that area will be measured against.
