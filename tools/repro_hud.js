#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the HUD rendering of the character status.
//
// It loads the real internal/swarm/webserver/web/app.js into a
// sandboxed context with a stub DOM and checks the reported problems:
//
// - the target panel shows the current target with its name, level
//   chip and HP/MP bars, and reads "no target" for a killed, removed
//   or missing target instead of a dangling "object <id>" reference;
// - the experience bar is fed from expPercent (the bot computes it
//   from the C1 experience table) and renders the fill and the
//   percentage text;
// - the HP and MP bars render cur/max values into the fill and text,
//   and show "—" while the server maximum is unknown;
// - renderHUD no longer writes the target into the character panel
//   (renderTarget owns the target panel).
//
// Usage: node tools/repro_hud.js [--app <app.js>]
// Exit code 0 = HUD rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const DEFAULT_APP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "app.js");

// makeElement returns a DOM element stub recording the last written
// textContent, style and class toggles.
function makeElement() {
    return {
        textContent: "",
        style: {},
        value: "",
        checked: true,
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
        innerHTML: "",
        append: () => {},
        addEventListener: () => {},
        scrollTop: 0,
        scrollHeight: 0,
        children: { length: 0 },
        removeChild: () => {},
        firstChild: null
    };
}

function loadAppJs(appFile) {
    const elements = new Map();
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) {
                elements.set(id, makeElement());
            }

            return elements.get(id);
        },
        createElement: () => makeElement(),
        documentElement: { dataset: {} }
    };
    const sandbox = {
        Math, JSON, Number, Date, isNaN,
        window: { localStorage: {
            getItem: () => null, setItem: () => {}
        } },
        document,
        EventSource: function () {
            this.addEventListener = () => {};
        },
        Event: function () {}
    };
    vm.createContext(sandbox);
    vm.runInContext(fs.readFileSync(appFile, "utf8"), sandbox,
        { filename: "app.js" });
    // Functions missing in an older app.js (the pre fix file has no
    // renderTarget) export as undefined so the checks fail instead of
    // crashing the harness.
    vm.runInContext(
        "globalThis.__hud = {" +
        " renderTarget: typeof renderTarget === 'function'" +
        " ? renderTarget : undefined," +
        " setVital: typeof setVital === 'function'" +
        " ? setVital : undefined," +
        " setExpVital: typeof setExpVital === 'function'" +
        " ? setExpVital : undefined," +
        " renderHUD: typeof renderHUD === 'function'" +
        " ? renderHUD : undefined };",
        sandbox);

    return { hud: sandbox.__hud, elements, sandbox };
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

function snapshotWith(targetId, objects) {
    return {
        id: "acc1",
        character: {
            objectId: 100, name: "test1", classId: 18, race: 1,
            level: 2, x: 45000, y: 50000, z: -3500, heading: 0,
            curHp: 87, maxHp: 100, curMp: 20, maxMp: 30,
            exp: 215, expPercent: 50, sp: 4, targetId, inCombat: false,
            sitting: false,
            load: 0, maxLoad: 0, inventorySlots: 2, inventoryMax: 80,
            adena: 0
        },
        objects: objects || [],
        events: [], status: "online", packets: 1
    };
}

function main() {
    const args = process.argv.slice(2);
    const appIndex = args.indexOf("--app");
    const appFile = appIndex >= 0 ? args[appIndex + 1] : DEFAULT_APP_JS;
    if (!fs.existsSync(appFile)) {
        console.error("app.js not found: " + appFile);
        process.exit(1);
    }
    const { hud, elements, sandbox } = loadAppJs(appFile);
    const results = [];

    if (typeof hud.renderTarget !== "function") {
        console.log("FAIL  renderTarget is missing from app.js");
        process.exit(1);
    }

    // The bot has no target: the panel must read as a status, not as a
    // dangling id.
    hud.renderTarget(snapshotWith(0));
    check(results, "no target id shows the no target status",
        elements.get("target-name").textContent === "no target",
        "got " + JSON.stringify(
            elements.get("target-name").textContent));
    check(results, "no target dims the panel",
        elements.get("hud-target").classList.contains("no-target"),
        "no-target class missing");

    // The target died: the tracker cleared it, but a snapshot that
    // still carries the dead id (for example a race between the kill
    // and the SSE delivery) must not fall back to "object 300".
    const deadTarget = [{
        objectId: 300, kind: "npc", name: "Keltir", level: 2, dead: true
    }];
    hud.renderTarget(snapshotWith(300, deadTarget));
    check(results, "dead target shows the no target status",
        elements.get("target-name").textContent === "no target",
        "got " + JSON.stringify(
            elements.get("target-name").textContent));

    // A living target renders with its name, level chip and vitals.
    const liveTarget = [{
        objectId: 300, kind: "npc", name: "Keltir", level: 2, dead: false,
        curHp: 40, maxHp: 50
    }];
    hud.renderTarget(snapshotWith(300, liveTarget));
    check(results, "living target shows the name",
        elements.get("target-name").textContent === "Keltir",
        "got " + JSON.stringify(
            elements.get("target-name").textContent));
    check(results, "living target shows the level chip",
        elements.get("target-level").textContent === "lv 2"
        && !elements.get("target-level").classList.contains("hidden"),
        "got " + JSON.stringify(
            elements.get("target-level").textContent));
    check(results, "living target shows the hp bar",
        elements.get("target-hp-fill").style.width === "80.0%"
        && elements.get("target-hp-text").textContent === "40/50",
        "got " + JSON.stringify(
            elements.get("target-hp-fill").style.width) + " "
        + JSON.stringify(elements.get("target-hp-text").textContent));

    // The mob mp values never arrive from the C1 server (the
    // StatusUpdate broadcast carries only hp): the mp row shows "—"
    // instead of a bogus 0/0.
    check(results, "unknown target mp reads as unknown",
        elements.get("target-mp-text").textContent === "—",
        "got " + JSON.stringify(
            elements.get("target-mp-text").textContent));

    // The experience bar renders from expPercent.
    sandbox.document.getElementById("xp-fill");
    sandbox.document.getElementById("xp-text");
    if (typeof hud.setExpVital !== "function") {
        check(results, "exp bar fill follows expPercent", false,
            "setExpVital is missing from app.js");
        check(results, "exp bar text shows the percentage", false,
            "setExpVital is missing from app.js");
    } else {
        hud.setExpVital(34.56);
        check(results, "exp bar fill follows expPercent",
            elements.get("xp-fill").style.width === "34.6%",
            "got " + JSON.stringify(
                elements.get("xp-fill").style.width));
        check(results, "exp bar text shows the percentage",
            elements.get("xp-text").textContent === "34.6%",
            "got " + JSON.stringify(
                elements.get("xp-text").textContent));
    }

    // The HP/MP bars render cur/max, or "—" while unknown.
    sandbox.document.getElementById("hp-fill");
    sandbox.document.getElementById("hp-text");
    if (typeof hud.setVital !== "function") {
        check(results, "hp bar fill and text", false,
            "setVital is missing from app.js");
    } else {
        hud.setVital("hp", 87, 100, true);
        check(results, "hp bar fill and text",
            elements.get("hp-fill").style.width === "87.0%"
            && elements.get("hp-text").textContent === "87/100",
            "got " + JSON.stringify(
                elements.get("hp-fill").style.width)
            + " " + JSON.stringify(
                elements.get("hp-text").textContent));
        hud.setVital("hp", 0, 0, false);
        check(results, "unknown hp reads as unknown",
            elements.get("hp-text").textContent === "—",
            "got " + JSON.stringify(
                elements.get("hp-text").textContent));
    }

    // renderHUD on a full snapshot works without the removed facing
    // element (it must not touch pos-heading at all) and leaves the
    // target panel to renderTarget: no duplication of the target row
    // in the character card.
    sandbox.document.getElementById("pos-heading");
    sandbox.document.getElementById("target-name");
    const before = elements.get("pos-heading").textContent;
    elements.get("target-name").textContent = "SENTINEL";
    if (typeof hud.renderHUD !== "function") {
        check(results, "renderHUD fills the exp bar from expPercent",
            false, "renderHUD is missing from app.js");
        check(results, "renderHUD no longer touches the facing field",
            false, "renderHUD is missing from app.js");
        check(results, "renderHUD leaves the target panel to renderTarget",
            false, "renderHUD is missing from app.js");
    } else {
        hud.renderHUD(snapshotWith(300, liveTarget));
        check(results, "renderHUD fills the exp bar from expPercent",
            elements.get("xp-fill").style.width === "50.0%",
            "got " + JSON.stringify(
                elements.get("xp-fill").style.width));
        check(results, "renderHUD no longer touches the facing field",
            elements.get("pos-heading").textContent === before,
            "facing field was written");
        check(results, "renderHUD leaves the target panel to renderTarget",
            elements.get("target-name").textContent === "SENTINEL",
            "renderHUD wrote the target name");
    }

    let failed = 0;
    for (const result of results) {
        const mark = result.ok ? "PASS" : "FAIL";
        console.log(mark + "  " + result.name);
        if (!result.ok) {
            failed++;
            console.log("      " + result.detail);
        }
    }
    console.log(failed === 0 ? "ALL PASS" : failed + " CHECKS FAILED");

    process.exit(failed === 0 ? 0 : 1);
}

main();
