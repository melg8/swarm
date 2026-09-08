/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// FightUI is the combat animation variant showcase of the
// -test-fight-ui-v1 mode: every column of the horizontal strip is one
// numbered visual idea of how the damage and the critical hits could
// read on the live map, every column stacks the four enemy positions
// (the enemy above, below, left and right of the hero) so the ideas
// are judged in every attack direction. All cells share one clock and
// one scripted fight loop: the hero hits the enemy, the enemy hits
// the hero, both land a critical, the health refills and the loop
// restarts - the same beat plays in every variant at the same moment,
// which makes the columns directly comparable. The background is the
// real world map tile the bot hunts on. The renderers are stateless
// functions of the effect age, so the whole showcase needs no event
// bookkeeping and never desyncs.
"use strict";

// The palette mirrors the live map combat layer (see map.js): the
// hero swings read light blue, the enemy swings orange red, the
// damage the hero deals is amber and the damage the hero takes is a
// hard red - the showcase variants judge ideas inside the same color
// language the live map already speaks.
const fightSwingSelfColor = "#7cc4ff";
const fightSwingMobColor = "#ff6b4a";
const fightDamageDealtColor = "#ffd25c";
const fightDamageTakenColor = "#ff5252";
const fightHeroColor = "#1a73e8";
const fightEnemyColor = "#d93025";
const fightCritColor = "#ffab00";
const fightRegenColor = "#34a853";

// FIGHT carries the whole scripted fight: the timing, the damages,
// the geometry of the demo cells and the world map crop used as the
// background. One shared clock drives every cell of every column.
const FIGHT = {
  // The loop: hit, take, crit, take crit, regen, restart.
  loopSec: 9,
  // The world units per CSS pixel of the demo cells - close to the
  // live map zoom so the unit markers keep their usual size.
  scale: 0.08,
  cellW: 260,
  cellH: 172,
  // The demo fight plays on the elven lands tile the bot actually
  // hunts (tile 21_19 of the map pyramid, the level 0 imagery for
  // the crispest crop at this zoom).
  heroWorldX: 46000,
  heroWorldY: 51000,
  tileLevel: 0,
  tileName: "21_19.jpg",
  tileZeroX: 20,
  tileZeroY: 18,
  tileWorldSize: 32768,
  // The scripted beat list: swings and damage landings with their
  // loop times. byHero is the attack direction, crit marks the
  // heavy hits, amount feeds the numbers and the health bars.
  timeline: [
    { at: 0.35, kind: "swing", byHero: true },
    { at: 0.9, kind: "damage", byHero: true, crit: false, amount: 38 },
    { at: 1.95, kind: "swing", byHero: false },
    { at: 2.5, kind: "damage", byHero: false, crit: false, amount: 26 },
    { at: 3.65, kind: "swing", byHero: true },
    { at: 4.2, kind: "damage", byHero: true, crit: true, amount: 95 },
    { at: 5.55, kind: "swing", byHero: false },
    { at: 6.1, kind: "damage", byHero: false, crit: true, amount: 68 },
    { at: 7.4, kind: "regen" }
  ],
  heroMaxHp: 220,
  enemyMaxHp: 160,
  enemyName: "Kaboo Orc",
  // The four vertical demo cells: where the enemy stands relative to
  // the hero (the world offset becomes the screen offset). The pair
  // shift keeps the health bars and the names of the top and the
  // bottom enemies inside the cell frame.
  positions: [
    { tag: "enemy top", dx: 0, dy: -840, heroShiftY: 24 },
    { tag: "enemy bottom", dx: 0, dy: 840, heroShiftY: -18 },
    { tag: "enemy left", dx: -900, dy: 0 },
    { tag: "enemy right", dx: 900, dy: 0 }
  ]
};

// fightEaseOutQuad eases t out: fast at the start, settled at the
// end.
function fightEaseOutQuad(t) {
  return 1 - (1 - t) * (1 - t);
}

// fightEaseOutBack eases t out with a small overshoot: the pop of
// the punched in numbers.
function fightEaseOutBack(t) {
  const c = 1.70158;

  return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
}

// fightHaloText draws one text with the dark halo the map labels
// use, so the numbers and captions read over the imagery and over
// both theme fills alike.
function fightHaloText(
  ctx, text, x, y, size, color, alpha, align, weight
) {
  ctx.save();
  const font = (weight || "700") + " " + size.toFixed(1) + "px " +
    (getComputedStyle(document.documentElement)
      .getPropertyValue("--sans").trim() || "sans-serif");
  ctx.font = font;
  ctx.textAlign = align || "center";
  ctx.textBaseline = "middle";
  ctx.globalAlpha = alpha;
  ctx.lineWidth = 3;
  ctx.strokeStyle = "rgba(15, 18, 22, 0.78)";
  ctx.lineJoin = "round";
  ctx.strokeText(text, x, y);
  ctx.fillStyle = color;
  ctx.fillText(text, x, y);
  ctx.restore();
}

// fightRing strokes one circle.
function fightRing(ctx, x, y, r, color, alpha, lineWidth) {
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.strokeStyle = color;
  ctx.lineWidth = lineWidth;
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.stroke();
  ctx.restore();
}

// fightStarPath traces one pointed star (n spikes) around x, y.
function fightStarPath(ctx, x, y, spikes, outer, inner, rotation) {
  ctx.beginPath();
  for (let i = 0; i < spikes * 2; i++) {
    const r = i % 2 === 0 ? outer : inner;
    const a = rotation + (i / (spikes * 2)) * Math.PI * 2;
    const px = x + Math.cos(a) * r;
    const py = y + Math.sin(a) * r;
    if (i === 0) {
      ctx.moveTo(px, py);
    } else {
      ctx.lineTo(px, py);
    }
  }
  ctx.closePath();
}

// fightFillStar draws one filled star with a dark halo stroke.
function fightFillStar(ctx, x, y, spikes, outer, inner, rotation, color, alpha) {
  ctx.save();
  ctx.globalAlpha = alpha;
  fightStarPath(ctx, x, y, spikes, outer, inner, rotation);
  ctx.fillStyle = color;
  ctx.fill();
  ctx.lineWidth = 1.4;
  ctx.strokeStyle = "rgba(15, 18, 22, 0.7)";
  ctx.stroke();
  ctx.restore();
}

// fightUnit draws one fight participant in the visual language of
// the live map: a filled circle with the outline, the look tick
// pointing at the opponent, the self pulse ring for the hero and a
// health bar with the name above the marker.
function fightUnit(ctx, unit, lookAt, opts) {
  const angle = Math.atan2(lookAt.y - unit.y, lookAt.x - unit.x);
  ctx.save();
  // The pulse ring of the hero and the targeting ring of the enemy.
  if (opts.hero) {
    ctx.globalAlpha = 0.45;
    ctx.strokeStyle = fightHeroColor;
    ctx.lineWidth = 1.3;
    ctx.beginPath();
    ctx.arc(unit.x, unit.y, unit.radius + 3
      + 1.4 * Math.sin(opts.nowMs / 480), 0, Math.PI * 2);
    ctx.stroke();
    ctx.globalAlpha = 1;
  } else if (opts.inCombat) {
    ctx.globalAlpha = 0.5;
    ctx.strokeStyle = fightEnemyColor;
    ctx.lineWidth = 1.2;
    ctx.setLineDash([3, 2]);
    ctx.beginPath();
    ctx.arc(unit.x, unit.y, unit.radius + 3.5, 0, Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.globalAlpha = 1;
  }
  // The circle body.
  ctx.beginPath();
  ctx.arc(unit.x, unit.y, unit.radius, 0, Math.PI * 2);
  ctx.fillStyle = opts.hero ? fightHeroColor : fightEnemyColor;
  ctx.fill();
  ctx.lineWidth = 1.3;
  ctx.strokeStyle = "#39424e";
  ctx.stroke();
  // The look tick toward the opponent.
  const tx = unit.x + Math.cos(angle) * (unit.radius + 3.5);
  const ty = unit.y + Math.sin(angle) * (unit.radius + 3.5);
  ctx.strokeStyle = "#ffffff";
  ctx.lineWidth = 2;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(unit.x + Math.cos(angle) * unit.radius * 0.4,
    unit.y + Math.sin(angle) * unit.radius * 0.4);
  ctx.lineTo(tx, ty);
  ctx.stroke();
  ctx.restore();
}

// fightHpBar draws the health bar with the name above one unit. The
// fill color degrades from green over orange to red with the health
// share, exactly like the HUD bars of the live panels.
function fightHpBar(ctx, unit, hp, maxHp, label, opts) {
  const w = 40;
  const h = 4.5;
  const x = unit.x - w / 2;
  const y = unit.y - unit.radius - 13;
  const share = Math.max(0, Math.min(1, hp / maxHp));
  ctx.save();
  ctx.globalAlpha = 0.9;
  ctx.fillStyle = "rgba(15, 18, 22, 0.62)";
  ctx.fillRect(x - 1, y - 1, w + 2, h + 2);
  let fill = "#34a853";
  if (share < 0.28) {
    fill = "#d93025";
  } else if (share < 0.55) {
    fill = "#e37400";
  }
  ctx.fillStyle = fill;
  ctx.fillRect(x, y, w * share, h);
  ctx.globalAlpha = 1;
  fightHaloText(ctx, label, unit.x, y - 7.5, 8.5,
    opts.hero ? "#ffffff" : "#ffb4a6", 0.95, "center");
  if (opts.regen) {
    ctx.globalAlpha = 0.4 + 0.3 * Math.sin(opts.nowMs / 160);
    ctx.strokeStyle = fightRegenColor;
    ctx.lineWidth = 1.2;
    ctx.strokeRect(x - 2.5, y - 2.5, w + 5, h + 5);
  }
  ctx.restore();
}

// fightDefaultSwing is the subtle default attack hint the variants
// without a styled swing use: a short fading dash from the attacker
// toward the target, so the attack still reads.
function fightDefaultSwing(ctx, v, ev, p) {
  const from = ev.byHero ? v.hero : v.enemy;
  const to = ev.byHero ? v.enemy : v.hero;
  const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
  const ang = Math.atan2(to.y - from.y, to.x - from.x);
  const reach = 6 + 9 * fightEaseOutQuad(Math.min(1, p / 0.6));
  ctx.save();
  ctx.globalAlpha = 0.6 * (1 - p);
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(from.x + Math.cos(ang) * (v.hero.radius + 2),
    from.y + Math.sin(ang) * (v.hero.radius + 2));
  ctx.lineTo(from.x + Math.cos(ang) * (v.hero.radius + 2 + reach),
    from.y + Math.sin(ang) * (v.hero.radius + 2 + reach));
  ctx.stroke();
  ctx.restore();
}

// VARIANTS is the showcase catalog: one entry per visual idea, every
// entry numbered for the pick. swing and damage are stateless
// renderers of the effect age fraction p (0..1); frame is an optional
// persistent overlay (banners, combo counters); shake and zoom are
// optional cell transforms (the whole cell shakes or punches in on
// the impact).
const VARIANTS = [
  {
    num: "01",
    name: "Floating numbers",
    note: "как сейчас на карте: число всплывает и тает, крит крупнее и с меткой",
    swingLife: 0.55,
    damageLife: 1.15,
    // The live map swing: a windup arc, a tapered streak toward the
    // target and an impact starburst.
    swing(ctx, v, ev, p) {
      const from = ev.byHero ? v.hero : v.enemy;
      const to = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
      const dx = to.x - from.x;
      const dy = to.y - from.y;
      const dist = Math.hypot(dx, dy);
      if (dist < 4) { return; }
      const ux = dx / dist;
      const uy = dy / dist;
      ctx.save();
      if (p < 0.45) {
        const w = fightEaseOutQuad(p / 0.45);
        const angle = Math.atan2(uy, ux);
        ctx.globalAlpha = 0.65 * (1 - w);
        ctx.strokeStyle = color;
        ctx.lineWidth = 2.2;
        ctx.lineCap = "round";
        ctx.beginPath();
        ctx.arc(from.x, from.y, 8 + 6 * w,
          angle - 1.1 + 0.5 * w, angle - 0.25 + 0.5 * w);
        ctx.stroke();
      }
      if (p < 0.62) {
        const travel = fightEaseOutQuad(Math.min(1, p / 0.62));
        const reach = Math.min(dist, 14 + dist * 0.25) * travel;
        const head = 6 + 13 * travel;
        const tail = Math.max(2, reach - head);
        ctx.globalAlpha = 0.9 * (1 - travel * 0.45);
        ctx.strokeStyle = color;
        ctx.lineCap = "round";
        ctx.lineWidth = 3;
        ctx.beginPath();
        ctx.moveTo(from.x + ux * tail, from.y + uy * tail);
        ctx.lineTo(from.x + ux * reach, from.y + uy * reach);
        ctx.stroke();
      }
      if (p > 0.5) {
        const burst = (p - 0.5) / 0.5;
        const len = 5 + 8 * fightEaseOutQuad(burst);
        ctx.globalAlpha = (1 - burst) * 0.9;
        ctx.strokeStyle = "#ffffff";
        ctx.lineWidth = 1.6;
        ctx.lineCap = "round";
        for (let i = 0; i < 6; i++) {
          const a = (i / 6) * Math.PI * 2 + burst * 0.6;
          const r0 = 2.5 + 2 * burst;
          ctx.beginPath();
          ctx.moveTo(to.x + Math.cos(a) * r0, to.y + Math.sin(a) * r0);
          ctx.lineTo(to.x + Math.cos(a) * (r0 + len),
            to.y + Math.sin(a) * (r0 + len));
          ctx.stroke();
        }
      }
      ctx.restore();
    },
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const rise = fightEaseOutQuad(p) * 26;
      const alpha = p < 0.75 ? 1 : 1 - (p - 0.75) / 0.25;
      const scale = p < 0.14 ? fightEaseOutBack(p / 0.14) : 1;
      const size = (ev.crit ? 17 : 12) + Math.sqrt(ev.amount) * 0.55;
      fightRing(ctx, target.x, target.y,
        (5 + 13 * p) * (ev.crit ? 1.3 : 1), color,
        alpha * 0.5 * (1 - p), 1.6);
      ctx.save();
      ctx.translate(target.x, target.y - 10 - rise);
      ctx.scale(scale * (ev.crit ? 1.25 : 1), scale * (ev.crit ? 1.25 : 1));
      fightHaloText(ctx, "-" + ev.amount, 0, 0, size, color, alpha);
      if (ev.crit) {
        fightHaloText(ctx, "CRIT", 0, -size * 0.85, 8.5,
          fightCritColor, alpha);
      }
      ctx.restore();
    }
  },
  {
    num: "02",
    name: "Comic pop",
    note: "звезда удара + число с рывком, крит трясёт всю клетку",
    swingLife: 0.5,
    damageLife: 1.0,
    swing(ctx, v, ev, p) {
      // The motion smear: three fading arcs behind the attacker.
      const from = ev.byHero ? v.hero : v.enemy;
      const to = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
      const ang = Math.atan2(to.y - from.y, to.x - from.x);
      ctx.save();
      ctx.strokeStyle = color;
      ctx.lineCap = "round";
      for (let i = 0; i < 3; i++) {
        const w = p - i * 0.12;
        if (w < 0 || w > 0.5) { continue; }
        ctx.globalAlpha = 0.55 * (1 - w / 0.5);
        ctx.lineWidth = 2.6 - i * 0.6;
        ctx.beginPath();
        ctx.arc(from.x, from.y, from.radius + 3 + i * 3,
          ang - 1.3, ang - 0.4);
        ctx.stroke();
      }
      ctx.restore();
    },
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const pop = fightEaseOutBack(Math.min(1, p / 0.3));
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The impact star behind the number.
      fightFillStar(ctx, target.x, target.y, ev.crit ? 10 : 8,
        (12 + 16 * pop) * (ev.crit ? 1.2 : 1),
        (5 + 7 * pop) * (ev.crit ? 1.2 : 1),
        p * 1.5, ev.crit ? fightCritColor : color, alpha * 0.85);
      fightRing(ctx, target.x, target.y, 6 + 20 * p, color,
        alpha * 0.45 * (1 - p), 2);
      // The punched in number with a shake of the first frames.
      const jitter = p < 0.35
        ? (1 - p / 0.35) * (ev.crit ? 3 : 1.6) : 0;
      const jx = Math.sin(p * 60) * jitter;
      const jy = Math.cos(p * 53) * jitter;
      ctx.save();
      ctx.translate(target.x + jx, target.y - 14 - fightEaseOutQuad(p) * 18 + jy);
      ctx.scale(pop * (ev.crit ? 1.5 : 1.2), pop * (ev.crit ? 1.5 : 1.2));
      fightHaloText(ctx, (ev.crit ? "CRIT " : "") + "-" + ev.amount,
        0, 0, ev.crit ? 16 : 13, ev.crit ? fightCritColor : color, alpha);
      ctx.restore();
    },
    shake(ev, p) {
      if (p > 0.4 || !ev.crit) { return { x: 0, y: 0 }; }
      const decay = 1 - p / 0.4;

      return { x: Math.sin(p * 70) * 3.4 * decay, y: 0 };
    }
  },
  {
    num: "03",
    name: "Slash",
    note: "белый разрез поперёк цели, крит — крест из двух разрезов",
    swingLife: 0.5,
    damageLife: 1.05,
    swing(ctx, v, ev, p) {
      // The attacker lunges: a fading ghost circle dashes toward the
      // target and back.
      const from = ev.byHero ? v.hero : v.enemy;
      const to = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
      const lunge = Math.sin(Math.min(1, p / 0.7) * Math.PI);
      const dx = (to.x - from.x) * 0.16 * lunge;
      const dy = (to.y - from.y) * 0.16 * lunge;
      ctx.save();
      ctx.globalAlpha = 0.5 * lunge;
      ctx.fillStyle = color;
      ctx.beginPath();
      ctx.arc(from.x + dx, from.y + dy, from.radius * 0.8, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
      fightDefaultSwing(ctx, v, ev, p);
    },
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The slash sweeps in fast then melts: a tapered bright line
      // across the target, the crit adds the crossing second slash.
      const grow = Math.min(1, p / 0.2);
      const reach = (16 + Math.sqrt(ev.amount) * 1.5) * fightEaseOutQuad(grow);
      ctx.save();
      ctx.lineCap = "round";
      const drawSlash = (angle) => {
        const cx = target.x + Math.cos(angle + Math.PI / 2) * 0;
        const cy = target.y;
        const x0 = cx + Math.cos(angle) * reach;
        const y0 = cy + Math.sin(angle) * reach;
        const x1 = cx - Math.cos(angle) * reach;
        const y1 = cy - Math.sin(angle) * reach;
        ctx.globalAlpha = alpha * (1 - p * 0.8);
        ctx.strokeStyle = "#ffffff";
        ctx.lineWidth = 3.2 * (1 - p * 0.5);
        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.stroke();
        ctx.globalAlpha = alpha * 0.5 * (1 - p);
        ctx.strokeStyle = color;
        ctx.lineWidth = 5.5 * (1 - p * 0.4);
        ctx.beginPath();
        ctx.moveTo(x0, y0);
        ctx.lineTo(x1, y1);
        ctx.stroke();
      };
      drawSlash(-Math.PI / 4 + p * 0.35);
      if (ev.crit) {
        drawSlash(Math.PI / 4 - p * 0.35);
      }
      ctx.restore();
      fightHaloText(ctx, "-" + ev.amount, target.x,
        target.y - 16 - fightEaseOutQuad(p) * 14,
        ev.crit ? 14.5 : 11.5, ev.crit ? fightCritColor : color, alpha);
    }
  },
  {
    num: "04",
    name: "Spark spray",
    note: "искры летят из цели в сторону удара, крит — золотой сноп",
    swingLife: 0.5,
    damageLife: 1.0,
    swing: fightDefaultSwing,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const count = ev.crit ? 22 : 12;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The spray direction: the hit continues through the target
      // (attacker -> target), the sparks scatter around it.
      const from = ev.byHero ? v.hero : v.enemy;
      const baseAng = Math.atan2(target.y - from.y, target.x - from.x);
      ctx.save();
      ctx.lineCap = "round";
      for (let i = 0; i < count; i++) {
        // Deterministic per particle (i): a spread angle, a speed
        // and a gravity arc - no state, the same sparks every frame.
        const seed = (i * 47 + ev.amount * 13) % 100 / 100;
        const spread = (seed - 0.5) * 1.5;
        const ang = baseAng + spread;
        const speed = 26 + seed * 40 + (ev.crit ? 16 : 0);
        const fly = fightEaseOutQuad(p);
        const px = target.x + Math.cos(ang) * speed * fly;
        const py = target.y + Math.sin(ang) * speed * fly
          + 34 * p * p;
        const size = 1 + seed * 1.6 + (ev.crit ? 0.6 : 0);
        ctx.globalAlpha = alpha * (1 - p * 0.85);
        ctx.fillStyle = ev.crit && i % 3 === 0 ? "#ffffff" : color;
        ctx.beginPath();
        ctx.arc(px, py, size, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
      fightRing(ctx, target.x, target.y, 4 + 10 * p, color,
        alpha * 0.5 * (1 - p), 1.6);
      fightHaloText(ctx, "-" + ev.amount, target.x,
        target.y - 14 - fightEaseOutQuad(p) * 12,
        ev.crit ? 14 : 11, ev.crit ? fightCritColor : color, alpha);
    }
  },
  {
    num: "05",
    name: "Shockwave",
    note: "ударная волна расходится кольцами, крит — три кольца",
    swingLife: 0.5,
    damageLife: 1.2,
    // The charge up: a shrinking ring gathers at the attacker.
    swing(ctx, v, ev, p) {
      const from = ev.byHero ? v.hero : v.enemy;
      const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
      const gather = 1 - fightEaseOutQuad(Math.min(1, p / 0.8));
      fightRing(ctx, from.x, from.y, 6 + 16 * gather, color,
        0.5 * (1 - p), 2);
      fightDefaultSwing(ctx, v, ev, p);
    },
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      const rings = ev.crit ? 3 : 1;
      ctx.save();
      for (let i = 0; i < rings; i++) {
        const phase = Math.max(0, p - i * 0.14);
        const grow = fightEaseOutQuad(phase);
        const r = 4 + grow * (26 + Math.sqrt(ev.amount) * 1.1)
          * (ev.crit ? 1.15 : 1);
        const ringAlpha = alpha * (1 - phase) * 0.9;
        fightRing(ctx, target.x, target.y, r,
          i % 2 === 1 ? "#ffffff" : color, ringAlpha, 2.4 - i * 0.5);
        if (ev.crit && i === 0) {
          fightRing(ctx, target.x, target.y, r * 0.66,
            fightCritColor, ringAlpha * 0.8, 1.4);
        }
      }
      ctx.restore();
      fightHaloText(ctx, "-" + ev.amount, target.x,
        target.y - 14 - fightEaseOutQuad(p) * 12,
        ev.crit ? 14 : 11, ev.crit ? fightCritColor : color, alpha);
    }
  },
  {
    num: "06",
    name: "HP bar chunk",
    note: "урон отрывается куском полоски HP и падает, крит — золотой кусок",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 1.25,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.crit ? fightCritColor
        : (ev.byHero ? fightDamageDealtColor : fightDamageTakenColor);
      const alpha = p < 0.75 ? 1 : 1 - (p - 0.75) / 0.25;
      // The torn off chunk: a piece proportional to the damage share
      // detaches from the health bar position, falls with gravity
      // and melts.
      const barW = 40;
      const barY = target.y - target.radius - 13;
      const share = Math.min(1, ev.amount / target.maxHp);
      const chunkW = Math.max(4, barW * share * 2.2);
      const fall = p * p * 30;
      const sway = Math.sin(p * 5) * 4;
      ctx.save();
      ctx.globalAlpha = alpha;
      ctx.fillStyle = color;
      ctx.fillRect(target.x - chunkW / 2 + sway,
        barY + 3 + fall, chunkW, 4.5);
      ctx.strokeStyle = "rgba(15, 18, 22, 0.6)";
      ctx.lineWidth = 1;
      ctx.strokeRect(target.x - chunkW / 2 + sway,
        barY + 3 + fall, chunkW, 4.5);
      // The cut flash on the bar itself.
      ctx.globalAlpha = alpha * (1 - p) * 0.8;
      ctx.strokeStyle = "#ffffff";
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.moveTo(target.x + chunkW / 2, barY);
      ctx.lineTo(target.x + chunkW / 2, barY + 4.5);
      ctx.stroke();
      ctx.restore();
      fightHaloText(ctx, "-" + ev.amount, target.x,
        barY - 9 - fightEaseOutQuad(p) * 10, 10.5, color, alpha);
    }
  }
];

// The second half of the catalog continues with the projectile, the
// cinematic and the HUD flavored ideas.
VARIANTS.push(
  {
    num: "07",
    name: "Arrow",
    note: "удар — летящая стрела, крит — огненная стрела со шлейфом",
    swingLife: 0.6,
    damageLife: 0.9,
    swing(ctx, v, ev, p) {
      const from = ev.byHero ? v.hero : v.enemy;
      const to = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightSwingSelfColor : fightSwingMobColor;
      const dx = to.x - from.x;
      const dy = to.y - from.y;
      const dist = Math.hypot(dx, dy);
      const travel = Math.min(1, p / 0.85);
      const headX = from.x + dx * travel;
      const headY = from.y + dy * travel;
      const ang = Math.atan2(dy, dx);
      ctx.save();
      // The crit burns: a particle trail behind the arrowhead.
      if (ev.crit) {
        for (let i = 1; i <= 7; i++) {
          const tp = Math.max(0, travel - i * 0.07);
          const tx = from.x + dx * tp;
          const ty = from.y + dy * tp;
          ctx.globalAlpha = 0.65 * (1 - i / 8) * (1 - p * 0.4);
          ctx.fillStyle = i % 2 === 0 ? fightCritColor : "#ff7043";
          ctx.beginPath();
          ctx.arc(tx, ty, 3.5 - i * 0.35, 0, Math.PI * 2);
          ctx.fill();
        }
      }
      // The arrow: shaft, head triangle and feather ticks.
      const shaft = 13;
      ctx.globalAlpha = 0.95;
      ctx.strokeStyle = ev.crit ? fightCritColor : color;
      ctx.lineWidth = 2;
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.moveTo(headX - Math.cos(ang) * shaft,
        headY - Math.sin(ang) * shaft);
      ctx.lineTo(headX, headY);
      ctx.stroke();
      ctx.fillStyle = "#ffffff";
      ctx.beginPath();
      ctx.moveTo(headX + Math.cos(ang) * 5,
        headY + Math.sin(ang) * 5);
      ctx.lineTo(headX + Math.cos(ang + 2.5) * 4,
        headY + Math.sin(ang + 2.5) * 4);
      ctx.lineTo(headX + Math.cos(ang - 2.5) * 4,
        headY + Math.sin(ang - 2.5) * 4);
      ctx.closePath();
      ctx.fill();
      ctx.strokeStyle = color;
      ctx.lineWidth = 1.4;
      for (const side of [-1, 1]) {
        ctx.beginPath();
        ctx.moveTo(headX - Math.cos(ang) * shaft,
          headY - Math.sin(ang) * shaft);
        ctx.lineTo(headX - Math.cos(ang) * shaft
          + Math.cos(ang + side * 0.7) * 5,
          headY - Math.sin(ang) * shaft
          + Math.sin(ang + side * 0.7) * 5);
        ctx.stroke();
      }
      ctx.restore();
    },
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The impact pop.
      fightFillStar(ctx, target.x, target.y, 5,
        7 + 7 * fightEaseOutQuad(Math.min(1, p / 0.4)),
        3 + 3 * p, p * 2, "#ffffff", alpha * (1 - p));
      fightRing(ctx, target.x, target.y, 4 + 12 * p, color,
        alpha * 0.5 * (1 - p), 1.8);
      fightHaloText(ctx, "-" + ev.amount, target.x,
        target.y - 15 - fightEaseOutQuad(p) * 13,
        ev.crit ? 14 : 11.5, ev.crit ? fightCritColor : color, alpha);
    }
  },
  {
    num: "08",
    name: "Hit-stop",
    note: "кинематографично: стоп-кадр с наездом камеры, крит дольше и с вспышкой",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 1.3,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The punched number: a heavy scale punch with the overshoot.
      const punch = fightEaseOutBack(Math.min(1, p / 0.24));
      ctx.save();
      ctx.translate(target.x, target.y - 13 - fightEaseOutQuad(p) * 12);
      ctx.scale(punch * (ev.crit ? 1.6 : 1.3), punch * (ev.crit ? 1.6 : 1.3));
      fightHaloText(ctx, "-" + ev.amount, 0, 0,
        ev.crit ? 16 : 13, ev.crit ? fightCritColor : color, alpha);
      ctx.restore();
      // The white flash outline of the hit frame.
      if (p < 0.3) {
        ctx.save();
        ctx.globalAlpha = (1 - p / 0.3) * 0.9;
        ctx.strokeStyle = "#ffffff";
        ctx.lineWidth = 2.2;
        ctx.beginPath();
        ctx.arc(target.x, target.y, target.radius + 2.5, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
      }
    },
    // The cinematic zoom punch of the whole cell around the impact.
    zoom(ev, p) {
      const hold = ev.crit ? 0.5 : 0.3;
      if (p > hold) { return null; }
      const k = p / hold;
      const scale = 1 + Math.sin(k * Math.PI) * (ev.crit ? 0.075 : 0.045);

      return { scale };
    },
    // The crit whites out the cell edges for the frozen instant.
    vignette(ev, p) {
      if (!ev.crit || p > 0.45) { return null; }
      const color = "255,255,255";
      const alpha = 0.5 * (1 - p / 0.45);

      return { color, alpha };
    }
  },
  {
    num: "09",
    name: "Cell flash",
    note: "вся клетка вспыхивает: синим — свой удар, красным — полученный",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 0.85,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      fightHaloText(ctx, "-" + ev.amount, target.x,
        target.y - 15 - fightEaseOutQuad(p) * 12,
        ev.crit ? 13.5 : 11, ev.crit ? fightCritColor : color, alpha);
    },
    // The flash covers the whole cell: a colored vignette pulse, the
    // crit pulses twice.
    vignette(ev, p) {
      const crit = ev.crit;
      let k = p;
      if (crit) {
        // The double pulse: two quarter second beats.
        k = p < 0.5 ? p * 2 : Math.max(0, (p - 0.5) * 2);
      }
      const flash = Math.sin(Math.min(1, k / 0.7) * Math.PI);
      if (flash <= 0.01) { return null; }
      const color = ev.byHero ? "120,180,255" : "255,70,70";
      const alpha = flash * (crit ? 0.34 : 0.22);

      return { color, alpha, crit };
    }
  },
  {
    num: "10",
    name: "Dizzy stars",
    note: "звёздочки кружат над головой пострадавшего, крит — пять золотых",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 1.5,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.72 ? 1 : 1 - (p - 0.72) / 0.28;
      const count = ev.crit ? 5 : 3;
      const headY = target.y - target.radius - 16;
      ctx.save();
      for (let i = 0; i < count; i++) {
        // The orbit: the stars circle the head, wobbling in with the
        // first frames of the effect.
        const appear = Math.min(1, p / 0.25);
        const a = p * (ev.crit ? 6.5 : 5) + (i / count) * Math.PI * 2;
        const orbit = (10 + 2 * Math.sin(p * 9 + i)) * appear;
        const sx = target.x + Math.cos(a) * orbit;
        const sy = headY + Math.sin(a) * orbit * 0.45;
        fightFillStar(ctx, sx, sy, 5,
          (ev.crit ? 4.6 : 3.6) * appear,
          (ev.crit ? 2.1 : 1.6) * appear,
          a + p * 3, ev.crit ? fightCritColor : "#ffe082",
          alpha * (0.55 + 0.45 * Math.sin(a * 2)));
      }
      ctx.restore();
      fightHaloText(ctx, "-" + ev.amount, target.x,
        headY - 12 - fightEaseOutQuad(p) * 10,
        ev.crit ? 14 : 11, color, alpha);
    }
  },
  {
    num: "11",
    name: "Arcade banner",
    note: "баннер сверху клетки: HIT / CRITICAL, слайдом как в аркаде",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 1.3,
    damage(ctx, v, ev, p) {
      // Only the small impact tick on the unit itself.
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      fightRing(ctx, target.x, target.y, 4 + 9 * p, color,
        alpha * 0.5 * (1 - p), 1.6);
    },
    // The banner slides in at the cell top and out: the crit banner
    // is gold and punches in harder. Reads the newest active damage
    // event out of the cell state.
    frame(ctx, v, state) {
      const active = state.damage;
      if (!active) { return; }
      const ev = active.ev;
      const p = active.p;
      const alpha = p < 0.15 ? p / 0.15
        : (p > 0.8 ? (1 - p) / 0.2 : 1);
      if (alpha <= 0) { return; }
      const slide = fightEaseOutQuad(Math.min(1, p / 0.3));
      const w = v.w - 36;
      const h = 17;
      const x = 18;
      const y = 6 - (1 - slide) * 14;
      const crit = ev.crit;
      const color = crit ? fightCritColor
        : (ev.byHero ? "#4da3ff" : "#d93025");
      ctx.save();
      ctx.globalAlpha = alpha * 0.92;
      ctx.fillStyle = "rgba(15, 18, 22, 0.85)";
      ctx.fillRect(x, y, w, h);
      ctx.strokeStyle = color;
      ctx.lineWidth = 1.4;
      ctx.strokeRect(x, y, w, h);
      // The left accent bar of the banner.
      ctx.fillStyle = color;
      ctx.fillRect(x, y, 3, h);
      const label = (crit ? "CRITICAL  " : (ev.byHero ? "HIT  " : "TAKEN  "))
        + "-" + ev.amount;
      const pop = crit ? fightEaseOutBack(Math.min(1, p / 0.22)) : 1;
      ctx.translate(x + 10, y + h / 2 + 0.5);
      ctx.scale(pop, pop);
      fightHaloText(ctx, label, 0, 0, crit ? 11.5 : 10,
        crit ? fightCritColor : "#ffffff", alpha, "left");
      ctx.restore();
    }
  },
  {
    num: "12",
    name: "Combo counter",
    note: "счётчик серии ударов, числа набегают снизу вверх, крит золотом",
    swingLife: 0.5,
    swing: fightDefaultSwing,
    damageLife: 1.2,
    damage(ctx, v, ev, p) {
      const target = ev.byHero ? v.enemy : v.hero;
      const color = ev.byHero ? fightDamageDealtColor : fightDamageTakenColor;
      const alpha = p < 0.7 ? 1 : 1 - (p - 0.7) / 0.3;
      // The number counts up to the amount first, then rides up.
      const countUp = Math.min(1, p / 0.45);
      const shown = Math.round(ev.amount * countUp);
      const rise = p < 0.45 ? 0 : fightEaseOutQuad((p - 0.45) / 0.55) * 22;
      fightHaloText(ctx, "-" + shown, target.x,
        target.y - 14 - rise, ev.crit ? 15 : 12,
        ev.crit ? fightCritColor : color, alpha);
      if (ev.crit && p < 0.5) {
        fightRing(ctx, target.x, target.y,
          4 + 16 * fightEaseOutQuad(p / 0.5), fightCritColor,
          (1 - p / 0.5) * 0.8, 2.2);
      }
    },
    // The combo chip of the cell corner: increments per landed hit
    // of this loop, the crits add the gold multiplier chip.
    frame(ctx, v, state) {
      if (state.combo < 1) { return; }
      ctx.save();
      const x = 8;
      const y = 6;
      ctx.globalAlpha = 0.92;
      ctx.fillStyle = "rgba(15, 18, 22, 0.8)";
      ctx.fillRect(x, y, 58, 15);
      ctx.strokeStyle = state.lastCrit ? fightCritColor : "#5f6b7a";
      ctx.lineWidth = 1.2;
      ctx.strokeRect(x, y, 58, 15);
      fightHaloText(ctx, "COMBO " + state.combo, x + 29, y + 8.5,
        9, state.lastCrit ? fightCritColor : "#ffffff", 1);
      if (state.lastCrit) {
        ctx.fillStyle = "rgba(15, 18, 22, 0.8)";
        ctx.fillRect(x + 62, y, 30, 15);
        ctx.strokeStyle = fightCritColor;
        ctx.strokeRect(x + 62, y, 30, 15);
        fightHaloText(ctx, "x2.5", x + 77, y + 8.5, 9,
          fightCritColor, 1);
      }
      ctx.restore();
    }
  }
);

// FightUI builds and drives the showcase: one column per variant,
// four cells per column, one shared clock. The render loop walks the
// cells every frame (throttled to ~30 fps - plenty for judging the
// ideas) and every cell replays the same scripted fight through its
// variant renderers.
const FightUI = {
  tile: null,
  tileReady: false,
  cells: [],
  lastRender: 0,

  // init builds the strip and starts the loop.
  init() {
    const count = document.getElementById("fight-count");
    if (count) {
      count.textContent = VARIANTS.length + " вариантов · цикл "
        + FIGHT.loopSec + " с";
    }
    this.loadTile();
    this.buildColumns();
    requestAnimationFrame((now) => this.render(now));
  },

  // loadTile fetches the world map tile the demo fight plays on: the
  // elven lands crop, the same imagery the live map draws.
  loadTile() {
    const img = new Image();
    img.onload = () => { this.tileReady = true; };
    img.src = "maps/" + FIGHT.tileLevel + "/" + FIGHT.tileName;
    this.tile = img;
  },

  // buildColumns fills the horizontal strip: the variant head (the
  // number, the name, the idea note) and the four position cells.
  buildColumns() {
    const strip = document.getElementById("fight-strip");
    if (!strip) { return; }
    const dpr = window.devicePixelRatio || 1;
    for (const variant of VARIANTS) {
      const column = document.createElement("div");
      column.className = "fight-column";
      const head = document.createElement("div");
      head.className = "fight-col-head";
      head.innerHTML = '<span class="fight-num">' + variant.num
        + '</span><span class="fight-name">' + variant.name + "</span>";
      column.appendChild(head);
      const note = document.createElement("div");
      note.className = "fight-note";
      note.textContent = variant.note;
      column.appendChild(note);
      for (const pos of FIGHT.positions) {
        const cell = document.createElement("figure");
        cell.className = "fight-cell";
        const canvas = document.createElement("canvas");
        canvas.width = Math.round(FIGHT.cellW * dpr);
        canvas.height = Math.round(FIGHT.cellH * dpr);
        canvas.style.width = FIGHT.cellW + "px";
        canvas.style.height = FIGHT.cellH + "px";
        const tag = document.createElement("figcaption");
        tag.className = "fight-cell-tag";
        tag.textContent = pos.tag;
        cell.appendChild(canvas);
        cell.appendChild(tag);
        column.appendChild(cell);
        const ctx = canvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        // The pair placement: the hero keeps the center, the pair
        // shifts a little so the top and the bottom enemies keep
        // their health bars and names inside the cell.
        const heroX = FIGHT.cellW / 2 + (pos.heroShiftX || 0);
        const heroY = FIGHT.cellH / 2 + (pos.heroShiftY || 0);
        this.cells.push({
          variant, pos, ctx,
          w: FIGHT.cellW, h: FIGHT.cellH,
          hero: {
            x: heroX, y: heroY, radius: 7,
            hp: FIGHT.heroMaxHp, maxHp: FIGHT.heroMaxHp
          },
          enemy: {
            x: heroX + pos.dx * FIGHT.scale,
            y: heroY + pos.dy * FIGHT.scale,
            radius: 6, hp: FIGHT.enemyMaxHp, maxHp: FIGHT.enemyMaxHp
          }
        });
      }
      strip.appendChild(column);
    }
  },

  // heroHpAt walks the scripted timeline and returns the health of
  // the hero at the loop time t (the regen refills both bars before
  // the restart).
  heroHpAt(t) {
    let hp = FIGHT.heroMaxHp;
    if (t >= 2.5) { hp -= 26; }
    if (t >= 6.1) { hp -= 68; }
    if (t >= 7.4) {
      hp += (FIGHT.heroMaxHp - hp) * Math.min(1, (t - 7.4) / 0.8);
    }

    return hp;
  },

  // enemyHpAt does the same for the enemy.
  enemyHpAt(t) {
    let hp = FIGHT.enemyMaxHp;
    if (t >= 0.9) { hp -= 38; }
    if (t >= 4.2) { hp -= 95; }
    if (t >= 7.4) {
      hp += (FIGHT.enemyMaxHp - hp) * Math.min(1, (t - 7.4) / 0.8);
    }

    return hp;
  },

  // render is the shared frame: throttled to ~30 fps, walks every
  // cell of every column with the same clock.
  render(nowMs) {
    if (nowMs - this.lastRender >= 31) {
      this.lastRender = nowMs;
      const t = (nowMs / 1000) % FIGHT.loopSec;
      for (const cell of this.cells) {
        this.renderCell(cell, t, nowMs);
      }
    }
    requestAnimationFrame((next) => this.render(next));
  },

  // renderCell draws one demo cell: the map background crop, the two
  // fighters, the variant effects of the active timeline events and
  // the overlays (vignette, banner, combo chip, caption).
  renderCell(cell, t, nowMs) {
    const ctx = cell.ctx;
    const v = {
      w: cell.w, h: cell.h,
      hero: Object.assign({}, cell.hero),
      enemy: Object.assign({}, cell.enemy),
      nowMs
    };
    v.hero.hp = this.heroHpAt(t);
    v.enemy.hp = this.enemyHpAt(t);
    const regen = t >= 7.4 && t < 8.5;

    // 1. The map background.
    this.drawBackground(ctx, cell);

    // 2. The active events of this cell clock.
    const swings = [];
    const damages = [];
    for (const ev of FIGHT.timeline) {
      const age = t - ev.at;
      if (age < 0) { continue; }
      if (ev.kind === "swing" && age < cell.variant.swingLife) {
        swings.push({ ev, p: age / cell.variant.swingLife });
      }
      if (ev.kind === "damage" && age < cell.variant.damageLife) {
        damages.push({ ev, p: age / cell.variant.damageLife });
      }
    }
    // The cell state of the frame overlays: the newest damage event
    // and the combo bookkeeping of this loop pass.
    const newest = damages.length > 0 ? damages[damages.length - 1] : null;
    let combo = 0;
    let lastCrit = false;
    for (const ev of FIGHT.timeline) {
      if (ev.kind !== "damage" || ev.at > t || t >= 7.4) { continue; }
      combo += 1;
      lastCrit = ev.crit;
    }
    const state = { nowMs, combo, lastCrit, damage: newest };

    // 3. The zoom / shake transforms of the variant wrap the scene.
    const zoomHit = newest && cell.variant.zoom
      ? cell.variant.zoom(newest.ev, newest.p) : null;
    ctx.save();
    if (zoomHit) {
      const target = newest.ev.byHero ? v.enemy : v.hero;
      ctx.translate(target.x, target.y);
      ctx.scale(zoomHit.scale, zoomHit.scale);
      ctx.translate(-target.x, -target.y);
    }
    if (newest && cell.variant.shake) {
      const shake = cell.variant.shake(newest.ev, newest.p);
      if (shake) {
        ctx.translate(shake.x, shake.y);
      }
    }

    // 4. The fighters.
    fightUnit(ctx, v.enemy, v.hero, { hero: false, nowMs, inCombat: true });
    fightUnit(ctx, v.hero, v.enemy, { hero: true, nowMs });
    fightHpBar(ctx, v.enemy, v.enemy.hp, v.enemy.maxHp,
      FIGHT.enemyName + "  lv 5", { regen });
    fightHpBar(ctx, v.hero, v.hero.hp, v.hero.maxHp, "Hero  lv 8",
      { hero: true, regen });

    // 5. The variant effects.
    for (const swing of swings) {
      cell.variant.swing(ctx, v, swing.ev, swing.p);
    }
    for (const damage of damages) {
      cell.variant.damage(ctx, v, damage.ev, damage.p);
    }
    ctx.restore();

    // 6. The overlays: the vignette flash, the frame overlays
    // (banners, combo chips) and the caption of the newest damage.
    if (newest && cell.variant.vignette) {
      const vin = cell.variant.vignette(newest.ev, newest.p);
      if (vin) {
        ctx.save();
        const grad = ctx.createRadialGradient(
          cell.w / 2, cell.h / 2, Math.min(cell.w, cell.h) * 0.22,
          cell.w / 2, cell.h / 2, Math.max(cell.w, cell.h) * 0.72);
        grad.addColorStop(0, "rgba(" + vin.color + ",0)");
        grad.addColorStop(1,
          "rgba(" + vin.color + "," + vin.alpha.toFixed(3) + ")");
        ctx.fillStyle = grad;
        ctx.fillRect(0, 0, cell.w, cell.h);
        ctx.restore();
      }
    }
    if (cell.variant.frame) {
      cell.variant.frame(ctx, v, state);
    }
    if (newest) {
      const ev = newest.ev;
      const caption = ev.crit
        ? "CRIT " + (ev.byHero ? "deals" : "takes") + " -" + ev.amount
        : "hero " + (ev.byHero ? "hits -" : "takes -") + ev.amount;
      const color = ev.crit ? fightCritColor
        : (ev.byHero ? fightDamageDealtColor : fightDamageTakenColor);
      const alpha = newest.p < 0.8 ? 1 : 1 - (newest.p - 0.8) / 0.2;
      fightHaloText(ctx, caption, cell.w / 2, cell.h - 13,
        9.5, color, alpha * 0.95);
    }
  },

  // drawBackground paints the world map crop of the cell: the same
  // elven lands tile the live map shows, level 0 imagery, the source
  // rectangle follows the cell world window. Without the tile (still
  // loading) the cell falls back to the dark theme fill.
  drawBackground(ctx, cell) {
    if (!this.tileReady || !this.tile) {
      ctx.save();
      ctx.fillStyle = "#20262e";
      ctx.fillRect(0, 0, cell.w, cell.h);
      ctx.restore();

      return;
    }
    const img = this.tile;
    const shiftX = cell.pos.heroShiftX || 0;
    const shiftY = cell.pos.heroShiftY || 0;
    // The world window of the cell: centered on the pair.
    const worldCX = FIGHT.heroWorldX + shiftX / FIGHT.scale;
    const worldCY = FIGHT.heroWorldY + shiftY / FIGHT.scale;
    const worldW = cell.w / FIGHT.scale;
    const worldH = cell.h / FIGHT.scale;
    // Map the world window into the tile image pixels: the tile
    // covers [tileX0, tileX0 + 32768] world units on its pixels.
    const tileX0 = (21 - FIGHT.tileZeroX) * FIGHT.tileWorldSize;
    const tileY0 = (19 - FIGHT.tileZeroY) * FIGHT.tileWorldSize;
    const sx = (worldCX - worldW / 2 - tileX0) / FIGHT.tileWorldSize
      * img.width;
    const sy = (worldCY - worldH / 2 - tileY0) / FIGHT.tileWorldSize
      * img.height;
    const sw = worldW / FIGHT.tileWorldSize * img.width;
    const sh = worldH / FIGHT.tileWorldSize * img.height;
    ctx.save();
    ctx.fillStyle = "#20262e";
    ctx.fillRect(0, 0, cell.w, cell.h);
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = "high";
    ctx.drawImage(img, sx, sy, sw, sh, 0, 0, cell.w, cell.h);
    ctx.restore();
  }
};
