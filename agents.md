# Agent Notes

## Session limits (owner instruction, mandatory)

- One agent process lives at most 2 hours from the owner prompt. Stop all work
  and hand control back to the user no later than 1 hour 30 minutes in: the
  last 30 minutes stay unused on purpose. Plan the work so every push happens
  before the 1h30m mark.

## Long running subprocesses (servers, builds)

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
