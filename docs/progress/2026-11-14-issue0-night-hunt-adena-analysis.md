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
