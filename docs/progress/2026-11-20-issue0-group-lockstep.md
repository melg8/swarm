# The group lockstep and shared recovery round (status: in progress)

Started 2026-11-20, branch main, no board issue yet.

## Goal

Make the C1 party act as one unit instead of three hunters that happen
to share a party window:

1. Cohesive movement in a small formation with per-member offsets; no
   member wanders off or stays alone.
2. One shared target: every member attacks the leader's target at the
   same time instead of picking private fights.
3. First attacker rotation for an all-warrior group: when the member
   that pulls the mob drops below a health line it stops being first,
   a healthier member takes the lead and the old first stands down so
   the damage spreads over the group instead of killing one member.
4. Keep enough mana for at least one instant self heal (no combat cast
   may spend the reserved mana).
5. Rest together: when a member needs recovery the group stops, gathers
   and sits/stands as one.

Constraints: the game server is never patched, the coordination rides
the existing process wide hub (all bots run in one fleet), the code,
comments, docs and commits stay English, and the strict gates stay
green. The live fleet is restarted after the changes land.

Acceptance: `go build ./...`, the affected package tests, `task
lint:new`, `task fmt:check` and `task prepush` pass; the live party
forms, members stay within the formation radius, share the target,
rotate the first attacker as health drops, keep the heal mana and rest
together.

## Context

- The preceding round landed the party packets, the hub, the group
  config and the basic follow/focus behavior; the fragments live under
  `docs/progress/archive/`.
- The live run11 showed the party forming but members still picked
  private targets and wandered between zones, so the next step is a
  strict group state instead of an opportunistic follow.
- The self heal already exists in `hunt/recovery_heal.go`
  (`npcdata.SelfHealSkill`, `SelfHealInitialConsumeOf`); the combat cast
  gate in `hunt/combat_skills.go` currently spends every available mp.
- Rest already exists in `hunt/loop_actions.go` (`rest`,
  `restBeforePick`) but is a private decision; the hub has to make it a
  group decision.

## Progress

### 2026-11-20 - start the round

The goal and the acceptance above are fixed. The implementation follows
in atomic commits: hub state first, then the hunt coordination, then
the mana reserve, then docs and the live restart.

## Status

Implementing the group coordination.

### 2026-11-20 - the hub carries the group state

`MemberState` now publishes vitals, the caster flag and the recovery
wish; a new `Coordination` carries the leader's shared target, the
formation anchor, the first attacker and the rest decision. The hub
stores one command per leader and exposes the stable formation slot of
every account. The hub tests pin the command channel and the slots.
