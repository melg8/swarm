# The party coordination and potion economy round (status: in progress)

Started 2026-11-20, branch main, issue #0 (no board issue yet).

## Goal

Cut the emergency logouts of the night run by two mechanisms the run7
evidence named as missing:

1. The healing potion economy: every bot keeps a stock of healing
   potions and restocks it on a town trip. Run7 drank 60 potions in 51
   hours and bought zero, because no plan line ever ordered one.
2. The group play: the accounts `test1`-`test3` form a party (leader
   `test1`), move together, focus the leader's target and restock as a
   group. Run7 logged out 1,355 times, about 600 of them from two to
   five mobs piling on one bot and about 600 from 55-70 percent HP
   under attack.

Constraints: the game server is never patched; the party forms through
the C1 client packets; the config is a separate JSON file; the code,
comments, docs and commits stay English.

Acceptance: `go build ./...`, the affected package tests, `task
lint:new` and `task fmt:check` pass; the live fleet starts with the
party config, the three characters show in one party and the potion
stock appears in the inventory and the shopping queue.

## Context

- The run7 record: `runs/night-run7.log` and
  `logs/session-20261116-025103-33464.1.jsonl`; 50.94 h, 13,040
  kills, 222 deaths, 1,355 emergency logouts, 56 stalls.
- The logout causes: 351 + 183 + 45 + 11 pile ups of two to five
  mobs, and HP 44-60 percent under attack lines (the flee budget
  expires below the 60 percent re-engage line).
- The potion record: `Hunt: HP ..% in the fight, drinking a healing
  potion` fires 60 times, `Hunt: shop: buying` only ever names Wooden
  Arrows (38 buys), and `sellableJunk` would sell potions back.
- The potions: 1060 Lesser Healing Potion (reuse 10 s, heal over time
  8 x 3) sold by the elven Herbiel (list 3015000) and the Dion Lara
  (list 3006300); 1061 Healing Potion (heal over time 24 x 3) sold by
  the Dion Lara only.
- The party wire facts: client `RequestJoinParty` 0x29
  [objectId:4][distributionType:4], `RequestAnswerJoinParty` 0x2A
  [response:4]; server `JoinParty` 0x3C [response:4], `AskJoinParty`
  0x4B [requestorObjectId:4][distributionType:4],
  `PartySmallWindowAll` 0x63, `Add` 0x64, `DeleteAll` 0x65,
  `Delete` 0x66, `Update` 0x67, `PartyMemberPosition` 0xC0. The
  layouts come from the Mobius C1 server packets and are recorded in
  `docs/protocol_description.md`.
- The fleet runs all three bots in one process (`-bots 3`), so the
  party coordination rides an in-process hub keyed by account; the
  server party itself is formed with the real packets.

## Progress

### 2026-11-20 - start the round

The run7 analysis is complete (the totals and causes above). The
implementation follows in atomic commits: the potion economy first,
then the party packets and state, then the hub and the hunt behavior,
then the live restart.

## Status

Implementing the potion economy; the party layer follows.
