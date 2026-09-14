#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# Builds the Recast/Detour research experiments with plain g++.
# No cmake, no new toolchain: the sandbox g++ is enough. The upstream
# checkout is cloned once at the pinned commit and reused.

set -euo pipefail

RECAST_COMMIT=9f4ce64
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UPSTREAM="$LAB_DIR/upstream"
BUILD="$LAB_DIR/build"

if [ ! -d "$UPSTREAM/.git" ]; then
    git clone -q https://github.com/recastnavigation/recastnavigation.git "$UPSTREAM"
fi

# git checkout of the pinned commit is idempotent and works offline
# once the commit is present.
if ! git -C "$UPSTREAM" rev-parse --verify --quiet "$RECAST_COMMIT^{commit}" >/dev/null; then
    git -C "$UPSTREAM" fetch -q origin
fi
git -C "$UPSTREAM" checkout -q "$RECAST_COMMIT"

mkdir -p "$BUILD/obj/recast" "$BUILD/obj/detour"

CXXFLAGS="-O2 -std=c++17 -Wall -Wextra -Wno-unused-parameter -DNDEBUG"

echo "== compiling Recast =="
for f in "$UPSTREAM"/Recast/Source/*.cpp; do
    g++ $CXXFLAGS -I "$UPSTREAM/Recast/Include" -I "$UPSTREAM/Detour/Include" \
        -c "$f" -o "$BUILD/obj/recast/$(basename "${f%.cpp}").o" &
done
wait

echo "== compiling Detour =="
for f in "$UPSTREAM"/Detour/Source/*.cpp; do
    g++ $CXXFLAGS -I "$UPSTREAM/Recast/Include" -I "$UPSTREAM/Detour/Include" \
        -c "$f" -o "$BUILD/obj/detour/$(basename "${f%.cpp}").o" &
done
wait

rm -f "$BUILD/librecast.a" "$BUILD/libdetour.a"
ar rcs "$BUILD/librecast.a" "$BUILD"/obj/recast/*.o
ar rcs "$BUILD/libdetour.a" "$BUILD"/obj/detour/*.o

echo "== compiling experiments =="
for f in "$LAB_DIR"/experiments/*.cpp; do
    name="$(basename "${f%.cpp}")"
    g++ $CXXFLAGS -I "$UPSTREAM/Recast/Include" -I "$UPSTREAM/Detour/Include" \
        "$f" "$BUILD/librecast.a" "$BUILD/libdetour.a" -o "$BUILD/$name"
    echo "built $name"
done

echo "BUILD_OK: $BUILD"
