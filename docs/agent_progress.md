# Agent progress log

Crash-safe task tracking: the current task, its full context and per-commit
progress live here (see the "Work protocol" section in AGENTS.md). Entries
are append-only; a new agent resumes the newest unfinished entry.

## Active task: gear auto-equip, shop buying strategy, multi-zone hunting

Started: 2026-09-08. Branch: `mobius-c1-client-1`.

### Goal

Three features for the swarm bot (elven fighter start, elven village area,
designed to generalize later):

1. **Gear auto-equip.** If the inventory contains an item strictly better
   than the equipped one for a paperdoll slot, the bot swaps it in. If an
   item fits a paperdoll slot that is currently empty, the bot equips it.
   The invariant is maintained continuously while the bot works (after
   inventory-changing events: loot, buy, craft, sweep, downgrade after
   death penalty if items are lost).
2. **Shop buying strategy.** A documented, implementable strategy for
   buying weapons and armor from the village weapon/armor merchants so the
   character can keep leveling efficiently and move to stronger mobs that
   match its level (spend adena at the right time on the right slots).
3. **Multi-zone hunting.** Additional hunting zones for higher levels
   (mobs ~5-7, ~10+, gated by gear/level readiness), all visible on the
   web map with the currently active zone highlighted; the hunt loop picks
   zones according to level and gear. Zones must be data-driven and
   region-extensible (orcs, dwarves, dark elves, humans) and the design
   must not preclude a mage variant later.

### Constraints

- Server integrity rules (AGENTS.md): never patch the server; the bot
  adapts. Only logging patches allowed.
- Mobius C1 protocol quirks go into AGENTS.md protocol notes, not into
  server fixes.
- Keep code in `internal/`; `tools/` is the only supported way to run the
  stack. Tests and linters must pass (`task check:all` equivalent:
  `go vet`, `go test ./...`, golangci-lint if available).
- Frequent atomic commits, each pushed immediately, progress appended here
  per commit.

### Acceptance criteria

- Bot with empty slots auto-equips fitting inventory items; bot with
  better inventory items swaps them in; state stays consistent after
  buy/loot/death events (unit tested; live-checked via logs and web UI).
- Buying strategy documented in `docs/` and implemented as bot logic that
  visits the merchant and buys per the strategy (live verified: adena
  decreases, items appear in inventory and get equipped).
- Hunt zones: at least 3 zones for the elven lands (starter 1-4, mid 5-7,
  10+ when gear allows), selectable, visible on the map with the active
  one highlighted; hunt loop moves between them by level/gear rules
  (live verified: bot hunts in the zone matching its state).
- All designed with an eye to generalization: zone registry is
  data-driven; no hardcoded elf-only assumptions in the zone selection
  core.

### Progress

- 2026-09-08 03a398b: task started; work protocol (atomic commits +
  progress file) added to AGENTS.md and pushed. Environment already
  deployed and verified (login :2106, game :7777, db :3306, E2E_OK from
  the earlier session).

### Next

- Study the codebase: `internal/swarm/state` (inventory/paperdoll
  tracking), `internal/swarm/hunt`, `internal/swarm/packets` (equip and
  shop packets, what C1 sends), `internal/swarm/webserver` (map), then
  design the gear scoring model.

- 2026-09-08 01e4c8d: item gear stats layer done - generate_item_stats.sh now also emits bodypart, weapon type and combat stats (pAtk/mAtk/pDef/mDef/sDef/rShld/pAtkSpd) for all 1204 equippable items; npcdata exposes GearStats.
- 2026-09-08 c9b9d45 + 2412508: the UserInfo parser now reads the 15 paperdoll object ids (the only source that tells which ear/finger slot an equipped jewel occupies) and state.Bot tracks them (ApplyPaperdoll/PaperdollSlotObjectIDs/InventoryItems).
- 2026-09-08 gear package + hunt equip manager: melee fighter scoring profile (weapon = pAtk x attack speed, armor = pDef, jewel = mDef, shield = expected block value), NextUpgrade planner (empty slot fill, strict slot swap, pair swap through freeing the worse jewel first - the server replaces the LEFT slot blindly, two hand weapon and shield interplay guards, one-piece chest+legs family guards), TotalGearPoints zone gate metric. The hunt loop runs maybeEquipGear every tick, sharing the confirmation gate with manual useItem commands (markInventoryAction) so the two never race. Unit tests cover every planner branch. Feature 1 (auto-equip) implemented; live verification pending the e2e run.
