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

status: done
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
resume: done 2026-09-12 07:45Z. The soak scenario (temp7, the
  supervised hunt loop, SWARM_SOAK_MINUTES env, default 10), the
  stagnation guard (no XP for 10m / no move for 5m, an
  acceptance-package tracker read with the now seam), the
  runs/metrics.jsonl writer (O_APPEND, one JSON line, the local C1
  XP table for the hourly rate) and tools/progress_report.sh ship
  together. Unit tests green, lint:new clean, the live 2m smoke run
  PASSED (temp7 1->2, 9984 XP/h, 0 deaths, 0 stuck, 28 adena). The
  8h M1 proof is the follow-up: SWARM_SOAK_MINUTES=480 then commit
  the PASS row + PROGRESS.md to close M1.

### T-002: the stagnation watch

status: done
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
done 2026-09-12 07:53 UTC by zai-agent: the watch lives in
hunt/stagnation.go (20 min xp / 10 min position windows, one line
per window through logf, the tick defer coverage, the manual/offline
reset), the dump diagnostics carry xpStallForMs and
positionStallForMs, 9 unit tests, the docs in hunting.md/webui.md,
the dev log round 63, the live e2e + autonomous smoke verified.

### T-003: the level milestone scenario

status: in_progress
milestone: M1
priority: P2
deps: T-002
scope: internal/swarm/acceptance/

Generalize farm-readiness: a scenario that DB-injects a character at
an arbitrary level with the zone-appropriate gear and passes when the
character reaches level N+1 within the time budget. This is the
building block every later milestone acceptance reuses.

claimed: zai-agent 2026-09-12 07:51 UTC
resume: claimed fresh right after T-002 (mine) closed and unlocked
the dep; next: study the soak scenario of T-001 and the zone ladder
per level, then design the injection (level N, near-threshold xp,
the zone-appropriate start) and the N+1 watch

### T-004: the quest subsystem research

status: in_progress
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

claimed: agent-quest 2026-09-12 07:35 UTC
resume: fresh claim; the plan: read the Mobius quest engine sources
  (the quest state machine, the quest packets, the html dialog flow,
  the class transfer script of the elven fighter) and write
  docs/quest_protocol.md in the shape of protocol_description.md.

### T-005: the hypotheses registry convention

status: done
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

claimed: hypotheses-e3f8 2026-09-12 07:38Z
done: 2026-09-12 07:55Z
resume: done. The AGENTS.md "Hypotheses and unknowns" section holds
  the convention and the seed entries H-001..H-004 (swimming,
  gatekeepers, boats, doors); navigation_analysis.md links its open
  items to the ids. Follow-up (next pathfind-touching task): the
  waterCostMultiplier comment gains its H-001 reference; every new
  unverified server assumption now mints an H-NNN entry before the
  relying code lands.

### T-006: the 20-25 band survey

status: done
milestone: M3
priority: P3
deps: -
scope: docs/, tools/generate_hunt_zones.py

Survey the Mobius spawn data for the 20-25 mob band reachable from
the elven lands, in the shape of the elven zone survey: territories,
towns, merchants, teachers. Output: a survey document plus the
follow-up task list (the registry generation itself is a separate
task once M1 is green).

claimed: band-survey-e3f8 2026-09-12 07:58Z
done: 2026-09-12 08:29Z
resume: done. docs/band_20_25_survey.md is the survey (transport,
  grounds, merchants, teachers, follow-ups); generate_hunt_zones.py
  grew the --survey mode (registry mode byte identical); the
  follow-ups T-008/T-009/T-010 are in the queue; the aggression
  default correction landed in navigation_analysis.md.

### T-007: the PROGRESS.md page

status: in_progress
milestone: M1
priority: P3
deps: T-001
scope: tools/, docs/

The one-page human dashboard: milestone ladder green/red by the last
acceptance results, the last soak metrics, the active BACKLOG tasks,
the last commits. Generated by the tooling of T-001, committed after
every milestone-relevant run so the page history is the project
history.

claimed: progress-page-e3f8 2026-09-12 08:41Z
resume: claim pushed; next: run tools/progress_report.sh, verify the
  page against the task acceptance (the milestone ladder green/red,
  the metrics, the tasks, the commits), fix the gaps, commit
  PROGRESS.md.

### T-008: the gatekeeper teleport flow

status: todo
milestone: M3
priority: P2
deps: -
scope: internal/swarm/connection/, internal/swarm/hunt/, docs/

Implement the gatekeeper dialog + teleport travel of the band survey
(docs/band_20_25_survey.md): read Teleporter.onBypassFeedback,
RequestBypassToServer, NpcHtmlMessage and TeleportToLocation (the
H-002 verification), then drive a live bot Mirabel -> Gludio -> Dion
and implement the packet chain the bot needs (the html bypass
commands, the fee deduction, the arrival Appearing answer the
teleport shares with the village revive).

claimed: -
resume: -

### T-009: the 20-25 zone registry generation

status: todo
milestone: M3
priority: P3
deps: T-008
gate: M1 green (the backlog protocol of the survey task)
scope: tools/generate_hunt_zones.py, internal/swarm/hunt/

Extend the zone generator with the Dion spawn sources of the survey
(CrumaMarshlands, ExecutionGrounds, PlainsOfDion) and generate the
spawn-true squares of the 20-25 band in the shape of zones_elven.go
(the survey territories are the input polygons; the band table of
the survey doc is the acceptance reference).

claimed: -
resume: -

### T-010: the band gear catalogs

status: todo
milestone: M3
priority: P3
deps: T-008
scope: internal/swarm/gear/, internal/swarm/npcdata/, tools/

Wire the D-grade weapon/armor/jewel catalogs of the Dion merchants
(buylists 3006000-3006300 of the survey) into the gear planner with
the band price brackets and the sell-first rules of the shop
strategy; the shopping strategy doc gains the Dion shop section.

claimed: -
resume: -
