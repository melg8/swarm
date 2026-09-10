#!/usr/bin/env python3
"""Generate the elven hunting SPOT registry (visibility-bounded anchors).

Why: the square zone registry (227 squares of generate_hunt_zones.py)
covers the spawn ground, but the squares ignore the visibility geometry
(the Mobius WorldRegion grid guarantees only ~2048 units of knownlist
around the character), overlap each other by 64 percent inside one
level band and rotate on a 10 second timer that loses against the
15-20 second respawn clock. The spot registry replaces the squares with
the anchors the spawn mass actually clusters around: cluster the
registry squares whose centers share the ground (grid adjacency),
emit one Spot per cluster with the mass weighted centroid as the
anchor and the radius clamped to the guaranteed visible circle of the
anchor. A dense territory yields several neighboring spots, a sparse
territory yields one spot anchored on the mass centroid (see
docs/hunting_system_redesign.md).

Inputs (env-overridable):
  MOBIUS_C1  the L2J_Mobius_C1_HarbingersOfWar dist tree; when present
             the spawn XML provides the real respawn windows per npc
             (respawn + respawnRandom attributes), otherwise the live
             measured elven window (15-20 s, AGENTS.md) applies
  REGISTRY   the generated zone registry to parse (the mass source)
  OUT        the generated Go file path
  JSON_OUT   the JSON twin for the visualization tool

Output: deterministic Go source (spots_elven.go) + JSON data file.
"""

import json
import math
import os
import re
import xml.etree.ElementTree as ET

MOBIUS_C1 = os.environ.get(
    "MOBIUS_C1",
    "/home/z/my-project/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar")
SPAWN_XML = os.path.join(
    MOBIUS_C1, "dist/game/data/spawns/ElvenTerritory/ElvenStarting.xml")
REGISTRY = os.environ.get(
    "REGISTRY",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                 "internal/swarm/hunt/zones_elven.go"))
OUT = os.environ.get(
    "OUT",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                 "internal/swarm/hunt/spots_elven.go"))
JSON_OUT = os.environ.get(
    "JSON_OUT",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                 "docs/hunt_analysis/spots_elven.json"))

# The anchor of the registry ordering: the elven village surroundings.
# The first spot of the sorted registry is the starter fallback of a
# fresh character (the village nearest spot of the lowest band).
VILLAGE_X, VILLAGE_Y = 46112, 41500

# The guaranteed visible radius of a standing character: the Mobius
# world is a grid of 2048x2048 WorldRegion cells and a client receives
# the objects of its own region plus the 8 adjacent ones, so everything
# within ~2048 units of the anchor is inside the knownlist by
# construction. A spot never grows past this circle.
VISIBILITY_RADIUS = 2048

# The clustering grid: territory anchors whose centers land in the
# same or in 8-adjacent CELL-unit cells share the ground and merge
# into one cluster before the visibility split (the same-band sub
# territories of the registry overlap by 64 percent, the parent
# polygons absorb most of them). The split - not the merge - is what
# shapes the final spots: every spot must fit the visibility circle,
# and a registry square of half 1300-1900 already fills most of it,
# so the spots settle at the territory granularity (~72 anchors for
# 73 territories, a 3.2x reduction of the 227 square decision space
# with the same-band overlap gone).
CELL = 2048

# SPOT_MIN_MASS drops the singleton leftovers: a spot worth anchoring
# holds at least two mobs of expected supply.
SPOT_MIN_MASS = 2.0

# The radius margin: the radius of a spot reaches past the farthest
# square center by its own half plus this margin, so the ground the
# square covers stays inside the spot circle.
RADIUS_MARGIN = 256

# The leash square of a spot is inscribed in the visibility circle
# (half = radius / sqrt(2)): every point the engage square covers stays
# within the guaranteed visible circle of the anchor.
SQRT2 = math.sqrt(2.0)

# The live measured respawn window of the elven lands (AGENTS.md):
# 15-20 s per mob. The spawn XML respawn attributes override it per
# species when the Mobius tree is available.
DEFAULT_RESPAWN = (15, 20)

COMPASS = ["E", "NE", "N", "NW", "W", "SW", "S", "SE"]


def compass(x, y):
    """The octant of the direction from the elven village (north up)."""
    dx, dy = x - VILLAGE_X, y - VILLAGE_Y
    if dx == 0 and dy == 0:
        return "C"
    sector = int(round(math.atan2(-dy, dx) / (math.pi / 4)))

    return COMPASS[sector % 8]


# ---------------------------------------------------------------------------
# Mass sources: the committed zone registry plus the Mobius spawn XML
# respawn windows when the dist tree is available.
# ---------------------------------------------------------------------------

def parse_registry(path):
    """Parse the generated zone registry Go source into zone dicts."""
    with open(path, encoding="utf-8") as fh:
        src = fh.read()
    zones = []
    zone_re = re.compile(
        r'ID:\s*"(?P<id>[^"]+)",\s*Name:\s*"(?P<name>[^"]+)",\s*'
        r'Region:\s*regionElven,\s*MinLevel:\s*(?P<minl>\d+),\s*'
        r'MaxLevel:\s*(?P<maxl>\d+),\s*MinGear:\s*(?P<gear>\d+),\s*'
        r'CX:\s*(?P<cx>-?\d+),\s*CY:\s*(?P<cy>-?\d+),\s*Half:\s*(?P<half>\d+)',
    )
    mob_re = re.compile(
        r'TemplateID:\s*(?P<tid>\d+),\s*Name:\s*"(?P<name>[^"]+)",\s*'
        r'Level:\s*(?P<level>\d+),\s*Count:\s*(?P<count>\d+),\s*'
        r'Priority:\s*(?P<priority>\d+)',
    )
    # The generated registry indents every zone literal with one tab
    # and every mob literal with three tabs, so splitting at the
    # one-tab zone starts yields one chunk per zone with its full mob
    # list inside (the deeper indents never split).
    for block in src.split("\n\t{\n"):
        m = zone_re.search(block)
        if not m:
            continue
        zone = {
            "id": m.group("id"), "name": m.group("name"),
            "min_level": int(m.group("minl")),
            "max_level": int(m.group("maxl")),
            "cx": int(m.group("cx")), "cy": int(m.group("cy")),
            "half": int(m.group("half")), "mobs": [],
        }
        for mm in mob_re.finditer(block):
            zone["mobs"].append({
                "template_id": int(mm.group("tid")),
                "name": mm.group("name"),
                "level": int(mm.group("level")),
                "count": int(mm.group("count")),
                "respawn": DEFAULT_RESPAWN,
            })
        zones.append(zone)

    return zones


def load_respawn_windows():
    """Read the per-npc respawn windows of the spawn XML when available.

    Returns {} when the Mobius tree is absent (the offline fallback):
    the DEFAULT_RESPAWN window then applies to every species.
    """
    if not os.path.exists(SPAWN_XML):
        return {}
    windows = {}
    root = ET.parse(SPAWN_XML).getroot()
    for npc in root.iter("npc"):
        tid = int(npc.get("id"))
        base = npc.get("respawn")
        rnd = npc.get("respawnRandom", "0")
        if base is None:
            continue
        low = int(base)
        high = low + int(rnd)
        windows[tid] = (min(low, high), max(low, high))

    return windows


def territory_key(zone_id):
    """The spawn territory a registry square belongs to: the id prefix
    before the cell suffix (elven-2119_01-a1 -> 2119_01)."""
    return zone_id.split("-")[1]


def build_squares(zones, respawn_windows):
    """The mass carrying squares of the registry grouped by territory.

    Every zone square carries the full mob list of its territory, so
    the territory mass spreads over its squares: each square holds
    total/n_squares of it, concentrated at its center (the generator
    placed the squares at the centroids of the spawn ground they
    cover). Returns (squares [(cx, cy, half, share, zone)],
    territory square counts).
    """
    territories = {}
    for zone in zones:
        territories.setdefault(territory_key(zone["id"]), []).append(zone)

    squares = []
    terr_counts = {}
    for terr, tzones in territories.items():
        terr_counts[terr] = len(tzones)
        species = {}
        for zone in tzones:
            for mob in zone["mobs"]:
                species[mob["template_id"]] = mob
        total = sum(m["count"] for m in species.values())
        if total <= 0:
            continue
        share = total / float(len(tzones))
        for zone in tzones:
            for mob in zone["mobs"]:
                mob["respawn"] = respawn_windows.get(
                    mob["template_id"], DEFAULT_RESPAWN)
            squares.append((zone["cx"], zone["cy"], zone["half"],
                            share, zone))

    return squares, terr_counts


def territory_anchors(zones):
    """The mass weighted center of every territory: the anchor its
    squares distribute around, weighted by the per square share."""
    territories = {}
    for zone in zones:
        territories.setdefault(territory_key(zone["id"]), []).append(zone)
    anchors = {}
    for terr, tzones in territories.items():
        species = {}
        for zone in tzones:
            for mob in zone["mobs"]:
                species[mob["template_id"]] = mob
        total = sum(m["count"] for m in species.values())
        share = total / float(len(tzones)) if tzones else 0.0
        mass = share * len(tzones)
        cx = sum(z["cx"] for z in tzones) / float(len(tzones))
        cy = sum(z["cy"] for z in tzones) / float(len(tzones))
        anchors[terr] = {"cx": cx, "cy": cy, "mass": mass,
                         "zones": tzones}

    return anchors


# ---------------------------------------------------------------------------
# Clustering: grid adjacency of the territory anchors, radius clamping.
# ---------------------------------------------------------------------------

def clusters_of_anchors(anchors):
    """The grid adjacency clusters of the territory anchors: the
    centers landing in the same or in 8-adjacent CELL cells share the
    ground. Returns lists of territory keys."""
    cells = {}
    for terr, info in anchors.items():
        cells.setdefault((int(info["cx"]) // CELL,
                          int(info["cy"]) // CELL), []).append(terr)
    seen = set()
    clusters = []
    for start in cells:
        if start in seen:
            continue
        stack = [start]
        seen.add(start)
        terrs = []
        while stack:
            gx, gy = stack.pop()
            terrs.extend(cells[(gx, gy)])
            for dx in (-1, 0, 1):
                for dy in (-1, 0, 1):
                    nxt = (gx + dx, gy + dy)
                    if nxt in cells and nxt not in seen:
                        seen.add(nxt)
                        stack.append(nxt)
        clusters.append(terrs)

    return clusters


def cluster_anchor(terrs, anchors):
    """The mass weighted centroid of a territory cluster."""
    mass = sum(anchors[t]["mass"] for t in terrs)
    if mass <= 0:
        n = float(len(terrs))
        return sum(anchors[t]["cx"] for t in terrs) / n, \
            sum(anchors[t]["cy"] for t in terrs) / n, mass
    cx = sum(anchors[t]["cx"] * anchors[t]["mass"] for t in terrs) / mass
    cy = sum(anchors[t]["cy"] * anchors[t]["mass"] for t in terrs) / mass

    return cx, cy, mass


def cluster_radius(cx, cy, terrs, anchors):
    """The radius reaching every square of the cluster's territories:
    the distance to the farthest square center plus its own half (the
    ground the square covers) plus the margin."""
    radius = 0.0
    for t in terrs:
        for zone in anchors[t]["zones"]:
            radius = max(radius, math.hypot(
                zone["cx"] - cx, zone["cy"] - cy) + zone["half"])

    return radius + RADIUS_MARGIN


def split_cluster(cx, cy, terrs, anchors):
    """Split an oversized cluster by the farthest-territory heuristic:
    the farthest territory anchor and the centroid become two seeds,
    every territory joins the nearer seed. Returns the two lists."""
    far = max(terrs, key=lambda t: math.hypot(
        anchors[t]["cx"] - cx, anchors[t]["cy"] - cy))
    fx, fy = anchors[far]["cx"], anchors[far]["cy"]
    left, right = [], []
    for t in terrs:
        tx, ty = anchors[t]["cx"], anchors[t]["cy"]
        if math.hypot(tx - cx, ty - cy) <= math.hypot(tx - fx, ty - fy):
            left.append(t)
        else:
            right.append(t)

    return left, right


def spot_candidates(anchors, clusters):
    """The spot list of the territory clusters: the oversized ones
    split recursively until every spot fits the visibility circle, the
    undersized ones (mass < SPOT_MIN_MASS) drop out."""
    spots = []
    queue = [terrs for terrs in clusters if terrs]
    while queue:
        terrs = queue.pop(0)
        if not terrs:
            continue
        cx, cy, mass = cluster_anchor(terrs, anchors)
        radius = cluster_radius(cx, cy, terrs, anchors)
        if radius > VISIBILITY_RADIUS and len(terrs) > 1:
            left, right = split_cluster(cx, cy, terrs, anchors)
            if left and right:
                queue.append(left)
                queue.append(right)
                continue
            # The split degenerated: clamp hard, the leash inscribes
            # inside the circle anyway.
            radius = VISIBILITY_RADIUS
        if mass < SPOT_MIN_MASS:
            continue
        spots.append({
            "cx": cx, "cy": cy, "radius": min(radius, VISIBILITY_RADIUS),
            "mass": mass, "territories": terrs,
            "terr_zones": {t: anchors[t]["zones"] for t in terrs},
        })

    return spots


def assign_squares(spots, anchors, squares):
    """Assign every registry square to the spot whose LEASH square (the
    inscribed square of the visibility circle, half = radius /
    sqrt(2)) contains the square center first, the nearest spot
    anchor otherwise (the tail squares between the spots). The
    assignment only drives the leash coverage metric, the composition
    counts the cluster partition."""
    for spot in spots:
        spot["zones"] = []
    for cx, cy, _half, _share, zone in squares:
        best = None
        best_dist = None
        inside = None
        for spot in spots:
            half = spot["radius"] / SQRT2
            dist = math.hypot(spot["cx"] - cx, spot["cy"] - cy)
            if abs(spot["cx"] - cx) <= half and abs(spot["cy"] - cy) <= half:
                if inside is None or dist < math.hypot(
                        inside["cx"] - cx, inside["cy"] - cy):
                    inside = spot
            if best_dist is None or dist < best_dist:
                best, best_dist = spot, dist
        target = inside if inside is not None else best
        if target is not None:
            target["zones"].append(zone)


def build_spots(spots, terr_counts):
    """The final spot records: composition, levels, respawn window.

    The composition counts the cluster partition: a territory of the
    cluster contributes its full species list, a territory the
    recursion split between two spots contributes proportionally to
    its squares on each side. The species mass therefore sums to the
    spawn mass exactly (up to the integer rounding)."""
    result = []
    for index, spot in enumerate(spots):
        species = {}
        for terr in spot["territories"]:
            # The full territory mass lands when every square of it
            # belongs to this spot, the proportional share otherwise
            # (the split partitioned the squares, not the polygons).
            zones_here = [z for z in spot["zones"]
                          if territory_key(z["id"]) == terr]
            all_zones = spot["terr_zones"].get(terr, [])
            here = len(zones_here)
            total_sq = len(all_zones)
            frac = here / float(total_sq) if total_sq > 0 else 0.0
            for mob in all_zones[0]["mobs"] if all_zones else []:
                key = mob["template_id"]
                entry = species.setdefault(key, {
                    "template_id": key, "name": mob["name"],
                    "level": mob["level"], "count": 0.0,
                    "respawn": mob["respawn"],
                })
                entry["count"] += mob["count"] * frac
        mobs = sorted(species.values(),
                      key=lambda m: (-m["level"], m["template_id"]))
        # Drop the trace species (a rounded count of zero): the spawn
        # mass keeps their share in the totals, the composition lists
        # the species the ground actually fields.
        mobs = [m for m in mobs if int(round(m["count"])) >= 1]
        total = sum(m["count"] for m in mobs)
        # The composition floor mirrors SPOT_MIN_MASS: dropping the
        # trace species can sink a marginal cluster below it.
        if total < SPOT_MIN_MASS or not mobs:
            continue
        levels = [m["level"] for m in mobs if m["count"] >= 0.5]
        if not levels:
            levels = [m["level"] for m in mobs]
        # The dominant respawn window: the count weighted midpoint.
        rlow = sum(m["count"] * m["respawn"][0] for m in mobs) / total
        rhigh = sum(m["count"] * m["respawn"][1] for m in mobs) / total
        dominant = max(mobs, key=lambda m: (m["count"], m["level"]))
        result.append({
            "id": "elven-spot-%02d" % (index + 1,),
            "name": "%s %s" % (dominant["name"], compass(spot["cx"], spot["cy"])),
            "region": "elven",
            "anchor_x": int(round(spot["cx"])),
            "anchor_y": int(round(spot["cy"])),
            "radius": int(round(min(spot["radius"], VISIBILITY_RADIUS))),
            "min_level": min(levels),
            "max_level": max(levels),
            "respawn_min": int(round(rlow)),
            "respawn_max": int(round(max(rhigh, rlow + 1))),
            "mass": round(total, 1),
            "mobs": [{
                "template_id": m["template_id"], "name": m["name"],
                "level": m["level"], "count": int(round(m["count"])),
                "respawn_min": m["respawn"][0],
                "respawn_max": m["respawn"][1],
            } for m in mobs],
            "zone_ids": sorted(z["id"] for z in spot["zones"]),
        })

    return result


def go_str(s):
    return '"' + s.replace('\\', '\\\\').replace('"', '\\"') + '"'


def emit_go(spots, source):
    lines = []
    lines.append("// SPDX-FileCopyrightText: 2026 Melg Eight "
                 "<public.melg8@gmail.com>")
    lines.append("//")
    lines.append("// SPDX-License-Identifier: MIT")
    lines.append("")
    lines.append("// Code generated by tools/generate_hunt_spots.py %s;" % source)
    lines.append("// DO NOT EDIT by hand - re-run the generator instead. The")
    lines.append("// spot registry replaces the square zone ladder of")
    lines.append("// zones_elven.go: every spot is an anchor on the spawn mass")
    lines.append("// (the grid adjacency clustering of the registry squares) with")
    lines.append("// a visibility-bounded radius (<= 2048, the guaranteed")
    lines.append("// knownlist circle of a standing character), the composition")
    lines.append("// of the ground it covers and the respawn window of its mobs.")
    lines.append("// See docs/hunting_system_redesign.md for the model.")
    lines.append("")
    lines.append("package hunt")
    lines.append("")
    lines.append("// elvenHuntingSpots anchors the elven lands in %d hunting" % len(spots))
    lines.append("// spots. The first entry is the starter fallback of the")
    lines.append("// spot picker (the village nearest spot of the lowest band).")
    lines.append("var elvenHuntingSpots = []Spot{")
    for spot in spots:
        lines.append("\t{")
        lines.append("\t\tID: %s, Name: %s," % (go_str(spot["id"]),
                                                go_str(spot["name"])))
        lines.append("\t\tRegion: regionElven, MinLevel: %d, MaxLevel: %d," %
                     (spot["min_level"], spot["max_level"]))
        lines.append("\t\tAnchorX: %d, AnchorY: %d, Radius: %d," %
                     (spot["anchor_x"], spot["anchor_y"], spot["radius"]))
        lines.append("\t\tRespawnMin: %d, RespawnMax: %d, Mass: %s," %
                     (spot["respawn_min"], spot["respawn_max"],
                      ("%.1f" % spot["mass"])))
        lines.append("\t\tMobs: []SpotMob{")
        for mob in spot["mobs"]:
            lines.append("\t\t\t{")
            lines.append("\t\t\t\tTemplateID: %d, Name: %s," %
                         (mob["template_id"], go_str(mob["name"])))
            lines.append("\t\t\t\tLevel: %d, Count: %d, RespawnMin: %d, "
                         "RespawnMax: %d," %
                         (mob["level"], mob["count"], mob["respawn_min"],
                          mob["respawn_max"]))
            lines.append("\t\t\t},")
        lines.append("\t\t},")
        lines.append("\t},")
    lines.append("}")
    lines.append("")
    lines.append("// ElvenHuntingSpots returns the hunting spots of the elven lands.")
    lines.append("func ElvenHuntingSpots() []Spot {")
    lines.append("\tspots := make([]Spot, len(elvenHuntingSpots))")
    lines.append("\tcopy(spots, elvenHuntingSpots)")
    lines.append("")
    lines.append("\treturn spots")
    lines.append("}")

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines) + "\n")


def main():
    zones = parse_registry(REGISTRY)
    respawn_windows = load_respawn_windows()
    source = "from the Mobius spawn XML" if respawn_windows else \
        "from the committed zone registry"
    squares, terr_counts = build_squares(zones, respawn_windows)
    anchors = territory_anchors(zones)
    clusters = clusters_of_anchors(anchors)
    spots = spot_candidates(anchors, clusters)
    assign_squares(spots, anchors, squares)
    result = build_spots(spots, terr_counts)

    # Sort by band, then by distance from the village: the first entry
    # is the starter fallback spot of the fresh characters.
    result.sort(key=lambda s: (
        s["min_level"], s["max_level"],
        math.hypot(s["anchor_x"] - VILLAGE_X, s["anchor_y"] - VILLAGE_Y)))

    emit_go(result, source)

    total_mass = sum(s["mass"] for s in result)
    composition = sum(m["count"] for s in result for m in s["mobs"])
    spawn_mass = sum(sq[3] for sq in squares)
    # The leash coverage: the fraction of the spawn mass whose square
    # center lies inside the leash square of its spot (the composition
    # counts the cluster partition, this measures the farm area).
    inside_mass = 0.0
    for spot in spots:
        half = spot["radius"] / SQRT2
        for z in spot["zones"]:
            if abs(spot["cx"] - z["cx"]) <= half and \
                    abs(spot["cy"] - z["cy"]) <= half:
                terr = territory_key(z["id"])
                terr_zones = spot["terr_zones"].get(terr)
                if not terr_zones:
                    # A tail square of a foreign territory assigned by
                    # the nearest fallback: its mass counts there.
                    continue
                terr_sq = terr_counts.get(terr, 1)
                terr_total = sum(
                    m["count"] for m in terr_zones[0]["mobs"])
                inside_mass += terr_total / float(terr_sq)
    payload = {
        "generated": "tools/generate_hunt_spots.py",
        "source": source,
        "village": {"x": VILLAGE_X, "y": VILLAGE_Y},
        "visibilityRadius": VISIBILITY_RADIUS,
        "spotCount": len(result),
        "spawnMass": round(spawn_mass, 1),
        "compositionMass": round(composition, 1),
        "respawnDefault": {"min": DEFAULT_RESPAWN[0], "max": DEFAULT_RESPAWN[1]},
        "spots": result,
    }
    with open(JSON_OUT, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=1, sort_keys=False)

    by_band = {}
    for spot in result:
        key = (spot["min_level"], spot["max_level"])
        by_band.setdefault(key, []).append(spot)
    print("source: %s" % source)
    print("zones parsed: %d, territories: %d, clusters: %d" %
          (len(zones), len(anchors), len(clusters)))
    print("spots: %d (registry squares: %d, %.1fx reduction)" %
          (len(result), len(zones), len(zones) / max(1, len(result))))
    for band in sorted(by_band):
        es = by_band[band]
        radii = [e["radius"] for e in es]
        print("  band %2d-%2d: %2d spots, radius %d-%d, respawn %d-%ds" %
              (band[0], band[1], len(es), min(radii), max(radii),
               es[0]["respawn_min"], es[0]["respawn_max"]))
    print("spawn mass: %.1f mobs; spot composition: %.1f (%.0f%%), "
          "leash inside: %.1f (%.0f%%)" %
          (spawn_mass, composition, 100.0 * composition /
           max(1.0, spawn_mass), inside_mass, 100.0 * inside_mass /
           max(1.0, spawn_mass)))
    print("total cluster mass: %.1f" % total_mass)
    print("written: %s" % OUT)
    print("written: %s" % JSON_OUT)


if __name__ == "__main__":
    main()
