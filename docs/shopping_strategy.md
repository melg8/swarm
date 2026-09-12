# Shop strategy of the swarm bot

How the bot spends its adena on the weapon and armor shops of the
Elven Village so it keeps leveling efficiently and moves up the mob
level ladder of the elven lands. The implementation lives in
`internal/swarm/gear` (`PlanPurchases`) and
`internal/swarm/hunt/shopping.go` (the town trip execution); this
document explains the reasoning and the numbers.

## The facts the strategy builds on

Prices (from the Mobius C1 data files, verified in the server
source):

- The merchants sell at **reference price x (1 + tax)**: the Elven
  Village has baseTax 15 (see `MerchantPriceConfig.xml`, priceConfig
  3) and no castle owns the tax on a fresh server, so the buy price
  is **reference x 1.15**. Selling returns **reference / 2** (the
  Mobius `RequestSellItem` computes it that way), so every bought
  item loses ~57 percent of its value when resold - gear is bought
  to be worn, not flipped.
- The elven village shops (buylists generated into
  `internal/swarm/npcdata/shop_catalogs.go`):
  - **Unoren** (packet template 7147, list 3014700): melee weapons -
    Short Sword 8 pAtk / 768, Broadsword 11 / 12.5k, Gladius and
    Handmade Sword 17 / 54.1k, Long Sword 24 / 136k.
  - **Ariel** (7148, list 3014800): light armor - Shirt 36 pDef /
    147, Leather Shirt 43 / 2.4k, Wooden Breastplate 47 / 8k, Bone
    Breastplate 50 / 20.3k, matching gaiters, caps, gloves, shoes
    and the small shields.
  - **Creamees** (7149, list 3014900): jewels - the mDef ladder from
    Magic Ring 7 / 33 up to Necklace of Wisdom 25 / 11.9k - and the
    spellbooks of the class lessons (list 3014901).
- The income of the elven lands: a level 1-4 keltir drops 7-11 adena
  and the occasional item; the level 5-7 goblins and kaboo orcs drop
  roughly double. The bot also sells every gear drop at reference/2,
  so the effective income grows with the mob tier.

## The strategy

**Rule 1 - the purchase phases: the weapon milestone, the pdef
maximizing armor set, the basic jewel floor, the leftover shield.**
The planner walks four phases; the order is the user rule of the
spending:

1. **The weapon milestone** - the top affordable STRICT weapon
   upgrade: the best tier the wallet plus the sale credits reach,
   never a rung below it. The top-tier slot guard of the planner
   drops every candidate that another viable candidate outgains on
   the same paperdoll slots, whatever its `gain / price` - the value
   per adena ranking only decides between the per-slot winners
   afterwards. The ladder of the elven catalogs plays out as Short
   Sword (883) -> Heavy Chisel (9280) -> Knife (14374) -> Sickle
   (21275) -> Brandish (62214, the two hand sword: 21 x 325) ->
   Long Sword (156400), and every trip buys the highest tier its
   adena covers.
2. **The pdef maximizing armor set** - one piece per armor family
   (chest, legs, head, gloves, feet, back) per trip, chosen by the
   exhaustive enumeration of the per family efficient frontiers
   inside the remaining budget (`gear.enumerateArmorSet`): the set
   that maximizes the summed pDef, ties preferring the cheaper set.
   A rich wallet buys the advanced pieces directly (the 100k adena
   acceptance wallet buys the Wooden Breastplate, the Hard Leather
   Pants, the Leather Helmet, the Gloves and the Leather Shoes -
   127 pDef for ~37k adena); a poor one fills many slots with the
   cheap offers (the 500 adena wallet buys the shoes and the shirt
   when they maximize pDef). Whether one expensive piece or several
   cheap ones is "better" is exactly what the enumeration answers:
   the set maximizes the total pDef either way. The sell credits of
   the displaced pieces discount the option costs, so an upgrade is
   within reach as soon as the adena plus the proceeds cover it.
3. **The basic jewel floor** - the cheapest jewel offer of every
   bodypart family fills the EMPTY jewel slots (Magic Ring 37,
   Apprentice's Earring 56, Necklace of Magic 75, all with tax -
   BOTH halves of the ring and earring pairs) - only after a real
   weapon is worn: the floor gate is the reference price of the worn
   weapon, and a starter weapon (or none) anchors zero, so no jewel
   is ever bought before the first weapon milestone lands. The floor
   fills, it never replaces: the jewels NEVER upgrade at any level -
   the starting locations barely attack with magic, the cheapest set
   covers the mDef needs (the user rule of the basic jewels).
4. **The leftover shield** - the best affordable shield strict
   upgrade behind a real (worn or planned) one hand weapon: the
   shield shares the hand family with the weapons, so a two hand
   milestone (the Brandish, the Long Sword of the two hand lines)
   blocks it and a one hand weapon leaves it open. The shield takes
   whatever the armor set left over.

The weapon budget rule of the old strategy (the defense value capped
at the weapon reference price) is gone: the armor set is bounded by
the actual adena of the wallet, which is the real constraint - the
user rule is to maximize the pDef the leftover budget reaches, not to
keep the defense below the weapon.

Supporting rules that survived the rework unchanged:

- **Nothing is bought twice** and **nothing the inventory already
  carries is bought** (the free upgrades are simulated first, the
  purchases compare against the paperdoll the auto equipment will
  reach anyway). A drop on the shopping list therefore satisfies the
  plan for free: the looted item id occupies the simulated slot, the
  plan moves on to the next step past it.
- **A looted or bought item the bot is about to wear never becomes
  junk** (`gear.PlannedEquips`, the planned equip keep set): the
  simulation's pending equips - the looted upgrades waiting for their
  paced use item request, the buy arrivals, the better halves of pair
  swaps mid flight - are excluded from both the shop sell list
  (`SellableItemsExcluding`) and the overflow destroy list
  (`DestroyableItemsExcluding`). Only the real junk sells: the
  duplicates beyond the family copy count, the looted downgrades and
  the displaced weaker halves of pair swaps.
- **One item per slot per trip**: the weapon phase buys one
  milestone, the armor set picks one piece per family, the jewel
  floor fills each empty slot once and the shield phase buys one
  shield - the next trip re-plans against the paperdoll the purchases
  reached and takes the next step. The upgrade chains are cut: a
  rich bot buys ONE weapon (the milestone), not the
  knife/short sword/sickle ladder in a single walk, and never two
  necklaces of which only the better one gets worn. The one exception
  by design: the pair families (the rings, the earrings) carry TWO
  copies in one plan - one per slot, the basic set covers every slot.
- **The frozen trip plan** (the 2026-09-11 two pairs of gloves
  report): the trip plans ONCE at its start
  (`maybeStartTownTrip` -> `Loop.tripPlan`) and everything after
  that reads that frozen plan - the sell first step banks exactly its
  SellFirst pieces, the stop planning distributes exactly its
  purchases, and nothing re-plans in between. One honest guard
  remains at the execution point: a surplus copy of an item the
  inventory already carries (beyond the family copy count) drops out
  of the stop before any request goes out (`dropOwnedPurchases`) -
  a second pair of gloves is never part of the plan, while the
  second ring half of the basic set still buys.
- **The top-tier slot guard**: within the weapon and shield picks the
  guard records the best viable gain per paperdoll slot and drops
  every candidate aspired above that record, whatever its value per
  adena. The guard holds across the whole plan: the eroding budget of
  the later phases must not crowd the top tier out and push a cheaper
  rung in - the slot stays unpurchased and waits for the next trip.
  The floors bypass it (they deliberately buy the cheapest offers of
  the empty families) and the wanted tail of the widget queue walks
  without it (the unbounded budget of the tail would collapse the
  displayed ladder of a slot to its top step and hide the milestones
  the bot saves for).

**Rule 2 - buy at the town trip, sell first - and sell everything.**
The purchases run inside the town trips the loop already makes: the
trip FREEZES its purchase plan once at the start (see the frozen trip
plan rule below), the junk selling frees the slots and banks the
sale credits the plan counted on, then the bot walks to every
merchant of the frozen plan (buy groups order by walking distance; a
group of the merchant the character already stands at buys without
an extra walk). A trip is worth it when the plan totals at least 100
adena - below that the walking time costs more than the gains.
**Every vendor visit sells the whole accumulated junk, not only past
the 50 percent inventory trigger**: the selling ends when nothing
sellable is left, so a buy trip never leaves the bag half full of
sellable drops (the bot would farm with them and walk back for the
sale later otherwise). A trip also never interrupts a running fight:
it waits for the kill, the loot pickup and the between-fights window,
because the drops of the kill are the point of the fight.

**Rule 2a - the weapon outranks the trip itself (the 2026-09-11
bare-handed report).** The weapon purchase is the highest priority of
the strategy: a plan with an affordable weapon routes the trip's sell
stop to the weapon's merchant (the junk sells at any merchant), so
the sell-first credit of the replaced weapon and the replacement buy
share ONE stop - the replacement lands seconds after the sale instead
of a village walk later. A character with NO weapon (nothing the
profile can fight with, equipped or bagged - see `gear.HasWeapon`)
runs the weapon errand with the short 45 second retry cooldown
(`weaponRunCooldown`), and the hunt loop holds its fresh target picks
while the run is pending (the 2 damage fists never farm when the plan
offers a sword).

**Rule 2b - one town visit buys everything (the 2026-09-12 acceptance
round).** The weapon run carries the learning stops too, and every
trip plans them behind its gear stops: the stop order is the weapon
stop (the sell stop routed to the weapon merchant), the armor stop,
the jewel stop (which absorbs the spellbook purchases when the
merchants match - Creamees sells both the basic jewels and the
books), the teacher stop and the return leg. The weapon stop runs
FIRST, so the stuck teacher leg that once aborted a weapon run with
the weapon already sold cannot strand a bare-handed bot anymore: the
weapon is bought and worn before the teacher leg ever runs. The gear
plan reserves the spellbook budget out of its planning wallet
(`Loop.pendingBookBudget` prices the books of the learnable lessons
the inventory does not carry yet), so the aggressive armor spending
can never eat the book money - the books always stay affordable in
the same visit, and the acceptance round finishes its shopping,
its book buying and its lessons in ONE walk through the village.

**Rule 2c - the gear debt (the 2026-09-12 pantsless report).** No
town trip may leave a paperdoll slot worse than it found it: the
trip start snapshots the worn slots (`Loop.snapshotTripGear`) and
every trip exit - the normal end, the aborts, the attacker
interrupt, the relogin-resumed return leg - compares the reached
paperdoll against the snapshot (`Loop.gearDebtCheck`). A slot that
was occupied, sits empty now and whose piece is gone from the
inventory was sold for a replacement that never landed (a silently
refused buy, a merchant that never showed up, a walk abort, a
session death mid trip): it becomes GEAR DEBT. The debt logs the
loss ("the trip left the legs slot empty - the Leather Pants it
started with is gone"), shortens the trip cooldown to the gear run
window (the 45 s `weaponRunCooldown` - farming without the armor
the merchant sold is the same wound the weapon run answers for
bare hands) and clears with a log line the moment the slot is
dressed again. The debt never justifies a trip on its own: a broke
wallet cannot buy the filler, the ordinary triggers (the affordable
plan of the refill) fire the trip on the short cadence the moment
the wallet affords it. The state dump answers for the holes the
equipment section used to hide: the "empty slots:" line under the
worn pieces names every unfilled family (the report's bot carried
an invisible wound - the dump printed only the occupied slots, so
a character farming without its legs armor read as a fine outfit).

**Rule 3 - the gear feeds the zone ladder.** The hunting zones gate
on gear points (`gear.TotalGearPoints`: the weapon damage per hit
plus the defenses, in character-stat-sheet units). The intended
ladder of the elven lands (the ten band gates of the granular zone
registry, `hunt/zones.go` - every band holds two to six small squares
anchored on the real spawn territories, the picker takes the nearest
ground of the highest band the level and the gear allow and rotates
between the siblings when a square runs dry):

| Band | Mob levels | Gear gate | The gear that passes it |
| --- | --- | --- | --- |
| Keltir ring around the village | 1-3 | 0 | anything (fists work) |
| Wolf downs | 3-4 | 10 | any weapon |
| Raider fields | 4-6 | 30 | Short Sword + Shirt |
| Goblin and kaboo camps | 5-7 | 40 | Short Sword + Shirt |
| Kaboo grunt woods | 7-8 | 80 | Broadsword or the wooden set start |
| Kaboo fighter woods | 8-10 | 110 | Broadsword + the wooden set |
| Lieutenant camps | 9-12 | 130 | Broadsword + the wooden set complete |
| Leader camps | 11-13 | 160 | Gladius + wooden set + first jewels |
| Elder and spider forests | 12-16 | 200-230 | Gladius/Long Sword + the bone set + jewels |
| Lirein and pincer grounds | 16-19 | 300 | the full shop dress |

Three deaths in one square demote its whole band (the ladder caps
below it until the level changes), so the gear gates and the death
regression steer the same ladder from both ends.

The strategy therefore aims the spending at the *next* gate: while
the character farms the goblins (level 5-7), the planner saves into
the Broadsword and the wooden set pieces because those are the
highest pDef-per-adena buys left - exactly the gear that unlocks the
fighter woods, where the income doubles again.

**Rule 4 - the melee fighter buys melee gear.** The scoring profile
filters the catalogs: bows score zero (the bot fights in melee),
staves score low pAtk. The mage extension reuses the same planner
with a caster profile (WeaponScore = mAtk, robes over leather) and
the magic buylists (Unoren's 3014701, Ariel's 3014801) - no planner
change needed, only a new `gear.Profile` implementation and its
shop lists.

## The numbers of a full elven shop run

The full melee dress of the elven village (top of every slot) costs
about 331k adena at the 15 percent tax: Long Sword 156k, Bone
Breastplate + Gaiters 38k, Leather Helmet 11.7k, Low Boots 7.8k,
Leather Gloves 7.8k, the basic jewel set 0.3k and the Round Shield
8.2k - and brings 234 gear points (the jewels stay basic: their mDef
upgrades never pay against the magic-free mobs). A character reaches
that around mob level 18-20 on the elven lands income; by then the
southwest forest (the level 13-18 zone) is unlocked and pays for the
D grade of the next town.

## The was/is journey of an elven fighter

The level journey simulation lives in
`internal/swarm/gear/shopping_strategy_test.go`
(`TestShoppingStrategyJourneyComparison`): the character starts with
the Squire's kit and zero adena, farms the elven lands mob ladder
(the income model = the experience.xml exp per level divided by the
exp of the level's mob, the adena drops of the npc data at 70
percent chance - gear drops not counted) and shops once per level
through both planners: the legacy greedy value-per-adena walk the
rework replaced and the phased walk. The full table prints with
`go test -run TestShoppingStrategyJourneyComparison -v
./internal/swarm/gear/`; the condensed comparison (the IS column
shows the weapon-first order: the milestone leads every trip that
affords one, the armor set maximizes the pDef of the leftover, the
jewels are the basic floor only - no upgrades at any level):

| Level | WAS (greedy value/adena) | IS (weapon first, pdef-maximizing set) |
| --- | --- | --- |
| 1 | Apprentice's Shoes 8 | **Apprentice's Shoes 8** |
| 2 | Leather Shield 34 | **Short Gloves 42** |
| 3 | Short Gloves 42, Cloth Cap 63, Magic Ring 37 | **Cloth Shoes 42, Cloth Cap 63** |
| 4 | Necklace of Magic 75, Apprentice's Earring 56, Magic Ring 37, Cloth Shoes 42, Pants 105 | **Pants 105, Shirt 169** |
| 5 | Short Sword 883 | **Short Sword 883**, Magic Ring 37 (2nd), Magic Ring 37 |
| 6 | Apprentice's Earring 56, Shirt 169, Ring of Knowledge 621, Leather Cap 1047 | **Cotton Shoes 698, Leather Cap 1047, Apprentice's Earring 56 (2nd), Apprentice's Earring 56** |
| 7 | Ring of Knowledge 621 (2nd), Short Leather Gloves 698, Cotton Shoes 698, Small Shield 733 | **Short Leather Gloves 698, Leather Pants 1747, Necklace of Magic 75** |
| 8 | Leather Pants 1747, Leather Shirt 2794, Necklace of Knowledge 1242, Mystic's Earring 932 | **Leather Shirt 2794, Wooden Helmet 4577** |
| 9 | **Heavy Chisel 9280**, Mystic's Earring 932 | **Heavy Chisel 9280** |
| 10 | **Knife 14374**, Earring of Strength 4036, Ring of Anguish 2691 | **Sickle 21275** (the chisel's sell-first credit closes the gap) |
| 11 | **Sickle 21275**, Earring of Strength 4036, Ring of Anguish 2691 | **Gloves 3047, Low Boots 7785, Wooden Breastplate 9154** (the pdef burst) |
| 12 | Buckler 3196, Leather Shoes 3047, Gloves 3047, Wooden Helmet 4577, Necklace of Anguish 5382, Wooden Breastplate 9154 | **Leather Gloves 7785, Leather Helmet 11730, Bone Gaiters 14604** |
| 13 | Hard Leather Pants 5715, Leather Helmet 11730, Round Shield 8176, Cat's Eye Earring 10223, Low Boots 7785 | **Bone Breastplate 23345, Round Shield 8176** |
| 14 | **Brandish 62214** | **Brandish 62214** (the sickle's credit) |
| 15 | Cat's Eye Earring 10223, Necklace of Wisdom 13684, Leather Gloves 7785, Bone Gaiters 14604, Ring of Wisdom 6807, Bone Breastplate 23345 | - (the wallet saves for the Long Sword) |
| 16 | Ring of Wisdom 6807 | **Long Sword 156400, Round Shield 8176** (the shield displaced by the two handers returns) |

The wastes the phase rework removes, visible in the WAS column:

- **The greedy filler soup**: the value per adena mixes the piece
  families with no order - the jewel floor items (Magic Ring 37 at
  level 3-4) run BEFORE the first weapon while the armor fillers
  dribble in around them, exactly the ordering the weapon-first rule
  replaces.
- **The jewel ladder in magic-free zones**: Ring of Knowledge at
  level 6, Mystic's Earring at 8, Earring of Strength at 10, Cat's
  Eye at 13, Necklace of Wisdom at 15 - mDef buys while nothing
  attacks with magic. The IS column carries the basic floor only.
- **The shield ladder after the two-hander**: Leather Shield ->
  Small Shield -> Buckler -> Round Shield, one shield per trip at
  levels 17-20 while the two-hander keeps displacing them. The IS
  column buys the Round Shield when the one-handers carry it.

The IS column walks the same milestone ladder (the chisel at 9, the
sickle at 10 through the chisel's credit, the brandish at 14, the
Long Sword at 16), spends every leftover into the pdef maximizing
armor set (the wooden burst at 11, the bone burst at 12-13) and
finishes the basic jewel set by level 7 - the same full dress
reached with no wasted intermediate rungs and no mDef spending.

## The acceptance wallet of the farm readiness round

The one-off check the journey test pins (the user acceptance
scenario): a bare level 15 character with 100,000 adena plans the
Brandish (62214), the 127 pDef armor set (Wooden Breastplate 47,
Hard Leather Pants 29, Leather Helmet 23, Leather Shoes 15, Gloves
13 - 37.4k adena) and the five basic jewels (both ring halves, both
earring halves, the necklace - 261 adena): 99.9 percent of the
wallet turns into combat stats in ONE plan, where the old planner
bought the 88 pDef cheap floor and burned 17k adena on the Cat's Eye
and Ring of Wisdom mDef upgrades.

## The Dion shop section (the 20-25 band)

The M3 band survey (`docs/band_20_25_survey.md`) names the Town of
Dion the service town of the 20-25 grounds: the bot reaches it through
the gatekeeper network (Mirabel -> Gludio -> Dion, the T-008 packet
chain) and shops the D-grade gear the band needs there. The shopping
strategy reuses the same planner; the only difference is the catalog
the town trip consults.

- The Dion merchants (`internal/swarm/hunt/town.go::dionMerchants`):
  Sabrin (7060, weapons), Casey (7061, armor), Sonia (7062, jewels +
  spellbooks) and Lara (7063, grocery), at the spawn positions of
  `spawns/Dion/DionNPCs.xml`. The buylist ids (3006000-3006300) are
  the file names of `dist/game/data/buylists/`; the npcdata generator
  loaded them into `npcdata.npcBuyLists` (the same map the elven
  catalog reads).
- The Dion tax (`internal/swarm/hunt/shopping.go::dionTownTaxRate`):
  20 percent (the `MerchantPriceConfig.xml` priceConfig id=8 baseTax=20;
  the castle tax is 0 on the local test server so the total is the
  base tax). The buy price formula is the same shape as the elven
  village: `price = baseItemPrice * (1 + taxRate)`, so the D-grade
  items cost 20 percent over the reference price (vs 15 percent in
  the elven village).
- The catalog selection (`internal/swarm/hunt/shopping.go::
  shopCatalogForRegion`): the hunt loop picks the Dion catalog when
  the active zone region is Dion (`Loop.SetHuntingZoneRegion("dion")`
  sets `Loop.zoneRegion`); the elven village catalog stays the
  default for the 1-19 band (the M0 acceptance still passes). The
  `gear.DionCatalog` of T-010 is the gear-package view of the same
  merchants; the hunt package builds its catalog from the npcdata
  buylists the same way the elven catalog does.
- The spellbook budget: the elven village teachers (the M0 round) buy
  their spellbooks at the elven merchants (`internal/swarm/hunt/
  learning.go::bookPurchase` reads the elven catalog); the Dion
  teachers (the M2 class transfer round) buy theirs at Sonia (the
  3006201 mystic spellbook list). The multi-town book purchase is the
  M2 follow-up (the T-015 quest data and the T-016 acceptance scenario
  own it).

The sell-first rule, the one town visit, the wallet reservation for
the spellbooks and the auto equipment all carry over unchanged: the
strategy is catalog-agnostic, the Dion round only swaps the catalog.

## Where each piece lives

- `tools/generate_item_stats.sh` - the prices, weights and combat
  stats of every item (from `data/stats/items/*.xml`).
- `tools/generate_shop_catalogs.sh` - the buylists of every merchant
  keyed by the packet template id (from `data/buylists/*.xml` and
  the CT0 display id table).
- `internal/swarm/gear/shopping.go` - the phased planner
  (`PlanPurchases`, `PlanPurchaseQueue`: the affordable plan plus
  the wanted tail with the cumulative missing adena), the strategy
  phases (the weapon milestone, the pdef maximizing armor set of
  `planWalk.armorPhase` / `familyFrontier` / `enumerateArmorSet`,
  the basic jewel floor behind a real weapon, the leftover shield),
  the top-tier slot guard (`viableCandidates` and `aspiredAbove`),
  the catalogs (`Shop`, `Catalog`) and the adena budget handling
  (the sell credit of `displacedValue` never counts the unsellable
  newbie kit - `gear.IsStarterItem`).
- `internal/swarm/gear/shopping_strategy_test.go` - the level
  journey simulation and the was/is comparison table (the legacy
  greedy planner copy, the income model, the ordering pins, the
  acceptance wallet check).
- `internal/swarm/hunt/shopping.go` - the trip trigger, the
  spellbook budget reservation (`Loop.pendingBookBudget`), the
  multi-stop buy execution, the transaction pacing and the widget
  view publish (`publishShoppingView`: the queue while hunting, the
  remaining trip buys while a town trip runs).
- `internal/swarm/hunt/round60_repro_test.go` - the gear debt
  reproduction of the 2026-09-12 pantsless dump: the full stranding
  flow (the sell first sale, the lost buys, the debt arming), the
  refill trip, the debt lifecycle and the fresh loop self heal.
- `internal/swarm/gear/round60_repro_test.go` - the dump wallet
  pins: the affordable prefix plans the Leather Pants filler for
  the empty legs slot.
- `internal/swarm/hunt/learning.go` - the learn stop planning (the
  books merge into the gear stop of their merchant, the teacher
  closes the trip) and the lesson execution.
- `internal/swarm/hunt/equip.go` - the auto equipment that wears
  everything the trips buy (and loot drops) immediately, and the
  planned equip keep set cache (`Loop.plannedEquipKeeps`) the junk
  flows consult.
- `internal/swarm/state/inventory.go` - the junk selection with the
  keep set filters (`SellableItemsExcluding`,
  `DestroyableItemsExcluding`) that keep the pending wearables out
  of the shop batches and the overflow destroys.
- `internal/swarm/state/shopping.go` - the published shopping queue
  of the web UI (`SetShoppingPlan`, the `shopping` snapshot field).
