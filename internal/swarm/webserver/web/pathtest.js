/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// PathTest is the walk plan visualization gallery of the -test-path-ui
// mode (webserver.NewTestPathServer answers mode "test-path"). It is a
// design decision aid, not a live view: twenty numbered ways to draw
// the planned walk of the bot on the map, every one animating the same
// route with the same clock, so a human compares them side by side and
// picks a favorite (or a combination of layers) by number.
//
// The layout: a four column grid of cards, one card per variant - the
// numbered head names the idea and its one line note, the canvas below
// shows the same planned walk. All cells share one route (the planning
// origin, five stops, the destination) and one walker: the walker
// walks the route at a constant pace, stands at every stop for a
// dwell (the follower re-issues the move there), holds at the
// destination and the loop restarts. Every effect is a pure function
// of the loop clock, so a repaint of the same moment is identical and
// the Node harness can assert that every variant moves.
//
// The complaint the gallery answers: the live plan line is static, its
// waypoint dots are too fat and nothing on it says "walk in progress
// in this direction". The variants separate the three axes - the line
// (dash flow, comet, chevrons, beads...), the stop marker (specks,
// ticks, numbers, rings) and the destination marker (ring, pin,
// bullseye) - so the best of each axis can be mixed afterwards.
const PATH_CELL_W = 380;
const PATH_CELL_H = 250;

// The grid columns; four cards fit a 1600 px viewport, the strip
// scrolls both ways otherwise.
const PATH_COLUMNS = 4;

// The map background: the 21_19 tile of the world pyramid (the open
// meadow east of the starter village, tile pixel 330/340) is the
// terrain the bot actually hunts, so the cells look like the live map
// while a thin line stays readable over the grass.
const PATH_TILE_URL = "maps/0/21_19.jpg";
const PATH_TILE_PX = 1024;
const PATH_TILE_UNITS = 32768;
const PATH_TILE_ORIGIN = { x: 32768, y: 32768 };
const PATH_BG_CENTER = { x: 43328, y: 43648 };
const PATH_BG_WORLD_W = 1900;

// The shared planned walk of every cell in cell pixels: the planning
// origin, five stops (the follower cursor targets) and the
// destination. The zig-zag mirrors a real geodata route - enough turns
// that a turn arrow and a numbered stop read.
const PATH_ROUTE = [
    { x: 46, y: 202 },
    { x: 92, y: 170 },
    { x: 140, y: 160 },
    { x: 176, y: 118 },
    { x: 232, y: 106 },
    { x: 286, y: 128 },
    { x: 334, y: 88 }
];

// The animation budget of one loop: the moving time over the whole
// route, the dwell at every intermediate stop, the arrival hold and
// the fade before the restart.
const PATH_TRAVEL_MS = 4200;
const PATH_DWELL_MS = 240;
const PATH_HOLD_MS = 900;
const PATH_RESET_MS = 300;

// The palette: the live magenta of the walk plan plus the accent
// colors the variants test against the light meadow imagery.
const PATH_FX = {
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
    dest: "#ff7a1a"
};

// PATH_TIMING is the arc length table of the route: the per point
// prefix distance, the arrival time of every point (dwells included)
// and the loop length. Computed once at load, every variant and the
// walker read it.
const PATH_TIMING = (() => {
    const route = PATH_ROUTE;
    const lens = [];
    let total = 0;
    for (let i = 1; i < route.length; i++) {
        const len = Math.hypot(route[i].x - route[i - 1].x,
            route[i].y - route[i - 1].y);
        lens.push(len);
        total += len;
    }
    const perMs = PATH_TRAVEL_MS / total;
    const prefix = [0];
    const arrivals = [0];
    let t = 0;
    for (let i = 1; i < route.length; i++) {
        prefix.push(prefix[i - 1] + lens[i - 1]);
        t += lens[i - 1] * perMs;
        arrivals.push(t);
        if (i < route.length - 1) { t += PATH_DWELL_MS; }
    }
    const arriveAt = arrivals[arrivals.length - 1];
    return {
        lens: lens,
        total: total,
        prefix: prefix,
        arrivals: arrivals,
        arriveAt: arriveAt,
        loopMs: arriveAt + PATH_HOLD_MS + PATH_RESET_MS
    };
})();

// PATH_SMOOTH is the Catmull-Rom resampling of the route with the arc
// distance of every sample: the smooth variants stroke it as one curve
// while the live map keeps the polyline.
const PATH_SMOOTH = (() => {
    const route = PATH_ROUTE;
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

// pathRgba expands a #rrggbb color into an rgba() string.
function pathRgba(hex, alpha) {
    const r = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return "rgba(" + r + ", " + g + ", " + b + ", " + alpha + ")";
}

// pathPointAtDistance returns the point, the unit direction and the arc
// length of one position of the route.
function pathPointAtDistance(distance) {
    const route = PATH_ROUTE;
    const timing = PATH_TIMING;
    const d = Math.max(0, Math.min(timing.total, distance));
    let index = route.length - 2;
    for (let i = 0; i < route.length - 1; i++) {
        if (d <= timing.prefix[i + 1]) {
            index = i;
            break;
        }
    }
    const a = route[index];
    const b = route[index + 1];
    const span = Math.max(1e-6,
        timing.prefix[index + 1] - timing.prefix[index]);
    const k = Math.max(0, Math.min(1,
        (d - timing.prefix[index]) / span));
    return {
        x: a.x + (b.x - a.x) * k,
        y: a.y + (b.y - a.y) * k,
        dirX: (b.x - a.x) / span,
        dirY: (b.y - a.y) / span,
        dist: d
    };
}

// pathRangePoints returns the polyline between two arc lengths, the
// route corners strictly inside the range included.
function pathRangePoints(from, to) {
    const total = PATH_TIMING.total;
    const a = Math.max(0, Math.min(total, from));
    const b = Math.max(0, Math.min(total, to));
    if (b - a < 0.5) { return []; }
    const head = pathPointAtDistance(a);
    const out = [{ x: head.x, y: head.y }];
    for (let i = 1; i < PATH_ROUTE.length - 1; i++) {
        const d = PATH_TIMING.prefix[i];
        if (d <= a || d >= b) { continue; }
        out.push({ x: PATH_ROUTE[i].x, y: PATH_ROUTE[i].y });
    }
    const tail = pathPointAtDistance(b);
    out.push({ x: tail.x, y: tail.y });
    return out;
}

// pathSamples returns the route points every spacing pixels starting
// at the phase offset, the direction of each included.
function pathSamples(spacing, phase) {
    const out = [];
    let d = phase === undefined ? 0 : phase;
    while (d <= PATH_TIMING.total) {
        out.push(pathPointAtDistance(d));
        d += spacing;
    }
    return out;
}

// pathStopDir returns the outgoing unit direction of one route point.
function pathStopDir(index) {
    const a = PATH_ROUTE[index];
    const b = PATH_ROUTE[index + 1];
    const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
    return { x: (b.x - a.x) / len, y: (b.y - a.y) / len };
}

// pathStroke strokes one polyline with the gallery style.
function pathStroke(ctx, points, style) {
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

// pathArrow draws one open chevron pointing along the direction.
function pathArrow(ctx, x, y, dirX, dirY, size, style) {
    const nx = -dirY;
    const ny = dirX;
    ctx.save();
    ctx.strokeStyle = style.color;
    ctx.globalAlpha = style.alpha === undefined ? 1 : style.alpha;
    ctx.lineWidth = style.width || 1.8;
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.beginPath();
    ctx.moveTo(x - dirX * size * 0.45 + nx * size * 0.62,
        y - dirY * size * 0.45 + ny * size * 0.62);
    ctx.lineTo(x + dirX * size * 0.55, y + dirY * size * 0.55);
    ctx.lineTo(x - dirX * size * 0.45 - nx * size * 0.62,
        y - dirY * size * 0.45 - ny * size * 0.62);
    ctx.stroke();
    ctx.restore();
}

// pathTick draws one perpendicular tick across the route.
function pathTick(ctx, x, y, dirX, dirY, half, style) {
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

// pathDot draws one filled marker with an optional rim.
function pathDot(ctx, x, y, radius, style) {
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

// pathDiamond draws one rotated square, the small stop marker several
// variants share.
function pathDiamond(ctx, x, y, size, color, alpha) {
    ctx.save();
    ctx.fillStyle = color;
    ctx.globalAlpha = alpha === undefined ? 1 : alpha;
    ctx.beginPath();
    ctx.moveTo(x, y - size);
    ctx.lineTo(x + size, y);
    ctx.lineTo(x, y + size);
    ctx.lineTo(x - size, y);
    ctx.closePath();
    ctx.fill();
    ctx.strokeStyle = "rgba(255, 255, 255, 0.9)";
    ctx.lineWidth = 0.9;
    ctx.stroke();
    ctx.restore();
}

// pathPulse draws one expanding ring of the 0..1 phase.
function pathPulse(ctx, x, y, phase, style) {
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

// pathGlow paints the radial halo of a bright head.
function pathGlow(ctx, x, y, radius, hex, alpha) {
    if (typeof ctx.createRadialGradient !== "function") { return; }
    const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius);
    gradient.addColorStop(0, pathRgba(hex, alpha));
    gradient.addColorStop(1, pathRgba(hex, 0));
    ctx.save();
    ctx.fillStyle = gradient;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
}

// pathStops walks the intermediate stops of the route (the planning
// origin and the destination excluded).
function pathStops(ctx, draw) {
    for (let i = 1; i < PATH_ROUTE.length - 1; i++) {
        draw(PATH_ROUTE[i], i);
    }
}

// pathDestRing is the shared breathing ring: a small core plus one
// expanding ring, the calmest destination marker of the gallery.
function pathDestRing(ctx, t, color) {
    const dest = PATH_ROUTE[PATH_ROUTE.length - 1];
    pathPulse(ctx, dest.x, dest.y, (t % 1200) / 1200, {
        from: 3.5, to: 11, color: color, alpha: 0.7, width: 1.4
    });
    pathDot(ctx, dest.x, dest.y, 2.8, {
        fill: color, ring: "rgba(255, 255, 255, 0.95)", ringWidth: 1.1
    });
}

// pathDestPin is the teardrop pin of the classic map style.
function pathDestPin(ctx, t, color) {
    const dest = PATH_ROUTE[PATH_ROUTE.length - 1];
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

// pathDestDiamond is the small diamond destination.
function pathDestDiamond(ctx, t, color) {
    const dest = PATH_ROUTE[PATH_ROUTE.length - 1];
    const size = 3.2 + 0.5 * Math.sin(t / 300);
    pathDiamond(ctx, dest.x, dest.y, size, color, 1);
}

// pathDestTarget is the bullseye destination: one pulse ring and two
// concentric strokes.
function pathDestTarget(ctx, t, color) {
    const dest = PATH_ROUTE[PATH_ROUTE.length - 1];
    pathPulse(ctx, dest.x, dest.y, (t % 1100) / 1100, {
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

// pathWalkAt maps the loop clock to the walker state: the point, the
// direction, the walked arc length, the number of passed stops and the
// arrival phase. The walker dwells at every intermediate stop and
// holds at the destination before the loop fades and restarts.
function pathWalkAt(clockMs) {
    const timing = PATH_TIMING;
    const last = PATH_ROUTE.length - 1;
    const loop = timing.loopMs;
    const t = ((clockMs % loop) + loop) % loop;
    if (t >= timing.arriveAt) {
        const dest = PATH_ROUTE[last];
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
            resetting: t >= timing.arriveAt + PATH_HOLD_MS
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
        (seg >= 1 ? PATH_DWELL_MS : 0);
    let dist = timing.prefix[seg];
    if (t >= dwellEnd) {
        const span = timing.arrivals[seg + 1] - dwellEnd;
        const k = span > 0 ? (t - dwellEnd) / span : 1;
        dist += timing.lens[seg] * Math.max(0, Math.min(1, k));
    }
    const p = pathPointAtDistance(dist);
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

// pathWave is the 0..1 travelling wave several dot variants sample:
// the position along the route sets the phase, the clock moves it.
function pathWave(dist, t, wavelength, periodMs) {
    return 0.5 + 0.5 * Math.sin((dist / wavelength - t / periodMs)
        * Math.PI * 2);
}

// PATH_VARIANTS is the gallery itself: every entry is one numbered
// idea with the shared route, the shared stops and the shared walker.
const PATH_VARIANTS = [
    {
        name: "marching ants",
        note: "dash flow · 2 px stop specks · ring",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 2.2, alpha: 0.95,
                dash: [7, 6], offset: -((t / 26) % 13)
            });
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2.2, {
                fill: PATH_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "comet tail",
        note: "bright head · 90 px glow tail · faint plan",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1.4, alpha: 0.3,
                dash: [3, 7]
            });
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2, {
                fill: PATH_FX.magenta, alpha: 0.75,
                ring: "rgba(255, 255, 255, 0.55)", ringWidth: 0.9
            }));
            const tail = pathRangePoints(
                Math.max(0, walk.dist - 96), walk.dist);
            for (let i = 1; i < tail.length; i++) {
                const k = i / Math.max(1, tail.length - 1);
                pathStroke(ctx, [tail[i - 1], tail[i]], {
                    color: i === tail.length - 1
                        ? PATH_FX.white : PATH_FX.glow,
                    width: 1.4 + 2.4 * k, alpha: 0.1 + 0.85 * k
                });
            }
            pathDestRing(ctx, t, PATH_FX.dest);
        },
        underlay(ctx, cell, t, walk) {
            pathGlow(ctx, walk.x, walk.y, 18, PATH_FX.glow, 0.45);
        }
    },
    {
        name: "walked vs ahead",
        note: "passed gray · ahead vivid · target ring",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, pathRangePoints(0, walk.dist), {
                color: PATH_FX.dim, width: 1.8, alpha: 0.55
            });
            const ahead = pathRangePoints(walk.dist, PATH_TIMING.total);
            pathStroke(ctx, ahead, {
                color: PATH_FX.magenta, width: 4.6, alpha: 0.14
            });
            pathStroke(ctx, ahead, {
                color: PATH_FX.magenta, width: 2.6, alpha: 0.95
            });
            let target = -1;
            for (let i = 1; i < PATH_ROUTE.length - 1; i++) {
                if (PATH_TIMING.prefix[i] > walk.dist) {
                    target = i;
                    break;
                }
            }
            pathStops(ctx, (p, i) => {
                const done = PATH_TIMING.prefix[i] <= walk.dist;
                const dir = pathStopDir(i);
                pathTick(ctx, p.x, p.y, dir.x, dir.y, 3.2, {
                    color: done ? PATH_FX.dim : PATH_FX.magenta,
                    alpha: done ? 0.55 : 0.9, width: 1.5
                });
                if (i === target) {
                    pathPulse(ctx, p.x, p.y, (t % 900) / 900, {
                        from: 4, to: 9, color: PATH_FX.magenta,
                        alpha: 0.65, width: 1.3
                    });
                }
            });
            pathDestDiamond(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "chevron conveyor",
        note: "arrow drift · passed dimmed · diamonds",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.dim, width: 1.2, alpha: 0.2
            });
            for (const s of pathSamples(26, (t / 34) % 26)) {
                const done = s.dist <= walk.dist;
                pathArrow(ctx, s.x, s.y, s.dirX, s.dirY, 7, {
                    color: done ? PATH_FX.dim : PATH_FX.magenta,
                    alpha: done ? 0.25 : 0.9, width: 1.7
                });
            }
            pathStops(ctx, (p) => {
                pathDiamond(ctx, p.x, p.y, 2.4, PATH_FX.magenta, 0.9);
            });
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "dot wave",
        note: "bead pulse wave · 3 px stop dots · ring",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1, alpha: 0.22
            });
            for (const s of pathSamples(15, 7.5)) {
                const wave = pathWave(s.dist, t, 70, 280);
                pathDot(ctx, s.x, s.y, 1.6 + 1.1 * wave, {
                    fill: PATH_FX.magenta, alpha: 0.3 + 0.65 * wave
                });
            }
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 3, {
                fill: PATH_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "neon glow",
        note: "additive halo · white core · cyan target",
        draw(ctx, cell, t) {
            ctx.save();
            ctx.globalCompositeOperation = "lighter";
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.cyan, width: 11, alpha: 0.1
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.cyan, width: 5, alpha: 0.22
            });
            ctx.restore();
            pathStroke(ctx, PATH_ROUTE, {
                color: "#d9fdff", width: 1.5, alpha: 0.95
            });
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2.1, {
                fill: "#eaffff", ring: PATH_FX.cyan, ringWidth: 1.2
            }));
            pathDestTarget(ctx, t, PATH_FX.cyan);
        },
        underlay(ctx, cell, t, walk) {
            pathGlow(ctx, walk.x, walk.y, 26, PATH_FX.cyan, 0.4);
        }
    },
    {
        name: "casing ribbon",
        note: "dark casing · center glint · pin",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.ink, width: 7, alpha: 0.45
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 2.6, alpha: 0.95
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.white, width: 0.8, alpha: 0.6
            });
            for (const s of pathSamples(46, 0)) {
                pathArrow(ctx, s.x, s.y, s.dirX, s.dirY, 6, {
                    color: PATH_FX.white, alpha: 0.8, width: 1.4
                });
            }
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2.6, {
                fill: PATH_FX.magenta,
                ring: "rgba(255, 255, 255, 0.95)", ringWidth: 1.2
            }));
            pathDestPin(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "numbered stops",
        note: "step badges 1-5 · target pulse · pin",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1.8, alpha: 0.8
            });
            let target = -1;
            for (let i = 1; i < PATH_ROUTE.length - 1; i++) {
                if (PATH_TIMING.prefix[i] > walk.dist) {
                    target = i;
                    break;
                }
            }
            pathStops(ctx, (p, i) => {
                const done = PATH_TIMING.prefix[i] <= walk.dist;
                if (i === target) {
                    pathPulse(ctx, p.x, p.y, (t % 900) / 900, {
                        from: 6, to: 10.5, color: PATH_FX.magenta,
                        alpha: 0.6, width: 1.2
                    });
                }
                pathDot(ctx, p.x, p.y, 5.4, {
                    fill: done ? PATH_FX.magenta
                        : "rgba(18, 24, 33, 0.88)",
                    ring: done ? "rgba(255, 255, 255, 0.95)"
                        : pathRgba(PATH_FX.magenta, 0.9),
                    ringWidth: 1.1
                });
                ctx.save();
                ctx.fillStyle = done ? "#ffffff"
                    : pathRgba(PATH_FX.magenta, 0.95);
                ctx.font = "700 8px system-ui, 'Segoe UI', sans-serif";
                ctx.textAlign = "center";
                ctx.textBaseline = "middle";
                ctx.fillText(String(i), p.x, p.y + 0.5);
                ctx.restore();
            });
            pathDestPin(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "footprints",
        note: "breadcrumb trail fading behind · dotted ahead",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx,
                pathRangePoints(walk.dist, PATH_TIMING.total), {
                    color: PATH_FX.violet, width: 2.2, alpha: 0.5,
                    dash: [1, 7]
                });
            for (const s of pathSamples(9, 4)) {
                if (s.dist > walk.dist) { continue; }
                const fade = Math.max(0.12,
                    1 - (walk.dist - s.dist) / 300);
                const side = Math.round(s.dist / 9) % 2 === 0 ? 1 : -1;
                const off = 2.6 * side;
                pathDot(ctx, s.x - s.dirY * off, s.y + s.dirX * off,
                    1.5 + 0.5 * fade, {
                        fill: PATH_FX.violet, alpha: 0.75 * fade
                    });
            }
            pathStops(ctx, (p, i) => {
                const dir = pathStopDir(i);
                pathTick(ctx, p.x, p.y, dir.x, dir.y, 2.6, {
                    color: PATH_FX.violet, alpha: 0.75, width: 1.3
                });
            });
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "distance ruler",
        note: "perpendicular ticks every 20 px · majors 100",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1.6, alpha: 0.85
            });
            for (const s of pathSamples(20, 0)) {
                const major = s.dist % 100 < 0.5;
                pathTick(ctx, s.x, s.y, s.dirX, s.dirY,
                    major ? 4.5 : 2.4, {
                        color: PATH_FX.magenta,
                        alpha: major ? 0.85 : 0.5,
                        width: major ? 1.6 : 1.1
                    });
            }
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2, {
                fill: PATH_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "smooth curve",
        note: "catmull-rom · sliding light window · ring",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_SMOOTH, {
                color: PATH_FX.magenta, width: 5.5, alpha: 0.14
            });
            pathStroke(ctx, PATH_SMOOTH, {
                color: PATH_FX.magenta, width: 2.4, alpha: 0.9
            });
            for (let i = 1; i < PATH_SMOOTH.length; i++) {
                const p = PATH_SMOOTH[i];
                const delta = Math.abs(p.dist - walk.dist);
                if (delta > 42) { continue; }
                const k = 1 - delta / 42;
                pathStroke(ctx, [PATH_SMOOTH[i - 1], p], {
                    color: PATH_FX.white, width: 1.4 + 1.6 * k,
                    alpha: 0.9 * k
                });
            }
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2.4, {
                fill: PATH_FX.magenta,
                ring: "rgba(255, 255, 255, 0.9)", ringWidth: 1
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "turn arrows",
        note: "white arrow at every bend · pin destination",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.ink, width: 6, alpha: 0.35
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 2.6, alpha: 0.95
            });
            for (let i = 1; i < PATH_ROUTE.length - 1; i++) {
                const p = PATH_ROUTE[i];
                const dir = pathStopDir(i);
                pathArrow(ctx, p.x + dir.x * 10, p.y + dir.y * 10,
                    dir.x, dir.y, 8, {
                        color: PATH_FX.white, alpha: 0.95, width: 2
                    });
                pathDot(ctx, p.x, p.y, 2.4, {
                    fill: PATH_FX.magenta,
                    ring: "rgba(255, 255, 255, 0.95)", ringWidth: 1.1
                });
            }
            pathDestPin(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "road median",
        note: "amber road · flowing center dash · crossing stops",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.ink, width: 7.5, alpha: 0.3
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.amber, width: 5, alpha: 0.85
            });
            pathStroke(ctx, PATH_ROUTE, {
                color: "#fff6e0", width: 1.3, alpha: 0.95,
                dash: [9, 7], offset: -((t / 30) % 16)
            });
            pathStops(ctx, (p, i) => {
                const dir = pathStopDir(i);
                pathTick(ctx, p.x, p.y, dir.x, dir.y, 4, {
                    color: "#fff6e0", alpha: 0.8, width: 1.6
                });
            });
            pathDestTarget(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "stop pulse rings",
        note: "ring wave down the stops · cyan target",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.cyan, width: 1.6, alpha: 0.5
            });
            pathStops(ctx, (p, i) => {
                let phase = (t / 1500 - i * 0.18) % 1;
                if (phase < 0) { phase += 1; }
                pathPulse(ctx, p.x, p.y, phase, {
                    from: 2.5, to: 11, color: PATH_FX.cyan,
                    alpha: 0.75, width: 1.3
                });
                pathDot(ctx, p.x, p.y, 2, {
                    fill: PATH_FX.cyan,
                    ring: "rgba(255, 255, 255, 0.6)", ringWidth: 0.8
                });
            });
            pathDestTarget(ctx, t, PATH_FX.cyan);
        }
    },
    {
        name: "energy beam",
        note: "width swells near the walker · white core",
        draw(ctx, cell, t, walk) {
            const total = PATH_TIMING.total;
            let prev = pathPointAtDistance(0);
            for (let d = 6; d <= total; d += 6) {
                const p = pathPointAtDistance(d);
                const k = Math.max(0,
                    1 - Math.abs(p.dist - walk.dist) / 150);
                pathStroke(ctx, [prev, p], {
                    color: k > 0.55 ? "#fff3fb" : PATH_FX.glow,
                    width: 1.3 + 5.5 * k * k,
                    alpha: 0.12 + 0.6 * k
                });
                prev = p;
            }
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.white, width: 1, alpha: 0.35
            });
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 2.2, {
                fill: PATH_FX.glow, alpha: 0.9
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "dash-dot march",
        note: "dash-dot rhythm · bracket stops · bullseye",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.lime, width: 2.2, alpha: 0.95,
                dash: [14, 5, 2, 5], offset: -((t / 30) % 26)
            });
            pathStops(ctx, (p, i) => {
                const dir = pathStopDir(i);
                const nx = -dir.y;
                const ny = dir.x;
                ctx.save();
                ctx.strokeStyle = PATH_FX.lime;
                ctx.globalAlpha = 0.9;
                ctx.lineWidth = 1.8;
                ctx.beginPath();
                ctx.moveTo(p.x - dir.x * 4 - nx * 4,
                    p.y - dir.y * 4 - ny * 4);
                ctx.lineTo(p.x - dir.x * 4 + nx * 4,
                    p.y - dir.y * 4 + ny * 4);
                ctx.moveTo(p.x + dir.x * 4 - nx * 4,
                    p.y + dir.y * 4 - ny * 4);
                ctx.lineTo(p.x + dir.x * 4 + nx * 4,
                    p.y + dir.y * 4 + ny * 4);
                ctx.stroke();
                ctx.restore();
            });
            pathDestTarget(ctx, t, PATH_FX.lime);
        }
    },
    {
        name: "bead chain",
        note: "beads every 20 px · hollow stop beads · ring",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1, alpha: 0.28
            });
            for (const s of pathSamples(20, 10)) {
                if (Math.abs(s.dist - walk.dist) < 12) { continue; }
                pathDot(ctx, s.x, s.y, 2.4, {
                    fill: PATH_FX.magenta, alpha: 0.9,
                    ring: "rgba(255, 255, 255, 0.75)", ringWidth: 0.8
                });
            }
            pathStops(ctx, (p) => {
                ctx.save();
                ctx.strokeStyle = PATH_FX.magenta;
                ctx.globalAlpha = 0.9;
                ctx.lineWidth = 1.6;
                ctx.beginPath();
                ctx.arc(p.x, p.y, 3.6, 0, Math.PI * 2);
                ctx.stroke();
                ctx.restore();
            });
            pathDestRing(ctx, t, PATH_FX.dest);
        },
        overlay(ctx, cell, t, walk) {
            pathDot(ctx, walk.x, walk.y, 5 + 0.8 * Math.sin(t / 200), {
                fill: PATH_FX.glow, alpha: 0.55,
                ring: "rgba(255, 255, 255, 0.85)", ringWidth: 1
            });
        }
    },
    {
        name: "contrail",
        note: "trail fades behind · dotted plan ahead",
        draw(ctx, cell, t, walk) {
            pathStroke(ctx,
                pathRangePoints(walk.dist, PATH_TIMING.total), {
                    color: PATH_FX.violet, width: 1.5, alpha: 0.35,
                    dash: [2, 8]
                });
            const from = Math.max(0, walk.dist - 130);
            let prev = pathPointAtDistance(from);
            for (let d = from + 5; d <= walk.dist; d += 5) {
                const p = pathPointAtDistance(d);
                const k = 1 - (walk.dist - p.dist) / 130;
                pathStroke(ctx, [prev, p], {
                    color: k > 0.75 ? PATH_FX.white : PATH_FX.violet,
                    width: 1 + 2.8 * k, alpha: 0.08 + 0.8 * k
                });
                prev = p;
            }
            pathStops(ctx, (p) => pathDot(ctx, p.x, p.y, 1.8, {
                fill: PATH_FX.violet, alpha: 0.6
            }));
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "railway",
        note: "sleeper ties every 9 px · bold station bars",
        draw(ctx, cell, t) {
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.amber, width: 2, alpha: 0.85
            });
            for (const s of pathSamples(9, 0)) {
                pathTick(ctx, s.x, s.y, s.dirX, s.dirY, 4, {
                    color: "#c77d10", alpha: 0.75, width: 1.6
                });
            }
            pathStops(ctx, (p, i) => {
                const dir = pathStopDir(i);
                pathTick(ctx, p.x, p.y, dir.x, dir.y, 5.5, {
                    color: PATH_FX.amber, alpha: 1, width: 3
                });
                pathDot(ctx, p.x, p.y, 1.8, {
                    fill: "#fff6e0", alpha: 0.95
                });
            });
            pathDestRing(ctx, t, PATH_FX.dest);
        }
    },
    {
        name: "draw-on reveal",
        note: "line draws ahead of the walker · spark head",
        draw(ctx, cell, t, walk) {
            const head = Math.min(PATH_TIMING.total, walk.dist + 90);
            pathStroke(ctx, PATH_ROUTE, {
                color: PATH_FX.magenta, width: 1.1, alpha: 0.2
            });
            pathStroke(ctx, pathRangePoints(0, head), {
                color: PATH_FX.magenta, width: 2.4, alpha: 0.9
            });
            pathStroke(ctx,
                pathRangePoints(Math.max(0, head - 46), head), {
                    color: PATH_FX.white, width: 2.6, alpha: 0.85
                });
            const spark = pathPointAtDistance(head);
            pathGlow(ctx, spark.x, spark.y, 12, PATH_FX.glow, 0.6);
            pathDot(ctx, spark.x, spark.y, 2.2, {
                fill: PATH_FX.white,
                ring: PATH_FX.magenta, ringWidth: 1
            });
            pathStops(ctx, (p, i) => {
                const dir = pathStopDir(i);
                pathTick(ctx, p.x, p.y, dir.x, dir.y, 3.2, {
                    color: PATH_FX.magenta, alpha: 0.8, width: 1.3
                });
            });
            pathDestTarget(ctx, t, PATH_FX.dest);
        }
    }
];

// PathTest is the engine: it builds the card grid, owns the virtual
// clock (pause and speed multiply it) and renders every visible card
// from the shared loop time.
const PathTest = {
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

    // init boots the gallery: the grid DOM, the map tile and the
    // render loop. Called by main.js when /api/config answers the
    // "test-path" mode.
    init() {
        this.grid = document.getElementById("path-grid");
        this.scroll = document.getElementById("path-scroll");
        const pause = document.getElementById("path-pause");
        if (pause) {
            pause.addEventListener("click", () => {
                this.paused = !this.paused;
                pause.textContent = this.paused ? "resume" : "pause";
            });
        }
        const speed = document.getElementById("path-speed");
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
        this.grid.style.gridTemplateColumns = "repeat(" + PATH_COLUMNS
            + ", " + PATH_CELL_W + "px)";
        const dpr = (typeof window !== "undefined"
            && window.devicePixelRatio) || 1;
        this.cells = [];
        for (let i = 0; i < PATH_VARIANTS.length; i++) {
            const variant = PATH_VARIANTS[i];
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
            canvas.width = Math.round(PATH_CELL_W * dpr);
            canvas.height = Math.round(PATH_CELL_H * dpr);
            canvas.style.width = PATH_CELL_W + "px";
            canvas.style.height = PATH_CELL_H + "px";
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
                width: PATH_CELL_W,
                height: PATH_CELL_H
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
        const label = document.getElementById("path-picked");
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
        image.src = PATH_TILE_URL;
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
    // the frame skips the cards outside the viewport the way the fight
    // gallery skips the off screen columns. The harness has no layout:
    // the missing offsets keep every card rendered.
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
            cell.width = cell.card.offsetWidth || PATH_CELL_W;
            cell.height = cell.card.offsetHeight || PATH_CELL_H;
        }
        this.measured = true;
    },

    // render repaints the visible cards from the shared loop time.
    render() {
        this.measure();
        const loop = PATH_TIMING.loopMs;
        const t = ((this.clock % loop) + loop) % loop;
        const walk = pathWalkAt(t);
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
        ctx.clearRect(0, 0, PATH_CELL_W, PATH_CELL_H);
        this.drawMapBackground(ctx);
        const variant = PATH_VARIANTS[cell.index];
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
            const scale = PATH_TILE_PX / PATH_TILE_UNITS;
            const cx = (PATH_BG_CENTER.x - PATH_TILE_ORIGIN.x) * scale;
            const cy = (PATH_BG_CENTER.y - PATH_TILE_ORIGIN.y) * scale;
            const sw = PATH_BG_WORLD_W * scale;
            const sh = sw * PATH_CELL_H / PATH_CELL_W;
            ctx.drawImage(this.tile, cx - sw / 2, cy - sh / 2, sw, sh,
                0, 0, PATH_CELL_W, PATH_CELL_H);
        } else {
            ctx.fillStyle = "#e8e2d2";
            ctx.fillRect(0, 0, PATH_CELL_W, PATH_CELL_H);
            ctx.strokeStyle = "rgba(21, 34, 50, 0.08)";
            ctx.lineWidth = 1;
            for (let x = 30; x < PATH_CELL_W; x += 30) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, PATH_CELL_H);
                ctx.stroke();
            }
            for (let y = 30; y < PATH_CELL_H; y += 30) {
                ctx.beginPath();
                ctx.moveTo(0, y);
                ctx.lineTo(PATH_CELL_W, y);
                ctx.stroke();
            }
        }
        const edge = ctx.createLinearGradient(0, 0, 0, PATH_CELL_H);
        edge.addColorStop(0, "rgba(10, 14, 20, 0.16)");
        edge.addColorStop(0.5, "rgba(10, 14, 20, 0)");
        edge.addColorStop(1, "rgba(10, 14, 20, 0.12)");
        ctx.fillStyle = edge;
        ctx.fillRect(0, 0, PATH_CELL_W, PATH_CELL_H);
    },

    // drawWalker paints the shared character marker: the blue self dot
    // of the map with the look direction tick. The marker fades in the
    // reset tail of the loop and pings when the walk arrives.
    drawWalker(ctx, walk) {
        const fade = walk.resetting
            ? Math.max(0, 1 - (walk.sinceArrive - PATH_HOLD_MS)
                / PATH_RESET_MS)
            : 1;
        if (walk.sinceArrive >= 0 && walk.sinceArrive < 700) {
            pathPulse(ctx, walk.x, walk.y, walk.sinceArrive / 700, {
                from: 4, to: 16, color: PATH_FX.dest,
                alpha: 0.8, width: 2
            });
        }
        ctx.save();
        ctx.globalAlpha = 0.2 * fade;
        ctx.fillStyle = PATH_FX.ink;
        ctx.beginPath();
        ctx.arc(walk.x + 1, walk.y + 2.5, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = 0.22 * fade;
        ctx.fillStyle = PATH_FX.self;
        ctx.beginPath();
        ctx.arc(walk.x, walk.y, 9, 0, Math.PI * 2);
        ctx.fill();
        ctx.globalAlpha = fade;
        ctx.fillStyle = PATH_FX.self;
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
