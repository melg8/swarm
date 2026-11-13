# The Scroll of Escape cast art and its town teleport scenario (status: in progress)

Started 2026-11-13, branch main, issue0 (owner report: during the
Scroll of Escape cast the web UI map draws no cast icon at all; the
icon must be the blue-white vortex art the client shows for the
scroll, like the unstuck one. Plus: the town teleport by scroll needs
its own acceptance scenario).

## Goal

Two delivrables on one scroll story:

- the web UI map cast plate draws the real art of the running cast for
  any skill, including an item skill of the bag that is never a
  learned skill - the Scroll of Escape draws `skill0000`, the
  blue-white vortex of the client;
- a new acceptance scenario `scroll-escape` proves the whole scroll
  leg of the town trip on the live stack: the level 15 fighter with
  one scroll in the bag casts it, the server consumes the item, the
  teleport lands on the elven town respawn ring, and the snapshot cast
  state carried the vortex icon during the cast.

Constraints: the bot log and the code stay English, the scenario runs
inside the existing acceptance manager (one temp account, a bounded
timeout, a graceful shutdown), the icon comes from the real client
art of the pack (`data/icons/skill0000.png`), never from a drawn
placeholder.

## Context

- Skill 2013 (Scroll of Escape) of the C1 datapack
  (`dist/game/data/stats/skills/02000-02099.xml`): operateType A1,
  target SELF, `hitTime` 20000 ms, consumes item 736, effect Escape
  with `escapeType="TOWN"`.
- The decrypted client `skillgrp.txt` names `icon.skill0000` for
  skill 2013; `data/icons/skill0000.png` is that blue-white vortex
  and ships with the icon pack.
- The generated `npcdata.skill_trees.go` carries `Icon: ""` for the
  155 item skills (the generator is stale against the current Mobius
  XML style); regeneration is out of scope, so `npcdata.SkillInfoOf`
  answers the client default art for an empty generated icon.
- The server broadcasts `MagicSkillUse` (0x5A) back to the caster
  (`Player.broadcastPacket`), so the cast window of the scroll reaches
  the tracker's `skillStates` like a learned skill's cast.
- The map resolves the cast icon from the learned skill list of the
  snapshot, which never carries the scroll - the reason the plate
  stayed empty.
- The teleport landing: `Escape.onStart` calls
  `MapRegionManager.getTeleToLocation(TOWN)`, which falls through to
  `getNearestTownRespawn` -> the map region of the creature ->
  `getSpawnLoc()`. The map cell formula is `(x>>15)+20` /
  `(y>>15)+18`; cell 21_19 belongs to `elf_town` (the
  `Mothertree_Glade` region carries no `<map>` entry), and
  `RandomRespawnInTownEnabled` picks one of its 12 non-chaotic
  respawn points at random.
- The start ground is the elven creation spawn of the Mothertree
  Glade (46045 41251 -3440, the first `Mothertree_Glade` respawn
  point). `NpcTemplate.java:196` defaults a missing `isAggressive`
  attribute to true, so the Spore Fungus ground used by the gear gap
  dump was dropped as a start: the Kaboo Orc Fighter 20471 spawns
  there without the attribute. The Keltirs (20530/20534/20537) and
  the Gremlins (20001) around the creation spawn all carry
  `isAggressive="false"`; the geodata cell is dry and unblocked.
- `hunt.maybeEscapeTo` gates on a trusted income metric
  (`cellMetricsTrustMin`, five minutes of samples), which a short
  scenario never earns - hence the exported seam `Loop.EscapeWithSOE`
  that issues the same `escapeWithSOE` the town trip uses.
- The scenario runs with `loop.SetAutonomy(false)` through the
  session hook, so the scroll is the only action of the run.
- The landing check is the ring predicate (center 45355 49561,
  radius 950, covering the 12 points plus the 50 unit teleport
  offset), not a fixed point, because the server randomises the
  landing.

## Progress

### 2026-11-13 - the cast icon of an item skill

`npcdata.SkillInfoOf` answers `skill0000` for a skill whose generated
entry carries no dedicated art; `state.SkillStateView` gained `Icon`
(the tracker resolves it from `npcdata`), the snapshot JSON emits it
and the size estimate moved to 160 bytes per state. `map.js`
(`ingestSkillStates`, `selfCastIconName`) prefers the state icon and
keeps the learned list as the fallback of an older server.
`tools/repro_map_render.js` covers the scroll scene: the state icon
resolves without any learned skill, the vortex lands in the icon
cache and the plate draws it. `docs/webui.md` carries the contract.

### 2026-11-13 - the scroll-escape acceptance scenario

`hunt.SOEItemID`/`hunt.Loop.EscapeWithSOE` expose the scroll seam.
`internal/swarm/acceptance/scroll_escape.go` owns temp26, a five
minute bound and five checks (online, cast, icon, consumed, landed):
the watch reads the cast state, the bag and the position every 250 ms
while the escape goroutine polls the arrival. The scenario owns the
head of the registry list; the account and list-order contracts of
the neighbouring scenarios moved by one slot.

## Status

In progress: the offline loop is green (`go build ./...`, the
acceptance, state and npcdata packages, the map render harness, the
lint gate). Next: the live run of `scroll-escape` on the Windows
stack, its result recorded here and in the issue, then the archive
move.
