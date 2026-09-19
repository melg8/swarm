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
