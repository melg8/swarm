#!/usr/bin/env node
/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Reproduction harness for the effects panel (the buff list).
//
// It loads the real internal/swarm/webserver/web/buffs_tooltip.js
// and web/buffs.js into a sandboxed context with a stub DOM and
// checks:
//
// - the markup: the panel opens as the single horizontal icon grid
//   (the vertical detailed list and the toggle chevron are gone by
//   decision, no view classes exist), the EFFECTS head label is gone,
//   the tooltip singleton exists and buffs_tooltip.js loads before
//   buffs.js before app.js (the flip module is deleted);
// - the styles: the grid packs 34px cells (the 32px native icon art
//   plus the 2px theme tinted separator) 10 columns wide, caps at the
//   two classic rows and clips past the 20 visible slots without a
//   scrollbar, the body pads the top edge only (the cell separators
//   answer the bottom chrome), the icons answer their native 32px,
//   the level badge reads white on its dark translucent plate in
//   both themes, the mini time strip rides 3px tall in the bright
//   per theme tint, nothing animates (no view morph transitions, no
//   spawn keyframes) and the pathfind mode still hides the panel;
// - the render: exactly the active effects show (two buffs build two
//   cells), the keyed cells keep their DOM nodes across the
//   snapshots (the icons must not blink), a refresh that already
//   sits in the snapshot order moves no node (a move detaches the
//   node, the browser drops its :hover state and the hover chip
//   flickered on every 300 ms snapshot), a reorder moves only the
//   out of place nodes and lands in the snapshot order, a buff
//   leaving the list drops only its own node, a joining one snaps
//   in with no spawn class and no queued animation timer;
// - the grid cells carry the mini time strip (left over total), a
//   zero total pins the strip empty, the countdown chip fills for
//   the hover only, every node names its skill id for the tooltip;
// - the hover tooltip card: hovering a cell shows the card with the
//   name, the level, the numeric effect summary, the level
//   description and the remaining time, leaving the panel hides it;
// - the countdown ticker: the anchored readings count the elapsed
//   wall clock off (a reading 61 seconds old at 120 left reads 59s
//   and turns the fading mark on);
// - the frame geometry: the width hugs the filled columns (1 cell
//   42px, 5 cells 178px, the full row 348px), the body height hugs
//   the filled rows capped at the classic two (36px one row, 70px
//   two rows, still 70px at 25 buffs);
// - the empty snapshot hides the panel and wipes the grid;
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
        // The layout answers nothing by default (the tooltip
        // positioning runs on its guarded path); a check arms the
        // rects it needs by hand.
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

function loadPanel(appFile) {
    const elements = new Map();
    const storage = new Map();
    const timers = { intervals: [], timeouts: [] };
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) { elements.set(id, makeElement()); }

            return elements.get(id);
        },
        // The panel reads no layout (the arithmetic covers the stub
        // DOM) and app.js queries nothing at load time.
        querySelector: () => null,
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
            requestAnimationFrame: (fn) => { fn(); },
            // The timers record without running: the harness fires
            // the countdown tick by hand and asserts the panel
            // queues no animation timeouts of its own.
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
    // then the panel core) before app.js: the renderSnapshot calls
    // its renderBuffs.
    for (const file of ["buffs_tooltip.js", "buffs.js"]) {
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
    check("markup: the panel opens hidden with no view class",
        html.includes('class="buffs-panel hidden"') &&
        !html.includes("view-icons") && !html.includes("view-list"));
    check("markup: no toggle button or dock exists",
        !html.includes('id="buffs-toggle"') &&
        !html.includes('id="buffs-chev"') &&
        !html.includes("buffs-dock"));
    check("markup: the view switch button stays gone",
        !html.includes('id="buffs-view-btn"') &&
        !html.includes("buffs-view-icon"));
    check("markup: the tooltip singleton of the buffs exists",
        html.includes('id="buffs-tooltip"'));
    check("markup: the grid exists and the detailed list is gone",
        html.includes('id="buffs-grid"') &&
        !html.includes('id="buffs-list-wrap"') &&
        !html.includes('id="buffs-list"'));
    const tooltipAt = html.indexOf('<script src="buffs_tooltip.js">');
    const flipAt = html.indexOf("buffs_flip.js");
    const buffsAt = html.indexOf('<script src="buffs.js">');
    const appAt = html.indexOf('<script src="app.js">');
    check("markup: the panel modules load in the page order",
        tooltipAt >= 0 && flipAt < 0 && buffsAt > tooltipAt &&
        appAt > buffsAt);
}

function checkStyles() {
    const css = fs.readFileSync(path.join(WEB_DIR, "style.css"), "utf8");
    check("styles: the grid packs 10 columns of 34px cells",
        css.includes("grid-template-columns: repeat(10, 34px)") &&
        css.includes("grid-auto-rows: 34px"));
    check("styles: the grid caps at the classic two rows and clips",
        /\.buffs-grid \{[^}]*max-height: 68px;/s.test(css) &&
        /\.buffs-grid \{[^}]*overflow: hidden;/s.test(css));
    check("styles: the grid owns no scrollbar",
        !/\.buffs-grid \{[^}]*overflow-y:\s*auto;/s.test(css) &&
        !css.includes(".buffs-grid::-webkit-scrollbar") &&
        !/\.buffs-grid \{[^}]*scrollbar-width/s.test(css));
    check("styles: no 30px cell of the scaled shape is left",
        !css.includes("repeat(10, 30px)"));
    check("styles: the body pads the top edge only",
        /\.buffs-panel-body \{[^}]*padding: 2px 3px 0;/s.test(css));
    check("styles: the cells paint the theme tinted separator strips",
        /\.buff-cell \{[^}]*border-right: 2px solid var\(--buff-separator\);/s
            .test(css) &&
        /\.buff-cell \{[^}]*border-bottom: 2px solid var\(--buff-separator\);/s
            .test(css) &&
        /\.buffs-grid \{[^}]*gap: 0;/s.test(css));
    check("styles: no hardcoded white separator is left",
        !/\.buff-cell \{[^}]*#ffffff/s.test(css));
    check("styles: the separator tint answers per theme",
        css.includes("--buff-separator: #ffffff;") &&
        css.includes("--buff-separator: var(--bg-panel);"));
    check("styles: the level badge reads white on its dark plate",
        /\.buff-cell \.badge-level \{[^}]*color: #ffffff;/s.test(css) &&
        /\.buff-cell \.badge-level \{[^}]*background: rgba\(0, 0, 0, 0\.55\);/s
            .test(css));
    check("styles: the icons answer the native 32px art unscaled",
        /\.buff-cell img \{[^}]*width: 32px;/s.test(css) &&
        /\.buff-cell img \{[^}]*height: 32px;/s.test(css));
    check("styles: the countdown chip shows on the hover only",
        /\.buff-cell \.buff-left \{[^}]*opacity: 0;/s.test(css) &&
        css.includes(".buff-cell:hover .buff-left { opacity: 1; }") &&
        /\.buff-cell \.buff-left \{[^}]*width: max-content;/s.test(css) &&
        /\.buff-cell \.buff-left \{[^}]*background: rgba\(0, 0, 0, 0\.62\);/s
            .test(css));
    check("styles: the mini time strip rides 3px tall on the bottom",
        /\.buff-cell \.buff-strip \{[^}]*height: 3px;/s.test(css) &&
        /\.buff-cell \.buff-strip \{[^}]*bottom: 0;/s.test(css) &&
        /\.buff-cell \.buff-strip \{[^}]*background: var\(--buff-strip\);/s
            .test(css));
    check("styles: the strip tint rides brighter per theme",
        css.includes("--buff-strip: #f59e0b;") &&
        css.includes("--buff-strip: #ffc061;"));
    check("styles: the frame renders strictly with no morph",
        !/\.buffs-panel \{[^}]*transition:/s.test(css) &&
        !/\.buffs-panel-body \{[^}]*transition:/s.test(css));
    check("styles: no spawn or leave animation exists",
        !css.includes("buff-spawn") && !css.includes("buff-enter"));
    check("styles: the view machinery is gone",
        !css.includes(".view-icons") && !css.includes(".view-list") &&
        !css.includes(".buffs-dock") && !css.includes(".buffs-chev") &&
        !css.includes(".buffs-list") && !css.includes(".buff-item") &&
        !css.includes(".buff-bar"));
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
    const { api, elements, timers } = harness;

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

    check("init: no view state exists and no view class is set",
        api.state.view === undefined &&
        api.setView === undefined &&
        !panel().classList.contains("view-icons") &&
        !panel().classList.contains("view-list"));
    check("init: the panel wires no toggle listener",
        !el("buffs-toggle").listeners.click);

    const two = [
        buff(91, 1, 1200, 1200, "Defense Aura", "skill0091"),
        buff(77, 2, 600, 1200, "Attack Aura", "skill0077")
    ];
    api.panel({ buffs: two });
    check("render: two buffs build exactly two cells",
        grid().children.length === 2);
    check("render: the panel unhides", !panel().classList.contains("hidden"));
    check("render: every node names its skill id for the tooltip",
        api.state.cells.get(91).item.getAttribute("data-skill-id") === "91" &&
        api.state.cells.get(77).item.getAttribute("data-skill-id") === "77");
    check("render: a joining cell snaps in with no spawn class",
        grid().children.every((cell) =>
            !cell.classList.contains("buff-spawn")));
    check("render: the panel queues no animation timer",
        timers.timeouts.length === 0);

    const cell91 = api.state.cells.get(91).item;
    check("keyed: the in place refresh moves no cell", (() => {
        const gridEl = grid();
        const realAppend = gridEl.append;
        let moves = 0;
        gridEl.append = (...added) => {
            moves += added.length;

            return realAppend.apply(gridEl, added);
        };
        // The same list again: every node already sits at its
        // snapshot index, the 300 ms snapshot stream may not touch
        // them - a move detaches the node, the browser drops its
        // :hover state and the hover countdown chip flickered.
        api.panel({ buffs: [
            buff(91, 1, 1190, 1200, "Defense Aura", "skill0091"),
            buff(77, 2, 590, 1200, "Attack Aura", "skill0077")
        ] });
        const steadyMoves = moves;
        // A reordered list moves the out of place nodes and lands
        // in the snapshot order.
        moves = 0;
        api.panel({ buffs: [
            buff(77, 2, 590, 1200, "Attack Aura", "skill0077"),
            buff(91, 1, 1190, 1200, "Defense Aura", "skill0091")
        ] });
        const reorderMoves = moves;
        const order = gridEl.children.map((cell) =>
            cell.getAttribute("data-skill-id")).join(",");
        gridEl.append = realAppend;
        api.panel({ buffs: two });

        return steadyMoves === 0 && reorderMoves > 0 && order === "77,91";
    })());
    api.panel({ buffs: [
        buff(91, 1, 1190, 1200, "Defense Aura", "skill0091"),
        buff(77, 2, 590, 1200, "Attack Aura", "skill0077")
    ] });
    check("keyed: the refresh rewrites the countdown in place",
        api.state.cells.get(91).item === cell91 &&
        api.state.cells.get(91).left.textContent === "19m");
    check("keyed: the refresh keeps the level badge current",
        api.state.cells.get(91).level.textContent === "1");

    api.panel({ buffs: [
        buff(91, 3, 1100, 1200, "Greater Defense Aura", "skill0091")
    ] });
    check("keyed: a stronger recast refreshes the anchor fields",
        api.state.cells.get(91).level.textContent === "3" &&
        api.state.cells.get(91).item === cell91);

    api.panel({ buffs: [
        buff(91, 1, 1190, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    check("keyed: the expired buff drops only its own node",
        grid().children.length === 2 &&
        api.state.cells.get(91).item === cell91 &&
        !api.state.cells.has(77) && api.state.cells.has(105));

    api.panel({ buffs: [
        buff(91, 1, 600, 1200, "Defense Aura", "skill0091"),
        buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
    ] });
    check("grid: the hover countdown fills the chip",
        api.state.cells.get(91).left.textContent === "10m");
    check("grid: the mini strip answers the remaining share",
        api.state.cells.get(91).strip.style.width === "50%" &&
        api.state.cells.get(105).strip.style.width !== "");
    check("grid: a zero total pins the strip empty", (() => {
        api.panel({ buffs: [buff(7, 1, 100, 0, "Mystery", "skill0007")] });
        const width = api.state.cells.get(7).strip.style.width;
        api.panel({ buffs: [
            buff(91, 1, 600, 1200, "Defense Aura", "skill0091"),
            buff(105, 1, 300, 1800, "Wind Walk", "skill0105")
        ] });

        return width === "0%";
    })());

    // The countdown ticker: age the anchor 61 seconds into a 120
    // second reading, the cell answers 59s and the fading mark.
    api.panel({ buffs: [buff(91, 1, 120, 1200, "Defense Aura",
        "skill0091")] });
    api.state.anchors.get(91).at = Date.now() - 61000;
    api.tick();
    check("tick: the grid countdown counts the wall clock off",
        api.state.cells.get(91).left.textContent === "59s" &&
        api.state.cells.get(91).left.classList.contains("fading"));

    // The hover tooltip card: hovering a cell fills the card with
    // the effect answers, leaving the panel hides it.
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

    check("size: the one row grid body keeps the single row height",
        (() => {
            api.panel({ buffs: [two[0]] });
            api.syncSize();

            return body().style.height === (34 + 2) + "px" &&
                panel().style.width === (1 * 34 + 8) + "px";
        })());
    check("size: the two rows grid body grows with the second row",
        (() => {
            const eleven = [];
            for (let i = 0; i < 11; i += 1) {
                eleven.push(buff(1000 + i, 1, 600, 1200, "s" + i, ""));
            }
            api.panel({ buffs: eleven });
            api.syncSize();
            const height = body().style.height;
            api.panel({ buffs: two });

            return height === (2 * 34 + 2) + "px";
        })());
    check("size: the grid holds the two row cap past 20 buffs",
        (() => {
            const many = [];
            for (let i = 0; i < 25; i += 1) {
                many.push(buff(2000 + i, 1, 600, 1200, "m" + i, ""));
            }
            api.panel({ buffs: many });
            api.syncSize();
            const height = body().style.height;
            const width = panel().style.width;
            api.panel({ buffs: two });

            return height === (2 * 34 + 2) + "px" &&
                width === (10 * 34 + 8) + "px";
        })());
    check("size: the frame hugs the filled columns",
        (() => {
            api.panel({ buffs: [two[0]] });
            api.syncSize();
            const one = panel().style.width;
            const five = [];
            for (let i = 0; i < 5; i += 1) {
                five.push(buff(4000 + i, 1, 600, 1200, "f" + i, ""));
            }
            api.panel({ buffs: five });
            api.syncSize();
            const half = panel().style.width;
            api.panel({ buffs: two });

            return one === (1 * 34 + 8) + "px" &&
                half === (5 * 34 + 8) + "px";
        })());

    api.panel({ buffs: [] });
    check("empty: the panel hides and the grid wipes",
        panel().classList.contains("hidden") &&
        grid().children.length === 0 &&
        api.state.cells.size === 0 && api.state.anchors.size === 0);

    check("format: the remaining time forms stay pinned",
        api.leftText(45) === "45s" && api.leftText(90) === "1m 30s" &&
        api.leftText(750) === "12m 30s" &&
        api.leftText(3900) === "1h 05m" && api.leftText(90000) === "\u2014" &&
        api.leftShort(45) === "45s" && api.leftShort(750) === "12m" &&
        api.leftShort(5400) === "1h30" && api.leftShort(90000) === "\u2014");
}

// The stubs of the sandbox answer no layout: the panel arithmetic
// covers it (see syncBuffsPanelSize).
process.exitCode = (function main() {
    const appFile = readArg();
    checkMarkup();
    checkStyles();
    const harness = loadPanel(appFile);
    checkBehavior(harness);
    console.log(failures === 0
        ? "repro_buffs: every check passed"
        : "repro_buffs: " + failures + " check(s) failed");

    return failures === 0 ? 0 : 1;
})();
