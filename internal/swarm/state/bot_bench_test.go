// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"
	"time"
)

// Benchmark fixture shape: the character stands in the middle of the
// elven starting zone (the default hunting square) and the surrounding
// world carries the mix a live hunting session observes - clan mobs,
// clanless mobs, corpses, players, two chasing attackers and ground
// loot - so the per packet and per tick paths measure against a
// realistic tracker instead of a nearly empty map.
const (
	benchSelfObjectID = 268473919
	benchSelfX        = 45000
	benchSelfY        = 50000
	benchSelfZ        = -3500
	benchZoneHalf     = 1650
	// benchGoblinTemplate is an attackable GOBLIN clan mob (clan help
	// range 300) of the elven starting grounds.
	benchGoblinTemplate = 1000003
	// benchGremlinTemplate is a clanless attackable starter mob.
	benchGremlinTemplate = 1000001
	// benchOrcArcherTemplate is an attackable ORC clan mob.
	benchOrcArcherTemplate = 1000006
	// benchSwordItem is the Short Sword display id of the item
	// dictionaries.
	benchSwordItem = 1
	// benchAdenaItem is the adena display id.
	benchAdenaItem = 57
)

// benchWorld builds a tracker loaded like a live hunting session with
// npcCount npcs, players, corpses, ground items, two mobs holding the
// character as their target and a 55 slot inventory (the town trip
// trigger point). The spread keeps every object inside the zone square
// through deterministic offsets, so the target searches always see the
// whole population.
func benchWorld(npcCount int) *Bot {
	bot := NewBot("bench")
	bot.SetCharacter("test1", benchSelfObjectID, 18,
		benchSelfX, benchSelfY, benchSelfZ, 100, 50)
	bot.SetHuntingZone(benchSelfX, benchSelfY, benchZoneHalf)
	for i := range npcCount {
		objectID := int32(1_000_000 + i*10)
		x := benchSelfX + int32((i*97)%2200-1100)
		y := benchSelfY + int32((i*131)%2200-1100)
		switch {
		case i%10 == 9:
			bot.ApplyPlayerInfo(benchPlayerInfo(objectID, x, y))
		case i%10 == 8:
			bot.ApplyNpcInfo(benchNpcInfo(objectID, benchGoblinTemplate, x, y))
			bot.ApplyStatusUpdate(objectID, []Attribute{
				{ID: AttrCurHP, Value: 0},
			})
		case i%10 < 2:
			// The attacker slice: the mob chases the character.
			bot.ApplyNpcInfo(benchNpcInfo(objectID, benchOrcArcherTemplate, x, y))
			bot.ApplyPawnMovement(PawnMovement{
				ObjectID: objectID,
				TargetID: benchSelfObjectID,
				Distance: 60,
				X:        x,
				Y:        y,
				Z:        benchSelfZ,
				TargetX:  benchSelfX,
				TargetY:  benchSelfY,
				TargetZ:  benchSelfZ,
			})
		case i%2 == 0:
			bot.ApplyNpcInfo(benchNpcInfo(objectID, benchGoblinTemplate, x, y))
		default:
			bot.ApplyNpcInfo(benchNpcInfo(objectID, benchGremlinTemplate, x, y))
		}
	}
	for i := range 8 {
		bot.ApplyItemInfo(ItemInfo{
			ObjectID:   int32(2_000_000 + i),
			TemplateID: benchSwordItem,
			Stackable:  true,
			Count:      1,
			X:          benchSelfX + int32(i*45),
			Y:          benchSelfY + int32(i*37),
			Z:          benchSelfZ,
		})
	}
	bot.ApplyItemList(benchInventory(55))

	return bot
}

// benchInventory builds an inventory of the given size: one adena
// stack, a few equipped pieces and junk of the lower item ids.
func benchInventory(size int) []InventoryItem {
	items := make([]InventoryItem, 0, size)
	items = append(items, InventoryItem{
		ObjectID: 900_001,
		ItemID:   benchAdenaItem,
		Count:    12345,
		Type1:    0,
		Type2:    itemType2Adena,
		Equipped: false,
		BodyPart: 0,
		Enchant:  0,
		Change:   0,
	})
	for i := range size - 1 {
		items = append(items, InventoryItem{
			ObjectID: int32(900_002 + i),
			ItemID:   int32(i%80) + 1,
			Count:    1,
			Type1:    0,
			Type2:    0,
			Equipped: i < 6,
			BodyPart: 0,
			Enchant:  0,
			Change:   0,
		})
	}

	return items
}

// benchNpcInfo builds a spawn packet view of an attackable mob.
func benchNpcInfo(objectID int32, templateID int32, x int32, y int32) NpcInfo {
	return NpcInfo{
		ObjectID:        objectID,
		TemplateID:      templateID,
		Attackable:      true,
		X:               x,
		Y:               y,
		Z:               benchSelfZ,
		Heading:         16384,
		RunSpeed:        165,
		WalkSpeed:       55,
		MoveSpeedMult:   1.15,
		CollisionRadius: 10,
		Running:         true,
		InCombat:        false,
		Dead:            false,
		Name:            "",
		Title:           "",
	}
}

// benchPlayerInfo builds a CharInfo view of another player.
func benchPlayerInfo(objectID int32, x int32, y int32) PlayerInfo {
	return PlayerInfo{
		ObjectID:        objectID,
		Name:            "player" + string('A'+objectID%26),
		Title:           "",
		Race:            1,
		ClassID:         18,
		RunSpeed:        140,
		WalkSpeed:       70,
		MoveSpeedMult:   1,
		CollisionRadius: 9,
		Running:         true,
		Dead:            false,
		InCombat:        false,
		X:               x,
		Y:               y,
		Z:               benchSelfZ,
	}
}

// BenchmarkApplyNpcInfo measures the npc spawn hot path: the npcdata
// dictionary lookups, the map upsert and the event ring write run for
// every NpcInfo the server broadcasts.
func BenchmarkApplyNpcInfo(b *testing.B) {
	bot := benchWorld(200)
	info := benchNpcInfo(1_000_500, benchGoblinTemplate,
		benchSelfX+300, benchSelfY+400)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bot.ApplyNpcInfo(info)
	}
}

// BenchmarkApplyMovement measures the movement broadcast path: the map
// read, the struct copy updates and the map write back per packet.
func BenchmarkApplyMovement(b *testing.B) {
	bot := benchWorld(200)
	move := Movement{
		ObjectID: 1_000_200,
		X:        benchSelfX + 100,
		Y:        benchSelfY + 100,
		Z:        benchSelfZ,
		DestX:    benchSelfX + 600,
		DestY:    benchSelfY + 700,
		DestZ:    benchSelfZ,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bot.ApplyMovement(move)
	}
}

// BenchmarkApplyStatusUpdate measures the vitals packet path: the
// attribute apply loop with the combat damage event recording.
func BenchmarkApplyStatusUpdate(b *testing.B) {
	bot := benchWorld(200)
	attrs := []Attribute{
		{ID: AttrCurHP, Value: 80},
		{ID: AttrMaxHP, Value: 100},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bot.ApplyStatusUpdate(1_000_200, attrs)
	}
}

// BenchmarkSelfAttackerCount measures the per tick aggro load scan of
// the emergency logout gate.
func BenchmarkSelfAttackerCount(b *testing.B) {
	bot := benchWorld(200)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if count := bot.SelfAttackerCount(); count < 2 {
			b.Fatalf("expected 2 attackers, got %d", count)
		}
	}
}

// BenchmarkNearestAttackable measures the plain target search of the
// engage loop: a full scan of the world map with the projected
// positions.
func BenchmarkNearestAttackable(b *testing.B) {
	for _, size := range []int{50, 200} {
		b.Run(mapSizeLabel(size), func(b *testing.B) {
			bot := benchWorld(size)
			zone := Zone{
				CX:   benchSelfX,
				CY:   benchSelfY,
				Half: benchZoneHalf,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, ok := bot.NearestAttackable(1500, &zone); !ok {
					b.Fatal("expected a target")
				}
			}
		})
	}
}

// BenchmarkNearestAttackableConstrained measures the safety constrained
// target search of the engage loop: the level filter plus the social
// clan check that rescans the whole world per candidate.
func BenchmarkNearestAttackableConstrained(b *testing.B) {
	for _, size := range []int{50, 200} {
		b.Run(mapSizeLabel(size), func(b *testing.B) {
			bot := benchWorld(size)
			zone := Zone{
				CX:   benchSelfX,
				CY:   benchSelfY,
				Half: benchZoneHalf,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _ = bot.NearestAttackableConstrained(
					1500, &zone, nil, 20, true)
			}
		})
	}
}

// BenchmarkZoneHasAttackable measures the zone emptiness reading the
// rotation timer polls every ten seconds.
func BenchmarkZoneHasAttackable(b *testing.B) {
	bot := benchWorld(200)
	zone := Zone{
		CX:   benchSelfX,
		CY:   benchSelfY,
		Half: benchZoneHalf,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if !bot.ZoneHasAttackable(&zone) {
			b.Fatal("expected attackable mobs in the zone")
		}
	}
}

// BenchmarkNearestGroundItemExcluding measures the loot search the hunt
// loop runs on every tick.
func BenchmarkNearestGroundItemExcluding(b *testing.B) {
	bot := benchWorld(200)
	zone := Zone{
		CX:   benchSelfX,
		CY:   benchSelfY,
		Half: benchZoneHalf,
	}
	skipped := map[int32]time.Time{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := bot.NearestGroundItemExcluding(300, skipped, &zone); !ok {
			b.Fatal("expected a ground item")
		}
	}
}

// BenchmarkMedianZoneMobLevel measures the delevel trigger statistic:
// the full scan plus the median sort of the mob levels.
func BenchmarkMedianZoneMobLevel(b *testing.B) {
	bot := benchWorld(200)
	zone := Zone{
		CX:   benchSelfX,
		CY:   benchSelfY,
		Half: benchZoneHalf,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = bot.MedianZoneMobLevel(&zone)
	}
}

// BenchmarkSnapshot measures the full web view copy: the object list,
// the inventory with the name and icon resolutions and the ring
// buffers, once per SSE snapshot poll.
func BenchmarkSnapshot(b *testing.B) {
	for _, size := range []int{50, 200} {
		b.Run(mapSizeLabel(size), func(b *testing.B) {
			bot := benchWorld(size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				snap := bot.Snapshot()
				if len(snap.Objects) == 0 {
					b.Fatal("expected objects in the snapshot")
				}
			}
		})
	}
}

// mapSizeLabel names the sub benchmark of a world size.
func mapSizeLabel(size int) string {
	switch size {
	case 50:
		return "npcs=50"
	case 200:
		return "npcs=200"
	default:
		return "npcs"
	}
}
