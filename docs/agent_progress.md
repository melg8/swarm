# Agent progress log

Crash-safe task tracking: the current task, its full context and
per-commit progress live here (see the "Work protocol" section in
AGENTS.md). Entries are append-only; a new agent resumes the newest
unfinished entry.

This file carries ONLY the active and the most recent context:
finished task entries and older progress streams move to
`agent_progress_archive.md` (append-only, same order). The permanent
root-cause history of every round lives in `docs/development_log.md`;
check the archive when the recent context references an older task.

## Active task: the delevel freeze under an attached client - the position ping pong
)

Started: 2026-09-10. Branch: `feature/proxy-server`. Commits as melg8.
Other agents may push to the same branch concurrently - rebase before
every push.

### Goal

The user request (2026-09-10, Russian): during the deleveling the bot
runs to a guard, hits it, dies, revives - and on the client an attack
animation plays as if it hit again (no visible damage), the bot "sees"
the target (its known list holds the guard after the respawn) and then
stands doing nothing for a long time instead of running back to the
guard. Debug it, find the root cause, cover it with tests, fix it and
explain it. The live dump (build d09e733) shows: the delevel death
cycles run 12:02-12:04, the event log holds full known-object list
flips between the Elven Village and the Starden guard camp every
second (12:04:16-12:04:19), no Die packet and no death event for the
12:04:41 sequence, then 26+ s of silence with the walk plan pointing
at the Starden spawn while the character stands at the village.

### Root cause (reproduced live on the local stack)

The Mobius `ValidatePosition.runImpl` "Check out of sync" branch
accepts any client position report beyond one move speed of the
server position **with no distance bound** and snaps the server side
character to it. The C1 client attached through the proxy reports its
local pawn position periodically, and that local view is stale for
every bot-initiated relocation it did not initiate itself: the death
spot held across the bot's village restart (the pawn freezes while
the death dialog is up), the teleport screen freeze, the long walk.
Every report then dragged the character back - the bot's walk legs
pulled it toward the village, the next report snapped it back to the
guard camp: the observed object list ping pong. The delevel state
machine desynced from the server truth (its tracker only follows the
server broadcasts, the snaps broadcast nothing): the fight stage
believed melee range while the character stood 3000+ units away, the
attacks swung from afar (the "animation without damage"), the guards
"never fought back", both guards got marked as tried, the delevel
aborted, and the return walk stalled in the same ping pong.

Secondary finding: the town guards are level 70 (Kendell/Starden
templates 30218/30220), the deleveling character is in the low tens,
and the vanilla `calcHitMiss` floors the hit chance at 20 percent
there - a whole 20 s fight timeout without a single landed blow is
the normal miss variance of that gap (~27 percent of stages), which
blacklisted healthy guards and aborted delevelings even without the
desync.

### Implementation

- The proxy intercepts the client's ValidatePosition
  (internal/swarm/proxy/game.go transitToServer): a report that
  contradicts the attached session's tracker position beyond
  validateDivergenceLimit (2000 units 2D) never transits; the proxy
  answers the client locally with a synthesized ValidateLocation
  (0x76) carrying the live place of the played character - the same
  correction the server sends for an out of sync report, which heals
  the client's stale view so the next reports transit again.
  In-sync reports transit unchanged (the manual control keeps the
  server contract).
- The delevel fight stage (internal/swarm/hunt/delevel.go) no longer
  blacklists a guard after a timeout window without a single landed
  blow: the tracker records the last landed own blow
  (internal/swarm/state SelfLandedHit, fed from the Attack packet hit
  flags) and the stage extends while nothing landed (the miss
  streak), switching guards only when landed damage went unanswered
  (the stale guard AI).
- Tests: internal/swarm/proxy/validateposition_test.go (the stale
  report corrected locally and never transiting, the healed report
  transiting again, the tracker move shifting the acceptance window,
  the live in-range report transiting unchanged) and the delevel
  suite (TestDelevelMissStreakExtendsFightStage new,
  TestDelevelFightTimeoutSwitchesGuard now requires landed damage).

### Verification

- go build, go vet, go test ./... all green.
- golangci-lint: no new issues in the touched files (the baseline
  holds pre-existing findings of the newer local linter version in
  untouched files).
- Live repro on the local stack (character pushed to the outleveled
  Pincer spot, deleveling to 23, a scripted C1 client attached
  through the proxy): before the fix the delevel broke within two
  death cycles (the guards "do not attack" timeouts, the abort, the
  stalled return walk - the exact user report); after the fix 23+
  death cycles at the healthy 35-40 s cadence, the correction firing
  at every restart (including the exact user signature: the client
  reporting the death spot 42971 51372 against the bot at the village
  45091 49002), the client view healed and following the walks, no
  aborts, no stalls.

### Status: done (2026-09-10)

- The fix, the tests, the docs (hunting.md deleveling section, the
  proxy.md "client position reports" section) and this entry land in
  one commit.
