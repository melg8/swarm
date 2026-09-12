#!/usr/bin/env python3
"""Generate the elven hunting zone registry from the Mobius C1 spawn data.

Why: the mobs of a territory spawn at uniformly random points inside the
spawn polygon (Spawn.initializeNpc -> NpcSpawnTerritory.getRandomPoint)
and their random walk stays inside the polygon (AttackableAI checks
spawnTerritory.isInsideZone for every wander target, MaxDriftRange 300
leashes the rest). A hunting square anchored on a "cluster centroid"
therefore only covers the fraction of the spawn area that happens to
fall inside it - the measured coverage of the old hand placed registry
was 18 percent of the expected mob mass. This generator derives the
squares from the polygons themselves: one or several compact squares
per territory, every square carrying the full mob list of its
territory (all species are farmed, level-sorted priorities bias the
engage toward the exp rich ones).

Inputs (env-overridable):
  MOBIUS_C1  the L2J_Mobius_C1_HarbingersOfWar dist tree
             (default: /home/z/my-project/l2j_mobius/...)
  OUT        the generated Go file path
             (default: internal/swarm/hunt/zones_elven.go of this repo)

Output: deterministic Go source with the elvenHuntingZones registry.

Survey mode: `generate_hunt_zones.py --survey MIN MAX` scans ALL spawn
territories (not just ElvenStarting.xml) for the mobs of the given
level band, joins the mob stats (hp, exp, the AI block - NpcTemplate
fills isAggressive TRUE when the xml omits it, so the survey reports
the effective aggression) and prints the grounds with their walking
distance from the teleport network arrival points, plus the anchors
themselves. The registry generation for such a band stays a separate
task (M1 first); the survey is the data the band survey document and
the follow-up tasks build on.
"""

import glob
import math
import os
import re
import sys
import xml.etree.ElementTree as ET

MOBIUS_C1 = os.environ.get(
    "MOBIUS_C1",
    "/home/z/my-project/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar",
)
SPAWN_XML = os.path.join(
    MOBIUS_C1, "dist/game/data/spawns/ElvenTerritory/ElvenStarting.xml")
SPAWNS_DIR = os.path.join(MOBIUS_C1, "dist/game/data/spawns")
NPC_STATS_DIR = os.path.join(MOBIUS_C1, "dist/game/data/stats/npcs")
TELEPORTER_DIR = os.path.join(
    MOBIUS_C1, "dist/game/data/teleporters/town")
OUT = os.environ.get(
    "OUT",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                 "internal/swarm/hunt/zones_elven.go"))

# The ten mob level bands of the elven ladder: (minLevel, maxLevel,
# minGear). The band of a territory is the lowest band whose window
# contains the territory's top mob level - the character meets the
# mobs of its own level through the zoneLevelLead picker gate.
BANDS = [
    (1, 3, 0),
    (3, 4, 20),
    (4, 6, 50),
    (5, 7, 70),
    (7, 8, 100),
    (8, 10, 140),
    (9, 12, 180),
    (11, 13, 220),
    (12, 14, 260),
    (13, 16, 300),
    (16, 19, 380),
]

# The anchor of the registry ordering: the elven village surroundings.
# The first zone of the sorted registry is the starter fallback of the
# picker (a fresh character hunts the nearest band 1-3 square).
VILLAGE_X, VILLAGE_Y = 46112, 41500

# Geometry knobs. SINGLE_MAX_R: a polygon whose every node sits within
# this radius of its centroid becomes one square. CELL mobs target:
# the partition grid cell side adapts to the spawn density so a square
# holds a small pack instead of one lone mob (clamped to keep every
# square compact). KEEP_FRAC drops the grid cells whose overlap with
# the polygon is a thin sliver (the spawn mass there is negligible).
SINGLE_MAX_R = 1250
CELL_MIN, CELL_MAX = 2600, 3600
CELL_MOBS_TARGET = 4.0
KEEP_FRAC = 0.22
HALF_MARGIN = 100

COMPASS = ["E", "NE", "N", "NW", "W", "SW", "S", "SE"]


def load_npc_stats():
    stats = {}
    for path in glob.glob(NPC_STATS_DIR + "/*.xml"):
        try:
            root = ET.parse(path).getroot()
        except ET.ParseError:
            continue
        for npc in root.iter("npc"):
            stats[int(npc.get("id"))] = (
                int(npc.get("level", "0")), npc.get("name", "?"))
    return stats


# The survey needs more of every mob than the registry: the vitals and
# the AI block of the npc xml (NpcTemplate fills isAggressive TRUE when
# the attribute is omitted - see the survey docstring).
def load_npc_survey_stats():
    stats = {}
    for path in glob.glob(NPC_STATS_DIR + "/*.xml"):
        try:
            root = ET.parse(path).getroot()
        except ET.ParseError:
            continue
        for npc in root.iter("npc"):
            ai = npc.find("ai")
            acquire = npc.find("acquire")
            vitals = npc.find(".//vitals")
            exp = int(float(acquire.get("exp", "0"))) \
                if acquire is not None else 0
            hp = int(float(vitals.get("hp", "0"))) if vitals is not None \
                else 0
            if ai is None:
                aggressive = True
                aggro_range = 0
                clan_help = 0
            else:
                aggressive = ai.get("isAggressive", "true") != "false"
                aggro_range = int(ai.get("aggroRange", "0"))
                clan_help = int(ai.get("clanHelpRange", "0"))
            stats[int(npc.get("id"))] = {
                "level": int(npc.get("level", "0")),
                "name": npc.get("name", "?"),
                "type": npc.get("type", "?"),
                "exp": exp, "hp": hp, "aggressive": aggressive,
                "aggro_range": aggro_range, "clan_help": clan_help,
            }
    return stats


# The teleport network of the survey: the arrival points the band
# grounds measure their walking distance against. The elven village
# gatekeeper chain (Mirabel -> Bella -> Trisha) plus the local
# destinations of the Dion gatekeeper (the service town of the 20-25
# band grounds).
# The teleport chain preference of the survey anchors: the elven
# village route (Mirabel the elven village, Bella the Gludio hub,
# Trisha the Dion service town). A destination name offered by many
# gatekeepers resolves to the hop of this chain so the printed fee is
# the one the route from the elven lands pays.
ROUTE_GATEKEEPERS = ("Mirabel", "Bella", "Trisha")


def load_teleport_arrivals():
    arrivals = {}
    pattern = re.compile(
        r'<location name="([^"]+)" x="(-?\d+)" y="(-?\d+)" '
        r'z="(-?\d+)" feeCount="(\d+)"')
    for path in sorted(glob.glob(TELEPORTER_DIR + "/*.xml")):
        content = open(path, encoding="utf-8").read()
        npc_id = re.search(r'<npc id="(\d+)">\s*<!-- ([A-Za-z]+) -->',
                           content)
        if npc_id is None:
            continue
        for match in pattern.finditer(content):
            name, x, y, z, fee = match.groups()
            if int(fee) > 30000:  # the noble/arena entries skip
                continue
            arrivals.setdefault(name, []).append((
                npc_id.group(2), int(x), int(y), int(fee)))
    return arrivals


def route_arrival(arrivals, name):
    """The arrival entry of the name on the elven route chain."""
    entries = arrivals.get(name)
    if not entries:
        return None
    for gatekeeper in ROUTE_GATEKEEPERS:
        for entry in entries:
            if entry[0] == gatekeeper:
                return entry
    return entries[0]


def shoelace(nodes):
    a = 0.0
    for i in range(len(nodes)):
        x0, y0 = nodes[i]
        x1, y1 = nodes[(i + 1) % len(nodes)]
        a += x0 * y1 - x1 * y0
    return a / 2.0


def centroid(nodes):
    a = shoelace(nodes)
    if abs(a) < 1e-9:
        n = float(len(nodes))
        return sum(p[0] for p in nodes) / n, sum(p[1] for p in nodes) / n
    cx = cy = 0.0
    for i in range(len(nodes)):
        x0, y0 = nodes[i]
        x1, y1 = nodes[(i + 1) % len(nodes)]
        cross = x0 * y1 - x1 * y0
        cx += (x0 + x1) * cross
        cy += (y0 + y1) * cross
    return cx / (6.0 * a), cy / (6.0 * a)


def point_in_poly(px, py, nodes):
    inside = False
    j = len(nodes) - 1
    for i in range(len(nodes)):
        xi, yi = nodes[i]
        xj, yj = nodes[j]
        if (yi > py) != (yj > py):
            xint = xi + (py - yi) * (xj - xi) / (yj - yi)
            if px < xint:
                inside = not inside
        j = i
    return inside


def sample_poly(nodes, step=50):
    """Grid sample points inside the polygon (spawn mass approximation)."""
    xs = [p[0] for p in nodes]
    ys = [p[1] for p in nodes]
    pts = []
    for x in range(int(min(xs)) + step // 2, int(max(xs)), step):
        for y in range(int(min(ys)) + step // 2, int(max(ys)), step):
            if point_in_poly(x, y, nodes):
                pts.append((x, y))
    return pts


def band_of(max_level):
    for minl, maxl, gear in BANDS:
        if max_level <= maxl:
            return (minl, maxl, gear)
    return BANDS[-1]


def compass(x, y):
    # The L2 world axes: x east, y south. The math angle of the
    # direction from the village with north up decides the octant.
    dx, dy = x - VILLAGE_X, y - VILLAGE_Y
    if dx == 0 and dy == 0:
        return "C"
    sector = int(round(math.atan2(-dy, dx) / (math.pi / 4)))
    return COMPASS[sector % 8]


def partition_zone(poly, nodes, mobs_count):
    """Return [(cx, cy, half)] squares covering the polygon."""
    cx, cy = centroid(nodes)
    maxr = max(math.hypot(p[0] - cx, p[1] - cy) for p in nodes)
    if maxr <= SINGLE_MAX_R:
        half = int(math.ceil((maxr + HALF_MARGIN) / 50.0) * 50)
        return [(round(cx), round(cy), half)], [poly]

    area = abs(shoelace(nodes))
    if area < 1.0:
        area = 1.0
    density = mobs_count / area
    cell_area = CELL_MOBS_TARGET / max(density, 1e-9)
    cell = math.sqrt(cell_area)
    cell = max(CELL_MIN, min(CELL_MAX, cell))
    cell = int(round(cell / 100.0)) * 100
    half = cell // 2 + HALF_MARGIN

    pts = sample_poly(nodes)
    if not pts:
        return [], []
    xs = [p[0] for p in nodes]
    ys = [p[1] for p in nodes]
    minx, miny = min(xs), min(ys)
    maxx, maxy = max(xs), max(ys)
    squares = []
    covered = []
    cell_area = float(cell * cell)
    for gx in range(int(minx), int(maxx), cell):
        for gy in range(int(miny), int(maxy), cell):
            inside = [p for p in pts
                      if gx <= p[0] < gx + cell and gy <= p[1] < gy + cell]
            if not inside:
                continue
            if len(inside) * (step := 50 * 50) / cell_area < KEEP_FRAC:
                continue
            scx = sum(p[0] for p in inside) / len(inside)
            scy = sum(p[1] for p in inside) / len(inside)
            squares.append((round(scx), round(scy), half))
            covered.append(inside)
    return squares, covered


def territory_coverage(nodes, squares, step=50):
    """Fraction of the polygon spawn mass covered by the square union."""
    pts = sample_poly(nodes, step)
    if not pts:
        return 0.0, 0
    covered = 0
    for px, py in pts:
        for cx, cy, half in squares:
            if abs(px - cx) <= half and abs(py - cy) <= half:
                covered += 1
                break
    return covered / len(pts), len(pts)


def go_str(s):
    return '"' + s.replace('\\', '\\\\').replace('"', '\\"') + '"'


def survey_mode(min_level, max_level):
    """Print the band survey: the grounds, the mobs, the transport."""
    stats = load_npc_survey_stats()
    ground_names = {
        "The Town of Dion": "dion_town",
        "Execution Ground": "execution_ground",
        "The Center of the Cruma Marshlands": "cruma_center",
        "Cruma Marshlands": "cruma_edge",
        "Plains of Dion": "plains_south",
        "The Town of Gludio": "gludio_town",
    }
    anchors = {}
    for name, short in ground_names.items():
        entry = route_arrival(load_teleport_arrivals(), name)
        if entry is None:
            continue
        gatekeeper, x, y, fee = entry
        anchors[short] = (name, gatekeeper, x, y, fee)

    rows = []
    for path in glob.glob(SPAWNS_DIR + "/**/*.xml", recursive=True):
        rel = os.path.relpath(path, SPAWNS_DIR)
        try:
            root = ET.parse(path).getroot()
        except ET.ParseError:
            continue
        for spawn in root.iter("spawn"):
            terr = spawn.find("territory")
            if terr is None:
                continue
            nodes = [(int(n.get("x")), int(n.get("y")))
                     for n in terr.findall("node")]
            if not nodes:
                continue
            mobs, band_mass, total = [], 0, 0
            for npc in spawn.findall("npc"):
                tid = int(npc.get("id"))
                count = int(npc.get("count"))
                info = stats.get(tid)
                if info is None:
                    continue
                mobs.append((tid, count, info))
                total += count
                if min_level <= info["level"] <= max_level:
                    band_mass += count
            if band_mass == 0 or not mobs:
                continue
            cx = sum(p[0] for p in nodes) / len(nodes)
            cy = sum(p[1] for p in nodes) / len(nodes)
            near, dist = None, None
            for short, (_, _, ax, ay, _) in anchors.items():
                d = math.hypot(cx - ax, cy - ay)
                if dist is None or d < dist:
                    near, dist = short, d
            rows.append((rel, spawn.get("zone") or "?", cx, cy, near,
                         dist, band_mass, total, mobs))

    rows.sort(key=lambda r: (r[4] if r[4] else "zzz", r[5] or 0))
    print("band %d-%d survey: %d territories" % (min_level, max_level,
                                                 len(rows)))
    print("anchors: " + "; ".join(
        "%s (%s, %d %d, fee %d)" % (k, v[1], v[2], v[3], v[4])
        for k, v in sorted(anchors.items())))
    print()
    for rel, zone, cx, cy, near, dist, band_mass, total, mobs in rows:
        print("%-42s %-24s (%7.0f,%7.0f) walk=%s %.0f band %d/%d" %
              (rel, zone, cx, cy, near, dist or 0, band_mass, total))
        for tid, count, info in sorted(mobs, key=lambda m: -m[2]["level"]):
            flag = "*" if min_level <= info["level"] <= max_level else " "
            aggro = ("aggro %d" % info["aggro_range"]) if info[
                "aggressive"] else "passive"
            print("  %s lvl %2d x%-3d hp %5d exp %4d %-10s clan %d "
                  "%s [%d]" % (flag, info["level"], count, info["hp"],
                               info["exp"], aggro, info["clan_help"],
                               info["name"], tid))


def main():
    if len(sys.argv) == 4 and sys.argv[1] == "--survey":
        survey_mode(int(sys.argv[2]), int(sys.argv[3]))
        return
    stats = load_npc_stats()
    root = ET.parse(SPAWN_XML).getroot()

    territories = []
    for spawn in root.iter("spawn"):
        terr = spawn.find("territory")
        nodes = [(int(n.get("x")), int(n.get("y")))
                 for n in terr.findall("node")]
        mobs = []
        total = 0
        for npc in spawn.findall("npc"):
            tid = int(npc.get("id"))
            count = int(npc.get("count"))
            level, name = stats.get(tid, (0, "npc%d" % tid))
            mobs.append((tid, name, level, count))
            total += count
        max_level = max(m[2] for m in mobs)
        min_level = min(m[2] for m in mobs)
        territories.append({
            "zone": spawn.get("zone"), "nodes": nodes, "mobs": mobs,
            "total": total, "max_level": max_level, "min_level": min_level,
            "band": band_of(max_level),
        })

    # Skip the sub territories fully covered by a same band parent:
    # their polygons fold into the parent squares (the mobs spawn inside
    # the same ground) and duplicating squares at the same spot only
    # makes the rotation bounce between identical grounds.
    kept = []
    for t in territories:
        covered = False
        for other in territories:
            if other is t or other["band"] != t["band"]:
                continue
            if other["total"] < t["total"]:
                continue
            if abs(shoelace(t["nodes"])) >= abs(shoelace(other["nodes"])):
                continue
            pts = sample_poly(t["nodes"], 100)
            if not pts:
                continue
            inside = sum(1 for p in pts
                         if point_in_poly(p[0], p[1], other["nodes"]))
            if inside / len(pts) >= 0.85:
                covered = True
                break
        if not covered:
            kept.append(t)

    # Generate the squares of every kept territory.
    entries = []
    total_mass = 0
    covered_mass = 0
    for t in kept:
        squares, _ = partition_zone(t, t["nodes"], t["total"])
        if not squares:
            continue
        frac, mass = territory_coverage(t["nodes"], squares)
        total_mass += mass
        covered_mass += mass * frac
        if len(squares) > 1:
            # label the cells like a chess board for readable ids
            labels = {}
            for i, (cx, cy, half) in enumerate(squares):
                labels[(cx, cy)] = chr(ord('a') + i % 8) + str(i // 8 + 1)
        for cx, cy, half in squares:
            dominant = max(t["mobs"], key=lambda m: (m[3], m[2]))
            short = t["zone"].split("_", 1)[1]
            if len(squares) > 1:
                name = "%s %s-%s" % (
                    dominant[1], compass(cx, cy), labels[(cx, cy)])
                zid = "elven-%s-%s" % (short, labels[(cx, cy)])
            else:
                name = "%s %s" % (dominant[1], compass(cx, cy))
                zid = "elven-%s" % short
            entries.append({
                "id": zid, "name": name, "band": t["band"],
                "cx": cx, "cy": cy, "half": half, "mobs": t["mobs"],
                "frac": frac, "terr": t["zone"],
            })

    # Sort by band, then by distance from the village: the first entry
    # is the starter fallback square of the fresh characters.
    entries.sort(key=lambda e: (
        BANDS.index(e["band"]),
        math.hypot(e["cx"] - VILLAGE_X, e["cy"] - VILLAGE_Y)))

    lines = []
    lines.append("// SPDX-FileCopyrightText: 2026 Melg Eight "
                 "<public.melg8@gmail.com>")
    lines.append("//")
    lines.append("// SPDX-License-Identifier: MIT")
    lines.append("")
    lines.append("// Code generated by tools/generate_hunt_zones.py from "
                 "the Mobius C1")
    lines.append("// spawn data (ElvenStarting.xml); DO NOT EDIT by hand - "
                 "re-run the")
    lines.append("// generator instead. The registry covers the spawn "
                 "polygons with one")
    lines.append("// or several compact squares per territory, every "
                 "square carrying")
    lines.append("// the full mob list of its territory (all species "
                 "farmed, the level")
    lines.append("// sorted priorities bias the engage toward the exp "
                 "rich mobs).")
    lines.append("")
    lines.append("package hunt")
    lines.append("")
    lines.append("// elvenHuntingZones ladders the elven lands in the ten "
                 "mob level")
    lines.append("// bands over %d compact squares generated from the %d "
                 "kept spawn" % (len(entries), len(kept)))
    lines.append("// territories of ElvenStarting.xml (the same-band "
                 "sub territories fold")
    lines.append("// into their parents). The squares sit on the real "
                 "spawn ground: the")
    lines.append("// mobs of a territory spawn at uniformly random points "
                 "of its polygon")
    lines.append("// (NpcSpawnTerritory.getRandomPoint) and the wander "
                 "stays inside the")
    lines.append("// polygon, so the measured spawn mass coverage of "
                 "this registry is")
    lines.append("// %.0f%% (the hand placed squares of the previous "
                 "registry covered" % (100.0 * covered_mass / total_mass))
    lines.append("// 18%). The first entry is the starter fallback of "
                 "the picker (the")
    lines.append("// village nearest square of the 1-3 band).")
    lines.append("var elvenHuntingZones = []HuntingZone{")
    for e in entries:
        minl, maxl, gear = e["band"]
        lines.append("\t{")
        lines.append("\t\tID: %s, Name: %s," % (go_str(e["id"]),
                                                 go_str(e["name"])))
        lines.append("\t\tRegion: regionElven, MinLevel: %d, MaxLevel: %d,"
                     " MinGear: %d," % (minl, maxl, gear))
        lines.append("\t\tCX: %d, CY: %d, Half: %d," %
                     (e["cx"], e["cy"], e["half"]))
        lines.append("\t\tMobs: []ZoneMob{")
        min_mob_level = min(m[2] for m in e["mobs"])
        for tid, name, level, count in sorted(e["mobs"],
                                              key=lambda m: (-m[2], m[0])):
            lines.append("\t\t\t{")
            lines.append("\t\t\t\tTemplateID: %d, Name: %s," %
                         (tid, go_str(name)))
            lines.append("\t\t\t\tLevel: %d, Count: %d, Priority: %d," %
                         (level, count, level - min_mob_level))
            lines.append("\t\t\t},")
        lines.append("\t\t},")
        lines.append("\t},")
    lines.append("}")
    lines.append("")
    lines.append("// ElvenHuntingZones returns the hunting grounds of the "
                 "elven lands.")
    lines.append("func ElvenHuntingZones() []HuntingZone {")
    lines.append("\tzones := make([]HuntingZone, len(elvenHuntingZones))")
    lines.append("\tcopy(zones, elvenHuntingZones)")
    lines.append("")
    lines.append("\treturn zones")
    lines.append("}")

    with open(OUT, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")

    # Report.
    print("territories kept: %d of %d (same band sub folds)" %
          (len(kept), len(territories)))
    print("zones: %d" % len(entries))
    by_band = {}
    for e in entries:
        by_band.setdefault(e["band"], []).append(e)
    for band in BANDS:
        es = by_band.get(band, [])
        if not es:
            continue
        halves = [e["half"] for e in es]
        print("  band %2d-%2d gear %3d: %3d squares, half %d-%d" %
              (band[0], band[1], band[2], len(es),
               min(halves), max(halves)))
    low = [e for e in entries if e["frac"] < 0.8]
    print("territory spawn mass coverage: %.0f%% (%d territories below "
          "80%%)" % (100.0 * covered_mass / total_mass, len(low)))
    for e in low:
        print("  low coverage %s -> %.0f%% (%s)" %
              (e["terr"], e["frac"] * 100, e["id"]))
    print("written: %s" % OUT)


if __name__ == "__main__":
    main()
