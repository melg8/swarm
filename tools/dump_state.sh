#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT

# tools/dump_state.sh - capture the state dump of a running bot for
# offline reproduction. The dump is the plain text report the web UI
# "Dump state" button copies to the clipboard, served by the
# GET /api/bots/{id}/dump endpoint of internal/swarm/webserver/dump.go.
#
# The dump carries the build identity, the character sheet, the world
# objects sorted by distance, the inventory, the walk plan, the combat
# events, the chat and the deep event log (600 entries) - everything
# an agent needs to reproduce a stuck/misbehaving bot without the user
# having to replay the situation by hand. See the dump-state-repro
# skill for the reproduction workflow.
#
# Usage:
#   tools/dump_state.sh                       list bots and exit
#   tools/dump_state.sh <bot-id>              dump to stdout
#   tools/dump_state.sh <bot-id> -o file     dump to file
#   tools/dump_state.sh <bot-id> | tee dump.txt
#
# Environment overrides:
#   SWARM_WEB_HOST   the web UI host:port (default 127.0.0.1:8080)
#   CURL             path to curl (default auto-detect)

set -euo pipefail

WEB_HOST="${SWARM_WEB_HOST:-127.0.0.1:8080}"
CURL_BIN="${CURL:-curl}"
OUT=""

usage() {
    cat <<EOF
Usage: $0 [<bot-id> [-o <file>]]

  no args           list the bot ids known to the web UI and exit
  <bot-id>          dump the bot state to stdout
  <bot-id> -o <f>   dump to file <f>

Env: SWARM_WEB_HOST (default 127.0.0.1:8080)
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help) usage; exit 0 ;;
        -o) OUT="$2"; shift 2 ;;
        *) BOT_ID="$1"; shift ;;
    esac
done

BOT_ID="${BOT_ID:-}"

if [ -z "$BOT_ID" ]; then
    echo "Bots known to the web UI at ${WEB_HOST}:"
    "${CURL_BIN}" -fsS "http://${WEB_HOST}/api/bots" 2>/dev/null \
        | tr ',' '\n' | grep -oE '"id":"[^"]+"' | sed 's/"id":"//;s/"$//' \
        | sed 's/^/  /'
    exit 0
fi

URL="http://${WEB_HOST}/api/bots/${BOT_ID}/dump"

if [ -n "$OUT" ]; then
    "${CURL_BIN}" -fsS "${URL}" -o "${OUT}"
    echo "Wrote ${OUT}"
else
    "${CURL_BIN}" -fsS "${URL}"
fi
