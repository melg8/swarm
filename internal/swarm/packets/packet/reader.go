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

// ErrNotEnoughBytes is returned when a read request exceeds the buffer. The
// sentinel costs no allocation per short read, the price the hot path of a
// fleet parsing thousands of packets a second paid once per EOF.
var ErrNotEnoughBytes = errors.New("not enough bytes to read")

// Reader is a little endian binary reader over a fixed byte slice.
// It stores a direct slice header and an offset instead of wrapping
// bytes.Reader: the direct form keeps one bounds check against len(data) and
// lets the integer readers call encoding/binary.LittleEndian directly, which
// compiles to a single unaligned load on little endian hosts.
type Reader struct {
    data   []byte
    offset int
}

// NewReader creates a reader over data.
func NewReader(buffer []byte) *Reader {
    return &Reader{data: buffer, offset: 0}
}

// Reset rebinds the reader to a new buffer and rewinds the offset; the
// benchmark loop reuses one reader, the production parse path builds a fresh
// one.
func (r *Reader) Reset(data []byte) {
    r.data = data
    r.offset = 0
}

// ReadBytes returns the next number bytes as a sub slice of the source buffer,
// with no allocation: the result aliases the data until the reader is Reset, so
// a caller that needs to outlive the source copies (the key packet reader
// does).
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

// Skip advances the offset by number bytes after one bounds check, so skipping
// a 200 byte trail costs one comparison.
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

// ReadStringFromUtf16Format reads a null terminated UTF-16LE string. The scan
// walks the source slice to the null terminator, takes the ASCII path (the high
// byte of every unit is 0) in one allocation and falls back to unicode/utf16
// for non-ASCII names; the x/text decoder stays the last resort for the
// surrogate pairs the proxy transformer contract expects.
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

    // Fast path: every UTF-16 unit is ASCII (low byte below 0x80, high byte 0),
    // the common case of L2 names. The low byte check is required too: a
    // Latin-1 character like U+00E9 encodes as [0xE9, 0x00], and a low byte
    // copy would produce invalid UTF-8.
    ascii := true
    for i := start; i < end; i += 2 {
        if data[i] >= 0x80 || data[i+1] != 0 {
            ascii = false

            break
        }
    }
    if ascii {
        // unsafe.String reuses the byte buffer as the string, so the fast path
        // allocates once instead of copying through string(buf); the GC keeps
        // the array alive as long as the string lives.
        buf := make([]byte, unitCount)
        for i := range unitCount {
            buf[i] = data[start+i*2]
        }

        return unsafe.String(unsafe.SliceData(buf), unitCount), nil
    }

    // BMP slow path: the stdlib decoder, one allocation for the units and one
    // for the final string.
    pairs := make([]uint16, unitCount)
    for i := range unitCount {
        pairs[i] = binary.LittleEndian.Uint16(data[start+i*2:])
    }

    decoded := utf16.Decode(pairs)

    // utf16.Decode returns a []rune; string([]rune) is one allocation.
    return string(decoded), nil
}
