// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// ---- the effects panel (the server buff list) ----
//
// The panel renders the active effects of the server
// AbnormalStatusUpdate list in two view states that share one frame:
//
// - the icon grid (the default): one cell per active effect, at most
//   10 columns by 2 rows for the classic buff bar (a longer list
//   scrolls inside the grid), every cell packs edge to edge with the
//   thin white separator strips on its right and bottom edges and
//   carries the icon with the level badge, the remaining time chip
//   on hover and the mini time strip pinned to its bottom pixels;
// - the detailed list: the full effect rows (icon, level, name, the
//   remaining time) with the remaining time percent bar under every
//   row, the tighter vertical rhythm and the scrollbar - the panel
//   body fills EXACTLY the height of the character HUD stack (never
//   shorter), so the pair reads as one block.
//
// Both views show exactly the active effects (two effects render two
// cells and two rows). The dock is the slim vertical strip on the
// left edge of the frame: it carries the single expand chevron (the
// only control - the old view switch is gone) pinned to the top, so
// it never moves between the states and flipping it needs no
// re-aim; the chevron rotates 180 degrees with a springy ease on
// every click. The view choice persists in the localStorage
// (swarm.buffsView). The panel hides entirely while no effect runs.
//
// Toggling the view flies the visible icons from their old spots to
// the new ones (the FLIP morph of buffs_flip.js) and hovering a buff
// in either view opens the rich tooltip card (buffs_tooltip.js).
//
// The rows and the cells are persistent DOM nodes keyed by the skill
// id - the icon builds once per effect, the later refreshes only
// rewrite the badges and the countdowns (the keyed rendering rule of
// the equipment widget - the icons never blink), a buff joining or
// leaving the list touches only its own node.

// The panel geometry constants: the grid packs buffCellSize px
// cells at most buffGridColumns per row and at most two rows (the
// classic buff bar; the cell matches the icon box of the detailed
// list, and ten of them keep the frame clear of the central status
// banner), the body adds buffBodyPaddingY px of panel colored frame
// on the top and the bottom edges (the horizontal one lives in the
// CSS next to the frame widths).
const buffGridColumns = 10;
const buffGridRows = 2;
const buffCellSize = 30;
const buffBodyPaddingY = 2;
// The horizontal chrome the frame width rides with: the chevron dock
// strip (the 22px button, its 2px paddings and the 1px border) plus
// the side body padding (2 x 3px) and the frame borders (2 x 1px).
const buffDockWidth = 27;
const buffPanelSide = 8;
// The detailed list falls back to this height cap when the HUD stack
// does not answer a measurement (the harness stub DOM).
const buffListFallbackCap = 320;
// The frame chrome of the panel: the body padding on the vertical
// edges plus the panel borders - the parts the pinned body height
// rides with.
const buffFrameChrome = buffBodyPaddingY * 2 + 2;

// The effects panel state: the keyed cells of the icon grid, the
// keyed rows of the detailed list, the countdown anchors of the last
// snapshot (the server sent left seconds at the at timestamp), the
// view choice (icons or list) and the flip run token of the view
// morph.
const BuffsPanel = {
  cells: new Map(),
  rows: new Map(),
  anchors: new Map(),
  view: "icons",
  flipRun: 0
};

// initBuffsPanel wires the dock chevron (the single view toggle),
// restores the stored view choice, tracks the HUD stack size (the
// detailed list fills exactly its height), starts the one second
// countdown ticker that keeps the remaining times honest between
// the server snapshots and wires the hover tooltip card.
function initBuffsPanel() {
  const panel = document.getElementById("buffs-panel");
  if (!panel || panel.dataset.buffsInit) { return; }
  panel.dataset.buffsInit = "1";
  BuffsPanel.view = buffsStoredView() === "list" ? "list" : "icons";
  const toggle = document.getElementById("buffs-toggle");
  if (toggle) { toggle.addEventListener("click", () => {
    setBuffsView(BuffsPanel.view === "icons" ? "list" : "icons");
  }); }
  initBuffsTooltip();
  const stack = document.querySelector(".hud-stack");
  if (typeof ResizeObserver === "function" && stack &&
    typeof stack.addEventListener === "function") {
    new ResizeObserver(() => syncBuffsPanelSize()).observe(stack);
  }
  window.addEventListener("resize", () => syncBuffsPanelSize());
  window.setInterval(() => tickBuffsCountdowns(), 1000);
  applyBuffsPanelState();
}

// buffsStoredView answers the persisted view choice (the storage
// can be blocked - the panel answers the icons default instead of
// dying, the same guard the gear mode carries).
function buffsStoredView() {
  try {
    return window.localStorage.getItem("swarm.buffsView");
  } catch (_e) {
    return null;
  }
}

// setBuffsView switches the panel between the icon grid and the
// detailed list (the FLIP morph of buffs_flip.js flies the visible
// icons across while the CSS morphs the frame) and persists the
// choice (best effort, see buffsStoredView).
function setBuffsView(view) {
  if (view !== "icons" && view !== "list") { return; }
  const previous = BuffsPanel.view;
  BuffsPanel.view = view;
  try {
    window.localStorage.setItem("swarm.buffsView", view);
  } catch (_e) {
    // The choice lives for the session only then.
  }
  flipBuffsView(previous, applyBuffsPanelState);
}

// applyBuffsPanelState syncs the panel DOM with the view choice: the
// view classes drive the frame width and the layer cross-fade in the
// CSS, the chevron title names the action the click performs.
function applyBuffsPanelState() {
  const panel = document.getElementById("buffs-panel");
  if (!panel) { return; }
  panel.classList.toggle("view-icons", BuffsPanel.view === "icons");
  panel.classList.toggle("view-list", BuffsPanel.view === "list");
  const toggle = document.getElementById("buffs-toggle");
  if (toggle) {
    toggle.title = BuffsPanel.view === "icons"
      ? "expand the detailed effect list"
      : "collapse the effect list back to the icons";
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

// syncBuffsPanelSize pins the panel shape to the view: the grid
// height (one row of cells or two) with the frame hugging the filled
// columns (a partially filled row never reserves dead space over the
// map, so the frame stays clear of the central status banner for
// every count up to the full 10 column row), or the EXACT height of
// the character HUD stack for the list (the body always fills it,
// never a shorter content measure - the vertical widget reads as
// tall as the character widget). The explicit heights and widths are
// what the CSS transitions animate between the states. The count
// arithmetic covers the stub DOM of the harness where the elements
// answer no measurements.
function syncBuffsPanelSize() {
  const body = document.getElementById("buffs-panel-body");
  const panel = document.getElementById("buffs-panel");
  if (!body || !panel) { return; }
  let height = 0;
  if (BuffsPanel.view === "icons") {
    const grid = document.getElementById("buffs-grid");
    height = grid ? grid.offsetHeight : 0;
    const cols = Math.min(buffGridColumns,
      Math.max(1, BuffsPanel.cells.size));
    panel.style.width =
      (cols * buffCellSize + buffDockWidth + buffPanelSide) + "px";
    if (!height) {
      // The grid CSS caps the shape at the classic two rows (a
      // longer list scrolls inside it), the arithmetic mirrors the
      // cap for the stub DOM that answers no measurements.
      const rows = Math.min(buffGridRows, Math.max(1, Math.ceil(
        BuffsPanel.cells.size / buffGridColumns)));
      height = rows * buffCellSize;
    }
  } else {
    // The list keeps its CSS reading width.
    panel.style.width = "";
    // The HUD ceiling counts the whole frame in: the body padding
    // and the panel borders ride on top of the pinned height, so
    // the chrome comes off the cap - the expanded panel matches
    // the character widget height exactly.
    height = Math.max(0,
      (buffsHudHeight() || buffListFallbackCap) - buffFrameChrome);
  }
  body.style.height = (height + buffBodyPaddingY * 2) + "px";
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
    // Every snapshot field refreshes: another cast can overwrite
    // the same skill id with a new level, name or icon.
    Object.assign(anchor, buff, { at: Date.now() });

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
// level badge, the remaining time chip (shown on hover only) and
// the mini time strip pinned to the bottom pixels. The icon image
// itself waits for the first snapshot entry (a missing icon leaves
// the plain box).
function makeBuffCell() {
  const item = document.createElement("div");
  item.className = "buff-cell";
  const img = document.createElement("img");
  img.alt = "";
  const level = document.createElement("span");
  level.className = "badge-level";
  const left = document.createElement("span");
  left.className = "buff-left";
  const strip = document.createElement("i");
  strip.className = "buff-strip";
  item.append(img, level, left, strip);

  return { item, img, level, left, strip };
}

// applyBuffCell refreshes one keyed grid cell to the anchored
// snapshot entry (the anchor carries the skill fields and the at
// moment of the reading): the icon source builds once (a failing
// load removes the image and leaves the plain box), the level badge,
// the hover countdown and the mini time strip (the remaining share
// of the duration) rewrite on every snapshot and every local tick.
function applyBuffCell(cell, buff) {
  if (!cell.img.src && buff.icon) {
    cell.img.src = "/icons/" + buff.icon + ".png";
    cell.img.addEventListener("error", () => cell.img.remove());
  }
  const left = buffLiveLeft(buff);
  cell.level.textContent = String(buff.level);
  cell.left.textContent = buffLeftShort(left);
  cell.left.classList.toggle("fading", left > 0 && left < 60);
  cell.strip.style.width = buff.total > 0
    ? Math.min(100, Math.max(0, left / buff.total * 100)) + "%"
    : "0%";
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
}

// syncBuffsKeyed diffs one keyed view (the grid cells or the list
// rows) against the snapshot list: the effects that left the server
// list drop their nodes, the effects that joined build fresh ones
// (with the spawn animation), the surviving ones refresh in place
// and every node ends in the snapshot order (appending an
// existing node moves it, the icons never re-decode for a reorder).
// Every node carries its skill id in the data-skill-id attribute -
// the hover tooltip card resolves the anchor through it.
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
      entry.item.classList.add("buff-spawn");
      window.setTimeout(((node) => () => node.classList.remove(
        "buff-spawn"))(entry.item), 700);
    }
    entry.item.setAttribute("data-skill-id", String(buff.skillId));
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
