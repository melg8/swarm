// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteWirePacket(t *testing.T) {
	t.Run("prepends little endian size header", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeWirePacket(&buf, []byte{0x00, 0x01, 0x02, 0x03})
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x06, 0x00, // size 6 little endian
			0x00, 0x01, 0x02, 0x03,
		}, buf.Bytes())
	})

	t.Run("empty payload writes bare header", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeWirePacket(&buf, nil)
		require.NoError(t, err)
		require.Equal(t, []byte{0x02, 0x00}, buf.Bytes())
	})

	t.Run("oversized payload is rejected", func(t *testing.T) {
		var buf bytes.Buffer
		payload := make([]byte, maxUint16)
		err := writeWirePacket(&buf, payload)
		require.Error(t, err)
	})

	t.Run("write failure is wrapped", func(t *testing.T) {
		err := writeWirePacket(&failingWriter{}, []byte{1, 2})
		require.Contains(t, err.Error(), "failed to write packet header")
	})
}

func TestReadWirePacket(t *testing.T) {
	t.Run("reads framed payload", func(t *testing.T) {
		buf := bytes.NewReader([]byte{
			0x06, 0x00, // size 6
			0x00, 0x01, 0x02, 0x03,
		})
		payload, err := readWirePacket(buf, nil)
		require.NoError(t, err)
		require.Equal(t, []byte{0x00, 0x01, 0x02, 0x03}, payload)
	})

	t.Run("reuses provided buffer", func(t *testing.T) {
		buf := bytes.NewReader([]byte{0x03, 0x00, 0xAA, 0xBB})
		reused := make([]byte, 0, 16)
		payload, err := readWirePacket(buf, reused)
		require.NoError(t, err)
		require.Len(t, payload, 1)
		require.Equal(t, byte(0xAA), payload[0])
	})

	t.Run("grows small buffer", func(t *testing.T) {
		buf := bytes.NewReader(append([]byte{0x20, 0x00}, make([]byte, 30)...))
		payload, err := readWirePacket(buf, make([]byte, 1))
		require.NoError(t, err)
		require.Len(t, payload, 30)
	})

	t.Run("empty payload returns zero slice", func(t *testing.T) {
		buf := bytes.NewReader([]byte{0x02, 0x00})
		payload, err := readWirePacket(buf, nil)
		require.NoError(t, err)
		require.Empty(t, payload)
	})

	t.Run("invalid size", func(t *testing.T) {
		buf := bytes.NewReader([]byte{0x01, 0x00})
		_, err := readWirePacket(buf, nil)
		require.Contains(t, err.Error(), "invalid packet size")
	})

	t.Run("truncated header", func(t *testing.T) {
		buf := bytes.NewReader([]byte{0x06})
		_, err := readWirePacket(buf, nil)
		require.Contains(t, err.Error(), "failed to read packet header")
	})

	t.Run("truncated payload", func(t *testing.T) {
		buf := bytes.NewReader([]byte{0x06, 0x00, 0x00, 0x01})
		_, err := readWirePacket(buf, nil)
		require.Contains(t, err.Error(), "failed to read packet payload")
	})
}

func TestWireRoundtrip(t *testing.T) {
	t.Run("write then read", func(t *testing.T) {
		var buf bytes.Buffer
		payload := []byte{0x0B, 0x74, 0x00, 0x65, 0x00}

		require.NoError(t, writeWirePacket(&buf, payload))

		reader := bytes.NewReader(buf.Bytes())
		got, err := readWirePacket(reader, nil)
		require.NoError(t, err)
		require.Equal(t, payload, got)
		require.Equal(t, 0, reader.Len(), "all bytes must be consumed")
	})
}

func TestWireEndian(t *testing.T) {
	t.Run("header is little endian", func(t *testing.T) {
		require.Equal(t, binary.LittleEndian, wireEndian)
	})
}

// failingWriter rejects every write.
type failingWriter struct{}

func (w *failingWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
