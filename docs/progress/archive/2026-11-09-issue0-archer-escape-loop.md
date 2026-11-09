# Task: break the death-relogin escape loop of the archer hunt (status: in progress)

Started 2026-11-09, branch feature/kite-range-contract, issue #0
(the GitHub board item has no number yet).

## Goal

Fix the defects the 14:03:18 `temp18` dump and the full ten minute
`archer-kite` run exposed, without weakening the round-19 archer
kite behavior (a winnable retreat race still runs and shoots).

The observed failure class: a death (or an emergency logout counted
as one) regresses the hunter to a safer cell, the supervisor logs
out / reconnects with a fresh session, and the fresh `hunt.Loop`
resumes the deadly cell because the login landed on its ground.
The character walks back into the same pocket and dies again: 8
emergency logouts and 8 regressions in 10 minutes.

Acceptance criteria:

1. The death regression survives the session: a fresh loop refuses
   to resume the cell the character just died on and picks the
   global alternative instead (both the death path and the
   emergency logout path, which counts as a death through
   `cellNoteDeath`).
2. `panicPileUpRun` reports "no escape lane" honestly when a
   threat still exists instead of claiming the pack dissolved.
3. The 12 s engage gate does not drop a target that is actively
   attacking the character while the bow reuse window refuses the
   character's own attack requests.
4. The impending-add sidestep passes the same terrain gates as the
   kite lanes (zone leash, geodata line of sight, water) and holds
   the fight when no lane is clear instead of clicking into a wall.
5. A flee whose chaser keeps pace ends the session on the closure
   rate instead of burning the full 20 s escape budget.
6. The kite re-click ladder does not attribute the server's
   bow-disable auto-attack `ActionFailed` to a healthy retreat
   click.

Constraints: only `temp*` accounts, no `-abuse` flag, the server
stays running, persistent `test*` characters are untouched, the
untracked `runs/*` evidence files stay uncommitted.

## Context

Evidence:

- Failed run: `logs/acceptance/archer-kite-20261109-140204-25460-1.log`
  (started 14:02:04, FAILED 4/6 after 10m6s; range 43% ranged /
  max 900 units, survives broke; 8 emergency logouts, 8 death
  regressions).
- Passing reference: `logs/acceptance/archer-kite-20261109-135558-26360-1.log`
  (2 minutes, PASSED 6/6, stall exits at +52.3 s and +92.5 s,
  shots 0.25 s later, 0 closed-connection lines).
- The dump `temp18` 14:03:18: HP 188/214, in combat, target
  268439305 at 380 units, 595 arrows, exp 11287, adena 0, walk
  plan none.

Verified facts from the log:

- Every `regressing from ... after the death` is overridden by
  `resuming the cell ... - the login landed on its ground`
  (`cell_policy.go`), because `runBot` builds a new `hunt.Loop`
  per session.
- 14:02:41: `2 mobs piled on us, running 600 units...` then
  `the pile up run shook the chase at 0 units, logging out` - no
  run at all; relogin at 14:02:43 landed in the same pack.
  Compare 14:04:26: 3 mobs, a real 685 unit run, clean logout.
- 12 s engage gate: last shot 14:02:25, `target 268439305 does
  not engage` at 14:02:40 while the grunt chased/attacked at
  17-34 units (8 occurrences).
- Add sidestep: 14:03:16 `dest=(35534,50426)` -> `ActionFailed`,
  no self move echo, position unchanged (dump `moving: no`,
  `inside: false` at x 35366 vs west edge 35367); same at
  14:08:24.
- Flee: 14:03:09 `fleeing for 21s without shaking the chase`
  against a level-10 Kaboo Orc Fighter holding ~19 units;
  `escapeStalled` measures only own displacement.
- False refusal: 14:06:50.263 retreat click + `ActionFailed`,
  re-click at .513, server accepted at .514 (`object 268463395
  moves to (35012,48435)`). The failure is the server's
  bow-disable attack retry (`Creature.canUseRangeWeapon`
  schedules `notifyActionReadyToAct` 1 s ahead and answers
  `ActionFailed` each time the bow is still disabled), not the
  click.
- The temporary geodata probe (since deleted) showed all refused
  destinations clear in the bot pack (`blockedAtZ=false`, floor
  layer clear, `los=true`): the server, not the shipped pack,
  refuses those cells, so the refusal evidence channel stays the
  online `ActionFailed` correlation.

## Progress

### 2026-11-09 - task start

Fragment created; no code changes yet. The fix order: (1) persist
the death regression and gate the resume; (2) honest pile-up
no-lane answer; (3) engage gate fed by incoming attacks; (4)
terrain-gated add sidestep; (5) chaser closure-rate flee exit;
(6) bow-retry refusal misattribution. Every fix lands with a unit
test, then the full verify loop and a short live `temp18`
archer-kite re-check.

### 2026-11-09 - hunt: keep the death regression across the session

- `state.Bot` carries the last deadly cell (`SetDeathRegression` /
  `DeathRegression`, a tracker field next to the danger spots) and
  `cellNoteDeath` records the cell there, so the record survives the
  supervisor rebuilding the loop on every relogin.
- `cellEvaluate` refuses the standing-ground resume of a cell whose
  record is younger than `cellDeathRegressionWindow` (15 minutes,
  half the heat half-life) and falls through to the global contest
  excluding that cell (a registry with no alternative still takes
  it).
- `followGround` refuses the same ground: the fresh login often
  stands inside the deadly cell, and following it would undo the
  regression on the next tick.
- Tests: `TestDeathRegressionSpansSessions`,
  `TestCellResumeRefusesTheDeathRegression`,
  `TestCellResumeKeepsTheStandingGround`.

### 2026-11-09 - hunt: name the walled pile up lane

- `panicPileUpRun` mirrors `fleeFromThreat` when
  `escapeWalkDestination` answers false: a live threat turns the
  line into "no escape lane opens from the pile up", the no-threat
  case keeps the pack-dissolved wording. The 14:02:41 dump claimed
  "shook the chase at 0 units" while two mobs still held the
  character.
- Test: `TestLoopPileUpNamesTheWalledLane` (a tiny leash square
  walls every lane; the cornered logout still fires, the log names
  the wall).

### 2026-11-09 - hunt: count an incoming attack as an engagement

- `state.Bot.SelfEngagedBy` answers the two-sided engagement: the
  own fresh swing/chase OR the target holding the character as its
  own target (`SelfFighting` alone missed the second side, which is
  exactly the bow reuse window case).
- The 12 s engage stuck gate uses it, so a mob that is swinging at
  the character keeps its target while `ActionFailed` answers the
  character's own attack requests; an idle selection still drops.
- Tests: `TestSelfEngagedByCountsTheIncomingAttack`,
  `TestEngageGateKeepsTheAttackingTarget`,
  `TestEngageGateDropsTheIdleSelection`.

### 2026-11-09 - hunt: gate the add sidestep through the terrain

- `avoidImpendingAdd` fans the sidestep: the straight away ray
  first, then the 30 and 60 degree flanking rays each side.
- `combatAvoidLane` resolves one candidate through the kite contract
  (zone leash, degenerate length, geodata line of sight, water):
  a walled, wet or out-of-leash lane never issues its click.
- With every lane refused the fight holds the ground (no click into
  the wall, the add scan retries at the step pace) and the paced
  diagnostic names the walled pocket. The 14:03:16 dump clicked the
  walled straight lane and froze; the add stayed on top.
- Tests: `TestFightStepFansAroundTheWalledLane`,
  `TestFightStepHoldsWhenEveryLaneIsWalled`.

### 2026-11-09 - hunt: end the flee on a chaser that keeps pace

- `fleeFromThreat` arms a closure-rate probe with the episode
  (`fleeGapAt`/`fleeGap`): after `fleeGapWindow` (8 s) the threat
  distance must have widened by `fleeGapOpen` (150 units), otherwise
  `escapeNotOpening` names the parity-speed chase and the session
  logs out early instead of burning the 20 s budget.
- The stalled watch measures the character's own travel, the closure
  watch measures the gap - a chaser running at the character's speed
  defeats the first and is caught by the second.
- `clearEscapeAnchor` ends both observations with the episode; the
  stagnation soft reset clears them too.
- Tests: `TestFleeLogsOutWhenTheChaserKeepsPace`,
  `TestEscapeClosureWatchdogReanchorsOnAnOpeningGap`.

### 2026-11-09 - hunt: keep the bow retry out of the kite verdict

- `kiteClickRefused` adds the retry look-back band
  (`kiteBowRetryLead`, 1350 ms): an action request sent inside it owns
  the `ActionFailed`, because the server schedules
  `notifyActionReadyToAct` one second after a bounced attack request
  (`Creature.canUseRangeWeapon`) and the retry's refusal lands far
  outside the tight click window.
- Evidence: the 14:06:50 dump of the failed run - attack request at
  49.013, `ActionFailed` at 50.263, the retreat click of that moment
  logged as "refused by the server" and a fan candidate burned; the
  same click was accepted on the re-issue 250 ms later.
- Tests: `TestKiteBowRetryRefusalIsNotAttributedToTheClick`,
  `TestKiteRetreatClickRefusalStillLatches`.

### 2026-11-09 - the lint round of the six fixes

- `combatAvoidFanStep` carries the sidestep fan loop, so
  `avoidImpendingAdd` stays under the cyclop limit; the umbra
  `rotatePlanar` answer splits into locals (a two-value return cannot
  feed a multi-argument call).
- `dropPileUpFight` carries the target drop of `panicPileUpRun`
  (skip list, cleared engage bookkeeping), so the function stays
  under the funlen limit without a suppression.
- `TestEscapeClosureWatchdogReanchorsOnAnOpeningGap` compares the
  re-anchored gap with `require.InDelta` (the testify float rule).
- Gate: `golangci-lint run ./internal/swarm/hunt/...
  ./internal/swarm/state/...` 0 issues; the focused
  fight/flee/escape/kite suite passes (66 s).

## Live round 2026-11-09 15:13 (the six-fix re-check)

The archer-kite scenario re-ran on the fixed build (`commit
84cf2adc`, temp18, 3 minute window, log
`logs/acceptance/archer-kite-20261109-151337-30468-1.log`):

- **Verdict FAILED 5 of 6 checks after 3m6s** - armed, retreats,
  survives and quiver pass; the range check fails (83 samples, 22%
  ranged, max 1281 units).
- **The survival side is green**: zero `emergency logout`, zero
  `regressing`, zero `piled on us` and zero `fleeing` lines in the
  window (the 14:02 run had 8 emergency logouts and 8 death
  regressions in 10 minutes), min HP 69 percent, 9 kite steps and 1
  cornered hold.
- **The engage gate held**: zero `does not engage` drops in the
  window (the failed run had 8, on a target that was chasing and
  hitting the character), and zero bow-retry refusal attributions.
- **The sidestep fan ran live**: three `stepping clear` lines
  (15:15:02, 15:15:08, 15:15:13) with no wall freeze; the click of
  15:15:13 went silent (the H-006 accepted-but-moved-nothing form)
  and the next walk re-entered the loop 3 s later.
- **The range failure is not one of the six defects.** The 1281
  unit maximum is an engage-approach artifact: at 15:15:00 the loop
  picked a new target 1340 units away after the kill, the auto
  attack stance of the previous fight was still on, and the monitor
  folded the cross-cell approach (`MoveToPawn` chase at 500
  distance) as a fight sample. The low ranged share is the walled
  pocket at the cell edge (15:14:36: `no walkable retreat lane on
  target 268439335 (cornered) ... 6 walled or dead`), where the
  chaser at parity keeps the fight at 12-150 units for a minute.
  The 14:02 run of the old build already failed the same check
  (43% ranged, max 900), so the contract weakness pre-dates this
  round: the ground's chaser speed and the leash wall make the
  ranged majority unattainable once the pack aggros; the 13:55
  pass was a two minute window whose first fight landed
  unaggroed. The next round owns it (the sample definition and
  the parity/walled-pocket kite), not this fix set.

## Status

Done: the six fixes landed, lint clean and unit pinned; the live
re-check confirms the survival side (no emergency logout, no death
regression) and leaves the pre-existing range contract failure to
the next round.

