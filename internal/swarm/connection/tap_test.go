// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/stretchr/testify/require"
)

// tapCollector collects the payloads of a game session tap. The copy
// semantics mirror what the proxy recorder does: the payload must be
// copied synchronously because the read buffer is reused, and the
// collection must be safe for the concurrent readers of the setup phase
// and of the session run loop.
type tapCollector struct {
	mu       sync.Mutex
	payloads [][]byte
}

func (c *tapCollector) tap(payload []byte) {
	copied := make([]byte, len(payload))
	copy(copied, payload)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.payloads = append(c.payloads, copied)
}

// opcodesOf returns the first byte of every collected payload.
func (c *tapCollector) opcodes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	codes := make([]byte, 0, len(c.payloads))
	for _, payload := range c.payloads {
		codes = append(codes, payload[0])
	}

	return codes
}

// TestGameClientTapObservesFullSession verifies that the tap sees every
// decrypted server packet from the character list onward: the char list,
// the creation results, the char selected packet and the world packets.
func TestGameClientTapObservesFullSession(t *testing.T) {
	server := startFakeGameServer(t)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	collector := &tapCollector{}
	client, err := NewGameClient(conn)
	require.NoError(t, err)
	client.SetTap(collector.tap)

	charList, err := client.Authenticate(GameSessionParams{
		Account:    "test1",
		LoginOkID1: 1,
		LoginOkID2: 2,
		PlayOkID1:  3,
		PlayOkID2:  4,
	})
	require.NoError(t, err)

	updated, err := client.EnsureCharacter(CharacterParams{
		Name:      "test1",
		Race:      1,
		Female:    0,
		ClassID:   18,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}, charList)
	require.NoError(t, err)

	slot, _, found := updated.FindCharacterByName("test1")
	require.True(t, found)
	require.NoError(t, client.EnterWorld(int32(slot)))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	require.NoError(t, client.Run(ctx, "test1"))

	opcodes := collector.opcodes()
	require.Contains(t, opcodes, byte(0x1F), "char list not tapped")
	require.Contains(t, opcodes, byte(0x25), "char create ok not tapped")
	require.Contains(t, opcodes, byte(0x21), "char selected not tapped")
	require.Contains(t, opcodes, byte(0x22), "npc info not tapped")
	require.Contains(t, opcodes, byte(0xEC), "net ping not tapped")
	require.NotContains(t, opcodes, byte(0x00), "key packet must not be tapped")
}

// TestGameClientSendRawForwardsClientPackets verifies the raw send path:
// a payload injected through SendRaw lands on the server socket as a
// correctly encrypted client packet, sharing the cipher chain with the
// typed sends (the cipher offset stays consistent for the next packet).
// The fake server reports the received packets through a channel so the
// assertions run before the test finishes (a goroutine asserting after
// the test completion is both flaky and unsynchronized).
func TestGameClientSendRawForwardsClientPackets(t *testing.T) {
	rawRead := make(chan [][]byte, 1)
	server := startFakeGameServer(t)
	server.flow = func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt) {
		s.characterFlow(conn, cipher)

		var selected []byte
		selected = append(selected, 0x21)
		selected = append(selected, utf16Bytes("test1")...)
		selected = binaryLittleEndianAppendUint32(selected, 100)
		selected = append(selected, utf16Bytes("")...)
		selected = binaryLittleEndianAppendUint32(selected, 42)
		for range 5 {
			selected = binaryLittleEndianAppendUint32(selected, 0)
		}
		selected = binaryLittleEndianAppendUint32(selected, 1) // active
		selected = binaryLittleEndianAppendUint32(selected, 45000)
		selected = binaryLittleEndianAppendUint32(selected, 50000)
		selected = binaryLittleEndianAppendUint32(selected, 0xFFFFF268)
		selected = binaryLittleEndianAppendUint64(selected, 50) // cur hp
		selected = binaryLittleEndianAppendUint64(selected, 30) // cur mp
		s.writeEncrypted(conn, cipher, selected)

		// Enter world of the client.
		payload := s.readEncrypted(conn, cipher)
		require.Equal(s.t, byte(0x03), payload[0])

		// Both raw packets of the relay arrive before the flow returns.
		first := s.readEncrypted(conn, cipher)
		second := s.readEncrypted(conn, cipher)
		rawRead <- [][]byte{first, second}
	}

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	client, err := NewGameClient(conn)
	require.NoError(t, err)

	charList, err := client.Authenticate(GameSessionParams{
		Account:    "test1",
		LoginOkID1: 1,
		LoginOkID2: 2,
		PlayOkID1:  3,
		PlayOkID2:  4,
	})
	require.NoError(t, err)

	updated, err := client.EnsureCharacter(CharacterParams{
		Name:      "test1",
		Race:      1,
		Female:    0,
		ClassID:   18,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}, charList)
	require.NoError(t, err)

	slot, _, found := updated.FindCharacterByName("test1")
	require.True(t, found)
	require.NoError(t, client.EnterWorld(int32(slot)))

	// A MoveToLocation client packet: [0x01][x][y][z][ox][oy][oz]ode].
	raw := []byte{0x01}
	raw = appendLE32(raw, 100)
	raw = appendLE32(raw, 200)
	raw = appendLE32(raw, -3500)
	raw = appendLE32(raw, 0)
	raw = appendLE32(raw, 0)
	raw = appendLE32(raw, 0)
	raw = append(raw, 0)
	require.NoError(t, client.SendRaw(raw))

	// A second raw packet proves the cipher chain stays in sync.
	secondRaw := []byte{0x0D, 0x00, 0x00, 0x00, 0x00}
	require.NoError(t, client.SendRaw(secondRaw))

	received := <-rawRead
	require.Equal(t, raw, received[0], "first raw packet bytes")
	require.Equal(t, secondRaw, received[1], "second raw packet bytes")
}

// appendLE32 appends a little endian int32 value to the packet.
func appendLE32(dst []byte, value int32) []byte {
	dst = append(dst, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))

	return dst
}

// binaryLittleEndianAppendUint32 appends a uint32 value in little endian.
func binaryLittleEndianAppendUint32(dst []byte, value uint32) []byte {
	return append(dst, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

// binaryLittleEndianAppendUint64 appends a uint64 value in little endian.
func binaryLittleEndianAppendUint64(dst []byte, value uint64) []byte {
	for i := range 8 {
		dst = append(dst, byte(value>>(8*i)))
	}

	return dst
}
