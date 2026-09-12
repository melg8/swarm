#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# progress_report.sh renders PROGRESS.md from three live sources:
#   1. the milestone ladder of docs/ROADMAP.md (done / green / red / pending)
#   2. the tail of runs/metrics.jsonl (the soak metrics trail)
#   3. the last 20 commits of git log --oneline
#
# The output is the one-page human dashboard of the project: the
# milestone ladder green/red by the last metrics run, the recent
# soak metrics and the recent commits. Run it after every
# milestone-relevant acceptance run; the page history is the project
# history.
#
# The script is dependency-light: bash, python3 (for the JSONL parse),
# git. It writes PROGRESS.md to the repository root by default, or to
# the path given as the first argument.

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
OUT="${1:-${REPO_DIR}/PROGRESS.md}"
METRICS="${REPO_DIR}/runs/metrics.jsonl"
ROADMAP="${REPO_DIR}/docs/ROADMAP.md"

# render_metrics turns the tail of the JSONL trail into a markdown
# table. python3 parses each line (the file is one JSON object per
# line); the last 10 rows ride the page. A missing file prints a
# placeholder line so the page is never empty on a fresh clone.
render_metrics() {
        if [ ! -f "${METRICS}" ]; then
                echo "_No metrics trail yet (runs/metrics.jsonl missing). " \
                        "Run \`-acceptance soak\` to seed it._"
                return
        fi
        local lines
        lines=$(tail -n 10 "${METRICS}" || true)
        if [ -z "${lines}" ]; then
                echo "_The metrics trail is empty._"
                return
        fi
        echo "| Date | Scenario | Duration | Start | End | XP/h | Deaths | Adena | Stuck | Status |"
        echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
        printf '%s\n' "${lines}" | python3 -c '
import json, sys
for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    try:
        row = json.loads(raw)
    except json.JSONDecodeError:
        continue
    xp = row.get("xpPerHour", 0)
    print("| {} | {} | {}s | {} | {} | {:.0f} | {} | {} | {} | {} |".format(
        row.get("date", ""),
        row.get("scenario", ""),
        row.get("durationSec", 0),
        row.get("startLevel", 0),
        row.get("endLevel", 0),
        xp,
        row.get("deaths", 0),
        row.get("adena", 0),
        row.get("stuckEvents", 0),
        row.get("status", ""),
    ))
'
}

# render_commits shows the last 20 commits.
render_commits() {
        git -C "${REPO_DIR}" log --oneline -20 2>/dev/null || \
                echo "_git log unavailable_"
}

# render_milestone builds the full milestone ladder from
# docs/ROADMAP.md: a heading `## M<N> - <title> (DONE)` marks a closed
# milestone (green), the first open milestone is the current one and
# colors green/red by the last metrics row status (PASS/FAIL), the
# rest stay pending. python3 keeps the heading parse and the ladder
# walk in one place.
render_milestone() {
        python3 -c '
import json, os, re, sys

repo = sys.argv[1]
roadmap = os.path.join(repo, "docs", "ROADMAP.md")
metrics = os.path.join(repo, "runs", "metrics.jsonl")

last_status = ""
if os.path.isfile(metrics):
    with open(metrics, encoding="utf-8") as handle:
        lines = [l for l in handle.read().splitlines() if l.strip()]
    if lines:
        try:
            row = json.loads(lines[-1])
            last_status = row.get("status", "")
        except json.JSONDecodeError:
            pass

steps = []
if os.path.isfile(roadmap):
    with open(roadmap, encoding="utf-8") as handle:
        for line in handle:
            m = re.match(r"^## (M[0-9]+) - (.+?)(?:\s+\(DONE\))?\s*$", line)
            if not m:
                continue
            title = m.group(2).strip()
            done = line.rstrip().endswith("(DONE)")
            steps.append((m.group(1), title, done))

current = True
for mid, title, done in steps:
    head = "**%s (%s):**" % (mid, title)
    if done:
        print("%s done" % head)
        continue
    if current:
        current = False
        if last_status == "PASS":
            print("%s green (last run PASS)" % head)
        elif last_status == "FAIL":
            print("%s red (last run FAIL)" % head)
        elif last_status:
            print("%s pending (last run %s)" % (head, last_status))
        else:
            print("%s pending (no metrics yet)" % head)
        continue
    print("%s pending" % head)
if not steps:
    print("_docs/ROADMAP.md milestones not found._")
' "${REPO_DIR}"
}

{
        echo "# PROGRESS"
        echo
        echo "Generated: $(date -u +'%Y-%m-%d %H:%M:%SZ')"
        echo
        echo "## Milestone ladder"
        echo
        render_milestone
        echo
        echo "## Soak metrics (last 10 runs)"
        echo
        render_metrics
        echo
        echo "## Recent commits"
        echo
        echo '```'
        render_commits
        echo '```'
} > "${OUT}"

echo "wrote ${OUT}"
