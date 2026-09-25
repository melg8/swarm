# 2026-09-26: the engage claims - the fight approaches ride the abuse channel

## The report

The -abuse flag left one direct leg in place: the run-up to the mobs.
The attack request is itself a movement order - the forced attack arms
the chase intention and the server AI runs the character to the target
at run speed while the chase stays healthy, and the stall watchdog of
the loop only walks when the chase gives up. A flag that swaps WalkTo
alone never touches that leg, so the bots kept running up to every mob
they engaged.

## The change

hunt.abuseEngageClaim (internal/swarm/hunt/abuse_engage.go): under
-abuse, every approach leg to a target beyond the weapon engage radius
(150 melee / 450 bow) sends ONE claim at the engage point - half the
radius on the line from the target toward the character - which lands
the character inside the attack range the moment the server adopts it;
the chase the server planned dies with the placement change, no run leg
ever covers the distance. Three hooks carry it: the armed-chase branch
of the engage (past the stall watchdog), the pre-attack branch (before
the first AttackTarget) and the manual attack flow (ahead of the
fighting early return). The claim paces itself at the engage retry
period and repeats while the target keeps its distance; without the
flag every leg keeps its ordinary shape (pinned by the unit tests).

## The live verification (vanilla stack, elven village hunt, 6 minutes,
character level 1 to 4)

- 26 engage claims, 34 kills, zero chase-stall fallbacks, zero errors;
  the log shows the arm line ("Movement abuse channel armed") and one
  claim line per approach.
- The claim-to-kill gap is distance independent: a 924 unit approach
  killed in 9 s while a 156 unit one took 8 s - the honest run of 924
  units alone costs about 7.4 s at the 125 u/s effective chase speed
  (total at least 14 s), so the approach leg itself is 0-2 s: the
  teleport. The gap is the fight, not the travel.
- The no-flag control run (150 s): zero claim lines, 12 kills, the
  ordinary run-up - and a slower kill cycle (12.4 s per kill against
  10.2 s with the claims).

Logs: /home/z/my-project/scripts/abuse_engage_hunt.log and
abuse_engage_control.log (sandbox artifacts, outside the repo).
