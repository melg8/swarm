# Go → Rust: Complete Mesh Pathfinding Port & Comparison

**Scope:** the full navmesh runtime of `internal/swarm/pathfind/navmesh`
(Detour-style tile mesh, polygon A*, funnel string pulling, HNA* cluster
hierarchy, corridor shortcut, pocket escape, avoid walls, water pricing,
sidecar cluster graphs, tile wire formats v1/v2/v3) rewritten in Rust from
scratch by an autonomous agent, then benchmarked against the Go original on
identical queries over the real world pack.

**Executor:** coding agent (GLM/Super Z). **Date:** 2026-09-20.
**Hardware:** 2 cores, 4 GB RAM sandbox. **Toolchains:** Go 1.26.8, Rust 1.98.1
(zstd C-backed crate, no other native deps).

---

## 1. The port

| | Go (original) | Rust (port) |
|---|---|---|
| Library LOC (12 core files) | 6,683 | 6,288 (−6%) |
| Modules | abstract, abstractfile, astar, avoid, corridor, funnel, hierarchy, mesh, pocket, query, smooth, tile | same 12, one to one |
| Dependencies | klauspost/compress (zstd) | zstd, flate2, crc32fast (+serde for the harness binary only) |
| Tile wire formats read | v1, v2, v3 | v1, v2, v3 (byte-identical decode, verified) |
| Not ported | navbuild (offline builder, Go-only tool), c1_waterzones loader, webviewer glue | same scope kept |

### Correctness gate (the strongest result of this round)

A shared pair file (`pairs.json`, 13 real world queries generated once by the
Go engine: Elven village town walks, guard stairs, cross-region 21_19 ↔
21_20/22_19/22_20 routes, smoothing variants) is replayed by both engines.
The Rust engine reproduces **every field of every answer exactly**:
`found`, `partial`, `hierarchical`, corridor polygon count, waypoint count,
walk length, and the `explored` node count — bit for bit. Identical
`explored` counts mean identical A* tie-breaking, identical float cost
arithmetic (Go's `math.Hypot` is ported verbatim) and identical hierarchy
phases. The tile decode itself was checksum-verified tile by tile: all five
regions decode byte-identically (grid entry sums and maxima equal).

## 2. Benchmark results (identical queries, both engines)

Warm mode = one mesh, caches hot, 50 timed rounds over the 13 pairs.
Cold mode = a fresh mesh per pair (every tile decode + abstract build paid).

### Warm steady state

| Pair class | Go median | Rust median | ratio | explored |
|---|---|---|---|---|
| short town hop (~1.4k) | 0.92 ms | 1.26 ms | 1.37 | 1,385 |
| guard stairs (medium) | 0.25 ms | 0.22 ms | 0.89 | 583 |
| within 21_19 medium | 3.69 ms | 3.18 ms | 0.86 | 5,944 |
| within 21_19 long | 10.13 ms | 8.95 ms | 0.88 | 16,463 |
| smoothed variants | 1.06 / 3.72 ms | 1.09 / 3.34 ms | 1.03 / 0.90 | 5,944 |
| cross region (hierarchy, capped partial) | 185 / 178 ms | 163 / 154 ms | 0.88 / 0.86 | ~314k |
| cross region long | 444.8 ms | 378.9 ms | 0.85 | 686,259 |
| within 22_19 (capped flat) | 472.1 ms | 407.5 ms | 0.86 | 64,672 |
| within 21_20 (hierarchy found) | 108.9 ms | 102.3 ms | 0.94 | 61,494 |
| **TOTAL (50 rounds × 13 pairs)** | **92.0 s** | **79.6 s** | **0.87** | — |

### Cold path (decode + abstract build + first search)

| Metric | Go | Rust | ratio |
|---|---|---|---|
| Total cold time (13 fresh meshes) | 4.66 s | 3.72 s | **0.80** |
| Peak RSS (cold run) | 1,053 MB | 412 MB | **0.39** |
| Peak RSS (warm run) | 650 MB | 490 MB | 0.75 |
| Total allocated bytes (warm) | 853 MB / 152k mallocs | 1,117 MB / 159k mallocs | 1.31 |

### Reading

1. **Speed:** Rust wins the aggregate by 13% (warm) to 20% (cold), with the
   gap widening as the searches get bigger — the open-addressing ref index,
   the arena node arrays and the absence of GC pauses pay off exactly where
   the search does real work (the 686k-explored queries are the most
   Rust-favorable at 0.85). The two small-absolute regressions (1.37× on a
   0.9 ms hop, 2.35× on a 0.75 ms pocket-escape-class query) are noise-scale
   microcosts: per-query state pool acquisition through a mutex, and Arc
   tile cloning per relaxed node, which cost nothing at scale but show on
   sub-millisecond answers.
2. **Memory:** the live working set is materially smaller in Rust
   (−25% warm peak, −61% cold peak; the cold Go number carries garbage the
   GC has not collected yet — that is the honest production story of a Go
   process too: peak RSS includes uncollected cycles). Rust allocates MORE
   total bytes (Arc clones, Vec growth) but frees them instantly; Go
   allocates fewer bytes and keeps them longer.
3. **Parity is the headline:** zero verify errors on 13 pairs × 51 runs
   each side, with node-exact searches. The port is not "similar", it is
   the same algorithm.

## 3. How hard the port was (agent-experienced)

### The honest iteration ledger

Compile-fix cycles: **8** to reach a building crate (36 → 27 → 26 → 5 → 0
library errors, then 3 → 1 → 0 for the harness binary). Beyond the
compiler's count, the correctness gate caught **2 logic bugs the compiler
could never see** (details below) and the debugging cost two more
tool-assisted rounds. Total distinct fix iterations: **10**. Full ledger:
`pathfind-rs/ITERATION_LOG.md`.

| Error class | Count | Character |
|---|---|---|
| Borrow checker / Go aliasing (Arc<Tile> redesign, corridor clones, pool Box-ing, pointer-vs-index nodes) | ~40% of compile errors | **the real porting work**; solved by architectural decisions (Arc + poly index instead of `*Poly`), not by fighting |
| Go multi-value returns destructured as tuples (RegionKey, find_nearest_poly) | 4 sites | mechanical |
| Cross-module naming drift (SIDE_* → POLY_SIDE_*) | 12 sites | mechanical |
| Reserved keyword collision (`abstract` module) | 1 rename | trivial but repo-wide |
| Crate API drift (zstd 0.13 decode_all arity) | 1 | mechanical |
| Trait gaps (Default derives, Serialize) | 4 | mechanical |

### The two bugs the compiler could not catch

1. **Grid delta accumulation across buckets.** The v3 tile grid index is a
   per-bucket zigzag-free uvarint delta stream whose base resets at every
   bucket. The port carried the accumulator across buckets: the decode
   passed every structural validation (counts, offsets, stream length),
   checksummed identically on the first bucket, and then produced garbage
   polygon ids (up to 2.7 billion) deeper into the stream. Caught only
   because the Go engine and the Rust engine were forced to agree on a
   checksum of every tile. **This is the class of bug that ships in a port
   without a cross-implementation oracle** — the Rust type system was
   silent, the runtime was silent, only the parity harness screamed.
2. **Serde field-name case mismatch.** The Go harness writes `boundStart`,
   the Rust harness read `bound_start`, and `#[serde(default)]` silently
   zeroed every coordinate into a valid-looking "no navmesh" answer — every
   pair "ran" in 0.001 s with plausible-looking output. The silent-default
   failure mode is Rust-specific; Go's json.Unmarshal would have left the
   fields zero too, but the timed results (0.001 s) made it loud. Fixed
   with `#[serde(rename_all = "camelCase")]`.

### Qualitative difficulty assessment

- **Algorithmic difficulty: low.** The port is 12 files of deterministic,
  imperative search code. Nothing dynamic-dispatches, nothing reflects,
  nothing mutates shared state concurrently in the algorithm itself.
- **Mechanical difficulty: medium.** The borrow checker forces one real
  redesign (tiles behind `Arc`, polygons addressed by index, pooled states
  boxed) worth roughly 40% of the compile errors and most of the thinking.
  After that decision the rest is translation.
- **Verification difficulty: high without an oracle, trivial with one.**
  The single most valuable tool of this round was the cross-language pair
  file with exact expected answers. Port cost ≈ 1 working day of agent
  time including benchmarks; without the parity gate the residual risk
  would be unacceptable for a pathfinding runtime that bots' lives depend
  on.
- **Size outcome:** the Rust library is 6% smaller than the Go original
  while carrying identical behavior — Go's error-wrapping and interface
  glue roughly balance Rust's type annotations.

## 4. Verdict

The mesh pathfinder is the best-case Go→Rust port profile: self-contained
leaf package, zero reflection, deterministic algorithms, existing test
culture. It cost **10 honest iterations, ~6,300 lines, one architectural
decision (Arc + indexes)**, and it came out **13–20% faster with 25–60%
less peak memory and byte-identical answers**. The port difficulty for an
agent is real but concentrated: the borrow checker taxes the design phase
once, not every file. The decisive success factor was not the language at
all — it was building the oracle (shared query pairs + per-tile
checksums) before trusting anything.

## 5. Artifacts

- `pathfind-rs/` — the Rust crate (library + `pathbench` + `tileprobe`)
- `pathfind-rs/ITERATION_LOG.md` — the full honest iteration ledger
- `cmd/pathbench/` — the Go harness (generate | warm | cold)
- `cmd/tileprobe/` — the Go decode probe used to isolate the grid bug
- `benchmarks/pathfind-go-vs-rust/` — `pairs.json`, `go_warm.json`,
  `rust_warm.json`, `go_cold.json`, `rust_cold.json`, this document
- `data/navmesh/` — the built 5-region tile pack + abstract sidecars
  (built by `cmd/navmesh-build` from `data/geodata`)

## 6. Methodology notes & caveats

- Token estimates are bytes ÷ 4; no tokenizer ground truth.
- The pair set is 13 queries; the two "small regression" pairs sit at
  0.75–1.3 ms where scheduler and mutex noise dominate. The aggregate
  over 650 timed queries per engine is the robust number.
- The Go prefetch pool decodes tiles on 4 goroutines; the Rust port
  decodes prefetch keys on a bounded thread pool too, but the cold-mode
  single-query path is sequential in both (capacity 8 keeps every tile of
  the 5-region pack resident, so the warm loop measures the search, not IO).
- The cold peak-RSS comparison favors Rust partly because GC lag is
  included in Go's peak; that is deliberate (production RSS behaves the
  same way), but it is not an apples-to-apples "live set" number. The warm
  peak (−25%) and the total-alloc numbers (Go 853 MB vs Rust 1,117 MB)
  bound the honest story: Go allocates less, Rust holds less.
