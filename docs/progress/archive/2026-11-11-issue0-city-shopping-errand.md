# The acceptance city shopping errand (status: complete)

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

### 2026-11-11 14:45 UTC - the homebound teleport and the fail fast

What the live run showed (13:57-14:31 UTC, account temp25): the walk
to Mirabel, the outbound ride (arrived 13:59:28), both merchant stops
(13:59:58), the dress (14:00:00) all landed; the ride home failed at
14:00:49 with `the teleport to Elven Village never landed`, and the
bot then idled out the remaining half hour of the scenario budget
without a single log line.

Root cause (source verified in `RequestBypassToServer` and
`FloodProtectorAction`): the homebound teleport bypass rode the
`showTeleports` bypass inside the server bypass flood protector
window (3 game ticks of 100 ms = 300 ms), so the server dropped it
before the `npc_` branch - no `ActionFailed`, no teleport. The
outbound hop passed only because its list page arrived late enough
(the run lost four `ActionFailed` on that page); the homebound round
trip was fast enough to ride under the window.

What changed:

- `bypassPacer` in `hunt/quest_walker.go`: one pace of
  `dialogBypassPace` (3.2 s) between the bypass sends of a
  conversation, shared by `DriveDialog` and `driveQuestTransfer`.
- `driveQuestTransfer` gates both page waits on the html arrival
  generation (a page left from an earlier talk never satisfies a
  wait), sends the teleport command the list button carries verbatim,
  and logs the geometry (`via "npc_..._teleport ..."`, self/npc
  coords, 2D/3D distance) so a stuck hop names the flood, the
  250-unit 3D range gate or a stale page.
- The arrival failure names the character position against the
  arrival square.
- `state.HuntErrand` (name, start, end, error) plus
  `SetHuntErrand`/`HuntErrand` and the session reset: the errand
  publishes its status race-free; the acceptance watch reads it.
- Fail fast: the driver runs under `cityShopBudget` (8 minutes) and
  reports `the errand ran past its ... budget`; the watch aborts on a
  stopped errand, an errand past `cityShoppingErrandWall` (12
  minutes) or a return ground silent for `cityShoppingFarmWindow` (4
  minutes); the scenario budget drops 30 -> 15 minutes.
- New tests: the pacer spacing, the stale-page gate, the errand
  status publication (a tracker with no self position fails the
  errand and publishes the error), the errand status round trip and
  session reset, and the watch abort matrix.

Note: `DriveGatekeeperTeleport` in `gatekeeper_step.go` carries the
same unpaced two-send pattern but is not wired into the tick machine;
left untouched.

Verified: `go build ./...`, the hunt, state and acceptance package
tests, `task fmt`, `task fmt:check`, `task lint:new` (0 issues),
`task prepush`.

Next: commit, then rerun `-acceptance city-shopping` live.

### 2026-11-11 15:05 UTC - the live rerun: the errand passed

The rerun (14:46-14:54 UTC, account temp25) proved the pacing fix: the
outbound hop logged `via "npc_268439795_teleport NORMAL 0"` (self/npc
2d 199, 3d 200) and landed at 14:48:14; Sydnia sold the Falchion and
Simplon the six armor pieces by 14:48:43; the dress completed at
14:48:45; the homebound hop logged `via "npc_268441673_teleport NORMAL
5"` and landed at 14:49:18; the walk back reached the start ground and
the errand finished at 14:50:53. Every errand stage is now live green.

The resumed farm check then failed at 14:54:36 with `the hunt did not
resume within 4m0s of the return`: the ordinary hunt opened the
standard restock trip right after the errand (the pull Bow, arrows,
the Scroll of Escape, the Newbie Guide buffs) and was still walking
back to the ground when the window lapsed. The restock is real bot
behavior, not a failure of the errand, so the farm window grows to
cover it: `cityShoppingFarmWindow` 4 -> 10 minutes, scenario budget
15 -> 20 minutes.

### 2026-11-11 15:20 UTC - the second live rerun: the latches

The rerun (14:59-15:19 UTC) repeated the whole errand green: outbound
hop landing 15:01:16, both merchant stops by 15:01:46, the dress at
15:01:48, the homebound hop landing 15:02:20, the errand finished
15:03:56, the restock trip back on the ground 15:08:04 and kills at
15:08:18, 15:08:32 and 15:08:43. The farm check latched at 15:08:18
("the experience grew after the return").

The run still hit the 20 minute budget: the `gludio` and `worn`
checks were live predicates, so leaving Gludio reopened `gludio`
(15:02:20) and the paperdoll swaps on the later town trips reopened
`worn` (15:09:14, 15:09:44, 15:14:52), leaving `allChecksDone` false
forever. Both are history facts the run already proved, so the watch
now latches them (`gludioSeen`/`wornSeen` with their details) and a
later move or gear swap never reopens them. New test
`TestCityShoppingWatchLatchesTheStory` pins the latch.

### 2026-11-11 15:36 UTC - the final live run: PASS

The final run (15:21-15:30 UTC) passed in 8m52s: the Gludio hop
landed 15:23:15, both merchant stops and the dress completed by
15:23:47, the homebound hop landed 15:24:19, the walk back reached
the ground and the errand finished 15:25:54, the standard restock
trip followed, and the first kill landed 15:30:20 - the farming check
latched and the scenario stopped the bot. The run log records
`[verdict] PASSED after 8m52.109s`, 7 of 7 checks done.

Acceptance criteria met. The seam stays off in every production
launch (`SetCityShoppingEnabled` is armed only by the scenario hook),
so the live bots keep the ordinary town trip.

## Status

Complete: the live `city-shopping` scenario passes every stage of the
owner story; the fragment moves to the archive.
