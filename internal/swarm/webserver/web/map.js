/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// MapView renders the world around the active bot on a canvas. The map is
// the source of truth: the character status lives on it as a HUD panel,
// world objects are drawn as circle + look direction ticks like L2Bot,
// colored by threat level, and a request animation frame loop interpolates
// every movement between the server updates so positions are always
// current.
const MapView = {
  canvas: null,
  ctx: null,
  tooltip: null,
  scale: 0.12,
  offsetX: 0,
  offsetY: 0,
  drag: null,
  hover: null,
  lastSnap: null,
  clockOffsetMs: 0,
  runtime: new Map(),
  animating: false,
  lastFrame: 0,
  colors: null,

  // The server world region grid: every region is 2048 units and every
  // object within the 3x3 region block around the player is loaded (see
  // World.broadcastPacket of the Mobius server).
  regionSize: 2048,

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
    for (const id of ["follow", "show-labels", "show-dest", "show-zone"]) {
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
      this.clockOffsetMs = snapshot.serverTimeMs - Date.now();
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
  },

  onDragMove(event) {
    if (!this.drag) { return; }
    document.getElementById("follow").checked = false;
    this.offsetX += event.clientX - this.drag.x;
    this.offsetY += event.clientY - this.drag.y;
    this.drag = { x: event.clientX, y: event.clientY };
    this.draw();
  },

  // ---- movement interpolation ----

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

  // updateRuntime advances the interpolated position of every object and
  // exponentially smooths the drawn position toward it. The played
  // character is interpolated the same way from its own movement
  // broadcasts.
  updateRuntime(dt) {
    if (!this.lastSnap) { return; }
    const nowMs = Date.now() + this.clockOffsetMs;
    const smoothing = 1 - Math.exp(-dt * 10);
    const turning = 1 - Math.exp(-dt * 12);
    this.updateSelfRuntime(nowMs, smoothing, turning);
    for (const obj of this.lastSnap.objects || []) {
      const target = this.projectObject(obj, nowMs);
      let rt = this.runtime.get(obj.objectId);
      if (!rt) {
        rt = {
          drawX: target.x, drawY: target.y, drawHeading: obj.heading,
          settled: true, init: true
        };
        this.runtime.set(obj.objectId, rt);
        continue;
      }
      const jump = Math.hypot(target.x - rt.drawX, target.y - rt.drawY);
      if (jump > 400 || !rt.init) {
        rt.drawX = target.x;
        rt.drawY = target.y;
        rt.drawHeading = obj.heading;
        rt.settled = true;
        rt.init = true;
        continue;
      }
      if (jump < 0.4) {
        rt.drawX = target.x;
        rt.drawY = target.y;
        rt.settled = true;
      } else {
        rt.drawX += (target.x - rt.drawX) * smoothing;
        rt.drawY += (target.y - rt.drawY) * smoothing;
        rt.settled = false;
      }
      rt.drawHeading = turnHeading(rt.drawHeading, obj.heading, turning);
    }
  },

  // projectObject estimates the current server position of an object:
  // standing objects keep their last position, moving ones traveled from
  // the packet position toward the destination at the transmitted speed.
  projectObject(obj, nowMs) {
    if (!obj.moving || !(obj.speed > 0)) {
      return { x: obj.x, y: obj.y };
    }
    const dx = obj.destX - obj.x;
    const dy = obj.destY - obj.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 1) {
      return { x: obj.destX, y: obj.destY };
    }
    const elapsed = Math.max(0, (nowMs - (obj.moveAtMs || 0)) / 1000);
    const traveled = Math.min(obj.speed * elapsed, dist);
    const frac = traveled / dist;

    return { x: obj.x + dx * frac, y: obj.y + dy * frac };
  },

  // updateSelfRuntime interpolates the played character position toward
  // its movement destination.
  updateSelfRuntime(nowMs, smoothing, turning) {
    const c = this.lastSnap.character;
    if (!c || !c.x) { return; }
    const target = this.projectObject({
      x: c.x, y: c.y, moving: c.moving, speed: c.speed,
      destX: c.destX, destY: c.destY, moveAtMs: c.moveAtMs
    }, nowMs);
    let rt = this.runtime.get("self");
    if (!rt) {
      rt = { drawX: target.x, drawY: target.y, drawHeading: c.heading, settled: true };
      this.runtime.set("self", rt);
    }
    const jump = Math.hypot(target.x - rt.drawX, target.y - rt.drawY);
    if (jump > 400) {
      rt.drawX = target.x;
      rt.drawY = target.y;
      rt.settled = true;
    } else if (jump < 0.4) {
      rt.drawX = target.x;
      rt.drawY = target.y;
      rt.settled = true;
    } else {
      rt.drawX += (target.x - rt.drawX) * smoothing;
      rt.drawY += (target.y - rt.drawY) * smoothing;
      rt.settled = false;
    }
    rt.drawHeading = turnHeading(rt.drawHeading, c.heading, turning);
  },

  // ---- drawing ----

  draw() {
    const ctx = this.ctx;
    if (!ctx || !this.colors) { return; }
    const rect = this.canvas.getBoundingClientRect();
    ctx.clearRect(0, 0, rect.width, rect.height);
    if (!this.lastSnap) { return; }

    this.drawGrid(ctx, rect);
    this.drawZone(ctx, rect);
    this.drawObjects(ctx, rect);
    this.drawSelf(ctx);
    this.updateMapInfo();
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
    const objects = this.lastSnap.objects || [];
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
        drawUnitTick(ctx, p.x, p.y, rt.drawHeading,
          radiusOf(obj, threat), this.colors[threat], this.colors.textBright, {
            dead: obj.dead,
            combat: threat === "combat",
            attackingMe: this.isAttackingMe(obj),
            pulse: performance.now()
          });
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

  drawSelf(ctx) {
    const c = this.lastSnap.character;
    if (!c || !c.x) { return; }
    const rt = this.runtime.get("self");
    const heading = rt ? rt.drawHeading : c.heading;
    const p = this.worldToScreen(
      rt ? rt.drawX : c.x, rt ? rt.drawY : c.y);

    // The self marker: bigger circle, accent ring and the look tick.
    drawUnitTick(ctx, p.x, p.y, heading, 7,
      this.colors.self, this.colors.textBright, { self: true, pulse: performance.now() });
    const ring = 10 + 1.5 * Math.sin(performance.now() / 500);
    ctx.strokeStyle = this.colors.selfRing;
    ctx.globalAlpha = 0.5;
    ctx.lineWidth = 1.25;
    ctx.beginPath();
    ctx.arc(p.x, p.y, ring, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;

    ctx.fillStyle = this.colors.textBright;
    ctx.font = "600 11px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(c.name || "self", p.x, p.y - 15);
    ctx.textAlign = "left";
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

  hideTooltip() {
    this.tooltip.classList.add("hidden");
    this.hover = null;
  },

  // ---- transforms ----

  // World to screen transform. Follow mode centers on the character.
  worldToScreen(wx, wy) {
    const rect = this.canvas.getBoundingClientRect();
    const cx = rect.width / 2;
    const cy = rect.height / 2;
    let originX = cx;
    let originY = cy;
    if (!this.followEnabled() && this.lastSnap) {
      originX += this.offsetX;
      originY += this.offsetY;
    }

    return {
      x: originX + (wx - this.centerX()) * this.scale,
      y: originY + (wy - this.centerY()) * this.scale
    };
  },

  centerX() {
    return this.followEnabled() ? this.charPos().x : this.panCenterX();
  },

  centerY() {
    return this.followEnabled() ? this.charPos().y : this.panCenterY();
  },

  charPos() {
    if (!this.lastSnap) { return { x: 0, y: 0 }; }
    const rt = this.runtime.get("self");
    if (rt) { return { x: rt.drawX, y: rt.drawY }; }

    return { x: this.lastSnap.character.x, y: this.lastSnap.character.y };
  },

  panCenterX() {
    return this.panAnchor ? this.panAnchor.x : this.charPos().x;
  },

  panCenterY() {
    return this.panAnchor ? this.panAnchor.y : this.charPos().y;
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
// L2Bot2.0 map.
function drawUnitTick(ctx, x, y, heading, radius, fill, tick, opts) {
  const angle = (heading / 65536) * 2 * Math.PI;
  const alpha = opts.dead ? 0.45 : 1;

  // Combat pulse ring and self ring for emphasis.
  if (opts.combat || opts.self) {
    const pulse = opts.self ? 3 : 2.5 * Math.sin(opts.pulse / 220) + 3;
    ctx.strokeStyle = opts.self ? fill : fill;
    ctx.globalAlpha = 0.35;
    ctx.lineWidth = 1.5;
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
  ctx.lineWidth = 1.25;
  ctx.strokeStyle = tick;
  ctx.stroke();

  // A mob attacking the character gets a thin targeting ring.
  if (opts.attackingMe) {
    ctx.lineWidth = 1.5;
    ctx.strokeStyle = tick;
    ctx.setLineDash([3, 2]);
    ctx.beginPath();
    ctx.arc(x, y, radius + 5, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  // The look direction tick from the center over the edge.
  const inner = radius * 0.2;
  const outer = radius + 4;
  ctx.beginPath();
  ctx.moveTo(x + Math.cos(angle) * inner, y + Math.sin(angle) * inner);
  ctx.lineTo(x + Math.cos(angle) * outer, y + Math.sin(angle) * outer);
  ctx.lineWidth = 2;
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
