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
# Usage: bash tools/coverage_report.sh [-html]
# (the full tree run takes minutes; keep the 6 minutes per run cap of
# AGENTS.md in mind on the constrained agent hosts)

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
RUNS_DIR="${REPO_DIR}/runs"

cd "${REPO_DIR}"
mkdir -p "${RUNS_DIR}"

echo "Running the suite with coverage (the raw profile: runs/cover.out)"
go test ./... -cover -count=1 -coverprofile="${RUNS_DIR}/cover.out" | \
    tee "${RUNS_DIR}/.cover-report-run.log"

python3 - "${RUNS_DIR}/.cover-report-run.log" <<'PYEOF'
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

if [ "${1:-}" = "-html" ]; then
    go tool cover -html="${RUNS_DIR}/cover.out" \
        -o "${RUNS_DIR}/coverage.html"
    echo "HTML report written to ${RUNS_DIR}/coverage.html"
fi

rm -f "${RUNS_DIR}/.cover-report-run.log"
