# The show-navmesh dual pack view (status: in progress)

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

## Status

NEXT (the viewer half, this branch):
1. `navmesh_view.js`: read `config.compare`; boot a second pane (a
   split render - two scissor viewports over one renderer, or an A/B
   toggle if the split proves heavy) with its own tile loading from
   `/api/navmesh/compare/geometry/{key}`.
2. The compare pathfind: the route pair click issues ONE
   `/api/navmesh/path` request (the server already answers both);
   draw the primary route and the compare route in distinct colors
   in their panes, surface the diff (the counts, the duration delta)
   in the overlay.
3. The per-tile diff highlighting: fetch
   `/api/navmesh/compare/diff/{key}` per visible tile, tint the tile
   by the vanished/added counts.
4. `docs/navmesh.md`: the dual view section (the flags, the
   endpoints, the reading).
5. Then: the PR ("Fixes #59"), the rebase on main, the CI watch, the
   issue report, the claim cleanup, Ready for review.
