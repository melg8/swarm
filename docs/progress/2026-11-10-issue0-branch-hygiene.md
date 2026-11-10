# Branch hygiene: retire the branches main already carries (status: complete)

Started 2026-11-10, branch main, issue0 (owner instruction: hygiene
only, no code).

## Goal

`git branch -a` must stop lying about what is still in flight. Delete
the branches whose work main already carries, and the one whose work
main superseded; keep every branch that still holds unlanded content
or an open review.

## Context

The inventory method: `git cherry origin/main <branch>` compares the
patch-ids, so a rebased, squashed or cherry-picked duplicate reports
as contained; a branch that landed through a cherry-pick with an extra
file in the same commit reports as unique even though its content
landed. The four branches that landed that way (the navmesh trio and
the launch config) were therefore verified by patch-id of the diff
with the bookkeeping paths excluded, and the result matched the
landed commit exactly.

The branch head SHAs below are the deletion record: a deleted branch
stays reachable through `git reflog` and the object store for the
grace period, and any of these can be restored with
`git branch <name> <sha>` while the objects live.

## Progress

### 2026-11-10 - the sweep

Deleted (30 remote branches, plus their local copies) in three groups:

- **landed by cherry-pick, patch-id equal** - `feature/navmesh-size-research`
  (`0287e2e8`), `feature/navmesh-route-corpus` (`d4b969b7`),
  `feature/navmesh-dual-view` (`ddc617ae`), `feature/config-launch`
  (`5dfa90df`, equal once the frozen-log appends are excluded);
- **contained by patch-id, no unique commit** - `feature/ci-flake-fixes`
  (`4c9b7292`), `feature/ci-time-budget` (`7a2b2f27`),
  `feature/contact-touch-no-shrink` (`c52245da`),
  `feature/improved-behaviour` (`3fdd13d1`),
  `feature/acceptance-archer-kite` (`7c283124`),
  `feature/alloc-reduction` (`b1b6c45b`), `feature/archer-archetype-check`
  (`17bb8ba4`), `feature/archer-bow-gear` (`48a58432`),
  `feature/archer-kiting` (`a874d64f`), `feature/click-waypoint-split`
  (`1aff3c87`), `feature/contact-pack-no-cross` (`5e3fc396`),
  `feature/dead-mob-icon-research` (`55fc863d`),
  `feature/dump-buffs-section` (`a866ad14`),
  `feature/guide-dialog-stale-page` (`f2a7aa12`),
  `feature/improved-kite` (`44f7c00c`), `feature/kite-config-knobs`
  (`7d630a12`), `feature/kite-docs` (`30c51bbf`),
  `feature/kite-range-contract` (`52b0a01e`),
  `feature/kite-shot-paced-retreat` (`fe7b7dbb`),
  `feature/kite-walk-reclick-ladder` (`942f82bb`),
  `feature/map-bg-fallback` (`f5e5163a`),
  `feature/navmesh-deterministic-links` (`d58d34fa`),
  `feature/persistent-kill-marks` (`11e71664`),
  `feature/webui-auto-reload` (`dbae87cf`);
- **superseded** - `feature/kiting-edge-cases` (`73dc0e47`): the
  train-direction retreat, the cornered hold and the lane fan it added
  live in `internal/swarm/hunt/kite.go` today, rewritten by the
  November kite rounds; `feature/merge-clash-policy` (`0d6a59f9`): its
  only unique commit flips the status line of an already archived
  fragment.

Kept on purpose:

- `feature/navmesh-merge-tolerance` - the open PR #62 waits on the
  corpus answer contract;
- `feature/abuse-verify` - six commits of live verified work that
  never landed (the 2026-09-26 push failed on a dead token);
- `feature/height-quantization` - the rejected candidate that issue
  #56 cites as its artifact;
- `screenshots/dead-mob-xeyes` - two live evidence images
  (`docs/images` does not carry them) for the closed issue #6.

### 2026-11-10 - the local copies

Deleted twelve local branches whose commits main carries (patch-id
equal, `ahead` counted against `origin/main` at the sweep):
`feature/contact-touch-no-shrink`, `feature/improved-behaviour`,
`feature/improved-kite`, `feature/kite-range-contract`,
`feature/mobius-c1-client`, `feature/new-patfind-alternative`,
`feature/new-pathfind`, `feature/new-pathfind-alternative`,
`feature/proxy-server`, `feature/proxy-server-backup`,
`mobius-c1-client-1`, `mobius-c1-client-2`.

Kept local-only, because they hold commits main does not (the heads
are the record; oldest first):

- `feature/webui_modern` (`d6b75391`, 6 commits, 2026-09-10) - the
  modernization implementation the proposal doc still marks awaiting
  the owner's approval;
- `feature/hunt-spots` (`e4b7df2f`, 1 commit, 2026-09-10) - the
  retired spot-anchored hunting map, superseded by the cells;
- `feature/dead-mob-icon-research` (`2744be68`, 4 commits, 2026-09-21)
  - the dead mob icon iteration;
- `feature/archer-kiting` (`50851c21`, 1 commit, 2026-09-21) - the
  September archer kite step, superseded by the November rounds;
- `feature/abuse-verify` (`598d2a99`, 6 commits) - the unlanded
  verified feature from the kept list above.

## Status

Complete: the remote lists `main` plus the four kept branches
(`abuse-verify`, `height-quantization`, `navmesh-merge-tolerance`,
`screenshots/dead-mob-xeyes`), `origin/dependabot/*` is gone (the
auto-prune on fetch removed the three the closed dependabot PRs left
behind), and the local tree is `main` plus the five branches above.
The four local-only branches with unique commits are the owner's call,
not a silent deletion.
