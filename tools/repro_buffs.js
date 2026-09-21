#!/usr/bin/env node
/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Reproduction harness for the effects panel (the buff list).
//
// It loads the real internal/swarm/webserver/web/buffs_tooltip.js,
// web/buffs_flip.js and web/buffs.js into a sandboxed context with
// a stub DOM and checks:
//
// - the markup: the panel opens in the icon grid view (the default),
//   the frame carries the left dock with the SINGLE chevron button
//   (the old view switch is gone) and the two content layers, the
//   EFFECTS head label is gone, the tooltip singleton exists and
//   buffs_tooltip.js / buffs_flip.js load before buffs.js before
//   app.js;
// - the styles: the grid packs 30px cells 10 columns wide with 2px
//   white separator gaps, the detailed list scrolls (the thin
//   scrollbar) and pins EXACTLY to the HUD stack height variable,
//   the view morphs animate (the frame width, the body height, the
//   layer cross-fade without the positional settle, the chevron
//   flip) and the pathfind mode still hides the panel;
// - the render: exactly the active effects show (two buffs build two
//   cells and two rows), the keyed cells keep their DOM nodes across
//   the snapshots (the icons must not blink), a buff leaving the
//   list drops only its own node, a joining one spawns with a pop;
// - the detailed list rows carry the name, the level, the countdown
//   and the remaining time percent bar (left over total), a zero
//   total pins the bar empty;
// - the grid cells carry the mini time strip (left over total), the
//   countdown chip fills for the hover only, every node names its
//   skill id for the tooltip;
// - the hover tooltip card: hovering a cell or a row shows the card
//   with the name, the level, the numeric effect summary, the level
//   description and the remaining time, leaving the panel hides it;
// - the countdown ticker: the anchored readings count the elapsed
//   wall clock off (a reading 61 seconds old at 120 left reads 59s
//   and turns the fading mark on in both views);
// - the view toggle: the single chevron flips the view both ways,
//   the choice persists in the localStorage, the panel body height
//   follows the active view (the grid rows, the exact HUD height)
//   and the FLIP morph flies the shared icons between the views
//   when the layout answers measurements;
// - the empty snapshot hides the panel and wipes both views;
// - the remaining time formatters keep the pinned readings.
//
// Usage: node tools/repro_buffs.js [--app <app.js>]
//
// The harness intentionally grows past the 500 line source rule: the
// webui-harness skill routes every new panel scenario into the
// existing repro file instead of fragmenting the verification across
// per-feature harnesses.
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
        dataset: {},
        children: [],
        listeners: {},
        offsetHeight: 0,
        scrollHeight: 0,
        // The layout answers nothing by default (the FLIP morph and
        // the tooltip positioning run on their guarded paths); a
        // check arms the rects it needs by hand.
        getBoundingClientRect() {
            return { left: 0, top: 0, right: 0, bottom: 0,
                width: 0, height: 0 };
        },
        matches(selector) {
            const id = this.id || "";
            const classes = " " + this.className + " ";

            return String(selector).split(",").some((part) => {
                const token = part.trim();
                if (token.startsWith("#")) {
                    return id === token.slice(1);
                }
                if (token.startsWith(".")) {
                    return classes.includes(" " + token.slice(1) + " ");
                }

                return false;
            });
        },
        closest(selector) {
            let node = this;
            while (node) {
                if (typeof node.matches === "function" &&
                    node.matches(selector)) { return node; }
                node = node._parent;
            }

            return null;
        },
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
    const state = { hudStack: null };
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) { elements.set(id, makeElement()); }

            return elements.get(id);
        },
        // The HUD stack answers a measurement only after the check
        // arms it (hudStack.offsetHeight > 0): until then the panel
        // runs on its height fallbacks.
        querySelector: (selector) => (selector === ".hud-stack" &&
            state.hudStack) ? state.hudStack : null,
        createElement: () => makeElement(),
        documentElement: { dataset: {} },
        // The tooltip card of the buffs appends here (the page body).
        body: makeElement(),
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
            // The frame hook runs the callback synchronously: the
            // FLIP morph settles inside the same tool call.
            requestAnimationFrame: (fn) => { fn(); },
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
    // The effects panel modules load in the page order (the tooltip,
    // the flip morph, then the panel core) before app.js: the
    // renderSnapshot calls its renderBuffs.
    for (const file of ["buffs_tooltip.js", "buffs_flip.js",
        "buffs.js"]) {
        vm.runInContext(fs.readFileSync(path.join(WEB_DIR, file),
            "utf8"), sandbox, { filename: file });
    }
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
        doc: document, state };
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
    check("markup: the view switch button is gone (the chevron only)",
        !html.includes('id="buffs-view-btn"') &&
        !html.includes("buffs-view-icon"));
    check("markup: the tooltip singleton of the buffs exists",
        html.includes('id="buffs-tooltip"'));
    check("markup: both content layers exist",
        html.includes('id="buffs-grid"') &&
        html.includes('id="buffs-list-wrap"') &&
        html.includes('id="buffs-list"'));
    const tooltipAt = html.indexOf('<script src="buffs_tooltip.js">');
    const flipAt = html.indexOf('<script src="buffs_flip.js">');
    const buffsAt = html.indexOf('<script src="buffs.js">');
    const appAt = html.indexOf('<script src="app.js">');
    check("markup: the panel modules load in the page order",
        tooltipAt >= 0 && flipAt > tooltipAt && buffsAt > flipAt &&
        appAt > buffsAt);
}

function checkStyles() {
    const css = fs.readFileSync(path.join(WEB_DIR, "style.css"), "utf8");
    check("styles: the grid packs 10 columns of 30px cells",
        css.includes("grid-template-columns: repeat(10, 30px)") &&
        css.includes("grid-auto-rows: 30px"));
    check("styles: the grid caps at the classic two rows",
        /\.buffs-grid \{[^}]*max-height: 60px;/s.test(css) &&
        /\.buffs-grid \{[^}]*overflow-y: auto;/s.test(css));
    check("styles: no 32px cell of the old shape is left",
        !css.includes("repeat(10, 32px)") && !css.includes("32px\""));
    check("styles: the body pads 2px on the vertical edges",
        /\.buffs-panel-body \{[^}]*padding: 2px 3px;/s.test(css));
    check("styles: the cells paint the white separator strips",
        /\.buff-cell \{[^}]*border-right: 2px solid #ffffff;/s.test(css) &&
        /\.buff-cell \{[^}]*border-bottom: 2px solid #ffffff;/s.test(css) &&
        /\.buffs-grid \{[^}]*gap: 0;/s.test(css));
    check("styles: the countdown chip shows on the hover only",
        /\.buff-cell \.buff-left \{[^}]*opacity: 0;/s.test(css) &&
        css.includes(".buff-cell:hover .buff-left { opacity: 1; }") &&
        /\.buff-cell \.buff-left \{[^}]*width: max-content;/s.test(css) &&
        /\.buff-cell \.buff-left \{[^}]*background: rgba\(0, 0, 0, 0\.62\);/s
            .test(css));
    check("styles: the mini time strip rides the bottom pixels",
        /\.buff-cell \.buff-strip \{[^}]*height: 2px;/s.test(css) &&
        /\.buff-cell \.buff-strip \{[^}]*bottom: 0;/s.test(css) &&
        /\.buff-cell \.buff-strip \{[^}]*background: var\(--accent\);/s
            .test(css));
    check("styles: the detailed list scrolls with the thin scrollbar",
        /\.buffs-list-wrap \{[^}]*overflow-y: auto;/s.test(css) &&
        css.includes(".buffs-list-wrap::-webkit-scrollbar { width: 6px; }"));
    check("styles: the morph animates the frame width and the body height",
        /\.buffs-panel \{[^}]*transition: width/s.test(css) &&
        /\.buffs-panel-body \{[^}]*transition: height/s.test(css));
    check("styles: the layers cross-fade without the positional settle",
        css.includes(".view-list .buffs-grid") &&
        css.includes(".view-icons .buffs-list-wrap") &&
        /visibility 0s linear 0\.3s/.test(css) &&
        !/\.view-list \.buffs-grid \{[^}]*translateY/s.test(css) &&
        !/\.view-icons \.buffs-list-wrap \{[^}]*translateY/s.test(css));
    check("styles: the entering layer cancels the visibility delay",
        /\.view-icons \.buffs-grid,[\s\S]*?\.view-list \.buffs-list-wrap \{[\s\S]*?visibility 0s;/.test(css));
    check("styles: the chevron is the bigger springy flip",
        css.includes(".buffs-panel.view-list .buffs-chev") &&
        css.includes("rotate(180deg)") &&
        /\.buffs-chev \{[^}]*font-size: 15px;/s.test(css) &&
        /\.buffs-chev \{[^}]*cubic-bezier\(0\.34, 1\.56, 0\.64, 1\)/s
            .test(css));
    check("styles: the dock button grew with the chevron",
        /\.buffs-dock-btn \{[^}]*width: 22px;/s.test(css) &&
        /\.buffs-dock-btn \{[^}]*height: 22px;/s.test(css));
    check("styles: the rows carry the remaining time percent bar",
        css.includes(".buff-bar") && /\.buff-bar i \{[^}]*transition: width/s
            .test(css));
    check("styles: the joining nodes spawn with the pop and glow",
        css.includes("@keyframes buff-spawn-pop") &&
        css.includes("@keyframes buff-spawn-glow") &&
        css.includes(".buff-cell.buff-spawn") &&
        css.includes(".buff-item.buff-spawn") &&
        !css.includes("buff-enter"));
    check("styles: the tooltip card of the buffs exists",
        css.includes(".buffs-tooltip") &&
        css.includes(".buffs-tooltip .bt-effect") &&
        css.includes(".buffs-tooltip .bt-desc"));
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
    check("render: every node names its skill id for the tooltip",
        api.state.cells.get(91).item.getAttribute("data-skill-id") === "91" &&
        api.state.rows.get(77).item.getAttribute("data-skill-id") === "77");
    harness.flushTimeouts();
    check("render: the joining cells drop the spawn class after the pop",
        grid().children.every((cell) =>
            !cell.classList.contains("buff-spawn")) &&
        list().children.every((row) =>
            !row.classList.contains("buff-spawn")));

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
        buff(91, 3, 1100, 1200, "Greater Defense Aura", "skill0091")
    ] });
    harness.flushTimeouts();
    check("keyed: a stronger recast refreshes the anchor fields",
        api.state.rows.get(91).level.textContent === "3" &&
        api.state.rows.get(91).name.textContent ===
            "Greater Defense Aura");

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
        const strip = api.state.cells.get(7).strip.style.width === "0%";

        return width === "0%" && strip;
    })());

    api.panel({ buffs: [
        buff(91, 1, 600, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    harness.flushTimeouts();
    check("grid: the hover countdown fills the chip",
        api.state.cells.get(91).left.textContent === "10m");
    check("grid: the mini strip answers the remaining share",
        api.state.cells.get(91).strip.style.width === "50%" &&
        api.state.cells.get(105).strip.style.width !== "");

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
    check("tick: the list body fills exactly the fallback (no HUD answer)",
        body().style.height === (320 - 6 + 4) + "px");

    // The hover tooltip card: hovering a cell or a row fills the
    // card with the effect answers, leaving the panel hides it.
    check("tooltip: the cell hover shows the card with the answers",
        (() => {
            const card = el("buffs-tooltip");
            const handlers = panel().listeners.mouseover || [];
            handlers.forEach((handler) => handler({ target: cell91 }));
            const shown = !card.classList.contains("hidden") &&
                card.children[0].textContent ===
                    "Defense Aura \u00b7 lvl 1" &&
                card.children[1].textContent.endsWith(" left") &&
                card.children[2].style.display === "none";
            panel().listeners.mouseleave.forEach((handler) =>
                handler({}));

            return shown && card.classList.contains("hidden");
        })());
    check("tooltip: the numeric effect line answers when known",
        (() => {
            const card = el("buffs-tooltip");
            api.state.anchors.get(91).effect = "+12% P. Def";
            api.state.anchors.get(91).desc =
                "Temporarily increases P. Def. Effect 1.";
            panel().listeners.mouseover.forEach((handler) =>
                handler({ target: cell91 }));
            const effect = card.children[2].textContent === "+12% P. Def";
            const desc = card.children[3].textContent ===
                "Temporarily increases P. Def. Effect 1.";
            const kept = card.children[2].style.display === "" &&
                card.children[3].style.display === "";
            panel().listeners.mouseleave.forEach((handler) =>
                handler({}));

            return effect && desc && kept;
        })());
    check("tooltip: the hover follows the cursor",
        (() => {
            const card = el("buffs-tooltip");
            panel().listeners.mouseover.forEach((handler) =>
                handler({ target: cell91 }));
            panel().listeners.mousemove.forEach((handler) =>
                handler({ clientX: 400, clientY: 220 }));
            const at = card.style.left === "414px" &&
                card.style.top === "236px";
            panel().listeners.mouseleave.forEach((handler) =>
                handler({}));

            return at;
        })());

    // The dock chevron: the single toggle, both ways, the choice
    // persists.
    const toggle = el("buffs-toggle");
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
    check("toggle: a stale view answer never breaks the flip",
        (() => {
            api.setView("neither");

            return api.state.view === "icons";
        })());

    check("size: the one row grid body keeps the single row height",
        (() => {
            api.panel({ buffs: [two[0]] });
            harness.flushTimeouts();
            api.setView("icons");
            api.syncSize();

            return body().style.height === (30 + 4) + "px";
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

            return height === (2 * 30 + 4) + "px";
        })());

    check("size: the grid holds the two row cap past 20 buffs",
        (() => {
            const many = [];
            for (let i = 0; i < 25; i += 1) {
                many.push(buff(2000 + i, 1, 600, 1200, "m" + i, ""));
            }
            api.panel({ buffs: many });
            harness.flushTimeouts();
            api.setView("icons");
            api.syncSize();
            const height = body().style.height;
            const width = panel().style.width;
            api.panel({ buffs: two });
            harness.flushTimeouts();

            return height === (2 * 30 + 4) + "px" &&
                width === (10 * 30 + 27 + 8) + "px";
        })());
    check("size: the frame hugs the filled columns",
        (() => {
            api.panel({ buffs: [two[0]] });
            harness.flushTimeouts();
            api.syncSize();
            const one = panel().style.width;
            const five = [];
            for (let i = 0; i < 5; i += 1) {
                five.push(buff(4000 + i, 1, 600, 1200, "f" + i, ""));
            }
            api.panel({ buffs: five });
            harness.flushTimeouts();
            api.syncSize();
            const half = panel().style.width;
            api.setView("list");
            api.syncSize();
            const cleared = panel().style.width === "";
            api.setView("icons");
            api.panel({ buffs: two });
            harness.flushTimeouts();

            return one === (1 * 30 + 27 + 8) + "px" &&
                half === (5 * 30 + 27 + 8) + "px" && cleared;
        })());
    check("size: the list body fills exactly the measured HUD stack",
        (() => {
            const stack = makeElement();
            stack.offsetHeight = 224;
            harness.state.hudStack = stack;
            const many = [];
            for (let i = 0; i < 30; i += 1) {
                many.push(buff(3000 + i, 1, 600, 1200, "h" + i, ""));
            }
            api.panel({ buffs: many });
            harness.flushTimeouts();
            api.setView("list");
            api.syncSize();
            const height = body().style.height;
            harness.state.hudStack = null;
            api.setView("icons");
            api.panel({ buffs: two });
            harness.flushTimeouts();
            // The exact fill: 224 stack - 6 chrome + 4 padding.
            return height === (224 - 6 + 4) + "px";
        })());

    check("flip: the shared icons fly between the views",
        (() => {
            api.panel({ buffs: two });
            harness.flushTimeouts();
            api.setView("icons");
            api.syncSize();
            const cell = api.state.cells.get(91).item;
            const rowIcon = api.state.rows.get(91).icon;
            const bodyEl = body();
            // The layout answers: the cell sits in the grid, the row
            // icon in the list, the body spans both.
            cell.getBoundingClientRect = () =>
                ({ left: 300, top: 20, right: 330, bottom: 50,
                    width: 30, height: 30 });
            rowIcon.getBoundingClientRect = () =>
                ({ left: 280, top: 40, right: 310, bottom: 70,
                    width: 30, height: 30 });
            bodyEl.getBoundingClientRect = () =>
                ({ left: 290, top: 10, right: 610, bottom: 230,
                    width: 320, height: 220 });
            api.setView("list");
            api.syncSize();
            const flown = rowIcon.style.transform === "" &&
                rowIcon.style.transition ===
                    "transform 320ms cubic-bezier(0.33, 1, 0.5, 1)";
            cell.getBoundingClientRect = () =>
                ({ left: 300, top: 20, right: 330, bottom: 50,
                    width: 30, height: 30 });
            rowIcon.getBoundingClientRect = () =>
                ({ left: 280, top: 40, right: 310, bottom: 70,
                    width: 30, height: 30 });
            api.setView("icons");
            api.syncSize();
            const back = cell.style.transform === "";
            harness.flushTimeouts();
            // The cleanup of the newer flight clears its own box; an
            // older flight's box keeps its settled transform cleared.
            const cleaned = cell.style.transform === "" &&
                cell.style.transition === "" &&
                rowIcon.style.transform === "";

            return flown && back && cleaned;
        })());
    check("flip: the invisible entries stay still",
        (() => {
            api.panel({ buffs: two });
            harness.flushTimeouts();
            api.setView("icons");
            api.syncSize();
            const rowIcon = api.state.rows.get(77).icon;
            const cell = api.state.cells.get(77).item;
            const bodyEl = body();
            // The entry sits below the pinned body height: it never
            // shows in either widget state, so no flight ever
            // touches its styles.
            rowIcon.getBoundingClientRect = () =>
                ({ left: 280, top: 400, right: 310, bottom: 430,
                    width: 30, height: 30 });
            cell.getBoundingClientRect = () =>
                ({ left: 300, top: 400, right: 330, bottom: 430,
                    width: 30, height: 30 });
            bodyEl.getBoundingClientRect = () =>
                ({ left: 290, top: 10, right: 610, bottom: 230,
                    width: 320, height: 220 });
            api.setView("list");
            api.syncSize();
            api.setView("icons");
            api.syncSize();
            const still = (rowIcon.style.transform || "") === "" &&
                (rowIcon.style.transition || "") === "" &&
                (cell.style.transform || "") === "" &&
                (cell.style.transition || "") === "";
            harness.flushTimeouts();

            return still;
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
