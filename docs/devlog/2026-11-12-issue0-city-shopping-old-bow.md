# The city shopping errand kept the replaced Forest Bow

## Round

- Date: 2026-11-12
- Scope: `internal/swarm/hunt/city_shopping.go`, the city shopping
  errand of the melee hunter
- Issue: #0 (owner report, no board issue)
- Predecessor: `docs/progress/archive/2026-11-12-issue0-city-shopping-bow.md`
  (the round that added the Composition Bow to the errand) and
  `docs/progress/archive/2026-11-11-issue0-city-shopping-sells-old-kit.md`
  (the sell first step of the replaced village kit)

## Problem

The owner ran bots that already owned the Forest Bow (272) through
the city shopping errand. The errand bought the Composition Bow (273)
as planned, but the bot came home still carrying the Forest Bow: the
weaker luring tool was never sold at the Gludio stop.

## Root cause

The errand sells only the pieces a purchase lists in `SellFirst`:
`cityShopDisplaced` resolved the paperdoll occupants of the bought
item's bodypart slots (`gear.DisplacedByItem`). A bow occupies
`lrhand`, whose slot set also holds the worn Falchion and the shield
the melee dress keeps wearing, so the function answered `(0, nil)` for
the Composition Bow and the purchase carried no `SellFirst` at all.
The errand stops run with `sell: false`, so the ordinary junk flow of
the town trips never saw the bagged Forest Bow either. The junk keeps
were not the problem: `addLureToolKeeps` already keeps only the
strongest owned bow (`TestJunkFlowKeepsTheLuringBow`), so the old bow
became plain junk the moment the Composition Bow landed - but by then
the errand had passed its last stop.

## Reproduction

- Offline: a loop whose bag carries the village kit plus the Forest
  Bow; `cityShopBuys(cityShopBowStop)` answered a purchase with an
  empty `SellFirst`, and the acceptance start state without the bow
  hid the defect.
- Live: the `city-shopping` acceptance with the Forest Bow 272 in the
  start state; before the fix the bag kept the old bow after the
  Composition Bow purchase (the `sold` check counts 8 old items now).
- Detect: `grep "sell Forest Bow" <acceptance log>` on a passing run;
  the item must appear in the sell request before the Composition Bow
  buy and never in a later inventory print.

## Fix

- `cityShopDisplaced` routes the Composition Bow purchase through
  `weakerBowSale`: every owned bow of a lower attack than the bought
  one, equipped or bagged, joins `SellFirst` at its
  referencePrice/2 credit (68 000 for the Forest Bow). The existing
  `stepReplacementSales` wiring of the stop sells the old tool before
  the new one lands; the bagged bow needs no unequip request.
- The errand plan gate is unchanged: the wallet must cover the full
  plan beside the old tool, the sale credit is a bonus, not a
  budget (the same conservative rule the village kit sale follows).
- The acceptance start state carries the Forest Bow and the `sold`
  check counts it (the replaced weapon, armor and bow); the watch
  test pins that a carried Forest Bow keeps the check open.
- Unit tests: the bow sell first line and credit
  (`TestCityShopBuysCarryTheDisplacedKit`), the bagged sale before the
  buy (`TestCityShopBowStopSellsTheOldBow`), the unchanged plan cost
  (`TestCityShoppingPlanCost`) and the watch latch
  (`TestCityShoppingWatchSellsTheOldKit`).

## Verification

- `task verify` green (build, vet, the full lint at zero findings, the
  full test suite, `fmt:check`).
- Live `city-shopping` acceptance:
  `logs/acceptance/city-shopping-20261112-145518-24936-1.log`,
  PASSED after 7m52s, 9 of 9 checks. The log shows the story: at
  14:57:46 the bow stop opened with 1 of 8 old pieces left (the
  Forest Bow), at 14:57:47 the stop offered it for sale, at 14:57:48
  the `sold` check latched, at 14:57:49 the Composition Bow buy went
  out, and no inventory print after the sale carries the item 272.

## Follow ups

- The errand still does not count sale credits in its wallet gate
  (`cityShoppingPlan`): a bot a few thousand adena short of the full
  plan waits for the next errand although the replaced pieces would
  cover the gap. Same conservative rule for the village kit; left
  alone until the owner asks.
