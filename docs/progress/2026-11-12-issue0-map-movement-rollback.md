# The web map marker jumping back (status: in progress)

Started 2026-11-12, branch main, issue0 (owner report: the character
marker on the web map periodically jumps back during a hunt).

## Goal

The marker of the played character follows the server movement without
a visible backwards step. Done means:

- the tracker never leaves the projection anchored at a stale movement
  stamp when a position sample of the character arrives;
- the drawn marker never travels away from its current destination;
- the movement harness measures the backwards step and fails on it;
- a live hunt recording replays with zero drawn-back steps.

## Context

- The render path is `state` snapshot -> SSE -> `web/map.js`
  `projectTickwise` (the exact Mobius tick recurrence) -> the
  `advanceRuntime` chase (1.35x speed cap).
- The root cause trace (`SWARM_DEBUG_POS=1`, `runs/posdebug-bot.log`):
  the position of the played character also arrives in packets that
  carry no movement state of their own - a UserInfo refresh, the target
  fields of a mob `MoveToPawn` chase (`TargetID == selfID`), an Attack
  hit. The tracker adopted the new x/y but kept the old `MoveAt`, so
  `projectTickwise` replayed from the new anchor with the old stamp and
  put the target ahead of the server by the already covered distance;
  the next `MoveToLocation` snapped it back.
- Verified in the Mobius C1 sources: `Player.broadcastPacket` sends every
  broadcast except CharInfo to the acting player itself; `MoveToPawn`
  reads the target coordinates at packet build time;
  `Creature.updatePosition` steps once per 100 ms tick and a non forced
  `MoveToLocation` broadcast is throttled to one per second.
- The display side adds a second source: the chase may glide the marker
  back when a new packet lands behind the projection (packet phase, a
  refused move, the server holding its position for two broadcasts).
- Recordings before the fix: `runs/rec2.json` (combat, target retreat
  93 u and 166 u), `runs/rec3.json` (walk, 28 u). The old renderer
  replayed rec2 with 27 frames of drawn-back motion, up to 3.7 u per
  frame.

## Progress

### 2026-11-12 - the position writers and the rollback trace

Read the whole path (`state.Apply*`, `snapshot_live.go`, `map.js`),
checked the server sources above and traced the live bot. The trace
confirmed the stale stamp shape: `MoveToPawn` target samples advanced
the character x from 18232 to 18144 while `moveAt` stayed 2 s old, and
the following `MoveToLocation` produced the rollback. Fixed the tracker
with `applySelfPositionSampleLocked`: the sample lands, and a moving
character restarts `MoveAt` at the sample arrival. Applied to
`ApplyUserInfo`, the `TargetID == selfID` branch of
`ApplyPawnMovement`, and both self blocks of `ApplyAttackAt` (the
attacker block previously cleared the movement). Tests:
`self_move_anchor_test.go` (UserInfo, pawn target, attack attacker and
victim, the standing sample that must not gain a stamp).

## Status

In progress: the state re-anchor is in place and tested. Next: the
`map.js` monotone guard, the harness back-step gate, and the live
recording after both fixes.
