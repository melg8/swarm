#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Static preview server for the swarm web UI: serves the real web
// directory with stub bot APIs so a headless browser can drive the
// map layers (spots, social links, fleet kills, hover) without a
// running bot process. The synthetic snapshot is injected from the
// browser side through MapView.update (see scripts below).
//
// Usage: node tools/webui_preview_server.js [port]

"use strict";

const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");

const WEB = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web");
const PORT = Number(process.argv[2] || 8093);

const MIME = {
    ".html": "text/html; charset=utf-8",
    ".js": "text/javascript; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".jpg": "image/jpeg",
    ".png": "image/png",
    ".svg": "image/svg+xml",
    ".ico": "image/x-icon"
};

// previewStatsSeries builds a synthetic 4 hour fleet history: 48
// points at a 5 minute cadence.
function previewStatsSeries() {
    const at = [];
    const online = [];
    const registered = [];
    const kills = [];
    const deaths = [];
    const killsPerMin = [];
    const deathsPerMin = [];
    const expGained = [];
    const avgTickMs = [];
    const packetRate = [];
    const heapMB = [];
    const sysMB = [];
    const goroutines = [];
    const numGC = [];
    const now = Math.floor(Date.now() / 1000);
    let killTotal = 0;
    let deathTotal = 0;
    let expTotal = 0;
    for (let i = 48; i >= 0; i--) {
        at.push(now - i * 300);
        online.push(2 + (i % 3 === 0 ? 1 : 0));
        registered.push(3);
        const k = 3 + (i % 5);
        killTotal += k;
        kills.push(killTotal);
        killsPerMin.push(k / 5);
        if (i % 9 === 0) { deathTotal += 1; }
        deaths.push(deathTotal);
        deathsPerMin.push(i % 9 === 0 ? 0.2 : 0);
        expTotal += 800 + k * 130;
        expGained.push(expTotal);
        avgTickMs.push(0.8 + (i % 7) * 0.1);
        packetRate.push(28 + (i % 4) * 3);
        heapMB.push(30 + (i % 6));
        sysMB.push(60 + (i % 5) * 2);
        goroutines.push(40 + (i % 8));
        numGC.push(Math.floor(i / 3));
    }

    return {
        at, online, registered, kills, deaths, killsPerMin, deathsPerMin,
        expGained, avgTickMs, packetRate, heapMB, sysMB, goroutines, numGC
    };
}

// previewStatsPayload builds the fleet statistics stub.
function previewStatsPayload() {
    return {
        nowMs: Date.now(), samplePeriodSec: 15, collectionSec: 14400,
        process: {
            uptimeSec: 14400, goroutines: 43, heapMB: 33.5, sysMB: 64,
            numGC: 16, lastGCPauseMs: 0.3
        },
        fleet: {
            registered: 3, online: 3, kills: 178, deaths: 6, rejoins: 4,
            expGained: 68000, killsPerHour: 44.5, avgTickMs: 1.05,
            packetRate: 87, hitRate: 0.84
        },
        bots: [
            previewBotStatsPayload("bot1"),
            previewBotStatsPayload("bot2"),
            previewBotStatsPayload("bot3")
        ],
        history: previewStatsSeries()
    };
}

// previewBotStatsPayload builds the per bot statistics stub.
function previewBotStatsPayload(id) {
    const seed = id.length + (id.charCodeAt(4) || 1);
    const series = previewStatsSeries();
    const phases = ["engage", "engage", "loot", "engage", "townWalk",
        "engage", "loot", "engage", "engage", "delevel",
        "engage", "loot", "engage", "townSell", "townReturn",
        "engage", "engage", "loot", "engage", "engage"];
    const events = [];
    for (let i = 0; i < 12; i++) {
        events.push({
            atMs: Date.now() - (12 - i) * 900000,
            kind: i % 5 === 0 ? "kill" : (i % 5 === 1 ? "level" : "kill"),
            value: i % 5 === 0 ? 1 : i
        });
    }
    events.push({ atMs: Date.now() - 40000, kind: "kill", value: 2 });
    events.push({ atMs: Date.now() - 3600000, kind: "death", value: 1 });
    events.push({ atMs: Date.now() - 3550000, kind: "relogin", value: 1 });

    return {
        id: id, name: id + "-char", kind: "", status: "online",
        phase: "engage", online: true, inCombat: true, level: 8 + seed,
        levelGained: 2, expGained: 6000 * seed, expPercent: 45.5,
        kills: 50 * seed, deaths: 2 * seed, kd: 25,
        killsPerHour: 12 * seed, deathsPerHour: 0.5, sessions: 1 + seed,
        rejoins: seed, swingsMade: 400 * seed, swingsLanded: 336 * seed,
        swingsTaken: 120 * seed, hitRate: 0.84, damageTaken: 9000.5,
        avgTickMs: 1.05, maxTickMs: 12.5, packetRate: 29, hpPercent: 76.5,
        adena: 4200 * seed, uptimeSec: 14400, lastKillAgoSec: 40,
        lastDeathAgoSec: 3600,
        phases: [
            { phase: "engage", seconds: 9000 },
            { phase: "loot", seconds: 2400 },
            { phase: "townWalk", seconds: 1200 },
            { phase: "townSell", seconds: 600 },
            { phase: "townReturn", seconds: 1100 },
            { phase: "delevel", seconds: 100 }
        ],
        events: events,
        history: {
            at: series.at, exp: series.expGained.map((v) => 100000 + v),
            expGained: series.expGained, level: series.at.map(() => 8),
            hpPercent: series.at.map((_, i) => 40 + (i % 10) * 5),
            adena: series.expGained.map((v) => v / 2),
            kills: series.kills, deaths: series.deaths,
            killsPerMin: series.killsPerMin, avgTickMs: series.avgTickMs,
            packetRate: series.packetRate,
            phase: series.at.map((_, i) => phases[i % phases.length])
        }
    };
}

const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://localhost");
    if (url.pathname === "/api/config") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ mode: "bot" }) + "\n");

        return;
    }
    if (url.pathname === "/api/bots") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end("[]\n");

        return;
    }
    if (url.pathname === "/api/fleet/kills") {
        // The synthetic fleet kill ring: fresh and aging marks around
        // the elven lands demo ground of the injected snapshot.
        const now = Date.now();
        const marks = [
            { botId: "bot1", x: 46050, y: 45050, atMs: now - 30000 },
            { botId: "bot2", x: 45950, y: 44950, atMs: now - 120000 },
            { botId: "bot1", x: 46200, y: 45150, atMs: now - 240000 },
            { botId: "bot3", x: 45800, y: 45100, atMs: now - 200000 }
        ];
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(marks) + "\n");

        return;
    }
    if (url.pathname === "/api/stats") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(previewStatsPayload()) + "\n");

        return;
    }
    if (url.pathname.startsWith("/api/stats/")) {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(previewBotStatsPayload(
            decodeURIComponent(url.pathname.slice("/api/stats/".length))
        )) + "\n");

        return;
    }
    if (url.pathname === "/api/proxy") {
        res.writeHead(404, { "Content-Type": "application/json" });
        res.end("{\"error\":\"no proxy\"}\n");

        return;
    }
    let file = path.join(WEB, url.pathname === "/" ? "index.html"
        : url.pathname);
    if (!file.startsWith(WEB)) {
        res.writeHead(403);
        res.end("forbidden");

        return;
    }
    fs.readFile(file, (err, data) => {
        if (err) {
            res.writeHead(404);
            res.end("not found");

            return;
        }
        res.writeHead(200, {
            "Content-Type": MIME[path.extname(file)]
                || "application/octet-stream"
        });
        res.end(data);
    });
});

server.listen(PORT, "127.0.0.1", () => {
    console.log("webui preview on http://127.0.0.1:" + PORT);
});
