# The AGENTS.md restructure: rules and map, not an encyclopedia (status: in progress)

Started 2026-11-10, branch main, no issue (owner conversation).

## Goal

`AGENTS.md` is the first file every agent reads, but it grew to 1270
lines / 73 KB and mostly duplicates `docs/`. Bring it back to "rules
plus the map": roughly 400-450 lines, with the depth staying in the
documents the map points at.

Acceptance criteria:

- no fact is lost: whatever leaves `AGENTS.md` lands in the matching
  `docs/` file first;
- the hypothesis registry moves to its own document and the session
  bootstrap keeps finding it;
- every remaining claim is verified against the code, the CI or the
  live stack - the stale sandbox-era lines go;
- `task fmt:check` stays green (markdown is under the whitespace
  gate) and every step lands as its own atomic commit.

Constraints: no history rewrite, no force push; the owner reviews the
scope before the deletion pass.

## Context

- The retired z.ai sandbox left rules that no longer describe the
  runtime: the 2 hour session life, the 1h45m stop mark, the 6 minute
  process cap, the 10 minute background reaper, the 10 GB root disk
  note and the `.session_start_ts` stamp. The harness is now the
  owner's local opencode session, so the time budget is a cost
  discipline, not a kill timer.
- The owner confirmed the drop of the wall-clock limit and asked to
  keep the "do not spend time on redundant long checks" spirit.
- The hypothesis registry (H-001..H-007, ~186 lines) has its own life
  cycle and belongs in `docs/hypotheses.md`; only the rule and the
  link stay in `AGENTS.md`.
- The biggest duplication blocks and their homes:
  `Protocol notes` -> `docs/protocol_description.md`;
  `Repository layout` -> the tree itself;
  `Commands` -> `Taskfile.yml`;
  `Mobius stack operational notes` -> `docs/deployment.md` and the
  `mobius-stack` skill;
  `Client proxy` -> `docs/proxy.md`;
  `Performance` -> the `performance` skill;
  `Code conventions` -> `.golangci.yml`.
- Already corrected in the preceding rounds: Go 1.27.2 and the
  toolchain pin, golangci-lint 2.15.0, the active CI, the
  `127.0.0.3:2106` login endpoint of the Windows stack.

## Progress

### 2026-11-10 - the sandbox-era rules replaced

`Session limits` became `Time and cost discipline`, the background
process block lost the reaper story, the `Measurements` section merged
into the cost discipline (it said the same thing twice), and
`tools/session_start.sh` lost the timer stamp (the numbering now runs
1..4 and the script prints the open hypothesis ids without the
trailing colon).

### 2026-11-10 - the factual pass is next

Every remaining claim gets checked against the code, the CI or the
live stack; the stale ones are fixed in place before anything is
deleted.

## Status

Step 1 of 3 (the sandbox cleanup) is committed. Step 2 is the factual
pass, step 3 the registry extraction plus the duplication squeeze.
