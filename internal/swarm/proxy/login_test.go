// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strconv"
	"testing"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromauthserver "github.com/melg8/swarm/internal/swarm/packets/from_auth_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
	"github.com/stretchr/testify/require"
)

// startTestServer starts a proxy on random ports with a discarding
// logger (swap in a verbose one when debugging a failing test).
func startTestServer(t *testing.T) *Server {
	t.Helper()
	server := NewServer(
		log.New(io.Discard, "", 0),
		WithLoginAddresses("127.0.0.1:0"),
		WithGameAddresses("127.0.0.1:0"),
	)
	require.NoError(t, server.Listen())
	go func() {
		_ = server.Serve()
	}()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})

	return server
}

// fakeLoginClient drives the client side of the login protocol exactly
// like the C1 client does (static Blowfish key framing).
type fakeLoginClient struct {
	t        *testing.T
	conn     net.Conn
	crypt    *crypt.LoginCrypt
	readBuf  []byte
	writeBuf []byte
	loginOk1 int32
	loginOk2 int32
}

func dialLogin(t *testing.T, address string) *fakeLoginClient {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return &fakeLoginClient{
		t:        t,
		conn:     conn,
		crypt:    crypt.NewLoginCrypt(crypt.MobiusAuthKey()),
		readBuf:  nil,
		writeBuf: nil,
		loginOk1: 0,
		loginOk2: 0,
	}
}

// readInit reads the unencrypted init packet of the emulated server.
func (c *fakeLoginClient) readInit() *fromauthserver.InitPacket {
	c.t.Helper()
	payload, err := readWirePacket(c.conn, c.readBuf)
	require.NoError(c.t, err)
	c.readBuf = payload
	require.GreaterOrEqual(c.t, len(payload), 1+4+4+128)
	require.Equal(c.t, byte(0x00), payload[0], "init opcode")

	initPacket := fromauthserver.NewInitPacket()
	require.NoError(c.t, fromauthserver.ParseInitPacket(initPacket, payload[1:]))
	require.EqualValues(c.t, 0x0000c621, initPacket.ProtocolVersion)
	require.Len(c.t, initPacket.RsaPublicKey, 128)

	return initPacket
}

// readPacket reads and decrypts one sealed login packet.
func (c *fakeLoginClient) readPacket() []byte {
	c.t.Helper()
	payload, err := readWirePacket(c.conn, c.readBuf)
	require.NoError(c.t, err)
	c.readBuf = payload
	content, err := c.crypt.Open(payload)
	require.NoError(c.t, err)

	return content
}

// sendPacket seals and sends one login packet.
func (c *fakeLoginClient) sendPacket(p crypt.Serializable) {
	c.t.Helper()
	writer := packet.NewWriter()
	require.NoError(c.t, p.ToBytes(writer))
	wire, err := c.crypt.Seal(c.writeBuf, writer.Bytes())
	require.NoError(c.t, err)
	c.writeBuf = wire
	_, err = c.conn.Write(wire)
	require.NoError(c.t, err)
}

// login accepts any credentials and returns the parsed LoginOk packet.
func (c *fakeLoginClient) login(account, password string) *fromauthserver.LoginOkPacket {
	c.t.Helper()
	c.sendPacket(&toauthserver.RequestAuthLogin{Account: account, Password: password})

	content := c.readPacket()
	c.t.Logf("login ok content: % x", content)
	require.Equal(c.t, byte(0x03), content[0], "login ok opcode")
	loginOk := fromauthserver.NewLoginOkPacket()
	require.NoError(c.t, fromauthserver.ParseLoginOkPacket(loginOk, content))
	c.loginOk1 = loginOk.LoginOkID1
	c.loginOk2 = loginOk.LoginOkID2

	return loginOk
}

// requestServerList returns the server list of the emulated server.
func (c *fakeLoginClient) requestServerList() *fromauthserver.ServerListPacket {
	c.t.Helper()
	c.sendPacket(toauthserver.NewRequestServerList(c.loginOk1, c.loginOk2))

	content := c.readPacket()
	require.Equal(c.t, byte(0x04), content[0], "server list opcode")
	list := fromauthserver.NewServerListPacket()
	require.NoError(c.t, fromauthserver.ParseServerListPacket(list, content))

	return list
}

// requestServerLogin claims the game server slot and returns PlayOk.
func (c *fakeLoginClient) requestServerLogin(serverID byte) *fromauthserver.PlayOkPacket {
	c.t.Helper()
	c.sendPacket(&toauthserver.RequestServerLogin{
		LoginOkID1: c.loginOk1,
		LoginOkID2: c.loginOk2,
		ServerID:   int8(serverID),
	})

	content := c.readPacket()
	require.Equal(c.t, byte(0x07), content[0], "play ok opcode")
	playOk := fromauthserver.NewPlayOkPacket()
	require.NoError(c.t, fromauthserver.ParsePlayOkPacket(playOk, content))

	return playOk
}

func TestLoginServerAcceptsAnyCredentials(t *testing.T) {
	server := startTestServer(t)

	client := dialLogin(t, server.LoginAddr())
	client.readInit()

	loginOk := client.login("totally", "wrong")
	require.NotZero(t, loginOk.LoginOkID1)
	require.NotZero(t, loginOk.LoginOkID2)
}

func TestLoginServerFullLoginFlow(t *testing.T) {
	server := startTestServer(t)

	client := dialLogin(t, server.LoginAddr())
	client.readInit()
	client.login("anyuser", "anypass")

	list := client.requestServerList()
	require.EqualValues(t, 1, list.Count)
	require.Len(t, list.Servers, 1)
	entry := list.Servers[0]
	require.EqualValues(t, 1, entry.ServerID)
	// The advertised port must match the actual game listener (a random
	// port in tests, the default 7778 in production).
	_, advertisedPort, err := net.SplitHostPort(server.GameAddr())
	require.NoError(t, err)
	require.Equal(t, advertisedPort, strconv.Itoa(int(entry.Port)))
	require.NotZero(t, entry.Status, "the proxy game server must be up")
	require.Equal(t, [4]byte{127, 0, 0, 1}, entry.IP)

	playOk := client.requestServerLogin(byte(entry.ServerID))
	require.NotZero(t, playOk.PlayOkID1)
	require.NotZero(t, playOk.PlayOkID2)
}

func TestLoginServerAnswersGGAuth(t *testing.T) {
	server := startTestServer(t)

	client := dialLogin(t, server.LoginAddr())
	initPacket := client.readInit()
	client.login("gguser", "ggpass")

	// RequestGGAuth: [0x07][sessionId: 4][4 unknown ints].
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(0x07))
	require.NoError(t, writer.WriteInt32(initPacket.SessionID))
	for range 4 {
		require.NoError(t, writer.WriteInt32(0))
	}
	client.sendPacket(&rawSerializable{payload: writer.Bytes()})

	content := client.readPacket()
	// The login framing pads the payload to the 8 byte alignment: the
	// content carries trailing zero filler after the packet fields.
	require.GreaterOrEqual(t, len(content), 9)
	require.Equal(t, byte(0x0B), content[0], "gg auth opcode")
	require.EqualValues(t, initPacket.SessionID,
		binary.LittleEndian.Uint32(content[1:5]))
	require.Equal(t, []byte{0, 0, 0, 0}, content[5:9])
}

// rawSerializable sends predefined content bytes through the sealed
// send path of the fake client.
type rawSerializable struct {
	payload []byte
}

func (r *rawSerializable) ToBytes(writer *packet.Writer) error {
	return writer.WriteBytes(r.payload)
}
