<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# ROADMAP: the autonomous solo 1-60 ladder

The goal ladder of the project: a single bot character levels from 1
to 60 with zero human intervention, verified by live acceptance runs.
This file is the operational ladder every unit of work must reference.

Rules:

- A milestone is closed ONLY by a live acceptance run against the
  deployed local Mobius stack, never by "the code is written".
- The progress of the project equals the highest green milestone.
- Every unit of work references the milestone it advances; work that
  advances none is disconnected work.
- Party play, raids and sieges are the NEXT ladder (see
  `docs/project_description.md`); they are deliberately out of scope
  here: the solo ladder builds the quest, zone and skill
  infrastructure they will reuse.

## M0 - elven lands autonomy (DONE)

Autonomous hunting in the elven starting lands: engage/loot/rest loop,
multi-zone registry from real spawn polygons, auto equipment, shop
strategy with town trips, skill lessons from teachers, deleveling,
death recovery, 24/7 supervision with reconnects, the web UI and the
acceptance manager. Verified by the farm-readiness scenario
(PASSED live 2026-09-11: a fresh character reached the Spore Fungus
SW zone in full gear with learned skills and killed a mob there).

## M1 - soak proof: level 1 to 20 unattended

The same autonomy, proven continuous and measured, not just capable.

Acceptance:

- A fresh account runs `-hunt` for at least 8 hours unattended (target:
  1 to 20; the zone ladder tops out at the liren 16-19 band).
- `runs/metrics.jsonl` receives one row per run: scenario, duration,
  start/end level, XP per hour, deaths, adena, stuck events, PASS/FAIL.
- No livelock: the stagnation watch (no XP gain for M minutes, no
  position change for K minutes) logs explicit events and fails the
  scenario when it fires.
- Graceful shutdown at the end; the bot is never online without
  hunting intent for more than the patience windows of the loop.

## M2 - first profession (class transfer at 20)

The quest subsystem: NPC dialogs (html message flow), quest state
packets, quest items, the elven class transfer quest at level 20
(Elven Fighter to Elven Knight or Elven Scout).

Acceptance: a DB-injected level 20 character (built by the level
milestone scenario) completes the class transfer quest end to end
with zero human actions; the class id change is verified through the
character selection packet.

## M3 - the 20 to 40 band

Zone registries for the 20+ grounds reachable from the elven lands
(survey the Mobius spawn data for the bands; generate the same
spawn-true squares), the teachers of the second class tier, the gear
planner catalogs for the band, navigation legs through the gatekeeper
network where walking is impractical.

Acceptance: level 20 to 40 demonstrated by a continuous soak run with
the same metrics and stagnation gates as M1.

## M4 - second profession (class transfer at 40)

The level 40 class transfer quests for the chosen first class.

Acceptance: same shape as M2 at level 40.

## M5 - the 40 to 60 band

Zone registries, teachers, gear and consumables of the 40+ bands.

Acceptance: level 40 to 60 soak with the M1 gates.

## M6 - the integral 1 to 60 run

A single account, freshly created, runs unattended from level 1 to
level 60 across multiple days (the 24/7 supervisor and the growing
backoff already own the restarts; the soak metrics accumulate).

Acceptance: the full metrics trail in `runs/metrics.jsonl`, a PROGRESS
report rendering the level curve, and the final state dump (level 60,
class, gear) committed as the milestone artifact.
