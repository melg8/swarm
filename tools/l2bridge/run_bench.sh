#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# The external engine benchmark round (docs/fastpath_research.md
# section 7): dumps the closed subgraph of the requested regions,
# builds the l2bridge harness and times the raasta and condor engines
# against the production Route probe of the same pack.
#
# Usage:
#
#     tools/l2bridge/run_bench.sh [geodata-dir] [regions] [from] [to]
#
# Defaults reproduce the owner diagonal: the 20_19..21_20 corridor,
# from=12338,42444,-3640 to=58630,91061,-3696 (filter=swim semantics).

set -euo pipefail

GEODATA_DIR="${1:-data/geodata}"
REGIONS="${2:-20_19,20_20,21_19,21_20}"
FROM="${3:-12338,42444,-3640}"
TO="${4:-58630,91061,-3696}"
PACK_DIR="${PACK_DIR:-data/navmesh-bench}"
DUMP="${DUMP:-/tmp/l2mesh.bin}"
HERE="$(cd "$(dirname "$0")" && pwd)"
TIMES="${TIMES:-3}"

cd "$HERE/../.."

echo "== pack build ($REGIONS) =="
go run ./cmd/navmesh-build -geodata "$GEODATA_DIR" -out "$PACK_DIR" \
    -regions "$REGIONS" -repair-fake -workers 2

echo
echo "== production probe =="
go run ./cmd/navmesh-export -navmesh "$PACK_DIR" -regions "$REGIONS" \
    -from "$FROM" -to "$TO" -out "$DUMP" -probe

echo
echo "== cargo build =="
cargo build --release --manifest-path "$HERE/Cargo.toml"

for engine in raasta channel tastar trastar; do
    echo
    echo "== $engine =="
    # The condor channel family strands at this scale (the linear
    # location and the per call vec allocations): the wall clock cap
    # keeps the round moving, the cap rides the timeout message.
    timeout 600 "$HERE/target/release/l2bridge" "$engine" "$DUMP" "$TIMES" ||
        echo "engine $engine: killed or failed after 600s"
done
