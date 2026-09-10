# Agent progress log

Crash-safe task tracking: the current task, its full context and
per-commit progress live here (see the "Work protocol" section in
AGENTS.md). Entries are append-only; a new agent resumes the newest
unfinished entry.

This file carries ONLY the active and the most recent context:
finished task entries and older progress streams move to
`agent_progress_archive.md` (append-only, same order). The permanent
root-cause history of every round lives in `docs/development_log.md`;
check the archive when the recent context references an older task.

## Active task: the delevel water loop - the walk the planner planned as a swim

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian): the bot hangs cycling forever.
The state dump (build 6255088) shows the exact cycle, 1.3 s per turn,
25+ minutes non stop: "level 11 is too high for level 2 mobs,
deleveling to 9 at the town guards" -> "walking to the guard Starden"
-> "the walk would enter water at 40648 43432, re-pathing around the
shore (1..3 of 3)" -> "town trip ended: aborted, the walk would cross
water" -> the deleveling restarts. The character (test3, level 11)
stands dry at the elven village shore (40648 43432 -3624), HP full,
not moving, the hunting zone behind the bay.

### Root cause

Two defects chained:

1. The planner planned water crossing routes for the shore walks: the
   water step of the search only costs three land steps
   (waterCostMultiplier), so the cheapest route from the shore to the
   guard swam across the bay (the dump walk plan: wp1 z -3800, below
   the water level -3780). The click guard of the walker
   (clickWouldEnterWater) refused the first leg, and the "re-path
   around the shore" re-planned the IDENTICAL route - the search is
   deterministic and its water tolerance did not change - three times,
   then aborted. The planned wet route and the refusing walker could
   never agree.
2. The abort of a walk that runs during the deleveling ended only the
   town trip: abortTownTrip left the delevel state armed without any
   cooldown, the phase flipped to engage, and the very next tick
   delevelWanted() fired again (level 11 over median 2, no cooldown)
   -> startDelevel -> the identical wet plan -> the identical abort.
   An infinite tight loop.

### Fix

- pathfind: the search carries a dry mode (FindPathApproachDry) - a
  step onto an underwater cell costs impassable, so every waypoint of
  a found route stands above the water level (a wet start exits to the
  shore first). The water cost search stays for everything that is
  allowed to swim.
- hunt: startWalkLeg (the shared planner of the town trips, the
  deleveling, the zone returns, the shore re-plans) navigates with the
  dry search; a destination the dry geodata cannot reach reports a
  planning failure - the callers abort the leg and arm their
  cooldowns. The old not-found fallback (a single direct walk the
  server routes itself) is gone: it planned the swim by definition,
  the click guard refused it leg by leg, and the direct walk itself
  runs the character into the lake the guard exists to keep it out of.
- hunt: abortTownTrip during the delevel phase delegates to
  abortDelevel - the cooldown arms (delevelEnd) and the walk home
  starts. Any future walk blocker breaks the restart cycle the same
  way.

### Status: done (2026-09-10)

- Commit "hunt: the shore walks plan dry paths and the water abort
  breaks the deleveling": the dry approach search of the pathfind
  engine (FindPathApproachDry), the shared shore leg planner
  (startWalkLeg) navigates with it, the not-found server-routing
  fallback is gone, and abortTownTrip during the delevel phase
  delegates to abortDelevel (the cooldown arms, the walk home
  starts). Tests: pathfind dry_search_test.go (the channel detour,
  the swim-only strait refusal, the real pack shore route) and hunt
  water_loop_test.go (the delevel walk abort ends the deleveling into
  the cooldown, no refused click reaches the server before the trust,
  the dry miss aborts the deleveling at the planning tick, the town
  trip variant arms the trip cooldown; the two old fallback tests
  rewritten to pin the honest abort). Docs: hunting.md water safety
  and path layer selection paragraphs, development_log.md Round 48.
- Rebase onto the concurrent village water raster round (407c8f2):
  the two rounds compose - the dry planner removes the deliberate
  swims from the plans, the trusted-leg release of that round answers
  the geodata raster artifacts of a planned route (the village plaza
  cells), and the abort of a delevel walk lands in abortDelevel with
  its cooldown. The hunt suite re-verified green on the rebased
  tree.
- Verify loop: go build, go vet, the full go test suite (18 packages
  green), -race green on the hunt package, gofumpt clean,
  golangci-lint zero new findings in the touched files (the 19
  pre-existing branch findings of the spot/zones code stay
  untouched).
- Live validation on the local stack: mobius_e2e.sh 45 and
  BOT_FLAGS=-hunt 90 both E2E_OK - the bot equips, anchors spots,
  pathfinds through the dry planner, engages and loots with no water
  incidents. The real pack probe: the ordinary search plans exactly
  the dump route (wp1 40328 44408 -3800, the swim), the dry search
  plans an all-dry bypass around the bay (15831 units against 11837)
  - the shore detour the "re-pathing around the shore" always
  claimed to do.
- The user-side check stays the project workflow: run a deleveling
  bot from the elven village shore and watch it walk around the bay
  (the planned waypoints all above z -3780) instead of cycling
  "deleveling to 9" / "the walk would cross water"; any trip that
  still cannot find a dry route aborts once with a cooldown instead
  of spinning.

## Active task: the npc talk target, the spellbook junk, the teacher walk and the aggro answer

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user report (2026-09-10, Russian, four bugs):

1. The bot must clear its target after talking to any npc: the
   merchant/teacher selection the trips leave behind is re-adopted by
   the engage (the server-side MyTargetSelected of the villager
   survives the trip end), the bot then spends the 12 s engage stuck
   timeout attacking the friendly npc before skipping it.
2. The sell flow must exclude the items that are needed for anything:
   the spellbooks the learning trips buy for the queued lessons are
   plain junk for the junk ranking (unequipped, priced, weighted), so
   the next trip sells the bought book for referencePrice/2, the
   lesson then waits for its book forever and the trip after that
   buys it back - a buy/sell loop that never learns.
3. None of the bots ever reached the teacher npc to learn a skill
   (some own the spellbook already): the learning stop of the town
   trip must be debugged live to the root cause and fixed, so the
   queued lessons actually land.
4. The bot must track the aggro on itself and never run to aggro a
   NEW mob while chased: either engage the attacker immediately and
   try to win it, or switch to the defense mode and log out through
   the standard escape walk (fleeFromThreat -> the flee budget ->
   emergencyLogout).

### Acceptance criteria

- A conversation with a villager (the merchant select, the teacher
  click) ends with the selection cleared: the self click (Action 0x04
  on the own object id, the official client way) replaces it, and the
  engage never adopts a non-attackable selection even if one lingers.
- The spellbooks of the unlocked queued lessons never enter the sell
  batches and never enter the destroy cleanup.
- The live stack shows a bot learning a lesson at the teacher (the
  SkillList bump and the "learned <skill> level N" log line).
- The engage picks the attacker that holds the character as its
  target over any fresh mob while the character is healthy; a hurt
  character or an unbeatable attacker keeps the defensive flow (the
  escape walk, the logout). A town trip interrupted by an attacker
  drops its walk and answers the same way.

### Status: in progress (2026-09-10)

- The environment deployed (STACK_READY, ports 2106/7777/3306, 75
  tables), the hunt/town/learning/state code surveyed, the Mobius
  C1 sources of Action (0x04), PlayerClick, Player.setTarget,
  AttackRequest and RequestAcquireSkill verified for the selection
  semantics: the server never clears a selection (only the next
  selection replaces it), the self click runs through the
  PlayerClick handler and selects the character itself - the
  official client way of dropping an npc selection.

- Commit "the npc talk selection clears when the conversation ends":
  (1) state grows ObjectAttackable and ObjectLevel reads (the
  friendly villagers, the own id and the unknown objects are never
  attackable; the level comes from the hot record the NpcInfo
  resolved through the generated dictionary). (2) GameClient grows
  ClearTarget: the self click (Action 0x04 on the own object id)
  replaces the server side selection, a no-op without a selection.
  (3) The hunt GameAPI carries it; advanceTripStop and endTownTrip
  call it through clearTalkedTarget, so the talked merchant/teacher
  selection never survives the stop/trip. (4) The engage adoption
  requires an attackable npc: a lingering friendly selection (a talk
  outside the trip end) is skipped instead of burning the 12 s stuck
  timeout on refused forced attacks. Tests:
  state/tracking_test.go (the villager/mob/corpse/unknown split of
  ObjectAttackable, the ObjectLevel read), hunt/loop_test.go (the
  engage never attacks the talked Ellenia selection, the fresh mob
  pick replaces it), hunt/town_test.go (the sell stop end fires the
  clear while Herbiel stays selected).

- Commit "the spellbooks of the queued lessons never sell": the sell
  and destroy junk flows of the state tracker keep the spellbooks the
  unlocked queued lessons demand (demandedBooksLocked walks the
  stored learning queue, keeps the books of lessons with ReqLevel <=
  the character level, cached per skills revision and level): a book
  bought at the book stop or looted for a near term lesson no longer
  re-enters the junk ranking - the reported loop bought the book,
  sold it for referencePrice/2 at the next trip, re-bought it full
  priced forever while the lesson it feeds waited. The planned
  equips keep set stays unchanged (the object id map), the book keep
  keys the ITEM id inside the state layer so both the sell batches
  and the overflow destroy get it for free. Tests:
  state/inventory_test.go (level 5: the books of locked lessons sell
  as before; level 15: the Attack Aura and Defence Aura books stay
  out of the sell list AND the destroy batch, only the stems sell).

- Commit "the aggro on the character is answered, never walked
  past": (1) The targetless pick of the engage answers an attacker
  that holds the character as its target (NearestAttacker - the
  swings or the chase both carry the character as the mob's target
  id): a healthy character with a winnable attacker (the level
  ceiling of the engage covers it, see attackerEngageable) fights it
  at once - the forced attack request fires on the same tick; a hurt
  character or an unwinnable one keeps the defensive flow (the
  standard escape walk of fleeFromThreat with its flee budget and
  the emergency logout behind it). (2) The town trips interrupt the
  same way (interruptTripForAttacker runs first in tickTownTrip): a
  mob on the walking seller drops the trip through the SOFT reset
  (resetTownTrip - no cooldown, the junk/books/sold state survives)
  and answers with the fight or the defense instead of dragging the
  chase through every camp on the route - the reported pile up death
  of the walkers. (3) adoptOutZoneFight checks the winnability of
  the attacker it finishes outside the zone: an unbeatable chase
  switches to the escape instead of pressing a losing fight.
  Tests: hunt/loop_test.go (the attacker beats the nearer fresh mob
  of the pick; the level 8 attacker of a level 3 character arms the
  escape instead of a fight), hunt/town_test.go (the trip drops
  without a cooldown and fights the attacker; the unwinnable
  attacker gets the escape walk).

- Root causes of the teacher walks (reproduced live on the local
  stack, a level 15 elven fighter with 2000 SP and the Attack/Defence
  Aura lessons queued injected through the database): (1) the first
  town trip of a session races the enter world packet burst - a trip
  that starts between the ItemList and the SkillList plans without
  the learning stops (the observed sessions shopped on their first
  walk and never carried the teach stop); (2) the teacher leg dies on
  the water guard: the deployed geodata pack models holes under the
  village plaza (cells without a floor layer resolve to the lake
  layer below them - the probe: every straight line Creamees ->
  Cobendell/Ellenia fails the dry raster while both endpoints stand
  dry at -2984/-2792), every re-path reproduces the same wet line and
  the third exhausts the budget into "town trip ended: aborted, the
  walk would cross water" - the books were bought (the book stop
  comes first), the teacher stop never ran.

- Commit "the teacher legs survive the skill list race and the
  village water raster": (1) maybeStartTownTrip holds its start until
  the server skill list arrived (state.SkillsListed, bounded by
  skillListWaitLimit 10s through state.StartedAt so a server that
  never lists skills keeps the trips selling). (2) The water guard
  releases a plan the geodata itself routes through water: past the
  re-path budget the leg is trusted (wetPlanTrusted, the clicks skip
  the dry check for the rest of the leg and the server routing
  carries the walk over the real plaza) while the standing water
  check and the shore escape stay armed - a genuine swim counts
  (waterEscapes per trip) and the SECOND exhausted budget aborts as
  before, so the open water case cannot loop the trust. Tests:
  hunt/learning_test.go (the trip waits for the skill list, then
  carries the learning stops), hunt/water_guard_test.go (the budget
  exhaustion trusts the plan and the clicks go out; the trusted plan
  that actually swims re-arms the guard and the next exhaustion
  aborts).

- Refinement of the water guard half (the trust gamble replaced by
  the real root cause): the live re-run with the trust fallback let
  the character swim west toward a far hunting spot across a REAL
  lake (the spot walks have no dry route at all - the geodata A* is
  water blind). The measurement that split the cases: sampling
  OverWater along the lines - the plaza teacher lines cross ZERO wet
  cells, the lake lines 736-2640 units. The plaza failure was never
  water: the line of sight half of DryLine fails on the height step
  between the village decks (-2984 shop deck -> -2792 teacher plaza,
  192 units against the passable 30) and the guard misread it. The
  guard now reads pathfind.WaterCrossed (the pure water raster, no
  sight gate): the teacher legs walk (the server routing handles the
  ramps), the real lake refusals keep the old abort. The wetPlanTrusted
  trust machinery was reverted entirely. The session gate also
  re-arms on every reconnect now (state.sessionAt - ResetSession
  drops the skill list and the relogin burst re-delivers it a second
  later; the reconnect loops of the emergency logout made every
  relogin trip a shopping trip, the gate keyed on the tracker uptime
  never held). Engine test:
  pathfind/water_escape_test.go (WaterCrossed splits the channel
  from the tall dry step), hunt tests: the reconnected session
  re-arms the wait, the abort-on-budget stays for real water.

- Live verification (the local stack, a level 15 elven fighter with
  2000 SP and the queued book lessons injected through the database):
  the full cycle ran - the trip planned the learning stops (2
  spellbooks at Creamees, the teacher Ellenia), the books were
  bought ("Spellbook: Advanced Attack Power/Advanced Defense
  Power"), the character walked to Ellenia (the plaza leg survived
  the guard), the teacher was found and clicked, and both lessons
  landed: "learned Attack Aura level 1 for 920 sp", "learned Defense
  Aura level 1 for 160 sp" - the database holds skills 77 and 91 at
  level 1, the SP dropped to 938 and the books were consumed by the
  server. The E2E shutdown stayed graceful (exit 0 on SIGINT).

- Commit "the npc talks, the books and the aggro own their rules in
  the docs": docs/hunting.md updated - the combat safety section
  carries the aggro answer (the attacker pick, the trip interrupt,
  the winnability gate of adoptOutZoneFight), the town trip section
  carries the npc talk selection clearing, the spellbook keep and
  the skill list gate, and the water safety section carries the
  WaterCrossed raster story.
