#!/usr/bin/env node
/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Reproduction harness for the effects panel (the buff list).
//
// It loads the real internal/swarm/webserver/web/buffs.js and
// web/app.js into a sandboxed context with a stub DOM and checks:
//
// - the markup: the panel opens in the icon grid view (the default),
//   the frame carries the left dock (the expand chevron plus the
//   view switch) and the two content layers, the EFFECTS head label
//   is gone and buffs.js loads before app.js;
// - the styles: the grid packs 32px cells 10 columns wide with 2px
//   white separator gaps, the detailed list scrolls (the thin
//   scrollbar) and caps at the HUD stack height variable, the view
//   morphs animate (the frame width, the body height, the layer
//   cross-fade, the chevron rotation) and the pathfind mode still
//   hides the panel;
// - the render: exactly the active effects show (two buffs build two
//   cells and two rows), the keyed cells keep their DOM nodes across
//   the snapshots (the icons must not blink), a buff leaving the
//   list drops only its own node, a joining one pops in;
// - the detailed list rows carry the name, the level, the countdown
//   and the remaining time percent bar (left over total), a zero
//   total pins the bar empty;
// - the countdown ticker: the anchored readings count the elapsed
//   wall clock off (a reading 61 seconds old at 120 left reads 59s
//   and turns the fading mark on in both views);
// - the view toggle: both dock buttons flip the view both ways, the
//   choice persists in the localStorage and the panel body height
//   follows the active view (the grid rows, the list capped);
// - the empty snapshot hides the panel and wipes both views;
// - the remaining time formatters keep the pinned readings.
//
// Usage: node tools/repro_buffs.js [--app <app.js>]
//
// The harness exits 0 when every check passes, 1 otherwise (a FAIL
// line names the broken check).

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const WEB_DIR = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web");
const DEFAULT_APP = path.join(WEB_DIR, "app.js");

let failures = 0;

// check records one check result: a FAIL line names the broken check
// and flips the exit code.
function check(name, ok, detail) {
    if (ok) {
        console.log("PASS " + name);

        return;
    }
    failures += 1;
    console.log("FAIL " + name + (detail ? " - " + detail : ""));
}

// makeElement returns a DOM element stub recording children and
// event listeners. append mirrors the real DOM: appending an
// existing child moves it to the end, it never duplicates.
function makeElement() {
    return {
        textContent: "",
        style: {},
        value: "",
        src: "",
        alt: "",
        title: "",
        className: "",
        children: [],
        listeners: {},
        offsetHeight: 0,
        scrollHeight: 0,
        addEventListener(type, handler) {
            if (!this.listeners[type]) { this.listeners[type] = []; }
            this.listeners[type].push(handler);
        },
        setAttribute(key, value) {
            (this.attributes || (this.attributes = {}))[key] = value;
        },
        getAttribute(key) {
            return (this.attributes || {})[key];
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
        _innerHTML: "",
        get innerHTML() { return this._innerHTML; },
        set innerHTML(value) {
            this._innerHTML = value;
            this.children.length = 0;
        },
        remove() {
            if (this._parent) {
                const at = this._parent.children.indexOf(this);
                if (at >= 0) { this._parent.children.splice(at, 1); }
            }
        },
        append(...added) {
            for (const child of added) {
                const at = this.children.indexOf(child);
                if (at >= 0) { this.children.splice(at, 1); }
                this.children.push(child);
                child._parent = this;
            }
        }
    };
}

// fire dispatches one stub DOM event on an element that records its
// listeners.
function fire(element, type) {
    const handlers = (element.listeners || {})[type] || [];
    for (const handler of handlers) { handler({}); }
}

function loadPanel(appFile) {
    const elements = new Map();
    const storage = new Map();
    const timers = { intervals: [], timeouts: [] };
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) { elements.set(id, makeElement()); }

            return elements.get(id);
        },
        // The HUD stack stays unmeasurable here (no .hud-stack
        // element): the panel answers with its height fallbacks.
        querySelector: () => null,
        createElement: () => makeElement(),
        documentElement: { dataset: {} },
        listeners: {},
        addEventListener(type, handler) {
            if (!this.listeners[type]) { this.listeners[type] = []; }
            this.listeners[type].push(handler);
        }
    };
    const sandbox = {
        Math, JSON, Number, isNaN, Set, Map, Object, String,
        document,
        console,
        window: {
            localStorage: {
                getItem: (key) => (storage.has(key)
                    ? storage.get(key) : null),
                setItem: (key, value) => storage.set(key, String(value))
            },
            addEventListener: () => {},
            // The timers record without running: the harness fires
            // the countdown tick by hand.
            setInterval: (fn) => { timers.intervals.push(fn); },
            setTimeout: (fn) => { timers.timeouts.push(fn); }
        },
        fetch: () => ({ catch: () => {} }),
        EventSource: function () {
            this.addEventListener = () => {};
        },
        Event: function () {}
    };
    vm.createContext(sandbox);
    // The effects panel module loads before app.js (the same order
    // the page declares): renderSnapshot calls its renderBuffs.
    vm.runInContext(fs.readFileSync(path.join(WEB_DIR, "buffs.js"),
        "utf8"), sandbox, { filename: "buffs.js" });
    vm.runInContext(fs.readFileSync(appFile, "utf8"), sandbox,
        { filename: "app.js" });
    vm.runInContext(
        "globalThis.__buffs = {" +
        " panel: typeof renderBuffs === 'function' ? renderBuffs : undefined," +
        " init: typeof initBuffsPanel === 'function'" +
        "  ? initBuffsPanel : undefined," +
        " tick: typeof tickBuffsCountdowns === 'function'" +
        "  ? tickBuffsCountdowns : undefined," +
        " setView: typeof setBuffsView === 'function'" +
        "  ? setBuffsView : undefined," +
        " syncSize: typeof syncBuffsPanelSize === 'function'" +
        "  ? syncBuffsPanelSize : undefined," +
        " state: typeof BuffsPanel === 'undefined' ? undefined : BuffsPanel," +
        " leftText: typeof buffLeftText === 'function'" +
        "  ? buffLeftText : undefined," +
        " leftShort: typeof buffLeftShort === 'function'" +
        "  ? buffLeftShort : undefined };",
        sandbox);

    return { api: sandbox.__buffs, elements, storage, timers, sandbox,
        doc: document };
}

// readArg answers the --app override of the app.js path.
function readArg() {
    const at = process.argv.indexOf("--app");

    return at >= 0 ? process.argv[at + 1] : DEFAULT_APP;
}

// ---- static file checks ----

function checkMarkup() {
    const html = fs.readFileSync(path.join(WEB_DIR, "index.html"),
        "utf8");
    check("markup: the EFFECTS head label is gone",
        !html.includes(">EFFECTS<"));
    check("markup: the panel keeps the server effect list tooltip",
        html.includes("the buffs and the debuffs of the server effect list"));
    check("markup: the panel opens in the icon grid view",
        html.includes('class="buffs-panel hidden view-icons"'));
    check("markup: the left dock with the expand chevron exists",
        html.includes('id="buffs-toggle"') &&
        html.includes('id="buffs-chev"'));
    check("markup: the view switch icon exists",
        html.includes('id="buffs-view-btn"'));
    check("markup: both content layers exist",
        html.includes('id="buffs-grid"') &&
        html.includes('id="buffs-list-wrap"') &&
        html.includes('id="buffs-list"'));
    const buffsAt = html.indexOf('<script src="buffs.js">');
    const appAt = html.indexOf('<script src="app.js">');
    check("markup: buffs.js loads before app.js",
        buffsAt >= 0 && appAt > buffsAt);
}

function checkStyles() {
    const css = fs.readFileSync(path.join(WEB_DIR, "style.css"), "utf8");
    check("styles: the grid packs 10 columns of 32px cells",
        css.includes("grid-template-columns: repeat(10, 32px)") &&
        css.includes("grid-auto-rows: 32px"));
    check("styles: the cells paint the white separator strips",
        /\.buff-cell \{[^}]*border-right: 2px solid #ffffff;/s.test(css) &&
        /\.buff-cell \{[^}]*border-bottom: 2px solid #ffffff;/s.test(css) &&
        /\.buffs-grid \{[^}]*gap: 0;/s.test(css));
    check("styles: the detailed list scrolls with the thin scrollbar",
        /\.buffs-list-wrap \{[^}]*overflow-y: auto;/s.test(css) &&
        css.includes(".buffs-list-wrap::-webkit-scrollbar { width: 6px; }"));
    check("styles: the morph animates the frame width and the body height",
        /\.buffs-panel \{[^}]*transition: width/s.test(css) &&
        /\.buffs-panel-body \{[^}]*transition: height/s.test(css));
    check("styles: the layers cross-fade with the settle",
        css.includes(".view-list .buffs-grid") &&
        css.includes(".view-icons .buffs-list-wrap") &&
        /visibility 0s linear 0\.3s/.test(css));
    check("styles: the chevron rotates in the list view",
        css.includes(".buffs-panel.view-list .buffs-chev") &&
        css.includes("rotate(180deg)"));
    check("styles: the rows carry the remaining time percent bar",
        css.includes(".buff-bar") && /\.buff-bar i \{[^}]*transition: width/s
            .test(css));
    check("styles: the enter pop-in never re-triggers on refreshes",
        css.includes(".buff-cell.buff-enter") &&
        css.includes(".buff-item.buff-enter"));
    check("styles: the pathfind mode still hides the panel",
        css.includes("body.mode-pathfind .buffs-panel { display: none; }"));
}

// ---- behavior checks ----

// buff builds one snapshot entry: the shape the server encodes.
function buff(skillId, level, left, total, name, icon) {
    return { skillId, level, left, total, name, icon };
}

function checkBehavior(harness) {
    const { api, elements, storage, timers } = harness;

    api.init();
    // el touches the same auto-creating getElementById the panel
    // uses: the stub elements exist from the first reference on.
    const el = (id) => {
        harness.doc.getElementById(id);

        return elements.get(id);
    };
    const panel = () => el("buffs-panel");
    const grid = () => el("buffs-grid");
    const body = () => el("buffs-panel-body");
    const list = () => el("buffs-list");

    check("init: the restored view is the icon grid (the default)",
        api.state.view === "icons" &&
        panel().classList.contains("view-icons"));

    const two = [
        buff(91, 1, 1200, 1200, "Defense Aura", "skill0091"),
        buff(77, 2, 600, 1200, "Attack Aura", "skill0077")
    ];
    api.panel({ buffs: two });
    check("render: two buffs build exactly two cells and two rows",
        grid().children.length === 2 && list().children.length === 2);
    check("render: the panel unhides", !panel().classList.contains("hidden"));
    harness.flushTimeouts();
    check("render: the joining cells drop the enter class after the pop",
        grid().children.every((cell) => !cell.classList.contains("buff-enter")));

    const cell91 = api.state.cells.get(91).item;
    const row91 = api.state.rows.get(91).item;
    api.panel({ buffs: [
        buff(91, 1, 1190, 1200, "Defense Aura", "skill0091"),
        buff(77, 2, 590, 1200, "Attack Aura", "skill0077")
    ] });
    harness.flushTimeouts();
    check("keyed: the refresh rewrites the countdown in place",
        api.state.cells.get(91).item === cell91 &&
        api.state.rows.get(91).item === row91 &&
        api.state.cells.get(91).left.textContent === "19m");
    check("keyed: the row keeps the name and the level",
        api.state.rows.get(91).name.textContent === "Defense Aura" &&
        api.state.rows.get(91).level.textContent === "1");

    api.panel({ buffs: [
        buff(91, 1, 1190, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    harness.flushTimeouts();
    check("keyed: the expired buff drops only its own node",
        grid().children.length === 2 && list().children.length === 2 &&
        api.state.cells.get(91).item === cell91 &&
        !api.state.cells.has(77) && api.state.cells.has(105));

    api.panel({ buffs: [
        buff(91, 1, 600, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    harness.flushTimeouts();
    check("list: the percent bar answers the remaining share",
        api.state.rows.get(91).fill.style.width === "50%");
    check("list: a zero total pins the bar empty", (() => {
        api.panel({ buffs: [buff(7, 1, 100, 0, "Mystery", "skill0007")] });
        harness.flushTimeouts();
        const width = api.state.rows.get(7).fill.style.width;

        return width === "0%";
    })());

    api.panel({ buffs: [
        buff(91, 1, 600, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    harness.flushTimeouts();
    check("grid: the short countdown overlays the cell",
        api.state.cells.get(91).left.textContent === "10m");

    // The countdown ticker: age the anchor 61 seconds into a 120
    // second reading, both views answer 59s and the fading mark.
    const aged = [buff(91, 1, 120, 1200, "Defense Aura", "skill0091")];
    api.panel({ buffs: aged });
    harness.flushTimeouts();
    api.state.anchors.get(91).at = Date.now() - 61000;
    api.setView("icons");
    api.tick();
    check("tick: the grid countdown counts the wall clock off",
        api.state.cells.get(91).left.textContent === "59s" &&
        api.state.cells.get(91).left.classList.contains("fading"));
    api.setView("list");
    api.tick();
    check("tick: the list countdown and the fading mark follow",
        api.state.rows.get(91).meta.textContent === "59s" &&
        api.state.rows.get(91).meta.classList.contains("fading") &&
        parseFloat(api.state.rows.get(91).fill.style.width) < 5.1);
    check("tick: the list body caps at the fallback (no HUD answer)",
        body().style.height === (1 * 36 + 6) + "px");

    // The dock buttons: both toggle, both ways, the choice persists.
    const toggle = el("buffs-toggle");
    const viewBtn = el("buffs-view-btn");
    api.setView("icons");
    fire(toggle, "click");
    check("toggle: the chevron expands to the detailed list",
        api.state.view === "list" &&
        panel().classList.contains("view-list") &&
        !panel().classList.contains("view-icons") &&
        storage.get("swarm.buffsView") === "list");
    fire(toggle, "click");
    check("toggle: the chevron collapses back to the icons",
        api.state.view === "icons" &&
        storage.get("swarm.buffsView") === "icons");
    fire(viewBtn, "click");
    check("toggle: the view switch flips to the list",
        api.state.view === "list");
    fire(viewBtn, "click");
    check("toggle: the view switch flips back to the grid",
        api.state.view === "icons");

    check("size: the one row grid body keeps the single row height",
        (() => {
            api.panel({ buffs: [two[0]] });
            harness.flushTimeouts();
            api.setView("icons");
            api.syncSize();

            return body().style.height === (32 + 6) + "px";
        })());
    check("size: the two rows grid body grows with the second row",
        (() => {
            const twenty = [];
            for (let i = 0; i < 11; i += 1) {
                twenty.push(buff(1000 + i, 1, 600, 1200, "s" + i, ""));
            }
            api.panel({ buffs: twenty });
            harness.flushTimeouts();
            api.syncSize();
            const height = body().style.height;
            api.panel({ buffs: two });
            harness.flushTimeouts();

            return height === (2 * 32 + 2 + 6) + "px";
        })());

    api.panel({ buffs: [] });
    check("empty: the panel hides and both views wipe",
        panel().classList.contains("hidden") &&
        grid().children.length === 0 && list().children.length === 0 &&
        api.state.cells.size === 0 && api.state.rows.size === 0 &&
        api.state.anchors.size === 0);

    check("format: the remaining time forms stay pinned",
        api.leftText(45) === "45s" && api.leftText(90) === "1m 30s" &&
        api.leftText(750) === "12m 30s" &&
        api.leftText(3900) === "1h 05m" && api.leftText(90000) === "\u2014" &&
        api.leftShort(45) === "45s" && api.leftShort(750) === "12m" &&
        api.leftShort(5400) === "1h30" && api.leftShort(90000) === "\u2014");
}

// The stubs settle synchronously: the recorded enter timeouts flush
// by hand so the checks see the settled classes.
process.exitCode = (function main() {
    const appFile = readArg();
    checkMarkup();
    checkStyles();
    const harness = loadPanel(appFile);
    harness.flushTimeouts = () => {
        const queued = harness.timers.timeouts.splice(0);
        for (const fn of queued) { fn(); }
    };
    checkBehavior(harness);
    console.log(failures === 0
        ? "repro_buffs: every check passed"
        : "repro_buffs: " + failures + " check(s) failed");

    return failures === 0 ? 0 : 1;
})();
