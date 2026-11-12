# The city shopping errand buys the top no-grade bow (status: in progress)

Started 2026-11-12, branch main, issue #0 (owner request).

## Goal

The city shopping errand (`hunt/city_shopping.go`) already buys the
top no-grade melee kit at the Gludio shops. Add the top no-grade bow
to the errand: when the wallet covers it, the bot buys the
Composition Bow (273) behind the complete kit, so the melee lurer
gets its ranged tool in the same trip instead of waiting for the
ordinary bow ladder of the town trips. The bow never delays the
kit - a wallet that covers only the melee kit buys the kit and
leaves the bow for a later errand, once the saved adena reaches it.

Done means:

- the errand plans the bow only for the melee lurer, only when the
  owned bow is weaker than the Composition Bow, only when the melee
  kit is complete (owned or bought in this trip) and only when the
  wallet covers the kit plus the bow;
- the bow rides a third Sydnia stop walked after the melee kit
  stops, the same weapon list 3032100; the purchase displaces
  nothing (the errand keeps the melee dress on, the two hand tool
  would drop the Falchion and the shield);
- a kit-complete character with the bow missing and the bow money
  in the wallet runs the errand for the bow alone;
- the `city-shopping` acceptance starts with 800 000 adena (the
  melee kit 455 760 plus the bow 292 800) and verifies the bow in
  the bag beside the weapon, the armor, the old gear sale, the worn
  kit, the return and the resumed farm;
- unit tests pin the plan gate, the stop order, the price and the
  no-displacement buy line;
- `docs/hunting.md` carries the bow rule.

## Context

- The errand and its live acceptance round are complete:
  `docs/progress/archive/2026-11-11-issue0-city-shopping-live-bots.md`,
  `docs/progress/archive/2026-11-11-issue0-city-shopping-sells-old-kit.md`.
- The stock is a verified Mobius C1 fact: Sydnia (display 7321) sells
  list 3032100, which carries 273 Composition Bow (64 pAtk,
  244 000 reference price, no `crystal_type` = no grade); the Gludio
  20 percent buy tax prices it at 292 800. The item 274 Strengthened
  Bow is D grade (`crystal_type D`) and stays out of the plan.
- The melee kit is 455 760 adena: Falchion 292 800, the armor set
  162 960.
- The ordinary town trip already plans the luring bow upgrade
  (`gear.planBowUpgrade`, best affordable bow strictly stronger than
  the owned one); the village list 3014700 stops at the Forest Bow
  272, so the Composition Bow is an errand-only piece.
- The errand is melee only (`cityShoppingProfileAccepts`), the bow
  lurer contract (`gear.BowLurer`) covers exactly the melee fighter.
- The acceptance stack on the Windows host is up (login
  127.0.0.3:2106, game :7777, MariaDB :3306).

## Progress

### 2026-11-12 - the task start

The fragment, the plan above. Next: the gear export and the errand
plan.

### 2026-11-12 - the errand buys the bow

- `gear.OwnedBow` is exported; the luring bow upgrade reads it too.
- `hunt/city_shopping.go`: the static stops split into the weapon and
  armor stops, `cityShopBowStop` added (Sydnia, list 3032100, item
  273); `CityShoppingBowID` exported; `cityShoppingBowUpgrade` prices
  the owned tool, `cityShoppingPlan` resolves the run cost and the bow
  inclusion (an unaffordable plan answers zero), `cityShoppingPlanStops`
  freezes the route (owned lines dropped, empty stops skipped, bow
  last); the drive walks the frozen stops; the bow buy carries no
  `SellFirst`/`SellCredit` (`cityShopDisplaced`).
- `hunt/equip.go`: `ownedBowAttack` reads the best owned bow through
  a per inventory version cache of the gear manager, so the per tick
  errand gate never copies the bag in the steady state.
- `hunt/city_shopping_test.go`: the plan cost matrix, the route order,
  the bow price (748 560 plan), the bow buy line, the gate matrix with
  the kit-only wallet and the bow-only errand, the bow scan cache.
- The acceptance scenario: 800 000 adena, the `bow` check after the
  armor (latched), the home gate waits for the bow, the description
  and the tests follow the new check order.

## Status

In progress: the live acceptance run and the gate.
