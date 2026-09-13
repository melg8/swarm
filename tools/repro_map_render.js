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

// The theme palette of style.css (the ui chrome reads it) plus the
// fixed map palette of map.js so recorded stroke styles are
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

// The fixed unit marker palette of map.js (theme independent).
const MARK = {
    self: "#1a73e8",
    player: "#9334e6",
    item: "#f9ab00",
    friendly: "#5f6368",
    passive: "#188038",
    aggressive: "#e37400",
    combat: "#d93025",
    dead: "#80868b"
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
    let pending = null;
    return {
        canvas: { width: CANVAS_W, height: CANVAS_H },
        clearRect: () => {},
        setTransform: () => {},
        save: () => {},
        restore: () => {},
        beginPath: () => { current = { segments: [], arcs: [] }; },
        closePath: () => {},
        // The path model: every lineTo completes one segment from the
        // pending point, so multi edge strokes (the hunting zone square)
        // record their edges faithfully.
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
        fillText: (text, x, y) => {
            record.texts.push({ text, x, y, style: record.fillStyle });
        },
        strokeText: (text, x, y) => {
            record.texts.push({ text, x, y, style: record.strokeStyle });
        },
        measureText: (text) => ({ width: (text || "").length * 6 }),
        setLineDash: (dash) => { record.dash = dash.slice(); },
        // The background cache blit: the arguments land in the record
        // so a scenario can assert what was composited and where.
        drawImage: (...args) => { record.blits.push(args); },
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
        strokes: [], fills: [], texts: [], blits: [], style: "",
        fillStyle: "", width: 1, dash: []
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
        "show-map": true, "show-zone": false, "show-targets": true,
        "show-hunt-zones": true, "show-aggro": true
    };
    const elements = new Map();
    // The sandbox clock: performance.now() stands still unless a
    // scenario advances it (advanceClock), so every timing behavior
    // of map.js (the fps windows, the tile commit throttle) is
    // deterministic under the scenario steps.
    let clockNow = 0;
    // The sandbox timers: setTimeout records the callback instead of
    // scheduling it, and runTimers() fires the pending ones in order
    // - the trailing commit of a tile storm lands inside the scenario
    // without real time passing.
    const timers = [];
    let timerSeq = 0;
    const sandbox = {
        Math, JSON,
        Date: { now: () => 0 },
        performance: { now: () => clockNow },
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
            // The offscreen canvas of the static background cache: a
            // stub canvas with its own recording context (exposed as
            // __record) so a scenario can see what rasterized into
            // the cache versus what painted per frame.
            createElement: (tag) => {
                if (tag !== "canvas") { return makeElementStub(false); }
                const bgRecord = {
                    strokes: [], fills: [], texts: [], blits: [],
                    style: "", fillStyle: "", width: 1, dash: []
                };

                return {
                    width: 0, height: 0,
                    getContext: () => makeRecordingContext(bgRecord),
                    __record: bgRecord
                };
            }
        },
        window: { addEventListener: listen(listeners.window) },
        getComputedStyle: () => ({
            getPropertyValue: (name) => THEME[name] || ""
        }),
        setTimeout: (fn) => {
            timerSeq += 1;
            timers.push({ id: timerSeq, fn, canceled: false });

            return timerSeq;
        },
        clearTimeout: (id) => {
            for (const timer of timers) {
                if (timer.id === id) { timer.canceled = true; }
            }
        }
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(mapFile, "utf8"), sandbox,
        { filename: "map.js" });
    vm.runInContext("globalThis.__MapView = MapView;", sandbox);

    return {
        MapView: sandbox.__MapView, record, elements,
        advanceClock: (ms) => { clockNow += ms; },
        runTimers: () => {
            const due = timers.splice(0, timers.length);
            for (const timer of due) {
                if (!timer.canceled) { timer.fn(); }
            }
        },
        fireCanvas: (type, event) => fire(listeners.canvas, type, event),
        fireWindow: (type, event) => fire(listeners.window, type, event)
    };
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
        findSegment(record, player, mob, MARK.player).length > 0,
        "expected a violet segment " + JSON.stringify(player) + " -> "
        + JSON.stringify(mob));

    // Bug 1: the claimed mob gets a violet ring.
    const claimedRadius = markerRadius("npc") + 5;
    check(results, "violet ring around the mob claimed by the player",
        findArc(record, mob, claimedRadius, MARK.player).length > 0,
        "expected a violet arc at " + JSON.stringify(mob)
        + " r=" + claimedRadius);

    // Bug 1: the own target link (regression guard) stays red.
    check(results, "own target link still drawn",
        findSegment(record, self, ownTarget, MARK.combat).length > 0,
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
        findSegment(record, player, self, MARK.player).length > 0,
        "expected a violet segment " + JSON.stringify(player) + " -> "
        + JSON.stringify(self));
    const selfRing = markerRadius("self") + 5;
    check(results, "violet ring around the bot targeted by the player",
        findArc(record, self, selfRing, MARK.player).length > 0,
        "expected a violet arc at " + JSON.stringify(self)
        + " r=" + selfRing);

    return results;
}

// runScenarioStableOrder covers the draw order of overlapping units:
// the snapshot objects arrive in random go map order, the renderer must
// draw them in one deterministic order (dead first, then north to
// south) or the overlapping markers swap their z position every
// snapshot and flicker.
function runScenarioStableOrder(mapFile) {
    const results = [];
    const orders = [];
    for (let round = 0; round < 3; round++) {
        const { MapView, record } = loadMapJs(mapFile);
        MapView.init();
        const snap = buildSnapshot(0, false);
        // Two npcs stacked at the same spot, ids and array positions
        // swapped per round to simulate the random map iteration.
        const first = snap.objects.find(
            (o) => o.objectId === WORLD.playerTargetMob.objectId);
        const second = snap.objects.find(
            (o) => o.objectId === WORLD.ownTargetMob.objectId);
        first.x = WORLD.self.x + 100;
        first.y = WORLD.self.y + 100;
        second.x = WORLD.self.x + 100;
        second.y = WORLD.self.y + 100;
        if (round % 2 === 1) {
            snap.objects.reverse();
        }
        MapView.update(snap);
        MapView.draw();
        // The fill centers of the two npcs in draw order.
        const centers = record.fills
            .map((fill) => fill.arcs[0])
            .filter((arc) => arc && Math.hypot(
                arc[0] - worldToScreen(WORLD.self.x + 100,
                    WORLD.self.y + 100).x) < 2)
            .map((arc) => arc[1]);
        orders.push(centers.join("|"));
    }
    check(results, "overlapping units draw in one stable order",
        orders[0] === orders[1] && orders[1] === orders[2],
        "draw orders: " + JSON.stringify(orders));
    check(results, "units draw north to south",
        orders[0].split("|").every((y, i, all) => i === 0
            || Number(all[i - 1]) <= Number(y)),
        "y order: " + orders[0]);

    return results;
}

// runScenarioRestMarker covers the resting state of the character: a
// sitting character draws the breathing zZ above its marker, a standing
// one does not.
function runScenarioRestMarker(mapFile) {
    const { MapView, record } = loadMapJs(mapFile);
    MapView.init();
    const snap = buildSnapshot(0, false);
    snap.character.sitting = true;
    MapView.update(snap);
    MapView.draw();

    const results = [];
    const self = {
        x: CANVAS_W / 2, y: CANVAS_H / 2
    };
    const zTexts = record.texts.filter((t) => t.text === "zZ"
        && Math.abs(t.x - (self.x + 7 + 4)) < 2);
    check(results, "sitting character draws the zZ marker",
        zTexts.length > 0, "no zZ text near the self marker");

    const standing = loadMapJs(mapFile);
    standing.MapView.init();
    standing.MapView.update(buildSnapshot(0, false));
    standing.MapView.draw();
    const zStanding = standing.record.texts.filter(
        (t) => t.text === "zZ").length;
    check(results, "standing character draws no zZ marker",
        zStanding === 0, "unexpected zZ texts: " + zStanding);

    // The fleet-wide rest icon: another bot of the fleet (a player
    // object of the watched bot's world) draws the same zZ marker when
    // it rests, so the overview shows every resting bot regardless of
    // which one the web UI focuses on.
    const sitting = loadMapJs(mapFile);
    sitting.MapView.init();
    const sittingSnap = buildSnapshot(0, false);
    sittingSnap.objects[0].sitting = true;
    sitting.MapView.update(sittingSnap);
    sitting.MapView.draw();
    const playerPos = worldToScreen(WORLD.player.x, WORLD.player.y);
    const zPlayer = sitting.record.texts.filter((t) => t.text === "zZ"
        && Math.abs(t.x - (playerPos.x + 4 + 4)) < 2
        && Math.abs(t.y - (playerPos.y - 4 - 3)) < 2);
    check(results, "sitting player object draws the zZ marker",
        zPlayer.length > 0, "no zZ text near the player marker");
    const sittingNoSelf = sitting.record.texts.filter(
        (t) => t.text === "zZ").length;
    check(results, "only the sitting player draws zZ (self stands)",
        sittingNoSelf === zPlayer.length,
        "unexpected extra zZ texts: " + sittingNoSelf);

    return results;
}

// runScenarioHuntingZone covers the hunting square: a snapshot with a
// configured zone draws the dashed amber rectangle at the world rect
// plus the hunting zone label, and a snapshot without a zone draws
// nothing.
function runScenarioHuntingZone(mapFile) {
    const { MapView, record, fireCanvas } = loadMapJs(mapFile);
    MapView.init();
    const snap = buildSnapshot(0, false);
    snap.huntingZone = { cx: WORLD.self.x, cy: WORLD.self.y, half: 450 };
    MapView.update(snap);
    MapView.draw();

    const results = [];
    const half = 450 * WORLD.scale; // 54 px
    const left = { x: CANVAS_W / 2 - half, y: CANVAS_H / 2 - half };
    const right = { x: CANVAS_W / 2 + half, y: CANVAS_H / 2 - half };
    const topEdge = record.strokes.filter((stroke) => {
        if (stroke.style !== MARK.item || stroke.segments.length === 0) {
            return false;
        }
        return stroke.segments.some((seg) => {
            const yOk = seg.length >= 4
                && Math.abs(seg[1] - left.y) < 2
                && Math.abs(seg[3] - left.y) < 2;
            const xOk = seg[0] >= left.x - 2 && seg[2] <= right.x + 2;

            return yOk && xOk && seg[2] > seg[0];
        });
    });
    check(results, "hunting zone draws the dashed square top edge",
        topEdge.length > 0,
        "expected dashed segments along the top edge at y="
        + left.y);
    // The zone names wait for the pointer: without a hover the square
    // stays silent, a mousemove inside it lights the label up.
    const labelIdle = record.texts.filter(
        (t) => t.text.startsWith("hunting zone")).length;
    check(results, "zone label stays hidden without the pointer",
        labelIdle === 0, "zone label drawn without a hover");

    // Hover the middle of the square (no object sits there, so the
    // tooltip never touches the DOM).
    fireCanvas("mousemove", {
        clientX: CANVAS_W / 2, clientY: CANVAS_H / 2
    });
    const label = record.texts.filter(
        (t) => t.text.startsWith("hunting zone")
        && Math.abs(t.x - (left.x + 6)) < 2);
    check(results, "hunting zone carries the label on hover",
        label.length > 0, "no label at the square corner");
    check(results, "the hovered zone reads the highlight stroke",
        record.strokes.some((stroke) => stroke.style === "#f9ab00"
            && stroke.width === 2),
        "no highlighted zone stroke");

    // Moving out of the square hides the label again (the record
    // resets between the draws so only the fresh pass counts).
    record.texts.length = 0;
    fireCanvas("mousemove", { clientX: 10, clientY: 10 });
    const labelAfter = record.texts.filter(
        (t) => t.text.startsWith("hunting zone")).length;
    check(results, "leaving the zone hides the label",
        labelAfter === 0, "label still drawn after the leave");

    const plain = loadMapJs(mapFile);
    plain.MapView.init();
    plain.MapView.update(buildSnapshot(0, false));
    plain.MapView.draw();
    const labelPlain = plain.record.texts.filter(
        (t) => t.text.startsWith("hunting zone")).length;
    check(results, "no hunting zone without the configuration",
        labelPlain === 0, "unexpected zone label");

    return results;
}

// runScenarioHuntZonesView covers the multi zone view layer: the
// inactive future grounds draw with the bright blue demonstration
// stroke, the aggro radius circles draw around the aggressive mobs,
// and the two toolbar checkboxes hide their layers.
function runScenarioHuntZonesView(mapFile) {
    const { MapView, record, elements } = loadMapJs(mapFile);
    MapView.init();
    const snap = buildSnapshot(0, false);
    snap.huntingZones = [{
        id: "z1", name: "Future Ground", region: "elven",
        minLevel: 1, maxLevel: 3, minGear: 0,
        cx: WORLD.self.x, cy: WORLD.self.y, half: 450,
        active: false, demoted: false, deaths: 0
    }];
    snap.objects.push({
        objectId: 500, kind: "npc", name: "Orc Raider",
        x: WORLD.self.x + 300, y: WORLD.self.y - 200, z: -3500,
        heading: 0, moving: false, speed: 0, targetId: 0,
        dead: false, attackable: true, aggressive: true, aggroRange: 400,
        inCombat: false, level: 4
    });
    MapView.update(snap);
    MapView.draw();

    const results = [];
    // The hunt layer rasterizes into its own offscreen cache (see
    // runScenarioHuntLayerCache): the blue square strokes land in
    // the cache record while the main canvas composites the raster.
    const hunt = MapView.huntBg;
    const huntRecord = hunt.canvas && hunt.canvas.__record;
    const future = (huntRecord ? huntRecord.strokes : []).filter(
        (stroke) => stroke.style === "#5b9bd5" && stroke.segments.length >= 3);
    check(results, "inactive hunting zone draws the bright blue square",
        future.length > 0, "no bright blue square stroke");

    const center = worldToScreen(WORLD.self.x + 300, WORLD.self.y - 200);
    const radius = 400 * WORLD.scale;
    const circles = record.strokes.filter((stroke) =>
        stroke.style === MARK.aggressive && stroke.arcs.length === 1
        && Math.hypot(stroke.arcs[0][0] - center.x,
            stroke.arcs[0][1] - center.y) < 3
        && Math.abs(stroke.arcs[0][2] - radius) < 3);
    check(results, "aggressive mob draws its aggro radius circle",
        circles.length > 0, "no circle at the mob position");

    // Both toggles hide their layers: flip them, clear the record and
    // redraw the same snapshot.
    elements.get("show-hunt-zones").checked = false;
    elements.get("show-aggro").checked = false;
    record.strokes.length = 0;
    record.fills.length = 0;
    record.texts.length = 0;
    MapView.draw();
    const zoneStrokes = record.strokes.filter((stroke) =>
        stroke.style === "#5b9bd5").length;
    check(results, "hunt zones checkbox hides the zone squares",
        zoneStrokes === 0, "zone strokes still drawn: " + zoneStrokes);
    const aggroStrokes = record.strokes.filter((stroke) =>
        stroke.style === MARK.aggressive && stroke.arcs.length === 1).length;
    check(results, "aggro checkbox hides the radius circles",
        aggroStrokes === 0, "circles still drawn: " + aggroStrokes);

    return results;
}

// runScenarioSocialMarker covers the social animation marker: a
// creature with a fresh socialUntilMs shows the small ring above its
// marker, and the ring fades out once the window is over.
function runScenarioSocialMarker(mapFile) {
    const { MapView, record } = loadMapJs(mapFile);
    MapView.init();
    const social = buildSnapshot(0, false);
    const mob = social.objects.find(
        (o) => o.objectId === WORLD.playerTargetMob.objectId);
    mob.socialUntilMs = 2000000000000; // far in the future of the stub clock
    MapView.update(social);
    MapView.draw();

    const results = [];
    const pos = {
        x: CANVAS_W / 2 + (WORLD.playerTargetMob.x - WORLD.self.x)
            * WORLD.scale,
        y: CANVAS_H / 2 + (WORLD.playerTargetMob.y - WORLD.self.y)
            * WORLD.scale
    };
    check(results, "social animation draws the ring above the npc",
        findArc(record, { x: pos.x, y: pos.y - 5 - 7 }, 3.5, null).length > 0,
        "expected a ring above " + JSON.stringify(pos));

    // Without the marker window there is no ring.
    const fresh = loadMapJs(mapFile);
    fresh.MapView.init();
    const plain = buildSnapshot(0, false);
    plain.objects.find(
        (o) => o.objectId === WORLD.playerTargetMob.objectId
    ).socialUntilMs = 0;
    fresh.MapView.update(plain);
    fresh.MapView.draw();
    check(results, "no social ring without an animation window",
        fresh.record.strokes.filter((stroke) => stroke.arcs.some(
            (arc) => Math.abs(arc[2] - 3.5) < 0.1)).length === 0,
        "unexpected social ring on a plain snapshot");

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

// runScenarioMapDrag covers the grab semantics of map panning: while the
// left button is held the map moves exactly with the cursor, so the
// world point grabbed under it stays pinned (the reported bug: the map
// slid in the wrong direction and 1/scale times too far, which felt
// like swiping instead of holding), and grabbing switches follow off.
function runScenarioMapDrag(mapFile) {
    const { MapView, elements, fireCanvas, fireWindow } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(0, false));

    const results = [];
    const mob = MapView.worldToScreen(
        WORLD.playerTargetMob.x, WORLD.playerTargetMob.y);
    const self = MapView.worldToScreen(WORLD.self.x, WORLD.self.y);

    // Grab the map at (300, 200) and drag it to (420, 260).
    fireCanvas("mousedown", { clientX: 300, clientY: 200 });
    check(results, "grabbing the map disables follow",
        elements.get("follow").checked === false,
        "follow is still " + elements.get("follow").checked);
    fireWindow("mousemove", { clientX: 420, clientY: 260 });
    fireWindow("mouseup", { clientX: 420, clientY: 260 });

    const dx = 120;
    const dy = 60;
    const mobAfter = MapView.worldToScreen(
        WORLD.playerTargetMob.x, WORLD.playerTargetMob.y);
    const selfAfter = MapView.worldToScreen(WORLD.self.x, WORLD.self.y);
    check(results, "dragged map moves exactly with the cursor",
        Math.abs(mobAfter.x - (mob.x + dx)) < 2
        && Math.abs(mobAfter.y - (mob.y + dy)) < 2,
        "expected (" + (mob.x + dx) + ", " + (mob.y + dy) + "), got "
        + JSON.stringify(mobAfter));
    check(results, "dragged map shifts every point by the cursor delta",
        Math.abs(selfAfter.x - (self.x + dx)) < 2
        && Math.abs(selfAfter.y - (self.y + dy)) < 2,
        "expected (" + (self.x + dx) + ", " + (self.y + dy) + "), got "
        + JSON.stringify(selfAfter));

    return results;
}

// runScenarioFpsMeter covers the on-screen fps counter: the meter is
// wired into the paint path (every draw feeds it), the chip renders
// the window reading with the health color, a spike shows its worst
// frame, and a steady reading rewrites nothing (a frozen clock keeps
// the windows from closing in this harness, so the chip is driven
// through its render entry point directly).
function runScenarioFpsMeter(mapFile) {
    const { MapView, elements } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(0, false));

    const results = [];
    const paintsBefore = MapView.fps.paints;
    MapView.draw();
    MapView.draw();
    check(results, "every paint feeds the fps meter",
        MapView.fps.paints === paintsBefore + 2,
        "paints went from " + paintsBefore + " to "
        + MapView.fps.paints);

    MapView.renderFpsChip(60, 2.1, 3.0);
    const chip = elements.get("foot-fps");
    check(results, "the chip reads the window fps and draw cost",
        chip.textContent === "fps: 60 · draw 2.1 ms",
        "chip says " + JSON.stringify(chip.textContent));
    check(results, "a healthy window colors the chip green",
        chip.style.color === "#188038",
        "color is " + JSON.stringify(chip.style.color));

    MapView.renderFpsChip(20, 9.4, 40.2);
    check(results, "a starved window colors the chip red and shows the worst frame",
        chip.textContent === "fps: 20 · draw 9.4 ms · worst 40.2 ms"
        && chip.style.color === "#d93025",
        "chip says " + JSON.stringify(chip.textContent)
        + " color " + JSON.stringify(chip.style.color));

    MapView.renderFpsChip(35, 4.0, 5.0);
    check(results, "a degraded window colors the chip amber",
        chip.style.color === "#9a6700",
        "color is " + JSON.stringify(chip.style.color));

    chip.textContent = "sentinel";
    MapView.renderFpsChip(35, 4.0, 5.0);
    check(results, "a steady reading rewrites nothing",
        chip.textContent === "sentinel",
        "chip says " + JSON.stringify(chip.textContent));

    return results;
}

// runScenarioBackgroundCache pins the static world cache: the tiles,
// the grid and the zone frame rasterize once into the offscreen
// canvas and every later frame composites it with one drawImage
// instead of re-stroking the grid; a camera pan inside the slack box
// only shifts the blit, and a zoom re-rasters (the cache key carries
// the scale). The dynamic layers (the unit markers) must keep
// painting on every frame.
function runScenarioBackgroundCache(mapFile) {
    const { MapView, elements, record } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(0, false));

    const results = [];
    const bg = MapView.bg;
    const bgRecord = bg.canvas && bg.canvas.__record;
    check(results, "the static world rasterizes into the offscreen cache",
        !!bgRecord && bgRecord.strokes.length > 0,
        "the cache is " + (bgRecord
            ? "holding " + bgRecord.strokes.length + " grid strokes"
            : "missing"));

    const strokesBefore = bgRecord.strokes.length;
    const blitsBefore = record.blits.length;
    const fillsBefore = record.fills.length;
    MapView.draw();
    check(results, "a steady frame re-strokes no static geometry",
        bgRecord.strokes.length === strokesBefore,
        (bgRecord.strokes.length - strokesBefore) + " new strokes");
    check(results, "a steady frame composites the cache once",
        record.blits.length === blitsBefore + 1,
        (record.blits.length - blitsBefore) + " blits");
    check(results, "the dynamic units still paint on every frame",
        record.fills.length > fillsBefore,
        "the unit fills went " + fillsBefore + " -> " + record.fills.length);

    // Pan the free camera inside the slack box: no re-raster, the
    // blit offset tracks the camera (the world slides opposite to it).
    elements.get("follow").checked = false;
    MapView.syncPanAnchor();
    const lastBlit = record.blits[record.blits.length - 1];
    MapView.panAnchor.x += 200;
    MapView.draw();
    check(results, "a pan inside the slack re-rasters nothing",
        bgRecord.strokes.length === strokesBefore,
        (bgRecord.strokes.length - strokesBefore) + " new strokes");
    const panned = record.blits[record.blits.length - 1];
    check(results, "the blit offset follows the camera",
        Math.abs((panned[5] - lastBlit[5]) + 200 * MapView.scale) <= 1,
        "the blit dx moved " + (panned[5] - lastBlit[5])
            + " instead of " + (-200 * MapView.scale).toFixed(2));

    // A zoom changes the raster key: the cache re-renders.
    MapView.zoom(1.5);
    check(results, "a zoom re-rasters the cache",
        bgRecord.strokes.length > strokesBefore,
        (bgRecord.strokes.length - strokesBefore) + " new strokes");

    return results;
}

// runScenarioHuntLayerCache pins the hunt layer cache: the spot
// circles and the squares of the registry rasterize once into their
// own offscreen canvas and every steady frame composites them with
// one drawImage; the economy only registry ticks (the respawn
// countdown, the adena rate) re-raster nothing, a visual change
// (the active flag, a heat bucket) does, and the hovered zone still
// reads its live label on top of the cached shapes.
function runScenarioHuntLayerCache(mapFile) {
    const { MapView, record, fireCanvas } = loadMapJs(mapFile);
    MapView.init();
    const base = buildSnapshot(0, false);
    base.huntingZones = [
        {
            id: "s1", name: "Spot One", kind: "spot",
            cx: WORLD.self.x, cy: WORLD.self.y, radius: 1200,
            active: true, demoted: false, deaths: 2, deathHeat: 0.4,
            killX: WORLD.self.x + 100, killY: WORLD.self.y + 100,
            respawnMinSec: 15, respawnMaxSec: 20, minLevel: 1,
            maxLevel: 3, nextRespawnSec: 30, adenaPerMin: 900,
            occupancy: 1
        },
        {
            id: "s2", name: "Spot Two", kind: "spot",
            cx: WORLD.self.x - 8000, cy: WORLD.self.y + 8000,
            radius: 900, active: false, demoted: false, deaths: 0,
            deathHeat: 0, nextRespawnSec: -1, adenaPerMin: 0,
            occupancy: 1
        },
        {
            id: "r1", name: "Future Ground",
            cx: WORLD.self.x + 9000, cy: WORLD.self.y - 9000,
            half: 2500, active: false, demoted: false, deaths: 0,
            minLevel: 4, maxLevel: 7, minGear: 0
        }
    ];
    MapView.update(base);
    MapView.draw();

    const results = [];
    const hunt = MapView.huntBg;
    const huntRecord = hunt.canvas && hunt.canvas.__record;
    check(results, "the hunt layer rasterizes into its own offscreen cache",
        !!huntRecord && huntRecord.strokes.length > 0,
        "the hunt cache is " + (huntRecord
            ? "holding " + huntRecord.strokes.length + " strokes"
            : "missing"));

    // A steady frame re-strokes no zone geometry and composites both
    // caches (the background and the hunt layer) with one blit each.
    const cacheStrokes = huntRecord.strokes.length;
    const blitsBefore = record.blits.length;
    MapView.draw();
    check(results, "a steady frame re-strokes no zone geometry",
        huntRecord.strokes.length === cacheStrokes,
        (huntRecord.strokes.length - cacheStrokes) + " new strokes");
    check(results, "a steady frame composites the hunt cache once",
        record.blits.length === blitsBefore + 2
        && record.blits.some((args) => args[0] === hunt.canvas),
        record.blits.length - blitsBefore + " blits");

    // An economy only registry tick (the countdown, the adena rate)
    // feeds the hovered label alone: the raster must not drop.
    const economy = JSON.parse(JSON.stringify(base));
    economy.huntingZones[0].nextRespawnSec = 29;
    economy.huntingZones[0].adenaPerMin = 950;
    economy.huntingZones[0].deaths = 3;
    MapView.update(economy);
    MapView.draw();
    check(results, "an economy only update re-rasters nothing",
        huntRecord.strokes.length === cacheStrokes,
        (huntRecord.strokes.length - cacheStrokes) + " new strokes");

    // A visual change (the active flag switch and a heat bucket step)
    // re-renders the raster.
    const switched = JSON.parse(JSON.stringify(base));
    switched.huntingZones[0].active = false;
    switched.huntingZones[0].deathHeat = 0.8;
    switched.huntingZones[1].active = true;
    MapView.update(switched);
    MapView.draw();
    check(results, "an active zone switch re-rasters the layer",
        huntRecord.strokes.length > cacheStrokes,
        (huntRecord.strokes.length - cacheStrokes) + " new strokes");

    // The hovered spot still reads its live economy label on top of
    // the cached shapes (the middle of the canvas sits inside s1).
    record.texts.length = 0;
    fireCanvas("mousemove", {
        clientX: CANVAS_W / 2, clientY: CANVAS_H / 2
    });
    const label = record.texts.find(
        (t) => t.text.startsWith("Spot One"));
    check(results, "the hovered spot carries its live label",
        !!label, "no Spot One label after the hover");
    check(results, "the label carries the economy suffixes",
        !!label && label.text.includes("a/min")
        && label.text.includes("deaths"),
        "the label reads " + (label ? label.text : "nothing"));

    // A zoom changes the raster key (the screen radius of every
    // circle derives from the scale): the cache re-renders.
    const beforeZoom = huntRecord.strokes.length;
    MapView.zoom(1.5);
    check(results, "a zoom re-rasters the hunt cache",
        huntRecord.strokes.length > beforeZoom,
        (huntRecord.strokes.length - beforeZoom) + " new strokes");

    return results;
}

// runScenarioTileLoadStorm pins the throttled tile arrival commits
// of the background cache: a zoom-out load burst lands dozens of
// tiles over seconds, and a raw arrival counter in the cache key
// would re-raster the whole static world on every animated frame of
// the load window (the render loop keeps painting while the tiles
// stream in - the low fps of a loading map). The first arrival of a
// window commits immediately, the burst coalesces into one trailing
// commit, and the steady frames between them stay blit only.
function runScenarioTileLoadStorm(mapFile) {
    const { MapView, record, advanceClock, runTimers } = loadMapJs(mapFile);
    MapView.init();
    MapView.update(buildSnapshot(0, false));
    MapView.draw();

    const results = [];
    const bg = MapView.bg;
    const bgRecord = bg.canvas && bg.canvas.__record;
    check(results, "the background cache exists",
        !!bgRecord, "the cache is missing");

    // Zoom out to the far view: the cache walk of the wide world
    // creates the pending tile entries of the burst (the sandbox has
    // no Image, so nothing loads until this scenario lands them). The
    // registry reset keeps every pending entry on the pyramid levels
    // the far view actually walks (the initial close view created its
    // entries at the full resolution level).
    advanceClock(1000);
    MapView.mapTiles = new Map();
    MapView.zoom(0.125);
    MapView.zoom(0.125);

    // landTiles lands the next pending tiles exactly like the image
    // loads would (the ready flag plus the arrival call of onload).
    const landed = [];
    const landTiles = (count) => {
        let touched = 0;
        for (const entry of MapView.mapTiles.values()) {
            if (touched >= count) { break; }
            if (entry.ready || entry.missing) { continue; }
            entry.ready = true;
            entry.img = { width: 512, height: 512 };
            landed.push(entry.img);
            MapView.tileArrived();
            touched += 1;
        }

        return touched;
    };

    // The first arrival of a window commits on the spot: the tile
    // rasterizes into the cache within its own arrival call.
    const firstBlits = bgRecord.blits.length;
    const firstCount = landTiles(1);
    check(results, "the first landed tile commits immediately",
        firstCount === 1 && bgRecord.blits.length > firstBlits
        && bgRecord.blits.some((args) => args[0] === landed[0]),
        (bgRecord.blits.length - firstBlits) + " tile blits for "
        + firstCount + " arrivals");

    // The burst: arrivals inside the commit window must not
    // re-render the cache - neither on their own arrival nor on a
    // steady frame painted while the burst streams.
    advanceClock(40);
    const stormBlits = bgRecord.blits.length;
    const stormStrokes = bgRecord.strokes.length;
    const mainBlits = record.blits.length;
    const stormCount = landTiles(24);
    MapView.draw();
    check(results, "a burst inside the window re-rasters nothing",
        stormCount === 24 && bgRecord.blits.length === stormBlits
        && bgRecord.strokes.length === stormStrokes,
        stormCount + " arrivals caused "
        + (bgRecord.blits.length - stormBlits) + " tile blits");
    check(results, "the steady frames of the storm stay blit only",
        record.blits.length === mainBlits + 1,
        (record.blits.length - mainBlits) + " main canvas blits");

    // The window closes: the trailing commit lands the whole burst
    // in one raster. The ancestor walk of the far view creates up to
    // three pending entries per position (the chosen level plus the
    // two coarser fallbacks) and draws exactly one of them, so the
    // burst of N arrivals covers at least N / 3 positions.
    advanceClock(260);
    runTimers();
    check(results, "the trailing commit rasterizes the burst once",
        bgRecord.blits.length >= stormBlits + Math.ceil(stormCount / 3)
        && bgRecord.strokes.length > stormStrokes,
        (bgRecord.blits.length - stormBlits) + " tile blits for "
        + stormCount + " arrivals");

    // The cache is stable again: a steady frame paints no static
    // geometry.
    const settledStrokes = bgRecord.strokes.length;
    MapView.draw();
    check(results, "the settled cache re-rasters nothing",
        bgRecord.strokes.length === settledStrokes,
        (bgRecord.strokes.length - settledStrokes) + " new strokes");

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
        ["follow off keeps the view static", runScenarioFollowOff(mapFile)],
        ["map drag keeps the grabbed point", runScenarioMapDrag(mapFile)],
        ["social animation marker", runScenarioSocialMarker(mapFile)],
        ["stable draw order", runScenarioStableOrder(mapFile)],
        ["resting marker", runScenarioRestMarker(mapFile)],
        ["hunting zone", runScenarioHuntingZone(mapFile)],
        ["hunt zones view", runScenarioHuntZonesView(mapFile)],
        ["hunt layer cache", runScenarioHuntLayerCache(mapFile)],
        ["fps meter", runScenarioFpsMeter(mapFile)],
        ["tile load storm", runScenarioTileLoadStorm(mapFile)],
        ["background cache", runScenarioBackgroundCache(mapFile)]
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
