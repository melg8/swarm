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
