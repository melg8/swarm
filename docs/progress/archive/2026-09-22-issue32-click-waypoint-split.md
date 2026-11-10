# The clickWaypoint split under the complexity gate limits (status: in review)

Started 2026-09-22, branch feature/click-waypoint-split, issue #32.

## Goal

The follow-up debt of the CI verify gate: `Loop.clickWaypoint`
measured cyclomatic complexity 16 (gate max 15) and sat one line under
the funlen bound, carried by a `//nolint:funlen,cyclop` waiver. Done
means:

- the waiver is gone, `golangci-lint run` reads the repository clean
  without the clickWaypoint waiver;
- behavior identical: the click tests (click_frame, click_guard,
  click_floor, partial_route, town/waypoint follower gates) stay green
  UNCHANGED - no test file edits on the branch.

## Context

- Issue: https://github.com/melg8/swarm/issues/32 (created 2026-09-21).
- The function chained the whole steering decision ladder: the
  distance gate + clip, the z anchoring, the short click extension
  ladder, the aggro camp skip, the aggro aware steering and the
  server click validation.
- The refactor follows the shape the issue sketches: the click
  extension decision into its own function, the z anchoring into its
  own function, the remaining ladder linear under the bound.

## Progress

### 2026-09-22 17:15 UTC - the two extractions

- `anchorWaypointClick` (new): the z anchoring (the server frame
  transport `anchorZToServerFrame` + the measured
  `segmentFrameOffset` shift) plus the `maxMoveDistance` clip with
  the interpolated z; returns the click target and the distance the
  extension ladder keys its rescue floor on.
- `resolveClickExtension` (new): the extension switch verbatim - the
  rescue floor re-aim, the armed behind recovery, the two hold
  verdicts - as `(moveX, moveY, moveZ, send)`; the hold rationale
  comments moved with the code they explain.
- `clickWaypoint`: the remaining linear gate chain (the hold verdict,
  the aggro camp skip, the aggro aware steering, the server click
  validation, the WalkTo) - no waiver, no behavior change; the `wp`
  parameter of the helper dropped on the linter's revive note.
- Verification: `go build ./...`, `go vet`, the full uncapped
  `golangci-lint run` (0 issues - the waiver is gone), the WHOLE hunt
  suite green in one run (79s) with the click test files untouched.

## Status

In review: PR opened with "Fixes #32", CI watched green.
