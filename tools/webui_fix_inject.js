// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Injects a synthetic snapshot exercising the three fixed areas: a
// fighting bot with a named mob (the banner detail), quest items in
// the inventory (the sub tab) and a mixed chat backlog (the line
// rhythm). An IIFE so the payload stays a single eval expression.
(() => {
  window.__snap = {
    id: "bot1",
    status: "online",
    phase: "engage",
    serverTimeMs: Date.now(),
    character: {
      objectId: 1001, name: "TestHunter", classId: 0, race: 0,
      level: 12, exp: 123456, sp: 9876, expPercent: 45.5,
      curHp: 180, maxHp: 220, curMp: 70, maxMp: 100,
      x: 45000, y: 45000, z: -3500, heading: 0,
      inventorySlots: 24, inventoryMax: 80,
      load: 1200, maxLoad: 5000, adena: 42000,
      inCombat: true, sitting: false, moving: false,
      targetId: 2002
    },
    objects: [{
      objectId: 2002, kind: "npc", name: "Keltir",
      x: 45200, y: 44850, z: -3500, heading: 0, moving: false,
      speed: 0, targetId: 1001, dead: false, attackable: true,
      aggressive: false, inCombat: true, level: 4, clanMask: "0",
      curHp: 55, maxHp: 90, curMp: 0, maxMp: 0
    }],
    huntingZones: [],
    walkPath: [],
    inventory: [
      { objectId: 1, itemId: 57, type2: 0, count: 42000,
        equipped: false, name: "Adena" },
      { objectId: 2, itemId: 1, type2: 0, count: 1,
        equipped: true, name: "Rusty Sword" },
      { objectId: 3, itemId: 1000, type2: 3, count: 3,
        equipped: false, name: "Bone Fragment" },
      { objectId: 4, itemId: 1001, type2: 3, count: 2,
        equipped: false, name: "Plague Dust" }
    ],
    skills: [],
    buffs: [],
    shopping: [],
    events: [],
    chat: [
      { time: Date.now() - 8000, kind: "system",
        text: "You picked up 25 adena." },
      { time: Date.now() - 7000, kind: "say", from: "Trader",
        text: "WTS wooden arrows, cheap" },
      { time: Date.now() - 6000, kind: "system",
        text: "You hit Keltir for 23 damage." },
      { time: Date.now() - 5000, kind: "say", from: "LongSenderName",
        text: "this is a deliberately long message that must wrap across two lines of the chat window to check that the vertical rhythm of the wrapped rows stays uniform" },
      { time: Date.now() - 4000, kind: "system",
        text: "Cannot see target." },
      { time: Date.now() - 3000, kind: "social",
        text: "Keltir reached a new level" },
      { time: Date.now() - 2000, kind: "shout", from: "Guard",
        text: "SELLING arrows and shields, best offer wins today" }
    ],
    diagnostics: {
      hunt: {
        targetId: 2002, targetForMs: 34000, killEtaMs: 12000,
        noTargetForMs: 0, skippedTargets: 0,
        waypointsLeft: 0, tripForMs: 0, walkEtaMs: 0
      }
    }
  };
  try {
    App.snapshot = window.__snap;
    renderSnapshot();
    return "injected: chat rows "
      + document.querySelectorAll(".chat-line").length;
  } catch (err) {
    return "ERR: " + (err && err.message ? err.message : String(err));
  }
})()
