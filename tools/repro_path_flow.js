#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the flow and snake speed lab of the
// -test-path-ui-v3 mode (internal/swarm/webserver/web/pathflow.js).
//
// It loads the real pathflow.js into a sandboxed context with a stub
// DOM and recording canvases, boots the lab and checks the page
// against its brief:
//
// - the grid structure: 40 numbered cards (one canvas each) over a
//   four column template, every variant named, noted and drawable;
// - the family split: 01-20 rainbow flow and 21-40 pulse snake, each
//   family a full five schemes x four speeds grid;
// - the flow direction: a fixed hue band of every palette appears at a
//   larger curve fraction as the clock runs (start -> destination),
//   never behind the walker, and the four speed classes differ;
// - the snake direction: the width crest travels toward the
//   destination and the four wave periods differ;
// - every variant engages at four moments of the walk loop: its layer
//   paints more than the engine baseline (the same clock with a
//   hook-less null variant);
// - every variant animates: the recorded paint trace of a card at two
//   moments of the loop differs (the stroke colors included);
// - the shared timeline: the walker starts at the planning origin,
//   stands at every stop for the dwell, arrives at the destination,
//   counts the passed stops and wraps the loop;
// - the walker rides the curve, not the straight polyline;
// - the map tile background: every card paints the meadow crop;
// - the shared walker: exactly one blue self marker per card at the
//   same position;
// - the scroll window skips the cards outside the viewport;
// - the controls: pause freezes the clock, the speed select retimes
//   it, clicking a variant head toggles the pick and the toolbar
//   carries the picked numbers.
//
// Usage: node tools/repro_path_flow.js [--path <pathflow.js>]
//        [--verbose]
// Exit code 0 = the lab is correct, 1 = a check failed.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const FLOW_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "pathflow.js");

const CELL_W = 380;
const CELL_H = 250;
const COLUMNS = 4;
const TILE_PX = 1024;

// The expected speed grids, ascending: five schemes x four speeds per
// family.
const FLOW_PERIODS = [2800, 5200, 9000, 16000];
const SNAKE_PERIODS = [320, 560, 950, 1600];

// makeClassList is a minimal class list with toggle support (the pick
// highlight of the cards uses it).
function makeClassList() {
    const classes = new Set();
    return {
        contains: (name) => classes.has(name),
        add: (name) => classes.add(name),
        remove: (name) => classes.delete(name),
        toggle: (name, force) => {
            const on = force === undefined
                ? !classes.has(name) : Boolean(force);
            if (on) {
                classes.add(name);
            } else {
                classes.delete(name);
            }
            return on;
        }
    };
}

// makeState builds the paint record of one canvas: the counters, the
// arcs (the marker geometry probe) and the trace (the animation
// signature - every op with the paint state that shaped it).
function makeState() {
    return {
        clears: 0, fills: 0, strokes: 0, texts: [], images: 0,
        imageArgs: null, arcs: [], trace: []
    };
}

// note appends one op to the trace with its rounded arguments.
function note(state, op, args) {
    let line = op;
    for (const arg of args || []) {
        line += ":" + (typeof arg === "number"
            ? Math.round(arg * 10) / 10 : String(arg));
    }
    state.trace.push(line);
}

// makeRecordingContext records the paint calls of one cell canvas and
// stamps the stroke state (the color included, so a hue shift alone
// still shows up as an animation) onto every stroke.
function makeRecordingContext(state) {
    const ctx = {
        canvas: { width: CELL_W, height: CELL_H },
        lineDashOffset: 0,
        globalAlpha: 1,
        globalCompositeOperation: "source-over",
        font: "",
        textAlign: "start",
        textBaseline: "alphabetic",
        strokeStyle: "#000000",
        fillStyle: "#000000",
        setTransform: () => {},
        clearRect: () => { state.clears++; note(state, "clear"); },
        save: () => {},
        restore: () => {},
        translate: () => {},
        scale: () => {},
        drawImage: (...args) => {
            state.images++;
            state.imageArgs = args;
            note(state, "image", [args.length]);
        },
        beginPath: () => {},
        closePath: () => {},
        arc: (x, y, radius) => {
            state.arcs.push({ x: x, y: y, r: radius });
            note(state, "arc", [x, y, radius]);
        },
        moveTo: (x, y) => note(state, "move", [x, y]),
        lineTo: (x, y) => note(state, "line", [x, y]),
        fill: () => {
            state.fills++;
            note(state, "fill", [ctx.globalAlpha]);
        },
        stroke: () => {
            state.strokes++;
            note(state, "stroke", [ctx.globalAlpha,
                ctx.lineDashOffset, ctx.strokeStyle]);
        },
        fillRect: () => { state.fills++; note(state, "rect"); },
        fillText: (text, x, y) => {
            state.texts.push(String(text));
            note(state, "text", [String(text), x, y]);
        },
        strokeText: () => {},
        createLinearGradient: () => ({ addColorStop: () => {} }),
        createRadialGradient: () => ({ addColorStop: () => {} }),
        setLineDash: (dash) => note(state, "dash", dash || [])
    };
    return ctx;
}

// makeElement builds one DOM stub; canvas elements carry their
// recording state on _state and keep one stable context.
function makeElement(tag) {
    const listeners = {};
    const state = makeState();
    const element = {
        tagName: tag,
        className: "",
        textContent: "",
        title: "",
        value: "1",
        style: {},
        classList: makeClassList(),
        children: [],
        scrollLeft: 0,
        scrollTop: 0,
        clientWidth: 0,
        clientHeight: 0,
        _listeners: listeners,
        _state: state,
        appendChild(child) {
            element.children.push(child);
            return child;
        },
        addEventListener(type, fn) {
            (listeners[type] = listeners[type] || []).push(fn);
        },
        fire(type) {
            for (const fn of listeners[type] || []) { fn({}); }
        }
    };
    if (tag === "canvas") {
        element.width = 0;
        element.height = 0;
        const ctx = makeRecordingContext(state);
        element.getContext = () => ctx;
    }
    return element;
}

// loadFlowJs creates the sandbox with the lab DOM stubs, loads the
// real pathflow.js and returns the engine handles plus the created
// element registry.
function loadFlowJs(pathFile) {
    const created = [];
    const ids = {};
    for (const id of ["flow-grid", "flow-scroll", "flow-pause",
        "flow-speed", "flow-picked"]) {
        ids[id] = makeElement(id === "flow-pause" ? "button"
            : (id === "flow-speed" ? "select" : "div"));
    }
    ids["flow-scroll"].clientWidth = 100000;
    ids["flow-scroll"].clientHeight = 100000;

    const rafCallbacks = [];
    let perfNow = 0;

    const sandbox = {
        Math, JSON,
        performance: { now: () => perfNow },
        requestAnimationFrame: (cb) => { rafCallbacks.push(cb); },
        document: {
            getElementById: (id) => ids[id] || null,
            createElement: (tag) => {
                const element = makeElement(tag);
                created.push(element);
                return element;
            }
        },
        window: { devicePixelRatio: 1, addEventListener: () => {} },
        Image: class StubImage {
            constructor() {
                this.complete = false;
                this.naturalWidth = 0;
                this.naturalHeight = 0;
            }
            set src(value) {
                this._src = value;
                this.complete = true;
                this.naturalWidth = TILE_PX;
                this.naturalHeight = TILE_PX;
                if (this.onload) { this.onload(); }
            }
            get src() { return this._src; }
        }
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(pathFile, "utf8"), sandbox,
        { filename: "pathflow.js" });
    vm.runInContext(
        "globalThis.__PF = PathFlow;"
        + " globalThis.__V = FLOW_VARIANTS;"
        + " globalThis.__TIMING = FLOW_TIMING;"
        + " globalThis.__ROUTE = FLOW_ROUTE;"
        + " globalThis.__WALK = flowWalkAt;"
        + " globalThis.__DWELL = FLOW_DWELL_MS;"
        + " globalThis.__SMOOTH = FLOW_SMOOTH;"
        + " globalThis.__STOPS = FLOW_STOP_DIST;"
        + " globalThis.__POINT = flowPointAtDistance;"
        + " globalThis.__COLOR = flowColorAt;"
        + " globalThis.__WAVE = flowWave;", sandbox);

    return {
        PathFlow: sandbox.__PF,
        variants: sandbox.__V,
        timing: sandbox.__TIMING,
        route: sandbox.__ROUTE,
        walk: sandbox.__WALK,
        dwell: sandbox.__DWELL,
        smooth: sandbox.__SMOOTH,
        stops: sandbox.__STOPS,
        point: sandbox.__POINT,
        colorAt: sandbox.__COLOR,
        wave: sandbox.__WAVE,
        ids,
        created,
        rafCallbacks,
        setPerfNow: (value) => { perfNow = value; }
    };
}

// resetStates clears the paint record of every canvas cell.
function resetStates(lab) {
    for (const element of lab.created) {
        if (element.tagName !== "canvas") { continue; }
        const state = element._state;
        state.clears = 0;
        state.fills = 0;
        state.strokes = 0;
        state.texts = [];
        state.images = 0;
        state.imageArgs = null;
        state.arcs = [];
        state.trace = [];
    }
}

// opsOf sums the paint operations of one canvas cell state.
function opsOf(state) {
    return state.fills + state.strokes + state.images
        + state.texts.length;
}

// check verifies one property and collects the results.
function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

// sortedPeriods returns the distinct periods of one family filtered by
// the key field, ascending.
function sortedPeriods(variants, field, key) {
    return variants.filter((variant) => variant[field] === key)
        .map((variant) => variant.period)
        .sort((a, b) => a - b);
}

// runScenarioStructure covers the lab DOM and the variant table: the
// numbered cards, the four column template, the family split and the
// five schemes x four speeds grid of each family.
function runScenarioStructure(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    const byClass = (name) => lab.created.filter(
        (el) => el.className === name);
    const cards = byClass("path-card");
    const heads = byClass("path-cardhead");
    const canvases = lab.created.filter(
        (el) => el.tagName === "canvas");

    check(results, "40 variant cards", cards.length === 40,
        "found " + cards.length);
    check(results, "40 card heads", heads.length === 40,
        "found " + heads.length);
    check(results, "40 canvas cells", canvases.length === 40,
        "found " + canvases.length);

    const numbers = heads.map((head) =>
        head.children[0].textContent);
    let numbered = numbers.length === 40;
    for (let i = 0; i < numbers.length; i++) {
        if (numbers[i] !== String(i + 1).padStart(2, "0")) {
            numbered = false;
        }
    }
    check(results, "heads are numbered 01..40", numbered,
        "numbers: " + JSON.stringify(numbers));

    const template = lab.ids["flow-grid"].style
        .gridTemplateColumns;
    check(results, "grid template spans the four columns",
        template === "repeat(" + COLUMNS + ", " + CELL_W + "px)",
        "template: " + template);

    let described = true;
    const names = new Set();
    for (const variant of lab.variants) {
        if (!variant.name || !variant.note
            || typeof variant.draw !== "function") {
            described = false;
        }
        names.add(variant.name);
    }
    check(results, "every variant has a name, a note and a draw hook",
        described, "one of the 40 variants is incomplete");
    check(results, "the variant names are unique",
        names.size === 40, "unique names: " + names.size);

    const flow = lab.variants.slice(0, 20);
    const snake = lab.variants.slice(20);
    check(results, "cards 01-20 are the rainbow flow",
        flow.every((variant) => variant.family === "flow"),
        "the first twenty are not all flow");
    check(results, "cards 21-40 are the pulse snake",
        snake.every((variant) => variant.family === "snake"),
        "the last twenty are not all snake");

    const palettes = new Set(flow.map((variant) => variant.palette));
    const schemes = new Set(snake.map((variant) => variant.scheme));
    check(results, "five flow palettes", palettes.size === 5,
        "palettes: " + JSON.stringify([...palettes]));
    check(results, "five snake crest schemes", schemes.size === 5,
        "schemes: " + JSON.stringify([...schemes]));

    let flowGrid = palettes.size === 5;
    for (const key of palettes) {
        const periods = sortedPeriods(flow, "palette", key);
        if (periods.join(",") !== FLOW_PERIODS.join(",")) {
            flowGrid = false;
        }
    }
    check(results, "every palette runs the four flow speeds",
        flowGrid, "a flow speed class is missing or repeated");

    let snakeGrid = schemes.size === 5;
    for (const key of schemes) {
        const periods = sortedPeriods(snake, "scheme", key);
        if (periods.join(",") !== SNAKE_PERIODS.join(",")) {
            snakeGrid = false;
        }
    }
    check(results, "every scheme runs the four snake periods",
        snakeGrid, "a snake speed class is missing or repeated");

    return results;
}

// runScenarioDirection covers the heart of the brief: the flow hue
// bands travel from the start to the destination (a fixed hue appears
// at a larger curve fraction as the clock runs, never at a smaller
// one), and the snake crest travels the same way.
function runScenarioDirection(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    const palette = {
        from: 0, span: 360, sat: 96, light: 60, bands: 2
    };

    let forward = true;
    for (const period of FLOW_PERIODS) {
        const u = 0.37;
        const k0 = u / palette.bands;
        const t1 = Math.round(period / 4);
        const k1 = (u + t1 / period) / palette.bands;
        const a = lab.colorAt(k0, 0, palette, period);
        const b = lab.colorAt(k1, t1, palette, period);
        if (a !== b || k1 <= k0) { forward = false; }
    }
    check(results, "the flow hue bands move from start to destination",
        forward, "a fixed hue jumped backwards or stood still");

    const drift = lab.colorAt(0.25, 0, palette, 16000)
        !== lab.colorAt(0.25, 800, palette, 16000);
    check(results, "the flow band recolors a fixed curve point",
        drift, "the stripe did not move over the point");

    const flowColors = new Set();
    for (const period of FLOW_PERIODS) {
        flowColors.add(lab.colorAt(0.5, 700, palette, period));
    }
    check(results, "the four flow speeds render differently",
        flowColors.size === 4, "colors: "
        + JSON.stringify([...flowColors]));

    const wavelength = 84;
    const period = 620;
    const crest0 = wavelength * 0.25;
    const crest1 = crest0 + wavelength * (155 / period);
    const atZero = lab.wave(crest0, 0, wavelength, period);
    const after = lab.wave(crest1, 155, wavelength, period);
    const before = lab.wave(crest1, 0, wavelength, period);
    check(results, "the snake crest travels toward the destination",
        Math.abs(atZero - 1) < 1e-9 && Math.abs(after - 1) < 1e-9
        && before < 0.6,
        "crest " + atZero.toFixed(3) + " -> " + after.toFixed(3)
        + " (was " + before.toFixed(3) + " ahead)");

    const snakeWaves = new Set();
    for (const periodMs of SNAKE_PERIODS) {
        snakeWaves.add(lab.wave(100, 500, wavelength, periodMs)
            .toFixed(4));
    }
    check(results, "the four snake periods render differently",
        snakeWaves.size === 4, "waves: "
        + JSON.stringify([...snakeWaves]));

    return results;
}

// runScenarioEngagement covers the heart of the brief: every variant
// must paint its own layer at four moments of the walk loop. The layer
// is measured against the engine baseline (the same clock rendered
// with a hook-less null variant).
function runScenarioEngagement(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    const cells = lab.PathFlow.cells;
    const samples = [120, 900, 2600, 5000];
    for (let index = 0; index < lab.variants.length; index++) {
        let engaged = true;
        const saved = lab.variants[index];
        for (const sample of samples) {
            lab.variants[index] = {};
            resetStates(lab);
            lab.PathFlow.clock = sample;
            lab.PathFlow.render();
            const baseline = opsOf(cells[index].canvas._state);
            lab.variants[index] = saved;
            resetStates(lab);
            lab.PathFlow.clock = sample;
            lab.PathFlow.render();
            const painted = opsOf(cells[index].canvas._state);
            if (painted <= baseline) { engaged = false; }
        }
        lab.variants[index] = saved;
        check(results, "variant " + (index + 1) + " paints its layer",
            engaged, lab.variants[index].name + " stayed silent");
    }

    return results;
}

// runScenarioAnimation covers the brief directly: an idea that does
// not move at all is a dead column. The recorded trace at two moments
// of the loop must differ (the stroke colors included, so a slow hue
// shift still counts).
function runScenarioAnimation(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    const cells = lab.PathFlow.cells;
    for (let index = 0; index < lab.variants.length; index++) {
        const traces = [];
        for (const sample of [400, 900]) {
            resetStates(lab);
            lab.PathFlow.clock = sample;
            lab.PathFlow.render();
            traces.push(cells[index].canvas._state.trace.join(";"));
        }
        check(results, "variant " + (index + 1) + " animates",
            traces[0] !== traces[1],
            lab.variants[index].name + " painted the same frame");
    }

    return results;
}

// runScenarioTimeline covers the shared walk model: the origin, the
// dwell at every stop, the destination, the passed stop count, the
// loop wrap and the walk along the curve (not the straight legs).
function runScenarioTimeline(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    const timing = lab.timing;
    const route = lab.route;
    const last = route.length - 1;
    const start = lab.walk(0);
    check(results, "the walk starts at the planning origin",
        Math.abs(start.x - route[0].x) < 0.01
        && Math.abs(start.y - route[0].y) < 0.01 && start.dist === 0,
        "start " + start.x + "/" + start.y);

    let dwellOk = true;
    let stopsOk = true;
    for (let i = 1; i < last; i++) {
        const at = lab.walk(timing.arrivals[i] + 10);
        if (Math.abs(at.x - route[i].x) > 0.01
            || Math.abs(at.y - route[i].y) > 0.01) {
            dwellOk = false;
        }
        const still = lab.walk(timing.arrivals[i]
            + lab.dwell - 10);
        if (Math.abs(still.dist - timing.prefix[i]) > 0.01) {
            dwellOk = false;
        }
        if (at.passedStops !== i) { stopsOk = false; }
    }
    check(results, "the walker stands at every stop for the dwell",
        dwellOk, "a stop was skipped");
    check(results, "passedStops counts the stops behind the walker",
        stopsOk, "the passed stop count is off");

    const arrived = lab.walk(timing.arriveAt + 120);
    const dest = route[last];
    check(results, "the walk reaches the destination",
        arrived.arrived && arrived.dist === timing.total
        && Math.abs(arrived.x - dest.x) < 0.01
        && Math.abs(arrived.y - dest.y) < 0.01,
        "arrival state " + JSON.stringify(arrived));
    check(results, "the arrival hold precedes the reset",
        !arrived.resetting
        && lab.walk(timing.arriveAt + 950).resetting,
        "the reset phase timing is off");

    const a = lab.walk(700);
    const b = lab.walk(timing.loopMs + 700);
    check(results, "the walk loop wraps without a jump",
        Math.abs(a.x - b.x) < 0.01 && Math.abs(a.y - b.y) < 0.01,
        "the wrapped frame differs");
    check(results, "the walker moves between two moments",
        lab.walk(300).dist < lab.walk(500).dist,
        "the walker stood still");

    let bow = 0;
    for (let i = 0; i < last; i++) {
        const depart = timing.arrivals[i]
            + (i >= 1 ? lab.dwell : 0);
        const mid = (depart + timing.arrivals[i + 1]) / 2;
        const p = lab.walk(mid);
        const sx = (route[i].x + route[i + 1].x) / 2;
        const sy = (route[i].y + route[i + 1].y) / 2;
        bow = Math.max(bow, Math.hypot(p.x - sx, p.y - sy));
    }
    check(results, "the walker rides the curve off the polyline",
        bow > 1, "max walker bow " + bow.toFixed(2) + " px");

    return results;
}

// runScenarioBackdrop covers the map background: every card paints the
// meadow tile crop.
function runScenarioBackdrop(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    resetStates(lab);
    lab.PathFlow.clock = 900;
    lab.PathFlow.render();

    const canvases = lab.created.filter(
        (el) => el.tagName === "canvas");
    const rendered = canvases.filter(
        (el) => el._state.clears > 0);
    check(results, "all 40 cards rendered in the full visibility view",
        rendered.length === 40, "rendered " + rendered.length);

    const withTile = rendered.filter(
        (el) => el._state.images === 1 && el._state.imageArgs
        && el._state.imageArgs[0].src === "maps/0/21_19.jpg");
    check(results, "every card paints the meadow map tile",
        withTile.length === 40, "tiled cards: " + withTile.length);

    return results;
}

// runScenarioWalker covers the shared character marker: one blue self
// dot per card at the same position.
function runScenarioWalker(pathFile) {
    const lab = loadFlowJs(pathFile);
    lab.PathFlow.init();

    const results = [];
    resetStates(lab);
    lab.PathFlow.clock = 1000;
    lab.PathFlow.render();

    const cells = lab.PathFlow.cells;
    const walk = lab.walk(1000);
    let found = 0;
    let same = true;
    for (const cell of cells) {
        const marks = cell.canvas._state.arcs.filter(
            (arc) => Math.abs(arc.r - 4.4) < 0.001
                && Math.abs(arc.x - walk.x) < 0.01
                && Math.abs(arc.y - walk.y) < 0.01);
        if (marks.length !== 1) {
            found++;
            continue;
        }
        if (marks[0].x !== walk.x || marks[0].y !== walk.y) {
            same = false;
        }
    }
    check(results, "every card draws exactly one shared walker",
        found === 0, found + " cards are off");
    check(results, "the walker sits at the same point in every card",
        same, "the cards disagree on the walker position");

    return results;
}

// runScenarioScroll covers the visibility skip: with a simulated two
// row viewport only the cards of the near rows repaint, and scrolling
// moves the repainted band down the grid.
function runScenarioScroll(pathFile) {
    const lab = loadFlowJs(pathFile);
    const engine = lab.PathFlow;
    engine.init();

    const results = [];
    for (const cell of engine.cells) {
        const col = cell.index % COLUMNS;
        const row = Math.floor(cell.index / COLUMNS);
        cell.top = row * (CELL_H + 60);
        cell.left = col * (CELL_W + 10);
        cell.width = CELL_W;
        cell.height = CELL_H + 40;
    }
    engine.measured = true;
    lab.ids["flow-scroll"].scrollTop = 0;
    lab.ids["flow-scroll"].clientHeight = 520;
    resetStates(lab);
    engine.clock = 900;
    engine.render();
    const first = engine.cells.filter(
        (cell) => cell.canvas._state.clears > 0);
    check(results, "the near rows repaint (8 of 40 cards)",
        first.length === 8, "repainted " + first.length);
    check(results, "the first view paints cards 01-08",
        first.every((cell) => cell.index < 8),
        "indices: " + first.map((cell) => cell.index).join(","));

    lab.ids["flow-scroll"].scrollTop = 650;
    resetStates(lab);
    engine.render();
    const second = engine.cells.filter(
        (cell) => cell.canvas._state.clears > 0);
    check(results, "the scrolled rows repaint (8 of 40 cards)",
        second.length === 8, "repainted " + second.length);
    check(results, "the scrolled view paints cards 09-16",
        second.every((cell) => cell.index >= 8 && cell.index < 16),
        "indices: " + second.map((cell) => cell.index).join(","));

    return results;
}

// runScenarioControls covers the toolbar: the pause, the speed select
// and the pick marks with the picked numbers chip.
function runScenarioControls(pathFile) {
    const lab = loadFlowJs(pathFile);
    const engine = lab.PathFlow;
    engine.init();

    const results = [];
    const pause = lab.ids["flow-pause"];
    const speed = lab.ids["flow-speed"];
    const chip = lab.ids["flow-picked"];

    check(results, "the picked chip starts empty",
        chip.textContent === "picked: none", chip.textContent);

    engine.frame(16.7);
    const clockAfterFrame = engine.clock;
    check(results, "the frame advances the clock",
        Math.abs(clockAfterFrame - 16.7) < 0.01,
        "clock " + clockAfterFrame);

    pause.fire("click");
    const frozen = lab.rafCallbacks.pop();
    frozen(33.4);
    check(results, "the pause button freezes the clock",
        engine.clock === clockAfterFrame,
        "clock moved to " + engine.clock);
    check(results, "the pause button relabels itself to resume",
        pause.textContent === "resume", "label " + pause.textContent);
    pause.fire("click");
    check(results, "the resume continues the clock",
        pause.textContent === "pause", "label " + pause.textContent);

    speed.value = "2";
    speed.fire("change");
    check(results, "the speed select sets the playback speed",
        engine.speed === 2, "speed " + engine.speed);

    const heads = lab.created.filter(
        (el) => el.className === "path-cardhead");
    heads[2].fire("click");
    check(results, "clicking a head marks the variant",
        heads[2].classList.contains("picked")
        && engine.cells[2].card.classList.contains("picked"),
        "the pick did not mark the card");
    check(results, "the chip lists the picked number",
        chip.textContent === "picked: 03", chip.textContent);

    heads[0].fire("click");
    check(results, "the chip lists a combination",
        chip.textContent === "picked: 01, 03", chip.textContent);

    heads[2].fire("click");
    check(results, "clicking again clears the mark",
        !heads[2].classList.contains("picked")
        && !engine.cells[2].card.classList.contains("picked"),
        "the pick did not clear");
    check(results, "the chip drops the cleared number",
        chip.textContent === "picked: 01", chip.textContent);

    return results;
}

function main() {
    const args = process.argv.slice(2);
    const verbose = args.includes("--verbose");
    const pathIndex = args.indexOf("--path");
    const pathFile = pathIndex >= 0 && args[pathIndex + 1]
        ? args[pathIndex + 1] : FLOW_JS;
    if (!fs.existsSync(pathFile)) {
        console.error("pathflow.js not found: " + pathFile);
        process.exit(1);
    }

    const scenarios = [
        ["lab structure", runScenarioStructure(pathFile)],
        ["flow and snake direction", runScenarioDirection(pathFile)],
        ["variant engagement", runScenarioEngagement(pathFile)],
        ["variant animation", runScenarioAnimation(pathFile)],
        ["curve walk timeline", runScenarioTimeline(pathFile)],
        ["map backdrop", runScenarioBackdrop(pathFile)],
        ["shared walker", runScenarioWalker(pathFile)],
        ["scroll window", runScenarioScroll(pathFile)],
        ["lab controls", runScenarioControls(pathFile)]
    ];
    let failed = 0;
    for (const [name, results] of scenarios) {
        console.log("scenario: " + name);
        for (const result of results) {
            const mark = result.ok ? "PASS" : "FAIL";
            console.log("  " + mark + "  " + result.name);
            if (!result.ok || verbose) {
                console.log("        " + result.detail);
            }
            if (!result.ok) { failed++; }
        }
    }
    console.log(failed === 0 ? "ALL PASS" : failed + " CHECKS FAILED");

    process.exit(failed === 0 ? 0 : 1);
}

main();
