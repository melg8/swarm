/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

"use strict";

// The bot statistics tab: the fleet overview (KPI cards, charts, the
// bots comparison table) and the per bot detail view (counters, the
// collected history, the phase distribution, the event timeline).
//
// The tab polls /api/stats and /api/stats/{id} every five seconds
// while it is visible and stops while another tab holds the screen.
// Everything renders through createElement + textContent (the XSS
// discipline of the web UI) and the charts draw on plain canvases -
// no framework, no bundler, no network dependency.

// Poll interval and the refresh pacing.
const STATS_POLL_MS = 5000;

// The state of the tab. Kept at the top level like App of app.js: the
// classic scripts share the bindings through the bare names.
const StatsTab = {
  active: false,
  timer: null,
  windowSec: 21600,
  fleet: null,
  botView: null,
  selectedBot: "",
  sortKey: "kills",
  sortDir: -1,
  lastRefreshAt: 0
};

// ---------- formatting helpers ----------

function statsFmtInt(value) {
  const n = Math.round(Number(value) || 0);
  return n.toLocaleString("en-US");
}

function statsFmtCompact(value) {
  const n = Number(value) || 0;
  const abs = Math.abs(n);
  if (abs >= 1e9) { return (n / 1e9).toFixed(1) + "B"; }
  if (abs >= 1e6) { return (n / 1e6).toFixed(1) + "M"; }
  if (abs >= 1e4) { return (n / 1e3).toFixed(1) + "k"; }
  return String(Math.round(n));
}

function statsFmtRate(value) {
  const n = Number(value) || 0;
  return n >= 100 ? Math.round(n).toString() : n.toFixed(1);
}

function statsFmtDuration(seconds) {
  let s = Math.max(0, Math.floor(Number(seconds) || 0));
  const days = Math.floor(s / 86400);
  s -= days * 86400;
  const hours = Math.floor(s / 3600);
  s -= hours * 3600;
  const minutes = Math.floor(s / 60);
  if (days > 0) { return days + "d " + hours + "h"; }
  if (hours > 0) { return hours + "h " + minutes + "m"; }
  if (minutes > 0) { return minutes + "m " + (s - minutes * 60) + "s"; }
  return s + "s";
}

function statsFmtAgo(seconds) {
  if (!Number(seconds)) { return "never"; }
  return statsFmtDuration(seconds) + " ago";
}

function statsFmtPct(value) {
  const n = Number(value) || 0;
  return n.toFixed(1) + "%";
}

function statsFmtClock(unixSec) {
  const d = new Date(unixSec * 1000);
  const pad = (v) => (v < 10 ? "0" + v : String(v));
  return pad(d.getHours()) + ":" + pad(d.getMinutes());
}

// statsFmtTick renders a millisecond duration with one decimal.
function statsFmtTick(ms) {
  const n = Number(ms) || 0;
  return n >= 100 ? Math.round(n).toString() : n.toFixed(1);
}

// ---------- palette ----------

// The chart colors resolve from the theme CSS variables of the root
// element; the fallbacks keep the harness (no getComputedStyle)
// rendering.
const STATS_COLORS = {
  blue: "--blue", green: "--green", red: "--red", gold: "--gold",
  violet: "--violet", gray: "--gray", accent: "--accent",
  text: "--text-dim", grid: "--grid", panel: "--bg-panel"
};

const STATS_COLOR_FALLBACK = {
  blue: "#0969da", green: "#1a7f37", red: "#cf222e", gold: "#9a6700",
  violet: "#8250df", gray: "#6e7781", accent: "#d97706",
  text: "#67707e", grid: "rgba(21, 34, 50, 0.10)", panel: "#ffffff"
};

function statsColor(name) {
  if (typeof getComputedStyle === "function") {
    try {
      const v = getComputedStyle(document.documentElement)
        .getPropertyValue(STATS_COLORS[name]).trim();
      if (v) { return v; }
    } catch (err) { /* fall through to the fallback */ }
  }
  return STATS_COLOR_FALLBACK[name];
}

// ---------- chart engine ----------

// drawStatsChart renders a time series chart onto a canvas element.
// series: [{color: "blue", points: [[unixSec, value], ...], fill: bool}]
// opts: {yLabel: format function, min: fixed y minimum, area: default fill}
function drawStatsChart(canvas, series, opts) {
  if (!canvas || !canvas.getContext) { return; }
  const ctx = canvas.getContext("2d");
  const dpr = (typeof devicePixelRatio === "number" && devicePixelRatio > 0)
    ? devicePixelRatio : 1;
  const width = Math.max(canvas.clientWidth || 300, 120);
  const height = Math.max(canvas.clientHeight || 120, 80);
  if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
    canvas.width = width * dpr;
    canvas.height = height * dpr;
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  const points = [];
  let minT = Infinity;
  let maxT = -Infinity;
  let minV = opts && opts.min !== undefined ? opts.min : Infinity;
  let maxV = -Infinity;
  for (const s of series) {
    for (const p of s.points) {
      points.push(p);
      if (p[0] < minT) { minT = p[0]; }
      if (p[0] > maxT) { maxT = p[0]; }
      if (p[1] < minV) { minV = p[1]; }
      if (p[1] > maxV) { maxV = p[1]; }
    }
  }
  if (!points.length || minT > maxT) {
    ctx.fillStyle = statsColor("text");
    ctx.font = "11px system-ui, sans-serif";
    ctx.textAlign = "center";
    ctx.fillText("no samples yet", width / 2, height / 2);
    return;
  }

  if (maxV === minV) { maxV = minV + 1; }
  const padV = (maxV - minV) * 0.08;
  maxV += padV;
  if (minV > 0) { minV = Math.max(0, minV - padV); }

  const padL = 42;
  const padR = 8;
  const padT = 8;
  const padB = 16;
  const plotW = Math.max(width - padL - padR, 10);
  const plotH = Math.max(height - padT - padB, 10);
  const spanT = Math.max(maxT - minT, 1);
  const x = (t) => padL + ((t - minT) / spanT) * plotW;
  const y = (v) => padT + plotH - ((v - minV) / (maxV - minV)) * plotH;

  // Grid: four horizontal lines with the y labels, three time ticks.
  ctx.strokeStyle = statsColor("grid");
  ctx.fillStyle = statsColor("text");
  ctx.font = "10px system-ui, sans-serif";
  ctx.textAlign = "right";
  ctx.lineWidth = 1;
  const fmtY = (opts && opts.yLabel) || statsFmtCompact;
  for (let i = 0; i <= 4; i++) {
    const v = minV + ((maxV - minV) * i) / 4;
    const yy = Math.round(y(v)) + 0.5;
    ctx.beginPath();
    ctx.moveTo(padL, yy);
    ctx.lineTo(width - padR, yy);
    ctx.stroke();
    ctx.fillText(fmtY(v), padL - 5, yy + 3);
  }
  ctx.textAlign = "center";
  for (let i = 0; i <= 3; i++) {
    const t = minT + (spanT * i) / 3;
    // The edge labels clamp inside the plot so the half of the text
    // never clips against the canvas border.
    const lx = Math.min(Math.max(x(t), padL + 14), width - padR - 14);
    ctx.fillText(statsFmtClock(t), lx, height - 4);
  }

  // The series lines with an optional translucent area fill.
  for (const s of series) {
    if (!s.points.length) { continue; }
    const fill = s.fill || (opts && opts.area);
    if (fill) {
      ctx.beginPath();
      ctx.moveTo(x(s.points[0][0]), padT + plotH);
      for (const p of s.points) { ctx.lineTo(x(p[0]), y(p[1])); }
      ctx.lineTo(x(s.points[s.points.length - 1][0]), padT + plotH);
      ctx.closePath();
      ctx.fillStyle = statsColor(s.color);
      ctx.save();
      ctx.globalAlpha = 0.12;
      ctx.fill();
      ctx.restore();
    }
    ctx.beginPath();
    for (let i = 0; i < s.points.length; i++) {
      const px = x(s.points[i][0]);
      const py = y(s.points[i][1]);
      if (i === 0) { ctx.moveTo(px, py); } else { ctx.lineTo(px, py); }
    }
    ctx.strokeStyle = statsColor(s.color);
    ctx.lineWidth = 1.6;
    ctx.stroke();
  }
}

// statsZipPairs converts the parallel arrays of a history payload
// into [t, v] pairs.
function statsZipPairs(at, values, transform) {
  const pairs = [];
  if (!Array.isArray(at) || !Array.isArray(values)) { return pairs; }
  const n = Math.min(at.length, values.length);
  for (let i = 0; i < n; i++) {
    let v = values[i];
    if (typeof transform === "function") { v = transform(v); }
    pairs.push([at[i], v]);
  }
  return pairs;
}

// ---------- KPI cards ----------

// statsKpiCard builds one KPI card element: the label, the value and
// the optional sub line.
function statsKpiCard(label, value, sub, tone) {
  const card = document.createElement("div");
  card.className = "stats-kpi" + (tone ? " stats-kpi-" + tone : "");
  const labelEl = document.createElement("span");
  labelEl.className = "stats-kpi-label";
  labelEl.textContent = label;
  const valueEl = document.createElement("b");
  valueEl.className = "stats-kpi-value";
  valueEl.textContent = value;
  card.appendChild(labelEl);
  card.appendChild(valueEl);
  if (sub !== undefined && sub !== null && sub !== "") {
    const subEl = document.createElement("span");
    subEl.className = "stats-kpi-sub";
    subEl.textContent = sub;
    card.appendChild(subEl);
  }
  return card;
}

// statsRenderKpis replaces the children of a container with cards.
function statsRenderKpis(containerId, cards) {
  const box = document.getElementById(containerId);
  if (!box) { return; }
  while (box.firstChild) { box.removeChild(box.firstChild); }
  for (const card of cards) { box.appendChild(card); }
}

// ---------- fleet view ----------

// statsRenderFleet renders the whole fleet overview from the payload
// of /api/stats.
function statsRenderFleet(payload) {
  statsRenderFleetKpis(payload);
  statsRenderFleetCharts(payload);
  statsRenderBotsTable(payload.bots || []);
  statsRenderBotOptions(payload.bots || []);
  statsUpdateClocks(payload);
}

// statsKdLabel formats the fleet kill to death label.
function statsKdLabel(kills, deaths) {
  const k = Number(kills) || 0;
  const d = Number(deaths) || 0;
  if (!k) { return "0"; }
  if (!d) { return String(k); }

  return statsFmtRate(k / d);
}

// statsRenderFleetKpis renders the fleet KPI cards.
function statsRenderFleetKpis(payload) {
  const fleet = payload.fleet || {};
  const process = payload.process || {};
  const collection = payload.collectionSec || 0;
  const cards = [
    statsKpiCard("bots online",
      (fleet.online || 0) + " / " + (fleet.registered || 0),
      "registered fleet"),
    statsKpiCard("kills", statsFmtInt(fleet.kills),
      fleet.killsPerHour ? statsFmtRate(fleet.killsPerHour) + " / hour" : "—"),
    statsKpiCard("deaths", statsFmtInt(fleet.deaths),
      "K/D " + statsKdLabel(fleet.kills, fleet.deaths)),
    statsKpiCard("rejoins", statsFmtInt(fleet.rejoins || 0),
      "relogins of the run"),
    statsKpiCard("experience gained", statsFmtCompact(fleet.expGained || 0),
      "net of the fleet"),
    statsKpiCard("avg tick", statsFmtTick(fleet.avgTickMs) + " ms",
      "hunt loop cadence"),
    statsKpiCard("memory", statsFmtRate(process.heapMB) + " MB",
      "sys " + statsFmtRate(process.sysMB) + " MB"),
    statsKpiCard("goroutines", statsFmtInt(process.goroutines || 0),
      "GC " + statsFmtInt(process.numGC || 0) +
        ", pause " + statsFmtTick(process.lastGCPauseMs) + " ms"),
    statsKpiCard("packet rate", statsFmtRate(fleet.packetRate) + " /s",
      "fleet total"),
    statsKpiCard("collecting", statsFmtDuration(collection),
      "history window " + (payload.samplePeriodSec || 15) + " s samples"),
    statsKpiCard("hit rate", fleet.hitRate
      ? statsFmtPct(fleet.hitRate * 100) : "—",
      "landed swings / swings")
  ];
  statsRenderKpis("stats-kpis", cards);
}

// statsRenderFleetCharts draws the fleet history charts.
function statsRenderFleetCharts(payload) {
  const h = payload.history || {};
  const at = h.at || [];
  drawStatsChart(document.getElementById("chart-fleet-online"), [
    { color: "gray", points: statsZipPairs(at, h.registered) },
    { color: "green", points: statsZipPairs(at, h.online) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-kd"), [
    { color: "green", points: statsZipPairs(at, h.kills) },
    { color: "red", points: statsZipPairs(at, h.deaths) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-rate"), [
    { color: "green", points: statsZipPairs(at, h.killsPerMin) },
    { color: "red", points: statsZipPairs(at, h.deathsPerMin) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-exp"), [
    { color: "blue", points: statsZipPairs(at, h.expGained), fill: true }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-tick"), [
    { color: "violet", points: statsZipPairs(at, h.avgTickMs) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-mem"), [
    { color: "accent", points: statsZipPairs(at, h.heapMB), fill: true },
    { color: "gray", points: statsZipPairs(at, h.sysMB) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-traffic"), [
    { color: "blue", points: statsZipPairs(at, h.packetRate) }
  ]);
  drawStatsChart(document.getElementById("chart-fleet-goroutines"), [
    { color: "gold", points: statsZipPairs(at, h.goroutines) }
  ]);
}

// statsUpdateClocks refreshes the toolbar clocks of the tab.
function statsUpdateClocks(payload) {
  const updated = document.getElementById("stats-updated");
  if (updated) { updated.textContent = "updated " +
    statsFmtClock(Date.now() / 1000); }
  const collection = document.getElementById("stats-collection");
  if (collection) {
    collection.textContent = "history: " +
      statsFmtDuration(payload.collectionSec || 0);
  }
}

// ---------- bots table ----------

// The sortable columns of the bots table: the key, the label, the
// value extractor and the alignment.
const STATS_TABLE_COLUMNS = [
  { key: "id", label: "bot", value: (b) => b.id },
  { key: "status", label: "status", value: (b) => b.status },
  { key: "phase", label: "phase", value: (b) => b.phase || "—" },
  { key: "level", label: "lvl", value: (b) => b.level, num: true },
  { key: "kills", label: "kills", value: (b) => b.kills, num: true },
  { key: "deaths", label: "deaths", value: (b) => b.deaths, num: true },
  { key: "kd", label: "K/D", value: (b) => b.kd, num: true },
  { key: "killsPerHour", label: "kills/h", value: (b) => b.killsPerHour,
    num: true },
  { key: "expGained", label: "exp+", value: (b) => b.expGained, num: true },
  { key: "rejoins", label: "rejoins", value: (b) => b.rejoins, num: true },
  { key: "hitRate", label: "hit%", value: (b) =>
    b.swingsMade ? b.hitRate * 100 : null, num: true },
  { key: "damageTaken", label: "dmg taken", value: (b) =>
    b.damageTaken ? Math.round(b.damageTaken) : 0, num: true },
  { key: "avgTickMs", label: "tick ms", value: (b) =>
    b.avgTickMs ? Math.round(b.avgTickMs * 10) / 10 : null, num: true },
  { key: "uptimeSec", label: "uptime", value: (b) => b.uptimeSec, num: true }
];

// statsRenderBotsTable rebuilds the comparison table of the bots.
function statsRenderBotsTable(bots) {
  const table = document.getElementById("stats-bots-table");
  if (!table) { return; }
  while (table.firstChild) { table.removeChild(table.firstChild); }

  const thead = document.createElement("thead");
  const headRow = document.createElement("tr");
  for (const col of STATS_TABLE_COLUMNS) {
    const th = document.createElement("th");
    th.textContent = col.label;
    if (col.num) { th.className = "stats-num"; }
    if (col.key === StatsTab.sortKey) {
      th.classList.add(StatsTab.sortDir < 0 ? "sorted-desc" : "sorted-asc");
    }
    th.addEventListener("click", () => {
      if (StatsTab.sortKey === col.key) {
        StatsTab.sortDir = -StatsTab.sortDir;
      } else {
        StatsTab.sortKey = col.key;
        StatsTab.sortDir = col.num ? -1 : 1;
      }
      statsRenderBotsTable(StatsTab.fleet ? StatsTab.fleet.bots || [] : []);
    });
    headRow.appendChild(th);
  }
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = document.createElement("tbody");
  const sorted = bots.slice().sort((a, b) => {
    const col = STATS_TABLE_COLUMNS.find((c) => c.key === StatsTab.sortKey);
    if (!col) { return 0; }
    const va = col.value(a);
    const vb = col.value(b);
    if (typeof va === "string" || typeof vb === "string") {
      return String(va).localeCompare(String(vb)) * StatsTab.sortDir;
    }
    const na = va === null || va === undefined ? -Infinity : Number(va);
    const nb = vb === null || vb === undefined ? -Infinity : Number(vb);
    return (na - nb) * StatsTab.sortDir;
  });
  for (const bot of sorted) {
    tbody.appendChild(statsBotRow(bot));
  }
  table.appendChild(tbody);
}

// statsBotRow builds one table row of a bot; the row click opens the
// detail view.
function statsBotRow(bot) {
  const row = document.createElement("tr");
  if (bot.id === StatsTab.selectedBot) { row.className = "selected"; }
  for (const col of STATS_TABLE_COLUMNS) {
    const td = document.createElement("td");
    if (col.num) { td.className = "stats-num"; }
    let v = col.value(bot);
    if (v === null || v === undefined) {
      td.textContent = "—";
    } else if (col.key === "uptimeSec") {
      td.textContent = statsFmtDuration(v);
    } else if (col.key === "expGained") {
      td.textContent = statsFmtCompact(v);
    } else if (col.key === "damageTaken") {
      td.textContent = statsFmtCompact(v);
    } else if (col.key === "kd") {
      td.textContent = statsFmtRate(v);
    } else if (col.key === "killsPerHour") {
      td.textContent = statsFmtRate(v);
    } else if (col.key === "hitRate") {
      td.textContent = statsFmtRate(v);
    } else if (col.key === "avgTickMs") {
      td.textContent = statsFmtTick(v);
    } else {
      td.textContent = String(v);
    }
    row.appendChild(td);
  }
  row.addEventListener("click", () => { statsSelectBot(bot.id); });
  return row;
}

// ---------- bot detail view ----------

// statsRenderBotOptions fills the bot selector with the fleet bots.
function statsRenderBotOptions(bots) {
  const select = document.getElementById("stats-bot-select");
  if (!select) { return; }
  const current = StatsTab.selectedBot;
  while (select.firstChild) { select.removeChild(select.firstChild); }
  const placeholder = document.createElement("option");
  placeholder.value = "";
  placeholder.textContent = "select a bot…";
  select.appendChild(placeholder);
  for (const bot of bots) {
    const option = document.createElement("option");
    option.value = bot.id;
    option.textContent = bot.name ? bot.id + " · " + bot.name : bot.id;
    if (bot.id === current) { option.selected = true; }
    select.appendChild(option);
  }
}

// statsSelectBot opens the detail view of a bot.
function statsSelectBot(id) {
  StatsTab.selectedBot = id;
  const panel = document.getElementById("stats-bot");
  if (panel) { panel.classList.remove("hidden"); }
  const select = document.getElementById("stats-bot-select");
  if (select) { select.value = id; }
  if (StatsTab.fleet) {
    statsRenderBotsTable(StatsTab.fleet.bots || []);
  }
  statsRefreshBot();
}

// statsCloseBot hides the detail view.
function statsCloseBot() {
  StatsTab.selectedBot = "";
  StatsTab.botView = null;
  const panel = document.getElementById("stats-bot");
  if (panel) { panel.classList.add("hidden"); }
  if (StatsTab.fleet) {
    statsRenderBotsTable(StatsTab.fleet.bots || []);
  }
}

// statsRenderBotDetail renders the per bot view from the payload of
// /api/stats/{id}.
function statsRenderBotDetail(payload) {
  const head = document.getElementById("stats-bot-name");
  if (head) {
    head.textContent = payload.name
      ? payload.id + " · " + payload.name : payload.id;
  }
  statsRenderBotBadges(payload);

  const cards = [
    statsKpiCard("level", String(payload.level || "—"),
      payload.levelGained
        ? (payload.levelGained > 0 ? "+" : "") + payload.levelGained
          + " in window" : null),
    statsKpiCard("kills", statsFmtInt(payload.kills),
      payload.killsPerHour ? statsFmtRate(payload.killsPerHour) + " / hour" : "—",
      "green"),
    statsKpiCard("deaths", statsFmtInt(payload.deaths),
      payload.deathsPerHour ? statsFmtRate(payload.deathsPerHour) + " / hour"
        : "deathless", payload.deaths ? "red" : ""),
    statsKpiCard("K/D", statsFmtRate(payload.kd), "kills per death"),
    statsKpiCard("experience", statsFmtCompact(payload.expGained),
      "net gained, " + statsFmtPct(payload.expPercent) + " of level"),
    statsKpiCard("rejoins", statsFmtInt(payload.rejoins),
      payload.sessions + " sessions"),
    statsKpiCard("hit rate", payload.swingsMade
      ? statsFmtPct(payload.hitRate * 100) : "—",
      payload.swingsMade
        ? statsFmtInt(payload.swingsLanded) + " of "
          + statsFmtInt(payload.swingsMade) + " swings" : "no swings yet"),
    statsKpiCard("swings taken", statsFmtInt(payload.swingsTaken),
      "blows aimed at the bot"),
    statsKpiCard("damage taken", statsFmtCompact(payload.damageTaken),
      "HP lost in total"),
    statsKpiCard("adena", statsFmtCompact(payload.adena), "wallet"),
    statsKpiCard("avg tick", statsFmtTick(payload.avgTickMs) + " ms",
      "worst " + statsFmtTick(payload.maxTickMs) + " ms (1 min)"),
    statsKpiCard("packet rate", statsFmtRate(payload.packetRate) + " /s",
      "received"),
    statsKpiCard("uptime", statsFmtDuration(payload.uptimeSec),
      "since first login"),
    statsKpiCard("last kill", statsFmtAgo(payload.lastKillAgoSec),
      "last death " + statsFmtAgo(payload.lastDeathAgoSec))
  ];
  statsRenderKpis("stats-bot-kpis", cards);

  const h = payload.history || {};
  const at = h.at || [];
  drawStatsChart(document.getElementById("chart-bot-exp"), [
    { color: "blue", points: statsZipPairs(at, h.expGained), fill: true }
  ]);
  drawStatsChart(document.getElementById("chart-bot-kd"), [
    { color: "green", points: statsZipPairs(at, h.kills) },
    { color: "red", points: statsZipPairs(at, h.deaths) }
  ]);
  drawStatsChart(document.getElementById("chart-bot-hp"), [
    { color: "red", points: statsZipPairs(at, h.hpPercent) }
  ], { min: 0 });
  drawStatsChart(document.getElementById("chart-bot-adena"), [
    { color: "gold", points: statsZipPairs(at, h.adena) }
  ]);
  drawStatsChart(document.getElementById("chart-bot-tick"), [
    { color: "violet", points: statsZipPairs(at, h.avgTickMs) }
  ]);
  drawStatsChart(document.getElementById("chart-bot-traffic"), [
    { color: "blue", points: statsZipPairs(at, h.packetRate) }
  ]);

  statsDrawPhaseTimeline(at, h.phase || []);
  statsRenderPhaseShares(payload.phases || []);
  statsRenderEvents(payload.events || []);
}

// statsRenderBotBadges renders the status and phase badges of the
// detail head.
function statsRenderBotBadges(payload) {
  const box = document.getElementById("stats-bot-badges");
  if (!box) { return; }
  while (box.firstChild) { box.removeChild(box.firstChild); }
  const status = document.createElement("span");
  status.className = "stats-badge stats-badge-" +
    (payload.online ? "online" : "offline");
  status.textContent = payload.status || "unknown";
  box.appendChild(status);
  if (payload.phase) {
    const phase = document.createElement("span");
    phase.className = "stats-badge stats-badge-phase";
    phase.textContent = payload.phase;
    box.appendChild(phase);
  }
  if (payload.inCombat) {
    const combat = document.createElement("span");
    combat.className = "stats-badge stats-badge-combat";
    combat.textContent = "combat";
    box.appendChild(combat);
  }
}

// ---------- phase timeline and distribution ----------

// The phase colors of the timeline strip.
const STATS_PHASE_COLORS = {
  engage: "green", loot: "gold", townWalk: "blue", townSell: "violet",
  townReturn: "blue", delevel: "gray", user: "accent", idle: "gray"
};

// statsDrawPhaseTimeline draws the horizontal phase band of the bot
// history.
function statsDrawPhaseTimeline(at, phases) {
  const canvas = document.getElementById("bot-phase-strip");
  if (!canvas || !canvas.getContext) { return; }
  const ctx = canvas.getContext("2d");
  const dpr = (typeof devicePixelRatio === "number" && devicePixelRatio > 0)
    ? devicePixelRatio : 1;
  const width = Math.max(canvas.clientWidth || 600, 120);
  const height = Math.max(canvas.clientHeight || 26, 12);
  if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
    canvas.width = width * dpr;
    canvas.height = height * dpr;
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);
  if (!at.length) {
    ctx.fillStyle = statsColor("text");
    ctx.font = "11px system-ui, sans-serif";
    ctx.textAlign = "center";
    ctx.fillText("no samples yet", width / 2, height / 2 + 4);
    return;
  }
  const minT = at[0];
  const spanT = Math.max(at[at.length - 1] - minT, 1);
  const top = 6;
  const bandH = height - top - 6;
  for (let i = 0; i < at.length; i++) {
    const x0 = ((at[i] - minT) / spanT) * (width - 2) + 1;
    const x1 = i + 1 < at.length
      ? ((at[i + 1] - minT) / spanT) * (width - 2) + 1
      : width - 1;
    const phase = phases[i] || "";
    ctx.fillStyle = statsColor(STATS_PHASE_COLORS[phase] || "gray");
    ctx.fillRect(x0, top, Math.max(x1 - x0, 1), bandH);
  }
  ctx.fillStyle = statsColor("text");
  ctx.font = "10px system-ui, sans-serif";
  ctx.textAlign = "left";
  ctx.fillText(statsFmtClock(minT), 1, height - 1);
  ctx.textAlign = "right";
  ctx.fillText(statsFmtClock(at[at.length - 1]), width - 1, height - 1);
}

// statsRenderPhaseShares renders the phase distribution bars.
function statsRenderPhaseShares(phases) {
  const box = document.getElementById("stats-phase-shares");
  if (!box) { return; }
  while (box.firstChild) { box.removeChild(box.firstChild); }
  const total = phases.reduce((sum, p) => sum + (p.seconds || 0), 0);
  if (!total) {
    const empty = document.createElement("div");
    empty.className = "stats-empty";
    empty.textContent = "no phases observed yet";
    box.appendChild(empty);
    return;
  }
  const sorted = phases.slice().sort((a, b) => (b.seconds || 0) - (a.seconds || 0));
  for (const share of sorted) {
    const row = document.createElement("div");
    row.className = "stats-phase-row";
    const label = document.createElement("span");
    label.className = "stats-phase-label";
    label.textContent = share.phase || "connecting";
    const bar = document.createElement("div");
    bar.className = "stats-phase-bar";
    const fill = document.createElement("div");
    fill.className = "stats-phase-fill phase-"
      + (STATS_PHASE_COLORS[share.phase] || "gray");
    fill.style.width = Math.max((share.seconds / total) * 100, 0.5) + "%";
    bar.appendChild(fill);
    const value = document.createElement("span");
    value.className = "stats-phase-value";
    value.textContent = statsFmtDuration(share.seconds) + " · "
      + statsFmtPct((share.seconds / total) * 100);
    row.appendChild(label);
    row.appendChild(bar);
    row.appendChild(value);
    box.appendChild(row);
  }
}

// ---------- events ----------

// The event renderers: the label text and the tone of every kind.
const STATS_EVENT_KINDS = {
  kill: (e) => ({ text: (e.value > 1 ? e.value + " kills" : "a kill"),
    tone: "green" }),
  death: (e) => ({ text: "died", tone: "red" }),
  relogin: (e) => ({ text: (e.value > 1 ? e.value + " relogins"
    : "relogged in"), tone: "violet" }),
  level: (e) => ({ text: "reached level " + e.value, tone: "gold" }),
  online: () => ({ text: "entered the world", tone: "green" }),
  offline: () => ({ text: "left the world", tone: "gray" })
};

// statsRenderEvents renders the recent events of the bot.
function statsRenderEvents(events) {
  const box = document.getElementById("stats-events");
  if (!box) { return; }
  while (box.firstChild) { box.removeChild(box.firstChild); }
  if (!events.length) {
    const empty = document.createElement("div");
    empty.className = "stats-empty";
    empty.textContent = "no events recorded yet";
    box.appendChild(empty);
    return;
  }
  const recent = events.slice(-40).reverse();
  const nowSec = Date.now() / 1000;
  for (const event of recent) {
    const render = STATS_EVENT_KINDS[event.kind] || null;
    const row = document.createElement("div");
    row.className = "stats-event";
    const time = document.createElement("span");
    time.className = "stats-event-time";
    time.textContent = statsFmtClock(event.atMs / 1000);
    const chip = document.createElement("span");
    chip.className = "stats-event-chip stats-event-"
      + (render ? render(event).tone : "gray");
    chip.textContent = event.kind;
    const text = document.createElement("span");
    text.className = "stats-event-text";
    text.textContent = render ? render(event).text : event.kind;
    const ago = document.createElement("span");
    ago.className = "stats-event-ago";
    ago.textContent = statsFmtAgo(Math.max(0, nowSec - event.atMs / 1000));
    row.appendChild(time);
    row.appendChild(chip);
    row.appendChild(text);
    row.appendChild(ago);
    box.appendChild(row);
  }
}

// ---------- fetch and lifecycle ----------

// statsFetchJson fetches a JSON document, null on any error.
async function statsFetchJson(path) {
  try {
    const response = await fetch(path);
    if (!response.ok) { return null; }
    return await response.json();
  } catch (err) {
    return null;
  }
}

// statsRefresh fetches the fleet view and renders it.
async function statsRefresh() {
  const payload = await statsFetchJson(
    "/api/stats?window=" + StatsTab.windowSec);
  if (!payload) {
    const updated = document.getElementById("stats-updated");
    if (updated) { updated.textContent = "unreachable"; }
    return;
  }
  StatsTab.fleet = payload;
  statsRenderFleet(payload);
}

// statsRefreshBot fetches and renders the selected bot view.
async function statsRefreshBot() {
  if (!StatsTab.selectedBot) { return; }
  const payload = await statsFetchJson("/api/stats/"
    + encodeURIComponent(StatsTab.selectedBot)
    + "?window=" + StatsTab.windowSec);
  if (!payload || payload.id !== StatsTab.selectedBot) { return; }
  StatsTab.botView = payload;
  statsRenderBotDetail(payload);
}

// statsSetWindow switches the history window and refreshes.
function statsSetWindow(seconds) {
  StatsTab.windowSec = seconds;
  statsRefresh();
  statsRefreshBot();
}

// StatsTab.init binds the controls of the tab. No polling until the
// tab becomes visible.
StatsTab.init = function () {
  const select = document.getElementById("stats-window");
  if (select) {
    select.value = String(StatsTab.windowSec);
    select.addEventListener("change", () => {
      const value = Number(select.value);
      if (value === 0 || (value >= 60 && value <= 30 * 86400)) {
        statsSetWindow(value);
      }
    });
  }
  const botSelect = document.getElementById("stats-bot-select");
  if (botSelect) {
    botSelect.addEventListener("change", () => {
      if (botSelect.value) {
        statsSelectBot(botSelect.value);
      } else {
        statsCloseBot();
      }
    });
  }
  const close = document.getElementById("stats-bot-close");
  if (close) {
    close.addEventListener("click", () => { statsCloseBot(); });
  }
};

// StatsTab.setActive starts or stops the polling of the tab.
StatsTab.setActive = function (on) {
  if (on === StatsTab.active) { return; }
  StatsTab.active = on;
  if (StatsTab.timer) {
    clearInterval(StatsTab.timer);
    StatsTab.timer = null;
  }
  if (on) {
    statsRefresh();
    statsRefreshBot();
    StatsTab.timer = setInterval(() => {
      statsRefresh();
      statsRefreshBot();
    }, STATS_POLL_MS);
  }
};
