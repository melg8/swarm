// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// fakeRawSender records the packets the proxy transits to the real game
// server on behalf of a client.
type fakeRawSender struct {
	sent chan []byte
}

func newFakeRawSender() *fakeRawSender {
	return &fakeRawSender{sent: make(chan []byte, 64)}
}

func (f *fakeRawSender) SendRaw(payload []byte) error {
	copied := make([]byte, len(payload))
	copy(copied, payload)
	f.sent <- copied

	return nil
}

// fakeGameClient drives the client side of the game protocol exactly
// like the C1 client: unencrypted ProtocolVersion/KeyPacket exchange,
// then the stateful XOR cipher for everything else.
type fakeGameClient struct {
	t       *testing.T
	conn    net.Conn
	crypt   *crypt.GameCrypt
	readBuf []byte
}

func dialGame(t *testing.T, address string) *fakeGameClient {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	return &fakeGameClient{t: t, conn: conn, crypt: nil, readBuf: nil}
}

// handshake sends the protocol version and enables the cipher with the
// key of the answered KeyPacket.
func (c *fakeGameClient) handshake() {
	c.t.Helper()
	request := []byte{0x00}
	request = binary.LittleEndian.AppendUint32(request, 419)
	require.NoError(c.t, writeWirePacket(c.conn, request))

	keyPacket, err := readWirePacket(c.conn, c.readBuf)
	require.NoError(c.t, err)
	c.readBuf = keyPacket
	require.Equal(c.t, byte(0x00), keyPacket[0], "key packet opcode")
	require.Equal(c.t, byte(0x01), keyPacket[1], "key packet result")
	var key [crypt.GameCryptKeySize]byte
	copy(key[:], keyPacket[2:10])
	c.crypt = crypt.NewGameCrypt(key)
	c.crypt.Enable()
}

// sendPacket encrypts and sends one client packet payload.
func (c *fakeGameClient) sendPacket(payload []byte) {
	c.t.Helper()
	copied := make([]byte, len(payload))
	copy(copied, payload)
	c.crypt.Encrypt(copied)
	require.NoError(c.t, writeWirePacket(c.conn, copied))
}

// readPacket reads and decrypts one server packet payload.
func (c *fakeGameClient) readPacket() []byte {
	c.t.Helper()
	payload, err := readWirePacket(c.conn, c.readBuf)
	require.NoError(c.t, err)
	c.readBuf = payload
	c.crypt.Decrypt(payload)

	return payload
}

// buildTestCharList builds a one character char list packet.
func buildTestCharList(name string) []byte {
	info := fromgameserver.CharacterInfo{
		Name:        name,
		ObjectID:    1055,
		Account:     "testbot",
		SessionID:   0,
		ClanID:      0,
		Sex:         0,
		Race:        1,
		BaseClassID: 18,
		X:           45000,
		Y:           50000,
		Z:           -3500,
		CurrentHP:   90,
		CurrentMP:   40,
		Level:       7,
		HairStyle:   0,
		HairColor:   0,
		Face:        0,
		MaxHP:       120,
		MaxMP:       50,
		DeleteTimer: 0,
	}
	list := &fromgameserver.CharSelectInfoPacket{
		Count:      1,
		Characters: []fromgameserver.CharacterInfo{info},
	}
	writer := packet.NewWriter()
	if err := list.ToBytes(writer); err != nil {
		panic(err)
	}

	return writer.Bytes()
}

// buildTestCharSelected builds a char selected packet with the full
// field tail the client parser expects.
func buildTestCharSelected(name string) []byte {
	data := []byte{0x21}
	data = appendUTF16(data, name)
	data = binary.LittleEndian.AppendUint32(data, 1055) // object id
	data = appendUTF16(data, "")                        // title
	data = binary.LittleEndian.AppendUint32(data, 42)   // session id
	for range 5 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}
	data = binary.LittleEndian.AppendUint32(data, 1) // active
	data = binary.LittleEndian.AppendUint32(data, 45000)
	data = binary.LittleEndian.AppendUint32(data, 50000)
	data = binary.LittleEndian.AppendUint32(data, 0xFFFFF268)
	data = binary.LittleEndian.AppendUint64(data, 90) // cur hp
	data = binary.LittleEndian.AppendUint64(data, 40) // cur mp

	return data
}

// appendUTF16 appends a null terminated UTF-16LE string.
func appendUTF16(dst []byte, value string) []byte {
	for _, r := range value {
		dst = append(dst, byte(r), 0)
	}

	return append(dst, 0, 0)
}

// buildTestWorldPacket builds a simple world observation packet with
// the given opcode and object id.
func buildTestWorldPacket(opcode byte, objectID int32) []byte {
	data := []byte{opcode}
	data = binary.LittleEndian.AppendUint32(data, uint32(objectID))

	return data
}

// registerFakeBot publishes a bot session with an online character and
// the given recorded history fed by the test.
func registerFakeBot(t *testing.T, server *Server, name string) (
	*Recorder, *state.Bot, *fakeRawSender,
) {
	t.Helper()
	tracker := state.NewBot("testbot")
	tracker.SetCharacter(name, 1055, 18, 45000, 50000, -3500, 90, 40)
	// The level arrives with the first UserInfo of the enter world burst.
	tracker.ApplyUserInfo(state.UserInfo{
		Name:    name,
		Level:   7,
		Race:    1,
		ClassID: 18,
		X:       45000,
		Y:       50000,
		Z:       -3500,
		CurHP:   90,
		MaxHP:   120,
		CurMP:   40,
		MaxMP:   50,
		Sp:      5,
		Exp:     1000,
	})
	tracker.SetOnline(name)

	sender := newFakeRawSender()
	recorder := server.RegisterSession("testbot", sender, tracker)
	t.Cleanup(func() { server.UnregisterSession("testbot") })

	return recorder, tracker, sender
}

// TestGameServerServesFullClientFlow walks a fake C1 client through the
// complete emulated game flow: handshake, any-credential auth login, the
// one character list, the char selection answered with the recorded
// packet, the enter world replay, the live relay and the client packet
// transit to the real server.
func TestGameServerServesFullClientFlow(t *testing.T) {
	server := startTestServer(t)
	recorder, tracker, sender := registerFakeBot(t, server, "ProxyChar")

	recorder.Record(buildTestCharList("ProxyChar"))
	recorder.Record(buildTestCharSelected("ProxyChar"))
	recorder.Record(buildTestWorldPacket(0x04, 1055)) // UserInfo
	recorder.Record(buildTestWorldPacket(0x22, 42))   // NpcInfo

	client := dialGame(t, server.GameAddr())
	client.handshake()

	// AuthLogin with arbitrary session keys must be accepted.
	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "whatever")
	for range 4 {
		authLogin = binary.LittleEndian.AppendUint32(authLogin, 7)
	}
	client.sendPacket(authLogin)

	charListPayload := client.readPacket()
	require.Equal(t, byte(0x1F), charListPayload[0], "char list opcode")
	charList := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(charList, charListPayload))
	require.EqualValues(t, 1, charList.Count)
	require.Len(t, charList.Characters, 1)
	require.Equal(t, "ProxyChar", charList.Characters[0].Name)
	require.EqualValues(t, 18, charList.Characters[0].BaseClassID)
	require.EqualValues(t, 7, charList.Characters[0].Level)
	require.InDelta(t, 90, charList.Characters[0].CurrentHP, 0.001)

	// Character selection returns the recorded char selected packet.
	client.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selectedPayload := client.readPacket()
	require.Equal(t, byte(0x21), selectedPayload[0], "char selected opcode")
	selected := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(selected, selectedPayload))
	require.Equal(t, "ProxyChar", selected.Name)
	require.EqualValues(t, 1055, selected.ObjectID)

	// The bot stays untouched by the client handshake.
	require.Zero(t, tracker.Packets())

	// Enter world replays the recorded stream.
	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0], "user info replay")
	require.Equal(t, byte(0x22), client.readPacket()[0], "npc info replay")

	// Packets recorded after the attach arrive as the live feed.
	recorder.Record(buildTestWorldPacket(0x01, 42))
	require.Equal(t, byte(0x01), client.readPacket()[0], "live relay")

	// A client packet transits to the real game server unchanged.
	move := []byte{0x01}
	move = binary.LittleEndian.AppendUint32(move, 100)
	move = binary.LittleEndian.AppendUint32(move, 200)
	move = binary.LittleEndian.AppendUint32(move, 0xFFFFF268)
	client.sendPacket(move)
	select {
	case received := <-sender.sent:
		require.Equal(t, move, received)
	case <-time.After(time.Second):
		t.Fatal("the client packet never reached the bot session")
	}

	// The end of the bot session disconnects the client.
	recorder.Close()
	_, err := client.conn.Read(make([]byte, 1))
	require.Error(t, err, "the client connection must close with the session")
}

// TestGameServerRejectsWrongProtocol pins the handshake behavior for a
// client with a different protocol version.
func TestGameServerRejectsWrongProtocol(t *testing.T) {
	server := startTestServer(t)
	registerFakeBot(t, server, "ProxyChar")

	client := dialGame(t, server.GameAddr())
	request := []byte{0x00}
	request = binary.LittleEndian.AppendUint32(request, 999)
	require.NoError(t, writeWirePacket(client.conn, request))

	keyPacket, err := readWirePacket(client.conn, client.readBuf)
	require.NoError(t, err)
	require.Equal(t, byte(0x00), keyPacket[0])
	require.Equal(t, byte(0x00), keyPacket[1], "the key packet must reject")

	_, err = client.conn.Read(make([]byte, 1))
	require.Error(t, err, "the connection must close after the rejection")
}

// TestGameServerSelectsTheHighlightedBot verifies the web UI selection:
// a client attaches to the selected bot even when it is not the first
// registered session.
func TestGameServerSelectsTheHighlightedBot(t *testing.T) {
	server := startTestServer(t)

	first := state.NewBot("bot1")
	first.SetCharacter("FirstChar", 1, 18, 1, 2, 3, 10, 10)
	first.ApplyUserInfo(state.UserInfo{
		Name:    "FirstChar",
		Level:   5,
		Race:    1,
		ClassID: 18,
		X:       1,
		Y:       2,
		Z:       3,
		CurHP:   10,
		MaxHP:   30,
		CurMP:   10,
		MaxMP:   30,
	})
	first.SetOnline("FirstChar")
	recorder1 := server.RegisterSession("bot1", newFakeRawSender(), first)
	t.Cleanup(func() { server.UnregisterSession("bot1") })
	recorder1.Record(buildTestCharSelected("FirstChar"))

	second := state.NewBot("bot2")
	second.SetCharacter("SecondChar", 2, 18, 4, 5, 6, 20, 20)
	second.ApplyUserInfo(state.UserInfo{
		Name:    "SecondChar",
		Level:   9,
		Race:    1,
		ClassID: 18,
		X:       4,
		Y:       5,
		Z:       6,
		CurHP:   20,
		MaxHP:   40,
		CurMP:   20,
		MaxMP:   40,
	})
	second.SetOnline("SecondChar")
	recorder2 := server.RegisterSession("bot2", newFakeRawSender(), second)
	t.Cleanup(func() { server.UnregisterSession("bot2") })
	recorder2.Record(buildTestCharSelected("SecondChar"))

	// Without a selection the first session serves the client.
	require.Empty(t, server.SelectedBot())

	// Selecting the second bot switches the session the client gets.
	server.SelectBot("bot2")
	require.Equal(t, "bot2", server.SelectedBot())

	client := dialGame(t, server.GameAddr())
	client.handshake()

	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "user")
	for range 4 {
		authLogin = binary.LittleEndian.AppendUint32(authLogin, 0)
	}
	client.sendPacket(authLogin)

	charListPayload := client.readPacket()
	charList := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(charList, charListPayload))
	require.Len(t, charList.Characters, 1)
	require.Equal(t, "SecondChar", charList.Characters[0].Name)

	// The second recorder holds the CharSelected packet the char
	// selection must answer with.
	client.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selectedPayload := client.readPacket()
	require.Equal(t, byte(0x21), selectedPayload[0])
	selected := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(selected, selectedPayload))
	require.Equal(t, "SecondChar", selected.Name)
}

// TestGameServerRefusesCharacterCreation answers creation attempts with
// the failure packet so the client cannot alter the bot account.
func TestGameServerRefusesCharacterCreation(t *testing.T) {
	server := startTestServer(t)
	recorder, _, _ := registerFakeBot(t, server, "ProxyChar")
	recorder.Record(buildTestCharSelected("ProxyChar"))

	client := dialGame(t, server.GameAddr())
	client.handshake()

	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "user")
	for range 4 {
		authLogin = binary.LittleEndian.AppendUint32(authLogin, 0)
	}
	client.sendPacket(authLogin)
	require.Equal(t, byte(0x1F), client.readPacket()[0])

	client.sendPacket([]byte{0x0B, 0x00, 0x00, 0x00, 0x00})
	failPayload := client.readPacket()
	require.Equal(t, byte(0x26), failPayload[0], "char create fail opcode")
	fail := fromgameserver.NewCharCreateFailPacket()
	require.NoError(t, fromgameserver.ParseCharCreateFailPacket(fail, failPayload))
	require.EqualValues(t, 0x01, fail.Reason)
}
