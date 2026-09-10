#!/usr/bin/env python3
"""Interactive visualization of the hunting spot registry.

Renders the spot anchored hunting system (see
docs/hunting_system_redesign.md and tools/generate_hunt_spots.py) as
one self contained HTML file: the elven map tiles under the spot
circles (the SIZES of the grounds), the level bands as the difficulty
colors, the composition and the respawn windows in the hover tooltips,
and - when a live state dump or the simulation runs - the RUNTIME
economy: the active spot, the per spot death heat (the red fill), the
measured adena per minute, the next respawn ETA countdowns and the
kill centroids.

Usage:
  python3 tools/visualize_hunt_spots.py [--out FILE]
       [--state STATE_JSON] [--simulate MINUTES] [--level N] [--seed N]

  --state    a bot state dump of the webserver (the same JSON the
             "state dump" button produces): the spot views of the
             snapshot merge onto the registry circles
  --simulate run a session simulation of MINUTES (default 30) over the
             registry instead of a live dump - the kill/respawn model
             of the spot policy drives the counters, so the death
             heat, the timers and the income rate demonstrate without
             the live server. Clearly labeled in the header.

The map tiles come from the web UI pack
(internal/swarm/webserver/web/maps/0, one 1024px tile per 32768 world
units, BX = x // 32768 + 20, BY = y // 32768 + 18).
"""

import argparse
import base64
import json
import math
import os
import random
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SPOTS_JSON = os.path.join(REPO, "docs/hunt_analysis/spots_elven.json")
TILES_DIR = os.path.join(REPO, "internal/swarm/webserver/web/maps/0")
DEFAULT_OUT = "/home/z/my-project/download/hunt_spots_map.html"

# The world window of the visualization: the elven spot registry spans
# x [1708, 54254], y [34387, 82406], the four tiles 20_19..21_20 cover
# x [0, 65536], y [32768, 98304] - draw that whole block, one 1024px
# tile per 32768 world units (the Mobius World.TILE_SIZE).
TILE = 32768
TILES = [("20", "19"), ("20", "20"), ("21", "19"), ("21", "20")]
VIEW = {"x": 0, "y": 32768, "w": 65536, "h": 65536}
VILLAGE = (46112, 41500)

# The difficulty palette of the level bands (the game difficulty
# gradient: the green grounds feed a fresh character, the red ones
# outgun it).
BAND_COLORS = [
    (1, 4, "#188038"),    # green
    (5, 8, "#7cb342"),    # light green
    (9, 12, "#f9ab00"),   # yellow
    (13, 16, "#e37400"),  # orange
    (17, 99, "#d93025"),  # red
]


def band_color(level):
    for low, high, color in BAND_COLORS:
        if low <= level <= high:
            return color
    return "#5f6368"


def go_const(name, default):
    """Read a decision constant mirror (documentation purpose only)."""
    return default


# ---------------------------------------------------------------------------
# The session simulation: the kill/respawn model of the spot policy.
# ---------------------------------------------------------------------------

# The mirrors of the hunt constants (see hunt/spot_policy.go and
# hunt/spot_metrics.go): the simulation runs the same economy so the
# runtime view demonstrates what the live bot would show.
SIM_KILL_CAPACITY = 12.0        # kills per minute of a solo farmer
SIM_ADENA_BASE = 18.0           # adena per kill of the bootstrap prior
SIM_ADENA_PER_LEVEL = 6.5
SIM_DEATH_RATE = 3.0           # demo-tuned deaths per hour on a fully
                                # aggressive ground (the live research
                                # measured the 16-19 band deaths of an
                                # undergeared farmer at this order)
SIM_METRICS_TRUST_MIN = 5.0     # minutes before the measured rate wins
SIM_LEVEL_PERIOD = 12.0 * 60.0  # the simulated character levels up


def aggressive_share(spot):
    """The static danger proxy of the simulation: the python side has
    no npc aggression dictionary, the elven research measured the
    aggressive share of the spawn mass growing with the band (0 in the
    low bands, 86 percent in the 16-19 band of the Lireni grounds)."""
    if spot["max_level"] <= 8:
        return 0.0
    if spot["max_level"] <= 14:
        return 0.35

    return 0.86


def simulate(spots, minutes, level, seed):
    """A faithful-enough session over the spot registry.

    The simulated hunter picks the best scoring eligible spot (window
    mass, respawn turnover, proximity), kills at the capacity or the
    supply rate whichever is lower, the mobs respawn on their windows,
    the adena accrues by the level priced prior, the deaths heat the
    grounds (the aggro share drives the rate) and the starved or
    outscored grounds switch. The output mirrors the state dump
    (character + huntingZones) so the visualizer treats it like the
    real thing.
    """
    rng = random.Random(seed)
    metrics = [{
        "kills": 0, "adena": 0, "activeMin": 0.0, "deaths": 0,
        "deathHeat": 0.0, "lastDeathAt": -1e9, "visitAdena": 0,
        "visitMin": 0.0, "visitKills": 0, "killX": 0, "killY": 0,
        "killKnown": False, "occupancy": 0,
    } for _ in spots]
    kills = []  # (spotIndex, respawnAtSec)
    picked = -1
    entered = 0.0
    empty_since = None
    pos = [VILLAGE[0], VILLAGE[1]]
    horizon = minutes * 60.0

    def window_mass(i, at_level):
        """The white-green supply [max(1, at_level-5), at_level]."""
        return sum(m2["count"] for m2 in spots[i]["mobs"]
                   if max(1, at_level - 5) <= m2["level"] <= at_level)

    def eligible(i, at_level):
        low = max(1, at_level - 8)
        for m2 in spots[i]["mobs"]:
            if low <= m2["level"] <= at_level + 2:
                return True
        return False

    def score(i, t):
        m = metrics[i]
        window = window_mass(i, level)
        if window <= 0:
            return 0.0
        resp_mid = (spots[i]["respawn_min"] + spots[i]["respawn_max"]) / 2.0
        supply = window * 60.0 / resp_mid
        kpm = min(supply, SIM_KILL_CAPACITY)
        wlevel = sum(m2["level"] * m2["count"] for m2 in spots[i]["mobs"]
                     if max(1, level - 5) <= m2["level"] <= level) / max(1.0, window)
        value = kpm * (SIM_ADENA_BASE + SIM_ADENA_PER_LEVEL * wlevel)
        if m["activeMin"] >= SIM_METRICS_TRUST_MIN and m["adena"] > 0:
            value = m["adena"] / m["activeMin"]
        heat = m["deaths"] * 0.5 ** (max(0.0, t / 60.0 - m["lastDeathAt"]) / 30.0)
        safety = 1.0 / (1.0 + 2.0 * heat)
        dist = math.hypot(spots[i]["anchor_x"] - pos[0],
                          spots[i]["anchor_y"] - pos[1])
        prox = 1.0 / (1.0 + dist / 8000.0)
        return value * safety * prox

    # The window population of the current ground: the mobs of the
    # white-green window the hunter actually kills (the rest of the
    # ground pays no adena and never enters a fight). The level growth
    # empties it as the mobs gray out - the income drops, the economy
    # moves the hunter on.
    walive = 0.0
    t = 0.0
    start_level = level
    last_level = -1
    while t < horizon:
        t += 1.0
        # The character grows through the session: the window shifts,
        # new grounds open, the economy re-picks.
        level = start_level + int(t / SIM_LEVEL_PERIOD)
        if picked < 0:
            best, best_score = -1, 0.0
            for i in range(len(spots)):
                if not eligible(i, level):
                    continue
                if (s := score(i, t)) > best_score:
                    best, best_score = i, s
            if best < 0:
                break
            picked = best
            entered = t
            empty_since = None
            walive = float(window_mass(picked, level))
            pos[0] = (pos[0] + spots[picked]["anchor_x"]) / 2.0
            pos[1] = (pos[1] + spots[picked]["anchor_y"]) / 2.0
        # The walk to the ground costs the distance at run speed 130.
        dist = math.hypot(spots[picked]["anchor_x"] - pos[0],
                          spots[picked]["anchor_y"] - pos[1])
        if dist > 600:
            frac = min(1.0, 130.0 / dist)
            pos[0] += (spots[picked]["anchor_x"] - pos[0]) * frac
            pos[1] += (spots[picked]["anchor_y"] - pos[1]) * frac
            continue
        m = metrics[picked]
        m["activeMin"] += 1.0 / 60.0
        # The level growth shifts the window of the ground: the grayed
        # out mobs leave the window population.
        if level != last_level:
            last_level = level
            walive = min(walive, float(window_mass(picked, level)))
        # The respawns land.
        fresh = [k for k in kills if k[1] <= t and k[0] == picked]
        for _ in fresh:
            walive = min(walive + 1.0, float(window_mass(picked, level)))
        kills = [k for k in kills if not (k[1] <= t and k[0] == picked)]
        # The kills at the capacity/supply rate of the window.
        resp_mid = (spots[picked]["respawn_min"] +
                    spots[picked]["respawn_max"]) / 2.0
        kpm = min(SIM_KILL_CAPACITY, max(0.0, walive * 60.0 / resp_mid))
        take = min(walive, kpm / 60.0)
        if take > 0:
            walive -= take
            m["kills"] += take
            m["visitKills"] += take
            wlevel = sum(m2["level"] * m2["count"] for m2 in spots[picked]["mobs"]
                         if max(1, level - 5) <= m2["level"] <= level)
            wcount = max(1.0, sum(m2["count"] for m2 in spots[picked]["mobs"]
                                  if max(1, level - 5) <= m2["level"] <= level))
            adena_kill = SIM_ADENA_BASE + SIM_ADENA_PER_LEVEL * (wlevel / wcount)
            gain = take * adena_kill * rng.uniform(0.8, 1.2)
            m["adena"] += gain
            m["visitAdena"] += gain
            # The kills drift the position over the ground (the corpse
            # positions): the kill centroid follows.
            kx = spots[picked]["anchor_x"] + rng.uniform(
                -0.5, 0.5) * spots[picked]["radius"]
            ky = spots[picked]["anchor_y"] + rng.uniform(
                -0.5, 0.5) * spots[picked]["radius"]
            pos[0], pos[1] = kx, ky
            if not m["killKnown"]:
                m["killX"], m["killY"], m["killKnown"] = kx, ky, True
            else:
                m["killX"] = m["killX"] * 0.8 + kx * 0.2
                m["killY"] = m["killY"] * 0.8 + ky * 0.2
            mid = (spots[picked]["respawn_min"] +
                   spots[picked]["respawn_max"]) / 2.0
            kills.append((picked, t + rng.uniform(mid * 0.7, mid * 1.3)))
        # The deaths on the aggressive grounds (the band proxy of the
        # static danger share).
        aggro = aggressive_share(spots[picked])
        death_p = SIM_DEATH_RATE * aggro / 3600.0
        if rng.random() < death_p:
            m["deaths"] += 1
            m["deathHeat"] = m["deaths"]
            m["lastDeathAt"] = t / 60.0
            # The village restart and the re-pick of an easier ground.
            pos[0], pos[1] = VILLAGE
            best, best_score = picked, score(picked, t)
            for i in range(len(spots)):
                if i == picked or not eligible(i, level):
                    continue
                if (s := score(i, t)) > best_score:
                    best, best_score = i, s
            if best != picked:
                picked = best
                entered = t
                empty_since = None
                walive = float(window_mass(picked, level))
            continue
        # The level growth re-picks the economy: a ground whose window
        # thins as the character grows loses to a richer one (the
        # hysteresis lite of the simulation).
        if picked >= 0 and t % 60.0 < 1.0:
            current_score = score(picked, t)
            best, best_score = -1, 0.0
            for i in range(len(spots)):
                if i == picked or not eligible(i, level):
                    continue
                if (s := score(i, t)) > best_score:
                    best, best_score = i, s
            if best >= 0 and best_score > current_score * 1.25 and \
                    t - entered > 300:
                picked = best
                entered = t
                empty_since = None
                walive = float(window_mass(picked, level))
        # The starvation economy: an empty ground with no pending
        # respawn switches after the patience window.
        pending = [k for k in kills if k[0] == picked and k[1] > t]
        if walive <= 0.05 and not pending:
            if empty_since is None:
                empty_since = t
            elif (t - empty_since > 60 and t - entered > 90):
                best, best_score = -1, 0.0
                for i in range(len(spots)):
                    if i == picked or not eligible(i, level):
                        continue
                    if (s := score(i, t)) > best_score:
                        best, best_score = i, s
                if best >= 0:
                    picked = best
                    entered = t
                    empty_since = None
                    walive = float(window_mass(picked, level))
        else:
            empty_since = None

    # The view payload of the simulation (the state dump shape).
    views = []
    for i, spot in enumerate(spots):
        m = metrics[i]
        eta = -1
        future = [k for k in kills if k[0] == i and k[1] > t]
        if future:
            eta = int(min(f[1] for f in future) - t)
        heat = m["deaths"] * 0.5 ** (
            max(0.0, t / 60.0 - m["lastDeathAt"]) / 30.0)
        views.append({
            "id": spot["id"], "active": i == picked,
            "deaths": m["deaths"], "deathHeat": heat,
            "adenaPerMin": (m["adena"] / m["activeMin"]
                            if m["activeMin"] > 0.5 else 0.0),
            "nextRespawnSec": eta, "occupancy": 1 if i == picked else 0,
            "killX": int(m["killX"]) if m["killKnown"] else 0,
            "killY": int(m["killY"]) if m["killKnown"] else 0,
        })
    character = {
        "name": "simulated hunter", "level": level,
        "x": int(pos[0]), "y": int(pos[1]),
    }

    return {"character": character, "huntingZones": views}


# ---------------------------------------------------------------------------
# The state dump merge.
# ---------------------------------------------------------------------------

def merge_state(spots, state):
    """Merge the runtime spot views of a state dump onto the registry."""
    by_id = {view.get("id"): view for view in state.get("huntingZones", [])}
    runtime = []
    for spot in spots:
        view = by_id.get(spot["id"], {})
        runtime.append(view)
    return runtime


def embed_tiles():
    parts = []
    for bx, by in TILES:
        path = os.path.join(TILES_DIR, "%s_%s.jpg" % (bx, by))
        with open(path, "rb") as fh:
            data = base64.b64encode(fh.read()).decode("ascii")
        x = (int(bx) - 20) * TILE
        y = (int(by) - 18) * TILE
        parts.append(
            '<image x="%d" y="%d" width="%d" height="%d" '
            'href="data:image/jpeg;base64,%s" preserveAspectRatio="none"/>'
            % (x, y, TILE, TILE, data))

    return "\n".join(parts)


def esc(text):
    return (str(text).replace("&", "&amp;").replace("<", "&lt;")
            .replace(">", "&gt;").replace('"', "&quot;"))


def render_spots_svg(spots, runtime, mode):
    parts = []
    for index, spot in enumerate(spots):
        view = runtime[index] if index < len(runtime) else {}
        color = band_color(spot["min_level"])
        active = bool(view.get("active"))
        deaths = int(view.get("deaths") or 0)
        heat = float(view.get("deathHeat") or 0)
        eta = int(view.get("nextRespawnSec", -1))
        adena = float(view.get("adenaPerMin") or 0)
        occupancy = int(view.get("occupancy") or 0)
        radius = spot["radius"]
        stroke = "#f9ab00" if active else color
        width = 160 if active else 90
        # The death heat fill: warmer grounds read redder.
        heat_alpha = min(0.55, heat * 0.30)
        fill = "rgba(217,48,37,%.2f)" % heat_alpha if heat_alpha > 0.03 \
            else (color + "22" if not active else "#f9ab00" + "26")
        mobs = "; ".join(
            "%s L%d x%d (%ds-%ds)" % (
                esc(m["name"]), m["level"], m["count"],
                m["respawn_min"], m["respawn_max"])
            for m in spot["mobs"])
        tooltip = (
            "%s\\n%s\\nlevels %d-%d - anchor %d,%d - radius %d\\n"
            "expected population %.1f - respawn %d-%ds\\n"
            "composition: %s\\n"
            "runtime: %sactive, %d deaths (heat %.1f), %.0f adena/min, "
            "next respawn %s, %d hunter(s)"
            % (esc(spot["name"]), esc(spot["id"]),
               spot["min_level"], spot["max_level"],
               spot["anchor_x"], spot["anchor_y"], spot["radius"],
               spot["mass"], spot["respawn_min"], spot["respawn_max"],
               mobs,
               "" if active else "not ", deaths, heat, adena,
               ("%ds" % eta) if eta >= 0 else "unknown", occupancy))
        parts.append(
            '<g class="spot" data-id="%s" data-tip="%s">'
            '<circle cx="%d" cy="%d" r="%d" fill="%s" stroke="%s" '
            'stroke-width="%d" stroke-dasharray="260 160"/>'
            '<circle cx="%d" cy="%d" r="130" fill="%s"/>'
            "%s%s%s"
            '</g>'
            % (esc(spot["id"]), tooltip,
               spot["anchor_x"], spot["anchor_y"], radius,
               fill, stroke, width,
               spot["anchor_x"], spot["anchor_y"], stroke,
               ('<circle cx="%d" cy="%d" r="220" fill="none" '
                'stroke="#f9ab00" stroke-width="60"/>'
                % (spot["anchor_x"], spot["anchor_y"])) if active else "",
               ('<path d="M %d %d l 260 260 M %d %d l 260 -260" '
                'stroke="#e37400" stroke-width="110" stroke-linecap="round"/>'
                % (view.get("killX", 0) - 130, view.get("killY", 0) - 130,
                   view.get("killX", 0) - 130, view.get("killY", 0) + 130))
               if view.get("killX") or view.get("killY") else "",
               ('<text x="%d" y="%d" class="spot-eta">%ds</text>'
                % (spot["anchor_x"], spot["anchor_y"] - radius - 200, eta))
               if eta >= 0 else ""))
    return "\n".join(parts)


def render_legend():
    rows = []
    for low, high, color in BAND_COLORS:
        rows.append('<span class="chip"><i style="background:%s"></i>'
                    "levels %d-%d</span>" % (color, low, high))
    rows.append('<span class="chip"><i class="hot"></i>death heat</span>')
    rows.append('<span class="chip"><i style="background:#f9ab00"></i>'
                "active</span>")
    rows.append('<span class="chip"><i class="killmark"></i>kill centroid</span>')

    return "\n".join(rows)


def render_rows(spots, runtime):
    rows = []
    for index, spot in enumerate(spots):
        view = runtime[index] if index < len(runtime) else {}
        eta = int(view.get("nextRespawnSec", -1))
        adena = float(view.get("adenaPerMin") or 0)
        rows.append(
            '<tr data-id="%s"><td>%s</td><td>%d-%d</td><td>%d</td>'
            "<td>%.1f</td><td>%d-%ds</td><td>%s</td><td>%s</td>"
            "<td>%s</td></tr>"
            % (esc(spot["id"]), esc(spot["name"]),
               spot["min_level"], spot["max_level"], spot["radius"],
               spot["mass"], spot["respawn_min"], spot["respawn_max"],
               (str(int(view.get("deaths") or 0)) if view else "-"),
               ("%.0f" % adena if adena > 0 else "-"),
               (("%ds" % eta) if eta >= 0 else "-")))
    return "\n".join(rows)


def build_html(spots, runtime, character, mode, source, tile_svg):
    runtime_note = {
        "static": "the static registry: no runtime data "
                  "(pass --state or --simulate for the economy)",
        "live": "live state dump: %s" % source,
        "simulation": "SIMULATION of a hunting session - the kill/respawn "
                      "model of the spot policy drives the counters, not "
                      "live server data",
    }[mode]
    total_mass = sum(s["mass"] for s in spots)
    html = HTML_TEMPLATE
    html = html.replace("{{TILES}}", tile_svg)
    html = html.replace("{{SPOTS}}", render_spots_svg(spots, runtime, mode))
    html = html.replace("{{LEGEND}}", render_legend())
    html = html.replace("{{ROWS}}", render_rows(spots, runtime))
    html = html.replace("{{MODE}}", mode)
    html = html.replace("{{RUNTIME_NOTE}}", esc(runtime_note))
    html = html.replace("{{COUNT}}", str(len(spots)))
    html = html.replace("{{MASS}}", "%.0f" % total_mass)
    if character:
        html = html.replace("{{BOT}}",
                            '<circle cx="%d" cy="%d" r="200" fill="#1a73e8" '
                            'stroke="#ffffff" stroke-width="60"/>'
                            '<text x="%d" y="%d" class="bot-label">%s (L%d)'
                            "</text>"
                            % (character["x"], character["y"],
                               character["x"] + 260, character["y"] - 160,
                               esc(character.get("name", "bot")),
                               character.get("level", 0)))
    else:
        html = html.replace("{{BOT}}", "")

    return html


HTML_TEMPLATE = r"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Swarm hunting spots - the elven lands</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #101318; color: #e8eaed; overflow: hidden;
         font: 13px/1.45 "Segoe UI", system-ui, sans-serif; }
  #wrap { display: flex; height: 100vh; }
  #map { flex: 1; position: relative; }
  #svg { width: 100%; height: 100%; display: block; cursor: grab;
         touch-action: none; }
  #svg.dragging { cursor: grabbing; }
  .spot circle:hover { stroke-width: 220; }
  .spot-eta { font-size: 240px; font-weight: 700; fill: #f9ab00;
              paint-order: stroke; stroke: #10131acc; stroke-width: 60px;
              text-anchor: middle; }
  .bot-label { font-size: 260px; font-weight: 700; fill: #8ab4f8;
               paint-order: stroke; stroke: #10131acc; stroke-width: 60px; }
  .grid-line { stroke: #ffffff10; stroke-width: 30; }
  .village-label { font-size: 260px; fill: #9aa0a6; text-anchor: middle;
                   paint-order: stroke; stroke: #10131acc; stroke-width: 60px; }
  #tooltip { position: fixed; max-width: 460px; background: #1b2028f2;
             border: 1px solid #3c4043; border-radius: 8px; padding: 10px 12px;
             font: 12px/1.5 "Segoe UI Mono", Consolas, monospace;
             white-space: pre-wrap; pointer-events: none; z-index: 10;
             display: none; box-shadow: 0 6px 24px #000a; }
  #panel { width: 380px; background: #14181f; border-left: 1px solid #2b313b;
           display: flex; flex-direction: column; }
  #panel header { padding: 14px 16px; border-bottom: 1px solid #2b313b; }
  #panel h1 { font-size: 15px; font-weight: 600; color: #8ab4f8; }
  #panel .sub { color: #9aa0a6; margin-top: 4px; font-size: 12px; }
  #panel .stats { display: flex; gap: 14px; margin-top: 10px; }
  #panel .stats b { display: block; font-size: 17px; color: #e8eaed; }
  #panel .stats span { font-size: 11px; color: #9aa0a6; }
  #legend { padding: 10px 16px; border-bottom: 1px solid #2b313b;
            display: flex; flex-wrap: wrap; gap: 6px 10px; }
  .chip { display: inline-flex; align-items: center; gap: 6px;
          font-size: 11.5px; color: #bdc1c6; }
  .chip i { width: 11px; height: 11px; border-radius: 50%; display: inline-block;
            background: #5f6368; }
  .chip i.hot { background: linear-gradient(90deg, #d93025, #d9302500); }
  .chip i.killmark { border-radius: 0; width: 10px; height: 10px;
            background: linear-gradient(45deg, transparent 42%, #e37400 42%,
                      #e37400 58%, transparent 58%); }
  #help { padding: 10px 16px; color: #9aa0a6; font-size: 11.5px;
          border-bottom: 1px solid #2b313b; }
  #table-wrap { flex: 1; overflow: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  thead th { position: sticky; top: 0; background: #14181f; color: #9aa0a6;
             text-align: left; font-weight: 600; padding: 8px 10px;
             border-bottom: 1px solid #2b313b; }
  tbody td { padding: 7px 10px; border-bottom: 1px solid #20242c;
             white-space: nowrap; }
  tbody tr { cursor: pointer; }
  tbody tr:hover { background: #1b212b; }
  tbody tr.active-row { background: #2a2413; }
  .badge { display: inline-block; padding: 1px 7px; border-radius: 10px;
           font-size: 10.5px; }
</style>
</head>
<body>
<div id="wrap">
  <div id="map">
    <svg id="svg" viewBox="{{VX}} {{VY}} {{VW}} {{VH}}" preserveAspectRatio="xMidYMid meet">
      <g id="tiles">{{TILES}}</g>
      <g id="gridlines"></g>
      <g id="village">
        <circle cx="46112" cy="41500" r="260" fill="#9aa0a6"/>
        <text x="46112" y="40900" class="village-label">Elven Village</text>
      </g>
      <g id="spots">{{SPOTS}}</g>
      <g id="bot">{{BOT}}</g>
    </svg>
    <div id="tooltip"></div>
  </div>
  <div id="panel">
    <header>
      <h1>Hunting spots - the elven lands</h1>
      <div class="sub">spot-anchored registry - {{COUNT}} grounds,
        {{MASS}} expected mobs</div>
      <div class="sub">{{RUNTIME_NOTE}}</div>
      <div class="stats">
        <div><b>{{COUNT}}</b><span>spots</span></div>
        <div><b>2048</b><span>visibility radius cap</span></div>
        <div><b>15-20s</b><span>respawn window</span></div>
      </div>
    </header>
    <div id="legend">{{LEGEND}}</div>
    <div id="help">scroll = zoom, drag = pan, hover = spot details,
      click a row = center the map. The circle is the visibility bounded
      ground of the spot, the dashed square would be its leash.</div>
    <div id="table-wrap">
      <table>
        <thead><tr>
          <th>spot</th><th>levels</th><th>radius</th><th>mass</th>
          <th>respawn</th><th>deaths</th><th>a/min</th><th>next</th>
        </tr></thead>
        <tbody>{{ROWS}}</tbody>
      </table>
    </div>
  </div>
</div>
<script>
(function () {
  "use strict";
  var svg = document.getElementById("svg");
  var tooltip = document.getElementById("tooltip");
  var base = svg.viewBox.baseVal;
  var home = [{{VX}}, {{VY}}, {{VW}}, {{VH}}];
  var drag = null;

  function worldToScreen(x, y) {
    var rect = svg.getBoundingClientRect();
    var scale = Math.min(rect.width / base.width, rect.height / base.height);
    var ox = (rect.width - base.width * scale) / 2;
    var oy = (rect.height - base.height * scale) / 2;
    return {
      x: ox + (x - base.x) * scale,
      y: oy + (y - base.y) * scale,
    };
  }

  svg.addEventListener("wheel", function (e) {
    e.preventDefault();
    var factor = e.deltaY < 0 ? 0.8 : 1.25;
    var rect = svg.getBoundingClientRect();
    var scale = Math.min(rect.width / base.width, rect.height / base.height);
    var ox = (rect.width - base.width * scale) / 2;
    var oy = (rect.height - base.height * scale) / 2;
    var wx = base.x + (e.clientX - rect.left - ox) / scale;
    var wy = base.y + (e.clientY - rect.top - oy) / scale;
    var nw = Math.max(2048, Math.min(65536, base.width * factor));
    var nh = nw * base.height / base.width;
    base.x = wx - (wx - base.x) * nw / base.width;
    base.y = wy - (wy - base.y) * nh / base.height;
    base.width = nw;
    base.height = nh;
  });

  svg.addEventListener("mousedown", function (e) {
    drag = { x: e.clientX, y: e.clientY, bx: base.x, by: base.y };
    svg.classList.add("dragging");
  });
  window.addEventListener("mousemove", function (e) {
    if (drag) {
      var rect = svg.getBoundingClientRect();
      var scale = Math.min(rect.width / base.width,
                           rect.height / base.height);
      base.x = drag.bx - (e.clientX - drag.x) / scale;
      base.y = drag.by - (e.clientY - drag.y) / scale;
      return;
    }
    var spot = e.target.closest && e.target.closest(".spot");
    if (spot && svg.contains(e.target)) {
      tooltip.textContent = spot.dataset.tip;
      tooltip.style.display = "block";
      tooltip.style.left = Math.min(window.innerWidth - 480,
                                    e.clientX + 16) + "px";
      tooltip.style.top = Math.min(window.innerHeight - 260,
                                    e.clientY + 14) + "px";
    } else {
      tooltip.style.display = "none";
    }
  });
  window.addEventListener("mouseup", function () {
    drag = null;
    svg.classList.remove("dragging");
  });

  // The region grid lines every 2048 units (the WorldRegion cells of
  // the knownlist visibility).
  var grid = document.getElementById("gridlines");
  var parts = [];
  for (var x = 0; x <= 65536; x += 2048) {
    parts.push('<line class="grid-line" x1="' + x + '" y1="32768" ' +
               'x2="' + x + '" y2="98304"/>');
  }
  for (var y = 32768; y <= 98304; y += 2048) {
    parts.push('<line class="grid-line" x1="0" y1="' + y + '" ' +
               'x2="65536" y2="' + y + '"/>');
  }
  grid.innerHTML = parts.join("");

  // The spot table rows center the map on their spot.
  var rows = document.querySelectorAll("tbody tr");
  rows.forEach(function (row) {
    row.addEventListener("click", function () {
      var spot = document.querySelector(
          '.spot[data-id="' + row.dataset.id + '"]');
      if (!spot) { return; }
      var circle = spot.querySelector("circle");
      var cx = parseFloat(circle.getAttribute("cx"));
      var cy = parseFloat(circle.getAttribute("cy"));
      var w = 8192;
      base.width = w;
      base.height = w;
      base.x = cx - w / 2;
      base.y = cy - w / 2;
    });
  });

  document.addEventListener("keydown", function (e) {
    if (e.key === "0") {
      base.x = home[0]; base.y = home[1];
      base.width = home[2]; base.height = home[3];
    }
  });
})();
</script>
</body>
</html>
"""


def main():
    parser = argparse.ArgumentParser(
        description="Render the hunting spot map")
    parser.add_argument("--out", default=DEFAULT_OUT)
    parser.add_argument("--spots", default=SPOTS_JSON)
    parser.add_argument("--state", default=None,
                        help="a bot state dump JSON (the web UI dump)")
    parser.add_argument("--simulate", type=float, default=0,
                        help="simulate N minutes of a hunting session")
    parser.add_argument("--level", type=int, default=8)
    parser.add_argument("--seed", type=int, default=7)
    args = parser.parse_args()

    with open(args.spots, encoding="utf-8") as fh:
        payload = json.load(fh)
    spots = payload["spots"]
    character = None
    runtime = [{} for _ in spots]
    mode = "static"
    if args.simulate > 0:
        state = simulate(spots, args.simulate, args.level, args.seed)
        runtime = merge_state(spots, state)
        character = state["character"]
        mode = "simulation"
    elif args.state:
        with open(args.state, encoding="utf-8") as fh:
            state = json.load(fh)
        runtime = merge_state(spots, state)
        character = state.get("character") or None
        mode = "live"

    html = build_html(spots, runtime, character, mode,
                      args.state or "simulation", embed_tiles())
    html = (html.replace("{{VX}}", str(VIEW["x"]))
                .replace("{{VY}}", str(VIEW["y"]))
                .replace("{{VW}}", str(VIEW["w"]))
                .replace("{{VH}}", str(VIEW["h"])))
    out_dir = os.path.dirname(os.path.abspath(args.out))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as fh:
        fh.write(html)
    print("spots: %d, mode: %s" % (len(spots), mode))
    print("written: %s (%.1f MB)" % (args.out, os.path.getsize(args.out) / 1e6))


if __name__ == "__main__":
    sys.exit(main())
