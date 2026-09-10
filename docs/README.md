# swarm documentation index

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The rules, conventions and the load-bearing facts live in the root
[`AGENTS.md`](../AGENTS.md) - start there. The documents below hold
the implementation-level detail of each subsystem; read the one that
matches the area you are about to touch.

| Document | Area |
| --- | --- |
| [deployment.md](deployment.md) | Bringing the Mobius C1 stack up: the fast deploy, the bootstrap, the script inventory, the Windows host layout, stack logs, the operational pitfalls |
| [hunting.md](hunting.md) | The autonomous hunt: hunt loop, combat safety, blind engage recovery, multi-zone hunting, auto equipment, shop strategy execution, town trips, deleveling, live-validated facts |
| [shopping_strategy.md](shopping_strategy.md) | The shop strategy reasoning, the prices, the purchase phases and the was/is level journey comparison |
| [webui.md](webui.md) | The web interface: launch modes, map rendering, movement interpolation, HUD, equipment and shop queue widgets, interactivity, snapshot encoding, state tracker internals, repro harnesses |
| [pathfinding.md](pathfinding.md) | The geodata pathfinder: format, engine, deviations from the original, test UI, benchmarks, geodata visualization |
| [navigation_analysis.md](navigation_analysis.md) | The measured gaps on the road to universal A to B world navigation |
| [proxy.md](proxy.md) | The MITM client proxy: running it, the l2.ini recipes, the protocol the client sees, the relogin handoff, debugging |
| [protocol_description.md](protocol_description.md) | The wire protocol: packet framing, login and game flows, per-packet layouts (legacy l2j-lisvus origin; Mobius C1 is the current reference) |
| [project_description.md](project_description.md) | The long term design goals and scalability ideas |
| [development_log.md](development_log.md) | The permanent record of the development rounds with root cause analyses |
| [agent_progress.md](agent_progress.md) | The active task handover file (crash-safe progress tracking); finished entries move to agent_progress_archive.md |
| [quality_review_and_agent_prompts.md](quality_review_and_agent_prompts.md) | The 2026-09-07 architecture review and improvement program (historical snapshot) |
| [webui_modernization_proposal.md](webui_modernization_proposal.md) | The pending web UI modernization proposal (awaiting user approval) |

Data folder notes: `data/geodata/Readme.txt` (geodata provenance) and
`data/icons/Readme.txt` (icon pack provenance).
