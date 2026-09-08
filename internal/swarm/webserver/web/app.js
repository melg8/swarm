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
  bots: [],
  // The client proxy state (null when the process runs without -proxy):
  // the bot a connecting C1 game client attaches to is the selected
  // bot, and clicking a bot row in the sidebar switches it.
  proxy: null
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
  await refreshProxy();
  renderBotList();
  if (!App.activeBotId && App.bots.length > 0) {
    const saved = window.localStorage.getItem("swarm.activeBot");
    const exists = App.bots.some((bot) => bot.id === saved);
    selectBot(exists ? saved : App.bots[0].id);
  }
}

// Fetch the proxy state (the endpoints stay absent without -proxy and
// the UI then hides the selection entirely).
async function refreshProxy() {
  try {
    const response = await fetch("/api/proxy");
    if (!response.ok) {
      App.proxy = null;
      return;
    }
    App.proxy = await response.json();
  } catch (err) {
    App.proxy = null;
  }
}

// selectProxyBot marks the bot a connecting game client attaches to.
async function selectProxyBot(botId) {
  if (!App.proxy || App.proxy.selectedBot === botId) { return; }
  try {
    const response = await fetch("/api/proxy/select", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ botId })
    });
    if (response.ok) {
      App.proxy = await response.json();
      renderBotList();
    }
  } catch (err) {
    // The selection is best effort: the proxy keeps its previous
    // target when the request fails.
  }
}

// proxyTargetId returns the bot a connecting game client attaches to:
// the proxy selection when set, the first bot otherwise (mirrors the
// selection fallback of the proxy server).
function proxyTargetId() {
  if (!App.proxy) { return null; }
  if (App.proxy.selectedBot) { return App.proxy.selectedBot; }
  const first = App.proxy.sessions && App.proxy.sessions[0];
  return first || (App.bots.length > 0 ? App.bots[0].id : null);
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
    // The proxy chip marks the bot a connecting C1 game client attaches
    // to (the proxy falls back to the first bot when nothing is
    // selected - the first row carries the chip implicitly then).
    if (App.proxy && App.proxy.enabled && bot.id === proxyTargetId()) {
      row.append(makeChip("bot-chip chip-proxy", "proxy"));
    }
    const level = document.createElement("span");
    level.className = "bot-level";
    level.textContent = bot.level > 0 ? "lv " + bot.level : bot.status;
    row.append(level);
    item.append(row);

    // The activity banner of the sidebar row: a compact one line
    // summary of the bot phase so the overview shows at a glance
    // what every session is doing (hunting, walking to town,
    // selling, deleveling). Hidden when no phase is published
    // (the manual only sessions and the pre-world sessions).
    const activity = botActivityLabel(bot);
    if (activity) {
      const act = document.createElement("div");
      act.className = "bot-activity kind-" + activity.kind;
      act.textContent = activity.text;
      item.append(act);
    }

    // The mini HP/MP/XP bars share the HUD palette: one look at the
    // sidebar shows what every session is doing.
    if (bot.status === "online") {
      item.append(buildMiniBars(bot));
    }

    item.addEventListener("click", () => selectBot(bot.id));
    list.append(item);
  }
}

// botActivityLabel mirrors phaseLabel for the compact BotInfo payload
// of the sidebar list: it maps the bot phase to a short text the
// overview row shows under the name. Returns null when no phase is
// published (the manual only sessions never set it) so the row
// stays compact.
function botActivityLabel(bot) {
  const status = bot.status;
  if (status === "offline") {
    return { kind: "offline", text: "offline" };
  }
  if (status === "connecting") {
    return { kind: "connecting", text: "connecting" };
  }
  const phase = bot.phase || "";
  switch (phase) {
  case "engage":
    return { kind: "hunt",
      text: bot.inCombat ? "hunting · combat" : "hunting" };
  case "loot":
    return { kind: "loot", text: "looting" };
  case "townWalk":
    return { kind: "town", text: "walking to town" };
  case "townSell":
    return { kind: "town", text: "selling" };
  case "townReturn":
    return { kind: "return", text: "walking to farm spot" };
  case "delevel":
    return { kind: "delevel", text: "deleveling" };
  case "user":
    if (bot.inCombat) {
      return { kind: "combat", text: "manual · attacking" };
    }

    return { kind: "user", text: "manual" };
  case "idle":
    return { kind: "idle", text: "idle" };
  default:
    return null;
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
  // The observed bot is also the bot a connecting C1 client attaches
  // to: one click in the sidebar switches both the map view and the
  // proxy target.
  selectProxyBot(botId);
  if (App.source) {
    App.source.close();
    App.source = null;
  }
  openEventStream(botId);
  renderBotList();
  resetPanels();
  resetGear();
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
  renderZones(snap);
  renderChat(snap);
  renderLog(snap);
  renderFooter(snap);
  renderBotStatus(snap);
  MapView.update(snap);
}

// phaseLabel maps the hunt loop phase (snap.phase, the same string the
// hunt package uses internally: engage, loot, townWalk, townSell,
// townReturn, delevel, user, idle) to a short human readable activity
// text. The session status (snap.status: connecting, online, offline)
// takes precedence when the loop has not published a phase yet - a
// manual only session never sets the phase, so its banner stays on
// the status text. The label is the headline of the bot status
// banner; the detail adds the next step (e.g. "selling junk at the
// trader", "walking back to the farm spot") so the user sees at a
// glance what every bot is doing right now.
function phaseLabel(snap) {
  const status = snap.status;
  if (status === "offline") {
    return { kind: "offline", text: "offline", detail: "session ended" };
  }
  if (status === "connecting") {
    return { kind: "connecting", text: "connecting",
      detail: "logging in to the world" };
  }
  const c = snap.character || {};
  const phase = snap.phase || "";
  switch (phase) {
  case "engage":
    if (c.inCombat) {
      return { kind: "combat", text: "hunting",
        detail: "fighting a target in the zone" };
    }
    return { kind: "hunt", text: "hunting",
      detail: "looking for the next target" };
  case "loot":
    return { kind: "loot", text: "looting",
      detail: "picking up the drops of the last kill" };
  case "townWalk":
    return { kind: "town", text: "walking to town",
      detail: "heading to the trader to sell junk" };
  case "townSell":
    return { kind: "town", text: "selling",
      detail: "selling the inventory at the trader" };
  case "townReturn":
    return { kind: "return", text: "walking to farm spot",
      detail: "heading back to the hunting zone" };
  case "delevel":
    return { kind: "delevel", text: "deleveling",
      detail: "dying at the town guards to drop levels" };
  case "user":
    return userPhaseLabel(snap);
  case "idle":
    return { kind: "idle", text: "idle",
      detail: "waiting for a manual command" };
  default:
    return { kind: "online", text: status || "online", detail: "" };
  }
}

// userPhaseLabel describes the manual command the loop is executing:
// a move, an attack or a pickup. The bot widget banner shows the
// activity so the user sees their click took effect.
function userPhaseLabel(snap) {
  // The snapshot does not carry the manual command kind directly,
  // so the label stays on the generic "manual" text. The combat and
  // sitting chips of the HUD already cover the in-fight and rest
  // states, the banner adds the manual mode context.
  const c = snap.character || {};
  if (c.inCombat) {
    return { kind: "combat", text: "manual · attacking",
      detail: "fighting the manually selected target" };
  }
  if (c.moving) {
    return { kind: "user", text: "manual · moving",
      detail: "walking to the clicked destination" };
  }
  return { kind: "user", text: "manual",
    detail: "executing a manual command" };
}

// renderBotStatus updates the floating activity banner of the bot
// widget: a compact chip pinned to the top of the map (the HUD
// stack area) that shows the current activity (hunting, walking to
// town, selling, deleveling, manual move). The banner uses the
// phase the hunt loop publishes through snap.phase; a manual only
// session falls back to the session status.
function renderBotStatus(snap) {
  const banner = document.getElementById("bot-status");
  if (!banner) { return; }
  const label = phaseLabel(snap);
  banner.dataset.kind = label.kind;
  banner.classList.toggle("hidden", false);
  const text = document.getElementById("bot-status-text");
  if (text) { text.textContent = label.text; }
  const detail = document.getElementById("bot-status-detail");
  if (detail) { detail.textContent = label.detail || ""; }
}

// ---- hunting zones panel ----

// The zone panel starts collapsed: the map corner chip carries the
// count, the click on the head expands the scrollable list. The
// sidebar stays the bots-only overview.
const zonePanelCollapsed = { value: true };

// initZonePanel wires the collapse toggle of the zone panel head.
function initZonePanel() {
  const head = document.getElementById("zone-panel-head");
  const panel = document.getElementById("zone-panel");
  if (!head || !panel) { return; }
  head.addEventListener("click", () => {
    zonePanelCollapsed.value = !zonePanelCollapsed.value;
    applyZonePanelState();
  });
}

// applyZonePanelState syncs the panel DOM with the collapse flag.
function applyZonePanelState() {
  const panel = document.getElementById("zone-panel");
  const chev = document.getElementById("zone-panel-chev");
  if (!panel) { return; }
  panel.classList.toggle("collapsed", zonePanelCollapsed.value);
  if (chev) {
    chev.textContent = zonePanelCollapsed.value ? "\u25B8" : "\u25BE";
  }
}

// renderZones refreshes the hunting zone list of the floating map
// panel: every zone of the registry with its level band and gear
// gate, the active one highlighted (the ground the bot hunts in or
// walks to), the demoted bands of the death regression marked, a
// hunt button switching the zone of the bot (the manual override of
// the automatic picker).
function renderZones(snap) {
  const section = document.getElementById("zone-panel");
  const list = document.getElementById("zone-list");
  const count = document.getElementById("zone-panel-count");
  if (!section || !list) { return; }
  const zones = Array.isArray(snap.huntingZones) ? snap.huntingZones : [];
  if (zones.length === 0) {
    section.classList.add("hidden");
    return;
  }
  section.classList.remove("hidden");
  if (count) { count.textContent = String(zones.length); }
  applyZonePanelState();
  if (renderZones.lastKey === JSON.stringify(zones)) { return; }
  renderZones.lastKey = JSON.stringify(zones);
  list.textContent = "";
  zones.forEach((zone, index) => {
    const item = document.createElement("li");
    item.className = "zone-item" + (zone.active ? " active" : "") +
      (zone.demoted ? " lost" : "");
    const row = document.createElement("div");
    row.className = "zone-row";
    const info = document.createElement("div");
    const name = document.createElement("div");
    name.className = "zone-name";
    name.textContent = zone.name;
    name.title = zone.id + " · " + zone.region;
    const meta = document.createElement("div");
    meta.className = "zone-meta";
    meta.textContent = "L" + zone.minLevel + "-" + zone.maxLevel +
      (zone.minGear > 0 ? " · gear " + zone.minGear + "+" : "") +
      (zone.active ? " · hunting" : "") +
      (zone.demoted ? " · too hard" : "") +
      (zone.deaths > 0
        ? " · " + zone.deaths + (zone.deaths === 1 ? " death" : " deaths")
        : "");
    info.appendChild(name);
    info.appendChild(meta);
    const button = document.createElement("button");
    button.className = "zone-hunt-btn";
    button.textContent = zone.active ? "here" : "hunt";
    button.title = "switch the hunting zone of the bot to " + zone.name;
    button.addEventListener("click", () => {
      postCommand({ kind: "zone", count: index });
    });
    row.appendChild(info);
    row.appendChild(button);
    item.appendChild(row);
    list.appendChild(item);
  });
}

// ---- equipment widget ----

// Wearable slot layout of the equipment widget: the left 3x3 block of
// the classic armor and weapon paperdoll, ordered like the C1 client
// doll - shirt over the head row left, cloak right, gloves bottom left,
// boots bottom right. mask is the body part mask of the inventory
// packets (see the C1 BodyPart enum).
const WEAR_SLOTS = [
  { key: "under", label: "shirt", mask: 0x1 },
  { key: "head", label: "head", mask: 0x40 },
  { key: "back", label: "cloak", mask: 0x2000 },
  { key: "rhand", label: "weapon", mask: 0x80 },
  { key: "chest", label: "chest", mask: 0x400 },
  { key: "lhand", label: "shield", mask: 0x100 },
  { key: "gloves", label: "gloves", mask: 0x200 },
  { key: "legs", label: "legs", mask: 0x800 },
  { key: "feet", label: "boots", mask: 0x1000 }
];

// Jewelry layout: a 2x3 block with only five real slots (two earrings,
// a necklace, two rings) - the necklace sits on the right of the middle
// row and the sixth position, the middle left cell, stays empty because
// the classic character has no sixth jewelry slot. The null entry
// renders the placeholder hole of the grid.
const JEWEL_SLOTS = [
  { key: "r_ear", label: "r.ear", mask: 0x2, group: "ear" },
  { key: "l_ear", label: "l.ear", mask: 0x4, group: "ear" },
  null,
  { key: "neck", label: "neck", mask: 0x8 },
  { key: "r_finger", label: "r.ring", mask: 0x10, group: "finger" },
  { key: "l_finger", label: "l.ring", mask: 0x20, group: "finger" }
];

// Masks that map onto another slot: two handed weapons and full armor
// occupy the weapon/chest slot, alldress also lands on the chest. The
// either-or masks (earrings, rings) carry the combined template mask
// and resolve into the first free slot of their group.
const SLOT_MASK_ALIASES = { 0x4000: "rhand", 0x8000: "chest", 0x20000: "chest" };
const EITHER_OR_MASKS = { 0x6: "ear", 0x30: "finger" };

// Fallback glyph per type2 family when an item has no icon.
const TYPE2_GLYPH = { 0: "W", 1: "A", 2: "J", 3: "Q", 4: "$", 5: "•" };

// Cell registry of the equipment widget: one record per rendered cell
// so a snapshot only touches the cells whose item actually changed.
// Rebuilding the whole grid on every snapshot recreated the <img>
// elements and made all icons flash for a moment - the images are
// cached by the browser, but a fresh element still decodes and paints
// asynchronously. Keyed records keep the elements alive instead.
const GearCells = {
  slots: new Map(), // slot key -> cell record
  inv: new Map(),   // item objectId -> cell record
  order: ""         // last inventory order signature
};

// itemSignature is the change signature of one cell content: two
// snapshots with equal signatures leave the DOM untouched.
function itemSignature(item) {
  if (!item) { return ""; }

  return [item.itemId, item.icon, item.count, item.enchant,
    item.type2, item.name, item.equipped].join("|");
}

// makeCellRecord creates a cell record with its persistent DOM cell.
// The slot variant carries a label span for the empty state. Every
// cell answers a double click (use the item: equip or unequip) and
// starts a drag (move the item to the paperdoll, the bag or the map).
function makeCellRecord(className, label) {
  const cell = document.createElement("div");
  cell.className = className;
  const record = {
    cell,
    label: null,
    img: null,
    glyph: null,
    badgeEn: null,
    badgeCount: null,
    sig: null,
    item: null
  };
  if (label) {
    const span = document.createElement("span");
    span.className = "slot-label";
    span.textContent = label;
    cell.append(span);
    record.label = span;
  }
  cell.draggable = true;
  cell.addEventListener("dblclick", () => activateGearCell(record));
  cell.addEventListener("dragstart", (event) => startGearDrag(record, event));

  return record;
}

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
      const wear = WEAR_SLOTS.concat(JEWEL_SLOTS).find(
        (s) => s && s.mask === item.bodyPart);
      key = wear ? wear.key : null;
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

// applyItemCell refreshes one cell record to show the item (or the
// empty label). Text badges may be recreated freely, but the icon
// image element survives every change that does not alter the icon
// itself - stack counts and enchant updates must not blink the icon.
function applyItemCell(record, item) {
  const sig = itemSignature(item);
  record.item = item || null;
  if (record.sig === sig) { return; }
  record.sig = sig;
  const cell = record.cell;

  if (record.glyph) { record.glyph.remove(); record.glyph = null; }
  if (record.badgeEn) { record.badgeEn.remove(); record.badgeEn = null; }
  if (record.badgeCount) { record.badgeCount.remove(); record.badgeCount = null; }

  if (!item) {
    if (record.img) { record.img.remove(); record.img = null; }
    cell.title = "";
    if (record.label) { record.label.style.display = ""; }

    return;
  }

  if (record.label) { record.label.style.display = "none"; }

  // The glyph layer sits under the image: it shows while the icon
  // loads, stays as the fallback when it fails (the error handler
  // removes the img) and tints by the item family.
  const glyph = document.createElement("span");
  glyph.className = "icon-glyph glyph-t" + (item.type2 || 0);
  glyph.textContent = TYPE2_GLYPH[item.type2] || "•";
  cell.append(glyph);
  record.glyph = glyph;

  if (item.icon) {
    const src = "/icons/" + item.icon + ".png";
    if (!record.img) {
      const img = document.createElement("img");
      img.alt = "";
      img.src = src;
      img.addEventListener("error", () => img.remove());
      cell.append(img);
      record.img = img;
    } else if (record.img.src !== src) {
      record.img.src = src;
    }
  } else if (record.img) {
    record.img.remove();
    record.img = null;
  }

  if (item.enchant > 0) {
    const badge = document.createElement("span");
    badge.className = "icon-badge badge-enchant";
    badge.textContent = "+" + item.enchant;
    cell.append(badge);
    record.badgeEn = badge;
  }
  if (item.count > 1) {
    const badge = document.createElement("span");
    badge.className = "icon-badge badge-count";
    badge.textContent = item.count >= 10000
      ? Math.round(item.count / 1000) + "k" : item.count;
    cell.append(badge);
    record.badgeCount = badge;
  }
  cell.title = itemTooltip(item);
}

// itemTooltip composes the hover title of an item cell.
function itemTooltip(item) {
  let tip = item.name || ("item #" + item.itemId);
  if (item.enchant > 0) { tip = "+" + item.enchant + " " + tip; }
  if (item.count > 1) { tip += " x" + item.count; }
  if (item.equipped) { tip += " (equipped)"; }

  return tip;
}

// ensureSlotCells creates the slot cells (and the jewelry hole) of one
// slot block once; later snapshots only refresh their contents.
function ensureSlotCells(box, slots) {
  if (box.children.length > 0) { return; }
  for (const slot of slots) {
    if (!slot) {
      const hole = document.createElement("div");
      hole.className = "jewel-hole";
      box.append(hole);
      continue;
    }
    const record = makeCellRecord("pd-cell", slot.label);
    GearCells.slots.set(slot.key, record);
    box.append(record.cell);
  }
}

// resetGear drops every cell record: switching the observed bot starts
// the widget from scratch instead of mixing two inventories. The
// pinned footer resets too, so a stale adena or load never survives
// into the next bot.
function resetGear() {
  GearCells.slots.clear();
  GearCells.inv.clear();
  GearCells.order = "";
  for (const id of ["gear-wear", "gear-jewel", "inv-grid"]) {
    const box = document.getElementById(id);
    if (box) { box.innerHTML = ""; }
  }
  GearDrag.item = null;
  closeDropDialog();
  const adena = document.getElementById("gear-adena");
  if (adena) { adena.textContent = "—"; adena.title = ""; }
  const fill = document.getElementById("gear-load-fill");
  if (fill) { fill.style.width = "0%"; fill.className = "load-fill"; fill.style.background = ""; }
  const loadText = document.getElementById("gear-load-text");
  if (loadText) { loadText.textContent = "—"; }
  const row = document.getElementById("gear-weight-row");
  if (row) { row.title = ""; }
}

// renderGear refreshes the paperdoll blocks and the inventory grid of
// the floating equipment widget with keyed cells: unchanged items
// leave their DOM untouched, so their icons never blink.
function renderGear(snap) {
  const wearBox = document.getElementById("gear-wear");
  const jewelBox = document.getElementById("gear-jewel");
  const invGrid = document.getElementById("inv-grid");
  const invCount = document.getElementById("inv-count");
  if (!wearBox || !jewelBox || !invGrid) { return; }

  const items = snap.inventory || [];
  const placed = assignPaperdoll(items);

  ensureSlotCells(wearBox, WEAR_SLOTS);
  ensureSlotCells(jewelBox, JEWEL_SLOTS);
  for (const [key, record] of GearCells.slots) {
    applyItemCell(record, placed[key] || null);
  }

  const order = [];
  const seen = new Set();
  for (const item of items) {
    if (item.equipped) { continue; }
    seen.add(item.objectId);
    order.push(item.objectId);
    let record = GearCells.inv.get(item.objectId);
    if (!record) {
      record = makeCellRecord("inv-cell", null);
      GearCells.inv.set(item.objectId, record);
      invGrid.append(record.cell);
    }
    applyItemCell(record, item);
  }
  for (const [id, record] of Array.from(GearCells.inv)) {
    if (!seen.has(id)) {
      record.cell.remove();
      GearCells.inv.delete(id);
    }
  }
  // Reordering moves the persistent cells (appendChild never reloads
  // an image) and only when the order actually changed.
  const orderSig = order.join(",");
  if (GearCells.order !== orderSig) {
    GearCells.order = orderSig;
    for (const id of order) {
      const record = GearCells.inv.get(id);
      if (record) { invGrid.append(record.cell); }
    }
  }

  invCount.textContent = (snap.character.inventorySlots || 0) + "/" +
    (snap.character.inventoryMax || 80);
  renderGearFoot(snap);
}

// ---- weight gauge coloring ----

// The weight penalty thresholds of the Mobius C1 server (Player
// refreshOverloaded): the load is measured per mille and the debuff
// levels switch at 500/666/800/1000 - 50%, 66.6%, 80% and 100% of the
// maximum load. Each level slows the character (speed x0.90 / 0.87 /
// 0.84 / 0.81) and the last one marks the character overloaded.
const WEIGHT_PENALTIES = [
  { percent: 100.0, level: 4, speed: "x0.81" },
  { percent: 80.0, level: 3, speed: "x0.84" },
  { percent: 66.6, level: 2, speed: "x0.87" },
  { percent: 50.0, level: 1, speed: "x0.90" }
];

// weightPenalty resolves the active weight debuff of a load percent,
// null while the load stays under the first threshold.
function weightPenalty(percent) {
  for (const penalty of WEIGHT_PENALTIES) {
    if (percent >= penalty.percent) { return penalty; }
  }

  return null;
}

// weightFillStyle colors the weight bar of a load percent: green below
// the first debuff threshold, then the color melts from yellow through
// orange into red as the load climbs the server debuff levels - the
// hue interpolates between the level anchors so the bar transfers
// smoothly instead of jumping.
function weightFillStyle(percent) {
  if (percent < WEIGHT_PENALTIES[WEIGHT_PENALTIES.length - 1].percent) {
    return "";
  }
  // Hue anchors: yellow at 50%, orange at 66.6%, red at 80%, deep red
  // at 100%; lightness darkens slightly toward the overload.
  const stops = [
    { percent: 50.0, hue: 50, light: 50 },
    { percent: 66.6, hue: 26, light: 48 },
    { percent: 80.0, hue: 8, light: 45 },
    { percent: 100.0, hue: 0, light: 42 }
  ];
  let hue = stops[stops.length - 1].hue;
  let light = stops[stops.length - 1].light;
  for (let i = 0; i + 1 < stops.length; i++) {
    const a = stops[i];
    const b = stops[i + 1];
    if (percent >= a.percent && percent <= b.percent) {
      const f = (percent - a.percent) / (b.percent - a.percent);
      hue = a.hue + (b.hue - a.hue) * f;
      light = a.light + (b.light - a.light) * f;
      break;
    }
  }

  return "hsl(" + hue.toFixed(1) + ", 85%, " + light.toFixed(1) + "%)";
}

// renderGearFoot refreshes the pinned footer of the floating widget:
// the adena line and the weight line stay visible below the inventory
// grid whatever the scroll position of the bag is. The load bar fills
// by the load percentage and colors by the server weight debuff
// thresholds - green below 50%, then yellow melting into orange and
// red toward the overload levels.
function renderGearFoot(snap) {
  const c = snap.character || {};
  const adena = document.getElementById("gear-adena");
  if (adena) {
    adena.textContent = formatNumber(c.adena);
    adena.title = "adena: " + formatNumber(c.adena);
  }

  const fill = document.getElementById("gear-load-fill");
  const text = document.getElementById("gear-load-text");
  const row = document.getElementById("gear-weight-row");
  let percent = null;
  if (c.maxLoad > 0) {
    percent = Math.max(0, Math.min(100, (c.load / c.maxLoad) * 100));
  }
  if (fill && text) {
    if (percent === null) {
      fill.style.width = "0%";
      fill.className = "load-fill";
      fill.style.background = "";
      text.textContent = "—";
    } else {
      const penalty = weightPenalty(percent);
      fill.style.width = percent.toFixed(1) + "%";
      fill.className = "load-fill" +
        (penalty ? " pen" + penalty.level : "");
      fill.style.background = weightFillStyle(percent);
      text.textContent = Math.round(percent) + "%";
      text.style.color = penalty ? weightFillStyle(percent) : "";
    }
  }
  if (row) {
    const penalty = percent === null ? null : weightPenalty(percent);
    row.title = c.maxLoad > 0
      ? "weight: " + formatNumber(c.load) + " / " +
        formatNumber(c.maxLoad) +
        (penalty
          ? " · weight debuff " + penalty.level +
            " (speed " + penalty.speed + ")"
          : " · no weight debuff")
      : "weight";
  }
}

// ---- manual commands: map clicks, cell double clicks and drags ----

// postCommand queues one manual command on the active bot: the hunt
// loop picks it up within a quarter second and turns it into world
// action.
function postCommand(fields) {
  const botId = App.activeBotId;
  if (!botId) { return; }
  fetch("/api/bots/" + botId + "/commands", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(fields)
  }).catch(() => {});
}

// Wearable item families of the type2 ids: 0 weapons, 1 armor,
// 2 jewelry. Everything else (quest, adena, materials) is not
// equippable and a double click must not use it.
const WEARABLE_TYPE2_MAX = 2;

function isWearable(item) {
  return Boolean(item) && item.type2 >= 0 && item.type2 <= WEARABLE_TYPE2_MAX;
}

// SLOT_PAIRS lists the slot keys of the either-or families (the
// earrings and the rings): a new item of the family takes the first
// free slot, or replaces the first one when both are taken.
const SLOT_PAIRS = { ear: ["r_ear", "l_ear"], finger: ["r_finger", "l_finger"] };

// slotKeyOf resolves the paperdoll slot key an item equips into, using
// the same placement rules as assignPaperdoll: the either-or families
// take the first free slot of their pair (or the first slot when both
// are taken), the aliases map onto their target slot, anything else
// resolves by the direct body part mask.
function slotKeyOf(item, placed) {
  const either = EITHER_OR_MASKS[item.bodyPart];
  if (either) {
    for (const key of SLOT_PAIRS[either]) {
      if (!placed[key]) { return key; }
    }

    return SLOT_PAIRS[either][0];
  }
  if (SLOT_MASK_ALIASES[item.bodyPart]) {
    return SLOT_MASK_ALIASES[item.bodyPart];
  }
  const slot = WEAR_SLOTS.concat(JEWEL_SLOTS).find(
    (s) => s && s.mask === item.bodyPart);

  return slot ? slot.key : null;
}

// equipItem equips a bag item: when its slot is already taken the
// equipped item comes off first, so the new item takes its place (the
// command queue preserves the order: useItem(old), useItem(new)).
function equipItem(item) {
  if (!item || item.equipped || !isWearable(item)) { return; }
  const items = App.snapshot ? (App.snapshot.inventory || []) : [];
  const placed = assignPaperdoll(items);
  const key = slotKeyOf(item, placed);
  const occupied = key ? placed[key] : null;
  if (occupied && occupied.objectId !== item.objectId) {
    postCommand({ kind: "useItem", objectId: occupied.objectId });
  }
  postCommand({ kind: "useItem", objectId: item.objectId });
}

// The item currently leaving the equipment widget through a drag: set
// by dragstart, read by the drop targets (the map prefers the drag
// event data, the stub harnesses read this state).
const GearDrag = { item: null };

// activateGearCell is the double click of one widget cell: a wearable
// bag item equips (the server toggles the slot), an equipped item
// unequips. The useItem packet covers both directions.
function activateGearCell(record) {
  const item = record.item;
  if (!item) { return; }
  if (item.equipped) {
    postCommand({ kind: "useItem", objectId: item.objectId });

    return;
  }
  equipItem(item);
}

// startGearDrag arms the drag of one widget cell.
function startGearDrag(record, event) {
  const item = record.item;
  if (!item) {
    event.preventDefault();

    return;
  }
  GearDrag.item = item;
  try {
    event.dataTransfer.setData("application/x-swarm-item",
      JSON.stringify(item));
    event.dataTransfer.effectAllowed = "move";
  } catch (err) {
    // Stub DOMs without a real dataTransfer fall back to GearDrag.
  }
}

// draggedItem resolves the dragged item of a drop event: the drag
// payload first, the widget drag state as the fallback.
function draggedItem(event) {
  try {
    const raw = event.dataTransfer.getData("application/x-swarm-item");
    if (raw) { return JSON.parse(raw); }
  } catch (err) {
    // Read the fallback state below.
  }

  return GearDrag.item;
}

// The pending stackable drop or destroy behind the count dialog. The
// mode picks the action: "drop" throws the item on the ground, the
// "destroy" of the trash target deletes it.
const PendingDrop = { item: null, mode: "drop" };

// dropItemOnMap starts the ground drop of a dragged item: stackable
// items (adena, bones) ask for the count first, plain items drop
// whole.
function dropItemOnMap(item) {
  if (!item) { return; }
  if (item.count > 1) {
    openDropDialog(item, "drop");

    return;
  }
  commitDrop(item, 1);
}

// destroyItemFromWidget starts the destroy of a dragged item (the
// trash target of the footer): stackable items ask for the count first,
// plain items destroy whole.
function destroyItemFromWidget(item) {
  if (!item) { return; }
  if (item.count > 1) {
    openDropDialog(item, "destroy");

    return;
  }
  commitDestroy(item, 1);
}

// commitDrop sends the drop command: the server drops at the feet of
// the character (it refuses drops farther than 150 units away) and
// refuses equipped items, so an equipped drag unequips first - the
// queue preserves the order.
function commitDrop(item, count) {
  if (item.equipped) {
    postCommand({ kind: "useItem", objectId: item.objectId });
  }
  postCommand({ kind: "drop", objectId: item.objectId, count });
}

// commitDestroy sends the destroy command: the item is deleted on the
// server without ever touching the ground.
function commitDestroy(item, count) {
  if (item.equipped) {
    postCommand({ kind: "useItem", objectId: item.objectId });
  }
  postCommand({ kind: "destroy", objectId: item.objectId, count });
}

// resolveDropCount parses the count dialog answer: null rejects
// garbage input, a valid answer clamps into 1..max.
function resolveDropCount(input, max) {
  const value = Math.floor(Number(input));
  if (!Number.isFinite(value) || value < 1) { return null; }

  return Math.min(value, max);
}

// bumpDropCount steps the count dialog answer by delta and clamps it
// into the stack bounds: the scroll wheel over the dialog tunes the
// count without touching the keyboard, a garbage field counts as one.
function bumpDropCount(delta) {
  const item = PendingDrop.item;
  const input = document.getElementById("drop-count");
  if (!item || !input) { return; }
  const current = resolveDropCount(input.value, item.count);
  const base = current === null ? 1 : current;
  input.value = Math.max(1, Math.min(item.count, base + delta));
  input.classList.remove("invalid");
}

function openDropDialog(item, mode) {
  PendingDrop.item = item;
  PendingDrop.mode = mode === "destroy" ? "destroy" : "drop";
  const dialog = document.getElementById("drop-dialog");
  if (!dialog) { return; }
  const destroy = PendingDrop.mode === "destroy";
  document.getElementById("drop-head").textContent = destroy
    ? "destroy the item" : "drop on the ground";
  document.getElementById("drop-note").textContent = destroy
    ? "the item is gone for good" : "lands at the character feet";
  const ok = document.getElementById("drop-ok");
  ok.textContent = destroy ? "destroy" : "drop";
  ok.title = destroy ? "destroy the typed count" : "drop the typed count";
  const all = document.getElementById("drop-all");
  all.title = destroy ? "destroy the whole stack" : "drop the whole stack";
  document.getElementById("drop-name").textContent =
    item.name || ("item " + item.itemId);
  const input = document.getElementById("drop-count");
  input.value = 1;
  input.max = item.count;
  input.classList.remove("invalid");
  document.getElementById("drop-max").textContent = "/ " + item.count;
  dialog.classList.remove("hidden");
  input.focus();
  input.select();
}

function closeDropDialog() {
  PendingDrop.item = null;
  PendingDrop.mode = "drop";
  const dialog = document.getElementById("drop-dialog");
  if (dialog) { dialog.classList.add("hidden"); }
}

function confirmDropDialog() {
  const item = PendingDrop.item;
  if (!item) {
    closeDropDialog();

    return;
  }
  const input = document.getElementById("drop-count");
  const count = resolveDropCount(input.value, item.count);
  if (count === null) {
    input.classList.add("invalid");

    return;
  }
  const destroy = PendingDrop.mode === "destroy";
  closeDropDialog();
  if (destroy) {
    commitDestroy(item, count);

    return;
  }
  commitDrop(item, count);
}

// initGearInteractions wires the drop targets of the widget: the
// paperdoll area equips a dragged bag item, the bag unequips a
// dragged paperdoll item, the drop dialog buttons commit the count.
// The map itself is wired by map.js.
function initGearInteractions() {
  if (typeof document.querySelector !== "function") {
    // Stub DOMs of the reproduction harnesses wire no drop targets.
    return;
  }
  const slots = document.querySelector(".gear-slots");
  if (slots) {
    slots.addEventListener("dragover", (event) => {
      event.preventDefault();
      slots.classList.add("drop-hover");
    });
    slots.addEventListener("dragleave", () => {
      slots.classList.remove("drop-hover");
    });
    slots.addEventListener("drop", (event) => {
      event.preventDefault();
      slots.classList.remove("drop-hover");
      const item = draggedItem(event);
      if (item && !item.equipped && isWearable(item)) {
        equipItem(item);
      }
      GearDrag.item = null;
    });
  }
  const grid = document.getElementById("inv-grid");
  if (grid) {
    grid.addEventListener("dragover", (event) => {
      event.preventDefault();
      grid.classList.add("drop-hover");
    });
    grid.addEventListener("dragleave", () => {
      grid.classList.remove("drop-hover");
    });
    grid.addEventListener("drop", (event) => {
      event.preventDefault();
      grid.classList.remove("drop-hover");
      const item = draggedItem(event);
      if (item && item.equipped) {
        postCommand({ kind: "useItem", objectId: item.objectId });
      }
      GearDrag.item = null;
    });
  }
  const trash = document.getElementById("gear-trash");
  if (trash) {
    trash.addEventListener("dragover", (event) => {
      event.preventDefault();
      trash.classList.add("drop-hover");
    });
    trash.addEventListener("dragleave", () => {
      trash.classList.remove("drop-hover");
    });
    trash.addEventListener("drop", (event) => {
      event.preventDefault();
      trash.classList.remove("drop-hover");
      destroyItemFromWidget(draggedItem(event));
      GearDrag.item = null;
    });
  }
  const ok = document.getElementById("drop-ok");
  if (ok) { ok.addEventListener("click", confirmDropDialog); }
  const all = document.getElementById("drop-all");
  if (all) {
    all.addEventListener("click", () => {
      const item = PendingDrop.item;
      if (item) {
        const destroy = PendingDrop.mode === "destroy";
        closeDropDialog();
        if (destroy) {
          commitDestroy(item, item.count);

          return;
        }
        commitDrop(item, item.count);
      }
    });
  }
  const cancel = document.getElementById("drop-cancel");
  if (cancel) { cancel.addEventListener("click", closeDropDialog); }
  const input = document.getElementById("drop-count");
  if (input) {
    input.addEventListener("input", () => input.classList.remove("invalid"));
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter") { confirmDropDialog(); }
      if (event.key === "Escape") { closeDropDialog(); }
    });
  }
  const dialog = document.getElementById("drop-dialog");
  if (dialog) {
    // The scroll wheel over the open dialog steps the count: one notch
    // up or down, clamped into the stack. The listener must stay
    // non-passive because the wheel also scrolls the page otherwise.
    dialog.addEventListener("wheel", (event) => {
      if (dialog.classList.contains("hidden")) { return; }
      event.preventDefault();
      bumpDropCount(event.deltaY < 0 ? 1 : -1);
    }, { passive: false });
  }
}

// initTargetWidget arms the double click of the target HUD panel: the
// current target gets the attack command, exactly like a double click
// on the map. Friendly or dead targets ignore the click.
function initTargetWidget() {
  const panel = document.getElementById("hud-target");
  if (!panel) { return; }
  panel.addEventListener("dblclick", () => {
    const snap = App.snapshot;
    const c = snap ? snap.character : null;
    const targetId = c ? c.targetId : 0;
    if (!targetId) { return; }
    const target = (snap.objects || []).find(
      (obj) => obj.objectId === targetId);
    if (!target || target.kind !== "npc" || !target.attackable ||
      target.dead) {
      return;
    }
    postCommand({ kind: "attack", objectId: target.objectId });
  });
}

// Wire the interactions at script load: the scripts run at the end of
// the body, the widget markup is parsed already.
initGearInteractions();
initTargetWidget();

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
