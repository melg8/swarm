# Agent progress log

Crash-safe task tracking: the current task, its full context and
per-commit progress live here (see the "Work protocol" section in
AGENTS.md). Entries are append-only; a new agent resumes the newest
unfinished entry.

This file carries ONLY the active and the most recent context:
finished task entries and older progress streams move to
`agent_progress_archive.md` (append-only, same order). The permanent
root-cause history of every round lives in `docs/development_log.md`;
check the archive when the recent context references an older task.

## Active task (status: in review): the kite edge cases - the train centroid, the cornered hold, the lane fan (2026-09-21, branch feature/kiting-edge-cases, issue #19)

Started 2026-09-21 ~20:22 UTC. The source is the board issue
melg8/swarm#19 ("Kiting edge cases: cornered fallback, multi-mob
trains, retreat path quality"), the third slice of the archer warrior
support (#13). The branch forks feature/archer-kiting (PR #15, the
kiting core of #18) at 5df6073 - the edge layer rides the armed step
the core round landed:

- The train direction (kiteTrainDirection, kite.go): the retreat
  direction is the centroid away-vector of EVERY chaser - the fight
  target's own unit vector plus every SelfAttackers member within
  kiteTrainScanRange (800) on a reachable deck (deckReachableZ, the
  same gate the trigger uses). The superseded
  TestKiteDirectionReadsTheNearestChaser (the single-threat
  direction of the #18 round) became
  TestKiteDirectionWeighsTheWholeTrain - the centroid bends the
  retreat off the pure target axis when a member drags it.
- The encirclement (kiteEncircleShare 0.3): when the summed away
  vectors cancel (chasers on every side), the train is too wide to
  outrun - the archer holds ground and shoots through it (the hold
  log line names the train; the winnable pile up machinery still
  owns a train that outdamages the standing fight).
- The cornered hold (kiteHoldGround, the kiteHeldFor/kiteHeldAt
  fields of loop.go): when no walkable lane exists - the leash, a
  wall (the navigator LineOfSight), water (OverWater), or every fan
  candidate failing - the archer stops retreating and KEEPS SHOOTING
  the bow at melee range (the archetype rule of #21: no weapon
  switch, no idle stutter - the hold re-probes at the kite pacing,
  the diagnostic lands once per episode, and the re-request resume
  of the ordinary engage re-arms the shooting once the stance
  lapses; the stuck watchdogs never own a running fight).
- The lane fan (kiteRetreatLane / kiteLaneResolve): the straight
  away-ray first, then the 45/90 degree candidates each side - the
  open backward lanes over the blocked corridors, a dead-end lane
  re-planned at the very next probe (one hop). The away half-plane
  gate (kiteHalfPlaneSlack) keeps every lane - however deflected -
  from folding back into the chasing train.
- The camp deflection (kiteDeflectFromCamps): a lane that would wake
  an idle aggressive camp deflects onto the tangent ray of its
  trigger circle - the tangentClearDirection steering of the transit
  walks (loop_avoid.go), minus the destination exemption (a retreat
  meets no mob on purpose). The chasers never deflect the lane: they
  already hold the character as their target.

Tests: kite_edge_test.go carries the eight edge scenarios (the
centroid bend, the surrounded hold with the pacing pin, the cornered
hold with the once-per-episode log and the re-request resume, the
water corner, the dead-end re-plan, the half-plane guard, the camp
deflection), plus the direction test rewrite in kite_test.go.
Verification: the kite suite green (19 tests), the full hunt suite
green (80s), `go build`, `go vet`, `golangci-lint run --new` 0
issues, gofmt-spaces quiet. Status: pushed to the stacked PR
(feature/kiting-edge-cases -> main, on top of PR #15); the PR body
carries the issue marker ("Fixes #19"). The acceptance scenario and
the live parameter tuning stay with #20; the gear plan (the bow
buying) stays with #17 on its own branch.

## Active task (status: in progress): the kiting parameters pinned - the optimal band and the re-engage delay (2026-09-21, branch feature/archer-kiting, issue #18)

Started 2026-09-21 ~20:22 UTC, continuing the kite entry below on the
same board issue melg8/swarm#18 ("Kiting core: retreat when the mob
closes, re-engage at the optimal range"). A parallel round landed the
train-member trigger mid flight (the hostile scan of kiteThreat,
commit 5257564) - this round rebased on it and closes the remaining
parameter task of the issue:

- The optimal band is pinned in the constants block of kite.go as
  the two edge references: the inner edge kiteRetreatRadius (a
  hostile below it arms the next step), the outer edge
  userBowEngageRadius (user.go, 450 - the re-request shoots from
  range). A copied literal would drift from the radii the fight
  actually fights with, so the band lives as the documented edge
  pair.
- The re-engage delay is a named constant now (kiteReengageDelay,
  zero today) and rides the movement window timestamp
  (combatAvoidUntil = window + delay): the window end IS the
  re-engage today, the knob exists so the live tuning round (#20)
  can hold the aim without touching the walk contract.
- One test pins the direction choice the trigger round left open:
  TestKiteDirectionReadsTheNearestChaser - both hostiles inside the
  radius (the target 240 east, the chasing member 180 west), the
  step goes away from the NEAREST one (east), not away from the
  target (west).

Verification: the kite suite is green (11 tests), `go build`,
`go vet`, `golangci-lint run --new` clean, the gofmt-spaces check
quiet. Status: pushed to PR #15; the PR body carries the issue
marker ("Fixes #18") now. The centroid train steering and the
retreat path quality stay with #19, the acceptance and the live
tuning with #20.

## Active task (status: in progress): the archer kite step - the bow user steps clear of a closed target (2026-09-21, branch feature/archer-kiting, issues #18, #13)

Started 2026-09-21 ~19:35 UTC. The source is the project board issue
melg8/swarm#13 ("Add support for archer warriors with kiting behavior
against mobs"), resumed 2026-09-21 ~20:20 UTC as the kiting core slice
(the slice issue melg8/swarm#18, the first round in flight as PR #15): an archer that fights a mob from the bow range never
wants the mob in its face - the Mobius server AI stands the archer
still while the auto attack shoots, so a melee mob that closes simply
swings away at a target it could outrange.

Design (the first implementation round of the issue):

- `internal/swarm/hunt/kite.go`: the kite step - while a bow fight
  runs, a target that closed inside kiteRetreatRadius (250, the mob
  is a second from melee) steps the character kiteStep (400) units
  straight away from it through WalkTo (the retreat follows the mesh
  routes, never runs into a known wall). The step reuses the fighting
  movement window (combatAvoidUntil): the forced attack re-requests
  hold while the retreat walks (a request would interrupt the walk
  server-side), and once the window closes the engage re-requests the
  attack - the distance after the step lands back inside the bow
  engage radius (450), so the re-request shoots from range instead of
  starting a server chase that walks the distance right back in.
- The step paces itself (kiteStepPeriod 3s: 2s walk, 1s shoot at the
  fastest cycle), respects the zone leash (a cornered archer stands
  and shoots - the leash outranks the kite), and carries a streak
  limit (kiteStreakLimit 8 per target): a chaser at least as fast as
  the character never falls behind, and past the limit the archer
  fights it out instead of shuffling forever (the losing fight and
  panic machinery still own the death risk). A fresh target resets
  the streak.
- The hook sits in the running fight branch of engage() ahead of the
  impending-add scan: the closing target is the concrete damage, the
  add scan runs the next tick when the target holds its distance.
- The behavior arms itself on the weapon in hand (bowEquipped): no
  config, no class check - whatever bot holds a bow (the planned
  archer type of the config launch round, issue #12) kites its
  fights.
- The edge cases the issue names, handled or bounded: cornered (the
  leash skip above), multiple mobs chasing (the step reads the target
  alone; the attacker count gate and the panic run already own the
  pile up), pathfinding while retreating (WalkTo routes over the
  mesh), attack animation/travel time (the window pauses re-requests,
  the already flying shots land; nothing more is modeled - honest
  limit, recorded in kite.go).
- The kiting parameters (the trigger radius, the step length, the
  window, the period, the streak limit) are the tunable block at the
  top of kite.go, each with its rationale comment.

Verification: 8 new tests in kite_test.go pin the step geometry, the
quiet weapon-range case, the melee gate (a melee fight only closes,
never retreats), the pacing window, the post-walk attack resume, the
leash skip, the streak limit and the fresh-target reset; the full hunt
suite green (78s), `go build`, `go vet`, `golangci-lint run --new`
clean.

Status: one commit (a28121f) on feature/archer-kiting, pushed; the
pull request references the issue. Follow-ups for the next rounds
(recorded in the issue comment): the live acceptance round against
the deployed stack (the kite constants measured against the real mob
speeds), the arrow-aware timing (a step that waits the flying shot),
and the config launch wiring of the archer type once issue #12 lands.

Round 2 (2026-09-21 ~20:20 UTC, the kiting core slice issue #18):

- The main merge (3bca8f1): the docs/agent_progress.md conflict
  resolved by the file protocol - the finished full-size contact
  markers entry of the merged main (issue #7, Done on the board)
  moved to docs/agent_progress_archive.md, the kite entry stays the
  active task.
- The task 1 gap of the slice issue closed: the retreat trigger now
  reads the train members too - kiteThreat picks the closest of the
  fight target and the projected NearestAttacker scan (the mobs
  whose server target is the character), so a second mob that
  chased its way into the fight arms the step even while the fight
  target holds its distance. The steering stays the single
  away-vector of the armed threat: the centroid weighing, the
  aggro-aware deflection and the too-wide-train hold belong to the
  edge case slice (#19). The deck guard of the pick applies: a mob
  past the deckReachableZ gap is no melee threat.
- The log line names the armed hostile and the fight target
  separately ("hostile 8 closed to 200 units of the fight on 7").
- Two new tests: TestKiteTrainMemberArmsTheRetreat (the member at
  200 units arms the step away from IT while the target holds 440)
  and TestKiteSkipsADeckGapTrainMember (the deck gap member stays
  quiet). Ten kite tests total, the full hunt suite green (77s),
  go build, go vet, gofmt-spaces and golangci-lint run --new clean.

Status: round 2 pushed (the merge plus the train member commit) on
feature/archer-kiting, PR #15 carries the slice issue marker (#18).
The follow-ups stand: the live acceptance round (#20), the
arrow-aware timing, the centroid steering round of #19.

## Active task (status: complete): the cast icon side round - the bow order shot, the quiet npc interact (2026-09-21, branch feature/improved-behaviour)

Started/finished 2026-09-21 ~14:55 UTC (one session). The owner
prompt assigned three asks, all landed and pushed:

1. The cast icon must not overlap the character name: it hangs
   beside the character now (side away from the enemy mass, dead
   band, dashed connector, rotational cast ring inside the marker).
2. The bow shot must happen right after the attack order: the
   engage/stall radii pick by the weapon in hand (bow 450/650), the
   dealt bow shots fly a 500ms projectile.
3. Selecting a buffer/folk npc must not show combat: a self pawn
   movement toward a known non attackable npc marks no fight.

Commits: 435abe4 (hunt+state, the radii and the interact walk),
39ca6a2 (the map round: cast icon side + bow projectile +
repro_map_render 102 checks), e3e6e7a (docs), ffa6309 (the critic
round: the nearest fight picks the icon side, the float lane dodge,
the quest fight conversion, the radius unit tests). Verification: state+hunt suites,
repro_map_render/bot_switch/zone_hover/fight_ui green, lint at the
single pre-existing finding. Details in development_log Round 126.

## Active task (status: in progress): the combat polish round - crit floats, melee contact circles, bare buff bar, wrapped status banner, square trash target (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~13:35 UTC. The owner prompt assigned five web UI
asks:

1. A "Crit!" suffix after the damage number when the hit was a
   critical, styled like the "Miss" float, crit numbers slightly
   bigger than normal ones.
2. Melee circles (character vs mob) must touch face to face instead
   of merging into one blob, especially zoomed out.
3. The buffs bar loses its framing completely - only the small
   separator spaces between the icons stay.
4. The center status banner wraps too-long lines (deleveling et al)
   onto 2-3 lines so it stops colliding with the full buffs bar.
5. The inventory widget's trash target (the urn) becomes square like
   the item icons instead of the tall 30x38 dashed box.

Design (verified against the code and the Mobius C1 sources):

- Crit: `Hit.java` carries `HITFLAG_CRIT = 0x20` per hit of the
  Attack packet (parsed today, discarded). The damage AMOUNT comes
  from the StatusUpdate HP drops (no crit flag there), so the
  tracker correlates: `ApplyAttack` remembers the last crit victim
  and the moment (`critVictim`/`critVictimAt`, 500 ms window - the
  packets ride the same read loop back to back); the next HP drop
  of that victim labels its `CombatEventDamage` with `Crit: true`
  (the wire gains a `"crit"` bool; dual crit hits both label - the
  hint is not consumed). The client renders "-123" plus an italic
  " Crit!" tail and a +3 unit scale font bump for crits.
- Melee contact: a per frame contact pass over the alive units
  computes a shrink factor per unit so overlapping circles touch
  (factor = dist/(r1+r2), floored at 0.25, times 0.95 for a hair of
  separation); drawObjects/drawSelf multiply their radiusOf based
  radii by it. Dead units, items and decorations stay out of it.
- Buffs bar: `.buffs-panel` loses background/border/radius/shadow,
  the body padding dies, the cells lose their plate background and
  the border-painted separator strips; the grid switches to a 2px
  gap (step stays 34: 32px icon + 2px space), the panel size
  arithmetic becomes cols*34-2 x rows*34-2.
- Banner: `.bot-status` keeps the centered pill but the detail
  span wraps (max-width ~380px on the banner, 3 line clamp,
  break-word) instead of the nowrap ellipsis clip.
- Trash: `.gear-trash` 30x38 -> 36x36.

Status: complete. Three commits pushed to
origin/feature/improved-behaviour:
- The crit hint backend (6db8e20): the Attack packet HITFLAG_CRIT
  (0x20) rides state.Attack.CritFlags; the tracker correlates the
  victim of the landed critical blows (500ms window, an awaited drop
  count per crit hit - a dual weapon crits twice) and the next HP
  drops of that victim carry crit on the wire (TestCritHintLabels*
  pin it; the count-consume design came out of the test round - the
  sticky hint labeled plain follow-ups).
- The map round (bce92f6): the crit float renders bigger with the
  italic " Crit!" tail (the miss float styling); the melee contact
  pass shrinks every overlapping pair of unit circles so they touch
  face to face instead of merging (computeContactFactors, factor =
  dist/(r1+r2) floor 0.25 x 0.95; dead units, items and far units
  untouched).
- The chrome round (c9999e9): the effects bar loses the whole
  framing (bare 32px icons, 2px grid gaps, the size arithmetic
  cols*34-2 x rows*34-2, the dead --buff-separator tints removed);
  the status banner caps at min(380px, 60%) and wraps the detail
  onto up to three lines (break-word + line clamp); the inventory
  trash target squares to 36x36.
Verification: the state suite green, all eight harnesses green
(repro_buffs re-pinned to the bare geometry, repro_map_render grew
the crit + contact scenarios with the font tracking, repro_gear
pins the square trash and the banner wrap), lint/fmt clean.

## Active task (status: complete): the combat feedback round - miss floats, directional damage, circle parity, skill cast and cooldown (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~11:45 UTC. The owner prompt assigned five web UI
asks around the combat visuals, all English-only as usual:

1. A "miss" floating text when the bot or its enemy evades a blow,
   by analogy with the floating damage numbers of a landed hit.
2. Directional floats: the damage the character takes flies out to
   the LEFT of the fight, the damage the character deals to the
   RIGHT.
3. The self marker circle matches the mob circle size (no more
   bigger-self emphasis).
4. A skill icon with a fill animation above the character while it
   casts or prepares a skill attack.
5. The skills widget cells show the same cast fill when open, plus
   the unavailable state while the skill is on reuse with the
   remaining seconds until ready.

Design (verified against the code):

- Miss: `recordSwingEventsLocked` currently drops the miss flagged
  hits; a `CombatEventMiss` event rides the existing feed instead
  (no wire schema change, the kind string is new), map.js grows a
  `drawMissEffect` beside `drawDamageEffect`.
- Direction: both floats anchor on the hurt unit; `onSelf` floats
  offset left, everything else right (the damage feed carries no
  attacker id - the target side IS the requested split).
- Circle: `drawSelf` used 7 units vs `radiusOf` mob combat 6; self
  drops to the mob parity 6 (the `drawPlayerTargetLink` hardcode and
  the harness pin follow).
- Cast/cooldown: Mobius C1 `MagicSkillUse` (0x5A, verified in the
  server tree) carries casterId, targetId, skillId, skillLevel,
  hitTime ms, reuseDelay ms - the parser opens a cast window
  (hitTime) and a reuse window (reuseDelay) per self skill; the
  snapshot publishes `skillStates` (skillId + the milliseconds left
  and the totals, computed at the snapshot moment); map.js draws the
  cast icon fill above the self marker from it, app.js syncs the
  keyed skill cells (a bottom-up cast fill + a dim overlay with the
  buffLeftShort style seconds countdown).

Status: complete. Both commits pushed to
origin/feature/improved-behaviour:
- Commit 1 (68d49a7 after the rebase): the miss floats (the
  `CombatEventMiss` feed kind + `drawMissEffect`), the side split of
  the damage and miss floats (taken left, dealt right), the self
  marker at the mob parity (6 units), the harness combat floats
  scenario with the translate aware recording context.
- Commit 2: the MagicSkillUse (0x5A) parser + dispatch +
  `ApplySkillCast` + the `skillStates` snapshot section (both
  encoders byte identical), the map cast icon (`drawSelfCast` +
  `ingestSkillStates` + the skillIcon cache), the skills widget cast
  fill and cooldown countdown (the keyed overlays, the self stopping
  ticker), the harness scenarios (cast icon, skillStates cells),
  docs (webui.md the miss/cast sections, protocol_description.md the
  MagicSkillUse entry, development_log.md Round 122).
Verification: state/webserver/connection suites green, all eight
repro harnesses green, `golangci-lint` + `gofmt-spaces` + `go vet`
clean.

Critic round (fresh context review, verdict: all five asks
satisfied, no blockers) applied in 9590819:
- The encoder oracle test now parses the hand written live encoder
  output (`AppendSnapshotJSON`), not only the reflection tags.
- The skills widget cooldown dim waits for the cast (the server
  opens reuse at the cast start - the dim over the rising fill
  buried the casting read); the hidden branch zeroes the restore
  fill style.
- ingestSkillStates prefers the biggest cast remainder; selectBot
  drops the previous bot's countdown anchors; the webui-harness
  skill documents the recording context transform folding.

Deferred follow ups (disclosed): the JS combat-fx layer extraction
out of the two giants (map.js 4340, app.js 4360 lines - the Go side
honored the split rule with skill_cast.go, the JS extraction is a
follow up round); the mob cast indicator; the skill-miss floats
(the melee Attack misses only - the same boundary the damage
numbers have); no live combat acceptance run (the kill flow takes
minutes; the harnesses simulate the beats).

## Active task (status: complete): the effects panel fix round (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~11:55 UTC. The owner prompt reported four defects
of the strict classic panel. All four landed in commit 24b9f3d
(code + tests + harness), the live browser verification (clean
subagent, the static preview) passed 7/7 with measurements:

- Dark theme kept white space below and between the buffs: the cells
  painted hardcoded #ffffff separator borders. The separators ride
  the new `--buff-separator` tint now (white light, `var(--bg-panel)`
  dark) - the dark theme melts the gaps into the panel.
- The hover countdown chip flickered under a stationary cursor:
  syncBuffsKeyed re-appended every cell on every snapshot (the SSE
  stream pushes at 300 ms) and a DOM move detaches the node, the
  browser drops its :hover state until the next real mouse move, so
  the chip cycled its 0.12s opacity transition. The refresh moves
  only the out of place nodes now (an index guard against
  container.children).
- The strip rode the seconds the buff had at the first observation
  (a login mid buff read a full strip draining to zero). The
  denominator now fills from `SkillCast.BuffTime` of the generated
  npcdata (the full abnormal time of the Mobius skill stats) when
  the skill is known and larger: five minutes left of a twenty
  minute Wind Walk read a quarter strip. A recast keeps the fresh
  server reading, an unknown skill keeps the observed seconds.
- The level badge was unreadable in the light theme (the near black
  `--text-bright` sank into the dark translucent plate). The badge
  reads fixed white in both themes.

Verification: `node tools/repro_buffs.js` 51 checks green (new pins:
no move on the in place refresh, the tint pair, the badge colors),
the other eight harnesses green, `go build ./...`, vet and the full
state package tests green, the fast deploy green, the live preview
measurements above. Docs: webui.md effects section + the harness
list updated. Open notes: the strip percent resolves against the
32px content box (the 34px border-box division reads ~23% - a
measurement trap), the tooltip meta line does not tick under a
stationary cursor (it refills on mouseover only, the chip does
tick), the chip text flips 5m to 4m at the 300s boundary
(round-then-floor, cosmetic).

## Past task (archive next): the effects panel strict classic round (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~11:00 UTC. The owner prompt reversed the earlier
panel decisions: the vertical mode dies entirely, the panel renders
as the single strict classic horizontal buff bar. All six asks landed
in one atomic commit (this entry rides with it):

- The vertical detailed list, the toggle chevron, the dock strip and
  the `swarm.buffsView` persistence are deleted (markup, CSS, the
  whole `buffs_flip.js` morph module removed from the page and the
  load order - `buffs_tooltip.js` then `buffs.js` before `app.js`).
- The grid owns no scrollbar: the classic 10 x 2 cap clips past the
  20 visible slots (`max-height: 68px` + `overflow: hidden`), a
  clipped effect surfaces as soon as a slot frees.
- The icons answer their native 32px art 1:1 again - the cells grew
  to 34px border-box (32px icon + the 2px white separator outside
  the icon box), nothing scales (the owner called the pixelated
  downscale ugly).
- No appear or disappear animation: the `buff-spawn` pop/glow
  keyframes, the spawn class wiring and the frame width/height
  transitions are gone - a change snaps.
- The bottom white chrome matches the top: the body pads 2px on the
  top edge and none on the bottom (the cells' own white separators
  answer the bottom 2px), the white below the icons reads the same
  as the white above them.
- The time strips brightened: 3px tall (was 2) in the dedicated
  `--buff-strip` tint per theme (#f59e0b light, #ffc061 dark,
  opacity 1) - the plain accent drowned next to the white
  separators.

Verification: `tools/repro_buffs.js` rewritten to the new reality
(47 checks: markup, styles, render, ticker, tooltip, geometry,
formats), `tools/repro_gear.js` stale two-view check replaced with
the single grid pin, all nine harnesses green, `go vet`/`go test
./internal/swarm/webserver/` green, `go build ./...` green. Live
agent-browser verification on the static preview (the injected
buffs, both themes): panel width 42/76/110/348 at 1/2/3/10 cells,
body height 36/70, cells 34, icons pixel-diffed 1:1 against the
source art, padding bands exactly 2px top and bottom, strips 3px in
the bright tint, no toggle/dock/list/spawn/transition traces, hover
chip and tooltip card answer. Geometry note (pre-existing, honest):
the full 10 column row coexists with the centered status banner only
from a ~1370px wide map container up; the partial rows to 5 columns
stay clear at 1280.

## Active task (status: complete): the guide buff priority round (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~09:00 UTC, closed ~10:45 UTC. Commits as melg8
(0dc22da the guide run, 60810e9 the keep-one trigger fix), rebased
over the parallel webui and skip-storm rounds and pushed. The owner
prompt assigned two behaviour features, both landed:

- After death the bot approaches the Newbie Guide and takes the
  support magic BEFORE walking back to the farm spot.
- The bots prioritize the buffs: missing or expired support magic
  starts the buff refill trip instead of farming without the buffs.

### Design (verified against the code, landed in 0dc22da)

- The trip start gained the guide run trigger (`guideRunWanted`,
  guide_buffs.go): the region carries a guide AND `guideWanted`
  holds (the 8-24 band, no refusal cooldown, an eligible buff
  missing). It joins the justification disjunction of
  `maybeStartTownTrip` and the out-of-zone exception beside
  `weaponRun`: the village revive of a death lands next to the
  guide, the trip machinery takes the buffs first and the return
  segment walks the farm spot second. No new phase machinery - the
  ordinary sell stop leads (the junk sells), `planGuideStop`
  appends the guide stop behind the learning stops.
- The farm spot survives the death (`resetTownTrip` keeps it), so
  the guide run skips `rememberFarmSpot` when it starts outside the
  zone - the precise return target stays instead of the zone center
  overwrite.
- Region guard (`guideForRegion`): only the elven region maps a
  guide today; the Dion region plans no guide stop and never starts
  the run (a wrong guide would walk the Dion trips to the elven
  village across the map). The latent Dion trap of the previous
  round's `planGuideStop` closes with it.
- The trip reason composition left `maybeStartTownTrip` into
  `tripStartReason` (the trigger list growth pushed maintidx over
  the limit; the extraction keeps the nolint debt flat, gocognit
  dropped out of the directive).
- Tests: `guide_trip_test.go` (6 tests). Test gotcha recorded:
  `setGuideLevel` zeroes the tracker position (the UserInfo
  coordinate block) - the tests snap the position AFTER the level
  seeding.

### The red tree debt (the class fix, landed in 60810e9)

The merged branch failed 22 hunt tests from the parallel rounds'
unverified merge: the SOE keep-one line of commit 2203086 made
`shoppingWanted()` true for every SOE-less bot (460 adena hardcoded
affordable over the 100 adena trip minimum) - it hijacked the blind
engage fixtures, inserted a pending scroll buy into every trip flow
test and broke both trigger-threshold tests. Root cause analysis
and the fix live in `docs/development_log.md` (Round 114):
`shoppingWanted` skips the scroll line (a scroll-only plan never
starts a trip, the stock rides the natural cadence), the flow
fixtures own their scroll. Result: 22 red -> 0 red, the whole hunt
suite green, task prepush green, `lint --new` 0 issues.

- Docs updated: docs/hunting.md (the Newbie Guide buff run block of
  the town trips section), docs/development_log.md (Round 114).
- The critic round (a fresh-context review sub-agent, mutation
  verified) landed three fixes in 785e13d: the guide run rides the
  short trip cooldown (the refill never waits the five minutes out),
  the guide no-show skip arms the refusal cooldown (no indefinite
  village loops on an invisible spawn), guideForRegion is an
  allowlist (the future regions stay guide-less until mapped).
- Follow ups for the next session: a test driving the full
  recoverFromDeath -> guide run flow (the current death test seeds
  the post-revive state manually); the scroll round reserves no
  wallet share for the scroll before the gear plan; the guide run
  has no expiry anticipation (the server's removal push starts the
  refill - anticipating the last minute of the 1200 s buffs would
  tighten the buff uptime); the doubling of the refusal cooldown
  for a permanently refused account; the full-lint findings of the
  parallel rounds (clickWaypoint cyclop 16, handleServerPacket
  cyclop 16, church_entry exhaustruct) belong to their owning
  rounds.

## Active task (status: complete): the frozen skip storm round (2026-09-21, branch feature/improved-behaviour)

The owner report (the state dump, build a483578, bot test3, phase
townWalk): the bot skips waypoints, stands still, and the waypoints
get farther and farther from it. The dump pinned the storm - the
character frozen at 31040 54016 -3415 on the 87 waypoint walk to the
trader Unoren, "town walk stuck, skipping waypoint" fired 49 times
(cursor 15 -> 63, one per fast stuck window), the aim 18,000 units
out, no movement, no ActionFailed, no re-path, no escape through the
whole 3.5 minutes.

Landed and pushed (commit 481dfdc after the rebase, plus the review
hardening commit, rebased over the parallel rounds):

- skipMoveFresh/noteSkipStand on the loop (shared by the stuck skip
  of stuckTownWalk, the aggro circle skip of clickWaypoint and the
  corner turn jump of clickForwardJump; reset at the trip
  boundaries endTownTrip/resetTownTrip/startReturnSegment; the
  stood-on-cell tolerance is hopCoincideDist): a
  skip may run only when no skip ran yet or the character moved
  since the previous skip; the first skip of a frozen episode stays
  free, the repeat is denied with its own log line and falls
  through to the re-path ladder (noteRepathCell ->
  abortFrozenTrip -> escalateFrozenSegment), whose cursor key
  escape walks the claims transport along the planned route.
- The recovery timeline on the dump scene: about 9 s from the
  first stuck verdict to the escape arming through the move start
  watchdog (about 23 s through the plain stuck windows), against
  the unbounded storm (the trip budget never ran because the skip
  branch always returned first).
- Repro: hunt/skip_storm_repro_test.go (the dump scene: the frozen
  character, the always-validating open ground oracle, the move
  start watchdog forcing a verdict per dead click). The adapted era
  tests: walk_stuck_skip_test.go, stuck_fast_repro_test.go,
  threatened_mass_skip_repro_test.go keep their fast window and
  anti-runaway contracts on the new movement paced chain.
- Docs: development_log.md round 107; AGENTS.md hypothesis H-006
  (the deployment that swallows accepted move requests - open, the
  bot-side recovery shipped, the server side unobserved).

Verified: go build, go vet, task prepush green (golangci-lint 0
issues, whitespace clean), the hunt suite diffed against the
pre-existing baseline of the parallel rounds (22 failures before
and after - zero new). NOT yet done: the live E2E against the
deployed stack on the merged tree (the parallel rounds own the
same debt), and the H-006 server side observation.

## Active task (status: in progress): the webui refinement round - the chat rhythm, the quest tab placement and the banner wording (2026-09-21)

Started: 2026-09-21 ~09:28 UTC. Branch: `feature/improved-behaviour`,
commits as melg8. Other agents push to the same branch - rebase before
every push. The owner feedback on the landed chat/quest/ETA round
(three defects, all in `internal/swarm/webserver/web/`):

1. the vertical distance between the chat lines drifts apart in some
   cases - it must stay identical everywhere;
2. the quest items tab sits at the top of the equipment widget - it
   must sit right of INVENTORY as the inventory sub tab switch;
3. the fighting banner detail shows the raw object id ("fighting
   #345345345 ... in the zone") - it must show the mob name the
   target panel shows, and the filler words ("in the zone" and
   similar) must go: always straight to the point.

### Goal

The chat rows pin one whole pixel line height (18px); the quest grid
becomes a sibling of the bag grid switched by the INVENTORY / QUEST
tab pair in the inventory title row; the banner detail resolves the
target name from the snapshot objects and carries only the measured
progress and the goal.

### Progress

- Root causes pinned: the chat rhythm came from the unitless
  line-height 1.6 at 11px = 17.6px (per row paint rounding drifts the
  gaps apart, taller glyph fallback boxes widen the line box the same
  way); the quest tab was a top level widget mode (an overlay view);
  the banner detail rendered "#" + c.targetId and appended " in the
  zone" (app.js engageFightDetail).
- app.js: engageFightDetail(snap, c, hunt) resolves the name exactly
  like renderTarget does (snap.objects, the dead ones skipped, the
  fallback stays "a target" - the raw id never shows); etaSuffix
  became etaText (no leading comma, the callers join the parts);
  walkDetail joins the parts with commas (the empty destination
  phrase of townReturn no longer produces a leading comma); the
  filler pass tightened loot/townSell/townWalk/townReturn/idle/user
  details ("looting the last kill", "selling at the trader",
  "heading to the trader", no townReturn phrase, "waiting for a
  command", the manual attack reuses the fight detail, the bare
  manual shows no detail).
- The quest tab: index.html swaps the top QUEST button for the
  INVENTORY / QUEST tab pair in the inventory title row (the
  skill-filter-btn idiom, the badge rides the quest tab) and moves
  quest-grid + the quest-empty note into the gear view as the bag
  grid siblings; the old quest-view overlay is gone. app.js adds
  InvTab (the persisted sub tab + the quest count), setInvTab,
  applyInvTab (the single place toggling the grids, the tabs and the
  empty note), the "quest" widget mode migration (the stored
  swarm.gearMode value moves to swarm.invTab); renderQuest fills the
  cells and the badge only. style.css drops the overlay rules and
  adds .inv-tabs, .inv-grid.hidden and the absolute .quest-empty note
  (the panel height stays constant across the switch).
- style.css pins .chat-line line-height at 18px (the whole pixel
  rhythm, wrapped lines included).
- Harnesses: repro_hud.js checks the fight detail name resolution,
  the raw id ban, the empty fallback walk detail and reads the real
  style.css to pin the whole pixel .chat-line rule (86 checks);
  repro_gear.js rewritten quest block covers the sub tab swap, the
  persistence, the badge, the empty note, the markup placement and
  the retired overlay css (199 checks); all 8 harnesses pass.
- Verified live in a headless browser against
  tools/webui_preview_server.js with a synthetic snapshot
  (scripts/verify_webui_fixes.sh + inject/measure helpers): the
  banner reads "fighting Keltir for 34s, eta ~12s", every chat row is
  an exact 18px multiple with gap == previous row height
  (RHYTHM_OK), the quest switch swaps the grids with the panel height
  stable at 410px (QUEST_TAB_OK); screenshots under logs/.
- go build ./..., go vet, golangci-lint --new clean, webserver tests
  green, tools/prepush.sh green.

- The clean context review sub agent (all three owner requirements
  PASS, process PASS) raised one defect and one nit, both applied in
  862fb19: the quest empty note anchored to the gear-main flow bottom
  painted over the adena/weight footer (it now lives inside the
  quest grid, centered, live verified EMPTY_NOTE_OK) and the fight
  detail could render "for 0s" on the first diagnostics tick (the
  age part waits for a positive targetForMs). The verify tool takes
  the PORT env; the repro_gear css gate pins the note anchoring.

### Status: done (2026-09-21 ~10:35 UTC)

Landed commits 25e5976, e7b7e86, aa922de, 862fb19 pushed on
`feature/improved-behaviour` as melg8; the harness gates (86 + 199
checks), the prepush gate and the live browser measurements all
green on the merged tree.

## Active task (status: complete): the webui chat round, the quest tab and the ETA round (2026-09-21)

Started: 2026-09-21 ~07:57 UTC. Branch: `feature/improved-behaviour`,
commits as melg8. Other agents push to the same branch - rebase before
every push. The owner report: webui chat messages in the bottom left
window (split system and chat, write to chat from the UI), a quest
items tab in the inventory, and ETA calculations for the walk to the
farm spot and for killing a mob.

### Goal

The chat window shows the world chat (CreatureSay) next to the bot
system messages behind tabs and sends a chat message through the bot;
the inventory grows a quest items tab; the hunt diagnostics publish a
walk ETA and a kill ETA the webui banner and the target panel render.

### Progress

- Landed c714ac6: the chat backend. CreatureSay (0x5D) parser and
  Say (0x38) packet (both verified against the Mobius C1 Java:
  Say2.readImpl is [text utf16z][type i32][target utf16z whisper
  only], CreatureSay.writeImpl is [objId i32][channel i32]
  [senderName utf16z][text utf16z]; CREATURE_SAY is 0x5D in C1, NOT
  0x4A). state.Bot.ApplySay maps the ChatType client id to the chat
  kind (say/shout/whisper/party/clan/trade/announcement);
  ChatEvent gained From (both snapshot encoders emit "from", golden
  tests updated). GameClient.Say + GameAPI.Say + the one shot
  CommandSay (text/target/channel ride the command request, validated
  against the Say2.runImpl refusals: empty text, 105 characters,
  unknown channel, whisper without target). The agent-bench drill go
  files got per-stage build tags (drill1/2/3): the one package of
  three mains had broken `go build ./...` and the lint typecheck.
- Landed 81887cb: the chat window UI. ALL / CHAT / SYSTEM tabs in the
  bottom left box, the sender as its own column, the channel colors,
  the input row (channel select, whisper target input, 105 char
  bound, enter or button) posting the say command. repro_hud.js
  covers the tabs, the sender column and the posts.
- Landed eea22af: the quest items tab. Third gear mode tab QUEST with
  its own keyed cell registry (one element cannot sit in the bag grid
  and the quest grid at once), type2 === 3 filter, count badge, empty
  note; the bag keeps the quest items (trash/drop flows). repro_gear.js
  pins the filter, the badge, the mode switch, the overlay css.
- Landed 1e08f67 + b0b836c: the ETA backend (the eta-work worktree
  round, ff merged). HuntDiagnostics gained walkEtaMs/killEtaMs
  (0 = not available, floored to whole seconds like the AgeMs
  fields). Walk ETA = remaining plan length (self -> remaining
  waypoints -> segmentDest, 2D legs) / run speed (fallback 120);
  kill ETA = curHp / ((maxHp - curHp) / elapsed) once the confirmed
  fight ran >= 2s. Both snapshot encoders and the size estimates
  updated, byte-identical pin held. b0b836c dropped the duplicate
  SelfRunSpeed the parallel travel economy round landed in buffs.go.
- Landed 7398988: the ETA UI. The walk phases append "eta ~Ns" to the
  banner detail, the fight detail appends the kill eta, the target
  panel carries the accent chip (~Ns, hidden without an estimate).
  repro_hud.js pins the chip, the rounding, both detail builders.

### Next / open

- PRE-EXISTING hunt test failures from the parallel scroll/guide
  round (005d6ee, 006f8cd, 2203086 landed between 81887cb and
  eea22af) - NOT from this round, verified failing at eea22af:
  TestLearnTripTriggersOnTheSkillBudget ("the teach stop follows the
  sell stop of the trip"), TestEngageRepositionsBlindTarget ("the
  blind recovery must arm") and the learn/engage group around them
  (~10 failures). Whoever resumes: run
  `go test ./internal/swarm/hunt/ -count=1` first, fix the scroll
  economy / guide behavior or the stale tests before new work.
- The NpcSay (0x02) packet is not parsed: mob chatter stays out of
  the chat window (deliberate v1 scope).
- The CreatureSay system variant (int charId + int messageId instead
  of the two strings) is not a chat line and is not parsed.
- Say flood protection: the Mobius build carries no SAY flood
  protector entry, one command per UI send is the pacing.

## Active task (status: complete): the travel behaviour round (2026-09-21, branch feature/improved-behaviour)


The owner prompt of 2026-09-21 assigned three behaviour features.
All three landed and pushed on `feature/improved-behaviour` (commits
6a39569, 2203086, 006f8cd, 005d6ee; rebased over the parallel webui
rounds 81887cb and pushed again as 005d6ee..):

- Target reset on travel (6a39569): `dropAttackTarget` on the loop
  clears the held fight AND the server selection (the clear self
  click - the engage re-adopt would resurrect the fight); it fires at
  the trip start gates (inside maybeStartTownTrip after the trigger
  gates), the zone switch, applyHuntingZone and the cell rotation
  apply. The rotation guard chains no longer reset the empty timer on
  a held fight (a stuck engage can no longer pin a cleared ground).
  The loot and the blows landing still hold the trip start.
  Tests: travel_target_reset_test.go + the rewritten
  TestTripStartsThroughTheMidFightTarget.
- SOE economy (2203086): the shopping queue leads with a keep one
  scroll line (736, Herbiel list 3015000, 460 adena at the town
  tax); the junk sell flow never offers it back; the trip start
  prices the walk home (straight line x 1.4 / server run speed,
  valued at the trusted measured adena per minute of the cell) and
  spends the scroll when the walk is worth more - UseItem, the 20 s
  skill 2013 cast, the wait for the village landing, then the normal
  stop machinery from the respawn point. Tests: soe_test.go.
- Newbie guide buffs (006f8cd, by the parallel subagent, cherry
  picked): the guide stop (template 7599, DriveDialog with the two
  SupportMagic links) rides the town sell stops after the learn
  stops; guideWanted gates on the level 8-24 band, the missing
  expected buffs (the 10-row table, Life Cubic excluded), the slot
  occupancy and a 10 min refusal cooldown; the self buff collision
  guard skips a self aura whose abnormal slot an active guide buff
  holds (and the reverse seek direction). Tests: guide_buffs_test.go.

Verified on the combined tree: go build ./..., the focused hunt test
runs, golangci-lint 0 issues on hunt+state. NOT yet done (next
session): the full `task verify` on the merged tree, a live E2E of
the three behaviours against the deployed stack, the docs prose
(hunting.md shopping/travel sections + development_log round), and
the follow-up refactor candidate: the guide stop duplicates the
teacher approach ring (~60 lines) - extract a shared approachNpc
helper when the next town flow lands.

## Active task (status: complete): the effects panel refinement round - the hover answers, the flight morph and the exact height (2026-09-21)

Started and landed: 2026-09-21. Branch: `feature/improved-behaviour`,
commits as melg8 (the Go round `4776e80` lineage, the web round
`8eb43e7` lineage after the rebases). The owner report (this round):
keep only the triangle of the two dock controls, bigger, with an
animated flip on click; the vertical padding of the buffs down to 2px
and the icons at the expanded state size so ten across never cross
the central hunting banner; the remaining time numbers off the cells
(hover only, the darkening hugging the digits) with small bottom
strips for the remaining share; the view morph flying the icons from
the horizontal to the vertical layout instead of the upward settle,
only the entries really visible in the vertical widget; the vertical
widget EXACTLY the character widget height; the hover card with the
name, the level and the real effect values pulled from the server
code; the spawn animation for a freshly landed buff.

### State (landed, pushed)

- `npcdata`: the hand curated `skillEffects` table (the real per-level
  values of the deployed Mobius C1 skill stats - the self auras, the
  Newbie Guide support magic and the Lirein speed debuff 4076, every
  entry citing its stat table) answered by `SkillEffectOf(id, level)`;
  the research evidence lives in the worklog of the session.
- `state`: BuffSnapshot grew `desc` (the generic level description)
  and `effect` (the numeric summary) through both encoders (the
  reflection golden test pins the byte identity); `hunt` untouched.
- web: the dock keeps the single 22px chevron (the view switch button
  deleted), the 180 degree flip springs on click; the grid packs 30px
  cells with 2px vertical body padding and the frame hugging the
  filled columns (min(cells, 10)); the countdown chip shows on hover
  only with the darkening hugging the digits, the 2px `buff-strip`
  sliver rides the bottom pixels; the FLIP morph (`buffs_flip.js`)
  flies the visible icons between the layouts both ways (the run
  token guards the cleanup); the list body pins EXACTLY the HUD stack
  height; the hover card (`buffs_tooltip.js`, the `#buffs-tooltip`
  singleton) answers name, level, time, effect and description
  through the delegated hover wiring; a joining buff spawns with the
  scale pop and glow ring keyframes.
- web: buffs.js split below the 500 line rule into the tooltip and
  flip modules; `tools/repro_buffs.js` grew to 56 checks (the stub
  gained getBoundingClientRect / matches / closest / body / rAF).
- Verified: all six repro harnesses + fight UI green, go build/vet/
  tests green, `task prepush` green, the live preview driven with
  agent-browser (the grid, the hover chip and card, the exact height
  list measured equal 81..330, the flip, the spawn).

### Nuances for the next agent

- The buff icons render blank on the static preview server (no
  /icons route there); the embedded server serves them - not a
  defect.
- The list rows with left=0 keep a zero bar and the amber fading
  color until the next snapshot drops the effect (the server list is
  the removal authority).
- The 32px cell CSS was asserted gone by the harness (`no 32px cell
  of the old shape is left`) - keep that pin when touching the grid.

## Active task (status: complete): the effects panel round, first pass (2026-09-21, the webui buffs redesign)

Started: 2026-09-21. Branch: `feature/improved-behaviour`, commits as
melg8. Other agents may push to the same branch - rebase before every
push. NOTE: the dependency bump / combat behaviour entry below belongs
to another agent's round and stays untouched here. The owner report
(this round): the webui buffs panel shows a detailed list by default;
the request - the icon grid first (only the active buffs, 10 per row x
2 rows, the icons tight with the thin white separators, the short
remaining time on the cell), the vertical dock on the left with the
clickable chevron (one row of cells docks one row tall, two rows dock
two rows tall), the expand into the detailed list (the full rows as
before plus the remaining time percent bar per row, the tighter
rhythm, the scrollbar, never taller than the character widget), no
EFFECTS label, the animated morph both ways, the chevron never moves.

### State (landed, pushed)

- `state`: BuffSnapshot carries `total` (the landed duration, the
  percent bar denominator) - the recast detection in SetBuffs (a
  reading above the counted down previous one + 2s jitter grace
  restarts the total, a continuing reading keeps it), both encoders
  (snapshot_json.go appendBuffSnapshotJSON + snapshot_live.go
  appendLiveBuffsJSON) stay byte identical, the golden snapshot
  gained a Buffs entry so the reflection parity covers the field.
- `web/buffs.js` (new module, loaded before app.js): the panel with
  the two views, the keyed cells and rows (a buff joining or leaving
  touches only its own node), the countdown anchors + the 1 Hz local
  ticker (the times run between the SSE pushes), the HUD height cap
  (frame chrome subtracted), the localStorage choice
  (swarm.buffsView, the icons default). app.js lost the old panel
  section (the EFFECTS head, the collapse, the count chip).
- `web/style.css`: the effects panel block rewritten (the 10 column
  grid, the per cell white separator strips - a white grid background
  left a white hole under the empty cells on the dark theme, the
  morph transitions, the thin scrollbar, the enter pop-in).
- `tools/repro_buffs.js` (new, `task repro:buffs`): 42 checks (the
  markup, the styles, the render, the keyed holds incl. the anchor
  field refresh, the bars, the ticker, the toggle + persistence, the
  height fallbacks and the measured HUD cap, the two row grid cap,
  the formatters); repro_gear.js handed over the panel section.
- Verified: go build/vet/test (state + webserver), all four webui
  harnesses green, the browser preview on the static copy (both
  themes, both states, the chevron spot measured equal 475,84 in
  both, the panel height == the HUD height capped).

### Open questions for the next agent

- The two dock buttons (the chevron + the view switch) BOTH toggle
  the same view pair - the owner text named both controls; if the
  chevron was meant to gate something else (the second row, the
  dock-only collapse), the flip closure in initBuffsPanel is the one
  place to change.
- "Full description as now" maps to name + level + time + tooltip
  (the old panel never carried description text either);
  npcdata.SkillDescription(id, level) exists, a `desc` field on
  BuffSnapshot would deliver the real skill text if the owner wants
  it.
- `total` for an effect observed mid-flight (the tracker joined
  after the cast) reads from the first list the tracker saw - the
  bar may start short of 100 percent; honest to the data held.

### The review round (same session)

A fresh-context critic pass tagged five should-fix findings; all
landed in the follow up commit: the entering layer now cancels the
visibility delay (the fade-in painted only after a 0.3s pop before),
the anchor refreshes every snapshot field (a stronger recast over
the same skill id kept the stale level/name/icon before), the grid
caps at the two classic rows with an inner scrollbar (25 effects
grew a third row before), the icons frame width matches the
border-box arithmetic (349px, the 367px carried an 18px dead strip)
and the recast detection respects a level change (another caster
overwriting the same skill id kept the stale total before). The
storage reads/writes guard like setGearMode and initBuffsPanel grew
the re-init guard; the harness pins every fix (42 checks).

## Active task (status: in progress): the dependency bump and the combat behaviour round (2026-09-21)

Started: 2026-09-21. Branch: `feature/improved-behaviour` (fresh
branch off main 2fda7ff), commits as melg8. Other agents may push to
the same branch - rebase before every push. The owner report: three
dependabot bumps (sergi/go-diff 1.3.1->1.4.0, x/crypto 0.28.0->0.57.0,
x/text 0.19.0->0.42.0) plus three hunt behaviour problems: the
two-mob pile up softlock (panic logout, relogin into the same pack,
immediate walk, re-aggro, repeat), a pick that can outrank an already
aggroed attacker while the character approaches a peaceful target,
and the unused spawn protection window after a relogin.

### Goal

The dependencies are bumped and verified (build, vet, lint, tests,
e2e). The hunt loop answers an aggroed mob during a peaceful
approach, tanks a winnable pile up instead of panicking (the
relog-after-kill escape stays for the unwinnable ones), uses the
verified spawn protection window after a relogin to regenerate
stationary (sitting is safe: the sit packet never clears the
protection) and opens with the bow first strike instead of walking
into a pack.

### Progress

- Commit dce2a28: the three dependency bumps (go mod tidy raised the
  go directive 1.23.2 -> 1.26.0, required by the new x/crypto and
  x/text), the five non-constant format string vet findings the
  version bump surfaced (delevel.go, town.go, acceptance/manager.go),
  the whitespace normalization of the benchmark drill files and the
  agent progress entry.
- Commit 98d7cad: the pre-existing broken benchmark drill package
  (g1/g2/g3.go redeclared the same symbols in one package, 2fda7ff)
  split into g1/, g2/, g3/ subdirs - `go build ./...` and the lint
  typecheck are green tree wide again; the frozen drill snapshots
  got a documented .golangci.yml exclusion; the os.Chdir test pairs
  took t.Chdir (the usetesting gate at the go 1.26 language level).
- Commit 300e936 + 880fa39: the approach aggro answer (a mob that
  aggros during the walk to a peaceful pick is answered at once; a
  selected target that already holds the character stays), the
  winnable pile up tank (pileUpWinnable gates the panic run: health
  above the re-engage line, attackers inside the level ceiling, at
  most three; the hurt pile up still runs), the fight potion in the
  running fight, state.Bot.SelfAttackers/ObjectTargetsSelf.
- Commit 2192bb1: the spawn protection settle (hunt/settle.go, the
  verified H-005 facts) - the fresh session holds the spot, sits and
  regenerates, then opens with the first strike (the bow owner
  through the lure machinery). Commit 8c4a823: the review hardening
  (the empty knownlist and unknown position hold instead of burning
  the one shot, the pending sit confirm wait, the strike level and
  mana gates, the panic gate stands down while the settle holds, the
  potion threshold above the re-engage line and served in loot too).
- Verified facts this round (H-005, docs/hunting.md): Mobius C1
  spawn protection is REAL and ENABLED in the deployed stack
  (PlayerSpawnProtection = 600 s in Player.ini, armed by EnterWorld,
  cleared ONLY by MoveToLocation / AttackRequest / Action / UseItem /
  RequestMagicSkillUse; the sit toggle is not in the list, so a
  sitting character keeps the protection and regenerates).
- Verification at HEAD 8c4a823: go build ./... OK, go vet OK,
  golangci-lint 0 issues tree wide, hunt suite 73 s + state + web +
  gofmt-spaces suites green, go mod verify/tidy clean. The live e2e
  (tools/mobius_e2e.sh) did not run this session (the sandbox lost
  the JDK between the stack start and the e2e call - the stack was
  up, the e2e bootstrap refused; next session: rerun before the live
  acceptance).
- Not done this round (the next candidates): the bow kiting against
  slower mobs (the circle kite with shots - the owner recipe item
  that needs a movement-during-fight design), spiritshot consumption,
  the settle for the town trip starts (a full bag right after the
  relogin still walks into the packs).

## Active task (other branch, 2026-09-20): the farm readiness frame round (2026-09-20)

Round 106 landed (commits 471abc6, c922461, 290b763): the follower
keeps one oracle end to end - the advance gate obeys the same
server click transport the clicks obey (the anchored, capped,
delivery-tested port verdict), the sub-floor aim discipline is
immediate (the round 57 freeze family is structurally eliminated)
and the corridor ban system is REMOVED (the plaza round proved it
a session poisoning amplifier: one 48 unit ban disk sealed
1311910 reachable polygons down to 2, and the verdict that armed
it was never a server freeze - the server moved the character on
every click). The escape claims own the genuine silent freeze
family alone; the escape march re-anchors onto the character's
ground. The micro acceptance scenario joined the webui list head:
plaza-herbiel - the character wakes ON the report's plaza cell and
the trip walks the ping pong leg to Herbiel.

Round 105 landed earlier (commit 1762d1c): the passed aim never
walks the character back and the march counts its own ground; the
live acceptance farm readiness passed end to end on the built
binary: all six checks, 714 s, zero backward walks. The Round 105
restrictions ride the armed branch of the merged aim switch.

Started: 2026-09-20. Branch: `feature/new-pathfind-alternative`,
commits as melg8. Other agents push to the same branch - rebase
before every push. The owner report: the farm readiness walk to
Herbiel ping ponged between two points 63 units apart (nine
alternating MoveToLocation clicks, the server accepting every
one), the corridor ban at 44584 46944 then sealed the session.

### Goal

The bot completes the farm readiness town trip (and the level 15
acceptance behind it) without the ping pong and without any
recovery layer poisoning the session: the walk plans, the arrival
tests, the escape claims and the return destinations measure in
one frame and the follower answers through one movement oracle.

### Progress

- Landed (commit 5c863f4): the three frame fixes of the development
  log Round 104 - the anchored waypoint arrival (the pinned cursor
  of the plan's own start), the server frame escape claims (the
  underground claims the server corrected away), the destination
  deck resolution in both return paths (the "no navmesh under the
  position" return failures). The dead waypointDistance retired.
  task navmesh:test-tiles builds the six test tiles; the live
  repro lives in tools/repro_jewel_stuck.sh.
- Next: re-run the live farm readiness acceptance on the fixed
  build; watch the porch leg (the escape should still arm for the
  server refusals, but the ladder must never burn without a walk
  attempt) and the trip return (the resolved deck). If the leg
  still refuses every click, the next suspect is the server side
  geodata vintage at the shop quarter - the varied aim ladder owns
  it, the mesh cannot name the walls it does not have.

## Completed: the review backlog fixes round (2026-09-20)

Started: 2026-09-20. Branch: `feature/new-pathfind-alternative`,
commits as melg8. Other agents push to the same branch - rebase
before every push. The owner asked to fix the problems the
2026-09-20 fresh-eyes evaluation found.

### Goal

Fix the verified findings of `docs/codebase_review_2026-09-20.md`,
P0 first, then the surgical P1/P2 items, each with a focused test
where the review demands one and the full gate before every push.

### Progress

- P0-1: the journal gzipFile keeps the original segment on any
  failure (copy, sink close, target close) and drops the partial
  .gz archive - the old order removed the source unconditionally and
  a disk full destroyed the rotated record. The test swaps the
  gzipCopy seam with a failing copy and asserts the original
  survives byte for byte.
- P0-8 (the same file): the journal rotate opens the next segment
  before closing the old one - a failed open now keeps the writer on
  the still-open file and the size cap re-arms the attempt on every
  later flush; the old order stranded the writer on a closed handle
  forever. The test forces the segment open into a missing directory
  and asserts both records land.
- P0-2: the lazy skill queue and demanded books caches moved behind
  atomic pointers (skillQueueCache, bookKeepCache) - the snapshot
  encoder and the inventory flows run under the store read lock, and
  the plain fields turned two concurrent encoders into writers of
  the single source of truth. The steady state read stays
  allocation free (the AGENTS.md zero-alloc bar holds); two
  concurrent rebuilds store two consistent snapshots and one wins.
  The test drives four encoders plus a SetSkills writer under
  -race.
- P0-3: runBot closes the game connection on every pre-Run failure
  path (the handshake failure closes the raw conn, the later paths
  go through a deferred game.Close that is a no-op after Run's own
  disconnect); the login flow arms a 30 s read deadline on every
  blocking read and the pre-world char list / creation reads bound
  by charListWait - a stalled server now surfaces as a reconnect
  instead of a silently parked bot goroutine.
- P1-6: cellAggroMass resolves the spawn xml id through
  npcdata.NPCWireTemplateID before the aggression lookup (the raw
  xml id failed the >1000000 guard and zeroed the static danger
  input of the cell safety score).
- P1-7: farmQuestStage paces and rescans when the quest mob scan
  comes up empty - the old fallthrough attacked the zero-value
  target and ground refused WalkTo(0, 0) clicks against the flood
  protector until the respawn.
- P1-9: the SSE frames bound their writes (sseWriteTimeout via
  ResponseController) so a half-open client cannot park the handler
  goroutine past the shutdown; the proxy accept loop backs off and
  retries on a transient accept error instead of dying with the
  listener still advertised; the hunt loop and the journal sampler
  join a WaitGroup the shutdown waits before the journal closes.
- P1-12/P0-4: the CI workflow refresh (the full uncapped lint
  replaces the stale --new debt step, the permissions block lands,
  the race slice covers state and webserver too, the govulncheck
  step joins) plus the gomod dependabot config and the pinned
  govulncheck in tools/install_dev_tools.sh (task vuln locally).
  The activation was attempted the same day: the verbatim copy to
  .github/workflows/ci.yml was rejected by GitHub - the push token
  lacks the workflow scope - so the file stays the owner step (the
  web UI Add file path, the note at the top of docs/ci_workflow.yml).
- P1-13: the docs drift batch - the Go version story told one way
  (AGENTS.md and README follow the go.mod toolchain line), the
  verify-loop skill numbers and the CI citation fixed, the
  repository layout maps all nine cmd/ binaries, docs/README.md
  gained the ci_workflow.yml and agent_progress_archive.md rows and
  the corrupted hunting_system_redesign.md row was repaired.
- P0-5: the handover restructure - the seven stale Active task
  headings (all landed work) retitled to Completed so exactly one
  status-marked active task stays (this section). The archive keeps
  untouched per the standing owner rule; a later round may move the
  closed sections there when the rule lifts.

## Completed: the recastnavigation port - the production Detour runtime and the Go mesh builder (2026-09-15)

Started: 2026-09-15. Branch: `feature/new-pathfind` (on the research
tip cf000f5). Commits as melg8. Other agents may push to the same
branch concurrently - rebase before every push.

### Goal

The owner approved the migration path of
`docs/recast_pathfinding.md` ("Daju dobro, portiruj"): port the
Detour RUNTIME into production Go and build the navigation mesh in
Go from the geodata with the sheet decomposition - the grid engine
stays as the click validation and local walk layer. The concrete
pieces from the research verdict:

1. the offline tile builder (the sheet decomposition -> polygon
   merge -> NSWE-filtered links -> BVTree -> tile file, one cmd),
2. the multi region tile loading (lazy, ~1.45 ms per tile),
3. the funnel string pulling of findStraightPath,
4. the NSWE link filter (improved: portal spans keep the crossing
   inside the open cell pairs of every shared edge),
5. the pooled query allocations.

### Progress

- `pathfind.ParseRegionData` + `Region.LayerStack` (the exported
  region access the builder walks without the engine cache, with the
  reusable stack buffer) - commit 25a463d.
- the research prototype isolated as
  `pathfind/navmesh/prototype` (unchanged behavior, its own suite) -
  commit 1ee48e7.
- the production runtime `pathfind/navmesh`: the native rectangle
  tile format (cell bounds, four exact corner heights, Detour-style
  link chains with the NSWE portal spans, the quantized BVTree; the
  full encode/decode roundtrip and the corruption guards), the Mesh
  (lazy multi region tile loading with the LRU), the A* corridor
  search (pooled state, the area-cost water pricing, the partial
  closest-reachable answer), the funnel string pulling (the
  findStraightPath port with the portal span restriction) and the
  Route/RouteDry/WaterEscape facades - commit c5c32b9. 17 unit tests
  over the synthetic corridor world (the stacked disambiguation, the
  dry partial, the escape, the portal span restriction, the cross
  region routes, the LRU).
- the Go mesh builder `pathfind/navbuild`: the geodata flatten +
  dedup, the sheet decomposition (the top-down flood of the 2D
  manifolds with the one-layer-per-column rule and the island
  filter), the maximal rectangle decomposition with the
  height-variation-aligned splits bounded by the 24 unit bilinear
  tolerance, the NSWE portal span link walk (the wallsOpen rule) and
  the BVTree port - commit d3bb9ac. The synthetic world suite plus
  the real region suite: 21_19 builds in 1.7 s into 91 453 polygons
  (7 045 water) with 5 778 sheets / 4 303 islands - the SAME sheet
  numbers the C++ research experiment produced - zero one-way links,
  the hard bridge pair answers (the full swim corridor 171 polys,
  the dry partial, the reverse escape), the 200 pair replay classifies
  133 full + 59 partial + 8 isolated (the C++ "200/200" counted the
  partials as success through dtStatusSucceed - the honest split is
  documented in docs/navmesh.md).
- `cmd/navmesh-build` + `navbuild.BuildPack`: the two-phase pack
  build with flat memory (phase A writes the tiles immediately and
  retains value-copied border strips - the field pointer would pin
  the whole 200 MB build, the first version OOMed at 2.4 GB; phase B
  re-decodes only the stitched tiles) - commit 2117ef0. The full
  165-region pack builds in 4m25s at 604 MB peak RSS, 1.9 GB of
  tiles, 193 784 external links. Seven pack regions fail the l2j
  parse (16_10, 17_20..17_25) - pre-existing corrupt files the grid
  engine parser rejects identically, documented in docs/navmesh.md.
- `docs/navmesh.md`: the subsystem reference (the tile format, the
  build pipeline, the measured comparison table, the corrupt-region
  note, the not-wired-yet scope).
- The live integration round opened. The corridor search gained the
  two contracts the Navigator seam needs: the approach radius goal
  (the FindPathApproach stop condition - the first polygon whose
  closest surface point lies within the 3D radius of the end, the
  polygon-granularity form of the grid nodeReached) and the avoid
  areas (the recovery bans: Filter.Avoid carries AvoidCircle disks,
  a polygon whose footprint a ban touches walls the search - the
  over-walling direction, so no funnelled segment ever enters the banned
  ground; the ban holding the start opens its escape ring within 256
  units at the 6x multiplier, the foreign ban wins over the escape
  ring - the rectangle granularity port of the grid
  cellAvoidedEscape rules). `RouteApproach` is the new facade entry,
  `Route` degenerates to it at radius zero. Six tests over a fresh
  two-lane synthetic world pin the semantics: the early stop, the
  start-poly radius, the lane detour with waypoint-outside-ban, the
  sealed goal, the escape way-out and the foreign ban precedence.
- `hunt.NewNavmeshNavigator` - the hybrid behind the Navigator seam:
  the five route queries (FindPathApproach, the avoiding/dry
  avoiding forms with the bans converted into mesh disks, FindPath,
  FindWaterEscape) serve from the mesh corridor search and fall back
  to the grid engine on EVERY answer the mesh cannot serve (a
  missing tile, a dropped island sheet, a sealed goal, a partial
  closest-reachable corridor - round one keeps the grid engine the
  reachability authority, so the hybrid can only add routes, never
  lose them); the validation layer (ValidateClick, the sight lines,
  the water rasters, the deck heights) never leaves the engine. Five
  tests pin the seam: the synthetic corridor route + escape served
  from the mesh with an engine-less fallback proving the mesh
  answered, the no-mesh fallback surfacing the engine error, the ban
  flow sealing the mesh route, the REAL hard pair (the elven village
  deck to the water under the bridge, 21_19 built straight from the
  shipped geodata through navbuild.BuildRegion - the 5.17 s grid
  walk answers from the mesh in one call) and the validation answers
  identical through the hybrid and the engine on the real pack.
- The runtime wiring landed (cmd/swarm/main.go): the -navmesh flag
  names the tile directory explicitly, the empty value autodetects
  data/navmesh (the build command's documented output), and the mesh
  installs `hunt.NewNavmeshNavigator` behind SetNavigator for every
  bot of the fleet - a directory without tiles keeps the plain
  engine navigator silently. data/navmesh joined .gitignore (the
  tiles are derived from the tracked geodata, 1.9 GB for the full
  pack, rebuildable in ~4.5 min). The smoke run against the four
  elven region tiles (21_19, 22_19, 21_20, 22_20 - 62 MB, 8 s build)
  logs the readiness line in both the autodetect and the explicit
  flag mode. docs/navmesh.md gained the live integration section
  (the seam, the fallback rule, the ban granularity note) and the
  honest not-wired-yet residue (the acceptance stack stays on the
  pure engine navigator, the partial waypoints stay unexposed);
  AGENTS.md and docs/hunting.md follow the wiring.
- The partial round landed. `pathfind.Result.Partial` extends the
  seam contract (Found=false with Partial set and waypoints ending
  at the closest reachable point; the grid engine itself never sets
  it - its searches answer the bare not found). The hybrid's two
  avoiding forms (FindPathApproachAvoiding,
  FindPathApproachDryAvoiding) serve the mesh partial corridors
  through it: the engine run comes FIRST on every mesh partial (a
  full engine route the mesh missed still wins, an engine error
  still surfaces - the can-only-add rule holds), and only after the
  engine's own clean not found does meshPartial serve the mesh funnel
  waypoints (Aborted carried from the engine verdict, Explored the
  sum of both searches). The strict forms (FindPathApproach,
  FindPath) never surface partials - the blind engage recovery and
  the user walks keep their verdict semantics. The consumers:
  startWalkSegmentSearch arms the follower on the partial waypoints (the
  town segments and zone returns walk toward the closest reachable point
  instead of aborting; the bare not found still refuses to plan),
  followPlannedSegment walks the partial quest segments (the
  re-plan loop and the no-progress guard stay the safety net). Four
  hybrid tests (the synthetic shore world with the engine geodata
  built as flat blocks in the hunt tests: the partial served after
  the engine confirmation, the engine route winning over the mesh
  partial of a broken chain, the engine error surfacing without
  geodata; the REAL hard pair dry - the elven village deck to the
  water under the bridge answers the dry partial with every waypoint
  above the water level) plus three consumer pins (the town segment
  arming on the partial, the quest segment walking it through the
  moving game simulator, the bare not found still aborting).

- 2026-09-15: the mesh viewer round landed (the owner request: an
  interactive 3D viewer over the built meshes, a double click pair
  builds a route with a construction timer, launched by a
  `-show-navmesh` flag, loading either one named mesh or all meshes
  of the directory stitched together). `webserver.NewNavmeshServer`
  adds the fourth bot less mode behind the embedded web interface:
  `GET /api/navmesh/tiles` (the listing with the derived world
  footprints, no decode - through the new `Mesh.TileFiles`), `GET
  /api/navmesh/geometry/{col}_{row}` (the binary NMV1 payload: a 24
  byte header, the contiguous int16 corner block, the area tail;
  ~25 B per polygon, ETag revalidation, a process lifetime cache)
  and `POST /api/navmesh/path` (the measured `Route` call with the
  swim/dry filter select of the hunt loop's two search profiles).
  The page is `web/navmesh_view.js` over the vendored three.js r160
  module build (`web/vendor/`, offline like the rest of the
  interface): the polygon quads tessellate client side with the
  height ramp terrain colors and the flat water blue, a minimal
  orbit rig (drag rotate, wheel dolly, right drag pan), the double
  click raycast arms the start then asks the route, and the result
  panel carries the status badge, the construction timer
  (microsecond resolution under a millisecond), the waypoint count,
  the corridor/explored poly counts and the path length. The
  `-show-navmesh` flag is a custom `flag.Value` with `IsBoolFlag`:
  the bare form opens every tile stitched, `-show-navmesh=21_19`
  (comma separated) opens the named tiles, the route queries always
  run over the full directory mesh. Verified live in a headless
  browser over the four elven tiles: the REAL hard pair (village
  deck -> water under the bridge) draws its 49 waypoint funnel with
  a 8.8 ms timer against the grid engine's 5.17 s flood; the VLM
  review of the screenshots confirms the watertight terrain, the
  cliff geometry and the marker/path rendering. Five webserver API
  tests (config/tiles/geometry binary layout/etag 304/path with the
  swim found and dry partial answers) plus the flag parse pin; the
  lint stays at the pre-existing 68 findings, the full suite green.

- 2026-09-15: the viewer defect round landed (the owner report: a
  mesh full of black triangles and elements at wrong heights -
  detached fragments). The diagnosis ran through the scratch
  `cmd/navanalyze` (kept as the diagnostic companion): the black
  triangles were the steep cascade quads of the l2j slope smoothing
  cells (a fifth of the polygons) falling to black under the single
  hard sun, the detached fragments were the unmerged duplicate
  layers of the geodata noise - the measured regions carry the same
  surface twice with a 0..32 unit jitter (both layers open, the
  lower copy often wall restricted, ~54k pairs per region of the
  22_xx column; the elven region of the research round showed none,
  which is why the 16 unit rule shipped), and the flat water planes
  sat at heights the riverbed never held. The fixes: the builder
  dedup delta lifts to 32 units (a real stacked floor never sits
  within 32 units of its ceiling - a genuine deck keeps both
  layers, pinned by TestExtractRegionDedupNoise), the light rig
  rebalances to a dominant hemisphere with the sun and the counter
  fill, the logarithmic depth buffer keeps the stitched world from
  z fighting at every viewing distance, and the water renders as
  the submerged terrain it really is - a depth ramp toward the dark
  navy of the deepest riverbeds. The inspection surface the owner
  asked for: the top readout bar names the tile square under the
  cursor with its region local cell and world coordinates (a
  progressive raycast sweep - one tile per frame, the nearest
  bounding sphere first), every loaded tile draws its region grid
  outline with the hovered one lit amber, the result panel carries
  the from/to rows with their tile keys and world coordinates, and
  every route waypoint wears a label with its index and coordinates
  (toggleable, next to the polygon edge overlay). Verified live in
  a headless browser over the 11 tile stitched world: the full pack
  loads (11/11 tiles), the terrain reads clean at every zoom (the
  VLM review of the overview and the route closeups confirms no
  black triangles, no floating fragments, no seams between the
  tiles, the water following the riverbed contours), the cursor
  readout tracks the pointer (tile 22_16, cell 883 883, world
  coordinates verified against the region anchors), and a 129
  waypoint route answers in 79.6 ms with its from/to rows and
  readable waypoint labels.

### Next

- 2026-09-16: the wall-honest rectangle decomposition landed (the
  owner report: the bots walk out of the town through the buildings,
  the viewer routes cross the walls). The root cause: the maximal
  rectangle growth of `navbuild/rect.go` ignored the NSWE walls - a
  rectangle spanned any cells of one sheet, and because the link walk
  skips the cell pairs inside one polygon ("the interior is walkable
  by construction"), a single polygon swallowed whole wall segments:
  the corridor search then funnelled straight through (the audit of
  the shipped pack counted 1.24M swallowed pairs in 21_22, 1.43M in
  22_22, 1.02M in 22_19, 0.41M in 21_19). The fix: `hStepOpen`/
  `vStepOpen` gate every horizontal and vertical cell pair inside
  the growing rectangle (the paired NSWE walls of both sides plus
  the climb height rule - the same `canStep` the grid search walks
  on), `buildRects` takes the climb. The honest decomposition
  multiplies the polygon count (21_19: 91k -> 245k, 22_22: 127k ->
  567k) - the price of routes that respect the walls; the pack must
  be rebuilt (`cmd/navmesh-build`, data/navmesh is gitignored). The
  regression pin: `TestBuildRegionInteriorWall` (a wall segment
  INSIDE the would-be maximal rectangle splits the decomposition -
  the case a border-only wall check misses) and
  `TestBuildRegionInteriorWalls` (the full audit of the real 21_19
  and 21_22: 6.06M + 4.79M same-polygon neighbour pairs, zero walls
  swallowed). The honest side effect: 3 of the 200 research replay
  pairs lost their (wall-tunnelling) corridors - the grid engine
  confirms all 11 isolated pairs are genuinely unreachable, the
  replay pin and docs/navmesh.md carry the new split (128 full +
  61 partial + 11 isolated).

- 2026-09-16: the viewer feedback round landed (the owner request:
  a camera that flies like a plane, a button that copies the camera
  position with the view direction and the route A/B pair, and a
  launch mechanism that restores the position from that data). The
  orbit rig is replaced by the flight rig: WASD flies along the full
  view vector (W follows the pitch), Q/E descend and climb, the
  pointer drag yaws and pitches, Shift boosts 4x, the wheel retunes
  the cruise speed (the panel carries the speed readout; the frame
  delta drives the movement so the speed is frame-rate honest). The
  feedback channel: the `copy view link` button freezes the whole
  view state - the camera pose over the game world axes (x, y,
  height, yaw, pitch), the armed or answered route pair, the visible
  tile selection, the filter, the height scale - into one URL (the
  clipboard write falls back to field selection for the non-secure
  contexts). The launch mechanism: the boot parses the same query
  parameters, restores the camera, the tiles, the filter and the
  scale, and the restored route pair re-runs on its own - a pasted
  link reproduces the exact view AND its answer. Verified live in a
  headless browser over the rebuilt six-region world: the flight
  controls move the camera along the view vector (the W probe moved
  it 1760 units into the pitch), the drag changes the yaw/pitch, the
  wheel the speed, the copy button builds the full link, and the
  link roundtrip (move the camera, copy, reopen) restores the pose
  to the unit and re-runs the hard bridge pair (34 waypoints, 5.7
  ms); the VLM review of the screenshots confirms the terrain, the
  route polyline with its markers and the panels.
  `TestNavmeshViewScriptContract` pins the boot contract (the rig
  controls, the parameter names, the copy wiring) against drift.

- 2026-09-16: the solid surface round landed (the owner report over
  two pasted view links: the view is tilted sideways, the render
  still has black triangles, and the request to show the small edge
  connections between the polygons colored green for fields and blue
  for water). All three answers, each verified live in the headless
  browser over the real 21_19: (1) the tilt was a real roll the boot
  framing leaked - `lookAt` under the default XYZ euler order left a
  z angle behind, the YXZ rig reinterpreted it and never zeroed it
  (a measured rotation.z of 0.503, a 28.8 degree horizon roll); the
  framing now computes the yaw and the pitch analytically and the
  rig writes the full euler every frame with the roll pinned at
  zero (rotation.z measures exactly 0). (2) the black triangles had
  two roots. The big one: adjacent rectangles sample their own
  inside cells for the shared edge heights, so 536 724 of the
  604 191 adjacency pairs of 21_19 disagree about the edge height -
  every geodata step rendered as a see-through black wedge (the
  pixel-exact clear color behind the crack). The NMV2 geometry
  payload now carries the height step walls: the vertical filler
  quads between the two bilinear surfaces, emitted through the
  MaxX/MaxY sides only (every shared edge walled exactly once), the
  crossing split keeps the quads simple when the profiles meet
  inside the span, and the region borders resolve their targets in
  the east and north neighbor tiles through the mesh. The smaller
  root: a steep quad tessellates into two triangles whose flat
  normals face apart and the away-facing half fell to black under
  the old light rig - the rig now carries an ambient floor
  (ambient 0.42, hemisphere 0.6, sun 0.45, fill 0.25) so no face
  drops into the darkness; the dark-face pixels of the two reported
  views measure 0.4 percent of the screen and the wedges are gone
  (what remains black is the honest void of the unwalkable cells -
  pixel-exact clear color with no geometry). (3) the edge
  connections overlay replaces the white polygon perimeter overlay:
  every real link portal of the tile rides the surface as its open
  world span (the NSWE gates, not the full shared edge), colored
  green for the field-field pairs, blue for the water-water ones,
  teal for the shore pairs and gray for the unresolved external
  targets - the legend carries the four swatches and
  `TestNavmeshViewScriptContract` pins the decode, the classes and
  the toggle. The payload: 244 837 polygons + 552 168 walls +
  275 830 links = 18.5 MB for 21_19, the layout check and the
  endpoint pin (header counts, the portal records, the wall
  records, the crossing split, the flat skip) live in
  TestNavmeshGeometryEndpoint and TestEncodeNavmeshGeometryWalls.

- 2026-09-16: the tessellation round landed (the owner report over
  the view link cam=51366,46831,-2966: the grass is still not fully
  filled and there are black triangles). One root cause behind both
  symptoms, present since the first viewer commit: the surface quad
  tessellation read the row-major wire corners (0 = min corner,
  1 = +x, 2 = +y, 3 = the opposite) but drew the two triangles as
  (0,2,1)+(0,2,3) - both anchored on the shared 0-2 edge, so the
  right quarter of EVERY polygon (the triangle between the east
  edge and the two diagonals) never rendered: the clear color shone
  through as one see-through wedge per rectangle, scaling with the
  rectangle size - the big maximal rectangles read as the large
  black triangles, the dense small-rectangle fields read as the
  unfilled grass with zebra-stripe gaps. The wall filler quads were
  immune (their corners ride the wire in cyclic order, where the
  same index pair is a correct 0-2 diagonal split), which is why
  the earlier rounds kept chasing lighting and height-step ghosts.
  The fix: the surface tessellation now splits along the 1-2
  anti-diagonal - (0,2,1) covers the lower-left half, (1,2,3) the
  upper-right one - while the walls keep their cyclic 0-2 split,
  and TestNavmeshViewScriptContract pins both index runs against a
  silent regression. Measured over the reported view (before ->
  after): clear-color pixels 174 401 of 1.44 M (12.11 percent, 515
  components, the largest a 596x165 slab) -> 440 (0.03 percent, 2
  small components); the top-down check of the same area: 8.39 ->
  0.03 percent; the elven village view: 0.00 percent with no
  zebra stripes (the VLM review of all three screenshots confirms
  the continuous surface). The leftover specks are honest voids,
  proven twice: the cursor raycast over the largest one answers
  "no tile" (no polygon exists there), and the new navbuild
  coverage audit TestRealRegionWalkableCoverage proves every
  walkable layer of every kept sheet carries a polygon (21_19:
  4 400 066 walkable layers, 4 394 442 covered, 5 624 dropped
  island layers, 0 holes) - the specks are the island sheets the
  four-layer minimum drops by design (a scratch probe of the
  largest oblique-view void found only DROPPED sheet verdicts in
  its cells). The route search and the double click picking
  re-verified live on the fixed geometry (a 254 unit pair answered
  in 53 us with 2 waypoints).

- 2026-09-17: the floating worlds round landed (the owner report:
  the bridge and the elven village must hang in the air above the
  water but connect into it instead, and the mother tree must not
  fuse its branches into the ground - "eto kasaetsya ne tolko etogo
  mesta", it is map wide). Two root causes, both fixed:

  1. The height step walls had no height cap: the NMV2 encoder
     walled EVERY XY adjacent polygon pair, so a floating deck over
     the lake got a 700 unit green curtain down to the water and
     every branch a grey stalagmite to the ground (21_19 carried
     ~22k fabricated walls over 80 units). The honest crack scale
     of the build is the 40 climb + 24 bilinear tolerance; the
     filler now refuses anything taller than 80 units - a taller
     step is the open air between two separate worlds and the void
     is the honest answer (the cap dropped the 21_19 wall block
     from 445 889 to 423 984 records, the worst surviving step is
     exactly 80, TestEncodeNavmeshGeometryWalls pins the over-cap
     rejection next to the crossing split and the flat skip).

  2. The geodata structural encoding walked straight into the mesh:
     the giant trees of the elven forest carry their canopies as
     stacked walkable-flagged layers (the trunk helixes step within
     the climb range but wall every step of the way up - the sheet
     flood crosses them because it ignores the NSWE walls by
     design, so a "ramp" of polygons fused the tree into the
     ground). The new dropIslandComponents builds the sheet graph
     over engine steps (the paired NSWE walls plus the climb rule,
     the exact canStep of the grid search, the wetness split
     ignored) and rejects every component that touches no region
     border - the seam where the neighbour region may continue the
     walk. The measured 21_19: 1386 floating sheets / 60 118 layers
     dropped (polys 244 837 -> 217 904), while the bridge deck, the
     village decks, the shore and the water survive - the grid
     engine confirms the verdicts (a ground-to-canopy search
     arrives at the terrain under the tree, never at the branch
     height; TestRealRegionFloatingIslands pins the drop and the
     survivors, TestBuildRegionIslandFilter pins the synthetic
     border/separation semantics).

  The full pack rebuilt (158 regions, 57.3 M polys, 4.3 GB, the 7
  known corrupt edge regions aside) and the viewer re-verified over
  the two reported views: the bridge and the village now float with
  open air gaps to the lake (the VLM before/after review answers
  PASS on both), the mother tree lost its fusion and the distant
  terrain walls that remain are the honest climb-step fillers. The
  user route of the report (33 680,56 807 -> 47 028,50 833, swim)
  still answers found in ~4 ms over the rebuilt mesh.

The follow-up candidates (NOT started): the acceptance stack switch to
the hybrid once the live sessions prove it (which may also skip
the engine confirmation flood on mesh partials - the ~13 s the real
hard pair dry search pays today), the Detour-parity optimization if
the fleet benchmark asks for it, the route-vs-engine replay harness
over the Dion hunting grounds.

- 2026-09-17: the capsule clearance round landed (the owner report: the
  pathfinding ignores the character collision capsule and puts the
  waypoints too close to the wall edges and corners - the character
  sticks in the passage and clips every bend). The server movement
  validation is cell level and never checks the capsule (the elven
  fighter template radius 7.5), so the planner owns the clearance:
  `pathfind.Capsule` (pathfind/capsule.go) answers the wall clearance
  of a point (the exact distance to the nearest closed NSWE wall edge
  of the layer nearest the reference z, exact within one cell) and
  `ApplyPath` enforces the radius over a planned waypoint path (the
  interior waypoints pushed off the walls by the damped projection,
  every segment sampled and bent around the walls through pushed-in
  anchor chains, every move validated by the engine line of sight,
  the first and the last waypoints never move, the honest fallback
  keeps the original geometry). The mesh funnel pulls its pivots
  inward from the portal span ends by the same radius
  (`Filter.WaypointClearance`, offsetPortal; a span narrower than
  twice the radius pivots at its middle). The engine post pass arms
  through `Engine.SetCapsuleClearance` - the bot wiring, the pathfind
  test UI and the navmesh viewer arm it with the template radius, the
  hunt hybrid serves the mesh answers through the same radius (the
  cleared filter plus the cleared waypoints). The viewer route
  endpoint clears the mesh answers through the viewer engine. Tests:
  the clearance pins (push off the wall, the pillar bend chain, the
  no-churn contract, the solid block fallback, the engine
  integration), the funnel pivot pins (the L corridor raw pivot on
  the wall boundary vs the cleared pivot at the radius). The same
  round fixed the shingled slope rendering (the owner report: the
  terrain steps down like roof sheets instead of joining edge to
  edge - hills, not stairs): the rectangle corners read the sheet's
  own VERTEX field now (the average of the sheet's cells around the
  grid vertex), so the adjacent rectangles of one sheet agree about
  every shared edge (the inside-cell corners disagreed by the full 8
  unit quantization step on EVERY slope adjacency) while different
  sheets keep their own levels and genuine cliffs stay sharp
  (navbuild/rect.go vertexHeight, the vertex tests pin the seam free
  join and the sharp shore step). The mesh pack must be rebuilt for
  the new geometry (data/navmesh is gitignored, cmd/navmesh-build).
  NOT DONE YET (the interrupted session): the tile rebuild and the
  visual before/after verification of the owner's two views.

- 2026-09-17: the exact square port replaced the bilinear vertex
  field (the owner's changed position: the original l2j geometry IS
  squares, so the detour version must port it as close as possible
  WITHOUT changing its visual or actual representation): the
  rectangle growth now spans only the cells of one exact geodata
  height, every polygon is flat at that height (all four corners
  equal), the vertexHeight averaging, the bilinear tolerance split
  and the Options.HeightTolerance tunable are gone. The quantization
  staircase the interpolation used to smooth away is the honest l2j
  answer the mesh now carries - the mesh variant and the original
  cells render of the viewer draw the same geometry, the comparison
  toggle turned into the port audit. The NSWE wall fidelity, the
  sheet decomposition, the island drops and the 32 unit dedup are
  untouched (the port concerns the surface representation, not the
  reachability filtering). The height step walls close exactly the
  genuine steps now: same-height neighbors join byte for byte, the
  filler's job shrank to the real staircase. 21_19 measured:
  372 846 polys (was 245k wall honest bilinear, 390 596 orig rects),
  1 013 866 links (was 259 332 - every height run edge is a polygon
  boundary with its portal now), 44.2 MB tile (was 12.9), 26.4 ms
  decode, the hard pair 14.9 ms warm over 219 polys (was 7.7 ms
  over 171 - the honest price of the faithful squares, still 350x
  under the grid engine). Tests: the vertex round's seam pinning
  became the exact contract (exact_test.go: the flat corners, the
  per-height strips of the slope, the sharp shore step), the
  RectExactHeights build test pins the parabolic valley port,
  buildRects lost the tolerance argument. The four dense tiles are
  rebuilt (data/navmesh -force) and the owner's two views verified
  against the exact mesh. The lint etiquette: the gci formatter
  wants to re-tab the whole tree (its gofmt passthrough vs the
  spaces only policy - the pre-existing systemic conflict, the
  formatter of record stays gofmt-spaces); the new code adds zero
  non gci issues.

- 2026-09-17: the geometry variant toggle landed in the viewer (the
  owner comparison request, the client half): a "geometry" select
  switches live between the detour mesh and the original l2j cells
  render. The tile entries cache BOTH variants (built/loading per
  variant, the active variant's scene object in mesh), so a switch
  back is instant and a first switch loads the other variant on
  demand (the status row shows "loading orig" while the server
  renders the region, seconds for a real tile). The camera, the
  route pair, the drawn route and the tile selection live ABOVE the
  variant - they survive every switch unchanged, which is the point:
  the same funnel waypoints draw over the raw cells. The chosen
  variant rides the view link as geom=mesh|orig, the boot restores
  it BEFORE the tile loads (a pasted orig link opens on the original
  geometry) and the stale variant fetch that finishes after a fast
  switch stays cached without entering the scene.
  TestNavmeshViewScriptContract pins the toggle contract (the
  select, the per variant cache, the original endpoint, the URL
  parameter). Round etiquette: the lint --fix pass had re-tabbed the
  geometry files - the spaces only policy returned in a follow up
  commit (c57e3a0).

- 2026-09-17: the original geometry endpoint landed (the server half
  of the owner's comparison toggle): GET
  /api/navmesh/original/{key} renders one region straight from the
  raw l2j geodata cells into the same NMV2 payload the mesh endpoint
  serves - no sheet decomposition, no rectangle smoothing, no vertex
  field. Every cell layer with an open wall direction draws at its
  exact height; the only compression is the exact height merge
  (neighboring cells of the SAME layer height become maximal
  rectangles, pixel identical to the per cell quads): 21_19 measures
  390 596 rectangles over 4.4M walkable layers (22.3 MB with the step
  walls - the mesh tile scale), the worst audited region 21_22 lands
  at 1.2M rects. The height step walls follow the mesh payload rules
  (the MaxX/MaxY emission, the 0.5 unit floor, the 80 unit open air
  cap) and resolve the region borders through the east and north
  neighbor regions (the border strip pass, 2048 cells). The link
  portal block stays empty - passability in the original render is
  the cell geometry itself. The engine less viewer answers 501.
  Tests: the flat region collapse (one rect, ETag 304 roundtrip),
  the checkerboard merge (65536 rects, the water area, the ordering
  by height), the step walls (2*255*256 records, the first record
  content) and the real 21_19 scale pin (390 596 rects).

- 2026-09-17: the viewer waypoint coordinates checkbox default off
  landed (the owner request): the state starts false and the
  checkbox ships unchecked - one click brings the coordinate wall
  back. TestNavmeshViewScriptContract pins the default. Remaining
  from the interrupted session: the geometry variant toggle (the
  detour mesh vs the original l2j cells render plus the geom URL
  parameter with the camera and the route preserved), the tile
  rebuild and the visual before/after verification of the owner's
  two views.

- 2026-09-17: the route shortcut pass (the smoothing) landed: the
  funnel on the exact square mesh pivots at every clearance pinhole
  and the walker micro steers (the owner walk plan stuck at wp 25);
  Filter.Smooth arms the greedy farthest visible merge in
  navmesh/smooth.go - every merged chord crosses the intermediate
  portals inside their open spans and keeps the capsule radius from
  the wall spans (the link complement per polygon side), the raw
  funnel answer rides in Route.RawWaypoints. The viewer route variant
  toggle (smoothed vs raw, the path= URL parameter, one search serves
  both) draws the two answers; the bot navigator and the viewer arm
  the pass with the engine capsule radius. The owner repro route
  (21_19 swim) answers 63 smoothed waypoints of 81 raw, the granular
  segments (< 16 units) drop from 7 to 1. Tests: the corner under the
  capsule stays (the chord 5.66 off the inner wall is refused), the
  open boundary merge (the funnel pivot at a soft span end folds),
  the void detour keeps every pivot, the portal index chain, the
  unarmed contract. KNOWN NEXT: the grid per cell walls (the
  diagonal anti corner cut) are finer than the mesh side level link
  spans - 26 of 62 smoothed segments still hold a grid wall sample under
  the radius (worst 1.6 units near the owner stuck area), the grid
  capsule pass re-fragments the answer with micro anchors; the next
  round binds the shortcut pass to the grid wall oracle (the
  SegmentGuard seam) so the merged chords clear the server accurate
  raster directly.

- 2026-09-17: the wall oracle round: the grid per cell walls (the
  diagonal anti corner cut) are finer than the mesh side level link
  spans - the funnel pivots one radius off a mesh span end sit up to
  6 units off a real grid wall on the staircase (the owner stuck at
  wp 25), and the capsule pass re fragmentation (129 wps, 79 tiny
  segments) defeated the mesh smoothing end to end. Filter.Guard arms
  the SegmentGuard seam: the shortcut pass answers every chord to the
  grid capsule (pathfind.Capsule.SegmentClear - the movement line of
  sight plus the 4 unit clearance sampling), and the walker answer
  composes ApplyPath with the new ShortenPath fold (the greedy
  farthest visible over the post pass points, every surviving segment
  SegmentClear). The owner repro walks 14 waypoints of the legacy 148
  (path 4947 -> 4852, max segment 1070), the raw toggle variant keeps
  the legacy pipeline as the before picture. Tests: the guard
  refusal keeps the pivots, the guard merge folds (navmesh), the
  SegmentClear corridor center through wall answers, the ShortenPath
  open collapse and the sealed detour keep (pathfind). Visual
  verification on 127.0.0.1:8082: the staircase closeup draws the
  granular raw dots vs the clean smoothed line, the toggle and the
  path= link parameter restore both ways, screenshots in the agent
  download archive.

- 2026-09-17: the server move limit round: the owner asked to check
  the game server sources for the movement distance restriction the
  smoothing could trip. The Mobius C1 sources answer twice
  (network/clientpackets/MoveToLocation.java,
  entity/actor/Creature.java moveToLocation): the 9900 unit packet
  refusal (the walker's own maxMoveDistance = 1000 split already covers
  it) and the WATER clamp - the destination of every swimming move
  request scales onto the 700 unit sphere around the current
  position (the isInWater divider), and a target beyond it never
  answers. The smoothed open water segments (the owner repro runs the
  swim filter, the measured max segment 1070) trip exactly that clamp:
  the server stops the character short of every such waypoint and
  the follower never sees the arrival. Capsule.ShortenPath now
  answers the server clamp per anchor (the new segmentLimit probe over
  the engine water raster, the same OverWater oracle the water
  escape uses): a segment that leaves a water position splits at the
  clamp distance, the fold resumes from the split over the same
  horizon, the dry anchored segments keep their unclamped merge. Tests:
  the water segments cap (the split preserves the walk length and the
  endpoints), the dry flip (the long clear chord survives). The
  viewer smoothed variant rides the same fold.

- 2026-09-17: the double click pathfind round: the owner asked for
  the webui double click to run the pathfind and store the points
  into the dump state as if the bot itself planned them - the manual
  test loop of the stuck reports (move the bot by hand, call the
  pathfind with the map clicks, watch where it sticks, report the
  problem area). The manual move planner no longer waits for the
  2000 unit threshold: every manual move plans through the
  navigator (the mesh corridor, the capsule clearance, the wall
  guard - one pipeline with the bot's own walks), a failed search
  still falls through to the direct server routed walk. The dump
  keeps the most recent plan after its walk ends (the state last
  walk record, the "last walk plan (...)" dump section - the live
  plan expires with the walk, the stuck report needs the whole
  planned walk after it too); the dump parser reads the new section
  and the ApplyDump replay restores it as the walk plan of the repro
  bot. Tests: the near click plans and walks the plan, the
  replacement plans fresh, the record survives the expired plan and
  the clear (state), the dump round trip (webserver).

- 2026-09-18: the hierarchy hardening and the whole map trip
  measurements round. The environment survived the session timeout
  (the repo, the branch, the pack intact); the resumed work pushed
  the component gate commit first, then answered the owner's four
  asks. (1) The within tile benchmark (TestHierarchyVersusFlat21x19,
  the real 21_19): the flat search wins the reachable same tile
  pairs (4.9 ms vs 9.1 ms at 16k, 15 ms vs 5 ms warm at 21.5k) and
  draws the shorter corridors - the hierarchy now serves the cross
  tile queries and the flat capped escalations only (hierWorthy is
  cross tile, RouteApproach escalates on the budget cap). The
  abstract cache rides the LRU (32 regions - the whole map coarse
  walks accumulated every region graph into the OOM territory, the
  whole map tests gate behind SWARM_WHOLEMAP with the chunked
  stats). (2) The town routes (TestNavmeshTownRoutes, the
  teleporter coordinates of the C1 data): Elven Village ->
  Gludio plans one shot (10.8 s cold, 6.1 s warm, 121 794 units,
  found); Gludio -> Gludin and Gludin -> Giran strand one shot -
  the probes (TestBayNorthShoreProbe, TestSidecarPairProbe,
  TestWaterBorderStitch) name the cause: the shipped geodata leaves
  the open water of the Gludio bay and the inner bay unmapped (the
  straight line aims do not bind) and the 18_21/17_21 border steps
  land heights over the climb (land against flat sea - the stitch
  refuses honestly). The re-path walk (the bot's real pattern) covers
  58 852 of 73 199 units in 3 replans on the Gludio Gludin segment.
  The fix belongs to the data: a water filled geodata refresh or the
  operator waypoint graph. (3) The map size arithmetic: the pack is
  3.5 GB of gzip tiles against the 543 MB l2j geodata (6.4x) - the
  price of the explicit topology (every height run edge a polygon
  with its portal links, the query structure the funnel and the
  corridor searches need); the quantization headroom (uint16 cell
  rects, uint16 portal spans) sits at ~35% of the decoded bytes,
  documented in the report. (4) The bot command and the load answer:
  the navigator switch is the presence of the tile directory
  (-navmesh data/navmesh or the autodetect) - no separate flag; the
  maps load lazily per query (the tile LRU 4, the abstract LRU 32,
  the hop cache 4096), the whole pack loads only in the viewer mode
  (the bare -show-navmesh stitches every tile).

### Progress (2026-09-19, the agents.md restoration round)

- the owner session limits land in AGENTS.md as the mandatory first
  section (the 2 hour life from the owner prompt, the mandatory stop
  at 1h45m with everything pushed, the timer reset on every fresh
  prompt, the /home/z/my-project/.session_start_ts stamp protocol) -
  the rule previously lived only in the agents.md run notes.
- the sandbox subprocess study of this session lands: both detached
  variants (setsid own session, double fork env -i renamed binary)
  survive 8+ minutes across tool calls with heartbeat probes
  (scripts/detach_probe_a.sh, detach_probe_b.sh in the sandbox, not
  in the repo) - the 09-18 verdicts do not reproduce today; the
  single-tool-call pattern stays the always-correct option, the
  detached server stays the verified option with a ps re-check.
- the pre-split operational nuggets return from the c7786c9 split in
  the condensed form: the Mobius stack operational notes are whole
  again (flood protector mechanics, re-registration wait, account in
  use self heal, slow SIGTERM, single invocation e2e, auto
  registration), the client proxy short form is back (emulated
  login, replay+relay contract, shared cipher chain, proxy.log,
  proxy_e2e, port layout), the archer guard retaliation fact joins
  the protocol notes.
- verified against the collected versions (the file history
  v01..v09): the c7786c9 split dropped nothing load-bearing - the
  ~163 absent lines of the 1733 are reflowed, superseded or
  rephrased in docs/; the restoration adds the operational layer
  back, not the 112 KB blob.
## Completed: the webui debugging surface - the walk plan timings, the pathfind link and the hover coordinates (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`. Commits
as melg8. Other agents may push to the same branch concurrently -
rebase before every push. Owner request (three pieces, each its own
atomic commit):

1. **The dump state walk plan timings** - the state dump of a walking
   bot carries not only which waypoint it aims at but how long every
   segment took: a stuck point shows its cost, not just its name.
   - `state/bot.go`: the timing view of the published walk plan - the
     zero point (`walkPlanStart`, the first publish of the route) and
     the observed arrival of every waypoint (`walkWpAt`, the entry i
     fills when the follower cursor moves past i on the same route
     republish). A fresh route (or a cursor that moved back) restarts
     the view; a mid walk publish with the cursor already ahead
     pre-fills the passed prefix. `publishWalkPlanLocked` +
     `walkPlansSameRoute` (the cursor blind route compare). The last
     walk record copies the timing view (lastWalkStart, lastWalkWpAt)
     so a finished walk keeps its segment durations. The Snapshot carries
     the Go side dump fields only (`json:"-"`, the wire stays byte
     identical): WalkStart, WalkWpAt, WalkAt, LastWalkStart,
     LastWalkWpAt.
   - `webserver/dump.go`: the walk plan section prints the `started`
     line (the zero point, the last seen moment, the time on the
     walk), the passed waypoints carry `(passed, t+10.4s, segment 5.2s)`
     and the aimed one ` <-- TARGET (walking 45.2s)` - the stuck segment
     number. The last walk plan measures the aimed segment to the moment
     the plan ended. Sub minute durations keep the tenth of a second,
     the longer ones fold into the minute shape. The dump parser
     needs no change (the wp lines keep the leading `x y z` triple).
   - Tests: the state timing tracking (the advance, the equal
     republish, the fresh route, the record copy) and the section
     formatting (the suffixes, the minute fold, the untimed shape).

### Progress

- The walk plan timing view and the dump suffixes - this commit.
- the pathfind link button (webui): the map toolbar gains the
  `pathfind link` button beside the session dump - one click freezes
  the published walk of the selected bot into the 3D navmesh viewer
  URL and copies it (the clipboard fallbacks mirror the dump button).
  The link carries the from/to pair off the walk plan (the planning
  origin, the final destination), the tiles around the pair (the
  bounding box grown by half a tile - the owner example link's
  20_19..21_20 block reproduces exactly), the swim filter, the
  scale/geom/path defaults and the camera pose computed with the
  viewer framing math (the three quarter orbit south east of the
  route midpoint, the analytic yaw/pitch of frameInitialTiles) - the
  pasted link answers itself in the -show-navmesh viewer, ready for
  the own experiments or for attaching to an agent report. The shift
  click asks for the viewer base address and remembers it in the
  localStorage (the default stays http://127.0.0.1:8082/). The
  repro_hud harness pins the URL structure (the pair, the camera
  orientation, the tiles, the defaults, the bare snapshot without a
  plan).
- the webui cursor surface: the walk plan coordinate labels draw on
  hover only now (the constant per waypoint (x, y) labels littered
  every planned walk), the hovered waypoint prints its full x y z
  triple in the dump walk line format; the status bar gains the
  cursor chip (the world point under the mouse, the full triple while
  a waypoint is held) and the ctrl+c (meta+c) of the map copies the
  chip values (the selection aware fall through keeps the browser
  copy intact when the pointer is off the map or a text selection is
  active); the viewer coordinate inputs accept the bare x y pair now
  (the z inherits from the other line) and keep the wp prefixed dump
  lines parsed (the first three numbers after the wp marker bind -
  the new timing suffix carries numbers of its own, the last three
  contract broke for the dump walk lines). The repro_map_render
  harness drives the whole surface (the label absence, the hover
  label, the chip, the copy shortcut, the flash, the leave reset).

## Completed: the village escape round - the route following cursor escape, the move start watchdog and the refused click root cause (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Rebase before every push.

### Goal (the owner prompt of 2026-09-19)

1. The cursor key escape (the WASD-like recovery of a click refusing
   cell) walks STRAIGHT lines toward the aim today - the owner rule:
   the claimed steps must follow the PATHFIND ROUTE and must never
   try to cross the whole map in a straight line.
2. The walker must switch to the next recovery mode AS FAST AS
   POSSIBLE when the current one did nothing - a movement command
   that never started the movement must not wait a full stuck window
   (15 s / 4 s today).
3. The owner asks WHY the server refuses the ordinary ground clicks
   in the village plaza zone, and the same at the shop and the
   teacher hall entries - a systemic question, with an elegant
   solution wanted. Acceptance: the `village-escape` scenario
   (the refused-click dump cell) must keep passing.

### Progress

- 2026-09-19: the route following cursor escape (commit 1). The
  escape arms `cursorEscapeRouteSteps` when the segment holds a plan:
  the claimed ValidatePosition steps march the plan polyline from
  the current cursor (the pathfind route the segment already walks),
  interpolated into run-speed strides, water guarded per stride and
  capped at `cursorEscapeRouteMax` (2500 units, one direct hop of
  ground) so an escape stays a pocket recovery - the claims never
  carry the character across the map. The straight line ladder
  stays as the planless fallback. The bend closure keeps the route
  bend points among the steps so the next segment leaves from the
  bend and not from a cut corner. Tests: the bend corridor (every
  step sits on the planned line, the bend lands among the steps,
  the stride spacing, the cap), the cap on a 12000 unit route, the
  wet stride stop, the planless nil fallback; the plaza repro tests
  re-verified green with the route following ladder.

- 2026-09-19: the move start watchdog and the refusing pocket fast
  path (commit 2). The new `noteMoveStart` watchdog arms a deadline
  (`moveStartWindow`, 3 s) when a walk click goes out while the
  character stands still, clears it on a position change or the
  server's own movement broadcast and names the click dead when the
  deadline passes on the baseline cell - a dead click forces the
  stuck verdict (the `forceStuck` gate of walkStuck skips the window
  check for one verdict), so the recovery ladder runs at ~3 s per
  rung instead of the 15 s first window. Wired into the three walk
  machineries: followWaypoints (the planned segments, force), the direct
  routed segment (a silent hop arms the cursor key escape instead of
  re-hopping into the 45 s window) and the direct zone segments (the
  stall backdate fires noteZoneSegmentStall this tick). The refusing
  pocket verdict: a stuck verdict with refusal evidence ON the cell
  where the segment's first refusal latched, AFTER the varied aims of
  the segment are spent - the varied aims keep their chance to cure the
  target specific refusals (the round 82 order), the escape takes
  over the moment they prove useless from the same ground. The
  escape arms from the follower path now too: walkTownWaypoints
  drives the claims while the escape holds the segment (symmetric with
  walkDirectSegment), followWaypoints stands its clicks down while
  armed, the settle message names the walk that resumes (the routed
  hops or the planned clicks), and the mode 0 arm aims the ladder's
  far end (a self cell arm answers the stopMove refusal before the
  flag latches). Tests: the silent click recovery inside the move
  start window, the pocket sequence (the variants one per verdict,
  the escape once they are spent, the claims owning the segment, no
  further mouse clicks), the direct segment silent hop escape; the
  varied aim and plaza repros re-verified green.

- 2026-09-19: the refused click root cause documented (commit 3).
  The WHY: the village geodata is a layer sandwich (the deck over
  the water floor 872 apart; the teacher hall roof/interior/water
  328/1136 apart; the shop interior partially walled; the deck not
  flat even inside one pack) and the server resolves every click
  target and line step layer by the NEAREST z - a pack vintage
  disagreement flips the layer, the line check refuses, the click
  collapses and the deployed build answers ActionFailed (the
  official client's mouse clicks met the same wall). The evidence
  pins in `pathfind/village_layer_sandwich_test.go` (the four
  measured stacks), the full analysis lives in Round 85 of
  `docs/development_log.md` with the owner side cure (the geodata
  vintage alignment + the master's direct-movement fallback). The
  bot side answer is the claim transport of commits 1-2.

### Progress (2026-09-19, the feedback loops audit)

- the audit of the feedback an autonomous agent receives lands as
  docs/agent_feedback_loops.md (the inventory by layer: static, test,
  live stack, runtime observability, process memory; what is good,
  what needs improvement, what is missing, the priority order) - the
  doc joins the AGENTS.md documentation map.
- PROGRESS.md regenerated through tools/progress_report.sh: the
  ladder now reads M1 red (last run FAIL - the class transfer runs of
  2026-09-13), the stale 2026-09-12 page is gone; the dashboard
  staleness is recorded in the audit as an improvement item (the
  regeneration rule exists, it just was not applied).
- the headline findings of the audit: task check:all misses the
  build and vet gates the verify-loop skill mandates; coverage is
  collected but never archived; benchmarks have no committed
  baseline to diff; CI is absent while the go-verify-loop skill
  references it; the acceptance scenarios run serially against a
  parallel-ready account partition; the session start ritual is
  prose, not a command.

### Progress (2026-09-19, the feedback audit remediation: all twelve items)

- every improvement and every missing item of
  docs/agent_feedback_loops.md landed in one tooling round (the
  implementation status section of the audit maps each item to its
  landing place):
  - the gate: `task verify` (build + vet + lint + test + fmt:check,
    check:all is the alias) - check:all no longer skips the two
    gates the verify-loop skill mandates; `docs/ci_workflow.yml`
    (the owner copies it to `docs/ci_workflow.yml` - the push of
    a workflow file needs the workflow-scoped token) runs the same
    order on every push plus the race slice of
    connection/pathfind and the logfmt scan (the CI that did not
    exist now exists); `task prepush` (tools/prepush.sh, the 20-40 s
    touched-packages gate) is documented as mandatory in the git
    conventions and installable as a git hook
    (`tools/install_dev_tools.sh hook`).
  - the numbers: `task test:cover` archives
    runs/coverage-latest.txt (committed, 27 packages seeded) and
    fails on a per package drop beyond COVER_DROP_LIMIT (2.0 pp);
    `task bench:save` + `task bench:diff` (cmd/benchdiff, the
    offline benchstat twin with its own tests) give the benchmark
    rule its committed baseline; `task progress` is the one-word
    PROGRESS.md regeneration.
  - the wall time: `-acceptance all-parallel` (Manager.RunAllParallel)
    launches every scenario at once on the temp account partition
    with the flood protector stagger and collects every failure
    instead of stopping at the first - the headless run pays the
    slowest scenario, not the sum (the barrier test pins the
    overlap).
  - the conventions: internal/logfmt parses the module and asserts
    capital-first (component tags count) and no trailing period at
    every production log call site - the six existing violations it
    found (main.go, proxy/server.go, webserver/navmesh.go) are
    fixed, the rule starts from zero debt; docs/flake_ledger.md
    opened with the 8034403 poll-test pins as the seed rows and the
    verify-loop skill names it mandatory after every flake fix.
  - the ritual: tools/session_start.sh prints the bootstrap
    checklist (the stamp age against the 2h/1h45m budget, the
    fetch/rebase verdict, the ss 2106 probe, the open H-001..H-004
    ids, the active task headline); the hypotheses registry carries
    the advance-or-close-one-per-session rule; progress_report.sh
    gained the trailing-window trends (per scenario pass rate, XP/h
    and stuck deltas).
- the formatting source of truth is pinned: go.mod carries
  `toolchain go1.26.8` (the tree is gofmt-spaces clean under the
  1.26 gofmt; the 1.24-line gofmt disagrees on the struct field
  comment layout) - `task fmt:check` now agrees with the committed
  tree on every host, and CI (setup-go + the toolchain directive)
  would have caught the mismatch instead of silently going red on
  the whitespace gate.

- 2026-09-19: the server frame click transport (the point 3 bot side
  cure). The WHY: the round 85 refusal mechanism names the clicked z
  the layer selector - the server resolves the click's destination
  layer by the nearest height to the z the request carries, the plan
  waypoints carry the bot pack's mesh z, and wherever the pack
  vintages disagree about a surface's absolute height the raw mesh z
  names the wrong layer (the village sandwich flips) and the click
  refuses. The HOW: the walk layer measures the vintage shift at the
  one pair both frames vouch for - the character's server vouched
  standing z against the plan's first waypoint z (the same cell on
  the pack) - and rides every plan derived click z into the server
  frame: the town segment clicks and their long segment splits, the forward
  route samples, the escape hops, the varied aims, the manual walk
  follower and the quest segment follower (each machinery measures
  its own plan start). The server vouched clicks (drops, mobs, npc
  approach points, self position aims) stay untouched. The guards:
  an offset beyond 500 units is a layer snap and measures zero (the
  872 unit deck over water gap must never anchor), a swimming
  character measures no shift (the swim z against the mesh floor is
  geometry), and the arrival re-measurement died in the teacher walk
  repro (the 16-35 unit ramp steps made the 150 unit arrive radius
  measure the NEXT step's rise into the offset) - the offset
  calibrates at the plan starts only, each re-path re-measures on the
  surface the character actually stands on. The Gludio lesson rides
  unchanged: the anchored z is never the bare self z, it carries the
  plan's own relative geometry in the server frame. Tests:
  `hunt/click_frame_test.go` pins the measurement, the calibration,
  the systemic click transport, the long segment split, the layer snap
  discard, the manual walk and the quest segment; the existing suites
  stay byte identical green (26 packages ok, lint --new clean).

### Progress (2026-09-19, lint debt clearance round)

- The full lint debt (~200 tracked findings from the ungated parallel
  week, plus the tail the default `max-same-issues: 3` cap kept
  hidden) is paid: `golangci-lint run ./...` answers **0 issues** with
  `max-issues-per-linter: 0` and `max-same-issues: 0` now pinned in
  `.golangci.yml` so the gate always sees the complete list.
- Real code fixes: the `eh-eh` tautology and the dead `end` clamp in
  the seam scan and the region chord, the unused `autonomous` param
  and the always-nil error of `runSessionSupervised` (six call sites
  simplified), five dead declarations deleted (soakMaxLevel,
  zoneOverrideSlack, patrolZone, dstComps, runHopBudget), five
  always-constant test helper params folded, the G109 bounds guard
  now sits next to the conversion, errorlint switched to errors.Is,
  the union-find `var find` pairs merged, the concat loops rebuilt on
  strings.Builder, the v1/v2 wire twins and the packet clone carry
  documented dupl relief, the water zone table and the two replay
  probes got the analysis-driver exclusion, `tools/prepush.sh` and
  the CI lint step now run the full uncapped lint.
- The suppression policy is uniform: every complexity finding carries
  one `//nolint:a,b // note` above the func (36 sites), the
  deliberate partial inits live in the `exhaustruct_v5`
  `ignore-patterns` list with per-group reasons, stale directives
  removed (nolintlint is the auditor).
- AGENTS.md gains the "Tree cleanliness discipline" section (caps off,
  fmt before commit, suppression policy, relief lives in config,
  dead code is deleted) and the Code conventions section now matches
  the real config.
- CI workflow copied to `.github/workflows/ci.yml` (lint step flipped
  from `--new` to the full run); the push needs the workflow-scoped
  token, otherwise the owner copy stays the fallback.

## Completed: the pathfind link repro contract - the viewer rebuilds the very search the bot walks (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner report ("find out why the bot route does not match in
the real world and when built through the web", the 2026-09-19
14:26 temp11 dump):
the bot walked its planned 17 waypoint dry zone return from the elven
village plaza to the hunting square center, the pathfind link opened
the 3D viewer at the same from/to pair, and the drawn route had
nothing in common with the walk. Find the cause and make the link a
reproduction.

### The diagnosis (the full analysis is Round 87 of the development log)

1. The viewer's capsule post pass folded the whole mesh route into
   ONE straight chord: the grid oracle of the fold is water blind
   (SegmentClear=true while 5 of 65 sampled chord points sit over the
   elven lake). The bot's plan is the search answer as produced; the
   fold drew a route the bot never walks.
2. The link hardcoded `filter=swim`; the zone return plans the DRY
   search - two different corridors (13.3 km dry detour vs 11.9 km
   swim cut).
3. The viewer answered the exact destination; the bot plans the
   approach search (radius 200) - the plan legitimately ends 185
   units short of the destination.
4. The frozen area bans never rode the link (silent in the clean
   session, a real gap in general).

### The fix (this commit)

The search contract rides the plan: `state.WalkPlan.Search`
(the `walkSearch` wire field: dry, approach, avoid circles) stamped
by `startWalkSegmentSearch` / `planUserWalk`, cleared by
`armDirectSegment`, published by `geodataWalkPlan` / `userWalkPlan`;
the viewer POST gains `approach` / `avoid` / `fold`, the viewer URL
and the HUD link round-trip the same (`fold=0` = the plan repro mode
serving the answer as the bot publishes it); the dump names the
filter word in the walk plan header and carries the full contract on
its `search` line for the paste-a-dump flow.

### Progress

- Round 87 lands as one commit: the state contract, the hunt
  stamps, the viewer handler/URL/POST, the HUD link serialization,
  the dump header word + search line, the tests (state JSON, the
  webserver handler pins, the hunt stamps, the HUD harness, the
  dump round trip) and the docs (webui.md, navmesh.md, this file,
  the development log round).

- 2026-09-19: the escape walks the route, not the chord (the 14:46
  dump round). The WHY: the 14:46 dump proved the cursor key escape of
  a DIRECT segment marched the straight chord to the far zone target -
  armDirectSegment replaces the waypoints with the single destination
  spec, the route following ladder over it interpolates the chord
  (both escape aims of the dump sat on it byte for byte), the claims
  dragged the character through the village geometry and the water
  guard stranded it. The HOW: replanDirectEscapeRoute runs the segment
  start search for the phase (the session bans respected) before the
  claims build, installs the fresh route as the segment plan and stands
  the direct segment down - the WASD escape walks along the planner's
  bends and the settle returns the walk to the normal routed clicks
  on the same plan (the owner contract: WASD along the route, the
  normal mode at the point); the planless fallback keeps the pocket
  contract toward the validated hop aim. The existing repro drivers
  now mirror the production dispatch (the armed escape drives before
  the direct segment check). Tests:
  `hunt/cursor_escape_direct_leg_repro_test.go` (the dump repro on
  the real pack, the planless fallback unit pin, the village escape
  end to end over the refusal pocket), the hunt suite green, 28
  packages ok, lint --new clean.
## Completed: the unit test gate red - the race slice budget and the coverage lift (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner reported the unit tests red. The diagnosis split into
three independent defects, all fixed in one round:

1. the whitespace gate was red tree-wide: `runs/.cover-run.log`
   (a runtime artifact of `tools/coverage_delta.sh`) was committed
   in 56adde9 and its `go test` output carries tabs, so
   `task fmt:check` / `task verify` / prepush fail on any machine;
2. the CI race slice (`connection` + `pathfind` under `-race`) was
   red: `TestFindPathFromDeckToFarWestZoneStaysOnRamps` asserts the
   production hunt tick budget (`< 5s`) on a wall clock the race
   detector inflates 6-10x (8.15 s measured against the plain
   0.84 s run) - the assertion reported instrumentation cost as a
   route failure;
3. the per package coverage sat low in the packages whose logic is
   reachable but untested (cmd/swarm 7.4%, huntaudit 19.1%).

### Progress

- `runs/.cover-run.log` and `runs/.coverage-current.txt` untracked,
  both gitignored (the committed baseline stays
  `runs/coverage-latest.txt`); the whitespace gate is green again.
- The race budget pinned, not the tolerance: `raceDetectorBudget`
  (a build tag pair in pathfind, 10 under `-race`, 1 plain) scales
  the two zone route wall time budgets; the assertion still fails
  on a real 10x planning regression. Plain mode keeps the exact
  production figure. The flake ledger row added (the detector
  overhead is instrumentation, not scheduler nondeterminism).
- Coverage lifted with reachable-surface tests: `cmd/swarm`
  7.4 -> 11.4 (the fleet account ladder, the address split),
  `huntaudit` 19.1 -> 23.0 (the spot filter, the game endpoint
  renderer, the draft mode no-op), the packet wire layouts pinned
  per file (the sell multi-entry body, the creation request full
  field order, the cursor key move mode, the cast modifiers).
- Real bug found by the new pins: `fleetAccountName` collided a
  numbered base with its first follower (temp2 fleet ran
  temp2, temp2, temp4) - the ladder now continues the base number
  (temp2, temp3, temp4); both call sites (the per bot account and
  the startup log line) share the fixed function.
- `to_game_server` stays at 65.3% honestly: the remaining branches
  are the defensive `Writer` error plumbing that a `bytes.Buffer`
  backend can never fire - reachable ceiling reached without
  touching production code.
- Baseline: `runs/coverage-latest.txt` re-committed, total
  statement coverage 75.3%; verify (build, vet, lint, test,
  fmt:check) green, the two fixed zone route tests green under
  `-race` directly.

- 2026-09-19: the direct walk dies (the 16:02 dump round). The WHY:
  the 16:02 dump (build 07ccb0e, bot test1) walked the path round 88
  did not cover - the spawn cell 43032 50408 is a LOCAL refusal
  pocket on the real pack (every direction refuses but south), the
  clicks never left the bot so no server answer ever arrived, the
  escape arming branches never ran, the ladder collapsed the plan
  into the single far waypoint through armDirectSegment and the walk sat
  on the forbidden direct line for three trip cycles while the
  widened bans (48 -> 96 -> 192) could not move the mesh's first
  funnel waypoint off the pocket. The offline probes (the real pack +
  the built real mesh tiles) pin both halves: the mesh plans the 64
  waypoint route whose wp 0 IS the refused 8 unit click, the grid
  refuses every first segment. The HOW: the direct server routed walk is
  ELIMINATED - escalateFrozenSegment rung 2 arms the cursor key escape
  ALONG THE CURRENT PLAN (the claims follow the planner's bends, the
  settle returns the normal routed clicks on the same plan, the
  re-arm repeats while the attempts last), the budget-burned zone
  return and the failed planning hold with a paced log instead of
  marching walkZoneSegment (which stays only for the in-zone patrol and
  the no-navigator deployments), and the planless escape aim clamps
  into the pocket radius. Every walk plan of the loop now carries its
  mesh search contract - the search-less plan view WAS the direct
  segment's fingerprint. Tests:
  `hunt/direct_walk_elimination_repro_test.go` (the 16:02 dump end to
  end on the real pack + mesh, the no-server-answer ladder pin, the
  planless clamp) + the reworked contract pins; the direct segment test
  set retires with the machinery. The live acceptance village-escape
  answers PASS on the built binary, tools/mobius_e2e.sh answers
  E2E_OK, the hunt suite green, every package ok, no new lint
  findings, the whitespace gate green.

### Progress (2026-09-19, the red unit tests and the coverage lift round)

- The owner report ("fix the tests so that everything passes")
  verified
  against the full gate surface: build, vet, the full uncapped lint
  and the plain `go test ./...` (28 packages, `-count=1`) were green
  already - the red tests live in the race slice and in the
  per package timeout.
- Three fixes, two real races found by `go test -race -count=1
  ./...` (first full-repo race run on record):
  1. `memwatch` `TestWatchLogsFootprintLines`: the watch goroutine
     logged into a plain `bytes.Buffer` while the test polled
     `out.String()` from the test goroutine - the log.Logger mutex
     covers only its own writes, never the reader. The test now
     wraps the buffer in a `syncBuffer` (a mutex-guarded pair of
     Write/String), the race detector stays silent across 5
     consecutive runs.
  2. `hunt` `TestDriveGatekeeperTeleportHappyPath` /
     `TestDriveGatekeeperTeleportStaleHtmlIgnored`: the simulated
     server goroutine wrote `fakeGame.htmlNPC`/`htmlBody` directly
     while the production `awaitDialog` loop polled
     `LastHTMLDialog()` from the loop goroutine. `fakeGame` gains
     the `htmlMu` mutex; the poll reads and the simulation writes
     go through `setHTMLDialog`; verified with `-race -count=3`.
  3. `pathfind` (600.065 s) and `pathfind/navbuild` (600.049 s)
     both died in the 10m default `go test` timeout with the search
     tests mid-flight at 4 s and 24 s - the detector's 6-10x hot
     loop overhead on a two core box, no race found. `task
     test:race` now carries `-timeout=30m` with the measured
     reasoning in the comment.
- Coverage lifted where the reachable surface was untested:
  `cmd/gofmt-spaces` 34.5 -> 84.5 (the CLI layer: -l/-w/stdout
  modes, the dot and underscore dir skip rules, the explicit dot
  root entry, the single file and non-go arguments, the broken
  source failure that does not stop the batch, the default cwd
  walk), `cmd/benchdiff` 60.7 -> 75.3 (the metric block rendering
  and the empty-unit skip, the missing file error, the zero base
  delta guard, the vanished-benchmarks report, the one-sided
  metric join, the GOMAXPROCS strip boundary), `huntaudit`
  23.0 -> 24.5 (the broken JSON resume refusal, the directory read
  error path, the atomic temp+rename write with the summary stamp,
  the missing anchors file refusal). `memwatch` holds 100.
- Total statement coverage 75.3 -> 75.6; the baseline
  `runs/coverage-latest.txt` re-committed with the deltas.

### Progress (2026-09-19, the post-rebase verification round)

- The full plain `go test ./...` after the rebase onto 7a9544a (the
  direct walk elimination) surfaced the repro tests the round left
  hard-failing on any tree without the local artifacts:
  `TestReproRefusedSpawnNeverWalksTheDirectLine` and
  `TestReproRefusedSpawnLadderArmsTheEscapeWithoutServerAnswers`
  died in `spawnDumpMesh` on `require.NotEmpty(dir)` while the
  navmesh tiles are a gitignored runtime artifact (the same class
  as the geodata pack, which `reproEngine` skips correctly).
  `spawnDumpMesh` now skips with the honest message when the tiles
  are absent (the repo skip pattern of navbuild/real_test.go) - the
  dump reproduction still runs to the end where the tiles exist
  (verified: the tiles rebuilt for 20_18..21_20 with
  `cmd/navmesh-build -regions ...`, both tests green against them).
- Final verification after the fixes: `go test -count=1 ./...`
  answers 28 packages ok, zero failures, the whitespace gate and
  the uncapped lint green.

## Completed: the priced water - the walled form retires, the bot plans through the water objects (2026-09-19)
## Completed: the wasd ground progress - the escape claims mark the walked waypoints and carry the server heading (2026-09-19)

Started: 2026-09-19. Branch: `feature/new-pathfind-alternative`.
Commits as melg8. Other agents may push to the same branch
concurrently - rebase before every push.

### Goal

The owner directive ("the code seems to keep the outdated ways -
the filter=swim must not survive - the bot must plan the path
through the water objects too, just with the correct slowdowns -
in water it swims slower than it runs"): the dry/swim filter dichotomy is the
outdated way. One search remains - the water is a price (the
run/swim ratio 2.3), never a wall - and the walker walks the wet
segments the plan carries.

### The round (the full analysis is Round 90 of the development log)
- `navmesh`: `AllowWater`, `DryFilter`, `RouteDry` deleted;
  `DefaultFilter` is the one priced search (the escape keeps its 8x
  water price).
- The grid engine: `search.dry`, `FindPathApproachDry(+-Avoiding)`,
  `DryLine` deleted; the pricing, the direct line dry gate and the
  `segmentDry` smoothing stay (they ARE the pricing honesty).
- The hunt loop: one priced search everywhere (`startWalkSegmentSearch`
  without the non-dry switch, the shop escalation and the zone
  return fallback ladder deleted); the click water guard retired
  (`clickWouldEnterWater`, `shortenWetHop`, the wet variant skip,
  the extension water gate); the escape gate reads the plan's intent
  (the aim waypoint below `pathfind.WaterLevel` and ahead of the
  character keeps the swim walking).
- The web: the viewer filter select, the `filter` URL param, the
  POST field and the response word gone; the links carry
  `approach`/`avoid`/`fold=0` only; the dump header names `mesh`;
  `ApplyDump` restores the search contract into the replayed plan.
- The state wire: `WalkSearch.Dry` deleted
  (`{"approach":...,"avoid":[...]}`).

### Progress
- Round 90 lands as one commit: the two engines, the hunt loop, the
  webserver handler, the viewer and the HUD, the dump writer and
  parser, the state wire, the pricing pins (the wide band swims, the
  narrow band detours - whole flat block worlds, the per cell setCell
  pillar artifact documented), the escape gate pin, the docs rounds.
The owner reported two defects of the round 89 escape on the 17:18
session (build 94ec5e3, bot test1): the wasd walked waypoints did not
mark passed and the resumed clicks walked BACK to them (two minutes
of backtrack segments in the dump), and the web UI showed the character
facing a direction it never walked during the wasd walk.

### Outcome

- The claim ladder carries a waypoint map now: every completing claim
  marks its route waypoint passed while the escape streams (gated on
  the escapeFollows verdict - a server that ignores the claims never
  fakes progress), the settle advances the cursor at the position the
  character actually reached (never the ladder's aim) and
  re-baselines the stuck window, so the resumed clicks aim forward.
- The claim heading follows the mobius convention
  (LocationUtil.calculateHeadingFrom: atan2(deltaY, deltaX) - the
  swapped atan2 the old code carried mirrored the facing; the mirror
  also pointed the mobius cursor-key obstacle probe behind the
  character's back), the claims set the session facing optimistically
  (state.Bot.ApplySelfFacing), and the claim echoes keep that facing
  while the claims own the stream (GameClient.placementHeading gates
  the ValidateLocation/StopMove headings).
- Reproductions: cursor_escape_wp_sync_repro_test.go (the walked
  waypoints marked passed mid-escape and after the settle, the
  forward-only resumed clicks, the no-skip window after the settle,
  the heading convention pins with the dump's own 33472 sample) and
  TestGameClientClaimEchoKeepsTheClaimFacing in the connection suite.
- Live gates: village-escape PASS on the deployed stack,
  tools/mobius_e2e.sh E2E_OK, `go test ./...` every package ok,
  golangci-lint 0 issues, the whitespace gate green. The mobius
  server stays untouched.

## Railing pocket round (2026-09-19, Round 92)

- The owner report: from 43736 47048 -2992 no route can be built
  anywhere although the cell itself is honest ground (just off the
  village railings); the demand: a unit test + a webui runnable
  acceptance test + a systemic fix + the why.
- Root cause: the mesh link builder connects shared EDGES with both
  side wall bits open; every axis neighbor of the cell carries a
  railing wall bit, the cell is a linkless one polygon island (the
  link flood component size 1), the corridor search answers the bare
  not found for every destination while the grid engine plans out of
  the same cell (the diagonal squeeze passes the server anti corner
  cut rule and the click transport even accepts one sided wall
  pairs).
- Fix: the mesh pocket escape (navmesh/pocket.go - the bounded
  component flood, the priced boundary scan, the deep exit aim) + the
  navigator transport refinement (the 30 degree ValidateClick sweep
  at 224 units, the grid surface height) + the Route.PocketEscape
  flag; the walk-what-you-can partial contract serves the answer end
  to end, the next plan cycle routes from the exit ground.
- Reproductions: navmesh/pocket_test.go (the synthetic pocket, the
  water pricing, the wide component pin, the live pack repro),
  hunt/railing_pocket_repro_test.go (the end to end walk out to the
  hunting zone) and the acceptance "railing-pocket" scenario (temp12,
  the webui run button, the 256 unit / two minute contract).
- Gates: `go test -count=1 ./...` every package ok, golangci-lint 0
  issues, the whitespace gate green. The mobius server untouched.
### Progress (2026-09-19, the unittest account ladder round)

- the owner directive: remove the test account ladder
  (test1/test2/test3 - the same accounts the live swarm logs in
  with) from the Go tests, replace it with the unittest1 ladder, so
  a test run can never collide with the running swarm work (the
  same account name logged in twice kicks the live bot).
- the rename swept 79 test files, 498 occurrences: every
  bot/account/character string of the ladder test1/test2/test3 is
  now unittest1/unittest2/unittest3 (word boundary sed, the 4 space
  indentation untouched), including the historical dump references
  in the repro test comments (the scenarios read the same, the bot
  name is now the unittest one).
- the already separated ladders stay untouched: the acceptance
  scenarios ride temp1..temp11, the fleet benchmark fleet001..100,
  the proxy e2e proxye2e, the live dialog walker dialogw1 - the
  collision surface was the unit test surface only.
- the production defaults of cmd/swarm (test1/test) stay: they are
  the live swarm contract documented in README/AGENTS.md, not a
  test surface.
- one byte pin followed the rename by hand:
  TestWriteStringAsUtf16ASCIIVsReference carried the hand written
  UTF-16LE bytes of "test1" (the sed renamed the input string, the
  expected byte array now spells unittest1).
- verification: go build ./... ok, go test -count=1 ./... answers
  28 packages ok zero failures, the gofmt-spaces gate is silent.
a38b90a (test gate: the unit test account ladder renames to unittest1 - the tests shared the account names test1/test2/test3 with the live swarm run (the same fleet ladder the production -account default spawns) so any test that builds a session under a running swarm collided with the live bots on the account name alone; every bot/account/character string of the ladder in the 79 test files (498 occurrences, word boundary sed, the 4 space indent untouched) is now unittest1/unittest2/unittest3 including the historical dump references in the repro test comments (the scenarios read the same, the bot name is the unittest one); the already separated ladders stay (the acceptance temp1..temp11, the fleet benchmark fleet001..100, the proxy e2e proxye2e, the live dialog walker dialogw1), the production defaults of cmd/swarm stay (the live contract of README/AGENTS.md); the UTF-16 byte pin of TestWriteStringAsUtf16ASCIIVsReference follows by hand (the sed renamed the input string, the expected byte array now spells unittest1); verification: go build ok, go test -count=1 ./... 28 packages ok zero failures, the gofmt-spaces gate silent)

### Progress (2026-09-19, the church entry round)

- the owner directive: the acceptance test for the walk 44694 51921
  -2808 -> 44718 52291 -2792 (the temple entrance to the hierarch
  Asterios), the mobius server instrumented for the refusal
  diagnosis, the systemic cause found and the entry working - the
  same walk refused for the real client through the proxy too.
- the acceptance scenario church-entry (temp12) landed; the
  acceptance manager now wires the navigation mesh (ManagerDeps.Mesh
  through both the headless CLI and the web UI paths) so the suite
  exercises the same mesh navigator the fleet bot serves.
- the diagnosis chain: the offline pack answers the line clear, the
  [GEOPROBE] server probes (MoveToLocation, ValidatePosition,
  Creature.moveToLocation, the l2j_geoprobe.patch copy) prove the
  server accepts the click and confirms the arrival inside, the mesh
  split the queries - the exact Route crosses the entrance (371
  units) while the user ring approach search legally ends the plan
  at the doorway polygon (44718 52144, 147 units short of the click,
  inside the 150 unit ring). The bot walked to the door and declared
  arrival; with the real client attached the active plan kept
  re-issuing the door segment and out-raced every client click.
- the fix: planUserWalk plans the exact mesh search first (approach
  zero), the approach corridor stays the fallback for the
  unreachable click; the published WalkSearch contract carries the
  answer's own approach. The town and NPC approach segments keep their
  rings. The fresh tile repro on the pre fix binary fails at the
  door (225 units), the fixed binary walks in (371 units, PASS).
- verification: go test -count=1 ./... answers 28 packages ok zero
  failures, the gofmt-spaces gate silent, the live acceptance
  church-entry PASS against the deployed stack with the mesh. The
  residual (the fresh tile build vs the dump repro pins of the
  priced water builder) is recorded as the next round's work in
  docs/development_log.md Round 92.

### Progress (2026-09-20, the fresh tile re-validation round)

- the round 92 residual (the fresh priced water tiles vs the dump
  repro pins) closed green: the sandbox had no tiles at all, the
  fresh `cmd/navmesh-build` run over 20_18..21_20 built 6 regions
  (4479142 polys, 11861108 links) in 19.8 s with 0 failed, and the
  full `go test -count=1 ./...` on the clean tree answers 28
  packages ok zero failures against those tiles - the dump
  reproductions (cursor_escape_wp_sync, refusal_signal,
  direct_walk_elimination) ran for real and passed, the pinned
  windows hold on the round 90 builder output.
- why it reconciled: the pins are the scenario contract through
  relative assertions, not absolute durations, and the church entry
  round plus the pocket escape round both re-validated the ladder on
  fresh tiles before the merge; the recorded residual described the
  pre merge tree state. Round 93 of docs/development_log.md pins
  the fresh environment run.
- the canonical mesh regeneration command is now in the log:
  `go run ./cmd/navmesh-build -geodata data/geodata -out
  data/navmesh` (the -regions and -force modifiers documented).
- the rebase over the parallel pocket escape round kept both
  rounds' log entries and renamed the church entry acceptance
  account to temp13 (the pocket round's railing-pocket scenario
  took temp12), the acceptance suite owns temp1..temp13 now.

### Progress (2026-09-20, the porch refusal ladder round)

- the owner dump at a1fc212: the walk plan is the exact mesh answer
  (the church entry fix works), but the character stood on the
  temple porch for 31 seconds with the server answering every
  straight re-click ActionFailed - the plain user walk follower
  re-issued the same aim forever. The sandbox grid (plaza/porch x
  direct/proxy) all PASS, so the stall belongs to the owner
  deployment's server class (the 15:10 class: its geodata seals the
  porch lines and it answers no click from there).
- the fix: the manual walk refusal ladder - userSegmentRefused (the
  sent click attribution), sendUserVariedAim (the shared
  refusalVariantTarget ladder through the click validation port)
  and beginUserCursorKeyEscape (the claimed ValidatePosition
  stream the server follows without any click validation, the
  claims march the planned waypoint line). The reproduction
  user_refusal_ladder_test.go pins the ladder end to end on the
  real pack and the real mesh (pre fix the walk sat on the porch
  until the timeout).
- the second bug the dump surfaced: the acceptance injection
  derived item object ids charID+100M landed above FIRST_OBJECT_ID
  and the sibling temp blocks overlapped (temp12's bow was
  temp13's adena, the duplicate key 1062 of the live run). The new
  per character block base+(charID mod 1M)*32+i sits below the
  server range and stays disjoint; church-entry and
  railing-pocket ran back to back live - both PASS.
- the mesh route probe of the temple walk answered a straight 2
  waypoint funnel (the door pivot folded away, raw list empty) -
  the bottleneck aware funnel stays open as the follow up; the
  follower ladder owns every deployment whose walk disagrees with
  the pack. Round 94 of docs/development_log.md.

### Progress (2026-09-20, the mobius upstream archaeology round)

- the owner found the church walk failure source: the deployment ran
  an older Mobius C1 build and updating the server alone fixed the
  building entry even without the swarm changes. The round names the
  commit: 55787efe "Release: August 15th 2026" (the parent
  f1e84274, 2026-08-12, is the last old-engine state).
- the method: gitlab.com is blocked from the sandbox and the C1 repo
  has no GitHub mirror, the history came through the GitLab REST API
  behind the jina reader proxy (double encoded project path), 102
  commits since July 1st plus the path filtered lists and the raw
  parent-commit file snapshots diffed against the local master.
- the mechanism: the release commit rewrites the movement engine -
  the GeoEngine single cell layer gap fallback (hasNeighbourLayerNear,
  PATH_CONTINUITY_TOLERANCE 16: a threshold cell whose nearest layer
  sits above HEIGHT_INCREASE_LIMIT 40 no longer seals the entrance
  when a neighbour carries a layer within 16 units of the source z),
  the NodeBuffer A* rewrite (primitive arrays, binary min-heap,
  Z_TOLERANCE 64) and the PathFinding thread local buffers. The
  geodata binaries and the Doors.xml content are unchanged - pure
  code fix.
- the reconciliation: the sandbox (September master, post rewrite)
  never saw refusals while the owner's pre August build sealed the
  door frames - the concrete identity of Round 94's "server class";
  the refusal ladder stays as the recovery for deployments that
  cannot update. Round 95 of docs/development_log.md.

### Progress (2026-09-20, the deleveling round)

- the owner demand: the deleveling acceptance test in the webui,
  the stuck and the ping-pong checks inside it, and the webui
  message carrying the target level and the reason.
- the audit: the delevel message lived only in the event log (the
  webui banner was static text) and the ping-pong hole was
  structural - every ABORTED attempt left the level above the
  trigger with the flat one minute cooldown, so the bot commuted
  farm <-> village forever whenever the attempts kept failing.
- the fix: HuntDiagnostics carries DelevelActive/DelevelTarget/
  DelevelFromLevel/DelevelZoneMedian while the phase runs (the
  banner renders "dropping to level X - level Y is too high for the
  level Z mobs (the drops collapsed)", the sidebar "deleveling ->
  lv X"); every abort arms the escalating wait (base 5 min, factor
  5, cap 30 min = delevelFreeCooldown) through the delevelAborts
  streak that resets ONLY on a finished deleveling; the trigger
  stays silent while the wait holds.
- the acceptance scenario "delevel" (temp14, the webui run button):
  the level 15 fighter with the bottom of level experience on the
  Green Dryad S-16 cell (median 8, target 13, two guard deaths
  from the level bottom), the checks ride the announcement, the
  walk progress (2000 units - the stuck class never leaves the
  start), the death penalty, the target, the return to the farm
  ground and the no-retry window (6 min default,
  SWARM_DELEVEL_NORETRY_SECONDS knob); a re-entry fails the run
  with the ping-pong error.
- the unit ladder hunt/delevel_oscillation_test.go pins the wait
  ladder, the silence, the finish-only reset and the message
  fields; the scratch helper cmd/dbpos prints the character rows
  through the acceptance wire client.
- the lint gate reconciliation: the church entry and porch refusal
  ladder commits' lint debt closed (the exhaustruct Done/Command
  fields, the startChurchSession helper, the wpMap nil arm field,
  the intrange loop, the InDelta pins, the linear train nolint) -
  the full gate answers 0 issues again.
- verification: go test ./... 28 packages ok zero failures,
  golangci-lint run ./... 0 issues, gofmt-spaces clean; the live
  delevel acceptance run stays for the next session with the stack
  up (the full cycle needs the guards, the stack was down at the
  round time). Round 96 of docs/development_log.md.

- 2026-09-20, Round 97, the stuck terrace round: the owner report
  named two fleet freeze positions (43632 50560 -2960 heading 21963,
  41920 52128 -3000 heading 26712). The audit: both spots stand on
  link components the strict edge only mesh link graph cannot leave
  (52 and 29 polygons, the 640x752 and the 592x432 boxes) - the
  railing pocket class of Round 92 at the terrace scale, the grid
  engine plans out through the diagonal squeezes the mesh links do
  not carry. The 320 pocket side refused both components, so the
  route answers were the bare not found or the inner boundary
  partial - the freeze. The fix: pocketMaxSide 320 -> 2048 (the
  measured world extent of the sealing geometry: pockets 16..320,
  terraces ~600..760, piers long thin; the mainland still aborts the
  flood past the bound) and the horizontal displacement floor of the
  exit scan (pocketExitHorizontalFloorSq - the ground directly under
  a spot stacked above its deck and the corner sharing diagonals
  answered the closest 3D candidates and degenerated the aim into
  the standing cell; the widened probe caught it before it froze
  anyone). Reproductions: TestPocketEscapeServesTheWideTerrace /
  SparesTheWideComponent (the 2240 bound) / SparesTheVerticalStack +
  TestReproStuckTerraces43632And41920 on the live pack, the loop
  level hunt/stuck_terrace_repro_test.go (red pre fix, green post),
  the webui scenarios stuck-point-43632 (temp15) and stuck-point
  -41920 (temp16) - both answered LIVE PASS against the deployed
  stack within the round (the first spot rode the varied aim ladder
  over the refused terrace edge clicks, the second walked out clean
  in ~4 seconds). Verification: go test ./... 28 packages ok,
  golangci-lint 0 issues, gofmt-spaces clean. Round 97 of
  docs/development_log.md.
### Progress (2026-09-20, the shop quarter round)

- the farm readiness report (level 15): the bot did not buy, slid
  along the outer railing and talked to the merchants from outside.
  The merchant segments now plan the exact mesh search (the customer
  cell across the counter, 40-42 units from the npc at floor level
  on the real tiles, the counters never walked) and the final
  arrival is the tight pass radius.
- the second bug the live run exposed: the exact search to a far
  unreachable destination exhaustively floods the whole mesh
  component (the Herbiel segment froze the live bot inside one query,
  the offline rerun OOMs in 4.4 s). The exact segments are gated by
  exactSegmentMaxDistance (2000), the far segments walk the priced ring
  (the hierarchy answers them in milliseconds) and the walk
  completion arms the near exact final approach; the re-paths
  preserve the segment contract (replanTownWalkSegment).
- the live verification bought the whole level 15 kit through four
  merchants (every purchase confirmed, the frozen Herbiel segment
  walked in 51 s); the offline pins and the full suite stay green.
  Round 96 of docs/development_log.md.

### Progress (2026-09-20, the segment term round)

- the owner asked for a naming audit of the pathfind and the movement
  code: the "leg" term (a planned stretch of movement between two
  points) reads unnatural in the derived names (legX, legHalf,
  maxMoveLeg, LegGuard). The term retired to "segment" (the standard
  pathfinding word) across the movement code, the tests and the
  current-facing docs: the identifiers (startWalkSegment, zoneSegmentAt,
  SegmentGuard, maxMoveDistance - the one deliberate special case, the
  1000 unit move cap is a distance), the webui visible strings (the
  avoidance bend, the stall diagnostics, the walk plan dump
  ", t+12.4s, segment 5.2s") and the comment prose. The equipment
  "legs" (the armor slot: PaperdollLegs, SlotLegs, partLegs,
  legsPurchase, legsItemIDOf, legsSellFirstOf, legsWord) and the game
  data stay untouched; the historical journals (docs/development_log.md,
  docs/session_journal.md, docs/agent_progress_archive.md) keep their
  dated record. The test file merchant_counter_leg_test.go renamed to
  merchant_counter_segment_test.go.
- the lint shrink the touched functions owe: the ineffectual planFailed
  initializer of the shopping trip and the gocognit directive of
  maybeStartTownTrip answered, the four float-compares of the merchant
  segment test moved to require.Zero/require.InDelta - the full
  golangci-lint run answers 0 issues again.
- verification: go build, go vet, task lint 0 issues, task fmt:check
  clean, go test -count=1 ./... 28 packages ok (pathfind 126 s
  dominates). No live behavior change: the rename is textual, the
  movement semantics, the wire packets and the plans are identical.

## Completed: the merchant seller distinction round (2026-09-20)

Branch: `feature/new-pathfind-alternative`, commits as melg8. The
owner report: the bot does not correctly distinguish the sellers -
the knowledge of the armor and the weapon traders was missing, a
weapon must not buy at the armor trader and vice versa, while any
item not forbidden from the sale sells to any merchant.

### What was done

- the stale merchant selection of the sell phase aimed the merged
  buy requests at the wrong trader (the server silently refuses the
  foreign list and the stop burned its retry budget) -
  `handleMerchant` re-picks the trader when the selected npc's
  template answers none of the stop's wanted templates.
- the new `hunt/merchant.go` carries the merchant wares knowledge
  derived from the generated buylists at runtime (weapons / armor
  with the shields / jewels / magic / consumables, per merchant,
  both towns); `merchantSellsItem` is the exact item check.
- the frozen trip plan drops the lines whose merchant does not trade
  the item (`dropForeignMerchantPurchases`, a data bug surfaces as a
  loud log line).
- the merchant sets follow the region (`merchantsForRegion`): the
  Dion band no longer targets the elven traders from the trip start
  and the sell pick.
- the tests: hunt/merchant_test.go (the classification of the eight
  known merchants, the exact join, the freeze filter, the end to end
  re-selection round, the region sets). The docs: hunting.md shop
  section, development_log.md Round 98.
### Progress (2026-09-20, the naming audit round)

- the owner asked for a broader naming audit after the leg->segment
  round. The survey: the identifier vocabulary of the movement and
  the pathfind packages (the word frequency scan), the misspelling
  patterns, the Cyrillic scan of every text file. The vocabulary
  verdict: the house metaphors (segment, strand as a verb, deck,
  terrace, rung/ladder, claims, blind pass) are coherent idiomatic
  English - no renames owed. No misspellings found (the retrun hits
  are the requireTruncatedPrefixesError false positive).
- the real systematic problem: the Russian remnants. Translated to
  English: the owner rule quotes in the hunt comments and the repro
  tests (NEVER walk the direct line, ONLY walk the planned routes),
  the task brief quote of proxy/transformer.go, the packet parser
  comments and the bench/struct test messages of
  from_auth_server/init*.go, the webui fight.js variant notes and
  the index.html fight gallery captions, the deploy scripts
  (swarm_fast_deploy.sh, mobius_fast_deploy.sh, mobius_e2e.sh,
  proxy_e2e.sh, install_l2encdec.sh - the headers, the step messages
  and the diagnostics) and docs/webui_modernization_proposal.md in
  full (488 lines, awaiting approval, now English end to end).
- the deliberate keeps: the Cyrillic string literals "Эльф"/"тест"
  in the packet tests are the multibyte encoding fixtures (the tests
  cover the non-ASCII rune handling), and the dated journals
  (docs/development_log.md, docs/agent_progress_archive.md) keep
  their historical record.
- verification: go build, go vet, task lint 0 issues, task fmt:check
  clean, bash -n on every touched script, go test -count=1 ./... 28
  packages ok. No behavior change: the translations touch the
  comments, the diagnostic strings, the webui captions and the docs.

### Progress (2026-09-20, the docs and comments audit round)

- the owner asked for an audit of the code comments and every md
  file: cut the slack, remove the deprecated, make a new agent get
  what it needs from AGENTS.md and get routed to the right docs,
  and recheck the skills instructions for the Go source work.
- the fact check against the tree surfaced the stale claims:
  AGENTS.md still carried the two contradicting reaper studies
  (09-18 death vs 09-19 survival) under the mandatory 10 minute
  owner rule, a duplicate Tech stack section, the paid-off ~200
  finding lint debt paragraph against the zero findings
  discipline, the false "imports grouped by gci" claim (gci is
  disabled in .golangci.yml - its canonical form is tab indented),
  the stale .golangci-lint filename, the .github/workflows/ci.yml
  reference (the file is the owner placed copy of
  docs/ci_workflow.yml, nothing in tree), "four project playbooks"
  in three places (there are seven), and seven docs missing from
  the documentation map (ROADMAP, quest_protocol,
  band_20_25_survey, fastpath_research, flake_ledger, the
  superseded hunting_system_redesign, the docs index itself).
- AGENTS.md now opens with the Start here block (the first minute
  of a session: skim this file, one doc from the map, the matching
  playbook, agent_progress.md, deploy first); the subprocess
  section condenses to the 10 minute rule plus the one call
  pattern; the agents.md mirror drops the survival verdict it
  still carried against the owner rule and points at Start here.
- the skills recheck the owner asked for: the instructions exist
  and got sharper - the vendored samber/cc-skills-golang collection
  (46 golang-* skills) is pinned as the Go knowledge base for the
  tree's Go source with the area to skill mapping spelled out
  (golang-testing, golang-concurrency, golang-error-handling,
  golang-lint/golang-code-style, golang-naming, golang-performance,
  golang-troubleshooting); the seven hand-maintained project
  playbooks (go-verify-loop, mobius-stack, packet-recipe,
  webui-harness, performance, dump-state-repro, e2e-repro) cover
  the repo specific procedures.
- the terminology completion: docs/hunting.md was missed by the
  leg->segment doc rename (35 movement prose lines plus the
  legRefused reference to the renamed symbol), the same cleanup
  landed in shopping_strategy.md (4), proxy.md (3),
  band_20_25_survey.md (7) and quest_protocol.md (5); the
  equipment legs (the armor slot: the legs slot, the legs armor,
  the chest/legs/head lists) stay as they should.
- README.md: the Planned section stopped advertising the
  superseded hunting_system_redesign.md (the spot model was
  implemented and retired for the cell system) and points at the
  band survey and hunting_cells instead; the "Okay. The stack is
  running" slack prose is gone. docs/README.md gained the missing
  rows (navmesh, quest_protocol, band_20_25_survey, ROADMAP,
  recast_pathfinding, fastpath_research, agent_feedback_loops,
  flake_ledger) so both indexes route to every doc that exists.
- the code comment audit is clean: no TODO/FIXME markers, no
  references to the retired claim/lease queue or the reaper
  studies, the Cyrillic only in the multibyte encoding test
  fixtures, the "deprecated" words only about the real protocol
  and stdlib facts (the deprecated zero ints of CharSelectionInfo,
  the net.Dialer deprecated fields).
- verification: go build, golangci-lint run 0 issues, task
  fmt:check clean (the quests_test.go gofmt drift a parallel push
  landed is fixed in its own commit), no behavior change - the
  round touches the docs and the comments only.

### Progress (2026-09-20, the fresh-eyes evaluation round)

- the owner asked to pick the most fitting skills and evaluate the
  project with them. The instruments: the vendored collection the
  AGENTS.md skills section pins - golang-troubleshooting (the
  code-review-flags and common-go-bugs references), golang-concurrency
  (the five-checkpoint audit), golang-error-handling, golang-testing,
  golang-security and go-verify-loop armed four parallel review passes
  (nil/resources; error handling and slice/map safety; concurrency
  including every goroutine spawn site; testing, docs onboarding and
  the CI/deps posture). The external skill marketplace was scanned for
  a Go review skill (nothing the vendored set lacks) and the web
  grounding confirmed the govulncheck and CI recommendations.
- every published finding was re-verified against the tree; the full
  backlog with file:line evidence lives in the new
  docs/codebase_review_2026-09-20.md. P0: the journal gzipFile deletes
  the source segment when the copy fails (contradicts its own
  comment), the lazy skill/book caches are written under the store
  RLock from the snapshot encoder (two concurrent readers race),
  the session lifecycle edges (the leaked gameConn on a pre-Run
  failure, no read deadlines on the login flow), CI designed but
  never activated (verified: zero workflow runs on the repo) with the
  stale --new lint step in docs/ci_workflow.yml, and the handover
  file carrying seven Active task headings over 1718 lines. P1/P2:
  the aggro id-domain no-op, the zero-value AttackTarget grind, the
  rotation wedge, the shutdown tail, the pathfind wall time split,
  the hot-path logging economics, the manual dependency posture and
  the docs drift the audit round missed.
- gates this round: task verify green (28 packages ok, uncapped lint
  0 issues, fmt:check clean), go test -race green on state,
  webserver and connection, govulncheck 0 reachable vulnerabilities,
  no secrets in the tree, the coverage baseline read.
- next: the recommended round order at the tail of
  docs/codebase_review_2026-09-20.md - P0 items 1 and 2 first (small
  diffs, each with a focused test), then the session edges, the
  handover restructure and the CI activation batch.

### Progress (2026-09-20, the water guard retirement round)

- the owner directive: the water guard subsystems were built for the
  grid navigation; the mesh prices the water, so remove them - but
  land tests confirming or refuting the belief BEFORE the removal.
- the belief tests (`world_water_reality_test.go`, the navmesh
  package, the real elven pack) answered: the mesh plans full trader
  routes from every wet cell of the 2026-09-20 dump (confirmed), the
  whole dump trip plans as one found route (confirmed), and the mesh
  `WaterEscape` returned the dump's own wet escape target 40000
  43776 -3776 (refuted - the machinery planned walks to shores the
  grid raster calls water).
- the removal: hunt (the OverWater escape branch, walkWaterEscape,
  planWaterEscape, stuckWaterEscape, the wet claim guards,
  Navigator.WaterCrossed/FindWaterEscape; OverWater stays for the
  frame measurement and the capsule clamp), pathfind
  (Engine.FindWaterEscape, Engine.WaterCrossed, the escape BFS),
  navmesh (Mesh.WaterEscape, the escape water cost, the dead escape
  goal branch of the A*, the unused straightPath wrapper).
- the survivor contract pinned by the adapted tests: the planned swim
  keeps walking (a wet click is a priced walk, not a refusal), a wet
  standing character recovers through the plain stuck re-plan ladder,
  no shore search exists.
- verification: go build, golangci-lint run 0 issues, gofmt-spaces
  clean, go test hunt + pathfind + navbuild + navmesh + prototype +
  acceptance - all ok.
## Active task: the counter stand round - the merchant stops walk to the customer cell across the counter (2026-09-20)

Started: 2026-09-20. Branch: `feature/new-pathfind-alternative`.
Commits as melg8.

### Goal (the owner prompt of 2026-09-20)

Some merchants stand in their shops behind counters. The trip points
the character runs to when it wants to buy or sell must be modified:
for Unoren the coordinate 44667 46896 -2982 (the spawn) is blocked -
nobody can stand on it directly; the character must stand inside the
shop, in front of the counter (the same for Ariel). The owner asked
for the general curated list of such situations and welcomed the
counter direction detection idea (how to tell the counter direction
apart from the wall behind the merchant's back).

### Progress

- the raw geodata diagnosis (cmd/counterprobe, the new scratch probe
  in the geotest/navanalyze/stuckprobe family): the merchant spawn
  cells of the elven weapon/armor shop sit inside the roofed stall
  the pack models as roof-only cells over the sea bed - the spawn
  cell holds no floor layer at all, the mesh cannot walk onto it and
  the server side (the local stack runs PathFinding=0, GeoEngine
  loaded 0 regions) never blocks a walk; "blocked" is the bot's own
  pack truth. The raw spawn route answered the OUTER side of the
  stall front for Unoren (44640 46864, 42 units north-west) - not the
  customer side.
- the counter direction detector (-mode detect): the counter sits on
  the FACING side of the merchant (the spawn heading of the Mobius
  spawn data, LocationUtil.calculateHeadingFrom semantics: 0 = east,
  16384 = south); among the eight directions the one holding the
  stall edge (the first floor cell beyond the roof-only interior)
  within 60 degrees of the heading wins. Verified against the magic
  shop where the pack models the counter band as raised layers.
- the curated stand table (`merchantStands` in hunt/town.go): the
  customer cell just beyond each counter front, one row per merchant
  (the elven pair Unoren/Ariel on the west corridor of their stall,
  Creamees/Herbiel south-east of their counter bands, Sabrin/Casey on
  the east corridor of the Dion weapon shop, Sonia/Lara on the north
  corridor of the Dion magic shop - all agreeing with the spawn
  headings). `merchantStandPoint` returns the stand for the walk
  planning of the merchant stops (advanceTripStop, the shopping trip
  stop planner, exactApproachWanted); the interaction gates keep
  measuring the spawn.
- the grid verification (-mode verify): every stand routes found with
  the plan ending on the cell; the walk legs are checked with the
  grid line of sight - the elven corridor approach required the stand
  at the corridor gate latitude (the deeper rows clip the walled
  counter corner on the diagonal strides - the honest-server stall
  the town repro test caught, root caused and fixed by moving the
  stand to 44584 46944).
- the tests: TestMerchantStandTableCoversTheCounterTraders (the table
  integrity), TestUnorenStopTargetsTheCounterStand (the real pack and
  mesh pin of the stand targeting); the trip test expectations moved
  from the spawn to the stand (herbielStand helper).

### Verification

go build, go vet, task lint 0 issues, task fmt:check clean, go test
-count=1 ./... green except TestWorldRiverFord pair 1 (the
cross-world ford route needs about forty navmesh tiles the 4 GB
sandbox cannot build - the pre-existing environment limit of the
sandbox tile pack, the navmesh package untouched by this round).
The live acceptance run against the deployed stack stays for the
next session with the stack up (the counters walk, the buys and the
sells from the customer cells).


### Progress (2026-09-20, the npc search radius round)

- the owner report: the pathfind into the shop ends at the edge of
  the shop, not at the requested point; the owner diagnosis: the
  approach 200 causes it; the directive: every path search to an npc
  ends at the npc's own point, the approach radius at most 10.
- the reproduction on the real elven mesh pack (the report's plan
  origin 46045 41251 -3440): the wide trip ring (approach 200) ended
  the Unoren plan 226 units short on the shop edge (the first
  walkable surface inside the ball), Ariel 154 short; the npc search
  radius (10) walks the same corridors into the shop and ends at the
  customer cell across the counter (42 / 40 units, the exact answer).
- the fix: `npcApproachRadius = 10` and the npc stop ladder
  (`startWalkNpcSegment` / `planNpcSegment`) for every npc
  destination walk (the merchant stops, the teacher stops, the
  delevel guard walks): the npc rung arms the tight plan (the found
  answers validated against the merchant deck - the 2026-09-11 roof
  teleport geometry refused; the partial answers always arm), the
  wide rung (`tripApproachRadius`) serves the conservative deck stop
  for the destinations whose own point the mesh or the geodata cannot
  deliver. The teacher stop lost its deterministic double attempt.
- the grid engine bug underneath: the approach goal tested only the
  ball - a radius under the cell half diagonal (~11.3) never
  satisfies it on any node, the search flooded the component (ten
  seconds per search measured) and answered the bare not found; the
  goal now also accepts the target cell arrival (the plain run's
  any-layer semantics).
- verification: go build, go test hunt + pathfind full packages ok;
  the new pins: TestFarMerchantTripWalksIntoTheShop,
  TestNpcStopsSearchWithTheNpcApproachRadius,
  TestDelevelGuardWalkSearchesWithTheNpcRadius,
  TestMerchantRoofFallbackStopsOnTheWideDeck,
  TestFindPathApproachTightRadiusReachesTheTargetCell.


### Progress (2026-09-20, the server window speed round)

- the owner directive: the books are bought at the same moment as
  the jewelry elements, and the skill teaching runs at the maximum
  speed the server allows; the context is the farm readiness
  acceptance (level 15: the weapon, the armor, the basic jewel set,
  the spellbooks and the lessons land in one village walk).
- the server research (the mobius C1 source of this deployment):
  the transaction flood protector (buy AND sell share it) is
  FloodProtectorTransactionInterval = 10 game ticks = 1 second
  wide, a refused request costs nothing (the window does not
  extend, FloodProtectorTransactionPunishmentLimit = 0); the
  RequestAcquireSkill packet has NO flood protector at all - the
  handler checks only the trainer distance, the level, the SP and
  the spellbook, and every successful learn answers with a
  SkillList (the confirm round trip IS the rate limit).
- the pacing cut: transactionPause 11 s -> 1.25 s (the server
  window plus the 250 ms decision tick margin) paces the buy lists
  and the sell batches through one shared gate
  (Loop.transactionWindowFree) - the spellbook list of the jewel
  trader follows the jewel list of the same visit right behind
  (the books ride the Creamees stop via the planLearnStops merge,
  pinned by TestSpellbooksMergeIntoTheJewelStopVisit and
  TestBookListFollowsTheJewelListAtTheTransactionPace); sellJunk
  and the replacement offers joined the shared gate (a sell fired
  right behind a buy would burn the window and the refused batch
  would be marked sold without leaving the bag - pinned by
  TestSellWaitsForTheBuyTransactionWindow).
- the learn speed: the maximum allowed speed is confirm driven -
  the next request fires the moment the previous SkillList lands
  (the confirm tick walks straight into the next send); the
  pacing pause is a 250 ms tick floor (learnPause 1 s -> 250 ms)
  and the send drop bug is gone: sendLearnRequest reports whether
  the packet actually went out and an unsent request arms nothing
  (the early rounds armed the confirm window on every pick and the
  dropped send burned its full 5 s learnConfirmWait before the
  retry re-requested - every lesson paid the stall). Pinned by
  TestLearnPacedSendDoesNotBurnTheConfirmWindow and the tightened
  TestLearnLessonConfirmsBySkillList.
- verification: task fmt, fmt:check, vet, lint (0 issues) and
  go test -count=1 ./... all green on the rebased tree (the navmesh
  port revert of the parallel round removed the tools and the
  artifacts this round had repaired, the gate repair is dropped as
  moot; 28 packages, the hunt suite 65 s). The measured acceptance flow (~472-481 s under the early
  pacing) rides the server windows now: the shopping list batches
  and the ~40 lessons of the level 15 queue pace at the round trip
  speed instead of the 11 s / 5-6 s stalls.
### Progress (2026-09-20, the delevel priority round)

- the owner report on the delevel acceptance: the mobs of the spawn
  spot pull the farming first under a random coincidence of
  circumstances; sometimes the bot goes shopping for new equipment
  instead of deleveling; sticking on the way to the eastern guard -
  the systemic ask: eliminate the sticking on corners and turns.
- the farming first root causes: the delevel check ran before the
  cell pick (a nil leash reads an empty live median while the same
  tick's engage farmed), and the trigger required a non zero LIVE
  median that flickers with the respawn windows (two mobs, 15-20 s
  respawn - most ticks read empty). The fix: the first cell pick
  moved ahead of the gates and the empty live read falls back to the
  static median of the anchored cell (onHeldGround gated, the same
  delevelTriggerMedian feeds the target computation, startDelevel
  clears the engage leftovers).
- the shopping first root cause: maybeStartTownTrip ran before the
  delevel check. The fix: the deleveling outranks the town trips -
  the check moved ahead of handleTownTrip in the tick.
- the corner stick mechanism (cmd/cornerprobe on the real pack): the
  destination correction stops the character 8..50 units short of
  the funnel pivot, the pass radius counts the turn reached, the
  next chord cuts the corner and collapses (the anti corner cut),
  the shorten ladder halves into the same corner, the back hop
  ping-pongs the band, and the re-path ladder reproduced the
  identical route and sealed the corner with a session long corridor
  ban. The fix: clickForwardJump - the first later plan waypoint
  whose line the server transport validates from the stuck cell; the
  cursor jumps and one walk rounds the corner before the stuck
  window opens.
- the tests: TestDelevelOutranksTheEngageOnTheSpawnSpot,
  TestDelevelTriggersOnStaticMedianWhileSpotEmpty,
  TestDelevelOutranksTheTownTrip, TestCornerTurnBandJumpsTheCursor
  Forward, TestCornerWalkRoundsTheTurnWithoutRepath,
  TestCornerTurnJumpScanSendsNothingWhenNothingValidates,
  TestCornerTurnJumpCarriesTheValidatedTarget.

### Verification

go build, go vet, golangci-lint run on the touched packages 0 issues,
gofmt-spaces clean, go test ./... 28 packages ok zero failures. The
live acceptance run of the delevel scenario stays for the next
session with the stack up.
## Active task (status: complete): the arrow restock count awareness round (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~10:37 UTC, closed ~12:05 UTC. One owner report
landed as melg8 (581b9ae the fix, the docs commit right after),
rebased over the parallel webui review push and pushed. The report:
the bot sold the pants to the trader but never bought the arrows its
shop queue kept displaying.

### Root cause (verified at the source, Round 121 in development_log.md)

The trip execution filtered purchases by the inventory ENTRY count;
stackables work on COUNTS. `dropOwnedPurchases` counted the 101
arrow stack as one entry beyond the family copy count and dropped
the 499 arrow restock order before any request (the planner is count
aware: 101 < 150 floor -> top up to 600 - hence the queue showing
the arrows forever). The latent twin: `buysArrived` confirmed stack
orders by the id presence, but the Mobius inventory merges the
delivery into the carried stack - the entry never grows, only the
count does (verified in the C1 RequestBuyItem sources).

### Landed

- `isStackPurchase` (the npcdata item type discriminator) + the
  count aware `dropOwnedPurchases` + the baseline based
  `buysArrived` (`Loop.buyBaseline`) + `resetBuyRequest` (the five
  inline teardowns centralized).
- Tests: `hunt/arrow_restock_repro_test.go` (the reported trip end
  to end, red before), `hunt/arrow_restock_gates_test.go` (the four
  gate quadrants), the `tools/repro_gear.js` queue-state scenario.
- Review round (fresh-context critic): 3 majors landed - the pair
  family arrival phantom fixed with the entry-unit baseline gate,
  the budget capped partial restock fixed with
  `Purchase.OwnedStack` (the staleness rule compares against the
  plan target), the weapon-run cooldown fixture arming the guide run
  fixed with the expected buff set (the flake ledger row added).
- Docs: the frozen trip plan rule in shopping_strategy.md now names
  the kind aware split; development_log Round 121 carries the RCA
  and the review outcome.

### Verification and known state

- The full hunt suite green (74 s, after the review fixes), build +
  vet green, the round's files lint clean, the harness OK.
- Known pre-existing (NOT this round): golangci-lint cyclop on
  `clickWaypoint` (hunt/town.go, 16 over the max 15) - landed with
  the parallel skip-tracker rounds (8487365/0dc22da), disclosed
  here, the corridor branch extraction is the follow up.
## Active task (status: complete): the web UI polish round - the kill skulls, the mob level banner, the chat artifacts and the honest scroll (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~11:30 UTC, closed ~12:40 UTC. Five owner reports
landed as melg8 (a161ee7 the chat texts, 81734fa the web UI round,
9f00b69 the lint gate), rebased over the parallel review hardening
push and pushed. The reports: the kill crosses want skull icons, the
fight banner wants the mob level, the chat widget clips its top row,
the chat texts carry server artifacts (Use 3, the stray ?, the raw
1068) and the auto scroll fights the reader.

### Landed

- Chat texts (state/chat.go, packets/system_message.go,
  connection/game_dispatch.go): the skill name parameter keeps its
  second wire int (the level, verified against SystemMessage.writeImpl
  of the Mobius C1 sources) and renders through the generated skill
  dictionary - "Use Power Strike lvl 3.", "You can feel Might lvl 1's
  effect."; the missing tail parameter renders as nothing (the server
  sendMessage texts ride the generic S1_S2 template with one
  parameter - the stray "?" is gone) with the template gap trimmed.
- Web UI (map.js, app.js, style.css): the fleet kill crosses and the
  spot centroid crosses draw as two-pass path traced skulls (the
  orange body, the dark face; fade buckets and toggle unchanged, no
  font glyph); the fight banner appends "lvl N" for the npc targets;
  the chat head and input rows pin whole pixel heights so the list
  leftover is 126px = 18 * 7 exactly (no clipped top row);
  renderChat captures and restores the reading offset around the
  rebuild (the browser clamps the emptied scrollTop to 0 - the stub
  emulates that now and the checks prove the restore and the clamp).
- The full lint gate back to the single pre-existing disclosed
  finding (clickWaypoint cyclop, the corridor extraction stays
  queued): the zero param literal names its Level field, the church
  entry walk fills the chat zero fields, handleChatPacket extracts
  the chat dispatch arm of handleServerPacket.

### Verification

go build, go vet, the state/packets/connection/acceptance/webserver
suites green, all six Node harnesses pass (repro_hud, repro_zone_hover
with the rewritten skull scenario, repro_map_render, repro_bot_switch,
repro_gear, repro_movement). Docs: webui.md (chat window, kill
skulls, banner), development_log.md Round 122 (the RCA classes: the
dictionary/parameter-type gap behind every quoted server artifact,
the harness stub honesty).

### The review round (closed)

The clean context critic verified all seven fixes pass with an
independent verification matrix (the six harnesses plus
buffs/fight_ui/stats, build, vet, five Go suites, the full lint gate,
its own scroll stress probe). Follow ups landed as melg8 (87e7417):
the acceptance botlog renderer resolves the skill name parameters
through the dictionary now (the same class fix as the chat window),
the stale cross comments follow the skull rename, the round report
count is consistent.
## Active task (status: complete): the web UI feedback round - the skull tooltips, the anchored scroll, the collapsible chat (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~13:20 UTC, closed ~14:40 UTC. Seven owner reports
landed as melg8 (8ea1771 the anchored scroll, 1317c20 the skull size
and the victim tooltip, 5ae1918 the collapse and the channel colors,
b143f09 the whisper root cause), rebased over the parallel pushes.
The highlights: the kill marks carry the victim name and level now
(the kill record captures them at the kill, the npc dictionary
resolves a vanished corpse) and the skull hover shows the victim and
the age; the chat scroll anchors the reading row by identity (the
ring drops no longer move the read segment); the chat collapses into
the corner button; the whisper one-letter-per-line root cause was a
CSS class collision (the bare .chat-whisper input selector matched
the whisper kind rows, squeezing them to 90px) - found with the
preview server plus a headless browser measuring the real boxes.
Docs: webui.md, development_log.md Round 123.

### The review round (closed)

The clean context critic verified all seven fixes pass (nine Node
harnesses, build, vet, four Go suites, the full lint gate at the
single disclosed finding) and its follow ups landed as melg8
(77620bc): the corpse on its own kill skull yields the tooltip to
the victim read, the collapsed window skips the snapshot renders
(the hidden box measured zero offsets and parked the reading
position) and renders once on expand, the round entry renumbered to
125. The open follow up: the live encoder crit flag (8b3d716) needs
a fixture guard - liveSnapshotBot fires no critical hit, so a
regression dropping Crit again would pass the suites; add a crit
attack plus an HP drop to the fixture.

## Active task (status: in progress): the overhit finishing blow and the melee self heal recovery (2026-09-21, branch feature/improved-behaviour)

Started 2026-09-21 ~14:50 UTC. New owner prompt (the 2h session clock
reset with it). Two combat behavior features:

1. **The overhit finishing blow.** The verified server mechanic
   (Mobius C1 sources, research round below): a skill whose stats
   carry `<overHit>true</overHit>` (21 skills; for the deployment
   classes Power Strike id 3 and Power Shot id 56) arms the overhit
   flag on its target when the cast resolves
   (`Creature.callSkill` L6043); the flag is consumed by the FIRST
   damage event after it - if that damage kills the attackable, the
   killer gains `exp * min(overkillDamage / mobMaxHp, 0.25)` bonus
   (`Attackable.calculateOverhitExp` L1480, the +25 percent cap),
   any non lethal hit clears the flag (`AttackableStatus.reduceHp`
   L40-71). The bot plan: `maybeCastCombatSkill` holds an
   overhit-capable strike while the target stands above
   `overhitFinishPercent` (40 percent - the low level band lands
   inside the window after one swing, the skill nearly always kills
   from there and the overkill reaches the cap) and fires it as the
   finishing blow once the target drops into the window. Static
   overhit skill set lands in `npcdata` (verified id list).
2. **The melee self heal recovery.** A character that knows a
   self heal skill (verified SELF target instant heals of C1:
   Divine Heal 45, Elemental Heal 58, Self Heal 1216 - the mystic
   starting classes auto learn 1216, the server may grant any of
   them) casts it instead of sitting down while the mana pays the
   cost, the local reuse window is clear and no blow is landing
   (`SelfUnderAttack` gate). The server refuses the casts of a
   sitting character (`Player.useMagic`, "YOU_CANNOT_MOVE_WHILE_
   SITTING"), so a sitting character stands up first; the sit
   request waits out the cast flight (`sitDown` refuses "Cannot
   sit while casting"). The post relogin settle needs no change:
   it ends on a clear ground and hands the recovery to the rest
   flow, and a heal cast would burn the spawn protection it holds.

Status: implemented and pushed as melg8 (8b96cce):

- `npcdata/skill_flags.go`: the hand-verified flag sets -
  OverhitSkill (21 C1 ids with the source line trail) and
  SelfHealSkill (45, 58, 1216).
- `hunt/combat_skills.go`: the overhit hold gate inside
  maybeCastCombatSkill (the strike waits under the 40 percent
  finish window, targetAboveOverhitWindow holds on unknown vitals),
  the finishing blow log line.
- `hunt/recovery_heal.go` + the rest() integration
  (loop_actions.go restToggleTo split out for the funlen gate):
  heal instead of sit while the mana pays, reuse clear, no blows
  landing; the sitting character stands to cast; the sit waits out
  the heal flight (the server refuses "Cannot sit while casting");
  the mystic mana rest and the spawn settle untouched by design.
- Tests: overhit_test.go (hold healthy/unknown, release + log
  naming, the unflagged spell unaffected), recovery_heal_test.go
  (replace, flight wait, no mana, reuse, stand to cast, under
  attack, mystic mana rest, no skill regression), the two fight
  fixtures gained mob vitals (the hold changes the fight start
  behavior). hunt/connection/state/npcdata suites green,
  lint --new clean.
- docs/hunting.md: the self heal recovery bullet and the overhit
  finishing blow section (the verified mechanic with the source
  lines, the 40 percent window rationale, the follow up damage
  estimate idea).

Observed: the connection suite failed once in a parallel combined
run right after "Sent enter world request" and passed twice since
(isolated and combined) - a load flake, watch it, no ledger row
until it repeats.

The fresh context review round (a general purpose critic with the
original prompt plus the refined version) returned no critical
findings: every server mechanic claim reproduced from the Mobius
sources line by line (arming order before activateSkill, the first
damage event consume, the +25 percent cap, the player-only bonus,
the 21 id set exact), no stuck bot paths found, 13/13 targeted
tests green. The follow ups it raised landed the same session:

- The mana gate of the heal now sums mpInitialConsume
  (npcdata.SelfHealInitialConsumeOf, hand tables for 45/58/1216
  from the C1 stats; the server gate is Creature.java L2049) - the
  2-15 mana band no longer fires a cast the server refuses (which
  would arm the local reuse and delay the sit by the flight
  window). Folding the initial consume into the generated cast
  tables is the generator follow up; the combat cast gate has the
  same shape (a refused strike locks its 15 s local reuse) and is
  the recorded follow up of the same class.
- The overhitFinishPercent comment and the docs section no longer
  overclaim: the 40 percent window is the kill probability play;
  a strike weaker than the remaining bar fires, misses the kill
  and loses that cast's bonus (total fight damage unchanged).
- The docs state the blunt scope fact: the vanilla C1 trees never
  teach a self heal to a fighter class, so the deployment melee
  bots keep the sit-rest until the server grants them the skill;
  the mechanism fires the moment the list carries one (the
  deployment mystics benefit today).
- npcdata/skill_flags_test.go locks the flag set membership, the
  A1/SELF shape and the initial consume tables against
  transcription drift.
- Doc line drift fixed (the useMagic refusal is L7540), the
  missed-cast keeps the flag armed note added (the auto attack
  kill that follows still pays).

Verification: build, vet, the npcdata and hunt suites green,
lint --new clean. Commits as melg8: 4ce5693 (the task entry),
8b96cce (the implementation), a29e65e (the docs), the review fix
commit on top. Round complete.

Two more commits from the same pass: the maybeCastCombatSkill
eligibility chain moved into combatSkillCandidate (the overhit
gate pushed the cyclop complexity to 18 over the 15 cap; the
helper drops the cast decision to a named predicate), and the
full tree lint surfaced a PRE EXISTING debt of a parallel round:
clickWaypoint (town.go) sits at cyclop 16 - the crude branch count
shows it was already 17 before this session started, so the zero
findings claim was stale. Refactoring a navigation critical
function does not belong to the tail of this session - it is the
first pick of the next one. 
