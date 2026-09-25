# Feature branch feature/abuse-verify: the movement abuse acceptance scenarios

Task: the owner asked for two acceptance tests, callable through the
webui, that demonstrate packet-level movement abuses against the
vanilla Mobius C1 stack - a created character moves faster than its
available run speed over long distances through

1. the desync branch of ValidatePosition (the "out of sync"
   correction adopts any claimed placement more than one move-speed
   unit away from the server position, with no distance cap and no
   terrain check), and
2. the cursor key movement channel (the keyboard-mode MoveToLocation
   latches the session cursor flag and every following
   ValidatePosition claim is synced straight into the world, at any
   speed, with no validation whatsoever).

Branch: `feature/abuse-verify` off `main`. The scenarios land in
`internal/swarm/acceptance` and register in `Definitions()` so the
webui acceptance panel serves them like every other scenario
(POST /api/acceptance/tests/{id}/run).

## Verified server facts (Mobius C1 source, 2026-09-26)

- `ValidatePosition.runImpl` (clientpackets/ValidatePosition.java):
  after the teleport/cast/vehicle/fall gates and the sane z gate, a
  claim whose 3D distance to the server position exceeds
  `getStat().getMoveSpeed()` (the run speed, 125 for the elven
  fighter) hits the "Check out of sync" branch and the server calls
  `player.setXYZ(claim)` - the desync correction TRUSTS the client.
  There is no upper distance bound on the jump and no geodata or door
  check in that branch; the only door check guards the
  `setLastServerPosition` bookkeeping. The z of the adopted placement
  is the claimed z when `realZ <= _z` (geodata getHeight runs only
  for downward claims, and the stack has no geodata loaded anyway).
- `ValidatePosition.runImpl` cursor key branch: while
  `player.isCursorKeyMovement()` holds, EVERY claim (any distance,
  even under the move-speed band) is synced into the world through
  `setSyncedXYZ` and broadcast as ValidateLocation - and
  `Player.broadcastPacket` sends the packet to the moving player
  itself, so the claiming session observes every adopted placement.
- `MoveToLocation.runImpl` (clientpackets/MoveToLocation.java):
  movementMode 0 (cursor keys) with `KeyboardMovement = true` (the
  reference default) adopts the packet origin within the 1000 unit
  window, latches the cursor key flag and sets the AI move intention;
  movementMode 1 (mouse) clears the flag and only walks (the server
  paced movement at run speed, distance capped at 9900 units).
- The stack runs with no geodata files (`dist/game/data/geodata`
  holds only the readme): `isFalling` never arms (it requires
  `hasGeo`), the obstructed-target click check passes and the desync
  branch adopts z as claimed.
- No flood protector covers ValidatePosition or MoveToLocation (the
  FloodProtector.ini list of 2026-09-26); the official client streams
  about one validation per second while moving.
- The elven lands plateau sits above the water zone cuboids (the
  zone boxes cap at z -3780, the creation spawn z is -3440), so a
  constant-z line stays on the land branch of the handler.

## Plan

1. Raw movement command channel: the `claimPosition`, `cursorWalk`
   and `clickWalk` webui commands (the hunt loop translates them into
   `ClaimValidatePosition`, `CursorKeyWalkTo` and the new raw
   `ClickWalkTo` - a mouse-mode click that does not hand the position
   stream back to the echo ticker, so the abuse probes never race a
   stale echo claim).
2. The `desync-position` scenario (temp19): hop the character 700
   units west per claim from the creation spawn, probe every adopted
   placement with a short mouse click (the self MoveToLocation echo
   origin reports the server position), verify the stored DB position
   after the logout.
3. The `cursor-movement` scenario (temp20): arm the cursor key mode
   toward the far end of the same line, stream fixed 94 unit claim
   hops (under the 125 move-speed band, so only the cursor branch can
   adopt them) and verify the echoed ride plus the stored DB
   position.
4. Docs: the protocol notes (the desync branch), the hypothesis
   registry entries, the webui command list.

## Acceptance criteria

- Both scenarios pass against the deployed vanilla stack, launched
  through the webui API path (the same Manager.Start the buttons
  drive).
- Each demonstrates a measured effective speed of at least twice the
  server-reported run speed over 3000+ units, with the server-side
  position confirmed (per-hop echoes and the stored database row).

## Progress

- (start) Branch created off main; the stack deployed
  (STACK_READY); the server sources read and the facts above pinned.
- (commit 0a43695c) The raw movement command channel: the
  `claimPosition`, `cursorWalk` and `clickWalk` webui commands, the
  hunt loop one shot handlers and `GameClient.ClickWalkTo` (the raw
  mouse-mode click that never hands the position stream back to the
  echo ticker - the probe of the desync round must not reopen it, an
  echo claim of the stale broadcast placement would teleport the
  character a hop backwards through the same desync branch). Unit
  tests: the command semantics (hunt/user_test.go), the click bytes
  and the stream ownership (connection/validate_position_stream_
  test.go).
- (commit 14bc3806) The two scenarios: `desync-position` (temp19,
  the 8 hop claim ladder with per hop probe clicks) and
  `cursor-movement` (temp20, the keyboard mode arm with the 60 claim
  ride in 94 unit hops), the shared abuse session harness (the
  character prologue, the manual session, the graceful logout, the
  stored placement read of the character row) and the webui
  registration at the head of the list. The pass gates: a majority of
  the hops/cycles confirmed by their echoes, the effective speed at
  least twice the server reported run speed, 3000+ units and the
  stored placement within the walk-past tolerance.
- (live verification, 2026-09-26) Both scenarios PASS against the
  deployed vanilla stack:
  - `desync-position`: 8 of 8 hops confirmed (every echo landed
    exactly on the probe target 60 units past its claim), 544 units
    per second effective against the 144 run speed the server
    reported (3.8x), 5660 units covered, the character row stored
    40385 41251 -3440 (60 from the final claim).
  - `cursor-movement`: 20 of 20 stream cycles confirmed (the echoes
    mirrored the claims exactly), 804 units per second effective
    (5.6x the run speed), 5640 units covered, the character row
    stored 40345 41251 -3440.
  - The webui invocation path verified end to end: the panel list
    serves both scenarios, `POST /api/acceptance/tests/cursor-
    movement/run` answers 202, the run passes and the API reports the
    checks and the log tail (the demo script:
    /home/z/my-project/scripts/webui_abuse_demo.sh, outside the
    repository).
- (docs) The protocol notes extended with the desync branch and the
  cursor key branch facts (docs/protocol_description.md), the webui
  command documentation extended with the three raw movement
  commands (docs/webui.md) and the hypothesis registry entries
  H-008/H-009 recorded as verified (AGENTS.md).

## Open items

- The push to origin fails: the GitHub token of the owner prompt
  answers 401 Bad credentials for every auth format (likely revoked
  by the GitHub secret scanning - the token was pasted in plain
  text). The commits sit on the local branch ready to push once a
  valid token arrives.
