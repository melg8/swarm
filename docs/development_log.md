# Development log

Running log of the work on the `mobius-c1-client-1` branch. Newest entries at
the bottom. This file exists so the context does not have to be repeated in
agent prompts: read this file first.

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
