# The fighter bow lure timing and range (status: in progress)

Started 2026-11-14, branch main, issue0 (owner report: three defects of
the melee fighter's bow lure - the standoff sits too close, the attack
starts seconds after the arrival, and the kite retreat continues after
the melee swap).

## Goal

The covered-pick lure (`hunt/lure.go`) behaves as one deliberate
ranged action:

- the standoff sits just inside the 500 unit bow range (480), and the
  forced attack fires the moment the live target is inside the weapon
  range (500), so the character stops at the bow range, never at the
  old 420 ring;
- the bow and the quiver arm during the approach: the first use item
  rides the pick tick, each next request waits only the paperdoll
  confirmation of the previous one (the UseItem flood protector is
  disabled), so nothing stands armed at the standoff waiting a 2 s
  window;
- the target is selected during the approach (the first Mobius
  `AttackRequest` only selects), so the forced request fires at the
  range entry instead of spending the one-second double-click window
  after the arrival;
- the kite layers never arm while the lure runs; the melee handoff
  stands the kite motion down (the re-click ladder, the walk window,
  the deferred click), and the first forced melee request waits only
  the melee paperdoll confirmation, not a leftover kite window;
- the spawn-settle first strike keeps its on-spot behavior (no
  standoff walk);
- every rule is pinned by a fake-clock test, and the live probe
  measures the standoff distance, arrival -> shot and swap -> attack.

## Context

- The three owner reports: (1) the fighter stops closer than the
  maximum bow distance; (2) it stands long without attacking while
  already at the target with the bow; (3) after deciding to switch
  back to the sword it keeps running the kite motion for seconds.
- Verified facts: every C1 bow carries `attack_range` 500
  (`dist/game/data/stats/items`), and the server range check is
  `getPhysicalAttackRange() + attacker collision + target collision`
  (`CreatureAI.maybeMoveToPawn`); `doAttack` stops a moving character
  before the shot.
- `lureStandoff` is 420: 80 units inside the range, so the owner sees
  the old ring; the approach hands over to the arm at 350 units from
  the ring (`patrolCenterMinDist/2`).
- `lureArmPeriod` 2 s is stale: `FloodProtectorUseItemInterval = 0`
  (the round of `35ad87f2`), the only real constraints are the
  per-item write set and the paperdoll confirmation.
- `DefaultKiteParams().Enabled` is true for every profile; during the
  lure the bow is in hand, so `kiteFromShot`/`kiteFromTarget` walk the
  character away while the pulled mob closes, and the melee swap
  inherits the kite walk window (`combatAvoidUntil`), which blocks the
  first melee request.
- The Mobius `AttackRequest` double-click semantics (verified in
  `AttackRequest.java`): a request for a different target only selects
  (`onAction`), a request for the already selected one forces the
  attack. `PlayerAI.thinkAttack` then walks to
  `getPhysicalAttackRange()` and shoots.

## Progress

### 2026-11-14 - the fragment

The task context recorded; no code changed yet.

### 2026-11-14 - the lure rework

`hunt/lure.go` rewritten around three changes:

- the standoff ring is 480 (`lureStandoff`, 20 units inside the range
  with the walk drift covered) and the forced attack gate is the live
  target distance against `lureShotRange` 500. The approach completes
  the moment the tool is confirmed and the target is inside the range,
  so the character never stops short of the bow range; a target that
  wandered past the range off the ring re-aims the standoff at its live
  position instead of standing out of it;
- `lureArm` and `lureArmPeriod` removed. The bow and the quiver arm
  inside the approach walk through `lureArmStep`: the bow first, the
  quiver once the paperdoll shows the bow (the interrupted-lure quiver
  in hand is detected and no toggle is sent), each request waiting only
  the paperdoll confirmation. `lureToolReady` is the completion gate;
- `lureSelect` sends the selection click on the first walk tick when
  no earlier action selected the target (Mobius `AttackRequest` selects
  on the new target and forces on the repeat), stamped with the shared
  `lastActionAt`, so the range entry only forces the attack.

The melee side gained the `lureMelee` phase: the arrival at 200 units
stands the kite motion down, sends the sword swap and holds the first
melee request until the paperdoll weapon family leaves `BOW` (the
server defers `useEquippableItem` to the bow attack end) or the 5 s
`lureSwapHold` passes, so the handoff fires the tick the sword is
confirmed.
`lureShootTick` and `lureMeleeTick` now consume the tick while they
hold it. `startLure` is the shared installer (kite stand-down, on-spot
flag) and the spawn-settle first strike calls it with `onSpot`.

`hunt/kite.go` gained the `lureArmed()` gate in `kiteLayerGates` and
`kiteStandDown()` (deferred click, re-click ladder, walk window,
cornered hold, one log line); `requestAttack` in `loop_engage.go` ticks
the lure ahead of the `combatAvoidUntil`/retry gates so a leftover kite
hold cannot starve the lure, while the approach walk keeps waiting the
hold out.

### 2026-11-14 - the tests and the docs

`hunt/lure_test.go` rewritten around the new machine:

- `TestLureAnswersTheCoveredPick` (standoff ring geometry),
- `TestLureFiresFromTheBowRange` (range entry fires without a second
  approach, a target outside the range keeps the walk going),
- `TestLureArmsTheToolOnTheApproach` (bow and quiver on the walk, no
  standoff wait, interrupted-lure quiver not toggled off),
- `TestLureStandsTheKiteDown` (no kite layer arms under the lure, the
  armed motion is dropped),
- `TestLureMeleeHandoffFiresAtOnce` (swap on arrival, first melee
  request on the paperdoll confirmation),
- `TestLureHandoffIgnoresTheAddHold` (a leftover `combatAvoidUntil`
  does not delay the handoff).

The existing hold/timeout/gate tests were updated to the new phases.
`hunt/settle_test.go` expects `lureApproach` for the on-spot strike.
`docs/hunting.md` carries the 480 ring, the arm-on-walk and
pre-select facts, the melee handoff and the on-spot note in the lure
section, and the new "lure gate" paragraph in the archer-kite section.

`go build ./...`, `go test ./internal/swarm/hunt/`, `task fmt` and
`task lint:new` pass.

## Status

Next: `task prepush`, the commit, then the live probe on the running
stack (standoff distance, arrival -> shot, swap -> attack) and the
devlog record of the round.
