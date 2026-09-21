/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// MapView renders the world around the active bot on a canvas. The map is
// the source of truth: the character status lives on it as a HUD panel,
// world objects are drawn as circle + look direction ticks like L2Bot,
// colored by threat level, and a request animation frame loop interpolates
// every movement between the server updates so positions are always
// current. The interpolation is calibrated against the Mobius arrival
// semantics (the server stops creatures a collision radius plus one game
// tick short of the destination and announces the arrival with a zero
// distance MoveToLocation), so units complete their path exactly when the
// arrival packet lands instead of teleporting the last stretch.
const MapView = {
  canvas: null,
  ctx: null,
  tooltip: null,
  // The hovered kill skull and the age line element the tooltip
  // refreshes (the age read ticks while the cursor rests on it).
  hoverMark: null,
  hoverAgeEl: null,
  scale: 0.12,
  panAnchor: { x: 0, y: 0 },
  drag: null,
  hover: null,
  lastSnap: null,
  // The last known world position of the observed character. The
  // observed bot state drops on a bot switch (resetBot), but the
  // camera and the static world must not: the anchor holds the view
  // in place while the new event stream reconnects, so a switch
  // paints the same map area it painted before instead of collapsing
  // to the world origin behind a cleared (white) canvas. The first
  // snapshot with a real position overwrites it.
  lastChar: null,
  clockOffsetMs: 0,
  clockSamples: [],
  runtime: new Map(),
  animating: false,
  lastFrame: 0,
  colors: null,
  mapTiles: new Map(),
  // Geodata visualization tiles of the pathfind test mode: rendered
  // region images (heightmap / walls / layers) served by the bot
  // process, keyed by url so switching the mode keeps the old tiles.
  geoTiles: new Map(),
  // Pathfind test mode state (enabled by PathfindUI): draggable start
  // and end markers plus the last search result overlay. Null in the
  // regular bot mode so every hook below stays a no-op.
  pathfind: null,

  // The hunting zone under the map cursor: null unless the mouse
  // rests inside a zone shape (spot circle or legacy square). The
  // hovered zone draws its name label - the zone names stay hidden
  // otherwise, the far zoom showed a smeared blob of labels.
  hoverZone: null,

  // The walk plan waypoint under the map cursor (the index into the
  // published walkPath, -1 when none): the coordinate label of a
  // waypoint draws on hover only - the constant per waypoint labels
  // littered the map on every planned walk, the footer cursor chip
  // and the ctrl+c copy carry the values instead.
  hoverWp: -1,

  // The world point the map cursor holds: the footer chip readout
  // and the ctrl+c copy source. Null while the pointer is off the
  // map (the chip reads the em dash then, the copy falls through to
  // the browser default).
  cursorWorld: null,

  // The fleet wide kill marks of /api/fleet/kills (every recent kill
  // of every bot): drawn as the skulls of the whole deployment so
  // they survive the bot switches of the view (the per zone kill
  // centroid of the observed bot alone does not).
  killMarks: [],

  // The parsed clan masks of the current snapshot (objectId to
  // {low, all}): the low 44 bits carry the clan alphabet as a plain
  // number (the pairwise AND stays a cheap integer op), the all flag
  // marks the ALL clan that links with every clan carrier. Rebuilt
  // per snapshot - the mask strings would otherwise parse to BigInt
  // on every frame of the social layer.
  socialMasks: new Map(),

  // The world point of the last manual command (a map double click):
  // drawn as a fading ring so the click answer stays visible while
  // the bot walks there.
  userMark: null,

  // ---- the per batch viewport and camera cache ----
  //
  // The map used to read getBoundingClientRect (and the follow
  // checkbox) inside every worldToScreen call: a zoomed out frame
  // fires it hundreds of times (tiles, kill marks, every object twice)
  // and each read between the DOM writes of the tooltip and the status
  // chips forces a synchronous layout - the drag and far zoom lag was
  // a layout thrash, not a draw cost. The geometry is now read once
  // per frame batch into these fields (syncView) and the transform is
  // pure math.
  view: { width: 800, height: 600, left: 0, top: 0 },
  followOn: false,
  camX: 0,
  camY: 0,

  // sortedObjects: the stable draw order of the snapshot objects
  // (dead first, then north to south, then object id), sorted once per
  // snapshot instead of once per frame.
  sortedObjects: [],

  // objectsText: the object count line of the map footer, derived
  // from the snapshot alone - it cannot change between snapshots, so
  // updateMapInfo re-writes the chip only when it actually differs.
  objectsText: "",

  // chipTexts: the last written text of the footer chips, so the
  // per frame info update writes the DOM only on a real change.
  chipTexts: {},

  // labelWidths: the measured pixel width of every label text. The
  // declutter pass called measureText per label per frame; the names
  // repeat across frames, so the width is measured once per text.
  labelWidths: new Map(),

  // The resolved fonts of the map text (getComputedStyle per draw
  // was a forced style resolve every frame): the zone and unit label
  // font and the sans stack of the damage numbers.
  labelFont: "600 10.5px sans-serif",
  sansStack: "sans-serif",

  // projectionCache: the per object cursor of the server movement
  // replay (see projectTickwise) - the tick recurrence is memoized at
  // the last whole tick so a long walk costs O(ticks since the last
  // frame), not O(ticks since the move started) on every frame.
  projectionCache: new Map(),

  // ---- the fps meter ----
  //
  // Counts every painted frame (the rAF animation loop and the event
  // driven repaints alike - a drag that repaints on every mousemove is
  // a real frame budget user) and the wall time each paint took. The
  // chip in the map corner shows the last half second window; every
  // five seconds the same numbers go to the console so a lag report
  // carries the measurements. The fields are mutated in place - the
  // meter must never allocate in the paint path.
  fps: {
    paints: 0, drawMs: 0, worstMs: 0, windowStart: 0,
    lastChip: "", lastChipAt: 0, lastLogAt: 0, chip: null
  },

  // ---- the static background cache ----
  //
  // The map tiles, the grid and the loaded zone frame are camera
  // only data: painting them costs the same whether the objects
  // moved or not, and the render loop repainted them on every
  // animation frame while anything moved (a hunting bot keeps
  // something moving almost always - that was the open map cpu
  // load). The cache rasterizes them once into an offscreen canvas
  // anchored in world coordinates (one viewport of slack around the
  // view), and every frame blits the visible slice with a single
  // drawImage - a GPU composite instead of a per frame re-raster of
  // dozens of scaled tiles. The cache re-renders on a zoom change, a
  // layer toggle, a committed batch of landed tiles (see
  // tileArrived), a theme flip, a resize, or the camera leaving the
  // slack box (see bgKey and ensureBackground).
  bg: {
    canvas: null, ctx: null, key: "",
    cx: 0, cy: 0, worldLeft: 0, worldTop: 0,
    cssW: 0, cssH: 0, dpr: 1, marginX: 0, marginY: 0,
    // tilesCommitted is the arrival count the cache key rides (see
    // tileArrived): the raw tileLoads counter would drop the key on
    // every arrival of a streaming burst, so the arrivals commit in
    // throttled batches instead. commitAt is the timestamp of the
    // last commit and commitTimer holds the trailing batch.
    tilesCommitted: 0,
    commitAt: 0, commitTimer: 0
  },

  // huntBg caches the hunting zone layer the same way the bg cache
  // holds the static world: the circles and the squares of the
  // registry rasterize once into an offscreen, world anchored canvas
  // and every frame composites them with one drawImage. The elven
  // spot registry alone carries ~290 grounds and the zoomed out view
  // has most of them on screen at once, so the per frame re-stroke
  // of the dashed circles was the cpu load that survived the
  // background cache (see huntZoneVisualKey for what invalidates
  // it - the live economy label fields deliberately do not).
  huntBg: {
    canvas: null, ctx: null, key: "",
    cx: 0, cy: 0, worldLeft: 0, worldTop: 0,
    cssW: 0, cssH: 0, dpr: 1, marginX: 0, marginY: 0
  },

  // huntKey is the visual fingerprint of the zone registry of the
  // current snapshot (computed once per update, not per frame): the
  // label only fields stay out so a ticking respawn countdown or a
  // drifting adena rate does not drop the hunt cache.
  huntKey: "",

  // huntMesh caches the static hexagon hunt mesh of the cell mode
  // (the /api/hunt-mesh payload: the version plus the uniform
  // hexagon polygons). The mesh is the same for the whole deployment,
  // the fetch happens once per version and the per second snapshot
  // only carries the version marker and the live record of the held
  // cell. huntMeshFetching guards the in-flight request. The mesh
  // serves the hover hit tests and the active hex lookup - the map
  // DRAWS only the hex the fight runs in and the one under the
  // cursor, never the partition.
  huntMesh: null,
  huntMeshFetching: false,

  // tileLoads counts the tile arrivals of both pyramids (the map
  // imagery, the geodata view, the loads and the misses). The cache
  // key does NOT ride the raw counter: a zoom-out load burst lands
  // dozens of tiles over seconds and every one of them would drop
  // the key while the render loop keeps painting - the arrivals
  // commit in throttled batches instead (bg.tilesCommitted, see
  // tileArrived).
  tileLoads: 0,

  // colorsRev bumps on every theme refresh: the cache raster carries
  // the theme colors (the grid, its labels), so the key must drop it.
  colorsRev: 0,

  // viewDpr is the device pixel ratio the main canvas renders at
  // (set by resize): the cache blit snaps to whole device pixels
  // through it.
  viewDpr: 1,

  // The live combat animation layer: the server observed swings and
  // damage landings replayed as short canvas effects (see
  // spawnCombatAnim). lastCombatSeq dedupes the events across the
  // repeated SSE snapshots of the two second server feed window.
  combatAnims: [],
  lastCombatSeq: 0,

  // The running self cast read from the snapshot skillStates (see
  // ingestSkillStates): the skill id, the cast total and the cast
  // end on the local performance clock. Null when nothing casts.
  // skillIconCache lazily loads the skill icon art of the cast icon
  // (name -> {img, ready, missing}, the geoTiles pattern).
  selfCast: null,
  skillIconCache: null,

  // castIconSide is the horizontal side the cast icon hangs on (+1
  // right, -1 left): updateCastIconSide moves it away from the enemy
  // mass so the plate overlaps no name band nor fight float, and the
  // default keeps it on the right when no hostiles are visible.
  castIconSide: 1,

  // The per frame contact offsets of the unit markers (key ->
  // {x, y}, see computeContactOffsets): the melee combatants keep
  // their full circle size and slide apart so they touch face to face
  // instead of merging into one blob. Null until the first draw pass
  // computes it.
  contactOffsets: null,

  // The server world region grid: every region is 2048 units and every
  // object within the 3x3 region block around the player is loaded (see
  // World.broadcastPacket of the Mobius server).
  regionSize: 2048,

  // The world map tiles: one tile covers 32768 world units (the Mobius
  // World.TILE_SIZE) at 1024 source pixels, named BX_BY.jpg with
  // BX = floor(x / 32768) + 20 and BY = floor(y / 32768) + 18 (the
  // World.TILE_ZERO_COORD anchors). The pyramid levels 0..3 halve the
  // resolution per level (1024, 512, 256, 128 px per tile) for the
  // zoomed out views.
  mapTileSize: 32768,
  mapTileZeroX: 20,
  mapTileZeroY: 18,
  mapTilePixels: [1024, 512, 256, 128],
  // The geodata pyramid adds a full resolution level: one 2048px tile
  // per region renders exactly one geodata cell per pixel.
  geoTilePixels: [2048, 1024, 512, 256, 128],

  init() {
    this.canvas = document.getElementById("map-canvas");
    this.ctx = this.canvas.getContext("2d");
    this.tooltip = document.getElementById("map-tooltip");
    this.refreshColors();
    this.resize();
    window.addEventListener("resize", () => this.resize());
    window.addEventListener("themechange", () => this.refreshColors());
    this.canvas.addEventListener("wheel", (e) => this.onWheel(e));
    this.canvas.addEventListener("mousedown", (e) => this.onDragStart(e));
    window.addEventListener("mousemove", (e) => this.onDragMove(e));
    window.addEventListener("mouseup", () => {
      const wasMarker = this.drag && this.drag.marker;
      this.drag = null;
      if (wasMarker) {
        this.canvas.style.cursor = "";
        this.notifyMarkersChanged(true);
      }
    });
    this.canvas.addEventListener("mousemove", (e) => this.onHover(e));
    this.canvas.addEventListener("mouseleave", () => this.onMapLeave());
    this.canvas.addEventListener("dblclick", (e) => this.onDoubleClick(e));
    this.canvas.addEventListener("dragover", (e) => this.onMapDragOver(e));
    this.canvas.addEventListener("drop", (e) => this.onMapDrop(e));
    // Zoom lives on the wheel only (anchored at the cursor, onWheel
    // above): the old toolbar +/- buttons are gone with the layer
    // dropdown rework of the toolbar.
    const follow = document.getElementById("follow");
    follow.addEventListener("change", () => {
      if (!follow.checked) { this.syncPanAnchor(); }
      this.draw();
    });
    for (const id of ["show-labels", "show-dest", "show-zone", "show-targets", "show-hunt-zones", "show-aggro", "show-social", "show-kills", "show-map", "show-geo"]) {
      document.getElementById(id).addEventListener("change", () => {
        this.draw();
      });
    }
    // The idle marker of the fps chip: when nothing painted for a
    // while the chip must say so instead of freezing the last reading
    // (a stopped render loop is a fact worth seeing). The interval
    // only exists where timers exist - the Node harnesses run the
    // meter fine without it.
    if (typeof setInterval === "function") {
      setInterval(() => this.markFpsIdle(), 1200);
    }
    // The ctrl+c copy of the map cursor coordinates (onKeyCopy): the
    // window level listener reads the pointer state this hover
    // handlers keep, a shortcut pressed with the pointer elsewhere
    // falls through to the browser default untouched.
    window.addEventListener("keydown", (e) => this.onKeyCopy(e));
  },

  // Canvas colors: the UI chrome (grid, text) follows the active theme
  // variables, while the unit markers use a fixed palette - the icons
  // must read identically over the light map imagery and over the dark
  // or light plain fill, so a theme switch never repaints the world.
  refreshColors() {
    const style = getComputedStyle(document.documentElement);
    const read = (name) => style.getPropertyValue(name).trim();
    this.colors = {
      grid: read("--grid"),
      gridText: read("--grid-text"),
      text: read("--text"),
      textBright: read("--text-bright"),
      textDim: read("--text-dim"),
      border: read("--border"),
      zone: read("--text-dim")
    };
    // The map text fonts resolve once here: the label draw and the
    // zone names used to run getComputedStyle per draw (per damage
    // number even), which forced a style resolve every frame.
    this.sansStack = read("--sans") || "sans-serif";
    this.labelFont = "600 10.5px " + this.sansStack;
    this.labelWidths.clear();
    this.mapColors = {
      self: "#1a73e8",
      player: "#9334e6",
      item: "#f9ab00",
      friendly: "#5f6368",
      passive: "#188038",
      aggressive: "#e37400",
      combat: "#d93025",
      dead: "#80868b",
      // The manual command feedback of the user: the walk plan line and
      // the destination marker use a bright magenta so they read on
      // both the light map imagery and the dark fill, and stay
      // distinct from the blue self marker and the red combat path.
      userPath: "#ff44cc",
      userMark: "#ff44cc",
      // The outline and the direction tick of the markers: a middle
      // slate that reads over the light map imagery and over both
      // theme fills alike.
      tick: "#39424e"
    };
    this.colorsRev += 1;
    this.redraw();
  },

  resize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.viewDpr = dpr;
    this.canvas.width = Math.max(1, Math.floor(rect.width * dpr));
    this.canvas.height = Math.max(1, Math.floor(rect.height * dpr));
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.redraw();
  },

  // A new snapshot arrives: sync the clock with the server, drop runtime
  // entries of despawned objects and keep the animation running.
  update(snapshot) {
    if (snapshot.serverTimeMs) {
      // Every offset sample underestimates the true server minus browser
      // clock difference by the snapshot transport delay (the server
      // timestamp is taken when the snapshot is built, Date.now() runs
      // when the event is handled), so the maximum of the recent samples
      // is the least biased estimate.
      this.clockSamples.push(snapshot.serverTimeMs - Date.now());
      if (this.clockSamples.length > clockSampleWindow) {
        this.clockSamples.shift();
      }
      this.clockOffsetMs = Math.max(...this.clockSamples);
    }
    // The camera anchor rides every snapshot: a zero position (a
    // pre-world session that publishes no character yet) leaves the
    // previous anchor in place, the same "no data" rule the runtime
    // advance and the zone frame follow.
    const character = snapshot.character;
    if (character && character.x) {
      this.lastChar = { x: character.x, y: character.y };
    }
    const alive = new Set(["self"]);
    for (const obj of snapshot.objects || []) {
      alive.add(obj.objectId);
    }
    for (const id of this.runtime.keys()) {
      if (!alive.has(id)) { this.runtime.delete(id); }
    }
    for (const id of this.projectionCache.keys()) {
      if (!alive.has(id)) { this.projectionCache.delete(id); }
    }
    this.ingestCombatEvents(snapshot);
    this.ingestSkillStates(snapshot);
    this.lastSnap = snapshot;
    this.huntKey = this.huntZoneVisualKey(snapshot);
    this.syncHuntMesh(snapshot);
    this.rebuildSocialMasks(snapshot);
    // The stable draw order and the footer object counts derive from
    // the snapshot alone: computing them here keeps every frame of the
    // render loop free of the per frame sort and the per frame count
    // walk (the sort ran on every draw before - a slice plus an
    // n log n compare on every animation frame).
    this.sortedObjects = (snapshot.objects || []).slice()
      .sort((a, b) => ((a.dead ? 0 : 1) - (b.dead ? 0 : 1))
        || (a.y - b.y) || (a.objectId - b.objectId));
    this.objectsText = this.buildObjectsText(snapshot);
    this.kickAnimation();
    // The repaint of a data driven update goes through redraw: the
    // snapshots keep arriving with the map tab hidden too, and
    // painting a display:none canvas is pure cpu waste.
    this.redraw();
  },

  // resetBot drops the observed bot state ahead of the first snapshot
  // of a bot switch: the map must not keep painting the previous
  // bot's world (its zones, its objects, its walk line) under the new
  // bot's HUD while the event stream reconnects. The static world (the
  // imagery, the grid, the zone frame) keeps painting through the gap
  // (see paint) and the camera holds the last known character position
  // instead of collapsing to the world origin. The fleet wide kill
  // marks stay - they belong to the whole deployment, not to one bot.
  resetBot() {
    if (this.lastSnap) {
      // The interpolated drawn position is the exact point the camera
      // follows this frame: the freshest anchor the outgoing bot has.
      const pos = this.charPos();
      this.lastChar = { x: pos.x, y: pos.y };
    }
    this.lastSnap = null;
    this.huntKey = "";
    this.runtime.clear();
    this.socialMasks.clear();
    this.projectionCache.clear();
    this.sortedObjects = [];
    this.objectsText = "";
    this.combatAnims = [];
    this.lastCombatSeq = 0;
    this.selfCast = null;
    this.castIconSide = 1;
    this.userMark = null;
    this.redraw();
  },

  // rebuildSocialMasks parses the clan mask strings of the fresh
  // snapshot once: the masks ride the wire as decimal strings (the
  // ALL bit of the top exceeds the safe integer range of JavaScript),
  // the social layer needs them as numbers on every frame.
  rebuildSocialMasks(snapshot) {
    this.socialMasks.clear();
    for (const obj of snapshot.objects || []) {
      if (obj.kind !== "npc" || !obj.clanMask) { continue; }
      let mask;
      try {
        mask = BigInt(obj.clanMask);
      } catch (err) {
        continue;
      }
      this.socialMasks.set(obj.objectId, {
        low: Number(mask & 0xFFFFFFFFFFFn),
        all: (mask & 0x8000000000000000n) !== 0n
      });
    }
  },

  zoom(factor) {
    this.scale = Math.max(0.008, Math.min(1.5, this.scale * factor));
    this.draw();
  },

  onWheel(event) {
    event.preventDefault();
    this.syncView();
    const cursor = {
      x: event.clientX - this.view.left, y: event.clientY - this.view.top
    };
    // Anchor the zoom at the cursor: the world point under it must
    // stay under it (like the google maps wheel zoom). While follow is
    // on the camera is pinned to the character, so the anchor only
    // applies to the free camera.
    const before = this.screenToWorld(cursor.x, cursor.y);
    const factor = event.deltaY < 0 ? 1.15 : 1 / 1.15;
    this.zoom(factor);
    if (!this.followEnabled()) {
      const after = this.screenToWorld(cursor.x, cursor.y);
      this.panAnchor.x += before.x - after.x;
      this.panAnchor.y += before.y - after.y;
      this.draw();
    }
  },

  // screenToWorld inverts the world to screen transform for a
  // viewport relative point. Pure math on the synced view cache -
  // the callers are input handlers that run syncView on entry.
  screenToWorld(sx, sy) {
    return {
      x: this.camX + (sx - this.view.width / 2) / this.scale,
      y: this.camY + (sy - this.view.height / 2) / this.scale
    };
  },

  // syncView reads the viewport geometry and the camera state once
  // per frame batch. draw() calls it at the top; the input handlers
  // that need world coordinates before a draw (hover, wheel, clicks,
  // drags) call it on entry. Between two syncView calls the transform
  // math is pure - no layout reads, no style resolves.
  syncView() {
    const rect = this.canvas.getBoundingClientRect();
    this.view.width = rect.width;
    this.view.height = rect.height;
    this.view.left = rect.left;
    this.view.top = rect.top;
    this.followOn = !this.pathfindEnabled()
      && document.getElementById("follow").checked;
    const pos = this.followOn ? this.charPos() : this.panAnchor;
    this.camX = pos.x;
    this.camY = pos.y;
  },

  onDragStart(event) {
    this.syncView();
    if (this.pathfindEnabled()) {
      if (this.pathfind.placeMode) {
        this.setPathfindMarker(this.pathfind.placeMode,
          this.eventWorld(event));
        this.setPlaceMode(null);
        this.drag = { x: event.clientX, y: event.clientY, inert: true };
        this.notifyMarkersChanged(true);

        return;
      }
      const marker = this.pathfindMarkerAt(event.clientX, event.clientY);
      if (marker) {
        this.drag = { marker };
        this.canvas.style.cursor = "grabbing";

        return;
      }
    }
    this.drag = { x: event.clientX, y: event.clientY };
    // The map is grabbed like a picture: follow is switched off at grab
    // time so the camera stops tracking the character under the held
    // point, and the pan anchor takes over without a visual jump.
    if (this.followEnabled()) {
      document.getElementById("follow").checked = false;
      this.syncPanAnchor();
    }
  },

  // Dragging pans the view like moving a sheet of paper: the grabbed
  // world point stays exactly under the cursor. The screen offset of a
  // world point is (wx - panAnchor) * scale, so the camera must move
  // opposite to the cursor and in world units of 1/scale per pixel.
  // While follow is off the camera never moves on its own: the map
  // shows the chosen area even when the bot walks away. A marker drag
  // moves the marker instead of the camera.
  onDragMove(event) {
    if (!this.drag) { return; }
    if (this.drag.marker) {
      this.setPathfindMarker(this.drag.marker, this.eventWorld(event));
      this.notifyMarkersChanged(false);

      return;
    }
    if (this.drag.inert) { return; }
    this.panAnchor.x -= (event.clientX - this.drag.x) / this.scale;
    this.panAnchor.y -= (event.clientY - this.drag.y) / this.scale;
    this.drag = { x: event.clientX, y: event.clientY };
    this.draw();
  },

  // syncPanAnchor anchors the free camera at the current character
  // position, so disabling follow never jumps the view.
  syncPanAnchor() {
    const pos = this.charPos();
    this.panAnchor = { x: pos.x, y: pos.y };
  },

  // ---- movement interpolation ----
  //
  // The Mobius server stops a moving creature once one 100 ms game tick
  // step would cover the remaining distance minus the collision radius
  // (Creature.updatePosition), snaps the creature to the exact
  // destination and broadcasts the zero distance MoveToLocation. A naive
  // client that animates the full packet distance at the transmitted
  // speed is therefore still short by roughly collision + one tick step
  // when the arrival packet lands, which looked like a fast teleport for
  // the last ~1/8 of short paths.
  //
  // The rendering compensates in two ways:
  // - the projection covers the segment minus the packet collision
  //   radius (NpcInfo/CharInfo carry it) in the time the server needs,
  //   which is the exact server stop rule without any learned tuning;
  // - the drawn position follows the projection plus a decaying offset
  //   that is only set when the projection itself jumps (a new segment,
  //   an arrival, a teleport), so continuous movement has no permanent
  //   smoothing lag and residual mismatches glide instead of snapping.

  // kickAnimation starts the render loop when something can still move.
  kickAnimation() {
    if (this.animating || !this.lastSnap) { return; }
    this.animating = true;
    this.lastFrame = performance.now();
    requestAnimationFrame((ts) => this.frame(ts));
  },

  frame(ts) {
    // The loop stops instead of ticking no-op frames while the map
    // tab is hidden: painting a display:none canvas at the display
    // refresh rate is pure cpu waste. The next snapshot (or the next
    // map interaction) re-kicks the loop within one poll period.
    if (!this.mapVisible()) {
      this.animating = false;

      return;
    }
    const dt = Math.min(0.1, (ts - this.lastFrame) / 1000);
    this.lastFrame = ts;
    this.updateRuntime(dt);
    this.draw();
    if (this.needsMoreFrames()) {
      requestAnimationFrame((next) => this.frame(next));
    } else {
      this.animating = false;
    }
  },

  mapVisible() {
    const tab = document.getElementById("tab-map");

    return tab.classList.contains("active");
  },

  needsMoreFrames() {
    if (!this.lastSnap) { return false; }
    if (this.lastSnap.character && this.lastSnap.character.moving) {
      return true;
    }
    for (const obj of this.lastSnap.objects || []) {
      if (obj.moving && obj.speed > 0) { return true; }
    }

    return this.combatAnims.length > 0 || this.selfCast !== null ||
      this.smoothingPending() ||
      this.userMarkAge() < 2500 || this.hasWalkPlan();
  },

  // hasWalkPlan reports whether the bot runs a manual walk right now:
  // the destination marker of the plan pulses on its own animation
  // even while the character itself already stands (the plan switches
  // to the first leg).
  hasWalkPlan() {
    return Boolean(this.lastSnap && this.lastSnap.walkPath &&
      this.lastSnap.walkPath.length);
  },

  smoothingPending() {
    for (const rt of this.runtime.values()) {
      if (!rt.settled) { return true; }
    }

    return false;
  },

  // updateRuntime advances the interpolated position of every object.
  updateRuntime(dt) {
    if (!this.lastSnap) { return; }
    const nowMs = Date.now() + this.clockOffsetMs;
    const turning = 1 - Math.exp(-dt * 12);
    const c = this.lastSnap.character;
    if (c && c.x) {
      this.advanceRuntime(c, "self", nowMs, dt, turning);
    }
    for (const obj of this.lastSnap.objects || []) {
      this.advanceRuntime(obj, obj.objectId, nowMs, dt, turning);
    }
  },

  // advanceRuntime drives one snapshot view (the character or a world
  // object). The target is the exact reproduction of the server side
  // movement recurrence (see projectTickwise), so in steady motion the
  // drawn position sits ON the target with zero lag; when a packet
  // update moves the target (delivery latency, a retarget, an arrival
  // snap), the drawn position follows with a speed cap of chaseFactor
  // times the unit speed - the correction becomes a slightly faster
  // glide instead of a jump, which is exactly the jerky artifact the
  // old learned gap model produced.
  advanceRuntime(view, key, nowMs, dt, turning) {
    let rt = this.runtime.get(key);
    if (!rt) {
      // A view seen for the first time mid move starts at the projected
      // position (anchored at the packet time), so a mob that walked
      // into the known list does not pop up at its segment start.
      const p = this.projectTickwise(view, key, nowMs);
      rt = {
        drawX: p.x, drawY: p.y, drawHeading: view.heading || 0,
        settled: true, lastV: 0
      };
      this.runtime.set(key, rt);
    }
    if (view.speed > 0) {
      rt.lastV = view.speed;
    }
    const target = this.projectTickwise(view, key, nowMs);
    const dx = target.x - rt.drawX;
    const dy = target.y - rt.drawY;
    const dist = Math.hypot(dx, dy);
    if (!Number.isFinite(dist) || dist > teleportUnits) {
      // A real teleport - or a poisoned drawn position (a NaN that
      // would otherwise keep reproducing itself through the chase
      // arithmetic): render the projection instantly.
      rt.drawX = target.x;
      rt.drawY = target.y;
      rt.settled = true;
    } else {
      const chase = Math.max(60, rt.lastV * chaseFactor) * dt;
      if (dist <= chase) {
        rt.drawX = target.x;
        rt.drawY = target.y;
        rt.settled = true;
      } else {
        rt.drawX += dx / dist * chase;
        rt.drawY += dy / dist * chase;
        rt.settled = false;
      }
    }
    rt.drawHeading = turnHeading(rt.drawHeading, view.heading, turning);
  },

  // projectTickwise reproduces the server movement recurrence exactly:
  // Creature.updatePosition runs on 100 ms game ticks and advances the
  // creature by xAccurate += (destination - xAccurate) * distFraction
  // where distFraction = speed * ticks / 10 / (remaining - collision),
  // counts it as arrived once distFraction exceeds 1 and snaps it to the
  // exact destination. Replaying the same recurrence from the packet
  // position and the packet speed gives the server position without any
  // learned tuning: the packet speeds are the real ones (the server
  // re-reads its move speed every tick, so buffs or a walk/run switch
  // take effect with the next broadcast, and the broadcast values are
  // divided by the move multiplier which the tracker multiplies back).
  // The server position is a step function (one jump per tick); the two
  // positions around the current tick are interpolated linearly, which
  // renders the same average motion as one straight constant speed move
  // - exactly what the official client animation does with the ticks.
  //
  // The replay is memoized per object (projectionCache): the cursor
  // of the last whole tick is kept, so a frame only advances the ticks
  // that passed since the previous frame. Without the memo a long walk
  // replayed its whole tick history on every animation frame - a two
  // minute town trip ran a 1200 iteration loop sixty times per second,
  // and the loop never shrank back after the arrival snap. The cache
  // key is the move signature; a new packet (a re-issued move, a
  // speed change) resets the cursor and replays from the packet
  // anchor - the arithmetic sequence is identical either way, so the
  // result is bit for bit what the from scratch replay produced.
  projectTickwise(view, key, nowMs) {
    if (!view.moving || !(view.speed > 0)) {
      return { x: view.x, y: view.y };
    }
    const dist = Math.hypot(view.destX - view.x, view.destY - view.y);
    if (dist < 1) {
      return { x: view.destX, y: view.destY };
    }
    const collision = view.collisionRadius > 0
      ? view.collisionRadius : defaultCollisionRadius;
    const tickFloat = Math.max(0, nowMs - (view.moveAtMs || 0)) / 100;
    const whole = Math.floor(tickFloat);
    const frac = tickFloat - whole;
    const moveAtMs = view.moveAtMs || 0;
    let memo = this.projectionCache.get(key);
    if (!memo || memo.x0 !== view.x || memo.y0 !== view.y
      || memo.dx !== view.destX || memo.dy !== view.destY
      || memo.speed !== view.speed || memo.coll !== collision
      || memo.atMs !== moveAtMs) {
      memo = {
        x0: view.x, y0: view.y, dx: view.destX, dy: view.destY,
        speed: view.speed, coll: collision, atMs: moveAtMs,
        x: view.x, y: view.y, tick: 0
      };
      this.projectionCache.set(key, memo);
    }
    const step = view.speed / 10;
    while (memo.tick < whole) {
      const remainingX = view.destX - memo.x;
      const remainingY = view.destY - memo.y;
      const remaining = Math.hypot(remainingX, remainingY);
      const delta = Math.max(0.00001, remaining - collision);
      const advance = step / delta;
      if (advance >= 1) {
        // The arrival snap: every later tick computes the same
        // destination, so the cursor jumps straight to the present.
        memo.x = view.destX;
        memo.y = view.destY;
        memo.tick = whole;

        break;
      }
      memo.x += remainingX * advance;
      memo.y += remainingY * advance;
      memo.tick += 1;
    }
    if (memo.tick > whole) {
      // The clock estimate moved backwards (a fresher snapshot with
      // an earlier sample): hold the last position until time passes
      // the cursor again instead of rewinding the walk.
      return { x: memo.x, y: memo.y };
    }
    // The window between the whole tick and the next one: interpolate.
    const remainingX = view.destX - memo.x;
    const remainingY = view.destY - memo.y;
    const remaining = Math.hypot(remainingX, remainingY);
    const delta = Math.max(0.00001, remaining - collision);
    const advance = step / delta;
    const nextX = advance >= 1 ? view.destX : memo.x + remainingX * advance;
    const nextY = advance >= 1 ? view.destY : memo.y + remainingY * advance;

    return {
      x: memo.x + (nextX - memo.x) * frac,
      y: memo.y + (nextY - memo.y) * frac
    };
  },

  // ---- world map background ----

  // drawMapBackground paints the game world map tiles under everything
  // else. The level of detail follows the zoom: the chosen pyramid
  // level is the smallest one whose native resolution stays within a
  // factor of two of the drawn tile size, so zoomed in views use the
  // full tiles and zoomed out views use the small ones without
  // over-magnifying.
  drawMapBackground(ctx, rect) {
    // The pathfind test can replace the photo imagery with the rendered
    // geodata view (heightmap / walls / layers).
    if (this.pathfindEnabled() && this.geoEnabled()) {
      this.drawGeodataBackground(ctx, rect);

      return;
    }
    if (!document.getElementById("show-map").checked) { return; }
    const drawnPx = this.mapTileSize * this.scale;
    let level = this.mapTilePixels.length - 1;
    for (let i = 0; i < this.mapTilePixels.length; i++) {
      // Google maps rule: allow at most a 2x upscale of the tile
      // pixels, everything smaller goes one level down the pyramid.
      if (this.mapTilePixels[i] <= drawnPx * 2) { level = i; break; }
    }
    const half = rect.width / 2;
    const worldLeft = this.centerX() - half / this.scale;
    const worldRight = this.centerX() + half / this.scale;
    const worldTop = this.centerY() - rect.height / 2 / this.scale;
    const worldBottom = this.centerY() + rect.height / 2 / this.scale;
    const tile = this.mapTileSize;
    const bxMin = Math.floor(worldLeft / tile) + this.mapTileZeroX;
    const bxMax = Math.floor(worldRight / tile) + this.mapTileZeroX;
    const byMin = Math.floor(worldTop / tile) + this.mapTileZeroY;
    const byMax = Math.floor(worldBottom / tile) + this.mapTileZeroY;
    ctx.imageSmoothingEnabled = true;
    // "low" (bilinear) instead of "high": the high quality filter of
    // the zoomed out frames downscaled a dozen 512px tiles per paint
    // and is visually indistinguishable on terrain imagery - the far
    // zoom tile pass was one of the biggest frame costs.
    ctx.imageSmoothingQuality = "low";
    for (let bx = bxMin; bx <= bxMax; bx++) {
      for (let by = byMin; by <= byMax; by++) {
        // The full resolution tiles ship only for the detail window:
        // outside it the pyramid walks up to the closest existing
        // level and stretches it over the tile rect, so every panned
        // to area keeps its map at any zoom.
        const entry = this.mapTileAncestor(level, bx, by);
        if (!entry || !entry.ready) { continue; }
        const screen = this.worldToScreen(
          (bx - this.mapTileZeroX) * tile, (by - this.mapTileZeroY) * tile);
        ctx.drawImage(entry.img, screen.x, screen.y,
          tile * this.scale + 1, tile * this.scale + 1);
      }
    }
  },

  // geoEnabled reports whether the geodata visualization layer is on.
  geoEnabled() {
    const box = document.getElementById("show-geo");

    return !!box && box.checked;
  },

  // drawGeodataBackground paints the rendered geodata tiles under
  // everything else. The pyramid adds a 2048px level that renders one
  // geodata cell per pixel, so zoomed in views show cell exact walls
  // and heights; the region tiles are produced by the bot process on
  // demand and cached by the browser.
  drawGeodataBackground(ctx, rect) {
    const drawnPx = this.mapTileSize * this.scale;
    let level = this.geoTilePixels.length - 1;
    for (let i = 0; i < this.geoTilePixels.length; i++) {
      if (this.geoTilePixels[i] <= drawnPx * 2) { level = i; break; }
    }
    const half = rect.width / 2;
    const worldLeft = this.centerX() - half / this.scale;
    const worldRight = this.centerX() + half / this.scale;
    const worldTop = this.centerY() - rect.height / 2 / this.scale;
    const worldBottom = this.centerY() + rect.height / 2 / this.scale;
    const tile = this.mapTileSize;
    const bxMin = Math.floor(worldLeft / tile) + this.mapTileZeroX;
    const bxMax = Math.floor(worldRight / tile) + this.mapTileZeroX;
    const byMin = Math.floor(worldTop / tile) + this.mapTileZeroY;
    const byMax = Math.floor(worldBottom / tile) + this.mapTileZeroY;
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = "low";
    for (let bx = bxMin; bx <= bxMax; bx++) {
      for (let by = byMin; by <= byMax; by++) {
        const entry = this.geoTileAncestor(level, bx, by);
        if (!entry || !entry.ready) { continue; }
        const screen = this.worldToScreen(
          (bx - this.mapTileZeroX) * tile, (by - this.mapTileZeroY) * tile);
        ctx.drawImage(entry.img, screen.x, screen.y,
          tile * this.scale + 1, tile * this.scale + 1);
      }
    }
  },

  // geoTileAncestor returns the finest READY pyramid entry of a
  // geodata tile (the same progressive rule as the map pyramid: a
  // coarser loaded region paints while the fine one renders).
  geoTileAncestor(level, bx, by) {
    let entry = this.geoTile(level, bx, by);
    if (entry && entry.ready) { return entry; }
    let lvl = level;
    while (lvl + 1 < this.geoTilePixels.length) {
      lvl += 1;
      const parent = this.geoTile(lvl, bx, by);
      if (parent && parent.ready) { return parent; }
    }

    return entry;
  },

  // geoTile returns the geodata tile entry and starts the background
  // load once.
  geoTile(level, bx, by) {
    const mode = (this.pathfind && this.pathfind.geoMode) || "height";
    const path = "api/geodata/tile/" + level + "/" + bx + "_" + by
      + ".png?mode=" + mode;
    let entry = this.geoTiles.get(path);
    if (entry) {
      return entry;
    }
    entry = { img: null, ready: false, missing: false };
    this.geoTiles.set(path, entry);
    if (typeof Image === "undefined") {
      return entry;
    }
    const img = new Image();
    // decode() hands the jpeg decode to the image pipeline: without
    // the hint the first drawImage of a landed tile decodes it inline
    // - a stack of synchronous decodes inside a cache re-render of
    // the zoomed out world (the low fps of a loading map).
    img.onload = () => {
      const landed = () => {
        entry.ready = true;
        entry.img = img;
        this.tileArrived();
      };
      if (typeof img.decode === "function") {
        // An undecodable bitmap stays un-drawn: the ancestor walk
        // keeps the coarser fallback of that region.
        img.decode().then(landed, () => this.tileArrived());
      } else {
        landed();
      }
    };
    img.onerror = () => {
      entry.missing = true;
      this.tileArrived();
    };
    img.src = path;

    return entry;
  },

  // mapTileAncestor returns the finest READY pyramid entry of a
  // tile. The requested level comes first; while it streams (or
  // where it is missing from the shipped world) the walk climbs to
  // the coarser levels and returns the first loaded one, so a
  // panning camera (the follow mode of a walking bot) never shows a
  // blank strip where a coarse ancestor could paint - the microsecond
  // white flash of the old not-ready skip. The entry of the finest
  // level still wins the next frames once its load lands.
  mapTileAncestor(level, bx, by) {
    let entry = this.mapTile(level, bx, by);
    if (entry && entry.ready) { return entry; }
    let lvl = level;
    while (lvl + 1 < this.mapTilePixels.length) {
      lvl += 1;
      const parent = this.mapTile(lvl, bx, by);
      if (parent && parent.ready) { return parent; }
    }

    return entry;
  },

  // mapTile returns the pyramid tile entry and starts the background
  // load once. Tiles outside the shipped world are remembered as
  // missing to avoid retrying and to let the ancestor walk skip them.
  mapTile(level, bx, by) {
    const path = "maps/" + level + "/" + bx + "_" + by + ".jpg";
    let entry = this.mapTiles.get(path);
    if (entry) {
      return entry;
    }
    entry = { img: null, ready: false, missing: false };
    this.mapTiles.set(path, entry);
    if (typeof Image === "undefined") {
      return entry;
    }
    const img = new Image();
    // decode() hands the jpeg decode to the image pipeline: without
    // the hint the first drawImage of a landed tile decodes it inline
    // - a stack of synchronous decodes inside a cache re-render of
    // the zoomed out world (the low fps of a loading map).
    img.onload = () => {
      const landed = () => {
        entry.ready = true;
        entry.img = img;
        this.tileArrived();
      };
      if (typeof img.decode === "function") {
        // An undecodable bitmap stays un-drawn: the ancestor walk
        // keeps the coarser fallback of that region.
        img.decode().then(landed, () => this.tileArrived());
      } else {
        landed();
      }
    };
    img.onerror = () => {
      entry.missing = true;
      this.tileArrived();
    };
    img.src = path;

    return entry;
  },

  // ---- pathfind test mode ----

  // enablePathfind switches the map into the bot less pathfind test
  // mode: markers, place mode and the result overlay, with callbacks
  // into PathfindUI for the marker and place mode changes.
  enablePathfind(state) {
    this.pathfind = {
      enabled: true,
      start: state.start,
      end: state.end,
      placeMode: null,
      result: null,
      onMarkersChanged: state.onMarkersChanged || null,
      onPlaceModeChanged: state.onPlaceModeChanged || null
    };
    this.draw();
  },

  pathfindEnabled() {
    return !!(this.pathfind && this.pathfind.enabled);
  },

  // setPlaceMode arms the click to place mode of a marker (null
  // disarms) and notifies the UI for the button state.
  setPlaceMode(key) {
    this.pathfind.placeMode = key;
    if (this.pathfind.onPlaceModeChanged) {
      this.pathfind.onPlaceModeChanged(key);
    }
    this.draw();
  },

  setPathfindMarker(key, world) {
    this.pathfind[key] = { x: world.x, y: world.y };
    this.draw();
  },

  notifyMarkersChanged(immediate) {
    if (this.pathfind && this.pathfind.onMarkersChanged) {
      this.pathfind.onMarkersChanged(immediate);
    }
  },

  // eventWorld converts a mouse event to world coordinates.
  eventWorld(event) {
    return this.screenToWorld(
      event.clientX - this.view.left, event.clientY - this.view.top);
  },

  // pathfindMarkerAt hit tests the draggable markers in screen space.
  pathfindMarkerAt(clientX, clientY) {
    if (!this.pathfindEnabled()) { return null; }
    const mx = clientX - this.view.left;
    const my = clientY - this.view.top;
    for (const key of ["end", "start"]) {
      const marker = this.pathfind[key];
      const p = this.worldToScreen(marker.x, marker.y);
      if (Math.hypot(p.x - mx, p.y - my) <= 12) { return key; }
    }

    return null;
  },

  // drawPathfindOverlay paints the search result and the markers: the
  // optional raw A* cell path as a faint line, the smoothed path as a
  // red dashed line with a small circle at every turning point (the
  // route changes the walker follows) and the A/B markers on top.
  drawPathfindOverlay(ctx) {
    const pf = this.pathfind;
    const result = pf.result;
    if (result && result.found && result.raw
      && document.getElementById("show-raw").checked) {
      this.drawWorldPolyline(ctx, result.raw, {
        color: this.colors.textDim, width: 1, alpha: 0.4, dash: []
      });
    }
    if (result && result.found && result.waypoints) {
      this.drawWorldPolyline(ctx, result.waypoints, {
        color: this.mapColors.combat, width: 2, alpha: 0.9, dash: [8, 6]
      });
      for (const waypoint of result.waypoints) {
        const p = this.worldToScreen(waypoint.x, waypoint.y);
        ctx.beginPath();
        ctx.arc(p.x, p.y, 3.5, 0, Math.PI * 2);
        ctx.fillStyle = this.mapColors.combat;
        ctx.fill();
        ctx.lineWidth = 1.2;
        ctx.strokeStyle = "#ffffff";
        ctx.stroke();
      }
    }
    this.drawPathfindMarker(ctx, "start", this.mapColors.self, "A");
    this.drawPathfindMarker(ctx, "end", this.mapColors.combat, "B");
  },

  // drawWorldPolyline strokes a world space polyline with the style.
  drawWorldPolyline(ctx, points, style) {
    if (!points || points.length < 2) { return; }
    ctx.save();
    ctx.strokeStyle = style.color;
    ctx.globalAlpha = style.alpha;
    ctx.lineWidth = style.width;
    ctx.setLineDash(style.dash);
    ctx.beginPath();
    for (let i = 0; i < points.length; i++) {
      const p = this.worldToScreen(points[i].x, points[i].y);
      if (i === 0) { ctx.moveTo(p.x, p.y); } else { ctx.lineTo(p.x, p.y); }
    }
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
  },

  // drawPathfindMarker renders one draggable marker: a soft halo, the
  // circle body and the label letter with a dark halo.
  drawPathfindMarker(ctx, key, color, label) {
    const marker = this.pathfind[key];
    const p = this.worldToScreen(marker.x, marker.y);
    const armed = this.pathfind.placeMode === key;
    ctx.save();
    ctx.globalAlpha = armed ? 0.4 : 0.22;
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(p.x, p.y, 13, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = 1;
    ctx.beginPath();
    ctx.arc(p.x, p.y, 7.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.lineWidth = 1.5;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.85)";
    ctx.stroke();
    ctx.font = "700 10px sans-serif";
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
    ctx.strokeText(label, p.x, p.y + 0.5);
    ctx.fillStyle = "#ffffff";
    ctx.fillText(label, p.x, p.y + 0.5);
    ctx.restore();
  },

  // ---- drawing ----

  // draw paints one frame: syncs the view cache once, paints every
  // layer and feeds the fps meter (the paint wall time is the number
  // the chip and the console log carry). Every repaint - the rAF
  // animation loop, a drag mousemove, a zoom wheel tick, a landed
  // tile - goes through here, so the meter sees the real frame
  // budget the user feels.
  draw() {
    const ctx = this.ctx;
    if (!ctx || !this.colors) { return; }
    const paintStart = performance.now();
    this.syncView();
    this.paint(ctx);
    this.notePaint(paintStart);
  },

  paint(ctx) {
    const rect = this.view;
    const pathfind = this.pathfindEnabled();
    if (!this.lastSnap && !pathfind) {
      // The bot switch gap: the observed bot state is dropped but the
      // canvas must not go blank - a cleared frame read as a full white
      // flash on every sidebar click, loudest when both bots share the
      // same grid and nothing else would change at all. The bot
      // independent layers paint on: the imagery (the cached blit or
      // the direct tiles), the grid, the zone frame of the held
      // position and the fleet kill ring. Without a camera anchor yet
      // (a fresh boot before the very first snapshot) the blank frame
      // stays the honest boot state.
      if (!this.lastChar) {
        ctx.clearRect(0, 0, rect.width, rect.height);

        return;
      }
      if (!this.blitBackground(ctx, rect)) {
        ctx.clearRect(0, 0, rect.width, rect.height);
        this.drawMapBackground(ctx, rect);
        this.drawGrid(ctx, rect);
        this.drawZone(ctx, rect);
      }
      this.drawKillMarks(ctx, rect);

      return;
    }

    this.unitScale = this.computeUnitScale();
    this.labelCandidates = [];
    // The static world (the map or geodata tiles, the grid, the loaded
    // zone frame) comes from the offscreen cache as one blit; the
    // fallback path paints it directly (the sandboxed Node harnesses
    // run without a real offscreen canvas). The hunt zones and the
    // kill marks stay per frame on purpose: the zone labels carry the
    // live economy fields (the respawn countdown, the adena rate) and
    // the kill skulls fade with age - freezing either in a cache
    // would need a re-render per snapshot and eat the win.
    if (!this.blitBackground(ctx, rect)) {
      ctx.clearRect(0, 0, rect.width, rect.height);
      this.drawMapBackground(ctx, rect);
      this.drawGrid(ctx, rect);
      this.drawZone(ctx, rect);
    }
    if (pathfind) {
      this.drawPathfindOverlay(ctx);
      this.updateMapInfo();

      return;
    }
    this.drawHuntingZone(ctx, rect);
    this.drawKillMarks(ctx, rect);
    this.computeContactOffsets();
    this.drawTargetLinks(ctx);
    this.drawAggroRanges(ctx, rect);
    this.drawSocialLinks(ctx, rect);
    this.drawObjects(ctx, rect);
    this.drawSelf(ctx);
    this.drawSelfCast(ctx);
    this.drawCombatEffects(ctx);
    this.drawWalkPlan(ctx);
    this.drawUserIntent(ctx);
    if (this.lastSnap
      && document.getElementById("show-labels").checked) {
      this.drawLabels(ctx);
    }
    this.updateMapInfo();
  },

  // ---- the static background cache ----

  // redraw is the data driven entry of the pipeline: the snapshots,
  // the kill ring polls, the tile loads and the theme flips all fire
  // with the map tab hidden too, and painting a display:none canvas
  // burns the cpu for pixels nobody sees. The map interactions (a
  // drag, a zoom, a layer toggle) only happen on the visible map, so
  // they keep the direct draw. A skipped paint is never lost: the
  // next snapshot (300 ms worst case) repaints the world.
  redraw() {
    if (this.mapVisible()) { this.draw(); }
  },

  // layerChecked reads one layer checkbox of the toolbar (the cache
  // key runs outside the draw functions that read the boxes again).
  layerChecked(id) {
    const box = document.getElementById(id);

    return !!(box && box.checked);
  },

  // tileArrived is the single arrival path of both tile pyramids
  // (the imagery, the geodata view, the loads and the misses): it
  // counts the arrival and commits it to the background cache key
  // on a throttled cadence. A zoom-out burst lands dozens of tiles
  // over seconds - committing each arrival straight into the key
  // would re-raster the whole static world on EVERY animated frame
  // of the load window (the render loop keeps painting while the
  // tiles stream in, and each frame sees the dropped key: the low
  // fps of a loading map that recovers once the burst goes quiet).
  // The first arrival of a window commits immediately (the leading
  // edge - the first coarse imagery appears at once), the rest of
  // the burst coalesces into one trailing commit at the window end,
  // so a streaming load costs at most a couple of cache rasters per
  // second and the final state always lands (the trailing timer
  // runs when the burst goes quiet, even with the render loop idle
  // and the tab hidden - the commit does not need a paint).
  tileArrived() {
    this.tileLoads += 1;
    const bg = this.bg;
    const now = performance.now();
    if (bg.commitAt === 0 || now - bg.commitAt >= tilesCommitMs) {
      bg.commitAt = now;
      bg.tilesCommitted = this.tileLoads;
      this.redraw();

      return;
    }
    if (bg.commitTimer) { return; }
    const wait = Math.max(1, tilesCommitMs - (now - bg.commitAt));
    bg.commitTimer = setTimeout(() => {
      bg.commitTimer = 0;
      bg.commitAt = performance.now();
      if (bg.tilesCommitted !== this.tileLoads) {
        bg.tilesCommitted = this.tileLoads;
        this.redraw();
      }
    }, wait);
  },

  // bgKey is the identity of the background raster: everything that
  // changes what drawMapBackground, drawGrid and drawZone would paint
  // rides it - the zoom (the tile pyramid level and the grid step
  // derive from the scale), the committed tile batches, the theme, the
  // loaded zone region of the character, the geodata mode, the layer
  // toggles and the canvas geometry. A camera move alone does NOT
  // change the key: the cache is world anchored, so the camera pans
  // inside it until the slack box runs out. The region reads the held
  // camera anchor (lastChar) and not the live snapshot, so the bot
  // switch gap keeps the same key - the first frame after the switch
  // blits the raster the previous frames built instead of re-rastering
  // the world around a "region x" that no frame ever painted.
  bgKey(rect, dpr) {
    const c = this.lastChar;
    const region = c && c.x
      ? Math.floor(c.x / this.regionSize) + "_"
        + Math.floor(c.y / this.regionSize)
      : "x";
    const geo = this.pathfindEnabled() && this.geoEnabled()
      ? (this.pathfind.geoMode || "height") : "off";

    return [this.scale, this.bg.tilesCommitted, this.colorsRev, region, geo,
      this.layerChecked("show-map"), this.layerChecked("show-zone"),
      Math.round(rect.width), Math.round(rect.height), dpr].join("|");
  },

  // createBgCanvas makes the offscreen raster of the cache. The
  // sandboxed Node harnesses run map.js against stub documents: a
  // document without createElement, or a stub canvas without a 2d
  // context, returns null and the map keeps the direct per frame
  // static path (the behavior before the cache).
  createBgCanvas() {
    try {
      if (typeof document === "undefined"
        || typeof document.createElement !== "function") {
        return null;
      }
      const canvas = document.createElement("canvas");
      if (!canvas || typeof canvas.getContext !== "function") {
        return null;
      }

      return canvas;
    } catch (err) {
      return null;
    }
  },

  // ensureBackground validates the cache for this frame and
  // re-rasterizes it when the key dropped or the camera left the
  // slack box. False means the caller must paint the static layers
  // itself (no offscreen canvas in this environment).
  ensureBackground(rect) {
    const bg = this.bg;
    if (!bg.canvas) {
      bg.canvas = this.createBgCanvas();
      if (!bg.canvas) { return false; }
      const bctx = bg.canvas.getContext("2d");
      // A context without drawImage (or a main context without it)
      // cannot blit: keep the direct path instead of a half cache.
      if (!bctx || typeof bctx.drawImage !== "function"
        || !this.ctx || typeof this.ctx.drawImage !== "function") {
        bg.canvas = null;

        return false;
      }
      bg.ctx = bctx;
    }
    const dpr = this.viewDpr || 1;
    const key = this.bgKey(rect, dpr);
    const drifted = Math.abs(this.camX - bg.cx) > bg.marginX
      || Math.abs(this.camY - bg.cy) > bg.marginY;
    if (bg.key !== key || drifted) {
      this.renderBackground(rect, dpr, key);
    }

    return true;
  },

  // renderBackground rasterizes the static world into the cache: the
  // canvas covers the viewport plus one viewport of slack per side and
  // is anchored at the current camera position in world coordinates.
  // The static draw functions read the synced camera state, so the
  // render swaps in the cache geometry for its duration and restores
  // the real view in a finally - no input handler and no other draw
  // can interleave (single threaded), and the dynamic layers below
  // paint against the untouched real view.
  renderBackground(rect, dpr, key) {
    const bg = this.bg;
    const cssW = Math.max(1, Math.round(rect.width * (1 + 2 * bgMarginOfView)));
    const cssH = Math.max(1, Math.round(rect.height * (1 + 2 * bgMarginOfView)));
    // The device pixel budget: a 4K class viewport at dpr 2 would hold
    // a hundred megabytes of cache, so the cache resolution steps
    // down to fit (the tile sources upscale anyway - one source pixel
    // covers several device pixels at every zoom, so the imagery
    // loses nothing).
    let cd = Math.min(dpr, 2);
    const fit = Math.sqrt(bgDevicePixels / (cssW * cssH));
    if (cd > fit) { cd = Math.max(1, fit); }
    const devW = Math.max(1, Math.round(cssW * cd));
    const devH = Math.max(1, Math.round(cssH * cd));
    if (bg.canvas.width !== devW) { bg.canvas.width = devW; }
    if (bg.canvas.height !== devH) { bg.canvas.height = devH; }
    const ctx = bg.ctx;
    ctx.setTransform(cd, 0, 0, cd, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);
    const keepView = this.view;
    const keepX = this.camX;
    const keepY = this.camY;
    const cacheView = {
      width: cssW, height: cssH,
      left: keepView.left, top: keepView.top
    };
    this.view = cacheView;
    this.camX = bg.cx = keepX;
    this.camY = bg.cy = keepY;
    try {
      this.drawMapBackground(ctx, cacheView);
      this.drawGrid(ctx, cacheView);
      this.drawZone(ctx, cacheView);
    } finally {
      this.view = keepView;
      this.camX = keepX;
      this.camY = keepY;
    }
    bg.cssW = cssW;
    bg.cssH = cssH;
    bg.dpr = cd;
    bg.worldLeft = bg.cx - cssW / 2 / this.scale;
    bg.worldTop = bg.cy - cssH / 2 / this.scale;
    // The slack box: the camera may drift this far (in world units)
    // before the cache re-renders - one viewport of overdraw minus a
    // safety band, so the edge never shows a stale strip.
    bg.marginX = Math.max(1, rect.width * bgMarginOfView - 64)
      / this.scale;
    bg.marginY = Math.max(1, rect.height * bgMarginOfView - 64)
      / this.scale;
    bg.key = key;
  },

  // blitBackground composites the cache onto the frame with one
  // drawImage - a GPU side copy of a world anchored texture instead
  // of the per frame re-raster of the tiles, the grid and the zone
  // frame. The offset snaps to whole device pixels: a fractional
  // offset makes the GPU resample the whole cache every frame (a
  // permanent sub pixel smear of the grid), a snapped one pans in 1px
  // steps at the display rate and stays crisp at rest.
  blitBackground(ctx, rect) {
    if (!this.ensureBackground(rect)) { return false; }
    const bg = this.bg;
    const dpr = this.viewDpr || 1;
    const left = this.camX - rect.width / 2 / this.scale;
    const top = this.camY - rect.height / 2 / this.scale;
    const dx = Math.round((bg.worldLeft - left) * this.scale * dpr) / dpr;
    const dy = Math.round((bg.worldTop - top) * this.scale * dpr) / dpr;
    ctx.drawImage(bg.canvas, 0, 0, bg.canvas.width, bg.canvas.height,
      dx, dy, bg.cssW, bg.cssH);

    return true;
  },

  // ---- the fps meter ----

  // notePaint closes one measurement: the paint wall time joins the
  // current window and, every fpsWindowMs, the window becomes the
  // chip reading (and every fpsLogMs a console line - the log line
  // is what a lag report pastes).
  notePaint(started) {
    const now = performance.now();
    const f = this.fps;
    f.paints += 1;
    const took = now - started;
    f.drawMs += took;
    if (took > f.worstMs) { f.worstMs = took; }
    f.lastChipAt = now;
    if (f.windowStart === 0) {
      f.windowStart = now;

      return;
    }
    const elapsed = now - f.windowStart;
    if (elapsed < fpsWindowMs) { return; }
    const fps = Math.round(f.paints * 1000 / elapsed);
    const avgMs = f.drawMs / Math.max(1, f.paints);
    this.renderFpsChip(fps, avgMs, f.worstMs);
    if (now - f.lastLogAt >= fpsLogMs) {
      f.lastLogAt = now;
      this.logFps(fps, avgMs, f.worstMs);
    }
    f.paints = 0;
    f.drawMs = 0;
    f.worstMs = 0;
    f.windowStart = now;
  },

  // renderFpsChip updates the on-screen counter: the last window
  // reading with the draw cost, colored by health. The write only
  // happens when the text changed - a steady 60 fps window must not
  // dirty the DOM at all.
  renderFpsChip(fps, avgMs, worstMs) {
    let text = "fps: " + fps + " · draw " + avgMs.toFixed(1) + " ms";
    if (worstMs > Math.max(fpsWorstShowMs, 3 * Math.max(avgMs, 0.1))) {
      text += " · worst " + worstMs.toFixed(1) + " ms";
    }
    if (text === this.fps.lastChip) { return; }
    this.fps.lastChip = text;
    const chip = this.fpsChip();
    if (!chip) { return; }
    // textContent and inline style only: the stub DOM of the Node
    // harnesses has no dataset/classList.add, and both are enough for
    // the chip.
    chip.textContent = text;
    chip.style.color = fps >= fpsGoodThreshold
      ? fpsGoodColor : fps >= fpsOkThreshold ? fpsOkColor : fpsBadColor;
  },

  // markFpsIdle flags the chip when nothing painted for a while - a
  // stopped render loop (no movers, no effects) is the normal idle
  // state and must not read as the last window forever.
  markFpsIdle() {
    const f = this.fps;
    if (f.windowStart === 0) { return; }
    if (performance.now() - f.lastChipAt < 1500) { return; }
    if (f.lastChip === "fps: idle") { return; }
    f.lastChip = "fps: idle";
    const chip = this.fpsChip();
    if (chip) {
      chip.textContent = "fps: idle";
      chip.style.color = "";
    }
  },

  // logFps prints the window to the console so the browser devtools
  // log of a lag report carries the numbers (the Node harnesses run
  // without a console - the typeof guard keeps them quiet).
  logFps(fps, avgMs, worstMs) {
    if (typeof console === "undefined" || !console.log) { return; }
    console.log("map fps: " + fps + " fps · draw avg "
      + avgMs.toFixed(1) + " ms · worst " + worstMs.toFixed(1) + " ms");
  },

  // fpsChip resolves the counter element once: it lives in the
  // status bar at the right end of the app footer (index.html
  // #foot-fps) - it shared the map corner with the HUD panels before
  // and overlapped them at the narrow window widths.
  fpsChip() {
    if (!this.fps.chip && typeof document !== "undefined") {
      this.fps.chip = document.getElementById("foot-fps");
    }

    return this.fps.chip;
  },

  // computeUnitScale maps the zoom into a marker size factor: the
  // markers shrink when the user zooms out and grow (bounded) when
  // zooming in. The factor follows the zoom sub linearly so far views
  // keep the units visible while they still shrink clearly.
  computeUnitScale() {
    const factor = Math.pow(this.scale / 0.12, 0.6);

    return Math.max(0.3, Math.min(1.6, factor));
  },

  // drawHuntingZone paints the hunting grounds of the deployment
  // (the registry the hunt loop switches through): every spot draws
  // as a dashed circle with its anchor dot and kill skull, the
  // legacy squares as dashed rectangles - the active ground in
  // bright amber with the thicker stroke, the future ones in a
  // clearly readable soft blue with a light fill (the demonstration
  // of the grounds the bot will hunt next, not a barely visible
  // hint), the demoted bands of the death regression in red. The
  // shapes come out of the world anchored hunt cache as one blit
  // (see huntBg) with the pointer and list emphasis painted fresh on
  // top; environments without an offscreen canvas (the sandboxed
  // Node harnesses) keep the direct batched path. The hunt zones
  // checkbox of the toolbar hides the whole layer. A snapshot
  // without the zone registry falls back to the single legacy
  // hunting square - one shape needs no cache and draws directly.
  drawHuntingZone(ctx, rect) {
    if (!this.layerChecked("show-hunt-zones")) { return; }
    const snap = this.lastSnap;
    if (!snap) { return; }
    if (snap.huntMesh) {
      this.drawHuntCells(ctx, rect, snap);

      return;
    }
    const zones = Array.isArray(snap.huntingZones)
      && snap.huntingZones.length > 0 ? snap.huntingZones : null;
    if (!zones) {
      const zone = snap.huntingZone;
      if (!zone) { return; }
      const legacy = [{
        cx: zone.cx, cy: zone.cy, half: zone.half,
        name: "hunting zone", minLevel: 0, maxLevel: 0, minGear: 0,
        active: true,
      }];
      this.drawHuntingZoneDirect(ctx, rect, legacy);
      this.drawZoneEmphasis(ctx, legacy);

      return;
    }
    if (this.blitHuntLayer(ctx, rect, zones)) {
      this.drawZoneEmphasis(ctx, zones);

      return;
    }
    this.drawHuntingZoneDirect(ctx, rect, zones);
    this.drawZoneEmphasis(ctx, zones);
  },

  // zoneLabeled reports whether a zone carries its name label right
  // now: only the zone under the map cursor (hoverZone) reads its
  // name - the always-on labels of the far zoom smeared into one
  // unreadable blob, so the names wait for the pointer.
  zoneLabeled(zone) {
    return this.hoverZone && this.hoverZone.id === zone.id
      && this.hoverZone.kind === zone.kind;
  },

  // zoneAt hit tests the hunting zones at one client point: the
  // world point of the cursor lands in the smallest containing shape
  // (spots beat squares on overlap, the smaller spot wins). Null
  // when the cursor rests outside every zone of the registry.
  zoneAt(clientX, clientY) {
    const snap = this.lastSnap;
    if (!snap) { return null; }
    if (snap.huntMesh && this.huntMesh) {
      return this.cellAt(clientX, clientY);
    }
    const zones = snap.huntingZones;
    if (!Array.isArray(zones) || zones.length === 0) {
      return snap.huntingZone || null;
    }
    const world = this.screenToWorld(
      clientX - this.view.left, clientY - this.view.top);
    let best = null;
    let bestArea = Infinity;
    for (const zone of zones) {
      const inside = zone.kind === "spot"
        ? Math.hypot(world.x - zone.cx, world.y - zone.cy)
          <= (zone.radius || zone.half)
        : Math.abs(world.x - zone.cx) <= zone.half
          && Math.abs(world.y - zone.cy) <= zone.half;
      if (!inside) { continue; }
      const area = zone.kind === "spot"
        ? Math.PI * Math.pow(zone.radius || zone.half, 2)
        : Math.pow(zone.half * 2, 2);
      if (area < bestArea) {
        bestArea = area;
        best = zone;
      }
    }

    return best;
  },

  // ---- the hunt layer raster and the live emphasis ----
  //
  // The registry splits by what changes its pixels: the shapes (the
  // circles, the squares, the anchor dots, the kill skulls) go into
  // the world anchored offscreen cache below and every frame
  // composites them with one drawImage, while the emphasis (the
  // hovered or listed zone - its thicker stroke, its brighter fill
  // and its name label with the live economy fields) paints fresh per
  // frame on top. The economy fields change every second (the
  // respawn countdown, the adena rate) but only feed the label of
  // at most two zones, which is exactly why they stay out of the
  // cache key.

  // huntZoneVisualKey fingerprints the pixels of the registry: the
  // geometry, the active and demoted markers, the death heat bucket
  // (quantized - the heat fades over minutes, one bucket step is
  // invisible) and the kill centroid. The label only fields (the
  // respawn window, the countdown, the adena rate, the occupancy,
  // the death count) change every second and never touch the
  // raster, so they stay out - keeping them in would re-stroke every
  // circle of the registry on every registry tick.
  huntZoneVisualKey(snapshot) {
    const zones = Array.isArray(snapshot.huntingZones)
      ? snapshot.huntingZones : null;
    if (!zones || zones.length === 0) {
      return snapshot.huntingZone ? "legacy" : "none";
    }
    const parts = [];
    for (const zone of zones) {
      const heat = Math.min(0.55, (zone.deathHeat || 0) * 0.35);
      parts.push(zone.id, zone.kind || "rect", zone.cx, zone.cy,
        zone.radius || zone.half, zone.active ? "a" : "-",
        zone.demoted ? "d" : "-", Math.round(heat * 20),
        zone.killX || 0, zone.killY || 0);
    }

    return parts.join("|");
  },

  // huntLayerKey is the identity of the hunt raster: the zoom (the
  // screen radius of every circle derives from the scale), the
  // visual fingerprint of the registry, the theme revision and the
  // canvas geometry. A camera pan alone does NOT change the key -
  // the raster is world anchored and pans inside its slack box.
  huntLayerKey(rect, dpr) {
    return [this.scale, this.huntKey, this.colorsRev,
      Math.round(rect.width), Math.round(rect.height), dpr].join("|");
  },

  // ensureHuntLayer validates the hunt cache for this frame and
  // re-rasterizes it when the key dropped or the camera left the
  // slack box. False means the caller must paint the layer itself
  // (no offscreen canvas in this environment).
  ensureHuntLayer(rect, zones) {
    const hb = this.huntBg;
    if (!hb.canvas) {
      hb.canvas = this.createBgCanvas();
      if (!hb.canvas) { return false; }
      const hctx = hb.canvas.getContext("2d");
      // A context without drawImage (or a main context without it)
      // cannot blit: keep the direct path instead of a half cache.
      if (!hctx || typeof hctx.drawImage !== "function"
        || !this.ctx || typeof this.ctx.drawImage !== "function") {
        hb.canvas = null;

        return false;
      }
      hb.ctx = hctx;
    }
    const dpr = this.viewDpr || 1;
    const key = this.huntLayerKey(rect, dpr);
    const drifted = Math.abs(this.camX - hb.cx) > hb.marginX
      || Math.abs(this.camY - hb.cy) > hb.marginY;
    if (hb.key !== key || drifted) {
      this.renderHuntLayer(rect, dpr, key, zones);
    }

    return true;
  },

  // renderHuntLayer rasterizes the hunting shapes into the cache.
  // The swap of the view geometry mirrors renderBackground: the
  // direct renderer reads the synced camera state, so the render
  // runs with the cache geometry and restores the real view in a
  // finally (no input handler and no other draw can interleave).
  renderHuntLayer(rect, dpr, key, zones) {
    const hb = this.huntBg;
    const cssW = Math.max(1,
      Math.round(rect.width * (1 + 2 * bgMarginOfView)));
    const cssH = Math.max(1,
      Math.round(rect.height * (1 + 2 * bgMarginOfView)));
    // The device pixel budget of the shape layer is leaner than the
    // imagery one (see huntDevicePixels): the circles are thin
    // strokes, a stepped down raster of them upscales cleanly.
    let cd = Math.min(dpr, 2);
    const fit = Math.sqrt(huntDevicePixels / (cssW * cssH));
    if (cd > fit) { cd = Math.max(1, fit); }
    const devW = Math.max(1, Math.round(cssW * cd));
    const devH = Math.max(1, Math.round(cssH * cd));
    if (hb.canvas.width !== devW) { hb.canvas.width = devW; }
    if (hb.canvas.height !== devH) { hb.canvas.height = devH; }
    const ctx = hb.ctx;
    ctx.setTransform(cd, 0, 0, cd, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);
    const keepView = this.view;
    const keepX = this.camX;
    const keepY = this.camY;
    const cacheView = {
      width: cssW, height: cssH,
      left: keepView.left, top: keepView.top
    };
    this.view = cacheView;
    this.camX = hb.cx = keepX;
    this.camY = hb.cy = keepY;
    try {
      this.drawHuntingZoneDirect(ctx, cacheView, zones);
    } finally {
      this.view = keepView;
      this.camX = keepX;
      this.camY = keepY;
    }
    hb.cssW = cssW;
    hb.cssH = cssH;
    hb.dpr = cd;
    hb.worldLeft = hb.cx - cssW / 2 / this.scale;
    hb.worldTop = hb.cy - cssH / 2 / this.scale;
    hb.marginX = Math.max(1, rect.width * bgMarginOfView - 64)
      / this.scale;
    hb.marginY = Math.max(1, rect.height * bgMarginOfView - 64)
      / this.scale;
    hb.key = key;
  },

  // blitHuntLayer composites the hunt cache onto the frame with one
  // drawImage - the same whole device pixel snap as the background
  // blit (a fractional offset would make the GPU resample the whole
  // raster every frame and smear the dashes).
  blitHuntLayer(ctx, rect, zones) {
    if (!this.ensureHuntLayer(rect, zones)) { return false; }
    const hb = this.huntBg;
    const dpr = this.viewDpr || 1;
    const left = this.camX - rect.width / 2 / this.scale;
    const top = this.camY - rect.height / 2 / this.scale;
    const dx = Math.round((hb.worldLeft - left) * this.scale * dpr) / dpr;
    const dy = Math.round((hb.worldTop - top) * this.scale * dpr) / dpr;
    ctx.drawImage(hb.canvas, 0, 0, hb.canvas.width, hb.canvas.height,
      dx, dy, hb.cssW, hb.cssH);

    return true;
  },

  // ---- the hexagon hunt layer ----
  //
  // The cell mode draws EXACTLY TWO hexagons of the whole partition:
  // the one the bot fights in (the active cell of the live record -
  // the fill, the bright stroke, the live label) and the one under
  // the map cursor (the hover hit test of the mesh - the outline,
  // the light fill and the name label). Every other hexagon of the
  // registry stays INVISIBLE: a 1k+ cell partition stroked as a
  // whole would drown the map, and the partition carries no
  // information the operator needs while the fights run. The mesh
  // payload still arrives once per registry version through
  // /api/hunt-mesh (the hover hit test needs every polygon); the
  // live record of the held cell rides the snapshot.

  // syncHuntMesh fetches the mesh when the snapshot carries a
  // version the cache does not hold. The fetch is async and
  // one-at-a-time; the layer simply draws nothing until the payload
  // lands (a missing mesh never blocks the rest of the map).
  syncHuntMesh(snapshot) {
    const version = snapshot.huntMesh;
    if (!version) { return; }
    if (this.huntMesh && this.huntMesh.version === version) {
      return;
    }
    if (this.huntMeshFetching) { return; }
    if (typeof fetch !== "function") { return; }
    this.huntMeshFetching = true;
    fetch("/api/hunt-mesh")
      .then((res) => {
        if (!res.ok) {
          throw new Error("hunt mesh " + res.status);
        }

        return res.json();
      })
      .then((mesh) => {
        this.huntMesh = mesh;
        this.huntMeshFetching = false;
        this.redraw();
      })
      .catch(() => {
        // A failed fetch retries on the next snapshot tick: the
        // registry endpoint is static, a transient failure is a
        // transient server hiccup.
        this.huntMeshFetching = false;
      });
  },

  // cellMeshIndex builds the id lookup of the cached mesh (the
  // active cell of the snapshot resolves through it).
  cellMeshIndex() {
    if (!this.huntMesh || this.huntMesh._index) {
      return this.huntMesh ? this.huntMesh._index : null;
    }
    const index = new Map();
    for (const cell of this.huntMesh.cells) {
      index.set(cell.id, cell);
    }
    this.huntMesh._index = index;

    return index;
  },

  // drawHuntCells paints the hunt layer of the cell mode: the active
  // hexagon (the one the bot holds or fights in) and the hovered
  // hexagon (the one under the cursor) - nothing else of the
  // partition draws, the render load is two polygons and their
  // labels.
  drawHuntCells(ctx, rect, snap) {
    const mesh = this.huntMesh;
    if (!mesh || mesh.version !== snap.huntMesh) {
      this.syncHuntMesh(snap);

      return;
    }
    this.drawActiveCell(ctx, snap);
    this.drawHoveredCell(ctx);
  },

  // cellPath appends one cell polygon to the ctx path (the flat
  // x,y vertex list, closed).
  cellPath(ctx, cell) {
    const verts = cell.verts;
    let first = this.worldToScreen(verts[0], verts[1]);
    ctx.moveTo(first.x, first.y);
    for (let i = 2; i + 1 < verts.length; i += 2) {
      const p = this.worldToScreen(verts[i], verts[i + 1]);
      ctx.lineTo(p.x, p.y);
    }
    ctx.closePath();
  },

  // drawActiveCell highlights the ONE hexagon the bot fights in (or
  // walks to): the light fill, the bright stroke, the focus dot and
  // the minimal label (the name, the state, the band, the respawn
  // clock and the income of the live record). No other hexagon of
  // the partition draws except the one under the cursor (see
  // drawHoveredCell).
  drawActiveCell(ctx, snap) {
    const live = snap.huntCell;
    if (!live || !live.id) { return; }
    const index = this.cellMeshIndex();
    const cell = index ? index.get(live.id) : null;
    if (!cell) { return; }
    ctx.save();
    ctx.globalAlpha = 0.16;
    ctx.fillStyle = "#f9ab00";
    ctx.beginPath();
    this.cellPath(ctx, cell);
    ctx.fill();
    ctx.globalAlpha = 0.95;
    ctx.strokeStyle = "#f9ab00";
    ctx.lineWidth = 2.5;
    ctx.beginPath();
    this.cellPath(ctx, cell);
    ctx.stroke();
    // The focus dot of the cell (the walk destination).
    const focus = this.worldToScreen(cell.focusX, cell.focusY);
    ctx.fillStyle = "#f9ab00";
    ctx.beginPath();
    ctx.arc(focus.x, focus.y, 4, 0, Math.PI * 2);
    ctx.fill();
    this.drawCellLabel(ctx, live, focus, true);
    ctx.restore();
  },

  // drawHoveredCell draws the hexagon under the map cursor: the
  // outline, the light fill and the name label with the band and the
  // mass of the mesh record (no live economy - the hovered hexagon
  // is a query, not the hunt). Never draws the active hexagon a
  // second time.
  drawHoveredCell(ctx) {
    if (!this.hoverZone || this.hoverZone.kind !== "cell") { return; }
    const live = this.lastSnap ? this.lastSnap.huntCell : null;
    if (live && this.hoverZone.id === live.id) { return; }
    const index = this.cellMeshIndex();
    if (!index) { return; }
    const hovered = index.get(this.hoverZone.id);
    if (!hovered) { return; }
    ctx.save();
    ctx.globalAlpha = 0.10;
    ctx.fillStyle = zoneFutureColor;
    ctx.beginPath();
    this.cellPath(ctx, hovered);
    ctx.fill();
    ctx.globalAlpha = 0.9;
    ctx.strokeStyle = zoneFutureColor;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([6, 4]);
    ctx.beginPath();
    this.cellPath(ctx, hovered);
    ctx.stroke();
    ctx.setLineDash([]);
    const focus = this.worldToScreen(hovered.focusX, hovered.focusY);
    this.drawCellLabel(ctx, {
      id: hovered.id, name: hovered.name,
      state: "hover", minLevel: hovered.minLevel,
      maxLevel: hovered.maxLevel, spawnMass: hovered.mass
    }, focus, false);
    ctx.restore();
  },

  // drawCellLabel writes the label block of a cell near the anchor:
  // the name with the state marker, the level band and the mass; the
  // active cell adds the respawn clock and the measured income of
  // the live record.
  drawCellLabel(ctx, live, anchor, active) {
    let label = live.name;
    if (active) {
      label += live.state === "moving"
        ? " · moving" : " · hunting";
    }
    if (live.maxLevel > 0) {
      label += " · L" + live.minLevel + "-" + live.maxLevel;
    }
    if (live.spawnMass > 0) {
      label += " · " + live.spawnMass + " mobs";
    }
    if (active) {
      if (live.respawnMaxSec > 0) {
        label += " · resp " + live.respawnMinSec + "-"
          + live.respawnMaxSec + "s";
      }
      if (live.adenaPerMin > 0) {
        label += " · " + Math.round(live.adenaPerMin) + "a/min";
      }
      if (live.nextRespawnSec >= 0) {
        label += " · next " + live.nextRespawnSec + "s";
      }
      if (live.occupancy > 1) {
        label += " · " + live.occupancy + " bots";
      }
    }
    ctx.font = this.labelFont;
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
    ctx.strokeText(label, anchor.x + 8, anchor.y - 8);
    ctx.fillStyle = active ? "#f9ab00" : zoneFutureColor;
    ctx.fillText(label, anchor.x + 8, anchor.y - 8);
  },

  // cellAt hit tests the mesh cells at one client point: the flat
  // vertex polygon containment (the edges walk counter-clockwise,
  // the inside keeps the point on the left of every edge). The
  // synthesized zone record feeds the tooltip path of the pointer.
  cellAt(clientX, clientY) {
    const world = this.screenToWorld(
      clientX - this.view.left, clientY - this.view.top);
    const index = this.cellMeshIndex();
    if (!index) { return null; }
    for (const cell of this.huntMesh.cells) {
      const verts = cell.verts;
      let inside = true;
      for (let i = 0; i + 1 < verts.length; i += 2) {
        const ax = verts[i], ay = verts[i + 1];
        const bx = verts[(i + 2) % verts.length];
        const by = verts[(i + 3) % verts.length];
        if ((bx - ax) * (world.y - ay) -
            (by - ay) * (world.x - ax) < 0) {
          inside = false;

          break;
        }
      }
      if (!inside) { continue; }

      return {
        id: cell.id, name: cell.name, kind: "cell",
        cx: cell.focusX, cy: cell.focusY, half: 300
      };
    }

    return null;
  },

  // drawHuntingZoneDirect rasterizes the base shapes of the layer
  // with one path per style group: the whole registry costs a
  // handful of stroke and fill calls instead of a save/restore, two
  // dash arrays and a label concatenation per zone (the per zone
  // state round trips dwarfed the arcs themselves on the ~290 spot
  // far view). The function paints no labels and no hover emphasis
  // - those live in drawZoneEmphasis so the result stays cacheable.
  drawHuntingZoneDirect(ctx, rect, zones) {
    const vw = rect.width;
    const vh = rect.height;
    const futureSpots = [];
    const activeSpots = [];
    const futureRects = [];
    const activeRects = [];
    const demotedRects = [];
    const futureDots = [];
    const activeDots = [];
    const futureSkulls = [];
    const activeSkulls = [];
    const futureHeat = new Map();
    const activeHeat = new Map();
    for (const zone of zones) {
      if (zone.kind === "spot") {
        const c = this.worldToScreen(zone.cx, zone.cy);
        const r = zone.radius * this.scale;
        if (c.x + r < 0 || c.y + r < 0
          || c.x - r > vw || c.y - r > vh) {
          continue;
        }
        const active = zone.active;
        (active ? activeSpots : futureSpots).push([c.x, c.y, r]);
        (active ? activeDots : futureDots).push([c.x, c.y]);
        // The kill centroid skull (where the kills actually happen).
        if (zone.killX || zone.killY) {
          const k = this.worldToScreen(zone.killX, zone.killY);
          if (k.x > -6 && k.y > -6 && k.x < vw + 6 && k.y < vh + 6) {
            (active ? activeSkulls : futureSkulls).push([k.x, k.y]);
          }
        }
        // The death heat fill: the warmer the ground, the redder. The
        // alpha bucket keeps the fills batchable (one bucket step is
        // invisible on a slowly fading heat).
        const heat = Math.min(0.55, (zone.deathHeat || 0) * 0.35);
        if (heat > 0.02) {
          const bucket = Math.round(heat * 20);
          const heatMap = active ? activeHeat : futureHeat;
          let list = heatMap.get(bucket);
          if (!list) { heatMap.set(bucket, (list = [])); }
          list.push([c.x, c.y, r]);
        }
        continue;
      }
      const p = this.worldToScreen(
        zone.cx - zone.half, zone.cy - zone.half);
      const size = zone.half * 2 * this.scale;
      if (p.x > vw || p.y > vh || p.x + size < 0 || p.y + size < 0) {
        continue;
      }
      (zone.active ? activeRects : zone.demoted ? demotedRects
        : futureRects).push([p.x, p.y, size]);
    }
    ctx.save();
    // A light fill demonstrates the future grounds even at the far
    // zoom where a thin dashed outline alone melts into the map.
    if (futureRects.length > 0) {
      ctx.globalAlpha = 0.7;
      ctx.fillStyle = zoneFutureFill;
      ctx.beginPath();
      for (const r of futureRects) {
        ctx.moveTo(r[0], r[1]);
        ctx.lineTo(r[0] + r[2], r[1]);
        ctx.lineTo(r[0] + r[2], r[1] + r[2]);
        ctx.lineTo(r[0], r[1] + r[2]);
        ctx.closePath();
      }
      ctx.fill();
    }
    if (futureSpots.length > 0) {
      ctx.globalAlpha = 0.35;
      ctx.fillStyle = zoneFutureFill;
      ctx.beginPath();
      for (const s of futureSpots) {
        ctx.moveTo(s[0] + s[2], s[1]);
        ctx.arc(s[0], s[1], s[2], 0, Math.PI * 2);
      }
      ctx.fill();
    }
    // The death heat fills, one path per alpha bucket.
    for (const [heatMap, base] of [[futureHeat, 0.75],
      [activeHeat, 0.95]]) {
      for (const [bucket, list] of heatMap) {
        ctx.globalAlpha = base;
        ctx.fillStyle =
          "rgba(217, 48, 37, " + (bucket / 20).toFixed(2) + ")";
        ctx.beginPath();
        for (const s of list) {
          ctx.moveTo(s[0] + s[2], s[1]);
          ctx.arc(s[0], s[1], s[2], 0, Math.PI * 2);
        }
        ctx.fill();
      }
    }
    // The dashed outlines: every zone of the registry shares the
    // dash pattern, so it is set once for the stroke passes below.
    ctx.setLineDash([10, 6]);
    if (futureRects.length > 0) {
      ctx.globalAlpha = 0.7;
      ctx.strokeStyle = zoneFutureColor;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      for (const r of futureRects) {
        ctx.moveTo(r[0], r[1]);
        ctx.lineTo(r[0] + r[2], r[1]);
        ctx.lineTo(r[0] + r[2], r[1] + r[2]);
        ctx.lineTo(r[0], r[1] + r[2]);
        ctx.closePath();
      }
      ctx.stroke();
    }
    if (demotedRects.length > 0) {
      ctx.globalAlpha = 0.75;
      ctx.strokeStyle = "#ff5c5c";
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      for (const r of demotedRects) {
        ctx.moveTo(r[0], r[1]);
        ctx.lineTo(r[0] + r[2], r[1]);
        ctx.lineTo(r[0] + r[2], r[1] + r[2]);
        ctx.lineTo(r[0], r[1] + r[2]);
        ctx.closePath();
      }
      ctx.stroke();
    }
    if (activeRects.length > 0) {
      ctx.globalAlpha = 0.9;
      ctx.strokeStyle = "#f9ab00";
      ctx.lineWidth = 2;
      ctx.beginPath();
      for (const r of activeRects) {
        ctx.moveTo(r[0], r[1]);
        ctx.lineTo(r[0] + r[2], r[1]);
        ctx.lineTo(r[0] + r[2], r[1] + r[2]);
        ctx.lineTo(r[0], r[1] + r[2]);
        ctx.closePath();
      }
      ctx.stroke();
    }
    if (futureSpots.length > 0) {
      ctx.globalAlpha = 0.75;
      ctx.strokeStyle = zoneFutureColor;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      for (const s of futureSpots) {
        ctx.moveTo(s[0] + s[2], s[1]);
        ctx.arc(s[0], s[1], s[2], 0, Math.PI * 2);
      }
      ctx.stroke();
    }
    if (activeSpots.length > 0) {
      ctx.globalAlpha = 0.95;
      ctx.strokeStyle = "#f9ab00";
      ctx.lineWidth = 2.5;
      ctx.beginPath();
      for (const s of activeSpots) {
        ctx.moveTo(s[0] + s[2], s[1]);
        ctx.arc(s[0], s[1], s[2], 0, Math.PI * 2);
      }
      ctx.stroke();
    }
    ctx.setLineDash([]);
    // The kill centroid skulls of the spots with known kill points:
    // the same two pass fill the fleet ring uses (the body in the
    // marker orange, the face in the dark contrast), just larger -
    // the centroid marks the ground, not a single corpse.
    for (const [list, alpha] of [[futureSkulls, 0.75],
      [activeSkulls, 0.95]]) {
      if (list.length === 0) { continue; }
      ctx.globalAlpha = alpha;
      ctx.fillStyle = killMarkColor;
      ctx.beginPath();
      for (const k of list) {
        traceKillSkull(ctx, k[0], k[1], 12.5, false);
      }
      ctx.fill();
      ctx.fillStyle = killMarkDetailColor;
      ctx.beginPath();
      for (const k of list) {
        traceKillSkull(ctx, k[0], k[1], 12.5, true);
      }
      ctx.fill();
    }
    // The anchor dots of the spots.
    if (futureDots.length > 0) {
      ctx.globalAlpha = 0.75;
      ctx.fillStyle = zoneFutureColor;
      ctx.beginPath();
      for (const d of futureDots) {
        ctx.moveTo(d[0] + 2.5, d[1]);
        ctx.arc(d[0], d[1], 2.5, 0, Math.PI * 2);
      }
      ctx.fill();
    }
    if (activeDots.length > 0) {
      ctx.globalAlpha = 0.95;
      ctx.fillStyle = "#f9ab00";
      ctx.beginPath();
      for (const d of activeDots) {
        ctx.moveTo(d[0] + 4, d[1]);
        ctx.arc(d[0], d[1], 4, 0, Math.PI * 2);
      }
      ctx.fill();
    }
    ctx.restore();
  },

  // drawZoneEmphasis paints the pointer and the list emphasis on top
  // of the cached shapes: the hovered zone and the zone focused from
  // the list panel re-stroke their outline with the highlight style,
  // brighten their fill and carry their name label. At most two
  // zones match (the hover and the focus), so the pass costs a
  // couple of shapes per frame - which keeps the seconds changing
  // economy fields (the respawn countdown, the adena rate) out of
  // the cache without ever freezing them on screen.
  drawZoneEmphasis(ctx, zones) {
    for (const zone of zones) {
      if (!this.zoneLabeled(zone)) { continue; }
      if (zone.kind === "spot") {
        this.drawSpotEmphasis(ctx, zone);
      } else {
        this.drawRectEmphasis(ctx, zone);
      }
    }
  },

  // spotLabel assembles the name of a hunting spot with its live
  // economy suffixes: the respawn window of the ground, the
  // measured adena per minute, the death count, the occupancy and
  // the next respawn ETA of the active spot (see drawZoneEmphasis -
  // the label is the only consumer of the per second registry
  // fields).
  spotLabel(zone) {
    let label = zone.name;
    if (zone.maxLevel > 0) {
      label += " · L" + zone.minLevel + "-" + zone.maxLevel;
    }
    if (zone.respawnMaxSec > 0) {
      label += " · resp " + zone.respawnMinSec + "-"
        + zone.respawnMaxSec + "s";
    }
    if (zone.occupancy > 1) {
      label += " · " + zone.occupancy + " bots";
    }
    if (zone.deaths > 0) {
      label += " · " + zone.deaths +
        (zone.deaths === 1 ? " death" : " deaths");
    }
    if (zone.adenaPerMin > 0) {
      label += " · " + Math.round(zone.adenaPerMin) + "a/min";
    }
    if (zone.active) {
      label += " · ACTIVE";
      if (zone.nextRespawnSec >= 0) {
        label += " · next " + zone.nextRespawnSec + "s";
      }
    }

    return label;
  },

  // rectLabel assembles the name of a registry square: the level
  // band, the gear gate of its ladder step, the active and demoted
  // markers and the death count.
  rectLabel(zone) {
    let label = zone.name;
    if (zone.maxLevel > 0) {
      label += " · L" + zone.minLevel + "-" + zone.maxLevel;
    }
    if (!zone.active && zone.minGear > 0) {
      label += " · gear " + zone.minGear + "+";
    }
    if (zone.active) {
      label += " · ACTIVE";
    }
    if (zone.demoted) {
      label += " · too hard";
    }
    if (zone.deaths > 0) {
      label += " · " + zone.deaths +
        (zone.deaths === 1 ? " death" : " deaths");
    }

    return label;
  },

  // drawSpotEmphasis highlights one hunting spot for the pointer or
  // the list: the thicker circle, the brighter fill and the live
  // economy label above the anchor (the base shapes underneath come
  // from the cache or the direct pass and stay untouched).
  drawSpotEmphasis(ctx, zone) {
    const center = this.worldToScreen(zone.cx, zone.cy);
    const radius = zone.radius * this.scale;
    ctx.save();
    const stroke = zone.active ? "#f9ab00" : zoneFutureColor;
    ctx.globalAlpha = 0.95;
    ctx.lineWidth = 2.5;
    ctx.strokeStyle = stroke;
    ctx.setLineDash([10, 6]);
    ctx.beginPath();
    ctx.arc(center.x, center.y, radius, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
    const heat = Math.min(0.55, (zone.deathHeat || 0) * 0.35);
    if (!zone.active && heat <= 0.02) {
      // A cold future ground brightens its fill on the hover; a hot
      // one keeps the death heat tint of the base layer.
      ctx.fillStyle = zoneHoverFill;
      ctx.globalAlpha = 0.5;
      ctx.beginPath();
      ctx.arc(center.x, center.y, radius, 0, Math.PI * 2);
      ctx.fill();
    }
    const label = this.spotLabel(zone);
    ctx.font = this.labelFont;
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
    ctx.strokeText(label, center.x + 8, center.y - 8);
    ctx.fillStyle = stroke;
    ctx.fillText(label, center.x + 8, center.y - 8);
    ctx.restore();
  },

  // drawRectEmphasis highlights one registry square for the pointer
  // or the list: the thicker outline, the brighter fill and the name
  // label at the corner (matching the base square of the layer).
  drawRectEmphasis(ctx, zone) {
    const p1 = this.worldToScreen(
      zone.cx - zone.half, zone.cy - zone.half);
    const size = zone.half * 2 * this.scale;
    ctx.save();
    const stroke = zone.active
      ? "#f9ab00" : zone.demoted ? "#ff5c5c" : zoneFutureColor;
    ctx.globalAlpha = 0.95;
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 2;
    if (!zone.active && !zone.demoted) {
      ctx.fillStyle = zoneHoverFill;
      ctx.fillRect(p1.x, p1.y, size, size);
    }
    ctx.setLineDash([10, 6]);
    ctx.beginPath();
    ctx.moveTo(p1.x, p1.y);
    ctx.lineTo(p1.x + size, p1.y);
    ctx.lineTo(p1.x + size, p1.y + size);
    ctx.lineTo(p1.x, p1.y + size);
    ctx.closePath();
    ctx.stroke();
    ctx.setLineDash([]);
    const label = this.rectLabel(zone);
    ctx.font = this.labelFont;
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
    ctx.strokeText(label, p1.x + 6, p1.y + 14);
    ctx.fillStyle = stroke;
    ctx.fillText(label, p1.x + 6, p1.y + 14);
    ctx.restore();
  },

  // drawKillMarks paints the fleet wide kill skulls: every recent
  // kill of every bot (the /api/fleet/kills ring) draws as a small
  // orange skull that melts away with its age. The layer survives
  // the bot switches of the view - the marks live in the map, not in
  // the snapshot of the observed bot. The fade quantizes into
  // buckets: every skull of one bucket shares a single fill call per
  // pass, so a full ring of kills costs a handful of fills instead
  // of one state round trip per skull (a bucket step of the alpha is
  // invisible on a five minute melt).
  drawKillMarks(ctx, rect) {
    if (!this.layerChecked("show-kills")) { return; }
    if (!this.killMarks || this.killMarks.length === 0) { return; }
    const nowMs = Date.now() + this.clockOffsetMs;
    const buckets = [];
    for (const mark of this.killMarks) {
      const age = nowMs - mark.atMs;
      if (!(age >= 0) || age > killMarkTTLms) { continue; }
      const p = this.worldToScreen(mark.x, mark.y);
      if (p.x < -14 || p.y < -14
        || p.x > rect.width + 14 || p.y > rect.height + 14) {
        continue;
      }
      // The fresh kills read full strength, the old ones melt toward
      // a quarter opacity and shrink before the ring drops them. The
      // fresh skull reads at 2.5x of the old cross size - a bit
      // smaller than the mob dots of the map.
      const bucket = Math.min(killFadeBuckets - 1,
        Math.floor(age / killMarkTTLms * killFadeBuckets));
      (buckets[bucket] = buckets[bucket] || []).push(p);
    }
    ctx.save();
    for (let bucket = 0; bucket < buckets.length; bucket++) {
      const marks = buckets[bucket];
      if (!marks) { continue; }
      const fade = bucket / killFadeBuckets;
      ctx.globalAlpha = 0.95 - 0.7 * fade;
      const size = 10 - 3.75 * fade;
      // The body pass: the head and jaw silhouette in the orange.
      ctx.fillStyle = killMarkColor;
      ctx.beginPath();
      for (const p of marks) {
        traceKillSkull(ctx, p.x, p.y, size, false);
      }
      ctx.fill();
      // The face pass: the eyes and mouth slots in the dark contrast.
      ctx.fillStyle = killMarkDetailColor;
      ctx.beginPath();
      for (const p of marks) {
        traceKillSkull(ctx, p.x, p.y, size, true);
      }
      ctx.fill();
    }
    ctx.restore();
  },

  // setKillMarks ingests the fleet kill ring of /api/fleet/kills (the
  // web app polls it with the bot list). The skulls draw on the next
  // frame - the poll period paces the fade steps well enough.
  setKillMarks(marks) {
    this.killMarks = Array.isArray(marks) ? marks : [];
    this.redraw();
  },

  // drawSocialLinks paints the clan assist network of the living
  // mobs: two npcs of the same clan inside their clan help range
  // connect with a solid line (attacking one pulls the mate - the
  // Mobius notifyActionAttacked clan call), a pair that only
  // approaches the range connects with a dashed warning line. The
  // links connect the units, not a radius circle, so a pack reads
  // as a pack without burying the map under circles.
  drawSocialLinks(ctx, rect) {
    if (!document.getElementById("show-social").checked) { return; }
    const snap = this.lastSnap;
    if (!snap || !snap.objects) { return; }
    const units = [];
    for (const obj of snap.objects) {
      if (obj.kind !== "npc" || obj.dead) { continue; }
      const mask = this.socialMasks.get(obj.objectId);
      if (!mask || !(obj.clanHelpRange > 0)) { continue; }
      const rt = this.runtime.get(obj.objectId);
      const x = rt ? rt.drawX : obj.x;
      const y = rt ? rt.drawY : obj.y;
      // The screen position is computed once per unit: the pair loop
      // below used to transform the same position again for every
      // candidate pair - a dense pack multiplied the transform cost
      // by its square. The position rides the contact offset of the
      // frame, so the pack links follow the slid markers.
      const p = this.unitScreenPos(obj.objectId, x, y);
      units.push({
        x: x, y: y, sx: p.x, sy: p.y,
        low: mask.low, all: mask.all,
        range: obj.clanHelpRange
      });
    }
    if (units.length < 2) { return; }
    ctx.save();
    for (let i = 0; i < units.length; i++) {
      const a = units[i];
      for (let j = i + 1; j < units.length; j++) {
        const b = units[j];
        // The ALL clan links with every clan carrier, the plain
        // clans need a shared bit of the alphabet.
        const linked = (a.all && (b.all || b.low)) || (b.all && a.low)
          || ((a.low & b.low) !== 0);
        if (!linked) { continue; }
        // The assist range of the pair: the wider clan help range of
        // the two governs (the Mobius faction walk of the caller).
        const range = Math.max(a.range, b.range);
        const warn = range * socialWarnFactor;
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const dist = Math.hypot(dx, dy);
        if (dist > warn) { continue; }
        const pa = a;
        const pb = b;
        if ((pa.sx < -40 && pb.sx < -40) || (pa.sy < -40 && pb.sy < -40)
          || (pa.sx > rect.width + 40 && pb.sx > rect.width + 40)
          || (pa.sy > rect.height + 40 && pb.sy > rect.height + 40)) {
          continue;
        }
        if (dist <= range) {
          // Inside the assist range: the pair answers as one.
          ctx.globalAlpha = 0.5;
          ctx.lineWidth = 1.2;
          ctx.setLineDash([]);
          ctx.strokeStyle = socialLinkColor;
        } else {
          // Approaching the range: the dashed warning line.
          ctx.globalAlpha = 0.38;
          ctx.lineWidth = 1;
          ctx.setLineDash([4, 4]);
          ctx.strokeStyle = socialWarnColor;
        }
        ctx.beginPath();
        ctx.moveTo(pa.sx, pa.sy);
        ctx.lineTo(pb.sx, pb.sy);
        ctx.stroke();
      }
    }
    ctx.setLineDash([]);
    ctx.restore();
  },

  drawGrid(ctx, rect) {
    let step = 500;
    while (step * this.scale < 36) { step *= 2; }
    while (step * this.scale > 160) { step /= 2; }
    const centerX = this.centerX();
    const centerY = this.centerY();
    const left = centerX - rect.width / 2 / this.scale;
    const right = centerX + rect.width / 2 / this.scale;
    const top = centerY - rect.height / 2 / this.scale;
    const bottom = centerY + rect.height / 2 / this.scale;
    ctx.lineWidth = 1;
    ctx.font = "10px monospace";
    for (let x = Math.floor(left / step) * step; x <= right; x += step) {
      const p = this.worldToScreen(x, 0);
      ctx.strokeStyle = this.colors.grid;
      ctx.beginPath();
      ctx.moveTo(p.x, 0);
      ctx.lineTo(p.x, rect.height);
      ctx.stroke();
      ctx.fillStyle = this.colors.gridText;
      ctx.fillText(String(x), p.x + 3, 11);
    }
    for (let y = Math.floor(top / step) * step; y <= bottom; y += step) {
      const p = this.worldToScreen(0, y);
      ctx.strokeStyle = this.colors.grid;
      ctx.beginPath();
      ctx.moveTo(0, p.y);
      ctx.lineTo(rect.width, p.y);
      ctx.stroke();
      ctx.fillStyle = this.colors.gridText;
      ctx.fillText(String(y), 3, p.y - 3);
    }
  },

  // drawZone outlines the loaded 3x3 region block around the character:
  // the server only spawns and updates objects inside this square. The
  // position comes from the held camera anchor, so the frame also draws
  // through the bot switch gap (paint paints the static world there)
  // instead of flickering off for the reconnect window.
  drawZone(ctx, rect) {
    if (!document.getElementById("show-zone").checked) { return; }
    const c = this.lastChar;
    if (!c || !c.x) { return; }
    const region = this.regionSize;
    const baseX = Math.floor(c.x / region) - 1;
    const baseY = Math.floor(c.y / region) - 1;
    const p1 = this.worldToScreen(baseX * region, baseY * region);
    const p2 = this.worldToScreen((baseX + 3) * region, (baseY + 3) * region);
    if (p2.x < -20 || p2.y < -20 || p1.x > rect.width + 20 || p1.y > rect.height + 20) {
      return;
    }
    ctx.save();
    ctx.strokeStyle = this.colors.zone;
    ctx.globalAlpha = 0.55;
    ctx.lineWidth = 1.25;
    ctx.setLineDash([7, 5]);
    ctx.strokeRect(p1.x, p1.y, p2.x - p1.x, p2.y - p1.y);
    ctx.restore();

    ctx.fillStyle = this.colors.textDim;
    ctx.font = "10px monospace";
    ctx.textAlign = "left";
    const label = "loaded zone · " + (region * 3) + "×" + (region * 3);
    const lx = Math.max(p1.x, 6);
    const ly = Math.min(Math.max(p1.y + 12, 14), rect.height - 6);
    ctx.fillText(label, lx, ly);
  },

  // drawAggroRanges draws the aggression radius circles of the
  // living aggressive mobs around their drawn (interpolated)
  // positions: the circle is the range the mob attacks a passing
  // player from, so the hunter reads which camps to steer around.
  // The aggro checkbox of the toolbar hides the layer - a full pack
  // of overlapping circles clutters the far zoom. A fighting mob
  // reads red (it already holds a target), the idle ones amber.
  drawAggroRanges(ctx, rect) {
    if (!document.getElementById("show-aggro").checked) { return; }
    if (!this.lastSnap || !this.lastSnap.objects) { return; }
    // The shared style of every circle is set once: the per mob
    // save/restore pairs (a full state vector round trip each) used
    // to cost more than the arcs themselves on a packed field.
    ctx.save();
    ctx.globalAlpha = 0.35;
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 6]);
    for (const obj of this.lastSnap.objects) {
      if (obj.kind !== "npc" || !obj.aggressive || obj.dead) { continue; }
      if (!obj.aggroRange || obj.aggroRange <= 0) { continue; }
      const rt = this.runtime.get(obj.objectId);
      const x = rt ? rt.drawX : obj.x;
      const y = rt ? rt.drawY : obj.y;
      const c = this.worldToScreen(x, y);
      const radius = obj.aggroRange * this.scale;
      if (c.x + radius < 0 || c.x - radius > rect.width
        || c.y + radius < 0 || c.y - radius > rect.height) {
        continue;
      }
      // A sub pixel radius circle at the far zoom is unreadable
      // clutter anyway - skipping it keeps the packed field of the
      // zoomed out view from stroking hundreds of dot circles.
      if (radius < 4) { continue; }
      ctx.strokeStyle = obj.inCombat
        ? this.mapColors.combat : this.mapColors.aggressive;
      ctx.beginPath();
      ctx.arc(c.x, c.y, radius, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.setLineDash([]);
    ctx.restore();
  },

  // computeContactOffsets builds the per frame contact offsets of
  // the unit markers: in melee the combatants stand at the collision
  // distance, and at the zoomed out scales the screen distance drops
  // below the sum of the marker radii - the full size circles would
  // merge into one blob. Every overlapping pair keeps its radii and
  // slides apart along the axis that connects the two centers, so
  // the circles touch face to face with a hair of separation instead
  // (the offset cap keeps a dense crowd from carrying one unit far
  // from its true place). Dead units, ground items and the
  // decorations stay out of it; a unit overlapping several partners
  // accumulates the pushes of every pair.
  computeContactOffsets() {
    const k = this.unitScale || 1;
    const units = [];
    const selfRt = this.runtime.get("self");
    const c = this.lastSnap.character;
    if (c && c.x) {
      const p = this.worldToScreen(
        selfRt ? selfRt.drawX : c.x, selfRt ? selfRt.drawY : c.y);
      units.push({ key: "self", ox: p.x, oy: p.y, x: p.x, y: p.y,
        r: selfMarkerUnits * k });
    }
    for (const obj of this.sortedObjects) {
      if (obj.kind === "item" || obj.dead) { continue; }
      const rt = this.runtime.get(obj.objectId);
      const p = this.worldToScreen(
        rt ? rt.drawX : obj.x, rt ? rt.drawY : obj.y);
      units.push({ key: obj.objectId, ox: p.x, oy: p.y, x: p.x, y: p.y,
        r: radiusOf(obj, threatOf(obj)) * k });
    }
    // The separation pass: pairwise pushes over the working
    // positions, where a later pair sees the pairs before it
    // resolved (a chain of touching units settles in one round, the
    // second catches the rare leftovers). The pairs run in the
    // deterministic snapshot order (the self first, then the sorted
    // objects), so the offsets never flicker frame to frame.
    for (let round = 0; round < 2; round++) {
      for (let i = 0; i < units.length; i++) {
        for (let j = i + 1; j < units.length; j++) {
          const a = units[i];
          const b = units[j];
          const want = a.r + b.r + contactGap;
          const dx = b.x - a.x;
          const dy = b.y - a.y;
          if (dx > want || dx < -want || dy > want || dy < -want) {
            continue;
          }
          const dist = Math.hypot(dx, dy);
          if (dist >= want) { continue; }
          const push = (want - dist) / 2;
          let ux = 1;
          let uy = 0;
          if (dist > 0.001) { ux = dx / dist; uy = dy / dist; }
          a.x -= ux * push;
          a.y -= uy * push;
          b.x += ux * push;
          b.y += uy * push;
        }
      }
    }
    if (this.contactOffsets) {
      this.contactOffsets.clear();
    } else {
      this.contactOffsets = new Map();
    }
    for (const u of units) {
      let dx = u.x - u.ox;
      let dy = u.y - u.oy;
      const drift = Math.hypot(dx, dy);
      const cap = 2 * u.r;
      if (drift > cap) {
        dx = dx / drift * cap;
        dy = dy / drift * cap;
      }
      if (dx !== 0 || dy !== 0) {
        this.contactOffsets.set(u.key, { x: dx, y: dy });
      }
    }
  },

  // unitScreenPos resolves the screen position of one unit marker:
  // the interpolated runtime position (the snapshot coordinates as
  // the fallback) plus the contact offset of the frame. Every
  // marker-anchored visual reads through here - the circle body, the
  // look tick, the name band, the target rings, the combat floats,
  // the cast plate and the hover hit test - so the full size contact
  // slide moves the whole unit together.
  unitScreenPos(key, wx, wy) {
    const rt = this.runtime.get(key);
    const p = this.worldToScreen(rt ? rt.drawX : wx, rt ? rt.drawY : wy);
    const off = this.contactOffsets
      ? this.contactOffsets.get(key) : undefined;
    if (off) { p.x += off.x; p.y += off.y; }

    return p;
  },

  drawObjects(ctx, rect) {
    const showLabels = document.getElementById("show-labels").checked;
    const showDest = document.getElementById("show-dest").checked;
    this.labelCandidates = [];
    // The snapshot objects come out of a go map in random order: the
    // draw order must be stable or overlapping units swap their z
    // position on every snapshot and flicker (dead units always render
    // below the living ones, then north to south). The order depends
    // on the snapshot alone, so update() sorts it once per snapshot
    // (this used to re-sort a copy on every animation frame).
    const objects = this.sortedObjects;
    const nowMs = performance.now();
    // The destination dash lines share their style: set it once for
    // the whole pass instead of a save/restore round trip per unit.
    let destStyled = false;
    for (const obj of objects) {
      const rt = this.runtime.get(obj.objectId) || {
        drawX: obj.x, drawY: obj.y, drawHeading: obj.heading
      };
      const p = this.unitScreenPos(obj.objectId, rt.drawX, rt.drawY);
      if (p.x < -30 || p.y < -30
        || p.x > rect.width + 30 || p.y > rect.height + 30) {
        continue;
      }
      const threat = threatOf(obj);
      if (showDest && obj.moving) {
        if (!destStyled) {
          ctx.save();
          ctx.strokeStyle = this.colors.textDim;
          ctx.setLineDash([4, 3]);
          ctx.lineWidth = 1;
          destStyled = true;
        }
        ctx.globalAlpha = obj.dead ? 0.15 : 0.35;
        const d = this.worldToScreen(obj.destX, obj.destY);
        ctx.beginPath();
        ctx.moveTo(p.x, p.y);
        ctx.lineTo(d.x, d.y);
        ctx.stroke();
      }
      let labelRadius = 4;
      if (obj.kind === "item") {
        drawDiamond(ctx, p.x, p.y, 4, this.mapColors.item);
      } else {
        labelRadius = radiusOf(obj, threat) * this.unitScale;
        drawUnitTick(ctx, p.x, p.y, rt.drawHeading,
          labelRadius, this.mapColors[threat], this.mapColors.tick, {
            dead: obj.dead,
            combat: threat === "combat",
            attackingMe: this.isAttackingMe(obj),
            pulse: nowMs,
            scale: this.unitScale
          });
        this.drawSocialMarker(ctx, p.x, p.y, labelRadius,
          obj.socialUntilMs);
        // The rest icon of every sitting unit, not only the observed
        // bot: the other bots of the fleet appear as player objects of
        // the watched bot's world, and their zZ marker must show them
        // resting regardless of which bot the web UI focuses on.
        if (obj.sitting) {
          this.drawRestMarker(ctx, p.x, p.y, labelRadius);
        }
      }
      if (showLabels && obj.name) {
        const suffix = obj.kind === "npc" && obj.level > 0
          ? " lv" + obj.level : "";
        // White names with the dark halo read over any terrain; the
        // threat stays on the marker dot, the own target keeps its red.
        const isTarget = this.lastSnap.character
          && this.lastSnap.character.targetId === obj.objectId;
        this.labelCandidates.push({
          x: p.x, y: p.y - labelRadius - 6,
          text: obj.name + suffix,
          color: isTarget ? this.mapColors.combat
            : (obj.dead ? "#9aa0a6" : "#ffffff"),
          priority: this.labelPriority(obj)
        });
      }
    }
    if (destStyled) {
      ctx.globalAlpha = 1;
      ctx.setLineDash([]);
      ctx.restore();
    }
  },

  // labelPriority ranks the labels for the declutter pass: the own
  // name, the own target and the hovered unit win over the crowd, the
  // fighting units come next and the rest sort by their distance to
  // the character (the closest names survive a tight zoom out).
  labelPriority(obj) {
    const c = this.lastSnap.character;
    if (this.hover && this.hover.objectId === obj.objectId) { return 0; }
    if (c && c.targetId === obj.objectId) { return 1; }
    if (obj.inCombat) { return 2; }
    const dist = c ? Math.hypot(obj.x - c.x, obj.y - c.y) : 0;

    return 3 + dist / 10000;
  },

  // drawLabels runs the declutter pass and paints the surviving names
  // with a dark halo, which keeps them readable over the light map
  // imagery and over both theme fills alike. The font is the cached
  // labelFont (a getComputedStyle ran per draw before) and the text
  // widths come from the labelWidths cache - measureText used to run
  // per label per frame.
  drawLabels(ctx) {
    ctx.font = this.labelFont;
    ctx.textAlign = "center";
    const taken = [];
    const candidates = this.labelCandidates.slice()
      .sort((a, b) => a.priority - b.priority);
    for (const cand of candidates) {
      const width = this.labelWidth(ctx, cand.text);
      const rect = {
        left: cand.x - width / 2, right: cand.x + width / 2,
        top: cand.y - 12, bottom: cand.y + 2
      };
      const overlaps = taken.some((r) => rect.left < r.right
        && rect.right > r.left && rect.top < r.bottom
        && rect.bottom > r.top);
      if (overlaps) { continue; }
      taken.push(rect);
      ctx.lineWidth = 3;
      ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
      ctx.strokeText(cand.text, cand.x, cand.y);
      ctx.fillStyle = cand.color;
      ctx.fillText(cand.text, cand.x, cand.y);
    }
    ctx.textAlign = "left";
  },

  // labelWidth measures a label text once: the names repeat across
  // frames and snapshots, so the width joins the cache (bounded - a
  // long live session sees many mob names but the cache clears with
  // the theme, and the cap drops a pathological set).
  labelWidth(ctx, text) {
    let width = this.labelWidths.get(text);
    if (width === undefined) {
      if (this.labelWidths.size > 4000) {
        this.labelWidths.clear();
      }
      width = ctx.measureText(text).width + 6;
      this.labelWidths.set(text, width);
    }

    return width;
  },

  isAttackingMe(obj) {
    const c = this.lastSnap.character;

    return obj.inCombat && c && obj.targetId === c.objectId;
  },

  // drawTargetLinks renders the selection links of the map: the own
  // target of the bot (red dashed line and ring, like the L2Bot target
  // markers) and the targets of the other visible players (violet
  // links). The selections of other players matter for the swarm: a
  // mob already selected by someone else is claimed (attacking it
  // trains the mob on the wrong character), and a player targeting
  // the bot itself is worth noticing immediately.
  drawTargetLinks(ctx) {
    if (!document.getElementById("show-targets").checked) { return; }
    const snap = this.lastSnap;
    const c = snap.character;
    if (!c || !c.x) { return; }
    this.drawOwnTargetLink(ctx, c);
    for (const obj of snap.objects || []) {
      if (obj.kind === "player" && obj.targetId) {
        this.drawPlayerTargetLink(ctx, obj, c);
      }
    }
  },

  // drawOwnTargetLink renders the target the bot selected: a red
  // dashed line from the character to the target and a ring around it.
  drawOwnTargetLink(ctx, c) {
    if (!c.targetId) { return; }
    const target = (this.lastSnap.objects || []).find(
      (obj) => obj.objectId === c.targetId);
    if (!target) { return; }
    const from = this.screenPosOf("self", c.x, c.y);
    const to = this.screenPosOf(target.objectId, target.x, target.y);
    ctx.save();
    ctx.strokeStyle = this.mapColors.combat;
    ctx.globalAlpha = 0.75;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([6, 4]);
    ctx.beginPath();
    ctx.moveTo(from.x, from.y);
    ctx.lineTo(to.x, to.y);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();

    const radius = radiusOf(target, threatOf(target))
      * this.unitScale + 6;
    ctx.save();
    ctx.strokeStyle = this.mapColors.combat;
    ctx.globalAlpha = 0.85;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(to.x, to.y, radius, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  },

  // drawPlayerTargetLink renders the selection of another visible
  // player: a violet dashed line to the target and a violet dashed
  // ring around it. The target may be the bot itself (the ring then
  // circles the self marker) or any known object; unknown ids (the
  // target left the loaded zone) are skipped.
  drawPlayerTargetLink(ctx, player, c) {
    const snap = this.lastSnap;
    let target = null;
    let self = false;
    if (player.targetId === c.objectId) {
      target = c;
      self = true;
    } else {
      target = (snap.objects || []).find(
        (obj) => obj.objectId === player.targetId);
    }
    if (!target) { return; }
    const from = this.screenPosOf(player.objectId, player.x, player.y);
    const to = this.screenPosOf(
      self ? "self" : target.objectId, target.x, target.y);

    ctx.save();
    ctx.strokeStyle = this.mapColors.player;
    ctx.globalAlpha = 0.55;
    ctx.lineWidth = 1.25;
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(from.x, from.y);
    ctx.lineTo(to.x, to.y);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();

    const radius = (self ? selfMarkerUnits
      : radiusOf(target, threatOf(target))) * this.unitScale + 5;
    ctx.save();
    ctx.strokeStyle = this.mapColors.player;
    ctx.globalAlpha = 0.75;
    ctx.lineWidth = 1.25;
    ctx.setLineDash([3, 2]);
    ctx.beginPath();
    ctx.arc(to.x, to.y, radius, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
  },

  // screenPosOf resolves the screen position of the runtime
  // interpolated position of an object, falling back to the snapshot
  // position.
  screenPosOf(key, x, y) {
    const rt = this.runtime.get(key);

    return this.worldToScreen(rt ? rt.drawX : x, rt ? rt.drawY : y);
  },

  drawSelf(ctx) {
    const c = this.lastSnap.character;
    if (!c || !c.x) { return; }
    const rt = this.runtime.get("self");
    const heading = rt ? rt.drawHeading : c.heading;
    const p = this.unitScreenPos("self", c.x, c.y);

    // The server side walk of the character gets the same dashed
    // destination line as every other moving object (the paths
    // toggle): the manual walk plan line of drawWalkPlan adds the
    // full planned route on top of it.
    if (document.getElementById("show-dest").checked && c.moving) {
      const d = this.worldToScreen(c.destX, c.destY);
      ctx.save();
      ctx.strokeStyle = this.colors.textDim;
      ctx.globalAlpha = 0.35;
      ctx.setLineDash([4, 3]);
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(p.x, p.y);
      ctx.lineTo(d.x, d.y);
      ctx.stroke();
      ctx.restore();
    }

    // The self marker: the mob parity circle, the accent ring and
    // the look tick. The radius stays the full unit scale whatever
    // the contacts - the marker slides (unitScreenPos above), it
    // never shrinks.
    const selfRadius = selfMarkerUnits * this.unitScale;
    drawUnitTick(ctx, p.x, p.y, heading, selfRadius,
      this.mapColors.self, this.mapColors.tick, {
        self: true, pulse: performance.now(), scale: this.unitScale
      });
    const ring = selfRadius + 3 * this.unitScale
      + 1.5 * Math.sin(performance.now() / 500);
    ctx.strokeStyle = this.mapColors.self;
    ctx.globalAlpha = 0.5;
    ctx.lineWidth = Math.max(1, 1.25 * this.unitScale);
    ctx.beginPath();
    ctx.arc(p.x, p.y, ring, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;

    this.drawSocialMarker(ctx, p.x, p.y, selfRadius, c.socialUntilMs);

    // The resting state of the character: a small breathing zZ over the
    // marker, matching the rest chips of the panels.
    if (c.sitting) {
      this.drawRestMarker(ctx, p.x, p.y, selfRadius);
    }

    if (document.getElementById("show-labels").checked) {
      this.labelCandidates.push({
        x: p.x, y: p.y - selfRadius - 6,
        text: c.name || "self",
        color: "#ffffff",
        priority: -1
      });
    }
  },

  // drawRestMarker draws the breathing zZ over a resting unit (the
  // own character and every observed creature that sits): the shared
  // body of the marker, so the fleet-wide rest icons read identically.
  drawRestMarker(ctx, x, y, radius) {
    ctx.save();
    ctx.font = "700 " + Math.max(8, 10 * this.unitScale)
      + "px sans-serif";
    ctx.textAlign = "left";
    ctx.globalAlpha = 0.6 + 0.4 * Math.sin(performance.now() / 450);
    const zx = x + radius + 4 * this.unitScale;
    const zy = y - radius - 3 * this.unitScale;
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
    ctx.strokeText("zZ", zx, zy);
    ctx.fillStyle = "#ffffff";
    ctx.fillText("zZ", zx, zy);
    ctx.restore();
  },

  // drawSocialMarker draws a small fading ring above a creature that is
  // playing a social animation (the SocialAction broadcast): an
  // unobtrusive hint of who is gesturing without chat spam.
  drawSocialMarker(ctx, x, y, radius, socialUntilMs) {
    const nowMs = Date.now() + this.clockOffsetMs;
    if (!(socialUntilMs > nowMs)) { return; }
    const frac = Math.min(1, (socialUntilMs - nowMs) / socialWindowMs);
    const k = this.unitScale;
    ctx.save();
    ctx.globalAlpha = 0.2 + 0.4 * frac;
    ctx.strokeStyle = this.mapColors.friendly;
    ctx.lineWidth = Math.max(0.75, k);
    ctx.beginPath();
    ctx.arc(x, y - radius - 7 * k, 3.5 * k, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  },

  updateMapInfo() {
    // The chips only re-write when the text actually changed: the
    // scale string holds still through a drag (it changes with the
    // zoom and the window size only) and the objects string is
    // derived per snapshot, so a pan or a render loop frame dirties
    // no layout at all.
    const across = Math.round(this.view.width / this.scale);
    this.setChipText("map-scale",
      "≈ " + across.toLocaleString("en-US") + " units across");
    if (!this.lastSnap) {
      this.setChipText("map-objects", "pathfind test");

      return;
    }
    this.setChipText("map-objects", this.objectsText);
  },

  // buildObjectsText derives the footer object counts of the map
  // from one snapshot: the counts cannot change between snapshots, so
  // the string is computed once per update instead of walked per
  // frame.
  buildObjectsText(snapshot) {
    const counts = { passive: 0, aggressive: 0, combat: 0, player: 0, item: 0 };
    for (const obj of snapshot.objects || []) {
      const threat = threatOf(obj);
      if (counts[threat] !== undefined) { counts[threat]++; }
    }

    return counts.passive + " passive · " + counts.aggressive + " aggro · "
      + counts.combat + " fighting · " + counts.player + " players · "
      + counts.item + " items";
  },

  // setChipText writes a footer chip only on change.
  setChipText(id, text) {
    if (this.chipTexts[id] === text) { return; }
    this.chipTexts[id] = text;
    const el = document.getElementById(id);
    if (el) { el.textContent = text; }
  },

  // updatePathfindCursor keeps the mouse cursor meaningful over the
  // pathfind map: crosshair while a placement is armed, grab over a
  // draggable marker, default elsewhere.
  updatePathfindCursor(event) {
    if (this.drag && this.drag.marker) { return; }
    let cursor = "";
    if (this.pathfind.placeMode) {
      cursor = "crosshair";
    } else if (this.pathfindMarkerAt(event.clientX, event.clientY)) {
      cursor = "grab";
    }
    this.canvas.style.cursor = cursor;
  },

  // ---- hovering ----

  onHover(event) {
    if (this.pathfindEnabled()) {
      this.updatePathfindCursor(event);

      return;
    }
    // A grabbed map never re-queries what sits under the cursor: the
    // drag itself repaints every mousemove and the hit test on top of
    // it doubled the per event work while the pointer moved.
    if (this.drag) { return; }
    this.syncView();
    const best = this.objectAt(event.clientX, event.clientY);
    const zone = this.zoneAt(event.clientX, event.clientY);
    const mx = event.clientX - this.view.left;
    const my = event.clientY - this.view.top;
    // The cursor coordinate readout: the world point under the mouse
    // rides the footer chip, the hovered walk plan waypoint upgrades
    // it to the full triple and draws the label on the map.
    const world = this.screenToWorld(mx, my);
    const wp = this.walkWaypointAt(mx, my);
    const wpChanged = wp !== this.hoverWp;
    this.hoverWp = wp;
    this.cursorWorld = world;
    this.updateCursorChip();
    // The kill skulls pick where no LIVING object does: the unit
    // tooltips own their pixels, but a dead mob (the corpse) sits
    // exactly on its own kill skull - the victim tooltip with the
    // kill age owns that spot until the corpse despawns.
    let mark = null;
    if (!best || best.dead) {
      mark = this.killMarkAt(mx, my);
    }
    if (best !== this.hover || zone !== this.hoverZone || wpChanged
        || mark !== this.hoverMark) {
      this.hover = best;
      this.hoverZone = zone;
      this.hoverMark = mark;
      if (best && !(mark && best.dead)) {
        this.showTooltip(best, mx, my);
      } else if (mark) {
        this.showKillTooltip(mark, mx, my);
      } else {
        this.hideTooltip();
      }
      // The zone hover repaints the highlight and the name label, the
      // waypoint hover the coordinate label.
      this.draw();
    } else if (mark) {
      // The cursor rests on the same skull: keep the age read fresh.
      this.refreshKillTooltipAge(mark);
    }
  },

  // onMapLeave clears the hover state the mouse left behind: the
  // tooltip, the waypoint label (a repaint only when one showed) and
  // the footer cursor chip fall back to the em dash.
  onMapLeave() {
    this.hideTooltip();
    this.cursorWorld = null;
    const hadWaypoint = this.hoverWp >= 0;
    this.hoverWp = -1;
    this.setChipText("foot-cursor", "cursor: —");
    if (hadWaypoint) { this.draw(); }
  },

  // walkWaypointAt hit tests the published walk plan waypoints at one
  // viewport point: the closest waypoint within the label pick radius
  // wins (-1 when the paths toggle is off or nothing sits close).
  walkWaypointAt(mx, my) {
    if (!this.lastSnap || !this.lastSnap.walkPath ||
        !this.lastSnap.walkPath.length) {
      return -1;
    }
    const paths = document.getElementById("show-dest");
    if (paths && !paths.checked) { return -1; }
    let best = -1;
    let bestDist = 12;
    const plan = this.lastSnap.walkPath;
    for (let i = 0; i < plan.length; i++) {
      const p = this.worldToScreen(plan[i].x, plan[i].y);
      const dist = Math.hypot(p.x - mx, p.y - my);
      if (dist < bestDist) {
        bestDist = dist;
        best = i;
      }
    }

    return best;
  },

  // cursorCoordsText renders the coordinate text the cursor chip
  // shows and the ctrl+c copy takes: the world point of the mouse,
  // upgraded to the full x y z triple while the pointer holds a walk
  // plan waypoint (the dump walk line format, paste ready).
  cursorCoordsText() {
    if (!this.cursorWorld) { return null; }
    const plan = this.lastSnap && this.lastSnap.walkPath;
    if (this.hoverWp >= 0 && plan && plan[this.hoverWp]) {
      const wp = plan[this.hoverWp];

      return Math.round(wp.x) + " " + Math.round(wp.y) + " " +
        Math.round(wp.z);
    }

    return Math.round(this.cursorWorld.x) + " " +
      Math.round(this.cursorWorld.y);
  },

  // updateCursorChip writes the footer cursor chip (the deduped
  // setChipText keeps the per mousemove cost at a string compare).
  updateCursorChip() {
    const text = this.cursorCoordsText();
    this.setChipText("foot-cursor",
      text === null ? "cursor: —" : "cursor: " + text);
  },

  // onKeyCopy serves the ctrl+c (and the meta+c) of the map: with the
  // pointer over the world view and no text selection fighting for
  // the clipboard, the shortcut copies the coordinates the cursor
  // chip shows - the full triple on a hovered walk plan waypoint, the
  // bare x y of the world point otherwise. A copy the map does not
  // take (the pointer elsewhere, a selection active) never calls
  // preventDefault, the browser default survives untouched.
  onKeyCopy(event) {
    if (!this.cursorWorld ||
        !(event.ctrlKey || event.metaKey) ||
        (event.key !== "c" && event.key !== "C" &&
          event.code !== "KeyC")) {
      return;
    }
    const selection = typeof window.getSelection === "function"
      ? window.getSelection() : null;
    if (selection && !selection.isCollapsed) { return; }
    const text = this.cursorCoordsText();
    if (!text) { return; }
    event.preventDefault();
    if (typeof navigator !== "undefined" && navigator.clipboard &&
        window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        this.flashCursorCopied(text);
      }, () => {
        if (legacyCopyText(text)) { this.flashCursorCopied(text); }
      });

      return;
    }
    if (legacyCopyText(text)) { this.flashCursorCopied(text); }
  },

  // flashCursorCopied marks the cursor chip with the copy answer for
  // a moment - the shortcut gives its feedback right where the
  // coordinates live.
  flashCursorCopied(text) {
    this.setChipText("foot-cursor", "copied: " + text);
    if (typeof setTimeout !== "function") { return; }
    setTimeout(() => {
      this.updateCursorChip();
    }, 1200);
  },

  // objectAt hit tests the world objects at one client point: the
  // interpolated draw position of every object is checked within the
  // tooltip pick radius.
  objectAt(clientX, clientY) {
    if (!this.lastSnap) { return null; }
    const mx = clientX - this.view.left;
    const my = clientY - this.view.top;
    let best = null;
    let bestDist = 14;
    for (const obj of this.lastSnap.objects || []) {
      const p = this.unitScreenPos(obj.objectId, obj.x, obj.y);
      const dist = Math.hypot(p.x - mx, p.y - my);
      if (dist < bestDist) {
        bestDist = dist;
        best = obj;
      }
    }

    return best;
  },

  // onDoubleClick turns a map double click into a manual command:
  // an attackable monster runs the attack, a ground item runs the
  // pickup, anything else walks to the clicked point (the click
  // height falls back to the character height - the server snap
  // corrects the z on arrival).
  onDoubleClick(event) {
    if (this.pathfindEnabled()) { return; }
    if (!this.lastSnap) { return; }
    this.syncView();
    const obj = this.objectAt(event.clientX, event.clientY);
    const world = this.eventWorld(event);
    const z = this.lastSnap.character ? this.lastSnap.character.z : 0;
    if (obj && obj.kind === "item") {
      postCommand({ kind: "pickup", objectId: obj.objectId });
      this.markUserIntent(obj.x, obj.y);
    } else if (obj && obj.kind === "npc" && obj.attackable && !obj.dead) {
      postCommand({ kind: "attack", objectId: obj.objectId });
      this.markUserIntent(obj.x, obj.y);
    } else {
      postCommand({
        kind: "move",
        x: Math.round(world.x),
        y: Math.round(world.y),
        z: z
      });
      this.markUserIntent(world.x, world.y);
    }
  },

  // The manual command click marker: kept alive by the animation
  // loop while it breathes and fades.
  userMarkAge() {
    if (!this.userMark) { return Infinity; }

    return performance.now() - this.userMark.at;
  },

  markUserIntent(x, y) {
    this.userMark = { x, y, at: performance.now() };
    this.draw();
    this.kickAnimation();
  },

  // drawUserMarker renders the blue destination marker of a manual
  // command: a solid blue dot with a breathing ring that pulses in
  // and out around it. The walk plan marker and the click ripple share
  // it so a click answer never changes style mid walk.
  drawUserMarker(ctx, x, y, nowMs, alpha) {
    const breathe = 0.5 + 0.5 * Math.sin(nowMs / 280);
    const ring = 6.5 + 5.5 * breathe;
    ctx.save();
    ctx.globalAlpha = alpha * (0.4 + 0.6 * (1 - breathe));
    ctx.strokeStyle = this.mapColors.userMark;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(x, y, ring, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = alpha;
    ctx.fillStyle = this.mapColors.userMark;
    ctx.beginPath();
    ctx.arc(x, y, 3.5, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = "rgba(255, 255, 255, 0.9)";
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.restore();
  },

  drawUserIntent(ctx) {
    const age = this.userMarkAge();
    if (age > 2500) {
      this.userMark = null;

      return;
    }
    const p = this.worldToScreen(this.userMark.x, this.userMark.y);
    // The marker stays fully visible for the first stretch, then melts
    // away over the last 700 ms.
    const fade = age < 1800 ? 1 : 1 - (age - 1800) / 700;
    this.drawUserMarker(ctx, p.x, p.y, performance.now(), fade);
  },

  // drawWalkPlan renders the walk plan of a running leg: while the
  // paths toggle is on (always on by default now), the full planned
  // line draws as a bright magenta dashed polyline from the planning
  // origin through every waypoint (the passed ones included - the
  // drift of the character against its plan is the debugging signal),
  // each waypoint carries a filled dot with a white outline and a
  // text label of its (x, y) coordinates, the waypoint the follower
  // aims at carries a larger ring and the destination itself carries
  // the pulsing marker.
  drawWalkPlan(ctx) {
    const plan = this.lastSnap.walkPath;
    if (!plan || plan.length === 0) { return; }
    const origin = this.lastSnap.walkOrigin;
    const target = this.lastSnap.walkIndex >= 0 &&
      this.lastSnap.walkIndex < plan.length
      ? plan[this.lastSnap.walkIndex] : null;
    const dest = this.lastSnap.walkDest ||
      plan[plan.length - 1];
    if (document.getElementById("show-dest").checked) {
      ctx.save();
      ctx.strokeStyle = this.mapColors.userPath;
      ctx.globalAlpha = 0.95;
      ctx.lineWidth = 2.5;
      ctx.setLineDash([7, 4]);
      ctx.beginPath();
      let head;
      if (origin) {
        head = this.worldToScreen(origin.x, origin.y);
      } else {
        const c = this.lastSnap.character;
        const rt = this.runtime.get("self");
        head = this.worldToScreen(
          rt ? rt.drawX : (c && c.x) || 0, rt ? rt.drawY : (c && c.y) || 0);
      }
      ctx.moveTo(head.x, head.y);
      for (const wp of plan) {
        const q = this.worldToScreen(wp.x, wp.y);
        ctx.lineTo(q.x, q.y);
      }
      ctx.stroke();
      // The waypoint dots: a filled magenta disc with a white outline
      // at every waypoint. The coordinate labels draw on hover only
      // (the constant labels littered every planned walk) - the
      // hovered waypoint prints its full x y z triple in the dump walk
      // line format, paste ready; the footer cursor chip and the
      // ctrl+c copy carry the same values.
      ctx.setLineDash([]);
      ctx.lineWidth = 2;
      ctx.font = "11px Consolas, \"Liberation Mono\", monospace";
      ctx.textBaseline = "middle";
      ctx.textAlign = "left";
      for (let i = 0; i < plan.length; i++) {
        const wp = plan[i];
        const q = this.worldToScreen(wp.x, wp.y);
        ctx.beginPath();
        ctx.fillStyle = this.mapColors.userPath;
        ctx.strokeStyle = "rgba(255, 255, 255, 0.95)";
        ctx.arc(q.x, q.y, 3.5, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
        if (i !== this.hoverWp) { continue; }
        const label = Math.round(wp.x) + " " + Math.round(wp.y) + " " +
          Math.round(wp.z);
        ctx.save();
        ctx.globalAlpha = 1;
        ctx.lineWidth = 3.5;
        ctx.strokeStyle = "rgba(0, 0, 0, 0.9)";
        ctx.strokeText(label, q.x + 8, q.y - 11);
        ctx.fillStyle = this.mapColors.userPath;
        ctx.fillText(label, q.x + 8, q.y - 11);
        ctx.restore();
      }
      if (target && plan.length > 1) {
        const t = this.worldToScreen(target.x, target.y);
        ctx.beginPath();
        ctx.arc(t.x, t.y, 6, 0, Math.PI * 2);
        ctx.stroke();
      }
      ctx.restore();
    }
    const t = this.worldToScreen(dest.x, dest.y);
    this.drawUserMarker(ctx, t.x, t.y, performance.now(), 1);
  },

  // onMapDragOver accepts the drag of an equipment widget cell over
  // the map: the browser only allows the drop when the target cancels
  // the default.
  onMapDragOver(event) {
    if (this.pathfindEnabled()) { return; }
    event.preventDefault();
    try {
      event.dataTransfer.dropEffect = "move";
    } catch (err) {
      // Stub events without a dataTransfer ignore the effect hint.
    }
  },

  // onMapDrop drops the dragged widget item on the ground: the server
  // drops at the feet of the character, stackable items open the
  // count dialog first (dropItemOnMap of app.js).
  onMapDrop(event) {
    if (this.pathfindEnabled()) { return; }
    event.preventDefault();
    dropItemOnMap(draggedItem(event));
  },

  showTooltip(obj, mx, my) {
    const deg = Math.round((obj.heading / 65536) * 360);
    const lines = [
      obj.name || ("object " + obj.objectId),
      obj.title ? obj.title : "",
      threatLabel(obj, this.isAttackingMe(obj))
        + (obj.kind === "npc" && obj.level > 0 ? " · lv " + obj.level : ""),
      obj.targetId ? "targets: " + this.displayNameOf(obj.targetId) : "",
      "pos: " + Math.round(obj.x) + " " + Math.round(obj.y) + " " + Math.round(obj.z),
      "facing: " + deg + "° " + cardinalOf(obj.heading),
      obj.kind !== "item" && obj.maxHp > 0
        ? "hp: " + Math.round(obj.curHp) + "/" + Math.round(obj.maxHp) : "",
      obj.kind === "npc" && obj.aggroRange > 0
        ? "aggro range: " + obj.aggroRange
          + (obj.aggressive ? " (attacks on sight)" : " (defensive)") : "",
      obj.kind === "npc" && obj.clanHelpRange > 0
        ? "clan help range: " + obj.clanHelpRange : "",
      obj.moving
        ? "moving to " + Math.round(obj.destX) + " " + Math.round(obj.destY) : "",
      obj.kind === "item" ? "count: " + obj.count : ""
    ].filter(Boolean);
    this.tooltip.innerHTML = "";
    const name = document.createElement("div");
    name.className = "tt-name";
    name.textContent = lines.shift();
    this.tooltip.append(name);
    for (const line of lines) {
      const div = document.createElement("div");
      div.textContent = line;
      this.tooltip.append(div);
    }
    this.positionTooltip(mx, my);
  },

  // displayNameOf resolves the label of a target id for tooltips: the
  // name of a known object (with the npc level), the own name of the
  // bot or a plain object reference.
  displayNameOf(objectId) {
    if (this.lastSnap.character
      && objectId === this.lastSnap.character.objectId) {
      return this.lastSnap.character.name || "self";
    }
    const target = (this.lastSnap.objects || []).find(
      (obj) => obj.objectId === objectId);
    if (target) {
      return target.name
        + (target.kind === "npc" && target.level > 0
          ? " (" + target.level + ")" : "");
    }

    return "object " + objectId;
  },

  // hideTooltip closes the tooltip box and clears the whole hover
  // state: the object, the kill skull and the age line reference.
  hideTooltip() {
    this.tooltip.classList.add("hidden");
    this.hover = null;
    this.hoverMark = null;
    this.hoverAgeEl = null;
  },

  // positionTooltip places the tooltip box near the cursor, clamped
  // to the map wrap (the object and the kill tooltips share it).
  positionTooltip(mx, my) {
    this.tooltip.classList.remove("hidden");
    const wrap = this.canvas.parentElement.getBoundingClientRect();
    const x = Math.min(mx + 14, wrap.width - 270);
    const y = Math.min(my + 14, wrap.height - 130);
    this.tooltip.style.left = x + "px";
    this.tooltip.style.top = y + "px";
  },

  // showKillTooltip shows the victim of a hovered kill skull: the
  // name and level of the mob and how long ago it died (the raw
  // object data never shows, a vanished corpse falls back to the
  // plain mob read).
  showKillTooltip(mark, mx, my) {
    this.tooltip.innerHTML = "";
    const name = document.createElement("div");
    name.className = "tt-name";
    name.textContent = (mark.name || "a mob")
      + (mark.level > 0 ? " lvl " + mark.level : "");
    this.tooltip.append(name);
    const age = document.createElement("div");
    age.textContent = killAgeText(
      Date.now() + this.clockOffsetMs - mark.atMs);
    this.tooltip.append(age);
    this.hoverAgeEl = age;
    this.positionTooltip(mx, my);
  },

  // refreshKillTooltipAge keeps the age read of the hovered skull
  // fresh while the cursor rests on it (a text write only when the
  // whole second stepped).
  refreshKillTooltipAge(mark) {
    if (!this.hoverAgeEl) { return; }
    const text = killAgeText(
      Date.now() + this.clockOffsetMs - mark.atMs);
    if (this.hoverAgeEl.textContent !== text) {
      this.hoverAgeEl.textContent = text;
    }
  },

  // killMarkAt picks the kill skull under the cursor: the nearest
  // fresh mark within the pick radius of the skull. Null when the
  // layer is hidden or nothing sits close enough.
  killMarkAt(mx, my) {
    if (!this.layerChecked("show-kills")) { return null; }
    if (!this.killMarks || this.killMarks.length === 0) { return null; }
    const nowMs = Date.now() + this.clockOffsetMs;
    let best = null;
    let bestDist = killMarkPickRadius;
    for (const mark of this.killMarks) {
      const age = nowMs - mark.atMs;
      if (!(age >= 0) || age > killMarkTTLms) { continue; }
      const p = this.worldToScreen(mark.x, mark.y);
      const dist = Math.hypot(p.x - mx, p.y - my);
      if (dist < bestDist) {
        bestDist = dist;
        best = mark;
      }
    }

    return best;
  },

  // ---- combat animation layer ----

  // ingestCombatEvents replays the fresh server combat events of
  // one snapshot: the sequence cursor dedupes them across the SSE
  // snapshots (every event rides along for the whole two second
  // feed window). The first snapshot after a page load and a bot
  // switch only accept the cursor, so the replay never fires beats
  // that are seconds old.
  ingestCombatEvents(snapshot) {
    const events = snapshot.combatEvents || [];
    const switched = this.lastSnap && this.lastSnap.id !== snapshot.id;
    const first = !this.lastSnap || switched;
    if (first) {
      this.lastCombatSeq = 0;
      this.combatAnims = [];
    }
    for (const ev of events) {
      if (ev.seq <= this.lastCombatSeq) { continue; }
      this.lastCombatSeq = ev.seq;
      if (!first) { this.spawnCombatAnim(ev); }
    }
  },

  // selfBowEquipped reports whether the observed character fights
  // with a bow in hand: the snapshot inventory marks the paperdoll
  // items with equipped and the bows carry the BOW weapon type (the
  // same fields the gear widget reads). A snapshot without the
  // inventory field reads as bare handed - the melee path. The read
  // is one snapshot stale (the combat events ingest before the
  // snapshot lands), so the first shot right after the equip renders
  // as a swing - a cosmetic one event lag.
  selfBowEquipped() {
    const items = this.lastSnap && this.lastSnap.inventory;
    if (!Array.isArray(items)) { return false; }

    return items.some((item) => item && item.equipped
      && item.weaponType === "BOW");
  },

  // spawnCombatAnim turns one fresh combat event into an animation
  // entry: a swing streak from the attacker to the hit target, a
  // flying arrow when the character attacks with a bow (the damage
  // lands only after the server bow wind-up, so the shot must read
  // as a projectile, not as a melee dash), a floating damage number
  // on the hurt unit, or a miss float on the unit an evaded blow was
  // thrown at. The entry captures the event placement so the effect
  // still renders after the unit despawns; while the unit stays on
  // the map the effect follows its interpolated position.
  spawnCombatAnim(ev) {
    const selfId = this.lastSnap.character
      && this.lastSnap.character.objectId;
    if (ev.kind === "attack") {
      const onSelf = ev.targetId === selfId;
      const bySelf = ev.attackerId === selfId;
      // The dealt bow shots fly as projectiles; the taken swings (the
      // mob attacks on the character) and the melee cases keep the
      // swing reading.
      if (bySelf && !onSelf && this.selfBowEquipped()) {
        this.combatAnims.push({
          kind: "bowshot", at: performance.now(), seq: ev.seq,
          attackerId: ev.attackerId, targetId: ev.targetId,
          fromWorld: { x: ev.x, y: ev.y },
          toWorld: { x: ev.targetX, y: ev.targetY },
          bySelf: true, onSelf: false
        });

        return;
      }
      this.combatAnims.push({
        kind: "swing", at: performance.now(), seq: ev.seq,
        attackerId: ev.attackerId, targetId: ev.targetId,
        fromWorld: { x: ev.x, y: ev.y },
        toWorld: { x: ev.targetX, y: ev.targetY },
        bySelf: ev.attackerId === selfId,
        onSelf: ev.targetId === selfId
      });

      return;
    }
    if (ev.kind === "miss") {
      const missJitter = ((ev.seq * 29) % 13 - 6) * 1.2;
      this.combatAnims.push({
        kind: "miss", at: performance.now(), seq: ev.seq,
        objectId: ev.targetId, onSelf: ev.targetId === selfId,
        world: { x: ev.targetX, y: ev.targetY },
        jitter: missJitter
      });

      return;
    }
    if (ev.kind !== "damage" || !(ev.amount > 0)) { return; }
    // The horizontal jitter spreads the numbers of a multi hit
    // burst instead of painting one blob.
    const jitter = ((ev.seq * 37) % 17 - 8) * 1.6;
    this.combatAnims.push({
      kind: "damage", at: performance.now(), seq: ev.seq,
      objectId: ev.targetId, amount: ev.amount,
      crit: Boolean(ev.crit),
      world: { x: ev.x, y: ev.y }, onSelf: ev.targetId === selfId,
      jitter
    });
  },

  // ingestSkillStates reads the running self cast from the snapshot
  // skillStates (the tracker publishes only the played character's
  // windows): the entry with a live cast window becomes the cast
  // icon beside the character, the end time lands on the local
  // performance clock so the fill runs smoothly between the
  // snapshots. A snapshot without a live cast clears the icon. The
  // server runs one cast per creature at a time; if two states ever
  // carried a live cast, the biggest remainder (the newest start)
  // wins.
  ingestSkillStates(snapshot) {
    const states = snapshot.skillStates || [];
    let live = null;
    for (const state of states) {
      if (state.castLeftMs > 0
        && (!live || state.castLeftMs > live.castLeftMs)) {
        live = state;
      }
    }
    if (!live) {
      this.selfCast = null;

      return;
    }
    const endsAt = performance.now() + live.castLeftMs;
    const current = this.selfCast;
    if (current && current.skillId === live.skillId
      && Math.abs(current.endsAt - endsAt) < 400) {
      // The same cast re-read: keep the anchor (the fill never jumps
      // backwards on a re-read).
      current.totalMs = live.castTotalMs;

      return;
    }
    this.selfCast = {
      skillId: live.skillId,
      totalMs: live.castTotalMs,
      endsAt
    };
  },

  // skillIcon returns the icon image entry of one skill icon name,
  // starting the background load once (the geoTiles pattern). The
  // vm sandboxes own no Image constructor - the entry stays
  // not-ready there and the draw falls back to the plain plate.
  skillIcon(name) {
    if (!this.skillIconCache) {
      this.skillIconCache = new Map();
    }
    let entry = this.skillIconCache.get(name);
    if (entry) { return entry; }
    entry = { img: null, ready: false, missing: false };
    this.skillIconCache.set(name, entry);
    if (typeof Image === "undefined") { return entry; }
    const img = new Image();
    img.onload = () => {
      entry.ready = true;
      entry.img = img;
      this.kickAnimation();
    };
    img.onerror = () => { entry.missing = true; };
    img.src = "/icons/" + name + ".png";

    return entry;
  },

  // selfCastIconName resolves the icon file name of the running
  // cast from the learned skill list of the last snapshot.
  selfCastIconName() {
    if (!this.selfCast || !this.lastSnap) { return ""; }
    const id = this.selfCast.skillId;
    for (const skill of this.lastSnap.skills || []) {
      if (skill.skillId === id) { return skill.icon || ""; }
    }

    return "";
  },

  // updateCastIconSide picks the side the cast icon hangs on from
  // the nearest hostile: the screen dx of the closest living
  // attackable object moves the icon to the side AWAY from the fight
  // (the fight floats and the combat labels crowd the enemy side).
  // The nearest fight decides - a distant pack must never outvote
  // the mob the character stands against - and inside the dead band
  // the previous side survives, so a crossing mob does not flip the
  // icon from frame to frame.
  updateCastIconSide(p) {
    let nearestDx = 0;
    let nearestD2 = Infinity;
    for (const obj of this.sortedObjects) {
      if (!obj.attackable || obj.dead) { continue; }
      // The interpolated position source of drawObjects: the moving
      // hostiles weigh with their drawn spot, not their snapshot one
      // (the contact offset included - the icon dodges the enemy as
      // it reads on the map).
      const s = this.unitScreenPos(obj.objectId, obj.x, obj.y);
      const dx = s.x - p.x;
      const d2 = dx * dx + (s.y - p.y) * (s.y - p.y);
      if (d2 < nearestD2) {
        nearestD2 = d2;
        nearestDx = dx;
      }
    }
    if (nearestD2 === Infinity) { return; }
    if (Math.abs(nearestDx) <= 10) { return; }
    this.castIconSide = nearestDx > 0 ? -1 : 1;
  },

  // drawSelfCast draws the cast icon beside the character while it
  // casts: the skill icon in a small plate, dimmed, with the bright
  // portion rising bottom up by the cast progress (the same reading
  // as the skills widget cast fill). The plate hangs off the marker
  // side that points away from the enemy mass (centering it above
  // the marker used to sit inside the self name band), a dotted
  // connector keeps it attached and a thin ring fills rotationally
  // inside the marker circle to mark the caster itself. Before the
  // icon art arrives (and in the icon-less sandboxes) a plain accent
  // plate shows the same fill.
  drawSelfCast(ctx) {
    if (!this.selfCast) { return; }
    const nowMs = performance.now();
    if (nowMs >= this.selfCast.endsAt) {
      this.selfCast = null;

      return;
    }
    const selfId = this.lastSnap.character
      && this.lastSnap.character.objectId;
    const pos = this.effectScreenPos(selfId, {
      x: this.lastChar ? this.lastChar.x : 0,
      y: this.lastChar ? this.lastChar.y : 0
    });
    const k = this.unitScale || 1;
    const selfRadius = selfMarkerUnits * k;
    this.updateCastIconSide(pos);
    const side = this.castIconSide;
    const size = Math.max(14, Math.min(30, 17 * k));
    const progress = Math.max(0, Math.min(1,
      1 - (this.selfCast.endsAt - nowMs) / this.selfCast.totalMs));
    // The plate center rides the marker side, dropped below the
    // combat float lane: the damage floats spawn 8k above the center
    // and rise, so a plate centered on the marker shared its first
    // frames with the number. The vertical dodge keeps the plate out
    // of both lanes (taken flies left, dealt flies right) at every
    // zoom; the name band above stays clear the same way. The inner
    // edge lands selfRadius + 5k from the center, clearing the pulse
    // ring around the full size marker.
    const dodgeY = 10 * k;
    const x = pos.x + side * (selfRadius + 5 * k + size / 2) - size / 2;
    const y = pos.y + dodgeY - size / 2;
    ctx.save();
    // The dotted connector from the marker edge to the plate: the
    // attachment read while the icon hangs beside the character.
    ctx.globalAlpha = 0.9;
    ctx.strokeStyle = "#7cc4ff";
    ctx.lineWidth = 1.2;
    ctx.setLineDash([2, 2]);
    ctx.beginPath();
    ctx.moveTo(pos.x + side * (selfRadius + 1), pos.y);
    ctx.lineTo(pos.x + side * (selfRadius + 5 * k - 1),
      pos.y + dodgeY);
    ctx.stroke();
    ctx.setLineDash([]);
    // The dim plate with the icon (or the plain fallback).
    ctx.globalAlpha = 0.85;
    ctx.fillStyle = "rgba(15, 18, 22, 0.72)";
    ctx.fillRect(x, y, size, size);
    const iconName = this.selfCastIconName();
    const entry = iconName ? this.skillIcon(iconName) : null;
    if (entry && entry.ready) {
      ctx.globalAlpha = 0.45;
      ctx.drawImage(entry.img, x, y, size, size);
    } else {
      ctx.globalAlpha = 0.5;
      ctx.fillStyle = "#7cc4ff";
      ctx.fillRect(x + 2, y + 2, size - 4, size - 4);
    }
    // The bright fill rising bottom up by the cast progress.
    const fillH = size * progress;
    if (fillH > 0.5) {
      ctx.beginPath();
      ctx.rect(x, y + size - fillH, size, fillH);
      ctx.clip();
      ctx.globalAlpha = 1;
      if (entry && entry.ready) {
        ctx.drawImage(entry.img, x, y, size, size);
      } else {
        ctx.fillStyle = "#9fd4ff";
        ctx.fillRect(x + 2, y + 2, size - 4, size - 4);
      }
      ctx.restore();
      ctx.save();
    }
    // The plate border.
    ctx.globalAlpha = 0.9;
    ctx.strokeStyle = "#7cc4ff";
    ctx.lineWidth = 1.2;
    ctx.strokeRect(x, y, size, size);
    // The cast ring inside the marker circle: the faint track plus
    // the bright progress arc sweeping clockwise from the top - the
    // circle fills rotationally while the cast runs. The radius
    // rides the drawn marker radius less the ring inset.
    const ringR = Math.max(2 * k, selfMarkerUnits * k - 2 * k);
    ctx.strokeStyle = "#7cc4ff";
    ctx.lineWidth = 2;
    ctx.lineCap = "round";
    ctx.globalAlpha = 0.25;
    ctx.beginPath();
    ctx.arc(pos.x, pos.y, ringR, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 0.9;
    ctx.beginPath();
    ctx.arc(pos.x, pos.y, ringR,
      -Math.PI / 2, -Math.PI / 2 + Math.PI * 2 * progress);
    ctx.stroke();
    ctx.restore();
  },

  // drawCombatEffects renders the live combat animation layer on
  // top of the units: the swing streaks of the landed hits, the bow
  // arrows of the dealt bow shots, the floating damage numbers of
  // the HP deltas and the miss floats of the evaded blows. Finished
  // entries drop out here; needsMoreFrames keeps the render loop
  // alive while any of them are still running.
  drawCombatEffects(ctx) {
    if (this.combatAnims.length === 0) {
      return;
    }
    const nowMs = performance.now();
    const keep = [];
    for (const anim of this.combatAnims) {
      const life = anim.kind === "swing" ? swingMs
        : anim.kind === "bowshot" ? bowShotMs
        : anim.kind === "miss" ? missMs : damageMs;
      const age = nowMs - anim.at;
      if (age >= life) { continue; }
      keep.push(anim);
      if (anim.kind === "swing") {
        this.drawSwingEffect(ctx, anim, age / life);
      } else if (anim.kind === "bowshot") {
        this.drawBowShotEffect(ctx, anim, age / life);
      } else if (anim.kind === "miss") {
        this.drawMissEffect(ctx, anim, age / life);
      } else {
        this.drawDamageEffect(ctx, anim, age / life);
      }
    }
    this.combatAnims = keep;
  },

  // drawSwingEffect draws one attack: a tapered streak that shoots
  // from the attacker toward the hit target, a windup swoosh arc
  // at the attacker and an impact starburst on the target when the
  // streak lands. The own attacks swing in light blue, the mob
  // attacks in red - both read over the light map imagery and the
  // dark theme fill alike.
  drawSwingEffect(ctx, anim, t) {
    const from = this.effectScreenPos(anim.attackerId, anim.fromWorld);
    const to = this.effectScreenPos(anim.targetId, anim.toWorld);
    const dx = to.x - from.x;
    const dy = to.y - from.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 2) { return; }
    const color = anim.bySelf ? swingSelfColor : swingMobColor;
    const ux = dx / dist;
    const uy = dy / dist;
    ctx.save();
    // The windup swoosh at the attacker: a short arc sweeping
    // around the direction of the strike.
    if (t < 0.45) {
      const w = easeOutQuad(t / 0.45);
      const angle = Math.atan2(uy, ux);
      ctx.globalAlpha = 0.7 * (1 - w);
      ctx.strokeStyle = color;
      ctx.lineWidth = 2.4;
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.arc(from.x, from.y, 9 + 7 * w,
        angle - 1.1 + 0.5 * w, angle - 0.25 + 0.5 * w);
      ctx.stroke();
    }
    // The traveling streak: a tapered dash that shoots from the
    // attacker to the target in the first half of the life.
    if (t < 0.62) {
      const travel = easeOutQuad(Math.min(1, t / 0.62));
      const reach = Math.min(dist, 16 + dist * 0.25) * travel;
      const head = 6 + 14 * travel;
      const tail = Math.max(2, reach - head);
      const x0 = from.x + ux * tail;
      const y0 = from.y + uy * tail;
      const x1 = from.x + ux * reach;
      const y1 = from.y + uy * reach;
      ctx.globalAlpha = 0.9 * (1 - travel * 0.45);
      ctx.strokeStyle = color;
      ctx.lineCap = "round";
      ctx.lineWidth = 3;
      ctx.beginPath();
      ctx.moveTo(x0, y0);
      ctx.lineTo(x1, y1);
      ctx.stroke();
      // A wider faint underlay gives the streak its glow.
      ctx.globalAlpha *= 0.55;
      ctx.lineWidth = 6;
      ctx.beginPath();
      ctx.moveTo(x0, y0);
      ctx.lineTo(x1, y1);
      ctx.stroke();
    }
    // The impact: a white starburst plus a colored flash ring on
    // the target.
    if (t > 0.5) {
      const burst = (t - 0.5) / 0.5;
      const spikes = 6;
      const len = (5 + 9 * easeOutQuad(burst)) *
        (anim.onSelf ? 1.25 : 1);
      ctx.globalAlpha = (1 - burst) * 0.95;
      ctx.strokeStyle = "#ffffff";
      ctx.lineWidth = 1.8;
      ctx.lineCap = "round";
      for (let i = 0; i < spikes; i++) {
        const a = (i / spikes) * Math.PI * 2 + burst * 0.6;
        const r0 = 2.5 + 2 * burst;
        ctx.beginPath();
        ctx.moveTo(to.x + Math.cos(a) * r0, to.y + Math.sin(a) * r0);
        ctx.lineTo(to.x + Math.cos(a) * (r0 + len),
          to.y + Math.sin(a) * (r0 + len));
        ctx.stroke();
      }
      ctx.globalAlpha = (1 - burst) * 0.5;
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(to.x, to.y, 3 + 10 * burst, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.restore();
  },

  // drawBowShotEffect draws one dealt bow attack as a flying arrow:
  // a short bright shaft oriented along the flight direction travels
  // from the shooter to the target over the whole flight time (the
  // damage lands with the arrow, not with the attack order), a faint
  // trail segment trails behind it and a small crossing stroke flash
  // blooms at the target end as the arrow arrives.
  drawBowShotEffect(ctx, anim, t) {
    const from = this.effectScreenPos(anim.attackerId, anim.fromWorld);
    const to = this.effectScreenPos(anim.targetId, anim.toWorld);
    const dx = to.x - from.x;
    const dy = to.y - from.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 2) { return; }
    const k = this.unitScale || 1;
    const ux = dx / dist;
    const uy = dy / dist;
    const head = 10 * k;
    const hx = from.x + dx * easeOutQuad(t);
    const hy = from.y + dy * easeOutQuad(t);
    ctx.save();
    ctx.strokeStyle = swingSelfColor;
    ctx.lineCap = "round";
    // The arrow shaft: a short segment on the flight line.
    ctx.globalAlpha = 0.95;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(hx - ux * head / 2, hy - uy * head / 2);
    ctx.lineTo(hx + ux * head / 2, hy + uy * head / 2);
    ctx.stroke();
    // The trail: a fainter segment behind the shaft tail.
    ctx.globalAlpha = 0.3;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(hx - ux * head / 2, hy - uy * head / 2);
    ctx.lineTo(hx - ux * (head / 2 + 8 * k),
      hy - uy * (head / 2 + 8 * k));
    ctx.stroke();
    // The arrival flash: two short crossing strokes at the target.
    if (t > 0.85) {
      const burst = (t - 0.85) / 0.15;
      const len = 3 + 5 * burst;
      ctx.globalAlpha = (1 - burst) * 0.9;
      ctx.strokeStyle = "#ffffff";
      ctx.lineWidth = 1.6;
      ctx.beginPath();
      ctx.moveTo(to.x - len, to.y);
      ctx.lineTo(to.x + len, to.y);
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(to.x, to.y - len);
      ctx.lineTo(to.x, to.y + len);
      ctx.stroke();
    }
    ctx.restore();
  },

  // drawDamageEffect renders one floating damage number: it pops
  // in with a slight overshoot, rises above the hurt unit and melts
  // away. The hits the character takes read red and fly out to the
  // LEFT of the fight, the damage the character deals amber and to
  // the RIGHT - the direction split of the owner brief; a short
  // flash ring under the number marks the hurt unit itself. The
  // critical blows read a slightly bigger number plus the italic
  // "Crit!" tail styled like the miss float.
  drawDamageEffect(ctx, anim, t) {
    const pos = this.effectScreenPos(anim.objectId, anim.world);
    const k = this.unitScale || 1;
    const rise = easeOutQuad(t) * 30 * k;
    const alpha = t < 0.75 ? 1 : 1 - (t - 0.75) / 0.25;
    const scale = t < 0.14 ? easeOutBack(t / 0.14) : 1;
    const crit = Boolean(anim.crit);
    const color = anim.onSelf ? damageSelfColor : damageMobColor;
    const size = (Math.max(10, Math.min(17,
      (anim.onSelf ? 12 : 11) + Math.sqrt(anim.amount) * 0.7))
      + (crit ? 3 : 0)) * k;
    const x = pos.x + anim.jitter * k
      + (anim.onSelf ? -1 : 1) * floatSideOffset * k;
    const y = pos.y - 8 * k - rise;
    ctx.save();
    // The flash ring under the number.
    ctx.globalAlpha = alpha * 0.55 * (1 - t);
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.arc(pos.x, pos.y, (5 + 13 * t) * k, 0, Math.PI * 2);
    ctx.stroke();
    // The number itself, scaled by the pop and with the same dark
    // halo the map labels use; a critical hit carries the italic
    // "Crit!" tail after it (the miss float styling).
    ctx.translate(x, y);
    ctx.scale(scale, scale);
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.75)";
    ctx.globalAlpha = alpha;
    const text = "-" + Math.round(anim.amount);
    if (crit) {
      const tail = " Crit!";
      ctx.font = "700 " + size.toFixed(1) + "px " + this.sansStack;
      const numWidth = ctx.measureText(text).width;
      ctx.font = "600 italic " + size.toFixed(1) + "px "
        + this.sansStack;
      const tailWidth = ctx.measureText(tail).width;
      const startX = -(numWidth + tailWidth) / 2;
      ctx.textAlign = "left";
      ctx.font = "700 " + size.toFixed(1) + "px " + this.sansStack;
      ctx.strokeText(text, startX, 0);
      ctx.fillStyle = color;
      ctx.fillText(text, startX, 0);
      ctx.font = "600 italic " + size.toFixed(1) + "px "
        + this.sansStack;
      ctx.strokeText(tail, startX + numWidth, 0);
      ctx.fillStyle = color;
      ctx.fillText(tail, startX + numWidth, 0);
    } else {
      ctx.font = "700 " + size.toFixed(1) + "px " + this.sansStack;
      ctx.textAlign = "center";
      ctx.strokeText(text, 0, 0);
      ctx.fillStyle = color;
      ctx.fillText(text, 0, 0);
    }
    ctx.restore();
  },

  // drawMissEffect renders one floating miss marker: the plain
  // "Miss" text where an evaded blow was thrown at, by the same
  // analogy as the damage numbers (pop, rise, melt - no streak, no
  // flash ring: nothing landed on the unit). The same direction
  // split applies: a blow the character dodged reads to the LEFT of
  // the fight, a blow the character missed reads to the RIGHT.
  drawMissEffect(ctx, anim, t) {
    const pos = this.effectScreenPos(anim.objectId, anim.world);
    const k = this.unitScale || 1;
    const rise = easeOutQuad(t) * 24 * k;
    const alpha = t < 0.7 ? 0.95 : 0.95 * (1 - (t - 0.7) / 0.3);
    const scale = t < 0.14 ? easeOutBack(t / 0.14) : 1;
    const size = 11 * k;
    const x = pos.x + anim.jitter * k
      + (anim.onSelf ? -1 : 1) * floatSideOffset * k;
    const y = pos.y - 8 * k - rise;
    ctx.save();
    ctx.translate(x, y);
    ctx.scale(scale, scale);
    ctx.font = "600 italic " + size.toFixed(1) + "px " + this.sansStack;
    ctx.textAlign = "center";
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(15, 18, 22, 0.75)";
    ctx.globalAlpha = alpha;
    ctx.strokeText("Miss", 0, 0);
    ctx.fillStyle = missColor;
    ctx.fillText("Miss", 0, 0);
    ctx.restore();
  },

  // effectScreenPos resolves the screen position of an animation
  // anchor: the interpolated runtime position while the unit is
  // still on the map (the self character included), the captured
  // event placement once it despawned. The contact offset applies
  // while the unit lives, so the floats and the swings stay glued to
  // the slid marker; a despawned anchor keeps its captured spot.
  effectScreenPos(id, fallbackWorld) {
    const selfId = this.lastSnap.character
      && this.lastSnap.character.objectId;

    return this.unitScreenPos(id === selfId ? "self" : id,
      fallbackWorld.x, fallbackWorld.y);
  },

  // ---- transforms ----

  // World to screen transform: pure math over the synced view cache
  // (one geometry read per frame batch, see syncView - this function
  // fires hundreds of times per frame and used to force a layout on
  // every call). Follow mode centers on the character, free mode
  // stays pinned to the pan anchor.
  worldToScreen(wx, wy) {
    return {
      x: this.view.width / 2 + (wx - this.camX) * this.scale,
      y: this.view.height / 2 + (wy - this.camY) * this.scale
    };
  },

  centerX() {
    return this.camX;
  },

  centerY() {
    return this.camY;
  },

  charPos() {
    if (!this.lastSnap) {
      // The bot switch gap: hold the last known character position so
      // the follow camera keeps framing the same map area while the
      // new event stream connects (the origin jump read as a white
      // flash on every sidebar click).
      return this.lastChar || { x: 0, y: 0 };
    }
    const rt = this.runtime.get("self");
    if (rt) { return { x: rt.drawX, y: rt.drawY }; }
    const c = this.lastSnap.character;
    if (!c || !c.x) { return this.lastChar || { x: 0, y: 0 }; }

    return { x: c.x, y: c.y };
  },

  followEnabled() {
    if (this.pathfindEnabled()) { return false; }

    return document.getElementById("follow").checked;
  }
};

// turnHeading rotates a heading value toward the target on the shortest
// arc by the given fraction.
function turnHeading(current, target, fraction) {
  const a = (current / 65536) * 360;
  const b = (target / 65536) * 360;
  let delta = ((b - a + 540) % 360) - 180;

  return normHeading(a + delta * fraction);
}

// ---- movement interpolation calibration ----

// clockSampleWindow bounds how many recent clock offset samples are kept
// for the maximum based server clock estimate.
const clockSampleWindow = 20;

// teleportUnits is the drawn displacement above which a discontinuity is
// treated as a teleport and rendered instantly instead of glided.
const teleportUnits = 400;

// defaultCollisionRadius is the arrival collision estimate for views
// whose packets carry no collision radius. The mob and player packets
// always carry one; only exotic views can fall back to it.
const defaultCollisionRadius = 9;

// chaseFactor bounds how much faster than its unit a drawn position may
// glide while catching up with the projected server position.
const chaseFactor = 1.35;

// chaseFloor is the minimum catch up speed in world units per second so
// near destination residuals settle quickly for slow units too.
const chaseFloor = 60;

// The fps meter windows: the chip refreshes every half second, the
// console log line lands every five seconds (a lag report pastes the
// log; the chip is for watching while it happens).
const fpsWindowMs = 500;
const fpsLogMs = 5000;

// The fps health thresholds of the chip coloring: 45 and up reads
// good, 28 and up reads degraded, below that the counter reads red.
// The colors follow the fixed diagnostic palette (they must read over
// the light map imagery in both themes, like the map markers).
const fpsGoodThreshold = 45;
const fpsOkThreshold = 28;
const fpsGoodColor = "#188038";
const fpsOkColor = "#9a6700";
const fpsBadColor = "#d93025";
// fpsWorstShowMs is the paint time a single worst frame must exceed
// before the chip shows it next to the average.
const fpsWorstShowMs = 8;

// bgMarginOfView is the overdraw of the static background cache: the
// raster covers the viewport plus this fraction of it per side, so
// the follow camera of a walking bot or a dragged view pans inside
// the cached world for a full viewport before the cache re-renders
// (0.5 = one viewport of slack around the view).
const bgMarginOfView = 0.5;

// tilesCommitMs is the throttle window of the tile arrival commits
// into the background cache key (see tileArrived): a zoom-out load
// burst lands dozens of tiles over seconds, and every arrival would
// otherwise re-raster the whole static world on the next animated
// frame - the load window the users felt as the low fps of a
// loading map. One immediate commit plus one coalesced commit per
// window keeps the progressive refinement at a readable cadence.
const tilesCommitMs = 250;

// bgDevicePixels caps the offscreen cache raster: beyond it the cache
// resolution steps down so even a 4K class viewport keeps the cache
// memory in the tens of megabytes, not the hundreds.
const bgDevicePixels = 4096 * 2560;

// huntDevicePixels caps the hunt layer cache raster: the layer holds
// thin vector strokes (the dashed circles, the kill skulls), so its
// budget is leaner than the imagery cache - a stepped down raster
// of a 1.5px dash upscales cleanly and keeps the second offscreen
// canvas in the low tens of megabytes even on a 4K class viewport.
const huntDevicePixels = 4096 * 1440;

// killFadeBuckets is the quantization of the kill skull fade: every
// skull of one bucket shares one fill call per pass (see
// drawKillMarks).
const killFadeBuckets = 8;

// socialWindowMs is how long the social animation marker stays visible
// (the tracker side window in state/chat.go).
const socialWindowMs = 3000;

// swingMs is the life of one attack animation: the windup swoosh,
// the streak that shoots from the attacker to the hit target and
// the impact starburst (the Mobius attack cadence is roughly one
// zoneFutureColor is the stroke of the inactive hunting zones: a
// bright soft blue that reads over the light map imagery and both
// theme fills alike, clearly distinct from the amber active square
// and the red demoted bands.
const zoneFutureColor = "#5b9bd5";

// zoneFutureFill is the faint fill of the inactive hunting zones: it
// demonstrates the future grounds at the far zoom where a thin
// outline alone melts into the map imagery.
const zoneFutureFill = "rgba(91, 155, 213, 0.07)";

// zoneHoverFill is the brighter fill of the hovered or listed zone:
// the pointer (map hover) or the list focus marks the ground among
// its neighbors at a glance.
const zoneHoverFill = "rgba(91, 155, 213, 0.22)";

// killMarkColor is the fill of the fleet kill skulls (the same
// orange the per spot kill centroid skull uses, so every kill marker
// on the map reads as one family).
const killMarkColor = "#e37400";

// killMarkDetailColor is the fixed dark contrast of the skull face
// (the eye dots and the mouth slots): a dark brown that stays
// readable over the orange fill on the light map imagery, theme
// independent like the rest of the kill marker palette.
const killMarkDetailColor = "#40230a";

// killMarkPickRadius bounds the hover pick of a kill skull: a touch
// wider than the fresh skull so the tooltip is easy to aim at.
const killMarkPickRadius = 13;

// killAgeText renders the age of a kill mark for the skull tooltip:
// a compact whole unit read (the map local helper - the HUD panels
// use the formatAgeMs of app.js, the vm harness loads map.js alone).
function killAgeText(ms) {
  const secs = Math.max(0, Math.floor(ms / 1000));
  if (secs < 60) { return "killed " + secs + "s ago"; }
  if (secs < 3600) {
    return "killed " + Math.floor(secs / 60) + "m ago";
  }

  return "killed " + Math.floor(secs / 3600) + "h "
    + Math.floor((secs % 3600) / 60) + "m ago";
}

// killMarkTTLms bounds the life of a fleet kill skull: the fresh kill
// reads full strength and melts away before the server ring drops
// it (the hunt loop keeps five minutes of kills per bot).
const killMarkTTLms = 5 * 60 * 1000;

// socialLinkColor connects the clan mates inside their clan help
// range: a calm teal, distinct from every threat color of the units
// (attacking one of the pair pulls its mate - the Mobius clan call).
const socialLinkColor = "#0aa5a5";

// socialWarnColor marks the pairs that only approach their clan help
// range: the dashed amber warning that the link is one step away.
const socialWarnColor = "#e37400";

// socialWarnFactor bounds the warning band of the social links: a
// pair within range times this factor but outside the range itself
// draws as approaching (the dashed line), so a spreading pack warns
// before it actually links.
const socialWarnFactor = 1.25;

// swing a second, so the effects of a running fight never overlap
// into one smear).
const swingMs = 340;

// bowShotMs is the flight time of one bow projectile: the arrow
// crosses from the shooter to the target while the server bow
// wind-up runs, so the arrival flash lands with the damage, not
// with the attack order.
const bowShotMs = 500;

// damageMs is the life of one floating damage number: it pops in,
// rises above the hurt unit and melts away.
const damageMs = 950;

// missMs is the life of one floating miss marker: the same reading
// as the damage number, slightly shorter (nothing landed, so the
// float clears out of the way sooner).
const missMs = 800;

// floatSideOffset is the horizontal float offset in unit scale
// pixels: the damage the character takes pops to the LEFT of the
// fight, the damage it deals (and its misses) to the RIGHT.
const floatSideOffset = 15;

// The combat animation palette: the own swings read light blue, the
// mob swings red; the damage numbers on the mobs the character
// grinds render amber, the hits the character takes red; the miss
// floats stay neutral gray white on both sides.
const swingSelfColor = "#7cc4ff";
const swingMobColor = "#ff6b4a";
const damageMobColor = "#ffd25c";
const damageSelfColor = "#ff5252";
const missColor = "#dfe6ee";

// selfMarkerUnits is the self marker radius in the same unit space
// radiusOf sizes the world markers with: the character reads as one
// combatant among the others (mob combat parity), no bigger-self
// emphasis.
const selfMarkerUnits = 6;

// contactGap is the hair of separation the contact pass leaves
// between two touching circles: exactly tangent rims fuse under the
// canvas anti aliasing, the half pixel seam keeps the pair readable.
const contactGap = 0.5;

// easeOutQuad eases t out: fast at the start, settled at the end.
function easeOutQuad(t) {
  return 1 - (1 - t) * (1 - t);
}

// easeOutBack eases t out with a small overshoot: the pop of the
// damage numbers when a hit lands.
function easeOutBack(t) {
  const c = 1.70158;

  return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
}

// normHeading maps an arbitrary degree value back to the game range.
function normHeading(deg) {
  const value = ((deg % 360) + 360) % 360;

  return Math.round(value * 65536 / 360);
}

// threatOf classifies an object for coloring by danger.
function threatOf(obj) {
  if (obj.kind === "player") { return "player"; }
  if (obj.kind === "item") { return "item"; }
  if (obj.dead) { return "dead"; }
  if (obj.inCombat) { return "combat"; }
  if (obj.aggressive) { return "aggressive"; }
  if (obj.attackable) { return "passive"; }

  return "friendly";
}

// threatLabel names the threat class for tooltips.
function threatLabel(obj, attackingMe) {
  if (obj.kind === "player") { return "player"; }
  if (obj.kind === "item") { return "ground item"; }
  if (attackingMe) { return "fighting YOU"; }
  if (obj.dead) { return "dead"; }
  if (obj.inCombat) { return "in combat"; }
  if (obj.aggressive) { return "aggressive monster"; }
  if (obj.attackable) { return "passive monster"; }

  return "friendly npc";
}

// radiusOf sizes the marker by importance.
function radiusOf(obj, threat) {
  if (obj.kind === "player") { return 5.5; }
  if (threat === "combat") { return 6; }
  if (threat === "dead") { return 4; }

  return 5;
}

// labelColor returns the name text color: white for everything alive
// (readable over any terrain with the dark halo), gray for the dead.
function labelColor(threat) {
  return threat === "dead" ? "#9aa0a6" : "#ffffff";
}

// drawUnitTick draws a circle marker with a short look direction tick
// leaving the center and reaching slightly over the circle edge, like the
// L2Bot2.0 map. opts.scale is the zoom factor of the marker (unitScale):
// the tick length, the ring radii and the line widths shrink with it so
// a zoomed out marker stays a small circle with a proportionally small
// tick instead of a dot with a fixed length stick.
function drawUnitTick(ctx, x, y, heading, radius, fill, tick, opts) {
  const k = opts.scale || 1;
  const angle = (heading / 65536) * 2 * Math.PI;
  const alpha = opts.dead ? 0.45 : 1;

  // Combat pulse ring and self ring for emphasis.
  if (opts.combat || opts.self) {
    const pulse = opts.self ? 3 * k : 2.5 * Math.sin(opts.pulse / 220) + 3 * k;
    ctx.strokeStyle = opts.self ? fill : fill;
    ctx.globalAlpha = 0.35;
    ctx.lineWidth = Math.max(1, 1.5 * k);
    ctx.beginPath();
    ctx.arc(x, y, radius + pulse, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  // The circle body.
  ctx.globalAlpha = alpha;
  ctx.beginPath();
  ctx.arc(x, y, radius, 0, Math.PI * 2);
  ctx.fillStyle = fill;
  ctx.fill();
  ctx.lineWidth = Math.max(0.75, 1.25 * k);
  ctx.strokeStyle = tick;
  ctx.stroke();

  // A mob attacking the character gets a thin targeting ring.
  if (opts.attackingMe) {
    ctx.lineWidth = Math.max(1, 1.5 * k);
    ctx.strokeStyle = tick;
    ctx.setLineDash([3 * k, 2 * k]);
    ctx.beginPath();
    ctx.arc(x, y, radius + 5 * k, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  // The look direction tick, drawn only outside the circle: the circle
  // body is a solid color fill and the heading ray starts at the edge.
  const outer = radius + 4.5 * k;
  ctx.beginPath();
  ctx.moveTo(x + Math.cos(angle) * radius, y + Math.sin(angle) * radius);
  ctx.lineTo(x + Math.cos(angle) * outer, y + Math.sin(angle) * outer);
  ctx.lineWidth = Math.max(1, 2 * k);
  ctx.lineCap = "round";
  ctx.strokeStyle = tick;
  ctx.stroke();
  ctx.lineCap = "butt";
  ctx.globalAlpha = 1;
}

function drawDiamond(ctx, x, y, size, color) {
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(Math.PI / 4);
  ctx.fillStyle = color;
  ctx.fillRect(-size, -size, size * 2, size * 2);
  ctx.restore();
}

// traceKillSkull traces one stylized skull onto the current canvas
// path, centered on (x, y) with the half size r. The body pass (face
// false) adds the cranium circle and the jaw circle, the face pass
// (face true) adds the two eye dots and the two mouth slots - the
// caller fills the passes with killMarkColor and killMarkDetailColor
// in two batched fill calls. Traced from primitives, never a font
// glyph, so the face stays crisp at every device pixel ratio and
// still reads as a skull at the ~8px melt of the oldest fade bucket.
function traceKillSkull(ctx, x, y, r, face) {
  if (!face) {
    // The cranium: a wide circle over a smaller jaw circle - the
    // silhouette alone reads skull from a distance.
    ctx.moveTo(x + r * 0.75, y - r * 0.15);
    ctx.arc(x, y - r * 0.15, r * 0.75, 0, Math.PI * 2);
    ctx.moveTo(x + r * 0.45, y + r * 0.45);
    ctx.arc(x, y + r * 0.45, r * 0.45, 0, Math.PI * 2);

    return;
  }
  // The eye dots sit inside the cranium, above the jaw line.
  ctx.moveTo(x - r * 0.13, y - r * 0.3);
  ctx.arc(x - r * 0.33, y - r * 0.3, r * 0.22, 0, Math.PI * 2);
  ctx.moveTo(x + r * 0.53, y - r * 0.3);
  ctx.arc(x + r * 0.33, y - r * 0.3, r * 0.22, 0, Math.PI * 2);
  // The mouth slots: the two dark teeth gaps of the jaw slab.
  ctx.moveTo(x - r * 0.3, y + r * 0.15);
  ctx.lineTo(x - r * 0.1, y + r * 0.15);
  ctx.lineTo(x - r * 0.1, y + r * 0.55);
  ctx.lineTo(x - r * 0.3, y + r * 0.55);
  ctx.closePath();
  ctx.moveTo(x + r * 0.1, y + r * 0.15);
  ctx.lineTo(x + r * 0.3, y + r * 0.15);
  ctx.lineTo(x + r * 0.3, y + r * 0.55);
  ctx.lineTo(x + r * 0.1, y + r * 0.55);
  ctx.closePath();
}

function cardinalOf(heading) {
  const deg = Math.round((heading / 65536) * 360);
  const names = ["E", "SE", "S", "SW", "W", "NW", "N", "NE"];

  return names[Math.round(deg / 45) % 8];
}
