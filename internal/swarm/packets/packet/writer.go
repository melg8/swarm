// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"bytes"
	"encoding/binary"
	"math"
	"unicode/utf16"
	"unsafe"
)

type Writer struct {
	*bytes.Buffer
}

func NewWriter() *Writer {
	return &Writer{Buffer: bytes.NewBuffer([]byte{})}
}

func NewWriterTo(data []byte) *Writer {
	return &Writer{Buffer: bytes.NewBuffer(data)}
}

func (b *Writer) WriteInt64(value int64) error {
	buf := (*[8]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt32(value int32) error {
	buf := (*[4]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt16(value int16) error {
	buf := (*[2]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt8(value int8) error {
	return b.WriteByte(byte(value))
}

// WriteFloat64 writes a little endian float64 value.
func (b *Writer) WriteFloat64(value float64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteBytes(bytes []byte) error {
	_, err := b.Write(bytes)

	return err
}

// WriteStringAsUtf16 writes value as a null terminated UTF-16LE byte
// sequence. The previous implementation allocated an intermediate
// []byte per call and then copied it into the buffer through Write,
// which on the 100 bot fleet path added up: every outbound packet
// that carries a name (the auth login, the character create request,
// the use item and say commands) paid one allocation just for the
// scratch buffer.
//
// The optimized form pre-sizes the buffer with Grow so the underlying
// bytes.Buffer reuses its capacity across writes, writes ASCII names
// (the overwhelming majority of L2 character, account and NPC names)
// directly into the buffer through a single stack array per pair, and
// only falls back to rune decoding for genuine non-ASCII input. The
// slow path now uses unicode/utf16.Encode so supplementary characters
// above the BMP produce correct surrogate pairs instead of the
// previous byte(r) truncation that silently corrupted non Latin-1
// names.
func (b *Writer) WriteStringAsUtf16(value string) error {
	// Fast path: ASCII only. Scan once to confirm, then write pairs
	// directly without allocating a scratch slice. The Grow hint
	// keeps the buffer from reallocating mid-write on repeated calls.
	ascii := true
	for i := range len(value) {
		if value[i] >= 0x80 {
			ascii = false

			break
		}
	}

	if ascii {
		// len(value) pairs plus the two byte null terminator.
		b.Grow(len(value)*2 + 2)
		var pair [2]byte
		for i := range len(value) {
			pair[0] = value[i]
			pair[1] = 0
			b.Write(pair[:])
		}

		// Null terminator.
		b.Write([]byte{0, 0})

		return nil
	}

	// Slow path: non-ASCII. Decode UTF-8 runes, encode to UTF-16
	// (producing surrogate pairs for supplementary characters), then
	// write each unit little endian. One allocation for the rune
	// slice and one for the encoded units - acceptable for the rare
	// non-ASCII name.
	runes := []rune(value)
	encoded := utf16.Encode(runes)
	var buf [2]byte
	for _, unit := range encoded {
		binary.LittleEndian.PutUint16(buf[:], unit)
		b.Write(buf[:])
	}
	b.Write([]byte{0, 0})

	return nil
}
