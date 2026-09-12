<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# The 20-25 band survey: the hunting grounds reachable from the elven lands

The survey of the first band beyond the elven ladder (the elven lands
top out at the liren 16-19 band of `zones_elven.go`): which grounds a
level 20-25 character can actually farm from the elven lands, how it
travels there, where it buys and where it learns. Produced by T-006
(ROADMAP milestone M3 preparation); the data was extracted with
`python3 tools/generate_hunt_zones.py --survey 20 25` from the local
Mobius C1 checkout (the spawn files, the npc stats, the teleporter
data) on 2026-09-12 and cross-checked against the Java sources named
below.

The shape follows the elven zone survey (the territory/mob/transport
knowledge behind `zones_elven.go` and the hunting.md sections): the
territories with their real spawn polygons, the towns, the merchants,
the teachers, the follow-up tasks. The registry generation itself is
a separate task and waits for M1 green.

## The transport reality (the ladder leaves the elven lands)

There are **no 20-25 mobs in the elven lands at all**: the closest
band ground sits 84 000+ world units from the elven village (the
Cruma Marshlands edge), past the walking range the navigation
analysis measured for the shipped pathfinder caps. The band is
reached through the gatekeeper network (the H-002 hypothesis of the
AGENTS.md registry covers its dialog protocol):

| Leg | Gatekeeper (npc) | From -> to | Fee (adena) | Arrival |
| --- | --- | --- | --- | --- |
| 1 | Mirabel (30146) | Elven Village -> Town of Gludio | 9 200 | -12694 122776 -3114 |
| 2 | Bella (30256) | Gludio -> Town of Dion | 3 400 | 15671 142994 -2704 |
| 3a | Trisha (30059) | Dion -> Execution Ground | 1 000 | 46165 150008 -3208 |
| 3b | Trisha (30059) | Dion -> Center of Cruma Marshlands | 1 000 | 5941 125455 -3400 |
| 3c | Trisha (30059) | Dion -> Plains of Dion | 1 500 | 630 179184 -3720 |

The one-way trip to the Execution Ground costs **13 600 adena**
(9 200 + 3 400 + 1 000); the return (Dion -> Gludio 3 400, Gludio ->
Elven Village 9 200) makes a lesson round trip ~25 200 adena - the
lesson economy of the band must plan the SP buys and the book buys
into the same trips. The walking alternative is MEASURED (T-018,
2026-09-12): the geodata pathfinder finds a walk route Elven Village
-> Gludio town of 111 852 units (49 waypoints, 1.25M node
expansions, 13 s with the raised 40M cap) and Gludio -> Dion of
42 831 units (25 waypoints, 0.94M nodes - within the shipped 1M
cap), so the corridor through the Neutral Zone exists end to end on
foot. The economics keep the gatekeeper chain the practical leg
(~154 700 units of pure running, over 14 minutes at the run speed,
against the instant 13 600 adena hop); the walk is the zero-adena
fallback of a broke character and the emergency return. The shipped
expansion cap (1M) aborts the elven -> Gludio leg - the cap must
become a search parameter (the navigation analysis roadmap item 1)
before the walk leg is usable in the bot.

## The grounds

Every line below names the spawn territory (the polygon the mobs
spawn and wander inside - the same spawn-true square logic the elven
registry uses), the mob mix with the effective aggression (the
Mobius `NpcTemplate` fills `isAggressive` TRUE when the npc xml omits
the attribute - `NpcTemplate.java` line `_isAggressive =
set.getBoolean("isAggressive", true)`; the earlier "mobs here never
attack on sight" note of `docs/navigation_analysis.md` was wrong and
is corrected there), the hp/exp of one kill and the walking distance
from the teleport arrival.

### The Execution Grounds (the entry ground of the band)

Teleport: Trisha "Execution Ground" 1 000 adena, arrival 46165
150008. The grounds sit 6.6-12.1k units from the arrival. The
mandragora family is a clean 20->25 ladder with a passive start:

- Mandragora Sprout (20154) level 21 and the sprout variant (20223)
  level 20, hp 342-376, exp 588-631, **passive** - the entry mobs of
  the band, safe to approach at the fresh level 20 gear.
- Mandragora Sapling (20155) level 23, hp 410, exp 725, aggressive
  (aggro 1000) - the mid step.
- Mandragora Blossom (20156) level 25, hp 460, exp 819, aggressive
  (aggro 1000) - the top step.
- Specter (20171) level 26, hp 503, exp 872, aggressive - the
  pull-over-25 neighbor; the picker gate keeps it out until 25+.

Territories: dion02_2122_19/18 (the sprout grounds, 6 mob spawns
each), dion02_2122_21s/20s (the sapling + blossom grounds),
dion02_2122_36/35 (the blossom + specter edge, 11-12k out).

### The Cruma Marshlands edge

Teleport: Trisha "The Center of the Cruma Marshlands" 1 000 (or the
760 marsh edge variant), the grounds 7.7-8.8k out. The level 25 mob
here is passive but fights with the magic the mandragoras lack:

- Giant Mist Leech (20225) level 25, hp 460, exp 819, **passive** -
  but carries NPC HP Drain (4002 v2), MP Drain (4039 v2) and a
  Decrease Atk. Spd. debuff (4038 v3): the combat safety flow must
  expect magic damage, not only melee swings.
- Gray Ant (20226) level 26, hp 487, exp 872, passive.
- Horror Mist Ripper (20227) level 27, hp 515, exp 919, aggressive.

### The Plains of Dion

Two reaches: the northern grounds 13-17k out of the Dion town square
(walkable without further teleports) and the southern grounds 3-13k
out of the "Plains of Dion" teleport (1 500 adena, arrival 630
179184).

- Dire Wolf (20205) level 24, hp 435, exp 772, aggressive (aggro
  1000) - the aggressive 24 of both reaches.
- Monster Eye Gazer (20266) level 25, hp 507, exp 819, passive.
- Kadif Werewolf (20206) level 25, hp 460, exp 819, passive (the
  southern reach only).
- Monster Eye Destroyer (20068) level 26, hp 536, exp 872,
  aggressive; Glass Jaguar (20250) level 27, passive, mixes into the
  far southern squares.

### The Wasteland edge (Gludio side)

The Monster Eye Watcher (20067) level 25, hp 507, exp 819, passive,
in squares mixed with the aggressive Lesser Basilisk (20070) level
27 and Basilisk (20072) level 28. The nearest square
(gludio04_1923_06) is 7.6k from the Plains of Dion teleport - a
band walk, not a separate leg. The deeper wasteland squares (30-45k
out) and the Turek orc camps (77k+ walking from Gludio) are out of
practical reach of this band and stay out of the survey scope.

## The town: Dion

The service town of the grounds (15671 142994, the Trisha teleport
target). The merchants the band needs (the buylist ids are the file
names of `dist/game/data/buylists/`, the positions from
`spawns/Dion/DionNPCs.xml`):

| Merchant (npc) | Shop | Buylists | Position |
| --- | --- | --- | --- |
| Sabrin (30060) | weapons (fighter + mystic) | 3006000, 3006001 | 17999 144484 |
| Casey (30061) | armor (fighter + mystic) | 3006100, 3006101 | 17948 144560 |
| Sonia (30062) | jewels + spellbooks | 3006200, 3006201 | 19313 146229 |
| Lara (30063) | grocery (arrows, potions, scrolls) | 3006300 | 19223 146228 |

The fighter weapon list (3006000) carries the NG-to-D transition
line (Long Sword, Falchion, Bastard Sword, the spear and bow
families); the armor list (3006100) the Bone/Bronze/Mithril
breastplate and gaiter lines, the shields, gloves, boots and helmets;
the jewel list (3006200) the D-grade necklace/earring/ring set. The
gear scoring of the band (what to buy at which wallet) is a
follow-up task - the survey only maps the counters.

## The teachers: the elven village stays the school

The class skill trees of the first profession (ElvenKnight classId
19, ElvenScout classId 22 - the M2 choice) land at level 20 and 24
(`stats/players/skillTrees/1stClass/ElvenKnight.xml`,
`ElvenScout.xml`), and their teachers stay the elven village masters
Ellenia (7155) and Cobendell (7156) - the `npcdata.skillTeachers`
map already covers classes 18-24, no new teacher ground opens. The
lesson flow of the band therefore rides the full teleport round
trip: grounds -> Dion -> Gludio -> village -> back (~25 200 adena
plus the SP costs - 1 400-8 800 SP per skill).

The spellbook gap the band adds: the level 24 lessons need Cure
Bleeding (book 1379), which the elven village book trader Creamees
(30149, buylist 3014901) does NOT sell - the elven list covers only
Charm (1513) and Poison Recovery (1377). Dion's Sonia (3006201)
sells all three. The lesson trips of the 24th level must route the
book buy through a Dion stop (or carry the book ahead from an
earlier trip).

## Follow-up tasks

1. **The gatekeeper teleport flow** (M3, the H-002 verification
   first): read `Teleporter.onBypassFeedback`,
   `RequestBypassToServer`, `NpcHtmlMessage` and
   `TeleportToLocation`, then live-drive Mirabel -> Gludio -> Dion
   with a bot and record the exact packet chain (the bypass command
   strings, the html dialog flow, the fee deduction packets, the
   arrival `TeleportToLocation` the Appearing answer must follow).
2. **The 20-25 zone registry generation** (M3, after M1 is green):
   extend `tools/generate_hunt_zones.py` with the Dion spawn sources
   (CrumaMarshlands/ExecutionGrounds/PlainsOfDion) and generate the
   spawn-true squares of the band in the same shape as
   `zones_elven.go` - the survey territories above are the exact
   input polygons.
3. **The band gear catalogs** (M3): the D-grade weapon/armor/jewel
   catalogs of the Dion merchants (buylists 3006000-3006300) wired
   into the gear planner with the band price brackets and the
   sell-first rules of the shop strategy.
4. **The walking leg verification** (M3, done as T-018, 2026-09-12):
   both routes found - Elven Village -> Gludio 111 852 units
   (aborts at the shipped 1M cap, 1.25M nodes at the raised cap) and
   Gludio -> Dion 42 831 units (within the shipped cap). The
   teleport-only assumption is refuted as a hard claim; the
   gatekeeper chain stays the practical leg, the walk is the
   zero-adena fallback.
5. **The lesson trip book routing** (M2/M3): the town trip planner
   learns the Dion book stop for the books the village does not sell
   (Cure Bleeding) - the shopping strategy owns the routing.
6. **The band acceptance scenario** (M3): a DB-injected level 20
   character (the level milestone scenario of T-003) rides the
   teleport chain to the Execution Ground and kills its first
   mandragora sprout - the M3 mirror of the farm-readiness gate.
