<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Recast/Detour research lab

The experiments of the `feature/new-pathfind` research round: what
recastnavigation (Recast navmesh build + Detour query) gives the
swarm pathfinder, what it costs to port the L2 geodata into it and
what a Go runtime port would look like. The research report itself
lives in `docs/recast_pathfinding.md`; this directory holds the
runnable evidence behind it.

This is research code, not bot code: C++ experiments live here
outside `internal/` on purpose (the bot itself stays pure Go, the
AGENTS.md "no C/C++ toolchain" rule applies to the bot and its
deploy; building these experiments needs only the g++ the sandbox
already ships). The final recommendation of the research keeps that
separation: the navmesh build is an offline tool, the runtime query
engine is the Go port.

## Layout

- `experiments/bridge_water.cpp` - the synthetic multi layer world
  (a floating village deck + a bridge over water) proving the same
  x/y different z navigation and the water area semantics.
- `experiments/geodata_navmesh.cpp` - the real geodata converter:
  l2j region file -> Recast heightfield -> Detour navmesh, queries
  and benchmarks on the real elven village region 21_19.
- `results/` - the captured outputs of the experiment runs (the
  evidence the report quotes).
- `upstream/` - the cloned recastnavigation checkout (gitignored).
- `build/` - the compiled libraries and binaries (gitignored).

## Building and running

`build.sh` clones the pinned upstream into `upstream/` (shallow,
submodules not needed - the experiments link only Recast + Detour),
compiles the two libraries with plain g++ (no cmake) and builds
every experiment in `experiments/`. Run it from this directory:

```bash
bash build.sh
./build/bridge_water | tee results/bridge_water.txt
./build/geodata_navmesh ../../data/geodata/21_19.l2j | tee results/geodata_navmesh.txt
```

The pinned upstream commit is recorded in `build.sh`
(`RECAST_COMMIT`); the pin matters because the upstream API moves
(functions get renamed, rcFilterSmallRegions was folded into
rcBuildRegions between releases).
