// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

func BenchmarkProtocolVersionToBytes(b *testing.B) {
	pv := NewProtocolVersion()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := pv.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthLoginToBytes(b *testing.B) {
	auth := &AuthLogin{
		Login:      "test1",
		PlayOkID1:  1,
		PlayOkID2:  2,
		LoginOkID1: 3,
		LoginOkID2: 4,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := auth.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCharacterCreateToBytes(b *testing.B) {
	create := &CharacterCreate{
		Name:      "test1",
		Race:      1,
		Female:    0,
		ClassID:   18,
		INT:       0,
		STR:       0,
		CON:       0,
		MEN:       0,
		DEX:       0,
		WIT:       0,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := create.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRequestSellItemToBytes(b *testing.B) {
	items := make([]SellItemEntry, 0, 80)
	for i := range 80 {
		items = append(items, SellItemEntry{
			ObjectID: int32(1000 + i),
			ItemID:   int32(34 + i),
			Count:    int32(i + 1),
		})
	}
	sell := NewRequestSellItemPacket()
	sell.Items = items

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := sell.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAppearingToBytes(b *testing.B) {
	appearing := NewAppearingPacket()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := appearing.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}
