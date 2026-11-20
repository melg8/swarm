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

All code and docs land; the live fleet restart is pending.

### 2026-11-20 - the potion wallet fallback

The restock line orders the potions strongest first: a Dion purse that
cannot cover the 1061 now buys the 1060 of the same Lara list instead
of skipping the line. The wallet test and the region test cover the
fallback; the rest of the potion economy stays in the preceding commit.

### 2026-11-20 - the party packets

The C1 invitation, answer, window and placement packets are parsed and
serialized: RequestJoinParty 0x29 and RequestAnswerJoinParty 0x2A on
the client side, AskJoinParty 0x4B, the window 0x63-0x67 and
PartyMemberPosition 0xC0 on the server side. The window entries trail
the class with two ints, the placement entries with three, as the
Mobius `writeImpl` classes define. The protocol notes record the layouts
and the 0x3C collision of JoinParty with AutoAttackStop. Unit tests
pin every layout (including the implausible-count guard).

### 2026-11-20 - the party mirror in the tracker

`state.Bot` now owns the party view: the window replacement, the joined
member, the vitals refresh, the removal, the reset on a new session and
the pending invitation. The placement packet also seeds world records
so a follower reads the leader position when its own client lost sight
of it. The state tests pin the whole packet walk and the reset.

### 2026-11-20 - the party packets in the game session

The session parses the party packets into reusable scratch structs and
mirrors them into the tracker, and it exposes the JoinParty and
AnswerJoinParty senders. The dispatch routes the window and the ask;
the JoinParty echo stays unrouted because it shares 0x3C with
AutoAttackStop in C1 - the acceptance arrives with PartySmallWindowAdd.
A dispatch test drives the packets through the table.

### 2026-11-20 - the group config and the fleet hub

The separate `configs/parties.json` defines the groups (test1 leads
test1-test3). The `party` package parses and validates it (leader listed,
no shared account, at most nine members) and hosts the process wide hub
that shares the live placement and the target between the sessions of
one fleet. Tests pin the parse, the validation and the hub contract.

### 2026-11-20 - walk together and focus fire

The hunt loop publishes its live state every tick, the leader invites
visible members in range with a per-member pacing, and the follower
accepts only the configured leader. A follower that falls behind walks
after the leader with a 450/280 hysteresis and adopts the leader's
target within 1200 units so the group focus fires one mob. An active
fight or an incoming blow holds the follow walk. The hunt docs record
the behavior; the fake games and the hunt tests cover the roles.

### 2026-11-20 - wire the party config into the launch

The `-party-config` flag (default `configs/parties.json`) loads the hub
once per process; both the fleet and the single-bot path share it and
each session attaches it to its hunt loop. The strict lint gate gets
the documented zero-value patterns for the party mirror structs. All
build, vet, package test, `lint:new` and `fmt:check` gates pass; the
live fleet restart is the remaining acceptance step.

### 2026-11-20 - run the group on every hunt decision

The live restart showed the group stuck: the members hunt in different
zones and spend long stretches in town trips, while the party hook only
ran inside the engage phase, so the hub never saw a fresh placement and
the leader never saw a member inside the invite range. The hook now
runs from `tickContext` on every decision: the hub stays fresh, the
follower walks after its leader from any phase and the leader invites
as soon as the member comes into view. A test pins the town walk case.

### 2026-11-20 - let the potion errand finish

The live run formed the party (test3, then test2 joined) and converged
the group, but the follow walk was hijacking the town trips: a follower
with a potion plan never reached Herbiel. A town trip now owns the
movement until it ends; the follower rejoins the leader afterwards.
The second live check confirms the party window and the 1060 restock.
