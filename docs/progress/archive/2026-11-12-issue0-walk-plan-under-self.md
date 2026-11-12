# The walk plan paints under the self marker (status: complete)

Started 2026-11-12, branch main, issue0 (owner report: the walk
route covers the character icon and cuts it in two).

## Goal

The character marker stays whole while the character walks a planned
route: the walk plan (the pastel flow line, the waypoint beads and
the destination marker) paints before the self marker, so the self
marker composites on top of the route. Done means:

- the `MapView.paint` layer order draws `drawWalkPlan` between
  `drawObjects` and `drawSelf`;
- the recording harness pins the order: the plan fills and strokes
  sit before the self body fill and the self ring stroke;
- `docs/webui.md` records the layer rule;
- `node tools/repro_map_render.js` and the other map harnesses pass,
  and `go test ./internal/swarm/webserver/` is green.

## Context

- The plan is `snapshot.walkPath` from `state.Bot.SetWalkPlan`; the
  hunt loop publishes it for every walking phase (manual move, town
  trips, guard walks).
- `drawWalkPlan` used to run after `drawSelf` since the visible walk
  plan landed, so the flow line, the beads and the destination marker
  painted over the character icon and split it in two.
- The character's own server side dashed destination line already
  draws under the marker inside `drawSelf`; only the plan needed the
  reorder.
- The pathfind test overlay (the A/B markers and the A* lines) has no
  character on the map, so it is unaffected.

## Progress

### 2026-11-12 - paint the walk plan under the self marker

- `map.js`: `drawWalkPlan` moved between `drawObjects` and
  `drawSelf`; the `drawWalkPlan` and `drawSelf` comments carry the
  new layer rule.
- `tools/repro_map_render.js`: the walk flow scenario checks the
  recorded order (plan fills before the self body fill, plan strokes
  before the self ring stroke). The check fails against the previous
  `map.js` (self fill 3 vs plan fill 6) and passes now.
- `docs/webui.md`: the walk path section records that the plan paints
  under the self marker.
- Harnesses: `repro_map_render`, `repro_hud`, `repro_movement`,
  `repro_bot_switch`, `repro_zone_hover`, `repro_gear` all pass;
  `go test ./internal/swarm/webserver/` is green.

## Status

Complete: the layer reorder, the harness check and the doc sentence
landed; `task prepush` is green.
