# The post-loot idle round (issue0)

Date: 2026-11-14. Scope: the targetless movement of the hunt loop
(`walkToFarTarget`, `patrolToCenter`), the `noTargetPatience` gate.
Issue0 (owner report: the bots stand idle after a kill - they stare at
the corpse after the loot and do not move to the next target at once),
landed on `main`: the fix in `5a841020`, this fragment and the archived
progress fragment in the commit carrying it.

## Problem statement

After the last drop of a kill is picked up the character stands on the
corpse for 5-18 s before it moves to the next target, even when a
valid mob sits in plain sight. The live probe of the pre-fix bot
(`test1`, level 17, `runs/live-loot-probe.log`) measured the pickup ->
next-selection gaps: 18, 13, 12, 8, 7, 7, 6 and 5 s.

## Root cause analysis

- `walkToFarTarget` armed `noTargetSince` on the first targetless tick
  and returned without acting while the stamp was younger than
  `noTargetPatience` (6 s); `patrolToCenter` repeated the same gate.
  The comment named the intent: "only an empty radius keeps the
  character walking".
- The gate predates the far-target walk: the combat safety round of
  2026-09-07 introduced the center patrol behind it. Nothing in the
  target pick depends on it - the per second target search picks up
  any mob the walk passes, and the walk only fires when the nearest
  valid mob sits outside `attackNearestRange` (1500) or the character
  stands more than `patrolCenterMinDist` (700) from the center.
- A kill leaves the bot in `engage` with no target; the first tick
  stamped `noTargetSince`, the next five seconds returned early, and
  the walk (or the selection of a mob the walk would have reached)
  only started after the sixth second. The 23:54:10 pickup ->
  23:54:22 selection entry is one such window (12 s: the gate plus the
  walk and the server reply).
- The 1 s between the target selection and the server side chase is
  the Mobius `FloodProtectorPlayerActionInterval` (1 s): the first
  `AttackRequest` only selects, the repeated one starts the attack, so
  the client cannot legally fire the forced request earlier.

## Reproduction

- Offline: `TestLoopWalksToTheNextTargetRightAfterLoot`
  (`internal/swarm/hunt/loop_test.go`) rebuilds the live stall with a
  fake clock - kill, loot, the last drop disappears, the first engage
  tick must send a walk toward a mob 2200 units away. It fails on the
  old code (`game.walks` empty) and passes after the fix.
- Live: run the bot against the stack, drop its state at 5 Hz through
  `GET /api/bots/test1/state`, kill and loot, and measure the sample
  where the last drop disappears against the next `moving` sample.

## Fix

- `walkToFarTarget` walks on the first targetless tick: the stamp is
  still armed for the diagnostics, no early return follows.
- `patrolToCenter` does the same; `noTargetPatience` is deleted.
- `noTargetSince` remains the no-target age of the diagnostics
  (`noTargetForMs` of the web state), so `docs/webui.md` and the
  `app.js` comment read "no-target age".
- `docs/hunting.md` describes the immediate handover.

## Verification

- `internal/swarm/hunt/loop_test.go`: the new regression test, the
  patrol test now pinning the immediate center walk, the far-target
  and diagnostic tests no longer burning the patience
  (`deck_freeze_repro_test.go`, `engage_repro_test.go` adjusted).
- Full `go test ./internal/swarm/hunt/` suite green, `go build ./...`,
  `task lint:new`, `task prepush`, `task fmt:check` clean.
- Live (`test1`, level 18, fixed binary of 2026-11-14 00:10, 5 min,
  5 kills and 5 pickups): the last drop of the 00:22:39 kill
  disappeared at 00:22:41.703, the phase left `loot` at 00:22:41.904
  and the character was `moving` at 00:22:42.103 - 0.2 s after the
  drop, with no target for the next 2.4 s of the walk (the old gate
  would have held it until 00:22:47.9). Near-target handovers:
  pickup 00:22:06.917 -> selection 00:22:07.309 (0.4 s) and pickup
  00:23:51.809 -> selection 00:23:52.310 (0.5 s); pickup -> movement
  1.4-1.6 s, the one-action-per-second pacing of the walk click.
  Numbers in `docs/progress/archive/2026-11-13-issue0-post-loot-idle.md`.

## Follow ups

- Two of the five live loot exits still stand 4.6-4.8 s before the
  selection, and the log names the cause: the bow lure ("stands
  covered by ...: luring it with the bow from the standoff"). Arming
  the bow and the quiver is a deliberate ranged approach, not the
  corpse idle; a separate round can evaluate the arm cycle.
