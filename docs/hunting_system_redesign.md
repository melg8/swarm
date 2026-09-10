# Redesign of the hunting system: spot-anchored farming with respawn awareness and efficiency scoring

Research document, 2026-09-10. Branch `feature/proxy-server`. Author: melg8
(agent session). Source data: the generated zone registry
(`internal/swarm/hunt/zones_elven.go` — the parsed Mobius
`ElvenStarting.xml`), `internal/swarm/npcdata` (aggressive flags, aggro and
clan help ranges), the live-validated server facts of `AGENTS.md`, a
quantitative analysis of the registry (`tools/analyze_hunt_registry.py`) and
a web survey of other bot implementations (Adrenaline, L2Tower, L2Bot,
HonorBuddy) plus game AI literature.

## Why the current zone system underperforms

The user-visible symptoms and the measured numbers behind them:

1. **Visibility does not follow the zone geometry.** The Mobius world is a
   grid of `WorldRegion` cells 2048x2048 units (`SHIFT_BY 11`); a client
   receives the objects of its own region plus the 8 adjacent ones. The
   guaranteed visible radius is therefore only ~2048 units (typical ~3072
   from the region center, hard cap 4096). The generated squares have
   `Half` 1300-1900 (side 2600-3800, corner distance 1840-2690 from the
   center), so from the square center the corners of the bigger squares are
   already outside the guaranteed circle, and once the bot walks to an edge
   (a chase, a flee leg) the opposite edge sits at the median distance of
   2800 units — in the position-dependent zone where mobs flicker out of
   the knownlist depending on which region cell the bot stands in. The
   tracker only knows what the server showed it, so `ZoneHasPickable` and
   the far-target search (range 6000, beyond the 4096 cap) reason over a
   stale partial view. A "cleared" square is often not cleared — half of it
   is simply not rendered.

2. **The registry is bloated and self-overlapping.** 227 squares cover 73
   territories with 722 spawned mobs — a median of **3.0 mobs per square**.
   Squares of the same level band overlap each other by a mean of **64%**
   (median 67%, max 100%); 192 of 227 squares have more than 30% of their
   area covered by a same-band sibling. The rotation treats them as
   distinct grounds, but on the map they are nearly the same place.

3. **The rotation is scheduled against the respawn clock and loses.**
   `zoneRotateAfter` is 10 s; the elven respawn runs at 15-20 s per mob
   (measured, AGENTS.md). The median walk to the nearest same-band sibling
   is 1832 units (~14 s at run speed ~130 u/s), 42 of 227 siblings sit
   farther than the whole respawn window. So the cost of one rotation is
   10 s of waiting plus ~14 s of walking (~24 s) versus ~15-20 s of
   waiting for the square to refill itself: **the current system always
   abandons squares that would refill faster than it can walk away**.

4. **The ladder optimizes level, not income.** `PickHuntingZone` always
   takes the highest band both gates open. Two measured consequences: the
   aggressive share of the spawn mass grows with the band (0% in bands
   1-8, 86% of the 16-19 band mass is aggressive — Lirein and friends
   attack on sight), and the kill speed drops as mobs approach the bot's
   level. For a gold-oriented farm the optimum is white-green mobs a few
   levels below the bot (full item drops while the player stays within
   `mob level + 5`, full adena within `+ 8`; exp/SP stops at `+ 11` — all
   verified against the Mobius C1 `NpcTemplate`/rates code), not the
   highest band the gates allow.

5. **24% of the registry is effectively out of reach.** 55 of 227 squares
   lie more than 15 000 units from the Elven Village (the spawn file
   includes far-flung western and southern territories). The "nearest
   ground of the winning band" tie-break protects the bot only locally: a
   fresh band pick can still target a square minutes of running away.

6. **Gear gates and death caps are the wrong safety mechanism.** The
   `MinGear` gates hold a character back from bands its equipment "cannot
   pay for", and three deaths in one square demote the whole band until a
   level change. But gear points do not predict combat outcome (a bot with
   a new weapon but no armor passes the same gate), and the demotion
   throws away the whole band instead of the one bad square. The safety
   signal that actually exists is the measured fight outcome: HP lost per
   kill, flee frequency, death rate at the spot.

## What other bots do (survey)

- **Adrenaline** (the best-documented L2 bot): six zone shapes — free
  (nearest target, known drift), center-at-start + radius, fixed point +
  radius, polygon corners, **"move using path" = an ordered list of
  center+radius waypoints walked in a ping-pong**, and drawn maps with
  obstacle zones. Target preferences: aggressive mobs first, finish
  started fights, **path distance instead of euclidean distance** ("calculate
  map distance" builds the route to every candidate). The Hunter plugin:
  multi-spot profiles with level conditions, GPS routes between spots,
  return-to-spot after death.
- **HonorBuddy** (WoW): profiles are lists of **hotspots (point + radius)**
  with "kill between hotspots" and **blackspots** — areas the profile
  avoids.
- **L2Bot** (Mobius-oriented): farm radius around a point or a rectangle,
  geodata pathfinding optional.
- **Respawn tracking** is the known trick of farm teams (boss respawn
  timers in Discord) and of wrobot-style bots (timed blacklist after a
  kill). The Mobius C1 respawn mechanics make it precise:
  `RespawnTaskManager` schedules `death + rnd[min, max]`, and with
  `EnableRandomMonsterSpawns = false` (server default) the mob returns at
  (near) its previous spawn point, so the corpse position is a usable
  respawn prediction.
- **Game AI literature**: utility-based scoring for target selection
  (weighted considerations, response curves), influence maps for threat,
  DBSCAN/KDE hotspot clustering for spawn concentrations, multi-armed
  bandit (UCB/Thompson) for exploration-exploitation when picking grounds,
  steering "containment" for leashing.

## The proposed system: spots, respawn awareness, efficiency scoring

### 1. The Spot replaces the square

A **Spot** is a farm anchor with a visibility-bounded radius:

```
Spot {
    ID, Region
    AnchorX, AnchorY          // density centroid of the spawn mass
    Radius                    // <= 2048 (guaranteed visible circle)
    Mobs []ZoneMob            // composition: template, level, count, drops
    Respawn [min, max]        // seconds, from the spawn data / measured
    AggroMass, SocialRisk     // static danger inputs
    LevelWindow [min, max]    // mob level band for the [L-5, L] filter
}
```

The registry is generated by the same pipeline that today emits 227
squares (`tools/generate_hunt_zones.py`), extended with a clustering
pass: grid-density the spawn mass (512-unit cells), threshold, take
connected components, and emit one Spot per component whose radius is
clamped to 2048 — a dense territory yields several neighboring spots
(the bot ping-pongs between them, Adrenaline-style), a sparse territory
yields one spot anchored on the mass, not the polygon centroid. The
prototype run on the current registry data produced **30 spots of mass
>= 2 mobs** versus 227 squares — a 7.5x reduction of the decision space
with the same 96% spawn mass coverage.

The radius invariant directly fixes the visibility complaint: from the
anchor the whole spot is inside the guaranteed visible circle, so the
knownlist equals the spot population while the bot holds the anchor.

### 2. Respawn-aware world model (the overlay)

The tracker keeps the live knownlist (what the server shows) and the loop
adds a prediction layer on top of the static spawn data:

- every kill records `(position, templateID, deathTime)`;
- the predicted respawn of that mob is `deathTime + rnd[min, max]`
  (window 15-20 s for the elven lands), position = the death position
  (server default: no random respawn offsets);
- the spot's **expected population** at time T = live visible mobs +
  predicted respawns with `eta <= T - now`, each with its predicted
  position.

The "is the spot empty" question changes from "does the knownlist show
a mob" to "does the expected population have a mob" — a spot with three
mobs predicted to respawn within 8 s is NOT empty, and the bot waits at
the anchor instead of rotating away.

### 3. The wait-or-move decision (the core economy)

Between fights the bot runs a cheap expected-value decision:

```
bestVisible  = utility pick over visible+pickable mobs (distance,
               level preference, aggro risk)      -> engage if found
respawnETA   = min over predicted respawns near the anchor (<= ~1500 u)
if respawnETA <= waitPatience:  wait at the anchor (patience now
                                legit ~15-20 s, not 10 s)
else: walk to the densest predicted-respawn cluster of the spot
if expectedSupply(spot, horizon 60 s) < bot kill capacity * margin:
        evaluate switch: score(other) vs score(current) * hysteresis
```

with `score(spot) = expectedAdenaPerHour(spot, bot) * safety(spot,
bot) * proximity(spot, bot)`. The switch fires only when the alternative
is >25% better for a sustained window (hysteresis) and a minimum stay of
~5 minutes has elapsed — no ping-ponging, no drift.

`expectedAdenaPerHour` is bootstrapped from statics (mob density x
expected kill speed from the level gap x drop tables) and then replaced
by the measured EMA: loot value per active minute (rest time included —
hard fights internalize their own cost). The `safety` multiplier
combines the static danger inputs (aggressive share, social pack
density) with the measured death/flee rate — this **replaces the gear
gate**: an undergeared bot automatically scores worse where it takes
damage, and drifts to easier spots without any `MinGear` numbers.
Deaths still regress the spot (a 24 h EMA blacklist), but per-spot, not
per-band, and with time decay instead of "until the level changes".

### 4. Anti-drift leashing

- **soft leash**: after a fight, if the bot is > ~1500 u from the anchor
  and no visible target, the return walk starts immediately (while
  scanning — the walk legs are engaged, not dead marching);
- **hard leash**: no new fight is initiated beyond `spotRadius + slack`
  from the anchor; an in-progress chase is finished anywhere;
- the anchor itself adapts slowly: EMA of kill positions (the pack
  centroid tracks the actual spawn mass);
- fights that drag the bot out (aggro pulls) end with a return-to-anchor
  leg, never with a new target on the far side.

Drift disappears because a spot switch is an explicit economic decision
with a hysteresis margin, not the automatic "nearest sibling" rotation.

### 5. Level policy: the white-green window

The target window is `[L-5, L]` (full item drops, no penalty on the
bot's exp), with the *preference* subrange `[L-4, L-1]` where mobs die
in a few swings — the gold optimum. The engage keeps the existing hard
ceiling `L + 2` as a safety guard, but the picker biases toward the
window, replacing the "highest open band wins" ladder. The supply
analysis shows the window is never starved: for bot levels 6-20 the
elven lands hold 155-320 mobs inside `[L-5, L]`.

### 6. Fleet capacity sharing

All bots run in one process, so the spot scores share a live registry:
`score(spot) /= (1 + botsAlreadyThere)` — the swarm spreads over the
spots of the same quality instead of stacking on the nearest one. This
is the cheap version of the long-term "eyes" concept from
`docs/project_description.md`: the respawn predictions of every bot
enrich the shared world model.

### 7. What stays

Combat safety (flee thresholds, pile-up logout, clan fence), looting,
town trips, auto-equip and the shopping strategy are unchanged — the
town return leg simply targets the spot anchor instead of the zone
center. The map UI swaps the zone layer for the spot layer (active
spot, expected population, measured adena/hour, danger heat).

## Implementation plan (4 phases, each shippable)

| Phase | Content | Touches |
|-------|---------|---------|
| 1 | Spot registry generator + `hunt/spots.go` data model; registry switch; window filter `[L-5, L]`; drop `MinGear` | `tools/generate_hunt_spots.py`, `hunt/zones*.go` (rename), `hunt/loop.go` (pick call) |
| 2 | Kill/death bookkeeping + respawn prediction in `state`; wait-or-move in the engage; soft/hard leash; patience 15-20 s | `state/tracking.go`, `state/scans.go`, `hunt/loop.go`, `hunt/loop_movement.go` |
| 3 | Spot scoring (static bootstrap + EMA measurement), hysteresis switching, per-spot death regression with decay, flee stats | new `hunt/spotscore.go`, `hunt/loop_safety.go` hooks |
| 4 | Fleet capacity sharing, web UI spot panel, respawn overlay on the map | `state/registry.go`, `webserver`, `web/main.js` |

Phase 1 alone fixes the visibility geometry and the registry bloat;
phase 2 fixes the rotation-vs-respawn economy; phase 3 replaces the
gear/death gates with measured efficiency; phase 4 is the swarm payoff.

## Success metrics

- adena per active hour (per bot, EMA over sessions) — target +30-100%
  versus the current system on the same levels (the rotation walk alone
  is ~24 s per square-cycle saved);
- walking distance per kill (map units) — target < 25% of the current;
- deaths per hour — not worse than the death-cap system at equal adena;
- spot switch frequency — bounded by the hysteresis (no ping-pong);
- knownlist coverage of the active spot (fraction of expected
  population visible from the anchor) — ~100% by construction.

## Risks and open questions

- **Respawn position assumption**: the prediction layer assumes the mob
  returns at (near) its death position. First live run must verify:
  log spawn-in coordinates vs death coordinates over ~50 kills; if the
  server re-randomizes inside the polygon, the prediction degrades to
  "the territory refills uniformly" (still sufficient for the wait
  decision, weaker for corpse camping). GitLab was unreachable from
  this sandbox (HTTP 403 on every endpoint), so the live stack could
  not be deployed for the verification — the next session should retry
  `tools/swarm_fast_deploy.sh` first.
- **Adena value of drops** needs the drop tables joined into the
  registry (the shopping catalogs already parse the reference prices);
  until then the bootstrap scoring can use level-based proxy value.
- **Multi-spot territories**: dense areas emit several spots; the
  ping-pong between them must respect the same hysteresis as spot
  switching to avoid micro-rotation.
- The deleveling flow assumes a "zone" for its trigger
  (`MedianZoneMobLevel`); it re-targets to the spot anchor without
  behavior changes.
