#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the walk plan visualization gallery of the
// -test-path-ui mode (internal/swarm/webserver/web/pathtest.js).
//
// It loads the real pathtest.js into a sandboxed context with a stub
// DOM and recording canvases, boots the gallery and checks the
// comparison grid against its brief:
//
// - the grid structure: 20 numbered cards (one canvas each) over a
//   four column template, every variant named, noted and drawable;
// - every variant engages at four moments of the walk loop: its layer
//   paints more than the engine baseline (the same clock with a
//   hook-less null variant) - a silent card is a dead variant;
// - every variant animates: the recorded paint trace of a card at two
//   moments of the loop differs (a static idea defeats the brief);
// - the shared timeline: the walker starts at the planning origin,
//   stands at every stop for the dwell, arrives at the destination,
//   counts the passed stops and wraps the loop;
// - the map tile background: every card paints the starter meadow
//   crop of the world tile pyramid;
// - the shared walker: exactly one blue self marker per card at the
//   same position;
// - the scroll window skips the cards outside the viewport;
// - the controls: pause freezes the clock, the speed select retimes
//   it, clicking a variant head toggles the pick and the toolbar
//   carries the picked numbers.
//
// Usage: node tools/repro_path_gallery.js [--path <pathtest.js>]
//        [--verbose]
// Exit code 0 = the gallery is correct, 1 = a check failed.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const PATH_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "pathtest.js");

const CELL_W = 380;
const CELL_H = 250;
const COLUMNS = 4;
const TILE_PX = 1024;

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
// stamps the stroke state onto every stroke, so a dash offset change
// alone still shows up as an animation.
function makeRecordingContext(state) {
    const ctx = {
        canvas: { width: CELL_W, height: CELL_H },
        lineDashOffset: 0,
        globalAlpha: 1,
        globalCompositeOperation: "source-over",
        font: "",
        textAlign: "start",
        textBaseline: "alphabetic",
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
                ctx.lineDashOffset]);
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

// loadPathJs creates the sandbox with the gallery DOM stubs, loads the
// real pathtest.js and returns the engine handles plus the created
// element registry.
function loadPathJs(pathFile) {
    const created = [];
    const ids = {};
    for (const id of ["path-grid", "path-scroll", "path-pause",
        "path-speed", "path-picked"]) {
        ids[id] = makeElement(id === "path-pause" ? "button"
            : (id === "path-speed" ? "select" : "div"));
    }
    ids["path-scroll"].clientWidth = 100000;
    ids["path-scroll"].clientHeight = 100000;

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
        { filename: "pathtest.js" });
    vm.runInContext(
        "globalThis.__PT = PathTest; globalThis.__V = PATH_VARIANTS;"
        + " globalThis.__TIMING = PATH_TIMING;"
        + " globalThis.__ROUTE = PATH_ROUTE;"
        + " globalThis.__WALK = pathWalkAt;"
        + " globalThis.__DWELL = PATH_DWELL_MS;", sandbox);

    return {
        PathTest: sandbox.__PT,
        variants: sandbox.__V,
        timing: sandbox.__TIMING,
        route: sandbox.__ROUTE,
        walk: sandbox.__WALK,
        dwell: sandbox.__DWELL,
        ids,
        created,
        rafCallbacks,
        setPerfNow: (value) => { perfNow = value; }
    };
}

// resetStates clears the paint record of every canvas cell.
function resetStates(gallery) {
    for (const element of gallery.created) {
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

// runScenarioStructure covers the gallery DOM: the numbered cards, the
// four column template and the variant table.
function runScenarioStructure(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    const byClass = (name) => gallery.created.filter(
        (el) => el.className === name);
    const cards = byClass("path-card");
    const heads = byClass("path-cardhead");
    const canvases = gallery.created.filter(
        (el) => el.tagName === "canvas");

    check(results, "20 variant cards", cards.length === 20,
        "found " + cards.length);
    check(results, "20 card heads", heads.length === 20,
        "found " + heads.length);
    check(results, "20 canvas cells", canvases.length === 20,
        "found " + canvases.length);

    const numbers = heads.map((head) =>
        head.children[0].textContent);
    let numbered = numbers.length === 20;
    for (let i = 0; i < numbers.length; i++) {
        if (numbers[i] !== String(i + 1).padStart(2, "0")) {
            numbered = false;
        }
    }
    check(results, "heads are numbered 01..20", numbered,
        "numbers: " + JSON.stringify(numbers));

    const template = gallery.ids["path-grid"].style
        .gridTemplateColumns;
    check(results, "grid template spans the four columns",
        template === "repeat(" + COLUMNS + ", " + CELL_W + "px)",
        "template: " + template);

    let described = true;
    const names = new Set();
    for (const variant of gallery.variants) {
        if (!variant.name || !variant.note
            || typeof variant.draw !== "function") {
            described = false;
        }
        names.add(variant.name);
    }
    check(results, "every variant has a name, a note and a draw hook",
        described, "one of the 20 variants is incomplete");
    check(results, "the variant names are unique",
        names.size === 20, "unique names: " + names.size);

    return results;
}

// runScenarioEngagement covers the heart of the brief: every variant
// must paint its own layer at four moments of the walk loop. The layer
// is measured against the engine baseline (the same clock rendered
// with a hook-less null variant).
function runScenarioEngagement(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    const cells = gallery.PathTest.cells;
    const samples = [120, 900, 2600, 5000];
    for (let index = 0; index < gallery.variants.length; index++) {
        let engaged = true;
        const saved = gallery.variants[index];
        for (const sample of samples) {
            gallery.variants[index] = {};
            resetStates(gallery);
            gallery.PathTest.clock = sample;
            gallery.PathTest.render();
            const baseline = opsOf(cells[index].canvas._state);
            gallery.variants[index] = saved;
            resetStates(gallery);
            gallery.PathTest.clock = sample;
            gallery.PathTest.render();
            const painted = opsOf(cells[index].canvas._state);
            if (painted <= baseline) { engaged = false; }
        }
        gallery.variants[index] = saved;
        check(results, "variant " + (index + 1) + " paints its layer",
            engaged, gallery.variants[index].name + " stayed silent");
    }

    return results;
}

// runScenarioAnimation covers the brief directly: an idea that does
// not move at all is a dead column. The recorded trace at two moments
// of the loop must differ.
function runScenarioAnimation(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    const cells = gallery.PathTest.cells;
    for (let index = 0; index < gallery.variants.length; index++) {
        const traces = [];
        for (const sample of [400, 900]) {
            resetStates(gallery);
            gallery.PathTest.clock = sample;
            gallery.PathTest.render();
            traces.push(cells[index].canvas._state.trace.join(";"));
        }
        check(results, "variant " + (index + 1) + " animates",
            traces[0] !== traces[1],
            gallery.variants[index].name + " painted the same frame");
    }

    return results;
}

// runScenarioTimeline covers the shared walk model: the origin, the
// dwell at every stop, the destination, the passed stop count and the
// loop wrap.
function runScenarioTimeline(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    const timing = gallery.timing;
    const route = gallery.route;
    const last = route.length - 1;
    const start = gallery.walk(0);
    check(results, "the walk starts at the planning origin",
        Math.abs(start.x - route[0].x) < 0.01
        && Math.abs(start.y - route[0].y) < 0.01 && start.dist === 0,
        "start " + start.x + "/" + start.y);

    let dwellOk = true;
    let stopsOk = true;
    for (let i = 1; i < last; i++) {
        const at = gallery.walk(timing.arrivals[i] + 10);
        if (Math.abs(at.x - route[i].x) > 0.01
            || Math.abs(at.y - route[i].y) > 0.01) {
            dwellOk = false;
        }
        const still = gallery.walk(timing.arrivals[i]
            + gallery.dwell - 10);
        if (Math.abs(still.dist - timing.prefix[i]) > 0.01) {
            dwellOk = false;
        }
        if (at.passedStops !== i) { stopsOk = false; }
    }
    check(results, "the walker stands at every stop for the dwell",
        dwellOk, "a stop was skipped");
    check(results, "passedStops counts the stops behind the walker",
        stopsOk, "the passed stop count is off");

    const arrived = gallery.walk(timing.arriveAt + 120);
    const dest = route[last];
    check(results, "the walk reaches the destination",
        arrived.arrived && arrived.dist === timing.total
        && Math.abs(arrived.x - dest.x) < 0.01
        && Math.abs(arrived.y - dest.y) < 0.01,
        "arrival state " + JSON.stringify(arrived));
    check(results, "the arrival hold precedes the reset",
        !arrived.resetting
        && gallery.walk(timing.arriveAt + 950).resetting,
        "the reset phase timing is off");

    const a = gallery.walk(700);
    const b = gallery.walk(timing.loopMs + 700);
    check(results, "the walk loop wraps without a jump",
        Math.abs(a.x - b.x) < 0.01 && Math.abs(a.y - b.y) < 0.01,
        "the wrapped frame differs");
    check(results, "the walker moves between two moments",
        gallery.walk(300).dist < gallery.walk(500).dist,
        "the walker stood still");

    return results;
}

// runScenarioBackdrop covers the map background: every card paints the
// starter meadow tile crop.
function runScenarioBackdrop(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    resetStates(gallery);
    gallery.PathTest.clock = 900;
    gallery.PathTest.render();

    const canvases = gallery.created.filter(
        (el) => el.tagName === "canvas");
    const rendered = canvases.filter(
        (el) => el._state.clears > 0);
    check(results, "all 20 cards rendered in the full visibility view",
        rendered.length === 20, "rendered " + rendered.length);

    const withTile = rendered.filter(
        (el) => el._state.images === 1 && el._state.imageArgs
        && el._state.imageArgs[0].src === "maps/0/21_19.jpg");
    check(results, "every card paints the meadow map tile",
        withTile.length === 20, "tiled cards: " + withTile.length);

    return results;
}

// runScenarioWalker covers the shared character marker: one blue self
// dot per card at the same position.
function runScenarioWalker(pathFile) {
    const gallery = loadPathJs(pathFile);
    gallery.PathTest.init();

    const results = [];
    resetStates(gallery);
    gallery.PathTest.clock = 1000;
    gallery.PathTest.render();

    const cells = gallery.PathTest.cells;
    const walk = gallery.walk(1000);
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
// row viewport only the cards of the near rows repaint.
function runScenarioScroll(pathFile) {
    const gallery = loadPathJs(pathFile);
    const engine = gallery.PathTest;
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
    gallery.ids["path-scroll"].scrollTop = 0;
    gallery.ids["path-scroll"].clientHeight = 520;
    resetStates(gallery);
    engine.clock = 900;
    engine.render();
    const first = engine.cells.filter(
        (cell) => cell.canvas._state.clears > 0).length;
    check(results, "the near rows repaint (8 of 20 cards)",
        first === 8, "repainted " + first);

    gallery.ids["path-scroll"].scrollTop = 650;
    resetStates(gallery);
    engine.render();
    const second = engine.cells.filter(
        (cell) => cell.canvas._state.clears > 0).length;
    check(results, "the scrolled rows repaint (8 of 20 cards)",
        second === 8, "repainted " + second);

    return results;
}

// runScenarioControls covers the toolbar: the pause, the speed select
// and the pick marks with the picked numbers chip.
function runScenarioControls(pathFile) {
    const gallery = loadPathJs(pathFile);
    const engine = gallery.PathTest;
    engine.init();

    const results = [];
    const pause = gallery.ids["path-pause"];
    const speed = gallery.ids["path-speed"];
    const chip = gallery.ids["path-picked"];

    check(results, "the picked chip starts empty",
        chip.textContent === "picked: none", chip.textContent);

    engine.frame(16.7);
    const clockAfterFrame = engine.clock;
    check(results, "the frame advances the clock",
        Math.abs(clockAfterFrame - 16.7) < 0.01,
        "clock " + clockAfterFrame);

    pause.fire("click");
    const frozen = gallery.rafCallbacks.pop();
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

    const heads = gallery.created.filter(
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
        ? args[pathIndex + 1] : PATH_JS;
    if (!fs.existsSync(pathFile)) {
        console.error("pathtest.js not found: " + pathFile);
        process.exit(1);
    }

    const scenarios = [
        ["gallery structure", runScenarioStructure(pathFile)],
        ["variant engagement", runScenarioEngagement(pathFile)],
        ["variant animation", runScenarioAnimation(pathFile)],
        ["walk timeline", runScenarioTimeline(pathFile)],
        ["map backdrop", runScenarioBackdrop(pathFile)],
        ["shared walker", runScenarioWalker(pathFile)],
        ["scroll window", runScenarioScroll(pathFile)],
        ["gallery controls", runScenarioControls(pathFile)]
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
