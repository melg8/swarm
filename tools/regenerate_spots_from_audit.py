#!/usr/bin/env python3
"""Regenerate the elven hunting SPOT registry from the Mobius spawn
territories, validated by the live spot audit.

Why: the committed registry was clustered from the square zone
registry, whose squares themselves were derived from the spawn
polygons through two lossy steps - the 2026-09-13 live audit
(docs/hunt_analysis/spot_audit.json, all 71 anchors probed on the
running stack) measured the leash coverage at 25-40 percent of the
visible mobs everywhere: the anchors sit off the real mass and the
leash squares (half <= 1448, inscribed in the 2048 visibility circle)
can never cover a territory whose polygon spans 3000-9500 units.

The regeneration goes back to the primary source - the spawn
territory polygons of the Mobius XML (the ground every mob of the
territory spawns and wanders on) - and cuts it into visibility-sized
pieces:

  1. every territory polygon is sampled on a dense grid (the spawn
     distribution is uniform over the polygon area);
  2. the sample cloud splits recursively (median cut on the widest
     axis) until every piece fits the leash square - the Chebyshev
     extent from the piece centroid stays under LEASH_HALF, so every
     point the leash covers sits inside the guaranteed knownlist
     circle of the anchor by construction;
  3. every piece becomes one spot: the anchor is the sample centroid,
     the radius is sqrt(2) * the piece Chebyshev extent (the circle
     that inscribes the leash square, clamped to 2048), the mob
     composition is the territory species list with the counts
     distributed by the area share of the piece.

The live audit cross-checks every generated spot: the mobs observed
at the nearest audited anchor are re-tested against the new leash -
the report (docs/hunt_analysis/spot_audit_changes.txt) prints the old
vs new in-leash counts, and tools also emits suggested_anchors.json
for the verification audit pass (-audit-anchors) of the new geometry.

Inputs (env-overridable):
  MOBIUS_C1   the L2J_Mobius_C1_HarbingersOfWar dist tree
  REGISTRY    the current spots_elven.go (the species name/level
              source and the old geometry for the change report)
  AUDIT       the live audit evidence JSON
  OUT         the generated Go file path
  JSON_OUT    the JSON twin for the visualization tool
  ANCHORS_OUT the suggested anchors JSON of the verification pass
  REPORT      the human readable change report

Output: deterministic Go source (spots_elven.go), the JSON twins and
the change report. Re-run after any audit refresh.
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
HERE = os.path.dirname(os.path.abspath(__file__))
REGISTRY = os.environ.get(
    "REGISTRY",
    os.path.join(HERE, "..", "internal", "swarm", "hunt", "spots_elven.go"))
AUDIT = os.environ.get(
    "AUDIT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "spot_audit.json"))
OUT = os.environ.get(
    "OUT",
    os.path.join(HERE, "..", "internal", "swarm", "hunt", "spots_elven.go"))
JSON_OUT = os.environ.get(
    "JSON_OUT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "spots_elven.json"))
ANCHORS_OUT = os.environ.get(
    "ANCHORS_OUT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "suggested_anchors.json"))
REPORT = os.environ.get(
    "REPORT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "spot_audit_changes.txt"))

# The elven village: the ordering anchor of the registry (the starter
# fallback of a fresh character is the village nearest spot of the
# lowest band).
VILLAGE_X, VILLAGE_Y = 46112, 41500

# The guaranteed visible radius of a standing character (the Mobius
# world grid broadcasts the own region plus the 8 adjacent ones).
VISIBILITY_RADIUS = 2048

# The leash square of a spot is inscribed in the visibility circle
# (half = radius / sqrt(2)): every point the leash covers stays within
# the guaranteed knownlist circle of the anchor.
SQRT2 = math.sqrt(2.0)
LEASH_HALF = int(round(VISIBILITY_RADIUS / SQRT2))

# The polygon sampling grid: the spawn distribution is uniform over
# the polygon area, a 128 unit grid resolves the pieces finely enough
# while a full elven territory holds a few hundred samples.
SAMPLE_STEP = 128

# A piece smaller than this Chebyshev extent is not worth its own
# anchor (the wander jitter covers it from the neighbor).
MIN_PIECE_EXTENT = 600

# The live measured respawn window of the elven lands (AGENTS.md):
# 15-20 s per mob.
DEFAULT_RESPAWN = (15, 20)

COMPASS = ["E", "NE", "N", "NW", "W", "SW", "S", "SE"]


def compass(x, y, ref_x, ref_y):
    """The octant of the direction from a reference point (north up)."""
    dx, dy = x - ref_x, y - ref_y
    if dx == 0 and dy == 0:
        return "C"
    sector = int(round(math.atan2(-dy, dx) / (math.pi / 4)))

    return COMPASS[sector % 8]


def parse_registry_species(path):
    """Parse the committed spot registry into the species table
    (template id -> name, level) and the old spot list for the change
    report."""
    with open(path, encoding="utf-8") as fh:
        src = fh.read()
    species = {}
    spot_re = re.compile(
        r'ID:\s*"(?P<id>[^"]+)",\s*Name:\s*"(?P<name>[^"]+)",\s*'
        r'Region:\s*regionElven,\s*MinLevel:\s*(?P<minl>\d+),\s*'
        r'MaxLevel:\s*(?P<maxl>\d+),\s*'
        r'AnchorX:\s*(?P<ax>-?\d+),\s*AnchorY:\s*(?P<ay>-?\d+),\s*'
        r'Radius:\s*(?P<radius>\d+),\s*'
        r'RespawnMin:\s*(?P<rmin>\d+),\s*RespawnMax:\s*(?P<rmax>\d+),\s*'
        r'Mass:\s*(?P<mass>[\d.]+)',
    )
    mob_re = re.compile(
        r'TemplateID:\s*(?P<tid>\d+),\s*Name:\s*"(?P<name>[^"]+)",\s*'
        r'Level:\s*(?P<level>\d+),\s*Count:\s*(?P<count>\d+),\s*'
        r'RespawnMin:\s*(?P<rmin>\d+),\s*RespawnMax:\s*(?P<rmax>\d+)',
    )
    spots = []
    for block in src.split("\n\t{\n"):
        m = spot_re.search(block)
        if not m:
            continue
        spot = {
            "id": m.group("id"), "name": m.group("name"),
            "min_level": int(m.group("minl")),
            "max_level": int(m.group("maxl")),
            "anchor_x": int(m.group("ax")), "anchor_y": int(m.group("ay")),
            "radius": int(m.group("radius")), "mobs": [],
        }
        for mm in mob_re.finditer(block):
            tid = int(mm.group("tid"))
            species[tid] = {
                "name": mm.group("name"), "level": int(mm.group("level")),
            }
            spot["mobs"].append({
                "template_id": tid, "count": int(mm.group("count")),
            })
        spots.append(spot)

    return species, spots


def load_species_from_stats(npc_ids):
    """The species table (id -> name, level) of the spawn npc ids from
    the Mobius npc stats XML: the authoritative name and level of
    every npc the spawn territories reference."""
    stats_dir = os.path.join(
        MOBIUS_C1, "dist", "game", "data", "stats", "npcs")
    species = {}
    wanted = set(npc_ids)
    for entry in sorted(os.listdir(stats_dir)):
        if not entry.endswith(".xml"):
            continue
        root = ET.parse(os.path.join(stats_dir, entry)).getroot()
        for npc in root.iter("npc"):
            npc_id = int(npc.get("id"))
            if npc_id not in wanted:
                continue
            species[npc_id] = {
                "name": npc.get("name"), "level": int(npc.get("level")),
            }
    missing = wanted - set(species)
    if missing:
        raise SystemExit(
            "error: the stats XML holds no entry for npcs %s"
            % sorted(missing))

    return species


def load_territories():
    """The spawn territories of the elven starting grounds: the
    polygon nodes and the npc spawns (id, count) of every zone."""
    root = ET.parse(SPAWN_XML).getroot()
    territories = []
    for spawn in root.iter("spawn"):
        terr = spawn.find("territory")
        if terr is None:
            continue
        nodes = [(int(n.get("x")), int(n.get("y")))
                 for n in terr.findall("node")]
        if len(nodes) < 3:
            continue
        npcs = []
        for npc in spawn.findall("npc"):
            count = int(npc.get("count", "1"))
            if count <= 0:
                continue
            npcs.append((int(npc.get("id")), count))
        if not npcs:
            continue
        territories.append({
            "zone": spawn.get("zone"), "nodes": nodes, "npcs": npcs,
        })

    return territories


def point_in_polygon(x, y, nodes):
    """The ray crossing point-in-polygon test."""
    inside = False
    n = len(nodes)
    j = n - 1
    for i in range(n):
        xi, yi = nodes[i]
        xj, yj = nodes[j]
        if (yi > y) != (yj > y):
            cross = (xj - xi) * (y - yi) / (yj - yi) + xi
            if x < cross:
                inside = not inside
        j = i

    return inside


def polygon_bbox(nodes):
    xs = [p[0] for p in nodes]
    ys = [p[1] for p in nodes]

    return min(xs), min(ys), max(xs), max(ys)


def sample_polygon(nodes):
    """The uniform grid samples of the polygon interior."""
    x0, y0, x1, y1 = polygon_bbox(nodes)
    samples = []
    for gx in range(x0, x1 + 1, SAMPLE_STEP):
        for gy in range(y0, y1 + 1, SAMPLE_STEP):
            if point_in_polygon(gx, gy, nodes):
                samples.append((gx, gy))

    return samples


def centroid(samples):
    if not samples:
        return 0.0, 0.0
    n = float(len(samples))

    return sum(p[0] for p in samples) / n, sum(p[1] for p in samples) / n


def chebyshev_extent(samples, cx, cy):
    """The Chebyshev (square) extent of the samples from a center."""
    reach = 0
    for x, y in samples:
        reach = max(reach, abs(int(round(x)) - int(round(cx))),
                    abs(int(round(y)) - int(round(cy))))

    return reach


def split_samples(samples):
    """Split the sample cloud by the median of its widest axis."""
    cx, cy = centroid(samples)
    spread_x = max(p[0] for p in samples) - min(p[0] for p in samples)
    spread_y = max(p[1] for p in samples) - min(p[1] for p in samples)
    axis = 0 if spread_x >= spread_y else 1
    ordered = sorted(samples, key=lambda p: p[axis])
    mid = len(ordered) // 2

    return ordered[:mid], ordered[mid:]


def cut_pieces(samples):
    """Recursively cut the sample cloud until every piece fits the
    leash square (the Chebyshev extent from the piece centroid stays
    under LEASH_HALF), merging the pieces too small to matter."""
    queue = [samples]
    pieces = []
    while queue:
        cloud = queue.pop()
        cx, cy = centroid(cloud)
        reach = chebyshev_extent(cloud, cx, cy)
        if reach <= LEASH_HALF:
            pieces.append(cloud)

            continue
        left, right = split_samples(cloud)
        if not left or not right:
            # A degenerate cloud (all samples on one line): keep it as
            # is, the radius clamps to the visibility circle.
            pieces.append(cloud)

            continue
        queue.append(left)
        queue.append(right)
    # The merge pass: a piece whose extent is below the minimum and a
    # neighbor within reach absorbs it (the wander jitter covers it).
    merged = []
    for piece in sorted(pieces, key=lambda p: -len(p)):
        cx, cy = centroid(piece)
        absorbed = False
        for host in merged:
            hx, hy = host["cx"], host["cy"]
            if abs(cx - hx) <= MIN_PIECE_EXTENT and abs(cy - hy) <= MIN_PIECE_EXTENT:
                host["samples"].extend(piece)
                host["cx"], host["cy"] = centroid(host["samples"])

                absorbed = True

                break
        if not absorbed:
            merged.append({"cx": cx, "cy": cy, "samples": piece})

    return merged


def cluster_territories(territories):
    """The grid adjacency clusters of the territories: the centroids
    landing in the same or 8-adjacent 2048 cells share one ground
    (the same clustering the old generator applied to the registry
    squares)."""
    cells = {}
    for terr in territories:
        cx, cy = centroid(terr["samples"])
        terr["cx"], terr["cy"] = cx, cy
        cells.setdefault((int(cx) // 2048, int(cy) // 2048), []).append(terr)
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


def cut_pieces(samples):
    """Recursively cut the sample cloud until every piece fits the
    leash square (the Chebyshev extent from the piece centroid stays
    under LEASH_HALF)."""
    queue = [samples]
    pieces = []
    while queue:
        cloud = queue.pop()
        cx, cy = centroid(cloud)
        reach = chebyshev_extent(cloud, cx, cy)
        if reach <= LEASH_HALF:
            pieces.append(cloud)

            continue
        left, right = split_samples(cloud)
        if not left or not right:
            # A degenerate cloud (all samples on one line): keep it as
            # is, the radius clamps to the visibility circle.
            pieces.append(cloud)

            continue
        queue.append(left)
        queue.append(right)

    return pieces


def pack_pieces(pieces):
    """The greedy packing of the cut pieces: a piece absorbs its
    nearest neighbor while the merged cloud still fits the leash
    square, so the piece count approaches the minimum the ground
    needs (a 3x2 piece mosaic of one territory becomes 2-3 packed
    spots instead of 6 thin slivers)."""
    packed = [list(piece) for piece in pieces]
    merged = True
    while merged:
        merged = False
        packed.sort(key=lambda c: -len(c))
        for host_i in range(len(packed)):
            host = packed[host_i]
            hx, hy = centroid(host)
            best_j = -1
            best_dist = None
            for other_i in range(len(packed)):
                if other_i == host_i:
                    continue
                ox, oy = centroid(packed[other_i])
                dist = max(abs(ox - hx), abs(oy - hy))
                if best_dist is None or dist < best_dist:
                    best_dist, best_j = dist, other_i
            if best_j < 0:
                continue
            candidate = host + packed[best_j]
            ccx, ccy = centroid(candidate)
            if chebyshev_extent(candidate, ccx, ccy) <= LEASH_HALF:
                packed[host_i] = candidate
                packed.pop(best_j)
                merged = True

                break

    return packed


def build_spots(territories, species):
    """The spot list of the territory clusters: every cluster sample
    cloud is cut into leash-sized pieces and packed into the minimum
    piece count; every piece carries the species composition of its
    samples (the counts distribute by the area share of the piece
    inside every contributing territory)."""
    for terr in territories:
        terr["samples"] = sample_polygon(terr["nodes"])
        terr["sample_set"] = set(terr["samples"])
        # The per-sample species share: every sample of a territory
        # carries count/len(samples) of each of its species.
        share = []
        for tid, count in terr["npcs"]:
            info = species[tid]
            share.append((tid, info, count / float(len(terr["samples"]))))
        terr["share"] = share
    spots = []
    for cluster in cluster_territories(territories):
        cloud = []
        for terr in cluster:
            cloud.extend(terr["samples"])
        if not cloud:
            continue
        pieces = pack_pieces(cut_pieces(cloud))
        # The dominant species names the ground.
        mass = {}
        for terr in cluster:
            for tid, count in terr["npcs"]:
                mass[tid] = mass.get(tid, 0) + count
        dom_tid = max(mass, key=lambda t: mass[t])
        dom_name = species[dom_tid]["name"]
        cluster_cx, cluster_cy = centroid(cloud)
        multi = len(pieces) > 1
        for index, piece in enumerate(pieces):
            ax, ay = centroid(piece)
            ax, ay = int(round(ax)), int(round(ay))
            # The species counts of the piece: the samples that landed
            # in it carry their territory shares.
            piece_mass = {}
            for x, y in piece:
                for terr in cluster:
                    if (x, y) not in terr["sample_set"]:
                        continue
                    for tid, info, per_sample in terr["share"]:
                        piece_mass[tid] = piece_mass.get(tid, 0.0) + per_sample
            mobs = []
            for tid, m in piece_mass.items():
                count = int(round(m))
                if count < 1:
                    continue
                info = species[tid]
                mobs.append({
                    "template_id": tid, "name": info["name"],
                    "level": info["level"], "count": count,
                    "respawn": DEFAULT_RESPAWN,
                })
            if not mobs:
                continue
            reach = chebyshev_extent(piece, ax, ay)
            radius = int(round(reach * SQRT2))
            radius = max(MIN_PIECE_EXTENT, min(radius, VISIBILITY_RADIUS))
            name = "%s %s" % (
                dom_name, compass(ax, ay, VILLAGE_X, VILLAGE_Y))
            if multi:
                name += "-%s%d" % ("abcdefgh"[index % 8], index + 1)
            spots.append({
                "zone": ",".join(sorted(t["zone"] for t in cluster)),
                "name": name,
                "anchor_x": ax, "anchor_y": ay, "radius": radius,
                "mobs": mobs,
                "samples": piece,
            })
    # The registry order: the lowest band first, the village nearest
    # ground of the band leads (the starter fallback of the picker).
    def band_key(spot):
        levels = [m["level"] for m in spot["mobs"]]
        low, high = min(levels), max(levels)
        dist = math.hypot(spot["anchor_x"] - VILLAGE_X,
                          spot["anchor_y"] - VILLAGE_Y)

        return (low, high, dist)

    spots.sort(key=band_key)

    return spots


def coverage_report(spots, territories):
    """The offline coverage check: every territory sample point must
    sit inside the leash square of some generated spot - the union of
    the leashes covers the whole spawn ground by construction, the
    check pins it."""
    covered = 0
    total = 0
    for terr in territories:
        for x, y in terr["samples"]:
            total += 1
            for spot in spots:
                half = int(round(spot["radius"] / SQRT2))
                if (abs(x - spot["anchor_x"]) <= half and
                        abs(y - spot["anchor_y"]) <= half):
                    covered += 1

                    break

    return covered, total


def cross_check_audit(spots, audit_path):
    """Validate the generated geometry against the live audit: every
    audited old anchor re-tested - the mobs its session observed are
    counted against the leash of the NEW spot covering that anchor
    (the same mobs, the same position, the new geometry)."""
    if not os.path.exists(audit_path):
        return []
    with open(audit_path, encoding="utf-8") as fh:
        audit = json.load(fh)
    evidence = []
    for record in audit["spots"]:
        ax, ay = record["anchor_x"], record["anchor_y"]
        covering = None
        for spot in spots:
            half = int(round(spot["radius"] / SQRT2))
            if (abs(ax - spot["anchor_x"]) <= half and
                    abs(ay - spot["anchor_y"]) <= half):
                covering = spot

                break
        if covering is None:
            best = None
            best_dist = None
            for spot in spots:
                dist = math.hypot(spot["anchor_x"] - ax, spot["anchor_y"] - ay)
                if best_dist is None or dist < best_dist:
                    best_dist, best = dist, spot
            covering = best
        half = int(round(covering["radius"] / SQRT2))
        in_leash_new = 0
        for npc in record["npcs"]:
            if (abs(npc["x"] - covering["anchor_x"]) <= half and
                    abs(npc["y"] - covering["anchor_y"]) <= half):
                in_leash_new += 1
        evidence.append({
            "old_id": record["id"], "old_name": record["name"],
            "new_id": covering["id"], "new_name": covering["name"],
            "new_anchor": (covering["anchor_x"], covering["anchor_y"]),
            "seen": record["attackable_count"],
            "old_leash": record["in_leash_count"],
            "new_leash": in_leash_new,
        })

    return evidence


def render_go(spots):
    """The generated Go source of the spot registry."""
    lines = []
    lines.append(
        "// SPDX-FileCopyrightText: 2026 Melg Eight "
        "<public.melg8@gmail.com>")
    lines.append("//")
    lines.append("// SPDX-License-Identifier: MIT")
    lines.append("")
    lines.append(
        "// Code generated by tools/regenerate_spots_from_audit.py from the")
    lines.append(
        "// Mobius spawn territory polygons, validated by the live spot")
    lines.append(
        "// audit (docs/hunt_analysis/spot_audit.json); DO NOT EDIT by hand -")
    lines.append(
        "// re-run the generator instead. Every spot is one")
    lines.append(
        "// visibility-sized piece of a spawn territory: the territory")
    lines.append(
        "// polygon is sampled on a uniform grid and cut until every")
    lines.append(
        "// piece fits the leash square (the Chebyshev extent from the")
    lines.append(
        "// piece centroid stays under 1448, so the leash - inscribed in")
    lines.append(
        "// the 2048 visibility circle - covers the piece by")
    lines.append(
        "// construction), the anchor is the piece centroid, the mob")
    lines.append(
        "// composition is the territory species list with the counts")
    lines.append(
        "// distributed by the area share. See")
    lines.append("// docs/hunting_system_redesign.md for the model.")
    lines.append("")
    lines.append("package hunt")
    lines.append("")
    lines.append(
        "// elvenHuntingSpots anchors the elven lands in %d hunting"
        % len(spots))
    lines.append(
        "// spots. The first entry is the starter fallback of the")
    lines.append(
        "// spot picker (the village nearest spot of the lowest band).")
    lines.append("var elvenHuntingSpots = []Spot{")
    for spot in spots:
        levels = [m["level"] for m in spot["mobs"]]
        mass = float(sum(m["count"] for m in spot["mobs"]))
        lines.append("\t{")
        lines.append('\t\tID: "%s", Name: "%s",' % (spot["id"], spot["name"]))
        lines.append(
            "\t\tRegion: regionElven, MinLevel: %d, MaxLevel: %d,"
            % (min(levels), max(levels)))
        lines.append(
            "\t\tAnchorX: %d, AnchorY: %d, Radius: %d,"
            % (spot["anchor_x"], spot["anchor_y"], spot["radius"]))
        lines.append(
            "\t\tRespawnMin: %d, RespawnMax: %d, Mass: %s,"
            % (DEFAULT_RESPAWN[0], DEFAULT_RESPAWN[1], format_mass(mass)))
        lines.append("\t\tMobs: []SpotMob{")
        for mob in sorted(spot["mobs"], key=lambda m: -m["level"]):
            lines.append("\t\t\t{")
            lines.append(
                '\t\t\t\tTemplateID: %d, Name: "%s",'
                % (mob["template_id"], mob["name"]))
            lines.append(
                "\t\t\t\tLevel: %d, Count: %d, RespawnMin: %d, "
                "RespawnMax: %d,"
                % (mob["level"], mob["count"],
                   mob["respawn"][0], mob["respawn"][1]))
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

    return "\n".join(lines) + "\n"


def format_mass(mass):
    if mass == int(mass):
        return "%.1f" % mass

    return "%f" % mass


def assign_ids(spots):
    """The stable sequential ids of the sorted registry."""
    for index, spot in enumerate(spots, start=1):
        spot["id"] = "elven-spot-%02d" % index


def main():
    _, old_spots = parse_registry_species(REGISTRY)
    territories = load_territories()
    npc_ids = set()
    for terr in territories:
        for tid, _ in terr["npcs"]:
            npc_ids.add(tid)
    species = load_species_from_stats(npc_ids)
    spots = build_spots(territories, species)
    assign_ids(spots)
    evidence = cross_check_audit(spots, AUDIT)
    covered, total = coverage_report(spots, territories)
    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write(render_go(spots))
    slim = []
    for spot in spots:
        slim.append({k: spot[k] for k in (
            "id", "name", "anchor_x", "anchor_y", "radius", "mobs")})
    with open(JSON_OUT, "w", encoding="utf-8") as fh:
        json.dump(slim, fh, indent=2)
    with open(ANCHORS_OUT, "w", encoding="utf-8") as fh:
        json.dump({"spots": {s["id"]: {
            "x": s["anchor_x"], "y": s["anchor_y"], "z": -3600,
            "radius": s["radius"]} for s in spots}}, fh, indent=2)
    with open(REPORT, "w", encoding="utf-8") as fh:
        fh.write("the elven spot regeneration report\n")
        fh.write("=================================\n\n")
        fh.write("old registry: %d spots (audited live: %d)\n"
                 % (len(old_spots), len(evidence)))
        fh.write("new registry: %d spots from %d territories\n"
                 % (len(spots), len(territories)))
        fh.write("the leash union covers %d of %d territory sample "
                 "points (%.1f%%)\n\n"
                 % (covered, total, 100.0 * covered / max(total, 1)))
        fh.write("the live audit cross-check (the observed mobs of every\n")
        fh.write("old anchor re-tested against the new covering leash):\n")
        improved = same = worse = 0
        for row in evidence:
            verdict = ""
            if row["new_leash"] > row["old_leash"]:
                improved += 1
                verdict = "IMPROVED"
            elif row["new_leash"] == row["old_leash"]:
                same += 1
            else:
                worse += 1
                verdict = "WORSE"
            fh.write(
                "  %-28s (%s -> %s): seen %d, old leash %d, new leash "
                "%d %s\n"
                % (row["old_name"], row["old_id"], row["new_id"],
                   row["seen"], row["old_leash"], row["new_leash"], verdict))
        fh.write("\nsummary: %d improved, %d same, %d worse\n"
                 % (improved, same, worse))
    print("generated %d spots from %d territories -> %s"
          % (len(spots), len(territories), OUT))
    print("coverage: %d of %d sample points; evidence -> %s"
          % (covered, total, REPORT))


if __name__ == "__main__":
    main()
