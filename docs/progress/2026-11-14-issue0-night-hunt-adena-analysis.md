# The night hunt adena analysis (status: in progress)

Started 2026-11-14, branch main, issue #0 (no board issue yet).

## Goal

Raise the fleet adena per hour from the evidence of the live night run,
without stopping the stack and without touching the game server. The
deliverable of this round is a measured, prioritized loss list with the
concrete fix each loss needs; the fixes land in their own rounds.

Acceptance of the analysis: every loss is quantified in wall-clock
hours and adena for the 18.7 hour run, and the source record is named
so a fresh session can recompute it.

## Context

- The run: `-hunt -proxy -login 127.0.0.3:2106
  -web 127.0.0.1:8081 -bots 3 -navmesh ./data/navmesh`, started
  2026-11-14 03:31 from `main` @ 16b3f6fc, still alive at 22:00.
- Sources (read only, the process keeps writing): the session journal
  `logs/session-20261114-033100-33468.jsonl` (structured kills, deaths,
  trip starts and ends, logouts, levels, 10 s state samples), the
  combined bot log `runs/night-run.log`, `proxy.log`, the live state
  API `http://127.0.0.1:8081/api/bots/<id>/state`, and the character
  rows of the stack database.
- The journal duplicates every Hunt line in the plain log, so a log
  grep counts each event twice; the journal counts below are unique.
- Phase shares are interval weighted over the 10-13 s state samples
  (gaps over 120 s dropped, so a bot that died mid gap loses nothing).

## Progress

### 2026-11-14 22:30 UTC - measure the night run

The 18.7 hour totals (net is the adena delta of the state samples,
gross excludes the matched junk sale):

| bot   | kills | deaths | net adena | net/h  | adena/kill |
| ----- | ----- | ------ | --------- | ------ | ---------- |
| test1 | 2919  | 118    | 240902    | 12905  | 82.5       |
| test2 | 2766  | 117    | 328160    | 17580  | 118.6      |
| test3 | 2679  | 88     | 235442    | 12613  | 87.9       |

The phase budget per bot: engage 59.9 / 61.2 / 60.5 percent, townReturn
14.1-16.3, townWalk 11.4-15.1, delevel 5.3-7.3, loot 3.6-4.2,
townSell 0.4-0.9. The town block is therefore 27.7-30.2 percent of the
wall clock, and the engage rate inside it is 217-238 kills/h.

### Finding 1: the guide trip after a delevel finish (the largest)

All 37 delevel finishes were followed by a Newbie Guide trip start
within 300 s, median 167 s. The 167 s is the walk home (the finish sets
the return walk) plus the 45 s `weaponRunCooldown`, and the guide trip
then walks the same 170-204 s back to the village. The character stood
next to the guide when it finished the delevel.

Cost: about 315 s of duplicated walking per finish, 3.2 h across the
fleet, 1.08 h per bot, 5.9 percent of the wall clock, 20-30k adena per
bot.

Fix: after `finishDelevel` and `abortDelevel` in the village, run the
guide stop (and the sell when the bag is full) before the return walk,
the way `recoverFromDeath` already clears `tripEndedAt` so a revived
character sells right away.

### Finding 2: the 16 to 17 delevel loop on a level 9-10 cell

test1 ran 12 extra delevel episodes between 17:13 and 19:25 (plus three
after 21:31), test3 three: 60-90 s each with 0-2 deaths. A death from
17 lands at 16 with the experience at the top of the 16 window (the
window is 417203..426222, 9k wide), so one or two kills restore 17 and
the trigger fires again. The fleet logged 45 "delevel death 1 removed
no experience" deaths.

Cost: about 1.1 h for test1 and 0.35 h for test3 without a single kill.

Fix: the delevel cooldown after a finish is one minute
(`delevelCooldown`), so the next trigger is always armed; a finished
delevel needs a longer settle, at least until a level up beyond the
trigger, and the trigger should read the ground the character could
hunt at its level rather than the ground it stands on.

### Finding 3: the emergency logout at a 55-60 percent bar

93 logouts (test1 13, test2 38, test3 42): 45 pile ups (23 of them
exactly two mobs) and 48 "HP 55-60 percent under attack". The label is
the health at the logout, not the trigger: the flee runs
`fleeLogoutAfter` (20 s) and ends the session when the chaser keeps
pace, which a 125 speed character cannot avoid against the same speed
mobs.

`losingFight` flees when the target leads the character by 25 percent
of the bar and the character is under 60 percent, a relative measure
that treats 59 percent of a level 18 pool against a level 12 mob as a
lost fight.

Cost: median 85 s to the next kill, test2 mean 327 s and p90 953 s,
about 2.2 h at the median across the fleet, 4 percent of the clock.

Fix: gate the pile up run on the effective threat (the mob level
against the character level and the absolute health pool), not the raw
count of two, and let a single same-or-lower-level mob be traded at
59 percent instead of abandoned.

### Finding 4: the interrupted town trip loses the whole plan

127 "town trip interrupted" lines, 107 of them a mob on the outbound
walk. `interruptTripForAttacker` calls `resetTownTrip`, so the frozen
sell and guide stops die with the walk and the next tick starts the
trip from scratch. The log shows the pattern: start at 04:22:54,
interruption at 04:23:20, a fresh start at 04:23:31, and the trip
finally ends at 04:28:29 (297 s measured from the restart).

Cost: about 40 s of lost walk per interruption, 1.4 h across the fleet.

Fix: keep `tripStops` across the interruption and re-path from the
current position, the way the trip already re-paths on a stuck
waypoint.

### Finding 5: the cell distance and the town cost

The median hunting distance to the guide is 21.2k / 24.4k / 25.5k
units (a 169-204 s straight line return walk at speed 125). The coarse
2k grid shows 311-366 adena/min at 17-27k and 162-253 at 30-35k, so the
far edge of the registry is already a loss. `cellProximity` is
1/(1+d/8000) anchored at the live character position, so a cell looks
closest exactly when the character stands in the village, and no term
models the recurring 250 s trip every 22 minutes.

Fix: score the travel against the village anchor and charge the trip
cost inside the cell value, so a cell that pays 100 adena/min more but
costs 100 s more per trip loses.

### Open measurement: the ActionFailed volume

40352 "Action failed" lines in 18.7 h, 4.8 per kill, clustered around
Power Strike and the attack spam. The cause is unidentified; the packet
is the server refusal of a client action, not a bot error. A debug line
naming the last sent opcode before each refusal answers it in one
round.

## Status

### 2026-11-14 23:05 UTC - land the fixes and re-measure finding 4

Four of the five findings landed as separate commits, each with its
tests; finding 4 was re-measured and dropped.

- Finding 1 (`b0b1442b`): `endDelevelWalk` hands the phase end back to
  the trip engine. The finish and the abort clear the walk state, and
  when `guideRunWanted` the refill trip starts on the next tick and
  returns to the preserved farm spot; the plain return segment stays
  for the regions without a guide. `TestDelevelFinishRefillsTheGuideFirst`
  and four updated phase assertions.
- Finding 2 (`1c4724ab`): `delevelDropMin` (2) in `delevelWanted` - a
  target that buys fewer than two levels never starts the guard walk.
  A level 17 character over the guide floor now farms instead of
  cycling 17 to 16. `TestDelevelSkipsTheSingleLevelDrop`.
- Finding 3 (`6f83ac22`): the gap rule stands down when the character
  leads the target by `fleeOutlevelSlack` (2) levels, the critical
  health still flees; `pileUpTwoHealthPercent` (45) tanks an exact
  two-mob pair between 45 and 60 percent. Three new tests in
  `aggro_answer_test.go` and `combat_policy_test.go`.
- Finding 4: DROPPED. The re-measurement shows the interruption costs
  the covered walking and the restart only, about 15 s each: 127
  interruptions are roughly 0.5 h fleet-wide, under 1 percent of the
  clock - the 1.4 h estimate priced the whole plan loss, which the
  restart does not pay.
- Finding 5 (`a374fc50`): `cellVillageDrag` multiplies the score by
  the dwell share of the trip cycle (1200 s dwell over dwell plus the
  round trip at speed 125, anchored at `guideForRegion`), while the
  live `cellProximity` keeps pricing the one-time relocation.
  `TestCellVillageDragPricesTheRecurringTrip` and
  `TestCellScorePricesTheVillageTrip`.

Docs: `docs/hunting.md` carries the four rules (the drop floor, the
phase-end refill, the outlevel flee and the pair floor, the village
drag). The hunt package tests, `go build ./...` and the changed-code
lint are green on every commit.

Next step: restart the same three-bot night run on the fixed build and
compare against this baseline (12905 / 17580 / 12613 adena per hour,
2919 / 2766 / 2679 kills, 118 / 117 / 88 deaths).

### 2026-11-14 22:43 UTC - restart on the fixed build

The old process was gone (its log ended 22:19:44, no listener on 8081).
The stack ports were re-checked before the start: login 127.0.0.3:2106,
game 7777 and MariaDB 3306 listening. The same command as the baseline
run started from 9ff12415: `-hunt -proxy -login 127.0.0.3:2106 -web
127.0.0.1:8081 -bots 3 -navmesh ./data/navmesh`; log
`runs/night-run2.log`, journal `logs/session-20261114-224321-31012.jsonl`.
All three bots reported online within 30 s (test1 level 17 engage,
test2 level 17 townWalk, test3 level 19 engage), and `delevelTarget`
stayed 0 on the level 17 pair - the drop floor holds.

### 2026-11-14 22:47 UTC - controlled stop and restart (the harness check)

The owner asked to stop the running bot shell and start it again
without touching the stack. The fleet process was `swarm.exe` (the
`go run` child); a second shell cannot deliver SIGINT to it on this
host, and `taskkill` without /F refuses a console process, so the stop
was the forced kill of the child. `go run` exited with code 1, port
8081 freed and no swarm or go process stayed behind; the stack ports
(login 127.0.0.3:2106, game 7777, MariaDB 3306) kept listening
throughout. The same command restarted the fleet from 2a9a7ad6 into
`runs/night-run3.log`, journal
`logs/session-20261114-224715-19364.jsonl`; all three bots reported
online (engage) within the same minute and the run stays up.

### 2026-11-14 23:40 UTC - the overhit finishing blow reads damage, not percent

Live measurement on `runs/night-run3.log`: the finished fights show
Power Strike level 9 hits of 105-141 damage while the auto attacks of
the same bots land 36-62, and test3's 105 strike into a mob of ~292
max HP left it alive with about one auto attack worth of HP. The old
rule fired the strike at 40 percent of the bar (116.8 HP for that
mob), above the absolute strike damage, so the cast could not kill.

Verified source: `Formulas.calcPhysDam` computes the non-critical
physical skill hit as `76 * (pAtk + power) * proximity / defence` and
the auto attack as `76 * pAtk * proximity / defence`, with the weapon
random multiplier applied to both (`RandomizeAutoAttackDamage` and
`RandomizePhysicalSkillDamage` are True; `getRandomDamageMultiplier`
runs twice in the same function), so the strike-to-swing ratio is
`(pAtk + power) / pAtk`. Power Strike level 9 carries `<power>70`;
the C1 `UserInfo` packet is the only carrier of the live P.Atk (the
server never broadcasts `StatusUpdate.P_ATK` 0x11).

The fix:
- the UserInfo parser reads the P.Atk leading the combat stat block
  after the 15 paperdoll display ids and skips the 11 stats behind
  it; `state.CharacterState.PAtk`, `state.UserInfo.PAtk` and
  `Bot.SelfPAtk` carry it, `connection.applyUserInfo` fills it;
- `state.objectCold` records the last landed hit (`LastDamage`,
  `LastDamageCrit`, `LastDamageAt`) from the HP delta that already
  feeds the combat feed, and `Bot.ObjectLastDamage` exposes it;
- `npcdata.SkillCast.Power` (generated from `<power>` with the
  `#power` table resolution; a float table like `#dot` carries no
  strike power) with `PowerOf`;
- `hunt/combat_skills.go` samples the smallest non-critical swing of
  the current fight (per-target reset, 8 s freshness, the own strike
  landing window excluded, crits skipped) and fires the overhit
  strike only when the bar fits
  `swing * (PAtk + power) / PAtk * 0.85`; no sample, no P.Atk or
  unknown vitals hold the strike.

Tests: `overhit_test.go` rewritten (hold above the estimate, unknown
vitals, no P.Atk, the low bar of a high HP mob that the percent rule
fooled, the release with the log line, the crit filter); the cast and
reuse tests of `combat_skills_test.go` feed the P.Atk and a swing.
Docs: `docs/hunting.md` (the damage-based window) and
`docs/protocol_description.md` (the UserInfo P.Atk field).

Next step: restart the three-bot fleet on the fixed build, confirm the
finished fights report "Over-hit!" with the strike as the killing
blow.

### 2026-11-14 23:52 UTC - live acceptance on the damage window

The three-bot fleet was restarted on aae8fe55 (`go run` child PID
16708, log `runs/night-run4.log`, journal
`logs/session-20261114-234636-16708.jsonl`, web 127.0.0.1:8081, the
same flags as the baseline run). The stack ports stayed up the whole
time (login 127.0.0.3:2106, game 7777, MariaDB 3306). All three bots
reported online and engaged within the first minute; `delevelTarget`
stayed 0 on both level 17 bots.

The damage window accepted six strikes that landed in the first six
minutes and every one killed with "Over-hit!": test1 118 and 150,
test2 106 and 322, test3 110 and 97, each followed by the +25 percent
experience share (294 -> 368 and similar). One further cast at
23:50:08 did not pay the bonus - the mob died to a separate 148 hit
(the log cannot tell a refused cast from a critical auto attack at
that resolution, and the tail of the independent weapon rolls can
also leave a strike just short of its estimate). The strike rate is
far below the night-run3 cadence: 12 casts in the first four minutes
across three bots instead of one every ~15 seconds per bot, because
the strike now waits for the bar to fit its own damage.

### 2026-11-15 02:50 UTC - run4 diagnosis and the delevel trigger fix

Run4 (`runs/night-run4.log`, journal
`logs/session-20261114-234636-16708.jsonl`; the figures below are the
2.86 h mid-run snapshot, the closed run is in the 03:15 entry) measured
39503 adena per hour on the fleet (13.2k per bot) with 1430 kills and
66 deaths, and the fleet **lost** 189342 experience (test3 dropped two
levels). 62 of the 66 deaths were guard deaths of the deleveling: the
phase owned 5/5/15 percent of the bot time, and the four runs
(23:48, 00:25, 00:35, 00:47) burned 39 minutes.

Root cause: the delevel trigger measured the **item** gap. Read from
the Mobius sources - item drops scale from 100 percent at a gap of 5
(`DropItemMinLevelDifference`) to 10 percent at 10, but that pays junk
only; adena holds 100 percent to a gap of 8
(`DropAdenaMinLevelDifference`) and reaches 10 percent at 15; and
experience has **no** gap scale at all (`Attackable.calculateExpAndSp`
awards the full template exp for every gap from -10 to +10 and cuts
off at 11). A trigger at 7 therefore killed the character inside the
full adena and full exp band - a pure loss of time, experience and the
level-keyed power that comes with it.

Fixed in this commit:

- `adenaFullLevelGap` (8) is the single constant of the server rule:
  `cellEligible` builds its lower window from it instead of a literal
  8 and `delevelTriggerDiff` (9) / `delevelTargetGap` (7) derive from
  it, so the picker window and the delevel trigger can never drift
  apart again.
- `delevelHasHigherGround` holds the guard deaths back while the cell
  picker has an eligible ground whose median beats the held one: a
  relocation costs one walk, a delevel costs the walk plus the death
  and the experience.
- The start log line stops claiming the drop collapsed: it names the
  character level, the ground median and the gap the adena scale
  starts at.

Tests: `TestDelevelKeepsAFullAdenaGround` (the run4 field case, a gap
of 7 over the median 8 Dryad ground, must not walk to a guard),
`TestDelevelTriggerConstantsMatchTheServerGap` (pins the three
constants to `Rates.ini`),
`TestDelevelLeavesAGroundWithAHigherOne` (the relocation guard); the
three priority tests moved to a genuinely outleveled level 18, and
`TestCellWindowHelpers` now pins the adena-safe flag on both sides of
the gap. Docs: `docs/hunting.md` (the trigger, the target and the
verified gap rules).

Measured time budget of run4 for the remaining work: engage 59
percent, loot 5, town 27 (24 trips, 14.3 percent of fleet time in the
journal), delevel 8.3.

Ranked plan, each with its expected gain:

- P0 (landed here) - the delevel loop: +8 percent time, +1.5 levels of
  experience, 62 deaths removed.
- P1 - the cell ranking: `cellWindowMass` reads `[level-5, level]`, so
  the mobs above the character level never count although the engage
  window admits `level+2`; `cellProximity` charges a one-time 20-26k
  relocation as a permanent 0.25-0.33 multiplier, so Dryad SW-7 (531)
  outscores the Lirein 16-19 ground (299) forever. Measured adena per
  minute by mob level is 246 (10), 297 (13), 339 (15) - the climb is
  +18-25 percent once the picker takes it.
- P2 - 76 of 883 adena drops were never picked (949 requests, 807
  confirmed): a pickup retry is worth 6-7 percent.
- P3 - soulshots (item 1835, 7 adena, sold in the elven village via
  buylist 0009938): 4.9 swings per kill puts the shot cost at half the
  kill income, so the real gain is +10-15 percent, not the +25-30
  estimated before.
- P4 - the Dion 20-25 handover: `cmd/swarm/session.go` hardcodes the
  elven region while `dionShopCatalog` and `zones_dion.go` are ready,
  and 6.1M adena sits unspent because the elven village sells nothing
  above no-grade.

Rejected as levers after measurement: the guide buff trips (24 trips,
1200 s buffs against a 280 s trip = 1200 * 1.24 = 1488 equivalent
seconds versus 1480 without the trip, break-even), and the gear
planner (correctly silent: no upgrade exists in the elven shops).

Next step: P1, the two cell ranking bugs, then a fleet restart to
compare against this baseline.

### 2026-11-15 03:05 UTC - the cell income window and the relocation cost

Two ranking bugs, both contradicted by the engage window the same policy
already applies:

- `cellWindowMass` and the inline price loop of `cellValue` read
  `[level-5, level]`, so the mobs **above** the character level never
  counted toward the income prior although `cellEligible` and
  `cellMobPriorities` admit `level+2` and the engage happily kills
  them. The server penalizes neither the adena nor the experience of a
  mob above the character level, so the prior was pricing a ground by
  the half of it the bot ignores. Both now read the shared
  `cellWindowBounds` (`[max(1, level-5), level+cellMaxLevelSlack]`),
  and `cellWindowLevel` replaces the duplicated loop in `cellValue`.
- `cellProximity` charged `1/(1+dist/8000)` - the half-score distance
  - for a walk that happens **once**. It now amortizes the relocation
    over the dwell the ground buys (`cellTripDwell/(cellTripDwell +
    dist/cellTripSpeed)`), the same shape `cellVillageDrag` uses for
    the recurring trip; `cellProximityScale` is gone.

Honest result of the re-ranking (offline model, `runs` scratch script):
the pick moves from Dryad SW-7 (894) to Pincer Spider SW-5 (896) at
level 17 - a tie. The measured adena per minute curve (246 at mob level
10, 297 at 13, 339 at 15) is real but the village drag cancels it: the
Lirein 16-19 ground sits 44000 units from the Newbie Guide against the
23000 of the Dryad ground, and the round trip repeats at every buff
cycle. So **ground level is not the lever**, the fixes are correctness
wins (the prior now prices what the bot kills) with a small expected
gain, and the remaining plan turns to the two certain losses: the
pickup failures (P2) and the fight time itself (P3 soulshots, P4 the
Dion handover for a weapon above no-grade).

Tests: `TestCellProximityAmortizesTheRelocation`,
`TestCellWindowCountsTheEngageCeiling`, and `TestCellWindowHelpers`
now pins the above-level band and the window level. Docs:
`docs/hunting_cells.md` (the economy formula and the window).

### 2026-11-15 03:15 UTC - run4 closed: the loot loss is small, the shot lever is dead

Run4 was stopped (force kill of the bot tree, the journal tail parsed)
so the baseline is a closed dataset. Final figures over 3.36 h: 1585
kills, **110 deaths of which 109 fired in the delevel phase**, adena
income 129566 (38560 per hour on the fleet, 12.9k per bot), and
**-539959 experience** with test3 falling 19 -> 17. Fleet time shares
from the 402 samples per bot: engage 57 percent, town
walk/return/sell 28 percent, delevel 11 percent, loot 5 percent.

P2 corrected by measurement - the earlier "76 of 883 drops never
picked, 6-7 percent" was a counting artifact. The loot loop re-sends
the pickup click once per `selectPeriod` (1 s) until the `GetItem`
confirms, so a request count above the confirmation count is the retry
window, not a loss. Per bot: test1 381 drops / 433 requests / 361
confirmed / 7 server removals, test2 341/362/314/8, test3
331/328/278/10. The only real loss is the server removal of a ground
item (`object removed: Adena`): 25 events fleet-wide, 2.4 percent of
the 1053 drop events, about 2.5 percent of the income. The bounded
retry path already exists (`pickupTimeout` 20 s, `pickupRetryDelay`
30 s, `itemSkip`) and never fired in 3.4 h - zero "timed out,
skipping" lines - so the abandoned drops are items the bot walked away
from, not failed pickups. No loot code changes this round.

P3 closed - **C1 sells no soulshots in this datapack**. Item 1835
exists (`dist/game/data/stats/items/1835.xml`, `SoulShots.java`), but:
buylist `0009938.xml` (the one the generated catalog carries at price
0) has no `<npcs>` block, `custom/00099380.xml` (price 7) is referenced
by no npc spawn, no `stats/npcs/*.xml` template sells 1835, and no mob
drop table lists it. The shot lever needs a source the server does not
provide, so the hunt stays shotless; the ranked plan turns to P4 (the
Dion 20-25 handover for a weapon above no-grade).

Next step: restart the 3-bot fleet on the P0+P1 build and compare
against this baseline (129566 adena, 110 deaths, -539959 experience).

### 2026-11-15 12:00 UTC - run5: the town time is the lever, five fixes landed

Run5 closed on the P0+P1 build (`runs/night-run5.log`, journal
`logs/session-20261115-030925-33068.jsonl`, build `7e5e65e9`): 3978
kills, 3 deaths, 359781 adena (45.5k per hour on the fleet, 15.2k per
bot-hour), +1319926 experience, levels 17 to 19/20/20, 92 town trips
(81 guide, 8 shop), 22 logouts and 16741 "Action failed" lines. The
delevel loop and the deaths were gone (P0 worked); the town block took
30 percent of the clock and the guide trips alone 6.4 of 23.8
bot-hours.

The four measurements behind the landed fixes:

- A dedicated guide trip costs a median 289 seconds and 76 of the 81
  trips started from the farm. The paired 44 cycle comparison on the
  same bots read 225 kills per hour buffed and 225 unbuffed, so the
  level 1 support magic buys no kill rate at this DPS (the guide band
  ends at 24; the bots are 17 hours of farming away from it).
- Two of the three deaths landed on a trip start under a mob.
- 9506 of the 16741 refusals (57 percent) arrived right after a kill's
  death broadcast: the corpse clicks and pickup retries were the
  candidates, and the pickup side is now answered within 2 seconds.
- Four emergency logout storms repeated the same ground within a
  minute.

Landed this round, each with its tests and its doc:

- `23db197a` - the town trip start waits a live target and an attack
  out, no trigger starts a dedicated guide trip (`planGuideStop` rides
  the sell trip), the delevel finish and abort walk straight home.
- `58b1e150` - the send path records the last request (opcode and
  time), the ActionFailed log names it, a refused pickup skips its drop
  at once, a dead target takes no strike and no attack request.
- `01f0ebd9` - an emergency logout demotes its cell to a fifth for the
  15 minute danger window; the picker, the resume handoff and the
  follow switch all refuse it, and the tracker ring seeds it across the
  session rebuild.
- `0450f632` - every stagnation fire logs the whole frozen loop state
  (target, loot, trip, waypoints, last action, vitals, purse), so the
  next stuck report needs no live repro.

Rejected after measurement: the ground tier at the current DPS (adena
per minute nearly flat 5.6/4.8/5.1 across the tiers - the Dion
handover only pays with a D-grade weapon), and any adena gap fix (the
target window already floors at `level - 8`).

Docs: `docs/hunting.md` (the trip start rule, the guide policy, the
pickup refusal, the stagnation dump, the delevel phase end) and
`docs/hunting_cells.md` (the danger factor of the score).

Next step: restart the 3-bot fleet on this build and compare against
the run5 baseline (359781 adena, 3 deaths, 92 trips, 45.5k adena per
hour).

### 2026-11-15 12:40 UTC - run6 restarted on the final build

The fleet runs the fixed build (`414234f5`, three bots, `-hunt -proxy
-login 127.0.0.3:2106 -web 127.0.0.1:8081 -bots 3 -navmesh
./data/navmesh`):

- `runs/night-run6.log`, journal
  `logs/session-20261115-122750-10868.jsonl` - the run of record.
- `runs/night-run6-pre.log`, journal
  `logs/session-20261115-120439-31560.jsonl` - the 20 minute ramp of
  the same build before the label fix (`c2518615`), kept as evidence.

First minutes of the record run: three bots online, twelve kills in
the first three minutes, no guide trip line at all (run5 started 81 of
them), no death and no stagnation fire.

The refusal attribution (`Action failed: the request 0x.. was .. ms
ago`) names the candidates the bare server packet never did, over the
first 167 seconds: 29 action clicks (0x04), 25 attack requests (0x0a),
22 move clicks (0x01), 10 skill casts (0x2f), 3 enter world (0x03) and
1 potion drink (0x14). The server sources explain the two leaders: an
`Action` on an object that already left the world and an
`AttackRequest` on a target the server holds dead or gone (both answer
`ActionFailed` at once), while a `MoveToLocation` is refused at once
when the destination is its own origin or the geodata blocks it.

`414234f5` - the pile up that forced an emergency logout logged
"death at the cell", the shared cell regression accounting reused the
death wording. The message now names the cause, so the run log counts
deaths and pile ups apart.

Next step: let the run collect its window and compare kills per hour,
adena per hour, deaths, trips and refusals against the run5 baseline.

### 2026-11-15 12:48 UTC - the journal baseline of run5 and the A/B tool

`tools/journal_stats.py` (new) prints the comparison table of a
measurement round from the journals alone, so both sides of an A/B are
counted the same way:

```
python tools/journal_stats.py logs/session-20261115-030925-33068.jsonl \
    logs/session-20261115-122750-10868.jsonl --phases
```

The run5 journal (`session-20261115-030925-33068`, build `7e5e65e9`,
window 03:09 to 12:02, 8.88 h) reads: 4500 kills, 4 deaths, 4 stalls,
22 logouts, 102 town trips, 65 sell batches, 7784 adena of buys, a net
purse gain of 398740 (44971 adena per hour on the fleet, 14990 per
bot-hour), 1474249 experience (55423 per bot-hour) and 9 level ups;
the phase share is engage 70.0 percent, townWalk 12.4, townReturn
12.7, townSell 0.5 and loot 4.4, so the town block is 25.6 percent of
the bot time. The figure differs from the 359781 of the earlier
analysis on purpose: that one summed the adena pickups the live stats
API reports, this one is the net purse (pickups plus sales minus the
buys) - the run comparison uses the net figure for both sides.

The run6 window at 12:36 (the first nine minutes, the restart ramp
with a boundary death) reads 48 kills, 1 death, 4468 net purse, 31539
adena per hour on the fleet and 339 kills per hour - not comparable
yet, the run needs its hours.

Next step: read both journals with the tool once the run6 window is
long enough to compare.

### 2026-11-15 18:40 UTC - run6 read: the ground climbed out of the gear tier

The run6 journal (`session-20261115-122750-10868`, build `414234f5`,
window 12:27-21:29, 9.02 h) against the run5 baseline reads:

| metric | run5 | run6 |
| --- | --- | --- |
| kills | 4500 | 3022 (-33%) |
| kills per hour (fleet) | 508 | 335 |
| net purse | 398740 | 269199 (-32%) |
| adena per hour (fleet) | 44971 | 29883 |
| adena per bot-hour | 14990 | 9961 |
| experience | 1474249 | 1348119 (-9%) |
| experience per bot-hour | 55423 | 49884 |
| level ups | 9 | 6 |
| deaths | 4 | 4 |
| town trips | 102 | 7 |
| emergency logouts | 22 | 249 |
| engage share | 70.0% | 92.3% |
| town share | 25.6% | 3.8% |
| mean fight | 9.9 s | 14.1 s |
| mean mob level | 11.6 | 14.5 |

Root cause: the level ladder opened the 16-19 band the moment the
level reached 20 (`zoneLevelLead` 1 over `MaxLevel` 19) and
`PickHuntingZone` always takes the highest opened band, so the fleet
moved from level 12-13 mobs to level 16-18 ones overnight. The server
tables price the step: adena per kill 74.7 -> 87.1 (+17%), HP 196.5 ->
262.6 (+34%), pAtk 22.5 -> 33.9 (+51%), experience per kill 348.8 ->
467.2 (+34%). The character DPS stayed at 18.6 HP/s (19.8 in run5), so
the fight grew 42% for 17% more income and the incoming damage grew by
half.

The journal confirms the tables on both sides: run5 collected 74.7
adena per kill from the mobs (the net purse minus the 65 junk sale
batches), run6 87.1 (minus 3 batches) - the drop curve is linear in
the level while the HP curve is not.

The cost per kill went 21.3 s -> 32.2 s: fight +4.2 s, emergency
logout +5.3 s (249 logouts; the median time to the next kill after one
is 96-111 s against a 19 s normal gap), rest +3.4 s (543 sittings,
3.54 bot-hours, median 10 s). The 22 percentage points of town time
the guide policy saved went into the longer fights, not into kills.

Second order: 262 session drops against 25, 327 pile ups against 35,
262 "HP under attack" against 16, 72 flights against 8, and 3097
refusals to follow the standing ground (1749 emergency logout, 1348
death regression) - the bot refuses the ground it stands on while the
zone has nowhere else to go.

The wallet: 2.58M adena, bags 23-31 of 80 slots, 7 trips, 3 sales, the
buys are 908-934 adena of arrows. The next weapon (Bastard Sword, pAtk
51 for 644000) is not sold in the elven village: Unoren's lists
(3014700/3014701) carry the no-grade line only, the D-grade line lives
at Dion (`docs/band_20_25_survey.md`) and the city shopping errand
stops at the no-grade kit.

Two accounting defects found on the way:

- The zone-return walk ("outside the hunting zone, pathfinding back")
  sets `phaseTownReturn` and `tripStart` without a `TripStart` record,
  so its end journals "town trip ended: back at the farm spot": 27
  trip ends against 7 trip starts in run6 (28 zone returns).
- `journal.Dropped()` counts the records a full queue refused and
  nothing reads it.

Next step: the ladder must price the ground, not the level. The
drop-gap scale (`NpcTemplate.java` 722, `Rates.ini` 109-117) plus the
npc HP curve is the deterministic input - the weakest mob inside the
full-drop window pays the best adena per HP. The fix needs the
per-template combat and drop table (HP, pAtk, pDef, exp, mean adena)
generated from the C1 npc stats, then a band score that divides the
gap-adjusted adena per kill by the fight time at the measured DPS; the
resulting cap feeds the existing `maxMinLevel` of `PickHuntingZone`.

### 2026-11-16 03:10 UTC - the ground economy landed

Committed `ca2f9b11` (the economy) and `f1b22350` (the trip
accounting):

- `npcdata/npc_combat_table.go` (1837 rows, generated by
  `tools/generate_npc_combat.sh` from the C1 npc stats): level, hp,
  pAtk, pDef, exp and the mean adena of the drop group
  (`group chance x item chance x (min+max)/2`), verified against the
  run6 journal to the tenth of adena on the six grounds the fleet
  farmed (Kaboo Leader 76.65, Dryad 78.8, Dryad Elder 81.5, Hook
  Spider 86.8, Pincer Spider 90.3, Lirein 94.5).
- `hunt/cell_economy.go`: the income window economy (the
  `DropAdena*` gap scale of `Rates.ini`), the process wide damage
  rate meter fed by the kill fight times, and the ground capacity
  `60 / (hp / rate + 6 s loot and pull + 0.30 s per point of pAtk)`.
- `hunt/cell_metrics.go`: `cellValue` is supply x capacity x adena;
  the level priced `cellAdenaBase/PerLevel` prior is gone.
- `hunt/cell.go`: the window floor follows the full drop gap
  (level-8), so the level 15-17 grounds are priced at level 23.
- The zone return walk writes no trip-end, arms no trip cooldown and
  joins no abort streak; `Journal.Close` reports the refused records.

The modelled ranking at level 23 with the meter at 19 HP/s (the run6
fight times): Dryad Elder W-11 / Crimson Spider SW (level 15,
202.6 adena/min) above Lirein SW-6 (180.4), +12.3%. The cap is the
ratio of the model, so the expected gain is +8-15% per bot, not the
+35-50% the level prior promised; the NG kit stays the binding
constraint and the D-grade line is the separate M3 work.

Next step: rebuild, restart the 3-bot fleet on this build, read the
picks in the log (the level 15-17 band instead of Lirein 18/19) and
A/B the journal with `tools/journal_stats.py` against run6.

### 2026-11-16 03:05 UTC - the fixed build is live

Run7 started 02:51 on `b2f1f99b` (fleet pid 33464,
`runs/night-run7.log`, journal `session-20261116-025103-33464`), the
stack untouched. The first ten minutes:

- the three bots resumed level 23; the re-picks after the death and
  the emergency logout landed on the level 16 grounds (Hook Spider
  SW-1/SW-2, Pincer Spider SW-5) instead of the Lirein 18/19 band
  run6 kept choosing - the intended shift; the held-ground hysteresis
  (`cellSwitchMargin` 1.25) is above the modelled +12%, so a healthy
  Lirein ground is not dropped for it and the gain accrues on the
  next pick (death, logout, starvation, level up);
- the zone return walk ended with "Hunt: zone return finished: a
  target stands inside the zone" and the journal holds no trip-end
  record - the accounting defect is closed live;
- the live measured rate of the Pincer ground read 249 adena/min at
  the first stops, the run6 Lirein rate was 165 (the model's 180 is
  the conservative side).

The A/B against run6 needs a few hours of run7; the journal is the
record and `tools/journal_stats.py` reads it.
