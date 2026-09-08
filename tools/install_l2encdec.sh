#!/usr/bin/env bash
# ============================================================================
# install_l2encdec.sh — сборка open-l2encdec (CLI l2encdec) в ~/opt.
#
# Используется для (пере)шифрования l2.ini клиента C1 (data/client/l2.ini).
# Источник: https://github.com/ritsuwastaken/open-l2encdec (MIT), коммит
# запинен ниже. Зависимости (mbedtls, miniz, blowfish) подтягиваются самим
# CMake через FetchContent. Требует gcc/g++, make и CMake >= 3.14: без root
# CMake можно поставить из deb.debian.org (apt-get download + dpkg -x в
# ~/opt/cmake-root), см. комментарии в функции ensure_cmake.
#
# Идемпотентен: готовый бинарник в $PREFIX/bin/l2encdec пропускает сборку.
# ============================================================================
set -euo pipefail

OPEN_L2ENCDEC_COMMIT="870fc5b" # v1.3.11 (2026-09), master head of the clone
SRC_DIR="${SRC_DIR:-$HOME/opt/open-l2encdec-src}"
PREFIX="${PREFIX:-$HOME/opt/l2encdec}"
BINARY="$PREFIX/bin/l2encdec"
LD_LIBRARY_PATH_CMAKE="${HOME}/opt/cmake-root/usr/lib/x86_64-linux-gnu"

if [ -x "${BINARY}" ]; then
    echo "l2encdec уже собран: ${BINARY}"
    exit 0
fi

# --- CMake: системный или распакованный из deb в ~/opt/cmake-root --------
if ! command -v cmake >/dev/null 2>&1; then
    if [ -x "$HOME/opt/cmake-root/usr/bin/cmake" ]; then
        export PATH="$HOME/opt/cmake-root/usr/bin:$PATH"
        export LD_LIBRARY_PATH="${LD_LIBRARY_PATH_CMAKE}:${LD_LIBRARY_PATH:-}"
    else
        echo "CMake не найден. Установите его или выполните:"
        echo "  apt-get download cmake cmake-data libjsoncpp26 libuv1 librhash1 libzstd1"
        echo "  mkdir -p ~/opt/cmake-root"
        echo "  for d in *.deb; do dpkg -x \"\$d\" ~/opt/cmake-root/; done"
        exit 1
    fi
fi

# --- Исходники ------------------------------------------------------------
if [ ! -d "${SRC_DIR}/.git" ]; then
    git clone --depth 1 https://github.com/ritsuwastaken/open-l2encdec.git \
        "${SRC_DIR}"
fi

# --- Сборка ---------------------------------------------------------------
BUILD_DIR="${SRC_DIR}/build"
mkdir -p "${BUILD_DIR}"
cmake -S "${SRC_DIR}" -B "${BUILD_DIR}" \
    -DBUILD_SHARED_LIBS=OFF \
    -DL2ENCDEC_BUILD_CLI=ON \
    -DCMAKE_BUILD_TYPE=Release
cmake --build "${BUILD_DIR}" --target l2encdec_cli -j"$(nproc)"

mkdir -p "${PREFIX}/bin"
cp "${BUILD_DIR}/cli/l2encdec" "${BINARY}"
echo "ГОТОВО: ${BINARY}"
