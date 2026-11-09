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
//     too wide to outrun: the widest-gap bisector keeps the step
//     moving until the geometry leaves no walkable straight ray, and
//     the archer holds ground and shoots through it then.
//   - the corner: when the straight retreat lane is blocked - the
//     fight leash, a wall behind (the geodata line of sight), water
//     behind (the OverWater oracle) - the archer stops retreating and
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
//   - the straight retreat (the owner rule of the 2026-11-09 round):
//     the step runs the straight away-ray and NOTHING else. The old
//     fan (the 45/90 degree candidates), the camp deflection, the
//     circling chord and the pocket breakouts all TURNED the
//     character, and a turn spends the distance the retreat just
//     bought - exactly the waste the owner measured on the fights
//     whose speeds sit too close. A blocked straight lane holds the
//     ground and shoots; it never bends the retreat.
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
//     surfaces through the re-click ladder and then the hold: the
//     straight endpoint is the only retreat this layer takes, so a
//     dead click holds the ground and the bow keeps shooting.
//   - the streak limit of the core is a DIAGNOSTIC since the
//     round-14 always-run redesign: a chaser at least as fast as
//     the character never falls behind, the counter names the
//     unwinnable race in the log, and the retreat keeps running
//     straight - the standing "fight it out" answer is gone (it was
//     the melee damage the kite exists to avoid).
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
// goes out again at the 250ms pace - toward exactly the same
// straight endpoint, because inside the once-per-second broadcast
// gate a healthy accepted click may legitimately not have answered
// yet. The three candidate mechanisms the ladder answers: the
// stance transition swallow (the click lands while the server side
// ATTACK intention of the just-broadcast shot is still tearing
// down), the destination-cell refusal (the server refuses the cell
// the bot side LineOfSight blessed) and the H-006 silent drop (the
// deployment that swallows accepted move requests). The probe line
// names which mechanism ate the clicks of a given fight; the
// straight design has no second lane to rotate onto, so a probe that
// stays dead past the age gate holds the ground and lets the bow
// shoot.
//
// The behavior arms itself on the weapon in hand (bowEquipped): no
// config, no class check - whatever bot ends up holding a bow (the
// planned archer type of the config launch, a lured guard round)
// kites its fights. The enemy weapon gate (the owner rule of the
// 2026-11-09 round) narrows it: the layer never arms against an
// archer fight target. The NpcInfo equipment block names the enemy
// template rhand item, the state resolves the weapon family
// (state.Bot.ObjectUsesBow) and a bow-wielding enemy closes every
// kite layer - a ranged opponent cannot be outrun, the retreat only
// spends the bow cooldown on walking while the enemy keeps shooting,
// so the standing archer trade answers it (the same hold rule the
// cornered fight follows).

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
    // while the retreat keeps running straight.
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
    // kiteHalfPlaneSlack is the tolerance of the away half-plane
    // gate: a lane may run perpendicular to the away-ray, but never
    // fold back toward the train - the guard rejects the endpoints
    // whose retreat component turned negative beyond the float
    // noise.
    kiteHalfPlaneSlack = -0.05
    // kiteReshotFloor is the re-shot gate of the retreat step: the
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
    // fight stands; under it the straight leg buys the deficit back
    // (the server keeps the auto-attack displaced while the character
    // moves, and a 110 chaser cannot hit what it cannot catch).
    kiteReshotFloor = 480.0
    // kiteRoamRadius bounds how far the fight's retreats may drift
    // from the FIGHT ANCHOR - the position of the fight target at
    // the fight's first retreat resolution (see kiteResolveAndClick).
    // The anchor is the live-observed farm point of the fight: the
    // straight legs keep the fight on its own ground wherever it
    // started, and the radius fences the long race legs. This
    // replaces the old hunting-square leash (the zone.Contains read
    // of the generated elven/dion tables): the kite must stay
    // LOCATION-GENERAL - it works on any map, any ground, with or
    // without a zone registry, and the target economy alone owns the
    // tables. Three bow engage radii leave the healthy band well
    // inside while a dragged chase still has room to maneuver.
    kiteRoamRadius = 3 * userBowEngageRadius
    // kiteMinStep is the shortest retreat step the ring budget may
    // leave: a stand already at the ring still takes a short straight
    // hop, and the lane gate refuses it when even that is walled.
    kiteMinStep = 250.0
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
    // misattribute that cadence to the kite clicks and latch
    // healthy endpoints dead (the local acceptance round caught
    // exactly that cascade before the tightening).
    kiteRefusalAnswerWindow = 250 * time.Millisecond
    // kiteBowRetryLead is the look-back band of the bow retry: an
    // attack request the server refused during the bow disable
    // schedules notifyActionReadyToAct one second ahead
    // (Creature.canUseRangeWeapon), and the retry's ActionFailed
    // lands with no client request inside the tight answer window -
    // the 14:06:50 dump of the round-19 run shows the chain: the
    // attack request at 49.013, the refusal at 50.263 (1.25 s, the
    // schedule plus the tick granularity). An action request inside
    // the band owns that failure, never the retreat click.
    kiteBowRetryLead = 1350 * time.Millisecond
    // kiteBowReuseDelay is the reuse delay of the bow family the
    // fleet shoots (the C1 item data: the Short Bow and the D-grade
    // bows carry 1500, dist/game/data/stats/items - the value feeds
    // the bow disable window formula of kiteWalkWindow).
    kiteBowReuseDelay = 1500.0
    // kiteWalkWindowFloor/Max clamp the bow-aware walk window: the
    // formula reads the live pAtkSpd, and a broken broadcast (a
    // zero, a partial) must never collapse or explode the window -
    // the clamp keeps the walk inside the sane attack-speed band
    // and falls back to the shipped kiteStepWindow outside it.
    kiteWalkWindowFloor = 1 * time.Second
    kiteWalkWindowMax   = 5 * time.Second
    // kiteGaugeMaxAge bounds how long a RED SetupGauge still counts
    // as the bow cycle of the shot in flight: the server sends the
    // gauge on every shot commit, so a live cycle always has a fresh
    // one; a gauge older than this belongs to a fight that ended and
    // the formula fallback owns the window.
    kiteGaugeMaxAge = 10 * time.Second
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
    // kiteRaceStepMax caps the race leg: the roam radius (three bow
    // engage radii) bounds the ground the fight keeps, so a leg past
    // ~1700 would leave the anchor fence before the walk ends - a
    // deficit that deep belongs to the next shot cycle, not one
    // walk.
    kiteRaceStepMax = 1700.0
    // kiteRingLeashShare caps the straight leg as a share of the
    // roam radius (kiteStraightLeg lands it on the ring): the ring
    // is the band the fight keeps, the fence is the emergency margin
    // between the ring and the roam radius, and an endpoint in
    // between would leave the next leg no room to run without
    // crossing the leash. The round-17 live verdict sized the share
    // at 0.72 (972 units): the audit drift leash is 1500 from the
    // anchor, and the ring at 972 keeps the whole band inside it.
    kiteRingLeashShare = 0.72
    // kiteArrivalEpsilon is the arrival radius of the leg-long
    // re-click ladder (round 17): a stand within this distance of
    // the walk endpoint completes the leg (the server's cell
    // granularity and a partial route's last walkable cell never
    // leave the character exactly ON the clicked cell), while a
    // stand beyond it - mid-route, or short of the endpoint after a
    // partial path - re-arms the episode and keeps pushing the
    // endpoint.
    kiteArrivalEpsilon = 100.0
    // kitePursuitContext bounds the freshness of the pursuit
    // context (kitePursuitFor/kitePursuitAt): the continuation
    // chain re-stamps every walk, so anything older than this
    // belongs to another fight - the killed mob respawns under the
    // SAME object id on the Mobius ground (Spawn.respawnNpc keeps
    // the id), and a stale context must never answer the approach
    // of the respawned mob with a retreat. The bound sits under the
    // fastest respawn of the elven ground (RespawnMin 15 s) while
    // surviving a live chain that pauses through a cornered hold
    // episode.
    kitePursuitContext = 12 * time.Second
    // kitePursuitStreakLimit is the runaway DIAGNOSTIC threshold of
    // the pursuit chain: the counter names the never-resolving chain
    // in the log while the chain keeps running. The stall exit
    // (kitePursuitDeadlockSteps) owns the stop of a chain whose
    // windows stop paying, and a chain that keeps OPENING distance
    // still runs out its race (the fight-anchor leash bounds the
    // drift, the mob's own home leash ends a truly unwinnable chase,
    // the affordable re-shot ends a winnable one).
    kitePursuitStreakLimit = 12
    // kitePursuitStallLimit is the unwinnable-race threshold: a
    // chaser at least as fast as the character never falls behind
    // (the walk ends where it started, distance-wise), and two
    // consecutive windows without a gain name the parity race in the
    // ledger. Two stalled windows alone keep the chain walking (the
    // moving chase trades the melee blows for chase-cadence blows at
    // worst), but a chain that ALSO spent its walk budget on a
    // walled pocket is not racing anything: the stall exit stands
    // the walk down and the bow fires from the hold.
    kitePursuitStallLimit = 2
    // kitePursuitDeadlockSteps is the walk budget of the stall exit,
    // counted whole (the opening walk of the shot cycle plus every
    // continuation). A chain that spent it and still sees stalled
    // windows holds the ground and shoots: the cornered archetype
    // answer reaches the bow one bow cycle after the race stopped
    // paying, instead of walking a walled pocket for a minute. A
    // chain that OPENS distance never reaches the exit - the ledger
    // clears on every gain - so the healthy race still runs its full
    // course to the re-shot floor.
    kitePursuitDeadlockSteps = 4
    // kitePursuitStallSlack is the progress bar of one pursuit
    // window: a walk must open at least this much distance on the
    // nearest threat to count as progress. The honest elven-ground
    // race gains ~45 units a window (the 125-vs-110 speed margin
    // over the ~3 s walk window); the slack sits well under it so
    // the winning cells never stall, while a parity crawl slower
    // than half the honest gain reads as the stall it is.
    kitePursuitStallSlack = 20.0
)

// KiteParams is the tunable block of the kite fight (owner issue
// #29): the four named knobs the live tuning round adjusts from the
// measured numbers, plus the enable gate. The launch config carries
// one per bot spec (the "kite" section of launch_config.go), so the
// standing-archer baseline (a bow bot with the kite disabled) and the
// tuned profile switch by editing the file instead of a code edit and
// a rebuild between the measurement runs. Every remaining kite
// constant (the streak limit, the train scan, the encirclement
// share) stays a package constant on purpose: the tuning round names
// these four, the rest ride the shipped values until the numbers ask
// otherwise.

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
    // The enemy weapon gate (the owner rule): against an archer the
    // kite never engages. A ranged enemy cannot be outrun - the
    // retreat spends the bot's bow cooldown on walking while the
    // enemy keeps shooting - so the standing archer trade is the
    // answer, exactly like the cornered hold. The fight still runs
    // through the ordinary attack machinery; only the retreat is off.
    if l.tracker.ObjectUsesBow(l.target) {
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

// kiteOwnsRetreat reports whether the archer kite owns the aggro
// answer of the current fight: the kite layer is enabled, the
// character holds a bow, a fight is selected and the enemy is not
// itself an archer (a ranged enemy cannot be outrun - the same
// weapon gate kiteLayerGates enforces). While it holds, the melee
// escape machinery must stand down: its flee fan is clamped to the
// CURRENT hunting zone, and the 2026-11-09 acceptance run measured
// what that clamp does on a kite fight that drifted across a
// cell boundary - every lane zeroed, the pile-up and losing-fight
// answers dropped the fight and logged the session out mid-kite
// (the Green Dryad cell center 37500,51095 against the character on
// the Kaboo ground at 36056,50360 in the 18:33 dump). The kite's own
// straight lane battery validates the real world instead of a zone
// box, and the critical-HP logout (< 12%) stays armed above both.

func (l *Loop) kiteOwnsRetreat() bool {
    return l.kite.Enabled && l.target != 0 && l.bowEquipped() &&
        !l.tracker.ObjectUsesBow(l.target)
}

// kiteFightLive reports whether a fight is actually live around the
// character: the selected target is engaged by either side (the own
// fresh swing or chase step, or the target holding the character as
// its victim - SelfEngagedBy), or another mob already holds the
// character as its target (a train member grinding the retreat).
// The kite triggers read this instead of the strict SelfFighting
// view: a long retreat leg spends seconds walking without a swing,
// and the strict view went stale exactly then, standing the whole
// kite layer down mid-cycle (the 2026-11-09 live run). An ordinary
// approach to a peaceful pick is not a live fight and never fires
// the retreat.

func (l *Loop) kiteFightLive() bool {
    if l.target == 0 {
        return false
    }
    if l.tracker.SelfEngagedBy(l.target) {
        return true
    }

    return l.tracker.SelfAttackerCount() > 0
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
            "running it out straight (step %d)",
            l.target, l.kiteStreak)
    }

    return true
}

// kiteFromTarget steps a bow fighting character away from the
// hostile that armed the step (the target or a train member, see
// kiteThreat): the archer buys back the weapon range instead of
// tanking the melee. The step DIRECTION is the straight away-ray of
// the whole chaser train (the centroid of the target plus every
// attacker in range, the widest-gap bisector of an encircled one -
// see kiteTrainDirection), and it is the ONLY lane: the owner rule
// of the 2026-11-09 round removed the fan, the camp deflection and
// the circling chord, because a turn spends the distance the
// retreat just bought. A train that surrounds the character or a
// blocked straight lane resolves to the cornered hold: the archer
// stops retreating and keeps shooting the bow at melee range - never
// a weapon switch (the archetype rule of issue #21). The step
// respects the movement window contract of the fight steps (the
// forced attack re-requests wait out the walk), and the streak limit
// (an unwinnable distance race keeps running straight - a
// diagnostic only). A fresh target resets the streak. The step
// timing honors the live shot cycle (kiteShotPhase): inside the bow
// windup the click defers to the windup end (the server would save
// an immediate one and replay it at the disable end where the
// re-shot cancels it), inside the accepted move window the click
// goes out at once with the cycle end as its window, and with no
// live cycle the ordinary full window applies. Reports whether the
// tick issued or armed the step (the caller skips the rest of the
// fighting branch then - the walk owns the movement).

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
        until = now.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
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
// strafe rides the same straight retreat the rhythm always runs
// (the distance holds near the bow's own max), capped at the train
// scan range - beyond it neither side reaches the other and the
// rhythm has nothing to pace.

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
// mob that keeps chasing the fight), the straight lane machinery
// (the train direction, the leash, the wall and the water gates,
// the cornered hold) resolves at the click moment - the chasers move
// while the character stands the windup out - and the rhythm does
// NOT count toward the streak limit: the shots never starve. A
// hostile beyond the band leaves the standing fight (the stall
// watchdog owns the re-approach). Reports whether the tick armed the
// deferred retreat.

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
    l.kitePursuitCycle(shotAt)
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

// kitePursuitCycle opens the pursuit ledger of a fresh shot cycle:
// every own Attack broadcast starts its own distance race - the
// re-shot is released only after the retreat regained the re-shot
// floor (or the race spent its walk budget) - so the steps, the
// stalls and the open walk of the previous cycle never inherit into
// the new one. The stamp makes the reset idempotent across the ticks
// that still catch the same broadcast (the shot trigger runs for the
// whole freshness window): only a NEW shot rewrites the ledger.

func (l *Loop) kitePursuitCycle(shotAt time.Time) {
    if shotAt.IsZero() || shotAt.Equal(l.kitePursuitShot) {
        return
    }
    l.kitePursuitShot = shotAt
    l.kitePursuitSteps = 0
    l.kitePursuitDist = 0
    l.kitePursuitStall = 0
    l.kitePursuitWalkOpen = false
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
// fired from the safe distance. The FORMULA carries no leash: the
// straight leg budgets against the ring afterwards (see
// kiteStraightLeg) - the round-18 QA audit measured the old
// self-anchored budget collapsing the leg to the 250 floor at the
// fence, the shuffle that never regained the floor.

func (l *Loop) kiteStepLength(dist float64) float64 {
    if dist >= kiteReshotFloor {
        return l.kite.Step
    }
    step := l.kite.Step + (kiteReshotFloor-dist)*kiteRaceFactor
    if step > kiteRaceStepMax {
        step = kiteRaceStepMax
    }

    return step
}

// kiteStraightLeg budgets a STRAIGHT leg against the opening ring:
// the endpoint of the away-ray must stay inside the fight's ring -
// the leash share of the roam radius around the fight anchor (the
// target's stand, see kiteResolveAndClick) - so the retreat stays on
// the ground the fight keeps. A stand already at the ring keeps at
// least the short hop of kiteMinStep; the lane gate refuses the hop
// when even that is walled, and the hold answers. The margin
// (kiteArrivalEpsilon's half) keeps the int32 cell rounding of the
// endpoint from crossing the ring the battery enforces - a
// ring-budget step rounded onto the ring plus one reads past the
// leash and the walk is refused. Reports the step unchanged before
// the fight anchored.

func (l *Loop) kiteStraightLeg(step float64, selfX, selfY int32) float64 {
    if l.kiteFightFor != l.target {
        return step
    }
    anchorDist := math.Hypot(
        float64(selfX-l.kiteFightX), float64(selfY-l.kiteFightY))
    ring := kiteRoamRadius * kiteRingLeashShare
    if budget := ring - anchorDist -
        kiteArrivalEpsilon/2; budget < step {
        step = math.Max(kiteMinStep, budget)
    }

    return step
}

// kiteResolveAndClick resolves the straight retreat lane against the
// live train and terrain and issues the retreat click through the
// ladder seam: the encircled train (the away vectors cancel - chasers
// stand on every side) and a blocked straight lane answer with the
// hold ground rule of the archetype (never a weapon switch - the bow
// is the always-weapon, and the owner rule of the 2026-11-09 round
// removed every turning answer), a walkable lane clicks at once with
// the movement window the caller computed. The shared seam serves the
// deferred click of the shot cycle (kiteClickWalk), the accepted
// window catch of the broadcast tick and the ordinary proximity step
// - one resolution, one click path, one ladder. The leg LENGTH is
// race-aware (kiteStepLength): the live threat distance at the
// resolution moment sizes the step, so a fight collapsed under the
// re-shot floor banks the whole deficit into one long straight walk
// instead of a chain of window-sized shuffles. Reports whether the
// click went out.

func (l *Loop) kiteResolveAndClick(
    now, until time.Time, selfX, selfY, selfZ int32,
) bool {
    threatID, threatDist, threatOK := l.kiteThreat(selfX, selfY, selfZ)
    if l.kiteFightFor != l.target {
        // A fresh fight: the anchor falls back to the self position
        // until the threat position resolves below.
        l.kiteFightFor = l.target
        l.kiteFightX, l.kiteFightY = selfX, selfY
    }
    if threatOK {
        if tx, ty, _, tok := l.tracker.ObjectPosition(threatID); tok {
            // The live-threat re-anchor (the 2026-11-09 owner round):
            // the threat MOVES while the fight runs, and a frozen
            // anchor turns the roam ring into a fence behind the
            // retreat. The live 17:43 dump decoded it: the shot stood
            // 111 units from the mob, the clamped 250-unit endpoint
            // read ~1404 from the fight's first stand, past the 1350
            // roam radius, and the "cornered" hold answered a runway
            // the geodata probe found open at every distance - the
            // second failure at 17:44 repeated it. Re-reading the
            // threat on EVERY resolution centers the ring on the
            // fight it belongs to; a chaser that follows keeps the
            // ring around itself, so the fence only holds back a
            // retreat from a threat that stands its ground. The
            // anchor follows the NEAREST threat (kiteThreat: the
            // fight target or the nearest attacker), not just the
            // selected target: a train member grinding the character
            // at point blank with the target standing 873 units out
            // (the 18:32 live run) clamped the step to the 250 floor
            // around the far target - the ring must center on the
            // danger that actually arms the step. A vanished threat
            // keeps the last anchor (the fight is over or the
            // broadcast lags).
            l.kiteFightX, l.kiteFightY = tx, ty
        }
    }
    dirX, dirY, encircled := l.kiteTrainDirection(selfX, selfY, selfZ)
    if encircled && dirX == 0 && dirY == 0 {
        // The degenerate no-chaser scene (a tracker gap inside the
        // broadcast window): no ray to guard, the hold keeps the
        // pacing until the next probe.
        l.kiteHoldGround(now, true)

        return false
    }
    // The straight retreat (the owner rule of the 2026-11-09 round):
    // the step runs exactly the away-ray of the train - the centroid
    // of a normal train, the widest-gap bisector of an encircled one.
    // There is no fan, no deflection and no chord: a turn spends the
    // distance the retreat just bought, and the fights whose speeds
    // sit too close cannot afford it. The raw ray stays both the
    // preferred lane and the half-plane reference of the battery.
    // The race-aware leg length (kiteStepLength): the deficit to the
    // re-shot floor at the resolution moment sizes the leg - one long
    // straight walk to the floor, not a window-sized shuffle that
    // ends short of it.
    stepLen := l.kite.Step
    if threatOK {
        stepLen = l.kiteStepLength(threatDist)
    }
    laneLen := l.kiteStraightLeg(stepLen, selfX, selfY)
    stepX, stepY, found := l.kiteRetreatLane(
        selfX, selfY, selfZ, dirX, dirY, dirX, dirY, laneLen)
    if !found {
        // The straight lane is blocked: the hold ground answer is
        // the archetype rule - the bow is the always-weapon, the
        // cornered archer never switches to a melee trade and never
        // turns the retreat, it stands and shoots the way out.
        l.kiteHoldGround(now, encircled)

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
// and the straight lane itself (the chasers moved while the character
// stood - the train direction resolves fresh at the click moment).
// The encircled and the cornered answers hold the ground as always.
// Reports whether the tick spent the click.

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
// own schedule whole. The re-click ladder keeps its own lifecycle
// (the walk it belongs to).

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
    until = shotAt.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
    if now.After(until) {
        // The disable lapsed: the re-engage owns the moment, and a
        // retreat riding the dead cycle would hold its window for
        // nothing.
        return time.Time{}, time.Time{}, false
    }
    clickAt = shotAt.Add(l.kiteWindupWindow(now) + kiteWindupLead)

    return clickAt, until, true
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
    // kiteStep), so the straight leg runs to its endpoint before
    // anything else owns the movement.
    if leg := math.Hypot(
        float64(stepX-selfX), float64(stepY-selfY)); leg > l.kite.Step {
        scaled := now.Add(time.Duration(
            leg / l.kite.Step * float64(l.kiteWalkWindow(now))))
        if scaled.After(until) {
            until = scaled
        }
    }
    l.kiteWalkUntil = until
    l.combatAvoidUntil = until
    l.kiteReclickAt = now
    l.kiteReclicks = 0
    l.kiteWalkDead = false
    if l.kitePursuitFor != l.target ||
        now.Sub(l.kitePursuitAt) > kitePursuitContext {
        // The walk starts a fresh pursuit chain: a new fight, or
        // the previous chain's context expired before this walk
        // (the respawned same-id target of the Mobius ground). The
        // pursuit ledger must not carry the old race's stalls into
        // the new one (the shot-cycle reset of kitePursuitCycle
        // covers the re-shot path; this seam covers the
        // proximity-first fights and the post-respawn walks that
        // re-arm the context without a shot in between).
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

func (l *Loop) kiteWindupWindow(now time.Time) time.Duration {
    return l.kiteWalkWindow(now) / 2
}

// kiteGaugeCycle returns the bow disable window the server announced
// with its own RED SetupGauge, while the gauge is fresh enough to
// belong to the shot cycle in flight: the exact timing the formula
// only approximates. The ATK_SPD attribute is not rebroadcast after a
// relogin until the gear changes, so the formula fell back to its 2 s
// default while the server ran the real 1483 ms windup - the 18:33:13
// retreat click landed 233 ms inside the windup and the server
// answered ActionFailed (docs/kite_timing_findings.md). The gauge
// carries no such gap: it arrives with every shot.

func (l *Loop) kiteGaugeCycle(now time.Time) (time.Duration, bool) {
    at, cycle := l.tracker.SelfBowCycle()
    if at.IsZero() || cycle <= 0 || now.Before(at) ||
        now.Sub(at) > kiteGaugeMaxAge {
        return 0, false
    }
    if cycle < kiteWalkWindowFloor || cycle > kiteWalkWindowMax {
        return 0, false
    }

    return cycle, true
}

// kiteWalkWindow resolves the movement window the kite walk owns:
// the bow disable window the C1 server enforces between the shots -
// timeAtk + reuse = 500000/pAtkSpd + reuseDelay*333/pAtkSpd
// (Creature.calculateTimeBetweenAttacks and calculateReuseTime, the
// reuseDelay of the bow family in the item data). The server's own
// RED SetupGauge (SetupGauge(RED, sAtk + reuse) at the shot commit,
// see Creature.doAttackHitByBow) outranks the formula whenever it is
// fresh: it is the exact window the server runs, whatever the
// client's pAtkSpd view says. The formula stays the fallback for a
// cycle without a parsed gauge, read from the live pAtkSpd the
// StatusUpdate broadcasts (attr 0x12, the value the acceptance dump
// shows as 337 for the Short Bow kit, giving roughly (500000 +
// 1500*333)/337 = 2.97s - the "3 seconds to draw shot" the owner
// measured by hand). The walk then spends the WHOLE cooldown
// running (the owner ask of the third round: the proper archering
// delays from the C1 Mobius formulas - if it is not shooting it
// should be moving away from the target) instead of the shipped
// fixed 2s that stood the character idle for the last second of
// every reload. A missing or absurd pAtkSpd broadcast falls back to
// the shipped kiteStepWindow (the walk contract never depends on
// the packet being parsed).

func (l *Loop) kiteWalkWindow(now time.Time) time.Duration {
    if cycle, ok := l.kiteGaugeCycle(now); ok {
        return cycle
    }
    spd := l.tracker.SelfPAtkSpd()
    if spd <= 0 {
        return kiteStepWindow
    }
    ms := (500000.0 + kiteBowReuseDelay*333.0) * float64(time.Millisecond) /
        float64(spd)
    if ms < float64(kiteWalkWindowFloor) || ms > float64(kiteWalkWindowMax) {
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

    // The failure may carry the SAME coarse clock stamp as the click
    // (the Windows timer granularity - the live server answers inside
    // the same millisecond), so the correlation starts at the click
    // moment inclusive instead of strictly after it.
    if failedAt.Sub(l.kiteReclickAt) > kiteRefusalAnswerWindow {
        return false
    }
    if failedAt.Before(l.kiteReclickAt) {
        return false
    }
    if l.tracker.OtherRequestBetween(l.kiteReclickAt, failedAt) {
        // An action request rides the answer window itself.
        return false
    }

    // The bow retry chain: an attack request the server bounced
    // through the bow disable answers ActionFailed about a second
    // later (Creature.canUseRangeWeapon schedules the retry), far
    // outside the tight window of the click. Attributing that chain
    // to the retreat click latched a dead endpoint on a healthy
    // click (the 14:06:50 dump).
    return !l.tracker.OtherRequestBetween(
        failedAt.Add(-kiteBowRetryLead), l.kiteReclickAt)
}

// kiteReclickWalk is the re-click ladder of the kite retreat (issue
// #60, the rounds two and three; the straight-lane rework of the
// 2026-11-09 round): the single retreat click of the first round
// proved fragile three ways, and the owner's dumps show the
// character standing through the whole bow reload while the mob
// closed. The third-round acceptance dump named the dominant
// mechanism exactly: the server answers SOME destination cells with
// an instant bare ActionFailed (the MoveToLocation handler refusal
// family - the isCompletelyBlocked destination check among others)
// while a lane a few cells aside walks fine, and the bot's own
// geodata reads the refused cells open, so only the ONLINE evidence
// settles it. The ladder answers with the manual behavior the owner
// demonstrated ("clicking behind the character runs 2 seconds no
// problem"), faster now per the third-round ask:
//
//   - while the movement window runs and the character stands
//     (wherever the leg finds it - the issue cell, or mid-route
//     after a silent drop or a short server path), the click goes
//     out again every kiteReclickPeriod (250ms - the tick cadence,
//     the owner ask: "it should spam clicks much faster"); a
//     re-click inside the broadcast gate just re-aims the same
//     endpoint server-side.
//   - the RACE legs run many times the ordinary step's walk time,
//     so the ladder lives for the WHOLE LEG: the walk adopting
//     refreshes the episode budget (a mid-leg drop later gets its
//     own re-clicks), a stand within kiteArrivalEpsilon of the
//     endpoint completes the leg, and a stand anywhere short of it
//     re-bases the ladder onto the current cell and re-clicks the
//     endpoint.
//   - the retreat has ONE lane now (the owner rule of the 2026-11-09
//     round): a refused or silent-probed endpoint has no fan to
//     rotate onto. The ladder keeps the spam while the walk is
//     younger than the broadcast gate, then latches the dead
//     verdict, stands itself down and holds the ground for the bow
//     (kiteHoldGround) - turning the retreat would spend the
//     distance it just bought.
//   - the window ending stands the ladder down; the pursuit hold
//     re-arms the retreat while the hostile still holds under the
//     re-shot floor (see kitePursuitHold), and the forced attack
//     re-request owns the tick from there once the distance opened
//     and fires the next shot.
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
        // (the straight race leg outlasts its per-unit window, so
        // this is the moment the walk really ends) - the
        // continuation re-arms the retreat, and the ordinary
        // stand-down hands the tick to the forced attack re-request
        // (the affordable shot).
        selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
        if selfOK && l.kitePursuitHold(now, selfX, selfY, selfZ) {
            return true
        }
        l.kiteWalkClear()

        return false
    }
    if l.tracker.SelfWalking() {
        // The walk runs: the click landed. The episode budget
        // refreshes (a mid-leg drop later gets its own re-clicks),
        // the leg state stays armed for the whole window.
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
        // cell's throw of) the endpoint. The movement gate of the
        // walk ends WITH the walk, and the pursuit continuation
        // re-arms at once: under the re-shot floor the next straight
        // walk issues on THIS tick, so the archer never stands
        // between the legs of the race; at or beyond the floor the
        // stand-down hands the tick to the arrival shot's own cycle.
        l.kiteWalkClear()

        return l.kitePursuitHold(now, selfX, selfY, selfZ)
    }
    if selfX != l.kiteWalkBaseX || selfY != l.kiteWalkBaseY {
        // A fresh standing episode: the character left the last
        // issue cell (a mid-leg silent drop, a short server path)
        // and stands short of the endpoint - the ladder re-bases
        // onto the current cell and keeps pushing the SAME endpoint,
        // the episode budget and the probe window re-arm whole.
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
    if !l.kiteReclickVerdict(now, selfX, selfY, refused) {
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
// and answers it with the hold. The evidence verdict comes first (the
// server answered the last click with ActionFailed - the probe line
// lands once per walk so the dump names the mechanism), the silent
// probe second (the walk stayed unanswered past the probe age with
// the character still on the issue cell - the H-006 silent drop
// signature). Both keep the spam going while the walk is younger
// than the broadcast gate (a healthy click's movement broadcast may
// still be in flight), and past the gate the straight design has no
// second lane: the ladder clears, the hold owns the cycle and the bow
// keeps shooting (the owner rule of the 2026-11-09 round - a turn
// spends the distance the retreat just bought). Reports whether the
// re-click may still go out.

func (l *Loop) kiteReclickVerdict(
    now time.Time, selfX, selfY int32, refused bool,
) bool {
    if refused && !l.kiteWalkDead {
        // The evidence verdict: the server answered the click with
        // ActionFailed inside the round trip. The probe line lands
        // once per walk so the dump names the mechanism; the spam
        // keeps going while the walk is younger than the broadcast
        // gate below - a refusal answer of the server AI's own attack
        // retries (the bow disable path schedules them a second
        // apart, each retry on a still-disabled bow bounces) rides
        // the same packet and only the age gate separates it from
        // the movement broadcast that may still be in flight for a
        // healthy click.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "was refused by the server (standing on %d %d), "+
            "holding ground at the probe age",
            l.target, selfX, selfY)
    } else if !refused && !l.kiteWalkDead &&
        now.Sub(l.kiteWalkIssuedAt) >= kiteProbeElapsed {
        // The silent probe: the initial click, the plain re-clicks
        // and the character still stands on the issue cell past the
        // broadcast gate plus the pacing headroom - the movement
        // never started and no refusal arrived. One line per walk,
        // the hold follows.
        l.kiteWalkDead = true
        l.logger.Printf("Hunt: the kite walk click on target %d "+
            "never started the movement (%d clicks out, standing "+
            "on %d %d), holding ground",
            l.target, l.kiteReclicks+1, selfX, selfY)
    }
    if !l.kiteWalkDead ||
        now.Sub(l.kiteWalkIssuedAt) < kiteProbeElapsed {
        // No verdict yet, or the walk is still younger than the
        // broadcast gate: the re-click keeps hitting the endpoint -
        // the spam the owner asked for, and the movement broadcast
        // of a healthy click still gets its chance to clear the
        // ladder before the hold spends the cycle.
        return true
    }
    // The straight design has no second lane to rotate onto (the
    // owner rule of the 2026-11-09 round): the dead endpoint stands
    // the ladder down and the hold ground answer keeps the bow
    // shooting. The next shot cycle re-arms the retreat fresh.
    l.kiteWalkClear()
    l.kiteHoldGround(now, false)

    return false
}

// kitePursuitHold answers whether a lapsed kite walk re-arms as the
// pursuit continuation of the same fight (issue #70, the owner rule
// of the 2026-11-09 round): the retreat must keep running between
// the shots. A walk window that ends while the nearest hostile still
// holds inside the re-shot floor re-arms one more straight walk
// through the ordinary lane resolution, so the chaser spends the
// reload tail closing the distance again instead of the archer
// standing through it - and the re-shot fires from the regained
// floor, never at the range the windup collapsed. The chain carries
// its OWN budget apart from the proximity streak: the progress
// ledger (kitePursuitSteps/Stall/Dist/WalkOpen) resets on every
// fresh shot cycle (kitePursuitCycle), counts only the COMPLETED
// walks (a cornered hold re-probe never mints a stall - the pocket
// cells survive their holds on exactly that), and stops the chain
// two stalled windows in a row (the unwinnable parity race) or at
// the flat backstop (the oscillating chase that resets the stall
// ledger forever). The first moment the hostile clears the floor -
// or the ledger, the context or the lane battery stops the chase -
// the walk stands down and the re-request fires the affordable shot
// from the opened distance. Reports whether the tick spent the
// continuation.

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
        // own cycle clears the ledger whole (kitePursuitCycle).
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
            // The parity crossing (once per crossing): two windows
            // in a row opened no distance. The walk budget of
            // kitePursuitDeadlockSteps decides below whether the
            // chain is still worth running.
            l.logf("Hunt: the pursuit race on %d is at parity "+
                "(%d stalled windows), checking the walk budget",
                l.target, l.kitePursuitStall)
        }
        l.kitePursuitWalkOpen = false
        l.kitePursuitDist = dist
    }
    if l.kitePursuitStall >= kitePursuitStallLimit &&
        l.kitePursuitSteps+1 >= kitePursuitDeadlockSteps {
        // The stall exit: the chain spent its walk budget and the
        // last windows opened no distance - walking on only repeats
        // the walled pocket shuffle while the chaser stays glued to
        // the bow (the temp18 acceptance dump: 33 walks and 63
        // seconds without a shot in the elven pocket). The hold
        // ground answer of the cornered archetype owns the fight
        // from here: the walk clears with this tick and the forced
        // attack re-request fires the bow. The pursuit context is
        // spent until the next shot cycle re-establishes it, so the
        // same tick's proximity probe cannot re-arm the walk the
        // exit just stood down.
        l.logf("Hunt: the pursuit race on %d is lost after %d "+
            "walks, standing to shoot", l.target,
            l.kitePursuitSteps+1)
        l.kitePursuitFor = 0
        l.kitePursuitSteps = 0
        l.kitePursuitStall = 0
        l.kitePursuitWalkOpen = false
        l.kitePursuitDist = 0

        return false
    }
    if l.kitePursuitSteps == kitePursuitStreakLimit {
        // The diagnostic crossing (once per crossing): the chain
        // never resolved in this many walks yet it still opens
        // distance, so the race keeps running - the stall exit
        // above owns the chain that stopped paying.
        l.logf("Hunt: the pursuit chain on %d ran %d walks "+
            "without a shot, holding the course",
            l.target, l.kitePursuitSteps)
    }
    until := now.Add(l.kiteWalkWindow(now) + l.kite.ReengageDelay)
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

// kiteWalkClear stands the re-click ladder down: the endpoint fields
// stay (they name the walk of record for the diagnostics), the gate
// (kiteWalkUntil), the issue stamp, the click count and the probe
// latch reset - the next kite step arms the ladder whole through
// kiteIssueWalk. The movement hold the walk itself armed
// (combatAvoidUntil equal to the walk window) ends with it: the
// arrival shot needs the gate open on the very tick the leg
// completes (the round-19 live log measured the hold surviving the
// arrival and standing the archer through the window remainder). A
// hold of any other owner - the deferred click of a live shot cycle,
// an impending-add step - keeps its own window.

func (l *Loop) kiteWalkClear() {
    if l.combatAvoidUntil.Equal(l.kiteWalkUntil) {
        l.combatAvoidUntil = time.Time{}
    }
    l.kiteWalkUntil = time.Time{}
    l.kiteWalkIssuedAt = time.Time{}
    l.kiteReclicks = 0
    l.kiteWalkDead = false
}

// kiteHoldGround arms the hold of the kite: the archer stops
// retreating and keeps shooting the bow at melee range. The hold
// answers a blocked straight lane (the owner rule of the 2026-11-09
// round: no turning, no side step - the straight line is the whole
// retreat contract), a refused or silent retreat click, and an
// encircled train with no walkable gap. It paces its own re-probe
// (the kite period) so a failed lane never stutters the tick loop,
// and logs its reason once per hold episode - a standing fight that
// names itself stays diagnosable in the event feed. The surrounded
// flag picks the diagnostic wording.

func (l *Loop) kiteHoldGround(now time.Time, surrounded bool) {
    fresh := l.kiteHeldFor != l.target ||
        l.kiteHeldAt.IsZero() ||
        now.Sub(l.kiteHeldAt) >= kiteHoldLogPeriod
    if fresh {
        if surrounded {
            l.logger.Printf("Hunt: the chasers surround the bow "+
                "fight on target %d, holding ground and shooting "+
                "through the train", l.target)
        } else {
            l.logger.Printf("Hunt: no walkable retreat lane on "+
                "target %d (cornered), holding ground and shooting "+
                "the bow", l.target)
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
