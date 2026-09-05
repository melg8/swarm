// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// Protocol constants.
const (
	gamePingPeriod    = 25 * time.Second
	gameWriteTimeout  = 10 * time.Second
	gameHandshakeWait = 30 * time.Second
	packetChanSize    = 32
	bufferInitialSize = 4096
)

// Packet ids used by the game flow state machine.
const (
	charSelectInfoID  = 0x1F
	charCreateOkID    = 0x25
	charCreateFailID  = 0x26
	charSelectedID    = 0x21
	userInfoID        = 0x04
	leaveWorldID      = 0x96
	serverCloseID     = 0x36
	netPingResponseID = 0xEC
)

// GameSessionParams carries the login session keys for the game server.
type GameSessionParams struct {
	Account    string
	LoginOkID1 int32
	LoginOkID2 int32
	PlayOkID1  int32
	PlayOkID2  int32
}

// CharacterParams describes the character the bot wants to play.
type CharacterParams struct {
	Name      string
	Race      int32
	Female    int32
	ClassID   int32
	HairStyle int32
	HairColor int32
	Face      int32
}

// GameClient drives a game server session of the Mobius C1 protocol.
type GameClient struct {
	conn        net.Conn
	crypt       *crypt.GameCrypt
	writeMu     sync.Mutex
	logger      *log.Logger
	packetCount atomic.Int64
	readBuf     []byte
}

// gameBuffer is the pooled receive buffer of the read loop. A pointer type
// keeps sync.Pool arguments pointer-like and allocation free.
type gameBuffer struct {
	data []byte
}

// bufferPool recycles receive buffers of the read loop.
var bufferPool = sync.Pool{
	New: func() any {
		return &gameBuffer{data: make([]byte, 0, bufferInitialSize)}
	},
}

// NewGameClient wraps a game server connection and performs the protocol
// handshake: sends the unencrypted ProtocolVersion and reads the
// unencrypted KeyPacket that enables the XOR cipher.
func NewGameClient(conn net.Conn) (*GameClient, error) {
	client := &GameClient{
		conn:        conn,
		crypt:       nil,
		writeMu:     sync.Mutex{},
		logger:      log.Default(),
		packetCount: atomic.Int64{},
		readBuf:     nil,
	}

	writer := packet.NewWriter()
	if err := togameserver.NewProtocolVersion().ToBytes(writer); err != nil {
		return nil, fmt.Errorf("failed to serialize protocol version: %w", err)
	}
	if err := writeWirePacket(conn, writer.Bytes()); err != nil {
		return nil, fmt.Errorf("failed to send protocol version: %w", err)
	}
	client.logger.Printf("Sent protocol version %d",
		togameserver.C1ProtocolVersion)

	if err := conn.SetReadDeadline(time.Now().Add(gameHandshakeWait)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}
	payload, err := readWirePacket(conn, client.readBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read key packet: %w", err)
	}
	client.readBuf = payload

	keyPacket := fromgameserver.NewKeyPacket()
	if err := fromgameserver.ParseKeyPacket(keyPacket, payload); err != nil {
		return nil, fmt.Errorf("failed to parse key packet: %w", err)
	}
	if !keyPacket.Ok() {
		return nil, errors.New("game server rejected the protocol version")
	}

	client.crypt = crypt.NewGameCrypt(keyPacket.Key)
	client.crypt.Enable()
	client.logger.Printf("Game server %d accepted the protocol",
		keyPacket.ServerID)

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("failed to reset read deadline: %w", err)
	}

	return client, nil
}

// SetLogger overrides the default logger of the client.
func (gc *GameClient) SetLogger(logger *log.Logger) {
	gc.logger = logger
}

// PacketCount returns the number of packets received so far.
func (gc *GameClient) PacketCount() int {
	return int(gc.packetCount.Load())
}

// sendPacket serializes, encrypts and sends a game server packet.
func (gc *GameClient) sendPacket(data crypt.Serializable) error {
	writer := packet.NewWriter()
	if err := data.ToBytes(writer); err != nil {
		return fmt.Errorf("failed to serialize game packet: %w", err)
	}

	gc.crypt.Encrypt(writer.Bytes())

	gc.writeMu.Lock()
	defer gc.writeMu.Unlock()
	if err := gc.conn.SetWriteDeadline(
		time.Now().Add(gameWriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	if err := writeWirePacket(gc.conn, writer.Bytes()); err != nil {
		return fmt.Errorf("failed to send game packet: %w", err)
	}

	return nil
}

// readPacket reads and decrypts the next game server packet payload into
// the client buffer.
func (gc *GameClient) readPacket(buf []byte) ([]byte, error) {
	payload, err := readWirePacket(gc.conn, buf)
	if err != nil {
		return nil, err
	}

	gc.crypt.Decrypt(payload)
	if len(payload) == 0 {
		return nil, nil
	}
	gc.packetCount.Add(1)

	return payload, nil
}

// Authenticate sends the game AuthLogin packet and waits for the character
// list of the account.
func (gc *GameClient) Authenticate(
	params GameSessionParams,
) (*fromgameserver.CharSelectInfoPacket, error) {
	if err := gc.sendPacket(&togameserver.AuthLogin{
		Login:      params.Account,
		PlayOkID1:  params.PlayOkID1,
		PlayOkID2:  params.PlayOkID2,
		LoginOkID1: params.LoginOkID1,
		LoginOkID2: params.LoginOkID2,
	}); err != nil {
		return nil, fmt.Errorf("failed to send auth login: %w", err)
	}
	gc.logger.Println("Sent game auth login for account " + params.Account)

	payload, err := gc.readPacket(gc.readBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read character list: %w", err)
	}
	gc.readBuf = payload
	if len(payload) == 0 {
		return nil, errors.New("empty character list packet")
	}
	if payload[0] != charSelectInfoID {
		return nil, fmt.Errorf(
			"unexpected packet id 0x%02x while waiting for characters",
			payload[0])
	}

	charList := fromgameserver.NewCharSelectInfoPacket()
	if err := fromgameserver.ParseCharSelectInfoPacket(
		charList, payload); err != nil {
		return nil, fmt.Errorf("failed to parse character list: %w", err)
	}
	gc.logger.Printf("Received character list with %d characters",
		len(charList.Characters))

	return charList, nil
}

// EnsureCharacter returns the character list that contains the character
// with the requested name, creating the character first when needed.
func (gc *GameClient) EnsureCharacter(
	params CharacterParams,
	charList *fromgameserver.CharSelectInfoPacket,
) (*fromgameserver.CharSelectInfoPacket, error) {
	if _, _, found := charList.FindCharacterByName(params.Name); found {
		return charList, nil
	}

	if err := gc.sendPacket(&togameserver.CharacterCreate{
		Name:      params.Name,
		Race:      params.Race,
		Female:    params.Female,
		ClassID:   params.ClassID,
		INT:       0,
		STR:       0,
		CON:       0,
		MEN:       0,
		DEX:       0,
		WIT:       0,
		HairStyle: params.HairStyle,
		HairColor: params.HairColor,
		Face:      params.Face,
	}); err != nil {
		return nil, fmt.Errorf("failed to send character create: %w", err)
	}
	gc.logger.Println("Sent character create for " + params.Name)

	// The server answers with the creation result and the updated list.
	for {
		done, updated, err := gc.awaitCharacterCreation(params.Name)
		if err != nil {
			return nil, err
		}
		if done {
			return updated, nil
		}
	}
}

// awaitCharacterCreation reads packets until the creation result resolves.
// The first return value reports completion, the second carries the updated
// character list when the requested character appeared in it.
func (gc *GameClient) awaitCharacterCreation(
	name string,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
	payload, err := gc.readPacket(gc.readBuf)
	if err != nil {
		return false, nil, fmt.Errorf("failed to read creation result: %w", err)
	}
	gc.readBuf = payload
	if len(payload) == 0 {
		return false, nil, nil
	}

	return gc.handleCharCreatePacket(name, payload)
}

// handleCharCreatePacket dispatches a single packet of the creation flow.
func (gc *GameClient) handleCharCreatePacket(
	name string, payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
	switch payload[0] {
	case charCreateOkID:
		if err := fromgameserver.ParseCharCreateOkPacket(payload); err != nil {
			return false, nil, err
		}
		gc.logger.Println("Character " + name + " created")
	case charCreateFailID:
		return gc.handleCharCreateFail(payload)
	case charSelectInfoID:
		return gc.handleUpdatedCharList(name, payload)
	default:
		gc.logger.Printf("Ignoring packet id 0x%02x while creating character",
			payload[0])
	}

	return false, nil, nil
}

// handleCharCreateFail converts a creation failure packet into an error.
func (gc *GameClient) handleCharCreateFail(
	payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
	fail := fromgameserver.NewCharCreateFailPacket()
	if err := fromgameserver.ParseCharCreateFailPacket(fail, payload); err != nil {
		return false, nil, err
	}

	return false, nil, fmt.Errorf(
		"character creation failed: %s", fail.ReasonText())
}

// handleUpdatedCharList checks the updated character list for the name.
func (gc *GameClient) handleUpdatedCharList(
	name string, payload []byte,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
	updated := fromgameserver.NewCharSelectInfoPacket()
	if err := fromgameserver.ParseCharSelectInfoPacket(
		updated, payload); err != nil {
		return false, nil, fmt.Errorf(
			"failed to parse updated character list: %w", err)
	}
	if _, _, found := updated.FindCharacterByName(name); found {
		return true, updated, nil
	}

	return false, nil, errors.New("created character missing in the updated list")
}

// EnterWorld selects the character slot and requests world entry.
func (gc *GameClient) EnterWorld(slot int32) error {
	if err := gc.sendPacket(
		&togameserver.CharacterSelect{CharSlot: slot}); err != nil {
		return fmt.Errorf("failed to send character select: %w", err)
	}

	// Wait for the char selected packet that allows entering the world.
	var selected fromgameserver.CharSelectedPacket
	for {
		payload, err := gc.readPacket(gc.readBuf)
		if err != nil {
			return fmt.Errorf("failed to read character selected: %w", err)
		}
		gc.readBuf = payload
		if len(payload) == 0 {
			continue
		}
		if payload[0] == charSelectedID {
			if err := fromgameserver.ParseCharSelectedPacket(
				&selected, payload); err != nil {
				return fmt.Errorf("failed to parse char selected: %w", err)
			}

			break
		}
		gc.logger.Printf("Ignoring packet id 0x%02x while entering world", payload[0])
	}
	gc.logger.Println("Selected character " + selected.Name)

	if err := gc.sendPacket(&togameserver.EnterWorld{}); err != nil {
		return fmt.Errorf("failed to send enter world: %w", err)
	}
	gc.logger.Println("Sent enter world request")

	return nil
}

// gamePacket couples a received payload with its pooled buffer.
type gamePacket struct {
	buf     *gameBuffer
	payload []byte
	err     error
}

// Run blocks while the character stays in the world. It reads server
// packets in a dedicated goroutine and sends periodic net ping requests.
// On context cancellation it sends the logout packet and closes.
func (gc *GameClient) Run(ctx context.Context, characterName string) error {
	packets := make(chan gamePacket, packetChanSize)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			buf := bufferPool.Get().(*gameBuffer)
			payload, err := gc.readPacket(buf.data)
			if err != nil {
				bufferPool.Put(buf)
				packets <- gamePacket{buf: nil, payload: nil, err: err}

				return
			}
			buf.data = payload
			packets <- gamePacket{buf: buf, payload: payload, err: nil}
		}
	}()

	pingTicker := time.NewTicker(gamePingPeriod)
	defer pingTicker.Stop()

	err := gc.runLoop(ctx, packets, pingTicker, characterName)
	gc.disconnect()

	// Unblock the reader goroutine and drain pending packets.
	drainPackets(packets, readerDone)

	return err
}

// runLoop is the main receive loop of the in game session.
func (gc *GameClient) runLoop(
	ctx context.Context,
	packets <-chan gamePacket,
	pingTicker *time.Ticker,
	characterName string,
) error {
	for {
		select {
		case <-ctx.Done():
			gc.logger.Println("Leaving the game world with " + characterName)

			return nil
		case <-pingTicker.C:
			if err := gc.sendPacket(&togameserver.RequestNetPing{}); err != nil {
				return fmt.Errorf("failed to send net ping: %w", err)
			}
		case msg, ok := <-packets:
			if !ok {
				return nil
			}
			if msg.err != nil {
				return fmt.Errorf("game connection lost: %w", msg.err)
			}
			gc.handleServerPacket(msg.payload)
			bufferPool.Put(msg.buf)
		}
	}
}

// disconnect sends the logout packet and closes the connection.
func (gc *GameClient) disconnect() {
	err := gc.sendPacket(&togameserver.Logout{})
	if err != nil {
		gc.logger.Printf("Failed to send logout: %v", err)
	}
	_ = gc.conn.Close()
}

// drainPackets unblocks the reader goroutine and returns buffers to the pool.
func drainPackets(packets <-chan gamePacket, readerDone <-chan struct{}) {
	for {
		select {
		case <-readerDone:
			return
		case msg, ok := <-packets:
			if !ok {
				return
			}
			if msg.buf != nil {
				bufferPool.Put(msg.buf)
			}
		}
	}
}

// handleServerPacket logs and dispatches known in game packets.
func (gc *GameClient) handleServerPacket(payload []byte) {
	switch payload[0] {
	case netPingResponseID:
		ping := fromgameserver.NewNetPingPacket()
		if err := fromgameserver.ParseNetPingPacket(ping, payload); err != nil {
			gc.logger.Printf("Failed to parse net ping: %v", err)

			return
		}
		gc.logger.Printf("Net ping with game time %d", ping.GameTime)
	case userInfoID:
		gc.logger.Println("Received user info update")
	case leaveWorldID:
		gc.logger.Println("Server confirmed leave world")
	case serverCloseID:
		gc.logger.Println("Server is closing the connection")
	default:
		gc.logger.Printf("Received packet id 0x%02x with %d bytes",
			payload[0], len(payload))
	}
}
