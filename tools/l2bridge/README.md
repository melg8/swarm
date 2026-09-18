# l2bridge: the external pathfinding benchmark harness

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The bridge loads the flat binary dump `cmd/navmesh-export` writes
(the closed polygon subgraph of the requested regions) into the
native substrates of the external navmesh engines and times the same
query the production Route answers:

- `raasta` - the navmesh A* of https://github.com/MacCracken/raasta
  (the water polys ride the NavPoly cost field at 3x, the swim
  filter semantics of the production Route);
- `channel` / `tastar` / `trastar` - the navmesh family of
  https://github.com/bnomei/condor (ChannelSearch, TA* and the
  prepared TRA* whose preprocess is timed as the build).

Both engines are pinned by revision in Cargo.toml, so a fresh
`cargo build` fetches exactly the measured code.

## Run

    # the whole round: pack build, production probe, the four engines
    tools/l2bridge/run_bench.sh

    # an explicit region set and endpoints
    tools/l2bridge/run_bench.sh data/geodata 21_19 \
        36000,36000,-3600 50000,50000,-3600

    # one engine against an existing dump
    cargo build --release --manifest-path tools/l2bridge/Cargo.toml
    tools/l2bridge/target/release/l2bridge raasta /tmp/l2mesh.bin 3

Every printed line carries the BUILD or QUERY marker; the
`docs/fastpath_research.md` section 7 tables quote these lines.
The condor family does not finish a corridor scale query (the wall
clock cap reports it), the raasta and the production rows land in
milliseconds - the measured verdict lives in the doc.
