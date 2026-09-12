<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# BACKLOG: the task queue and the claim protocol

The single work queue for all agent sessions (up to five run in
parallel on `feature/proxy-server`, no direct communication). This
file is the arbiter: work is claimed by commit, leased for a bounded
time and handed over through resume notes. The reasoning lives in
`docs/agent_selforganization.md` (section 4); the goal ladder lives in
`docs/ROADMAP.md`.

## Protocol

- A task moves `todo -> in_progress -> done` (or `blocked`). The claim
  is a commit that sets the status, the agent label and the time, and
  pushes immediately; whoever's push lands first owns the task.
- If a fetch+rebase shows the task was taken in the meantime, pick the
  next one. Do not fight over tasks.
- Lease: an `in_progress` task with no progress entry in
  `docs/agent_progress.md` for more than 4 hours (judged by commit
  timestamps) is abandoned; any agent may re-claim it after reading
  the resume notes.
- Scope: a task lists the files/packages it may touch. Do not edit
  outside the declared scope; cross-cutting concerns become their own
  tasks. Journals (`agent_progress.md`, this file) are append-only to
  their own sections.
- Closing a task requires: tests for the touched packages green,
  `golangci-lint run --new` clean, a `docs/agent_progress.md` entry
  and, when the task changes behavior or protocol, a live
  verification (`tools/mobius_e2e.sh 45` or an acceptance scenario).
- Every task references a ROADMAP milestone. A task that advances no
  milestone needs a justification line.

Task template:

```markdown
### T-XXX: <imperative one-liner>

status: todo | in_progress | blocked | done
milestone: M<N>
priority: P1 | P2 | P3
deps: T-YYY, ...
scope: <packages/files>
estimate: <hours, must fit a 2h session with the round gates>

<What and why, the acceptance line, the references.>

claimed: <agent label> <UTC time>
resume: <where the previous agent stopped, what is next>
```

## Tasks

### T-001: the soak metrics trail

status: in_progress
milestone: M1
priority: P1
deps: -
scope: internal/swarm/acceptance/, cmd/swarm/, tools/, docs/

Add the `soak` acceptance scenario: a long supervised farm run (N
hours, fresh account) that appends one JSON line per run to
`runs/metrics.jsonl` (date, scenario, duration, start/end level,
XP per hour, deaths, adena, stuck events, PASS/FAIL). Include the
stagnation watch of T-008 in the pass criteria. Ship a tiny
`tools/progress_report.sh` that renders PROGRESS.md from the metrics
tail, the BACKLOG statuses and `git log --oneline -20`.

claimed: soak-z 2026-09-12 07:27Z
resume: claim just pushed; next: design the soak scenario, the
  metrics writer, the stagnation guard and the progress report, then
  implement in the acceptance package, wire the CLI flag and verify
  with a smoke run.

### T-002: the stagnation watch

status: in_progress
milestone: M1
priority: P1
deps: -
scope: internal/swarm/hunt/, internal/swarm/state/, docs/

The hunt loop logs explicit events when the character gains no XP for
M minutes or holds the same position for K minutes (both constants
named and unit tested). The events surface in the web UI event feed
and in the bot log with the current phase, so a silent livelock
becomes a loud line. Cover with unit tests (the clock seam of the
loop tests) and a live note in the dev log.

claimed: zai-agent 2026-09-12 07:36 UTC
resume: claimed fresh, no code yet - the acceptance claim of T-001
was lost to soak-z fairly (their push landed first), taking T-002;
the hunt loop structure study is next

### T-003: the level milestone scenario

status: todo
milestone: M1
priority: P2
deps: T-002
scope: internal/swarm/acceptance/

Generalize farm-readiness: a scenario that DB-injects a character at
an arbitrary level with the zone-appropriate gear and passes when the
character reaches level N+1 within the time budget. This is the
building block every later milestone acceptance reuses.

### T-004: the quest subsystem research

status: todo
milestone: M2
priority: P2
deps: -
scope: docs/

Read the Mobius C1 Java sources and write `docs/quest_protocol.md`:
the quest packet flow (the quest list, the NPC html dialog packets,
quest state transitions, quest item drops), with a link to every
relevant Java class, in the shape of `docs/protocol_description.md`.
Research only: no code changes. List the follow-up code tasks in the
resume notes.

### T-005: the hypotheses registry convention

status: todo
milestone: M1
priority: P3
deps: -
scope: AGENTS.md, docs/navigation_analysis.md

Add the "Hypotheses / Unknowns" convention to the AGENTS.md
documentation rules: every unverified server assumption must live in
a registry section with a verification plan, and code relying on it
must reference it. Seed the registry with the open items of
`docs/navigation_analysis.md` (swimming semantics, doors, the
gatekeeper graph) so the pattern starts populated.

### T-006: the 20-25 band survey

status: todo
milestone: M3
priority: P3
deps: -
scope: docs/, tools/generate_hunt_zones.py

Survey the Mobius spawn data for the 20-25 mob band reachable from
the elven lands, in the shape of the elven zone survey: territories,
towns, merchants, teachers. Output: a survey document plus the
follow-up task list (the registry generation itself is a separate
task once M1 is green).

### T-007: the PROGRESS.md page

status: todo
milestone: M1
priority: P3
deps: T-001
scope: tools/, docs/

The one-page human dashboard: milestone ladder green/red by the last
acceptance results, the last soak metrics, the active BACKLOG tasks,
the last commits. Generated by the tooling of T-001, committed after
every milestone-relevant run so the page history is the project
history.
