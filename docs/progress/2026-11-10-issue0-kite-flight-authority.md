# The archer kite flight authority (status: in progress)

Started 2026-11-10, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue). Follow-up of
`docs/progress/archive/2026-11-10-issue0-kite-pile-ceiling.md`: the
closing run 27 passed, the consecutive confirmation run 28 exposed
the unkillable-attacker flight below.

## Goal

Make the archer give up a fight its arrows cannot win and keep the
straight flight until the server itself clears the aggro, so the
window spends its time killing winnable mobs instead of re-running a
losing melee race on every skip expiry.

## Context

- Run 28 (5 minutes, the run-27 build) failed `kills` with 1 kill of
  the wanted 2, min HP 24 percent, 29 shots and only 21 shot-cycle
  steps of 73.
- At +62.8 s a level 10 Kaboo Orc Fighter spawned and aggroed on the
  level 7 archer fighting a Spore Fungus. The old `+5` kite ceiling
  (`kiteEngageLevelSlack`) had marked it engageable, so the approach
  switch adopted it at +67 s. A second Fighter joined at +78 s.
- The pair took the archer from 100 to 53-59 percent of its health
  in 16 s while the target lost a third of its bar: about half of
  the arrows missed, the Fighters dealt 13-16 per swing at 163 HP
  each. The health/pile-up gate dropped each fight at ~59 percent.
- The targetless flight then ran, but the 2-minute flee skip expired
  at +214.05 s and the aggro answer re-adopted the same Fighter
  exactly there. The out-of-zone adoption re-adopted a Fighter again
  at +267 s ("keeps attacking outside the zone, finishing it"). Both
  re-adoptions restarted the losing race mid-flight.
- Server fact (Mobius C1 `AttackableAI.MAX_ATTACK_TIMEOUT`, 1200
  ticks of 100 ms): a mob that cannot land a hit drops its aggro two
  minutes after the last swing and walks/teleports back to spawn. A
  flight that outlasts that clock ends the chase by itself.

## Decisions

- The `+5` kite engage ceiling is reverted: `attackerEngageable` and
  `pileUpWinnable` read the general pick ceiling (`maxTargetLevel`,
  character level plus two) for every profile. Run 28 measured the
  trade the widened ceiling never bought; the winnable packs keep
  the melee rule and the bow rhythm (run 27 farmed its kills on
  them).
- `fleeSkipDelay` grows from 2 to 3 minutes so the skip outlives the
  server timeout and cannot lapse into a re-adoption.
- New `kiteFlightHolds`: while any current attacker still carries a
  live flee skip, the straight flight owns the aggro answer. It
  guards both the targetless aggro answer (`engage`, loop.go) and
  the out-of-zone adoption (`adoptOutZoneFight`, loop_movement.go).
- A target inside the kill window (< 30 percent) is still finished
  before the flight converts.

## Acceptance criteria

- The full `internal/swarm/hunt` and `internal/swarm/state` suites,
  the build and `golangci-lint run --new` stay green.
- A 5-minute live `archer-kite` run passes all eight checks,
  including `kills` and `survives`, and a confirmation run does the
  same.

## Constraints

- Only `temp*` accounts for live checks; persistent test1..testN
  untouched; no `-abuse`; the Mobius server stays running; untracked
  `runs/*` evidence never committed.
- Committer `melg8 <public.melg8@gmail.com>`.

## Progress

### 2026-11-10 - the flight authority fix

- `attackerEngageable` and `pileUpWinnable` reverted to the general
  pick ceiling (`internal/swarm/hunt/loop_safety.go`); the
  `kiteEngageLevelSlack` / `kiteAnswersMelee` /
  `kiteEngageLevelCeiling` helpers are removed from
  `internal/swarm/hunt/kite.go`.
- `fleeSkipDelay` is 3 minutes with the C1 attack-timeout rationale
  (`internal/swarm/hunt/loop.go`).
- `kiteFlightHolds` added and wired into the targetless aggro answer
  and `adoptOutZoneFight`.
- Tests: the stale ceiling scenes rewritten (`kite_edge_test.go`):
  the level 10 Fighter past the ceiling is fled, the within-ceiling
  pair keeps the fight, the mixed pile up (Spore Fungus + Fighter)
  converts to the flight; new tests pin the flight hold against a
  fresh add and the skip outliving the server attack timeout.
- `go build ./...`, the hunt and state suites and
  `golangci-lint run --new` pass.

### 2026-11-10 - the contact standstill (run 29)

Run 29 (the 5-minute window of the flight-authority build) got 3
kills (the floor is 2) and 25 of 27 shot gaps inside the cooldown,
but failed `survives`: at +117 s the loot interrupt dropped a fight
whose target died, and the character stood motionless for 21 s at
(35516,51008,-3639) while a level 10 Kaboo Orc Fighter broadcast
from the very same coordinates landed 15 damage every 1.9 s. The
health fell to 11 percent, the death reconnected the session at 6
percent and the survival check reopened.

Root cause (two halves, both in the flight):

- `kiteChaserBearings` skips a chaser at distance under one unit (no
  away direction exists), so the train direction resolved the zero
  vector. `kiteFleeThreat` read that as "no bearing yet", returned
  without a walk, and `fleeFromThreat` fell through to the silent
  `kiteLayerArmed()` return. Every tick spent 21 s on nothing.
- The shot that preceded the interrupt armed its deferred retreat
  click 1.2 s earlier; the loot interrupt zeroed the target, and the
  orphaned schedule kept the disable hold (`combatAvoidUntil`) armed:
  the flight would have waited the whole bow window out even with a
  bearing.

Fixes:

- `kiteContactAway` (kite_lane.go): a melee attacker in contact
  takes the first open ring ray (the character's facing, the two
  perpendiculars, the back ray) at the full escape segment, each a
  straight run. Contact is the one scene the ground hold never owns.
- `kiteFleeThreat` and `kiteResolveAndClick` use it instead of the
  degenerate skip/hold; a fully walled ring still holds and logs.
- `kiteClickDrop` extracts the schedule stand-down of
  `kiteClickWalk`; `kiteFleeThreat` runs it for an orphaned schedule
  before the busy check, so the flight starts on the first accepted
  tick (the real bow windup still gates through `kiteFlightMovable`).
- The `dist < 1` guards of the shot-cycle triggers (`kiteFromShot`,
  `kiteClickWalk`, `kiteFromTarget`) are removed: they refused the
  retreat exactly when the mob stood on the character.
- Tests: the contact flight, the orphaned-schedule release and the
  contact shot-cycle retreat (`kite_flee_test.go`).
- `go build ./...` and the full `internal/swarm/hunt` suite pass.

### 2026-11-10 - the stale selection of the flight (run 31)

Run 30 (the contact-fix build) passed all eight checks: 3 kills,
minimum health 69 percent, 28 shots with 23 of 24 gaps inside the
cooldown, 76 kite steps. Run 31 failed only the bow band: at +118 s a
Kaboo Orc Archer add fired the bow-add verdict, the loop dropped the
grunt it was fighting and ran a healthy 132-second flight (health
recovered from 35 to 70 percent), but the dropped fight left the
server side selection and the tracked fight target on the abandoned
grunt. The attack stance lingers on the incoming blows, so the
standing samples at the leg ends read a fight with the mob 868 units
behind (751, 881, 903 later) and reopened the range check.

Fix: `fleeFromTarget` sends the clear self click with the dropped
fight (the official unselect the town trips already use, now
`clearServerSelection`), and the answered `MyTargetSelected` of the
own object zeroes the tracked fight target
(`Bot.ApplySelfTarget`) - a flight never reads as a fight.
`ClearTarget` also treats a selection of the own object as a no-op,
so a repeated clear cannot re-arm the follow-self intention. Tests:
the fight clear of the disengage (`kite_edge_test.go`), the fight
target drop on the self selection (`tracking_test.go`) and the
repeated-clear guard (`game_handlers_test.go`).
- `go build ./...`, the hunt, state and connection suites and
  `golangci-lint run --new` pass.
- Run 32 (5 minutes, the cleared build) passed all eight checks: 48
  shots with every gap inside the cooldown, 61 kite steps (36
  shot-cycle), 0 holds, 5 kills of the wanted 2, 89 standing fight
  samples with the farthest at 613 units, min HP 63 percent, min
  quiver 552. The bow-add flight of +225.8 s started with the clear
  self click (`Action` on the own object, the `MyTargetSelected`
  answer on the next line) and contributed no fight samples.

### 2026-11-10 - the dead target of the loot stop (run 33)

Run 33 (5 minutes, the cleared build) passed seven of eight checks
(61 shots, 5 kills, min HP 22 percent, 0 holds) and failed the bow
band by one unit: 114 standing samples, max 781 against the 780
ceiling. The reconstruction of the packet log
(`logs/acceptance/archer-kite-20261110-131107-36916-1.log`) found a
single scene behind it: at +163 s the character killed 268439322,
walked 781 units to a drop of that same kill and stopped to loot.
The attack stance lingered on the incoming blows and the tracker
still reported the dead mob as the fight target, so the loot stop
sampled as a standing fight at 781 units five seconds after the kill
ended. No shot was released beyond 426 units: the band never saw a
real long-range trade.

Fix: `clearSelfTargetLocked` (the target died or left the world) now
also zeroes the tracked fight target, and the explicit zero or
own-object answer of `ApplySelfTarget` does the same - a corpse is
not the object of an ongoing fight. Test:
`TestFightTargetClearedWhenTargetDies` (the run-33 scene).
- `go build ./...`, the state, connection and hunt suites and
  `golangci-lint run --new` pass.
