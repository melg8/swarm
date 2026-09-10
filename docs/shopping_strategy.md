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

**Rule 1 - the purchase phases: the jewel floor, the weapon
milestone, the defense upgrades.** The planner walks the candidates
in three phases; every pick ranks by its phase first, the phase
specific order second (the melee fighter scoring stays: weapon pAtk x
attack speed, armor pDef, jewel mDef, shield expected block value):

1. **The jewel floor** - the cheapest jewel offer of every bodypart
   family (ring, earring, necklace: Magic Ring 37, Apprentice's
   Earring 56, Necklace of Magic 75, all with tax) fills the EMPTY
   jewel slots. The starting locations barely attack with magic (the
   keltirs, wolves, goblins and kaboo orcs of the elven lands are
   pure melee - their magical attack data is zero), so the cheapest
   set covers the mDef needs until level 15 (`jewelUpgradeLevel`):
   below that level NO jewel upgrade is ever planned, past it the
   jewel upgrades join the defense phase below. The floor fills, it
   never replaces: a worn jewel blocks its family until the gate
   opens.
2. **The weapon milestone** - the best value STRICT weapon upgrade
   (the highest `gain / price` among the weapons that beat the worn
   one) is the saving target. Only that one weapon is eligible: a
   cheaper but worse value weapon never intercepts the wallet, so
   the bot either buys the milestone or keeps the money. The
   milestone ladder of the elven catalogs plays out as Short Sword
   (883) -> Knife (14374, daggers swing faster: 10 pAtk x 433 beats
   the Broadsword's 11 x 379 per adena) -> Brandish (62214, the two
   hand sword: 21 x 325, the best value of the 54k tier) -> Long
   Sword (156400).
3. **The defense upgrades** - the armor, shield and (past the jewel
   gate) jewel upgrades, ranked by the raw defense gain (the
   maximum defense per buy, not per adena) and **bounded by the
   weapon budget**: the reference value of the whole worn defense
   gear after a swap may not exceed the reference price of the worn
   weapon. A starter weapon (or none) anchors zero - the first real
   weapon comes before any armor buy; after every weapon tier the
   defense may grow inside its budget, the next weapon tier always
   outranks it. Consequence: the Shirt travels with the Short Sword,
   the wooden set with the Knife, the bone set with the Brandish,
   the jewels with the Long Sword.

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

**Rule 2 - buy at the town trip, sell first - and sell everything.**
The purchases run inside the town trips the loop already makes: the
junk selling frees the slots and the adena first, the purchase plan is
recomputed with the fresh numbers, then the bot walks to every
merchant of the plan (buy groups order by walking distance; a group of
the merchant the character already stands at buys without an extra
walk). A trip is worth it when the plan totals at least 100 adena -
below that the walking time costs more than the gains. **Every vendor
visit sells the whole accumulated junk, not only past the 50 percent
inventory trigger**: the selling ends when nothing sellable is left,
so a buy trip never leaves the bag half full of sellable drops (the
bot would farm with them and walk back for the sale later otherwise).
A trip also never interrupts a running fight: it waits for the kill,
the loot pickup and the between-fights window, because the drops of
the kill are the point of the fight.

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
./internal/swarm/gear/`; the condensed comparison:

| Level | WAS (greedy value/adena) | IS (phased) |
| --- | --- | --- |
| 1 | Apprentice's Shoes 8 | - (saving the floor) |
| 2 | Leather Shield 34 | Magic Ring 37 |
| 3 | Short Gloves 42, Cloth Cap 63, Magic Ring 37 | Magic Ring 37 (2nd), Apprentice's Earring 56 |
| 4 | Necklace of Magic 75, Apprentice's Earring 56, Magic Ring 37, Cloth Shoes 42, Pants 105 | Apprentice's Earring 56 (2nd), Necklace of Magic 75 |
| 5 | Short Sword 883, Apprentice's Earring 56 | **Short Sword 883**, Cloth Cap 63, Leather Shield 34, Cloth Shoes 42, Short Gloves 42, Pants 105 |
| 6 | Shirt 169, Ring of Knowledge 621, Leather Cap 1047 | Shirt 169, Pants 105 |
| 7 | Ring of Knowledge 621 (2nd), Short Leather Gloves 698, Cotton Shoes 698, Small Shield 733 | - (saving the Knife) |
| 8 | Leather Pants 1747, Leather Shirt 2794, Necklace of Knowledge 1242, Mystic's Earring 932 | - (saving the Knife) |
| 9 | **Heavy Chisel 9280**, Mystic's Earring 932 | **Knife 14374**, Leather Shirt 2794, Wooden Helmet 4577 |
| 10 | **Knife 14374**, Earring of Strength 4036, Ring of Anguish 2691 | Hard Leather Pants 5715, Short Leather Gloves 698 |
| 11 | **Sickle 21275**, Earring of Strength 4036, Ring of Anguish 2691 | - (saving the Brandish) |
| 12 | Buckler 3196, Leather Shoes 3047, Gloves 3047, Wooden Helmet 4577, Necklace of Anguish 5382, Wooden Breastplate 9154 | **Brandish 62214**, Cotton Shoes 698 |
| 13 | Hard Leather Pants 5715, Leather Helmet 11730, Round Shield 8176, Cat's Eye Earring 10223, Low Boots 7785 | Bone Breastplate 23345, Low Boots 7785, Leather Gloves 7785 |
| 14 | **Brandish 62214** | Leather Helmet 11730 |
| 15 | Cat's Eye Earring 10223, Necklace of Wisdom 13684, Leather Gloves 7785, Bone Gaiters 14604, Ring of Wisdom 6807, Bone Breastplate 23345 | Necklace of Anguish 5382 (the jewel gate opens) |
| 16 | Ring of Wisdom 6807 | **Long Sword 156400**, Round Shield 8176, Cat's Eye Earring 10223, Earring of Strength 4036, Ring of Wisdom 6807, Ring of Anguish 2691, Necklace of Wisdom 13684 |
| 17 | **Long Sword 156400**, Leather Shield 34 | Cat's Eye Earring 10223 (2nd), Bone Gaiters 14604, Ring of Wisdom 6807 (2nd) |

The wastes the rework removes, visible in the WAS column:

- **The cheap filler detour**: 10 non-weapon buys (372 adena of
  shoes, gloves, caps, shields) run before the first weapon at
  level 5 - a whole sword tier of income spent on pieces that score
  nothing against the mob ladder.
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

The IS column buys the jewel floor once (levels 2-4), one weapon
per tier with the armor inside each tier's budget, no jewel upgrade
before level 15 and the defense burst (the bone set, the shield,
the wisdom jewels) inside the Long Sword budget at level 16-17 -
the same 251 gear points of the full dress reached without the
detours, roughly 60k adena (about 17 percent of the journey income)
saved by level 17.

## Where each piece lives

- `tools/generate_item_stats.sh` - the prices, weights and combat
  stats of every item (from `data/stats/items/*.xml`).
- `tools/generate_shop_catalogs.sh` - the buylists of every merchant
  keyed by the packet template id (from `data/buylists/*.xml` and
  the CT0 display id table).
- `internal/swarm/gear/shopping.go` - the phased planner
  (`PlanPurchases`, `PlanPurchaseQueue`: the affordable plan plus
  the wanted tail with the cumulative missing adena), the strategy
  phases (`shopStrategy.classify`: the jewel floor, the weapon
  milestone, the defense budget), the catalogs (`Shop`,
  `Catalog`) and the adena budget handling.
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
