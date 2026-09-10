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
| [proxy.md](proxy.md) | The MITM client proxy: running it, the l2.ini recipes, the protocol the client sees, the relogin handoff, debugging |

## The bot

| Document | Area |
| --- | --- |
| [hunting.md](hunting.md) | The autonomous hunt: hunt loop, combat safety, blind engage recovery, multi-zone hunting, auto equipment, shop strategy execution, town trips, deleveling, live-validated facts |
| [shopping_strategy.md](shopping_strategy.md) | The shop strategy reasoning, the prices, the purchase phases and the was/is level journey comparison |
| [pathfinding.md](pathfinding.md) | The geodata pathfinder: format, engine, deviations from the original, test UI, benchmarks, geodata visualization |
| [navigation_analysis.md](navigation_analysis.md) | The measured gaps on the road to universal A to B world navigation |
| [protocol_description.md](protocol_description.md) | The wire protocol: packet framing, login and game flows, per-packet layouts (legacy l2j-lisvus origin; Mobius C1 is the current reference) |

## The web interface

| Document | Area |
| --- | --- |
| [webui.md](webui.md) | The web interface: launch modes, map rendering, movement interpolation, HUD, equipment and shop queue widgets, interactivity, snapshot encoding, state tracker internals, repro harnesses |

## History and plans

| Document | Area |
| --- | --- |
| [project_description.md](project_description.md) | The long term design goals and scalability ideas |
| [development_log.md](development_log.md) | The permanent record of the development rounds with root cause analyses |
| [quality_review_and_agent_prompts.md](quality_review_and_agent_prompts.md) | The 2026-09-07 architecture review and improvement program (historical snapshot) |

## Pending redesigns (not implemented yet)

| Document | Area |
| --- | --- |
| [hunting_system_redesign.md](hunting_system_redesign.md) | The spot-anchored farming research: respawn awareness, efficiency scoring, the zone visibility measurements (figures in [hunt_analysis/](hunt_analysis/)) |
| [webui_modernization_proposal.md](webui_modernization_proposal.md) | The web UI modernization proposal (awaiting user approval) |

## The working process

| Document | Area |
| --- | --- |
| [agent_progress.md](agent_progress.md) | The active task handover file (crash-safe progress tracking); finished entries move to agent_progress_archive.md |

Data folder notes: `data/geodata/Readme.txt` (geodata provenance) and
`data/icons/Readme.txt` (icon pack provenance).

The README screenshots live in [images/](images/) - captured from the
live local stack (a headless browser against a running `-hunt -bots 3 -web`
session) and cropped to the interface region each feature shows, so the
panels stay readable at the width the README renders at.
