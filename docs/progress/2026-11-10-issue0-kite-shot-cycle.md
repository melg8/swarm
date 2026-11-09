# The archer kite shot cycle: one shot per bow cooldown (status: in progress)

Started 2026-11-10, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue). Follow-up of
`docs/progress/archive/2026-11-09-issue0-kite-survival-gate.md`.

## Goal

Make the bow archer's fight ideal on the live Mobius C1 server: the
shot broadcast starts the retreat, the reload tail is spent walking,
and the forced attack at the bow disable end fires the next shot at
whatever range the chaser reached meanwhile.

- The owner rule: no kiting against bow-wielding enemies (the
  anti-archer gate stays); the kite runs straight only - no fan, no
  turn, no circling.
- The owner rule: shoot, retreat as soon as the arrow is out, and
  shoot again the moment the bow is ready. The retreat must start
  IMMEDIATELY after the broadcast, never wait the cooldown standing
  at the collapsed range.
- The owner rule: keep retreating even after the chasers reach melee
  between the shots, so every shot buys the reload tail of distance
  and the mob spends it re-closing.
- Question the code and the tests: the round re-reads the timing
  against the Mobius C1 sources and re-verifies on the live stack
  until the 5-minute `archer-kite` acceptance passes every check.
- The acceptance scenario must judge the shot-cycle contract: the
  shot cadence, the standing band, the retreats, the resolved
  fights, the survival and the quiver - the old ranged-majority
  verdict is obsolete by design.

## Context

- The server facts the cycle rides: H-007 in AGENTS.md (the Attack
  broadcast is the shot commit), `docs/kite_timing_findings.md` (the
  windup end at ~1.5 s, the disable end at ~3 s) and the Mobius C1
  `PlayerAI.setIntentionMoveTo` save/replay (a click inside the
  windup is replayed at the disable end, where the re-shot cancels
  it).
- The live run of 2026-11-09 23:15 (5 minutes, temp18) proved the
  central defect of the previous shape: the re-shot floor
  (`kiteReshotFloor` 480) plus the race leg (`kiteStepLength`, the
  parity rate, `kiteRaceStepMax` 1700) walked 20-30 s without
  shooting against an equal-speed chaser; the mob regenerated faster
  than the arrows, the fight never resolved and the drift carried the
  session ~20,000 units into a level-15 zone.
- The live run of 2026-11-09 23:57 (`logs/acceptance/archer-kite-
  20261109-235745-33876-1.log`, the shot-cycle code with the OLD
  acceptance verdict): 44 shots, 8 kills, 38 shot-cycle retreats,
  110 fight samples with the farthest STANDING fight at 510 units,
  min HP 74 percent, min arrows 542, no death. The same-fight shot
  gaps read 3.0-3.25 s (35 of 36 inside the 4 s cooldown; one 4.5 s
  re-engage). The old verdict failed only the obsolete ranged share
  (33 percent): the shot-cycle contract deliberately fights at melee
  between the shots, so a ranged-majority demand now contradicts the
  owner rule.
- The earlier run of 2026-11-09 11:19 (`runs/acceptance-r19-kite.log`,
  the old shape): 10 shots, 0 kills, 13 kite steps - the cadence and
  kills verdicts would fail it, which is the discrimination the new
  checks must keep.

## Decisions

- The shot-per-cycle contract: the shot broadcast arms the DEFERRED
  retreat click (`kiteShotPhase`, the windup end plus
  `kiteWindupLead` 150 ms), the walk owns the reuse tail and ends at
  the bow disable end (`kiteWalkWindow`, the fresh RED SetupGauge or
  the formula fallback), and the forced attack at that end IS the
  next shot. No re-shot floor, no race-length leg, no pursuit hold
  or ledger, no `kiteDeferShot`.
- The leg is the plain `kite.Step` (400), ring-budgeted
  (`kiteStraightLeg`); the walk is cut by the next shot, so a longer
  leg only names a line the character never finishes.
- The acceptance rework: a cadence verdict (the same-fight shot gaps
  inside the 4 s cooldown on at least three quarters of the evidence,
  floor 6 gaps), the band verdict on STANDING samples only (the
  retreat legs legitimately open wider), a kill floor of one per two
  minutes (the shot cycle exists to resolve fights), and the
  shot-cycle and kill log counters. The ranged share is removed from
  the verdicts.
- `archerKiteLog` counts the shot-cycle retreat lines ("kiting the
  reload") and the kill lines ("died, looting") beside the step and
  hold counters; the fleet scenario keeps reading the first two.

## Acceptance criteria

- The full `internal/swarm/hunt`, `internal/swarm/state` and
  `internal/swarm/acceptance` suites, the build, `task lint` and
  `task fmt:check` stay green.
- A 5-minute live `archer-kite` run passes all eight checks with the
  new code (cadence, band, retreats, kills included).
- The live log shows the shot -> windup-end retreat -> reload-tail
  walk -> shot cadence with no standing shot at the collapsed range,
  and the run resolves kills instead of the treadmill.

## Constraints

- Only `temp*` accounts for live checks; persistent test1..testN
  untouched; no `-abuse`; the Mobius server stays running; untracked
  `runs/*` evidence never committed.
- Committer `melg8 <public.melg8@gmail.com>`.

## Progress

### 2026-11-10 - the shot-cycle code and the acceptance rework

- Removed the safe-distance machinery from the kite (`kiteStepLength`,
  the re-shot floor, the race factor, the pursuit hold and ledger,
  `kiteDeferShot`); the forced attack at the disable end fires the
  next shot at any range.
- The walk window is the bow disable window, not scaled by the leg;
  the deferred click rides the accepted window past the windup.
- The acceptance scenario gained the cadence and kills verdicts, the
  standing-only band, the shot-cycle and kill log counters, and lost
  the ranged-majority half; the scenario description and the docs
  telemetry anchors describe the new contract.
- `internal/swarm/state/scans.go` split the social candidate filter
  out of the constrained search (the committed bow fence pushed the
  function over the cyclop cap; the full lint gate is red without
  the split).

### 2026-11-10 - the bow add survival answer (run 21)

- Run 21 failed only the `survives` check: a level 8 Kaboo Orc Archer
  joined a nearly-dead melee target at +257 s, the shot cycle kept
  firing (the nearly-dead exemption returned before the health gate),
  the health fell 68 -> 10 percent and the critical-HP emergency
  logout ended the session at +275 s.
- The losing-fight verdict now drops the fight the moment a
  bow-wielding add holds the character as its target
  (`state.HasBowAttackerBesides`), before the nearly-dead exemption:
  a ranged add outranges the straight retreat, so its reload-tail
  fire is free damage the shot cycle cannot answer.
- The targetless flight after the drop is protected from the pile up
  gate (`kiteFlightOwnsEscape`): while the bow is in hand and the
  nearest chaser is melee, the straight flight owns the escape and
  the zoned fan with its emergency logout stands down. The
  critical-HP logout (< 12 percent) stays armed above both.

## Status

Code and tests written; the full suites and the lint gate are green.
Next: the atomic commits, the 5-minute live confirmation with the new
acceptance code, then the fragment archive.

### 2026-11-10 - run 22: the pack focus and the pile-up flight

- Run 22 (5 minutes, build d8bc1f71) failed `kills` and `survives`:
  the character joined a social grunt pack, three fresh adds took
  turns stealing the pick through the aggro-answer switch (the strict
  `SelfFighting` view goes stale during every reload tail), the
  damage spread across the pack while each mob out-regenerated it,
  and the window resolved no kill in 131 seconds. The four-mob pack
  then ground the health from 64 to 16 percent while the standing
  kite held and the session relogged at the death.
- The live kite fight now keeps its pick: the aggro-answer switch is
  fenced off while the bow owns the fight and the pick is engaged by
  either side. A peaceful pick on the approach is still given up to
  the nearer attacker.
- The unwinnable pile-up converts the standing kite into the
  straight flight (`dropFightForFlight`): the shot cycle stands half
  of every reload inside the melee range, so a pack the arrows cannot
  out-damage is dropped and flown instead of held. The critical-HP
  logout stays armed above it.
- Run 23 (5 minutes, build 606055ca) PASSED: 60 shots, 50 of 50 shot
  gaps inside the 4 s cooldown, 55 kite steps (47 shot-cycle), 9
  kills, 107 fight samples, min HP 25 percent, min 540 arrows, online
  the whole window. The run carried the run-21 scene again: a level 8
  Kaboo Orc Archer joined at +117 s, the losing verdict dropped the
  grunt and the straight flight answered (29 + 63 + 63 damage taken
  before the archer fell out of range, no logout).
- The flight log showed the drop/adopt ping-pong: the targetless
  aggro answer re-adopted the fled grunt every half second (it
  ignored the flee skip) while the archer was still firing, so the
  character turned back into the archer range until the next drop.
  The targetless answer now skips a just-fled attacker and keeps the
  flight while a bow add is present.

### 2026-11-10 - run 24: the pocket logout and the deferred flight click

- Run 24 (5 minutes, build 6b74bc6c) failed `survives` with two
  escape-stall logouts, both from the flight falling through to the
  zone-clamped melee fan:
  - +84 s: a level 10 Kaboo Orc Fighter joined a 24 percent grunt,
    the pile up gate dropped the fight at 73 percent health, the
    flight ran ~30 s north-west to the pocket at (33480, 53016),
    the away half-plane was walled, the hold returned false and the
    fan logged "no escape lane opens from the chase" at 99 percent
    health.
  - +131 s: the flight click was saved by the bow windup, replayed
    2.7 s later, and the 3 s `escapeStalled` watchdog reset the aggro
    at 99 percent health.
- The bow character no longer falls back to the zone-clamped fan
  (`kiteLayerArmed` guard of `fleeFromThreat`): a refused flight tick
  is answered by the hunt's standing trade, a walled lane is held by
  the flight itself, and only the critical-HP logout (< 12 percent)
  can still reset the session.
- The walled hold keeps the tick (`kiteFleeThreat` returns true for
  the hold period) and the away half-plane gets a breakout attempt
  through the open back half-plane - the ground behind the character,
  through the chaser's bearing - before the full pocket holds.
- The flight waits out the bow windup before its first click
  (`kiteFlightMovable`): the server saves a move issued inside the
  windup and replays it at the disable end, so the stall watchdog
  read the saved seconds as a dead click.
- The pile up gate keeps a fight whose target is inside the kill
  window (`pileUpKillWindowPercent` 30): one or two arrows finish the
  kill the fight already paid for instead of throwing it away into a
  flight (the run-24 24 percent grunt).
- Unit tests: `TestKiteFleeThreatHoldsTheWalledLane`,
  `TestKiteFleeThreatBreaksOutOfThePocket`,
  `TestKiteFleeThreatWaitsTheBowWindup`,
  `TestKiteFleeThreatIgnoresAStaleGauge`,
  `TestKiteFleeThreatLeavesTheBowAttackerToTheHunt`,
  `TestKitePileUpFinishesTheKillWindow`.
- The full `internal/swarm/hunt` and `internal/swarm/state` suites
  pass, and `golangci-lint run --new` reports zero issues.
- Run 25 (5 minutes, the fix set above) PASSED: 47 shots, 44 of 47
  shot gaps inside the 4 s cooldown (94 percent), 63 kite steps (28
  shot-cycle), 0 holds, 12 kills, 91 standing fight samples with the
  farthest at 533 units, min HP 66 percent, min 554 arrows, online
  the whole window, no escape-stall logout. The log carried a
  two-minute continuous kite against the level 10 Kaboo Orc Fighter
  (the +73 s to +186 s stretch, 15-30 steps of 700 units) without a
  single hold or reset.
- Next: the `docs/hunting.md` update (the bow add rule, the pile-up
  conversion and its kill window, the windup-wait flight, the
  breakout and the fan removal) and the fragment archive.
