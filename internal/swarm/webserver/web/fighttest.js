/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// FightTest is the fight FX comparison gallery of the -test-fight-ui
// mode (webserver.NewTestFightServer answers mode "test-fight"). It is
// a design decision aid, not a live view: the goal is to let a human
// compare many visual ideas for "the hero dealt damage", "the hero
// received damage" and their critical counterparts side by side and
// pick a favorite by number.
//
// The layout answers the comparison brief directly:
// - vertically four rows, one per enemy placement: the enemy above,
//   below, left and right of the character (the relative angle changes
//   how directional effects read);
// - horizontally one numbered column per visualization idea, eighteen
//   of them, in a horizontally scrollable strip;
// - every cell paints the same world map tile crop as its background
//   (the starter keltir meadow the bot actually hunts) and runs the
//   same synchronized beat loop: the hero hits, the hero takes a hit,
//   the hero lands a critical, the hero takes a critical, pause,
//   repeat.
//
// Every effect is a pure function of the shared loop clock (see the
// seeded per beat random tables), so scrolling the gallery back to a
// column repaints it exactly, skipping off screen columns is free and
// the Node harness can assert that every variant actually draws during
// its beats.
const FIGHT_CELL_W = 300;
const FIGHT_CELL_H = 190;

// The loop: four beats plus a rest, then the HP resets.
const FIGHT_LOOP_MS = 9000;

// The width of the sticky row header column of the grid.
const FIGHT_ROWHEAD_W = 112;

// The map background: the 21_19 tile of the world pyramid (the starter
// keltir meadow, zone center 46112/41500) is the terrain the bot
// actually hunts, so the cells look exactly like the live map. One
// tile is 32768 world units at 1024 source pixels; the world point of
// the zone center maps to tile pixel 417/273.
const FIGHT_TILE_URL = "maps/0/21_19.jpg";
const FIGHT_TILE_PX = 1024;
const FIGHT_TILE_UNITS = 32768;
// The world origin of the 21_19 tile: BX = 21 maps to world x
// (21 - 20) * 32768, BY = 19 to y (19 - 18) * 32768 (the
// mapTileZeroX/Y anchors of map.js).
const FIGHT_TILE_ORIGIN = { x: 32768, y: 32768 };
const FIGHT_BG_CENTER = { x: 46112, y: 41500 };

// The cell shows 1600 x ~1013 world units: a zoom of 0.1875, right in
// the band the live map runs at, so the blur of the upscaled imagery
// matches the real thing.
const FIGHT_BG_WORLD_W = 1600;

// The four enemy placements of the rows. The world Y axis grows to the
// south on screen, so "above" is a negative dy, exactly like the map.
const FIGHT_ROWS = [
    { label: "ENEMY\nABOVE", dx: 0, dy: -58 },
    { label: "ENEMY\nBELOW", dx: 0, dy: 58 },
    { label: "ENEMY\nLEFT", dx: -68, dy: 0 },
    { label: "ENEMY\nRIGHT", dx: 68, dy: 0 }
];

// The beat loop every cell runs, synchronized: the hero deals, the
// hero receives, the hero deals a critical (double damage), the hero
// receives a critical. rnd is the per beat seeded random table the
// procedural effects sample, so every cell and every repaint of the
// same beat produces the identical shapes.
const FIGHT_BEATS = [
    { at: 800, attacker: "hero", crit: false, amount: 46 },
    { at: 3000, attacker: "enemy", crit: false, amount: 12 },
    { at: 4900, attacker: "hero", crit: true, amount: 92 },
    { at: 6900, attacker: "enemy", crit: true, amount: 24 }
];

// The palette of the gallery: the live combat layer colors for the
// dealt (light blue streaks, amber numbers) and received (red) sides
// plus the gold critical accent, and the fixed unit marker palette of
// the map so the cells read like the real view.
const FIGHT_FX = {
    dealt: "#7cc4ff",
    dealtNum: "#ffd25c",
    taken: "#ff6b4a",
    takenNum: "#ff5252",
    crit: "#ffcc33",
    white: "#ffffff",
    ink: "#39424e",
    hero: "#1a73e8",
    mob: "#e37400",
    blood: "#a3122a",
    bloodDark: "#7d0d20"
};

// mulberry32 is the small deterministic PRNG of the seeded tables.
function fightMulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

(function seedFightBeats() {
    for (let i = 0; i < FIGHT_BEATS.length; i++) {
        const rng = fightMulberry32(0x9E3779B9 ^ ((i + 1) * 2654435761));
        const table = [];
        for (let k = 0; k < 64; k++) { table.push(rng()); }
        FIGHT_BEATS[i].rnd = table;
    }
})();

// The easing helpers of the effects (local copies so the file stands
// alone in the harness sandbox).
function fightEaseOutQuad(t) { return 1 - (1 - t) * (1 - t); }
function fightEaseInQuad(t) { return t * t; }
function fightEaseOutCubic(t) {
    return 1 - Math.pow(1 - t, 3);
}
function fightEaseOutBack(t) {
    const c = 1.70158;
    return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
}
function fightClamp01(v) { return Math.max(0, Math.min(1, v)); }

// fightHaloText draws one text run with a dark halo, the label style
// of the live map: readable over the light imagery and any effect.
function fightHaloText(ctx, text, x, y, fill, size, align, weight) {
    ctx.font = (weight || "bold") + " " + size
        + "px system-ui, 'Segoe UI', sans-serif";
    ctx.textAlign = align || "center";
    ctx.textBaseline = "middle";
    ctx.lineJoin = "round";
    ctx.lineWidth = Math.max(2, size * 0.24);
    ctx.strokeStyle = "rgba(12, 18, 28, 0.85)";
    ctx.strokeText(text, x, y);
    ctx.fillStyle = fill;
    ctx.fillText(text, x, y);
}

// fightBeatTarget / fightBeatAttacker name the sides of a beat.
function fightBeatTarget(beat) {
    return beat.attacker === "hero" ? "enemy" : "hero";
}

// fightSideColor picks the palette pair of a beat: the dealt side
// reads light blue / amber numbers, the received side red.
function fightSideColor(beat) {
    return beat.attacker === "hero" ? FIGHT_FX.dealt : FIGHT_FX.taken;
}
function fightSideNumColor(beat) {
    return beat.attacker === "hero" ? FIGHT_FX.dealtNum : FIGHT_FX.takenNum;
}

// The eighteen visualization variants. Every variant is a small bag of
// optional hooks the engine calls while it renders one cell:
//   draw(ctx, cell, loopT)      the fx layer on top of the units;
//   underlay(ctx, cell, loopT)  the fx layer between the map and the
//                               units (ground stains, cracks, auras);
//   unitOffset(name, cell, loopT) extra motion of one unit: {x, y,
//                               sx, sy, rot, flash, flashColor};
//   cellShake(cell, loopT)      a {x, y} translate of the whole cell;
//   zoom(cell, loopT)           a {k, x, y} zoom of the whole cell
//                               around a point (hitstop);
//   freeze(cell, loopT)         freezes the unit lunge clock at a
//                               returned time during a hitstop.
// All hooks are pure functions of the loop time - no per cell state.
const FIGHT_VARIANTS = [
// 1. The baseline replica: exactly the effects the live combat layer
// of map.js plays today (the windup arc, the tapered streak, the
// starburst, the floating number) so every other column is compared
// against the current production look.
{
    name: "Classic popups",
    note: "live style: streak + starburst + number",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 950)) {
            const age = loopT - beat.at;
            const t = age / 950;
            const color = fightSideColor(beat);
            const impact = FightTest.fightImpact(cell, beat);
            const attacker = cell.units[beat.attacker];
            const dx = impact.x - attacker.x;
            const dy = impact.y - attacker.y;
            const dist = Math.hypot(dx, dy) || 1;
            const ux = dx / dist;
            const uy = dy / dist;
            // The windup swoosh at the attacker.
            if (age < 150) {
                const w = fightEaseOutQuad(age / 150);
                const angle = Math.atan2(uy, ux);
                ctx.globalAlpha = 0.7 * (1 - w);
                ctx.strokeStyle = color;
                ctx.lineWidth = 2.4;
                ctx.lineCap = "round";
                ctx.beginPath();
                ctx.arc(attacker.x, attacker.y, 9 + 7 * w,
                    angle - 1.1 + 0.5 * w, angle - 0.25 + 0.5 * w);
                ctx.stroke();
                ctx.lineCap = "butt";
            }
            // The traveling streak.
            if (age < 210) {
                const travel = fightEaseOutQuad(age / 210);
                const reach = Math.min(dist, 16 + dist * 0.25) * travel;
                const head = 6 + 14 * travel;
                const tail = Math.max(2, reach - head);
                ctx.globalAlpha = 0.9 * (1 - travel * 0.45);
                ctx.strokeStyle = color;
                ctx.lineCap = "round";
                ctx.lineWidth = 3;
                ctx.beginPath();
                ctx.moveTo(attacker.x + ux * tail, attacker.y + uy * tail);
                ctx.lineTo(attacker.x + ux * reach, attacker.y + uy * reach);
                ctx.stroke();
                ctx.globalAlpha *= 0.55;
                ctx.lineWidth = 6;
                ctx.beginPath();
                ctx.moveTo(attacker.x + ux * tail, attacker.y + uy * tail);
                ctx.lineTo(attacker.x + ux * reach, attacker.y + uy * reach);
                ctx.stroke();
                ctx.lineCap = "butt";
            }
            // The impact starburst.
            if (age > 120 && age < 340) {
                const burst = (age - 120) / 220;
                const len = (5 + 9 * fightEaseOutQuad(burst))
                    * (beat.crit ? 1.3 : 1);
                ctx.globalAlpha = (1 - burst) * 0.95;
                ctx.strokeStyle = "#ffffff";
                ctx.lineWidth = 1.8;
                ctx.lineCap = "round";
                for (let i = 0; i < 6; i++) {
                    const a = (i / 6) * Math.PI * 2 + burst * 0.6;
                    const r0 = 2.5 + 2 * burst;
                    ctx.beginPath();
                    ctx.moveTo(impact.x + Math.cos(a) * r0,
                        impact.y + Math.sin(a) * r0);
                    ctx.lineTo(impact.x + Math.cos(a) * (r0 + len),
                        impact.y + Math.sin(a) * (r0 + len));
                    ctx.stroke();
                }
                ctx.lineCap = "butt";
                ctx.globalAlpha = (1 - burst) * 0.5;
                ctx.strokeStyle = color;
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.arc(impact.x, impact.y, 3 + 10 * burst, 0, Math.PI * 2);
                ctx.stroke();
            }
            // The floating number.
            const target = cell.units[fightBeatTarget(beat)];
            const pop = Math.min(1, age / 130);
            const scale = pop < 1 ? fightEaseOutBack(pop) : 1;
            const rise = fightEaseOutQuad(t) * 30;
            const alpha = t < 0.75 ? 1 : 1 - (t - 0.75) / 0.25;
            const size = (beat.crit ? 17 : 12) * scale;
            ctx.globalAlpha = alpha;
            fightHaloText(ctx, String(beat.amount),
                target.x + ((beat.rnd[0] * 2 - 1) * 8),
                target.y - 16 - rise,
                beat.crit ? FIGHT_FX.crit : fightSideNumColor(beat),
                Math.max(4, size));
            ctx.globalAlpha = 1;
        }
    }
},

// 2. One bold number: no streak, no burst, just the digit punching in
// at the hurt unit, holding and dropping out. The minimalist glyph
// direction - the loudest possible reading of the value.
{
    name: "Punch numbers",
    note: "one bold number punches in, drops, fades",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 1000)) {
            const age = loopT - beat.at;
            const t = age / 1000;
            const target = cell.units[fightBeatTarget(beat)];
            const crit = beat.crit;
            let scale = 1;
            if (age < 90) {
                scale = 0.3 + 1.1 * fightEaseOutBack(age / 90);
            } else if (age < 350) {
                scale = 1.4 - 0.4 * fightEaseOutQuad((age - 90) / 260);
            }
            let dy = 0;
            if (age > 400) {
                dy = fightEaseInQuad((age - 400) / 600) * 12;
            }
            const alpha = age > 780 ? 1 - (age - 780) / 220 : 1;
            if (crit && age < 300) {
                const ring = fightEaseOutQuad(Math.min(1, age / 300));
                ctx.globalAlpha = 0.6 * (1 - ring);
                ctx.strokeStyle = FIGHT_FX.crit;
                ctx.lineWidth = 2.5;
                ctx.beginPath();
                ctx.arc(target.x, target.y - 20, 6 + 20 * ring, 0,
                    Math.PI * 2);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            ctx.globalAlpha = alpha;
            const size = (crit ? 21 : 14) * scale;
            const fill = crit ? FIGHT_FX.crit
                : (beat.attacker === "hero" ? "#ffffff" : FIGHT_FX.takenNum);
            fightHaloText(ctx, String(beat.amount),
                target.x, target.y - 20 + dy, fill, Math.max(5, size));
            if (crit) {
                fightHaloText(ctx, "CRIT",
                    target.x, target.y - 20 + dy - size * 0.85,
                    FIGHT_FX.crit, 8);
            }
            ctx.globalAlpha = 1;
        }
    }
},

// 3. The anime slash: a bright crescent sweeps across the hurt unit
// perpendicular to the hit direction, with two speed lines behind it.
// A critical draws the mirrored X cross a beat later in gold.
{
    name: "Slash crescent",
    note: "an anime slash sweeps across the hurt unit",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 620)) {
            const age = loopT - beat.at;
            const impact = FightTest.fightImpact(cell, beat);
            const dir = FightTest.fightDir(cell, beat);
            const angle = Math.atan2(dir.y, dir.x);
            const crit = beat.crit;
            const slashes = crit ? 2 : 1;
            for (let s = 0; s < slashes; s++) {
                const sAge = age - s * 90;
                if (sAge < 0 || sAge > 520) { continue; }
                const t = sAge / 520;
                const mirror = s === 1;
                const sweep = fightEaseOutCubic(
                    Math.min(1, sAge / 180));
                const radius = 24 * (1 + 0.1 * t);
                const width = 9 * (1 - t * 0.6);
                const base = angle + Math.PI / 2 + (mirror ? Math.PI : 0);
                const span = 1.9 * sweep;
                const a0 = base - span / 2;
                const a1 = base + span / 2;
                ctx.save();
                ctx.translate(impact.x, impact.y);
                ctx.rotate(base);
                ctx.globalAlpha = 0.9 * (1 - t * 0.8);
                ctx.beginPath();
                ctx.arc(0, 0, radius, -span / 2, span / 2);
                ctx.arc(0, 0, Math.max(1, radius - width), span / 2,
                    -span / 2, true);
                ctx.closePath();
                ctx.fillStyle = crit ? FIGHT_FX.crit : "#ffffff";
                ctx.fill();
                ctx.lineWidth = 1.5;
                ctx.strokeStyle = crit ? "#ffffff"
                    : fightSideColor(beat);
                ctx.stroke();
                // The speed lines parallel to the slash.
                if (sAge < 260) {
                    ctx.globalAlpha = 0.45 * (1 - sAge / 260);
                    ctx.strokeStyle = crit ? FIGHT_FX.crit
                        : fightSideColor(beat);
                    ctx.lineWidth = 1.5;
                    const off = mirror ? -radius - 8 : radius + 8;
                    for (const side of [-1, 1]) {
                        const l = 14 * sweep * (side > 0 ? 1 : 0.6);
                        ctx.beginPath();
                        ctx.moveTo(-l / 2, off + side * 2);
                        ctx.lineTo(l / 2, off + side * 2);
                        ctx.stroke();
                    }
                }
                ctx.restore();
            }
            if (crit && age > 90 && age < 300) {
                const cut = (age - 90) / 210;
                ctx.globalAlpha = 0.8 * (1 - cut);
                ctx.strokeStyle = "#ffffff";
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.moveTo(impact.x - dir.x * 26, impact.y - dir.y * 26);
                ctx.lineTo(impact.x + dir.x * 26, impact.y + dir.y * 26);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 4. The wordless burst: a white star flash plus expanding shockwave
// rings at the impact, tinted by the side. Criticals add a third ring,
// more spikes and flying dust. For the reviewer who wants damage to
// read at a glance without any numbers.
{
    name: "Starburst + wave",
    note: "spikes and expanding rings, no numbers",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 620)) {
            const age = loopT - beat.at;
            const t = age / 620;
            const impact = FightTest.fightImpact(cell, beat);
            const color = fightSideColor(beat);
            const crit = beat.crit;
            const spikes = crit ? 9 : 6;
            // The core flash.
            if (age < 130) {
                ctx.globalAlpha = 0.9 * (1 - age / 130);
                ctx.fillStyle = "#ffffff";
                ctx.beginPath();
                ctx.arc(impact.x, impact.y, (crit ? 11 : 7)
                    * (1 - age / 130 * 0.4), 0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
            // The spikes.
            if (age < 360) {
                const st = (age - 60) / 300;
                if (st > 0) {
                    const len = (6 + 11 * fightEaseOutQuad(st))
                        * (crit ? 1.25 : 1);
                    ctx.globalAlpha = (1 - st) * 0.95;
                    ctx.strokeStyle = "#ffffff";
                    ctx.lineWidth = 1.8;
                    ctx.lineCap = "round";
                    for (let i = 0; i < spikes; i++) {
                        const a = (i / spikes) * Math.PI * 2 + st * 0.7;
                        const r0 = 3 + 2.5 * st;
                        ctx.beginPath();
                        ctx.moveTo(impact.x + Math.cos(a) * r0,
                            impact.y + Math.sin(a) * r0);
                        ctx.lineTo(impact.x + Math.cos(a) * (r0 + len),
                            impact.y + Math.sin(a) * (r0 + len));
                        ctx.stroke();
                    }
                    ctx.lineCap = "butt";
                    ctx.globalAlpha = 1;
                }
            }
            // The rings.
            const rings = crit ? 3 : 2;
            for (let ring = 0; ring < rings; ring++) {
                const rAge = age - ring * 70;
                if (rAge < 0 || rAge > 520) { continue; }
                const rt = rAge / 520;
                ctx.globalAlpha = 0.7 * (1 - rt);
                ctx.strokeStyle = ring === 0 ? "#ffffff" : color;
                ctx.lineWidth = Math.max(0.8, 2.5 - rt * 1.6);
                ctx.beginPath();
                ctx.arc(impact.x, impact.y,
                    4 + 34 * fightEaseOutQuad(rt), 0, Math.PI * 2);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            // The critical dust.
            if (crit) {
                for (let i = 0; i < 8; i++) {
                    const dAge = age - 60;
                    if (dAge < 0 || dAge > 480) { continue; }
                    const dt = dAge / 480;
                    const a = (i / 8) * Math.PI * 2 + beat.rnd[i + 4];
                    const r = 6 + 22 * fightEaseOutQuad(dt);
                    ctx.globalAlpha = 0.5 * (1 - dt);
                    ctx.fillStyle = "#e0dcd2";
                    ctx.beginPath();
                    ctx.arc(impact.x + Math.cos(a) * r,
                        impact.y + Math.sin(a) * r * 0.7,
                        2.2 * (1 - dt * 0.5), 0, Math.PI * 2);
                    ctx.fill();
                    ctx.globalAlpha = 1;
                }
            }
        }
    }
},

// 5. The blood spray: droplets burst from the hurt unit along the hit
// direction, arc under gravity and leave small stains on the ground
// that linger. The realistic direction - visceral, loud, and blind to
// which side dealt the blow (blood is blood).
{
    name: "Blood spray",
    note: "droplets burst along the hit, stains remain",
    underlay(ctx, cell, loopT) {
        // The ground stains and the critical splatter decal live under
        // the units so they read as terrain.
        for (const beat of FightTest.fightActiveBeats(loopT, 3000)) {
            const age = loopT - beat.at;
            const impact = FightTest.fightImpact(cell, beat);
            const dir = FightTest.fightDir(cell, beat);
            const target = cell.units[fightBeatTarget(beat)];
            const fade = age > 2200 ? 1 - (age - 2200) / 800 : 1;
            if (beat.crit) {
                for (let i = 0; i < 3; i++) {
                    const rx = (beat.rnd[i + 20] * 2 - 1) * 16;
                    const ry = 8 + beat.rnd[i + 24] * 6;
                    ctx.globalAlpha = 0.3 * fade;
                    ctx.fillStyle = FIGHT_FX.bloodDark;
                    ctx.beginPath();
                    ctx.ellipse(target.x + rx, target.y + ry,
                        4 + beat.rnd[i + 28] * 5, 2.5, 0, 0, Math.PI * 2);
                    ctx.fill();
                    ctx.globalAlpha = 1;
                }
            }
            const drops = beat.crit ? 16 : 9;
            for (let i = 0; i < drops; i++) {
                const spread = (beat.rnd[i] * 2 - 1) * 1.1;
                const angle = Math.atan2(dir.y, dir.x) + spread;
                const speed = 50 + beat.rnd[i + 8] * 95;
                const vy = Math.sin(angle) * speed - 35;
                const vx = Math.cos(angle) * speed;
                const g = 300 / 1000;
                const dy = 6;
                // The landing time of the closed form ballistic arc.
                const disc = vy * vy + 2 * g * dy;
                const land = (vy + Math.sqrt(Math.max(0, disc))) / g;
                if (age / 1000 >= land && age < 3000) {
                    const lx = impact.x + vx * land;
                    const ly = impact.y + dy;
                    ctx.globalAlpha = 0.45 * fade;
                    ctx.fillStyle = FIGHT_FX.bloodDark;
                    ctx.beginPath();
                    ctx.ellipse(lx, ly, 1.6 + beat.rnd[i + 16] * 2.4, 1.1,
                        0, 0, Math.PI * 2);
                    ctx.fill();
                    ctx.globalAlpha = 1;
                }
            }
        }
    },
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 800)) {
            const age = loopT - beat.at;
            const impact = FightTest.fightImpact(cell, beat);
            const dir = FightTest.fightDir(cell, beat);
            const drops = beat.crit ? 16 : 9;
            const g = 300 / 1000;
            for (let i = 0; i < drops; i++) {
                const spread = (beat.rnd[i] * 2 - 1) * 1.1;
                const angle = Math.atan2(dir.y, dir.x) + spread;
                const speed = 50 + beat.rnd[i + 8] * 95;
                const life = 0.45 + beat.rnd[i + 12] * 0.3;
                const tt = age / 1000;
                if (tt > life) { continue; }
                const vy = Math.sin(angle) * speed - 35;
                const vx = Math.cos(angle) * speed;
                const dy = 6;
                const disc = vy * vy + 2 * g * dy;
                const land = (vy + Math.sqrt(Math.max(0, disc))) / g;
                if (tt >= land) { continue; }
                const py = impact.y - (vy * tt + 0.5 * g * tt * tt);
                const px = impact.x + vx * tt;
                const fade = tt > life * 0.7
                    ? 1 - (tt - life * 0.7) / (life * 0.3) : 1;
                ctx.globalAlpha = fade;
                ctx.fillStyle = i % 3 === 0 ? FIGHT_FX.bloodDark
                    : FIGHT_FX.blood;
                ctx.beginPath();
                ctx.arc(px, py,
                    (1.3 + beat.rnd[i + 16] * 1.8) * (beat.crit ? 1.5 : 1),
                    0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
            // A short red flash on the wound itself.
            if (age < 160) {
                const target = cell.units[fightBeatTarget(beat)];
                ctx.globalAlpha = 0.5 * (1 - age / 160);
                ctx.fillStyle = FIGHT_FX.blood;
                ctx.beginPath();
                ctx.arc(target.x, target.y, (beat.crit ? 10 : 7)
                    * (1 - age / 160 * 0.3), 0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 6. The knockback recoil: the hurt unit is physically shoved along
// the hit direction, squashes and springs back with a little overshoot.
// The cell itself shares a small tremor when the hero takes the blow,
// and criticals add a two frame white strobe on the unit.
{
    name: "Knockback recoil",
    note: "the hurt unit is shoved back and squashes",
    unitOffset(name, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 460)) {
            if (name !== fightBeatTarget(beat)) { continue; }
            const age = loopT - beat.at;
            const dir = FightTest.fightDir(cell, beat);
            const amp = beat.crit ? 20 : 11;
            let k = 0;
            if (age < 90) {
                k = fightEaseOutQuad(age / 90);
            } else if (age < 350) {
                k = 1 - fightEaseOutBack((age - 90) / 260);
            } else {
                k = 0;
            }
            const mod = {
                x: dir.x * amp * k,
                y: dir.y * amp * k
            };
            // The squash along the hit axis.
            const squash = 0.16 * Math.abs(Math.sin(Math.PI
                * fightClamp01(age / 350)));
            mod.rot = Math.atan2(dir.y, dir.x);
            mod.sx = 1 + squash * Math.abs(Math.cos(0));
            mod.sy = 1 - squash;
            // The critical strobe.
            if (beat.crit) {
                const strobe = Math.floor(age / 70) % 2 === 0
                    && age < 210;
                mod.flash = strobe ? 0.85 : 0;
                mod.flashColor = "#ffffff";
            }
            return mod;
        }
        return null;
    },
    cellShake(cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 300)) {
            const age = loopT - beat.at;
            const dir = FightTest.fightDir(cell, beat);
            const amp = beat.attacker === "enemy"
                ? (beat.crit ? 6 : 3.5) * (1 - age / 300)
                : (beat.crit ? 2.2 : 1.1) * (1 - age / 300);
            return { x: dir.x * amp, y: dir.y * amp };
        }
        return null;
    },
    draw(ctx, cell, loopT) {
        // Two motion lines trail the shoved unit.
        for (const beat of FightTest.fightActiveBeats(loopT, 260)) {
            const age = loopT - beat.at;
            const target = cell.units[fightBeatTarget(beat)];
            const dir = FightTest.fightDir(cell, beat);
            const t = age / 260;
            ctx.globalAlpha = 0.5 * (1 - t);
            ctx.strokeStyle = fightSideColor(beat);
            ctx.lineWidth = 1.6;
            const back = -dir;
            for (const side of [-1, 1]) {
                const px = -dir.y * side * 9;
                const py = dir.x * side * 9;
                ctx.beginPath();
                ctx.moveTo(target.x + px + back.x * 6,
                    target.y + py + back.y * 6);
                ctx.lineTo(target.x + px + back.x * (6 + 14 * (1 - t)),
                    target.y + py + back.y * (6 + 14 * (1 - t)));
                ctx.stroke();
            }
            ctx.globalAlpha = 1;
        }
    }
},
// 7. The HP chunk ghost: the lost bar width detaches from the HP bar,
// floats up as a pale ghost plate with the number and melts away,
// while a white afterimage of the old fill shrinks onto the new value.
// The fighting game bar direction - the damage reads on the vitals.
{
    name: "HP chunk ghost",
    note: "the lost HP detaches from the bar and floats",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 900)) {
            const age = loopT - beat.at;
            const t = age / 900;
            const target = fightBeatTarget(beat);
            const rect = FightTest.fightBarRect(cell, target);
            const max = target === "hero" ? 100 : 200;
            const hpBefore = FightTest.fightHPBefore(beat);
            const hpNow = FightTest.hpOf(target, loopT);
            const beforeW = rect.w * hpBefore / max;
            const nowW = rect.w * hpNow / max;
            const lostW = Math.max(0, beforeW - nowW);
            const shrink = fightEaseOutQuad(Math.min(1, age / 500));
            // The afterimage of the lost fill.
            if (age < 500 && lostW > 0) {
                const ghostW = lostW * (1 - shrink);
                ctx.fillStyle = "rgba(255, 255, 255, 0.6)";
                ctx.fillRect(rect.x + nowW, rect.y, ghostW, rect.h);
                ctx.strokeStyle = "rgba(255, 255, 255, 0.9)";
                ctx.lineWidth = 1;
                ctx.strokeRect(rect.x + nowW, rect.y, ghostW, rect.h);
            }
            // The critical bar flash.
            if (beat.crit && age < 130) {
                ctx.fillStyle = "rgba(255, 255, 255, "
                    + (0.75 * (1 - age / 130)) + ")";
                ctx.fillRect(rect.x, rect.y, rect.w, rect.h);
            }
            // The floating chunk with the number.
            const rise = fightEaseOutQuad(t) * 20;
            const alpha = t < 0.62 ? 1 : 1 - (t - 0.62) / 0.38;
            const chunkX = rect.x + nowW + Math.min(lostW, 18) / 2;
            ctx.globalAlpha = alpha;
            ctx.fillStyle = target === "hero" ? "#ff5252" : "#ffd25c";
            if (beat.crit) {
                // A jagged double plate for the crit.
                ctx.fillRect(chunkX - 4, rect.y - 6 - rise, 5, 4);
                ctx.fillRect(chunkX - 1, rect.y - 8 - rise, 5, 4);
            } else {
                ctx.fillRect(chunkX - 3, rect.y - 7 - rise, 6, 4);
            }
            fightHaloText(ctx, String(beat.amount),
                chunkX + 8, rect.y - 10 - rise,
                beat.crit ? FIGHT_FX.crit
                    : (target === "hero" ? FIGHT_FX.takenNum
                        : FIGHT_FX.dealtNum),
                beat.crit ? 11 : 9);
            ctx.globalAlpha = 1;
        }
    }
},

// 8. The lightning jolt: a jagged bolt snaps from the attacker to the
// target and flickers like an arc lamp, with short branches on a
// critical. The electric direction - fast, aggressive, very audible.
{
    name: "Lightning jolt",
    note: "a flickering bolt snaps between the two",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 460)) {
            const age = loopT - beat.at;
            const from = cell.units[beat.attacker];
            const to = cell.units[fightBeatTarget(beat)];
            const windows = [[0, 110, 1], [140, 240, 0.8], [280, 400, 0.55]];
            let alpha = 0;
            for (const w of windows) {
                if (age >= w[0] && age < w[1]) { alpha = w[2]; }
            }
            if (alpha <= 0) { continue; }
            const color = beat.crit ? FIGHT_FX.crit
                : fightSideColor(beat);
            const segments = 8;
            const dx = (to.x - from.x) / segments;
            const dy = (to.y - from.y) / segments;
            // One polyline with the perpendicular jitter of the seeded
            // table, drawn twice (glow + core).
            const path = [];
            for (let i = 0; i <= segments; i++) {
                const env = 1 - Math.abs(i / segments - 0.5) * 1.7;
                const jitter = (beat.rnd[i % 16] * 2 - 1) * 7
                    * Math.max(0, env);
                const nx = -dy / (Math.hypot(dx, dy) || 1);
                const ny = dx / (Math.hypot(dx, dy) || 1);
                path.push({
                    x: from.x + dx * i + nx * jitter,
                    y: from.y + dy * i + ny * jitter
                });
            }
            const strokePath = (width, colorStyle, lineAlpha) => {
                ctx.globalAlpha = lineAlpha;
                ctx.strokeStyle = colorStyle;
                ctx.lineWidth = width;
                ctx.lineJoin = "round";
                ctx.lineCap = "round";
                ctx.beginPath();
                ctx.moveTo(path[0].x, path[0].y);
                for (let i = 1; i < path.length; i++) {
                    ctx.lineTo(path[i].x, path[i].y);
                }
                ctx.stroke();
                ctx.lineCap = "butt";
            };
            strokePath(beat.crit ? 6 : 4.5, color, alpha * 0.4);
            strokePath(beat.crit ? 2.4 : 1.8, "#ffffff", alpha);
            // The branches.
            const branches = beat.crit ? 3 : 1;
            for (let b = 0; b < branches; b++) {
                const node = path[2 + b * 3];
                if (!node) { continue; }
                const a = beat.rnd[30 + b] * Math.PI * 2;
                ctx.globalAlpha = alpha * 0.7;
                ctx.strokeStyle = color;
                ctx.lineWidth = 1.2;
                ctx.beginPath();
                ctx.moveTo(node.x, node.y);
                ctx.lineTo(node.x + Math.cos(a) * 7,
                    node.y + Math.sin(a) * 7);
                ctx.lineTo(node.x + Math.cos(a) * 7
                    + Math.cos(a + 0.6) * 5,
                    node.y + Math.sin(a) * 7 + Math.sin(a + 0.6) * 5);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            // The endpoint spark.
            if (age < 120) {
                ctx.globalAlpha = alpha * (1 - age / 120);
                ctx.fillStyle = "#ffffff";
                ctx.beginPath();
                ctx.arc(to.x, to.y, 4 * (1 - age / 120 * 0.5), 0,
                    Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 9. The comic burst: a star balloon pops up over the hurt unit and
// shouts HIT or CRIT at a jaunty angle. The loud cartoon direction -
// playful, unmistakable, zero ambiguity about the size of the blow.
{
    name: "Comic burst",
    note: "a comic star balloon shouts HIT or CRIT",
    draw(ctx, cell, loopT) {
        for (const beat of FIGHT_BEATS) {
            const beatIndex = FIGHT_BEATS.indexOf(beat);
            const age = loopT - beat.at;
            if (age < 0 || age > 760) { continue; }
            const target = cell.units[fightBeatTarget(beat)];
            const crit = beat.crit;
            let scale = fightEaseOutBack(Math.min(1, age / 130));
            let alpha = 1;
            if (age > 640) {
                const out = (age - 640) / 120;
                scale = 1 - fightEaseInQuad(out);
                alpha = 1 - out;
            }
            const R = (crit ? 34 : 24) * scale;
            const r = R * 0.45;
            const spikes = 12;
            const wobble = Math.sin(age / 76) * 0.04;
            const cx = target.x + (beatIndex % 2 === 0 ? 10 : -10);
            const cy = target.y - 26;
            const rotation = -0.14 + wobble
                + (beatIndex % 2 === 0 ? 0.08 : -0.08);
            ctx.save();
            ctx.translate(cx, cy);
            ctx.rotate(rotation);
            ctx.globalAlpha = alpha;
            ctx.beginPath();
            for (let i = 0; i < spikes * 2; i++) {
                const rad = i % 2 === 0 ? R : r;
                const a = (i / (spikes * 2)) * Math.PI * 2;
                const px = Math.cos(a) * rad;
                const py = Math.sin(a) * rad;
                if (i === 0) { ctx.moveTo(px, py); }
                else { ctx.lineTo(px, py); }
            }
            ctx.closePath();
            ctx.fillStyle = crit ? FIGHT_FX.crit
                : (beat.attacker === "hero" ? FIGHT_FX.dealtNum
                    : FIGHT_FX.takenNum);
            ctx.fill();
            ctx.lineWidth = 2;
            ctx.strokeStyle = "#3a1400";
            ctx.stroke();
            if (scale > 0.5) {
                ctx.globalAlpha = alpha;
                ctx.font = "bold " + Math.round((crit ? 14 : 11) * scale)
                    + "px system-ui, 'Segoe UI', sans-serif";
                ctx.textAlign = "center";
                ctx.textBaseline = "middle";
                ctx.fillStyle = "#3a1400";
                ctx.fillText(crit ? "CRIT!" : "HIT!", 0, 0);
            }
            ctx.restore();
            ctx.globalAlpha = 1;
        }
    }
},

// 10. The arrow volley: arrows spawn at the attacker, fly to the
// target and stick with a wobbling thunk (three at once on a critical).
// The projectile direction - reads beautifully at range and up close.
{
    name: "Arrow volley",
    note: "arrows fly over and stick with a wobble",
    draw(ctx, cell, loopT) {
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            if (age < 0 || age > 760) { continue; }
            const from = cell.units[beat.attacker];
            const to = cell.units[fightBeatTarget(beat)];
            const dir = FightTest.fightDir(cell, beat);
            const angle = Math.atan2(dir.y, dir.x);
            const color = beat.crit ? "#f59e0b"
                : (beat.attacker === "hero" ? "#3b82f6" : "#ef4444");
            const arrows = beat.crit ? 3 : 1;
            for (let i = 0; i < arrows; i++) {
                const aAge = age - i * 45;
                if (aAge < 0) { continue; }
                const flight = 120;
                const spread = arrows === 1 ? 0 : (i - 1) * 7;
                const tx = to.x - dir.y * spread;
                const ty = to.y + dir.x * spread;
                let x;
                let y;
                let stuck = false;
                let wobble = 0;
                if (aAge < flight) {
                    const p = aAge / flight;
                    x = from.x + (tx - from.x) * fightEaseInQuad(p);
                    y = from.y + (ty - from.y) * fightEaseInQuad(p);
                } else {
                    x = tx;
                    y = ty;
                    stuck = true;
                    const since = aAge - flight;
                    wobble = Math.sin(since / 34)
                        * Math.exp(-since / 160) * 0.22;
                    if (aAge > 520) { continue; }
                }
                const alpha = stuck && aAge > 360
                    ? 1 - (aAge - 360) / 400 : 1;
                ctx.save();
                ctx.globalAlpha = alpha;
                ctx.translate(x, y);
                ctx.rotate(angle + wobble);
                // The shaft.
                ctx.strokeStyle = color;
                ctx.lineWidth = 1.8;
                ctx.beginPath();
                ctx.moveTo(-9, 0);
                ctx.lineTo(4, 0);
                ctx.stroke();
                // The head.
                ctx.fillStyle = color;
                ctx.beginPath();
                ctx.moveTo(7, 0);
                ctx.lineTo(3, -2.6);
                ctx.lineTo(3, 2.6);
                ctx.closePath();
                ctx.fill();
                // The fletch.
                ctx.lineWidth = 1.2;
                ctx.beginPath();
                ctx.moveTo(-9, 0);
                ctx.lineTo(-12, -2.4);
                ctx.moveTo(-9, 0);
                ctx.lineTo(-12, 2.4);
                ctx.stroke();
                ctx.restore();
                ctx.globalAlpha = 1;
            }
            // The impact tick.
            if (age > 110 && age < 320) {
                const t = (age - 110) / 210;
                ctx.globalAlpha = 0.8 * (1 - t);
                ctx.strokeStyle = color;
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.arc(to.x, to.y, 3 + 9 * t, 0, Math.PI * 2);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 11. The local shake: the hit speaks through motion alone. The hurt
// unit jitters, the whole cell shares a directional tremor scaled by
// the side and the size of the blow, and a critical flashes the cell
// frame for a beat. Nothing else - the quietest possible loud hit.
{
    name: "Local shake",
    note: "the hit speaks through motion, not glyphs",
    unitOffset(name, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 400)) {
            if (name !== fightBeatTarget(beat)) { continue; }
            const age = loopT - beat.at;
            const amp = (beat.crit ? 4 : 2) * (1 - age / 400);
            const seed = beat.at;
            return {
                x: Math.sin(age * 0.11 + seed) * amp,
                y: Math.sin(age * 0.13 + seed + 1.7) * amp
            };
        }
        return null;
    },
    cellShake(cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 380)) {
            const age = loopT - beat.at;
            const dir = FightTest.fightDir(cell, beat);
            const base = beat.attacker === "enemy"
                ? (beat.crit ? 6.5 : 3.8)
                : (beat.crit ? 2.6 : 1.3);
            const amp = base * (1 - age / 380);
            return {
                x: dir.x * amp + Math.sin(age * 0.09) * amp * 0.3,
                y: dir.y * amp + Math.cos(age * 0.11) * amp * 0.3
            };
        }
        return null;
    },
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 120)) {
            if (!beat.crit) { continue; }
            const age = loopT - beat.at;
            ctx.globalAlpha = 0.5 * (1 - age / 120);
            ctx.strokeStyle = "#ffffff";
            ctx.lineWidth = 3;
            ctx.strokeRect(1.5, 1.5, FIGHT_CELL_W - 3, FIGHT_CELL_H - 3);
            ctx.globalAlpha = 1;
        }
    }
},

// 12. The damage tally: the exchange is metered in the corners. Every
// point of damage the hero deals flies as a tick into the top right
// dealt meter, every point taken slides a small red mark into the top
// left. The bookkeeping direction - precise, calm, HUD-native.
{
    name: "Damage tally",
    note: "corner meters accumulate the exchange",
    draw(ctx, cell, loopT) {
        // The meters: the sums of the beats that already landed.
        let dealt = 0;
        let taken = 0;
        let dealtCrit = false;
        let takenCrit = false;
        let dealtBump = 0;
        let takenBump = 0;
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            if (age < 0) { continue; }
            if (beat.attacker === "hero") {
                dealt += beat.amount;
                if (beat.crit) { dealtCrit = true; }
                dealtBump = Math.max(0, 1 - age / 300);
            } else {
                taken += beat.amount;
                if (beat.crit) { takenCrit = true; }
                takenBump = Math.max(0, 1 - age / 300);
            }
        }
        // The dealt meter, top right.
        const dealtScale = 1 + 0.25 * fightEaseOutQuad(dealtBump);
        fightHaloText(ctx, "DEALT " + dealt,
            FIGHT_CELL_W - 8, 12,
            dealtCrit ? FIGHT_FX.crit : FIGHT_FX.dealtNum,
            10 * dealtScale, "right");
        // The taken meter, top left under the caption.
        const takenScale = 1 + 0.25 * fightEaseOutQuad(takenBump);
        fightHaloText(ctx, "TAKEN " + taken,
            8, 24, takenCrit ? FIGHT_FX.crit : FIGHT_FX.takenNum,
            10 * takenScale, "left");
        // The flying ticks: the number rides from the hurt unit into
        // its meter.
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            if (age < 0 || age > 400) { continue; }
            const target = cell.units[fightBeatTarget(beat)];
            const dest = beat.attacker === "hero"
                ? { x: FIGHT_CELL_W - 44, y: 12 }
                : { x: 44, y: 24 };
            const p = fightEaseInQuad(age / 400);
            const x = target.x + (dest.x - target.x) * p;
            const y = target.y + (dest.y - target.y) * p
                - Math.sin(Math.PI * p) * 14;
            ctx.globalAlpha = 1 - fightEaseInQuad(age / 400) * 0.4;
            fightHaloText(ctx, "+" + beat.amount, x, y,
                beat.crit ? FIGHT_FX.crit : fightSideNumColor(beat), 9);
            ctx.globalAlpha = 1;
        }
    }
},
// 13. The minimal strobe: the hurt unit's marker flashes (white when
// the hero dealt it, red when the hero took it) with one expanding
// ring; a critical triples the pulse and lances a short beam between
// the two. The quietest direction that still answers "who hit whom".
{
    name: "Unit flash + ring",
    note: "minimal: marker strobe and one ring",
    unitOffset(name, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 400)) {
            if (name !== fightBeatTarget(beat)) { continue; }
            const age = loopT - beat.at;
            const crit = beat.crit;
            let flash = 0;
            if (crit) {
                const phase = age % 110;
                if (age < 330 && phase < 55) { flash = 0.9; }
            } else if (age < 150) {
                flash = 0.85 * (1 - age / 150);
            }
            return {
                flash,
                flashColor: beat.attacker === "hero"
                    ? "#ffffff" : "#ff5252"
            };
        }
        return null;
    },
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 520)) {
            const age = loopT - beat.at;
            const target = cell.units[fightBeatTarget(beat)];
            const rings = beat.crit ? 2 : 1;
            for (let ring = 0; ring < rings; ring++) {
                const rAge = age - ring * 100;
                if (rAge < 0 || rAge > 420) { continue; }
                const t = rAge / 420;
                ctx.globalAlpha = 0.8 * (1 - t);
                ctx.strokeStyle = beat.attacker === "hero"
                    ? "#ffffff" : "#ff5252";
                ctx.lineWidth = Math.max(0.8, 2.4 - t * 1.5);
                ctx.beginPath();
                ctx.arc(target.x, target.y,
                    6 + (beat.crit ? 28 : 18) * fightEaseOutQuad(t),
                    0, Math.PI * 2);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            if (beat.crit && age < 180) {
                const from = cell.units[beat.attacker];
                ctx.globalAlpha = 0.3 * (1 - age / 180);
                ctx.strokeStyle = "#ffffff";
                ctx.lineWidth = 3;
                ctx.beginPath();
                ctx.moveTo(from.x, from.y);
                ctx.lineTo(target.x, target.y);
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 14. The ground cracks: the terrain under the hurt unit shatters -
// dark cracks radiate from the impact, dust puffs rise, and a
// critical opens a small crater. The scars linger for seconds so a
// busy fight leaves a history on the ground.
{
    name: "Ground cracks",
    note: "the ground under the target shatters",
    underlay(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 3000)) {
            const age = loopT - beat.at;
            const target = cell.units[fightBeatTarget(beat)];
            const fade = age > 2300 ? 1 - (age - 2300) / 700 : 1;
            const grow = fightEaseOutCubic(Math.min(1, age / 200));
            if (beat.crit) {
                ctx.globalAlpha = 0.35 * fade;
                ctx.fillStyle = "#181818";
                ctx.beginPath();
                ctx.ellipse(target.x, target.y + 5, 11 * grow, 5.5 * grow,
                    0, 0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
            const cracks = beat.crit ? 6 : 3;
            for (let i = 0; i < cracks; i++) {
                const baseAngle = beat.rnd[i] * Math.PI * 2;
                const len = (beat.crit ? 22 + beat.rnd[i + 8] * 24
                    : 13 + beat.rnd[i + 8] * 16) * grow;
                const segments = 4;
                ctx.globalAlpha = 0.7 * fade;
                ctx.strokeStyle = "#1c1c1c";
                ctx.lineWidth = 1.6;
                ctx.beginPath();
                let px = target.x;
                let py = target.y + 4;
                ctx.moveTo(px, py);
                let angle = baseAngle;
                for (let s = 0; s < segments; s++) {
                    angle += (beat.rnd[(i * 4 + s) % 32] * 2 - 1) * 0.5;
                    const step = len / segments;
                    px += Math.cos(angle) * step;
                    py += Math.sin(angle) * step * 0.5;
                    ctx.lineTo(px, py);
                }
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            // The dust puffs.
            const puffs = beat.crit ? 7 : 4;
            if (age < 650) {
                for (let i = 0; i < puffs; i++) {
                    const t = age / 650;
                    const a = beat.rnd[(i + 16) % 32] * Math.PI * 2;
                    const r = 6 + 16 * fightEaseOutQuad(t);
                    ctx.globalAlpha = 0.3 * (1 - t);
                    ctx.fillStyle = "#d8d2c4";
                    ctx.beginPath();
                    ctx.arc(target.x + Math.cos(a) * r,
                        target.y + 4 + Math.sin(a) * r * 0.45
                        - fightEaseOutQuad(t) * 8,
                        2.5 * (1 - t * 0.4), 0, Math.PI * 2);
                    ctx.fill();
                    ctx.globalAlpha = 1;
                }
            }
        }
    },
    draw(ctx, cell, loopT) {
        // A short white tick at the impact moment ties the crack to
        // the blow.
        for (const beat of FightTest.fightActiveBeats(loopT, 200)) {
            const age = loopT - beat.at;
            const impact = FightTest.fightImpact(cell, beat);
            ctx.globalAlpha = 0.9 * (1 - age / 200);
            ctx.fillStyle = "#ffffff";
            ctx.beginPath();
            ctx.arc(impact.x, impact.y, 5 * (1 - age / 200), 0,
                Math.PI * 2);
            ctx.fill();
            ctx.globalAlpha = 1;
        }
    }
},

// 15. The ticker feed: the cell carries its own two line combat log.
// Every beat types itself in as it lands, color coded, with a blinking
// bullet; the older line shifts up. For the reviewer who wants damage
// to read as text without staring at the units.
{
    name: "Ticker feed",
    note: "a mini combat log types the exchange",
    draw(ctx, cell, loopT) {
        const lines = [];
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            if (age >= 0 && age < 2200) {
                const side = beat.attacker === "hero"
                    ? "hero hit keltir" : "keltir hit hero";
                lines.push({ beat, age, side });
            }
        }
        // The newest line sits at the bottom.
        lines.sort((a, b) => a.beat.at - b.beat.at);
        for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            const slide = fightEaseOutQuad(
                Math.min(1, line.age / 200));
            const x = -60 + slide * 66;
            const y = FIGHT_CELL_H - 16 - (lines.length - 1 - i) * 13;
            const alpha = line.age > 1800
                ? 1 - (line.age - 1800) / 400 : 1;
            ctx.globalAlpha = alpha;
            const crit = line.beat.crit;
            const color = crit ? FIGHT_FX.crit
                : (line.beat.attacker === "hero"
                    ? FIGHT_FX.dealtNum : FIGHT_FX.takenNum);
            // The blinking bullet.
            if (line.age < 320 && Math.floor(line.age / 80) % 2 === 0) {
                ctx.fillStyle = color;
                ctx.beginPath();
                ctx.arc(x - 5, y, 2.2, 0, Math.PI * 2);
                ctx.fill();
            } else {
                ctx.fillStyle = color;
                ctx.globalAlpha = alpha * 0.5;
                ctx.beginPath();
                ctx.arc(x - 5, y, 1.6, 0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = alpha;
            }
            const text = (crit ? "CRIT " : "") + line.side + " · "
                + line.beat.amount;
            ctx.font = "bold 9px ui-monospace, Menlo, Consolas, monospace";
            ctx.textAlign = "left";
            ctx.textBaseline = "middle";
            ctx.lineWidth = 2.4;
            ctx.strokeStyle = "rgba(12, 18, 28, 0.85)";
            ctx.strokeText(text, x, y);
            ctx.fillStyle = color;
            ctx.fillText(text, x, y);
            ctx.globalAlpha = 1;
        }
    }
},

// 16. The hitstop punch: on impact the cell freezes - the units stop
// mid swing, the view zooms slightly toward the impact point, radial
// speed lines snap in - and then the world releases. The fighting
// game direction: damage reads as weight, not as decoration.
{
    name: "Hitstop punch",
    note: "a fighting game freeze frame with zoom",
    freeze(cell, loopT) {
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            const hold = beat.crit ? 240 : 120;
            if (age >= 0 && age < hold) {
                return beat.at;
            }
        }
        return null;
    },
    zoom(cell, loopT) {
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            const hold = beat.crit ? 240 : 120;
            const release = hold + 150;
            if (age < 0 || age > release) { continue; }
            const impact = FightTest.fightImpact(cell, beat);
            let k = 1;
            if (age < 60) {
                k = 1 + 0.05 * fightEaseOutQuad(age / 60);
            } else if (age < hold) {
                k = 1.05;
            } else {
                k = 1.05 - 0.05 * fightEaseOutQuad((age - hold) / 150);
            }
            return { k, x: impact.x, y: impact.y };
        }
        return null;
    },
    draw(ctx, cell, loopT) {
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            const hold = beat.crit ? 240 : 120;
            if (age < 0 || age > hold + 260) { continue; }
            const impact = FightTest.fightImpact(cell, beat);
            // The radial speed lines during the freeze.
            if (age < hold) {
                const lines = 8;
                ctx.globalAlpha = 0.55 * (1 - age / hold * 0.5);
                ctx.strokeStyle = "#ffffff";
                ctx.lineWidth = 1.6;
                ctx.lineCap = "round";
                for (let i = 0; i < lines; i++) {
                    const a = (i / lines) * Math.PI * 2
                        + beat.rnd[i] * 0.6;
                    const r0 = 10 + beat.rnd[i + 8] * 5;
                    const len = 9 + beat.rnd[i + 16] * 9;
                    ctx.beginPath();
                    ctx.moveTo(impact.x + Math.cos(a) * r0,
                        impact.y + Math.sin(a) * r0);
                    ctx.lineTo(impact.x + Math.cos(a) * (r0 + len),
                        impact.y + Math.sin(a) * (r0 + len));
                    ctx.stroke();
                }
                ctx.lineCap = "butt";
                ctx.globalAlpha = 1;
            }
            // The clean number after the release, beside the unit so
            // it never collides with the name label above it.
            if (age > hold && age < hold + 260) {
                const t = (age - hold) / 260;
                const target = cell.units[fightBeatTarget(beat)];
                ctx.globalAlpha = 1 - t;
                fightHaloText(ctx, String(beat.amount),
                    target.x + 24, target.y + 2,
                    beat.crit ? FIGHT_FX.crit : "#ffffff",
                    beat.crit ? 15 : 11);
                if (beat.crit) {
                    fightHaloText(ctx, "CRIT",
                        target.x + 24, target.y + 16, FIGHT_FX.crit, 8);
                }
                ctx.globalAlpha = 1;
            }
        }
    }
},

// 17. The beam lance: a straight energy beam fires from the attacker
// through the target - a fast grow, a glowing hold with a white core
// and a flare at the far end, a quick fade. A critical thickens the
// core and streaks a hairline of light across the whole cell.
{
    name: "Beam lance",
    note: "an energy beam lances through the target",
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 460)) {
            const age = loopT - beat.at;
            const from = cell.units[beat.attacker];
            const to = cell.units[fightBeatTarget(beat)];
            const dir = FightTest.fightDir(cell, beat);
            const color = beat.crit ? FIGHT_FX.crit
                : fightSideColor(beat);
            const grow = fightEaseOutQuad(
                Math.min(1, age / 80));
            const fade = age > 220 ? 1 - (age - 220) / 240 : 1;
            const end = age < 80
                ? { x: from.x + (to.x - from.x) * grow,
                    y: from.y + (to.y - from.y) * grow }
                : { x: to.x, y: to.y };
            ctx.globalAlpha = fade;
            // The critical cell wide streak.
            if (beat.crit) {
                ctx.globalAlpha = fade * 0.14;
                ctx.strokeStyle = color;
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.moveTo(from.x - dir.x * 400, from.y - dir.y * 400);
                ctx.lineTo(to.x + dir.x * 400, to.y + dir.y * 400);
                ctx.stroke();
                ctx.globalAlpha = fade;
            }
            // The glow pass, then the core.
            ctx.lineCap = "round";
            ctx.strokeStyle = color;
            ctx.lineWidth = beat.crit ? 14 : 9;
            ctx.globalAlpha = fade * 0.35;
            ctx.beginPath();
            ctx.moveTo(from.x, from.y);
            ctx.lineTo(end.x, end.y);
            ctx.stroke();
            ctx.strokeStyle = "#ffffff";
            ctx.lineWidth = beat.crit ? 4.5 : 2.5;
            ctx.globalAlpha = fade;
            ctx.beginPath();
            ctx.moveTo(from.x, from.y);
            ctx.lineTo(end.x, end.y);
            ctx.stroke();
            ctx.lineCap = "butt";
            // The flares at both ends.
            const flare = age < 220 ? 1 - age / 220 : 0;
            if (flare > 0) {
                for (const point of [from, end]) {
                    ctx.globalAlpha = fade * flare * 0.8;
                    ctx.fillStyle = color;
                    ctx.beginPath();
                    ctx.arc(point.x, point.y,
                        (beat.crit ? 7 : 4.5) * (1.3 - flare * 0.3),
                        0, Math.PI * 2);
                    ctx.fill();
                }
            }
            ctx.globalAlpha = 1;
        }
    }
},

// 18. The attacker aura: satisfaction lives on the dealer. Landing a
// hit flares a glowing aura around the attacker, spins their tick and
// pulses the marker scale; the target only gets a tiny white ping.
// A critical burns a layered flame ring for half a second.
{
    name: "Attacker aura",
    note: "the dealer flares, the target only pings",
    underlay(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 700)) {
            const age = loopT - beat.at;
            const t = age / 700;
            const attacker = cell.units[beat.attacker];
            const color = fightSideColor(beat);
            // The soft glow disc.
            ctx.globalAlpha = 0.35 * (1 - t);
            for (let layer = 0; layer < 3; layer++) {
                ctx.globalAlpha = (0.12 - layer * 0.03) * (1 - t);
                ctx.fillStyle = color;
                ctx.beginPath();
                ctx.arc(attacker.x, attacker.y, 10 + layer * 5
                    + fightEaseOutQuad(t) * 8, 0, Math.PI * 2);
                ctx.fill();
            }
            ctx.globalAlpha = 1;
        }
    },
    unitOffset(name, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 500)) {
            if (name !== beat.attacker) { continue; }
            const age = loopT - beat.at;
            const pulse = Math.sin(Math.PI
                * fightClamp01(age / 320));
            const scale = 1 + (beat.crit ? 0.25 : 0.14) * pulse;
            return { sx: scale, sy: scale };
        }
        return null;
    },
    draw(ctx, cell, loopT) {
        for (const beat of FightTest.fightActiveBeats(loopT, 700)) {
            const age = loopT - beat.at;
            const t = age / 700;
            const attacker = cell.units[beat.attacker];
            const target = cell.units[fightBeatTarget(beat)];
            const color = fightSideColor(beat);
            // The aura rings.
            const rings = beat.crit ? 3 : 1;
            for (let ring = 0; ring < rings; ring++) {
                const rAge = age - ring * 90;
                if (rAge < 0 || rAge > 520) { continue; }
                const rt = rAge / 520;
                ctx.globalAlpha = 0.75 * (1 - rt);
                ctx.strokeStyle = beat.crit ? FIGHT_FX.crit : color;
                ctx.lineWidth = Math.max(1, 3 - rt * 2);
                ctx.beginPath();
                const radius = 9 + 16 * fightEaseOutQuad(rt);
                const wob = beat.crit ? 3 : 0;
                const steps = 26;
                for (let i = 0; i <= steps; i++) {
                    const a = (i / steps) * Math.PI * 2;
                    const r = radius + Math.sin(a * 6 + rAge / 40) * wob;
                    const px = attacker.x + Math.cos(a) * r;
                    const py = attacker.y + Math.sin(a) * r;
                    if (i === 0) { ctx.moveTo(px, py); }
                    else { ctx.lineTo(px, py); }
                }
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
            // The landing ping on the target: a small white blink.
            if (age < 240 && Math.floor(age / 80) % 2 === 0) {
                ctx.globalAlpha = 0.9 * (1 - age / 240);
                ctx.fillStyle = "#ffffff";
                ctx.beginPath();
                ctx.arc(target.x, target.y, 2.4, 0, Math.PI * 2);
                ctx.fill();
                ctx.globalAlpha = 1;
            }
        }
    }
}
];

// FightTest is the engine: it builds the grid DOM, owns the virtual
// clock (pause and speed multiply it) and renders every visible cell
// every frame with the shared beat loop.
const FightTest = {
    grid: null,
    scroll: null,
    cells: [],
    columns: [],
    tile: null,
    clock: 0,
    speed: 1,
    paused: false,
    running: false,
    lastTs: 0,

    // init boots the gallery: the grid DOM, the map tile and the
    // render loop. Called by main.js when /api/config answers the
    // "test-fight" mode.
    init() {
        this.grid = document.getElementById("fight-grid");
        this.scroll = document.getElementById("fight-scroll");
        const pause = document.getElementById("fight-pause");
        pause.addEventListener("click", () => {
            this.paused = !this.paused;
            pause.textContent = this.paused ? "resume" : "pause";
        });
        const speed = document.getElementById("fight-speed");
        speed.addEventListener("change", () => {
            this.speed = parseFloat(speed.value) || 1;
        });
        this.buildGrid();
        this.loadTile();
        this.startLoop();
    },

    // buildGrid assembles the comparison grid: the corner cell, one
    // numbered head per variant, one sticky row head per enemy
    // placement and one canvas cell per (variant, placement). Clicking
    // a head toggles the picked highlight of its whole column so a
    // reviewer can shortlist favorites while scrolling.
    buildGrid() {
        this.grid.style.gridTemplateColumns = FIGHT_ROWHEAD_W + "px repeat("
            + FIGHT_VARIANTS.length + ", " + FIGHT_CELL_W + "px)";
        this.grid.style.gridTemplateRows = "auto repeat("
            + FIGHT_ROWS.length + ", " + FIGHT_CELL_H + "px)";
        const dpr = window.devicePixelRatio || 1;

        const corner = document.createElement("div");
        corner.className = "fight-corner";
        corner.textContent = "variant →\nenemy ↓";
        this.grid.appendChild(corner);

        this.columns = [];
        for (let c = 0; c < FIGHT_VARIANTS.length; c++) {
            const variant = FIGHT_VARIANTS[c];
            const head = document.createElement("div");
            head.className = "fight-colhead";
            const top = document.createElement("div");
            top.className = "fh-top";
            const num = document.createElement("span");
            num.className = "fh-num";
            num.textContent = String(c + 1).padStart(2, "0");
            num.title = "mark this variant";
            const name = document.createElement("span");
            name.className = "fh-name";
            name.textContent = variant.name;
            top.appendChild(num);
            top.appendChild(name);
            const note = document.createElement("div");
            note.className = "fh-note";
            note.textContent = variant.note;
            head.appendChild(top);
            head.appendChild(note);
            head.addEventListener("click", () => this.togglePick(c));
            this.grid.appendChild(head);
            this.columns.push({ head, cells: [] });
        }

        this.cells = [];
        for (let r = 0; r < FIGHT_ROWS.length; r++) {
            const rowHead = document.createElement("div");
            rowHead.className = "fight-rowhead";
            rowHead.textContent = FIGHT_ROWS[r].label;
            this.grid.appendChild(rowHead);

            for (let c = 0; c < FIGHT_VARIANTS.length; c++) {
                const canvas = document.createElement("canvas");
                canvas.className = "fxg-cell";
                canvas.width = Math.round(FIGHT_CELL_W * dpr);
                canvas.height = Math.round(FIGHT_CELL_H * dpr);
                canvas.style.width = FIGHT_CELL_W + "px";
                canvas.style.height = FIGHT_CELL_H + "px";
                this.grid.appendChild(canvas);
                const cell = {
                    variant: FIGHT_VARIANTS[c],
                    variantIndex: c,
                    row: r,
                    col: c,
                    canvas,
                    ctx: canvas.getContext("2d"),
                    hero: { x: FIGHT_CELL_W / 2, y: FIGHT_CELL_H / 2 - 1 },
                    enemy: null,
                    units: { hero: { x: 0, y: 0 }, enemy: { x: 0, y: 0 } }
                };
                cell.enemy = {
                    x: cell.hero.x + FIGHT_ROWS[r].dx,
                    y: cell.hero.y + FIGHT_ROWS[r].dy
                };
                this.cells.push(cell);
                this.columns[c].cells.push(cell);
            }
        }
    },

    // togglePick highlights or clears the picked state of one variant
    // column (the heads and the cells share the class).
    togglePick(index) {
        const column = this.columns[index];
        if (!column) { return; }
        const picked = !column.head.classList.contains("picked");
        column.head.classList.toggle("picked", picked);
        for (const cell of column.cells) {
            cell.canvas.classList.toggle("picked", picked);
        }
    },

    // loadTile fetches the world map tile the cells use as their
    // background. The engine falls back to a plain terrain fill while
    // the image is in flight (and forever if it is unavailable).
    loadTile() {
        const image = new Image();
        image.onload = () => { this.tile = image; };
        image.onerror = () => { this.tile = null; };
        image.src = FIGHT_TILE_URL;
    },

    // startLoop runs the requestAnimationFrame loop: the virtual
    // clock advances by dt * speed unless paused, then every visible
    // cell is repainted from the shared loop time.
    startLoop() {
        this.running = true;
        this.lastTs = performance.now();
        requestAnimationFrame((ts) => this.frame(ts));
    },

    frame(ts) {
        const dt = Math.min(0.1, (ts - this.lastTs) / 1000);
        this.lastTs = ts;
        if (!this.paused) {
            this.clock += dt * 1000 * this.speed;
        }
        this.render();
        if (this.running) {
            requestAnimationFrame((next) => this.frame(next));
        }
    },

    // render repaints the cells of the visible column window only: a
    // column's x offset is exactly FIGHT_ROWHEAD_W + col * CELL_W, so
    // the scroll position yields the range without reading the DOM
    // layout (the harness sandbox has none).
    render() {
        const loopT = ((this.clock % FIGHT_LOOP_MS) + FIGHT_LOOP_MS)
            % FIGHT_LOOP_MS;
        const scrollLeft = this.scroll.scrollLeft || 0;
        const viewW = this.scroll.clientWidth || FIGHT_CELL_W * 4;
        for (const cell of this.cells) {
            const x0 = FIGHT_ROWHEAD_W + cell.col * FIGHT_CELL_W;
            if (x0 + FIGHT_CELL_W < scrollLeft - 80
                || x0 > scrollLeft + viewW + 80) {
                continue;
            }
            this.renderCell(cell, loopT);
        }
    },

    // renderCell paints one cell: the map background, the optional
    // underlay fx, the two units (with the beat lunge and the variant
    // unit offsets), the variant fx layer and the beat caption. The
    // optional hooks of the variant wrap the whole paint: cellShake
    // translates, zoom scales around a point, freeze holds the lunge
    // clock.
    renderCell(cell, loopT) {
        const ctx = cell.ctx;
        const v = cell.variant;
        if (!ctx) { return; }
        ctx.setTransform(window.devicePixelRatio || 1, 0, 0,
            window.devicePixelRatio || 1, 0, 0);
        ctx.clearRect(0, 0, FIGHT_CELL_W, FIGHT_CELL_H);

        const shake = v.cellShake ? (v.cellShake(cell, loopT) || null)
            : null;
        const zoom = v.zoom ? (v.zoom(cell, loopT) || null) : null;

        ctx.save();
        if (zoom && zoom.k !== 1) {
            ctx.translate(zoom.x, zoom.y);
            ctx.scale(zoom.k, zoom.k);
            ctx.translate(-zoom.x, -zoom.y);
        }
        if (shake) {
            ctx.translate(shake.x, shake.y);
        }

        this.drawMapBackground(ctx);
        if (v.underlay) { v.underlay(ctx, cell, loopT); }

        let unitT = loopT;
        if (v.freeze) {
            const frozen = v.freeze(cell, loopT);
            if (frozen !== null && frozen !== undefined) {
                unitT = frozen;
            }
        }
        this.drawUnits(ctx, cell, unitT, loopT);
        if (v.draw) { v.draw(ctx, cell, loopT); }
        ctx.restore();

        this.drawCaption(ctx, cell, loopT);
    },

    // drawMapBackground paints the world map tile crop of the starter
    // meadow, stretched to the cell. The world crop maps to tile
    // pixels by TILE_PX / TILE_UNITS; the fallback is a flat terrain
    // fill with the grid lines of the map so the cells still read as
    // map views when the tile is unavailable.
    drawMapBackground(ctx) {
        if (this.tile && this.tile.complete
            && this.tile.naturalWidth > 0) {
            const scale = FIGHT_TILE_PX / FIGHT_TILE_UNITS;
            const cx = (FIGHT_BG_CENTER.x - FIGHT_TILE_ORIGIN.x) * scale;
            const cy = (FIGHT_BG_CENTER.y - FIGHT_TILE_ORIGIN.y) * scale;
            const sw = FIGHT_BG_WORLD_W * scale;
            const sh = sw * FIGHT_CELL_H / FIGHT_CELL_W;
            ctx.drawImage(this.tile, cx - sw / 2, cy - sh / 2, sw, sh,
                0, 0, FIGHT_CELL_W, FIGHT_CELL_H);
        } else {
            ctx.fillStyle = "#e8e2d2";
            ctx.fillRect(0, 0, FIGHT_CELL_W, FIGHT_CELL_H);
            ctx.strokeStyle = "rgba(21, 34, 50, 0.08)";
            ctx.lineWidth = 1;
            for (let x = 30; x < FIGHT_CELL_W; x += 30) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, FIGHT_CELL_H);
                ctx.stroke();
            }
            for (let y = 30; y < FIGHT_CELL_H; y += 30) {
                ctx.beginPath();
                ctx.moveTo(0, y);
                ctx.lineTo(FIGHT_CELL_W, y);
                ctx.stroke();
            }
        }
        // A soft dark frame keeps the cell edges readable against the
        // light imagery without hiding the terrain.
        const edge = ctx.createLinearGradient(0, 0, 0, FIGHT_CELL_H);
        edge.addColorStop(0, "rgba(10, 14, 20, 0.16)");
        edge.addColorStop(0.5, "rgba(10, 14, 20, 0)");
        edge.addColorStop(1, "rgba(10, 14, 20, 0.12)");
        ctx.fillStyle = edge;
        ctx.fillRect(0, 0, FIGHT_CELL_W, FIGHT_CELL_H);
    },

    // drawUnits paints the hero and the enemy the way the live map
    // does: a filled circle with the look direction tick outside the
    // edge, the name above and the HP bar below. The attacker of the
    // beat in its lunge window steps toward the target (the melee
    // approach of the game), the variant unitOffset hook adds the per
    // variant motion (knockback, strobe flashes, aura scale).
    drawUnits(ctx, cell, unitT, loopT) {
        const v = cell.variant;
        for (const name of ["hero", "enemy"]) {
            const base = name === "hero" ? cell.hero : cell.enemy;
            let x = base.x;
            let y = base.y;
            const beat = this.lungeBeat(unitT);
            if (beat) {
                const from = name === "hero" ? cell.hero : cell.enemy;
                const to = name === "hero" ? cell.enemy : cell.hero;
                const dx = to.x - from.x;
                const dy = to.y - from.y;
                const dist = Math.hypot(dx, dy) || 1;
                const k = this.lungeAt(beat, unitT);
                if (beat.attacker === name && k > 0) {
                    x += (dx / dist) * dist * k;
                    y += (dy / dist) * dist * k;
                }
            }
            let mod = null;
            if (v.unitOffset) {
                mod = v.unitOffset(name, cell, loopT) || null;
            }
            if (mod) {
                x += mod.x || 0;
                y += mod.y || 0;
            }
            cell.units[name].x = x;
            cell.units[name].y = y;
            cell.units[name].mod = mod;
        }

        for (const name of ["hero", "enemy"]) {
            const p = cell.units[name];
            const other = cell.units[name === "hero" ? "enemy" : "hero"];
            const angle = Math.atan2(other.y - p.y, other.x - p.x);
            const mod = cell.units[name].mod;
            this.drawUnit(ctx, p.x, p.y,
                name === "hero" ? 6 : 5.5,
                name === "hero" ? FIGHT_FX.hero : FIGHT_FX.mob,
                angle, mod);
            const label = name === "hero" ? "HERO" : "KELTIR";
            fightHaloText(ctx, label, p.x, p.y - 14,
                "#ffffff", 8, "center");
            this.drawHPBar(ctx, name, p.x, p.y + 12, loopT);
        }
    },

    // drawUnit paints one unit marker: the circle body with the dark
    // outline, the look tick outside the edge and the optional flash
    // overlay of the variant hooks (a white or red strobe).
    drawUnit(ctx, x, y, radius, fill, angle, mod) {
        const sx = mod && mod.sx ? mod.sx : 1;
        const sy = mod && mod.sy ? mod.sy : 1;
        const rot = mod && mod.rot ? mod.rot : 0;
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(rot);
        ctx.scale(sx, sy);
        ctx.beginPath();
        ctx.arc(0, 0, radius, 0, Math.PI * 2);
        ctx.fillStyle = fill;
        ctx.fill();
        ctx.lineWidth = 1.25;
        ctx.strokeStyle = FIGHT_FX.ink;
        ctx.stroke();
        const outer = radius + 4.5;
        ctx.beginPath();
        ctx.moveTo(Math.cos(angle) * radius, Math.sin(angle) * radius);
        ctx.lineTo(Math.cos(angle) * outer, Math.sin(angle) * outer);
        ctx.lineWidth = 2;
        ctx.lineCap = "round";
        ctx.strokeStyle = FIGHT_FX.ink;
        ctx.stroke();
        ctx.lineCap = "butt";
        if (mod && mod.flash > 0) {
            ctx.globalAlpha = Math.min(1, mod.flash);
            ctx.beginPath();
            ctx.arc(0, 0, radius, 0, Math.PI * 2);
            ctx.fillStyle = mod.flashColor || "#ffffff";
            ctx.fill();
            ctx.globalAlpha = 1;
        }
        ctx.restore();
    },

    // drawHPBar paints the small HP bar under a unit. The values come
    // from the beat timeline: the hero starts at 100, the enemy at
    // 200, every beat that already landed subtracts its amount.
    drawHPBar(ctx, name, x, y, loopT) {
        const max = name === "hero" ? 100 : 200;
        const hp = this.hpOf(name, loopT);
        const w = 44;
        const h = 4;
        ctx.fillStyle = "rgba(10, 14, 20, 0.55)";
        ctx.fillRect(x - w / 2 - 1, y - 1, w + 2, h + 2);
        ctx.fillStyle = name === "hero" ? "#2ecc40" : "#ff7b25";
        ctx.fillRect(x - w / 2, y, Math.max(0, w * hp / max), h);
    },

    // hpOf integrates the beat loop into the current HP of one side.
    hpOf(name, loopT) {
        let hp = name === "hero" ? 100 : 200;
        for (const beat of FIGHT_BEATS) {
            if (loopT < beat.at) { continue; }
            if (beat.attacker === "hero" && name === "enemy") {
                hp -= beat.amount;
            }
            if (beat.attacker === "enemy" && name === "hero") {
                hp -= beat.amount;
            }
        }
        return Math.max(0, hp);
    },

    // lungeBeat returns the beat whose attack lunge window covers the
    // given unit clock, or null.
    lungeBeat(t) {
        for (const beat of FIGHT_BEATS) {
            if (t >= beat.at - 400 && t <= beat.at + 250) {
                return beat;
            }
        }
        return null;
    },

    // lungeAt is the lunge envelope: the attacker steps 36% of the
    // distance toward the target, easing in until the impact moment
    // and gliding back after it.
    lungeAt(beat, t) {
        if (t < beat.at - 400 || t > beat.at + 250) { return 0; }
        if (t < beat.at) {
            return fightEaseInQuad((t - (beat.at - 400)) / 400) * 0.36;
        }
        return (1 - fightEaseOutQuad((t - beat.at) / 250)) * 0.36;
    },

    // drawCaption paints the beat indicator of the cell, the same one
    // in every variant: the latest beat stays labeled for 1.2 s so a
    // scrolling reviewer always knows which half of the exchange they
    // are watching.
    drawCaption(ctx, cell, loopT) {
        let beat = null;
        for (const b of FIGHT_BEATS) {
            if (loopT >= b.at && loopT - b.at < 1200) { beat = b; }
        }
        if (!beat) { return; }
        let text;
        let color;
        if (beat.attacker === "hero") {
            text = beat.crit ? "HERO CRIT! " + beat.amount
                : "HERO HIT " + beat.amount;
            color = beat.crit ? FIGHT_FX.crit : FIGHT_FX.dealtNum;
        } else {
            text = beat.crit ? "CRIT! TAKEN " + beat.amount
                : "TAKEN " + beat.amount;
            color = beat.crit ? FIGHT_FX.crit : FIGHT_FX.takenNum;
        }
        fightHaloText(ctx, text, 6, 11, color, 9, "left");
    },

    // fightImpact returns the impact point of a beat in cell pixels:
    // the hurt unit center nudged a bit toward the attacker.
    fightImpact(cell, beat) {
        const target = cell.units[fightBeatTarget(beat)];
        const attacker = cell.units[beat.attacker];
        const dx = attacker.x - target.x;
        const dy = attacker.y - target.y;
        const dist = Math.hypot(dx, dy) || 1;
        return {
            x: target.x + (dx / dist) * 5,
            y: target.y + (dy / dist) * 5
        };
    },

    // fightDir returns the unit vector of the hit direction (from the
    // attacker toward the target) of a beat in cell pixels.
    fightDir(cell, beat) {
        const attacker = cell.units[beat.attacker];
        const target = cell.units[fightBeatTarget(beat)];
        const dx = target.x - attacker.x;
        const dy = target.y - attacker.y;
        const dist = Math.hypot(dx, dy) || 1;
        return { x: dx / dist, y: dy / dist };
    },

    // fightBarRect returns the HP bar geometry of one side (the base
    // position, not the knocked one - the HP chunk variant wants the
    // stable rect).
    fightBarRect(cell, name) {
        const base = name === "hero" ? cell.hero : cell.enemy;
        return { x: base.x - 22, y: base.y + 12, w: 44, h: 4 };
    },

    // fightBeatAge returns the ms since the impact of a beat at the
    // loop time, or -1 when the beat has not landed yet.
    fightBeatAge(beat, loopT) {
        return loopT - beat.at;
    },

    // fightActiveBeats yields the beats whose fx window could be open
    // (age from 0 up to life). The variants pass their life time.
    fightActiveBeats(loopT, life) {
        const out = [];
        for (const beat of FIGHT_BEATS) {
            const age = loopT - beat.at;
            if (age >= 0 && age <= life) { out.push(beat); }
        }
        return out;
    },

    // fightHPBefore returns the HP of the target of a beat right
    // before its impact (the width the bar just lost).
    fightHPBefore(beat) {
        const target = fightBeatTarget(beat);
        let hp = target === "hero" ? 100 : 200;
        for (const b of FIGHT_BEATS) {
            if (b === beat) { break; }
            if (b.at < beat.at
                && fightBeatTarget(b) === target) {
                hp -= b.amount;
            }
        }
        return hp;
    }
};
