#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# bench_save.sh runs the benchmarks of one package with -benchmem and
# saves the output into runs/bench-<name>.txt - the committed
# baseline `task bench:diff` (cmd/benchdiff) compares the next run
# against. The benchmark workflow of AGENTS.md ("compare allocations
# before/after") reads the numbers from this baseline instead of the
# agent's memory or the previous commit message.
#
# Usage: bash tools/bench_save.sh <package> [name]
#   <package>  the package pattern, e.g. ./internal/swarm/pathfind
#   [name]     the baseline file stem (default: the last path
#              element of the package, e.g. pathfind)
#
# Environment: BENCH_PATTERN="^BenchmarkZone$" narrows the run.

set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Error usage: bench_save.sh <package> [name]" >&2

    exit 2
fi
PACKAGE="$1"
NAME="${2:-}"
if [ -z "${NAME}" ]; then
    NAME="$(basename "${PACKAGE}")"
fi

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
RUNS_DIR="${REPO_DIR}/runs"
OUT="${RUNS_DIR}/bench-${NAME}.txt"
mkdir -p "${RUNS_DIR}"

if [ -f "${OUT}" ]; then
    echo "bench_save: overwriting ${OUT} (the old baseline lives in git history)"
fi

echo "bench_save: go test -bench=. -benchmem -run='^$' ${PACKAGE}"
go test -bench=. -benchmem -run='^$' \
    ${BENCH_PATTERN:+-bench="${BENCH_PATTERN}"} "${PACKAGE}" | \
    tee "${OUT}"
echo "bench_save: baseline written to ${OUT} (commit it)"
echo "bench_save: compare later with task bench:diff BASE=${OUT} NEW=<fresh run>"
