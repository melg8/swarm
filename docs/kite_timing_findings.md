# The kite timing findings: the retreat window of the Mobius C1 bow cycle

The applied experiment of issue #70 (the `kite-timing-probe`
acceptance scenario, account temp19) measured how the server answers a
retreat click issued at a controlled delay after the own bow shot.
The live stack: the Short Bow kit at level 7 (pAtkSpd 337), the Kaboo
Orc Grunt SW-5 cell, one raw wire session (no hunt loop), ten ladder
rounds plus the cursor key round. The run of 2026-09-23 14:06-14:08
(all ten rounds landed, the scenario PASSED).

## The server bow cycle (live numbers, pAtkSpd 337)

```
t=0.00s  the own Attack broadcast (the shot commit: the hit roll, the
         arrow consumption and the HitTask schedule already happened)
t=1.48s  the windup ends (isAttackingNow lapses at (timeAtk+reuse)/2;
         the HitTask lands the damage about here)
t=2.97s  the disable window ends (timeAtk+reuse; the READY_TO_ACT
         replay point; the next shot can start)
```

## The measured retreat ladder (mouse-mode MoveToLocation)

| Click delay after the shot | Verdict | Movement started |
| --- | --- | --- |
| 0 ms | deferred | 2.97 s after the shot |
| 300 ms | deferred | 2.97 s |
| 600 ms | deferred | 2.97 s |
| 900 ms | deferred | 2.97 s |
| 1200 ms | deferred | 2.96 s |
| **1500 ms** | **immediate** | **1.52 s** |
| 1800 ms | immediate | 1.82 s |
| 2200 ms | immediate | 2.22 s |
| 2600 ms | immediate | 2.62 s |
| 3000 ms | deferred | 5.93 s (the NEXT cycle end) |

Readings:

- A click inside the windup (the first ~1.5 s) is DEFERRED, not
  dropped: the movement starts at the full cycle end (2.97 s). The
  deferred click costs the whole reload window of standing time.
- The earliest immediate retreat sits at **1500 ms** after the shot -
  the windup end, exactly the `(timeAtk+reuse)/2` boundary the Mobius
  `PlayerAI.setIntentionMoveTo` check draws.
- The 3000 ms round crossed into the NEXT auto-attack cycle (the
  attack stance re-shot at ~2.97 s) and deferred to 5.93 s - two full
  cycles. A retreat click that races the re-shot loses to it.
- Zero ActionFailed answers were attributed before the movement (the
  deferral answer either rides the same broadcast tick or the refusal
  attribution window of the recorder missed it; the movement timing is
  the reliable signal).

## The cursor key (WASD) round

- The mode-0 MoveToLocation right after the shot plus the 200 ms
  ValidatePosition claim stream: the server processed the claims (7
  ValidateLocation broadcasts) but the net movement was 0 units - the
  cursor key latch did not carry the character through the windup
  under these conditions (the deferred AI intention may have snapped
  the adopted positions back; the follow-up round belongs to the next
  session).
- The damage landed regardless: the target HP fell 89% -> 76% while
  the claim stream ran. The WASD packets never aborted or spoiled the
  shot - the arrow of the anchored shot always lands.

## What this means for the improved kite

The current `kiteFromShot` (hunt/kite.go) clicks the retreat at the
Attack broadcast (t=0). The server defers that click to 2.97 s, and
the re-attack request (gated by `combatAvoidUntil`, sized to the full
`kiteWalkWindow` = 2.97 s) fires at the same moment - the walk starts
only to be cancelled by the next attack. The bot spends the whole
reload standing still: the PREDICTED fleet retreat lag (from the
measured deferral plus the code reading of the gate; the
`archer-fleet` audit is the instrument that will confirm it live)
rides at ~3 s per cycle. The fleet audit itself has not run live yet -
its live round is the next step of the issue.

The measured window says the retreat should click at the windup end:

1. **The first retreat click waits the windup**: delay the kite walk
   to `(timeAtk+reuse)/2` (1500 ms at pAtkSpd 337; the formula reads
   the live `SelfPAtkSpd`) after the own Attack broadcast. The click
   then lands in the accepted window and the movement starts at once
   - 1.47 s of walking per cycle instead of none.
2. **The walk window shrinks to the reuse tail**: from the windup end
   to the disable end (2.97 s - 1.5 s = 1.47 s, ~250 units at the run
   speed 170) - the re-attack request fires at the disable end
   exactly as it does today, but the bot ARRIVES at the re-shot with
   real retreat distance banked.
3. **The deferred click is not a refusal**: the re-click ladder's
   ActionFailed attribution (`kiteClickRefused`) must not rotate the
   endpoint for a click issued inside the windup - the answer is a
   deferral, not a dead cell. The ladder can simply stop re-clicking
   inside the windup (one click at the boundary replaces the 250 ms
   re-click storm).
4. **The re-shot owns the boundary**: a retreat click that arrives at
   or after the disable end (2.97 s) races the auto re-shot and loses
   a full cycle (the 3000 ms row). The retreat click must fire
   strictly between the windup end and the disable end.
5. **The WASD stream is safe but unproven as movement**: the claims
   never spoiled a shot, so a future cursor-key retreat lane can ride
   the claim stream (the escape lane already uses it); making it move
   through the windup needs the follow-up probe round.

The mass audit that measures which of the five behaviors the fleet
implements today is the `archer-fleet` scenario (five archers on five
cells, under five minutes, `SWARM_ARCHER_FLEET_MINUTES`); the fold
thresholds encode the measured numbers (the retreat lag ceiling of
2200 ms separates a windup-end click from a deferred one).

## The verification ladder

- The pure halves (the round classification, the ladder summary, the
  fleet fold verdicts) are pinned by the unit tests of
  `kite_timing_probe_test.go` and `archer_fleet_test.go`.
- The live probe: `go run ./cmd/swarm -acceptance kite-timing-probe`
  (PASSED 2026-09-23, all ten rounds plus the cursor key round).
- The live fleet audit: `go run ./cmd/swarm -acceptance archer-fleet`.

## The implementation (2026-09-23, the deferred click round)

The recommendations 1, 2 and 4 landed on `feature/improved-kite`
(issue #70):

- `kiteArmClick`/`kiteClickWalk`/`kiteShotPhase` (kite.go): the own
  Attack broadcast now ARMS a deferred retreat instead of clicking at
  once - `kiteShotPhase` reads the live `SelfPAtkSpd` through the
  same clamped C1 formula as `kiteWalkWindow`, the windup is exactly
  its first half, and the click waits the windup end plus
  `kiteWindupLead` (150 ms - the safety margin for the quarter-second
  loop cadence and the broadcast lag against the measured 1483 ms
  boundary). The click fires only strictly before the disable end
  (the `live` gate of `kiteShotPhase`) - the re-shot race of the
  3000 ms probe row cannot happen.
- The fire re-checks everything the standing windup changed: the
  owning fight (a target switch disarms the schedule AND releases
  the arming's own movement hold - the Equal guard keeps a walk in
  flight and the fire-time threat re-check keeps the shot's real
  disable hold), the pursue band and the whole lane machinery (the
  train direction, the camp deflection, the dead cells resolve at
  the click moment, not the broadcast moment).
- `kiteResolveAndClick` is the one seam every path shares (the
  deferred click, the accepted-window catch of a fast bow, the
  ordinary proximity step): one resolution, one click path, one
  re-click ladder. The walk window of a live cycle ends at the
  shot's disable end - the walk banks the reuse tail (finding 2).
- `avoidImpendingAdd` now respects the shared `combatAvoidUntil`
  window: an add click inside a windup would defer to the disable
  end and its window overwrite would reopen the re-request gate onto
  the running kite walk.
- `state.Bot.ApplyAttackAt` is the timestamped twin of
  `ApplyAttack` - the repro scenes stage a shot a controlled age in
  the past without sleeping the test out.
- Recommendation 3 came free: a click issued inside the windup no
  longer exists, so the ladder's refusal attribution never sees the
  deferral answer. Recommendation 5 (the WASD retreat lane through
  the windup) stays open - it needs the follow-up probe round.

## The live fleet audit (2026-09-23, the third round - the measured verdicts)

The archer-fleet audit ran to its first COMPLETE live verdict (three
rounds, three infrastructure fixes: the entry latch for the
launch-minute death of a starter archer, the registry wiring that
left the audit blind to its own five bots, the fold direction gate
below). The window: 2m54s, five slots, 163 kite steps and 12 holds
across the fleet. The per-behavior verdicts of the four fighting
slots:

- **The deferred retreat works live**: every shot logged the arm
  ("the retreat clicks at the windup end") and the fire ("kiting the
  reload tail") ~1.6 s later; the median shot-to-retreat lag
  measured 1.8-2.0 s - inside the 2200 ms ceiling of the accepted
  window (the click lands, the movement broadcast follows). The
  early-retreat behavior PASSES on all four fighting slots.
- **quick-reshot measured 0 of 5 on the raw fold** - a measurement
  artifact, not a behavior gap: the plain walk population fed the
  median loot pickups and cell-rotation approaches (a 3.0-3.2 s
  walk-end-to-shot median on walks the kite never issued). The fold
  now gates on the away direction (commit 169e8fa); the honest
  re-measure rides the next live round.
- **max-range failed on the three Kaboo cells** (median fight
  distance 145-160 units): the level 7 starter kit runs at speed 125
  - the same ground speed as the Kaboo Orcs - so the 1.47 s walk
  tail gains nothing the 1.5 s windup standstill does not give back.
  The retreat rhythm keeps firing (the behavior is implemented), but
  the kit cannot hold the 250-450 band at speed parity; the mixed
  Dryad slot held a 496 median. Holding the band needs a speed edge
  (haste, buffs, a faster kit) - a gear question, not a kite bug.
  **[CORRECTED 2026-09-23, the continuation round: the parity claim
  was WRONG.** The Mobius NPC tables (dist/game/data/stats/npcs)
  give every elven-ground chaser - Kaboo Orc, Kaboo Orc Grunt,
  Kaboo Orc Archer, Kaboo Orc Fighter, Green Dryad, Spore Fungus,
  Gray Wolf - a run speed of 110 against the character's 125: the
  fight loses the distance on the WINDUP DEBT alone (the ~1.45 s
  movement standstill costs ~160 units a cycle, the reload-tail
  walk banks back only ~22 at the 15 unit edge). The fix is not
  gear: the pursuit hold (the re-shot gated on the 410 unit floor)
  keeps the walk running until the distance is regained - the
  guarded round measured the medians rising 3-6x (322/188 against
  68/49) on the slots that fought.]
- **curved-retreat failed on four slots** (the anchor leash broke at
  1644-2367 units): the retreat lane machinery walks the away-ray
  with lane skipping and camp deflection, but nothing rotates the
  retreat direction around the farm point - the fifth behavior of
  the issue (the curving circle that stays near the farm point) is
  genuinely NOT implemented. The audit names it precisely now.

The open ladder after this round: the direction-gated re-measure of
quick-reshot (one more live round), the curved retreat
implementation (a rotating retreat direction preference around the
cell anchor), and the kit speed question for max-range.

## The final round (2026-09-23, the curve live)

The curving retreat (commit 3a30513) ran its first live round: the
fight's own circle (the opening straight retreat, then the fixed 70
degree tangential bearing on the turn side picked toward the zone
center), the window-lapse guard of the deferred click and the fold
reject-path test from the QA round all landed first. The verdicts:

- **quick-reshot holds on the fighting slots** (median walk-end-to-
  shot gap 0 s over 11-18 walks each): the re-shot interrupts its
  own retreat walk - the shoot, run the tail, re-shoot rhythm the
  issue asks for, measured.
- **max-range rose to 3 of 5** (276, 442 and the non-fighting
  slot): the tangential circle keeps the mob near the band edge
  better than the straight march; the two failing slots are the
  speed-parity Kaboo cells of the previous round (the gear
  question).
- **early-retreat holds where the fight ran** (1.9-2.0 s medians);
  the slot that spent the window at melee (4 shots) reports no
  evidence - the floors name it.
- **curved-retreat still fails on the fighting slots - now a
  METRIC question, not a code gap**: the anchor leash (1500 units
  from the SPAWN point) measures the total displacement including
  the hunt machinery's own cell rotation ("the fight crossed into
  Green Dryad SW-5, following the ground" - the legitimate cell
  drift of the farm cluster), and the leash sits below the natural
  cell-cluster span. The curve itself is unit-pinned (the opening
  straight step, the 70 degree bearing, the fresh-target reset, the
  zone-centered turn side) and bounds the retreat-attributable
  drift; the honest next step is the same attribution fix the
  reshot gap got - measure the leash on the retreat displacement,
  not the spawn anchor.
