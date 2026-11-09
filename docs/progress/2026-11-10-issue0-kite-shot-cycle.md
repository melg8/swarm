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

## Status

Code and tests written; the full suites and the lint gate are green.
Next: the atomic commits, the 5-minute live confirmation with the new
acceptance code, then the fragment archive.
