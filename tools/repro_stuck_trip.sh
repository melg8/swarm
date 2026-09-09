#!/bin/bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Live reproduction of the round 35 stuck report (see
# docs/development_log.md): the character test1 is placed at the exact
# reported stuck position (45544 45880 -2992, the pond deck edge west
# of the elven village shop quarter) with a full bag of junk, and the
# bot runs one hunt session: the town trip must walk to the trader
# Unoren over the geodata instead of stalling at the deck edge. The
# state dump is captured mid-walk - the same material the user's
# problem report was made of.
#
# The character must be offline (the stack up, no bot connected); the
# script moves it through the database and inserts 41 non stackable
# daggers as the trip trigger (stackable junk merges into one slot on
# login and never fills the bag). The trip itself sells the daggers,
# so the run cleans its own trigger state up.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tools/mobius_env.sh
source "${SCRIPT_DIR}/mobius_env.sh"

# The reported stuck position and the shop goal (round 35).
STUCK_X=45544
STUCK_Y=45880
STUCK_Z=-2992
JUNK_ITEM_ID=10 # Dagger: non stackable, sellable, fills real slots.
JUNK_COUNT=41   # 41 of 80 slots passes the 50 percent trip trigger.
SECONDS_IN_WORLD="${SECONDS_IN_WORLD:-90}"

log() { echo "[repro-stuck] $*"; }
die() { echo "[repro-stuck] FAIL: $*" >&2; exit 1; }

MARIADB="${MARIADB_DIR}/bin/mariadb"
[ -x "${MARIADB}" ] || die "mariadb client not found at ${MARIADB}"
[ -d "${LOGS_DIR}" ] || die "logs directory ${LOGS_DIR} not found"
port_open "${LOGIN_PORT}" || die "login server not on :${LOGIN_PORT}"
port_open "${GAME_PORT}" || die "game server not on :${GAME_PORT}"

# The go toolchain of the fast deploy layout when the caller did not
# export it (the AGENTS.md PATH export covers this otherwise).
if ! command -v go >/dev/null 2>&1; then
    for candidate in "${OPT_DIR}"/go-root/usr/lib/go-*/bin; do
        if [ -x "${candidate}/go" ]; then
            PATH="${candidate}:${PATH}"
            export PATH
            break
        fi
    done
fi
command -v go >/dev/null 2>&1 || die "go toolchain not found on PATH"

db() {
    "${MARIADB}" --socket="${MYSQL_SOCK}" -u root "${MYSQL_DB_NAME}" "$@"
}

CHAR_ID=$(db -N -e "SELECT charId FROM characters WHERE char_name='test1'")
[ -n "${CHAR_ID}" ] || die "character test1 not found in the database"

# Move the offline character to the stuck spot and arm the trip
# trigger: fresh junk daggers with fresh object ids (the old ones were
# sold by the previous reproduction runs).
db -e "DELETE FROM items WHERE item_id=${JUNK_ITEM_ID} AND loc='INVENTORY' \
    AND owner_id=${CHAR_ID} AND object_id>=268461000"
db -e "UPDATE characters SET x=${STUCK_X}, y=${STUCK_Y}, z=${STUCK_Z} \
    WHERE char_name='test1'"
for i in $(seq 1 "${JUNK_COUNT}"); do
    db -e "INSERT INTO items (owner_id, object_id, item_id, count, loc, \
        loc_data, mana_left, time) VALUES (${CHAR_ID}, 2684620$(printf %02d "${i}"), \
        ${JUNK_ITEM_ID}, 1, 'INVENTORY', 0, -1, 0)"
done
log "character placed at ${STUCK_X} ${STUCK_Y} ${STUCK_Z} with ${JUNK_COUNT} junk daggers"

# Build the bot with the identity flags (the dump must name the exact
# code it came from, see round 41).
cd "${SWARM_ROOT}"
SWARM_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
[ "${SWARM_BRANCH}" = "HEAD" ] && SWARM_BRANCH=""
SWARM_COMMIT="$(git rev-parse HEAD 2>/dev/null || true)"
SWARM_DIRTY="false"
[ -n "$(git status --porcelain 2>/dev/null)" ] && SWARM_DIRTY="true"
go build -ldflags "\
-X github.com/melg8/swarm/internal/version.Branch=${SWARM_BRANCH} \
-X github.com/melg8/swarm/internal/version.Commit=${SWARM_COMMIT} \
-X github.com/melg8/swarm/internal/version.Dirty=${SWARM_DIRTY} \
-X github.com/melg8/swarm/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o "${LOGS_DIR}/swarm_bot" ./cmd/swarm
log "bot built: commit ${SWARM_COMMIT:0:40} dirty=${SWARM_DIRTY}"

RUN_LOG="${LOGS_DIR}/repro_stuck_run.log"
DUMP="${LOGS_DIR}/repro_stuck_dump.txt"
rm -f "${RUN_LOG}" "${DUMP}"
timeout -s INT --preserve-status "${SECONDS_IN_WORLD}s" \
    "${LOGS_DIR}/swarm_bot" -hunt > "${RUN_LOG}" 2>&1 &
BOT_PID=$!

# The walk from the stuck spot to the shop ring takes ~13 s: capture
# the dump while the trip is walking.
sleep 7
curl -s --max-time 5 "http://127.0.0.1:8080/api/bots/test1/dump" > "${DUMP}" || true

# Wait out the session window (timeout delivers the SIGINT itself).
for _ in $(seq 1 $((SECONDS_IN_WORLD + 30))); do
    kill -0 "${BOT_PID}" 2>/dev/null || break
    sleep 1
done
kill -KILL "${BOT_PID}" 2>/dev/null || true
wait "${BOT_PID}" 2>/dev/null || true

echo
log "state dump (character + walk plan at the capture moment):"
sed -n '1,16p' "${DUMP}"
WALK_PLAN_LINE=$(grep -n 'walk plan' "${DUMP}" | head -1 | cut -d: -f1)
[ -n "${WALK_PLAN_LINE}" ] && sed -n "${WALK_PLAN_LINE},$((WALK_PLAN_LINE+8))p" "${DUMP}"
echo
log "hunt log of the session (the trip story):"
grep -E "Hunt: (inventory|shop|trading|offered|town|walking|back)" "${RUN_LOG}" | head -15
echo
grep -q "Hunt: town walk stuck" "${RUN_LOG}" && \
    die "the walk reported a stuck re-path - the reproduction FAILED"
grep -q "Hunt: shop reached" "${RUN_LOG}" || \
    die "the trip never reached the shop - the reproduction FAILED"
log "REPRO_OK: the trip from the stuck spot reached the shop without a stuck re-path"
