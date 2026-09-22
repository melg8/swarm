# The persistent kill marks and the corpse face retirement (status: in review)

Started 2026-09-22, branch feature/persistent-kill-marks, issue #6.

## Goal

The reopened issue #6 feedback (2026-09-22 15:47 UTC): the X eyes corpse
pattern disappears when the mob despawns - the owner wants a LONG TERM
collection of the death places on the map instead of a transient corpse
decoration.

What done means:

- every kill of every bot stays visible on the map for the whole
  session (nothing expires by time, only a count cap trims the oldest
  marks);
- the transient X eyes face on the dead unit marker is removed - the
  corpse marker reads as a plain faded gray circle again;
- the fleet kill marks keep the variant 26 X eyes pattern, the
  whole-map scope (all bots), the victim tooltip and the kills toggle;
- the respawn prediction logic of the cell hunt keeps its exact
  behavior (the split of the two logs must not touch the hunt policy).

## Context

- Issue: https://github.com/melg8/swarm/issues/6 (reopened 2026-09-22).
  Previous round: PR #8 (merged) - the icon research, the variant 26
  face, the second round that retired the fleet layer by mistake, the
  third round that restored the fleet ring as a five minute melt.
- The melt chain (why the marks vanished): the hunt loop pruned its
  kill records at `cellKillTTL` (5 min, the records also carry the
  respawn predictions), the registry merged at most 400, the client
  dropped everything older than `killMarkTTLms` (5 min) and melted the
  rest in eight alpha buckets.
- The corpse face: `drawUnitTick` dead branch drew two X eyes on the
  gray body; the user read it as the "short term icon of the
  disappearing dead body" that must go.
- The split: `h.kills` (respawn predictions, 5 min TTL, cap 96) stays
  untouched; a new `h.mapKills` log (no TTL, cap 400) feeds the map.

## Progress

### 2026-09-22 16:05 UTC - the server split and the client rewrite

- `internal/swarm/hunt/cell_policy.go`: `cellMapKillCap = 400`, the
  `mapKills` field, `recordMapKill` (append + count cap) called from
  `cellNoteKill`.
- `internal/swarm/hunt/cell_view.go`: `publishView` publishes the
  persistent log every view tick - picked cell or not (the marks
  survive the hunt stops; the old code cleared the ring when
  `picked < 0`).
- `internal/swarm/webserver/server.go`: `fleetKillMarkLimit` 400 ->
  2000 (the merged session ring).
- `internal/swarm/webserver/web/map.js`: `drawKillMarks` constant
  style (alpha 0.85, size 10, one fill + one stroke pass, no buckets,
  no TTL), `killMarkAt` picks any age, `killFadeBuckets` and
  `killMarkTTLms` removed, `drawUnitTick` dead branch reduced to the
  plain circle (no face), the globalAlpha restore kept before the
  early return.
- Harnesses: repro_map_render (the dead face scenario pins NO X eyes
  on the corpse), repro_zone_hover (constant style, the hours old
  mark still draws), repro_bot_switch, repro_fight_ui, repro_hud,
  repro_movement, repro_stats, repro_buffs, repro_gear - all PASS.
- Go: `go build ./...` clean, the kill mark tests pass
  (`TestCellViewPublishesKillMarks`, `TestCellMapKillLogOutlivesPredictionTTL`,
  `TestCellMapKillLogCapDropsOldest`,
  `TestCellViewPublishesKillMarksWithoutPick`).
- `docs/webui.md`: the four stale spots rewritten (the unit palette
  line, the dead face paragraph, the orange skull paragraph, the fade
  bucket paragraph).
- Not verified live (no Mobius stack in this sandbox): the reviewer
  pulls the branch and reads the map on the running game.

## Status

In review: the branch rebased onto the merged main (57eed69, the
contact pack relaxation of PR #45 - one docs/webui.md hunk merged by
hand, both sides kept), force pushed, PR #46 rides head 5a21081 with
the full 11 check CI green and a clean mergeable state. The next step
is the reviewer read on the issue; the live stack read of the map is
the only unverified leg.
