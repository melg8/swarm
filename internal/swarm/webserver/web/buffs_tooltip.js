// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// ---- the effects tooltip (the rich hover card of a buff) ----
//
// Hovering a buff in the effects panel (an icon grid cell) shows the
// floating tooltip card the page keeps as the #buffs-tooltip
// singleton (the same mechanics the item tooltip of the gear widget
// uses): the card answers what the effect really is - the name with
// the level, the numeric effect summary the server skill stats bite
// with (e.g. Wind Walk 2 answers "+33 Speed"), the generic level
// description text and the remaining time. The lines the snapshot
// carries no data for collapse away.
//
// The wiring is delegated on the panel (one mouseover / mousemove /
// mouseleave trio instead of listeners per keyed node), so the keyed
// re-appends of the renderer never rewire and never orphan a
// handler. The hovered cell identifies itself through its
// data-skill-id attribute; the anchor map of the panel carries the
// display fields. All texts reach the card as textContent.

// BuffsTooltip holds the tooltip session: the singleton element, the
// four content lines (built once, refilled per hover) and the box
// element the card currently answers.
const BuffsTooltip = {
    element: null,
    parts: null,
    box: null
};

// buffsTooltipElement lazily fetches the singleton DOM node of the
// card. The node lives once on the page and is repurposed on every
// hover; a page without the node simply never shows the card.
function buffsTooltipElement() {
    if (BuffsTooltip.element) { return BuffsTooltip.element; }
    const el = document.getElementById("buffs-tooltip");
    if (!el) { return null; }
    BuffsTooltip.element = el;

    return el;
}

// buffsTooltipParts lazily builds the four content lines of the card
// (the name with the level, the remaining time, the numeric effect
// summary and the generic description). The lines persist with the
// card, the hovers only rewrite their text.
function buffsTooltipParts() {
    if (BuffsTooltip.parts) { return BuffsTooltip.parts; }
    const el = buffsTooltipElement();
    if (!el) { return null; }
    const name = document.createElement("div");
    name.className = "bt-name";
    const meta = document.createElement("div");
    meta.className = "bt-meta";
    const effect = document.createElement("div");
    effect.className = "bt-effect";
    const desc = document.createElement("div");
    desc.className = "bt-desc";
    el.append(name, meta, effect, desc);
    BuffsTooltip.parts = { name, meta, effect, desc };

    return BuffsTooltip.parts;
}

// showBuffsTooltip fills the card for one hovered buff box (a grid
// cell) and unhides it near the pointer. The box names its effect
// through the data-skill-id attribute; an unknown one hides the card
// instead of guessing.
function showBuffsTooltip(box, x, y) {
    const el = buffsTooltipElement();
    const parts = buffsTooltipParts();
    if (!el || !parts) { return; }
    const id = Number.parseInt(box.getAttribute("data-skill-id"), 10);
    const anchor = Number.isNaN(id)
        ? null : BuffsPanel.anchors.get(id);
    if (!anchor) { hideBuffsTooltip(); return; }
    const left = buffLiveLeft(anchor);
    const name = anchor.name || ("effect #" + anchor.skillId);
    parts.name.textContent = name + " \u00b7 lvl " + anchor.level;
    parts.meta.textContent = buffLeftText(left) + " left";
    if (anchor.effect) {
        parts.effect.textContent = anchor.effect;
        parts.effect.style.display = "";
    } else {
        parts.effect.style.display = "none";
    }
    if (anchor.desc) {
        parts.desc.textContent = anchor.desc;
        parts.desc.style.display = "";
    } else {
        parts.desc.style.display = "none";
    }
    BuffsTooltip.box = box;
    el.classList.remove("hidden");
    positionBuffsTooltip(x, y);
}

// positionBuffsTooltip keeps the card near the cursor (the fixed
// position works in viewport coordinates, the offsets place the card
// to the lower right of the pointer so it never covers the hovered
// buff). An event without coordinates (the harness stub) leaves the
// card where it is.
function positionBuffsTooltip(x, y) {
    if (!BuffsTooltip.element) { return; }
    if (typeof x !== "number" || typeof y !== "number") { return; }
    BuffsTooltip.element.style.left = (x + 14) + "px";
    BuffsTooltip.element.style.top = (y + 16) + "px";
}

// hideBuffsTooltip hides the card and drops the session box.
function hideBuffsTooltip() {
    BuffsTooltip.box = null;
    if (BuffsTooltip.element) {
        BuffsTooltip.element.classList.add("hidden");
    }
}

// initBuffsTooltip wires the delegated hover handlers of the effects
// panel. Called once from initBuffsPanel; the panel without the
// card node (and the stub DOM of the harness) stays silent instead
// of dying.
function initBuffsTooltip() {
    const panel = document.getElementById("buffs-panel");
    if (!panel || panel.dataset.tooltipInit) { return; }
    panel.dataset.tooltipInit = "1";
    panel.addEventListener("mouseover", (event) => {
        const target = event && event.target;
        const box = target && typeof target.closest === "function"
        ? target.closest(".buff-cell") : null;
        if (!box) { hideBuffsTooltip(); return; }
        if (box === BuffsTooltip.box) { return; }
        showBuffsTooltip(box, event.clientX, event.clientY);
    });
    panel.addEventListener("mousemove", (event) => {
        if (!BuffsTooltip.box) { return; }
        positionBuffsTooltip(event.clientX, event.clientY);
    });
    panel.addEventListener("mouseleave", () => hideBuffsTooltip());
}
