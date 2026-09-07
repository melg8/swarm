#!/usr/bin/env node
/*
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
*/

// Reproduction harness for the equipment widget rendering.
//
// It loads the real internal/swarm/webserver/web/app.js into a
// sandboxed context with a stub DOM and checks:
//
// - the paperdoll places the equipped items by the body part mask:
//   weapon 0x80 on the weapon slot, chest 0x400, legs 0x800, boots
//   0x1000, shirt 0x1, the two handed weapon mask 0x4000 on the
//   weapon slot, full armor 0x8000 on the chest;
// - two earrings with the either-or mask 0x6 land on different ear
//   slots, two rings with 0x30 on different finger slots;
// - the empty slots keep their labels;
// - the inventory grid skips the equipped items and renders the stack
//   count and enchant badges;
// - the slot counter shows the inventory usage.
//
// Usage: node tools/repro_gear.js [--app <app.js>]
// Exit code 0 = the equipment widget renders correctly, 1 = bug.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const DEFAULT_APP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "app.js");

// makeElement returns a DOM element stub recording children.
function makeElement() {
    return {
        textContent: "",
        style: {},
        value: "",
        checked: true,
        src: "",
        alt: "",
        title: "",
        loading: "",
        children: [],
        append: function (...added) {
            for (const child of added) { this.children.push(child); }
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
        addEventListener: () => {},
        remove: function () {
            // The image error fallback calls remove; the stub drops
            // the element from the parent when attached to one.
            if (this._parent) {
                const at = this._parent.children.indexOf(this);
                if (at >= 0) { this._parent.children.splice(at, 1); }
            }
        },
        scrollTop: 0,
        scrollHeight: 0,
        removeChild: function (child) {
            const at = this.children.indexOf(child);
            if (at >= 0) { this.children.splice(at, 1); }
        },
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
        createElement: () => {
            const el = makeElement();
            el.append = function (...added) {
                for (const child of added) {
                    child._parent = this;
                    this.children.push(child);
                }
            };
            return el;
        },
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
    vm.runInContext(
        "globalThis.__gear = {" +
        " renderGear: typeof renderGear === 'function'" +
        " ? renderGear : undefined," +
        " assignPaperdoll: typeof assignPaperdoll === 'function'" +
        " ? assignPaperdoll : undefined };",
        sandbox);

    return { gear: sandbox.__gear, elements, sandbox };
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

// gearSnapshot builds a snapshot with the given inventory.
function gearSnapshot(inventory, slots, maxSlots) {
    return {
        id: "acc1",
        character: {
            objectId: 100, name: "test1", classId: 18, race: 1, level: 9,
            inventorySlots: slots, inventoryMax: maxSlots || 80
        },
        inventory,
        objects: [], events: [], status: "online"
    };
}

// item builds one inventory entry.
function item(itemId, bodyPart, equipped, extra) {
    return Object.assign({
        objectId: itemId * 10, itemId, count: 1, type2: 0,
        equipped, bodyPart, enchant: 0,
        name: "item" + itemId, icon: "icon" + itemId
    }, extra || {});
}

// cellText collects the text of a stub cell (labels and badges).
function cellText(cell) {
    let text = cell.textContent || "";
    for (const child of cell.children) {
        text += " " + (child.textContent || "");
    }
    return text.trim();
}

// findIconCell returns the cell holding the given item icon src.
function findIconCell(root, itemId) {
    for (const cell of root.children) {
        for (const child of cell.children) {
            if (child.src === "/icons/icon" + itemId + ".png") {
                return cell;
            }
        }
    }
    return null;
}

function main() {
    const args = process.argv.slice(2);
    const appIndex = args.indexOf("--app");
    const appFile = appIndex >= 0 ? args[appIndex + 1] : DEFAULT_APP_JS;
    if (!fs.existsSync(appFile)) {
        console.error("app.js not found: " + appFile);
        process.exit(1);
    }
    const { gear, elements } = loadAppJs(appFile);
    const results = [];

    if (typeof gear.renderGear !== "function" ||
        typeof gear.assignPaperdoll !== "function") {
        console.log("FAIL  the equipment widget functions are missing");
        process.exit(1);
    }

    // Full gear set: weapon, chest, legs, boots, shirt, two earrings
    // and two rings with the either-or masks.
    gear.renderGear(gearSnapshot([
        item(1, 0x80, true),
        item(2, 0x400, true),
        item(3, 0x800, true),
        item(4, 0x1000, true),
        item(5, 0x1, true),
        item(6, 0x6, true),
        item(7, 0x6, true),
        item(8, 0x30, true),
        item(9, 0x30, true)
    ], 9));
    const paperdoll = elements.get("paperdoll");
    const invGrid = elements.get("inv-grid");

    // The paperdoll counts 15 slots, the equipped items fill nine of
    // them, the inventory grid holds nothing.
    check(results, "paperdoll has 15 slot cells",
        paperdoll.children.length === 15,
        "got " + paperdoll.children.length + " cells");
    check(results, "empty inventory grid",
        invGrid.children.length === 0,
        "got " + invGrid.children.length + " cells");

    const placed = gear.assignPaperdoll([
        item(1, 0x80, true), item(6, 0x6, true), item(7, 0x6, true),
        item(8, 0x30, true), item(9, 0x30, true)
    ]);
    check(results, "weapon mask 0x80 lands on the weapon slot",
        placed.rhand.itemId === 1, "rhand is " + placed.rhand);
    check(results, "two earrings 0x6 fill both ear slots",
        (placed.r_ear && placed.l_ear &&
            placed.r_ear.itemId !== placed.l_ear.itemId) === true,
        "r_ear=" + placed.r_ear + " l_ear=" + placed.l_ear);
    check(results, "two rings 0x30 fill both finger slots",
        (placed.r_finger && placed.l_finger &&
            placed.r_finger.itemId !== placed.l_finger.itemId) === true,
        "r_finger=" + placed.r_finger + " l_finger=" + placed.l_finger);

    // Aliased masks: two handed weapon and full armor.
    const aliased = gear.assignPaperdoll([
        item(70, 0x4000, true), item(1146, 0x8000, true)
    ]);
    check(results, "two handed mask 0x4000 lands on the weapon slot",
        aliased.rhand.itemId === 70, "rhand is " + aliased.rhand);
    check(results, "full armor mask 0x8000 lands on the chest slot",
        aliased.chest.itemId === 1146, "chest is " + aliased.chest);

    // Mixed inventory: equipped gear stays out of the bag, the bag
    // renders one cell per item with the badges.
    gear.renderGear(gearSnapshot([
        item(1, 0x80, true, { name: "Squire's Sword" }),
        item(57, 0, false, { count: 4242, type2: 4, name: "Adena" }),
        item(10, 0x80, false, { enchant: 3, name: "Dagger" })
    ], 3));
    check(results, "inventory grid holds the bag items only",
        invGrid.children.length === 2,
        "got " + invGrid.children.length + " cells");

    const adenaCell = findIconCell(invGrid, 57);
    const daggerCell = findIconCell(invGrid, 10);
    check(results, "adena cell renders the stack count badge",
        adenaCell !== null && cellText(adenaCell).includes("4242"),
        "cell text " + cellText(adenaCell || makeElement()));
    check(results, "dagger cell renders the enchant badge",
        daggerCell !== null && cellText(daggerCell).includes("+3"),
        "cell text " + cellText(daggerCell || makeElement()));
    check(results, "slot counter shows the usage",
        elements.get("inv-count").textContent === "3/80",
        "counter is " + elements.get("inv-count").textContent);

    // Empty slots keep their labels for a readable paperdoll.
    gear.renderGear(gearSnapshot([], 0));
    let labels = 0;
    for (const cell of paperdoll.children) {
        if (cell.textContent && cell.textContent.length > 0) { labels++; }
    }
    check(results, "empty paperdoll labels every slot",
        labels === 15, "labeled " + labels + " of 15");

    let failed = 0;
    for (const result of results) {
        console.log((result.ok ? "PASS  " : "FAIL  ") + result.name +
            (result.ok ? "" : " - " + result.detail));
        if (!result.ok) { failed++; }
    }
    console.log(failed === 0 ? "OK" : "BROKEN");
    process.exit(failed === 0 ? 0 : 1);
}

main();
