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
