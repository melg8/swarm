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

The dump is a compact JSON of the live `state.Bot` plus the bot
identity (branch, commit, dirty flag, build time). The fields an agent
needs to reproduce the behavior:

- **identity**: branch, commit, dirty, build time (so the dump pins the
  exact code that produced it - matches the `Build:` log line).
- **bot**: id, status, phase, server time, version, started, updated.
- **character**: name, level, race, class, position (X/Y/Z/heading),
  HP/MP, target id, moving, sitting, in-combat, stats (STR/DEX/CON/INT
  /WIT/MEN), exp/sp, adena, inventory slot count.
- **inventory**: object id, item id, count, equipped, bodypart,
  enchant, name (no icons, no weights - they are NPC data lookups).
- **objects**: the world object store - object id, kind, name, level,
  attackable, aggressive, dead, sitting, moving, running, position,
  target id, hp/mp, in-combat. This is the scan input the hunt tick
  sees.
- **events**: the rolling event log (last `snapshotEvents` entries).
- **chat**: the rolling chat window.
- **combatEvents**: the combat animation beats still in the TTL
  window.
- **walkPath**: the published walk plan + the current target index.
- **huntingZone** + **huntingZones**: the current zone and the
  registry.
- **skills**, **skillPlan**, **buffs**: the learned skills, the
  learning queue, the active effects.
- **shopping**: the published shopping plan (when fresh).
- **packets**: the received packet counter (sanity: a frozen bot has
  a stopped counter).

The dump is the full `AppendSnapshotJSON` output - the same encoder
the web UI uses. The web UI adds presentation fields (icons, colors,
weights) on top of it; the dump omits those because they are pure
lookups against `npcdata` and reproduce identically from the item id.

## Capturing the dump

Three ways, in order of preference:

1. **Live HTTP endpoint** (the bot is already running with `-web`):
   ```bash
   curl -s http://127.0.0.1:8080/api/snapshot > stuck_bot.json
   ```
   The endpoint returns the same JSON the SSE stream pushes. The bot
   keeps running, so the dump is a point-in-time snapshot.

2. **Build identity line** (the bot log): the line right after
   `Starting swarm bot` carries `Build: <branch>:<commit> dirty=<bool>
   time=<iso>`. Pair every dump with the build line so the code state
   is pinned.

3. **The user pastes the dump** from the web UI "state dump" button
   (the web UI exposes the same JSON for download). The button is the
   user-facing escape hatch when the agent is not connected to the
   live server.

## Reproducing the dump in a test

The dump is JSON of the `state.Bot` view structs, so a unit test
loads it directly:

```go
func TestStuckBotRepro(t *testing.T) {
    data, err := os.ReadFile("testdata/stuck_bot.json")
    require.NoError(t, err)

    bot := state.NewBot("stuck")
    require.NoError(t, json.Unmarshal(data, &bot)) // or use the
    // public Apply API to rebuild the live state field by field

    // The behavior under test: the hunt tick should pick a target,
    // not return "no attackable in range" while NPC 11085 is alive
    // at distance 73.
    target, dist := bot.NearestAttackableConstrained(1500, nil, nil, 0, true)
    require.NotEqual(t, int32(0), target.ObjectID,
        "expected NPC 11085 at distance %d to be picked", dist)
}
```

When the JSON does not map cleanly to the internal types (the snapshot
view is a projection, not the storage layout), build the live state
through the public `Apply` API: replay the parsed packet fields into a
fresh `state.Bot` so the storage invariants (the SoA split, the index
map) hold. The `state/*_test.go` helpers already pattern this.

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
