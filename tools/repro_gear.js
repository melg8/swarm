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
// - the compact layout: the wear block renders its nine slots (cloak,
//   head, shirt, weapon, chest, shield, boots, legs, gloves - the head
//   sits top-center above the chest, the gloves bottom-right), the
//   jewelry block renders five slots plus the blank middle-right hole
//   (the classic character has only five jewelry slots);
// - the paperdoll places the equipped items by the body part mask:
//   weapon 0x80 on the weapon slot, chest 0x400, legs 0x800, boots
//   0x1000, shirt 0x1, the two handed weapon mask 0x4000 on the
//   weapon slot, full armor 0x8000 on the chest;
// - two earrings with the either-or mask 0x6 land on different ear
//   slots, two rings with 0x30 on different finger slots;
// - the empty slots keep their labels;
// - the inventory grid skips the equipped items and renders the stack
//   count and enchant badges; the slot counter shows the usage;
// - the pinned footer: the adena line formats the carried amount and
//   the weight line fills the load bar by the percentage, colored by
//   the server weight debuff thresholds (green below 50%, then the
//   fill melts from yellow through orange into red at 50/66.6/80/
//   100 percent of the load; a dash when the load is unknown);
// - keyed updates: re-rendering the same snapshot or changing only a
//   stack count keeps the icon image elements alive (the icons must
//   not blink on every snapshot), removed items drop their cells;
// - the manual interactions: a double click posts the useItem command
//   for wearable and equipped cells (never for adena or materials), a
//   drag arms the item, dropping it on the map opens the count dialog
//   for stacks (a plain item drops whole, an equipped item unequips
//   first), the dialog commits the typed count and rejects garbage;
// - the floating placement: the widget is an overlay inside the map
//   wrap (index.html), the CSS pins it to the top right corner at the
//   same height as the player HUD (top 10px), the icons keep their
//   32px metric, the footer lines and the drop dialog exist.
//
// Usage: node tools/repro_gear.js [--app <app.js>]
// Exit code 0 = the equipment widget renders correctly, 1 = bug.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const DEFAULT_APP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "app.js");
const DEFAULT_INDEX_HTML = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "index.html");
const DEFAULT_STYLE_CSS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "style.css");

// makeElement returns a DOM element stub recording children and
// event listeners. append mirrors the real DOM: appending an existing
// child moves it to the end, it never duplicates an element.
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
        className: "",
        max: "",
        draggable: false,
        children: [],
        listeners: {},
        addEventListener(type, handler) {
            if (!this.listeners[type]) { this.listeners[type] = []; }
            this.listeners[type].push(handler);
        },
        focus: () => {},
        select: () => {},
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
        remove: function () {
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
        firstChild: null,
        append: function (...added) {
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
// listeners. The handler return values are ignored like in the real
// DOM (preventDefault only mutates the event object).
function fire(element, type, event) {
    const handlers = (element.listeners || {})[type] || [];
    const prepared = Object.assign({
        preventDefault: () => {}, key: "",
        dataTransfer: {
            setData: () => {},
            getData: () => "",
            dropEffect: "",
            effectAllowed: ""
        }
    }, event || {});
    for (const handler of handlers) {
        handler(prepared);
    }

    return prepared;
}

function loadAppJs(appFile) {
    const elements = new Map();
    const posts = [];
    const document = {
        getElementById: (id) => {
            if (!elements.has(id)) {
                elements.set(id, makeElement());
            }
            return elements.get(id);
        },
        // querySelector answers null: the harness owns no .gear-slots
        // element, but a defined querySelector lets initGearInteractions
        // wire the real dialog listeners (the wheel stepping included).
        querySelector: () => null,
        createElement: () => makeElement(),
        documentElement: { dataset: {} },
        // app.js registers a document-level click and an Escape close
        // for the view layers dropdown of the toolbar; record them so
        // the harness can fire both.
        listeners: {},
        addEventListener(type, handler) {
            if (!this.listeners[type]) { this.listeners[type] = []; }
            this.listeners[type].push(handler);
        }
    };
    const sandbox = {
        Math, JSON, Number, Date, isNaN,
        window: { localStorage: {
            getItem: () => null, setItem: () => {}
        } },
        document,
        fetch: (url, options) => {
            posts.push({ url, options });
            return { catch: () => {} };
        },
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
        " ? assignPaperdoll : undefined," +
        " dropItemOnMap: typeof dropItemOnMap === 'function'" +
        " ? dropItemOnMap : undefined," +
        " equipItem: typeof equipItem === 'function'" +
        " ? equipItem : undefined," +
        " destroyItemFromWidget: typeof destroyItemFromWidget === 'function'" +
        " ? destroyItemFromWidget : undefined," +
        " commitDestroy: typeof commitDestroy === 'function'" +
        " ? commitDestroy : undefined," +
        " confirmDropDialog: typeof confirmDropDialog === 'function'" +
        " ? confirmDropDialog : undefined," +
        " resolveDropCount: typeof resolveDropCount === 'function'" +
        " ? resolveDropCount : undefined," +
        " weightPenalty: typeof weightPenalty === 'function'" +
        " ? weightPenalty : undefined," +
        " weightFillStyle: typeof weightFillStyle === 'function'" +
        " ? weightFillStyle : undefined," +
        " bumpDropCount: typeof bumpDropCount === 'function'" +
        " ? bumpDropCount : undefined," +
        " itemFamily: typeof itemFamily === 'function'" +
        " ? itemFamily : undefined," +
        " renderItemTooltip: typeof renderItemTooltip === 'function'" +
        " ? renderItemTooltip : undefined," +
        " renderShopping: typeof renderShopping === 'function'" +
        " ? renderShopping : undefined," +
        " resetShop: typeof resetShop === 'function'" +
        " ? resetShop : undefined," +
        " renderShoppingTooltip: typeof renderShoppingTooltip ===" +
        " 'function' ? renderShoppingTooltip : undefined," +
        " ShopPanel: typeof ShopPanel !== 'undefined' ? ShopPanel" +
        " : undefined," +
        " App: App, GearDrag: GearDrag };",
        sandbox);

    return { gear: sandbox.__gear, elements, sandbox, posts };
}

function check(results, name, ok, detail) {
    results.push({ name, ok, detail });
}

// gearSnapshot builds a snapshot with the given inventory. The extra
// character fields (adena, load, maxLoad) feed the pinned footer.
function gearSnapshot(inventory, slots, maxSlots, char) {
    return {
        id: "acc1",
        character: Object.assign({
            objectId: 100, name: "test1", classId: 18, race: 1, level: 9,
            inventorySlots: slots, inventoryMax: maxSlots || 80,
            adena: 0, load: 0, maxLoad: 0
        }, char || {}),
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

// deepText recursively collects the text of a stub element and every
// descendant: the shop queue rows nest their text nodes two levels
// deep (li > div.info > div.name).
function deepText(el) {
    let text = el.textContent || "";
    for (const child of el.children || []) {
        text += " " + deepText(child);
    }
    return text.trim();
}

// findImg returns the icon image element of the given item.
function findImg(root, itemId) {
    for (const cell of root.children) {
        for (const child of cell.children) {
            if (child.src === "/icons/icon" + itemId + ".png") {
                return child;
            }
        }
    }
    return null;
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

// Slot order of the two paperdoll blocks (mirrors app.js).
const WEAR_KEYS = ["under", "head", "back", "rhand", "chest",
    "lhand", "gloves", "legs", "feet"];
const JEWEL_KEYS = ["r_ear", "l_ear", null, "neck", "r_finger", "l_finger"];

// slotCell returns the rendered cell of a slot key.
function slotCell(wearBox, jewelBox, key) {
    const wearAt = WEAR_KEYS.indexOf(key);
    if (wearAt >= 0) { return wearBox.children[wearAt]; }
    const jewelAt = JEWEL_KEYS.indexOf(key);
    if (jewelAt >= 0) { return jewelBox.children[jewelAt]; }

    return null;
}

// hasImg reports whether the cell shows the icon of the item.
function hasImg(cell, itemId) {
    if (!cell) { return false; }
    for (const child of cell.children) {
        if (child.src === "/icons/icon" + itemId + ".png") {
            return true;
        }
    }
    return false;
}

// labelOf returns the label span of a slot cell.
function labelOf(cell) {
    for (const child of cell.children) {
        if (child.className === "slot-label") { return child; }
    }
    return null;
}

// labelVisible reports whether the slot shows its empty label.
function labelVisible(cell) {
    const label = labelOf(cell);
    return Boolean(label) && label.textContent.length > 0 &&
        label.style.display !== "none";
}

function main() {
    const args = process.argv.slice(2);
    const appIndex = args.indexOf("--app");
    const appFile = appIndex >= 0 ? args[appIndex + 1] : DEFAULT_APP_JS;
    if (!fs.existsSync(appFile)) {
        console.error("app.js not found: " + appFile);
        process.exit(1);
    }
    const { gear, elements, sandbox, posts } = loadAppJs(appFile);
    const results = [];

    // The manual interactions need an active bot to post to.
    gear.App.activeBotId = "acc1";

    // lastPost returns the newest captured command post body.
    const lastPost = () => {
        const post = posts[posts.length - 1];
        return post ? JSON.parse(post.options.body) : null;
    };
    const postCount = () => posts.length;

    if (typeof gear.renderGear !== "function" ||
        typeof gear.assignPaperdoll !== "function") {
        console.log("FAIL  the equipment widget functions are missing");
        process.exit(1);
    }

    // Full gear set: weapon, chest, legs, boots, shirt, two earrings
    // and two rings with the either-or masks.
    const fullSet = [
        item(1, 0x80, true),
        item(2, 0x400, true),
        item(3, 0x800, true),
        item(4, 0x1000, true),
        item(5, 0x1, true),
        item(6, 0x6, true),
        item(7, 0x6, true),
        item(8, 0x30, true),
        item(9, 0x30, true)
    ];
    gear.renderGear(gearSnapshot(fullSet, 9));
    const wearBox = elements.get("gear-wear");
    const jewelBox = elements.get("gear-jewel");
    const invGrid = elements.get("inv-grid");

    // The compact layout: nine wear cells, five jewelry cells and the
    // blank middle-left hole, nothing in the bag.
    check(results, "wear block holds 9 slot cells",
        wearBox.children.length === 9,
        "got " + wearBox.children.length + " cells");
    check(results, "jewel block holds 5 slots and the blank hole",
        jewelBox.children.length === 6 &&
        jewelBox.children[2].className === "jewel-hole",
        "got " + jewelBox.children.length + " children, hole class " +
        (jewelBox.children[2] || makeElement()).className);
    check(results, "neck sits right of the blank hole",
        labelOf(jewelBox.children[3]) &&
        labelOf(jewelBox.children[3]).textContent === "neck",
        "middle row: " + JSON.stringify([
            jewelBox.children[2].className,
            labelOf(jewelBox.children[3])
                ? labelOf(jewelBox.children[3]).textContent : "?"]));
    check(results, "empty inventory grid",
        invGrid.children.length === 0,
        "got " + invGrid.children.length + " cells");

    // Placement by the body part mask.
    check(results, "weapon mask 0x80 fills the weapon slot",
        hasImg(slotCell(wearBox, jewelBox, "rhand"), 1),
        "weapon slot children " + JSON.stringify(
            (slotCell(wearBox, jewelBox, "rhand") || {}).children.length));
    check(results, "chest mask 0x400 fills the chest slot",
        hasImg(slotCell(wearBox, jewelBox, "chest"), 2), "chest slot");
    check(results, "legs mask 0x800 fills the legs slot",
        hasImg(slotCell(wearBox, jewelBox, "legs"), 3), "legs slot");
    check(results, "boots mask 0x1000 fill the boots slot",
        hasImg(slotCell(wearBox, jewelBox, "feet"), 4), "boots slot");
    check(results, "shirt mask 0x1 fills the shirt slot",
        hasImg(slotCell(wearBox, jewelBox, "under"), 5), "shirt slot");
    check(results, "two earrings 0x6 fill both ear slots",
        hasImg(slotCell(wearBox, jewelBox, "r_ear"), 6) &&
        hasImg(slotCell(wearBox, jewelBox, "l_ear"), 7), "ear slots");
    check(results, "two rings 0x30 fill both finger slots",
        hasImg(slotCell(wearBox, jewelBox, "r_finger"), 8) &&
        hasImg(slotCell(wearBox, jewelBox, "l_finger"), 9), "finger slots");
    check(results, "filled slots hide their labels",
        labelOf(slotCell(wearBox, jewelBox, "rhand")).style.display === "none",
        "weapon label display " +
        labelOf(slotCell(wearBox, jewelBox, "rhand")).style.display);

    // assignPaperdoll unit checks, including the aliased masks.
    const placed = gear.assignPaperdoll(fullSet);
    check(results, "assignPaperdoll keys the weapon",
        placed.rhand.itemId === 1, "rhand is " + placed.rhand);
    const aliased = gear.assignPaperdoll([
        item(70, 0x4000, true), item(1146, 0x8000, true)
    ]);
    check(results, "two handed mask 0x4000 lands on the weapon slot",
        aliased.rhand.itemId === 70, "rhand is " + aliased.rhand);
    check(results, "full armor mask 0x8000 lands on the chest slot",
        aliased.chest.itemId === 1146, "chest is " + aliased.chest);

    // Mixed inventory: equipped gear stays out of the bag, the bag
    // renders one cell per item with the badges.
    const mixed = [
        item(1, 0x80, true, { name: "Squire's Sword" }),
        item(57, 0, false, { count: 4242, type2: 4, name: "Adena" }),
        item(10, 0x80, false, { enchant: 3, name: "Dagger" })
    ];
    gear.renderGear(gearSnapshot(mixed, 3));
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

    // Keyed updates: the flicker regression. A repeated snapshot must
    // keep every image element (a rebuilt element decodes again and
    // the icon blinks), a count change keeps the adena image too.
    const adenaImg = findImg(invGrid, 57);
    const daggerImg = findImg(invGrid, 10);
    const weaponImg = findImg(slotCell(wearBox, jewelBox, "rhand"), 1);
    gear.renderGear(gearSnapshot(mixed, 3));
    check(results, "unchanged snapshot keeps the icon elements",
        findImg(invGrid, 57) === adenaImg &&
        findImg(invGrid, 10) === daggerImg &&
        findImg(slotCell(wearBox, jewelBox, "rhand"), 1) === weaponImg,
        "icons were recreated");
    check(results, "unchanged snapshot does not duplicate cells",
        invGrid.children.length === 2,
        "got " + invGrid.children.length + " cells");

    gear.renderGear(gearSnapshot([
        item(1, 0x80, true, { name: "Squire's Sword" }),
        item(57, 0, false, { count: 9999, type2: 4, name: "Adena" })
    ], 2));
    check(results, "count change keeps the adena icon element",
        findImg(invGrid, 57) === adenaImg, "adena icon was recreated");
    check(results, "count badge updates in place",
        cellText(findIconCell(invGrid, 57)).includes("9999"),
        "cell text " + cellText(findIconCell(invGrid, 57) || makeElement()));
    check(results, "removed item drops its cell",
        invGrid.children.length === 1,
        "got " + invGrid.children.length + " cells");

    // Empty slots keep their labels for a readable paperdoll.
    gear.renderGear(gearSnapshot([], 0));
    let labels = 0;
    for (const cell of wearBox.children) {
        if (labelVisible(cell)) { labels++; }
    }
    for (const cell of jewelBox.children) {
        if (cell.className !== "jewel-hole" && labelVisible(cell)) {
            labels++;
        }
    }
    check(results, "empty paperdoll labels all 14 slots",
        labels === 14, "labeled " + labels + " of 14");
    check(results, "emptied inventory grid",
        invGrid.children.length === 0,
        "got " + invGrid.children.length + " cells");

    // Pinned footer: adena and weight always visible below the bag.
    gear.renderGear(gearSnapshot([], 0, 80,
        { adena: 424242, load: 3000, maxLoad: 6000 }));
    check(results, "adena line shows the carried amount",
        elements.get("gear-adena").textContent === "424,242",
        "adena line is " + JSON.stringify(
            elements.get("gear-adena").textContent));
    check(results, "weight line shows the load percent",
        elements.get("gear-load-text").textContent === "50%" &&
        elements.get("gear-load-fill").style.width === "50.0%",
        "text " + elements.get("gear-load-text").textContent +
        ", width " + elements.get("gear-load-fill").style.width);
    check(results, "half load crosses the first weight debuff",
        elements.get("gear-load-fill").className === "load-fill pen1" &&
        (elements.get("gear-load-fill").style.background || "")
            .startsWith("hsl("),
        "class " + elements.get("gear-load-fill").className +
        ", background " +
        elements.get("gear-load-fill").style.background);
    check(results, "weight tooltip carries the debuff level",
        (elements.get("gear-weight-row").title || "")
            .includes("3,000") &&
        (elements.get("gear-weight-row").title || "")
            .includes("6,000") &&
        (elements.get("gear-weight-row").title || "")
            .includes("weight debuff 1") &&
        (elements.get("gear-weight-row").title || "")
            .includes("x0.90"),
        "title " + elements.get("gear-weight-row").title);

    gear.renderGear(gearSnapshot([], 0, 80,
        { adena: 123456, load: 3996, maxLoad: 6000 }));
    check(results, "two thirds load turns the bar orange",
        elements.get("gear-load-fill").className === "load-fill pen2" &&
        (elements.get("gear-load-fill").style.background || "")
            .startsWith("hsl(26"),
        "class " + elements.get("gear-load-fill").className +
        ", background " +
        elements.get("gear-load-fill").style.background);

    gear.renderGear(gearSnapshot([], 0, 80,
        { adena: 123456, load: 7000, maxLoad: 6000 }));
    check(results, "overload clamps to the deepest debuff",
        elements.get("gear-load-fill").className === "load-fill pen4" &&
        elements.get("gear-load-text").textContent === "100%",
        "class " + elements.get("gear-load-fill").className +
        ", text " + elements.get("gear-load-text").textContent);

    gear.renderGear(gearSnapshot([], 0, 80,
        { adena: 123456, load: 5900, maxLoad: 6000 }));
    check(results, "near limit load colors the bar red",
        elements.get("gear-load-fill").className === "load-fill pen3" &&
        (elements.get("gear-load-fill").style.background || "")
            .startsWith("hsl("),
        "class " + elements.get("gear-load-fill").className +
        ", background " +
        elements.get("gear-load-fill").style.background);

    gear.renderGear(gearSnapshot([], 0, 80,
        { adena: 0, load: 0, maxLoad: 0 }));
    check(results, "unknown load shows the dash and an empty bar",
        elements.get("gear-load-text").textContent === "—" &&
        elements.get("gear-load-fill").style.width === "0%" &&
        elements.get("gear-load-fill").className === "load-fill",
        "text " + elements.get("gear-load-text").textContent +
        ", class " + elements.get("gear-load-fill").className);

    // Floating placement: the widget overlays the map instead of
    // squeezing it as a fixed right column.
    const html = fs.readFileSync(DEFAULT_INDEX_HTML, "utf8");
    const mapWrapAt = html.indexOf('<div class="map-wrap">');
    const canvasAt = html.indexOf('<canvas id="map-canvas">');
    const gearAt = html.indexOf('id="gear-panel"');
    const chatAt = html.indexOf('id="chat-box"');
    check(results, "gear panel lives inside the map wrap",
        mapWrapAt >= 0 && canvasAt > mapWrapAt && gearAt > canvasAt &&
        chatAt > gearAt,
        "mapWrap " + mapWrapAt + ", canvas " + canvasAt +
        ", gear " + gearAt + ", chat " + chatAt);
    check(results, "no fixed gear column remains in the app body",
        !html.includes('aside class="gear-panel"'),
        "the aside column is still there");
    check(results, "footer holds the adena and weight lines",
        html.includes('id="gear-adena"') &&
        html.includes('id="gear-load-fill"') &&
        html.includes('id="gear-load-text"'),
        "missing footer ids");
    check(results, "footer numbers come before the trash bin",
        html.indexOf('class="gear-foot-cols"') <
        html.indexOf('id="gear-trash"') &&
        html.indexOf('id="gear-adena"') <
        html.indexOf('id="gear-trash"'),
        "the trash bin is not on the right side");

    const css = fs.readFileSync(DEFAULT_STYLE_CSS, "utf8");
    const gearCssBlock = css.slice(css.indexOf(".gear-panel {"),
        css.indexOf(".gear-panel {") + 400);
    check(results, "gear panel floats at the HUD height",
        gearCssBlock.includes("position: absolute") &&
        gearCssBlock.includes("top: 10px") &&
        gearCssBlock.includes("right: 12px"),
        "panel block: " + gearCssBlock.slice(0, 140));
    check(results, "compass rose moved out of the panel corner",
        css.includes(".map-rose") &&
        !css.slice(css.indexOf(".map-rose {"),
            css.indexOf(".map-rose {") + 200).includes("top: 10px"),
        "the rose still sits at the top right");
    check(results, "icons keep the 32px metric",
        css.includes(".pd-cell img, .inv-cell img") &&
        css.includes("width: 32px") && css.includes("height: 32px"),
        "icon size css changed");
    check(results, "load bar melt replaces the threshold classes",
        !css.includes(".load-fill.warn") &&
        !css.includes(".load-fill.heavy") &&
        css.includes("melts from yellow"),
        "the fixed threshold classes are still styled");
    const fillStyle = gear.weightFillStyle;
    const melt = [
        fillStyle(40), fillStyle(50), fillStyle(66.6),
        fillStyle(80), fillStyle(100)
    ];
    check(results, "weightFillStyle melts yellow to red",
        melt[0] === "" && melt[1].startsWith("hsl(50") &&
        melt[2].startsWith("hsl(26") &&
        melt[3].startsWith("hsl(8") &&
        melt[4].startsWith("hsl(0"),
        "melt: " + JSON.stringify(melt));
    const penalties = [
        gear.weightPenalty(49.9), gear.weightPenalty(50).level,
        gear.weightPenalty(67).level, gear.weightPenalty(85).level,
        gear.weightPenalty(100).level
    ];
    check(results, "weightPenalty mirrors the server thresholds",
        JSON.stringify(penalties) === '[null,1,2,3,4]',
        "penalties: " + JSON.stringify(penalties));

    // ---- manual interactions ----

    // A mixed inventory again: a weapon to equip and unequip, adena
    // to drop with a count dialog.
    gear.renderGear(gearSnapshot([
        item(1, 0x80, true, { name: "Squire's Sword" }),
        item(57, 0, false, { count: 4242, type2: 4, name: "Adena" }),
        item(10, 0x80, false, { enchant: 3, name: "Dagger" })
    ], 3));
    const weaponCell2 = slotCell(wearBox, jewelBox, "rhand");
    const adenaCell2 = findIconCell(invGrid, 57);
    const daggerCell2 = findIconCell(invGrid, 10);

    // Double click: wearable items and equipped items post useItem,
    // adena never posts.
    fire(weaponCell2, "dblclick");
    check(results, "double click on an equipped weapon posts useItem",
        postCount() === 1 && lastPost().kind === "useItem" &&
        lastPost().objectId === 10,
        "posts: " + JSON.stringify(posts.map((p) => p.options.body)));
    fire(daggerCell2, "dblclick");
    check(results, "double click on a bag weapon posts useItem",
        postCount() === 2 && lastPost().kind === "useItem" &&
        lastPost().objectId === 100,
        "posts: " + JSON.stringify(posts.map((p) => p.options.body)));
    fire(adenaCell2, "dblclick");
    check(results, "double click on adena posts nothing",
        postCount() === 2,
        "posts: " + JSON.stringify(posts.map((p) => p.options.body)));

    // Drag of a stackable item onto the map: the count dialog opens
    // instead of an immediate drop.
    fire(adenaCell2, "dragstart");
    check(results, "dragstart arms the dragged item",
        gear.GearDrag.item && gear.GearDrag.item.itemId === 57,
        "drag state: " + JSON.stringify(gear.GearDrag.item));
    gear.dropItemOnMap(gear.GearDrag.item);
    const dialog = elements.get("drop-dialog");
    check(results, "dropping a stack opens the count dialog",
        !dialog.classList.contains("hidden") &&
        elements.get("drop-name").textContent === "Adena" &&
        elements.get("drop-max").textContent === "/ 4242",
        "dialog hidden=" + dialog.classList.contains("hidden") +
        ", name " + elements.get("drop-name").textContent);

    // Garbage input is rejected, a typed count commits the drop.
    elements.get("drop-count").value = "banana";
    gear.confirmDropDialog();
    check(results, "garbage count keeps the dialog open",
        !dialog.classList.contains("hidden") && postCount() === 2,
        "dialog closed or posted");
    elements.get("drop-count").value = "12";
    gear.confirmDropDialog();
    check(results, "typed count commits the drop",
        dialog.classList.contains("hidden") &&
        lastPost().kind === "drop" && lastPost().objectId === 570 &&
        lastPost().count === 12,
        "last post: " + JSON.stringify(lastPost()));

    // An equipped item dragged to the map unequips first: the queue
    // preserves the order useItem -> drop.
    fire(weaponCell2, "dragstart");
    gear.dropItemOnMap(gear.GearDrag.item);
    check(results, "equipped drop asks no count (plain item)",
        dialog.classList.contains("hidden"),
        "dialog opened for a count 1 item");
    check(results, "equipped drop posts unequip then drop",
        postCount() === 5 &&
        JSON.parse(posts[3].options.body).kind === "useItem" &&
        lastPost().kind === "drop" && lastPost().count === 1,
        "posts: " + JSON.stringify(posts.map((p) => p.options.body)));

    // The count resolver itself.
    const resolved = [
        gear.resolveDropCount("3", 10),
        gear.resolveDropCount("99", 10),
        gear.resolveDropCount("", 10),
        gear.resolveDropCount("x", 10),
        gear.resolveDropCount("2.7", 10)
    ];
    check(results, "resolveDropCount parses, clamps and rejects",
        JSON.stringify(resolved) === "[3,10,null,null,2]",
        "resolved: " + JSON.stringify(resolved));

    // The drop dialog markup exists in the html.
    check(results, "drop dialog exists in the html",
        html.includes('id="drop-dialog"') &&
        html.includes('id="drop-count"') &&
        html.includes('id="drop-ok"') &&
        html.includes('id="drop-all"') &&
        html.includes('id="drop-head"') &&
        html.includes('id="drop-note"'),
        "missing dialog ids");

    // ---- interactive round 2 ----

    // The wear order of the C1 client doll: shirt top-left, head
    // top-center, cloak top-right, gloves bottom-left, legs bottom
    // center, boots bottom-right.
    const orderKeys = [];
    for (const cell of wearBox.children) {
        const label = labelOf(cell);
        orderKeys.push(label ? label.textContent : "(filled)");
    }
    check(results, "wear grid: shirt, head, cloak row",
        orderKeys[0] === "shirt" && orderKeys[1] === "head" &&
        orderKeys[2] === "cloak",
        "row1: " + JSON.stringify(orderKeys));
    check(results, "wear grid: weapon, chest, shield row",
        orderKeys[3] === "weapon" && orderKeys[4] === "chest" &&
        orderKeys[5] === "shield",
        "row2: " + JSON.stringify(orderKeys));
    check(results, "wear grid: gloves, legs, boots row",
        orderKeys[6] === "gloves" && orderKeys[7] === "legs" &&
        orderKeys[8] === "boots",
        "row3: " + JSON.stringify(orderKeys));

    // The trash target of the footer: markup, css and the svg icon.
    check(results, "trash target exists right of the footer lines",
        html.indexOf('id="gear-adena"') <
        html.indexOf('id="gear-trash"') &&
        html.includes('class="gear-foot-cols"'),
        "footer order wrong");
    check(results, "trash target carries the bin icon",
        html.includes("<svg") &&
        html.indexOf("<svg", html.indexOf('id="gear-trash"')) <
        html.indexOf("</div>", html.indexOf('id="gear-trash"')),
        "no svg inside the trash cell");
    check(results, "trash css styles the destroy target",
        css.includes(".gear-trash") &&
        css.includes(".gear-trash.drop-hover"),
        "missing trash css rules");

    // No cursor change over the item cells.
    const cursorBlock = css.slice(css.indexOf(".pd-cell, .inv-cell"),
        css.indexOf(".pd-cell, .inv-cell") + 260);
    check(results, "item cells keep the plain cursor",
        !cursorBlock.includes("cursor: grab") &&
        !cursorBlock.includes("cursor: grabbing") &&
        cursorBlock.includes("cursor: default"),
        "cursor block: " + cursorBlock.slice(0, 120));

    // The dialog buttons size for readable text (the .btn base pins
    // 26x24 px for the toolbar icons, the dialog must override it).
    const btnBlock = css.slice(css.indexOf(".drop-btn {"),
        css.indexOf(".drop-btn {") + 260);
    check(results, "dialog buttons use readable sizing",
        btnBlock.includes("width: auto") &&
        btnBlock.includes("height: auto") &&
        btnBlock.includes("min-height") &&
        btnBlock.includes("text-align: center"),
        "button block: " + btnBlock.slice(0, 140));

    // The equip swap: a bag weapon double click with the slot taken
    // unequips the old weapon first, then equips the new one.
    gear.App.snapshot = gearSnapshot([
        item(1, 0x80, true, { name: "Squire's Sword" }),
        item(10, 0x80, false, { name: "Dagger" })
    ], 2);
    gear.renderGear(gear.App.snapshot);
    const swapCell = findIconCell(invGrid, 10);
    const postsBefore = posts.length;
    fire(swapCell, "dblclick");
    check(results, "occupied slot swap posts useItem old then new",
        posts.length === postsBefore + 2 &&
        JSON.parse(posts[posts.length - 2].options.body).kind ===
        "useItem" &&
        JSON.parse(posts[posts.length - 2].options.body).objectId === 10 &&
        JSON.parse(posts[posts.length - 1].options.body).kind ===
        "useItem" &&
        JSON.parse(posts[posts.length - 1].options.body).objectId === 100,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore).map((p) => p.options.body)));

    // The same swap through the paperdoll drop helper.
    const postsBefore2 = posts.length;
    gear.equipItem(findIconCell(invGrid, 10) && {
        objectId: 100, itemId: 10, count: 1, type2: 0, equipped: false,
        bodyPart: 0x80, name: "Dagger", enchant: 0, icon: "icon10"
    });
    check(results, "equipItem swaps the occupied slot too",
        posts.length === postsBefore2 + 2 &&
        JSON.parse(posts[posts.length - 2].options.body).objectId === 10 &&
        JSON.parse(posts[posts.length - 1].options.body).objectId === 100,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore2).map((p) => p.options.body)));

    // A free slot equips without the unequip request.
    gear.App.snapshot = gearSnapshot([
        item(10, 0x80, false, { name: "Dagger" })
    ], 1);
    const postsBefore3 = posts.length;
    gear.equipItem({ objectId: 100, itemId: 10, count: 1, type2: 0,
        equipped: false, bodyPart: 0x80, name: "Dagger", enchant: 0,
        icon: "icon10" });
    check(results, "free slot equips with a single useItem",
        posts.length === postsBefore3 + 1 &&
        JSON.parse(posts[posts.length - 1].options.body).objectId === 100,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore3).map((p) => p.options.body)));

    // The trash flow: a plain item destroys whole, a stack opens the
    // dialog in the destroy mode, an equipped item unequips first.
    const postsBefore4 = posts.length;
    gear.destroyItemFromWidget({ objectId: 570, itemId: 57, count: 1,
        type2: 4, equipped: false, bodyPart: 0, name: "Adena",
        enchant: 0, icon: "icon57" });
    check(results, "plain item drag to trash destroys whole",
        posts.length === postsBefore4 + 1 &&
        JSON.parse(posts[posts.length - 1].options.body).kind ===
        "destroy" &&
        JSON.parse(posts[posts.length - 1].options.body).count === 1,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore4).map((p) => p.options.body)));

    gear.destroyItemFromWidget({ objectId: 570, itemId: 57, count: 4242,
        type2: 4, equipped: false, bodyPart: 0, name: "Adena",
        enchant: 0, icon: "icon57" });
    check(results, "stack drag to trash opens the dialog",
        !dialog.classList.contains("hidden"),
        "dialog stayed hidden");
    check(results, "destroy mode relabels the dialog",
        elements.get("drop-head").textContent === "destroy the item" &&
        elements.get("drop-note").textContent ===
        "the item is gone for good" &&
        elements.get("drop-ok").textContent === "destroy",
        "head " + elements.get("drop-head").textContent +
        ", note " + elements.get("drop-note").textContent +
        ", ok " + elements.get("drop-ok").textContent);
    elements.get("drop-count").value = "777";
    const postsBefore5 = posts.length;
    gear.confirmDropDialog();
    check(results, "typed count commits the destroy",
        dialog.classList.contains("hidden") &&
        posts.length === postsBefore5 + 1 &&
        JSON.parse(posts[posts.length - 1].options.body).kind ===
        "destroy" &&
        JSON.parse(posts[posts.length - 1].options.body).count === 777,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore5).map((p) => p.options.body)));

    gear.destroyItemFromWidget({ objectId: 300, itemId: 30, count: 5,
        type2: 0, equipped: true, bodyPart: 0x400, name: "Tunic",
        enchant: 0, icon: "icon30" });
    elements.get("drop-count").value = "5";
    const postsBefore6 = posts.length;
    gear.confirmDropDialog();
    check(results, "equipped trash posts unequip then destroy",
        posts.length === postsBefore6 + 2 &&
        JSON.parse(posts[posts.length - 2].options.body).kind ===
        "useItem" &&
        JSON.parse(posts[posts.length - 1].options.body).kind ===
        "destroy" &&
        JSON.parse(posts[posts.length - 1].options.body).objectId === 300,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore6).map((p) => p.options.body)));

    // The drop dialog keeps its ground-drop labels in the drop mode.
    gear.dropItemOnMap({ objectId: 570, itemId: 57, count: 4242,
        type2: 4, equipped: false, bodyPart: 0, name: "Adena",
        enchant: 0, icon: "icon57" });
    check(results, "drop mode keeps the ground labels",
        elements.get("drop-head").textContent ===
        "drop on the ground" &&
        elements.get("drop-note").textContent ===
        "lands at the character feet" &&
        elements.get("drop-ok").textContent === "drop",
        "head " + elements.get("drop-head").textContent);

    // The scroll wheel over the open dialog steps the count.
    const wheelInput = elements.get("drop-count");
    wheelInput.value = "1";
    let wheelDefaulted = false;
    fire(dialog, "wheel", { deltaY: -100,
        preventDefault() { wheelDefaulted = true; } });
    check(results, "wheel up steps the count",
        String(wheelInput.value) === "2" && wheelDefaulted,
        "count " + wheelInput.value +
        ", default prevented " + wheelDefaulted);
    fire(dialog, "wheel", { deltaY: 100 });
    check(results, "wheel down steps back",
        String(wheelInput.value) === "1", "count " + wheelInput.value);
    fire(dialog, "wheel", { deltaY: 100 });
    check(results, "wheel down clamps at one",
        String(wheelInput.value) === "1", "count " + wheelInput.value);
    wheelInput.value = "4242";
    fire(dialog, "wheel", { deltaY: -100 });
    check(results, "wheel up clamps at the stack size",
        String(wheelInput.value) === "4242", "count " + wheelInput.value);

    // The target widget double click: an attackable target posts the
    // attack command, a friendly or dead one posts nothing.
    const targetPanel = elements.get("hud-target");
    const postsBefore7 = posts.length;
    gear.App.snapshot = gearSnapshot([], 0);
    gear.App.snapshot.character.targetId = 7;
    gear.App.snapshot.objects = [
        { objectId: 7, kind: "npc", name: "Gremlin", attackable: true,
        dead: false, level: 1, curHp: 30, maxHp: 30, curMp: 10,
        maxMp: 10 }
    ];
    fire(targetPanel, "dblclick");
    check(results, "target widget dblclick attacks the target",
        posts.length === postsBefore7 + 1 &&
        JSON.parse(posts[posts.length - 1].options.body).kind ===
        "attack" &&
        JSON.parse(posts[posts.length - 1].options.body).objectId === 7,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore7).map((p) => p.options.body)));

    gear.App.snapshot.objects = [
        { objectId: 7, kind: "npc", name: "Newbie Helper",
        attackable: false, dead: false }
    ];
    fire(targetPanel, "dblclick");
    check(results, "friendly target dblclick posts nothing",
        posts.length === postsBefore7 + 1,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore7).map((p) => p.options.body)));

    gear.App.snapshot.objects = [
        { objectId: 7, kind: "npc", name: "Gremlin", attackable: true,
        dead: true }
    ];
    fire(targetPanel, "dblclick");
    check(results, "dead target dblclick posts nothing",
        posts.length === postsBefore7 + 1,
        "posts: " + JSON.stringify(
            posts.slice(postsBefore7).map((p) => p.options.body)));

    // ---- item status tooltip ----
    //
    // The custom DOM tooltip replaces the native title attribute of
    // every equipment widget cell. The markup, the CSS and the
    // renderItemTooltip function together build the family specific
    // layout (weapon / armor / jewelry / etc). The harness exercises
    // the markup, the css, the family classification and the per
    // family lines.

    check(results, "tooltip container exists in the html",
        html.includes('id="item-tooltip"') &&
        html.includes('class="item-tooltip'),
        "missing tooltip container");

    check(results, "tooltip css covers the family layouts",
        css.includes(".item-tooltip") &&
        css.includes(".item-tooltip.hidden") &&
        css.includes(".item-tooltip .tip-name") &&
        css.includes(".item-tooltip .tip-line") &&
        css.includes(".item-tooltip .tip-key") &&
        css.includes(".item-tooltip .tip-val") &&
        css.includes(".item-tooltip .tip-foot") &&
        css.includes(".item-tooltip.fam-weapon .tip-name") &&
        css.includes(".item-tooltip.fam-armor .tip-name") &&
        css.includes(".item-tooltip.fam-jewel .tip-name") &&
        css.includes(".item-tooltip.fam-etc .tip-name"),
        "missing tooltip css rules");

    // The family classifier drives the per family layout. A weapon
    // (XML type Weapon) lands in the weapon family, an armor piece in
    // the armor family, a jewelry entry (the bodypart key carries
    // the either-or mask of earrings/rings/necklace) in the jewel
    // family, adena and other etc items in the etc family.
    check(results, "itemFamily classifies weapons",
        gear.itemFamily({
            type: "Weapon", weaponType: "SWORD", bodyPartKey: "rhand"
        }) === "weapon",
        "weapon family wrong");
    check(results, "itemFamily classifies armor",
        gear.itemFamily({
            type: "Armor", armorType: "HEAVY", bodyPartKey: "chest"
        }) === "armor",
        "armor family wrong");
    check(results, "itemFamily classifies jewelry by bodypart",
        gear.itemFamily({
            type: "Armor", bodyPartKey: "rear;lear"
        }) === "jewel" &&
        gear.itemFamily({
            type: "Armor", bodyPartKey: "rfinger;lfinger"
        }) === "jewel" &&
        gear.itemFamily({
            type: "Armor", bodyPartKey: "neck"
        }) === "jewel",
        "jewel family wrong");
    check(results, "itemFamily classifies etc items",
        gear.itemFamily({
            type: "EtcItem", bodyPartKey: ""
        }) === "etc",
        "etc family wrong");

    // The tooltip HTML payload: a weapon carries the name, the
    // weapon type, the body part label, P. Atk, M. Atk, Atk. Spd,
    // SoulShot and Spiritshot. A shield carries the body part and
    // Shield Def / Block Rate instead of P. Def.
    const swordTip = gear.renderItemTooltip({
        name: "Short Sword", itemId: 1, count: 1, enchant: 0,
        equipped: true,
        type: "Weapon", weaponType: "SWORD", armorType: "",
        bodyPartKey: "rhand",
        pAtk: 8, mAtk: 6, pDef: 0, mDef: 0, sDef: 0, rShld: 0,
        pAtkSpd: 379, soulShots: 1, spiritShots: 1,
        weight: 1600, price: 768
    });
    check(results, "weapon tooltip renders the family lines",
        swordTip.includes("tip-name") &&
        swordTip.includes("Short Sword") &&
        swordTip.includes("SWORD") &&
        swordTip.includes("Right Hand") &&
        swordTip.includes("P. Atk") && swordTip.includes(">8<") &&
        swordTip.includes("M. Atk") && swordTip.includes(">6<") &&
        swordTip.includes("Atk. Spd") && swordTip.includes(">379<") &&
        swordTip.includes("SoulShot") && swordTip.includes(">x1<") &&
        swordTip.includes("Spiritshot") && swordTip.includes(">x1<") &&
        swordTip.includes("Weight") && swordTip.includes(">1600<") &&
        swordTip.includes("Sell"),
        "tip: " + swordTip.slice(0, 200));

    const armorTip = gear.renderItemTooltip({
        name: "Leather Tunic", itemId: 21, count: 1, enchant: 0,
        equipped: true,
        type: "Armor", weaponType: "", armorType: "LIGHT",
        bodyPartKey: "chest",
        pAtk: 0, mAtk: 0, pDef: 36, mDef: 0, sDef: 0, rShld: 0,
        pAtkSpd: 0, soulShots: 0, spiritShots: 0,
        weight: 1320, price: 12500
    });
    check(results, "armor tooltip renders the family lines",
        armorTip.includes("tip-name") &&
        armorTip.includes("Leather Tunic") &&
        armorTip.includes("LIGHT") &&
        armorTip.includes("Chest") &&
        armorTip.includes("P. Def") && armorTip.includes(">36<") &&
        !armorTip.includes("P. Atk") &&
        !armorTip.includes("SoulShot"),
        "tip: " + armorTip.slice(0, 200));

    const jewelTip = gear.renderItemTooltip({
        name: "Echo Crystal", itemId: 112, count: 1, enchant: 0,
        equipped: true,
        type: "Armor", weaponType: "", armorType: "",
        bodyPartKey: "rear;lear",
        pAtk: 0, mAtk: 0, pDef: 0, mDef: 11, sDef: 0, rShld: 0,
        pAtkSpd: 0, soulShots: 0, spiritShots: 0,
        weight: 150, price: 100
    });
    check(results, "jewel tooltip renders the family lines",
        jewelTip.includes("tip-name") &&
        jewelTip.includes("Echo Crystal") &&
        jewelTip.includes("Earring") &&
        jewelTip.includes("M. Def") && jewelTip.includes(">11<") &&
        !jewelTip.includes("P. Def"),
        "tip: " + jewelTip.slice(0, 200));

    const etcTip = gear.renderItemTooltip({
        name: "Adena", itemId: 57, count: 4242, enchant: 0,
        equipped: false,
        type: "EtcItem", weaponType: "", armorType: "",
        bodyPartKey: "",
        pAtk: 0, mAtk: 0, pDef: 0, mDef: 0, sDef: 0, rShld: 0,
        pAtkSpd: 0, soulShots: 0, spiritShots: 0,
        weight: 0, price: 0
    });
    check(results, "etc tooltip renders name, type and count",
        etcTip.includes("tip-name") &&
        etcTip.includes("Adena") &&
        etcTip.includes("Etc") &&
        etcTip.includes("x4242"),
        "tip: " + etcTip.slice(0, 200));

    check(results, "tooltip omits empty fields",
        !etcTip.includes("P. Atk") &&
        !etcTip.includes("Weight"),
        "tip should omit empty fields: " + etcTip.slice(0, 200));

    // The enchant prefix is rendered as a green +N span in front of
    // the item name.
    const enchantedTip = gear.renderItemTooltip({
        name: "Long Sword", itemId: 2, count: 1, enchant: 3,
        equipped: false,
        type: "Weapon", weaponType: "SWORD", armorType: "",
        bodyPartKey: "rhand",
        pAtk: 24, mAtk: 17, pDef: 0, mDef: 0, sDef: 0, rShld: 0,
        pAtkSpd: 379, soulShots: 2, spiritShots: 2,
        weight: 1560, price: 136000
    });
    check(results, "tooltip renders the enchant prefix",
        enchantedTip.includes("tip-enchant") &&
        enchantedTip.includes("+3"),
        "tip: " + enchantedTip.slice(0, 120));

    // ---- shop queue widget ----
    //
    // The purchase plan of the bot renders as a flyout of the
    // equipment panel: a small triangle tab on the left edge slides
    // the queue out to the left (the equipment panel itself never
    // resizes), the keyed rows of the published queue (snap.shopping),
    // the head summary, the pinned buy/have/save foot and the rich
    // purchase tooltip to the left of the hovered row. The harness
    // exercises the markup, the css, the rendering, the keying and
    // the tab toggle.

    check(results, "shop queue markup lives in the equipment panel as an edge tab and a flyout",
        html.includes('<button id="shop-tab"') &&
        html.includes('id="shop-panel"') &&
        html.includes('<div id="shop-head"') &&
        html.includes('id="shop-list"') &&
        html.includes('id="shop-buy"') &&
        html.includes('id="shop-have"') &&
        html.includes('id="shop-save"') &&
        html.indexOf('id="gear-panel"') < html.indexOf('id="shop-tab"') &&
        html.indexOf('id="inv-grid"') < html.indexOf('id="shop-tab"') &&
        html.indexOf('id="shop-panel"') > html.indexOf('id="gear-trash"'),
        "missing shop queue markup or wrong placement");

    check(results, "shop queue css covers the flyout chrome",
        css.includes(".shop-panel") &&
        css.includes(".shop-panel.open") &&
        css.includes(".shop-tab") &&
        css.includes("right: calc(100% + 14px)") &&
        css.includes(".shop-head") &&
        css.includes(".shop-summary") &&
        css.includes(".shop-list") &&
        css.includes("overflow-y: auto") &&
        css.includes(".shop-item") &&
        css.includes(".shop-item.want") &&
        css.includes(".shop-item.buying") &&
        css.includes(".shop-foot") &&
        css.includes(".item-tooltip .tip-plan"),
        "missing shop queue css rules");

    // The published plan: two affordable buys and one wanted entry.
    const shopPlan = {
        entries: [
            {
                itemId: 1121, name: "Apprentice's Shoes", icon: "icon1121",
                merchant: "Ariel", type: "Armor", armorType: "LIGHT",
                bodyPartKey: "feet", pDef: 8, weight: 210, price: 9,
                sellCredit: 0, missing: 0, gain: 8, affordable: true,
                buying: false
            },
            {
                itemId: 1146, name: "Cloth Cap", icon: "icon1146",
                merchant: "Ariel", type: "Armor", armorType: "LIGHT",
                bodyPartKey: "head", pDef: 10, weight: 40, price: 20,
                sellCredit: 0, missing: 0, gain: 10, affordable: true,
                buying: false
            },
            {
                itemId: 1, name: "Short Sword", icon: "icon1",
                merchant: "Unoren", type: "Weapon", weaponType: "SWORD",
                bodyPartKey: "rhand", pAtk: 8, pAtkSpd: 379, weight: 1600,
                price: 883, sellCredit: 0, missing: 383, gain: 3.5,
                affordable: false, buying: false
            }
        ],
        adena: 500,
        total: 29,
        trip: false
    };
    const shopSnapshot = gearSnapshot([], 0, 80);
    shopSnapshot.shopping = shopPlan;

    gear.renderShopping(shopSnapshot);
    const shopPanel = elements.get("shop-panel");
    const shopList = elements.get("shop-list");
    const shopSummary = elements.get("shop-summary");
    const shopTab = elements.get("shop-tab");
    const shopTabChev = elements.get("shop-tab-chev");
    check(results, "a published plan shows the flyout and its edge tab",
        !shopPanel.classList.contains("hidden") &&
        !shopTab.classList.contains("hidden"),
        "the flyout or the tab stays hidden with a plan");
    check(results, "every queue entry renders its row",
        shopList.children.length === 3,
        "got " + shopList.children.length + " rows");
    check(results, "the rows show the item names and prices",
        deepText(shopList.children[0]).includes("Apprentice's Shoes") &&
        deepText(shopList.children[0]).includes("9") &&
        deepText(shopList.children[2]).includes("Short Sword") &&
        deepText(shopList.children[2]).includes("883"),
        "row text: " + deepText(shopList.children[0]));
    check(results, "the wanted row carries the missing adena",
        deepText(shopList.children[2]).includes("need 383") &&
        shopList.children[2].classList.contains("want"),
        "row text: " + deepText(shopList.children[2]));
    check(results, "the affordable rows carry no missing adena",
        !deepText(shopList.children[0]).includes("need") &&
        !shopList.children[0].classList.contains("want"),
        "row text: " + deepText(shopList.children[0]));
    check(results, "the head summary carries the queue shape",
        deepText(shopSummary).includes("3") &&
        deepText(shopSummary).includes("29") &&
        deepText(shopSummary).includes("save 383"),
        "summary: " + deepText(shopSummary));
    check(results, "the foot carries buy, have and save",
        elements.get("shop-buy").textContent === "29" &&
        elements.get("shop-have").textContent === "500" &&
        elements.get("shop-save").textContent === "383",
        "buy/have/save: " + elements.get("shop-buy").textContent +
        "/" + elements.get("shop-have").textContent + "/" +
        elements.get("shop-save").textContent);

    // Keyed rendering: the same plan re-rendered keeps the row icon
    // image elements alive (the icons never blink).
    const rowImg = shopList.children[0].children[0].children[0];
    gear.renderShopping(shopSnapshot);
    check(results, "re-rendering the same plan keeps the icons",
        shopList.children[0].children[0].children[0] === rowImg,
        "the icon image element was recreated");

    // A vanished entry drops its row; the remaining rows stay keyed.
    const shrunk = JSON.parse(JSON.stringify(shopPlan));
    shrunk.entries = shrunk.entries.slice(0, 2);
    const shrunkSnapshot = gearSnapshot([], 0, 80);
    shrunkSnapshot.shopping = shrunk;
    gear.renderShopping(shrunkSnapshot);
    check(results, "a vanished queue entry drops its row",
        shopList.children.length === 2 &&
        deepText(shopList.children[1]).includes("Cloth Cap"),
        "got " + shopList.children.length + " rows");

    // The trip view: the buying entry carries the chip.
    const tripPlan = JSON.parse(JSON.stringify(shopPlan));
    tripPlan.trip = true;
    tripPlan.entries[0].buying = true;
    const tripSnapshot = gearSnapshot([], 0, 80);
    tripSnapshot.shopping = tripPlan;
    gear.renderShopping(tripSnapshot);
    check(results, "the buying entry of a trip carries its chip",
        shopList.children[0].classList.contains("buying") &&
        deepText(shopList.children[0]).includes("buying") &&
        deepText(shopSummary).includes("left"),
        "row text: " + deepText(shopList.children[0]));

    // No plan hides the widget again - the flyout and the tab both.
    gear.renderShopping(gearSnapshot([], 0, 80));
    check(results, "no plan hides the flyout and the edge tab",
        shopPanel.classList.contains("hidden") &&
        shopTab.classList.contains("hidden") &&
        shopList.children.length === 0,
        "the flyout or the tab stays visible without a plan");

    // The tab toggle: the triangle click slides the flyout in and
    // back out, the glyph flips with the state and the tab answers
    // with the aria state.
    gear.renderShopping(shopSnapshot);
    check(results, "the flyout starts slid out for the passive glance",
        shopPanel.classList.contains("open") &&
        shopTabChev.textContent === "\u25B8" &&
        shopTab.getAttribute("aria-expanded") === "true",
        "the flyout did not start open");
    fire(shopTab, "click");
    check(results, "the tab click slides the flyout in",
        !shopPanel.classList.contains("open") &&
        shopTabChev.textContent === "\u25C2" &&
        shopTab.getAttribute("aria-expanded") === "false",
        "the flyout did not slide in");
    fire(shopTab, "click");
    check(results, "the second tab click slides the flyout out again",
        shopPanel.classList.contains("open") &&
        shopTabChev.textContent === "\u25B8",
        "the flyout did not slide out");

    // The purchase tooltip: the item shape plus the planning lines
    // (the gain, the value per adena, the sell credit, the missing
    // adena and the pick status).
    const swordShopTip = gear.renderShoppingTooltip(shopPlan.entries[2]);
    check(results, "purchase tooltip renders the plan lines",
        swordShopTip.includes("tip-name") &&
        swordShopTip.includes("Short Sword") &&
        swordShopTip.includes("Unoren") &&
        swordShopTip.includes("Gain") &&
        swordShopTip.includes("Value / adena") &&
        swordShopTip.includes("Missing") &&
        swordShopTip.includes("383") &&
        swordShopTip.includes("saving up") &&
        swordShopTip.includes("tip-plan"),
        "tip: " + swordShopTip.slice(0, 200));
    const shoesShopTip = gear.renderShoppingTooltip(shopPlan.entries[0]);
    check(results, "affordable purchase tooltip shows the trip status",
        shoesShopTip.includes("next trip") &&
        !shoesShopTip.includes("Missing"),
        "tip: " + shoesShopTip.slice(0, 200));

    // ---- toolbar view dropdown ----
    //
    // The map toolbar folds into a single row: the -/+ zoom buttons
    // are gone (the wheel owns the zoom) and the layer checkboxes
    // (labels, paths, zone, targets, hunt zones, aggro, map bg) live
    // in a dropdown that opens under the "view" button.

    check(results, "toolbar markup drops the zoom buttons and carries the view dropdown",
        !html.includes('id="zoom-in"') &&
        !html.includes('id="zoom-out"') &&
        html.includes('<div id="view-menu"') &&
        html.includes('id="view-menu-btn"') &&
        html.includes('id="view-menu-pop"') &&
        html.indexOf('id="view-menu-btn"') <
            html.indexOf('id="view-menu-pop"'),
        "zoom buttons remain or dropdown markup missing");

    check(results, "the layer checkboxes live inside the dropdown pop",
        ["show-labels", "show-dest", "show-zone", "show-targets",
            "show-hunt-zones", "show-aggro", "show-map"]
            .every((id) =>
                html.indexOf('id="' + id + '"') >
                html.indexOf('id="view-menu-pop"')) &&
        html.indexOf('id="show-map"') < html.indexOf('id="map-scale"'),
        "a layer checkbox sits outside the pop");

    check(results, "the dropdown css keeps the pop under the button and the toolbar on one row",
        css.includes(".view-menu-pop") &&
        css.includes(".view-menu.open") &&
        css.includes("top: calc(100% + 6px)") &&
        css.includes("z-index: 40") &&
        css.slice(css.indexOf(".map-toolbar {"),
            css.indexOf(".map-toolbar {") + 260)
            .includes("flex-wrap: nowrap") &&
        css.includes("body.mode-pathfind .view-menu .bot-layer"),
        "missing dropdown css rules");

    // The dropdown behavior: the button toggles the pop, an outside
    // click closes it, the Escape key closes it. The pop clicks stop
    // their propagation, so the checklist survives several toggles.
    const viewMenu = elements.get("view-menu");
    const viewBtn = elements.get("view-menu-btn");
    const viewPop = elements.get("view-menu-pop");
    const docListeners = (sandbox.document || {}).listeners || {};
    const fireDoc = (type, event) => {
        for (const handler of docListeners[type] || []) {
            handler(event);
        }
    };
    check(results, "the dropdown starts closed",
        viewPop.classList.contains("hidden") &&
        !viewMenu.classList.contains("open") &&
        viewBtn.getAttribute("aria-expanded") === "false",
        "the dropdown did not start closed");
    fire(viewBtn, "click");
    check(results, "the view button click opens the dropdown",
        !viewPop.classList.contains("hidden") &&
        viewMenu.classList.contains("open") &&
        viewBtn.getAttribute("aria-expanded") === "true",
        "the dropdown did not open");
    fire(viewPop, "click");
    check(results, "a click inside the checklist keeps it open",
        !viewPop.classList.contains("hidden") &&
        viewBtn.getAttribute("aria-expanded") === "true",
        "a checklist click closed the dropdown");
    fire(viewBtn, "click");
    check(results, "the second view button click closes the dropdown",
        viewPop.classList.contains("hidden") &&
        viewBtn.getAttribute("aria-expanded") === "false",
        "the dropdown did not close");
    fire(viewBtn, "click");
    fireDoc("click", { target: null });
    check(results, "an outside click closes the dropdown",
        viewPop.classList.contains("hidden") &&
        viewBtn.getAttribute("aria-expanded") === "false",
        "the outside click did not close it");
    fire(viewBtn, "click");
    fireDoc("keydown", { key: "Escape" });
    check(results, "the Escape key closes the dropdown",
        viewPop.classList.contains("hidden") &&
        viewBtn.getAttribute("aria-expanded") === "false",
        "the escape key did not close it");

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
