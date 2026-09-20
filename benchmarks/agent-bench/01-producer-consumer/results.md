# Agent Benchmark 01: Producer–Consumer Queue (Rust `Arc<Mutex>` vs Go `sync.Mutex`)

**Executor:** coding agent (GLM/Super Z), autonomous, no human review during the task.
**Date:** 2026-09-20. **Question:** how much context (tokens) and how many fix
iterations does an agent burn to deliver the same concurrent program in Rust vs Go?

## Task spec (identical for both languages)

Bounded blocking queue (capacity 8) over shared state guarded by a mutex:
2 producers × 25 items, 3 consumers, graceful shutdown (close flag + broadcast),
main verifies total = 50. Rust: `Arc<Mutex<VecDeque>>` + `Condvar` (std only,
zero dependencies). Go: `struct + sync.Mutex + sync.Cond`. No channels in either
version — the point is to mirror the *shared-state mutex* paradigm.

## Main task results (first draft → working, lint-clean)

| Metric | Rust | Go | Δ |
|---|---|---|---|
| Code written (bytes) | 4,195 | 3,176 | **Rust 1.32×** |
| Code written (lines) | 144 | 143 | ≈ equal |
| Code tokens (est. ÷4) | 1,049 | 794 | Rust +255 |
| Compile → success iterations | **1 (0 fixes)** | **1 (0 fixes)** | tie |
| Compiler feedback consumed | 183 B (clean) | 13 B (clean) | tie-ish |
| Linter pass | clippy: 0 warnings | go vet + gofmt: clean | tie |
| Incremental rebuild (touch + build) | 0.324 s | 0.049 s | **Go 6.6×** |
| Binary size | 518 KB | 2,176 KB | **Rust 4.2× smaller** |
| Correctness (3 runs each) | 50/50 OK | 50/50 OK + `-race` clean | tie |

**Reading:** at this task size a competent agent writes both languages first-try.
Rust costs ~32 % more generated code (type annotations, `Arc::clone` plumbing,
`Result`/`Option` handling), all of it flowing through the context on every
subsequent read.

## Error-recovery drill (3 planted bugs + 1 mirror per language)

Protocol: plant the mistakes agents actually make most in this exact code, compile,
capture diagnostics verbatim, fix, recompile. Nothing cherry-picked.

| Bug | Language | Detection | Diagnostics | Fix cycles |
|---|---|---|---|---|
| R1: moved `Arc` instead of cloning | Rust | compile (E0382 ×2) | 2,004 B, incl. copy-paste fix | 1 |
| R2: `MutexGuard` lifetime/import error | Rust | compile (E0425) | 427 B + 431 B | 2 |
| R3: `Rc` shared across threads | Rust | compile (E0277 `Send`) | 1,278 B + 1,100 B | 2 |
| R4: naive Go→Rust translation, **Mutex dropped** | Rust | **compile-time rejection** (E0308/E0599/E0596 ×4) | 2,029 B | 0 — cannot ship |
| G1: unused import | Go | compile | **66 B**, one line | 1 |
| G2: `int64` passed where `int` expected | Go | compile | **116 B**, one line | 1 |
| G3: **locking silently removed** | Go | **NOT detected by compiler**; caught only by `go run -race` at runtime | 0 B at compile; **7,733 B** race report | 1 (after runtime detection) |

### Per-error context cost (drill totals)

| View | Rust | Go |
|---|---|---|
| Compile-time diagnostics per error | 909 B / error (~227 tok) — driven by R1/R3 cascades | 91 B / error (~23 tok) |
| Full feedback per bug (incl. runtime tooling) | 1,817 tok total, **all pre-runtime** | 1,979 tok total, **7.7 KB of it only after shipping a race** |

## Findings (agent-centric)

1. **The tie that matters: iterations.** For a well-specified small concurrent task,
   both languages cost the same number of fix cycles: one compile, zero fixes.
   The "borrow checker forces N recompiles" folklore applies to unfamiliar trait
   work, not to textbook mutex/condvar code an agent has seen thousands of times.

2. **The asymmetry that matters: failure mode, not verbosity.** Go's compiler is
   nearly silent (66–116 B per error — cheap in tokens) but *silent about the worst
   bug in the experiment*: G3 (lock removed) compiled cleanly and would have shipped
   without a human remembering `-race`. Rust made the identical mistake unrepresentable
   (R4: 4 errors, refuses to build). For agent-driven development — where the review
   layer is another agent and the runtime owner is nobody — **compile-time refusal is
   worth more than token-cheap errors**.

3. **Verbose errors are agent-friendly errors.** Rust's 0.4–2 KB diagnostics contain
   line-precise spans and machine-applicable suggestions (`queue.clone()`,
   `use std::sync::Arc;`). Both R1 and R3 were fixed by literally applying the
   compiler's suggestion. Go's one-liners are cheaper but assume the fix is obvious
   from the error text alone; when it isn't, the agent burns an extra round-trip.

4. **Context tax of Rust is real but bounded:** +32 % code bytes, ~+250 tokens per
   file written, plus re-reading that overhead in every later touch of the file.
   Go pays it back with slower tooling asymmetries: no built-in equivalent of
   `cargo clippy`'s fix suggestions in one command and a 6.6× slower loop is the
   reverse (Go rebuild 0.049 s vs Rust 0.324 s on 2 cores).

5. **Honest caveats.** Two Rust drill bugs needed 2 cycles instead of 1 — partly
   genuine layer-by-layer fixing (R2), partly a harness sed mistake (R3), i.e. agent
   tooling friction, not language friction; both cycles still consumed real context.
   Token counts are estimates (bytes ÷ 4), not tokenizer ground truth. Rust code used
   zero dependencies; real crates would add doc-lookup context on the Rust side.

## Verdict

For **agents**, the per-task cost of Rust in the small is +25–30 % generated tokens
and slower compile loops; the payoff is a compiler that turns the most dangerous
class of agent mistakes (unguarded shared state) into compile errors instead of
production races. Go is cheaper per byte and faster per build, but its safety net
ends where the agent's attention ends.
