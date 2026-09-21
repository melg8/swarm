// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// ---- the effects panel (the server buff list) ----
//
// The panel renders the active effects of the server
// AbnormalStatusUpdate list as the single horizontal icon grid - the
// classic buff bar (the vertical detailed list is gone by decision):
// one cell per active effect, at most 10 columns by 2 rows, every
// cell packs edge to edge with the thin separator strips on its
// right and bottom edges (theme tinted, the white chrome of the
// light theme and the panel chrome of the dark one) and carries the
// icon at its native 32px size - nothing scales - plus the level
// badge, the remaining time chip on hover
// and the mini time strip pinned to its bottom pixels.
//
// The panel renders strictly and classically: no view switch, no
// toggle control, no appear or disappear animation - a buff that
// joins or leaves the list touches only its own cell and the frame
// follows instantly. More than the 20 visible slots stay clipped
// (no scrollbar - the strict classic bar owns the cap; a clipped
// effect surfaces as soon as a slot frees). The panel hides entirely
// while no effect runs. Hovering a cell opens the rich tooltip card
// (buffs_tooltip.js).
//
// The cells are persistent DOM nodes keyed by the skill id - the icon
// builds once per effect, the later refreshes only rewrite the badge
// and the countdown (the keyed rendering rule of the equipment widget
// - the icons never blink), a buff joining or leaving the list
// touches only its own node.

// The panel geometry constants: the grid packs buffCellSize px bare
// icons at most buffGridColumns per row and at most buffGridRows rows
// (the classic buff bar), every two neighbours separate by
// buffCellGap px of empty space (buffCellStep is the full stride the
// arithmetic rides with - the stride counts the gap once, so a filled
// row measures cols*step - gap and a filled stack rows*step - gap).
// The framing is gone by decision: no panel background, border,
// shadow or padding - the icons float over the map and only the
// small separator spaces between them stay. Keep in sync with the
// .buffs-panel geometry in style.css.
const buffGridColumns = 10;
const buffGridRows = 2;
const buffCellSize = 32;
const buffCellGap = 2;
const buffCellStep = buffCellSize + buffCellGap;

// The effects panel state: the keyed cells of the icon grid and the
// countdown anchors of the last snapshot (the server sent left
// seconds at the at timestamp).
const BuffsPanel = {
  cells: new Map(),
  anchors: new Map()
};

// initBuffsPanel starts the one second countdown ticker that keeps
// the remaining times honest between the server snapshots, sizes the
// frame for the first render and wires the hover tooltip card.
function initBuffsPanel() {
  const panel = document.getElementById("buffs-panel");
  if (!panel || panel.dataset.buffsInit) { return; }
  panel.dataset.buffsInit = "1";
  initBuffsTooltip();
  window.setInterval(() => tickBuffsCountdowns(), 1000);
  syncBuffsPanelSize();
}

// syncBuffsPanelSize pins the panel shape to the filled grid: the
// frame hugs the filled columns (a partially filled row never
// reserves dead space over the map, so the frame stays clear of the
// central status banner for every count up to the full 10 column
// row) and the body hugs the filled rows capped at the classic two.
// The stride arithmetic drops the trailing gap of the last column
// and row. The arithmetic covers the stub DOM of the harness and the
// real DOM alike - one code path, no measurement dependency.
function syncBuffsPanelSize() {
  const body = document.getElementById("buffs-panel-body");
  const panel = document.getElementById("buffs-panel");
  if (!body || !panel) { return; }
  const cols = Math.min(buffGridColumns,
    Math.max(1, BuffsPanel.cells.size));
  const rows = Math.min(buffGridRows, Math.max(1, Math.ceil(
    BuffsPanel.cells.size / buffGridColumns)));
  panel.style.width = (cols * buffCellStep - buffCellGap) + "px";
  body.style.height = (rows * buffCellStep - buffCellGap) + "px";
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

// syncBuffsKeyed diffs the keyed grid against the snapshot list: the
// effects that left the server list drop their nodes, the effects
// that joined build fresh ones, the surviving ones refresh in place
// and every node ends in the snapshot order. Appending an existing
// node moves it to the end, and a move detaches the node for a
// moment - the browser then drops its :hover state until the next
// real mouse move, which flickered the hover countdown chip on every
// snapshot of the 300 ms event stream under a stationary cursor. So
// a node already sitting at its snapshot index stays untouched, an
// out of place one inserts directly at its snapshot index (insert
// before the node that sits there now - one move lands it, no matter
// the permutation), and the icons never re-decode for a reorder.
// No spawn or leave animation runs - a change snaps, the strict
// classic read. Every node carries its skill id in the data-skill-id
// attribute - the hover tooltip card resolves the anchor through it.
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
  let index = 0;
  for (const buff of buffs) {
    let entry = store.get(buff.skillId);
    if (!entry) {
      entry = make();
      store.set(buff.skillId, entry);
    }
    entry.item.setAttribute("data-skill-id", String(buff.skillId));
    const anchor = buffAnchorOf(buff.skillId, buff);
    apply(entry, anchor);
    if (container.children[index] !== entry.item) {
      container.insertBefore(entry.item,
        container.children[index] || null);
    }
    index += 1;
  }
}

// renderBuffs refreshes the effects panel of the map: the icon grid
// diffs against the snapshot list and the frame follows the filled
// shape. The panel hides while no effect runs and holds the previous
// bot's content through the switch gap (the keyed diff of the first
// snapshot of the new bot answers the hold, nothing wipes here).
function renderBuffs(snap) {
  const section = document.getElementById("buffs-panel");
  const grid = document.getElementById("buffs-grid");
  if (!section || !grid) { return; }
  const buffs = Array.isArray(snap.buffs) ? snap.buffs : [];
  if (buffs.length === 0) {
    section.classList.add("hidden");
    BuffsPanel.cells.clear();
    BuffsPanel.anchors.clear();
    grid.innerHTML = "";

    return;
  }
  section.classList.remove("hidden");
  syncBuffsKeyed(grid, buffs, BuffsPanel.cells,
    makeBuffCell, applyBuffCell);
  syncBuffsPanelSize();
}

// tickBuffsCountdowns keeps the remaining times running between the
// server snapshots: every second the anchored readings count the
// elapsed wall clock off and every cell rewrites its countdown, the
// fading mark and the strip fill. A zero reading stays pinned until
// the next snapshot removes the effect.
function tickBuffsCountdowns() {
  for (const [id, cell] of BuffsPanel.cells) {
    const anchor = BuffsPanel.anchors.get(id);
    if (anchor) { applyBuffCell(cell, anchor); }
  }
}
