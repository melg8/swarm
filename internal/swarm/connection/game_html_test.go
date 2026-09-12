// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/stretchr/testify/require"
)

// buildNpcHTMLPayload builds a raw NpcHtmlMessage payload (opcode 0x1B
// + npc obj id + html + item id) the way the server sends it, without
// going through the packet serializer (the test owns the byte order).
func buildNpcHTMLPayload(npcObjID int32, html string, itemID int32,
) []byte {
	var data []byte
	data = append(data, 0x1B)
	var id [4]byte
	binary.LittleEndian.PutUint32(id[:], uint32(npcObjID))
	data = append(data, id[:]...)
	for i := range len(html) {
		data = append(data, html[i], 0)
	}
	data = append(data, 0, 0) // null terminator
	binary.LittleEndian.PutUint32(id[:], uint32(itemID))
	data = append(data, id[:]...)

	return data
}

// newHTMLTestClient builds a GameClient with just the fields the html
// dispatch path needs (no handshake, no live socket): a logger and the
// zero scratch structs. The dispatch and the LastHTMLMessage getter
// never touch the conn or the crypt.
func newHTMLTestClient(logger *log.Logger) *GameClient {
	return &GameClient{
		logger:  logger,
		npcHTML: *fromgameserver.NewNpcHTMLMessage(),
		htmlMu:  sync.Mutex{},
	}
}

// TestApplyNpcHTMLMessage pins the dispatch: a 0x1B packet routes to
// applyNpcHTMLMessage, the parse fills the scratch, LastHTMLMessage
// returns the copy the hunt loop reads.
func TestApplyNpcHTMLMessage(t *testing.T) {
	client := newHTMLTestClient(log.New(&testLogBuffer{}, "", 0))

	payload := buildNpcHTMLPayload(30146, "teleport list", 0)
	client.handleServerPacket(payload)

	html := client.LastHTMLMessage()
	require.Equal(t, int32(30146), html.NpcObjID)
	require.Equal(t, "teleport list", html.HTML)
	require.Equal(t, int32(0), html.ItemID)
}

// TestApplyNpcHTMLMessageItemID pins the item html variant: a non-zero
// item id rides the tail and survives the LastHTMLMessage copy.
func TestApplyNpcHTMLMessageItemID(t *testing.T) {
	client := newHTMLTestClient(log.New(&testLogBuffer{}, "", 0))

	payload := buildNpcHTMLPayload(42, "x", 1001)
	client.handleServerPacket(payload)

	html := client.LastHTMLMessage()
	require.Equal(t, int32(42), html.NpcObjID)
	require.Equal(t, int32(1001), html.ItemID)
}

// TestApplyNpcHTMLMessageMalformedLogsAndSkips pins the error path: a
// truncated 0x1B packet logs the parse failure and does not clobber the
// last good html.
func TestApplyNpcHTMLMessageMalformedLogsAndSkips(t *testing.T) {
	logBuf := &testLogBuffer{}
	client := newHTMLTestClient(log.New(logBuf, "", 0))

	// Seed a good html first.
	good := buildNpcHTMLPayload(30146, "good", 0)
	client.handleServerPacket(good)
	// A truncated packet (only the opcode) fails the parse.
	client.handleServerPacket([]byte{0x1B})
	require.Contains(t, logBuf.String(), "Failed to parse npc html")
	// The last good html survives.
	html := client.LastHTMLMessage()
	require.Equal(t, int32(30146), html.NpcObjID)
	require.Equal(t, "good", html.HTML)
}

// TestSendBypassWritesRequestBypassToServer pins the outbound side:
// SendBypass serializes a 0x21 packet with the command and writes it
// through the encrypted game channel. The fake peer reads the frame,
// decrypts it and verifies the opcode and the command string.
func TestSendBypassWritesRequestBypassToServer(t *testing.T) {
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		_ = connA.Close()
		_ = connB.Close()
	})
	key := crypt.DefaultGameCryptKey()
	clientCrypt := crypt.NewGameCrypt(key)
	clientCrypt.Enable()
	client := &GameClient{
		conn:    connA,
		crypt:   clientCrypt,
		writeMu: sync.Mutex{},
		logger:  log.New(&testLogBuffer{}, "", 0),
	}
	serverCrypt := crypt.NewGameCrypt(key)
	serverCrypt.Enable()

	done := make(chan []byte, 1)
	go func() {
		done <- readEncryptedFrameForTest(t, connB, serverCrypt)
	}()

	require.NoError(t, client.SendBypass("npc_30146_teleport NORMAL 0"))
	select {
	case payload := <-done:
		require.Equal(t, byte(0x21), payload[0],
			"the opcode must be RequestBypassToServer 0x21")
		require.Equal(t, "npc_30146_teleport NORMAL 0",
			readUtf16String(payload[1:]))
	case <-time.After(time.Second):
		t.Fatal("the bypass packet never arrived")
	}
}

// TestSendBypassEmptyRefused pins the guard: an empty command is
// refused before the wire write (the server disconnects on an empty
// bypass).
func TestSendBypassEmptyRefused(t *testing.T) {
	client := newHTMLTestClient(log.New(&testLogBuffer{}, "", 0))
	require.Error(t, client.SendBypass(""))
}

// readEncryptedFrameForTest reads one wire frame from the connection,
// decrypts it with the given cipher and returns the payload. The
// helper mirrors the fake server readEncrypted path for the pipe test
// (no listener needed).
func readEncryptedFrameForTest(
	t *testing.T, conn net.Conn, cipher *crypt.GameCrypt,
) []byte {
	t.Helper()
	header := make([]byte, wireHeaderSize)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read header: %v", err)
	}
	size := int(wireEndian.Uint16(header))
	if size < wireHeaderSize {
		t.Fatalf("invalid size %d", size)
	}
	payload := make([]byte, size-wireHeaderSize)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	cipher.Decrypt(payload)

	return payload
}

// testLogBuffer is a minimal io.Writer for the test logger.
type testLogBuffer struct {
	data []byte
}

func (b *testLogBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)

	return len(p), nil
}

func (b *testLogBuffer) String() string {
	return string(b.data)
}
