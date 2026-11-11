/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// PathCurve is the smooth curve animation lab of the -test-path-ui-v2
// mode (webserver.NewTestPathCurveServer answers mode "test-path-v2").
// It is the follow-up of the walk plan gallery: the owner picked the
// Catmull-Rom curve as the base, so all thirty numbered variants draw
// the same smooth walk and differ in motion first (01-21), then color
// (22-25) and stroke approach (26-30).
//
// The layout and the clock match the first gallery one to one: a four
// column grid of cards, the same seven point route with five stops,
// the same travel, dwell, hold and fade budget, and the same walker.
// Every effect is a pure function of the loop clock, so a repaint of
// the same moment is identical and the Node harness can assert that
// every variant moves.
//
// The curve is a real Catmull-Rom resampling of the route: the walker
// rides the curve itself (not the polyline), the arc length drives the
// timeline and every stroke samples the curve. The route control
// points stay the stop positions, so the walk passes through and
// dwells at every stop.
const CURVE_CELL_W = 380;
const CURVE_CELL_H = 250;

// The grid columns; four cards fit a 1600 px viewport, the strip
// scrolls both ways otherwise.
const CURVE_COLUMNS = 4;

// The map background: the open meadow crop of the 21_19 world tile,
// the terrain the bot actually hunts.
const CURVE_TILE_URL = "maps/0/21_19.jpg";
const CURVE_TILE_PX = 1024;
const CURVE_TILE_UNITS = 32768;
const CURVE_TILE_ORIGIN = { x: 32768, y: 32768 };
const CURVE_BG_CENTER = { x: 43328, y: 43648 };
const CURVE_BG_WORLD_W = 1900;

// The shared planned walk: the planning origin, five stops (the
// follower cursor targets) and the destination, the exact route of
// the first gallery.
const CURVE_ROUTE = [
    { x: 46, y: 202 },
    { x: 92, y: 170 },
    { x: 140, y: 160 },
    { x: 176, y: 118 },
    { x: 232, y: 106 },
    { x: 286, y: 128 },
    { x: 334, y: 88 }
];

// The animation budget of one loop: the moving time over the whole
// curve, the dwell at every intermediate stop, the arrival hold and
// the fade before the restart.
const CURVE_TRAVEL_MS = 4200;
const CURVE_DWELL_MS = 240;
const CURVE_HOLD_MS = 900;
const CURVE_RESET_MS = 300;

// The palette: the live magenta of the walk plan plus the accent
// colors the variants test against the light meadow imagery.
const CURVE_FX = {
    magenta: "#ff44cc",
    glow: "#ff8ae2",
    cyan: "#17c6de",
    amber: "#ffb020",
    lime: "#7fc81a",
    violet: "#a78bfa",
    white: "#ffffff",
    ink: "#1b2430",
    dim: "#78828f",
    self: "#1a73e8",
    dest: "#ff7a1a",
    gold: "#ffd166",
    teal: "#2dd4bf",
    rose: "#fb7185"
};

// CURVE_SMOOTH is the Catmull-Rom resampling of the route (16 samples
// per segment) with the arc distance of every sample. Every stroke,
// the walker and the timeline read this single curve.
const CURVE_SMOOTH = (() => {
    const route = CURVE_ROUTE;
    const out = [];
    for (let i = 0; i < route.length - 1; i++) {
        const p0 = route[Math.max(0, i - 1)];
        const p1 = route[i];
        const p2 = route[i + 1];
        const p3 = route[Math.min(route.length - 1, i + 2)];
        for (let s = 0; s < 16; s++) {
            const k = s / 16;
            const k2 = k * k;
            const k3 = k2 * k;
            out.push({
                x: 0.5 * ((2 * p1.x) + (-p0.x + p2.x) * k +
                    (2 * p0.x - 5 * p1.x + 4 * p2.x - p3.x) * k2 +
                    (-p0.x + 3 * p1.x - 3 * p2.x + p3.x) * k3),
                y: 0.5 * ((2 * p1.y) + (-p0.y + p2.y) * k +
                    (2 * p0.y - 5 * p1.y + 4 * p2.y - p3.y) * k2 +
                    (-p0.y + 3 * p1.y - 3 * p2.y + p3.y) * k3)
            });
        }
    }
    const last = route[route.length - 1];
    out.push({ x: last.x, y: last.y });
    let d = 0;
    out[0].dist = 0;
    for (let i = 1; i < out.length; i++) {
        d += Math.hypot(out[i].x - out[i - 1].x,
            out[i].y - out[i - 1].y);
        out[i].dist = d;
    }
    return out;
})();

// CURVE_STOP_DIST is the arc length of every route control point: the
// resampling hits each control point exactly at sample 16 * index.
const CURVE_STOP_DIST = CURVE_ROUTE.map((p, i) => (
    i < CURVE_ROUTE.length - 1
        ? CURVE_SMOOTH[i * 16].dist
        : CURVE_SMOOTH[CURVE_SMOOTH.length - 1].dist
));

// CURVE_STOPS is the smooth curve point of every stop with its
// outgoing direction, the anchor of the stop markers.
const CURVE_STOPS = CURVE_ROUTE.map((p, i) => {
    const a = CURVE_ROUTE[i];
    const b = CURVE_ROUTE[Math.min(CURVE_ROUTE.length - 1, i + 1)];
    const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
    return {
        x: p.x,
        y: p.y,
        dist: CURVE_STOP_DIST[i],
        dirX: (b.x - a.x) / len,
        dirY: (b.y - a.y) / len
    };
});

// CURVE_STRANDS is the double strand geometry: every smooth sample
// pushed to both sides of its normal, precomputed once.
const CURVE_STRANDS = (() => {
    const left = [];
    const right = [];
    const samples = CURVE_SMOOTH;
    for (let i = 0; i < samples.length; i++) {
        const p = samples[i];
        const prev = samples[Math.max(0, i - 1)];
        const next = samples[Math.min(samples.length - 1, i + 1)];
        const len = Math.hypot(next.x - prev.x, next.y - prev.y) || 1;
        const nx = -(next.y - prev.y) / len;
        const ny = (next.x - prev.x) / len;
        left.push({ x: p.x + nx * 3.2, y: p.y + ny * 3.2 });
        right.push({ x: p.x - nx * 3.2, y: p.y - ny * 3.2 });
    }
    return { left: left, right: right };
})();

// CURVE_TIMING is the arc length table of the smooth curve: the per
// stop distances, the arrival time of every stop (dwells included) and
// the loop length. Computed once at load, every variant and the walker
// read it.
const CURVE_TIMING = (() => {
    const stops = CURVE_STOP_DIST;
    const lens = [];
    let total = 0;
    for (let i = 1; i < stops.length; i++) {
        const len = stops[i] - stops[i - 1];
        lens.push(len);
        total += len;
    }
    const perMs = CURVE_TRAVEL_MS / total;
    const prefix = [0];
    const arrivals = [0];
    let t = 0;
    for (let i = 1; i < stops.length; i++) {
        prefix.push(prefix[i - 1] + lens[i - 1]);
        t += lens[i - 1] * perMs;
        arrivals.push(t);
        if (i < stops.length - 1) { t += CURVE_DWELL_MS; }
    }
    const arriveAt = arrivals[arrivals.length - 1];
    return {
        lens: lens,
        total: total,
        prefix: prefix,
        arrivals: arrivals,
        arriveAt: arriveAt,
        loopMs: arriveAt + CURVE_HOLD_MS + CURVE_RESET_MS
    };
})();

// curveRgba expands a #rrggbb color into an rgba() string.
function curveRgba(hex, alpha) {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return "rgba(" + r + ", " + g + ", " + b + ", " + alpha + ")";
}

// curvePointAtDistance returns the point, the unit direction and the
// arc length of one position on the smooth curve.
function curvePointAtDistance(distance) {
    const samples = CURVE_SMOOTH;
    const total = samples[samples.length - 1].dist;
    const d = Math.max(0, Math.min(total, distance));
    let index = samples.length - 2;
    for (let i = 0; i < samples.length - 1; i++) {
        if (d <= samples[i + 1].dist) {
            index = i;
            break;
        }
    }
    const a = samples[index];
    const b = samples[index + 1];
    const span = Math.max(1e-6, b.dist - a.dist);
    const k = Math.max(0, Math.min(1, (d - a.dist) / span));
    return {
        x: a.x + (b.x - a.x) * k,
        y: a.y + (b.y - a.y) * k,
        dirX: (b.x - a.x) / span,
        dirY: (b.y - a.y) / span,
        dist: d
    };
}

// curveRange returns the smooth curve between two arc lengths as a
// polyline the stroke helpers can paint.
function curveRange(from, to) {
    const samples = CURVE_SMOOTH;
    const total = samples[samples.length - 1].dist;
    const a = Math.max(0, Math.min(total, from));
    const b = Math.max(0, Math.min(total, to));
    if (b - a < 0.5) { return []; }
    const head = curvePointAtDistance(a);
    const out = [{ x: head.x, y: head.y }];
    for (const sample of samples) {
        if (sample.dist <= a + 1e-6 || sample.dist >= b - 1e-6) {
            continue;
        }
        out.push({ x: sample.x, y: sample.y });
    }
    const tail = curvePointAtDistance(b);
    out.push({ x: tail.x, y: tail.y });
    return out;
}

// curveSamples returns the curve points every spacing pixels starting
// at the phase offset, the direction of each included.
function curveSamples(spacing, phase) {
    const out = [];
    let d = phase === undefined ? 0 : phase;
    while (d <= CURVE_TIMING.total) {
        out.push(curvePointAtDistance(d));
        d += spacing;
    }
    return out;
}

// curveStroke strokes one polyline with the lab style.
function curveStroke(ctx, points, style) {
    if (!points || points.length < 2) { return; }
    ctx.save();
    ctx.strokeStyle = style.color;
    ctx.globalAlpha = style.alpha === undefined ? 1 : style.alpha;
    ctx.lineWidth = style.width;
    ctx.lineCap = style.cap || "round";
    ctx.lineJoin = "round";
    ctx.setLineDash(style.dash || []);
    ctx.lineDashOffset = style.offset || 0;
    ctx.beginPath();
    ctx.moveTo(points[0].x, points[0].y);
    for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y);
    }
    ctx.stroke();
    ctx.restore();
}

// curveTick draws one perpendicular tick across the curve.
function curveTick(ctx, x, y, dirX, dirY, half, style) {
    const nx = -dirY;
    const ny = dirX;
    ctx.save();
    ctx.strokeStyle = style.color;
    ctx.globalAlpha = style.alpha === undefined ? 1 : style.alpha;
    ctx.lineWidth = style.width || 1.4;
    ctx.beginPath();
    ctx.moveTo(x - nx * half, y - ny * half);
    ctx.lineTo(x + nx * half, y + ny * half);
    ctx.stroke();
    ctx.restore();
}

// curveDot draws one filled marker with an optional rim.
function curveDot(ctx, x, y, radius, style) {
    ctx.save();
    ctx.globalAlpha = style.alpha === undefined ? 1 : style.alpha;
    ctx.fillStyle = style.fill;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fill();
    if (style.ring) {
        ctx.strokeStyle = style.ring;
        ctx.lineWidth = style.ringWidth || 1;
        ctx.stroke();
    }
    ctx.restore();
}

// curvePulse draws one expanding ring of the 0..1 phase.
function curvePulse(ctx, x, y, phase, style) {
    const k = Math.max(0, Math.min(1, phase));
    ctx.save();
    ctx.strokeStyle = style.color;
    ctx.globalAlpha = (style.alpha === undefined ? 1 : style.alpha)
        * (1 - k);
    ctx.lineWidth = style.width || 1.6;
    ctx.beginPath();
    ctx.arc(x, y, style.from + (style.to - style.from) * k,
        0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
}

// curveGlow paints the radial halo of a bright head.
function curveGlow(ctx, x, y, radius, hex, alpha) {
    if (typeof ctx.createRadialGradient !== "function") { return; }
    const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius);
    gradient.addColorStop(0, curveRgba(hex, alpha));
    gradient.addColorStop(1, curveRgba(hex, 0));
    ctx.save();
    ctx.fillStyle = gradient;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// curveStar draws one four point sparkle.
function curveStar(ctx, x, y, size, color, alpha) {
    ctx.save();
    ctx.strokeStyle = color;
    ctx.globalAlpha = alpha === undefined ? 1 : alpha;
    ctx.lineWidth = 1;
    ctx.lineCap = "round";
    ctx.beginPath();
    ctx.moveTo(x - size, y);
    ctx.lineTo(x + size, y);
    ctx.moveTo(x, y - size);
    ctx.lineTo(x, y + size);
    ctx.stroke();
    ctx.restore();
}

// curveNib draws the pen nib triangle pointing along the direction.
function curveNib(ctx, x, y, dirX, dirY, size, color) {
    const nx = -dirY;
    const ny = dirX;
    ctx.save();
    ctx.fillStyle = color;
    ctx.strokeStyle = "rgba(255, 255, 255, 0.9)";
    ctx.lineWidth = 0.9;
    ctx.beginPath();
    ctx.moveTo(x + dirX * size, y + dirY * size);
    ctx.lineTo(x - dirX * size * 0.4 + nx * size * 0.55,
        y - dirY * size * 0.4 + ny * size * 0.55);
    ctx.lineTo(x - dirX * size * 1.1, y - dirY * size * 1.1);
    ctx.lineTo(x - dirX * size * 0.4 - nx * size * 0.55,
        y - dirY * size * 0.4 - ny * size * 0.55);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
    ctx.restore();
}

// curveStops walks the intermediate stops of the route (the planning
// origin and the destination excluded).
function curveStops(ctx, draw) {
    for (let i = 1; i < CURVE_STOPS.length - 1; i++) {
        draw(CURVE_STOPS[i], i);
    }
}

// curveDestRing is the shared breathing ring: a small core plus one
// expanding ring, the calmest destination marker of the lab.
function curveDestRing(ctx, t, color) {
    const dest = CURVE_ROUTE[CURVE_ROUTE.length - 1];
    curvePulse(ctx, dest.x, dest.y, (t % 1200) / 1200, {
        from: 3.5, to: 11, color: color, alpha: 0.7, width: 1.4
    });
    curveDot(ctx, dest.x, dest.y, 2.8, {
        fill: color, ring: "rgba(255, 255, 255, 0.95)", ringWidth: 1.1
    });
}

// curveDestTarget is the bullseye destination: one pulse ring and two
// concentric strokes.
function curveDestTarget(ctx, t, color) {
    const dest = CURVE_ROUTE[CURVE_ROUTE.length - 1];
    curvePulse(ctx, dest.x, dest.y, (t % 1100) / 1100, {
        from: 3, to: 10.5, color: color, alpha: 0.65, width: 1.2
    });
    ctx.save();
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.arc(dest.x, dest.y, 3.6, 0, Math.PI * 2);
    ctx.stroke();
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(dest.x, dest.y, 1.4, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// curveDestPin is the teardrop pin of the classic map style.
function curveDestPin(ctx, t, color) {
    const dest = CURVE_ROUTE[CURVE_ROUTE.length - 1];
    const x = dest.x;
    const y = dest.y;
    const breathe = 0.5 + 0.5 * Math.sin(t / 340);
    ctx.save();
    ctx.globalAlpha = 0.2 + 0.16 * breathe;
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(x, y + 1.5, 5 + 2 * breathe, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = 1;
    ctx.fillStyle = color;
    ctx.strokeStyle = "rgba(24, 30, 38, 0.85)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.arc(x, y - 7, 4.4, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(x - 2.7, y - 4.6);
    ctx.lineTo(x, y + 1.4);
    ctx.lineTo(x + 2.7, y - 4.6);
    ctx.closePath();
    ctx.fill();
    ctx.fillStyle = "#ffffff";
    ctx.beginPath();
    ctx.arc(x, y - 7, 1.6, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// curveWalkAt maps the loop clock to the walker state: the curve
// point, the direction, the walked arc length, the number of passed
// stops and the arrival phase. The walker dwells at every intermediate
// stop and holds at the destination before the loop fades and
// restarts.
function curveWalkAt(clockMs) {
    const timing = CURVE_TIMING;
    const last = CURVE_ROUTE.length - 1;
    const loop = timing.loopMs;
    const t = ((clockMs % loop) + loop) % loop;
    if (t >= timing.arriveAt) {
        const dest = CURVE_ROUTE[last];
        return {
            t: t,
            x: dest.x,
            y: dest.y,
            dirX: 1,
            dirY: 0,
            dist: timing.total,
            arrived: true,
            passedStops: last - 1,
            sinceArrive: t - timing.arriveAt,
            resetting: t >= timing.arriveAt + CURVE_HOLD_MS
        };
    }
    let seg = 0;
    for (let i = 0; i < last; i++) {
        if (t < timing.arrivals[i + 1]) {
            seg = i;
            break;
        }
    }
    const dwellEnd = timing.arrivals[seg] +
        (seg >= 1 ? CURVE_DWELL_MS : 0);
    let dist = timing.prefix[seg];
    if (t >= dwellEnd) {
        const span = timing.arrivals[seg + 1] - dwellEnd;
        const k = span > 0 ? (t - dwellEnd) / span : 1;
        dist += timing.lens[seg] * Math.max(0, Math.min(1, k));
    }
    const p = curvePointAtDistance(dist);
    let passed = 0;
    for (let i = 1; i < last; i++) {
        if (timing.prefix[i] <= dist + 1e-6) { passed++; }
    }
    return {
        t: t,
        x: p.x,
        y: p.y,
        dirX: p.dirX,
        dirY: p.dirY,
        dist: dist,
        arrived: false,
        passedStops: passed,
        sinceArrive: -1,
        resetting: false
    };
}

// curveWave is the 0..1 travelling wave several variants sample: the
// position along the curve sets the phase, the clock moves it.
function curveWave(dist, t, wavelength, periodMs) {
    return 0.5 + 0.5 * Math.sin((dist / wavelength - t / periodMs)
        * Math.PI * 2);
}

// curveRainbowHue is the hue of the rainbow flow at the curve
// fraction k: the clock subtracts, so a fixed hue stripe moves toward
// the destination as time runs.
function curveRainbowHue(k, t) {
    return ((k * 320 - t / 11) % 360 + 360) % 360;
}

// curveNoise is the deterministic 0..1 scatter of the star field: the
// same index returns the same value, so a repaint never flickers.
function curveNoise(index) {
    const v = Math.sin(index * 12.9898) * 43758.5453;
    return v - Math.floor(v);
}

// curveWobble displaces every sample along its normal, the elastic
// band of the approach group.
function curveWobble(amount, phase) {
    const samples = CURVE_SMOOTH;
    const out = [];
    for (let i = 0; i < samples.length; i++) {
        const p = samples[i];
        const prev = samples[Math.max(0, i - 1)];
        const next = samples[Math.min(samples.length - 1, i + 1)];
        const len = Math.hypot(next.x - prev.x, next.y - prev.y) || 1;
        const nx = -(next.y - prev.y) / len;
        const ny = (next.x - prev.x) / len;
        const off = Math.sin(p.dist / 55 - phase) * amount;
        out.push({ x: p.x + nx * off, y: p.y + ny * off });
    }
    return out;
}

// CURVE_VARIANTS is the lab itself: every entry draws the shared
// smooth walk and differs only in the motion, the color or the stroke
// approach named in its head.
const CURVE_VARIANTS = [
    {
        group: "motion",
        name: "sliding light window",
        note: "base curve · white window rides the walker · ring",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 5.5, alpha: 0.14
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 2.4, alpha: 0.9
            });
            const windowPts = curveRange(walk.dist - 46,
                walk.dist + 46);
            curveStroke(ctx, windowPts, {
                color: CURVE_FX.glow, width: 9, alpha: 0.22
            });
            curveStroke(ctx, windowPts, {
                color: CURVE_FX.white, width: 2.6, alpha: 0.92
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.3, {
                fill: CURVE_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "comet head",
        note: "96 px glow tail · white hot head · faint plan",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 1.3, alpha: 0.28,
                dash: [3, 7]
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.9, {
                fill: CURVE_FX.magenta, alpha: 0.7,
                ring: "rgba(255, 255, 255, 0.5)", ringWidth: 0.9
            }));
            const from = Math.max(0, walk.dist - 96);
            let prev = curvePointAtDistance(from);
            for (let d = from + 6; d <= walk.dist; d += 6) {
                const p = curvePointAtDistance(d);
                const k = Math.max(0, Math.min(1,
                    (p.dist - from) / 96));
                curveStroke(ctx, [prev, p], {
                    color: k > 0.82 ? CURVE_FX.white
                        : CURVE_FX.glow,
                    width: 1.2 + 2.8 * k, alpha: 0.08 + 0.85 * k
                });
                prev = p;
            }
            curveDestRing(ctx, t, CURVE_FX.dest);
        },
        underlay(ctx, cell, t, walk) {
            curveGlow(ctx, walk.x, walk.y, 18, CURVE_FX.glow, 0.45);
        }
    },
    {
        group: "motion",
        name: "dash drift",
        note: "flowing dashes · soft cyan casing · target",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 7, alpha: 0.12
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 2.4, alpha: 0.92,
                dash: [11, 8], offset: -((t / 34) % 19)
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.2, {
                fill: CURVE_FX.cyan,
                ring: "rgba(255, 255, 255, 0.85)", ringWidth: 1
            }));
            curveDestTarget(ctx, t, CURVE_FX.cyan);
        }
    },
    {
        group: "motion",
        name: "twin windows",
        note: "two light bands · one on the walker, one ahead",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.violet, width: 1.5, alpha: 0.35,
                dash: [2, 7]
            });
            const bands = [
                [walk.dist - 64, walk.dist + 64],
                [walk.dist + 120, walk.dist + 248]
            ];
            for (let i = 0; i < bands.length; i++) {
                const pts = curveRange(bands[i][0], bands[i][1]);
                if (pts.length < 2) { continue; }
                curveStroke(ctx, pts, {
                    color: CURVE_FX.violet, width: 9, alpha: 0.16
                });
                curveStroke(ctx, pts, {
                    color: CURVE_FX.white, width: 2.4,
                    alpha: i === 0 ? 0.9 : 0.55
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.1, {
                fill: CURVE_FX.violet,
                ring: "rgba(255, 255, 255, 0.8)", ringWidth: 0.9
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "breathing glow",
        note: "additive tube inhales · white core · target",
        draw(ctx, cell, t) {
            const breathe = 0.5 + 0.5 * Math.sin(t / 280);
            ctx.save();
            ctx.globalCompositeOperation = "lighter";
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.teal, width: 10 + 5 * breathe,
                alpha: 0.1
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.teal, width: 5 + 2 * breathe,
                alpha: 0.16 + 0.1 * breathe
            });
            ctx.restore();
            curveStroke(ctx, CURVE_SMOOTH, {
                color: "#dcfff9", width: 1.5 + 0.6 * breathe,
                alpha: 0.95
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.2, {
                fill: CURVE_FX.teal,
                ring: "rgba(255, 255, 255, 0.85)", ringWidth: 0.9
            }));
            curveDestTarget(ctx, t, CURVE_FX.teal);
        }
    },
    {
        group: "motion",
        name: "chase spark",
        note: "gold spark runs the curve · white star head",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.dim, width: 1.2, alpha: 0.35
            });
            const chaser = (t / 6) % CURVE_TIMING.total;
            const from = Math.max(0, chaser - 58);
            let prev = curvePointAtDistance(from);
            for (let d = from + 5; d <= chaser; d += 5) {
                const p = curvePointAtDistance(d);
                const k = (p.dist - from) / 58;
                curveStroke(ctx, [prev, p], {
                    color: k > 0.8 ? CURVE_FX.white : CURVE_FX.gold,
                    width: 1 + 2.4 * k, alpha: 0.1 + 0.8 * k
                });
                prev = p;
            }
            const spark = curvePointAtDistance(chaser);
            curveGlow(ctx, spark.x, spark.y, 14, CURVE_FX.gold, 0.55);
            curveStar(ctx, spark.x, spark.y, 3.4, CURVE_FX.white, 0.95);
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.8, {
                fill: CURVE_FX.lime, alpha: 0.75
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "draw-on reveal",
        note: "line draws 90 px ahead · white hot tip",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 1.1, alpha: 0.18
            });
            const head = Math.min(CURVE_TIMING.total,
                walk.dist + 90);
            curveStroke(ctx, curveRange(0, head), {
                color: CURVE_FX.magenta, width: 2.5, alpha: 0.9
            });
            curveStroke(ctx, curveRange(Math.max(0, head - 46), head), {
                color: CURVE_FX.white, width: 2.4, alpha: 0.85
            });
            const tip = curvePointAtDistance(head);
            curveGlow(ctx, tip.x, tip.y, 13, CURVE_FX.glow, 0.6);
            curveDot(ctx, tip.x, tip.y, 2.1, {
                fill: CURVE_FX.white,
                ring: CURVE_FX.magenta, ringWidth: 1
            });
            curveStops(ctx, (p, i) => curveTick(ctx, p.x, p.y,
                CURVE_STOPS[i].dirX, CURVE_STOPS[i].dirY, 3, {
                    color: CURVE_FX.magenta, alpha: 0.8, width: 1.3
                }));
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "scan band",
        note: "white scan sweeps the curve · stops light up",
        draw(ctx, cell, t) {
            const band = (t / 9) % CURVE_TIMING.total;
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 1.6, alpha: 0.3,
                dash: [5, 7]
            });
            const pts = curveRange(band - 34, band + 34);
            curveStroke(ctx, pts, {
                color: CURVE_FX.cyan, width: 10, alpha: 0.18
            });
            curveStroke(ctx, pts, {
                color: CURVE_FX.white, width: 2.6, alpha: 0.92
            });
            curveStops(ctx, (p, i) => {
                const lit = Math.max(0, 1 - Math.abs(
                    CURVE_STOP_DIST[i] - band) / 80);
                curveDot(ctx, p.x, p.y, 2 + 1.4 * lit, {
                    fill: lit > 0.4 ? CURVE_FX.white : CURVE_FX.cyan,
                    alpha: 0.45 + 0.5 * lit,
                    ring: "rgba(23, 198, 222, 0.9)", ringWidth: 1
                });
            });
            curveDestTarget(ctx, t, CURVE_FX.cyan);
        }
    },
    {
        group: "motion",
        name: "pulse snake",
        note: "width wave travels the curve · violet core",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.violet, width: 1, alpha: 0.22
            });
            const pts = curveSamples(6, 0);
            for (let i = 1; i < pts.length; i++) {
                const wave = curveWave(pts[i].dist, t, 84, 620);
                curveStroke(ctx, [pts[i - 1], pts[i]], {
                    color: wave > 0.6 ? CURVE_FX.glow
                        : CURVE_FX.violet,
                    width: 0.8 + 2.8 * wave,
                    alpha: 0.18 + 0.72 * wave
                });
            }
            curveStops(ctx, (p) => {
                ctx.save();
                ctx.strokeStyle = CURVE_FX.violet;
                ctx.globalAlpha = 0.85;
                ctx.lineWidth = 1.5;
                ctx.beginPath();
                ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
                ctx.stroke();
                ctx.restore();
            });
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "pen nib",
        note: "ink trail behind · gold nib rides the walker",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.amber, width: 1.2, alpha: 0.25,
                dash: [3, 6]
            });
            curveStroke(ctx, curveRange(0, walk.dist), {
                color: CURVE_FX.amber, width: 2.6, alpha: 0.92
            });
            const size = 6.5 + 0.9 * Math.sin(t / 180);
            curveNib(ctx, walk.x, walk.y, walk.dirX, walk.dirY,
                size, CURVE_FX.gold);
            curveDot(ctx, walk.x, walk.y, 1.6, {
                fill: CURVE_FX.white, alpha: 0.95
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.8, {
                fill: CURVE_FX.amber, alpha: 0.75
            }));
            curveDestPin(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "bead wave",
        note: "beads every 14 px · radius wave · ring",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 1, alpha: 0.22
            });
            for (const s of curveSamples(14, 7)) {
                const wave = curveWave(s.dist, t, 90, 700);
                curveDot(ctx, s.x, s.y, 1.2 + 1.6 * wave, {
                    fill: CURVE_FX.magenta,
                    alpha: 0.3 + 0.65 * wave,
                    ring: wave > 0.75
                        ? "rgba(255, 255, 255, 0.8)" : null,
                    ringWidth: 0.8
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.8, {
                fill: CURVE_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "fireflies",
        note: "ten drifting lights orbit the curve · twinkle",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.lime, width: 1, alpha: 0.16,
                dash: [2, 8]
            });
            for (let i = 0; i < 10; i++) {
                const d = ((i / 10) * CURVE_TIMING.total
                    + t * (0.018 + i * 0.0012))
                    % CURVE_TIMING.total;
                const p = curvePointAtDistance(d);
                const side = Math.sin(t / 650 + i * 2.1)
                    * (5 + (i % 3) * 2);
                const x = p.x - p.dirY * side;
                const y = p.y + p.dirX * side;
                const tw = 0.55 + 0.45 * Math.sin(t / 380 + i * 1.3);
                curveGlow(ctx, x, y, 7, i % 2 ? CURVE_FX.lime
                    : CURVE_FX.gold, 0.35 * tw);
                curveDot(ctx, x, y, 1.3 + 0.8 * tw, {
                    fill: i % 2 ? "#eaffb0" : "#fff3c4",
                    alpha: 0.85
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.8, {
                fill: CURVE_FX.lime, alpha: 0.7
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "spark trail",
        note: "sparks scatter behind the walker · amber fade",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.amber, width: 1.2, alpha: 0.25
            });
            for (let i = 0; i < 14; i++) {
                const back = i * 9 + (t / 26) % 9;
                const d = walk.dist - back;
                if (d < 0) { break; }
                const p = curvePointAtDistance(d);
                const side = Math.sin(t / 140 + i * 1.9)
                    * (2 + back * 0.045);
                const fade = Math.max(0.05, 1 - back / 150);
                curveDot(ctx, p.x - p.dirY * side,
                    p.y + p.dirX * side, 0.9 + 1.1 * fade, {
                        fill: i < 2 ? CURVE_FX.white : CURVE_FX.amber,
                        alpha: 0.85 * fade
                    });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.8, {
                fill: CURVE_FX.amber, alpha: 0.6
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "stardust",
        note: "thirty twinkling stars scattered off the curve",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.violet, width: 1, alpha: 0.2
            });
            for (let i = 0; i < 30; i++) {
                const d = curveNoise(i + 1) * CURVE_TIMING.total;
                const p = curvePointAtDistance(d);
                const side = (curveNoise(i + 41) - 0.5) * 30;
                const x = p.x - p.dirY * side;
                const y = p.y + p.dirX * side;
                const tw = 0.5 + 0.5 * Math.sin(t / 300 + i * 1.7);
                if (i % 4 === 0) {
                    curveGlow(ctx, x, y, 7, CURVE_FX.violet,
                        0.3 * tw);
                }
                curveStar(ctx, x, y, 1 + 1.6 * tw,
                    CURVE_FX.white, 0.35 + 0.6 * tw);
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.6, {
                fill: CURVE_FX.violet, alpha: 0.55
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "glow footprints",
        note: "alternating steps fade behind · dotted ahead",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, curveRange(walk.dist,
                CURVE_TIMING.total), {
                color: CURVE_FX.violet, width: 1.4, alpha: 0.3,
                dash: [2, 7]
            });
            for (let i = 0; i < 15; i++) {
                const d = walk.dist - i * 7.5;
                if (d < 0) { break; }
                const p = curvePointAtDistance(d);
                const side = (i % 2 === 0 ? 1 : -1) * 3.4;
                const fade = Math.max(0.08, 1 - i / 16);
                const x = p.x - p.dirY * side;
                const y = p.y + p.dirX * side;
                if (i < 3) {
                    curveGlow(ctx, x, y, 7, CURVE_FX.violet,
                        0.3 * fade);
                }
                curveDot(ctx, x, y, 1.4 + 0.8 * fade, {
                    fill: CURVE_FX.violet, alpha: 0.8 * fade
                });
            }
            curveStops(ctx, (p, i) => curveTick(ctx, p.x, p.y,
                CURVE_STOPS[i].dirX, CURVE_STOPS[i].dirY, 2.6, {
                    color: CURVE_FX.violet, alpha: 0.75, width: 1.3
                }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "dotted march",
        note: "dots march along the curve · hollow stop rings",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 1, alpha: 0.25
            });
            for (const s of curveSamples(11, (t / 45) % 11)) {
                curveDot(ctx, s.x, s.y, 1.7, {
                    fill: CURVE_FX.glow, alpha: 0.9
                });
            }
            curveStops(ctx, (p) => {
                ctx.save();
                ctx.strokeStyle = CURVE_FX.magenta;
                ctx.globalAlpha = 0.9;
                ctx.lineWidth = 1.6;
                ctx.beginPath();
                ctx.arc(p.x, p.y, 3.6, 0, Math.PI * 2);
                ctx.stroke();
                ctx.restore();
            });
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "meteor shower",
        note: "four meteors race the curve · gold streaks",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.dim, width: 1.2, alpha: 0.3,
                dash: [3, 8]
            });
            const span = CURVE_TIMING.total;
            for (let i = 0; i < 4; i++) {
                const head = ((t / 7) + i * (span / 4)) % span;
                const from = Math.max(0, head - 64);
                if (head - from < 5) { continue; }
                curveStroke(ctx, curveRange(from, head), {
                    color: i % 2 === 0 ? CURVE_FX.gold
                        : CURVE_FX.white,
                    width: 2.4, alpha: 0.75
                });
                const tip = curvePointAtDistance(head);
                curveGlow(ctx, tip.x, tip.y, 10, CURVE_FX.gold, 0.5);
                curveDot(ctx, tip.x, tip.y, 1.7, {
                    fill: CURVE_FX.white, alpha: 0.9
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.8, {
                fill: CURVE_FX.gold, alpha: 0.65
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "ripple rings",
        note: "three rings trail the walker · cyan target",
        draw(ctx, cell, t, walk) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 1.3, alpha: 0.3
            });
            for (let k = 0; k < 3; k++) {
                const age = ((t + k * 520) % 1560) / 1560;
                const d = Math.max(0, walk.dist - age * 70);
                const p = curvePointAtDistance(d);
                curvePulse(ctx, p.x, p.y, age, {
                    from: 3, to: 15, color: CURVE_FX.cyan,
                    alpha: 0.55, width: 1.3
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 1.9, {
                fill: CURVE_FX.cyan, alpha: 0.7
            }));
            curveDestTarget(ctx, t, CURVE_FX.cyan);
        }
    },
    {
        group: "motion",
        name: "heartbeat",
        note: "double pulse swells the whole curve · rose",
        draw(ctx, cell, t) {
            const beat = (t % 1100) / 1100;
            const first = Math.exp(
                -Math.pow((beat - 0.1) / 0.055, 2));
            const second = 0.6 * Math.exp(
                -Math.pow((beat - 0.3) / 0.07, 2));
            const pulse = Math.min(1, first + second);
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.rose, width: 8 + 9 * pulse,
                alpha: 0.1 + 0.22 * pulse
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.rose, width: 2.2 + 2.4 * pulse,
                alpha: 0.7 + 0.3 * pulse
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.white, width: 0.6 + 1.4 * pulse,
                alpha: 0.5 + 0.5 * pulse
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.1, {
                fill: CURVE_FX.rose,
                ring: "rgba(255, 255, 255, 0.85)", ringWidth: 0.9
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "wave sweep",
        note: "bright wave front sweeps start to destination",
        draw(ctx, cell, t) {
            const span = CURVE_TIMING.total;
            const wave = ((t / 1500) % 1) * (span + 180) - 90;
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.amber, width: 1.6, alpha: 0.3
            });
            curveStroke(ctx, curveRange(wave - 80, wave), {
                color: CURVE_FX.amber, width: 10, alpha: 0.16
            });
            curveStroke(ctx, curveRange(wave - 80, wave), {
                color: CURVE_FX.white, width: 2.6, alpha: 0.9
            });
            curveStops(ctx, (p, i) => {
                const lit = CURVE_STOP_DIST[i] < wave ? 0.95 : 0.4;
                curveDot(ctx, p.x, p.y, 1.9 + 0.7 * lit, {
                    fill: lit > 0.6 ? CURVE_FX.white
                        : CURVE_FX.amber,
                    alpha: lit
                });
            });
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "motion",
        name: "stop ping",
        note: "rings fire down the stops in sequence · dots",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.lime, width: 1.6, alpha: 0.5
            });
            curveStops(ctx, (p, i) => {
                let phase = (t / 1400 - i * 0.16) % 1;
                if (phase < 0) { phase += 1; }
                curvePulse(ctx, p.x, p.y, phase, {
                    from: 2.5, to: 11, color: CURVE_FX.lime,
                    alpha: 0.75, width: 1.3
                });
                curveDot(ctx, p.x, p.y, 2, {
                    fill: CURVE_FX.lime,
                    ring: "rgba(255, 255, 255, 0.6)", ringWidth: 0.8
                });
            });
            curveDestTarget(ctx, t, CURVE_FX.lime);
        }
    },
    {
        group: "color",
        name: "hue ramp",
        note: "magenta to cyan along the curve · slow shimmer",
        draw(ctx, cell, t) {
            const total = CURVE_TIMING.total;
            const steps = 30;
            const shade = 14 * Math.sin(t / 700);
            for (let i = 1; i <= steps; i++) {
                const hue = 302 - 118 * (i / steps) + shade;
                curveStroke(ctx, curveRange(
                    total * (i - 1) / steps, total * i / steps), {
                        color: "hsl(" + hue.toFixed(1)
                            + ", 92%, 62%)",
                        width: 2.9, alpha: 0.92
                    });
            }
            curveStops(ctx, (p, i) => {
                const hue = 302 - 118 * (i /
                    (CURVE_ROUTE.length - 2));
                curveDot(ctx, p.x, p.y, 2.3, {
                    fill: "hsl(" + hue.toFixed(1) + ", 92%, 62%)",
                    ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
                });
            });
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "color",
        name: "temperature split",
        note: "warm walked · cool ahead · white seam glow",
        draw(ctx, cell, t, walk) {
            const total = CURVE_TIMING.total;
            curveStroke(ctx, curveRange(0, walk.dist), {
                color: CURVE_FX.amber, width: 7, alpha: 0.15
            });
            curveStroke(ctx, curveRange(0, walk.dist), {
                color: CURVE_FX.amber, width: 2.6, alpha: 0.95
            });
            curveStroke(ctx, curveRange(walk.dist, total), {
                color: CURVE_FX.cyan, width: 7, alpha: 0.13
            });
            curveStroke(ctx, curveRange(walk.dist, total), {
                color: CURVE_FX.cyan, width: 2.6, alpha: 0.9
            });
            curveGlow(ctx, walk.x, walk.y, 15, CURVE_FX.white, 0.35);
            curveStops(ctx, (p, i) => {
                const done = CURVE_STOP_DIST[i] <= walk.dist;
                curveTick(ctx, p.x, p.y, CURVE_STOPS[i].dirX,
                    CURVE_STOPS[i].dirY, 3.2, {
                        color: done ? CURVE_FX.amber
                            : CURVE_FX.cyan,
                        alpha: 0.9, width: 1.5
                    });
            });
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "color",
        name: "rainbow flow",
        note: "hue cycles along the curve and over time",
        draw(ctx, cell, t) {
            const total = CURVE_TIMING.total;
            const steps = 32;
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 7, alpha: 0.12
            });
            for (let i = 1; i <= steps; i++) {
                const hue = curveRainbowHue(i / steps, t);
                curveStroke(ctx, curveRange(
                    total * (i - 1) / steps, total * i / steps), {
                        color: "hsl(" + hue.toFixed(1)
                            + ", 95%, 60%)",
                        width: 3, alpha: 0.9
                    });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2, {
                fill: CURVE_FX.white, alpha: 0.85
            }));
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "color",
        name: "split duotone",
        note: "amber and violet split at a moving seam",
        draw(ctx, cell, t, walk) {
            const total = CURVE_TIMING.total;
            const split = (walk.dist + 90) % total;
            curveStroke(ctx, curveRange(0, split), {
                color: CURVE_FX.amber, width: 2.8, alpha: 0.95
            });
            curveStroke(ctx, curveRange(split, total), {
                color: CURVE_FX.violet, width: 2.8, alpha: 0.95
            });
            curveStroke(ctx, curveRange(split - 12, split + 12), {
                color: CURVE_FX.white, width: 3.2, alpha: 0.85
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.1, {
                fill: CURVE_FX.white,
                ring: "rgba(27, 36, 48, 0.7)", ringWidth: 1
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "stroke",
        name: "tapered ribbon",
        note: "width tapers along the curve · rose with casing",
        draw(ctx, cell, t) {
            const total = CURVE_TIMING.total;
            const steps = 36;
            for (let i = 1; i <= steps; i++) {
                const a = total * (i - 1) / steps;
                const b = total * i / steps;
                const k = (i - 0.5) / steps;
                const taper = Math.sin(Math.PI * k);
                const wob = 0.4 * Math.sin(t / 380 + i * 0.7);
                const width = 0.6 + 3.8 * taper + wob;
                const pts = curveRange(a, b);
                curveStroke(ctx, pts, {
                    color: CURVE_FX.ink, width: width + 2,
                    alpha: 0.25
                });
                curveStroke(ctx, pts, {
                    color: CURVE_FX.rose, width: width, alpha: 0.92
                });
            }
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.1, {
                fill: CURVE_FX.rose,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 0.9
            }));
            curveDestPin(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "stroke",
        name: "double strand",
        note: "two offset strands · dashes run opposite ways",
        draw(ctx, cell, t) {
            curveStroke(ctx, CURVE_STRANDS.left, {
                color: CURVE_FX.magenta, width: 1.7, alpha: 0.9,
                dash: [10, 8], offset: -((t / 34) % 18)
            });
            curveStroke(ctx, CURVE_STRANDS.right, {
                color: CURVE_FX.cyan, width: 1.7, alpha: 0.9,
                dash: [10, 8], offset: (t / 34) % 18
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.white, width: 0.7, alpha: 0.25
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2, {
                fill: CURVE_FX.white,
                ring: "rgba(27, 36, 48, 0.65)", ringWidth: 1
            }));
            curveDestTarget(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "stroke",
        name: "emboss",
        note: "dark shadow · white bevel · magenta face",
        draw(ctx, cell, t) {
            const shimmer = 0.5 + 0.5 * Math.sin(t / 420);
            ctx.save();
            ctx.translate(1.7, 2.2);
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.ink, width: 4.8, alpha: 0.5
            });
            ctx.restore();
            ctx.save();
            ctx.translate(-1.2 - 0.5 * shimmer,
                -1.4 - 0.5 * shimmer);
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.white, width: 1.7, alpha: 0.55
            });
            ctx.restore();
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.magenta, width: 2.6, alpha: 0.95
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2.2, {
                fill: CURVE_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            curveDestPin(ctx, t, CURVE_FX.dest);
        }
    },
    {
        group: "stroke",
        name: "neon tube",
        note: "additive cyan tube · white core · energy pulse",
        draw(ctx, cell, t) {
            const pulse = (t / 9) % CURVE_TIMING.total;
            ctx.save();
            ctx.globalCompositeOperation = "lighter";
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 12, alpha: 0.08
            });
            curveStroke(ctx, CURVE_SMOOTH, {
                color: CURVE_FX.cyan, width: 6, alpha: 0.18
            });
            ctx.restore();
            curveStroke(ctx, CURVE_SMOOTH, {
                color: "#e8feff", width: 1.4, alpha: 0.95
            });
            const p = curvePointAtDistance(pulse);
            curveGlow(ctx, p.x, p.y, 20, CURVE_FX.cyan, 0.42);
            curveDot(ctx, p.x, p.y, 2.1, {
                fill: CURVE_FX.white,
                ring: CURVE_FX.cyan, ringWidth: 1
            });
            curveStops(ctx, (p2) => curveDot(ctx, p2.x, p2.y, 2.1, {
                fill: "#eaffff",
                ring: CURVE_FX.cyan, ringWidth: 1.2
            }));
            curveDestTarget(ctx, t, CURVE_FX.cyan);
        }
    },
    {
        group: "stroke",
        name: "elastic band",
        note: "the curve wobbles along its normal · violet",
        draw(ctx, cell, t) {
            const amount = 3.2 + 0.7 * Math.sin(t / 600);
            const pts = curveWobble(amount, t / 260);
            curveStroke(ctx, pts, {
                color: CURVE_FX.violet, width: 8, alpha: 0.14
            });
            curveStroke(ctx, pts, {
                color: CURVE_FX.violet, width: 2.6, alpha: 0.92
            });
            curveStroke(ctx, pts, {
                color: CURVE_FX.white, width: 0.9, alpha: 0.5,
                dash: [2, 9], offset: -((t / 40) % 11)
            });
            curveStops(ctx, (p) => curveDot(ctx, p.x, p.y, 2, {
                fill: CURVE_FX.violet,
                ring: "rgba(255, 255, 255, 0.85)", ringWidth: 0.9
            }));
            curveDestRing(ctx, t, CURVE_FX.dest);
        }
    }
];

// PathCurve is the engine: it builds the card grid, owns the virtual
// clock (pause and speed multiply it) and renders every visible card
// from the shared loop time.
const PathCurve = {
    grid: null,
    scroll: null,
    cells: [],
    tile: null,
    clock: 0,
    speed: 1,
    paused: false,
    running: false,
    lastTs: 0,
    measured: false,

    // init boots the lab: the grid DOM, the map tile and the render
    // loop. Called by main.js when /api/config answers the
    // "test-path-v2" mode.
    init() {
        this.grid = document.getElementById("curve-grid");
        this.scroll = document.getElementById("curve-scroll");
        const pause = document.getElementById("curve-pause");
        if (pause) {
            pause.addEventListener("click", () => {
                this.paused = !this.paused;
                pause.textContent = this.paused ? "resume" : "pause";
            });
        }
        const speed = document.getElementById("curve-speed");
        if (speed) {
            speed.addEventListener("change", () => {
                this.speed = parseFloat(speed.value) || 1;
            });
        }
        if (typeof window !== "undefined" && window.addEventListener) {
            window.addEventListener("resize", () => {
                this.measured = false;
            });
        }
        this.buildGrid();
        this.updatePicked();
        this.loadTile();
        this.startLoop();
    },

    // buildGrid assembles the card grid: one numbered head and one
    // canvas per variant, the grid template fixed to the cell size.
    buildGrid() {
        this.grid.style.gridTemplateColumns = "repeat("
            + CURVE_COLUMNS + ", " + CURVE_CELL_W + "px)";
        const dpr = (typeof window !== "undefined"
            && window.devicePixelRatio) || 1;
        this.cells = [];
        for (let i = 0; i < CURVE_VARIANTS.length; i++) {
            const variant = CURVE_VARIANTS[i];
            const card = document.createElement("div");
            card.className = "path-card";
            const head = document.createElement("div");
            head.className = "path-cardhead";
            const num = document.createElement("span");
            num.className = "pc-num";
            num.textContent = String(i + 1).padStart(2, "0");
            const name = document.createElement("span");
            name.className = "pc-name";
            name.textContent = variant.name;
            const note = document.createElement("span");
            note.className = "pc-note";
            note.textContent = variant.note;
            head.appendChild(num);
            head.appendChild(name);
            head.appendChild(note);
            head.addEventListener("click", () => this.togglePick(i));
            card.appendChild(head);
            const canvas = document.createElement("canvas");
            canvas.className = "path-cell";
            canvas.width = Math.round(CURVE_CELL_W * dpr);
            canvas.height = Math.round(CURVE_CELL_H * dpr);
            canvas.style.width = CURVE_CELL_W + "px";
            canvas.style.height = CURVE_CELL_H + "px";
            card.appendChild(canvas);
            this.grid.appendChild(card);
            this.cells.push({
                index: i,
                card: card,
                head: head,
                canvas: canvas,
                ctx: canvas.getContext("2d"),
                picked: false,
                top: undefined,
                left: undefined,
                width: CURVE_CELL_W,
                height: CURVE_CELL_H
            });
        }
    },

    // togglePick marks or clears one variant and refreshes the picked
    // list of the toolbar, so a combination is easy to name.
    togglePick(index) {
        const cell = this.cells[index];
        if (!cell) { return; }
        cell.picked = !cell.picked;
        cell.card.classList.toggle("picked", cell.picked);
        cell.head.classList.toggle("picked", cell.picked);
        this.updatePicked();
    },

    // updatePicked writes the picked numbers into the toolbar chip.
    updatePicked() {
        const picked = [];
        for (const cell of this.cells) {
            if (cell.picked) {
                picked.push(String(cell.index + 1).padStart(2, "0"));
            }
        }
        const label = document.getElementById("curve-picked");
        if (label) {
            label.textContent = "picked: "
                + (picked.length ? picked.join(", ") : "none");
        }
    },

    // loadTile fetches the world map tile the cells use as their
    // background. The engine falls back to a plain terrain fill while
    // the image is in flight (and forever if it is unavailable).
    loadTile() {
        if (typeof Image === "undefined") { return; }
        const image = new Image();
        image.onload = () => { this.tile = image; };
        image.onerror = () => { this.tile = null; };
        image.src = CURVE_TILE_URL;
    },

    // startLoop runs the requestAnimationFrame loop: the virtual
    // clock advances by dt * speed unless paused.
    startLoop() {
        if (typeof requestAnimationFrame !== "function") { return; }
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

    // measure caches the card offsets of the scroll content once, so
    // the frame skips the cards outside the viewport the way the walk
    // plan gallery skips the off screen cards. The harness has no
    // layout: the missing offsets keep every card rendered.
    measure() {
        if (this.measured || !this.grid) { return; }
        if (typeof this.grid.offsetWidth !== "number"
            || this.grid.offsetWidth <= 0) {
            return;
        }
        for (const cell of this.cells) {
            if (typeof cell.card.offsetTop !== "number") { return; }
            cell.top = cell.card.offsetTop;
            cell.left = cell.card.offsetLeft;
            cell.width = cell.card.offsetWidth || CURVE_CELL_W;
            cell.height = cell.card.offsetHeight || CURVE_CELL_H;
        }
        this.measured = true;
    },

    // render repaints the visible cards from the shared loop time.
    render() {
        this.measure();
        const loop = CURVE_TIMING.loopMs;
        const t = ((this.clock % loop) + loop) % loop;
        const walk = curveWalkAt(t);
        const scrollTop = this.scroll ? (this.scroll.scrollTop || 0) : 0;
        const scrollLeft = this.scroll
            ? (this.scroll.scrollLeft || 0) : 0;
        const viewH = this.scroll && this.scroll.clientHeight
            ? this.scroll.clientHeight : 100000;
        const viewW = this.scroll && this.scroll.clientWidth
            ? this.scroll.clientWidth : 100000;
        for (const cell of this.cells) {
            if (typeof cell.top === "number") {
                if (cell.top + cell.height < scrollTop - 40
                    || cell.top > scrollTop + viewH + 40
                    || cell.left + cell.width < scrollLeft - 40
                    || cell.left > scrollLeft + viewW + 40) {
                    continue;
                }
            }
            this.renderCell(cell, t, walk);
        }
    },

    // renderCell paints one card: the map background, the variant
    // layers around the shared walker, and the arrival burst.
    renderCell(cell, t, walk) {
        const ctx = cell.ctx;
        if (!ctx) { return; }
        const dpr = (typeof window !== "undefined"
            && window.devicePixelRatio) || 1;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, CURVE_CELL_W, CURVE_CELL_H);
        this.drawMapBackground(ctx);
        const variant = CURVE_VARIANTS[cell.index];
        if (variant.underlay) {
            variant.underlay(ctx, cell, t, walk);
        }
        if (variant.draw) { variant.draw(ctx, cell, t, walk); }
        this.drawWalker(ctx, walk);
        if (variant.overlay) {
            variant.overlay(ctx, cell, t, walk);
        }
    },

    // drawMapBackground paints the world map tile crop of the starter
    // meadow, stretched to the card. The fallback is a flat terrain
    // fill with a soft grid so the cards still read as map views when
    // the tile is unavailable.
    drawMapBackground(ctx) {
        if (this.tile && this.tile.complete
            && this.tile.naturalWidth > 0) {
            const scale = CURVE_TILE_PX / CURVE_TILE_UNITS;
            const cx = (CURVE_BG_CENTER.x - CURVE_TILE_ORIGIN.x) * scale;
            const cy = (CURVE_BG_CENTER.y - CURVE_TILE_ORIGIN.y) * scale;
            const sw = CURVE_BG_WORLD_W * scale;
            const sh = sw * CURVE_CELL_H / CURVE_CELL_W;
            ctx.drawImage(this.tile, cx - sw / 2, cy - sh / 2,
                sw, sh, 0, 0, CURVE_CELL_W, CURVE_CELL_H);
        } else {
            ctx.fillStyle = "#e8e2d2";
            ctx.fillRect(0, 0, CURVE_CELL_W, CURVE_CELL_H);
            ctx.strokeStyle = "rgba(21, 34, 50, 0.08)";
            ctx.lineWidth = 1;
            for (let x = 30; x < CURVE_CELL_W; x += 30) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, CURVE_CELL_H);
                ctx.stroke();
            }
            for (let y = 30; y < CURVE_CELL_H; y += 30) {
                ctx.beginPath();
                ctx.moveTo(0, y);
                ctx.lineTo(CURVE_CELL_W, y);
                ctx.stroke();
            }
        }
        const edge = ctx.createLinearGradient(0, 0, 0, CURVE_CELL_H);
        edge.addColorStop(0, "rgba(10, 14, 20, 0.16)");
        edge.addColorStop(0.5, "rgba(10, 14, 20, 0)");
        edge.addColorStop(1, "rgba(10, 14, 20, 0.12)");
        ctx.fillStyle = edge;
        ctx.fillRect(0, 0, CURVE_CELL_W, CURVE_CELL_H);
    },

    // drawWalker paints the shared character marker: the blue self dot
    // of the map with the look direction tick. The marker fades in the
    // reset tail of the loop and pings when the walk arrives.
    drawWalker(ctx, walk) {
        const fade = walk.resetting
            ? Math.max(0, 1 - (walk.sinceArrive - CURVE_HOLD_MS)
                / CURVE_RESET_MS)
            : 1;
        if (walk.sinceArrive >= 0 && walk.sinceArrive < 700) {
            curvePulse(ctx, walk.x, walk.y, walk.sinceArrive / 700, {
                from: 4, to: 16, color: CURVE_FX.dest,
                alpha: 0.8, width: 2
            });
        }
        ctx.save();
        ctx.globalAlpha = 0.2 * fade;
        ctx.fillStyle = CURVE_FX.ink;
        ctx.beginPath();
        ctx.arc(walk.x + 1, walk.y + 2.5, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 0.22 * fade;
        ctx.fillStyle = CURVE_FX.self;
        ctx.beginPath();
        ctx.arc(walk.x, walk.y, 9, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = fade;
        ctx.fillStyle = CURVE_FX.self;
        ctx.beginPath();
        ctx.arc(walk.x, walk.y, 4.4, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = "rgba(255, 255, 255, 0.95)";
        ctx.lineWidth = 1.4;
        ctx.stroke();
        ctx.strokeStyle = "rgba(15, 18, 22, 0.75)";
        ctx.lineWidth = 1.6;
        ctx.beginPath();
        ctx.moveTo(walk.x + walk.dirX * 5.2, walk.y + walk.dirY * 5.2);
        ctx.lineTo(walk.x + walk.dirX * 10, walk.y + walk.dirY * 10);
        ctx.stroke();
        ctx.restore();
    }
};
