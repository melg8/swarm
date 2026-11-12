# The npc aggro data and the map circles

## Scope

2026-11-12, issue #0, branch `main`. The effective Mobius C1 aggro
ranges of the generated `internal/swarm/npcdata` tables, the on-sight
aggression flag of `AttackableAI`, and the aggro circle layer of the
web UI map.

## Problem

The map drew a faint aggro circle around every npc the generated data
flagged `aggressive`: 656 display templates, including 233 folk, 110
guards and a merchant/trainer/friendly mob. At the same time 16 named
monsters with minions (Dre Vanul, Varikan Brigand Leader, Vrykolakas,
Pirate Captain Uthanka, White Fang, Kobold Looter Bepook, Wild
Desperado, Roxide, Death Fire, Dark Lord, Talakin, Dragon Bearer
Chief, Pytan, Lord of Plain, Barif, Conjurer Bat Lord) had no circle
at all, and the tooltip of every monster read the same 450 radius.

## Root cause

1. The embedded Python of `tools/generate_npc_names.sh` matched every
   `<npc ...>` textually, including the nested `<npc id="..." count="...">`
   references of the `<minions>` lists. A nested tag cut the parent
   block before its `<ai>` element, so the parent template fell back
   to `aggroRange=0` and `isAggressive=false` (89 aggression and 37
   raw range mismatches). The same truncation dropped the `<clanList>`
   of those parents.
2. `NPCAggroRange` clamped every type at the hardcoded
   `mobiusMaxAggroRange = 450`, but `NpcTemplate.java` applies the
   `MaxAggroRange` clamp only when `type.equalsIgnoreCase("Monster")`
   (the default type is `Folk`), so guards (1000) and grand bosses
   (1000/2000) keep their raw xml value.
3. `NPCIsAggressive` copied the raw xml `isAggressive` flag, while
   `AttackableAI.isAggressiveTowards` attacks a neutral player on
   sight only in its generic branch (`me.isAggressive()`). The Guard
   branch requires a player with karma (or an aggressive monster),
   the FriendlyMob branch requires karma, and folk never attack, so
   their xml flag is not an on-sight threat.

## Reproduction

`tools/generate_npc_names.sh <C1 module>` against the official module
before the fix, then compare the generated maps with the stats xml:

- display 117 (Dre Vanul, Monster, xml `aggroRange=1000`,
  `isAggressive=true`) reads `aggroRange=0`, `aggressive=false`;
- display 1 (Gremlin, passive Monster) reads 1000 instead of the
  effective 450;
- display 7039 (Gilbert, Guard) reads aggressive and draws a town
  circle;
- the map footer reads hundreds of aggressive town npcs.

## Fix

- The generator parses the top level `<npc>` elements with
  `xml.etree.ElementTree` only, reads `MaxAggroRange` from the
  deployed `NPC.ini` (default 450), emits the effective range (clamp
  for `Monster` only) and the effective on-sight flag (a monster
  class type `Monster`/`RaidBoss`/`GrandBoss` with `isAggressive`
  true and a positive effective range).
- `NPCAggroRange` returns the generated effective value;
  `NPCIsAggressive` returns the effective on-sight flag; the stale
  hardcoded clamp is gone.
- The map draws a circle only for a living **attackable** aggressive
  npc, at the effective range, with a 2 px stroke at 0.85 alpha, a
  `[6, 4]` dash and a faint 0.1 alpha zone fill. The tooltip shows
  the radius only for attackable npcs and calls a passive monster
  passive, not defensive.

## Verification

- An independent ElementTree scan of all 39 stats xml files against
  the generated `names.go`: 0 range mismatches, 0 aggression
  mismatches, 326 on-sight templates (all effective 450).
- `go test ./internal/swarm/npcdata/ ./internal/swarm/state/
  ./internal/swarm/webserver/ ./internal/swarm/hunt/` passes.
- `node tools/repro_map_render.js`, `node tools/repro_zone_hover.js`
  and `node tools/repro_bot_switch.js` pass; the render harness now
  asserts the 2 px stroke, the zone fill and the absence of a circle
  at a non-attackable town npc.
- `task lint:new` reports 0 issues, `task fmt:check` passes.

## Follow ups

None. The runtime `game/data` and the source `dist/game/data` of the
module are byte identical, so the generated tables match both.
