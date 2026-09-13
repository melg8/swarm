#!/usr/bin/env python3
"""Generate the elven hunting CELL registry: the Voronoi partition of
the spawn ground.

Why: the spot registry anchored the hunting on CIRCLES - one anchor
per visibility-sized piece of a territory, the leash square inscribed
in the circle. The circle geometry has a structural failure the user
measured live (2026-09-13): a circle that covers HALF of a respawn
ground farms that half to exhaustion while the other half accumulates
an unfarmed mob mass (the Dryad case); the followup circle that
covers the second half then faces an oversaturated ground it cannot
clear. The partition must own every spawn point EXACTLY ONCE - that
is the definition of a Voronoi cell:

  1. the spawn territory polygons of the Mobius XML are sampled on a
     dense uniform grid (the spawn distribution is uniform over the
     polygon area);
  2. the sample cloud is cut into visibility-sized pieces (the same
     median cut + greedy packing the spot generator used - the live
     audited geometry of 2026-09-13 validated this seed placement),
     and every piece centroid becomes one Voronoi SEED;
  3. the cell of a seed is the convex polygon of the points closer to
     it than to any other seed (the half-plane intersection against
     the ground envelope, Sutherland-Hodgman clipping), so every
     ground point - every spawn point, every respawn - belongs to
     EXACTLY ONE cell: the half-covered respawn failure is impossible
     by construction;
  4. every sample point is assigned to its nearest seed (the exact
     Voronoi assignment), the mob counts of a territory species
     distribute over the cells by the sample share (largest
     remainder, the territory total is preserved);
  5. cells whose sample extent from the focus exceeds the visibility
     budget split (a fresh seed at the far half centroid, recompute),
     so the whole cell stays inside the knownlist circle of a bot
     standing anywhere in its patrol square: no "left part loaded,
     right part not" depletion;
  6. the adjacency graph of the partition (the seeds whose bisector
     bounds the cell) rides along - the cell rotation of the hunt
     policy walks the graph, never the far map.

Every cell carries: the focus (the sample centroid - the patrol
destination), the patrol square (the axis-aligned square inscribed
in the cell polygon at the focus, bounded so the whole cell stays
visible from inside it - the movement leash of the square-based hunt
machinery), the convex polygon (the target leash - the engage, the
far search and the emptiness reading use the WHOLE cell), the mob
composition, the respawn window, the mass and the neighbors.

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
import re
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
# fallback of a fresh character is the village nearest cell of the
# lowest band).
VILLAGE_X, VILLAGE_Y = 46112, 41500

# The guaranteed visible radius of a standing character (the Mobius
# world grid broadcasts the own region plus the 8 adjacent ones).
VISIBILITY_RADIUS = 2048

# The leash square half of the seed pieces (the Chebyshev extent the
# seed cutting respects, inherited from the live audited geometry).
SQRT2 = math.sqrt(2.0)
LEASH_HALF = int(round(VISIBILITY_RADIUS / SQRT2))

# The maximum Euclidean sample radius of a cell from its focus: the
# whole cell must stay inside the knownlist circle of a bot standing
# anywhere in the patrol square (patrol half + cell radius <= the
# visibility radius), with the patrol floor leaving this headroom.
CELL_RADIUS_MAX = 1600

# The minimum patrol square half: a thinner cell keeps the floor and
# the report flags it (the movement leash tightens, the target leash
# stays the whole polygon).
PATROL_HALF_FLOOR = 256

# The ground envelope margin around the spawn sample bounding box:
# the polygon of an edge cell extends this far past the ground into
# the empty map (the wander jitter of the border mobs).
ENVELOPE_MARGIN = 512

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

# The densification iteration bound (the split loop converges in a
# few rounds; the bound breaks a pathological oscillation).
DENSIFY_MAX_ROUNDS = 12

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


def euclidean_radius(samples, cx, cy):
    """The Euclidean radius of the samples from a center."""
    radius = 0.0
    for x, y in samples:
        radius = max(radius, math.hypot(x - cx, y - cy))

    return radius


def split_samples(samples):
    """Split the sample cloud by the median of its widest axis."""
    spread_x = max(p[0] for p in samples) - min(p[0] for p in samples)
    spread_y = max(p[1] for p in samples) - min(p[1] for p in samples)
    axis = 0 if spread_x >= spread_y else 1
    ordered = sorted(samples, key=lambda p: p[axis])
    mid = len(ordered) // 2

    return ordered[:mid], ordered[mid:]


def cluster_territories(territories):
    """The grid adjacency clusters of the territories: the centroids
    landing in the same or 8-adjacent 2048 cells share one ground
    (the same clustering the spot generator applied to the registry
    squares - a seed of one ground never falls into another,
    disconnected ground)."""
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
            # is, the split loop cannot separate it further.
            pieces.append(cloud)

            continue
        queue.append(left)
        queue.append(right)

    return pieces


def pack_pieces(pieces):
    """The greedy packing of the cut pieces: a piece absorbs its
    nearest neighbor while the merged cloud still fits the leash
    square, so the piece count approaches the minimum the ground
    needs (the live audited seed placement)."""
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


def build_seeds(territories):
    """The Voronoi seed set: the packed piece centroids of every
    territory cluster (the live audited spot anchors of the 2026-09-13
    geometry - the same cutting, the same packing). Returns the seeds
    and the cluster membership of every seed (the ground naming)."""
    seeds = []
    seed_clusters = []
    for cluster in cluster_territories(territories):
        cloud = []
        for terr in cluster:
            cloud.extend(terr["samples"])
        if not cloud:
            continue
        pieces = pack_pieces(cut_pieces(cloud))
        for piece in pieces:
            cx, cy = centroid(piece)
            seeds.append((int(round(cx)), int(round(cy))))
            seed_clusters.append(len(seeds) - 1)

    return seeds, seed_clusters


def clip_halfplane(poly, dx, dy, c):
    """Clip the convex polygon (CCW vertex list) against the closed
    half-plane  dx*x + dy*y <= c  (the bisector of the two seeds it
    comes from keeps the points nearer to the owner). Returns the
    clipped polygon."""
    if not poly:
        return poly
    out = []
    n = len(poly)
    for i in range(n):
        ax, ay = poly[i]
        bx, by = poly[(i + 1) % n]
        fa = dx * ax + dy * ay - c
        fb = dx * bx + dy * by - c
        a_in = fa <= 0.0
        b_in = fb <= 0.0
        if a_in and b_in:
            out.append((bx, by))
        elif a_in and not b_in:
            t = fa / (fa - fb)
            out.append((ax + (bx - ax) * t, ay + (by - ay) * t))
        elif not a_in and b_in:
            t = fa / (fa - fb)
            out.append((ax + (bx - ax) * t, ay + (by - ay) * t))
            out.append((bx, by))
        # both out: nothing
    # The degenerate repeats (a vertex exactly on the line twice)
    # collapse here.
    dedup = []
    for p in out:
        if not dedup or p != dedup[-1]:
            dedup.append(p)
    if len(dedup) > 1 and dedup[0] == dedup[-1]:
        dedup.pop()

    return dedup


def polygon_area(poly):
    """The signed area of the polygon (positive when CCW)."""
    total = 0.0
    n = len(poly)
    for i in range(n):
        ax, ay = poly[i]
        bx, by = poly[(i + 1) % n]
        total += ax * by - bx * ay

    return total / 2.0


def bisector(seed_a, seed_b):
    """The bisector half-plane of two seeds: the points nearer to
    seed_a than to seed_b satisfy  dx*x + dy*y <= c."""
    ax, ay = seed_a
    bx, by = seed_b
    dx = 2.0 * (bx - ax)
    dy = 2.0 * (by - ay)
    c = (bx * bx + by * by) - (ax * ax + ay * ay)

    return dx, dy, c


def normalize_polygon(poly):
    """The emitted polygon form: the vertices rounded to integers (the
    runtime leash arithmetic runs on ints) with the collinear and the
    rounding-concave vertices dropped (a rounded vertex can turn a
    hair right - the containment test of the leash assumes a convex
    counter-clockwise ring)."""
    rounded = [(int(round(x)), int(round(y))) for x, y in poly]
    if len(rounded) < 3:
        return rounded
    kept = []
    n = len(rounded)
    for i in range(n):
        ax, ay = rounded[(i - 1) % n]
        bx, by = rounded[i]
        cx, cy = rounded[(i + 1) % n]
        cross = (bx - ax) * (cy - ay) - (by - ay) * (cx - ax)
        if cross > 0:
            kept.append((bx, by))
    if len(kept) < 3:
        # A fully degenerate ring (a sliver thinner than the rounding
        # step): keep the raw rounded ring, the leash of such a cell
        # is its focus neighborhood anyway.
        return rounded

    return kept


def voronoi_cells(seeds, envelope):
    """The Voronoi diagram of the seeds clipped to the rectangular
    envelope: the convex cell polygon of every seed (CCW) plus the
    neighbor seeds whose bisector bounds it. The half-plane
    intersection IS the Voronoi cell - every point of the plane
    closer to the seed than to any other satisfies every bisector
    half-plane, so the exact cell shape falls out of the clipping."""
    x0, y0, x1, y1 = envelope
    box = [(x0, y0), (x1, y0), (x1, y1), (x0, y1)]
    cells = []
    for i, seed in enumerate(seeds):
        poly = list(box)
        # A seed outside the envelope owns nothing (the seeds sit on
        # the ground, inside by construction).
        for j, other in enumerate(seeds):
            if j == i:
                continue
            dx, dy, c = bisector(seed, other)
            poly = clip_halfplane(poly, dx, dy, c)
            if not poly:
                break
        neighbors = set()
        if poly and polygon_area(poly) > 0.0:
            for j, other in enumerate(seeds):
                if j == i:
                    continue
                dx, dy, c = bisector(seed, other)
                norm = math.hypot(dx, dy)
                if norm <= 0.0:
                    continue
                # A bisector bounds the cell when a polygon edge lies
                # on its line (both endpoints within one unit of it).
                n = len(poly)
                for e in range(n):
                    ax, ay = poly[e]
                    bx, by = poly[(e + 1) % n]
                    if (abs(dx * ax + dy * ay - c) <= norm and
                            abs(dx * bx + dy * by - c) <= norm):
                        neighbors.add(j)

                        break
        cells.append({
            "seed": i, "polygon": poly if poly else [],
            "neighbors": neighbors,
        })

    return cells


def assign_nearest(samples, seeds):
    """The exact Voronoi assignment: every sample point to its
    nearest seed (the first seed wins the ties - deterministic).
    Vectorized through numpy when available (the plain loop over
    ~15k samples x ~300 seeds costs seconds per densify round)."""
    try:
        import numpy
    except ImportError:
        numpy = None
    if numpy is not None and len(samples) > 0:
        pts = numpy.asarray(samples, dtype=numpy.float64)
        sd = numpy.asarray(seeds, dtype=numpy.float64)
        owners = []
        for start in range(0, len(pts), 4096):
            chunk = pts[start:start + 4096]
            d = ((chunk[:, None, :] - sd[None, :, :]) ** 2).sum(-1)
            owners.extend(d.argmin(1).tolist())

        return owners
    owners = []
    for x, y in samples:
        best_i = 0
        best_d = None
        for i, (sx, sy) in enumerate(seeds):
            d = (x - sx) * (x - sx) + (y - sy) * (y - sy)
            if best_d is None or d < best_d:
                best_d, best_i = d, i
        owners.append(best_i)

    return owners


def ground_envelope(samples):
    """The bounding box of the whole spawn ground widened by the
    envelope margin: the edge cells extend this far into the empty
    map, the border mobs wander inside it."""
    xs = [p[0] for p in samples]
    ys = [p[1] for p in samples]

    return (min(xs) - ENVELOPE_MARGIN, min(ys) - ENVELOPE_MARGIN,
            max(xs) + ENVELOPE_MARGIN, max(ys) + ENVELOPE_MARGIN)


def densify(seeds, samples):
    """The stabilization loop of the partition: drop the seeds that
    own no samples (the diagram fills the hole), split the cells
    whose sample radius from the focus exceeds the visibility budget
    (a fresh seed at the far half centroid). Returns the stable seed
    list and the per-seed sample buckets of the final assignment."""
    all_samples = [p for cloud in samples for p in cloud]
    for _ in range(DENSIFY_MAX_ROUNDS):
        owners = assign_nearest(all_samples, seeds)
        buckets = [[] for _ in seeds]
        for point, owner in zip(all_samples, owners):
            buckets[owner].append(point)
        drop = [i for i, b in enumerate(buckets) if not b]
        if drop:
            keep = [i for i, b in enumerate(buckets) if b]
            seeds = [seeds[i] for i in keep]
            buckets = [buckets[i] for i in keep]

            continue
        grew = False
        for bucket in buckets:
            if len(bucket) < 2:
                continue
            cx, cy = centroid(bucket)
            if euclidean_radius(bucket, cx, cy) <= CELL_RADIUS_MAX:
                continue
            left, right = split_samples(bucket)
            if not left or not right:
                continue
            lx, ly = centroid(left)
            rx, ry = centroid(right)
            if math.hypot(lx - cx, ly - cy) >= math.hypot(rx - cx, ry - cy):
                fresh = (int(round(lx)), int(round(ly)))
            else:
                fresh = (int(round(rx)), int(round(ry)))
            if fresh in seeds:
                continue
            seeds = seeds + [fresh]
            grew = True

            break
        if grew:
            continue

        return seeds, buckets

    raise SystemExit(
        "error: the densification did not converge in %d rounds"
        % DENSIFY_MAX_ROUNDS)


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


def inscribed_patrol_half(poly, fx, fy, radius):
    """The axis-aligned patrol square half centered at the focus: the
    largest square that stays inside the convex polygon (the
    inward-normal distance of every edge divided by its |nx| + |ny|),
    capped so the whole cell stays inside the knownlist circle from
    ANY point of the square (the corner sits radius x sqrt(2) from
    the focus, the farthest cell sample radius away: half x sqrt(2) +
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
    visible = (VISIBILITY_RADIUS - radius) / SQRT2
    half = int(math.floor(min(best, visible, float(LEASH_HALF))))
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
    """The cell registry: the Voronoi partition of the ground (every
    sample point assigned to exactly one cell by the nearest seed),
    the mob composition by the largest-remainder sample share (the
    territory totals preserved), the focus/patrol/neighbors geometry,
    the compass naming and the band ordering."""
    all_samples = []
    for terr in territories:
        terr["samples"] = sample_polygon(terr["nodes"])
        all_samples.extend(terr["samples"])
    if not all_samples:
        raise SystemExit("error: the spawn XML holds no ground samples")
    seeds, _ = build_seeds(territories)
    sample_clouds = [terr["samples"] for terr in territories]
    seeds, buckets = densify(seeds, sample_clouds)
    envelope = ground_envelope(all_samples)
    cells_raw = voronoi_cells(seeds, envelope)
    for cell in cells_raw:
        # The emitted geometry is integer: the runtime leash and the
        # patrol square derive from the same rounded ring the Go
        # registry carries (a patrol square computed against the float
        # polygon can poke a corner out of the rounded one).
        cell["polygon"] = normalize_polygon(cell["polygon"])
    for i in range(len(seeds)):
        bucket = buckets[i]
        if not bucket:
            raise SystemExit(
                "error: the cell of seed %d owns no samples" % i)
        fx, fy = centroid(bucket)
        cells_raw[i]["focus"] = (int(round(fx)), int(round(fy)))
        cells_raw[i]["radius"] = euclidean_radius(bucket, fx, fy)
        cells_raw[i]["samples"] = bucket
        cells_raw[i]["mass"] = {}
    # The mob distribution: the owner of every territory sample comes
    # from the exact Voronoi assignment against the final seed list.
    for terr in territories:
        if not terr["samples"]:
            continue
        terr_owners = assign_nearest(terr["samples"], seeds)
        cell_counts = {}
        for owner in terr_owners:
            cell_counts[owner] = cell_counts.get(owner, 0) + 1
        for tid, count in terr["npcs"]:
            weights = {cell: float(n) for cell, n in cell_counts.items()}
            for cell, given in largest_remainder(count, weights).items():
                cells_raw[cell]["mass"][tid] = (
                    cells_raw[cell]["mass"].get(tid, 0) + given)
    # The dominant species of a cell names it: "<species> <compass>-
    # <sequence>" (the compass octant from the village, the sequence
    # disambiguates the shared octants). A cell the largest-remainder
    # rounding left without mobs (the corner ground of a sparse
    # territory) names after the empty ground: it stays a pass
    # through neighbor of the partition, the picker never selects it.
    octant_seq = {}
    ground_seq = {}
    cells = []
    for i in range(len(seeds)):
        cell = cells_raw[i]
        fx, fy = cell["focus"]
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
        patrol = inscribed_patrol_half(
            cell["polygon"], fx, fy, cell["radius"])
        # The generator-side invariant guard: the patrol square stays
        # inside the emitted polygon (the registry test pins the same
        # property, the assert catches the drift at generation time).
        for corner in ((fx - patrol, fy - patrol),
                       (fx + patrol, fy - patrol),
                       (fx + patrol, fy + patrol),
                       (fx - patrol, fy + patrol)):
            if not point_in_convex(cell["polygon"], *corner):
                raise SystemExit(
                    "error: the patrol square of %s leaves the cell at "
                    "%r" % (cell.get("name"), corner))
        cells.append({
            "name": name, "focus_x": fx, "focus_y": fy,
            "patrol_half": patrol, "radius": cell["radius"],
            "mobs": mobs, "polygon": cell["polygon"],
            "neighbors": sorted(cell["neighbors"]),
            "samples": cell["samples"], "seed_index": i,
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
    order = {}
    for index, cell in enumerate(cells):
        order[cell["seed_index"]] = index
    for cell in cells:
        cell["neighbors"] = sorted(
            order[j] for j in cell["neighbors"] if j in order)
    for index, cell in enumerate(cells, start=1):
        cell["id"] = "elven-cell-%03d" % index

    return cells


def cross_check_audit(cells):
    """Validate the partition against the live audit evidence: every
    observed mob of every audited anchor lands in exactly one cell
    (the partition covers the world by construction, the audit pins
    it against live positions), and the mob cloud a standing bot
    sees decomposes over the anchor cell plus its near neighbors
    (the mesh scale property: the bot's own cell plus the 1-2 hop
    ring holds the visible mass, so the cell rotation moves short
    distances)."""
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
                # The observation belongs to no cell (a position off
                # the ground envelope or a corrupted record).
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
        "// re-run the generator instead. Every cell is one Voronoi")
    lines.append(
        "// cell of the spawn ground partition: the seeds are the")
    lines.append(
        "// visibility-sized piece centroids of the live audited")
    lines.append(
        "// 2026-09-13 spot geometry, the polygon is the half-plane")
    lines.append(
        "// intersection (the points nearer to the seed than to any")
    lines.append(
        "// other), every spawn sample point belongs to exactly ONE")
    lines.append(
        "// cell (the nearest seed assignment), the cell extent from")
    lines.append(
        "// the focus stays inside the knownlist circle, and the")
    lines.append(
        "// patrol square is the axis-aligned square inscribed in the")
    lines.append(
        "// polygon at the focus. See docs/hunting_cells.md for the")
    lines.append("// model and docs/hunting.md for the hunt policy.")
    lines.append("")
    lines.append("package hunt")
    lines.append("")
    lines.append(
        "// elvenHuntingCells partitions the elven spawn ground in")
    lines.append(
        "//    %d Voronoi hunting cells." % len(cells))
    lines.append(
        "// The first entry is the starter fallback of the cell picker")
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
    (the coverage, the extent, the patrol, the mass preservation, the
    adjacency symmetry) and the audit cross-check."""
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
    lines = []
    lines.append("elven hunting cells generation report")
    lines.append("====================================")
    lines.append("")
    lines.append("cells: %d" % len(cells))
    lines.append(
        "spawn mass: %d mobs over %d territories, the cells carry %d "
        "(preserved: %s)"
        % (total_spawn, len(territories), cell_mass,
           "yes" if total_spawn == cell_mass else "NO"))
    lines.append(
        "cell radius (focus -> farthest sample): min %d, median %d, "
        "max %d (budget %d)"
        % (radii[0], radii[len(radii) // 2], radii[-1], CELL_RADIUS_MAX))
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
        cell["patrol_half"] * SQRT2 + cell["radius"] <= VISIBILITY_RADIUS
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
