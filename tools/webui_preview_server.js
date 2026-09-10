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
