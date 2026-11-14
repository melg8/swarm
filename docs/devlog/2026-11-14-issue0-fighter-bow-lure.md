# The fighter bow lure round (issue0)

Date: 2026-11-14. Scope: the covered-pick lure of the melee hunter
(`hunt/lure.go`, the kite gate in `hunt/kite.go`, the engage tick
order in `hunt/loop_engage.go`). Issue0 (owner report: the fighter
stops closer than the bow range, stands long before the first shot
and keeps running the kite motion after the melee swap). Landed on
`main`: the fix in `acef3042` and `f45f8433`, the round fragment in
the commit carrying it.

## Problem statement

The owner watched the level 18 fighter lure a covered mob with the
bow and reported three defects:

- the character stops short of the maximum bow distance;
- it stands at the target with the bow for seconds before the first
  shot;
- after the decision to swap back to the sword it keeps retreating,
  so the melee weapon spends its first seconds out of range.

## Root cause analysis

- `lureStandoff` was 420: the approach stopped 80 units inside the
  500 unit bow range, which is what the owner called "closer than the
  maximum distance".
- The arm phase was a separate state after the arrival: the bow and
  the quiver equipped there, paced by `lureArmPeriod` 2 s, and the
  shoot phase started only after both paperdoll confirmations. The
  delay was a leftover of the old use item flood protector window;
  `FloodProtectorUseItemInterval` is 0 in the C1 config, so only the
  write set and the paperdoll confirmation pace the equips.
- The pull was a single forced attack sent after the arrival, but the
  Mobius `AttackRequest` selects on the first request for a new
  target and forces on the repeat (`AttackRequest.java`): after a
  fresh pick the arrival request only selected, and the forced one
  waited `engageRetryPeriod`.
- The kite layers are enabled for every profile and gate only on
  "bow in hand": during the lure the bow is out, so `kiteFromShot`
  started a retreat while the pulled mob was closing. After the
  melee swap the kite walk window (`combatAvoidUntil`) and the
  re-click ladder kept the character moving away, and the engage
  retry gate dropped the first melee request.

## Reproduction

- Offline: `hunt/lure_test.go` pins the whole machine with a fake
  clock - the standoff ring, the range entry, the arming on the walk,
  the kite gate and stand-down, and the melee handoff on the
  paperdoll confirmation. `hunt/settle_test.go` pins the on-spot
  first strike. The tests fail on the pre-fix code (the old arm phase
  and the absent gate).
- Live: a fresh bot on the deployed C1 stack sampled at 5 Hz through
  `GET /api/bots/<bot>/state`, with the lure log events read from the
  same snapshot. Covered picks occur several times an hour in the
  elven forest; each episode reports the standoff distance, the
  arrival -> shot latency and the swap -> first melee request
  latency.

## Fix

- The standoff ring moved to 480 and the shot gate is the live target
  distance against `lureShotRange` 500, checked at every 250 ms tick,
  so the character stops at the range instead of inside it. A target
  that wandered out of the range re-aims the standoff at its live
  position.
- The arm phase folded into the approach: the bow equips on the first
  walk tick, the quiver once the paperdoll shows the bow, and the
  arrival only needs `lureToolReady` (the paperdoll family and the
  left hand slot).
- The pull pre-selects the target on the first walk tick (`lureSelect`
  through the shared action stamp), so the range entry sends the
  forced request at once (`AttackRequest` select vs forced semantics).
- `kiteLayerGates` answers false while `lureArmed()`; `kiteStandDown`
  drops the deferred click, the re-click ladder, the walk window and
  the cornered hold at the lure start and at the melee arrival.
- The melee handoff is its own `lureMelee` phase: the swap goes out on
  the 200 unit arrival, and the first melee request fires the tick the
  paperdoll confirms the sword (the server defers `UseItem` equips to
  the bow attack end), bounded by `lureSwapHold` 5 s.
- `requestAttack` ticks the lure ahead of the movement and retry
  gates, so a leftover kite or flood stamp cannot starve it.

## Verification

- `go build ./...`, `go vet`, `go test ./internal/swarm/hunt/`,
  `task lint:new`, `task fmt` and `task prepush` are green; the
  rewritten lure tests pin every rule above.
- Live, first measurement (level 18 fighter, 5 minutes): one covered
  pick at 03:10:04 - the range entry at 483 units, the character
  stopped 455 units from the target on the attack commit, the first
  damage 1.47 s after the entry (the bow windup) and the sword
  confirmation 0.25 s after the swap request.
- Live, second measurement (the same character back from the town
  trip, 10 minutes, 2997 state samples): nine covered picks, eight of
  them reached the range entry at 479-480 units; the last moving
  sample preceded the entry by 0.08-0.36 s, so the character never
  stood at the target waiting the shot; the first damage landed
  1.31-1.97 s after the entry (the windup of the shot fired on the
  entry tick); every measured handoff confirmed the sword 0.25 s
  after the swap request, one loop tick.
- The one episode without a range entry was interrupted on the walk by
  an add that aggroed on the approach: the engage answered it and the
  lure ended before the standoff (the ordinary flow owned it).

## Follow ups

- The standoff is 20 units inside the range; the walk overshoot
  between ticks is bounded by the 250 ms loop period (the server
  stops the character on the attack commit), so the live stop sits at
  455-483 units.
