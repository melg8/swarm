// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

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

func TestArrivalMovementKeepsHeading(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 0, Y: 0, Z: 0, Name: "Gremlin"})

	// Movement to the south yields heading 90 degrees.
	bot.ApplyMovement(Movement{
		ObjectID: 7, X: 0, Y: 0, Z: 0, DestX: 0, DestY: 100, DestZ: 0,
	})
	snap := bot.Snapshot()
	obj := findObject(snap, 7)
	require.True(t, obj.Moving)
	require.Equal(t, int32(16384), obj.Heading)

	// The server broadcasts the arrival as a zero distance movement.
	// The heading must stay at the movement direction, not reset to east.
	bot.ApplyMovement(Movement{
		ObjectID: 7, X: 0, Y: 100, Z: 0, DestX: 0, DestY: 100, DestZ: 0,
	})
	snap = bot.Snapshot()
	obj = findObject(snap, 7)
	require.False(t, obj.Moving)
	require.Equal(t, int32(16384), obj.Heading)
	require.Equal(t, int32(0), obj.X)
	require.Equal(t, int32(100), obj.Y)
	require.Equal(t, int32(100), obj.DestY)
}

func TestNpcInfoCarriesSpeedAndAggro(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, TemplateID: 1000533, Attackable: true,
		X: 0, Y: 0, Name: "Keltir",
		RunSpeed: 165, WalkSpeed: 55, MoveSpeedMult: 1.2, Running: true,
	})

	snap := bot.Snapshot()
	obj := findObject(snap, 7)
	require.True(t, obj.Running)
	require.InDelta(t, 165*1.2, obj.Speed, 0.001)
	require.False(t, obj.Aggressive)
	require.Equal(t, int32(1000), obj.AggroRange)
	require.Equal(t, int32(2), obj.Level)
	//nolint:testifylint // Positive is unavailable in testify 1.4
	require.True(t, snap.ServerTimeMs > 0)
	//nolint:testifylint // Positive is unavailable in testify 1.4
	require.True(t, obj.MoveAtMs > 0)
}

func TestAttackMarksCombat(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 0, Y: 0, Name: "Gremlin"})

	var targets [4]int32
	targets[0] = 100
	//nolint:exhaustruct // partial attack fields for the case
	bot.ApplyAttack(Attack{
		AttackerID: 7, X: 10, Y: 10, TargetIDs: targets, TargetCount: 1,
	})

	snap := bot.Snapshot()
	obj := findObject(snap, 7)
	require.True(t, obj.InCombat)
	require.Equal(t, int32(100), obj.TargetID)
	require.Equal(t, int32(10), obj.X)
	require.True(t, snap.Character.InCombat)

	// Attacking an unknown object is a no-op.
	//nolint:exhaustruct // attacker id only
	bot.ApplyAttack(Attack{AttackerID: 99, TargetCount: 0})
	require.Len(t, bot.Snapshot().Objects, 1)
}

func TestAutoAttackFlags(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 0, Y: 0, Name: "Gremlin"})

	bot.ApplyAutoAttackStart(7)
	obj := findObject(bot.Snapshot(), 7)
	require.True(t, obj.InCombat)

	bot.ApplyAutoAttackStop(7)
	snap := bot.Snapshot()
	obj = findObject(snap, 7)
	// The combat window keeps the state warm for a moment.
	require.True(t, obj.InCombat)
}

func TestPawnMovementChasesTarget(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 500, 500, 0, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 7, X: 300, Y: 300, Name: "Orc"})

	bot.ApplyPawnMovement(PawnMovement{
		ObjectID: 7, TargetID: 100, Distance: 40,
		X: 300, Y: 300, Z: 0, TargetX: 500, TargetY: 500, TargetZ: 0,
	})

	snap := bot.Snapshot()
	obj := findObject(snap, 7)
	require.True(t, obj.Moving)
	require.True(t, obj.InCombat)
	require.Equal(t, int32(100), obj.TargetID)
	// The destination keeps the stop distance from the target.
	require.InDelta(t, 500-28, float64(obj.DestX), 2)
	require.InDelta(t, 500-28, float64(obj.DestY), 2)
	// The character position is corrected from the target location.
	require.Equal(t, int32(500), snap.Character.X)
	// Facing the target to the south east is 45 degrees.
	require.Equal(t, int32(8192), obj.Heading)
}

func TestStatusUpdateMarksDead(t *testing.T) {
	bot := NewBot("acc1")
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 9, X: 0, Y: 0, Name: "Orc"})
	bot.ApplyStatusUpdate(9, []Attribute{{ID: AttrCurHP, Value: 0}})
	obj := findObject(bot.Snapshot(), 9)
	require.True(t, obj.Dead)

	bot.ApplyStatusUpdate(9, []Attribute{{ID: AttrCurHP, Value: 30}})
	obj = findObject(bot.Snapshot(), 9)
	require.False(t, obj.Dead)
}

func TestEffectiveSpeedFallbacks(t *testing.T) {
	obj := newWorldObject(1, KindNPC)
	require.InDelta(t, defaultRunSpeed, obj.EffectiveSpeed(), 0.001)

	obj.RunSpeed = 200
	obj.WalkSpeed = 60
	obj.Running = false
	obj.MoveSpeedMult = 1.5
	require.InDelta(t, 60*1.5, obj.EffectiveSpeed(), 0.001)

	obj.Running = true
	require.InDelta(t, 200*1.5, obj.EffectiveSpeed(), 0.001)
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

func TestNearestAttackable(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, X: 100, Y: 0, Name: "Gremlin", Attackable: true,
	})
	//nolint:exhaustruct // partial fields for the case
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 8, X: 500, Y: 0, Name: "Far", Attackable: true,
	})
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{ObjectID: 9, X: 50, Y: 0, Name: "Friendly"})
	// Dead mobs are not attackable.
	bot.ApplyStatusUpdate(8, []Attribute{{ID: AttrCurHP, Value: 0}})

	target, ok := bot.NearestAttackable(1500, nil)
	require.True(t, ok)
	require.Equal(t, int32(7), target.ObjectID)
	require.Equal(t, "Gremlin", target.Name)

	_, ok = bot.NearestAttackable(50, nil)
	require.False(t, ok)
}

func TestSelfMovementTracksDestination(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 0, 0, 0, 50, 30)

	bot.ApplyMovement(Movement{
		ObjectID: 100, X: 0, Y: 0, Z: 0, DestX: 300, DestY: 400, DestZ: 0,
	})
	snap := bot.Snapshot()
	require.True(t, snap.Character.Moving)
	require.Equal(t, int32(300), snap.Character.DestX)
	//nolint:testifylint // Positive is unavailable in testify 1.4
	require.True(t, snap.Character.MoveAtMs > 0)

	// Arrival clears the movement.
	bot.ApplyMovement(Movement{
		ObjectID: 100, X: 300, Y: 400, Z: 0, DestX: 300, DestY: 400, DestZ: 0,
	})
	snap = bot.Snapshot()
	require.False(t, snap.Character.Moving)
	require.Equal(t, int32(300), snap.Character.X)
}

func TestNearestAttackableUsesProjectedPosition(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	// A standing mob recorded near the character but the farthest away.
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 7, X: 45300, Y: 50000, Name: "Standing", Attackable: true,
	})

	// A moving mob whose movement packet started 400 units away but ran
	// toward the character: the projection must place it next to the
	// character, the stale packet position must not win.
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 8, X: 46000, Y: 50000, Name: "Runner", Attackable: true,
		RunSpeed: 200,
	})
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyMovement(Movement{
		ObjectID: 8, X: 46000, Y: 50000, DestX: 45050, DestY: 50000,
	})
	runner := bot.objects[8]
	runner.MoveAt = time.Now().Add(-5 * time.Second)
	bot.objects[8] = runner

	target, ok := bot.NearestAttackable(1500, nil)
	require.True(t, ok)
	require.Equal(t, int32(8), target.ObjectID,
		"the mob that ran toward the character is the nearest now")
	require.InDelta(t, float64(45050), float64(target.X), 150,
		"the attack position is the projected current position")
}

func TestSelfHealthPercent(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)

	require.InDelta(t, 100.0, bot.SelfHealthPercent(), 0.001)

	bot.ApplyStatusUpdate(100, []Attribute{
		{ID: stateAttrMaxHPTest, Value: 200},
		{ID: stateAttrCurHPTest, Value: 90},
	})
	require.InDelta(t, 45.0, bot.SelfHealthPercent(), 0.001)
}

// Aliases keep the test table above short without exported constants.
const (
	stateAttrMaxHPTest = 0x0A
	stateAttrCurHPTest = 0x09
)

func TestExpPercent(t *testing.T) {
	// Level 2 base is 68 exp, level 3 base is 363: halfway between
	// them is (68+363)/2 = 215.5 -> 50%.
	require.InDelta(t, 0.0, ExpPercent(2, 68), 0.001)
	require.InDelta(t, 50.0, ExpPercent(2, 215), 0.5)
	require.InDelta(t, 100.0, ExpPercent(2, 363), 0.001)
	require.InDelta(t, 100.0, ExpPercent(2, 1000), 0.001)
	require.Zero(t, ExpPercent(2, 10))
	// Level 1 starts at zero experience.
	require.InDelta(t, 34.0, ExpPercent(1, 23), 0.5)
	// Out of table bounds clamp: no level yet is zero progress, the
	// levels past the table are complete.
	require.InDelta(t, 100.0, ExpPercent(90, 0), 0.001)
	require.Zero(t, ExpPercent(0, 0))
}

func TestSnapshotCarriesExpPercent(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	//nolint:exhaustruct // partial fields for the case
	bot.ApplyUserInfo(UserInfo{Level: 2, Exp: 215})

	snap := bot.Snapshot()
	require.InDelta(t, 50.0, snap.Character.ExpPercent, 0.5)
}
