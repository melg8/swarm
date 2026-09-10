// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
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
// packet transits to the real game server through the bot connection -
// except the keepalive: the RequestNetPing of the client is answered by
// the proxy itself and never reaches the server (the NetPing answers of
// the bot session are filtered out of the relay, see
// isNetPingAnswerPayload - transit plus relay would close a ping
// feedback loop that accelerates without bound).
//
// A mid-session switch onto another bot (the WebUI selection) or onto
// the replacement session of the same bot (the relogin handoff) rides
// the restart flow: the proxy plays the server side of the in-game
// Restart button - RestartResponse moves the client to its char select
// screen (the client tears its own world down there, so no ghost
// objects and no stale self pawn survive), the char list of the target
// bot is offered, and the unsolicited CharSelected answer follows
// shortly after, exactly as the user's own double click of the only
// listed character. The client then loads and sends its EnterWorld, and
// the relay continues with the replay and the live feed of the target.
type gameConn struct {
	server  *Server
	conn    net.Conn
	crypt   *crypt.GameCrypt
	id      int64
	state   gameState
	readBuf []byte

	// mu guards the session handoff fields: the relay goroutine
	// swaps the session when the held client is resynced onto the
	// replacement bot session, while the read loop keeps reading
	// the client packets and transiting them through the current
	// session (or swallowing them while the client is held). It also
	// guards netPingTime, the game time harvested from the suppressed
	// NetPing answers of the bot session and carried by the locally
	// synthesized client answers.
	mu          sync.Mutex
	session     *botSession
	holding     bool
	userLogout  bool
	netPingTime int32

	charSelectedSeq int64
	queue           [][]byte

	// relayLive, autoSelect and enterWorldCh are guarded by mu:
	// relayLive marks the relay goroutine as the owner of the
	// stream (the read loop signals instead of starting a second
	// one when a restart dance hands the flow back), autoSelect is
	// the pending unsolicited char selected answer of a dance, and
	// enterWorldCh carries the replay sequence the read loop hands
	// the parked relay when the client re-enters the world.
	relayLive    bool
	autoSelect   *time.Timer
	enterWorldCh chan int64

	outCh      chan []byte
	done       chan struct{}
	closeOnce  sync.Once
	senderLive atomic.Bool
	senderDone chan struct{}
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

// shutdownFlushWait bounds how long shutdown waits for the sender to
// flush the queued packets before the socket is force closed: the
// graceful flush of a shutdown (the LeaveWorld of the held logout)
// takes one write on a live client, so a second is generous, and a
// stalled write must not hold the connection teardown hostage.
const shutdownFlushWait = time.Second

// outboundQueueSize bounds the client bound packet queue of one client.
const outboundQueueSize = 512

// botSwitchPollPeriod is how often the live relay retries a pending
// bot switch: the WebUI selection fired but the target bot was not
// online yet (registered but still connecting, or not registered at
// all), so the relay keeps serving the current live feed and polls
// for the target to enter the world - the client lands on the newly
// selected bot within one period of it appearing, without a stalled
// feed or a reconnect.
const botSwitchPollPeriod = 250 * time.Millisecond

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
		mu:              sync.Mutex{},
		session:         nil,
		holding:         false,
		userLogout:      false,
		charSelectedSeq: 0,
		queue:           nil,
		relayLive:       false,
		autoSelect:      nil,
		enterWorldCh:    make(chan int64, 1),
		outCh:           make(chan []byte, outboundQueueSize),
		done:            make(chan struct{}),
		closeOnce:       sync.Once{},
		senderLive:      atomic.Bool{},
		senderDone:      make(chan struct{}),
		netPingTime:     0,
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
	switch gc.currentState() {
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
		if opcode == gameOpCharacterSelect {
			// The client is still at (or back on) the char
			// select screen: the unsolicited answer of the
			// auto select may not have taken (the packet raced
			// the client state), and the user's own double
			// click is served the same way - idempotent.
			return gc.handleCharacterSelect(payload)
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
	gc.setState(gameStateChars)

	return nil
}

// handleCharacterSelect answers a client character select with the
// recorded CharSelected packet of the bot session patched to the live
// tracker state (see serveCharSelectedLive). The slot index itself is
// irrelevant (the client asked for the only offered slot), but the
// answer must carry the live place of the character: a client picking
// the char after a restart dance lands where the bot actually stands.
// A user click cancels the pending auto select of a dance (the user
// wins the race against the proxy's own double click).
func (gc *gameConn) handleCharacterSelect(payload []byte) error {
	slot := int32(0)
	if len(payload) >= 5 {
		slot = int32(uint32(payload[1]) | uint32(payload[2])<<8 |
			uint32(payload[3])<<16 | uint32(payload[4])<<24)
	}
	gc.cancelAutoSelect()
	if err := gc.serveCharSelectedLive(); err != nil {
		return err
	}

	live := gc.currentSession().tracker.SelfSnapshot()
	gc.server.logger.Printf(
		"game#%d: char select (slot %d) answered with the recorded packet, "+
			"live state %d %d %d hp %.0f/%.0f level %d",
		gc.id, slot, live.X, live.Y, live.Z, live.CurHP, live.MaxHP,
		live.Level)

	return nil
}

// serveCharSelectedLive serves the char selected answer of the current
// bot session: the newest WebUI selection wins (a selection change that
// fired while the client was loading or sitting on the char select
// screen redirects the entry onto the newly selected bot), then the
// recorded CharSelected packet of the resolved session is rewritten with
// the live snapshot (the position, vitals and progression fields, see
// patchCharSelectedLive) and queued to the client. Idempotent: the read
// loop (the user's double click), the auto select timer of a restart
// dance and a re-click after a lost answer all ride this one path.
func (gc *gameConn) serveCharSelectedLive() error {
	if next := gc.server.tryResolveSelectedSession(); next != nil {
		gc.setSession(next)
	}
	session := gc.currentSession()
	if session == nil {
		return errors.New("no bot session available for the char selection")
	}

	gc.mu.Lock()
	if gc.state != gameStateChars && gc.state != gameStateSelected {
		gc.mu.Unlock()

		return nil // the client is not selecting a character
	}
	seq := session.recorder.FirstPacketSeq(charSelectedOpcode)
	if seq == 0 {
		gc.mu.Unlock()

		return errors.New("no char selected packet recorded for the session")
	}
	selected := session.recorder.Entry(seq)
	if selected == nil {
		gc.mu.Unlock()

		return errors.New("the recorded char selected entry is gone")
	}
	live := session.tracker.SelfSnapshot()
	answer := patchCharSelectedLive(selected, live)
	gc.charSelectedSeq = seq
	gc.state = gameStateSelected
	gc.mu.Unlock()

	return gc.sendToClient(answer)
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
// selection and the world entry, then hands the stream to the relay: a
// fresh connection starts the relay goroutine, while a re-entry after a
// restart dance wakes the parked relay (it owns the lifecycle) with the
// replay sequence of the just served char selected answer.
func (gc *gameConn) handleEnterWorld() error {
	gc.server.logger.Printf("game#%d: enter world, starting the session relay",
		gc.id)

	gc.mu.Lock()
	if gc.state != gameStateSelected {
		gc.mu.Unlock()

		return nil // not selecting: a stray re-entry
	}
	gc.state = gameStateWorld
	seq := gc.charSelectedSeq
	relayLive := gc.relayLive
	gc.relayLive = true
	gc.mu.Unlock()

	for _, queued := range gc.queue {
		if err := gc.transitToServer(queued); err != nil {
			return err
		}
	}
	gc.queue = nil

	if relayLive {
		// A restart dance parked the relay on this connection:
		// the re-entry is its signal to continue.
		select {
		case gc.enterWorldCh <- seq:
		case <-gc.done:
		}

		return nil
	}

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

// currentSession returns the bot session the connection is attached to
// right now (the read loop transits the client packets through it). The
// relogin handoff swaps it under the same lock.
func (gc *gameConn) currentSession() *botSession {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	return gc.session
}

// setSession swaps the bot session the connection is attached to (the
// resync of the relogin handoff).
func (gc *gameConn) setSession(session *botSession) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.session = session
}

// setHolding toggles the hold of the relogin handoff: while held, the
// client packets are swallowed instead of transiting - the bot session
// they would reach is gone and the world behind the client is frozen
// anyway.
func (gc *gameConn) setHolding(holding bool) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.holding = holding
}

// isHolding reports whether the client is held for the bot relogin.
func (gc *gameConn) isHolding() bool {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	return gc.holding
}

// markUserLogout records that the client itself asked for the logout: the
// LeaveWorld answer is then relayed (the client returns to the login
// screen on its own) and the session end closes the connection instead
// of holding it.
func (gc *gameConn) markUserLogout() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.userLogout = true
}

// clientLoggedOut reports whether the client asked for the logout.
func (gc *gameConn) clientLoggedOut() bool {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	return gc.userLogout
}

// transitToServer forwards one decrypted client packet to the real game
// server through the bot session, applying the transformer seam. The
// keepalive (RequestNetPing) is the one exception: it is answered by the
// proxy itself and never transits (see answerNetPingLocally) - a held
// client gets its answer too, the keepalive must survive the freeze.
// Every other packet of a held client (its bot session ended, the
// relogin pending) is swallowed: the character is offline, and the
// frozen world state behind the client makes every action meaningless
// anyway - except the logout: a user that wants out of the frozen world
// gets the login screen (the synthesized LeaveWorld), not a swallowed
// intent.
//
// A send failure never kills the client connection: it means the bot
// session link died under the transit (the narrow window before the
// relay noticed the recorder close and engaged the hold). The recorder
// close is the authority on the session end - it triggers the hold of
// the relogin handoff, and killing the client here would break it.
func (gc *gameConn) transitToServer(payload []byte) error {
	if payload[0] == clientOpLogout {
		gc.markUserLogout()
		if gc.isHolding() {
			// The user wants out of the frozen world: the character is
			// offline, so the server cannot answer the logout - the
			// proxy hands the client the login screen itself.
			gc.relayToClient(buildLeaveWorldPacket())
			gc.shutdown("the user logged out while held for the bot relogin")

			return nil
		}
	}
	if payload[0] == clientOpNetPing {
		// The client keepalive never reaches the real server: the bot
		// session already keeps that connection alive with its own ping
		// cycle, and a transit would close the ping feedback loop (the
		// server answer would be recorded and relayed, an answer driven
		// client would re-ping per answer, and the loop would accelerate
		// without bound on the attached bot session). The proxy answers
		// the keepalive itself, exactly like the server would.
		gc.answerNetPingLocally()

		return nil
	}
	if gc.isHolding() {
		gc.server.logger.Printf(
			"game#%d: client packet 0x%02x swallowed while held for the bot relogin",
			gc.id, payload[0])

		return nil
	}
	transformed, ok := gc.server.transformer.ClientToServer(payload)
	if !ok {
		gc.server.logger.Printf("game#%d: dropped client packet 0x%02x",
			gc.id, payload[0])

		return nil
	}
	if err := gc.currentSession().client.SendRaw(transformed); err != nil {
		gc.server.logger.Printf(
			"game#%d: client packet 0x%02x not sent, the bot "+
				"session link is failing: %v",
			gc.id, payload[0], err)

		return nil
	}
	gc.server.logger.Printf("game#%d: client -> server 0x%02x (%d bytes)",
		gc.id, payload[0], len(payload))

	return nil
}

// answerNetPingLocally serves the client keepalive without a server
// round trip: the synthesized NetPing answer carries the game time the
// relay harvested from the last real answer of the bot session (the
// C1 client uses it for its clock cosmetics; before the first harvest
// it is zero, which only delays the day-night rendering). A send
// failure is silent: the connection teardown owns the reporting, and
// the answer rate of an answer driven client forbids per packet logs.
func (gc *gameConn) answerNetPingLocally() {
	gc.mu.Lock()
	gameTime := gc.netPingTime
	gc.mu.Unlock()

	answer := make([]byte, 5)
	answer[0] = serverOpNetPing
	binary.LittleEndian.PutUint32(answer[1:], uint32(gameTime))
	_ = gc.sendToClient(answer)
}

// noteNetPingGameTime harvests the game time of one suppressed NetPing
// answer of the bot session so the locally synthesized client answers
// carry the value the real server would have sent.
func (gc *gameConn) noteNetPingGameTime(payload []byte) {
	if len(payload) < 5 {
		return
	}
	gameTime := int32(binary.LittleEndian.Uint32(payload[1:5]))
	gc.mu.Lock()
	gc.netPingTime = gameTime
	gc.mu.Unlock()
}

// runSender is the single writer of the client socket: it encrypts and
// sends every outbound payload in order, so the outbound cipher chain
// stays consistent no matter which goroutine produced the packet. On
// the connection close it flushes the packets already queued (a
// shutdown right behind a queued packet - the LeaveWorld of the held
// logout - must still reach the client) and then closes the socket
// itself: the reader is released by the close, not by the done flag.
func (gc *gameConn) runSender() {
	gc.senderLive.Store(true)
	defer func() {
		close(gc.senderDone)
		_ = gc.conn.Close()
	}()

	buf := make([]byte, 0, 1024)
	for {
		select {
		case <-gc.done:
			gc.drainOutbound(buf)

			return
		case payload := <-gc.outCh:
			if !gc.writeOutbound(buf, payload) {
				return
			}
		}
	}
}

// drainOutbound flushes the packets queued at the moment of the close;
// a failed write aborts the drain (the socket is gone).
func (gc *gameConn) drainOutbound(buf []byte) {
	for {
		select {
		case payload := <-gc.outCh:
			if !gc.writeOutbound(buf, payload) {
				return
			}
		default:
			return
		}
	}
}

// writeOutbound encrypts one payload on the scratch buffer and writes
// it to the client socket. It reports false when the write failed: the
// socket is closed right away (the reader must be released), the
// remaining queue is dropped.
func (gc *gameConn) writeOutbound(buf []byte, payload []byte) bool {
	// The payload may be shared with the recorder history: the
	// encryption transforms in place, so it runs on the copy.
	buf = append(buf[:0], payload...)
	gc.crypt.Encrypt(buf)
	if err := gc.conn.SetWriteDeadline(
		time.Now().Add(gameWriteTimeout)); err != nil {
		gc.server.logger.Printf("game#%d: write deadline failed: %v",
			gc.id, err)
		_ = gc.conn.Close()

		return false
	}
	if err := writeWirePacket(gc.conn, buf); err != nil {
		gc.server.logger.Printf("game#%d: client write failed: %v",
			gc.id, err)
		_ = gc.conn.Close()

		return false
	}

	return true
}

// runRelay streams the sessions of the client's bot until the
// connection ends. Each cycle replays the recorded history of one bot
// session and follows it with the live feed; when that session ends and
// the client did not ask for the logout itself, the cycle performs the
// relogin handoff (hold, restart dance) and the loop continues with the
// replacement session. A WebUI selection change mid cycle swaps the
// client onto the newly selected bot through the same dance, so the
// relay goroutine stays the single owner of the stream for the whole
// life of the connection.
func (gc *gameConn) runRelay() {
	defer gc.setRelayLive(false)
	fromSeq := gc.charSelectedSeq
	for {
		next, ok := gc.streamSession(fromSeq)
		if !ok {
			return
		}
		fromSeq = next
	}
}

// streamSession serves one bot session cycle: the replay of everything
// recorded after the given sequence (the enter world burst and the
// history, live-patched for the played character) followed by the live
// feed. It returns the replay start sequence of the next cycle (the
// CharSelected the client was served after a restart dance) when the
// session switched, ok=false when the connection is done.
//
// The LeaveWorld answer of a bot initiated logout is suppressed in both
// the replay and the live feed (see handoff.go): a client that processes
// it drops itself to the login screen, which breaks the hold. A client
// that asked for the logout itself still receives it.
func (gc *gameConn) streamSession(fromSeq int64) (int64, bool) {
	session := gc.currentSession()
	recorder := session.recorder
	entries, sub := recorder.Attach(fromSeq)
	defer recorder.removeSubscriber(sub)

	// Snapshot the selection channel at the start of the cycle: when
	// SelectBot fires it (closes it), the live feed select wakes up
	// and the client switches onto the newly selected bot.
	selectionCh := gc.server.selectionChannel()

	// A selection whose target bot is not online yet (registered but
	// still connecting, or not registered at all) arms a pending
	// switch: the live feed keeps flowing while the poll ticker below
	// retries the switch, so the client lands on the target the
	// moment it enters the world. Without the pending the relay would
	// stay on the current bot forever: SelectBot of the same id is a
	// no-op that never refires the channel.
	pendingSwitch := ""
	switchPoll := time.NewTicker(botSwitchPollPeriod)
	defer switchPoll.Stop()

	selfID := session.tracker.SelfObjectID()
	lastSelfMoveSeq := lastSelfMovementSeq(entries, selfID)

	replayBytes := 0
	for i := range entries {
		replayBytes += len(entries[i].payload)
	}
	gc.server.logger.Printf(
		"game#%d: replaying %d recorded packets (%d bytes) with the live self state "+
			"(self id %d), then live",
		gc.id, len(entries), replayBytes, selfID)

	gc.replayHistory(entries, selfID, lastSelfMoveSeq, session)
	gc.server.logger.Printf("game#%d: replay done, live relay active", gc.id)

	for {
		select {
		case <-gc.done:
			return 0, false
		case <-sub.poison:
			gc.shutdown("the client fell behind the live feed")

			return 0, false
		case <-recorder.CloseSignal():
			return gc.serveRelogin(session)
		case <-selectionCh:
			target, next := gc.serveBotSwitch(session)
			if next != nil {
				return gc.parkAtCharSelect()
			}
			// The selection did not resolve to a different online
			// session: stay on the current live feed, and arm the
			// pending switch when the target is not online yet
			// ("" when the selection names the current bot or
			// nothing).
			pendingSwitch = target
			selectionCh = gc.server.selectionChannel()
		case <-switchPoll.C:
			next, ok := gc.retryPendingSwitch(session, pendingSwitch)
			if !ok {
				return 0, false
			}
			if next != nil {
				return gc.parkAtCharSelect()
			}
		case update := <-sub.ch:
			if !gc.deliverLiveUpdate(update.payload) {
				return 0, false
			}
		}
	}
}

// deliverLiveUpdate applies the live feed policy of one recorded
// packet: the LeaveWorld of a bot logout is suppressed (the client
// stays for the relogin hold), the NetPing answers of the bot session
// are filtered out with their game time harvested (the keepalive round
// trips carry no world state and relaying them arms the ping feedback
// loop, see isNetPingAnswerPayload), and everything else is relayed to
// the client. It reports false when the connection is done.
func (gc *gameConn) deliverLiveUpdate(payload []byte) bool {
	if isLeaveWorldPayload(payload) && !gc.clientLoggedOut() {
		gc.server.logger.Printf(
			"game#%d: leave world suppressed, the client stays for "+
				"the bot relogin",
			gc.id)

		return true
	}
	if isNetPingAnswerPayload(payload) {
		gc.noteNetPingGameTime(payload)

		return true
	}

	return gc.relayToClient(payload)
}

// serveBotSwitch handles a WebUI selection change while a client is
// connected: it resolves the newly selected bot session and, when it
// differs from the current one and is ready (online with its char
// selected answer recorded), starts the restart dance onto it. The
// client returns to its char select screen, is auto selected onto the
// new character and re-enters the world on it: the position, the
// appearance, the race, the class and the equipment all arrive through
// the enter world packets of the new bot - no reconnect needed. Returns
// the target id together with the new session when the dance started;
// the target id with a nil session when the target is not ready yet
// (the caller arms the pending switch and keeps serving the current
// live feed); "" with a nil session when the selection resolves to the
// current bot or nothing at all.
func (gc *gameConn) serveBotSwitch(current *botSession) (string, *botSession) {
	target := gc.server.SelectedBot()
	if target == "" || target == current.id {
		return "", nil
	}
	next := gc.server.sessionByID(target)
	if next == nil || next == current || !switchReady(next) {
		gc.server.logger.Printf(
			"game#%d: selection switched to %q but the bot is not online "+
				"yet, serving %q until it enters the world",
			gc.id, target, current.id)

		return target, nil
	}
	gc.server.logger.Printf(
		"game#%d: selection switched from %q to %q, starting the restart dance",
		gc.id, current.id, next.id)
	if !gc.beginRestartSwitch(next) {
		return "", nil
	}

	return target, next
}

// retryPendingSwitch resolves one poll tick of the pending bot switch:
// the WebUI selection named a bot that was not online yet when it
// fired, and the tick checks whether the target entered the world in
// the meantime. It starts the restart dance onto it and returns the
// new session with ok=true, returns nil with ok=true while the target
// is still not online (the live feed keeps flowing), and ok=false when
// the connection died mid dance.
func (gc *gameConn) retryPendingSwitch(
	session *botSession, pending string,
) (*botSession, bool) {
	if pending == "" || pending == session.id {
		return nil, true
	}
	next := gc.server.sessionByID(pending)
	if next == nil || next == session || !switchReady(next) {
		return nil, true
	}
	gc.server.logger.Printf(
		"game#%d: the pending switch target %q entered the world, "+
			"starting the restart dance",
		gc.id, pending)
	if !gc.beginRestartSwitch(next) {
		return nil, false
	}

	return next, true
}

// beginRestartSwitch starts the restart dance for a live in-world
// client: the exact packet pair the real server answers the in-game
// Restart button with (RestartResponse followed by the char list - see
// RequestRestart.handlePacket of the Mobius C1 server) moves the client
// to its own char select screen, and the session of the connection
// swaps to the target bot so the char list, the char selected answer
// and the following replay all serve the new character. The dance
// cannot fail on a live connection: the synthesized packets are fixed
// layout and the char list builder degrades gracefully.
//
// The C1 client tears its whole world down when it processes the
// RestartResponse - that is what makes the dance the only correct
// switch: the teleport + DeleteObject sweep approach crashes the client
// (a UserInfo of an object id the client never spawned dereferences a
// missing pawn in the UserInfoPacket handler - the reported General
// protection fault), and a UserInfo of a nearby known object leaves the
// client controlling its old pawn. A restart is the one mid-session
// identity change the client is designed to process.
func (gc *gameConn) beginRestartSwitch(next *botSession) bool {
	gc.setSession(next)
	gc.setState(gameStateChars)

	if err := gc.sendToClient(buildRestartResponsePacket()); err != nil {
		gc.server.logger.Printf(
			"game#%d: the restart answer did not reach the client: %v",
			gc.id, err)

		return false
	}
	list, err := gc.buildCharacterList()
	if err != nil {
		gc.server.logger.Printf(
			"game#%d: the char list of %q could not be built (%v), "+
				"continuing with the dance",
			gc.id, next.id, err)
	} else if err := gc.sendToClient(list); err != nil {
		gc.server.logger.Printf(
			"game#%d: the char list of %q did not reach the client: %v",
			gc.id, next.id, err)

		return false
	}
	gc.armAutoSelect()
	gc.server.logger.Printf(
		"game#%d: restart dance onto %q offered (auto select in %s)",
		gc.id, next.id, switchTimings())

	return true
}

// offerCharList re-points a parked client at the given session: the
// client already sits on its char select screen, so the dance only
// swaps the session, pushes the char list of the newly selected bot and
// re-arms the auto select - the screen re-renders the model of the new
// character and the unsolicited answer follows as before.
func (gc *gameConn) offerCharList(next *botSession) bool {
	gc.setSession(next)
	list, err := gc.buildCharacterList()
	if err != nil {
		gc.server.logger.Printf(
			"game#%d: the char list of %q could not be built: %v",
			gc.id, next.id, err)

		return true // the auto select still lands on the new session
	}
	if err := gc.sendToClient(list); err != nil {
		gc.server.logger.Printf(
			"game#%d: the char list of %q did not reach the client: %v",
			gc.id, next.id, err)

		return false
	}
	gc.armAutoSelect()
	gc.server.logger.Printf(
		"game#%d: the parked char select screen re-offered onto %q",
		gc.id, next.id)

	return true
}

// parkAtCharSelect parks the relay while the client sits on the char
// select screen of a restart dance: it waits for the read loop to
// report the re-entry (the EnterWorld after the served char selected
// answer), tracks newer WebUI selections (the screen re-offers the
// newest char list) and polls for a pending target that was offline at
// the selection time. It returns the replay sequence of the entered
// session, or ok=false when the connection ended.
func (gc *gameConn) parkAtCharSelect() (int64, bool) {
	// Drain any stale re-entry report before parking: a duplicate
	// EnterWorld of the previous cycle must not leak into this one.
	for len(gc.enterWorldCh) > 0 {
		<-gc.enterWorldCh
	}

	pending := ""
	poll := time.NewTicker(botSwitchPollPeriod)
	defer poll.Stop()
	for {
		selectionCh := gc.server.selectionChannel()
		select {
		case <-gc.done:
			return 0, false
		case seq := <-gc.enterWorldCh:
			return seq, true
		case <-selectionCh:
			target, next := parkSelectionTarget(
				gc.server, gc.currentSession().id)
			if next != nil {
				if !gc.offerCharList(next) {
					return 0, false
				}
				pending = ""
			} else {
				pending = target
			}
		case <-poll.C:
			var ok bool
			pending, ok = gc.parkPendingOffer(pending)
			if !ok {
				return 0, false
			}
		}
	}
}

// parkSelectionTarget resolves what a selection change means for a
// parked client: the ready session of the newest selection to
// re-offer, or the pending target id when the selection is not ready
// yet ("" when the selection names the current bot or nothing - the
// park keeps offering what it has).
func parkSelectionTarget(s *Server, currentID string) (string, *botSession) {
	target := s.SelectedBot()
	if target == "" || target == currentID {
		return "", nil
	}
	if next := s.sessionByID(target); next != nil && switchReady(next) {
		return "", next
	}

	return target, nil
}

// parkPendingOffer completes one poll tick of a pending target armed
// while the client is parked: when the target became ready, the char
// list is re-offered onto it and the pending clears. It reports ok=false
// only when the re-offer failed (the client connection is gone).
func (gc *gameConn) parkPendingOffer(pending string) (string, bool) {
	if pending == "" || pending == gc.currentSession().id {
		return pending, true
	}
	next := gc.server.sessionByID(pending)
	if next == nil || !switchReady(next) {
		return pending, true
	}
	if !gc.offerCharList(next) {
		return "", false
	}

	return "", true
}

// armAutoSelect schedules the unsolicited char selected answer of the
// dance: after the delay the proxy plays the double click of the only
// listed character itself, so the WebUI selection alone carries the
// client into the world of the new bot (a hands-off switch). The user's
// own double click cancels the pending answer (see cancelAutoSelect);
// whichever fires first wins and the other becomes a no-op.
func (gc *gameConn) armAutoSelect() {
	delay := switchTimings()
	gc.mu.Lock()
	if gc.autoSelect != nil {
		gc.autoSelect.Stop()
	}
	gc.autoSelect = time.AfterFunc(delay, gc.autoSelectCharSelected)
	gc.mu.Unlock()
}

// cancelAutoSelect drops the pending unsolicited answer: the user's own
// double click arrived first.
func (gc *gameConn) cancelAutoSelect() {
	gc.mu.Lock()
	if gc.autoSelect != nil {
		gc.autoSelect.Stop()
		gc.autoSelect = nil
	}
	gc.mu.Unlock()
}

// autoSelectCharSelected is the proxy's own double click: it serves the
// char selected answer of the offered character while the client still
// sits on the char select screen. A client that moved on (the state
// left the chars screen - the user clicked first or the connection is
// gone) makes the timer a no-op.
func (gc *gameConn) autoSelectCharSelected() {
	gc.mu.Lock()
	gc.autoSelect = nil
	if gc.state != gameStateChars {
		gc.mu.Unlock()

		return
	}
	gc.mu.Unlock()

	gc.server.logger.Printf(
		"game#%d: auto selecting the offered character, serving the char "+
			"selected answer",
		gc.id)
	if err := gc.serveCharSelectedLive(); err != nil {
		gc.shutdown("the auto char select failed: " + err.Error())
	}
}

// setRelayLive records whether the relay goroutine owns the stream of
// the connection (the read loop signals a parked relay instead of
// starting a second one).
func (gc *gameConn) setRelayLive(live bool) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.relayLive = live
}

// setState transitions the client state machine of the connection.
func (gc *gameConn) setState(state gameState) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.state = state
}

// currentState snapshots the client state machine of the connection.
func (gc *gameConn) currentState() gameState {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	return gc.state
}

// serveRelogin handles the end of the streamed bot session: a client
// that asked for the logout itself is closed with the session, any
// other client is held through the bot relogin and taken through the
// restart dance onto the replacement session (the same character
// re-enters its fresh world). It returns the replay start sequence of
// the replacement session (ok=true) or the end of the connection.
func (gc *gameConn) serveRelogin(session *botSession) (int64, bool) {
	if gc.clientLoggedOut() {
		gc.shutdown("the bot session ended after the client logout")

		return 0, false
	}
	next := gc.holdForRelogin(session)
	if next == nil {
		return 0, false
	}
	if !gc.beginRestartSwitch(next) {
		return 0, false
	}

	return gc.parkAtCharSelect()
}

// replayHistory streams the recorded history of one session cycle: the
// LeaveWorld of a bot logout is skipped (the hold depends on the client
// staying in the world), the NetPing answers of the bot session are
// skipped with their game time harvested (see isNetPingAnswerPayload),
// the UserInfo of the played character carries the live state and the
// stale self movement is dropped except the newest packet.
func (gc *gameConn) replayHistory(
	entries []RecorderEntry, selfID int32, lastSelfMoveSeq int64,
	session *botSession,
) {
	droppedMoves := 0
	for i := range entries {
		payload := entries[i].payload
		if isLeaveWorldPayload(payload) && !gc.clientLoggedOut() {
			continue
		}
		if isNetPingAnswerPayload(payload) {
			gc.noteNetPingGameTime(payload)

			continue
		}
		if isSelfMovementPayload(payload, selfID) &&
			entries[i].seq != lastSelfMoveSeq {
			droppedMoves++

			continue
		}
		if payload[0] == replayOpUserInfo {
			payload = patchUserInfoSelfLive(
				payload, selfID, session.tracker.SelfSnapshot())
		}
		if !gc.relayToClient(payload) {
			return
		}
	}
	if droppedMoves > 0 {
		gc.server.logger.Printf(
			"game#%d: replay dropped %d stale self movement packets",
			gc.id, droppedMoves)
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
// When the sender goroutine is live it flushes the queued packets and
// closes the socket itself; shutdown only waits a bounded time for
// that (a stalled write cannot hold the teardown hostage) and force
// closes the socket afterwards. A connection without the sender (a
// failed handshake) is closed directly.
func (gc *gameConn) shutdown(reason string) {
	gc.closeOnce.Do(func() {
		gc.server.logger.Printf("game#%d: closing, %s", gc.id, reason)
		gc.cancelAutoSelect()
		close(gc.done)
		if gc.senderLive.Load() {
			select {
			case <-gc.senderDone:
			case <-time.After(shutdownFlushWait):
				_ = gc.conn.Close()
			}
		}
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
