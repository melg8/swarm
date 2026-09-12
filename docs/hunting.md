# The autonomous hunt: combat, zones, gear, shopping, deleveling

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The growth subsystems of the autonomous hunt (all unit tested; the
design goal is per-class and per-region extension). The `-hunt` flag
of `cmd/swarm` enables the whole machine; without it the loop runs in
the manual only mode (commands execute exactly the same way, the
autonomous hunting, town trips and deleveling stay off, the village
restart after a death still works).

The four pillars, each detailed in its section below:

- **Auto equipment** (`internal/swarm/gear`, `hunt/equip.go`) wears
  every equippable item the loot and the buys produce.
- **Shop strategy** (`gear/shopping.go`, `hunt/shopping.go`,
  `docs/shopping_strategy.md`) plans and executes the purchases.
- **Multi-zone hunting** (`hunt/zones.go`, the generated registry
  `hunt/zones_elven.go`) climbs the mob level ladder of the region.
- **Spot-anchored hunting** (`hunt/spot*.go`, the generated registry
  `hunt/spots_elven.go`) replaces the ladder on the elven lands:
  visibility bounded anchors, respawn awareness and measured scoring
  (the redesign of `docs/hunting_system_redesign.md`).

Extension path: a mage class implements `gear.Profile` (mAtk weapons,
robe preference - the planner, the strategy and the trip execution
stay unchanged), a new region adds its `townMerchants` list, its zone
registry entries and its tax rate. The elven deployment is the
reference wiring of all three (`main.go`:
`SetHuntingZoneRegion("elven")`).

## The hunt loop (internal/swarm/hunt)

`internal/swarm/hunt` runs a small state machine (engage -> loot ->
engage) that attacks the closest attackable npc inside the hunting
square, picks up the drops around the corpse and destroys junk
inventory items (RequestDestroyItem 0x59) when the slots reach 70% of
the 80 slot limit or the weight reaches 75%, so a long living bot
never litters the server. The destroy cleanup is the last resort only
- the normal overflow handling is the town trip below, and the cleanup
is suspended while a trip runs so it never destroys what the shop
would have paid for.

- **Attack start**: the Mobius AttackRequest 0x0A has double click
  semantics (AttackRequest.runImpl) - the first request for a new
  target only selects it (NpcClick.onAction -> setTarget, answered
  with MyTargetSelected), the repeated request for the already
  selected target resolves to onForcedAttack and starts the fight. The
  hunt loop therefore sends the selecting request
  (GameClient.AttackNearest), then re-sends the target once per second
  (the PlayerActionFloodProtector interval; GameClient.AttackTarget)
  until the tracker sees the character engaged with it
  (state.Bot.SelfEngaged: the chase MoveToPawn, Attack or
  AutoAttackStart broadcasts set the fighting target); an engage that
  never goes fresh is dropped by the stuck timeout or recovered by the
  blind engage walk (see below). The whole flow is covered end to end
  by the fake server in internal/swarm/connection/hunt_flow_test.go.
- **Target death and idle chaining**: the server never clears the
  selection of a killed target (MyTargetSelected only answers new
  selections; the removal path only broadcasts the own
  TargetUnselected), so the tracker clears the character target itself
  when the target dies, is removed or is unselected, and the hunt loop
  only adopts the server side target id while it is alive - otherwise
  the stale dead id re-enters the loop every tick and the next target
  is never selected.
- **Loot approach**: far items are reached with an explicit walk first
  (GameClient.WalkTo sends the client MoveToLocation 0x01 packet, the
  ground click of the official client, in mouse mode) and the click
  (Action 0x04) starts at 60 units: the server AI covers the last
  stretch and executes the pickup (StopMove to self, GetItem broadcast,
  InventoryUpdate), the walk keeps the approach smooth on the map.
  Failed pickups (protected or unreachable items) are skipped for 30
  seconds after a 20 second attempt.
- **Cadence and rest**: the loop decides on a 250 ms cadence, chains
  the next target immediately while the character HP is at or above 60
  percent (rate limited to one player action per second for the flood
  protector) and rests with a logged reason below it. Resting sits the
  character down below 60 percent (RequestActionUse 0x45 action 0,
  GameClient.ActionSitStand - the sitting regeneration is faster) and
  stands up at 90 percent; the toggle is confirmed by the
  ChangeWaitType 0x3F broadcast (state.Bot.SelfSitting) and a repeat is
  only sent when the flip never happened, so a lost packet can never
  leave the character toggling between sit and stand. While the
  character is under attack (Bot.SelfUnderAttack) the loop keeps
  fighting instead of resting, and a dead character (Bot.SelfDead:
  CUR_HP 0) restarts at the nearest village automatically
  (RequestRestartPoint 0x6D type 0, GameClient.RestartAtVillage,
  retried every 5 s until the server revives it) so a 24/7 session
  survives a death.
- The nearest target ranking uses the projected current positions of
  moving npcs (state.projectedPosition), not the stale movement packet
  starts. The hunt chain behavior is covered by
  internal/swarm/hunt/loop_test.go (next target selection after a
  kill, rest gate, walk to far loot, sit/stand transitions, no sitting
  under attack) and the tracker clearing by
  internal/swarm/state/tracking_test.go.

- **Stagnation watch (hunt/stagnation.go)**: the M1 soak proof
  requires a silent livelock to become a loud line, so the tick
  publish defer observes two progress values every tick - the
  character experience and the exact standing cell. No experience
  change for `stagnationXPWindow` (20 min, calibrated over the
  measured town trip rounds: the weapon run round takes 2.2 min, the
  full farm readiness round 14m54s) or the same position held for
  `stagnationPositionWindow` (10 min) logs one explicit event per
  window (`Hunt: stagnation: no experience change ...` / `Hunt:
  stagnation: position held ... at X Y Z`), carrying the loop phase so
  the log explains what the state machine was doing while frozen. The
  lines route through `Loop.logf` (console, tracker event log, the
  web UI log tab) and the stall ages ride the state dump diagnostics
  (`xpStallForMs`, `positionStallForMs`). The windows re-arm after
  each event (a frozen character logs its stall every window, not
  every tick) and reset on the offline gaps, the manual only
  sessions and the missing position (a relogin is not a livelock).
  The unit tests live in hunt/stagnation_test.go (the seeded
  baselines of the loop tests are the clock seam: fire, re-arm,
  refresh-while-farming, manual/offline quiet, the tick path
  coverage, the event feed surface, the diagnostics wiring).

## Combat safety (hunt/loop.go + the constrained target search of state)

- The engage never initiates on mobs above the character level + 2 or
  on social pulls - a mob whose clan mates stand within its
  `clanHelpRange` (mirroring the Mobius AttackableAI clan call: the
  ALL clan of the attacked mob matches everything, a 600 unit z
  distance blocks the assist, the projected positions measure moving
  packs, a 200 unit margin covers the mates wandering mid fight).
- The aggro answer (2026-09-10): a mob that already holds the
  character as its target (its swings or its chase - both carry the
  character as the mob's target id, `NearestAttacker`) owns the
  targetless pick: a healthy character with a winnable attacker
  (inside the engage level ceiling, `attackerEngageable`) engages it
  at once - the forced attack request fires on the same tick and the
  fight answers the aggro where it stands - while anything else (a
  hurt character, an attacker above the ceiling) keeps the defensive
  flow, the standard escape walk with its logout budget. The town
  trips answer the same way (`interruptTripForAttacker` runs first
  in the trip tick): a mob on the walking seller drops the trip
  through the soft reset (no cooldown, the junk, the books and the
  sold proceeds survive) and fights or runs instead of dragging the
  chase through every camp on the route - the reported pile up
  deaths of the walkers. `adoptOutZoneFight` checks the same
  winnability before it finishes an attacker outside the zone: an
  unbeatable chase switches to the escape instead of a losing fight.
- A running fight that turns into a death risk is fled: under 25%
  health, or under 60% while the target holds a 25+ percent health
  lead, the target is dropped with a two minute skip and paced escape
  legs open distance (toward the zone center when the straight line
  leaves the square); a target one swing from dead is finished
  instead. A hurt character under attack keeps fleeing instead of
  standing in the blows.
- Two triggers end the session: critical health (12%) under attack, or
  a social pile up - two or more living attackable mobs holding the
  character as their target (`SelfAttackerCount`; a chasing mob
  carries the same target id as a swinging one, the character's own
  engagement never counts). One last escape leg keeps the offline
  character moving through the 15 s combat stance the server holds it
  in, the `RequestLogout` packet plus the socket close follow, and a
  30 s login cooldown the supervisor (`runBotForever`) honors before
  the next session (the tracker carries the cooldown across sessions -
  half a minute covers the combat stance plus the mob reset walk home
  without idling the farm for minutes).
- Entering a hunting zone engages the first valid target the entry
  radius offers (`engagesOnZoneEntry` of the return phase), and a
  targetless hunter patrols toward the zone center after a 6 s
  patience window instead of standing still. A big square whose pack
  sits outside the engage radius (1500) is walked toward directly:
  `walkToFarTarget` searches the whole zone for the nearest valid mob
  and follows one paced leg per second, so the hunter closes on a far
  pack instead of standing central in an empty radius (the rotation
  only fires on a fully empty square).
- A flee that never shakes the chase ends the session too: one escape
  episode older than `fleeLogoutAfter` (20 s) logs the character out
  (a recovered health or a fresh fight clears the episode) - the
  relogin after the pause resets the mob aggro instead of the bot
  running from the pack forever. A zone switch drops the remembered
  farm spot of the previous square (the returns aim at the new center
  until a fresh spot is remembered inside it).

## Blind engage recovery (walk around the obstacle, then switch)

A small obstacle (a column) between the character and its target locks
the plain engage - `Creature.onForcedAttack` arms the ATTACK intention
WITHOUT a line of sight check, `Creature.doAttack` then answers every
swing with SystemMessage 181 ("Cannot see target.") while keeping the
intention armed, the attack stance never stops (no swing lands, no
AutoAttackStop arrives), so SelfEngaged stays true forever and the old
stuck timeout never fired (observed live: a 53 minute stall at 115
units with the refusal spam every ~3.5 s, walk plan empty, no events).

The recovery (hunt/loop_los.go) reads the refusal through the tracker
(ApplySystemMessage records the last CANNOT_SEE_TARGET arrival in
SelfCannotSeeTargetAt) and has two levels:

- Level A samples a ring of melee range standing points
  (blindMeleeRadius, 16 directions) around the target, keeps the
  nearest one with a bot side geodata sight line (Navigator.LineOfSight
  over pathfind.Engine), plans the geodata path to it and walks it with
  paced ground clicks (the attack re-requests stop while the walk runs
  - an attack request would replace the walk intention and cancel the
  detour).
- Level B drops the target and skips it for blindSkipDelay (1 min)
  when no navigator or vantage point exists, the path search fails, the
  walk misses blindWalkBudget (15 s) or blindMaxAttempts (2) attempts
  could not clear the block - the next pick selects a different mob,
  which also replaces the stale server side selection.

The arming of both levels survived the phantom chase of the refused
attack (the 2026-09-12 03:25 dump): the server AI of an armed ATTACK
intention keeps broadcasting the character's OWN MoveToPawn chase steps
(`PlayerAI.thinkAttack` -> `maybeMoveToPawn` -> `startFollow`) while
every `doAttack` of the same intention fails the `canSeeTarget` check
and answers "Cannot see target." - the chase stream refreshed the
tracker's `CombatActiveAt`/`FightingTargetID`, so `SelfFighting` read
true in a livelock, the fighting branch re-anchored the engage clock
past every refusal (holding the stuck timeout and the detection window
away forever) and the detection gate itself refused to arm while the
fight view looked fresh. The bot stood 80 units from its target for
minutes with the refusal spam every ~3 s, a Power Strike cast at the
invisible mob every 15 s, no walk, no switch, no landed blow. The fix
is an ordering rule: the refusal being the NEWEST fight activity
(nothing landed or stepped after the server said "cannot see") marks
the obstructed engage even while the chase view is fresh - only
activity strictly newer than the refusal (a swing or a chase step that
landed after it) proves the sight line cleared, both for the detection
(`blindEngageBlocked`), the standdown of a running recovery
(`fightClearedRefusal`) and the engage clock re-anchor of the fighting
branch. Pinned by hunt/round59_repro_test.go against the exact dump
scene.

The stuck timeout itself now measures the FRESH fight view (the gate is
!SelfFighting, not !SelfEngaged) and a running fight that progressed
past the last refusal re-anchors the engage clock, so a stale attack
stance without refusals still trips it after 12 s of no fight packets;
while the blind recovery is armed the timeout stays held (the recovery
manages its own budgets). Covered by hunt/loop_los_test.go
(reposition walk, arrival re-engage, both switch levels, retry budget,
stale stance timeout, timeout hold, attempt scoping, fresh fight
guard) and the state tracker test of the refusal recording.

## Spot-anchored hunting (hunt/spot*.go, hunt/spots_elven.go)

The spot mode replaces the square zone ladder of the elven lands (the
redesign of `docs/hunting_system_redesign.md`; `main.go` wires it
through `SetHuntingZoneRegion("elven")` -> `SetHuntingSpotRegion`).
The measured failures of the squares drive it: the 2048-unit
WorldRegion grid makes only ~2048 units of knownlist guaranteed, so
the 1300-1900-half squares lose their far corners from the knownlist
once the bot walks to an edge; the same-band squares overlap by 64
percent (227 squares over 73 territories, a median of 3 mobs each);
the 10 s rotation always abandons grounds that refill in 15-20 s.

- **The registry** (`tools/generate_hunt_spots.py` ->
  `hunt/spots_elven.go`, 71 spots): the territory anchors cluster by
  grid adjacency, every cluster splits until it fits the visibility
  circle, the spot is the mass weighted centroid with `Radius` clamped
  to 2048, the species composition and the per-species respawn
  windows (the Mobius spawn XML when the tree is present, the measured
  15-20 s elven window otherwise). The leash square INSCRIBES in the
  circle (`leashHalf` = radius / sqrt(2)): every point the engage
  square covers stays inside the guaranteed knownlist of the anchor,
  the "cleared ground is not cleared" failure cannot happen by
  construction.
- **The white-green window**: the target search filters on
  `[max(1, L-8), L+2]` (the C1 full-adena edge mob level + 8, the hard
  engage ceiling stays `targetMaxLevelSlack`), the engage priorities
  bias `[L-4, L-1]` strongest (full loot, few swings - the gold
  optimum), `[L-5, L]` medium, the wide window tail weakest.
- **The respawn overlay**: every kill records the corpse position and
  the window midpoint of its species as the predicted respawn (the
  server default keeps spawns near the death place); the wait-or-move
  economy holds a ground whose predicted respawn lands within
  `spotWaitPatience` (20 s) and drifts the hunter toward the predicted
  corpse position - waiting beats the ~14 s walk to any equivalent
  ground.
- **The economy**: `spotScore = value x safety x proximity /
  (1 + occupancy)`. The value starts as the bootstrap prior (window
  mass x respawn turnover x level-priced kills) and becomes the
  measured adena per active minute after 5 minutes on the ground (rest
  included, town trips excluded); the safety folds the static
  aggressive share and the decayed per-spot death heat (half-life 30
  minutes - no band demotion, the heat fades on its own); the
  occupancy divides the score across the fleet (`spotHub` shared by
  every loop of the process, the swarm spreads over the spots of one
  quality). A voluntary switch needs the alternative 25 percent above
  (`spotSwitchMargin`) after a 5 minute fair trial
  (`spotMinStay`); a starved ground (60 s of emptiness, no pending
  respawn) leaves after 90 s. A death re-picks at once: the fresh heat
  crushed the score of the ground, the village restart aims at the
  easier alternative.
- **The views**: `ZoneView` carries the spot economy (kind "spot",
  radius, respawn window, expected population, measured rate, death
  heat, next respawn ETA, occupancy, kill centroid EMA); the web map
  draws the spots as circles with the economy labels, and
  `tools/visualize_hunt_spots.py` renders the standalone interactive
  map of the registry (or of a live state dump, or of a simulated
  session: `--simulate N`) for offline study.

The legacy square system stays for the manual `SetHuntingZones`
setups and the generated registries of the regions that have not
migrated yet; `SetHuntingZones` stands the spot mode down and
`SetHuntingSpots` stands the zone mode down - one loop, two registries.

## Multi-zone hunting (hunt/zones.go, hunt/zones_elven.go)

The hunting squares derive from the real spawn polygons - a Mobius
territory spawns its mobs at uniformly random points of its polygon
(`NpcSpawnTerritory.getRandomPoint`) and the random walk stays inside
it (the wander target must pass `isInsideZone`, `MaxDriftRange` 300
leashes the rest), so a square anchored on a "cluster centroid" misses
most of the ground (the measured spawn mass coverage of the old hand
placed thirty squares was 18 percent). `tools/generate_hunt_zones.py`
parses ElvenStarting.xml plus the npc stats and generates 227 compact
squares (1300-1900 halves, one square for a small territory, a density
adaptive grid partition for a big one, same-band sub-territories folded
into their parents) covering 96 percent of the spawn mass in the ten
mob level bands (keltirs 1-3 gear 0, wolves 3-4 gear 20, raiders 4-6
gear 50, goblins 5-7 gear 70, grunts 7-8 gear 100, fighters 8-10 gear
140, lieutenants 9-12 gear 180, leaders 11-13 gear 220, elders 12-14
gear 260, spiders 13-16 gear 300, lirein 16-19 gear 380).

- Every square carries the full mob list of its territory (`ZoneMob`
  with a level-sorted `Priority`), so the engage farms every species of
  the ground - the priorities bias the pick by `targetPriorityBias`
  (200 units per point) through `state.Bot.NearestAttackablePreferred`,
  tilting the fight toward the exp richer mobs while a far preferred
  mob still loses to a doorstep one.
- `PickHuntingZone` gates on level AND gear points - a band opens only
  above its top mob level plus the lead (`zoneLevelLead` 1), so the
  character always hunts mobs 1-2 levels below itself instead of
  engaging mobs above its own level - keeps the current zone of the
  winning band (the 30 s re-pick never bounces between same-band
  grounds) and takes the nearest ground of an open band; a character
  below every band (or under a death cap that closed the ladder) falls
  back to the starter band contest (the nearest ground of the first
  band, the current ground keeps its post).
- A cleared-out square rotates: no attackable mob inside the square
  that passes the engage level ceiling for 10 s while the hunter stands
  central (the `ZoneHasAttackableBelow` reading - a square whose
  survivors all sit above the max target level is as good as empty)
  moves it to the nearest non-cooling sibling of the same band, and the
  rotated-away square keeps a 40 s empty cooldown (`zoneEmptyCooldown`)
  so the rotation sweeps forward through the band (A to B to C) instead
  of ping ponging back into the square it just left (A to B to A) - the
  fights, rests, walks and town trips reset the timer.
- The death regression: three deaths in one square demote its whole
  band, the ladder caps below it until the level changes (a level up or
  a delevel resets the bookkeeping; delevel-phase deaths never count);
  deaths count against the square they happen in (the position lookup -
  an emergency logout death lands on a fresh session before any zone
  pick), a manual zone override dies with the demotion.
- A manual zone selection stops the walks aimed at the old square
  (`stopForZoneSwitch`: a manual move, a town trip walk and a zone
  return leg are cancelled, one walk request to the current spot
  replaces the running server walk; the deleveling refuses the stop
  like every movement command, the selling stop keeps running and its
  return leg re-targets the new zone). The map draws every zone and
  the floating collapsible zone panel of the map switches zones
  manually (the `zone` command, index in the Count field; the override
  holds until the character outgrows the band or dies it out) - see
  docs/webui.md.

## The Dion 20-25 band registry (hunt/zones_dion.go)

The first registry beyond the elven ladder: the hunting grounds of
the 20-25 character band (the M3 survey of
docs/band_20_25_survey.md) generated by `tools/generate_hunt_zones.py
--dion` from the three Dion spawn sources (ExecutionGrounds,
CrumaMarshlands, PlainsOfDion) in the shape of `zones_elven.go` - the
same polygon partition into spawn-true squares, every square carrying
the full mob list of its territory, the territory filter keeping the
grounds with at least one mob of the 20-25 window (the survey
territory lists, the over-25 neighbors of a ground stay in its mob
list). 75 squares over 25 kept territories (the two same-band sub
polygons fold into their parents), 96 percent spawn mass coverage,
the same compact halves (1400-1800).

The five band windows continue the elven ladder onto the D-grade
dress stages (the gear gates calibrated against the
`gear.TotalGearPoints` probes of the buyable dress: the NG full dress
284, the Falchion dress 292, Bastard+bone 312, the partial mithril
step 341, the full D dress 391):

| Band | Mob levels | Gear gate | The grounds (the survey tables) |
| --- | --- | --- | --- |
| Sprout grounds | 20-21 | 280 | the passive mandragora sprouts of the Execution Grounds - the entry ground of the band |
| Northern plains | 23-24 | 290 | the aggressive dire wolf grounds of the walkable Dion reach |
| Sapling and blossom | 24-25 | 310 | the mandragora sapling+blossom, monster eye gazer and kadif werewolf grounds |
| Specter and destroyer | 25-26 | 340 | the specter, gray ant and monster eye destroyer grounds |
| Cruma and jaguar | 26-27 | 380 | the cruma marsh edge (leech/ant/ripper) and the glass jaguar squares |

The starter fallback anchor is the Trisha "Execution Ground" teleport
arrival (46165 150008): a character below every band (a fresh level
20) hunts the nearest sprout square of the arrival through the
starter contest - the survey's "safe to approach" entry - and the
ladder opens the bands at their top mob level plus the lead (the
wolves at 25, the sapling+blossom grounds at 26, the specter grounds
at 27, cruma at 28). `zones_dion_test.go` pins the survey tables as
the acceptance reference: the thirteen mob species at their table
levels, the sprout/cruma ground mob sets, the walking distances of
the survey anchors (the sprout squares within 10k of the execution
arrival, the cruma squares within 12k of the cruma center, every
square within 18.5k of a teleport arrival), the picker ladder on the
dress stages and the death cap.

A deployment selects the band explicitly through
`SetHuntingZoneRegion("dion")`: the loop installs
`DionHuntingZones()` (the zone registry stands the spot mode down)
and records the region for the gear catalog selection of the shopping
trips (`shopCatalogForRegion` selects the Dion merchants at the 20
percent Dion tax, hunt/shopping.go of T-017). The default elven flow
is unchanged. The survey protocol gates the production band entry on
M1 green (the 8 hour soak): the registry, the picker gates and the
catalog selection are the data the M3 band acceptance scenario
consumes.

## Auto equipment (internal/swarm/gear, hunt/equip.go)

The melee fighter profile scores every equippable item (weapon = pAtk x
attack speed, armor = pDef, jewel = mDef, shield = expected block
value; bows score zero for melee), `NextUpgrade` plans the next
strictly improving use item request against the tracked paperdoll
(empty slot fills, strict slot swaps, the pair swap through freeing the
weaker jewel, the two hand weapon and one-piece family guards) and the
hunt loop executes one action every 2 seconds behind the shared
confirmation gate of the manual inventory commands, so the paperdoll
stays optimal after every loot, buy and death event.
`gear.TotalGearPoints` summarizes the equipped gear for the zone gates
(weapon damage per hit plus defenses).

A looted or bought item the bot is about to wear never becomes junk
(`gear.PlannedEquips`, the planned equip keep set): the simulation's
pending equips - the looted upgrades waiting for their paced use item
request, the buy arrivals, the better halves of pair swaps mid flight -
are excluded from both the shop sell list (`SellableItemsExcluding`)
and the overflow destroy list (`DestroyableItemsExcluding`). Only the
real junk sells: the duplicates beyond one copy per slot, the looted
downgrades and the displaced weaker halves of pair swaps. Without the
keep set the first sell batch could race the pair swap window and eat
the very jewel the bot was putting on, and the overflow destroy (which
ranks gear drops before stackables) could destroy a fresh upgrade for
bag space.

## Shop strategy and the town trips

The strategy reasoning, prices and the level journey comparison live
in `docs/shopping_strategy.md` - read it before touching the planner.
The short form:

- The phased planner buys in the strategy order: **the weapon
  milestone first** (the top affordable strict upgrade - the top-tier
  guard keeps the cheaper rungs of the hand ladder out whatever their
  value per adena), **the pdef maximizing armor set second** (one
  piece per armor family per trip, chosen by the exhaustive
  enumeration of the per family efficient frontiers inside the
  remaining budget - a rich wallet buys the advanced pieces directly,
  a poor one fills many slots with the cheap offers, the set
  maximizes the summed pDef either way), **the basic jewel floor
  third** (the cheapest offer of every empty slot, both halves of the
  pairs - the jewels never upgrade: the starting locations barely
  attack with magic) and **the shield with the leftover**. It never
  buys what the inventory already carries and it respects the adena
  budget plus the sell credits of the displaced pieces; **one item
  per paperdoll slot per trip** - the next trip re-plans from the
  reached paperdoll.
- The town trips (internal/swarm/hunt/town.go) trigger when the
  inventory passes 50% of the slots or 50% of the maximum weight. A
  trip start never interrupts a fight: `fightBusy` (a living target, a
  pending loot pickup or an incoming hit) holds it until the
  between-fights window, and a resting character is stood up first
  (the server refuses move requests while sitting; the stand toggle
  shares the pending transition gate with the rest logic).
- **The weapon leads every trip that buys one** (the 2026-09-11
  bare-handed report): a plan with an affordable weapon purchase
  routes its sell stop to the weapon's merchant (the junk sells at
  any merchant), so the sell-first of the replaced weapon and the buy
  share ONE stop and the replacement lands seconds after the sale -
  the old order sold the weapon at the nearest merchant and walked
  the village for the replacement, and every trip killer in between
  (a stuck teacher leg, an attacker interrupt, a merchant no-show)
  left the character bare-handed. A character with NO weapon at all
  runs the weapon errand (`weaponlessRunWanted`: no profile
  usable weapon in the inventory and an affordable weapon in the plan
  - see `gear.HasWeapon`): the run carries the learning stops too
  (the 2026-09-12 one town visit rule - the weapon stop runs FIRST,
  so a stuck teacher leg can no longer strand a bare-handed
  character: the weapon is bought and worn before the teacher leg
  ever runs), the retry cooldown shortens to 45
  seconds (`weaponRunCooldown`) instead of the five minute trip
  cooldown. While the weapon run is pending the engage holds its
  fresh target picks (`logWeaponWait` paces the hold line) and the
  zone entry engage of the return leg skips the same way - the fists
  land 2 damage and nothing outranks fixing that; the attacker self
  defense answer stays armed whatever the weapon state is. A wallet
  that cannot afford any weapon keeps farming: the gate only holds
  when the plan offers a weapon, so a fresh bot still punches
  keltirs until the wallet crosses the cheapest offer.
- **One town visit buys everything** (the 2026-09-12 acceptance
  round): the gear stops distribute first (`planShoppingStops`), the
  learn stops close the trip behind them (`planLearnStops` at the
  sell stop) - the book stop merges into the gear stop of its
  merchant when they match (the jewel trader Creamees sells both the
  basic jewels and the spellbooks, one visit buys them all) and the
  teacher walk follows, so a single visit buys the weapon, the armor,
  the jewels, the books and teaches the lessons. The gear plan
  reserves the spellbook budget out of its planning wallet
  (`Loop.pendingBookBudget`: the books of the learnable lessons the
  inventory does not carry yet, at the town tax price), so the
  aggressive armor spending can never starve the books.
- The trip **sells all the accumulated junk first** (the selling ends
  when nothing sellable is left, not at the 50 percent trigger),
  **sells the replaced gear before the buys** (a planned purchase that
  displaces an equipped piece carries its `SellFirst` object ids and
  its sell credit of referencePrice/2 - the planner counts the credit
  toward the budget, so the character shops for a replacement as soon
  as the adena plus the proceeds cover it instead of hoarding the full
  price; the trip unequips the displaced pieces - they become plain
  sellable candidates the junk flow or the replacement batch sells -
  and waits out their removal before re-planning, so the fresh adena
  of the sale funds the buy; the auto equipment stays suspended over
  that window so nothing re-equips a piece bound for the merchant),
  re-plans with the fresh adena and walks to every merchant of the plan
  (one buylist per transaction request, 11 second pacing). The auto
  equipment runs during the trips so the purchases are worn at the shop
  already. The catalogs are generated from the Mobius buylists
  (`tools/generate_shop_catalogs.sh`, keyed by packet template id).
- **The gear debt** (the 2026-09-12 04:58 pantsless dump): the
  sell-first step is one-way - a sale that lands while its
  replacement buy fails (a silently refused request, a merchant
  no-show, a walk abort, a session death the relogin resumed into
  the return leg) strands the slot, and every trip exit used to end
  the trip as a success. Now the trip start snapshots the worn
  slots (`Loop.snapshotTripGear`) and every exit (`endTownTrip`,
  the interrupt `resetTownTrip`) compares the reached paperdoll
  against it (`Loop.gearDebtCheck`): a slot that was occupied, sits
  empty and whose piece is gone becomes gear debt - the map entry
  carries the lost item id, the arming logs the wound ("the trip
  left the legs slot empty - the Leather Pants it started with is
  gone") and the debt shortens the trip cooldown to the gear run
  window (`gearDebtRunWanted`, the 45 s window of the weapon run)
  until the refill dresses the slot (a log line clears the entry).
  The debt never justifies a trip on its own - the ordinary
  triggers fire the refill the moment the plan affords it, on the
  short cadence. The state dump names the holes the equipment
  section used to hide: the "empty slots:" line below the worn
  pieces (the report's bot farmed the Kaboo woods without its legs
  armor and the dump printed nothing about it). Pinned by
  hunt/round60_repro_test.go and the live "gear-gap" acceptance
  scenario (acceptance/gearGapReset: the exact dump character must
  buy its legs armor back).
- The path plan comes from the pathfind engine through the
  hunt.Navigator interface (set in main.go with hunt.NewNavigator from
  the auto detected geodata directory; without geodata the bot hunts
  without trips). A waypoint follower walks the smoothed path with
  ground click walks (one per 2 s; an intermediate waypoint counts as
  reached within 50 units - a bridge ramp entry or a detour turn must
  be walked through, not seen from the side - and the final waypoint
  within the 150 unit trip arrival radius; a waypoint the character
  already passed ON the route skips ahead through the projection pass
  test, while a character beside the route keeps targeting the
  waypoint it missed). Every skip is gated on the walkable line: the
  straight line from the actual standing cell to the successor
  waypoint must pass the geodata line of sight, else the waypoint
  stays the target until walking onto it re-opens the line - the
  server (the deployment runs PathFinding = 0) validates every click
  as a straight line and cancels a move whose first step hits a
  closed wall at the character's own position, and the 2026-09-11
  teacher walk stuck exactly that way (the follower skipped the 16
  unit ramp steps of the trainer plaza approach inside the 50 unit
  pass radius and clicked the plaza waypoint through the railing;
  every re-path reproduced the identical route, the budget burned
  and the trip aborted before the teacher - the lessons never
  landed). Every click is gated through the server click validation
  port (`Navigator.ValidateClick`, see round 52): the server refuses
  whole lines its Bresenham raster walks into walled corners - a
  refused click collapses onto the walker and never moves the
  character, so the follower reacts instead of sending it (the leg
  shortening first, then the hop back to the nearest swallowed plan
  bend - the escape out of the geodata trap cells - and finally the
  re-path). The walker re-paths around obstacles after
  15 s of standing still (the later stucks fire on the 4 s fast
  window; 3 re-paths abort the trip): the stuck skip only jumps onto a
  waypoint with a walkable line from the standing cell (the same gate
  as the cursor advance - the 2026-09-11 06:19 trainer hall aisle
  dump: the blind skip armed the east hall waypoint whose click the
  server collapsed onto the first step, and the partial clicks crept
  the character 16 units at a time into the dead-end pocket cell east
  of the aisle whose closed east wall then refused the click
  wholesale), and the re-planned leg advances its cursor past the
  fresh plan's wp 0 (the standing cell itself) before the click fires
  - a click at the character's own position is a guaranteed server
  refusal that would burn the re-path budget on nothing. A re-path
  that starts from the same cell the previous one planned from,
  without a single cell of movement in between, aborts the trip at
  once instead of re-planning the identical route: the deterministic
  planner reproduces the same first click the server already refused
  twice (the 2026-09-11 11:34 village return dump froze through two
  whole trip cycles this way), and the zone return escalates straight
  to the direct server routed legs (a frozen return sets the zone
  fail budget). The town walk legs climb their own escalation ladder
  instead of dying on the first freeze (the 2026-09-12 03:56 trainer
  hall dump: the learn leg froze at the aisle entrance through every
  re-path of two whole trips - the server walled the corridor the
  pack modeled as open): the frozen abort first bans the aimed
  waypoint's cells as the session's avoid areas and re-plans the
  detour around them (the pathfind search takes the banned patches -
  a step onto banned ground costs impassable, and neither the direct
  line shortcut nor the smoothing may collapse a leg across them, so
  the trainer hall route goes north over the terrace and east past
  the hall), then - if the detour freezes as well - the follower
  drops the plan and clicks the stop target directly by the server's
  own routing (the npc approach point of the stop, the water guard
  and the aggro steering stay on), bounded by a 45 s window; only a
  direct walk that also makes no progress ends the trip with its
  cooldown. The ban list survives for the session (bounded to 8
  areas) so the later trips route around the frozen corridor too.
  The `waypointBehindRoute` gate only counts a waypoint the character
  stands near: the V-shaped detour routes (the hall recovery climbs
  far north before doubling back south east) carry waypoints hundreds
  of units ahead whose position projects beyond the doubling segment,
  and the projection test alone re-aimed their climb clicks at far
  route samples whose straight lines cross the terrace walls.
  After the first stuck with no clear successor the short click
  extension arms: the clicks whose target sits under the server
  rescue floor (50 units - the server's own move validation only
  hands a collapsed click to its pathfinder when the original line
  was longer than 30 units, so a shorter collapse is silently
  canceled with ActionFailed and never moves the character) or
  behind the character on the route re-aim at the forward route
  samples past the floor (the march along the plan polyline skips
  the under-floor and backward samples, the water guard and the
  click validation port gate every sample, and a walled sample only
  skips forward to the next route cell). A trip timeout (20 min) and a trigger cooldown
  (5 min after every trip end) bound the whole feature, and a death -
  mid trip or not - clears the cooldown: the village restart lands next
  to the shops and a full inventory sells right after the revival
  instead of walking to the farm spot with the junk first.
- Water safety of the trips (the 2026-09-10 stuck regression): the
  server moves characters into water without any hesitation - its own
  move routing carries no water cost, its getValidLocation accepts the
  gradual underwater beds, and every move request of a character it
  considers swimming (inside a water zone: the whole elven region below
  z -3780) skips the geodata validation entirely and just walks the
  character straight at the click, clamped to 700 units. A town trip
  once swam below the elven village plateau this way and stood
  paralyzed under its cliff: the swim z floated above the water zone
  bound, so the zone flipped in and out while every click toward the
  village deck resolved onto a layer the lake bed has no walkable
  connection to (the server answers such moves with the character's own
  position - a 0 length walk) and the re-paths replanned from the same
  floating spot. Four defenses keep the trips ashore now: (0) the
  planning itself is dry (FindPathApproachDry, the 2026-09-10 delevel
  water loop): the water is a wall for the trip leg searches, a
  destination only swimming reaches aborts the leg at once (the
  cooldowns arm) instead of planning a route the walker refuses leg by
  leg - the wet route of the ordinary search is exactly what looped the
  deleveling "walking to the guard" -> "the walk would enter water" ->
  "aborted, the walk would cross water" every 1.3 s in the reported
  state dump, (1) the
  smoothing never collapses a leg between two dry points across water
  (pathfind legDry), (2) the follower verifies every click line with
  pathfind.WaterCrossed before sending it - the pure water raster, NOT
  the DryLine answer: the line of sight half of DryLine fails on the
  height steps of the village deck ramps, the teacher legs of the
  learning trips read as water that way and every trip that carried
  them aborted on the 3 re-path budget (the 2026-09-10 teacher round) -
  a wet click is refused and the walk re-paths around the shore (the
  refusals share the 3 re-path budget), and (3) a character that still
  ends up over a lake bed
  (the geodata surface under it below the water level, OverWater)
  enters the water escape: the walk to the nearest shore
  (FindWaterEscape, a breadth first flood over the walkable surface)
  replaces the leg, a stuck escape re-plans itself, and once the
  character stands dry the interrupted leg re-plans from the shore with
  a fresh re-path budget. An abort of a walk machinery that runs during
  the deleveling aborts the deleveling itself (abortTownTrip
  delegates to abortDelevel): the plain trip end left the delevel
  state armed without a cooldown and the next tick restarted the walk
  into the same blocker. One release valve exists for the geodata
  raster artifacts on a planned route (the village plaza cells
  without a modeled floor resolve to the lake layer below them, every
  straight line over the plaza center fails the dry raster while the
  points stand dry): when the re-path budget exhausts without an
  escape on the trip, the plan is trusted for the rest of the leg and
  walks over the server routing (wetPlanTrusted; the standing water
  check and the shore escape stay armed, and a next budget exhaustion
  after an escape ends the trip for real). The dump and the map carry the whole leg -
  origin, every waypoint with the passed markers, the TARGET marker on
  the current waypoint and the destination - for exactly this class of
  debugging (see docs/webui.md).
- Path layer selection: the trip legs navigate with
  pathfind.Engine.FindPathApproachDry (through the Navigator's
  FindPathApproachDryAvoiding - the dry search with the session's
  frozen corridor bans) and the leg approach radius: the merchant
  stops and the returns use the wide trip ring (200 units, under the
  interaction distance): the walk ends on the deck ring around the
  merchant, which handles the C1 shop interiors (the geodata holds no
  floor layer at the real merchant z - only a raised surface and the
  water below) and the counters the same way, while the water deck
  below the shop never satisfies the radius (the z difference counts
  in the 3D distance). The teacher stops search the close ring
  instead (npcApproachOffset, 150 units) with the wide ring as the
  fallback: the character walks right up to the training npc - the
  geodata search is the one that knows the walkable ring cells (the
  trainer hall interior carries its floor along the hall rows, the
  straight line offset ring lands on the roof-only bands between
  them), and the tight legs complete their route end with the pass
  radius instead of the wide trip slack. The water is a wall for the
  search, so the walks cross the village ramps instead of swimming
  the lake under the floating island (the 2026-09-09 fix; regression
  tests `TestFindPathToShopDeck`, the synthetic water tests of
  `search_test.go` and the dry search tests of `dry_search_test.go`).
  approachMerchant also
  gives up targeting when the merchant stands more than the interaction
  distance above or below the character. The teacher approach
  (approachTeacher) walks the npc approach point ring before the talk
  click fires (the 2026-09-12 user rule: the character walks from the
  building entrance right up to the training npc), and the approach
  window bounds the walk: a ring the server routing refuses to close
  still talks from wherever the character stands when the 3D distance
  fits the interaction gate (the 2026-09-11 05:45 z gap rule), a
  teacher on a deck the ring cannot reach skips the stop.
- The npc talk selection clears when the conversation ends: the
  merchant select and the teacher talk click leave the villager
  selected server side (the server never clears a selection, only the
  next selection replaces it), and the hunting engage that follows the
  trip would re-adopt it - the forced attack requests on a friendly
  npc only burn the 12 s engage stuck timeout. The trip machinery
  sends the self click (Action 0x04 on the own object id, the official
  client way of dropping a selection, `GameClient.ClearTarget`) when a
  stop finishes (`advanceTripStop`) and when the whole trip ends
  (`endTownTrip`), and the engage adoption itself requires an
  attackable npc (`ObjectAttackable`) so a lingering selection
  (a talk outside the trip end, the self selection of the clear click)
  never becomes a hunt target.
- Merchants: townMerchants carries the shop npcs of the known towns
  with their spawn coordinates; the C1 spawn ids map to the client
  display ids the NpcInfo packets carry (30147..30150 -> 7147..7150
  through CT0_to_C4_ids.txt). The sell uses RequestSellItem 0x1E with
  the standard inventory sell list (list id 0, the CUSTOM_CB_SELL_LIST
  of the official client): the server prices every item itself at
  referencePrice/2 (no prices in the packet), answers with
  InventoryUpdate removals, ItemList, a CUR_LOAD StatusUpdate and the
  "The transaction is complete." SystemMessage, and accepts the
  transaction even without a targeted merchant - the bot still walks to
  the merchant and selects it (interaction distance 250, one selecting
  request per second) like the official client, and falls back to
  selling without one after a 45 s wait. The transaction flood
  protector paces the batches (one per 11 s, up to 25 items); every
  item is offered once per trip, so items the server refuses to sell
  can not stall the phase. The junk ranking lives in
  state.Bot.SellableItems: duplicated gear pieces first (every piece of
  an item id but one), then the lowest sell value per unit weight (the
  generated npcdata.ItemPrice/ItemWeight dictionaries, see
  tools/generate_item_stats.sh); equipped gear, adena and quest items
  never sell, and neither do the spellbooks of the unlocked queued
  lessons (demandedBooksLocked walks the stored learning queue): a
  book bought at the book stop or looted for a near term lesson feeds
  it, selling it for referencePrice/2 only buys it back full priced
  at the next learning trip - the 2026-09-10 report loop. The
  overflow destroy (DestroyableItemsExcluding) keeps them the same
  way. The first town trip of a session also waits for the server
  skill list (state.SkillsListed, bounded by 10 s from the session
  start, re-armed on every reconnect): the enter world packet burst
  (UserInfo, ItemList, SkillList) races the first hunt ticks, and a
  trip that starts between the ItemList and the SkillList plans
  without the learning stops.
- The live verified cycle (2026-09-07): trigger at 55 slots/73% weight,
  walk to the trader ~18k units in ~60 s, one batch of 25 items sold
  (55 -> 30 slots, 73% -> 36% weight), walk back and hunting resumed;
  covered offline by internal/swarm/hunt/town_test.go (trigger,
  waypoints, merchant selection, batches without a merchant, no
  destroy during the trip, stuck re-paths, death reset) and by the
  stuck report reproduction of `town_repro_test.go` (the round 35
  scenario with the exact reported positions 45544 45880 -2992 ->
  Unoren 44667 46896 -2982: a straight MoveToLocation sent at the
  merchant stalls at the reported spot under the server simulating
  walker, the planned trip walks it with zero stuck re-paths; the same
  scenario runs live through tools/repro_stuck_trip.sh, which places
  the character at the stuck spot through the database and captures
  the mid-walk state dump).

## Deleveling (internal/swarm/hunt/delevel.go)

When the character level exceeds the median level of the living
attackable npcs inside the zone by 7 or more AND the level is at least
10, the bot walks to the nearest archer guard (delevelGuards: the
Elven village sentinels Kendell and Starden only - display ids 7218 and
7220, Elven Bow, ARCHER ai type; the melee sentinels Veltress and
Rayen may never retaliate and are never provoked), approaches it into
melee range (60 units) and provokes it with the same
select-then-attack flow the hunt uses; the archer always answers a
provocation in its line of sight with bow shots and kills the
character, the village restart revives it next to the guards, where the
walk to the next death starts again. Only deaths at level 10+ remove
experience (Lucky absorbs the penalty below 10), so the deleveling
never triggers below 10 and its target never aims below 9. The vanilla
guard deaths pay the penalty from level 10 up (verified live: level
10, exp 48229 died to the archer Kendell -> level 9, exp 46190, lost
2039 of the 22972 level span), and a free death counter stays as the
safety net for servers where the deaths remove nothing: three
consecutive penalty-free deaths abort the deleveling and arm a 30 min
cooldown.

The deleveling stops at the target level = median zone mob level + 5
(the last level with the full item drop chance, floored at 9) and
re-triggers after hunting raised the level back above the trigger. The
fight stage re-paths when the guard ignores landed damage within 20 s
and aborts the deleveling after 3 failed re-paths; a stage without a
single landed blow never counts - the town guards sit at level 70
while the deleveling character climbs down from the low tens, the
vanilla `calcHitMiss` floors the hit chance at 20 percent there
(`(80 + 2 * (accuracy - evasion)) * 10` clamped to 200..980), so a
whole timeout window of misses is the normal variance of the gap and
the stage extends instead of blacklisting a guard that had no damage
to retaliate against (internal/swarm/state SelfLandedHit feeds the
distinction). The whole deleveling is bounded by a 60 min timeout and
a 1 min cooldown after it ends. The
walk legs are split into at most 1000 unit steps because the server
refuses move requests with a target farther than 9900 units
(MoveToLocation readImpl); the smoothed geodata routes happily exceed
that over open terrain. A death during the deleveling keeps the phase
running (the town trip aborts instead) and the destroy cleanup stays
suspended like during the town trips. Covered by
internal/swarm/hunt/delevel_test.go (trigger, hysteresis, guard fight,
death continuation, target exit, fight timeout, free death abort).

Delevel trigger caveat: `MedianZoneMobLevel` sees only the known
objects around the char; at the village (8000+ units from the fields)
the median is 0 and the delevel does not re-trigger - by design the
trigger fires only from the farm zone. The live median flickers with
the respawn windows (a ground whose designed mix sits close to the
character level shows a transient median a full trigger lower while
its higher species are down), so the trigger also requires the static
median of the anchored spot to agree with the live one (the 2026-09-12
building entry acceptance round de-leveled a healthy level 15 on the
Kaboo Orc Fighter SW respawn window whose static median is 9), and the
spot picker itself skips eligible grounds whose static median sits at
the delevel gap - the spot window (level-8) admits grounds the
deleveling would immediately answer with guard deaths.

## The quest dialog walker (hunt/quest_walker.go)

The dialog engine the M2 quest brain runs on: DriveDialog walks one
NPC conversation end to end from the two-click talk entry to the
answer page of the last bypass. The caller (the future quest trip
phase) keeps the character within the 250 unit interaction distance
of the npc through the whole conversation - the server gates every
bypass and every quest event on it (Player.processScriptEvent
resolves the npc through the last-folk memory and re-checks the
distance); the walker's entry clicks refresh that memory.

The walk: the paced two-click talk (the first Action click of a new
target only selects it, the second opens the html - NpcClick.onAction;
a click on an npc that is already the target interacts right away, so
the entry is idempotent), the bounded new-page wait (5 s per page,
polled every 250 ms), the link match by visible text (the
case-insensitive containment match: a route step names a distinctive
substring of the link label, "Challenge the test", "Change profession
to an Elven Knight"), the IsDialogCommand validation of the tracker
(only commands the open page offered are sent - the server drops the
rest silently) and the bypass send. Every page the walker sees is
parsed (ParseHTMLLinks) and applied to the tracker dialog section
BEFORE any bypass of that page fires, so the validation runs against
the page the command came from (the hunt-side feed of the state
dialog section; the connection layer stores the raw html only).

The arrival signal of a new page is the content change: the
connection layer keeps only the last html, and the pages of one
conversation all arrive from the same npc, so "the html of npc X
differs from the last seen" is the only arrival evidence. Every quest
page transition changes the content (the links of a page carry the
next page's file name); the one blind spot - the server re-sending a
byte identical page after a bypass - reads as "no answer yet" and
lapses into the step timeout, which the caller treats as a failed
conversation (never a silent success).

The route steps are data: []DialogStep with the link texts in
conversation order. A step whose page never arrives, whose link text
the page does not offer, or whose command fails the open page
validation fails the walk with an error naming the step; the caller
retries the conversation from the entry (the server state of the
quest makes the re-entry idempotent - re-talking re-sends the page
of the current cond).

Live verified (the T-014 round, account dialogw1): the trainer page
of Ellenia at the elven village (the character injected at her
approach ring through the DB), the "Quest" link (the bare `bypass
Script` command the trainer pages carry) walked through to the
no-quest answer page in 1.7 s - the full click -> html -> link ->
bypass -> answer chain against the deployed stack. The SkillList
link of the same page answers with the SkillList packet, NOT an html
page - the walker routes only fit page-answering links. The opt-in
live suite (SWARM_LIVE_DIALOG=1, quest_walker_live_test.go) replays
the round trip; SWARM_LIVE_DIALOG_LINK overrides the route link for
ad-hoc experiments.

Covered by hunt/quest_walker_test.go (the scripted fakeGame wrapper
emulates the server html action cache: a bypass the open page never
offered is never answered) on the real Q00406 and
ElfHumanFighterChange1 datapack pages: the accept chain, the -h
prefix strip of the class change link, the stale-npc page guard, the
missing-link guard, the same-page repeat blind spot, the timeouts
and the argument guards.

## Live validated facts (2026-09-07 round, do not re-derive)

The town trips (sell loop) and the deleveling cycle are live verified
end-to-end on the vanilla server (only logging patches: the
DEATHLOG/GUARDDMG/MOVEDBG lines of the local checkout; the earlier
guard revenge + NPC kill penalty server patch was reverted - see the
server integrity rules in AGENTS.md).

- **Appearing fix confirmed working**: after adding the 0x30 reply to
  the self TeleportToLocation (connection/game.go applyTeleport), the
  death -> village revive -> walk cycle worked: consecutive provoke ->
  die -> revive -> walk-to-guard cycles.
- **Archer guards always retaliate, melee guards may not**: an archer
  guard in the attack intention shoots at everything within its 850+
  unit bow range (the thinkAttack doAttack branch applies no karma
  gate); a melee guard only follows the provoker (Guard.addDamage
  startFollow) and the chase dies in the checkTarget gate
  (Player.isAutoAttackable returns karma > 0 for guards). Verified in
  the world. The deleveling therefore provokes the archer sentinels
  Kendell and Starden only, in melee (60 unit approach).
- **Guard death experience penalty**: verified live on the vanilla
  server: a guard death at level 9 removes no experience while at
  level 10 the penalty lands (the Lucky newbie skill absorbs it below
  10; the doDie penalty branch runs for every killer, guards
  included). Live sequence: test1 level 10, exp 48229 provoked the
  archer Kendell in melee, died, and the penalty removed 2039 of the
  22972 level span -> level 9, exp 46190 (DEATHLOG lines in the game
  log). The bot's free death counter stays armed as the safety net for
  penalty-free servers and does not interfere (the productive death
  resets it).
- **Server geodata loads after the restart**: the game server runs
  with PathFinding = 2 and the region 21_19 in dist/game/data/geodata
  ("GeoEngine: Loaded 1 regions"); the bot pathfinds over its own repo
  copy in data/geodata (first candidate of the geodata detection).
- **Stale server side selections after abrupt disconnects**: an abrupt
  disconnect (a killed process, a dropped pipe) while the character
  auto attacks leaves the server side attack running; when that target
  dies the corpse stays SELECTED (the server never clears the
  selection, only the next selection replaces it) and every forced
  attack on the same object id of the next session comes back
  ActionFailed forever - observed live twice (inCombat true, 2 refused
  actions per second, no kills) after killing the bot mid-farm.
  Bot-side recovery (no server patch): the engage drops a target that
  never starts the fight within engageStuckTimeout (12 s) and skips it
  for engageSkipDelay (30 s), so the next pick selects a DIFFERENT
  object id - that selection replaces the stale one and the hunt
  resumes. The ATTACKLOG diagnostics lines in the local server
  checkout log which AttackRequest branch refused an action. The
  reproduction is timing dependent (the kill must land while the auto
  attack runs); three deliberate kill -9 attempts did not hit the
  window again, the fix is covered by unit tests instead.
- **Live validation result (2026-09-07)**: the full cycle ran on the
  vanilla server: trigger at level 10 over the level 1 gremlins ->
  pathfinding walk to Kendell (~85 s, geodata from the repo
  data/geodata) -> melee provocation -> the archer killed the
  character in ~7 s (GUARDDMG at distance 0) -> exp penalty: level 10
  -> 9 (DEATHLOG) -> "delevel finished at level 9, walking back" ->
  pathfinding walk back to the farm spot (~62 s) -> farming resumed
  (kills with loot). The bot then kept hunting at level 9 until the
  test window ended.
