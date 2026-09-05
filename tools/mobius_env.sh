# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# Shared configuration and helpers for the Mobius C1 deployment tools.
# Sourced by mobius_bootstrap.sh, mobius_start.sh and mobius_e2e.sh.
# Every path variable can be overridden from the calling environment.

# Root of this swarm checkout (parent of tools/).
SWARM_TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWARM_ROOT="$(cd "${SWARM_TOOLS_DIR}/.." && pwd)"

# Third party binaries and runtime state (global, outside the repo).
OPT_DIR="${OPT_DIR:-${HOME}/opt}"
JDK_DIR="${JDK_DIR:-${OPT_DIR}/jdk25}"
MARIADB_DIR="${MARIADB_DIR:-${OPT_DIR}/mariadb}"
MYSQL_DATA_DIR="${MYSQL_DATA_DIR:-${HOME}/mysql_data}"
MYSQL_TMP_DIR="${MYSQL_TMP_DIR:-${HOME}/mysql_tmp}"
MYSQL_SOCK="${MYSQL_TMP_DIR}/mysql.sock"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_DB_NAME="${MYSQL_DB_NAME:-l2jmobiusc1}"
# Marker file that the Mobius SQL schema has been loaded.
SQL_LOADED_MARKER="${MYSQL_DATA_DIR}/.sql_loaded"

# Mobius sources, cloned next to the swarm checkout by default.
MOBIUS_ROOT="${MOBIUS_ROOT:-$(dirname "${SWARM_ROOT}")/l2j_mobius}"
MOBIUS_C1="${MOBIUS_C1:-${MOBIUS_ROOT}/L2J_Mobius_C1_HarbingersOfWar}"
MOBIUS_GIT_URL="https://gitlab.com/MobiusDevelopment/L2J_Mobius.git"
MOBIUS_BUILD_DIR="${MOBIUS_BUILD_DIR:-${MOBIUS_C1}/build_bin}"
MOBIUS_LIBS_DIR="${MOBIUS_C1}/dist/libs"
# Marker file that the java classes have been compiled.
MOBIUS_COMPILE_MARKER="${MOBIUS_BUILD_DIR}/.compile_ok"

# Stack logs, written next to the swarm checkout by default.
LOGS_DIR="${LOGS_DIR:-$(dirname "${SWARM_ROOT}")/logs}"

# Download sources used by mobius_bootstrap.sh only.
JDK_TARBALL_URL="https://api.adoptium.net/v3/binary/latest/25/ga/linux/x64/jdk/hotspot/normal/eclipse"
MARIADB_TARBALL_URL="https://archive.mariadb.org/mariadb-11.8.9/bintar-linux-systemd-x86_64/mariadb-11.8.9-linux-systemd-x86_64.tar.gz"

# Ports: 2106 login (clients), 9014 login (game server registration),
# 7777 game (clients), 3306 MariaDB.
LOGIN_PORT="${LOGIN_PORT:-2106}"
GAME_PORT="${GAME_PORT:-7777}"

# Port readiness timeouts in seconds.
WAIT_MYSQL_SECS="${WAIT_MYSQL_SECS:-30}"
WAIT_LOGIN_SECS="${WAIT_LOGIN_SECS:-40}"
WAIT_GAME_SECS="${WAIT_GAME_SECS:-150}"

# JVM memory flags for the two server processes.
LOGIN_JAVA_MEM="${LOGIN_JAVA_MEM:--Xms64m -Xmx256m}"
GAME_JAVA_MEM="${GAME_JAVA_MEM:--Xms256m -Xmx1300m}"

# Return 0 when something is listening on 127.0.0.1:$1. Prefers ss so no
# connection is made: the Mobius login server counts every accepted socket
# in its in-memory flood protector (FastConnectionTime 350 ms,
# FastConnectionLimit 15, MaxConnectionPerIP 50) and silently drops further
# connections, which breaks naive /dev/tcp readiness polling.
port_open() {
    if command -v ss >/dev/null 2>&1; then
        ss -ltn "sport = :${1}" 2>/dev/null | grep -q LISTEN
    else
        (exec 3<>"/dev/tcp/127.0.0.1/${1}") 2>/dev/null
    fi
}

# Wait until 127.0.0.1:$1 is listening, fail after $2 seconds. Adds a short
# settle delay after detection so the server finishes its final init steps
# before the first real client connects.
wait_port() {
    local port="$1" timeout="$2" name="$3" i
    for i in $(seq 1 "${timeout}"); do
        if port_open "${port}"; then
            sleep 1
            echo "${name} is up on port ${port}"
            return 0
        fi
        sleep 1
    done
    echo "Error ${name} did not open port ${port} within ${timeout} seconds"
    return 1
}

# Run the mariadb CLI as the passwordless socket root user.
mysql_root() {
    "${MARIADB_DIR}/bin/mariadb" --socket="${MYSQL_SOCK}" -u root "$@"
}

# Start MariaDB when it is not running yet. Idempotent.
start_mariadb() {
    if mysql_root -e "SELECT 1" >/dev/null 2>&1; then
        echo "MariaDB already running"
        return 0
    fi
    mkdir -p "${MYSQL_DATA_DIR}" "${MYSQL_TMP_DIR}" "${LOGS_DIR}"
    "${MARIADB_DIR}/bin/mariadbd" --no-defaults \
        --basedir="${MARIADB_DIR}" \
        --datadir="${MYSQL_DATA_DIR}" \
        --tmpdir="${MYSQL_TMP_DIR}" \
        --socket="${MYSQL_SOCK}" \
        --port="${MYSQL_PORT}" \
        --bind-address=127.0.0.1 \
        --user="${USER}" >"${LOGS_DIR}/mariadb.log" 2>&1 &
    local i
    for i in $(seq 1 "${WAIT_MYSQL_SECS}"); do
        if mysql_root -e "SELECT 1" >/dev/null 2>&1; then
            echo "MariaDB is up (socket ${MYSQL_SOCK})"
            return 0
        fi
        sleep 1
    done
    echo "Error MariaDB failed to start, see ${LOGS_DIR}/mariadb.log"
    return 1
}
