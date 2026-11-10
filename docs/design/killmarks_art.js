// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The death mark art of the map: one draw function per candidate.
// Every draw runs in a unit box where one unit is the marker radius
// and the origin is the mark centre, so the same art paints at the
// 10px map scale and at the 5x detail zoom. The art is meant to be
// pre-rendered into a sprite once the winner is ported into map.js.

const KP = {
  ink: "#242b33",
  inkSoft: "#39424e",
  bone: "#f1ebdc",
  boneShade: "#cfc6ae",
  boneDeep: "#a49a82",
  stone: "#bcc2c8",
  stoneShade: "#939aa2",
  stoneDark: "#666d75",
  iron: "#a6aeb6",
  ironDark: "#606870",
  wood: "#a4764a",
  woodShade: "#7d562f",
  woodDark: "#54371c",
  rust: "#a6602b",
  blood: "#7c2a26",
  bloodDark: "#511a18",
  soil: "#6f5537",
  soilDark: "#4b3921",
  moss: "#5c7d38",
  leaf: "#477330",
  poison: "#7c3054",
  poisonLight: "#b0779c",
  ghost: "#b6e9e6",
  ghostDeep: "#63b9bb",
  ember: "#f6a83a",
  emberDeep: "#c04f12",
  gold: "#e2b45c",
  pale: "#fbf9f2",
  shade: "rgba(28, 32, 39, 0.26)"
};

const kkGradStore = new WeakMap();

// kkGrad builds (once per context and key) a linear gradient that
// lives in the unit box, so the same object serves every mark.
function kkGrad(ctx, key, x0, y0, x1, y1, stops) {
  let store = kkGradStore.get(ctx);
  if (!store) {
    store = new Map();
    kkGradStore.set(ctx, store);
  }
  let grad = store.get(key);
  if (grad) { return grad; }
  grad = ctx.createLinearGradient(x0, y0, x1, y1);
  for (const stop of stops) { grad.addColorStop(stop[0], stop[1]); }
  store.set(key, grad);

  return grad;
}

const kkBone = (ctx) => kkGrad(ctx, "bone", 0, -0.9, 0.4, 0.9, [
  [0, "#f7f2e5"], [0.55, "#e9e2cf"], [1, "#c6bca2"]
]);
const kkStone = (ctx) => kkGrad(ctx, "stone", 0, -0.9, 0.35, 0.9, [
  [0, "#cdd2d7"], [0.6, "#b2b9c0"], [1, "#8b929a"]
]);
const kkIron = (ctx) => kkGrad(ctx, "iron", 0, -0.9, 0.5, 0.9, [
  [0, "#c2c9d0"], [0.45, "#9fa7af"], [1, "#6b737b"]
]);
const kkWood = (ctx) => kkGrad(ctx, "wood", 0, -0.9, 0.4, 0.9, [
  [0, "#b98a58"], [0.55, "#a0713f"], [1, "#744d29"]
]);
const kkSoil = (ctx) => kkGrad(ctx, "soil", 0, -0.2, 0.2, 1, [
  [0, "#7e6242"], [1, "#4c3820"]
]);
const kkBlood = (ctx) => kkGrad(ctx, "blood", 0, -0.6, 0.3, 0.8, [
  [0, "#94352f"], [1, "#4e1a18"]
]);

// kkAddEllipse appends a full ellipse subpath to the current path.
function kkAddEllipse(ctx, cx, cy, rx, ry, rot) {
  const a = rot || 0;
  const ax = Math.max(0.001, rx);
  const ay = Math.max(0.001, ry);
  ctx.moveTo(cx + ax * Math.cos(a), cy + ax * Math.sin(a));
  ctx.ellipse(cx, cy, ax, ay, a, 0, Math.PI * 2);
}

// kkEllipse starts a full ellipse subpath.
function kkEllipse(ctx, cx, cy, rx, ry, rot) {
  ctx.beginPath();
  kkAddEllipse(ctx, cx, cy, rx, ry, rot);
}

// kkAddRound appends a rounded rectangle subpath.
function kkAddRound(ctx, x, y, w, h, r) {
  const rr = Math.min(r, Math.min(w, h) / 2);
  ctx.moveTo(x + rr, y);
  ctx.arcTo(x + w, y, x + w, y + h, rr);
  ctx.arcTo(x + w, y + h, x, y + h, rr);
  ctx.arcTo(x, y + h, x, y, rr);
  ctx.arcTo(x, y, x + w, y, rr);
  ctx.closePath();
}

// kkPoly starts a polygon subpath.
function kkPoly(ctx, pts) {
  ctx.beginPath();
  ctx.moveTo(pts[0][0], pts[0][1]);
  for (let i = 1; i < pts.length; i += 1) {
    ctx.lineTo(pts[i][0], pts[i][1]);
  }
  ctx.closePath();
}

// kkRound starts a rounded rectangle subpath.
function kkRound(ctx, x, y, w, h, r) {
  ctx.beginPath();
  kkAddRound(ctx, x, y, w, h, r);
}

// kkPaint fills and strokes the current path.
function kkPaint(ctx, fill, line, lw) {
  if (fill) {
    ctx.fillStyle = fill;
    ctx.fill();
  }
  if (line) {
    ctx.strokeStyle = line;
    ctx.lineWidth = lw === undefined ? 0.12 : lw;
    ctx.lineJoin = "round";
    ctx.lineCap = "round";
    ctx.stroke();
  }
}

// kkShadow lays the soft ground shadow that holds the mark down.
function kkShadow(ctx, rx, ry, alpha) {
  ctx.fillStyle = "rgba(24, 29, 35, " + alpha + ")";
  kkEllipse(ctx, 0, 0.66, rx, ry);
  ctx.fill();
}

// kkSkullBody outlines a cranium with the tapered jaw.
function kkSkullBody(ctx, k) {
  ctx.beginPath();
  ctx.ellipse(0, -0.06 * k, 0.6 * k, 0.58 * k, 0, Math.PI, Math.PI * 2);
  ctx.lineTo(0.48 * k, 0.14 * k);
  ctx.lineTo(0.37 * k, 0.25 * k);
  ctx.lineTo(0.33 * k, 0.52 * k);
  ctx.quadraticCurveTo(0.31 * k, 0.62 * k, 0.19 * k, 0.62 * k);
  ctx.lineTo(-0.19 * k, 0.62 * k);
  ctx.quadraticCurveTo(-0.31 * k, 0.62 * k, -0.33 * k, 0.52 * k);
  ctx.lineTo(-0.37 * k, 0.25 * k);
  ctx.lineTo(-0.48 * k, 0.14 * k);
  ctx.closePath();
}

// kkSkullFace paints the empty sockets, the nose and the teeth.
function kkSkullFace(ctx, k) {
  ctx.fillStyle = KP.ink;
  kkEllipse(ctx, -0.24 * k, 0.02 * k, 0.155 * k, 0.175 * k, -0.2);
  ctx.fill();
  kkEllipse(ctx, 0.24 * k, 0.02 * k, 0.155 * k, 0.175 * k, 0.2);
  ctx.fill();
  kkPoly(ctx, [[0, 0.2 * k], [-0.085 * k, 0.36 * k], [0.085 * k, 0.36 * k]]);
  ctx.fill();
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = 0.065 * k;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(-0.2 * k, 0.43 * k);
  ctx.lineTo(0.2 * k, 0.43 * k);
  for (const tx of [-0.105, 0, 0.105]) {
    ctx.moveTo(tx * k, 0.43 * k);
    ctx.lineTo(tx * k, 0.57 * k);
  }
  ctx.stroke();
}

// kkSkull draws the whole skull in bone with a soft cheek shade.
function kkSkull(ctx, k, line) {
  kkSkullBody(ctx, k);
  kkPaint(ctx, kkBone(ctx), line || KP.ink, 0.12 * k);
  kkSkullFace(ctx, k);
  ctx.save();
  kkSkullBody(ctx, k);
  ctx.clip();
  ctx.fillStyle = "rgba(120, 104, 76, 0.2)";
  kkEllipse(ctx, 0.62 * k, 0.42 * k, 0.62 * k, 0.5 * k, 0.3);
  ctx.fill();
  ctx.restore();
}

// kkLimb strokes a bone with an ink casing: the wide dark pass under
// the narrower bone pass keeps the thin shapes readable at map scale.
function kkLimb(ctx, build, wide, thin, fill) {
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = wide;
  build(ctx);
  ctx.stroke();
  ctx.strokeStyle = fill || kkBone(ctx);
  ctx.lineWidth = thin;
  build(ctx);
  ctx.stroke();
}

// kkBoneBar draws one knobbed bone from end to end.
function kkBoneBar(ctx, x1, y1, x2, y2, fill, w) {
  const width = w || 0.13;
  const len = Math.hypot(x2 - x1, y2 - y1);
  const ang = Math.atan2(y2 - y1, x2 - x1);
  ctx.save();
  ctx.translate((x1 + x2) / 2, (y1 + y2) / 2);
  ctx.rotate(ang);
  ctx.beginPath();
  kkAddRound(ctx, -len / 2 + 0.08, -width, len - 0.16, width * 2, width);
  for (const sx of [-1, 1]) {
    kkAddEllipse(ctx, sx * (len / 2 - 0.07), -width * 0.95, 0.19, 0.15, 0);
    kkAddEllipse(ctx, sx * (len / 2 - 0.07), width * 0.95, 0.19, 0.15, 0);
  }
  kkPaint(ctx, fill || kkBone(ctx), KP.ink, 0.1);
  ctx.restore();
}

// kkGrass paints short tufts of grass at the base of a mark.
function kkGrass(ctx, xs, y, color, size) {
  const s = size || 0.2;
  ctx.strokeStyle = color || KP.moss;
  ctx.lineWidth = 0.07;
  ctx.lineCap = "round";
  ctx.beginPath();
  for (const x of xs) {
    ctx.moveTo(x, y);
    ctx.quadraticCurveTo(x - s * 0.6, y - s * 0.8, x - s, y - s * 1.6);
    ctx.moveTo(x, y);
    ctx.quadraticCurveTo(x + s * 0.2, y - s * 0.9, x + s * 0.35, y - s * 1.7);
    ctx.moveTo(x, y);
    ctx.quadraticCurveTo(x + s * 0.9, y - s * 0.7, x + s * 1.2, y - s * 1.2);
  }
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V00 - the marker the map ships today: the gray corpse face.

function v00(ctx) {
  ctx.fillStyle = "#80868b";
  ctx.beginPath();
  ctx.arc(0, 0, 0.96, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = "#39424e";
  ctx.lineWidth = 0.15;
  ctx.lineCap = "round";
  const eye = 0.15;
  ctx.beginPath();
  for (const ex of [-0.31, 0.31]) {
    ctx.moveTo(ex - eye, -0.11 - eye);
    ctx.lineTo(ex + eye, -0.11 + eye);
    ctx.moveTo(ex + eye, -0.11 - eye);
    ctx.lineTo(ex - eye, -0.11 + eye);
  }
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V01 - the plain skull.

function v01(ctx) {
  kkShadow(ctx, 0.72, 0.22, 0.3);
  kkSkull(ctx, 1);
}

// ---------------------------------------------------------------------
// V02 - skull over crossed bones.

function v02(ctx) {
  kkShadow(ctx, 0.92, 0.24, 0.3);
  kkBoneBar(ctx, -0.92, -0.66, 0.92, 0.66, kkBone(ctx), 0.12);
  kkBoneBar(ctx, -0.92, 0.66, 0.92, -0.66, kkBone(ctx), 0.12);
  ctx.save();
  ctx.translate(0, -0.04);
  kkSkull(ctx, 0.78);
  ctx.restore();
}

// ---------------------------------------------------------------------
// V03 - skull on a blade planted in the ground.

function v03(ctx) {
  kkShadow(ctx, 0.62, 0.2, 0.3);
  // Spear planted through the skull: knobbed shaft, then the head.
  kkRound(ctx, -0.065, -0.6, 0.13, 1.44, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.09);
  kkPoly(ctx, [[-0.16, -0.5], [0.16, -0.5], [0.11, -0.76], [0, -1.0],
    [-0.11, -0.76]]);
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.1);
  ctx.strokeStyle = "rgba(255, 255, 255, 0.55)";
  ctx.lineWidth = 0.035;
  ctx.beginPath();
  ctx.moveTo(-0.02, -0.56);
  ctx.lineTo(-0.01, -0.9);
  ctx.stroke();
  // Socket collar.
  kkRound(ctx, -0.13, -0.54, 0.26, 0.13, 0.05);
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.09);
  // The skull sits low on the shaft.
  ctx.save();
  ctx.translate(0, 0.28);
  ctx.scale(0.66, 0.66);
  kkSkull(ctx, 1);
  ctx.restore();
}

// ---------------------------------------------------------------------
// V04 - skull under a rusted helm.

function v04(ctx) {
  kkShadow(ctx, 0.74, 0.22, 0.3);
  ctx.save();
  ctx.translate(0, 0.3);
  ctx.scale(0.72, 0.72);
  kkSkull(ctx, 1);
  ctx.restore();
  // Dome, crest and brim.
  ctx.beginPath();
  ctx.ellipse(0, -0.16, 0.56, 0.52, 0, Math.PI, Math.PI * 2);
  ctx.lineTo(0.56, -0.02);
  ctx.quadraticCurveTo(0.56, 0.06, 0.42, 0.06);
  ctx.lineTo(-0.42, 0.06);
  ctx.quadraticCurveTo(-0.56, 0.06, -0.56, -0.02);
  ctx.closePath();
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.11);
  // Crest ridge.
  ctx.beginPath();
  ctx.ellipse(0, -0.16, 0.5, 0.46, 0, Math.PI * 1.08, Math.PI * 1.92);
  ctx.strokeStyle = KP.ironDark;
  ctx.lineWidth = 0.07;
  ctx.lineCap = "round";
  ctx.stroke();
  // Nose guard and cheek plates.
  kkPoly(ctx, [[-0.07, 0.02], [0.07, 0.02], [0.05, 0.3], [0, 0.36],
    [-0.05, 0.3]]);
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.09);
  for (const s of [-1, 1]) {
    kkPoly(ctx, [[s * 0.4, 0.0], [s * 0.56, -0.02], [s * 0.5, 0.32],
      [s * 0.36, 0.3]]);
    kkPaint(ctx, kkIron(ctx), KP.ink, 0.09);
  }
  // Rivets and a highlight.
  ctx.fillStyle = KP.ink;
  for (const rx of [-0.3, 0.3]) {
    kkEllipse(ctx, rx, -0.12, 0.04, 0.04);
    ctx.fill();
  }
  ctx.strokeStyle = "rgba(255, 255, 255, 0.42)";
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.28, -0.46);
  ctx.quadraticCurveTo(-0.06, -0.58, 0.18, -0.5);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V05 - skull with a memorial candle.

function v05(ctx, t) {
  kkShadow(ctx, 0.66, 0.2, 0.3);
  ctx.save();
  ctx.translate(0, 0.3);
  kkSkull(ctx, 0.66);
  ctx.restore();
  // Candle stub with a wax drip.
  kkRound(ctx, -0.1, -0.56, 0.2, 0.36, 0.05);
  kkPaint(ctx, kkGrad(ctx, "wax", 0, -0.6, 0, -0.2,
    [[0, "#fffdf4"], [1, "#ded5bf"]]), KP.ink, 0.08);
  ctx.fillStyle = "#efe7d3";
  kkEllipse(ctx, -0.1, -0.3, 0.055, 0.1, 0.1);
  ctx.fill();
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(0, -0.56);
  ctx.lineTo(0, -0.64);
  ctx.stroke();
  // Flame, flickering with the clock.
  const wob = Math.sin((t || 0) * 9) * 0.03
    + Math.sin((t || 0) * 23) * 0.012;
  ctx.beginPath();
  ctx.moveTo(0, -0.66);
  ctx.bezierCurveTo(0.09, -0.74, 0.07, -0.86, wob, -0.98);
  ctx.bezierCurveTo(-0.07, -0.86, -0.09, -0.74, 0, -0.66);
  ctx.closePath();
  kkPaint(ctx, kkGrad(ctx, "flame", 0, -1, 0, -0.64, [
    [0, "#ffe6a8"], [0.5, "#f7a838"], [1, "#d9621a"]
  ]), null);
  ctx.fillStyle = "rgba(255, 224, 150, 0.35)";
  kkEllipse(ctx, 0, -0.8, 0.22, 0.24);
  ctx.fill();
}

// ---------------------------------------------------------------------
// V06 - two skulls of a wiped pack.

function v06(ctx) {
  kkShadow(ctx, 0.92, 0.22, 0.3);
  ctx.save();
  ctx.translate(0.3, 0.24);
  ctx.rotate(0.18);
  ctx.scale(0.56, 0.56);
  kkSkull(ctx, 1);
  ctx.restore();
  ctx.save();
  ctx.translate(-0.28, 0.05);
  ctx.rotate(-0.14);
  ctx.scale(0.62, 0.62);
  kkSkull(ctx, 1);
  ctx.restore();
}

// ---------------------------------------------------------------------
// V07 - cracked skull.

function v07(ctx) {
  kkShadow(ctx, 0.72, 0.22, 0.3);
  kkSkull(ctx, 1);
  // The crack runs crown to jaw.
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = 0.06;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(-0.04, -0.62);
  ctx.lineTo(-0.12, -0.44);
  ctx.lineTo(0.02, -0.36);
  ctx.lineTo(-0.05, -0.18);
  ctx.lineTo(0.06, -0.08);
  ctx.stroke();
  // The chipped crown.
  kkPoly(ctx, [[0.18, -0.6], [0.36, -0.55], [0.44, -0.4], [0.3, -0.36],
    [0.24, -0.46]]);
  ctx.fillStyle = KP.ink;
  ctx.fill();
}

// ---------------------------------------------------------------------
// V08 - horned beast skull.

function v08(ctx) {
  kkShadow(ctx, 0.86, 0.22, 0.3);
  // Horns behind the skull.
  for (const s of [-1, 1]) {
    ctx.beginPath();
    ctx.moveTo(s * 0.26, -0.14);
    ctx.bezierCurveTo(s * 0.6, -0.3, s * 0.92, -0.52, s * 1.0, -0.78);
    ctx.bezierCurveTo(s * 0.78, -0.52, s * 0.55, -0.4, s * 0.2, -0.32);
    ctx.closePath();
    kkPaint(ctx, kkBone(ctx), KP.ink, 0.1);
  }
  // Long skull with a heavy muzzle.
  ctx.beginPath();
  ctx.ellipse(0, -0.12, 0.4, 0.38, 0, Math.PI, Math.PI * 2);
  ctx.lineTo(0.3, 0.1);
  ctx.lineTo(0.2, 0.7);
  ctx.quadraticCurveTo(0, 0.82, -0.2, 0.7);
  ctx.lineTo(-0.3, 0.1);
  ctx.closePath();
  kkPaint(ctx, kkBone(ctx), KP.ink, 0.11);
  ctx.fillStyle = KP.ink;
  kkEllipse(ctx, -0.16, -0.02, 0.1, 0.13, -0.35);
  ctx.fill();
  kkEllipse(ctx, 0.16, -0.02, 0.1, 0.13, 0.35);
  ctx.fill();
  // Nasal slits and the tooth row.
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = 0.05;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(-0.05, 0.3);
  ctx.lineTo(-0.045, 0.46);
  ctx.moveTo(0.05, 0.3);
  ctx.lineTo(0.045, 0.46);
  ctx.moveTo(-0.15, 0.62);
  ctx.lineTo(0.15, 0.62);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V09 - round topped headstone.

function v09(ctx) {
  kkShadow(ctx, 0.78, 0.2, 0.3);
  ctx.beginPath();
  ctx.moveTo(-0.52, 0.62);
  ctx.lineTo(-0.52, -0.34);
  ctx.bezierCurveTo(-0.52, -0.72, 0.52, -0.72, 0.52, -0.34);
  ctx.lineTo(0.52, 0.62);
  ctx.closePath();
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.12);
  // Engraved cross with a carved shadow.
  ctx.strokeStyle = "rgba(255, 255, 255, 0.55)";
  ctx.lineWidth = 0.1;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(0, -0.4);
  ctx.lineTo(0, 0.2);
  ctx.moveTo(-0.17, -0.22);
  ctx.lineTo(0.17, -0.22);
  ctx.stroke();
  ctx.strokeStyle = KP.stoneDark;
  ctx.lineWidth = 0.07;
  ctx.beginPath();
  ctx.moveTo(0, -0.42);
  ctx.lineTo(0, 0.18);
  ctx.moveTo(-0.16, -0.24);
  ctx.lineTo(0.16, -0.24);
  ctx.stroke();
  // Weathering and grass.
  ctx.strokeStyle = "rgba(255, 255, 255, 0.35)";
  ctx.lineWidth = 0.045;
  ctx.beginPath();
  ctx.moveTo(-0.4, -0.44);
  ctx.lineTo(-0.4, 0.3);
  ctx.stroke();
  kkGrass(ctx, [-0.42, 0.36, 0.5], 0.66, KP.moss, 0.18);
}

// ---------------------------------------------------------------------
// V10 - celtic cross.

function v10(ctx) {
  kkShadow(ctx, 0.66, 0.2, 0.3);
  ctx.save();
  ctx.rotate(0.06);
  // The wide ring reads as a ring only when the bars stay narrow.
  ctx.beginPath();
  ctx.arc(0, -0.2, 0.48, 0, Math.PI * 2);
  ctx.arc(0, -0.2, 0.33, 0, Math.PI * 2, true);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.09);
  kkRound(ctx, -0.12, -0.86, 0.24, 1.48, 0.06);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.1);
  kkRound(ctx, -0.56, -0.32, 1.12, 0.24, 0.06);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.1);
  // Engraved lines.
  ctx.strokeStyle = KP.stoneDark;
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.04, -0.72);
  ctx.lineTo(-0.04, 0.5);
  ctx.stroke();
  ctx.restore();
  kkGrass(ctx, [-0.24, 0.2], 0.7, KP.moss, 0.17);
}

// ---------------------------------------------------------------------
// V11 - leaning cracked slab.

function v11(ctx) {
    kkShadow(ctx, 0.7, 0.2, 0.3);
    ctx.save();
    ctx.rotate(0.16);
    ctx.beginPath();
    ctx.moveTo(-0.4, 0.68);
    ctx.lineTo(-0.44, -0.5);
    ctx.lineTo(-0.14, -0.78);
    ctx.lineTo(0.2, -0.84);
    ctx.lineTo(0.44, -0.6);
    ctx.lineTo(0.4, 0.68);
    ctx.closePath();
    kkPaint(ctx, kkStone(ctx), KP.ink, 0.12);
    // The carved cross: a deep groove with a lit lower lip.
    ctx.strokeStyle = "#4d545c";
    ctx.lineWidth = 0.13;
    ctx.lineCap = "butt";
    ctx.beginPath();
    ctx.moveTo(0, -0.6);
    ctx.lineTo(0, 0.14);
    ctx.moveTo(-0.24, -0.38);
    ctx.lineTo(0.24, -0.38);
    ctx.stroke();
    ctx.strokeStyle = "rgba(255, 255, 255, 0.75)";
    ctx.lineWidth = 0.045;
    ctx.beginPath();
    ctx.moveTo(0.08, -0.58);
    ctx.lineTo(0.08, 0.12);
    ctx.moveTo(-0.22, -0.33);
    ctx.lineTo(0.22, -0.33);
    ctx.stroke();
    ctx.strokeStyle = KP.ink;
    ctx.lineWidth = 0.05;
    ctx.beginPath();
    ctx.moveTo(0.26, -0.76);
    ctx.lineTo(0.18, -0.44);
    ctx.lineTo(0.3, -0.24);
    ctx.lineTo(0.22, 0.06);
    ctx.stroke();
    // Moss at the foot.
    ctx.fillStyle = KP.moss;
    kkEllipse(ctx, -0.14, 0.62, 0.26, 0.11);
    ctx.fill();
    kkEllipse(ctx, 0.24, 0.66, 0.14, 0.07);
    ctx.fill();
    ctx.restore();
    kkGrass(ctx, [-0.2, 0.24], 0.72, KP.moss, 0.16);
}

// ---------------------------------------------------------------------
// V12 - wooden cross.

function v12(ctx) {
  kkShadow(ctx, 0.66, 0.2, 0.3);
  ctx.save();
  ctx.rotate(-0.1);
  kkRound(ctx, -0.14, -0.76, 0.28, 1.46, 0.07);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.11);
  ctx.save();
  ctx.rotate(0.11);
  kkRound(ctx, -0.56, -0.4, 1.12, 0.24, 0.07);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.11);
  ctx.restore();
  // Wood grain.
  ctx.strokeStyle = "rgba(90, 60, 30, 0.45)";
  ctx.lineWidth = 0.035;
  ctx.beginPath();
  ctx.moveTo(-0.06, -0.6);
  ctx.lineTo(-0.05, 0.5);
  ctx.moveTo(0.06, -0.5);
  ctx.lineTo(0.05, 0.56);
  ctx.stroke();
  // Nails and the rope lashing.
  ctx.fillStyle = KP.ink;
  for (const n of [[-0.4, -0.28], [0.4, -0.28]]) {
    kkEllipse(ctx, n[0], n[1], 0.05, 0.05);
    ctx.fill();
  }
  ctx.strokeStyle = "#d3c093";
  ctx.lineWidth = 0.07;
  ctx.beginPath();
  ctx.moveTo(-0.18, -0.4);
  ctx.lineTo(0.14, -0.14);
  ctx.moveTo(0.16, -0.4);
  ctx.lineTo(-0.16, -0.14);
  ctx.stroke();
  ctx.restore();
  kkGrass(ctx, [-0.18, 0.22], 0.7, KP.moss, 0.16);
}

// ---------------------------------------------------------------------
// V13 - stone cross overgrown with ivy.

function v13(ctx) {
  kkShadow(ctx, 0.64, 0.2, 0.3);
  ctx.save();
  ctx.rotate(-0.05);
  kkRound(ctx, -0.12, -0.86, 0.24, 1.44, 0.05);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.11);
  kkRound(ctx, -0.44, -0.42, 0.88, 0.24, 0.05);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.11);
  // Ivy climbs the foot.
  ctx.fillStyle = KP.leaf;
  for (const leaf of [[-0.2, 0.3, 0.14, 0.09, -0.5],
    [0.16, 0.42, 0.15, 0.1, 0.4], [-0.06, 0.5, 0.13, 0.09, 0.1],
    [0.22, 0.2, 0.12, 0.08, 0.7]]) {
    kkEllipse(ctx, leaf[0], leaf[1], leaf[2], leaf[3], leaf[4]);
    ctx.fill();
  }
  ctx.strokeStyle = "#6b8a45";
  ctx.lineWidth = 0.04;
  ctx.beginPath();
  ctx.moveTo(-0.16, 0.58);
  ctx.quadraticCurveTo(0.05, 0.34, 0.2, 0.24);
  ctx.stroke();
  ctx.restore();
  kkGrass(ctx, [-0.34, 0.34], 0.72, KP.moss, 0.16);
}

// ---------------------------------------------------------------------
// V14 - grave mound with a shovel.

function v14(ctx, t) {
  kkShadow(ctx, 0.92, 0.2, 0.3);
  // A fresh mound with a grass edge.
  ctx.beginPath();
  ctx.moveTo(-0.9, 0.6);
  ctx.bezierCurveTo(-0.72, 0.04, 0.66, -0.02, 0.9, 0.6);
  ctx.closePath();
  kkPaint(ctx, kkSoil(ctx), KP.ink, 0.11);
  ctx.strokeStyle = "rgba(214, 196, 150, 0.45)";
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.62, 0.1);
  ctx.quadraticCurveTo(-0.1, -0.02, 0.5, 0.14);
  ctx.stroke();
  // Wooden marker on the crown.
  ctx.save();
  ctx.translate(-0.46, 0.0);
  ctx.rotate(-0.12);
  kkRound(ctx, -0.09, -0.34, 0.18, 0.56, 0.03);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.09);
  kkRound(ctx, -0.2, -0.3, 0.4, 0.1, 0.02);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.08);
  ctx.restore();
  // Spade stuck in the mound at the far side.
  ctx.save();
  ctx.translate(0.5, 0.12);
  ctx.rotate(0.26);
  kkRound(ctx, -0.055, -0.94, 0.11, 0.84, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.09);
  kkRound(ctx, -0.16, -1.02, 0.32, 0.1, 0.04);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.08);
  ctx.beginPath();
  ctx.moveTo(-0.17, -0.14);
  ctx.lineTo(0.17, -0.14);
  ctx.lineTo(0.15, 0.1);
  ctx.quadraticCurveTo(0.1, 0.28, 0, 0.38);
  ctx.quadraticCurveTo(-0.1, 0.28, -0.15, 0.1);
  ctx.closePath();
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.09);
  ctx.strokeStyle = "rgba(255, 255, 255, 0.5)";
  ctx.lineWidth = 0.035;
  ctx.beginPath();
  ctx.moveTo(-0.08, -0.06);
  ctx.lineTo(-0.02, 0.24);
  ctx.stroke();
  ctx.restore();
  // Soil crumbs.
  ctx.fillStyle = kkSoil(ctx);
  for (const c of [[-0.76, 0.52, 0.07], [0.66, 0.54, 0.06],
    [0.3, 0.64, 0.05]]) {
    kkEllipse(ctx, c[0], c[1], c[2], c[2] * 0.7);
    ctx.fill();
  }
  kkGrass(ctx, [-0.66, 0.72], 0.66, KP.moss, 0.15);
}

// ---------------------------------------------------------------------
// V15 - weathered obelisk.

function v15(ctx) {
  kkShadow(ctx, 0.62, 0.18, 0.3);
  // Base plinth.
  kkRound(ctx, -0.5, 0.44, 1.0, 0.24, 0.04);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.11);
  // Tapered shaft with a pyramid cap.
  kkPoly(ctx, [[-0.34, 0.46], [0.34, 0.46], [0.2, -0.62], [0, -0.92],
    [-0.2, -0.62]]);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.12);
  ctx.strokeStyle = KP.stoneDark;
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.24, 0.2);
  ctx.lineTo(0.24, 0.2);
  ctx.moveTo(-0.2, 0.02);
  ctx.lineTo(0.2, 0.02);
  ctx.stroke();
  ctx.fillStyle = KP.moss;
  kkEllipse(ctx, -0.3, 0.4, 0.14, 0.08);
  ctx.fill();
}

// ---------------------------------------------------------------------
// V16 - coffin lid.

function v16(ctx) {
  kkShadow(ctx, 0.6, 0.18, 0.3);
  ctx.save();
  ctx.rotate(-0.12);
  ctx.beginPath();
  ctx.moveTo(-0.4, -0.72);
  ctx.lineTo(0.4, -0.72);
  ctx.lineTo(0.54, -0.3);
  ctx.lineTo(0.4, 0.7);
  ctx.lineTo(-0.4, 0.7);
  ctx.lineTo(-0.54, -0.3);
  ctx.closePath();
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.12);
  // Plank seam and a carved cross.
  ctx.strokeStyle = KP.woodDark;
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(0, -0.68);
  ctx.lineTo(0, 0.66);
  ctx.stroke();
  ctx.strokeStyle = "#e2d3ab";
  ctx.lineWidth = 0.075;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(0, -0.5);
  ctx.lineTo(0, -0.08);
  ctx.moveTo(-0.18, -0.34);
  ctx.lineTo(0.18, -0.34);
  ctx.stroke();
  ctx.fillStyle = KP.ink;
  for (const n of [[-0.3, -0.6], [0.3, -0.6], [-0.3, 0.56], [0.3, 0.56]]) {
    kkEllipse(ctx, n[0], n[1], 0.045, 0.045);
    ctx.fill();
  }
  ctx.restore();
}

// ---------------------------------------------------------------------
// V17 - crossed femurs.

function v17(ctx) {
  kkShadow(ctx, 0.9, 0.2, 0.3);
  kkBoneBar(ctx, -0.92, 0.62, 0.92, -0.62, kkBone(ctx), 0.13);
  kkBoneBar(ctx, -0.92, -0.62, 0.92, 0.62, kkBone(ctx), 0.13);
}

// ---------------------------------------------------------------------
// V18 - half buried rib cage.

function v18(ctx) {
  kkShadow(ctx, 0.72, 0.18, 0.3);
  // Spine.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(0, -0.76);
    c.quadraticCurveTo(0.06, 0, 0, 0.62);
  }, 0.2, 0.13);
  // Three rib pairs as tapering bands.
  const ribs = [[-0.5, 0.5], [-0.18, 0.46], [0.14, 0.4]];
  for (const rib of ribs) {
    const y = rib[0];
    const w = rib[1];
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.moveTo(s * 0.03, y);
      ctx.bezierCurveTo(s * w * 0.72, y - 0.06, s * w, y + 0.1,
        s * w * 0.68, y + 0.3);
      ctx.bezierCurveTo(s * w * 0.48, y + 0.34, s * w * 0.34, y + 0.2,
        s * 0.03, y + 0.1);
      ctx.closePath();
      kkPaint(ctx, kkBone(ctx), KP.ink, 0.07);
    }
  }
  // Collarbones.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(-0.04, -0.7);
    c.quadraticCurveTo(-0.3, -0.8, -0.48, -0.64);
    c.moveTo(0.04, -0.7);
    c.quadraticCurveTo(0.3, -0.8, 0.48, -0.64);
  }, 0.14, 0.08);
}

// ---------------------------------------------------------------------
// V19 - a heap of bones.

function v19(ctx) {
  kkShadow(ctx, 0.86, 0.2, 0.3);
  kkBoneBar(ctx, -0.78, 0.34, 0.74, 0.52, kkBone(ctx), 0.11);
  kkBoneBar(ctx, -0.6, 0.02, 0.62, 0.3, kkBone(ctx), 0.1);
  // A small skull on the heap.
  ctx.save();
  ctx.translate(0.06, -0.24);
  ctx.scale(0.42, 0.42);
  kkSkull(ctx, 1);
  ctx.restore();
  // Vertebrae along the heap.
  for (const v of [[-0.44, -0.02, 0.12], [-0.24, -0.16, 0.11],
    [0.36, 0.02, 0.1]]) {
    kkEllipse(ctx, v[0], v[1], v[2], v[2] * 0.72);
    kkPaint(ctx, kkBone(ctx), KP.ink, 0.08);
    ctx.fillStyle = KP.ink;
    kkEllipse(ctx, v[0], v[1], v[2] * 0.3, v[2] * 0.22);
    ctx.fill();
  }
  kkBoneBar(ctx, -0.34, -0.44, 0.36, -0.1, kkBone(ctx), 0.09);
}

// ---------------------------------------------------------------------
// V20 - skeleton hand out of the soil.

function v20(ctx) {
  kkShadow(ctx, 0.7, 0.18, 0.3);
  // Soil pile the hand rises from.
  ctx.beginPath();
  ctx.moveTo(-0.62, 0.66);
  ctx.bezierCurveTo(-0.4, 0.36, 0.36, 0.34, 0.66, 0.66);
  ctx.closePath();
  kkPaint(ctx, kkSoil(ctx), KP.ink, 0.1);
  // Wrist and palm.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(0, 0.5);
    c.lineTo(0.02, 0.16);
  }, 0.24, 0.16);
  ctx.beginPath();
  ctx.ellipse(0.02, 0.06, 0.19, 0.16, 0, 0, Math.PI * 2);
  kkPaint(ctx, kkBone(ctx), KP.ink, 0.09);
  // Four fingers and the thumb.
  const fingers = [[-0.15, -0.6], [-0.05, -0.72], [0.06, -0.7],
    [0.16, -0.56]];
  for (const f of fingers) {
    kkLimb(ctx, (c) => {
      c.beginPath();
      c.moveTo(f[0] * 0.8, 0.02);
      c.lineTo(f[0], f[1] * 0.64);
      c.lineTo(f[0] + f[0] * 0.3, f[1]);
    }, 0.13, 0.075);
  }
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(-0.14, 0.08);
    c.lineTo(-0.34, -0.16);
    c.lineTo(-0.44, -0.34);
  }, 0.13, 0.075);
  // Knuckle dots.
  ctx.fillStyle = KP.ink;
  for (const f of fingers) {
    kkEllipse(ctx, f[0], f[1] * 0.64, 0.035, 0.035);
    ctx.fill();
  }
}

// ---------------------------------------------------------------------
// V21 - cairn of stacked stones.

function v21(ctx) {
  kkShadow(ctx, 0.7, 0.18, 0.3);
  const stones = [
    [-0.05, 0.5, 0.5, 0.16, 0.02],
    [0.03, 0.24, 0.42, 0.15, -0.05],
    [-0.03, 0.0, 0.34, 0.13, 0.06],
    [0.02, -0.21, 0.26, 0.11, -0.08],
    [0.0, -0.38, 0.17, 0.09, 0.03]
  ];
  for (const s of stones) {
    kkEllipse(ctx, s[0], s[1], s[2], s[3], s[4]);
    kkPaint(ctx, kkStone(ctx), KP.ink, 0.1);
    ctx.strokeStyle = "rgba(255, 255, 255, 0.3)";
    ctx.lineWidth = 0.04;
    ctx.beginPath();
    ctx.moveTo(s[0] - s[2] * 0.5, s[1] - s[3] * 0.2);
    ctx.lineTo(s[0] + s[2] * 0.4, s[1] - s[3] * 0.35);
    ctx.stroke();
  }
  ctx.fillStyle = KP.moss;
  kkEllipse(ctx, -0.2, -0.34, 0.1, 0.05);
  ctx.fill();
  kkGrass(ctx, [-0.48, 0.46], 0.68, KP.moss, 0.15);
}

// ---------------------------------------------------------------------
// V22 - reaper scythe.

function v22(ctx) {
  kkShadow(ctx, 0.8, 0.2, 0.3);
  // Blade: a long crescent off the top of the snath.
  ctx.beginPath();
  ctx.moveTo(-0.34, -0.8);
  ctx.bezierCurveTo(-0.82, -0.94, -1.14, -0.4, -1.06, 0.2);
  ctx.bezierCurveTo(-0.98, -0.2, -0.74, -0.44, -0.34, -0.54);
  ctx.closePath();
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.1);
  // Bright edge along the cutting side.
  ctx.strokeStyle = "rgba(255, 255, 255, 0.7)";
  ctx.lineWidth = 0.045;
  ctx.beginPath();
  ctx.moveTo(-0.4, -0.74);
  ctx.bezierCurveTo(-0.8, -0.84, -1.04, -0.42, -1.0, 0.06);
  ctx.stroke();
  // Dark back of the blade.
  ctx.strokeStyle = KP.ironDark;
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.36, -0.6);
  ctx.bezierCurveTo(-0.7, -0.48, -0.9, -0.22, -0.98, 0.1);
  ctx.stroke();
  // Snath running to the ground.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(0.62, 0.84);
    c.lineTo(-0.38, -0.68);
  }, 0.2, 0.13, kkWood(ctx));
  // Grip stubs and the binding at the joint.
  kkRound(ctx, 0.06, 0.02, 0.34, 0.11, 0.05);
  kkPaint(ctx, KP.woodShade, KP.ink, 0.07);
  kkRound(ctx, -0.2, -0.4, 0.3, 0.1, 0.05);
  kkPaint(ctx, KP.woodShade, KP.ink, 0.07);
  ctx.strokeStyle = "#d8c9a1";
  ctx.lineWidth = 0.05;
  ctx.beginPath();
  ctx.moveTo(-0.46, -0.76);
  ctx.lineTo(-0.26, -0.6);
  ctx.moveTo(-0.4, -0.6);
  ctx.lineTo(-0.2, -0.44);
  ctx.stroke();
  kkGrass(ctx, [-0.5, 0.32], 0.82, KP.moss, 0.14);
}

// ---------------------------------------------------------------------
// V23 - hooded reaper silhouette.

function v23(ctx) {
  kkShadow(ctx, 0.66, 0.18, 0.3);
  ctx.beginPath();
  ctx.moveTo(0, -0.88);
  ctx.bezierCurveTo(0.44, -0.9, 0.56, -0.36, 0.42, 0.0);
  ctx.bezierCurveTo(0.66, 0.24, 0.72, 0.6, 0.56, 0.88);
  ctx.lineTo(-0.56, 0.88);
  ctx.bezierCurveTo(-0.72, 0.6, -0.66, 0.24, -0.42, 0.0);
  ctx.bezierCurveTo(-0.56, -0.36, -0.44, -0.9, 0, -0.88);
  ctx.closePath();
  kkPaint(ctx, "#2b333c", "rgba(178, 220, 224, 0.75)", 0.05);
  // The void of the hood with two pale eyes.
  ctx.fillStyle = "#0f1418";
  kkEllipse(ctx, 0, -0.48, 0.27, 0.3);
  ctx.fill();
  ctx.fillStyle = KP.ghost;
  kkEllipse(ctx, -0.11, -0.5, 0.07, 0.085);
  ctx.fill();
  kkEllipse(ctx, 0.11, -0.5, 0.07, 0.085);
  ctx.fill();
  ctx.fillStyle = "rgba(150, 235, 232, 0.35)";
  kkEllipse(ctx, 0, -0.48, 0.18, 0.2);
  ctx.fill();
  ctx.fillStyle = KP.ghost;
  kkEllipse(ctx, -0.11, -0.5, 0.055, 0.07);
  ctx.fill();
  kkEllipse(ctx, 0.11, -0.5, 0.055, 0.07);
  ctx.fill();
  // Sleeve fold highlights keep the silhouette readable on dark fill.
  ctx.strokeStyle = "rgba(150, 190, 200, 0.55)";
  ctx.lineWidth = 0.04;
  ctx.beginPath();
  ctx.moveTo(-0.4, 0.1);
  ctx.quadraticCurveTo(-0.2, 0.24, -0.12, 0.5);
  ctx.moveTo(0.4, 0.1);
  ctx.quadraticCurveTo(0.2, 0.24, 0.12, 0.5);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V24 - hourglass.

function v24(ctx) {
  kkShadow(ctx, 0.56, 0.18, 0.3);
  // Caps.
  kkRound(ctx, -0.46, -0.82, 0.92, 0.16, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.1);
  kkRound(ctx, -0.46, 0.62, 0.92, 0.16, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.1);
  // Glass.
  ctx.beginPath();
  ctx.moveTo(-0.34, -0.68);
  ctx.lineTo(0.34, -0.68);
  ctx.lineTo(0.07, -0.02);
  ctx.lineTo(0.34, 0.6);
  ctx.lineTo(-0.34, 0.6);
  ctx.lineTo(-0.07, -0.02);
  ctx.closePath();
  kkPaint(ctx, "rgba(206, 232, 236, 0.4)", KP.ink, 0.09);
  // Sand.
  ctx.beginPath();
  ctx.moveTo(-0.26, 0.56);
  ctx.lineTo(0.26, 0.56);
  ctx.quadraticCurveTo(0.1, 0.3, 0, 0.16);
  ctx.quadraticCurveTo(-0.1, 0.3, -0.26, 0.56);
  ctx.closePath();
  kkPaint(ctx, kkGrad(ctx, "sand", -0.3, 0, 0.3, 0.6,
    [[0, "#ecc778"], [1, "#b9862f"]]), null);
  ctx.fillStyle = "#d9a94a";
  for (const g of [[0, 0.05, 0.03], [-0.03, 0.16, 0.025],
    [0.04, 0.26, 0.02]]) {
    kkEllipse(ctx, g[0], g[1], g[2], g[2] * 1.4);
    ctx.fill();
  }
  // Glass highlight.
  ctx.strokeStyle = "rgba(255, 255, 255, 0.7)";
  ctx.lineWidth = 0.04;
  ctx.beginPath();
  ctx.moveTo(-0.26, -0.6);
  ctx.lineTo(-0.06, -0.1);
  ctx.moveTo(-0.28, 0.5);
  ctx.lineTo(-0.14, 0.34);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V25 - tattered banner.

function v25(ctx) {
  kkShadow(ctx, 0.78, 0.18, 0.3);
  // Pole with a spear finial and a crossbar the cloth hangs from.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(-0.56, 0.86);
    c.lineTo(-0.52, -0.78);
  }, 0.16, 0.1, kkWood(ctx));
  kkPoly(ctx, [[-0.62, -0.74], [-0.52, -1.0], [-0.42, -0.74]]);
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.08);
  kkRound(ctx, -0.54, -0.74, 1.24, 0.13, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.09);
  // Hanging cloth with a torn swallow tail.
  ctx.beginPath();
  ctx.moveTo(-0.46, -0.62);
  ctx.quadraticCurveTo(0.1, -0.68, 0.66, -0.6);
  ctx.lineTo(0.64, 0.34);
  ctx.lineTo(0.5, 0.18);
  ctx.lineTo(0.36, 0.46);
  ctx.lineTo(0.2, 0.22);
  ctx.lineTo(0.02, 0.44);
  ctx.lineTo(-0.16, 0.2);
  ctx.lineTo(-0.32, 0.4);
  ctx.lineTo(-0.46, 0.2);
  ctx.closePath();
  kkPaint(ctx, "#6e2b28", "rgba(36, 43, 51, 0.9)", 0.08);
  ctx.strokeStyle = "rgba(255, 255, 255, 0.2)";
  ctx.lineWidth = 0.045;
  ctx.beginPath();
  ctx.moveTo(-0.36, -0.5);
  ctx.quadraticCurveTo(0.08, -0.56, 0.56, -0.48);
  ctx.stroke();
  // Bone cross emblem.
  ctx.strokeStyle = "#e3d9c0";
  ctx.lineWidth = 0.1;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(0.08, -0.42);
  ctx.lineTo(0.08, 0.1);
  ctx.moveTo(-0.14, -0.24);
  ctx.lineTo(0.3, -0.24);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V26 - broken shield.

function v26(ctx) {
  kkShadow(ctx, 0.7, 0.18, 0.3);
  // Left half.
  ctx.beginPath();
  ctx.moveTo(-0.58, -0.64);
  ctx.lineTo(0, -0.6);
  ctx.lineTo(-0.06, 0.78);
  ctx.lineTo(-0.4, 0.26);
  ctx.closePath();
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.1);
  // Right half, knocked loose.
  ctx.save();
  ctx.translate(0.14, 0.1);
  ctx.rotate(0.16);
  ctx.beginPath();
  ctx.moveTo(-0.04, -0.66);
  ctx.lineTo(0.58, -0.7);
  ctx.lineTo(0.44, 0.2);
  ctx.lineTo(0.06, 0.5);
  ctx.closePath();
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.1);
  ctx.restore();
  // Iron rim and boss.
  ctx.strokeStyle = KP.ironDark;
  ctx.lineWidth = 0.07;
  ctx.beginPath();
  ctx.moveTo(-0.52, -0.56);
  ctx.lineTo(-0.44, 0.2);
  ctx.moveTo(-0.02, -0.54);
  ctx.lineTo(0.04, 0.2);
  ctx.stroke();
  ctx.fillStyle = kkIron(ctx);
  kkEllipse(ctx, -0.25, -0.06, 0.13, 0.13);
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.06);
}

// ---------------------------------------------------------------------
// V27 - raven on a branch.

function v27(ctx) {
  kkShadow(ctx, 0.72, 0.18, 0.3);
  // Branch.
  kkLimb(ctx, (c) => {
    c.beginPath();
    c.moveTo(-0.8, 0.62);
    c.quadraticCurveTo(0, 0.5, 0.82, 0.66);
  }, 0.14, 0.08, "#5b4126");
  // Body, head and tail in one silhouette.
  ctx.beginPath();
  ctx.moveTo(-0.72, 0.42);
  ctx.lineTo(-0.3, 0.06);
  ctx.bezierCurveTo(-0.22, -0.34, 0.06, -0.52, 0.28, -0.36);
  ctx.bezierCurveTo(0.4, -0.28, 0.42, -0.14, 0.34, -0.02);
  ctx.bezierCurveTo(0.44, 0.22, 0.3, 0.46, 0.02, 0.54);
  ctx.lineTo(-0.3, 0.56);
  ctx.closePath();
  kkPaint(ctx, "#2c343d", "rgba(180, 210, 220, 0.55)", 0.045);
  // Beak, eye and legs.
  kkPoly(ctx, [[0.32, -0.34], [0.62, -0.24], [0.32, -0.14]]);
  kkPaint(ctx, KP.emberDeep, null);
  ctx.fillStyle = KP.pale;
  kkEllipse(ctx, 0.24, -0.3, 0.05, 0.05);
  ctx.fill();
  ctx.fillStyle = KP.ink;
  kkEllipse(ctx, 0.24, -0.3, 0.025, 0.025);
  ctx.fill();
  ctx.strokeStyle = KP.ink;
  ctx.lineWidth = 0.045;
  ctx.beginPath();
  ctx.moveTo(-0.06, 0.52);
  ctx.lineTo(-0.04, 0.62);
  ctx.moveTo(0.1, 0.5);
  ctx.lineTo(0.12, 0.6);
  ctx.stroke();
}

// ---------------------------------------------------------------------
// V28 - bat.

function v28(ctx) {
  kkShadow(ctx, 0.8, 0.18, 0.3);
  ctx.beginPath();
  ctx.moveTo(0, -0.24);
  ctx.bezierCurveTo(-0.16, -0.5, -0.6, -0.68, -1.0, -0.5);
  ctx.bezierCurveTo(-0.86, -0.24, -0.82, -0.02, -0.92, 0.24);
  ctx.bezierCurveTo(-0.66, 0.06, -0.5, 0.16, -0.38, 0.44);
  ctx.bezierCurveTo(-0.24, 0.16, -0.1, 0.1, 0, 0.3);
  ctx.bezierCurveTo(0.1, 0.1, 0.24, 0.16, 0.38, 0.44);
  ctx.bezierCurveTo(0.5, 0.16, 0.66, 0.06, 0.92, 0.24);
  ctx.bezierCurveTo(0.82, -0.02, 0.86, -0.24, 1.0, -0.5);
  ctx.bezierCurveTo(0.6, -0.68, 0.16, -0.5, 0, -0.24);
  ctx.closePath();
  kkPaint(ctx, "#2b333c", "rgba(178, 212, 222, 0.6)", 0.05);
  // Ears and eyes.
  kkPoly(ctx, [[-0.14, -0.34], [-0.2, -0.62], [-0.02, -0.4]]);
  kkPaint(ctx, "#2b333c", KP.ink, 0.05);
  kkPoly(ctx, [[0.14, -0.34], [0.2, -0.62], [0.02, -0.4]]);
  kkPaint(ctx, "#2b333c", KP.ink, 0.05);
  ctx.fillStyle = KP.ember;
  kkEllipse(ctx, -0.07, -0.28, 0.035, 0.045);
  ctx.fill();
  kkEllipse(ctx, 0.07, -0.28, 0.035, 0.045);
  ctx.fill();
}

// ---------------------------------------------------------------------
// V29 - death cap mushrooms.

function v29(ctx) {
  kkShadow(ctx, 0.66, 0.16, 0.3);
  const caps = [
    [-0.28, 0.02, 0.3, 0.18],
    [0.16, -0.2, 0.36, 0.2],
    [0.42, 0.16, 0.2, 0.13]
  ];
  for (const c of caps) {
    // Stem.
    ctx.beginPath();
    ctx.moveTo(c[0] - 0.05, c[1] + 0.02);
    ctx.quadraticCurveTo(c[0] - 0.08, c[1] + 0.34, c[0] - 0.07, c[1] + 0.5);
    ctx.lineTo(c[0] + 0.07, c[1] + 0.5);
    ctx.quadraticCurveTo(c[0] + 0.08, c[1] + 0.3, c[0] + 0.05, c[1] + 0.02);
    ctx.closePath();
    kkPaint(ctx, kkGrad(ctx, "stem", 0, -1, 0, 1,
      [[0, "#f4eeda"], [1, "#c9c0a4"]]), KP.ink, 0.07);
    // Cap.
    ctx.beginPath();
    ctx.ellipse(c[0], c[1], c[2], c[3], 0, Math.PI, Math.PI * 2);
    ctx.closePath();
    kkPaint(ctx, kkGrad(ctx, "cap", 0, -1, 0, 1,
      [[0, "#9c4269"], [1, "#6d2749"]]), KP.ink, 0.08);
    // Spots.
    ctx.fillStyle = "rgba(245, 238, 220, 0.9)";
    for (const s of [[-0.42, -0.4], [-0.05, -0.5], [0.4, -0.34]]) {
      kkEllipse(ctx, c[0] + s[0] * c[2], c[1] + s[1] * c[3], 0.035, 0.028);
      ctx.fill();
    }
  }
  kkGrass(ctx, [-0.5, 0.54], 0.64, KP.moss, 0.16);
}

// ---------------------------------------------------------------------
// V30 - stump with an axe.

function v30(ctx) {
  kkShadow(ctx, 0.7, 0.18, 0.3);
  // Trunk.
  ctx.beginPath();
  ctx.moveTo(-0.42, 0.66);
  ctx.lineTo(-0.38, -0.34);
  ctx.lineTo(0.38, -0.34);
  ctx.lineTo(0.42, 0.66);
  ctx.closePath();
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.11);
  // Cut face with rings.
  kkEllipse(ctx, 0, -0.34, 0.38, 0.13);
  kkPaint(ctx, "#c9a374", KP.ink, 0.09);
  ctx.strokeStyle = KP.woodShade;
  ctx.lineWidth = 0.035;
  ctx.beginPath();
  ctx.ellipse(0, -0.34, 0.2, 0.07, 0, 0, Math.PI * 2);
  ctx.stroke();
  kkEllipse(ctx, 0, -0.34, 0.08, 0.03);
  ctx.stroke();
  // Axe.
  ctx.save();
  ctx.rotate(0.38);
  kkRound(ctx, 0.06, -0.95, 0.1, 0.72, 0.04);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.09);
  ctx.beginPath();
  ctx.moveTo(0.02, -0.36);
  ctx.lineTo(0.34, -0.44);
  ctx.lineTo(0.4, -0.22);
  ctx.lineTo(0.04, -0.14);
  ctx.closePath();
  kkPaint(ctx, kkIron(ctx), KP.ink, 0.09);
  ctx.restore();
  kkGrass(ctx, [-0.56, 0.56], 0.66, KP.moss, 0.15);
}

// ---------------------------------------------------------------------
// V31 - poppies over a mossy stone.

function v31(ctx, t) {
  kkShadow(ctx, 0.72, 0.18, 0.3);
  kkRound(ctx, -0.36, 0.06, 0.72, 0.5, 0.2);
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.1);
  ctx.fillStyle = KP.moss;
  kkEllipse(ctx, -0.26, 0.14, 0.16, 0.08);
  ctx.fill();
  kkEllipse(ctx, 0.22, 0.1, 0.12, 0.06);
  ctx.fill();
  const sway = Math.sin((t || 0) * 1.6) * 0.04;
  // Stems.
  for (const s of [[-0.32, -0.74], [0.16, -0.56]]) {
    ctx.strokeStyle = KP.leaf;
    ctx.lineWidth = 0.06;
    ctx.beginPath();
    ctx.moveTo(s[0] * 0.6, 0.14);
    ctx.quadraticCurveTo(s[0] * 0.8, -0.2, s[0] + sway * 2, s[1]);
    ctx.stroke();
  }
  // Poppy heads.
  for (const s of [[-0.32, -0.74, 0.21], [0.16, -0.56, 0.17]]) {
    ctx.fillStyle = "#b03a30";
    for (let i = 0; i < 5; i += 1) {
      const a = (i / 5) * Math.PI * 2 - 0.4;
      kkEllipse(ctx, s[0] + sway * 2 + Math.cos(a) * s[2] * 0.62,
        s[1] + Math.sin(a) * s[2] * 0.62, s[2] * 0.5, s[2] * 0.42, a);
      ctx.fill();
    }
    ctx.fillStyle = KP.ink;
    kkEllipse(ctx, s[0] + sway * 2, s[1], s[2] * 0.26, s[2] * 0.26);
    ctx.fill();
  }
  kkGrass(ctx, [-0.56, 0.5], 0.64, KP.moss, 0.16);
}

// ---------------------------------------------------------------------
// V32 - soul wisp.

function v32(ctx, t) {
  const time = t || 0;
  const bob = Math.sin(time * 1.7) * 0.05;
  kkShadow(ctx, 0.5, 0.14, 0.24);
  // The bone the soul left behind.
  kkBoneBar(ctx, -0.46, 0.62, 0.4, 0.54, kkBone(ctx), 0.07);
  // Glow.
  ctx.fillStyle = "rgba(150, 226, 226, 0.18)";
  kkEllipse(ctx, 0, -0.2 + bob, 0.5, 0.54);
  ctx.fill();
  ctx.fillStyle = "rgba(170, 235, 232, 0.3)";
  kkEllipse(ctx, 0, -0.2 + bob, 0.3, 0.34);
  ctx.fill();
  // The wisp: a flame with a curling tail.
  ctx.beginPath();
  ctx.moveTo(0, -0.72 + bob);
  ctx.bezierCurveTo(0.2, -0.42 + bob, 0.24, -0.16 + bob,
    0.1, 0.06 + bob);
  ctx.bezierCurveTo(0.02, 0.12 + bob, -0.1, 0.1 + bob, -0.16, 0.02 + bob);
  ctx.bezierCurveTo(-0.3, -0.14 + bob, -0.16, -0.3 + bob,
    -0.02, -0.42 + bob);
  ctx.bezierCurveTo(-0.1, -0.28 + bob, -0.1, -0.16 + bob,
    0.0, -0.08 + bob);
  ctx.bezierCurveTo(0.12, -0.2 + bob, 0.1, -0.46 + bob, 0, -0.72 + bob);
  ctx.closePath();
  kkPaint(ctx, kkGrad(ctx, "wisp", 0, -0.8, 0, 0.2,
    [[0, "#eafcf9"], [0.55, "#a5e4e0"], [1, "#5fb6b8"]]), null);
  // Two sparks.
  ctx.fillStyle = "#e8fbf8";
  kkEllipse(ctx, 0.36, -0.4 + bob, 0.05, 0.05);
  ctx.fill();
  kkEllipse(ctx, -0.3, -0.06 + bob, 0.035, 0.035);
  ctx.fill();
}

// ---------------------------------------------------------------------
// V33 - blood pool with a broken blade.

function v33(ctx) {
  kkShadow(ctx, 0.72, 0.18, 0.24);
  // Pool.
  ctx.beginPath();
  ctx.moveTo(-0.72, 0.34);
  ctx.bezierCurveTo(-0.8, 0.14, -0.44, 0.02, -0.16, 0.08);
  ctx.bezierCurveTo(0.1, -0.04, 0.5, 0.02, 0.66, 0.2);
  ctx.bezierCurveTo(0.84, 0.34, 0.5, 0.62, 0.1, 0.58);
  ctx.bezierCurveTo(-0.28, 0.66, -0.68, 0.56, -0.72, 0.34);
  ctx.closePath();
  kkPaint(ctx, kkBlood(ctx), KP.bloodDark, 0.07);
  ctx.strokeStyle = "rgba(255, 180, 160, 0.35)";
  ctx.lineWidth = 0.04;
  ctx.beginPath();
  ctx.moveTo(-0.5, 0.3);
  ctx.quadraticCurveTo(-0.2, 0.18, 0.1, 0.26);
  ctx.stroke();
  // Spatter.
  ctx.fillStyle = KP.blood;
  for (const d of [[-0.78, 0.02, 0.06], [-0.62, -0.12, 0.04],
    [0.74, 0.02, 0.05], [0.58, -0.16, 0.035], [0.2, 0.72, 0.05]]) {
    kkEllipse(ctx, d[0], d[1], d[2], d[2] * 0.8);
    ctx.fill();
  }
  // Broken blade across the pool.
  ctx.save();
  ctx.translate(0, 0.18);
  ctx.rotate(-0.22);
  kkPoly(ctx, [[-0.56, -0.1], [0.3, -0.18], [0.5, -0.02], [0.3, 0.1],
    [-0.56, 0.06]]);
  kkPaint(ctx, "#d3d8dd", KP.ink, 0.075);
  ctx.strokeStyle = "rgba(255, 255, 255, 0.8)";
  ctx.lineWidth = 0.035;
  ctx.beginPath();
  ctx.moveTo(-0.46, -0.03);
  ctx.lineTo(0.28, -0.1);
  ctx.stroke();
  kkRound(ctx, -0.74, -0.12, 0.2, 0.22, 0.05);
  kkPaint(ctx, kkWood(ctx), KP.ink, 0.08);
  ctx.restore();
}

// ---------------------------------------------------------------------
// V34 - ash pile with embers.

function v34(ctx, t) {
  const time = t || 0;
  kkShadow(ctx, 0.78, 0.18, 0.26);
  // Ash bed.
  ctx.beginPath();
  ctx.moveTo(-0.74, 0.56);
  ctx.quadraticCurveTo(0, 0.28, 0.74, 0.56);
  ctx.quadraticCurveTo(0, 0.72, -0.74, 0.56);
  ctx.closePath();
  kkPaint(ctx, "#6d6862", KP.ink, 0.07);
  // Charred logs, crossed.
  for (const log of [[-0.62, 0.18, 0.16], [0.6, 0.34, -0.2]]) {
    ctx.save();
    ctx.translate(log[1], log[0]);
    ctx.rotate(log[2]);
    kkRound(ctx, -0.44, -0.1, 0.88, 0.21, 0.09);
    kkPaint(ctx, kkGrad(ctx, "char", 0, -0.1, 0, 0.12,
      [[0, "#4a3a2a"], [1, "#241a12"]]), KP.ink, 0.06);
    ctx.restore();
  }
  // Ember cracks between the logs.
  for (let i = 0; i < 3; i += 1) {
    const pulse = 0.55 + 0.45 * Math.sin(time * 2.4 + i * 2.1);
    ctx.fillStyle = "rgba(246, 130, 40, " + (0.2 + 0.24 * pulse) + ")";
    kkEllipse(ctx, -0.36 + i * 0.34, 0.42, 0.17, 0.1);
    ctx.fill();
  }
  // Flames: three tongues, tallest in the middle.
  for (const f of [[-0.3, 0.5, 0.34], [0.02, 0.72, 0.44], [0.32, 0.46, 0.32]]) {
    const sway = 0.05 * Math.sin(time * 3 + f[0] * 9);
    const h = f[1] * (1 + 0.08 * Math.sin(time * 2.1 + f[0] * 5));
    ctx.beginPath();
    ctx.moveTo(f[0] - f[2], 0.34);
    ctx.quadraticCurveTo(f[0] - f[2] * 0.5, 0.34 - h * 0.6,
      f[0] + sway, 0.34 - h);
    ctx.quadraticCurveTo(f[0] + f[2] * 0.5, 0.34 - h * 0.6,
      f[0] + f[2], 0.34);
    ctx.closePath();
    kkPaint(ctx, kkGrad(ctx, "flame", 0, 0.4, 0, -0.6,
      [[0, "#c04f12"], [0.6, "#f6a83a"], [1, "#ffe9a8"]]), null);
  }
  // Bright core.
  ctx.fillStyle = "rgba(255, 233, 168, 0.85)";
  kkEllipse(ctx, 0.02, 0.2, 0.1, 0.14);
  ctx.fill();
  // Smoke curls.
  ctx.strokeStyle = "rgba(200, 198, 192, 0.35)";
  ctx.lineWidth = 0.05;
  ctx.lineCap = "round";
  for (const s of [-0.14, 0.1]) {
    ctx.beginPath();
    ctx.moveTo(s, -0.3);
    ctx.bezierCurveTo(s + 0.14, -0.5, s - 0.14, -0.68, s + 0.04, -0.9);
    ctx.stroke();
  }
}

// ---------------------------------------------------------------------
// V35 - rune stone.

function v35(ctx, t) {
  const time = t || 0;
  const glow = 0.55 + 0.45 * Math.sin(time * 1.9);
  kkShadow(ctx, 0.6, 0.16, 0.3);
  ctx.save();
  ctx.rotate(-0.06);
  ctx.beginPath();
  ctx.moveTo(-0.38, 0.6);
  ctx.lineTo(-0.42, -0.4);
  ctx.quadraticCurveTo(-0.38, -0.7, -0.08, -0.78);
  ctx.quadraticCurveTo(0.3, -0.72, 0.36, -0.42);
  ctx.lineTo(0.4, 0.6);
  ctx.closePath();
  kkPaint(ctx, kkStone(ctx), KP.ink, 0.11);
  ctx.restore();
  // The thorn rune, glowing.
  ctx.strokeStyle = "rgba(150, 235, 230, " + (0.25 + 0.2 * glow) + ")";
  ctx.lineWidth = 0.2;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(-0.04, -0.56);
  ctx.lineTo(-0.04, 0.34);
  ctx.moveTo(-0.04, -0.3);
  ctx.lineTo(0.18, -0.1);
  ctx.lineTo(-0.04, 0.1);
  ctx.stroke();
  ctx.strokeStyle = "rgba(186, 248, 244, " + (0.55 + 0.35 * glow) + ")";
  ctx.lineWidth = 0.07;
  ctx.beginPath();
  ctx.moveTo(-0.04, -0.56);
  ctx.lineTo(-0.04, 0.34);
  ctx.moveTo(-0.04, -0.3);
  ctx.lineTo(0.18, -0.1);
  ctx.lineTo(-0.04, 0.1);
  ctx.stroke();
  kkGrass(ctx, [-0.44, 0.42], 0.64, KP.moss, 0.15);
}

// ---------------------------------------------------------------------

const KILL_FAMILIES = [
  { id: "baseline", name: "Baseline" },
  { id: "skull", name: "Skulls" },
  { id: "grave", name: "Graves" },
  { id: "bone", name: "Bones" },
  { id: "fallen", name: "The fallen" },
  { id: "omen", name: "Omens & nature" },
  { id: "rune", name: "Magic" }
];

const KILL_MARKS = [
  { id: "V00", name: "Corpse face", family: "baseline",
    note: "The marker the map ships today: a gray disc with X eyes.",
    tags: ["flat", "cheapest"], draw: v00 },
  { id: "V01", name: "Skull", family: "skull",
    note: "The plain bone skull: the safest read of a dead mob.",
    tags: ["bone", "solo"], draw: v01 },
  { id: "V02", name: "Skull & crossbones", family: "skull",
    note: "The pirate classic: bones behind push the read at any zoom.",
    tags: ["bone", "bold"], draw: v02 },
  { id: "V03", name: "Impaled skull", family: "skull",
    note: "A blade planted through the skull: a fallen soldier read.",
    tags: ["bone", "iron"], draw: v03 },
  { id: "V04", name: "Helmed skull", family: "skull",
    note: "A rusted helm on the skull: the guard and soldier kills.",
    tags: ["bone", "iron"], draw: v04 },
  { id: "V05", name: "Candle skull", family: "skull",
    note: "A candle on the crown: the memorial reading, warm accent.",
    tags: ["bone", "flame", "animated"], animated: true, draw: v05 },
  { id: "V06", name: "Twin skulls", family: "skull",
    note: "Two skulls of a wiped pack: a heavier mark for a camp kill.",
    tags: ["bone", "pair"], draw: v06 },
  { id: "V07", name: "Cracked skull", family: "skull",
    note: "A split crown and a crack: reads as a violent, recent death.",
    tags: ["bone", "rough"], draw: v07 },
  { id: "V08", name: "Horned skull", family: "skull",
    note: "A beast skull with swept horns: non humanoid mobs.",
    tags: ["bone", "beast"], draw: v08 },
  { id: "V09", name: "Headstone", family: "grave",
    note: "The round topped stone with a carved cross: a quiet grave.",
    tags: ["stone", "grave"], draw: v09 },
  { id: "V10", name: "Celtic cross", family: "grave",
    note: "The ringed cross: more ornamental than a plain headstone.",
    tags: ["stone", "grave"], draw: v10 },
  { id: "V11", name: "Leaning slab", family: "grave",
    note: "An old cracked slab, moss at the foot: an abandoned grave.",
    tags: ["stone", "old"], draw: v11 },
  { id: "V12", name: "Wooden cross", family: "grave",
    note: "Planks and rope: a field grave, warmer than stone.",
    tags: ["wood", "grave"], draw: v12 },
  { id: "V13", name: "Ivy cross", family: "grave",
    note: "A stone cross the forest reclaimed: green accents.",
    tags: ["stone", "ivy"], draw: v13 },
  { id: "V14", name: "Mound & shovel", family: "grave",
    note: "A fresh mound with the shovel still in it: the freshest read.",
    tags: ["soil", "scene"], draw: v14 },
  { id: "V15", name: "Obelisk", family: "grave",
    note: "A tall monument: the strongest vertical silhouette.",
    tags: ["stone", "tall"], draw: v15 },
  { id: "V16", name: "Coffin lid", family: "grave",
    note: "The hexagonal lid: a coffin read without a body.",
    tags: ["wood", "grave"], draw: v16 },
  { id: "V17", name: "Crossed femurs", family: "bone",
    note: "Two bones in an X: the cheapest unmistakable bone mark.",
    tags: ["bone", "cheap"], draw: v17 },
  { id: "V18", name: "Rib cage", family: "bone",
    note: "A half buried rib cage: an organic, skeletal read.",
    tags: ["bone", "detailed"], draw: v18 },
  { id: "V19", name: "Bone heap", family: "bone",
    note: "A heap of bones and vertebrae: a much eaten camp.",
    tags: ["bone", "heap"], draw: v19 },
  { id: "V20", name: "Skeleton hand", family: "bone",
    note: "A hand out of the soil: the most dramatic bone read.",
    tags: ["bone", "soil"], draw: v20 },
  { id: "V21", name: "Cairn", family: "bone",
    note: "Stacked stones: earthy, calm, and very cheap to paint.",
    tags: ["stone", "cheap"], draw: v21 },
  { id: "V22", name: "Scythe", family: "fallen",
    note: "The reaper scythe: the death symbol with no body.",
    tags: ["iron", "wood"], draw: v22 },
  { id: "V23", name: "Reaper", family: "fallen",
    note: "The hooded silhouette: the darkest, most ominous mark.",
    tags: ["dark", "bold"], draw: v23 },
  { id: "V24", name: "Hourglass", family: "fallen",
    note: "Memento mori: time ran out for this mob.",
    tags: ["wood", "glass"], draw: v24 },
  { id: "V25", name: "Death banner", family: "fallen",
    note: "A torn banner on a spear: a battlefield read.",
    tags: ["cloth", "tall"], draw: v25 },
  { id: "V26", name: "Broken shield", family: "fallen",
    note: "A split shield: the gear of the fallen, no bones.",
    tags: ["wood", "iron"], draw: v26 },
  { id: "V27", name: "Raven", family: "omen",
    note: "The carrion bird: nature already at the body.",
    tags: ["dark", "beast"], draw: v27 },
  { id: "V28", name: "Bat", family: "omen",
    note: "A bat over the kill: night and cave grounds.",
    tags: ["dark", "beast"], draw: v28 },
  { id: "V29", name: "Death caps", family: "omen",
    note: "Poison mushrooms: an organic spot of colour on green.",
    tags: ["colour", "nature"], draw: v29 },
  { id: "V30", name: "Stump & axe", family: "omen",
    note: "A woodcutter's stump: the kill as an aftermath scene.",
    tags: ["wood", "scene"], draw: v30 },
  { id: "V31", name: "Poppies", family: "omen",
    note: "A mossy stone under poppies: the softest, warmest grave.",
    tags: ["colour", "soft"], draw: v31 },
  { id: "V32", name: "Soul wisp", family: "omen",
    note: "The escaping soul over a bone: luminous, animated.",
    tags: ["glow", "animated"], animated: true, draw: v32 },
  { id: "V33", name: "Blood pool", family: "omen",
    note: "A dark pool and a broken blade: the freshest kill read.",
    tags: ["blood", "scene"], draw: v33 },
  { id: "V34", name: "Ash & embers", family: "omen",
    note: "A burnt out pile with live embers: warm animated accent.",
    tags: ["ash", "animated"], animated: true, draw: v34 },
  { id: "V35", name: "Rune stone", family: "rune",
    note: "A carved thorn rune: a magic death, glowing faintly.",
    tags: ["glow", "animated"], animated: true, draw: v35 }
];
