<!-- SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com> -->
<!-- SPDX-License-Identifier: MIT -->

# runs/

The soak metrics trail and the milestone artifacts of the autonomous
solo 1-60 ladder. The directory is created by the first acceptance
run; the files here are the evidence the ROADMAP milestones close on.

## metrics.jsonl

One JSON line per acceptance run (the `soak` scenario and the
`level-milestone` scenario of T-003; any future scenario that adopts
the trail). The file is append-only: the acceptance package opens it
with `O_APPEND` and writes one marshalled row per run, so parallel
runs never interleave.

Note on the early rows: the rows written before 2026-09-12 08:10 UTC
carry an inflated `xpPerHour` - the cumulative XP helper of the trail
double counted the level start threshold on top of the total exp the
server broadcasts (the fix and the source evidence live in
`internal/swarm/acceptance/soak_metrics.go`, the round 66 entry of
`docs/development_log.md`). The rows stay as written: the trail is
append-only history.

Fields (the M1 acceptance contract, see `docs/BACKLOG.md` T-001 and
`docs/ROADMAP.md` M1):

| Field | Type | Meaning |
| --- | --- | --- |
| `date` | string | The run end time, ISO 8601 UTC (RFC 3339). |
| `scenario` | string | The scenario id (`soak`, `level-milestone`). |
| `durationSec` | int | The actual run duration in seconds. |
| `startLevel` | int | The character level at the run start. |
| `endLevel` | int | The character level at the run end. |
| `xpPerHour` | float | The experience gained per hour (cumulative XP delta / duration hours). |
| `deaths` | int | The alive->dead transition count over the run. |
| `adena` | int | The adena total at the run end. |
| `stuckEvents` | int | The hunt loop re-path count delta (each re-path is a stuck-and-replanned leg). |
| `status` | string | `PASS` or `FAIL`. |
| `failReason` | string | The fail reason (omitted on PASS). |

The `tools/progress_report.sh` renderer reads the tail of this file
into `PROGRESS.md`; the M1 milestone closes when a PASS row with an
8-hour `durationSec` lands here.
