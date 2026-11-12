# P03 residue: the bench playground and the offset duplicate (status: in progress)

Started 2026-11-12, branch main, issue0 (owner instruction: continue
the refactor after the file split rounds and the packet router).

## Goal

The P03 prompt of `docs/quality_review_and_agent_prompts.md` listed
five items; the three code deletions (the unused connector, the
big-endian crypt stack, the dead `cellLayers`) and the v2 lint
migration landed in earlier rounds. The residue is closed here:

- the duplicated packet benchmark generation is reconciled -
  `init_bench_v2_test.go` is a Rust-mirror playground (`unsafe`
  pointer casts, its own `PackedInitHeader`/`InitPacketView`, no SPDX
  header, nothing it defines is used by the real parser or by any
  other file), while `init_bench_test.go` benchmarks the production
  `InitPacket`; the playground goes, the junk `BenchmarkPlusPlus`
  (a `2 + 2` loop) goes with it, the three real benchmarks stay;
- `npcDisplayOffset = 1000000` used to be declared three times
  (`hunt/town_merchants.go`, `state/bot_info.go` and the unexported
  `npcdata.npcTemplateOffset`); one exported definition in `npcdata`
  replaces all three, because the package already owns the display id
  dictionaries and the offset is their addressing rule.

## Constraints

- No behavior change: the same three benchmarks, the same constant
  value, the same name resolution.
- The packets package keeps its coverage: the parser and the
  serializer benchmarks stay (`ParseInitPacket`, `ToBytes`,
  `WriteTo`).
- `task fmt` (gofmt-spaces) after the edits, then the full gate.

## Context

Measured 2026-11-12 before the change:

- `init_bench_v2_test.go` is 142 lines, has no SPDX header and
  exercises only its own helpers (`parseInitPacketPackedInto`,
  `parseInitPacketPackedWithAlloc`, `TestStructSize`); a grep over the
  tree finds no other file referencing any of them;
- `init_bench_test.go` is 106 lines, of which `BenchmarkPlusPlus` is
  a two line arithmetic loop that measures nothing;
- the offset constant is read by the hunt trip planning (the merchant,
  quest and guide template lists), the delevel guard lookup and the
  state name resolution, so the exported home removes three copies
  and one stale name (`npcTemplateOffset`).

## Progress

### 2026-11-12 - the playground removal
`init_bench_v2_test.go` deleted, `BenchmarkPlusPlus` removed from
`init_bench_test.go`. The remaining benchmarks run:
`BenchmarkInitPacketParsing` 0 B/op, `BenchmarkInitPacket_WriteTo`
0 B/op, `BenchmarkInitPacket_ToBytes` 4 allocs.

## Status

Both slices land; the full gate and the live run are next.

### 2026-11-12 - the offset unification
`npcdata.NPCDisplayOffset` is the single definition: the package's own
resolvers, the hunt trip planning (the merchant, quest and guide
template lists of `delevel.go`, `guide_buffs.go`, `learning.go`,
`quest_chains.go`, `quest_trip.go`, `shopping.go`,
`town_merchants.go`) and the state name resolution of `bot_info.go`
read it. The two duplicate declarations and the stale
`npcTemplateOffset` name are gone.

Verified: `go build ./...`, `go vet` on the four touched packages,
`golangci-lint run` 0 issues on them, `go test` of `npcdata` (0.4 s),
`state` (1.2 s) and `hunt` (97 s) green, the three `InitPacket`
benchmarks run.
