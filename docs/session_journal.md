<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# The session journal (the session dump)

The persistent record of everything that happened to the bots of one
swarm process, from the application start to the shutdown. Where the
state dump is a point-in-time snapshot and the tracker event log is a
512-entry ring, the journal is the full story on disk: a long-lived run
on the user machine (8-24 hours) leaves behind a complete trail an
agent reads to answer why the bot farmed little money, how many stalls
it hit and which fights went badly.

## The three layers

1. **The journal file** `logs/session-<timestamp>-<pid>.jsonl` (the
   `-session-dir` flag, default `logs`, the empty value disables the
   whole subsystem). One file per process, append-only JSONL, one
   record per line, every bot of a fleet writing its own records
   carrying its account id. The file rotates at 64 MB and the rotated
   segment gzips in the background (`logs/` is gitignored).
2. **The in-memory aggregate** the writer goroutine maintains as it
   encodes: the hourly state buckets, the level marks, the per-mob
   fight statistics, the trip and purchase counters, the stall lists
   and the newest story ring.
3. **The compact report** rendered from the aggregate: the artifact an
   agent actually reads (tens of kilobytes for a 24 hour run).

## The wire format

One JSON object per line with short flat keys (the kill line reads
`{"t":1730000000,"b":"test1","e":"kill","mob":"Kaboo Orc","lvl":4,
"dur":8.2,"hp":87}` - roughly 80 bytes):

| Kind | Fields | What it pins |
| --- | --- | --- |
| `build` | `v` | the process identity (the first line of the file) |
| `story` | `m` | a mirrored tracker event: every hunt decision and game event |
| `s` | `lv,xp,ad,hp,x,y,ph` | a 30 s character state sample (the xp is the cumulative total the server tracks) |
| `kill` | `mob,lvl,dur,hp` | one killed mob: the fight length from the confirmed fight start and the health the character ended with |
| `death` | `lv,x,y` | one character death |
| `level` | `lv,xp` | a level up |
| `trip-start` / `trip-end` | `r`, `r,dur` | a town trip bracket with the reason and the duration |
| `buy` | `items,n,cost` | a buy batch with its adena cost |
| `sell` | `n` | a sell batch with its item count |
| `zone` | `mob,r` | a hunting zone switch with the reason |
| `stall` | `r,dur,x,y` | a stagnation watch event (xp or position hold) |
| `repath` | `n` | a stuck-and-replanned walk leg |
| `logout` | `r,ti` | one intentional emergency logout of the hunt safety layer: the honest cause (the mob pile up, the critical health) and the login cooldown - the supervisor's lost record carries the same reason instead of the misleading socket-close error |
| `connect` / `lost` / `shutdown` | `r,m` | the session lifecycle of the supervisor |

The fight clock honesty: the `kill` duration anchor resets with every
kill (and every dropped target), because the Mobius object id free
list hands the SAME id to the respawn of the same spawn point - a
stale anchor matching the recycled id would inherit the elapsed
seconds of every earlier fight against it (the observed six hour run
credited one lieutenant spawn with a 4369 second "fight" accumulated
over 188 kills of the recycled id).

## The data volume (measured)

A live elven-lands hunt writes roughly 60-70 story lines a minute
(the spawn chatter of the world dominates: npc spawns, removals,
combat flags), one kill record per kill and one sample per 30 seconds.
The measured 2.5 minute run wrote 170 lines / 14 KB - a 24 hour run
lands around 6-8 MB raw, under 1 MB gzipped, per bot. The report
stays at tens of kilobytes whatever the journal size: the aggregates
carry the numbers, the story ring carries the last 128 lines and the
journal file stays the drill-down artifact for the deep questions.

The flood insurance: a misbehaving loop that floods the event log
mutes past 120 story lines per bot per minute (the muted count rides
the report), and the 64 MB rotation bounds the disk usage of the worst
runaway.

The flush discipline: the writer goroutine flushes the buffered
records every 2 s, and the very first record of the file lands on disk
at once - a run hard-killed inside the first period still leaves its
build identity line, so a journal file that exists is never a zero
byte mystery (an empty file can only come from a process that died
before NewJournal finished).

## Reading it back

- **The web UI button** "session dump" (next to "dump state" on the
  map toolbar) fetches `GET /api/bots/{id}/session-report` and copies
  the plain text report to the clipboard - one click hands the whole
  run to the agent. A failure never stays silent: the report endpoint
  opens in a new tab with the server answer (a disabled journal, an
  unknown bot), so a broken dump is readable, not just a red flash.
- **The offline CLI** `swarm -session-report <file>` renders the same
  report from any journal file (plain or gzipped, a torn final line of
  a crashed run is skipped): the post-mortem path for a run that is
  already over. `-account` selects one bot of a fleet journal. The
  `-from`/`-to` window bounds the records folded into the aggregates
  (RFC3339 timestamps or bare `15:04` clocks of the session day - a
  run crossing midnight binds 01:30 to the morning after the 22:51
  start), so one interesting stretch of a long session reports alone.
- **The anomaly scan** `swarm -session-anomalies <file>` answers
  "where should I look": it streams the journal once and prints the
  ranked behavior findings of the long run - emergency logout loops
  (the closed-socket reconnect churn of a bot re-entering too-hot
  ground), repeated decision lines (a loop re-issuing the same shape
  every tick: steering ping-pongs, walled leg retries, targetless
  patrols), town trip abort loops, fight duration outliers (the
  respawn clock suspicion - the server recycles object ids), death
  streaks with their position clusters, stagnation stalls, offline
  gaps and the story flood mute. Every finding carries its count, its
  time window and a ready-made `swarm -session-query` drill-down
  command line, so the deep dive starts with a copy-paste.
- **The drill-down query** `swarm -session-query <file>` is the grep
  of the journal: it streams the records through the filters
  (`-account`, `-events kill,death`, `-match "steering"`, `-from`,
  `-to`, `-limit`) and prints one compact line per record. The
  `-context 2m` flag prints the records within a time window around
  every match as dimmed context lines (the `~` prefix), the analog of
  `grep -C` for a time series - the six hour fleet journal answers a
  windowed question without reading anything else.
- **The raw file** rides along when the questions need the full
  record trail: attach it to the report and the agent greps the
  timeline directly.

## The report shape

```
swarm session report
journal: logs/session-...jsonl
period: ... (2h30m)
bot: test1 (entered 1, lost 1)

== character journey ==      level marks, hourly xp/adena/kills table
== economy ==                adena curve, itemized purchases, sell volume
== kills & combat ==         rates, per-mob stats, fight duration histogram
== deaths ==                 the death list with positions
== downtime ==               town trips with reasons, offline gaps, phase share
== stalls & stucks ==        xp/position stalls, freezes, re-paths, flood mute
== session story ==          the newest 60 event lines
```

## Where the events come from

- The **story layer** mirrors every `state.Bot` recorded event through
  `SetEventSink` (installed by `wireSessionBot` of cmd/swarm): the hunt
  decisions of the loop logger mirror and the game events (pickups,
  targets, spawns, rest, chat) land verbatim. The sink is called under
  the tracker write lock, so it hands the line to the journal through a
  non-blocking channel send and never calls back into the tracker.
- The **samples** come from the `session.Sampler` goroutine of each bot
  (30 s reads of the public tracker accessors).
- The **structured events** are emitted at the decision points of the
  hunt loop: the kill and death sites of loop.go, the trip brackets of
  town.go, the buy batches of shopping.go, the zone switches of
  zones.go, the stall events of stagnation.go and the re-paths of the
  town walk.
- The **lifecycle marks** come from the supervisor of cmd/swarm (the
  world entry, the lost sessions with their reasons, the reconnect
  waits and the shutdown).

A nil journal (the disabled mode, the acceptance scenarios, the unit
tests) is a valid receiver: every emission call is a no-op on it.

## The double-record fix that came with it

The loop `logf` used to record every Hunt: line twice - once through
the logger mirror write and once through `NoteAction`. The journal
made the duplication visible (every hunt decision appeared twice in
the story). `logf` now updates the last action view through
`state.NoteLastAction` (no record of its own); the logger mirror of
the wiring (huntEventLogger of cmd/swarm, sessionLogger of the
acceptance runner) is the single recorder.
