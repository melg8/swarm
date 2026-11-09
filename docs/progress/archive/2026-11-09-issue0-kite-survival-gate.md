# The archer kite survival gate: disengage below the escape line (status: complete)

Started 2026-11-09, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue). Follow-up of
`docs/progress/archive/2026-11-09-issue0-kite-continuous-retreat.md`.

## Goal

Close the survival gaps the 5-minute live confirmation of the
continuous-retreat round exposed: the archer must disengage and fly
when the running fight turns into a death risk, the acceptance band
verdict must judge standing fights instead of retreat legs, and the
bow profile must not initiate the ranged trade the owner rule already
refuses to kite.

- The `losingFight` gate (self HP under `escapeHealthPercent`, 25,
  or a 25 percent health lead of the target) must no longer be
  suppressed for the kite: at that line the fight is dropped and
  the character flies.
- The flight is the straight `kiteFleeThreat` (a targetless fleeing
  character has no fight cycle), not the zone-clamped melee fan -
  that is why the old suppression existed and why it is stale now.
- The critical-HP logout (< 12 percent) and the pile-up run stay
  armed independently.
- The acceptance range verdict latches the bow band on STANDING
  samples only: a retreat leg legitimately opens the gap past the
  band while the chaser lags.
- The bow profile never INITIATES a fight with a bow-wielding mob
  (`avoidBowTargets` / the `avoidBows` fence of the constrained
  target search): the standing trade is a break-even race that turns
  lethal with a melee add, while an attacking archer is still
  answered through the attacker scan.

## The evidence

Run 7 of the round (5 minutes, `SWARM_ARCHER_KITE_MINUTES=5`,
temp18, Kaboo Orc Grunt SW-5 -> drifted into Kaboo Orc Fighter
SW-11): 61 fight samples, 75 percent ranged, 10 kite steps, 0
holds, min HP 10 percent, min arrows 591, then a death at +223s
inside a two-mob melee pile-up (the Kaboo Orc Fighter Leader and a
second Fighter). The samples read 100 percent at +200s, 69 percent
at +210s, 32 percent at +220s, 21 percent at +222.8s, death at
+223.0s. The kite kept walking the pursuit chain while the chasers
glued at 14-38 units, and no disengage gate fired: `losingFight`
was true but `!kiteOwnsRetreat()` skipped it, and the critical-HP
logout (12 percent) never saw a tick under its threshold. The
acceptance verdict failed the `survives` check (the online status
dropped inside the window).

Run 8 (5 minutes, after the disengage fix): survives green (min HP
78 percent), retreats green (17 steps, 0 holds), quiver green (587
arrows), ranged share 95 percent, but the range check failed on
"max 796 units" against the 780 ceiling (`archerKiteBowBand` 650 +
`archerKiteBandSlack` 130). The 796 sample was taken while the
character walked a retreat leg - not a standing fight.

Run 9a (the 22:42 5-minute window, the walking fix in but no bow
fence): the pick selected a level 8 Kaboo Orc Archer (template
20469, bow item 14, 203 HP). The anti-archer gate correctly kept
the kite off, so the character stood at 385 units and traded 19-31
damage shots for about a minute while potions held the HP at 52-70
percent. A Grunt added at +57s, the two-mob pile-up became
non-winnable under the 60 percent re-engage line and the emergency
logout fired at +66s: the 600-unit run crossed the archer's 1100 bow
range in about 9 seconds and the session dropped at 52 percent
health - no death (the relogin at +77s landed alive at 146/214 HP),
but the `survives` check counts any online drop as a failure. The
range check was green there (standing max 386 units), proving the
walking fix.

Run 9b (5 minutes, the walking fix and the bow fence):
`archer-kite-20261109-230523-34856-1.log` - every check green: 107
fight samples, 71 percent ranged, farthest standing fight 527
units, 19 kite steps, 0 cornered holds, min HP 61 percent, min
arrows 590.

## Acceptance criteria

- The updated `TestKiteDisengagesTheLosingFight` pins the drop and
  the straight flight at 20 percent HP, no logout above 12 percent.
- The walking sample never latches `maxFightDist`
  (`TestArcherKiteFoldLatchesTheTelemetry`,
  `TestArcherKiteSampleOfReadsTheTracker`).
- The bow fence picks the melee neighbor over the nearer archer for
  the bow profile and still lets the melee profile initiate
  (`TestArcherAvoidsInitiatingOnTheBowMob`) with the state core
  (`TestNearestAttackablePreferredAvoidsBowWielders`).
- The full `internal/swarm/hunt`, `state` and `acceptance` suites,
  lint and build stay green.
- The 5-minute live `archer-kite` run passes every check.

## Constraints

- Only `temp*` accounts for live checks; persistent test1..testN
  untouched; no `-abuse`; the Mobius server stays running; untracked
  `runs/*` evidence never committed.
- Committer `melg8 <public.melg8@gmail.com>`.

## Progress

### 2026-11-09 - the 5-minute failure and the stale suppression

- Root cause: `loop.go` gated `losingFight()` on `!kiteOwnsRetreat()`,
  introduced by 980a3d8a when the flight did not exist yet (the
  drop handed the melee fan a zone box the kite ground had left).
  The run-3 round then added `kiteFleeThreat`, which answers exactly
  that scene, so the suppression became the death path.
- Fix: the losing-fight gate drops the fight for the kite too; the
  targetless flight (`kiteFleeThreat`) is the escape, the pile-up
  answer and the critical-HP logout keep their own gates.

### 2026-11-09 - the walking-sample band verdict (run 8)

- Root cause: `archerKiteWatch.fold` latched `maxFightDist` from
  every fight sample, including the walking retreat legs whose gap
  to the lagging chaser is supposed to open past the band.
- Fix: `archerKiteSample.walking` from `Bot.SelfWalking`; `fold`
  skips the band latch on a walking sample (fight and ranged counts
  stay). The pure fold test pins a 796-unit walking sample that must
  not move the 420-unit standing ceiling.

### 2026-11-09 - the bow-mob pick fence (run 9a)

- Root cause: the pick initiated on a bow-wielding mob; the owner
  rule refuses the kite against archers, so the fight became the
  standing trade the anti-archer gate leaves. The trade was a
  break-even race that a melee add turned into a two-mob pile-up;
  the emergency logout (the designed answer at <60 percent HP with
  two attackers) dropped the session and failed `survives`.
- Fix: `Loop.avoidBowTargets()` (the kite enabled and the bow in
  hand) threads the `avoidBows` flag into the constrained target
  search (`NearestAttackablePreferred*`,
  `ZoneHasPickableWindowed`, `NearestBlockedTargetsWindowed`): bow
  mobs never enter the fresh pick, the diagnostic names them "bow
  wielder (the archer trade is avoided)" and an archer-only ground
  reads empty for the zone rotation. The attacker scan is untouched,
  so an attacking archer is still answered.
- The losing-fight disengage and the walking band fix stay as is;
  run 9b confirms all three together.

### 2026-11-09 - the live confirmation (run 9b)

- Patched binary `swarm-kite-bowfence.exe`, the stack on
  `127.0.0.3:2106` / `7777` / `3306`, temp18, the 5-minute window.
- `logs/acceptance/archer-kite-20261109-230523-34856-1.log`:
  `Acceptance: PASS`, all six checks green (see the telemetry in the
  evidence section). The full `hunt` (107 s), `state` and
  `acceptance` suites, `task --force lint:new` (0 issues),
  `task fmt:check` and the build passed before the run.

## Status

Complete. The commit `aa1e8119` carries the disengage gate, the
standing-sample band and the bow fence; the 5-minute run 9b
(`logs/acceptance/archer-kite-20261109-230523-34856-1.log`) passed
every check. The follow-up round of the shot cycle continues on the
same branch (see `2026-11-10-issue0-kite-shot-cycle.md`).
