#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
# SPDX-License-Identifier: MIT
#
# One-shot, idempotent bootstrap of the local L2J Mobius C1 stack from a
# clean machine: Temurin JDK 25 + MariaDB 11.8 + Mobius sources + compiled
# classes + database schema. Finished steps are detected and skipped, so it
# is safe to re-run at any time. See AGENTS.md for the full usage story.
#
# Requirements: bash, curl, git, tar, ~4 GB free disk space.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=mobius_env.sh
source "${SCRIPT_DIR}/mobius_env.sh"

log() { echo "[bootstrap] ${*}"; }
die() { echo "Error ${*}"; exit 1; }

for cmd in curl git tar; do
    command -v "${cmd}" >/dev/null 2>&1 || die "${cmd} is required"
done

# Extract $1 (a .tar.gz) into $2 and move its single top level directory
# to $3, replacing any previous content.
install_tarball() {
    local tarball="$1" dest_root="$2" dest="$3" tmp top
    tmp="$(mktemp -d)"
    log "Downloading $(basename "${tarball}")..."
    curl -fSL --retry 3 -o "${tmp}/pkg.tar.gz" "${tarball}"
    top="$(tar -tzf "${tmp}/pkg.tar.gz" | head -1 | cut -d/ -f1)"
    [ -n "${top}" ] || die "could not inspect tarball ${tarball}"
    mkdir -p "${dest_root}"
    tar -xzf "${tmp}/pkg.tar.gz" -C "${dest_root}"
    rm -rf "${dest}"
    mv "${dest_root}/${top}" "${dest}"
    rm -rf "${tmp}"
}

# --- 1. Temurin JDK 25 (javac is required to build Mobius) -------------------
if [ -x "${JDK_DIR}/bin/javac" ]; then
    log "JDK already installed at ${JDK_DIR}"
else
    install_tarball "${JDK_TARBALL_URL}" "${OPT_DIR}" "${JDK_DIR}"
    log "JDK installed: $("${JDK_DIR}/bin/java" -version 2>&1 | head -1)"
fi

# --- 2. MariaDB 11.8 binary tarball ------------------------------------------
if [ -x "${MARIADB_DIR}/bin/mariadbd" ]; then
    log "MariaDB already installed at ${MARIADB_DIR}"
else
    install_tarball "${MARIADB_TARBALL_URL}" "${OPT_DIR}" "${MARIADB_DIR}"
    log "MariaDB installed: $("${MARIADB_DIR}/bin/mariadb" --version)"
fi

# --- 3. MariaDB data directory ----------------------------------------------
if [ -d "${MYSQL_DATA_DIR}/mysql" ]; then
    log "MariaDB data directory already initialized"
else
    log "Initializing MariaDB data directory ${MYSQL_DATA_DIR}"
    mkdir -p "${MYSQL_DATA_DIR}" "${MYSQL_TMP_DIR}"
    "${MARIADB_DIR}/scripts/mariadb-install-db" --no-defaults \
        --basedir="${MARIADB_DIR}" \
        --datadir="${MYSQL_DATA_DIR}" \
        --auth-root-authentication-method=normal \
        --user="${USER}" >/dev/null
fi

# --- 4. Mobius sources -------------------------------------------------------
if [ -d "${MOBIUS_C1}/dist" ]; then
    log "Mobius sources already cloned at ${MOBIUS_C1}"
else
    rm -rf "${MOBIUS_ROOT}"
    log "Cloning ${MOBIUS_GIT_URL} (large repo, this takes a while)"
    mkdir -p "$(dirname "${MOBIUS_ROOT}")"
    if ! git clone --filter=blob:none --single-branch \
        "${MOBIUS_GIT_URL}" "${MOBIUS_ROOT}"; then
        log "Partial clone failed, retrying with a full clone"
        rm -rf "${MOBIUS_ROOT}"
        git clone "${MOBIUS_GIT_URL}" "${MOBIUS_ROOT}"
    fi
    log "Mobius cloned at revision $(git -C "${MOBIUS_ROOT}" rev-parse --short HEAD)"
fi

# --- 5. Compile Mobius java sources ------------------------------------------
if [ -f "${MOBIUS_COMPILE_MARKER}" ]; then
    log "Mobius classes already compiled in ${MOBIUS_BUILD_DIR}"
else
    log "Compiling Mobius java sources (a few minutes)"
    cd "${MOBIUS_C1}"
    find java -name "*.java" ! -path "*/tools/*" >sources.txt
    classpath=""
    for jar in "${MOBIUS_LIBS_DIR}"/*.jar; do
        case "${jar}" in
        *-sources.jar) ;;
        *) classpath="${classpath}:${jar}" ;;
        esac
    done
    "${JDK_DIR}/bin/javac" -encoding UTF-8 -nowarn \
        -cp "${classpath#:}" -d "${MOBIUS_BUILD_DIR}" @sources.txt
    touch "${MOBIUS_COMPILE_MARKER}"
    log "Compiled $(wc -l <sources.txt) java files"
fi

# --- 6. Local configuration tweaks (idempotent) ------------------------------
# Pathfinding is heavy for a test container; disable it.
sed -i 's/^PathFinding = 2$/PathFinding = 0/' \
    "${MOBIUS_C1}/dist/game/config/GeoEngine.ini"
# Explicit 127.0.0.1 network config (same content as default-ipconfig.xml).
if [ ! -f "${MOBIUS_C1}/dist/game/config/ipconfig.xml" ]; then
    cp "${MOBIUS_C1}/dist/game/config/default-ipconfig.xml" \
        "${MOBIUS_C1}/dist/game/config/ipconfig.xml"
fi
log "Configuration applied (PathFinding=0, ipconfig 127.0.0.1)"

# --- 7. Database schema -------------------------------------------------------
start_mariadb
if [ -f "${SQL_LOADED_MARKER}" ]; then
    log "Database schema already loaded"
else
    log "Loading Mobius SQL schema into ${MYSQL_DB_NAME}"
    mysql_root -e "DROP DATABASE IF EXISTS ${MYSQL_DB_NAME}; \
        CREATE DATABASE ${MYSQL_DB_NAME} DEFAULT CHARACTER SET utf8;"
    for f in "${MOBIUS_C1}"/dist/db_installer/sql/login/*.sql; do
        log "  login/$(basename "${f}")"
        mysql_root "${MYSQL_DB_NAME}" <"${f}"
    done
    for f in "${MOBIUS_C1}"/dist/db_installer/sql/game/*.sql; do
        log "  game/$(basename "${f}")"
        mysql_root "${MYSQL_DB_NAME}" <"${f}"
    done
    touch "${SQL_LOADED_MARKER}"
fi
tables="$(mysql_root -N -e \
    "SELECT COUNT(*) FROM information_schema.tables \
     WHERE table_schema='${MYSQL_DB_NAME}';")"
log "Database ${MYSQL_DB_NAME} contains ${tables} tables"
log "Bootstrap complete, start the stack with tools/mobius_start.sh"
