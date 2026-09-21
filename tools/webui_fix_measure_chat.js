// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Measures the vertical rhythm of the rendered chat rows: every row
// height must be a multiple of the 18px line height and the gap
// between consecutive rows must equal the previous row height (no
// margins, no drift). Returns a compact verdict string.
(() => {
  const rows = Array.from(document.querySelectorAll(".chat-line"));
  if (rows.length < 2) { return "ERR: only " + rows.length + " rows"; }
  const rects = rows.map((r) => r.getBoundingClientRect());
  const heights = rects.map((r) => Math.round(r.height * 100) / 100);
  const gaps = [];
  for (let i = 1; i < rects.length; i++) {
    gaps.push(Math.round((rects[i].top - rects[i - 1].top) * 100) / 100);
  }
  const onGrid = heights.every((h) => Math.abs(h / 18 - Math.round(h / 18)) < 0.05);
  const uniform = gaps.every((g, i) => Math.abs(g - heights[i]) < 0.05);
  return JSON.stringify({
    heights: heights, gaps: gaps,
    allMultiplesOf18: onGrid,
    gapEqualsPreviousHeight: uniform,
    verdict: onGrid && uniform ? "RHYTHM_OK" : "RHYTHM_BROKEN"
  });
})()
