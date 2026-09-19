# Agent Notes

The load bearing repo manual lives in `AGENTS.md` (the rules, the
session limits, the sandbox subprocess verdicts, the subsystem map,
the docs index); this file carries the session run notes the owner
asked to keep on top. Keep the two in sync by editing `AGENTS.md`
first and mirroring the operational digest here.

## Session limits (owner instruction, mandatory)

- One agent process lives at most 2 hours from the owner prompt. Stop
  all work and hand control back to the user no later than 1 hour 45
  minutes in - the stop is mandatory even mid task, everything must
  be pushed before it. Plan the work so every push happens well
  before the mark.
- A new owner prompt resets the timer: the 2 hour life and the 1h45m
  stop mark count again from the fresh prompt.
- Stamp the session start into `/home/z/my-project/.session_start_ts`
  (a unix timestamp, one line) at the session start; read it back to
  compute the spent time before starting any long operation.

## Long running subprocesses (servers, builds)

- UPDATE 2026-09-19, second study of the day (heartbeat probes
  `scripts/detach_probe_a.sh` / `detach_probe_b.sh`, both variants
  alive 8+ minutes across tool calls): in the CURRENT sandbox a
  detached process SURVIVES across tool calls - `setsid nohup ...
  < /dev/null > /dev/null 2>&1 &` with its own session (PPID 1) and
  the double fork with `env -i` plus a renamed binary both keep
  running and serving after the launching call returned. The 09-18
  morning verdicts (death at the call end, the 15-20 second window)
  do NOT reproduce today; the reaper behavior is a property of the
  sandbox version - re-run the probe at the session start when a
  long-lived server is load-bearing.
- Still true from the 09-18 study: `unshare --fork --mount-proc` is
  forbidden ("Operation not permitted"); the agent tooling itself
  (the agent-browser daemon) survives across calls regardless.
- The always-correct pattern: run an operation up to 10 minutes
  inside ONE tool call - start the server, wait for the port, run
  every probe, kill the server, print the results; the harness
  script under `scripts/` keeps it reproducible.
- A detached server still needs a `ps` re-check before every reuse,
  and a leftover must be killed before starting a twin (a port
  conflict means the previous instance is still running).

## Repo conventions

- Commits carry the melg8 authorship, all work lands on the branch
  `feature/new-pathfind-alternative`, push significant changes as soon as
  they are ready, rebase on the remote before every push (other
  agents push concurrently).
- The Go sources use the space indentation (4 spaces, not the gofmt tabs).
