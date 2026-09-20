#!/usr/bin/env bash
# ============================================================================
# install_l2encdec.sh - builds open-l2encdec (the l2encdec CLI) into
# ~/opt.
#
# Used to (re)encrypt the C1 client l2.ini (data/client/l2.ini).
# Source: https://github.com/ritsuwastaken/open-l2encdec (MIT), the
# commit is pinned below. The dependencies (mbedtls, miniz, blowfish)
# come through the CMake FetchContent. Needs gcc/g++, make and
# CMake >= 3.14: without root the CMake installs from deb.debian.org
# (apt-get download + dpkg -x into ~/opt/cmake-root), see the
# ensure_cmake comments.
#
# Idempotent: a ready binary in $PREFIX/bin/l2encdec skips the build.
# ============================================================================
set -euo pipefail

OPEN_L2ENCDEC_COMMIT="870fc5b" # v1.3.11 (2026-09), master head of the clone
SRC_DIR="${SRC_DIR:-$HOME/opt/open-l2encdec-src}"
PREFIX="${PREFIX:-$HOME/opt/l2encdec}"
BINARY="$PREFIX/bin/l2encdec"
LD_LIBRARY_PATH_CMAKE="${HOME}/opt/cmake-root/usr/lib/x86_64-linux-gnu"

if [ -x "${BINARY}" ]; then
    echo "l2encdec already built: ${BINARY}"
    exit 0
fi

# --- CMake: the system one or unpacked from the deb into ~/opt/cmake-root -
if ! command -v cmake >/dev/null 2>&1; then
    if [ -x "$HOME/opt/cmake-root/usr/bin/cmake" ]; then
        export PATH="$HOME/opt/cmake-root/usr/bin:$PATH"
        export LD_LIBRARY_PATH="${LD_LIBRARY_PATH_CMAKE}:${LD_LIBRARY_PATH:-}"
    else
        echo "CMake not found. Install it or run:"
        echo "  apt-get download cmake cmake-data libjsoncpp26 libuv1 librhash1 libzstd1"
        echo "  mkdir -p ~/opt/cmake-root"
        echo "  for d in *.deb; do dpkg -x \"\$d\" ~/opt/cmake-root/; done"
        exit 1
    fi
fi

# --- Sources --------------------------------------------------------------
if [ ! -d "${SRC_DIR}/.git" ]; then
    git clone --depth 1 https://github.com/ritsuwastaken/open-l2encdec.git \
        "${SRC_DIR}"
fi

# --- Build ----------------------------------------------------------------
BUILD_DIR="${SRC_DIR}/build"
mkdir -p "${BUILD_DIR}"
cmake -S "${SRC_DIR}" -B "${BUILD_DIR}" \
    -DBUILD_SHARED_LIBS=OFF \
    -DL2ENCDEC_BUILD_CLI=ON \
    -DCMAKE_BUILD_TYPE=Release
cmake --build "${BUILD_DIR}" --target l2encdec_cli -j"$(nproc)"

mkdir -p "${PREFIX}/bin"
cp "${BUILD_DIR}/cli/l2encdec" "${BINARY}"
echo "DONE: ${BINARY}"
