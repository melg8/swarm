#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
"""Quantitative analysis of the hunting zone registry: the research
input of docs/hunting_system_redesign.md.

Parses the generated registry internal/swarm/hunt/zones_elven.go (the
parsed Mobius ElvenStarting.xml spawn data) and the npc stat maps of
internal/swarm/npcdata/names.go (aggressive flags, aggro and clan help
ranges, wire id map), prints the summary stats to stdout and writes the
charts plus summary.json into docs/hunt_analysis/ (OUT_DIR overrides).

Run: python3 tools/analyze_hunt_registry.py  (matplotlib required)
"""

import json
import math
import os
import re
import sys
from collections import defaultdict

import matplotlib
matplotlib.use("Agg")
import matplotlib.font_manager as fm
for f in [
    "/usr/share/fonts/truetype/chinese/NotoSansSC-Regular.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
]:
    if os.path.exists(f):
        fm.fontManager.addfont(f)
import matplotlib.pyplot as plt
plt.rcParams["font.sans-serif"] = ["Noto Sans SC", "DejaVu Sans"]
plt.rcParams["axes.unicode_minus"] = False

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHART_DIR = os.environ.get("OUT_DIR", os.path.join(REPO, "docs", "hunt_analysis"))
os.makedirs(CHART_DIR, exist_ok=True)

# World region grid: Mobius WorldRegion 2048x2048 units (SHIFT_BY 11).
# A player sees its own region + the 8 adjacent ones: guaranteed
# visible radius 2048, typical from region center ~3072, hard cap 4096.
REGION = 2048
VIS_GUARANTEED = 2048   # always visible
VIS_TYPICAL = 3072      # from region center
VIS_CAP = 4096          # never visible beyond

VILLAGE = (46112, 41500)

BAND_COLORS = {
    (1, 3): "#8bc34a", (3, 4): "#7cb342", (4, 6): "#689f38",
    (5, 7): "#558b2f", (7, 8): "#fbc02d", (8, 10): "#f9a825",
    (9, 12): "#f57f17", (11, 13): "#e64a19", (12, 14): "#d84315",
    (13, 16): "#ad1457", (16, 19): "#880e4f",
}


# ---------------------------------------------------------------- parsing

def parse_zones():
    src = open(os.path.join(
        REPO, "internal/swarm/hunt/zones_elven.go"), encoding="utf-8").read()
    zones = []
    # split per zone block
    for block in re.finditer(r"\{\s*ID:\s*\"([^\"]+)\",\s*"
                             r"Name:\s*\"([^\"]+)\",\s*Region: regionElven,"
                             r"\s*MinLevel:\s*(\d+),\s*MaxLevel:\s*(\d+),"
                             r"\s*MinGear:\s*(\d+),\s*CX:\s*(-?\d+),"
                             r"\s*CY:\s*(-?\d+),\s*Half:\s*(\d+),"
                             r"\s*Mobs:\s*\[\]ZoneMob\{(.*?)\},\s*\},",
                             src, re.S):
        zid, name = block.group(1), block.group(2)
        minl, maxl, gear = int(block.group(3)), int(block.group(4)), int(block.group(5))
        cx, cy, half = int(block.group(6)), int(block.group(7)), int(block.group(8))
        mobs = []
        for m in re.finditer(r"TemplateID:\s*(\d+),\s*Name:\s*\"([^\"]+)\","
                             r"\s*Level:\s*(\d+),\s*Count:\s*(\d+),"
                             r"\s*Priority:\s*(\d+)", block.group(9)):
            mobs.append({"tid": int(m.group(1)), "name": m.group(2),
                         "level": int(m.group(3)), "count": int(m.group(4)),
                         "priority": int(m.group(5))})
        zones.append({"id": zid, "name": name, "min": minl, "max": maxl,
                      "gear": gear, "cx": cx, "cy": cy, "half": half,
                      "mobs": mobs})
    return zones


def parse_map(src, name, value_conv=int):
    m = re.search(r"var %s = map\[int32\][^{]*\{(.*?)\n\}" % name, src, re.S)
    out = {}
    for kv in re.finditer(r"(\d+):\s*([^,\n]+)[,\n]", m.group(1)):
        out[int(kv.group(1))] = value_conv(kv.group(2).strip())
    return out


def parse_npc():
    src = open(os.path.join(
        REPO, "internal/swarm/npcdata/names.go"), encoding="utf-8").read()
    data = {
        "aggro": parse_map(src, "npcAggroRanges"),
        "help": parse_map(src, "npcClanHelpRanges"),
        "wire": parse_map(src, "npcInternalWireIDs"),
        "aggr": parse_map(src, "npcAggressives", value_conv=lambda v: v == "true"),
    }
    return data


# ---------------------------------------------------------------- helpers

def territory_of(zone):
    # elven-2119_01-a1 -> 2119_01 ; elven-xxx -> xxx
    core = zone["id"][len("elven-"):]
    if re.match(r"^[0-9]+_[0-9]+-[a-z]\d+$", core):
        return core.rsplit("-", 1)[0]
    return core


def area(z):
    return (2 * z["half"]) ** 2


def rect_overlap(a, b):
    ox = max(0, min(a["cx"] + a["half"], b["cx"] + b["half"]) -
              max(a["cx"] - a["half"], b["cx"] - b["half"]))
    oy = max(0, min(a["cy"] + a["half"], b["cy"] + b["half"]) -
              max(a["cy"] - a["half"], b["cy"] - b["half"]))
    return ox * oy


def frac_visible_from_center(z, vis):
    """Fraction of the square within `vis` of the center (circle)."""
    # integrate: for radius r = vis (capped at half*sqrt2), the covered
    # area inside the square = min(vis, half)^2 * 4 when vis <= half
    # (full inscribed square); plus circle segments beyond.
    h, r = z["half"], vis
    if r >= h * math.sqrt(2):
        return 1.0
    if r <= h:
        # circle area minus 4 corner segments outside the square
        seg = r * r * (math.acos(0.0) * 0 - 0)  # placeholder, integrate below
        # area of circle of radius r = pi r^2; the parts sticking out
        # of the square: 4 segments beyond each side when r > h (not
        # here). For r <= h the circle is fully inside the square.
        return math.pi * r * r / area(z)
    # h < r < h*sqrt2: circle fully covers the inscribed cross; area
    # inside square = pi r^2 - 4 * circular segments beyond sides.
    # Segment beyond a side at distance h from center: area of circular
    # region with x > h (and x < r), same for y sides.
    theta = 2 * math.acos(h / r)
    seg = 0.5 * r * r * (theta - math.sin(theta))
    inside = math.pi * r * r - 4 * seg
    return min(1.0, inside / area(z))


def main():
    zones = parse_zones()
    npc = parse_npc()
    print("zones parsed: %d" % len(zones))

    # territory grouping and mass split
    terr = defaultdict(list)
    for z in zones:
        terr[territory_of(z)].append(z)
    for t, zs in terr.items():
        total_area = sum(area(z) for z in zs)
        # unique mob species total per territory (mobs list identical
        # across squares of one territory)
        counts = {m["tid"]: m["count"] for m in zs[0]["mobs"]}
        terr_total = sum(counts.values())
        for z in zs:
            share = area(z) / total_area
            z["mass"] = {m["tid"]: m["count"] * share for m in z["mobs"]}
            z["mass_total"] = terr_total * share
            z["density"] = z["mass_total"] / (area(z) / 1e6)  # mobs / km2-ish

    # ---- global spawn mass by level
    mass_by_level = defaultdict(int)
    for t, zs in terr.items():
        for m in zs[0]["mobs"]:
            mass_by_level[m["level"]] += m["count"]
    total_mass = sum(mass_by_level.values())
    print("territories: %d, total spawn mass: %d mobs"
          % (len(terr), total_mass))

    # ---- aggro join (zone template ids are CT0 xml ids -> wire)
    def npc_join(tid, table):
        wire = npc["wire"].get(tid, None)
        if wire is None:
            # unmapped internal ids: pass through (NPCWireTemplateID
            # semantics) then subtract offset
            key = tid - 1000000 if tid > 1000000 else tid
            if key not in table and tid in table:
                key = tid
        else:
            key = wire - 1000000
        return table.get(key, None)

    aggr_mass = 0
    social_mass = 0
    for t, zs in terr.items():
        for m in zs[0]["mobs"]:
            is_aggr = npc_join(m["tid"], npc["aggr"])
            help_r = npc_join(m["tid"], npc["help"])
            if is_aggr:
                aggr_mass += m["count"]
            if help_r and help_r > 0:
                social_mass += m["count"]
    print("aggressive spawn mass: %d/%d (%.0f%%)"
          % (aggr_mass, total_mass, 100.0 * aggr_mass / total_mass))
    print("social (clanHelpRange>0) mass: %d/%d (%.0f%%)"
          % (social_mass, total_mass, 100.0 * social_mass / total_mass))

    # ---- per band stats
    band_stats = defaultdict(lambda: {
        "zones": 0, "area": 0, "mass": 0, "halves": [],
        "vis2048": [], "vis3072": [], "densities": [], "aggr": 0})
    for z in zones:
        b = band_stats[(z["min"], z["max"])]
        b["zones"] += 1
        b["area"] += area(z)
        b["mass"] += z["mass_total"]
        b["halves"].append(z["half"])
        b["vis2048"].append(frac_visible_from_center(z, VIS_GUARANTEED))
        b["vis3072"].append(frac_visible_from_center(z, VIS_TYPICAL))
        b["densities"].append(z["density"])
        aggr_share = sum(cnt for tid, cnt in z["mass"].items()
                         if npc_join(tid, npc["aggr"]))
        b["aggr"] += aggr_share

    print("\n%-12s %6s %8s %8s %8s %10s %10s" % (
        "band", "zones", "mass", "avg half", "dens", "vis2048", "vis3072"))
    for band in sorted(band_stats):
        b = band_stats[band]
        print("%-12s %6d %8.0f %8.0f %8.1f %10.2f %10.2f" % (
            "%d-%d" % band, b["zones"], b["mass"],
            sum(b["halves"]) / len(b["halves"]),
            b["mass"] / (b["area"] / 1e6),
            sum(b["vis2048"]) / len(b["vis2048"]),
            sum(b["vis3072"]) / len(b["vis3072"])))

    # ---- overlap inside bands
    overlap_frac = []
    for band, zs_same in defaultdict(list, {
            k: [] for k in band_stats}).items():
        pass
    by_band = defaultdict(list)
    for z in zones:
        by_band[(z["min"], z["max"])].append(z)
    for band, zs in by_band.items():
        for i, a in enumerate(zs):
            cov = 0.0
            for j, b in enumerate(zs):
                if i == j:
                    continue
                cov += rect_overlap(a, b)
            overlap_frac.append(min(1.0, cov / area(a)))
    print("\nsame-band zone overlap: mean %.0f%%, median %.0f%%, "
          "max %.0f%%, zones with >30%% overlap: %d/%d" % (
              100 * sum(overlap_frac) / len(overlap_frac),
              100 * sorted(overlap_frac)[len(overlap_frac) // 2],
              100 * max(overlap_frac),
              sum(1 for f in overlap_frac if f > 0.30), len(overlap_frac)))

    # ---- per zone: worst-case walk visibility (bot at one edge, mobs
    # at the opposite edge: distance up to 2*half)
    far_corner = [2 * z["half"] for z in zones]
    print("opposite-edge distance (2*half): median %d, p90 %d, max %d" % (
        sorted(far_corner)[len(far_corner) // 2],
        sorted(far_corner)[int(len(far_corner) * 0.9)],
        max(far_corner)))
    n_beyond_cap = sum(1 for d in far_corner if d > VIS_CAP)
    n_beyond_typ = sum(1 for d in far_corner if d > VIS_TYPICAL)
    print("zones whose opposite edge exceeds the typical visible radius "
          "(3072): %d/%d; exceeds the hard cap (4096): %d/%d" % (
              n_beyond_typ, len(zones), n_beyond_cap, len(zones)))

    # ---- respawn capacity (15-20s per mob, avg 17.5)
    RESPAWN = 17.5
    caps = []
    for z in zones:
        pickable = z["mass_total"]
        caps.append(60.0 * pickable / RESPAWN)
    print("\nrespawn capacity per zone (kills/min, all mobs incl. "
          "unpickable): median %.1f, min %.1f" % (
              sorted(caps)[len(caps) // 2], min(caps)))

    # ---- level gap windows (white-green farming)
    # item drops: 100% at player<=mob+5 .. 10% at mob+10; adena 100% at
    # +8 .. 10% at +15; exp/SP zero at gap>=11 (mob above player has no
    # penalty but slow kills).
    print("\nwhite-green windows: for bot level L, mobs in [L-5..L+0] "
          "keep full drops")
    windows = []
    for L in range(6, 21):
        m = sum(cnt for lvl, cnt in mass_by_level.items()
                if L - 5 <= lvl <= L)
        windows.append((L, m))
    print(" ".join("L%d:%d" % w for w in windows))

    # ----------------------------------------------------------------
    # charts
    # ----------------------------------------------------------------

    # 1. spawn mass by level
    fig, ax = plt.subplots(figsize=(7.2, 3.4), constrained_layout=True)
    lv = sorted(mass_by_level)
    vals = [mass_by_level[l] for l in lv]
    cols = ["#7cb342" if l <= 8 else "#fbc02d" if l <= 13 else "#e64a19"
            for l in lv]
    ax.bar(lv, vals, color=cols)
    ax.set_xlabel("Уровень моба")
    ax.set_ylabel("Число заспавненных мобов")
    ax.set_title("Масса спавнов эльфийских земель по уровням мобов\n"
                 "(ElvenStarting.xml, %d мобов, %d территорий)"
                 % (total_mass, len(terr)))
    ax.grid(axis="y", alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "spawn_mass_by_level.png"), dpi=150)
    plt.close(fig)

    # 2. spatial map of zones by band
    fig, ax = plt.subplots(figsize=(7.2, 6.0), constrained_layout=True)
    for band in sorted(by_band):
        zs = by_band[band]
        ax.scatter([z["cx"] for z in zs], [z["cy"] for z in zs],
                   s=[area(z) / 2600.0 for z in zs],
                   c=BAND_COLORS.get(band, "#999"), alpha=0.55,
                   edgecolors="none", label="%d-%d" % band)
    ax.scatter(*VILLAGE, marker="*", s=280, c="#1a237e", zorder=5,
               label="Эльфийская деревня")
    ax.invert_yaxis()
    ax.set_xlabel("X мира")
    ax.set_ylabel("Y мира")
    ax.set_title("227 текущих зон охоты (размер точки = площадь квадрата)")
    ax.legend(loc="upper left", bbox_to_anchor=(1.01, 1.0), fontsize=8,
              title="Полоса уровней")
    ax.grid(alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "zones_map.png"), dpi=150)
    plt.close(fig)

    # 3. visibility fraction from center per zone
    fig, ax = plt.subplots(figsize=(7.2, 3.4), constrained_layout=True)
    vis_g = [f for z in zones for f in [frac_visible_from_center(z, VIS_GUARANTEED)]]
    vis_t = [f for z in zones for f in [frac_visible_from_center(z, VIS_TYPICAL)]]
    ax.scatter([z["half"] for z in zones], vis_g, s=14, alpha=0.6,
               c="#3949ab", label="Гарантированный радиус 2048")
    ax.scatter([z["half"] for z in zones], vis_t, s=14, alpha=0.6,
               c="#fbc02d", label="Типичный радиус 3072 (из центра региона)")
    ax.set_xlabel("Половина стороны зоны (half), юниты")
    ax.set_ylabel("Доля площади зоны,\nвидимая из центра")
    ax.set_title("Видимость зоны из её центра: гарантированный и типичный радиусы\n"
                 "WorldRegion 2048, собственный регион + 8 соседних")
    ax.legend(loc="lower left")
    ax.grid(alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "zone_visibility.png"), dpi=150)
    plt.close(fig)

    # 4. density vs mass (the concentration argument)
    fig, ax = plt.subplots(figsize=(7.2, 3.6), constrained_layout=True)
    xs = [z["density"] for z in zones]
    ys = [z["mass_total"] for z in zones]
    band_of_z = defaultdict(list)
    for z in zones:
        band_of_z[(z["min"], z["max"])].append(z)
    for band in sorted(band_of_z):
        zs = band_of_z[band]
        ax.scatter([z["density"] for z in zs], [z["mass_total"] for z in zs],
                   s=16, alpha=0.65, c=BAND_COLORS.get(band, "#999"),
                   label="%d-%d" % band)
    ax.set_xlabel("Плотность мобов (мобов на 1000×1000 юнитов)")
    ax.set_ylabel("Масса спавна в квадрате (мобов)")
    ax.set_title("Плотность и масса спавна по квадратам: концентрация мобов\n"
                 "различается в разы между квадратами одной полосы")
    ax.legend(loc="upper right", fontsize=8, title="Полоса")
    ax.grid(alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "density_mass.png"), dpi=150)
    plt.close(fig)

    # 5. white-green windows
    fig, ax = plt.subplots(figsize=(7.2, 3.2), constrained_layout=True)
    Ls = [w[0] for w in windows]
    ms = [w[1] for w in windows]
    ax.bar(Ls, ms, color="#43a047", alpha=0.85)
    ax.set_xlabel("Уровень бота L")
    ax.set_ylabel("Масса мобов в окне [L-5..L]")
    ax.set_title("Сколько мобов доступно в «белом-зелёном» окне\n"
                 "(полный дроп: игрок не выше моба + 5)")
    ax.grid(axis="y", alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "white_green_window.png"), dpi=150)
    plt.close(fig)

    # summary json
    summary = {
        "zones": len(zones),
        "territories": len(terr),
        "total_mass": total_mass,
        "mass_by_level": dict(mass_by_level),
        "aggressive_mass": aggr_mass,
        "social_mass": social_mass,
        "band_stats": {
            "%d-%d" % k: {
                "zones": v["zones"], "mass": v["mass"],
                "area_km2": v["area"] / 1e6,
                "avg_half": sum(v["halves"]) / len(v["halves"]),
                "avg_vis2048": sum(v["vis2048"]) / len(v["vis2048"]),
                "avg_vis3072": sum(v["vis3072"]) / len(v["vis3072"]),
                "density": v["mass"] / (v["area"] / 1e6),
                "aggr_mass": v["aggr"],
            } for k, v in band_stats.items()},
        "overlap": {
            "mean": sum(overlap_frac) / len(overlap_frac),
            "median": sorted(overlap_frac)[len(overlap_frac) // 2],
            "max": max(overlap_frac),
            "gt30": sum(1 for f in overlap_frac if f > 0.30),
        },
    }
    # ---- rotation costs, pack sizes and the hotspot prototype -------
    RUN_SPEED = 130.0  # units/sec, an elven fighter's run speed
    RESPAWN_MIN, RESPAWN_MAX = 15.0, 20.0
    by_band = defaultdict(list)
    for z in zones:
        by_band[(z["min"], z["max"])].append(z)
    nearest = []
    for band, zs in by_band.items():
        for a in zs:
            d = min(math.hypot(b["cx"] - a["cx"], b["cy"] - a["cy"])
                    for b in zs if b is not a)
            nearest.append(d)
    nearest.sort()
    med = nearest[len(nearest) // 2]
    print("\nnearest same-band sibling distance: median %d units (%.0fs "
          "walk), max %d" % (med, med / RUN_SPEED, nearest[-1]))
    wait_mid = (RESPAWN_MIN + RESPAWN_MAX) / 2
    print("zones whose rotation walk (%.0fs median wait) exceeds the "
          "respawn window: %d/%d" % (
              wait_mid,
              sum(1 for d in nearest if d / RUN_SPEED > wait_mid),
              len(nearest)))
    walks = [d / RUN_SPEED for d in nearest]
    pack = sorted(z["mass_total"] for z in zones)
    print("mobs per zone square: median %.1f" % (pack[len(pack) // 2]))

    # density grid + hotspot clustering prototype of the spot model
    GRID = 512
    cells = defaultdict(float)
    for z in zones:
        per_cell = z["mass_total"] / ((2 * z["half"] / GRID) ** 2)
        for gx in range(z["cx"] - z["half"], z["cx"] + z["half"], GRID):
            for gy in range(z["cy"] - z["half"], z["cy"] + z["half"], GRID):
                cells[(gx // GRID, gy // GRID)] += per_cell
    hot = {k for k, v in cells.items() if v >= 0.30}
    seen, spots = set(), []
    for k in sorted(hot):
        if k in seen:
            continue
        stack, comp = [k], []
        seen.add(k)
        while stack:
            c = stack.pop()
            comp.append(c)
            for dx in (-1, 0, 1):
                for dy in (-1, 0, 1):
                    n = (c[0] + dx, c[1] + dy)
                    if n in hot and n not in seen:
                        seen.add(n)
                        stack.append(n)
        mass = sum(cells[c] for c in comp)
        big = mass >= 2.0
        if big:
            cx = sum(c[0] for c in comp) / len(comp) * GRID + GRID / 2
            cy = sum(c[1] for c in comp) / len(comp) * GRID + GRID / 2
            spots.append((cx, cy, mass))
    print("hotspot clustering prototype: %d spots of mass >= 2 mobs "
          "(vs %d squares)" % (len(spots), len(zones)))

    fig, ax = plt.subplots(figsize=(7.2, 3.2), constrained_layout=True)
    ax.hist(walks, bins=32, color="#5c6bc0", alpha=0.85)
    ax.axvspan(RESPAWN_MIN, RESPAWN_MAX, color="#e53935", alpha=0.15,
               label="Respawn window 15-20 s")
    ax.set_xlabel("Walk time to the nearest same-band zone, s")
    ax.set_ylabel("Zones")
    ax.set_title("Zone rotation cost vs the respawn wait")
    ax.legend()
    ax.grid(axis="y", alpha=0.3)
    fig.savefig(os.path.join(CHART_DIR, "rotation_vs_respawn.png"), dpi=150)
    plt.close(fig)

    with open(os.environ.get("SUMMARY", os.path.join(CHART_DIR, "summary.json")), "w",
              encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=1)
    print("\ncharts written to %s" % CHART_DIR)


if __name__ == "__main__":
    sys.exit(main())
