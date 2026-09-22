# Kite walk round three: the refusal-driven rotation and the bow-speed window (status: in review)

Started 2026-09-22, branch feature/kite-walk-reclick-ladder, issue #60
(the third round - the follow-up feedback after PR #66 merged).

## Goal

The owner feedback on the merged re-click ladder: "Still didnt fix,
bot should always try to stay at max range in this mode, it should
spam clicks much faster, if its not moving it should be shooting, if
its not shooting it should be moving away from target asap,
calculate proper delays for archering based on bow speed, and
formulas from c1 mobius server, and test them against real server,
not just unit test, review how different implementations allow to
reduce damage on character and select best from real tests."

The third-round acceptance dump the feedback carries names the
dominant mechanism exactly: every kite cycle shows the probe line
("never started the movement, 3 clicks out") followed by the
rotation line - and the character moving only after the rotation.
Done means: the refusal answer of the server rotates the endpoint
AT ONCE (no probe wait), the click pace matches the tick cadence,
the walk window equals the C1 bow disable window computed from the
live pAtkSpd, and the live acceptance run measures the result.

## Context

- Issue: https://github.com/melg8/swarm/issues/60 (the third-round
  dump with the probe/rotation pairs). Project board claim: Agent
  vgkehKoy.
- THE FINDING (the dump, corroborated by the Mobius C1 source read):
  the send and the bare ActionFailed answer land the SAME
  millisecond on specific destination cells while the rotated fan
  lanes walk fine - the MoveToLocation packet handler's refusal
  family (isCompletelyBlocked on the destination cell among
  others). The offline probe (cmd/blockedcellprobe, since removed -
  the finding lives here) checked every dead endpoint of the dump
  against the shipped geodata pack: ALL read open, both at the
  terrain-following z (ValidateClick) and at the click's own z
  (CellBlockedAtZ, the packet-level mirror added to the engine for
  the probe). CONCLUSION: the bot's geodata does not predict the
  server's refusals - the ONLINE ActionFailed evidence is the only
  refusal channel that never lies, and the rotation must ride it
  directly.
- The C1 formulas (the Mobius source read, Creature.java +
  StatusUpdate.java + the item data):
  - timeAtk = 500000/pAtkSpd, reuse = reuseDelay*333/pAtkSpd; the
    Short Bow: reuseDelay 1500 (00000-00099.xml), pAtkSpd 337 (the
    StatusUpdate ATK_SPD 0x12 the dump shows as attr18=337) ->
    (500000 + 1500*333)/337 = 2.97s, the owner's "3 seconds to draw
    shot".
  - The bot now tracks the live pAtkSpd (CharacterState.PAtkSpd,
    the AttrAtkSpd decode) and the walk window reads the formula
    with a sane clamp (1s..5s) and the shipped 2s fallback.
- Dead ends ruled out:
  - The offline destination-cell gate (CellBlockedAtZ in
    kiteLaneResolve): the bot's pack reads the dump's refused cells
    open - the gate would filter nothing; the online rotation stays
    the answer. The engine method stayed (CellBlockedAtZ) as the
    public packet-level mirror for the future rounds.
  - Measuring the disable from the own-shot cadence (the
    last-to-previous shot interval): self-referential - the cadence
    includes the walk window and the re-request latency of the bot
    itself; the formula on the live pAtkSpd is the honest source.
  - Extending kiteStep past 400 (a longer retreat per cycle): the
    walk covers ~430 units in the 3s window at run speed 144
    already - the step length is not the bottleneck; noted for the
    tuning round.

## Progress

### 2026-09-22 21:25 UTC - the refusal rotation and the window

- state: CharacterState.PAtkSpd + the AttrAtkSpd (0x12) decode +
  SelfPAtkSpd; pathfind: Engine.CellBlockedAtZ (the packet-level
  isCompletelyBlocked mirror - the research seam).
- hunt: kiteClickRefused (the ActionFailed correlation on the kite
  click send time, the OtherRequestBetween attribution guard), the
  refused-cell set (kiteWalkDeadCells - the rotation never
  re-clicks a cell the server already refused), kiteRotateDeadEndpoint
  (the shared rotation of the evidence and the probe paths),
  kiteReclickVerdict (the latch + the rotation), kiteWalkWindow
  (the C1 disable formula on the live pAtkSpd, clamped, the 2s
  fallback), the 250ms pacing, the limit 8, the silent probe at
  kiteProbeElapsed (1.2s - past the broadcast gate).
- The combatAvoidUntil of both kite layers rides the bow window -
  the walk spends the whole disable, the forced re-request lands
  the moment the window ends (the engage retry pacing of 1s sat
  inside the window already).

### 2026-09-22 21:40 UTC - the tests and the docs

- hunt: kite_reclick_test.go rewritten for the new semantics (the
  refusal rotation at once - fresh pacing, no probe wait; the
  refused-cell memory through consecutive refusals; the silent
  probe rotation; the no-lane-left stand-down; the bow window
  formula test at pAtkSpd 337; the fallback test) - the full hunt
  suite ok (81s), the state and pathfind suites ok, lint 0
  findings.
- docs: the hunting.md ladder section rewritten for the third round
  (the refusal mechanism, the rotation, the window formula, the
  dump markers), the constants table rows.

## Status

The branch carries the complete round: the refusal-driven rotation,
the refused-cell memory, the bow-speed window, the faster pace, the
tests, the docs. Next: the live acceptance run against the local
deployment stack (the owner ask: test against the real server), the
PR, the issue update, the board move. The user's own stack serves
the final verdict - the refusal verdict line is the instrumentation
that names the mechanism there.
