# The coverage reporting continuation - six rounds: the merge refresh, the top-up, the journal, the smoke pass, the botlog decoders and the character wire formats (status: in review)

Started 2026-09-22, branch feature/coverage-reporting, issue #9.

## Goal

Keep melg8/swarm#9 ("Improve test coverage") moving: PR #24 must
stay mergeable and CI-green against the evolving main, the committed
coverage baseline must reflect the merged tree honestly, and the
remaining low-coverage packages named by the audit get topped up.

## Context

- The first two rounds of this issue (2026-09-21) landed on PR #24:
  the coverage_delta.sh repair, the packet builder failure-walk
  suites (to_game_server 65.3 -> 97.9, to_auth_server 78.1 -> 98.6),
  the cmd/swarm helper tests (11.4 -> 19.6), the version identity
  fallbacks (78.3 -> 87.0), tools/coverage_report.sh (the
  worst-first audit) and the live CI workflow. The full history is
  in the issue #9 comments and the Round 129 entry of
  docs/development_log.md.
- 2026-09-22: main absorbed PR #39 (the CI flake fixes, the trigger
  split, the baseline refresh split) and PR #34 (the merge clash
  policy - docs/progress/ fragments replace the shared
  agent_progress.md appends), which left PR #24 with merge
  conflicts (docs/agent_progress.md, runs/coverage-latest.txt,
  add/add tools/coverage_report.sh).
- The audit's remaining candidates, recorded in the issue:
  acceptance 27.8, huntaudit 24.3, acceptance/botlog 37.3 (the
  live-stack-bound orchestration - a design round of their own),
  npcdata 82.5 and connection 73.9 (the drift packages to top up),
  cmd/swarm 19.6 (the main.go wiring, needs seams), the eight 0%
  probe tools (one combined smoke pass at most).

## Progress

### 2026-09-22 08:20 UTC - the main merge resolves the PR #24 conflicts
- agent_progress.md: main's frozen version wins (the file is a
  frozen archive since the clash policy round); the coverage round
  context lives on in this fragment.
- tools/coverage_report.sh: both sides byte-identical, kept ours.
- runs/coverage-latest.txt: resolved provisionally with ours, then
  regenerated from the actual merged tree (the numbers of both
  parents were measured on different trees - neither is the truth
  of the merge result).

## Status

The merge commit is in; next: the full gate pass (build, vet, lint,
fmt, tests, the coverage regen and the delta gate), then push and
the top-up round for the drift packages.

### 2026-09-22 08:45 UTC - the top-up round: the drift packages close
- connection 73.8 -> 82.1 (+8.3, past the 76.3 pre-drift level): the
  interaction actions suite (Say, ClickObject, InteractPull,
  ClearTarget, AcquireSkill, UseMagicSkill - the seven dead 0%
  senders), the SetSendTap plaintext observer (the send-side tap
  pair), the CreatureSay world chat dispatch (valid line lands in
  the tracker window, truncated packet logs and never lands) and
  the skill round (SkillList bookkeeping + the self MagicSkillUse
  cast/reuse windows).
- npcdata 82.5 -> 100.0 (+17.5, past the 85.6 pre-drift level): the
  merchant spawn accessors, the self heal flags and the initial
  consume clamps, the class teacher dictionary, ClassCastsMagic,
  the MPCostOf clamps, the itoa fallback branches and the mapped
  branch of NPCWireTemplateID.
- The baseline regenerated from the merged tree (all 28 packages
  measured fresh): only gains plus one -0.1 hunt jitter (main's
  flake-fix test changes), no drop near the 2 pp gate.
- Gates: build, vet, golangci-lint 0 issues, fmt:check clean, the
  logfmt scan green, the touched suites green.

## Status (updated)

The merge + top-up work is complete on the branch; next: push, the
CI verdict (verify, coverage, acceptance-list), then the issue
comment and the board move to Ready for review.

### 2026-09-22 09:05 UTC - the round closes
- Pushed 15b3270: CI verify, coverage and acceptance-list all
  success, the PR mergeable clean against main.
- The issue comment carries the round report; the board card sits
  in Ready for review.

## Status (final)

In review: PR #24 head 15b3270, green, mergeable. The next round of
this issue (if it reopens) starts from the remaining candidates list
in the issue comment.

### 2026-09-22 09:40 UTC - round three: the journal dispatches close
- connection 82.1 -> 86.2, zero 0% functions left in the package:
  the quest journal (QuestList 0x98: the states, the quest bound
  items, the truncated parse failure), the buff bar
  (AbnormalStatusUpdate 0x97: both effects with levels through
  Snapshot().Buffs), the npc dialog round (NpcHTMLMessage 0x1B: the
  last-html latch through LastHTMLDialog, the bypass links through
  dialogLinks into the tracker dialog page) and the client Close
  lifecycle (the first close nil, the second errors, a send after
  the close fails).
- The baseline line moves with the measurement (the test-only change
  touches no other package).
- Gates: build, vet, lint 0 issues, fmt clean, logfmt green.

## Status (round three)

The journal round is on the branch; next: push, the CI verdict,
the issue comment and the board move.

### 2026-09-22 10:00 UTC - round three closes
- dc75e52: CI verify, coverage, acceptance-list all success;
  PR #24 mergeable clean.
- The issue comment carries the round report (connection 86.2, no
  0% functions left); the card sits in Ready for review, the claims
  are clean.

## Status (round three, final)

In review: PR #24 head dc75e52. The connection package is at 86.2
with every dispatch path, sender and observer under test. The
remaining #9 candidates: the live-stack packages (a design round),
the cmd/swarm wiring, the probe tool smoke pass.

### 2026-09-22 10:25 UTC - round four: the probe tool smoke pass
- The eight 0% cmd tools all carry tests now: navmesh-export 25.0
  (the region spec and the endpoint parsers), navmesh-build 28.1
  (the region selection: the spec forms and the directory scan),
  navanalyze 16.0 (the z fight counter on synthetic tiles),
  cornerprobe 7.1 (the leg height back extrapolation, the adapters,
  the hunt navigator filter mirror), dbpos 22.2 (the plain name
  filter), counterprobe 2.5 (the id parse, the region path
  arithmetic, the key readback), geotest 58.8 and stuckprobe 47.6
  (the missing-data smoke runs: no panic, clean exit).
- The baseline gains the eight tool lines (36 packages total).
- Gates: build, vet, lint 0 issues, fmt clean, logfmt green.

## Status (round four)

The smoke pass is on the branch; next: push, CI, the issue comment
and the board move. The remaining #9 candidates stay the live-stack
seam design round and the cmd/swarm wiring.

### 2026-09-22 10:35 UTC - round four closes
- c674d23: all 11 checks green, PR #24 mergeable clean.
- The issue comment carries the round report; the claims are clean.

## Status (round four, final)

In review: PR #24 head c674d23. Four rounds on this issue today:
the merge repair, the drift top-up (npcdata 100, connection past
its pre-drift level), the journal round (connection 86.2, no 0%
functions) and the probe tool smoke pass (eight tools off 0%).

### 2026-09-22 11:05 UTC - round five: the botlog decoder round
- The "live-stack-bound" label on acceptance/botlog was partly
  wrong: its gaps are mostly the recv packet decoders - pure
  functions over packet bytes, needing fixtures not live sockets.
- botlog 37.3 -> 51.0: the movement family (MoveToLocation), the
  target family (TargetSelected/Unselected/MyTargetSelected), the
  status update (with the attribute name table), the ground item
  flow (SpawnItem/DropItem/GetItem/DeleteObject), the npc dialog,
  the social action, the teleport and the wait type - plus the
  truncated-packet table walking the dispatch with one byte
  packets (every decoder's short fallback).
- The remaining 0% decoders (CharInfo, UserInfo, CharSelectInfo,
  CharSelected, ItemList/InventoryUpdate internals) need the
  heavier character wire formats - recorded for the next round.
- Gates: build, vet, lint 0 issues, fmt clean. Baseline line
  updated.

## Status (round five)

The botlog decoder round is on the branch; next: push, CI, the
issue comment and the board move.

### 2026-09-22 11:55 UTC - round six: the character wire formats
- The owner moved the issue back to Ready (11:11 UTC, no comment);
  the recorded next round started: the character wire decoders of
  botlog, the last 0% decoders of the package.
- botlog 51.0 -> 87.6, zero 0% functions left in the package:
  - The recv character family (decode_recv_character_test.go):
    CharInfo (the player spawn line with the running/standing and
    the dead flag suffixes), UserInfo (the self vitals/load line),
    CharSelectInfo (the two character account list, serialized
    through the exported ToBytes serializer - the round five note
    about the unused serializer), CharSelected (the handover line),
    ItemList and InventoryUpdate (the adena/arrow/dagger stacks
    with the equipped and +3 enchant suffixes, the add/modify/
    remove change verbs).
  - The recv world broadcasts (decode_recv_world_test.go): the
    stance and rotation family (ChangeMoveType run/walk, StopMove,
    the rotation pair, the auto attack start/stop), the chase and
    placement pair (MoveToPawn, ValidateLocation), the skill book
    (the passive Long Shot and the active Power Strike through the
    dictionary) and the buff bar (AbnormalStatusUpdate with the
    Wind Strike and Self Heal names) - plus the long link trimming
    of the npc dialog (the 80/48 cut with the "..." suffix through
    trimText).
  - The send request family (decode_send_round_test.go) runs
    against the PRODUCTION serializers of to_game_server (no hand
    built twins): the session family (protocol version, enter
    world, logout, appearing), the action click (plain and shift),
    the session handover (auth login, character create, character
    select), the item requests (drop, use, destroy, the run/walk
    stance switch), the shop batches (the sell and buy continuation
    lines), the bypass command, the skill cast (with the ctrl
    marker), the action use, the acquire skill, the restart point
    (village and agathion) and the validate position claim - plus
    the unknown opcode line and the truncated send fallbacks.
- The truncated recv dispatch table gained the four character
  opcodes (0x03, 0x04, 0x1F, 0x21).
- The production fix of the round: decodeRecvRotation parsed both
  rotation packets through ParseBeginRotationPacket, but the stop
  rotation wire is two ints shorter (speed + unknown byte behind
  the heading, not side + speed) - every legit StopRotation logged
  as "stop rotation (short payload)". parseRotation now picks the
  parser by the opcode; the log renders the real stop heading.
- The baseline regenerated from the full tree (all 36 packages,
  the delta tool's honest run): the botlog line is the only move,
  no drops. Total statement coverage 74.4%.
- Gates: build, vet, golangci-lint 0 issues (full tree), fmt:check
  clean, the botlog suite green at 87.6.

## Status (round six)

The character wire round is on the branch; next: the atomic
commits, the rebase onto main, the push, the CI verdict, the issue
comment and the board move. The botlog decoder surface is closed;
the remaining #9 candidates are the live-stack seam design round
(acceptance 27.8, huntaudit 24.3) and the cmd/swarm wiring.
