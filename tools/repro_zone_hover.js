#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the hover, social and fleet kill layers of
// the web map.
//
// It loads the real internal/swarm/webserver/web/map.js into a
// sandboxed context with a recording canvas and checks:
//
// - zone list focus: focusZone pins the camera on the ground, scales
//   it to fit and lights the zone label; blurZone restores the camera
//   the user had (the follow flag, the pan anchor, the zoom);
// - fleet kill crosses: setKillMarks draws the crosses of every bot
//   (the layer lives in the map, not the observed bot's snapshot),
//   the old marks melt away, the checkbox hides the layer;
// - social links: two npcs of the same clan inside their clan help
//   range connect with the solid teal line, a pair that only
//   approaches the range with the dashed amber warning line, npcs of
//   different clans never link, the ALL clan links with everyone;
// - the pyramid ancestor fallback: a still loading fine tile yields
//   to its loaded coarse ancestor, so a panning camera never flashes
//   the blank background.
//
// Usage: node tools/repro_zone_hover.js [--map <map.js>] [--verbose]
// Exit code 0 = rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const MAP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "map.js");

const THEME = {
    "--sans": "sans-serif",
    "--grid": "rgba(21, 34, 50, 0.10)",
    "--grid-text": "rgba(60, 72, 90, 0.55)",
    "--text": "#2a3140",
    "--text-bright": "#111722",
    "--text-dim": "#67707e",
    "--border": "#d5dbe3"
};

const CANVAS_W = 800;
const CANVAS_H = 600;
const SCALE = 0.12;

// The world of the scenario: the character sits at the origin of the
// view, two orc clan mobs stand close enough to link, a lone keltir
// and a wolf pair spread the map.
const WORLD = {
    self: { objectId: 100, x: 45000, y: 50000, name: "test1" },
    orcA: { objectId: 201, x: 45200, y: 50200 },
    orcB: { objectId: 202, x: 45400, y: 50300 },
    orcFar: { objectId: 203, x: 46400, y: 51300 },
    allClan: { objectId: 204, x: 45550, y: 50150 },
    keltir: { objectId: 301, x: 45800, y: 50800 }
};

// The orc clan mask (bit 0 of the alphabet) and the ALL mask.
const ORC_MASK = "1";
const ALL_MASK = "9223372036854775808";

function worldToScreen(wx, wy) {
    return {
        x: CANVAS_W / 2 + (wx - WORLD.self.x) * SCALE,
        y: CANVAS_H / 2 + (wy - WORLD.self.y) * SCALE
    };
}

function makeRecordingContext(record) {
    let current = null;
    let pending = null;
    return {
        canvas: { width: CANVAS_W, height: CANVAS_H },
        clearRect: () => {},
        setTransform: () => {},
        save: () => {},
        restore: () => {},
        beginPath: () => { current = { segments: [], arcs: [] }; },
        closePath: () => {},
        moveTo: (x, y) => { pending = [x, y]; },
        lineTo: (x, y) => {
            if (current && pending) {
                current.segments.push([pending[0], pending[1], x, y]);
                pending = [x, y];
            }
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
                    width: record.width,
                    dash: record.dash.slice()
                });
            }
            current = null;
        },
        fill: () => { current = null; },
        fillRect: () => {},
        drawImage: () => {},
        fillText: (text, x, y) => {
            record.texts.push({ text, x, y, style: record.fillStyle });
        },
        strokeText: (text, x, y) => {
            record.texts.push({ text, x, y, style: record.strokeStyle });
        },
        measureText: (text) => ({ width: (text || "").length * 6 }),
        setLineDash: (dash) => { record.dash = dash.slice(); },
        get strokeStyle() { return record.style; },
        set strokeStyle(v) { record.style = v; },
        get fillStyle() { return record.fillStyle; },
        set fillStyle(v) { record.fillStyle = v; },
        get lineWidth() { return record.width; },
        set lineWidth(v) { record.width = v; },
        get globalAlpha() { return record.alpha; },
        set globalAlpha(v) { record.alpha = v; },
        get lineCap() { return "butt"; },
        set lineCap(v) {},
        get font() { return ""; },
        set font(v) {},
        get textAlign() { return "left"; },
        set textAlign(v) {},
        get imageSmoothingEnabled() { return true; },
        set imageSmoothingEnabled(v) {},
        get imageSmoothingQuality() { return "high"; },
        set imageSmoothingQuality(v) {}
    };
}

function makeElementStub(checked) {
    return {
        classList: { contains: () => true, add: () => {}, remove: () => {} },
        addEventListener: () => {},
        appendChild: () => {},
        append: () => {},
        style: {},
        textContent: "",
        innerHTML: "",
        title: "",
        checked: checked === undefined ? false : checked,
        clientWidth: CANVAS_W,
        clientHeight: CANVAS_H,
        getBoundingClientRect: () => ({
            width: CANVAS_W, height: CANVAS_H, left: 0, top: 0
        })
    };
}

// loadMapJs creates a fresh sandboxed context and returns MapView
// together with the stroke record. BigInt must ride the sandbox: the
// clan mask parsing of the social layer needs it.
function loadMapJs(mapFile) {
    const record = {
        strokes: [], texts: [], style: "", fillStyle: "",
        width: 1, alpha: 1, dash: []
    };
    const listeners = { canvas: {}, window: {} };
    const listen = (registry) => (type, fn) => {
        (registry[type] = registry[type] || []).push(fn);
    };
    const fire = (registry, type, event) => {
        for (const fn of registry[type] || []) { fn(event); }
    };
    const canvas = {
        addEventListener: listen(listeners.canvas),
        getContext: () => makeRecordingContext(record),
        clientWidth: CANVAS_W,
        clientHeight: CANVAS_H,
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
        "show-zone": false, "show-targets": false,
        "show-hunt-zones": true, "show-aggro": false,
        "show-social": true, "show-kills": true
    };
    const elements = new Map();
    const sandbox = {
        Math, JSON, BigInt,
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
            },
            // The tooltip of an object hit builds its rows through
            // createElement; a plain stub keeps the DOM work inert.
            createElement: () => makeElementStub(false)
        },
        window: { addEventListener: listen(listeners.window) },
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

    return {
        MapView: sandbox.__MapView, record, elements,
        fireCanvas: (type, event) => fire(listeners.canvas, type, event),
        fireWindow: (type, event) => fire(listeners.window, type, event)
    };
}

// buildSnapshot assembles the scenario snapshot: two spot zones, the
// orc clan group with one ALL clan neighbor, a lone keltir.
function buildSnapshot() {
    const npc = (spec, clanMask, clanHelp) => ({
        objectId: spec.objectId, kind: "npc",
        name: spec.name || "mob" + spec.objectId,
        x: spec.x, y: spec.y, z: -3500,
        heading: 0, moving: false, speed: 0, targetId: 0,
        dead: false, attackable: true, aggressive: false,
        inCombat: false, level: 4,
        aggroRange: 0,
        clanMask: clanMask, clanHelpRange: clanHelp
    });
    return {
        serverTimeMs: 0,
        character: {
            objectId: WORLD.self.objectId,
            name: WORLD.self.name,
            x: WORLD.self.x, y: WORLD.self.y, z: -3500,
            heading: 0, targetId: 0, moving: false
        },
        huntingZones: [
            {
                id: "s1", kind: "spot", name: "Wolf Meadow",
                region: "elven", minLevel: 1, maxLevel: 4, minGear: 0,
                cx: 45000, cy: 50000, half: 700, radius: 1000,
                active: true, demoted: false, deaths: 0,
                respawnMinSec: 15, respawnMaxSec: 20,
                adenaPerMin: 0, deathHeat: 0,
                nextRespawnSec: -1, occupancy: 1
            },
            {
                id: "s2", kind: "spot", name: "Deep Forest",
                region: "elven", minLevel: 5, maxLevel: 8, minGear: 0,
                cx: 48000, cy: 53000, half: 600, radius: 900,
                active: false, demoted: false, deaths: 0,
                respawnMinSec: 15, respawnMaxSec: 20,
                adenaPerMin: 0, deathHeat: 0,
                nextRespawnSec: -1, occupancy: 0
            }
        ],
        objects: [
            npc(WORLD.orcA, ORC_MASK, 300),
            npc(WORLD.orcB, ORC_MASK, 300),
            npc(WORLD.orcFar, ORC_MASK, 300),
            npc(WORLD.allClan, ALL_MASK, 300),
            npc(WORLD.keltir, "", 0)
        ]
    };
}

// findSegment returns the strokes whose single segment runs between
// the two screen points (within tolerance).
function findSegment(record, from, to, style) {
    return record.strokes.filter((stroke) => {
        if (style && stroke.style !== style) { return false; }
        if (stroke.segments.length !== 1) { return false; }
        const seg = stroke.segments[0];

        return Math.hypot(seg[0] - from.x, seg[1] - from.y) < 3
            && Math.hypot(seg[2] - to.x, seg[3] - to.y) < 3;
    });
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

// Scenario 1: the zone list focus. focusZone pins the camera on the
// ground and lights the label; blurZone restores the saved camera.
function runScenarioZoneFocus(mapFile) {
    const { MapView, record, elements } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot());

    const results = [];
    const before = MapView.worldToScreen(48000, 53000);

    const zone = MapView.lastSnap.huntingZones[1];
    MapView.focusZone(zone);

    // The camera centers the zone: its anchor lands mid canvas.
    const anchor = MapView.worldToScreen(48000, 53000);
    check(results, "focusZone centers the zone anchor",
        Math.abs(anchor.x - CANVAS_W / 2) < 2
        && Math.abs(anchor.y - CANVAS_H / 2) < 2,
        "anchor at " + JSON.stringify(anchor));

    // The zone carries its name while focused.
    record.texts.length = 0;
    MapView.draw();
    const label = record.texts.filter(
        (t) => t.text.startsWith("Deep Forest")).length;
    check(results, "focused zone carries the name label",
        label > 0, "no Deep Forest label while focused");

    // The follow flag survives: the checkbox stays on, the camera
    // just ignores it during the focus.
    check(results, "focusZone leaves the follow checkbox on",
        elements.get("follow").checked === true,
        "follow checkbox changed");

    MapView.blurZone();
    const after = MapView.worldToScreen(48000, 53000);
    check(results, "blurZone restores the camera",
        Math.abs(after.x - before.x) < 2
        && Math.abs(after.y - before.y) < 2,
        "expected " + JSON.stringify(before) + ", got "
        + JSON.stringify(after));
    check(results, "blurZone releases the label",
        (() => {
            record.texts.length = 0;
            MapView.draw();

            return record.texts.filter(
                (t) => t.text.startsWith("Deep Forest")).length === 0;
        })(), "label still drawn after the blur");

    return results;
}

// Scenario 2: the fleet kill crosses. The marks live in the map layer
// independent of the observed bot's snapshot.
function runScenarioKillMarks(mapFile) {
    const { MapView, record, elements } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot());

    const results = [];
    const fresh = worldToScreen(45100, 50100);
    const old = worldToScreen(45300, 50300);
    const stale = worldToScreen(47000, 52000);

    MapView.setKillMarks([
        { botId: "a", x: 45100, y: 50100, atMs: 0 },
        { botId: "b", x: 45300, y: 50300, atMs: -240000 },
        { botId: "a", x: 47000, y: 52000, atMs: -400000 }
    ]);
    const crossAt = (p) => record.strokes.filter((stroke) =>
        stroke.style === "#e37400" && stroke.segments.length === 2
        && Math.hypot(stroke.segments[0][0] - (p.x - 4),
            stroke.segments[0][1] - (p.y - 4)) < 3
        && Math.hypot(stroke.segments[1][0] - (p.x + 4),
            stroke.segments[1][1] - (p.y - 4)) < 3);

    check(results, "fresh kill draws its cross",
        crossAt(fresh).length > 0,
        "no cross at " + JSON.stringify(fresh));
    check(results, "aged kill draws a smaller cross",
        crossAt(old).length > 0,
        "no aged cross at " + JSON.stringify(old));
    check(results, "marks past the TTL never draw",
        crossAt(stale).length === 0,
        "stale cross drawn at " + JSON.stringify(stale));

    // The layer toggle hides everything.
    elements.get("show-kills").checked = false;
    record.strokes.length = 0;
    MapView.draw();
    // The dashed social warnings share the orange: only the solid
    // two-segment crosses count as kill marks.
    const hidden = record.strokes.filter((stroke) =>
        stroke.style === "#e37400" && stroke.dash.length === 0
        && stroke.segments.length === 2).length;
    check(results, "kills checkbox hides the crosses",
        hidden === 0, hidden + " crosses still drawn");

    return results;
}

// Scenario 3: the social links. Same clan in range links solid, the
// approaching pair warns dashed, different clans stay unlinked, the
// ALL clan links with every carrier.
function runScenarioSocialLinks(mapFile) {
    const { MapView, record, elements } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot());
    MapView.draw();

    const results = [];
    const orcA = worldToScreen(WORLD.orcA.x, WORLD.orcA.y);
    const orcB = worldToScreen(WORLD.orcB.x, WORLD.orcB.y);
    const orcFar = worldToScreen(WORLD.orcFar.x, WORLD.orcFar.y);
    const allClan = worldToScreen(WORLD.allClan.x, WORLD.allClan.y);
    const keltir = worldToScreen(WORLD.keltir.x, WORLD.keltir.y);

    // orcA-orcB: 224 units apart, inside the 300 range - solid teal.
    check(results, "clan mates inside the range link solid",
        findSegment(record, orcA, orcB, "#0aa5a5").length > 0,
        "no teal link between the clan mates");

    // orcA-allClan: 354 units apart - outside the shared 300 range
    // but inside the 375 warning band, the dashed amber link.
    check(results, "approaching pair warns with the dashed link",
        (() => {
            const dashed = record.strokes.filter((stroke) =>
                stroke.style === "#e37400" && stroke.dash.length === 2);
            return dashed.some((stroke) => stroke.segments.length === 1
                && (Math.hypot(stroke.segments[0][0] - orcA.x,
                    stroke.segments[0][1] - orcA.y) < 3
                    || Math.hypot(stroke.segments[0][0] - allClan.x,
                        stroke.segments[0][1] - allClan.y) < 3));
        })(), "no dashed warning link near the range edge");

    // The keltir carries no clan: nothing links to it.
    const keltirLinks = record.strokes.filter((stroke) =>
        (stroke.style === "#0aa5a5" || stroke.style === "#e37400")
        && stroke.segments.some((seg) =>
            Math.hypot(seg[0] - keltir.x, seg[1] - keltir.y) < 3
            || Math.hypot(seg[2] - keltir.x, seg[3] - keltir.y) < 3));
    check(results, "the clanless npc links with nothing",
        keltirLinks.length === 0,
        keltirLinks.length + " links touch the keltir");

    // The ALL clan mob links orcB (212 units, inside the range).
    check(results, "the ALL clan links the plain clan carrier",
        findSegment(record, orcB, allClan, "#0aa5a5").length > 0,
        "no ALL clan link to the orcs");

    // The toggle hides the whole layer.
    elements.get("show-social").checked = false;
    record.strokes.length = 0;
    MapView.draw();
    const left = record.strokes.filter((stroke) =>
        stroke.style === "#0aa5a5"
        || (stroke.style === "#e37400" && stroke.dash.length === 2)).length;
    check(results, "social checkbox hides the links",
        left === 0, left + " links still drawn");

    return results;
}

// Scenario 4: the tile pyramid fallback. A loading fine tile must
// yield to its loaded coarse ancestor instead of leaving the block
// blank (the follow mode white flash).
function runScenarioTileFallback(mapFile) {
    const { MapView } = loadMapJs(mapFile);
    MapView.init();

    const results = [];
    const fine = "maps/0/13_22.jpg";
    const coarse = "maps/2/13_22.jpg";
    const coarseImg = { tag: "coarse" };
    MapView.mapTiles.set(coarse, {
        img: coarseImg, ready: true, missing: false
    });
    MapView.mapTiles.set(fine, {
        img: null, ready: false, missing: false
    });

    const picked = MapView.mapTileAncestor(0, 13, 22);
    check(results, "loading fine tile falls back to the loaded coarse",
        picked && picked.ready === true && picked.img === coarseImg,
        "ancestor walk returned "
        + (picked ? "ready=" + picked.ready : "null"));

    // The ready fine tile wins over the coarse one.
    const fineImg = { tag: "fine" };
    MapView.mapTiles.set(fine, { img: fineImg, ready: true, missing: false });
    const pickedFine = MapView.mapTileAncestor(0, 13, 22);
    check(results, "the loaded fine tile wins the draw",
        pickedFine && pickedFine.img === fineImg,
        "fine tile not picked");

    // A tile with no loaded ancestor stays not ready (the caller
    // skips it) - the first sight of an area still streams in.
    const virgin = "maps/0/99_99.jpg";
    MapView.mapTiles.set(virgin, { img: null, ready: false, missing: false });
    const pickedVirgin = MapView.mapTileAncestor(0, 99, 99);
    check(results, "a virgin block without ancestors stays skipped",
        !pickedVirgin || pickedVirgin.ready === false,
        "unexpected ready entry for a virgin block");

    return results;
}

// Scenario 5: the spot hover of the map. The pointer inside the spot
// circle lights the label without any list interaction.
function runScenarioSpotHover(mapFile) {
    const { MapView, record, fireCanvas } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot());
    MapView.draw();

    const results = [];
    const idle = record.texts.filter(
        (t) => t.text.startsWith("Wolf Meadow")).length;
    check(results, "spot names stay hidden without the pointer",
        idle === 0, "spot label drawn without a hover");

    // Rest the pointer inside the active spot (over the character
    // would hit the object pick; aim off center but inside).
    const inSpot = worldToScreen(45400, 50400);
    fireCanvas("mousemove", {
        clientX: inSpot.x, clientY: inSpot.y
    });
    const hovered = record.texts.filter(
        (t) => t.text.startsWith("Wolf Meadow")).length;
    check(results, "the pointer inside the spot lights the name",
        hovered > 0, "no Wolf Meadow label on the map hover");

    // The hit test resolves the hovered zone object.
    check(results, "the hover resolves the spot under the cursor",
        MapView.hoverZone && MapView.hoverZone.id === "s1",
        "hoverZone is " + JSON.stringify(MapView.hoverZone && MapView.hoverZone.id));

    // Rest the pointer outside every zone: the label goes away.
    record.texts.length = 0;
    fireCanvas("mousemove", { clientX: 10, clientY: 10 });
    const left = record.texts.filter(
        (t) => t.text.startsWith("Wolf Meadow")).length;
    check(results, "leaving the spot hides the name",
        left === 0, "label still drawn after leaving");

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
        ["zone list focus", runScenarioZoneFocus(mapFile)],
        ["fleet kill crosses", runScenarioKillMarks(mapFile)],
        ["social links", runScenarioSocialLinks(mapFile)],
        ["tile ancestor fallback", runScenarioTileFallback(mapFile)],
        ["spot hover", runScenarioSpotHover(mapFile)]
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
            } else if (verbose) {
                console.log("        " + result.detail);
            }
        }
    }
    if (failed > 0) {
        console.log(failed + " CHECKS FAILED");
        process.exit(1);
    }
    console.log("ALL PASS");
}

main();
