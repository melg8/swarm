# Shot-paced kite retreat: the bow cooldown spent walking (status: in review)

Started 2026-09-22, branch feature/kite-shot-paced-retreat, issue #60.

## Goal

The issue #60 report: the archer bot stands through the bow reload
while the mob closes, then kites only after the mob crossed the
retreat radius - the acceptance dump shows the mob glued at 12-57
units dealing one 10 damage blow per second through every kite step.
Done means: the retreat arms the moment the character's own shot
released (the Attack broadcast), the bow cooldown is spent walking,
and the cycle becomes shoot, run the reload, gain distance, shoot
again - the melee uptime drops to the stand moments. The acceptance
criteria are the unit suite of the kite layer (the rhythm trigger,
the pursue band, the streak exemption, the shared holds) plus the
lint/format gates; the live verdict lands through the archer
acceptance scenario (#28) on the deployment stack.

## Context

- Issue: https://github.com/melg8/swarm/issues/60 (the state dump,
  the kiting ask). Project board claim: Agent Tz9kV4mB.
- The kite layer pre-change: the proximity-triggered step of
  kite.go (issue #13/#19, the tuning knobs of #29) - the mob had to
  close inside 250 units before the archer moved.
- The verified server facts (the Mobius C1 source read of
  Creature.doAttack / doAttackHitByBow / onHitTimer, registered as
  H-007 in AGENTS.md, documented in docs/hunting.md):
  - The Attack packet is the shot commit: the hit roll, the arrow
    consumption and the HitTask schedule happen before the
    broadcast; the damage task carries no attacker movement check.
  - The bow disable window is timeAtk + reuse (500000/pAtkSpd +
    reuseDelay*333/pAtkSpd, ~3s for the Short Bow), mirrored by
    SetupGauge(RED).
  - A forced attack while moving stops the walk and attacks
    (stopMove before the launch).
- Dead ends ruled out:
  - Adapting the walk window to the measured cooldown: the
    SetupGauge packet is not parsed today; the 2s window fits the
    ~3s disable window and the staleness gate paces the cycle
    anyway - noted as the tuning follow-up, not this round.
  - Re-requesting the attack while the fight view is fresh (to
    tighten the cycle past the 3s staleness gate): touches the
    blind-recovery ordering rules, left out of this round.
- The test seam: the fight freshness can be stamped without a shot
  via the character's own chase step (ApplyPawnMovement for self -
  the Mobius MoveToPawn broadcast), which is what keeps the
  proximity-path tests (the streak bound, the radius knob, the deck
  gap, the archetype bookkeeping) pinning their original semantics.

## Progress

### 2026-09-22 18:40 UTC - the shot commit field and the rhythm step

- state: CharacterState.LastSelfShotAt stamped on every self Attack
  broadcast (hit or miss), the SelfLastShotAt accessor added.
- hunt: kiteShotWindow (700ms) and kiteFromShot added to kite.go -
  the shot-paced retreat sharing the lane machinery and the holds
  with the proximity path, pacing itself on the shot cycle and
  skipping the streak count; wired ahead of kiteFromTarget in the
  fighting branch of loop.go.
- The kite.go header comment documents the new layer.

### 2026-09-22 18:55 UTC - the tests and the doc round

- hunt: kite_shot_test.go pins the rhythm (the trigger, the band,
  the streak exemption and bookkeeping, the double-trigger guard,
  the encircled and cornered holds, the profile gate); the chase
  step scene (kiteBowBotChaseFresh) keeps the proximity semantics
  tests (streak limit, streak reset, widened radius, deck gap,
  archetype bookkeeping) on their original paths.
- docs: the shot-paced retreat section in docs/hunting.md, the
  H-007 registry entry in AGENTS.md.
- Verification: go build ./... clean, go vet clean, the full hunt
  package suite ok (81s), the state package suite ok.

## Status

The branch carries the complete round: the state field, the rhythm
step, the loop wiring, the tests, the docs. Next: the lint gate,
the atomic commits, the PR, the rebase to main, the issue update
and the board move to Ready for review.
