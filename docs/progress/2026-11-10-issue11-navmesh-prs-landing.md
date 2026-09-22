# The old navmesh PRs: land the size round, the corpus gate and the dual view (status: in progress)

Started 2026-11-10, branch main, issues #11, #27 and #59.

## Goal

The six open PRs of the September navmesh rounds are 125-230 commits
behind main and nothing of them is on main. Land the three the owner
approved, close the two the evidence refuses, and leave the one that
waits for a contract decision alone:

- #26 (`feature/navmesh-size-research`): the zstd best level and the
  compressed abstract sidecars, 1287.4 -> 843.0 MB (-34.5 percent),
  routes bit identical;
- #55 (`feature/navmesh-route-corpus`): the recorded-corpus harness of
  `cmd/navpack-verify` (generate / replay / compare), stacked on #26;
- #67 (`feature/navmesh-dual-view`): the side by side pack viewer and
  the compare pathfind endpoints;
- #16 (`feature/config-launch`): already on main as `a2582bc2` and
  `27bed31b`; close as landed (no unique patch);
- #61 (`feature/height-quantization`): the zero-drift corpus gate
  refuses every candidate step (48 percent of the corner heights move
  at step 16, waypoint counts drift); close as evaluated and
  rejected, the report stays in issue #56;
- #62 (`feature/navmesh-merge-tolerance`): the same gate refuses every
  tolerance above zero; stays open until the owner decides on the
  corpus answer contract (a Z tolerant hash).

Acceptance criteria:

- the three landings are linear commits on main (no merge commits, no
  force push) and each keeps the original commit message;
- every landing passes build, vet, `task lint`, the affected package
  tests and `fmt:check`; the #67 landing also passes the web build
  test and the navmesh viewer harness;
- the historical progress fragment of #67 moves straight to
  `docs/progress/archive/` (it must not enter the active set);
- the PRs themselves stay for the owner to mark (no gh or API token
  on the machine).

## Context

Verified on 2026-11-10:

- `git merge-tree` against current main: #26, #55, #62 and #67 merge
  cleanly; #61 and #16 conflict (the latter is stale by content);
- the CI of #26, #55, #62 and #67 is green on its recorded head;
- #26 is 1 commit on 8b9a1db0, #55 is #26 + 1 commit, #67 is 2 commits
  on e6519b1a (an ancestor of main, with no main side changes to
  `navmesh_view.js`, `style.css` or `navmesh.go` since);
- main already carries the zstd tile round (`c841b549`, SpeedDefault)
  and the legacy tile upgrade; #26 layers SpeedBestCompression and the
  sidecar frame on top and keeps the legacy loaders readable;
- issue #56 records the height quantization verdict and the corpus
  harness memory bomb; issue #57 records the merge tolerance table
  (0 of 329 drift at tolerance 0, 242-245 at 8 and above).

Constraints: no history rewrite, no force push; the pack itself
(`data/`) is not rebuilt in this round, only the builder and the
loaders change.

## Progress

### 2026-11-10 - the evaluation of the six PRs

The PR/issue/CI/merge-tree state above was collected; the owner chose
to land #26, #55 and #67, close #16 and #61, and leave #62 open. The
landing starts now.

### 2026-11-10 - the size round lands (#26)

Cherry-picked `0287e2e8` onto current main without conflicts: the
builder writes the abstract sidecars as one zstd frame and the tile
encoder moves to `SpeedBestCompression`; the loader sniffs the frame
magic and keeps the plain legacy sidecars readable. Verified:
`go build ./...`, `go test ./internal/swarm/pathfind/...` (the three
packages and the prototype, all green), `task lint:new` (0 issues) and
`task fmt:check`. The pack binaries in `data/` are not rebuilt.

### 2026-11-10 - the corpus gate lands (#55)

Cherry-picked `d4b969b7` after the size round without conflicts:
`cmd/navpack-verify` grows from the five pairwise queries into the
recorded-corpus harness (generate / replay / compare), its tests land
with it and `docs/navmesh.md` gains the section. Verified: build, the
harness unit tests, `task lint:new` and `task fmt:check`. The memory
bomb of the full-pack generate (issue #56, finding 5) stays a known
open issue of the harness, not a regression of this landing.

## Status

#26 and #55 are landed and verified; the dual pack view (#67) is the
next step.
