// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sampleAt builds a sample record helper for the aggregate tests.
func sampleAt(t int64, lv int32, xp int64, ad int64, x int32, y int32, ph string) record {
	r := newRecord("test1", kindSample, time.Unix(t, 0))
	r.Lv = lv
	r.Xp = xp
	r.Ad = ad
	r.X = x
	r.Y = y
	r.Ph = ph

	return r
}

// TestAggregatorLevelMarks verifies the level curve synthesis from
// the sample stream (the first level is the baseline, not a mark; the
// duplicates dedup on the level value).
func TestAggregatorLevelMarks(t *testing.T) {
	agg := newBotAgg("test1")
	agg.apply(sampleAt(1700000000, 1, 100, 0, 10, 10, "engage"))
	agg.apply(sampleAt(1700000030, 2, 300, 0, 20, 20, "engage"))
	agg.apply(sampleAt(1700000060, 3, 700, 0, 30, 30, "engage"))
	agg.apply(sampleAt(1700000090, 3, 900, 0, 40, 40, "engage"))
	require.Len(t, agg.levels, 2)
	require.Equal(t, int32(2), agg.levels[0].lv)
	require.Equal(t, int32(3), agg.levels[1].lv)
}

// TestAggregatorKillStats verifies the kill counters, the per-mob
// statistics and the fight histogram.
func TestAggregatorKillStats(t *testing.T) {
	agg := newBotAgg("test1")
	kills := []struct {
		mob string
		lvl int32
		dur float64
	}{
		{"Kaboo Orc", 4, 8},
		{"Kaboo Orc", 4, 12},
		{"Wolf", 2, 3},
		{"Elder Wolf", 7, 65},
	}
	for i, k := range kills {
		r := newRecord("test1", kindKill, time.Unix(int64(1700000000+i*60), 0))
		r.Mob = k.mob
		r.Lvl = k.lvl
		r.Dur = k.dur
		r.Hp = 90
		agg.apply(r)
	}
	require.Equal(t, 4, agg.kills)
	require.Len(t, agg.mobs, 3)
	orc := agg.mobs["Kaboo Orc"]
	require.Equal(t, 2, orc.count)
	require.InDelta(t, 10.0, orc.durSum/2, 0.001)
	require.InDelta(t, 12.0, orc.durMax, 0.001)
	// The 65s fight lands in the last histogram bin, the 3s fight in
	// the third.
	require.Equal(t, int32(1), agg.durHist[3])
	require.Equal(t, int32(1), agg.durHist[len(agg.durHist)-1])
	// The slowest fight list keeps the Elder Wolf fight first.
	require.Equal(t, "Elder Wolf", agg.slowFight[0].mob)
	p50, p90, p99 := percentiles(agg.durHist, agg.kills)
	require.GreaterOrEqual(t, p90, p50)
	require.GreaterOrEqual(t, p99, p90)
}

// TestAggregatorTrips verifies the town trip brackets: the duration,
// the reason counting and the longest trip.
func TestAggregatorTrips(t *testing.T) {
	agg := newBotAgg("test1")
	start := newRecord("test1", kindTripStart, time.Unix(1700000000, 0))
	start.R = "inventory full"
	agg.apply(start)
	end := newRecord("test1", kindTripEnd, time.Unix(1700000600, 0))
	end.R = "walked back"
	end.Dur = 600
	agg.apply(end)
	require.Equal(t, 1, agg.trips)
	require.InDelta(t, 600.0, agg.tripSec, 0.001)
	require.Equal(t, 1, agg.tripReason["walked back"])
	require.InDelta(t, 600.0, agg.tripMax.dur, 0.001)
	// A trip end without a start (a zone return walk that reuses the
	// machinery) is a no-op, not a crash.
	orphan := newRecord("test1", kindTripEnd, time.Unix(1700001200, 0))
	orphan.R = "zone return"
	agg.apply(orphan)
	require.Equal(t, 1, agg.trips)
}

// TestAggregatorFreezeDetection verifies the position freeze marks:
// six consecutive samples on one cell while the phase farms record a
// freeze; the moving samples reset the run.
func TestAggregatorFreezeDetection(t *testing.T) {
	agg := newBotAgg("test1")
	for i := range freezeRunThreshold + 2 {
		agg.apply(sampleAt(
			int64(1700000000+i*samplePeriodSec), 5, 100, 0, 10, 10, "engage"))
	}
	require.Len(t, agg.freezes, 1)
	// A walk away resets the freeze run: no second mark from fresh
	// consecutive holds shorter than the threshold... the new run
	// needs the full threshold again.
	for i := range freezeRunThreshold {
		agg.apply(sampleAt(int64(1700000300+i*samplePeriodSec),
			5, 100, 0, 10, 10, "engage"))
	}
	require.Len(t, agg.freezes, 1)
	agg.apply(sampleAt(1700000600, 5, 100, 0, 99, 99, "engage"))
	for i := range freezeRunThreshold {
		agg.apply(sampleAt(int64(1700000700+i*samplePeriodSec),
			5, 100, 0, 77, 77, "engage"))
	}
	require.Len(t, agg.freezes, 2)
}

// TestAggregatorGapDetection verifies the offline gap marks between
// distant samples.
func TestAggregatorGapDetection(t *testing.T) {
	agg := newBotAgg("test1")
	agg.apply(sampleAt(1700000000, 5, 100, 0, 1, 1, "engage"))
	agg.apply(sampleAt(1700000000+gapThresholdSec+10, 5, 100, 0, 1, 1,
		"engage"))
	require.Len(t, agg.gaps, 1)
	require.InDelta(t, float64(gapThresholdSec+10), agg.gaps[0].sec, 0.001)
}

// TestAggregatorHourBuckets verifies the hourly attribution: the
// phase seconds, the xp and adena deltas of the bucket.
func TestAggregatorHourBuckets(t *testing.T) {
	agg := newBotAgg("test1")
	agg.apply(sampleAt(1700000000, 5, 1000, 500, 1, 1, "engage"))
	agg.apply(sampleAt(1700000030, 5, 1300, 700, 2, 2, "engage"))
	agg.apply(sampleAt(1700000060, 5, 1600, 900, 3, 3, "townWalk"))
	agg.apply(sampleAt(1700000090, 5, 1600, 900, 4, 4, "engage"))
	key := int32(1700000000 / 3600)
	bucket := agg.hours[key]
	require.InDelta(t, 60.0, bucket.huntSec, 0.001)
	require.InDelta(t, 30.0, bucket.townSec, 0.001)
	require.Equal(t, int64(600), bucket.xpLast-bucket.xpFirst)
	require.Equal(t, int64(400), bucket.adLast-bucket.adFirst)
}

// TestAggregatorStallKind verifies the stall split between the xp and
// position lists.
func TestAggregatorStallKind(t *testing.T) {
	agg := newBotAgg("test1")
	xp := newRecord("test1", kindStall, time.Unix(1700000000, 0))
	xp.R = "xp"
	xp.Dur = 1200
	agg.apply(xp)
	pos := newRecord("test1", kindStall, time.Unix(1700000200, 0))
	pos.R = "pos"
	pos.Dur = 600
	pos.X, pos.Y = 10, 20
	agg.apply(pos)
	require.Len(t, agg.xpStalls, 1)
	require.Len(t, agg.posStalls, 1)
	require.InDelta(t, 1200.0, agg.xpStalls[0].sec, 0.001)
}

// TestAggregatorMoneyTrail verifies the buy marks.
func TestAggregatorMoneyTrail(t *testing.T) {
	agg := newBotAgg("test1")
	buy := newRecord("test1", kindBuy, time.Unix(1700000000, 0))
	buy.Items = "Short Sword, Leather Shield"
	buy.N = 2
	buy.Cost = 2500
	agg.apply(buy)
	require.Len(t, agg.buys, 1)
	require.Equal(t, int64(2500), agg.buys[0].cost)
	require.Equal(t, "Short Sword, Leather Shield", agg.buys[0].items)
}

// TestAppendCapped verifies the capped lists keep the newest entries.
func TestAppendCapped(t *testing.T) {
	dst := make([]int, 0, 4)
	for i := range 6 {
		dst = appendCapped(dst, i, 4)
	}
	require.Equal(t, []int{2, 3, 4, 5}, dst)
}

// TestAggregatorStoryRing verifies the ring keeps the newest line
// last.
func TestAggregatorStoryRing(t *testing.T) {
	agg := newBotAgg("test1")
	for i := range storyRingLen + 5 {
		r := newRecord("test1", kindStory, time.Unix(int64(1700000000+i), 0))
		r.M = "line " + itoa(int64(i))
		agg.apply(r)
	}
	require.Len(t, agg.ring, storyRingLen)
	require.Equal(t, "line "+itoa(storyRingLen+4), agg.ring[len(agg.ring)-1].msg)
}
