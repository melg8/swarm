<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# The acceptance run logs (logs/acceptance)

The hyper-detailed per-run trail of the acceptance test bots. Every
launch of a scenario writes one plain-text file into
`logs/acceptance/` (the `-acceptance-log-dir` flag, empty derives
`<session-dir>/acceptance`, `off` disables) that carries the complete
story of the run: what the test is, how the run was wired, every
packet the temp bot received and sent, every decision the hunt loop
made, every verdict the monitor check evaluator reached and how the
run ended.

The problem it solves: the acceptance scenarios run on the live
Mobius stack for minutes, and the web UI test log is a 40-line ring
while the state dump is a point-in-time snapshot. When a run hangs,
drifts or passes without the bot really doing the thing, neither can
answer what happened at the moment things went wrong. The run log is
built so the owner attaches the one file to the next agent prompt
("it does not work" or "the test passed but the bot never did X")
and the file alone carries every fact needed to diagnose the run
without reproducing it.

## The file

`logs/acceptance/<test-id>-<yyyymmdd>-<hhmmss>-<pid>-<seq>.log` - one
file per run (a restart of a running test opens a fresh file, the
generation counter separates them). The file is plain text, one
event per line, written by a dedicated goroutine through a buffered
queue: the session reader and the hunt loop never wait on disk, and
a burst past the queue depth drops lines with a visible counter in
the verdict instead of stalling the bot.

## The layout

The header is the contract of the run, the same card the web UI
shows:

```
==================================================
 swarm acceptance run log
==================================================
 test      : farm-readiness - farm readiness - the level 15 walk
 account   : temp1
 run       : generation 3
 started   : 2026-09-20 15:45:12.345 (unix 178990812)
 timeout   : 20m0s
 go        : go1.26.8
 args      : ./swarm -acceptance farm-readiness

--- test description (the web ui card) ---
Start: ... Flow: ... Pass: ...

--- pass / fail contract ---
the scenario publishes its check list at the run start (the
first [check] block below): the monitor rewrites those checks
from the live tracker state, the run passes when the scenario
returns nil (every check done) and fails on the scenario
error, the timeout (20m0s) or a cancel.

--- environment ---
  build: swarm 2ca864a (branch feature/new-pathfind-alternative)
  login server: 127.0.0.1:2106
  database: 127.0.0.1:3306/l2jmobiusc1 (user root)
  proxy: login 127.0.0.1:3333, game 127.0.0.1:3334, 0 clients
  navigator: navmesh hybrid (grid + mesh)
  bots in registry: 13

--- event log ---
format: [wall clock +offset] [tag] message
```

The first `check` event is the contract publication (the scenarios
own their check lists, so the list lands with the first lines of the
event log):

```
[15:45:12.400 +0.055s] [check] the pass/fail contract of this run: ...
          [ ] online      entered the world
          [ ] equipped    wears weapon, armor and jewels
          [ ] skills      learned every affordable lesson
          ...
```

The event log lines follow one shape -
`[15:46:10.200 +57.855s] [recv 0x22 NpcInfo] ...` - with the tag
naming the source:

| Tag | Source | What it answers |
| --- | --- | --- |
| `scenario` | the scenario narration | the phase the test is in, the injections, the waits |
| `hunt` | the hunt loop logger (the "Hunt:" lines) | every decision the bot made and why |
| `db` | the character reset | the exact SQL that prepared the server side |
| `session` | the connection layer | the dials, the handshakes, the world entry, the reconnects |
| `recv` | the inbound packet tap | everything the bot saw, decoded (names, positions, vitals) |
| `send` | the outbound packet tap | everything the bot did - every walk click, attack, buy, bypass |
| `check` | the monitor evaluator | the moments the tester decided a condition holds or slipped |
| `sample` | the state sampler (10 s) | the position, vitals, wallet, phase, target, zone between the packets |
| `verdict` | the run end | the outcome, the reason, the final check list, the packet counters |

The packet decoders resolve names through the npcdata catalogs (the
npc template levels, the item names, the skill names, the system
message texts), so a recv line reads `npc "Kaboo Orc" (object
268501317, template 20538, level 5) at (43896,54088,-3640)
attackable aggressive` instead of raw ints. The opcode names resolve
per direction (the byte 0x21 is RequestBypassToServer from the
client and CharSelected from the server). Unknown opcodes fall back
to the size and the first 48 bytes of hex.

The verdict block ends the file:

```
[16:01:00.500 +948.155s] [verdict] FAILED after 15m48s
        error: the bot never entered the world within 90s (status offline)
        [x] online (entered the world): online as temp1
        [ ] equipped (wears weapon, armor and jewels): 0 armor slots...
        checks: 1 of 6 done
        packets: 8421 received, 1203 sent, 0 log lines dropped
```

## How to read a suspicious run

1. The verdict block names the failure and the elapsed time; the
   checks list shows which conditions never held.
2. The `sample` lines give the ten-second state curve: a flat
   position with a standing phase between two samples brackets
   exactly where the bot froze; grep the offsets around them.
3. The `send` lines around a freeze show what the bot tried (the
   walk clicks and their refused `ActionFailed` answers ride the
   adjacent `recv` lines); the `hunt` lines explain the reasoning.
4. The `session` lines bracket every reconnect; a gap between two
   session lines with no packets in between is a dead or silent
   connection window.
5. The packet counters of the verdict tell whether the run was
   starved of traffic (a server that stopped answering) or flooded.

## The wiring

The `botlog` package (internal/swarm/acceptance/botlog) owns the
writer and the decoders; the acceptance manager opens one file per
run in `execute`, mirrors the narration, the check flips and the
verdict through it, and the session runner chains the inbound tap
after the proxy recorder (the client replay history stays intact)
and installs the outbound tap (`GameClient.SetSendTap`, the plaintext
handoff before the encryption). The state sampler reads the tracker
every ten seconds. The login handshake rides the summary `session`
lines (five packets per session, the fixed C1 exchange - the game
session is where the per-packet detail pays).
