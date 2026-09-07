/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Shared state of the web UI.
const App = {
  activeBotId: null,
  snapshot: null,
  source: null,
  seenEvents: 0,
  bots: []
};

// Class and race names of the known C1 ids.
const CLASS_NAMES = {
  0: "Human Fighter", 1: "Warrior", 2: "Gladiator", 3: "Human Mystic",
  4: "Wizard", 10: "Human Knight", 18: "Elven Fighter",
  19: "Elven Knight", 22: "Elven Scout", 25: "Elven Mystic",
  31: "Elven Wizard", 38: "Dark Fighter", 44: "Dark Mystic",
  53: "Orc Fighter", 56: "Orc Mystic", 71: "Dwarf Fighter"
};

const RACE_NAMES = {
  0: "Human", 1: "Elf", 2: "Dark Elf", 3: "Orc", 4: "Dwarf"
};

// Cardinal direction of a heading value (0..65535, 0 = east, clockwise).
function headingDegrees(heading) {
  return Math.round((heading / 65536) * 360);
}

function headingCardinal(heading) {
  const deg = headingDegrees(heading);
  const names = ["E", "SE", "S", "SW", "W", "NW", "N", "NE"];
  const index = Math.round(deg / 45) % 8;

  return names[index];
}

function formatNumber(value) {
  if (value === null || value === undefined) { return "—"; }

  return Number(value).toLocaleString("en-US");
}

function formatDuration(fromISO, untilISO) {
  const from = new Date(fromISO).getTime();
  const until = untilISO ? new Date(untilISO).getTime() : Date.now();
  if (!from || Number.isNaN(from)) { return "—"; }
  let secs = Math.max(0, Math.floor((until - from) / 1000));
  const hours = Math.floor(secs / 3600);
  secs -= hours * 3600;
  const minutes = Math.floor(secs / 60);
  secs -= minutes * 60;
  const pad = (n) => String(n).padStart(2, "0");

  return pad(hours) + ":" + pad(minutes) + ":" + pad(secs);
}

// ---- theme ----

// applyTheme switches the color scheme and persists the choice. The light
// theme is the default.
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  window.localStorage.setItem("swarm.theme", theme);
  const icon = document.getElementById("theme-icon");
  if (icon) { icon.textContent = theme === "dark" ? "☾" : "☀"; }
  window.dispatchEvent(new Event("themechange"));
}

function initTheme() {
  const saved = window.localStorage.getItem("swarm.theme");
  applyTheme(saved === "dark" ? "dark" : "light");
  document.getElementById("theme-toggle").addEventListener("click", () => {
    const current = document.documentElement.dataset.theme === "dark"
      ? "dark" : "light";
    applyTheme(current === "dark" ? "light" : "dark");
  });
}

// Fetch the bot list and keep the sidebar in sync.
async function refreshBots() {
  try {
    const response = await fetch("/api/bots");
    App.bots = await response.json();
  } catch (err) {
    return;
  }
  renderBotList();
  if (!App.activeBotId && App.bots.length > 0) {
    const saved = window.localStorage.getItem("swarm.activeBot");
    const exists = App.bots.some((bot) => bot.id === saved);
    selectBot(exists ? saved : App.bots[0].id);
  }
}

function renderBotList() {
  const list = document.getElementById("bot-list");
  list.innerHTML = "";
  for (const bot of App.bots) {
    const item = document.createElement("li");
    item.className = "bot-item" + (bot.id === App.activeBotId ? " active" : "");
    item.dataset.id = bot.id;

    const row = document.createElement("div");
    row.className = "bot-row";
    const dot = document.createElement("span");
    dot.className = "dot " + bot.status;
    const name = document.createElement("span");
    name.className = "bot-name";
    name.textContent = bot.name || bot.id;
    row.append(dot, name);
    if (bot.inCombat) {
      row.append(makeChip("bot-chip chip-combat", "combat"));
    }
    if (bot.sitting) {
      row.append(makeChip("bot-chip chip-rest", "rest"));
    }
    const level = document.createElement("span");
    level.className = "bot-level";
    level.textContent = bot.level > 0 ? "lv " + bot.level : bot.status;
    row.append(level);
    item.append(row);

    // The mini HP/MP/XP bars share the HUD palette: one look at the
    // sidebar shows what every session is doing.
    if (bot.status === "online") {
      item.append(buildMiniBars(bot));
    }

    item.addEventListener("click", () => selectBot(bot.id));
    list.append(item);
  }
}

// makeChip renders one small status chip.
function makeChip(className, label) {
  const chip = document.createElement("span");
  chip.className = className;
  chip.textContent = label;

  return chip;
}

// buildMiniBars renders the compact HP/MP/XP bar trio of a bot row.
function buildMiniBars(bot) {
  const bars = document.createElement("div");
  bars.className = "bot-bars";
  const kinds = [
    ["hp", bot.curHp, bot.maxHp],
    ["mp", bot.curMp, bot.maxMp],
    ["xp", bot.expPercent, 100]
  ];
  for (const [kind, cur, max] of kinds) {
    const bar = document.createElement("div");
    bar.className = "mini-bar";
    bar.title = kind + " " + Math.round(cur || 0) + "/" + Math.round(max || 0);
    const fill = document.createElement("div");
    fill.className = "mini-fill " + kind;
    const percent = max > 0 ? Math.max(0, Math.min(100, (cur / max) * 100)) : 0;
    fill.style.width = percent.toFixed(1) + "%";
    bar.append(fill);
    bars.append(bar);
  }

  return bars;
}

// Switch the observed bot and reopen the event stream.
function selectBot(botId) {
  if (App.activeBotId === botId) { return; }
  App.activeBotId = botId;
  App.snapshot = null;
  App.seenEvents = 0;
  window.localStorage.setItem("swarm.activeBot", botId);
  if (App.source) {
    App.source.close();
    App.source = null;
  }
  openEventStream(botId);
  renderBotList();
  resetPanels();
}

function resetPanels() {
  document.getElementById("log-list").innerHTML = "";
  document.getElementById("log-count").textContent = "";
}

// Subscribe to the SSE stream of the active bot.
function openEventStream(botId) {
  const source = new EventSource("/api/bots/" + botId + "/events");
  App.source = source;
  source.addEventListener("snapshot", (event) => {
    App.snapshot = JSON.parse(event.data);
    setLive(true);
    renderSnapshot();
  });
  source.onerror = () => {
    setLive(false);
  };
}

function setLive(live) {
  const dot = document.getElementById("live-indicator");
  const label = document.getElementById("live-label");
  dot.className = "live-dot " + (live ? "online" : "offline");
  label.textContent = live ? "live" : "reconnecting";
}

// Render one snapshot into all panels.
function renderSnapshot() {
  const snap = App.snapshot;
  if (!snap) { return; }
  renderHUD(snap);
  renderTarget(snap);
  renderGear(snap);
  renderChat(snap);
  renderLog(snap);
  renderFooter(snap);
  MapView.update(snap);
}

// ---- equipment widget ----

// Paperdoll slot layout of the equipment widget: three columns of
// classic character window slots. mask is the body part mask of the
// inventory packets (see the C1 BodyPart enum), anyOf marks the
// either-or masks (earrings and rings carry the combined template
// mask) resolved into the first free slot of the group.
const PAPERDOLL_SLOTS = [
  { key: "hair", label: "hair", mask: 0x10000 },
  { key: "r_ear", label: "r.ear", mask: 0x2, group: "ear" },
  { key: "l_ear", label: "l.ear", mask: 0x4, group: "ear" },
  { key: "neck", label: "neck", mask: 0x8 },
  { key: "r_finger", label: "r.ring", mask: 0x10, group: "finger" },
  { key: "l_finger", label: "l.ring", mask: 0x20, group: "finger" },
  { key: "head", label: "head", mask: 0x40 },
  { key: "chest", label: "chest", mask: 0x400 },
  { key: "legs", label: "legs", mask: 0x800 },
  { key: "rhand", label: "weapon", mask: 0x80 },
  { key: "lhand", label: "shield", mask: 0x100 },
  { key: "back", label: "cloak", mask: 0x2000 },
  { key: "gloves", label: "gloves", mask: 0x200 },
  { key: "feet", label: "boots", mask: 0x1000 },
  { key: "under", label: "shirt", mask: 0x1 }
];

// Masks that map onto another slot: two handed weapons and full armor
// occupy the weapon/chest slot, alldress also lands on the chest. The
// either-or masks (earrings, rings) carry the combined template mask
// and resolve into the first free slot of their group.
const SLOT_MASK_ALIASES = { 0x4000: "rhand", 0x8000: "chest", 0x20000: "chest" };
const EITHER_OR_MASKS = { 0x6: "ear", 0x30: "finger" };

// Fallback glyph per type2 family when an item has no icon.
const TYPE2_GLYPH = { 0: "W", 1: "A", 2: "J", 3: "Q", 4: "$", 5: "•" };

// assignPaperdoll places every equipped item on a paperdoll slot.
// Returns the slot key -> item map.
function assignPaperdoll(items) {
  const placed = {};
  const free = { ear: ["r_ear", "l_ear"], finger: ["r_finger", "l_finger"] };
  for (const item of items) {
    if (!item.equipped) { continue; }
    let key = null;
    const either = EITHER_OR_MASKS[item.bodyPart];
    if (either) {
      key = free[either].shift() || null;
    } else if (SLOT_MASK_ALIASES[item.bodyPart]) {
      key = SLOT_MASK_ALIASES[item.bodyPart];
    } else {
      const slot = PAPERDOLL_SLOTS.find((s) => s.mask === item.bodyPart);
      key = slot ? slot.key : null;
    }
    if (!key || placed[key]) { continue; }
    placed[key] = item;
    for (const group of Object.values(free)) {
      const idx = group.indexOf(key);
      if (idx >= 0) { group.splice(idx, 1); }
    }
  }

  return placed;
}

// makeIconCell builds one icon cell: the pack image when the item
// resolves to an icon, the type glyph otherwise (also the image error
// fallback - a 404 icon must not leave an empty box).
function makeIconCell(item, className) {
  const cell = document.createElement("div");
  cell.className = className;
  if (!item) { return cell; }

  const glyph = document.createElement("span");
  glyph.className = "icon-glyph glyph-t" + (item.type2 || 0);
  glyph.textContent = TYPE2_GLYPH[item.type2] || "•";
  cell.append(glyph);

  if (item.icon) {
    const img = document.createElement("img");
    img.alt = "";
    img.loading = "lazy";
    img.src = "/icons/" + item.icon + ".png";
    img.addEventListener("error", () => img.remove());
    cell.append(img);
  }

  if (item.enchant > 0) {
    const en = document.createElement("span");
    en.className = "icon-badge badge-enchant";
    en.textContent = "+" + item.enchant;
    cell.append(en);
  }
  if (item.count > 1) {
    const count = document.createElement("span");
    count.className = "icon-badge badge-count";
    count.textContent = item.count >= 10000
      ? Math.round(item.count / 1000) + "k" : item.count;
    cell.append(count);
  }
  cell.title = itemTooltip(item);

  return cell;
}

// itemTooltip composes the hover title of an item cell.
function itemTooltip(item) {
  let tip = item.name || ("item #" + item.itemId);
  if (item.enchant > 0) { tip = "+" + item.enchant + " " + tip; }
  if (item.count > 1) { tip += " x" + item.count; }
  if (item.equipped) { tip += " (equipped)"; }

  return tip;
}

// renderGear draws the paperdoll and the inventory grid of the right
// side equipment widget.
function renderGear(snap) {
  const paperdoll = document.getElementById("paperdoll");
  const invGrid = document.getElementById("inv-grid");
  const invCount = document.getElementById("inv-count");
  if (!paperdoll || !invGrid) { return; }

  const items = snap.inventory || [];
  const placed = assignPaperdoll(items);

  paperdoll.innerHTML = "";
  for (const slot of PAPERDOLL_SLOTS) {
    const item = placed[slot.key] || null;
    const cell = makeIconCell(item, "pd-cell" + (item ? "" : " pd-empty"));
    if (!item) {
      cell.textContent = slot.label;
    }
    paperdoll.append(cell);
  }

  invGrid.innerHTML = "";
  let invItems = 0;
  for (const item of items) {
    if (item.equipped) { continue; }
    invItems++;
    invGrid.append(makeIconCell(item, "inv-cell"));
  }
  invCount.textContent = (snap.character.inventorySlots || 0) + "/" +
    (snap.character.inventoryMax || 80);
}

// Chat window state: auto scroll follows the newest line while the
// user stays at the bottom; scrolling up reads the history, scrolling
// back to the bottom resumes the follow.
const ChatWindow = { stick: true };

// chatAtBottom reports whether the scroll position of the chat list is
// within a few pixels of the newest line.
function chatAtBottom(list) {
  return list.scrollTop + list.clientHeight >= list.scrollHeight - 4;
}

// initChat attaches the scroll tracking of the chat window.
function initChat() {
  const list = document.getElementById("chat-list");
  list.addEventListener("scroll", () => {
    ChatWindow.stick = chatAtBottom(list);
  });
}

// Chat window rendering: the parsed system messages and the social
// animations of the creatures around the bot, oldest at the top. The
// view scrolls to the newest line only while the follow is stuck.
function renderChat(snap) {
  const list = document.getElementById("chat-list");
  const lines = snap.chat || [];
  list.innerHTML = "";
  for (const line of lines) {
    const row = document.createElement("div");
    row.className = "chat-line chat-" + line.kind;
    const time = document.createElement("span");
    time.className = "chat-time";
    time.textContent = new Date(line.time).toTimeString().slice(0, 8);
    const msg = document.createElement("span");
    msg.className = "chat-msg";
    msg.textContent = line.text;
    row.append(time, msg);
    list.append(row);
  }
  if (ChatWindow.stick) {
    list.scrollTop = list.scrollHeight;
  }
}

// Character status rendering on the map HUD: the map is the single source
// of truth for the bot state. The target lives in its own panel next to
// the character one, so long target names never break the layout.
function renderHUD(snap) {
  const c = snap.character;
  document.getElementById("hud-name").textContent = c.name || snap.id;
  document.getElementById("hud-class").textContent =
    CLASS_NAMES[c.classId] || ("class " + c.classId);
  document.getElementById("hud-race").textContent =
    RACE_NAMES[c.race] || ("race " + c.race);
  document.getElementById("hud-level").textContent = c.level;
  document.getElementById("hud-exp").textContent = formatNumber(c.exp);
  document.getElementById("hud-sp").textContent = formatNumber(c.sp);
  document.getElementById("hud-combat").classList
    .toggle("hidden", !c.inCombat);
  document.getElementById("hud-rest").classList
    .toggle("hidden", !c.sitting);

  setVital("hp", c.curHp, c.maxHp, c.maxHp > 0);
  setVital("mp", c.curMp, c.maxMp, c.maxMp > 0);
  setExpVital(c.expPercent);

  document.getElementById("pos-x").textContent = c.x;
  document.getElementById("pos-y").textContent = c.y;
  document.getElementById("pos-z").textContent = c.z;
  document.getElementById("hud-slots").textContent =
    (c.inventorySlots || 0) + "/" + (c.inventoryMax || 80);
  document.getElementById("hud-weight").textContent =
    c.maxLoad > 0 ? Math.round((c.load / c.maxLoad) * 100) + "%" : "—";
  document.getElementById("hud-adena").textContent = formatNumber(c.adena);
}

// Target panel rendering: resolves the current target object and shows
// its name, level and vitals. The panel exists only while there is a
// target: a killed, removed or missing target (the tracker clears them
// because the server never does) hides the whole panel.
function renderTarget(snap) {
  const panel = document.getElementById("hud-target");
  const c = snap.character;
  const targetId = c ? c.targetId : 0;
  const target = targetId
    ? (snap.objects || []).find(
      (obj) => obj.objectId === targetId && !obj.dead)
    : null;
  panel.classList.toggle("hidden", !target);
  if (!target) {
    return;
  }
  document.getElementById("target-name").textContent =
    target.name || ("object " + target.objectId);
  const level = document.getElementById("target-level");
  const hasLevel = target.kind === "npc" && target.level > 0;
  level.textContent = hasLevel ? "lv " + target.level : "";
  level.classList.toggle("hidden", !hasLevel);
  setVital("target-hp", target.curHp, target.maxHp, target.maxHp > 0);
  setVital("target-mp", target.curMp, target.maxMp, target.maxMp > 0);
}

function setVital(kind, cur, max, known) {
  const fill = document.getElementById(kind + "-fill");
  const text = document.getElementById(kind + "-text");
  if (!known) {
    fill.style.width = "0%";
    text.textContent = "—";

    return;
  }
  const maxN = max > 0 ? max : 1;
  const percent = Math.max(0, Math.min(100, (cur / maxN) * 100));
  fill.style.width = percent.toFixed(1) + "%";
  text.textContent = Math.round(cur) + "/" + Math.round(max);
}

// setExpVital renders the experience bar: the fill is the percentage of
// the experience gathered toward the next level (computed by the bot
// from the C1 experience table).
function setExpVital(expPercent) {
  const percent = Math.max(0, Math.min(100, expPercent || 0));
  document.getElementById("xp-fill").style.width = percent.toFixed(1) + "%";
  document.getElementById("xp-text").textContent =
    percent >= 99.95 ? "100%" : percent.toFixed(1) + "%";
}

// Log panel rendering with incremental append.
function renderLog(snap) {
  const list = document.getElementById("log-list");
  const events = snap.events || [];
  if (events.length < App.seenEvents) {
    list.innerHTML = "";
    App.seenEvents = 0;
  }
  const filter = document.getElementById("log-filter").value.toLowerCase();
  const autoScroll = document.getElementById("log-scroll").checked;
  for (let i = App.seenEvents; i < events.length; i++) {
    const line = buildLogLine(events[i], filter);
    if (line) { list.append(line); }
  }
  App.seenEvents = events.length;
  while (list.children.length > 400) {
    list.removeChild(list.firstChild);
  }
  document.getElementById("log-count").textContent =
    events.length + " events";
  if (autoScroll) {
    list.scrollTop = list.scrollHeight;
  }
}

function buildLogLine(event, filter) {
  if (filter && !event.message.toLowerCase().includes(filter)) {
    return null;
  }
  const line = document.createElement("div");
  line.className = "log-line";
  const time = document.createElement("span");
  time.className = "log-time";
  const stamp = new Date(event.time);
  time.textContent = stamp.toTimeString().slice(0, 8);
  const msg = document.createElement("span");
  msg.className = "log-msg " + logLineClass(event.message);
  msg.textContent = event.message;
  line.append(time, msg);

  return line;
}

function logLineClass(message) {
  if (message.startsWith("npc spawned") || message.startsWith("player appeared")
    || message.startsWith("item dropped") || message.startsWith("item appeared")
    || message.startsWith("entered")) {
    return "spawn";
  }
  if (message.includes("combat") || message.startsWith("target selected")) {
    return "combat";
  }
  if (message.startsWith("picked up") || message.startsWith("received ")
    || message.startsWith("lost ")) {
    return "spawn";
  }
  if (message.startsWith("object removed") || message.startsWith("left")) {
    return "remove";
  }
  if (message.startsWith("packet ") || message.startsWith("inventory ")) {
    return "packet";
  }

  return "";
}

// Footer rendering.
function renderFooter(snap) {
  document.getElementById("foot-status").textContent = "status: " + snap.status;
  document.getElementById("foot-packets").textContent =
    "packets: " + formatNumber(snap.packets);
  document.getElementById("foot-objects").textContent =
    "objects: " + (snap.objects ? snap.objects.length : 0);
  document.getElementById("foot-uptime").textContent =
    "uptime: " + formatDuration(snap.startedAt, null);
  const stamp = new Date(snap.updatedAt);
  document.getElementById("foot-updated").textContent =
    "updated: " + (isNaN(stamp.getTime()) ? "—" : stamp.toTimeString().slice(0, 8));
}
