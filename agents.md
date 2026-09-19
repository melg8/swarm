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

- UPDATE 2026-09-19 (fresh full study, supersedes the 09-18 note):
  nothing escapes the reaper, no matter how detached. Dead between
  tool calls in every variant: `setsid nohup ... &` with own session
  (PID=PGID=SID, PPID 1), the same plus `env -i`, and a renamed
  binary copy; `unshare --fork --mount-proc` stays "Operation not
  permitted". The death lands at the END of the launching tool call,
  not on a fixed timer - the process serves the whole call (the UI
  harness answered curls and browser probes for minutes) and is gone
  the moment the call returns. The agent tooling itself survives (the
  agent-browser daemon and its Chrome keep living across calls), so
  the reaper tracks the session bookkeeping, not the process tree or
  the session id - no pid trick escapes it.
- The working pattern (verified): run the long operation inside ONE
  tool call. Start the server, wait for the port, run every probe,
  kill the server, print the results - a bash script under
  /home/z/my-project/scripts keeps it reproducible, the Bash tool
  allows 10 minutes per call. For a UI check the agent-browser daemon
  persists between calls (it is whitelisted tooling), only the page
  needs the server alive during the same call.
- A detached server still serves the seconds of its own call - quick
  curl checks fit, anything longer needs the single call pattern.
- A port conflict means the previous instance still runs: check
  `ps -eo pid,cmd | grep CMD` and reuse or kill it before starting a
  twin.

## Repo conventions

- Commits carry the melg8 authorship, all work lands on the branch
  `feature/new-pathfind-alternative`, push significant changes as soon as
  they are ready.
- The Go sources use the space indentation (4 spaces, not the gofmt tabs).
