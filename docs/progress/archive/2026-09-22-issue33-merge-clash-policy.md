# The merge clash policy round - per-task fragments retire the shared hot files (status: in review)

Started 2026-09-22, branch feature/merge-clash-policy, issue #33
("Reduce merge conflicts and agents clash").

## Goal

Every open PR currently conflicted with main on the same file:
measured 2026-09-22, 7 of the 8 open branches fail a plain
`git merge-tree` against main on `docs/agent_progress.md` alone, and
a sequential-merge simulation of all 8 conflicts on it at every step
plus `docs/development_log.md` twice. The AGENTS.md work protocol
mandated updating that single shared file with every atomic commit,
so any two concurrent agents are guaranteed to clash and every landed
PR forces a rebase on every other branch.

Done means: the policy stops mandating writes to shared hot files,
the crash-safe handover contract survives unchanged (pushed,
resumable task state), the permanent root-cause record survives
(per-round devlog fragments), and the transition is documented for
the branches still carrying legacy edits.

## Context

- The clash inventory (measured, the method is in the devlog
  fragment of this round): `docs/agent_progress.md` 8/8 branches
  (protocol-mandated per-commit appends at the same anchor),
  `docs/development_log.md` 5/8 (per-round appends),
  `docs/agent_progress_archive.md` 3/8 (finished-entry moves),
  `AGENTS.md` + `docs/README.md` 4/8 (documentation map row
  insertions at the same table anchor, low severity).
- Root cause: central append-only files with a shared write anchor.
  Two agents appending different entries to the same region of the
  same file is a conflict by construction; the frequency comes from
  the protocol making the writes mandatory per commit.
- The fix pattern: fragment directories (one file per task / per
  round, the changelog-fragment convention) - different agents write
  different files, merges become set unions, conflicts become
  structurally impossible for these paths.
- The owner's board workflow (issues + claim comments + branch per
  task) already provides the discovery layer the old single file
  tried to provide; the fragment carries the per-task resume state.
- Constraint: the 8 already-open branches carry legacy
  agent_progress.md edits - they still need their one-time rebase;
  the policy must not add new conflict surface for future rounds.

## Progress

### 2026-09-22 05:20 UTC - docs: the fragment conventions

The two convention readers land: docs/progress/README.md (naming,
fragment structure, ownership and archive rules) and
docs/devlog/README.md (the per-round root-cause record), plus this
task's own fragment started in the new format.

### 2026-09-22 05:30 UTC - docs: the AGENTS.md work protocol rewrite

The work protocol now mandates the per-task fragment (write it with
every atomic commit, archive it in the last commit of the branch),
the task discovery flows through the board + the issue + the
fragment, and the new "shared-file clash policy" section records the
rule (never append to a shared monolithic log from a feature branch),
the measured evidence, the transition rule for legacy-carrying
branches and the keep-both-rows rule for the documentation maps.
docs/README.md rows updated the same way.

### 2026-09-22 05:35 UTC - docs: the legacy logs freeze

agent_progress.md, agent_progress_archive.md and development_log.md
carry the frozen-archive banners pointing at the fragment
directories; history stays readable, appends stop forever.

### 2026-09-22 05:40 UTC - tools: session_start reads the fragments

The bootstrap checklist item 5 lists the active fragments (count +
newest headline) from docs/progress/ instead of grepping the frozen
file; verified locally (exit 0, the fragment found).

### 2026-09-22 05:45 UTC - docs: the devlog record of the round

docs/devlog/2026-09-22-issue33-merge-clash-policy.md carries the
root cause analysis (the measurement method, the numbers, the fix,
the verification, the follow ups).

## Status

In review: the PR carries the five commits; the issue received the
research summary and the transition instructions for the open
branches. The fragment archives itself in this last commit per the
policy it introduces.
