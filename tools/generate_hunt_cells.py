#!/usr/bin/env python3
"""Generate the elven hunting CELL registry: the uniform HEXAGON grid
partition of the spawn ground.

Why a hex grid (the 2026-09-13 user order): the Voronoi partition
solved the half-covered respawn failure (every ground point owned by
exactly one cell), but its cells varied in shape and size with the
seed placement - thin slivers next to fat blocks, patrol squares from
260 to 678. The hexagon grid replaces it with ONE hexagon shape at
ONE size over the whole map:

  1. the spawn territory polygons of the Mobius XML are sampled on a
     dense uniform grid (the spawn distribution is uniform over the
     polygon area);
  2. every sample point maps ANALYTICALLY onto its hexagon (the axial
     cube rounding of the flat-top hex grid - no nearest-seed search,
     no clipping): the plane is fully covered by construction, so
     every spawn point, every respawn, belongs to EXACTLY ONE hexagon
     of one fixed size - the half-covered respawn failure stays
     impossible;
  3. the hexagon circumradius HEX_RADIUS is sized to the visibility
     budget: the patrol square inscribed in the hexagon plus the hex
     extent from the focus stays inside the guaranteed knownlist
     circle (~2048 units, the Mobius world region grid), so a bot
     standing anywhere in its patrol square sees the WHOLE hexagon -
     no "left part loaded, right part not" depletion;
  4. the mob counts of a territory species distribute over the
     hexagons by the sample share (largest remainder, the territory
     total is preserved);
  5. the adjacency graph is the hex grid ring itself (the six grid
     neighbors present in the registry, symmetric by construction) -
     the rotation of the hunt policy walks the mesh, never the far
     map.

Every cell carries: the focus (the hexagon center - the patrol
destination), the patrol square (the axis-aligned square inscribed in
the hexagon at the center, the movement leash of the square-based
hunt machinery), the hexagon polygon (the target leash - the engage,
the far search and the emptiness reading use the WHOLE hexagon), the
mob composition, the respawn window, the mass and the neighbors.

Inputs (env-overridable):
  MOBIUS_C1  the L2J_Mobius_C1_HarbingersOfWar dist tree
  AUDIT      the live spot audit evidence JSON (the cross-check)
  OUT        the generated Go file path
  JSON_OUT   the JSON twin for the map mesh and the tools
  REPORT     the human readable generation report

Output: deterministic Go source (hunt/cells_elven.go, spaces only),
the JSON twin and the report. Re-run after any spawn data change.
"""

import json
import math
import os
import xml.etree.ElementTree as ET

MOBIUS_C1 = os.environ.get(
    "MOBIUS_C1",
    "/home/z/my-project/l2j_mobius/L2J_Mobius_C1_HarbingersOfWar")
SPAWN_XML = os.path.join(
    MOBIUS_C1, "dist/game/data/spawns/ElvenTerritory/ElvenStarting.xml")
HERE = os.path.dirname(os.path.abspath(__file__))
AUDIT = os.environ.get(
    "AUDIT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "spot_audit.json"))
OUT = os.environ.get(
    "OUT",
    os.path.join(HERE, "..", "internal", "swarm", "hunt", "cells_elven.go"))
JSON_OUT = os.environ.get(
    "JSON_OUT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "cells_elven.json"))
REPORT = os.environ.get(
    "REPORT",
    os.path.join(HERE, "..", "docs", "hunt_analysis", "cells_report.txt"))

# The elven village: the ordering anchor of the registry (the starter
# fallback of a fresh character is the village nearest hexagon of the
# lowest band).
VILLAGE_X, VILLAGE_Y = 46112, 41500

# The guaranteed visible radius of a standing character (the Mobius
# world grid broadcasts the own region plus the 8 adjacent ones).
VISIBILITY_RADIUS = 2048

# The hexagon circumradius of the uniform grid: every hexagon of the
# registry is exactly this size. The visibility budget sizes it: the
# axis-aligned square inscribed in a flat-top hexagon of radius R has
# half 0.634*R, the farthest hexagon point sits R from the center, and
# 0.634*R*sqrt(2) + R = 1.897*R must stay under the knownlist radius -
# R = 1000 leaves 152 units of headroom for the wander of the border
# mobs past the hexagon edge.
HEX_RADIUS = 1000

# The grid pitch derived from the circumradius: the flat-top hexagon
# column pitch is 1.5*R, the row pitch sqrt(3)*R, the odd columns
# shift half a row.
HEX_COL_PITCH = 1.5 * HEX_RADIUS
HEX_ROW_PITCH = math.sqrt(3.0) * HEX_RADIUS

# The minimum patrol square half: a hexagon never goes below this
# (the registry keeps the uniform geometry, the floor only guards the
# rounding of the emitted integers).
PATROL_HALF_FLOOR = 256

# The polygon sampling grid: the spawn distribution is uniform over
# the polygon area, a 128 unit grid resolves the assignment finely
# enough while a full elven territory holds a few hundred samples.
SAMPLE_STEP = 128

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


def sample_polygon(nodes):
    """The uniform grid samples of the polygon interior."""
    xs = [p[0] for p in nodes]
    ys = [p[1] for p in nodes]
    samples = []
    for gx in range(min(xs), max(xs) + 1, SAMPLE_STEP):
        for gy in range(min(ys), max(ys) + 1, SAMPLE_STEP):
            if point_in_polygon(gx, gy, nodes):
                samples.append((gx, gy))

    return samples


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


def hex_of_point(x, y):
    """The offset coordinates of the hexagon a world point falls into:
    the axial fractional coordinates of the flat-top grid rounded to
    the nearest hexagon center (the cube rounding - the point maps to
    the hexagon whose center sits nearest, which for a tessellation
    IS the hexagon containing it). The grid covers the plane, so every
    point answers a hexagon: the complete-coverage partition property
    holds by construction, no clipping, no search."""
    qf = (2.0 / 3.0) * x / HEX_RADIUS
    rf = (math.sqrt(3.0) / 3.0) * y / HEX_RADIUS - qf / 2.0
    # The cube rounding: round all three axes, the largest rounding
    # error gives way so the sum stays zero.
    xf, yf, zf = qf, rf, -qf - rf
    xq, yq, zq = round(xf), round(yf), round(zf)
    dx, dy, dz = abs(xq - xf), abs(yq - yf), abs(zq - zf)
    if dx > dy and dx > dz:
        xq = -yq - zq
    elif dy > dz:
        yq = -xq - zq
    else:
        zq = -xq - yq
    q, r = xq, yq
    # The odd-q offset rows: odd columns shift half a row down.
    row = r + (q - (q & 1)) // 2

    return q, row


def hex_center(q, row):
    """The world center of the hexagon at the offset coordinates: the
    column pitch 1.5*R, the row pitch sqrt(3)*R, the odd columns
    shifted half a row."""
    x = int(round(HEX_COL_PITCH * q))
    y = int(round(HEX_ROW_PITCH * (row + (q & 1) / 2.0)))

    return x, y


def hex_polygon(cx, cy):
    """The six counter-clockwise vertices of the flat-top hexagon at
    the center: the circumradius HEX_RADIUS, the vertices at the
    multiples of 60 degrees, every hexagon of the registry carries the
    exact same shape and size."""
    third = math.pi / 3.0
    verts = []
    for corner in range(6):
        angle = third * corner
        vx = int(round(cx + HEX_RADIUS * math.cos(angle)))
        vy = int(round(cy + HEX_RADIUS * math.sin(angle)))
        verts.append((vx, vy))

    return verts


def hex_neighbor_offsets(q):
    """The six grid neighbor offsets of the odd-q hex layout: the east
    and west columns, the two diagonal step rows (the parity of the
    column decides which way the diagonals lean), the north and the
    south."""
    if q & 1:
        return [(1, 1), (1, 0), (0, 1), (0, -1), (-1, 1), (-1, 0)]

    return [(1, 0), (1, -1), (0, 1), (0, -1), (-1, 0), (-1, -1)]


def point_in_convex(poly, x, y):
    """The convex containment test (the polygon is CCW): the point
    stays on the inner side of every edge."""
    n = len(poly)
    for i in range(n):
        ax, ay = poly[i]
        bx, by = poly[(i + 1) % n]
        if (bx - ax) * (y - ay) - (by - ay) * (x - ax) < 0.0:
            return False

    return True


def point_in_convex_slack(poly, x, y, slack):
    """The tolerant containment test: the point stays within slack
    units PAST the edge. The sample assignment runs against the
    integer-rounded hexagon polygon - a spawn sample sitting exactly
    on the analytic boundary line can fall a rounding hair (under
    half a unit) outside the emitted polygon while the analytic grid
    still owns it."""
    n = len(poly)
    for i in range(n):
        ax, ay = poly[i]
        bx, by = poly[(i + 1) % n]
        cross = (bx - ax) * (y - ay) - (by - ay) * (x - ax)
        edge = math.hypot(bx - ax, by - ay)
        if cross < -slack * edge:
            return False

    return True


def euclidean_radius(samples, cx, cy):
    """The Euclidean radius of the samples from a center."""
    radius = 0.0
    for x, y in samples:
        radius = max(radius, math.hypot(x - cx, y - cy))

    return radius


def inscribed_patrol_half(poly, fx, fy, radius):
    """The axis-aligned patrol square half centered at the focus: the
    largest square that stays inside the convex polygon (the
    inward-normal distance of every edge divided by its |nx| + |ny|),
    capped so the whole cell stays inside the knownlist circle from
    ANY point of the square (the corner sits half x sqrt(2) from the
    focus, the farthest cell sample radius away: half x sqrt(2) +
    radius <= the visibility radius), floored at the patrol
    minimum."""
    best = None
    n = len(poly)
    for i in range(n):
        ax, ay = poly[i]
        bx, by = poly[(i + 1) % n]
        ex, ey = bx - ax, by - ay
        length = math.hypot(ex, ey)
        if length <= 0.0:
            continue
        # The unit inward normal of a CCW polygon edge (left of the
        # edge direction).
        nx, ny = -ey / length, ex / length
        dist = nx * (fx - ax) + ny * (fy - ay)
        if dist <= 0.0:
            return PATROL_HALF_FLOOR
        limit = dist / (abs(nx) + abs(ny))
        if best is None or limit < best:
            best = limit
    if best is None:
        return PATROL_HALF_FLOOR
    visible = (VISIBILITY_RADIUS - radius) / math.sqrt(2.0)
    half = int(math.floor(min(best, visible, float(HEX_RADIUS))))
    if half < PATROL_HALF_FLOOR:
        half = PATROL_HALF_FLOOR

    return half


def largest_remainder(count, weights):
    """Distribute the integer count over the weight keys preserving
    the total exactly: the floors first, the remainders by the
    largest fractional part (the ties break by the key order - the
    distribution is deterministic)."""
    total_weight = sum(weights.values())
    if total_weight <= 0.0 or count <= 0:
        return {key: 0 for key in weights}
    assigned = {}
    remainders = []
    for key in sorted(weights):
        exact = count * weights[key] / total_weight
        base = int(math.floor(exact))
        assigned[key] = base
        remainders.append((exact - base, key))
    left = count - sum(assigned.values())
    for frac, key in sorted(remainders, key=lambda r: (-r[0], r[1])):
        if left <= 0:
            break
        assigned[key] += 1
        left -= 1

    return assigned


def build_cells(territories, species):
    """The cell registry: the uniform hexagon grid partition of the
    ground (every sample point maps analytically to exactly one
    hexagon), the mob composition by the largest-remainder sample
    share (the territory totals preserved), the focus/patrol/
    neighbors geometry, the compass naming and the band ordering."""
    all_samples = []
    for terr in territories:
        terr["samples"] = sample_polygon(terr["nodes"])
        all_samples.extend(terr["samples"])
    if not all_samples:
        raise SystemExit("error: the spawn XML holds no ground samples")
    # The registry hexagons: every grid hexagon owning at least one
    # sample point. The map (q, row) -> index keeps the grid identity
    # for the neighbor pass.
    grid = {}
    order = []
    for x, y in all_samples:
        coords = hex_of_point(x, y)
        if coords not in grid:
            grid[coords] = len(order)
            order.append({
                "coords": coords, "samples": [],
                "mass": {}, "seed_index": 0,
            })
        order[grid[coords]]["samples"].append((x, y))
    # The mob distribution: the owner of every territory sample comes
    # from the same analytic hexagon lookup.
    for terr in territories:
        if not terr["samples"]:
            continue
        cell_counts = {}
        for x, y in terr["samples"]:
            index = grid[hex_of_point(x, y)]
            cell_counts[index] = cell_counts.get(index, 0) + 1
        for tid, count in terr["npcs"]:
            weights = {cell: float(n) for cell, n in cell_counts.items()}
            for cell, given in largest_remainder(count, weights).items():
                order[cell]["mass"][tid] = (
                    order[cell]["mass"].get(tid, 0) + given)
    # The geometry of every hexagon: the center focus, the uniform
    # polygon, the inscribed patrol square, the grid neighbors that
    # exist in the registry.
    for index, cell in enumerate(order):
        q, row = cell["coords"]
        fx, fy = hex_center(q, row)
        cell["focus_x"], cell["focus_y"] = fx, fy
        cell["polygon"] = hex_polygon(fx, fy)
        cell["radius"] = euclidean_radius(cell["samples"], fx, fy)
        cell["neighbors"] = []
        for dq, drow in hex_neighbor_offsets(q):
            neighbor = (q + dq, row + drow)
            if neighbor in grid:
                cell["neighbors"].append(grid[neighbor])
    # The generator-side invariant guards: the sample points the
    # registry assigns to a hexagon actually fall inside its emitted
    # integer polygon (the rounding of the vertices moves an edge by
    # at most half a unit, a sample on the boundary line maps to the
    # neighbor the analytic rounding chose - only a sample DEEP
    # outside the polygon would be a grid bug), and the patrol square
    # stays inside the hexagon.
    for cell in order:
        poly = cell["polygon"]
        fx, fy = cell["focus_x"], cell["focus_y"]
        patrol = inscribed_patrol_half(poly, fx, fy, cell["radius"])
        cell["patrol_half"] = patrol
        for corner in ((fx - patrol, fy - patrol),
                       (fx + patrol, fy - patrol),
                       (fx + patrol, fy + patrol),
                       (fx - patrol, fy + patrol)):
            if not point_in_convex(poly, *corner):
                raise SystemExit(
                    "error: the patrol square of %r leaves the hexagon "
                    "at %r" % (cell["coords"], corner))
        outside = 0
        for x, y in cell["samples"]:
            if not point_in_convex_slack(poly, x, y, 2.0):
                outside += 1
        if outside > 0:
            raise SystemExit(
                "error: %d samples of the hexagon %r fall outside its "
                "polygon - the grid math broke" % (outside, cell["coords"]))
    # The dominant species of a hexagon names it: "<species> <compass>-
    # <sequence>" (the compass octant from the village, the sequence
    # disambiguates the shared octants). A hexagon the
    # largest-remainder rounding left without mobs (the corner ground
    # of a sparse territory) names after the empty ground: it stays a
    # pass through neighbor of the partition, the picker never
    # selects it.
    octant_seq = {}
    ground_seq = {}
    cells = []
    for index, cell in enumerate(order):
        fx, fy = cell["focus_x"], cell["focus_y"]
        octant = compass(fx, fy, VILLAGE_X, VILLAGE_Y)
        positive = {
            tid: count for tid, count in cell["mass"].items()
            if count > 0}
        if positive:
            dom = min(positive, key=lambda t: (-positive[t], t))
            key = (dom, octant)
            octant_seq[key] = octant_seq.get(key, 0) + 1
            name = "%s %s-%d" % (species[dom]["name"], octant,
                                 octant_seq[key])
        else:
            ground_seq[octant] = ground_seq.get(octant, 0) + 1
            name = "Ground %s-%d" % (octant, ground_seq[octant])
        mobs = []
        for tid in sorted(cell["mass"]):
            count = cell["mass"][tid]
            if count <= 0:
                continue
            mob_info = species[tid]
            mobs.append({
                "template_id": tid, "name": mob_info["name"],
                "level": mob_info["level"], "count": count,
                "respawn": DEFAULT_RESPAWN,
            })
        mobs.sort(key=lambda m: -m["level"])
        cells.append({
            "name": name, "focus_x": fx, "focus_y": fy,
            "patrol_half": cell["patrol_half"],
            "radius": cell["radius"],
            "mobs": mobs, "polygon": cell["polygon"],
            "neighbors": cell["neighbors"],
            "samples": cell["samples"], "seed_index": index,
        })
    # The registry order: the lowest band first, the village nearest
    # ground of the band leads (the starter fallback of the picker).
    def band_key(cell):
        if not cell["mobs"]:
            # The empty ground sorts past every band.
            return (99, 99, math.hypot(
                cell["focus_x"] - VILLAGE_X, cell["focus_y"] - VILLAGE_Y))
        levels = [m["level"] for m in cell["mobs"]]
        low, high = min(levels), max(levels)
        dist = math.hypot(cell["focus_x"] - VILLAGE_X,
                          cell["focus_y"] - VILLAGE_Y)

        return (low, high, dist)

    cells.sort(key=band_key)
    # The neighbor indices remap onto the sorted registry order.
    remap = {}
    for index, cell in enumerate(cells):
        remap[cell["seed_index"]] = index
    for cell in cells:
        cell["neighbors"] = sorted(
            remap[j] for j in cell["neighbors"] if j in remap)
    for index, cell in enumerate(cells, start=1):
        cell["id"] = "elven-hex-%03d" % index

    return cells


def cross_check_audit(cells):
    """Validate the partition against the live audit evidence: every
    observed mob of every audited anchor lands in exactly one hexagon
    (the grid covers the world by construction, the audit pins it
    against live positions), and the mob cloud a standing bot sees
    decomposes over the anchor hexagon plus its near neighbors (the
    mesh scale property: the bot's own hexagon plus the 1-2 hop ring
    holds the visible mass, so the rotation moves short distances)."""
    if not os.path.exists(AUDIT):
        return []
    with open(AUDIT, encoding="utf-8") as fh:
        audit = json.load(fh)
    index = {}
    for i, cell in enumerate(cells):
        index[cell["id"]] = i
    # The 1/2 hop rings of every cell.
    ring1 = {}
    ring2 = {}
    for i, cell in enumerate(cells):
        r1 = set(cell["neighbors"])
        r2 = set(r1)
        for j in cell["neighbors"]:
            r2.update(cells[j]["neighbors"])
        r2.discard(i)
        ring1[i] = r1
        ring2[i] = {j for j in r2 if j not in r1}
    evidence = []
    for record in audit["spots"]:
        ax, ay = record["anchor_x"], record["anchor_y"]
        owning = None
        for cell in cells:
            if point_in_convex(cell["polygon"], ax, ay):
                owning = cell

                break
        if owning is None:
            best = None
            best_dist = None
            for cell in cells:
                dist = math.hypot(cell["focus_x"] - ax,
                                  cell["focus_y"] - ay)
                if best_dist is None or dist < best_dist:
                    best_dist, best = dist, cell
            owning = best
        own = index[owning["id"]]
        in_cell = 0
        hops = {0: 0, 1: 0, 2: 0, 3: 0}
        unowned = 0
        for npc in record["npcs"]:
            owner = None
            for cell in cells:
                if point_in_convex(cell["polygon"], npc["x"], npc["y"]):
                    owner = cell

                    break
            if owner is None:
                # The observation belongs to no hexagon (a position
                # off the audited ground or a corrupted record).
                unowned += 1

                continue
            j = index[owner["id"]]
            if j == own:
                in_cell += 1
                hops[0] += 1
            elif j in ring1[own]:
                hops[1] += 1
            elif j in ring2[own]:
                hops[2] += 1
            else:
                hops[3] += 1
        evidence.append({
            "audited": record["id"], "seen": record["attackable_count"],
            "cell": owning["id"], "in_cell": in_cell, "hops": hops,
            "unowned": unowned,
        })

    return evidence


def format_mass(mass):
    if mass == int(mass):
        return "%.1f" % mass

    return "%f" % mass


def render_go(cells):
    """The generated Go source of the cell registry (spaces only -
    the repository whitespace convention)."""
    lines = []
    lines.append(
        "// SPDX-FileCopyrightText: 2026 Melg Eight "
        "<public.melg8@gmail.com>")
    lines.append("//")
    lines.append("// SPDX-License-Identifier: MIT")
    lines.append("")
    lines.append(
        "// Code generated by tools/generate_hunt_cells.py from the")
    lines.append(
        "// Mobius spawn territory polygons; DO NOT EDIT by hand -")
    lines.append(
        "// re-run the generator instead. Every cell is one hexagon of")
    lines.append(
        "// the UNIFORM grid partition: the flat-top hexagons of one")
    lines.append(
        "// circumradius tile the whole map, every sample point maps")
    lines.append(
        "// analytically (the axial cube rounding) onto exactly ONE")
    lines.append(
        "// hexagon - the half-covered respawn failure is impossible by")
    lines.append(
        "// construction - the hexagon extent from the focus stays")
    lines.append(
        "// inside the knownlist circle, the patrol square is the")
    lines.append(
        "// axis-aligned square inscribed in the hexagon at the center,")
    lines.append(
        "// and the neighbors are the six grid ring members present in")
    lines.append(
        "// the registry. See docs/hunting_cells.md for the model and")
    lines.append("// docs/hunting.md for the hunt policy.")
    lines.append("")
    lines.append("package hunt")
    lines.append("")
    lines.append(
        "// elvenHuntingCells partitions the elven spawn ground in")
    lines.append("//")
    lines.append(
        "//    %d uniform hexagon hunting cells." % len(cells))
    lines.append("//")
    lines.append(
        "// The first entry is the starter fallback of the cell picker")
    lines.append("//")
    lines.append(
        "//    (the village nearest cell of the lowest band).")
    lines.append("var elvenHuntingCells = []Cell{")
    for cell in cells:
        if cell["mobs"]:
            low = min(m["level"] for m in cell["mobs"])
            high = max(m["level"] for m in cell["mobs"])
        else:
            low, high = 0, 0
        mass = float(sum(m["count"] for m in cell["mobs"]))
        lines.append("    {")
        lines.append(
            '        ID: "%s", Name: "%s",'
            % (cell["id"], cell["name"]))
        lines.append(
            "        Region: regionElven, MinLevel: %d, MaxLevel: %d,"
            % (low, high))
        lines.append(
            "        FocusX: %d, FocusY: %d, PatrolHalf: %d,"
            % (cell["focus_x"], cell["focus_y"], cell["patrol_half"]))
        lines.append(
            "        RespawnMin: %d, RespawnMax: %d, Mass: %s,"
            % (DEFAULT_RESPAWN[0], DEFAULT_RESPAWN[1],
               format_mass(mass)))
        if cell["mobs"]:
            lines.append("        Mobs: []CellMob{")
            for mob in cell["mobs"]:
                lines.append("            {")
                lines.append(
                    '                TemplateID: %d, Name: "%s",'
                    % (mob["template_id"], mob["name"]))
                lines.append(
                    "                Level: %d, Count: %d, "
                    "RespawnMin: %d, RespawnMax: %d,"
                    % (mob["level"], mob["count"],
                       mob["respawn"][0], mob["respawn"][1]))
                lines.append("            },")
            lines.append("        },")
        else:
            lines.append("        Mobs: []CellMob{},")
        lines.append("        Vertices: []CellVertex{")
        for vx, vy in cell["polygon"]:
            lines.append("            {X: %d, Y: %d}," % (int(round(vx)),
                                                         int(round(vy))))
        lines.append("        },")
        neighbors = ", ".join(str(n) for n in cell["neighbors"])
        lines.append("        Neighbors: []int32{%s}," % neighbors)
        lines.append("    },")
    lines.append("}")
    lines.append("")
    lines.append(
        "// ElvenHuntingCells returns the hunting cells of the elven lands.")
    lines.append("func ElvenHuntingCells() []Cell {")
    lines.append("    cells := make([]Cell, len(elvenHuntingCells))")
    lines.append("    copy(cells, elvenHuntingCells)")
    lines.append("")
    lines.append("    return cells")
    lines.append("}")

    return "\n".join(lines) + "\n"


def render_json(cells):
    """The JSON twin of the registry: the map mesh payload and the
    tool input of the future regions."""
    payload = []
    for cell in cells:
        payload.append({
            "id": cell["id"], "name": cell["name"],
            "focus_x": cell["focus_x"], "focus_y": cell["focus_y"],
            "patrol_half": cell["patrol_half"],
            "radius": round(cell["radius"], 1),
            "min_level": (min(m["level"] for m in cell["mobs"])
                          if cell["mobs"] else 0),
            "max_level": (max(m["level"] for m in cell["mobs"])
                          if cell["mobs"] else 0),
            "mass": sum(m["count"] for m in cell["mobs"]),
            "respawn": list(DEFAULT_RESPAWN),
            "mobs": [
                {"template_id": m["template_id"], "name": m["name"],
                 "level": m["level"], "count": m["count"]}
                for m in cell["mobs"]
            ],
            "vertices": [
                [int(round(vx)), int(round(vy))]
                for vx, vy in cell["polygon"]
            ],
            "neighbors": cell["neighbors"],
        })

    return payload


def render_report(cells, territories, evidence):
    """The human readable generation report: the partition invariants
    (the coverage, the uniform size, the patrol, the mass
    preservation, the adjacency symmetry) and the audit
    cross-check."""
    total_spawn = sum(
        count for terr in territories
        for _, count in terr["npcs"])
    cell_mass = sum(
        m["count"] for cell in cells for m in cell["mobs"])
    radii = sorted(cell["radius"] for cell in cells)
    patrols = sorted(cell["patrol_half"] for cell in cells)
    edge_count = sum(len(cell["neighbors"]) for cell in cells)
    symmetric = True
    for cell in cells:
        for j in cell["neighbors"]:
            if j < 0 or j >= len(cells):
                symmetric = False

                continue
            if cells[j] is cell:
                symmetric = False

                continue
            found = False
            for back in cells[j]["neighbors"]:
                if cells[back] is cell:
                    found = True

                    break
            if not found:
                symmetric = False
    # The uniform geometry: every hexagon carries six vertices, the
    # same circumradius (the registry pins the size against the
    # generator drift) and the same area.
    uniform_vertex_count = all(
        len(cell["polygon"]) == 6 for cell in cells)
    areas = []
    for cell in cells:
        poly = cell["polygon"]
        area = 0.0
        for i in range(len(poly)):
            ax, ay = poly[i]
            bx, by = poly[(i + 1) % len(poly)]
            area += ax * by - bx * ay
        areas.append(abs(area) / 2.0)
    hex_area = 3.0 * math.sqrt(3.0) / 2.0 * HEX_RADIUS ** 2
    uniform_area = all(
        abs(area - hex_area) <= hex_area * 0.02 for area in areas)
    lines = []
    lines.append("elven hunting cells generation report")
    lines.append("====================================")
    lines.append("")
    lines.append("cells: %d (uniform hexagons, circumradius %d)"
                 % (len(cells), HEX_RADIUS))
    lines.append(
        "spawn mass: %d mobs over %d territories, the cells carry %d "
        "(preserved: %s)"
        % (total_spawn, len(territories), cell_mass,
           "yes" if total_spawn == cell_mass else "NO"))
    lines.append(
        "hexagon size: %d corners everywhere: %s, area %d (uniform: "
        "%s, expected %d)"
        % (6, "yes" if uniform_vertex_count else "NO",
           int(round(sum(areas) / max(1, len(areas)))),
           "yes" if uniform_area else "NO", int(round(hex_area))))
    lines.append(
        "cell radius (focus -> farthest sample): min %d, median %d, "
        "max %d (budget %d)"
        % (radii[0], radii[len(radii) // 2], radii[-1], HEX_RADIUS))
    lines.append(
        "patrol half: min %d, median %d, max %d"
        % (patrols[0], patrols[len(patrols) // 2], patrols[-1]))
    lines.append(
        "adjacency: %d edges, symmetric: %s, mean degree %.1f"
        % (edge_count, "yes" if symmetric else "NO",
           edge_count / max(1, len(cells))))
    flagged = [
        cell for cell in cells
        if cell["patrol_half"] == PATROL_HALF_FLOOR]
    lines.append(
        "patrol floor cells: %d (the inscribed square tighter than %d)"
        % (len(flagged), PATROL_HALF_FLOOR))
    visible_ok = all(
        cell["patrol_half"] * math.sqrt(2.0) + cell["radius"]
        <= VISIBILITY_RADIUS
        for cell in cells)
    lines.append(
        "visibility invariant (patrol*sqrt(2) + radius <= %d): %s"
        % (VISIBILITY_RADIUS, "yes" if visible_ok else "NO"))
    samples_assigned = sum(len(cell["samples"]) for cell in cells)
    lines.append(
        "samples: %d assigned, every cell owns at least one"
        % samples_assigned)
    if evidence:
        seen = sum(e["seen"] for e in evidence)
        hops = {0: 0, 1: 0, 2: 0, 3: 0}
        unowned = 0
        for e in evidence:
            for hop, count in e["hops"].items():
                hops[hop] += count
            unowned += e["unowned"]
        owned = seen - unowned
        lines.append("")
        lines.append(
            "audit cross-check (%d anchors, %d observed mobs): %d "
            "belong to exactly one cell (%.1f%%, the partition "
            "coverage pinned against the live positions); the visible "
            "cloud of a standing bot decomposes into the own cell %d "
            "(%.0f%%), the 1-hop ring %d, the 2-hop ring %d, beyond "
            "the 2-hop ring %d - the mesh scale keeps the farm moves "
            "short"
            % (len(evidence), seen, owned,
               100.0 * owned / max(1, seen), hops[0],
               100.0 * hops[0] / max(1, seen), hops[1], hops[2],
               hops[3]))

    return "\n".join(lines) + "\n"


def main():
    territories = load_territories()
    npc_ids = set()
    for terr in territories:
        for tid, _ in terr["npcs"]:
            npc_ids.add(tid)
    species = load_species_from_stats(npc_ids)
    cells = build_cells(territories, species)
    evidence = cross_check_audit(cells)
    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write(render_go(cells))
    with open(JSON_OUT, "w", encoding="utf-8") as fh:
        json.dump(render_json(cells), fh, indent=1, sort_keys=False)
        fh.write("\n")
    report = render_report(cells, territories, evidence)
    with open(REPORT, "w", encoding="utf-8") as fh:
        fh.write(report)
    print(report)
    print("wrote %s" % OUT)
    print("wrote %s" % JSON_OUT)
    print("wrote %s" % REPORT)


if __name__ == "__main__":
    main()
