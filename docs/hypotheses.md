# The hypotheses registry: the server facts that are not yet verified

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

Every server fact in AGENTS.md and in `docs/` is verified - either
read from the Mobius C1 Java sources or observed on the live stack.
Anything about the server that is NOT yet verified is a hypothesis,
and a hypothesis never lives silently in code comments, in a
conversation or in a commit message: it lives in this registry.

## The rules

- Before code that relies on an unverified server assumption is
  written, the assumption becomes a registry entry (the next free
  H-NNN) carrying the assumption, the verification plan and the
  status; a comment on the relying line references the id.
- The verification plan names its evidence: the Mobius Java classes
  to read and, whenever behavior matters, the live experiment (the
  deployed local stack, `tools/mobius_e2e.sh` or an acceptance
  scenario in `internal/swarm/acceptance`).
- Running the plan closes the entry. A confirmed fact moves into the
  matching subsystem doc (`docs/protocol_description.md`,
  `docs/hunting.md`, ...) and the entry records `verified <date>,
  see <doc>`; a refuted assumption records what the server actually
  did and what changed in the bot as a result.
- Entries append at the end; ids are never reused or renumbered. The
  seed entries came from the open items of
  `docs/navigation_analysis.md` (T-005).
- **Advance or close one hypothesis per session** when the touched
  area matches its verification plan: the session bootstrap
  (`tools/session_start.sh`) lists the open ids, and an entry ages out
  of "open" only by the evidence its plan asks for - not by a session
  deciding it no longer matters.

## The entries

### H-001: the swimming semantics of deep water crossings

- Assumption: a water cell is walkable at the 3x step cost
  (`waterCostMultiplier`) and a character crossing deep water
  survives the breath gauge; the sea routes the search returns
  (Talking Island to Giran on the sea floor) are accepted by the
  server as-is.
- Relied on by: the water cost model of the geodata search
  (`internal/swarm/pathfind`, the -3780 water surface) and the route
  claims of `docs/navigation_analysis.md`.
- Verify: read `Player.checkWaterState`/`startWaterTask` (the 60 s
  breath base scaled by `Stat.BREATH`, gated on `ALLOW_WATER`),
  `WaterTask` (maxHp/100 damage per second once the gauge empties),
  `CreatureTemplate` (`baseSwimRunSpd` defaults to the run speed)
  and the `WaterZone`/`ZoneId.WATER` machinery; then walk a
  DB-injected character from a shore into deep water on the live
  stack and record the `SetupGauge` packet, the breath damage
  message, the observed swim speed and any `ValidateLocation`
  correction.
- Status: open.

### H-002: the gatekeeper teleport graph

- Assumption: the teleporter data (`data/teleporters/town/*.xml`
  and `data/teleporters/others/`, 25 npcs, 352 destinations) parses
  into navigation teleport edges priced by the fee, and a bot drives
  a gatekeeper through the html dialog bypass flow.
- Relied on by: the meta transport plan of
  `docs/navigation_analysis.md` and the M3 navigation segments.
- Verify: read `Teleporter.onBypassFeedback` (the `chat`,
  `show teleports <list>` and `teleport <list> <index>` bypass
  commands), `RequestBypassToServer` (the client packet routing the
  commands), `NpcHtmlMessage`, `TeleporterData`/`TeleportHolder`
  and `TeleportToLocation`; then click a village gatekeeper with a
  live bot, walk the bypass chain, and confirm the fee deduction
  and the arrival coordinates against the xml.
- Status: open.

### H-003: the boats as scheduled transport edges

- Assumption: the three boat routes (BoatTalkingGludin,
  BoatGiranTalking, BoatGludinRune of
  `dist/game/data/scripts/vehicles`) are scheduled edges a bot can
  board through the wharf managers and ride with the vehicle
  packets.
- Relied on by: the meta transport plan of
  `docs/navigation_analysis.md` (the alternative to sea walking).
- Verify: read `Boat.java`, `BoatManager`, the three scripts and
  the vehicle packet family (`RequestGetOnVehicle`,
  `RequestGetOffVehicle`, `MoveToLocationInVehicle` and their
  server answers); then observe the `VehicleInfo` and
  `VehicleDeparture` broadcasts at a wharf at the schedule time and
  ride one segment live, recording the boarding bypass command and the
  oust position.
- Status: open.

### H-004: the doors as passable obstacles

- Assumption: a closed door is a wall to the geodata search while
  the server opens it on demand (click, skill, item, time), so
  interior routes through closed doors are planned as blocked
  although they are passable live.
- Relied on by: the static geodata walls of the search and the
  dynamic world elements section of
  `docs/navigation_analysis.md`.
- Verify: read `Door.java` (`isOpen`, `openMe`, `closeMe`, the
  isOpenableBy* families) with `DoorInfo`/`DoorStatusUpdate`; then
  stand a live bot by a default-closed door of `Doors.xml` (41
  entries), record the initial status broadcast, request the open
  and walk through, noting whether the server blocks the move.
- Status: open.

### H-005: the spawn protection window of a fresh login

- Assumption: every EnterWorld arms the Mobius C1 spawn protection
  (PlayerSpawnProtection, 600 s in the deployed Player.ini): the
  Attackable.getHating gate strips the aggro of a protected player
  every AI tick, so aggressive mobs stand next to a stationary
  fresh login without attacking. The protection clears ONLY on one
  of five action packets - MoveToLocation, AttackRequest, Action,
  UseItem, RequestMagicSkillUse - the sit toggle of
  RequestActionUse is NOT in the list.
- Relied on by: the post relogin settle of the hunt loop
  (internal/swarm/hunt/settle.go): the sit-regeneration under the
  protection and the deliberate first strike that ends it.
- Verify: read Player.setSpawnProtection/onActionRequest,
  EnterWorld (the arming), Attackable.getHating (the aggro strip),
  AttackableAI.isAggressiveTowards (no movement or sitting check in
  the aggro itself) and Player.ini (the window); the owner observed
  the live behavior on the deployed stack (mobs 1 m away ignore the
  stationary fresh character until it moves).
- Status: verified 2026-09-21 by the Mobius source read (the
  mechanism above) plus the owner live observation; see
  docs/hunting.md (the spawn protection section).

### H-006: the deployment that swallows accepted move requests

- Assumption: the owner's live deployment (the 2026-09-21 town walk
  dump, build a483578, bot test3) answers some MoveToLocation
  requests with silence - no movement broadcast, no ActionFailed -
  while the reference Mobius master either moves the character or
  refuses the request. The dump evidence: 49 stuck skips over 3.5
  frozen minutes, the position byte identical (31040 54016 -3415),
  clicks leaving (the dump's walking timer armed), the last
  ActionFailed 55 seconds before the first skip (11:59:40, the walk
  start, never again through the storm), and the unknown packet
  fingerprints (0x57/53, 0xe7/21, 0x5a/53, 0x8e/21, 0x85/13) that
  mark a build the reference tree does not match. The bot must
  treat "the click validated, went out, and moved nothing" as a
  state of its own: the move start watchdog names it, and the
  recovery must not answer it with more of the same transport.
- Relied on by: the frozen skip rule of the town walk follower
  (skipMoveFresh, internal/swarm/hunt/town_stuck.go) - a skip that moved
  the character nothing may not repeat, the re-path ladder and the
  cursor key escape own the dead click transport instead. The kite
  re-click ladder of the archer retreat (kiteReclickWalk,
  internal/swarm/hunt/kite.go, issue #60 second round) rides the
  same hypothesis from the fight side: the retreat click that moved
  nothing gets its paced re-issues and the fan rotation, and the
  probe line of the ladder names the silent drop in the hunt dumps
  exactly the way the town walk skip rule does.
- Verify: reproduce on the deployed stack - place a character on
  the dump cell, click a waypoint the local oracle validates, and
  record the server answers (the game server log with a
  MOVEDBG-style diagnostics line is the allowed evidence form).
  If the reference stack moves the character, the silence is a
  deployment build difference; pin the build the owner runs and
  the packet that differs.
- Status: open (the bot-side recovery shipped in 481dfdc and 8487365; the
  server side stays unobserved).

### H-007: the bow shot commit rides the Attack broadcast

- Assumption: the server finalizes a bow shot the moment it
  broadcasts the character's own Attack packet - the hit roll, the
  arrow consumption and the HitTask schedule all happen before the
  broadcast, and the damage task carries no attacker movement check,
  so a character that starts moving after the packet still lands the
  shot. The bow re-use window is `timeAtk + reuse`
  (500000/pAtkSpd + reuseDelay*333/pAtkSpd, ~3s for the Short Bow),
  and a forced attack request while moving stops the walk and
  attacks.
- Relied on by: the shot-paced kite retreat of the archer fight
  (kiteFromShot, internal/swarm/hunt/kite.go, issue #60) - the
  retreat starts on the broadcast and spends the cooldown walking.
- Verify: read `Creature.doAttack`, `Creature.doAttackHitByBow`
  (the roll, `reduceArrowCount`, the HitTask schedule at `sAtk`, the
  SetupGauge(RED, sAtk + reuse)), `Creature.onHitTimer` (no
  attacker-movement check), `Creature.calculateTimeBetweenAttacks` /
  `calculateReuseTime` / `useWeapon` (the `_disableBowAttackEndTime`
  gate) and `Creature.stopMove` before the attack launch; the shot
  cadence of the live dumps corroborates (the issue #60 dump: one
  "Getting ready to shoot arrows" per ~3.2s kite cycle).
- Status: verified 2026-09-22 by the Mobius C1 source read (the
  mechanisms above) plus the issue #60 dump cadence; see
  docs/hunting.md (the shot-paced retreat section).
