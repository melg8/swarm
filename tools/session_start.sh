#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# session_start.sh prints the machine-checkable session bootstrap
# checklist: the ritual every agent re-implements by hand from the
# AGENTS.md prose, turned into one command with visible verdicts.
#
# It answers the five questions the session start ritual asks:
#   1. the timer stamp - is /home/z/my-project/.session_start_ts
#      written and how much of the 2h budget (hard stop 1h45m) is
#      already gone;
#   2. the git state - is the branch behind origin (rebase before
#      any push, other agents commit in parallel);
#   3. the deploy probe - is the Mobius login port listening
#      (checked through ss, never /dev/tcp - the flood protector
#      answers SYN differently);
#   4. the open hypotheses - the H-NNN entries with Status: open
#      (the work rule: advance or close one per session when the
#      touched area matches its verification plan);
#   5. the active task - the headline of the newest Progress entry
#      in docs/agent_progress.md (resume it or close it first).
#
# Usage: bash tools/session_start.sh
# Environment: SKIP_FETCH=1 skips the origin fetch (offline runs).

set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
STAMP="/home/z/my-project/.session_start_ts"
BRANCH="$(git -C "${REPO_DIR}" rev-parse --abbrev-ref HEAD)"
BUDGET_SECONDS=7200        # the 2h process life the owner enforces
HARD_STOP_SECONDS=6300     # everything pushed by 1h45m

mark() {
    if [ "$2" = "ok" ]; then
        echo "[ok] $1"
    else
        echo "[!!] $1"
    fi
}

# 1. The session timer stamp.
if [ -f "${STAMP}" ]; then
    stamp_value="$(cat "${STAMP}" 2>/dev/null || echo 0)"
    now="$(date +%s)"
    age=$(( now - stamp_value ))
    if [ "${age}" -lt 0 ]; then
        mark "session stamp ${stamp_value} is in the future - restamp it (date +%s > ${STAMP})" "warn"
    else
        left=$(( BUDGET_SECONDS - age ))
        stop_left=$(( HARD_STOP_SECONDS - age ))
        mark "session stamp ok, ${age}s spent, ${left}s of budget left, hard stop in ${stop_left}s - push before it" "ok"
    fi
else
    mark "no session stamp - run: date +%s > ${STAMP} (the 2h budget measures from it)" "warn"
fi

# 2. The git state (fetch, then count).
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

# 3. The login port probe (ss, never /dev/tcp).
if command -v ss > /dev/null 2>&1; then
    if ss -ltn 2>/dev/null | grep -q ':2106 '; then
        mark "login port 2106 listening - the Mobius stack is up" "ok"
    else
        mark "login port 2106 NOT listening - verify the deploy (tools/mobius_start.sh) before any live scenario" "warn"
    fi
else
    mark "ss unavailable - probe port 2106 by hand (never /dev/tcp)" "warn"
fi

# 4. The open hypotheses.
open_hypotheses="$(awk '/^### H-[0-9]+/ { id = $2 }
    /^- Status: open/ && id != "" { print id; id = "" }' \
    "${REPO_DIR}/AGENTS.md" 2>/dev/null | tr '\n' ' ')"
if [ -n "${open_hypotheses}" ]; then
    mark "open hypotheses: ${open_hypotheses}- advance or close one this session when the touched area matches its plan (AGENTS.md, the hypotheses registry)" "warn"
else
    mark "open hypotheses: none (the registry is clean)" "ok"
fi

# 5. The active task headline (the newest Progress entry).
headline="$(grep -n '^### Progress' "${REPO_DIR}/docs/agent_progress.md" 2>/dev/null | tail -n 1 | cut -d: -f1)"
if [ -n "${headline}" ]; then
    active="$(sed -n "$(( headline + 1 )),$(( headline + 2 ))p" \
        "${REPO_DIR}/docs/agent_progress.md" | grep -v '^$' | head -n 1)"
    if [ -n "${active}" ]; then
        mark "active task (docs/agent_progress.md, newest entry): ${active}" "ok"
    else
        mark "the newest Progress entry has no headline - fix docs/agent_progress.md" "warn"
    fi
else
    mark "no Progress entries found - docs/agent_progress.md is missing or empty" "warn"
fi

echo "session_start: checklist done (the [!!] lines are the work order)"
