# Task: the kite timing probe and the archer fleet audit (issue #70)

Goal: measure how the Mobius C1 server answers a retreat click after
the bow shot started (the applied experiment), and build the mass
acceptance audit that names which of the five improved-kite behaviors
the fleet does not implement today.

## The five audited behaviors

1. Hold the maximum distance from the target.
2. Shoot the bow.
3. Regain the distance before the reload ends (not waiting the full
   cooldown).
4. Re-shoot the moment the distance is back.
5. Retreat on a curve that stays near the farm point (no straight
   line runaways).

## State (2026-09-23, session 1)

- `internal/swarm/acceptance/kite_timing_probe.go`: the
  `kite-timing-probe` scenario (account temp19). A raw wire session
  (no hunt loop): dresses the bow kit via UseItem, forces attacks on
  the Kaboo Orc cell mobs, walks a ten step retreat delay ladder
  (0..3000 ms after the own Attack broadcast), classifies every round
  (immediate / deferred / no-move, refusal counts, move lag from the
  shot), then a cursor key round (MoveToLocation mode 0 +
  ValidatePosition claim stream) that measures whether the character
  moves through the windup and whether the target still takes the
  damage while it moves.
- `internal/swarm/acceptance/archer_fleet.go`: the `archer-fleet`
  scenario (temp20..temp24) - five archers on five different Kaboo
  cells (elven-hex-104, 106, 105, 086, 087), staggered logins, a 2.5
  minute audit window (SWARM_ARCHER_FLEET_MINUTES, whole scenario
  under five minutes), 250 ms per-bot sampling folded into the five
  behavior verdicts (median fight distance, shot count, median
  shot-to-retreat lag, median walk-end-to-shot gap, anchor leash plus
  walk turns). The failure list names the unimplemented behaviors.
- Unit tests: `kite_timing_probe_test.go` (classification, ladder
  summary, recorder, geometry, registration), `archer_fleet_test.go`
  (slots, reset, fold verdicts against synthetic improved and legacy
  streams, medians, registration).
- The exhaustruct ignore patterns of the new aggregates landed in
  `.golangci.yml`.

## Server facts the probe is built on (the Mobius source read)

- The bow shot commits at the Attack broadcast (t=0): the hit roll,
  the arrow consumption and the HitTask schedule happen before it,
  and the damage task carries no attacker movement check - nothing
  after the broadcast cancels the hit.
- `PlayerAI.setIntentionMoveTo` DEFERS a mouse-mode MoveToLocation
  while `isAttackingNow()` holds (the first half of the bow cycle,
  (timeAtk+reuse)/2 ~= 1.48 s for the Short Bow at pAtkSpd 337): the
  click draws an immediate ActionFailed and replays at the cycle end
  (timeAtk+reuse ~= 2.97 s). A click in the reuse tail lands
  immediately.
- The cursor key movement (MoveToLocation mode 0 + the
  ValidatePosition claim stream) bypasses the deferral: the packet
  handler latches the cursor key flag before the deferrable AI
  intention, and the per-claim position adoption has no attack check
  - the true WASD shoot-while-moving window.

## Next steps

- Run `kite-timing-probe` on the live stack and record the measured
  boundary (the earliest immediate retreat delay) and the WASD round
  answer into `docs/kite_timing_findings.md`.
- Run `archer-fleet` and record which behaviors fail today.
- Write the fix proposals (see the findings doc skeleton in the
  report of this branch).

## Results (2026-09-23, session 1, live run 14:06-14:08)

The probe PASSED - all ten ladder rounds plus the cursor key round:

- The mouse-mode retreat click is deferred inside the windup: rounds
  at 0/300/600/900/1200 ms all moved at 2.97 s after the shot.
- The earliest immediate retreat: 1500 ms after the shot (the windup
  end, (timeAtk+reuse)/2 at pAtkSpd 337) - rounds at 1500/1800/2200/
  2600 ms all moved at delay + RTT.
- The 3000 ms round raced the auto re-shot (2.97 s) and deferred into
  the NEXT cycle (5.93 s).
- The cursor key round: 7 claim adoptions echoed, 0 net units moved,
  the damage landed anyway (HP 89% -> 76%) - the WASD stream never
  spoiled the shot.

The full analysis and the fix proposals live in
docs/kite_timing_findings.md.

## Next steps

- Run `archer-fleet` on the live stack (the code is registered and
  unit-tested; the session clock ended before its live round).
- Implement the windup-end retreat click in hunt/kite.go per the
  findings doc.
- The cursor-key follow-up round: pin why the adopted claims produced
  no net movement (the deferred intention snap-back hypothesis).

## The critique round (2026-09-23, session 1 end)

The review subagent verdict: the probe half is sound (live-measured,
defensible); the fleet half is untested live code with known
weaknesses. Applied in the final pass: the first distinct shot counts
(off-by-one), the retreat evidence floor rose to 2, the audit window
truncation now fails loudly instead of issuing a verdict on partial
data, the dead recorder/watch fields are gone, the findings doc no
longer claims the fleet lag was observed (it is a prediction until
the fleet runs live).

Known open gaps for the next session (ranked):

- P0: run `archer-fleet` live and rewrite the findings from its
  measurements (the scenario, the folds and the thresholds are
  unvalidated against the real five-bot behavior).
- P1: the fleet fold attributes EVERY walk to the retreat medians
  (loot pickups and approach walks pollute); gate the lags on walks
  that move away from the fight target.
- P1: the turn heuristic counts any 40 degree direction change (the
  approach/loot turns rack up); count turns only during
  retreat-flagged walks.
- P1: the WASD round measured net gain only - log the per-claim self
  positions to separate "never moved" from "moved and snapped back",
  and stream past the disable end (the reload tail the task also
  asked about).
- P1: the ladder wants a 2800 ms rung (the 2.97 s re-shot boundary)
  and repeated boundary rungs.
- P1: the probe passes with red checks by design (the measured
  values are data, not contracts) - the semantics deserve a cleaner
  pass/fail split.
- P2: the gofmt field alignment of the new files (run task fmt).

## 2026-09-23, the fix-and-acceptance round (the second session of the day)

Pushed (all on feature/improved-kite):

- 34ee42e: the DEFERRED RETREAT itself - kiteArmClick/kiteClickWalk/
  kiteShotPhase (the windup-end click with the 150 ms lead, the
  window ending at the shot disable end, the ownership disarm that
  releases only the arming's own movement hold), the shared
  kiteResolveAndClick seam, the avoidImpendingAdd window gate, the
  state.Bot.ApplyAttackAt timestamped seam, and the whole kite suite
  re-pinned to the deferred-click contract (the tickPastTheWindup
  dance; the honest target-switch scene through the server
  selection).
- 39b232c: the fleet entry latch (a bot that died mid-launch still
  entered).
- c543a43: THE REGISTRY WIRING - the fleet slots' trackers were
  never in the bot registry (NewManager seeds one tracker per
  scenario definition), so the audit polled throwaway offline twins;
  ensureFleetTracker registers the five slots before the launch.
- 169e8fa: the fold direction gate - only walks that move away from
  the fight target feed the retreat medians (the raw round measured
  a 3.1 s reshot median on loot/approach walks).
- b17d712: the findings doc carries the measured live verdicts.

The live verdicts (the first COMPLETE fleet round): early-retreat
PASSES on the four fighting slots (median shot-to-retreat 1.8-2.0 s
- the deferred click works live); quick-reshot needs the gated
re-measure; max-range fails at speed parity (the starter runs 125,
the Kaboo runs ~125 - the walk tail cannot outpace the windup
standstill; a gear question, not a kite bug); curved-retreat is
genuinely unimplemented (the anchor leash broke at 1644-2367 units
on four slots - nothing rotates the retreat around the farm point).

Next steps, ranked:

- P0: rerun the fleet with the direction-gated fold (the honest
  quick-reshot number).
- P1: implement the curved retreat - a rotating retreat direction
  preference around the cell anchor (the fifth behavior of the
  issue; the straight away-ray drifts off the farm point).
- P1: the max-range speed parity - evaluate a haste/buff path or a
  faster kit for the level 7 window.

## 2026-09-23, the QA round and the curve (the session close)

The clean-context QA sub-agent verdict (12 findings: the unexecuted
gated re-measure, the unimplemented curve, the untested fold reject
path, the window-lapse race, plus polish) drove the closing round:

- The gated re-measure ran (two live rounds): quick-reshot PASSES
  on the fighting slots at a 0 s median - the re-shot interrupts
  its own walk. Measured, not predicted.
- 3a30513: the CURVING RETREAT implemented (the fifth behavior):
  the opening straight retreat, then the fixed 70 degree tangential
  bearing on the fight's own turn side (toward the zone center),
  the constant contract pinned (cos(step) >= the half-plane slack),
  the rotation ladder stays away-centered, the window-lapse guard
  and the fold reject-path test landed with it.
- 8174d8b: the final live round documented - max-range 3 of 5 (the
  two fails are the speed-parity cells, a gear question), the
  curved-retreat leash metric named as the remaining attribution
  gap (it measures the spawn-anchor displacement including the
  hunt's own cell rotation).

Open for the next round, ranked:

- P0: the leash metric attribution - measure the curved-retreat
  leash on the retreat displacement (the same fix the reshot gap
  got), or anchor it to the live cell focus instead of the spawn.
- P1: the max-range speed parity on the Kaboo cells (haste/buffs/
  a faster kit for the level 7 window).
- P1: the fleet's per-slot evidence floors could name the
  spent-the-window-recovering slots in the fleet verdict line (the
  per-bot lines already do).

## 2026-09-23, the fix-and-acceptance continuation round (the third session)

The task: continue fixes, upload BEFORE the tests, run the acceptance
live rounds, fix the remaining issues. Six commits pushed (all unit-
gated, all live-measured where the clock allowed):

- 96c1c56 (pushed by the prior context): the per-fight leash metric
  (the P0 of the open ladder) - the drift anchors on the mob's stand
  at each fight's start, only confirmed-retreat fights feed it, the
  curve rides the signed corner between consecutive retreats of ONE
  fight.
- c19bc59: the intrange lint fix on top.
- c1ebecb: THE GAP RETREAT - the fleet round 6 measured the standing
  encircled answer as a death trap (temp21 stood 26 s shooting one
  spot while the train grew to three chasers): the encircled train
  now takes the widest-gap bisector (the perpendicular break of a
  two-mob line opens distance from BOTH chasers), the lane battery
  keeps the terrain authority, the hold survives for the walled gap
  alone (kiteChaserBearings feeds both the centroid and the gap).
- a64d492: THE FULL-HP START - the reset wrote the gain-table 167 HP
  but the server recomputes the level 7 maxima at login (214/82
  measured), so the slots entered wounded (temp20: 167 of 214, the
  window spent on potions, an emergency logout and a 60 s sit): the
  fleet reset now overrides the vitals with the measured values.
- 568f7f8: the window 2.5 -> 3.0 minutes on the fleet's own measured
  launch budget (40 s; the entry latch keeps a late slot harmless) -
  the corner evidence of the curving verdict needed the fights.
- d46eccd + c5d845b: THE PURSUIT HOLD - the Mobius tables settled
  the speeds (every elven-ground chaser runs 110, the elven fighter
  125 - the old 'speed parity' verdict was WRONG): the re-shot at
  the walk-window end restarted the windup and handed ~160 units
  back every cycle (the max-range medians 13-227). The hold gates
  the re-request on the re-shot floor (the windup debt + the retreat
  radius, 410): a lapsed window whose hostile holds inside the floor
  continues the retreat (the streak counts it), the affordable shot
  answers the moment the floor clears. The second commit added the
  retreat-context guard (the approach phase never retreats - the
  round 9 live measurement: a slot walking its cell empty, 0 shots).

The live ladder (all rounds in runs/fleet-2026-09-23/, the per-run
logs under logs/acceptance/):

- Round 6 (the per-fight leash): shoots 4/5, early-retreat 3/5,
  quick-reshot 4/5, max-range 2/5, curved 2/5 - the audit wiring
  finally measured real behavior (the entry/registry/fold fixes of
  the prior sessions hold).
- Round 7 (the gap retreat + the full HP): shoots 5/5, early-retreat
  4/5 (temp21's 3.1 s lag GONE - the gap escape works), temp23 5/5.
- Round 8 (the 3-minute window): shoots 5/5, early-retreat 5/5
  (1.7-2.0 s on every slot), quick-reshot 5/5 (0 s medians),
  curved-retreat 4/5 (temp21 1597 against the 1500 leash), max-range
  0/5 - the longer exposure settled the honest verdict: the windup
  debt collapses every fight to melee without the hold.
- Round 9 (the pursuit hold, pre-guard): temp24 5/5 (the max-range
  median 322 against 68, the re-shot gap 100 ms), temp23 4/5 (188
  against 49), the medians rose 3-6x on the fighting slots - but the
  approach-phase regression (the missing context guard) spent three
  slots walking instead of fighting; the guard landed as c5d845b.

Open for the next round, ranked:

- P0: re-run the fleet live with c5d845b (the guard was unit-pinned
  but not live-measured - the approach slots must fight again while
  the pursuit medians hold).
- P1: temp22's 8.5 s retreat lag (4 retreats, the walled pocket of
  elven-hex-105 - the cornered holds still eat the windows; the
  dead-cell rotation may need the gap ray in its sweep).
- P1: the fleet verdict line could name the spent-the-window slots
  separately from the true fails (the per-bot lines already do).
- P2: the max-range floor of the audit (250) sits below the pursuit
  equilibrium (~320): consider raising it to the measured band once
  the guarded round confirms.
