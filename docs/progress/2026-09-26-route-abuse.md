# The route abuse acceptance scenarios: the elf forest to Gludio rides

Task: the owner asked for the two movement abuse scenarios one scale
up - the bot is a LEVEL 19 character holding ONE MILLION ADENA that
travels from the elven forest creation spawn to Gludio using the
pathfinding as the guide ladder - plus a separate scenario that
covers the same trip the fastest way the packet channels allow.
Branch: `feature/abuse-verify` (on top of the two short movement
abuse rounds of `docs/progress/2026-09-26-abuse-verify.md`).

## The contract

- The start state: the elven fighter wakes at the creation point of
  the elven forest (46045 41251 -3440) as a bare level 19 character
  (339 hp / 137 mp / 135 cp, the C1 level table) with 1,000,000
  adena in the wallet.
- The route: the mesh navigator (the navmesh corridor search the
  fleet bot serves, `hunt.NewNavmeshNavigator`) plans the corridor
  from the spawn to the Gludio teleporter arrival point
  (-12787 122779 -3114) - the segmented hierarchical round answers
  109,000..112,000 units through the Neutral Zone (a twelve minute
  honest run at the 144 run speed the server reports).
- The three rounds:
  1. `desync-route` (temp21): the desync claim ladder hops the
     pathfinding guides (one claim per guide at least 700 units past
     the previous - the stride clears the correction band of
     ValidatePosition.runImpl), a probe click every eighth guide
     reads the adopted placements back.
  2. `cursor-route` (temp22): the keyboard mode arm latches the
     cursor key flag and the interpolated 94 unit claim steps (every
     step UNDER the move speed band, so only the cursor key branch
     can adopt them) ride the corridor polyline.
  3. `fast-route` (temp23): the same corridor the fastest way - the
     whole guide ladder dumped in batches of 16 claims per hunt loop
     tick (two queued batches fill the 32 slot command queue exactly,
     a delayed tick never drops a claim), no pacing between the
     claims, one arrival probe at the far end.
- The pass gates (all three): the plan check (the corridor binds and
  the guides hold), the movement check (the echoes confirm the ride),
  the speed check (at least twice the server reported run speed), the
  distance check (100,000+ units of corridor), the arrival check (the
  final placement within 400 units of the Gludio point) and the store
  check (the character row keeps the Gludio placement; the z band
  tolerates the monotonic z drift of the desync branch - the server
  keeps the higher z of a descending claim because the stack loads
  no geodata).

## Verified live (2026-09-26, the deployed vanilla stack)

- `desync-route`: PASS, 7 of 7 checks - 112 guide claims over 109,003
  units, 14 of 14 probes confirmed (every echo 59 units off its
  claim), 1913 units per second effective against the 144 run speed
  (13x), the whole round 60 seconds; the character row stored
  -12847 122779 -3112.
- `cursor-route`: PASS, 7 of 7 checks - 1383 stream claims over
  112,512 units, 460 of 461 cycles confirmed, 622 units per second
  effective (4.3x), the round 171 seconds; the row stored the Gludio
  placement.
- `fast-route`: PASS, 7 of 7 checks - 112 guides in 7 batches of 16
  claims, the corridor crossed in 3.4 seconds of riding, 29,536
  units per second effective (205x the run speed), the arrival echo
  60 units from the final claim; the row stored the Gludio placement.

## Implementation notes

- The planning prologue (`planAbuseRoute`) runs the segmented round
  the town route corpus measures: one hierarchical query per
  iteration, the partial corridor waypoints appended onto the guide
  ladder, the re-plan from the partial end, at most 12 iterations,
  the whole window bounded by 4 minutes. The tile cache lifts to 12
  for the planning (the corridor crosses nine regions, the production
  default cache of four would thrash) and restores the production
  default after.
- The navmesh tiles are a local build artifact (gitignored):
  `go run ./cmd/navmesh-build -geodata data/geodata -out data/navmesh`
  builds them; the corridor needs the regions 19_19 through 21_21
  (the full pack build also works). Without tiles the route rounds
  fail their plan check with the build hint.
- The route probe aim rides the direction toward Gludio (the short
  rounds probed west along their line) - `pushRouteProbe` walks 60
  units past the guide toward the arrival point, so the mid corridor
  probes never aim backwards into the walked ground.
- The measured z drift of the desync channel over the corridor: the
  server kept z -3112 at the Gludio point (the claimed z), the
  store check tolerance stays at 1500 for the monotonic drift the
  hills could add.

## Files

- `internal/swarm/acceptance/route_abuse.go` - the shared harness:
  the level 19 reset, the segmented route planning, the guide
  ladders (the stride collapse, the 94 unit interpolation), the
  command pushers, the ride evaluation and the store checks.
- `internal/swarm/acceptance/desync_route.go` - the desync route
  scenario (temp21).
- `internal/swarm/acceptance/cursor_route.go` - the cursor route
  scenario (temp22).
- `internal/swarm/acceptance/fast_route.go` - the fast route sprint
  (temp23).
- `internal/swarm/acceptance/route_abuse_test.go` - the unit tests
  (the reset contract, the arrival geometry, the ladder strides, the
  interpolation band, the batching bound, the speed math, the check
  gates, the command payloads).
- `internal/swarm/pathfind/navmesh/route_probe_test.go` - the
  corridor measurement probe (skips without the local tiles).
- The scenario registry: the three rounds own the head of the
  Definitions list (the webui serves them first), the short abuse
  rounds follow at the fourth and fifth slots.
- AGENTS.md: the hypotheses H-010 (the world scale ride) and H-011
  (the unthrottled burst) recorded as verified.
