// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "time"
)

// Timing and threshold constants of the hunt loop.
const (
    // tickPeriod is the decision cadence of the loop: short enough that a
    // state transition (target died, loot finished, health recovered) acts
    // within a quarter second, while every request stays rate limited by the
    // engage periods (PlayerActionFloodProtector allows one per second).
    tickPeriod = 250 * time.Millisecond
    // lootRadius is the distance around the character within drops are
    // picked up after a kill.
    lootRadius = 900.0
    // attackNearestRange bounds the target search radius.
    attackNearestRange = 1500.0
    // lootApproachRadius is the distance within which a ground item is clicked
    // instead of walked to: the server AI covers the last stretch.
    lootApproachRadius = 60.0
    // selectPeriod is the minimum pause between two nearest target selections:
    // the server accepts one player action per second
    // (PlayerActionFloodProtector).
    selectPeriod = 1 * time.Second
    // reengageHealthPercent is the HP level above which the next target is
    // engaged right after a kill instead of resting: 30 percent may not be
    // enough to kill the next mob, and the sit threshold matches it.
    reengageHealthPercent = 60.0
    // sitDownHealthPercent is the HP level below which the resting
    // character sits down: the sitting regeneration is much faster.
    sitDownHealthPercent = 60.0
    // standUpHealthPercent is the HP level at which a sitting character
    // stands up again and resumes the hunt.
    standUpHealthPercent = 90.0
    // restRetryPeriod guards the sit/stand toggle against confirmation lag: the
    // ChangeWaitType broadcast confirms each transition, so a repeat is only
    // sent when the flip never happened.
    restRetryPeriod = 3 * time.Second
    // standSettlePeriod holds the movement through the server side stand
    // animation: the ChangeWaitType broadcast arrives at once, but the server
    // keeps the character paralyzed on the REST intention for the 2.5 s
    // StandUpTask window, and move requests of that window answer ActionFailed.
    standSettlePeriod = 3 * time.Second
    // deathRestartPeriod is the pause between village restart requests: the
    // server refuses early revives, so the request retries until it lands.
    deathRestartPeriod = 5 * time.Second
    // engageRetryPeriod is the pause between repeated forced attack requests:
    // one second is the fastest useful cadence (PlayerActionFloodProtector).
    engageRetryPeriod = 1 * time.Second
    // pickupTimeout is how long one pickup attempt may take before the item is
    // skipped: it covers the walk to the farthest item of the loot radius plus
    // the server side pickup.
    pickupTimeout = 20 * time.Second
    // pickupRetryDelay is how long a failed item stays skipped.
    pickupRetryDelay = 30 * time.Second
    // cleanupSlotPercent destroys junk at this inventory fill level.
    cleanupSlotPercent = 70.0
    // cleanupWeightPercent destroys junk at this weight level.
    cleanupWeightPercent = 75.0
    // destroyBatch is the number of junk items destroyed per cleanup.
    destroyBatch = 4
    // zoneReturnFailBudget bounds the consecutive pathfound zone return
    // segments that end without reaching the zone: past it the return falls
    // back to the direct segments, which at least keep the character moving
    // home.
    zoneReturnFailBudget = 3
    // roadFightBudget bounds the consecutive fights the out of zone adoption
    // may start during one walk home: the aggressive road feeds a fresh
    // attacker every respawn window, so without the cap the adoption holds the
    // character on the road forever. Past it the walk continues through the
    // blows; the budget resets on the zone entry.
    roadFightBudget = 3
    // engageStuckTimeout bounds how long the engage keeps re-requesting a
    // target that never starts the fight: the usual cause is a stale server
    // side selection (the server never clears it, only a different object
    // replaces it), so every forced attack on the same object id comes back
    // refused.
    engageStuckTimeout = 12 * time.Second
    // userEngageRadius is the melee approach distance of an attack command:
    // inside it the forced attack starts the swings, outside it the chase
    // closes the distance first.
    userEngageRadius = 150.0
    // userBowEngageRadius is the approach distance of an attack command for a
    // bow user: the Mobius bow attack range is the weapon range (~500 units),
    // so the forced request starts the shot long before the melee distance.
    userBowEngageRadius = 450.0
    // userBowStallRadius bounds the healthy distance of a ranged fight in the
    // chase stall watchdog: a bow fight stands and shoots inside the weapon
    // range without closing chase distance, which is progress, not a stall.
    userBowStallRadius = 650.0
    // chaseProgressWindow bounds one progress sample of a chase: the
    // distance to the target is measured once per window.
    chaseProgressWindow = 3 * time.Second
    // chaseProgressStep is the distance a healthy chase closes within
    // one progress window; less than that counts as stalled.
    chaseProgressStep = 50.0
    // engageSkipDelay keeps a stuck target out of the target search: a
    // different selection already breaks the stale state, the delay only stops
    // the immediate re-pick of the same nearest npc.
    engageSkipDelay = 30 * time.Second
    // targetMaxLevelSlack bounds the mob level above the character level the
    // engage initiates on: two levels keep the experience flow without turning
    // every pull into a death risk.
    targetMaxLevelSlack = 2
    // escapeHealthPercent is the HP level below which a running fight is
    // dropped and the character runs: pressing on below a quarter of the bar
    // dies far more often than it kills.
    escapeHealthPercent = 25.0
    // losingFightGap is the health lead a target may hold over the hurt
    // character before the fight counts as lost (the level gap, the adds of a
    // social pack).
    losingFightGap = 25.0
    // escapeTargetBeatenPercent is the target health below which a hurt
    // character finishes the fight instead of fleeing: the kill is one swing
    // away.
    escapeTargetBeatenPercent = 15.0
    // fleeSkipDelay keeps a fled target out of the search: walking straight
    // back into the mob the character just ran from re-creates the same death
    // risk at lower health. It outlasts the server's own attack timeout
    // (AttackableAI MAX_ATTACK_TIMEOUT, 1200 ticks of 100 ms), which drops the
    // aggro two minutes after the last swing.
    fleeSkipDelay = 3 * time.Minute
    // escapeWalkDistance is one escape segment of the flee flow.
    escapeWalkDistance = 700.0
    // escapeFanStep is the angular spacing of the ordered escape lane fan: the
    // away ray first, then the lanes each side, so the run never flips between
    // two opposite points through the chaser.
    escapeFanStep = math.Pi / 4
    // escapeFanSteps bounds the fan on each side of the away ray
    // (2: the 45 and 90 degree lanes).
    escapeFanSteps = 2
    // escapeZoneMargin keeps an escape endpoint off the exact square edge: the
    // server rounds the move coordinates, so the next segment must plan inside.
    escapeZoneMargin = 60.0
    // escapeMinLane is the shortest escape segment worth a click: a shorter
    // lane sits on the square edge and the walk would read as a stall.
    escapeMinLane = 150.0
    // escapeNoProgressWindow is the wall clock window of the escape watchdog:
    // a flee that moved less than escapeNoProgressDistance inside it is no
    // escape.
    escapeNoProgressWindow = 3 * time.Second
    // escapeNoProgressDistance is the planar distance the escape must open per
    // window: a healthy escape clears it by an order of magnitude.
    escapeNoProgressDistance = 120.0
    // noTargetPatience is the idle time before a targetless hunter patrols
    // toward the zone center: only an empty radius keeps the character walking.
    noTargetPatience = 6 * time.Second
    // patrolCenterMinDist suppresses the center patrol when the
    // character already stands central: the respawns come to it.
    patrolCenterMinDist = 700.0
    // farTargetRange bounds the far target lookup of a targetless hunter: the
    // granular zone squares reach past the engage radius (a 1300 half corner
    // sits 1800+ units from the center), so the nearest in-zone mob can stand
    // far outside the pick radius. The tracker only knows the mobs the server
    // showed.
    farTargetRange = 6000.0
    // deckReachableZ is the z gap a direct walk can clear: a visible mob
    // farther than this from the character in z counts as unreachable (the
    // village terrace deck sits ~600 units above the field deck, the straight
    // line crosses the railing and the server cancels the move). It matches the
    // elven deck step (the village at z -2992, the fields at -3500..-3600) and
    // stays above the terrain undulation of the field (under 200 units).
    deckReachableZ = 400.0
    // noPickLogPeriod paces the targetless diagnostic of the engage: the
    // nearest rejected mobs with their positions and reasons print at most once
    // per period while the pick comes up empty.
    noPickLogPeriod = 5 * time.Second
    // noPickLogLimit bounds the targetless diagnostic: the log
    // lists the nearest rejected mobs, not the whole knownlist.
    noPickLogLimit = 3
    // panicLogoutHealthPercent is the HP level below which a character under
    // attack logs out for a pause: the escape could not shake the chase, and
    // staying means dying.
    panicLogoutHealthPercent = 12.0
    // panicLogoutPause is the login cooldown of the emergency logout: the aggro
    // resets the moment the character leaves the world, so a pause that covers
    // the logout round trip is all it needs (a two second relogin lands clean).
    panicLogoutPause = 2 * time.Second
    // panicLogoutAttackers is the aggro count that starts the pile up run: two
    // swinging mobs outdamage a lone farmer, and a social pack only grows while
    // the fight lasts (panicRunDistance).
    panicLogoutAttackers = 2
    // panicRunDistance is the escape distance the pile up run must open from
    // the aggro point before the logout fires: logged out there, the relogin
    // lands outside the pack's aggro range instead of on top of it.
    panicRunDistance = 600.0
    // fleeLogoutAfter bounds one flee episode: an escape that has not shaken
    // the chase within this budget ends the session, and the relogin resets the
    // aggro while the character regenerates. The pile up run shares the budget.
    fleeLogoutAfter = 20 * time.Second
    // fleeGapWindow and fleeGapOpen are the closure-rate contract of the
    // escape: within the window the threat distance must widen by the open
    // margin, otherwise the chaser keeps pace and the session ends early.
    fleeGapWindow = 8 * time.Second
    fleeGapOpen   = 150.0
    // pileUpFightMaxAttackers is the attacker count a winnable pile up still
    // tanks (pileUpWinnable): two mobs are the reported softlock case, the
    // third is the headroom for a wandering add.
    pileUpFightMaxAttackers = 3
    // pileUpKillWindowPercent is the target health under which the pile up gate
    // keeps the standing fight instead of the flight: one or two arrows finish
    // the kill the fight already paid for.
    pileUpKillWindowPercent = 30.0
    // settleWindow bounds the post relogin settle (settle.go): the spawn
    // protection lasts far longer (600 s), the window only stops the hold from
    // outliving the stagnation watch budget.
    settleWindow = 5 * time.Minute
    // firstStrikeRange is the distance the post settle first strike answers an
    // aggressive mob from: the Power Shot cast range of the bow, so a shot
    // lands before the mob closes the melee distance.
    firstStrikeRange = 700.0
    // fightPotionHealthPercent is the health level a running fight drinks a
    // healing potion at: it sits above the re-engage and flee lines, which
    // would otherwise always fire first and the potion would never pour.
    fightPotionHealthPercent = 70.0
)
