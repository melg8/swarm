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
    "sync/atomic"
    "time"

    "github.com/melg8/swarm/internal/swarm/crypt"
    "github.com/melg8/swarm/internal/swarm/packets/packet"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// gameConn drives one client connection of the emulated game server:
// the static-key handshake, the emulated pre-world packets, then the
// replay and live feed of the selected bot (docs/proxy.md).
type gameConn struct {
    server  *Server
    conn    net.Conn
    crypt   *crypt.GameCrypt
    id      int64
    state   gameState
    readBuf []byte

    // mu guards the session handoff fields (the session swap, the hold
    // and the logout flags) and netPingTime, the game time harvested
    // from the suppressed NetPing answers for the local client answers.
    mu          sync.Mutex
    session     *botSession
    holding     bool
    userLogout  bool
    netPingTime int32

    charSelectedSeq int64
    queue           [][]byte

    // viewLogAt bounds the client view divergence logging of the
    // ValidatePosition interception (the corrections themselves may repeat with
    // every report), vetoLogAt the same for the dropped CannotMoveAnymore
    // vetoes. Owned by the read loop: the transits all run there.
    viewLogAt time.Time
    vetoLogAt time.Time

    // relayLive, autoSelect and enterWorldCh are guarded by mu: relayLive marks
    // the relay goroutine as the owner of the stream (the read loop signals
    // instead of starting a second one when a restart dance hands the flow
    // back), autoSelect is the pending unsolicited char selected answer of a
    // dance, and enterWorldCh carries the replay sequence the read loop hands
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

// The periodic client position report (ValidatePosition, 0x48), the
// client walk veto (CannotMoveAnymore, 0x36) and the server position
// correction answer (ValidateLocation, 0x76), all Mobius C1 values.
const (
    clientOpValidatePosition  = 0x48
    clientOpCannotMoveAnymore = 0x36
    serverOpValidateLocation  = 0x76
)

// validateDivergenceSlack widens the server out of sync rule for the projection
// error: the self projection runs up to one 100 ms tick ahead and the client
// report is one network hop old. The rule is the live move speed; the server
// corrects only above 500 units (or a >200 z gap), so the band under 500 is the
// silent snap window this check closes.
const validateDivergenceSlack = 25.0

// validateLogPeriod bounds the divergence logging: a client whose
// view stays stale reports once per second, one log line per report
// would flood the proxy log.
const validateLogPeriod = 10 * time.Second

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

// botSwitchPollPeriod is how often the live relay retries a pending bot switch:
// the WebUI selection fired but the target bot was not online yet (registered
// but still connecting, or not registered at all), so the relay keeps serving
// the current live feed and polls for the target to enter the world - the
// client lands on the newly selected bot within one period of it appearing,
// without a stalled feed or a reconnect.
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
        viewLogAt:       time.Time{},
        vetoLogAt:       time.Time{},
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
        gc.server.logger.Printf("game#%d: key packet write failed: %v",
            gc.id, err)
    }
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

// setHolding toggles the hold of the relogin handoff: while held, the client
// packets are swallowed instead of transiting - the bot session they would
// reach is gone and the world behind the client is frozen anyway.
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
// LeaveWorld answer is then relayed (the client returns to the login screen on
// its own) and the session end closes the connection instead of holding it.
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

// shutdown closes the client connection once with the given reason. When the
// sender goroutine is live it flushes the queued packets and closes the socket
// itself; shutdown only waits a bounded time for that (a stalled write cannot
// hold the teardown hostage) and force closes the socket afterwards. A
// connection without the sender (a failed handshake) is closed directly.
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
