#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# coverage_delta.sh runs the test suite with coverage, writes the
# per package summary into runs/coverage-latest.txt (committed - the
# baseline the next run compares against) and the raw profile into
# runs/cover.out (gitignored, feed it to `go tool cover -html`).
#
# The comparison rule: a package whose coverage drops by more than
# COVER_DROP_LIMIT percentage points against the committed summary
# fails the run (default 2.0 - small float jitter passes, real
# regressions surface). New packages print "new, no baseline";
# packages that vanished from the run print "dropped". Review the
# fresh summary and commit it together with the change that moved
# the numbers.
#
# Usage: task test:cover (or bash tools/coverage_delta.sh)

set -euo pipefail

DROP_LIMIT="${COVER_DROP_LIMIT:-2.0}"
REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
RUNS_DIR="${REPO_DIR}/runs"
PREV="${RUNS_DIR}/coverage-latest.txt"
CUR="${RUNS_DIR}/.coverage-current.txt"

# cleanup_run_log drops the scratch files of the run - unless
# KEEP_RUN_LOG=1 asks to preserve the per package run log (the CI
# coverage job feeds the same log to coverage_report.sh -from-log,
# so the suite runs once, not twice).
cleanup_run_log() {
    rm -f "${CUR}"
    if [ "${KEEP_RUN_LOG:-0}" != "1" ]; then
        rm -f "${RUNS_DIR}/.cover-run.log"
    fi
}

cd "${REPO_DIR}"
mkdir -p "${RUNS_DIR}"

echo "Running the suite with coverage (the raw profile: runs/cover.out)"
go test ./... -cover -count=1 -coverprofile=runs/cover.out | \
    tee "${RUNS_DIR}/.cover-run.log"

# Extract the "ok  <pkg>  <time>  coverage: <pct>% of statements"
# lines into the sorted "pkg pct" summary. Packages without
# statements ("coverage: [no statements]") and the [no test files]
# entries stay out.
python3 - "${RUNS_DIR}/.cover-run.log" "${CUR}" <<'PYEOF'
import re, sys

run_log, out_path = sys.argv[1], sys.argv[2]
rows = {}
pattern = re.compile(
    r"^ok\s+(\S+)\s+\S+\s+coverage:\s+([0-9.]+)% of statements")
with open(run_log, encoding="utf-8") as handle:
    for line in handle:
        match = pattern.match(line)
        if match:
            rows[match.group(1)] = float(match.group(2))
with open(out_path, "w", encoding="utf-8") as out:
    for package in sorted(rows):
        out.write("%s %.1f\n" % (package, rows[package]))
print("packages with coverage: %d" % len(rows))
PYEOF

if [ ! -f "${PREV}" ]; then
    cp "${CUR}" "${PREV}"
    cleanup_run_log
    total=$(go tool cover -func=runs/cover.out | tail -n 1 | awk '{print $NF}')
    echo "Total statement coverage: ${total}"
    echo "First coverage summary written to ${PREV} (commit it as the baseline)"

    exit 0
fi

python3 - "${PREV}" "${CUR}" "${DROP_LIMIT}" <<'PYEOF'
import sys

prev_path, cur_path, limit = sys.argv[1], sys.argv[2], float(sys.argv[3])

def read(path):
    rows = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            parts = line.split()
            if len(parts) == 2:
                rows[parts[0]] = float(parts[1])
    return rows

prev, cur = read(prev_path), read(cur_path)
failures = []
print("\n| package | base | now | delta |")
print("| --- | --- | --- | --- |")
for package in sorted(set(prev) | set(cur)):
    old, new = prev.get(package), cur.get(package)
    if old is None:
        print("| %s | - | %.1f | new, no baseline |" % (package, new))
    elif new is None:
        print("| %s | %.1f | - | dropped from the run |" % (package, old))
    else:
        delta = new - old
        mark = "" if delta >= -limit else " REGRESSION"
        if delta < -limit:
            failures.append(package)
        print("| %s | %.1f | %.1f | %+.1f pp%s |"
              % (package, old, new, delta, mark))
print("")
if failures:
    print("Error coverage dropped by more than %.1f pp in %d package(s):"
          % (limit, len(failures)))
    for package in failures:
        print("  %s" % package)
    sys.exit(1)
print("Coverage deltas inside the %.1f pp budget" % limit)
PYEOF

total=$(go tool cover -func=runs/cover.out | tail -n 1 | awk '{print $NF}')
echo "Total statement coverage: ${total}"
cp "${CUR}" "${PREV}"
echo "Fresh summary written to ${PREV} (commit it together with the change)"
cleanup_run_log
