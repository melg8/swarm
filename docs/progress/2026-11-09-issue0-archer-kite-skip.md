# Skip the kite against archer enemies (status: in progress)

Started 2026-11-09, branch feature/kite-range-contract, issue #0 (owner
instruction in session; no board issue).

## Goal

The owner instruction: the kite must not engage against archer
enemies. A ranged enemy cannot be outrun: retreating from it only
spends the bow cooldown on walking while the archer keeps shooting.
The standing archer trade is the answer against another archer.

Acceptance criteria:

- The enemy weapon family is visible to the hunt loop: the NpcInfo
  right-hand equipment id (the template rhand item id) is parsed and
  resolved through npcdata.ItemGearStats into the object store.
- A bow-wielding fight target closes every kite layer: no proximity
  step, no shot-paced retreat, no pursuit continuation, no hold.
- A melee fight target keeps the verified round-19 behavior byte for
  byte.
- Unit tests pin both branches (archer target quiet, melee target
  kites) plus the parser and the state accessor.
- Lint and the affected package tests are green.

Constraints:

- temp* accounts only; never touch test1-testN.
- No -abuse flag; the Mobius stack stays running.
- The untracked runs/* evidence files stay uncommitted.
- Committer melg8 <public.melg8@gmail.com>.

## Context

Verified against the official Mobius C1 source (gitlab
MobiusDevelopment/L2J_Mobius, path L2J_Mobius_C1_HarbingersOfWar):

- AbstractNpcInfo.NpcInfo.writeImpl writes the equipment block as
  [rhand: 4][chest: 4][lhand: 4] right after the collision height,
  so the right hand is the FIRST int of the 12-byte group.
- Npc.getRightHandItem returns the template rhand item id; Kendell
  (npc 30218, display 7218) and Starden (npcs 30220/30222) carry
  rhand="276" (Elven Bow), and npcdata.ItemGearStats(276) resolves
  WeaponType "BOW".
- The Kaboo Orc Archer (npc 20469) carries rhand="14" (Bow), and
  npcdata.ItemGearStats(14) resolves the same WeaponType.
- The NpcInfo parser used to skip the whole 20-byte tail after the
  collision radius (collision height + equipment), so the equipment
  was invisible to the tracker.

Design: parse the rhand id, store a BowWielding flag on the hot
object record (read every fight tick by the kite gate), expose it
through state.Bot.ObjectUsesBow, and close the shared kite gates
(kiteLayerGates) when the fight target carries a bow. The
kiteLayerGates seam is shared by the three kite entry points
(proximity step, shot-paced retreat, pursuit continuation), so one
gate closes every layer.

Dead ends already ruled out:

- No npcdata archer flag exists; the generated NPC tables carry
  level, aggression, clan and aggro range only.
- A server-side flag or patch is out of bounds (server integrity
  rules).
- Template-id allowlists would drift from the datapack.

## Progress

### 2026-11-09 - task start

Fragment created after verifying the wire order and the Kendell/
Starden bow item against the Mobius C1 sources. Next: the parser
field, the state plumbing and the kite gate.

### 2026-11-09 - implementation

- `npc_info.go`: the 20-byte tail split into `npcInfoHeightLead` (8),
  the right hand int and `npcInfoEquipmentTail` (8);
  `NpcInfoPacket.RightHandItem` carries the template rhand id.
- `state.NpcInfo.RightHandItem`, `objectHot.BowWielding`,
  `state.Bot.ObjectUsesBow` and the `npcBowWielding` resolver through
  `npcdata.ItemGearStats`/`bowWeaponType`.
- `connection.applyNpcInfo` copies the field into the tracker.
- `hunt.kiteLayerGates` refuses a bow-wielding fight target.
- Tests: parser fixture + assertion, the state accessor test, the
  hunt gate test (proximity and shot-paced subtests) and the fixture
  layout updates of the benchmark/error-path builders.
- `docs/hunting.md`: the enemy weapon gate paragraph.

### 2026-11-09 - pre-existing connection flake

`TestSendPacketAttributesOnlyTheAnsweredRequests` failed on this
Windows host (and on a clean HEAD worktree, 4/5 repeats): the test
stamps `before` at its start and `ApplyOtherRequestSent` stamps the
send inside the same coarse timer tick, so the strict `After(before)`
read the request as outside the window. `OtherRequestBetween` now
uses inclusive bounds (the conservative reading: a request on either
boundary may own the ActionFailed answer), with boundary cases in
`refusal_attribution_test.go` and a `docs/flake_ledger.md` row.

### 2026-11-09 - verification

- `go test ./internal/swarm/state/ ./internal/swarm/packets/from_game_server/ -count=1`:
  green.
- `go test ./internal/swarm/connection/ -count=1`: green;
  `TestSendPacketAttributesOnlyTheAnsweredRequests -count=10`: 10/10.
- `go test ./internal/swarm/hunt/ -count=1`: green (160 s).
- `go build ./...`, `go vet ./...`: green.

## Status

In progress. Next step: format and lint gates, the atomic commits, a
short live temp18 positive control (the melee target keeps kiting),
then archive the fragment.
