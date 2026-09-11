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
    Magic Ring 7 / 33 up to Necklace of Wisdom 25 / 11.9k.
- The income of the elven lands: a level 1-4 keltir drops 7-11 adena
  and the occasional item; the level 5-7 goblins and kaboo orcs drop
  roughly double. The bot also sells every gear drop at reference/2,
  so the effective income grows with the mob tier.

## The strategy

**Rule 1 - the purchase phases: the armor floor, the weapon
milestone, the jewel floor, the defense upgrades.** The planner
walks the candidates in four phases; every pick ranks by its phase
first, the phase specific order second (the melee fighter scoring
stays: weapon pAtk x attack speed, armor pDef, jewel mDef, shield
expected block value):

1. **The armor floor** - the cheapest armor offer of every bodypart
   family (chest, legs, head, gloves, feet: Apprentice's Shoes 8,
   Short Gloves 42, Cloth Cap 63, Pants 105, Shirt 169, all with
   tax) fills the EMPTY armor slots. The floor is the opening of the
   journey: the cheap armor comes first, ahead of every weapon and
   jewel - the empty defense slots of the Squire's kit (head,
   gloves, feet) fill with the cheapest pieces the armor trader
   sells while the wallet saves for the weapon. The floor fills, it
   never replaces: a worn piece blocks its family until the defense
   phase opens. The shield has no floor entry - it shares the hand
   family with the weapons (a two hand milestone displaces it), so
   it stays a defense upgrade inside the weapon budget.
2. **The weapon milestone** - the top affordable STRICT weapon
   upgrade: the best tier the wallet plus the sale credits reach,
   never a rung below it. The top-tier slot guard of the planner
   (see the supporting rules) drops every candidate that another
   viable candidate outgains on the same paperdoll slots, whatever
   its `gain / price` - the value per adena ranking only decides
   between the per-slot winners afterwards. The ladder of the elven
   catalogs plays out as Short Sword (883) -> Heavy Chisel (9280) ->
   Knife (14374) -> Sickle (21275) -> Brandish (62214, the two hand
   sword: 21 x 325) -> Long Sword (156400), and every trip buys the
   highest tier its adena covers: the bot that sold its replaced 14k
   weapon plans the 62k tier directly (the sale proceeds included)
   instead of the 1k sword it just sold - the reported round the
   guard fixes.
3. **The jewel floor** - the cheapest jewel offer of every bodypart
   family (ring, earring, necklace: Magic Ring 37, Apprentice's
   Earring 56, Necklace of Magic 75, all with tax) fills the EMPTY
   jewel slots - only after a real weapon is worn: the floor gate is
   the reference price of the worn weapon, and a starter weapon (or
   none) anchors zero, so no jewel is ever bought before the armor
   is assembled and the first weapon milestone landed (the user
   rule of the opening game). The floor fills, it never replaces: a
   worn jewel blocks its family until the gate opens.
4. **The defense upgrades** - the armor, shield and (past the jewel
   gate) jewel upgrades, ranked by the raw defense gain (the
   maximum defense per buy, not per adena) and **bounded by the
   weapon budget**: the reference value of the whole worn defense
   gear after a swap may not exceed the reference price of the worn
   weapon. After every weapon tier the defense may grow inside its
   budget, the next weapon tier always outranks it. Consequence:
   the Shirt travels with the Short Sword, the wooden set with the
   Knife, the bone set with the Brandish, the wisdom jewels with
   the Long Sword. The jewel UPGRADES additionally gate on level
   15 (`jewelUpgradeLevel`): below it only the floor items are
   planned, past it the upgrades join this phase (the starting
   locations barely attack with magic, the cheapest set covers the
   mDef needs until then).

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
  duplicates beyond one copy per slot, the looted downgrades and the
  displaced weaker halves of pair swaps. Without the keep set the
  first sell batch could race the pair swap window and eat the very
  jewel the bot was putting on, and the overflow destroy (which
  ranks gear drops before stackables) could destroy a fresh upgrade
  for bag space.
- **One item per slot per trip**: each purchase marks the paperdoll
  slots it fills or clears (the family interplay included - a
  two-hander owns both hands, a one-piece owns chest and legs) and the
  later picks skip the candidates that would write into them. The
  upgrade chains are cut: a rich bot buys ONE weapon (the milestone),
  not the knife/short sword/sickle ladder in a single walk, and
  never two necklaces of which only the better one gets worn. The next
  trip re-plans against the paperdoll the purchases reached and takes
  the next step - the progression converges over the trips without
  ever paying for a step that ends up in the sale bag instead.
- **The frozen trip plan** (the 2026-09-11 two pairs of gloves
  report): the trip plans ONCE at its start
  (`maybeStartTownTrip` -> `Loop.tripPlan`) and everything after
  reads that frozen plan - the sell first step banks exactly its
  SellFirst pieces, the stop planning distributes exactly its
  purchases, and nothing re-plans in between. The re-planning shop
  this replaces computed the replacements against the trip start
  state and the buys against the freed slots and the fresh adena -
  the two plans drifted and both failure modes fired at once: the
  re-plan re-bought the Apprentice's Shoes the trip had just sold
  (the armor floor pulled the cheapest piece into the emptied feet
  slot, blocking the planned Low Boots upgrade) and it planned the
  Leather Gloves whose displaced Gloves were never queued for the
  sale (the replacement phase had already run) - the bot walked home
  wearing the new gloves with the old pair in the bag. The frozen
  plan makes the manager know exactly what and how much this trip
  buys before it walks, and the buy execution waits for every
  request's inventory confirmation as before. One honest guard
  remains at the execution point: a purchase whose item id the
  inventory already carries (a loot drop the auto equipment wore mid
  trip, a manual user purchase) drops out of the stop before any
  request goes out (`dropOwnedPurchases`) - a second copy is never
  part of the plan.
- **The top-tier slot guard** (the 2026-09-11 sold-weapon round):
  within the upgrade phases the guard records the best viable gain
  per paperdoll slot (the scan runs in the score descending order,
  so a lower score item can never outgain a higher score one on
  overlapping slots - the shared displacement subtracts the same
  scores) and drops every candidate aspired above that record,
  whatever its value per adena. The guard persists across the pick
  rounds of one plan: the eroding budget of the later rounds must
  not crowd the top tier out and push a cheaper rung in - the slot
  stays unpurchased and waits for the next trip. The floors bypass
  it (they deliberately buy the cheapest offers of the empty
  families - the opening outfit rule above) and the widget queue's
  wanted tail walks without it (the unbounded budget of the tail
  would collapse the displayed ladder of a slot to its top step and
  hide the milestones the bot saves for).

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
runs the weapon errand alone: the teach stops and the books wait for
the next trip, the retry cooldown shortens to 45 seconds
(`weaponRunCooldown`), and the hunt loop holds its fresh target picks
while the run is pending (the 2 damage fists never farm when the plan
offers a sword). The weaponless past explains the rule: the sell-first
flow once sold the worn weapon at the nearest merchant and the buy
stop walked the village behind the teacher stop - the stuck teacher
leg of the report aborted the trip with the weapon already sold, and
the bot farmed bare-handed for hours while every retry carried the
same fragile order.

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
highest gain-per-adena buys left - exactly the gear that unlocks the
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
about 345k adena at the 15 percent tax: Long Sword 156k, Bone
Breastplate + Gaiters 38k, Leather Helmet 11.7k, Low Boots 7.8k,
Leather Gloves 7.8k, Necklace of Wisdom 13.7k, Cat's Eye Earring
10.2k, two Rings of Wisdom 13.6k and the Round Shield 8.2k - and
brings 251 gear points. A character reaches that around mob level
18-20 on the elven lands income; by then the southwest forest (the
level 13-18 zone) is unlocked and pays for the D grade of the next
town.

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
./internal/swarm/gear/`; the condensed comparison (the phased column
shows the armor-floor-first order: the cheap armor opens the
journey, the weapon follows, the jewels wait for both):

| Level | WAS (greedy value/adena) | IS (phased: armor first) |
| --- | --- | --- |
| 1 | Apprentice's Shoes 8 | **Apprentice's Shoes 8** (the armor floor opens) |
| 2 | Leather Shield 34 | **Short Gloves 42** |
| 3 | Short Gloves 42, Cloth Cap 63, Magic Ring 37 | **Cloth Cap 63** (the empty armor slots are filled) |
| 4 | Necklace of Magic 75, Apprentice's Earring 56, Magic Ring 37, Cloth Shoes 42, Pants 105 | - (saving the Short Sword) |
| 5 | Short Sword 883, Apprentice's Earring 56 | **Short Sword 883** (the weapon milestone lands), Magic Ring 37, Apprentice's Earring 56, Necklace of Magic 75 (the jewel floor opens behind the weapon), Shirt 169 (the defense inside the sword budget) |
| 6 | Shirt 169, Ring of Knowledge 621, Leather Cap 1047 | Magic Ring 37 (2nd), Apprentice's Earring 56 (2nd) |
| 7 | Ring of Knowledge 621 (2nd), Short Leather Gloves 698, Cotton Shoes 698, Small Shield 733 | - (the wallet climbs the weapon ladder) |
| 8 | Leather Pants 1747, Leather Shirt 2794, Necklace of Knowledge 1242, Mystic's Earring 932 | **Heavy Chisel 9280** (the top affordable tier) |
| 9 | **Heavy Chisel 9280**, Mystic's Earring 932 | **Knife 14374** (the chisel's sell-first credit closes the gap) |
| 10 | **Knife 14374**, Earring of Strength 4036, Ring of Anguish 2691 | **Sickle 21275** (the knife's sell-first credit closes the gap) |
| 11 | **Sickle 21275**, Earring of Strength 4036, Ring of Anguish 2691 | Round Shield 8176, Leather Helmet 11730 (the defense inside the sickle budget) |
| 12 | Buckler 3196, Leather Shoes 3047, Gloves 3047, Wooden Helmet 4577, Necklace of Anguish 5382, Wooden Breastplate 9154 | - (the wallet climbs to the 62k tier) |
| 13 | Hard Leather Pants 5715, Leather Helmet 11730, Round Shield 8176, Cat's Eye Earring 10223, Low Boots 7785 | **Brandish 62214**, Bone Gaiters 14604 |
| 14 | **Brandish 62214** | Bone Breastplate 23345, Low Boots 7785 |
| 15 | Cat's Eye Earring 10223, Necklace of Wisdom 13684, Leather Gloves 7785, Bone Gaiters 14604, Ring of Wisdom 6807, Bone Breastplate 23345 | - |
| 16 | Ring of Wisdom 6807 | **Long Sword 156400**, Round Shield 8176, Necklace of Wisdom 13684, Cat's Eye Earring 10223, Mystic's Earring 932 (the defense burst inside the sword budget) |
| 17 | **Long Sword 156400**, Leather Shield 34 | Leather Gloves 7785, Cat's Eye Earring 10223 (2nd), Ring of Wisdom 6807, Ring of Anguish 2691 |
| 18 | - | Ring of Wisdom 6807 (2nd) |

The wastes the phase rework removes, visible in the WAS column:

- **The greedy filler soup**: the value per adena mixes the piece
  families with no order - the jewel floor items (Magic Ring 37 at
  level 3-4) run BEFORE the first weapon while the armor fillers
  dribble in around them, exactly the ordering the armor-first rule
  replaces (cheap armor, then the weapon, then the jewels).
- **The intermediate weapon ladder**: Heavy Chisel 9280 -> Knife
  14374 -> Sickle 21275 -> Brandish 62214 - every step resells at
  reference/2, the chisel alone wastes 5.2k adena (the buy pays
  x1.15, the sale returns x0.5).
- **The jewel ladder in magic-free zones**: Ring of Knowledge at
  level 6, Mystic's Earring at 8, Earring of Strength at 10 - mDef
  buys while nothing attacks with magic.
- **The shield ladder after the two-hander**: Leather Shield ->
  Small Shield -> Buckler -> Round Shield, one shield per trip at
  levels 17-20 while the two-hander keeps displacing them.

The IS column opens with the cheap armor floor (levels 1-3: the
shoes, the gloves, the cap - the empty slots of the Squire's kit),
buys the Short Sword the moment its wallet reaches it, the jewel
floor ONLY after the sword landed (level 5: the weapon first, the
jewels behind it) and then the TOP affordable weapon tier of every
trip: the chisel at 8, the knife at 9 through the chisel's
sell-first credit, the sickle at 10 through the knife's credit, the
brandish at 13, the Long Sword at 16 - the intermediate rungs are
bought only while they ARE the top affordable tier, never below a
reachable better one, and the replaced weapon funds the next tier
through its sale. The armor and the jewels follow inside each
tier's budget, no jewel upgrade before level 15, the defense bursts
behind the sickle, the brandish and the Long Sword - the same 251
gear points of the full dress reached without the save up trips.

## Where each piece lives

- `tools/generate_item_stats.sh` - the prices, weights and combat
  stats of every item (from `data/stats/items/*.xml`).
- `tools/generate_shop_catalogs.sh` - the buylists of every merchant
  keyed by the packet template id (from `data/buylists/*.xml` and
  the CT0 display id table).
- `internal/swarm/gear/shopping.go` - the phased planner
  (`PlanPurchases`, `PlanPurchaseQueue`: the affordable plan plus
  the wanted tail with the cumulative missing adena, the queue floor
  of `shoppingQueueMin` entries through the wishlist extension), the
  strategy phases (`shopStrategy.classify`: the armor floor, the
  weapon milestone, the jewel floor behind a real weapon, the
  defense budget), the top-tier slot guard (`viableCandidates` and
  `aspiredAbove`: the best viable gain per slot drops the
  intermediate rungs of the upgrade phases, the floors and the
  widget tail bypass it), the catalogs (`Shop`, `Catalog`) and the
  adena budget handling (the sell credit of `displacedValue` never
  counts the unsellable newbie kit - `gear.IsStarterItem`).
- `internal/swarm/gear/shopping_strategy_test.go` - the level
  journey simulation and the was/is comparison table (the legacy
  greedy planner copy, the income model, the ordering pins).
- `internal/swarm/hunt/shopping.go` - the trip trigger, the
  multi-stop buy execution, the transaction pacing and the widget
  view publish (`publishShoppingView`: the queue while hunting, the
  remaining trip buys while a town trip runs).
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
