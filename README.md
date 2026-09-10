<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# swarm

An out-of-game (OOG) multi-instance proxy botting tool written in Go:
it emulates a swarm of characters connected to a Lineage 2 world. The
target server is a locally hosted
[L2J Mobius](https://gitlab.com/MobiusDevelopment/L2J_Mobius/)
emulator, the `L2J_Mobius_C1_HarbingersOfWar` module (Chronicle 1).

The bot connects, creates an elven fighter when needed, enters the
world and farms autonomously 24/7 (hunt loop, gear upgrades, shopping
trips, deleveling control), re-establishing lost sessions with a
growing backoff. A built-in web UI (map, HUD, inventory, shop queue)
observes and controls every session, and a MITM proxy lets a real C1
client attach to the bot character at any moment.

## Quick start

```bash
bash tools/swarm_fast_deploy.sh   # brings the Mobius stack up (~90 s, idempotent)
go run ./cmd/swarm -hunt -web 127.0.0.1:8080   # one bot, web UI at :8080
go run ./cmd/swarm -hunt -bots 3              # a fleet of three
```

The default account is `test1`/`test` (auto-created on first login).

## Development

- **Read [`AGENTS.md`](AGENTS.md) first** - the rules, conventions and
  the documentation map for working on this repository (humans and AI
  agents alike).
- [`docs/`](docs/) holds the subsystem documentation: deployment,
  hunting, shopping strategy, web UI, pathfinding, proxy, protocol,
  the development log and the long term goals. `docs/README.md` is the
  index.
- Routine commands are wrapped in `Taskfile.yml` (go-task): `task
  check:all` (lint + test), `task run:app`, `task test:cover`.

## License

MIT - see [license.md](license.md).
