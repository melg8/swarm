#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the fight FX gallery of the -test-fight-ui
// mode (internal/swarm/webserver/web/fighttest.js).
//
// It loads the real fighttest.js into a sandboxed context with a stub
// DOM and recording canvases, boots the gallery and checks the
// comparison grid against its brief:
//
// - the grid structure: 18 numbered variant columns x 4 enemy
//   placement rows (above, below, left, right) = 72 canvas cells, all
//   under the horizontally scrollable strip;
// - every variant engages during ALL FOUR beats of the loop (hero
//   hit, taken hit, hero crit, crit taken): either its glyph layer
//   draws more than the engine baseline (the same clock rendered with
//   a hook-less null variant) or one of its motion hooks returns a
//   non null effect - a silent variant is a dead column;
// - the beat caption and the map tile background: every cell paints
//   the starter meadow tile crop and labels the running beat;
// - the HP integration and the attack lunge geometry follow the beat
//   timeline (the attacker steps toward the target, the target
//   stays);
// - the horizontal scroll window skips the off screen columns;
// - the controls work: pause freezes the clock, the speed select
//   changes it, the column pick toggles the highlight.
//
// Usage: node tools/repro_fight_ui.js [--fight <fighttest.js>] [--verbose]
// Exit code 0 = the gallery is correct, 1 = a check failed.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const FIGHT_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "fighttest.js");

const CELL_W = 300;
const ROWHEAD_W = 112;
const TILE_PX = 1024;

// makeClassList is a minimal class list with toggle support (the pick
// highlight of the column heads uses it).
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

// makeRecordingContext counts the paint operations of one cell canvas
// so the harness can assert which layers drew during a render.
function makeRecordingContext(state) {
    return {
        canvas: { width: CELL_W, height: 190 },
        setTransform: () => {},
        clearRect: () => { state.clears++; },
        save: () => {},
        restore: () => {},
        translate: () => {},
        scale: () => {},
        rotate: () => {},
        drawImage: (...args) => {
            state.images++;
            state.imageArgs = args;
        },
        beginPath: () => {},
        closePath: () => {},
        arc: () => {},
        ellipse: () => {},
        moveTo: () => {},
        lineTo: () => {},
        fill: () => { state.fills++; },
        stroke: () => { state.strokes++; },
        fillRect: () => { state.fills++; },
        strokeRect: () => { state.strokes++; },
        fillText: (text) => { state.texts.push(String(text)); },
        strokeText: () => {},
        createLinearGradient: () => ({ addColorStop: () => {} }),
        setLineDash: () => {}
    };
}

// makeElement builds one DOM stub; canvas elements carry their
// recording state on _state and keep one stable context.
function makeElement(tag) {
    const listeners = {};
    const state = {
        clears: 0, fills: 0, strokes: 0, texts: [], textOps: 0,
        images: 0, imageArgs: null
    };
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
        clientWidth: 99999,
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

// loadFightJs creates the sandbox with the gallery DOM stubs, loads
// the real fighttest.js and returns the engine handles plus the
// created element registry.
function loadFightJs(fightFile) {
    const created = [];
    const ids = {};
    for (const id of ["fight-grid", "fight-scroll", "fight-pause",
        "fight-speed"]) {
        ids[id] = makeElement(id === "fight-grid" || id === "fight-scroll"
            ? "div" : (id === "fight-pause" ? "button" : "select"));
    }
    ids["fight-scroll"].clientWidth = 99999;

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
    vm.runInContext(fs.readFileSync(fightFile, "utf8"), sandbox,
        { filename: "fighttest.js" });
    vm.runInContext(
        "globalThis.__FT = FightTest; globalThis.__V = FIGHT_VARIANTS;"
        + " globalThis.__B = FIGHT_BEATS;", sandbox);

    return {
        FightTest: sandbox.__FT,
        variants: sandbox.__V,
        beats: sandbox.__B,
        ids,
        created,
        rafCallbacks,
        setPerfNow: (value) => { perfNow = value; }
    };
}

// resetStates clears the paint counters of every canvas cell.
function resetStates(gallery) {
    for (const element of gallery.created) {
        if (element.tagName !== "canvas") { continue; }
        const state = element._state;
        state.clears = 0;
        state.fills = 0;
        state.strokes = 0;
        state.texts = [];
        state.textOps = 0;
        state.images = 0;
        state.imageArgs = null;
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

// runScenarioStructure covers the gallery DOM: the corner, the 18
// numbered column heads, the 4 placement row heads and the 72 canvas
// cells under the scroll strip.
function runScenarioStructure(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();

    const results = [];
    const byClass = (name) => gallery.created.filter(
        (el) => el.className === name);
    const colHeads = byClass("fight-colhead");
    const rowHeads = byClass("fight-rowhead");
    const canvases = gallery.created.filter(
        (el) => el.tagName === "canvas");

    check(results, "18 variant column heads", colHeads.length === 18,
        "found " + colHeads.length);
    check(results, "4 enemy placement row heads", rowHeads.length === 4,
        "found " + rowHeads.length);
    check(results, "72 canvas cells (18 x 4)", canvases.length === 72,
        "found " + canvases.length);
    check(results, "corner cell present",
        byClass("fight-corner").length === 1,
        "found " + byClass("fight-corner").length);

    const labels = rowHeads.map((head) => head.textContent);
    check(results, "row heads name the four placements",
        labels[0] === "ENEMY\nABOVE" && labels[1] === "ENEMY\nBELOW"
        && labels[2] === "ENEMY\nLEFT" && labels[3] === "ENEMY\nRIGHT",
        "labels: " + JSON.stringify(labels));

    const numbers = colHeads.map((head) =>
        head.children[0].children[0].textContent);
    let numbered = numbers.length === 18;
    for (let i = 0; i < numbers.length; i++) {
        if (numbers[i] !== String(i + 1).padStart(2, "0")) {
            numbered = false;
        }
    }
    check(results, "column heads are numbered 01..18", numbered,
        "numbers: " + JSON.stringify(numbers));

    const template = gallery.ids["fight-grid"].style
        .gridTemplateColumns;
    check(results, "grid template spans the 18 columns",
        template === ROWHEAD_W + "px repeat(18, " + CELL_W + "px)",
        "template: " + template);

    // Every variant must be a comparable object: named, described and
    // drawable.
    let described = true;
    for (const variant of gallery.variants) {
        if (!variant.name || !variant.note
            || typeof variant.draw !== "function") {
            described = false;
        }
    }
    check(results, "every variant has a name, a note and a draw hook",
        described, "one of the 18 variants is incomplete");

    return results;
}

// runScenarioEngagement covers the heart of the brief: every variant
// must be visibly active during all four beats (the hero hit, the
// taken hit, the hero crit, the crit taken) in every placement row.
// The glyph layer is measured against the engine baseline (the same
// clock rendered with a hook-less null variant), the motion hooks
// against their own return values.
function runScenarioEngagement(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();

    // Wrap the motion hooks of every variant so a non null return
    // during a render marks the variant engaged.
    const hookHits = new Set();
    for (let i = 0; i < gallery.variants.length; i++) {
        const variant = gallery.variants[i];
        for (const hook of ["unitOffset", "cellShake", "zoom",
            "freeze"]) {
            if (typeof variant[hook] !== "function") { continue; }
            const original = variant[hook];
            variant[hook] = function (...args) {
                const result = original.apply(this, args);
                if (result) { hookHits.add(i); }
                return result;
            };
        }
    }

    const results = [];
    const cells = gallery.FightTest.cells;
    const savedVariants = cells.map((cell) => cell.variant);

    const renderAt = (clock) => {
        resetStates(gallery);
        gallery.FightTest.clock = clock;
        gallery.FightTest.render();
    };
    const cellsByVariant = (index) => cells.filter(
        (cell) => cell.variantIndex === index);

    for (let beatIndex = 0; beatIndex < gallery.beats.length;
        beatIndex++) {
        const beat = gallery.beats[beatIndex];
        let allEngaged = true;
        const silent = [];
        for (let variantIndex = 0;
            variantIndex < gallery.variants.length; variantIndex++) {
            let engaged = false;
            for (const sample of [60, 180, 300]) {
                // The engine baseline: the same clock with the null
                // variant on every cell.
                for (const cell of cells) { cell.variant = {}; }
                renderAt(beat.at + sample);
                const baseline = cellsByVariant(variantIndex).map(
                    (cell) => opsOf(cell.canvas._state));
                for (const cell of cells) {
                    cell.variant = savedVariants[cell.variantIndex];
                }
                // The real render.
                hookHits.clear();
                renderAt(beat.at + sample);
                const withVariant = cellsByVariant(variantIndex).map(
                    (cell) => opsOf(cell.canvas._state));
                const drew = withVariant.some((ops, row) =>
                    ops > baseline[row]);
                if (drew || hookHits.has(variantIndex)) {
                    engaged = true;
                }
            }
            if (!engaged) {
                allEngaged = false;
                silent.push((variantIndex + 1) + " "
                    + gallery.variants[variantIndex].name);
            }
        }
        const kind = beat.crit
            ? (beat.attacker === "hero" ? "hero crit" : "crit taken")
            : (beat.attacker === "hero" ? "hero hit" : "taken hit");
        check(results, "every variant is active during the " + kind,
            allEngaged, "silent variants: " + silent.join(", "));
    }

    return results;
}

// runScenarioBackdrop covers the map background and the caption: the
// cells paint the starter meadow tile crop and label the beat.
function runScenarioBackdrop(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();

    const results = [];
    resetStates(gallery);
    gallery.FightTest.clock = 900;
    gallery.FightTest.render();

    const canvases = gallery.created.filter(
        (el) => el.tagName === "canvas");
    const rendered = canvases.filter(
        (el) => el._state.clears > 0);
    check(results, "all 72 cells rendered in the full visibility view",
        rendered.length === 72, "rendered " + rendered.length);

    const withTile = rendered.filter(
        (el) => el._state.images === 1 && el._state.imageArgs
        && el._state.imageArgs[0].src === "maps/0/21_19.jpg");
    check(results, "every cell paints the meadow map tile",
        withTile.length === 72, "tiled " + withTile.length);

    // The crop must stay inside the 1024px source tile.
    let cropOk = true;
    for (const el of withTile) {
        const args = el._state.imageArgs;
        const sx = args[1];
        const sy = args[2];
        const sw = args[3];
        const sh = args[4];
        if (sx < 0 || sy < 0 || sx + sw > TILE_PX
            || sy + sh > TILE_PX) {
            cropOk = false;
        }
    }
    check(results, "the tile crop stays inside the source image", cropOk,
        "crop out of bounds");

    const captioned = rendered.filter((el) =>
        el._state.texts.some((text) => text === "HERO HIT 46"));
    check(results, "the beat caption labels the hero hit",
        captioned.length === 72, "captioned " + captioned.length);

    // The caption follows the beats through the whole loop.
    const expect = [
        [3100, "TAKEN 12"],
        [5000, "HERO CRIT! 92"],
        [7000, "CRIT! TAKEN 24"]
    ];
    for (const [clock, text] of expect) {
        resetStates(gallery);
        gallery.FightTest.clock = clock;
        gallery.FightTest.render();
        const found = rendered.some((el) =>
            el._state.texts.some((entry) => entry === text));
        check(results, "the caption reads \"" + text + "\"",
            found, "no caption at clock " + clock);
    }

    return results;
}

// runScenarioTimeline covers the HP integration and the lunge
// geometry: the bars drop exactly by the beat amounts and the
// attacker steps toward the target at the impact moment.
function runScenarioTimeline(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();

    const results = [];
    const FightTest = gallery.FightTest;

    check(results, "hero starts at 100 hp",
        FightTest.hpOf("hero", 0) === 100,
        "hp " + FightTest.hpOf("hero", 0));
    check(results, "enemy starts at 200 hp",
        FightTest.hpOf("enemy", 0) === 200,
        "hp " + FightTest.hpOf("enemy", 0));
    check(results, "hero hp after the taken hit and the taken crit",
        FightTest.hpOf("hero", 7000) === 64,
        "hp " + FightTest.hpOf("hero", 7000));
    check(results, "enemy hp after the dealt hit and the dealt crit",
        FightTest.hpOf("enemy", 5000) === 62,
        "hp " + FightTest.hpOf("enemy", 5000));
    check(results, "hp resets for the next loop",
        FightTest.hpOf("hero", 0) === 100
        && FightTest.hpOf("enemy", 0) === 200,
        "the loop restart did not reset the bars");

    // The lunge: row 0 has the enemy above the hero, so a hero attack
    // moves the hero up and an enemy attack moves the enemy down.
    const row0 = gallery.FightTest.cells[0];
    gallery.FightTest.clock = 800;
    gallery.FightTest.render();
    check(results, "the hero lunges toward the enemy above",
        row0.units.hero.y < row0.hero.y - 10
        && Math.abs(row0.units.hero.x - row0.hero.x) < 0.01,
        "hero y " + row0.units.hero.y + " of " + row0.hero.y);
    check(results, "the target holds its ground during the lunge",
        Math.abs(row0.units.enemy.y - row0.enemy.y) < 0.01
        && Math.abs(row0.units.enemy.x - row0.enemy.x) < 0.01,
        "enemy moved");

    gallery.FightTest.clock = 3000;
    gallery.FightTest.render();
    check(results, "the enemy lunges toward the hero below",
        row0.units.enemy.y > row0.enemy.y + 10,
        "enemy y " + row0.units.enemy.y + " of " + row0.enemy.y);

    // Off the lunge window the units sit at their bases again (past
    // the third beat return phase, before the fourth beat windup).
    gallery.FightTest.clock = 5600;
    gallery.FightTest.render();
    const row1 = gallery.FightTest.cells[1 * 18];
    check(results, "units rest at their bases between the beats",
        Math.abs(row1.units.hero.y - row1.hero.y) < 0.01,
        "hero y " + row1.units.hero.y + " of " + row1.hero.y);

    return results;
}

// runScenarioScroll covers the horizontal scroll window: only the
// columns inside the strip viewport repaint.
function runScenarioScroll(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();
    const scroll = gallery.ids["fight-scroll"];

    const results = [];
    scroll.clientWidth = 900;
    scroll.scrollLeft = 0;
    resetStates(gallery);
    gallery.FightTest.clock = 1200;
    gallery.FightTest.render();

    const cells = gallery.FightTest.cells;
    const visible = cells.filter((cell) => {
        const x0 = ROWHEAD_W + cell.col * CELL_W;
        return x0 <= 980;
    });
    const hidden = cells.filter((cell) => visible.indexOf(cell) < 0);
    check(results, "the in window columns repaint",
        visible.length > 0 && visible.every(
            (cell) => cell.canvas._state.clears > 0),
        "a visible column did not repaint");
    check(results, "the off window columns are skipped",
        hidden.length > 0 && hidden.every(
            (cell) => cell.canvas._state.clears === 0),
        "hidden columns: " + hidden.length + ", some repainted");

    // Scrolling right brings the far columns back to life.
    scroll.scrollLeft = 112 + 17 * CELL_W - 100;
    resetStates(gallery);
    gallery.FightTest.render();
    const lastCol = cells.filter((cell) => cell.col === 17);
    check(results, "scrolling right repaints the last column",
        lastCol.every((cell) => cell.canvas._state.clears > 0),
        "the last column stayed blank");

    return results;
}

// runScenarioControls covers the gallery controls: the pause button
// freezes the clock, the speed select retimes it, the rAF loop
// advances it and the column pick toggles the highlight.
function runScenarioControls(fightFile) {
    const gallery = loadFightJs(fightFile);
    gallery.FightTest.init();

    const results = [];
    const FightTest = gallery.FightTest;
    const pause = gallery.ids["fight-pause"];
    const speed = gallery.ids["fight-speed"];

    // The rAF loop advances the virtual clock.
    const firstFrame = gallery.rafCallbacks.pop();
    firstFrame(16.7);
    const clockAfterFrame = FightTest.clock;
    check(results, "one frame advances the clock by its dt",
        Math.abs(clockAfterFrame - 16.7) < 0.01,
        "clock " + clockAfterFrame);

    // The pause freezes it.
    pause.fire("click");
    const secondFrame = gallery.rafCallbacks.pop();
    secondFrame(33.4);
    check(results, "the pause button freezes the clock",
        FightTest.clock === clockAfterFrame,
        "clock moved to " + FightTest.clock);
    check(results, "the pause button relabels itself to resume",
        pause.textContent === "resume",
        "label " + pause.textContent);
    pause.fire("click");
    check(results, "the resume continues the clock",
        pause.textContent === "pause", "label " + pause.textContent);

    // The speed select retimes the loop.
    speed.value = "2";
    speed.fire("change");
    check(results, "the speed select sets the playback speed",
        FightTest.speed === 2, "speed " + FightTest.speed);

    // The column pick toggles the highlight of the whole column.
    const colHead = gallery.created.filter(
        (el) => el.className === "fight-colhead")[3];
    colHead.fire("click");
    const pickedCells = FightTest.cells.filter(
        (cell) => cell.variantIndex === 3);
    check(results, "clicking a head marks the variant column",
        colHead.classList.contains("picked")
        && pickedCells.every(
            (cell) => cell.canvas.classList.contains("picked")),
        "the pick did not spread to the cells");
    colHead.fire("click");
    check(results, "clicking again clears the mark",
        !colHead.classList.contains("picked")
        && pickedCells.every(
            (cell) => !cell.canvas.classList.contains("picked")),
        "the pick did not clear");

    return results;
}

function main() {
    const args = process.argv.slice(2);
    const verbose = args.includes("--verbose");
    const fightIndex = args.indexOf("--fight");
    const fightFile = fightIndex >= 0 && args[fightIndex + 1]
        ? args[fightIndex + 1] : FIGHT_JS;
    if (!fs.existsSync(fightFile)) {
        console.error("fighttest.js not found: " + fightFile);
        process.exit(1);
    }

    const scenarios = [
        ["gallery structure", runScenarioStructure(fightFile)],
        ["variant engagement", runScenarioEngagement(fightFile)],
        ["map backdrop and captions", runScenarioBackdrop(fightFile)],
        ["beat timeline", runScenarioTimeline(fightFile)],
        ["scroll window", runScenarioScroll(fightFile)],
        ["gallery controls", runScenarioControls(fightFile)]
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
