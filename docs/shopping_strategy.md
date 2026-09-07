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

**Rule 1 - value per adena decides every purchase.** The marginal
value of a purchase is the score gain it brings to the paperdoll
(the melee fighter scoring: weapon pAtk x attack speed, armor pDef,
jewel mDef, shield expected block value). The planner picks the
purchase with the highest `gain / price` first, applies it to a
virtual paperdoll and repeats. Consequences that match the data:

- The **empty slot fillers come first**: Apprentice's Shoes are 8
  pDef for 8.05 adena (0.99 pDef per adena) - the single best buy of
  the whole village. Cloth Cap, Short Gloves, Magic Ring follow in
  the 0.18-0.21 range. A fresh character with 100-500 adena fills
  every slot long before it can afford a real weapon.
- The **weapon ladder waits for the wallet**: Short Sword (883 with
  tax) brings 3 pAtk over bare fists - good value once the fillers
  are done; the Long Sword (156k) only wins when nothing cheaper
  remains.
- **Nothing is bought twice** and **nothing the inventory already
  carries is bought** (the free upgrades are simulated first, the
  purchases compare against the paperdoll the auto equipment will
  reach anyway).

**Rule 2 - buy at the town trip, sell first.** The purchases run
inside the town trips the loop already makes: the junk selling frees
the slots and the adena first, the purchase plan is recomputed with
the fresh numbers, then the bot walks to every merchant of the plan
(buy groups order by walking distance; a group of the merchant the
character already stands at buys without an extra walk). A trip is
worth it when the plan totals at least 100 adena - below that the
walking time costs more than the gains.

**Rule 3 - the gear feeds the zone ladder.** The hunting zones gate
on gear points (`gear.TotalGearPoints`: the weapon damage per hit
plus the defenses, in character-stat-sheet units). The intended
ladder of the elven lands:

| Zone | Mob levels | Gear gate | The gear that passes it |
| --- | --- | --- | --- |
| Elven Village Keltir Field | 1-4 | 0 | anything (fists work) |
| East Forest Goblin Camp | 5-7 | 40 | Short Sword + Shirt |
| West Kaboo Woods | 8-12 | 110 | Broadsword + the wooden set |
| Southwest Dryad Forest | 13-18 | 200 | Gladius/Long Sword + the bone set + jewels |

The strategy therefore aims the spending at the *next* gate: while
the character farms the goblins (level 5-7), the planner saves into
the Broadsword and the wooden set pieces because those are the
highest gain-per-adena buys left - exactly the gear that unlocks the
kaboo woods, where the income doubles again.

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

## Where each piece lives

- `tools/generate_item_stats.sh` - the prices, weights and combat
  stats of every item (from `data/stats/items/*.xml`).
- `tools/generate_shop_catalogs.sh` - the buylists of every merchant
  keyed by the packet template id (from `data/buylists/*.xml` and
  the CT0 display id table).
- `internal/swarm/gear/shopping.go` - the greedy planner
  (`PlanPurchases`), the catalogs (`Shop`, `Catalog`) and the adena
  budget handling.
- `internal/swarm/hunt/shopping.go` - the trip trigger, the
  multi-stop buy execution and the transaction pacing.
- `internal/swarm/hunt/equip.go` - the auto equipment that wears
  everything the trips buy (and loot drops) immediately.
