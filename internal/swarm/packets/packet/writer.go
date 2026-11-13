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
    // writeErr is the sticky failure the tests arm (FailWrites or
    // FailWritesAfter): every write method returns it once the budget runs out.
    // A memory buffer write never fails on its own, so the guarded error
    // returns of the builders are only reachable through an armed writer.
    writeErr error
    // writeBudget counts the writes that still pass before the armed failure
    // fires (FailWritesAfter), so a builder test walks a write sequence branch
    // by branch.
    writeBudget int
}

func NewWriter() *Writer {
    return &Writer{
        Buffer:      bytes.NewBuffer([]byte{}),
        writeErr:    nil,
        writeBudget: 0,
    }
}

func NewWriterTo(data []byte) *Writer {
    return &Writer{
        Buffer:      bytes.NewBuffer(data),
        writeErr:    nil,
        writeBudget: 0,
    }
}

// FailWrites arms the sticky write failure: every write method returns err from
// the next call on and the buffer stays untouched. The packet builders guard
// every write with an error return, and the failure injection is how the tests
// reach those branches.
func (b *Writer) FailWrites(err error) {
    b.writeErr = err
    b.writeBudget = 0
}

// FailWritesAfter arms the counted write failure: the first n calls succeed,
// every later one returns err. A sticky arm would exercise only the first error
// branch of a builder, the counted form walks the whole write sequence.
func (b *Writer) FailWritesAfter(n int, err error) {
    b.writeErr = err
    b.writeBudget = n
}

// writeGate hands back the armed failure once the budget is spent
// (nil keeps the write going and consumes one budget unit).
func (b *Writer) writeGate() error {
    if b.writeErr == nil {
        return nil
    }
    if b.writeBudget > 0 {
        b.writeBudget--

        return nil
    }

    return b.writeErr
}

func (b *Writer) WriteInt64(value int64) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    buf := (*[8]byte)(unsafe.Pointer(&value))
    _, err := b.Write(buf[:])

    return err
}

func (b *Writer) WriteInt32(value int32) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    buf := (*[4]byte)(unsafe.Pointer(&value))
    _, err := b.Write(buf[:])

    return err
}

func (b *Writer) WriteInt16(value int16) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    buf := (*[2]byte)(unsafe.Pointer(&value))
    _, err := b.Write(buf[:])

    return err
}

func (b *Writer) WriteInt8(value int8) error {
    if err := b.writeGate(); err != nil {
        return err
    }

    return b.WriteByte(byte(value))
}

// WriteFloat64 writes a little endian float64 value.
func (b *Writer) WriteFloat64(value float64) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    var buf [8]byte
    binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
    _, err := b.Write(buf[:])

    return err
}

func (b *Writer) WriteBytes(bytes []byte) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    _, err := b.Write(bytes)

    return err
}

// WriteStringAsUtf16 writes value as a null terminated UTF-16LE byte sequence.
// The ASCII fast path (the overwhelming majority of L2 names) writes the pairs
// from one stack array behind a Grow hint, so a call allocates nothing; the
// non-ASCII path encodes the runes through utf16.Encode, so supplementary
// characters produce surrogate pairs instead of a truncated byte.
func (b *Writer) WriteStringAsUtf16(value string) error {
    if err := b.writeGate(); err != nil {
        return err
    }
    // Fast path: ASCII only. Scan once to confirm, then write the pairs from
    // one stack array without a scratch slice; the Grow hint keeps the buffer
    // from reallocating mid-write.
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

    // Slow path: decode the runes and utf16.Encode them (surrogate pairs above
    // the BMP), then write each unit little endian.
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
