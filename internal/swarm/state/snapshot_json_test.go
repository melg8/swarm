// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// plainSnapshot drops the MarshalJSON method for the reflection
// reference: the defined type inherits the fields but not the method
// set, so json.Marshal walks it with the generic encoder. The golden
// test pins the hand rolled writer to produce exactly its bytes.
type plainSnapshot Snapshot

// goldenSnapshot builds a snapshot exercising every encoding branch:
// the string edge cases (HTML characters, quotes, control bytes, the
// Unicode line separators, invalid UTF-8), the float formats, the
// zero and loaded times, the nil and populated slices and the nil
// and live zone pointer.
func goldenSnapshot() Snapshot {
	moment := time.Date(2026, 9, 8, 12, 34, 56, 123456789,
		time.FixedZone("bench", 3*3600+7*60))

	return Snapshot{
		ID:     "acc<&>\"\\µ1",
		Status: StatusOnline,
		Phase:  "engage",
		Character: CharacterSnapshot{
			ObjectID:        268473919,
			Name:            "test\u2028line\u2029sep",
			TargetID:        42,
			Moving:          true,
			DestX:           45001,
			DestY:           -50002,
			DestZ:           35003,
			Speed:           1e-7,
			CollisionRadius: 1e21,
			SocialUntilMs:   1725800496123,
			MoveAtMs:        1725800496456,
			Level:           9,
			Race:            1,
			ClassID:         18,
			X:               45000,
			Y:               50000,
			Z:               -3500,
			Heading:         65535,
			CurHP:           87.5,
			MaxHP:           100,
			CurMP:           0.001,
			MaxMP:           -0.1,
			Sitting:         true,
			STR:             36,
			DEX:             35,
			CON:             36,
			INT:             23,
			WIT:             14,
			MEN:             25,
			Exp:             48229,
			ExpPercent:      12.34,
			Sp:              122,
			InCombat:        true,
			CurrentLoad:     12345,
			MaxLoad:         65536,
			InventorySlots:  55,
			InventoryMax:    80,
			Adena:           987654,
		},
		Inventory: []InventoryItemSnapshot{
			{
				ObjectID:    900001,
				ItemID:      57,
				Count:       12345,
				Type2:       4,
				Equipped:    false,
				BodyPart:    0,
				Enchant:     0,
				Name:        "Adena",
				Icon:        "",
				Type:        "EtcItem",
				WeaponType:  "",
				ArmorType:   "",
				BodyPartKey: "",
				PAtk:        0,
				MAtk:        0,
				PDef:        0,
				MDef:        0,
				SDef:        0,
				RShld:       0,
				PAtkSpd:     0,
				SoulShots:   0,
				SpiritShots: 0,
				Weight:      0,
				Price:       0,
			},
			{
				ObjectID:    900002,
				ItemID:      1,
				Count:       1,
				Type2:       0,
				Equipped:    true,
				BodyPart:    128,
				Enchant:     3,
				Name:        "Short Sword \x01\x1f\x7f",
				Icon:        "weapon_small_sword_i00",
				Type:        "Weapon",
				WeaponType:  "SWORD",
				ArmorType:   "",
				BodyPartKey: "rhand",
				PAtk:        8,
				MAtk:        6,
				PDef:        0,
				MDef:        0,
				SDef:        0,
				RShld:       0,
				PAtkSpd:     379,
				SoulShots:   1,
				SpiritShots: 1,
				Weight:      1600,
				Price:       768,
			},
		},
		Objects: []ObjectSnapshot{
			{
				ObjectID:        1000000,
				Kind:            KindNPC,
				Name:            "Keltiré",
				Title:           "Lv 1",
				TemplateID:      1001277,
				Attackable:      true,
				Aggressive:      false,
				AggroRange:      300,
				Level:           1,
				TargetID:        268473919,
				InCombat:        true,
				Dead:            false,
				Moving:          true,
				Running:         true,
				Speed:           189.75,
				CollisionRadius: 10,
				SocialUntilMs:   1725800496000,
				Count:           1,
				X:               45300,
				Y:               50400,
				Z:               -3500,
				Heading:         16384,
				DestX:           45400,
				DestY:           50500,
				DestZ:           -3500,
				MoveAtMs:        1725800496500,
				CurHP:           42,
				MaxHP:           42,
				CurMP:           0,
				MaxMP:           0,
			},
			{
				ObjectID: 2000000,
				Kind:     KindItem,
				Name:     "\xff\xfe",
			},
		},
		Events: []Event{
			{Time: moment, Message: "npc spawned: Keltir"},
			{Time: time.Time{}, Message: "object removed: \x00"},
		},
		Chat: []ChatEvent{
			{Time: moment, Kind: "system", Text: "You earned 42 adena."},
		},
		WalkPath:   []WalkPoint{{X: 45100, Y: 50100, Z: -3500}},
		WalkOrigin: &WalkPoint{X: 45000, Y: 50000, Z: -3500},
		WalkIndex:  0,
		WalkDest:   &WalkPoint{X: 45110, Y: 50110, Z: -3500},
		Shopping: &ShoppingPlanView{
			Entries: []ShoppingEntryView{
				{
					ItemID:      1121,
					Name:        "Apprentice's Shoes",
					Icon:        "armor_t01_b_i00",
					MerchantID:  7148,
					Merchant:    "Ariel",
					Type:        "Armor",
					WeaponType:  "",
					ArmorType:   "LIGHT",
					BodyPartKey: "feet",
					PAtk:        0,
					MAtk:        0,
					PDef:        8,
					MDef:        0,
					SDef:        0,
					RShld:       0,
					PAtkSpd:     0,
					SoulShots:   0,
					SpiritShots: 0,
					Weight:      210,
					Price:       9,
					SellCredit:  0,
					Missing:     0,
					Gain:        8,
					Affordable:  true,
					Buying:      false,
					Reason:      "buying Apprentice's Shoes (+8 for 9 adena)",
				},
				{
					ItemID:      1,
					Name:        "Short Sword",
					Icon:        "weapon_small_sword_i00",
					MerchantID:  7147,
					Merchant:    "Unoren",
					Type:        "Weapon",
					WeaponType:  "SWORD",
					ArmorType:   "",
					BodyPartKey: "rhand",
					PAtk:        8,
					MAtk:        0,
					PDef:        0,
					MDef:        0,
					SDef:        0,
					RShld:       0,
					PAtkSpd:     0,
					SoulShots:   0,
					SpiritShots: 0,
					Weight:      1600,
					Price:       883,
					SellCredit:  0,
					Missing:     383,
					Gain:        3.5,
					Affordable:  false,
					Buying:      false,
					Reason:      "buying Short Sword (+3.5 for 883 adena)",
				},
			},
			Adena: 500,
			Total: 9,
			Trip:  false,
		},
		CombatEvents: []CombatEventView{
			{
				Seq:        7,
				Kind:       CombatEventAttack,
				AttackerID: 1000000,
				TargetID:   268473919,
				Amount:     0,
				AtMs:       1725800496700,
				X:          45300,
				Y:          50400,
				TargetX:    45000,
				TargetY:    50000,
			},
		},
		HuntingZone: &Zone{CX: 45000, CY: 50000, Half: 1650},
		HuntingZones: []ZoneView{
			{
				ID:       "elven-gremlin-hollow",
				Name:     "Gremlin Hollow",
				Region:   "elven",
				MinLevel: 1,
				MaxLevel: 3,
				MinGear:  0,
				CX:       45563,
				CY:       49930,
				Half:     1100,
				Active:   true,
				Deaths:   2,
				Demoted:  false,
			},
		},
		Packets:      123456,
		Version:      987654321,
		ServerTimeMs: 1725800496999,
		StartedAt:    moment,
		UpdatedAt:    moment,
	}
}

// TestSnapshotJSONMatchesReflection pins the hand rolled snapshot
// writer to the reflection encoder: byte identical output for the
// golden fixture that covers every field, escape and number format.
func TestSnapshotJSONMatchesReflection(t *testing.T) {
	snapshot := goldenSnapshot()
	handRolled, err := json.Marshal(snapshot)
	require.NoError(t, err)
	reflected, err := json.Marshal(plainSnapshot(snapshot))
	require.NoError(t, err)
	require.Equal(t, string(reflected), string(handRolled))
}

// TestSnapshotJSONNilSlicesMatchReflection pins the nil slice and nil
// pointer encodings (null against the populated arrays).
func TestSnapshotJSONNilSlicesMatchReflection(t *testing.T) {
	snapshot := goldenSnapshot()
	snapshot.Inventory = nil
	snapshot.Objects = nil
	snapshot.Events = nil
	snapshot.Chat = nil
	snapshot.WalkPath = nil
	snapshot.Shopping = nil
	snapshot.CombatEvents = nil
	snapshot.HuntingZone = nil
	snapshot.HuntingZones = nil
	handRolled, err := json.Marshal(snapshot)
	require.NoError(t, err)
	reflected, err := json.Marshal(plainSnapshot(snapshot))
	require.NoError(t, err)
	require.Equal(t, string(reflected), string(handRolled))
}

// TestSnapshotJSONEmptySlicesMatchReflection pins the empty non nil
// slices (they encode as empty arrays, not null).
func TestSnapshotJSONEmptySlicesMatchReflection(t *testing.T) {
	snapshot := goldenSnapshot()
	snapshot.Inventory = []InventoryItemSnapshot{}
	snapshot.Objects = []ObjectSnapshot{}
	snapshot.Events = []Event{}
	snapshot.Chat = []ChatEvent{}
	snapshot.WalkPath = []WalkPoint{}
	snapshot.CombatEvents = []CombatEventView{}
	snapshot.HuntingZones = []ZoneView{}
	handRolled, err := json.Marshal(snapshot)
	require.NoError(t, err)
	reflected, err := json.Marshal(plainSnapshot(snapshot))
	require.NoError(t, err)
	require.Equal(t, string(reflected), string(handRolled))
}

// TestSnapshotJSONRoundTrip decodes the hand rolled output back into
// the struct and compares the deep copy: the stream consumers parse
// the payload with a standard decoder.
func TestSnapshotJSONRoundTrip(t *testing.T) {
	snapshot := goldenSnapshot()
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var decoded Snapshot
	require.NoError(t, json.Unmarshal(data, &decoded))
	decodedRaw, err := json.Marshal(plainSnapshot(decoded))
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(decodedRaw))
}

// TestAppendJSONFloatTable pins the number formats of the float
// writer against the reflection encoder.
func TestAppendJSONFloatTable(t *testing.T) {
	values := []float64{
		0, 1, -1, 0.5, -0.5, 87.5, 0.001, 1e-7, -1e-7, 1e21, 123.456,
		42, 100, 1e-6, 9.999999e-7, 1.5e21, 3.141592653589793,
	}
	for _, value := range values {
		handRolled := appendJSONFloat(nil, value)
		reflected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(reflected), string(handRolled),
			"value %v", value)
	}
}

// TestAppendJSONStringTable pins the string escaping of the string
// writer against the reflection encoder.
func TestAppendJSONStringTable(t *testing.T) {
	values := []string{
		"", "plain", `quote"inside`, `back\slash`, "<html>&</html>",
		"line\nbreak", "carriage\rreturn", "tab\tsep", "\x00\x01\x1f",
		"\x7f", "unicode éµ漢", "sep  end", "\xff\xfe", "mix\"\\\n<&>",
	}
	for _, value := range values {
		handRolled := appendJSONString(nil, value)
		reflected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(reflected), string(handRolled),
			"value %q", value)
	}
}

// TestAppendJSONStringInvalidUTF8Modes pins both replacement forms
// the writer may emit for invalid UTF-8 bytes: the \ufffd escape
// sequence of the classic encoder and the literal U+FFFD replacement
// rune of the v2 backed one. The active form follows the stdlib of
// the running toolchain (the probe behind
// jsonInvalidUTF8Replacement); the test also pins the other branch so
// a toolchain switch flips a loud test instead of a silent byte
// drift.
func TestAppendJSONStringInvalidUTF8Modes(t *testing.T) {
	const invalid = "\xff\xfe"
	escapeForm := "\"\\ufffd\\ufffd\""
	rawForm := "\"\uFFFD\uFFFD\""

	active := string(appendJSONString(nil, invalid))
	reflected, err := json.Marshal(invalid)
	require.NoError(t, err)
	require.Equal(t, string(reflected), active)
	require.Contains(t, []string{escapeForm, rawForm}, active)

	saved := jsonInvalidUTF8Replacement
	defer func() { jsonInvalidUTF8Replacement = saved }()

	if active == escapeForm {
		jsonInvalidUTF8Replacement = []byte("\uFFFD")
		require.Equal(t, rawForm, string(appendJSONString(nil, invalid)))
	} else {
		jsonInvalidUTF8Replacement = []byte(`\ufffd`)
		require.Equal(t, escapeForm, string(appendJSONString(nil, invalid)))
	}
}

// TestAppendJSONTimeTable pins the time encoding against the
// reflection encoder.
func TestAppendJSONTimeTable(t *testing.T) {
	utc := time.Date(2026, 9, 8, 12, 34, 56, 123456789, time.UTC)
	offset := time.FixedZone("+3", 3*3600)
	zero := time.Time{}
	values := []time.Time{
		zero, utc, time.Now(),
		time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		time.Date(2026, 9, 8, 12, 34, 56, 987, offset),
	}
	for _, value := range values {
		handRolled := appendJSONTime(nil, value)
		reflected, err := json.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, string(reflected), string(handRolled),
			"value %v", value)
	}
}
