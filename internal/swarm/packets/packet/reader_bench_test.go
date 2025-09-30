// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"testing"
)

func BenchmarkPlusPlus(b *testing.B) {
	for range b.N {
		value := 2 + 2
		if value != 4 {
			b.Fatal("value is not 4")
		}
	}
}

func BenchmarkReadInt32(b *testing.B) {
	data := []byte{0x78, 0x56, 0x34, 0x12}
	reader := NewReader(data)

	for range b.N {
		reader.Reset(data)
		int32Value, err := reader.ReadInt32()
		if err != nil {
			b.Fatal(err)
		}
		if int32Value != 0x12345678 {
			b.Fatal("expected value 0x12345678, got: ", int32Value)
		}
	}
}

func BenchmarkReadInt64(b *testing.B) {
	data := []byte{0xf1, 0xef, 0xcd, 0xab, 0x78, 0x56, 0x34, 0x12}
	reader := NewReader(data)

	for range b.N {
		reader.Reset(data)
		int64Value, err := reader.ReadInt64()
		if err != nil {
			b.Fatal(err)
		}
		if int64Value != 0x12345678abcdeff1 {
			b.Fatal("expected value 0x12345678abcdeff1, got: ", int64Value)
		}
	}
}
