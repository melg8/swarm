# The autonomous hunt: combat, zones, gear, shopping, deleveling

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
SPDX-License-Identifier: MIT

The growth subsystems of the autonomous hunt (all unit tested; the
design goal is per-class and per-region extension). The `-hunt` flag
of `cmd/swarm` enables the whole machine; without it the loop runs in
the manual only mode (commands execute exactly the same way, the
autonomous hunting, town trips and deleveling stay off, the village
restart after a death still works).

The four pillars, each detailed in its section below:

- **Auto equipment** (`internal/swarm/gear`, `hunt/equip.go`) wears
  every equippable item the loot and the buys produce.
- **Shop strategy** (`gear/shopping.go` and its `shopping_*.go`
  siblings, `hunt/shopping.go`, `docs/shopping_strategy.md`) plans and
  executes the purchases.
- **Multi-zone hunting** (`hunt/zones.go`, the generated registry
  `hunt/zones_elven.go`) climbs the mob level ladder of the region.
- **Hexagon cell hunting** (`hunt/cell*.go`, the generated registry
  `hunt/cells_elven.go`) replaces the ladder on the elven lands: the
  uniform hexagon partition, the respawn-paced neighbor rotation, the
  enemy-first free roam and the measured scoring (the design of
  `docs/hunting_cells.md`).

Extension path: a mage class implements `gear.Profile` (mAtk weapons,
robe preference - the planner, the strategy and the trip execution
stay unchanged), a new region adds its `townMerchants` list, its zone
registry entries and its tax rate. The elven deployment is the
reference wiring of all three (`main.go`:
`SetHuntingZoneRegion("elven")`).

## The hunt loop (internal/swarm/hunt)

`internal/swarm/hunt` runs a small state machine (engage -> loot ->
engage) that attacks the closest attackable npc inside the hunting
square, picks up the drops around the corpse and destroys junk
inventory items (RequestDestroyItem 0x59) when the slots reach 70% of
the 80 slot limit or the weight reaches 75%, so a long living bot
never litters the server. The destroy cleanup is the last resort only
- the normal overflow handling is the town trip below, and the cleanup
is suspended while a trip runs so it never destroys what the shop
would have paid for.

- **Attack start**: the Mobius AttackRequest 0x0A has double click
  semantics (AttackRequest.runImpl) - the first request for a new
  target only selects it (NpcClick.onAction -> setTarget, answered
  with MyTargetSelected), the repeated request for the already
  selected target resolves to onForcedAttack and starts the fight. The
  hunt loop therefore sends the selecting request
  (GameClient.AttackNearest), then re-sends the target once per second
  (the PlayerActionFloodProtector interval; GameClient.AttackTarget)
  until the tracker sees the character engaged with it
  (state.Bot.SelfEngaged: the chase MoveToPawn, Attack or
  AutoAttackStart broadcasts set the fighting target); an engage that
  never goes fresh is dropped by the stuck timeout or recovered by the
  blind engage walk (see below). The whole flow is covered end to end
  by the fake server in internal/swarm/connection/hunt_flow_test.go.
- **Target death and idle chaining**: the server never clears the
  selection of a killed target (MyTargetSelected only answers new
  selections; the removal path only broadcasts the own
  TargetUnselected), so the tracker clears the character target itself
  when the target dies, is removed or is unselected, and the hunt loop
  only adopts the server side target id while it is alive - otherwise
  the stale dead id re-enters the loop every tick and the next target
  is never selected.
- **Loot approach**: far items are reached with an explicit walk first
  (GameClient.WalkTo sends the client MoveToLocation 0x01 packet, the
  ground click of the official client, in mouse mode) and the click
  (Action 0x04) starts at 60 units: the server AI covers the last
  stretch and executes the pickup (StopMove to self, GetItem broadcast,
  InventoryUpdate), the walk keeps the approach smooth on the map.
  Failed pickups (protected or unreachable items) are skipped for 30
  seconds after a 20 second attempt.
- **Cadence and rest**: the loop decides on a 250 ms cadence, chains
  the next target immediately while the character HP is at or above 60
  percent (rate limited to one player action per second for the flood
  protector) and rests with a logged reason below it. Resting sits the
  character down below 60 percent (RequestActionUse 0x45 action 0,
  GameClient.ActionSitStand - the sitting regeneration is faster) and
  stands up at 90 percent; the toggle is confirmed by the
  ChangeWaitType 0x3F broadcast (state.Bot.SelfSitting) and a repeat is
  only sent when the flip never happened, so a lost packet can never
  leave the character toggling between sit and stand. While the
  character is under attack (Bot.SelfUnderAttack) the loop keeps
  fighting instead of resting, and a dead character (Bot.SelfDead:
  CUR_HP 0) restarts at the nearest village automatically
  (RequestRestartPoint 0x6D type 0, GameClient.RestartAtVillage,
  retried every 5 s until the server revives it) so a 24/7 session
  survives a death.
- **The self heal recovery (hunt/recovery_heal.go)**: a character
  that knows an instant self heal (npcdata.SelfHealSkill - the
  verified C1 SELF target Heal skills: Divine Heal 45, Elemental
  Heal 58, Self Heal 1216) casts it instead of sitting down while
  the mana pays the cost, the local reuse window is clear and no
  blow is landing (SelfUnderAttack). The cast recovers the chunk of
  the bar in one hit time where the sitting regeneration grinds for
  half a minute, and a standing character keeps the flee option the
  sit gives up. The verified server refusals shape the flow
  (Mobius C1, the cast refusal of a sitting character at
  Player.useMagic L7540 and the sit refusal of a casting character
  at Player.sitDown L2383): a
  sitting character cannot cast, so the deep rest (below the sit
  threshold) stands up first and casts on the next window; a
  casting character cannot sit, so the sit transition waits the
  heal flight (hit time plus a 1 s margin) out instead of bouncing
  off the server. The mana gate matches the server one
  (Creature.java L2049 sums mpConsume and mpInitialConsume -
  npcdata.SelfHealInitialConsumeOf covers the initial part the
  generated cast tables do not carry yet). The blunt scope fact:
  the vanilla C1 skill trees never teach a self heal to a fighter
  class - the deployment melee bots keep the sit-rest until the
  server actually grants them a heal skill (a custom grant, a
  changed tree), and the mechanism above fires the moment the
  skill list carries one. The deployment mystics (Self Heal 1216
  is auto learned) benefit today. The mana rest of the mystic is
  untouched - a dry
  caster needs the sitting mana regeneration, a heal only spends
  it - and the post relogin settle needs no heal branch: a cast is
  one of the five packets that burn the spawn protection (see
  settle.go), so while aggressive mobs stand in reach the settle
  keeps sitting, and once the ground is clear the settle ends and
  this flow owns the recovery.
- The nearest target ranking uses the projected current positions of
  moving npcs (state.projectedPosition), not the stale movement packet
  starts. The hunt chain behavior is covered by
  internal/swarm/hunt/loop_test.go (next target selection after a
  kill, rest gate, walk to far loot, sit/stand transitions, no sitting
  under attack) and the tracker clearing by
  internal/swarm/state/tracking_test.go.

- **Stagnation watch (hunt/stagnation.go)**: the M1 soak proof
  requires a silent livelock to become a loud line, so the tick
  publish defer observes two progress values every tick - the
  character experience and the exact standing cell. No experience
  change for `stagnationXPWindow` (20 min, calibrated over the
  measured town trip rounds: the weapon run round takes 2.2 min, the
  full farm readiness round 14m54s) or the same position held for
  `stagnationPositionWindow` (10 min) logs one explicit event per
  window (`Hunt: stagnation: no experience change ...` / `Hunt:
  stagnation: position held ... at X Y Z`), carrying the loop phase so
  the log explains what the state machine was doing while frozen. The
  lines route through `Loop.logf` (console, tracker event log, the
  web UI log tab) and the stall ages ride the state dump diagnostics
  (`xpStallForMs`, `positionStallForMs`). The windows re-arm after
  each event (a frozen character logs its stall every window, not
  every tick) and reset on the offline gaps, the manual only
  sessions and the missing position (a relogin is not a livelock).
  The unit tests live in hunt/stagnation_test.go (the seeded
  baselines of the loop tests are the clock seam: fire, re-arm,
  refresh-while-farming, manual/offline quiet, the tick path
  coverage, the event feed surface, the diagnostics wiring).

## The stagnation watch (hunt/stagnation.go)

The M1 soak proof requires that a silently stuck bot becomes a loud line and
then recovers itself, so the loop watches two values: the experience
(`stagnationXPWindow`, 20 min) and the exact position
(`stagnationPositionWindow`, 10 min). The windows are calibrated against the
measured honest flows of the elven autonomy: the longest healthy experience
stall is the full town trip round (2.2 min for the weapon run, 14m54s for
the whole farm readiness round with every stop landed), and the longest
honest stand on one cell is a rest or a paced lesson stop (minutes). A
frozen walk (one village cell for over an hour) or a dead engage blows
straight past both.

An event logs one line per window with the current loop phase (the console,
the web UI event feed and the state dump all mirror the loop logger), lands
in the tracker action note and in the session journal (`Stall`), and
re-arms only its own window, so a frozen character logs its stall every
window instead of every 250 ms tick. The baselines reset on the offline
gaps (a supervisor relogin is not a livelock) and on the manual only
sessions (an interactive character may stand still).

The escalation rides the same watch: the first position stall clears the
frozen loop state in place (the soft reset - the target skip for the frozen
object, the engage timers, the blind recovery, the pending loot, the panic
and flee episodes, a mid-freeze town trip and the return budget all restart
from the live world state, and a sitting character stands up), while a
stall that survives it or a full experience window rebuilds the session
through the emergency logout (the hard reset - the known cure of the stale
selection, the refused collapsed walk clicks and the frozen engage stance),
paced by `stagnationHardCooldown` (15 min) so a livelocked loop retries one
rebuild per window instead of thrashing reconnects. The delevel phase is
exempt from both (its deaths refresh the experience, its own timeout bounds
it). The experience check runs first and its hard recovery arms
`logoutDone`; the position check must not run after it, or the soft reset
would run on the dying session (the 2026-09-14 03:55 dump: the position
branch logged "clearing the loop state" while the emergency logout was
already unwinding, and the relogin inherited none of the cleanup).

## Combat safety (hunt/loop_safety.go + the constrained target search of state)

- The engage never initiates on mobs above the character level + 2 or
  on social pulls - a mob whose clan mates stand within its
  `clanHelpRange` (mirroring the Mobius AttackableAI clan call: the
  ALL clan of the attacked mob matches everything, a 600 unit z
  distance blocks the assist, the projected positions measure moving
  packs, a 200 unit margin covers the mates wandering mid fight).
- The aggro answer (2026-09-10, extended 2026-11-09): a mob that
  already holds the character as its target (its swings or its chase -
  both carry the character as the mob's target id, `NearestAttacker`)
  owns the targetless pick: a healthy character with a winnable
  attacker (inside the engage level ceiling, `attackerEngageable`)
  engages it at once - the forced attack request fires on the same
  tick and the fight answers the aggro where it stands - while
  anything else (a hurt character, an attacker above the ceiling)
  keeps the defensive flow, the standard escape walk with its logout
  budget. The answer also outranks the approach: on every engage tick
  where the selected target is not fighting yet, a DIFFERENT nearer
  attacker replaces the pick before the stuck timeout, the
  losing-fight verdict and the blind recovery run - the pick holding
  the character as its own target no longer suppresses the switch,
  the mob that actually swings owns the next swing. The loot phase
  yields the same way (`loot` hands the tick to the engage when a
  living attacker exists): the winnable attacker is fought before the
  next corpse step, the rest run the defense flow. The town trips
  answer the same way (`interruptTripForAttacker` runs first
  in the trip tick): a mob on the walking seller drops the trip
  through the soft reset (no cooldown, the junk, the books and the
  sold proceeds survive) and fights or runs instead of dragging the
  chase through every camp on the route - the reported pile up
  deaths of the walkers. `adoptOutZoneFight` checks the same
  winnability before it finishes an attacker outside the zone: an
  unbeatable chase switches to the escape instead of a losing fight.
- A RUNNING fight that turns into a death risk is fled: under 25%
  health, or under 60% while the target holds a 25+ percent health
  lead, the target is dropped with a two minute skip and paced escape
  segments open distance; a target one swing from dead is finished
  instead. The percent-gap rule needs the confirmed fight
  (`SelfFighting`): before the first swing the target bar is simply
  full, and the old pre-fight rule fled a winnable aggroed mob seconds
  after the answer (the 2026-11-09 live round: the adopted spider sat
  at 100% against a 56% character and the bot ran). A hurt character
  under attack keeps fleeing instead of standing in the blows.
- The escape runs an ordered lane fan (`escapeWalkDestination`): the
  straight away ray first, then the 45 and 90 degree lanes each side
  of it, each clamped along its own ray into the patrol square at the
  `escapeZoneMargin` inner line. The old out-of-square fallback ran
  toward the zone center - the camp focus in cell mode - which
  flipped the character 180 degrees back through the chaser on every
  tick the away point left the square (the reported chaotic flee).
  The fan never folds back toward the threat and keeps the direction
  stable between ticks while the geometry does.
- **The overhit finishing blow (hunt/combat_skills.go)**: the bot
  farms the server overhit experience bonus deliberately. The
  verified mechanic (Mobius C1): a skill whose stats carry
  `<overHit>true</overHit>` arms the overhit flag on its attackable
  target when the cast resolves (Creature.callSkill L6043); the
  flag is consumed by the FIRST damage event after it, and only a
  killing blow pays - any non lethal hit clears it
  (AttackableStatus.reduceHp L40). The bonus is
  `exp * min(overkillDamage / mobMaxHp, 0.25)`
  (Attackable.calculateOverhitExp L1480), capped at +25 percent of
  the base share, and the killer must be the player character
  itself (summon kills pay nothing). The overhit skill set lives in
  npcdata.OverhitSkill (21 verified C1 skills; the deployment
  classes meet Power Strike 3 and Power Shot 56). The combat cast
  therefore holds the flagged strike while the target stands above
  the 40 percent finish window (`overhitFinishPercent`) and fires
  it once the bar drops into it: in the low level band one melee
  swing takes 25-45 percent of the mob bar, so a bar inside the
  window usually holds about a strike's worth of damage - the cast
  lands the kill and the overkill pays the bonus. A strike weaker
  than the remaining bar (the level +2 pulls, the early strike
  levels) fires inside the window, misses the kill and loses this
  cast's bonus; the total fight damage is unchanged, the cost is
  the delay and the mana - the absolute damage estimate follow up
  (observed swing damage vs ObjectVitals) closes the gap. The
  trash that dies inside one swing never opens the window (the
  strike stays sheathed, the mana is saved), and a missed cast
  keeps the server flag armed - the auto attack kill that follows
  still pays the bonus (only a landed non lethal hit clears it).
  An "Over-hit!" system message (id 361) confirms a
  landed overhit on the wire.
- Two triggers end the session: critical health (12%) under attack, or
  a social pile up - two or more living attackable mobs holding the
  character as their target (`SelfAttackerCount`; a chasing mob
  carries the same target id as a swinging one, the character's own
  engagement never counts). One last escape segment keeps the offline
  character moving through the 15 s combat stance the server holds it
  in, the `RequestLogout` packet plus the socket close follow, and a
  30 s login cooldown the supervisor (`runBotForever`) honors before
  the next session (the tracker carries the cooldown across sessions -
  half a minute covers the combat stance plus the mob reset walk home
  without idling the farm for minutes).
- Entering a hunting zone engages the first valid target the entry
  radius offers (`engagesOnZoneEntry` of the return phase), and a
  targetless hunter patrols toward the zone center after a 6 s
  patience window instead of standing still. A big square whose pack
  sits outside the engage radius (1500) is walked toward directly:
  `walkToFarTarget` searches the whole zone for the nearest valid mob
  and follows one paced segment per second, so the hunter closes on a far
  pack instead of standing central in an empty radius (the rotation
  only fires on a fully empty square).
- A flee that never shakes the chase ends the session too: one escape
  episode older than `fleeLogoutAfter` (20 s) logs the character out
  (a recovered health or a fresh fight clears the episode) - the
  relogin after the pause resets the mob aggro instead of the bot
  running from the pack forever. A cornered escape (every lane clamped
  under `escapeMinLane`) and a dead one (less than
  `escapeNoProgressDistance` = 120 units covered inside
  `escapeNoProgressWindow` = 3 s - the H-006 silent click drop, a
  terrain pin) log out at once instead of standing in the blows; the
  observation window re-anchors on every covered window and clears
  with the flee episode. A zone switch drops the remembered
  farm spot of the previous square (the returns aim at the new center
  until a fresh spot is remembered inside it).

## The transit aggro steering (hunt/loop_avoid.go)

The Mobius `AttackableAI` attacks a player on sight when the 3D distance
drops inside the mob's effective aggro range with a line of sight (the xml
`aggroRange` clamped at `MaxAggroRange` 450 for the `Monster` type, see
the live facts below), so a transit walk - a town trip in either
direction, a zone return, an inter-ground walk - that passes an idle
aggressive camp collects a chaser, the train that later forces the pile up
run and the emergency relogin. The steering deflects one issued segment at
a time (`steerClearOfAggro`): the endpoint moves onto the tangent ray of
the first threat circle (the effective aggro range plus the
`avoidClearance` 150 margin) on the side the segment leans to, and the
walk follower re-issues the request every period from the live position,
so the chained tangents arc around the camp while the waypoint plan stays
untouched. A character already inside the margin circle side-steps
straight out (no tangent exists from inside, and the perpendicular keeps
the radius constant). The endangerment check is 3D like the server trigger
and the tangent is planar; a threat already holding a target belongs to
the flee machinery, the current fight target drops out through its id, and
the mobs within `avoidArriveExempt` 500 of the walk destination stay
exempt - the ground the walk enters carries its own mobs and the engage
phase answers them. Segments under `avoidMinSegment` 100 never steer, the
scan bounds at `avoidScanRange` 1600, and the detour log line paces at
`avoidLogPeriod` 5 s.

A waypoint inside the trigger circle of an idle aggressive mob is skipped
instead of clicked (`segmentTargetThreatened`): no tangent can land there,
so the receding horizon would re-issue from alternating tangent endpoints
and the character would ping-pong without net progress - the observed
delevel walk burned an hour around one idle Kaboo Orc Fighter whose
waypoint sat 338 units inside the 600 unit margin circle. The check takes
the waypoint itself, not the clipped click target: the clip parks the
click next to the character, and a camp between the character and the
horizon would condemn every far waypoint of the direction (the mass skip
of the 2026-09-20 town walk).

A running fight steps clear of an impending add (`avoidImpendingAdd`): an
idle aggressive mob inside its trigger band plus `combatAvoidWarning` 150
around a fighting character is about to join, and a short
`combatAvoidStep` 300 sidestep away from the deepest-band threat opens the
distance before the trigger fires - backing off a second opponent beats
the two-attacker pile up logout. The step owns the shared movement window
`combatAvoidStepPeriod` 2 s (a forced attack request interrupts a running
walk server-side, so the engage holds its re-requests until the step ends
and the swings resume after), the steps pace at `combatAvoidPeriod` 4 s,
and the threat scan runs at `combatAvoidScanPeriod` 1 s over
`combatAvoidScanRange` 800. A straight away lane walled by the terrain,
water or the leash tries the flanking lanes at 30 and 60 degrees each side
(`combatAvoidFanStep`) through the terrain contract the kite retreat lanes
run (`combatAvoidLane`); when every lane is walled the fight holds the
ground and logs it once per `combatAvoidHoldLogPeriod` 15 s (a refused
click freezes the character and burns the avoid window). The leash
outranks the add - a step that would leave the hunting square is skipped -
and the mobs on another deck never enter the scan (the 3D trigger cannot
fire across the gap).

## The spawn protection settle and the pile up policy (2026-09-21)

The verified server facts (H-005 in AGENTS.md, the Mobius sources):
every EnterWorld arms the spawn protection - PlayerSpawnProtection,
600 s in the deployed Player.ini. Attackable.getHating strips the
aggro of a protected player on every one second AI tick, so
aggressive mobs stand around a stationary fresh login without
attacking. The protection clears on the FIRST of these client
packets: MoveToLocation, AttackRequest, Action, UseItem,
RequestMagicSkillUse. The sit toggle (RequestActionUse) is not in
the list - a sitting character keeps the protection and regenerates.

The settle (hunt/settle.go, opt in via EnableSpawnSettle, the
supervisor enables it on every session): while the session is fresh
(the settleWindow cap), no blow has landed and the loaded ground
holds an aggressive mob within its own on-sight circle, the
character holds the spot and sits down to regenerate until the stand
threshold (90 percent). An empty knownlist (the enter world burst
has not arrived) or an unknown position holds without deciding - the
one shot settle is never burned on the packet gap. A landed blow
ends the settle at once: the protection strips hate, it never
absorbs damage.

The recovered character opens with the first strike instead of
walking: the nearest aggressive mob inside the firstStrikeRange (the
700 unit Power Shot cast range) is attacked FROM THE SPOT - the bow
owner arms the ranged tool through the lure machinery (the
lureArm/lureShoot phases run the ranged fight and swap the melee
weapon back when the mob closes to lureArriveRange), everyone else
answers in melee at the chosen spot with a full bar. The strike
keeps the engage discipline (the level ceiling, the mystic mana
gate); without an aggressive mob in range the ordinary pick flow
resumes. Breaking the protection exposes the character to every
aggressive mob on sight within the same second - that is the point:
the pack arrives at a chosen spot, full health, into a ranged opener.

The pile up policy answers the reported two mob softlock: a pack of
at most pileUpFightMaxAttackers (3) attackers inside the engage
level ceiling with the health above the re-engage threshold is
TANKED (pileUpWinnable in loop_safety.go) - the tick gate skips the
panic run, the aggro answer fights the nearest attacker, the fight
potion (fightPotionHealthPercent = 70, above the re-engage line so
the flee gates never pre-empt it; the loot phase serves it too)
keeps the bar under the swings. The kill that thins the pack to one
attacker hands the answer back to the ordinary flow; the health
drop re-arms the panic run mid fight. The unwinnable pile up keeps
the old run, relogin and regenerate cycle - which now lands into
the settle instead of an immediate walk back into the pack.

The CEILING is one for every layer: the general pick ceiling
(targetMaxLevelSlack, level plus two) bounds the melee trade, the
armed archer kite and the pile up verdict alike (attackerEngageable
and pileUpWinnable in loop_safety.go). The 2026-11-10 archer round
once widened the kite answer to level plus five and run 28 measured the
cost: the archer adopted a level 10 Fighter against a Spore Fungus,
lost half of its arrows and every fight at ~59 percent health. The
widened ceiling is reverted. A pack whose attackers all sit inside
the ceiling is TANKED (run 27 farmed its kills this way); only a
pack holding an above-ceiling attacker converts into the straight
flight (dropFightForFlight, see the archer kite section) instead of
the zoned panic run, and the kill window (30 percent) still
finishes first.

## Blind engage recovery (walk around the obstacle, then switch)

A small obstacle (a column) between the character and its target locks
the plain engage - `Creature.onForcedAttack` arms the ATTACK intention
WITHOUT a line of sight check, `Creature.doAttack` then answers every
swing with SystemMessage 181 ("Cannot see target.") while keeping the
intention armed, the attack stance never stops (no swing lands, no
AutoAttackStop arrives), so SelfEngaged stays true forever and the old
stuck timeout never fired (observed live: a 53 minute stall at 115
units with the refusal spam every ~3.5 s, walk plan empty, no events).

The recovery (hunt/loop_los.go) reads the refusal through the tracker
(ApplySystemMessage records the last CANNOT_SEE_TARGET arrival in
SelfCannotSeeTargetAt) and has two levels:

- Level A samples a ring of melee range standing points
  (blindMeleeRadius, 16 directions) around the target, keeps the
  nearest one with a bot side geodata sight line (Navigator.LineOfSight
  over pathfind.Engine), plans the geodata path to it and walks it with
  paced ground clicks (the attack re-requests stop while the walk runs
  - an attack request would replace the walk intention and cancel the
  detour).
- Level B drops the target and skips it for blindSkipDelay (1 min)
  when no navigator or vantage point exists, the path search fails, the
  walk misses blindWalkBudget (15 s) or blindMaxAttempts (2) attempts
  could not clear the block - the next pick selects a different mob,
  which also replaces the stale server side selection.

The arming of both levels survived the phantom chase of the refused
attack (the 2026-09-12 03:25 dump): the server AI of an armed ATTACK
intention keeps broadcasting the character's OWN MoveToPawn chase steps
(`PlayerAI.thinkAttack` -> `maybeMoveToPawn` -> `startFollow`) while
every `doAttack` of the same intention fails the `canSeeTarget` check
and answers "Cannot see target." - the chase stream refreshed the
tracker's `CombatActiveAt`/`FightingTargetID`, so `SelfFighting` read
true in a livelock, the fighting branch re-anchored the engage clock
past every refusal (holding the stuck timeout and the detection window
away forever) and the detection gate itself refused to arm while the
fight view looked fresh. The bot stood 80 units from its target for
minutes with the refusal spam every ~3 s, a Power Strike cast at the
invisible mob every 15 s, no walk, no switch, no landed blow. The fix
is an ordering rule: the refusal being the NEWEST fight activity
(nothing landed or stepped after the server said "cannot see") marks
the obstructed engage even while the chase view is fresh - only
activity strictly newer than the refusal (a swing or a chase step that
landed after it) proves the sight line cleared, both for the detection
(`blindEngageBlocked`), the standdown of a running recovery
(`fightClearedRefusal`) and the engage clock re-anchor of the fighting
branch. Pinned by hunt/round59_repro_test.go against the exact dump
scene.

The stuck timeout itself now measures the FRESH fight view (the gate is
!SelfFighting, not !SelfEngaged) and a running fight that progressed
past the last refusal re-anchors the engage clock, so a stale attack
stance without refusals still trips it after 12 s of no fight packets;
while the blind recovery is armed the timeout stays held (the recovery
manages its own budgets). Covered by hunt/loop_los_test.go
(reposition walk, arrival re-engage, both switch levels, retry budget,
stale stance timeout, timeout hold, attempt scoping, fresh fight
guard) and the state tracker test of the refusal recording.

## The covered pick lure (hunt/lure.go)

A melee hunter answers a pick that stands inside the aggro radius of a
covering pack the archer way, using the bow the gear plan buys behind
the weapon milestone (`gear/bow.go` restocks the quiver on the town
trips):

- the cover detection (`targetCovered`): an idle aggressive mob whose
  on-sight circle plus the 200 unit slack reaches the pick or the
  straight approach segment;
- the approach stops at the 420 unit standoff, a hair inside the 500
  unit bow range and outside every threat circle. The standoff
  direction is the one from the target toward the character, rotated
  through the eight compass directions until a candidate is clean; the
  fallback keeps the character side;
- the arm phase equips the bow and the quiver through the ordinary use
  item requests, paced at the 2 s flood protector window and held by
  the paperdoll confirmation gate;
- the pull is the ordinary forced attack request: with the bow in hand
  the server starts the ranged auto attack and the learned Power Shot
  casts through the combat skill flow. The hunter holds the position,
  so the attacked mob runs to the shooter and the cover does not (the
  character never entered its aggro radius);
- the arrival at 200 units swaps the melee weapon back and the fight
  continues as an ordinary one. A pull that never engages releases the
  target within 30 s to the ordinary skip machinery.

Pinned by `hunt/lure_test.go`.

## The archer kite (hunt/kite.go)

The typed fleet of the config launch (docs/launch_config.md) runs
`archer` bots: the ranged archetype whose weapon preference is the bow -
the gear plan arms it (the weapon milestone buys the bow, the quiver
restocks behind it, the arrows hold the left hand) and the fight NEVER
switches to a melee weapon (the archetype rule of issue #21: a cornered
archer holds ground and keeps shooting, it never degrades to a melee
trade). The standing archer's problem is the mob that closes: the
Mobius server AI stands the shooter still while the auto attack runs,
and a melee mob inside its swing range simply trades blow for blow with
a bot it could outrange. The kite is the answer: while the bow fight
runs, a hostile that crossed the retreat radius makes the character
step away, the movement window pauses the forced attack re-requests (a
forced attack request interrupts a running walk server-side, the same
contract the impending-add step follows), and once the step finished
the re-request shoots again from the opened distance. The cycle -
stand, shoot, step clear, shoot - trades a slice of damage speed for a
melee uptime the mob cannot pay. The layer landed in slices: the core
step (#18, PR #15), the edge cases - the train centroid, the cornered
hold, the lane fan of the early design (#19, PR #25; the fan retired
by the straight-lane rule of the 2026-11-09 round), the archetype
composition (#21, PR #31).

The radii of the ranged fight (hunt/kite.go, hunt/loop_params.go): the melee swing range
sits around 150 units and the chasers run 120+ units per second, so
the kite trigger `kiteRetreatRadius` (250) fires while the closing
hostile is still a second or two from the blows; the bow approach
`userBowEngageRadius` (450) is the outer edge of the standing fight,
and the optimal band between the two - 250 to 450 - is where the
standing archer is the good case (a fight sample beyond the chase
stall boundary `userBowStallRadius`, 650, belongs to the stall
watchdogs, not the kite). The band edges are pinned as references on
purpose: the outer edge IS the engage radius the fight fights with, a
copy of it in kite.go would drift.

The enemy weapon gate (the owner rule of the 2026-11-09 round): the
kite never engages against an archer. A ranged enemy cannot be
outrun - the retreat spends the character's bow cooldown on walking
while the enemy keeps shooting - so the standing trade is the answer,
the same hold the cornered fight follows. The NpcInfo equipment block
names the enemy template right-hand item id (the first int of the
`[rhand][chest][lhand]` group after the collision height), the
tracker resolves its weapon family through the generated item stats
(`npcdata.ItemGearStats`, the `BOW` family) and `kiteLayerGates`
closes every layer when the fight target carries a bow: the elven
sentinels Kendell and Starden (the Elven Bow, item 276) and the Kaboo
Orc Archer (the plain Bow, item 14) are the live cases. The gate
reads the fight target alone: a melee-target fight keeps the
established behavior, and an archer add in a mixed fight does not
switch the layer off. A natural-weapon mob (right hand 0) is not an
archer.

The same owner rule fences the archers out of the INITIATION
(`avoidBowTargets`, threaded into the constrained target search as
`avoidBows`): a bow profile never picks a bow-wielding mob as a fresh
hunt target, while an archer that attacks the character is still
answered through the attacker scan (`attackerEngageable`, the
defensive flow of the tick top). The fence exists because the
standing trade is a break-even race a low level archer loses the
moment a melee add joins: the 2026-11-09 22:42 acceptance run picked
a level 8 Kaboo Orc Archer, traded 19-31 damage shots at 385 units
for a minute (203 HP template against the 214 HP character kept
alive by potions), then a Grunt added and the two-mob pile-up fired
the emergency logout at 52 percent health - not a death, but a
session drop the survives check counts as a failure. The fence also
marks the skipped archer in the no-pick diagnostic ("bow wielder
(the archer trade is avoided)") and makes an archer-only ground read
empty, so the zone rotation moves the bot instead of leaving it in a
square it will never initiate in.

The state machine of one kite step (the tick ladder of the fighting
branch, loop.go - the kite sits after the engage/reposition rungs and
before the impending-add step):

- The admission (kiteStepAdmitted): the bow must be equipped and a
  fight target held; the shared movement window must be free (a
  running step or an impending-add walk owns it, combatAvoidUntil);
  and the kite pacing must have aged out (kiteStepPeriod, 3s - the
  cycle spends the step window walking and the rest standing and
  shooting, the period bounds the walking share from above). The
  streak counter (kiteStreakLimit, 8 consecutive steps per target;
  a fresh target resets it) is a DIAGNOSTIC threshold, never a
  gate - the unwinnable-race line lands once per crossing and the
  retreat keeps running.
- The threat (kiteThreat): the fight target or the nearest projected
  attacker (the NearestAttacker scan), whichever sits closer - a mob
  on a deck the walk cannot reach (the z gap past deckReachableZ) is
  no melee threat. The step arms when the threat closes inside 250.
- The direction (kiteTrainDirection): the centroid away-vector of the
  whole chaser train - the target's own away unit vector plus every
  SelfAttackers member within kiteTrainScanRange (800) on a reachable
  deck, each chaser weighing one unit. A train whose away vectors
  cancel (the summed length under kiteEncircleShare, 0.3, of its
  count - chasers stand on every side) has no centroid direction: the
  WIDEST-GAP bisector takes the step instead (kiteGapDirection - the
  perpendicular break of a two-mob line opens the distance to both
  chasers at once). The chaser positions are the raw last-known
  packet ones - the projected scan serves only the nearest chaser,
  and the 250 unit trigger margin absorbs the broadcast lag.
- The lane (kiteRetreatLane): the STRAIGHT ray of kiteTrainDirection
  as the primary direction (the owner rule of the 2026-11-09 round).
  The leg is the plain `kite.Step` (400) - the shot-cycle contract of
  the same round removed the race-length formula: the walk is cut at
  the bow disable end by the next shot, so a longer leg would only
  name a longer line the character never finishes - and it is
  ring-budgeted (kiteStraightLeg: the endpoint stays on the ring -
  kiteRoamRadius * kiteRingLeashShare = 972 units around the fight
  anchor, the target's stand - with kiteMinStep as the shortest hop).
  The lane
  runs the gate battery (kiteLaneResolve): the fight leash
  (kiteRoamRadius around the anchor), the away half-plane (the lane
  unit's retreat component under kiteHalfPlaneSlack, -0.05), the
  minimum useful hop (a validated point that collapses shorter than
  kiteMinStep is no lane) and the terrain gates that need the
  navigator - a wall behind (the geodata line of sight) or water
  behind (the OverWater oracle) closes the lane; without a navigator
  (the no-geodata runtime) the leash alone fences the step. The old
  fan, the camp deflection and the circling chord are GONE: a turn
  spends the distance the retreat just bought, and the fights whose
  speeds sit too close cannot afford it. When EVERY rung of the
  preferred ray is walled, the corner-only escape battery
  (kiteLaneEscapeAngles) walks the closest straight bearing the
  terrain offers - +-22.5, 45, 67.5 and 90 degrees from the away-ray,
  all still inside the away half-plane - and a scene every angle
  refuses holds the ground and shoots. The escape never replaces a
  walking direct rung and never fans wide of the perpendicular: a
  turn only spends the distance where there was no straight walk to
  spend.
- The step: the window is stamped (combatAvoidUntil = now +
  kiteWalkWindow + kiteReengageDelay 0 - the C1 bow disable window,
  the fallback kiteStepWindow 2s), the streak counts up, the log line
  lands ("kiting clear (step N of 8)"), and the walk issues. The
  forced attack re-requests of the engage branch wait out the window -
  the re-engage knob (kiteReengageDelay, 0 today: the window end IS the
  re-engage) is pinned for the live tuning round of #29. The walk
  window is NOT scaled by the leg length: the window end is the bow
  disable end where the next shot owns the tick, whatever distance
  the character covered meanwhile.

The fallbacks, in the order the machine meets them:

- The encircled train: the centroid cancelled, every step direction
  walks INTO a chaser - the widest-gap bisector still names the
  straight ray of the widest opening, and only when that whole ray
  is walled does the archer hold ground and shoot through the train
  ("the chasers surround the bow fight ..., holding ground and
  shooting through the train"), the pile up and losing fight
  machinery still owning a train that outdamages the standing fight.
- The cornered hold: no walkable straight lane - a wall, water, the
  leash or a refused endpoint. The archetype rule answers: the
  archer stops retreating and KEEPS SHOOTING the bow at melee range
  ("no walkable retreat lane ... (cornered), holding ground and
  shooting the bow"). The hold re-probes at the kite pacing, not
  every tick - a failed probe never becomes the idle stutter - and
  the diagnostic lands once per hold episode (kiteHoldLogPeriod,
  15s): a standing fight that names itself stays diagnosable. The
  hunt loop's watchdogs stay quiet through it: the stuck timeout
  only owns a fight that never started (the cornered fight runs),
  and the chase stall watchdog only owns the stretches beyond 650
  (the cornered fight sits at ~150).
- The streak limit: crossing 8 steps names the unwinnable distance
  race in the log, but the retreat keeps running straight - the
  answer to a chaser at least as fast as the character is never the
  standing fight (it was the melee damage the kite exists to avoid).
  The losing fight machinery (the panic run) still owns the death
  risk separately.
- The refused walk: a retreat the gates cleared but the server
  refused (the click validation collapses, a mob body sits on the
  click point) surfaces through the re-click ladder - the spam keeps
  the endpoint until the probe age, then the hold owns the cycle.
  The ladder keeps pushing the SAME endpoint: the corner-only escape
  angles belong to the lane battery of a NEW walk, not to the click
  retries - a live turn per click would spend the distance the
  retreat just bought.

The knobs of kite.go as pinned on main (the tuning round of #29
adjusts the first four from live measurements; the band edges above
are references, not copies):

| constant | value | meaning |
|---|---|---|
| `kiteRetreatRadius` | 250.0 | the trigger distance and the inner edge of the optimal band |
| `kiteStep` | 400.0 | the retreat length, sized so the distance after the step lands back inside the band |
| `kiteStepWindow` | 2s | the movement window fallback (the bow disable window of the live pAtkSpd wins) |
| `kiteReengageDelay` | 0 | the hold between the walk end and the first re-request |
| `kiteStepPeriod` | 3s | the step pacing - bounds the walking share of the cycle |
| `kiteStreakLimit` | 8 | the consecutive step bound per target, then the fight |
| `kiteTrainScanRange` | 800.0 | the chaser scan band of the retreat direction |
| `kiteEncircleShare` | 0.3 | the encirclement threshold (hold and shoot through) |
| `kiteMinStep` | 250.0 | the shortest retreat hop the ring budget may leave |
| `kiteRingLeashShare` | 0.72 | the straight leg's cap as a share of the roam radius (the 972 unit ring) |
| `kiteHalfPlaneSlack` | -0.05 | the away half-plane guard (no fold-back into the train) |
| `kiteHoldLogPeriod` | 15s | the cornered hold diagnostic pacing |
| `kiteShotWindow` | 700ms | the freshness window of the own-shot broadcast that arms the shot-paced retreat |
| `kiteReclickPeriod` | 250ms | the re-click pace of the kite walk ladder (the tick cadence - the owner ask: spam much faster) |
| `kiteReclickLimit` | 8 | the re-click bound of one kite walk (the packet budget of a dead transport) |
| `kiteProbeElapsed` | 1.2s | the silent-verdict age of the walk (past the movement broadcast gate - the H-006 signature) |
| `kiteRefusalAnswerWindow` | 1s | the round-trip window of the ActionFailed attribution |
| `kiteBowReuseDelay` | 1500 | the reuse delay of the bow family (the C1 item data) feeding the walk window formula |
| `kiteWindupLead` | 150ms | the safety lead past the windup end before the deferred retreat clicks |

### The shot-paced retreat (issue #60)

The proximity trigger of the ladder above arms the step only after
the hostile crossed the 250 unit radius - a fast chaser spends the
shot cycle closing unopposed and reaches the swings between the
steps (the 2026-09-22 dump of issue #60: the mob glued at 12-57
units, one 10 damage blow per second through every kite step). The
shot-paced layer flips the trigger: the character's own Attack
broadcast arms the same retreat the moment the shot released, so the
bow cooldown is spent walking and the cycle becomes shoot, run the
reload, gain distance, shoot again - the melee uptime drops to the
stand moments and a chaser slower than the character never reaches
the swings.

The server facts the layer rides (verified by the Mobius C1 source
read, `Creature.doAttack` / `doAttackHitByBow` / `onHitTimer`, and
the shot cadence of the issue #60 dump):

- The Attack packet is the shot COMMIT, not the hit: the hit roll,
  the arrow consumption and the HitTask schedule all happen before
  the packet broadcast, and the damage task carries no attacker
  movement check - moving after the broadcast never cancels the
  shot.
- The bow disable window is `timeAtk + reuse`
  (`calculateTimeBetweenAttacks` 500000/pAtkSpd plus
  `calculateReuseTime` reuseDelay*333/pAtkSpd - roughly three
  seconds for the Short Bow), mirrored to the client by the
  SetupGauge(RED) packet and the "Getting ready to shoot arrows"
  message. The 2s step window fits inside it.
- A forced attack request while moving stops the walk and attacks
  (`stopMove` before the launch) - the re-request after the step
  window owns the re-engage, the same contract the proximity path
  uses.

The layer (kiteFromShot) shares the whole step machinery with the
proximity path - the straight train direction, the leash, the wall
and the water gates, the cornered and encircled holds, the movement
window - and differs in three rules:

- The trigger: the shot broadcast inside kiteShotWindow (700ms - the
  loop ticks four times a second, the broadcast stays catchable for
  at least two ticks) while the nearest hostile holds the pursue
  band (inside the bow engage radius, 450 - a mob that keeps
  chasing). Beyond the band the standing fight and the stall
  watchdog's re-approach own the scene.
- The pacing: the shot cycle paces the steps (the 3s step period
  would collide with the ~3s bow cooldown); the step re-arms on the
  next broadcast, the movement window guards the double trigger.
- The streak exemption: the rhythm does NOT count into the
  kiteStreak limit. The bound exists because an endless step race
  starves the fight of every swing - the rhythm keeps swinging once
  per cooldown, so the shuffle bound does not apply to the fight
  itself. The proximity path keeps its own counting (the
  composition test pins the bookkeeping through the chase-fresh
  scene).

The telemetry marker of the layer: the step line reads "shot
released on <target>, hostile <id> holds <dist> units, kiting the
reload".

The telemetry anchors: the step line ("kiting clear (step"), the
shot-cycle line ("kiting the reload"), the hold line ("holding
ground and shooting") and the kill line ("died, looting") are the
markers the archer acceptance scenario counts
(acceptance/archer_kite.go, the ranged farm contract of issue #28).
The scenario asserts the same contract this section documents: the
same-fight shot gaps sit inside the C1 bow cooldown (the shots
follow the reload, not the old 20-30 s treadmill), the STANDING
fight samples stay inside the bow band (the retreat legs
legitimately open wider), the kite lines narrate repeated retreats,
the fights resolve (one kill per two minutes at least), the online
status and the HP hold through the window, and the arrow stock
never empties (the restock gates of the gear plan feed the quiver).

### The re-click ladder of the kite walk (issues #60, rounds two and three)

The first round of the shot-paced retreat shipped the rhythm - the
retreat arms at the own Attack broadcast, the dump showed a "shot
released" line every cycle - but the owner's follow-up dumps showed
the WALK barely executing: "moving: no" through every reload while
the mob closed ~380 units per cycle. The manual clicking the owner
demonstrated behind the character ("it runs 2 seconds no problem,
covering large distance") proved the server accepts the retreat
clicks fine when they arrive right - the single bot click of each
cycle was the problem, not the lane.

The third-round acceptance dump named the dominant dead-click
mechanism exactly: the server answers SOME destination cells with an
instant bare ActionFailed (the send and the answer land the same
millisecond) while a lane a few cells aside walks fine. The
MoveToLocation handler refusal family is the source (the
Mobius C1 packet handler runs `isCompletelyBlocked` on the
destination cell - among other checks - BEFORE any movement starts),
and the bot's own geodata reads the dump's refused cells open (the
offline probe of the round: every dead endpoint of the dump passes
the shipped pack's own cell check), so only the ONLINE evidence
settles a refusal. The three candidate mechanisms the ladder
answers:

- The destination-cell refusal (the dominant one, the dump
  evidence): the server refuses the cell outright - a refused cell
  never starts a walk, and under the straight-lane rule of the
  2026-11-09 owner round there is no fan to rotate onto. The spam
  keeps the endpoint while the walk is younger than the broadcast
  gate, then the dead verdict stands the ladder down and the hold
  keeps the bow shooting.
- The stance transition swallow (the round-two theory): the click
  lands ~250ms after the own Attack broadcast while the server side
  ATTACK intention of the just-committed shot is still tearing down.
  The plain same-endpoint retry at the 250ms pace answers it.
- The H-006 silent drop: the deployment that swallows accepted move
  requests whole (no broadcast, no ActionFailed). The silent probe
  answers it - a walk unanswered past kiteProbeElapsed with the
  character still on the issue cell names the endpoint dead.

The ladder (kiteReclickWalk, armed by kiteIssueWalk on every kite
step of both layers) replicates the manual behavior, faster per the
third-round ask: while the movement window runs and the character
still stands on the issue cell - the SelfWalking tracker flag is the
oracle, the issue cell the baseline - the click goes out again every
kiteReclickPeriod (250ms, the tick cadence; a re-click inside the
once-per-second broadcast gate just re-aims the same endpoint
server-side, the click is not wasted). The refusal evidence
(kiteClickRefused - the ActionFailed answer the server gave the last
click, correlated by the send time the same way the town walker's
refusalEvidence runs) latches the dead verdict at once, but the spam
keeps going while the walk is younger than kiteProbeElapsed: the age
gate separates a genuine click refusal from the straggler refusals
of the server AI's own attack retries (the bow disable path answers
ActionFailed at a one-second cadence while the character stands) and
gives the movement broadcast of a healthy click its chance. The
silent probe is the H-006 fallback: a walk unanswered past the same
age names the endpoint dead. Either verdict past the age stands the
ladder down - the hold owns the cycle, the straight design has no
second lane to rotate onto, and the next shot cycle re-arms the
retreat fresh. The ladder also stands down when the walk runs (the
movement broadcast), the leg completes within kiteArrivalEpsilon of
the endpoint, the character re-bases on a fresh standing cell
mid-leg (the spam continues against the SAME endpoint), or the
window ends (the forced attack re-request owns the tick).

The walk WINDOW itself is the bow-aware one now (kiteWalkWindow -
the third-round ask: the proper archering delays from the C1 Mobius
formulas): the C1 disable window the server enforces between the
shots, timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
(Creature.calculateTimeBetweenAttacks and calculateReuseTime), read
from the live pAtkSpd the StatusUpdate broadcasts (attribute 0x12 -
the acceptance dump shows 337 for the Short Bow kit, giving roughly
(500000 + 1500*333)/337 = 2.97s, the "3 seconds to draw shot" the
owner measured by hand; the reuse 1500 is the bow family value of
the C1 item data). The walk then spends the WHOLE cooldown running -
if it is not shooting it is moving away from the target - and the
forced attack re-request lands the moment the window ends (the
engage retry pacing of 1s already sat inside the window), closing
the cycle to walk-the-disable, shoot, walk again. A missing or
absurd pAtkSpd broadcast falls back to the shipped kiteStepWindow
(2s) - the walk contract never depends on the packet being parsed.

The dump markers the ladder adds, and what each names: the refusal
verdict line ("the kite walk click on target ... was refused by the
server") past the probe age names the destination-cell refusal and
the hold that follows it; the silent probe line ("never started the
movement") with nothing ever moving across the whole ladder names
the H-006 silent drop; the hold line ("no walkable retreat lane ...
(cornered), holding ground and shooting the bow") names the
cornered-by-refusal answer of the straight design.

The measured numbers of the live tuning round (issue #29: the kited
archer against the standing-archer baseline - a bow bot with the kite
disabled - on the deployment stack) land in the table below; every
parameter change of the round arrives with its before/after row.

| metric | standing baseline | kited (current knobs) | kited (tuned) |
|---|---|---|---|
| retreat frequency per fight | - | - | - |
| melee fallback rate (blows taken in melee reach) | - | - | - |
| kill rate | - | - | - |
| death rate | - | - | - |

### The straight retreat leg (issue #70, rounds 17-19; the 2026-11-09 owner rework)

The round-17/18 live verdicts replaced the window-sized shuffle with a
continuous race the shot cycle rides (the full record lives in
`docs/progress/2026-09-23-issue70-kite-timing-probe.md`). The
2026-11-09 owner round removed the turning geometry of that design
(the circle, the chords, the camp deflection, the pocket breakouts):
when the chaser's speed sits close to the character's, a turn spends
the distance the retreat just bought, so the leg runs straight away
from the train and the archer shoots the moment the bow is ready.

- The LEG is the plain `kite.Step` (400): the shot-cycle contract of
  the round removed the race-length formula (`kiteStepLength`, the
  re-shot floor and the parity exchange rate). The walk is cut at the
  bow disable end by the forced attack that fires the next shot, so a
  longer leg would only name a line the character never finishes; the
  400 unit endpoint keeps the line of the retreat and the arrival
  geometry of a genuinely completed walk.
- The STRAIGHT LANE (`kiteStraightLeg`): the leg direction is the
  away-ray of the whole chaser train and nothing else. The endpoint
  stays on the fight's ring - `kiteRoamRadius * kiteRingLeashShare` =
  972 units around the anchor, the tallest hop the leash leaves capped
  by `kiteMinStep` (250) as the shortest - and the lane battery
  resolves the leash, the away half-plane, the geodata wall and the
  water gate against it. A blocked straight lane holds the ground:
  the owner rule of the 2026-11-09 round admits no fan, no deflection
  and no circling chord.
- The ANCHOR is the MOB'S STAND (`kiteResolveAndClick`), not the
  character's first-retreat cell: the reopening live audit measured the
  self anchor parking the fight at the 1500-unit drift leash, where
  every lane pointed inward and the away half-plane refused it (the
  fence-edge hold). The opening straight leg budgets against the ring
  around the mob's stand (`kiteStraightLeg`), so the fight opens on
  the ring it keeps.
- The WINDOW is the bow disable window and NOT scaled by the walked
  leg (`kiteIssueWalk`): the C1 formula at the live pAtkSpd (roughly
  2.97s for the Short Bow at 337, the fallback 2s) ends exactly where
  the forced attack fires the next shot, so the window end and the
  re-shot are the same moment whatever distance the walk covered.
- The ARRIVAL (`kiteReclickWalk`): a stand within `kiteArrivalEpsilon`
  (100) of the endpoint completes the leg and stops the re-clicks -
  but the movement hold survives to the bow disable end: the forced
  attack must not fire at the collapsed arrival range, because that
  shot would commit its windup with the chaser standing on top of the
  character. A mid-leg stand short of the endpoint re-bases the
  ladder onto the current cell and keeps pushing the SAME endpoint
  (the spam continues), and a dead endpoint holds the ground instead
  of turning.
- The CELL-FOLLOW (`cellHunter.applyFollowing`): a fight that crosses a
  hexagon boundary keeps its target when the held ground follows it -
  the follow is bookkeeping, not a travel reset. The round-19 live log
  measured the old `apply` dropping the fight target twice inside 40s
  on every cell boundary the kite crossed.

### The continuous retreat (the 2026-11-09 owner round, issue #0)

The owner instruction of the round replaced the stop-and-shoot cycle
with a continuous one: shoot, run - the moment the cooldown is back,
shoot again - and the chaser must spend the reload tail closing the
distance again instead of the archer standing through it. The round
questioned the shipped layer AND the tests with it; runs 5 and 6 of
2026-11-09 verified the first shape, and the 5-minute runs that
followed removed its last safe-distance gates.

- The ONE shot per cycle is the contract. The shot broadcast arms the
  deferred retreat (the click at the windup end, `kiteShotPhase`),
  the walk banks the rest of the bow disable window, and the forced
  attack request at the window end IS the next shot - at ANY range
  the chaser reached meanwhile. The route is: shot -> windup-end
  retreat click -> walk the reload tail -> forced attack at the
  disable end -> the next broadcast arms the next retreat. There is
  no re-shot floor, no safe-distance gate and no pursuit ledger any
  more: the 5-minute acceptance run measured what they cost against
  an equal-speed chaser (20-30 s between the shots, the mob
  regenerating faster than the arrows, the fight never resolving and
  the drift carrying the session into a level-15 zone).
- The walk window is the bow disable window (`kiteWalkWindow`, the C1
  formula on the live pAtkSpd, the fallback `kiteStepWindow` 2s), not
  scaled by the leg. The arrival within `kiteArrivalEpsilon` stops the
  re-click ladder but KEEPS the movement hold to the disable end: the
  forced attack must not fire at the collapsed arrival range, and a
  walk that never finished still spends the whole reload moving.
- The cornered and encircled holds keep their shot (a walled lane is
  the one legitimate standing-shoot case); the retire-click ladder
  and its dead-endpoint verdicts stand the ladder down, and the next
  shot cycle re-arms the retreat fresh.
- The distance floor reads the CONSERVATIVE closed distance
  (`closedDistance`, `Bot.ObjectTrackOf`): the server broadcasts a
  chasing mob's `MoveToPawn` at a one to two second cadence, and the
  live acceptance measured a 512 unit raw gap against an actual 256.
  The bound assumes the chaser kept running at its FASTEST speed for
  the whole age of its movement segment, so the threat scans only
  ever see a distance at or below the true gap.
- The click validation rejects a partial run: a validated point that
  collapses onto a stand shorter than `kiteMinStep` (250) is no lane
  (the live run issued a 38 unit "walk" and stood through the next
  cycle). A genuine shorter hop cannot pass the rung ladder anyway -
  it floors at `kiteMinStep`.
- The angular escape is the corner-only fallback of the straight
  rule (`kiteLaneEscapeAngles`): when EVERY rung of the preferred
  away-ray is walled, the battery walks the closest straight bearing
  the terrain offers - +-22.5, 45, 67.5 and 90 degrees, all inside
  the away half-plane - and a scene every angle refuses stays the
  cornered hold. The live 2026-11-09 spot (34792, 49416) measured
  the direct west ray walled at every length while the south, east
  and north rays stood open at 250-900 units; a hard hold stood the
  archer at melee for the whole fight. The primary retreat is never
  bent, never fans wide of the perpendicular and never folds back
  toward the train.

### The defensive flight of the archer (the run-3 finding)

The defensive aggro answer (`fleeFromThreat`) used to open with the
melee fan, whose escape lanes are clamped to the CURRENT hunting
zone box. A kite fight drifts across the cell boundaries, and the
run-3 acceptance dump measured the failure exactly: the fan walked a
competent 700 units while the parity chaser kept pace at 38, the
closure watchdog (`fleeLogoutAfter` / the `escapeNotOpening` probe)
named the held gap, and the session logged out MID-KITE with the bow
healthy and the quiver full.

The flight (`kiteFleeThreat`) hooks in first: a bow in hand, the
kite enabled and a melee attacker holding the character make the
flee fly the same straight away-ray the fight cycle uses
(`kiteTrainDirection`), walked at the full escape segment
(`escapeWalkDistance`, 700) through the shared walk seam - its own
window, the re-click ladder of the dead transports and the stand-up
guard. A targetless flee has no pursuit continuation (the hold is a
no-op with target 0), so the flight IS the continuation: a lapsed
window reports free and the next straight leg issues on the same
tick.

The flight deliberately bypasses the fan's budgets (the closure
watchdog and the logout after): the straight run IS the designed
archer answer, and a mob that keeps pace with it is outlasted by the
server's own attack timeout (`AttackableAI` `MAX_ATTACK_TIMEOUT`,
two minutes without a landed hit, then the aggro clears and the mob
teleports home) instead of the session logout. The critical-HP
logout of the tick top stays armed above both.

The fan never answers an ARMED bow character - the kite layer enabled
and the bow in hand (the `kiteLayerArmed` guard of `fleeFromThreat`).
A melee weapon in hand or a disabled kite profile keeps the plain
fan: those are the melee answers, not the archer's. The zone-clamped
lanes also logged a healthy 99 percent flight out 30 s after it left
the cell box (the run-24 acceptance dump), so the archer's refusal
answers never fall through to them. A flight tick the layer refuses
(a bow attacker, another deck, no bearing yet) is answered by the
hunt's standing trade, a walled lane is held by the flight itself,
and only the critical-HP logout (< 12 percent) still resets the
session.

The flight geometry around the refusal:

- The bow windup owns the first click (`kiteFlightMovable`): the
  server SAVES a move issued inside the windup and replays it at the
  disable end (`PlayerAI.setIntentionMoveTo`), and the escape stall
  watchdog read the saved seconds as a dead click (the run-24 +131 s
  reset at 99 percent health). The flight issues its first leg on the
  accepted side of the cycle and arms nothing on a deferred click.
- A chaser standing ON the character has no away bearing (the train
  geometry skips a member at zero distance): the flight takes the
  first open ray of the contact ring - the facing, the two
  perpendiculars, the back ray - at the full escape segment. Contact
  is the one scene the ground hold never owns (the run-29 dump
  stood 21 s inside a level 10 Fighter, health 78 -> 11 percent,
  then the emergency logout).
- A walled away half-plane gets a breakout attempt before the hold:
  the back half-plane - the ground behind the character, THROUGH the
  chaser's bearing - is walked when the away ray and its corner
  escapes all refuse. The run-24 flight ended in a north-west pocket
  whose open ground lay back south-east through the train.
- A full pocket holds the ground and re-probes at the kite period,
  KEEPING the ticks (the flight owns the defensive answer end to
  end); the diagnostic lands once per hold episode. The flight line
  reads "<mob> holds the bow flee at N units, kiting clear"; the
  breakout logs "the away lane from <mob> is walled, breaking out
  through the open back lane"; the full pocket logs "no straight bow
  flee lane opens from the chase" without handing the fan a tick.

The fight side of the same round:

- A bow-wielding add invalidates the fight: `HasBowAttackerBesides`
  turns the losing verdict on before the nearly-dead exemption, the
  pick is dropped and the straight flight owns the escape (a ranged
  add outranges the retreat, so its reload-tail fire is free damage).
  The targetless answer does not re-adopt a just-fled attacker and
  keeps flying while a bow add is present.
- The dropped fight leaves no selection behind: `fleeFromTarget`
  sends the clear self click (the same official unselect the town
  trips use, `clearServerSelection`), and the answered
  `MyTargetSelected` of the own object zeroes the tracked fight
  target (`Bot.ApplySelfTarget`). Without it the attack stance
  lingering on the incoming blows kept reporting the abandoned mob
  as a live fight - the run-31 acceptance sampled it at 868 units
  during a healthy flight and failed the bow band.
- A dead or removed target stops being a fight: the tracker drops
  the tracked fight target with the selection
  (`clearSelfTargetLocked`), so the loot walk of the kill and the
  stance lingering after the last blow never sample as a far fight.
  The run-33 acceptance sampled the corpse of the mob it had just
  killed at 781 units on the loot stop and failed the bow band by
  one unit, while no shot of the window was released beyond 426
  units.
- A pile up holding an attacker ABOVE the engage ceiling converts
  the STANDING kite into the flight (`dropFightForFlight`): the shot
  cycle spends half of every reload inside the melee range of a mob
  the arrows cannot trade with; a pack inside the ceiling is tanked
  instead (see the pile up policy and the ceiling section). The kill
  window (`pileUpKillWindowPercent`, 30) is the exception: a target
  one or two arrows from death keeps the fight, because the flight
  throws the kill away - the run-24 dump dropped a 24 percent grunt
  for a 30 s flight into the pocket.

### The kite engage ceiling (the run-4 finding, reverted by run 28)

The general pick ceiling (`targetMaxLevelSlack`, character level
plus two) protects the MELEE trade: the higher level lands more and
takes less. The run-4 acceptance dump measured the ceiling refusing
a level 10 Kaboo Orc Fighter (template 20471) on the level 7 ground
(`elven-hex-104`, band 7-10): the targetless pick fled it, the
defensive flight dragged the session across the map and the archer
spent the window shooting nothing. The round answered with a
kite-only ceiling of level plus five
(`kiteEngageLevelSlack` / `kiteAnswersMelee` /
`kiteEngageLevelCeiling`) and measured the opposite cost in run 28:
the confirmation window adopted exactly that level 10 Fighter into
the fight against a Spore Fungus, the pair took the archer from 100
to 53-59 percent in 16 s (about half of the arrows missed, 13-16
damage per Fighter swing at 163 HP each) and the health gate dropped
every fight at ~59 percent - 1 kill of the wanted 2 in the window.

The widened ceiling is REVERTED. `attackerEngageable` and
`pileUpWinnable` (`loop_safety.go`) read the general ceiling for
every profile, the armed archer included: a within-ceiling pack is
TANKED (run 27 farmed its kills this way - the melee rule plus the
bow rhythm), and only a pack holding an above-ceiling attacker
converts into the straight flight (`dropFightForFlight`). A
bow-wielding attacker is never a kite answer - the anti-archer gate
refuses it at every level, and the bow-add verdict drops the fight
for the flight (see the pile up section).

The flight that answers the above-ceiling attacker is bounded by
the server, not by the bot: `fleeSkipDelay` is 3 minutes and
`kiteFlightHolds` keeps the flight in charge while any attacker
still carries a live skip, because Mobius C1 `AttackableAI`
`MAX_ATTACK_TIMEOUT` (1200 ticks of 100 ms) drops the aggro of a mob
that cannot land a hit two minutes after its last swing. A 2-minute
skip lapsed into a re-adoption in run 28 exactly there.

### The disengage of the losing fight (the run-7 finding)

The lost-target gate of the tick top (`losingFight`: the health
under `escapeHealthPercent`, 25, or a 25 percent health lead of the
target over a hurt character) used to skip the archer kite
(`!l.kiteOwnsRetreat()`), because at the time the drop handed the
melee flee fan a zone box the kite ground had already left. The
run-3 round then added the straight `kiteFleeThreat`, which answers
exactly that scene, so the suppression became the death path: the
5-minute run 7 of 2026-11-09 died at +223s inside a two-mob pile-up
at the Green Dryad boundary with the HP falling 100 -> 69 -> 32 ->
21 percent while the pursuit chain kept walking at 14-38 units and
the critical-HP logout (12 percent) never saw a tick under its
line. The gate now drops the fight for the kite too - the targetless
flight is the escape, and the pile-up run and the critical-HP logout
keep their own gates above it. The updated test
(`TestKiteDisengagesTheLosingFight`) pins the drop and the straight
flight at 20 percent health, no logout above 12 percent.

### The standing-sample band verdict (the run-8 finding)

The acceptance range check latches the farthest fight distance from
the tracker samples, and a walking sample is not a standing fight:
a retreat leg deliberately opens 700-900 units from the lagging
chaser, so the run-8 window measured a 796 unit "fight" on a 780
ceiling while every retreat step was healthy. `archerKiteSample`
now carries the `walking` flag (`Bot.SelfWalking`) and `fold` only
latches `maxFightDist` on a standing sample - the fight/ranged
counts keep every sample. Run 9 (5 minutes, the bow fence of the
next section) measured 107 fight samples, 71 percent ranged, the
farthest STANDING fight at 527 units, 19 kite steps, 0 holds, the
minimum HP at 61 percent and the minimum quiver at 590 arrows: all
checks green.

### The bow-mob pick fence (the run-9 finding)

Run 9's first 5-minute attempt before the fence (the 22:42 window)
died the safe way: the pick selected a Kaboo Orc Archer, the
anti-archer gate kept the kite off (correct - a ranged enemy cannot
be outrun), the standing trade ground the HP down through potions,
a Grunt added at +57s and the two-mob pile-up fired the emergency
logout at 52 percent health - a session drop the survives check
counts as a failure (no death happened; the relogin landed alive).
The pick fence of `avoidBowTargets` (see the enemy weapon gate
above) keeps the archer profile initiating only on melee mobs and
turned the same 5-minute ground green on the next run: 107 samples,
71 percent ranged, 19 steps, 0 holds, min HP 61 percent, min arrows
590.

The acceptance band verdict reads standing samples only (see the
previous section), so the walking legs the fence redirects cannot
fail the range check.

## Hexagon cell hunting (hunt/cell*.go, hunt/cells_elven.go)

The cell mode replaces the square zone ladder of the elven lands (the
design and the full reasoning live in `docs/hunting_cells.md`;
`main.go` wires it through `SetHuntingZoneRegion("elven")` ->
`SetHuntingCellRegion`). The partition is a UNIFORM hexagon grid
(one circumradius over the whole map) that owns every spawn point of
the ground exactly once (the analytic point-to-hex assignment - the
half-covered respawn failure of the circle geometry cannot build),
the hexagon extent stays inside the knownlist circle from anywhere in
its patrol square (the emptiness reading never lies about the load
boundary), and the rotation moves through the grid adjacency paced
by the respawn ripeness: a ground cleared at T is unripe until T +
its respawn window, so the hunter cycles its neighborhood at the
respawn rate - the walks stay short, no ground depletes while its
neighbor saturates. The hunt roams enemy-first: the pick takes the
nearest VISIBLE enemy wherever it stands (no held-hexagon fence for
new fights), the far walk goes to the zone that holds the enemies,
the walk home fires only when nothing pickable is visible at all,
and the held hexagon follows the actual fight ground (the map
highlight tracks where the bot really farms); the kills of a ranged
shot are looted at the corpse - the bot approaches and picks up the
drops and the adena, never leaves them on the ground. The economy of
the spot era carries over (the measured income, the death heat, the
occupancy division of the fleet, the white-green window, the respawn
overlay with the corpse camping); the manual ground selection is
gone (the registry of a full project grows past a thousand cells -
the economy owns the rotation, the map hover shows the cell info
read-only). The web map draws EXACTLY TWO hexagons: the one the bot
fights in (the active cell highlight with the live label) and the
one under the cursor; the static mesh is served per version through
`/api/hunt-mesh`.

The legacy square system stays for the manual `SetHuntingZones`
setups and the generated registries of the regions that have not
migrated yet; `SetHuntingZones` stands the cell mode down and
`SetHuntingCells` stands the zone mode down - one loop, two
registries.

## Multi-zone hunting (hunt/zones.go, hunt/zones_elven.go)

The hunting squares derive from the real spawn polygons - a Mobius
territory spawns its mobs at uniformly random points of its polygon
(`NpcSpawnTerritory.getRandomPoint`) and the random walk stays inside
it (the wander target must pass `isInsideZone`, `MaxDriftRange` 300
leashes the rest), so a square anchored on a "cluster centroid" misses
most of the ground (the measured spawn mass coverage of the old hand
placed thirty squares was 18 percent). `tools/generate_hunt_zones.py`
parses ElvenStarting.xml plus the npc stats and generates 227 compact
squares (1300-1900 halves, one square for a small territory, a density
adaptive grid partition for a big one, same-band sub-territories folded
into their parents) covering 96 percent of the spawn mass in the ten
mob level bands (keltirs 1-3 gear 0, wolves 3-4 gear 20, raiders 4-6
gear 50, goblins 5-7 gear 70, grunts 7-8 gear 100, fighters 8-10 gear
140, lieutenants 9-12 gear 180, leaders 11-13 gear 220, elders 12-14
gear 260, spiders 13-16 gear 300, lirein 16-19 gear 380).

- Every square carries the full mob list of its territory (`ZoneMob`
  with a level-sorted `Priority`), so the engage farms every species of
  the ground - the priorities bias the pick by `targetPriorityBias`
  (200 units per point) through `state.Bot.NearestAttackablePreferred`,
  tilting the fight toward the exp richer mobs while a far preferred
  mob still loses to a doorstep one.
- `PickHuntingZone` gates on level AND gear points - a band opens only
  above its top mob level plus the lead (`zoneLevelLead` 1), so the
  character always hunts mobs 1-2 levels below itself instead of
  engaging mobs above its own level - keeps the current zone of the
  winning band (the 30 s re-pick never bounces between same-band
  grounds) and takes the nearest ground of an open band; a character
  below every band (or under a death cap that closed the ladder) falls
  back to the starter band contest (the nearest ground of the first
  band, the current ground keeps its post).
- A cleared-out square rotates: no attackable mob inside the square
  that passes the engage level ceiling for 10 s while the hunter stands
  central (the `ZoneHasAttackableBelow` reading - a square whose
  survivors all sit above the max target level is as good as empty)
  moves it to the nearest non-cooling sibling of the same band, and the
  rotated-away square keeps a 40 s empty cooldown (`zoneEmptyCooldown`)
  so the rotation sweeps forward through the band (A to B to C) instead
  of ping ponging back into the square it just left (A to B to A) - the
  fights, rests, walks and town trips reset the timer.
- The death regression: three deaths in one square demote its whole
  band, the ladder caps below it until the level changes (a level up or
  a delevel resets the bookkeeping; delevel-phase deaths never count);
  deaths count against the square they happen in (the position lookup -
  an emergency logout death lands on a fresh session before any zone
  pick), a manual zone override dies with the demotion.
- A manual zone selection stops the walks aimed at the old square
  (`stopForZoneSwitch`: a manual move, a town trip walk and a zone
  return segment are cancelled, one walk request to the current spot
  replaces the running server walk; the deleveling refuses the stop
  like every movement command, the selling stop keeps running and its
  return segment re-targets the new zone). The map draws every zone and
  the floating collapsible zone panel of the map switches zones
  manually (the `zone` command, index in the Count field; the override
  holds until the character outgrows the band or dies it out) - see
  docs/webui.md.

## The Dion 20-25 band registry (hunt/zones_dion.go)

The first registry beyond the elven ladder: the hunting grounds of
the 20-25 character band (the M3 survey of
docs/band_20_25_survey.md) generated by `tools/generate_hunt_zones.py
--dion` from the three Dion spawn sources (ExecutionGrounds,
CrumaMarshlands, PlainsOfDion) in the shape of `zones_elven.go` - the
same polygon partition into spawn-true squares, every square carrying
the full mob list of its territory, the territory filter keeping the
grounds with at least one mob of the 20-25 window (the survey
territory lists, the over-25 neighbors of a ground stay in its mob
list). 75 squares over 25 kept territories (the two same-band sub
polygons fold into their parents), 96 percent spawn mass coverage,
the same compact halves (1400-1800).

The five band windows continue the elven ladder onto the D-grade
dress stages (the gear gates calibrated against the
`gear.TotalGearPoints` probes of the buyable dress: the NG full dress
284, the Falchion dress 292, Bastard+bone 312, the partial mithril
step 341, the full D dress 391):

| Band | Mob levels | Gear gate | The grounds (the survey tables) |
| --- | --- | --- | --- |
| Sprout grounds | 20-21 | 280 | the passive mandragora sprouts of the Execution Grounds - the entry ground of the band |
| Northern plains | 23-24 | 290 | the aggressive dire wolf grounds of the walkable Dion reach |
| Sapling and blossom | 24-25 | 310 | the mandragora sapling+blossom, monster eye gazer and kadif werewolf grounds |
| Specter and destroyer | 25-26 | 340 | the specter, gray ant and monster eye destroyer grounds |
| Cruma and jaguar | 26-27 | 380 | the cruma marsh edge (leech/ant/ripper) and the glass jaguar squares |

The starter fallback anchor is the Trisha "Execution Ground" teleport
arrival (46165 150008): a character below every band (a fresh level
20) hunts the nearest sprout square of the arrival through the
starter contest - the survey's "safe to approach" entry - and the
ladder opens the bands at their top mob level plus the lead (the
wolves at 25, the sapling+blossom grounds at 26, the specter grounds
at 27, cruma at 28). `zones_dion_test.go` pins the survey tables as
the acceptance reference: the thirteen mob species at their table
levels, the sprout/cruma ground mob sets, the walking distances of
the survey anchors (the sprout squares within 10k of the execution
arrival, the cruma squares within 12k of the cruma center, every
square within 18.5k of a teleport arrival), the picker ladder on the
dress stages and the death cap.

A deployment selects the band explicitly through
`SetHuntingZoneRegion("dion")`: the loop installs
`DionHuntingZones()` (the zone registry stands the cell mode down)
and records the region for the gear catalog selection of the shopping
trips (`shopCatalogForRegion` selects the Dion merchants at the 20
percent Dion tax, hunt/shopping.go of T-017). The default elven flow
is unchanged. The survey protocol gates the production band entry on
M1 green (the 8 hour soak): the registry, the picker gates and the
catalog selection are the data the M3 band acceptance scenario
consumes.

## Auto equipment (internal/swarm/gear, hunt/equip.go)

The melee fighter profile scores every equippable item (weapon = pAtk x
attack speed, armor = pDef, jewel = mDef, shield = expected block
value; bows score zero for melee), `NextUpgrade` plans the strictly
improving use item requests against the tracked paperdoll (empty slot
fills, strict slot swaps, the pair swap through freeing the weaker
jewel, the two hand weapon and one-piece family guards) and
`BurstUpgrade` turns them into the burst the loop sends per tick. The
acceleration limit of the sequential equip chain on this server build
is pinned by two source facts (both verified in the deployed Mobius
C1 module):

- the UseItem flood protector is disabled
  (`FloodProtectorUseItemInterval = 0`, the retail matching config)
  and the UseItem request never touches the one second
  PlayerActionFloodProtector of the attack and select packets, so no
  server side rate limit sits between the requests;
- the packet executor is one shared ThreadPoolExecutor
  (`commons/network/packet/PacketExecutor.java`): every received
  packet becomes its own pool task, so two packets of the SAME client
  may run concurrently and in either order - the server gives no same
  client ordering guarantee.

The second fact is the binding one: requests whose server write sets
overlap (the same item - a second request would toggle the first one's
effect back; the same paperdoll slot - the pair swap refill, the
one-piece drop follow up, the shield slot behind a two hand weapon)
can interleave on the server and cancel each other, while requests on
independent slots never touch the same state. `BurstUpgrade` plans on
a simulated copy of the equipment with the server write semantics
(`equipWriteSlots`: the landing slot plus the pieces the server
displaces - the shield behind a two hand weapon, the legs behind a
one-piece, the one-piece behind legs, the two hand weapon behind a
shield) and returns the maximum independent prefix of the chain -
the whole starting bag on an empty paperdoll dresses in ONE tick, the
dependent steps ride the next tick after their prerequisite confirmed.
The per item confirmation gate (`markInventoryAction`,
`inventoryItemAllowed`) is shared with the manual inventory commands
and paces only the requests that actually race; a character that
enters the world with the gear in the inventory wears the whole bag
within one tick, and the paperdoll stays optimal after every loot,
buy and death event. The acceptance suite pins the burst end to end
(the `full-dress` scenario dresses eleven injected pieces within ten
seconds of the world entry from the web UI).
`gear.TotalGearPoints` summarizes the equipped gear for the zone gates
(weapon damage per hit plus defenses).

A looted or bought item the bot is about to wear never becomes junk
(`gear.PlannedEquips`, the planned equip keep set): the simulation's
pending equips - the looted upgrades waiting for their paced use item
request, the buy arrivals, the better halves of pair swaps mid flight -
are excluded from both the shop sell list (`SellableItemsExcluding`)
and the overflow destroy list (`DestroyableItemsExcluding`). Only the
real junk sells: the duplicates beyond one copy per slot, the looted
downgrades and the displaced weaker halves of pair swaps. Without the
keep set the first sell batch could race the pair swap window and eat
the very jewel the bot was putting on, and the overflow destroy (which
ranks gear drops before stackables) could destroy a fresh upgrade for
bag space.

## Shop strategy and the town trips

The strategy reasoning, prices and the level journey comparison live
in `docs/shopping_strategy.md` - read it before touching the planner.
The short form:

- The phased planner buys in the strategy order: **the weapon
  milestone first** (the top affordable strict upgrade - the top-tier
  guard keeps the cheaper rungs of the hand ladder out whatever their
  value per adena), **the pdef maximizing armor set second** (one
  piece per armor family per trip, chosen by the exhaustive
  enumeration of the per family efficient frontiers inside the
  remaining budget - a rich wallet buys the advanced pieces directly,
  a poor one fills many slots with the cheap offers, the set
  maximizes the summed pDef either way), **the basic jewel floor
  third** (the cheapest offer of every empty slot, both halves of the
  pairs - the jewels never upgrade: the starting locations barely
  attack with magic) and **the shield with the leftover**. It never
  buys what the inventory already carries and it respects the adena
  budget plus the sell credits of the displaced pieces; **one item
  per paperdoll slot per trip** - the next trip re-plans from the
  reached paperdoll.
- The town trips (internal/swarm/hunt/town.go, the trip lifecycle;
  the walk, the sell and the recovery files of the same package hold
  the rest) trigger when the
  inventory passes 50% of the slots or 50% of the maximum weight. A
  trip start never interrupts a fight: `fightBusy` (a living target, a
  pending loot pickup or an incoming hit) holds it until the
  between-fights window, and a resting character is stood up first
  (the server refuses move requests while sitting; the stand toggle
  shares the pending transition gate with the rest logic).
- **The weapon leads every trip that buys one** (the 2026-09-11
  bare-handed report): a plan with an affordable weapon purchase
  routes its sell stop to the weapon's merchant (the junk sells at
  any merchant), so the sell-first of the replaced weapon and the buy
  share ONE stop and the replacement lands seconds after the sale -
  the old order sold the weapon at the nearest merchant and walked
  the village for the replacement, and every trip killer in between
  (a stuck teacher segment, an attacker interrupt, a merchant no-show)
  left the character bare-handed. A character with NO weapon at all
  runs the weapon errand (`weaponlessRunWanted`: no profile
  usable weapon in the inventory and an affordable weapon in the plan
  - see `gear.HasWeapon`): the run carries the learning stops too
  (the 2026-09-12 one town visit rule - the weapon stop runs FIRST,
  so a stuck teacher segment can no longer strand a bare-handed
  character: the weapon is bought and worn before the teacher segment
  ever runs), the retry cooldown shortens to 45
  seconds (`weaponRunCooldown`) instead of the five minute trip
  cooldown. While the weapon run is pending the engage holds its
  fresh target picks (`logWeaponWait` paces the hold line) and the
  zone entry engage of the return segment skips the same way - the fists
  land 2 damage and nothing outranks fixing that; the attacker self
  defense answer stays armed whatever the weapon state is. A wallet
  that cannot afford any weapon keeps farming: the gate only holds
  when the plan offers a weapon, so a fresh bot still punches
  keltirs until the wallet crosses the cheapest offer.
- **The merchants specialize and the bot carries the same knowledge**
  (the 2026-09-20 seller distinction round): a weapon buys only at
  the weapon trader, an armor piece (with the shields) only at the
  armor trader, the jewels and the spellbooks only at the jeweler -
  the server resolves every RequestBuyItem through the targeted folk
  npc and silently refuses the list the targeted npc does not trade,
  so a buy aimed at the wrong vendor burns the whole retry budget
  for nothing. The wares of every town merchant derive from the
  generated buylists at runtime (`hunt/merchant.go`:
  `merchantWares` classifies the goods families, `merchantSellsItem`
  is the exact item check): the elven village splits into Unoren
  (weapons), Ariel (armor), Creamees (jewels + spellbooks + the
  mystic amulets), Herbiel (consumables) and the Dion quarter into
  Sabrin, Casey, Sonia, Lara the same way. The enforcement points:
  the frozen trip plan drops the lines whose merchant's lists do not
  carry the item (`dropForeignMerchantPurchases` at the freeze, a
  data bug surfaces as a loud log line instead of refused requests),
  and `handleMerchant` re-picks the trader when the selected npc's
  template answers none of the stop's wanted templates. The sells
  stay universal: any merchant of the region accepts the sale of any
  sellable item (the standard inventory sell list), the sell stop
  picks the nearest vendor - and the stale selection of the sell
  phase used to aim the merged weapon buys at the armor trader
  standing next door (the re-pick closes that hole). The merchant
  sets follow the region (`merchantsForRegion`): the elven default
  and the Dion set for the 20-25 band, the same set the region shop
  catalog builds from.
- **The Newbie Guide buff run** (the 2026-09-21 guide priority
  round): the support magic of the village guide (template 7599, the
  8-24 level band, the two dialog links - see `hunt/guide_buffs.go`)
  is a trip reason of its own (`guideRunWanted`: the region carries a
  guide AND `guideWanted` - the level band, no refusal cooldown, an
  eligible buff missing). The run joins the weapon run as the second
  trip start that may fire OUTSIDE the hunting zone: the village
  revive of a death wipes every buff and lands next to the guide, so
  the recovered character takes the support magic first and the trip
  return segment walks the preserved farm spot second (`resetTownTrip`
  keeps `farmX/Y/Z`; the guide run skips the out-of-zone
  `rememberFarmSpot` zone-center overwrite). In zone, the expired
  support magic starts the refill trip at the next between-fights
  window - the bot never keeps farming unbuffed while the guide is
  reachable. The trip machinery is shared: the ordinary sell stop
  leads (the junk sells), the guide stop plans behind the learning
  stops (`planGuideStop`), the refusal cooldown of a refused gate
  keeps the later trips away for 10 minutes. The support magic is
  keyed by the character level: the buff windows widen and narrow
  across the 8-24 band and the 16..19 band also carries the Life
  Cubic (skill 67), so the deleveling never aims below 16 once the
  character reached it (see the deleveling section). The keep-one
  scroll line of the shopping plan stays out of the trip justification
  total (`shoppingWanted` skips the scroll): a scroll-only plan
  never clears the trip minimum, the stock rides the natural trip
  cadence and restores itself on the same trip that spends it.
  Regions without a mapped guide (Dion today) neither start the run
  nor plan the stop (`guideForRegion`) - a wrong guide would walk
  the Dion trips to the elven village across the whole map.
- **One town visit buys everything** (the 2026-09-12 acceptance
  round): the gear stops distribute first (`planShoppingStops`), the
  learn stops close the trip behind them (`planLearnStops` at the
  sell stop) - the book stop merges into the gear stop of its
  merchant when they match (the jewel trader Creamees sells both the
  basic jewels and the spellbooks, one visit buys them all) and the
  teacher walk follows, so a single visit buys the weapon, the armor,
  the jewels, the books and teaches the lessons. The gear plan
  reserves the spellbook budget out of its planning wallet
  (`Loop.pendingBookBudget`: the books of the learnable lessons the
  inventory does not carry yet, at the town tax price), so the
  aggressive armor spending can never starve the books.
- The trip **sells all the accumulated junk first** (the selling ends
  when nothing sellable is left, not at the 50 percent trigger),
  **sells the replaced gear before the buys** (a planned purchase that
  displaces an equipped piece carries its `SellFirst` object ids and
  its sell credit of referencePrice/2 - the planner counts the credit
  toward the budget, so the character shops for a replacement as soon
  as the adena plus the proceeds cover it instead of hoarding the full
  price; the trip unequips the displaced pieces - they become plain
  sellable candidates the junk flow or the replacement batch sells -
  and waits out their removal before re-planning, so the fresh adena
  of the sale funds the buy; the auto equipment stays suspended over
  that window so nothing re-equips a piece bound for the merchant),
  re-plans with the fresh adena and walks to every merchant of the plan
  (one buylist per transaction request, 1.25 s pacing - the server
  transaction window is 10 game ticks = 1 s wide and a too early
  request costs nothing, so the spellbook list of a multi list stop
  follows its jewel list right behind; see the transactionPause
  comment in hunt/shopping.go). The auto
  equipment runs during the trips so the purchases are worn at the shop
  already. The catalogs are generated from the Mobius buylists
  (`tools/generate_shop_catalogs.sh`, keyed by packet template id).
- **The gear debt** (the 2026-09-12 04:58 pantsless dump): the
  sell-first step is one-way - a sale that lands while its
  replacement buy fails (a silently refused request, a merchant
  no-show, a walk abort, a session death the relogin resumed into
  the return segment) strands the slot, and every trip exit used to end
  the trip as a success. Now the trip start snapshots the worn
  slots (`Loop.snapshotTripGear`) and every exit (`endTownTrip`,
  the interrupt `resetTownTrip`) compares the reached paperdoll
  against it (`Loop.gearDebtCheck`): a slot that was occupied, sits
  empty and whose piece is gone becomes gear debt - the map entry
  carries the lost item id, the arming logs the wound ("the trip
  left the legs slot empty - the Leather Pants it started with is
  gone") and the debt shortens the trip cooldown to the gear run
  window (`gearDebtRunWanted`, the 45 s window of the weapon run)
  until the refill dresses the slot (a log line clears the entry).
  The debt never justifies a trip on its own - the ordinary
  triggers fire the refill the moment the plan affords it, on the
  short cadence. The state dump names the holes the equipment
  section used to hide: the "empty slots:" line below the worn
  pieces (the report's bot farmed the Kaboo woods without its legs
  armor and the dump printed nothing about it). Pinned by
  hunt/round60_repro_test.go and the live "gear-gap" acceptance
  scenario (acceptance/gearGapReset: the exact dump character must
  buy its legs armor back).
- The path plan comes from the pathfind engine through the
  hunt.Navigator interface (set in main.go with hunt.NewNavmeshNavigator
  when the navmesh tile directory holds tiles - the long routes serve
  from the mesh corridor search, the grid engine stays the click
  validation and the fallback authority - and with hunt.NewNavigator
  from the auto detected geodata directory otherwise; without geodata
  the bot hunts without trips). A waypoint follower walks the smoothed
  path with
  ground click walks (one per 2 s; an intermediate waypoint counts as
  reached within 50 units - a bridge ramp entry or a detour turn must
  be walked through, not seen from the side - and the final waypoint
  within the 150 unit trip arrival radius; a waypoint the character
  already passed ON the route skips ahead through the projection pass
  test, while a character beside the route keeps targeting the
  waypoint it missed). The click target spans the route, not one
  point: the follower aims the farthest waypoint the server click
  transport delivers inside the aim window (the run speed times two
  click periods, floored at 300 units for the pre UserInfo clicks and
  capped at the 1000 unit move limit), so the next request re-aims
  before the character reaches the target - the server replaces the
  destination of a running move without stopping the creature, only a
  reached destination stops it, and the old one-point aim turned
  every dense plan point into an arrival stop. A corner pre-click
  re-aims within 80 units of the current town waypoint while the
  character still walks, paced at half a request period (the 50 unit
  intermediate arrival radius leaves that ring); the manual walk of
  the web UI and `-move` re-aims the same way every request period
  instead of standing down while the move runs, and a planned manual
  route walks on a route sized budget (the 90 s base plus twice the
  estimated travel - the plan length over the measured run speed -
  capped at 30 min), so a cross-map order is not cut off halfway; the
  direct server routed walk keeps the base window. The far click must
  stay on the drawn route: every skipped waypoint projects inside the
  click chord within the 100 unit corridor and within 100 units of
  the chord height, so a switchback, a priced water dip or a ramp the
  plan walks around is never cut. The scan runs only at the click and
  pre-click moments, and the pre-click scan is paced like its click
  (at most one per half request period), never per tick; the
  dead-cell skip verdict short-circuits it, so a dead transport still
  reaches the stuck recovery. Every skip is gated on the walkable
  line: the straight line from the actual standing cell to the
  successor waypoint must pass the server click transport - the same
  oracle the click itself faces, `Navigator.ValidateClick` - else the
  waypoint stays the target until walking onto it re-opens the line -
  the server (the deployment runs PathFinding = 0) validates every
  click as a straight line and cancels a move whose first step hits a
  closed wall at the character's own position, and the 2026-09-11
  teacher walk stuck exactly that way (the follower skipped the 16
  unit ramp steps of the trainer plaza approach inside the 50 unit
  pass radius and clicked the plaza waypoint through the railing;
  every re-path reproduced the identical route, the budget burned
  and the trip aborted before the teacher - the lessons never
  landed). Every click is gated through the server click validation
  port (`Navigator.ValidateClick`, see round 52): the server refuses
  whole lines its Bresenham raster walks into walled corners - a
  refused click collapses onto the walker and never moves the
  character, so the follower reacts instead of sending it (the segment
  shortening first, then the hop back to the nearest swallowed plan
  bend - the escape out of the geodata trap cells - and finally the
  re-path). The walker re-paths around obstacles after
  15 s of standing still (the later stucks fire on the 4 s fast
  window; 3 re-paths abort the trip): the stuck skip only jumps onto a
  waypoint with a walkable line from the standing cell (the same gate
  as the cursor advance - the 2026-09-11 06:19 trainer hall aisle
  dump: the blind skip armed the east hall waypoint whose click the
  server collapsed onto the first step, and the partial clicks crept
  the character 16 units at a time into the dead-end pocket cell east
  of the aisle whose closed east wall then refused the click
  wholesale), and the re-planned segment advances its cursor past the
  fresh plan's wp 0 (the standing cell itself) before the click fires
  - a click at the character's own position is a guaranteed server
  refusal that would burn the re-path budget on nothing. A re-path
  that starts from the same cell the previous one planned from,
  without a single cell of movement in between, aborts the trip at
  once instead of re-planning the identical route: the deterministic
  planner reproduces the same first click the server already refused
  twice (the 2026-09-11 11:34 village return dump froze through two
  whole trip cycles this way), and the zone return escalates straight
  to the direct server routed segments (a frozen return sets the zone
  fail budget). The town walk segments climb their own escalation ladder
  instead of dying on the first freeze (the 2026-09-12 03:56 trainer
  hall dump: the learn segment froze at the aisle entrance through every
  re-path of two whole trips - the server walled the corridor the
  pack modeled as open): the frozen abort first bans the aimed
  waypoint's cells as the session's avoid areas and re-plans the
  detour around them (the pathfind search takes the banned patches -
  a step onto banned ground costs impassable, and neither the direct
  line shortcut nor the smoothing may collapse a segment across them, so
  the trainer hall route goes north over the terrace and east past
  the hall), then - if the detour freezes as well - the follower
  drops the plan and clicks the stop target directly by the server's
  own routing (the npc approach point of the stop, the aggro
  steering stays on), bounded by a 45 s window; only a
  direct walk that also makes no progress ends the trip with its
  cooldown. The ban list survives for the session (bounded to 8
  areas) so the later trips route around the frozen corridor too.
  The `waypointBehindRoute` gate only counts a waypoint the character
  stands near: the V-shaped detour routes (the hall recovery climbs
  far north before doubling back south east) carry waypoints hundreds
  of units ahead whose position projects beyond the doubling segment,
  and the projection test alone re-aimed their climb clicks at far
  route samples whose straight lines cross the terrace walls.
  After the first stuck with no clear successor the short click
  extension arms: the clicks whose target sits under the server
  rescue floor (50 units - the server's own move validation only
  hands a collapsed click to its pathfinder when the original line
  was longer than 30 units, so a shorter collapse is silently
  canceled with ActionFailed and never moves the character) or
  behind the character on the route re-aim at the forward route
  samples past the floor (the march along the plan polyline skips
  the under-floor and backward samples, the
  click validation port gates every sample, and a walled sample only
  skips forward to the next route cell). A trip timeout (20 min) and a trigger cooldown
  (5 min after every trip end) bound the whole feature, and a death -
  mid trip or not - clears the cooldown: the village restart lands next
  to the shops and a full inventory sells right after the revival
  instead of walking to the farm spot with the junk first.
- The walk parameter table (`hunt/walk_params.go`) pins the remaining
  numbers: the refusal answer window 4 s, the move start watchdog 3 s
  (the once per second movement broadcast plus the lag, under the 4 s
  fast stuck window), the net progress margin 50 units, the varied aim
  ladder 4 attempts with the perpendicular offset 192 units, the cursor
  key escape at 144 units per 1 s claim with 5 follow claims of
  patience, a 64 unit drift margin, a 2500 unit route cap, 3 attempts
  per trip and a 2 s settle, the short click extension at 16 unit march
  steps capped at 5 samples, the click shortening floor 100 units and
  the hop coincidence 15 units.
- The ActionFailed refusal channel (the 2026-09-14 10:18 dump): the
  server answers every refused MoveToLocation with the one byte
  ActionFailed (0x35), and the tracker records its arrival
  (state.ApplyActionFailed) while the dump prints its age next to the
  session header - a server that refuses the walk requests names
  itself in the report. The walk machinery reads the timestamp as the
  online refusal evidence (refusalEvidence, valid inside a 4 s window
  around the sent click - walkZoneLeg records its clicks in the
  shared moveAt slot too): a stuck verdict with the evidence first
  varies the aim at the current waypoint (sendVariedAim - the half
  and the quarter of the capped click, then the perpendicular
  offsets, every variant offline validated) because
  the refusal is target specific; the corridor ban rung of the
  escalation ladder is SKIPPED for a segment whose evidence latched
  (segmentRefused) - a segment the server refused does not name a frozen
  corridor, banning it would seal innocent ground for the session
  (the dump poisoned six corridors and a widened r768 ban across both
  village exits of a server whose build simply refuses those clicks);
  the server routed walk hops under the 3000 straight line boundary
  (directHopMax 2500: Mobius refuses a request beyond 9900 units
  outright and walks a player click beyond 3000 as a plain straight
  line with no geodata routing - the old single far click of the
  fallback was structurally dead), every hop passes the offline click
  port, a wet hop line shortens to its dry shore prefix and a refused
  routed hop aborts the trip early with the honest reason; and the
  direct zone segment stall splits on the ground covered - a character
  that progressed re-arms the pathfound return (the partially
  refusing deployment walks out through the varied aims), a
  character that stood on the same cell as the previous refusal stall
  holds the return backoff (the fully refusing deployment never
  restarts the cycle the server keeps refusing).
- Water safety of the trips (the 2026-09-10 stuck regression, the
  priced form of the mesh round): the
  server moves characters into water without any hesitation - its own
  move routing carries no water cost, its getValidLocation accepts the
  gradual underwater beds, and every move request of a character it
  considers swimming (inside a water zone: the whole elven region below
  z -3780) skips the geodata validation entirely and just walks the
  character straight at the click, clamped to 700 units. A town trip
  once swam below the elven village plateau this way and stood
  paralyzed under its cliff: the swim z floated above the water zone
  bound, so the zone flipped in and out while every click toward the
  village deck resolved onto a layer the lake bed has no walkable
  connection to (the server answers such moves with the character's own
  position - a 0 length walk) and the re-paths replanned from the same
  floating spot. The grid era answered with four defenses (the dry
  search, the dry smoothing, the wet click refusals and the shore
  escape); the mesh round retired all four - the mesh plans out of
  every standing cell, wet included, and prices the water at the
  measured swim rate (the run/swim speed ratio 2.3, the C1 zone data
  arms the pricing): the plan may swim, the slowdown priced, and the
  land detour wins whenever it is the faster walk (the navmesh world
  water reality tests pin the planning on the real elven pack: every
  wet cell of the 2026-09-20 dump answers a full trader route whose
  tail stands dry). The follower walks the wet segments it planned -
  a swim click is a priced walk, not a refusal. An off-plan drift
  over a lake bed (a server push, a click drift) recovers through the
  plain stuck ladder: the walk re-plans from the wet standing cell,
  the fresh plan owns the segment - the dedicated shore escape
  machinery retired with the grid (the 2026-09-20 dump round proved
  it harmful: the escape clicked at 40000 43776 -3776 four times, a
  target the grid raster itself calls over water, and burned the trip
  into "aborted, the water escape could not leave the water"). The
  standing water check stays for the frame measurement (a swimming
  character measures no vintage shift, see click_frame.go) and the
  capsule mirrors the server water clamp (the 700 unit swim moves).
  An abort of a walk machinery that runs during
  the deleveling aborts the deleveling itself (abortTownTrip
  delegates to abortDelevel): the plain trip end left the delevel
  state armed without a cooldown and the next tick restarted the walk
  into the same blocker. The
  dump and the map carry the whole segment - origin, every waypoint
  with the passed markers, the TARGET marker on the current waypoint
  and the destination - for exactly this class of debugging (see
  docs/webui.md).
- Path layer selection: the trip segments navigate through the
  Navigator's mesh corridor search (FindPathApproach) and the segment
  approach radius. The avoiding form (FindPathApproachAvoiding) stays
  on the seam for the library contract; the session's frozen corridor
  bans are gone - the plaza ping-pong round removed them because the
  rectangle granularity sealed whole mesh sheets. Every npc
  destination walk (the merchant stops, the teacher stops, the delevel
  guard walks) plans
  through the npc stop ladder (`startWalkNpcSegment`): the npc rung
  searches with the npc search radius (`npcApproachRadius`, 10 units -
  the user rule of the 2026-09-20 report: the plan must end at the
  npc's own point, not at the first walkable surface a wide ball
  catches - the wide ring plans held the shop edge 96-232 units short
  of the trader while the requested point sat inside the shop) and
  refuses a found plan that resolved onto a foreign deck (a connected
  roof over the shop answers the destination cell's closest-layer
  resolution hundreds of units above the npc floor - the 2026-09-11
  roof teleport geometry the exact planner refuses the same way); the
  wide rung (`tripApproachRadius`, 200 units, under the interaction
  distance) serves the conservative deck stop for the destinations
  whose own point the mesh or the geodata cannot deliver, and the
  offset click window of the talk machinery owns the last stretch
  there. The merchant cells without a modeled floor layer or behind a
  counter stay reached through the partial answer (the search walks
  the corridor to the closest reachable point - the customer cell
  across the counter), while the water deck below the shop never
  satisfies any radius (the z difference counts in the 3D distance).
  The teacher stops walk right up to the training npc on the same npc
  rung - the mesh search is the one that knows the walkable hall rows
  (the trainer hall interior carries its floor along the rows, the
  straight line offset ring lands on the roof-only bands between
  them), and the tight segments complete their route end with the pass
  radius instead of the wide trip slack. The returns (the farm spot,
  the zone) search the wide trip ring directly. The water prices at
  the swim rate in the search, so the walks cross the village ramps
  whenever the swim is the slower walk (the priced round; regression
  tests `TestFindPathToShopDeck`, the synthetic water tests of
  `search_test.go` and the priced swim tests of
  `priced_search_test.go`).
  approachMerchant also
  gives up targeting when the merchant stands more than the interaction
  distance above or below the character. The teacher approach
  (approachTeacher) walks the npc approach point ring before the talk
  click fires (the 2026-09-12 user rule: the character walks from the
  building entrance right up to the training npc), and the approach
  window bounds the walk: a ring the server routing refuses to close
  still talks from wherever the character stands when the 3D distance
  fits the interaction gate (the 2026-09-11 05:45 z gap rule), a
  teacher on a deck the ring cannot reach skips the stop.
- **The exact merchant segment** (`startWalkExactSegment`): the
  merchant stops plan through the mesh corridor search with the
  approach radius zero - the plan must arrive at the destination cell,
  whatever walkable cell the mesh resolves it onto. The approach ring
  catches the first deck polygon inside its radius and ends the plan
  short: the shop quarter report held the wide ring plans 147-232
  units from the merchant, on the outer railing side of the stall, and
  the talk fired from there (or slid along the railing forever). The
  exact search answers the customer cell across the counter instead:
  the merchant's own cell sits on ground the mesh never walks onto (the
  counter row), so the corridor ends at the closest walkable floor cell
  to the npc. The ring fallback owns exactly one failure class - the
  plan resolved onto a foreign deck (a connected roof over the shop
  answers the closest layer of the destination cell): the conservative
  ring stop on the surrounding deck is the safe answer there. A hard
  navigator error or a corridor-less mesh answer fails the ring search
  the same way (the reachable component is the same for both goals), so
  the callers skip the fallback and the trip attempt keeps its one
  search cost. After a ring walk completes, `exactApproachWanted` runs
  the exact final approach when the merchant sits beyond the customer
  ring but inside the exact distance gate - near enough for the bounded
  search, far enough that the talk would fire from the ring edge
  instead of the counter.
- **The npc arrival handoff** (`stopNpcArrivalMet`): a running segment
  hands the merchant stop to the talk machinery once the character
  stands within the wide arrive radius of the segment destination (the
  customer stand point) AND within the interaction distance of the
  merchant npc. The tight verification of the plan's final cell can
  grind forever against the vintage geodata disagreement - the live
  server resolves the clicked destination onto its own surface (the
  pack holds cells 64 units off the live floor, the creation building
  interior among them) - and the stuck ladder would burn the whole
  escalation down to the trip abort while the character stands ready to
  trade. The stop exists to bring the talk within reach, not to verify
  the pack's cell; the teach stops keep their own walk contract (the
  hall rows need the real arrival) and the non merchant segments answer
  false.
- The npc talk selection clears when the conversation ends: the
  merchant select and the teacher talk click leave the villager
  selected server side (the server never clears a selection, only the
  next selection replaces it), and the hunting engage that follows the
  trip would re-adopt it - the forced attack requests on a friendly
  npc only burn the 12 s engage stuck timeout. The trip machinery
  sends the self click (Action 0x04 on the own object id, the official
  client way of dropping a selection, `GameClient.ClearTarget`) when a
  stop finishes (`advanceTripStop`) and when the whole trip ends
  (`endTownTrip`), and the engage adoption itself requires an
  attackable npc (`ObjectAttackable`) so a lingering selection
  (a talk outside the trip end, the self selection of the clear click)
  never becomes a hunt target.
- Merchants: townMerchants carries the shop npcs of the known towns
  with their spawn coordinates; the C1 spawn ids map to the client
  display ids the NpcInfo packets carry (30147..30150 -> 7147..7150
  through CT0_to_C4_ids.txt). The sell uses RequestSellItem 0x1E with
  the standard inventory sell list (list id 0, the CUSTOM_CB_SELL_LIST
  of the official client): the server prices every item itself at
  referencePrice/2 (no prices in the packet), answers with
  InventoryUpdate removals, ItemList, a CUR_LOAD StatusUpdate and the
  "The transaction is complete." SystemMessage, and accepts the
  transaction even without a targeted merchant - the bot still walks to
  the merchant and selects it (interaction distance 250, one selecting
  request per second) like the official client, and falls back to
  selling without one after a 45 s wait. The transaction flood
  protector paces the batches (one per 1.25 s, up to 25 items, the
  same shared window the buys pace through - see
  Loop.transactionWindowFree); every
  item is offered once per trip, so items the server refuses to sell
  can not stall the phase. The junk ranking lives in
  state.Bot.SellableItems: duplicated gear pieces first (every piece of
  an item id but one), then the lowest sell value per unit weight (the
  generated npcdata.ItemPrice/ItemWeight dictionaries, see
  tools/generate_item_stats.sh); equipped gear, adena and quest items
  never sell, and neither do the spellbooks of the unlocked queued
  lessons (demandedBooksLocked walks the stored learning queue): a
  book bought at the book stop or looted for a near term lesson feeds
  it, selling it for referencePrice/2 only buys it back full priced
  at the next learning trip - the 2026-09-10 report loop. The
  overflow destroy (DestroyableItemsExcluding) keeps them the same
  way. The first town trip of a session also waits for the server
  skill list (state.SkillsListed, bounded by 10 s from the session
  start, re-armed on every reconnect): the enter world packet burst
  (UserInfo, ItemList, SkillList) races the first hunt ticks, and a
  trip that starts between the ItemList and the SkillList plans
  without the learning stops.
- **The merchant approach gates**: the walk stops on the 200 unit
  approach ring around the npc approach point, never on the merchant
  cell - a click on an interior cell steps the server Bresenham line
  onto the roof layer (the 2026-09-11 roof teleport). The select
  fires from the ring inside the 250 unit interaction distance, which
  the server measures in 3D: x, y and z count together, so a small z
  gap can keep the distance above the 200 approach gate while the
  ring already stands within 250 (the 05:45 dump looped on the ring
  without that early return). A merchant on a foreign deck hands the
  last stretch to the select + attack analog pull ladder for the
  bounded deck window; when that window burns out the stop gives up
  (the sells work without the merchant, its buys are skipped).
- The live verified cycle (2026-09-07): trigger at 55 slots/73% weight,
  walk to the trader ~18k units in ~60 s, one batch of 25 items sold
  (55 -> 30 slots, 73% -> 36% weight), walk back and hunting resumed;
  covered offline by internal/swarm/hunt/town_test.go (trigger,
  waypoints, merchant selection, batches without a merchant, no
  destroy during the trip, stuck re-paths, death reset) and by the
  stuck report reproduction of `town_repro_test.go` (the round 35
  scenario with the exact reported positions 45544 45880 -2992 ->
  Unoren 44667 46896 -2982: a straight MoveToLocation sent at the
  merchant stalls at the reported spot under the server simulating
  walker, the planned trip walks it with zero stuck re-paths; the same
  scenario runs live through tools/repro_stuck_trip.sh, which places
  the character at the stuck spot through the database and captures
  the mid-walk state dump).
- The counter stand round (2026-09-20): the merchant stops target the
  curated customer stand table (`merchantStands` in
  town_merchants.go) instead
  of the merchant spawn. The spawn cell of a stall merchant sits
  inside the roofed stall the geodata pack models as roof-only cells
  (the cell holds no floor layer - nobody can stand on it directly,
  the owner report), and the raw spawn route answered the OUTER side
  of the counter for Unoren (42 units north-west of the spawn). The
  stands sit on the customer side of each counter front: the counter
  line is read off the geodata as the closed NSWE cell faces along
  the merchant's facing heading (cmd/counterprobe -mode detect names
  the direction, -mode scan pins the cell, -mode verify walks the
  mesh route and checks every leg with the grid line of sight). The
  table covers the elven pair (Unoren, Ariel on the west corridor of
  their stall), the elven magic shop (Creamees, Herbiel south-east of
  their counter bands) and the Dion pair (Sabrin, Casey on the east
  corridor; Sonia, Lara on the north corridor). The interaction
  gates keep measuring the spawn; the stands sit 60-100 units from
  it, well inside the 250 interaction distance. Pinned by
  TestMerchantStandTableCoversTheCounterTraders and
  TestUnorenStopTargetsTheCounterStand.
- The attack analog pull round (2026-09-20): every npc talk fires the
  attack analog of the official client - the second plain click on the
  already selected npc (InteractPull, the Action 0x04 packet the
  plain talk rides). The Mobius NpcClick handler resolves it to the
  interact branch: a folk npc outside the 250 interaction distance
  takes the INTERACT intention and the server walks the character to
  the npc along the straight line (36 units, thinkInteract) and opens
  the dialog on arrival, so the interaction distance is met with
  certainty however the geometry between them sits. It is not the
  real attack (the AttackRequest 0x0A the guard engage drives - no
  forced attack intention, no swings, nothing takes damage). The
  merchant select fires the pull once per merchant after the
  selection confirms; the deck wait (the z gap the ground clicks
  cannot close) replaces its ground click re-walk with the select +
  pull ladder; the teacher talk clicks turn select-or-pull; the
  gatekeeper transfer sends the paced pull after the entry click;
  the quest talk already was the two click practice. Pinned by
  TestMerchantSelectionFiresTheAttackAnalogPull and
  TestMerchantDeckCaseHandsTheWalkToTheServer.
- The world stand round (2026-09-20): the same machinery covers every
  merchant spawn of the world. The generated spawn table
  (`npcdata/merchant_spawns.go`, tools/generate_merchant_spawns.sh)
  joins the Mobius spawn xml files with the CT0_to_C4 display ids and
  the buylist owners: 93 merchant spawns across the towns of the
  continent. The counterprobe world pass
  (`cmd/counterprobe -mode world` derives and verifies,
  `-mode world-emit` prints the table) probes every spawn on the grid
  engine: a spawn the pack serves (floor at the spawn height plus a
  verified route onto it) needs no row - the spawn fallback of
  merchantStandPoint answers it; every other spawn (the roofed shop
  interiors, the counter fronts) derives its customer cell: the
  heading push 64 units along the facing direction, then the ring
  scan over the 48..240 radius bands (16 units per band) that tries
  the 60 degree facing sector first and then every direction,
  keeping the candidate with the most verified ways. The derivation
  keeps a stand within 250 units of the spawn and within 96 units of
  its z (a roof deck or the flat zero layer is a foreign floor the
  interaction distance can never serve), and the route verification
  starts from a probed neighborhood cell 200 or 400 units out whose
  height sits within 96 units of the spawn; two independent starting
  directions is the cap - one proves the walk, the second only names
  the redundant side. The pass pins a cell only when the grid route
  ends exactly on it with the line of sight of every leg.
  The pass verified 58 spawn served (the five Talking Island traders
  sit in the flat placeholder regions of the pack - no geometry to
  derive from, the spawn fallback serves), 27 derived stands
  (merchantStandsWorld in merchant_stands_world.go) and re-verified
  the 8 curated rows, 0 failed. The approach point resolution reads
  the curated table, then the world table, then the spawn. Pinned by
  TestWorldStandTableJoinsTheSpawnData and
  TestMerchantStandPointResolvesEveryWorldMerchant.

### The cursor key escape (hunt/town_cursor_escape.go)

A live refusal family leaves the routed walk with no click answer at
all: the server answers no mouse click from such a cell, the official
client stood frozen there too, and only the ARROW KEYS walked the
character out (`ValidatePosition.runImpl` syncs the claimed placement
into the world and broadcasts it). The escape emulates exactly that:

- The arm sends the keyboard-mode move request (`MoveToLocation`
  movement mode 0), which latches the server cursor key flag; the aim
  is the ladder's far end (a target on the own cell would answer the
  `stopMove` refusal first).
- The claims pace at the official client cadence, one run-speed step
  per second, and the claimed z rides the segment frame offset into
  the server frame (a raw pack z would place the character below its
  own ground and the server correction snaps it back).
- The steps follow the planned route when the segment holds one, so
  the ladder bends where the plan bends, and fall back to the straight
  line toward the aim clamped into the pocket radius without a plan.
  The route length caps, so one escape stays a pocket recovery.
- The march starts at the first waypoint the character's ground still
  owes, never at the plan cursor: the click ladder bets far waypoints
  (the forward jump, the stuck skip) and a refused bet leaves the
  cursor ahead of the walked ground.
- The follow probe watches the position: a drift past the margin
  re-arms the patience, while an ignoring server burns it and the
  escape ends for the caller's abort ladder. The settle window lets
  the broadcasts land, and only the position the character actually
  reached advances the plan cursor; the ladder aim is never trusted.
  The stuck window re-baselines from the settle ground.
- The claimed heading follows the Mobius convention
  (`LocationUtil.calculateHeadingFrom`, east 0, south 16384), the same
  one `state.HeadingFromDelta` applies; the cursor key movement probes
  its obstacle front along it, so a mirrored heading probes behind the
  character.

## The server frame click transport (hunt/click_frame.go)

The Mobius C1 server resolves the destination layer of a mouse click by
the NEAREST height to the z the request carries: `isCompletelyBlocked(target,
targetZ)` in the packet handler and the final layer rule of `getValidLocation`
(`previousZ != nearestToZ` collapses the destination back onto the walker,
the distance drops below the cancellations limit and the server answers
`ActionFailed`). The plan waypoints carry the layer heights of the bot's own
geodata pack (`node.layer.Height`), and where the two packs disagree about
the absolute height of a surface - the village plaza deck stacked over the
water floor 872 units apart, the teacher hall interior whose flip margins
shrink to 164 units - the mesh z names the wrong layer, the walk arrives on
the standing surface, the final rule collapses the click and the character
never moves.

The transport is the bot side answer with the server kept as the source of
truth: the one z the server itself vouches for is the character's standing z
(every position broadcast carries it), and the mesh knows the same standing
cell in its own frame (the plan's first waypoint IS the character's cell
resolved on the pack). The difference of the two is the measured frame offset
of the surface the character stands on, and adding it to every plan waypoint
z re-anchors the plan into the server frame while keeping the mesh's own
relative geometry - the slopes, the stairs, the 864 unit rises the quest
walks proved the raw self z can never ride (the Gludio segment run: every
click on the self z answered `ActionFailed`, the waypoint height walked).
The offset is measured once per plan from the ground truth pair the character
stands on and capped at `frameOffsetLimit` 500: a real pack vintage
disagreement shifts a surface by tens to a few hundred units (the measured
flip margins sit at 164 for the teacher hall interior and 436 for the plaza
deck over the water floor), while a larger measurement is a layer snap of
the sandwich (the 872 unit deck over water gap) and anchoring by it would
corrupt every click of the plan. Such a measurement is discarded (the offset
stays zero, the raw mesh z rides) and the refusal ladder of the walker owns
the plans whose start the pack cannot resolve onto the standing layer.

## The city shopping errand (hunt/city_shopping.go)

A live bot that already owns a weapon and whose wallet covers the
missing top no-grade kit runs the city errand on its own; when the
wallet also covers the top no-grade bow, the same trip carries it
behind the kit. The errand blocks the tick between the deleveling and
the town trips: it owns every move and transaction from the village
walk to the return home, then the ordinary hunt resumes from the spot
the errand ended on. The trigger (`cityShoppingWanted`) fires when a
navigator exists, no town trip runs, the 10 minute retry cooldown
expired, the character hunts the elven lands (`zoneRegion` empty or
`elven`), its gear profile is the melee fighter, it owns any weapon
(worn or bagged, `cityShoppingArmed`) and the plan cost is positive
and covered (`cityShoppingPlan`): the missing kit pieces, plus the bow
when the wallet covers the whole plan beside a tool weaker than the
Composition Bow (`cityShoppingBowUpgrade` reads `gear.OwnedBow`). A
kit-complete character with the bow money runs the errand for the bow
alone.

- The route: the walk to Mirabel (7146, 46926 51511 -2976) of the
  Elven Village, the gatekeeper list "The Town of Gludio" (arrive
  -12694 122776 -3114), Sydnia (7321, list 3032100) for the Falchion
  68, Simplon (7253, list 3025300) for the Bronze Breastplate 26,
  Bronze Gaiters 34, Hard Leather Helmet 1148, Bracer 51, Boots 39
  and Bone Shield 625, Sydnia again for the Composition Bow 273 (the
  same weapon list, the last stop so the rest of the kit is in the
  bag first), then Bella (7256, -12736 122816 -3114) back to the
  Elven Village and the walk to the start spot. The teleports are free
  below `MaxFreeTeleportLevel` 40 and cost 9 200 adena above.
- The Gludio 20 percent buy tax prices the kit at 455 760 adena
  (292 800 for the weapon and 162 960 for the armor) and the bow at
  another 292 800 (the whole plan 748 560): the wallet gate prices
  every missing piece, so a bot that already carries some of the
  pieces pays only for the rest. The bow never delays the kit - a
  wallet that stops at the kit cost leaves the tool for a later
  errand once the saved adena reaches it.
- The plan is fixed, not the ordinary planner's: the stop list buys
  exactly the top no-grade set (the pieces the bag still misses; an
  owned line drops out and an empty stop costs no walk), one buylist
  per merchant (one buy transaction), through the same
  `tickStopShopping` pipeline the town trips use. Each kit purchase
  names the worn village piece of its slot (`SellFirst`), the stop
  sells it before the replacement lands (`stepReplacementSales`), and
  the errand dresses the bought kit once all stops are done. The bow
  purchase names the weaker owned bows it replaces: the two hand slot
  it would take belongs to the Falchion and the shield the melee dress
  keeps wearing, so nothing comes off the paperdoll, but the lure
  keeps only the strongest bow (see the junk keeps) and the old tool
  (the Forest Bow 272 behind the Composition Bow 273) sells before the
  new one lands, at its referencePrice/2 credit. The new bow rides
  home bagged for the lure (`EquipBaggedGear` never wears it under the
  melee profile).
- A fight holds the start: a pending loot (`phaseLoot`) or an
  incoming hit (`SelfUnderAttack`) keeps the errand off the tick, and
  a merely held engage target drops first (`dropAttackTarget`), so
  the walk never starts under the blows or over a kill to loot.
- The Scroll of Escape leg reuses the town trip economy
  (`maybeEscapeTo`): when the bag carries a scroll and the held
  ground's measured income prices the walk to Mirabel above the
  scroll, the scroll replaces the walk and the character lands at
  the village respawn next to Mirabel. A failed escape keeps the
  walking route.
- The bounds: `cityShopBudget` 30 minutes for the whole errand (the
  far elven cells sit up to 53 000 units from the village, near the
  10 minute quest walk timeout per walking segment),
  `cityShopStopTimeout` 4 minutes per merchant stop,
  `cityShopCooldown` 10 minutes between attempts and
  `cityShopHomeRadius` 300 units for the walk home. A hung errand
  ends inside the budget; the next ordinary tick hunts from wherever
  it stopped.
- The acceptance scenario `city-shopping` rides this production gate
  (no seam): it sets a level 15 character with 800 000 adena, the
  village dress and a Forest Bow on the Kaboo ground and verifies the
  Gludio arrival, the purchases (the Falchion, the armor set and the
  Composition Bow), the sales of the replaced village pieces and of
  the old bow, the worn kit, the walk home and the resumed farm.

Pinned by `internal/swarm/hunt/city_shopping_test.go` (the stock and
the list ids, the prices, the frozen purchase lines, the bow gate and
its sell first line of the weaker owned bows, the plan route order, the
sell first wiring, the start gate matrix with the region and profile
fences, the combat hold, the errand status hand-off) and the
`city-shopping` acceptance scenario
(`internal/swarm/acceptance/city_shopping.go`).

## Deleveling (internal/swarm/hunt/delevel.go)

When the character level exceeds the median level of the living
attackable npcs inside the zone by 7 or more AND the level is at least
10, the bot walks to the nearest archer guard (delevelGuards: the
Elven village sentinels Kendell and Starden only - display ids 7218 and
7220, Elven Bow, ARCHER ai type; the melee sentinels Veltress and
Rayen may never retaliate and are never provoked), approaches it into
melee range (60 units) and provokes it with the same
select-then-attack flow the hunt uses; the archer always answers a
provocation in its line of sight with bow shots and kills the
character, the village restart revives it next to the guards, where the
walk to the next death starts again. Only deaths at level 10+ remove
experience (Lucky absorbs the penalty below 10), so the deleveling
never triggers below 10 and its target never aims below 9. The vanilla
guard deaths pay the penalty from level 10 up (verified live: level
10, exp 48229 died to the archer Kendell -> level 9, exp 46190, lost
2039 of the 22972 level span), and a free death counter stays as the
safety net for servers where the deaths remove nothing: three
consecutive penalty-free deaths abort the deleveling and arm a 30 min
cooldown.

The guide floor caps the descent for the characters that reached the
support magic window: the Newbie Guide grants its buffs by level, and
the Life Cubic (skill 67) rides the narrow 16..19 band of the gate
(`SupportMagic.java`; the cubic is a summoned effect, never an
`AbnormalStatusUpdate`). The owner rule keeps that level-keyed reward
over the drop chance the deaths would restore below the window, so a
level 16 character never starts a deleveling at all (the target would
be the level itself) and the guard deaths of a level 17+ character stop
at 16 - the target is the level floor, never the raw drop boundary
that sits under it. A character under the window still delevels to the
Lucky floor: it never held the cubic.

The level gap rules of the drop calculation
(`NpcTemplate.calculateGroupDrops`) set the farming window: item drops
slide from 100 percent at mob level + 5 to 10 percent at + 10 and
beyond, adena from 100 percent at + 8 to 10 percent at + 15, and
experience and SP stop at + 11 (`MonsterExpMaxLevelDifference`). The
elven fields mobs are levels 1-5, so a level 11 character gets 10
percent item drops - the deleveling exists to fix that.

The deleveling stops at the target level = median zone mob level + 5
(the last level with the full item drop chance, floored at 9 and, once
the character stands above the guide window, at 16) and re-triggers
after hunting raised the level back above the trigger. The
fight stage re-paths when the guard ignores landed damage within 20 s
and aborts the deleveling after 3 failed re-paths; a stage without a
single landed blow never counts - the town guards sit at level 70
while the deleveling character climbs down from the low tens, the
vanilla `calcHitMiss` floors the hit chance at 20 percent there
(`(80 + 2 * (accuracy - evasion)) * 10` clamped to 200..980), so a
whole timeout window of misses is the normal variance of the gap and
the stage extends instead of blacklisting a guard that had no damage
to retaliate against (internal/swarm/state SelfLandedHit feeds the
distinction). The whole deleveling is bounded by a 60 min timeout and
a 1 min cooldown after it ends. The
walk segments are split into at most 1000 unit steps because the server
refuses move requests with a target farther than 9900 units
(MoveToLocation readImpl); the smoothed geodata routes happily exceed
that over open terrain. A death during the deleveling keeps the phase
running (the town trip aborts instead) and the destroy cleanup stays
suspended like during the town trips. Covered by
internal/swarm/hunt/delevel_test.go (trigger, hysteresis, guard fight,
death continuation, target exit, fight timeout, free death abort).

Delevel trigger caveat: `MedianZoneMobLevel` sees only the known
objects around the char; at the village (8000+ units from the fields)
the median is 0 and the delevel does not re-trigger - by design the
trigger fires only from the farm zone. The live median flickers with
the respawn windows (a ground whose designed mix sits close to the
character level shows a transient median a full trigger lower while
its higher species are down), so the trigger also requires the static
median of the anchored spot to agree with the live one (the 2026-09-12
building entry acceptance round de-leveled a healthy level 15 on the
Kaboo Orc Fighter SW respawn window whose static median is 9), and the
spot picker itself skips eligible grounds whose static median sits at
the delevel gap - the spot window (level-8) admits grounds the
deleveling would immediately answer with guard deaths.

## The quest dialog walker (hunt/quest_walker.go)

The dialog engine the M2 quest brain runs on: DriveDialog walks one
NPC conversation end to end from the two-click talk entry to the
answer page of the last bypass. The caller (the future quest trip
phase) keeps the character within the 250 unit interaction distance
of the npc through the whole conversation - the server gates every
bypass and every quest event on it (Player.processScriptEvent
resolves the npc through the last-folk memory and re-checks the
distance); the walker's entry clicks refresh that memory.

The walk: the paced two-click talk (the first Action click of a new
target only selects it, the second opens the html - NpcClick.onAction;
a click on an npc that is already the target interacts right away, so
the entry is idempotent), the bounded new-page wait (5 s per page,
polled every 250 ms), the link match by visible text (the
case-insensitive containment match: a route step names a distinctive
substring of the link label, "Challenge the test", "Change profession
to an Elven Knight"), the IsDialogCommand validation of the tracker
(only commands the open page offered are sent - the server drops the
rest silently) and the bypass send. Every page the walker sees is
parsed (ParseHTMLLinks) and applied to the tracker dialog section
BEFORE any bypass of that page fires, so the validation runs against
the page the command came from (the hunt-side feed of the state
dialog section; the connection layer stores the raw html only).

The arrival signal of a new page is the ARRIVAL GENERATION of the
dialog store, not the content: the connection layer keeps only the
last html (`LastHTMLDialogArrival` also returns the arrival counter),
the walker captures the counter before the entry talk, and a page
satisfies a wait only when it arrived past the last accepted one. A
page the store already held - the recurring Newbie Guide stop meeting
its own yesterday entry page - never passes, so the first bypass
fires only after the fresh entry page, which on the server means the
interact click already opened the dialog and rebuilt the html action
cache every bypass validates against (`RequestBypassToServer` drops
an unmatched bypass silently and no answer ever comes; the issue #35
dump logged the step 1 bypass before the fresh 401 byte entry page
arrived). The generation also retires the byte identical re-send
blind spot of a content comparison: a re-sent page is a fresh arrival
(the counter advanced) and the route proceeds through it instead of
lapsing into the timeout.

The pacing of the walk (hunt/quest_walker.go): the two entry clicks
space at the player action flood protector rate (one action per
second, `dialogClickPause` = `selectPeriod`), the bypass sends space
at `dialogBypassPace` (3.2 s) against the
`FloodProtectorServerBypassInterval` window (three 100 ms game ticks,
a bypass inside it is dropped without an answer), the page wait is
`questDialogWait` (5 s) polled at `questDialogPoll` (250 ms), and the
entry click outside the interaction distance makes the server pull
the character to the npc with the INTERACT intention. The last step
of a route may carry `DialogStep.AnswerIsEffect`: its bypass answers
with the effect instead of a page (the Newbie Guide support magic
casts every level-eligible buff and sends no NpcHtmlMessage), so the
walker returns without the final page wait and the caller owns the
effect wait.

The route steps are data: []DialogStep with the link texts in
conversation order. A step whose page never arrives, whose link text
the page does not offer, or whose command fails the open page
validation fails the walk with an error naming the step; the caller
retries the conversation from the entry (the server state of the
quest makes the re-entry idempotent - re-talking re-sends the page
of the current cond).

Live verified (the T-014 round, account dialogw1): the trainer page
of Ellenia at the elven village (the character injected at her
approach ring through the DB), the "Quest" link (the bare `bypass
Script` command the trainer pages carry) walked through to the
no-quest answer page in 1.7 s - the full click -> html -> link ->
bypass -> answer chain against the deployed stack. The SkillList
link of the same page answers with the SkillList packet, NOT an html
page - the walker routes only fit page-answering links. The opt-in
live suite (SWARM_LIVE_DIALOG=1, quest_walker_live_test.go) replays
the round trip; SWARM_LIVE_DIALOG_LINK overrides the route link for
ad-hoc experiments.

Covered by hunt/quest_walker_test.go (the scripted fakeGame wrapper
emulates the server html action cache: a bypass the open page never
offered is never answered) on the real Q00406 and
ElfHumanFighterChange1 datapack pages: the accept chain, the -h
prefix strip of the class change link, the stale-npc page guard, the
missing-link guard, the same-page repeat blind spot, the timeouts
and the argument guards.

## The quest trip engine (hunt/quest_trip.go)

The imperative chain runner the class transfer acceptance drives on
a manual loop (SetAutonomy(false), the loop never ticks - the trip
owns every move): `DriveQuestChain(ctx, chain)` consumes a
QuestChain of hunt/quest_chains.go end to end. The accept
conversation runs when the journal does not carry the quest (the
QuestEntryLinks static page prefix plus the chain Accept links
through DriveDialog), then the stage ladder walks by the journal
cond the tracker reports: a talk stage walks to its station
(FindQuestNpc, the knownlist scan of the display id on the wire
space) and drives the entry prefix plus the stage links, a kill
stage walks to the ground and engages the quest mobs until the
journal counters fill. The run ends when the exit talk drops the
quest from the journal (the proof item survives in the inventory,
the class change segment of the caller consumes it).

The script facts the engine rides (Q00406 read from the Mobius
source, no guessing): the 20th piece drop itself sets the next cond
(the giveItems setCond branch - the journal advances AT the kill,
the QuestList push follows), so the kill stage exits on the
counters and the ladder reads the next stage off the fresh cond. A
journal cond outside the ladder, a stage deadline (20 min per kill
stage, 5 min per walk) or a health floor breach (20 percent - the
manual trip has no rest phase, a dying character is the caller's
decision) returns an error naming the stage. The kill engage paces
the attack requests at 700 ms (the Mobius double click semantics:
the repeated request starts the fight), a ground with no living
quest mob in the 1600 scan radius holds for the respawn instead of
walking away. The class change segment (the Rains route) and the level
19 DB injection stay with the acceptance scenario of T-016.

## Live validated facts (2026-09-07 round, do not re-derive)

The town trips (sell loop) and the deleveling cycle are live verified
end-to-end on the vanilla server (only logging patches: the
DEATHLOG/GUARDDMG/MOVEDBG lines of the local checkout; the earlier
guard revenge + NPC kill penalty server patch was reverted - see the
server integrity rules in AGENTS.md).

- **Appearing fix confirmed working**: after adding the 0x30 reply to
  the self TeleportToLocation (connection/game_apply.go applyTeleport), the
  death -> village revive -> walk cycle worked: consecutive provoke ->
  die -> revive -> walk-to-guard cycles.
- **The aggressive default is TRUE**: the Mobius `NpcTemplate`
  `isAggressive` falls back to TRUE when the npc xml omits the
  attribute, so a mob like the Kaboo Orc Fighter attacks players on
  sight; the generated npc data mirrors this since the clan round.
  The effective on-sight attack is that flag AND a monster class
  template AND a positive effective range. `NpcTemplate` clamps the
  xml `aggroRange` at `MaxAggroRange` 450 (NPC.ini) for the `Monster`
  type only, so every aggressive C1 monster triggers at 450; the
  grand bosses keep their raw 1000-2000 but all four are
  `isAggressive=false`. Guards and friendly mobs reuse `AttackableAI`
  but their branches answer only a player with karma (or an
  aggressive monster), and folk never attack, so their xml
  `isAggressive=true` is not an on-sight threat.
- **Archer guards always retaliate, melee guards may not**: an archer
  guard in the attack intention shoots at everything within its 850+
  unit bow range (the thinkAttack doAttack branch applies no karma
  gate); a melee guard only follows the provoker (Guard.addDamage
  startFollow) and the chase dies in the checkTarget gate
  (Player.isAutoAttackable returns karma > 0 for guards). Verified in
  the world. The deleveling therefore provokes the archer sentinels
  Kendell and Starden only, in melee (60 unit approach).
- **Guard death experience penalty**: verified live on the vanilla
  server: a guard death at level 9 removes no experience while at
  level 10 the penalty lands (the Lucky newbie skill absorbs it below
  10; the doDie penalty branch runs for every killer, guards
  included). Live sequence: test1 level 10, exp 48229 provoked the
  archer Kendell in melee, died, and the penalty removed 2039 of the
  22972 level span -> level 9, exp 46190 (DEATHLOG lines in the game
  log). The bot's free death counter stays armed as the safety net for
  penalty-free servers and does not interfere (the productive death
  resets it).
- **Server geodata loads after the restart**: the game server runs
  with PathFinding = 2 and the region 21_19 in dist/game/data/geodata
  ("GeoEngine: Loaded 1 regions"); the bot pathfinds over its own repo
  copy in data/geodata (first candidate of the geodata detection).
- **Stale server side selections after abrupt disconnects**: an abrupt
  disconnect (a killed process, a dropped pipe) while the character
  auto attacks leaves the server side attack running; when that target
  dies the corpse stays SELECTED (the server never clears the
  selection, only the next selection replaces it) and every forced
  attack on the same object id of the next session comes back
  ActionFailed forever - observed live twice (inCombat true, 2 refused
  actions per second, no kills) after killing the bot mid-farm.
  Bot-side recovery (no server patch): the engage drops a target that
  never starts the fight within engageStuckTimeout (12 s) and skips it
  for engageSkipDelay (30 s), so the next pick selects a DIFFERENT
  object id - that selection replaces the stale one and the hunt
  resumes. The ATTACKLOG diagnostics lines in the local server
  checkout log which AttackRequest branch refused an action. The
  reproduction is timing dependent (the kill must land while the auto
  attack runs); three deliberate kill -9 attempts did not hit the
  window again, the fix is covered by unit tests instead.
- **Live validation result (2026-09-07)**: the full cycle ran on the
  vanilla server: trigger at level 10 over the level 1 gremlins ->
  pathfinding walk to Kendell (~85 s, geodata from the repo
  data/geodata) -> melee provocation -> the archer killed the
  character in ~7 s (GUARDDMG at distance 0) -> exp penalty: level 10
  -> 9 (DEATHLOG) -> "delevel finished at level 9, walking back" ->
  pathfinding walk back to the farm spot (~62 s) -> farming resumed
  (kills with loot). The bot then kept hunting at level 9 until the
  test window ended.
