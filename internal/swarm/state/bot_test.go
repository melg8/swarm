// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBotStartsConnecting(t *testing.T) {
	bot := NewBot("acc1")
	require.Equal(t, "acc1", bot.ID())
	require.Equal(t, StatusConnecting, bot.Status())
	require.Equal(t, uint64(0), bot.Version())
}

func TestSetCharacterAndPlacement(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.SetOnline("test1")

	// The self placement corrects position and heading.
	//nolint:exhaustruct // partial placement
	bot.ApplyPlacement(Placement{
		ObjectID: 100, X: 45100, Y: 50100, Z: -3500, Heading: 16384,
	})

	snap := bot.Snapshot()
	require.Equal(t, StatusOnline, snap.Status)
	require.Equal(t, "test1", snap.Character.Name)
	require.Equal(t, int32(45100), snap.Character.X)
	require.Equal(t, int32(50100), snap.Character.Y)
	require.Equal(t, int32(16384), snap.Character.Heading)
	require.InDelta(t, 50, snap.Character.CurHP, 0.001)
}

func TestApplyNpcInfoUpsert(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 1, TemplateID: 1001277, Attackable: true,
		X: 100, Y: 200, Z: -3000, Heading: 8192,
		Name: "Keltir", Title: "",
	})

	// Respawn of the same object updates the position.
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 1, TemplateID: 1001277, Attackable: true,
		X: 150, Y: 260, Z: -3000, Heading: 4096,
		Name: "Keltir", Title: "",
	})

	snap := bot.Snapshot()
	require.Len(t, snap.Objects, 1)
	obj := snap.Objects[0]
	require.Equal(t, KindNPC, obj.Kind)
	require.Equal(t, int32(150), obj.X)
	require.Equal(t, int32(4096), obj.Heading)
	require.True(t, obj.Attackable)
	require.Equal(t, "Keltir", obj.Name)
}

func TestMovementUpdatesHeadingAndDestination(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 0, Y: 0, Z: 0, Name: "Gremlin"})

	// Movement to the south east (dx=1, dy=1) yields heading 45 degrees.
	bot.ApplyMovement(Movement{
		ObjectID: 7, X: 0, Y: 0, Z: 0, DestX: 100, DestY: 100, DestZ: 0,
	})

	snap := bot.Snapshot()
	obj := findObject(snap, 7)
	require.NotNil(t, obj)
	require.True(t, obj.Moving)
	require.Equal(t, int32(100), obj.DestX)
	require.Equal(t, int32(8192), obj.Heading)

	// Stopping the movement clears the moving flag.
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyPlacement(Placement{
		ObjectID: 7, X: 100, Y: 100, Z: 0, Heading: 0,
	})
	snap = bot.Snapshot()
	obj = findObject(snap, 7)
	require.False(t, obj.Moving)
}

func TestSelfMovementUpdatesCharacter(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)

	bot.ApplyMovement(Movement{
		ObjectID: 100, X: 0, Y: 0, Z: 0, DestX: 0, DestY: 100, DestZ: 0,
	})

	snap := bot.Snapshot()
	// Pure south movement is 90 degrees.
	require.Equal(t, int32(16384), snap.Character.Heading)
	require.Empty(t, snap.Objects)
}

func TestRemoveObject(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 1, X: 0, Y: 0, Name: "Keltir"})
	bot.RemoveObject(1)
	bot.RemoveObject(1) // second delete is a no-op

	snap := bot.Snapshot()
	require.Empty(t, snap.Objects)
}

func TestStatusUpdateAppliesVitals(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // vitals fields only
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 3, MaxHP: 90, CurHP: 90, MaxMP: 40, CurMP: 40,
	})

	bot.ApplyStatusUpdate(100, []Attribute{
		{ID: AttrCurHP, Value: 70},
		{ID: AttrCurMP, Value: 25},
	})

	snap := bot.Snapshot()
	require.InDelta(t, 70, snap.Character.CurHP, 0.001)
	require.InDelta(t, 25, snap.Character.CurMP, 0.001)
	require.Equal(t, int32(3), snap.Character.Level)
}

func TestStatusUpdateOfObjectHP(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 9, X: 0, Y: 0, Name: "Orc"})
	bot.ApplyStatusUpdate(9, []Attribute{
		{ID: AttrMaxHP, Value: 120},
		{ID: AttrCurHP, Value: 45},
	})

	snap := bot.Snapshot()
	obj := findObject(snap, 9)
	require.InDelta(t, 45, obj.CurHP, 0.001)
	require.InDelta(t, 120, obj.MaxHP, 0.001)
}

func TestEventRingBuffer(t *testing.T) {
	bot := NewBot("acc1")
	for i := range eventCapacity + 10 {
		bot.RecordEvent(itoaInt(i))
	}

	snap := bot.Snapshot()
	require.Len(t, snap.Events, snapshotEvents)
	// The snapshot carries the newest events only: with 522 recorded the
	// first one is number 522-100.
	require.Equal(t, "422", snap.Events[0].Message)
	require.Equal(t,
		itoaInt(eventCapacity+9), snap.Events[len(snap.Events)-1].Message)
}

func itoaInt(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	return string(digits)
}

func TestSnapshotJSONShape(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 1, 2, 3, 50, 30)
	bot.SetOnline("test1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 1, X: 10, Y: 20, Attackable: true, Name: "Keltir",
	})

	data, err := json.Marshal(bot.Snapshot())
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, "acc1", decoded["id"])
	require.Equal(t, "online", decoded["status"])
	char, ok := decoded["character"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "test1", char["name"])
	objects, ok := decoded["objects"].([]any)
	require.True(t, ok)
	require.Len(t, objects, 1)
}

func TestVersionBumpsOnChanges(t *testing.T) {
	bot := NewBot("acc1")
	before := bot.Version()

	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 1, Name: "Keltir"})
	require.Greater(t, bot.Version(), before)

	// Unknown object placement does not bump the version.
	same := bot.Version()
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyPlacement(Placement{ObjectID: 42, X: 1, Y: 1, Heading: 1})
	require.Equal(t, same, bot.Version())
}

func TestBotConcurrentAccess(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			//nolint:exhaustruct // partial fields for the case
			bot.ApplyNpcInfo(NpcInfo{ObjectID: id, Name: "mob", X: id, Y: id})
			bot.Snapshot()
			bot.Version()
		}(int32(i)) //nolint:gosec // small loop counter
	}
	wg.Wait()

	require.Len(t, bot.Snapshot().Objects, 8)
}

func TestHeadingFromDelta(t *testing.T) {
	require.Equal(t, int32(0), HeadingFromDelta(100, 0))
	require.Equal(t, int32(16384), HeadingFromDelta(0, 100))
	require.Equal(t, int32(32768), HeadingFromDelta(-100, 0))
	require.Equal(t, int32(49152), HeadingFromDelta(0, -100))
}

func findObject(snap Snapshot, objectID int32) *ObjectSnapshot {
	for i := range snap.Objects {
		if snap.Objects[i].ObjectID == objectID {
			return &snap.Objects[i]
		}
	}

	return nil
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	botA := NewBot("a")
	botB := NewBot("b")
	registry.Add(botA)
	registry.Add(botB)

	found, ok := registry.Get("a")
	require.True(t, ok)
	require.Same(t, botA, found)

	missing, ok := registry.Get("c")
	require.False(t, ok)
	require.Nil(t, missing)

	infos := registry.List()
	require.Len(t, infos, 2)
	require.Equal(t, "a", infos[0].ID)
	require.Equal(t, "b", infos[1].ID)

	// Replacing a bot keeps the order.
	botA2 := NewBot("a")
	registry.Add(botA2)
	infos = registry.List()
	require.Len(t, infos, 2)
	require.Same(t, botA2, registry.mustGet("a"))
}

func TestBotInfo(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // level update only
	bot.ApplyUserInfo(UserInfo{Name: "test1", Level: 5})
	bot.SetOnline("test1")

	info := bot.Info()
	require.Equal(t, "acc1", info.ID)
	require.Equal(t, "test1", info.Name)
	require.Equal(t, StatusOnline, info.Status)
	require.Equal(t, int32(5), info.Level)
}

func TestSetOffline(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetOnline("test1")
	bot.SetOffline()
	require.Equal(t, StatusOffline, bot.Status())
}
