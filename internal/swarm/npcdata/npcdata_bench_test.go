// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import "testing"

// The npc lookups run six times per NpcInfo packet (the spawn hot path
// of the state tracker) and the item lookups run per inventory entry
// on every web snapshot, so the dictionary access cost is part of the
// per packet budget. The benchmarks use goblin (a clanned mob) and
// the Short Sword (an equipped weapon id).
const (
	benchGoblinTemplate = 1000003
	benchSwordItem      = 1
	benchAdenaItem      = 57
)

// BenchmarkNPCName measures the npc name dictionary lookup.
func BenchmarkNPCName(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if NPCName(benchGoblinTemplate) == "" {
			b.Fatal("expected a resolved name")
		}
	}
}

// BenchmarkNPCLevel measures the npc level dictionary lookup.
func BenchmarkNPCLevel(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if NPCLevel(benchGoblinTemplate) == 0 {
			b.Fatal("expected a resolved level")
		}
	}
}

// BenchmarkNPCIsAggressive measures the npc ai flag dictionary lookup.
func BenchmarkNPCIsAggressive(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = NPCIsAggressive(benchGoblinTemplate)
	}
}

// BenchmarkNPCAggroRange measures the npc aggro range dictionary lookup.
func BenchmarkNPCAggroRange(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = NPCAggroRange(benchGoblinTemplate)
	}
}

// BenchmarkNPCClanHelpRange measures the clan help range dictionary
// lookup.
func BenchmarkNPCClanHelpRange(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = NPCClanHelpRange(benchGoblinTemplate)
	}
}

// BenchmarkNPCClans measures the clan split of the npc dictionary: the
// strings.Split allocation runs per NpcInfo packet.
func BenchmarkNPCClans(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if clans := NPCClans(benchGoblinTemplate); len(clans) == 0 {
			b.Fatal("expected clans")
		}
	}
}

// BenchmarkItemName measures the item name dictionary lookup.
func BenchmarkItemName(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if ItemName(benchSwordItem) == "" {
			b.Fatal("expected a resolved name")
		}
	}
}

// BenchmarkItemIcon measures the item icon dictionary lookup.
func BenchmarkItemIcon(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if ItemIcon(benchSwordItem) == "" {
			b.Fatal("expected a resolved icon")
		}
	}
}

// BenchmarkItemGearStats measures the combat stats dictionary lookup of
// the gear scoring.
func BenchmarkItemGearStats(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		if _, ok := ItemGearStats(benchSwordItem); !ok {
			b.Fatal("expected gear stats")
		}
	}
}

// BenchmarkItemPrice measures the price dictionary lookup of the sell
// ranking.
func BenchmarkItemPrice(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = ItemPrice(benchAdenaItem)
	}
}

// BenchmarkItemWeight measures the weight dictionary lookup of the
// sell ranking.
func BenchmarkItemWeight(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = ItemWeight(benchAdenaItem)
	}
}
