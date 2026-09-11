// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// TestParseDumpRoundTrip pins the round trip: BuildStateDump of a
// bot with a known world, then ParseDump back into a Snapshot, then
// assert the reconstructed snapshot carries the character, the
// objects, the walk plan and the zone the dump printed. This is the
// contract the dump-state-repro skill relies on: a dump captured
// from a long-lived user server reconstructs the exact world a
// misbehaving bot saw, without the user having to replay the
// situation by hand.
func TestParseDumpRoundTrip(t *testing.T) {
	_, bot := newTestServer(t)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 40},
	})
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	bot.RecordEvent("sat down to rest")
	bot.SetHuntingZone(46112, 41500, 450)
	bot.SetWalkPlan(state.WalkPlan{
		Origin: &state.WalkPoint{X: 45800, Y: 41700, Z: -3500},
		Points: []state.WalkPoint{
			{X: 46000, Y: 41600, Z: -3500},
			{X: 46112, Y: 41500, Z: -3510},
		},
		Index: 1,
		Dest:  &state.WalkPoint{X: 46150, Y: 41480, Z: -3512},
	})

	dump := BuildStateDump(bot)
	snap, err := ParseDump(dump)
	require.NoError(t, err)

	// The identity and the phase.
	require.Equal(t, "test1", snap.ID)
	require.Equal(t, state.StatusOnline, snap.Status)

	// The character: the position, the HP, the sit state.
	require.Equal(t, "test1", snap.Character.Name)
	require.Equal(t, int32(100), snap.Character.ObjectID)
	require.Equal(t, int32(45000), snap.Character.X)
	require.Equal(t, int32(50000), snap.Character.Y)
	require.Equal(t, int32(-3500), snap.Character.Z)
	require.InDelta(t, 40, snap.Character.CurHP, 0.5)
	require.True(t, snap.Character.Sitting,
		"the dump reported sitting true")

	// The hunting zone: center, half.
	require.NotNil(t, snap.HuntingZone)
	require.Equal(t, int32(46112), snap.HuntingZone.CX)
	require.Equal(t, int32(41500), snap.HuntingZone.CY)
	require.Equal(t, int32(450), snap.HuntingZone.Half)

	// The walk plan: the origin, the two waypoints, the dest, the
	// aiming index.
	require.Len(t, snap.WalkPath, 2)
	require.NotNil(t, snap.WalkOrigin)
	require.Equal(t, int32(45800), snap.WalkOrigin.X)
	require.Equal(t, int32(41700), snap.WalkOrigin.Y)
	require.Equal(t, int32(-3500), snap.WalkOrigin.Z)
	require.Equal(t, int32(46000), snap.WalkPath[0].X)
	require.Equal(t, int32(41600), snap.WalkPath[0].Y)
	require.Equal(t, int32(46112), snap.WalkPath[1].X)
	require.Equal(t, int32(41500), snap.WalkPath[1].Y)
	require.Equal(t, int32(-3510), snap.WalkPath[1].Z)
	require.NotNil(t, snap.WalkDest)
	require.Equal(t, int32(46150), snap.WalkDest.X)
	require.Equal(t, int32(41480), snap.WalkDest.Y)
	require.Equal(t, int32(-3512), snap.WalkDest.Z)
	require.Equal(t, 1, snap.WalkIndex,
		"the dump reported aiming at wp 1")

	// The events: the hunt decisions mirror into the event log.
	require.NotEmpty(t, snap.Events, "the dump carried events")
	require.Contains(t, snap.Events[len(snap.Events)-1].Message,
		"sat down to rest")
}

// TestParseDumpCharacterFields pins the character field parser: the
// class line, the position, the vitals, the stats and the load all
// read back correctly.
func TestParseDumpCharacterFields(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 80},
		{ID: state.AttrMaxHP, Value: 113},
		{ID: state.AttrCurMP, Value: 30},
		{ID: state.AttrMaxMP, Value: 39},
	})
	dump := BuildStateDump(bot)
	snap, err := ParseDump(dump)
	require.NoError(t, err)

	c := snap.Character
	require.Equal(t, int32(18), c.ClassID)
	require.Equal(t, int32(45000), c.X)
	require.Equal(t, int32(50000), c.Y)
	require.Equal(t, int32(-3500), c.Z)
	require.InDelta(t, 80, c.CurHP, 0.5)
	require.InDelta(t, 113, c.MaxHP, 0.5)
	require.InDelta(t, 30, c.CurMP, 0.5)
	require.InDelta(t, 39, c.MaxMP, 0.5)
}

// TestParseDumpObjects pins the world object parser: the object id,
// the kind, the name, the HP, the position and the flags all read
// back correctly. The level depends on the npcdata lookup of the
// template id; the parser reads whatever level the dump printed.
func TestParseDumpObjects(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID:   200,
		TemplateID: 10001,
		Attackable: true,
		Name:       "Keltir",
		X:          45200,
		Y:          50100,
		Z:          -3500,
	})
	bot.ApplyStatusUpdate(200, []state.Attribute{
		{ID: state.AttrCurHP, Value: 50},
		{ID: state.AttrMaxHP, Value: 50},
	})
	dump := BuildStateDump(bot)
	snap, err := ParseDump(dump)
	require.NoError(t, err)

	require.Len(t, snap.Objects, 1)
	obj := snap.Objects[0]
	require.Equal(t, int32(200), obj.ObjectID)
	require.Equal(t, state.KindNPC, obj.Kind)
	require.Equal(t, "Keltir", obj.Name)
	require.Equal(t, int32(45200), obj.X)
	require.Equal(t, int32(50100), obj.Y)
	require.Equal(t, int32(-3500), obj.Z)
	// The dump prints hp <cur>/<max>; the parser reads both.
	require.InDelta(t, 50, obj.CurHP, 0.5)
	require.InDelta(t, 50, obj.MaxHP, 0.5)
	require.True(t, obj.Attackable)
}

// TestParseDumpEmptyDump pins the tolerant contract: an empty or
// header-only dump returns an empty snapshot without error.
func TestParseDumpEmptyDump(t *testing.T) {
	snap, err := ParseDump("")
	require.NoError(t, err)
	require.Equal(t, state.Snapshot{}, snap)

	snap, err = ParseDump("swarm state dump\nbuild: test\n")
	require.NoError(t, err)
	require.Equal(t, state.Snapshot{}, snap)
}

// TestParseDumpNoWalkPlan pins the "walk plan: none" case: the
// snapshot carries no walk plan and no origin/dest.
func TestParseDumpNoWalkPlan(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	dump := BuildStateDump(bot)
	snap, err := ParseDump(dump)
	require.NoError(t, err)
	require.Empty(t, snap.WalkPath)
	require.Nil(t, snap.WalkOrigin)
	require.Nil(t, snap.WalkDest)
}

// TestParseDumpNoHuntingZone pins the "hunting zone: none" case: the
// snapshot carries no hunting zone.
func TestParseDumpNoHuntingZone(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	dump := BuildStateDump(bot)
	snap, err := ParseDump(dump)
	require.NoError(t, err)
	require.Nil(t, snap.HuntingZone)
}
