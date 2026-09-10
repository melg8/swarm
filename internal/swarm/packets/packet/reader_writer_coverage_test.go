// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

// TestReaderReset verifies the Reset method rebinds the reader to a
// new buffer and rewinds the offset. The benchmark loop uses Reset to
// avoid reallocating the reader; the production parse path constructs
// a fresh reader per packet but Reset is part of the API contract.
func TestReaderReset(t *testing.T) {
	reader := NewReader([]byte{1, 2, 3, 4})

	// Consume some bytes.
	v, err := reader.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(0x04030201), v)

	// Read past the end must fail.
	_, err = reader.ReadInt8()
	require.Error(t, err)

	// Reset to a new buffer.
	data := []byte{0x78, 0x56, 0x34, 0x12}
	reader.Reset(data)
	v, err = reader.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(0x12345678), v)
}

// TestReadBytesNegative verifies ReadBytes rejects negative counts.
func TestReadBytesNegative(t *testing.T) {
	reader := NewReader([]byte{1, 2, 3})
	_, err := reader.ReadBytes(-1)
	require.Error(t, err)
}

// TestSkipNegative verifies Skip rejects negative counts.
func TestSkipNegative(t *testing.T) {
	reader := NewReader([]byte{1, 2, 3})
	require.Error(t, reader.Skip(-1))
}

// TestReadStringBMPSlowPath exercises the non ASCII branch of
// ReadStringFromUtf16Format: a name with Cyrillic letters that have
// a non zero high byte in their UTF 16LE encoding. The fast ASCII
// path skips this branch, so the BMP path was uncovered after the
// reader rewrite.
func TestReadStringBMPSlowPath(t *testing.T) {
	// "Эльф" (Elven in Russian) - four Cyrillic letters, all in the BMP.
	runes := []rune("Эльф")
	encoded := utf16.Encode(runes)

	data := make([]byte, 0, len(encoded)*2+2)
	for _, u := range encoded {
		data = append(data, byte(u), byte(u>>8))
	}
	data = append(data, 0, 0)

	reader := NewReader(data)
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, "Эльф", result)

	// The offset must be past the null terminator.
	require.Equal(t, len(data), reader.offset)
}

// TestReadStringWithSupplementaryCharacter verifies the BMP slow path
// handles a supplementary character (above U+FFFF) through surrogate
// pairs. The emoji "🌟" (U+1F31C) is encoded as a surrogate pair in
// UTF 16.
func TestReadStringWithSupplementaryCharacter(t *testing.T) {
	original := "🌟"
	runes := []rune(original)
	encoded := utf16.Encode(runes)

	data := make([]byte, 0, len(encoded)*2+2)
	for _, u := range encoded {
		data = append(data, byte(u), byte(u>>8))
	}
	data = append(data, 0, 0)

	reader := NewReader(data)
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, original, result)
}

// TestReadStringMixedASCIIAndBMP verifies a string that starts ASCII
// but has a non ASCII character in the middle takes the slow path and
// decodes correctly.
func TestReadStringMixedASCIIAndBMP(t *testing.T) {
	original := "test café"
	runes := []rune(original)
	encoded := utf16.Encode(runes)

	data := make([]byte, 0, len(encoded)*2+2)
	for _, u := range encoded {
		data = append(data, byte(u), byte(u>>8))
	}
	data = append(data, 0, 0)

	reader := NewReader(data)
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, original, result)
}

// TestReadStringNoTerminatorInBuffer verifies the error path where
// the buffer ends without a null terminator. The reader must return
// ErrNotEnoughBytes and set the offset to the end of the buffer.
func TestReadStringNoTerminatorInBuffer(t *testing.T) {
	// Two UTF 16 units, no null terminator.
	data := []byte{0x41, 0x00, 0x42, 0x00}
	reader := NewReader(data)
	_, err := reader.ReadStringFromUtf16Format()
	require.ErrorIs(t, err, ErrNotEnoughBytes)
	require.Equal(t, len(data), reader.offset,
		"offset must advance to the end on a missing terminator")
}

// TestReadStringOddLengthBuffer verifies the reader handles a buffer
// with an odd number of bytes (the last byte has no pair). The scan
// loop steps by 2, so it must not read past the end.
func TestReadStringOddLengthBuffer(t *testing.T) {
	// One full UTF 16 unit plus a trailing byte with no pair.
	data := []byte{0x41, 0x00, 0x42}
	reader := NewReader(data)
	_, err := reader.ReadStringFromUtf16Format()
	require.Error(t, err)
}

// TestWriteStringAsUtf16NonASCII verifies the slow path of
// WriteStringAsUtf16: a non ASCII string goes through rune decoding
// and utf16.Encode, producing correct UTF 16LE bytes.
func TestWriteStringAsUtf16NonASCII(t *testing.T) {
	writer := NewWriter()
	require.NoError(t, writer.WriteStringAsUtf16("café"))

	// Verify the round trip: read the bytes back and check the string.
	reader := NewReader(writer.Bytes())
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, "café", result)
}

// TestWriteStringAsUtf16Supplementary verifies the slow path handles
// supplementary characters (above U+FFFF) through surrogate pairs.
func TestWriteStringAsUtf16Supplementary(t *testing.T) {
	writer := NewWriter()
	original := "🌟 star"
	require.NoError(t, writer.WriteStringAsUtf16(original))

	reader := NewReader(writer.Bytes())
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, original, result)
}

// TestWriteStringAsUtf16Cyrillic verifies the slow path with Cyrillic
// characters that are in the BMP but outside Latin 1.
func TestWriteStringAsUtf16Cyrillic(t *testing.T) {
	writer := NewWriter()
	original := "Эльф"
	require.NoError(t, writer.WriteStringAsUtf16(original))

	reader := NewReader(writer.Bytes())
	result, err := reader.ReadStringFromUtf16Format()
	require.NoError(t, err)
	require.Equal(t, original, result)
}

// TestWriteFloat64RoundTrip verifies the WriteFloat64 / ReadFloat64
// round trip for a range of values. The WriteFloat64 method was
// uncovered by the existing tests after the writer reorganization.
func TestWriteFloat64RoundTrip(t *testing.T) {
	values := []float64{0.0, 1.0, -1.0, 3.14159, 1e10, -1e-10, 123456.789}

	for _, v := range values {
		writer := NewWriter()
		require.NoError(t, writer.WriteFloat64(v))

		reader := NewReader(writer.Bytes())
		result, err := reader.ReadFloat64()
		require.NoError(t, err)
		require.InDelta(t, v, result, 1e-9)
	}
}

// TestWriteStringAsUtf16Empty verifies the empty string writes just
// the null terminator.
func TestWriteStringAsUtf16Empty(t *testing.T) {
	writer := NewWriter()
	require.NoError(t, writer.WriteStringAsUtf16(""))
	require.Equal(t, []byte{0, 0}, writer.Bytes())
}

// TestWriteStringAsUtf16ASCIIVsReference verifies the fast ASCII path
// produces the same bytes as the original byte by byte implementation:
// each ASCII character as [char, 0], followed by [0, 0].
func TestWriteStringAsUtf16ASCIIVsReference(t *testing.T) {
	value := "test1"
	writer := NewWriter()
	require.NoError(t, writer.WriteStringAsUtf16(value))

	expected := []byte{'t', 0, 'e', 0, 's', 0, 't', 0, '1', 0, 0, 0}
	require.Equal(t, expected, writer.Bytes())
}
