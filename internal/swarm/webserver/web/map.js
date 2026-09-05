/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// MapView renders the world around the active bot on a canvas. Every
// object is drawn as a direction arrow rotated by its heading value
// (0..65535, 0 = east, clockwise) like the map of L2Bot.
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

  init() {
    this.canvas = document.getElementById("map-canvas");
    this.ctx = this.canvas.getContext("2d");
    this.tooltip = document.getElementById("map-tooltip");
    this.resize();
    window.addEventListener("resize", () => this.resize());
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
    document.getElementById("follow").addEventListener("change", () => {
      this.draw();
    });
    document.getElementById("show-labels").addEventListener("change", () => {
      this.draw();
    });
    document.getElementById("show-dest").addEventListener("change", () => {
      this.draw();
    });
  },

  resize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.canvas.width = Math.max(1, Math.floor(rect.width * dpr));
    this.canvas.height = Math.max(1, Math.floor(rect.height * dpr));
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.draw();
  },

  update(snapshot) {
    this.lastSnap = snapshot;
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

  onHover(event) {
    if (!this.lastSnap) { return; }
    const rect = this.canvas.getBoundingClientRect();
    const mx = event.clientX - rect.left;
    const my = event.clientY - rect.top;
    let best = null;
    let bestDist = 14;
    for (const obj of this.lastSnap.objects || []) {
      const p = this.worldToScreen(obj.x, obj.y);
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
      "kind: " + obj.kind + (obj.attackable ? " (hostile)" : ""),
      "pos: " + obj.x + " " + obj.y + " " + obj.z,
      "facing: " + deg + "° " + cardinalOf(obj.heading),
      obj.kind === "npc" && obj.maxHp > 0
        ? "hp: " + Math.round(obj.curHp) + "/" + Math.round(obj.maxHp) : "",
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
    const y = Math.min(my + 14, wrap.height - 120);
    this.tooltip.style.left = x + "px";
    this.tooltip.style.top = y + "px";
  },

  hideTooltip() {
    this.tooltip.classList.add("hidden");
    this.hover = null;
  },

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
  },

  draw() {
    const ctx = this.ctx;
    if (!ctx) { return; }
    const rect = this.canvas.getBoundingClientRect();
    ctx.clearRect(0, 0, rect.width, rect.height);
    if (!this.lastSnap) { return; }

    this.drawGrid(ctx, rect);
    this.drawObjects(ctx);
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
      ctx.strokeStyle = "rgba(58, 66, 82, 0.35)";
      ctx.beginPath();
      ctx.moveTo(p.x, 0);
      ctx.lineTo(p.x, rect.height);
      ctx.stroke();
      ctx.fillStyle = "rgba(125, 133, 144, 0.5)";
      ctx.fillText(String(x), p.x + 3, 11);
    }
    for (let y = Math.floor(top / step) * step; y <= bottom; y += step) {
      const p = this.worldToScreen(0, y);
      ctx.strokeStyle = "rgba(58, 66, 82, 0.35)";
      ctx.beginPath();
      ctx.moveTo(0, p.y);
      ctx.lineTo(rect.width, p.y);
      ctx.stroke();
      ctx.fillStyle = "rgba(125, 133, 144, 0.5)";
      ctx.fillText(String(y), 3, p.y - 3);
    }
  },

  drawObjects(ctx) {
    const showLabels = document.getElementById("show-labels").checked;
    const showDest = document.getElementById("show-dest").checked;
    const objects = this.lastSnap.objects || [];
    for (const obj of objects) {
      const p = this.worldToScreen(obj.x, obj.y);
      const rect = this.canvas.getBoundingClientRect();
      if (p.x < -30 || p.y < -30
        || p.x > rect.width + 30 || p.y > rect.height + 30) {
        continue;
      }
      if (showDest && obj.moving) {
        const d = this.worldToScreen(obj.destX, obj.destY);
        ctx.strokeStyle = "rgba(240, 160, 58, 0.35)";
        ctx.setLineDash([4, 3]);
        ctx.beginPath();
        ctx.moveTo(p.x, p.y);
        ctx.lineTo(d.x, d.y);
        ctx.stroke();
        ctx.setLineDash([]);
      }
      const color = objectColor(obj);
      if (obj.kind === "item") {
        drawDiamond(ctx, p.x, p.y, 4, color);
      } else {
        drawArrow(ctx, p.x, p.y, obj.heading, 6, color, obj.inCombat);
      }
      if (showLabels && obj.name) {
        ctx.fillStyle = "rgba(201, 205, 214, 0.75)";
        ctx.font = "10px sans-serif";
        ctx.textAlign = "center";
        ctx.fillText(obj.name, p.x, p.y - 9);
        ctx.textAlign = "left";
      }
    }
  },

  drawSelf(ctx) {
    const c = this.lastSnap.character;
    const p = this.worldToScreen(c.x, c.y);
    const angle = (c.heading / 65536) * 2 * Math.PI;

    // View cone of 90 degrees like the L2Bot map.
    const rect = this.canvas.getBoundingClientRect();
    const radius = Math.min(rect.width, rect.height) * 0.4;
    ctx.fillStyle = "rgba(63, 185, 80, 0.09)";
    ctx.beginPath();
    ctx.moveTo(p.x, p.y);
    ctx.arc(p.x, p.y, radius, angle - Math.PI / 4, angle + Math.PI / 4);
    ctx.closePath();
    ctx.fill();

    // Pulsing ring around the self position.
    const ring = 10 + 2 * Math.sin(Date.now() / 400);
    ctx.strokeStyle = "rgba(63, 185, 80, 0.5)";
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(p.x, p.y, ring, 0, 2 * Math.PI);
    ctx.stroke();

    drawArrow(ctx, p.x, p.y, c.heading, 10, "#3fb950", false);
    ctx.fillStyle = "#eef1f5";
    ctx.font = "600 11px sans-serif";
    ctx.textAlign = "center";
    ctx.fillText(c.name || "self", p.x, p.y - 14);
    ctx.textAlign = "left";
  },

  updateMapInfo() {
    const rect = this.canvas.getBoundingClientRect();
    const across = Math.round(rect.width / this.scale);
    document.getElementById("map-scale").textContent =
      "≈ " + across.toLocaleString("en-US") + " units across";
    const objects = this.lastSnap.objects || [];
    const counts = { npc: 0, player: 0, item: 0 };
    for (const obj of objects) {
      if (counts[obj.kind] !== undefined) { counts[obj.kind]++; }
    }
    document.getElementById("map-objects").textContent =
      counts.npc + " npc · " + counts.player + " players · "
      + counts.item + " items";
  }
};

// Direction arrow rotated by the heading (0 = east, clockwise).
function drawArrow(ctx, x, y, heading, size, color, inCombat) {
  const angle = (heading / 65536) * 2 * Math.PI;
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(angle);
  ctx.beginPath();
  ctx.moveTo(size, 0);
  ctx.lineTo(-size * 0.7, -size * 0.55);
  ctx.lineTo(-size * 0.35, 0);
  ctx.lineTo(-size * 0.7, size * 0.55);
  ctx.closePath();
  ctx.fillStyle = color;
  ctx.fill();
  ctx.restore();
  if (inCombat) {
    ctx.strokeStyle = "#f0a03a";
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(x, y, size + 3, 0, 2 * Math.PI);
    ctx.stroke();
  }
}

function drawDiamond(ctx, x, y, size, color) {
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(Math.PI / 4);
  ctx.fillStyle = color;
  ctx.fillRect(-size, -size, size * 2, size * 2);
  ctx.restore();
}

function objectColor(obj) {
  if (obj.kind === "player") { return "#58a6ff"; }
  if (obj.kind === "item") { return "#d29922"; }
  if (obj.attackable) { return "#e5484d"; }

  return "#8b949e";
}

function cardinalOf(heading) {
  const deg = Math.round((heading / 65536) * 360);
  const names = ["E", "SE", "S", "SW", "W", "NW", "N", "NE"];

  return names[Math.round(deg / 45) % 8];
}
