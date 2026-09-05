#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# End-to-end test of the swarm bot against the local Mobius C1 stack:
# build the bot, start the stack (idempotent), keep the bot in the world for
# N seconds, then send SIGINT and check the graceful shutdown.
#
# Usage: tools/mobius_e2e.sh [seconds_in_world] (default 45)
#
# Extra bot flags can be passed with the BOT_FLAGS environment variable,
# for example BOT_FLAGS="-hunt" enables the auto hunt loop.
#
# Everything runs inside this single invocation, which also works in
# restricted sandbox shells where background processes do not survive the
# invoking shell.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=mobius_env.sh
source "${SCRIPT_DIR}/mobius_env.sh"

log() { echo "[e2e] ${*}"; }
die() { echo "Error ${*}"; exit 1; }

SECONDS_IN_WORLD="${1:-45}"

# Common go toolchain locations for sandbox and normal hosts.
for candidate in /usr/local/go/bin "${HOME}/.local/go/bin"; do
    if [ -x "${candidate}/go" ]; then
        PATH="${candidate}:${PATH}"
    fi
done
export PATH
command -v go >/dev/null 2>&1 || die "go toolchain not found in PATH"

mkdir -p "${LOGS_DIR}"

log "Building the swarm bot"
cd "${SWARM_ROOT}"
go build -o "${LOGS_DIR}/swarm_bot" ./cmd/swarm
log "Bot built: ${LOGS_DIR}/swarm_bot"

log "Starting the server stack"
bash "${SCRIPT_DIR}/mobius_start.sh"

log "Running the bot for ${SECONDS_IN_WORLD}s, then SIGINT"
set +e
timeout -s INT --preserve-status "${SECONDS_IN_WORLD}s" \
    "${LOGS_DIR}/swarm_bot" ${BOT_FLAGS:-} 2>&1 | tee "${LOGS_DIR}/bot.log"
BOT_EXIT=$?
set -e

log "Bot exit code: ${BOT_EXIT}"
log "Packets received: $(grep -c 'Received packet' "${LOGS_DIR}/bot.log" || true)"
log "Game server log tail:"
tail -15 "${LOGS_DIR}/game.log" || true

[ "${BOT_EXIT}" -eq 0 ] || die "bot exited with code ${BOT_EXIT}"
log "E2E_OK"
