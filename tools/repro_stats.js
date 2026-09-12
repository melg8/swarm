#!/usr/bin/env node
/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Reproduction harness for the statistics tab of the web UI.
//
// It loads the real internal/swarm/webserver/web/stats.js into a
// sandboxed context with a stub DOM (recording canvases) and a stub
// fetch, then checks:
//
// - the script loads without any top level DOM access (the vm
//   sandbox has none of it, a load time call throws before the
//   harness can act);
// - activating the tab fetches /api/stats and renders the KPI cards,
//   the charts (the recording 2d contexts received draw calls), the
//   bots comparison table and the bot selector options;
// - selecting a bot fetches /api/stats/{id} and renders the detail
//   view: the KPI cards, the events timeline, the phase distribution;
// - switching the window refetches with the new query parameter;
// - deactivating the tab stops the polling timer.
//
// Usage: node tools/repro_stats.js [--stats <stats.js>]
// Exit code 0 = the statistics tab renders correctly, 1 = bug
// reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const DEFAULT_STATS_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "stats.js");

// makeElement returns a DOM element stub recording the written
// textContent and the child operations.
function makeElement() {
    return {
        textContent: "",
        style: {},
        value: "",
        checked: false,
        children: [],
        dataset: {},
        appendChild: function (child) {
            this.children.push(child);

            return child;
        },
        removeChild: function (child) {
            const at = this.children.indexOf(child);
            if (at >= 0) { this.children.splice(at, 1); }
        },
        get firstChild() {
            return this.children[0] || null;
        },
        classList: {
            _classes: new Set(),
            contains(cls) { return this._classes.has(cls); },
            add(cls) { this._classes.add(cls); },
            remove(cls) { this._classes.delete(cls); },
            toggle(cls, force) {
                const on = force === undefined
                    ? !this._classes.has(cls) : Boolean(force);
                if (on) { this._classes.add(cls); } else {
                    this._classes.delete(cls);
                }

                return on;
            }
        },
        addEventListener: () => {},
        clientWidth: 320,
        clientHeight: 130
    };
}

// makeCanvas returns a canvas stub whose 2d context records every
// draw call.
function makeCanvas() {
    const calls = [];
    const ctx = {
        _calls: calls,
        setTransform: () => {}, save: () => {}, restore: () => {},
        clearRect: (...a) => calls.push(["clear", ...a]),
        beginPath: () => calls.push(["begin"]),
        closePath: () => calls.push(["close"]),
        moveTo: (...a) => calls.push(["move", ...a]),
        lineTo: (...a) => calls.push(["line", ...a]),
        stroke: () => calls.push(["stroke"]),
        fill: () => calls.push(["fill"]),
        fillRect: (...a) => calls.push(["rect", ...a]),
        fillText: (...a) => calls.push(["text", ...a]),
        font: "", textAlign: "", lineWidth: 1, fillStyle: "",
        strokeStyle: "", globalAlpha: 1
    };
    const canvas = makeElement();
    canvas.getContext = (kind) => (kind === "2d" ? ctx : null);
    canvas.width = 0;
    canvas.height = 0;

    return canvas;
}

// fleetPayload builds the /api/stats fixture: two bots (one long
// running online, one acceptance), a five point history.
function fleetPayload() {
    return {
        nowMs: 1770000000000,
        samplePeriodSec: 15,
        collectionSec: 3600,
        process: {
            uptimeSec: 3600, goroutines: 42, heapMB: 12.5, sysMB: 40.25,
            numGC: 7, lastGCPauseMs: 0.4
        },
        fleet: {
            registered: 1, online: 1, kills: 57, deaths: 2, rejoins: 1,
            expGained: 43000, killsPerHour: 57, avgTickMs: 1.2,
            packetRate: 33.5, hitRate: 0.86
        },
        bots: [
            {
                id: "test1", name: "Test1", kind: "", status: "online",
                phase: "engage", online: true, inCombat: true, level: 12,
                levelGained: 1, expGained: 43000, expPercent: 62.5,
                kills: 57, deaths: 2, kd: 28.5, killsPerHour: 57,
                deathsPerHour: 2, sessions: 2, rejoins: 1, swingsMade: 300,
                swingsLanded: 258, swingsTaken: 120, hitRate: 0.86,
                damageTaken: 4500.5, avgTickMs: 1.2, maxTickMs: 25.5,
                packetRate: 33.5, hpPercent: 78.5, adena: 13162,
                uptimeSec: 3600, lastKillAgoSec: 5, lastDeathAgoSec: 1800
            },
            {
                id: "temp1", name: "Temp1", kind: "acceptance",
                status: "online", phase: "idle", online: true,
                inCombat: false, level: 3, levelGained: 0, expGained: 400,
                expPercent: 10, kills: 2, deaths: 0, kd: 2,
                killsPerHour: 2, deathsPerHour: 0, sessions: 1, rejoins: 0,
                swingsMade: 10, swingsLanded: 10, swingsTaken: 0, hitRate: 1,
                damageTaken: 0, avgTickMs: 0, maxTickMs: 0, packetRate: 4,
                hpPercent: 100, adena: 0, uptimeSec: 120,
                lastKillAgoSec: 30, lastDeathAgoSec: 0
            }
        ],
        history: {
            at: [1769996400, 1769997300, 1769998200, 1769999100, 1770000000],
            online: [1, 1, 0, 1, 1],
            registered: [1, 1, 1, 1, 1],
            kills: [0, 12, 20, 40, 57],
            deaths: [0, 0, 1, 1, 2],
            killsPerMin: [0, 0.8, 0.5, 1.3, 1.1],
            deathsPerMin: [0, 0, 0.05, 0, 0.05],
            expGained: [0, 9000, 15000, 30000, 43000],
            avgTickMs: [1.1, 1.2, 1.0, 1.3, 1.2],
            packetRate: [30, 33, 0, 35, 33.5],
            heapMB: [10, 11, 12, 12.4, 12.5],
            sysMB: [38, 39, 40, 40.1, 40.25],
            goroutines: [40, 41, 38, 42, 42],
            numGC: [0, 2, 4, 6, 7]
        }
    };
}

// botPayload builds the /api/stats/test1 fixture.
function botPayload() {
    return {
        id: "test1", name: "Test1", kind: "", status: "online",
        phase: "engage", online: true, inCombat: true, level: 12,
        levelGained: 1, expGained: 43000, expPercent: 62.5, kills: 57,
        deaths: 2, kd: 28.5, killsPerHour: 57, deathsPerHour: 2, sessions: 2,
        rejoins: 1, swingsMade: 300, swingsLanded: 258, swingsTaken: 120,
        hitRate: 0.86, damageTaken: 4500.5, avgTickMs: 1.2, maxTickMs: 25.5,
        packetRate: 33.5, hpPercent: 78.5, adena: 13162, uptimeSec: 3600,
        lastKillAgoSec: 5, lastDeathAgoSec: 1800,
        phases: [
            { phase: "engage", seconds: 1800 },
            { phase: "loot", seconds: 900 },
            { phase: "townWalk", seconds: 400 }
        ],
        events: [
            { atMs: 1769996800000, kind: "online", value: 0 },
            { atMs: 1769997300000, kind: "level", value: 12 },
            { atMs: 1769998200000, kind: "death", value: 1 },
            { atMs: 1769998600000, kind: "relogin", value: 1 },
            { atMs: 1769999900000, kind: "kill", value: 3 }
        ],
        history: {
            at: [1769996400, 1769997300, 1769998200, 1769999100, 1770000000],
            exp: [100000, 109000, 115000, 130000, 143000],
            expGained: [0, 9000, 15000, 30000, 43000],
            level: [11, 11, 12, 12, 12],
            hpPercent: [-1, 90, 0, 60, 78.5],
            adena: [2000, 5000, 7000, 11000, 13162],
            kills: [0, 12, 20, 40, 57],
            deaths: [0, 0, 1, 1, 2],
            killsPerMin: [0, 0.8, 0.5, 1.3, 1.1],
            avgTickMs: [1.1, 1.2, 1.0, 1.3, 1.2],
            packetRate: [30, 33, 0, 35, 33.5],
            phase: ["", "engage", "loot", "engage", "engage"]
        }
    };
}

// loadStatsJs loads stats.js into the vm sandbox with the recording
// stubs and the scripted fetch.
function loadStatsJs(statsFile) {
    const elements = new Map();
    const canvasIds = new Set();
    const fetches = [];
    const timers = { started: 0, cleared: 0 };
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) {
                const el = id.startsWith("chart-") || id === "bot-phase-strip"
                    ? makeCanvas() : makeElement();
                if (id.startsWith("chart-") || id === "bot-phase-strip") {
                    canvasIds.add(id);
                }
                elements.set(id, el);
            }

            return elements.get(id);
        },
        createElement: () => makeElement(),
        documentElement: {}
    };
    const sandbox = {
        Math, JSON, Number, Date, isNaN, encodeURIComponent,
        devicePixelRatio: 1,
        document,
        setInterval: () => { timers.started++; return 1; },
        clearInterval: () => { timers.cleared++; },
        fetch: async (url) => {
            fetches.push(String(url));
            if (String(url).indexOf("/api/stats/") === 0) {
                return { ok: true, json: async () => botPayload() };
            }

            return { ok: true, json: async () => fleetPayload() };
        }
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(statsFile, "utf8"), sandbox,
        { filename: "stats.js" });
    vm.runInContext(
        "globalThis.__stats = {" +
        " StatsTab: typeof StatsTab === 'undefined' ? undefined : StatsTab," +
        " select: typeof statsSelectBot === 'function'" +
        " ? statsSelectBot : undefined," +
        " close: typeof statsCloseBot === 'function'" +
        " ? statsCloseBot : undefined," +
        " setWindow: typeof statsSetWindow === 'function'" +
        " ? statsSetWindow : undefined };",
        sandbox);

    return { stats: sandbox.__stats, elements, canvasIds, fetches, timers };
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

async function settle() {
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
}

async function main() {
    const args = process.argv.slice(2);
    const statsIndex = args.indexOf("--stats");
    const statsFile = statsIndex >= 0 ? args[statsIndex + 1] : DEFAULT_STATS_JS;
    if (!fs.existsSync(statsFile)) {
        console.error("stats.js not found: " + statsFile);
        process.exit(1);
    }
    const { stats, elements, canvasIds, fetches, timers } =
        loadStatsJs(statsFile);
    const results = [];

    if (typeof stats.StatsTab !== "object" ||
        typeof stats.StatsTab.init !== "function" ||
        typeof stats.StatsTab.setActive !== "function") {
        console.log("FAIL  StatsTab is missing or incomplete in stats.js");
        process.exit(1);
    }

    // Activating the tab polls the fleet endpoint.
    stats.StatsTab.setActive(true);
    await settle();
    check(results, "activation fetches the fleet view",
        fetches.some((u) => u.indexOf("/api/stats?window=") === 0),
        "no fleet fetch recorded");
    check(results, "activation starts the polling timer",
        timers.started === 1, "interval not started");

    // The KPI cards rendered.
    check(results, "fleet KPI cards render",
        elements.get("stats-kpis").children.length >= 10,
        "expected at least ten KPI cards, got " +
        elements.get("stats-kpis").children.length);

    // The charts received draw calls.
    let drawCalls = 0;
    for (const id of canvasIds) {
        const canvas = elements.get(id);
        if (id === "bot-phase-strip") { continue; }
        drawCalls += canvas.getContext("2d")._calls.length;
    }
    check(results, "fleet charts draw",
        drawCalls > 20, "the canvas contexts saw no drawing, got " + drawCalls);

    // The bots table and the selector options.
    const table = elements.get("stats-bots-table");
    // thead holds a tr of th cells; tbody holds the bot rows.
    const thead = table.children[0];
    const tbody = table.children[1];
    const headerCells = thead ? thead.children[0].children.length : 0;
    check(results, "bots table builds the header and rows",
        table.children.length === 2 && !!tbody && headerCells === 14 &&
        tbody.children.length === 2,
        "expected thead with 14 columns and two rows, got " +
        table.children.length + " sections, " + headerCells +
        " columns, " + (tbody ? tbody.children.length : 0) + " rows");
    const options = elements.get("stats-bot-select").children;
    check(results, "bot selector lists every bot",
        options.length === 3, "expected three options (placeholder + 2)");

    // The updated clock.
    check(results, "update clock reports",
        elements.get("stats-updated").textContent.indexOf("updated") === 0,
        "clock text is " + elements.get("stats-updated").textContent);

    // Selecting a bot renders the detail view.
    stats.select("test1");
    await settle();
    check(results, "bot selection fetches the bot view",
        fetches.some((u) => u.indexOf("/api/stats/test1") === 0),
        "no bot fetch recorded");
    check(results, "bot detail panel opens",
        !elements.get("stats-bot").classList.contains("hidden"),
        "the panel stayed hidden");
    check(results, "bot KPI cards render",
        elements.get("stats-bot-kpis").children.length >= 10,
        "expected at least ten bot KPI cards");
    check(results, "bot events render",
        elements.get("stats-events").children.length === 5,
        "expected five event rows, got " +
        elements.get("stats-events").children.length);
    check(results, "phase distribution renders",
        elements.get("stats-phase-shares").children.length === 3,
        "expected three phase rows");
    check(results, "phase timeline draws",
        elements.get("bot-phase-strip").getContext("2d")._calls.length > 4,
        "the phase strip saw no drawing");

    // The window switch refetches with the new parameter.
    stats.setWindow(3600);
    await settle();
    check(results, "window switch refetches",
        fetches.some((u) => u.indexOf("window=3600") >= 0),
        "no refetch with the new window");

    // Closing the bot detail.
    stats.close();
    check(results, "bot detail panel closes",
        elements.get("stats-bot").classList.contains("hidden"),
        "the panel stayed visible");

    // Deactivation stops the timer.
    stats.StatsTab.setActive(false);
    check(results, "deactivation stops the timer",
        timers.cleared >= 1, "the interval was not cleared");

    const failed = results.filter((r) => !r.ok);
    for (const r of results) {
        console.log((r.ok ? "ok    " : "FAIL  ") + r.name +
            (r.ok ? "" : " - " + r.detail));
    }
    console.log(results.length - failed.length + "/" + results.length +
        " checks passed");
    if (failed.length) { process.exit(1); }
}

main().catch((err) => {
    console.error("harness crashed: " + err.stack);
    process.exit(1);
});
