# Agent Notes

The load bearing repo manual lives in `AGENTS.md` (start at its
"Start here" section: the rules, the session limits, the sandbox
subprocess rules, the documentation map, the skills index); this
file carries the session run notes the owner asked to keep on top.
Keep the two in sync by editing `AGENTS.md` first and mirroring the
operational digest here.

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

- Owner instruction (2026-09-20, mandatory): background processes
  live at most 10 minutes - treat a detached process as dead 10
  minutes after its launch. The reaper behavior changed between the
  09-18 and 09-19 studies (death at the call end vs survival across
  calls), so never rely on a stale verdict: re-check a server with
  `ps -eo pid,ppid,sid,cmd | grep NAME` immediately before every
  reuse, kill the leftover before starting a twin (a port conflict
  means the previous instance is still running), and never hand a
  long task to a background process and walk away.
- The always-correct pattern: run an operation up to 10 minutes
  inside ONE tool call - start the server, wait for the port, run
  every probe, kill the server, print the results; the harness
  script under `scripts/` keeps it reproducible.
- Still true from the studies: `unshare --fork --mount-proc` is
  forbidden ("Operation not permitted"); the agent tooling itself
  (the agent-browser daemon) survives across calls regardless.

## Language (owner instruction, mandatory)

- Reason and answer in English always, whatever language the owner
  prompt arrives in - no other language in the replies, not even
  when quoting the prompt verbatim (translate the quote).
- The code, the comments, the commit messages and every other
  produced text in the repository are English always.

## Repo conventions

- Commits carry the melg8 authorship, all work lands on the branch
  `feature/new-pathfind-alternative`, push significant changes as soon as
  they are ready, rebase on the remote before every push (other
  agents push concurrently).
- The Go sources use the space indentation (4 spaces, not the gofmt tabs).
