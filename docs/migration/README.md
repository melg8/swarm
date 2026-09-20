# Go → Rust transfer analysis for swarm

An evidence-based feasibility assessment of porting swarm from Go to Rust,
produced 2026-09-20 by an AI coding agent (the project's own development
model). The full illustrated report lives in
[rust-transfer-analysis.html](rust-transfer-analysis.html) — open it in a
browser; it is a single self-contained page with no external dependencies.

## Scope of what would be ported

| Fact | Value | Measured how |
| --- | --- | --- |
| Non-test Go LOC | 130,520 | scripted census of 199 files |
| Test LOC | 49,317 (ratio 0.378) | 197 test files |
| Generated code (`npcdata`) | 62,438 LOC | `Code generated` headers |
| Effective hand-written logic | ~68k LOC | census minus generated |
| Direct deps | 4 (+1 indirect) | go.mod |
| cgo / C deps | 0 | census |
| Interfaces / goroutines / channels / locks | 7 / 63 / 58 / 208 | pattern scan |
| `unsafe` uses | 12 (LE wire casts) | pattern scan |
| Test suite without the game server | 24/24 packages pass | `go test ./...` on this machine |

## Matched benchmarks (2-core sandbox, Go 1.23.4 vs Rust 1.98.1)

| Benchmark | Go | Rust | Edge |
| --- | --- | --- | --- |
| JSON marshal+unmarshal, ops/s | 132,909 | 530,064 | Rust 4.0× |
| SHA-256, MB/s | 1,381 | 1,531 | Rust 1.11× |
| Worker pool, jobs/s | 130,263 | 116,653 | Go 1.12× |
| Spawn 200k tasks, ms | 318 | 208 | Rust 1.5× |
| Spawn 200k tasks, peak RSS | 529 MB | 78.5 MB | Rust 6.7× |
| HTTP echo req/s (64 clients) | 33,891 | 40,956 | Rust 1.21× |
| HTTP mean latency | 1.89 ms | 1.56 ms | Rust 1.21× |

## Developer-loop economics (the agent-critical dimension)

| Metric | Go | Rust |
| --- | --- | --- |
| Cold build (toy service w/ deps) | 14.5 s | 78 s (151 crates resolved) |
| Incremental rebuild → binary | 0.088 s (toy) / 0.57 s (real swarm) | 6.3 s release, 0.27 s check |
| Compiler diagnostics for the same 6 planted bugs | 372 B / 9 lines (62 B/error) | 1,685 B / 44 lines (421 B/error) |
| First-try compile of the benchmark suite | yes | 3 fix-compile rounds |

## Verdict

- **Technical feasibility: 7/10.** Ecosystem coverage of every feature the
  project uses is complete; no cgo; the offline test suite is a full
  behavioral spec for a port; the 12 `unsafe` wire casts would end *safer*
  as `from_le_bytes`.
- **Strategic fit: 3/10.** Performance is not a bottleneck at the stated
  100-bot target (both runtimes overshoot by orders of magnitude); Go is a
  stated project goal (`AGENTS.md` forbids Rust); agent iteration economics
  strongly favor Go (10–100× faster feedback, 4.5–6.8× cheaper error
  context); a realistic port is ~18 weeks of frozen roadmap.
- **Recommendation: stay on Go.** Revisit only when a defined trigger fires:
  fleet ≥1,000 bots per process, a profiled CPU hot spot that survives
  optimization, FFI/WASM/static-distribution product needs, or a production
  race the detector misses. If one fires, go hybrid (sidecar crate), not
  big-bang.

## Reproducing the numbers

```bash
cd benchmarks/go-vs-rust/go-bench   && go build -o go-bench . && ./go-bench json|cpu|conc|spawn|http
cd benchmarks/go-vs-rust/rust-bench && cargo run --release -- json|cpu|conc|spawn|http
```

Raw outputs of the runs behind this report: `benchmarks/go-vs-rust/results/`.
