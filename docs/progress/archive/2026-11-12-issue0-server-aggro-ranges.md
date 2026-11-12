# The server aggro ranges and the map circles (status: in progress)

Started 2026-11-12, branch main, issue #0.

## Goal

The map aggro circles and the bot's `AggroRange` data must carry the
exact on-sight trigger radius the Mobius C1 server acts on, not a raw
xml approximation, and the circles of the web UI must be clearly
visible.

Acceptance criteria:

- The generated `npcdata` tables hold the effective template values:
  the xml `aggroRange` clamped at `MaxAggroRange` (450) for the
  `Monster` type only, the raw value for every other type.
- The on-sight aggression flag matches `AttackableAI`: only the
  monster class ai types attack a neutral player on sight; guards and
  friendly mobs answer only player killers, folk never attack.
- The map draws a circle only around a living attackable on-sight
  aggressor, at the effective range, and the circle is readable at
  the normal zoom (stronger alpha and stroke than the faint dashed
  one).
- `go build ./...`, the affected package tests, `task lint:new`,
  `task fmt:check` and the web UI harnesses pass.

## Context

Verified against the official module
`E:/work/lineage_workspace_fresh/L2J_Mobius/L2J_Mobius_C1_HarbingersOfWar`
(sources and `dist/game/data`, both identical) and the live stack
(login 2106, game 7777, MariaDB 3306):

- `dist/game/config/NPC.ini`: `MaxAggroRange = 450` (the L2jMobius
  override of the L2J default 1500).
- `NpcTemplate.java` clamps `aggroRange` at `MaxAggroRange` only when
  `type.equalsIgnoreCase("Monster")`; the default type is `Folk`.
  `isAggressive` defaults true.
- `AttackableAI.isAggressiveTowards` checks
  `isInsideRadius3D(target, getAggroRange())` (strict, center to
  center, Z included) plus the `canSeeTarget` line of sight. Its Guard
  branch returns true only for a player with karma > 0 (or an
  aggressive monster); the FriendlyMob branch only for a player with
  karma > 0.
- `Monster.isAggressive()` is the template `isAggressive` flag and
  not the `PASSIVE` effect; `NpcStatMultipliers` are disabled.
- Top-level scan of the stats xml: 326 on-sight display templates,
  all `Monster` type at the effective 450. The four `GrandBoss`
  templates are all `isAggressive=false`. The 114 `Guard` and the 5
  `FriendlyMob` templates carry `isAggressive=true` but their AI
  branch never attacks a neutral player.
- The old generator regex also matched the nested `<npc>` tags inside
  `<minions>`, truncated the parent block before `<ai>` and produced
  37 raw range mismatches and 89 aggression mismatches (for example
  Dre Vanul display 117 read `0` / false instead of `1000` / true);
  the old `NPCAggroRange` clamped every type, and the UI drew a
  circle for every aggressive-flagged town npc (233 folk, 110
  guards).

## Progress

### 2026-11-12 - npcdata: derive the effective aggro values

Rewrote the generator parser to `xml.etree.ElementTree` (top level
`<npc>` only, no minion shadowing), read `MaxAggroRange` from the
deployed `NPC.ini`, and emit the effective range and on-sight flag
(type aware). Regenerated `names.go`: 1837 display templates, 326
on-sight aggressors (all effective 450), zero mismatches against an
independent xml scan. Updated `npcdata.go`, its tests, the state
comments and the hunting doc. Commit `6f38f199`.

### 2026-11-12 - web: draw the server on-sight circles

The map circle layer now requires an attackable aggressive npc, draws
a 2 px `[6, 4]` dashed stroke at 0.85 alpha plus a 0.1 alpha zone
fill, and the tooltip shows the radius only for attackable npcs. The
render harness asserts the strong stroke, the fill and the absence of
a circle at a non-attackable town npc. `docs/webui.md` carries the
effective data model, the devlog fragment records the root cause.

Verification: `go test ./internal/swarm/npcdata/
./internal/swarm/state/ ./internal/swarm/webserver/
./internal/swarm/hunt/`, the three map harnesses, `task lint:new`,
`task fmt:check`.

## Status

Complete: both commits landed, the fragment moves to the archive.
