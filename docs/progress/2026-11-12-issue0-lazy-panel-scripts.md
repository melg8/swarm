# Lazy boot of the statistics tab and the pathfind UI (status: complete)

Started 2026-11-12, branch main, issue #0. Follow-up of the lazy
gallery round (`archive/2026-11-12-issue0-lazy-gallery-boot.md`).

## Goal

- The map-only bot page must not download the statistics tab or the
  pathfind UI scripts.
- `stats.js` loads on the first visit of the Stats tab; `initTabs`
  initializes it exactly once and starts the poll only when the tab
  is still active.
- `pathfind.js` loads only in the pathfind mode, after `MapView.init`.
- Acceptance: no eager tags in `index.html`; the served files stay
  reachable; `web_lazy_test.go` pins the chain; the twelve harnesses
  and the webserver tests pass.

## Context

- `stats.js`: 35,629 B raw / 9,422 B gzip; `pathfind.js`: 5,853 B raw
  / 1,974 B gzip.
- `StatsTab.init` only binds the controls (no polling);
  `StatsTab.setActive` owns the timer. No other file references
  `StatsTab`.
- `PathfindUI.init(config)` needs `MapView` initialized, so the map
  boots before the script is injected.
- `main.js` now has one `loadScript(src)` helper shared by the five
  galleries, the statistics tab and the pathfind UI.

## Progress

### 2026-11-12 13:52 UTC - lazy boot of the statistics tab and the pathfind UI

Removed the two eager `<script>` tags from `index.html`; `main.js`
loads `stats.js` on the first Stats tab click and `pathfind.js` in
the pathfind mode. `ensureStats` caches the first load and never
injects the same classic script twice (a second `const StatsTab`
declaration would collide). Added `web_lazy_test.go` to pin the
whole lazy set, and documented the policy in `docs/webui.md`.

Measured regular-page JavaScript: 387,649 B raw / 110,635 B gzip
(427,861 raw / 118,563 gzip before this round). Both rounds together
cut 310,057 B raw and 66,290 B gzip from the first load (galleries
plus these two scripts).

Verification: all twelve `tools/repro_*.js` harnesses pass,
`go test ./internal/swarm/webserver/` passes, `go build ./...`,
`go vet`, `task lint:new` and `task fmt:check` are clean.

## Status

Complete: the regular bot page carries only `buffs_tooltip.js`,
`buffs.js`, `app.js`, `map.js` and `main.js`. The next weight lever
would be a lazy panel split of `app.js`/`map.js`, which does not
reduce bytes by itself and conflicts with the classic-script harness
contract if done as ES modules.
