# The web interface and the live state tracker

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The bot embeds a web UI (`internal/swarm/webserver`, files in `web/`)
served from the bot process itself, so it is reachable exactly while
the bot runs. The web UI is plain HTML/CSS/JS without a build step;
keep it that way (embedded via go:embed). Watch out: top level `const`
declarations are not `window` properties, so cross script references
must use the bare binding name.

## The four launch modes

The UI boots in one of four modes chosen by `GET /api/config`:

1. **Bot control** (the default; `-web 127.0.0.1:8080`, `-web ""`
   disables it): the mode described in this document.
2. **Pathfind test** (`-pathfind-test`): the bot less map that reuses
   the same map canvas, camera and theme - a `body.mode-pathfind`
   class hides the bot panels and shows the pathfind sidebar panel
   (see docs/pathfinding.md).
3. **Fight FX gallery** (`-test-fight-ui`, mode `test-fight`,
   `body.mode-test-fight`), a design decision aid for the combat damage
   visualization: a horizontally scrollable grid of 18 numbered variant
   columns x 4 enemy placement rows (the enemy above, below, left and
   right of the character), where every cell runs the same synchronized
   beat loop (the hero hits, the hero takes a hit, the hero lands a
   critical, the hero takes a critical) on the same starter meadow map
   tile crop. All effects are pure functions of the loop clock seeded
   per beat, so repainting is deterministic and the off screen columns
   are skipped. Clicking a variant head marks the column (the pick
   highlight); the pause button and the speed select drive the loop.
   The harness is `tools/repro_fight_ui.js`.
4. **Fight FX v1 showcase** (`-test-fight-ui-v1`, mode `fight`,
   `body.mode-fight`): the v1 idea set of the concurrent session - a
   bot less page of twelve numbered combat animation ideas
   (`web/fight.js`): every column is one damage visualization variant
   (floating numbers, comic pop, slash, spark spray, shockwave, HP
   chunk, arrow, hit-stop, cell flash, dizzy stars, arcade banner,
   combo counter), every column stacks the four enemy positions (top,
   bottom, left, right of the hero) so each idea is judged from every
   attack direction. All cells share one clock and one scripted fight
   loop (hit, take, crit, take crit, regen), the background is the real
   map tile of the hunting grounds and the strip scrolls horizontally.
   Both pages stay side by side until one idea set wins.

The fight modes hide the whole toolbar; the pathfind mode keeps only
the map background row of the view menu and hides the bot status
banner.

## Layout philosophy

The UI follows the classic bot tool layout (L2Walker/Adrenaline style:
compact window, top tabs, left bot list, bottom status bar) and is
designed for many bots: the bot registry enumerates every session, the
sidebar lists them and clicking switches the observed bot. The light
theme is the default, a header button toggles to a dark theme (both
are CSS variable sets on `html[data-theme]`; the canvas reads its
colors from the same variables).

- Header: brand, Map/Log tabs and the live indicator share one compact
  34 px row - the working area must not lose vertical space to chrome.
- Tabs: Map is the source of truth: the character status lives as a HUD
  panel on the canvas (name, class, level, HP/MP bars, position,
  combat/rest chips, exp/sp) and the world is drawn around it; Log is
  the rolling event feed with a filter.
- Bot list: every row shows the status dot, the name, `combat`/`rest`
  chips, the level and three mini bars (HP red, MP blue, XP silver,
  same gradients as the HUD) fed by the extended `/api/bots` payload -
  the overview shows at a glance what every session is doing. The
  sidebar bot row also carries the activity text under the name
  (`botActivityLabel`, fed by the `phase` field of the BotInfo payload)
  so the overview shows what every session is doing.

## Camera and the world map background

- Camera: follow mode centers on the interpolated character position;
  free mode (follow unchecked or a map drag) pins the view to a pan
  anchor captured at the moment follow was disabled and never moves on
  its own, so the map shows the chosen area regardless of bot movement.
  Dragging grabs the map like a sheet of paper: the camera moves by
  `-delta cursor / scale` in world units, so the grabbed world point
  stays exactly under the cursor. The wheel zoom anchors at the cursor
  in the free camera mode (the world point under the cursor stays under
  it) and centers on the character while follow is on.
- World map background: the map draws the game world map tiles under
  everything (`show-map` toggle in the toolbar, on by default). The
  tiles come from the L2Bot2.0 assets (`E:\work\L2Bot2.0\Client\Assets\maps`,
  32 world units per source pixel, one 1024x1024 tile per 32768 units
  block named `BX_BY.jpg` with BX = floor(x / 32768) + 20 and
  BY = floor(y / 32768) + 18 - the same anchors as World.TILE_ZERO_COORD
  of the Mobius server; the `_1`/`_2` suffixed files are dungeon floors
  and are not shipped). `tools/generate_map_tiles.sh` builds a google
  maps style pyramid into `web/maps/{level}/{bx}_{by}.jpg`: every base
  tile ships at levels 0..3 (1024/512/256/128 px, jpeg quality 65) -
  the whole world at full resolution, about 31 MB total.
  `web/map.js drawMapBackground` picks the pyramid level allowing at
  most a 2x upscale of the tile pixels, draws the tiles covering the
  viewport through the usual world to screen transform (follow and pan
  included) and lazy loads them via the static file server; a tile
  missing from the source set falls back to the closest existing
  pyramid level of the same block stretched over the tile rect; the
  grid, the zone square, the units and the links draw on top.

## Map rendering

- Every unit (character, mob, player) is a circle with a short look
  direction tick from the center over the edge (L2Bot2.0 style),
  colored by threat: friendly gray, passive monster green, aggressive
  amber, fighting red, dead gray-faded, players violet, self blue with
  an accent ring; ground items are gold diamonds. The dashed square is
  the server loaded zone: the 3x3 world region block (region size 2048,
  `World.broadcastPacket` reaches exactly these regions) around the
  character region - the server only spawns/updates objects inside it,
  and it scales with zoom like every world element.
- The unit markers use a fixed theme independent palette (mapColors in
  map.js, also mirrored into the legend css) so the icons read
  identically over the light map imagery and over both theme fills; the
  marker outline and the direction tick use the fixed slate
  mapColors.tick for the same reason; the labels draw white (gray for
  the dead, the own target red) with a dark halo and pass a declutter
  pass (priority: hovered, own target, in combat, then the distance to
  the character - overlapping names are dropped, the closest win). The
  unit markers scale with the zoom (a sub linear factor of the map
  scale, clamped 0.3..1.6) so zooming out shrinks them together with
  the map - the direction ticks, the line widths, the link rings, the
  labels and the social marker follow the same factor - and the draw
  order is deterministic - dead units first, then north to south, then
  the object id - because the snapshot objects arrive in random go map
  order and overlapping units would flicker otherwise. A sitting
  character draws the breathing zZ marker above its dot. Unit markers
  draw the look direction tick only outside the circle; inside the
  radius the marker is a solid fill.
- Map target links: the map renders the selection of every visible
  player, not only the own one. The own target is a red dashed line
  with a ring; the targets of other players are violet dashed lines
  with violet dashed rings around the claimed objects (and a violet
  ring around the bot itself when the bot is the target). A mob ringed
  in violet is claimed by someone else - the precondition for not
  training other players' mobs. The tooltips show what a unit targets.
- Zone drawing: the map draws every zone (active amber, the future
  grounds in a bright soft blue with a light fill - the demonstration
  of where the bot will hunt next, dimmed hints do not read; demoted
  bands red - labels only when the square is big enough on screen; the
  `hunt zones` row of the map toolbar view dropdown hides the whole
  layer like the targets and map background toggles) and the floating
  collapsible zone panel of the map (bottom right corner, collapsed by
  default, the count chip carries the registry total; the left sidebar
  lists bots only) carries the death counts and switches zones manually
  (the `zone` command, index in the Count field; the override holds
  until the character outgrows the band or dies it out).
- Threat data: the npc level, `aggroRange` and `isAggressive` ai flags
  come from the generated `internal/swarm/npcdata` maps (the C1 data
  pack marks every monster `isAggressive=false`: they only defend). The
  map draws the aggression radius of every living aggressive mob as a
  dashed circle around its drawn position (amber idle, red once it
  fights; the `aggro` toolbar checkbox hides the layer).

## Movement interpolation (map.js projectTickwise)

`web/map.js projectTickwise` reproduces the server movement exactly
instead of approximating it. The Mobius movement loop
(Creature.updatePosition) runs on 100 ms game ticks and advances a
creature by `xAccurate += (dest - xAccurate) * frac` with
`frac = speed * ticks / 10 / (remaining - collision)` - a converging
geometric walk that is slightly faster than the nominal speed, stops
collision units short and snaps to the exact destination once frac
exceeds 1. The map replays the same recurrence tick by tick from the
packet position, speed and collision radius (NpcInfo and CharInfo
carry it; UserInfo does not, the played character uses the constant 9)
and interpolates linearly between the two surrounding tick positions,
because the server truth is a step function of one jump per tick and
the official client renders it smoothed the same way.

The drawn position chases this projection with a speed cap of 1.35x
the unit speed, so delivery latency, retargets and arrival snaps become
a slightly faster glide instead of a jump, and in steady motion the
drawn position sits on the projection with zero lag. The packet speeds
are exact: the server re-reads its move speed every tick (buffs and
walk/run switches take effect with the next broadcast), races differ
through their base speeds, and the transmitted values divide by the
move multiplier which the tracker multiplies back. Snapshots arrive at
most every ~300 ms (SSE poll), but the projection is anchored at the
packet time, so staleness does not bias the position. The clock is
corrected against the snapshot `serverTimeMs` (the maximum of the
recent samples, because every sample underestimates by the snapshot
transport delay). MoveToLocation for playables only arrives at move
start and arrival, non forced broadcasts are throttled to one per
second (a re-issued move inside the window stays invisible). The played
character is interpolated the same way from its own movement
broadcasts (Player.broadcastPacket sends every broadcast except
CharInfo to the acting player itself).

Heading semantics (Mobius `LocationUtil.calculateHeadingFrom`):
`atan2(dy, dx) * 65535 / 2pi`, 0 faces east, the angle grows clockwise
because world y points south. The server announces arrival with a zero
distance MoveToLocation (current == destination): keep the previous
heading in that case, never recompute it (the delta is zero and would
point every mob east - this was a real bug). CharInfo carries no
heading at all: the server announces the facing of a standing player
through the StartRotation + StopRotation pair it sends to every new
observer (see `Player.sendInfo`), and keyboard rotation of a visible
player is broadcast as BeginRotation 0x77 / StopRotation 0x78 (the
client sends StartRotating 0x4A / FinishRotating 0x4B which the server
relays). The Attack packet carries the attacker and target locations
but no heading: the attacker faces its target, so the map computes the
heading from the attacker -> first hit target vector (the server sets
exactly this heading in `Creature.doAttack` before broadcasting).

## Combat animation layer (map.js + state combatEvents)

The map plays the combat the tracker observes: every `Attack`
broadcast lands as one swing per hit that actually connected (the
packet carries the Mobius miss flag per hit - an evaded blow draws
nothing) - a colored streak runs from the attacker to the hit target,
light blue for the own attacks, red for the mob ones, with a windup
swoosh at the attacker and a white impact starburst on the target;
every `StatusUpdate` HP drop floats a damage number above the hurt
unit (amber on mobs, red on the character) with a flash ring under it.
The server side is `state.CombatEvent` (the `combatEvents` ring of the
tracker): `ApplyAttack` records the swings, the HP deltas of
`ApplyStatusUpdate` record the damage (the Attack broadcast carries no
damage value, heals record nothing), the snapshot carries the last 2 s
with a monotonic `seq` and the client dedupes on it across the SSE
snapshots (the first snapshot after a page load only accepts the
cursor). The effects track the interpolated runtime positions while
the units stay on the map and fall back to the event placement after
they despawn; `needsMoreFrames` keeps the render loop alive while any
effect lives. The feed is bounded (64 events) and the snapshots only
grow by the 2 s window, so the payload stays small.

## HUD, target panel and the status banner

- HUD: the map top left holds one vertical stack of two panels: the
  character panel (name as the heading, the class text and the
  combat/rest chips share the line under it, HP/MP/EXP bars, then a two
  column grid: level/race, x/exp, y/sp, z/slots, weight/adena with the
  weight as a bare percentage) and directly below it the target panel
  of the currently selected object (name, level chip, HP bar, MP row
  that reads no data for npcs - the C1 server never sends their MP).
  The target panel exists only while there is a target: a killed,
  removed or missing target hides it completely. Long target names live
  in their own panel and cannot break the layout. The experience bar is
  the third bar under HP and MP, filled from the snapshot `expPercent`
  that the bot computes via the C1 experience table in
  `internal/swarm/state/experience.go`, regenerated by
  `tools/generate_experience_table.sh`. The bar colors follow the
  classic L2 C1 palette (HP red, MP blue, EXP light silver - gold is
  the CP color of later chronicles) with light-top/dark-bottom
  cylindrical gradients; the three gradients are shared CSS variables
  (`--grad-hp/mp/xp`) reused by the sidebar mini bars.
- State dump: the Dump state button of the bot HUD copies a plain text
  report of everything the tracker knows into the clipboard
  (`GET /api/bots/{id}/dump`, `BuildStateDump` in
  `internal/swarm/webserver/dump.go`) - the character sheet, the
  attackers, the hunting zone, the inventory, the world objects with
  their combat state, the walk plan, the recent combat/chat and a 600
  event deep window (the hunt loop decisions mirror into it). The
  second line of the report is the build identity - `build: branch
  <name>, commit <full 40 char hash> (dirty|clean), built <time>` - so
  a live problem report always tells which exact code produced it (see
  `internal/version`): the report plus a `git log` are all it takes to
  line the behavior up with the source.
- Bot status banner: a compact chip pinned to the top center of the
  map (`#bot-status` in index.html, `renderBotStatus` in app.js) shows
  the current activity of the active bot at a glance - hunting,
  looting, walking to town, selling, walking to farm spot, deleveling,
  manual move, idle. The activity text and the data-kind attribute come
  from `phaseLabel(snap)` which maps the hunt loop phase
  (`snapshot.phase`, published by `state.Bot.SetPhase` from the hunt
  loop tick through a defer) to a human readable label and color kind.
  The session status takes precedence when no phase is published (the
  manual only sessions never set the phase): the banner falls back to
  the connecting/offline text. The dot pulses while the bot is active
  so the banner reads as live; the pathfind test mode hides it.
- Walk path view: the map draws the manual walk plan
  (`snapshot.walkPath`) as a blue dashed line from the character to the
  clicked destination. The publishWalkPlan path covers every walking
  phase of the hunt loop, not only the manual move: town trips (the
  walk to the trader and the walk back to the farm spot) and the
  deleveling guard walks publish their remaining geodata waypoints too,
  so the map draws the planned path of every autonomous walk. The plan
  clears on the non walking phases (engage, loot, sell, idle) through
  the `ClearWalkPlan` call of the tick. The destination marker (the
  pulsing blue dot) draws at the last waypoint of the published plan.
  While a manual move runs, the loop publishes the walk plan into the
  tracker (`state.Bot.SetWalkPlan`: the remaining waypoints with the
  clicked destination last, refreshed every tick, expiring on its own
  after 2 s without a refresh); the map draws it while the paths toggle
  is on - a blue dashed polyline from the character through the
  remaining waypoints - plus the always visible destination marker
  shared with the click ripple: a light blue dot with a pulsing
  breathing ring (the self character also gets the same dashed
  destination line as every other moving object while it runs).

## Equipment widget and the shop queue flyout

- Equipment widget: a floating overlay on the map in the top right
  corner (same panel chrome as the player HUD on the left; starts below
  the compass rose, hidden in the pathfind test mode) - not a layout
  column, so the map keeps the full body width. The inventory packets
  parse the body part mask and the enchant level of every entry (see
  AbstractItemPacket.writeItem), the snapshot carries the whole
  inventory as `snapshot.inventory` with the resolved display name and
  icon file name per item, sorted equipped-first. The paperdoll is
  compact: a 3x3 wear block ordered like the C1 client doll (shirt,
  head, cloak / weapon, chest, shield / gloves, legs, boots) on the
  left and a 2x3 jewelry block on the right whose middle-left cell is
  a blank hole with the necklace on the middle-right - the classic
  character has only five jewelry slots (two earrings, a necklace, two
  rings). app.js places the equipped items by the C1 BodyPart mask (two
  handed weapons and full armor alias onto the weapon/chest slot, the
  either-or earring/ring masks 0x6/0x30 fill the first free slot of
  their pair); below them the bag renders six columns by four visible
  rows with a scrollbar, one icon cell per item with stack count and
  enchant badges; an empty or missing icon falls back to a per type2
  glyph. The cells are keyed (slot key / item objectId) and rendered
  incrementally: unchanged items keep their DOM - the icon `<img>`
  elements are never recreated by a snapshot (a fresh element re-decodes
  and the icon blinks), stack count and enchant updates only rewrite
  the text badges, and reordering moves the persistent cells. A pinned
  footer under the scrolling bag stays always visible: the adena line
  (gold, from `character.adena`) and the weight line (fill by load
  percent from `character.load/maxLoad`) and the trash bin at the far
  right end - a cell dragged onto it destroys the item. The weight fill
  colors by the server weight debuff thresholds (Player
  refreshOverloaded of the Mobius C1 source: the load per mille
  switches the penalty at 500/666/800/1000 - 50%, 66.6%, 80% and 100%):
  green below the first threshold, then the fill melts from yellow
  through orange into red (a JS hue interpolation between the level
  anchors, set inline over the green gradient), the percent text takes
  the fill color, and the tooltip carries the raw numbers plus the
  active debuff level and its speed modifier (x0.90/x0.87/x0.84/x0.81).
  The icon pack lives in `data/icons` (3134 PNGs of the classic client
  naming scheme from the l2walker mirror, C1 compatibility verified
  4222/4222 items - see data/icons/Readme.txt), the web server serves
  it at `/icons/<name>.png` with a day of cache - the icons directory
  is resolved from the process working directory with a walk-up, so
  launch the bot from the repo root; the item id to icon mapping is
  generated into `npcdata/item_icons.go`
  (`tools/generate_item_icons.sh`).
- Queue flyout widget: a SHOP QUEUE flyout of the equipment panel (the
  inspection tool of the shop strategy - what the bot plans to buy
  next, at what price and how much adena is still missing). A single
  small triangle tab sticks out of the left edge of the panel (top
  aligned with the title row) and owns BOTH queues - whichever view
  the widget tab shows (see the skills view below): the shop plan
  under EQUIPMENT, the learning plan under SKILLS, never both at once;
  the click slides the queue of the current view out to the LEFT of
  the panel as an absolutely positioned flyout, so the equipment panel
  itself never changes size (the queue overlays the map, the glyph
  points left while the queue is hidden and right while it is out).
  Switching the widget mode re-docks the flyout: the open state is
  shared (`QueueFlyout` of web/app.js), so the queue of the new view
  slides out from the same dock and the other one closes; the tab
  hides while the current view owns no queue.
  The snapshot field `shopping` (null when nothing is published)
  carries the full purchase queue: the affordable plan of the next trip
  first, then the wanted tail - the best value-per-adena picks the
  wallet cannot pay for yet (`gear.PlanPurchaseQueue`, at most 8 tail
  entries, one purchase per paperdoll slot per trip like the plan) with
  the cumulative missing adena per entry (the sell credits of displaced
  gear counted, so a replacement only misses the difference). The hunt
  loop publishes it on every tick through
  `state.Bot.SetShoppingPlan` (the 5 s recompute cache shared with the
  trip trigger, an identical view is a no-op, an unpublished 10 s window
  expires like a walk plan); while a town trip runs the view switches
  to the remaining trip buys - the in-flight batch marked `buying`
  first, then the pending purchases of the stops. The widget: the head
  of the flyout keeps one summary line (entry count, affordable total,
  the save-up missing of the whole queue), the body adds the scrollable
  row list (icon, name, merchant and score gain, price; the wanted rows
  dimmed with their missing amount, the buying rows accented with a
  chip) and the pinned buy/have/save foot. Every row hovers into the
  rich purchase tooltip anchored to the LEFT of the row (the queue sits
  left of the equipment panel, so the tooltip never covers it) - the
  classic item tooltip stat lines plus the planning block (Gain,
  Value/adena - the optimality metric that flags a suspicious pick,
  Sell first credit, Missing, the pick status). The rows are keyed by
  item id and refresh in place (the icon images never blink); the
  flyout and the tab hide when no plan is published (the manual-only
  sessions, sessions without the shop strategy).

## Skills view and the skill learning queue

The equipment widget is a two view widget that behaves like real tabs
- switching the mode fully replaces the visible content. The EQUIPMENT
/ SKILLS mode tabs replaced the static title row. The gear content
stays in the flow and keeps sizing the panel, the skills view is an
absolutely positioned overlay of exactly that area: the hidden view
turns invisible (`visibility` swap, never `display none` - the panel
must not change its dimensions) AND the overlay carries `z-index: 4`
so it paints above every gear child - the paperdoll icon, glyph and
badge cells stack at z-index 1..3 and would otherwise bleed through a
z-index auto sibling (the mode class lands on `#gear-main`, which
must carry that id in the markup). The skills view carries the ACTIVE
/ PASSIVE filter tabs, the learned skill grid and the pinned sp/next
foot (the SP wallet and the head of the learning queue, anchored at
the panel bottom like the adena/weight footer); the mode and the
filter persist in localStorage. The learned grid is a small fixed
grid - the same six 36px column metric as the bag and the same four
visible rows (153px) - one keyed cell per skill with the icon and the
green level badge (the icons never re-decode, `SkillCells` of
`web/app.js`); the trailing cells are dashed EMPTY placeholders, the
future slots of the queued lessons: the grid always holds complete
rows (at least the four visible ones, `SKILL_GRID_MIN_CELLS`), so the
few learned skills sit in a ready cell grid instead of hanging in the
air, and it scrolls once the learned list outgrows the four rows. An
entirely empty filter tab shows the muted note instead of the slots.
The data comes from the SkillList packets (0x6D, `state.Bot.SetSkills`)
enriched with the generated skill dictionary (name, icon, passive
flag, the description of the learned level - the classic client
tooltip texts the Mobius C1 skill stats carry as XML comments, one
run per changing level, `npcdata.SkillDescription`). Hovering a cell
shows the floating tooltip with the name, the description (clamped to
seven lines by the css) and the passive/active kind with the learned
level.

The skill learning queue is the second content of the single queue
flyout dock (see the shop queue widget above): in SKILLS mode the same
triangle tab slides the lesson plan out to the left edge instead of
the shop plan - one keyed row per remaining lesson of the class tree
(`snapshot.skillPlan`, at most 256 entries) with the icon, the name
with the learned level, the warrior priority category (attack power /
defense / other, from the effect stats of the Mobius skill
definitions), the unlock level and the SP cost with the missing SP;
the head summary and the pinned sp/need/save foot mirror the shop
queue. Hovering a lesson row shows the floating tooltip with the
name, the description of the level being learned, the category, the
unlock level, the cost, the status and the position of the lesson in
the queue (`In queue #N of M`) - the plan wide totals (the lesson
count, the missing SP) live in the pinned summary of the flyout head
and foot, they never repeat in every lesson tooltip. The queue order is the warrior priority: the physical weapon
attack power lessons first (the strikes and the masteries), the
defense lessons second, everything else last; within a category by the
unlock level. The learning function itself is NOT implemented by
design - the queue only shows the planned order. The skill dictionary
regenerates with `tools/generate_skill_trees.sh` (skill stats + class
trees of the Mobius C1 datapack into `npcdata/skill_trees.go`). The
widget checks live in `tools/repro_gear.js`.

## Chat window

The bottom left corner of the map shows the parsed system messages and
the social animations (`snapshot.chat`, a rolling 64 line ring fed by
ApplySystemMessage/ApplySocialAction). The auto scroll follows the
newest line only while the view is at the bottom (`chatAtBottom`, 4 px
tolerance): scrolling up detaches the follow to read the history,
scrolling back to the bottom resumes it. The list itself is the scroll
container (the box clips, the list scrolls). SystemMessage texts
resolve through the generated `npcdata/system_messages.go` dictionary
(id -> client text with $sN placeholders, substituted positionally with
the packet parameters; item and npc name parameters resolve through
the item and npc dictionaries). Regenerate with `task
generate:system-messages` (tools/generate_system_messages.sh) after
Mobius updates.

## Map toolbar

One compact row (a 37 px bar, never a wrapped checkbox column). The
old `-`/`+` zoom buttons are gone - the wheel owns the zoom alone
(cursor-anchored, `onWheel` of `web/map.js`). The layer checkboxes
(labels, paths, zone, targets, hunt zones, aggro, map background) fold
into a `view` dropdown (`web/app.js` `initViewMenu`): the button
toggles the pop under it, a click anywhere else or Escape closes it,
clicks inside the pop stop their propagation so several toggles survive
one open; the bot-only rows carry the `bot-layer` class and hide in
the pathfind mode (only the map background row stays there), the whole
toolbar hides in the fight modes. The `follow` checkbox and the
pathfind arm buttons stay inline; the scale and object counters pin to
the right.

## Interactivity (commands and manual mode)

The web UI is interactive in every launch mode: a double click on the
map (move/attack/pickup - hit test over the interpolated object
positions) or on the target HUD panel (attack the shown target), a
double click on a widget cell (useItem: equips a wearable bag item into
its slot, unequips an equipped one - the same C1 packet toggles both,
see UseItem.runImpl; an occupied slot swaps: the equipped item comes
off first, the new one equips after it) and drags (bag cell ->
paperdoll equips with the same swap, paperdoll cell -> bag unequips,
any cell -> map drops on the ground at the character feet, stackable
items ask the count through a small dialog - the scroll wheel over the
open dialog steps the count by one, clamped into the stack (with the
all/cancel/drop buttons, Enter and Escape); a cell dragged onto the
trash target right of the adena and weight lines destroys the item -
RequestDestroyItem 0x59 `[objectId][count]`, stacks open the count
dialog in the destroy mode, an equipped drag unequips first).

The commands flow through `POST /api/bots/{id}/commands` ->
`state.Bot.PushCommand` (a 32 entry queue, newest wins) -> the loop
drains it every tick (hunt/user.go): useItem/drop/destroy execute at
once, gated on the server confirmation of the previous one - the
Mobius packet executor runs every client packet as its own thread pool
task, so a same-burst unequip+equip pair raced in the paperdoll and
cancelled each other; the gate holds the newcomer until the tracker
observed the effect of the previous action (the equipped flag flipped,
the count changed or the item vanished, see `InventoryItemState`) with
a 600 ms fallback timeout so a refused request never blocks the queue
(the UseItem flood protector of this build is disabled:
FloodProtectorUseItemInterval = 0, retail matching) - a swap pair lands
in ~350 ms live instead of the old fixed one second pause; a deferred
command retries on a later tick with the pair order intact,
move/attack/pickup switch the `phaseUser` manual mode that overrides
the autonomous hunting until arrival/death/timeout (an active town trip
is cancelled, the deleveling refuses movement commands - the guard walk
must finish). The attack phase falls into the loot phase on a killed
target, the same way as the autonomous engage. The command queue is
drained on session resets so a reconnect never replays stale clicks.

Without `-hunt` the loop runs in the manual only mode
(`SetAutonomy(false)`, the `phaseIdle` phase): the commands execute
exactly the same way, the autonomous hunting, town trips and deleveling
stay off, the village restart after a death still works. The geodata
engine loads in every mode and serves the manual long walks: the
server side pathfinder silently refuses far targets (observed stuck
walks past a few thousand units), so a click beyond 2000 units plans
the geodata path once and follows the waypoints in server accepted legs
(userWaypoints, 1s request pace, legs capped at 1000 units). A manual
command that replaces a walk still running on the server re-issues the
walk request at once (`userRedirect`: the next tick fires the new
MoveToLocation instead of waiting for the old walk - the server
replaces the destination of a running walk), so a click somewhere else
changes the direction immediately.

## Endpoints

- `GET /api/bots` (list), `GET /api/bots/{id}/state` (full JSON
  snapshot), `GET /api/bots/{id}/events` (SSE stream that pushes a
  snapshot whenever the bot state version changes), `GET` and the
  static assets.
- `GET /api/config` answers the launch mode.
- `GET /api/bots/{id}/dump` (the plain text state report).
- `POST /api/bots/{id}/commands` (the command queue).
- `GET /api/proxy`, `POST /api/proxy/select` (the client proxy
  selection, see docs/proxy.md).

## Snapshot encoding and the state tracker internals

- Snapshot encoding: the state endpoint and the SSE stream serialize
  the bot state through the direct live encoder
  (`state.Bot.AppendSnapshotJSON`, see `state/snapshot_live.go`): it
  walks the live records under the read lock, builds the per element
  view structs on the call stack and reuses the golden append functions
  of `state/snapshot_json*.go` (the exact bytes encoding/json produces
  - field order, ES6 float formatting, HTML escaping, RFC3339Nano
  times), so the steady state of a watched stream allocates nothing per
  event. Do not route these paths back through `json.Marshal` (its
  reflection walk plus the compacting scan over the MarshalJSON result
  costs 4x the direct write) and do not insert a `Snapshot()` copy in
  between (the copy paid the object, combat, event and chat slice
  allocations, ~26 KB of garbage per event on a 100 npc bot):
  `Snapshot()` remains the deep copy view for the tests and external
  readers, and both paths share the `objectSnapshotLocked` view builder
  - keep them byte identical (pinned by
  `TestAppendSnapshotJSONMatchesSnapshot` and
  `TestSnapshotJSONMatchesReflection`). The invalid UTF-8 bytes of a
  string encode through the probed `jsonInvalidUTF8Replacement`
  (`state/snapshot_json_encode.go`): the classic `encoding/json` writes
  the `\ufffd` escape sequence while the v2 backed stdlib
  (GOEXPERIMENT=jsonv2 and the newer toolchains that ship it by
  default) writes the literal U+FFFD replacement rune - never hardcode
  either form, the writer mirrors the stdlib of the toolchain running
  the tests (both forms pinned by
  `TestAppendJSONStringInvalidUTF8Modes`).
- The world objects live in the SoA halves of the `state.objectStore`
  (the `world` field of the bot): `hot` (an 88 byte `objectHot` per
  object - position, destination, level, kind code, attack flags, clan
  bitmask, speeds, move and combat unix nanosecond stamps) and `cold`
  (an `objectCold` - names, title, template, heading, vitals, social
  marker), length locked and indexed by the same slot, removals swap
  the last records of both halves in. The scans and the movement
  projection walk the hot array only (see scans.go); timestamps store
  unix nanoseconds with 0 as the zero time whose JSON view matches
  `time.Time.UnixMilli` exactly; the object kind is a one byte code
  (`kindCode`/`kindString`). The inventory lives in the dense
  `inventoryStore` (canonical widget order restored once per mutation
  batch, see `inventory_store.go`). The social pull check of the target
  search works on precomputed clan bitmasks (`npcdata.NPCClanMask`),
  and the NPC clans resolve to shared pre-split lists - keep new
  tracker code on those layouts. The tracker itself is split into
  components: `objectStore` (dense SoA world storage + scans in
  scans.go), `inventoryStore`, `eventLog` and `chatLog` (lazily
  allocated rings - a fleet of idle sessions pays no per bot log
  memory), `combatFeed` (the animation feed), with `Bot` as the locking
  facade; the SSE stream of the webserver reuses its frame and payload
  buffers per connection (see `sseStream`), so a watched fleet costs no
  per event buffer garbage.
- Fleet E2E benchmark: `internal/swarm/fleete2e` runs the real 100 bot
  fleet against the live stack (login, elven fighters, hunt loops, the
  24/7 reconnect supervisor) and measures the state layer under the
  real packet load - the aggregate live encode sweep, the engage scan
  sweep and the fleet packet rate. It needs the deployed stack and the
  explicit opt in (`SWARM_FLEET_E2E=1 go test
  ./internal/swarm/fleete2e/ -bench . -benchtime 20x -timeout 25m`);
  the crowded starting area cycles sessions through the emergency
  logout cooldowns, so the fleet reaches a breathing steady state
  (about two thirds online) instead of a static hundred. The in-process
  cache pressure shape lives in `state.BenchmarkFleetScanPressure` and
  `state.BenchmarkFleetLiveEncodePressure` (a hundred 200 npc worlds
  walked back to back - the working set that shows the hot/cold record
  split, the single world benches fit any cache).
- The state tracker (`internal/swarm/state`) is fed by the game session
  from these packets: UserInfo (self vitals, weight and speeds),
  CharInfo (players with speeds and running/dead/combat flags), NpcInfo
  (npcs with heading, speeds, move multiplier, running/dead/combat
  flags and the attackable flag), DropItem 0x16 (fresh drops) and
  SpawnItem 0x15 (items that already exist when they enter the known
  list - for example after a relogin; without it old drops stay
  invisible), GetItem 0x17 (a player picked up an item: removal only -
  the packet position is the item position, not the picker one, so it
  must never snap the picker), MoveToLocation, MoveToPawn 0x75 (chasing
  creatures: stop point at distance from the target, marks combat and
  the target reference - also for the played character itself when it
  chases its attack target), StopMove/ValidateLocation (placement and
  heading), Attack 0x06 (attacker position, hit targets with their per
  hit flags - the Mobius miss flag filters the swing feed: only the
  landed blows animate; marks combat for both sides, attacker faces the
  target, target location refreshes the own position when the bot is
  the target), AutoAttackStart 0x3B / AutoAttackStop 0x3C (auto attack
  flags), MyTargetSelected 0xBF (the own target), TargetSelected 0x39 /
  TargetUnselected 0x3A (targets of other players), ChangeMoveType 0x3E
  (walk/run switch: mobs walk while idle and run when aggroed),
  ChangeWaitType 0x3F (sit/stand transitions, broadcast to the acting
  player itself too), SystemMessage 0x7A (message id plus typed
  parameters, resolved to the client text by the generated
  npcdata.SystemMessageText dictionary - the loot and status lines of
  the chat window), SocialAction 0x3D (idle animations of npcs, player
  emotes and level ups - a short lived ring marker above the animating
  creature on the map; only level ups also become chat lines),
  ActionFailed 0x35 (the one byte refusal answer of the server, logged
  as "Action failed" without further reaction), TeleportToLocation
  0x38 (position snap), StatusUpdate (vitals, weight CUR_LOAD 0x0E /
  MAX_LOAD 0x0F changes, dead when hp is 0; for npcs the server sends
  only CUR_HP/MAX_HP and only to whoever targeted them - npc MP never
  arrives), ItemList 0x27 / InventoryUpdate 0x37 (the inventory: slot
  tracking for the hunt cleanup), DeleteObject (removals). Unknown
  packets land in the event log; setting `SWARM_TRACE_PACKETS=1` logs
  every received packet id for live protocol debugging. The combat
  window is 10 seconds after the last fight packet; the last hit on the
  character is tracked separately (`Bot.SelfUnderAttack`, 3 s) so the
  rest logic never sits into the blows of a running fight.
- The server sends NpcInfo names empty for most npcs (the classic
  client resolves them from NPCName-e.dat by display template id). The
  bot resolves them through `internal/swarm/npcdata`, generated from
  the Mobius data files by `tools/generate_npc_names.sh` (npc: template
  id - 1000000 -> CT0_to_C4_ids.txt -> stats/npcs/*.xml with name,
  level, ai aggroRange/isAggressive; item: stats/items). Regenerate
  after Mobius updates.

## Reproduction harnesses (all exit 1 while their bug is present)

- `tools/repro_movement.js` (`task repro:movement`) for the movement
  interpolation: the simulation mode replays seven patterns (random
  walk, long run, character walk, chase, pickup hops, retarget
  approach, npc chase) against a faithful server simulation (100 ms
  ticks, the exact recurrence, the 1 s broadcast throttle) and measures
  per frame position error against the simulated truth (threshold 0.25x
  speed in world units) and frame speed spikes (threshold 2.5x);
  `--record <seconds> [file] [url] [bot]` captures the live SSE stream
  and `--replay file [--frames]` replays it at 60 fps, printing the
  drawn position of every moving unit per frame and failing on spikes.
- `tools/repro_map_render.js` (`task repro:map`) for the target links,
  unit markers and the static free camera (recording canvas in a Node
  vm sandbox).
- `tools/repro_hud.js` (`task repro:hud`) for the HUD and target panel
  rendering (stub DOM).
- `tools/repro_gear.js` (`task repro:gear`) for the equipment widget
  (paperdoll masks, either-or slot resolution, badges, slot counter,
  the keyed rendering, the pinned footer values, the floating placement
  and the manual interactions) and the shop queue widget.
- `tools/repro_fight_ui.js` for the fight FX gallery harness.
