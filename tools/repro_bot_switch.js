#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the bot switch state of the web map.
//
// It loads the real internal/swarm/webserver/web/map.js into a
// sandboxed context with a recording canvas and checks the reset the
// app performs when the observed bot changes (selectBot ->
// MapView.resetBot):
//
// - the map state of the previous bot (its snapshot, its runtime
//   object cache, its social masks, its combat effects) drops on the
//   switch, so a redraw paints no stale zone circles, no stale unit
//   markers and no stale walk line while the new event stream
//   connects (the reported bug: the previous bot's map lingered under
//   the new bot's HUD, and a fresh bot with no published zones read
//   as "the circles are missing" next to it);
// - the fleet kill marks survive the reset (they belong to the whole
//   deployment, not to one bot);
// - the first snapshot of the new bot repaints normally.
//
// Usage: node tools/repro_bot_switch.js [--map <map.js>] [--verbose]
// Exit code 0 = the switch resets cleanly, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const MAP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "map.js");

// The theme palette of style.css (the ui chrome reads it).
const THEME = {
    "--text": "#2a3140",
    "--text-bright": "#111722",
    "--text-dim": "#67707e",
    "--border": "#d5dbe3",
    "--grid": "rgba(21, 34, 50, 0.10)",
    "--grid-text": "rgba(60, 72, 90, 0.55)"
};

const CANVAS_W = 800;
const CANVAS_H = 600;

const WORLD = {
    scale: 0.12,
    self: { objectId: 100, x: 45000, y: 50000, name: "test1" },
    other: { objectId: 200, x: 46000, y: 51000, name: "test2" }
};

function worldToScreen(wx, wy) {
    return {
        x: CANVAS_W / 2 + (wx - WORLD.self.x) * WORLD.scale,
        y: CANVAS_H / 2 + (wy - WORLD.self.y) * WORLD.scale
    };
}

// RecordingContext captures the arcs and texts of every draw.
function makeRecordingContext(record) {
    return {
        canvas: { width: CANVAS_W, height: CANVAS_H },
        clearRect: () => {},
        setTransform: () => {},
        save: () => {},
        restore: () => {},
        beginPath: () => {},
        closePath: () => {},
        moveTo: () => {},
        lineTo: () => {},
        arc: (x, y, r) => { record.arcs.push([x, y, r]); },
        ellipse: () => {},
        fill: () => { record.fills++; },
        stroke: () => { record.strokes++; },
        fillText: (text, x, y) => {
            record.texts.push({ text, x, y });
        },
        strokeText: () => {},
        measureText: () => ({ width: 6 }),
        set fillStyle(v) { record.fillStyle = v; },
        get fillStyle() { return record.fillStyle; },
        set strokeStyle(v) { record.strokeStyle = v; },
        get strokeStyle() { return record.strokeStyle; },
        set lineWidth(v) { record.width = v; },
        get lineWidth() { return record.width; },
        set font(v) { record.font = v; },
        get font() { return record.font; },
        set textAlign(v) { record.align = v; },
        get textAlign() { return record.align; },
        set textBaseline(v) { record.baseline = v; },
        get textBaseline() { return record.baseline; },
        set globalAlpha(v) { record.alpha = v; },
        get globalAlpha() { return record.alpha; },
        set lineDashOffset(v) {},
        get lineDashOffset() { return 0; },
        setLineDash: () => {},
        strokeRect: () => { record.strokes++; },
        fillRect: () => { record.fills++; },
        translate: () => {},
        rotate: () => {},
        scale: () => {},
        quadraticCurveTo: () => {},
        bezierCurveTo: () => {},
        rect: () => {},
        clip: () => {}
    };
}

function makeElementStub(checked) {
    return {
        checked,
        addEventListener: () => {},
        classList: {
            add: () => {}, remove: () => {}, toggle: () => {},
            contains: () => false
        },
        style: {},
        textContent: "",
        setAttribute: () => {},
        getBoundingClientRect: () => ({ width: 0, height: 0 })
    };
}

function loadMapJs(mapFile) {
    const record = {
        arcs: [], texts: [], fills: 0, strokes: 0,
        fillStyle: "", strokeStyle: "", width: 1
    };
    const canvas = {
        addEventListener: () => {},
        getContext: () => makeRecordingContext(record),
        getBoundingClientRect: () => ({
            width: CANVAS_W, height: CANVAS_H, left: 0, top: 0
        }),
        parentElement: {
            getBoundingClientRect: () => ({
                width: CANVAS_W, height: CANVAS_H, left: 0, top: 0
            })
        },
        style: {}
    };
    const elements = new Map();
    // The layer checkboxes the map reads: the world layers on (the
    // zones and the units are the subject of this harness).
    const checkboxes = {
        follow: true, "show-labels": false, "show-dest": true,
        "show-zone": true, "show-targets": true,
        "show-hunt-zones": true, "show-aggro": true
    };
    const sandbox = {
        Math, JSON,
        Date: { now: () => 0 },
        performance: { now: () => 0 },
        requestAnimationFrame: () => 0,
        document: {
            getElementById: (id) => {
                if (id === "map-canvas") { return canvas; }
                if (!elements.has(id)) {
                    const checked = Object.prototype.hasOwnProperty.call(
                        checkboxes, id) ? checkboxes[id] : false;
                    elements.set(id, makeElementStub(checked));
                }

                return elements.get(id);
            }
        },
        window: { addEventListener: () => {}, devicePixelRatio: 1 },
        getComputedStyle: () => ({
            getPropertyValue: (name) => THEME[name] || ""
        }),
        setTimeout: () => 0,
        clearTimeout: () => {}
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(mapFile, "utf8"), sandbox,
        { filename: "map.js" });
    vm.runInContext("globalThis.__MapView = MapView;", sandbox);

    return { MapView: sandbox.__MapView, record };
}

// buildSnapshot assembles a snapshot of the given bot with an active
// hunting spot circle around its position.
function buildSnapshot(bot) {
    return {
        serverTimeMs: 0,
        character: {
            objectId: bot.objectId, name: bot.name,
            x: bot.x, y: bot.y, z: -3500, heading: 0,
            targetId: 0, moving: false
        },
        objects: [{
            objectId: bot.objectId + 1, kind: "npc", name: "Keltir",
            x: bot.x + 200, y: bot.y - 150, z: -3500, heading: 0,
            moving: false, speed: 0, targetId: 0, dead: false,
            attackable: true, aggressive: false, inCombat: false,
            level: 2, clanMask: "0"
        }],
        huntingZones: [{
            id: bot.name, name: bot.name + " Ground", region: "elven",
            minLevel: 1, maxLevel: 5, minGear: 0,
            kind: "spot", radius: 500,
            cx: bot.x, cy: bot.y, half: 500,
            active: true, demoted: false, deaths: 0
        }],
        walkPath: [{
            x: bot.x + 400, y: bot.y + 400, z: -3500
        }]
    };
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

function runScenario(mapFile) {
    const { MapView, record } = loadMapJs(mapFile);
    MapView.init();
    MapView.setKillMarks([]);
    MapView.update(buildSnapshot(WORLD.self));
    MapView.draw();

    const results = [];
    const spotRadius = 500 * WORLD.scale;
    const zoneArcs = record.arcs.filter(
        (arc) => Math.abs(arc[2] - spotRadius) < 2).length;
    check(results, "the observed bot draws its spot circle",
        zoneArcs > 0, "no spot circle of radius " + spotRadius);

    // The bot switch: resetBot must exist and drop the whole observed
    // state (the app calls it from selectBot).
    check(results, "MapView exposes resetBot for the bot switch",
        typeof MapView.resetBot === "function",
        "resetBot is missing from map.js");
    if (typeof MapView.resetBot !== "function") {
        return results;
    }
    MapView.setKillMarks([{ x: 45000, y: 50000, atMs: 0 }]);
    MapView.resetBot();

    check(results, "the reset drops the previous snapshot",
        MapView.lastSnap === null,
        "lastSnap survived the reset");
    check(results, "the reset drops the runtime object cache",
        MapView.runtime.size === 0,
        MapView.runtime.size + " runtime entries survived");
    check(results, "the reset drops the social mask cache",
        MapView.socialMasks.size === 0,
        MapView.socialMasks.size + " masks survived");
    check(results, "the reset drops the combat effects",
        MapView.combatAnims.length === 0,
        MapView.combatAnims.length + " effects survived");
    check(results, "the fleet kill marks survive the reset",
        MapView.killMarks.length === 1,
        "the fleet layer must stay across the switches");

    // The redraw after the reset paints nothing of the old bot.
    record.arcs.length = 0;
    record.texts.length = 0;
    record.fills = 0;
    MapView.draw();
    check(results, "the reset map paints no stale spot circles",
        record.arcs.filter(
            (arc) => Math.abs(arc[2] - spotRadius) < 2).length === 0,
        "a spot circle is still drawn");
    check(results, "the reset map paints no stale unit markers",
        record.fills === 0, record.fills + " unit fills still drawn");

    // The first snapshot of the new bot repaints normally: with follow
    // on the camera centers on the new character, its own spot circle
    // appears around the middle of the canvas.
    MapView.update(buildSnapshot(WORLD.other));
    const otherSpot = { x: CANVAS_W / 2, y: CANVAS_H / 2 };
    const otherArcs = record.arcs.filter((arc) =>
        Math.abs(arc[2] - spotRadius) < 2
        && Math.hypot(arc[0] - otherSpot.x, arc[1] - otherSpot.y) < 3);
    check(results, "the new bot repaints its own spot circle",
        otherArcs.length > 0,
        "no spot circle at " + JSON.stringify(otherSpot));

    return results;
}

function main() {
    const args = process.argv.slice(2);
    const mapArg = args.indexOf("--map");
    const mapFile = mapArg >= 0 ? args[mapArg + 1] : MAP_JS;
    const verbose = args.includes("--verbose");

    const results = runScenario(mapFile);
    let failed = 0;
    for (const result of results) {
        if (!result.ok) { failed++; }
        console.log((result.ok ? "PASS  " : "FAIL  ") + result.name
            + (result.ok || !result.detail ? "" : " - " + result.detail));
    }
    if (verbose) {
        console.log("arcs", JSON.stringify(require("node:util")
            .inspect(runScenario)));
    }
    if (failed > 0) {
        console.log("bot switch reproduction: FAIL (" + failed + ")");
        process.exit(1);
    }
    console.log("bot switch reproduction: PASS (the map resets cleanly)");
}

main();
