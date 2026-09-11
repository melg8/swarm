---
name: dump-state-repro
description: >-
  How to capture, read and reproduce the world state of a bot that
  misbehaves on a long-lived user server. Use when a bot gets stuck,
  stops attacking, picks the wrong target, walks into a wall, misses a
  loot drop or shows any behavior the user reports on a live server -
  the goal is to give the agent the exact world state to fix against,
  without the user having to reproduce the situation by hand.
---

# Dump state for reproduction (swarm)

A bot running on a long-lived user server can misbehave in ways the
fake-server unit tests do not cover: stuck behind a wall, target
selection off, a fight that never lands, a shopping trip that loops.
The user should not have to reproduce the situation by hand - the bot
captures its world state and hands it to the agent.

## What the dump carries (the minimal repro set)

The dump is a plain text report of the live `state.Bot` plus the bot
identity (branch, commit, dirty flag, build time). The sections an
agent needs to reproduce the behavior:

- **identity**: build line (branch, commit, dirty, build time) - so the
  dump pins the exact code that produced it (matches the `Build:`
  log line).
- **bot**: id, status, phase, dumped time, session started, uptime,
  packets, state version.
- **character**: name, level, race, class, position (X/Y/Z/heading),
  HP/MP, target id, moving, sitting, in-combat, stats (STR/DEX/CON/
  INT/WIT/MEN), exp/sp, adena, inventory slot count.
- **attackers**: the living attackable npcs that hold the character
  as their target - the aggro load of the moment.
- **hunting zone**: center, half-size, character inside flag.
- **equipment + bag**: object id, item id, count, bodypart slot name,
  enchant, item name (no icons, no weights - they are NPC data
  lookups).
- **objects**: the world object store sorted by distance to the
  character - object id, kind, name (with title), level, hp/mp,
  position, distance, dead/combat/moving/target/aggressive flags.
  This is the scan input the hunt tick sees.
- **walk plan**: the published walk plan, the origin, every waypoint
  (passed/target/pending), the destination.
- **recent combat**: the combat animation beats still in the TTL
  window.
- **chat**: the recent chat window (40 entries).
- **events**: the deep event log (600 entries) - the mirrored hunt
  loop decisions live here.

The dump is the full `BuildStateDump` output of
`internal/swarm/webserver/dump.go`. The web UI snapshot JSON
(`AppendSnapshotJSON`) is a separate payload for the live SSE stream;
the dump is the human-readable, agent-friendly text format.

## Capturing the dump

Three ways, in order of preference:

1. **The dump_state.sh wrapper** (preferred): the wrapper calls the
   live HTTP endpoint, lists the bots known to the web UI when called
   with no argument, and writes the dump to stdout or a file:
   ```bash
   tools/dump_state.sh                       # list bots known to the web UI
   tools/dump_state.sh test1 -o stuck.txt   # capture to file
   ```
   The dump is the plain text report the web UI "Dump state" button
   copies to the clipboard, served by
   `GET /api/bots/{id}/dump` (`internal/swarm/webserver/dump.go`).

2. **Live HTTP endpoint** (when the wrapper is not available):
   ```bash
   curl -s http://127.0.0.1:8080/api/bots/test1/dump > stuck_bot.txt
   ```
   Same payload as the wrapper. The bot keeps running, so the dump is
   a point-in-time snapshot.

3. **Build identity line** (the bot log): the line right after
   `Starting swarm bot` carries `Build: <branch>:<commit> dirty=<bool>
   time=<iso>`. Pair every dump with the build line so the code state
   is pinned.

The dump format is plain text, NOT JSON: a human-readable report with
sections (character, attackers, hunting zone, equipment, bag,
objects, walk plan, recent combat, chat, events). The deep event
window (600 entries) carries the mirrored hunt loop decisions, so the
story of a stuck session lives minutes back.

## Reproducing the dump in a test

The dump is a plain text report, not a JSON of the storage layout. A
unit test reproduces the behavior in two ways:

1. **Parse the dump back into view structs** (when the test cares
   about the visible state): a small parser reads the character, the
   objects and the walk plan sections into the `state.Snapshot` view
   types, then the test runs the hunt tick against them. This is the
   pattern for "the bot should have picked NPC 11085 at distance 73"
   type assertions.

2. **Rebuild the live state through the Apply API** (when the test
   cares about the storage invariants): replay the parsed packet
   fields into a fresh `state.Bot` so the SoA split, the index map and
   the version counter hold. The `state/*_test.go` helpers already
   pattern this; the dump narrows which packets to replay.

```go
func TestStuckBotRepro(t *testing.T) {
    data, err := os.ReadFile("testdata/stuck_bot.txt")
    require.NoError(t, err)

    // Option A: parse the dump into the view structs.
    snap := dumpparse.State([]byte(data)) // a small helper
    require.Equal(t, "test1", snap.ID)
    require.NotZero(t, snap.Character.TargetID)

    // Option B: rebuild the live state through Apply.
    bot := state.NewBot("test1")
    applyDump(t, bot, data) // a helper that replays the dump fields
    target, dist := bot.NearestAttackableConstrained(1500, nil, nil, 0, true)
    require.NotEqual(t, int32(0), target.ObjectID,
        "expected NPC 11085 at distance %d to be picked", dist)
}
```

The `applyDump` / `dumpparse.State` helpers do not exist yet in the
codebase - they are the first thing to add when a dump-driven repro
lands. Keep them in `internal/swarm/webserver/dump_test.go` or a new
`internal/swarm/webserver/dump_repro_test.go` so the dump format and
the parser stay in sync.

## What the dump is NOT

- **Not a packet capture.** The dump is the world view, not the wire
  traffic. A protocol bug (a parse error, a wrong field) needs the
  raw packet log (`SWARM_TRACE_PACKETS=1`), not the dump.
- **Not a stack trace.** A crash needs the bot log with the panic;
  the dump is the state, not the runtime.
- **Not a fleet view.** The dump is one bot; a fleet-wide regression
  needs the `fleete2e` benchmark, not 100 dumps.

## The dump-state discipline

- A bug report that lacks a dump is half a bug report. Ask the user
  for the dump (or the build line + the steps to reach the web UI
  button) before reproducing.
- The dump pins the code state through the build identity. A fix is
  verified against the dump AND a fresh live session, never against
  the live session alone (the live state moves on; the dump does
  not).
- When the fix lands, the dump becomes the regression test fixture:
  drop it in `testdata/` and the repro test reads it.
