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
    // FailWritesAfter): every write method returns it once the budget
    // runs out. A memory buffer write never fails on its own, so the
    // guarded error returns of the packet builders are unreachable in
    // production - the armed writer is the exercise those branches
    // get (see the builder error path tests of the packet packages).
    writeErr error
    // writeBudget counts the writes that still pass before the armed
    // failure fires (FailWritesAfter): the counted form lets the
    // builder tests walk a whole write sequence branch by branch.
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

// FailWrites arms the sticky write failure: every write method of
// this writer returns err from the next call on, without touching
// the buffer. The packet builders guard every write with an error
// return - a contract the wire layer could enforce one day - and the
// failure injection is how the tests reach those branches. Pass a
// fresh writer (or never arm one) for the ordinary error free
// behavior.
func (b *Writer) FailWrites(err error) {
    b.writeErr = err
    b.writeBudget = 0
}

// FailWritesAfter arms the counted write failure: the first n write
// calls succeed, every later call returns err. A sticky arm from the
// first write would exercise only the FIRST error branch of a
// builder - the counted form walks the whole write sequence branch
// by branch (the builder error tests step n through 0, 1, 2, ...).
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
    if err := b.writeGate(); err != nil {
        return err
    }
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
