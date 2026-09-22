# The contact pack regression fix (status: in review)

Started 2026-09-22, branch feature/contact-pack-no-cross, issue #7.

## Goal

The owner reopened #7 with "Again reappeared.": the icon size
reduction (and the pile up that reads like it) came back on the live
map. Done means: the pack fight geometry reproduced in the render
harness, the separation pass fixed so a real melee pack (the bot
plus two to four melee mobs) separates radially at full radii with
no crossings, every existing contact scenario still green, the PR
rides current main with the CI gate green.

## Context

- The map.js no-shrink machinery of the 2026-09-21 rounds (the
  offsets instead of the factor shrink, the facing aware tight pair
  axis) is intact on main - the render harness contact scenarios
  (melee, stacked, facing, near) all pass. The reappearance is not
  a reverted hunk.
- The repro the owner sees is the PACK: a real melee is rarely one
  pair. The bot fights two or three melee mobs at once, every one
  at the collision distance (3.6px at the harness scale, the marker
  radii sum 11.5 - every pair deep overlapping). The old pass
  applied each pair's push immediately (Gauss-Seidel style), so a
  later pair chased the already moved units: the E/W pushes of the
  self cancelled while the mob pairs shoved the south mob east
  through the self's spot, circles stacked exactly on each other
  and the whole pack collapsed onto the west-east line. A stacked
  pair of circles reads as ONE smaller icon - the size reduction
  "reappeared" without any shrink code ever returning.
- The tight pair rule only read the FIRST unit's look direction:
  with every unit heading the same way (the no-heading mobs render
  heading 0 = east) every tight pair separated along the same
  west-east line, which fed the same collapse.

## The fix

- The separation pass is a Jacobi style pack relaxation now: every
  overlapping pair accumulates its push onto the unit's per round
  accumulator and the net pushes apply once per round; the rounds
  repeat until no pair overlaps or the 8 round budget ends. The
  pair order cannot chase a unit across another - the pack spreads
  radially, deterministically (the snapshot order still drives the
  accumulation, the offsets never flicker frame to frame).
- contactAxis(a, b, ...) reads BOTH look directions now: when the
  two look lines agree as one line (mod the direction flip, the
  30 degree slack of lookLineAgreeCos) the pair separates along
  the shared line, each unit backing away from what it looks at
  (the 2026-09-21 field report fix, preserved); when the look
  lines disagree the true connecting axis separates the pair - the
  pack needs the radial spread, and no facing expectation exists
  to violate.
- The new render harness scenario packContact pins the pack: four
  combat mobs at the collision distance east, west, south, north
  of the character. Checks: five full size bodies, no overlap
  deeper than the measurement slack, and every mob stays on the
  side it started (no crossing through the character).
- docs/webui.md: the contact paragraph names the pack relaxation
  and the disagreeing look line fallback.

## Verification

- The harness: all scenarios pass with the fix; the pack scenario
  fails on the pre-fix map.js (the worst overlap depth 2.91, the
  south mob misplaced) - the regression is reproduced and closed.
- go build of the whole tree green; no Go code changed (the round
  is map.js + the harness + docs only).

## Status

The branch is one commit on current main; PR (Fixes #7) with the
CI gate. The issue returns to Ready for review once CI is green.
