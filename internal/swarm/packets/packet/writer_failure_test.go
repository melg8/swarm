// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/require"
)

// The failure injection of the writer (FailWrites): the packet
// builders guard every write with an error return, but a memory
// buffer write never fails - the armed writer is the only way those
// branches execute. These tests pin the seam itself: the armed error
// comes back from every write method, the buffer stays untouched,
// and a fresh writer keeps the ordinary error free behavior.

var errWriteBoom = errors.New("write boom")

func TestFailWritesFailsEveryWriteMethod(t *testing.T) {
    writer := NewWriter()
    writer.FailWrites(errWriteBoom)

    require.ErrorIs(t, writer.WriteInt8(1), errWriteBoom)
    require.ErrorIs(t, writer.WriteInt16(1), errWriteBoom)
    require.ErrorIs(t, writer.WriteInt32(1), errWriteBoom)
    require.ErrorIs(t, writer.WriteInt64(1), errWriteBoom)
    require.ErrorIs(t, writer.WriteFloat64(1), errWriteBoom)
    require.ErrorIs(t, writer.WriteBytes([]byte{1}), errWriteBoom)
    require.ErrorIs(t, writer.WriteStringAsUtf16("ab"), errWriteBoom)
    require.ErrorIs(t, writer.WriteStringAsUtf16("ä"), errWriteBoom,
        "the non ascii slow path honors the armed failure too")
    require.Empty(t, writer.Bytes(),
        "the armed writer must not touch the buffer")
}

func TestFreshWriterStaysErrorFree(t *testing.T) {
    writer := NewWriter()
    require.NoError(t, writer.WriteInt8(0x38))
    require.NoError(t, writer.WriteInt32(1))
    require.NoError(t, writer.WriteInt16(2))
    require.NoError(t, writer.WriteInt64(3))
    require.NoError(t, writer.WriteFloat64(4))
    require.NoError(t, writer.WriteBytes([]byte{5}))
    require.NoError(t, writer.WriteStringAsUtf16("ok"))
    require.Len(t, writer.Bytes(), 1+4+2+8+8+1+(2*2+2),
        "the unarmed writer writes everything (the utf16 string is "+
            "two pairs plus the null terminator)")
}

func TestFailWritesAfterCountsTheWritesDown(t *testing.T) {
    writer := NewWriter()
    writer.FailWritesAfter(2, errWriteBoom)

    require.NoError(t, writer.WriteInt8(1), "the first write passes")
    require.NoError(t, writer.WriteInt32(2), "the second write passes")
    require.ErrorIs(t, writer.WriteInt16(3), errWriteBoom,
        "the budget ran out: the failure fires")
    require.ErrorIs(t, writer.WriteBytes([]byte{4}), errWriteBoom,
        "the failure stays armed past the budget")
    require.Equal(t, []byte{1, 2, 0, 0, 0}, writer.Bytes(),
        "the writes before the budget ran out land in the buffer")
}
