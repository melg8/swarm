# The delayed approach walk of a fresh pick (status: done)

Started 2026-11-14, branch main, issue0 (owner report: the target
switching is fast now, but the movement toward the new target starts
fractions of a second late - it must start at once).

## Goal

The hunt loop starts the walk toward a freshly picked target on the
pick tick:

- the target selection and the approach `MoveToLocation` leave the
  client in the same tick, so the character is moving during the
  second the server spends before the forced attack is legal;
- the click stops at the engage radius (150 melee, 450 bow), so a
  ranged profile keeps its shooting distance and the melee profile
  stops on reach;
- the approach stays out of the bow lure (a deliberate standoff) and
  out of the combat-avoid window;
- the behavior is pinned by a fake-clock regression test that fails on
  the old code;
- the live probe measures selection -> movement latency below the
  1.2-1.3 s of the pre-fix trace.

## Context

- The pre-fix live trace (2026-11-14 00:22, bot `test1`) measured a
  selection at 00:22:07.309 and the first movement at 00:22:08.6:
  ~1.3 s of standing on the spot after the target was chosen.
- The root cause: the Mobius `AttackRequest` has double click
  semantics and sits under `FloodProtectorPlayerActionInterval`
  (1 s). The first request only selects the target; the server starts
  the chase on the repeated (forced) request, which the loop sends
  after `engageRetryPeriod` (1 s). Nothing moved the character in that
  window: `MoveToLocation` is outside the player action flood window.
- The pick tick already knows the target position (`AttackTarget`
  carries X/Y), so the approach click needs no extra scan.
- The bow lure (`maybeBeginLure`) owns the covered pick and stays
  untouched: its standoff is the deliberate ranged approach measured
  at 4.6-4.8 s in the previous round.

## Progress

### 2026-11-14 - the regression test

`TestLoopWalksToThePickedTargetOnThePickTick` rebuilds the handover
with a fake clock: kill, loot, the last drop disappears, the engage
tick picks a mob 1000 units east and must send the walk click in the
same tick, stopped 150 units short of the mob. It failed on the old
code (`game.walks` empty) and passes after the fix.

`TestLoopKeepsStandingWhenTheFarMobEntersTheEngageRadius` now pins the
approach click of the direct pick (45850, the melee reach) instead of
an empty walk list.

### 2026-11-14 - the implementation

`approachPick` (`loop_movement.go`) runs after `maybeBeginLure` on the
pick path: it skips an armed lure and the combat-avoid window, clicks
one segment capped at
`min(returnWalkSegment, dist - pickApproachStopFor())` toward the mob
and does not touch `lastActionAt`, so the attack pacing is unchanged.
`pickApproachStopFor` answers the bow weapon range (450) and the melee
swing contact (40): the first probe showed that a pick inside the old
150 unit stop (102 and 120 units away) still stood for 1.2-1.4 s,
because no click was sent and the server chase only starts at the
forced attack. `docs/hunting.md` and `docs/protocol_description.md`
describe the approach start.

### 2026-11-14 - the live measurement

The fixed binary (`swarm.exe`, built 2026-11-14 02:09) ran as `probe1`
(level 1, a fresh account: the acceptance manager already owns every
`temp*` registry entry, so its record shadowed the `temp2` probe) on
the deployed C1 stack while a 5 Hz web-state probe sampled it for
200 s (`runs/live-pick-walk3.log`, the trace at
`%TEMP%/opencode/pick_walk_b.jsonl`).

Fifteen in-window selections: thirteen picks beyond the 150 unit stop
walked 0.00-0.29 s after the selection (the pre-fix trace stood
~1.3 s), one at 156 units clicked the 6 unit remainder and moved at
0.00 s. The two lagging entries (1.36 and 1.17 s) are the picks at 102
and 120 units - inside the old stop, where no click went out; that
finding produced the `pickApproachStopRadius` (40) refinement.

### 2026-11-14 - the second live measurement

The refined binary (`swarm.exe`, built 2026-11-14 02:19) ran as
`probe2` (level 1, a fresh account) for 200 s
(`runs/live-pick-walk4.log`, the trace at
`%TEMP%/opencode/pick_walk_c.jsonl`). Eighteen in-window selections:
seventeen picks moved 0.01-0.45 s after the selection, including the
near picks at 70, 89, 96 and 109 units (0.02-0.21 s) that the old stop
left standing for 1.2-1.4 s. The eighteenth pick sits at exactly 40
units: no click is needed there, the character stands and swings (the
mob died 9 s later) - the movement that follows is the loot walk, not
the approach.

## Status

The delayed approach is gone. Offline: `go test ./internal/swarm/hunt`
(the regression tests fail on the old code), `task lint:new`,
`go build ./...` and `task prepush` are green. Live: far and near
picks start moving within 0.45 s of the selection against ~1.3 s
before; only a pick already at the 40 unit swing contact stands
still, because it fights in place.
