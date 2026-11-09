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

## Live round 2026-11-09 13:13 (the temp18 acceptance dump)

The dump came from the `archer-kite` acceptance scenario (bot
`temp18`, `kind: acceptance`, PID 5168), build abde9178 - the fleet
bots test1-test3 in the same process stayed healthy. Two independent
problems show in the log
(`logs/acceptance/archer-kite-20261109-131027-5168-3.log`).

### The acceptance runner leaks one hunt loop per relogin

`runSession` starts `go loop.Run(ctx)` on the SUPERVISOR context
(`internal/swarm/acceptance/runner.go`, the line after
`wireSessionLoop`), and `runSessionSupervised` reconnects with that
same context. Every emergency logout/relogin therefore leaves the
previous loop ticking against the closed `GameClient`.

Evidence: 226 `use of closed network connection` lines, every
decision printed twice after the 13:11:49 relogin (`target ... does
not engage` and `is on us` pairs at 13:12:19/13:12:20 and later),
two independent pursuit counters interleaved (`step 33 of 12` next
to `step 1 of 12`, `step 93 of 12` later), and the same target
declared dead at 13:11:49 and shot at two seconds later. The
stale loops also overwrite the shared tracker diagnostics, so the
dump mixes two loops' views (the target hp and the 0-damage swing
of the dump are not trustworthy).

Fix: per-session `context.WithCancel` plus a wait for the loop
goroutine, exactly the `runBot` pattern of `cmd/swarm/main.go`.

### The archer kite starves the bow in the walled pocket

The level-7 archer (speed 144) fights a level-7 Kaboo Orc Grunt
(speed 110) on the elven ground. The re-shot gate
(`kiteReshotFloor = 480`) assumes the retreat can open 480 units
before the next windup; the walled hemisphere and the repeated
`clearance lane` breaks keep the achieved gap at 300-450, so the
bot walks for a minute at a time and fires only when the cornered
hold answers.

Evidence: 600 arrows at 13:10:31, 594 at the 13:13:08 dump, about
20 shots in the first 5.5 minutes against a ~3 s bow cycle; no shot
between 13:12:05 and 13:13:08 (63 s of pursuit steps); `the pursuit
race ... at parity (2 stalled windows)` repeatedly; 8 walled
hemisphere breaks; `12 walks without a shot` crossed and continued
by design (the round-14 always-run rule); the only damage in the
drought comes from the 13:13:09/13:13:12 cornered hold (20 + 51).

The 12 s `does not engage` timeout then drops the target (the
`FightingTargetID` freshness lapses during the shot drought) and
the aggro answer re-picks it, which is the 12-13 s `does not
engage` -> `is on us` churn (13:11:02, :14, :27, :39).

Fix direction: drive the cornered hold from the stall ledger (the
gap not growing for N windows AND the hemisphere walled), not only
from the full six-ray cone exhaustion; and keep the fight clock
anchored while the target is still attacking the character instead
of dropping it every 12 s.

### The fixes of this task did fire

`13:11:53 Kaboo Orc Grunt aggroed on the approach, answering it
first` and `13:09:43 no escape lane opens from the chase, resetting
the aggro via logout` are the new aggro answer and the ordered
escape working on the live round. The kills are slow and the
character still dies/logs out in that pocket (HP 57 %, the
relogin lands on the same cell and the same grunt re-aggros), so
the acceptance-runner leak and the kite drought stay open.
