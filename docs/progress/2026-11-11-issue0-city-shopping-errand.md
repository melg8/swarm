# The acceptance city shopping errand (status: in progress)

Started 2026-11-11, branch main, issue #0 (owner request).

## Goal

Verify the owner scenario on the live stack: a character that already
wears the village dress and has saved 500 000 adena walks to the
Elven Village gatekeeper, teleports to Gludio, buys the top no-grade
weapon and armor set, rides back to the Elven Village and resumes the
hunt on its ground. Acceptance only: the seam stays off in every
production launch, so the live bots keep the ordinary town trip.

Acceptance criteria: the `city-shopping` scenario reaches every check
(done): entered world, Gludio arrival, weapon bought, armor set
bought, the whole kit worn, back home, farm resumed. Build with the
seam so a later round can enable the behavior outside the test.

## Context

- The route and the stock are verified Mobius C1 facts: Mirabel of
  the Elven Village teleports to Gludio ("The Town of Gludio",
  arrive -12694 122776 -3114, 9 200 adena - free at level <= 40,
  `MaxFreeTeleportLevel = 40`); Bella of Gludio teleports back to the
  Elven Village (arrive 46890 51531 -2976); Sydnia (display 7321,
  spawn -15238 123282 -3107, stand -15238 123362 -3104) sells the
  Falchion on list 3032100; Simplon (display 7253, spawn -15132
  123282 -3107, stand -15132 123362 -3104) sells the whole armor set
  on list 3025300.
- Prices at the Gludio 20 percent buy tax: weapon 292 800, chest
  59 040, legs 36 840, head 22 080, gloves 14 760, feet 14 760,
  shield 15 480 - 455 760 for the full kit, 44 240 left of the
  500 000 wallet.
- Every top-NG piece outscores the village piece: Falchion 31 vs
  Long Sword 24 pAtk; Bronze Breastplate 68/43/26/17/17 vs Bone
  50/32/23/15/15; Bone Shield 90 vs Round Shield 79 sdef (the melee
  profile's `gear.NextUpgrade` dresses the bought kit).
- The driver (`hunt/city_shopping.go`) blocks inside the hunt tick
  after the delevel block and before the town trip; it reuses the
  quest trip engine (`walkToQuestPoint`, `FindQuestNpc`,
  `driveQuestTransfer`, `EquipBaggedGear`) and the town shopping
  pipeline (`tickStopShopping` against one installed `tripStop`).
- The gate is `cityShoppingArmed` (a village weapon worn or bagged)
  plus full missing-kit affordability, so the ordinary town trip
  cannot claim the session first.
- Scenario fixture: Kaboo Orc Grunt SW-5 cell (36000 50229 -3456),
  level 15 (cell median 9, no delevel trigger), 500 000 adena,
  village kit in the bag, account temp25 (temp20..24 belong to the
  archer fleet), budget 30 minutes.
- The teleport arrival is checked within 3500 units of the Gludio
  square, the return within 3000 units of the start ground, the
  resumed farm by a kill mark stamped after the return or the
  experience growth of it.

## Progress

### 2026-11-11 10:56 UTC - (uncommitted round)

What changed: the seam driver in `hunt/city_shopping.go`, its tests,
the `phaseCityShop` phase plus the loop insert, the `city-shopping`
acceptance scenario with its watch, the registry row at the head (the
full-dress and village-escape slot tests moved with it), the web UI
phase labels and the stats phase code.

Verified: `go build ./...`, `task fmt`, `task fmt:check`, `task
lint:new` (0 issues), `task prepush`, the hunt, acceptance and
webserver package tests.

Next: commit the round, then run the live scenario with
`-acceptance city-shopping` against the deployed stack and confirm
every check reaches done.

## Status

In progress: code complete and green offline, the live run pending.
