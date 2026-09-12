# Development log

Running log of the work on the `mobius-c1-client-1` branch. Newest entries at
the bottom. This file exists so the context does not have to be repeated in
agent prompts: read this file first.

Navigation: the rounds are `## Round N: title (date)` headers, so
`grep -n "^## " docs/development_log.md` lists the whole index; jump
to the round you need. The newest rounds describe the current
subsystem behavior - the older ones the root cause history behind it
(the current behavior summary lives in the docs/ modules referenced
from AGENTS.md).

Entry format:

- date, scope
- problem statement
- root cause analysis (with references into the Mobius server sources)
- reproduction: how to trigger, how to detect, expected output
- fix, verification, follow ups

## 2026-09-05: hunt MVP web interface follow up

Scope: two problems reported against the MVP bot web interface on the
`mobius-c1-client` branch.

### Problem 1: movement display ends with a fast teleport

Statement: characters move along the path on the map, but after roughly 7/8
of the way they are dragged to the destination almost instantly. The first
7/8 looks slightly slowed down; the error is around 10-15 percent of the
real speed.

Root cause analysis (verified against the Mobius C1 java sources,
`L2J_Mobius_C1_HarbingersOfWar/java`):

1. The web map interpolated the full packet distance `D` at the transmitted
   speed `v`, while the Mobius server stops a moving creature early:
   `Creature.updatePosition` treats the creature as arrived when
   `speed * elapsedTicks` covers `distance - collisionRadius` (the collision
   radius of the mover, or of the chase target while attacking) and then
   snaps the server position to the exact destination and broadcasts the
   zero distance `MoveToLocation`. A client that animates the whole `D`
   therefore still has `collisionRadius` units left when the arrival packet
   lands; with the typical C1 monster radii (5-15) and random walk hops
   (50-300 units inside `MaxDriftRange = 300`) that alone is 5-15 percent
   of the path.
2. The render loop smoothed the drawn position toward the projected
   position with a first order filter (`1 - exp(-dt*10)`, time constant
   100 ms). A first order tracker of a ramp input lags by `v * tau`
   permanently: at `v = 120` that is 12 units of permanent visual lag, on
   top of the collision gap.
3. The movement ticks of the server are quantized (100 ms for npcs,
   50 ms for playables, `MovementTaskManager`), the snapshot poll adds up
   to 300 ms and the SSE clock offset estimate has a small negative bias,
   which adds a few more units of trailing error.

Combined, the drawn character is 10-30 units short when the arrival packet
arrives (10-20 percent of a typical hop), and the arrival then snaps the
position: the reported "slightly slowed 7/8 then quick drag".

### Problem 2: `-hunt` selects a target but never attacks

Statement: with `-hunt` the character picks a target (MyTargetSelected
arrives) and then stands still forever.

Root cause analysis (Mobius C1 java sources):

- `AttackRequest.runImpl` (client packet 0x0A) implements the classic
  double click semantics:
  - when the requested target is **not** the current target it calls
    `target.onAction(player)`, which resolves to the `NpcClick` script
    handler: `player.setTarget(target)` plus `MyTargetSelected` — a plain
    selection, no attack;
  - when the requested target **is already** the current target it calls
    `target.onForcedAttack(player)` → `player.getAI().setIntentionAttack`,
    which starts the chase and the auto attack.
- The hunt loop sent exactly one `AttackRequest` per target and then waited
  for the kill: `engage()` copies the confirmed server target into
  `l.target` and returns early while the target lives, so the second
  request that would trigger `onForcedAttack` is never sent.
- The server flood protector (`PlayerActionFloodProtector`,
  `FloodProtectorPlayerActionInterval = 1`) allows one player action per
  second, so a bot that re-requests once per tick is fine.

### Reproduction and verification plan

- `tools/repro_movement.js` — a Node script that loads the real
  `web/map.js` interpolation, drives it with a faithful simulation of the
  Mobius movement/broadcast semantics and reports the arrival position
  error and the end of path jump. Exit code 1 while the bug is present.
- `internal/swarm/hunt/loop_test.go` and
  `internal/swarm/connection/hunt_flow_test.go` — Go tests that implement
  the same server semantics in-process (fake game server with the
  double click behavior) and assert the bot actually starts attacking.
  They fail while the bug is present.

The entries below describe the reproduction tooling, the fixes and how
to verify everything on any checkout.

### Fix 1: movement rendering (commit "fix movement interpolation end of
path teleport")

The web map now models the server arrival schedule instead of the naive
full distance animation:

- `projectObject` scales the interpolation speed by
  `distance / (distance - gap)` where `gap` estimates how early the
  server stops the creature (collision radius plus about one 100 ms
  tick step). The estimate is learned per npc template id (and for
  players/self) with an exponential moving average fed by every
  observed arrival: when the arrival packet lands, the tracker compares
  the traveled projection distance with the segment distance. The gap
  is bounded (1..60 units) and the speed scale is capped at 1.6 so a
  bad estimate can never dash units.
- The drawn position follows the projection plus a decaying offset that
  is only set when the projection jumps discontinuously (a new segment,
  an arrival, a teleport above 400 units snaps instantly). Continuous
  movement has no permanent lag anymore; residual calibration errors
  glide closed within ~100 ms.
- The server clock offset estimate takes the maximum of the last 20
  snapshot samples instead of the latest one, removing the transport
  delay bias.

Verification: `node tools/repro_movement.js` (or `task repro:movement`)
drives the real `web/map.js` inside a Node vm sandbox against the
simulated Mobius movement (see the harness header for the reproduced
server semantics). Before the fix it reported `FAIL` with progress at
arrival 0.33-0.96 and end of path jumps up to 4.9x the normal frame
speed; after the fix every scenario reports `PASS` with progress
0.94-1.00 and jumps below 1.25x. The harness exit code is 1 while the
bug is present, so it is usable as a regression check.

### Fix 2: hunt attack start (commit "fix hunt loop never starting the
attack")

The hunt loop implements the double click flow now:

1. `AttackNearest` sends the first `AttackRequest` for the nearest
   attackable npc and latches its id (the server answers
   `MyTargetSelected`, which the tracker records as the target).
2. While the target lives and the tracker does not report the character
   engaged with it (`state.Bot.SelfEngaged`: the chase MoveToPawn, the
   Attack or the AutoAttackStart broadcasts set the fighting target),
   the loop repeats `AttackTarget` for the same id at most once per
   second (the `PlayerActionFloodProtector` interval).
3. As soon as the fight packets arrive, the requests stop.

Verification: `internal/swarm/connection/hunt_flow_test.go`
(`go test ./internal/swarm/connection -run TestGameClientHuntFlowStartsAttack`)
spins up a fake game server that implements the extracted Mobius
semantics (first request -> MyTargetSelected only, second request for
the already selected target -> AutoAttackStart + Attack). Before the
fix the test fails after 10 seconds with exactly one received request
`[1]` and no combat; after the fix it passes in about two seconds with
requests `[1 1]` and the fight running. Unit level coverage of the
retry and stop conditions lives in `internal/swarm/hunt/loop_test.go`.

### How to reproduce and detect both problems on any checkout

- Movement: `node tools/repro_movement.js [--verbose]` (needs Node 18+;
  no Go or server required). Exit code 1 and `FAIL` lines mean the end
  of path teleport is present.
- Hunt: `go test ./internal/swarm/connection -count=1
  -run TestGameClientHuntFlowStartsAttack`. A failing test with a
  single recorded attack request means the bot never starts attacking.
- Full suite: `task check:all` (lint + tests) plus
  `task repro:movement`.

Status: both problems fixed and verified; the branch
`mobius-c1-client-1` carries the analysis, the reproduction tooling and
the fixes.

### Regression detector validation (2026-09-05)

Re-checked that the harness really detects the bug: running
`node tools/repro_movement.js` against the pre fix `web/map.js`
(checkout of the branch point `34f6884`) reports FAIL with progress at
arrival 0.897-0.940 and jumps up to 2.23x; running it against the fixed
file reports PASS on all scenarios. Restoring the fixed file afterwards
keeps the branch clean, and the full suite (`go test ./...`, golangci
lint on new code, the movement harness) is green.

## Round 2: five hunt and web interface problems (2026-09-05)

User report after testing the bot with a second account in the world:

1. selecting the bot or a mob with the second character showed nothing
   on the map;
2. the bot idles for many seconds between mobs instead of chaining the
   next kill when its HP is high;
3. the unit direction line is drawn inside the circle marker;
4. the bot often picks a target that is not the nearest one right now;
5. HUD issues: a killed target shows `object <id>`, the `target` label
   visually merges with the value, the `facing` field duplicates the
   map, the experience is a bare number instead of a progress bar and
   the HP/MP bar colors are not the classic L2 C1 ones.

### Root causes

- Selections of other players: the tracker already records them
  (TargetSelected 0x39 -> `ApplyObjectTarget`, TargetUnselected 0x3A ->
  `ApplyTargetClear`, verified against `Player.setTarget` of the Mobius
  sources which broadcasts `TargetSelected(getObjectId(),
  newTarget.getObjectId(), ...)` to every visible player), and the
  snapshot carries `targetId` - but `web/map.js` only rendered the
  selection of the bot itself. Pure rendering gap.
- Hunt idle between mobs: the Mobius server never clears the target
  selection of a killed target (`Player.setTarget` answers only new
  selections with MyTargetSelected; the removal path broadcasts
  TargetUnselected to everyone including the actor, but nothing tells
  the acting client "you have no target now"). The bot tracker kept the
  dead object id in `char.TargetID`, the hunt loop re-adopted that
  stale id every tick ("prefer the server view") and flipped into the
  loot phase, which immediately flipped back to engage with the target
  reset: an engage/loot ping-pong in which `AttackNearest` was never
  called again. Additionally the fixed 4 second `engagePeriod` pause
  and the 1 second decision cadence added idle time after every loot
  phase even without the ping-pong.
- Direction line: `drawUnitTick` drew the heading ray from 20 percent
  of the radius inside the circle.
- Nearest target: `NearestAttackable` ranked candidates by their last
  movement packet start position. The server re-broadcasts
  MoveToLocation at most once per second per moving creature, so a
  moving mob is tens or hundreds of units away from its recorded start
  when the choice is made.
- HUD: the tracker never cleared `char.TargetID` (same server
  semantics as above), so a removed corpse left a dangling id that the
  HUD rendered as `object <id>`; the HUD had no experience bar because
  the bot never computed the level progress (the C1 experience table
  lives in the server data pack); the `.kv` grid used
  `justify-content: space-between` without a gap or overflow handling,
  so the `target` label and long values merged.

### Fixes

- Tracker (`internal/swarm/state`): the character target is cleared
  when the target dies (StatusUpdate CUR_HP 0), when the target object
  is removed (DeleteObject) and on the own TargetUnselected broadcast
  (`ApplyTargetClear(selfID)`), mirroring what the official client
  shows. `SelfHealthPercent` exposes the HP level, `ExpPercent` (C1
  experience table generated from the Mobius data pack by
  `tools/generate_experience_table.sh` into
  `internal/swarm/state/experience.go`) feeds the new
  `expPercent` snapshot field.
- Hunt loop (`internal/swarm/hunt`): the server target is only adopted
  while it is alive, the loop resets its own target when the target
  dies (no ping-pong), decisions run on a 250 ms cadence, a healthy
  character (HP >= 50 percent) selects the next target immediately
  (rate limited to one request per second for the flood protector) and
  a hurt character rests with a logged reason until regeneration
  recovers.
- `NearestAttackable` ranks the candidates by their projected current
  position (moving npcs advance from the segment start toward the
  destination at their effective speed, the server side counterpart of
  the web map interpolation).
- `web/map.js`: every visible player renders its selection as a violet
  dashed line to the target plus a violet dashed ring around it
  (including a ring around the bot itself when the bot is the target);
  the tooltip shows what a unit targets; the direction tick starts at
  the circle edge and the circle is a solid fill inside.
- HUD (`web/app.js`, `web/index.html`, `web/style.css`): unresolved or
  dead targets display `no target`; the `facing` field is gone; the
  experience is the third bar under HP and MP filled from `expPercent`
  with the percentage as text; HP/MP/EXP use the classic L2 C1 palette
  (HP red `#f04040 -> #c00000 -> #8b0000`, MP blue `#40a0ff ->
  #2060c0 -> #103080`, EXP gold `#ffe9a0 -> #d4af37 -> #8b6508`, light
  top / dark bottom cylindrical gradients - taken from C1 screenshots
  of the original client: red HP and blue MP bars in the status window,
  gold exp bar above the shortcut bar); the `.kv` rows separate label
  and value with a gap and ellipsize long values.

### Reproduction and verification

- `tools/repro_map_render.js` (`task repro:map`): drives the real
  `web/map.js` in a Node vm sandbox with a recording canvas. Checks the
  player target link (violet line + ring), the ring around the bot when
  it is targeted, the own target regression guard and that every
  direction tick starts at the circle edge. Validated RED: against the
  pre fix map.js 8 checks fail; against the fixed file all pass.
- `tools/repro_hud.js` (`task repro:hud`): drives the real `web/app.js`
  against a stub DOM. Checks the `no target` status (zero id, dead
  target, unresolvable id), the living target format, the exp bar fill
  and text, the HP bar and that `renderHUD` never touches the removed
  facing field. Validated RED: the pre fix app.js fails 7 checks.
- Hunt chain behavior: `go test ./internal/swarm/hunt` covers the
  post-kill target selection (`TestLoopSelectsNextTargetAfterKill`
  feeds the stale dead server target and requires a new selection),
  the rest gate (`TestLoopWaitsForHealthWhenHurt`) and the tracker
  clearing (`TestSelfTargetCleared*` in `internal/swarm/state`).
- Nearest choice: `TestNearestAttackableUsesProjectedPosition` spawns a
  standing mob near the character and a moving mob whose stale packet
  start is far away, and requires the moving one to be chosen at its
  projected position.
- Live run against the local Mobius stack (2026-09-05, server rebuilt
  from the gitee.com mirror of the Mobius repository because gitlab is
  unreachable from this environment; the AttackRequest double click,
  Player.setTarget and experience.xml of the mirror were byte compared
  against the gitlab HEAD analysis sources and carry the same
  semantics): 15 kills in 200 seconds with no idle gaps (kill, loot,
  next engagement every 5-11 seconds), the rest gate engaged exactly at
  HP 45/38 percent below the 50 percent threshold with logged reasons
  until regeneration recovered, the snapshot polled over the SSE state
  endpoint showed `expPercent` climbing 34.2 -> 48.6 percent across
  kills (and matching the hand computed (exp - base)/(next - base)
  value 23.35 percent at level 3 with 551 exp exactly) and `targetId`
  dropping to 0 right after each kill instead of dangling at the dead
  object id.

## Round 3: 24/7 supervisor, pickup approach, resting and web UI rework (2026-09-05)

User report after the long run on the fresh Windows deployment
(`E:\work\lineage_workspace_fresh`, server rebuilt from the current gitlab
master):

1. the process died after a long run (`game connection lost` -> exit 1):
   it must run 24/7 and never end without the user asking;
2. when picking up a drop the character did not run toward the adena on
   the map and teleported at the end;
3. the HUD needs a target window (name, level, hp, mp) next to the
   character panel and the `target` row must leave the character card;
4. the bot list needs mini HP/MP/XP bars and a combat status;
5. the gold EXP bar color is wrong: gold is the CP color of later
   chronicles, the C1 experience bar is light silver below HP/MP;
6. the header wastes vertical space (brand row + tab row before the map);
7. the bot must sit down at low HP and stand up recovered (sitting
   regeneration is faster);
8. unchecking `follow` still moved the map with the character;
9. the bootstrap installation analysis for the Windows host.

### Root causes and fixes

- Death recovery (found during the verification run): a level 6 hunt
  session died mid fight (StatusUpdate CUR_HP 0) and the hunt loop sat
  in the rest gate forever - the server refuses sit requests of a
  corpse, so the bot stayed a corpse and hunted nothing. A dead
  character now runs the death dialog choice automatically:
  `GameClient.RestartAtVillage` sends RequestRestartPoint 0x6D type 0
  (the server revives the character at the nearest village with
  restored vitals), the request retries every 5 s until the revival
  lands and the stale target/loot references are dropped
  (`state.Bot.SelfDead`, `hunt.recoverFromDeath`).
- Process exit: `main` ran exactly one session and turned every session
  error into `os.Exit(1)`. The fresh deployment additionally proved that
  the server kicks an old session with LeaveWorld 0x96 + a forced socket
  close on a double login (`World.addPlayer`), so sessions genuinely end
  from the outside. `cmd/swarm` now runs `runBotForever`: every failed
  session is logged, the tracker is reset (`state.Bot.ResetSession`,
  objects and inventory do not leak across sessions, events and uptime
  survive), the hunt loop of the old session is stopped through a derived
  context and the full login flow is retried with a 2..30 s backoff that
  only grows across unstable sessions. `GameClient.run` only announces
  the Logout packet while the connection is still usable, which removes
  the misleading `Failed to send logout` after a transport error.
- Pickup teleport: two independent causes. (a) `ApplyItemPickup` snapped
  the picker to the GetItem coordinates, but that packet carries the ITEM
  position (`Item.pickupMe`), so every completed pickup teleported the
  marker to the drop. The tracker no longer moves the picker on GetItem.
  (b) The hunt loop clicked the item from any distance and waited: the
  visible approach depended on the server AI alone. The loop now walks to
  the item first - `GameClient.WalkTo` sends the client MoveToLocation
  0x01 packet (the ground click of the official client, mouse mode) -
  and starts clicking at 60 units, so the map shows a smooth run toward
  the drop. Live packet trace (SWARM_TRACE_PACKETS=1): DropItem 0x16 ->
  [walk 0x01] -> StopMove 0x59 (arrival broadcast) -> StopMove 0x59
  (`doPickupItem` self copy) -> GetItem 0x17.
- Resting: the hunt loop only paused below 50 percent HP without any
  action. It now sits down below 30 percent (`GameClient.ActionSitStand`
  sends RequestActionUse 0x45 action 0) and stands up at 90 percent.
  The toggle is confirmed by the ChangeWaitType 0x3F broadcast (tracked
  as `Bot.Sitting`); a repeat is only sent when the flip never happened,
  so a slow confirmation can never toggle back. `Bot.SelfUnderAttack`
  (last hit within 3 s via Attack/MoveToPawn broadcasts with the bot as
  target) keeps the loop fighting instead of sitting into the blows.
- Target panel: the C1 server sends NPC vitals only to status listeners,
  that is to whoever targeted the npc (`Player.setTarget` answers with a
  one shot StatusUpdate MAX_HP/CUR_HP and `broadcastStatusUpdate` sends
  only those two attributes on damage). The tracker already stored
  object HP; it now also stores CUR_MP/MAX_MP (players) and the web
  resolves the target object from the snapshot. The HUD stack grew the
  TARGET panel (name, level chip, HP bar, MP row that reads "—" for
  npcs because the server never sends their MP), the `target` row left
  the character card and both panels share one vertical stack, so long
  names can no longer break the layout.
- EXP bar color: light silver `#f4f4f4 -> #c7cbd1 -> #9096a0`
  (gold is the CP color of later chronicles, per the user's C1
  screenshot). The three gradients now live in shared CSS variables
  (`--grad-hp/mp/xp`) reused by the HUD bars and the sidebar mini bars.
- Header: brand, Map/Log tabs and the live indicator share one 34 px
  row; the tab strip above the map is gone.
- Bot list: every row renders the status dot, name, `combat`/`rest`
  chips and the level, plus three mini bars (HP red, MP blue, XP
  silver) fed by the extended `/api/bots` payload (`curHp/maxHp/
  curMp/maxMp/expPercent/inCombat/sitting`).
- Follow: the free camera was centered on the character unless dragged,
  so the map still tracked the bot with follow off. The view now pins
  to a pan anchor that is captured at the moment follow is disabled
  (checkbox or drag) and never moves on its own.
- Installation analysis: from this Windows host the fastest working
  path was BellSoft Liberica JDK 25 (MSI, JAVA_HOME), MariaDB through
  XAMPP (`C:\xampp`, started by `mysql_start.bat`, database loaded by
  the dist `DatabaseInstaller`) and a plain gitlab clone built with
  Apache Ant; the servers start from the dist `.vbs` launchers. The
  bootstrap script URLs (Adoptium JDK 25, MariaDB 11.8.9 bintar) were
  re-verified reachable on 2026-09-05 and stay unchanged; the old
  "gitlab is unreachable" note is obsolete for this environment.

### Reproduction and verification

- `tools/repro_hud.js` (`task repro:hud`): the target panel checks (no
  target status for zero/dead ids, name + level chip + HP bar for a
  living target, MP "—" without server data), the unknown-vitals "—"
  rendering and that `renderHUD` no longer writes the target.
- `tools/repro_map_render.js` (`task repro:map`): new scenario
  "follow off keeps the view static" - a fixed world point keeps its
  screen position while the character moves with follow off, and the
  view tracks again when follow is re-enabled (RED against the pre fix
  map.js: the view moved with the character).
- Hunt unit tests (`internal/swarm/hunt`): `TestLoopWalksToFarLoot`
  (walk first, click on arrival), `TestLoopSitsDownWhenExhausted`
  (sit at 20 percent, no toggle spam while confirmed, stand at 95,
  engage again), `TestLoopDoesNotSitWhileUnderAttack` and
  `TestLoopRestartsAfterDeath` (restart request on death, retry until
  the revival, fresh target afterwards). State tests:
  `TestSpawnItemAndPickup` now asserts the picker does NOT move to the
  GetItem position.
- Live run on the local Mobius stack (2026-09-05, account swarmqa,
  level 5 -> 7 in two sessions): kills chain with zero pickup timeouts,
  the traced pickup flow matches the packet list above, the web UI
  (checked in a real browser at 1440x860) shows the compact header,
  the silver XP bar, the stacked target panel with live HP of the
  claimed mob and the sidebar mini bars with the combat chip.


## Round 4: map drag grab semantics (2026-09-06)

User report: dragging the map with the held left button moved it
asynchronously - like swiping instead of holding the map under the
cursor.

### Root cause and fix

The drag handler added the raw mouse delta to the world space pan
anchor of the free camera: the map moved in the same direction as the
cursor (a grab needs the opposite) and unscaled, so at the default zoom
0.12 it slid about eight times further than the pointer. The handler
now moves the camera by `-delta cursor / scale`, which pins the grabbed
world point to the cursor exactly, and follow is switched off already
at mousedown so the grab starts without the camera drifting away under
the held point.

### Reproduction and verification

- `tools/repro_map_render.js` (new scenario "map drag keeps the
  grabbed point"): fires a real mousedown/mousemove/mouseup sequence
  through the captured canvas and window listeners and requires every
  world point's screen position to shift exactly by the cursor delta
  plus the follow disabling. Validated RED against the pre fix map.js
  (the map moved the wrong direction and off scale), GREEN now.
- Live browser check on the local stack (1440x860): a held button drag
  by (+150, +120) px shifted the self marker, the npc labels and the
  grid lines by exactly (+150, +120) px and unchecked follow.


## Round 5: exact movement recurrence, jerk free rendering (2026-09-06)

User report: the drawn character and mobs still moved jerkily - constant
linear speed along a straight line, then a sudden displacement at a much
higher speed, especially during loot pickup and the approach to mobs.

### Root causes

- The web map scaled the interpolation speed by a learned per template
  arrival gap. Interrupted segments (the hunt loop re-issuing its walk
  to the same drop, chase re-targets) poisoned the gap estimate up to
  the 60 unit bound; the scaled projection then arrived early, stood at
  the destination and caught up with the next packet in one fast glide:
  exactly the reported "linear motion, then a sudden jerk". Short
  pickup hops suffered most because a 30-60 unit gap error is huge
  relative to a 40-150 unit hop.
- Nobody modeled what the server actually does. The Mobius
  Creature.updatePosition recurrence is not a constant speed walk: it
  advances `xAccurate += (dest - xAccurate) * frac` with
  `frac = speed * ticks / 10 / (remaining - collision)` on 100 ms game
  ticks and snaps to the destination once frac exceeds 1 - a converging
  geometric walk that is slightly faster than the nominal speed and
  stops collision units short. Any linear model diverges from it by a
  few percent over a segment, which the old code then "corrected" with
  the learned gap.

### Fixes

- NpcInfo and CharInfo now parse the collision radius (a writeDouble in
  both packets, previously skipped) and carry it through the tracker
  into the snapshots; the played character uses the constant 9 (UserInfo
  has no collision field).
- `web/map.js projectTickwise` replays the exact server recurrence tick
  by tick from the packet position, speed and collision radius and
  interpolates linearly between the two surrounding tick positions (the
  server truth is a step function; the official client renders it the
  same smoothed way). The drawn position chases this projection with a
  speed cap of 1.35 times the unit speed: delivery latency bursts,
  retargets and arrival snaps become a slightly faster glide instead of
  a jump, and in steady motion the drawn position sits exactly on the
  projection with zero lag. The whole learned arrival gap machinery is
  gone.
- Speed semantics verified in the server sources and documented: the
  move speed is re-read every tick (buffs and debuffs apply immediately
  with the next broadcast), walk/run is the tracked ChangeMoveType
  flag, races differ through their base speeds and the move multiplier,
  and the broadcast values divide by the multiplier which the tracker
  multiplies back. With PathFinding enabled a long move is a chain of
  node segments, each announced by its own forced MoveToLocation - the
  per packet recurrence handles that without extra work.

### Verification

- tools/repro_movement.js was rebuilt as the position tracking harness
  the user asked for: seven scenarios (npc random walk, npc long run,
  character walk, character chase, pickup hops, retarget approach, npc
  chase of the player) run a faithful server simulation (100 ms ticks,
  the exact recurrence including its geometric advance, the 1 second
  broadcast throttle) and compare every rendered frame against the
  simulated truth: max position error <= 0.21x the unit speed in world
  units (one tick phase plus the step vs linear rendering difference),
  arrival error <= 6 units, max frame speed spike 1.35x (the chase cap
  itself). `--frames` prints the per frame position log of every moving
  unit with the drawn position, the server truth and the error.
- `--record <seconds> [file] [url] [bot]` captures the live SSE stream
  to a JSON file; `--replay file [--frames]` replays it through the
  real map at 60 fps and fails on spikes above 2.5x. Live check: 100
  seconds of the hunt on the local stack (account test1, level 9, 14
  kills, 30 moving units) replayed at 6012 frames with zero spikes and
  the frame log showing the constant per frame displacement of every
  unit.


## Round 6: HUD polish (2026-09-06)

User requests: the target widget must exist only while there is a
target, the word `target` on it only distracts, x/y/z belong in the
left grid column with exp/sp starting the right one, the weight should
read as a bare percentage, and the combat chip must not crowd the name
- the name is the heading.

### Changes

- `web/index.html`: the target panel starts hidden and the `target`
  panel label is gone (the name row carries the level chip directly);
  the character grid is reordered to level/race, x/exp, y/sp, z/slots,
  weight/adena; the combat and rest chips moved from the name row into
  the class line.
- `web/app.js renderTarget` hides the whole panel instead of rendering
  a `no target` placeholder; the weight renders as `54%` without the
  exact load value.
- `web/style.css`: the class line layout, the panel label and the
  no-target dimming rules removed.
- `tools/repro_hud.js` follows: a missing or dead target must hide the
  panel (hidden class), a living target must show it; the mp no data
  and renderHUD separation checks stay.

### Verification

- All three harnesses pass; live screenshot on the local stack (bot in
  the village, no target) shows the target panel absent, the name as
  the heading and the reordered grid.


## Round 7: system message and social action chat (2026-09-06)

User request: parse SystemMessage (0x7A) and SocialAction (0x3D) - the
two unknown packets flooding the log during a hunt - and show them in a
small chat window in the bottom left corner of the web interface.

### Implementation

- `tools/generate_system_messages.sh` (task `generate:system-messages`)
  generates `npcdata/system_messages.go` from the @ClientString
  annotations of SystemMessageId.java: 778 message ids with their client
  side texts ("You picked up $s1 adena." and so on).
- Parsers: `from_game_server/system_message.go` reads the message id and
  the typed parameters (text and player name strings, skill name int
  pairs, zone name triples, plain ints), `from_game_server/social_action.go`
  reads the actor object id and the action id (15 = level up). Both
  leave the unknown packet log, so the hunt log stays clean.
- Tracker: `state/chat.go` keeps a rolling 64 line chat window per bot
  (snapshot `chat`), formats the message text by substituting the $sN
  placeholders positionally and resolves item and npc name parameters
  through the generated dictionaries. Social actions render as
  "<name> plays social animation N" or "<name> reached a new level".
- Web: a translucent chat window sits in the bottom left corner of the
  map, system lines in the normal text color, social lines violet, auto
  scrolled to the newest line.

### Verification

- Packet tests: adena message with an int parameter, item name plus
  text parameter reuse of the scratch buffer, implausible parameter
  count rejection, social action fields.
- Tracker tests: adena text formatting, item name resolution through
  the generated dictionary, unknown id fallback, actor naming
  (self/npc/unknown) and the 64 line ring roll over.
- `tools/repro_hud.js`: the chat window renders one line per message
  with the message text and the social class, an empty snapshot clears
  it.
- Live: the bot in the village shows "Welcome to the World of Lineage
  II." and the idle animations of the surrounding npcs in the chat
  window; the unknown packet log lines for 0x7A and 0x3D are gone.


## Round 8: chat window auto scroll (2026-09-06)

User request: the chat window must follow the newest message by
default, stop following when the user scrolls up to read the history
and resume once the view returns to the bottom.

### Implementation and fixes

- `web/app.js`: the chat keeps a `stick` state (ChatWindow), driven by
  the scroll position (`chatAtBottom`, 4 px tolerance) and consumed by
  renderChat - the view scrolls to the newest line only while stuck.
  `main.js` attaches the scroll tracking at boot.
- Live verification exposed a real rendering bug: the auto scroll set
  scrollTop on the inner list while the scroll container was the outer
  box, so the window actually never followed the newest line (it
  showed the top of the log). The overflow now lives on the list
  itself (the box only clips).
- `tools/repro_hud.js` covers the state machine: a stuck window scrolls
  to the newest line, a scrolled up window keeps the chosen offset and
  chatAtBottom distinguishes the bottom within the tolerance.

### Verification

- All hud harness checks pass.
- Live cycle on the local stack: scrolling the list to the top
  detached the follow (stick false, scrollTop stayed 0 while new
  social lines arrived), scrolling back to the bottom resumed it
  (stick true, scrollTop pinned at the newest line).


## Round 9: social markers instead of chat spam (2026-09-06)

User request: the social interactions spammed the chat window; keep
them out of the chat and hint them unobtrusively on the animating npc
instead, widen the chat by 30 percent (long standard messages did not
fit) and test against the hunting character (test1) instead of the
village stuck swarmqa.

### Implementation

- ApplySocialAction no longer writes the "plays social animation"
  lines: every social lights a 3 second marker on the animating
  creature instead (WorldObject.SocialUntil / the character one, snapshot
  socialUntilMs), rendered by web/map.js drawSocialMarker as a small
  fading ring above the unit. Level ups stay in the chat (rare and
  meaningful). Views without the new field are guarded against the NaN
  window.
- The chat window widened 320 -> 416 px.

### Verification

- state tests: idle gestures produce no chat lines but set the marker
  window on the npc; level ups of self and named npcs stay in the chat.
- tools/repro_map_render.js scenario "social animation marker": the
  ring draws above the animating npc and does not draw without the
  window.
- Live run against test1 (hunting Gremlins and Red Keltirs): the chat
  shows only the combat and loot messages ("You did 12 damage.",
  "Critical hit!", "Earned 10 adena.", "You picked up Apprentice's
  Shoes.") with zero social lines; the visible side effect of the
  system messages: the junk destroy requests of the hunt loop hit the
  server flood protector ("You are destroying items too fast.") - the
  destroy batch needs a rate limit as a follow up.


## Round 10: world map background with a tile pyramid (2026-09-06)

User request: the map canvas is a flat fill - use the game map images
like L2Bot2.0 does (its Client/Assets/maps tiles), ship them in this
repository, add a background toggle and take care of the far zoom with
a google maps style level of detail.

### Implementation

- Coordinate anchors (L2Bot2.0 MapImageSelector plus World.java of the
  Mobius server): one tile covers 32768 world units at 1024x1024
  source pixels (32 units per pixel), named `BX_BY.jpg` with
  BX = floor(x / 32768) + 20, BY = floor(y / 32768) + 18. The `_1` and
  `_2` suffixed source files are dungeon floors, not detail levels, and
  are skipped.
- `tools/generate_map_tiles.sh` builds the pyramid into
  `web/maps/{level}/{bx}_{by}.jpg` (jpeg quality 72, lanczos): level 0
  full resolution only for the detail window around the hunting
  grounds (bx 20..22, by 17..20 by default, parameters), levels 1..3
  (512/256/128 px) for the whole world - 573 tiles, ~11 MB, embedded
  through the existing go:embed of the web tree (binary grew ~11 MB).
- `web/map.js drawMapBackground` draws the tiles covering the viewport
  through the usual world to screen transform (follow and pan work
  unchanged), picks the pyramid level allowing at most a 2x upscale of
  the tile pixels (google maps rule), lazy loads the images through
  the static file server and redraws on arrival. The toggle is the
  `show-map` toolbar checkbox; the grid, zone, units and links draw on
  top of the background.

### Verification

- All tile pyramid levels serve through the web server (L0 139 KB,
  L1 38 KB, L2 12 KB, L3 4 KB for 21_19).
- Live screenshots on test1 (hunting at the Elven Fortress): the
  default view shows the fortress courtyard with the units at their
  real positions; four zoom out steps keep the world continuous and
  sharp at level 0 (777 px drawn tiles, within the 2x upscale rule);
  the harnesses run unchanged because the vm sandbox has neither the
  show-map checkbox nor an Image constructor.


## Round 11: stable draw order and zoom scaled markers (2026-09-06)

User report: when the map is zoomed out the mobs flicker - one draws
above the other, then swaps - and the unit markers should shrink
together with the map on zoom out.

### Root cause and implementation

- The flicker: the snapshot objects arrive in the iteration order of a
  go map, which is random on every snapshot; drawObjects walked them as
  they came, so the z order of overlapping units changed from frame to
  frame. The draw order is now deterministic: dead units first, then
  north to south (a pseudo depth), then the object id.
- The markers scale with the zoom: the unit radius is multiplied by a
  sub linear factor of the map scale (`(scale / 0.12) ^ 0.6`, clamped
  0.3..1.6) - zooming out shrinks the markers together with the map
  while they stay visible at the far end, zooming in grows them
  bounded. The self marker, the target link rings and the social
  marker follow the same factor; the direction tick and the combat
  pulse scale with the radius inside drawUnitTick.

### Verification

- tools/repro_map_render.js scenario "stable draw order": three draw
  rounds with the snapshot array reversed between rounds require one
  stable draw order and the north to south y order (RED against the
  pre fix map.js, which drew in array order).
- Live check on test1 at scale 0.0158: the markers render at the
  0.30 factor, the runtime tracks every unit and the pixel probe finds
  the map, the mobs and the self marker on the canvas.
- Diagnostics note: an in app browser pane that is not visible never
  fires requestAnimationFrame, so the map canvas stays at its last
  painted state and the runtime map stays empty - a blank canvas in
  the probes means the pane is hidden, not that the rendering broke.


## Round 12: direction ticks scale with the zoom (2026-09-06)

User report: on zoom out only the circles shrink, the look direction
ticks keep their length.

### Fix

- drawUnitTick takes the marker zoom factor (unitScale) as opts.scale:
  the tick length beyond the circle edge, every line width (body, combat
  pulse, attack ring, self ring) and the dash pattern scale with it, so
  a zoomed out marker is a proportionally small circle with a small
  tick. The social marker ring and the self accent ring scale the same
  way.
- advanceRuntime now snaps the drawn position to the projection when
  the chase distance is not finite: a NaN that reached the drawn
  position used to reproduce itself through the chase arithmetic
  forever (dist stays NaN and no branch ever reassigns).

### Verification

- Live measurement on test1 through a patched canvas context: at scale
  0.02 the median tick length is 1.54 px (unitScale 0.34), at scale
  0.12 it is 4.50 px (unitScale 1.0).
- All harnesses pass.


## Round 14: theme independent marker outlines (2026-09-06)

User report: with the dark theme on the marker outlines and the
direction ticks turned near white (they followed the theme text bright
variable) and looked bad over the light map imagery.

### Fix

The outline and the direction tick use the fixed mapColors.tick (a
middle slate 39424e) instead of the theme variable: the slate reads
over the light map tiles and over both theme fills, matching the rest
of the fixed marker palette.

### Verification

Live pixel probe on test1 in both themes: the near white tick pixels of
the dark theme are gone (the remaining near white pixels are the map
imagery itself, identical across the themes), the slate tick pixels are
present at the same count in both themes.


## Round 16: map background outside the detail window (2026-09-06)

User report: panning two tiles left of the character and zooming in
blanked the map background for that area, while the zoomed out views
showed it fine.

### Root cause and fix

The tile pyramid ships full resolution level 0 only for the detail
window around the hunting grounds; a zoomed in view outside the window
requested the missing level 0 tiles and drew nothing (the zoomed out
views worked because levels 1..3 cover the whole world). The draw walk
now follows the pyramid upwards: a tile whose own level is missing
(404 remembered as entry.missing) falls back to the closest existing
level of the same block and stretches it over the tile rect - the area
renders at a proportionally softer detail instead of disappearing.

### Verification

- Live reproduction of the user scenario (pan two tiles left, scale
  0.08): the tile states walk 0/19_19 missing -> 1/19_19 ready and the
  left half of the canvas paints at 100 percent coverage with ~4800
  distinct colors.


## Round 17: full resolution world map everywhere (2026-09-06)

User request: every region must support the full resolution - after
panning two tiles left of the character and zooming in, the map stayed
blank even though the zoomed out views showed the area.

### Implementation

- tools/generate_map_tiles.sh drops the detail window: every base tile
  of the source set ships at all pyramid levels 0..3 (1024/512/256/128
  px, jpeg quality 65) - 748 tiles, 31.1 MB embedded (the binary grew
  accordingly). The ancestor fallback of the draw walk stays as the
  safety net for the blocks that do not exist in the source set.

### Verification

- The user scenario (pan two tiles left, scale 0.08): the left canvas
  half paints at 100 percent coverage, 10416 distinct colors, the full
  resolution level 0 tile 19_19 loaded and used directly.
- /maps/0/19_19.jpg serves 200 with the full 156 KB tile.


## Round 19: rest threshold raised to 60 percent (2026-09-06)

User request: resting should start below 60 percent hp, not 30 - a
character that leaves a fight at 30 percent may not have enough health
to kill the next mob, which turns every engagement into a death risk.

### Implementation

- The sit down threshold and the engage gate moved together from 30/50
  to 60 percent: a hurt character sits right away instead of standing
  around, stands up at 90 percent and only then chains the next target.
- TestLoopSitsAtTheNewThreshold covers the band: 55 percent sits down,
  65 percent engages again.


## Round 20: hunting zone (2026-09-06)

User request: a hunting zone concept - an area the bot attacks inside
and never leaves; a 900 unit square centered just below the Newbie
Helper for the start.

### Implementation

- state: the Zone type (square center + half, Contains) filters the
  target selection (NearestAttackable) and the loot selection
  (NearestGroundItemExcluding); SetHuntingZone stores it on the bot and
  the snapshot carries it for the map.
- hunt: SetHuntingZone configures the loop; the engage phase leashes -
  a character outside the square walks back to the zone center instead
  of hunting (this also covers the village respawn: the death spot
  return walk was replaced by the zone walk). The target pick moved
  from GameClient.AttackNearest into the loop (the tracker pick with
  the zone filter + the existing AttackTarget request flow), and the
  drops outside the zone are ignored.
- cmd/swarm configures the default zone; web/map.js drawHuntingZone
  draws the dashed amber square with a label under the units.

### Verification

- state tests: the zone filtered loot and target selection.
- hunt tests: TestLoopWalksBackIntoTheZone (the leash walks to the
  zone center and stops once inside), TestLoopIgnoresMobsOutsideTheZone
  (an outside mob is never attacked, an inside mob is), plus the
  reworked engagement tests for the tracker based target pick.
- tools/repro_map_render.js scenario "hunting zone": the dashed square
  edges and the label draw with a configured zone and do not without.
- Live: the zone renders around the Elven fortress hunt and the
  character hunted inside it (no leash events needed).
- tools/repro_map_render.js recorder bug fixed on the way: multi edge
  strokes recorded their segments from the wrong start point, which
  misrendered the zone square check.


## Round 21: hunting zone widened to 1900 (2026-09-06)

User request: the 900 unit square turned out too small.

### Implementation

DefaultHuntingZone half moved 450 -> 950 (a 1900x1900 square around the
same center below the Newbie Helper).


## Round 22: hunting zone widened to 2500 (2026-09-06)

User request: the zone grows again - 2500x2500 units (half 950 -> 1250)
around the same center below the Newbie Helper.

## Round 23: deleveling against the vanilla server (2026-09-07)

User request: delevel through a melee provocation of archer guards only
(melee guards may never join the fight, archers always retaliate in the
line of sight), never try to delevel at level 9 (a guard death there
removes no experience - Lucky absorbs it - while at level 10 the penalty
lands, verified by the user personally), and stop patching the game
server behavior: the server is the reference and only logging patches
are allowed from now on. The geodata moves into the repository so the
bot never depends on the server tree again.

What was measured and changed:

- The behavior patches of the previous session (guard revenge through
  the aggro list, the NPC kill exp penalty, the removed startFollow)
  were reverted; the local server checkout now differs from vanilla
  only by three logging lines (DEATHLOG in Player.doDie and
  Player.calculateDeathExpPenalty, GUARDDMG in Guard.addDamage, MOVEDBG
  in MoveToLocation). The old mobius_server_delevel.patch left the
  tools/ folder and AGENTS.md gained the "server integrity rules".
- The exp penalty misreading is corrected: the doDie penalty branch
  runs for EVERY killer, not only playable ones - the braces place it
  inside `if (killer != null)` but outside the playable killer gate.
  The guard death at level 10 therefore pays the vanilla penalty (live
  DEATHLOG: level 10, exp 48229 -> level 9, exp 46190, lost 2039 of the
  22972 level span), and the earlier "NPC deaths never lose exp"
  diagnosis was wrong. The previous patch had only masked that with an
  equivalent branch.
- Why archer guards: thinkAttack lets an ATTACK-intention NPC strike
  its most hated target without any karma gate once it is inside the
  weapon range - 850+ units for the ARCHER ai type (Kendell and
  Starden, Elven Bow, range 1100), but only ~57 units for the melee
  sentinels (Veltress and Rayen, Elven Sword, range 40). A melee guard
  whose provoker stands farther than that drops the chase in the
  checkTarget gate (Player.isAutoAttackable returns karma > 0 for
  guards) and only follows (Guard.addDamage startFollow), so it may
  never swing at all; an archer always shoots back. The bot now
  provokes Kendell and Starden only, walking into melee (60 units)
  before the attack request.
- The deleveling rules: the trigger requires level >= 10
  (delevelMinLevel; below that the deaths are free because of Lucky,
  Player.isLucky gates at level <= 9) and the target clamps at 9 - the
  last productive death happens at 10 and drops the character to 9.
  A free death counter compares the exp before and after every death
  (the server refreshes the UserInfo exp on every change through
  PlayerStat removeExpAndSp -> updateUserInfo): three consecutive
  penalty-free deaths abort the deleveling with a 30 min cooldown, a
  safety net for penalty-free configurations that never triggers on
  this stack.
- The geodata region 21_19.l2j now lives in data/geodata of this
  repository - the first candidate of the bot's geodata detection - so
  "no geodata found" sessions like today's cannot happen again; the
  server keeps its own copy in dist/game/data/geodata with
  PathFinding = 2.
- Live validation on the vanilla server (test1 pushed to level 10,
  exp 48229 through the database): trigger -> pathfinding walk to
  Kendell (~85 s) -> melee provocation (GUARDDMG at distance 0) -> the
  archer killed the character in ~7 s -> exp penalty to level 9
  (DEATHLOG) -> "delevel finished at level 9, walking back" ->
  pathfinding walk back to the farm spot (~62 s) -> farming resumed
  with loot pickups. The character ended the window at level 9,
  exp 46277 in the database.

Round 23 addendum: stale selections after abrupt disconnects (2026-09-07).

After the live validation above, two extra stability runs exposed a
reconnect pathology: a bot process killed abruptly mid-farm (SIGPIPE of
the log pipe) left the character auto attacking server side; the target
died under the ownerless auto attack and the corpse stayed SELECTED
(the vanilla server never clears a corpse selection, only the next
selection of a different object replaces it). The next login then had
every forced attack on that object id answered ActionFailed - two
refusals per second, zero kills, no server side log (the refusals are
silent in the vanilla AttackRequest branches). The state API showed
inCombat true with the stale target id, the tracker saw the respawned
npc alive at ~200 units, and a game server restart healed it instantly
(farming resumed), which isolated the state to the character object.

Fixes, all bot side (the server behavior stays the reference):
- The engage now drops a target whose repeated attack requests never
  started the fight within engageStuckTimeout (12 s) and skips the
  object id for engageSkipDelay (30 s): the next pick necessarily
  selects a DIFFERENT object id, and that selection replaces the stale
  corpse selection on the server - the only vanilla way out. The server
  target adoption ignores skipped ids for the same reason.
- The state tracker grew NearestAttackableExcept so the pick can
  exclude the skipped objects while they cool down.
- The local server checkout gained ATTACKLOG diagnostics lines (pure
  logging, allowed by the server integrity rules) in every
  AttackRequest and Creature.onForcedAttack refusal branch, so the next
  occurrence names the refusing branch directly.

The reproduction is timing dependent (the kill must land exactly while
the auto attack runs and the target must die after it); three deliberate
kill -9 attempts missed the window, so the recovery path is covered by
TestEngageSwitchesStuckTarget instead.

## Round 24: the full geodata pack moves into the repository (2026-09-07)

User request: store ALL geodata files in the project, not just the one
region of the current hunting zone, so future zones can use them
without another download hunt.

What was done:

- The full old-world pack (the C1 continent) now lives in
  data/geodata: 165 region files X_Y.l2j covering the region grid
  16_10..26_26 (about 544 MB, the l2j headerless format the server and
  the pathfind engine read). Source: the INTERLUDE/Geodata2/geodata
  directory of the LGK-Games/Geodata GitHub mirror of the upstream
  pack. Every file was sha1-verified against the upstream git tree
  during the download (scripts/download_full_geodata.sh outside the
  repository keeps the manifest); the pack lineage was proven before
  the bulk import: its 21_19.l2j is byte identical (md5
  5f45ef9f0924ba691dfe962bf892cab1) with the region the elven lands
  bot already navigated and the running server already loaded, so no
  format or grid surprises are possible.
- data/geodata/Readme.txt documents the provenance, and .gitattributes
  marks *.l2j binary so git never rewrites the region files.
- The town route test now finds the in-repository pack on its own:
  townGeodataCandidates walks up from the test CWD to the repository
  root (go test always runs in the package directory, so the old
  relative "data/geodata" never resolved; on the reference Windows
  machine only the absolute E:\ candidate matched). TestFindPathToShopDeck
  left its skip state and ran for real over the multi-region pack
  (10.8 s, PASS): the plain search reaches the Elven Village shop deck
  across regions, the strict deck-targeted search reports the known
  deck disconnect and the town trips keep the plain-search fallback.
- go test ./... green over the full repository (12 packages).

The server keeps its own subset in dist/game/data/geodata (21_19 for
the current elven lands hunt); adding every region there would only
grow the GeoEngine memory for no present need. The bot detects
data/geodata first and is now self-sufficient for any future hunting
ground on the old continent.

## Round 25: the equipment widget with the item icon pack (2026-09-07)

User request: show the equipped items and the inventory of the
character as a separate right side widget, with the item icons from
the xMlex/l2walker data/l2icons pack added to the project as data -
but verified against our C1 version first.

What was verified and done:

- C1 compatibility of the icon pack: the Mobius C1 item stats carry
  the canonical icon name of every item (set name="icon"), so the pack
  was checked against the server data first: 4044 of the 4222 C1
  items resolve by the exact icon name. The missing 178 are almost
  entirely the low grade starter armor (bone/bronze/leather gear of
  the 20-60 id range) whose icons the later clients renamed to the
  generic armor_tXX pattern; every one of them resolves through the
  l2walker Interlude item database (data/db/db.sqlite of the l2walker
  checkout) by the same item id onto a renamed icon of the same
  artwork family. Result: 4222/4222 = 100% coverage, documented in
  data/icons/Readme.txt; the mapping is generated into
  npcdata/item_icons.go by tools/generate_item_icons.sh (the sqlite
  path is an argument of the script).
- The pack itself (3134 PNGs, 32x32, 13 MB) moved into data/icons of
  the repository; the web server serves it at /icons/<name>.png with
  a day of cache (webserver/icons.go): the route validates the name
  against the classic client naming scheme (letters, digits,
  underscore, hyphen), walks up from the working directory to find
  the pack like the geodata detection, honors the SWARM_ICONS
  override for tests, and answers 404 for every name when no pack
  exists so the widget falls back to its glyphs.
- The inventory packets now parse the body part mask and the enchant
  level of every entry (they sat inside the previously skipped 8
  bytes, see AbstractItemPacket.writeItem), the state snapshot gained
  the whole inventory as snapshot.inventory - every item with its
  slot mask, enchant level, resolved display name, icon file name and
  count, sorted equipped-first - and the connection layer copies the
  new fields through.
- The widget (index.html/.app-body third column, app.js renderGear,
  style.css): a 15 slot paperdoll in the classic three column layout
  (hair, earrings, neck, rings, head, chest, legs, weapon, shield,
  cloak, gloves, boots, shirt) where every equipped item lands by its
  C1 BodyPart mask - two handed weapons and full armor alias onto the
  weapon/chest slot, the either-or earring and ring masks (0x6/0x30)
  fill the first free slot of their pair - and a scrollable inventory
  grid below with one icon cell per bag item, stack count and
  +enchant badges, hover tooltips and the slots-used counter. Items
  without an icon (or a 404) fall back to a tinted per type2 glyph so
  no cell ever renders empty. The pathfind test mode hides the panel.
- Tests: TestParseInventoryBodyPartAndEnchant (packet layer),
  TestSnapshotInventoryWidget and TestSnapshotInventoryEnchant
  (state), TestIconServed/TestIconNotFound/
  TestIconDisabledWithoutPack/TestDetectIconsDirWalkUp (web server),
  and the tools/repro_gear.js stub DOM harness (12 checks: masks,
  aliases, either-or pairs, labels, badges, counter; task repro:gear).
- Live validation on the running stack: the snapshot of test1
  (Squire's Shirt/Pants/Sword equipped, dagger, sandals, adena 316 in
  the bag) renders 15 paperdoll cells with the three equipped icons
  on the right slots (chest 0x400, legs 0x800, rhand 0x80 straight
  from the server packets), the inventory grid shows every bag item
  with its icon - including armor_t01_u_i00, a renamed starter armor
  icon that only exists through the sqlite resolution - and
  /icons/etc_adena_i00.png answers 200 image/png. go test ./... green
  (12 packages).

## Round 26: compact equipment widget, no icon flicker, go 1.23 build fix (2026-09-07)

Three fixes reported from a fresh Windows pull:

- `go test ./... --cover` failed to build the webserver package:
  `testing.(*T).Chdir requires go1.24 or later (file is go1.23)` - the
  module pins `go 1.23.2` and icons_test.go used the go 1.24 only
  `t.Chdir`. Replaced with a local `chdir` helper (os.Getwd +
  os.Chdir + t.Cleanup restore) so the file compiles on both 1.23
  and 1.24 toolchains; no other go 1.24 API is used anywhere.
- The items widget flickered: every SSE snapshot wiped
  paperdoll/inv-grid innerHTML and recreated all cells, so every icon
  `<img>` was a fresh element - the browser re-decodes and re-paints
  even a cached image asynchronously, and all icons vanished for a
  fraction of a second on every snapshot (the bot pushes snapshots
  continuously while farming). renderGear is now a keyed incremental
  renderer: one persistent cell record per paperdoll slot (slot key)
  and per bag item (objectId) in a module-level registry; a snapshot
  only touches the cells whose change signature (itemId, icon,
  count, enchant, type2, name, equipped) actually differs. Within a
  changed cell the `<img>` element survives everything that does not
  alter the icon itself - stack counts and enchant updates rewrite
  only the text badges - and reordering moves the persistent cells
  (appendChild never reloads an image). Removed the pointless
  `img.loading = "lazy"` (a fresh lazy element postpones the decode
  even longer). Switching the observed bot resets the registry
  (resetGear in selectBot) so two inventories never mix.
- Compact layout: the 15-cell three-column paperdoll became two
  blocks sharing the 36px cell metric of the bag - the 3x3 wear
  block (head, cloak, gloves / weapon, chest, shield / shirt, legs,
  boots) on the left and the 2x3 jewelry block on the right with
  five slots (r.ear, l.ear / neck, hole / r.ring, l.ring): the
  middle-right cell is a blank `jewel-hole` because the classic
  character has only five jewelry slots. The inventory grid is six
  columns by four visible rows (36px cells, 3px gaps, fixed 153px
  height) with a thin scrollbar; the C1 hair mask (0x10000, unused
  in C1 items) lost its dedicated slot with the rest of the layout
  unchanged (masks, aliases 0x4000/0x8000/0x20000 and the either-or
  pairs 0x6/0x30 resolve exactly as before).
- repro_gear.js grew from 12 to 25 checks: the block layout (9 wear
  cells, 5 jewelry cells + hole), placement per mask, hidden labels
  on filled slots, and the flicker regression - the icon image
  element identity must survive an identical re-render, a count
  change and the weapon slot across snapshots; removed items drop
  their cells. The stub DOM append now mirrors the real one
  (appending an existing child moves it, never duplicates).
- Live validation on the running stack (bot -hunt, test1 lv 9 in
  combat): DOM probe marked all 9 icon elements and re-checked after
  4s and after 14s of active farming (snapshots flowing, adena badge
  updating from loot) - 9/9 survived, 0 recreated, all naturalWidth
  > 0, 0 JS errors; screenshot scripts/gear_compact_live.png.
  `go test ./... --cover --count=1` green (12 packages ok of 15, three without test files), gofmt/govet
  clean, node repro_hud.js ALL PASS.

## Round 27: floating equipment widget with the pinned adena and weight footer (2026-09-07)

The right column redesign from the review: the icons keep their
size, the widget stops being a huge white panel.

- The `<aside class="gear-panel">` column left the app body: the
  widget moved into `.map-wrap` as a floating overlay (position
  absolute, top 34px under the compass rose, right 12px, width
  254px, z-index 4) with the same chrome as the player HUD -
  panel background, border, radius and shadow - so the two read as
  a pair of overlays. The map reclaims the full body width (1400px
  at the smoke viewport instead of ~1150), the pathfind test mode
  still hides it. Icon metrics untouched: 36px cells, 32px icons.
- The pinned footer (`gear-foot`) under the scrolling bag: the
  adena line (`character.adena`, formatted, gold `--gold`) and the
  weight line below it - a load bar filled by
  `character.load / character.maxLoad` with the classic threshold
  colors (green below half load, amber `warn` past 50, red `heavy`
  past 90) and the percent text; the raw weight numbers live in the
  row tooltip, an unknown maxLoad shows the dash. Both lines stay
  visible whatever the bag scroll position is, and the values track
  the live stream (adena grows with the loot in the smoke).
  `resetGear` clears the footer on a bot switch so stale money or
  load never bleeds into the next character.
- New theme gradients `--grad-load/-warn/-heavy` in `:root`, shared
  by both themes like the vital bars.
- repro_gear.js grew from 25 to 37 checks: the footer values
  (formatted adena, percent text, fill width, amber at half load,
  red near the limit, dash on unknown load, tooltip numbers) and
  the placement pins - the panel sits inside `.map-wrap` after the
  canvas and before the chat box, no `aside` column remains in the
  html, the CSS block pins `position: absolute` at `top: 34px` /
  `right: 12px`, the 32px icon metric survives and the threshold
  color rules exist.
- Live smoke on the running stack (script
  scripts/floating_gear_smoke.sh, one shell so the bot survives the
  probe): panel parent = `.map-wrap`, computed style absolute
  34px/12px, adena 588 gold updating with loot, weight 18% green
  with tooltip "weight: 15,731 / 88,320", 3 paperdoll icons + 8 bag
  cells, flicker probe 11/11 icon elements alive after 10s of
  farming, 0 JS errors, screenshot
  scripts/gear_floating_live.png. Pitfall rediscovered: the icon
  pack resolves from the process working directory (data/icons with
  walk-up), so the bot must launch from the repo root - from
  /home/z/my-project every icon 404s and the error handler strips
  the imgs; the smoke script cds into the repo first.
- `go build`, `go vet`, `go test ./... --count=1` green (12
  packages ok, three without test files), node repro_gear.js OK
  (37/37), node repro_hud.js ALL PASS.

## Round 28: the interactive web UI - manual commands from the map and the equipment widget (2026-09-07)

The web UI stops being a passive observer: double clicks and
drag-and-drop drive the character.

- Two new C1 client packets (formats verified against the Mobius
  source): `RequestUseItem` 0x14 `[objectId]` - the equip/unequip
  toggle, one packet for both directions (UseItem.runImpl ->
  useEquippableItem); `RequestDropItem` 0x12
  `[objectId][count][x][y][z]` - the server accepts drops within 150
  units of the character only (RequestDropItem.runImpl), so the bot
  drops at its own feet, and splits stacks server side.
- Command pipeline: `POST /api/bots/{id}/commands` (webserver/
  commands.go, validated kinds: move/attack/pickup/useItem/drop,
  receipt 202, an event log line "user command: ...") ->
  `state.Bot.PushCommand` (commands.go: a 32 entry channel queue,
  non blocking, oldest dropped on overflow, drained on session
  resets) -> the hunt loop drains the queue every tick (hunt/user.go)
  and acts: useItem/drop execute immediately, move/attack/pickup
  switch the new `phaseUser` manual mode. The mode overrides the
  autonomous hunting: an active town trip is cancelled
  (resetTownTrip), the deleveling refuses movement commands (the
  guard walk must finish for the level to drop), the death recovery
  resets it. move re-clicks WalkTo at the 1s action cadence until
  arrival (60u) or a 90s timeout; attack re-requests the forced
  attack until the fight starts (30s timeout) and a died target
  falls into the loot phase like the autonomous engage; pickup walks
  to the item and clicks it within the 60u approach radius (30s
  timeout). GameAPI grew UseItem/DropItem, the GameClient implements
  them, GroundItemByID resolves the pickup clicks.
- Frontend: the canvas answers dblclick (map.js objectAt - the same
  hit test the tooltip uses: an attackable npc -> attack, a ground
  item -> pickup, ground -> move with the character z) with a fading
  click ring (userMark) drawn by the animation loop; the canvas also
  accepts widget drags (drop on the map -> dropItemOnMap). The
  widget cells are draggable and double clickable (app.js): dblclick
  posts useItem for wearable bag items and equipped items (never for
  adena/materials, type2 filter), a drag to the paperdoll area
  equips, to the bag unequips, to the map drops - stackable items
  (count > 1) open the drop-dialog (a small modal: typed count with
  clamp validation, all/cancel/drop, Enter/Escape), an equipped drag
  posts useItem then drop (the queue preserves the order).
- Widget alignment fix from the review: the gear panel sits at
  top 10px - exactly the height of the player HUD stack (both
  measured 117px in the live probe), the compass rose moved to the
  bottom right corner so the panel owns the top right.
- Tests: packet serialization (RequestUseItem/RequestDropItem byte
  layouts), hunt/user_test.go (9 tests: immediate useItem/drop at the
  self position, the manual phase switch with the suppressed
  autonomous engage, arrival and timeout, the killed manual target
  into the loot phase, the pickup walk/click/vanish, the delevel
  guard, the queue drain on session reset), webserver/commands_test
  (queue happy path, validation matrix, 404, all kinds round trip).
  repro_gear.js grew to 43 checks with a behavioral DOM: the stub
  elements record event listeners and a stub fetch captures the
  posts - the dblclick matrix (weapon/adena), the drag arm, the
  count dialog (stack opens it, garbage keeps it, a typed count
  commits 12), the equipped drag posting useItem -> drop in order,
  and the resolveDropCount pure function.
- Live smoke on the running stack (scripts/interactive_ui_smoke.sh;
  the account id of the endpoint is "test1", and the first run
  caught the character mid-delevel - the wait loop now polls for
  attackable mobs in reach before the probes): panel top == HUD top
  (117/117), the mob dblclick logs "user command: attacking object
  <id>" and cancels the town trip, the ground dblclick logs
  "user command: walking to ...", the weapon cell dblclick unequips
  (icon out, label back) and the bag dblclick re-equips (icon back),
  the adena drop command returns 202, the map shows "item appeared:
  Adena" and the adena line ticks 588 -> 587, the delevel guard
  refuses a move with a log line, 0 JS errors, screenshot
  scripts/interactive_ui_live.png.
- `go build`, `go vet`, `go test ./... -count=1` green (12 packages
  ok, three without test files), node repro_gear.js OK (43/43),
  repro_hud.js / repro_map_render.js / repro_movement.js ALL PASS.

## Round 29: commands without -hunt, the attack fixes, the trash destroy and the widget polish (2026-09-07)

User feedback round: the manual commands must work without `-hunt`,
double clicking the selected target must start the melee attack, an
occupied slot must swap, the trash destroys, the dialog buttons stay
readable and the wear grid takes the classic placement.

- Manual only mode: the loop always runs now (`cmd/swarm/main.go`),
  `-hunt` only toggles the autonomy. `Loop.SetAutonomy(false)` keeps
  the loop in the new `phaseIdle` phase: it drains the manual web
  commands exactly like the hunt mode, never engages, loots, trips or
  delevels on its own, and still restarts at the village after a
  death. The geodata engine loads in every mode (the region files are
  indexed lazily, a no-hunt session pays nothing until a long walk
  asks for a path).
- The "double click the selected target" bug, root cause one: the
  stale engagement. `SelfEngaged` trusted the auto attack flag and a
  10s combat window; after an interrupted fight (a retreat walk) both
  linger while the character stands - and being hit refreshed the
  window forever, so the manual attack and the autonomous engage both
  believed the fight was running and never re-requested the forced
  attack (the "change the target so the attack starts" symptom). The
  new `SelfFighting` view requires fresh fight evidence (a swing or a
  chase step within 3s, tracked by `CombatActiveAt`), the manual
  attack and the engage re-request on stale engagements, and a fresh
  fight refreshes the manual deadline so a long fight never hands
  control back mid swing.
- The same bug, root cause two: the stalled server chase. The Mobius
  packet executor runs every client packet as its own thread pool
  task (PacketExecutor -> ThreadPoolExecutor), and the AI chase
  stalls on some routes while the stuck chase packets keep the
  engagement fresh - the character stands next to a far target
  forever (the standing 2/s "Action failed" stream of the hunt was
  the same stall). The chase progress watchdog samples the distance
  to the target once per 3s window: no 50 units of progress with the
  target beyond the 150u melee radius makes the loop walk toward the
  target itself (the client WalkTo works where the AI chase's
  path search gives up - verified live: a 2427 unit click stalled
  for 2s, the loop walked, killed the target and pathfound back
  into the zone). The manual attack approaches far targets the same
  way: the first request selects, the walk closes the distance, the
  forced attack lands in swing range.
- Manual long walks: the server MoveToLocation handler silently
  refuses far targets (a village-to-farm 8.7k request never moved the
  character; short walks up to ~2k work). A click beyond 2000 units
  plans the bot side geodata path once (the navigator now exists in
  every mode) and follows the waypoints in server accepted legs (1s
  request pace, 1000u leg cap, pass-skip, stall re-issue) - the
  village-to-farm walk arrives in ~51s. Short clicks keep the direct
  request, and a walking character is never re-clicked (the old 1s
  re-click restart of the server path search halved the walk speed).
- Inventory pacing: two same-burst useItem packets (the unequip and
  the equip of a swap) race in the server thread pool and cancelled
  each other (each useItem is a toggle: equip A then equip B lands
  back at A). The loop spaces useItem/drop/destroy one second apart
  - a request inside the window defers to a later tick (the swap
  pair order is preserved by the deferred list).
- Equip swap: a double click or a paperdoll drop of a bag item whose
  slot is taken now unequips the old item first and equips the new
  one after it (`slotKeyOf` resolves the target slot with the same
  placement rules as the paperdoll renderer, the either-or pairs
  included).
- Trash destroy: the gear footer carries a trash bin icon left of
  the adena and weight lines; a cell dragged onto it destroys the
  item (the new `destroy` command -> RequestDestroyItem 0x59
  `[objectId][count]`, the packet and the server behavior were
  already in the tree for the junk cleanup). Stacks open the count
  dialog in the destroy mode (labels, note and the commit button
  relabel), an equipped drag unequips first - the server side
  handler would unequip itself, the explicit request keeps the flow
  uniform with the drop.
- The count dialog buttons: the `.btn` base pins 26x24px for the
  toolbar icons, which squashed the dialog buttons and shifted their
  text. `.drop-btn` now overrides with auto sizing, a min height and
  centered text.
- The target HUD panel answers a double click with the attack
  command for the shown target (attackable npc, alive - a friendly
  or dead target ignores the click).
- The wear grid takes the classic paperdoll placement: cloak top
  left, head top-center above the chest, shirt top right, weapon and
  shield flanking the chest, boots bottom left, legs bottom center,
  gloves bottom right. The item cells keep the plain cursor (no grab
  hint on hover), the drags work unchanged.
- Tests: hunt/user_test.go grew the destroy cases, the stale
  engagement re-request (a self-calibrating poll on the public
  tracker views), the fresh fight quiet, the manual only mode
  (idle, commands, kill, death restart), the far click planning,
  the near click direct walk, the command replacement, the swap
  spacing and the stall/approach walks; state tracks SelfFighting
  and SelfWalking freshness; the webserver matrix covers the
  destroy kind; repro_gear.js pins the new layout order, the cursor,
  the trash markup/css/flow, the dialog relabeling, the button
  sizing, the swap order, and the target widget clicks (68 checks).
- Live verification on the running stack
  (scripts/manual_ui_smoke.sh, the bot started without -hunt): 0
  autonomous activity lines, the wear rows read cloak/head/shirt and
  boots/legs/gloves, the destroy dialog is readable (67x26px,
  centered), the map dblclick walks and arrives, the bag weapon
  double click posts useItem(old) then useItem(new) ~1s apart and
  the swap lands (dagger equipped, sword in the bag), the trash drag
  destroys 5 adena (577 -> 572, zero ground items), the attack on a
  mob engages (the target panel shows it), the interrupted fight
  re-clicked on the same selected target restarts the swings (the
  mob dies, control returns to idle), and the target widget double
  click posts the attack. The hunt mode run: the 2427 unit far click
  stall-walked, killed, and the zone return resumed the hunt. 0 JS
  errors; screenshots scripts/manual_ui_live.png and
  scripts/manual_ui_swap.png.
- `go build`, `go vet`, `go test ./... -count=1` green (12 packages
  ok), node repro_gear.js OK (68/68), repro_hud.js /
  repro_map_render.js / repro_movement.js ALL PASS.

## Round 30: the C1 paperdoll order, the debuff weight gauge, the confirmation-paced swaps and the walk plan view (2026-09-07)

The review of the interactive UI: the slot layout of the classic
client doll, the weight bar tied to the actual server debuffs, the
swap pacing by server confirmation instead of a fixed pause, and the
manual walks visible on the map.

- The wear grid of the equipment widget reorders to the C1 client
  doll: shirt, head, cloak / weapon, chest, shield / gloves, legs,
  boots (WEAR_SLOTS in app.js - the labels and the mask placement
  move together, the masks/aliases/either-or logic is untouched).
  The jewelry block flips: the blank hole sits middle-left, the
  necklace occupies the middle-right cell (JEWEL_SLOTS null/neck
  swap).
- The weight line recolors by the server weight penalty thresholds,
  verified in the Mobius C1 source (Player.refreshOverloaded: the
  load per mille switches the penalty at 500/666/800/1000 - 50%,
  66.6%, 80% and 100%, each level slows the speed to
  x0.90/x0.87/x0.84/x0.81 and the last marks the overload). Below
  the first threshold the bar keeps the green gradient; past it the
  fill melts from yellow through orange into red - a JS hue
  interpolation between the level anchors (weightFillStyle), set as
  the inline background over the gradient, the percent text takes
  the same color, and the row tooltip carries the raw numbers plus
  the active debuff level with its speed modifier. The fixed
  .load-fill.warn/.heavy classes are gone.
- The inventory command gate paces by the server confirmation
  instead of the fixed one second: the Mobius UseItem flood
  protector is disabled in this build
  (FloodProtectorUseItemInterval = 0, retail matching), so the only
  real hazard is the packet executor thread pool racing a same
  burst unequip+equip pair. markInventoryAction now records the
  item state as it left (equipped flag, stack count), the gate
  holds the next inventory command until the tracker observed the
  change (or the item vanished) - InventoryItemState on the state
  bot - with a 600 ms fallback timeout so a refused request never
  blocks the queue. A live swap pair (Squire's Sword into the
  occupied Dagger slot) lands in 334 ms, both useItem requests in
  the same second, the pair order intact.
- A manual command replaces a running walk instantly: userMovement
  arms userRedirect, the next tick re-issues the walk request at
  once even while the old server walk is still flagged moving (the
  server replaces the destination of a running walk with the next
  MoveToLocation) instead of waiting for the old destination; the
  flag clears when the new walk request fires. Live: the redirect
  click logs "replaces the manual move" and "walking to <new>" in
  the same second as the POST, the published plan switches to the
  new target.
- The manual walks become visible: the loop publishes the walk
  plan into the tracker (state.Bot.SetWalkPlan - the remaining
  waypoints with the clicked destination last, refreshed every
  tick while phaseUser runs, a no-op republish only refreshes the
  2 s lifetime so the event stream never churns, an empty or stale
  plan clears/expiring on its own so a crashed loop leaves no
  stale line). The snapshot carries it as walkPath; the map draws
  the plan while the paths toggle is on - a blue dashed polyline
  from the character through the remaining waypoints (the same
  light blue #4da3ff as the command feedback) - and the
  destination marker drawn from the plan: a solid blue dot with a
  pulsing breathing ring (shared drawUserMarker with the click
  ripple, so the click answer never changes style mid walk). The
  self character now also draws the same dashed destination line
  as every other moving object while it runs (it was the only
  unit without one). The animation loop keeps pulsing while a
  walk plan exists.
- The trash bin moves from the left of the footer to the far
  right end, after the adena and weight numbers (the footer
  columns take the width, the bin hugs the right edge; the drag
  destroy flow is unchanged).
- The drop count dialog gains the scroll wheel: a wheel notch
  over the open dialog steps the count by one, clamped into the
  stack (bumpDropCount shares the resolveDropCount clamping), the
  listener stays non-passive so the page does not scroll; a small
  hint line under the note documents the wheel, Enter and Escape.
- Tests: user_test.go replaces the fixed spacing test with
  TestUserSwapWaitsForServerConfirmation (the deferred command
  releases on the observed inventory flip) and
  TestUserSwapFallbackTimeout, adds the redirect test
  (TestUserMoveRedirectsARunningWalk: the running walk re-issues
  at once, an uninterrupted walk does not) and the walk plan test
  (TestUserWalkPlanPublishesAndClears: the direct plan, the
  planned waypoints with the destination last, the shrink on
  passed waypoints, the clear on arrival); bot_test.go adds the
  walk plan publish/clear/expire/reset and the InventoryItemState
  tests; repro_gear.js pins the new slot order, the hole/neck
  flip, the debuff melt (unit probes of weightFillStyle and
  weightPenalty plus the rendered pen classes and tooltips), the
  footer order, and the wheel stepping (79 checks; the stub DOM
  now answers querySelector so initGearInteractions wires the
  real dialog listeners).
- Live verification on the running stack
  (scripts/round10_smoke.sh + scripts/round10_swap_smoke.sh, the
  bot started without -hunt): the wear rows read shirt/head/cloak
  and gloves/legs/boots with the hole left of the neck; the trash
  bin sits 10 px from the right footer edge after the numbers;
  the live 21% load stays green with "no weight debuff" in the
  tooltip while the unit probes return hsl(50/26/8/0) and penalty
  levels null..4; the far double click publishes the walk plan
  (the last point equals the target, the server leg runs) and the
  screenshot shows the blue dashed path with the pulsing blue dot
  marker; the redirect POST replaces the plan and the walk within
  the same second and the walk arrives ("manual walk arrived
  (path done)"); the equip swap lands in 334/373 ms; the wheel
  steps 1 -> 2 -> 1 and clamps at the stack size; the trash
  destroy takes 5 adena (935 -> 930) with the ground unchanged; 0
  JS errors; screenshots scripts/round10_live.png and
  scripts/round10_dialog.png.
- `go build`, `go vet`, `go test ./... -count=1` green (12 packages
  ok), node repro_gear.js OK (79/79), repro_hud.js /
  repro_map_render.js / repro_movement.js ALL PASS.


## Round 31: the send and layer pool data races, the race task and the agent skills (2026-09-08)

Scope: the two data races identified by the architecture review
(`docs/quality_review_and_agent_prompts.md` P01) plus the toolchain
remainders of the golangci-lint v2 migration (P03).

### Problem statement

1. `connection.GameClient.sendPacket` encrypted the payload with the
   stateful rolling XOR cipher BEFORE taking `writeMu`: the run loop
   (ping, Appearing, Logout) and the hunt loop (client actions) call it
   concurrently, so two packets could be encrypted in one order and
   written in the other. The game cipher advances its rolling offset
   per packet, so any encryption/write order mismatch desyncs the chain
   and the server decrypts garbage from the affected frame on.
2. `pathfind` parsed regions under the engine mutex while the shared
   `layerPool.intern` mutated the pool, and concurrent searches read
   `layerPool.get` without any lock - a data race whenever one
   goroutine's FindPath triggered a region load while another searched
   a loaded region.

### Reproduction

- Race 1: `TestGameClientConcurrentSendKeepsCipherOrder`
  (`connection/send_order_test.go`) - 8 goroutines x 50 `sendPacket`
  calls against a raw socket that mirrors the session cipher and
  verifies every decrypted first opcode byte (RequestNetPing, 0xA8).
  Any encryption/write order violation corrupts the affected and all
  subsequent frames, so the test fails without the race detector too.
- Race 2: `TestEngineConcurrentSearchesRaceFree`
  (`pathfind/concurrency_test.go`) - 4 goroutines search across a 2x2
  synthetic region grid with the cache capped at 2 regions, so every
  search re-parses two regions (intern) while the others read (get).
- The detector itself needs cgo with gcc, which the Windows dev host
  lacks; `task test:race` (`CGO_ENABLED=1 go test ./... -race -count=1`)
  runs the suite where cgo exists (the Linux sandbox, CI). The plain
  `task test` stays race free.

### Fix

- `sendPacket`: the `crypt.Encrypt` call moved inside the `writeMu`
  critical section - serialize, encrypt and write now share one lock
  hold, which guarantees the encryption order equals the wire order.
- `layerPool`: own `sync.RWMutex`; `intern` writes under the write
  lock, `get` reads under the read lock (one slice index, contention
  negligible).

### Follow-up tooling of the same round

- Dead code deleted: the unused big endian login framing stack
  (`crypt` Encryptor/Decryptor/Checksum + their tests and benches;
  the live `ChecksumLE` stayed in `login_crypt.go`, which absorbed the
  `Serializable` interface definition).
- `task` 3.53.1 installed (`go install
  github.com/go-task/task/v3/cmd/task@latest`), `task test:race` added
  to the Taskfile, AGENTS.md documents the cgo/gcc caveat.
- Agent skills added under `.agents/skills/` (go-verify-loop,
  webui-harness, packet-recipe, mobius-stack) - see the new "Agent
  skills" section of AGENTS.md.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` clean;
  `golangci-lint run` 0 issues; `go test ./... -count=1` all 13
  packages ok (the two new tests included). The `-race` suite runs in
  the cgo environments (`task test:race`).

## Round 32: the forced destruction of the replaced Squire's starter kit (2026-09-08)

Scope: the user request to stop hauling the dead weight of the
starter set once a replacement is worn - the Squire's pieces are
neither sellable to a shop nor droppable on the ground, so the
character would carry them forever.

### Problem statement

The starter kit (Squire's Shirt 1146, Squire's Pants 1147,
Squire's Sword 2369) weighs 3301 + 1750 + 1600 = 6651 units the
character can never shed: the Mobius item xml flags them
`is_sellable=false` and `is_dropable=false`, so no shop buys them,
no buylist prices them, the ground refuses them, and the trash bin
(destroy) was never automated. Once a real replacement is worn the
pieces are pure encumbrance - 6651 units against the ~50% weight
penalty threshold of a low level character.

### Fix

- `gear.ReplacedStarterItems` (new `internal/swarm/gear/starters.go`)
  lists every unequipped starter piece whose paperdoll slot already
  holds an equal or better scored item - the equip planner swaps
  only on a strictly better candidate, so a listed piece will never
  be worn again. An empty slot keeps its starter piece (the auto
  equipment still wears it); the legs special case: a one-piece
  chest armor displaces the starter pants with itself. The order is
  deterministic (object ids ascending).
- The hunt loop (`maybeDestroyReplacedStarters`) destroys the listed
  pieces through the destroy request behind the shared confirmation
  gate: the equip always lands first, the destroy fires only once
  the tracker shows the replacement worn, one request at a time in
  the equip action budget, a failed request retries after a 10 s
  delay instead of re-logging every pacing period.
- The detection is datapack-verified: the official item xml carries
  the flags and weights above and `is_destroyable` defaults true in
  `ItemTemplate`, so the destroy request is the one open exit and
  the bot takes it.

### Verification

- `go build ./...`, `go vet ./...` clean; `go test ./... -count=1`
  all packages ok.
- New tests: `gear/starters_test.go` (the whole-kit listing with
  weights and reasons, the empty-slot survival, the better-kit-only
  guard, the one-piece chest displacing the pants, the deterministic
  order) and `hunt/starters_test.go` (the destroy lands behind the
  gate and never repeats after the vanish, the equip-first ordering,
  the retry pacing of refused requests).

## Round 33: official GitLab only - the server source of truth, the mirror ban and the archive channel (2026-09-08)

Scope: the server stack provenance. The user directive of 2026-09-08
made the official GitLab repository the only acceptable source of the
Mobius C1 server code; every outdated copy is forbidden.

### Problem statement

The sandbox server checkout was a GitHub mirror clone
(`tichopad/L2J_Mobius`, last commit 2026-06-20) - three months behind
the official `MobiusDevelopment/L2J_Mobius` master (activity through
2026-08-29+). The drift was measurable: the mirror SQL produced a
74 table schema where the official SQL produces 75, and the git
clone of the official tip compiles 1314 java files against the
mirror's 1318. The bot had started adapting to a stale reference.

### Fix

- The official source is pinned:
  `https://gitlab.com/MobiusDevelopment/L2J_Mobius` (project id
  70889258), branch `master`, module `L2J_Mobius_C1_HarbingersOfWar`,
  commit `43ac8878` (2026-08-29).
- GitLab answers the git upload-pack endpoints with intermittent
  Cloudflare 403s (the clone passes, the promisor blob batch fetch
  fails). The official REST API stays stable, so
  `tools/swarm_fast_deploy.sh` now walks two official channels: the
  sparse git clone with retries, then the repository archive API
  (`/repository/archive.tar.gz?path=L2J_Mobius_C1_HarbingersOfWar`,
  one GET, same repo, same commit). The mirror fallback path that
  produced the outdated checkout is gone.
- The deployed checkout is a real git repository grafted onto the
  archive download: refs fetched blob-less from GitLab, the working
  tree blobs rehydrated from the byte identical archive files, the
  one divergent blob (GeoEngine.ini, PathFinding 0 deployment patch)
  rehydrated through the blob raw API with sha1 verification.
  `git fetch`/`git pull` work against the official remote again.
- The database was reloaded from the official SQL (75 tables now).
- AGENTS.md records the rule in "Server integrity rules": official
  GitLab only, mirrors and outdated copies are forbidden.

### Verification

- The starter kit assumptions of round 32 hold on the official
  datapack: items 1146/1147/2369 are `is_sellable=false` and
  `is_dropable=false` with weights 3301/1750/1600, and
  `is_destroyable` defaults true in `ItemTemplate` - the destroy
  request is the only exit and it is open, exactly as
  `gear.ReplacedStarterItems` expects.
- Full redeploy from the official code: 1314 files compiled,
  `STACK_READY: login :2106, game :7777, db :3306`, and
  `tools/mobius_e2e.sh 45` prints `E2E_OK` against the fresh
  official database.

## Round 34: the fight FX variant showcase page of -test-fight-ui-v1 (2026-09-08)

Scope: the user asked for a dedicated test command that demos how the
combat damage and the critical hits could be visualized, so the live
map style can be picked from working examples instead of words.

### Problem statement

The live map combat layer (round 30) renders one chosen style of the
swings and the damage numbers, but the visual language of "the hero
dealt damage / received damage / landed or took a critical" was never
compared against alternatives. Judging the ideas needs them running
side by side, in every attack direction, on the real map background.

### Fix

- The new `-test-fight-ui-v1` flag boots a bot less web mode
  (`webserver.NewFightServer`, mode `fight` in `/api/config`): no
  game connection, the showcase runs entirely in the browser.
- `web/fight.js` builds the showcase: twelve numbered variant
  columns in a horizontally scrolling strip - floating numbers (the
  current live style), comic pop with the cell shake, the slash
  streak and the crit X cross, the spark spray, the shockwave rings,
  the falling HP bar chunk, the projectile arrow with the crit flame
  trail, the cinematic hit-stop zoom punch, the full cell flash
  vignette, the dizzy orbit stars, the arcade banner and the combo
  counter with the count up numbers.
- Every column stacks four demo cells - the enemy above, below,
  left and right of the hero - so each idea is judged from every
  attack direction. All 48 cells share one clock and one scripted
  loop (the hero hits 38, takes 26, crits 95, takes a 68 crit, the
  health refills, restart), so the same beat plays in every variant
  at the same moment and the columns compare directly.
- The cell background is the real map tile the bot hunts on (the
  elven lands crop), the fighters render in the live map unit
  language (the blue hero circle with the pulse ring, the red enemy
  with the combat ring, the health bars and the names) and every
  damage event carries a caption ("hero hits -38", "CRIT deals
  -95").
- The renderers are stateless functions of the effect age, so the
  showcase holds no event bookkeeping and cannot desync; the frame
  loop throttles to ~30 fps for 48 canvases.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` clean; `go test
  ./... -count=1` all packages ok; `node --check` on the touched
  JS files; the four repro harnesses keep their state (repro_map_
  render holds its pre-existing zone label failure).
- New `webserver/fight_test.go`: the mode handshake answers
  `fight`, the index page carries the strip container, the fight
  script and the map tile of the background answer.
- Live run: `swarm -test-fight-ui-v1 -web 127.0.0.1:8090`, the
  headless browser loads the page with zero console errors, the
  strip holds 12 columns / 48 cells / 3664 px of scroll width, and
  the captured screenshots (download/fight_shots/) show the numbered
  columns, the map background, the hero/enemy markers, the HP bars
  and the variant effects rendering correctly.

## Round 35: the elven village navigation - approach radius, water cost, server step rules (2026-09-09)

Scope: the user reported the bot stuck at x 45544 y 45880 z -2992
while trying to reach the trader Unoren (44667 46896 -2982) of the
floating elven village, and earlier sessions swam through the lake
under the village instead of crossing a bridge. The task demanded the
full diagnosis (which Z coordinates reach the pathfinder, whether it
runs at all, what it returns) and a fix that makes the pathfinding
correct on such multilayer terrain.

### Diagnosis (measured on the deployed pack, live verified)

- The town trips DO use the pathfinder: `startWalkLeg` calls
  `Navigator.FindPathTo(from, dest, int16(dest.Z))` - the start z is
  the live character z, the target z the merchant spawn z.
- The Unoren shop cell holds layers -2632 (a raised surface) and
  -3928 (the lake floor) but NO deck layer -2984: the C1 l2j geodata
  does not model the shop interiors (the NPC spawn z -2982 is the
  real floor). The strict search resolved the target against the
  merchant z, picked the raised surface and aborted at the 1M
  expansion cap hunting an unreachable layer.
- The plain fallback resolved the target against the START z, which
  picked the lake floor under the shop; water costs the same as land
  and the swim (5683 units) is shorter than the bridge route
  (~6300), so the planned waypoints led under the village - the
  observed swimming. The approachMerchant direct far walks then
  depended on the server's own routing, which stalls at the pond
  edge north of the shop (the straight line to the merchant crosses
  the water gap; the user's stuck position 45544 45880 is exactly
  that deck edge).
- The three village bridges are wide gentle ramps (8..16 unit steps,
  the fields -3488 up to the deck -2984, fully connected in the
  geodata); the earlier "the geodata pack disconnects the Elven
  village decks" conclusion was a misreading of the missing shop
  floor layers plus the water preference.
- The Mobius movement validation (GeoEngine.getValidLocation) gates
  upward steps at HEIGHT_INCREASE_LIMIT 40, accepts any drop, and the
  water surface sits at -3780 (the maxZ of the water.xml cuboids).

### Fix

- The A* step rules now mirror the server (canStep): walls of the
  source cell, upward <= 40, drops walkable; the line of sight keeps
  the strict symmetric rule so the smoothing never collapses a
  detour into a straight drop. DefaultMaxPassableHeight 30 -> 40.
- Water cost: layers below -3780 cost 3x per step
  (waterCostMultiplier), so bridges and shores beat swimming
  whenever they exist.
- FindPathTo is replaced by FindPathApproach(start, end, radius): the
  search succeeds on the first node within the 3D radius of the
  target point - the z difference counts, so the water deck below a
  shop never satisfies the radius while the deck ring around the
  merchant (the counter front, within the 250 unit interaction
  distance) does. The town trips use radius 200, the manual long
  walks 150; a fully reachable target is still reached exactly
  (the exact node pops first).
- The plain FindPath resolves the target layer against the target z
  like the server's own pathfinder (getHeight(tx, ty, tz)).

### Verification

- Regression tests over the real pack: from the lake shore farm
  (47320 42216 -3488) and from the user's stuck spot the approach
  search ends on the village deck (z -2992, 189 units from Unoren,
  zero water waypoints); the bridge route runs through the ramp
  entrance at (45912, 42776) in both cases.
- Synthetic tests pin the water preference (a bridge beats a shorter
  swim; water alone still routes), the approach radius (a sealed
  counter is reached at its front; an open target is reached
  exactly), the any-height drop walkability and the 40 unit climb
  gate.
- Live E2E on the deployed stack (PathFinding=2): a fresh bot walked
  spawn -> fields -> bridge ramp -> village deck -> 59 units from
  Unoren in 45 s (search 0.46 s, 4 waypoints, z never below -3440),
  then selected the merchant (MyTargetSelected, target id set, the
  character at 25 units). tools/mobius_e2e.sh 45 prints E2E_OK, the
  full suite and golangci-lint stay green.

## Round 36: the net ping answer log spam of the attached client (2026-09-09)

Scope: the user reported the process log spamming "Net ping with game
time" messages many times per second whenever a real C1 client
connects to the game through the proxy. The task: identify the source
and, if it is just a debug line, remove it.

### Diagnosis

- The line lived in `connection/game_dispatch.go` `handleNetPing`:
  every NetPing (0xEC) answer of the game server was logged with the
  game time it carries, and nothing consumed that value (no pong
  tracking, no keepalive decision - the quality review P02 already
  lists pong tracking as a future improvement).
- The rate is the client's own ping rate, not a bug: the C1 client
  sends RequestNetPing (0xA8) continuously (its connection monitor).
  Behind the proxy each request transits through the bot session
  (`transitToServer` -> `SendRaw`), so the server answers at the
  client's pace - several replies per second observed live, one log
  line each, all into `log.Default` (the process log the user
  watches).
- The answers are healthy otherwise: each one is tapped to the
  recorder and relayed back to the client, the connection stays
  alive. Only the unconditional logging was the problem.
- The proxy's own per-packet relay line ("game#N: client -> server
  0xa8") is untouched: it belongs to the separate `proxy.log`, whose
  documented purpose is exactly that per-packet trace
  (docs/proxy.md, "Debugging the client connection").

### Fix

- `handleNetPing` stays silent on the happy path. The packet is
  still parsed, so a malformed one (a cipher or protocol desync)
  still logs "Failed to parse net ping".
- Regression coverage: `TestNetPingAnswersStaySilent` floods a fake
  server session with 100 valid replies plus one truncated - the
  spam line must stay absent while the parse failure must log. The
  live proxy E2E gained the keepalive leg: the fake client sends a
  burst of 10 RequestNetPing, all 10 answers must return through the
  live relay, and the final log assertions demand "client -> server
  0xa8" present in proxy.log while "Net ping with game time" stays
  absent.

### Verification

- go build/vet, go test ./... (16 packages), gofmt clean,
  golangci-lint 0 issues.
- The live proxy E2E PASS against the running stack (login 2106,
  game 7777) with the new ping burst leg: 10 requests sent, 10
  answers relayed back in 1.2 s, the session log silent about them.

## Round 37: the deferred pile up logout and the two second relogin (2026-09-09)

Scope: the user called the emergency logout of the hunt loop too
blunt. A character with two or more aggro mobs on it logged out on
the spot, and the relogin waited half a minute. The requested
behavior: run at least 600 units away from the point the aggro
happened on before logging out (the mobs stay behind and walk home
while the character is offline, so the relogin lands outside their
aggro range), and cut the relogin pause to two seconds - the aggro
resets on the disappearance on this stack, a long pause only idles
the farm.

### Problem statement

- The attacker-count branch of the emergency logout
  (`hunt/loop.go` tick, `panicLogoutAttackers = 2`) called
  `emergencyLogout` at once. The server stores the character where it
  stood - in the middle of the pack - and the relogin (even after the
  30 s pause) dropped the character right back into the same aggro.
- `panicLogoutPause = 30 * time.Second` idled the farm: the pause was
  sized to cover the fifteen second combat stance the server holds an
  offline body in plus the mob walk home, but the aggro itself resets
  the moment the character leaves the world - observed live, a two
  second relogin lands clean.

### Fix

- `hunt/loop_safety.go` gained `panicPileUpRun`: the attacker-count
  branch now starts a run instead of the instant logout. The first
  call anchors the aggro point (`panicX`/`panicY`/`panicAt` on the
  Loop), drops the current fight (the target lands on the long skip
  list, the engage bookkeeping clears) and walks the paced escape
  legs away from the threats - the same legs, pacing (one leg per
  second) and zone clamping the hurt flee uses. The logout fires once
  `panicAnchorDistance` reports `panicRunDistance` (600) units or
  more between the character and the anchor.
- The run is committed once armed: the panic block of `tick` re-enters
  on the armed anchor (`!l.panicAt.IsZero()`), not on the live mob
  count, so a pack that thins out mid-run cannot turn the run back
  into a lost fight - the distance, not the count, ends it.
- Two bounded exits: the run shares the `fleeLogoutAfter` (20 s)
  budget, so a cornered run (the legs never open the distance) logs
  out wherever it got to; and a run whose pack dissolved on the way
  (no mob holds the target anymore, nothing attackable within the
  escape range - `escapeWalkDestination` comes back empty) logs out
  at once instead of idling out the budget.
- The critical-health branch (HP under 12% with the blows landing)
  stays instant: one hit from death, the run has nothing left to
  protect.
- `panicLogoutPause` 30 s -> 2 s. The supervisor
  (`cmd/swarm/main.go` `runBotForever`) honors the cooldown only on
  top of its 2 s minimum reconnect delay and retries a failed login
  with an exponential backoff, so a too-early reconnect (the server
  still holding the combat stance body) costs one retry, nothing
  more.

### Verification

- New unit tests: `TestLoopRunsFromThePileUpBeforeLoggingOut` (the
  pile up arms the anchor, walks the first leg away from the pack, no
  logout on the spot, the one-leg-per-second pacing holds; after the
  character covers the leg past the 600 unit mark the logout fires
  with one last leg and the 2 s cooldown; the request stays one
  shot), `TestLoopLogsOutWhenThePileUpRunNeverMakesDistance` (the
  cornered budget ends the session).
- The cooldown assertions of `TestLoopLogsOutAtCriticalHealthUnderAttack`
  and `TestLoopLogsOutWhenTheFleeNeverShakesTheChase` re-pinned from
  the half-minute to the 2 s pause.
- go build/vet, go test ./... (16 packages), gofmt clean,
  golangci-lint 0 issues.
- The live proxy E2E PASS against the running stack (login 2106,
  game 7777, 1.2 s): the connection path is untouched, the smoke run
  confirms the deploy.

### Follow ups

- Watch the live log for the new lines: "N mobs piled on us, running
  600 units from the aggro point before the logout", the distance
  report at the logout, and the reconnect after two seconds.

## Round 38: the zone switch sit freeze, the mid air search goal and the dump state button (2026-09-09)

Scope: two live problems of the manual hunting zone switch reported
from the deployed bot plus a web UI feature for the live bug reports:
a zone switch over a sitting bot spams packet errors, and the bot
does not pathfind on a zone switch - it got stuck at
x 42536 y 48648 z -2992 on the flying elven city heading to the
Spore Fungus SW-d1 zone. Plus: a Dump state button that copies the
full debug state to the clipboard.

### Diagnosis

- Mobius C1 move-vs-sit: `PlayerAI.setIntentionMoveTo` answers
  ActionFailed while the AI intention is REST; `sitDown`/`standUp`
  drop toggles inside the 2.5 s `_sittingInProgress` window;
  `StandUpTask` clears the paralysis and the REST intention 2.5 s
  AFTER the ChangeWaitType(WT_STANDING) broadcast. The bot's
  `returnToZone` had no stand-up gate at all (only the town trip
  start and the flee used `standUpGuarded`): a zone switch over a
  resting character walked into the refusals every 2 s (the
  "Action failed" packet error spam the user saw) and nothing
  outside the zone ever stood the character up - the bot sat
  outside its zone forever.
- The zone return search goal: `returnToZone` built
  `Vec3{zone.CX, zone.CY, selfZ}` and searched with the 200 3D
  approach radius. The walker height at the zone center is a
  fabricated z: the city deck (-2992) against the spore zone ground
  (-3664) put the goal 600+ units mid air, no cell could satisfy the
  radius, the A* burned the full 1M expansion cap - 12-14 s of a
  FROZEN hunt tick per attempt (reproduced on the real geodata
  pack), then `startWalkLeg` fell back to the single direct waypoint
  and the character walked the straight line into the west city
  railing - the exact reported stuck coordinates (the deck ends
  there, x 42300 is already lake bottom).

### Fix

- `standUpGuarded` (hunt/town.go) gained `standSettlePeriod` (3 s):
  after the stand broadcast confirms, the movement waits out the
  server side stand animation - the first walk request of the return
  lands on a movable character. `returnToZone` (hunt/
  loop_movement.go) gates on the guard before any walk planning: the
  zone switch over a resting bot waits out an in-flight sit
  transition, stands up, settles, then plans - exactly the requested
  "wait for the sit to finish, stand up, continue".
- `pathfind.Engine.ClosestHeight(x, y, refZ)` (new): the layer
  height at a world position closest to refZ - the deck the server
  itself resolves a destination to. The hunt `Navigator` interface
  carries it; `zoneReturnDestination` resolves the real zone center
  deck before the approach search, a lookup failure keeps the self
  height. The city->spore route plans in ~0.5 s (the waypoints drop
  off the city edge onto the west lake shore - the geodata walk the
  server itself accepts, no fall damage exists in C1).
- The dump state button: `GET /api/bots/{id}/dump` (webserver/
  dump.go) assembles the plain text report - the character sheet
  (position, vitals, sit state, stats, load), the aggro load, the
  hunting zone, the equipment with paperdoll slot names, the bag,
  the objects sorted by distance with combat state and targets, the
  walk plan, the combat beats, the chat, and a 600 entry event
  window (`state.Bot.NewestEvents`, the snapshot streams 100 for
  the log tail, the ring caps at 512). The HUD name row gained the
  copy button (navigator.clipboard, the legacy textarea fallback, a
  new tab escape for refusing clipboards, a copied/failed flash).
- The hunt loop logger mirrors its decision lines into the tracker
  event log (`Loop.SetLogger`, the MultiWriter wiring in
  cmd/swarm/main.go, the console format untouched): the web UI log
  tab and the dump carry the reasoning of the loop (zone switches,
  escapes, stuck re-paths) next to the raw game events.

### Verification

- New tests: TestLoopStandsUpBeforeTheZoneReturnWalk (no walk, no
  search while sitting; the stand request; the plan and the walk
  after the settle), TestLoopResolvesTheZoneReturnDeckHeight (the
  goal carries the resolved deck height),
  TestLoopKeepsSelfHeightWhenTheZoneDeckLookupFails,
  TestTripStandsUpBeforeWalking (re-pinned to the settle window),
  TestClosestHeight (the multilayer resolution on a synthetic
  world), TestFindPathFromCityDeckToGroundZone (the LIVE stuck case
  against the real geodata: the route exists, plans under 5 s, ends
  within the approach radius on dry ground),
  TestFindPathFailsOnAFabricatedGoalHeight (the mid air goal starves
  the search), TestBotDumpEndpoint, TestBuildStateDumpEventWindow,
  TestHuntEventLoggerMirrorsHuntLines,
  TestHuntEventLoggerKeepsTheConsoleFormat. The flaky
  TestAppendSnapshotJSONEmptyBot compares without the serverTimeMs
  clock race now.
- go build/vet, go test ./... (18 packages), gofmt clean,
  golangci-lint 0 new issues (6 pre-existing on the clean HEAD - the
  v2.6.2 build of this sandbox is stricter than the repo's last
  run).
- Live: SWARM_PROXY_E2E=1 E2E PASS (0.6 s), tools/mobius_e2e.sh 45
  E2E_OK, and a manual live run against the stack verified the dump
  endpoint end to end (the character sheet, the zone, the equipment
  with slot names, the sorted objects, the events with the mirrored
  hunt lines).

### Follow ups

- The next live zone switch on a resting bot should log the stand
  up, the ~3 s settle, then "outside the hunting zone,
  pathfinding back" with a fast plan - and the dump button should
  hand the user the full story for any new report.

## Round 39: the terrace rule - the pathfinder stays on walkable surfaces (2026-09-09)

Scope: the follow-up live report of the zone switch (a state dump):
the bot on the floating elven city deck (42440 49032 z -2992) heading
to the Spore Fungus SW-e1 zone ground into the city walls with "town
walk stuck, re-pathing" repeating, the remaining walk plan pointing
straight west on the ground layer. The user's diagnosis: the
pathfinding has no descent - the railings block the city edge, the
character must use one of the three bridges with their gradual ramps.
Explicitly demanded: a general multi-terrace solution, no hardcoded
bridge routes.

### Diagnosis

- Replanning the dump route on the real geodata showed the A* DID
  find a route: a 920 unit drop off the deck edge into the lake bed
  at (42360, 49048) and a ~10000 unit swim west. The geodata does not
  model the railings (the deck edge cells are fully open NSWE) and
  the search's canStep mirrored only the upward Mobius
  HEIGHT_INCREASE_LIMIT - any drop was walkable, so the jump off the
  deck planned cleanly.
- The follower made it worse: its 2D waypoint arrival consumed the
  drop waypoint (80 units away horizontally, 920 below), so the
  character skipped straight to the long west leg, walked it into the
  city building walls, stuck, re-path, the same drop route, forever.
- The height profiles of the real geodata separate the world cleanly:
  the bridge ramps and the lake shores step 8..24 units per cell, the
  deck edge jumps 920. Stacked terraces connect only through gradual
  ramps.

### Changes

- `search.canStep` is symmetric now: a step between neighbouring
  cells is walkable only when the height difference stays within the
  passable height in BOTH directions; a bigger step is a terrace
  boundary. The descent from the city deck routes through the ramps
  with nothing hardcoded - the A* finds the south bridge itself
  (deck -> 42824 51224 -> -3224 -> -3480 -> -3680, 14702 units, ~0.6
  s, 46k expansions, no swimming). `canMoveTo` of the line of sight
  raster delegates to the same rule, and the FindPath docs state the
  terrace contract.
- `waypointDistance` (hunt/town.go): the town trip and the manual
  walk followers measure the waypoint arrival in full 3D - a drop
  waypoint hundreds of units below is never consumed as reached.
- `directOrAstar` (search.go): run()'s straight line shortcut answers
  only DRY walks. The old form returned any walkable raster line,
  including a lake ford, which bypassed the water cost while the
  cheaper bridge sat unplanned - the synthetic channel route proved
  it by swimming straight across. A wet, walled or truncated line
  defers to the cost aware A*; the accepted dry line is its own
  smoothing (the two endpoints), keeping the region crossing
  concurrent searches instant (the full raster would have fed the
  quadratic smoothing cascade - 269 s on the concurrency test).

### Tests

- New: TestFindPathTerraceBoundaryNeedsARamp (a 500 unit synthetic
  terrace stays sealed without a ramp and connects through one, every
  raw step within the limit, the climb back works);
  TestFindPathFromDeckToFarWestZoneStaysOnRamps (the live dump
  coordinates, the ramp invariant, no swimming, under 5 s).
- Updated: TestFindPathFromCityDeckToGroundZone asserts the ramp
  steps; the water channel fixture slopes both shores (the real lake
  shores do) and sits closer to the bridge (the old margins flipped
  inside the diagonal shortcut noise); TestLoopPathfindsBackIntoTheZone
  pins the 3D arrival against a realistic zone deck height.
- go build/vet, go test ./... (18 packages), gofmt, golangci-lint (no
  new issues), tools/mobius_e2e.sh 45 E2E_OK, SWARM_PROXY_E2E=1 PASS.

## Round 40: the bot relogin handoff - the client survives the session cycle (2026-09-09)

Scope: the second half of the proxy contract. The live self state
(round 4) answered "where is the character right now" for a client
that reconnects on its own; this round answers the mirror question:
what happens to a client that STAYS connected while the bot cycles
its session. The hunt loop logs the character out when the situation
turns hopeless (the emergency logout of the pile up escape) and the
supervisor logs it back in seconds later - the user did nothing and
must not be kicked to the login screen for a decision the bot made.

### Design

The relay of one client connection now streams a chain of bot
sessions instead of exactly one (runRelay -> streamSession ->
serveRelogin):

- **The LeaveWorld suppression.** The `LeaveWorld` the real server
  answers to the bot's logout is dropped from the live feed and from
  the recorded history replay: a C1 client that processes it drops
  itself to the login screen, which would break the hold. The
  suppression is conditional on the logout NOT being the client's
  own: the classic user logout keeps the relayed answer and the
  session end closes the connection (the login screen is what the
  user asked for).
- **The hold.** The recorder close (the authority on the session end)
  parks the relay instead of closing the client: it snapshots the old
  known list (the tracker is cleared by the next login, so the sweep
  must run against what the client actually saw - the new
  `Bot.KnownObjectIDs`), then polls `Server.sessionByID` for a
  replacement session of the same bot id (a fresh recorder, a fresh
  send path, `StatusOnline`). The client packets in between are
  swallowed: the character is offline and the world behind the client
  is frozen, so every action is meaningless - except the `Logout`
  itself, which the proxy answers with a synthesized `LeaveWorld`
  (the server cannot answer, the character is gone). The hold
  releases the client after 2 minutes of a missing bot (a frozen
  world beats a login screen the user did not ask for, but not
  forever).
- **The resync.** The replacement session resyncs the client view: a
  synthesized `TeleportToLocation` of the played character to its
  live position (self object id, live x/y/z, the heading - the client
  also clears its own known list on the teleport, but the sweep does
  not rely on it), a `DeleteObject` for every object id of the old
  known list (a delete of an unknown id is a no-op on the client, so
  the sweep is safe either way), the enter world burst of the new
  session replayed through the ordinary replay path (the live self
  state patch of round 4 applies - the UserInfo, the inventory, the
  new known list), and the session reference of the connection swaps
  so the client packets transit to the live bot link again. The
  result is exactly the view a fresh client would get, minus the
  login screens the held client never sees. A replacement session
  without a recorded `CharSelected` (the defensive path) keeps the
  teleport resync and the live feed alone.

### Engineering details

- The session swap is mutex-guarded on the connection (`mu`,
  `currentSession`/`setSession`, `setHolding`): the read loop keeps
  reading and transiting client packets while the relay goroutine
  performs the handoff - the two goroutines no longer share the
  session field unsynchronized. The knob reads of the hold timing
  are guarded the same way (the tests rewrite them while a previous
  connection still unwinds).
- The sender got a bounded shutdown flush: the packets queued at the
  moment of the close (the synthesized `LeaveWorld` of the held
  logout rides exactly this path) are encrypted and written before
  the socket closes, and the sender itself closes the socket (the
  reader is released by the close, not by the done flag). A stalled
  write cannot hold the teardown hostage: shutdown waits one second
  and force closes.
- A transit failure no longer kills the client connection: it means
  the bot session link died under the transit - the narrow window
  before the relay noticed the recorder close and engaged the hold.
  The recorder close is the authority; killing the client here would
  break the hold it is about to start.

### Tests

- New: TestHandoffPacketBuilders (the byte layouts of the
  synthesized TeleportToLocation/DeleteObject/LeaveWorld against the
  Mobius C1 writeImpl bodies); TestGameServerHoldsClientThroughBotRelogin
  (the core scenario end to end: the recorded and the live LeaveWorld
  suppressed, the hold swallowing a client packet, the relogin, the
  teleport, the sweep of the old known npcs, the replayed UserInfo at
  the live place, the live feed of the new session, the client packet
  transiting through the replacement link, the second hold);
  TestGameServerUserLogoutReturnsToLoginScreen (the classic flow);
  TestGameServerUserLogoutWhileHeldClosesClient (the frozen world
  exit); TestGameServerHoldTimeoutReleasesClient (the dead bot path);
  TestGameServerReloginWithoutCharSelectedServesLiveFeedOnly (the
  defensive path).
- Updated: TestGameServerServesFullClientFlow - the session end now
  keeps the client open (the hold), unwound explicitly at the end.
- One stack side note, not a code defect: the live E2E failed at the
  bot login with `Session key incorrect` in game.log - the login
  server (which survived a sandbox restart) held a stale session key
  for the account. A login+game restart cleared it and both E2E
  suites went green again.
- go build/vet, go test ./... (18 packages), go test -race on the
  proxy package, gofmt clean, golangci-lint (2 pre-existing gosec on
  the HEAD, no new issues), tools/mobius_e2e.sh E2E_OK,
  SWARM_PROXY_E2E=1 PASS.

## Round 41: the build identity of the state dump (2026-09-09)

Scope: a live problem report must tell which code produced it. The
"Cannot see target" investigation opened with a state dump whose
origin had to be inferred from the user's `git pull` output - the
report itself said nothing about the branch or the commit it came
from, and a parallel-session push moving the branch in between would
have made any such inference a guess. This round pins the origin into
the artifacts themselves: the second line of every state dump and the
line right after the startup banner of every bot log now carry the
exact build identity.

### Design

- New package `internal/version` with four link-time fields (Branch,
  Commit, Dirty, BuildTime) that the build scripts fill via
  `-ldflags -X`. Two fallback levels cover the binaries nobody
  stamped: the `vcs.*` settings Go embeds into every binary built
  from a git repository (the revision, the modified flag, the commit
  time) and, for the branch - the one thing the VCS stamp does not
  carry - the `.git/HEAD` of the working directory (the
  "ref: refs/heads/<name>" form, the "gitdir: <path>" pointer of a
  linked worktree, empty on a detached HEAD, since a stale branch
  hint would be worse than none).
- The state dump prints the identity right under the title:
  `build: branch feature/proxy-server, commit 6a0c183 (dirty), built
  ...`. The dirty flag matters as much as the hash - a dump from a
  tree with uncommitted edits must not be mistaken for the clean
  commit it names.
- The identity renders unknown fields away instead of inventing them;
  a binary with no git metadata at all reports
  `unknown (built outside a git repository)`.

### Engineering details

- The three build sites bake the identity in: swarm_fast_deploy.sh
  and mobius_fast_deploy.sh (byte-identical, as the deploy rule
  requires) and mobius_e2e.sh - `git rev-parse --abbrev-ref HEAD`
  (emptied on a detached HEAD), `git rev-parse --short HEAD`, `git
  status --porcelain` for the dirty flag, `date -u` for the build
  time. Branch names and the timestamp carry no spaces, so the
  multi-word -ldflags value needs no quoting tricks, only the
  in-quote line continuation. The e2e log echoes the identity right
  after the build, before the stack even starts.
- The dump endpoint test asserts only the `build: ` prefix: the
  identity values of the test binary depend on the working tree of
  the moment (vcs.modified flips to true the second a fix is being
  written), pinning them would flake.
- gosec flags the gitdir pointer read (G703, path traversal via
  taint analysis) - nolint-ed with the justification that the pointer
  comes from the `.git` file of the worktree being inspected, a
  local build hint, not a user-supplied path.

### Tests

- New: the internal/version package tests - TestRenderIdentityLine
  (the line layout: comma separated fields, the commit tree state,
  the unknown fields dropping out), TestShortHash, TestWorktreeBranch
  (the ref form with slashed branch names, the detached HEAD, the
  gitdir pointer of a linked worktree, the missing .git),
  TestIdentityPrefersLinkTimeFields (the link-time values win over
  the embedded VCS stamp).
- Updated: TestBotDumpEndpoint - the dump header carries the build
  line.
- Live verification on the deployed stack: both build paths produced
  the identity in the dump of a bot in the world - the ldflags build
  (`build: branch feature/proxy-server, commit 6a0c183 (dirty),
  built 2026-09-09T21:24:29Z`, the build time of the link) and a
  plain `go build` (the same line with the commit time from the VCS
  stamp) - plus the matching `Build:` first line of the bot log in
  both runs; tools/mobius_e2e.sh E2E_OK.
- go build/vet, go test ./... (19 packages), gofmt clean,
  golangci-lint 0 issues on the touched packages.

## Round 42: the full commit hash and the file path build identity (2026-09-09)

Scope: the follow-up of round 41. Two gaps surfaced the moment the
identity line met a real run: the hash rendered in the short 7
character form (fine for `git show`, awkward to search in an editor,
a GitHub page or a log archive), and a `go run ./cmd/swarm/main.go` -
the exact form the Taskfile `run:app` task used - carries NO VCS
stamp at all (a file path build compiles the command-line-arguments
package, which Go does not stamp), so the dump showed the branch but
no commit. The user run mode must not degrade the identity.

### Design

- The commit renders in the full 40 character form everywhere: the
  scripts bake `git rev-parse HEAD` (not `--short`) and the VCS stamp
  passes through untrimmed (the shortHash helper is gone) - one line
  of a report is now searchable against any git interface as is.
- The .git fallback grew a commit half: HEAD resolves through the
  loose ref file (refs/heads/<name>), packed-refs, the gitdir pointer
  of a linked worktree and the common dir behind its commondir file;
  a detached HEAD carries the hash itself. A binary without any
  stamp still identifies both the branch AND the commit.
- `task run:app` now runs the package path form (`go run ./cmd/swarm`)
  so the ordinary local run keeps the full VCS stamp - the file path
  form stays supported through the .git fallback anyway.

### Engineering details

- All local git file reads go through one readGitEntry helper: the
  gosec taint analysis does not flag a read through a plain function
  parameter, so no nolint is needed at all (the round 41 nolint on
  the gitdir pointer read turned out to be covering a line gosec
  never flagged once the reads were restructured; the helper doc
  keeps the "never user input" rationale).
- The dirty flag and the build time stay stamp/ldflags only: they
  drop out of a file path build line rather than being guessed.

### Tests

- Updated: TestRenderIdentityLine (full hash fixtures),
  TestIdentityPrefersLinkTimeFields (full hash).
- New: TestReadWorktree - the loose ref, the packed-refs table, the
  detached HEAD, the gitdir pointer with a local ref, the gitdir
  pointer with the commondir indirection, the missing ref, the
  missing .git.
- Live verification on the deployed stack: the file path build (NO
  vcs stamp - confirmed with go version -m) rendered
  `build: branch feature/proxy-server, commit 675d2e5...e09fc` in
  the dump of a bot in the world; the ldflags build rendered the
  same hash with the dirty flag and the link timestamp;
  tools/mobius_e2e.sh E2E_OK (its log echoes the full hash too).
- go build/vet, go test ./... (18 packages ok), gofmt clean,
  golangci-lint 0 issues on the touched packages.

## Round 43: the stuck spot reproduction - re-verified and pinned (2026-09-09)

Scope: the user asked to return to the original stuck problem of round
35 (the character stuck at x 45544 y 45880 z -2992 while a town trip
tried to reach the trader Unoren 44667 46896 -2982), re-check the
state dump and everything around it once more, and create a
reproduction test with the same positions.

### The re-check (all measured again on the deployed pack)

- The geodata of the reported scene: the stuck cell holds the village
  deck (-2992) above the pond floor (-3840), the pond cells between
  the spot and the shop hold water floor only (-3880, below the -3780
  surface) and the Unoren cell holds a raised surface (-2632) and the
  lake floor (-3928) but no floor layer at the real merchant z.
- The straight line the old direct far walk sent is not walkable: the
  line of sight from the stuck spot to the merchant is false, and a
  cell by cell simulation of the server move validation
  (GeoEngine.getValidLocation semantics) stalls at the very first step
  - the reported coordinates 45544 45880 -2992 ARE the deck edge stall
  point of that line.
- The current approach search from the same spot: 3 waypoints, 1466
  units, 182 nodes explored, the route stays on the deck at z -2992
  around the pond and ends 189 units from Unoren (regression suite
  TestFindPathToShopDeck re-run green).
- The live reproduction on the deployed stack: the character test1
  was placed at the stuck position through the database with a bag of
  41 non stackable daggers (the 50% slot trigger; stackable junk
  merges into one slot on login and never triggers), the bot ran one
  hunt session - the trip started at the stuck spot, walked the
  geodata route (13 s), sold the daggers in two batches, bought
  nothing, walked back to the farm spot, hunting resumed. Zero stuck
  re-paths in the log. The state dump captured mid walk carried the
  build identity line, the live position with the active leg and the
  published walk plan (3 waypoints: the deck edge route, the approach
  ring, the merchant) - the same material the original report was
  made of, now showing a working plan instead of a stall.

### The reproduction tests (internal/swarm/hunt/town_repro_test.go)

- TestReproStraightWalkStallsAtTheUserStuckSpot pins the mechanism of
  the original problem: a server simulating walker (reproServer, a
  MoveToLocation follower that advances cell by cell with the geodata
  line of sight as the conservative model of the server's straight
  line validation) sent straight at the merchant stalls exactly at the
  reported coordinates, stays on the deck, never reaches the 250 unit
  interaction distance - a straight MoveToLocation is not a route, the
  contract the geodata planning exists to satisfy.
- TestReproTownTripFromTheUserStuckSpot walks the full trip from the
  same positions against the real pack: the trip targets Unoren (the
  nearest merchant of the spot), plans a real geodata route (never the
  single direct fallback leg), the follower walks it under the
  simulated server, the walk ends inside the interaction distance with
  ZERO stuck re-paths (the original failure signature), the walk plan
  publishes for the dump view on the way, and the merchant selection
  plus the first sell batch run at the reached shop.
- Both tests need the real geodata pack (the same candidate directory
  discovery as the pathfind town route tests, the Windows reference
  deployment included) and skip on machines without it.

### The live reproduction tool (tools/repro_stuck_trip.sh)

- Repeatable end to end scenario: checks the stack, moves the offline
  test1 to the stuck spot, arms the trigger with fresh daggers, builds
  the bot with the identity flags, runs one hunt session, captures the
  mid walk state dump into logs/repro_stuck_dump.txt and fails loudly
  when the hunt log shows a stuck re-path or the shop was never
  reached. Prints REPRO_OK otherwise. The tool run of this round: the
  mid walk dump showed `walk plan (3 waypoints)` with the deck route
  and the trip story ran trigger -> shop reached -> 25+17 items sold
  -> back at the farm spot.

### Verification

- gofmt clean, go build/vet, go test ./... (19 packages),
  golangci-lint 0 issues on the hunt package (the first lint pass
  flagged 4 nits in the new test - an ineffectual assignment, the
  ireturn interface helper, the `clear` builtin shadow - all fixed by
  returning the concrete engine and renaming).
- tools/mobius_e2e.sh 45 -> E2E_OK (the standard live verification on
  the same stack).

## Round 44: the standing hunter - the socially fenced square and the dead zone mob priorities (2026-09-10)

Scope: the user report (2026-09-10 02:11:07 state dump, the bot test1)
asked why the hunter stands in the phase engage without attacking
anyone - find the problem, fix it and log the opponent positions so
the log alone answers the "what does the bot see" question.

### The root cause (read straight out of the dump)

- The dump object list held 22 npcs with their exact positions, the
  character stood at the exact center of the elven-2019_23-b1 square
  (30502 62755 -3576), full hp, no attackers, zero recent combat for
  49 seconds after the last kill and loot.
- The square itself was NOT empty: two living attackable mobs stood
  inside it - a Kaboo Orc Fighter at 31126 61892 -3560 and a Kaboo
  Orc Fighter Lieutenant at 31137 61598 -3523, 294 units apart. Both
  carry the ORC clan with the 300 unit help range, and the engage
  pick skips any mob whose clan mate stands within help range + 200:
  the pair fenced each other out of the target search completely.
- The deadlock chain: no pickable target -> no far target walk; no
  far walk -> no patrol leg off the center; the plain emptiness
  reading of the zone rotation still counted the two fenced mobs as
  "the square has mobs" -> no rotation; and the aggressive fighter
  sat 1065 units out, just past its 1000 unit aggro range, so it
  never opened the fight either. The hunter stood still forever.

### The fix 1: the emptiness reads through the pick's own filters

- internal/swarm/state.ZoneHasPickable: the new emptiness probe runs
  the pick's own filters (the level ceiling, the skip list, the
  social clan fence) over the whole square - the distance plays no
  role because the far target walk covers the whole square.
- internal/swarm/hunt maybeRotateEmptyZone uses the pick-shaped
  reading: a socially fenced square rotates away after the regular
  10 s window exactly like a cleared-out one and the hunt moves to
  the next square instead of standing.

### The fix 1b: the targetless diagnostic (the opponent positions in the log)

- internal/swarm/state.NearestBlockedTargets classifies the living
  attackable npcs around the character the way the pick does and
  returns the nearest rejected ones with the projected position and
  the reason: the skip list, the level ceiling, the zone square or
  the social clan fence with the blocking pack mate.
- internal/swarm/hunt logNoPickableTargets: once the far search
  confirms that nothing in the square is pickable, the hunt log
  names the nearest opponents with their positions and reasons
  (one line per 5 s pacing) - the object list of the dump no longer
  lives only in the dump.

### The fix 2: the zone mob priorities resolve through the wire template ids

- The zone mob priority bias was silently dead since its
  introduction: the generated zone registries carry the Mobius CT0
  xml template ids of the spawn data (20471 for the Kaboo Orc
  Fighter) while the NpcInfo packets identify the same npc by the C4
  display id plus the 1000000 offset (1000471), so the priority map
  keys never matched a scan template id and every pick fell back to
  plain nearest-first.
- internal/swarm/npcdata gained the generated npcInternalWireIDs map
  (5781 entries from CT0_to_C4_ids.txt - the converter table is NOT
  a uniform offset, the deltas spread across 20000, 22000 and a
  dozen more values) and the NPCWireTemplateID accessor;
  zoneMobPriority translates the registry ids onto their wire keys
  at build time. An id outside the table passes through unchanged,
  so the hand written wire-id zones of the tests keep working. The
  regenerated dictionary also resyncs one aggression flag (display
  445, the Uthanka Pirate) with the current pack data.

### The reproduction tests (the dump scene verbatim)

- internal/swarm/hunt/engage_repro_test.go rebuilds the dump scene
  1:1: the character test1 at 30502 62755 -3576 (level 13, hp
  306/321) and the 22 dumped npcs at their exact positions with the
  wire template ids. TestReproDumpStandingBotExplainsItselfInLog
  pins the diagnostic (the opponents named with the dump's own
  coordinates and reasons, the pacing repeat, no attack and no walk
  request); TestReproDumpStandingBotRotatesOutOfTheFencedSquare pins
  the fix (the empty-window expiry rotates the hunter out of the
  fenced square, the next square engages).
- internal/swarm/state/scans_blocked_test.go pins the reading gap
  (TestZoneHasPickableTreatsTheFencedClanPackAsEmpty: the plain
  reading sees the fenced pair, the pick-shaped one does not, the
  broken-up pair becomes pickable again), the filter parity
  (TestZoneHasPickableAppliesLevelCeilingAndSkips) and the blocked
  list itself (TestNearestBlockedTargetsReportsTheClanPack).
- internal/swarm/hunt/zones_test.go
  TestZoneMobPriorityTranslatesTheRegistryIDs pins the id
  translation: a registry-style zone (the xml ids of the elven
  Kaboo grounds) builds the priority map on the wire keys and the
  priority 2 lieutenant wins the pick over the priority 1 fighter
  from farther out through the translated bias.

### Verification

- gofmt clean, go build/vet, go test ./... (19 packages, all green
  on the fix commits and re-run after the docs round).
- tools/mobius_e2e.sh 60 on the fix head fc2d95d -> E2E_OK (the
  standard live verification on the deployed stack: login, 60 s in
  the world, graceful shutdown, exit code 0).

## Round 45: the fight follows out of the square - rest at the kill spot, finishing chases, zone free loot (2026-09-10)

User report (2026-09-10, Russian), three behavior complaints of the
hunt loop: (1) the character runs too far away after a fight before it
sits down to rest; (2) the character stops interacting with the mobs
when the fight carries it out of the hunting zone - it should finish
them off; (3) the character does not always pick up the items on the
ground - the drops outside the hunting zone stay there, they must be
picked up regardless.

### Root causes

- The escape threat lookup kept a last resort: when no mob held the
  character as its target, `threatPosition` fell back to the nearest
  living attackable npc within 900 units. A finished fight leaves a
  fresh 3 s `SelfUnderAttack` window (the dying mob landed its last
  blow), so the hurt character right after the kill armed the flee
  against the nearest PASSIVE bystander and ran 700 unit escape legs
  away from it - up to three legs (about 2100 units, sometimes out of
  the square) before the window expired and the rest finally
  happened. The bystander never attacked; the run was pure loss.
- The zone leash of the engage dropped everything the moment the
  character stood outside the square: `returnToZone` cleared the
  target, the loot reference and walked home over the geodata - while
  the mob that dragged the character out (a chase, an aggressive
  pull) kept hitting it on the way. The fight that crossed the square
  line was abandoned instead of finished.
- The loot search passed the hunting zone to
  `NearestGroundItemExcluding`: a drop outside the square (the kill
  happened past the line, the drop scatter carried it over) was
  invisible to the loot phase forever - the Round 20 "drops outside
  the zone are ignored" policy read as a pure loss for the kills the
  chase dragged out of the square.

### Fixes

- `hunt/loop_safety.go` `threatPosition`: the last resort is gone.
  The escape runs from the living engaged target or from a mob that
  actually holds the character as its target (`NearestAttacker` -
  the Attack and MoveToPawn broadcasts set the attacker's target id).
  With no mob holding the target the escape has nothing to run from:
  the flee legs stop, the under attack window expires within 3 s and
  the rest happens at the kill spot. The pile up run and the
  emergency logout share the lookup - a dissolved pack now reads as
  "nothing to run from" instead of extending the run away from a
  bystander. The `escapeThreatRange` constant left with the fallback.
- `hunt/loop_movement.go` `adoptOutZoneFight` (new) + the leash branch
  of `engage`: before `returnToZone` the loop adopts a live fight -
  the own living target (the chase drag), the fresh server side
  selection or the nearest attacker while the blows still land (the
  chaser that followed out). All of them continue the normal engage
  flow outside the square (the losing fight flee, the pile up run and
  the stuck timeout still own their cases); a target the flee flow
  held out (the skip list) stays out - the escape decision keeps its
  authority. Without a live fight the leash walks home unchanged, and
  new fights still start inside the square only (the target pick
  keeps the zone filter).
- `hunt/loop_actions.go` `loot`: the zone argument is gone - every
  drop within the 900 unit loot radius of the character is picked up,
  wherever it lies relative to the square. The radius (not the zone)
  bounds the search, so the loot phase never wanders.

### Tests

- `TestLoopRestsAtTheKillSpot`: the dead target, its fresh last blow,
  a passive bystander within 900 units, hurt health - no escape walk
  across the under attack window, then the sit toggle fires right
  there (fails on the old code: the bystander armed a 700 unit run).
- `TestLoopFinishesTheFightOutsideTheZone`: the chase dragged the
  character out mid-fight - the attack requests continue, no leash
  walk (fails on the old code: returnToZone dropped the fight).
- `TestLoopFightsBackOutsideTheZone`: a chaser attacks outside the
  square with healthy health - the adoption picks it and the fight
  back starts (fails on the old code: the walk home went through the
  blows).
- `TestLoopPicksUpLootOutsideTheZone`: a drop past the square line is
  picked up and the loot phase keeps running (fails on the old code:
  the zone filter hid the item).
- `TestLoopEscapesALosingFight` and `TestLoopEscapesTheLevelGapFight`
  grew the missing Attack broadcast: the fleeing mob must actually
  hold the character as its target for the escape direction - the old
  tests relied on the removed bystander fallback to find the threat.

### Verification

- go build/vet, gofmt clean, go test ./... (18 packages green),
  golangci-lint: only the pre-existing unparam on
  pathfind/search_test.go.
- Live against the running Windows stack (the real login server at
  127.0.0.3:2106 per the proxy Recipe A layout, game 7777, the test1
  character at level 14): the bot picked the Kaboo ground, killed a
  mob, looted it and sat down to regenerate 3 s after the kill log
  line - the rest happened at the kill spot with mobs standing
  nearby, no escape run (the old code armed the flee against the
  nearest bystander right there). The full cycle played out: rest to
  91 percent, stand up, re-engage; the pile up safety layer fired
  when the clanned orc pack joined a fight and the session cycled
  through its documented run + logout + relogin. The session was
  stopped with a hard kill (the SIGINT sandbox pitfall), the
  shutdown path itself is untouched by this round and stays covered
  by tools/mobius_e2e.sh on the Linux deployments.

## Round 46: the relogin ground loop - the leaked fleet claim and the aggro-aware movement (2026-09-10)

User report (2026-09-10, Russian): the bot often runs to a hunting
ground THROUGH aggressive mobs, arrives with the train, resets it with
the emergency relogin - and the fresh session then walks straight to
the NEXT ground, ignoring the one it just arrived at (the user's
hypothesis: the mobs are invisible at the relogin moment). Plus the
feature request: teach the bot to move from A to B AROUND aggressive
mobs at a safe distance whenever they are not the walked-to target -
between the grounds, on the town runs both ways, and while already
fighting (stepping clear of a potential second opponent beats the pile
up logout of the real one).

### Root causes

- The fleet occupancy claim of the hunted spot LEAKED on every session
  death: `spotHunter.leave` only ran inside `apply` on a LIVE ground
  switch, but `runBot` builds a fresh `Loop` (and a fresh
  `spotHunter`) per session - nothing released the claim of the dead
  loop. Every emergency relogin left one more ghost hunter in the
  process wide `globalSpotHub`, and the occupancy division of the
  picker halved the score of the standing ground with each cycle: the
  fresh pick after the relogin preferred the neighbor ground, the
  walk there pulled another train, the next relogin leaked another
  claim - the reported "goes to the next zone right after the
  relogin" loop, compounding with every aggro reset. The user's
  invisible-mobs hypothesis pointed at the wrong layer: the relogin
  knownlist rebuild costs seconds, but the wait-or-move economy paces
  its emptiness reading in tens of seconds - the economy itself was
  sound, its occupancy input was poisoned.
- The fresh session re-contested the whole spot economy instead of
  resuming the ground under its feet: the panic run of the emergency
  logout ends a few hundred units off the anchor the character had
  just walked to, and with the ghost claim discount the scored pick
  had every reason to walk away.
- Nothing watched the aggressive camps on the transit lines: the
  Mobius AttackableAI attacks a passing player on sight inside the
  effective aggro radius (the xml value clamped at the server wide
  MaxAggroRange, 450 on this deployment) with a line of sight, so
  every straight transit through a camp collected a chaser - the
  train that forced the pile up run and the relogin in the first
  place. And nothing watched the SECOND aggressive mob closing on a
  running fight before its trigger fired - two attackers is exactly
  the pile up threshold.

### Fix

- `Loop.Run` defers `releaseSpotClaim` (`spotHunter.releaseClaim`):
  the claim lives exactly as long as the loop goroutine - the
  emergency logout, a server restart and a lost connection all hand
  it back through the same exit.
- The first pick of a fresh session (`spotEvaluate`) checks
  `standingGround` before the scored contest: a character entering
  the world inside a spot circle (the panic run endpoint sits on the
  ground it just walked to) resumes THAT ground while it stays inside
  the level window; a relogin between the grounds still falls to the
  scored pick, and an outgrown ground never resumes.
- `state.AppendAggroThreats` scans the world for exactly the mobs
  whose on-sight trigger could fire on a passing character: living
  IDLE aggressive npcs at their projected positions (the scan skips
  the chasers - the flee machinery owns them - and the busy fighters
  of somebody else's fight). The callers hand reused buffers in, the
  scan allocates nothing.
- `hunt/loop_avoid.go` deflects one walk leg at a time onto the
  tangent of the first threat circle its straight line would enter
  (the effective aggro range plus a 150 unit clearance); a character
  already inside the margin circle side-steps straight out first - no
  tangent exists from inside. The walk followers re-issue their
  requests every period from the live position, so the chained
  tangent legs arc the route around the camp while the waypoint plan
  stays untouched. Every autonomous transit walk passes through it
  (the town trip follower, the direct zone return leg - the
  inter-ground walks of the spot economy ride the same paths); the
  mobs standing at the destination stay exempt (the ground the walk
  deliberately enters is its content, the engage phase answers the
  entry radius).
- `avoidImpendingAdd` (the SelfFighting branch of the engage): an
  idle aggressive mob inside its trigger band around a FIGHTING
  character draws one paced reposition step (300 units straight away
  from the deepest-band threat, every 4 s) that owns the movement for
  its 2 s window - the forced attack re-request waits the window out
  (the server interrupts a running walk on the attack order, and the
  add would meet the character right back where it stood), the melee
  target follows the stepping character, the swings resume, and the
  distance to the add opened before its trigger fired. The leash
  outranks the add (a step leaving the square is skipped) and another
  deck never counts (the 3D gate).

### Verification

- go build, go vet, the full go test suite, gofumpt clean,
  golangci-lint with zero new findings, -race green on the hunt and
  state packages.
- The new tests: `spot_relogin_test.go` (the claim released on the
  session end, the reported scene end to end, the off-ground scored
  pick, the outgrown ground, the overlap tie) and `loop_avoid_test.go`
  (the core deflection, the tangent graze, the receding-horizon
  simulation that never dips inside the raw trigger circle and still
  passes the camp, the filters, the destination exemption, the other
  deck gate, the projected position of a moving camp, the combat step
  suite).
- The live stack validation through `mobius_e2e.sh` (the GitLab
  git-clone throttle of this sandbox broke the stock deploy at the
  116K hang; the official GitLab API archive of the same repository -
  the deploy script's own fallback channel - unblocked it): E2E_OK
  with `BOT_FLAGS=-hunt`, the bot anchored the spot economy, walked
  the pathfound return through the steering hooks and farmed the
  keltir ground.

## Round 47: the town trip water stuck - the lake under the elven village (2026-09-10)

### The report

The bot swam below the elven village plateau on its way to the trader
Ariel and stood paralyzed in the water (the state dump: the character
at 47136 46564 -3738, phase townWalk, three re-paths burned, the
remaining plan pointing at the village cliff top 45480 46680 -2992).

### The root cause chain (researched against the Mobius C1 sources)

- The server side has no water awareness at all: `Creature.
  moveToLocation` clamps a swimming click to 700 units and SKIPS the
  geodata validation entirely for a character inside a water zone (the
  whole elven region below z -3780, `21_19_water1` of water.xml);
  `GeoEngine.getValidLocation` walks the gradual lake beds like any
  other slope; `PathFinding.findPath` (7000 iteration budget, windowed
  buffer) returns null on failure and `moveToLocation` then moves the
  character STRAIGHT at the click (disregardingGeodata).
- The drowning swim z floats the character ABOVE the water zone bound
  (-3738 vs -3780): the zone membership flips with every step, so the
  character alternates between the swim semantics (straight, no
  geodata) and the dry semantics (full geodata pathfinding).
- A dry-semantics click from the floating position toward the village
  deck resolves the target cell onto the deck layer while the walk
  runs on the lake bed - `getValidLocation` ends with `previousZ !=
  nearestToZ` and returns the CHARACTER'S OWN position: the move
  request becomes a zero length walk, the character never moves
  again, and every re-path replans the same geometry from the same
  floating spot (the re-path budget burned in three 15 s stuck
  windows).
- The waypoint skip rule ("the next waypoint is closer") made it
  worse: from the water, the village cliff top waypoint (1818 units)
  beat the planned northern shore escape waypoints (4616 units), so
  the follower aimed at the unclimbable cliff and never followed the
  escape the re-path had planned - the dump only showed the tail, the
  northern leg was invisible to the debugging.
- The trip that entered the lake at all: the server's own routing of
  the per click legs (no water cost, straight fallbacks, the swim
  semantics above) - the bot's own planner always routed north around
  the lake (verified: every plan from the hunting zone is dry), but
  the follower only aimed its clicks at waypoints and let the server
  route the ground between them.

### The fix (three defenses plus observability)

- pathfind: the smoothing verifies every collapsed leg between two dry
  points stays dry (`legDry`) - the water blind string pulling used to
  ford the very bays the cost aware search paid to route around; the
  exemption for water endpoints keeps the escape legs legal.
- hunt: the follower verifies every click line with
  `Navigator.DryLine` before sending it (a clean dry walk or nothing):
  a wet click is refused and the walk re-paths around the shore; the
  refusals share the trip re-path budget.
- hunt: the water escape - a character whose geodata surface lies
  below the water level (`Navigator.OverWater`) replaces the current
  leg with `Navigator.FindWaterEscape` (the BFS flood to the nearest
  dry cell over the walkable surface), walks it without the click
  guard, re-plans a stuck escape itself, and re-plans the interrupted
  town leg from the shore with a fresh budget once dry.
- The walk plan dump and map now carry the WHOLE leg: the origin
  (where we wanted to go from), every planned waypoint with `(passed)`
  markers, the `<-- TARGET` marker on the current waypoint and the
  `dest` line - the exact debugging ask of the report.

### Tests

- pathfind: `TestSmoothedLegsStayDry` (the synthetic bay the old
  smoothing forded), `TestFindWaterEscape` / `TestFindWaterEscapeSealed
  Lake` / `TestFindWaterEscapeDryStart`, `TestDryLine`, `TestOverWater`
  and the real pack regression `TestElvenLakeStuckEscape` (the reported
  stuck position: the escape finds the shore, the route to Ariel stays
  dry leg by leg, OverWater separates the lake from the deck).
- hunt: `TestTripWaterEscapePlansShoreWalk`, `TestTripWaterEscape
  ReplansLegOnShore`, `TestTripWaterEscapeWithoutShoreAborts`,
  `TestTripWaterEscapeStuckReplans`, `TestTripWetClickRepatsAround
  Shore`, `TestTripDryClickStillWalks`, `TestTripWalkPlanCarries
  FullLeg` - and the dump format pinned in webserver.

### Verification

- go build/vet, gofumpt clean, go test ./... (18 packages green);
  golangci-lint: zero new findings over the base (the shared branch
  carries 19 pre-existing ones in the spot/zones code of the parallel
  agents).

## Round 48: the delevel water loop - the shore walks plan dry (2026-09-10)

Scope: the hang the user reported against build 6255088 (state dump:
bot test3, level 11, phase delevel, stuck at the elven village shore
40648 43432 -3624 for 25+ minutes).

### Problem statement

The bot cycles forever, 1.3 s per turn: "level 11 is too high for
level 2 mobs, deleveling to 9 at the town guards" -> "walking to the
guard Starden" -> "the walk would enter water at 40648 43432,
re-pathing around the shore (1 of 3)" (2 of 3, 3 of 3) -> "town trip
ended: aborted, the walk would cross water" -> the deleveling
restarts. The character stands dry, HP full, never moves a step.

### Root cause analysis

Two defects chained:

1. The planner and the walker disagreed about the water. The leg
   searches ran with the water tolerance of the ordinary
   FindPathApproach: a step onto an underwater cell only costs three
   land steps (search.go waterCostMultiplier), so from the shore the
   cheapest route to Starden swims across the bay - the dump walk
   plan carries wp1 40328 44408 -3800, below the C1 water level
   -3780 (reproduced byte identical on the real pack: the ordinary
   search plans exactly the dump route). The click guard of the
   follower (clickWouldEnterWater, Round 47) refuses the wet first
   leg, and the "re-path around the shore" re-planned the IDENTICAL
   route: the search is deterministic and nothing changed between the
   runs - three wasted re-paths, then the abort. A plan the walker
   refuses leg by leg can never execute.
2. The abort of a walk that runs during the deleveling ended only the
   town trip. The water guard lives in the shared town walk
   machinery and calls abortTownTrip, which ended the trip (phase
   back to engage) but left the whole delevel state armed without any
   cooldown: the very next tick delevelWanted() fired again (level 11
   over median 2, delevelEnd never armed), startDelevel reset the
   state, planDelevelWalk planned the same wet route, the same guard
   refused it. An infinite tight loop with no backoff anywhere.

### Reproduction

Real pack pathfind probe (now a regression test): FindPathApproach
from 40648 43432 -3624 to the Starden spawn 42971 51372 -2992 returns
the dump route with the wet leg; the unit loop: a delevel loop with a
navigator that answers wet lines cycles the dump log forever before
the fix, aborts into the walk home with the cooldown armed after it.

### Fix

- pathfind: the search carries a dry mode; FindPathApproachDry walls
  the water off (a step onto an underwater cell costs impassable), so
  every waypoint of a found route stands above the water level. The
  water cost search stays for everything allowed to swim.
- hunt: startWalkLeg - the shared planner of the town trips, the
  deleveling, the zone returns and the shore re-plans - navigates
  with the dry search. A destination the dry geodata cannot reach
  reports a planning failure; the callers abort the leg and arm their
  cooldowns. The old not-found fallback (a single direct walk the
  server routes itself) is gone: it planned the swim by definition
  and the direct walk itself runs the character into the lake.
- hunt: abortTownTrip during the delevel phase delegates to
  abortDelevel - the delevel cooldown arms (delevelEnd, 1 min) and
  the walk home starts. Any future walk blocker breaks the restart
  cycle the same way instead of looping.

### Tests

- pathfind: TestDryApproachDetoursAroundAChannel (the dry search
  still finds the land detour), TestDryApproachRefusesASwimOnlyTarget
  (the strait: the ordinary search swims, the dry one refuses),
  TestDelevelShoreRouteStaysDry (the real pack: the ordinary search
  plans the dump route with a wet leg, the dry route stays dry leg by
  leg).
- hunt: TestDelevelWaterAbortArmsCooldown (a deleveling whose walk
  machinery aborts - a character standing over water without a shore
  path - ends the deleveling into the walk home with the cooldown
  armed, and no tick restarts it while the cooldown runs),
  TestDelevelWetPlanTrustsAfterBudget (wet click lines on a planned
  leg exhaust the budget into the trust of the parallel village water
  raster round - the deleveling keeps walking over the server routing
  instead of the old refuse-and-restart cycle), TestDelevelWetClicks
  NeverWalk (no refused click reaches the server before the trust),
  TestDelevelDryMissAborts (a guard without a dry path aborts at the
  planning tick), TestTripDryMissArmsCooldown (the town trip variant:
  the old not-found fallback tests rewritten - the fallback planned
  the swim and is gone).

Composition with the concurrent village water raster round (rebased
onto 407c8f2): the dry planner removes the deliberate swims from the
plans, and the trusted-leg release of that round stays as the answer
to the geodata raster artifacts on a planned route (the plaza cells
without a modeled floor); a genuine swim still runs the standing
water check and the shore escape, and the abort of a delevel walk
lands in abortDelevel with its cooldown.

### Verification

- go build/vet, gofumpt clean, go test ./... (18 packages green);
  -race green on the hunt package; golangci-lint: zero new findings
  in the touched files (the 19 pre-existing branch findings in the
  spot/zones code stay untouched).
- Live stack: mobius_e2e.sh 45 and BOT_FLAGS=-hunt 90 both E2E_OK -
  the fresh elven fighter equips, pathfinds (through the dry planner),
  engages and loots with no water incidents.
- The real pack probe: the wet search plans the dump route (the
  regression is real), the dry search plans an all-dry bypass around
  the bay through the village deck (15831 units against the swim's
  11837 - the shore detour the "re-pathing around the shore" always
  claimed to do).

## Round 49: the town walk stuck loop - the reverse wall check and the waypoint skip (2026-09-11)

### The report

The bot hung cycling "town walk stuck, re-pathing (1 of 3)" -> "(2 of
3)" -> "(3 of 3)" -> "town trip ended: aborted, walk stuck" and
restarting, never reaching the trader or the teacher. The state dump
(build 36bfe99, branch feature/proxy-server): the character test1
(level 13, 25023 SP) stood at the elven village teacher plaza
(44440 52552 -2832), the hunting zone 42278 56761 behind it. The
learning trip planned 25 lessons worth 6570 SP at the teacher
Cobendell, walked to the trader Herbiel first (the sell stop) and
stuck on the very first leg - the character never moved, the 15 s
stuck timer fired, the re-path planned the identical route (the A* is
deterministic, the start and the destination were the same) and the
walker burned its whole 3 re-path budget on the same refused click.

### The root cause

The pathfinder's `wallsOpen` checked only the SOURCE cell's wall in
the step direction. The Mobius `MoveToLocation` handler checks the
TARGET cell too: `isCompletelyBlocked` rejects any target whose walls
are all closed, and the movement validation checks the target cell's
wall in the approach direction. A path that stepped onto a cell whose
reverse wall was closed was a path the server refused to walk - the
character stood still, the stuck timer fired, and the deterministic
re-path planned the identical route from the identical start. The
reverse wall check was missing from the port of the L2jGeodataPathFinder.

### The fix

- pathfind: `wallsOpen` now checks the TARGET cell's wall in the
  reverse direction too - a step onto a cell whose reverse wall is
  closed is rejected. The `Layer.IsCompletelyBlocked` helper (NSWE ==
  0) documents the server's `isCompletelyBlocked` check the pathfinder
  must never violate.
- hunt: `walkStuck` first tries to SKIP the current waypoint before
  re-planning the whole leg. The current waypoint may sit on a cell
  the server refuses to enter (a completely blocked cell, a layer
  mismatch), while the next waypoint on the planned route may be
  reachable through a different cell. The skip breaks the deterministic
  re-path loop: the follower cursor advances, the move pacer clears and
  the next tick sends a fresh click at the new target. When no more
  waypoints remain to skip, the leg re-plans from the current position
  as before. The water escape branch is unchanged (a stuck escape
  re-plans the escape itself).
- The skill teacher data was verified against the Mobius C1
  SkillLearn.xml: both Ellenia (30155/7155) and Cobendell (30156/7156)
  teach the elven fighter classes 18-24, Greenis (30157/7157) and
  Esrandell (30158/7158) teach the elven mystic classes 25-30. The
  bot's nearest teacher selection (Cobendell for a character at the
  teacher plaza) is correct - the dump's "walking to the teacher
  Cobendell" is the right teacher, the trip just never reached it.

### Tests

- pathfind/reverse_wall_test.go: the reverse wall check rejects a
  target whose reverse wall is closed, accepts one whose reverse wall
  is open, rejects a completely blocked target; the Layer helper
  IsCompletelyBlocked; the dry path from the dump stuck spot to
  Herbiel and to Cobendell both found and dry.
- hunt/walk_stuck_skip_test.go: the first stuck skips the current
  waypoint and sends a fresh walk at the new target; the re-path
  fallback fires after all waypoints are skipped; the trip aborts
  after the re-path budget; the water escape branch is unchanged.
- hunt/skill_teacher_test.go: the elven fighter has Ellenia and
  Cobendell, the elven mystic has Greenis and Esrandell, the nearest
  teacher picks the closest, the town merchants are never teachers.

### Verification

- go build, go vet, go test ./internal/swarm/... (16 packages green);
  gofmt clean; golangci-lint: zero new findings in the touched files
  (the pre-existing branch findings in the spot/zones/delevel code
  stay untouched).
- The real pack probe: the dry search from the dump stuck spot
  (44440 52552 -2832) to Herbiel (42766 50037 -2984) now plans a
  9 waypoint route of length 3258 (was 3451 before the reverse wall
  fix) that stays on the z -2832 deck and descends directly, avoiding
  the plaza detour at z -2792 that the old route climbed and dropped
  back from - the detour that triggered the stuck loop.


## Round 50: the bare-handed bot - the weapon run owns the town trips (2026-09-11)

### The report

The user report (Russian): the bot never may fight with bare hands,
buying a weapon is the highest priority whenever no weapon exists, and
the reason why it sold its weapon without buying the replacement right
away had to be found.

The state dump (build 36bfe99, bot test2, level 11, 14814 adena, the
same session family as Round 48/49) shows the character punching Kaboo
Orc Grunts for 2 damage with an empty right hand - the chat window is
a wall of "You did 2 damage" against "Kaboo Orc Grunt gave you 14
damage", the bot flees at 23 percent health and emergency logs out
from a level 7 mob it would shred with any weapon. The plan line says
"the shop strategy plans purchases worth 883 adena" - the Short Sword,
the first weapon milestone from an empty hand - and the trip carrying
that buy dies on the teacher walk: "town walk stuck, re-pathing (1..3
of 3)" -> "town trip ended: aborted, walk stuck", with the weapon buy
stop appended BEHIND the Ellenia teach stop, never reached.

### The root cause

Two defects chained, one strategic and one mechanical:

1. The sell-first replacement flow banks the worn weapon's
   referencePrice/2 credit before the buy lands (stepReplacementSales
   unequips and sells the displaced piece at the FIRST stop), but the
   buy of the replacement ran LAST: the sell stop walked to the
   NEAREST merchant (junk sells anywhere), the learning stops rode
   behind it, and planShoppingStops appended the buy groups by walking
   distance at the shop. The weapon sale and the weapon purchase were
   separated by a village walk through the stuck plaza of Round 49 -
   and by every other trip killer (an attacker interrupt drops the
   stops through resetTownTrip, a merchant that never showed up skips
   them, an exhausted buy retry budget gives them up). Whatever killed
   the leg in between, the character kept farming bare-handed: nothing
   tied the weapon sale to the weapon purchase.
2. Nothing in the hunt loop treated "no weapon" as the emergency it
   is. The trip trigger would eventually re-plan the Short Sword, but
   the 100 adena trip minimum was satisfied by any junk plan, the five
   minute trip cooldown held the retries back after each abort, and
   the engage happily punched mobs for 2 damage in between - for
   hours, as the dump uptime shows.

### The fix

The weapon now leads everything:

- gear.HasWeapon probes the whole inventory (equipped or bagged) for
  any weapon the profile scores positively - a bow is no weapon for
  the melee fighter, the starter dagger is one.
- A plan that buys a weapon routes the trip's sell stop to the
  weapon's merchant (the junk sells at any merchant): the sell-first
  of the replaced weapon and the buy share ONE stop, so the
  replacement lands seconds after the sale instead of a village walk
  later.
- The weapon run: a character with NO weapon and an affordable weapon
  in the plan runs the errand alone - no teach stops, no books, and a
  45 second retry cooldown (weaponRunCooldown) instead of the five
  minute trip cooldown, so an aborted run retries instead of punching
  mobs through it.
- The bare-handed engage gate: while the weapon run is pending, the
  targetless pick holds (logWeaponWait paces the hold line) and the
  zone entry engage of the return leg skips the same way. The attacker
  answer stays armed - a mob already on the character is fought
  whatever the weapon state is, self defense outranks shopping.
- A wallet that cannot afford any weapon keeps farming: the gate only
  holds when the plan actually offers a weapon, so a fresh bot with no
  adena still punches keltirs (the intended opening game) and the gate
  arms itself the moment the wallet crosses the cheapest offer.

### Tests

- gear/gear_test.go TestHasWeapon: the equipped sword, the bagged
  sword, the bow that is no melee weapon, the empty inventory, the
  starter dagger.
- hunt/weapon_run_test.go: the weapon run starts the trip at the
  weapon merchant with the sell stop only; the queued lessons never
  ride it; the fresh picks hold while the weapon run is pending and
  the attacker still gets fought; the pick proceeds when no weapon is
  affordable; the weapon run cooldown is 45 s while an armed character
  waits out the ordinary five minutes; the weapon upgrade routes its
  sell stop to the weapon merchant; the zone entry engage holds.
- webserver/dump_test.go TestDumpSlotNames: the body part mask labels
  of the equipment lines mirror the Mobius BodyPart enum (the old
  dumpSlotNames table shifted the jewel and armor labels - a Cloth Cap
  printed as [lfinger], a Necklace of Magic as [lear ear], Pants as
  [part 0x800] - which made the healthy paperdoll of the report read
  like corrupted equipment; the masks are verified against
  entity/item/enums/BodyPart.java).

### Verification

- go build, go vet, the full go test suite (18 packages green), -race
  green on the hunt package, gofmt/gofumpt clean, golangci-lint zero
  new findings in the touched files (the pre-existing branch findings
  stay untouched).
- Live validation on the local stack: the dump state injected through
  the database (level 11, 14814 adena, the full armor floor of the
  report, NO weapon, standing at the dump hunting spot). The bot held
  its target picks, ran the weapon errand at once ("no weapon in hand,
  the weapon run comes first, walking to the trader Unoren"), sold the
  junk at Unoren, bought the Short Sword two seconds later (list
  3014700), equipped it into the empty right hand and walked back to
  the farm spot - the database holds the sword in PAPERDOLL slot 7 and
  the wallet at 14005 (the 883 price plus the loot of the walk back).
  The SIGINT shutdown stayed graceful (exit 0).

## Round 51: the teacher walk stuck - the walled skip of the follower, the walkable line gate (2026-09-11)

The user report (Russian): the bots freeze and never learn - the
state dump (build 36bfe99, level 11 elven fighter test2, phase
townWalk) showed the learning trip walking to the teacher Ellenia,
passing waypoints 0..10 of the plan and then standing frozen at
46152 51656 -2808 aiming at wp 11 (45992 52040 -2792) while the
re-path budget burned ("town walk stuck, re-pathing (1..3 of 3)" ->
"town trip ended: aborted, walk stuck"). The character then walked
back to the hunting zone, fought bare-handed (no weapon - the
planned 883 adena Short Sword purchase never ran either), nearly
died to a level 7 Kaboo Orc Grunt, emergency-logged-out, relogged
and restarted the identical trip - forever, no lesson ever learned.

### Root cause

The server (Mobius C1, the deployment sets PathFinding = 0) never
routes a ground click: `Creature.moveToLocation` runs the
`getValidLocation` straight line raster, and a click whose first
step hits a closed cell wall resolves to the character's own
position - the distance collapses under 1 and the move is canceled
silently (setIntentionIdle + ActionFailed, nothing in the log). The
waypoint follower of the town trips skipped waypoints inside the 50
unit pass radius without checking the line ahead: the trainer plaza
approach climbs a ramp whose smoothed steps sit 16..48 units apart,
the server stopped the character one cell east of the ramp top
(46152 51656 -2808), the follower skipped wp 9 AND wp 10 (both
within 50 units) and clicked wp 11 - the straight line from that
pocket cell crosses the closed NORTH wall of the standing cell (the
plaza railing; the WEST wall of the same cell is closed too, the
only way out is south). The click canceled, the character never
moved a unit, the 15 s stuck detector re-planned - and the
deterministic search reproduced the identical route, the follower
skipped the same tight waypoints from the same pocket, the third
budget burned and the trip aborted before the teacher stop ever
ran. The cooldown did not even damp the loop: a fresh Loop per
session carries no tripEndedAt, so the emergency-logout relogin
re-armed the walk within ~100 s.

Probed against the real geodata pack (pathfind/teacher_walk_test.go
pins the exact geometry): LineOfSight(pocket -> plaza) is false,
LineOfSight(pocket -> ramp top) is false, the south line to the
ramp foot is clear, the climb from the foot is clear, and the
re-plan from the pocket finds the detour through the foot. The
engine and the server read the same NSWE cells.

### Fix

The follower gates every waypoint skip on the walkable line
(`legAdvanceClear` through the Navigator.LineOfSight the blind
engage already uses): a waypoint only counts as passed when the
straight line from the CURRENT standing cell to the successor
waypoint passes the geodata line of sight. A gated waypoint stays
the target - walking onto it re-opens the line - and the follower
never again clicks a line the server would cancel at its first
step. A line the geodata cannot verify stays clear (the pre-gate
behavior), and the skip cursor moved into `advanceWaypoints` to
keep the follower under the complexity limit. The stuck detector
stays the backstop for a truly off-route position (a teleport, a
chase), where the re-path now also works: the fresh route's tight
steps no longer get blindly skipped.

### Composition with the concurrent rounds

The parallel session of 2026-09-11 fixed the same user report from a
second dump: Round 49 added the reverse wall rule to the pathfinder
(a route never steps onto a cell whose reverse wall is closed - the
planning level) and the stuck waypoint skip (a stuck walk advances
past its current waypoint before re-planning - the recovery level).
This round adds the missing prevention level: the follower never
skips a waypoint whose successor line is not walkable from the
actual standing cell, so the walk never clicks through a wall in the
first place. The three fixes compose: the routes avoid reverse
walled cells, the follower only skips along verified lines, and a
genuinely off-route position still recovers through the stuck
machinery. Round 50 (the weapon run) fixed the bare-handed half of
the report: the Short Sword purchase now leads the town trips.

### Tests

- pathfind/teacher_walk_test.go (the real pack): the dump route
  reproduces exactly (TestTeacherRouteMatchesTheDumpPlan), the
  pocket corner walls (TestTeacherCornerWallBlocksTheStraightClick)
  and the re-plan detour (TestTeacherReplanFromTheStuckCorner).
- hunt/teacher_walk_test.go: the gate itself on the exact dump state
  (TestTeacherWalkKeepsTheWaypointWhenTheLineAheadIsWalled - the
  fake navigator answers the probed lines; the first click goes
  south to the ramp foot, never to the walled plaza; the walk then
  advances through the corner) and the end-to-end recovery under the
  simulated server from the reported stuck spot
  (TestTeacherWalkRecoversFromTheDumpStuckSpot - the leg completes
  into the stop phase within one recovery re-path). The fake
  navigator's LineOfSight default flipped to clear (the `blind`
  flag) because the follower gate now queries it on every walk.

---

## Round 52: the town walk click collapse - the server refuses what the plan crosses (2026-09-11)

Composition with the concurrent rounds of the same report: round 49
(the reverse wall check of the search) and round 51 (the walkable
line gate of the waypoint skip) closed the loops their dumps showed;
this round closes the mechanism underneath all of them - the click
itself. The server click validation collapses a click whose
Bresenham line cuts a walled corner (the anti corner cut) or lands
on the wrong layer, and no planned route, skip gate or reverse wall
check survives that: the plan itself must only contain legs the
server accepts. The three fixes compose (the reverse walls, the
gated skips and the validated clicks each answer a distinct refusal
channel of the same server pipeline).

Scope: the 2026-09-10 state dump report - the bot test1 (level 13,
phase townReturn) stood frozen at 44440 51688 -2832 (the elven village
terrace east of the plaza) with "Hunt: town walk stuck, re-pathing
(1 of 3) .. (3 of 3)" grinding the whole budget while the character
never moved a single unit.

### Problem statement

The zone return after a relogin planned an 11 waypoint route over the
village plaza (wp1 z -2792, wp2 z -2832, 58 units away), the follower
clicked wp2 every 2 seconds and the server never executed any of the
clicks: no movement broadcast, no position change, three re-paths
reproducing the same plan and the trip aborting into the same frozen
state again (the returnToZone re-trigger loop visible in the dump).

### Root cause analysis

The live server log (the MOVEDBG diagnostics patch of the local
Mobius checkout, MoveToLocation.runImpl + Creature.moveToLocation)
names the mechanism exactly:

```
MOVEDBG: test1 click 44408 51736 -2832 from 44440 51688 -2832 mode 1
MOVEDBG: test1 move CANCELED, distance=0.0 (geodata collapsed the
target onto the walker), cur 44440 51688 -2832 -> 44440 51688 -2832
```

The Mobius C1 click pipeline (all references in
`L2J_Mobius_C1_HarbingersOfWar/java`):

1. `MoveToLocation.runImpl` hands the click to the AI, which calls
   `Creature.moveToLocation(x, y, z, 0)`.
2. With `PathFinding > 0` (the reference deployment runs 2) the
   destination is corrected by `GeoEngine.getValidLocation(cur, target)`:
   it walks the Bresenham cell line of `GridLineIterator2D` with the
   running height of `getNearestZ`, refuses a step that climbs more
   than `HEIGHT_INCREASE_LIMIT` (40) without a near neighbour layer,
   refuses a blocked cell and applies `checkNearestNsweAntiCornerCut`
   to every step - a DIAGONAL step needs both flanking cells to allow
   the crossing (the SW step wants (x, y+1) open west and (x-1, y)
   open south). The line of the stuck click cuts the plaza corner:
   its first diagonal step (43737,40094)->(43736,40095) needs cell
   (43737,40095)@-2832 open west, and that cell carries nswe 0x0d -
   the terrace wall. The validation stops BEFORE the first step and
   collapses the destination onto the walker cell center - which for
   the relogin position (44440 51688, the exact cell center) IS the
   character position: distance 0, the move is canceled, ActionFailed
   is sent, the character never moves.
3. The click distance of 58 units is NOT the trigger - any click whose
   straight line crosses the walled corner collapses the same way
   (longer lines to the same heading refused identically in the
   traces). The user hypothesis ("too close to walk") was close in
   effect (the collapsed target equals the walker only for short
   in-cell clicks) but the mechanism is the line validation, not the
   distance.

Why the bot planned a route the server refuses: two raster
mismatches between the pathfind engine and the server.

- The A* expands 8 neighbors but `wallsOpen` checked only the SOURCE
  cell walls of the diagonal - the server (both the click validation
  and its own `NodeBuffer.expandNeighbors` diagonal gating) requires
  the flanking cells too. The planned route climbed the plaza wall
  through the corner the terrace had walled off.
- The smoothing verified its collapsed legs with the t/k supercover
  raster, which decomposes a diagonal line into cardinal steps -
  the server Bresenham steps diagonally and applies the anti corner
  cut. A leg can be supercover-legal and Bresenham-refused at once
  (the second boxed spot of the offline replay: a 700 unit leg over
  the field terraces).

### Reproduction

- Live: place test1 at 44440 51688 -2832 with level 13 (the dump
  state), run the bot with -hunt against a stack with `PathFinding = 2`
  and the 21_19 geodata region loaded (the reference deployment
  layout; the sandbox fast deploy defaults to `PathFinding = 0`, which
  skips the whole validation - the bug cannot reproduce there). The
  old build: "town walk stuck" three times, "town trip ended: aborted,
  walk stuck", the character frozen. The MOVEDBG patch (diagnostics
  only, the server integrity rules allow logging) prints the
  CANCELED line above for every refused click.
- Offline: `TestReproVillageZoneReturnWalksThePlan`
  (hunt/village_return_repro_test.go) replays the exact dump position
  against the real geodata pack with the server click semantics
  ported into the test sim.

### Fix

The bot adapts to the server (no server behavior changes - the
MOVEDBG patch is logging only):

1. `pathfind/search.go`: the diagonal step rule of the search mirrors
   the server anti corner cut (`wallsOpen` -> `diagonalFlanksOpen`):
   the A*, the line of sight raster, the direct line answers and the
   step costs all refuse a corner cut the server would refuse. The
   planned routes stop relying on walled diagonals.
2. `pathfind/click_validate.go` (new): `Engine.ValidateClick` - a
   faithful port of the server click pipeline (the Bresenham
   `GridLineIterator2D` raster, the running `getNearestZ` height
   resolution, the 40 unit climb limit with the neighbour layer
   step-over, the blocked cell check, the anti corner cut, the final
   layer match rule that collapses a line arriving on the wrong deck,
   and the far click rule beyond 3000 units). The smoothing now
   verifies every collapsed leg with it (`serverLegVerified`) - a
   planned leg is a click the server accepts, by construction.
3. `hunt/town.go`: the follower gates every click through the port
   (`Navigator.ValidateClick`): a refused click is never sent. The
   reaction chain: shorten the leg (the Bresenham prefix of a split
   leg is not a prefix of the full raster, a shorter line often
   validates), hop back to the nearest plan bend the arrival slack
   swallowed (the escape step out of the geodata trap cells - cells
   entered legally whose own walls box the walker in), and finally
   the re-path of the stuck path, bounded by the same budget.

### Verification

- go build/vet, gofmt clean, go test ./... (19 packages green);
  golangci-lint: zero new findings in the touched files.
- Offline regression: the full village walk replays against the
  ported server rules in 13 clicks with zero refused clicks and zero
  re-paths (`TestReproVillageZoneReturnWalksThePlan`), the synthetic
  wall corner and layer collapse cases pin the port itself
  (`click_validate_test.go`), the follower reactions pin the
  shorten/hop/re-path chain (`click_guard_test.go`).
- Live: the same dump scenario on the fixed build walks 44440 51688
  -> the Kaboo Orc Grunt S zone in 54 seconds (16 clicks, all
  ACCEPTED on the server MOVEDBG log, zero CANCELED, zero stuck
  re-paths), engages and kills on the zone entry, E2E_OK on the
  graceful shutdown. The mobius_e2e.sh 45 run stays E2E_OK.
- Note: tools/repro_stuck_trip.sh (the round 35 harness) fails on
  BOTH the baseline and the fixed build with the current test1 state
  (a level 13 character with equipped gear - the auto equipment
  destroys the injected junk daggers one per 2 s before the trip
  sells them); the failure predates this round and reproduces on the
  unmodified commit 36bfe99 build.

### Follow ups

- The manual long walk follower (hunt/user.go) and the blind engage
  recovery walker (hunt/loop_los.go) still send unvalidated clicks;
  the same gate would harden them (they have the same freeze risk on
  walled corners).
- The server MOVEDBG logging patch stays in the local Mobius checkout
  only (never committed to the swarm repository - the server
  integrity rules).

## Round 53: the roof teleport - the npc approach clicks the offset, not the exact cell (2026-09-11)

Scope: the 2026-09-11 user report (Russian) - the bots run somewhere
behind the building trying to talk to the village teachers (Cobendell
et al.), and when the character approaches the npc the server
teleports it onto the roof of the building instead of letting it
enter inside. The user asked to study where Cobendell and the similar
npcs stand and to make the bot approach them at a short distance, not
talk through the wall.

### Problem statement

The state dump (build bb12d42, bot test3, phase idle) showed the
character at 44831 52389 -2796 (26 units from Cobendell at 44823
52414 -2792) with Cobendell selected as the target. The user's manual
test confirmed the roof teleport: clicking on Cobendell from certain
directions lands the character on the roof (z -2456..-2576) instead
of the ground floor (z -2792).

### Root cause analysis

A geodata probe (scripts/probe_cobendell, run against the real
21_19.l2j region) confirmed the mechanism:

- The Cobendell cell (44823 52414) carries two layers: the ground
  floor at z -2792 (where the npc stands) and the roof at z -2448
  (the building's roof layer). The cell south and west of Cobendell
  has only the water/abyss layer at z -3872..-3904 (the lake below
  the floating island).
- The server's `GeoEngine.getValidLocation` (ported as
  `Engine.ValidateClick` in round 52) walks a Bresenham cell line
  from the click origin to the click target. When the click targets
  Cobendell's exact cell from the south or west, the line crosses
  the building wall, the height-step fallback
  (`neighbourLayerNear`, the `clickLayerTolerance` of 16) resolves
  the target onto the roof layer, and the bot ends up on the roof.
  The probe measured it directly: clicking on Cobendell from
  deg 30 (south-east) redirects to z -2576, from deg 150
  (south-south-west) to z -2568, from deg 180 (west) to z -2456 -
  all roof heights, not the ground floor.
- The bot's `approachTeacher` and `approachMerchant` (hunt/learning.go,
  hunt/town.go) clicked the npc's EXACT spawn cell when the bot was
  far (dist3D > 200): `l.walkToward(x, y, z, now)` sent the npc's
  coordinates directly. The pathfinder had already planned a route
  to within 200 units of the npc, but the final approach leg clicked
  the exact cell - and the server teleported the bot onto the roof.

The "deck hop" code (dist2D <= 200, dist3D > 200, the z mismatch
case) had the same problem: it clicked the npc's exact cell to let
the server routing walk the bot up a ramp, but the click line crossed
the wall and the height-step fallback put the bot on the roof.

### Fix

The approach walk clicks the npc approach point, not the npc's exact
cell. The approach point is `npcApproachOffset` (150) units from the
npc toward the bot, so the Bresenham click line stays outside the
building walls and the server validates it on the ground floor. The
150 unit offset keeps the bot within the 250 unit server interaction
distance (the talk click that follows works) but outside the walled
interior.

- `hunt/town.go`: `npcApproachOffset = 150.0` and `npcApproachPoint`
  compute the offset target. `approachMerchant` uses it for both the
  far walk and the deck hop case. The deck hop case skips the click
  when the offset collapses onto the bot's own cell (the
  `hopCoincideDist` gate) - the deck window bounds the wait before
  the merchant is given up.
- `hunt/learning.go`: `approachTeacher` uses the same offset for both
  the far walk and the deck hop case, with the same skip gate.

### Verification

- go build/vet, gofmt clean, go test ./... (19 packages green);
  golangci-lint: zero new findings in the touched files.
- The offset computation tests pin the geometry: the target lies on
  the line from the npc to the bot at the configured distance, never
  past the bot, and collapses onto the bot's own cell when the bot is
  already within the offset (`TestNpcApproachPointOffsetsTowardTheBot`,
  `TestNpcApproachPointCollapsesOntoTheBotWhenClose`).
- The approach tests pin the fix: a bot 500 units from Cobendell
  clicks the offset point (44823 52264 -2792), never the exact cell
  (44823 52414 -2792) - the roof teleport root cause
  (`TestApproachTeacherClicksTheOffsetNotTheExactCell`). The same for
  the merchant approach (`TestApproachMerchantClicksTheOffsetNotTheExactCell`).
- The deck hop test pins the safety: a bot within the 2D interaction
  distance but on a different z does NOT click the teacher's exact
  cell - the offset collapses and the `hopCoincideDist` gate skips
  the click (`TestApproachTeacherDeckHopSkipsTheClickWhenTooClose`).
- The probe (deleted after the analysis, the findings live in the
  tests) confirmed the safe approach directions: north and east of
  Cobendell validate fine (the click lands on the ground floor),
  south and west redirect to the roof or water. The offset keeps the
  click on the safe side regardless of the bot's approach direction.

### Follow ups

- The manual long walk follower (hunt/user.go) and the blind engage
  recovery walker (hunt/loop_los.go) still send unvalidated clicks
  to npc positions; the same offset rule would harden them if they
  approach town npcs.
- The pathfinder's approach radius (`tripApproachRadius = 200`) and
  the waypoint arrival slack (`waypointArriveDist = 150`) can leave
  the bot 350 units from the npc, which triggers the far walk case.
  A tighter approach radius for the teach stop would reduce the gap,
  but the offset fix already keeps the far walk safe.

## Round 54: the offset ring stuck - the talk click fires within the server interaction distance (2026-09-11)

Scope: the 2026-09-11 05:45 user follow-up dump. The roof teleport
fix of round 53 (the npc approach offset point) closed the roof
teleport, but the bot then stuck on the offset ring: the talk click
waited for dist3D <= 200 (the approach gate) while the bot stood at
dist3D 244 (within the server 250 interaction gate but above the 200
approach gate, because of the z gap between the approach deck and the
trainer hall floor).

### Problem statement

The dump (build 86b4c86, bot test1, phase townSell) showed the
character at 44616 52536 -2832 (dist 244 from Cobendell at 44823
52414 -2792, dz 40), target=self, stuck for 31 seconds after "learn:
teacher Cobendell found, walking to it". The bot sold junk at Herbiel,
advanced to the Cobendell teach stop, walked to the offset ring and
then looped: approachTeacher clicked the offset point, the bot walked
there, but the z gap kept dist3D above 200 forever, the talk click
never fired, the bot never selected Cobendell, the lessons never
landed.

### Root cause analysis

A geodata probe reproduced the scenario against the real 21_19.l2j
region:

- FindPathApproachDry from 44616 52536 -2832 to Cobendell with radius
  200 returns 2 waypoints: wp0=self, wp1=44680 52536 (64 units from
  the bot, 188 from Cobendell). The bot is already within
  waypointArriveDist (150) of wp1, so walkTownWaypoints returns true
  at once -> enterSellPhase -> teachStop -> handleTeacher finds
  Cobendell -> approachTeacher fires.
- approachTeacher: dist3D=244 > merchantApproachDist (200),
  dist2D=240 > 200, so the "far walk" branch fires. It clicks the
  offset point (44694 52490, 91 units from the bot). The bot walks
  there, but that is NOT closer to Cobendell in 3D - dz=40 keeps
  dist3D above 200. The next tick recomputes the offset from the new
  position, the bot circles on the offset ring forever.

The approach gate (200) was a planning heuristic (where to aim the
walk), but the talk click gate must match the server
INTERACTION_DISTANCE (250) - the server accepts the ClickObject
action and the transactions within 250 in 3D, regardless of the
approach gate. The 40 unit z gap (the trainer hall floor at -2792
vs the approach deck at -2832) is a single geodata step - walkable
in principle, but the pathfinder's approach radius (200) stops the
walk short of it. The talk click must fire from the offset ring.

### Fix

The talk click (and the merchant select) fire as soon as the bot is
within the server interaction distance (npcInteractionDist = 250 in
3D), even when the z gap keeps dist3D above the approach gate (200).

- `hunt/town.go`: `npcInteractionDist = 250.0` mirrors the server
  INTERACTION_DISTANCE. `approachMerchant` checks it first: if
  dist3D <= 250, the merchant select proceeds (the new
  `selectMerchant` helper). The far walk only fires when dist3D >
  250.
- `hunt/learning.go`: `approachTeacher` checks `npcInteractionDist`
  first: if dist3D <= 250, the talk click fires (the new
  `clickTeacher` helper). The far walk only fires when dist3D > 250.
- The deck hop case (dist2D <= 200, dist3D > 200) is unchanged: the
  offset collapses onto the bot's own cell, the deck window bounds
  the wait. The new early return takes over before the deck hop
  branch when dist3D <= 250, so the deck hop now only fires when the
  z gap is large enough to push dist3D above 250 (a real deck
  mismatch the server routing must handle).

### Verification

- go build/vet, gofmt clean, go test ./... (19 packages green);
  golangci-lint: zero new findings in the touched files.
- The offset ring tests pin the fix: the bot at the exact dump
  position (44616 52536 -2832, dist3D 244) clicks the teacher
  directly - no ground walk, the talk click fires
  (`TestApproachTeacherTalkClickFiresFromTheOffsetRing`). The deck
  hop case with dist3D in (200, 250] also clicks the teacher
  (`TestApproachTeacherDeckHopClicksFromTheOffsetRing`).
- The existing offset tests stay green: the far walk still clicks
  the offset point when dist3D > 250, the deck hop skip gate still
  fires when the offset collapses onto the bot.

### Follow ups

- The deck hop window (30 s) now only fires when dist3D > 250 - a
  real deck mismatch. The trainer hall floor (dz 40) no longer
  triggers it; the talk click lands at once.
- The manual long walk follower (hunt/user.go) and the blind engage
  recovery walker (hunt/loop_los.go) still use the old approach gate
  for their npc interactions; the same npcInteractionDist early
  return would harden them if they approach town npcs.

## Round 55: the zone return stuck - the town trip blocks out, the non-dry fallback routes home (2026-09-11)

Scope: the 2026-09-11 06:00 user follow-up dump. The bot test3
(build c1faefb, phase engage) stood at 43000 50184 -2992 (near
Herbiel, outside the zone) for 3 minutes. Events: "no dry path to
38553 50080, the walk would swim" repeated, then a learning trip
started and also failed with "no dry path to 42766 50037" (Herbiel
is only 276 units away). The bot never moved.

### Problem statement

Two bugs composed:

1. The town trip started while the bot was outside the zone.
   `handleTownTrip` runs before `engage()` in the tick, so
   `maybeStartTownTrip` fired and started a learning trip (2090 SP
   worth of lessons) before the zone return had a chance. The
   learning trip's `startWalkLeg(Herbiel)` failed ("no dry path"),
   the trip aborted and armed the 5 minute cooldown. The zone return
   then tried and also failed. The bot was stuck at the village.

2. The zone return had no non-dry fallback. `returnToZone` called
   `startWalkLeg` (dry search only). When the dry search failed, it
   fell back to `walkZoneLeg` (direct walks toward the zone center).
   The direct line from 43000 50184 to 38553 50080 crosses water
   (the probe showed water at 741..1928 units along the line) and
   walls, so the server refused the walks and the bot stood still.

A geodata probe (scripts/probe_zone_return, since deleted) confirmed
that `FindPathApproachDry` from 43000 50184 -2992 to both Herbiel
(276 units) and the zone center (4447 units) SUCCEEDS in the offline
probe against the project geodata pack. The runtime failure reason
is unresolved (possibly a different geodata directory at runtime, or
a race condition), but the non-dry fallback gives the bot a route
regardless.

### Fix

1. **The town trip is blocked while the bot is outside the zone.**
   `maybeStartTownTrip` checks `l.zone() != nil && !l.inZoneSelf()`
   and returns early (the zone return owns the walk). The weapon run
   is the sole exception: a bare-handed character shops for a weapon
   at once, even outside the zone (punching mobs through the walk
   home is worse than a late return).

2. **The zone return tries the non-dry search as a fallback.** The
   new `startZoneReturnLeg` method runs the dry search first, and
   when it fails, runs the non-dry search (`FindPathApproach`,
   water allowed with a cost penalty). The click guard of the town
   walk follower refuses water legs and re-paths around the shore,
   so a non-dry plan with water legs is still safe to walk - the bot
   follows the dry parts and re-plans at the waterline. If both
   searches fail, the fallback to `walkZoneLeg` (direct walks) stays.

   The refactoring splits `startWalkLeg` into:
   - `startWalkLeg(dest)` - the dry-only search (the town trips,
     unchanged behavior).
   - `startZoneReturnLeg(dest)` - the dry + non-dry fallback (the
     zone return).
   - `startWalkLegSearch(dest, nonDry)` - the shared core.

### Verification

- go build/vet, gofmt clean, go test ./... (19 packages green);
  golangci-lint: zero new findings in the touched files.
- The zone return tests pin both fixes:
  - `TestTownTripBlockedOutsideTheZone`: a bot outside the zone with
    a learning budget does NOT start a town trip (the zone return
    is armed instead).
  - `TestTownTripStartsInsideTheZone`: a bot inside the zone starts
    the town trip normally.
  - `TestTownTripWeaponRunStartsOutsideTheZone`: the weapon run
    exception - a bare-handed character starts the weapon trip even
    outside the zone.
  - `TestZoneReturnNonDryFallback`: when the dry search fails, the
    non-dry search runs and the zone return walks.
  - `TestZoneReturnDryFailureFallsBackToWalkZoneLeg`: when both
    searches fail, the direct walk fallback fires.
  - `TestStartZoneReturnLegTriesDryThenNonDry`: the search order is
    dry first, non-dry second.
- The existing town trip tests stay green: the dry search behavior
  for the town trips is unchanged.

### Follow ups

- The runtime dry search failure reason is unresolved. The offline
  probe against the project geodata succeeds; the bot's runtime
  geodata might differ (a different `detectGeodataDir` candidate, a
  different geodata pack). The non-dry fallback masks the issue but
  does not root-cause it.
- The `walkZoneLeg` direct walk fallback still sends unvalidated
  clicks (no `ValidateClick` gate). The same gate the town walk
  follower uses would harden it.

## Round 53: the town walk stuck budget - the shared re-path budget and the slow skip recovery (2026-09-11)

Composition with round 52 (the click validation port): round 52 made
the pathfinder's plan sound - every click the follower sends would
survive the server validation. Round 53 closes the recovery half: the
budget that bounds the recovery ate itself on the dump scenario, even
though the plan itself was correct.

Scope: the 2026-09-11T05:06:14+03:00 state dump report - the bot test2
(level 11, phase townReturn) stood frozen at (46008, 51992, -2792) -
the elven village teacher plaza - cycling "town walk stuck, skipping
waypoint (1 of 3)" -> "the server would refuse the walk click to 45304
52152, re-pathing (2 of 3)" -> "town walk stuck, skipping waypoint (3
of 3)" -> "town trip ended: aborted, the server refuses every walk
click" TWICE within 90 seconds. The build f1c3136 (the round 52 fix)
reproduced the freeze on a new spot.

### Root cause

The pathfinder's plan was SOUND. The offline reproduction
(TestReproRound53ZoneReturnWalksThePlan) walks the exact dump position
to the hunting zone in 13 validated clicks with zero refused clicks and
zero re-paths. The round 52 click validation port correctly mirrors the
Mobius GeoEngine.getValidLocation (verified against the actual Mobius
Java source).

The freeze was in the RECOVERY BUDGET. Two compounding design flaws:

1. The waypoint skip (a cursor advance, no navigator call) consumed the
   same maxRePaths=3 budget as the full leg re-plan (startWalkLeg, an
   A* search). The dump alternated between walkStuck (skip, rePaths++)
   and clickServerValidated (re-path, rePaths++), consuming the budget
   in 2 cycles (40 seconds) and aborting.

2. The stuck timeout was 15 seconds for EVERY stuck. Once the walker
   knew the server refused its clicks, waiting 15 seconds for every
   subsequent waypoint just burned the trip's time budget.

### Fix

1. `hunt/town.go`: the waypoint skip of `walkStuck` no longer consumes
   the re-path budget. Only the full leg re-plan and the water escape
   re-plan consume it.
2. `hunt/town.go`: the `stuckFastTimeout` (4 seconds) arms after the
   first skip. The first stuck keeps the full 15s window; subsequent
   stucks fire on the shorter window.
3. `hunt/loop.go`: the `stuckFast` field added to the Loop struct.
4. `walkStuck` split into `walkStuck` + `stuckWaterEscape` +
   `stuckTownWalk` to stay under the funlen limit.

### Verification

- go build/vet, gofmt clean, go test ./... (18 packages green).
- The exact round 53 dump walk replays with zero refused clicks and
  zero re-paths (TestReproRound53ZoneReturnWalksThePlan). The skip-no-
  budget and fast-timeout behavior pinned by the walk_stuck_skip tests.

## Round 56: the building stuck - the skip gate and the re-planned self-click (2026-09-11)

The user report (the 06:19 state dump, build 896865d, bot test2, phase
townWalk): the bot froze trying to enter the elven village trainer
hall on the teach walk to Ellenia - the plan aimed at wp 2 (44728
52040 -2792), 48 units south of the character standing in the aisle
entrance (44728 51992 -2792), and the events showed "town walk stuck,
skipping waypoint (1 of 3)" -> "the server would refuse the walk click
to 45160 52120, re-pathing (2 of 3)" - the trip burning its budget
the way to an abort. The user asked how the building is represented,
why an impassable path to the NPC gets planned (suspecting the roof
and floor z coordinates), a fix, and a test-proven guarantee that the
bot reaches this NPC from different positions in town.

### The building in the geodata

The trainer hall cells carry three layers: the sloped roof
(-2600..-2448, NSWE mostly 0xB), the walkable floor (-2792..-2832,
the walls encoded in the floor NSWE flags) and the water deck (-3928)
under everything. The west aisle column (x 44712..44744) is fully
open down to the south hall (rows y 52040+), the south hall row
(y 52040..52055) has the north wall closed east of the aisle (the
building's south wall), and the interior east of the aisle
(x 44744..44792, y 52008..52039) has NO floor layer at all - only
the roof and the water. The dump's route through the aisle is
walkable (the 48 unit south leg validates in full against the
ported server rules and the live server walks it - verified on the
local stack), so the plan itself was never impassable.

### Root cause

Two follower defects composed:

1. The blind stuck skip: `stuckTownWalk` advanced the cursor to the
   next waypoint without checking the line from the standing cell.
   From the aisle entrance the click to the east hall waypoint
   (45160 52120) is a PARTIAL under the server rules - the Bresenham
   line's SW diagonal flank (44760 51992) carries the building's
   north wall (its south wall is closed), the anti corner cut stops
   the line at the first step and the server walks the character 16
   units east only. Every re-click crept the character one cell
   further along the building's north wall row into the dead-end
   pocket cell (44776 51992 -2808, its east and south walls closed),
   and from the pocket the click to the hall waypoint collapses onto
   the walker itself - the refused click the dump logged.

2. The re-planned self-click: after a stuck re-path the follower
   clicked the fresh plan's wp 0 - the standing cell itself - in the
   same tick. The server always refuses a click whose destination is
   the character's own position (the distance is below the
   cancellation limit), so the recovery burned a second re-path on
   the guaranteed refusal. The round 56 reproduction caught this
   live: the first freeze tick fired the re-path, the same-tick
   self-click was refused, and the budget sat at 2 of 3 before the
   walk even restarted.

### Fix

1. `hunt/town.go`: `stuckTownWalk` skips only onto a waypoint with a
   walkable line from the standing cell - the new `nextClearWaypoint`
   scans the plan ahead through the same `legAdvanceClear` gate the
   cursor advance uses. When no successor is reachable, the leg
   re-plans at once (the fresh aisle route starts with the 48 unit
   south click the server always accepts) instead of arming the
   partial click that creeps into the pocket.
2. `hunt/town.go`: `followWaypoints` re-runs the cursor advance after
   the stuck handling, so a re-planned leg aims its first real
   waypoint, never the standing-cell wp 0.

### Verification

- go build/vet, gofmt clean, golangci-lint --new zero findings, the
  full go test suite green.
- The exact dump walk replays against the real geodata pack with the
  simulated server: the aisle entrance walk to Ellenia arrives with
  zero refused clicks and zero re-paths
  (TestReproRound56AisleWalksToEllenia), and the frozen-aisle
  simulation (the clicks silenced, the stuck armed) recovers through
  exactly one re-path with the character never creeping east of the
  aisle entrance and no click ever refused
  (TestReproRound56StuckSkipNeverCreepsIntoThePocket).
- The Ellenia reach acceptance: the dry approach search from seven
  village positions (the dump aisle entrance, the trap pocket, the
  north terrace bend, the south approach, the east plaza, the shop
  deck, the southwest shore path) all find routes whose every leg
  validates in full against the ported server rules
  (TestElleniaReachableFromEveryVillageApproach), the aisle route
  pinned to the dump plan (TestElleniaAisleRouteMatchesTheDumpPlan)
  and the pocket/aisle click geometry pinned to the refusal answers
  (TestElleniaPocketLinesRefuseTheHallClick).
- The live stack validation: the full dump scenario (the character
  injected into the east hunting zone, level 11, 934 SP, Power Strike
  wiped, the spellbook unbought) ran the whole town trip - the junk
  sale, the book purchase at Creamees, the walk to Ellenia - and the
  teacher was reached and all the lessons landed with no stuck and
  no refused click; the raw surgical clicks of the aisle legs
  (44728 51992 -> 44728 52200 -> 45160 52120 -> 45725 52105) all
  walked on the live server.

## Round 57: the pathfind freeze - the un-rescuable short click and the identical re-plan (2026-09-11)

The user report (the 11:34 state dump, build d0cd543, bot test2,
phase townReturn): after a relogin the character stood at
(44296 51480 -2848, the village south terrace) with an 11 waypoint
plan to the Kaboo Orc Grunt S zone whose FIRST waypoint sat 22 units
out (44280 51464 -2832, up the 16 unit terrace step), and the
character never moved a single cell: "town walk stuck, re-pathing"
fired three times, the trip aborted, the zone return restarted the
identical cycle, and the dump caught the second trip mid-cycle - two
complete trip cycles with no movement and no "the server would
refuse" line (the local click validation kept blessing every click).

### Root cause (probed live with the MOVEDBG diagnostics build)

The plan itself is valid - every leg validates against the server
click port, and the local Mobius stack (both PathFinding=0 and =2,
the byte-identical 21_19 geodata region injected) walks the exact
dump scenario in one go: every click ACCEPTED, the zone reached in
about 50 seconds. The freeze lives in the interaction of the SHORT
first click with the server's own move machinery:
`Creature.moveToLocation` hands a geodata-collapsed click over to
the server side pathfinder only when `(originalDistance - distance)
> 30` - a collapsed click whose ORIGINAL line was under ~31 units is
silently canceled (ActionFailed, no movement, no feedback the
offline validation could see). The dump's 22 unit first waypoint
click was exactly that: collapsed by the user's server (whatever its
runtime state held against that line), un-rescuable by construction,
and re-clicked forever.

The bot's recovery then compounded the freeze: the re-path re-plans
from the standing position, so it reproduced the identical route
(the deterministic A*) with the identical un-rescuable first click -
three re-paths per trip, the abort, the zone return restart, the
same cycle again. Only after three full trip cycles
(zoneReturnFailBudget) would the direct server routed legs (whose
long clicks ARE rescue eligible) finally move the character - about
five minutes of the reported hang.

### Fix

1. `hunt/town.go`: `minWalkClick` (50) - the armed short click
   extension. After the first stuck that finds no clear successor
   (the pinned cursor - the proof the plain clicks do not move the
   character), the follower's clicks re-aim at the forward route
   samples (`extendShortClickCandidates`: the march along the plan
   polyline, one cell stride, skipping the samples under the floor
   or behind the character) - every armed click then clears the 30
   unit rescue threshold, so a server side collapse hands it to the
   server pathfinder instead of canceling it. The water guard and
   the click validation port gate every sample; a walled sample only
   skips forward (the next route cell carries the click). A waypoint
   behind the character with no validating forward sample holds the
   click (the backward click walked the character off the ground the
   extension just walked - the reproduction ping ponged on exactly
   that).
2. `hunt/town.go`: `frozenRepathLimit` - the identical re-plan rule.
   A re-path that started from the same cell as the previous one
   without a single cell of movement in between proves the fresh
   plan cannot move the character either; the trip aborts at once
   and the zone return escalates straight to the direct server
   routed legs (`abortFrozenTrip` sets `zoneFails` to the budget)
   instead of burning two more full trip cycles. The shop trips keep
   their cooldown recovery.

### Verification

- go build/vet, gofmt clean, golangci-lint zero findings, the full
  go test suite green (19 packages).
- The exact dump scenario under the freeze server model (a sim that
  cancels the sub-31-unit clicks the way the user's server did and
  walks or pathfinder-rescues the longer ones): the walk arrives at
  the zone within ONE recovery re-path, every armed click at least
  the floor length (TestReproRound57ShortClickFreezeWalksThePlan).
- The zone return sweep from eight village positions (the dump
  terrace cell, the round 56 aisle/pocket/terrace/plaza approaches,
  the shop deck, the southwest shore path) all arrive under the same
  freeze model within two re-paths
  (TestRound57ZoneReturnFromEveryVillageStart).
- The total-freeze escalation: a server that moves nothing aborts
  the trip after one no-movement re-path (two stuck windows instead
  of the dump's four per trip times the restart cycle) and the zone
  return escalates to the direct legs at once
  (TestReproRound57FrozenServerEscalatesFast).
- The extension gating pins: the march skips the backward and
  under-floor samples, the unarmed follower keeps the plain short
  waypoint clicks (the round 56 teacher ramp design), the armed
  follower extends them, the armed backward aim with no validating
  sample holds the click (click_floor_test.go).
- The live stack validation: the dump position injected into the
  test2 database row, the bot walked the plan to the zone with every
  click ACCEPTED on the MOVEDBG diagnostics build (both PathFinding
  modes), and `tools/mobius_e2e.sh` stays E2E_OK on the fixed build.

## Round 58: the engage freeze - the inherited frozen re-path and the unguarded direct zone legs (2026-09-12)

The user report (the 01:50 state dump, build 2149ad1, bot test1,
phase engage, uptime 10h32m): the character stood at (43048 50312
-2992, the elven village main street next to Herbiel) with the
hunting zone 7900 units away (the Kaboo Orc Fighter SW spot leash at
35214 51358, half 1448), the walk plan empty and the last 1h15m of
the event log holding nothing but the server pings - no hunt decision,
no movement, a single frozen position.

### Root cause (probed against the real geodata pack)

The event trail: a level 14 character started a deleveling at the
town guards, three guard deaths respawned it in the village, the
guard walk's first click failed the offline validation from the
respawn cell ("the server would refuse the walk click"), the round 57
frozen re-path rule aborted the deleveling, and the machinery started
the return leg to the farm spot. Two defects then chained into the
permanent freeze:

1. The return leg INHERITED the frozen re-path cell of the guard
   walk: `startDelevelReturnLeg` resets the trip timeout and the
   re-path budget but not `repathX/repathY/frozenRepaths` (only
   `endTownTrip` clears them, and the delevel abort path bypasses
   it), so the return leg's own first refused click read as the
   second frozen re-path from the same cell and `abortFrozenTrip`
   ended it within one second ("town trip ended: aborted, the server
   refuses the walk click from this cell") - the delevel walk and
   the return leg died back to back.
2. The `abortFrozenTrip` escalation armed `zoneFails` to the budget
   and the engage phase fell back to the direct zone legs
   (`walkZoneLeg`). The direct leg aims 1000 units toward the zone
   center - from the dump cell that line runs straight into the
   walled southern side of the street: the server click validation
   collapses it onto the walker, the move is silently canceled
   (ActionFailed, no movement, no feedback) and `walkZoneLeg` sent
   the same refused click once per second forever, without
   validation, without a stuck detection and without a single log
   line. The offline probe against the real geodata confirmed both
   halves: the direct leg click (43048 50312 -2992) -> (42056 50444
   -2992) is refused wholesale (the destination collapses onto the
   walker, 0 of 1000 units) while the geodata route out of the same
   cell exists and every click of it validates.

### Fix

1. `hunt/town.go` (`startReturnLeg`): the return leg is a fresh
   logical unit of the trip machinery - the frozen re-path cell of
   the guard walk (or the sell approach) dies at its start. The
   delevel abort path keeps its fast escalation for a genuinely
   frozen cell, but the handover never condemns the next leg.
2. `hunt/loop_movement.go` (`guardZoneLegClick`): every direct zone
   leg is validated through the server click port before it leaves.
   A refused leg is never sent; the refusal re-arms the pathfound
   zone return (`zoneFails` back under the budget, the next
   `returnToZone` plans a fresh geodata route whose first click the
   offline probe has already validated) and the paced log line
   ("the direct zone leg ... is walled, re-arming the pathfound
   return") explains the standing hunter in the dump. Without a
   navigator the legacy behavior stands.

### Verification

- go build/vet, gofmt clean, golangci-lint --new zero findings, the
  full go test suite green.
- The exact dump state against the real geodata pack (the dump cell,
  the dump zone, the post-abort escalation state, the faithful click
  server): the zone return walks out of the walled street pocket and
  arrives inside the zone with every sent click validated - the
  inversion of the dump signature (TestReproRound58VillageStuckCell
  WalksToZone).
- The guard pins: a refused direct leg is never sent and re-arms the
  pathfound return, a validated leg goes out and keeps the
  escalation state
  (TestReproRound58ZoneLegGuardRefusalReArmsPathfoundReturn).
- The frozen reset pins: the return leg starts with a clean frozen
  budget and survives its first refused click with an ordinary
  re-path, both directly and through the full delevel abort path
  (TestReproRound58ReturnLegResetsFrozenRepath,
  TestReproRound58DelevelAbortKeepsFrozenBudgetClean).
- The round 56/57 contracts stay green: the short click extension,
  the frozen re-path escalation of the total-freeze server and the
  follower reaction tests all pass unchanged (the reset moved to the
  leg boundary, the in-plan frozen detection is untouched).
- The `zone-return` acceptance scenario (the user runnable mirror of
  the report): the temp character temp4 starts at the dump cell
  (43048 50312 -2992) with the exact dump inventory (level 14, exp
  192206, sp 7549, adena 31857, the 25 stacks of the report - the
  injection lands everything in the bag and the auto equipment dresses
  the character from it), and the pass condition is standing inside
  its selected hunting zone. The live stack run of 2026-09-12
  (build a4c9e15 + the scenario) passed in 68 s: the gear dressed,
  the spot anchored (Spore Fungus SW), the village-to-zone route
  walked clean, a Kaboo Orc Fighter engaged on the zone entry. The
  scenario pins live in acceptance/zone_return_test.go (the dump
  reset values, the exact item set, the check list, the condition
  evaluation and the database item injection).

## Round 59: the phantom chase livelock - the refused attack that kept reading as a running fight (2026-09-12)

The user report (the 03:25 state dump, build a4c9e15, bot test1,
phase engage, uptime 7m13s): the level 14 character stood at
(35224 47288 -3656, the Spore Fungus SW spot) with the selected
target 268439361 (Kaboo Orc Fighter) at 35144 47288 -3656 - 80
units, the exact melee standoff - with "moving: no", "walk plan:
none", "in combat: true" and an empty combat feed, while the chat
window held "Cannot see target." every ~3 s for over a minute and
the event log showed nothing but a Power Strike cast every 15 s. The
bot neither moved, nor switched the target, nor landed a blow.

### Root cause (traced through the Mobius AI and the tracker apply path)

The event trail: the previous kill finished at 03:24:12, the picker
selected the aggressive Kaboo that had stalked the fight, the forced
attack armed the server side ATTACK intention. From there a server
cycle ran forever:

1. `PlayerAI.thinkAttack` -> `maybeMoveToPawn` -> `startFollow`:
   the chase of the armed intention broadcasts the character's OWN
   MoveToPawn steps, and `applySelfPawnMovementLocked` refreshes
   `CombatActiveAt`/`FightingTargetID` on every one of them (the
   steps are zero distance at the standoff - "moving: no" in the
   dump while the fight view stays fresh).
2. `Creature.doAttack` fails the `canSeeTarget` geodata check and
   answers SystemMessage 181; `setIntentionActive` disarms the
   intention (the chase stops, the view ages out after the 3 s
   fightingFreshWindow).
3. The loop's 1 s paced re-request re-arms the ATTACK intention -
   back to step 1. The observed ~3 s refusal cadence is exactly
   this cycle (the 3 s view staleness + the 1 s pacing).

While the phantom chase held `SelfFighting` true, the engage branch
re-anchored `engageAt` on every tick (loop.go, "the fresh fight also
re-anchors the engage clock"), which held the 12 s engage stuck
timeout away forever; the `blindEngageBlocked` detection died at its
`SelfFighting` gate and at `cannotSeeAt.Before(l.engageAt)` (the
refusals always landed before the newest re-anchor); the blind
engage recovery - the mechanism built for exactly this refusal -
never armed. The bot stood 80 units from a mob it could not see,
casting Power Strike at it every 15 s, forever.

### Fix (the refusal-vs-activity ordering rule)

1. `state/bot.go` (`SelfCombatActiveAt`): the raw timestamp of the
   last fight activity (a swing attempt or a chase step), so the
   loop can order it against the last refusal.
2. `hunt/loop_los.go` (`blindEngageBlocked`): the `SelfFighting`
   early-out is gone; the block is detected when a fresh refusal of
   the current attempt is the NEWEST fight activity - nothing
   landed or stepped after the server said "cannot see". A fresh
   chase view alone no longer clears the block (the phantom chase
   of the refused attack produces exactly that view); a chase step
   or swing strictly newer than the refusal (the mob walked past
   the obstacle edge, the walk cleared the sight line) still does.
3. `hunt/loop_los.go` (`fightClearedRefusal`, used by
   `recoverBlindEngage`): the standdown of a running recovery
   requires the same progression past the refusal, not merely the
   fresh chase view - otherwise the recovery would arm and stand
   down in a loop through the phantom chase.
4. `hunt/loop.go` (the fighting branch): the engage clock
   re-anchor requires the same progression, so the phantom chase
   can no longer slide the clock past every refusal. The plain
   stuck timeout (the no-refusal stale stance mode) and every other
   fight behavior are unchanged.

### Verification

- go build/vet, gofmt clean, golangci-lint --new zero findings, the
  full go test suite green (the pre-existing full-lint findings of
  the branch are untouched files).
- The exact dump scene replays and pins the three halves
  (hunt/round59_repro_test.go): the phantom chase arms the recovery
  and walks the first reposition leg on the arming tick
  (TestReproRound59PhantomChaseArmsBlindRecovery); the persisting
  refusal spends the attempt budget and ends in the target switch
  with the skip list holding the Kaboo out and the next pick taking
  the Spore Fungus of the dump object list
  (TestReproRound59PhantomChaseSwitchesTarget); the phantom chase
  no longer re-anchors the engage clock while a fight that truly
  progressed past the refusal still does
  (TestReproRound59EngageClockHoldsPastRefusal).
- The round 56/57/58 contracts and the whole loop_los_test.go suite
  pass unchanged: the fresh fight guard (a chase AFTER the refusal
  reads as a running fight), the stale refusal scoping, the stuck
  timeout through the stale attack stance, the timeout hold during
  the recovery walk.

## Round 60: the pantsless return - the town trip that sold the legs armor and never bought it back (2026-09-12)

The user report (the 04:58 state dump, build 4deb888, bot test2,
phase engage, uptime 1m44s): the level 14 elven fighter returned
from its town trip WITHOUT the legs armor - the equipment held 11
pieces (Sickle, Buckler, Leather Shirt, Wooden Helmet, Gloves,
Leather Shoes, the basic jewel set), every slot filled except the
legs, the bag held nothing but the 13162 adena, and the event log
closed with "Hunt: town trip ended: back at the farm spot" ten
seconds before the dump. The session had started at the village
(the previous session died mid trip; the loop state survives the
relogin, so the return leg resumed and finished as a success). The
state dump itself could not even show the wound: the equipment
section prints only the occupied slots, so the missing legs line
was invisible.

### Root cause (the sell first step is one-way)

The town trip machinery banks the sell credit of displaced
equipped pieces BEFORE the replacement buy runs (the planner
counts the credit toward the budget so a character shops for a
replacement as soon as the adena plus the proceeds cover it). Every
exit path between the sale and the buy strands the slot silently:

1. The buy requests the server silently refuses (the transaction
   flood window, a lost adena update, a selection reset) never
   deliver the items; the stop re-requests the batch three times
   and then SKIPS it ("purchases never arrived, skipping them") -
   the purchase dies with the sold piece already gone.
2. A merchant that never showed up (or stands on another deck)
   resets the stop buys the same way.
3. An attacker interrupt mid trip drops the whole trip
   (resetTownTrip) - the sold pieces stay sold.
4. A walk abort, the trip timeout, a session death (the exact dump
   path: the relogin landed in the return leg) - the trip ends as
   a success without the replacement.

`endTownTrip` cleared the trip state happily and armed the five
minute ordinary trip cooldown; nothing compared the paperdoll the
trip started with against the paperdoll it reached, so the bot
farmed the Kaboo woods half dressed with no diagnostics line and
no priority. The weapon had its dedicated weaponless run (the
short 45 s cooldown and the trip justification of Rule 2a); the
armor slots had nothing.

The planner itself was never broken: against the exact dump state
(legs empty, 13162 adena) it plans the Leather Pants filler
(1747 adena with tax) plus the Wooden Breastplate - the recovery
path existed, it just was neither prioritized nor visible.

### Fix (the gear debt - the trip answers for what it stranded)

1. `hunt/town.go` (`snapshotTripGear`, called from
   `maybeStartTownTrip`): the trip start snapshots the paperdoll
   object ids and item ids it begins with.
2. `hunt/shopping.go` (`gearDebtCheck`, called from every trip
   exit - `endTownTrip` and the interrupt/death `resetTownTrip`):
   every slot that was occupied at the start, sits empty now and
   whose piece is gone from the inventory becomes gear debt - the
   map entry carries the lost item id. A piece that still sits in
   the bag is no debt (the auto equipment re-wears it). The arming
   logs the wound for the state dump reports: "the trip left the
   legs slot empty - the Leather Pants it started with is gone
   (sold for a replacement that never landed)".
3. `hunt/shopping.go` (`gearDebtRunWanted`): the armed debt
   shortens the trip cooldown to the gear run window (the 45 s
   weapon run cooldown - farming without the armor the merchant
   sold is the same wound the weapon run answers for bare hands).
   The debt does NOT justify a trip by itself: a broke character
   cannot buy the filler anyway, the ordinary triggers fire the
   moment the plan affords it - on the short cadence. The refilled
   entries clear with a log line ("the legs slot is dressed again,
   the gear debt clears"), so the priority never outlives its
   wound.
4. `webserver/dump.go` (`dumpEmptySlots`): the state dump
   equipment section lists the EMPTY paperdoll families below the
   worn pieces ("empty slots: legs, ..."), so the next pantsless
   report shows the hole at a glance instead of hiding it.
5. The trip start reason appends "(the gear debt refill)" while a
   debt is armed, so the refill trip names itself in the log tail.

The trip justification and the shopping planner are unchanged: the
debt only hurries the cooldown, the frozen trip plan and the
one-item-per-slot-per-trip invariants keep their semantics.

### Verification

- go build, gofmt clean, golangci-lint --new zero findings, the
  full go test suite green (19 packages).
- The exact dump state replays and pins the machinery
  (hunt/round60_repro_test.go): the full stranding flow (the sell
  first sale lands, the buy requests never arrive, the retry
  budget skips them, the return leg ends the trip) arms the debt
  with the log line and the short cooldown
  (TestRound60DebtArmsOnStrandedReplacement); the debt runs the
  refill trip whose plan carries the Leather Pants
  (TestRound60DebtRunsTheRefillTrip); the refill clears the debt
  (TestRound60DebtClearsOnRefill); the interrupt exit
  (resetTownTrip) arms the debt of the already sold piece
  (TestRound60ResetTownTripArmsDebt); a fresh loop with no trip
  history refills the dump state through the ordinary trigger
  alone (TestRound60FreshLoopRefillsTheDumpState).
- The planner side of the dump state is pinned in
  gear/round60_repro_test.go: the affordable prefix plans the
  Leather Pants for the empty legs slot of the 13162 adena wallet
  and the slot model of the dump reconstruction is correct.
- Live verification against the deployed Mobius stack (the new
  "gear-gap" acceptance scenario, acceptance gearGapReset): the
  temp5 character wakes at the reported farm spot (38344 46248
  -3592) as the exact dump character - the 11 piece paperdoll
  minus the legs, the 13162 adena, nothing in the bag - and the
  run must dress the legs slot again. Observed: the plan triggered
  the town trip, the sell first sold the displaced Leather Shirt,
  Creamees sold the spellbook, Ariel bought "Leather Pants, Wooden
  Breastplate" (list 3014800) and the auto equipment equipped
  "Leather Pants (27) into the empty legs slot" - PASS.
=======
## Round 61: the trainer hall entry - the frozen corridor ban, the close teacher ring and the delevel median agreement (2026-09-12)

The user reproduced the 2026-09-12 03:56 hang locally (build 6a2ac91,
bot temp1, phase townWalk): the learn leg to the teacher Ellenia froze
at the trainer hall west aisle entrance (44728 51992 -2792) - the
geodata plan entered the building through the aisle column (44728
51992 -> 44728 52040 -> 45160 52120), the 48 unit click into the
aisile moved the character nowhere while the ported click validation
accepted it in full, and the deterministic re-path reproduced the
identical route until the budget aborted the trip. The user rule that
came with the report: the character must walk from the building
entrance right up to the training npc, not talk to it through the
wall from wherever the leg happened to end.

The live probe against the sandbox stack (the aisle cell, the exact
dump clicks) walked the whole route clean - the sandbox server runs
without geodata regions and accepts every click - which localized the
freeze to a server whose geodata disagrees with the pack at the aisle
cells. The fix therefore had to make the recovery work on any server
configuration, not just the sandbox.

Root causes found and fixed:

- The frozen re-path had no escalation for town walk legs: the
  identical deterministic route was re-planned forever. The escalation
  ladder now climbs one rung per frozen abort of the same leg: (1) the
  aimed waypoint's cells join the session's avoid areas (a new
  FindPathApproachDryAvoiding search with banned patches - the A*, the
  direct line shortcut and the smoothing all refuse the banned ground)
  and the leg re-plans the detour around the corridor (the trainer
  hall route goes north over the terrace and east past the hall); (2)
  the follower drops the plan and clicks the stop target directly by
  the server's own routing (the npc approach point of the stop, the
  water guard and the aggro steering stay on), bounded by a 45 s
  window; (3) the plain trip abort with its cooldown. The zone return
  keeps its existing escalation (the direct zone legs).
- waypointBehindRoute misjudged V-shaped detour routes: a far waypoint
  the route doubles back from (the north climb of the hall detour)
  projected "behind" the character although it sat 900+ units ahead,
  and the short click extension then re-aimed the perfectly good
  climb click at far route samples whose straight lines cross the
  terrace walls - the freeze the walled aisle reproduction exposed.
  Only a waypoint the character stands near counts as behind now.
- The teach stop ended its walk on the wide 200 unit approach ring and
  talked through the wall from there. The teach legs now search their
  route within the close ring (npcApproachOffset, 150) with the wide
  ring as the fallback, the tight legs complete their route end with
  the pass radius instead of the wide trip slack, and the teacher
  approach walks the npc approach point ring before the talk (the
  approach window bounds the walk - a ring the server routing refuses
  still talks from within the interaction distance, the 05:45 rule).
- The delevel trigger fired on the flickering live zone median: the
  building entry acceptance anchored Kaboo Orc Fighter SW (static
  median 9, gap 6 - safe) and de-leveled a healthy level 15 anyway
  when the respawn window left only the level 7 species alive. The
  trigger now requires the static median of the anchored spot to agree
  with the live one, and the spot picker skips grounds whose static
  median sits at the delevel gap (the spot window admits level-8
  grounds the deleveling immediately answers with guard deaths).

The regressions and the acceptance:

- The offline reproduction (hunt/building_entry_test.go) models the
  freeze server with the walled aisle patches on the repro server sim:
  the agreeing server walks the aisle plan clean and talks at 149
  units; the walled server freezes exactly like the dump, climbs the
  ladder (the ban, the detour), and talks within the interaction
  distance. The pathfind ban tests pin the detour shape, the empty
  ban equivalence and the sealed goal refusal.
- The `building-entry` acceptance scenario (the user runnable mirror
  of the report): the temp character temp5 wakes at the dump aisle
  entrance with the dump's town visit start state (level 15, 20,000
  SP, 100,000 adena, the empty inventory - the dump's own weapon run
  trip arms at once, its exact first log line reproduces), buys the
  gear and the spellbooks across the village merchants and walks the
  teacher leg through the building entrance right up to Ellenia. The
  live stack run of 2026-09-12 passed in 2 m 10 s: the weapon run,
  the eleven purchases across Ariel, Unoren and Creamees, the teacher
  leg reached Ellenia inside the hall and the first lesson (Power
  Strike level 1) consumed its SP.

## Round 62: the hypotheses registry - the unverified assumption becomes a tracked artifact (2026-09-12)

The self-organization round (agent_progress 2026-09-12) added the
boot prompt rule "an unverified assumption goes into the hypotheses
registry with a verification plan" - but the registry itself did not
exist: the open server questions of the navigation work (the swim
semantics, the gatekeeper graph, the boats, the doors) lived as prose
inside docs/navigation_analysis.md with no ids, no tracking and no
obligation for relying code to reference them. An agent reading
AGENTS.md had no way to tell a verified server fact from a hopeful
guess, and the T-004 quest research round was about to mint a fresh
set of protocol assumptions with the same problem.

The fix is the convention plus the seed (T-005, scope: AGENTS.md and
docs/navigation_analysis.md only):

- AGENTS.md carries the "Hypotheses and unknowns" section: every
  unverified server assumption becomes an H-NNN registry entry
  BEFORE the relying code is written (a comment on the relying line
  references the id), every entry names its evidence (the Mobius
  Java classes to read, the live experiment to run), and running the
  plan closes the entry - a confirmed fact moves into the matching
  subsystem doc, a refuted one records what the server actually did.
- The registry is seeded with the four open navigation unknowns:
  H-001 the swimming semantics (the 60 s breath of
  Player.startWaterTask, the WaterTask damage, the swim speed
  defaults), H-002 the gatekeeper teleport graph (the
  Teleporter.onBypassFeedback bypass chain, RequestBypassToServer,
  the teleporter xml shape), H-003 the boats as scheduled edges (the
  three vehicle scripts, the GetOn/GetOff vehicle packet family),
  H-004 the doors as passable obstacles (the Door openable
  families, DoorStatusUpdate). Every class named in a verification
  plan was located in the local Mobius checkout before the entry
  was written - the plans reference real files, not guesses.
- docs/navigation_analysis.md links its open items to the registry
  ids so the analysis doc and the registry agree both ways.

Verification: docs-only change - `go build ./...` green,
`golangci-lint run --new` clean (0 issues), the stack was up
(STACK_READY login :2106 game :7777 db :3306, 75 tables) for the
source reading; no behavior changed so no e2e run was required. The
follow-up the convention itself demands: the next pathfind change
adds the H-001 reference to the waterCostMultiplier comment (outside
the docs scope of this round, noted in the BACKLOG resume notes).

## Round 63: the soak metrics trail - the M1 acceptance vehicle (2026-09-12)

The self-organization round named the M1 acceptance ("a fresh account
runs -hunt for 8 hours, runs/metrics.jsonl receives one row per run,
the stagnation watch fails the livelock") but the vehicle did not
exist: no scenario drove a long supervised farm run, no metrics
trail recorded the outcome, and a silent livelock (no XP, no move)
had no acceptance-level gate. The soak scenario, the metrics writer
and the stagnation guard close that gap (T-001, scope:
internal/swarm/acceptance/, cmd/swarm/, tools/, docs/).

- The `soak` acceptance scenario (internal/swarm/acceptance/soak.go):
  a fresh level 1 elven fighter temp7 enters the world under the
  supervised hunt loop (the same runSessionSupervised the 24/7 fleet
  supervisor uses, so a lost session reconnects with the growing
  backoff) and farms the elven lands for SWARM_SOAK_MINUTES (default
  the 10 minute smoke; the real M1 proof sets 480). The check list
  (entered the world, stayed online the window, never stagnated, shut
  down gracefully) drives the pass verdict; the scenario splits into
  soakSetup (the ensure, the session launch, the world wait, the
  start-state capture), soakMonitor (the guard + check loop) and
  soakAwaitOutcome (the session teardown + the metrics write) so the
  funlen gate holds.
- The stagnation guard (soak_guard.go): a tracker-public-API read of
  the cumulative experience (level plus the within-level exp, through
  a local copy of the C1 experience table) and the floored position.
  The guard fires when the experience holds for M minutes
  (stagnationNoXpMinutes = 10) or the position holds for K minutes
  (stagnationNoMoveMinutes = 5), after a 90 s startup grace (the
  world entry settles before the thresholds count). The now seam
  (a func() time.Time) lets the unit tests drive the clock; the
  production path uses time.Now. The guard is single-goroutine (the
  soak monitor owns it), so it never races the tracker lock. The
  scope stays inside the acceptance package on purpose: T-002 (the
  stagnation watch) will later surface the same events from the hunt
  loop and the soak scenario can consume them.
- The metrics trail (soak_metrics.go): one JSON line per run, the
  fields the M1 contract names (date, scenario, durationSec,
  startLevel, endLevel, xpPerHour, deaths, adena, stuckEvents,
  status, failReason). The writer opens runs/metrics.jsonl with
  O_APPEND and writes one marshalled row, so parallel runs never
  interleave. The XP rate is the cumulative-experience delta over the
  duration hours; the cumulative XP uses a local copy of the C1
  experience table (byte identical with
  internal/swarm/state/experience.go) so the acceptance package never
  crosses into state (T-002 and T-003 territory). The deaths are the
  alive->dead rising edges the deathEdgeTracker counts off
  SelfDead(); the stuck events are the hunt loop re-path count delta
  (each re-path is a stuck-and-replanned leg, read off the snapshot
  diagnostics).
- The progress report (tools/progress_report.sh): renders PROGRESS.md
  from the metrics tail, the BACKLOG task statuses and the last 20
  commits. The M1 milestone line colors green on the last soak PASS,
  red on FAIL, pending otherwise. runs/README documents the schema.

Verification: the unit tests cover the guard (the no-XP fire, the
no-move fire, the healthy never-fire, the startup-grace hold), the
metrics writer (the atomic single-line append, the JSON field
names), the XP math, the death edge tracker, the duration env and
the cumulative XP table - `go test ./internal/swarm/acceptance/`
green, `golangci-lint run --new` clean (0 issues). The live smoke run
(SWARM_SOAK_MINUTES=2 against the deployed stack) PASSED: temp7
farmed level 1 to 2 in 120 s (9984 XP/h, 0 deaths, 0 stuck events, 28
adena), the bot left the world gracefully, and runs/metrics.jsonl
received the PASS row. The 8-hour M1 proof is a follow-up operator
run (the machinery is duration-agnostic).

## Round 63: the 20-25 band survey - the ladder beyond the elven lands and the aggression default correction (2026-09-12)

The M3 preparation needed the ground truth of the first band beyond
the elven ladder: which 20-25 grounds exist, how a character reaches
them from the elven lands and what services them. The elven survey
knowledge lived scattered across hunting.md and the generated
registry, and nothing mapped the next band - the ROADMAP M3 entry
("zone registries for the 20+ grounds reachable from the elven
lands") had no data to build on.

The survey (T-006, scope: docs/ and tools/generate_hunt_zones.py):

- tools/generate_hunt_zones.py grew the `--survey MIN MAX` mode: it
  scans every spawn territory of the data pack for the band mobs,
  joins the npc stats (level, hp, exp, the ai block) and prints the
  grounds with the walking distance from the teleport network
  arrival points. The default registry mode is untouched (verified:
  the regenerated zones_elven.go is byte identical).
- docs/band_20_25_survey.md: the transport reality (no 20-25 mobs in
  the elven lands at all - the band lives 84k+ units out and rides
  the Mirabel -> Bella -> Trisha gatekeeper chain, 13 600 one way),
  the four ground families (the Execution Grounds mandragora ladder
  20-25 with a passive start, the magic-fighting mist leeches of the
  Cruma edge, the mixed plains grounds, the wasteland watchers), the
  Dion merchant counters (Sabrin/Casey/Sonia/Lara with their
  buylists), the teacher economy (the elven village masters stay the
  teachers - a 25 200 adena lesson round trip) and the spellbook gap
  (Cure Bleeding is not sold in the elven village; Dion's Sonia has
  it).
- The follow-up list became the backlog tasks T-008 (the gatekeeper
  teleport flow), T-009 (the band zone registry) and T-010 (the band
  gear catalogs); the rest (the walk leg verification, the book
  routing, the band acceptance scenario) is documented in the survey.
- The survey also refuted a "verified" claim of
  docs/navigation_analysis.md: the C1 monsters do NOT "never attack
  on sight" - NpcTemplate fills isAggressive TRUE when the xml
  omits the attribute, so the mandragoras and dire wolves of the
  band (and the Kaboo Orc Fighter of the elven lands, as AGENTS.md
  already recorded live) attack on sight; only the explicit
  isAggressive="false" mobs (the leeches, the gazers) ignore a
  passing walker. The danger model section now carries the corrected
  reading with the Java line reference.

Verification: the survey numbers come from the live Mobius checkout
the stack runs (the spawn files, npc stats, teleporter xml); the
generator regression is byte identical output;
`go build ./...` green, `golangci-lint run --new` 0 issues; docs +
tooling only, no bot behavior changed, no e2e required. The
per-mob aggression readings were double-checked against
NpcTemplate.java and the npc xml directly.

## Round 64: the hunt loop stagnation watch - a silent livelock becomes a loud line (2026-09-12)

The M1 milestone (ROADMAP: the soak proof, level 1 to 20 unattended)
demands that no livelock survives silently: "the stagnation watch
(no XP gain for M minutes, no position change for K minutes) logs
explicit events and fails the scenario when it fires". The freeze
history of the project proves the need - the round 58 stuck cell
held a character on one village cell for over an hour (the dump was
the only signal), the round 60 pantsless bot farmed half dressed for
an hour, and every one of those reports arrived through a manual
state dump instead of a live log line. The hunt loop had watchdogs
for the narrow cases (the engage stuck timeout, the walk stuck
re-path) but no global progress watch: a character that stopped
farming without tripping any single-phase watchdog looked exactly
like a healthy one in the bot log.

The fix is the watch itself (T-002, hunt/stagnation.go):

- Two named windows calibrated against the measured honest flows:
  stagnationXPWindow (20 min - the longest legit experience stall is
  the full town trip round, measured 2.2 min for the weapon run and
  14m54s for the whole farm readiness round) and
  stagnationPositionWindow (10 min - walks, fights and death cycles
  all move the character; the freeze dumps stood on one cell for an
  hour).
- The observation hooks the tick publish defer in Loop.tick, so it
  runs on every tick whatever early return the state machine took
  (the death recovery, the emergency logout, the phase dispatches).
- One event line per window (the timer re-arms after firing, a
  permanently frozen character logs its stall every window instead
  of every 250 ms tick), the line carries the loop phase and the
  held value ("Hunt: stagnation: no experience change for 21m0s,
  exp 1000 held, phase engage" / "Hunt: stagnation: position held
  11m0s at 43048 50312 -2992, phase townWalk"), and it routes
  through Loop.logf: the console log, the tracker event log and the
  web UI log tab all carry it.
- The watch stays quiet and resets outside the autonomous online
  session: a manual only session may stand still legitimately, an
  offline gap (the supervisor relogin) re-arms the windows fresh -
  neither fires an event nor inherits a stale baseline.
- The state dump diagnostics gained the stall ages (xpStallForMs,
  positionStallForMs in state.HuntDiagnostics + the snapshot
  encoder): a freeze report now shows how long the experience and
  the cell have been static at a glance, next to the phase age the
  dump already carried.

Verification: 9 unit tests (hunt/stagnation_test.go) through the
seeded-baseline clock seam of the loop tests - the fire, the re-arm
(the immediate repeat stays quiet), the refresh while farming and
while moving, the manual/offline quiet with the reset, the tick path
coverage through the early returns (the dead character tick still
publishes), the event feed surface (the tracker event log carries
the line) and the diagnostics wiring (the stalls report and reset).
go build, go vet, the full test suite, gofmt and
golangci-lint run --new (0 issues) green; the full golangci-lint
reports only the pre-existing findings of the branch. Live: the
tools/mobius_e2e.sh 45 run printed E2E_OK (the manual session
regression), and a 60 s autonomous -hunt smoke run against the live
stack killed a mob with the watch armed - the live snapshot
diagnostics carried the new fields tracking the real progress
(xpStallForMs following the kill, positionStallForMs following the
loot walk) and zero stagnation lines on the healthy run, with the
graceful SIGINT shutdown. The positive fire (a real 10/20 min
freeze) is owned by the M1 soak acceptance runs where the windows
are the pass criteria.

## Round 65: the PROGRESS.md page - the ladder renders from the roadmap (2026-09-12)

The T-001 soak round delivered the metrics trail and the
progress_report.sh renderer, but its milestone section was
hardcoded to a single M1 line - the task T-007 acceptance asks for
the "milestone ladder green/red by the last acceptance results", and
the renderer could not build a ladder. The human dashboard also
existed only as a script, not as a committed page (the task: the
page is committed after every milestone-relevant run so the page
history is the project history).

The fix (T-007, scope: tools/ and docs/):

- render_milestone now parses docs/ROADMAP.md (the `## M<N> - title
  (DONE)` headings) and walks the whole ladder: a DONE heading
  renders done (M0), the first open milestone is the current one and
  colors green/red by the last metrics row (PASS/FAIL), the rest
  render pending. The section header became "Milestone ladder".
- The first PROGRESS.md page is committed: M0 done, M1 green (the
  07:45 smoke soak PASS row of runs/metrics.jsonl - the 8 hour
  acceptance run will overwrite the verdict row when it runs), the
  ten BACKLOG tasks and the last 20 commits.

Verification: `bash tools/progress_report.sh` re-rendered the page
and the ladder matches docs/ROADMAP.md (M0-M6, one current, five
pending); `go build ./...` green and `golangci-lint run --new` 0
issues (no Go code touched - the renderer is bash + python3);
no behavior change, no e2e required.

## Round 66: the quest subsystem research - the M2 protocol map before the code (2026-09-12)

Problem: the M2 milestone (the first profession, the class transfer
at level 20) needs the quest machinery - NPC dialogs, bypass
commands, quest state, quest items - and none of it was documented
or understood: the bot drops every dialog packet it receives
(0x1B) and never sent a single bypass (0x21). Starting the quest
code from guesses would burn the next sessions on protocol
rediscovery (the exact failure mode the verification pyramid of
docs/agent_selforganization.md exists to prevent).

Root cause: pure knowledge gap - the quest engine, its packets and
the two elven class transfer quests had never been read against the
Mobius C1 sources.

Fix (T-004, docs only):

- docs/quest_protocol.md: the quest engine (Quest/QuestState/State,
  the CREATED/STARTED/COMPLETED machine, the character_quests
  persistence), the dialog flow with the packet layouts (Action
  0x04, RequestBypassToServer 0x21 with the command grammar,
  NpcHtmlMessage 0x1B, QuestList 0x98, PlaySound 0xB1, MoveToPawn
  0x75), the html action cache (the anti-injection gate: only the
  links of the currently open page validate, 250 units of the
  origin npc, the $-parameter prefix), the quest events (the 2.5 s
  ON_ATTACKABLE_KILL delay, the dialog gates), the Q00406/Q00407
  chains walked end to end with the npc positions, the class
  change script at Rains 30288 and the gatekeeper geography
  (Mirabel -> Gludio 9200a, Bella -> Gludin 7300a).
- The follow-up code tasks are listed in the BACKLOG resume notes
  (the dialog stream parser, the quest journal parser, the bypass
  sender with the cache mirror, the hunt quest module, the M2
  acceptance scenario); the T-008 packet work of soak-z already
  covers the first two items.

Verification: the sources were read in the local Mobius checkout
(every class named in the document was opened); two live facts were
traced against the deployed stack (SWARM_TRACE_PACKETS=1): the
QuestList 0x98 (5 bytes, empty) pushes at world entry without any
client request (EnterWorld.java:303), and the merchant sell round
streams NpcHtmlMessage 0x1B (813 bytes, about one per second) that
the bot currently drops. `go build ./...` green,
`golangci-lint run --new` 0 issues (no Go file touched).

## Round 67: the quest journal lands in the tracker - the 0x98 push stops dropping (2026-09-12)

Problem: the server pushes the quest journal (QuestList 0x98) at
world entry and after every quest state change, but the bot's
dispatcher had no case for the opcode - the packet fell into the
unknown-packet log line and the quest state of the character was
invisible to every layer (the M2 quest brain has nothing to read).

Root cause: the quest protocol was researched only minutes before
(the round 66 research); no code existed for any quest packet.

Fix (T-011):

- packets/from_game_server/quest_list.go: the parser (the quest
  count + [questId][state] entries, the item count + the
  [objectId][itemId][count][bodyPart] stacks), the count caps
  (64 quests - the engine itself refuses more than 25 started, 256
  items) and the reusable entry buffers (the ParseXxx packet struct
  is a field of the game client, so the buffers reset, not grow).
- state/quests.go: the journal maps (quest id -> the cond or
  completion flags int, quest item id -> count) with the
  whole-list replacement semantics (the server always resends the
  full journal, the same contract as the skill list), the accessors
  for the hunt loop (QuestCond, QuestCount, QuestIDs) and the
  quest-item answers for the future sell filter (IsQuestItem,
  QuestItemCount); ResetSession clears both maps - the journal is
  session state the server repushes at world entry.
- connection: the 0x98 dispatcher case, applyQuestList and the
  convert helpers that copy the parsed entries into the state view
  without aliasing the reused parse buffer.

Verification: 7 parser unit tests (the golden empty form is the
live 5 byte packet of the round 66 trace, the populated form is the
two class transfer quests with the topaz/emerald stacks) and 4
state unit tests (the journal pins, the replacement, the sorted
accessor, the session reset); go build ./... green, the three
touched packages' tests green, golangci-lint run --new: 0 issues;
live against the deployed stack - the fresh trace3 bot logs "Quest
journal with 0 quests, 0 quest items" at the enter world second,
proving the 0x98 push now parses and lands.
## Round 66: the level milestone scenario - the building block of the milestone acceptances (2026-09-12)

Every milestone acceptance from M2 on needs the same vehicle: a
character injected at an arbitrary level with the zone-appropriate
start that must reach the next level within a budget. The farm
readiness scenario hard-coded level 15 and its own kill condition;
nothing generalized, and the near-threshold trick (start the
character one short stretch of kills below the level-up) did not
exist - so a level milestone acceptance would either burn tens of
kills per run or need its own reset plumbing.

The fix is the scenario (T-003, scope: internal/swarm/acceptance/):

- The `level-milestone` scenario: the temp8 character injected at
  level N (the SWARM_LEVEL_MILESTONE_LEVEL env knob, default 10,
  valid 1..20) with the near-threshold experience (one twentieth of
  the level span below the N+1 threshold), the template vitals of
  the level, the farm readiness wallet (100k adena, 20k sp, the
  empty bag - the shopping strategy dresses the character for its
  band) and the creation spawn point (the zone ladder walks it out).
  The pass line is the observed level N+1: the UserInfo broadcast of
  the Mobius level-up (Player.levelUp -> broadcastUserInfo) refreshes
  the tracker level the monitor reads.
- The elven fighter vitals table (levels 1..20, floored hp/mp/cp)
  read from the Mobius player template
  dist/game/data/stats/players/templates/StartingClass/ElvenFighter.xml
  (lvlUpgainData): the injected database row agrees with the values
  the server would have leveled the character along (the level 15
  row matches the farm readiness constants 280/111/112).
- The run appends one metrics row to runs/metrics.jsonl on every
  outcome (the PASS row, the session teardown fail, the setup fail),
  reusing the T-001 trail shape so the level milestone runs and the
  soak runs accumulate in one place.

The round also fixed a live-found bug of the young metrics trail:
the level 10 live run reported xpPerHour 187169 for a real 1266 xp
gain. The cumulative XP helper of the acceptance package added the
level start threshold on top of the exp value
(table[level-1] + exp), but the Mobius exp is the running TOTAL
(PlayableStat.addExp does setExp(getExp()+value) and derives the
level by walking the experience table, UserInfo broadcasts
(int) player.getExp() - both read in the local Java checkout), so
the helper double counted the whole start-level span. The fix
returns the exp as the cumulative value; the historical rows stay
as written (the trail is append-only, the note lives in
runs/README.md).

Verification: 7 unit tests (acceptance/level_milestone_test.go) -
the reset construction (the near-threshold exp strictly inside the
last twentieth of the span, the vitals, the wallet, the spawn), the
tail scaling for every injectable level, the env knob matrix (16, 1,
20, the out-of-range and malformed fallbacks), the check evaluation
(distance detail, the reached flip), the check list, the vitals
table against the template constants and the definition
registration; the manager temp account contract extended to temp8.
go build, go vet, the full acceptance suite, gofumpt and
golangci-lint run --new (0 issues) green. Live against the deployed
stack: the default run (level 10 -> 11) PASSED in 7m46s (466 s, 0
deaths, 0 stuck events, the full weapon run round, the band walk
and the leveling kills), the env-parameterized run (level 2 -> 3)
PASSED in 2m30s with the corrected metrics row (2111 xp/h against
the real 88 xp gain - no phantom span), both rows in
runs/metrics.jsonl.


## Round 68: the dialog links leave the html - the walker gets its eyes (2026-09-12)

Problem: the dialog pages the server streams (one NpcHtmlMessage
per second during a merchant round, the quest and class transfer
pages of M2) carry the actions as raw html anchors; without the
link extraction the dialog walker (T-008's teleport buttons, the
M2 quest pages) would have to regex the html inline - duplicated,
untested parsing in every consumer.

Root cause: the research round defined the follow-up but only the
packet parser (the raw html string) had landed (T-008).

Fix (T-012): packets/from_game_server/html_links.go -
ParseHTMLLinks mirrors the server side scan of
HtmlUtil.buildHtmlBypassCache (the case-insensitive "=\"bypass "
attribute match, the command to the closing quote, the "-h "
prefix strip, the trim) and adds the link text the walker matches
pages by; the unterminated attribute ends the scan, a 128 link cap
bounds the pathological page.

Verification: 8 unit tests on the real page forms (the Sorius
quest page, the Rains class master page with the class change
command, the $ parameter, the case-insensitive attribute); a
datapack cross-check - the parser walked every Q00406 script page,
every ElfHumanFighterChange1 master page and the Mirabel teleporter
page (112 links, commands and texts exact; the %objectId% token of
the file form is the raw placeholder, the packet form arrives
resolved). go build ./... green, the package tests green,
golangci-lint run --new: 0 issues.

## Round 69: the open dialog lands in the tracker - the bypass cache mirror exists client side (2026-09-12)

Problem: the dialog pages stream in (one per second during a
merchant round) but nothing stores "the page that is currently
open" - and the bypass sender must never fire a command the open
page did not offer (the server's html action cache drops unmatched
commands silently, the anti-injection gate of the quest research).

Root cause: the parse chain (the packet, the links) had just landed
(T-008 partial, T-012) but the tracker half of the NPC_HTML scope
mirror did not exist.

Fix (T-013): state/dialog.go - the open dialog page (the npc
origin the 250 unit interaction check rides on, the links with
their texts), replaced whole on every page arrival (the server
cache clears and re-caches per scope on every NpcHtmlMessage),
cleared with the session; IsDialogCommand mirrors the exact match
of Player.validateHtmlAction plus the variable parameter rule (the
'$' marker stripped, the remainder trimmed, the prefix matched -
the Java compares substring(0, len-1).trim(), the first draft
forgot the marker strip and the test caught it).

Verification: 7 unit tests (the real quest page form, the page
replacement, the parameter prefix rule with its near-miss, the
defensive copy against aliasing, the session reset, the explicit
clear); go build ./... green, the state and packets package tests
green, golangci-lint run --new: 0 issues. The 0x1B dispatcher
wiring that feeds ApplyDialog stays in T-008's scope (connection/).

## Round 64: the gatekeeper teleport packet chain (T-008, 2026-09-12)

The M3 band survey (T-006) named the gatekeeper teleport flow as the
first follow-up: the 20-25 grounds (Dion) are reachable only through the
gatekeeper network (Mirabel -> Gludio -> Dion), and the bot had no
packet support for the html bypass dialog the teleporters use. The
H-002 hypothesis (the gatekeeper teleport graph) stayed open because
the bot could not drive a single leg.

The fix is the full packet chain the gatekeeper flow needs (T-008,
scope: connection/, hunt/, docs/):

- The packet parsers (round 63, the foundation): RequestBypassToServer
  (to_game, opcode 0x21, the bypass command string) and NpcHTMLMessage
  (from_game, opcode 0x1B, the npc object id + the html body + the item
  id). Both unit-tested against the Mobius Java sources
  (RequestBypassToServer.readImpl, NpcHtmlMessage.writeImpl).
- The connection dispatch (game_dispatch.go + game_html.go): the 0x1B
  packet routes to applyNpcHTMLMessage, which parses it into the scratch
  and stores the last html under a lock. LastHTMLMessage /
  LastHTMLDialog return the copy the hunt loop reads. SendBypass sends
  a RequestBypassToServer through the encrypted game channel. The
  arrival TeleportToLocation + the Appearing answer were already wired
  (applyTeleport sends Appearing on the self teleport).
- The html bypass parser (hunt/gatekeeper_html.go): ParseGatekeeperHTML
  extracts the <a action="bypass [(-h )]<command>"> buttons the server
  html carries, strips the bypass prefix, classifies the command
  (showTeleports, teleport, chat) and, for the teleport buttons,
  extracts the npc object id, the list name (defaulting to NORMAL) and
  the destination index. FindTeleportButton and FindShowTeleportsButton
  are the lookups the step uses.
- The gatekeeper step (hunt/gatekeeper_step.go): DriveGatekeeperTeleport
  drives one leg - sends the showTeleports bypass, waits for the
  teleport list html (LastHTMLDialog, bounded by 5 s), finds the
  destination button and sends the teleport bypass. The GameAPI
  interface gains SendBypass and LastHTMLDialog (the GameClient
  implements both). Four unit tests pin the happy path, the dialog
  timeout, the missing-destination guard and the stale-html rejection.
- The protocol docs (docs/protocol_description.md): the
  RequestBypassToServer (0x21) and NpcHtmlMessage (0x1B) wire layouts
  with the field tables and the Mobius Java class references.

Verification: the unit tests are green (the hunt gatekeeper step, the
html parser, the connection html dispatch and the bypass send), the
golangci-lint run --new is clean (0 issues). The live verification: the
building-entry acceptance scenario (which drives the bot to a teacher
npc and talks through the html dialog) ran with SWARM_TRACE_PACKETS=1
and PASSED; the trace showed the server NpcHTMLMessage packets (0x1B,
813 bytes each) arriving and the dispatch parsing them (no "Failed to
parse npc html", no "unknown packet" - the new 0x1B case consumed
them). The full Mirabel -> Gludio -> Dion live drive is the follow-up
(the hunt-loop gatekeeper trip phase, the T-009 prerequisite): the
packet chain, the parser and the step are ready to wire.

## Round 65: the Dion band gear catalog (T-010, 2026-09-12)

The M3 band survey (T-006) named the Dion merchants the 20-25 band
shopping trip targets, and the gatekeeper teleport flow (T-008) built
the packet chain the bot reaches them through. The gear planner had no
catalog for them - the elven village catalog (the 1-19 band) was the
only one wired.

The fix is the Dion catalog (T-010, scope: gear/, npcdata/, tools/):

- gear.DionCatalog() (gear/dion_catalog.go): the four Dion merchants
  the survey names - Sabrin (7060, weapons), Casey (7061, armor), Sonia
  (7062, jewels + spellbooks) and Lara (7063, grocery) - at the 20
  percent Dion buy tax (MerchantPriceConfig.xml priceConfig id=8,
  baseTax=20; the castle tax is 0 on the local test server so the total
  is the base tax, the same shape the elven 15 percent uses). The
  buylist ids (3006000-3006300) match the file names of the Mobius
  buylists; the npcdata generator (tools/generate_shop_catalogs.sh)
  already loaded them into npcdata.npcBuyLists (shop_catalogs.go:
  7060-7063).
- The D-grade item GearStats were already generated (npcdata/item_stats.go
  covers the D-grade weapons and armor of the buylists; item 256
  verified: DUALFIST PAtk 29, D-grade soulshots), so the gear planner
  scores the Dion items with the existing MeleeFighter profile.
- Two unit tests pin the merchant set, the tax rate, the buylist ids
  and the tax-shape mirror against the elven catalog.

Verification: go build green, the gear tests green, golangci-lint run
--new: 0 issues. The out-of-scope follow-up (a new task): the hunt
multi-town catalog selection (hunt/shopping.go switches to the Dion
catalog when the bot farms the 20-25 band at Dion) and the
shopping_strategy.md Dion shop section. The gate: M1 green before the
20-25 band is reached.

## Round 70: the dialog walker walks - the quest brain gets its legs (T-014, 2026-09-12)

Problem: the M2 quest brain needs a dialog engine that drives one
NPC conversation end to end (the class transfer chain of
docs/quest_protocol.md is four NPCs, nine pages), but the landed
pieces (the 0x1B parse of T-008, the link parser of T-012, the
tracker dialog section of T-013) were three separate seams with no
driver that composes them - and no code fed ApplyDialog (the
connection layer stores the raw html only).

Root cause: the composition was scoped as the follow-up 4 engine
half of the quest research; nobody had claimed it yet.

Fix (T-014): hunt/quest_walker.go - DriveDialog walks one NPC
conversation: the paced two-click talk entry (NpcClick.onAction:
the first Action click of a new target only selects, the second
opens the html - verified in the Java sources; every click
refreshes the last-folk memory the quest events resolve their npc
through), the bounded new-page wait with content change detection
(the connection stores only the last html and the quest pages all
arrive from the same npc - the identical page re-send is the
documented blind spot), the hunt-side feed of the tracker dialog
section (applyDialogPage parses the links and applies the page
before any bypass fires, so IsDialogCommand validates against the
page the command came from) and the link-by-text bypass walk (the
case-insensitive containment match, the validation, the send).

Verification: 10 unit tests through a scripted fakeGame wrapper
(the html action cache emulation: a bypass the open page never
offered is never answered) on the real Q00406 and
ElfHumanFighterChange1 datapack pages - the accept chain, the -h
strip of the class change link, the stale-npc page guard, the
missing-link guard, the same-page repeat blind spot, the timeouts
and the argument guards; the live round trip
(SWARM_LIVE_DIALOG=1, hunt/quest_walker_live_test.go): the
character injected at the trainer approach ring of Ellenia talked
to her, the walker matched the "Quest" link (the bare `bypass
Script` command her trainer page carries), sent it, and the
no-quest answer page arrived and landed in the tracker - the full
click -> html -> link -> bypass -> answer chain against the
deployed stack in 1.7 s. go build ./... green, the hunt package
tests green (the live suite skipped by default),
golangci-lint run --new: 0 issues, tools/mobius_e2e.sh 45 prints
E2E_OK.

Live facts of the round (the elven spawn glade, account dialogw1):
the fresh elven fighter spawns at (46048, 41141, -3440) among
Gremlins, Nerupa (template 7370, 59 units away) and the Newbie
Helper (7400); both glade guide pages are plain text with NO links
(Nerupa's 249 byte intro, the helper's 397 byte gremlin tutorial)
- the nearest link-carrying pages live at the trainer/merchant
sprawl 6-11k south. The trainer pages (Ellenia 30155, Cobendell
30156) carry exactly two links: `bypass -h npc_%objectId%_SkillList`
("Learn skills" - answers with the SkillList packet, NOT an html
page) and the bare `bypass Script` ("Quest" - ScriptLink answers
with the quest choose window or the no-quest message, both html;
verified in the Java). The position injection between
EnsureCharacter and EnterWorld (the mariadb CLI channel) moves the
character cleanly - the game server loads the row at the world
entry.

## Round 71: the quest chains become data - the brain gets its map (T-015, 2026-09-12)

Problem: the quest brain (T-014's walker) had legs but no map - the
class transfer chains lived only as prose tables in the research
doc (docs/quest_protocol.md), and the M2 acceptance scenario plus
the future quest trip phase need the executable form: which npc at
which cond, which links to click, which mobs to farm until which
item count.

Root cause: the research round (T-004) was documentation-only by
design; the data task was queued as its follow-up.

Fix (T-015): hunt/quest_chains.go - ElvenKnightChain (Q00406, the
cond 1..6 ladder: the six Ruins of Agony skeleton species with the
70 percent topaz hunt to 20, the Sorius letter talk, Kluto's favor
link, the 50 percent emerald hunt of the Ol Mahum Novices, the
closing brooch talk) and ElvenScoutChain (Q00407, the cond 1..8
ladder: Moretti's two-link briefing, the torn letter hunt of the
Ol Mahum Patrols (the sequential four-piece drops), the Prias
rescue legs, the 60 percent sentry key, the recommendation close),
plus the two class change routes at Rains (the -h Script
ElfHumanFighterChange1 19/22 legs with the proof items). Every
npc station carries the npcdata display id and the live spawn
position; every kill stage carries the mob display set, the item
ids, the drop chance, the piece target and the spawn hull ground.

Verification: 6 unit tests (the stations resolve through
npcdata.NPCName, the kill mobs resolve to the script's species,
the quest items resolve through the item names, the cond ladders
run 1..6/1..8 with every stage exactly one action, the kill
economy and the route texts pin, the accessors share no state).
go build ./... green, the hunt package tests green,
golangci-lint run --new: 0 issues.

The research correction of the round: the Q00407 cond 2 kill mob
is the Ol Mahum Patrol (npc 20053, display 53), NOT the Bugbear
the research doc claimed - the Mobius npc stats (20053
name="Ol Mahum Patrol", level 21) and the spawn file comments
agree; the Bugbear is npc 20133 (display 133) and no quest of this
chain kills it. docs/quest_protocol.md corrected accordingly.

## Round 66: the multi-town gear catalog selection (T-017, 2026-09-12)

The T-010 follow-up the previous session left: the gear package had
the Dion catalog (gear.DionCatalog), but the hunt shopping loop still
consulted the single elven village townShopCatalog, so a bot that
farmed the 20-25 band at Dion would shop the elven village merchants
(the wrong town, the wrong tax, the wrong buylists).

The fix is the per-region catalog selection (T-017, scope: hunt/,
docs/):

- The Dion merchants (hunt/town.go::dionMerchants): Sabrin (7060,
  weapons), Casey (7061, armor), Sonia (7062, jewels + spellbooks)
  and Lara (7063, grocery), at the spawn positions of
  spawns/Dion/DionNPCs.xml. The buylist ids (3006000-3006300) are the
  file names of the Mobius buylists; the npcdata generator already
  loaded them.
- The Dion catalog (hunt/shopping.go::dionShopCatalog): built at the
  20 percent Dion buy tax (MerchantPriceConfig.xml priceConfig id=8
  baseTax=20; the castle tax is 0 on the local test server). The tax
  differs from the elven 15 percent, so the Dion catalog has its own
  builder.
- The selector (hunt/shopping.go::shopCatalogForRegion): the hunt
  loop picks the Dion catalog when the active zone region is Dion;
  the elven village catalog stays the default. shoppingQueue and
  shoppingTripEnabled switch to it through Loop.zoneRegion (a new
  field the SetHuntingZoneRegion call records). The elven village
  behavior stays unchanged (the M0 acceptance still passes).
- The region key (hunt/zones.go::regionDion): the Dion case of
  SetHuntingZoneRegion logs the pending state (the zone registry is
  T-009, gated on M1 green) and lets the gear catalog selection fire.
- The docs (docs/shopping_strategy.md): the Dion shop section names
  the merchants, the tax, the catalog selection and the sell-first
  rule carry-over.

Verification: 5 unit tests pin the elven default, the Dion selection,
the tax rates, the merchant set and the live buylist ids. The full
hunt test suite (97 s) is green (no regression of the M0 round), and
golangci-lint run --new is clean (0 issues). The Dion zone registry
(T-009, gated on M1 green) and the multi-town spellbook budget (M2
follow-up of T-015/T-016) stay out of scope.

## Round 72: the 20-25 band zone registry of the Dion grounds (T-009, 2026-09-12)

The M3 data gap the survey round named: the 20-25 character band had
its grounds surveyed (docs/band_20_25_survey.md), its gear catalog
(gear.DionCatalog) and its town merchants wired (T-017), but no
hunting zone registry - a bot that reached the band had no squares to
pick, and the region switch of T-017 could only log "the registry is
pending".

The fix is the generated registry (T-009, scope: the generator, the
hunt package, docs/):

- The generator (tools/generate_hunt_zones.py): the elven-specific
  main became a parameterized registry pipeline (parse_territories
  with the band window filter, the fold, the partition, the sort, the
  emit - all shared); the `--dion` flag runs the Dion spec (the three
  spawn sources of the survey, the five band windows, the execution
  arrival anchor, the regionDion constant, the dionHuntingZones var
  and the DionHuntingZones accessor, the zones_dion.go output). The
  elven mode stays byte-identical: regenerated and diffed against the
  committed zones_elven.go as the regression gate.
- The registry (hunt/zones_dion.go): 75 spawn-true squares over 25
  kept territories (dion15_2122_03s folds into dion15_2122_01,
  dion17_2023_14s into dion17_2023_05 - the same-band sub fold of the
  elven pipeline), 96 percent spawn mass coverage, 0 territories
  below 80 percent. Every square carries the full mob list of its
  territory (the over-25 neighbors included, the engage filters on
  the level ceiling).
- The band ladder: five windows continuing the elven ladder
  (20-21/280, 23-24/290, 24-25/310, 25-26/340, 26-27/380) - the
  gear gates calibrated against gear.TotalGearPoints probes of the
  buyable dress stages measured with a temporary gear-package probe
  run: the NG full dress + bone shield 284, the Falchion + bone +
  kite + D jewels 292, Bastard + bone + kite 312, the partial mithril
  step 341, the full D dress 391 (the Dion buylist items, the melee
  fighter profile). The gates sit at the stage the dress passes them.
- The seam closure with T-017: the regionDion case of
  SetHuntingZoneRegion installs DionHuntingZones() (the pending log
  of the T-017 round became stale the moment the registry landed -
  the rebase conflict of the two regionDion constants resolved into
  one comment carrying both the registry and the catalog-selection
  aspects). A deployment selects the band explicitly; the default
  elven flow is unchanged, and the production band entry waits for
  M1 green per the survey protocol (the M3 band acceptance scenario
  is the consumer).

Verification: zones_dion_test.go pins the survey tables as the
acceptance reference - the thirteen mob species at their table
levels, the sprout ground mob sets (only the two sprout species),
the cruma edge mob sets (exactly the leech/ant/ripper mix, every
ripper square also carries the leech), the walking distances of the
survey anchors (the sprout squares within 10k of the execution
arrival, the cruma squares within 12k of the cruma center, the
northern wolf squares within 18k of the Dion town square, every
square within 18.5k of a teleport arrival), the picker ladder (the
starter fallback at level 20, the NG dress opening the sprouts at
22, the Falchion dress the wolves at 25, the Bastard dress at 26,
the partial mithril at 27, the full D dress the cruma grounds at 28,
the death cap demotion) and the region install (the loop gets the
registry, the spot mode stands down, the region feeds the catalog
selection). Build green, the full hunt suite green (97 s),
golangci-lint run --new clean (0 issues), the elven regeneration
byte-identical.

## Round 72: the class transfer accept stage - the M2 vehicle drives its first leg live (T-016, 2026-09-12)

Problem: the M2 acceptance needs its vehicle - a scenario that
injects the level 19 elven fighter at Gludio and drives the quest
chain - but the acceptance package had no quest scenario and no
way to drive the dialog walker from a scenario (the sessions of
the manager encapsulate the game client, the scenarios only
observe the tracker).

Root cause: the quest pieces (the walker of T-014, the chain data
of T-015) landed without their acceptance composition; the T-003
injection machinery and the session patterns existed but never
carried a dialog walk.

Fix (T-016, the staged build): acceptance/class_transfer.go - the
class-transfer scenario (temp9, the level 19 injection with the
milestone wallet shape standing on Sorius's approach ring at
Gludio, 150 units off his trainer hall cell), the manual session
seam (startClassTransferSession exposes the game client for the
walker drive - no hunt autonomy, the elven zone machinery would
drag the character home), the Sorius find through the world store,
the composed accept route (QuestEntryLinks + the chain accept) and
the journal flip gate (awaitQuestCond on the tracker QuestCond).
The staged pass line: Q00406 cond 1 within the budget.

Verification: live run PASSED (the 3-link accept walk, the journal
"0 quests -> 1 quests" flip at the accept bypass, 7 s); the hunt
and acceptance package tests green, golangci-lint --new clean,
tools/mobius_e2e.sh 45 prints E2E_OK; the metrics trail carries
the FAIL discovery row and the PASS row.

Two server facts the live iterations pinned (the first run FAILED
twice before passing - the honest record):
- The first talk of a quest npc without an ON_NPC_FIRST_TALK
  listener opens the npc's STATIC page (Sorius: his trainer html
  with the Learn skills and Quest links), NOT the quest page; the
  quest page of the current cond rides the bare `bypass Script`
  command - ScriptLink.showQuestWindow resolves the single quest
  of the station straight through Quest.notifyTalk (the choose
  window link list only appears for npcs offering several quests).
  The chain data gained QuestEntryLinks (the one-link prefix).
- The bypass flood protector (FloodProtectorServerBypassInterval
  = 3 of the deployed FloodProtector.ini) silently drops a bypass
  riding the previous one - the second page link of the accept
  walk vanished until the walker learned to pace its sends
  (dialogBypassPace, 3.2 s). The player action protector runs at
  1 s (the talk clicks were always fine).

## Round 73: the walking leg verification of the elven lands to Dion (T-018, 2026-09-12)

The open item the band survey stood on: the 20-25 band was reachable
only through the gatekeeper chain (13 600 adena one way) because the
walking alternative was never measured - the survey recorded it as
"unverified: the geodata pack covers the corridor but no route was
run".

The verification ran as a throwaway probe (not committed, the
T-018 scope: a temporary patch of MaxSearchExpansions to 40M, a
temporary probe test in the pathfind package reusing the
townTestEngine harness, the region cache raised to 16 - the exact
recipe of the navigation analysis scale measurements):

- Elven Village (Mirabel 46926 51511) -> Gludio town (-12694
  122776): **found** - 111 852 units, 49 waypoints, 1 253 518 node
  expansions, 13.0 s. At the SHIPPED 1M cap the same search aborts
  (explored exactly 1 000 000) - the leg exceeds the shipped budget
  by 25 percent.
- Gludio town -> Dion town (15671 142994): **found** - 42 831
  units, 25 waypoints, 939 096 expansions, 8.9 s, within the shipped
  cap.
- Both searches used FindPathApproach (radius 150, the town trip
  form) over the in-repository geodata pack; the engine.go patch and
  the probe test were reverted after the run (the committed tree
  carries only the findings).

The consequences recorded in the docs:

- The teleport-only assumption of the survey is refuted as a hard
  claim (docs/band_20_25_survey.md): the corridor through the
  Neutral Zone exists on foot end to end. The gatekeeper chain
  stays the practical leg (~154 700 units of running, over 14
  minutes, against the instant 13 600 adena hop); the walk is the
  zero-adena fallback of a broke character and the emergency
  return.
- The navigation analysis gains the two route rows
  (docs/navigation_analysis.md): the "cap as a parameter" roadmap
  item now has its concrete number - the elven -> Gludio leg needs
  the cap raised above 1.25M before the bot can use it.

Verification: the probe run itself (both legs found, the shipped-cap
abort reproduced for leg 1), the revert verified by git status
clean, the findings cross-recorded in the survey, the navigation
analysis, this log and the agent progress notes. No committed code
changed - go build and the pathfind package tests stay green by
construction (the tree is byte-identical to the T-009 close).

## Round 74: the quest trip phase engine (T-019, 2026-09-12)

The T-016 hand-off plan named the missing hunt half: the acceptance
scenario can drive the accept conversation (the staged round passed
live), but the full class transfer chain needs a chain runner in
hunt/ that the acceptance scope cannot write (acceptance/ + docs/
only).

The engine (hunt/quest_trip.go, a new file):

- DriveQuestChain(ctx, chain): the accept conversation when the
  journal does not carry the quest, then the stage ladder by the
  journal cond - talk stages walk to their station and drive the
  entry prefix plus the stage links through the T-014 walker, kill
  stages walk to the ground and engage the quest mobs until the
  journal counters fill - until the exit talk drops the quest.
- FindQuestNpc: the knownlist scan of the station (the npcdata
  display id on the wire template space, the +1000000 convention).
- walkToQuestPoint: the walk request plus the arrival poll (the
  200 unit arrival radius sits inside the 250 interaction distance;
  the kill ground z is unknown to the chain data, the self z rides
  the request and the server corrects the height).
- The knobs: the 20 minute kill stage deadline, the 5 minute walk
  deadline, the 700 ms attack pace (the Mobius double click
  semantics), the 20 percent health floor (the manual trip has no
  rest phase - a dying character is the caller's decision, not the
  engine's).

The design gap caught before it shipped: the kill stage exit. The
first sketch re-read the journal cond after the counters filled,
which would loop the kill stage forever if the cond only moved at a
turn-in talk. The Mobius script source settled it (rule 5 - never
guess the server): the 20th piece drop itself runs st.setCond(2,
true) (the giveItems branch of ON_ATTACKABLE_KILL), so the journal
advances AT the kill and the ladder reads the next stage off the
fresh cond. The Q00407 letter drops follow the same shape.

Verification: the two scripted unit tests on the walker test
harness - the happy path (the static page entry, the accept route,
the journal flip, the kill stage engaging the mob until the 20th
piece sets cond 2, the exit talk dropping the quest) and the ladder
guard (a journal cond outside the ladder returns the error naming
it). Build green, the full hunt suite green (99 s), golangci-lint
run --new clean (0 issues). The live verification rides the T-016
acceptance round that consumes the engine (the M2 closer).

## Round 75: the spellbook catalog resolution across towns (T-020, 2026-09-12)

The lesson stall the survey follow-up 5 named: the level 24 lessons
of the first profession demand the Cure Bleeding spellbook (1379)
the elven village does not sell (the village book list carries only
Charm 1513 and Poison Recovery 1377), so the lesson trips of a
character past level 23 could never buy the book - the lesson
waited forever ("no known merchant for the spellbooks, learning
without them").

The fix (the data plumbing of the band lesson economy):

- bookPurchase (hunt/learning.go) walks the two town catalogs: the
  elven village first (the near stop of the 1-19 band), the Dion
  catalog as the fallback - Sonia (7062, buylist 3006201) sells all
  three books of the first profession band. The price carries the
  selling town's tax (the shopTaxPercent read of the catalog rate,
  the integer math of the original 15/100 form kept for both towns).
- merchantByTemplate (hunt/shopping.go) covers the Dion merchants:
  the trip stop planning finds the Sonia station at her survey
  position when the book routes through Dion.

Verification: 3 unit tests pin the routing - the village books stay
local at the 15 percent tax, the Cure Bleeding book falls back to
the Sonia stop at the 20 percent Dion tax, the merchant lookup
covers the Dion stations. The full hunt suite green (98 s),
golangci-lint run --new clean (0 issues). The production walk legs
(the village -> Dion book trip of a band character) ride the band
wiring gated on M1 green; the M2/M3 acceptance drives the stops
directly.
