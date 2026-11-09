# The aggro answer and the ordered escape (status: in progress)

Started 2026-11-09, branch feature/kite-range-contract, issue #0 (the
owner report, no board issue yet).

## Goal

The reported hunt failure: when no target is selected, or the fight
with the selected target has not started, the bot must switch to the
mob that aggroed on it instead of pressing the peaceful pick. The
escape from that mob must be ordered - one lane away from the chaser,
never a 180 degree flip through it - with kiting where the archetype
supports it, and a relogin (or a started fight) whenever the escape
cannot run.

Acceptance criteria:

- A picked target that has not started fighting is replaced by the
  nearest living attacker on the very next engage tick, whatever
  branch (approach, stuck timeout, blind recovery) the pick sat in.
- A fight that has not started is never "lost" by the percent health
  gap alone: only the critical health flees before the first swing.
- The escape runs the ordered fan (away ray, then 45 and 90 degree
  lanes each side), clamped along its own ray into the patrol square.
  No segment folds back toward the threat and none heads for the zone
  center through it.
- An escape with no open lane, or one that covers less than 120 units
  in 3 seconds, resets the aggro through the emergency logout.
- The loot phase yields to a live attacker (fight it when winnable,
  flee and relog when not).
- The round-19 archer kite stays intact: once the aggro answer starts
  the fight, the shot-paced retreat owns the motion.

Constraints: `temp*` accounts only for the live round, never the
persistent `test1`-`test3` characters; no `-abuse`; the bot process is
the only process started or stopped freely (the stack already runs).

## Context

Live evidence (`logs/session-20261109-115239-11232.jsonl`, test2 level
19 on `Crimson Spider SW-2`): a spider aggroed while the character
approached a pick, the bot adopted it at 87 percent health, then fled
at 56 percent five seconds later - the untouched target bar (100)
counted as a 44 point lead under the old `losingFight` percent rule.
The escape covered about 28 units over 30 seconds, zigzagged through
the chaser (the zone-center fallback of `escapeWalkDestination` points
at the camp focus whenever the away endpoint leaves the square), the
bar sank to 38 percent and the emergency logout fired; the relog died
at the same cell minutes later.

Root causes found in the code:

1. `losingFight` applied `targetHP-selfHP > 25` before the fight
   started; a fresh target always sits at 100 percent.
2. The approach aggro answer sat after the stuck-timeout,
   losing-fight and blind-recovery branches and was skipped whenever
   the selected pick also held the character as its target
   (`!ObjectTargetsSelf`), even with a different, nearer attacker.
3. `escapeWalkDestination` replaced any out-of-square away point with
   the zone center direction - the camp focus in cell mode - and had
   no stall guard, so the H-006 silent click drop looked like a
   running escape.
4. `loot` ignored living attackers until the drops were picked up.

## Progress

### 2026-11-09 - the aggro answer, the pre-fight rule, the ordered escape

- `engage` runs the aggro answer after the target-death block and
  before the stuck timeout, the losing-fight verdict and the blind
  recovery; the `ObjectTargetsSelf` gate is gone (the
  `attacker.ObjectID != l.target` guard stays - the pick itself is not
  a shuffle).
- `losingFight` needs `SelfFighting(target)` for the percent-gap rule;
  the beaten-target exception and the critical 25 percent escape stay
  in front of it.
- `escapeWalkDestination` is the ordered lane fan (0, +/-45, +/-90
  degrees over `rotatePlanar`), each ray clamped into the patrol
  square at `escapeZoneMargin`; the center fallback is gone.
  `escapeStalled` anchors a rolling 3 s / 120 unit window. No lane or
  a stalled window calls the emergency logout. The anchor clears with
  the flee episode (recovery, fresh fight, death restart, logout).
- `loot` hands the tick to the engage when `NearestAttacker` exists:
  winnable attackers become the target, the rest run the defense flow.
- Tests: `aggro_flee_test.go` (12 cases: the pick-holds-us switch, the
  stuck-order answer, the pre-fight vs running-fight vs critical
  verdicts, the corner lane, the no-fold-back invariant, the no-lane
  logout, the stall logout and its re-anchor, the loot interrupt and
  the loot defense). `TestLoopEscapesTheLevelGapFight` now stages the
  confirmed fight (the gap rule needs it by design).

## Status

In progress: the code and its unit tests are committed (the loot
interrupt, the aggro answer together with the ordered escape); the
full hunt suite (156 s) and the full `golangci-lint run ./...` are
green, `task fmt:check` passes. The live round on the running stack
with a `temp*` account is the remaining step. First live attempt
(12:18) burned the login server IP ban with a wrong password guess
(`temp10` already existed with password `temp`): the only failure was
the ban, no hunt evidence came out of it. The account was re-launched
with the right password and reconnects on the backoff until the
900 s ban lapses, then the journal watch starts.

## Next step

Watch `logs/session-*.jsonl` of the running `temp10` bot for the
`aggroed on the approach` line, an ordered escape (lanes at
0/±45/±90 degrees, no fold back through the chaser) and the loot
interrupt; then archive this fragment.
