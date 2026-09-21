# swarm documentation index

<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

The rules, conventions and load-bearing facts live in the root
[`AGENTS.md`](../AGENTS.md) - start there. The documents below hold the
implementation-level detail of each subsystem; read the one that matches
the area you are about to touch, not all of them.

## Getting the stack running

| Document | Area |
| --- | --- |
| [deployment.md](deployment.md) | Bringing the Mobius C1 stack up: the fast deploy, the bootstrap, the script inventory, the Windows host layout, stack logs, the operational pitfalls |
| [launch_config.md](launch_config.md) | The launch configuration file (the -config flag): the JSON format, the flag precedence rules, the fleet composition of bot types and counts, the default config |
| [proxy.md](proxy.md) | The MITM client proxy: running it, the l2.ini recipes, the protocol the client sees, the relogin handoff, debugging |

## The bot

| Document | Area |
| --- | --- |
| [hunting.md](hunting.md) | The autonomous hunt: hunt loop, combat safety, blind engage recovery, the hexagon cell hunting, multi-zone hunting, auto equipment, shop strategy execution, town trips, deleveling, live-validated facts |
| [hunting_cells.md](hunting_cells.md) | The hexagon cell hunting: the uniform ground partition, the enemy-first roam, the visibility budget, the ripeness-paced neighbor rotation, the mesh endpoint, the registry invariants |
| [shopping_strategy.md](shopping_strategy.md) | The shop strategy reasoning, the prices, the purchase phases and the was/is level journey comparison |
| [pathfinding.md](pathfinding.md) | The geodata pathfinder: format, engine, deviations from the original, test UI, benchmarks, geodata visualization |
| [navmesh.md](navmesh.md) | The navmesh pathfinding of feature/new-pathfind: the Detour-style runtime, the Go mesh builder, the tile format, the live hybrid integration |
| [navigation_analysis.md](navigation_analysis.md) | The measured gaps on the road to universal A to B world navigation |
| [quest_protocol.md](quest_protocol.md) | The quest subsystem protocol (Mobius C1, the M2 research): the quest machine, the packet flows, the live traces |
| [band_20_25_survey.md](band_20_25_survey.md) | The 20-25 band survey (M3 preparation): the hunting grounds, the travel, the shopping, the learning |
| [protocol_description.md](protocol_description.md) | The wire protocol: packet framing, login and game flows, per-packet layouts (legacy l2j-lisvus origin; Mobius C1 is the current reference) |

## The web interface

| Document | Area |
| --- | --- |
| [webui.md](webui.md) | The web interface: launch modes, map rendering, movement interpolation, HUD, equipment and shop queue widgets, interactivity, snapshot encoding, state tracker internals, repro harnesses |
| [session_journal.md](session_journal.md) | The persistent session journal and the session dump report: the JSONL record of the whole run, the web UI button, the offline CLI |
| [acceptance_botlog.md](acceptance_botlog.md) | The acceptance run logs: the hyper-detailed per-run trail of the test bots (the contract, the environment, every packet, every monitor verdict) an agent reads from one file |

## History and plans

| Document | Area |
| --- | --- |
| [ROADMAP.md](ROADMAP.md) | The goal ladder: the milestones with binary live-acceptance criteria; every change must advance one |
| [project_description.md](project_description.md) | The long term design goals and scalability ideas |
| [development_log.md](development_log.md) | The permanent record of the development rounds with root cause analyses |
| [recast_pathfinding.md](recast_pathfinding.md) | The recastnavigation research: what Detour gives the geodata pathfinder, the converter, the prototype, the migration verdict |
| [fastpath_research.md](fastpath_research.md) | The fast route planning research: the measured baseline, the hierarchical options, the 10 second budget |
| [quality_review_and_agent_prompts.md](quality_review_and_agent_prompts.md) | The 2026-09-07 architecture review and improvement program (historical snapshot) |
| [codebase_review_2026-09-20.md](codebase_review_2026-09-20.md) | The 2026-09-20 fresh-eyes review: the verified findings (the journal data-loss path, the read-lock cache writes, the session lifecycle edges, the never-activated CI, the handover file) and the prioritized P0/P1/P2 backlog |
| [hunting_system_redesign.md](hunting_system_redesign.md) | The spot-anchored farming research behind the retired spot mode: respawn awareness, efficiency scoring, the zone visibility measurements - superseded by hunting_cells.md (figures in [hunt_analysis/](hunt_analysis/)) |

## Pending redesigns (not implemented yet)

| Document | Area |
| --- | --- |
| [webui_modernization_proposal.md](webui_modernization_proposal.md) | The web UI modernization proposal (awaiting user approval) |

## The working process

| Document | Area |
| --- | --- |
| [agent_progress.md](agent_progress.md) | The active task handover file (crash-safe progress tracking); finished entries move to agent_progress_archive.md |
| [agent_progress_archive.md](agent_progress_archive.md) | The archive of the finished agent rounds (the append-only history the handover file points back to) |
| [ci_workflow.yml](ci_workflow.yml) | The CI verification gate designed for GitHub Actions (build, vet, test, whitespace, the full lint, the race slice, govulncheck): inactive until a workflow-scoped token copies it to .github/workflows/ci.yml |
| [agent_feedback_loops.md](agent_feedback_loops.md) | The audit of the feedback an autonomous agent receives: the verification loop, the live acceptance, the observability, the process memory |
| [flake_ledger.md](flake_ledger.md) | The searchable memory of observed test flakes: every flake gets one row (the cause, the fix, the pin that closed it) |

Data folder notes: `data/geodata/Readme.txt` (geodata provenance) and
`data/icons/Readme.txt` (icon pack provenance).

The README screenshots live in [images/](images/) - captured from the
live local stack (a headless browser against a running `-hunt -bots 3 -web`
session) and cropped to the interface region each feature shows, so the
panels stay readable at the width the README renders at.
