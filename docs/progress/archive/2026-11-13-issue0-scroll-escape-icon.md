# The Scroll of Escape cast art and its town teleport scenario (status: complete)

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
  `getNearestTownRespawn`. That method first asks the `RespawnZone`
  the character stands in (`dist/game/data/zones/respawn.xml`, one
  zone per starting ground, keyed by race) and otherwise the map
  region of its cell -> `getSpawnLoc()`. The map cell formula is
  `(x>>15)+20` / `(y>>15)+18`; cell 21_19 belongs to `elf_town` (the
  `Mothertree_Glade` region carries no `<map>` entry), and
  `RandomRespawnInTownEnabled` picks one of its 12 non-chaotic
  respawn points at random.
- Live-verified 2026-11-13: the Mothertree Glade is a `RespawnZone`
  (`Mothertree_Glade_21_19`, polygon 44096 40595 - 47903 41035 -
  46459 42066 - 45948 42089) that maps every race to the
  `Mothertree_Glade` region, so a scroll used there lands on the
  Mothertree respawn points a few hundred units away, not in the
  village. The scenario start moved off it to the Keltir glade.
- The start ground is the Keltir glade (45496 44544 -3704, the
  `oren02_2119_08` territory of the elven starting grounds, map cell
  21_19): the geodata cell is dry and unblocked, every Keltir of the
  ground (20530/20534/20537) carries `isAggressive="false"` in the
  datapack, and no `RespawnZone` covers it - so the scroll lands on
  the town ring and the arrival check (`displacement > 500`) holds.
  `NpcTemplate.java:196` defaults a missing `isAggressive` attribute
  to true, which is why the Spore Fungus ground (the gear gap dump
  cell, the Kaboo Orc Fighter 20471 without the attribute) was
  dropped as a start. The live log confirms the protection drop
  ("You are no longer protected from aggressive monsters") happens at
  level 15, so a passive ground matters.
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

### 2026-11-13 - the first live run and the landing root cause

Ran `scroll-escape` against the Windows stack (`-login
127.0.0.3:2106`, temp26, 42 s). The icon leg passed on the live
server: the server broadcast `MagicSkillUse` 0x5A with skill 2013 and
hit time 20000, the snapshot cast state carried `skill0000`, the item
left the bag 250 ms after the use and the cast `TeleportToLocation`
0x38 arrived 20 s later - `cast`, `icon` and `consumed` latched. The
landing leg failed: the scroll teleported the character to
(46144,41185), 130 units from the start, because the Mothertree Glade
is inside the `Mothertree_Glade_21_19` respawn zone. The start moved
to the Keltir glade (45496 44544 -3704), whose cell owns no respawn
zone and whose mobs are all passive; the rule went into
`docs/hunting.md` and the `hunt/soe.go` package comment.

### 2026-11-13 - the live proof of the scroll leg

The rerun from the Keltir glade passed the whole story in 40 s
(`logs/acceptance/scroll-escape-20261113-143000-12748-1.log`): the
world entry, the `cast` window (skill 2013, 20000 ms), the `icon`
(`skill0000`), the `consumed` line and the landing - the server
`TeleportToLocation` 0x38 put the character at (45829,49885,-2945),
one of the twelve `elf_town` respawn points, 5 019 units from the
start; the checks read "standing at 45829,49885" and the bot left the
world gracefully (`Acceptance: PASS`). The icon fix is therefore
verified end to end: the live cast state carried the vortex art and
the map harness draws it.

## Status

Complete: the icon fix is live-verified, `scroll-escape` passes on the
deployed C1 stack, the landing rule is documented and the fragment
moves to the archive. Follow-up candidate (not started): the arrival
check of `escapeWithSOE` reads a scroll used inside a starting ground
respawn zone as "never landed" although the server did teleport - the
snapshot teleport of the played object would be the honest signal.
