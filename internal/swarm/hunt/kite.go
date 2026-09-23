// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The kiting layer of the archer fight (issue #13, the edge case
// slice of issue #19): a bow user that fights a mob from the weapon
// range never wants the mob in its face. The Mobius server AI stands
// the archer still while the auto attack shoots, and a melee mob that
// closes the distance simply swings away - the standing archer trades
// blow for blow with a mob it could outrange. The kite step answers
// exactly that: while the fight runs, a hostile that closed inside
// the retreat radius (the mob is a second or two from melee) makes
// the character step away, the movement window pauses the forced
// attack re-requests (a forced attack request interrupts a running
// walk server-side, the same rule the impending-add step of
// loop_avoid.go follows), and once the step finished the engage
// re-requests the attack - the bow shoots again from the opened
// distance. The cycle - stand, shoot, step clear, shoot - is the
// kiting behavior of the issue: the damage slows by the step
// overhead, the melee uptime drops to zero on a ground the mob cannot
// cross faster than the character.
//
// The edge case layer of this slice (issue #19) rides the same step
// the core round armed:
//
//   - the train direction: the retreat direction weighs EVERY chaser
//     on the character (the centroid away-vector of the fight target
//     plus the SelfAttackers train), not the armed threat alone - a
//     train member dragging west bends the retreat west before it
//     becomes the pile up. A train that surrounds the character (the
//     summed away vectors cancel - chasers stand on every side) is
//     too wide to outrun: the archer holds ground and shoots through
//     it. The neighboring camps never join the train: a retreat lane
//     that would wake an idle aggressive mob (the straight step
//     enters its on-sight trigger circle) deflects onto the tangent
//     ray of that circle - the same aggro-aware steering the transit
//     walks use (tangentClearDirection of loop_avoid.go).
//   - the corner: when no walkable retreat lane exists - the zone
//     leash, a wall behind (the geodata line of sight), water behind
//     (the OverWater oracle), or a lane every fan candidate of the
//     away hemisphere fails on - the archer stops retreating and
//     KEEPS SHOOTING THE BOW at melee range: no weapon switch, no
//     standing idle (the archetype rule of issue #21 - the bow is the
//     always-weapon, a cornered kite never degrades to a melee
//     trade). The cornered hold re-probes at the kite pacing, not
//     every tick - no idle stutter of failed probes - and the hunt
//     loop's watchdogs stay quiet through it: the stuck timeout only
//     owns a fight that never started (the cornered fight runs - the
//     swings keep it fresh), the chase stall watchdog only owns the
//     stretches beyond the bow stall radius (the cornered fight sits
//     at melee range, well inside it).
//   - the lane quality: the step prefers the open backward lanes
//     over the blocked corridors - the straight away-ray first, then
//     the fan candidates at 45 degree steps around the away
//     hemisphere; the wall, water, leash and camp gates run per
//     candidate, so a dead-end lane is re-planned at the very next
//     probe (one hop) instead of walking the character into it, and
//     no deflected lane ever folds back into the train (the away
//     half-plane gate).
//
// The honest limits of the layer, by the edge cases the issue names:
//   - the chaser positions of the DIRECTION scan are the raw
//     last-known packet positions (SelfAttackers), not the projected
//     ones - the centroid needs every chaser, the projected scan only
//     serves the nearest one (the armed threat of kiteThreat); a
//     chaser mid-chase lags its broadcast by up to a second, which
//     the 250 unit trigger margin absorbs.
//   - a retreat the gates clear but the server refuses (the click
//     validation collapses it, a mob body blocks the click point)
//     surfaces through the ordinary move-start watchdog machinery,
//     not through the kite - the kite re-probes on its pacing and
//     picks the next fan candidate then.
//   - the streak limit of the core is a DIAGNOSTIC since the
//     round-14 always-run redesign: a chaser at least as fast as
//     the character never falls behind, the counter names the
//     unwinnable race in the log, and the retreat keeps running on
//     the fight's own circle - the standing "fight it out" answer
//     is gone (it was the melee damage the kite exists to avoid).
//   - attack animation/travel time: the bow arrow flies ~500ms; the
//     step window pauses the re-requests but the server keeps the
//     already flying shots - a shot fired a moment before the step
//     still lands. Not modeled beyond that.
//
// The shot-paced layer of issue #60 rides the same machinery and
// flips the trigger: the character's own Attack broadcast is the
// server commit of the shot (the Mobius doAttack rolls the hit,
// consumes the arrow and schedules the HitTask BEFORE the packet
// goes out, and the damage task carries no attacker movement check),
// so the retreat starts the moment the shot released - inside the
// bow cooldown (timeAtk + reuse, roughly three seconds for the short
// bow) instead of after the mob crossed the retreat radius. The
// cycle becomes shoot, run the reload, gain distance, shoot again:
// the melee uptime drops to the stand moments, a chaser slower than
// the character never reaches the swings. The shot-paced step paces
// itself on the shot cycle, shares the lane battery and the holds
// with the proximity path and skips the streak counting - the
// shuffle bound exists for the swing-starving race, the rhythm keeps
// shooting once per cooldown.
//
// The re-click ladder of the issue #60 second round owns the WALK
// of the retreat: the owner's follow-up dump showed the rhythm
// trigger working (a "shot released" line every cycle) with the
// character still standing through every reload ("moving: no",
// the mob closing ~380 units per cycle) - the single retreat click
// barely executed. The manual clicking the owner demonstrated
// ("clicking behind the character runs 2 seconds no problem") is
// the recovery the ladder replicates: while the movement window
// runs and the character stands still on the issue cell, the click
// goes out again at the 600ms pace - the same endpoint first, then
// (once the probe names the endpoint click dead at the second
// re-click) the next fan candidate, then the rotated endpoint once
// more. The three candidate mechanisms the ladder answers: the
// stance transition swallow (the click lands while the server side
// ATTACK intention of the just-broadcast shot is still tearing
// down), the destination-cell refusal (the server refuses the cell
// the bot side LineOfSight blessed) and the H-006 silent drop (the
// deployment that swallows accepted move requests). The probe line
// and the rotation line of the ladder are the dump markers that
// name which mechanism ate the clicks of a given fight.
//
// The behavior arms itself on the weapon in hand (bowEquipped): no
// config, no class check - whatever bot ends up holding a bow (the
// planned archer type of the config launch, a lured guard round)
// kites its fights.

import (
    "math"
    "time"
)

// The kiting constants of the archer fight.
const (
    // kiteRetreatRadius is the trigger distance of the kite step and
    // the INNER edge of the optimal band: any bow fight hostile (the
    // target or a train member) closer than this to the fighting
    // character is about to reach melee (the swing range sits around
    // 150 units and the chasers run 120+ units per second), so the
    // step fires before the blows land. Above it the archer stands
    // and shoots - the standing fight inside the optimal band is the
    // good case.
    kiteRetreatRadius = 250.0
    // kiteStep is the retreat length away from the closed threat:
    // sized so the distance after the step (the mob follows at its
    // run speed through the ~2s window) lands back inside the
    // optimal band - the forced attack re-request then shoots from
    // range instead of starting a server side chase that would walk
    // the distance right back in.
    kiteStep = 400.0
    // kiteStepWindow is the movement window the step owns: the walk
    // runs ~400 units (about two seconds at the run speed), and a
    // forced attack request would interrupt it server-side, so the
    // engage holds its re-requests until the window closes - the
    // same contract the impending-add step uses
    // (combatAvoidStepPeriod).
    kiteStepWindow = 2 * time.Second
    // kiteReengageDelay is the hold between the step walk ending and
    // the first forced attack re-request of the re-engage: zero
    // today - the window end IS the re-engage (the opened distance
    // sits inside the optimal band, the re-request shoots from
    // range), the knob is pinned here so the live tuning round (#20)
    // can hold the aim without touching the walk contract.
    kiteReengageDelay time.Duration = 0
    // kiteStepPeriod paces the steps: the kite cycle spends the
    // window walking and the rest standing and shooting; the period
    // bounds the walking share from above (2s walk, 1s shoot at the
    // fastest). The cornered hold re-probes on the same pacing.
    kiteStepPeriod = 3 * time.Second
    // kiteStreakLimit is the DIAGNOSTIC threshold of the proximity
    // shuffle: a chaser at least as fast as the character never
    // falls behind, and the shuffle repeats without opening
    // distance. The round-14 redesign REMOVED the old "stop the
    // shuffle, fight it out" answer - the standing trade is the
    // melee damage the kite exists to avoid (the owner feedback: a
    // same-speed chase answered by standing eats the whole health
    // bar while a moving one outwaits the mob's home leash) - so
    // the counter now only names the unwinnable race in the log
    // while the retreat keeps running on the fight's own circle.
    kiteStreakLimit = 8
    // kiteTrainScanRange bounds the chaser scan of the retreat
    // DIRECTION: a train member farther than this from the character
    // cannot reach it inside a step window, so it cannot drag the
    // centroid - the flee machinery owns chasers that distant. The
    // value matches the combat avoid scan band (the impending-add
    // step reads the same neighborhood).
    kiteTrainScanRange = 800.0
    // kiteEncircleShare is the encirclement threshold of the train:
    // when the summed away vectors of the chasers shrink under this
    // share of their count (the unit vectors cancel - chasers stand
    // on every side), the centroid names no retreat direction and
    // the widest-gap bisector takes the step over (kiteGapDirection
    // - the live fleet round of issue #70 measured the standing
    // encircled fight as a death trap on the mass cells). A single
    // chaser always sums to a full unit (share 1.0); two chasers on
    // opposite sides sum to ~0.
    kiteEncircleShare = 0.3
    // kiteMaxTrainMembers caps the bearing scratch of the retreat
    // direction (kiteBearings): the gap geometry of a wider train
    // reads its nearest members alone - a train that wide outranks
    // the kite (the flee machinery's logout case), and the scratch
    // must stay a fixed array to keep the resolution allocation
    // free.
    kiteMaxTrainMembers = 16
    // kiteFanStep is the rotation increment of the retreat lane fan:
    // the straight away-ray first, then the candidates at its
    // multiples - a wall or a camp blocking the straight lane leaves
    // the 45 degree lanes, the leash corner leaves the 90 degree
    // ones.
    kiteFanStep = math.Pi / 4
    // kiteFanSteps bounds the fan on each side of the away-ray (2:
    // the 45 and 90 degree candidates each side). The fan spans the
    // away hemisphere only - a wider bend would step toward the
    // chasing train, and the half-plane gate rejects any deep camp
    // deflection that folds a lane back the same way.
    kiteFanSteps = 2
    // kiteHalfPlaneSlack is the tolerance of the away half-plane
    // gate: a lane may run perpendicular to the away-ray (the camp
    // side-step exits the trigger circle sideways), but never fold
    // back toward the train - the guard rejects the endpoints whose
    // retreat component turned negative beyond the float noise.
    kiteHalfPlaneSlack = -0.05
    // kiteBreakoutFlankCos is the flank-clearance bound of the
    // cornered-pocket breakout (issue #70, the walled-pocket round):
    // a breakout candidate serves the step only when its direction
    // keeps more than ~36 degrees (the cosine bound 0.809) off EVERY
    // chaser bearing. The breakout threads the cone the failed
    // hemisphere sweep never covered - BETWEEN the chasers when the
    // geometry leaves a lane - but it never runs onto a mob's own
    // ray: that is the melee the kite exists to avoid. The bound
    // clears the tightest weave the live pockets measured (the 50
    // and 45 degree margins of the two-chaser and three-chaser
    // corners) while refusing anything within a step of a bearing.
    kiteBreakoutFlankCos = 0.809
    // kiteCurveStep is the tangential bearing of the curving retreat
    // (issue #70, the fifth behavior): after the opening straight
    // retreat of a fight every later retreat leans this far off the
    // away-ray, on a turn side the fight keeps for its whole life -
    // the character circles the fight instead of marching one ray,
    // and the drift away from the farm point stays bounded by the
    // circle's radius instead of the leash. The bearing rides the
    // away half-plane (a mob that circled behind or a cornered
    // hemisphere mirrors the turn side rather than fold a lane
    // back into the train).
    kiteCurveStep = 70 * math.Pi / 180
    // kiteReshotFloor is the re-shot gate of the pursuit hold: the
    // distance under which the next bow windup would drag the
    // hostile down to melee - the windup debt (the ~1.45 s movement
    // standstill the live probe measured times the 110 run speed of
    // the elven-ground chasers the Mobius tables carry, ~160 units)
    // plus the retreat radius (the melee danger line the proximity
    // step holds) puts the BREAK-EVEN at 410; the fleet rounds of
    // issue #70 measured the passing slots riding a whole cycle
    // higher (the 496-511 medians of the quiet cells - the fight
    // distance oscillates between the floor and the floor minus the
    // debt, so the median sits ~80 under the floor), and the
    // break-even floor left the marginal cells a step under the
    // audit's 250 line once a hold or a volley dragged a cycle. The
    // floor sits ~70 over the break-even: the windup still ends
    // ~320 units clear of the melee line while the cycle median
    // rides the measured passing band. The fleet rounds without the
    // gate measured the spiral: the re-shot fires at the
    // walk-window end, restarts the windup, and the fight distance
    // collapses to the 13-68 unit medians of the max-range
    // verdicts. Above the floor the shot is affordable wherever the
    // fight stands; under it the pursuit continues the walk (the
    // server keeps the auto-attack displaced while the character
    // moves, and a 110 chaser cannot hit what it cannot catch).
    kiteReshotFloor = 480.0
    // kiteRoamRadius bounds how far the fight's retreats may drift
    // from the FIGHT ANCHOR - the self position at the fight's first
    // retreat resolution (see kiteResolveAndClick). The anchor is
    // the live-observed farm point of the fight: the curving circle
    // keeps the fight on its own ground wherever it started, and the
    // radius only fences the edge cases (a long straight pursuit
    // race, a breakout shove). This replaces the old hunting-square
    // leash (the zone.Contains read of the generated elven/dion
    // tables): the kite must stay LOCATION-GENERAL - it works on any
    // map, any ground, with or without a zone registry, and the
    // target economy alone owns the tables. Three bow engage radii
    // leave the healthy circle well inside while a dragged chase
    // still has room to maneuver.
    kiteRoamRadius = 3 * userBowEngageRadius
    // kiteShoveStep is the step length of the cornered-pocket SHOVE
    // (issue #70, the round-14 always-run redesign): the last escape
    // tier before the hold answers - a shorter, faster burst through
    // the widest angular gap of the chaser bearings, sized under the
    // ordinary retreat step so the shove clears the seam quickly
    // instead of marching the whole corridor.
    kiteShoveStep = 250.0
    // kiteShoveFlankCos is the flank-clearance bound of the shove:
    // the gap bisector must keep at least ~25 degrees (cos bound
    // 0.906) off every chaser bearing - TIGHTER than the breakout
    // cone (the 36 degrees of kiteBreakoutFlankCos) because the
    // shove is the last resort of a sealed pocket, where the only
    // lanes left thread closer to the flanks than the breakout
    // tolerates. A gap narrower than this is melee, not a seam.
    kiteShoveFlankCos = 0.906
    // kitePursuitContext bounds the freshness of the pursuit
    // context (kitePursuitFor/kitePursuitAt): the continuation
    // chain re-stamps every walk (the ~1.5 s window cadence), so
    // anything older than a few windows belongs to another fight -
    // the killed mob respawns under the SAME object id on the
    // Mobius ground (Spawn.respawnNpc keeps the id), and a stale
    // context must never answer the approach of the respawned mob
    // with a retreat. The bound sits under the fastest respawn of
    // the elven ground (RespawnMin 15 s) while surviving a LIVE
    // chain that pauses through a cornered hold episode: the fleet
    // round of the walled-pocket fix measured the holds stacking
    // between the walk windows (a 3 s hold block plus a windup
    // pushes a healthy chain past the earlier 6 s stamp - the fight
    // never ended, the continuation still belonged to it).
    kitePursuitContext = 12 * time.Second
    // kitePursuitStreakLimit is the runaway DIAGNOSTIC threshold of
    // the pursuit chain (issue #70, the round-14 always-run
    // redesign): the old "the chain cannot walk forever - the
    // standing fight keeps the damage on" backstop is GONE (the
    // standing trade is the melee damage the kite exists to avoid;
    // the fight-anchor leash and the curving circle bound the
    // drift instead), so the counter now only names the
    // never-resolving chain in the log while the chain keeps
    // running - the mob's own home leash ends a truly unwinnable
    // chase, the affordable re-shot ends a winnable one.
    kitePursuitStreakLimit = 12
    // kitePursuitStallLimit is the unwinnable-race DIAGNOSTIC
    // threshold: a chaser at least as fast as the character never
    // falls behind (the walk ends where it started, distance-wise).
    // The round-14 redesign removed the old "two stalled windows -
    // stand and fight" answer: the moving chase trades the melee
    // blows for chase-cadence blows at worst and keeps every
    // future shot affordable, so the ledger only names the parity
    // race in the log now.
    kitePursuitStallLimit = 2
    // kitePursuitStallSlack is the progress bar of one pursuit
    // window: a walk must open at least this much distance on the
    // nearest threat to count as progress. The honest elven-ground
    // race gains ~45 units a window (the 125-vs-110 speed margin
    // over the ~3 s walk window); the slack sits well under it so
    // the winning cells never stall, while a parity crawl slower
    // than half the honest gain reads as the stall it is (the
    // distance race that never reaches the floor must die at the
    // stall limit, not burn the backstop).
    kitePursuitStallSlack = 20.0
    // kiteHoldLogPeriod paces the cornered hold diagnostic: the hold
    // itself re-probes at the kite pacing, the log line lands once
    // per period - a cornered fight is a standing fight, the line is
    // its visibility, not a complaint.
    kiteHoldLogPeriod = 15 * time.Second
    // kiteShotWindow is the freshness window of the character's own
    // Attack broadcast that arms the shot-paced retreat (issue #60):
    // the packet is the server commit of the shot (the hit roll, the
    // arrow consumption and the HitTask schedule all happened before
    // the broadcast, and the damage task carries no attacker movement
    // check). The broadcast arms the DEFERRED retreat schedule (see
    // kiteArmClick - the issue #70 findings: the server forbids the
    // movement through the windup, so the click waits the windup end
    // instead of going out at the broadcast). The loop ticks four
    // times a second - the window keeps the broadcast catchable for
    // at least two ticks while still bounding the schedule to the
    // shot it belongs to.
    kiteShotWindow = 700 * time.Millisecond
    // kiteReclickPeriod paces the re-click ladder of the kite walk
    // (issue #60, the second round - the owner feedback: the bot
    // click "doesnt cover enough distance, dont start running",
    // while the manual clicking behind the character "runs 2
    // seconds no problem, covering large distance"): while the kite
    // movement window runs and the character still stands on the
    // issue cell, the retreat click goes out again at this pace -
    // the manual behavior the owner demonstrated, replicated by
    // the bot one click at a time. The period sits under the
    // server's once per second movement broadcast gate on purpose:
    // an accepted click may take up to a second to answer with the
    // broadcast, and a re-click that lands inside that gate just
    // re-aims the same endpoint server-side (the walk continues,
    // the click is not wasted) - exactly what the repeated manual
    // clicks do.
    kiteReclickPeriod = 250 * time.Millisecond
    // kiteReclickLimit bounds the re-clicks of one kite walk: the
    // initial click plus this many re-issues - the owner ask of the
    // third round is the FASTER spam ("it should spam clicks much
    // faster"), so the bound pins the packet budget of a dead
    // transport while the 250ms pace fills the whole window with
    // retries.
    kiteReclickLimit = 8
    // kiteProbeElapsed is the silent-verdict age of the walk: the
    // initial click plus the re-clicks stayed unanswered for this
    // long while the character still stands on the issue cell (no
    // ActionFailed, no movement broadcast - the H-006 silent drop
    // signature) names the endpoint click dead the way the probe
    // does. The value sits past the server's once-per-second
    // movement broadcast gate plus the pacing headroom - a healthy
    // accepted click answers with the broadcast inside it.
    kiteProbeElapsed = 1200 * time.Millisecond
    // kiteRefusalAnswerWindow bounds the ActionFailed attribution of
    // the kite clicks: the genuine refusal answer of a refused
    // endpoint arrives within the round trip (the acceptance dump
    // shows it landing the same millisecond), so a failure older
    // than one tick past the click belongs to some other request,
    // not the click. The window is deliberately TIGHT: the server
    // AI's own attack retries answer ActionFailed at a one-second
    // cadence through the whole bow disable (Creature.doAttack
    // schedules notifyActionReadyToAct a second ahead, each retry
    // on a still-disabled bow bounces) - a wide window would
    // misattribute that cadence to the kite clicks and rotate
    // healthy endpoints away (the local acceptance round caught
    // exactly that cascade before the tightening).
    kiteRefusalAnswerWindow = 250 * time.Millisecond
    // kiteBowReuseDelay is the reuse delay of the bow family the
    // fleet shoots (the C1 item data: the Short Bow and the D-grade
    // bows carry 1500, dist/game/data/stats/items - the value feeds
    // the bow disable window formula of kiteWalkWindow).
    kiteBowReuseDelay = 1500.0
    // kiteWalkWindowMin/Max clamp the bow-aware walk window: the
    // formula reads the live pAtkSpd, and a broken broadcast (a
    // zero, a partial) must never collapse or explode the window -
    // the clamp keeps the walk inside the sane attack-speed band
    // and falls back to the shipped kiteStepWindow outside it.
    kiteWalkWindowMin = 1 * time.Second
    kiteWalkWindowMax = 5 * time.Second
    // kiteWindupLead is the safety lead the deferred retreat click
    // adds past the windup end before it goes out (issue #70, the
    // kite timing findings): the live ladder measured the accepted
    // move window OPENING at the windup end - a click 1500 ms after
    // the shot moved at once (1.52 s) while a 1200 ms one stood to
    // the cycle end (2.96 s), the boundary sitting at the
    // theoretical (timeAtk+reuse)/2 = 1483 ms of pAtkSpd 337 - so
    // the click must land safely PAST that boundary despite the
    // quarter-second loop cadence and the broadcast lag. The lead
    // trades 150 ms of the walk tail for the certainty the click
    // rides the accepted window instead of the deferred one (the
    // deferred click is replayed only at the disable end where the
    // re-shot cancels it - the whole-reload stall the findings
    // measured as the fleet retreat lag).
    kiteWindupLead = 150 * time.Millisecond
    // kiteRaceFactor scales the DISTANCE DEFICIT into the race leg
    // length (issue #70, the round-17 race redesign): a retreat
    // issued under kiteReshotFloor must arrive BACK at the floor or
    // the arrival shot (the server auto re-shot the armed stance
    // fires the moment the walk ends) lands at the collapsed range
    // and the windup standstill hands the chaser the melee. The
    // honest physics of the parity chase: the character nets
    // (runSpeed - chaserSpeed)/runSpeed of every walked unit (~0.12
    // for the 125 runner vs the 110 elven chasers), so buying back
    // one unit of distance costs ~8 walked units - the factor that
    // turns the deficit into the leg. The base kiteStep rides on
    // top: the leg never shortens below the ordinary step.
    kiteRaceFactor = 8.0
    // kiteRaceStepMax caps the race leg: the leash-bounded circle
    // (kiteRoamRadius, three bow engage radii) carries a chord of
    // roughly 1.4x its radius at the 90 degree arc cap, so a leg
    // past ~1700 has no room on any circle the anchor leashes - the
    // deficit that deep belongs to the pursuit continuation chain,
    // not one walk.
    kiteRaceStepMax = 1700.0
    // kiteCircleMinRadius is the anchor-circle radius under which
    // the curving retreat keeps the straight away-ray: the opening
    // legs of a fight sit on (or near) the anchor cell itself - the
    // anchor IS the self position at the first retreat resolution -
    // and a chord of a near-zero circle folds straight through the
    // farm point. The circle takes over once the opening legs
    // carried the fight onto its own ring.
    kiteCircleMinRadius = 300.0
    // kiteCircleArcHalf caps the half-arc the circle chord
    // subtends (45 degrees, a 90 degree endpoint swing): past it
    // the chord direction folds toward the chase half-plane the
    // lane battery guards, and the equidistant swing buys drift
    // correction slower than it spends safety.
    kiteCircleArcHalf = math.Pi / 4
    // kiteCircleLeashShare caps the anchor-circle radius as a share
    // of the roam radius: the leash check of the lane battery
    // rejects endpoints OUTSIDE kiteRoamRadius, so the circle the
    // chord lands on stays a share inside it - float noise and the
    // cell granularity never push an equidistant endpoint over the
    // fence the straight legs respect.
    kiteCircleLeashShare = 0.92
    // kiteArrivalEpsilon is the arrival radius of the leg-long
    // re-click ladder (round 17): a stand within this distance of
    // the walk endpoint completes the leg (the server's cell
    // granularity and a partial route's last walkable cell never
    // leave the character exactly ON the clicked cell), while a
    // stand beyond it - mid-route, or short of the endpoint after a
    // partial path - re-arms the episode and keeps pushing the
    // endpoint.
    kiteArrivalEpsilon = 100.0
)

// KiteParams is the tunable block of the kite fight (owner issue
// #29): the four named knobs the live tuning round adjusts from the
// measured numbers, plus the enable gate. The launch config carries
// one per bot spec (the "kite" section of launch_config.go), so the
// standing-archer baseline (a bow bot with the kite disabled) and the
// tuned profile switch by editing the file instead of a code edit and
// a rebuild between the measurement runs. Every remaining kite
// constant (the streak limit, the train scan, the lane fan) stays a
// package constant on purpose: the tuning round names these four,
// the rest ride the shipped values until the numbers ask otherwise.

type KiteParams struct {
    // Enabled gates the kite layer whole: false turns every bow fight
    // into the standing archer the baseline measures (the mob closes,
    // the archer stands and shoots - the melee fallback rate of the
    // baseline comes from exactly this).
    Enabled bool
    // RetreatRadius is the trigger distance of the kite step and the
    // inner edge of the optimal band (kiteRetreatRadius is the
    // shipped value).
    RetreatRadius float64
    // Step is the retreat length away from the closed threat
    // (kiteStep is the shipped value).
    Step float64
    // StepPeriod paces the steps and the cornered hold re-probes
    // (kiteStepPeriod is the shipped value).
    StepPeriod time.Duration
    // ReengageDelay is the hold between the step walk ending and the
    // first forced attack re-request (kiteReengageDelay is the
    // shipped value).
    ReengageDelay time.Duration
}

// DefaultKiteParams returns the shipped tuning: the constants block
// above read into the params shape. A loop starts with these - the
// config file overrides per bot spec, everything else keeps the
// behavior the acceptance scenario (#28) pins.

func DefaultKiteParams() KiteParams {
    return KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: kiteReengageDelay,
    }
}

// SetKiteParams overrides the tunable block of the loop. A
// non-positive number or period falls back to the shipped value (a
// broken config line degrades to the shipped tuning, never to a
// degenerate fight), the re-engage delay keeps zero - it is a
// legitimate value (the window end IS the re-engage) - so only the
// negative side normalizes. The kite state carries over: the override
// between fights never resets the pacing or the streak.

func (l *Loop) SetKiteParams(p KiteParams) {
    if p.RetreatRadius <= 0 {
        p.RetreatRadius = kiteRetreatRadius
    }
    if p.Step <= 0 {
        p.Step = kiteStep
    }
    if p.StepPeriod <= 0 {
        p.StepPeriod = kiteStepPeriod
    }
    if p.ReengageDelay < 0 {
        p.ReengageDelay = kiteReengageDelay
    }
    l.kite = p
}

// The optimal band of the kite fight is the distance range where the
// standing archer is the good case: the inner edge is
// kiteRetreatRadius (a hostile below it arms the next step), the
// outer edge is the bow engage radius of the weapon-aware radii
// (userBowEngageRadius, user.go - 450), where the forced attack
// re-request shoots from range. The band is pinned as these two
// edge references on purpose - a copy of the engage radius here
// would drift from the radii the fight actually fights with - and
// the kiteStep length is sized against both edges.

// kiteThreat locates the hostile that arms the kite step: the fight
// target or the nearest train member - a living mob that already
// chases the character (its server target is the character, the
// projected NearestAttacker scan reads it, see scans.go) - whichever
// sits closer. A mob on a deck the walk cannot reach (the z gap past
// deckReachableZ, the same test the pick uses) is no melee threat.
// Returns the threat identity for the logs and its ground distance
// to the character (the retreat DIRECTION weighs the whole train
// through kiteTrainDirection, so the position itself has no reader).

func (l *Loop) kiteThreat(
    selfX, selfY, selfZ int32,
) (id int32, dist float64, ok bool) {
    if tx, ty, _, tok := l.tracker.ObjectPosition(l.target); tok {
        dx := float64(selfX) - float64(tx)
        dy := float64(selfY) - float64(ty)
        id, dist, ok = l.target, math.Hypot(dx, dy), true
    }
    if attacker, aok := l.tracker.NearestAttacker(); aok &&
        attacker.ObjectID != l.target {
        if attacker.Z == 0 ||
            math.Abs(float64(attacker.Z-selfZ)) <= deckReachableZ {
            dx := float64(selfX) - float64(attacker.X)
            dy := float64(selfY) - float64(attacker.Y)
            adist := math.Hypot(dx, dy)
            if !ok || adist < dist {
                id, dist, ok = attacker.ObjectID, adist, true
            }
        }
    }

    return id, dist, ok
}

// kiteLayerGates covers the gates every kite layer shares: the
// params triple (the enabled profile, the bow in hand, the fight
// target), the movement window (a running step or an impending-add
// walk owns it - no second walk may interrupt it) and the cornered
// hold pacing (a retreat with no walkable lane re-probes at the
// kite period, not every tick). Reports whether a kite step of any
// layer may run now.

func (l *Loop) kiteLayerGates(now time.Time) bool {
    // The params gate comes first: a kite-disabled profile (the
    // standing-archer baseline of issue #29) never arms the layer,
    // whatever the bow and the fight report.
    if !l.kite.Enabled || !l.bowEquipped() || l.target == 0 {
        return false
    }
    if now.Before(l.combatAvoidUntil) {
        return false
    }
    if !l.kiteHeldAt.IsZero() && l.kiteHeldFor == l.target &&
        now.Sub(l.kiteHeldAt) < l.kite.StepPeriod {
        return false
    }

    return true
}

// kiteStepAdmitted guards the kite step of one tick: the shared
// layer gates must pass (see kiteLayerGates), the kite pacing must
// have aged out (the step re-probe), and a fresh target resets the
// streak of the previous one (the distance race of one mob is not
// the race of the next). The round-14 always-run redesign removed
// the old streak-limit hard stop ("stop the shuffle, fight it
// out"): the standing trade is the melee damage the kite exists to
// avoid, so the proximity step keeps answering a closed hostile
// for as long as the fight runs - the streak counter crosses
// kiteStreakLimit only as the diagnostic that names the unwinnable
// race. Reports whether the step may run now.

func (l *Loop) kiteStepAdmitted(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    if !l.kiteAt.IsZero() && now.Sub(l.kiteAt) < l.kite.StepPeriod {
        return false
    }
    if l.kiteFor != l.target {
        // A fresh target: the streak of the previous one is history.
        l.kiteFor = l.target
        l.kiteStreak = 0
    }
    if l.kiteStreak == kiteStreakLimit {
        // The diagnostic crossing (exactly once per crossing): the
        // race is unwinnable, the retreat keeps running anyway - a
        // same-speed chase outwaits the mob's home leash while a
        // standing one eats the health bar.
        l.logf("Hunt: the distance race on %d is unwinnable, "+
            "running it out on the circle (step %d)",
            l.target, l.kiteStreak)
    }

    return true
}

// kiteFromTarget steps a bow fighting character away from the
// hostile that armed the step (the target or a train member, see
// kiteThreat): the archer buys back the weapon range instead of
// tanking the melee. The step DIRECTION is the centroid away-vector
// of the whole chaser train (the target plus every attacker in
// range, see kiteTrainDirection), the lane prefers the open backward
// lanes over the blocked corridors (the 45 degree fan candidates
// behind the straight ray), and a neighborhood camp the lane would
// wake deflects it onto the tangent ray of the camp's trigger
// circle. A train that surrounds the character or a retreat with no
// walkable lane resolves to the cornered hold: the archer stops
// retreating and keeps shooting the bow at melee range - never a
// weapon switch (the archetype rule of issue #21). The step
// respects the movement window contract of the fight steps (the
// forced attack re-requests wait out the walk), and the streak limit
// (an unwinnable distance race falls back to the ordinary fight). A
// fresh target resets the streak. The step timing honors the live
// shot cycle (kiteShotPhase): inside the bow windup the click
// defers to the windup end (the server would save an immediate one
// and replay it at the disable end where the re-shot cancels it),
// inside the accepted move window the click goes out at once with
// the cycle end as its window, and with no live cycle the ordinary
// full window applies. Reports whether the tick issued or armed the
// step (the caller skips the rest of the fighting branch then - the
// walk owns the movement).

func (l *Loop) kiteFromTarget(now time.Time) bool {
    if !l.kiteStepAdmitted(now) {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= l.kite.RetreatRadius || dist < 1 {
        return false
    }
    clickAt, until, live := l.kiteShotPhase(now)
    if live && now.Before(clickAt) {
        // The shot cycle holds its windup: an immediate click would
        // defer to the disable end and die at the re-shot (the
        // measured deferral of the issue #70 findings). The pressure
        // trigger rides the same deferred schedule the shot-paced
        // rhythm arms - one click at the boundary, the whole reload
        // tail of walking.
        if l.kiteArmClick(clickAt, until) {
            l.logger.Printf("Hunt: hostile %d closed to %d units "+
                "of the fight on %d inside the windup, the retreat "+
                "waits the windup end", threatID,
                int(math.Round(dist)), l.target)

            return true
        }

        return false
    }
    if !live {
        // No shot cycle owns the moment (the re-engage wait, a
        // chase without a broadcast): the ordinary full window.
        until = now.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered (see
        // kiteResolveAndClick): the hold ground rule owns the cycle.
        return false
    }
    l.kiteStreak++
    l.logger.Printf("Hunt: hostile %d closed to %d units of the "+
        "fight on %d, kiting clear (step %d, the unwinnable-race "+
        "diagnostic bound %d)",
        threatID, int(math.Round(dist)), l.target,
        l.kiteStreak, kiteStreakLimit)

    return true
}

// kiteThreatBeyondThePursue reports whether the threat left the
// pursue band with no live attack behind it (the round-14 fleet
// duel finding): a hostile beyond the bow engage radius that is NOT
// an attacker (a fleeing melee mob, a target standing off) is the
// stall watchdog's re-approach case, while a LIVE ATTACKER beyond
// the band - the measured ranged duel, a Kaboo shooter holding 474
// units and trading arrows - keeps the shot-paced rhythm moving:
// the standing answer ate 21 s of idle trading in one measured
// window while the melee adds closed on the stationary archer. The
// strafe rides the same curving retreat the rhythm always runs (the
// distance holds near the bow's own max), capped at the train scan
// range - beyond it neither side reaches the other and the rhythm
// has nothing to pace.

func (l *Loop) kiteThreatBeyondThePursue(
    threatID int32, dist float64,
) bool {
    if dist < userBowEngageRadius || dist >= kiteTrainScanRange {
        return dist >= kiteTrainScanRange
    }
    for _, attacker := range l.tracker.SelfAttackers() {
        if attacker.ObjectID == threatID {
            // A live attacker beyond the band (the ranged duel)
            // still rides the rhythm.
            return false
        }
    }

    return true
}

// kiteFromShot arms the shot-paced retreat of the bow fighting
// character (issue #60, reworked on the issue #70 findings): the
// Attack broadcast of the character is the server commit of the bow
// shot - the hit roll, the arrow consumption and the HitTask
// schedule all happened before the packet, the damage task carries
// no attacker movement check - but the measured server cycle FORBIDS
// the movement through the first half of the bow disable window (the
// windup): a MoveToLocation inside it is saved by the server AI and
// replayed only at the disable end, where the re-shot cancels it
// (PlayerAI.setIntentionMoveTo saves the intention, the replay rides
// notifyActionReadyToAct - see docs/kite_timing_findings.md). The
// broadcast therefore arms the DEFERRED retreat (kiteArmClick): the
// click waits the windup end plus the safety lead, then fires
// through kiteClickWalk into the accepted move window and the walk
// banks the reload tail - the shoot, run the reuse tail, re-shoot
// cycle the issue asks for. The retreat fires once per shot while a
// hostile holds the pursue band (inside the bow engage radius - a
// mob that keeps chasing the fight), the lane machinery (the train
// direction, the camp deflection, the leash, the wall and the water
// gates, the cornered hold) resolves at the click moment - the
// chasers move while the character stands the windup out - and the
// rhythm does NOT count toward the streak limit: the shots never
// starve. A hostile beyond the band leaves the standing fight (the
// stall watchdog owns the re-approach). Reports whether the tick
// armed the deferred retreat.

func (l *Loop) kiteFromShot(now time.Time) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    // The trigger itself: the character's own shot, fresh inside the
    // broadcast window.
    shotAt := l.tracker.SelfLastShotAt()
    if shotAt.IsZero() || now.Sub(shotAt) > kiteShotWindow {
        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || l.kiteThreatBeyondThePursue(threatID, dist) || dist < 1 {
        return false
    }
    clickAt, until, live := l.kiteShotPhase(now)
    if !live {
        // The disable window cannot lapse while the broadcast is
        // fresh (the window outlasts the freshness gate on every
        // clamped speed); the guard keeps a degenerate broadcast
        // (a zero pAtkSpd tearing the formula) from arming a
        // schedule in the past.
        return false
    }
    if now.Before(clickAt) {
        // The windup holds the movement: the retreat defers to the
        // accepted window (the measured deferral of an immediate
        // click costs the whole reload of standing time).
        if l.kiteArmClick(clickAt, until) {
            l.logger.Printf("Hunt: shot released on %d, hostile %d "+
                "holds %d units, the retreat clicks at the windup "+
                "end", l.target, threatID, int(math.Round(dist)))

            return true
        }

        return false
    }
    // The broadcast tick landed inside the accepted window already
    // (the late catch of a fast bow): the retreat clicks at once,
    // the walk owns the remainder of the cycle.
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered.
        return false
    }
    l.logger.Printf("Hunt: shot released on %d, hostile %d holds "+
        "%d units, kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteStepLength resolves the retreat leg length the moment arms
// (issue #70, the round-17 race redesign): the plain kiteStep while
// the fight holds the re-shot floor (the ordinary band step - the
// fight distance is safe, the shot is affordable wherever it
// stands), and the RACE leg under it - the deficit to the floor
// bought back at the parity exchange rate (kiteRaceFactor) plus the
// base step, so the leg ARRIVES back at the floor instead of ending
// a few dozen units short of it. The round-15/16 fleet rounds
// measured the fixed 400-unit leg landing at the melee medians
// (141 units) cycle after cycle: the walk window equals the shot
// cycle, the character stops at the endpoint exactly when the
// server's re-shot is ready, and the armed stance fires it at the
// collapsed range - the second shot of the cycle the owner named,
// and the windup standstill of that shot hands the chaser the
// melee. The race leg arrives at the floor, so the arrival shot (or
// the re-request at the window end) is the ONE shot of the cycle,
// fired from the safe distance. The leash budget caps the straight
// legs: the endpoint of a straight step must stay inside the roam
// radius around the fight anchor, and a fight already at the leash
// edge keeps at least the shove tier of room (the circle chord of
// the curving legs lands equidistant by construction and never
// needs the budget).

func (l *Loop) kiteStepLength(
    dist float64, selfX, selfY int32,
) float64 {
    if dist >= kiteReshotFloor {
        return l.kite.Step
    }
    step := l.kite.Step + (kiteReshotFloor-dist)*kiteRaceFactor
    if step > kiteRaceStepMax {
        step = kiteRaceStepMax
    }
    if l.kiteFightFor == l.target {
        // The leash budget of the straight legs: the worst-case ray
        // runs the radius straight out, so the anchor distance plus
        // the step must stay inside the roam fence.
        anchorDist := math.Hypot(
            float64(selfX-l.kiteFightX),
            float64(selfY-l.kiteFightY))
        if budget := kiteRoamRadius - anchorDist; budget < step {
            step = math.Max(kiteShoveStep, budget)
        }
    }

    return step
}

// kiteResolveAndClick resolves the retreat lane against the live
// train and terrain and issues the retreat click through the ladder
// seam: the encircled train (the away vectors cancel - chasers stand
// on every side) and the cornered battery (no walkable lane in the
// whole away hemisphere) answer with the hold ground rule of the
// archetype (never a weapon switch - the bow is the always-weapon),
// a walkable lane clicks at once with the movement window the caller
// computed. The shared seam serves the deferred click of the shot
// cycle (kiteClickWalk), the accepted-window catch of the broadcast
// tick and the ordinary proximity step - one resolution, one click
// path, one ladder. The leg LENGTH is race-aware (kiteStepLength):
// the live threat distance at the resolution moment sizes the step,
// so a fight collapsed under the re-shot floor banks the whole
// deficit into one long continuous walk instead of a chain of
// window-sized shuffles. Reports whether the click went out.

func (l *Loop) kiteResolveAndClick(
    now, until time.Time, selfX, selfY, selfZ int32,
) bool {
    if l.kiteFightFor != l.target {
        // The fight's own anchor (the round-14 location-general
        // leash): the self position at the FIRST retreat resolution
        // of this target is the live-observed farm point the whole
        // fight circles - the retreat leash (kiteTerrainLane) and
        // the curve turn side (kiteCurveSide) read it instead of
        // the hunting-square registry, so the kite runs on any
        // ground, mapped or not.
        l.kiteFightFor = l.target
        l.kiteFightX, l.kiteFightY = selfX, selfY
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        // The degenerate no-chaser scene (a tracker gap inside the
        // broadcast window): no ray to guard, the hold keeps the
        // pacing until the next probe.
        l.kiteHoldGround(now, true, "")

        return false
    }
    // The curving retreat (issue #70, the fifth behavior): the
    // opening retreat of a fight runs the straight away-ray, every
    // later one aims the CHORD of the fight's anchor circle - the
    // endpoint lands exactly the circle's radius away from the
    // anchor (the equidistant point of the owner's circling spec),
    // on the turn side the fight keeps for its whole life. The RAW
    // away vector stays the half-plane reference of the lane
    // battery (the chord may bend the preferred ray, never the
    // guard). An encircled train KEEPS its raw ray as the
    // preference: the gap bisector already encodes the escape
    // geometry, and a circle chord on top of it would fold the step
    // back onto a flanker.
    _, threatDist, threatOK := l.kiteThreat(selfX, selfY, selfZ)
    stepLen := l.kite.Step
    if threatOK {
        // The race-aware leg length (kiteStepLength): the deficit
        // to the re-shot floor at the resolution moment sizes the
        // leg - one long continuous walk to the floor, not a
        // window-sized shuffle that ends short of it.
        stepLen = l.kiteStepLength(threatDist, selfX, selfY)
    }
    prefX, prefY := dirX, dirY
    if !encircled {
        prefX, prefY = l.kiteCurveDirection(
            dirX, dirY, selfX, selfY, stepLen)
    }
    stepX, stepY, found := l.kiteRetreatLaneSkipping(
        selfX, selfY, selfZ, prefX, prefY, dirX, dirY,
        stepLen, l.kiteDeadCellsFor())
    if !found {
        // The hemisphere sweep is walled (the raw away-ray of a
        // normal train, the gap ray of an encircled one): BEFORE the
        // hold answers, the pocket breakout probes the cone the
        // sweep never covered - the anti-gap rays that thread
        // between the chasers (issue #70, the walled-pocket round:
        // the fleet measured the standing answer eating whole
        // windows, an 8.5 s median shot-to-retreat lag on the
        // pocket cell, while a lane through the train's own seams
        // stood open south of the chaser line). The refusal reason
        // rides the hold diagnostic - a breakout that never fires
        // must name WHY it refused, or the pocket stays invisible
        // in the event feed.
        endX, endY, why, broke := l.kiteBreakoutResolve(
            selfX, selfY, selfZ, l.kiteDeadCellsFor())
        if broke {
            l.kiteHeldAt = time.Time{}
            l.kiteAt = now
            l.kiteIssueWalk(now, until, selfX, selfY, selfZ,
                endX, endY)
            l.logger.Printf("Hunt: the retreat hemisphere on "+
                "target %d is walled, breaking out through the "+
                "clearance lane %d %d", l.target, endX, endY)

            return true
        }
        // The SHOVE tier (issue #70, the round-14 always-run
        // redesign): the hemisphere and the breakout cone both
        // refused, and the old answer - hold the ground and shoot
        // the way out - is the standing melee trade the kite exists
        // to avoid (the owner feedback named it: the archer does
        // not run, eats the damage). The shove threads the WIDEST
        // ANGULAR GAP of the chaser bearings at a tighter flank
        // bound than the breakout tolerates - a last-resort burst
        // through the seam the cone refused - so a sealed pocket
        // still answers with movement whenever the geometry leaves
        // any lane at all. Only a pocket with no gap geometry left
        // (every ray crowded or walled) keeps the hold.
        endX, endY, shoved := l.kiteShoveResolve(
            selfX, selfY, selfZ, l.kiteDeadCellsFor())
        if shoved {
            l.kiteHeldAt = time.Time{}
            l.kiteAt = now
            l.kiteIssueWalk(now, until, selfX, selfY, selfZ,
                endX, endY)
            l.logger.Printf("Hunt: the pocket on target %d has no "+
                "clean lane, shoving through the widest gap %d %d",
                l.target, endX, endY)

            return true
        }
        // Cornered: no walkable lane in the whole guarded
        // hemisphere (the raw away-ray of a normal train, the gap
        // ray of an encircled one), the breakout cone and the
        // shove all refused. The hold ground answer is the archetype
        // rule - the bow is the always-weapon, the cornered archer
        // never switches to a melee trade, it stands and shoots the
        // way out.
        l.kiteHoldGround(now, encircled, why)

        return false
    }
    l.kiteHeldAt = time.Time{}
    l.kiteAt = now
    l.kiteIssueWalk(now, until, selfX, selfY, selfZ, stepX, stepY)

    return true
}

// kiteArmClick arms the deferred retreat click of the live shot
// cycle: the click time, the walk window end and the owning fight
// land in the scheduler state (see kiteClickWalk - the issue #70
// findings: the server saves a MoveToLocation inside the bow windup
// and replays it only at the disable end, where the re-shot cancels
// it, so the click waits the windup out) and the movement window the
// forced attack re-requests respect stretches to the same end - a
// re-request inside the cycle would interrupt the planned walk, and
// inside the windup it only burns the server's bow disable answer
// anyway. The arming is idempotent per schedule: the very same click
// time (the very same shot) re-arms silently - the caller yields the
// tick only on a fresh schedule. Reports whether the arming spent
// the tick.

func (l *Loop) kiteArmClick(clickAt, until time.Time) bool {
    if l.kiteClickFor == l.target && l.kiteClickAt.Equal(clickAt) {
        // The schedule of this very shot is already armed (a second
        // trigger of the same cycle): the tick stays with the fight
        // machinery (the potions, the casts).
        return false
    }
    l.kiteClickAt = clickAt
    l.kiteClickFor = l.target
    l.kiteClickUntil = until
    l.combatAvoidUntil = until
    // A fresh shot cycle starts a fresh pursuit race: the re-shot
    // just fired (the distance the previous chain raced for opened,
    // or the windup ended affordable), so the continuation ledger
    // of the old cycle - its spent steps, its stalls, its open
    // walk - is history (the same-id respawn edge dies here too:
    // whatever a previous fight against this mob id left in the
    // ledger never inherits into the new cycle's chain).
    l.kitePursuitSteps = 0
    l.kitePursuitDist = 0
    l.kitePursuitStall = 0
    l.kitePursuitWalkOpen = false

    return true
}

// kiteClickWalk fires the deferred retreat click at the scheduled
// boundary (see kiteArmClick): the windup of the arming shot held
// the movement until now, so the click rides the accepted move
// window - the server adopts the intention at once (the movement
// broadcast follows) instead of saving it for the disable end. The
// fire re-checks everything the windup may have changed: the fight
// the schedule belongs to (a target switch - the old target died, a
// fresh pick - disarms it), the threat band (a hostile that left the
// pursue band is the stall watchdog's re-approach, not a retreat)
// and the lane itself (the chasers moved while the character stood -
// the train direction, the camp deflection and the dead-cell memory
// all resolve fresh at the click moment). The encircled and the
// cornered answers hold the ground as always. Reports whether the
// tick spent the click.

func (l *Loop) kiteClickWalk(now time.Time) bool {
    if l.kiteClickAt.IsZero() {
        return false
    }
    if l.kiteClickFor != l.target || l.target == 0 {
        // The fight moved on: the schedule served the shot that
        // armed it, a fresh fight re-arms its own. The movement
        // hold dies with the ownership - but only the hold the
        // arming itself set (the Equal guard): a walk already in
        // flight keeps its own window, and the fire-time threat
        // re-check below keeps the shot's real disable hold (a
        // re-request the server would only refuse through the
        // disable).
        if l.combatAvoidUntil.Equal(l.kiteClickUntil) {
            l.combatAvoidUntil = time.Time{}
        }
        l.kiteClickClear()

        return false
    }
    if now.Before(l.kiteClickAt) {
        // The windup still holds the movement: the tick stays with
        // the fight machinery (the potions, the casts) - the
        // movement window and the re-request gate are already armed.
        return false
    }
    if now.After(l.kiteClickUntil) {
        // The disable lapsed between the ticks (a stalled tick, a
        // blocking write): a click now would race the re-shot and
        // lose a whole cycle (the 3000 ms probe row of the findings)
        // - its walk window is already past, the ladder would die
        // on arrival and the forced re-request would cancel the
        // just-issued walk. The schedule stands down; the next shot
        // arms its own.
        l.kiteClickClear()

        return false
    }
    until := l.kiteClickUntil
    l.kiteClickClear()
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || l.kiteThreatBeyondThePursue(threatID, dist) || dist < 1 {
        // The hostile left the pursue band (or the chase dissolved):
        // the retreat has nothing to retreat from - the stall
        // watchdog owns the re-approach, the next shot cycle re-arms
        // the rhythm on its own broadcast.
        return false
    }
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered (the hold paces
        // its own re-probe): the next cycle re-arms fresh.
        return false
    }
    l.logger.Printf("Hunt: the windup on %d ended (hostile %d at "+
        "%d units), kiting the reload tail", l.target, threatID,
        int(math.Round(dist)))

    return true
}

// kiteClickClear stands the deferred click schedule down: the next
// shot cycle (or the proximity trigger of the next fight) arms its
// own schedule whole. The dead-cell memory and the ladder state
// belong to their own lifecycles (the target and the walk).

func (l *Loop) kiteClickClear() {
    l.kiteClickAt = time.Time{}
    l.kiteClickFor = 0
    l.kiteClickUntil = time.Time{}
}

// kiteShotPhase resolves the server bow cycle a retreat must
// respect: the shot broadcast anchors the windup (the server forbids
// the movement before its end - the saved intention of
// PlayerAI.setIntentionMoveTo) and the disable end (no re-shot
// before it lapses), and the retreat click owns the reuse tail
// between the two (the accepted move window of the issue #70
// findings: the earliest immediate retreat sat at the windup end,
// 1500 ms after the shot at pAtkSpd 337, while a click inside the
// windup stood to the full cycle end). Reports the earliest accepted
// click time (the windup end plus the safety lead), the walk window
// end (the disable end plus the re-engage delay) and whether the
// cycle is still live - a cycle that ended leaves the retreat to
// the ordinary full window.

func (l *Loop) kiteShotPhase(
    now time.Time,
) (clickAt, until time.Time, live bool) {
    shotAt := l.tracker.SelfLastShotAt()
    if shotAt.IsZero() {
        return time.Time{}, time.Time{}, false
    }
    until = shotAt.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    if now.After(until) {
        // The disable lapsed: the re-engage owns the moment, and a
        // retreat riding the dead cycle would hold its window for
        // nothing.
        return time.Time{}, time.Time{}, false
    }
    clickAt = shotAt.Add(l.kiteWindupWindow() + kiteWindupLead)

    return clickAt, until, true
}

// kiteCurveDirection bends the retreat ray into the circling ring of
// the fight (issue #70, the fifth behavior, reworked round 17): the
// OPENING retreat of a fight runs the straight away-ray (the panic
// retreat opens the distance at once), every later one aims the
// CHORD of the fight's anchor circle - the endpoint sits exactly the
// circle's radius away from the FIGHT ANCHOR (the live-observed
// ground the fight started on, see kiteResolveAndClick), swung the
// fight's fixed turn side, so the character keeps the SAME distance
// to the point it kites around: the owner's equidistant circling
// spec, exactly. The chord carries the race leg (kiteStepLength)
// when the circle has the room for it: the radius grows to the
// length the leg needs (capped by the leash share), the half-arc
// caps at kiteCircleArcHalf (a 90 degree endpoint swing - past it
// the chord folds toward the chase half-plane the battery guards),
// and a leg the capped circle cannot carry walks the shorter chord
// anyway (the pursuit continuation re-arms the remainder). The
// turn side is the fight's own constant (kiteCurveSide picks it
// once: toward the FIGHT ANCHOR when the first retreat anchored the
// fight, a fixed side otherwise). The advance to the curved steps
// belongs to the issued walk alone (kiteIssueWalk): a cornered or
// refused retreat spends nothing, the opening straight step stays
// armed until a walk really goes out. The degenerate scenes - no
// anchor, the opening radius under kiteCircleMinRadius, a chord
// that degenerates to a point - keep the straight away-ray: the
// circle owns the fight only where the fight really runs on one.

func (l *Loop) kiteCurveDirection(
    dirX, dirY float64, selfX, selfY int32, step float64,
) (float64, float64) {
    if l.kiteCurveFor != l.target {
        l.kiteCurveFor = l.target
        l.kiteCurved = false
        l.kiteCurveSign = l.kiteCurveSide(dirX, dirY, selfX, selfY)
    }
    if !l.kiteCurved {
        return dirX, dirY
    }
    if l.kiteFightFor != l.target || step < 1 {
        // No anchored circle (or a degenerate leg): the straight
        // away-ray serves the step.
        return dirX, dirY
    }
    toSelfX := float64(selfX - l.kiteFightX)
    toSelfY := float64(selfY - l.kiteFightY)
    radius := math.Hypot(toSelfX, toSelfY)
    if radius < kiteCircleMinRadius {
        // The opening legs still sit on the anchor cell: a chord of
        // a near-zero circle folds straight through the farm point.
        return dirX, dirY
    }
    // The circle the chord lands on: the current radius, grown to
    // the length the leg needs (the chord of the half-arc cap
    // subtends 2*R*sin(kiteCircleArcHalf) ~= 1.41*R), fenced by the
    // leash share so the equidistant endpoint never crosses the
    // roam fence the battery enforces.
    radiusTarget := math.Max(
        radius, step*math.Sin(kiteCircleArcHalf))
    leashMax := kiteRoamRadius * kiteCircleLeashShare
    if radiusTarget > leashMax {
        radiusTarget = leashMax
    }
    // The half-arc the chord subtends: asin(step / 2R), capped at
    // the arc bound (the chord of a capped circle walks shorter than
    // the leg - the pursuit continuation re-arms the remainder).
    half := math.Asin(math.Min(1, step/(2*radiusTarget)))
    if half > kiteCircleArcHalf {
        half = kiteCircleArcHalf
    }
    // The equidistant endpoint: the radius vector swung the full
    // arc (2*half) on the fight's turn side, rescaled to the target
    // radius - the endpoint keeps the circle's distance to the
    // anchor whatever the straight legs drifted it to.
    unitX, unitY := toSelfX/radius, toSelfY/radius
    swingX, swingY := rotatePlanar(
        unitX, unitY, l.kiteCurveSign*2*half)
    endX := float64(l.kiteFightX) + swingX*radiusTarget
    endY := float64(l.kiteFightY) + swingY*radiusTarget
    prefX := endX - float64(selfX)
    prefY := endY - float64(selfY)
    if size := math.Hypot(prefX, prefY); size >= 1 {
        return prefX / size, prefY / size
    }

    return dirX, dirY
}

// kiteCurveSide picks the turn side of a fight's circle once: the
// side whose tangential ray leans back toward the FIGHT ANCHOR
// (the live-observed ground the fight started on, see
// kiteResolveAndClick - the round-14 location-general redesign
// replaced the hunting-zone center read with it), a fixed
// counterclockwise side before the first retreat anchors the fight
// (and on any ground the anchor never landed).

func (l *Loop) kiteCurveSide(
    dirX, dirY float64, selfX, selfY int32,
) float64 {
    if l.target == 0 || l.kiteFightFor != l.target {
        return 1
    }
    toCenterX := float64(l.kiteFightX - selfX)
    toCenterY := float64(l.kiteFightY - selfY)
    if size := math.Hypot(toCenterX, toCenterY); size >= 1 {
        ccwX, ccwY := rotatePlanar(dirX, dirY, kiteCurveStep)
        if ccwX*toCenterX/size+ccwY*toCenterY/size >=
            dirX*toCenterX/size+dirY*toCenterY/size {
            return 1
        }

        return -1
    }

    return 1
}

// kiteIssueWalk sends the retreat click of one kite step and arms
// the re-click ladder on it (issue #60, the second round): the walk
// endpoint, the issue cell and the window land in the ladder state
// (see kiteReclickWalk) so the dead-click recovery - the repeated
// click the owner's manual evidence names - rides the SAME retreat
// instead of the character standing through the reload a swallowed
// click leaves it. Every kite path (the deferred click of the shot
// cycle, the accepted-window catch, the proximity step) issues its
// walks through this one seam; a fresh step resets the ladder whole
// (a walk already in flight owns the movement, the layer gates hold
// the double step). The window end is the caller's phase-aware
// moment: the deferred click of a live cycle ends at the shot's
// disable end (the walk owns the reuse tail, the forced attack
// re-request fires the moment it lapses), a cycle-less step keeps
// the full window from now.

func (l *Loop) kiteIssueWalk(
    now, until time.Time, selfX, selfY, selfZ, stepX, stepY int32,
) {
    l.kiteWalkX, l.kiteWalkY, l.kiteWalkZ = stepX, stepY, selfZ
    l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
    l.kiteWalkIssuedAt = now
    // The window covers the WHOLE leg (the round-17 race redesign):
    // the race leg runs many times the ordinary step's walk time,
    // and a window that lapses mid-leg stands the ladder down and
    // hands the tick to the forced attack re-request - which fires
    // the shot at whatever collapsed range the leg has reached so
    // far, the exact two-shots-per-cycle failure the owner named.
    // The window scales with the walk distance at the per-unit pace
    // of the bow-aware window formula (kiteWalkWindow per
    // kiteStep), so the leg runs to its endpoint (or the pursuit
    // continuation re-arms the next one) before anything else owns
    // the movement.
    if leg := math.Hypot(
        float64(stepX-selfX), float64(stepY-selfY)); leg > l.kite.Step {
        scaled := now.Add(time.Duration(
            leg / l.kite.Step * float64(l.kiteWalkWindow())))
        if scaled.After(until) {
            until = scaled
        }
    }
    l.kiteWalkUntil = until
    l.combatAvoidUntil = until
    // The curving circle advances on the issued walk alone: the
    // opening straight retreat of the fight is spent, every later
    // retreat leans the tangential bearing (kiteCurveDirection).
    l.kiteCurved = true
    l.kiteReclickAt = now
    l.kiteReclicks = 0
    l.kiteWalkDead = false
    if l.kiteDeadFor != l.target {
        // A fresh fight: the dead-cell memory of the previous
        // target is history - the terrain refusal verdicts belong
        // to the ground the OLD fight stood on. The memory of THIS
        // target persists across the walk cycles (the cells the
        // server refused stay refused - the terrain does not
        // change between the shots, and the later cycles start on
        // a lane that already proved walkable instead of paying
        // the probe tax on the same dead straight cell every
        // cycle).
        l.kiteDeadFor = l.target
        l.kiteWalkDeadCount = 0
    }
    if l.kitePursuitFor != l.target ||
        now.Sub(l.kitePursuitAt) > kitePursuitContext {
        // The walk starts a fresh pursuit chain: a new fight, or the
        // previous chain's context expired before this walk (the
        // respawned same-id target of the Mobius ground). The
        // pursuit ledger must not carry the old race's stalls into
        // the new one (the shot-cycle reset of kiteArmClick covers
        // the re-shot path; this seam covers the proximity-first
        // fights and the post-respawn walks that re-arm the context
        // without a shot in between).
        l.kitePursuitSteps = 0
        l.kitePursuitStall = 0
        l.kitePursuitWalkOpen = false
        l.kitePursuitDist = 0
    }
    l.kitePursuitFor = l.target
    l.kitePursuitAt = now
    if err := l.game.WalkTo(stepX, stepY, selfZ); err != nil {
        l.logger.Printf("Hunt: kite walk failed: %v", err)
    }
}

// kiteWindupWindow resolves the bow windup the live server forbids
// the movement in: the Attack launch arms isAttackingNow for the
// first half of the bow disable window (Creature.doAttack, the BOW
// branch: _attackEndTime = now + timeToHit + reuse/2 = now +
// (timeAtk+reuse)/2, with timeToHit = timeAtk/2 - the Mobius C1
// source read of the issue #70 round), and PlayerAI.setIntentionMoveTo
// answers a move inside it by SAVING the intention and replaying it
// at notifyActionReadyToAct - the disable end, where the re-shot
// cancels the replay. The live probe confirmed the boundary: the
// 1500 ms click after the shot moved at once while the 1200 ms one
// stood to the cycle end (docs/kite_timing_findings.md). The window
// rides the same pAtkSpd formula, clamps and fallback as
// kiteWalkWindow - exactly its first half.

func (l *Loop) kiteWindupWindow() time.Duration {
    return l.kiteWalkWindow() / 2
}

// kiteWalkWindow resolves the movement window the kite walk owns:
// the bow disable window the C1 server enforces between the shots -
// timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
// (Creature.calculateTimeBetweenAttacks and calculateReuseTime, the
// reuseDelay of the bow family in the item data) - read from the
// live pAtkSpd the StatusUpdate broadcasts (attr 0x12, the value
// the acceptance dump shows as 337 for the Short Bow kit, giving
// roughly (500000 + 1500*333)/337 = 2.97s - the "3 seconds to draw
// shot" the owner measured by hand). The walk then spends the WHOLE
// cooldown running (the owner ask of the third round: the proper
// archering delays from the C1 Mobius formulas - if it is not
// shooting it should be moving away from the target) instead of the
// shipped fixed 2s that stood the character idle for the last
// second of every reload. A missing or absurd pAtkSpd broadcast
// falls back to the shipped kiteStepWindow (the walk contract
// never depends on the packet being parsed).

func (l *Loop) kiteWalkWindow() time.Duration {
    spd := l.tracker.SelfPAtkSpd()
    if spd <= 0 {
        return kiteStepWindow
    }
    ms := (500000.0 + kiteBowReuseDelay*333.0) * float64(time.Millisecond) /
        float64(spd)
    if ms < float64(kiteWalkWindowMin) || ms > float64(kiteWalkWindowMax) {
        return kiteStepWindow
    }

    return time.Duration(ms)
}

// kiteClickRefused reports whether the server answered the last
// kite click with ActionFailed: the one byte refusal answer carries
// no request identity, so the correlation is the send time (the
// same contract the town walker's refusalEvidence runs) - a failure
// that arrived after the click, inside the round-trip window, with
// no other request between, belongs to the click. The acceptance
// dump of the third round shows the kite endpoint refusals landing
// the same millisecond the click went out: the server (the Mobius
// MoveToLocation handler - the isCompletelyBlocked destination
// check among others) refuses specific destination cells outright,
// and the bot's own geodata does not always agree (the dump cells
// read open in the shipped pack), so the ONLINE evidence is the
// only refusal channel that never lies.

func (l *Loop) kiteClickRefused() bool {
    if l.kiteReclickAt.IsZero() {
        return false
    }
    failedAt := l.tracker.LastActionFailed()

    return failedAt.After(l.kiteReclickAt) &&
        failedAt.Sub(l.kiteReclickAt) <= kiteRefusalAnswerWindow &&
        !l.tracker.OtherRequestBetween(l.kiteReclickAt, failedAt)
}

// kiteRotateDeadEndpoint rotates the dead endpoint of the ladder
// onto the next fan candidate: the dead cell joins the refused set
// (the rotation never re-clicks a cell the server already refused -
// a refused cell never starts a walk) and the lane battery resolves
// the next walkable lane skipping the whole set, the fresh train
// direction included (the chasers moved while the character stood).
// Reports whether a fresh endpoint serves the retreat: an
// encircled train or a battery that finds no lane leaves the
// rotation empty and the caller stands the ladder down.

func (l *Loop) kiteRotateDeadEndpoint(
    selfX, selfY, selfZ int32,
) bool {
    if l.kiteWalkDeadCount < len(l.kiteWalkDeadCells) {
        l.kiteWalkDeadCells[l.kiteWalkDeadCount] =
            [2]int32{l.kiteWalkX, l.kiteWalkY}
        l.kiteWalkDeadCount++
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        return false
    }
    // The rotation stays DIRECTION-CENTERED (the raw ray the
    // half-plane guards - the centroid of a normal train, the gap
    // bisector of an encircled one): the dead-endpoint recovery is
    // a coverage sweep of the whole hemisphere, the circle owns
    // the fresh retreat preference alone - a rotation that clung
    // to the curved ray would sweep only the three candidates left
    // of its fan. The rotation keeps the leg LENGTH race-aware:
    // the live threat at the rotation moment sizes the step.
    stepLen := l.kite.Step
    if _, threatDist, threatOK := l.kiteThreat(
        selfX, selfY, selfZ); threatOK {
        stepLen = l.kiteStepLength(threatDist, selfX, selfY)
    }
    stepX, stepY, found := l.kiteRetreatLaneSkipping(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY,
        stepLen, l.kiteWalkDeadCells[:l.kiteWalkDeadCount])
    if !found {
        // The dead-endpoint sweep exhausted the hemisphere: the
        // pocket breakout probes the cone it never covered with the
        // SAME dead set - a refused or silent anti-gap lane lands in
        // the memory like any other, the next probe starts on what
        // the battery has left.
        endX, endY, _, broke := l.kiteBreakoutResolve(
            selfX, selfY, selfZ,
            l.kiteWalkDeadCells[:l.kiteWalkDeadCount])
        if broke {
            l.logger.Printf("Hunt: rotating the kite retreat of "+
                "target %d onto the breakout lane %d %d (the "+
                "hemisphere exhausted, the endpoint %d %d refused "+
                "or silent)",
                l.target, endX, endY, l.kiteWalkX, l.kiteWalkY)
            l.kiteWalkX, l.kiteWalkY = endX, endY

            return true
        }

        return false
    }
    l.logger.Printf("Hunt: rotating the kite retreat of target %d "+
        "onto the lane %d %d (the endpoint %d %d refused or silent)",
        l.target, stepX, stepY, l.kiteWalkX, l.kiteWalkY)
    l.kiteWalkX, l.kiteWalkY = stepX, stepY

    return true
}

// kitePursuitHold answers whether a lapsed kite walk re-arms as the
// pursuit continuation of the same fight (issue #70, the max-range
// behavior): the fleet rounds measured the walk-window cycle losing
// the distance race - the re-shot at the window end restarts the
// windup standstill, the 110-speed chaser collects ~160 units of it
// back, and the fight spirals to the melee medians of the max-range
// verdicts. The issue's contract inverts the priority: the retreat
// strives for the maximum distance FIRST, the re-shot waits for it.
// While the nearest hostile holds inside the re-shot floor (the
// windup debt plus the retreat radius - the distance the next
// windup would eat down to melee) the lapsed window re-arms one
// more walk through the ordinary lane resolution, and the server
// keeps the auto-attack displaced while the character moves (a 110
// chaser cannot hit what it cannot catch). The chain carries its
// OWN budget apart from the proximity streak: the progress ledger
// (kitePursuitSteps/Stall/Dist/WalkOpen) resets on every fresh shot
// cycle (kiteArmClick), counts only the COMPLETED walks (a cornered
// hold re-probe never mints a stall - the pocket cells survive
// their holds on exactly that), and stops the chain two stalled
// windows in a row (the unwinnable parity race) or at the flat
// backstop (the oscillating chase that resets the stall ledger
// forever). The first moment the hostile clears the floor - or the
// ledger, the leash or the lane battery stops the chase - the walk
// stands down and the re-request fires the affordable shot from the
// opened distance. Reports whether the tick spent the continuation.

func (l *Loop) kitePursuitHold(
    now time.Time, selfX, selfY, selfZ int32,
) bool {
    if !l.kiteLayerGates(now) {
        return false
    }
    if l.kitePursuitFor != l.target ||
        now.Sub(l.kitePursuitAt) > kitePursuitContext {
        // No FRESH kite retreat of THIS fight stands behind the
        // moment (the approach phase, a chase without a shot, or a
        // stale context of a previous fight against the SAME mob id
        // - the Mobius respawn keeps the object id, the stamp
        // expires the history): the continuation belongs to a live
        // retreat chain alone - the engage keeps its attack request
        // (the live fleet round measured the approach slot walking
        // its cell empty).
        return false
    }
    threatID, dist, ok := l.kiteThreat(selfX, selfY, selfZ)
    if !ok || dist >= kiteReshotFloor || dist < 1 {
        // The shot is affordable (or the scene is degenerate): the
        // re-request owns the tick. The walk the ledger still owes
        // ends on the WON race here - the arming of the re-shot's
        // own cycle clears the ledger whole (kiteArmClick).
        return false
    }
    if l.kitePursuitWalkOpen {
        // A pursuit-issued walk spent its window: the progress
        // accounting it owed runs now. The comparison is the
        // distance at THIS window end against the distance at the
        // walk's own issue - the walk opened the gap or it did not.
        // A hold re-probe never reaches here (no walk issued behind
        // it, the flag stays down): the cornered pause between the
        // walks is not a lost race step.
        if dist <= l.kitePursuitDist+kitePursuitStallSlack {
            l.kitePursuitStall++
        } else {
            l.kitePursuitStall = 0
        }
        if l.kitePursuitStall == kitePursuitStallLimit {
            // The diagnostic crossing (once per crossing): the race
            // is at parity, the chain keeps running anyway - see
            // kitePursuitStallLimit, the round-14 always-run rule.
            l.logf("Hunt: the pursuit race on %d is at parity "+
                "(%d stalled windows), running the chase out",
                l.target, l.kitePursuitStall)
        }
        l.kitePursuitWalkOpen = false
        l.kitePursuitDist = dist
    }
    if l.kitePursuitSteps == kitePursuitStreakLimit {
        // The diagnostic crossing (once per crossing): the chain
        // never resolved yet, the walk keeps going - the
        // fight-anchor leash and the circle bound the drift (see
        // kitePursuitStreakLimit, the round-14 always-run rule).
        l.logf("Hunt: the pursuit chain on %d ran %d walks "+
            "without a shot, holding the course",
            l.target, l.kitePursuitSteps)
    }
    until := now.Add(l.kiteWalkWindow() + l.kite.ReengageDelay)
    if !l.kiteResolveAndClick(now, until, selfX, selfY, selfZ) {
        // The encircled or cornered hold answered the continuation
        // the same way it answers the opening step.
        return false
    }
    l.kitePursuitSteps++
    l.kitePursuitDist = dist
    l.kitePursuitWalkOpen = true
    l.logger.Printf("Hunt: hostile %d holds %d units at the walk "+
        "end, the pursuit continues the retreat (step %d of %d)",
        threatID, int(math.Round(dist)), l.kitePursuitSteps,
        kitePursuitStreakLimit)

    return true
}

// kiteReclickWalk is the re-click ladder of the kite retreat (issue
// #60, the rounds two and three; the leg-long rework of round 17):
// the single retreat click of the first round proved fragile three
// ways, and the owner's dumps show the character standing through
// the whole bow reload while the mob closed. The third-round
// acceptance dump named the dominant mechanism exactly: the server
// answers SOME destination cells with an instant bare ActionFailed
// (the MoveToLocation handler refusal family - the
// isCompletelyBlocked destination check among others) while the
// rotated fan lane a few cells aside walks fine, and the bot's own
// geodata reads the refused cells open, so only the ONLINE
// evidence settles it. The ladder answers with the manual behavior
// the owner demonstrated ("clicking behind the character runs 2
// seconds no problem"), faster now per the third-round ask:
//
//   - while the movement window runs and the character stands
//     (wherever the leg finds it - the issue cell, or mid-route
//     after a silent drop or a short server path), the click goes
//     out again every kiteReclickPeriod (250ms - the tick cadence,
//     the owner ask: "it should spam clicks much faster"); a
//     re-click inside the broadcast gate just re-aims the same
//     endpoint server-side.
//   - the RACE legs of round 17 run many times the ordinary
//     step's walk time, so the ladder now lives for the WHOLE LEG:
//     the walk adopting refreshes the episode budget (a mid-leg
//     drop later gets its own re-clicks), a stand within
//     kiteArrivalEpsilon of the endpoint completes the leg, and a
//     stand anywhere short of it re-bases the ladder onto the
//     current cell and re-clicks the endpoint - the recovery the
//     window-sized ladder never had (a silent drop mid-leg used to
//     stand the character through the rest of the window while
//     the chaser collected the difference).
//   - the moment the refusal evidence lands (kiteClickRefused - the
//     ActionFailed answer the server gave the last click), the dead
//     endpoint rotates onto the next fan candidate AT ONCE: the
//     rotation no longer waits for the silent probe, the refused
//     cell joins the refused set and the battery skips the whole
//     set (a refused cell never starts a walk, re-clicking it
//     changes nothing).
//   - the silent probe stays as the fallback for the H-006 silent
//     drop (no ActionFailed at all): a walk unanswered past
//     kiteProbeElapsed with the character still on the episode's
//     issue cell names the endpoint dead and rotates the same way.
//   - an encircled train or a lane battery with nothing left stands
//     the ladder down (the cornered hold owns the answer), the
//     window ending stands it down (the pursuit hold or the forced
//     attack re-request owns the tick).
//
// Reports whether this tick spent a re-click so the caller yields
// the tick to the walk.

func (l *Loop) kiteReclickWalk(now time.Time) bool {
    if l.kiteWalkUntil.IsZero() {
        return false
    }
    if now.After(l.kiteWalkUntil) {
        // The window ended: the pursuit hold asks first whether the
        // fight's distance still collapses under the re-shot floor
        // (the race leg outlasts its per-unit window, so this is
        // the moment the walk really ends) - the continuation re-arms
        // the retreat, the ordinary stand-down hands the tick to
        // the forced attack re-request (the affordable shot).
        if selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition(); selfOK &&
            l.kitePursuitHold(now, selfX, selfY, selfZ) {
            return true
        }
        // The re-engage machinery owns the next
        // ticks (the forced attack re-request restarts the shooting
        // from the opened distance), and a re-click past it would
        // only fight the re-engage for the movement.
        l.kiteWalkClear()

        return false
    }
    if l.tracker.SelfWalking() {
        // The walk runs: the click landed. The episode budget
        // refreshes (a mid-leg drop later gets its own re-clicks),
        // the leg state stays armed for the whole window - the
        // round-17 race legs need the ladder alive past the first
        // adoption.
        l.kiteReclicks = 0

        return false
    }
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !selfOK {
        return false
    }
    if math.Hypot(
        float64(selfX-l.kiteWalkX),
        float64(selfY-l.kiteWalkY)) <= kiteArrivalEpsilon {
        // The leg completed: the character stands on (or within a
        // cell's throw of) the endpoint. The ladder stands down,
        // the window's remainder belongs to the arrival shot's own
        // cycle (the shot-paced rhythm re-arms the next leg).
        l.kiteWalkClear()

        return false
    }
    if selfX != l.kiteWalkBaseX || selfY != l.kiteWalkBaseY {
        // A fresh standing episode: the character left the last
        // issue cell (a mid-leg silent drop, a short server path)
        // and stands short of the endpoint - the ladder re-bases
        // onto the current cell and keeps pushing the SAME endpoint
        // (the rotation machinery owns the dead cells), the episode
        // budget and the probe window re-arm whole.
        l.kiteWalkBaseX, l.kiteWalkBaseY = selfX, selfY
        l.kiteReclicks = 0
        l.kiteWalkIssuedAt = now
        l.kiteWalkDead = false
    }
    if l.kiteReclicks >= kiteReclickLimit {
        return false
    }
    // The refusal evidence outranks the pacing: a click the server
    // answered ActionFailed never starts a walk, waiting only burns
    // the movement window.
    refused := l.kiteClickRefused()
    if !refused && now.Sub(l.kiteReclickAt) < kiteReclickPeriod {
        return false
    }
    l.kiteReclicks++
    l.kiteReclickAt = now
    if !l.kiteReclickVerdict(now, selfX, selfY, selfZ, refused) {
        return false
    }
    if err := l.game.WalkTo(
        l.kiteWalkX, l.kiteWalkY, l.kiteWalkZ); err != nil {
        l.logger.Printf("Hunt: kite re-click failed: %v", err)

        return false
    }

    return true
}

// kiteReclickVerdict latches the dead-endpoint verdict of the ladder
// and runs the rotation it demands. The evidence verdict comes first
// (the server answered the last click with ActionFailed - the probe
// line lands once per walk so the dump names the mechanism), the
// silent probe second (the walk stayed unanswered past the probe age
// with the character still on the issue cell - the H-006 silent drop
// signature). Either verdict arms the rotation: the refused set skips
// every cell already named dead, the fresh train direction included -
// the chasers moved while the character stood. Reports whether the
// re-click may still go out: a rotation with nothing left (encircled,
// every lane refused or blocked) stands the whole ladder down - the
// cornered hold owns the answer, the next shot cycle re-arms the
// retreat fresh.

func (l *Loop) kiteReclickVerdict(
    now time.Time, selfX, selfY, selfZ int32, refused bool,
) bool {
    if refused && !l.kiteWalkDead {
        // The evidence verdict: the server answered the click with
        // ActionFailed inside the round trip. The probe line lands
        // once per walk so the dump names the mechanism; the
        // ROTATION itself waits for the probe age below - a refusal
        // answer of the server AI's own attack retries (the bow
        // disable path schedules them a second apart, each retry on
        // a still-disabled bow bounces) rides the same packet and
        // only the age gate separates it from the movement
        // broadcast that may still be in flight for a healthy
        // click.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "was refused by the server (standing on %d %d), "+
            "rotating the retreat lane at the probe age",
            l.target, selfX, selfY)
    } else if !refused && !l.kiteWalkDead &&
        now.Sub(l.kiteWalkIssuedAt) >= kiteProbeElapsed {
        // The silent probe: the initial click, the plain re-clicks
        // and the character still stands on the issue cell past the
        // broadcast gate plus the pacing headroom - the movement
        // never started and no refusal arrived. One line per walk,
        // the rotation follows.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "never started the movement (%d clicks out, standing "+
            "on %d %d), rotating the retreat lane",
            l.target, l.kiteReclicks+1, selfX, selfY)
    }
    if !l.kiteWalkDead ||
        now.Sub(l.kiteWalkIssuedAt) < kiteProbeElapsed {
        // No verdict yet, or the walk is still younger than the
        // broadcast gate: the re-click keeps hitting the endpoint -
        // the spam the owner asked for, and the movement broadcast
        // of a healthy click still gets its chance to clear the
        // ladder before any rotation spends the fan battery.
        return true
    }
    if !l.kiteRotateDeadEndpoint(selfX, selfY, selfZ) {
        // The rotation found nothing: every candidate of the away
        // hemisphere is refused or blocked - the cornered answer
        // owns the cycle, the next shot re-arms the retreat fresh
        // (the dead-cell memory persists per target, the next
        // cycle starts on what the battery has left).
        l.logger.Printf("Hunt: no walkable retreat lane left on "+
            "target %d (every candidate refused or silent), "+
            "holding ground", l.target)
        l.kiteWalkClear()

        return false
    }

    return true
}

// kiteDeadCellsFor returns the dead-cell memory of the CURRENT
// fight: the cells the server refused (or that stayed silent
// through the probe) stay skipped by the initial lane resolution of
// every later cycle of the same target - the terrain does not
// change between the shots, and a cycle that starts on a lane that
// already proved walkable never pays the probe tax on the same dead
// straight cell again. A memory of another target (or an empty one)
// answers nil - the plain resolution.

func (l *Loop) kiteDeadCellsFor() [][2]int32 {
    if l.kiteDeadFor != l.target || l.kiteWalkDeadCount == 0 {
        return nil
    }

    return l.kiteWalkDeadCells[:l.kiteWalkDeadCount]
}

// kiteWalkClear stands the re-click ladder down: the endpoint fields
// stay (they name the walk of record for the diagnostics), the gate
// (kiteWalkUntil), the issue stamp, the click count and the probe
// latch reset - the next kite step arms the ladder whole through
// kiteIssueWalk. The dead-cell memory STAYS: it belongs to the
// target, not the walk, and the next cycle of the same fight skips
// the cells the server already refused.

func (l *Loop) kiteWalkClear() {
    l.kiteWalkUntil = time.Time{}
    l.kiteWalkIssuedAt = time.Time{}
    l.kiteReclicks = 0
    l.kiteWalkDead = false
}

// kiteChaserBearings appends the planar bearing (self to chaser,
// the atan2 convention) of every chaser the retreat direction
// weighs to out and returns the filled slice: the fight target
// first, then every SelfAttackers member within the scan range on a
// reachable deck. The enumeration is the single source of the train
// geometry - the centroid sum of kiteTrainDirection and the gap
// search of kiteGapDirection read the same members, so the two
// answers can never disagree about the train's shape. The output
// caps at the capacity of out (the kiteBearings scratch): a wider
// train is the flee machinery's case, not a kite shape.

const (
    kiteBreakoutOpen = iota
    kiteBreakoutCrowded
    kiteBreakoutWalled
)

// kiteBreakoutCandidate evaluates one breakout ray under the
// strict contract both ladders share: the flank clearance off every
// chaser bearing (the raw candidate first, then the deflected lane
// the terrain battery may return - the deflection can bend a clear
// candidate back onto a chaser's ray), the full terrain battery
// (the camp deflection, the leash, the geodata wall, the water) and
// the dead-cell memory. The half-plane guard of the normal fan is
// deliberately absent: the breakout exists to admit lanes the
// hemisphere sweep forbids by design.

func (l *Loop) kiteHoldGround(now time.Time, surrounded bool,
    breakout string,
) {
    fresh := l.kiteHeldFor != l.target ||
        l.kiteHeldAt.IsZero() ||
        now.Sub(l.kiteHeldAt) >= kiteHoldLogPeriod
    if fresh {
        suffix := ""
        if breakout != "" {
            suffix = " - the pocket breakout: " + breakout
        }
        if surrounded {
            l.logger.Printf("Hunt: the chasers surround the bow "+
                "fight on target %d, holding ground and shooting "+
                "through the train%s", l.target, suffix)
        } else {
            l.logger.Printf("Hunt: no walkable retreat lane on "+
                "target %d (cornered), holding ground and shooting "+
                "the bow%s", l.target, suffix)
        }
    }
    l.kiteHeldFor = l.target
    l.kiteHeldAt = now
}

// rotatePlanar rotates a planar unit direction by the given angle
// (counterclockwise, the mathematical sign convention).

func rotatePlanar(x, y, angle float64) (float64, float64) {
    cos, sin := math.Cos(angle), math.Sin(angle)

    return x*cos - y*sin, x*sin + y*cos
}
