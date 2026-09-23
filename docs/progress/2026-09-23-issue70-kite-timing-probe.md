# Task: the kite timing probe and the archer fleet audit (issue #70)

Goal: measure how the Mobius C1 server answers a retreat click after
the bow shot started (the applied experiment), and build the mass
acceptance audit that names which of the five improved-kite behaviors
the fleet does not implement today.

## The five audited behaviors

1. Hold the maximum distance from the target.
2. Shoot the bow.
3. Regain the distance before the reload ends (not waiting the full
   cooldown).
4. Re-shoot the moment the distance is back.
5. Retreat on a curve that stays near the farm point (no straight
   line runaways).

## State (2026-09-23, session 1)

- `internal/swarm/acceptance/kite_timing_probe.go`: the
  `kite-timing-probe` scenario (account temp19). A raw wire session
  (no hunt loop): dresses the bow kit via UseItem, forces attacks on
  the Kaboo Orc cell mobs, walks a ten step retreat delay ladder
  (0..3000 ms after the own Attack broadcast), classifies every round
  (immediate / deferred / no-move, refusal counts, move lag from the
  shot), then a cursor key round (MoveToLocation mode 0 +
  ValidatePosition claim stream) that measures whether the character
  moves through the windup and whether the target still takes the
  damage while it moves.
- `internal/swarm/acceptance/archer_fleet.go`: the `archer-fleet`
  scenario (temp20..temp24) - five archers on five different Kaboo
  cells (elven-hex-104, 106, 105, 086, 087), staggered logins, a 2.5
  minute audit window (SWARM_ARCHER_FLEET_MINUTES, whole scenario
  under five minutes), 250 ms per-bot sampling folded into the five
  behavior verdicts (median fight distance, shot count, median
  shot-to-retreat lag, median walk-end-to-shot gap, anchor leash plus
  walk turns). The failure list names the unimplemented behaviors.
- Unit tests: `kite_timing_probe_test.go` (classification, ladder
  summary, recorder, geometry, registration), `archer_fleet_test.go`
  (slots, reset, fold verdicts against synthetic improved and legacy
  streams, medians, registration).
- The exhaustruct ignore patterns of the new aggregates landed in
  `.golangci.yml`.

## Server facts the probe is built on (the Mobius source read)

- The bow shot commits at the Attack broadcast (t=0): the hit roll,
  the arrow consumption and the HitTask schedule happen before it,
  and the damage task carries no attacker movement check - nothing
  after the broadcast cancels the hit.
- `PlayerAI.setIntentionMoveTo` DEFERS a mouse-mode MoveToLocation
  while `isAttackingNow()` holds (the first half of the bow cycle,
  (timeAtk+reuse)/2 ~= 1.48 s for the Short Bow at pAtkSpd 337): the
  click draws an immediate ActionFailed and replays at the cycle end
  (timeAtk+reuse ~= 2.97 s). A click in the reuse tail lands
  immediately.
- The cursor key movement (MoveToLocation mode 0 + the
  ValidatePosition claim stream) bypasses the deferral: the packet
  handler latches the cursor key flag before the deferrable AI
  intention, and the per-claim position adoption has no attack check
  - the true WASD shoot-while-moving window.

## Next steps

- Run `kite-timing-probe` on the live stack and record the measured
  boundary (the earliest immediate retreat delay) and the WASD round
  answer into `docs/kite_timing_findings.md`.
- Run `archer-fleet` and record which behaviors fail today.
- Write the fix proposals (see the findings doc skeleton in the
  report of this branch).

## Results (2026-09-23, session 1, live run 14:06-14:08)

The probe PASSED - all ten ladder rounds plus the cursor key round:

- The mouse-mode retreat click is deferred inside the windup: rounds
  at 0/300/600/900/1200 ms all moved at 2.97 s after the shot.
- The earliest immediate retreat: 1500 ms after the shot (the windup
  end, (timeAtk+reuse)/2 at pAtkSpd 337) - rounds at 1500/1800/2200/
  2600 ms all moved at delay + RTT.
- The 3000 ms round raced the auto re-shot (2.97 s) and deferred into
  the NEXT cycle (5.93 s).
- The cursor key round: 7 claim adoptions echoed, 0 net units moved,
  the damage landed anyway (HP 89% -> 76%) - the WASD stream never
  spoiled the shot.

The full analysis and the fix proposals live in
docs/kite_timing_findings.md.

## Next steps

- Run `archer-fleet` on the live stack (the code is registered and
  unit-tested; the session clock ended before its live round).
- Implement the windup-end retreat click in hunt/kite.go per the
  findings doc.
- The cursor-key follow-up round: pin why the adopted claims produced
  no net movement (the deferred intention snap-back hypothesis).
