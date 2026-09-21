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
  pyramid level of the same block stretched over the tile rect, and a
  tile still streaming falls back the same way - the ancestor walk of
  `mapTileAncestor` returns the finest READY level, so the coarse
  ancestors keep the ground painted while the fine tile loads and a
  panning camera (the follow mode of a walking bot) never flashes the
  bare background for the loading gap; the
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
- Zone drawing: the cell mode draws EXACTLY TWO hexagons of the
  uniform grid partition - the ACTIVE one (the cell the bot fights
  in or walks to: the light amber fill, the bright stroke, the focus
  dot and the live label with the farming/moving marker, the respawn
  clock and the measured income; the map answers "which zone is the
  bot going to" through this element alone) and the HOVERED one (the
  hexagon under the map cursor: the dashed outline, the light fill
  and the name label of the mesh record). Every other hexagon stays
  invisible - the full-partition edge raster retired with the
  Voronoi layer (a 1k+ hexagon partition would drown the map). The
  mesh payload arrives once per registry version through
  `GET /api/hunt-mesh` (the ETag is the version) - the mesh bytes
  never ride the per-second snapshot, which carries only the version
  marker and the live record of the held cell. The pointer hover
  resolves the cell under the cursor through the polygon hit test.
  The manual zone panel and the `zone` command retired with the spot
  mode (the
  registry of a full project grows past a thousand cells - the hunt
  economy owns the rotation). The legacy zone mode keeps the square
  drawing of the registry grounds (the active amber, the demoted red,
  the pointer-hover labels) and the same toolbar `hunt zones` toggle
  hides both layers.
- Threat data: the npc level, `aggroRange` and `isAggressive` ai flags
  come from the generated `internal/swarm/npcdata` maps (the C1 data
  pack marks every monster `isAggressive=false`: they only defend). The
  resolved aggro range caps at the server wide MaxAggroRange 450 of the
  Mobius C1 NPC.ini (the NpcTemplate constructor clamps every xml
  aggroRange, most monsters carry 1000, down to it before the
  AttackableAI on-sight check ever runs), so the circles draw the
  radius the server actually attacks from. The map draws the aggression
  radius of every living aggressive mob as a dashed circle around its
  drawn position (amber idle, red once it fights; the `aggro` toolbar
  checkbox hides the layer).
- Social links: every living npc carries its ai `clanHelpRange` and
  clan bitmask in the snapshot (`clanMask` rides the wire as a decimal
  string - the ALL clan bit of the top exceeds the safe integer range
  of JavaScript, the map parses it with BigInt once per snapshot).
  Two npcs of the same clan inside their clan help range connect with
  a solid teal line (attacking one pulls the mate - the Mobius
  notifyActionAttacked clan call walks the attackables within
  clanHelpRange plus the collision radius), a pair that only approaches
  the range (within 1.25x of it) connects with a dashed amber warning
  line, so a spreading pack warns before it actually links. The links
  connect the units, never radius circles - a pack reads as a pack. The
  `social` toolbar checkbox hides the layer; the tooltips carry the
  clan help range of the hovered npc.
- Fleet kill crosses: every recent kill of every bot draws as a small
  orange cross that melts away over five minutes. The marks come from
  `/api/fleet/kills` (the hunt loop publishes its kill ring to the bot
  state, `Bot.SetKillMarks`; the registry merges the rings of all bots
  oldest first, capped at 400) which the web app polls with the bot
  list - the crosses live in the map layer, so they survive the bot
  switches of the view (the per zone kill centroid of the observed bot
  alone did not). The `kills` toolbar checkbox hides the layer.
- Bot switch gap: clicking another bot in the sidebar drops the
  observed bot state (its objects, its zones, its walk line, its
  combat effects) ahead of the new event stream (`MapView.resetBot`),
  but the canvas must not go blank during the reconnect window: the
  imagery blits from the world anchored background cache (the cache
  key survives the gap - the region reads the held camera anchor
  `lastChar`, not the dropped snapshot), the grid, the loaded zone
  frame and the fleet kill ring repaint, and the follow camera holds
  the last known character position until the first snapshot of the
  new bot replaces it. The old behavior cleared the whole canvas and
  collapsed the camera to the world origin, so every sidebar click
  flashed the map white for the stream reconnect - loudest between two
  bots of the same grid where nothing else would change at all. A
  fresh boot before the very first snapshot keeps the blank frame
  (there is no anchor to hold yet). The snapshot driven panels hold
  through the same gap: the equipment widget (paperdoll, bag, the
  skills view), the queue flyouts and the effects panel keep the
  previous bot's content until the first snapshot of the new bot
  diffs their keyed cells in place, so the sidebar click never
  flashes the right side panel blank and the icons the two bots
  share never re-decode (the same hold the HUD answers with; only
  the log resets - a stream panel that would otherwise append the
  new bot's events below the previous bot's history).

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

The tick replay is memoized per object (`projectionCache` of map.js):
the cursor of the last whole tick is kept, so a frame only advances
the ticks that passed since the previous frame. Without the memo a
long walk replayed its whole tick history on every animation frame -
a two minute town trip ran a 1200 iteration loop sixty times per
second, and the loop never shrank back after the arrival snap. A move
signature change (a re-issued move packet, a speed change) resets the
cursor and replays from the packet anchor; the arithmetic sequence is
identical either way, so the drawn position is bit for bit the same
as the from scratch replay (the movement harness pins this).

## Render performance and the fps meter (map.js)

The map renders through one `draw()` pipeline (the rAF animation loop,
every drag mousemove, every zoom wheel tick, every landed tile and
every SSE snapshot repaint). The pipeline is tuned so a zoomed out,
actively dragged map stays at the display refresh rate:

- **One geometry read per frame batch** (`syncView`): the viewport rect
  and the camera (the follow flag, the character or pan anchor) are
  read once at the top of every draw and input handler batch, and
  `worldToScreen`/`screenToWorld` are pure math. The transform used to
  call `getBoundingClientRect` (plus the follow checkbox through
  `centerX`/`centerY`) on every invocation - hundreds of forced layout
  flushes per zoomed out frame between the DOM writes of the tooltip
  and the status chips, which was the drag lag.
- **Fonts and text metrics are cached**: the label font resolves once
  per theme (`labelFont`, `sansStack` of `refreshColors`), and the
  label declutter pass measures every unique text once
  (`labelWidths`). Both used to run `getComputedStyle` and
  `measureText` per draw (per damage number even).
- **Per snapshot derived data is computed once**: the stable draw
  order (`sortedObjects`) and the footer object count line
  (`objectsText`) are built in `update()`; the footer chips re-write
  their DOM nodes only when the text actually changed
  (`setChipText`), so a pan or an animation frame dirties no layout.
- **The social links precompute their screen positions** once per
  unit (the pair loop transformed the same position per candidate
  pair), and the aggro circles set their shared style once for the
  whole pass instead of a save/restore round trip per mob.
- **The map tiles draw with `imageSmoothingQuality = "low"`**: the
  default `"high"` filter of the zoomed out frames downscaled a
dozen 512px tiles per paint and is visually indistinguishable on
  terrain imagery.
- **A grabbed map skips hover hit testing**: the drag itself repaints
  on every mousemove; re-querying what sits under the cursor on top
  of it doubled the per event work.
- **The static world renders from an offscreen cache**: the map or
  geodata tiles, the grid and the loaded zone frame are camera-only
  data, but the render loop repainted them on every animation frame
  while anything moved - the open map cpu load. They now rasterize
  once into an offscreen canvas anchored in world coordinates (one
  viewport of slack around the view, the device resolution capped so
  the raster stays in the tens of megabytes), and every frame
  composites it with a single `drawImage` - a GPU side copy instead
  of a per frame re-raster of dozens of scaled tiles. The cache
  re-renders only on a zoom change, a layer toggle, a committed tile
  batch, a theme flip, a resize, or the camera leaving the slack box
  (a walking follow camera re-renders every half viewport of travel;
  a drag re-renders every viewport). The blit offset snaps to whole
  device pixels so the grid stays crisp at rest. While the map tab is
  hidden the render loop stops and the data driven repaints
  (snapshots, kill polls, tile loads) skip the hidden canvas; the next
  snapshot repaints within one 300 ms poll.
- **The tile arrivals commit in throttled batches** (`tileArrived`,
  `tilesCommitMs`): a zoom-out makes the whole visible world start
  loading at once, and the burst lands dozens of tiles over seconds.
  A raw arrival counter in the cache key meant every one of those
  arrivals dropped the key, so every animated frame of the load
  window re-rasterized the entire static world - the low fps of a
  loading map that recovers once the burst goes quiet. The arrivals
  now commit in batches: the first tile of a window rasterizes
  immediately (the first coarse imagery appears at once), the rest of
  the burst coalesces into one trailing commit at the window end, and
  the render loop between them stays blit only. A streaming load
  costs at most a couple of cache rasters per second instead of one
  per frame, and the final state always lands (the trailing timer
  fires even with the render loop idle and the tab hidden). The
  image loads also go through `decode()`, so the jpeg decode of a
  landed tile happens in the image pipeline instead of the first
  `drawImage` inside a cache re-render.
- **The hunting zones render from their own offscreen cache**: the
  zoomed out view has most of the registry on screen at once, so
  re-stroking the shapes on every animation frame was the cpu load
  that survived the background cache. The cell mode draws the static
  partition edges into a version-keyed raster (the mesh never changes
  at runtime - the cache key is the version, the zoom and the canvas
  geometry alone, the active cell highlight paints fresh per frame as
  one polygon plus its label); the legacy spot/square registry keeps
  the shape-keyed cache below. The shapes (the circles, the squares, the anchor
  dots, the kill centroid crosses) now rasterize into a second world
  anchored cache exactly like the static world (the same slack box and
  device pixel cap, sized for thin strokes instead of imagery) and
  every frame composites them with one more `drawImage`. The layer
  re-renders only when its pixels actually change - a zoom step, a
  resize, a camera pan beyond the slack box, or a visual registry
  change (a zone switch, a death heat bucket step, a kill centroid
  move; see `huntZoneVisualKey`). The per second economy fields (the
  respawn countdown, the adena rate, the occupancy, the death count)
  deliberately stay out of the cache key: they only feed the label of
  the hovered or listed zone, which `drawZoneEmphasis` paints fresh on
  top of the cached shapes per frame (at most two zones match, a
  couple of shapes), so the countdown still ticks live without
  re-rastering ~290 circles every second. The base raster itself draws
  in one path per style group (all future circles in a single stroke
  call, the heat fills bucketed by alpha) instead of a save/restore,
  two dash arrays and a label concatenation per zone.
- **The kill crosses batch by fade bucket**: the fleet kill ring caps
  at 96 marks; each cross used to cost its own begin/stroke round
  trip per frame, the fade now quantizes into eight buckets that share
  one stroke call each (a bucket step is invisible on a five minute
  melt). The aggro circles skip sub pixel radii at the far zoom - a
  circle that reads as a dot is unreadable clutter anyway, and the
  packed field of the zoomed out view no longer strokes hundreds of
  them.

The **fps meter** makes the frame budget visible: the counter item at
the right end of the status bar (the app footer, `#foot-fps`) shows the
map render rate of the last half second window with the average paint
cost (`fps: 58 · draw 2.1 ms`, the worst frame joins when it spiked -
`worst 40.2 ms`), colored green at 45 fps and up, amber at 28 and up,
red below. It lived on the map canvas corner before and overlapped the
HUD panels at the narrow window widths. Every paint counts - the rAF
loop and the event driven repaints alike - and when nothing has painted
for a while the chip reads `fps: idle` (the render loop stops when
nothing can move; a stopped loop is a fact worth seeing, not a frozen
reading). Every five seconds the same numbers land in the browser
console (`map fps: 58 fps · draw avg 2.1 ms · worst 3.4 ms`), so a lag
report pastes the measurements next to the build identity of the state
dump.

The **cursor chip** (`#foot-cursor`, left of the fps counter) shows the
world coordinates under the mouse - the bare x y pair of the world
point, upgraded to the full x y z triple while the pointer holds a
walk plan waypoint (the dump walk line format). The `ctrl+c` (or
`meta+c`) pressed with the pointer over the map copies exactly what
the chip shows (the selection aware fall through never masks the
browser copy: a shortcut with the pointer off the map or an active
text selection goes through untouched) and flashes `copied: ...` into
the chip for a moment. The 3D navmesh viewer coordinate inputs accept
the copied lines: the wp prefixed dump lines bind the first three
numbers after the `wp` marker (the timing suffix carries numbers of
its own), every other line keeps the last three numbers contract, and
a bare x y pair inherits the z from the other line.

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
  of the currently selected object (name, level chip, kill ETA chip,
  HP bar, MP row that reads no data for npcs - the C1 server never
  sends their MP). The kill ETA chip (`#target-eta`, accent colored)
  shows the `diagnostics.hunt.killEtaMs` estimate rounded to whole
  seconds (~12s) while a confirmed fight runs at least 2 seconds: the
  rate is the damage dealt over the elapsed fight, the estimate is
  the remaining health over the rate. Without an estimate (no fight,
  too fresh, no HP data) the chip hides. The status banner appends
  the walk ETA (`walkEtaMs` - the remaining plan length over the run
  speed) to the walk phase details and the kill ETA to the fight
  detail (`eta ~Ns`, `web/app.js etaSuffix`).
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
- Session dump: the Session dump button of the map toolbar (next to
  the state dump) copies the compact session report of the whole run
  (`GET /api/bots/{id}/session-report`, the session journal package)
  - the hourly state curves, the kill and death statistics, the
  money trail, the stalls and the story tail. The long-run analysis
  material of an 8-24 hour session in one click; the offline
  post-mortem renders the same report through `-session-report
  <file>` (see [session_journal.md](session_journal.md)).
- Pathfind link: the pathfind link button of the map toolbar (next to
  the session dump) copies a 3D navmesh viewer URL of the current
  walk (`buildPathfindLink` in app.js) - the `from`/`to` pair off the
  published walk plan (the planning origin and the final
  destination), the tile keys around the pair (the bounding box
  grown by half a tile), the default `scale=1 geom=mesh path=smooth`
  toggles and a camera pose computed with the viewer framing math
  (the three quarter orbit south east of the route midpoint, the
  analytic yaw/pitch of `frameInitialTiles`). Opening the link in the
  `-show-navmesh` viewer reproduces the route context and answers the
  route itself - the fast path to experiment in the 3D world or to
  attach a reproducible route to an agent report. The base address
  defaults to the documented local viewer
  (`http://127.0.0.1:8082/`); the shift click asks for a different
  one and remembers it in the localStorage
  (`swarm.pathfindViewerBase`). A snapshot without a published plan
  opens the viewer bare (no route pair, no camera, the defaults
  stay).
- Pathfind link repro contract: a walk plan that answers a mesh
  search carries that search's contract
  (`state.WalkPlan.Search` -> the `walkSearch` snapshot field: the
  approach radius, the ban circles), and the link replays it -
  `approach=200` for the trip searches, `avoid=x,y,r;...` for the
  frozen area bans and `fold=0`
  (the plan repro mode: the viewer serves the search answer as the
  bot publishes it, skipping the capsule post pass whose grid
  oracle is water blind and folds the route the bot never walks -
  the 2026-09-19 mismatch report). No filter parameter exists: the
  priced round of 2026-09-19 retired the walled form of the water,
  every search prices the crossings at the swim rate (swimming is
  slower than running) and the plan may swim. A plan from no mesh
  search (the direct segments)
  keeps the viewer defaults. The viewer POST body carries the same
  fields (`approach`, `avoid`, `fold`) and the shared view links
  round-trip them (`parseViewParams` / `buildViewStateUrl` in
  navmesh_view.js), so a double click experiment under a pasted
  repro link answers under the plan's own conditions. The state
  dump names the mesh word in the walk plan header
  (`17 waypoints, mesh, aiming at wp 1`) and prints the full
  contract on its `search approach 200 avoid ...` line, so a pasted
  dump restores the repro contract through the dump parser too.
- Bot status banner: a compact chip pinned to the top center of the
  map (`#bot-status` in index.html, `renderBotStatus` in app.js) shows
  the current activity of the active bot at a glance - hunting,
  looting, walking to town, selling, walking to farm spot, deleveling,
  manual move, idle. The activity text and the data-kind attribute come
  from `phaseLabel(snap)` which maps the hunt loop phase
  (`snapshot.phase`, published by `state.Bot.SetPhase` from the hunt
  loop tick through a defer) to a human readable label and color kind.
  The detail stays straight to the point: the goal phrase and the
  measured progress only (the engagement age, the waypoints left, the
  trip age, the eta) - the fighting detail resolves the mob name from
  the snapshot objects exactly like the target panel does (the raw
  object id never shows), and the context restatements ("in the zone"
  and the like) stay out. The session status takes precedence when no
  phase is published (the
  manual only sessions never set the phase): the banner falls back to
  the connecting/offline text. The dot pulses while the bot is active
  so the banner reads as live; the pathfind test mode hides it.
- Walk path view: the map draws the published walk plan
  (`snapshot.walkPath`) as a blue dashed polyline from the planning
  origin (`snapshot.walkOrigin`, falling back to the character when
  absent) through every planned waypoint - the passed ones included,
  so the drift of the character against its own plan is the debugging
  signal. The publishWalkPlan path covers every walking phase of the
  hunt loop, not only the manual move: town trips (the walk to the
  trader and the walk back to the farm spot) and the deleveling guard
  walks publish their full geodata segment too, so the map draws the
  planned path of every autonomous walk. The follower cursor
  (`snapshot.walkIndex`) carries a small ring on the waypoint the
  walker currently aims at, and the plan clears on the non walking
  phases (engage, loot, sell, idle) through the `ClearWalkPlan` call
  of the tick. The destination marker (the pulsing blue dot) draws at
  `snapshot.walkDest` (the merchant spawn, the farm spot, the clicked
  point), falling back to the last waypoint when the plan carries it.
  While a manual move runs, the loop publishes the walk plan into the
  tracker (`state.Bot.SetWalkPlan`: the origin, the full waypoint
  list, the follower cursor and the destination, refreshed every tick,
  expiring on its own after 2 s without a refresh); the map draws it
  while the paths toggle is on. The coordinate labels of the
  waypoints draw on hover only: the constant per waypoint labels
  littered every planned walk, the hovered waypoint prints its full
  x y z triple (the dump walk line format) next to the dot instead,
  and the status bar cursor chip plus the ctrl+c copy carry the same
  values (see the cursor chip below). The state dump
  (`/api/bots/<id>/dump`) prints the same whole segment with the origin
  ("from"), the walk zero point (the `started` line with the last
  seen moment and the time on the walk), every passed waypoint with
  its timing (`(passed, t+10.4s, segment 5.2s)` - the moment the follower
  reached it on the walk timeline and the segment duration that ended
  there) and the `<-- TARGET` marker on the current one with its
  walking time (`(walking 45.2s)`, the stuck segment number - the last
  walk plan measures the aimed segment to the moment the plan ended), and
  the `dest` line last - a stuck or dawdling walk reads at a glance
  (the 2026-09-10 water stuck report drove the format: the walk plan
  (3 waypoints) of the dump hid the northern escape segment the re-path
  had planned and the character had skipped past; the 2026-09-19
  timing round added the per waypoint cost so a dawdling point shows
  how long it eats, not just its name). The tracker observes the
  arrivals through the cursor advance of the every tick republish
  (`walkWpAt`, see `publishWalkPlanLocked`), so the resolution is the
  250 ms hunt tick and the last walk record keeps the timing after
  the walk ends.

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
  head, cloak / weapon, chest, shield / gloves, segments, boots) on the
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
  the text badges, and reordering moves the persistent cells. The
  observed bot switch rides the same keys: the widget survives the
  switch gap (no wipe - a wipe rebuilt every cell and flickered the
  panel on every sidebar click), the first snapshot of the new bot
  diffs the cells in place, and only the in-flight item interactions
  cancel (an armed drag or an open drop dialog would post the old
  bot's item against the newly observed bot). A pinned
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

The equipment widget is a three view widget that behaves like real
tabs - switching the mode fully replaces the visible content. The
EQUIPMENT / SKILLS / QUEST mode tabs replaced the static title row.
The gear content stays in the flow and keeps sizing the panel, the
skills view is an absolutely positioned overlay of exactly that area:
the hidden view turns invisible (`visibility` swap, never `display
none` - the panel must not change its dimensions) AND the overlay
carries `z-index: 4`
so it paints above every gear child - the paperdoll icon, glyph and
badge cells stack at z-index 1..3 and would otherwise bleed through a
z-index auto sibling (the mode class lands on `#gear-main`, which
must carry that id in the markup). The skills view carries the ACTIVE
/ PASSIVE filter tabs, the learned skill grid and the pinned sp/next
foot (the SP wallet and the head of the learning queue, anchored at
the panel bottom like the adena/weight footer); the mode and the
filter persist in localStorage. The QUEST items are not a widget mode:
they are the second tab of the inventory area - the INVENTORY / QUEST
tab pair (the `.skill-filter-btn` idiom, plus the count badge) sits in
the inventory title row, and the quest grid (`#quest-grid`, the keyed
`QuestCells` registry - one element cannot sit in the bag grid and the
quest grid at once) is a sibling of the bag grid inside the gear view,
so the swap never changes the panel height (`.inv-grid.hidden` leaves
the flow; the absolute `.quest-empty` note covers the empty state).
The grid shows only the type2 quest family items of the inventory
(`isQuestItem`, type2 === 3 of the Mobius item packets); the bag keeps
showing
the quest items too (the trash and the drop flows work from it).
The sub tab persists in `swarm.invTab`; the retired "quest" widget
mode value in `swarm.gearMode` migrates to it on boot.
The learned grid is a small fixed
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

## The effects panel (buffs.js)

The floating frame right of the character HUD (left: 272, top: 10 of
the map wrap) renders the server effect list (`snapshot.buffs`, the
AbnormalStatusUpdate view: skillId, level, name, icon, the remaining
seconds `left` and the landed duration `total`) in two view states
that share one frame and morph into each other (the frame width and
the body height transition, the layers cross-fade with a slight
settle, the chevron rotates in place):

- the icon grid (the default): one 32px cell per active effect, 10
  columns wide and at most 2 rows for the classic buff bar (more
  than 20 active effects scroll inside the grid, the dock keeps the
  two row height), every cell packs edge to edge and paints its own
  2px white strip on the right and the bottom edge (the separators
  run only between the icons that actually show - a white grid
  background would leave a white hole under the empty cells of a
  partially filled row on the dark theme), the short remaining time
  overlays the cell bottom edge and the level badge sits top right;
- the detailed list: the full effect rows (icon, level badge, name,
  the human remaining time) with the remaining time percent bar
  pinned to every row bottom edge (left over total, the fill eases
  toward the next tick), the tight vertical rhythm and the thin
  scrollbar - the body height caps at the character HUD stack height
  (synced on the view change, the window resize and the HUD
  ResizeObserver, the frame chrome subtracted) so the expanded panel
  never outgrows the character widget.

Exactly the active effects show in both states (two effects build two
cells and two rows); the panel hides entirely while no effect runs.
The left dock is the slim vertical strip stretching with the frame
(one row of cells docks one row tall, two rows dock two rows tall);
it carries the expand chevron and the view switch icon, both buttons
toggle between the states and neither ever moves (the chevron stays
pinned to the top left corner in both states), so collapsing and
expanding again needs no re-aim (the entering layer cancels the
visibility delay - the fade-in paints immediately, the hiding layer
finishes its fade before it un-hooks). The view choice persists in
the localStorage (`swarm.buffsView`; the panel answers the restored
choice on boot). The countdowns run locally: every snapshot entry
anchors its reading (the anchor carries the skill fields plus the at
timestamp) and a 1 Hz ticker counts the elapsed wall clock off it, so
the times keep running between the server snapshots (the SSE poll
pushes at most every ~300 ms while the bot runs and stops when it
idles). The keyed rendering survives the module move: the cells and
the rows are persistent DOM nodes keyed by the skill id, a buff
joining or leaving touches only its own node (the icons never blink
on a refresh), the panel holds the previous bot's content through
the switch gap like every snapshot driven panel. The panel code
lives in `web/buffs.js` (loaded before `app.js`, which calls its
`renderBuffs` from the shared render path); the harness is
`tools/repro_buffs.js` (`task repro:buffs`).

## Chat window

The bottom left corner of the map shows the world chat lines, the
parsed system messages and the social animations (`snapshot.chat`, a
rolling 64 line ring fed by ApplySystemMessage/ApplySocialAction and
ApplySay). The auto scroll follows the newest line only while the view
is at the bottom (`chatAtBottom`, 4 px tolerance): scrolling up
detaches the follow to read the history, scrolling back to the bottom
resumes it. The list itself is the scroll container (the box clips,
the list scrolls). SystemMessage texts resolve through the generated
`npcdata/system_messages.go` dictionary (id -> client text with $sN
placeholders, substituted positionally with the packet parameters;
item and npc name parameters resolve through the item and npc
dictionaries). Regenerate with `task generate:system-messages`
(tools/generate_system_messages.sh) after Mobius updates.

The three tabs split the stream: ALL shows everything, CHAT keeps the
world chat kinds of the CreatureSay packet (say, shout, whisper,
party, clan, trade, announcement - the channel to kind mapping lives
in `state.ChatEvent`), SYSTEM keeps the bot system messages and the
social lines. A world chat line renders the sender as its own column
(`ChatEvent.from`) and colors by channel (shout/whisper/trade stand
out). Every row pins the same whole pixel line height (18px on
`.chat-line`): a unitless ratio (the earlier 1.6 at 11px = 17.6px)
rounds per row at paint time and the vertical distance between the
lines drifted apart on some rows - taller glyph fallback boxes (emoji,
arrows) widen the line box the same way; the wrapped message lines
follow the same 18px rhythm. The input row below the list sends a chat
message through the
bot: the channel select (all, shout, trade, party, clan, whisper),
the whisper recipient input (whisper only), the 105 character bound
of Say2 and the empty text refusal are validated on both the client
and the server side; the message rides the say command
(POST /api/bots/{id}/commands, one shot like useItem) into
GameClient.Say, and the own CreatureSay echo lands back in the
window. The CreatureSay system variant (int charId + messageId
instead of the two strings) and the NpcSay packet are not parsed -
they are not chat lines of this window.

## Map toolbar

One compact row (a 37 px bar, never a wrapped checkbox column). The
old `-`/`+` zoom buttons are gone - the wheel owns the zoom alone
(cursor-anchored, `onWheel` of `web/map.js`). The layer checkboxes
(labels, paths, zone, targets, hunt zones, aggro, social, kills, map
background) fold
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
the geodata path once and follows the waypoints in server accepted segments
(userWaypoints, 1s request pace, segments capped at 1000 units). A manual
command that replaces a walk still running on the server re-issues the
walk request at once (`userRedirect`: the next tick fires the new
MoveToLocation instead of waiting for the old walk - the server
replaces the destination of a running walk), so a click somewhere else
changes the direction immediately.

## The statistics tab

The Stats tab (`web/stats.js`, the collector of
`webserver/stats.go` and the endpoints of `webserver/stats_api.go`)
answers the behavior statistics of the bots: how effectively they
fight, how they behave, how often they die and rejoin the game - the
fleet wide picture and the per bot picture side by side, with the
long term history a 24/7 run needs (the rings hold the last day at
full 15 second resolution, older data ages into coarser steps
through the half-on-full compaction).

The fleet overview renders:

- the KPI cards: bots online, kills (with the hourly rate), deaths
  (with the K/D), rejoins, the net experience gained, the adena income
  (the net gain of the fleet with the per hour rate), the average
  hunt tick, the process memory (heap and sys), the goroutine count
  (with the GC count and the last pause), the fleet packet rate, the
  collecting window and the landed swing share (the hit rate);
- the history charts: online/registered bots, the cumulative kills
  and deaths, the per minute kill/death rates, the net experience,
  the fleet adena wallets, the average hunt tick time (the loop
  cadence health of a loaded process), the process memory, the fleet
  packet rate and the goroutine count;
- the sortable comparison table of every registry bot (the kills,
  deaths, K/D, kills per hour, the net experience, the adena per hour,
  the rejoins, the hit rate, the damage taken, the average tick, the
  uptime); a row click opens the detail view of that bot. The
  acceptance test bots are listed but stay out of the fleet
  aggregates.

The per bot detail view adds the counter cards (the level with the
window gain, kills, deaths, K/D, the net experience, the rejoins and
sessions, the swing counters with the hit rate, the damage taken,
the adena wallet with its net gain and per hour rate, the average
and the worst tick, the packet rate, the uptime, the ages of the
last kill and death), the per bot history charts (experience with
the level staircase, kills and deaths, health, the adena wallet with
the net gained line, tick time, packet rate), the phase timeline
strip, the phase distribution (the share of the window spent in every
hunt phase) and the event timeline (the kills, deaths, rejoins, level
changes and the online/offline transitions of the collected
history).

The counter sources (`state/metrics.go`): a kill counts when the
object the character actively fights dies (the fighting target of
the last swings, packet level attribution - a kill stolen between
two swings counts too, the solo farm case is exact), a death counts
on the alive to dead HP transition of the character (the village
restart revives without counting), a session counts every
`ResetSession` (the first one opens the deployment, every later one
is a rejoin), the swing counters split the made/landed/taken blows
of the Attack broadcasts by the miss flag, the damage taken
accumulates the observed HP drops, and the hunt loop feeds its tick
duration through `NoteHuntTick` (an EMA plus the worst tick of the
last minute). The counters survive the session resets: a 24/7
process reports the whole deployment story.

The window selector (hour, 6 hours, day, everything) refetches both
views with the matching `?window=` parameter; the endpoints
downsample the rings to at most 256 points per response. The
downsampling picks the LAST sample of every epoch aligned time
bucket (the bucket width derives from the requested window alone),
so the served points are a pure function of the sample timestamps:
consecutive polls of one window derive the very same points, a
fresh sample only refreshes the trailing bucket and the window
slide removes points at whole bucket granularity - the charts of a
live poll cycle never flicker (an index stride re-aligned its picks
on every change and the whole chart visibly jumped).

Three stability rules of the collected series matter for the chart
readability:

- The experience is the C1 cumulative total (UserInfo broadcasts
  `(int) getExp()`, the level derives from the experience table), so
  the net gained line grows linearly THROUGH the level ups and never
  resets; the level itself rides the same chart on its own right
  hand scale drawn as a golden staircase (the level ups step up, the
  delevel penalties step down).
- A sample that lands inside the reconnect gap (the tracker
  character zeroed by `ResetSession` before the fresh UserInfo
  arrives) carries the last known character values: the exp, adena
  and level of a bot do not change while it is away, and recording
  the zero flash crashed the exp chart to the bottom and faked
  "reached level 0" events. The live view holds the same last known
  state through the gap.
- The adena wallet comes from the inventory store
  (`SelfSnapshot` sums the adena items), the net gained and the per
  hour rate derive from the series baseline the first valid sample
  anchors.

The tab polls every 5 seconds while it is visible and stops while
another tab holds the screen. The event list renders keyed: an
unchanged event set keeps its DOM (only the ago labels refresh in
place), a new event rebuilds the rows. The charts draw on plain
canvases with the theme colors of the CSS variables - no framework,
no bundler, no network dependency; every dynamic text lands through
`textContent`.

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
- `GET /api/stats` (the fleet statistics view: the live counters of
  every bot, the fleet totals and the downsampled fleet history),
  `GET /api/stats/{id}` (the per bot view with the phase
  distribution, the event timeline and the per bot history); both
  take `?window=<seconds>` (the default day, 0 walks everything
  collected).

## Snapshot encoding and the state tracker internals

- Dump state diagnostics: the snapshot ends with a `diagnostics`
  section - the report payload for stuck or misbehaving bots on a
  live server (`state/diagnostics.go`). The liveness half is computed
  by the encoders from the live state: `phaseForMs` (the age of the
  current hunt phase, `SetPhase` stamps it - a phase age of minutes
  is the top stuck signature), `updatedAgoMs` (the age of the last
  state change, the session start while nothing changed yet),
  `packetsPerSecond` (the 10 second window `CountPacket` feeds), the
  `loginCooldownMs` remainder of an emergency logout, the combat
  nuance (`autoAttacking`, `fightingTargetId`, `combatActiveAgoMs`,
  `lastHitAgoMs`, `underAttack`, the `attackerCount` tally of the
  object walk), the walk freshness (`walkFresh` exposes the stale
  moving flag of a lost stop packet with its `moveAgoMs`) and the
  `objects` summary (npc/player/item/dead counts, collected during
  the object walk without a second pass). The `hunt` subview carries
  the internals the hunt loop publishes every tick
  (`Loop.diagnostics` through `SetHuntDiagnostics`: the target and
  its engagement age, the active skip count of both skip maps, the
  no-target patience, the re-path count, the stuck watchdog age, the
  waypoints left, the trip and flee episode ages, the buy retries,
  the stagnation stall ages `xpStallForMs` and `positionStallForMs`
  - the time the experience and the exact standing cell have been
  static, the livelock watch inputs of hunt/stagnation.go - whose
  recovery escalation also drives the unsticking: the first
  position stall clears the frozen loop state in place, a
  surviving stall or the experience window rebuilds the session
  through the emergency logout, so the dump `events` carries the
  recovery lines next to the stall)
  plus the tracker owned `lastAction` with its age and the
  `tickAgoMs` loop heartbeat (a growing value means the loop
  goroutine stopped ticking while the session stays online). The
  hunt decision log lines route through `Loop.logf`: they print on
  the console logger and land in the tracker event log
  (`Bot.NoteAction`), so the dump `events` array carries the
  decision history of the session. Age values floor to whole seconds
  through `state.AgeMs` (0 means the reference never happened, not
  now); the diagnostics publication never bumps the state version -
  the values ride the snapshots the packet traffic drives. The web
  UI footer, the activity banner and the log tab colors read the
  same section.
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
  unit markers, the static free camera, the grab semantics of map
  panning and the fps meter chip (recording canvas in a Node vm
  sandbox).
- `tools/repro_hud.js` (`task repro:hud`) for the HUD and target panel
  rendering (stub DOM).
- `tools/repro_buffs.js` (`task repro:buffs`) for the effects panel:
  the markup (no EFFECTS head, the dock buttons, the layers), the
  styles (the 10 column grid, the per cell white separators, the
  scroll, the morph transitions), the render (exactly the active
  effects, the keyed nodes surviving the refreshes, the percent bars
  from left over total), the local countdown ticker, the view toggle
  with the persistence and the height fallbacks (stub DOM).
- `tools/repro_gear.js` (`task repro:gear`) for the equipment widget
  (paperdoll masks, either-or slot resolution, badges, slot counter,
  the keyed rendering, the pinned footer values, the floating placement
  and the manual interactions), the shop queue widget and the
  observed bot switch: the widget DOM survives the switch gap with
  the shared cells and icons kept, the in-flight drag and drop dialog
  cancel, and the new bot's first snapshot diffs the keyed cells in
  place (the right side flicker fix). The effects panel checks live
  in `tools/repro_buffs.js` since the panel code moved to
  `web/buffs.js` (the gear sandbox loads app.js only).
- `tools/repro_stats.js` for the statistics tab: the fleet overview
  (activation fetch, KPI cards, chart drawing, the bots table, the
  bot selector), the bot detail view (KPI cards, events, phase
  distribution, the timeline strip), the window switch and the
  polling stop.
- `tools/repro_fight_ui.js` for the fight FX gallery harness.
- `tools/repro_bot_switch.js` for the observed bot switch: the map
  resets the previous bot's world (snapshot, runtime objects, social
  masks, combat effects) ahead of the new stream, keeps the fleet
  kill marks, paints the static world through the switch gap with the
  camera held on the last known position (the white flash fix) and
  repaints the new bot's zones on its first snapshot.
