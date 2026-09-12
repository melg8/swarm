// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newMetricsBot creates a bot with a known self id and vitals for the
// metrics tests: the character stands at full health (100 HP, 50 MP)
// with the object id 100.
func newMetricsBot() *Bot {
	bot := NewBot("metrics-bot")
	bot.SetCharacter("Metrics", 100, 18, 45000, 50000, -3500, 100, 50)

	return bot
}

// selfAttributes builds the StatusUpdate attribute list of the self.
func selfAttributes(pairs ...Attribute) []Attribute {
	return pairs
}

func TestMetricsCountsSessions(t *testing.T) {
	bot := newMetricsBot()
	require.EqualValues(t, 0, bot.Metrics().Sessions)

	bot.ResetSession()
	bot.ResetSession()

	require.EqualValues(t, 2, bot.Metrics().Sessions)
}

func TestMetricsDeathCountsTransitions(t *testing.T) {
	bot := newMetricsBot()
	// The maximum arrives with the first vitals update of the world.
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrMaxHP, Value: 100},
	))

	// The first zero HP update is the death.
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 0},
	))
	require.EqualValues(t, 1, bot.Metrics().Deaths)
	require.False(t, bot.Metrics().LastDeathAt.IsZero())

	// Repeated zero updates (the corpse sync) do not recount.
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 0},
	))
	require.EqualValues(t, 1, bot.Metrics().Deaths)

	// The village restart revives without a death.
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 50},
	))
	require.EqualValues(t, 1, bot.Metrics().Deaths)

	// The second demise counts again.
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 0},
	))
	require.EqualValues(t, 2, bot.Metrics().Deaths)
}

func TestMetricsKillAttribution(t *testing.T) {
	bot := newMetricsBot()
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, Name: "Keltir", Attackable: true})
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 8, Name: "Orc", Attackable: true})

	// The character swings at the Keltir: the fight target arms the
	// attribution.
	bot.ApplyAttack(Attack{
		AttackerID: 100,
		X:          45000, Y: 50000, Z: -3500,
		TargetX: 45100, TargetY: 50100, TargetZ: -3500,
		TargetIDs:   [AttackTargets]int32{7},
		TargetCount: 1,
	})

	// The Keltir dies while the character fights it: a kill.
	bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 0}})
	require.EqualValues(t, 1, bot.Metrics().Kills)
	require.False(t, bot.Metrics().LastKillAt.IsZero())

	// The Orc dies without the character ever fighting it: no kill.
	bot.ApplyStatusUpdate(8, []Attribute{{ID: AttrCurHP, Value: 0}})
	require.EqualValues(t, 1, bot.Metrics().Kills)
}

func TestMetricsKillRequiresFreshFight(t *testing.T) {
	bot := newMetricsBot()
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, Name: "Keltir", Attackable: true})
	bot.ApplyAttack(Attack{
		AttackerID:  100,
		TargetIDs:   [AttackTargets]int32{7},
		TargetCount: 1,
	})

	// The last swing sits outside the attribution window: another
	// hunter finished the mob, the death does not count for the bot.
	bot.mu.Lock()
	bot.char.CombatActiveAt = time.Now().Add(-killAttributionWindow -
		time.Second)
	bot.mu.Unlock()
	bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 0}})

	require.EqualValues(t, 0, bot.Metrics().Kills)
}

func TestMetricsSwingCounters(t *testing.T) {
	bot := newMetricsBot()
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, Name: "Keltir", Attackable: true})

	// A landed blow of the character.
	bot.ApplyAttack(Attack{
		AttackerID:  100,
		TargetIDs:   [AttackTargets]int32{7},
		HitFlags:    [AttackTargets]int8{0},
		TargetCount: 1,
	})
	// An evaded blow of the character (the miss flag is negative on
	// the wire, see attackHitMissFlag).
	bot.ApplyAttack(Attack{
		AttackerID:  100,
		TargetIDs:   [AttackTargets]int32{7},
		HitFlags:    [AttackTargets]int8{attackHitMissFlag},
		TargetCount: 1,
	})
	// A blow aimed at the character by the Keltir.
	bot.ApplyAttack(Attack{
		AttackerID:  7,
		TargetIDs:   [AttackTargets]int32{100},
		HitFlags:    [AttackTargets]int8{0},
		TargetCount: 1,
	})

	view := bot.Metrics()
	require.EqualValues(t, 2, view.SwingsMade)
	require.EqualValues(t, 1, view.SwingsLanded)
	require.EqualValues(t, 1, view.SwingsTaken)
}

func TestMetricsDamageTakenAccumulates(t *testing.T) {
	bot := newMetricsBot()

	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 60},
	))
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 100},
	))
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrCurHP, Value: 30},
	))

	require.InDelta(t, 110.0, bot.Metrics().DamageTaken, 0.001)
}

func TestMetricsHuntTickAverages(t *testing.T) {
	bot := newMetricsBot()

	bot.NoteHuntTick(10 * time.Millisecond)
	bot.NoteHuntTick(10 * time.Millisecond)
	bot.NoteHuntTick(20 * time.Millisecond)

	view := bot.Metrics()
	require.EqualValues(t, 3, view.TickCount)
	require.Equal(t, 20*time.Millisecond, view.TickMax)
	// The EMA converges towards the mean from zero with the weight
	// 0.1: 1ms, 1.9ms, 3.71ms after the three ticks.
	require.InDelta(t, 3.71*float64(time.Millisecond),
		float64(view.TickEMA), float64(time.Microsecond))
}

func TestMetricsHuntTickIgnoresNegative(t *testing.T) {
	bot := newMetricsBot()

	bot.NoteHuntTick(-time.Second)

	require.EqualValues(t, 0, bot.Metrics().TickCount)
}

func TestMetricsSurviveSessionReset(t *testing.T) {
	bot := newMetricsBot()
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, Name: "Keltir", Attackable: true})
	bot.ApplyAttack(Attack{
		AttackerID:  100,
		TargetIDs:   [AttackTargets]int32{7},
		TargetCount: 1,
	})
	bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 0}})
	bot.ApplyStatusUpdate(100, selfAttributes(
		Attribute{ID: AttrMaxHP, Value: 100},
		Attribute{ID: AttrCurHP, Value: 0},
	))
	bot.ResetSession()

	view := bot.Metrics()
	require.EqualValues(t, 1, view.Kills)
	require.EqualValues(t, 1, view.Deaths)
	require.EqualValues(t, 1, view.Sessions)
}
