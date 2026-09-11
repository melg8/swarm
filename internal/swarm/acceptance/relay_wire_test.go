// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRelayWireFramingRoundTrip pins the Mobius wire convention of the
// relay client helpers: the 2 byte size header includes itself, so a
// write followed by a read of the same connection returns the payload
// untouched. The 2026-09-11 regression wrote the exclusive payload
// size and read the header as exclusive too, which deadlocked the
// proxy relay scenario on the very first init packet.
func TestRelayWireFramingRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer func() {
		_ = client.Close()
	}()

	payload := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
	go func() {
		defer func() {
			_ = server.Close()
		}()
		if err := writeWirePacket(server, payload); err != nil {
			t.Errorf("write: %v", err)

			return
		}
	}()

	got, err := readWirePacket(client)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

// TestRelayWireWriteMarksTheHeaderInclusive verifies the exact header
// bytes: the announced size equals the payload length plus the two
// header bytes, the framing the proxy and the real Mobius server read.
func TestRelayWireWriteMarksTheHeaderInclusive(t *testing.T) {
	reader, writer := net.Pipe()
	defer func() {
		_ = writer.Close()
	}()

	payload := bytes.Repeat([]byte{0xAA}, 10)
	go func() {
		defer func() {
			_ = reader.Close()
		}()
		if err := writeWirePacket(reader, payload); err != nil {
			t.Errorf("write: %v", err)
		}
	}()

	header := make([]byte, 2)
	require.NoError(t, readFull(writer, header))
	size := int(header[0]) | int(header[1])<<8
	require.Equal(t, len(payload)+2, size, "size includes the header")

	body := make([]byte, len(payload))
	require.NoError(t, readFull(writer, body))
	require.Equal(t, payload, body)
}

// TestRelayWireReadAcceptsTheInclusiveHeader verifies the read side
// against a hand framed inclusive header packet.
func TestRelayWireReadAcceptsTheInclusiveHeader(t *testing.T) {
	server, client := net.Pipe()
	defer func() {
		_ = client.Close()
	}()

	payload := []byte{0x2F, 0x00}
	go func() {
		defer func() {
			_ = server.Close()
		}()
		header := []byte{byte(len(payload) + 2), 0x00}
		_, err := server.Write(header)
		if err == nil {
			_, err = server.Write(payload)
		}
		if err != nil {
			t.Errorf("frame write: %v", err)
		}
	}()

	got, err := readWirePacket(client)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}
