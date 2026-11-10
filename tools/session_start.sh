#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# session_start.sh prints the machine-checkable session bootstrap
# checklist: the ritual every agent re-implements by hand from the
# AGENTS.md prose, turned into one command with visible verdicts.
#
# It answers the four questions the session start ritual asks:
#   1. the git state - is the branch behind origin (rebase before
#      any push, other agents commit in parallel);
#   2. the deploy probe - is the Mobius login port listening
#      (checked through ss, never /dev/tcp - the flood protector
#      answers SYN differently);
#   3. the open hypotheses - the H-NNN entries with Status: open
#      (the work rule: advance or close one per session when the
#      touched area matches its verification plan);
#   4. the active tasks - the task fragments in docs/progress/
#      (one file per task, newest by name; resume it or close it
#      first - the board's "In progress" column names the owner).
#
# Usage: bash tools/session_start.sh
# Environment: SKIP_FETCH=1 skips the origin fetch (offline runs).

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
BRANCH="$(git -C "${REPO_DIR}" rev-parse --abbrev-ref HEAD)"

mark() {
    if [ "$2" = "ok" ]; then
        echo "[ok] $1"
    else
        echo "[!!] $1"
    fi
}

# 1. The git state (fetch, then count).
if [ "${SKIP_FETCH:-0}" != "1" ]; then
    if git -C "${REPO_DIR}" fetch origin --quiet 2>/dev/null; then
        behind="$(git -C "${REPO_DIR}" rev-list --count HEAD..origin/${BRANCH} 2>/dev/null || echo '?')"
        ahead="$(git -C "${REPO_DIR}" rev-list --count origin/${BRANCH}..HEAD 2>/dev/null || echo '?')"
        if [ "${behind}" = "0" ]; then
            mark "git: ${BRANCH} in sync with origin (${ahead} unpushed commits)" "ok"
        else
            mark "git: ${BRANCH} is ${behind} commit(s) behind origin - fetch + rebase origin/${BRANCH} before any work or push" "warn"
        fi
    else
        mark "git: fetch failed (offline?) - rebase before the push anyway" "warn"
    fi
fi

# 2. The login port probe (ss, never /dev/tcp).
if command -v ss > /dev/null 2>&1; then
    if ss -ltn 2>/dev/null | grep -q ':2106 '; then
        mark "login port 2106 listening - the Mobius stack is up" "ok"
    else
        mark "login port 2106 NOT listening - verify the deploy (tools/mobius_start.sh) before any live scenario" "warn"
    fi
else
    mark "ss unavailable - probe port 2106 by hand (never /dev/tcp)" "warn"
fi

# 3. The open hypotheses.
open_hypotheses="$(awk '/^### H-[0-9]+/ { id = $2; sub(/:$/, "", id) }
    /^- Status: open/ && id != "" { printf "%s ", id; id = "" }' \
    "${REPO_DIR}/docs/hypotheses.md" 2>/dev/null)"
if [ -n "${open_hypotheses}" ]; then
    mark "open hypotheses: ${open_hypotheses}- advance or close one this session when the touched area matches its plan (docs/hypotheses.md)" "warn"
else
    mark "open hypotheses: none (the registry is clean)" "ok"
fi

# 4. The active task fragments (docs/progress/, one file per task).
progress_dir="${REPO_DIR}/docs/progress"
if [ -d "${progress_dir}" ]; then
    fragments=$(find "${progress_dir}" -maxdepth 1 -type f -name '*.md' ! -name 'README.md' | sort)
    fragment_count=$(printf '%s\n' "${fragments}" | grep -c . || true)
    if [ "${fragment_count}" -gt 0 ]; then
        newest=$(printf '%s\n' "${fragments}" | tail -n 1)
        headline=$(grep -m1 -v '^$' "${newest}" 2>/dev/null | head -c 120)
        mark "active task fragments: ${fragment_count} in docs/progress/ (newest: $(basename "${newest}")) - ${headline}" "ok"
    else
        mark "no active fragments in docs/progress/ - take work from the board (Ready) and start a fragment per docs/progress/README.md" "warn"
    fi
else
    mark "docs/progress/ is missing - check the checkout (the legacy docs/agent_progress.md is a frozen archive, never appended)" "warn"
fi

echo "session_start: checklist done (the [!!] lines are the work order)"
