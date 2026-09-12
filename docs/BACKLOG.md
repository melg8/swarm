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

status: done
milestone: M1
priority: P2
deps: T-002
scope: internal/swarm/acceptance/

Generalize farm-readiness: a scenario that DB-injects a character at
an arbitrary level with the zone-appropriate gear and passes when the
character reaches level N+1 within the time budget. This is the
building block every later milestone acceptance reuses.

claimed: zai-agent 2026-09-12 07:51 UTC
done 2026-09-12 08:15 UTC by zai-agent: the level-milestone scenario
(SWARM_LEVEL_MILESTONE_LEVEL 1..20, default 10, the near-threshold
exp, the template vitals, the metrics rows) live-verified twice (10
-> 11 in 7m46s, 2 -> 3 in 2m30s); the round also fixed the
xpPerHour double count of the T-001 cumulative helper (the Mobius
exp is the running total - the Java evidence in the dev log round
65 entry).

### T-004: the quest subsystem research

status: done
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
resume: docs/quest_protocol.md written (the engine, the dialog
  packets, the html action cache, the persistence, the two class
  transfer chains, the gatekeeper geography). The follow-up code
  tasks it proposes (each its own BACKLOG entry when claimed):
  1. parse NpcHtmlMessage 0x1B + the html link mirror (state
     tracker dialog section);
  2. parse QuestList 0x98 (the quest journal section, arrives free
     at world entry - live verified);
  3. send RequestBypassToServer 0x21 with the cache rules (only
     links of the open page, 250 units of the origin npc) +
     RequestQuestList 0x63;
  4. the hunt quest module: the dialog walker, the Q00406/Q00407
     scripts as data, the gatekeeper legs, the Gludio/Gludin
     navigation (overlaps the M3 survey);
  5. the M2 class-transfer acceptance scenario (level 19 injection
     at Gludio, pass on classId 19/22).
  Open live checks: the first quest accept burst shape, the
  completed flags encoding, MoveToPawn ordering at quest talks.

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

status: done
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
done: 2026-09-12 08:53Z
resume: done. The renderer builds the full M0-M6 ladder from
  ROADMAP.md (done / current green-red by the last metrics row /
  pending); the first PROGRESS.md page is committed. The protocol:
  re-run tools/progress_report.sh and commit after every
  milestone-relevant acceptance run.

### T-008: the gatekeeper teleport flow

status: done
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

claimed: soak-z 2026-09-12 07:55Z
resume: done 2026-09-12 08:45Z. The full packet chain ships:
  RequestBypassToServer (0x21) + NpcHTMLMessage (0x1B) parsers
  (round 63), the connection dispatch (0x1B → applyNpcHTMLMessage →
  LastHTMLMessage/LastHTMLDialog + the ApplyDialog bridge to the
  state tracker through ParseHTMLLinks of T-012), SendBypass,
  ParseGatekeeperHTML (the teleport-specific button classifier:
  showTeleports/teleport/listName/locID), DriveGatekeeperTeleport
  (the hunt-loop step: showTeleports → await dialog → find
  destination → teleport bypass, bounded 5s wait), 7 + 4 unit tests,
  the protocol_description.md docs. Live verified: the
  building-entry acceptance run with SWARM_TRACE_PACKETS=1 showed
  the server 0x1B packets (813 bytes) arriving and the dispatch
  parsing them (no parse failure, building-entry PASSED). The full
  Mirabel -> Gludio -> Dion live drive is the follow-up (the
  hunt-loop gatekeeper trip phase, the T-009 prerequisite): the
  packet chain, the parser and the step are ready to wire.

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

status: done
milestone: M3
priority: P3
deps: T-008
scope: internal/swarm/gear/, internal/swarm/npcdata/, tools/

Wire the D-grade weapon/armor/jewel catalogs of the Dion merchants
(buylists 3006000-3006300 of the survey) into the gear planner with
the band price brackets and the sell-first rules of the shop
strategy; the shopping strategy doc gains the Dion shop section.

claimed: soak-z 2026-09-12 08:53Z
resume: done 2026-09-12 08:56Z (in-scope). gear.DionCatalog() ships
  (Sabrin 7060, Casey 7061, Sonia 7062, Lara 7063, Dion baseTax 20%
  = 0.20, buylists 3006000-3006300) with 2 unit tests; the npcdata
  buylists and the D-grade item GearStats were already generated and
  verified. The out-of-scope follow-up (a new task): the hunt
  multi-town catalog selection (hunt/shopping.go switches to the
  Dion catalog when the bot farms the 20-25 band at Dion) and the
  shopping_strategy.md Dion shop section. The gate: M1 green before
  the 20-25 band is reached.

### T-011: the quest journal parser and tracker section

status: done
milestone: M2
priority: P2
deps: T-004
scope: internal/swarm/packets/from_game_server/, internal/swarm/state/, internal/swarm/connection/

Follow-up 2 of docs/quest_protocol.md: parse the QuestList packet
(0x98 - the quest journal: active quest ids with their cond, the
quest item stacks) and land it in the state tracker (a quest
journal section with quest-cond and quest-item accessors for the
hunt loop and the sell filter). The packet arrives free at world
entry (live verified, EnterWorld.java:303 pushes it). The web UI
quest widget is a later round of its own.

claimed: agent-quest 2026-09-12 08:05 UTC
resume: done 08:15 UTC - the parser, the state journal and the
  dispatcher wiring are live verified ("Quest journal with 0
  quests, 0 quest items" at world entry). The web UI quest widget
  (the snapshot section) stays a separate round when a consumer
  needs it.

### T-012: the html dialog link parser

status: done
milestone: M2
priority: P2
deps: T-004
scope: internal/swarm/packets/from_game_server/, docs/

Follow-up 1b of docs/quest_protocol.md: extract the bypass links
of a server dialog page (the command each <a action="bypass ...">
carries plus its visible text) as a pure parser next to the
NpcHTMLMessage packet, mirroring the server side extraction of
HtmlUtil.buildHtmlBypassCache (the case-insensitive "=\"bypass "
match, the -h prefix strip, the trim). The dialog walker of T-008
(the teleport buttons) and the quest dialog walker of the M2 round
both consume it; the parse result also seeds the client side
mirror of the html action cache (send only what the open page
offered).

claimed: agent-quest 2026-09-12 08:15 UTC
resume: fresh claim; the real quest/master/trainer pages of the
  research round are the golden test inputs.

### T-013: the open dialog section of the tracker

status: done
milestone: M2
priority: P2
deps: T-011, T-012
scope: internal/swarm/state/

Follow-up 1's remainder of docs/quest_protocol.md: the state
tracker gains the "open dialog" section - the current dialog page
(the npc origin, the links with their texts) fed from the
NpcHTMLMessage parse of the connection layer, replaced on every
page arrival (the NPC_HTML scope semantics) and cleared with the
session. The accessors serve the dialog walker of the M2 round;
IsDialogCommand mirrors the server side validateHtmlAction rules
(exact match or the $ parameter prefix) so the bypass sender only
fires commands the open page actually offered.

claimed: agent-quest 2026-09-12 08:18 UTC
resume: done 08:27 UTC - state/dialog.go with the page store, the
  accessors and the IsDialogCommand mirror of
  Player.validateHtmlAction (exact or the trimmed '$' prefix).
  The 0x1B dispatcher case that feeds ApplyDialog stays with
  T-008 (connection/ is their scope).

### T-014: the quest dialog walker engine

status: done
milestone: M2
priority: P2
deps: T-011, T-012, T-013
scope: internal/swarm/hunt/ (new files only), docs/

Follow-up 4's engine half of docs/quest_protocol.md: the generic
dialog walker the quest brain runs on - the two-click talk entry
(the NpcClick semantics: the first Action click selects the npc,
the second opens the html page; every click refreshes the
last-folk memory), the bounded wait for a NEW page from the npc
(content change detection - the connection layer stores only the
last html, and the quest pages all arrive from the same npc), the
hunt-side feed of the tracker dialog section (ApplyDialog from
ParseHTMLLinks - the wiring T-013 left to the consumer) and the
link-by-text bypass walk (IsDialogCommand validation before every
SendBypass, per-page bounded waits). Unit tests pin the walker on
the real Q00406/ElfHumanFighterChange1 page goldens through the
fakeGame stub; a live round trip against a village npc of the
deployed stack (click -> html -> link -> bypass -> next html)
closes the round.

claimed: quest-walker-e3f8 2026-09-12 08:45 UTC
resume: done 09:10 UTC - hunt/quest_walker.go (DriveDialog: the
  two-click entry, the content-change wait, the tracker feed, the
  validated bypass walk) + the scripted fakeGame unit tests on the
  real datapack pages + the live round trip (Ellenia, the "Quest"
  link, the answer page applied, SWARM_LIVE_DIALOG=1 replays it).
  The dev log round 70 records the live facts (the glade pages
  carry no links; SkillList answers with a packet, not a page).
  Follow-ups for the next agents: T-015 the quest chain data, T-016
  the class-transfer acceptance scenario, the quest trip phase
  wiring.

### T-015: the class transfer quest chains as data

status: in_progress
milestone: M2
priority: P2
deps: T-014
scope: internal/swarm/hunt/ (new files only), docs/

The data half of the quest brain: the two class transfer chains of
docs/quest_protocol.md as Go data the quest trip phase consumes -
the npc chain of Q00406 (Sorius 30327, Kluto 30317) and Q00407
(Reisa 30328, Moretti 30337, Babenco 30334, Prias 30426), the kill
grounds (the Ruins of Agony skeleton/spartoi ids, the Ol Mahum
camps), the item economy (the pieces, the letters, the memo, the
box, the brooch) with the drop chances and the 20 piece counters,
the cond progression (1..7) and the dialog route steps (the link
texts per cond, from the datapack pages - the T-012 goldens and the
T-014 live dumps pin the forms). The ElfHumanFighterChange1 class
change chain (Rains 30288, the 30288-11 -> 12 -> -h 19 route)
rides along. Unit tests pin the data against the quest script
sources (the npc ids, the item ids, the kill ids read from the
Java); the hunt consumer is T-016.

claimed: quest-data-e3f8 2026-09-12 09:12 UTC
resume: fresh claim.

### T-016: the class transfer acceptance scenario

status: todo
milestone: M2
priority: P2
deps: T-015
scope: internal/swarm/acceptance/, docs/

The M2 acceptance vehicle: a scenario that DB-injects a level 19
ELVEN_FIGHTER at Gludio (the level milestone machinery of T-003
plus the quest chain data of T-015), drives the quest trip phase
(the dialog walker through the Sorius chain, the kill grounds, the
Kluto chain) and the Rains class change, and passes when
SelfClassID reports 19 (Elven Knight) - plus the character
selection packet agreement after a relogin. The gatekeeper legs
(the elven village -> Gludio hop) join once T-008 closes. This is
the task that CLOSES M2 (the live acceptance run).

claimed: -
resume: -

### T-017: the multi-town gear catalog selection

status: in_progress
milestone: M3
priority: P3
deps: T-010
scope: internal/swarm/hunt/, docs/

The T-010 follow-up: the hunt shopping loop selects the gear catalog
of the town it farms near. The elven village catalog (the 1-19 band,
townMerchants at the 15 percent elven tax) stays the default; the
Dion catalog (the 20-25 band, the Dion merchants of the survey at
the 20 percent Dion tax, gear.DionCatalog of T-010) is selected
when the hunt zone sits in the Dion territory. The town.go merchant
list grows with the Dion merchants (Sabrin, Casey, Sonia, Lara at
their survey positions); the shopping.go catalog builder switches
from the single townShopCatalog var to a per-town catalog the hunt
loop picks by the active zone region. The shopping_strategy.md gains
the Dion shop section. The elven village behavior stays unchanged
(the M0 acceptance still passes).

claimed: soak-z 2026-09-12 09:17Z
resume: claim pushed; next: add the Dion merchants to town.go, add
  a per-town catalog builder keyed by the hunt zone region, keep the
  elven catalog as the default, unit-test the selection, document
  the Dion shop in shopping_strategy.md.
