// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// liveSnapshotBot builds a bot whose state exercises every section of
// the snapshot encoding: character with vitals and movement, a mixed
// inventory with equipped gear and adena, npcs in and out of combat,
// a dead one, a moving one, a ground item, a filled event ring (over
// the snapshot limit), chat lines, live combat beats of the TTL
// window, a walk plan, the active hunting zone and the zone registry.
func liveSnapshotBot(t *testing.T) *Bot {
	t.Helper()
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 268473919, 18, 45000, 50000, -3500, 100, 50)
	bot.SetOnline("test1")
	bot.SetPhase("engage")
	bot.SetHuntingZone(45000, 50000, 1650)
	bot.SetHuntingZones([]ZoneView{
		{
			ID: "z1", Name: "Elven Ruins", Region: "elven",
			MinLevel: 1, MaxLevel: 10, MinGear: 0,
			CX: 45000, CY: 50000, Half: 1650,
			Active: true, Deaths: 2, Demoted: false,
		},
		{
			ID: "z2", Name: "Swamp", Region: "orc",
			MinLevel: 8, MaxLevel: 16, MinGear: 40,
			CX: 44000, CY: 51000, Half: 900,
			Active: false, Deaths: 0, Demoted: true,
		},
	})

	for i := range 40 {
		info := NpcInfo{
			ObjectID:      int32(1_000_000 + i),
			TemplateID:    1000003,
			Attackable:    true,
			X:             45000 + int32(i*37-700),
			Y:             50000 + int32(i*53-1000),
			Z:             -3500 + int32(i%5),
			Heading:       int32(i * 1600),
			RunSpeed:      165,
			WalkSpeed:     55,
			MoveSpeedMult: 1.15,
			Running:       true,
			InCombat:      i%3 == 0,
			Dead:          i == 39,
			Name:          "Orc Archer",
			Title:         "Guard",
		}
		if i == 7 {
			info.Attackable = false
			info.Name = "Roxxy"
			info.Title = "Merchant"
		}
		bot.ApplyNpcInfo(info)
		if i%4 == 0 {
			bot.ApplyMovement(Movement{
				ObjectID: info.ObjectID,
				X:        info.X, Y: info.Y, Z: info.Z,
				DestX: info.X + 300, DestY: info.Y - 200, DestZ: info.Z,
			})
		}
	}
	bot.ApplyItemInfo(ItemInfo{
		ObjectID: 500_001, TemplateID: 57, Count: 12,
		X: 45100, Y: 50100, Z: -3500,
	})

	items := make([]InventoryItem, 0, 20)
	for i := range 20 {
		items = append(items, InventoryItem{
			ObjectID: int32(900_001 + i),
			ItemID:   int32(i%15) + 1,
			Count:    int32(i%3) + 1,
			Type1:    0,
			Type2:    int16(i % 6),
			Equipped: i < 4,
			BodyPart: int32(i * 64),
			Enchant:  int16(i % 4),
			Change:   0,
		})
	}
	items = append(items, InventoryItem{
		ObjectID: 999_999, ItemID: 57, Count: 12345, Type2: 4, Change: 0,
	})
	bot.ApplyItemList(items)

	for i := range 150 {
		bot.RecordEvent("event line " + itoa(i))
	}
	bot.ApplySystemMessage(SystemMessage{
		ID:     28,
		Params: []ChatMessageParam{{Type: 1, Int: 25}},
	})
	bot.ApplySystemMessage(SystemMessage{ID: 99999})

	bot.ApplyAttack(Attack{
		AttackerID:  1_000_005,
		TargetIDs:   [AttackTargets]int32{268473919},
		TargetCount: 1,
		X:           45100, Y: 50100,
		TargetX: 45000, TargetY: 50000,
	})
	bot.ApplyStatusUpdate(1_000_009, []Attribute{
		{ID: AttrCurHP, Value: 40},
		{ID: AttrMaxHP, Value: 95},
	})
	bot.SetWalkPlan([]WalkPoint{
		{X: 45010, Y: 50010, Z: -3500},
		{X: 45020, Y: 50025, Z: -3498},
		{X: 45040, Y: 50044, Z: -3497},
	})

	return bot
}

// ittoa formats small numbers without pulling strconv into the test
// helper path (the message strings only need to be distinct).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}

	return string(digits)
}

// TestAppendSnapshotJSONMatchesSnapshot pins the byte equality of the
// direct live state encoding and the snapshot copy encoding. Both
// encode a serverTimeMs field stamped at their own time.Now(), so the
// comparison retries until both calls land in the same millisecond
// (the content difference of a boundary crossing is exactly that one
// field).
func TestAppendSnapshotJSONMatchesSnapshot(t *testing.T) {
	bot := liveSnapshotBot(t)

	var direct, viaCopy []byte
	for range 200 {
		direct = bot.AppendSnapshotJSON(nil)
		viaCopy = bot.Snapshot().AppendJSON(nil)
		if bytes.Equal(direct, viaCopy) {
			return
		}
	}

	t.Fatalf("live encode and snapshot encode differ:\nlive: %s\ncopy: %s",
		direct, viaCopy)
}

// TestAppendSnapshotJSONEmptyBot pins the empty state shape: the
// always present collections encode as empty arrays, the optional
// walk plan and hunting zone fall back to null.
func TestAppendSnapshotJSONEmptyBot(t *testing.T) {
	bot := NewBot("acc1")

	direct := bot.AppendSnapshotJSON(nil)
	require.Contains(t, string(direct), `"objects":[]`)
	require.Contains(t, string(direct), `"inventory":[]`)
	require.Contains(t, string(direct), `"events":[]`)
	require.Contains(t, string(direct), `"chat":[]`)
	require.Contains(t, string(direct), `"combatEvents":[]`)
	require.Contains(t, string(direct), `"huntingZones":[]`)
	require.Contains(t, string(direct), `"walkPath":null`)
	require.Contains(t, string(direct), `"huntingZone":null`)

	viaCopy := bot.Snapshot().AppendJSON(nil)
	// The serverTimeMs of the two encodes reads the clock
	// independently: a millisecond rollover between the calls is not
	// a state difference, the field stays out of the comparison.
	serverTime := regexp.MustCompile(`"serverTimeMs":\d+,`)
	require.Equal(t,
		serverTime.ReplaceAllString(string(viaCopy), ""),
		serverTime.ReplaceAllString(string(direct), ""))
}

// TestAppendSnapshotJSONExpiredWalkPlan pins the walk plan TTL gate:
// a stale plan encodes as null on both paths.
func TestAppendSnapshotJSONExpiredWalkPlan(t *testing.T) {
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 100, 50)
	bot.SetWalkPlan([]WalkPoint{{X: 1, Y: 2, Z: 3}})

	bot.mu.Lock()
	bot.walkPathAt = time.Now().Add(-2 * walkPlanTTL)
	bot.mu.Unlock()

	require.Contains(t, string(bot.AppendSnapshotJSON(nil)), `"walkPath":null`)
	require.Contains(t, string(bot.Snapshot().AppendJSON(nil)),
		`"walkPath":null`)
}
