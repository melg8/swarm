# World navigation analysis: what is missing for universal A → B routing

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

An analysis of the swarm pathfinding system against the goal of a
universal navigation tool: walk (or otherwise travel) from any point A
to any point B of the Lineage 2 C1 world. Written after porting the
geodata pathfinder (`internal/swarm/pathfind`, ported from L2Bot2.0 /
L2jGeodataPathFinder) and building the `-pathfind-test` map UI with the
geodata visualization. Every number below was measured on the deployed
Mobius C1 stack and its geodata pack (215 regions in
`data/geodata`), not guessed.

## What already works

- Connected geodata over the mainland: a route from Gludin to Aden town
  was found end to end (312 848 world units, 116 smoothed waypoints).
- Multi region search, multilayer cells (bridges, decks, interiors),
  line of sight, post smoothing, cell level visualization of the
  geodata on the map (`-pathfind-test`, the `geodata` overlay with the
  height / walls / layers modes).
- Long distance route found across the sea floor: Talking Island to
  Giran (224 871 units, 87 waypoints) - the geodata water is walkable
  for the search.

## Measured scale limits (the core problem)

Temporary test with the expansion cap raised to 40M and the region
cache to 16 (the shipped constants are 1M and 4):

| Route | Result |
|---|---|
| Gludin → Aden town (land, half the world) | found: 312 848 units, 116 waypoints, **25.2M nodes expanded, 106 s** |
| Obelisk of Victory (Talking Island) → Giran shop (across the sea) | found: 224 871 units, 87 waypoints, 6.1M nodes, 25 s |
| Orc village → Elven village | **not found at all, 40M nodes, 3 min aborted** |
| The same routes with the shipped cap (1M) | all three abort |

Conclusions:

1. The grid A* does not scale to world size: a half world land route
   costs 25M node expansions. With the shipped 1M cap the system only
   navigates between neighboring areas. A hierarchical search (HPA*
   style clustering) or a precomputed road/waypoint graph between towns
   is required for world scale.
2. The region LRU cache (4 regions, ~20 MB each) thrashes on long
   routes: the search frontier touches 10-15 regions and keeps evicting
   regions it still needs (each miss costs ~140 ms of parsing). The
   cache size must become a search parameter (pin the regions of an
   active search, or a bigger cap for the service mode).
3. Some village pairs have no geodata connection at all (Orc ↔ Elven
   village: no path even at 40M expansions). Universal navigation is
   impossible on foot alone - a meta transport layer is mandatory.

## Water movement (swimming vs walking) - a separate cost dimension

The current search treats water as ordinary ground: the ocean floor
heights are open walkable cells, so routes happily cross the sea. That
is wrong in three ways for a real walker:

1. **Speed.** Swimming in C1 is much slower than running on land. The
   A* cost model is pure distance, so a water shortcut looks
   deceptively cheap while costing several times more *time* per unit
   of distance. The cost function must become medium aware: water cells
   cost `runSpeed / swimSpeed` times more (a measurable constant of the
   client/server movement stats).
2. **Drowning.** In some directions a character can simply drown:
   Lineage 2 has a breath meter underwater, and when it empties the
   character takes damage and can die mid route. Deep crossings (the
   open ocean between Talking Island and the mainland, the deep basins
   around Innadril) may exceed the breath capacity entirely. A
   universal navigator must know depth and breath limits and refuse or
   detour such directions instead of walking the bot into a drowning
   death.
3. **Server semantics.** The pathfinder walks the ocean floor Z while a
   swimming player is validated against the water surface Z by the
   server. Whether the C1 server accepts the deep water route at all
   (water zones, swimming checks, `ValidateLocation` corrections) is
   unverified; the found sea routes must not be trusted until a live
   swim test passes.

Direction: make the water a first class terrain in the engine - detect
water cells (height below the local water level, or the water zone
data), apply the swim speed cost, forbid depths beyond the breath
budget, and otherwise route water travel through the boat edges below.

## Meta transport (the layer above geodata)

The measured Orc ↔ Elven gap proves that walking alone cannot cover the
world. The server data for the missing transport modes already exists:

- **Gatekeeper teleports**: `data/teleporters/town/*.xml` and
  `data/teleporters/others/` - 25 teleporter NPCs with 352 named
  destinations, each with arrival coordinates and a fee. This is a
  ready made graph whose edges connect otherwise disconnected geodata
  clusters. Needed: parse it into the navigation graph as
  teleport edges (cost = fee + walk to/from the NPC), so a route
  becomes multimodal: walk to the gatekeeper, teleport, walk on.
- **Boats**: `data/scripts/vehicles/BoatTalkingGludin.java`,
  `BoatGiranTalking.java`, `BoatGludinRune.java` - three routes with
  explicit waypoint paths and schedules, boarded through the wharf
  managers. Needed: model them as scheduled transport edges (the bot
  must also handle boarding, riding and disembarking packets).
- The universal A → B route planner should run over the combined graph
  (walk edges from the pathfinder + teleport edges + boat edges) and
  return a sequence of legs, each leg being a walkable path or a
  transport ride.

## Dynamic world elements

- **Doors**: 41 entries in `Doors.xml`. The geodata is static - a
  closed door is a wall to the search while the server opens it on
  demand. A passable-obstacle layer (doors, paid passages) is needed
  for interior and siege navigation; town to town routes are not
  affected.
- Mobs and players are not movement obstacles (the server does not
  validate collisions against creatures), so the static path stays
  valid under them.

## Danger model (mob spawns)

`data/spawns/` holds 103 xml files with 8 761 spawn points grouped by
territory. The peculiarity of this C1 data pack: `stats/npcs` contains
**zero** monsters with `isAggressive="true"` - verified against the
server source (`AttackableAI.isAggressiveTowards` returns
`me.isAggressive() && canSeeTarget`), so mobs here never attack on
sight; they only fight back when attacked. Walking past spawns is
therefore safe on this server unless the bot starts a fight itself.

For a universal navigator the danger layer is still required in
general: mob levels (already generated into `internal/swarm/npcdata`
together with aggroRange, 1 765 mobs carry an aggroRange value) against
the character level, guard zones for karma carriers, siege zones. The
search needs a cost overlay so it routes around territories the walker
cannot survive.

## Execution layer (the bot side)

A found path is not movement. The bot needs a walker that consumes the
waypoint list: a MoveToLocation chain over the segments, arrival
detection per segment, stuck detection with re-pathing, handling of the
server `ValidateLocation` corrections (the server validates every move
against its own geodata - the same files now, but corrections will
still happen), run/walk switching, and water behavior once the swim
rules are settled. The hunt loop already contains a minimal far loot
walk that can grow into this walker.

## Geodata coverage notes

The pack covers 215 regions; inside the C1 world tile range
(16..26 x 10..25, 176 tiles) only 7 are missing: 25_25, 26_10, 26_13,
26_17, 26_23, 26_24, 26_25 - the far east column and outer corners
(x 196 608..229 376 and beyond), which are edge ocean and do not affect
mainland routes. Inside parsed regions no further blocking holes were
found after the layer poisoning fix (the Elven village bridge case,
regression test `TestFindPathBridgeOverWater`); the Gludin → Aden route
proves the mainland is connected.

## Prioritized roadmap

1. Speed: hierarchical search or a town road graph (turn 25M nodes /
   106 s into sub second), search scoped region cache, cap as a
   parameter.
2. Meta graph: parse teleporters and boat routes, multimodal route
   planner returning legs.
3. Walker: waypoint execution, stuck detection, server corrections,
   arrival handling.
4. Water: swim speed cost, depth/breath limits, live swim verification
   against the server, boat edges replace sea walking.
5. Danger costing over the spawn data (mob level vs character level),
   doors as passable obstacles.
