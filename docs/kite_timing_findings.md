# The kite timing findings: the retreat window of the Mobius C1 bow cycle

The applied experiment of issue #70 (the `kite-timing-probe`
acceptance scenario, account temp19) measured how the server answers a
retreat click issued at a controlled delay after the own bow shot.
The live stack: the Short Bow kit at level 7 (pAtkSpd 337), the Kaboo
Orc Grunt SW-5 cell, one raw wire session (no hunt loop), ten ladder
rounds plus the cursor key round. The run of 2026-09-23 14:06-14:08
(all ten rounds landed, the scenario PASSED).

## The server bow cycle (live numbers, pAtkSpd 337)

```
t=0.00s  the own Attack broadcast (the shot commit: the hit roll, the
         arrow consumption and the HitTask schedule already happened)
t=1.48s  the windup ends (isAttackingNow lapses at (timeAtk+reuse)/2;
         the HitTask lands the damage about here)
t=2.97s  the disable window ends (timeAtk+reuse; the READY_TO_ACT
         replay point; the next shot can start)
```

## The measured retreat ladder (mouse-mode MoveToLocation)

| Click delay after the shot | Verdict | Movement started |
| --- | --- | --- |
| 0 ms | deferred | 2.97 s after the shot |
| 300 ms | deferred | 2.97 s |
| 600 ms | deferred | 2.97 s |
| 900 ms | deferred | 2.97 s |
| 1200 ms | deferred | 2.96 s |
| **1500 ms** | **immediate** | **1.52 s** |
| 1800 ms | immediate | 1.82 s |
| 2200 ms | immediate | 2.22 s |
| 2600 ms | immediate | 2.62 s |
| 3000 ms | deferred | 5.93 s (the NEXT cycle end) |

Readings:

- A click inside the windup (the first ~1.5 s) is DEFERRED, not
  dropped: the movement starts at the full cycle end (2.97 s). The
  deferred click costs the whole reload window of standing time.
- The earliest immediate retreat sits at **1500 ms** after the shot -
  the windup end, exactly the `(timeAtk+reuse)/2` boundary the Mobius
  `PlayerAI.setIntentionMoveTo` check draws.
- The 3000 ms round crossed into the NEXT auto-attack cycle (the
  attack stance re-shot at ~2.97 s) and deferred to 5.93 s - two full
  cycles. A retreat click that races the re-shot loses to it.
- Zero ActionFailed answers were attributed before the movement (the
  deferral answer either rides the same broadcast tick or the refusal
  attribution window of the recorder missed it; the movement timing is
  the reliable signal).

## The cursor key (WASD) round

- The mode-0 MoveToLocation right after the shot plus the 200 ms
  ValidatePosition claim stream: the server processed the claims (7
  ValidateLocation broadcasts) but the net movement was 0 units - the
  cursor key latch did not carry the character through the windup
  under these conditions (the deferred AI intention may have snapped
  the adopted positions back; the follow-up round belongs to the next
  session).
- The damage landed regardless: the target HP fell 89% -> 76% while
  the claim stream ran. The WASD packets never aborted or spoiled the
  shot - the arrow of the anchored shot always lands.

## What this means for the improved kite

The current `kiteFromShot` (hunt/kite.go) clicks the retreat at the
Attack broadcast (t=0). The server defers that click to 2.97 s, and
the re-attack request (gated by `combatAvoidUntil`, sized to the full
`kiteWalkWindow` = 2.97 s) fires at the same moment - the walk starts
only to be cancelled by the next attack. The bot spends the whole
reload standing still: the observed fleet retreat lag (the
`archer-fleet` audit measures it) rides at ~3 s per cycle.

The measured window says the retreat should click at the windup end:

1. **The first retreat click waits the windup**: delay the kite walk
   to `(timeAtk+reuse)/2` (1500 ms at pAtkSpd 337; the formula reads
   the live `SelfPAtkSpd`) after the own Attack broadcast. The click
   then lands in the accepted window and the movement starts at once
   - 1.47 s of walking per cycle instead of none.
2. **The walk window shrinks to the reuse tail**: from the windup end
   to the disable end (2.97 s - 1.5 s = 1.47 s, ~250 units at the run
   speed 170) - the re-attack request fires at the disable end
   exactly as it does today, but the bot ARRIVES at the re-shot with
   real retreat distance banked.
3. **The deferred click is not a refusal**: the re-click ladder's
   ActionFailed attribution (`kiteClickRefused`) must not rotate the
   endpoint for a click issued inside the windup - the answer is a
   deferral, not a dead cell. The ladder can simply stop re-clicking
   inside the windup (one click at the boundary replaces the 250 ms
   re-click storm).
4. **The re-shot owns the boundary**: a retreat click that arrives at
   or after the disable end (2.97 s) races the auto re-shot and loses
   a full cycle (the 3000 ms row). The retreat click must fire
   strictly between the windup end and the disable end.
5. **The WASD stream is safe but unproven as movement**: the claims
   never spoiled a shot, so a future cursor-key retreat lane can ride
   the claim stream (the escape lane already uses it); making it move
   through the windup needs the follow-up probe round.

The mass audit that measures which of the five behaviors the fleet
implements today is the `archer-fleet` scenario (five archers on five
cells, under five minutes, `SWARM_ARCHER_FLEET_MINUTES`); the fold
thresholds encode the measured numbers (the retreat lag ceiling of
2200 ms separates a windup-end click from a deferred one).

## The verification ladder

- The pure halves (the round classification, the ladder summary, the
  fleet fold verdicts) are pinned by the unit tests of
  `kite_timing_probe_test.go` and `archer_fleet_test.go`.
- The live probe: `go run ./cmd/swarm -acceptance kite-timing-probe`
  (PASSED 2026-09-23, all ten rounds plus the cursor key round).
- The live fleet audit: `go run ./cmd/swarm -acceptance archer-fleet`.
