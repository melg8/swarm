// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReaderSkip(t *testing.T) {
	t.Run("small skip lands on the next byte", func(t *testing.T) {
		reader := NewReader([]byte{1, 2, 3, 4, 5})
		require.NoError(t, reader.Skip(2))
		value, err := reader.ReadInt8()
		require.NoError(t, err)
		require.Equal(t, int8(3), value)
	})

	t.Run("skip of the whole buffer consumes everything", func(t *testing.T) {
		reader := NewReader([]byte{1, 2, 3})
		require.NoError(t, reader.Skip(3))
		_, err := reader.ReadInt8()
		require.Error(t, err)
	})

	t.Run("multi chunk skip walks sixty four byte steps", func(t *testing.T) {
		data := make([]byte, 200)
		for i := range data {
			data[i] = byte(i)
		}
		reader := NewReader(data)
		require.NoError(t, reader.Skip(130))
		value, err := reader.ReadInt8()
		require.NoError(t, err)
		require.Equal(t, int8(-126), value)
	})

	t.Run("skip past the end errors", func(t *testing.T) {
		reader := NewReader([]byte{1, 2, 3})
		require.Error(t, reader.Skip(4))
	})

	t.Run("skip on an empty buffer errors", func(t *testing.T) {
		reader := NewReader(nil)
		require.Error(t, reader.Skip(1))
	})
}

func TestReaderReadFloat64(t *testing.T) {
	t.Run("round trip through the writer", func(t *testing.T) {
		writer := NewWriter()
		value := 3.14159
		require.NoError(t, writer.WriteInt64(int64(math.Float64bits(value))))

		reader := NewReader(writer.Bytes())
		result, err := reader.ReadFloat64()
		require.NoError(t, err)
		require.InDelta(t, value, result, 1e-9)
	})

	t.Run("empty buffer errors", func(t *testing.T) {
		reader := NewReader(nil)
		_, err := reader.ReadFloat64()
		require.Error(t, err)
	})

	t.Run("truncated buffer errors", func(t *testing.T) {
		reader := NewReader([]byte{1, 2, 3, 4, 5, 6, 7})
		_, err := reader.ReadFloat64()
		require.Error(t, err)
	})
}

func TestReaderReadInt16Truncated(t *testing.T) {
	reader := NewReader([]byte{0x34})
	_, err := reader.ReadInt16()
	require.Error(t, err)
}

func TestNewWriterToAppendsToExistingData(t *testing.T) {
	writer := NewWriterTo([]byte{0x0B})
	require.NoError(t, writer.WriteInt32(18))
	require.Equal(t, []byte{0x0B, 0x12, 0x00, 0x00, 0x00}, writer.Bytes())

	reader := NewReader(writer.Bytes())
	id, err := reader.ReadInt8()
	require.NoError(t, err)
	require.Equal(t, int8(0x0B), id)
	classID, err := reader.ReadInt32()
	require.NoError(t, err)
	require.Equal(t, int32(18), classID)
}
