# The delayed approach walk round (issue0)

Date: 2026-11-14. Scope: the fresh-pick movement of the hunt loop
(`pickTarget`, `approachPick`, the melee approach stop). Issue0 (owner
report: the target switching is fast, but the movement toward the new
target starts fractions of a second late), landed on `main`: the fix
in `f2ed90d9`, this fragment and the archived progress fragment in the
commit carrying it.

## Problem statement

After the loop picks a target the character keeps standing for about
1.3 s before it moves. The pre-fix live trace (`test1`, level 18,
00:22): selection at 00:22:07.309, first movement at 00:22:08.6. The
previous round removed the post-loot standstill, so the remaining gap
is between the pick and the walk.

## Root cause analysis

- The Mobius `AttackRequest` has double click semantics and sits under
  `FloodProtectorPlayerActionInterval` (1 s): the first request only
  selects the target, the repeated one starts the forced attack. The
  loop waits `engageRetryPeriod` (1 s) before the repeated request, so
  the server chase (`MoveToPawn`) starts a whole flood window after
  the selection.
- Nothing walked the character in that window. `MoveToLocation` is
  outside the player action flood window, and the pick tick already
  knows the target position (`state.AttackTarget` carries X/Y).
- `walkToFarTarget` covers only the targetless state: it never runs on
  the pick tick, so a pick inside `attackNearestRange` (1500) had no
  movement of its own.

## Reproduction

- Offline: `TestLoopWalksToThePickedTargetOnThePickTick`
  (`internal/swarm/hunt/loop_test.go`) rebuilds the handover with a
  fake clock - kill, loot, the last drop disappears, the engage tick
  picks a mob 1000 units east and must send the walk click in the same
  tick. It failed on the old code (`game.walks` empty) and passes
  after the fix. `TestLoopWalksToANearPickedTarget` covers the pick
  inside the old 150 unit stop.
- Live: the bot state at 5 Hz through `GET /api/bots/<bot>/state`
  (a fresh account: the acceptance manager registers every `temp*`
  account in the same bot registry, so a hunt bot on `temp2` was
  shadowed by the offline acceptance record and the probe read zeros).

## Fix

- `approachPick` (`loop_movement.go`) runs after `maybeBeginLure` on
  the pick path: it skips an armed lure and the combat-avoid window,
  walks one click segment toward the mob capped at
  `min(returnWalkSegment, dist - pickApproachStopFor())` and leaves
  `lastActionAt` alone, so the attack pacing is untouched.
- `pickApproachStopFor` (`user.go`) answers the bow weapon range (450)
  and the melee swing contact (40): the first measurement showed picks
  at 102 and 120 units standing 1.2-1.4 s because the 150 unit stop
  suppressed the click, while the server chase settled near 15. A pick
  at 40 stands and fights in place, which the second measurement
  confirmed.
- `docs/hunting.md` and `docs/protocol_description.md` describe the
  approach start.

## Verification

- Full `go test ./internal/swarm/hunt/` suite green (the new tests
  fail on the old code), `go build ./...`, `go vet`, `task lint:new`,
  `task prepush`, `task fmt` clean.
- Live, first binary (02:09, the 150 unit stop) as `probe1` (level 1,
  200 s, `runs/live-pick-walk3.log`): thirteen picks beyond the stop
  moved 0.00-0.29 s after the selection; the two picks at 102 and 120
  units stood 1.36 and 1.17 s (no click, the server chase waiting for
  the forced attack).
- Live, refined binary (02:19, the 40 unit stop) as `probe2` (level 1,
  200 s, `runs/live-pick-walk4.log`): seventeen of eighteen picks
  moved 0.01-0.45 s after the selection, including the near picks at
  70, 89, 96 and 109 units (0.02-0.21 s); the eighteenth sat at
  exactly 40 units, swung in place and killed the mob 9 s later.
  Numbers in
  `docs/progress/archive/2026-11-14-issue0-pick-approach-walk.md`.

## Follow ups

- The bow lure standoff (4.6-4.8 s) stays by design (`lure.go`), out
  of this round.
- A pick already at the 40 unit swing contact still waits the flood
  window before the first swing; that is the server action pacing, not
  movement.
