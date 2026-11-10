# The web UI build identity and the auto reload of a stale page (status: in review)

Started 2026-09-22, branch feature/webui-auto-reload, issue #48.

## Goal

The issue: a tab opened before a backend deployment keeps running the
old embedded UI over the new endpoints - the user sees no changes and
has to hard reload by hand. Done means: when the page's build differs
from the server's build, the page reloads itself into the full new
version (no manual alt+f5, no reload loops).

## Context

- Issue: https://github.com/melg8/swarm/issues/48 (created 2026-09-22).
- The UI is `go:embed`ed in the binary: the deployment changes the
  assets, the stale tab never refetches them on its own.
- The embedded files carry a zero modtime - no validator on the wire,
  so heuristic browser caching could serve the OLD asset even after a
  reload; the cache headers are part of the fix, not an afterthought.
- The natural detection vehicle is the two second `/api/bots` poll
  (refreshBots) - no new endpoint, no extra request.

## Progress

### 2026-09-22 17:05 UTC - the build plumbing end to end

- `internal/swarm/webserver/web_build.go` (new): `webBuildID` (the
  hex sha256 over the sorted embedded file paths and bytes - the
  embed itself is the version truth, no git or ldflags plumbing),
  `injectBuildMeta` (the `<meta name="swarm-build">` into the index
  head) and `staticHandler` ("/" and "/index.html" serve the
  prebuilt page with no-cache; .js/.css/.html answer no-cache; the
  tiles, icons and meshes keep their default caching).
- `internal/swarm/webserver/server.go`: the `webBuild`/`indexPage`
  fields prepared once in `newServer`, the static mount wrapped,
  `handleBotList` answers `X-Swarm-Build`.
- `internal/swarm/webserver/web/app.js`: `App.buildId` read from the
  meta (`initBuildWatch`), `checkBuild(response)` on every bot poll -
  a mismatch reloads once, the sessionStorage guard
  (`swarm.buildReload`) stops the loop; the guard clears itself when
  the page actually runs the guarded build.
- `internal/swarm/webserver/web/main.js`: `initBuildWatch()` first
  line of the boot.
- Tests: `TestWebBuildIDStableAndContentBound`,
  `TestInjectBuildMeta`, `TestIndexServesBuildMeta`,
  `TestBotListCarriesBuildHeader`, `TestStaticScriptsAnswerNoCache`.
- `go build ./...`, the full uncapped golangci-lint, the webserver
  suite - all green.

## Status

In review: the branch rides current main, PR opened with "Fixes #48",
CI watched green.
