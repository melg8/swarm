#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# coverage_report.sh prints the audit view of the test coverage: the
# per package statement coverage sorted worst first with the total,
# so the next tests to write name themselves (coverage_delta.sh
# guards the per package regressions against the committed baseline,
# this report ranks the remaining gaps). With -html it also renders
# runs/coverage.html (gitignored, open it in a browser for the per
# function drill down).
#
# The run reuses the committed baseline format of
# runs/coverage-latest.txt (see coverage_delta.sh): pipe the output
# into it when the refreshed numbers should ship with a change.
#
# Usage: bash tools/coverage_report.sh [-from-log <run.log>] [-html]
# -from-log skips the suite run and reads the per package log a
# coverage_delta.sh KEEP_RUN_LOG=1 run left behind (the CI coverage
# job measures the suite once, both views reading the same run;
# the raw profile runs/cover.out of that run must exist).
# (the full tree run takes minutes; keep the 6 minutes per run cap of
# AGENTS.md in mind on the constrained agent hosts)

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
RUNS_DIR="${REPO_DIR}/runs"
RUN_LOG=""

while [ $# -gt 0 ]; do
    case "$1" in
        -from-log)
            if [ $# -lt 2 ]; then
                echo "Error -from-log needs the run log path" >&2
                exit 2
            fi
            RUN_LOG="$2"
            shift 2
            ;;
        -html)
            HTML=1
            shift
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 2
            ;;
    esac
done

cd "${REPO_DIR}"
mkdir -p "${RUNS_DIR}"

if [ -n "${RUN_LOG}" ]; then
    echo "Reading the coverage run log: ${RUN_LOG}"
    if [ ! -f "${RUN_LOG}" ] || [ ! -f "${RUNS_DIR}/cover.out" ]; then
        echo "Error -from-log needs ${RUN_LOG} and ${RUNS_DIR}/cover.out" >&2
        exit 1
    fi
else
    echo "Running the suite with coverage (the raw profile: runs/cover.out)"
    go test ./... -cover -count=1 -coverprofile="${RUNS_DIR}/cover.out" | \
        tee "${RUNS_DIR}/.cover-report-run.log"
    RUN_LOG="${RUNS_DIR}/.cover-report-run.log"
fi

python3 - "${RUN_LOG}" <<'PYEOF'
import re
import sys

run_log = sys.argv[1]
rows = {}
pattern = re.compile(
    r"^ok\s+(\S+)\s+\S+\s+coverage:\s+([0-9.]+)% of statements")
no_tests = []
with open(run_log, encoding="utf-8") as handle:
    for line in handle:
        match = pattern.match(line)
        if match:
            rows[match.group(1)] = float(match.group(2))
            continue
        if "[no test files]" in line:
            no_tests.append(line.split()[1])

print("")
print("Coverage audit (worst first, the packages to test next):")
for package, pct in sorted(rows.items(), key=lambda item: item[1]):
    bar = "#" * int(round(pct / 5.0))
    print("  %5.1f%%  %-6s %s" % (pct, bar, package))
if no_tests:
    print("")
    print("Packages without any test file:")
    for package in sorted(no_tests):
        print("  %s" % package)
print("")
PYEOF

total=$(go tool cover -func="${RUNS_DIR}/cover.out" | tail -n 1 | \
    awk '{print $NF}')
echo "Total statement coverage: ${total}"

if [ "${HTML:-}" = "1" ]; then
    go tool cover -html="${RUNS_DIR}/cover.out" \
        -o "${RUNS_DIR}/coverage.html"
    echo "HTML report written to ${RUNS_DIR}/coverage.html"
fi

if [ -z "${RUN_LOG}" ] || [ "${RUN_LOG}" = "${RUNS_DIR}/.cover-report-run.log" ]; then
    rm -f "${RUNS_DIR}/.cover-report-run.log"
fi
