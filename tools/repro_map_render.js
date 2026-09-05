#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the web map target links and unit markers.
//
// It loads the real internal/swarm/webserver/web/map.js into a sandboxed
// context with a recording canvas, feeds it snapshots and checks the
// drawing geometry:
//
// - player target links: every visible player that selected something
//   (TargetSelected 0x39 reaches the tracker as targetId) must be drawn
//   as a violet line to the target plus a violet ring around the
//   target, including the case where the target is the bot itself (the
//   reported bug: logging in with a second character and selecting the
//   bot or a mob showed nothing on the map);
// - unit markers: the look direction tick must start at the circle
//   edge and stay outside the circle (the reported bug: the tick was
//   drawn from inside the circle, so the fill looked split);
// - regression guard: the own target link of the bot still renders.
//
// Usage: node tools/repro_map_render.js [--map <map.js>] [--verbose]
// Exit code 0 = rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const MAP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "map.js");

// The light theme palette of style.css so recorded stroke styles are
// distinguishable.
const THEME = {
    "--red": "#cf222e",
    "--violet": "#8250df",
    "--blue": "#0969da",
    "--accent": "#d97706",
    "--green": "#1a7f37",
    "--gold": "#9a6700",
    "--gray": "#6e7781",
    "--text": "#2a3140",
    "--text-bright": "#111722",
    "--text-dim": "#67707e",
    "--border": "#d5dbe3",
    "--grid": "rgba(21, 34, 50, 0.10)",
    "--grid-text": "rgba(60, 72, 90, 0.55)"
};

// canvas geometry of the harness
const CANVAS_W = 800;
const CANVAS_H = 600;

// world layout: the self marker in the center, a player to the north
// east, the mob it selects closer to the center, the own target to the
// south east.
const WORLD = {
    scale: 0.12,
    self: { objectId: 100, x: 45000, y: 50000, name: "test1" },
    player: { objectId: 200, x: 45600, y: 50600, name: "second" },
    playerTargetMob: { objectId: 300, x: 45150, y: 50150, name: "Keltir" },
    ownTargetMob: { objectId: 400, x: 45300, y: 49700, name: "Gremlin" }
};

function worldToScreen(wx, wy) {
    return {
        x: CANVAS_W / 2 + (wx - WORLD.self.x) * WORLD.scale,
        y: CANVAS_H / 2 + (wy - WORLD.self.y) * WORLD.scale
    };
}

// RecordingContext captures the path geometry of every stroke with the
// style active at stroke time.
function makeRecordingContext(record) {
    let current = null;
    return {
        canvas: { width: CANVAS_W, height: CANVAS_H },
        clearRect: () => {},
        setTransform: () => {},
        save: () => {},
        restore: () => {},
        beginPath: () => { current = { segments: [], arcs: [] }; },
        moveTo: (x, y) => { current.segments.push([x, y]); },
        lineTo: (x, y) => {
            if (current.segments.length === 0) { return; }
            const start = current.segments[current.segments.length - 1];
            current.segments[current.segments.length - 1] =
                [start[0], start[1], x, y];
        },
        arc: (x, y, r) => {
            if (current) { current.arcs.push([x, y, r]); }
        },
        stroke: () => {
            if (current) {
                record.strokes.push({
                    segments: current.segments.slice(),
                    arcs: current.arcs.slice(),
                    style: record.style,
                    width: record.width
                });
            }
            current = null;
        },
        fill: () => {
            if (current) {
                record.fills.push({
                    segments: current.segments.slice(),
                    arcs: current.arcs.slice(),
                    style: record.fillStyle
                });
            }
            current = null;
        },
        fillRect: () => {},
        fillText: () => {},
        setLineDash: (dash) => { record.dash = dash.slice(); },
        // style properties tracked through the record object
        get strokeStyle() { return record.style; },
        set strokeStyle(v) { record.style = v; },
        get fillStyle() { return record.fillStyle; },
        set fillStyle(v) { record.fillStyle = v; },
        get lineWidth() { return record.width; },
        set lineWidth(v) { record.width = v; },
        get globalAlpha() { return 1; },
        set globalAlpha(v) {},
        get lineCap() { return "butt"; },
        set lineCap(v) {},
        get font() { return ""; },
        set font(v) {},
        get textAlign() { return "left"; },
        set textAlign(v) {}
    };
}

function makeElementStub(checked) {
    return {
        classList: { contains: () => true, add: () => {}, remove: () => {} },
        addEventListener: () => {},
        style: {},
        textContent: "",
        checked: checked === undefined ? true : checked,
        getBoundingClientRect: () => ({
            width: CANVAS_W, height: CANVAS_H, left: 0, top: 0
        })
    };
}

// loadMapJs creates a fresh sandboxed context and returns MapView
// together with the stroke record.
function loadMapJs(mapFile) {
    const record = {
        strokes: [], fills: [], style: "", fillStyle: "", width: 1,
        dash: []
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
        }
    };
    const checkboxes = {
        follow: true, "show-labels": false, "show-dest": false,
        "show-zone": false, "show-targets": true
    };
    const elements = new Map();
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
        window: { addEventListener: () => {} },
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

    return { MapView: sandbox.__MapView, record, elements };
}

// buildSnapshot assembles a snapshot view with the given target links.
function buildSnapshot(playerTarget, selfTarget) {
    return {
        serverTimeMs: 0,
        character: {
            objectId: WORLD.self.objectId,
            name: WORLD.self.name,
            x: WORLD.self.x, y: WORLD.self.y, z: -3500,
            heading: 0,
            targetId: selfTarget ? WORLD.ownTargetMob.objectId : 0,
            moving: false
        },
        objects: [
            {
                objectId: WORLD.player.objectId, kind: "player",
                name: WORLD.player.name,
                x: WORLD.player.x, y: WORLD.player.y, z: -3500,
                heading: 0, moving: false, speed: 0, targetId: playerTarget,
                dead: false, attackable: false, aggressive: false,
                inCombat: false, level: 0
            },
            {
                objectId: WORLD.playerTargetMob.objectId, kind: "npc",
                name: WORLD.playerTargetMob.name,
                x: WORLD.playerTargetMob.x, y: WORLD.playerTargetMob.y,
                z: -3500, heading: 0, moving: false, speed: 0, targetId: 0,
                dead: false, attackable: true, aggressive: false,
                inCombat: false, level: 2
            },
            {
                objectId: WORLD.ownTargetMob.objectId, kind: "npc",
                name: WORLD.ownTargetMob.name,
                x: WORLD.ownTargetMob.x, y: WORLD.ownTargetMob.y,
                z: -3500, heading: 0, moving: false, speed: 0, targetId: 0,
                dead: false, attackable: true, aggressive: false,
                inCombat: false, level: 2
            }
        ]
    };
}

// findSegment returns the strokes whose single segment starts and ends
// near the given points (within tolerance).
function findSegment(record, from, to, style) {
    return record.strokes.filter((stroke) => {
        if (style && stroke.style !== style) { return false; }
        if (stroke.segments.length !== 1) { return false; }
        const seg = stroke.segments[0];

        return Math.hypot(seg[0] - from.x, seg[1] - from.y) < 2
            && Math.hypot(seg[2] - to.x, seg[3] - to.y) < 2;
    });
}

// findArc returns the strokes with an arc near the center and radius.
function findArc(record, center, radius, style) {
    return record.strokes.filter((stroke) => {
        if (style && stroke.style !== style) { return false; }
        if (stroke.arcs.length !== 1) { return false; }
        const arc = stroke.arcs[0];

        return Math.hypot(arc[0] - center.x, arc[1] - center.y) < 2
            && Math.abs(arc[2] - radius) < 2;
    });
}

// check verifies one property and collects the results.
function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

// The marker radius of the map (radiusOf): player 5.5, passive npc 5,
// combat 6, self 7.
function markerRadius(kind) {
    return kind === "player" ? 5.5 : kind === "self" ? 7 : 5;
}

function runScenario(mapFile, verbose) {
    const { MapView, record } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(WORLD.playerTargetMob.objectId, true));

    const results = [];
    const player = worldToScreen(WORLD.player.x, WORLD.player.y);
    const mob = worldToScreen(WORLD.playerTargetMob.x,
        WORLD.playerTargetMob.y);
    const ownTarget = worldToScreen(WORLD.ownTargetMob.x,
        WORLD.ownTargetMob.y);
    const self = worldToScreen(WORLD.self.x, WORLD.self.y);

    // Bug 1: the player target link is drawn from the player to the
    // mob it selected, in the player color.
    check(results, "player target link drawn to the selected mob",
        findSegment(record, player, mob, THEME["--violet"]).length > 0,
        "expected a violet segment " + JSON.stringify(player) + " -> "
        + JSON.stringify(mob));

    // Bug 1: the claimed mob gets a violet ring.
    const claimedRadius = markerRadius("npc") + 5;
    check(results, "violet ring around the mob claimed by the player",
        findArc(record, mob, claimedRadius, THEME["--violet"]).length > 0,
        "expected a violet arc at " + JSON.stringify(mob)
        + " r=" + claimedRadius);

    // Bug 1: the own target link (regression guard) stays red.
    check(results, "own target link still drawn",
        findSegment(record, self, ownTarget, THEME["--red"]).length > 0,
        "expected a red segment " + JSON.stringify(self) + " -> "
        + JSON.stringify(ownTarget));

    // Bug 3: every direction tick starts at its unit circle edge: the
    // distance between the tick start and the unit center must be the
    // marker radius (never inside the circle).
    const units = [
        { center: self, radius: markerRadius("self") },
        { center: player, radius: markerRadius("player") },
        { center: mob, radius: markerRadius("npc") },
        { center: ownTarget, radius: markerRadius("npc") }
    ];
    for (const unit of units) {
        // the tick of a unit: a short segment (under 8 px) whose start
        // is within radius+3 of the unit center.
        const ticks = record.strokes.filter((stroke) => {
            if (stroke.width !== 2) { return false; }

            return stroke.segments.some((seg) =>
                Math.hypot(seg[0] - unit.center.x, seg[1] - unit.center.y)
                    < unit.radius + 8
                && Math.hypot(seg[2] - seg[0], seg[3] - seg[1]) < 8);
        });
        const tickStarts = [];
        for (const stroke of ticks) {
            for (const seg of stroke.segments) {
                const dStart = Math.hypot(
                    seg[0] - unit.center.x, seg[1] - unit.center.y);
                if (dStart < unit.radius + 8
                    && Math.hypot(seg[2] - seg[0], seg[3] - seg[1]) < 8) {
                    tickStarts.push(dStart);
                }
            }
        }
        check(results,
            "direction tick starts at the circle edge of "
            + JSON.stringify(unit.center),
            tickStarts.length > 0
            && tickStarts.every((d) => d >= unit.radius - 0.75),
            "tick starts at " + tickStarts.join(", ")
            + " but the marker radius is " + unit.radius);
    }

    if (verbose) {
        for (const stroke of record.strokes) {
            console.log("stroke", JSON.stringify(stroke));
        }
    }

    return results;
}

// runScenarioTargetingBot covers the case of another player selecting
// the bot itself: the violet ring lands around the self marker.
function runScenarioTargetingBot(mapFile) {
    const { MapView, record } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(WORLD.self.objectId, false));

    const results = [];
    const player = worldToScreen(WORLD.player.x, WORLD.player.y);
    const self = worldToScreen(WORLD.self.x, WORLD.self.y);

    check(results, "player target link drawn to the bot itself",
        findSegment(record, player, self, THEME["--violet"]).length > 0,
        "expected a violet segment " + JSON.stringify(player) + " -> "
        + JSON.stringify(self));
    const selfRing = markerRadius("self") + 5;
    check(results, "violet ring around the bot targeted by the player",
        findArc(record, self, selfRing, THEME["--violet"]).length > 0,
        "expected a violet arc at " + JSON.stringify(self)
        + " r=" + selfRing);

    return results;
}

// runScenarioFollowOff covers the follow checkbox: with follow off the
// camera stays pinned to the chosen area — a fixed world point keeps its
// screen position while the bot walks away — and with follow on the view
// tracks the character again (the reported bug: unchecking follow still
// moved the map with the character).
function runScenarioFollowOff(mapFile) {
    const { MapView, elements } = loadMapJs(mapFile);
    if (typeof MapView.syncPanAnchor !== "function") {
        console.log("FAIL  syncPanAnchor is missing from map.js");
        process.exit(1);
    }
    MapView.init();
    MapView.update(buildSnapshot(0, false));

    const results = [];
    const pin = { x: WORLD.self.x, y: WORLD.self.y };
    elements.get("follow").checked = false;
    MapView.syncPanAnchor();
    MapView.draw();
    const before = MapView.worldToScreen(pin.x, pin.y);

    // The character moves 300 units east with follow off: the view must
    // not shift, the world point stays at its screen position.
    const moved = buildSnapshot(0, false);
    moved.character.x = WORLD.self.x + 300;
    MapView.update(moved);
    const after = MapView.worldToScreen(pin.x, pin.y);
    check(results, "follow off keeps the map static while the bot moves",
        Math.abs(after.x - before.x) < 2 && Math.abs(after.y - before.y) < 2,
        "fixed point moved from " + JSON.stringify(before) + " to "
        + JSON.stringify(after));

    // Follow on: the same world point shifts because the camera tracks
    // the character (300 units at scale 0.12 = 36 px).
    elements.get("follow").checked = true;
    MapView.draw();
    const followed = MapView.worldToScreen(pin.x, pin.y);
    check(results, "follow on tracks the moving bot",
        Math.abs(followed.x - (before.x - 36)) < 2,
        "expected x ≈ " + (before.x - 36) + ", got "
        + JSON.stringify(followed));

    return results;
}

function main() {
    const args = process.argv.slice(2);
    const verbose = args.includes("--verbose");
    const mapIndex = args.indexOf("--map");
    const mapFile = mapIndex >= 0 ? args[mapIndex + 1] : MAP_JS;
    if (!fs.existsSync(mapFile)) {
        console.error("map.js not found: " + mapFile);
        process.exit(1);
    }

    const scenarios = [
        ["map targets", runScenario(mapFile, verbose)],
        ["player selects the bot", runScenarioTargetingBot(mapFile)],
        ["follow off keeps the view static", runScenarioFollowOff(mapFile)]
    ];
    let failed = 0;
    for (const [name, results] of scenarios) {
        console.log("scenario: " + name);
        for (const result of results) {
            const mark = result.ok ? "PASS" : "FAIL";
            console.log("  " + mark + "  " + result.name);
            if (!result.ok) {
                failed++;
                console.log("        " + result.detail);
            }
        }
    }
    console.log(failed === 0 ? "ALL PASS" : failed + " CHECKS FAILED");

    process.exit(failed === 0 ? 0 : 1);
}

main();
