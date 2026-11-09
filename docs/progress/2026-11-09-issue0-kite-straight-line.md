# The straight-line kite: shot, retreat, re-shoot (status: in progress)

Started 2026-11-09, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue).

## Goal

Replace the turning kite geometry with a straight-line retreat, per
the owner instruction: when the chaser speeds sit close to the
character's, a turn wastes the distance the retreat just bought.

- The retreat runs the straight away-ray of the chaser train and
  NOTHING else: no fan, no camp deflection, no circling chord, no
  pocket breakout, no shove/pursuit turns.
- The rhythm stays shot-paced: shoot, retreat through the reload,
  shoot again the moment the bow cooldown lapses.
- A blocked straight lane, a refused click or a silent endpoint holds
  the ground and keeps shooting - never a turn.
- The anti-archer gate stays: no kite against a bow-wielding enemy.
- Only `temp*` accounts for live checks; the persistent test1..testN
  characters stay untouched; the Mobius server stays running.
- No `-abuse` flag; untracked `runs/*` evidence is never committed;
  committer `melg8 <public.melg8@gmail.com>`.

## Context

- The predecessor round (archer kite skip) lives in
  `docs/progress/archive/2026-11-09-issue0-archer-kite-skip.md` and is
  pushed on this branch.
- The turning design came from issue #70 rounds 17-19: same-endpoint
  spam plus a ring chord and a pocket breakout. The straight rule
  removes the chord/fan/breakout/pursuit machinery and keeps only the
  straight lane battery (`kite_lane.go`), the re-click ladder
  (`kiteReclickWalk`) and the hold (`kiteHoldGround`).
- Deleted: `kite_escape.go` (the breakout/shove/pursuit helpers) and
  the `kite_curve_test.go` / `kite_breakout_test.go` /
  `kite_pursuit_test.go` suites.
- Renamed: `kiteCircleLeashShare` -> `kiteRingLeashShare`, the ring
  budget `kiteRoamRadius * kiteRingLeashShare` = 972 units around the
  fight anchor; the shortest hop is `kiteMinStep` (250).
- The full `task verify` still stops on the pre-existing Windows
  session-journal gzip close failure; `stash@{0}` carries that fix and
  must not be dropped. Verify the touched packages individually.

## Progress

### 2026-11-09 - the straight-lane rewrite of the hunt kite

- `kite.go`: the step direction is the straight away-ray (the
  centroid of a normal train, the widest-gap bisector of an encircled
  one); the leg stays race-aware and ring-budgeted; the re-click
  ladder latches the dead verdict at the probe age and holds the
  ground instead of rotating; `kiteHoldGround` keeps the
  cornered/encircled wording.
- `kite_lane.go`: straight-only lane battery (fight leash, away
  half-plane, geodata wall, water); the fan battery is gone.
- `loop.go`: the curve/pursuit/dead-cell fields and the
  `kitePursuitHold` call block removed; the streak limit is a
  diagnostic only.
- `kite_ledger.go`: the motion buckets describe the straight retreat
  and the re-click recovery.
- Tests: straight/hold edge cases, the refusal/silent hold without
  rotation, the race leg and ring budget, the shot rhythm; the
  obsolete curve/breakout/pursuit suites deleted.
- `docs/hunting.md`: the state machine, the fallbacks, the knobs, the
  re-click ladder, the race-leg section rewritten to the straight
  design.

Next: run the gates (`task fmt`, the hunt package tests,
`task lint`, `task prepush`), commit and push, then an optional short
`temp18` live re-check of the straight cycle.

## Status

The code and docs rewrite is complete in the working tree. Remaining:
the verification gates, the atomic commits, the push and the short
live check.
