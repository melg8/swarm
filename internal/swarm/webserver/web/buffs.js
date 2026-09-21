// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// ---- the effects panel (the server buff list) ----
//
// The panel renders the active effects of the server
// AbnormalStatusUpdate list in two view states that share one frame:
//
// - the icon grid (the default): one cell per active effect, at most
//   10 columns by 2 rows for the classic buff bar, every cell packs
//   edge to edge with the thin white separator strips between the
//   cells and carries the icon with the level badge and the short
//   remaining time overlaid at its bottom edge;
// - the detailed list: the full effect rows (icon, level, name, the
//   remaining time) with the remaining time percent bar under every
//   row, the tighter vertical rhythm and the scrollbar - the panel
//   never grows taller than the character HUD stack.
//
// Both views show exactly the active effects (two effects render two
// cells and two rows). The dock is the slim vertical strip on the
// left edge of the frame: it carries the expand chevron and the view
// switch icon, both buttons toggle between the two states and
// neither ever moves (the chevron stays pinned to the top left
// corner in both states, so collapsing and expanding again needs no
// re-aim). The view choice persists in the localStorage
// (swarm.buffsView). The panel hides entirely while no effect runs.
//
// The rows and the cells are persistent DOM nodes keyed by the skill
// id - the icon builds once per effect, the later refreshes only
// rewrite the badges and the countdowns (the keyed rendering rule of
// the equipment widget - the icons never blink), a buff joining or
// leaving the list touches only its own node.

// The panel geometry constants: the grid packs buffBuffsCellSize px
// cells with buffSepSize px white separator strips between them, at
// most buffGridColumns per row; the body adds buffBodyPadding px of
// panel colored frame around the content. The panel frame width per
// view state lives in the CSS next to these numbers.
const buffGridColumns = 10;
const buffCellSize = 32;
const buffSepSize = 2;
const buffBodyPadding = 3;
// The detailed list row pitch (the row box plus its tight padding)
// for the height fallback when the DOM does not answer measurements.
const buffListRowPitch = 36;
// The detailed list falls back to this height cap when the HUD stack
// does not answer a measurement (the harness stub DOM).
const buffListFallbackCap = 320;
// The frame chrome of the panel: the body padding on both edges plus
// the panel borders - the parts the pinned body height rides with.
const buffFrameChrome = buffBodyPadding * 2 + 2;

// The effects panel state: the keyed cells of the icon grid, the
// keyed rows of the detailed list, the countdown anchors of the last
// snapshot (the server sent left seconds at the at timestamp) and the
// view choice (icons or list).
const BuffsPanel = {
  cells: new Map(),
  rows: new Map(),
  anchors: new Map(),
  view: "icons"
};

// initBuffsPanel wires the dock buttons (both toggle the view
// state), restores the stored view choice, tracks the HUD stack size
// (the detailed list never outgrows it) and starts the one second
// countdown ticker that keeps the remaining times honest between the
// server snapshots.
function initBuffsPanel() {
  const panel = document.getElementById("buffs-panel");
  if (!panel) { return; }
  BuffsPanel.view = window.localStorage.getItem("swarm.buffsView") === "list"
    ? "list"
    : "icons";
  const flip = () => {
    setBuffsView(BuffsPanel.view === "icons" ? "list" : "icons");
  };
  const toggle = document.getElementById("buffs-toggle");
  const viewBtn = document.getElementById("buffs-view-btn");
  if (toggle) { toggle.addEventListener("click", flip); }
  if (viewBtn) { viewBtn.addEventListener("click", flip); }
  const stack = document.querySelector(".hud-stack");
  if (typeof ResizeObserver === "function" && stack &&
    typeof stack.addEventListener === "function") {
    new ResizeObserver(() => syncBuffsPanelSize()).observe(stack);
  }
  window.addEventListener("resize", () => syncBuffsPanelSize());
  window.setInterval(() => tickBuffsCountdowns(), 1000);
  applyBuffsPanelState();
}

// setBuffsView switches the panel between the icon grid and the
// detailed list (the CSS transition morphs the frame both ways) and
// persists the choice.
function setBuffsView(view) {
  if (view !== "icons" && view !== "list") { return; }
  BuffsPanel.view = view;
  window.localStorage.setItem("swarm.buffsView", view);
  applyBuffsPanelState();
}

// applyBuffsPanelState syncs the panel DOM with the view choice: the
// view classes drive the frame width and the layer cross-fade in the
// CSS, the button titles name the action each click performs.
function applyBuffsPanelState() {
  const panel = document.getElementById("buffs-panel");
  if (!panel) { return; }
  panel.classList.toggle("view-icons", BuffsPanel.view === "icons");
  panel.classList.toggle("view-list", BuffsPanel.view === "list");
  const toggle = document.getElementById("buffs-toggle");
  const viewBtn = document.getElementById("buffs-view-btn");
  if (toggle) {
    toggle.title = BuffsPanel.view === "icons"
      ? "expand the detailed effect list"
      : "collapse the effect list back to the icons";
  }
  if (viewBtn) {
    viewBtn.title = BuffsPanel.view === "icons"
      ? "switch to the detailed effect list"
      : "switch back to the icon grid";
  }
  syncBuffsPanelSize();
}

// buffsHudHeight returns the rendered height of the character HUD
// stack (the character panel plus the target panel): the ceiling the
// detailed list view must never outgrow. Zero when the stack does
// not answer a measurement.
function buffsHudHeight() {
  const stack = document.querySelector(".hud-stack");
  return stack && stack.offsetHeight > 0 ? stack.offsetHeight : 0;
}

// syncBuffsPanelSize pins the panel body height to the content of
// the active view: the grid height (one row of cells or two, the
// dock follows the same height) or the detailed list content capped
// at the HUD stack height. The explicit heights are what the CSS
// height transition animates between the states. The count
// arithmetic covers the stub DOM of the harness where the elements
// answer no measurements.
function syncBuffsPanelSize() {
  const body = document.getElementById("buffs-panel-body");
  if (!body) { return; }
  let height = 0;
  if (BuffsPanel.view === "icons") {
    const grid = document.getElementById("buffs-grid");
    height = grid ? grid.offsetHeight : 0;
    if (!height) {
      const rows = Math.max(1, Math.ceil(BuffsPanel.cells.size /
        buffGridColumns));
      height = rows * buffCellSize + (rows - 1) * buffSepSize;
    }
  } else {
    const list = document.getElementById("buffs-list");
    height = list ? list.scrollHeight : 0;
    // The HUD ceiling counts the whole frame in: the body padding
    // and the panel borders ride on top of the pinned height, so
    // the chrome comes off the cap (the expanded panel never
    // outgrows the character widget).
    const cap = Math.max(0,
      (buffsHudHeight() || buffListFallbackCap) - buffFrameChrome);
    if (height > cap) { height = cap; }
    if (!height) {
      height = Math.min(BuffsPanel.rows.size * buffListRowPitch, cap);
    }
  }
  body.style.height = (height + buffBodyPadding * 2) + "px";
}

// buffLeftText formats the remaining seconds of an effect: the short
// human form (90s, 12m 30s, 1h 05m), a dash for the ones the server
// never times out (the clamped day-long list entries still tick down
// but a fresh self buff never shows as expired).
function buffLeftText(left) {
  if (left >= 86400) { return "\u2014"; }
  const secs = Math.max(0, left);
  if (secs < 60) { return secs + "s"; }
  if (secs < 3600) {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return m + "m" + (s > 0 ? " " + s + "s" : "");
  }
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return h + "h" + (m > 0 ? " " + (m < 10 ? "0" : "") + m + "m" : "");
}

// buffLeftShort formats the remaining seconds for the overlay strip
// of a grid cell: the same reading as buffLeftText compressed to the
// couple of characters a 32px cell fits (45s, 12m, 1h30), a dash for
// the never timed out ones.
function buffLeftShort(left) {
  if (left >= 86400) { return "\u2014"; }
  const secs = Math.max(0, left);
  if (secs < 60) { return secs + "s"; }
  if (secs < 3600) { return Math.floor(secs / 60) + "m"; }
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return h + "h" + (m > 0 ? String(m).padStart(2, "0") : "");
}

// buffAnchorOf stores and returns the countdown anchor of one effect:
// a shallow copy of the last snapshot entry (the skill fields the
// renderers read) plus the at timestamp - the server sent left
// seconds at that moment, the local ticker counts the elapsed wall
// clock off that reading between the snapshots.
function buffAnchorOf(skillId, buff) {
  const anchor = BuffsPanel.anchors.get(skillId);
  if (anchor) {
    anchor.left = buff.left;
    anchor.total = buff.total;
    anchor.at = Date.now();

    return anchor;
  }
  const fresh = Object.assign({ at: Date.now() }, buff);
  BuffsPanel.anchors.set(skillId, fresh);

  return fresh;
}

// buffLiveLeft reads the anchored remaining seconds now: the snapshot
// reading minus the wall clock seconds since it arrived, floored at
// zero (the effect that outlived its reading waits for the next
// snapshot to disappear).
function buffLiveLeft(anchor) {
  return Math.max(0, Math.round(anchor.left - (Date.now() - anchor.at) / 1000));
}

// makeBuffCell creates one keyed grid cell: the icon box with its
// level badge and the countdown overlay strip. The icon image itself
// waits for the first snapshot entry (a missing icon leaves the
// plain box).
function makeBuffCell() {
  const item = document.createElement("div");
  item.className = "buff-cell";
  const img = document.createElement("img");
  img.alt = "";
  const level = document.createElement("span");
  level.className = "badge-level";
  const left = document.createElement("span");
  left.className = "buff-left";
  item.append(img, level, left);

  return { item, img, level, left };
}

// applyBuffCell refreshes one keyed grid cell to the anchored
// snapshot entry (the anchor carries the skill fields and the at
// moment of the reading): the icon source builds once (a failing
// load removes the image and leaves the plain box), the level badge
// and the countdown rewrite on every snapshot and every local tick.
function applyBuffCell(cell, buff) {
  if (!cell.img.src && buff.icon) {
    cell.img.src = "/icons/" + buff.icon + ".png";
    cell.img.addEventListener("error", () => cell.img.remove());
  }
  const left = buffLiveLeft(buff);
  cell.level.textContent = String(buff.level);
  cell.left.textContent = buffLeftShort(left);
  cell.left.classList.toggle("fading", left > 0 && left < 60);
  const name = buff.name || ("effect #" + buff.skillId);
  cell.item.title = name + " level " + buff.level + " \u00b7 " +
    buffLeftText(left) + " left";
}

// makeBuffRow creates one keyed effect row of the detailed list: the
// icon box with its level badge, the name plus the countdown line
// and the remaining time percent bar pinned to the row bottom edge.
function makeBuffRow() {
  const item = document.createElement("li");
  item.className = "buff-item";
  const icon = document.createElement("div");
  icon.className = "buff-icon";
  const level = document.createElement("span");
  level.className = "badge-level";
  icon.append(level);
  const text = document.createElement("div");
  text.className = "buff-text";
  const name = document.createElement("div");
  name.className = "buff-name";
  const meta = document.createElement("div");
  meta.className = "buff-meta";
  text.append(name, meta);
  const bar = document.createElement("span");
  bar.className = "buff-bar";
  const fill = document.createElement("i");
  bar.append(fill);
  item.append(icon, text, bar);

  return { item, icon, img: null, name, meta, level, bar, fill };
}

// applyBuffRow refreshes one keyed effect row to the anchored
// snapshot entry: the icon image builds once (a failing load removes
// it and leaves the plain box), the name, the level badge, the
// countdown and the remaining time percent bar (the remaining share
// of the duration the effect landed with) rewrite on every snapshot
// and every local tick. The rows keep the tight vertical rhythm -
// the list packs more effects into the HUD height.
function applyBuffRow(row, buff) {
  if (!row.img && buff.icon) {
    const img = document.createElement("img");
    img.alt = "";
    img.src = "/icons/" + buff.icon + ".png";
    img.addEventListener("error", () => img.remove());
    row.icon.append(img);
    row.img = img;
  }
  const left = buffLiveLeft(buff);
  row.name.textContent = buff.name || ("effect #" + buff.skillId);
  row.level.textContent = String(buff.level);
  row.meta.textContent = buffLeftText(left);
  row.meta.classList.toggle("fading", left > 0 && left < 60);
  row.fill.style.width = buff.total > 0
    ? Math.min(100, Math.max(0, left / buff.total * 100)) + "%"
    : "0%";
  row.item.title = (buff.name || ("effect #" + buff.skillId)) +
    " level " + buff.level + " \u00b7 " + buffLeftText(left) + " left";
}

// syncBuffsKeyed diffs one keyed view (the grid cells or the list
// rows) against the snapshot list: the effects that left the server
// list drop their nodes, the effects that joined build fresh ones
// (with the brief enter animation), the surviving ones refresh in
// place and every node ends in the snapshot order (appending an
// existing node moves it, the icons never re-decode for a reorder).
function syncBuffsKeyed(container, buffs, store, make, apply) {
  const seen = new Set();
  for (const buff of buffs) { seen.add(buff.skillId); }
  for (const [id, entry] of store) {
    if (!seen.has(id)) {
      entry.item.remove();
      store.delete(id);
      BuffsPanel.anchors.delete(id);
    }
  }
  for (const buff of buffs) {
    let entry = store.get(buff.skillId);
    const fresh = !entry;
    if (fresh) {
      entry = make();
      store.set(buff.skillId, entry);
      entry.item.classList.add("buff-enter");
      window.setTimeout(((node) => () => node.classList.remove("buff-enter"))(
        entry.item), 260);
    }
    const anchor = buffAnchorOf(buff.skillId, buff);
    apply(entry, anchor);
    container.append(entry.item);
  }
}

// renderBuffs refreshes the effects panel of the map: the icon grid
// and the detailed list both diff against the same snapshot list,
// the frame sizes follow the active view. The panel hides while no
// effect runs and holds the previous bot's content through the
// switch gap (the keyed diff of the first snapshot of the new bot
// answers the hold, nothing wipes here).
function renderBuffs(snap) {
  const section = document.getElementById("buffs-panel");
  const grid = document.getElementById("buffs-grid");
  const list = document.getElementById("buffs-list");
  if (!section || !grid || !list) { return; }
  const buffs = Array.isArray(snap.buffs) ? snap.buffs : [];
  if (buffs.length === 0) {
    section.classList.add("hidden");
    BuffsPanel.cells.clear();
    BuffsPanel.rows.clear();
    BuffsPanel.anchors.clear();
    grid.innerHTML = "";
    list.innerHTML = "";

    return;
  }
  section.classList.remove("hidden");
  syncBuffsKeyed(grid, buffs, BuffsPanel.cells,
    makeBuffCell, applyBuffCell);
  syncBuffsKeyed(list, buffs, BuffsPanel.rows, makeBuffRow, applyBuffRow);
  applyBuffsPanelState();
}

// tickBuffsCountdowns keeps the remaining times running between the
// server snapshots: every second the anchored readings count the
// elapsed wall clock off and the visible view rewrites its
// countdowns, the fading marks and the percent bar fills. A zero
// reading stays pinned until the next snapshot removes the effect.
function tickBuffsCountdowns() {
  if (BuffsPanel.view === "icons") {
    for (const [id, cell] of BuffsPanel.cells) {
      const anchor = BuffsPanel.anchors.get(id);
      if (anchor) { applyBuffCell(cell, anchor); }
    }

    return;
  }
  for (const [id, row] of BuffsPanel.rows) {
    const anchor = BuffsPanel.anchors.get(id);
    if (anchor) { applyBuffRow(row, anchor); }
  }
}
