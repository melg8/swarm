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
