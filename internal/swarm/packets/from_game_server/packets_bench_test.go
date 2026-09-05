// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"encoding/binary"
	"math"
	"testing"
)

// buildBenchmarkCharList builds a char select info payload with n entries.
func buildBenchmarkCharList(n int) []byte {
	data := []byte{0x1F}
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], uint32(n)) //nolint:gosec // bench
	data = append(data, count[:]...)

	entry := utf16("test1")
	entry = putInt32(entry, 100)
	entry = append(entry, utf16("test1")...)
	entry = putInt32(entry, 0) // session id
	entry = putInt32(entry, 0) // clan id
	entry = putInt32(entry, 0) // builder
	entry = putInt32(entry, 0) // sex
	entry = putInt32(entry, 1) // race
	entry = putInt32(entry, 18)
	entry = putInt32(entry, 1) // gs name
	entry = putInt32(entry, 100)
	entry = putInt32(entry, 200)
	entry = putInt32(entry, 300)
	var hp [8]byte
	binary.LittleEndian.PutUint64(hp[:], math.Float64bits(50))
	entry = append(entry, hp[:]...)
	entry = append(entry, hp[:]...)
	entry = putInt32(entry, 0) // sp
	entry = putInt32(entry, 0) // exp
	entry = putInt32(entry, 1) // level
	for range 40 {
		entry = putInt32(entry, 0)
	}
	entry = putInt32(entry, 2)      // hair style
	entry = putInt32(entry, 1)      // hair color
	entry = putInt32(entry, 0)      // face
	entry = append(entry, hp[:]...) // max hp
	entry = append(entry, hp[:]...) // max mp
	entry = putInt32(entry, 0)      // delete timer

	for range n {
		data = append(data, entry...)
	}

	return data
}

func BenchmarkParseCharSelectInfoPacket(b *testing.B) {
	data := buildBenchmarkCharList(7)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		p := NewCharSelectInfoPacket()
		if err := ParseCharSelectInfoPacket(p, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseKeyPacket(b *testing.B) {
	data := []byte{0x00, 0x01}
	data = append(data, 0x94, 0x35, 0x00, 0x00, 0xa1, 0x6c, 0x54, 0x87)
	data = putInt32(data, 2)
	data = putInt32(data, 1)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		p := NewKeyPacket()
		if err := ParseKeyPacket(p, data); err != nil {
			b.Fatal(err)
		}
	}
}

// buildNpcInfoPayload builds a realistic NpcInfo packet.
func buildNpcInfoPayload() []byte {
	data := []byte{0x22}
	data = putInt32(data, 268473919)
	data = putInt32(data, 1001277)
	data = putInt32(data, 1)
	data = putInt32(data, 45000)
	data = putInt32(data, 50000)
	data = putInt32(data, -3500)
	data = putInt32(data, 16384)
	data = append(data, make([]byte, npcInfoSpeedLead)...)
	data = putInt32(data, 165)
	data = putInt32(data, 55)
	data = append(data, make([]byte, npcInfoSpeedTrail)...)
	data = putFloat64(data, 1.15)
	data = append(data, make([]byte, npcInfoAppearanceTail)...)
	data = append(data, 1, 1, 0, 0, 0)
	data = append(data, utf16("Keltir")...)
	data = append(data, utf16("Lv 1")...)

	return data
}

// buildUserInfoPayload builds a realistic UserInfo packet tail.
func buildUserInfoPayload() []byte {
	data := []byte{0x04}
	data = putInt32(data, 45000)
	data = putInt32(data, 50000)
	data = putInt32(data, -3500)
	data = putInt32(data, 0)
	data = putInt32(data, 268473919)
	data = append(data, utf16("test1")...)
	for _, value := range []int32{
		1, 0, 18, 5, 2000, 36, 35, 36, 23, 14, 25, 122, 100, 40, 39,
	} {
		data = putInt32(data, value)
	}

	return data
}

// BenchmarkParseNpcInfoPacket measures the npc spawn hot path.
func BenchmarkParseNpcInfoPacket(b *testing.B) {
	data := buildNpcInfoPayload()
	packet := NewNpcInfoPacket()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := ParseNpcInfoPacket(packet, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseUserInfoPacket measures the self state refresh path.
func BenchmarkParseUserInfoPacket(b *testing.B) {
	data := buildUserInfoPayload()
	packet := NewUserInfoPacket()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := ParseUserInfoPacket(packet, data); err != nil {
			b.Fatal(err)
		}
	}
}
