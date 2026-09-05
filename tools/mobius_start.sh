#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Start the local L2J Mobius C1 stack: MariaDB + LoginServer (port 2106) +
# GameServer (port 7777). Idempotent: components that are already listening
# on their ports are left alone. Prints STACK_READY when everything is up.
#
# Run tools/mobius_bootstrap.sh first on a clean machine.
#
# Note for restricted sandbox shells: background processes are killed when
# the invoking shell exits there, so run the bot in the same invocation via
# tools/mobius_e2e.sh instead of calling this script separately.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=mobius_env.sh
source "${SCRIPT_DIR}/mobius_env.sh"

log() { echo "[stack] ${*}"; }
die() { echo "Error ${*}"; exit 1; }

[ -x "${JDK_DIR}/bin/java" ] || die "JDK missing, run tools/mobius_bootstrap.sh"
[ -d "${MOBIUS_BUILD_DIR}" ] || die "Mobius classes missing, run tools/mobius_bootstrap.sh"

mkdir -p "${LOGS_DIR}"

# --- MariaDB -----------------------------------------------------------------
start_mariadb

# --- Login server -------------------------------------------------------------
if port_open "${LOGIN_PORT}"; then
    log "Login server already running on ${LOGIN_PORT}"
else
    log "Starting login server"
    cd "${MOBIUS_C1}/dist/login"
    nohup "${JDK_DIR}/bin/java" -server -Dfile.encoding=UTF-8 \
        -Dorg.slf4j.simpleLogger.log.com.zaxxer.hikari=warn \
        ${LOGIN_JAVA_MEM} \
        -cp "${MOBIUS_BUILD_DIR}:${MOBIUS_LIBS_DIR}/*" \
        org.l2jmobius.loginserver.LoginServer \
        >"${LOGS_DIR}/login.log" 2>&1 &
    log "Login server starting (pid ${!})"
fi
wait_port "${LOGIN_PORT}" "${WAIT_LOGIN_SECS}" "login server" || {
    tail -30 "${LOGS_DIR}/login.log" || true
    die "login server failed"
}

# --- Game server --------------------------------------------------------------
if port_open "${GAME_PORT}"; then
    log "Game server already running on ${GAME_PORT}"
else
    log "Starting game server (world init takes a while)"
    cd "${MOBIUS_C1}/dist/game"
    nohup "${JDK_DIR}/bin/java" -server -Dfile.encoding=UTF-8 \
        -Dorg.slf4j.simpleLogger.log.com.zaxxer.hikari=warn \
        ${GAME_JAVA_MEM} \
        -cp "${MOBIUS_BUILD_DIR}:${MOBIUS_LIBS_DIR}/*" \
        org.l2jmobius.gameserver.GameServer \
        >"${LOGS_DIR}/game.log" 2>&1 &
    log "Game server starting (pid ${!})"
fi
wait_port "${GAME_PORT}" "${WAIT_GAME_SECS}" "game server" || {
    tail -40 "${LOGS_DIR}/game.log" || true
    die "game server failed"
}

# --- Game server registration with the login server ---------------------------
# A game server that was started (or restarted) while the login server was
# down re-registers itself within seconds, but the server list stays empty
# until then. Wait for the registration line in the login log.
WAIT_REGISTER_SECS="${WAIT_REGISTER_SECS:-30}"
registered=0
for i in $(seq 1 "${WAIT_REGISTER_SECS}"); do
    registered="$(grep -c 'Updated Gameserver' "${LOGS_DIR}/login.log" || true)"
    if [ "${registered}" -ge 1 ]; then
        break
    fi
    sleep 1
done
if [ "${registered}" -lt 1 ]; then
    tail -20 "${LOGS_DIR}/login.log" || true
    die "game server did not register with the login server"
fi
log "Game server registered with the login server"

log "STACK_READY: login :${LOGIN_PORT}, game :${GAME_PORT}, db :${MYSQL_PORT}"
log "Logs in ${LOGS_DIR} (login.log, game.log, mariadb.log)"
