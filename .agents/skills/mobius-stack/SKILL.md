---
name: mobius-stack
description: >-
  Bringing up, verifying and debugging the local Mobius C1 server stack
  the swarm bot runs against (login 2106, game 7777, MariaDB 3306) and
  running the end-to-end bot test. Use when a task needs the live
  server, when connections fail, when the bot cannot login or stays
  stuck, for E2E verification of behavior changes, and at the start of
  any debugging round - even if the user just says "сервер упал" or
  "проверь на живом сервере".
---

# Mobius C1 stack (swarm)

## Mandatory first step of every task

Any task in this repository starts by deploying and verifying the
stack, even pure code tasks - nearly everything needs the live login
(2106), game (7777) and MariaDB (3306) to reproduce and validate:

```bash
bash tools/swarm_fast_deploy.sh   # idempotent, ~90 s from a blank sandbox
ss -ltn | grep -E ':(2106|7777|3306) '
```

Deploy is successful only when the script printed
`STACK_READY: login :2106, game :7777, db :3306`, all three ports
listen and the schema has 75 tables. Deeper check: `task` equivalent of
`tools/mobius_e2e.sh 45` printing `E2E_OK` (bot in the world for 45 s,
graceful SIGINT shutdown).

On the Windows dev host the stack runs from the manual deployment in
`E:\work\lineage_workspace_fresh` (see the AGENTS.md section "Windows
host deployment" for the layout, launchers and caveats - XAMPP
MariaDB, dist `.vbs` launchers, `PathFinding = 2`).

## Operational pitfalls (each was a real debugging session)

- **Never probe the login port by connecting to it.** The login server
  flood protector counts accepted sockets per IP and silently drops
  the client afterwards (the bot sees EOF on the first read). Readiness
  checks use `ss -ltn`, never `/dev/tcp`. A clean bot reconnect clears
  the counter.
- **A restarted login server loses the game server registration** for
  seconds; `tools/mobius_start.sh` waits for the `Updated Gameserver`
  line. An empty server list means "wait", not "broken".
- **Stuck `account in use` self-heals** when the login connection
  closes; retrying is the fix, restarting the login server is the fast
  one.
- **SIGTERM on the game server is slow** (the shutdown hook saves the
  world); wait for the process to disappear before restarting.
- **A character left auto-attacking by an abrupt disconnect keeps the
  corpse selected server-side**; forced attacks on that object id then
  fail forever with ActionFailed. The bot recovers by selecting a
  different target (the engage skip logic). See
  `docs/development_log.md`, round 23 addendum.
- **After any self TeleportToLocation the client must answer
  Appearing (0x30)** or the server silently ignores every move request
  - the bot does this in `applyTeleport`; a "bot is stuck after
  revive/teleport" report starts there.

## Logs and diagnostics

- Stack logs: `../logs/login.log`, `../logs/game.log`,
  `../logs/mariadb.log`, `../logs/bot.log` (relative to the repo).
- The local server checkout carries diagnostic-only log lines
  (DEATHLOG, GUARDDMG, ATTACKLOG, MOVEDBG). Gameplay patches of the
  server are forbidden - the server is the reference, the bot adapts;
  only pure logging patches are allowed (see the server integrity
  rules in AGENTS.md).
- `SWARM_TRACE_PACKETS=1 go run ./cmd/swarm ...` logs every received
  packet id for live protocol debugging.
- The bot test account is `test1`/`test` (auto-created on first login);
  long living characters of that account may be mid-delevel or
  mid-trip - check the state before assuming a bug.

## E2E and sandbox notes

- `tools/mobius_e2e.sh 45` is the reliable end-to-end run: it builds
  the bot, starts the stack, keeps the character in the world and
  sends SIGINT (run bots that must exit gracefully in the foreground
  behind `timeout -s INT` - background jobs inherit SIG_IGN and Go
  cannot catch that).
- The fast deploy unpacks its own OpenJDK/MariaDB/Go under `~/opt` and
  rebinds `tools/mobius_start.sh` / `tools/mobius_e2e.sh` through the
  `JDK_DIR` and `MARIADB_DIR` environment variables.
