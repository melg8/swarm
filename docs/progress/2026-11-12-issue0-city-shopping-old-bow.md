# The city shopping errand sells the replaced bow (status: in progress)

Started 2026-11-12, branch main, issue #0 (owner report).

## Goal

Owner report: when a bot owns the Forest Bow, the errand does not sell
it - the bot enters the city shopping errand, buys the Composition Bow
(273) and comes home still carrying the weaker bow.
The replaced luring tool must leave the bag at the Gludio stop that
sells the new bow, exactly like the replaced village weapon and armor
leave it at their stops.

Done means:

- the bow purchase of the errand names every owned bow weaker than the
  Composition Bow as a `SellFirst` piece of the bow stop: the stop
  sells the old tool before the new one lands (the same
  `stepReplacementSales` wiring the worn kit purchases use);
- the sale credit of the bow purchase carries the old bows' sell value
  (reference price / 2) instead of zero;
- the ordinary junk flow keeps only the strongest owned bow, so a
  weaker bow left behind by any other path is still plain junk (pinned
  by a unit test);
- the `city-shopping` acceptance starts with a Forest Bow in the bag
  and verifies the old bow left the bag during the errand;
- unit tests pin the bow sell-first line and the stop wiring;
- `docs/hunting.md` names the old bow sale.

## Context

- The errand and the bow round:
  `docs/progress/archive/2026-11-12-issue0-city-shopping-bow.md`,
  `docs/progress/archive/2026-11-11-issue0-city-shopping-sells-old-kit.md`.
- Root cause: `cityShopDisplaced` answered `(0, nil)` for the
  Composition Bow because the two hand slot belongs to the Falchion and
  the shield; the bagged Forest Bow was never named by `SellFirst`, and
  the errand stops are `sell: false`, so no junk flow of the trip saw
  it either. The bow purchase itself was correct.
- The keep set of the junk flows (`addLureToolKeeps`) already keeps the
  strongest owned bow only, so the weaker bow becomes junk once the
  Composition Bow is in the bag - but the errand had already passed its
  stops by then.
- Facts: Forest Bow 272 reference 136 000 (sell 68 000), Composition
  Bow 273 reference 244 000, Gludio price 292 800; the acceptance
  wallet 800 000 covers the 748 560 plan beside the credit.

## Progress

### 2026-11-12 - sell the replaced bow at the bow stop

- Root cause read from the sources: `cityShopDisplaced` answered
  `(0, nil)` for the Composition Bow, so the bow purchase carried no
  `SellFirst`; the errand stops run `sell: false`, so no junk flow of
  the trip saw the bagged Forest Bow either. The keep set of the junk
  flows already keeps only the strongest owned bow
  (`TestJunkFlowKeepsTheLuringBow`), so the weaker bow was junk that
  no stop sold.
- `cityShopDisplaced` now routes the bow purchase through
  `weakerBowSale`: every owned bow of a lower attack than the bought
  one, equipped or bagged, joins the `SellFirst` list at its
  referencePrice/2 credit. The existing `stepReplacementSales` wiring
  sells the old tool before the new one lands.
- Unit tests: `TestCityShopBuysCarryTheDisplacedKit` now pins the
  Forest Bow sell-first line and its 68 000 credit,
  `TestCityShopBowStopSellsTheOldBow` walks the stop (bagged bow, no
  unequip request, sale before the buy), and
  `TestCityShoppingPlanCost` pins that the weaker owned bow still
  plans the upgrade at the full price.
- Acceptance: the start state carries the Forest Bow 272, the `sold`
  check includes it (the replaced weapon, armor and bow), the bow
  latch stays as is, and the watch test pins the owner report (a
  carried Forest Bow keeps the sale check open).
- Docs: `docs/hunting.md` names the old bow sale.
- Green: `task lint:new`, the affected package tests and
  `task verify`.
- Next: the live acceptance run on the deployed stack.

## Status

In progress: offline green, the live `city-shopping` acceptance run
next, then the commit and push.
