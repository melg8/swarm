// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// ---- the view flip (the morph between the grid and the list) ----
//
// Toggling the effects panel between the icon grid and the detailed
// list flies every visible icon from its old spot to its new one (a
// FLIP transition: the old boxes are measured first, the view
// switches, the new boxes are measured, and each icon animates from
// the inverted delta back to rest). The frame morph, the layer
// cross-fade and the chevron flip ride on in the CSS - the icons
// glide over them, so the list reads as the grid standing up into
// rows instead of two panels swapping.
//
// Only the buffs the destination widget really shows take the
// flight: the grid clips past its two rows and the list scrolls -
// the entries outside the pinned body height stay still (their rows
// simply fade in with the layer). A measurement that answers
// nothing (the stub DOM of the harness, a hidden frame) skips the
// flight entirely - the plain cross-fade remains.

// The flight duration in milliseconds; the cleanup timer rides the
// same number with headroom.
const buffsFlipMs = 320;

// flipBuffsView morphs the panel from the previous view state to the
// current one: apply toggles the view classes (and pins the new body
// height), the two captures around it bracket the icon positions.
function flipBuffsView(previous, apply) {
    const before = buffsCaptureRects(previous);
    apply();
    const after = buffsCaptureRects(BuffsPanel.view);
    buffsPlayFlip(before, after);
}

// buffsCaptureRects measures the icon boxes of one view state: the
// keyed entries of that view (the cells or the row icons), filtered
// to the boxes the widget really shows - inside the pinned body
// rectangle - and to the ones a measurement answers at all. The view
// argument reads the store; the DOM must already match it.
function buffsCaptureRects(view) {
    const rects = new Map();
    const body = document.getElementById("buffs-panel-body");
    if (!body) { return rects; }
    const store = view === "icons"
        ? BuffsPanel.cells : BuffsPanel.rows;
    const boxOf = (entry) => view === "icons"
        ? entry.item : entry.icon;
    const bodyRect = typeof body.getBoundingClientRect === "function"
        ? body.getBoundingClientRect() : null;
    const finalHeight = Number.parseFloat(body.style.height) || 0;
    for (const [id, entry] of store) {
        const box = boxOf(entry);
        if (!box || typeof box.getBoundingClientRect !== "function") {
            continue;
        }
        const rect = box.getBoundingClientRect();
        if (!(rect.width > 0) || !(rect.height > 0)) { continue; }
        // The destination shows only what fits the pinned body
        // height (the grid rows cap, the list scrolls); the source
        // answers with the same test against its own height.
        if (bodyRect && finalHeight > 0 &&
            (rect.top > bodyRect.top + finalHeight ||
                rect.bottom < bodyRect.top)) {
            continue;
        }
        rects.set(id, rect);
    }

    return rects;
}

// buffsPlayFly runs one icon flight: the destination box starts at
// the inverted delta of its old position (and the size ratio) and
// eases back to rest over the flip duration. The run token guards
// the cleanup against a newer flight that reused the same box.
function buffsPlayFly(box, from, to, token) {
    const dx = from.left - to.left;
    const dy = from.top - to.top;
    const scale = to.width > 0 ? from.width / to.width : 1;
    box.style.transition = "none";
    box.style.transform = "translate(" + dx + "px, " + dy + "px)" +
        (scale !== 1 ? " scale(" + scale + ")" : "");
    if (typeof window === "undefined" ||
        typeof window.requestAnimationFrame !== "function") {
        // No frame hook (the harness sandbox): drop the start pose,
        // the cross-fade carries the transition alone.
        box.style.transform = "";
        box.style.transition = "";

        return;
    }
    const settle = () => {
        if (BuffsPanel.flipRun !== token) { return; }
        box.style.transition = "transform " + buffsFlipMs +
            "ms cubic-bezier(0.33, 1, 0.5, 1)";
        box.style.transform = "";
    };
    window.requestAnimationFrame(() => window.requestAnimationFrame(
        settle));
    window.setTimeout(() => {
        if (BuffsPanel.flipRun !== token) { return; }
        box.style.transition = "";
        box.style.transform = "";
    }, buffsFlipMs + 180);
}

// buffsPlayFlip flies the shared buffs between the two captures: a
// buff measured on both sides takes the flight, the rest stay still.
// Each run bumps the flip run token, so the cleanup of an older
// interrupted flight never erases a newer one mid-air.
function buffsPlayFlip(before, after) {
    if (before.size === 0 || after.size === 0) { return; }
    const token = (BuffsPanel.flipRun || 0) + 1;
    BuffsPanel.flipRun = token;
    const store = BuffsPanel.view === "icons"
        ? BuffsPanel.cells : BuffsPanel.rows;
    const boxOf = (entry) => BuffsPanel.view === "icons"
        ? entry.item : entry.icon;
    for (const [id, entry] of store) {
        const from = before.get(id);
        const to = after.get(id);
        if (!from || !to) { continue; }
        const box = boxOf(entry);
        if (!box) { continue; }
        buffsPlayFly(box, from, to, token);
    }
}
