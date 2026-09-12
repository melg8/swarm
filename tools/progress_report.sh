#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# progress_report.sh renders PROGRESS.md from three live sources:
#   1. the tail of runs/metrics.jsonl (the soak metrics trail)
#   2. the task statuses of docs/BACKLOG.md (todo / in_progress / done)
#   3. the last 20 commits of git log --oneline
#
# The output is the one-page human dashboard of the project: the
# milestone ladder green/red by the last metrics run, the active
# BACKLOG tasks and the recent commits. Run it after every
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
BACKLOG="${REPO_DIR}/docs/BACKLOG.md"
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

# render_backlog extracts the task headers and their status lines.
render_backlog() {
        if [ ! -f "${BACKLOG}" ]; then
                echo "_docs/BACKLOG.md not found._"
                return
        fi
        echo "| Task | Status | Milestone | Priority |"
        echo "| --- | --- | --- | --- |"
        awk '
                /^### T-[0-9]+:/ { id = $2; gsub(/:$/, "", id) }
                /^status: / { status = $2 }
                /^milestone: / { ms = $2 }
                /^priority: / {
                        prio = $2
                        if (id != "")
                                printf "| %s | %s | %s | %s |\n", id, status, ms, prio
                }
        ' "${BACKLOG}"
}

# render_commits shows the last 20 commits.
render_commits() {
        git -C "${REPO_DIR}" log --oneline -20 2>/dev/null || \
                echo "_git log unavailable_"
}

# render_milestone reads the last metrics status per scenario and
# colors the M1 milestone: PASS green, FAIL red, none pending.
render_milestone() {
        local last
        if [ -f "${METRICS}" ]; then
                last=$(tail -n 1 "${METRICS}" 2>/dev/null || true)
        else
                last=""
        fi
        if [ -z "${last}" ]; then
                echo "**M1 (soak proof):** pending (no metrics yet)"
                return
        fi
        local status
        status=$(printf '%s' "${last}" | python3 -c '
import json, sys
try:
    print(json.loads(sys.stdin.read()).get("status", "pending"))
except Exception:
    print("pending")
' 2>/dev/null || echo "pending")
        case "${status}" in
                PASS) echo "**M1 (soak proof):** green (last soak PASS)" ;;
                FAIL) echo "**M1 (soak proof):** red (last soak FAIL)" ;;
                *)    echo "**M1 (soak proof):** pending (last soak ${status})" ;;
        esac
}

{
        echo "# PROGRESS"
        echo
        echo "Generated: $(date -u +'%Y-%m-%d %H:%M:%SZ')"
        echo
        echo "## Milestone"
        echo
        render_milestone
        echo
        echo "## Soak metrics (last 10 runs)"
        echo
        render_metrics
        echo
        echo "## BACKLOG"
        echo
        render_backlog
        echo
        echo "## Recent commits"
        echo
        echo '```'
        render_commits
        echo '```'
} > "${OUT}"

echo "wrote ${OUT}"
