# The show-navmesh dual pack view (status: ready for review)

Started 2026-09-22, branch feature/navmesh-dual-view, issue #59.

## Goal

The owner ask from the #27 thread: show-navmesh demonstrates both
packs side by side (the old and the reduced), with the per-tile diff
highlighting (the polygons that moved or vanished) and the compare
pathfind (the same click routes through both packs, both answers
drawn). The programmatic gate is navpack-verify (PR #55); this slice
is its visual counterpart.

## Context

- Issue: https://github.com/melg8/swarm/issues/59 (sub-issue of #27).
- The viewer: `internal/swarm/webserver/web/navmesh_view.js` (the
  three.js single scene app, 2163 lines) over the
  `internal/swarm/webserver/navmesh.go` server mode (the NMV2 tile
  geometry of navmesh_geometry.go, the route endpoint, the original
  geodata toggle).
- The server half of the slice LANDED on this branch (see Progress).
  The VIEWER half is the remaining work: dual pane rendering, the
  diff highlight colors, the both-routes overlay.

## Progress

### 2026-09-22 17:55 UTC - the server half

- `NavmeshOptions.CompareMesh` arms the dual view;
  `Server.navmeshCompare` + `navmeshCompareGeo` (its own geometry
  payload cache, pre-initialized in newServer, guarded by the shared
  mutex).
- `navmeshConfigResponse.Compare` (nil when single pack): the
  compare tile list rides the boot config.
- `GET /api/navmesh/compare/geometry/{key}`: the NMV2 payload of the
  compare tile (the shared `navmeshGeometryCached` core; the primary
  endpoint refactored onto the same helper - the dupl finding).
- `GET /api/navmesh/compare/diff/{key}`: the per tile diff - the
  poly counts of both packs and the vanished/added rect lists (the
  identity is the rect bounds + the four corner heights; a tile
  absent on one side diffs against an empty set; both absent = 404;
  the compare endpoints refuse with 501 when no compare mesh is
  armed).
- `POST /api/navmesh/path` answers BOTH meshes when armed: the
  search core extracted into `navmeshSearchAnswer(mesh, request,
  label)` with the shared filter contract (`navmeshRouteFilter` -
  the water pricing, the capsule clearance, the guard) and the
  console line (`logNavmeshRoute`, the label distinguishes the
  packs). The response carries `compare` (the same shape).
- Tests: 7 new (the config compare section, the single pack refusal,
  the compare geometry differs, the diff lists the vanished water
  rect, the added tile case, the unknown tile 404, the compare path
  answer). `go build`, vet, the full uncapped golangci-lint (0
  issues), the webserver suite - all green.

### 2026-09-22 20:55 UTC - the viewer half (agent AGNTZ7QK, round 2)

- The scissor split render: pane A and pane B as two viewports over
  ONE renderer, ONE flight camera drawing both scenes (the camera,
  the route pair, the tile selection stay shared state above the
  panes); pane chrome (labels A / B + dir, the divider), the camera
  aspect matches one pane.
- `armDualView` reads `config.compare`; the compare tile registry
  mirrors the primary entries; `entriesFor` / `sceneFor` /
  `geometryUrlFor` are the pane seams. A tile the compare pack
  dropped renders as the honest empty pane B (`B absent` in the
  status cell); compare-only rows join the listing.
- The compare pathfind: ONE POST, the answer's `compare` draws cyan
  (`COMPARE_PATH_COLOR`) in pane B while the primary stays amber in
  pane A; markers (start ring / end disc) draw in BOTH scenes; the
  result panel carries the B rows (waypoints, corridor polys, path
  length, length delta) and the `pack A . B <dur> (<delta> ms)` sub
  line.
- The per tile diff highlight: `ensureDiff` fetches the diff once
  per session, the tile tints in BOTH panes (vanished red, added
  green, mixed amber, the alpha scaling with the changed share),
  the vanished/added rects draw as their colored outlines while the
  lists stay under 512 (bigger changes tint whole); the status cell
  gains `-vanished/+added`; the `tile diff tint` toggle hides the
  layer without refetching; `diff=0` view link parameter restores
  the hidden state.
- `cmd/swarm/main.go`: the `-navmesh-compare` flag loads the second
  pack (`navmeshComparePack`); an empty compare directory stops the
  mode with an explanation (no silent fallback).
- `docs/navmesh.md`: the dual pack view section (the flag, the
  panes, the endpoints, the diff reading) + the compare bullets in
  the endpoint list.
- Smoke verified live (synthetic 3-tile vs 2-tile pair, headless
  chromium): the boot arms dual, the meshes load in both panes, the
  diff tints fetch and tint, the route pair draws in both panes,
  the single pack boot is byte for byte the old viewer (dual=false,
  no pane chrome, full width canvas, aspect 1.778). Two viewer bugs
  caught and fixed in the round: the status cell printed the rect
  arrays instead of counts, and the diff toggle rebuild skipped the
  cached counts.
- Gates: gofmt-spaces clean, go build, go vet, golangci-lint 0
  issues (funlen pushed `navmeshComparePack` out of
  runNavmeshViewer), the webserver suite, cmd/swarm and pathfind
  suites green. (The go1.26.8 toolchain reformat of three
  pre-existing files rides the commit: the branch's first commit
  predates the gofmt-spaces -l pass that the rebase onto main made
  required.)

## Status

DONE (the whole slice landed, see the round above):
1. The split render (two scissor viewports over one renderer).
2. The compare pathfind (one request, both answers drawn).
3. The per tile diff highlighting (the tints, the rect outlines,
   the counts).
4. `docs/navmesh.md`: the dual pack view section.
5. The PR ("Fixes #59"), rebased on main, the CI watch, the issue
   report, the claim cleanup, Ready for review.

The live verdict on the real pack pair (data/navmesh vs the reduced
output of the structural rounds) is the reviewer's step: boot the
two directories, fly the pair, click a route - the corpus gate
(navpack-verify) already proved the routes identical; the viewer
now shows it.
