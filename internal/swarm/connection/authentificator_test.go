// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/stretchr/testify/require"
)

// fakeLoginServer implements the server side of the Mobius login
// protocol: it sends the Init packet, then answers every client packet
// with the next scripted reply content.
type fakeLoginServer struct {
	listener net.Listener
	t        *testing.T
	// initContent is the unencrypted Init payload.
	initContent []byte
	// replies are the sealed reply contents, one per client packet.
	replies [][]byte
	// contents collects the decrypted client packets for assertions.
	contents [][]byte
	// done closes when the scripted session is over.
	done chan struct{}
}

// startFakeLoginServer starts the scripted login server.
func startFakeLoginServer(t *testing.T, server *fakeLoginServer) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server.listener = listener
	server.done = make(chan struct{})
	go server.serve()
}

// Addr returns the address of the fake login server.
func (s *fakeLoginServer) Addr() string {
	return s.listener.Addr().String()
}

// serve runs one scripted login session.
func (s *fakeLoginServer) serve() {
	defer close(s.done)

	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if err := writeWirePacket(conn, s.initContent); err != nil {
		return
	}

	cipher := crypt.NewLoginCrypt(crypt.MobiusAuthKey())
	for _, reply := range s.replies {
		payload, err := readWirePacket(conn, nil)
		if err != nil {
			return
		}
		content, err := cipher.Open(payload)
		if err != nil {
			return
		}
		s.contents = append(s.contents, content)
		if reply == nil {
			continue
		}
		wire, err := cipher.Seal(nil, reply)
		if err != nil {
			return
		}
		if _, err := conn.Write(wire); err != nil {
			return
		}
	}
}

// buildLoginInit builds the unencrypted Init packet content.
func buildLoginInit() []byte {
	data := []byte{0x00}
	data = binary.LittleEndian.AppendUint32(data, 0x7c610eca)
	data = binary.LittleEndian.AppendUint32(data, 0x0000c621)
	data = append(data, make([]byte, 128)...) // rsa key
	data = binary.LittleEndian.AppendUint32(data, 1)
	data = binary.LittleEndian.AppendUint32(data, 2)
	data = binary.LittleEndian.AppendUint32(data, 3)
	data = binary.LittleEndian.AppendUint32(data, 4)
	data = append(data, bytes.Repeat([]byte{0x5A}, 21)...) // blowfish key

	return data
}

// buildLoginOkContent builds the LoginOk reply content.
func buildLoginOkContent() []byte {
	data := []byte{0x03}
	data = binary.LittleEndian.AppendUint32(data, 11)
	data = binary.LittleEndian.AppendUint32(data, 22)

	return append(data, make([]byte, 5*4)...) // unused tail
}

// buildServerListContent builds a ServerList with one live server.
func buildServerListContent(count int) []byte {
	data := []byte{0x04, byte(count), 0x01} // opcode, count, last server
	for range count {
		data = append(data, 0x01)         // server id
		data = append(data, 127, 0, 0, 1) // ip
		data = binary.LittleEndian.AppendUint32(data, 7777)
		data = append(data, 0)                             // age limit
		data = append(data, 0)                             // pvp
		data = binary.LittleEndian.AppendUint16(data, 2)   // current
		data = binary.LittleEndian.AppendUint16(data, 100) // max
		data = append(data, 0x01)                          // status: up
		data = binary.LittleEndian.AppendUint32(data, 0)   // bits
		data = append(data, 0)                             // brackets
	}

	return data
}

// buildPlayOkContent builds the PlayOk reply content.
func buildPlayOkContent() []byte {
	data := []byte{0x07}
	data = binary.LittleEndian.AppendUint32(data, 33)
	data = binary.LittleEndian.AppendUint32(data, 44)

	return data
}

// buildLoginFailContent builds the LoginFail reply content for a reason.
func buildLoginFailContent(reason int32) []byte {
	data := []byte{0x01}

	return binary.LittleEndian.AppendUint32(data, uint32(reason))
}

// loginContentField returns the zero padded field at the given offset.
func loginContentField(content []byte, offset int, size int) string {
	field := content[offset : offset+size]
	if end := bytes.IndexByte(field, 0); end >= 0 {
		field = field[:end]
	}

	return string(field)
}

func TestAuthenticateFullFlow(t *testing.T) {
	server := &fakeLoginServer{
		initContent: buildLoginInit(),
		replies: [][]byte{
			buildLoginOkContent(),
			buildServerListContent(1),
			buildPlayOkContent(),
		},
	}
	startFakeLoginServer(t, server)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	result, err := Authenticate(conn, "test1", "test")
	require.NoError(t, err)
	require.Equal(t, "test1", result.Account)
	require.Equal(t, int32(11), result.LoginOkID1)
	require.Equal(t, int32(22), result.LoginOkID2)
	require.Equal(t, int32(33), result.PlayOkID1)
	require.Equal(t, int32(44), result.PlayOkID2)
	require.Equal(t, 1, result.ServerID)
	require.Equal(t, [4]byte{127, 0, 0, 1}, result.ServerIP)
	require.Equal(t, int32(7777), result.ServerPort)

	<-server.done
	require.Len(t, server.contents, 3)

	// The auth login carries the credentials in fixed 14 byte fields.
	require.Equal(t, byte(0x00), server.contents[0][0])
	require.Equal(t, "test1", loginContentField(server.contents[0], 1, 14))
	require.Equal(t, "test", loginContentField(server.contents[0], 15, 14))

	// The server list request repeats the login session keys.
	require.Equal(t, byte(0x05), server.contents[1][0])
	require.Equal(t, int32(11),
		int32(binary.LittleEndian.Uint32(server.contents[1][1:5])))
	require.Equal(t, int32(22),
		int32(binary.LittleEndian.Uint32(server.contents[1][5:9])))

	// The server login selects the first server of the list.
	require.Equal(t, byte(0x02), server.contents[2][0])
	require.Equal(t, byte(0x01), server.contents[2][9])
}

func TestAuthenticateReportsLoginFail(t *testing.T) {
	server := &fakeLoginServer{
		initContent: buildLoginInit(),
		replies: [][]byte{
			buildLoginFailContent(0x02),
		},
	}
	startFakeLoginServer(t, server)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	_, err = Authenticate(conn, "test1", "wrong")
	require.Error(t, err)
	require.Contains(t, err.Error(), "login failed")
	require.Contains(t, err.Error(), "user or password wrong")

	<-server.done
}

func TestAuthenticateRejectsEmptyServerList(t *testing.T) {
	server := &fakeLoginServer{
		initContent: buildLoginInit(),
		replies: [][]byte{
			buildLoginOkContent(),
			buildServerListContent(0),
		},
	}
	startFakeLoginServer(t, server)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	_, err = Authenticate(conn, "test1", "test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no available game server")

	<-server.done
}

func TestAuthenticateRejectsUnexpectedInit(t *testing.T) {
	server := &fakeLoginServer{
		initContent: []byte{0xFF, 1, 2, 3},
		replies:     nil,
	}
	startFakeLoginServer(t, server)

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)

	_, err = Authenticate(conn, "test1", "test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected packet id")

	<-server.done
}
