#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# Visual verification of the webui against the preview server
# (tools/webui_preview_server.js): a synthetic fighting snapshot goes
# into the real web app through agent-browser eval and the three
# tricky areas get measured in the live DOM - the chat line rhythm
# (every row an exact 18px multiple, gap == previous row height), the
# quest sub tab (the grid swap with a stable panel height, the badge
# count) and the status banner wording (the mob name, no raw id).
# Leaves three screenshots in the output directory.
#
# Usage: bash tools/webui_fix_verify.sh [shot-dir]
# Needs: node, agent-browser (headless browser CLI).
# PORT overrides the preview port (8093 default) so concurrent runs
# never collide.
set -uo pipefail
PORT="${PORT:-8093}"
TOOL_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$TOOL_DIR")"
cd "$REPO_DIR"

SHOT_DIR="${1:-/tmp/webui_verify}"
mkdir -p "$SHOT_DIR"

node tools/webui_preview_server.js "$PORT" > /tmp/preview.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null; agent-browser close 2>/dev/null' EXIT
sleep 1

agent-browser set viewport 1360 900 > /dev/null 2>&1
agent-browser open "http://127.0.0.1:$PORT" 2>&1 | tail -1
sleep 2

agent-browser eval "$(cat "$TOOL_DIR/webui_fix_inject.js")" 2>&1 | tail -2

echo "--- banner text/detail ---"
agent-browser eval "document.getElementById('bot-status-text').textContent + ' || ' + document.getElementById('bot-status-detail').textContent" 2>&1 | tail -1

echo "--- chat line rhythm (heights and gaps, px) ---"
agent-browser eval "$(cat "$TOOL_DIR/webui_fix_measure_chat.js")" 2>&1 | tail -1

echo "--- gear panel height stability across the tab switch ---"
agent-browser eval "$(cat "$TOOL_DIR/webui_fix_measure_quest.js")" 2>&1 | tail -1

agent-browser screenshot "$SHOT_DIR/webui_fix_gear_items_tab.png" 2>&1 | tail -1
agent-browser eval "document.getElementById('inv-tab-quest').click(); 'switched'" 2>&1 | tail -1
agent-browser screenshot "$SHOT_DIR/webui_fix_gear_quest_tab.png" 2>&1 | tail -1
agent-browser eval "document.getElementById('inv-tab-items').click(); 'back'" 2>&1 | tail -1
agent-browser screenshot "$SHOT_DIR/webui_fix_chat_and_banner.png" 2>&1 | tail -1

kill $SRV 2>/dev/null
agent-browser close > /dev/null 2>&1
trap - EXIT
echo "DONE"
