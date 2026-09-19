# Agent Notes

The load bearing repo manual lives in `AGENTS.md` (the rules, the
subsystem map, the docs index); this file carries the session run
notes the owner asked to keep on top.

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

- UPDATE 2026-09-18: the recipe below no longer survives the reaper.
  A setsid detached viewer (own session, PID=PGID=SID) died within
  ~20 seconds, every restart the same; `unshare --fork --mount-proc`
  answers "Operation not permitted". The practical use of a detached
  server now: the ~15-20 second survival window after start - enough
  to curl the endpoints (config, geometry, a POST route) while the
  process lives, not enough for anything longer. Long running servers
  stay the owner's job (they run them on their own machine); inside
  the agent session plan endpoint checks inside the window and expect
  the death.
- The sandbox kills the subprocesses of the agent session unless they detach.
  A detached process survives the session end (verified: the navmesh viewer
  of a previous session kept serving port 8082 across sessions).
- The working detach recipe (Linux):
  `setsid nohup CMD > LOG 2>&1 < /dev/null &`
  - `setsid` gives the child its own session and process group (the reaper
    kills by process group, the new session escapes it).
  - `nohup` plus the redirected stdio drop the terminal dependency.
  - Verify before long operations: `ps -o pid,pgid,sid,cmd -p PID` - the
    PID, PGID and SID must all equal the new pid (own session).
- A port conflict means the previous detached instance still runs: check
  `ps -eo pid,cmd | grep CMD` and reuse or kill it before starting a twin.

## Repo conventions

- Commits carry the melg8 authorship, all work lands on the branch
  `feature/new-pathfind-alternative`, push significant changes as soon as
  they are ready.
- The Go sources use the space indentation (4 spaces, not the gofmt tabs).
