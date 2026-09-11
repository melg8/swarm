// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromauthserver "github.com/melg8/swarm/internal/swarm/packets/from_auth_server"
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
	"github.com/melg8/swarm/internal/swarm/proxy"
	"github.com/melg8/swarm/internal/swarm/state"
)

// relayTimeout bounds each client step of the relay scenario.
const relayTimeout = 30 * time.Second

// relayDialer is the dialer of the fake client connections.
//
//nolint:exhaustruct_v5 // the zero defaults are intended
var relayDialer = &net.Dialer{Timeout: relayTimeout}

// relayPingBurst is the net ping burst of the fake client: the proxy
// answers them locally, the answers must come back without any
// transit to the bot session.
const relayPingBurst = 5

// relayProxy is the dedicated client proxy of the relay scenario: it
// owns its ephemeral listeners so the main -proxy of the process is
// never touched.
type relayProxy struct {
	server  *proxy.Server
	logFile *os.File
}

// startRelayProxy boots the dedicated proxy of the relay scenario on
// ephemeral ports.
func startRelayProxy() (*relayProxy, relayAddresses, error) {
	addresses := relayAddresses{
		login: "", gameHost: "", gamePort: 0,
	}
	logPath := os.TempDir() + "/swarm_acceptance_proxy.log"
	logFile, err := os.OpenFile(logPath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, addresses, fmt.Errorf("proxy log: %w", err)
	}
	logger := log.New(logFile, "relay ", log.LstdFlags|log.Lmicroseconds)
	server := proxy.NewServer(logger,
		proxy.WithLoginAddresses("127.0.0.1:0"),
		proxy.WithGameAddresses("127.0.0.1:0"))
	if err := server.Listen(); err != nil {
		_ = logFile.Close()

		return nil, addresses, fmt.Errorf("proxy listen: %w", err)
	}
	go func() {
		_ = server.Serve()
	}()
	addresses.login = server.LoginAddr()
	addresses.gameHost, addresses.gamePort = splitHostPort(server.GameAddr())

	return &relayProxy{server: server, logFile: logFile}, addresses, nil
}

// Stop shuts the dedicated proxy down.
func (r *relayProxy) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = r.server.Shutdown(ctx)
	_ = r.logFile.Close()
}

// relayAddresses carries the endpoints of the dedicated proxy.
type relayAddresses struct {
	login    string
	gameHost string
	gamePort int32
}

// splitHostPort splits an address without the net.SplitHostPort
// error dance of the fixed local endpoints.
func splitHostPort(address string) (string, int32) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "127.0.0.1", 0
	}
	var value int32
	for i := range len(port) {
		value = value*10 + int32(port[i]-'0')
	}

	return host, value
}

// runRelaySession plays the relay bot behind the dedicated proxy: the
// session registers with the scenario server only (the main proxy of
// the process keeps its own sessions), the world entry stays manual.
func (m *Manager) runRelaySession(
	ctx context.Context, server *relayProxy, logLine func(string),
) error {
	return m.runSession(ctx, relayAccount, relayPassword, relayAccount,
		false, server.server, logLine)
}

// relayClient is the fake C1 client of the relay scenario: the login
// and game protocol client driving the dedicated proxy exactly like
// the harness of internal/swarm/proxy/e2e_test.go does.
type relayClient struct {
	login   *relayLoginClient
	game    *relayGameClient
	readBuf []byte
}

// relayAddresses exposes the endpoints for the client dialing.
type relayLoginClient struct {
	conn     net.Conn
	crypt    *crypt.LoginCrypt
	writeBuf []byte
	loginOk1 int32
	loginOk2 int32
}

// relayGameClient drives the game side through the proxy.
type relayGameClient struct {
	conn     net.Conn
	crypt    *crypt.GameCrypt
	readBuf  []byte
	writeBuf []byte
}

// driveRelayClient walks the full client path of the relay scenario
// and marks the checks along the way: the emulated login, the one
// character list, the world entry replay, the live movement echo and
// the locally answered net pings.
func driveRelayClient(
	_ context.Context, addresses relayAddresses, tracker *state.Bot,
	test *Test,
) (*relayClient, error) {
	client := &relayClient{login: nil, game: nil, readBuf: nil}
	login, err := dialRelayLogin(addresses.login)
	if err != nil {
		return nil, err
	}
	client.login = login

	// The emulated login: any credentials pass, the server list offers
	// the one game server, the play ok claims its slot.
	if _, err := login.readInit(); err != nil {
		client.close()

		return nil, fmt.Errorf("read init: %w", err)
	}
	if _, err := login.accept("whatever", "credentials"); err != nil {
		client.close()

		return nil, fmt.Errorf("login: %w", err)
	}
	list, err := login.serverList()
	if err != nil {
		client.close()

		return nil, fmt.Errorf("server list: %w", err)
	}
	if len(list.Servers) != 1 {
		client.close()

		return nil, fmt.Errorf("the server list offers %d servers",
			len(list.Servers))
	}
	if _, err := login.play(list.Servers[0].ServerID); err != nil {
		client.close()

		return nil, fmt.Errorf("play ok: %w", err)
	}
	test.updateCheck("login", true,
		"arbitrary credentials accepted, one game server")

	if err := driveRelayWorld(client, addresses, tracker, test); err != nil {
		client.close()

		return nil, err
	}

	return client, nil
}

// driveRelayReplay walks the entering part of the game side: the char
// list of the bot and the world entry through the recorded replay.
func driveRelayReplay(game *relayGameClient, test *Test) error {
	charListPayload, err := game.readRelayPacket()
	if err != nil {
		return fmt.Errorf("read char list: %w", err)
	}
	if charListPayload[0] != 0x1F {
		return fmt.Errorf("char list opcode 0x%02X", charListPayload[0])
	}
	charList := fromgameserver.NewCharSelectInfoPacket()
	if err := fromgameserver.ParseCharSelectInfoPacket(
		charList, charListPayload); err != nil {
		return fmt.Errorf("parse char list: %w", err)
	}
	if len(charList.Characters) != 1 ||
		charList.Characters[0].Name != relayAccount {
		return fmt.Errorf("the char list is not the one %s character",
			relayAccount)
	}

	// The world entry through the recorded replay: the char selected
	// answer of the bot, then the enter world burst with the UserInfo.
	if err := game.sendRaw([]byte{0x0D, 0x00, 0x00, 0x00, 0x00}); err != nil {
		return fmt.Errorf("send char select: %w", err)
	}
	selectedPayload, err := game.readRelayPacket()
	if err != nil {
		return fmt.Errorf("read char selected: %w", err)
	}
	if selectedPayload[0] != 0x21 {
		return fmt.Errorf("char selected opcode 0x%02X",
			selectedPayload[0])
	}
	if err := game.sendRaw([]byte{0x03}); err != nil {
		return fmt.Errorf("send enter world: %w", err)
	}
	if _, err := game.readUntil(func(payload []byte) bool {
		return payload[0] == 0x04
	}, "the replayed user info"); err != nil {
		return err
	}
	test.updateCheck("replay", true, "the world entry replayed")

	return nil
}

// driveRelayTransit walks the live part of the game side: the
// movement echo through the bot session and the locally answered net
// pings of the keepalive.
func driveRelayTransit(
	game *relayGameClient, tracker *state.Bot, test *Test,
) error {
	// The live relay: a MoveToLocation transits to the real server
	// and the movement echo comes back through the bot session.
	snapshot := tracker.Snapshot()
	move := []byte{0x01}
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.X+100))
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.Y+100))
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.Z))
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.X))
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.Y))
	move = binary.LittleEndian.AppendUint32(
		move, uint32(snapshot.Character.Z))
	// The movement mode is a full int (1 = mouse click, see
	// MoveToLocation.readImpl).
	move = binary.LittleEndian.AppendUint32(move, 1)
	if err := game.sendRaw(move); err != nil {
		return fmt.Errorf("send move: %w", err)
	}
	selfID := snapshot.Character.ObjectID
	if _, err := game.readUntil(func(payload []byte) bool {
		return payload[0] == 0x01 && len(payload) >= 5 &&
			int32(binary.LittleEndian.Uint32(payload[1:5])) == selfID
	}, "the movement echo through the live relay"); err != nil {
		return err
	}
	test.updateCheck("relay", true, "the walk echo came back")

	// The keepalive: the proxy answers the net pings locally.
	for range relayPingBurst {
		if err := game.sendRaw([]byte{0xA8}); err != nil {
			return fmt.Errorf("send net ping: %w", err)
		}
	}
	for range relayPingBurst {
		reply, err := game.readUntil(func(payload []byte) bool {
			return payload[0] == 0xEC
		}, "the locally answered net ping")
		if err != nil {
			return err
		}
		if len(reply) != 5 {
			return fmt.Errorf("net ping answer of %d bytes", len(reply))
		}
	}
	test.updateCheck("ping", true, "the pings never left the proxy")

	return nil
}

// driveRelayWorld walks the game side of the client path: the char
// list, the world entry replay, the live movement echo and the net
// pings of the keepalive.
func driveRelayWorld(
	client *relayClient, addresses relayAddresses, tracker *state.Bot,
	test *Test,
) error {
	game, err := dialRelayGame(addresses)
	if err != nil {
		return err
	}
	client.game = game
	if err := game.handshake(); err != nil {
		return fmt.Errorf("game handshake: %w", err)
	}
	if err := game.authLogin(client.login); err != nil {
		return fmt.Errorf("client auth login: %w", err)
	}

	if err := driveRelayReplay(game, test); err != nil {
		return err
	}

	return driveRelayTransit(game, tracker, test)
}

// close drops the client connections.
func (c *relayClient) close() {
	if c.login != nil {
		_ = c.login.conn.Close()
	}
	if c.game != nil {
		_ = c.game.conn.Close()
	}
}

// dialRelayLogin connects to the emulated login server.
func dialRelayLogin(address string) (*relayLoginClient, error) {
	conn, err := relayDialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("login dial: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(relayTimeout)); err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("login deadline: %w", err)
	}

	return &relayLoginClient{
		conn:     conn,
		crypt:    crypt.NewLoginCrypt(crypt.MobiusAuthKey()),
		writeBuf: nil,
		loginOk1: 0,
		loginOk2: 0,
	}, nil
}

// readInit reads the unencrypted init packet of the emulated server.
func (c *relayLoginClient) readInit() (*fromauthserver.InitPacket, error) {
	payload, err := readWirePacket(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(payload) < 1+4+4+128 || payload[0] != 0x00 {
		return nil, fmt.Errorf("malformed init packet of %d bytes",
			len(payload))
	}
	initPacket := fromauthserver.NewInitPacket()
	if err := fromauthserver.ParseInitPacket(initPacket, payload[1:]); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(initPacket.RsaPublicKey) != 128 {
		return nil, errors.New("the init packet carries no rsa modulus")
	}

	return initPacket, nil
}

// accept seals and sends the login request and reads the LoginOk.
func (c *relayLoginClient) accept(
	account string, password string,
) (*fromauthserver.LoginOkPacket, error) {
	if err := c.send(&toauthserver.RequestAuthLogin{
		Account: account, Password: password,
	}); err != nil {
		return nil, err
	}
	content, err := c.readSealed()
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || content[0] != 0x03 {
		return nil, fmt.Errorf("login ok opcode 0x%02X", opcodeOf(content))
	}
	loginOk := fromauthserver.NewLoginOkPacket()
	if err := fromauthserver.ParseLoginOkPacket(loginOk, content); err != nil {
		return nil, fmt.Errorf("parse login ok: %w", err)
	}
	c.loginOk1 = loginOk.LoginOkID1
	c.loginOk2 = loginOk.LoginOkID2

	return loginOk, nil
}

// serverList reads the server list of the emulated server.
func (c *relayLoginClient) serverList() (
	*fromauthserver.ServerListPacket, error,
) {
	if err := c.send(toauthserver.NewRequestServerList(
		c.loginOk1, c.loginOk2)); err != nil {
		return nil, err
	}
	content, err := c.readSealed()
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || content[0] != 0x04 {
		return nil, fmt.Errorf("server list opcode 0x%02X", opcodeOf(content))
	}
	list := fromauthserver.NewServerListPacket()
	if err := fromauthserver.ParseServerListPacket(list, content); err != nil {
		return nil, fmt.Errorf("parse server list: %w", err)
	}

	return list, nil
}

// play claims the game server slot and reads the PlayOk.
func (c *relayLoginClient) play(
	serverID int8,
) (*fromauthserver.PlayOkPacket, error) {
	if err := c.send(&toauthserver.RequestServerLogin{
		LoginOkID1: c.loginOk1,
		LoginOkID2: c.loginOk2,
		ServerID:   serverID,
	}); err != nil {
		return nil, err
	}
	content, err := c.readSealed()
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || content[0] != 0x07 {
		return nil, fmt.Errorf("play ok opcode 0x%02X", opcodeOf(content))
	}
	playOk := fromauthserver.NewPlayOkPacket()
	if err := fromauthserver.ParsePlayOkPacket(playOk, content); err != nil {
		return nil, fmt.Errorf("parse play ok: %w", err)
	}

	return playOk, nil
}

// send seals and sends one login packet.
func (c *relayLoginClient) send(p crypt.Serializable) error {
	writer := packet.NewWriter()
	if err := p.ToBytes(writer); err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	wire, err := c.crypt.Seal(c.writeBuf, writer.Bytes())
	if err != nil {
		return fmt.Errorf("seal: %w", err)
	}
	c.writeBuf = wire
	if _, err := c.conn.Write(wire); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// readSealed reads and opens one sealed login packet.
func (c *relayLoginClient) readSealed() ([]byte, error) {
	payload, err := readWirePacket(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	content, err := c.crypt.Open(payload)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	return content, nil
}

// opcodeOf renders the opcode byte of a possibly empty payload.
func opcodeOf(content []byte) byte {
	if len(content) == 0 {
		return 0
	}

	return content[0]
}

// dialRelayGame connects to the emulated game server of the proxy.
func dialRelayGame(addresses relayAddresses) (*relayGameClient, error) {
	address := net.JoinHostPort(addresses.gameHost,
		strconv.FormatInt(int64(addresses.gamePort), 10))
	conn, err := relayDialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("game dial: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(relayTimeout)); err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("game deadline: %w", err)
	}

	return &relayGameClient{
		conn:     conn,
		crypt:    nil,
		readBuf:  nil,
		writeBuf: nil,
	}, nil
}

// handshake sends the protocol version and enables the cipher with
// the key of the answered KeyPacket.
func (c *relayGameClient) handshake() error {
	request := []byte{0x00}
	request = binary.LittleEndian.AppendUint32(request, 419)
	if err := writeWirePacket(c.conn, request); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	keyPacket, err := readWirePacket(c.conn)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if len(keyPacket) < 10 || keyPacket[0] != 0x00 || keyPacket[1] != 0x01 {
		return fmt.Errorf("malformed key packet of %d bytes",
			len(keyPacket))
	}
	var key [crypt.GameCryptKeySize]byte
	copy(key[:], keyPacket[2:10])
	c.crypt = crypt.NewGameCrypt(key)
	c.crypt.Enable()

	return nil
}

// authLogin sends the game server authentication of the fake client:
// the keys of the emulated login flow (any pair works).
func (c *relayGameClient) authLogin(login *relayLoginClient) error {
	payload := []byte{0x08}
	payload = appendUTF16LE(payload, "whatever")
	payload = binary.LittleEndian.AppendUint32(payload, 0)
	payload = binary.LittleEndian.AppendUint32(payload, 0)
	payload = binary.LittleEndian.AppendUint32(
		payload, uint32(login.loginOk1))
	payload = binary.LittleEndian.AppendUint32(
		payload, uint32(login.loginOk2))

	return c.sendRaw(payload)
}

// sendRaw encrypts and sends one client packet payload.
func (c *relayGameClient) sendRaw(payload []byte) error {
	owned := make([]byte, len(payload))
	copy(owned, payload)
	c.crypt.Encrypt(owned)
	if err := writeWirePacket(c.conn, owned); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// readRelayPacket reads and decrypts one server packet payload.
func (c *relayGameClient) readRelayPacket() ([]byte, error) {
	payload, err := readWirePacket(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	c.crypt.Decrypt(payload)

	return payload, nil
}

// readUntil reads packets until the predicate matches or the deadline
// of the connection expires; the failure names what was awaited.
func (c *relayGameClient) readUntil(
	match func(payload []byte) bool, what string,
) ([]byte, error) {
	for {
		payload, err := c.readRelayPacket()
		if err != nil {
			return nil, fmt.Errorf("await %s: %w", what, err)
		}
		if match(payload) {
			return payload, nil
		}
	}
}

// appendUTF16LE appends a null terminated little endian UTF-16
// string (the ascii subset the protocol strings need).
func appendUTF16LE(dst []byte, value string) []byte {
	for i := range len(value) {
		dst = append(dst, value[i], 0)
	}

	return append(dst, 0, 0)
}

// readWirePacket reads one 2 byte framed packet payload.
func readWirePacket(conn net.Conn) ([]byte, error) {
	header := make([]byte, 2)
	if err := readFull(conn, header); err != nil {
		return nil, err
	}
	length := int(header[0]) | int(header[1])<<8
	payload := make([]byte, length)
	if err := readFull(conn, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// writeWirePacket frames and writes one packet payload.
func writeWirePacket(conn net.Conn, payload []byte) error {
	header := []byte{byte(len(payload)), byte(len(payload) >> 8)}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)

	return err
}

// readFull fills the buffer completely.
func readFull(conn net.Conn, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}

	return nil
}
