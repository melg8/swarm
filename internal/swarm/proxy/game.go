// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
)

// gameConn drives one client connection of the emulated game server.
//
// The handshake mirrors the real server (ProtocolVersion, KeyPacket
// with the static Mobius session key, then the stateful XOR cipher). The pre
// world packets
// are fully emulated: any AuthLogin session keys are accepted, the char
// list shows exactly the played character of the selected bot and the
// CharSelected answer is the recorded packet of the bot session. After
// EnterWorld the client receives the recorded server packet stream of
// the bot (the replay) followed by the live feed, and every client
// packet transits to the real game server through the bot connection.
type gameConn struct {
	server  *Server
	conn    net.Conn
	crypt   *crypt.GameCrypt
	id      int64
	state   gameState
	readBuf []byte

	session         *botSession
	charSelectedSeq int64
	queue           [][]byte

	outCh     chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

// gameState mirrors the Mobius ConnectionState machine of the client.
type gameState int

const (
	gameStateHandshake gameState = iota
	gameStateAuthed
	gameStateChars
	gameStateSelected
	gameStateWorld
)

// Client game packet opcodes handled by the emulated server.
const (
	gameOpEnterWorld      = 0x03
	gameOpAuthLogin       = 0x08
	gameOpCharacterCreate = 0x0B
	gameOpCharacterDelete = 0x0C
	gameOpCharacterSelect = 0x0D
	gameOpNewCharacter    = 0x0E
)

// Server packet opcodes sent by the emulated server.
const (
	gameOpKeyPacket      = 0x00
	gameOpCharCreateFail = 0x26
)

// charSelectedOpcode marks the recorded packet replayed on char select.
const charSelectedOpcode = 0x21

// charCreateFailTooMany is the creation failure reason of the proxy
// ("too many characters"): the emulated account has its single slot.
const charCreateFailTooMany = 0x01

// gameWriteTimeout bounds one client bound write of the sender.
const gameWriteTimeout = 10 * time.Second

// outboundQueueSize bounds the client bound packet queue of one client.
const outboundQueueSize = 512

// handleGameConn runs the game server emulation for one client.
func (s *Server) handleGameConn(conn net.Conn) {
	id := s.nextConnID()
	gc := &gameConn{
		server:          s,
		conn:            conn,
		crypt:           nil,
		id:              id,
		state:           gameStateHandshake,
		readBuf:         nil,
		session:         nil,
		charSelectedSeq: 0,
		queue:           nil,
		outCh:           make(chan []byte, outboundQueueSize),
		done:            make(chan struct{}),
		closeOnce:       sync.Once{},
	}
	s.clients.Add(1)
	s.logger.Printf("game#%d: client connected from %s", id, conn.RemoteAddr())
	defer func() {
		gc.shutdown("connection closed")
		s.clients.Add(-1)
	}()

	if err := gc.run(); err != nil {
		s.logger.Printf("game#%d: %v", id, err)
	}
}

// run performs the handshake and then serves the packet exchange until
// the client disconnects or the relay winds down.
func (gc *gameConn) run() error {
	if err := gc.handshake(); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	for {
		payload, err := readWirePacket(gc.conn, gc.readBuf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The client closed the connection itself: a normal
				// disconnect, not a failure.
				return nil
			}

			return fmt.Errorf("client read failed: %w", err)
		}
		gc.readBuf = payload
		if len(payload) == 0 {
			continue
		}

		gc.crypt.Decrypt(payload)
		if err := gc.handleClientPacket(payload); err != nil {
			return err
		}
	}
}

// handshake expects the unencrypted ProtocolVersion packet and answers
// with an unencrypted KeyPacket carrying the proxy session key; after
// it returns every packet of the connection rides the XOR cipher.
func (gc *gameConn) handshake() error {
	payload, err := readWirePacket(gc.conn, gc.readBuf)
	if err != nil {
		return fmt.Errorf("failed to read protocol version: %w", err)
	}
	gc.readBuf = payload
	if len(payload) < 5 || payload[0] != 0x00 {
		return fmt.Errorf("unexpected first packet % x", payload)
	}
	version := int32(uint32(payload[1]) | uint32(payload[2])<<8 |
		uint32(payload[3])<<16 | uint32(payload[4])<<24)
	if version == -2 {
		return errors.New("client ping probe (protocol -2), disconnecting")
	}
	if version != togameserver.C1ProtocolVersion {
		gc.sendRawKeyPacket(0, crypt.DefaultGameCryptKey())

		return fmt.Errorf("client protocol version %d rejected", version)
	}

	// The real Mobius C1 server answers with one fixed session key
	// (GameClient.CRYPT_KEY: "the last 4 bytes are fixed") for every
	// connection, so a C1 client build with a hardcoded key stays
	// compatible. A random per connection key desynchronized the real
	// client: its encrypted AuthLogin decrypted into garbage and the
	// proxy closed the connection. The exact static key replicates the
	// real server for both client behaviors (hardcoded and honored).
	key := crypt.DefaultGameCryptKey()
	gc.sendRawKeyPacket(1, key)
	gc.crypt = crypt.NewGameCrypt(key)
	gc.crypt.Enable()
	gc.state = gameStateAuthed
	gc.server.logger.Printf(
		"game#%d: protocol %d accepted, cipher enabled (static key)",
		gc.id, version)

	go gc.runSender()

	return nil
}

// sendRawKeyPacket writes the unencrypted KeyPacket of the handshake:
// [opcode 0x00][result: 1][key: 8][serverID: 4][tail 1: 4].
func (gc *gameConn) sendRawKeyPacket(
	result byte, key [crypt.GameCryptKeySize]byte,
) {
	writer := packet.NewWriter()
	if err := writer.WriteInt8(gameOpKeyPacket); err != nil {
		return
	}
	if err := writer.WriteInt8(int8(result)); err != nil {
		return
	}
	if err := writer.WriteBytes(key[:]); err != nil {
		return
	}
	if err := writer.WriteInt32(1); err != nil {
		return
	}
	if err := writer.WriteInt32(1); err != nil {
		return
	}
	if err := writeWirePacket(gc.conn, writer.Bytes()); err != nil {
		gc.server.logger.Printf("game#%d: key packet write failed: %v", gc.id, err)
	}
}

// handleClientPacket dispatches one decrypted client packet by state.
func (gc *gameConn) handleClientPacket(payload []byte) error {
	opcode := payload[0]
	switch gc.state {
	case gameStateHandshake:
		// The handshake completes before the read loop starts; a packet
		// arriving here means a desynchronized client.
		gc.server.logger.Printf(
			"game#%d: packet 0x%02x before the handshake finished",
			gc.id, opcode)

	case gameStateAuthed:
		if opcode == gameOpAuthLogin {
			return gc.handleAuthLogin(payload)
		}
	case gameStateChars:
		switch opcode {
		case gameOpCharacterSelect:
			return gc.handleCharacterSelect(payload)
		case gameOpCharacterCreate:
			return gc.handleCharacterCreate()
		case gameOpNewCharacter, gameOpCharacterDelete:
			gc.server.logger.Printf(
				"game#%d: character management packet 0x%02x ignored by the emulation",
				gc.id, opcode)

			return nil
		}
	case gameStateSelected:
		if opcode == gameOpEnterWorld {
			return gc.handleEnterWorld()
		}
		gc.queueClientPacket(payload)

		return nil
	case gameStateWorld:
		return gc.transitToServer(payload)
	}

	gc.server.logger.Printf("game#%d: ignored packet 0x%02x in state %d",
		gc.id, opcode, gc.state)

	return nil
}

// handleAuthLogin accepts any session keys, resolves the bot session to
// serve (waiting for one to enter the world) and answers with the one
// character char list.
func (gc *gameConn) handleAuthLogin(payload []byte) error {
	login := readGameAuthLogin(payload)
	if login == "" {
		hexLen := min(len(payload), authLoginHexDumpLimit)
		gc.server.logger.Printf(
			"game#%d: auth login packet unreadable (len %d, decrypted % x): "+
				"continuing, any pair is accepted",
			gc.id, len(payload), payload[:hexLen])
		login = authLoginFallbackAccount
	}
	gc.server.logger.Printf(
		"game#%d: auth login for account %q (any pair is accepted)", gc.id, login)

	gc.session = gc.server.resolveSession()
	if gc.session == nil {
		return errors.New("no bot session available for the client")
	}
	if gc.session.tracker.Status() != state.StatusOnline {
		return errors.New("the bot session is not in the world yet")
	}
	gc.server.logger.Printf("game#%d: attached to bot session %q",
		gc.id, gc.session.id)

	charList, err := gc.buildCharacterList()
	if err != nil {
		return fmt.Errorf("failed to build the char list: %w", err)
	}
	if err := gc.sendToClient(charList); err != nil {
		return err
	}
	gc.state = gameStateChars

	return nil
}

// handleCharacterSelect answers with the recorded CharSelected packet
// of the bot session: the client asked for the only offered slot, so
// the slot index itself is irrelevant.
func (gc *gameConn) handleCharacterSelect(payload []byte) error {
	slot := int32(0)
	if len(payload) >= 5 {
		slot = int32(uint32(payload[1]) | uint32(payload[2])<<8 |
			uint32(payload[3])<<16 | uint32(payload[4])<<24)
	}
	gc.charSelectedSeq = gc.session.recorder.FirstPacketSeq(charSelectedOpcode)
	selected := gc.session.recorder.Entry(gc.charSelectedSeq)
	if selected == nil {
		return errors.New("no char selected packet recorded for the session")
	}
	gc.server.logger.Printf(
		"game#%d: char select (slot %d) answered with the recorded packet",
		gc.id, slot)
	if err := gc.sendToClient(selected); err != nil {
		return err
	}
	gc.state = gameStateSelected

	return nil
}

// handleCharacterCreate refuses creation attempts: the emulated account
// offers exactly one character.
func (gc *gameConn) handleCharacterCreate() error {
	gc.server.logger.Printf(
		"game#%d: character create refused (the proxy serves one character)",
		gc.id)
	writer := packet.NewWriter()
	if err := writer.WriteInt8(gameOpCharCreateFail); err != nil {
		return err
	}
	if err := writer.WriteInt32(charCreateFailTooMany); err != nil {
		return err
	}

	return gc.sendToClient(writer.Bytes())
}

// handleEnterWorld flushes the client packets queued between the char
// selection and the world entry, then starts the replay and the live
// relay of the bot session.
func (gc *gameConn) handleEnterWorld() error {
	gc.server.logger.Printf("game#%d: enter world, starting the session relay",
		gc.id)
	gc.state = gameStateWorld
	for _, queued := range gc.queue {
		if err := gc.transitToServer(queued); err != nil {
			return err
		}
	}
	gc.queue = nil

	go gc.runRelay()

	return nil
}

// queueClientPacket buffers a client packet of the selected state until
// the world entry.
func (gc *gameConn) queueClientPacket(payload []byte) {
	copied := make([]byte, len(payload))
	copy(copied, payload)
	gc.queue = append(gc.queue, copied)
}

// transitToServer forwards one decrypted client packet to the real game
// server through the bot session, applying the transformer seam.
func (gc *gameConn) transitToServer(payload []byte) error {
	transformed, ok := gc.server.transformer.ClientToServer(payload)
	if !ok {
		gc.server.logger.Printf("game#%d: dropped client packet 0x%02x",
			gc.id, payload[0])

		return nil
	}
	if err := gc.session.client.SendRaw(transformed); err != nil {
		return fmt.Errorf("failed to send client packet 0x%02x: %w",
			payload[0], err)
	}
	gc.server.logger.Printf("game#%d: client -> server 0x%02x (%d bytes)",
		gc.id, payload[0], len(payload))

	return nil
}

// runSender is the single writer of the client socket: it encrypts and
// sends every outbound payload in order, so the outbound cipher chain
// stays consistent no matter which goroutine produced the packet.
func (gc *gameConn) runSender() {
	buf := make([]byte, 0, 1024)
	for {
		select {
		case <-gc.done:
			return
		case payload := <-gc.outCh:
			// The payload may be shared with the recorder history: the
			// encryption transforms in place, so it runs on the copy.
			buf = append(buf[:0], payload...)
			gc.crypt.Encrypt(buf)
			if err := gc.conn.SetWriteDeadline(
				time.Now().Add(gameWriteTimeout)); err != nil {
				gc.shutdown("write deadline failed: " + err.Error())

				return
			}
			if err := writeWirePacket(gc.conn, buf); err != nil {
				gc.shutdown("client write failed: " + err.Error())

				return
			}
		}
	}
}

// runRelay brings the client up to the current world state (the
// recorded stream of the bot session after its CharSelected packet) and
// then continues with the live feed. Packets pass the transformer seam
// in both cases.
func (gc *gameConn) runRelay() {
	recorder := gc.session.recorder
	entries, sub := recorder.Attach(gc.charSelectedSeq)
	defer recorder.removeSubscriber(sub)

	replayBytes := 0
	for i := range entries {
		replayBytes += len(entries[i].payload)
	}
	gc.server.logger.Printf(
		"game#%d: replaying %d recorded packets (%d bytes), then live",
		gc.id, len(entries), replayBytes)

	for i := range entries {
		if !gc.relayToClient(entries[i].payload) {
			return
		}
	}
	gc.server.logger.Printf("game#%d: replay done, live relay active", gc.id)

	for {
		select {
		case <-gc.done:
			return
		case <-sub.poison:
			gc.shutdown("the client fell behind the live feed")

			return
		case <-recorder.CloseSignal():
			gc.shutdown("the bot session ended")

			return
		case update := <-sub.ch:
			if !gc.relayToClient(update.payload) {
				return
			}
		}
	}
}

// relayToClient transforms one server packet and hands it to the
// sender; it reports false when the connection is done.
func (gc *gameConn) relayToClient(payload []byte) bool {
	transformed, ok := gc.server.transformer.ServerToClient(payload)
	if !ok {
		return true
	}
	select {
	case gc.outCh <- transformed:
		return true
	case <-gc.done:
		return false
	}
}

// sendToClient queues one synthesized packet for the sender.
func (gc *gameConn) sendToClient(payload []byte) error {
	select {
	case gc.outCh <- payload:
		return nil
	case <-gc.done:
		return errors.New("the client connection is closing")
	}
}

// shutdown closes the client connection once with the given reason.
func (gc *gameConn) shutdown(reason string) {
	gc.closeOnce.Do(func() {
		gc.server.logger.Printf("game#%d: closing, %s", gc.id, reason)
		close(gc.done)
		_ = gc.conn.Close()
	})
}

// authLoginFallbackAccount is the placeholder account name used when
// the login string of the game AuthLogin packet cannot be decoded: the
// emulated server accepts any account, so a cosmetic field must never
// fail the client connection.
const authLoginFallbackAccount = "<unreadable>"

// authLoginHexDumpLimit bounds the decrypted hex dump logged for an
// unreadable auth login packet.
const authLoginHexDumpLimit = 48

// authLoginPrefixLen is the byte size of the opcode plus the short
// length prefix of the fallback auth login layout.
const authLoginPrefixLen = 3

// readGameAuthLogin extracts the login string of the game AuthLogin
// packet: [opcode 0x08][login: utf16][4 session key ints]. The session
// key values are accepted as whatever the client carries. The login is
// cosmetic, so the reader accepts both known utf16 layouts (null
// terminated like the Mobius readString, and short length prefixed) and
// degrades to an empty string instead of failing on anything else.
func readGameAuthLogin(payload []byte) string {
	if len(payload) < 2 {
		return ""
	}

	// The documented Mobius layout: a null terminated UTF-16LE string.
	login, err := packet.NewReader(payload[1:]).ReadStringFromUtf16Format()
	if err == nil {
		return login
	}

	// The fallback layout: [short char count][UTF-16LE data].
	if len(payload) >= authLoginPrefixLen {
		charCount := int(uint16(payload[1]) | uint16(payload[2])<<8)
		if charCount > 0 && authLoginPrefixLen+charCount*2 <= len(payload) {
			data := payload[authLoginPrefixLen : authLoginPrefixLen+charCount*2]
			units := make([]uint16, 0, charCount)
			for i := 0; i+1 < len(data); i += 2 {
				units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
			}

			return string(utf16.Decode(units))
		}
	}

	return ""
}
