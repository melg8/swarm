# The task progress fragments

Crash-safe task tracking, one file per task: the current task, its
full context and the per-commit progress entries live in a fragment
file here (see the "Work protocol" section in AGENTS.md). Each
fragment belongs to exactly one task and only the task owner writes
it, so two concurrent agents never share a write anchor and never
force a rebase on each other (the shared-file clash policy in
AGENTS.md; the legacy single-file logs are frozen archives).

## Naming

    YYYY-MM-DD-issue<N>-<slug>.md

- the date is the task start day (the filename sorts chronologically,
  `ls docs/progress/` is the index);
- `issue<N>` is the GitHub project board issue the task answers
  (`issue0` when there is none yet);
- the slug is two to four dash-separated words of the task headline.

## Fragment structure

    # <task headline> (status: <in progress | in review | complete>)

    Started YYYY-MM-DD, branch feature/<name>, issue #<N>.

    ## Goal
    what done means, the acceptance criteria, the constraints

    ## Context
    everything a fresh session needs to resume without rediscovery:
    the links (issue, PR, related docs), the verified facts, the
    dead ends already ruled out

    ## Progress
    ### YYYY-MM-DD HH:MM UTC - <the atomic commit subject>
    what changed, what was measured, what is next

    ## Status
    the current state machine position and the next step

## Rules

- One task, one fragment: never edit another task's fragment, never
  append to a landed fragment - a follow-up round (review feedback,
  a reopened task) writes its own fragment that links the previous
  one.
- Commit and push the fragment together with every atomic commit of
  the task (the crash-safety contract: a session can die at any
  moment and the next session resumes from the pushed fragment, the
  issue comments and the branch).
- The fragment is the working state, the GitHub issue is the
  reviewer-facing channel: findings, decisions and anything the
  owner must read go to the issue (and the PR review), the fragment
  keeps the resume state machine.
- When the task completes, move the fragment to
  `docs/progress/archive/` in the last commit of the branch, so the
  merged main only ever carries finished fragments in the archive and
  `docs/progress/` reads as the active set.
- The frozen legacy logs (`docs/agent_progress.md`,
  `docs/agent_progress_archive.md`) stay readable as the history
  before the fragment split - never append to them.
