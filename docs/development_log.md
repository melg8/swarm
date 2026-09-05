# Development log

Running log of the work on the `mobius-c1-client-1` branch. Newest entries at
the bottom. This file exists so the context does not have to be repeated in
agent prompts: read this file first.

Entry format:

- date, scope
- problem statement
- root cause analysis (with references into the Mobius server sources)
- reproduction: how to trigger, how to detect, expected output
- fix, verification, follow ups

## 2026-09-05: hunt MVP web interface follow up

Scope: two problems reported against the MVP bot web interface on the
`mobius-c1-client` branch.

### Problem 1: movement display ends with a fast teleport

Statement: characters move along the path on the map, but after roughly 7/8
of the way they are dragged to the destination almost instantly. The first
7/8 looks slightly slowed down; the error is around 10-15 percent of the
real speed.

Root cause analysis (verified against the Mobius C1 java sources,
`L2J_Mobius_C1_HarbingersOfWar/java`):

1. The web map interpolated the full packet distance `D` at the transmitted
   speed `v`, while the Mobius server stops a moving creature early:
   `Creature.updatePosition` treats the creature as arrived when
   `speed * elapsedTicks` covers `distance - collisionRadius` (the collision
   radius of the mover, or of the chase target while attacking) and then
   snaps the server position to the exact destination and broadcasts the
   zero distance `MoveToLocation`. A client that animates the whole `D`
   therefore still has `collisionRadius` units left when the arrival packet
   lands; with the typical C1 monster radii (5-15) and random walk hops
   (50-300 units inside `MaxDriftRange = 300`) that alone is 5-15 percent
   of the path.
2. The render loop smoothed the drawn position toward the projected
   position with a first order filter (`1 - exp(-dt*10)`, time constant
   100 ms). A first order tracker of a ramp input lags by `v * tau`
   permanently: at `v = 120` that is 12 units of permanent visual lag, on
   top of the collision gap.
3. The movement ticks of the server are quantized (100 ms for npcs,
   50 ms for playables, `MovementTaskManager`), the snapshot poll adds up
   to 300 ms and the SSE clock offset estimate has a small negative bias,
   which adds a few more units of trailing error.

Combined, the drawn character is 10-30 units short when the arrival packet
arrives (10-20 percent of a typical hop), and the arrival then snaps the
position: the reported "slightly slowed 7/8 then quick drag".

### Problem 2: `-hunt` selects a target but never attacks

Statement: with `-hunt` the character picks a target (MyTargetSelected
arrives) and then stands still forever.

Root cause analysis (Mobius C1 java sources):

- `AttackRequest.runImpl` (client packet 0x0A) implements the classic
  double click semantics:
  - when the requested target is **not** the current target it calls
    `target.onAction(player)`, which resolves to the `NpcClick` script
    handler: `player.setTarget(target)` plus `MyTargetSelected` — a plain
    selection, no attack;
  - when the requested target **is already** the current target it calls
    `target.onForcedAttack(player)` → `player.getAI().setIntentionAttack`,
    which starts the chase and the auto attack.
- The hunt loop sent exactly one `AttackRequest` per target and then waited
  for the kill: `engage()` copies the confirmed server target into
  `l.target` and returns early while the target lives, so the second
  request that would trigger `onForcedAttack` is never sent.
- The server flood protector (`PlayerActionFloodProtector`,
  `FloodProtectorPlayerActionInterval = 1`) allows one player action per
  second, so a bot that re-requests once per tick is fine.

### Reproduction and verification plan

- `tools/repro_movement.js` — a Node script that loads the real
  `web/map.js` interpolation, drives it with a faithful simulation of the
  Mobius movement/broadcast semantics and reports the arrival position
  error and the end of path jump. Exit code 1 while the bug is present.
- `internal/swarm/hunt/loop_test.go` and
  `internal/swarm/connection/hunt_flow_test.go` — Go tests that implement
  the same server semantics in-process (fake game server with the
  double click behavior) and assert the bot actually starts attacking.
  They fail while the bug is present.

The entries below describe the reproduction tooling, the fixes and how
to verify everything on any checkout.

### Fix 1: movement rendering (commit "fix movement interpolation end of
path teleport")

The web map now models the server arrival schedule instead of the naive
full distance animation:

- `projectObject` scales the interpolation speed by
  `distance / (distance - gap)` where `gap` estimates how early the
  server stops the creature (collision radius plus about one 100 ms
  tick step). The estimate is learned per npc template id (and for
  players/self) with an exponential moving average fed by every
  observed arrival: when the arrival packet lands, the tracker compares
  the traveled projection distance with the segment distance. The gap
  is bounded (1..60 units) and the speed scale is capped at 1.6 so a
  bad estimate can never dash units.
- The drawn position follows the projection plus a decaying offset that
  is only set when the projection jumps discontinuously (a new segment,
  an arrival, a teleport above 400 units snaps instantly). Continuous
  movement has no permanent lag anymore; residual calibration errors
  glide closed within ~100 ms.
- The server clock offset estimate takes the maximum of the last 20
  snapshot samples instead of the latest one, removing the transport
  delay bias.

Verification: `node tools/repro_movement.js` (or `task repro:movement`)
drives the real `web/map.js` inside a Node vm sandbox against the
simulated Mobius movement (see the harness header for the reproduced
server semantics). Before the fix it reported `FAIL` with progress at
arrival 0.33-0.96 and end of path jumps up to 4.9x the normal frame
speed; after the fix every scenario reports `PASS` with progress
0.94-1.00 and jumps below 1.25x. The harness exit code is 1 while the
bug is present, so it is usable as a regression check.

### Fix 2: hunt attack start (commit "fix hunt loop never starting the
attack")

The hunt loop implements the double click flow now:

1. `AttackNearest` sends the first `AttackRequest` for the nearest
   attackable npc and latches its id (the server answers
   `MyTargetSelected`, which the tracker records as the target).
2. While the target lives and the tracker does not report the character
   engaged with it (`state.Bot.SelfEngaged`: the chase MoveToPawn, the
   Attack or the AutoAttackStart broadcasts set the fighting target),
   the loop repeats `AttackTarget` for the same id at most once per
   second (the `PlayerActionFloodProtector` interval).
3. As soon as the fight packets arrive, the requests stop.

Verification: `internal/swarm/connection/hunt_flow_test.go`
(`go test ./internal/swarm/connection -run TestGameClientHuntFlowStartsAttack`)
spins up a fake game server that implements the extracted Mobius
semantics (first request -> MyTargetSelected only, second request for
the already selected target -> AutoAttackStart + Attack). Before the
fix the test fails after 10 seconds with exactly one received request
`[1]` and no combat; after the fix it passes in about two seconds with
requests `[1 1]` and the fight running. Unit level coverage of the
retry and stop conditions lives in `internal/swarm/hunt/loop_test.go`.

### How to reproduce and detect both problems on any checkout

- Movement: `node tools/repro_movement.js [--verbose]` (needs Node 18+;
  no Go or server required). Exit code 1 and `FAIL` lines mean the end
  of path teleport is present.
- Hunt: `go test ./internal/swarm/connection -count=1
  -run TestGameClientHuntFlowStartsAttack`. A failing test with a
  single recorded attack request means the bot never starts attacking.
- Full suite: `task check:all` (lint + tests) plus
  `task repro:movement`.

Status: both problems fixed and verified; the branch
`mobius-c1-client-1` carries the analysis, the reproduction tooling and
the fixes.

### Regression detector validation (2026-09-05)

Re-checked that the harness really detects the bug: running
`node tools/repro_movement.js` against the pre fix `web/map.js`
(checkout of the branch point `34f6884`) reports FAIL with progress at
arrival 0.897-0.940 and jumps up to 2.23x; running it against the fixed
file reports PASS on all scenarios. Restoring the fixed file afterwards
keeps the branch clean, and the full suite (`go test ./...`, golangci
lint on new code, the movement harness) is green.
