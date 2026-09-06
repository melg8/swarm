#!/usr/bin/env bash
# ============================================================================
# swarm_fast_deploy.sh — кратчайший путь развёртывания swarm + L2J Mobius C1
# в среде серии: Debian 13 (trixie), БЕЗ root/sudo, есть git+curl+gcc+
# OpenJDK 21 JRE, НЕТ javac/go/mariadb-server.
#
# Итоговое время от чистого состояния до работающего стека: ~90 секунд.
# Скрипт идемпотентен — каждый шаг пропускается, если уже выполнен.
#
# Ключевые оптимизации против "наивного" пути:
#   1. Всё ПО ставится из deb.debian.org (10-90 МБ/с) через apt-get download
#      + dpkg -x в ~/opt — root не нужен. Temurin/JDK-tarball и MariaDB-bintar
#      (archive.mariadb.org ≈ 14 КБ/с!) НЕ используются.
#   2. Mobius клонируется sparse (--filter=blob:none --sparse --depth 1 +
#      sparse-checkout только модуля C1): 167 МБ вместо гигабайтов, ~9 с.
#   3. JDK берётся deb-версией 25 (build.xml Mobius требует source=25),
#      компиляция javac 25 — 1318 файлов за ~7-8 с.
#   4. Go 1.24 из deb (49.9 МБ) быстрее официального tarball (78 МБ).
#   5. Штатные скрипты репозитория swarm (tools/mobius_start.sh, mobius_e2e.sh)
#      используются как есть через env-переопределения JDK_DIR/MARIADB_DIR.
# ============================================================================
set -euo pipefail

BASE="${BASE:-/home/z/my-project}"
SWARM="${SWARM:-${BASE}/swarm}"
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
t1() { echo ">>> шаг занял $((SECONDS - STEP_T0)) с"; }

mkdir -p "${LOGS_DIR}"

# ---------------------------------------------------------------------------
step "1/8. Клонирование swarm (ветка mobius-c1-client-1)"
if [ -d "${SWARM}/.git" ]; then
    echo "swarm уже склонирован"
else
    t0
    git clone --depth 1 --branch mobius-c1-client-1 \
        https://github.com/melg8/swarm "${SWARM}"
    t1
fi

# ---------------------------------------------------------------------------
step "2/8. Sparse-clone L2J_Mobius (только модуль C1, ~167 МБ)"
if [ -d "${MOBIUS_C1}/dist" ]; then
    echo "Mobius уже склонирован"
else
    t0
    if [ ! -d "${MOBIUS_ROOT}/.git" ]; then
        # GitLab (Cloudflare) иногда отдаёт 403 — ретраим до 5 раз
        for i in 1 2 3 4 5; do
            if git clone --filter=blob:none --sparse --depth 1 \
                https://gitlab.com/MobiusDevelopment/L2J_Mobius.git "${MOBIUS_ROOT}"; then
                break
            fi
            echo ">>> клон не удался (попытка ${i}/5), повтор через 10 с"
            rm -rf "${MOBIUS_ROOT}"
            [ "${i}" = 5 ] && { echo "GitLab недоступен"; exit 1; }
            sleep 10
        done
    fi
    git -C "${MOBIUS_ROOT}" sparse-checkout set L2J_Mobius_C1_HarbingersOfWar
    t1
fi

# ---------------------------------------------------------------------------
step "3/8. Зависимости сервера из deb.debian.org (~157 МБ, root не нужен)"
mkdir -p "${OPT}/debs-mobius" "${JDK_ROOT}" "${MARIA_ROOT}"
cd "${OPT}/debs-mobius"
if [ -x "${JDK_DIR}/bin/javac" ] && [ -x "${MARIA_ROOT}/usr/sbin/mariadbd" ]; then
    echo "JDK 25 и MariaDB уже распакованы"
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
step "4/8. Починка симлинков JDK (Debian указывает на /etc/java-25-openjdk)"
if [ -e "${JDK_DIR}/conf/security/java.security" ]; then
    echo "симлинки JDK в порядке"
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
step "5/8. Compat-структура MariaDB для скриптов репозитория swarm"
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
step "6/8. Компиляция сервера Mobius (javac 25, ~7-8 с)"
if [ -f "${BUILD_DIR}/.compile_ok" ]; then
    echo "сервер уже скомпилирован"
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
    echo "скомпилировано $(wc -l <sources.txt) файлов"
    t1
fi
# конфиги: pathfinding выключен, ipconfig на 127.0.0.1
sed -i 's/^PathFinding = 2$/PathFinding = 0/' \
    "${MOBIUS_C1}/dist/game/config/GeoEngine.ini"
[ -f "${MOBIUS_C1}/dist/game/config/ipconfig.xml" ] || \
    cp "${MOBIUS_C1}/dist/game/config/default-ipconfig.xml" \
       "${MOBIUS_C1}/dist/game/config/ipconfig.xml"

# ---------------------------------------------------------------------------
step "7/8. БД: datadir, запуск MariaDB, импорт SQL (75 таблиц)"
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
echo "таблиц в БД: $(mysql_cmd -N -e \
    "SELECT COUNT(*) FROM information_schema.tables \
     WHERE table_schema='${MYSQL_DB_NAME}';")"

# ---------------------------------------------------------------------------
step "8/8. Go 1.24 из deb (49.9 МБ, быстрее tarball) + сборка swarm"
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
    go build -o "${LOGS_DIR}/swarm_bot" ./cmd/swarm
    t1
fi

# ---------------------------------------------------------------------------
step "Запуск стека (login 2106 + game 7777) штатным скриптом swarm"
export JDK_DIR MARIADB_DIR
cd "${SWARM}"
bash tools/mobius_start.sh

step "ГОТОВО. Бот: ${LOGS_DIR}/swarm_bot (или tools/mobius_e2e.sh 45)"
