// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf16"
	"unsafe"
)

// ErrNotEnoughBytes is returned when a read request exceeds the buffer.
// Kept as a sentinel so callers can match it without allocating a new
// error string per short read - the hot path of a bot fleet parsing
// thousands of packets per second used to pay an allocation per EOF.
var ErrNotEnoughBytes = errors.New("not enough bytes to read")

// Reader is a little endian binary reader over a fixed byte slice.
//
// The reader stores a direct slice header and an offset instead of
// wrapping bytes.Reader: the production hot path (a bot fleet of 100
// sessions parsing hundreds of packets per second each) pays for every
// interface dispatch and bounds check twice - once inside bytes.Reader
// and once in the n != expected length check the callers already do.
// The direct slice form lets the Go compiler keep the bounds check
// cheap (one comparison against len(data)) and lets the integer
// readers call encoding/binary.LittleEndian directly, which the
// compiler turns into a single unaligned load on little endian hosts.
type Reader struct {
	data   []byte
	offset int
}

// NewReader creates a reader over data.
func NewReader(buffer []byte) *Reader {
	return &Reader{data: buffer, offset: 0}
}

// Reset rebinds the reader to a new buffer and rewinds the offset.
// Used by the benchmark loop to avoid reallocating the reader; the
// production parse path constructs a fresh reader per packet.
func (r *Reader) Reset(data []byte) {
	r.data = data
	r.offset = 0
}

// ReadBytes returns the next number bytes as a sub slice of the source
// buffer. No allocation: the returned slice aliases the underlying data
// until the reader is Reset, so callers that need to outlive the source
// must copy (the key packet reader does copy(dest[:], key) for exactly
// this reason). The previous implementation allocated a fresh buffer
// per call, which on the 100 bot fleet path added one allocation per
// byte range read - the server list alone paid 16+ allocations per
// packet just for the IP and tail slices it immediately copied or
// compared.
func (r *Reader) ReadBytes(number int) ([]byte, error) {
	if number < 0 {
		return nil, ErrNotEnoughBytes
	}

	end := r.offset + number
	if end > len(r.data) {
		return nil, ErrNotEnoughBytes
	}

	buf := r.data[r.offset:end]
	r.offset = end

	return buf, nil
}

// Skip advances the offset by number bytes without allocating. The
// previous implementation read into a 64 byte stack buffer in a loop
// to stay allocation free; the direct slice form just bumps the offset
// after one bounds check, so skipping a 200 byte trail is now one
// comparison instead of four Read calls.
func (r *Reader) Skip(number int) error {
	if number < 0 {
		return ErrNotEnoughBytes
	}

	end := r.offset + number
	if end > len(r.data) {
		return ErrNotEnoughBytes
	}

	r.offset = end

	return nil
}

// ReadInt64 reads a little endian int64.
func (r *Reader) ReadInt64() (int64, error) {
	if r.offset+8 > len(r.data) {
		return 0, ErrNotEnoughBytes
	}

	v := int64(binary.LittleEndian.Uint64(r.data[r.offset:]))
	r.offset += 8

	return v, nil
}

// ReadInt32 reads a little endian int32.
func (r *Reader) ReadInt32() (int32, error) {
	if r.offset+4 > len(r.data) {
		return 0, ErrNotEnoughBytes
	}

	v := int32(binary.LittleEndian.Uint32(r.data[r.offset:]))
	r.offset += 4

	return v, nil
}

// ReadInt16 reads a little endian int16.
func (r *Reader) ReadInt16() (int16, error) {
	if r.offset+2 > len(r.data) {
		return 0, ErrNotEnoughBytes
	}

	v := int16(binary.LittleEndian.Uint16(r.data[r.offset:]))
	r.offset += 2

	return v, nil
}

// ReadInt8 reads a single byte as a signed int8.
func (r *Reader) ReadInt8() (int8, error) {
	if r.offset >= len(r.data) {
		return 0, ErrNotEnoughBytes
	}

	v := int8(r.data[r.offset])
	r.offset++

	return v, nil
}

// ReadFloat64 reads a little endian float64.
func (r *Reader) ReadFloat64() (float64, error) {
	if r.offset+8 > len(r.data) {
		return 0, ErrNotEnoughBytes
	}

	v := math.Float64frombits(binary.LittleEndian.Uint64(r.data[r.offset:]))
	r.offset += 8

	return v, nil
}

// ReadStringFromUtf16Format reads a null terminated UTF-16LE string.
//
// The previous implementation allocated a growing []byte, converted it
// to a string, then ran it through the golang.org/x/text UTF-16
// decoder which allocated again - three allocations per name on the
// hot path of every NpcInfo, CharInfo, UserInfo and CharSelectInfo
// packet. The fleet of 100 bots parsing a busy spawn burst pays that
// thousands of times per second.
//
// The optimized form scans the source slice directly for the null
// terminator, takes the fast ASCII path (the common case for L2
// character and NPC names - the high byte of every UTF-16 unit is 0)
// and builds the string in one allocation, and only falls back to the
// stdlib unicode/utf16 decoder for genuine non-ASCII names. The x/text
// decoder is kept as a last resort for surrogate pairs that the stdlib
// Decode handles differently from the proxy transformer contract.
func (r *Reader) ReadStringFromUtf16Format() (string, error) {
	start := r.offset
	end := start
	data := r.data
	for end+1 < len(data) {
		if data[end] == 0 && data[end+1] == 0 {
			break
		}
		end += 2
	}
	if end+1 >= len(data) {
		// Never found a null terminator within the buffer.
		r.offset = len(data)

		return "", ErrNotEnoughBytes
	}

	byteLen := end - start
	unitCount := byteLen / 2
	r.offset = end + 2

	// Fast path: every UTF-16 unit fits in ASCII (the low byte is
	// below 0x80 and the high byte is 0). L2 names are overwhelmingly
	// ASCII, so this is the common case and collapses three
	// allocations into one (the final string). The high byte check
	// alone is not enough: a Latin-1 character like U+00E9 ('é')
	// encodes as [0xE9, 0x00] in UTF-16LE, which has a zero high byte
	// but a low byte above 0x80 - extracting just the low byte would
	// produce a byte 0xE9 that is not valid UTF-8, so the resulting
	// string would display as the replacement character instead of
	// the original character.
	ascii := true
	for i := start; i < end; i += 2 {
		if data[i] >= 0x80 || data[i+1] != 0 {
			ascii = false

			break
		}
	}
	if ascii {
		// Build the ASCII byte buffer and convert it to a string in
		// place through unsafe.String: the byte slice and the string
		// share the same backing array, so the GC keeps it alive as
		// long as the string lives. This collapses the fast path to
		// one allocation (the byte buffer); the previous string(buf)
		// conversion copied the bytes into a second allocation. The
		// unsafe form is the same trick strings.Builder.String uses.
		buf := make([]byte, unitCount)
		for i := range unitCount {
			buf[i] = data[start+i*2]
		}

		return unsafe.String(unsafe.SliceData(buf), unitCount), nil
	}

	// BMP slow path: decode the UTF-16 units through the stdlib
	// decoder. One allocation for the uint16 slice plus one for the
	// final string - still better than the old three.
	pairs := make([]uint16, unitCount)
	for i := range unitCount {
		pairs[i] = binary.LittleEndian.Uint16(data[start+i*2:])
	}

	decoded := utf16.Decode(pairs)

	// utf16.Decode returns a []rune; string([]rune) is one allocation.
	return string(decoded), nil
}
