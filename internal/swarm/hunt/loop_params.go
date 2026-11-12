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
    // tickPeriod is the decision cadence of the loop. The Mobius
    // PlayerActionFloodProtector accepts one player action per second,
    // so the actual requests stay rate limited by the engage periods
    // below; the short cadence only makes the state transitions (target
    // died, loot finished, health recovered) act within a quarter
    // second instead of a full one.
    tickPeriod = 250 * time.Millisecond
    // lootRadius is the distance around the character within drops are
    // picked up after a kill.
    lootRadius = 900.0
    // attackNearestRange bounds the target search radius.
    attackNearestRange = 1500.0
    // lootApproachRadius is the distance within which a ground item is
    // clicked instead of walked to: the server AI covers the last
    // stretch and executes the pickup.
    lootApproachRadius = 60.0
    // selectPeriod is the minimum pause between two nearest target
    // selections. The server accepts one player action per second
    // (PlayerActionFloodProtector), so selecting faster is pointless.
    selectPeriod = 1 * time.Second
    // reengageHealthPercent is the HP level above which the next target
    // is engaged immediately after a kill instead of resting. Below it
    // the loop waits for the regeneration - the same threshold as the
    // sit down, so a hurt character sits right away instead of standing
    // around: 30 percent may not be enough to kill the next mob and
    // turns every fight into a death risk.
    reengageHealthPercent = 60.0
    // sitDownHealthPercent is the HP level below which the resting
    // character sits down: the sitting regeneration is much faster.
    sitDownHealthPercent = 60.0
    // standUpHealthPercent is the HP level at which a sitting character
    // stands up again and resumes the hunt.
    standUpHealthPercent = 90.0
    // restRetryPeriod guards the sit/stand toggle against confirmation
    // lag: the ChangeWaitType broadcast confirms each transition and a
    // repeat is only sent when the flip never happened (lost packet),
    // so a slow confirmation can never toggle the character back.
    restRetryPeriod = 3 * time.Second
    // standSettlePeriod holds the movement through the server side
    // stand animation: the ChangeWaitType broadcast confirms the
    // standing at once, but the server keeps the character
    // paralyzed on the REST intention for the 2.5 s StandUpTask
    // window after it - move requests of that window bounce off
    // ActionFailed. Waiting the window out (plus a margin) makes
    // the first walk request land on a movable character.
    standSettlePeriod = 3 * time.Second
    // deathRestartPeriod is the pause between village restart requests
    // of a dead character: the server keeps a short death delay and
    // refuses early revives, so the request retries until it lands.
    deathRestartPeriod = 5 * time.Second
    // engageRetryPeriod is the pause between repeated forced attack
    // requests for the selected target. The server accepts one player
    // action per second (PlayerActionFloodProtector), so one second is
    // the fastest useful cadence.
    engageRetryPeriod = 1 * time.Second
    // pickupTimeout is how long one pickup attempt may take before the
    // item is skipped. It must cover the walk to the farthest item of
    // the loot radius plus the server side pickup.
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
    // segments that end without reaching the zone (a stuck walk aborts the
    // segment): past the budget the return falls back to the direct legacy
    // segments, which at least keep the character moving home.
    zoneReturnFailBudget = 3
    // roadFightBudget bounds the consecutive fights the out of zone
    // adoption may start during one walk home: the aggressive
    // territory on the road feeds a fresh attacker every respawn
    // window, so without a cap the adoption holds the character on
    // the road forever (the 2026-09-11 08:04 parallel round: the
    // walk home never resumed, the farm segment timed out on the road
    // fights). Past the budget the walk home continues through the
    // blows - the flee flow owns the hurt case, the mobs leash back
    // once the character leaves the aggro radius. The budget resets
    // on the zone entry.
    roadFightBudget = 3
    // engageStuckTimeout bounds how long the engage keeps re-requesting
    // a target that never actually starts the fight: the usual cause
    // is a stale server side selection (an abrupt disconnect left the
    // auto attack running, the target died and the server keeps the
    // corpse selected - it never clears the selection, only the next
    // selection of a DIFFERENT object replaces it), so every forced
    // attack on the same object id comes back refused forever.
    engageStuckTimeout = 12 * time.Second
    // userEngageRadius is the melee approach distance of an attack
    // command: inside it the forced attack request starts the swings,
    // outside it the chase (or the fallback walk) closes the distance
    // first.
    userEngageRadius = 150.0
    // userBowEngageRadius is the approach distance of an attack
    // command for a bow user: the Mobius bow attack range is the
    // weapon range (~500 units), so the forced request starts the
    // shot long before the melee distance is reached.
    userBowEngageRadius = 450.0
    // userBowStallRadius bounds the healthy distance of a ranged fight
    // in the chase stall watchdog: a bow fight stands and shoots
    // inside the weapon range and closes no chase distance while
    // doing it, which is progress, not a stalled chase.
    userBowStallRadius = 650.0
    // chaseProgressWindow bounds one progress sample of a chase: the
    // distance to the target is measured once per window.
    chaseProgressWindow = 3 * time.Second
    // chaseProgressStep is the distance a healthy chase closes within
    // one progress window; less than that counts as stalled.
    chaseProgressStep = 50.0
    // engageSkipDelay keeps a stuck target out of the target search:
    // the next selection of a different object already breaks the
    // stale state, the delay only stops the immediate re-pick of the
    // very same nearest npc.
    engageSkipDelay = 30 * time.Second
    // targetMaxLevelSlack bounds the mob level the engage initiates
    // on above the character level: two levels keep the experience
    // flow without turning every pull into a death risk (the
    // observed death ran at half health into a level 10 mob).
    targetMaxLevelSlack = 2
    // escapeHealthPercent is the HP level below which a running
    // fight is dropped and the character runs: pressing on below a
    // quarter of the health bar dies far more often than it kills.
    escapeHealthPercent = 25.0
    // losingFightGap is the health lead a target may hold over the
    // hurt character before the fight counts as lost (the level
    // gap pulls, the adds that joined a social pack).
    losingFightGap = 25.0
    // escapeTargetBeatenPercent is the target health below which a
    // hurt character finishes the fight instead of fleeing: the
    // kill is one swing away.
    escapeTargetBeatenPercent = 15.0
    // fleeSkipDelay keeps a fled target out of the search: walking
    // straight back into the mob the character just ran from
    // re-creates the same death risk at lower health. The delay
    // outlasts the server's own attack timeout (AttackableAI
    // MAX_ATTACK_TIMEOUT, 1200 ticks of 100 ms): a mob that keeps
    // chasing without landing a hit drops its aggro two minutes
    // after the last swing, and the skip must not lapse first. The
    // 2026-11-10 run-28 acceptance dump re-adopted a fled Kaboo Orc
    // Fighter exactly at the two-minute mark and restarted the
    // losing race that cost the window.
    fleeSkipDelay = 3 * time.Minute
    // escapeWalkDistance is one escape segment of the flee flow.
    escapeWalkDistance = 700.0
    // escapeFanStep is the angular spacing of the ordered escape
    // lane fan: the away ray first, then the lanes each side of it.
    // The run picks the first open lane of the order instead of
    // flipping between two opposite points through the chaser (the
    // reported 180 degree flips that ran the character back into
    // the mob it fled).
    escapeFanStep = math.Pi / 4
    // escapeFanSteps bounds the fan on each side of the away ray
    // (2: the 45 and 90 degree lanes).
    escapeFanSteps = 2
    // escapeZoneMargin keeps an escape endpoint off the exact square
    // edge: the server rounds the move coordinates, and the next
    // segment must still plan from inside the square.
    escapeZoneMargin = 60.0
    // escapeMinLane is the shortest escape segment worth a click: a
    // lane clamped shorter sits on the square edge and the walk
    // would read as a stall.
    escapeMinLane = 150.0
    // escapeNoProgressWindow is the wall clock window of the escape
    // no-progress watchdog: a flee that moved less than
    // escapeNoProgressDistance inside it is not an escape at all.
    escapeNoProgressWindow = 3 * time.Second
    // escapeNoProgressDistance is the planar distance the escape
    // must open per window. The accept rate of the server side walk
    // is hundreds of units per second, so a healthy escape clears it
    // by an order of magnitude.
    escapeNoProgressDistance = 120.0
    // noTargetPatience is the idle time before a targetless hunter
    // patrols toward the zone center: entering a zone engages the
    // first mob in reach, and only an empty radius keeps the
    // character walking toward the middle.
    noTargetPatience = 6 * time.Second
    // patrolCenterMinDist suppresses the center patrol when the
    // character already stands central: the respawns come to it.
    patrolCenterMinDist = 700.0
    // farTargetRange bounds the far target lookup of a targetless
    // hunter: the granular zone squares reach past the engage radius
    // (a 1300 half corner sits 1800+ units from the center), so the
    // nearest in-zone mob can stand far outside the pick radius. The
    // tracker only knows the mobs the server showed the character,
    // so this stays inside the loaded region block anyway.
    farTargetRange = 6000.0
    // deckReachableZ is the z gap a direct walk can clear: the
    // cell mode treats a visible mob whose z differs from the
    // character's z by more than this threshold as unreachable by a
    // direct walk (the village terrace deck sits ~600 units above
    // the field deck, the straight line crosses the railing and the
    // server cancels the move). The threshold matches the deck step
    // the elven lands geography models (the village at z -2992, the
    // fields at z -3500..-3600) and stays under the small height
    // steps of the field itself (the gentle terrain undulation
    // stays under 200 units, well inside the threshold). See the
    // cellEnemiesVisible deck gate and the 2026-09-14 07:53 dump.
    deckReachableZ = 400.0
    // noPickLogPeriod paces the targetless diagnostic of the
    // engage: the nearest rejected mobs with their positions and
    // the rejection reasons print at most once per period while
    // the pick comes up empty, so a standing hunter explains
    // itself in the log without spamming every tick.
    noPickLogPeriod = 5 * time.Second
    // noPickLogLimit bounds the targetless diagnostic: the log
    // lists the nearest rejected mobs, not the whole knownlist.
    noPickLogLimit = 3
    // panicLogoutHealthPercent is the HP level below which a
    // character under attack logs out for a pause: the escape
    // could not shake the chase, staying means dying (the
    // experience loss of a death at level 10+ costs hours of
    // farming).
    panicLogoutHealthPercent = 12.0
    // panicLogoutPause is the login cooldown of the emergency
    // logout: the supervisor waits it out before the next session.
    // The aggro resets the moment the character leaves the world -
    // the chasing mobs drop the target and start their walk home -
    // so a short pause that covers the logout round trip is all
    // the reset needs (observed live: a two second relogin lands
    // clean), and the character regenerates sitting through the
    // sessions that follow. A failed early login (the server still
    // holds the combat stance body) only costs one retry: the
    // supervisor backoff doubles it.
    panicLogoutPause = 2 * time.Second
    // panicLogoutAttackers is the aggro count that starts the
    // pile up run: two swinging mobs outdamage anything a lone
    // farmer can answer, and a social pack only grows while the
    // fight lasts - the fight is dropped and the logout happens
    // at a distance instead (see panicRunDistance).
    panicLogoutAttackers = 2
    // panicRunDistance is the escape distance the pile up run
    // must open from the aggro point before the logout fires:
    // logged out there, the character leaves the chasing pack
    // behind, the mobs walk home while the character is offline,
    // and the short relogin lands far outside their aggro range
    // instead of on top of the same pack.
    panicRunDistance = 600.0
    // fleeLogoutAfter bounds one flee episode: an escape that has
    // not shaken the chase within this budget ends the session
    // instead - the mobs keep the character running forever
    // otherwise, and the relogin after the pause resets their
    // aggro while the character regenerates sitting. The pile up
    // run shares the budget: a run that cannot open the escape
    // distance within it (a cornered or blocked escape) still
    // ends the session rather than running forever.
    fleeLogoutAfter = 20 * time.Second
    // fleeGapWindow and fleeGapOpen are the closure-rate contract of
    // the escape: within the window the threat distance must widen by
    // the open margin, otherwise the chaser keeps pace (the mobs of
    // the deployment run at the character's speed) and the session
    // ends early instead of burning the whole flee budget against a
    // chase that cannot be shaken. The stalled watch measures the
    // character's own travel; this one measures the gap.
    fleeGapWindow = 8 * time.Second
    fleeGapOpen   = 150.0
    // pileUpFightMaxAttackers is the attacker count a winnable pile
    // up still tanks (see pileUpWinnable): two mobs are the reported
    // softlock case, the third is the headroom for a wandering add
    // that trails the fight - the health gate escalates into the
    // panic run the moment the tanking turns into a death risk.
    pileUpFightMaxAttackers = 3
    // pileUpKillWindowPercent is the target health under which the
    // pile up gate never converts the standing kite into the flight:
    // one or two arrows finish the kill the fight already paid for.
    // The 2026-11-10 run-24 dump dropped a 24 percent grunt when a
    // level 10 Kaboo Orc Fighter joined; the kill was two arrows
    // away and the flight that replaced it ran into a corner pocket
    // 30 seconds later.
    pileUpKillWindowPercent = 30.0
    // settleWindow bounds the post relogin settle (see settle.go):
    // the spawn protection of the deployed stack lasts far longer
    // (600 s), the window only stops the hold from outliving the
    // stagnation watch budget - the sitting regeneration of the low
    // levels covers it with a wide margin.
    settleWindow = 5 * time.Minute
    // firstStrikeRange is the distance the post settle first strike
    // answers an aggressive mob from: the Power Shot cast range of
    // the bow - a shot or two land before the mob closes the melee
    // distance, and a farther mob is not worth breaking the spot
    // for (the ordinary pick flow walks to it).
    firstStrikeRange = 700.0
    // fightPotionHealthPercent is the health level a running fight
    // drinks a healing potion at: it must sit ABOVE the re-engage
    // threshold - the losing fight gates (the flee at 60, the panic
    // run at the same line) would otherwise always fire first and
    // the potion would never pour (the tanked pile up grinds the
    // bar down faster than the natural regeneration, and the potion
    // is the difference between the kill and the emergency logout).
    fightPotionHealthPercent = 70.0
)
