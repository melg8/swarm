---
name: e2e-repro
description: >-
  How to set up a reproducible scenario for an E2E test of the swarm
  bot against the live Mobius C1 server: the three repro levels
  (in-process fake server, single-bot live, 100-bot fleet), the
  character creation recipe, the position-and-target setup and the
  E2E verification contract. Use when a fix needs a live verification,
  when the user says "проверь на живом", when a regression needs a
  scenario that cannot be faked, or when an E2E test flaps.
---

# E2E reproduction (swarm)

The bot cannot patch the Mobius server (gameplay changes are out of
bounds, see the server integrity rules in AGENTS.md). Reproducing a
scenario in E2E therefore uses the bot's own state API and the live
server's vanilla behavior - never a server-side GM command.

## Three repro levels (pick the cheapest that answers the question)

1. **In-process fake server** (the `connection/game_test.go` pattern).
   A scripted game session that feeds the parser the exact packet
   sequence of the scenario. Cheapest, fastest, deterministic. Use
   when the bug is in the parser, the apply path or the hunt decision
   - any code path that does not need real server latency, real NPC
   AI or real world geometry.

2. **Single-bot live E2E** (`tools/mobius_e2e.sh 45`). Builds the
   bot, starts the stack, keeps the character in the world for 45 s,
   sends SIGINT, checks the graceful shutdown. The smoke test of
   every change. ~47 s with the stack up. Use for "the bot connects,
   enters the world, hunts, shuts down cleanly" - the contract that
   must never break.

3. **Fleet E2E** (`internal/swarm/fleete2e`, opt-in via
   `SWARM_FLEET_E2E=1`). 100 live sessions against the stack with the
   autonomous hunt loop and the reconnect supervisor. ~10 minutes
   setup. Use when a hot-path change could regress the 100-bot
   stretch goal (the `performance` skill).

A change ships with the cheapest level that proves it. A parser fix
needs level 1; a snapshot encoder change needs levels 1 and 3 (the
fleet benchmark covers the encode under load); a connection lifecycle
change needs level 2; a hunt behavior change needs levels 1 and 2.

## The character creation recipe (every repro shares it)

The MVP character is an elven fighter. The same recipe the cmd/swarm
supervisor and the fleet launcher use:

```go
const (
    elfRaceID    = 1  // PlayerRace.ELF
    elfFighterID = 18 // PlayerClass.ELVEN_FIGHTER
    elfFemale    = 0
    defaultHair  = 0
    defaultFace  = 0
)
```

The account auto-creates on first login (the server's
`AutoCreateAccounts` is enabled by default in Mobius C1). A scenario
that needs a specific level or inventory builds it through the
character creation packet sequence, not a server-side GM command.

## Setting up a specific scenario (the bot-side approach)

The bot cannot teleport a character, spawn a mob or change its level
through a GM command - those are server-side gameplay changes the
integrity rules forbid. Instead, the scenario is built from the bot
side:

- **A specific position**: the bot walks there. The pathfinder
  (`internal/swarm/pathfind`) computes the route from the geodata; a
  test can set the bot's starting position through the public Apply
  API (a `TeleportToLocation` packet applied to a fresh `state.Bot`)
  and let the hunt loop take over.
- **A specific target**: the bot selects it. The target selection is
  a hunt-loop decision (`hunt/loop.go::NearestAttackableConstrained`);
  a test forces a target by calling `state.Bot.SetTarget(objectID)`
  (or its public Apply equivalent) and asserting the next tick
  attacks it.
- **A specific inventory**: the bot receives `ItemList` packets that
  set the inventory. A test feeds the parsed `ItemList` fields through
  the Apply API; the live server does the same when the bot buys or
  loots.
- **A specific world state**: the bot receives the `NpcInfo`,
  `CharInfo`, `DeleteObject` packets that build the world view. A
  test replays them through the Apply API to construct the exact
  scan input the hunt tick sees.

When the scenario needs a live NPC AI (aggression, clan assist, guard
retaliation), level 2 or 3 is the answer - the in-process fake server
does not model the server-side decisions. The dump-state-repro skill
covers capturing a real scenario from a long-lived server and
replaying it deterministically.

## The E2E verification contract

A change is E2E-verified when:

- `tools/mobius_e2e.sh 45` prints `E2E_OK` (the single-bot contract:
  the bot enters the world, runs for 45 s, exits on SIGINT with code
  0). This is the merge gate.
- When the change touches a hot path,
  `SWARM_FLEET_E2E=1 go test ./internal/swarm/fleete2e/ -bench .
  -benchtime 30x -timeout 30m` does not regress the encode or the
  scan sweep by more than 10%.
- When the change is a hunt behavior fix, the dump-state-repro test
  (the fixture of the dump that exposed the bug) passes.

## When an E2E test flaps

A flaky E2E is a real bug, not a tolerance to widen. The common
causes (each has a fix in the codebase already):

- **Login cooldown** after an emergency logout - the reconnect
  supervisor backoff handles it; a test that races the login is the
  bug, not the server.
- **Flood protector** silently dropping the socket after 50
  connections from one IP - never probe the login port by connecting
  to it, use `ss -ltn`.
- **Server list empty** for seconds after a login server restart -
  wait, do not fail.
- **A character left auto-attacking by an abrupt disconnect** keeps
  the corpse selected server-side - the bot's engage skip logic
  recovers by selecting a different target.

See the `mobius-stack` skill for the full pitfall list and the
`go-verify-loop` skill for the deterministic-time discipline (injected
clocks, not real wall-clock sleeps).
