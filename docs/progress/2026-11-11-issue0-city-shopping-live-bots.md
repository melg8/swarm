# The city shopping errand runs on the live bots (status: in progress)

Started 2026-11-11, branch main, issue #0 (owner request: continue the
city shopping work, but now let the real bots use it - if they have
enough funds, they buy the top no-grade weapon and armor in town).

## Goal

A live `-hunt` bot that owns a village weapon and whose wallet covers
the missing top no-grade kit walks to the Elven Village gatekeeper,
teleports to Gludio, sells the village pieces the purchases displace,
buys the Falchion and the whole Bronze/Bone armor set, wears it, rides
home and resumes the hunt. No acceptance-only seam: the production
loop decides on its own, the way the ordinary town trips do.

Done means:

- the errand is on by default for the elven region and every live
  faithful launch (`cmd/swarm -hunt`) runs it;
- it stays out of the Dion region (its shops and its walks belong to
  the 20-25 band) and out of non melee gear profiles (the fixed plan
  is the Falchion and the shield, useless to a bow or staff class);
- it never starts mid fight: a live attacker or a pending loot holds
  it, a merely held target drops before the walk;
- it reaches the gatekeeper from the far elven cells: the scroll of
  escape leg the ordinary town trip already prices is reused, and the
  driver budget covers the longest walks the registry can start from;
- the `city-shopping` acceptance passes with the seam removed (the
  production gate, not a test hook, starts the errand);
- unit tests pin the default gate, the region and profile fences, the
  fight hold and the scroll leg;
- `docs/hunting.md` carries the production policy.

## Context

- The errand driver (`internal/swarm/hunt/city_shopping.go`) was added
  by the acceptance round and verified live on 2026-11-11
  (`docs/progress/archive/2026-11-11-issue0-city-shopping-errand.md`),
  then the sell first step landed
  (`docs/progress/archive/2026-11-11-issue0-city-shopping-sells-old-kit.md`).
  Both rounds kept the `SetCityShoppingEnabled` seam off in production.
- The route and the stock are verified Mobius C1 facts: Mirabel of the
  Elven Village teleports to Gludio (9 200 adena, free at level <= 40,
  `MaxFreeTeleportLevel = 40`), Bella of Gludio teleports back, Sydnia
  sells the Falchion (list 3032100) and Simplon the whole no-grade
  armor set (list 3025300). The 20 percent Gludio buy tax prices the
  kit at 455 760 adena.
- The trigger gates already exist: a village weapon worn or bagged
  (`cityShoppingArmed`), the missing kit priced against the live bag
  (`cityShoppingMissingCost`), the wallet covering it, a 10 minute
  retry cooldown and no running trip.
- The acceptance scenarios all start below the kit price except the
  city shopping one (the highest other wallet is 100 000), so the
  production default cannot hijack another story. The acceptance
  runner always installs the elven region.
- The elven cell registry spreads the level 12-16 grounds 18 000 to
  42 000 units from Mirabel and the level 17-19 grounds up to 53 000:
  the walk alone can take most of an eight minute budget, which is
  why the scroll leg and the wider budget are part of this round.
- The scroll of escape economy (`hunt/soe.go`) is the town trip's
  existing answer to the same walk: it teleports to the nearest
  village (the elven respawn sits beside Mirabel), costs one scroll
  and runs only when the measured ground income makes the saved
  walking worth the scroll.

## Progress

### 2026-11-11 - the seam prep

- Extracted the per stop install and clear of the errand
  (`cityShopOpen`/`cityShopClose`), the quest waypoint click
  (`clickQuestWaypoint`) and the gatekeeper list open
  (`openTransferList`), so the production round can reuse them and
  the stop cleanup stays one function.
- Verified: `go build ./...`, the hunt and acceptance package tests,
  `task lint:new` (0 issues).

### 2026-11-11 15:32 UTC - the production gate

- The errand runs on the production trigger: the `cityShopping` flag
  and `SetCityShoppingEnabled` are gone. `cityShoppingWanted` gates on
  the elven region (empty counts as elven) and the melee gear profile
  (the C1 Falchion carries a magic attack, so the weapon score alone
  would admit the caster). The acceptance scenario now rides the
  production gate: its hook is removed.
- The fight gate matches the town trip: a pending loot or an incoming
  hit holds the start, a merely held target drops
  (`dropAttackTarget`) before the walk.
- The scroll of escape leg is reused: `maybeEscapeWithSOE(merchant)`
  became a wrapper over `maybeEscapeTo(x, y)`, and the errand prices
  the walk to Mirabel through it. The event text no longer says "walk
  home" for an outbound leg.
- `cityShopBudget` 8 -> 30 minutes: the far elven cells sit up to
  53 000 units from the village, near the 10 minute quest walk
  timeout per walking segment.
- The armed scan moved last in `cityShoppingWanted`: it copies the
  inventory, so a poor or fully dressed bot never allocates on the
  tick.
- `docs/hunting.md` carries the production policy section.
- Tests: the gate matrix gained the elven/Dion and profile fences,
  the combat hold and drop cases, and a scroll test through the new
  coordinate entry.
- Verified: `go build ./...`, the hunt, acceptance and gear package
  tests, `task fmt`, `task fmt:check`, `task lint:new` (0 issues).

Next: check the stack, run the live `city-shopping` acceptance with
the production trigger, then `task prepush` and push.

### 2026-11-11 15:44 UTC - the live acceptance: PASS

The production-trigger run of `-acceptance city-shopping` on the
deployed stack passed the whole check list: the Gludio arrival, the
Falchion purchase, the armor set purchase, the sale of the replaced
village pieces, the worn kit, the walk home and the resumed farm.
The bot then left the world gracefully (`Acceptance: PASS`). The
scenario armed nothing: the errand started from `cityShoppingWanted`
on the first eligible tick, and the standard restock trip plus the
first kills followed the return.

Next: `task prepush`, rebase on `origin/main`, push the round, then
move the fragment to the archive.
