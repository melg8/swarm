// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// BenchmarkReadInt8 measures the single byte read path. The optimized
// reader does one bounds check and one byte load; the previous
// bytes.Reader backed form paid for the Read call dispatch and the
// returned (n, err) check on every byte.
func BenchmarkReadInt8(b *testing.B) {
	data := make([]byte, 1)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadInt8(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadInt16 measures the two byte read path.
func BenchmarkReadInt16(b *testing.B) {
	data := make([]byte, 2)
	binary.LittleEndian.PutUint16(data, 0x1234)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if v, err := reader.ReadInt16(); err != nil || v != 0x1234 {
			b.Fatal(v, err)
		}
	}
}

// BenchmarkReadFloat64 measures the eight byte float read path used
// by the StatusUpdate and UserInfo parsers for HP/MP and the move
// multiplier.
func BenchmarkReadFloat64(b *testing.B) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, 0x3FF0000000000000) // 1.0
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadFloat64(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadBytes measures the byte range read used by the key
// packet, the server list and the inventory parser. The optimized
// form returns a sub slice of the source buffer with no copy.
func BenchmarkReadBytes(b *testing.B) {
	data := make([]byte, 64)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadBytes(16); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSkip measures the offset advance used by every parser
// that jumps over fixed wire trails (the UserInfo skips 108 bytes of
// paperdoll display ids and combat stats, the CharInfo skips the
// speed lead and trail).
func BenchmarkSkip(b *testing.B) {
	data := make([]byte, 256)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if err := reader.Skip(200); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadStringASCIIFastPath measures the common case of the
// UTF-16 string read: an ASCII name (every byte's high half is 0).
// Every NpcInfo, CharInfo, UserInfo and CharSelectInfo packet carries
// at least one such string, so the 100 bot fleet pays this once per
// spawn and once per character appearance.
func BenchmarkReadStringASCIIFastPath(b *testing.B) {
	// "Keltir" as null terminated UTF-16LE - the real NpcInfo name
	// of the level 1 mob the elven fields farm.
	name := "Keltir"
	data := make([]byte, 0, len(name)*2+2)
	for _, c := range name {
		data = append(data, byte(c), 0)
	}
	data = append(data, 0, 0)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		s, err := reader.ReadStringFromUtf16Format()
		if err != nil {
			b.Fatal(err)
		}
		if s != name {
			b.Fatalf("got %q want %q", s, name)
		}
	}
}

// BenchmarkReadStringLongASCII measures the read of a longer ASCII
// string (the character name plus the title of a CharInfo packet is
// the worst realistic case).
func BenchmarkReadStringLongASCII(b *testing.B) {
	name := strings.Repeat("a", 48)
	data := make([]byte, 0, len(name)*2+2)
	for _, c := range name {
		data = append(data, byte(c), 0)
	}
	data = append(data, 0, 0)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadStringFromUtf16Format(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadStringBMPSlowPath measures the non-ASCII path: a name
// with Cyrillic letters that require the utf16.Decode fallback. This
// is the rare case, but it must stay correct and measurable so a
// regression that silently drops back to the slow path on ASCII shows
// up against the fast path bench above.
func BenchmarkReadStringBMPSlowPath(b *testing.B) {
	// "Эльф" (Elven in Russian) - BMP but not ASCII.
	runes := []rune("Эльф")
	encoded := utf16.Encode(runes)
	data := make([]byte, 0, len(encoded)*2+2)
	for _, u := range encoded {
		data = append(data, byte(u), byte(u>>8))
	}
	data = append(data, 0, 0)
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadStringFromUtf16Format(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadStringEmpty measures the empty string read: the
// CharInfo title is empty for most mobs, so this is the second most
// common case after the short ASCII name.
func BenchmarkReadStringEmpty(b *testing.B) {
	data := []byte{0, 0}
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		if _, err := reader.ReadStringFromUtf16Format(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadStringNoTerminator measures the error path: the buffer
// ends before a null terminator. The optimized form detects this in
// one pass without allocating an error string (the sentinel
// ErrNotEnoughBytes is reused).
func BenchmarkReadStringNoTerminator(b *testing.B) {
	data := []byte{0x41, 0x00, 0x42, 0x00} // no null terminator
	reader := NewReader(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.Reset(data)
		_, _ = reader.ReadStringFromUtf16Format()
	}
}

// BenchmarkWriteStringAsUtf16ASCII measures the common outbound path:
// the auth login, the character create request and the use item
// commands all write ASCII names through this method. The optimized
// form writes directly into the buffer without a scratch allocation.
func BenchmarkWriteStringAsUtf16ASCII(b *testing.B) {
	value := "test1"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := NewWriter()
		if err := w.WriteStringAsUtf16(value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteStringAsUtf16ReusedWriter measures the steady state
// of a long lived writer: the buffer capacity stabilizes after the
// first write, so subsequent writes reuse the slab without growing
// it. The hunt loop and the proxy build most of their outbound
// packets through fresh writers today, but a future pooling change
// would land here.
func BenchmarkWriteStringAsUtf16ReusedWriter(b *testing.B) {
	value := "test1"
	w := NewWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.Reset()
		if err := w.WriteStringAsUtf16(value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteStringAsUtf16NonASCII measures the slow path: a
// Cyrillic name goes through rune decoding and utf16.Encode.
func BenchmarkWriteStringAsUtf16NonASCII(b *testing.B) {
	value := "Эльф"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := NewWriter()
		if err := w.WriteStringAsUtf16(value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNewReader measures the construction cost. The optimized
// reader is a plain struct with a slice header and an int - no
// bytes.Reader allocation. The 100 bot fleet constructs one reader
// per inbound packet, so this multiplies by the packet rate.
func BenchmarkNewReader(b *testing.B) {
	data := make([]byte, 64)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r := NewReader(data)
		_ = r
	}
}
