# The continuous archer retreat: never stand through the reload (status: in progress)

Started 2026-11-09, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue).

## Goal

Make the archer kite retreat continuously and shoot only from the
safe distance, per the owner instruction:

- The bot keeps running away even when the mobs reached point blank
  between the shots: the chaser must spend time closing the distance
  again after every strike.
- The bot must not fire two shots standing still: the retreat starts
  the moment the server accepts the movement after a shot, and the
  next shot waits until the retreat regained the safe distance
  instead of firing at the reload lapse into the mob.
- Question both the code and the tests: the tests may pin the wrong
  behavior, so every change is re-verified on the live server until
  the behavior is right.

Constraints:

- The retreat stays STRAIGHT (the owner rule of the previous round):
  no fan, chord, camp deflection, breakout or pursuit turns; a
  genuinely blocked lane holds the ground and keeps shooting.
- The anti-archer gate stays (no kite against a bow-wielding enemy).
- Only `temp*` accounts for live checks; the persistent test1..testN
  characters stay untouched; no `-abuse`; the Mobius server stays
  running; untracked `runs/*` evidence is never committed.
- Committer `melg8 <public.melg8@gmail.com>`.

## Acceptance criteria

- The live `archer-kite` scenario passes: the fight samples stay at
  the bow radii (the ranged majority beyond the melee radius), the
  character stays online the whole window, and the quiver never
  runs dry.
- The shot cadence shows one shot per retreat cycle, fired from the
  re-shot floor, never a second standing shot at the collapsed
  range.
- The retreater runs between the shots: no idle standing in the
  reload tail beyond the windup the server forces.

## Progress

### 2026-11-09 - the pursuit hold of the walk tail

- `kitePursuitHold` (kite.go) re-arms the straight retreat at every
  walk-window end and every leg arrival while the nearest hostile
  still holds inside the re-shot floor: the chaser spends the reload
  tail closing the distance again instead of the archer standing
  through it, and the re-shot fires only from the regained floor
  (the forced attack re-request owns the tick once the hold stands
  down).
- The hold carries its OWN per-shot ledger (`kitePursuitSteps`,
  `kitePursuitDist`, `kitePursuitStall`, `kitePursuitWalkOpen`) apart
  from the proximity streak: the reset seams are the fresh shot
  broadcast (`kitePursuitCycle`) and a walk issued outside the
  previous context (a new fight, the respawned same-id target).
- The stall exit (`kitePursuitStallLimit = 2`,
  `kitePursuitDeadlockSteps = 4`, `kitePursuitStallSlack = 20`) stands
  a spent budget down and lets the bow fire from the hold; the
  `kitePursuitStreakLimit = 12` crossing is the diagnostic of a chain
  that still opens distance; `kitePursuitContext = 12 s` expires the
  stale fight behind the reused Mobius object id.
- `kite_pursuit_test.go` pins the continuation, the straight ray, the
  won race re-shot, the stall exit, the progress reset, the context
  expiry and both reset seams (7 tests, no added real sleeps).
- Next: the live `archer-kite` acceptance run against the deployed
  stack.

### 2026-11-09 - the gauge timing and the retreat ownership

- New parser `setup_gauge.go` (0x85, the RED bow cycle) and the
  state/connection plumbing (`BowCycleAt`, `ApplyShotGauge`,
  `SelfBowCycle`).
- `kiteWalkWindow` prefers the live server gauge over the pAtkSpd
  formula; `kiteWindupWindow` is its first half; the deferred click
  fires at the windup end (the live run showed the formula fallback
  clicking inside the real windup, which the server saved and the
  re-shot canceled).
- The retreat anchors on the NEAREST threat, and `kiteOwnsRetreat`
  keeps the melee escape machinery out of a running kite.
- Live run 1: the deferred click always fires at the windup end and
  the server accepts it (no refused deferred clicks). The scenario
  still fails on the pre-existing two: the post-kill pile-up logout
  and the ranged share (the shots fire at the reload lapse and the
  windup collapses the band).

### 2026-11-09 - the continuous retreat, the flight and the ceiling

- The pursuit hold lost its stall exit: the owner rule removed the
  "stand the walk down and shoot" answer even for a parity race, so
  a slow chain keeps walking and the ONLY standing-shoot case is a
  genuinely walled lane. `kitePursuitStreakLimit` (12) is now a
  runaway diagnostic, not a stop.
- `kiteDeferShot` (the engage rung ahead of `AttackTarget`) holds
  the forced attack request while a retreat of this fight ran inside
  `kitePursuitContext` and the nearest hostile holds inside
  `kiteReshotFloor`: the reload is walked instead of spent standing.
  A fresh fight keeps its first shot, the cornered hold keeps its
  shot.
- The distance floors use the conservative closed distance
  (`closedDistance`, `Bot.ObjectTrackOf`, `chaseSpeed`): the raw
  `MoveToPawn` gap is aged by the movement segment and shortened by
  the fastest chaser speed (the live run measured 512 raw against
  256 actual).
- The click validation rejects a partial run shorter than
  `kiteMinStep` (the live 38 unit walk).
- The corner-only angular escape (`kiteLaneEscapeAngles`, +-22.5/45/
  67.5/90 degrees inside the away half-plane) fires only when every
  rung of the preferred away-ray is walled; the direct ray stays the
  primary retreat (the live spot 34792,49416 had the west ray walled
  at every length while south/east/north stood open).
- The defensive flight (`kiteFleeThreat`) hooks first in
  `fleeFromThreat`: a bow character with the kite enabled flies the
  straight away-ray at the full escape segment and bypasses the
  fan's closure watchdog/logout; the fan stays the fallback of a
  walled lane, a bow attacker, a melee weapon, another deck or a
  disabled profile, and the critical-HP logout stays armed.
- The kite engage ceiling (`kiteEngageLevelSlack` = 5): the run-4
  dump showed the general +2 ceiling refusing the level 10 Kaboo
  Orc Fighter of the level 7 cell - the pick and the loot answer
  fled it, the parity chaser dragged the flight and the window
  passed without a shot. The widened ceiling answers the Fighter
  with the bow; a bow attacker and a melee attacker six levels up
  still keep the flight.
- Tests: `kite_pursuit_test.go` (11: continuation, won race, slow
  race, context, both reset seams, closed distance, deferred shot x3,
  partial-run rejection, walled-ray escape), `kite_flee_test.go`
  (7: straight flight, live leg, watchdog bypass, fan fallback, bow
  attacker, disabled profile, stand-up guard),
  `TestKiteAnswersTheMeleeAttacker` plus the updated add-answer test.
- Verification: `task fmt`, `task fmt:check`, `task --force
  lint:new`, full `task lint` (0 issues), `go build ./...`, full
  `internal/swarm/hunt` suite (107 s) green.
- Live acceptance run 5 PASS: 51 fight samples, 78% ranged, max 656
  units, 6 kite steps, 0 holds, min HP 78%, min arrows 593 (log
  `logs/acceptance/archer-kite-20261109-220729-26760-1.log`). Run 6
  PASS reproduced the shot -> windup-end retreat -> pursuit chain ->
  re-shot cadence (22:12:36).
- Next: `task verify`, the atomic commits, and the fragment archive.
