#!/bin/bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Live reproduction of the farm readiness stuck round (2026-09-20 owner
# report): the level 15 character is placed at the jewelry shop porch
# (44584 46944 -2920, the plan repro origin the owner attached) with
# the level 15 wallet (100k adena, 20k SP, no gear, no skills) and one
# hunt session runs. The trip must buy at the village merchants and
# walk the reported Ariel -> Herbiel leg across the plaza. The report:
# the bot steps away from the route origin one step, walks back,
# repeats several times, then gives the merchant up and builds a short
# route toward the farm zone.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tools/mobius_env.sh
source "${SCRIPT_DIR}/mobius_env.sh"

# The reported plan repro origin (the jewelry shop porch) and the
# level 15 vitals of the acceptance scenario.
STUCK_X=44584
STUCK_Y=46944
STUCK_Z=-2920
LEVEL15_EXP=254327
SECONDS_IN_WORLD="${SECONDS_IN_WORLD:-240}"

log() { echo "[repro-jewel] $*"; }
die() { echo "[repro-jewel] FAIL: $*" >&2; exit 1; }

MARIADB="${MARIADB_DIR}/bin/mariadb"
[ -x "${MARIADB}" ] || die "mariadb client not found at ${MARIADB}"
[ -d "${LOGS_DIR}" ] || die "logs directory ${LOGS_DIR} not found"
port_open "${LOGIN_PORT}" || die "login server not on :${LOGIN_PORT}"
port_open "${GAME_PORT}" || die "game server not on :${GAME_PORT}"

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

CHAR_ID=$(db -N -e "SELECT charId FROM characters WHERE char_name='temp1'")
[ -n "${CHAR_ID}" ] || die "character temp1 not found in the database"

# The level 15 wallet of the farm readiness scenario at the reported
# porch position: the weapon run trip arms at once (no weapon, empty
# bag) and walks the merchant ring the report froze on. The wipe
# covers the paperdoll rows too (the starter kit sits equipped) - the
# acceptance resetCharacter wipes the owned rows the same way.
db -e "DELETE FROM items WHERE owner_id=${CHAR_ID}"
db -e "DELETE FROM character_skills WHERE charId=${CHAR_ID}"
db -e "DELETE FROM character_skills_save WHERE charId=${CHAR_ID}"
db -e "DELETE FROM character_shortcuts WHERE charId=${CHAR_ID}"
db -e "UPDATE characters SET x=${STUCK_X}, y=${STUCK_Y}, z=${STUCK_Z}, \
    level=15, exp=${LEVEL15_EXP}, sp=20000, maxHp=280, maxMp=111, \
    maxCp=112, online=0 WHERE charId=${CHAR_ID}"
db -e "INSERT INTO items (owner_id, object_id, item_id, count, loc, \
    loc_data, mana_left, time) VALUES (${CHAR_ID}, 268463001, 57, \
    100000, 'INVENTORY', 0, -1, 0)"
log "character temp1 placed at ${STUCK_X} ${STUCK_Y} ${STUCK_Z}, level 15 wallet"

cd "${SWARM_ROOT}"
go build -o "${LOGS_DIR}/swarm_bot" ./cmd/swarm
log "bot built"

RUN_LOG="${LOGS_DIR}/repro_jewel_run.log"
rm -f "${RUN_LOG}"
timeout -s INT --preserve-status "${SECONDS_IN_WORLD}s" \
    "${LOGS_DIR}/swarm_bot" -hunt -account temp1 -password temp1 \
    -char temp1 -web "" \
    > "${RUN_LOG}" 2>&1 &
BOT_PID=$!

for _ in $(seq 1 $((SECONDS_IN_WORLD + 30))); do
    kill -0 "${BOT_PID}" 2>/dev/null || break
    sleep 1
done
kill -KILL "${BOT_PID}" 2>/dev/null || true
wait "${BOT_PID}" 2>/dev/null || true

echo
log "hunt log of the session:"
grep -E "Hunt:" "${RUN_LOG}" | head -60
echo
log "the walk plan lines and stuck evidence:"
grep -cE "cursor key escape" "${RUN_LOG}" || true
grep -E "stuck|escape|banning|widening|refuse|no route|closest" "${RUN_LOG}" | head -40
log "REPRO_DONE (inspect the log above)"
