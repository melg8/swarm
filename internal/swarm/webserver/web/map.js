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
  scale: 0.12,
  panAnchor: { x: 0, y: 0 },
  drag: null,
  hover: null,
  lastSnap: null,
  clockOffsetMs: 0,
  clockSamples: [],
  runtime: new Map(),
  animating: false,
  lastFrame: 0,
  colors: null,
  mapTiles: new Map(),

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
    window.addEventListener("mouseup", () => { this.drag = null; });
    this.canvas.addEventListener("mousemove", (e) => this.onHover(e));
    this.canvas.addEventListener("mouseleave", () => this.hideTooltip());
    document.getElementById("zoom-in")
      .addEventListener("click", () => this.zoom(1.5));
    document.getElementById("zoom-out")
      .addEventListener("click", () => this.zoom(1 / 1.5));
    const follow = document.getElementById("follow");
    follow.addEventListener("change", () => {
      if (!follow.checked) { this.syncPanAnchor(); }
      this.draw();
    });
    for (const id of ["show-labels", "show-dest", "show-zone", "show-targets", "show-map"]) {
      document.getElementById(id).addEventListener("change", () => {
        this.draw();
      });
    }
  },

  // Canvas colors follow the active theme variables.
  refreshColors() {
    const style = getComputedStyle(document.documentElement);
    const read = (name) => style.getPropertyValue(name).trim();
    this.colors = {
      grid: read("--grid"),
      gridText: read("--grid-text"),
      self: read("--blue"),
      selfRing: read("--accent"),
      player: read("--violet"),
      item: read("--gold"),
      friendly: read("--gray"),
      passive: read("--green"),
      aggressive: read("--accent"),
      combat: read("--red"),
      dead: read("--gray"),
      text: read("--text"),
      textBright: read("--text-bright"),
      textDim: read("--text-dim"),
      border: read("--border"),
      zone: read("--text-dim"),
      path: read("--accent")
    };
    this.draw();
  },

  resize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = Math.max(1, Math.floor(rect.width * dpr));
    this.canvas.height = Math.max(1, Math.floor(rect.height * dpr));
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.draw();
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
    const alive = new Set(["self"]);
    for (const obj of snapshot.objects || []) {
      alive.add(obj.objectId);
    }
    for (const id of this.runtime.keys()) {
      if (!alive.has(id)) { this.runtime.delete(id); }
    }
    this.lastSnap = snapshot;
    this.kickAnimation();
    this.draw();
  },

  zoom(factor) {
    this.scale = Math.max(0.008, Math.min(1.5, this.scale * factor));
    this.draw();
  },

  onWheel(event) {
    event.preventDefault();
    const factor = event.deltaY < 0 ? 1.15 : 1 / 1.15;
    this.zoom(factor);
  },

  onDragStart(event) {
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
  // shows the chosen area even when the bot walks away.
  onDragMove(event) {
    if (!this.drag) { return; }
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
    const dt = Math.min(0.1, (ts - this.lastFrame) / 1000);
    this.lastFrame = ts;
    if (this.mapVisible()) {
      this.updateRuntime(dt);
      this.draw();
    }
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

    return this.smoothingPending();
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
      const p = this.projectTickwise(view, nowMs);
      rt = {
        drawX: p.x, drawY: p.y, drawHeading: view.heading || 0,
        settled: true, lastV: 0
      };
      this.runtime.set(key, rt);
    }
    if (view.speed > 0) {
      rt.lastV = view.speed;
    }
    const target = this.projectTickwise(view, nowMs);
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
  projectTickwise(view, nowMs) {
    if (!view.moving || !(view.speed > 0)) {
      return { x: view.x, y: view.y };
    }
    const dist = Math.hypot(view.destX - view.x, view.destY - view.y);
    if (dist < 1) {
      return { x: view.destX, y: view.destY };
    }
    const collision = view.collisionRadius > 0
      ? view.collisionRadius : defaultCollisionRadius;
    const step = view.speed / 10;
    const tickFloat = Math.max(0, nowMs - (view.moveAtMs || 0)) / 100;
    const whole = Math.floor(tickFloat);
    const frac = tickFloat - whole;
    let x = view.x;
    let y = view.y;
    for (let k = 0; k <= whole; k++) {
      const remainingX = view.destX - x;
      const remainingY = view.destY - y;
      const remaining = Math.hypot(remainingX, remainingY);
      const delta = Math.max(0.00001, remaining - collision);
      const advance = step / delta;
      const nextX = advance >= 1 ? view.destX : x + remainingX * advance;
      const nextY = advance >= 1 ? view.destY : y + remainingY * advance;
      if (k === whole) {
        // The window between tick k and tick k+1: interpolate.
        return { x: x + (nextX - x) * frac, y: y + (nextY - y) * frac };
      }
      x = nextX;
      y = nextY;
    }

    return { x, y };
  },

  // ---- world map background ----

  // drawMapBackground paints the game world map tiles under everything
  // else. The level of detail follows the zoom: the chosen pyramid
  // level is the smallest one whose native resolution stays within a
  // factor of two of the drawn tile size, so zoomed in views use the
  // full tiles and zoomed out views use the small ones without
  // over-magnifying.
  drawMapBackground(ctx, rect) {
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
    ctx.imageSmoothingQuality = "high";
    for (let bx = bxMin; bx <= bxMax; bx++) {
      for (let by = byMin; by <= byMax; by++) {
        const tileImg = this.mapTile(level, bx, by);
        if (!tileImg) { continue; }
        const screen = this.worldToScreen(
          (bx - this.mapTileZeroX) * tile, (by - this.mapTileZeroY) * tile);
        ctx.drawImage(tileImg, screen.x, screen.y,
          tile * this.scale + 1, tile * this.scale + 1);
      }
    }
  },

  // mapTile returns the loaded image of a pyramid tile or null and
  // starts the background load once. Missing tiles (outside the
  // shipped world) are remembered to avoid retrying.
  mapTile(level, bx, by) {
    const path = "maps/" + level + "/" + bx + "_" + by + ".jpg";
    let entry = this.mapTiles.get(path);
    if (entry) {
      return entry.ready ? entry.img : null;
    }
    entry = { img: null, ready: false };
    this.mapTiles.set(path, entry);
    if (typeof Image === "undefined") {
      return null;
    }
    const img = new Image();
    img.onload = () => {
      entry.ready = true;
      entry.img = img;
      this.draw();
    };
    img.onerror = () => {};
    img.src = path;

    return null;
  },

  // ---- drawing ----

  draw() {
    const ctx = this.ctx;
    if (!ctx || !this.colors) { return; }
    const rect = this.canvas.getBoundingClientRect();
    ctx.clearRect(0, 0, rect.width, rect.height);
    if (!this.lastSnap) { return; }

    this.drawMapBackground(ctx, rect);
    this.unitScale = this.computeUnitScale();
    this.drawGrid(ctx, rect);
    this.drawZone(ctx, rect);
    this.drawTargetLinks(ctx);
    this.drawObjects(ctx, rect);
    this.drawSelf(ctx);
    this.updateMapInfo();
  },

  // computeUnitScale maps the zoom into a marker size factor: the
  // markers shrink when the user zooms out and grow (bounded) when
  // zooming in. The factor follows the zoom sub linearly so far views
  // keep the units visible while they still shrink clearly.
  computeUnitScale() {
    const factor = Math.pow(this.scale / 0.12, 0.6);

    return Math.max(0.3, Math.min(1.6, factor));
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
  // the server only spawns and updates objects inside this square.
  drawZone(ctx, rect) {
    if (!document.getElementById("show-zone").checked) { return; }
    const c = this.lastSnap.character;
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

  drawObjects(ctx, rect) {
    const showLabels = document.getElementById("show-labels").checked;
    const showDest = document.getElementById("show-dest").checked;
    // The snapshot objects come out of a go map in random order: the
    // draw order must be stable or overlapping units swap their z
    // position on every snapshot and flicker (dead units always render
    // below the living ones, then north to south).
    const objects = (this.lastSnap.objects || []).slice()
      .sort((a, b) => ((a.dead ? 0 : 1) - (b.dead ? 0 : 1))
        || (a.y - b.y) || (a.objectId - b.objectId));
    for (const obj of objects) {
      const rt = this.runtime.get(obj.objectId) || {
        drawX: obj.x, drawY: obj.y, drawHeading: obj.heading
      };
      const p = this.worldToScreen(rt.drawX, rt.drawY);
      if (p.x < -30 || p.y < -30
        || p.x > rect.width + 30 || p.y > rect.height + 30) {
        continue;
      }
      const threat = threatOf(obj);
      if (showDest && obj.moving) {
        const d = this.worldToScreen(obj.destX, obj.destY);
        ctx.save();
        ctx.strokeStyle = this.colors.path;
        ctx.globalAlpha = obj.dead ? 0.15 : 0.35;
        ctx.setLineDash([4, 3]);
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(p.x, p.y);
        ctx.lineTo(d.x, d.y);
        ctx.stroke();
        ctx.restore();
      }
      if (obj.kind === "item") {
        drawDiamond(ctx, p.x, p.y, 4, this.colors.item);
      } else {
        const radius = radiusOf(obj, threat) * this.unitScale;
        drawUnitTick(ctx, p.x, p.y, rt.drawHeading,
          radius, this.colors[threat], this.colors.textBright, {
            dead: obj.dead,
            combat: threat === "combat",
            attackingMe: this.isAttackingMe(obj),
            pulse: performance.now(),
            scale: this.unitScale
          });
        this.drawSocialMarker(ctx, p.x, p.y, radius, obj.socialUntilMs);
      }
      if (showLabels && obj.name) {
        ctx.fillStyle = labelColor(this.colors, threat);
        ctx.font = "10px sans-serif";
        ctx.textAlign = "center";
        const suffix = obj.kind === "npc" && obj.level > 0
          ? " (" + obj.level + ")" : "";
        ctx.fillText(obj.name + suffix, p.x, p.y - 11);
        ctx.textAlign = "left";
      }
    }
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
    ctx.strokeStyle = this.colors.combat;
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
    ctx.strokeStyle = this.colors.combat;
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
    ctx.strokeStyle = this.colors.player;
    ctx.globalAlpha = 0.55;
    ctx.lineWidth = 1.25;
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(from.x, from.y);
    ctx.lineTo(to.x, to.y);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();

    const radius = (self ? 7 : radiusOf(target, threatOf(target)))
      * this.unitScale + 5;
    ctx.save();
    ctx.strokeStyle = this.colors.player;
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
    const p = this.worldToScreen(
      rt ? rt.drawX : c.x, rt ? rt.drawY : c.y);

    // The self marker: bigger circle, accent ring and the look tick.
    const selfRadius = 7 * this.unitScale;
    drawUnitTick(ctx, p.x, p.y, heading, selfRadius,
      this.colors.self, this.colors.textBright, {
        self: true, pulse: performance.now(), scale: this.unitScale
      });
    const ring = selfRadius + 3 * this.unitScale
      + 1.5 * Math.sin(performance.now() / 500);
    ctx.strokeStyle = this.colors.selfRing;
    ctx.globalAlpha = 0.5;
    ctx.lineWidth = Math.max(1, 1.25 * this.unitScale);
    ctx.beginPath();
    ctx.arc(p.x, p.y, ring, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;

    this.drawSocialMarker(ctx, p.x, p.y, 7, c.socialUntilMs);

    ctx.fillStyle = this.colors.textBright;
    ctx.font = "600 11px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(c.name || "self", p.x, p.y - 15);
    ctx.textAlign = "left";
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
    ctx.strokeStyle = this.colors.textDim;
    ctx.lineWidth = Math.max(0.75, k);
    ctx.beginPath();
    ctx.arc(x, y - radius - 7 * k, 3.5 * k, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  },

  updateMapInfo() {
    const rect = this.canvas.getBoundingClientRect();
    const across = Math.round(rect.width / this.scale);
    document.getElementById("map-scale").textContent =
      "≈ " + across.toLocaleString("en-US") + " units across";
    const objects = this.lastSnap.objects || [];
    const counts = { passive: 0, aggressive: 0, combat: 0, player: 0, item: 0 };
    for (const obj of objects) {
      const threat = threatOf(obj);
      if (counts[threat] !== undefined) { counts[threat]++; }
    }
    document.getElementById("map-objects").textContent =
      counts.passive + " passive · " + counts.aggressive + " aggro · "
      + counts.combat + " fighting · " + counts.player + " players · "
      + counts.item + " items";
  },

  // ---- hovering ----

  onHover(event) {
    if (!this.lastSnap) { return; }
    const rect = this.canvas.getBoundingClientRect();
    const mx = event.clientX - rect.left;
    const my = event.clientY - rect.top;
    let best = null;
    let bestDist = 14;
    for (const obj of this.lastSnap.objects || []) {
      const rt = this.runtime.get(obj.objectId);
      const p = this.worldToScreen(
        rt ? rt.drawX : obj.x, rt ? rt.drawY : obj.y);
      const dist = Math.hypot(p.x - mx, p.y - my);
      if (dist < bestDist) {
        bestDist = dist;
        best = obj;
      }
    }
    if (best !== this.hover) {
      this.hover = best;
      if (best) {
        this.showTooltip(best, mx, my);
      } else {
        this.hideTooltip();
      }
    }
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
    this.tooltip.classList.remove("hidden");
    const wrap = this.canvas.parentElement.getBoundingClientRect();
    const x = Math.min(mx + 14, wrap.width - 270);
    const y = Math.min(my + 14, wrap.height - 130);
    this.tooltip.style.left = x + "px";
    this.tooltip.style.top = y + "px";
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

  hideTooltip() {
    this.tooltip.classList.add("hidden");
    this.hover = null;
  },

  // ---- transforms ----

  // World to screen transform. Follow mode centers on the character,
  // free mode stays pinned to the pan anchor.
  worldToScreen(wx, wy) {
    const rect = this.canvas.getBoundingClientRect();
    const cx = rect.width / 2;
    const cy = rect.height / 2;

    return {
      x: cx + (wx - this.centerX()) * this.scale,
      y: cy + (wy - this.centerY()) * this.scale
    };
  },

  centerX() {
    return this.followEnabled() ? this.charPos().x : this.panAnchor.x;
  },

  centerY() {
    return this.followEnabled() ? this.charPos().y : this.panAnchor.y;
  },

  charPos() {
    if (!this.lastSnap) { return { x: 0, y: 0 }; }
    const rt = this.runtime.get("self");
    if (rt) { return { x: rt.drawX, y: rt.drawY }; }

    return { x: this.lastSnap.character.x, y: this.lastSnap.character.y };
  },

  followEnabled() {
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

// socialWindowMs is how long the social animation marker stays visible
// (the tracker side window in state/chat.go).
const socialWindowMs = 3000;

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

// labelColor picks a readable label color for a threat class.
function labelColor(colors, threat) {
  if (threat === "dead") { return colors.dead; }
  if (threat === "combat") { return colors.combat; }
  if (threat === "aggressive") { return colors.aggressive; }

  return colors.textDim;
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

function cardinalOf(heading) {
  const deg = Math.round((heading / 65536) * 360);
  const names = ["E", "SE", "S", "SW", "W", "NW", "N", "NE"];

  return names[Math.round(deg / 45) % 8];
}
