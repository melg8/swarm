# Lazy boot of the five web UI dev galleries (status: in progress)

Started 2026-11-12, branch main, issue #0 (no board item).

## Goal

- The regular bot UI must not download the five design galleries
  (`fight.js`, `fighttest.js`, `pathtest.js`, `pathcurve.js`,
  `pathflow.js` - about 270 KB, 39 percent of the shipped JavaScript).
- Every `-test-*` mode loads exactly its own gallery script.
- The galleries stay classic scripts: the twelve `tools/repro_*.js`
  harnesses evaluate them with `vm.runInContext`, so an `export` would
  break the gates.
- Acceptance: `index.html` carries no gallery `<script src>`;
  `main.js` injects the active mode's script and boots its `init`;
  every affected harness passes; `go test ./internal/swarm/webserver/`
  passes; the bot mode payload drops by the five file sizes; the
  boot contract is documented in `docs/webui.md` and pinned by the
  Go static asset tests.

## Context

- The five mode dispatches live in `web/main.js`, the mode handshake
  in `GET /api/config` (`internal/swarm/webserver/server.go`).
- `navmesh_view.js` already boots through a dynamic `import()`; it is
  a real ES module over the vendored three.js build, unlike the
  galleries.
- Top level `const` in a classic script lands in the shared global
  lexical environment, so an injected script is visible to `main.js`
  through the bare binding name (never `window`).
- Go gates pinning the old eager boot chain:
  `pathgallery_test.go` (`galleryCase.scriptTag`),
  `fighttest_test.go` (literal `<script src="fighttest.js">`) and
  `fight_test.go` (index page markup).
- The galleries reference no other web module (checked for `App`,
  `MapView`, `StatsTab`, `PathfindUI` and the shared render helpers):
  each one only needs its own markup, the map tile and `document`.

## Progress

### 2026-11-12 13:40 UTC - lazy boot of the five galleries

Moved the five gallery downloads out of `index.html` into a
`bootGallery` helper of `main.js`: a dynamic classic `<script>`
injection followed by the mode's `init`. Updated the Go asset tests
(`pathgallery_test.go`, `fighttest_test.go`, `fight_test.go`) to pin
the lazy chain instead of the eager tags, and `docs/webui.md` to
document it.

Measured regular-page JavaScript: 697,706 bytes before (427,861 core
plus 269,845 of galleries), 427,861 bytes after; gzip transfer of the
galleries was 58,362 bytes. Verification: all twelve `tools/repro_*.js`
harnesses pass, `go test ./internal/swarm/webserver/` passes,
`go build ./...`, `go vet`, `task lint:new` and `task fmt:check`
are clean.

## Status

Complete: the galleries load on demand and the regular page carries
none of them. Next: the follow-up question of splitting `app.js`
(4,529 lines) and `map.js` (5,120 lines) into ES modules, keeping the
classic-script harness contract.
