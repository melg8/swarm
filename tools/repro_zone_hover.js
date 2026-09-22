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
// - the fleet kill marks: the marks live in the map layer, drawing
//   the dead mob face (the gray corpse circle with the X eyes - the
//   same icon the corpse marker carries, issue #6) for every kill of
//   every bot (the layer lives in the map, not the observed bot's
//   snapshot), the old marks melt away, the checkbox hides the layer;
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
        // The fill pass records land in record.fills (the skull
        // marker of the kill layer draws as two fill passes: the
        // orange body and the dark face), so the scenario can assert
        // the filled geometry, style and alpha at fill time.
        fill: () => {
            if (current) {
                record.fills.push({
                    segments: current.segments.slice(),
                    arcs: current.arcs.slice(),
                    style: record.fillStyle,
                    alpha: record.alpha
                });
            }
            current = null;
        },
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
    // The appended children and the removed classes record the DOM
    // writes the scenarios assert on (the tooltip content, the hidden
    // flips); the other stubs stay inert.
    const children = [];
    const removedClasses = [];
    const addedClasses = [];
    return {
        _children: children,
        _removedClasses: removedClasses,
        _addedClasses: addedClasses,
        classList: {
            contains: () => true,
            add: (c) => addedClasses.push(c),
            remove: (c) => removedClasses.push(c)
        },
        addEventListener: () => {},
        appendChild: (c) => children.push(c),
        append: (c) => children.push(c),
        style: {},
        textContent: "",
        // The honest emulation of the browser clear: the innerHTML
        // assignment drops the appended children.
        set innerHTML(value) { children.length = 0; },
        get innerHTML() { return ""; },
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
        strokes: [], fills: [], texts: [], style: "", fillStyle: "",
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
        clearTimeout: () => {},
        // The hunt mesh layer fetches /api/hunt-mesh: the scenarios
        // program the stub before the update that triggers the sync.
        fetch: (url) => sandbox.__fetchImpl(url)
    };
    sandbox.__fetchImpl = () => Promise.reject(new Error("no mesh"));
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(mapFile, "utf8"), sandbox,
        { filename: "map.js" });
    vm.runInContext("globalThis.__MapView = MapView;", sandbox);

    return {
        MapView: sandbox.__MapView, record, elements, sandbox,
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

// Scenario 1: the hexagon hunt cell layer. The snapshot carries the
// mesh version and the live record of the held cell; the layer
// fetches the static mesh once and draws EXACTLY TWO hexagons: the
// one the bot fights in (the fill, the bright stroke, the live
// label) and the one under the map cursor (the hover hit test of
// the mesh). Every other hexagon of the partition stays invisible -
// no edges, no fills, no labels.
const CELL_MESH = {
    version: "test-cells-1",
    cells: [
        { id: "c1", name: "Home Cell", minLevel: 1, maxLevel: 4,
          focusX: 45000, focusY: 50000, mass: 6,
          verts: [44500, 49500, 45500, 49500, 45500, 50500,
                  44500, 50500] },
        { id: "c2", name: "Neighbor Cell", minLevel: 5, maxLevel: 8,
          focusX: 46500, focusY: 51500, mass: 12,
          verts: [45500, 50500, 47500, 50500, 47500, 52500,
                  45500, 52500] }
    ]
};

function cellSnapshot(cellID, state) {
    const snap = buildSnapshot();
    snap.huntingZones = [];
    snap.huntMesh = CELL_MESH.version;
    snap.huntCell = {
        id: cellID, name: "Home Cell", state: state,
        minLevel: 1, maxLevel: 4, spawnMass: 6,
        respawnMinSec: 15, respawnMaxSec: 20,
        nextRespawnSec: 7, adenaPerMin: 42, deathHeat: 0,
        occupancy: 1, killX: 0, killY: 0
    };

    return snap;
}

// cellEdgeStrokes counts the strokes that trace an edge of the
// given cell polygon (a segment between two adjacent vertices of
// its world outline, within the tolerance).
function cellEdgeStrokes(record, cell) {
    const verts = cell.verts;
    let count = 0;
    for (const stroke of record.strokes) {
        for (const seg of stroke.segments) {
            for (let i = 0; i + 1 < verts.length; i += 2) {
                const a = worldToScreen(verts[i], verts[i + 1]);
                const b = worldToScreen(
                    verts[(i + 2) % verts.length],
                    verts[(i + 3) % verts.length]);
                if (Math.hypot(seg[0] - a.x, seg[1] - a.y) < 3
                    && Math.hypot(seg[2] - b.x, seg[3] - b.y) < 3) {
                    count++;

                    break;
                }
            }
        }
    }

    return count;
}

async function runScenarioHuntCells(mapFile) {
    const { MapView, record, fireCanvas, sandbox } = loadMapJs(mapFile);
    MapView.init();
    // The mesh fetch resolves through the stub of the sandbox: the
    // json() answer carries the cell payload of the scenario.
    let fetches = 0;
    sandbox.__fetchImpl = () => {
        fetches++;

        return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(CELL_MESH)
        });
    };
    MapView.update(cellSnapshot("c1", "farming"));

    const results = [];
    // The update triggered the mesh sync: the fetch fired once and
    // the cache holds the payload after the promise drains.
    await new Promise((resolve) => setImmediate(resolve));
    MapView.update(cellSnapshot("c1", "farming"));
    check(results, "the hunt mesh fetches once per version",
        fetches === 1 && !!MapView.huntMesh
            && MapView.huntMesh.version === "test-cells-1",
        "huntMesh is " + JSON.stringify(MapView.huntCell && null)
            + ", fetches " + fetches);

    record.strokes.length = 0;
    record.texts.length = 0;
    MapView.draw();

    // The active cell: the ONLY hexagon that draws by itself - its
    // outline strokes and its label carry the name, the state and
    // the live economy.
    const activeEdges = cellEdgeStrokes(record, CELL_MESH.cells[0]);
    check(results, "the held hexagon draws its outline",
        activeEdges >= 3, "no Home Cell outline stroke found");
    const label = record.texts.find(
        (t) => t.text.startsWith("Home Cell")
            && t.text.indexOf("hunting") >= 0
            && t.text.indexOf("42a/min") >= 0
            && t.text.indexOf("next 7s") >= 0);
    check(results, "the held cell carries the live label",
        !!label, "no live Home Cell label, texts: "
        + record.texts.map((t) => t.text).join(" | "));

    // The inactive neighbor stays INVISIBLE: no outline stroke of
    // its polygon, no label - the partition itself never draws.
    const neighborEdges = cellEdgeStrokes(record, CELL_MESH.cells[1]);
    check(results, "the inactive hexagons draw nothing",
        neighborEdges === 0,
        "the Neighbor Cell outline drew " + neighborEdges + " edges");
    const neighborLabel = record.texts.filter(
        (t) => t.text.startsWith("Neighbor Cell")).length;
    check(results, "the inactive cells stay labelless",
        neighborLabel === 0, "Neighbor Cell labeled without a hover");

    // The hexagon under the cursor draws: the hover hit test
    // resolves the mesh cell at the pointer, its outline and its
    // name label light up next to the active one.
    const overNeighbor = worldToScreen(46500, 51500);
    fireCanvas("mousemove", {
        clientX: overNeighbor.x, clientY: overNeighbor.y
    });
    record.strokes.length = 0;
    record.texts.length = 0;
    MapView.draw();
    check(results, "the hover resolves the hexagon under the cursor",
        MapView.hoverZone && MapView.hoverZone.kind === "cell"
            && MapView.hoverZone.id === "c2",
        "hoverZone is "
            + JSON.stringify(MapView.hoverZone && MapView.hoverZone.id));
    const hoverEdges = cellEdgeStrokes(record, CELL_MESH.cells[1]);
    check(results, "the hovered hexagon draws its outline",
        hoverEdges >= 3, "no Neighbor Cell outline on the hover");
    const hoverLabel = record.texts.filter(
        (t) => t.text.startsWith("Neighbor Cell")).length;
    check(results, "the hovered hexagon carries its name label",
        hoverLabel > 0, "no Neighbor Cell label on the hover");
    const activeKept = record.texts.filter(
        (t) => t.text.startsWith("Home Cell")).length;
    check(results, "the active hexagon keeps drawing under the hover",
        activeKept > 0, "the Home Cell label vanished on the hover");

    // The pointer leaves the mesh: the hovered hexagon disappears,
    // the active one stays.
    fireCanvas("mousemove", { clientX: 10, clientY: 10 });
    record.strokes.length = 0;
    record.texts.length = 0;
    MapView.draw();
    const leftEdges = cellEdgeStrokes(record, CELL_MESH.cells[1]);
    const keptLabel = record.texts.filter(
        (t) => t.text.startsWith("Home Cell")).length;
    check(results, "leaving the mesh hides the hovered hexagon",
        leftEdges === 0 && keptLabel > 0,
        "neighbor edges " + leftEdges + ", home label " + keptLabel);

    // The moving state: the label flips when the bot walks to the
    // cell (the map answers which zone the bot is heading to).
    record.texts.length = 0;
    MapView.update(cellSnapshot("c1", "moving"));
    MapView.draw();
    const moving = record.texts.find(
        (t) => t.text.startsWith("Home Cell") && t.text.indexOf("moving") >= 0);
    check(results, "the moving marker names the travel target",
        !!moving, "no moving label");

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

// Scenario 5: the fleet kill marks. The marks live in the map layer
// independent of the observed bot's snapshot and draw as the dead mob
// face (the gray corpse circle body with the X eyes in the dark
// slate, issue #6) that NEVER fades: the marks are the session death
// statistics, every age reads the same constant style - never a font
// glyph, never a melt.
function runScenarioKillMarks(mapFile) {
    const { MapView, record, elements, fireCanvas } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot());

    const results = [];
    const fresh = worldToScreen(45100, 50100);
    const old = worldToScreen(45300, 50300);
    const stale = worldToScreen(47000, 52000);

    MapView.setKillMarks([
        { botId: "a", x: 45100, y: 50100, atMs: 0,
            name: "Keltir", level: 4 },
        { botId: "b", x: 45300, y: 50300, atMs: -240000 },
        { botId: "a", x: 47000, y: 52000, atMs: -3 * 3600000 }
    ]);

    // The body pass lands as a fill (the corpse circle in the dead
    // marker gray), the eyes pass as a stroke (the X eyes in the dark
    // slate). The filters read a few px around the mark point.
    const bodyFills = (p) => record.fills.filter((fill) =>
        fill.style === "#80868b"
        && fill.arcs.some(([ax, ay]) =>
            Math.hypot(ax - p.x, ay - p.y) < 6));
    const bodyRadiusOf = (fill, p) => Math.max(...fill.arcs
        .filter(([ax, ay]) => Math.hypot(ax - p.x, ay - p.y) < 6)
        .map(([, , ar]) => ar));
    const eyeStrokes = (p) => record.strokes.filter((stroke) =>
        stroke.style === "#39424e"
        && stroke.dash.length === 0
        && stroke.segments.some(([x1, y1, x2, y2]) =>
            Math.hypot((x1 + x2) / 2 - p.x, (y1 + y2) / 2 - p.y) < 6));

    record.fills.length = 0;
    record.strokes.length = 0;
    record.texts.length = 0;
    MapView.draw();

    const freshBody = bodyFills(fresh);
    const freshEyes = eyeStrokes(fresh);
    const oldBody = bodyFills(old);
    check(results, "fresh kill draws its corpse body and X eyes",
        freshBody.length > 0 && freshEyes.length > 0,
        "body fills " + freshBody.length + ", eye strokes "
            + freshEyes.length);
    check(results, "aged kill draws the same constant style",
        oldBody.length > 0 && freshBody.length > 0
        && bodyRadiusOf(oldBody[0], old)
            === bodyRadiusOf(freshBody[0], fresh)
        && oldBody[0].alpha === freshBody[0].alpha,
        "aged body radius "
            + (oldBody.length > 0
                ? bodyRadiusOf(oldBody[0], old).toFixed(2) : "-")
            + " vs fresh "
            + (freshBody.length > 0
                ? bodyRadiusOf(freshBody[0], fresh).toFixed(2) : "-"));
    check(results, "no mark rides a font glyph",
        !record.texts.some(
            (t) => (t.text || "").indexOf("\u2620") >= 0),
        "the skull glyph was fillTexted");
    check(results, "an hours old mark still draws",
        bodyFills(stale).length > 0 && eyeStrokes(stale).length > 0,
        "a session long mark dropped its draw at "
            + JSON.stringify(stale));

    // The hover pick resolves the fresh mark with its victim data
    // (the empty map ground 60px away resolves nothing).
    const picked = MapView.killMarkAt(fresh.x, fresh.y);
    check(results, "the hover picks the kill mark with its victim",
        picked && picked.name === "Keltir" && picked.level === 4,
        "picked " + JSON.stringify(picked && picked.name));
    check(results, "the empty ground picks no mark",
        MapView.killMarkAt(fresh.x + 60, fresh.y + 60) === null,
        "a mark picked on the empty ground");

    // The tooltip of the hovered mark reads the victim (the name
    // with the level) and the age of the kill; a mark without the
    // captured victim falls back to the plain mob read.
    MapView.showKillTooltip(picked, fresh.x, fresh.y);
    const tooltip = elements.get("map-tooltip");
    check(results, "the mark tooltip names the victim and level",
        tooltip._children.length >= 2
        && tooltip._children[0].textContent === "Keltir lvl 4",
        "tooltip line " + (tooltip._children.length > 0
            ? JSON.stringify(tooltip._children[0].textContent) : "-"));
    check(results, "the mark tooltip reads the age",
        tooltip._children.length >= 2
        && tooltip._children[1].textContent === "killed 0s ago",
        "tooltip line " + (tooltip._children.length > 1
            ? JSON.stringify(tooltip._children[1].textContent) : "-"));
    const unnamed = MapView.killMarks.find(
        (mark) => !mark.name && mark.atMs === -240000);
    MapView.showKillTooltip(unnamed, old.x, old.y);
    check(results, "a victimless mark tooltip falls back",
        tooltip._children.length >= 1
        && tooltip._children[0].textContent === "a mob",
        "tooltip line " + (tooltip._children.length > 0
            ? JSON.stringify(tooltip._children[0].textContent) : "-"));

    // The full hover flow: the pointer over the mark shows the
    // tooltip (the hidden class drops), the pointer over the empty
    // ground hides it again.
    tooltip._removedClasses.length = 0;
    tooltip._addedClasses.length = 0;
    fireCanvas("mousemove", { clientX: fresh.x, clientY: fresh.y });
    check(results, "the pointer on the mark shows the tooltip",
        MapView.hoverMark === picked
        && tooltip._removedClasses.includes("hidden"),
        "hoverMark " + JSON.stringify(MapView.hoverMark
            && MapView.hoverMark.name)
        + ", removed " + tooltip._removedClasses.join(","));
    fireCanvas("mousemove", { clientX: 10, clientY: 10 });
    check(results, "leaving the mark hides the tooltip",
        MapView.hoverMark === null
        && tooltip._addedClasses.includes("hidden"),
        "hoverMark " + JSON.stringify(MapView.hoverMark)
        + ", added " + tooltip._addedClasses.join(","));

    // The layer toggle hides the fleet ring: the gray body fills and
    // the eye strokes near the marks drop while the dashed social
    // warnings keep drawing (the layer isolation of the toggles). The
    // pristine scene has no corpse object yet - the only gray+eyes
    // paint near the marks is the fleet layer itself.
    elements.get("show-kills").checked = false;
    record.fills.length = 0;
    record.strokes.length = 0;
    MapView.draw();
    const hiddenBodies = [fresh, old, stale].flatMap((p) =>
        bodyFills(p)).length;
    const hiddenEyes = [fresh, old, stale].flatMap((p) =>
        eyeStrokes(p)).length;
    const dashedWarn = record.strokes.filter((stroke) =>
        stroke.style === "#e37400" && stroke.dash.length === 2).length;
    check(results, "kills checkbox hides the fleet marks",
        hiddenBodies === 0 && hiddenEyes === 0,
        hiddenBodies + " body fills, " + hiddenEyes
            + " eye strokes still drawn");
    check(results, "the social layer ignores the kills toggle",
        dashedWarn > 0, "no dashed warning drew - the check went blind");
    elements.get("show-kills").checked = true;

    // The corpse sits exactly on its own kill mark: the victim
    // tooltip with the kill age wins over the dead unit tooltip (the
    // living units keep their tooltips everywhere).
    const snap = buildSnapshot();
    snap.objects.push({ objectId: 999, kind: "npc", name: "Dead Wolf",
        level: 3, dead: true, x: 45100, y: 50100 });
    MapView.update(snap);
    fireCanvas("mousemove", { clientX: fresh.x, clientY: fresh.y });
    check(results, "the corpse mark tooltip wins the dead unit",
        MapView.hoverMark === picked,
        "hoverMark " + JSON.stringify(MapView.hoverMark
            && MapView.hoverMark.name));

    return results;
}

async function main() {
    const args = process.argv.slice(2);
    const verbose = args.includes("--verbose");
    const mapIndex = args.indexOf("--map");
    const mapFile = mapIndex >= 0 ? args[mapIndex + 1] : MAP_JS;
    if (!fs.existsSync(mapFile)) {
        console.error("map.js not found: " + mapFile);
        process.exit(1);
    }

    const scenarios = [
        ["hunt cell layer", await runScenarioHuntCells(mapFile)],
        ["fleet kill marks", runScenarioKillMarks(mapFile)],
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

main().catch((err) => {
    console.error(err);
    process.exit(1);
});
