# The deleveling dropped the level-keyed Newbie Guide buffs

## Round

- Date: 2026-11-12
- Scope: `internal/swarm/hunt/delevel.go` (the target computation and
  the trigger), `internal/swarm/hunt/guide_buffs.go` (the gate
  constants and comments)
- Issue: #0 (owner instruction, no board issue)
- Predecessor: `docs/progress/archive/2026-11-12-issue0-city-shopping-bow.md`
  (no direct dependency - the neighboring round of the same day)

## Problem

The owner saw bots that reached level 16 still walk to the town guards
and delevel back to the Lucky floor: the support magic of the Newbie
Guide is keyed by the character level, so the descent stripped the
higher-level buff windows and the Life Cubic the guides grant in the
16..19 band.

## Root cause

`startDelevel` computed the target as `median + 5` (the last level of
the full item drop chance) and floored it only at the Lucky protection
level 9. Nothing bounded a level 16+ character from crossing back under
16: on the elven level-1 ground a level 16 bot aimed at 9, and the
delevel trigger (level - median >= 7, level >= 10) kept starting the
walk. The guide table itself was correct: it intentionally leaves the
Life Cubic out because the cubic is a summoned effect rather than an
`AbnormalStatusUpdate` (`SupportMagic.java` grants skills 1204, 1040,
1045, 1068, 1044, 1086 and 67 by the level windows), but no code
connected that level table to the delevel floor.

## Reproduction

- Offline: a level 16 character over the level-1 test gremlins had a
  target of 9 before the fix (`delevelTarget`), and the same scene
  with the level-8 cell ground (the `delevel` acceptance dryad scene)
  sent a level 16 character to the guards.
- Detect: `TestDelevelStopsAtTheGuideFloor` and
  `TestDelevelSkipsAtTheGuideFloorOnTheCell` fail on the old code
  (phase `delevel`, target 9 or 13); a passing run keeps the engage
  and announces no target.
- Live: the `delevel` acceptance scenario starts at level 15, under the
  floor, so it stays a valid regression check for the pre-16 cycle.

## Fix

- `delevelTargetFor(level, median)` owns the target: `median + 5`,
  raised to the Lucky floor 9 and, for a character at or above 16, to
  the guide floor 16 (`delevelGuideFloorLevel = guideCubicLevelMin`).
- `delevelWanted` returns false when the computed target cannot sit
  below the current level: a level 16 character never starts the guard
  walk, and a level 17+ character stops at 16. The trigger ordering of
  the live/static medians is unchanged; the minimum level check moved
  to the top of the gate.
- The `guide_buffs.go` comments now record the real Life Cubic window
  and why it stays out of the buff table; the delevel package comment
  and the const comment carry the floor rule.
- `docs/hunting.md`: the deleveling section gains the guide floor
  paragraph, the target formula names the 16 floor, and the guide run
  bullet states the level-keyed coupling.

## Verification

- `go build ./...`, `go vet ./...`.
- `go test ./internal/swarm/hunt -count=1` green (97 s), including the
  three new tests.
- `go test ./... -count=1` green.
- `golangci-lint run` zero findings, `task fmt:check` clean.

## Follow ups

- The live delevel acceptance (level 16 start) is an owner-side run:
  the scenario resets to level 15 today; raising the start level would
  verify the floor live once the stack is available.
- A level 16 character that a death overshoot drops under 16 still
  delevels to the Lucky floor (the rule is level-based, no history is
  persisted); the per-death penalty removes well under one level span,
  so the case stays theoretical.
