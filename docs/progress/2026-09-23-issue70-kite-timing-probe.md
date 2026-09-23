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

## 2026-09-23, the walled-pocket round (the fourth session)

The task: continue fixes, upload BEFORE the tests, run the acceptance
live rounds, fix the remaining issues. Pushed 1773892a BEFORE the
live round, then measured:

- 1773892a: THE CORNERED-POCKET BREAKOUT - the round-9/10 fleet
  measured the standing holds eating whole windows on the pocket
  cells (temp22: an 8.5 s then a 15.2 s median shot-to-retreat lag,
  23 holds in the round-10 window alone). When the whole retreat
  hemisphere answers walled, the anti-gap ladder now probes the cone
  the sweep never covered - the two 135 degree weaves off the gap
  ray, then the straight anti-gap ray - under the flank clearance
  (every candidate keeps more than ~36 degrees off EVERY chaser
  bearing), the full terrain battery, and the dead-cell skip. The
  terrain core (kiteTerrainLane) factored out of kiteLaneResolve;
  both call sites wired (the opening/deferred resolution and the
  dead-endpoint rotation).
- 1773892a: the fleet verdict line splits "not passing" (the
  evidence armed and fell short) from "no evidence" (the slot spent
  the window recovering) - the fleet round rendered it live:
  "quick-reshot (3 of 5 bots (no evidence: temp21, temp22))".

The round-10 live verdict (runs/fleet-2026-09-23/round10.log, the
log under logs/acceptance/archer-fleet-20260923-181200-*.log):

- shoots 5/5 (36/36/9/34/34 shots - the c5d845b approach guard
  confirmed live: the round-9 walkers fight again), early-retreat
  3/5 (temp23 1.9 s, temp24 2.0 s), quick-reshot 3/5 (100 ms and
  0 s medians), curved-retreat 2/5 (temp23 1029 units, temp24 765).
- The breakout fired 0 times across 14 encircled holds - the
  round-10 log could not say WHY (the refusal was silent). The
  diagnostics landed after the round: the hold line now names the
  refusal (no gap geometry / the flanks crowd the cone / the terrain
  walls the cone, with the counts).
- temp21 (hex-106) 8.8 s / temp22 (hex-105) 15.2 s retreat lags:
  both pocket cells hold the archer through multi-cycle pockets -
  the breakout diagnostics of the next round name the refusing gate.
- max-range 1/5 (temp21 511): the honest physics - the fresh-target
  streak reset (e2537f7) lets every pursuit chain run its full 8
  steps, and the windup debt (~160 units per cycle) exceeds the walk
  gain (~48 units per window at 125 vs 110 speed), so the crowded
  cells collapse to 62-220 medians no matter the chain depth. The
  round-9 322 median was the never-resetting streak capping the
  chains at ~4 steps - the re-shot fired from the higher distance.
  The quiet cell (temp21) holds 511: the kite works where the fight
  starts at range and the train stays thin. A gear question (haste)
  on the mass cells, not a kite bug - the audit floor stays 250.
- kitePursuitContext 6 -> 12 s: the live chains pause through
  cornered hold episodes (a 3 s hold block + a windup) that never
  ended the fight - the 12 s bound still expires under the fastest
  same-id respawn (RespawnMin 15 s).

Open for the next round, ranked:

- P0: the diagnostics live round - the hold lines must name which
  gate refuses the breakout cone on the pocket cells (the crowd or
  the terrain); the fix follows the reason.
- P1: the curved drift 1604/1698 against the 1500 leash on temp20/
  temp22 (the pursuit chains run farther per fight than the leash
  was calibrated for - the pocket cells push the retreat around).
- P2: the max-range gear question (haste/kit) stays open on the mass
  cells.

## 2026-09-23, the diagnostics round (round 11) and the QA close

The round-11 live verdict (runs/fleet-2026-09-23/round11.log, the
per-run log under logs/acceptance/archer-fleet-20260923-182224-*.log)
- the diagnostics round the fragment above asked for, run BEFORE this
section was written:

- THE POCKET CELL RECOVERED: temp22 (hex-105) passes max-range (280
  units), early-retreat (2.0 s median) and curved-retreat (1196
  units, 4 same-side corners) - the cell that measured 15.2 s and
  1698 units one round earlier. temp23 hits 496 units max-range.
  The leading suspect is the 12 s pursuit context (d9e173b8): the
  chains now survive the cornered hold episodes that the 6 s stamp
  expired mid-fight. UNVERIFIED attribution - the round changed one
  variable, the next round owns the confirmation.
- THE BREAKOUT HYPOTHESIS IS REFUTED for the observed pockets: the
  hold diagnostics answered "no gap geometry (the lone chaser owns
  the corner)" on every logged refusal - the pocket holds are
  LONE-CHASER terrain corners, not encircled trains. The breakout
  ladder (needs 2+ bearings) cannot even run there and fired 0 times
  across both rounds; its flank-clearance tier is live dead code
  until a real encircled pocket appears. The honest count of the
  round-10 "surround" holds is UNATTRIBUTABLE (the wording covered
  both the encircled and the degenerate zero-bearing scene before
  the diagnostics existed).
- REGRESSIONS vs round 10 on the same code plus the context change:
  early-retreat 1/5 (temp24 6.2 s vs 2.0 s, temp23 2.4 s vs 1.9 s),
  quick-reshot 1/5 (was 3/5), temp21 max-range 49 units (was 511),
  temp20 a spent window (1 shot, 69 samples - named "no evidence"
  only on four behaviors; the max-range line misclassified it, fixed
  below). The plausible mechanism (UNTESTED): with the context at
  12 s a pursuit continuation can start a walk up to 12 s after the
  last shot, and the fold's pendingLag has no ceiling - the
  multi-second pursuit walks mint into the early-retreat medians;
  the longer chains also starve quick-reshot of walk-end shots
  (round 10: 42 continuation lines; round 11: 22).
- The clean-context QA audit (Task ID 2, 10 findings) drove the
  close: the max-range evidence split now arms on the shooting
  evidence too (a slot with fight samples but no shots reads "no
  evidence", not a behavior fail - the mixed case pinned in
  TestFleetVerdictsNameTheSpentWindow), and this section records
  the refuted hypothesis the pushed tree was missing.

Open for the next round, ranked:

- P0: attribute the round-11 regressions - cap the fold's
  pendingLag or gate the pursuit-continuation walks out of the
  early-retreat metric, decide 6 s vs 12 s context on the evidence,
  re-run the fleet.
- P1: the pocket re-scope - the lone-chaser terrain corner needs a
  terrain-aware escape (a perpendicular sweep along the wall face)
  or an explicit decision to accept the hold-and-shoot answer; the
  anti-gap breakout waits for a real encircled pocket.
- P1: the max-range levers the "gear question" framing dismissed:
  the re-shot floor (410) sits ~100 units under the measured passing
  band (496-511), and the flat streak limit (8) measurably lowers
  the medians (the round-9 never-resetting streak measured 322).
  Try the floor toward the band and a leash-aware streak budget
  live before resting the claim on gear.
- P2: the pursuit-context expiry (12 s) has no unit test - pin the
  13 s backdated stamp refusing the continuation; the breakout
  bounds (the 93-135 degree wedges unswept, the angular-only flank
  clearance) belong in the doc comment.

## 2026-09-23, the attribution round and the wall-face escape (rounds 12-13)

The task: continue fixes, upload BEFORE the tests, run the acceptance
live rounds, fix the remaining issues. Pushed 05f69db7 (the
attribution + the levers) BEFORE round 12, then 593b4fd1 (the
wall-face escape) BEFORE round 13.

- 05f69db7: THE ROUND-11 REGRESSION ATTRIBUTED - the wire-level log
  mining paired every pursuit-continuation walk to the last shot:
  temp24's 6.2 s median was 7 continuation walks (3.0-9.0 s lags)
  against 4 honest 1.8 s windup-end retreats of the same slot. The
  behavior was right, the metric was wrong. The fold now books the
  FIRST confirmed retreat of each shot alone (lastRetreatShotAt).
  The pursuit chain also owns its budget apart from the proximity
  streak: the per-shot-cycle ledger (kiteArmClick resets it every
  fresh shot, kiteIssueWalk on a fresh chain) counts only COMPLETED
  walks (a cornered hold re-probe never mints a stall), stops the
  parity race after two stalled windows (the progress gate, slack
  20 under the honest 45-unit window gain) and the never-resolving
  chain at the flat backstop 12. The re-shot floor rose 410 -> 480
  (the cycle median rides ~80 under the floor; the passing slots
  measured 496-511).
- 593b4fd1: THE LONE-CHASER WALL-FACE ESCAPE - the round-12 live
  verdict named the remaining early-retreat fails (temp24 6.2 s over
  3 retreats) as refused windup-end retreats, and every pocket
  refusal read "no gap geometry (the lone chaser owns the corner)".
  The corner now probes the wall-face wedges the fan never swept
  (the away ray folded 105/120/135 degrees each side) under the
  flank clearance, the full terrain battery and the dead-cell skip;
  the sealed pocket keeps its honest named refusal. The candidate
  evaluation factored into kiteBreakoutCandidate (both ladders
  share it; the refactor clears three pre-existing nlreturn
  findings).

The round-12 live verdict (runs/fleet-2026-09-23/round12.log, the
per-run log under logs/acceptance/archer-fleet-20260923-193759-*):

- 16 of 25 behaviors (the best round yet; round 11 measured
  10): shoots 5/5, max-range 3/5 (temp20 443, temp23 471, temp24
  334 - the passing medians rode the 480 floor; temp21 177 and
  temp22 88 are the crowded shared central cells), early-retreat
  3/5 (the fold gate cleaned temp21 to 1.9 s and temp23 to 2.0 s;
  temp22 2.6 s and temp24 6.2 s are the hold-dominated slots),
  quick-reshot 3/5 (temp20 2.8 s, temp23 1.6 s - the walk ends into
  holds), curved 2/5 (the no-evidence slots spent their windows on
  short volley fights).
- ERRATUM (the QA audit of this round caught it): the commit
  message of 593b4fd1 and an earlier draft of this section misread
  the fleet line's "N of 5 bots" as the FAILING count and claimed
  14/25 - the actual round-12 verdict line reads max-range 3 of 5,
  early-retreat 3 of 5, quick-reshot 3 of 5, curved 2 of 5. The
  numbers above are the corrected record.

The round-13 live verdict (runs/fleet-2026-09-23/round13.log, the
per-run log under logs/acceptance/archer-fleet-20260923-195432-*):

- THE ESCAPE FIRED LIVE: 21 "breaking out" lines (round 12: 0), the
  old lone-chaser refusal is GONE - exactly one sealed pocket reads
  the new "the lone-chaser cone refused (0 of 6 rays crowded by the
  flanks, 6 walled or dead)".
- quick-reshot 3/5 (200 ms medians), early-retreat 2/5 (temp21 6.2
  s over 12 retreats on the hex-106 crowd, temp24 31.2 s over 2 - a
  recovery-dominated window), max-range 1/5 - the crowded-cell
  medians fell (temp23 471 -> 224, temp24 334 -> 74): the round
  stacked the bots onto the shared central cells harder (temp20 a
  spent window at 71 samples, temp24 at 320). 14 of 25 behaviors -
  a REAL 2-point decline from round 12's 16, not a hold: the escape
  fired 21 times but the max-range verdict fell 3/5 -> 1/5 on the
  crowd (the 105-135 degree folds open distance slower, the stall
  gate may stand them down - the P1 below), and the surround holds
  own the rest. The P0 crowd attribution owns the next round.

Open for the next round, ranked:

- P0: the crowd attribution - the fleet's five slots share the
  central cells run-to-run (round 13 stacked three bots on the same
  ground; the medians swing 74-471 on the same cell across rounds).
  The audit needs either a per-bot cell lock in the scenario reset
  or a crowd-aware verdict split before the max-range behavior is
  measurable on the mass cells.
- P1: temp21's 12 late first-retreats on hex-106 (the surround-hold
  cell): the surround hold has no escape ladder - the encircled
  train case (2+ bearings, the anti-gap cone) refused 0 times in
  round 13, the HOLDS are the gap. The hold-and-shoot answer may be
  correct there; the verdict needs the hold time named per slot.
- P1: the wall-face escape's race: the 105-135 degree folds open
  distance slower than the straight retreat - the pursuit ledger's
  stall gate may cut them short on the crowded cells (check the
  stall refusals against the 21 breakouts).
- P2: the round-to-round variance itself (three rounds, three
  different fail sets on the same code) argues for a repeated-round
  median-of-medians verdict in the audit before any single-round
  FAIL names a behavior unimplemented.

## 2026-09-23, the round-14/15 always-run redesign (the owner
## escalation: "the archer still does not run, eats damage, the
## solution is location-tied, the fleet dress is super slow")

The round started from the owner's four named failures and answered
each; 88eebef6 (the redesign), d453419c (the pins), 2d93da71 (the
live verdict fixes) all landed BEFORE the live rounds.

- THE ALWAYS-RUN RULE (88eebef6): every "stand and fight it out"
  fallback is GONE - the proximity streak limit (8), the pursuit
  stall (2) and streak (12) stops now only NAME the unwinnable race
  in the log (the once-per-crossing diagnostics) while the retreat
  keeps running. The cornered pocket gained the SHOVE tier
  (kiteShoveResolve): the gap ray itself (+-15/+-30 edges) at a 25
  degree flank bound and a shorter 250 unit step - the seam the
  breakout cone refuses by design.
- THE LOCATION-GENERAL LEASH (88eebef6): the retreat leash and the
  curve turn side read the FIGHT ANCHOR (the live self position at
  the fight's first retreat resolution, kiteFightX/Y) instead of
  the hunting-square registry - the zone.Contains read is gone, the
  kite works on any map with or without a zone registry, the
  elven/dion tables own the target economy alone. kiteRoamRadius
  (1350) fences the drift around the anchor.
- THE MOTION LEDGER (88eebef6, kite_ledger.go): every fight books
  moving/windup/hold/idle seconds and prints the summary line with
  the moving share and the health cost at the fight end - the
  feedback the owner asked for, in the live log.
- THE EQUIP DEFERRAL GUARD (88eebef6): the Mobius UseItem handler
  DEFERS an equip landing inside the attack window to the attack
  end (~1.5 s windup / ~3 s cycle); the old 600 ms same-item
  re-send expiry re-sent INTO the deferral and the second packet
  TOGGLED the piece back off (the equip/unequip war = the "super
  slow" dress; a stuck quiver leaves the bow unable to shoot). The
  same-item guard now outlives the deferral (4 s) and the burst
  serializes one equip per tick (the PacketExecutor has no
  same-client ordering - same-tick jewel pairs raced the paperdoll
  placement read). The round-14 log measured the fix: the whole kit
  lands in ~2.5 s, plan order, no toggles.
- THE FLEET CROWD FIX (88eebef6): SetCellPin locks a hunt to one
  named cell - each fleet slot farms its OWN ground (the rounds
  12/13 crowd measured three bots converging onto the shared ripe
  cells). The fleet audit gained the ALWAYS-RUN verdict
  (checkFleetRuns): the avoidable standing share of the fight
  samples (standing outside the 1.65 s windup window), ceiling 35
  percent.
- THE RANGED-DUEL STRAFE (2d93da71): the round-14 ledger measured a
  49 s fight with 2.3 s of moving - a Kaboo SHOOTER held 474 units
  (beyond the 450 pursue band) and traded arrows while the archer
  stood 21 s of idle; the melee adds closed on the stationary
  target. A LIVE ATTACKER beyond the band now keeps the rhythm
  (kiteThreatBeyondThePursue) - the strafe rides the curving
  retreat, capped at the train scan range.
- THE PASS-THROUGH TIER (2d93da71): the round-14 surround holds
  (the 3+ mob pack at 29-80 units) had NO ray inside the 25 degree
  bound; the least-crowded terrain-passable ray now carries the
  shove anyway - a glance blow on the way out beats the whole
  pack's swings for as long as the fight stands. Only a pocket
  walled on every side keeps the hold.

The round-14 live verdict (runs/fleet-2026-09-23/round14.log): 21
of 30 behaviors (the best round yet; round 13 measured 14 of 25).
always-run 4/5, early-retreat 4/5, quick-reshot 4/5, curved 4/5,
shoots 4/5; the fails: max-range 1/5 (the melee-collapse medians
57-179 on the pack cells), temp20 a spent window (0 shots, the
death-restart cycle on the shooter cell).

The round-15 live verdict (runs/fleet-2026-09-23/round15.log,
after 2d93da71): 22 of 30 - always-run 5/5 (the owner's
headline ask; ERRATUM from the QA audit: the temp21 pass was
VACUOUS - 0 shots, 109 samples, 12 percent avoidable standing - a
slot that never fought read as a pass; the honest count is 4/5
plus one no-evidence, and the verdict's evidence floor now rides
the shooting evidence), max-range 2/5 with the fixed cells at
458/466 medians (was 1291/57), temp20 now fights (34 shots), the
STRAFE visible in the log (the "holds 475 units, the retreat
clicks at the windup end" lines). ERRATUM (the clean-context QA
audit of the round caught it): the PASS-THROUGH tier NEVER FIRED
live - zero "shoving through" lines across rounds 14 and 15 while
19 surround holds remained in round 15 - the tier is unit-pinned
(kite_breakout_test.go) but live-unvalidated; the 21->22 gain rode
the strafe alone (temp20/22). The remaining
fails: temp21 a spent window (the starter death-restart cycle on
the surround cell - a survivability question, not a kite behavior),
temp23/24 max-range (134/141 medians) and temp23 quick-reshot.

### The measured mechanism the next round owns (the P0)

THE RACE-EQUIILIBRIUM: the walk window equals the shot cycle (the
C1 disable ~2.97 s), so the walk ends exactly when the server's
auto re-shot is ready - the character stops at the 400-unit
endpoint, the auto-shot fires within ~400 ms, and the rhythm
re-shoots at ~141 units EVERY CYCLE (temp24: 43 shots, 400 ms
walk-end gaps, the median stuck at 141). Each cycle nets ~-20
units: the 1.65 s windup standstill hands the chaser ~180 units
back while the 1.3 s walk tail buys ~160. The pursuit continuation
chain DOES break the equilibrium when the attack stance drops
during the walk (temp23: 12.1 s walk-end-to-shot gaps - the honest
race to the 480 floor), but the chain loses the race whenever the
stance survives to the endpoint.

THE FIX, DESIGNED AND DEFERRED (the round-15 attempt taught the
shape): extending the walk WINDOW alone changes nothing - the walk
ends at its ENDPOINT regardless, and the character stops there.
The race needs a LEASH-AWARE RACE-LENGTH ENDPOINT: when the threat
holds under kiteReshotFloor at the issue moment, the step length
becomes min(kiteStep + (floor - dist) * raceFactor, raceStepMax,
the remaining anchor-leash budget) - ONE long continuous walk (the
moving character displaces the auto-shot the whole way) with the
window scaled to the step (step/kiteStep x the walk window,
kiteIssueWalk seam) and the re-click ladder keeping it alive
through the silent drops. raceFactor ~6-8 approximates the elven
parity margin (a 110 chaser on a 125 runner nets ~15/s - the raw
opening must buy the chase back). The unit-scene cost: every
endpoint-pinning test under the floor (mob at 200-260) sees the
race length - budget the test rewrites before the code.

The quick-reshot verdict ALSO needs the race re-scoping: the 1.5 s
walk-end-to-shot gap ceiling reads the honest race (12 s of walking
to the floor) as a fail - the metric should read "the re-shot
lands at the max distance" (the post-walk distance >= the fight
dist floor), the gap alone was the walk-ends-near-the-floor era's
proxy.

The round-15 erratum: the temp21 spent window is the STARTER DEATH
CYCLE (the level 7 kit dies on the shooter/surround cells mid-dress
or mid-duel, the village restart eats the window) - the fleet
scenario's entry latch tolerates it by design (the evidence floors
name it "no evidence"), but TWO consecutive rounds lost the same
slot to it. The next round should either raise the fleet kit (the
death is a survivability fact, not a kite fact) or drop a
healing-potion cadence into the acceptance reset.

## The clean-context QA round (the standing owner rule)

The QA sub-agent audited the session against the original owner
prompt on a clean context and scored it 74/100 with five
must-fixes; all five landed in the QA follow-up commit:

1. THE DOC MISREPRESENTATION: "the pass-through visible in the
   log" was half false (see the erratum above) - corrected here.
2. THE PHANTOM TEST: equip_test.go cited
   TestInventoryResendOutlivesTheAttackDeferral without the test
   existing - the test now exists and pins the 4 s same-item guard
   against the 600 ms slot-overlap release.
3. THE STALE COMMENTS: the file header, the curve doc and the
   kiteStreak field doc still described the removed "fight it out"
   and the zone-center curve - all three rewritten to the
   always-run contract (a future agent reading them would have
   re-introduced the standing behavior).
4. THE VACUOUS ALWAYS-RUN PASS: the verdict's evidence floor now
   requires the shooting evidence (fleetMinShots) - a slot that
   never fought reads "no evidence", not a pass.
5. THE SHOVE'S POST-DEFLECTION FLANK READ: the camp deflection can
   bend the shove's endpoint onto a chaser ray - the clearance
   verdicts now read the RESOLVED lane's direction (the same
   double read the breakout tier runs), and the ledger tick moved
   to the top of the fight tick (the placement bias: samples
   skipped behind the ladder returns under-counted the idle
   bucket).

The QA verdict also named the god-file growth (kite.go 2154 ->
2375 against the 500-line owner rule) - the honest debt the next
refactor round owns: the escape ladder family (the fan, the
breakout, the shove) is a clean split candidate.
