<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# The flake ledger

The determinism policy of AGENTS.md says a flaky test is a bug in
the test, not in the tolerance. The policy has a blind spot though:
the fix of a flake is only remembered by the session that made it
(a development log round, a commit message) and the next agent has
no way to search the history before paying the same failure again.
This ledger is the searchable memory: every observed flake gets one
row, the fix (or the pin) names itself, and a new session that sees
a familiar failure reads here first.

The rules:

- A row is appended the same session the flake was observed and
  fixed (or pinned). Never delete rows - a repeated entry of the
  same test means the previous pin was insufficient, which is the
  signal.
- The fix always follows the policy: pin the determinism (the
  injected clock, the ordered channel, the synchronised stub), not
  the tolerance (the sleep, the retry count). A row whose fix is a
  longer timeout is a confession, not a cure.
- Live-stack scenarios (the acceptance manager, the e2e scripts)
  flake for different reasons (the server pacing, the flood
  protector, the world state) - those belong here too when the
  flake is in the harness, not in the bot under test.

| Date | Test (package) | Symptom | Fix or pin | Ref |
| --- | --- | --- | --- | --- |
| 2026-09-13 | session report poll (webserver) | `require` inside the `Eventually` condition aborted the poll goroutine on the first transient miss - the miss became a guaranteed 10 s timeout failure (the kill and the story land on two separate aggregator writes) | the condition reports, the caller requires: the poll collects the state and the assertion moved out of the goroutine FailNow path | 8034403 |
| 2026-09-13 | stats history stability (webserver) | the history base was built at an arbitrary clock phase - the bucket boundary decided whether the window held the expected rows | the clock phase pinned: the base is constructed relative to the injected clock so the buckets align the same way every run | 8034403 |
| 2026-09-19 | TestFindPathFromDeckToFarWestZoneStaysOnRamps (pathfind, -race) | the wall time budget of the zone route (`< 5s`, the production hunt tick figure) failed under the race detector: the instrumented binary runs the A* hot loop 6-10x slower, the deck route measured 8.15 s against the plain 0.84 s run | the detector overhead pinned, not the tolerance: the budget scales by `raceDetectorBudget` (a build tag pair, 10 under -race, 1 plain), so the assertion keeps failing on a real 10x planning regression while the instrumented mode stops reporting instrumentation cost as a route failure | e786361 (the race slice first ran the heavy zone route under -race) |
| 2026-09-21 | TestWeaponRunCooldownIsShort (hunt) | the buffless level 11 fixture sat inside the guide band, so the guide priority round's `guideRunWanted` armed the short cooldown window and the "armed character waits out the ordinary cooldown" assertion failed deterministically per fixture, while the whole-suite runs passed (the fixture state, not the clock) | the fixture arms the expected guide buff set (`expectedGuideBuffs(11, false)` through `SetBuffs`) - the guide run stays disarmed and the test pins the ordinary cooldown it meant to pin | 6677b94+ |
| 2026-09-22 | TestGameClientRunStreamsThePositionValidation (connection, -race) | the 2600 ms session window was eaten by the handshake (the char-create drain alone waits up to `charCreateOkWait` = 2 s under -race on a loaded runner), starving the 1 s validation ticker: 0-1 fires where 2 are asserted (the PR #34 reruns measured both, the same tree passing other attempts) | the window stopgap, honestly labeled: the session window 5 s and the poll deadline 5 s, the nudger walk capped at its original 800 unit envelope - the assertion keeps demanding two real ticker fires at the production 1 s period; the deterministic pin (a follow-up round): an injected clock/ticker seam for the run loop so the test drives the ticker instead of racing the handshake wall time | issue #38, issue #33 (the account) |
| 2026-09-22 | the pathfind race slice suite duration (pathfind, -race) | the suite measured 474-558 s on normal runners and hit the 600 s default package timeout on a slow one (the dump shows CPU-bound `parseRegion`, no hang) - runner-class variance on instrumentation cost, not a flaky assertion | the bound raised where it belongs: `-timeout=15m` on the race step (both the live workflow and the canonical docs copy) - a hung test still fails at 15 m; the structural fix (a follow-up round): split the race slice into parallel jobs so per-job duration stops scaling with the whole slice | issue #38, issue #33 (the account) |
| 2026-09-22 | TestGameClientRunStreamsThePositionValidation (connection, -race and plain) | the repeated entry of the morning row - the window widening did not close it. The instrumented -race -count=N runs reproduce at ~every second iteration and show the REAL mechanism: the absorb loop read with a rolling 500 ms deadline that raced the 1 s validation ticker - a mid-frame expiry made io.ReadFull consume and lose the leading bytes of a packet, the fake server's framing desynced permanently (the server read 0x5f where the client sent 0x48) and the reports channel stayed empty (the 0-report mode). Two aggravators: the fake wrote the creation answer inverted (ok before the list; CharacterCreate.java sends the list first, the ok after it) so drainCharCreateOk burned its full 2 s on EVERY handshake ("Char create ok not drained in time" on every pass AND fail), and the 4 s absorb budget sat under the widened 5 s window - the flow tore the conn mid session, Run returned EOF (the PR #39 head mode) | the determinism pins, all three: the fake rides the real creation order (the drain confirms in milliseconds - the 2 s tax is gone everywhere), the absorb loop arms ONE read deadline at the budget end (10 s, sized over the 5 s window for a loaded runner) so a read completes whole or the budget closes the flow, the budget outlives every session window. Measured: 10/10 under -race -count=10 (was ~50% fail at count>=2), the whole connection package -race 35 s -> 11 s | issue #38 (the feedback round), the fake server and absorb fixes of this branch |
