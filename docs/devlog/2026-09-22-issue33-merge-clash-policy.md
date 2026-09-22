# The merge clash policy round (issue #33)

Date: 2026-09-22. Scope: the agent work protocol, the progress and
devlog file layout, the session bootstrap tool. Issue #33 ("Reduce
merge conflicts and agents clash"), landed through the PR of branch
`feature/merge-clash-policy`.

## Problem statement

Every landed PR forced a rebase on every other open branch. The
owner's example: `agent_progress.md` is a file each agent modifies on
each unit of work, so after any single PR lands the rest must rebase.

## Root cause analysis

The AGENTS.md work protocol mandated a write of ONE shared file
(`docs/agent_progress.md`) with every atomic commit of every task:
all concurrent branches carry edits at the same anchor (the top of
the file, right after the fixed preamble), which is a guaranteed
text conflict for any pair of branches - the clash is by
construction, not by accident.

Measured 2026-09-22 over the 8 then-open branches (all siblings of
merge-base 8647471 or 55f5c82, none stacked):

- `docs/agent_progress.md`: touched by 8/8 branches;
  `git merge-tree` against plain main conflicts on it for 7/8
  (the 8th, navmesh-size-research, instead conflicts on it in the
  sequential simulation after other branches merge).
- `docs/development_log.md`: touched by 5/8, conflicts in the
  sequential simulation twice.
- `docs/agent_progress_archive.md`: touched by 3/8 (the finished
  entry moves).
- `AGENTS.md` and `docs/README.md`: touched by 4/8 - documentation
  map table rows inserted at the same anchor (the four share the
  config-launch doc row; low severity, one-line resolution).
- A sequential-merge simulation of all 8 branches into main
  conflicted at every step that touched the shared logs; only
  feature/alloc-reduction (pure code files) merged clean.

## Fix

The changelog-fragment convention applied to the agent logs:

- `docs/progress/` - one fragment file per task
  (`YYYY-MM-DD-issue<N>-<slug>.md`), written only by the task owner,
  committed and pushed with every atomic commit (the crash-safety
  contract unchanged), moved to `docs/progress/archive/` by the last
  commit of the branch. Two agents on different tasks write
  different files: the merge of both is a set union, not a text
  clash - conflicts become structurally impossible on these paths.
- `docs/devlog/` - one fragment file per development round carrying
  the root-cause record (the reading contract of the old
  development_log.md, index = `ls docs/devlog/`, date-prefixed
  names).
- The legacy files (`agent_progress.md`, `agent_progress_archive.md`,
  `development_log.md`) freeze with archive banners - full history
  preserved, read-only forever.
- AGENTS.md: the work protocol rewritten (per-task fragments, the
  discovery through the board + the issue + the fragment), a new
  "shared-file clash policy" section (never append to a shared
  monolithic log from a feature branch; the transition rule for the
  branches still carrying legacy edits - drop the legacy appends on
  the one-time rebase, carry the context in a fragment; the
  documentation-map keep-both-rows rule).
- `tools/session_start.sh`: the active-task checklist item reads the
  fragment directory (count + newest headline) instead of grepping
  the frozen file.

## Reproduction

- The measurement: for each open branch `git merge-tree --write-tree
  origin/main origin/<branch>` lists the conflicted paths without a
  worktree; the sequential simulation merges the branches one by one
  into a scratch branch off main, records the conflicts, resolves
  with --theirs and continues.
- The regression pin: two dummy branches each adding a DIFFERENT
  fragment file under docs/progress/ must merge clean - trivially
  true by construction (different paths), which is exactly the
  property the policy buys.

## Verification

- `bash tools/session_start.sh` (SKIP_FETCH=1): the new section 5
  prints the fragment count and the newest headline, exit 0.
- The repo gate (build, vet, lint, test, fmt:check) is unaffected:
  the round touches only markdown and one bash tool; CI runs the
  full gate on the PR push.
- The whitespace policy (spaces only, LF, final newline) followed in
  every new file.

## Follow ups

- The 8 open branches each need their one-time rebase; on it they
  drop their legacy `agent_progress.md` / `development_log.md`
  appends and (optionally) re-home the task context into their own
  fragment.
- The eventual mechanical split of the frozen development_log.md
  into per-round devlog fragments (readability nicety, no policy
  pressure) can be a separate backlog round if wanted.
