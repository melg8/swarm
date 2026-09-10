// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// testSpots builds a two spot registry for the policy tests: a home
// ground at the character position (the village coordinates of
// setZoneTestLevel) and a richer alternative a walk away.
func testSpots() []Spot {
	return []Spot{
		{
			ID: "test-home", Name: "Home Keltirs", Region: "elven",
			MinLevel: 1, MaxLevel: 5,
			AnchorX: 46112, AnchorY: 41500, Radius: 1500,
			RespawnMin: 15, RespawnMax: 20, Mass: 6,
			Mobs: []SpotMob{
				{
					TemplateID: 20534, Name: "Red Keltir",
					Level: 2, Count: 4,
					RespawnMin: 15, RespawnMax: 20,
				},
				{
					TemplateID: 20537,
					Name:       "Elder Red Keltir",
					Level:      4, Count: 2,
					RespawnMin: 15, RespawnMax: 20,
				},
			},
		},
		{
			ID: "test-rich", Name: "Rich Wolves", Region: "elven",
			MinLevel: 4, MaxLevel: 8,
			AnchorX: 49000, AnchorY: 54000, Radius: 1800,
			RespawnMin: 15, RespawnMax: 20, Mass: 12,
			Mobs: []SpotMob{
				{
					TemplateID: 20014, Name: "Gray Wolf",
					Level: 6, Count: 8,
					RespawnMin: 15, RespawnMax: 20,
				},
				{
					TemplateID: 20017,
					Name:       "Elder Wolf",
					Level:      8, Count: 4,
					RespawnMin: 15, RespawnMax: 20,
				},
			},
		},
	}
}

// spotTestBot prepares a level 6 character standing on the home spot
// anchor.
func spotTestBot(t *testing.T) *state.Bot {
	t.Helper()
	bot := newTestBot()
	setZoneTestLevel(bot, 6)

	return bot
}

// pickedSpotLoop prepares a hunting loop in the spot mode with the
// home ground picked. The fleet claim releases when the test ends.
func pickedSpotLoop(t *testing.T) (*Loop, *fakeGame, *spotHunter) {
	t.Helper()
	bot := spotTestBot(t)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingSpots(testSpots())
	loop.tick()
	require.Equal(t, "test-home", loop.zonePickedID)
	require.NotNil(t, loop.spot)
	require.GreaterOrEqual(t, loop.spot.picked, 0)
	hunter := loop.spot
	t.Cleanup(func() {
		if hunter.leave != nil {
			hunter.leave()
		}
	})

	return loop, game, hunter
}

func TestElvenSpotRegistryShape(t *testing.T) {
	spots := ElvenHuntingSpots()
	require.Greater(t, len(spots), 30)
	require.Less(t, len(spots), len(ElvenHuntingZones()))
	totalMass := 0.0
	for index := range spots {
		spot := spots[index]
		require.NotEmpty(t, spot.ID)
		require.NotEmpty(t, spot.Name)
		require.Equal(t, regionElven, spot.Region)
		require.Positive(t, spot.Radius)
		// The visibility invariant: the spot circle never grows
		// past the guaranteed knownlist radius.
		require.LessOrEqual(t, spot.Radius, int32(2048))
		// The leash square inscribes in the circle: the square
		// corners stay inside the guaranteed visible circle
		// of the anchor (half * sqrt(2) <= 2048).
		leash := spot.leashHalf()
		require.LessOrEqual(t, float64(leash)*1.4143, 2048.5)
		require.Positive(t, spot.Mass)
		require.GreaterOrEqual(t, spot.Mass, 2.0)
		require.Positive(t, spot.RespawnMin)
		require.GreaterOrEqual(t, spot.RespawnMax, spot.RespawnMin)
		require.NotEmpty(t, spot.Mobs)
		for mobIndex := range spot.Mobs {
			mob := &spot.Mobs[mobIndex]
			require.Positive(t, mob.Level)
			require.Positive(t, mob.Count)
			require.GreaterOrEqual(
				t, mob.RespawnMax, mob.RespawnMin)
		}
		totalMass += spot.Mass
	}
	// The registry covers most of the spawn mass (the elven lands
	// hold 722 expected mobs, the singleton grounds drop out).
	require.Greater(t, totalMass, 600.0)
	// The first entry is the starter fallback: the village nearest
	// spot of the lowest band.
	require.LessOrEqual(t, spots[0].MinLevel, int32(2))
}

func TestSpotLeashInscribesInVisibilityCircle(t *testing.T) {
	spot := Spot{
		ID: "s", Name: "s", Region: regionElven,
		MinLevel: 1, MaxLevel: 3,
		AnchorX: 0, AnchorY: 0, Radius: 2048,
		RespawnMin: 15, RespawnMax: 20, Mass: 2,
		Mobs: []SpotMob{{
			TemplateID: 1, Name: "m", Level: 1, Count: 2,
			RespawnMin: 15, RespawnMax: 20,
		}},
	}
	half := spot.leashHalf()
	// half * sqrt(2) <= 2048: the square corners stay inside the
	// guaranteed visible circle of the anchor.
	require.LessOrEqual(t, float64(half)*1.41422, 2048.0)
	require.Greater(t, half, int32(1000))
	zone := spot.zoneSquare()
	require.Equal(t, spot.AnchorX, zone.CX)
	require.Equal(t, spot.AnchorY, zone.CY)
	require.Equal(t, half, zone.Half)
}

func TestSpotWindowHelpers(t *testing.T) {
	spot := Spot{
		ID: "s", Name: "s", Region: regionElven,
		MinLevel: 2, MaxLevel: 12,
		AnchorX: 0, AnchorY: 0, Radius: 1500,
		RespawnMin: 15, RespawnMax: 20, Mass: 12,
		Mobs: []SpotMob{
			{
				TemplateID: 1, Name: "low", Level: 2, Count: 3,
				RespawnMin: 15, RespawnMax: 20,
			},
			{
				TemplateID: 2, Name: "mid", Level: 7, Count: 5,
				RespawnMin: 15, RespawnMax: 20,
			},
			{
				TemplateID: 3, Name: "high", Level: 12, Count: 4,
				RespawnMin: 15, RespawnMax: 20,
			},
		},
	}
	// The white-green window [L-5, L] for level 10: mid only.
	require.Equal(t, 5.0, spotWindowMass(spot, 10))
	// The wide eligibility window [L-8, L+2]: mid and high.
	require.True(t, spotEligible(spot, 10))
	require.False(t, spotEligible(spot, 22))
	// A fresh character hunts the low mobs too (the ceiling L+2
	// covers the level 2 mob of the ground).
	require.True(t, spotEligible(spot, 1))
	require.Equal(t, int32(3), spotLevelDistance(spot, 15))
}

func TestSpotMobPrioritiesBiasWindow(t *testing.T) {
	spot := Spot{
		ID: "s", Name: "s", Region: regionElven,
		MinLevel: 2, MaxLevel: 12,
		AnchorX: 0, AnchorY: 0, Radius: 1500,
		RespawnMin: 15, RespawnMax: 20, Mass: 12,
		Mobs: []SpotMob{
			{
				TemplateID: 20534, Name: "keltir", Level: 2,
				Count: 3, RespawnMin: 15, RespawnMax: 20,
			},
			{
				TemplateID: 20014, Name: "wolf", Level: 6,
				Count: 5, RespawnMin: 15, RespawnMax: 20,
			},
			{
				TemplateID: 20017, Name: "elder", Level: 8,
				Count: 4, RespawnMin: 15, RespawnMax: 20,
			},
		},
	}
	priorities := spotMobPriorities(spot, 10)
	// The preference subrange [L-4, L-1] = levels 6-9: the wolves
	// carry the strongest bias.
	require.Equal(t, int32(3),
		priorities[npcdata.NPCWireTemplateID(20014)])
	require.Equal(t, int32(3),
		priorities[npcdata.NPCWireTemplateID(20017)])
	// The keltir (level 2) sits inside the wide adena window
	// [L-8, L+2] = [2, 12]: the weakest tail bias.
	require.Equal(t, int32(1),
		priorities[npcdata.NPCWireTemplateID(20534)])
	// Below the wide window: no bias at all.
	require.NotContains(t, priorities, npcdata.NPCWireTemplateID(1))
}

func TestSpotPickAppliesLeashAndBias(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	spot := hunter.spots[hunter.picked]
	// The leash square of the loop mirrors the spot.
	require.Equal(t, spot.AnchorX, loop.zoneCX)
	require.Equal(t, spot.AnchorY, loop.zoneCY)
	require.Equal(t, spot.leashHalf(), loop.zoneHalf)
	require.Equal(t, int32(6), hunter.level)
	// The engage bias of the window carries the keltir wire ids.
	require.Contains(t, loop.zoneMobPriority,
		npcdata.NPCWireTemplateID(20534))
	// The zone registry of the legacy mode stands down.
	require.Nil(t, loop.zones)
	// The fleet claim holds the spot.
	require.Equal(t, 1, globalSpotHub.occupancy(spot.ID))
}

func TestSpotKillPredictionOverlay(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	bot := loop.tracker
	now := time.Now()
	// A keltir of the home ground dies 800 units off the anchor.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20534),
		X: 46912, Y: 41500, Z: -3500, Attackable: true,
	})
	loop.spotNoteKill(7001, now)
	require.Len(t, hunter.kills, 1)
	kill := hunter.kills[0]
	require.Equal(t, "test-home", kill.spotID)
	require.Equal(t, int32(46912), kill.x)
	// The predicted respawn: the window midpoint (17.5 s of the
	// 15-20 s elven default).
	require.Equal(t, now.Add(17*time.Second+500*time.Millisecond),
		kill.respawnAt)
	eta, predX, predY, pending := hunter.earliestPendingRespawn(
		"test-home", now.Add(time.Second))
	require.True(t, pending)
	require.Equal(t, int32(46912), predX)
	require.Equal(t, int32(41500), predY)
	require.InDelta(t, 16.5, eta.Seconds(), 0.1)
	// Expired predictions leave the overlay.
	_, _, _, pending = hunter.earliestPendingRespawn(
		"test-home", now.Add(30*time.Second))
	require.False(t, pending)
	// The kill folded into the visit metrics and the centroid EMA.
	metric := &hunter.metrics[hunter.picked]
	require.Equal(t, 1, metric.visitKills)
	require.True(t, metric.killPosKnown)
	require.Equal(t, float64(46912), metric.killX)
}

func TestSpotWaitsForPredictedRespawn(t *testing.T) {
	loop, game, _ := pickedSpotLoop(t)
	bot := loop.tracker
	now := time.Now()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20534),
		X: 46912, Y: 41500, Z: -3500, Attackable: true,
	})
	loop.spotNoteKill(7001, now)
	// The square reads empty (the keltir is dead): the overlay
	// predicts the respawn within the patience window, so the
	// hunter holds the ground and drifts toward the corpse. The
	// ticks walk the full state machine: the dead target clears
	// through the loot phase, the emptiness arms, the economy
	// reads the prediction.
	bot.ApplyStatusUpdate(7001, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	for tick := 0; tick < 6; tick++ {
		loop.tick()
	}
	require.Equal(t, "test-home", loop.zonePickedID)
	require.NotEmpty(t, game.walks)
	// The drift aims at the predicted respawn position.
	last := game.walks[len(game.walks)-1]
	require.Equal(t, int32(46912), last[0])
	require.Equal(t, int32(41500), last[1])
}

func TestSpotStarvedSwitchesGround(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	now := time.Now()
	// The square reads empty, the overlay holds nothing, the stay
	// and the emptiness outran the starved thresholds.
	hunter.emptySince = now.Add(-70 * time.Second)
	hunter.enteredAt = now.Add(-2 * time.Minute)
	loop.spotEvaluate(now)
	require.Equal(t, "test-rich", loop.zonePickedID)
	require.Equal(t, 1, globalSpotHub.occupancy("test-rich"))
	require.Equal(t, 0, globalSpotHub.occupancy("test-home"))
}

func TestSpotHysteresisHoldsComparableGround(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	now := time.Now()
	// A starved square but a short stay: the ground keeps its post
	// until the exhaustion floor passes.
	hunter.emptySince = now.Add(-70 * time.Second)
	hunter.enteredAt = now.Add(-10 * time.Second)
	loop.spotEvaluate(now)
	require.Equal(t, "test-home", loop.zonePickedID)
	// A longer stay but a fresh emptiness: also no switch.
	hunter.enteredAt = now.Add(-2 * time.Minute)
	hunter.emptySince = now.Add(-5 * time.Second)
	loop.spotEvaluate(now)
	require.Equal(t, "test-home", loop.zonePickedID)
}

func TestSpotDeathHeatRegresses(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	now := time.Now()
	loop.spotNoteDeath(now)
	require.GreaterOrEqual(t, hunter.metrics[0].deaths, 1.0)
	require.Equal(t, "test-rich", loop.zonePickedID)
	// The heat decays with time: a half-life later it halves.
	heatNow := hunter.deathHeat(&hunter.metrics[0], now)
	heatLater := hunter.deathHeat(&hunter.metrics[0],
		now.Add(30*time.Minute))
	require.InDelta(t, heatNow*0.5, heatLater, 1e-6)
}

func TestSpotMeasuredRateReplacesBootstrap(t *testing.T) {
	_, _, hunter := pickedSpotLoop(t)
	metric := &hunter.metrics[hunter.picked]
	// The bootstrap prior prices the window mass and the level.
	prior := hunter.spotValue(&hunter.spots[hunter.picked], metric)
	require.Greater(t, prior, 0.0)
	// Six thousand adena over ten active minutes: the measured
	// rate (600/min) takes over once trusted.
	metric.activeMin = 10
	metric.adena = 6000
	measured := hunter.spotValue(&hunter.spots[hunter.picked], metric)
	require.InDelta(t, 600.0, measured, 0.001)
	// The visit rate outranks the historical one.
	metric.visitActiveMin = 1
	metric.visitAdena = 1200
	measured = hunter.spotValue(&hunter.spots[hunter.picked], metric)
	require.InDelta(t, 1200.0, measured, 0.001)
}

func TestSpotAccumulateAttributesAdena(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	now := time.Now()
	metric := &hunter.metrics[hunter.picked]
	hunter.accumulate(loop, now)
	// A loot gain lands in the inventory: the next accumulation
	// attributes it to the ground (the adena item, type 2 class 4).
	loop.tracker.ApplyItemList([]state.InventoryItem{
		{ObjectID: 999, ItemID: 57, Count: 900, Type2: 4, Change: 1},
	})
	hunter.accumulate(loop, now.Add(2*time.Second))
	require.InDelta(t, 2.0/60.0, metric.visitActiveMin, 0.001)
	require.Equal(t, int64(900), metric.visitAdena)
	// The fold closes the visit into the totals.
	hunter.foldVisit(hunter.picked)
	require.Equal(t, int64(900), metric.adena)
	require.Equal(t, 2.0/60.0, metric.activeMin, 0.001)
}

func TestSpotViewCarriesTheEconomy(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	bot := loop.tracker
	now := time.Now()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20534),
		X: 46912, Y: 41500, Z: -3500, Attackable: true,
	})
	loop.spotNoteKill(7001, now)
	hunter.publishView(loop, now.Add(time.Second))
	views := bot.Snapshot().HuntingZones
	require.Len(t, views, 2)
	active := -1
	for index := range views {
		view := views[index]
		require.Equal(t, "spot", view.Kind)
		require.Positive(t, view.Radius)
		require.Equal(t, int32(15), view.RespawnMinSec)
		require.Equal(t, int32(20), view.RespawnMaxSec)
		require.Positive(t, view.SpawnMass)
		require.Positive(t, view.Half)
		if view.Active {
			active = index
		}
	}
	require.GreaterOrEqual(t, active, 0)
	// The active spot carries the respawn prediction and the
	// occupancy of the fleet claim.
	require.GreaterOrEqual(t, views[active].NextRespawnSec, int32(0))
	require.Equal(t, int32(1), views[active].Occupancy)
	require.NotEqual(t, 0, views[active].KillX)
}

func TestSpotViewPublishesKillMarks(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	bot := loop.tracker
	now := time.Now()
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7001, TemplateID: npcdata.NPCWireTemplateID(20534),
		X: 46912, Y: 41500, Z: -3500, Attackable: true,
	})
	require.Nil(t, bot.KillMarks())
	loop.spotNoteKill(7001, now)
	hunter.publishView(loop, now.Add(time.Second))

	// The kill ring rides the view refresh: the fleet cross layer of
	// the map reads the positions of the recent kills.
	marks := bot.KillMarks()
	require.Len(t, marks, 1)
	require.Equal(t, int32(46912), marks[0].X)
	require.Equal(t, int32(41500), marks[0].Y)
	require.Equal(t, now.UnixMilli(), marks[0].AtMs)

	// The registry aggregation stamps the bot id over the marks.
	registry := state.NewRegistry()
	registry.Add(bot)
	merged := registry.FleetKillMarks(100)
	require.Len(t, merged, 1)
	require.Equal(t, bot.ID(), merged[0].BotID)
}

func TestSpotUserSelectOverridesEconomy(t *testing.T) {
	loop, _, hunter := pickedSpotLoop(t)
	loop.userZoneSelect(1)
	require.Equal(t, "test-rich", loop.zonePickedID)
	require.Equal(t, 1, hunter.userOverride)
	// The economy respects the override: no automatic re-pick
	// while the window still matches the character level.
	loop.spotEvaluate(time.Now())
	require.Equal(t, "test-rich", loop.zonePickedID)
	// Outgrowing the window clears the override: the level 20
	// character leaves the wolf ground (mobs 4-8, wide window
	// [12, 22] closed) and the economy re-picks.
	setZoneTestLevel(loop.tracker, 20)
	loop.spotEvaluate(time.Now())
	require.Equal(t, -1, hunter.userOverride)
}

func TestSpotMobRespawnLookup(t *testing.T) {
	spots := testSpots()
	rmin, rmax, ok := spotMobRespawn(
		spots, npcdata.NPCWireTemplateID(20014))
	require.True(t, ok)
	require.Equal(t, int32(15), rmin)
	require.Equal(t, int32(20), rmax)
	_, _, ok = spotMobRespawn(spots, 999123)
	require.False(t, ok)
}
