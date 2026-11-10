# The Newbie Guide dialog stale page race (status: in review)

Started 2026-09-22, branch feature/guide-dialog-stale-page, issue #35.

## Goal

The bot sometimes does not take the Newbie Guide support magic
(issue #35 state dump: `dialog: step 2 ("Receive supplemental
magic."): the next page never arrived`, cooldown 10m, no buffs).
Fix the guide stop so the buffs land every time the server grants
them; never regress the quest and class transfer dialogs that share
the walker.

Acceptance criteria:

- the first bypass of a dialog route is sent only after the fresh
  entry page ARRIVED (never off a stale page of a previous
  conversation of the same npc);
- the guide flow does not fail after the buffs land (the apply
  bypass answers with the effect, no page follows it);
- the full test tree, the lint gate and the fmt gate stay green.

## Context

- Issue #35 with the full state dump; the timeline analysis lives in
  the issue comments (the event ring is insertion ordered and the
  hunt logger mirror is synchronous, so the dump order is causal:
  `step 1 sends` was recorded BEFORE `npc html ... 401 bytes`).
- Root cause 1 (the race): `awaitNewDialogPage` accepted any page of
  the npc that differed from the last page by CONTENT - a stale page
  of the same npc from the previous guide conversation (the store
  holds one slot, no arrival stamp) satisfied the entry wait
  instantly, the first bypass went out BEFORE the server processed
  the interact click. The Mobius C1 server validates every `npc_`
  bypass against the per-scope html action cache built when the
  server SENDS a page (RequestBypassToServer.validateHtmlAction ->
  silent drop on no match) and runs client packets on a thread pool
  (PacketExecutor, 2+ threads), so the early bypass races the cache
  rebuild and is dropped silently - no page ever answers, step 2
  times out.
- Root cause 2 (the false failure): the Mobius SupportMagic handler
  (dist/game/data/scripts/handlers/bypass/npc/SupportMagic.java)
  casts the level-eligible buffs and sends NO html page on success -
  DriveDialog's unconditional final page wait always timed out on
  the guide flow, arming the 10m refusal cooldown even when the
  buffs landed; combined with root cause 1 the bot took the buffs at
  best every other refill.
- Verified server facts (the Mobius C1 source, sparse cloned from
  gitlab): FloodProtectorServerBypassInterval = 3 is 3 GAME TICKS
  (300 ms), not 3 s - the old dialogBypassPace comment misread it;
  the pace itself stays 3.2 s (proven safe, shortening is a separate
  live measurement).
- The deployed 30599.htm entry page carries no comment lines (the
  401 byte page of the dump); the SupportMagic.htm page is 498 bytes
  with the `%objectId%` substituted.

## Progress

### 2026-09-22 16:20 UTC - connection: the html dialog arrival generation

- `GameClient` counts every parsed NpcHTMLMessage (htmlGen, under
  the html lock); `LastHTMLDialogArrival()` returns the page and its
  arrival generation from one locked read.
- Test: TestLastHTMLDialogArrivalGeneration (a byte identical
  re-send advances the generation; a malformed packet does not).

### 2026-09-22 16:25 UTC - hunt: the dialog walker gates its steps on the page arrival generation

- `DriveDialog` captures the arrival generation before the entry
  talk; every step wait (and the final page wait) accepts only a
  page that arrived PAST the previously accepted generation - a
  stale same-npc page never passes, a byte identical re-send does
  (the old content-comparison blind spot is retired).
- The scripted dialog stub models arrivals (gen per served page);
  the quest chain stub bumps the gen on the second conversation's
  entry delivery.
- Tests: TestDriveDialogNoAnswerTimesOut,
  TestDriveDialogSamePageResendAdvances,
  TestDriveDialogStaleSameNpcPageWaitsForFreshEntry (the issue #35
  regression - the golden guide pages of the datapack),
  TestDriveDialogEffectAnswerSkipsFinalWait; the quest chain happy
  path still green.
- DialogStep joins the exhaustruct option aggregates (LinkText is
  the identity, the answer flags are rare opt-ins).

### 2026-09-22 16:30 UTC - hunt: the guide support magic answers with the effect, not a page

- `DialogStep.AnswerIsEffect`: the last step of the guide route
  declares the effect answer - the walker sends the apply bypass
  and returns without the final page wait.
- `receiveGuideMagic` owns the outcome: the buff watch decides
  success ("the support magic landed", no cooldown) or the refusal
  (the 10m cooldown). The happy path no longer arms the cooldown
  after the buffs landed.
- guideBuffWait became a test seam var; tests:
  TestReceiveGuideMagicLandsTheBuffs,
  TestReceiveGuideMagicArmsRefusalWithoutBuffs.
- The dialogBypassPace comment states the real unit (300 ms) and
  the corrected attribution of the 2026-09-12 Q00406 drop.

## Status

In review: branch pushed, PR open, the issue carries the full
analysis. Verification: go build, go vet, golangci-lint run 0
issues, task fmt:check clean, go test ./... green (the full tree).
The live stack verification (a real guide stop through the fixed
walker) is the owner's call - the sandbox of this round had no
deployed Mobius stack.
