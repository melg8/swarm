/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// PathFlow is the flow and snake speed lab of the -test-path-ui-v3
// mode (webserver.NewTestPathFlowServer answers mode "test-path-v3").
// It is the follow-up of the smooth curve lab: the owner picked the
// rainbow flow and the pulse snake of that page and asked for forty
// more variants (twenty each) in different color schemes and at
// different movement speeds. The rainbow travels from the planning
// origin toward the destination, never the opposite way.
//
// The layout and the clock match the first two labs one to one: a four
// column grid of cards, the same seven point route with five stops,
// the same travel, dwell, hold and fade budget, and the same walker.
// Every effect is a pure function of the loop clock, so a repaint of
// the same moment is identical and the Node harness can assert that
// every variant moves.
//
// Cards 01-20 are the rainbow flow: five palettes x four hue cycle
// speeds. Cards 21-40 are the pulse snake: five crest schemes x four
// wave periods. In both families the moving crest travels from the
// origin to the destination.
const FLOW_CELL_W = 380;
const FLOW_CELL_H = 250;

// The grid columns; four cards fit a 1600 px viewport, the strip
// scrolls both ways otherwise.
const FLOW_COLUMNS = 4;

// The map background: the open meadow crop of the 21_19 world tile,
// the terrain the bot actually hunts.
const FLOW_TILE_URL = "maps/0/21_19.jpg";
const FLOW_TILE_PX = 1024;
const FLOW_TILE_UNITS = 32768;
const FLOW_TILE_ORIGIN = { x: 32768, y: 32768 };
const FLOW_BG_CENTER = { x: 43328, y: 43648 };
const FLOW_BG_WORLD_W = 1900;

// The shared planned walk: the planning origin, five stops (the
// follower cursor targets) and the destination, the exact route of
// the first two labs.
const FLOW_ROUTE = [
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
const FLOW_TRAVEL_MS = 4200;
const FLOW_DWELL_MS = 240;
const FLOW_HOLD_MS = 900;
const FLOW_RESET_MS = 300;

// The palette: the live magenta of the walk plan plus the accent
// colors the cards test against the light meadow imagery.
const FLOW_FX = {
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

// FLOW_SMOOTH is the Catmull-Rom resampling of the route (16 samples
// per segment) with the arc distance of every sample. Every stroke,
// the walker and the timeline read this single curve.
const FLOW_SMOOTH = (() => {
    const route = FLOW_ROUTE;
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

// FLOW_STOP_DIST is the arc length of every route control point: the
// resampling hits each control point exactly at sample 16 * index.
const FLOW_STOP_DIST = FLOW_ROUTE.map((p, i) => (
    i < FLOW_ROUTE.length - 1
        ? FLOW_SMOOTH[i * 16].dist
        : FLOW_SMOOTH[FLOW_SMOOTH.length - 1].dist
));

// FLOW_STOPS is the smooth curve point of every stop with its outgoing
// direction, the anchor of the stop markers.
const FLOW_STOPS = FLOW_ROUTE.map((p, i) => {
    const a = FLOW_ROUTE[i];
    const b = FLOW_ROUTE[Math.min(FLOW_ROUTE.length - 1, i + 1)];
    const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
    return {
        x: p.x,
        y: p.y,
        dist: FLOW_STOP_DIST[i],
        dirX: (b.x - a.x) / len,
        dirY: (b.y - a.y) / len
    };
});

// FLOW_TIMING is the arc length table of the smooth curve: the per
// stop distances, the arrival time of every stop (dwells included) and
// the loop length. Computed once at load, every variant and the walker
// read it.
const FLOW_TIMING = (() => {
    const stops = FLOW_STOP_DIST;
    const lens = [];
    let total = 0;
    for (let i = 1; i < stops.length; i++) {
        const len = stops[i] - stops[i - 1];
        lens.push(len);
        total += len;
    }
    const perMs = FLOW_TRAVEL_MS / total;
    const prefix = [0];
    const arrivals = [0];
    let t = 0;
    for (let i = 1; i < stops.length; i++) {
        prefix.push(prefix[i - 1] + lens[i - 1]);
        t += lens[i - 1] * perMs;
        arrivals.push(t);
        if (i < stops.length - 1) { t += FLOW_DWELL_MS; }
    }
    const arriveAt = arrivals[arrivals.length - 1];
    return {
        lens: lens,
        total: total,
        prefix: prefix,
        arrivals: arrivals,
        arriveAt: arriveAt,
        loopMs: arriveAt + FLOW_HOLD_MS + FLOW_RESET_MS
    };
})();

// flowRgba expands a #rrggbb color into an rgba() string.
function flowRgba(hex, alpha) {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return "rgba(" + r + ", " + g + ", " + b + ", " + alpha + ")";
}

// flowPointAtDistance returns the point, the unit direction and the
// arc length of one position on the smooth curve.
function flowPointAtDistance(distance) {
    const samples = FLOW_SMOOTH;
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

// flowRange returns the smooth curve between two arc lengths as a
// polyline the stroke helpers can paint.
function flowRange(from, to) {
    const samples = FLOW_SMOOTH;
    const total = samples[samples.length - 1].dist;
    const a = Math.max(0, Math.min(total, from));
    const b = Math.max(0, Math.min(total, to));
    if (b - a < 0.5) { return []; }
    const head = flowPointAtDistance(a);
    const out = [{ x: head.x, y: head.y }];
    for (const sample of samples) {
        if (sample.dist <= a + 1e-6 || sample.dist >= b - 1e-6) {
            continue;
        }
        out.push({ x: sample.x, y: sample.y });
    }
    const tail = flowPointAtDistance(b);
    out.push({ x: tail.x, y: tail.y });
    return out;
}

// flowSamples returns the curve points every spacing pixels starting
// at the phase offset, the direction of each included.
function flowSamples(spacing, phase) {
    const out = [];
    let d = phase === undefined ? 0 : phase;
    while (d <= FLOW_TIMING.total) {
        out.push(flowPointAtDistance(d));
        d += spacing;
    }
    return out;
}

// flowStroke strokes one polyline with the lab style.
function flowStroke(ctx, points, style) {
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

// flowDot draws one filled marker with an optional rim.
function flowDot(ctx, x, y, radius, style) {
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

// flowPulse draws one expanding ring of the 0..1 phase.
function flowPulse(ctx, x, y, phase, style) {
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

// flowGlow paints the radial halo of a bright crest.
function flowGlow(ctx, x, y, radius, hex, alpha) {
    if (typeof ctx.createRadialGradient !== "function") { return; }
    const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius);
    gradient.addColorStop(0, flowRgba(hex, alpha));
    gradient.addColorStop(1, flowRgba(hex, 0));
    ctx.save();
    ctx.fillStyle = gradient;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// flowStops walks the intermediate stops of the route (the planning
// origin and the destination excluded).
function flowStops(ctx, draw) {
    for (let i = 1; i < FLOW_STOPS.length - 1; i++) {
        draw(FLOW_STOPS[i], i);
    }
}

// flowDestRing is the shared breathing ring: a small core plus one
// expanding ring, the calmest destination marker of the lab.
function flowDestRing(ctx, t, color) {
    const dest = FLOW_ROUTE[FLOW_ROUTE.length - 1];
    flowPulse(ctx, dest.x, dest.y, (t % 1200) / 1200, {
        from: 3.5, to: 11, color: color, alpha: 0.7, width: 1.4
    });
    flowDot(ctx, dest.x, dest.y, 2.8, {
        fill: color, ring: "rgba(255, 255, 255, 0.95)", ringWidth: 1.1
    });
}

// flowWalkAt maps the loop clock to the walker state: the curve point,
// the direction, the walked arc length, the number of passed stops and
// the arrival phase. The walker dwells at every intermediate stop and
// holds at the destination before the loop fades and restarts.
function flowWalkAt(clockMs) {
    const timing = FLOW_TIMING;
    const last = FLOW_ROUTE.length - 1;
    const loop = timing.loopMs;
    const t = ((clockMs % loop) + loop) % loop;
    if (t >= timing.arriveAt) {
        const dest = FLOW_ROUTE[last];
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
            resetting: t >= timing.arriveAt + FLOW_HOLD_MS
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
        (seg >= 1 ? FLOW_DWELL_MS : 0);
    let dist = timing.prefix[seg];
    if (t >= dwellEnd) {
        const span = timing.arrivals[seg + 1] - dwellEnd;
        const k = span > 0 ? (t - dwellEnd) / span : 1;
        dist += timing.lens[seg] * Math.max(0, Math.min(1, k));
    }
    const p = flowPointAtDistance(dist);
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

// flowFrac wraps a number into the 0..1 cycle, negative included.
function flowFrac(value) {
    return value - Math.floor(value);
}

// flowPhase is the fractional position of the hue bands at the curve
// fraction k: the clock subtracts, so a fixed band moves to a larger k
// (toward the destination) as time runs.
function flowPhase(k, t, bands, periodMs) {
    return flowFrac(k * bands - t / periodMs);
}

// flowPaletteHue maps the band phase to the hue of one palette.
function flowPaletteHue(palette, u) {
    return palette.from + u * palette.span;
}

// flowColorAt is the css color of the flow band at the curve fraction
// k and the loop time t: the one direction formula every flow card
// and the harness read.
function flowColorAt(k, t, palette, periodMs) {
    const hue = flowPaletteHue(palette,
        flowPhase(k, t, palette.bands, periodMs));
    return "hsl(" + hue.toFixed(1) + ", " + palette.sat + "%, "
        + palette.light + "%)";
}

// flowWave is the 0..1 travelling width wave of the snake cards: the
// position along the curve sets the phase, the clock moves the crest
// toward the destination.
function flowWave(dist, t, wavelength, periodMs) {
    return 0.5 + 0.5 * Math.sin((dist / wavelength - t / periodMs)
        * Math.PI * 2);
}

// FLOW_PALETTES are the five color schemes of the flow cards: a hue
// range (the span may wrap past 360), the saturation and lightness of
// the band, the band count across the route and the faint casing.
const FLOW_PALETTES = [
    {
        key: "classic",
        label: "classic",
        note: "full spectrum · hue bands run start to goal",
        from: 0, span: 360, sat: 96, light: 60,
        bands: 2, base: "#ff44cc"
    },
    {
        key: "neon",
        label: "neon",
        note: "magenta → lime → cyan",
        from: 300, span: 240, sat: 100, light: 62,
        bands: 2, base: "#a78bfa"
    },
    {
        key: "pastel",
        label: "pastel",
        note: "soft spectrum · low saturation",
        from: 190, span: 260, sat: 58, light: 78,
        bands: 2, base: "#7c8aa0"
    },
    {
        key: "sunset",
        label: "sunset",
        note: "red → orange → gold",
        from: 350, span: 120, sat: 94, light: 58,
        bands: 2, base: "#ff7a1a"
    },
    {
        key: "aurora",
        label: "aurora",
        note: "green → cyan → violet",
        from: 120, span: 140, sat: 88, light: 62,
        bands: 2, base: "#2dd4bf"
    }
];

// FLOW_SPEEDS are the four hue cycle speeds of the flow cards: one
// full band cycle at a fixed curve point takes the period, so a stripe
// crosses the whole route in half of it.
const FLOW_SPEEDS = [
    { label: "16s", period: 16000 },
    { label: "9s", period: 9000 },
    { label: "5.2s", period: 5200 },
    { label: "2.8s", period: 2800 }
];

// FLOW_SCHEMES are the five crest schemes of the snake cards: the body
// color, the bright crest and the halo of the pulse.
const FLOW_SCHEMES = [
    {
        key: "magenta", label: "magenta",
        note: "white crest · magenta body",
        body: "#ff44cc", crest: "#ff8ae2", glow: "#ff44cc"
    },
    {
        key: "cyan", label: "cyan",
        note: "ice crest · cyan body",
        body: "#17c6de", crest: "#e8feff", glow: "#17c6de"
    },
    {
        key: "lime", label: "lime",
        note: "pale crest · lime body",
        body: "#7fc81a", crest: "#eaffb0", glow: "#7fc81a"
    },
    {
        key: "amber", label: "amber",
        note: "gold crest · amber body",
        body: "#ffb020", crest: "#fff3c4", glow: "#ffb020"
    },
    {
        key: "violet", label: "violet",
        note: "lilac crest · violet body",
        body: "#a78bfa", crest: "#e9dcff", glow: "#a78bfa"
    }
];

// FLOW_WAVES are the four crest periods of the snake cards: the wave
// travels one wavelength per period, so the period is the speed dial.
const FLOW_WAVES = [
    { label: "1.6s", period: 1600 },
    { label: "0.95s", period: 950 },
    { label: "0.56s", period: 560 },
    { label: "0.32s", period: 320 }
];

// flowDrawFlow paints one rainbow flow card: the curve split into
// short segments, every segment colored by the moving hue band.
function flowDrawFlow(ctx, palette, periodMs, t) {
    const total = FLOW_TIMING.total;
    const steps = 36;
    flowStroke(ctx, FLOW_SMOOTH, {
        color: palette.base, width: 7, alpha: 0.12
    });
    for (let i = 1; i <= steps; i++) {
        const k = (i - 0.5) / steps;
        flowStroke(ctx, flowRange(total * (i - 1) / steps,
            total * i / steps), {
            color: flowColorAt(k, t, palette, periodMs),
            width: 3.2, alpha: 0.92
        });
    }
    flowStops(ctx, (p) => flowDot(ctx, p.x, p.y, 2, {
        fill: FLOW_FX.white, alpha: 0.85
    }));
    flowDestRing(ctx, t, FLOW_FX.dest);
}

// flowDrawSnake paints one pulse snake card: the width and the color
// crest follow the travelling wave toward the destination.
function flowDrawSnake(ctx, scheme, periodMs, t) {
    const pts = flowSamples(6, 0);
    flowStroke(ctx, FLOW_SMOOTH, {
        color: scheme.body, width: 1, alpha: 0.22
    });
    for (let i = 1; i < pts.length; i++) {
        const wave = flowWave(pts[i].dist, t, 84, periodMs);
        flowStroke(ctx, [pts[i - 1], pts[i]], {
            color: wave > 0.62 ? scheme.crest : scheme.body,
            width: 0.8 + 3 * wave,
            alpha: 0.18 + 0.75 * wave
        });
        if (wave > 0.94) {
            flowGlow(ctx, pts[i].x, pts[i].y, 9, scheme.glow, 0.35);
        }
    }
    flowStops(ctx, (p) => {
        ctx.save();
        ctx.strokeStyle = scheme.body;
        ctx.globalAlpha = 0.85;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
    });
    flowDestRing(ctx, t, FLOW_FX.dest);
}

// FLOW_VARIANTS is the lab itself: the twenty flow cards (palette
// major, speed minor) followed by the twenty snake cards (scheme
// major, wave period minor).
const FLOW_VARIANTS = [];

for (const palette of FLOW_PALETTES) {
    for (const speed of FLOW_SPEEDS) {
        FLOW_VARIANTS.push({
            group: "flow",
            family: "flow",
            palette: palette.key,
            period: speed.period,
            name: palette.label + " flow " + speed.label,
            note: palette.note,
            draw(ctx, cell, t) {
                flowDrawFlow(ctx, palette, speed.period, t);
            }
        });
    }
}

for (const scheme of FLOW_SCHEMES) {
    for (const wave of FLOW_WAVES) {
        FLOW_VARIANTS.push({
            group: "snake",
            family: "snake",
            scheme: scheme.key,
            period: wave.period,
            name: scheme.label + " snake " + wave.label,
            note: scheme.note,
            draw(ctx, cell, t) {
                flowDrawSnake(ctx, scheme, wave.period, t);
            }
        });
    }
}

// PathFlow is the engine: it builds the card grid, owns the virtual
// clock (pause and speed multiply it) and renders every visible card
// from the shared loop time.
const PathFlow = {
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
    // "test-path-v3" mode.
    init() {
        this.grid = document.getElementById("flow-grid");
        this.scroll = document.getElementById("flow-scroll");
        const pause = document.getElementById("flow-pause");
        if (pause) {
            pause.addEventListener("click", () => {
                this.paused = !this.paused;
                pause.textContent = this.paused ? "resume" : "pause";
            });
        }
        const speed = document.getElementById("flow-speed");
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
            + FLOW_COLUMNS + ", " + FLOW_CELL_W + "px)";
        const dpr = (typeof window !== "undefined"
            && window.devicePixelRatio) || 1;
        this.cells = [];
        for (let i = 0; i < FLOW_VARIANTS.length; i++) {
            const variant = FLOW_VARIANTS[i];
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
            canvas.width = Math.round(FLOW_CELL_W * dpr);
            canvas.height = Math.round(FLOW_CELL_H * dpr);
            canvas.style.width = FLOW_CELL_W + "px";
            canvas.style.height = FLOW_CELL_H + "px";
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
                width: FLOW_CELL_W,
                height: FLOW_CELL_H
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
        const label = document.getElementById("flow-picked");
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
        image.src = FLOW_TILE_URL;
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
            cell.width = cell.card.offsetWidth || FLOW_CELL_W;
            cell.height = cell.card.offsetHeight || FLOW_CELL_H;
        }
        this.measured = true;
    },

    // render repaints the visible cards from the shared loop time.
    render() {
        this.measure();
        const loop = FLOW_TIMING.loopMs;
        const t = ((this.clock % loop) + loop) % loop;
        const walk = flowWalkAt(t);
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
        ctx.clearRect(0, 0, FLOW_CELL_W, FLOW_CELL_H);
        this.drawMapBackground(ctx);
        const variant = FLOW_VARIANTS[cell.index];
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
            const scale = FLOW_TILE_PX / FLOW_TILE_UNITS;
            const cx = (FLOW_BG_CENTER.x - FLOW_TILE_ORIGIN.x) * scale;
            const cy = (FLOW_BG_CENTER.y - FLOW_TILE_ORIGIN.y) * scale;
            const sw = FLOW_BG_WORLD_W * scale;
            const sh = sw * FLOW_CELL_H / FLOW_CELL_W;
            ctx.drawImage(this.tile, cx - sw / 2, cy - sh / 2,
                sw, sh, 0, 0, FLOW_CELL_W, FLOW_CELL_H);
        } else {
            ctx.fillStyle = "#e8e2d2";
            ctx.fillRect(0, 0, FLOW_CELL_W, FLOW_CELL_H);
            ctx.strokeStyle = "rgba(21, 34, 50, 0.08)";
            ctx.lineWidth = 1;
            for (let x = 30; x < FLOW_CELL_W; x += 30) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, FLOW_CELL_H);
                ctx.stroke();
            }
            for (let y = 30; y < FLOW_CELL_H; y += 30) {
                ctx.beginPath();
                ctx.moveTo(0, y);
                ctx.lineTo(FLOW_CELL_W, y);
                ctx.stroke();
            }
        }
        const edge = ctx.createLinearGradient(0, 0, 0, FLOW_CELL_H);
        edge.addColorStop(0, "rgba(10, 14, 20, 0.16)");
        edge.addColorStop(0.5, "rgba(10, 14, 20, 0)");
        edge.addColorStop(1, "rgba(10, 14, 20, 0.12)");
        ctx.fillStyle = edge;
        ctx.fillRect(0, 0, FLOW_CELL_W, FLOW_CELL_H);
    },

    // drawWalker paints the shared character marker: the blue self dot
    // of the map with the look direction tick. The marker fades in the
    // reset tail of the loop and pings when the walk arrives.
    drawWalker(ctx, walk) {
        const fade = walk.resetting
            ? Math.max(0, 1 - (walk.sinceArrive - FLOW_HOLD_MS)
                / FLOW_RESET_MS)
            : 1;
        if (walk.sinceArrive >= 0 && walk.sinceArrive < 700) {
            flowPulse(ctx, walk.x, walk.y, walk.sinceArrive / 700, {
                from: 4, to: 16, color: FLOW_FX.dest,
                alpha: 0.8, width: 2
            });
        }
        ctx.save();
        ctx.globalAlpha = 0.2 * fade;
        ctx.fillStyle = FLOW_FX.ink;
        ctx.beginPath();
        ctx.arc(walk.x + 1, walk.y + 2.5, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 0.22 * fade;
        ctx.fillStyle = FLOW_FX.self;
        ctx.beginPath();
        ctx.arc(walk.x, walk.y, 9, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = fade;
        ctx.fillStyle = FLOW_FX.self;
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
