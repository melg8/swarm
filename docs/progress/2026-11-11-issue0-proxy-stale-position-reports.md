# The proxy position reports of an attached client (status: in progress)

Started 2026-11-11, branch main, issue0 (owner report: the city
shopping errand works with the bot alone, but with a real C1 client
attached the bot gets stuck on the stair railing, opens the teleport
menu, never teleports, idles tens of seconds and walks off to the
ordinary shop).

## Goal

With a real client attached through `-proxy`, the bot-driven character
must complete the same errand as without the client: the attached
client's local view never moves the server side character, and a
client view that desynchronises (the frozen pawn of the stair railing)
heals through the proxy's own `ValidateLocation` correction. Done
means:

- the proxy never transits a client `ValidatePosition` (0x48) that the
  Mobius out of sync branch would answer by snapping the character
  (`distance3D > getStat().getMoveSpeed()`, 125 for the level 15 elf);
- a live client that follows the bot transits unchanged (manual
  control keeps the server contract);
- regression tests cover the short stale view (a few hundred units,
  inside the old 2000 unit window) and the projection math;
- the real bot and the `city-shopping` acceptance stay green.

## Context

- The Mobius handler
  (`ValidatePosition.runImpl`,
  `java/org/l2jmobius/gameserver/network/clientpackets/ValidatePosition.java`)
  snaps the character to the reported place whenever the 3D distance
  exceeds one move speed; there is no upper bound. The observation
  branch only answers a report 500+ units away, so a report between
  one move speed and 500 units snapped the character with no
  correction to the client at all.
- The proxy already intercepts reports beyond 2000 units
  (`validateDivergenceLimit`, `internal/swarm/proxy/game.go`) and
  answers them with a synthesized `ValidateLocation`. The 2000 was
  sized to one walk segment against the tracker position, which holds
  the *segment start* while the character moves.
- The railing case is inside that window: the client pawn froze at the
  stair railing, the bot stood at the gatekeeper ~500 units away, the
  report transited, the server snapped the character back, and the
  teleport bypass then failed the 250 unit `Npc.INTERACTION_DISTANCE`
  gate silently (the menu had opened before the snap).
- The fix estimates the server side position instead of comparing
  against the segment start: the Mobius move loop advances the
  creature toward the destination at `getRunSpeed()` per 100 ms tick
  (`Creature.updatePosition`), so the tracker can project the current
  place from the broadcast origin, the destination, the run speed and
  `MoveAt` (the same estimate `projectedPosition` makes for the other
  objects, in 3D because the snap check is 3D).
- The owner run's `proxy.log` (15:43-15:45, bot temp25) shows the
  client reporting 0x48 at ~1 Hz and no stale-view log line: every
  report stayed inside the 2000 unit window.
- The 3.2 s `bypassPacer` between the gatekeeper menu and the teleport
  click widened the window in which a snap can move the character
  out of range; the proxy fix removes the snap, the pacing stays.

## Progress

### 2026-11-11 - investigation and the fix plan

Read the proxy relay and the Mobius handler, pinned the threshold
mismatch (proxy 2000 units 2D against the segment start, server one
move speed 3D against the live position) and the failure chain. The
fix: a `state.Bot.SelfProjectedPosition` estimate plus a proxy limit of
the live run speed plus one tick of slack.

### 2026-11-11 - the self projection in the tracker

Added `state.Bot.SelfProjectedPosition` and the
`CharacterState.projectedPosition` helper: a standing character keeps
its broadcast place, a mover advances along its segment at the live run
speed and clamps at the destination. Verified against the server:
`GameClient.java:493` sets a player running at login
(`player.setRunning(); // running is default`), so
`CreatureStat.getMoveSpeed()` returns the run speed the tracker carries
and the server's 100 ms `Creature.updatePosition` step matches the
projection. The server re-broadcasts `MoveToLocation` once per second
mid movement (`broadcastMoveToLocation`), so the projection origin is
at most one second old. Tests: `self_projected_position_test.go`
(segment interpolation with z, the arrival clamp, the standing and
speedless fallbacks, the empty state).

### 2026-11-11 - the proxy threshold against the projected place

`clientViewDiverged` now compares the report in 3D against
`SelfProjectedPosition` with the limit `SelfRunSpeed() + 25`
(`validateDivergenceSlack`), and `correctClientView` answers with the
projected place. The old 2000 unit 2D window against the broadcast
origin let the railing report transit. Tests:
`validateposition_test.go` (the stale death spot, the railing case at
the gatekeeper, the live 141 unit transit, the report beyond the window
corrected, the zero report passing through). `docs/proxy.md` carries
the new contract.

### 2026-11-11 - the live proof on the deployed stack

The proxy E2E attaches a client, sends a position report 400 units away
(inside the old 2000 unit window, outside the server's one move speed)
and asserts the local `ValidateLocation`; the click that follows makes
the server broadcast the origin it holds, so a transited report would
have snapped the character 400 units away before that click. Green
against the deployed C1 stack on the Windows host
(`SWARM_PROXY_E2E=1 SWARM_PROXY_E2E_LOGIN=127.0.0.3:2106`). The login
address became overridable (`SWARM_PROXY_E2E_LOGIN`, the Windows
deployment binds 127.0.0.3) and the proxy log file now closes before
the temp dir removal (Windows refuses to delete an open file).

### 2026-11-11 - the stuck run root cause: the client move veto

The owner rerun with the real client still stuck: the bot at
(45651, 51288, -2832) re-planned the identical waypoint every ~5.5 s.
The packet log (`logs/acceptance/city-shopping-20261111-162140-2212-1.log`,
lines ~825-1000) shows the client pawn frozen at (45604, 51295) sending
`CannotMoveAnymore` (0x36) 5 ms after every bot click, then the server
answering `StopMove` at the bot place. The Mobius handler
(`CannotMoveAnymore.runImpl` -> `notifyActionArrivedBlocked` ->
`setIntentionIdle` -> `clientStopMoving` -> `stopMove`) cancels the
player's move intention and ignores the reported place, so the client's
local collision vetoed the bot's own walk. The proxy now drops every
veto and answers it with the same `ValidateLocation` correction the
server sends for an out of sync report (`discardClientVeto`, the
correction sender extracted into `sendViewCorrection`). Tests:
`cannotmove_test.go` with the captured payload of the stuck run. The
veto policy and the reason a distance test cannot classify it are in
`docs/proxy.md`, the packet layout in `docs/protocol_description.md`.

### 2026-11-11 - the quest walker move start watchdog

Defense in depth for any deployment that swallows accepted move
requests (H-006): `followWaypoint` now returns a `waypointOutcome`,
names a waypoint `waypointBlocked` when the click started no movement
within `questMoveStartWait` (3 s) and the tracker reports no walk, and
`followPlannedSegment` skips blocked waypoints within the plan (all
blocked -> the direct click, a bounded failure, never the same
waypoint loop). A blocked waypoint is never re-clicked while the
verdict is pending, and `awaitSegmentProgress` gives the direct click
the same verdict so a dead click fails the route at once instead of
standing out the 10 minute walk budget. Tests:
`quest_waypoint_blocked_test.go` (the skip of every waypoint, the
walked plan untouched, the fast dead click, the running walk).

## Status

In progress: both fixes are in the tree with offline tests and the
earlier proxy position fix is verified against the live stack; the
remaining confirmation is the live proxy E2E rerun and the owner rerun
with the real C1 client attached.
