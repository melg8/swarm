# The city shopping errand sells the replaced village gear (status: complete)

Started 2026-11-11, branch main, issue #0 (owner request).

## Goal

The `city-shopping` acceptance errand must sell the village weapon and
armor the top no-grade purchases replace, at the Gludio merchants of
the errand, instead of leaving the old dress in the bag until the
ordinary restock trip sells it as junk. Acceptance: the errand's live
run sells the old kit before the replacements land and the new
"sold" check passes together with the rest of the story.

## Context

- Owner report: the acceptance test of the city shopping trip does not
  sell the old weapon and armor, although it must.
- The errand driver (`internal/swarm/hunt/city_shopping.go`,
  `cityShopAt`) installed `tripStop{... sell: false}` and built the
  purchases with `SellFirst: nil`, so the shared sell first step
  (`stepReplacementSales`, the one the ordinary town trip runs) never
  fired. The previous live log
  (`logs/acceptance/city-shopping-20261111-171554-26244-1.log`)
  proves the defect: at 17:21:57, after the errand ended, the
  ordinary restock trip sold the seven old pieces (items 2, 24, 31,
  44, 50, 38, 102) as junk.
- The old kit is sellable: the starter filter covers only
  {10, 1146, 1147, 2369} (`gear/starters.go`), none of the village
  dress.
- The errand's fixed plan cannot use the ordinary planner (it scores
  by value per adena and would pick D-grade pieces), but it can reuse
  the planner's displaced-piece pricing: the paperdoll occupants of
  the purchased item's bodypart slots at half the reference price
  (`gear.displacedValue`).
- The acceptance watch must latch the sale only while the errand is
  open (`tracker.HuntErrand()` named `city-shopping` and
  `EndedAt.IsZero()`): the later restock trip sells the leftovers and
  would otherwise turn a broken errand green.

## Progress

### 2026-11-11 - sell the displaced kit at each city stop

- Added `gear.DisplacedByItem`: the sell credit and the object ids of
  the paperdoll occupants an item's bodypart slots displace (the same
  `displacedValue` pricing the planner uses, starter items excluded).
- `cityShopBuys` became a `Loop` method and fills `SellFirst` /
  `SellCredit` of every kit purchase from the live equipment.
- `cityShopAt` now installs the purchases as the frozen `tripPlan`,
  resets the replacement sales state and `sold` map per stop, and
  drives the stop through `cityShopTrade`: the sell first step of the
  displaced pieces must confirm before `tickStopShopping` runs, so
  the old weapon sells at Sydnia before the Falchion lands and the
  old armor plus shield sell at Simplon before the armor set lands.
  The stop cleanup clears the plan and the replacement state.
- Unit tests: `TestDisplacedByItemPricesTheWornOccupant` (gear),
  `TestCityShopBuysCarryTheDisplacedKit` and
  `TestCityShopTradeSellsBeforeTheBuy` (hunt).
- Acceptance: the `sold` check ("the replaced weapon and armor were
  sold") follows the armor check; it latches when the new weapon and
  armor are in the bag, no old piece of
  `cityShoppingOldGear` is carried, and the errand is still open.
  Covered by `TestCityShoppingWatchSellsTheOldKit` (the latch, the
  leftover detail, and the ended errand never turning it green).

### 2026-11-11 - the live acceptance run

- `go run ./cmd/swarm -acceptance city-shopping -login 127.0.0.3:2106
  -web ""` passed all 8 checks in 8m58s
  (`logs/acceptance/city-shopping-20261111-175117-15172-1.log`).
- The log proves the timing: at 17:53:28 Sydnia sold the Long Sword
  (object 164441729, one `RequestSellItem` of 1 entry) and at 17:53:31
  the Falchion buy went out; at 17:53:31-42 Simplon unequipped and
  sold the six old armor pieces (one `RequestSellItem` of 6 entries)
  and at 17:53:44 the armor set buy followed. The errand ended at
  17:55:52, so the old dress was sold inside the errand; the later
  restock trip at 17:57:25 found no junk to sell.
- The `sold` check latched with "the old village gear left the bag at
  Gludio".

## Status

Complete: the fix, the offline tests and the live acceptance are in.
