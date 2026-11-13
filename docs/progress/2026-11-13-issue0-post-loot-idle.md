# The post-loot idle of the hunt loop (status: in progress)

Started 2026-11-13, branch main, issue0 (owner report: the bots stand
idle after a kill - they stare at the corpse after the loot and do not
move to the next target at once).

## Goal

The hunt loop never idles between two fights:

- the loot phase ends the moment the last drop is gone (or the corpse
  grace expires) and the very next engage tick moves the character:
  toward the next valid mob when one sits outside the engage radius,
  toward the zone center otherwise;
- a hunter that already stands central still waits for the respawn
  (there is nowhere to go);
- the behavior is pinned by a fake-clock regression test that fails on
  the old code;
- the live run measures pickup-to-move latencies and shows no 5-6 s
  standstill after the loot.

## Context

- The live probe (2026-11-13 23:50-23:55, bot `test1`, level 17,
  `runs/live-loot-probe.log` plus the 5 Hz state trace) measured the
  real pickup -> next target selection gaps: 18, 13, 12, 8, 7, 7, 6
  and 5 s.
- The root cause: `walkToFarTarget` armed `noTargetSince` on the first
  targetless tick and held the walk for `noTargetPatience` (6 s) -
  "only an empty radius keeps the character walking" - and
  `patrolToCenter` repeated the same gate. After a loot the character
  stood on the corpse until the window expired, then walked the far
  pack (the 23:54:10 pickup -> 23:54:22 selection entry).
- The patience predates the far-target walk (the combat safety round
  of 2026-09-07 added the center patrol behind it); nothing in the
  pick depends on it: the per second target search picks up any mob the
  walk passes, and the walk only fires when the nearest valid mob sits
  outside `attackNearestRange` (1500) or the character stands more than
  `patrolCenterMinDist` (700) from the center.
- The 1 s gap between the target selection and the server side chase is
  the Mobius `FloodProtectorPlayerActionInterval` (1 s): the first
  `AttackRequest` only selects, the repeated one starts the attack, so
  the client cannot legally fire the forced request earlier.
- The bow lure (a covered pick) stands at its standoff while arming the
  bow and the quiver; that is a deliberate ranged approach, not the
  corpse standstill, and stays out of this change.

## Progress

### 2026-11-13 - the regression test and the patience removal

`TestLoopWalksToTheNextTargetRightAfterLoot` rebuilds the live stall
with a fake clock: kill, loot, the last drop disappears, the first
engage tick must send the walk toward a far (2200 units) mob. It failed
on the old code (`game.walks` empty) and passes after the fix.

`walkToFarTarget` and `patrolToCenter` now walk on the first targetless
tick; `noTargetPatience` is deleted and the diagnostics keep
`noTargetSince` as the no-target age. The patrol test now pins the
immediate center walk, the far-target and diagnostic tests no longer
burn the patience. `docs/hunting.md` describes the immediate handover,
`docs/webui.md` and the web UI comment read "no-target age".

### 2026-11-14 - the live measurement of the fixed bot

The fixed binary (`swarm.exe`, built 2026-11-14 00:10) ran as `test1`
(level 18) on the deployed C1 stack while a 5 Hz web-state probe
sampled the bot for 5 minutes (`runs/live-loot-fix.log`, the trace at
`%TEMP%/opencode/loot_probe_t1b.jsonl`). The window carried 5 kills,
5 pickups and 5 loot-phase exits.

The corpse idle is gone. The cleanest entry: the last drop of the
00:22:39 kill disappeared at 00:22:41.703, the phase left `loot` at
00:22:41.904, and the character was `moving` at 00:22:42.103 - 0.2 s
after the drop and with no target for the next 2.4 s of the walk (the
old gate would have held it until 00:22:47.9). Pickup -> next
selection on the near targets: 00:22:06.917 -> 00:22:07.309 (0.4 s)
and 00:23:51.809 -> 00:23:52.310 (0.5 s); pickup -> movement 1.4-1.6 s
(the one-action-per-second pacing of the walk click).

Two of the five exits still stand 4.6-4.8 s before the selection, and
the log names the cause: both are the bow lure
(`Crimson Spider stands covered by Crimson Spider: luring it with the
bow from the standoff`, 00:23:19 and 00:24:09) - the deliberate ranged
pull that arms the bow and the quiver before the shot, not the corpse
idle. The pre-fix log entry it replaces (23:54:08 kill, 23:54:10
pickup, 23:54:22 next selection, 12 s, no lure line) was the patience.

## Status

The fix is measured offline and live. Offline: `go test
./internal/swarm/hunt` (the new regression test fails on the old code),
`task lint:new`, `go build ./...`, `task prepush`. Live: the 0.2 s
drop-to-walk handover above against the 12 s pre-fix entry.

The remaining multi-second standstill is the bow lure standoff, a
deliberate ranged approach (`lure.go`), out of this change: a follow-up
can evaluate shortening the arm cycle, but it is not the corpse idle
the owner reported. Next: commit, push, archive this fragment.
