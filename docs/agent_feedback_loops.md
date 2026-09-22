<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# The feedback loops of an autonomous agent session

This document answers one question: when an agent works on this
repository alone (no human watching), what signal tells it that the
change is right, that it is not slower, that the bot still works
live - and which signals are missing? The inventory was taken on
2026-09-19 from the tree state of the feature branch; every claim
references the file that provides the mechanism. The improvement
lists at the end are the actionable part: each item names the
cheapest sufficient implementation.

## The inventory: what feedback the agent receives today

### Static code signal (seconds)

- `go build ./...` - the compile verdict, the first gate of every
  round. Not packaged into a task; the verify-loop skill mandates it
  by prose.
- `go vet ./...` - cheap static checks; same prose-only packaging.
- `golangci-lint run` (the `task lint` CI gate, ~39 s full, ~2 s
  with `--new` on a warm cache) - about 45 enabled linters
  (`.golangci.yml`): complexity containment (`funlen`, `cyclop`,
  `gocognit`, `maintidx`), error hygiene (`errorlint`, `nilerr`,
  `wrapcheck` as an opt-out), performance (`prealloc`,
  `perfsprint`), test quality (`testifylint`, `thelper`,
  `tparallel`), plus `nolintlint` which fails the build on a stale
  or unused `//nolint` directive - the debt markers clean
  themselves.
- `task fmt:check` - the spaces-only whitespace gate (tabs fail the
  build), enforced by `cmd/gofmt-spaces` and
  `tools/normalize_whitespace.sh`.
- The generated-files contract: `internal/swarm/npcdata/*.go` carry
  the canonical `// Code generated ... DO NOT EDIT.` marker and must
  never surface lint findings (the go-verify-loop skill explains
  how a broken marker fails).

### Test signal (tens of seconds to minutes)

- `go test ./...` (~123 s cold, pathfind dominates) - 263 test
  files, deterministic by policy: injected clocks, the fake-server
  pattern for protocol flows (`connection/game_test.go`), the
  "fix the determinism, not the tolerance" rule from the
  go-verify-loop skill.
- `task test:race` - the race detector suite (needs cgo; the
  Windows host is exempt by the documented tooling caveats).
- `task test:cover` - coverage numbers printed per package. No
  threshold, no artifact, no trend (see the improvement list).
- Benchmarks in 28 `*_bench_test.go` files with `-benchmem` and the
  written allocation-comparison rule (AGENTS.md, the performance
  section); the fleet regression gate `fleete2e` (gated by
  `SWARM_FLEET_E2E=1`, 10-minute 100-session setup, opt-in).
- The external comparison harness `tools/l2bridge` (the Rust cargo
  project): the same query through the raasta and condor navmesh
  engines, so the production numbers have competitors to be judged
  against (docs/navmesh.md section 7).

### Live stack signal (minutes)

- `tools/mobius_e2e.sh [seconds]` - the whole path in one
  invocation: build with the stamped identity, stack bring-up, N
  seconds in the world, SIGINT, the graceful shutdown check,
  prints `E2E_OK`. Designed for restricted sandbox shells
  (foreground execution, no background survivors).
- `tools/proxy_e2e.sh` - the same for the client proxy path,
  prints `PROXY_E2E_OK`.
- The acceptance manager (`internal/swarm/acceptance`): named
  scenarios (farm readiness, full dress, class transfer, village
  escape, relay, building entry, level milestones, soak) against
  temp accounts temp1..temp11, a 2 s condition monitor with bounded
  waits (`onlineWait` 90 s), run through the web UI Scenarios tab
  or `POST /api/acceptance/tests/{id}/run`; the parallel "run all"
  respects the account partition.
- The soak scenario writes one JSONL row per run into
  `runs/metrics.jsonl` (`soak_metrics.go`): scenario, duration,
  start/end level, XP per hour, deaths, adena, stuck events,
  status, failReason - the quantitative trail M1 of
  `docs/ROADMAP.md` requires, 26 rows deep today.
- The milestone ladder of `docs/ROADMAP.md`: a milestone closes
  only by a live acceptance run, the project progress equals the
  highest green milestone, and every unit of work must reference
  one - the strongest anti-self-deception rule in the repository.

### Runtime observability (the bot reports itself)

- The build identity (`internal/version`: Branch, Commit, Dirty,
  BuildTime) printed by the bot log and carried by every state
  dump - a log tail pairs with the exact code state, so a
  reproduction is never attributed to the wrong build.
- The session journal (`docs/session_journal.md`): append-only
  JSONL per process, rotating at 64 MB with background gzip, plus
  the in-memory aggregate (hourly buckets, level marks, per-mob
  fight stats, trip counters, stall lists) and the compact report
  the agent actually reads after a 24 hour run.
- The state dump (`GET /api/bots/{id}/dump`,
  `tools/dump_state.sh`): build identity, character sheet, the buff
  strip (every active effect with its level, remaining seconds and
  the derived applied age), world objects by distance, inventory,
  walk plan, combat events, chat, the 600 entry event log - the
  offline reproduction artifact of the dump-state-repro skill.
- The web API surface (`internal/swarm/webserver/server.go`):
  `/api/bots`, the per bot state/events/dump/commands, the fleet
  kills, the stats, the acceptance endpoints, the proxy status,
  the geodata and navmesh tiles, the pathfind query endpoints -
  every one of them is a probe an agent can curl from a script.
- The hunt self-diagnostics: the `noteMoveStart` 3 s watchdog, the
  refusal evidence collection, the stall backdating, the
  stagnation watch of the M1 acceptance criteria; `ActionFailed`
  is parsed into the log instead of being discarded (commit
  e8644ce), so a server refusal is a first-class event.
- The server-side logging patches (DEATHLOG / GUARDDMG / MOVEDBG)
  - the only allowed server changes, pure diagnostics.

### Process memory (across sessions)

- `docs/agent_progress.md` - the crash-safe active task with dated
  per-commit entries; the archive carries the history. The first
  file an agent reads after AGENTS.md.
- `docs/development_log.md` - 88 rounds of root cause analyses;
  the rule "read it before reworking the area" keeps the same bug
  from being rediscovered (and re-fixed) by a fresh session.
- The hypotheses registry (AGENTS.md): an unverified server
  assumption may not live silently in code or comments; it becomes
  an H-NNN entry with a verification plan and closes only by
  evidence.
- `tools/progress_report.sh` renders `PROGRESS.md` (the milestone
  ladder, the last metrics rows, the last 20 commits) - the one
  page the owner reads.
- The skills (`.agents/skills/`): the hand-written playbooks
  (`go-verify-loop`, `mobius-stack`, `packet-recipe`,
  `webui-harness`, `performance`, `dump-state-repro`,
  `e2e-repro`) plus the vendored golang collection with the
  benchmark references (benchstat, ci-regression).
- The measured cycle times in AGENTS.md (deploy ~99 s, lint:new
  ~2 s, test ~123 s, e2e ~47 s, the full loop ~5.5 min) - the
  agent budgets its 2 h session against measured numbers, not
  guesses.

## What is already good

- **The verification loop is explicit, ordered and measured.** The
  go-verify-loop skill names the exact order (build, vet, test,
  fmt:check, lint), the expected durations and the failure
  interpretation rules; AGENTS.md budgets the whole cycle at
  ~5.5 minutes. An agent never has to invent its own bar.
- **The lint gate is strict and self-cleaning.** ~45 linters with
  documented exclusions, and `nolintlint` fails on an unused
  directive, so the suppression markers cannot rot silently.
- **Determinism is a policy, not a wish.** Injected clocks, fake
  servers, and the explicit "a flaky test is a bug in the test"
  rule; the historical poll-test pinning (commit 8034403) shows
  the rule applied.
- **Milestones close only on live evidence.** The ROADMAP ladder
  with binary acceptance criteria and the metrics trail kills the
  classic agent failure of declaring work done because it
  compiles.
- **Every quantitative claim has a trail.** metrics.jsonl rows
  carry failReason; the journal carries the full story; the dump
  carries the build identity; the commit message carries the full
  analysis prose. The evidence chain from "the bot failed" back to
  "the exact code state" is unbroken.
- **Reproduction is a first-class workflow.** dump-state-repro and
  e2e-repro skills, the dump endpoint, the repro harnesses per UI
  area (seven `tools/repro_*` entries), the repro Go tools
  (`repro_gamepath`, `repro_rawpath`, `repro_stuck_trip.sh`).
- **The sandbox reality is documented, not folkloric.** The
  foreground execution rule, the single-invocation E2E scripts,
  the measured subprocess study with re-runnable probe scripts -
  the environment traps are written down where every session
  reads them.
- **Performance has competitors.** The l2bridge harness benches
  the production engine against raasta and condor on the same
  dump, so "faster" is a measured comparative claim, not an
  impression.

## What needs improvement

1. **`task check:all` does not run `go build` or `go vet`.** The
   skill mandates both, but the aggregate task runs only
   lint + test + fmt:check - an agent that trusts the task skips
   two gates the skill demands. Fix: add a `verify` task
   (`go build ./...`, `go vet ./...`, lint, test, fmt:check) and
   make it the documented CI gate; keep `check:all` as the alias.
2. **Coverage is collected and then dropped.** `test:cover` prints
   numbers nobody archives; a regression from 74% to 60% in a
   package passes every gate. Fix: write `cover.out` into
   `runs/`, print the per package delta against the last
   committed run in the same task, fail on a drop beyond a
   threshold the round documents (cheap: `go tool cover -func`).
3. **Benchmarks have no baseline to diff against.** The rule says
   "compare allocations before/after", but the before lives only
   in the agent's memory or the previous commit message. Fix: a
   `task bench:save` that commits `runs/bench-<pkg>.txt` per
   touched package and a `task bench:diff` wrapping benchstat;
   the golang-benchmark skill already documents the workflow, it
   only lacks the repository-side plumbing.
4. **The dashboard goes stale.** `PROGRESS.md` was generated on
   2026-09-12 while metrics.jsonl has runs through 2026-09-13 and
   the ladder statement ("M1 green") predates the class transfer
   FAILs. Fix: regenerate in the same commit as any
   metrics-affecting run (the script comment already asks for
   exactly this) and consider a `task progress` alias so the
   rule is one word.
5. **The acceptance scenarios are serial.** The account partition
   (temp1..temp11) was designed for parallel "run all", yet a
   fresh character farm-readiness costs ~8 minutes of wall time
   per scenario; an agent session pays it serially. Fix: allow
   the manager to run the non-overlapping scenarios concurrently
   (the zone collision list is small and static) - the session
   budget gain is the largest single lever in this list.
6. **The log conventions are prose, not checks.** "Capital first
   letter, no trailing period, Error prefix" is enforced by
   review only. Fix: a small unit test walking the packages for
   the shared log helpers and asserting the format (or a
   gocritic rule where one applies) - imperfect but free.

## What is missing

1. **CI.** There is no `.github/`, no GitLab CI, no hooks - the
   go-verify-loop skill even mentions "the Linux sandbox, CI" as
   a race-suite environment that does not exist. Every gate is
   the local agent's honesty; a broken push is discovered by the
   next session paying the full cycle, or by the owner. Cheapest
   sufficient fix: one GitHub Actions workflow on push/PR (ubuntu
   runner: checkout, Go setup, `go build`, `go vet`,
   `golangci-lint run`, `go test ./... -count=1`, `task
   fmt:check`, `CGO_ENABLED=1 go test -race` on the connection
   and pathfind packages only). Free tier absorbs the ~5 minutes.
2. **A pre-push gate.** The rebase-before-push rule protects
   history, not breakage: parallel agents push to one branch and
   a red push burns the next session's budget. Fix: a `task
   prepush` (build, vet, `lint --new`, fmt:check, the tests of
   the packages the diff touches; ~20-40 s) documented as
   mandatory in the git conventions, optionally installed as a
   pre-push hook by `tools/install_dev_tools.sh`.
3. **A session bootstrap command.** The session-start ritual
   (stamp the timer, verify the deploy, fetch and rebase, list
   the open hypotheses, resume the newest unfinished progress
   entry) is prose spread across AGENTS.md sections; each agent
   re-implements it and forgets a step. Fix:
   `tools/session_start.sh` printing a machine-checkable
   checklist (the stamp file, the port probe, the rebase verdict,
   the open H-NNN ids, the active task headline) - turns the
   ritual into one command with visible output.
4. **A metrics trend reader.** The JSONL trail is written but
   nothing answers "is the bot degrading over the last N runs"
   beyond the last 10 rows of the dashboard table. Fix: extend
   `tools/progress_report.sh` (or a `tools/metrics_trend.py`)
   with per scenario pass rate, XP/hour delta and stuck-event
   delta over the trailing window.
5. **A hypothesis driver.** H-001..H-004 are all open since
   2026-09-12 with written verification plans nobody is reminded
   of. Fix: the session bootstrap above lists them; plus the
   work-protocol rule "close or advance one hypothesis per
   session when the touched area matches its plan".
6. **A flake ledger.** Determinism policy aside, live-stack tests
   occasionally flake (the poll tests were pinned once) and the
   only memory of it is a development log round. Fix: a small
   section in the go-verify-loop skill (or
   `docs/flake_ledger.md`) where a session records the test, the
   observed flake and the pin - searchable by the next agent.

## The priority order for the next rounds

1. CI workflow (the shared gate every other item leans on).
2. The `verify` task with build + vet folded in; `prepush` next.
3. Coverage and benchmark baselines under `runs/`.
4. Parallel acceptance scenarios (the session budget lever).
5. Session bootstrap script; hypothesis and flake ledgers ride it.
6. Metrics trend reader; the PROGRESS.md regeneration rule.

## Implementation status (2026-09-19, the same day)

The whole actionable list landed in one tooling round on the same
feature branch. Where each item lives now:

- Improvement 1 + missing 2: `task verify` (build, vet, lint, test,
  fmt:check) is the canonical full gate, `check:all` its alias;
  `task prepush` (`tools/prepush.sh`, installable as a git hook via
  `tools/install_dev_tools.sh hook`) is the 20-40 s pre-push gate
  the git conventions now mandate before every push; the gate lands as a
  GitHub Actions workflow (kept in `docs/ci_workflow.yml` - the
  push of a workflow file needs the workflow-scoped token, the owner
  copies it to `.github/workflows/ci.yml` verbatim to activate).
  The CI lint step runs `lint --new` for now: the full lint surfaced
  ~200 tracked pre-existing findings (the parallel commits of the
  week landed without the full gate - the exact failure mode this
  document describes), the strict whole-tree lint returns to CI
  when the debt clears.
- Improvement 2: `task test:cover` runs `tools/coverage_delta.sh` -
  the suite with `-coverprofile=runs/cover.out` (gitignored), the
  per package delta table against the committed
  `runs/coverage-latest.txt`, and a hard fail beyond
  `COVER_DROP_LIMIT` (default 2.0 pp); the fresh summary must be
  committed with the change that moved the numbers.
- Improvement 3: `task bench:save` (`tools/bench_save.sh`) commits
  the `-benchmem` output of a touched package into
  `runs/bench-<name>.txt`; `task bench:diff` wraps
  `cmd/benchdiff` (the offline benchstat twin: ns/op, B/op,
  allocs/op, MB/s deltas, the missing-baseline list).
- Improvement 4: `task progress` regenerates PROGRESS.md in one
  word.
- Improvement 5: the CLI gained `-acceptance all-parallel`
  (`Manager.RunAllParallel`): every scenario launches at once on
  its own temp account (the flood protector stagger of the web
  parallel mode), one failure no longer stops the rest, the
  combined error names every failure - the headless run pays the
  wall time of the slowest scenario instead of the sum.
- Improvement 6: `internal/logfmt` parses the module and asserts
  the message conventions at every production log call site
  (`TestRepoLogConventions`, also a CI step); the six existing
  violations it found were fixed, so the rule starts from zero
  debt.
- Missing 3: `tools/session_start.sh` prints the bootstrap
  checklist (the stamp age against the 2h/1h45m budget, the
  fetch/rebase verdict, the ss-based 2106 port probe, the open
  H-NNN ids, the active task headline).
- Missing 4: `tools/progress_report.sh` gained the trailing-window
  trends section (per scenario pass rate, XP/h and stuck-event
  deltas).
- Missing 5: the bootstrap lists the open hypotheses; the AGENTS.md
  hypotheses registry carries the "advance or close one per
  session" rule.
- Missing 6: `docs/flake_ledger.md` opened with the two pinned
  poll-test flakes of 8034403 as the seed rows; the
  go-verify-loop skill names it as the mandatory stop after every
  flake fix.

The serial `-acceptance all` stays available (stop at the first
failure, definition order) for the runs that want the strict
ladder; `all-parallel` is the default recommendation for the agent
sessions.
