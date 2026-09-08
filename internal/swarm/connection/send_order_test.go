// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
	"github.com/stretchr/testify/require"
)

// TestGameClientConcurrentSendKeepsCipherOrder is the regression test of
// the send path race (see docs/development_log.md, 2026-09-08): the game
// cipher is a stateful rolling XOR chain, so the encryption order of the
// outbound packets must match the wire write order exactly. The send
// calls come from two goroutines in production (the run loop pings and
// answers teleports, the hunt loop attacks and walks), so serializing
// only the wire write while encrypting outside the lock desynced the
// chain: the server then decrypted garbage.
//
// The drain side mirrors the session cipher one to one, which turns any
// order violation into a wrong first opcode byte of the affected and all
// subsequent frames, so the test fails without the race detector too;
// under `-race` (task test:race, needs cgo) it also flags the raw data
// race of the pre-fix code.
func TestGameClientConcurrentSendKeepsCipherOrder(t *testing.T) {
	const goroutines = 8
	const perGoroutine = 50
	const total = goroutines * perGoroutine

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	opcodes := make(chan byte, total)
	drainErr := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			drainErr <- acceptErr

			return
		}
		defer conn.Close()
		drainErr <- drainEncryptedPings(conn, total, opcodes)
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	client, err := NewGameClient(conn)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				sendErr := client.sendPacket(&togameserver.RequestNetPing{})
				if sendErr != nil {
					t.Errorf("concurrent send failed: %v", sendErr)

					return
				}
			}
		}()
	}
	wg.Wait()
	require.NoError(t, <-drainErr)
	close(opcodes)

	require.Len(t, opcodes, total)
	for opcode := range opcodes {
		require.Equal(t, byte(0xA8), opcode,
			"cipher order desynced: the server decrypted garbage")
	}
}

// drainEncryptedPings answers the client handshake with the default
// session key, then reads total framed packets, decrypts them with the
// mirrored cipher and pushes the first opcode byte of every payload
// into the channel. A divergent encryption order produces garbage
// opcodes on the affected and every subsequent frame.
func drainEncryptedPings(conn net.Conn, total int, opcodes chan<- byte) error {
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}

	// Mirror of the client handshake: read the protocol version frame,
	// reply the unencrypted key packet carrying the default key (the
	// same key packet the scripted fake game server sends, so the client
	// enables exactly the cipher mirrored below).
	if _, err := drainReadFrame(conn); err != nil {
		return err
	}
	key := crypt.DefaultGameCryptKey()
	body := append([]byte{0x00, 0x01}, key[:]...)
	body = binary.LittleEndian.AppendUint32(body, 2)
	body = binary.LittleEndian.AppendUint32(body, 1)
	// The wire framing carries the 2 byte size header (it includes
	// itself); the raw socket of this drain adds it by hand.
	keyPacket := binary.LittleEndian.AppendUint16(nil, uint16(len(body)+2))
	keyPacket = append(keyPacket, body...)
	if _, err := conn.Write(keyPacket); err != nil {
		return err
	}
	serverCrypt := crypt.NewGameCrypt(key)
	serverCrypt.Enable()

	for range total {
		frame, err := drainReadFrame(conn)
		if err != nil {
			return err
		}
		serverCrypt.Decrypt(frame)
		if len(frame) == 0 {
			return errEmptyDecryptedFrame
		}
		opcodes <- frame[0]
	}

	return nil
}

// errEmptyDecryptedFrame marks a frame that decrypted to nothing.
var errEmptyDecryptedFrame = errors.New("decrypted frame is empty")

// drainReadFrame reads one size-prefixed frame and returns its payload.
func drainReadFrame(conn net.Conn) ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	size := int(header[0]) | int(header[1])<<8
	payload := make([]byte, size-2)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}

	return payload, nil
}
