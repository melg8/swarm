# Kite walk re-click ladder: the dead retreat click recovered (status: in review)

Started 2026-09-22, branch feature/kite-walk-reclick-ladder, issue #60
(the second round - the follow-up feedback after PR #64 merged).

## Goal

The owner feedback on the merged shot-paced retreat: "Its slightly
better, it tries to move, but ... timing of click is wrong - so it
doesnt cover enough distance, dont start running or amount of clicks
needed to get running is not enough idk. Its like 3 seconds to draw
shot, but i can manually start clicking behind character and it runs
2 seconds no problem, covering large distance, and bot can replicate
it right now."

The follow-up dump proves the rhythm trigger works (a "shot released
... kiting the reload" line every cycle) while the WALK barely
executes: 22:09:11 the mob holds 393 units, 22:09:14 it holds 16 -
the mob covered ~380 units in one cycle, the character covered ~0 of
its 400 unit retreat ("moving: no" through every reload). Done means:
the retreat click survives its transport - the ladder re-issues the
click the way the owner's manual clicking does, the dead endpoint
rotates onto the next fan candidate once the probe names it, and the
next live dump reads the probe/rotation lines to name which mechanism
ate the clicks. Acceptance: the unit suite of the ladder (the
re-click pacing, the probe and rotation, the bound, the stand-down
paths) plus the lint/format gates; the live verdict lands through the
next acceptance dump on the deployment stack.

## Context

- Issue: https://github.com/melg8/swarm/issues/60 (the follow-up
  dump, the manual-clicking evidence). Project board claim: Agent
  vgkehKoy.
- The resume point the first-round agent left (the 19:25 comment):
  the re-click ladder, the move-start watchdog probe on the kite
  walk, the fan fallback on the destination-cell refusal - this
  round implements exactly that plan.
- The three candidate mechanisms, all named in the tree already:
  the stance transition swallow (the click ~250ms after the own
  Attack broadcast, the server side ATTACK intention still tearing
  down), the destination-cell refusal (the C1 MoveToLocation
  handler refuses a cell the bot side LineOfSight blessed) and the
  H-006 silent drop (the deployment that swallows accepted move
  requests).
- Dead ends ruled out:
  - Pausing the forced attack re-request LONGER (widening the
    window so the walk covers more): the window length is not the
    problem - the walk never started at all; the ladder fixes the
    start, not the length.
  - The cursor key escape as the kite transport: the escape is the
    town walk's last-resort machinery (a claimed position stream),
    far too heavy for a per-shot-cycle retreat; the mouse click
    re-issues the owner demonstrated are the right tool.
  - The SetupGauge parse (timing the re-click to the exact cooldown
    window): the gauge packet is not parsed today and the 600ms
    pacing against the SelfWalking oracle already replicates the
    manual behavior; noted as the possible tuning follow-up, not
    this round.

## Progress

### 2026-09-22 20:15 UTC - the ladder core

- hunt: kiteIssueWalk (both kite layers issue their walks through
  the one seam - the endpoint, the issue cell and the window land
  in the ladder state), kiteReclickWalk (the paced re-click ladder:
  the same endpoint first, the probe + the fan rotation at the
  second re-click, the rotated lane retried last), kiteWalkClear,
  kiteRetreatLaneSkipping (the lane battery minus the dead endpoint
  - the rotation machinery).
- hunt: the constants kiteReclickPeriod (600ms), kiteReclickLimit
  (3), kiteReclickProbe (2); the loop state block (the endpoint,
  the window, the base cell, the pacing stamp, the click count, the
  probe latch); the ladder ticks in engage() ahead of the fight
  flag branch so it runs whether the server still holds the
  fighting stance or the walk already tore it down.

### 2026-09-22 20:30 UTC - the tests and the docs

- hunt: kite_reclick_test.go pins the ladder (the dead click
  re-issued at the pacing, the same endpoint first, the probe latch
  and the fan rotation at the probe ordinal, the rotated lane
  retried, the click bound, the walk-runs stand-down, the
  base-cell stand-down, the window expiry, the cornered rotation
  keeping the dead endpoint, the proximity seam, the fresh pacing
  gate).
- docs: the re-click ladder section in docs/hunting.md (the
  mechanisms, the ladder rules, the dump markers and what each
  names), the constants table rows, the H-006 "Relied on by"
  extension in AGENTS.md (the ladder rides the same hypothesis from
  the fight side).
- Verification: go build ./... clean, go vet clean, the full hunt
  package suite ok (81s), the state package suite ok, golangci-lint
  0 findings, gofmt-spaces clean on the touched files.

## Status

The branch carries the complete round: the ladder, the probe, the
rotation, the tests, the docs. Next: the PR, the rebase to main, the
issue update (the findings summary for the reviewer) and the board
move to Ready for review. The live verdict (which mechanism the
deployment actually serves) lands in the NEXT acceptance dump - the
probe/rotation lines are the instrumentation for it.
