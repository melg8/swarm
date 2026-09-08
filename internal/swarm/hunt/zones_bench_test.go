// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import "testing"

// BenchmarkPickHuntingZone measures the 30 s zone re-pick over the full
// elven registry (30 zones): the ladder walk with the band rank and
// distance tie breaks the picker runs per re-evaluation.
func BenchmarkPickHuntingZone(b *testing.B) {
	zones := ElvenHuntingZones()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		zone, ok := PickHuntingZone(
			zones, 9, 50, "elven-gremlin-hollow", 45000, 50000, -1)
		if !ok {
			b.Fatal("expected a picked zone")
		}
		if zone.ID == "" {
			b.Fatal("expected a named zone")
		}
	}
}

// BenchmarkPickHuntingZoneStarter measures the fallback contest of a
// character below every band (the starter band walk).
func BenchmarkPickHuntingZoneStarter(b *testing.B) {
	zones := ElvenHuntingZones()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := PickHuntingZone(
			zones, 1, 0, "", 45000, 50000, -1); !ok {
			b.Fatal("expected a picked zone")
		}
	}
}
