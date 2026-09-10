# The autonomous hunt: combat, zones, gear, shopping, deleveling

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The growth subsystems of the autonomous hunt (all unit tested; the
design goal is per-class and per-region extension). The `-hunt` flag
of `cmd/swarm` enables the whole machine; without it the loop runs in
the manual only mode (commands execute exactly the same way, the
autonomous hunting, town trips and deleveling stay off, the village
restart after a death still works).

The three pillars, each detailed in its section below:

- **Auto equipment** (`internal/swarm/gear`, `hunt/equip.go`) wears
  every equippable item the loot and the buys produce.
- **Shop strategy** (`gear/shopping.go`, `hunt/shopping.go`,
  `docs/shopping_strategy.md`) plans and executes the purchases.
- **Multi-zone hunting** (`hunt/zones.go`, the generated registry
  `hunt/zones_elven.go`) climbs the mob level ladder of the region.

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

## Combat safety (hunt/loop.go + the constrained target search of state)

- The engage never initiates on mobs above the character level + 2 or
  on social pulls - a mob whose clan mates stand within its
  `clanHelpRange` (mirroring the Mobius AttackableAI clan call: the
  ALL clan of the attacked mob matches everything, a 600 unit z
  distance blocks the assist, the projected positions measure moving
  packs, a 200 unit margin covers the mates wandering mid fight).
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

The stuck timeout itself now measures the FRESH fight view (the gate is
!SelfFighting, not !SelfEngaged) and a running fight re-anchors the
engage clock, so a stale attack stance without refusals still trips it
after 12 s of no fight packets; while the blind recovery is armed the
timeout stays held (the recovery manages its own budgets). Covered by
hunt/loop_los_test.go (reposition walk, arrival re-engage, both switch
levels, retry budget, stale stance timeout, timeout hold, attempt
scoping, fresh fight guard) and the state tracker test of the refusal
recording.

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

- The greedy value-per-adena planner buys the best score gain per
  adena first (the cheap empty slot fillers beat the weapon upgrades
  early), never buys what the inventory already carries and respects
  the adena budget; **one item per paperdoll slot per trip** - every
  purchase marks the slots it fills or clears (the family interplay
  included) and the later picks skip them, so no upgrade chains are
  bought in a single walk (the next trip re-plans from the reached
  paperdoll).
- The town trips (internal/swarm/hunt/town.go) trigger when the
  inventory passes 50% of the slots or 50% of the maximum weight. A
  trip start never interrupts a fight: `fightBusy` (a living target, a
  pending loot pickup or an incoming hit) holds it until the
  between-fights window, and a resting character is stood up first
  (the server refuses move requests while sitting; the stand toggle
  shares the pending transition gate with the rest logic).
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
- The path plan comes from the pathfind engine through the
  hunt.Navigator interface (set in main.go with hunt.NewNavigator from
  the auto detected geodata directory; without geodata the bot hunts
  without trips). A waypoint follower walks the smoothed path with
  ground click walks (one per 2 s, arrival within 150 units) and
  re-paths around obstacles after 15 s of standing still (3 re-paths
  abort the trip); a trip timeout (20 min) and a trigger cooldown
  (5 min after every trip end) bound the whole feature, and a death -
  mid trip or not - clears the cooldown: the village restart lands next
  to the shops and a full inventory sells right after the revival
  instead of walking to the farm spot with the junk first.
- Path layer selection: the trip legs navigate with
  pathfind.Engine.FindPathApproach and the trip approach radius (200
  units, under the interaction distance): the walk ends on the deck
  ring around the merchant, which handles the C1 shop interiors (the
  geodata holds no floor layer at the real merchant z - only a raised
  surface and the water below) and the counters the same way, while the
  water deck below the shop never satisfies the radius (the z difference
  counts in the 3D distance). The water cost of the search keeps the
  routes on bridges and shores, so the walks cross the village ramps
  instead of swimming the lake under the floating island (the
  2026-09-09 fix; regression tests `TestFindPathToShopDeck` and the
  synthetic water tests of `search_test.go`). approachMerchant also
  gives up targeting when the merchant stands more than the interaction
  distance above or below the character.
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
  never sell.
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
fight stage re-paths when the guard does not fight back within 20 s
and aborts the deleveling after 3 failed re-paths; the whole deleveling
is bounded by a 60 min timeout and a 1 min cooldown after it ends. The
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
trigger fires only from the farm zone.

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
