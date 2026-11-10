# The archer kite pile-up ceiling (status: complete)

Started 2026-11-10, branch feature/kite-range-contract, issue #0
(the owner instruction, no board issue). Follow-up of
`docs/progress/archive/2026-11-10-issue0-kite-shot-cycle.md`: the
closing run 25 passed, the consecutive confirmation run 26 exposed
the pile-up ceiling mismatch below.

## Goal

Keep the archer's fight alive when the whole attacking pack sits
inside the KITE engage ceiling, so the shot cycle resolves kills
instead of converting a normal two-mob fight into a window-long
flight.

## Context

- Run 26 (5 minutes, the run-25 build) failed `kills` with 0 kills
  of the wanted 2. At +12 s the level 7 archer fought a level 9
  Spore Fungus while a level 10 Kaboo Orc Fighter chased it. Both
  mobs sit inside the kite engage ceiling (`kiteEngageLevelSlack`
  5, so level+5 = 12), but `pileUpWinnable` read the MELEE pick
  ceiling (`targetMaxLevel`, level+2 = 9): the Fighter was "past
  the ceiling", the pile up gate called the pack unwinnable at 79
  percent health and `dropFightForFlight` dropped the Spore Fungus.
  The targetless aggro answer re-adopted the Fighter half a second
  later and the same gate dropped it too. Both mobs went on the
  2-minute flee skip, the nearest chaser was the parity Fighter
  (130 vs 125 speed), and the character flew from it for the rest of
  the window (81 kite steps, 6 shot-cycle, 9 shots).
- At +93 s and again at +218 s the character briefly re-engaged new
  mobs and the same gate dropped each fight within seconds while the
  Fighter kept chasing - the failure repeated until the verdict.
- The pile-up policy of `docs/hunting.md` names the ceiling: "every
  attacker sits inside the engage level ceiling". The verdict read
  the wrong ceiling for the bow profile.

## Decisions

- `pileUpWinnable` uses the ceiling of the layer that fights the
  pack: `kiteEngageLevelCeiling` (level+5) when the archer kite is
  armed, the general `maxTargetLevel` (level+2) otherwise. The health
  gate (< 60 percent) and the attacker count gate (> 3) still
  escalate a genuinely bad pack into the flight.
- The pile-up unit scenes move to a genuinely past-ceiling attacker
  (Gora Werewolf, template 1000012, level 16) so the "unwinnable"
  tests keep testing what their names say.
- New regression test: the run-26 pair (Spore Fungus level 9 +
  Kaboo Orc Fighter level 10 against a level 7 archer) stays
  winnable and the tick keeps a fight.

## Acceptance criteria

- The full `internal/swarm/hunt` and `internal/swarm/state` suites,
  the build and `golangci-lint run --new` stay green.
- A 5-minute live `archer-kite` run passes all eight checks,
  including `kills`.

## Constraints

- Only `temp*` accounts for live checks; persistent test1..testN
  untouched; no `-abuse`; the Mobius server stays running; untracked
  `runs/*` evidence never committed.
- Committer `melg8 <public.melg8@gmail.com>`.

## Progress

### 2026-11-10 - the ceiling fix

- `pileUpWinnable` reads `kiteEngageLevelCeiling` for the armed
  archer kite (`internal/swarm/hunt/loop_safety.go`).
- The two "unwinnable pile up" unit scenes use the level 16 Gora
  Werewolf (template 1000012) instead of the level 15 High Orc
  (template 1000010, exactly at the level 10 kite ceiling).
- `TestKitePileUpKeepsTheWithinCeilingPack` pins the run-26 scene:
  the Spore Fungus + Kaboo Orc Fighter pair is winnable and the
  tick keeps a fight.
- The hunt and state suites and `golangci-lint run --new` pass.
- Run 27 (5 minutes, the fix set above) PASSED: 69 shots, 95
  percent of the gaps inside the 4 s cooldown, 57 kite steps (52
  shot-cycle), 0 holds, 7 kills, 141 standing fight samples with the
  farthest at 495 units, min HP 55 percent, min 531 arrows, online
  the whole window. The shot-cycle share is the highest measured so
  far - the kept two-mob fight spent its reload tails walking.
