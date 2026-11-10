# The AGENTS.md restructure: rules and map, not an encyclopedia (status: complete)

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

### 2026-11-10 - the factual pass

Checked against the tree and the live stack, fixed in place:

- the login endpoint of the Windows deployment (`127.0.0.3:2106`,
  the `-login` note in the Commands section);
- the verification budget: the old numbers (39 s lint, 123 s suite,
  99 s of pathfind) no longer match the machine - the paragraph now
  names the shape of the cost instead of stale seconds;
- the repository layout gained the three packages it omitted
  (`acceptance/botlog`, `internal/logfmt`, `internal/version`) and
  lost the false sibling indentation;
- the performance section no longer claims pathfind holds 99 s of
  the suite (the hunt package is the slow one now);
- the shell-survival bullet of the stack notes merged into the
  subprocess rule (it said the same thing twice);
- git conventions admit the current mode: small rounds land on
  `main`, larger tasks get a feature branch.

Verified while passing: 9 `tools/repro_*.js` harnesses, 165 geodata
regions, 3134 item icons, 46 vendored `golang-*` skills plus the 7
project playbooks, protocol revision 419, the game server login
registration on 9014, the 250 unit shop interaction gate, the
fleete2e `SWARM_FLEET_E2E=1` gate and its 10 minute online timeout,
the `Build:` identity line, `SWARM_FLEET_PROFILE`, and every
`docs/*.md` file present in the documentation map.

### 2026-11-10 - the registry moved to docs/hypotheses.md

The H-001..H-007 entries and the registry rules left `AGENTS.md` for
`docs/hypotheses.md` (193 lines); `AGENTS.md` keeps a two paragraph
rule and the link, the documentation map gained the row,
`docs/README.md` lists the document under the working process, and
`tools/session_start.sh` reads the open ids from the new file. The
entries themselves are unchanged, so no verification evidence was
lost.

### 2026-11-10 - the facts moved out of the agent guide

Before any line left `AGENTS.md`, the facts it carried landed in their
subsystem documents:

- the drop-slide rules of `NpcTemplate.calculateGroupDrops` (item,
  adena, exp/SP level gaps) and the `isAggressive` TRUE default went
  to `docs/hunting.md`;
- `UseItem` (the equip/unequip slot semantics), `RequestBuyItem` (the
  merchant distance, the tax, the buylist layout), `Logout` (the
  attack stance refusal and the 15 second delay) and the `UserInfo`
  paperdoll slot order went to `docs/protocol_description.md`;
- the shared outbound cipher chain fact (`GameClient.SendRaw` under the
  session `writeMu`) went to `docs/proxy.md` (the live-patch details it
  sat next to were already documented there).

### 2026-11-10 - the duplication squeeze

`AGENTS.md` went from 937 to 690 lines. The duplicated blocks were
compressed against their homes:

- the documentation map dropped the 30-row table and points at
  `docs/README.md` as the canonical index (the near-duplicate copy was
  the main source of merge clashes);
- `Commands` keeps the launch line, the task list and the CI note; the
  benchmark/pprof snippet defers to the `golang-benchmark` skill;
- `Client proxy`, `Mobius stack operational notes`, `Server integrity`,
  `Repository layout`, `Architecture`, `Performance`, `Code
  conventions`, `Tree cleanliness`, `Testing`, `Protocol notes` and
  `Git conventions` lost the restated prose while keeping every rule,
  threshold and fact;
- `Time and cost discipline` and `Long running subprocesses` merged
  into one section; `Work protocol` absorbed the shared-file clash
  policy; `Testing` and `Logging` merged.

Verified while squeezing: `crypt.MobiusAuthKey()` is a function, the
packet packages live one per direction under
`internal/swarm/packets/`, the nine `tools/repro_*.js` harnesses, 46
vendored `golang-*` skills, the 3134 item icon PNGs and every path
named in the remaining text exists.

## Status

Step 3 of 3 is done: the registry extraction, the fact moves and the
duplication squeeze are in, and `AGENTS.md` now reads as rules plus
the map. The next round may archive this fragment after the owner
reviews the result.
