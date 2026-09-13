// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
        "strings"
        "testing"
        "time"

        "github.com/melg8/swarm/internal/swarm/pathfind"
        "github.com/melg8/swarm/internal/swarm/session"
        "github.com/melg8/swarm/internal/swarm/state"
        "github.com/stretchr/testify/require"
)

// The reproductions of the six hour fleet run of 2026-09-13
// (logs/session-20260913-015130-28264.jsonl): every test here pins one
// observed misbehavior and its fix. The run showed four time-wasters -
// the steering tangent flip-flop that burned an hour of the delevel
// walk, the emergency logout loop on the aggressive spider ground (85
// logouts, median session 82 s), the town trip abort loop (47 identical
// "no walkable path" aborts) and the fight clock that accumulated
// across the kills of one recycled object id (a "4369 second" fight).

// spawnAggressiveMob spawns the aggressive Kaboo Orc Fighter template
// (the wire id 1000471) at a point with the given object id.
func spawnAggressiveMob(bot *state.Bot, objectID int32, x int32, y int32) {
        bot.ApplyNpcInfo(state.NpcInfo{
                ObjectID: objectID, TemplateID: 1000471, Attackable: true,
                X: x, Y: y, Z: -3500, Name: "Kaboo Orc Fighter",
        })
}

// TestSteerFlipFlopWaypointIsThreatened pins the geometry of the
// observed ping-pong: a waypoint inside the trigger circle of an idle
// aggressive mob (the threat 338 units from the click target against
// the 600 unit margin) is unreachable by any tangent arc - the
// receding horizon flips the tangent side at every re-issue, so the
// walk oscillates between the two tangent endpoints forever. The
// follower must recognize the situation instead of aiming at it.
func TestSteerFlipFlopWaypointIsThreatened(t *testing.T) {
        bot := avoidSceneBot()
        // The observed scene: the camp mob 338 units from the waypoint the
        // walk aims at (the delevel walk of the dump aimed at a waypoint
        // with the Kaboo Orc Fighter standing 338 units past it).
        avoidCampMob(bot, 45338, 50000)
        loop := NewLoop(&fakeGame{}, bot)

        // The click target sits inside the 600 unit margin circle.
        require.True(t, loop.legTargetThreatened(45676, 50000, 49000, 50000),
                "a click target 338 units from the camp is threatened")
        // A destination-exempt mob never blocks: the ground the walk
        // deliberately enters carries its own mobs.
        require.False(t, loop.legTargetThreatened(45676, 50000, 45338, 50000),
                "the mob at the walk destination is exempt")
        // A click target far outside every circle stays clean.
        require.False(t, loop.legTargetThreatened(47000, 50000, 49000, 50000),
                "a click target 1662 units from the camp is clean")
}

// TestClickWaypointSkipsThreatenedWaypoint pins the follower side: a
// planned route whose next waypoint sits inside a camp circle advances
// the cursor onto the clear successor instead of clicking into the
// circle (the skip reuses the stuck machinery's walkable-line gate).
func TestClickWaypointSkipsThreatenedWaypoint(t *testing.T) {
        loop, game, bot, nav := newTripLoop()
        nav.found = true
        // A route whose second waypoint sits 338 units inside the camp
        // circle: the first waypoint is the standing cell, the second would
        // be the flip-flop target, the third lies past the camp.
        nav.route = []pathfind.Vec3{
                {X: 45000, Y: 50000, Z: -3500},
                {X: 45338, Y: 50000, Z: -3500},
                {X: 46300, Y: 50000, Z: -3500},
        }
        fillInventory(bot)
        spawnAggressiveMob(bot, 7, 45676, 50000)
        loop.tick()
        require.Equal(t, phaseTownWalk, loop.phase)
        // The cursor skipped the threatened waypoint onto the successor and
        // no click aimed into the camp circle.
        require.Equal(t, 2, loop.wpIndex,
                "the threatened waypoint is skipped onto the successor")
        require.Empty(t, game.walks,
                "no click aimed at the threatened waypoint")
}

// TestWalkStuckFiresOnOscillation pins the net progress watchdog: a
// character that keeps moving without closing on its waypoint is as
// stuck as one that stands still. The plain same-cell check reset the
// window on every hop of the tangent flip-flop and the walk ran for an
// hour between two points 300 units apart.
func TestWalkStuckFiresOnOscillation(t *testing.T) {
        loop, _, bot, nav := newTripLoop()
        nav.found = true
        nav.route = []pathfind.Vec3{
                {X: 45000, Y: 50000, Z: -3500},
                {X: 45600, Y: 50600, Z: -3500},
        }
        fillInventory(bot)
        loop.tick()
        require.Equal(t, phaseTownWalk, loop.phase)
        require.Equal(t, 1, loop.wpIndex)

        // The oscillation: the position alternates between two points whose
        // distance to the waypoint never improves by the progress margin.
        // The old code reset the stuck window on every position change and
        // the stuck never fired.
        loop.stuckAt = time.Now().Add(-stuckTimeout - time.Second)
        loop.stuckX, loop.stuckY = 45000, 50000
        loop.stuckWP = loop.wpIndex
        loop.stuckBest = loop.stuckWaypointDistance(45000, 50000)
        moveSelfTo(bot, 44700, 50300, -3500)
        loop.tick()
        require.GreaterOrEqual(t, loop.rePaths, 1,
                "the oscillation must fire the stuck escalation")
        require.Equal(t, phaseTownWalk, loop.phase,
                "the re-plan keeps the trip walking")
}

// TestZoneDangerRegressesHotGround pins the emergency logout counting:
// a ground that keeps piling mobs on the character is as hostile as
// one that keeps killing it, and the third pile up demotes the band
// like the third death does.
func TestZoneDangerRegressesHotGround(t *testing.T) {
        bot := newTestBot()
        game := &fakeGame{}
        loop := NewLoop(game, bot)
        zones := ElvenHuntingZones()
        loop.SetHuntingZones(zones)
        setZoneTestLevel(bot, 8)
        equipZoneWithGear(bot, 212)
        loop.tick()
        goblin := zoneIndexAt(zones, 51707, 50504)
        require.GreaterOrEqual(t, goblin, 0)
        bot.ApplyPlacement(state.Placement{
                ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
        })
        // Two pile ups count, the third demotes the band.
        for range 2 {
                loop.noteZoneDanger()
                require.Equal(t, int32(-1), loop.zoneDeathCap)
        }
        loop.noteZoneDanger()
        require.Equal(t, int32(4), loop.zoneDeathCap,
                "the third pile up caps the ladder below the goblin band")
        require.Equal(t, int32(3), loop.zoneDeaths[zones[goblin].ID])
        // The tracker ring carries the spots for the next session.
        require.Len(t, bot.DangerSpots(time.Hour), 3)
}

// TestZoneDangerSeedsAcrossSessions pins the cross-session seeding:
// an emergency logout ends its own loop instance, so the fresh session
// must fold the tracker's danger spots back into the regression before
// the first tick - otherwise the too-hot ground never accumulates the
// limit (the observed run: 85 logouts on the same spider ground).
func TestZoneDangerSeedsAcrossSessions(t *testing.T) {
        bot := newTestBot()
        zones := ElvenHuntingZones()
        // The previous sessions logged out three times on the goblin
        // ground; the fresh loop seeds the regression from the ring.
        for range 3 {
                bot.NoteDangerSpot(51707, 50504)
        }
        game := &fakeGame{}
        loop := NewLoop(game, bot)
        loop.SetHuntingZones(zones)
        setZoneTestLevel(bot, 8)
        equipZoneWithGear(bot, 212)
        loop.seedZoneDanger()
        require.Equal(t, int32(3),
                loop.zoneDeaths[zones[zoneIndexAt(zones, 51707, 50504)].ID])
        // One more live pile up crosses the limit and demotes.
        bot.ApplyPlacement(state.Placement{
                ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
        })
        loop.noteZoneDanger()
        require.Equal(t, int32(4), loop.zoneDeathCap)
}

// TestEmergencyLogoutHonestReason pins the reason handover: the
// emergency logout records its cause in the tracker, the supervisor
// consumes it once for the lost record and the next network drop
// reports itself honestly again.
func TestEmergencyLogoutHonestReason(t *testing.T) {
        bot := newTestBot()
        _, ok := bot.ConsumeEmergencyLogout()
        require.False(t, ok)

        bot.SetEmergencyLogout("3 mobs piled on us")
        reason, ok := bot.ConsumeEmergencyLogout()
        require.True(t, ok)
        require.Equal(t, "3 mobs piled on us", reason)

        _, ok = bot.ConsumeEmergencyLogout()
        require.False(t, ok, "the reason is consumed once")
}

// TestEmergencyLogoutCountsTheZoneAndJournals pins the full combat
// logout path: the loop counts the zone danger, records the tracker
// spot and emits the structured logout event with its cooldown.
func TestEmergencyLogoutCountsTheZoneAndJournals(t *testing.T) {
        bot := newTestBot()
        game := &fakeGame{}
        loop := NewLoop(game, bot)
        zones := ElvenHuntingZones()
        loop.SetHuntingZones(zones)
        setZoneTestLevel(bot, 8)
        equipZoneWithGear(bot, 212)
        loop.tick()
        bot.ApplyPlacement(state.Placement{
                ObjectID: 100, X: 51707, Y: 50504, Z: -3529,
        })
        journal, err := session.NewJournal(t.TempDir(), nil)
        require.NoError(t, err)
        loop.SetJournal(journal)

        // Two attackers pile up and the run lapsed its escape budget: the
        // panic branch of the tick fires the emergency logout.
        spawnAggressiveMob(bot, 7, 51700, 50500)
        spawnAggressiveMob(bot, 8, 51740, 50530)
        bot.ApplyObjectTarget(7, 100)
        bot.ApplyObjectTarget(8, 100)
        loop.panicAt = time.Now().Add(-fleeLogoutAfter - time.Second)
        loop.tick()
        require.True(t, loop.logoutDone, "the pile up fired the logout")
        require.Equal(t, int32(1),
                loop.zoneDeaths[zones[zoneIndexAt(zones, 51707, 50504)].ID],
                "the logout counted against the goblin zone")
        require.Len(t, bot.DangerSpots(time.Hour), 1,
                "the logout spot landed in the tracker ring")
        require.True(t, bot.LoginCooldownRemaining() > 0,
                "the login cooldown is armed")
        journal.Close()
        records := journalRecords(t, journal.Path())
        found := false
        for _, r := range records {
                if r.E == "logout" && strings.Contains(r.R, "mobs piled") {
                        found = true
                }
        }
        require.True(t, found,
                "the journal carries the structured logout event")
}

// TestTripAbortCooldownEscalates pins the abort streak ladder: the
// first two identical aborts keep the base cooldown, every further one
// doubles it up to the hour cap, and a successful trip resets the
// streak.
func TestTripAbortCooldownEscalates(t *testing.T) {
        loop, _, _, _ := newTripLoop()
        require.Equal(t, tripCooldown, tripAbortCooldown(1))
        require.Equal(t, tripCooldown, tripAbortCooldown(2))
        require.Equal(t, 2*tripCooldown, tripAbortCooldown(3))
        require.Equal(t, 4*tripCooldown, tripAbortCooldown(4))
        require.Equal(t, tripAbortMaxCooldown, tripAbortCooldown(20))

        for i := range 4 {
                loop.endTownTrip("aborted, no walkable path to the shop")
                require.Equal(t, i+1, loop.tripAbortRun)
        }
        require.Equal(t, 4, loop.tripAbortRun)
        loop.endTownTrip("back at the farm spot")
        require.Equal(t, 0, loop.tripAbortRun, "a good ending resets the streak")
}
