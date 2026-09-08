// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strings"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromauthserver "github.com/melg8/swarm/internal/swarm/packets/from_auth_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// loginConn drives one client connection of the emulated login server.
// The emulation is complete: any account/password pair is accepted, the
// server list advertises exactly one game server (the proxy game
// listener on the address family the client used) and the PlayOk
// session keys are generated locally. The client never talks to the
// real Mobius login server.
type loginConn struct {
	server   *Server
	conn     net.Conn
	crypt    *crypt.LoginCrypt
	id       int64
	state    loginState
	account  string
	loginOk1 int32
	loginOk2 int32
	readBuf  []byte
	writeBuf []byte
}

// loginState mirrors the Mobius LoginClientState machine.
type loginState int

const (
	loginStateConnected loginState = iota
	loginStateAuthed
)

// loginInitGG constants of the Mobius Init packet.
const (
	loginProtocolRevision = 0x0000c621
	loginGG1              = 0x29DD954E
	loginGG2              = 0x77C39CFC
	loginGG3              = -1750223328 // 0x97ADB620 as int32
	loginGG4              = 0x07BDE0F7
)

// Login packet opcodes (client to login server).
const (
	loginOpRequestAuthLogin   = 0x00
	loginOpRequestGGAuth      = 0x07
	loginOpRequestServerLogin = 0x02
	loginOpRequestServerList  = 0x05
)

// loginOpGGAuth is the opcode of the GGAuth server answer.
const loginOpGGAuth = 0x0B

// handleLoginConn runs the login server emulation for one client.
func (s *Server) handleLoginConn(conn net.Conn) {
	id := s.nextConnID()
	lc := &loginConn{
		server:   s,
		conn:     conn,
		crypt:    crypt.NewLoginCrypt(crypt.MobiusAuthKey()),
		id:       id,
		state:    loginStateConnected,
		account:  "",
		loginOk1: 0,
		loginOk2: 0,
		readBuf:  nil,
		writeBuf: nil,
	}
	s.logger.Printf("login#%d: client connected from %s", id, conn.RemoteAddr())
	defer func() {
		_ = conn.Close()
		s.logger.Printf("login#%d: connection closed", id)
	}()

	if err := lc.run(); err != nil {
		s.logger.Printf("login#%d: %v", id, err)
	}
}

// run drives the packet exchange until the client disconnects.
func (lc *loginConn) run() error {
	if err := lc.sendInit(); err != nil {
		return fmt.Errorf("failed to send init: %w", err)
	}

	for {
		payload, err := readWirePacket(lc.conn, lc.readBuf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The client closed the connection: the normal end of
				// a login session, not a failure.
				return nil
			}

			return fmt.Errorf("login read failed: %w", err)
		}
		lc.readBuf = payload
		if len(payload) == 0 {
			continue
		}

		content, err := lc.crypt.Open(payload)
		if err != nil {
			return fmt.Errorf("login decrypt failed: %w", err)
		}

		if err := lc.handlePacket(content); err != nil {
			return err
		}
	}
}

// sendInit writes the unencrypted Init packet, mirroring the layout of
// the real login server (session id, protocol revision, scrambled RSA
// modulus and the four GameGuard constants).
func (lc *loginConn) sendInit() error {
	initPacket := &fromauthserver.InitPacket{
		SessionID:       rand.Int32(), //nolint:gosec // a session id, not a secret
		ProtocolVersion: loginProtocolRevision,
		RsaPublicKey:    lc.server.rsaModulusBytes(),
		GameGuard1:      loginGG1,
		GameGuard2:      loginGG2,
		GameGuard3:      loginGG3,
		GameGuard4:      loginGG4,
		BlowfishKey:     nil,
	}
	// The InitPacket serializer starts at the session id: the opcode
	// byte is written by the framing here (see ParseInitPacket, which
	// receives the payload without it).
	writer := packet.NewWriter()
	if err := writer.WriteInt8(0x00); err != nil {
		return fmt.Errorf("failed to write init opcode: %w", err)
	}
	if err := initPacket.ToBytes(writer); err != nil {
		return fmt.Errorf("failed to serialize init: %w", err)
	}
	if err := writeWirePacket(lc.conn, writer.Bytes()); err != nil {
		return fmt.Errorf("failed to write init: %w", err)
	}
	lc.server.logger.Printf("login#%d: sent init (session 0x%08x)",
		lc.id, initPacket.SessionID)

	return nil
}

// handlePacket dispatches one decrypted login packet of the client.
func (lc *loginConn) handlePacket(content []byte) error {
	opcode := content[0]
	switch lc.state {
	case loginStateConnected:
		if opcode == loginOpRequestAuthLogin {
			return lc.handleAuthLogin(content)
		}
	case loginStateAuthed:
		switch opcode {
		case loginOpRequestServerList:
			return lc.handleServerList(content)
		case loginOpRequestServerLogin:
			return lc.handleServerLogin(content)
		case loginOpRequestGGAuth:
			return lc.handleGGAuth(content)
		}
	}

	lc.server.logger.Printf("login#%d: ignored packet 0x%02x in state %d",
		lc.id, opcode, lc.state)

	return nil
}

// handleAuthLogin accepts any credentials, mirrors the Mobius field
// layout ([opcode][account: 14][password: 14]) and answers LoginOk.
func (lc *loginConn) handleAuthLogin(content []byte) error {
	account, password, err := parseRequestAuthLogin(content)
	if err != nil {
		return fmt.Errorf("failed to parse auth login: %w", err)
	}
	lc.account = strings.ToLower(account)
	lc.server.logger.Printf(
		"login#%d: auth login accepted for account %q "+
			"(password %q, any pair is accepted)",
		lc.id, lc.account, password)

	// The login ok session ids only have to be unique within the
	// process: they are not secrets.
	lc.loginOk1 = rand.Int32() //nolint:gosec // see above
	lc.loginOk2 = rand.Int32() //nolint:gosec // see above
	reply := &fromauthserver.LoginOkPacket{
		LoginOkID1: lc.loginOk1,
		LoginOkID2: lc.loginOk2,
	}
	if err := lc.sendPacket(reply); err != nil {
		return fmt.Errorf("failed to send login ok: %w", err)
	}
	lc.state = loginStateAuthed

	return nil
}

// handleServerList answers the one entry server list that points the
// client at the proxy game listener of the address family the client
// used for this login connection.
func (lc *loginConn) handleServerList(_ []byte) error {
	ip := lc.localIP()
	list := &fromauthserver.ServerListPacket{
		Count:      1,
		LastServer: 1,
		Servers: []fromauthserver.ServerListEntry{{
			ServerID:       1,
			IP:             ip,
			Port:           lc.server.gamePort(),
			AgeLimit:       0,
			Pvp:            0,
			CurrentPlayers: int16(lc.server.ClientCount()),
			MaxPlayers:     100,
			Status:         1,
			Bits:           0,
			Brackets:       0,
		}},
	}
	if err := lc.sendPacket(list); err != nil {
		return fmt.Errorf("failed to send server list: %w", err)
	}
	lc.server.logger.Printf(
		"login#%d: sent server list, proxy game at %d.%d.%d.%d:%d",
		lc.id, ip[0], ip[1], ip[2], ip[3], lc.server.gamePort())

	return nil
}

// handleServerLogin answers PlayOk with locally generated session keys
// (the emulated game server accepts any pair).
func (lc *loginConn) handleServerLogin(content []byte) error {
	serverID := byte(0)
	if len(content) >= 10 {
		serverID = content[9]
	}
	lc.server.logger.Printf("login#%d: server login for server id %d",
		lc.id, serverID)

	reply := &fromauthserver.PlayOkPacket{
		PlayOkID1: rand.Int32(), //nolint:gosec // a session key, not a secret
		PlayOkID2: rand.Int32(), //nolint:gosec // a session key, not a secret
	}
	if err := lc.sendPacket(reply); err != nil {
		return fmt.Errorf("failed to send play ok: %w", err)
	}

	return nil
}

// handleGGAuth answers the GGAuth exchange some classic clients run
// after LoginOk: [opcode 0x0B][sessionId: 4][unknown: 4].
func (lc *loginConn) handleGGAuth(content []byte) error {
	sessionID := int32(0)
	if len(content) >= 5 {
		sessionID = int32(uint32(content[1]) | uint32(content[2])<<8 |
			uint32(content[3])<<16 | uint32(content[4])<<24)
	}
	lc.server.logger.Printf("login#%d: gg auth answered (session 0x%08x)",
		lc.id, sessionID)

	writer := packet.NewWriter()
	if err := writer.WriteInt8(loginOpGGAuth); err != nil {
		return err
	}
	if err := writer.WriteInt32(sessionID); err != nil {
		return err
	}
	if err := writer.WriteInt32(0); err != nil {
		return err
	}
	if err := lc.sendSealed(writer.Bytes()); err != nil {
		return fmt.Errorf("failed to send gg auth: %w", err)
	}

	return nil
}

// sendPacket serializes, seals and sends a login server packet.
func (lc *loginConn) sendPacket(p crypt.Serializable) error {
	writer := packet.NewWriter()
	if err := p.ToBytes(writer); err != nil {
		return fmt.Errorf("failed to serialize login packet: %w", err)
	}

	return lc.sendSealed(writer.Bytes())
}

// sendSealed seals raw content bytes and sends them.
func (lc *loginConn) sendSealed(content []byte) error {
	wire, err := lc.crypt.Seal(lc.writeBuf, content)
	if err != nil {
		return fmt.Errorf("failed to seal login packet: %w", err)
	}
	lc.writeBuf = wire
	if _, err := lc.conn.Write(wire); err != nil {
		return fmt.Errorf("failed to write login packet: %w", err)
	}

	return nil
}

// localIP returns the IPv4 address the client used to reach this login
// listener (the server list entry advertises the same family for the
// game connection).
func (lc *loginConn) localIP() [4]byte {
	addr, ok := lc.conn.LocalAddr().(*net.TCPAddr)
	if ok {
		if ip := addr.IP.To4(); ip != nil {
			return [4]byte{ip[0], ip[1], ip[2], ip[3]}
		}
	}

	return [4]byte{127, 0, 0, 1}
}

// parseRequestAuthLogin extracts the credentials of the client auth
// attempt: [opcode 0x00][account: 14 zero padded][password: 14 zero
// padded]. Trailing fillers are ignored exactly like the Mobius parser.
func parseRequestAuthLogin(content []byte) (string, string, error) {
	const fieldSize = 14
	if len(content) < 1+fieldSize*2 {
		return "", "", errors.New("auth login packet is too short")
	}
	account := strings.TrimSpace(string(content[1 : 1+fieldSize]))
	password := strings.TrimSpace(string(content[1+fieldSize : 1+fieldSize*2]))

	return account, password, nil
}
