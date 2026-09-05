// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package connection implements client flows for the Mobius C1 protocol:
// authentication on the login server and the game server session.
package connection

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/melg8/swarm/internal/swarm/helpers"
	fromauthserver "github.com/melg8/swarm/internal/swarm/packets/from_auth_server"
	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
)

const (
	initPacketID = 0x00
	loginFailID  = 0x01
)

// AuthResult contains everything needed to connect to the game server.
type AuthResult struct {
	Account    string
	LoginOkID1 int32
	LoginOkID2 int32
	PlayOkID1  int32
	PlayOkID2  int32
	ServerID   int
	ServerIP   [4]byte
	ServerPort int32
}

// LoginClient drives the login server flow of the Mobius protocol.
type LoginClient struct {
	conn      net.Conn
	crypt     *crypt.LoginCrypt
	readBuf   []byte
	writeBuf  []byte
	loginOK   fromauthserver.LoginOkPacket
	serverIDs fromauthserver.ServerListPacket
	playOK    fromauthserver.PlayOkPacket
}

// NewLoginClient wraps an established login server connection.
func NewLoginClient(conn net.Conn) *LoginClient {
	return &LoginClient{
		conn:      conn,
		crypt:     crypt.NewLoginCrypt(crypt.MobiusAuthKey()),
		readBuf:   nil,
		writeBuf:  nil,
		loginOK:   fromauthserver.LoginOkPacket{LoginOkID1: 0, LoginOkID2: 0},
		serverIDs: *fromauthserver.NewServerListPacket(),
		playOK:    fromauthserver.PlayOkPacket{PlayOkID1: 0, PlayOkID2: 0},
	}
}

// readPacket reads and decrypts the next login server packet.
func (lc *LoginClient) readPacket() ([]byte, error) {
	payload, err := readWirePacket(lc.conn, lc.readBuf)
	if err != nil {
		return nil, err
	}
	lc.readBuf = payload

	content, err := lc.crypt.Open(payload)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// readInitPacket reads the unencrypted Init packet of the session.
func (lc *LoginClient) readInitPacket() (*fromauthserver.InitPacket, error) {
	payload, err := readWirePacket(lc.conn, lc.readBuf)
	if err != nil {
		return nil, err
	}
	lc.readBuf = payload

	if len(payload) < 1 {
		return nil, errors.New("empty init packet")
	}
	if payload[0] != initPacketID {
		return nil, fmt.Errorf("unexpected packet id 0x%02x while waiting for init",
			payload[0])
	}

	initPacket := fromauthserver.NewInitPacket()
	if err := fromauthserver.ParseInitPacket(initPacket, payload[1:]); err != nil {
		return nil, fmt.Errorf("failed to parse init packet: %w", err)
	}

	return initPacket, nil
}

// sendPacket serializes, seals and sends a login server packet.
func (lc *LoginClient) sendPacket(data crypt.Serializable) error {
	wire, err := lc.crypt.SealPacket(lc.writeBuf, data)
	if err != nil {
		return err
	}
	lc.writeBuf = wire

	if _, err := lc.conn.Write(wire); err != nil {
		return fmt.Errorf("failed to write login packet: %w", err)
	}

	return nil
}

// readAuthedPacket reads a packet and treats login fail as an error.
func (lc *LoginClient) readAuthedPacket() ([]byte, error) {
	content, err := lc.readPacket()
	if err != nil {
		return nil, err
	}
	if len(content) < 1 {
		return nil, errors.New("empty packet from login server")
	}
	if content[0] == loginFailID {
		var failPacket fromauthserver.LoginFailPacket
		if err := fromauthserver.ParseLoginFailPacket(
			&failPacket, content); err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("login failed: %s", failPacket.ReasonText())
	}

	return content, nil
}

// authAccount exchanges the account credentials for a LoginOk session key.
func (lc *LoginClient) authAccount(account, password string) error {
	initPacket, err := lc.readInitPacket()
	if err != nil {
		return fmt.Errorf("failed to receive init: %w", err)
	}
	log.Println("Received init packet with session id " +
		helpers.HexStringFromInt32(initPacket.SessionID))

	if err := lc.sendPacket(&toauthserver.RequestAuthLogin{
		Account:  account,
		Password: password,
	}); err != nil {
		return fmt.Errorf("failed to send auth login: %w", err)
	}
	log.Println("Sent auth login for account " + account)

	content, err := lc.readAuthedPacket()
	if err != nil {
		return fmt.Errorf("failed to auth: %w", err)
	}
	if err := fromauthserver.ParseLoginOkPacket(&lc.loginOK, content); err != nil {
		return fmt.Errorf("failed to parse login ok: %w", err)
	}
	log.Println("Authenticated on login server")

	return nil
}

// requestServerList fetches the game server list and picks the first one.
func (lc *LoginClient) requestServerList() (
	*fromauthserver.ServerListEntry, error,
) {
	err := lc.sendPacket(
		toauthserver.NewRequestServerList(
			lc.loginOK.LoginOkID1,
			lc.loginOK.LoginOkID2))
	if err != nil {
		return nil, fmt.Errorf("failed to send server list request: %w", err)
	}

	content, err := lc.readAuthedPacket()
	if err != nil {
		return nil, fmt.Errorf("failed to request server list: %w", err)
	}
	if err := fromauthserver.ParseServerListPacket(
		&lc.serverIDs, content); err != nil {
		return nil, fmt.Errorf("failed to parse server list: %w", err)
	}
	log.Printf("Received server list with %d servers", len(lc.serverIDs.Servers))

	server := lc.serverIDs.FirstAvailableServer()
	if server == nil {
		return nil, errors.New("no available game server in the server list")
	}

	return server, nil
}

// requestServerLogin claims a slot on the given game server and stores the
// play session key of the response.
func (lc *LoginClient) requestServerLogin(
	server *fromauthserver.ServerListEntry,
) error {
	err := lc.sendPacket(&toauthserver.RequestServerLogin{
		LoginOkID1: lc.loginOK.LoginOkID1,
		LoginOkID2: lc.loginOK.LoginOkID2,
		ServerID:   server.ServerID,
	})
	if err != nil {
		return fmt.Errorf("failed to send server login request: %w", err)
	}

	content, err := lc.readAuthedPacket()
	if err != nil {
		return fmt.Errorf("failed to login on game server: %w", err)
	}
	if err := fromauthserver.ParsePlayOkPacket(&lc.playOK, content); err != nil {
		return fmt.Errorf("failed to parse play ok: %w", err)
	}

	return nil
}

// Authenticate performs the full login flow: Init, RequestAuthLogin,
// LoginOk, RequestServerList, ServerList, RequestServerLogin and PlayOk.
// The login connection is closed before returning.
func Authenticate(
	conn net.Conn, account, password string,
) (*AuthResult, error) {
	client := NewLoginClient(conn)
	defer client.conn.Close()

	if err := client.authAccount(account, password); err != nil {
		return nil, err
	}

	server, err := client.requestServerList()
	if err != nil {
		return nil, err
	}

	if err := client.requestServerLogin(server); err != nil {
		return nil, err
	}

	log.Printf("Selected game server %d at %d.%d.%d.%d:%d",
		server.ServerID, server.IP[0], server.IP[1], server.IP[2], server.IP[3],
		server.Port)

	return &AuthResult{
		Account:    account,
		LoginOkID1: client.loginOK.LoginOkID1,
		LoginOkID2: client.loginOK.LoginOkID2,
		PlayOkID1:  client.playOK.PlayOkID1,
		PlayOkID2:  client.playOK.PlayOkID2,
		ServerID:   int(server.ServerID),
		ServerIP:   server.IP,
		ServerPort: server.Port,
	}, nil
}
