// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
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
