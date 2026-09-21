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
//   chip and HP/MP bars, and is hidden completely when the target is
//   killed, removed or missing (no placeholder state);
// - the experience bar is fed from expPercent (the bot computes it
//   from the C1 experience table) and renders the fill and the
//   percentage text;
// - the HP and MP bars render cur/max values into the fill and text,
//   and show "—" while the server maximum is unknown;
// - renderHUD no longer writes the target into the character panel
//   (renderTarget owns the target panel);
// - the bot status banner maps the hunt loop phase to a human readable
//   activity (hunting, walking to town, selling, deleveling) and the
//   data-kind attribute colors the banner by activity.
//
// Usage: node tools/repro_hud.js [--app <app.js>]
// Exit code 0 = HUD rendering is correct, 1 = bug reproduced.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const DEFAULT_APP_JS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "app.js");
const DEFAULT_STYLE_CSS = path.join(__dirname, "..", "internal", "swarm",
    "webserver", "web", "style.css");

// makeElement returns a DOM element stub recording the last written
// textContent, style and class toggles.
function makeElement() {
    return {
        textContent: "",
        style: {},
        value: "",
        checked: true,
        children: [],
        dataset: {},
        append: function (...added) {
            for (const child of added) { this.children.push(child); }
        },
        appendChild: function (child) {
            this.children.push(child);

            return child;
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
        // the view layers dropdown of the toolbar syncs its
        // aria-expanded attribute at init; record it.
        attributes: {},
        setAttribute(key, value) {
            this.attributes[key] = String(value);
        },
        getAttribute(key) {
            return this.attributes[key];
        },
        _innerHTML: "",
        get innerHTML() { return this._innerHTML; },
        set innerHTML(value) {
            this._innerHTML = value;
            this.children.length = 0;
        },
        addEventListener: () => {},
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
        createElement: () => makeElement(),
        documentElement: { dataset: {} }
    };
    const sandbox = {
        Math, JSON, Number, Date, isNaN,
        URLSearchParams,
        window: { localStorage: {
            getItem: () => null, setItem: () => {}
        } },
        document,
        // The chat input and the other command posters ride fetch:
        // record the posts so the harness can assert the body.
        fetch: (url, options) => {
            sandbox.__posts.push({ url, options });

            return { catch: () => {} };
        },
        __posts: [],
        // app.js renderZones calls MapView.blurZone() (map.js owns the
        // real one) when it rebuilds the zone list: a no-op stub keeps
        // the harness free of the map bundle.
        MapView: { blurZone: () => {} },
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
        " renderChat: typeof renderChat === 'function'" +
        " ? renderChat : undefined," +
        " chatAtBottom: typeof chatAtBottom === 'function'" +
        " ? chatAtBottom : undefined," +
        " ChatWindow: typeof ChatWindow === 'undefined'" +
        " ? undefined : ChatWindow," +
        " chatTabAccepts: typeof chatTabAccepts === 'function'" +
        " ? chatTabAccepts : undefined," +
        " setChatTab: typeof setChatTab === 'function'" +
        " ? setChatTab : undefined," +
        " sendChatInput: typeof sendChatInput === 'function'" +
        " ? sendChatInput : undefined," +
        " walkDetail: typeof walkDetail === 'function'" +
        " ? walkDetail : undefined," +
        " engageFightDetail: typeof engageFightDetail === 'function'" +
        " ? engageFightDetail : undefined," +
        " renderHUD: typeof renderHUD === 'function'" +
        " ? renderHUD : undefined," +
        " renderBotStatus: typeof renderBotStatus === 'function'" +
        " ? renderBotStatus : undefined," +
        " phaseLabel: typeof phaseLabel === 'function'" +
        " ? phaseLabel : undefined," +
        " renderBuffs: typeof renderBuffs === 'function'" +
        " ? renderBuffs : undefined," +
        " buildPathfindLink: typeof buildPathfindLink === 'function'" +
        " ? buildPathfindLink : undefined };",
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

    // The bot has no target: the whole panel must be hidden, not shown
    // with a placeholder.
    hud.renderTarget(snapshotWith(0));
    check(results, "no target hides the target panel",
        elements.get("hud-target").classList.contains("hidden"),
        "hidden class missing");

    // The target died: the tracker cleared it, but a snapshot that
    // still carries the dead id (for example a race between the kill
    // and the SSE delivery) must not show a stale panel either.
    const deadTarget = [{
        objectId: 300, kind: "npc", name: "Keltir", level: 2, dead: true
    }];
    hud.renderTarget(snapshotWith(300, deadTarget));
    check(results, "dead target keeps the target panel hidden",
        elements.get("hud-target").classList.contains("hidden"),
        "hidden class missing");

    // A living target renders with its name, level chip and vitals.
    const liveTarget = [{
        objectId: 300, kind: "npc", name: "Keltir", level: 2, dead: false,
        curHp: 40, maxHp: 50
    }];
    hud.renderTarget(snapshotWith(300, liveTarget));
    check(results, "living target shows the panel",
        !elements.get("hud-target").classList.contains("hidden"),
        "hidden class still present");
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

    // The kill ETA chip rides the target panel: the diagnostics carry
    // the estimate (0 = not available), the chip appears with the
    // rounded seconds and hides without a running estimate.
    sandbox.document.getElementById("target-eta");
    const etaChip = elements.get("target-eta");
    hud.renderTarget(snapshotWith(300, liveTarget));
    check(results, "no diagnostics hides the kill eta chip",
        etaChip.classList.contains("hidden"),
        "the chip showed without diagnostics");
    hud.renderTarget(Object.assign(snapshotWith(300, liveTarget), {
        diagnostics: { hunt: { killEtaMs: 12400 } }
    }));
    check(results, "the kill eta chip shows the rounded seconds",
        etaChip.textContent === "~12s" &&
        !etaChip.classList.contains("hidden"),
        "got " + JSON.stringify(etaChip.textContent));
    hud.renderTarget(Object.assign(snapshotWith(300, liveTarget), {
        diagnostics: { hunt: { killEtaMs: 0 } }
    }));
    check(results, "a zero kill eta hides the chip",
        etaChip.classList.contains("hidden"),
        "the chip stayed visible without an estimate");

    // The walk detail and the fight detail append the ETA suffix: the
    // walk ETA on the walk phases, the kill ETA on the fight, both
    // rounded to whole seconds and absent at zero.
    if (typeof hud.walkDetail !== "function"
        || typeof hud.engageFightDetail !== "function") {
        check(results, "the detail builders expose the eta suffix", false,
            "walkDetail/engageFightDetail missing from app.js");
    } else {
        const walk = hud.walkDetail("",
            { waypointsLeft: 3, tripForMs: 0, walkEtaMs: 45300 });
        check(results, "the walk detail carries the walk eta",
            walk === "3 waypoints left, eta ~45s",
            "got " + JSON.stringify(walk));
        const walkNoEta = hud.walkDetail("heading back",
            { waypointsLeft: 0, tripForMs: 0, walkEtaMs: 0 });
        check(results, "the walk detail omits an absent eta",
            walkNoEta === "heading back",
            "got " + JSON.stringify(walkNoEta));
        const fight = hud.engageFightDetail(
            { objects: [{ objectId: 300, name: "Keltir", dead: false }] },
            { targetId: 300 },
            { targetId: 300, targetForMs: 4000, killEtaMs: 9000 });
        check(results, "the fight detail carries the target name",
            fight === "fighting Keltir for 4s, eta ~9s",
            "got " + JSON.stringify(fight));
        const fightUnnamed = hud.engageFightDetail(
            { objects: [] },
            { targetId: 301 },
            null);
        check(results, "an unknown target never shows the raw id",
            fightUnnamed === "fighting a target",
            "got " + JSON.stringify(fightUnnamed));
    }

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
        const sittingSnap = snapshotWith(300, liveTarget);
        sittingSnap.character.sitting = true;
        hud.renderHUD(sittingSnap);
        check(results, "renderHUD shows the rest chip while sitting",
            !elements.get("hud-rest").classList.contains("hidden"),
            "rest chip hidden while sitting");
        hud.renderHUD(snapshotWith(300, liveTarget));
        check(results, "renderHUD hides the rest chip while standing",
            elements.get("hud-rest").classList.contains("hidden"),
            "rest chip visible while standing");
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

    // The chat window renders one line per system or social message
    // and clears when the snapshot carries none.
    sandbox.document.getElementById("chat-list");
    if (typeof hud.renderChat !== "function") {
        check(results, "chat window renders the message lines", false,
            "renderChat is missing from app.js");
    } else {
        hud.renderChat({ chat: [
            { time: "2026-09-06T10:00:00Z", kind: "system",
                text: "You picked up 25 adena." },
            { time: "2026-09-06T10:00:01Z", kind: "social",
                text: "Gremlin plays social animation 2" }
        ] });
        const chatList = elements.get("chat-list");
        check(results, "chat window renders the message lines",
            chatList.children.length === 2,
            "got " + chatList.children.length + " lines");
        if (chatList.children.length === 2) {
            check(results, "chat line carries the message text",
                chatList.children[0].children[1].textContent
                    === "You picked up 25 adena.",
                "got " + JSON.stringify(
                    chatList.children[0].children[1].textContent));
            check(results, "social lines carry the social class",
                chatList.children[1].className === "chat-line chat-social",
                "got " + JSON.stringify(chatList.children[1].className));
        }
        hud.renderChat({ chat: [] });
        check(results, "empty chat clears the window",
            chatList.children.length === 0,
            "got " + chatList.children.length + " lines");

        // The vertical rhythm of the chat lines is a whole pixel line
        // height pinned on .chat-line: a unitless ratio (the old 1.6
        // at 11px = 17.6px) rounds per row at paint time and the
        // distance between the lines drifts apart on some rows. The
        // rule lives in the real style.css, so the check reads it.
        const chatCss = fs.readFileSync(DEFAULT_STYLE_CSS, "utf8");
        const chatLineRule = chatCss.match(/\.chat-line\s*\{[^}]*\}/);
        check(results, "chat lines pin a whole pixel line height",
            Boolean(chatLineRule) && /line-height:\s*\d+px/
                .test(chatLineRule[0]),
            "rule: " + (chatLineRule ? chatLineRule[0]
                : ".chat-line missing"));

        // Auto scroll follows the newest line only while stuck: the
        // default state scrolls to the bottom, a scrolled up user keeps
        // the chosen view, reaching the bottom resumes the follow.
        const scrollList = { scrollTop: 0, clientHeight: 300,
            scrollHeight: 500,
            children: [], innerHTML: "",
            append(child) { this.children.push(child); } };
        hud.ChatWindow.stick = true;
        hud.renderChat.call(null, { chat: [
            { time: "2026-09-06T10:00:00Z", kind: "system", text: "l1" }
        ] });
        // renderChat resolves the element by id: point the sandbox at
        // the scroll probe list.
        elements.set("chat-list", scrollList);
        hud.renderChat({ chat: [
            { time: "2026-09-06T10:00:00Z", kind: "system", text: "l1" }
        ] });
        check(results, "stuck chat scrolls to the newest line",
            scrollList.scrollTop === scrollList.scrollHeight,
            "scrollTop " + scrollList.scrollTop);
        hud.ChatWindow.stick = false;
        scrollList.scrollTop = 120;
        hud.renderChat({ chat: [
            { time: "2026-09-06T10:00:01Z", kind: "system", text: "l2" }
        ] });
        check(results, "scrolled up chat keeps the chosen view",
            scrollList.scrollTop === 120,
            "scrollTop " + scrollList.scrollTop);
        check(results, "chatAtBottom detects the bottom",
            hud.chatAtBottom({ scrollTop: 196, clientHeight: 300,
                scrollHeight: 500 })
            && !hud.chatAtBottom({ scrollTop: 120, clientHeight: 300,
                scrollHeight: 500 }),
            "bottom detection broken");
        elements.set("chat-list", chatList);

        // The ALL / CHAT / SYSTEM tabs split the world chat lines
        // (the CreatureSay packets) from the bot system messages: the
        // chat tab keeps only the chat kinds, the system tab only the
        // system and social ones, the all tab everything. The world
        // chat lines carry the sender as its own column.
        if (typeof hud.chatTabAccepts !== "function"
            || typeof hud.setChatTab !== "function") {
            check(results, "chat window exposes the tab filter", false,
                "chatTabAccepts/setChatTab missing from app.js");
        } else {
            hud.ChatWindow.stick = false;
            hud.setChatTab("all");
            const world = [
                { time: "2026-09-06T10:00:00Z", kind: "say",
                    from: "Melg", text: "hello" },
                { time: "2026-09-06T10:00:01Z", kind: "system",
                    text: "You picked up 25 adena." }
            ];
            hud.renderChat({ chat: world });
            const allList = elements.get("chat-list");
            check(results, "all tab renders every line",
                allList.children.length === 2,
                "got " + allList.children.length + " lines");
            check(results, "world chat line renders the sender column",
                allList.children[0].children.length === 3
                && allList.children[0].children[1].textContent
                    === "Melg:",
                "got " + JSON.stringify(
                    allList.children[0].children.map(
                        (c) => c.textContent)));

            hud.setChatTab("chat");
            hud.renderChat({ chat: world });
            const chatOnly = elements.get("chat-list");
            check(results, "chat tab keeps only the chat kinds",
                chatOnly.children.length === 1
                && chatOnly.children[0].className === "chat-line chat-say",
                "got " + chatOnly.children.length + " lines: "
                + JSON.stringify(chatOnly.children.map(
                    (c) => c.className)));

            hud.setChatTab("system");
            hud.renderChat({ chat: world });
            const sysOnly = elements.get("chat-list");
            check(results, "system tab keeps only the system kinds",
                sysOnly.children.length === 1
                && sysOnly.children[0].className
                    === "chat-line chat-system",
                "got " + sysOnly.children.length + " lines");
            check(results, "system line renders without a sender",
                sysOnly.children[0].children.length === 2,
                "got " + sysOnly.children[0].children.length
                + " columns");
            hud.setChatTab("all");
        }

        // The chat input posts a say command for the active bot: the
        // text rides the body, the whisper channel adds the target and
        // the input clears after the send.
        if (typeof hud.sendChatInput !== "function") {
            check(results, "chat input posts a say command", false,
                "sendChatInput missing from app.js");
        } else {
            sandbox.__posts.length = 0;
            vm.runInContext("App.activeBotId = 'acc1'", sandbox);
            sandbox.document.getElementById("chat-input");
            sandbox.document.getElementById("chat-channel");
            sandbox.document.getElementById("chat-whisper");
            const chatInput = elements.get("chat-input");
            const channelInput = elements.get("chat-channel");
            const whisperInput = elements.get("chat-whisper");
            channelInput.value = "0";
            whisperInput.value = "";
            chatInput.value = "  hello there  ";
            hud.sendChatInput();
            const posted = sandbox.__posts.length === 1
                ? JSON.parse(sandbox.__posts[0].options.body) : null;
            check(results, "chat input posts to the command endpoint",
                sandbox.__posts.length === 1
                && sandbox.__posts[0].url
                    === "/api/bots/acc1/commands",
                "got " + JSON.stringify(sandbox.__posts));
            check(results, "chat input trims and posts the text",
                posted !== null && posted.kind === "say"
                && posted.text === "hello there"
                && posted.channel === 0,
                "got " + JSON.stringify(posted));
            check(results, "chat input clears after the send",
                chatInput.value === "",
                "got " + JSON.stringify(chatInput.value));

            channelInput.value = "2";
            chatInput.value = "psst";
            whisperInput.value = "Melg";
            hud.sendChatInput();
            const whisper = sandbox.__posts.length === 2
                ? JSON.parse(sandbox.__posts[1].options.body) : null;
            check(results, "whisper send carries the recipient",
                whisper !== null && whisper.channel === 2
                && whisper.target === "Melg",
                "got " + JSON.stringify(whisper));

            chatInput.value = "   ";
            const before = sandbox.__posts.length;
            hud.sendChatInput();
            check(results, "empty chat input posts nothing",
                sandbox.__posts.length === before
                && chatInput.value === "   ",
                "posts grew by "
                + (sandbox.__posts.length - before));
            chatInput.value = "";
        }
    }

    // The bot status banner maps the hunt loop phase to a human
    // readable activity and the data-kind attribute colors the banner
    // by activity. Every phase of the hunt loop must resolve to a
    // label and a kind so the banner reads correctly.
    sandbox.document.getElementById("bot-status");
    sandbox.document.getElementById("bot-status-text");
    sandbox.document.getElementById("bot-status-detail");
    if (typeof hud.renderBotStatus !== "function") {
        check(results, "renderBotStatus writes the activity text",
            false, "renderBotStatus is missing from app.js");
        check(results, "renderBotStatus colors the banner by activity",
            false, "renderBotStatus is missing from app.js");
        check(results, "renderBotStatus falls back to status text",
            false, "renderBotStatus is missing from app.js");
    } else {
        const baseSnap = {
            status: "online",
            character: { inCombat: false, moving: false },
            phase: ""
        };
        const cases = [
            { phase: "engage", inCombat: false, wantKind: "hunt",
                wantText: "hunting" },
            { phase: "engage", inCombat: true, wantKind: "combat",
                wantText: "hunting" },
            { phase: "loot", inCombat: false, wantKind: "loot",
                wantText: "looting" },
            { phase: "townWalk", inCombat: false, wantKind: "town",
                wantText: "walking to town" },
            { phase: "townSell", inCombat: false, wantKind: "town",
                wantText: "selling" },
            { phase: "townReturn", inCombat: false, wantKind: "return",
                wantText: "walking to farm spot" },
            { phase: "delevel", inCombat: false, wantKind: "delevel",
                wantText: "deleveling" },
            { phase: "user", inCombat: false, moving: true,
                wantKind: "user", wantText: "manual · moving" },
            { phase: "user", inCombat: true, wantKind: "combat",
                wantText: "manual · attacking" },
            { phase: "idle", inCombat: false, wantKind: "idle",
                wantText: "idle" }
        ];
        let allLabels = true;
        for (const c of cases) {
            const snap = Object.assign({}, baseSnap, {
                phase: c.phase,
                character: { inCombat: c.inCombat, moving: c.moving }
            });
            const label = hud.phaseLabel(snap);
            const labelOk = label && label.kind === c.wantKind &&
                label.text === c.wantText;
            check(results,
                "phaseLabel " + c.phase +
                (c.inCombat ? " combat" : c.moving ? " moving" : "") +
                " -> " + c.wantKind + " / " + c.wantText,
                Boolean(labelOk),
                "got " + JSON.stringify(label));
            if (!labelOk) { allLabels = false; }
            hud.renderBotStatus(snap);
            const banner = elements.get("bot-status");
            const textOk = elements.get("bot-status-text").textContent
                === c.wantText;
            const kindOk = banner.dataset.kind === c.wantKind;
            check(results,
                "renderBotStatus " + c.phase + " writes text and kind",
                textOk && kindOk,
                "text=" + JSON.stringify(
                    elements.get("bot-status-text").textContent) +
                " kind=" + JSON.stringify(banner.dataset.kind));
            if (!textOk || !kindOk) { allLabels = false; }
        }
        check(results, "every hunt phase resolves to a label",
            allLabels, "see the case failures above");

        // The session status takes precedence when no phase is
        // published (the manual only sessions never set it): the
        // banner falls back to the connecting and offline status.
        const connecting = hud.phaseLabel({ status: "connecting" });
        check(results, "connecting status banner",
            connecting.kind === "connecting" &&
            connecting.text === "connecting",
            "got " + JSON.stringify(connecting));
        const offline = hud.phaseLabel({ status: "offline" });
        check(results, "offline status banner",
            offline.kind === "offline" && offline.text === "offline",
            "got " + JSON.stringify(offline));

        // The detail line carries the human readable description so
        // the user sees what every activity means at a glance.
        const detailSnap = Object.assign({}, baseSnap, {
            phase: "townSell"
        });
        hud.renderBotStatus(detailSnap);
        const detailText = elements.get("bot-status-detail")
            .textContent;
        check(results, "renderBotStatus carries the detail text",
            typeof detailText === "string" && detailText.length > 0,
            "got " + JSON.stringify(detailText));
    }

    // The pathfind link button freezes the live walk into the 3D
    // navmesh viewer URL: the from/to pair off the walk plan, the
    // tiles around the pair, the defaults of the
    // viewer toggles and the three quarter orbit camera computed with
    // the viewer framing math. No filter parameter exists anymore:
    // every search prices the water at the swim rate.
    if (typeof hud.buildPathfindLink !== "function") {
        check(results, "buildPathfindLink exists", false,
            "app.js carries no buildPathfindLink");
    } else {
        const walkSnap = snapshotWith(0);
        walkSnap.walkOrigin = { x: 45257, y: 49353, z: -3059 };
        walkSnap.walkPath = [
            { x: 40000, y: 50000, z: -3100 },
            { x: 25500, y: 51095, z: -3408 }
        ];
        walkSnap.walkDest = { x: 25500, y: 51095, z: -3408 };
        const url = hud.buildPathfindLink(walkSnap);
        check(results, "the pathfind link opens the viewer base",
            url.startsWith("http://127.0.0.1:8082/?"), "got " + url);
        const query = new URLSearchParams(
            url.slice(url.indexOf("?") + 1));
        check(results, "the pathfind link carries the from pair",
            query.get("from") === "45257,49353,-3059",
            "got " + JSON.stringify(query.get("from")));
        check(results, "the pathfind link carries the to pair",
            query.get("to") === "25500,51095,-3408",
            "got " + JSON.stringify(query.get("to")));
        const cam = (query.get("cam") || "").split(",").map(Number);
        check(results, "the pathfind link camera has five numbers",
            cam.length === 5 && cam.every((n) => Number.isFinite(n)),
            "got " + JSON.stringify(query.get("cam")));
        check(results, "the camera flies south east above the route",
            cam[0] > 45257 && cam[1] > 51095 && cam[2] > 0,
            "got " + JSON.stringify(cam.slice(0, 3)));
        check(results, "the camera yaw faces the route",
            Math.abs(cam[3] - Math.PI / 4) < 0.01,
            "got yaw " + cam[3]);
        check(results, "the camera pitch looks down",
            cam[4] < -0.3, "got pitch " + cam[4]);
        const tiles = (query.get("tiles") || "").split(",");
        check(results, "the tiles cover the route neighborhood",
            ["20_19", "20_20", "21_19", "21_20"].every(
                (t) => tiles.includes(t)),
            "got " + JSON.stringify(tiles));
        check(results, "the viewer defaults ride the link",
            !query.has("filter") &&
            query.get("scale") === "1" &&
            query.get("geom") === "mesh" &&
            query.get("path") === "smooth",
            "got " + url);
        check(results, "the default link arms no repro contract",
            !query.has("approach") && !query.has("avoid") &&
            !query.has("fold"), "got " + url);

        // The plan's mesh search contract rides the link (the repro
        // guarantee of the 2026-09-19 route mismatch round): the zone
        // return plans emit the approach radius and the ban circles,
        // and the plan repro mode (fold=0) so the viewer serves the
        // search answer the bot publishes instead of the water blind
        // grid fold.
        const planSnap = snapshotWith(0);
        planSnap.walkOrigin = { x: 46045, y: 41251, z: -3504 };
        planSnap.walkPath = [{ x: 36184, y: 46744, z: -3720 }];
        planSnap.walkDest = { x: 36000, y: 46765, z: -3712 };
        planSnap.walkSearch = {
            approach: 200,
            avoid: [{ x: 43000, y: 42000, r: 300 }],
        };
        const planUrl = hud.buildPathfindLink(planSnap);
        const planQuery = new URLSearchParams(
            planUrl.slice(planUrl.indexOf("?") + 1));
        check(results, "the plan link carries no filter word",
            !planQuery.has("filter"), "got " + planUrl);
        check(results, "the plan link carries the approach radius",
            planQuery.get("approach") === "200", "got " + planUrl);
        check(results, "the plan link carries the ban circles",
            planQuery.get("avoid") === "43000,42000,300",
            "got " + planUrl);
        check(results, "the plan link arms the plan repro mode",
            planQuery.get("fold") === "0", "got " + planUrl);
        check(results, "the plan link keeps the pair and the camera",
            planQuery.get("from") === "46045,41251,-3504" &&
            planQuery.get("to") === "36000,46765,-3712" &&
            planQuery.has("cam") && planQuery.has("tiles"),
            "got " + planUrl);

        const cleanSearchSnap = snapshotWith(0);
        cleanSearchSnap.walkOrigin = { x: 45000, y: 50000, z: -3500 };
        cleanSearchSnap.walkPath = [{ x: 46200, y: 51100, z: -3500 }];
        cleanSearchSnap.walkDest = { x: 46200, y: 51100, z: -3500 };
        cleanSearchSnap.walkSearch = { approach: 150 };
        const cleanSearchUrl = hud.buildPathfindLink(cleanSearchSnap);
        const cleanSearchQuery = new URLSearchParams(
            cleanSearchUrl.slice(cleanSearchUrl.indexOf("?") + 1));
        check(results, "the clean plan link carries its approach radius",
            cleanSearchQuery.get("approach") === "150",
            "got " + cleanSearchUrl);
        check(results, "the clean plan link carries no bans",
            !cleanSearchQuery.has("avoid"), "got " + cleanSearchUrl);
        check(results, "the clean plan link arms the plan repro mode",
            cleanSearchQuery.get("fold") === "0",
            "got " + cleanSearchUrl);

        // A snapshot without a walk plan opens the viewer bare: the
        // route pair and the camera stay out, the defaults stay in.
        const bareUrl = hud.buildPathfindLink(snapshotWith(0));
        const bareQuery = new URLSearchParams(
            bareUrl.slice(bareUrl.indexOf("?") + 1));
        check(results, "the bare link carries no route pair",
            !bareQuery.has("from") && !bareQuery.has("to") &&
            !bareQuery.has("cam") && !bareQuery.has("tiles"),
            "got " + bareUrl);
        check(results, "the bare link keeps the viewer defaults",
            !bareQuery.has("filter") &&
            bareQuery.get("path") === "smooth", "got " + bareUrl);
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
