# The development log fragments

The permanent record of the development rounds, one file per round:
the root cause analyses, the reproduction steps, the fix and its
verification live in a fragment file here. Each round writes its own
fragment, so concurrent agents never share a write anchor (the
shared-file clash policy in AGENTS.md; `docs/development_log.md` is
the frozen pre-split archive of the rounds before 2026-09-22).

## Naming

    YYYY-MM-DD-issue<N>-<slug>.md

- the date is the round's landing day, `issue<N>` the board issue it
  answers (`issue0` when there is none);
- the slug names the round, two to four dash-separated words;
- `ls docs/devlog/` (date-prefixed names) is the whole index, newest
  last.

## Fragment structure

Follow the entry format the frozen log used (see its header):

- date, scope, the issue and the PR it landed through;
- problem statement;
- root cause analysis (with references into the sources);
- reproduction: how to trigger, how to detect, expected output;
- fix, verification, follow ups.

## Rules

- One round, one fragment: written once when the round completes and
  travels inside its PR - never edited after landing, a follow-up
  round writes its own fragment that links the previous one.
- Read the matching rounds before reworking movement rendering or
  the hunt behavior (the same reading contract the frozen log had;
  grep the directory by slug or area keyword).
- The frozen `docs/development_log.md` keeps the rounds up to
  2026-09-21 with its `## Round N` navigation - never append to it.
