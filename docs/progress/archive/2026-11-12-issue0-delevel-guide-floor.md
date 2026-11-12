# The delevel guide floor: level 16 keeps the NPC buffs (status: complete)

Started 2026-11-12, branch main, issue0 (owner instruction: once a bot
reaches level 16 it must not delevel below that level, so the
level-determined Newbie Guide buffs are never lost).

## Goal

- A character standing at level 16 never starts a deleveling: the
  target would be the level itself.
- A character above 16 delevels only down to 16, never to the raw
  drop boundary below it.
- A character under 16 keeps the pre-existing cycle (Lucky floor 9):
  it never held the level-keyed guide effect.
- The verification loop and the lint gate stay green.

## Context

- The Newbie Guide support magic is level-keyed: the 8-24 band grants
  the abnormal buffs per window and the 16..19 band additionally
  grants the Life Cubic (skill 67) - verified in the official Mobius
  C1 source `dist/game/data/scripts/handlers/bypass/npc/SupportMagic.java`
  (the cubic is a summoned effect, never an `AbnormalStatusUpdate`).
- The delevel target used to be `median + 5`, floored at the Lucky
  protection level 9, so a level 16+ bot on a low ground walked to the
  guards and dropped well below the band, losing the cubic and the
  higher-level buff windows.
- Owner wording (translated): "once the bots reach level 16 they must
  not try to delevel below that level, so the NPC buffer buffs, which
  are exactly determined by the level, are not lost."

## Progress

### 2026-11-12 16:10 UTC - hunt: the delevel never aims below the guide window

- `guide_buffs.go`: the `guideCubicLevelMin` constant (16) names the
  lower bound of the Life Cubic window of the gate; the buff table
  comments now state that the cubic is a summon and stays out of the
  table.
- `delevel.go`: `delevelTargetFor` centralizes the target computation
  (median + 5, Lucky floor 9, guide floor 16 for a character at or
  above 16); `delevelWanted` refuses a trigger whose target cannot
  sit below the current level (so level 16 starts nothing) and
  `startDelevel` reuses the helper.
- Unit tests: `TestDelevelStopsAtTheGuideFloor` (level 16 starts no
  guard walk), `TestDelevelTargetClampsAtTheGuideFloor` (level 18 ->
  target 16, the walk starts),
  `TestDelevelSkipsAtTheGuideFloorOnTheCell` (level 16 on the
  outleveled cell keeps farming).
- Docs: the deleveling section and the guide run bullet of
  `docs/hunting.md`; the devlog fragment of the round.

## Status

Complete: build, vet, the hunt package tests, the full suite and the
lint gate green; the live acceptance stays an owner-side run (it
starts at level 15, below the floor, so the scenario is unaffected).
