#!/usr/bin/env bash
# ============================================================================
# swarm_fast_deploy.sh - the shortest path to deploying swarm + L2J
# Mobius C1 on the sandbox series: Debian 13 (trixie), NO root/sudo,
# with git+curl+gcc+OpenJDK 21 JRE, NO javac/go/mariadb-server.
#
# Total time from a clean state to a working stack: ~90 seconds.
# The script is idempotent - every step already done is skipped.
#
# The key optimizations against the "naive" path:
#   1. All the software installs from deb.debian.org (10-90 MB/s)
#      through apt-get download + dpkg -x into ~/opt - no root needed.
#      The Temurin/JDK tarball and the MariaDB bintar
#      (archive.mariadb.org at ~14 KB/s!) are NOT used.
#   2. Mobius clones sparse (--filter=blob:none --sparse --depth 1 +
#      a sparse-checkout of the C1 module only): 167 MB instead of
#      gigabytes, ~9 s.
#   3. The JDK comes as the deb version 25 (the Mobius build.xml
#      requires source=25), the javac 25 compilation - 1318 files in
#      ~7-8 s.
#   4. Go 1.24 from the deb (49.9 MB) is faster than the official
#      tarball (78 MB).
#   5. The stock swarm repository scripts (tools/mobius_start.sh,
#      mobius_e2e.sh) run as is through the JDK_DIR/MARIADB_DIR env
#      overrides.
# ============================================================================
set -euo pipefail

BASE="${BASE:-/home/z/my-project}"
SWARM="${SWARM:-${BASE}/swarm}"
# The branch the swarm clone builds the bot from. The historical
# development branch mobius-c1-client-1 is merged into main and
# deleted from the remote - the default is main.
SWARM_BRANCH="${SWARM_BRANCH:-main}"
MOBIUS_ROOT="${MOBIUS_ROOT:-${BASE}/l2j_mobius}"
MOBIUS_C1="${MOBIUS_ROOT}/L2J_Mobius_C1_HarbingersOfWar"
OPT="${OPT:-${HOME}/opt}"
JDK_ROOT="${OPT}/jdk25-root"
MARIA_ROOT="${OPT}/mariadb-root"
GO_ROOT="${OPT}/go-root"
JDK_DIR="${JDK_ROOT}/usr/lib/jvm/java-25-openjdk-amd64"
MARIADB_DIR="${OPT}/mariadb"
GOROOT_DIR="${GO_ROOT}/usr/lib/go-1.24"
MYSQL_DATA_DIR="${HOME}/mysql_data"
MYSQL_TMP_DIR="${HOME}/mysql_tmp"
MYSQL_SOCK="${MYSQL_TMP_DIR}/mysql.sock"
MYSQL_DB_NAME="l2jmobiusc1"
LOGS_DIR="${BASE}/logs"
BUILD_DIR="${MOBIUS_C1}/build_bin"
LIBS_DIR="${MOBIUS_C1}/dist/libs"
MARIA_LIBDIR="${MARIA_ROOT}/usr/lib/x86_64-linux-gnu"

step() { echo; echo ">>> [$(date +%H:%M:%S)] $*"; }
t0() { STEP_T0=${SECONDS}; }
t1() { echo ">>> the step took $((SECONDS - STEP_T0)) s"; }

mkdir -p "${LOGS_DIR}"

# ---------------------------------------------------------------------------
step "1/8. Cloning swarm (branch ${SWARM_BRANCH})"
if [ -d "${SWARM}/.git" ]; then
    echo "swarm already cloned"
else
    t0
    git clone --depth 1 --branch "${SWARM_BRANCH}" \
        https://github.com/melg8/swarm "${SWARM}"
    t1
fi

# ---------------------------------------------------------------------------
step "2/8. Sparse-clone of L2J_Mobius (the C1 module only, ~167 MB)"
# POLICY: the Mobius server comes ONLY from the official GitLab
# repository https://gitlab.com/MobiusDevelopment/L2J_Mobius. Stale
# copies/mirrors (GitHub and the like) are FORBIDDEN - a datapack/SQL
# mismatch costs hours to fix afterwards.
# GitLab (Cloudflare) answers 403 to a series of fast git requests,
# so three official channels by descending reliability:
#   a) git clone (sparse, blob:none) with retries;
#   b) the API archive of the same commit of the same repository
#      (one GET, stable);
#   c) the blob backfill into an existing sparse clone (git restore).
if [ -d "${MOBIUS_C1}/dist" ] && [ -d "${MOBIUS_ROOT}/.git" ]; then
    echo "Mobius already cloned"
else
    t0
    gitlab_archive_fetch() {
        # Downloads the official API archive of the C1 subdirectory and
        # unpacks it. The same repository, the same master commit - the
        # source stays official, only the transport changes.
        local api="https://gitlab.com/api/v4/projects/MobiusDevelopment%2FL2J_Mobius/repository/archive.tar.gz"
        local path="L2J_Mobius_C1_HarbingersOfWar"
        local tarball="${BASE}/l2j_mobius_c1.tar.gz"
        local i
        for i in 1 2 3 4 5; do
            if curl -sfL --connect-timeout 20 --max-time 570 \
                -o "${tarball}" "${api}?path=${path}"; then
                mkdir -p "${MOBIUS_ROOT}"
                # strip-components=1: the archive path <proj>-<branch>-<sha>/<module>/...
                tar -xzf "${tarball}" -C "${MOBIUS_ROOT}" --strip-components=1
                [ -d "${MOBIUS_C1}/dist" ] && return 0
            fi
            echo ">>> the API archive download failed (try ${i}/5), retry in 10 s"
            sleep 10
        done
        return 1
    }
    if [ ! -d "${MOBIUS_ROOT}/.git" ]; then
        # the git clone with retries (an occasional 403 on the promiscuous
# blob backfill)
        ok=0
        for i in 1 2 3 4 5; do
            if git clone --filter=blob:none --sparse --depth 1 \
                https://gitlab.com/MobiusDevelopment/L2J_Mobius.git "${MOBIUS_ROOT}"; then
                ok=1
                break
            fi
            echo ">>> the clone failed (try ${i}/5), retry in 10 s"
            rm -rf "${MOBIUS_ROOT}"
            sleep 10
        done
        if [ "${ok}" = 1 ]; then
            # The blob backfill for the sparse cone; a 403 falls back to the
# API archive.
            if ! git -C "${MOBIUS_ROOT}" sparse-checkout set \
                L2J_Mobius_C1_HarbingersOfWar 2>/dev/null; then
                echo ">>> the git blob backfill did not pass, taking the API archive"
                rm -rf "${MOBIUS_ROOT}"
                gitlab_archive_fetch || { echo "GitLab unreachable"; exit 1; }
            fi
        else
            # The git protocol is rate limited entirely - the API archive.
            gitlab_archive_fetch || { echo "GitLab unreachable"; exit 1; }
        fi
    else
        # .git exists but the working files are missing (an interrupted
# blob backfill).
        if ! git -C "${MOBIUS_ROOT}" sparse-checkout set \
            L2J_Mobius_C1_HarbingersOfWar 2>/dev/null; then
            echo ">>> the git blob backfill did not pass, taking the API archive"
            rm -rf "${MOBIUS_ROOT}"
            gitlab_archive_fetch || { echo "GitLab unreachable"; exit 1; }
        fi
    fi
    t1
fi

# ---------------------------------------------------------------------------
step "3/8. The server dependencies from deb.debian.org (~157 MB, no root)"
mkdir -p "${OPT}/debs-mobius" "${JDK_ROOT}" "${MARIA_ROOT}"
cd "${OPT}/debs-mobius"
if [ -x "${JDK_DIR}/bin/javac" ] && [ -x "${MARIA_ROOT}/usr/sbin/mariadbd" ]; then
    echo "JDK 25 and MariaDB already unpacked"
else
    t0
    apt-get download openjdk-25-jdk-headless openjdk-25-jre-headless \
        mariadb-server mariadb-server-core mariadb-client-core \
        libaio1t64 liburing2 libncurses6
    dpkg -x openjdk-25-jre-headless_*.deb "${JDK_ROOT}"
    dpkg -x openjdk-25-jdk-headless_*.deb "${JDK_ROOT}"
    for d in mariadb-server_*.deb mariadb-server-core_*.deb \
             mariadb-client-core_*.deb libaio1t64_*.deb liburing2_*.deb \
             libncurses6_*.deb; do
        dpkg -x "${d}" "${MARIA_ROOT}"
    done
    t1
fi

# ---------------------------------------------------------------------------
step "4/8. Fixing the JDK symlinks (Debian points at /etc/java-25-openjdk)"
if [ -e "${JDK_DIR}/conf/security/java.security" ]; then
    echo "the JDK symlinks are fine"
else
    t0
    find "${JDK_DIR}" -type l | while IFS= read -r l; do
        t=$(readlink "${l}")
        case "${t}" in
        /etc/java-25-openjdk*) ln -sfn "${JDK_ROOT}${t}" "${l}" ;;
        esac
    done
    t1
fi

# ---------------------------------------------------------------------------
step "5/8. The MariaDB compat layout for the swarm repository scripts"
mkdir -p "${MARIADB_DIR}/bin" "${MARIADB_DIR}/scripts"
make_wrapper() {
    printf '#!/usr/bin/env bash\nexport LD_LIBRARY_PATH="%s${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"\nexec %s "$@"\n' \
        "${MARIA_LIBDIR}" "$2" > "$1"
    chmod +x "$1"
}
make_wrapper "${MARIADB_DIR}/bin/mariadb"        "${MARIA_ROOT}/usr/bin/mariadb"
make_wrapper "${MARIADB_DIR}/bin/mariadbd"       "${MARIA_ROOT}/usr/sbin/mariadbd"
make_wrapper "${MARIADB_DIR}/scripts/mariadb-install-db" \
    "${MARIA_ROOT}/usr/bin/mariadb-install-db"
"${JDK_DIR}/bin/java" -version 2>&1 | head -1
"${JDK_DIR}/bin/javac" -version
"${MARIADB_DIR}/bin/mariadbd" --version

# ---------------------------------------------------------------------------
step "6/8. Compiling the Mobius server (javac 25, ~7-8 s)"
if [ -f "${BUILD_DIR}/.compile_ok" ]; then
    echo "the server is already compiled"
else
    t0
    cd "${MOBIUS_C1}"
    find java -name "*.java" ! -path "*/tools/*" > sources.txt
    classpath=""
    for jar in "${LIBS_DIR}"/*.jar; do
        case "${jar}" in *-sources.jar) ;; *) classpath="${classpath}:${jar}" ;; esac
    done
    "${JDK_DIR}/bin/javac" -encoding UTF-8 -nowarn \
        -cp "${classpath#:}" -d "${BUILD_DIR}" @sources.txt
    touch "${BUILD_DIR}/.compile_ok"
    echo "compiled $(wc -l <sources.txt) files"
    t1
fi
# configs: pathfinding off, ipconfig at 127.0.0.1
sed -i 's/^PathFinding = 2$/PathFinding = 0/' \
    "${MOBIUS_C1}/dist/game/config/GeoEngine.ini"
[ -f "${MOBIUS_C1}/dist/game/config/ipconfig.xml" ] || \
    cp "${MOBIUS_C1}/dist/game/config/default-ipconfig.xml" \
       "${MOBIUS_C1}/dist/game/config/ipconfig.xml"
# the login server listens on 127.0.0.1:2106 only, so the swarm MITM
# proxy can take 127.0.0.2:2106 for the C1 clients with the hardcoded
# port (see data/client/Readme.txt and docs/proxy.md)
sed -i 's/^LoginserverHostname = 0.0.0.0$/LoginserverHostname = 127.0.0.1/' \
    "${MOBIUS_C1}/dist/login/config/Server.ini"

# ---------------------------------------------------------------------------
step "7/8. DB: the datadir, the MariaDB start, the SQL import (75 tables)"
mysql_cmd() { "${MARIADB_DIR}/bin/mariadb" --socket="${MYSQL_SOCK}" -u root "$@"; }
if [ ! -d "${MYSQL_DATA_DIR}/mysql" ]; then
    t0
    mkdir -p "${MYSQL_DATA_DIR}" "${MYSQL_TMP_DIR}" "${LOGS_DIR}"
    "${MARIADB_DIR}/scripts/mariadb-install-db" --no-defaults \
        --basedir="${MARIA_ROOT}/usr" --datadir="${MYSQL_DATA_DIR}" \
        --auth-root-authentication-method=normal --user="${USER}" >/dev/null
    t1
fi
if ! mysql_cmd -e "SELECT 1" >/dev/null 2>&1; then
    t0
    "${MARIADB_DIR}/bin/mariadbd" --no-defaults \
        --basedir="${MARIA_ROOT}/usr" --datadir="${MYSQL_DATA_DIR}" \
        --tmpdir="${MYSQL_TMP_DIR}" --socket="${MYSQL_SOCK}" \
        --port=3306 --bind-address=127.0.0.1 --user="${USER}" \
        >"${LOGS_DIR}/mariadb.log" 2>&1 &
    for _ in $(seq 1 30); do
        mysql_cmd -e "SELECT 1" >/dev/null 2>&1 && break
        sleep 1
    done
    t1
fi
if [ ! -f "${MYSQL_DATA_DIR}/.sql_loaded" ]; then
    t0
    mysql_cmd -e "DROP DATABASE IF EXISTS ${MYSQL_DB_NAME}; \
        CREATE DATABASE ${MYSQL_DB_NAME} DEFAULT CHARACTER SET utf8;"
    for f in "${MOBIUS_C1}"/dist/db_installer/sql/login/*.sql \
             "${MOBIUS_C1}"/dist/db_installer/sql/game/*.sql; do
        mysql_cmd "${MYSQL_DB_NAME}" <"${f}"
    done
    touch "${MYSQL_DATA_DIR}/.sql_loaded"
    t1
fi
echo "tables in the DB: $(mysql_cmd -N -e \
    "SELECT COUNT(*) FROM information_schema.tables \
     WHERE table_schema='${MYSQL_DB_NAME}';")"

# ---------------------------------------------------------------------------
step "8/8. Go 1.24 from the deb (49.9 MB) + the swarm build"
mkdir -p "${OPT}/debs-go" "${GO_ROOT}"
if [ ! -x "${GOROOT_DIR}/bin/go" ]; then
    t0
    cd "${OPT}/debs-go"
    apt-get download golang-1.24-go golang-1.24-src
    dpkg -x golang-1.24-go_*_amd64.deb "${GO_ROOT}"
    dpkg -x golang-1.24-src_*.deb "${GO_ROOT}"
    t1
fi
export GOROOT="${GOROOT_DIR}"
export PATH="${GOROOT_DIR}/bin:${PATH}"
go version
if [ ! -x "${LOGS_DIR}/swarm_bot" ]; then
    t0
    cd "${SWARM}"
    # The build identity (branch/commit/dirty/time): the bot log
    # opening line and the webui state dump print it, so any dump
    # tells exactly which code it came from. In a non-git checkout the
    # values stay empty - the commit carries the Go buildinfo anyway.
    SWARM_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
    if [ "${SWARM_BRANCH}" = "HEAD" ]; then
        SWARM_BRANCH=""
    fi
    SWARM_COMMIT="$(git rev-parse HEAD 2>/dev/null || true)"
    if [ -n "$(git status --porcelain 2>/dev/null || true)" ]; then
        SWARM_DIRTY="true"
    else
        SWARM_DIRTY="false"
    fi
    go build -ldflags "\
-X github.com/melg8/swarm/internal/version.Branch=${SWARM_BRANCH} \
-X github.com/melg8/swarm/internal/version.Commit=${SWARM_COMMIT} \
-X github.com/melg8/swarm/internal/version.Dirty=${SWARM_DIRTY} \
-X github.com/melg8/swarm/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        -o "${LOGS_DIR}/swarm_bot" ./cmd/swarm
    t1
fi

# ---------------------------------------------------------------------------
step "Starting the stack (login 2106 + game 7777) with the stock swarm script"
export JDK_DIR MARIADB_DIR
cd "${SWARM}"
bash tools/mobius_start.sh

step "DONE. Bot: ${LOGS_DIR}/swarm_bot (or tools/mobius_e2e.sh 45)"

# ---------------------------------------------------------------------------
# Post-deploy dev-tool preflight: the Go toolchain is up, but the
# workflow also expects task, golangci-lint, gci and gofumpt. The
# fast deploy does not install them (they belong to the developer
# side, not the server stack); the dedicated installer does. This is
# a check, not an install - it prints a clear next step when a tool
# is missing instead of paying the install cost on every deploy.
if [ -x "${SWARM}/tools/install_dev_tools.sh" ]; then
    echo
    echo ">>> dev tools preflight (run tools/install_dev_tools.sh to fix):"
    PATH="${GOROOT_DIR}/bin:${HOME}/go/bin:${PATH}" \
        bash "${SWARM}/tools/install_dev_tools.sh" check \
        2>&1 | sed 's/^/    /' || true
fi
