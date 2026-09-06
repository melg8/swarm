#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Generate the world map tile pyramid for the web map from the L2Bot2.0
# map assets (32 world units per source pixel, 1024x1024 tiles, tile
# name BX_BY.jpg with BX = floor(x / 32768) + 20 and
# BY = floor(y / 32768) + 18, see World.java TILE_ZERO_COORD of the
# Mobius server).
#
# The full resolution level 0 is generated only for the detail window
# around the hunting grounds (the default view of the map magnifies the
# tiles), the whole world ships at levels 1..3 for the zoomed out views
# - a google maps style pyramid that keeps the embedded asset weight
# bounded.
#
# Usage:
#   tools/generate_map_tiles.sh <path/to/L2Bot2.0/Client/Assets/maps> \
#       [detailBxMin detailBxMax detailByMin detailByMax]
# Output: internal/swarm/webserver/web/maps/{level}/{bx}_{by}.jpg

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWARM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SRC="${1:-E:/work/L2Bot2.0/Client/Assets/maps}"
OUT="${SCRIPT_DIR}/../internal/swarm/webserver/web/maps"
DETAIL_BX_MIN="${2:-20}"
DETAIL_BX_MAX="${3:-22}"
DETAIL_BY_MIN="${4:-17}"
DETAIL_BY_MAX="${5:-20}"

[ -d "${SRC}" ] || {
    echo "Error map assets not found at ${SRC}"
    echo "Usage: $0 <path/to/L2Bot2.0/Client/Assets/maps> [detail window]"
    exit 1
}

mkdir -p "${OUT}"

python3 - "${SRC}" "${OUT}" \
    "${DETAIL_BX_MIN}" "${DETAIL_BX_MAX}" \
    "${DETAIL_BY_MIN}" "${DETAIL_BY_MAX}" << 'PYEOF'
import os
import re
import sys
from PIL import Image

src, out = sys.argv[1], sys.argv[2]
detail = (int(sys.argv[3]), int(sys.argv[4]), int(sys.argv[5]),
    int(sys.argv[6]))

# Levels of the pyramid: name -> pixels per tile.
LEVELS = {"0": 1024, "1": 512, "2": 256, "3": 128}
QUALITY = 72

tile_pattern = re.compile(r"^(\d+)_(\d+)\.jpg$")
for name in sorted(os.listdir(src)):
    match = tile_pattern.match(name)
    if not match:
        continue
    bx, by = int(match.group(1)), int(match.group(2))
    image = Image.open(os.path.join(src, name)).convert("RGB")
    for level, pixels in LEVELS.items():
        in_detail = (level == "0"
            and detail[0] <= bx <= detail[1]
            and detail[2] <= by <= detail[3])
        if level != "0" and not in_detail:
            if level == "1":
                pass  # level 1 ships for the whole world
            elif level in ("2", "3"):
                pass
            else:
                continue
        if level == "0" and not in_detail:
            continue
        resized = image.resize((pixels, pixels), Image.LANCZOS)
        directory = os.path.join(out, level)
        os.makedirs(directory, exist_ok=True)
        target = os.path.join(directory, f"{bx}_{by}.jpg")
        resized.save(target, "JPEG", quality=QUALITY, progressive=True,
            optimize=True)

total = 0
count = 0
for root, _, files in os.walk(out):
    for name in files:
        total += os.path.getsize(os.path.join(root, name))
        count += 1
print(f"Generated {count} tiles, {total / 1e6:.1f} MB total")
PYEOF
