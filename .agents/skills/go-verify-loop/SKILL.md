---
name: go-verify-loop
description: >-
  The mandatory verification loop and lint etiquette of the swarm
  repository (Go 1.23 module, strict golangci-lint v2 set). Use before
  declaring any Go change done, whenever a build, test, vet or lint
  failure appears, when adding //nolint directives, or when the user
  asks to check, verify, commit or finish work - even if they only say
  "готово" or "проверь".
---

# Go verification loop (swarm)

Work is not done when it compiles; it is done when this loop is green.
Run it from the repository root, in this order:

```bash
go build ./...            # compiles everything including cmd/
go vet ./...              # cheap static checks
go test ./... -count=1    # the full suite (~40 s, pathfind dominates)
gofmt -l cmd internal     # empty output = formatted
golangci-lint run         # the strict gate; 0 issues required
```

`task check:all` runs lint + test; the individual tasks are `task test`,
`task lint`, `task test:race`. The Windows dev host has `task` 3.53.1
and golangci-lint v2.13.2 installed; `-race` needs cgo with gcc, which
the Windows host lacks - `task test:race` belongs to environments with
cgo (the Linux sandbox, CI).

## Interpreting failures

- Lint findings in generated files (`internal/swarm/npcdata/*.go`) must
  never appear: they carry the canonical `// Code generated ... DO NOT
  EDIT.` marker. If they do, the marker line was broken - it must be a
  single line matching `^// Code generated .* DO NOT EDIT.$` before the
  package clause.
- The lint configuration carries documented exclusions (see the
  comments in `.golangci.yml`): gosec G115 for the wire-parser integer
  conversions, cyclop at 15, funlen 65/45, and test-file relief for
  fixtures. Do not widen them without a reason written next to them.
- A failing test that involves time windows should fail for a reason,
  not for timing: the hunt/state logic uses injected clocks where it
  matters. If your change makes a test flaky, fix the determinism, not
  the tolerance.

## //nolint etiquette

The strict set stays strict; a `nolint` is a debt marker with a name on
it (nolintlint keeps them honest and fails on unused ones).

1. Prefer fixing the code. Extract the constant, split the function,
   add the guard.
2. If the complexity is deliberate (a linear wire-format field
   dispatch, a constructor that initializes every field), mark it:
   ```go
   // Splitting the wire-format dispatch hurts the protocol
   // readability; the parser mirrors the Java field order.
   func ParseNpcInfoPacket(p *NpcInfoPacket, data []byte) error { //nolint:cyclop
   ```
   The reason lives in a regular comment above; the directive itself
   stays short on the function line - a long directive trips `lll`
   (80 columns) and you will chase your own tail.
3. A `nolint` on a function line only covers findings reported on that
   line. `funlen`/`cyclop` report there; `gosec` G602/G703 report on
   the offending expression line.
4. Never stack a "just in case" nolint: nolintlint fails the build on
   unused directives, so stale markers surface on the next run - delete
   them when the underlying code changes.

## exhaustruct without the noise

`exhaustruct` demands every struct field explicit in composite
literals. For zero-value sentinels do not write nested empty literals
(`T{Item: state.InventoryItem{}, ...}` - the nested literals get
flagged too); declare plain zero vars and use them:

```go
var (
	emptyItem  state.InventoryItem
	emptyStats npcdata.GearStats
)

var clearedScoredItem = ScoredItem{
	Item:  emptyItem,
	Stats: emptyStats,
	Score: 0,
	Slot:  slotInvalid,
}
```

`net.Dialer` is excluded from the linter in `.golangci.yml`: listing
its deprecated fields trips staticcheck SA1019.
