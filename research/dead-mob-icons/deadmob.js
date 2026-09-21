/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// The dead mob icon research page of issue #6: the live map draws a
// corpse as a washed out gray circle with a look direction tick (the
// dead threat color at 0.45 alpha - drawUnitTick of map.js with
// threatOf() "dead"), which reads as "a faded alive mob" instead of
// "a kill that happened here". This page shows the same hunting scene
// - the character, seven killed mobs, five alive mobs, two ground
// items, three fleet kill skulls - and swaps ONLY the corpse icon
// through 31 numbered styles: 0 is the current live map style (the
// baseline to beat), 1..30 are the new ideas grouped in seven
// families (skulls, crosses, graves, ghosts, ground stains, fallen
// figures, symbols).
//
// Everything the scene paints copies the real visual language of
// internal/swarm/webserver/web/map.js: the exact palette constants,
// the drawUnitTick marker geometry, the label halo, the fleet kill
// skull (traceKillSkull), the world grid, the active hunting cell
// hexagon and the dead-first draw order. A chosen variant can be
// ported into drawObjects of map.js almost line for line.
//
// The page is a standalone research artifact: no build step, no npm,
// opened straight from disk. The world tile background (the 21_19
// keltir meadow tile the fight gallery uses too) loads through a
// candidate path list and falls back to a procedural meadow when the
// tile is unreachable, so the page always renders.
"use strict";

// ---- the palette, mirrored from map.js (mapColors + kill marks) ----
const DM_COLORS = {
  self: "#1a73e8",
  player: "#9334e6",
  item: "#f9ab00",
  friendly: "#5f6368",
  passive: "#188038",
  aggressive: "#e37400",
  combat: "#d93025",
  dead: "#80868b",
  tick: "#39424e",
  killMark: "#e37400",
  killMarkDetail: "#40230a",
  labelDead: "#9aa0a6",
  labelHalo: "rgba(15, 18, 22, 0.7)",
  grid: "rgba(128, 134, 139, 0.22)",
  gridText: "rgba(128, 134, 139, 0.65)",
  cell: "#f9ab00"
};

// The world tile background candidates, tried in order: the web
// content root (served next to the shipped web dir), the repository
// root server or the plain file:// open of this directory. The last
// resort is the procedural meadow, so the page never depends on the
// tile being reachable.
const DM_TILE_CANDIDATES = [
  "maps/0/21_19.jpg",
  "../../internal/swarm/webserver/web/maps/0/21_19.jpg",
  "/internal/swarm/webserver/web/maps/0/21_19.jpg",
  "internal/swarm/webserver/web/maps/0/21_19.jpg"
];
// The 21_19 tile: 32768 world units at 1024 source pixels; the zone
// center 46112/41500 sits at tile pixel 417/273 (same crop the fight
// gallery uses, the real keltir meadow the bot hunts).
const DM_TILE_UNITS = 32768;
const DM_TILE_PX = 1024;
const DM_TILE_ORIGIN = { x: 21 * DM_TILE_UNITS, y: 19 * DM_TILE_UNITS };
const DM_TILE_ANCHOR_PX = { x: 417, y: 273 };

// ---- the scene: world coordinates, the same every frame ----
// The character stands where the real hunt runs (the zone center);
// heading is the L2 0..65535 heading (0 = east, the angle convention
// of drawUnitTick: angle = heading / 65536 * 2pi).
const SCENE = {
  character: { x: 46112, y: 41500, heading: 15200, name: "testbot" },
  // The own target of the bot: a3 (the red target link + ring).
  targetId: 3003,
  corpses: [
    // The fresh kill at melee distance, right beside the loot drop
    // and under a fresh fleet kill skull (the overlap case the zone
    // hover tests pin: "the corpse sits exactly on its own kill
    // skull").
    { id: 2001, name: "Keltir", level: 3, x: 46280, y: 41470, heading: 12000 },
    // The kill cluster: two corpses overlapping.
    { id: 2002, name: "Keltir", level: 2, x: 45980, y: 41180, heading: 40000 },
    { id: 2003, name: "Keltir", level: 2, x: 46080, y: 41230, heading: 30000 },
    // The previous kill under the feet of the mob in combat now.
    { id: 2004, name: "Elder Keltir", level: 4, x: 46390, y: 41760, heading: 50000 },
    // The scattered rest: isolated kills of the last minutes.
    { id: 2005, name: "Keltir", level: 3, x: 46620, y: 40890, heading: 20000 },
    { id: 2006, name: "Keltir", level: 2, x: 45520, y: 42120, heading: 9000 },
    { id: 2007, name: "Elder Keltir", level: 4, x: 46900, y: 41620, heading: 33000 }
  ],
  alive: [
    // An aggressive mob east of the character, out of combat.
    { id: 3001, name: "Elder Keltir", level: 4, x: 46450, y: 41400, heading: 22000, threat: "aggressive" },
    // Two passive keltirs grazing the meadow north west.
    { id: 3002, name: "Keltir", level: 2, x: 45780, y: 41220, heading: 35000, threat: "passive" },
    { id: 3005, name: "Keltir", level: 2, x: 45680, y: 41590, heading: 10000, threat: "passive" },
    // The mob the bot fights right now: combat red, attacking the
    // character (the dashed "attacking me" ring of drawUnitTick).
    { id: 3003, name: "Keltir", level: 3, x: 46220, y: 41880, heading: 30000, threat: "combat", attackingMe: true },
    // A friendly guard far south east.
    { id: 3004, name: "Guard", level: 10, x: 46760, y: 42050, heading: 14000, threat: "friendly" },
    // Another bot of the fleet walking the meadow.
    { id: 4001, name: "test2", level: 6, x: 45530, y: 40980, heading: 8000, threat: "player" }
  ],
  items: [
    // The loot of the fresh kill and of the cluster.
    { x: 46260, y: 41440 },
    { x: 46040, y: 41260 }
  ],
  // The fleet wide kill skull ring (/api/fleet/kills): three ages so
  // the candidate icon is judged next to a fresh, a middle and a
  // nearly melted skull - the corpse marker and the kill skull share
  // the ground on the live map.
  killMarks: [
    { x: 46280, y: 41470, ageMs: 6000 },
    { x: 46080, y: 41230, ageMs: 100000 },
    { x: 46620, y: 40890, ageMs: 240000 }
  ],
  // The active hunting cell: the pointy-top hexagon the bot holds,
  // gold like drawActiveCell of map.js.
  cell: { cx: 46112, cy: 41450, radius: 650, focus: { x: 46330, y: 41390 },
    label: "keltir meadow", state: "fight", band: "lv 1-6" }
};

// ---- shared drawing helpers (the map.js geometry, verbatim logic) ----

// headingOf converts the L2 heading (0..65535) to the canvas angle.
function dmAngle(heading) {
  return (heading / 65536) * 2 * Math.PI;
}

// dmDrawUnitTick is drawUnitTick of map.js: a circle marker with the
// look direction tick, the combat pulse ring, the attacking-me dashed
// ring and the alpha fade of the dead.
function dmDrawUnitTick(ctx, x, y, heading, radius, fill, tick, opts) {
  const k = opts.scale || 1;
  const angle = dmAngle(heading);
  const alpha = opts.dead ? 0.45 : 1;

  if (opts.combat || opts.self) {
    const pulse = opts.self
      ? 3 * k : 2.5 * Math.sin(opts.pulse / 220) + 3 * k;
    ctx.strokeStyle = fill;
    ctx.globalAlpha = 0.35;
    ctx.lineWidth = Math.max(1, 1.5 * k);
    ctx.beginPath();
    ctx.arc(x, y, radius + pulse, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  ctx.globalAlpha = alpha;
  ctx.beginPath();
  ctx.arc(x, y, radius, 0, Math.PI * 2);
  ctx.fillStyle = fill;
  ctx.fill();
  ctx.lineWidth = Math.max(0.75, 1.25 * k);
  ctx.strokeStyle = tick;
  ctx.stroke();

  if (opts.attackingMe) {
    ctx.lineWidth = Math.max(1, 1.5 * k);
    ctx.strokeStyle = tick;
    ctx.setLineDash([3 * k, 2 * k]);
    ctx.beginPath();
    ctx.arc(x, y, radius + 5 * k, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
  }

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

// dmTraceSkull is traceKillSkull of map.js: the stylized skull traced
// from primitives (the cranium circle over the jaw circle, the eye
// dots, the mouth slots), two passes the caller fills.
function dmTraceSkull(ctx, x, y, r, face) {
  if (!face) {
    ctx.moveTo(x + r * 0.75, y - r * 0.15);
    ctx.arc(x, y - r * 0.15, r * 0.75, 0, Math.PI * 2);
    ctx.moveTo(x + r * 0.45, y + r * 0.45);
    ctx.arc(x, y + r * 0.45, r * 0.45, 0, Math.PI * 2);

    return;
  }
  ctx.moveTo(x - r * 0.13, y - r * 0.3);
  ctx.arc(x - r * 0.33, y - r * 0.3, r * 0.22, 0, Math.PI * 2);
  ctx.moveTo(x + r * 0.53, y - r * 0.3);
  ctx.arc(x + r * 0.33, y - r * 0.3, r * 0.22, 0, Math.PI * 2);
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

// dmSkull paints the full skull in one call: the body pass in the
// body color, the face pass in the detail color (the same two-batch
// fill order drawKillMarks uses).
function dmSkull(ctx, x, y, r, body, detail, outline) {
  ctx.beginPath();
  dmTraceSkull(ctx, x, y, r, false);
  ctx.fillStyle = body;
  ctx.fill();
  if (outline) {
    ctx.lineWidth = Math.max(0.75, r * 0.09);
    ctx.strokeStyle = outline;
    ctx.stroke();
  }
  ctx.beginPath();
  dmTraceSkull(ctx, x, y, r, true);
  ctx.fillStyle = detail;
  ctx.fill();
}

// dmDiamond is drawDiamond of map.js (the ground item drop).
function dmDiamond(ctx, x, y, size, color) {
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate(Math.PI / 4);
  ctx.fillStyle = color;
  ctx.fillRect(-size, -size, size * 2, size * 2);
  ctx.restore();
}

// dmLabel paints a unit label with the dark halo of drawLabels.
function dmLabel(ctx, x, y, text, color, font) {
  ctx.font = font;
  ctx.textAlign = "center";
  ctx.lineWidth = 3;
  ctx.strokeStyle = DM_COLORS.labelHalo;
  ctx.strokeText(text, x, y);
  ctx.fillStyle = color;
  ctx.fillText(text, x, y);
  ctx.textAlign = "left";
}

// dmX strokes a bold two leg cross centered on (x, y).
function dmX(ctx, x, y, leg, width, color, alpha) {
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.strokeStyle = color;
  ctx.lineWidth = width;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(x - leg, y - leg);
  ctx.lineTo(x + leg, y + leg);
  ctx.moveTo(x + leg, y - leg);
  ctx.lineTo(x - leg, y + leg);
  ctx.stroke();
  ctx.restore();
}

// ---- the 31 variants ----
// Every entry: id (0 = the live map baseline), name, family, a one
// line note for the card, and draw(ctx, x, y, r, k, obj, t) where
// (x, y) is the corpse position on screen, r the corpse marker
// radius (4 * unitScale, the dead radiusOf of map.js), k the unit
// scale, obj the corpse record (name, level, heading) and t the
// animation time (most variants ignore it; the few living ones - the
// ghost, the dissolve, the ash, the wisp - breathe with it).
const VARIANTS = [
  {
    id: 0,
    name: "current",
    family: "baseline",
    note: "the live map style today: a dead-threat circle with the look tick at 0.45 alpha - reads as a faded alive mob, the style this issue wants replaced.",
    draw(ctx, x, y, r, k, obj, t) {
      dmDrawUnitTick(ctx, x, y, obj.heading, r,
        DM_COLORS.dead, DM_COLORS.tick,
        { dead: true, scale: k, pulse: t });
    }
  },

  // ---- the skull family: the universal death glyph ----
  {
    id: 1,
    name: "skull-gray",
    family: "skulls",
    note: "the fleet kill skull geometry in the corpse gray: one death glyph everywhere, but the corpse stays neutral gray so the live kill marks keep their orange urgency.",
    draw(ctx, x, y, r, k) {
      dmSkull(ctx, x, y, r * 1.05, DM_COLORS.dead, DM_COLORS.tick);
    }
  },
  {
    id: 2,
    name: "skull-orange",
    family: "skulls",
    note: "the corpse joins the kill mark family: the exact orange skull a fleet kill draws, so a corpse and its skull become one mark instead of two overlapping ones.",
    draw(ctx, x, y, r, k) {
      dmSkull(ctx, x, y, r * 1.05, DM_COLORS.killMark, DM_COLORS.killMarkDetail);
    }
  },
  {
    id: 3,
    name: "skull-bone",
    family: "skulls",
    note: "an ivory skull with the dark outline: the highest contrast glyph, bones white on any terrain - loud but unmissable zoomed out.",
    draw(ctx, x, y, r, k) {
      dmSkull(ctx, x, y, r * 1.05, "#f1f3f4", "#202124", DM_COLORS.tick);
    }
  },
  {
    id: 4,
    name: "skull-in-ring",
    family: "skulls",
    note: "a hybrid: the corpse keeps the current circle footprint (the muscle memory of the eye) and a small skull explains it from inside.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.35;
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(1, 1.25 * k);
      ctx.beginPath();
      ctx.arc(x, y, r * 1.15, 0, Math.PI * 2);
      ctx.stroke();
      ctx.restore();
      dmSkull(ctx, x, y, r * 0.72, DM_COLORS.dead, DM_COLORS.tick);
    }
  },
  {
    id: 5,
    name: "skull-bones",
    family: "skulls",
    note: "the jolly roger: the ivory skull over two crossed slate bones - the most literal 'dead' of the family, a bit busy at the corpse size.",
    draw(ctx, x, y, r, k) {
      const bone = r * 1.05;
      ctx.save();
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.5 * k);
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.moveTo(x - bone, y - bone * 0.55);
      ctx.lineTo(x + bone, y + bone * 0.55);
      ctx.moveTo(x + bone, y - bone * 0.55);
      ctx.lineTo(x - bone, y + bone * 0.55);
      ctx.stroke();
      ctx.restore();
      dmSkull(ctx, x, y - r * 0.05, r * 0.8, "#f1f3f4", "#202124", DM_COLORS.tick);
    }
  },

  // ---- the cross family: the minimal kill mark ----
  {
    id: 6,
    name: "x-bold",
    family: "crosses",
    note: "a bold slate X: the smallest change that still says 'done' - no circle, no fill, just the mark on the terrain.",
    draw(ctx, x, y, r, k) {
      dmX(ctx, x, y, r * 0.72, Math.max(1.4, 1.7 * k), DM_COLORS.tick, 0.85);
    }
  },
  {
    id: 7,
    name: "x-in-circle",
    family: "crosses",
    note: "the X inside a hollow circle: keeps the mob footprint, swaps the direction tick for the verdict - the 'closed' road sign reading.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.55;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.25 * k);
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.stroke();
      ctx.restore();
      dmX(ctx, x, y, r * 0.5, Math.max(1, 1.5 * k), DM_COLORS.tick, 0.8);
    }
  },
  {
    id: 8,
    name: "x-soft",
    family: "crosses",
    note: "the X in the corpse gray at half alpha: the quietest of the crosses - the mob fades like today but the mark explains why.",
    draw(ctx, x, y, r, k) {
      dmX(ctx, x, y, r * 0.8, Math.max(1, 1.4 * k), DM_COLORS.dead, 0.5);
    }
  },
  {
    id: 9,
    name: "cross-tilt",
    family: "crosses",
    note: "a small dried-blood cross tilted like a battlefield grave marker - warm tone, distinct from every threat color of the alive mobs.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.translate(x, y);
      ctx.rotate(-0.35);
      ctx.globalAlpha = 0.85;
      ctx.strokeStyle = "#8c4632";
      ctx.lineWidth = Math.max(1, 1.5 * k);
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.moveTo(0, -r * 0.85);
      ctx.lineTo(0, r * 0.85);
      ctx.moveTo(-r * 0.4, -r * 0.3);
      ctx.lineTo(r * 0.4, -r * 0.3);
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 10,
    name: "x-double",
    family: "crosses",
    note: "two small X marks side by side - the density of a farming spot shows as a field of little kills, each mark half the size of a single X.",
    draw(ctx, x, y, r, k) {
      const w = Math.max(1, 1.2 * k);
      dmX(ctx, x - r * 0.55, y, r * 0.42, w, DM_COLORS.tick, 0.8);
      dmX(ctx, x + r * 0.55, y, r * 0.42, w, DM_COLORS.tick, 0.8);
    }
  },

  // ---- the grave family: the burial metaphor ----
  {
    id: 11,
    name: "tombstone",
    family: "graves",
    note: "a small slate slab with the rounded top: reads 'buried here' at a glance - heavier than a glyph but unmistakable.",
    draw(ctx, x, y, r, k) {
      dmTombstone(ctx, x, y, r * 1.05, r * 1.25, 0, "#9aa0a6", 0.9);
      dmGroundLine(ctx, x, y + r * 0.08, r * 1.3, 0.2);
    }
  },
  {
    id: 12,
    name: "tombstone-cross",
    family: "graves",
    note: "the tombstone with the engraved cross: the richest of the graves - the engraving carries the meaning when the slab is small.",
    draw(ctx, x, y, r, k) {
      dmTombstone(ctx, x, y, r * 1.05, r * 1.25, 0, "#9aa0a6", 0.9);
      ctx.save();
      ctx.globalAlpha = 0.8;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(0.8, 0.7 * k);
      ctx.beginPath();
      ctx.moveTo(x, y - r * 0.85);
      ctx.lineTo(x, y - r * 0.25);
      ctx.moveTo(x - r * 0.22, y - r * 0.62);
      ctx.lineTo(x + r * 0.22, y - r * 0.62);
      ctx.stroke();
      ctx.restore();
      dmGroundLine(ctx, x, y + r * 0.08, r * 1.3, 0.2);
    }
  },
  {
    id: 13,
    name: "tombstone-tilt",
    family: "graves",
    note: "the fallen-over slab, tilted: the mob died mid stride, the marker tumbled - a playful read, memorable among the upright ones.",
    draw(ctx, x, y, r, k) {
      dmTombstone(ctx, x, y, r * 1.0, r * 1.15, -0.38, "#9aa0a6", 0.85);
      dmGroundLine(ctx, x, y + r * 0.12, r * 1.25, 0.2);
    }
  },
  {
    id: 14,
    name: "grave-mound",
    family: "graves",
    note: "the fresh grave: a dark mound of ground with the upright cross - the earth itself tells the story, nothing stands above the terrain.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.45;
      ctx.fillStyle = DM_COLORS.tick;
      ctx.beginPath();
      ctx.ellipse(x, y + r * 0.3, r * 0.75, r * 0.26, 0, Math.PI, 0);
      ctx.fill();
      ctx.restore();
      ctx.save();
      ctx.globalAlpha = 0.85;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.3 * k);
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.moveTo(x, y - r * 0.9);
      ctx.lineTo(x, y + r * 0.05);
      ctx.moveTo(x - r * 0.3, y - r * 0.55);
      ctx.lineTo(x + r * 0.3, y - r * 0.55);
      ctx.stroke();
      ctx.restore();
    }
  },

  // ---- the ghost family: the corpse as an absence ----
  {
    id: 15,
    name: "ghost-circle",
    family: "ghosts",
    note: "the current marker with the fill removed: a hollow outline where the mob stood - the minimal 'it is not there anymore' read.",
    draw(ctx, x, y, r, k, obj, t) {
      ctx.save();
      ctx.globalAlpha = 0.4;
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(1, 1.25 * k);
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.stroke();
      ctx.globalAlpha = 0.12;
      const angle = dmAngle(obj.heading);
      ctx.beginPath();
      ctx.moveTo(x + Math.cos(angle) * r, y + Math.sin(angle) * r);
      ctx.lineTo(x + Math.cos(angle) * (r + 4.5 * k),
        y + Math.sin(angle) * (r + 4.5 * k));
      ctx.lineWidth = Math.max(1, 2 * k);
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 16,
    name: "ghost-blob",
    family: "ghosts",
    note: "the little ghost hovering over the spot: the soul of the keltir, gently bobbing - cartoon warm, the friendliest read of death.",
    draw(ctx, x, y, r, k, obj, t) {
      const bob = Math.sin(t / 900) * r * 0.1;
      const gy = y - r * 0.2 + bob;
      ctx.save();
      ctx.globalAlpha = 0.55;
      ctx.fillStyle = "#9aa0a6";
      ctx.beginPath();
      ctx.moveTo(x - r * 0.75, gy + r * 0.55);
      ctx.lineTo(x - r * 0.75, gy - r * 0.1);
      ctx.arc(x, gy - r * 0.1, r * 0.75, Math.PI, 0);
      ctx.lineTo(x + r * 0.75, gy + r * 0.55);
      for (let i = 0; i < 3; i++) {
        const sx = x + r * 0.75 - (i + 0.5) * r * 0.5;
        ctx.quadraticCurveTo(sx + r * 0.25, gy + r * 0.3, sx, gy + r * 0.55);
      }
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = 0.9;
      ctx.fillStyle = DM_COLORS.tick;
      ctx.beginPath();
      ctx.arc(x - r * 0.28, gy - r * 0.15, r * 0.1, 0, Math.PI * 2);
      ctx.arc(x + r * 0.28, gy - r * 0.15, r * 0.1, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  },
  {
    id: 17,
    name: "dissolve",
    family: "ghosts",
    note: "the corpse evaporating: the circle breaking into dots that drift out and fade - an animated read, the kill literally dissolves.",
    draw(ctx, x, y, r, k, obj, t) {
      ctx.save();
      ctx.globalAlpha = 0.15;
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(1, 1.25 * k);
      ctx.beginPath();
      ctx.arc(x, y, r * 0.8, 0, Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = DM_COLORS.dead;
      for (let i = 0; i < 8; i++) {
        const phase = ((t / 1600) + i / 8) % 1;
        const ang = (i / 8) * 2 * Math.PI + 0.3;
        const dist = r * (0.55 + 0.75 * phase);
        ctx.globalAlpha = 0.55 * (1 - phase);
        ctx.beginPath();
        ctx.arc(x + Math.cos(ang) * dist, y + Math.sin(ang) * dist,
          Math.max(0.8, r * 0.14), 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.globalAlpha = 0.3;
      ctx.beginPath();
      ctx.arc(x, y, Math.max(0.8, r * 0.2), 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  },
  {
    id: 18,
    name: "dashed-ring",
    family: "ghosts",
    note: "a dashed outline circle: the mob turned into a placeholder - light, quiet, reads as 'dropped from the world' rather than 'killed'.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.55;
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(1, 1.4 * k);
      ctx.setLineDash([1.5 * k, 1.2 * k]);
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.restore();
    }
  },
  {
    id: 19,
    name: "smudge",
    family: "ghosts",
    note: "a soft radial stain: no shape at all, just a gray shadow on the grass where something stopped moving - the quietest option.",
    draw(ctx, x, y, r, k) {
      const grad = ctx.createRadialGradient(x, y, r * 0.1, x, y, r * 1.6);
      grad.addColorStop(0, "rgba(128, 134, 139, 0.35)");
      grad.addColorStop(1, "rgba(128, 134, 139, 0)");
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.arc(x, y, r * 1.6, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 0.3;
      ctx.fillStyle = DM_COLORS.dead;
      ctx.beginPath();
      ctx.arc(x, y, Math.max(0.8, r * 0.25), 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;
    }
  },

  // ---- the ground family: the corpse as a stain on the terrain ----
  {
    id: 20,
    name: "splat-dark",
    family: "ground",
    note: "a dark ink splat on the ground: the kill left a mark on the terrain itself - organic, no symbol, reads instantly as 'something ended here'.",
    draw(ctx, x, y, r, k) {
      dmSplat(ctx, x, y, r, DM_COLORS.tick, 0.5);
    }
  },
  {
    id: 21,
    name: "splat-blood",
    family: "ground",
    note: "the dried blood splat with droplets: the visceral option - the dark red stays away from the combat threat color so it never reads as danger.",
    draw(ctx, x, y, r, k) {
      dmSplat(ctx, x, y, r, "#7a2e22", 0.55);
      ctx.save();
      ctx.fillStyle = "#7a2e22";
      ctx.globalAlpha = 0.55;
      const drops = [[1.2, -0.35], [-1.05, 0.55], [0.35, 1.15]];
      for (const d of drops) {
        ctx.beginPath();
        ctx.arc(x + d[0] * r, y + d[1] * r,
          Math.max(0.7, r * 0.12), 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
    }
  },
  {
    id: 22,
    name: "ash-pile",
    family: "ground",
    note: "the ash pile with rising smoke wisps: the mob burned down to ash - animated gently, the wisps climb and fade on a loop.",
    draw(ctx, x, y, r, k, obj, t) {
      ctx.save();
      ctx.globalAlpha = 0.65;
      ctx.fillStyle = DM_COLORS.dead;
      ctx.beginPath();
      ctx.ellipse(x, y + r * 0.25, r * 0.95, r * 0.4, 0, Math.PI, 0);
      ctx.fill();
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(0.8, 0.7 * k);
      ctx.beginPath();
      ctx.ellipse(x, y + r * 0.25, r * 0.95, r * 0.4, 0, Math.PI, 0);
      ctx.stroke();
      for (let i = 0; i < 2; i++) {
        const phase = ((t / 2400) + i * 0.5) % 1;
        const rise = phase * r * 1.3;
        ctx.globalAlpha = 0.35 * (1 - phase);
        ctx.lineWidth = Math.max(0.8, k * 0.8);
        ctx.beginPath();
        ctx.moveTo(x - r * 0.2 + i * r * 0.4, y);
        ctx.quadraticCurveTo(
          x - r * 0.5 + i * r * 0.9, y - rise * 0.6,
          x - r * 0.1 + i * r * 0.55, y - rise);
        ctx.stroke();
      }
      ctx.restore();
    }
  },
  {
    id: 23,
    name: "puddle",
    family: "ground",
    note: "a flat dark puddle: the mob melted into the ground - flat and wide, never collides with the labels above the spot.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.4;
      ctx.fillStyle = DM_COLORS.tick;
      ctx.beginPath();
      ctx.ellipse(x, y, r * 1.35, r * 0.42, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 0.5;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    }
  },

  // ---- the fallen family: the mob itself toppled ----
  {
    id: 24,
    name: "capsule",
    family: "fallen",
    note: "the mob lying flat: the circle becomes a rounded capsule bar on the ground, a body seen from above - no death symbol at all, pure posture.",
    draw(ctx, x, y, r, k, obj) {
      const angle = dmAngle(obj.heading);
      ctx.save();
      ctx.globalAlpha = 0.6;
      ctx.strokeStyle = DM_COLORS.dead;
      ctx.lineWidth = Math.max(1.6, 2.2 * k);
      ctx.lineCap = "round";
      const len = r * 0.8;
      ctx.beginPath();
      ctx.moveTo(x - Math.cos(angle) * len, y - Math.sin(angle) * len);
      ctx.lineTo(x + Math.cos(angle) * len, y + Math.sin(angle) * len);
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 25,
    name: "fallen-figure",
    family: "fallen",
    note: "a tiny stick figure lying down: head, body, sprawled limbs - the most literal 'dead body' read; honest test of whether detail survives the size.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.75;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.2 * k);
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.arc(x - r * 0.55, y, r * 0.3, 0, Math.PI * 2);
      ctx.moveTo(x - r * 0.25, y);
      ctx.lineTo(x + r * 0.55, y + r * 0.08);
      ctx.moveTo(x + r * 0.1, y);
      ctx.lineTo(x + r * 0.55, y - r * 0.45);
      ctx.moveTo(x + r * 0.05, y + r * 0.04);
      ctx.lineTo(x + r * 0.5, y + r * 0.5);
      ctx.moveTo(x - r * 0.35, y - r * 0.05);
      ctx.lineTo(x - r * 0.05, y - r * 0.4);
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 26,
    name: "x-eyes",
    family: "fallen",
    note: "the current gray circle with cartoon X eyes: the comic death - the footprint of today, the meaning arrives with two tiny marks.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.55;
      ctx.fillStyle = DM_COLORS.dead;
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fill();
      ctx.lineWidth = Math.max(0.75, 1.25 * k);
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.stroke();
      ctx.restore();
      const eye = Math.max(0.7, r * 0.16);
      dmX(ctx, x - r * 0.32, y - r * 0.12, eye, Math.max(0.7, 0.7 * k),
        DM_COLORS.tick, 0.95);
      dmX(ctx, x + r * 0.32, y - r * 0.12, eye, Math.max(0.7, 0.7 * k),
        DM_COLORS.tick, 0.95);
    }
  },

  // ---- the symbol family: signs and metaphors ----
  {
    id: 27,
    name: "no-symbol",
    family: "symbols",
    note: "the prohibition sign: circle and slash - means 'out of play' rather than 'dead'; the cleanest geometry of the sign family.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.6;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.6 * k);
      ctx.beginPath();
      ctx.arc(x, y, r * 0.95, 0, Math.PI * 2);
      ctx.moveTo(x - r * 0.67, y - r * 0.67);
      ctx.lineTo(x + r * 0.67, y + r * 0.67);
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 28,
    name: "deflated",
    family: "symbols",
    note: "the marker deflated: the circle squashed to an ellipse with a drooping tick pointing down - the mob 'leaked out', melancholic and light.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.45;
      ctx.fillStyle = DM_COLORS.dead;
      ctx.beginPath();
      ctx.ellipse(x, y, r, r * 0.55, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.lineWidth = Math.max(0.75, 1.25 * k);
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x, y + r * 0.55);
      ctx.lineTo(x, y + r * 0.55 + 3.5 * k);
      ctx.lineWidth = Math.max(1, 1.5 * k);
      ctx.lineCap = "round";
      ctx.stroke();
      ctx.restore();
    }
  },
  {
    id: 29,
    name: "broken-swords",
    family: "symbols",
    note: "two crossed swords, both broken: the fight itself is over - the only variant that tells HOW the mob died, not just that it did.",
    draw(ctx, x, y, r, k) {
      ctx.save();
      ctx.globalAlpha = 0.85;
      ctx.strokeStyle = DM_COLORS.tick;
      ctx.lineWidth = Math.max(1, 1.4 * k);
      ctx.lineCap = "round";
      const blades = [[-1, -1, 1, 1], [1, -1, -1, 1]];
      for (const b of blades) {
        const x1 = x + b[0] * r * 0.95;
        const y1 = y + b[1] * r * 0.95;
        const x2 = x + b[2] * r * 0.28;
        const y2 = y + b[3] * r * 0.28;
        ctx.beginPath();
        ctx.moveTo(x1, y1);
        ctx.lineTo(x2, y2);
        ctx.stroke();
        // The broken-off tip, fallen aside of the blade line.
        ctx.beginPath();
        ctx.moveTo(x2 + b[2] * r * 0.1, y2 + b[3] * r * 0.1);
        ctx.lineTo(x2 + b[2] * r * 0.45 - b[1] * r * 0.14,
          y2 + b[3] * r * 0.45 - b[0] * r * 0.14);
        ctx.stroke();
        // The crossguard near the outer end.
        ctx.beginPath();
        ctx.moveTo(x1 - b[1] * r * 0.22, y1 - b[0] * r * 0.22);
        ctx.lineTo(x1 + b[1] * r * 0.22, y1 + b[0] * r * 0.22);
        ctx.stroke();
        // The red pommel dot.
        ctx.globalAlpha = 0.7;
        ctx.fillStyle = DM_COLORS.combat;
        ctx.beginPath();
        ctx.arc(x1, y1, Math.max(0.7, r * 0.1), 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 0.85;
      }
      ctx.restore();
    }
  },
  {
    id: 30,
    name: "soul-wisp",
    family: "symbols",
    note: "the soul leaving: a faint ground dot and an ivory wisp floating above it, slowly bobbing - the most poetic read; a kill becomes a rising spirit.",
    draw(ctx, x, y, r, k, obj, t) {
      ctx.save();
      ctx.globalAlpha = 0.25;
      ctx.fillStyle = DM_COLORS.dead;
      ctx.beginPath();
      ctx.ellipse(x, y + r * 0.2, r * 0.8, r * 0.25, 0, 0, Math.PI * 2);
      ctx.fill();
      const bob = Math.sin(t / 700) * r * 0.12;
      const wy = y - r * 1.15 + bob;
      ctx.globalAlpha = 0.8;
      ctx.fillStyle = "#f1f3f4";
      ctx.beginPath();
      ctx.moveTo(x - r * 0.35, wy + r * 0.45);
      ctx.arc(x, wy, r * 0.35, Math.PI, 0);
      ctx.quadraticCurveTo(x + r * 0.28, wy + r * 0.3, x, wy + r * 0.55);
      ctx.quadraticCurveTo(x - r * 0.28, wy + r * 0.3, x - r * 0.35, wy + r * 0.45);
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = 0.3;
      ctx.beginPath();
      ctx.arc(x, wy + r * 0.85, Math.max(0.7, r * 0.12), 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 0.15;
      ctx.beginPath();
      ctx.arc(x, wy + r * 1.15, Math.max(0.6, r * 0.09), 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  }
];

// dmTombstone paints the slab: a rounded top rectangle centered on
// (x, y) with the base sitting at y + h/2, rotated by rot when asked.
function dmTombstone(ctx, x, y, w, h, rot, fill, alpha) {
  ctx.save();
  ctx.translate(x, y);
  if (rot) { ctx.rotate(rot); }
  ctx.globalAlpha = alpha;
  ctx.fillStyle = fill;
  ctx.beginPath();
  ctx.moveTo(-w / 2, h / 2);
  ctx.lineTo(-w / 2, -h / 2 + w / 2);
  ctx.arc(0, -h / 2 + w / 2, w / 2, Math.PI, 0);
  ctx.lineTo(w / 2, h / 2);
  ctx.closePath();
  ctx.fill();
  ctx.lineWidth = 1;
  ctx.strokeStyle = DM_COLORS.tick;
  ctx.stroke();
  ctx.restore();
}

// dmGroundLine paints the little ground shadow line under a grave.
function dmGroundLine(ctx, x, y, half, alpha) {
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.strokeStyle = DM_COLORS.tick;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(x - half, y);
  ctx.lineTo(x + half, y);
  ctx.stroke();
  ctx.restore();
}

// dmSplat paints the fixed blob splat: five overlapping circles of
// the same pseudo random offsets for every corpse, one fill call.
function dmSplat(ctx, x, y, r, color, alpha) {
  const parts = [
    [-0.4, -0.1, 0.5], [0.2, -0.35, 0.45], [0.45, 0.25, 0.4],
    [-0.1, 0.3, 0.55], [-0.55, 0.3, 0.3]
  ];
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.fillStyle = color;
  ctx.beginPath();
  for (const p of parts) {
    ctx.moveTo(x + p[0] * r + p[2] * r, y + p[1] * r);
    ctx.arc(x + p[0] * r, y + p[1] * r, p[2] * r, 0, Math.PI * 2);
  }
  ctx.fill();
  ctx.restore();
}

// ---- the scene view ----
// The one object of the page: owns the canvas, the camera (the
// character anchored center, the zoom slider moves the scale), the
// loaded tile (or the procedural meadow) and the selected variant.
const DMView = {
  canvas: null,
  ctx: null,
  width: 960,
  height: 600,
  scale: 0.35,
  tile: null,
  variant: 0,
  showKills: true,
  showLabels: true,
  showCell: true,
  showGrid: true,

  // unitScale mirrors computeUnitScale of map.js (the clamp 0.3..1.6
  // included), so the marker sizes on this page are the marker sizes
  // of the live map at the same zoom.
  unitScale() {
    const factor = Math.pow(this.scale / 0.12, 0.6);

    return Math.max(0.3, Math.min(1.6, factor));
  },

  worldToScreen(wx, wy) {
    const c = SCENE.character;

    return {
      x: this.width / 2 + (wx - c.x) * this.scale,
      y: this.height / 2 + (wy - c.y) * this.scale
    };
  },

  // loadTile tries the candidate paths in order; the first image
  // that decodes becomes the background, otherwise the procedural
  // meadow paints instead (tile stays null).
  loadTile(done) {
    let index = 0;
    const tryNext = () => {
      if (index >= DM_TILE_CANDIDATES.length) {
        done(false);

        return;
      }
      const img = new Image();
      img.onload = () => {
        if (img.naturalWidth > 0) {
          this.tile = img;
          done(true);
        } else {
          index += 1;
          tryNext();
        }
      };
      img.onerror = () => {
        index += 1;
        tryNext();
      };
      img.src = DM_TILE_CANDIDATES[index];
    };
    tryNext();
  },

  // drawBackground paints the tile crop the viewport shows (the
  // character anchored world window), or the procedural meadow when
  // no tile loaded: a soft grass gradient with a few darker patches
  // and dots, so the contrast judgment survives the fallback. The
  // width and height arrive as parameters so the gallery cells can
  // borrow the same background without touching the scene view.
  drawBackground(ctx, w, h) {
    if (this.tile) {
      const pxPerUnit = DM_TILE_PX / DM_TILE_UNITS;
      const sx = DM_TILE_ANCHOR_PX.x - (w / 2) / this.scale * pxPerUnit;
      const sy = DM_TILE_ANCHOR_PX.y - (h / 2) / this.scale * pxPerUnit;
      const sw = w / this.scale * pxPerUnit;
      const sh = h / this.scale * pxPerUnit;
      ctx.drawImage(this.tile, sx, sy, sw, sh, 0, 0, w, h);

      return;
    }
    const grad = ctx.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, "#4a5d43");
    grad.addColorStop(0.5, "#57704f");
    grad.addColorStop(1, "#4f6849");
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, w, h);
    // The deterministic meadow patches: blobs of darker grass.
    ctx.fillStyle = "rgba(62, 80, 58, 0.5)";
    const patches = [
      [120, 90, 130, 60], [520, 140, 200, 80], [300, 320, 170, 90],
      [740, 380, 160, 70], [80, 480, 150, 80], [620, 520, 200, 90],
      [420, 560, 130, 60], [860, 120, 120, 60]
    ];
    for (const p of patches) {
      ctx.beginPath();
      ctx.ellipse(p[0] % w, p[1] % h, p[2], p[3], 0.6, 0, Math.PI * 2);
      ctx.fill();
    }
    // The grass dots: a coarse sprinkle on a fixed lattice.
    ctx.fillStyle = "rgba(103, 126, 95, 0.5)";
    for (let gx = 12; gx < w; gx += 34) {
      for (let gy = 9; gy < h; gy += 26) {
        const jitter = ((gx * 31 + gy * 17) % 13) - 6;
        ctx.fillRect(gx + jitter, gy + (jitter % 5), 2, 1);
      }
    }
  },

  // drawGrid mirrors drawGrid of map.js: the 250/500 world unit
  // ladder with the coordinate numbers.
  drawGrid(ctx) {
    if (!this.showGrid) { return; }
    let step = 500;
    while (step * this.scale < 36) { step *= 2; }
    while (step * this.scale > 160) { step /= 2; }
    const c = SCENE.character;
    const left = c.x - this.width / 2 / this.scale;
    const right = c.x + this.width / 2 / this.scale;
    const top = c.y - this.height / 2 / this.scale;
    const bottom = c.y + this.height / 2 / this.scale;
    ctx.lineWidth = 1;
    ctx.font = "10px monospace";
    ctx.strokeStyle = DM_COLORS.grid;
    ctx.fillStyle = DM_COLORS.gridText;
    for (let x = Math.floor(left / step) * step; x <= right; x += step) {
      const p = this.worldToScreen(x, 0);
      ctx.beginPath();
      ctx.moveTo(p.x, 0);
      ctx.lineTo(p.x, this.height);
      ctx.stroke();
      ctx.fillText(String(x), p.x + 3, 11);
    }
    for (let y = Math.floor(top / step) * step; y <= bottom; y += step) {
      const p = this.worldToScreen(0, y);
      ctx.beginPath();
      ctx.moveTo(0, p.y);
      ctx.lineTo(this.width, p.y);
      ctx.stroke();
      ctx.fillText(String(y), 3, p.y - 3);
    }
  },

  // drawCell mirrors drawActiveCell of map.js: the pointy top
  // hexagon the bot fights in, the gold fill, the stroke, the focus
  // dot and the label.
  drawCell(ctx) {
    if (!this.showCell) { return; }
    const cell = SCENE.cell;
    ctx.save();
    ctx.globalAlpha = 0.16;
    ctx.fillStyle = DM_COLORS.cell;
    ctx.beginPath();
    this.cellPath(ctx, cell);
    ctx.fill();
    ctx.globalAlpha = 0.95;
    ctx.strokeStyle = DM_COLORS.cell;
    ctx.lineWidth = 2.5;
    ctx.beginPath();
    this.cellPath(ctx, cell);
    ctx.stroke();
    const focus = this.worldToScreen(cell.focus.x, cell.focus.y);
    ctx.fillStyle = DM_COLORS.cell;
    ctx.beginPath();
    ctx.arc(focus.x, focus.y, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.font = "600 10px sans-serif";
    ctx.textAlign = "left";
    ctx.fillStyle = DM_COLORS.cell;
    ctx.fillText(cell.label + " - " + cell.state + " - " + cell.band,
      focus.x + 8, focus.y - 8);
    ctx.restore();
  },

  // cellPath traces the pointy top hexagon of the hunting cell.
  cellPath(ctx, cell) {
    for (let i = 0; i <= 6; i++) {
      const ang = -Math.PI / 2 + i * Math.PI / 3;
      const wx = cell.cx + Math.cos(ang) * cell.radius;
      const wy = cell.cy + Math.sin(ang) * cell.radius;
      const p = this.worldToScreen(wx, wy);
      if (i === 0) { ctx.moveTo(p.x, p.y); } else { ctx.lineTo(p.x, p.y); }
    }
  },

  // drawKillMarks mirrors drawKillMarks of map.js: the orange skull
  // melts with age through the same fade buckets.
  drawKillMarks(ctx) {
    if (!this.showKills) { return; }
    const buckets = 8;
    const ttl = 5 * 60 * 1000;
    for (const mark of SCENE.killMarks) {
      const age = mark.ageMs;
      if (age > ttl) { continue; }
      const bucket = Math.min(buckets - 1,
        Math.floor(age / ttl * buckets));
      const fade = bucket / buckets;
      const p = this.worldToScreen(mark.x, mark.y);
      // The fresh skull reads at 2.5x of the old cross size.
      const size = 11 * (0.7 + 0.3 * (1 - fade)) * this.unitScale() / 1.6;
      ctx.save();
      ctx.globalAlpha = 1 - fade * 0.75;
      ctx.beginPath();
      dmTraceSkull(ctx, p.x, p.y, size, false);
      ctx.fillStyle = DM_COLORS.killMark;
      ctx.fill();
      ctx.beginPath();
      dmTraceSkull(ctx, p.x, p.y, size, true);
      ctx.fillStyle = DM_COLORS.killMarkDetail;
      ctx.fill();
      ctx.restore();
    }
  },

  // drawTargetLinks mirrors drawTargetLinks: the own target gets the
  // red dashed line and ring (the L2Bot target marker language).
  drawTargetLinks(ctx) {
    const target = SCENE.alive.find((o) => o.id === SCENE.targetId);
    if (!target) { return; }
    const c = this.worldToScreen(SCENE.character.x, SCENE.character.y);
    const p = this.worldToScreen(target.x, target.y);
    const k = this.unitScale();
    ctx.save();
    ctx.strokeStyle = DM_COLORS.combat;
    ctx.setLineDash([5, 4]);
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(c.x, c.y);
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
    ctx.setLineDash([]);
    const r = this.radiusOf(target) * k;
    ctx.globalAlpha = 0.8;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(p.x, p.y, r + 3 * k, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  },

  // radiusOf mirrors radiusOf of map.js.
  radiusOf(obj) {
    if (obj.threat === "player") { return 5.5; }
    if (obj.threat === "combat") { return 6; }
    if (obj.threat === "dead") { return 4; }

    return 5;
  },

  // drawObjects is drawObjects of map.js with the corpse branch
  // swapped for the selected variant: the draw order stays dead
  // first, then north to south, the labels collect into the same
  // candidates list, the halo paint runs after every marker.
  drawObjects(ctx, t) {
    const k = this.unitScale();
    const variant = VARIANTS[this.variant];
    const objects = SCENE.corpses.map((o) => Object.assign(
      { threat: "dead" }, o)).concat(SCENE.alive.map(
      (o) => Object.assign({ moving: false }, o)));
    objects.sort((a, b) => ((a.threat === "dead" ? 0 : 1)
        - (b.threat === "dead" ? 0 : 1))
      || (a.y - b.y) || (a.id - b.id));
    const labels = [];
    for (const obj of objects) {
      const p = this.worldToScreen(obj.x, obj.y);
      if (p.x < -30 || p.y < -30
        || p.x > this.width + 30 || p.y > this.height + 30) {
        continue;
      }
      if (obj.threat === "dead") {
        // THE swap: everything else on the canvas stays fixed while
        // this one call changes with the selected variant.
        variant.draw(ctx, p.x, p.y, this.radiusOf(obj) * k, k, obj, t);
      } else {
        const r = this.radiusOf(obj) * k;
        dmDrawUnitTick(ctx, p.x, p.y, obj.heading, r,
          DM_COLORS[obj.threat], DM_COLORS.tick, {
            combat: obj.threat === "combat",
            attackingMe: obj.attackingMe || false,
            pulse: t,
            scale: k
          });
      }
      if (this.showLabels) {
        const suffix = obj.threat !== "player" && obj.level > 0
          ? " lv" + obj.level : "";
        const color = obj.threat === "dead" ? DM_COLORS.labelDead
          : "#ffffff";
        labels.push({
          x: p.x, y: p.y - this.radiusOf(obj) * k - 6,
          text: obj.name + suffix, color
        });
      }
    }
    // The ground items after the units (the diamonds of the drops).
    for (const item of SCENE.items) {
      const p = this.worldToScreen(item.x, item.y);
      dmDiamond(ctx, p.x, p.y, 4, DM_COLORS.item);
    }
    return labels;
  },

  // drawSelf mirrors drawSelf of map.js: the blue marker, the look
  // tick and the breathing accent ring.
  drawSelf(ctx, t) {
    const c = SCENE.character;
    const p = this.worldToScreen(c.x, c.y);
    const k = this.unitScale();
    const r = 6 * k;
    dmDrawUnitTick(ctx, p.x, p.y, c.heading, r,
      DM_COLORS.self, DM_COLORS.tick, { self: true, pulse: t, scale: k });
    const ring = r + 3 * k + 1.5 * Math.sin(t / 500);
    ctx.strokeStyle = DM_COLORS.self;
    ctx.globalAlpha = 0.5;
    ctx.lineWidth = Math.max(1, 1.25 * k);
    ctx.beginPath();
    ctx.arc(p.x, p.y, ring, 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;

    return { x: p.x, y: p.y - r - 6, text: c.name, color: "#ffffff" };
  },

  // paint runs the whole frame in the live map order: background,
  // grid, cell, kill marks, target links, objects, self, labels.
  paint() {
    const ctx = this.ctx;
    const t = performance.now();
    ctx.clearRect(0, 0, this.width, this.height);
    this.drawBackground(ctx, this.width, this.height);
    this.drawGrid(ctx);
    this.drawCell(ctx);
    this.drawKillMarks(ctx);
    this.drawTargetLinks(ctx);
    const labels = this.drawObjects(ctx, t);
    const selfLabel = this.drawSelf(ctx, t);
    if (this.showLabels) {
      labels.push(selfLabel);
      const font = "600 10.5px sans-serif";
      for (const lab of labels) {
        dmLabel(ctx, lab.x, lab.y, lab.text, lab.color, font);
      }
    }
  },

  // frame is the render loop: ~30 fps is plenty for the breathing
  // rings and the animated variants.
  frame() {
    if (DMView.lastPaint === undefined
      || performance.now() - DMView.lastPaint > 33) {
      DMView.lastPaint = performance.now();
      DMView.paint();
    }
    requestAnimationFrame(DMView.frame);
  },

  // resize adopts the css size of the canvas into the backing store
  // with the device pixel ratio, like resize() of map.js.
  resize() {
    const rect = this.canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const cssW = Math.min(960, rect.width || 960);
    const cssH = cssW * 600 / 960;
    this.canvas.style.width = cssW + "px";
    this.canvas.style.height = cssH + "px";
    this.width = cssW;
    this.height = cssH;
    this.canvas.width = Math.max(1, Math.floor(cssW * dpr));
    this.canvas.height = Math.max(1, Math.floor(cssH * dpr));
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  },

  init() {
    this.canvas = document.getElementById("scene");
    this.ctx = this.canvas.getContext("2d");
    this.resize();
    window.addEventListener("resize", () => {
      this.resize();
    });
    this.loadTile((ok) => {
      DMUI.tileLoaded(ok);
    });
    requestAnimationFrame(this.frame);
  }
};

// ---- the self test ----
// Runs headless friendly: every variant paints on a fresh 120x120
// canvas (no background, no tile - the pixel read stays legal) and
// the test counts the covered pixels of the glyph. A variant that
// throws or paints nothing fails loudly; the report lands in the
// #selftest block so a screenshot of the page carries it.
function dmRunSelfTest(report) {
  const results = [];
  for (const variant of VARIANTS) {
    const canvas = document.createElement("canvas");
    canvas.width = 120;
    canvas.height = 120;
    const ctx = canvas.getContext("2d");
    const obj = SCENE.corpses[0];
    let ok = true;
    let covered = 0;
    try {
      variant.draw(ctx, 60, 60, 8, 1.6, obj, 1000);
      const data = ctx.getImageData(0, 0, 120, 120).data;
      for (let i = 3; i < data.length; i += 4) {
        if (data[i] > 0) { covered++; }
      }
      if (covered < 12) { ok = false; }
    } catch (err) {
      ok = false;
      results.push([variant, false, "threw: " + err.message]);
      continue;
    }
    results.push([variant, ok, covered + " px covered"]);
  }
  const lines = [];
  let failures = 0;
  for (const [variant, ok, detail] of results) {
    if (!ok) { failures++; }
    lines.push((ok ? "ok   " : "FAIL ") + "#" + variant.id + " "
      + variant.name + " - " + detail);
  }
  lines.push(failures === 0
    ? "SELF TEST PASS: all " + VARIANTS.length + " variants draw"
    : "SELF TEST FAIL: " + failures + " variants broken");
  report(lines, failures);
}

// ---- the page UI ----
// DMUI owns everything outside the canvas: the variant button list
// (grouped by family), the info card, the gallery grid, the toggles,
// the zoom slider and the keyboard.
const DMUI = {
  // select changes the active variant and refreshes every chrome.
  select(id) {
    DMView.variant = id;
    const variant = VARIANTS[id];
    document.getElementById("card-num").textContent = id;
    document.getElementById("card-name").textContent = variant.name;
    document.getElementById("card-family").textContent = variant.family;
    document.getElementById("card-note").textContent = variant.note;
    document.getElementById("head-variant").textContent = id + " / 30";
    for (const btn of document.querySelectorAll(
      "#variant-list button, .gallery-cell")) {
      const active = Number(btn.dataset.variant) === id;
      btn.classList.toggle("active", active);
    }
    if (Number.isInteger(id)) {
      DMGallery.render(id);
    }
  },

  // tileLoaded notes the background source in the footer area: the
  // reviewer should know whether the real tile or the procedural
  // meadow is behind the markers. A late tile re-renders the gallery
  // cells that were built against the meadow fallback.
  tileLoaded(ok) {
    const head = document.querySelector("header .meta");
    if (head) {
      head.insertAdjacentHTML("beforeend",
        " &middot; background: " + (ok ? "21_19 map tile" : "procedural meadow"));
    }
    if (ok && document.getElementById("opt-gallery").checked) {
      DMGallery.build();
    }
  },

  // buildList renders the family grouped variant buttons.
  buildList() {
    const list = document.getElementById("variant-list");
    const families = [];
    for (const variant of VARIANTS) {
      let fam = families.find((f) => f.name === variant.family);
      if (!fam) {
        fam = { name: variant.family, variants: [] };
        families.push(fam);
      }
      fam.variants.push(variant);
    }
    for (const fam of families) {
      const block = document.createElement("div");
      block.className = "family-block";
      const label = document.createElement("div");
      label.className = "family-name";
      label.textContent = fam.name;
      const row = document.createElement("div");
      row.className = "variant-buttons";
      for (const variant of fam.variants) {
        const btn = document.createElement("button");
        btn.textContent = variant.id;
        btn.dataset.variant = variant.id;
        btn.title = variant.id + " " + variant.name + " - " + variant.note;
        if (variant.id === 0) { btn.className = "baseline"; }
        btn.addEventListener("click", () => {
          DMUI.select(variant.id);
        });
        row.appendChild(btn);
      }
      block.appendChild(label);
      block.appendChild(row);
      list.appendChild(block);
    }
  },

  // wire binds the toggles, the zoom and the keyboard.
  wire() {
    document.getElementById("opt-kills").addEventListener("change", (e) => {
      DMView.showKills = e.target.checked;
    });
    document.getElementById("opt-labels").addEventListener("change", (e) => {
      DMView.showLabels = e.target.checked;
    });
    document.getElementById("opt-cell").addEventListener("change", (e) => {
      DMView.showCell = e.target.checked;
    });
    document.getElementById("opt-grid").addEventListener("change", (e) => {
      DMView.showGrid = e.target.checked;
    });
    const zoom = document.getElementById("opt-zoom");
    zoom.addEventListener("input", (e) => {
      DMView.scale = Number(e.target.value);
      document.getElementById("zoom-val").textContent
        = e.target.value;
    });
    document.getElementById("opt-gallery").addEventListener("change", (e) => {
      document.getElementById("gallery").classList
        .toggle("open", e.target.checked);
      if (e.target.checked) { DMGallery.build(); }
    });
    document.getElementById("opt-selftest").addEventListener("change", (e) => {
      const block = document.getElementById("selftest");
      if (e.target.checked) {
        dmRunSelfTest((lines, failures) => {
          block.textContent = lines.join("\n");
          block.classList.remove("ok", "bad");
          block.classList.add(failures === 0 ? "ok" : "bad");
        });
        block.classList.add("open");
      } else {
        block.classList.remove("open");
      }
    });
    window.addEventListener("keydown", (e) => {
      if (e.target.tagName === "INPUT") { return; }
      if (e.key === "ArrowLeft") {
        DMUI.select(Math.max(0, DMView.variant - 1));
      } else if (e.key === "ArrowRight") {
        DMUI.select(Math.min(VARIANTS.length - 1, DMView.variant + 1));
      } else if (e.key >= "0" && e.key <= "9") {
        const num = Number(e.key);
        // 0 jumps to 0/10/20/30 by repeats; a plain digit selects
        // 0..9, the tens live in the button list.
        const id = num;
        if (id < VARIANTS.length) { DMUI.select(id); }
      } else if (e.key === "g" || e.key === "G") {
        const box = document.getElementById("opt-gallery");
        box.checked = !box.checked;
        box.dispatchEvent(new Event("change"));
      } else if (e.key === "k" || e.key === "K") {
        const box = document.getElementById("opt-kills");
        box.checked = !box.checked;
        box.dispatchEvent(new Event("change"));
      }
    });
  }
};

// ---- the gallery ----
// The side by side strip: every variant as a big icon (r = 16) on a
// crop of the same tile (or the meadow), numbered and clickable.
const DMGallery = {
  cells: {},

  build() {
    const grid = document.getElementById("gallery-grid");
    grid.textContent = "";
    this.cells = {};
    const w = 178;
    const h = 110;
    for (const variant of VARIANTS) {
      const cell = document.createElement("div");
      cell.className = "gallery-cell";
      cell.dataset.variant = variant.id;
      const canvas = document.createElement("canvas");
      canvas.width = w * 2;
      canvas.height = h * 2;
      canvas.style.width = w + "px";
      canvas.style.height = h + "px";
      const label = document.createElement("div");
      label.className = "cell-label";
      label.innerHTML = "<span class='num'>" + variant.id
        + "</span><span class='name'>" + variant.name + "</span>";
      cell.appendChild(canvas);
      cell.appendChild(label);
      cell.addEventListener("click", () => {
        DMUI.select(variant.id);
      });
      grid.appendChild(cell);
      this.cells[variant.id] = canvas;
    }
    this.render(DMView.variant);
  },

  // render repaints one cell (or all, when the tile arrives late).
  render(activeId) {
    for (const variant of VARIANTS) {
      const canvas = this.cells[variant.id];
      if (!canvas) { continue; }
      const ctx = canvas.getContext("2d");
      ctx.setTransform(2, 0, 0, 2, 0, 0);
      ctx.clearRect(0, 0, 178, 110);
      // The same background the scene shows: the tile crop or the
      // meadow, so the icons judge on the real ground.
      DMView.drawBackground(ctx, 178, 110);
      variant.draw(ctx, 89, 55, 16, 1.6, SCENE.corpses[0],
        performance.now());
    }
  }
};

// ---- boot ----
window.addEventListener("DOMContentLoaded", () => {
  DMView.init();
  DMUI.buildList();
  DMUI.wire();
  DMUI.select(0);
});

