# The archetype integration branch rides the new main (status: in progress)

Started 2026-09-22, branch feature/archer-archetype-check, issue #21.

## Goal

PR #31 (the archetype integration contract of issue #21) went red
and dirty against the evolved main: the base sat at 23732b7 while
main moved to the fragment-policy era, and the CI verify + coverage
jobs failed. Done means: the branch merge-clean against current
main, the full uncapped lint at zero, the coverage delta gate
inside its budget, all three CI jobs green on the pushed head, the
issue moved back to Ready for review.

## Context

- The four in-review slices (PR #16 config launch, #23 bow gear,
  #15 kiting core, #25 edge cases) compose on this branch; their
  merge-clean proof lives in the issue #21 comments (merge commit
  c79354a). The contract test of the previous round
  (TestArcherArchetypeComposesTheLayers, hunt/archetype_test.go)
  pins the cross-layer seam.
- The CI failures on the old head a335d69: verify failed on four
  lint findings (combat_policy_test.go:36 unparam selfSwingsAt
  targetID always 7; cmd/swarm/main.go:202 exhaustruct missing
  configPath/botPlans/botType; hunt/town.go:2262 cyclop clickWaypoint
  16 > 15 - main's waiver arrives with the merge; hunt/kite.go:244
  cyclop kiteFromTarget 18 > 15), coverage failed on the stale
  baseline of the pre-merge tree.
- The merge conflict was docs/agent_progress.md only - the frozen
  legacy log, resolved per the clash policy: main's version wins,
  this fragment carries the branch context (AGENTS.md "The
  shared-file clash policy").
- The live-stack acceptance and the kiting parameter tuning stay
  with issue #20 - this round is offline repair only.

## Progress

### 2026-09-22 09:50 UTC - the merge of origin/main into the branch

Main from dab5e4b down brought the fragment policy, the refreshed
coverage baseline, the CI trigger split and the town.go complexity
waiver. One conflict (docs/agent_progress.md), resolved to main's
side per the clash policy.

### 2026-09-22 10:05 UTC - the lint findings closed, the baseline refreshed

- cmd/swarm/main.go: the parseFlags literal spells configPath,
  botPlans, botType (the three fields the config launch slice
  added) - exhaustruct clean.
- hunt/combat_policy_test.go: selfSwingsAt drops the fake targetID
  parameter; the staged fight mob id becomes the named
  stagedMobID constant - unparam clean (every scene stages mob 7
  as the fight target, the parameter always received 7).
- hunt/kite.go: kiteFromTarget decomposes - the admission guard
  ladder (bow/target present, movement window, kite pacing, hold
  pacing, fresh-target streak reset, streak limit) moves to
  kiteStepAdmitted; the step decision stays in kiteFromTarget.
  cyclop 18 -> 11/9, behavior identical (pure extraction).
- runs/coverage-latest.txt: the fresh numbers of the merged tree
  (cmd/swarm 11.4 -> 20.3 via the launch config tests, gear 90.3 ->
  90.5, hunt 84.2 -> 84.4) - the delta gate reads the tree's truth.

Verified: go build, go vet, the hunt suite (80.7s green), the
cmd/swarm and gear suites, golangci-lint full tree 0 issues,
task fmt:check clean, task test:cover all deltas inside the 2 pp
budget.

## Status

The fixes are committed locally; the push and the CI watch are the
next step. After green: report in issue #21, move to Ready for
review, remove the claim message.
