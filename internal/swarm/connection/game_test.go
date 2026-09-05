// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/stretchr/testify/require"
)

// fakeGameServer implements the server side of the game protocol handshake
// and character flow.
type fakeGameServer struct {
	listener net.Listener
	t        *testing.T
}

// startFakeGameServer starts a scripted game server on a random port.
func startFakeGameServer(t *testing.T) *fakeGameServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &fakeGameServer{listener: listener, t: t}
	go server.serve()

	return server
}

// Addr returns the address of the fake server.
func (s *fakeGameServer) Addr() string {
	return s.listener.Addr().String()
}

// serve runs a single scripted session.
func (s *fakeGameServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	serverCrypt := s.handshake(conn)
	s.characterFlow(conn, serverCrypt)
	s.worldFlow(conn, serverCrypt)
}

// handshake expects the protocol version and returns the session cipher.
func (s *fakeGameServer) handshake(conn net.Conn) *crypt.GameCrypt {
	payload := s.readPacket(conn)
	require.Equal(s.t, byte(0x00), payload[0])
	version := binary.LittleEndian.Uint32(payload[1:5])
	require.Equal(s.t, uint32(419), version)

	// Reply with an unencrypted key packet.
	var keyPacket []byte
	keyPacket = append(keyPacket, 0x00, 0x01)
	gameKey := crypt.DefaultGameCryptKey()
	keyPacket = append(keyPacket, gameKey[:]...)
	keyPacket = binary.LittleEndian.AppendUint32(keyPacket, 2)
	keyPacket = binary.LittleEndian.AppendUint32(keyPacket, 1)
	s.writePacket(conn, keyPacket)

	// The rest of the session is encrypted with the default key.
	serverCrypt := crypt.NewGameCrypt(gameKey)
	serverCrypt.Enable()

	return serverCrypt
}

// characterFlow exchanges the auth login, creation and selection packets.
func (s *fakeGameServer) characterFlow(conn net.Conn, cipher *crypt.GameCrypt) {
	payload := s.readEncrypted(conn, cipher)
	require.Equal(s.t, byte(0x08), payload[0])
	login := readUtf16String(payload[1:])
	require.Equal(s.t, "test1", login)

	var charList []byte
	charList = append(charList, 0x1F)
	charList = binary.LittleEndian.AppendUint32(charList, 0)
	s.writeEncrypted(conn, cipher, charList)

	payload = s.readEncrypted(conn, cipher)
	require.Equal(s.t, byte(0x0B), payload[0])
	name := readUtf16String(payload[1:])
	require.Equal(s.t, "test1", name)
	race := binary.LittleEndian.Uint32(payload[1+len("test1")*2+2:])
	require.Equal(s.t, uint32(1), race, "elf race id")

	s.writeEncrypted(conn, cipher,
		append([]byte{0x25}, 0x01, 0x00, 0x00, 0x00))

	var updated []byte
	updated = append(updated, 0x1F)
	updated = binary.LittleEndian.AppendUint32(updated, 1)
	updated = append(updated, buildCharacterEntry()...)
	s.writeEncrypted(conn, cipher, updated)

	payload = s.readEncrypted(conn, cipher)
	require.Equal(s.t, byte(0x0D), payload[0])
	slot := binary.LittleEndian.Uint32(payload[1:])
	require.Equal(s.t, uint32(0), slot)
}

// worldFlow exchanges the enter world and shutdown packets.
func (s *fakeGameServer) worldFlow(conn net.Conn, cipher *crypt.GameCrypt) {
	var selected []byte
	selected = append(selected, 0x21)
	selected = append(selected, utf16Bytes("test1")...)
	selected = binary.LittleEndian.AppendUint32(selected, 100)
	selected = append(selected, utf16Bytes("")...)
	selected = binary.LittleEndian.AppendUint32(selected, 42)
	for range 5 {
		selected = binary.LittleEndian.AppendUint32(selected, 0)
	}
	s.writeEncrypted(conn, cipher, selected)

	payload := s.readEncrypted(conn, cipher)
	require.Equal(s.t, byte(0x03), payload[0])

	s.writeEncrypted(conn, cipher,
		append([]byte{0xEC}, 0x0A, 0x00, 0x00, 0x00))

	// Expect the logout packet when the client stops.
	for {
		payload, err := s.readEncryptedResult(conn, cipher)
		if err != nil {
			return
		}
		if len(payload) > 0 && payload[0] == 0x09 {
			return
		}
	}
}

// readPacket reads one unencrypted framed packet.
func (s *fakeGameServer) readPacket(conn net.Conn) []byte {
	var header [2]byte
	_, err := io.ReadFull(conn, header[:])
	require.NoError(s.t, err)

	size := int(binary.LittleEndian.Uint16(header[:]))
	payload := make([]byte, size-2)
	_, err = io.ReadFull(conn, payload)
	require.NoError(s.t, err)

	return payload
}

// readEncrypted reads and decrypts one framed packet.
func (s *fakeGameServer) readEncrypted(
	conn net.Conn, cipher *crypt.GameCrypt,
) []byte {
	payload, err := s.readEncryptedResult(conn, cipher)
	require.NoError(s.t, err)

	return payload
}

// readEncryptedResult reads and decrypts one framed packet with an error.
func (s *fakeGameServer) readEncryptedResult(
	conn net.Conn, cipher *crypt.GameCrypt,
) ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}

	size := int(binary.LittleEndian.Uint16(header[:]))
	if size < 2 {
		return nil, nil
	}
	payload := make([]byte, size-2)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	cipher.Decrypt(payload)

	return payload, nil
}

// writePacket writes one unencrypted framed packet.
func (s *fakeGameServer) writePacket(conn net.Conn, payload []byte) {
	s.writeFrame(conn, payload)
}

// writeEncrypted encrypts and writes one framed packet.
func (s *fakeGameServer) writeEncrypted(
	conn net.Conn, cipher *crypt.GameCrypt, payload []byte,
) {
	// The first server packet after the key packet is already encrypted.
	cipher.Encrypt(payload)
	s.writeFrame(conn, payload)
}

// writeFrame prepends the size header and writes the payload.
func (s *fakeGameServer) writeFrame(conn net.Conn, payload []byte) {
	frame := make([]byte, 0, len(payload)+2)
	var header [2]byte
	//nolint:gosec // the frame length fits the protocol limit
	binary.LittleEndian.PutUint16(header[:], uint16(len(payload)+2))
	frame = append(frame, header[:]...)
	frame = append(frame, payload...)
	_, err := conn.Write(frame)
	require.NoError(s.t, err)
}

// utf16Bytes encodes a string as null terminated utf16le.
func utf16Bytes(value string) []byte {
	result := make([]byte, 0, len(value)*2+2)
	for _, r := range value {
		result = append(result, byte(r), byte(r>>8))
	}
	result = append(result, 0, 0)

	return result
}

// readUtf16String decodes a null terminated utf16le string.
func readUtf16String(data []byte) string {
	result := make([]byte, 0, len(data))
	for i := 0; i+1 < len(data); i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			break
		}
		result = append(result, data[i])
	}

	return string(result)
}

// buildCharacterEntry builds a binary char list entry for the bot character.
func buildCharacterEntry() []byte {
	data := utf16Bytes("test1")
	data = binary.LittleEndian.AppendUint32(data, 100)
	data = append(data, utf16Bytes("test1")...)
	data = binary.LittleEndian.AppendUint32(data, 0) // session id
	data = binary.LittleEndian.AppendUint32(data, 0) // clan
	data = binary.LittleEndian.AppendUint32(data, 0) // builder
	data = binary.LittleEndian.AppendUint32(data, 0) // sex
	data = binary.LittleEndian.AppendUint32(data, 1) // race
	data = binary.LittleEndian.AppendUint32(data, 18)
	data = binary.LittleEndian.AppendUint32(data, 1) // gs name
	data = binary.LittleEndian.AppendUint32(data, 100)
	data = binary.LittleEndian.AppendUint32(data, 200)
	data = binary.LittleEndian.AppendUint32(data, 300)
	data = binary.LittleEndian.AppendUint64(data, 50) // cur hp
	data = binary.LittleEndian.AppendUint64(data, 30) // cur mp
	data = binary.LittleEndian.AppendUint32(data, 0)  // sp
	data = binary.LittleEndian.AppendUint32(data, 0)  // exp
	data = binary.LittleEndian.AppendUint32(data, 1)  // level
	for range 40 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}
	data = binary.LittleEndian.AppendUint32(data, 2)  // hair style
	data = binary.LittleEndian.AppendUint32(data, 1)  // hair color
	data = binary.LittleEndian.AppendUint32(data, 0)  // face
	data = binary.LittleEndian.AppendUint64(data, 50) // max hp
	data = binary.LittleEndian.AppendUint64(data, 30) // max mp
	data = binary.LittleEndian.AppendUint32(data, 0)  // delete timer

	return data
}

func TestGameClientFullFlow(t *testing.T) {
	server := startFakeGameServer(t)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	client, err := NewGameClient(conn)
	require.NoError(t, err)
	require.NotNil(t, client.crypt)

	charList, err := client.Authenticate(GameSessionParams{
		Account:    "test1",
		LoginOkID1: 1,
		LoginOkID2: 2,
		PlayOkID1:  3,
		PlayOkID2:  4,
	})
	require.NoError(t, err)
	require.Empty(t, charList.Characters)

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

	slot, info, found := updated.FindCharacterByName("test1")
	require.True(t, found)
	require.Equal(t, int32(18), info.BaseClassID)

	require.NoError(t, client.EnterWorld(int32(slot))) //nolint:gosec // small list

	// The client must stay in the world until the context is done.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = client.Run(ctx, "test1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, client.PacketCount(), 1)
}
