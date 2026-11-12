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
    "os"
    "sync"
    "sync/atomic"
    "time"

    "github.com/melg8/swarm/internal/swarm/crypt"
    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/packets/packet"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Protocol constants.
const (
    gamePingPeriod    = 25 * time.Second
    gameWriteTimeout  = 10 * time.Second
    gameHandshakeWait = 30 * time.Second
    packetChanSize    = 32
    bufferInitialSize = 4096
    // validatePositionPeriod paces the client position validation the
    // official client streams while its character moves: the ticker
    // only sends when the observed placement actually changed (or the
    // standing heartbeat elapsed), so a calm session stays quiet.
    validatePositionPeriod = time.Second
    // validatePositionIdleGap bounds the standing heartbeat of the
    // validation: a character that stands still keeps the server's
    // clientX/clientY/clientZ view fresh at a whisper rate instead of
    // the movement rate, far under every flood protector threshold.
    validatePositionIdleGap = 15 * time.Second
    // moveToLocationClientPacketID mirrors togameserver.
    // moveToLocationPacketID: the send path classifies the request by
    // its serialized opcode so the tracker can attribute the
    // ActionFailed answers honestly (see sendPacket).
    moveToLocationClientPacketID = 0x01
)

// silentSessionOpcodes lists the outbound maintenance opcodes that
// never own an ActionFailed answer - the refusal attribution
// bookkeeping of sendPacket skips them. The set is the complement of
// the answered family of the reference server (the ActionFailed grep
// of the Mobius C1 clientpackets: the move, attack, action, skill,
// item and transaction requests all answer it, the maintenance
// stream never does): the handshake family (ProtocolVersion 0x00,
// AuthLogin 0x08, CharacterCreate 0x0B, CharacterSelect 0x0D), the
// in-session maintenance (ChangeMoveType2 0x1C, Appearing 0x30,
// RequestNetPing 0xA8 which answers NetPing) and the client position
// validation stream (ValidatePosition 0x48, which the server answers
// with nothing at all). Counting any of these as a request would
// dismiss the genuine walk refusals that arrive while the stream
// flows - the one second validation ticker alone would shadow every
// refusal of a moving session (see sendPacket).
var silentSessionOpcodes = map[byte]bool{
    0x00: true, // ProtocolVersion
    0x08: true, // AuthLogin
    0x0B: true, // CharacterCreate
    0x0D: true, // CharacterSelect
    0x1C: true, // ChangeMoveType2
    0x30: true, // Appearing
    0x48: true, // ValidatePosition
    0xA8: true, // RequestNetPing
}

// gameSilenceTimeout bounds the absolute packet silence of a live
// game session. The server answers every RequestNetPing with a
// NetPing broadcast (RequestNetPing.runImpl sends it unconditionally,
// ~gamePingPeriod apart), so a healthy connection never goes quiet
// for minutes even in an empty world at night. A socket that
// delivers nothing while the ping writes keep "succeeding" is a
// half-open connection (the host slept, the network black-holed, the
// server JVM froze): the write side sinks the pings into the OS
// buffer for a long time, the read side blocks forever and the bot
// stands in the world doing nothing - exactly the frozen session the
// 24/7 runs must not tolerate. The var (not a const) is a test seam:
// the unit tests shorten it to keep the silence case under a second.
var gameSilenceTimeout = 3 * time.Minute

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

// Packet ids of the observed world packets.
const (
    moveToLocationID   = 0x01
    charInfoID         = 0x03
    attackID           = 0x06
    spawnItemID        = 0x15
    npcHTMLMessageID   = 0x1B
    dropItemID         = 0x16
    getItemID          = 0x17
    statusUpdateID     = 0x1A
    setupGaugeID       = 0x85
    deleteObjectID     = 0x1E
    npcInfoID          = 0x22
    itemListID         = 0x27
    inventoryUpdateID  = 0x37
    changeMoveTypeID   = 0x3E
    changeWaitTypeID   = 0x3F
    targetSelectedID   = 0x39
    targetUnselectedID = 0x3A
    autoAttackStartID  = 0x3B
    autoAttackStopID   = 0x3C
    teleportID         = 0x38
    stopMoveID         = 0x59
    moveToPawnID       = 0x75
    validateLocationID = 0x76
    beginRotationID    = 0x77
    stopRotationID     = 0x78
    myTargetSelectedID = 0xBF
    systemMessageID    = 0x7A
    creatureSayID      = 0x5D
    magicSkillUseID    = 0x5A
    skillListID        = 0x6D
    questListID        = 0x98
    abnormalStatusID   = 0x97
    socialActionID     = 0x3D
    actionFailedID     = 0x35
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
    conn           net.Conn
    crypt          *crypt.GameCrypt
    writeMu        sync.Mutex
    logger         *log.Logger
    trace          bool
    packetCount    atomic.Int64
    readBuf        []byte
    tracker        *state.Bot
    tap            func(payload []byte)
    sendTap        func(payload []byte)
    rawWriteBuf    []byte
    npcInfo        fromgameserver.NpcInfoPacket
    userInfo       fromgameserver.UserInfoPacket
    charInfo       fromgameserver.CharInfoPacket
    moveTo         fromgameserver.MoveToLocationPacket
    moveToPawn     fromgameserver.MoveToPawnPacket
    stopMove       fromgameserver.StopMovePacket
    validateLoc    fromgameserver.ValidateLocationPacket
    deleted        fromgameserver.DeleteObjectPacket
    dropItem       fromgameserver.DropItemPacket
    spawnItem      fromgameserver.SpawnItemPacket
    getItem        fromgameserver.GetItemPacket
    statusUpd      fromgameserver.StatusUpdatePacket
    setupGauge     fromgameserver.SetupGaugePacket
    attack         fromgameserver.AttackPacket
    attackStart    fromgameserver.AutoAttackStartPacket
    attackStop     fromgameserver.AutoAttackStopPacket
    beginRotation  fromgameserver.BeginRotationPacket
    stopRotation   fromgameserver.StopRotationPacket
    changeMoveType fromgameserver.ChangeMoveTypePacket
    changeWait     fromgameserver.ChangeWaitTypePacket
    teleport       fromgameserver.TeleportToLocationPacket
    myTarget       fromgameserver.MyTargetSelectedPacket
    targetSelected fromgameserver.TargetSelectedPacket
    targetDropped  fromgameserver.TargetUnselectedPacket
    systemMessage  fromgameserver.SystemMessagePacket
    creatureSay    fromgameserver.CreatureSayPacket
    magicSkillUse  fromgameserver.MagicSkillUsePacket
    socialAction   fromgameserver.SocialActionPacket
    actionFailed   fromgameserver.ActionFailedPacket
    itemList       fromgameserver.ItemListPacket
    invUpdate      fromgameserver.InventoryUpdatePacket
    skillList      fromgameserver.SkillListPacket
    questList      fromgameserver.QuestListPacket
    abnormalStatus fromgameserver.AbnormalStatusUpdatePacket
    npcHTML        fromgameserver.NpcHTMLMessage
    invItems       []state.InventoryItem
    skills         []state.LearnedSkill
    statusAttrs    [statusAttrsCapacity]state.Attribute
    htmlMu         sync.Mutex
    lastHTML       fromgameserver.NpcHTMLMessage
    // htmlGen counts the NpcHTMLMessage packets the connection
    // received: the arrival generation of the dialog store above.
    // The hunt-side dialog walker gates its page waits on it (a
    // page left over from a previous conversation never satisfies
    // the wait of a fresh talk), so it rides the same lock as
    // lastHTML and the snapshot read stays consistent.
    htmlGen uint64
    // claimsOwnStream gates the echo ticker of the client position
    // validation while the cursor key escape drives the session:
    // the claimed placements own the stream exactly the way the
    // official client's own movement simulation does while the
    // player walks with the arrows, and the echo of the broadcast
    // position would fight the claims one broadcast behind (see
    // ClaimValidatePosition).
    claimsOwnStream atomic.Bool
}

// statusAttrsCapacity bounds the scratch attributes of status updates.
const statusAttrsCapacity = 8

// packetTraceEnv enables a one line trace of every received packet id
// when set, which is the fastest way to follow a protocol flow live.
const packetTraceEnv = "SWARM_TRACE_PACKETS"

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
// The constructor initializes every reusable packet struct (the
// exhaustruct convention), which exceeds the line budget.
func NewGameClient(conn net.Conn) (*GameClient, error) { //nolint:funlen
    client := &GameClient{
        conn:            conn,
        crypt:           nil,
        writeMu:         sync.Mutex{},
        logger:          log.Default(),
        trace:           os.Getenv(packetTraceEnv) != "",
        packetCount:     atomic.Int64{},
        readBuf:         nil,
        tracker:         nil,
        tap:             nil,
        sendTap:         nil,
        rawWriteBuf:     nil,
        npcInfo:         *fromgameserver.NewNpcInfoPacket(),
        userInfo:        *fromgameserver.NewUserInfoPacket(),
        charInfo:        *fromgameserver.NewCharInfoPacket(),
        moveTo:          *fromgameserver.NewMoveToLocationPacket(),
        moveToPawn:      *fromgameserver.NewMoveToPawnPacket(),
        stopMove:        *fromgameserver.NewStopMovePacket(),
        validateLoc:     *fromgameserver.NewValidateLocationPacket(),
        deleted:         *fromgameserver.NewDeleteObjectPacket(),
        dropItem:        *fromgameserver.NewDropItemPacket(),
        spawnItem:       *fromgameserver.NewSpawnItemPacket(),
        getItem:         *fromgameserver.NewGetItemPacket(),
        statusUpd:       *fromgameserver.NewStatusUpdatePacket(),
        setupGauge:      *fromgameserver.NewSetupGaugePacket(),
        attack:          *fromgameserver.NewAttackPacket(),
        attackStart:     *fromgameserver.NewAutoAttackStartPacket(),
        attackStop:      *fromgameserver.NewAutoAttackStopPacket(),
        beginRotation:   *fromgameserver.NewBeginRotationPacket(),
        stopRotation:    *fromgameserver.NewStopRotationPacket(),
        changeMoveType:  *fromgameserver.NewChangeMoveTypePacket(),
        changeWait:      *fromgameserver.NewChangeWaitTypePacket(),
        teleport:        *fromgameserver.NewTeleportToLocationPacket(),
        myTarget:        *fromgameserver.NewMyTargetSelectedPacket(),
        targetSelected:  *fromgameserver.NewTargetSelectedPacket(),
        targetDropped:   *fromgameserver.NewTargetUnselectedPacket(),
        systemMessage:   *fromgameserver.NewSystemMessagePacket(),
        creatureSay:     *fromgameserver.NewCreatureSayPacket(),
        magicSkillUse:   *fromgameserver.NewMagicSkillUsePacket(),
        socialAction:    *fromgameserver.NewSocialActionPacket(),
        actionFailed:    *fromgameserver.NewActionFailedPacket(),
        itemList:        *fromgameserver.NewItemListPacket(),
        invUpdate:       *fromgameserver.NewInventoryUpdatePacket(),
        skillList:       *fromgameserver.NewSkillListPacket(),
        questList:       *fromgameserver.NewQuestListPacket(),
        abnormalStatus:  *fromgameserver.NewAbnormalStatusUpdatePacket(),
        invItems:        nil,
        skills:          nil,
        statusAttrs:     [statusAttrsCapacity]state.Attribute{},
        claimsOwnStream: atomic.Bool{},
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

    if err := conn.SetReadDeadline(
        time.Now().Add(gameHandshakeWait)); err != nil {
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

// SetTracker attaches the state tracker that observes the session. The
// tracker is optional; without it the client only logs packets.
func (gc *GameClient) SetTracker(tracker *state.Bot) {
    gc.tracker = tracker
}

// SetTap installs a callback that observes every decrypted server packet
// of the session, from the first CharSelectionInfo onward (the handshake
// KeyPacket is unencrypted and never tapped). The callback runs on the
// session reader goroutine and must copy the payload synchronously: the
// buffer is reused by the next read. The proxy server installs the
// history recorder here.
func (gc *GameClient) SetTap(tap func(payload []byte)) {
    gc.tap = tap
}

// SetSendTap installs a callback that observes every client packet of the
// session right before the encryption: the serialized plaintext of the
// typed send calls and of SendRaw (the acceptance run log installs its
// outbound decoder here). The callback runs on the sending goroutine and
// must copy the payload synchronously: the encryption transforms the
// buffer in place afterwards.
func (gc *GameClient) SetSendTap(tap func(payload []byte)) {
    gc.sendTap = tap
}

// notifySendTap hands the plaintext of one outbound packet to the
// installed observer. The nil check stays on the hot path: the tap is
// absent on the fleet sessions of the process.
func (gc *GameClient) notifySendTap(payload []byte) {
    if gc.sendTap != nil {
        gc.sendTap(payload)
    }
}

// SendRaw sends a raw decrypted client packet payload (opcode and body,
// without wire framing) through the session cipher, exactly like a typed
// sendPacket call: the encryption and the wire write share the same
// critical section so hunt loop actions and proxied client packets keep
// one consistent outbound cipher chain.
func (gc *GameClient) SendRaw(payload []byte) error {
    if len(payload) == 0 {
        return nil
    }
    if gc.trace {
        gc.logger.Printf("Sent raw packet id 0x%02x", payload[0])
    }
    gc.notifySendTap(payload)

    gc.writeMu.Lock()
    defer gc.writeMu.Unlock()

    // The encryption transforms the buffer in place and the payload
    // may be backed by a shared proxy buffer, so it is copied into the
    // reusable outbound scratch first.
    wire := make([]byte, 0, len(payload))
    wire = append(wire, payload...)
    gc.rawWriteBuf = wire
    gc.crypt.Encrypt(wire)

    if err := gc.conn.SetWriteDeadline(
        time.Now().Add(gameWriteTimeout)); err != nil {
        return fmt.Errorf("failed to set write deadline: %w", err)
    }
    if err := writeWirePacket(gc.conn, wire); err != nil {
        return fmt.Errorf("failed to send raw game packet: %w", err)
    }

    return nil
}

// PacketCount returns the number of packets received so far.
func (gc *GameClient) PacketCount() int {
    return int(gc.packetCount.Load())
}

// sendPacket serializes, encrypts and sends a game server packet. The
// encryption and the wire write share one critical section: the game
// cipher is a stateful rolling XOR chain, so the encryption order must
// match the wire order exactly (the run loop and the client action
// callers run on different goroutines).
func (gc *GameClient) sendPacket(data crypt.Serializable) error {
    writer := packet.NewWriter()
    if err := data.ToBytes(writer); err != nil {
        return fmt.Errorf("failed to serialize game packet: %w", err)
    }
    if gc.trace {
        gc.logger.Printf("Sent packet id 0x%02x", writer.Bytes()[0])
    }
    gc.notifySendTap(writer.Bytes())
    if gc.tracker != nil {
        // The request bookkeeping of the refusal channel: the walk
        // clicks (opcode 0x01) and the action requests (the equips,
        // the skills, the transactions) answer ActionFailed the same
        // way, so their send times decide which arrival may be
        // attributed to a click at all (an equip refusal riding the
        // click would otherwise read as a refused walk, see
        // state.Bot.OtherRequestBetween). The maintenance stream
        // never owns an ActionFailed answer, so its opcodes stay out
        // of the bookkeeping (see silentSessionOpcodes).
        opcode := writer.Bytes()[0]
        now := time.Now()
        if opcode == moveToLocationClientPacketID {
            gc.tracker.ApplyMoveRequestSent(now)
        } else if !silentSessionOpcodes[opcode] {
            gc.tracker.ApplyOtherRequestSent(now)
        }
    }

    gc.writeMu.Lock()
    defer gc.writeMu.Unlock()
    gc.crypt.Encrypt(writer.Bytes())

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
    if gc.tap != nil {
        gc.tap(payload)
    }

    return payload, nil
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
    if gc.tracker != nil {
        gc.tracker.SetOnline(characterName)
    }

    err := gc.run(ctx, characterName)
    if gc.tracker != nil {
        gc.tracker.SetOffline()
    }

    return err
}

// run is the implementation of Run without the tracker bookkeeping.
func (gc *GameClient) run(ctx context.Context, characterName string) error {
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
    validateTicker := time.NewTicker(validatePositionPeriod)
    defer validateTicker.Stop()

    err := gc.runLoop(ctx, packets, pingTicker, validateTicker,
        characterName)
    // The logout is only announced while the connection is still usable:
    // after a transport error the packet would fail and only spam the log.
    gc.disconnect(err == nil)

    // Unblock the reader goroutine and drain pending packets.
    drainPackets(packets, readerDone)

    return err
}

// runLoop is the main receive loop of the in game session. The
// silence watchdog bounds the packet gaps: every received packet
// re-arms the timer, and a session that stays silent past
// gameSilenceTimeout unwinds with an error so the supervisor
// reconnects instead of blocking on a dead socket forever (see
// gameSilenceTimeout). The validateTicker drives the client position
// validation of the official client (see validatePosition).
func (gc *GameClient) runLoop(
    ctx context.Context,
    packets <-chan gamePacket,
    pingTicker *time.Ticker,
    validateTicker *time.Ticker,
    characterName string,
) error {
    silence := time.NewTimer(gameSilenceTimeout)
    defer silence.Stop()
    // The validation state: the last placement the session reported
    // and the moment of the last report, so the ticker sends only on
    // a change (the movement rate) or on the idle heartbeat (see
    // validatePositionIdleGap).
    validation := positionValidation{
        x:        0,
        y:        0,
        z:        0,
        heading:  0,
        reported: false,
        at:       time.Time{},
    }
    for {
        select {
        case <-ctx.Done():
            gc.logger.Println("Leaving the game world with " + characterName)

            return nil
        case <-pingTicker.C:
            if err := gc.sendPacket(
                &togameserver.RequestNetPing{}); err != nil {
                return fmt.Errorf("failed to send net ping: %w", err)
            }
        case <-validateTicker.C:
            if err := gc.validatePosition(&validation); err != nil {
                return fmt.Errorf("failed to validate position: %w", err)
            }
        case <-silence.C:
            return fmt.Errorf("game session silent for %s, closing the "+
                "connection (the server stopped answering while the "+
                "socket stayed writable)", gameSilenceTimeout)
        case msg, ok := <-packets:
            if !ok {
                return nil
            }
            if msg.err != nil {
                return fmt.Errorf("game connection lost: %w", msg.err)
            }
            gc.handleServerPacket(msg.payload)
            bufferPool.Put(msg.buf)
            silence.Reset(gameSilenceTimeout)
        }
    }
}

// RequestLogout ends the session from another goroutine (the hunt
// loop of the emergency logout): the logout packet announces the
// leave while the server still accepts it (out of combat), the
// socket close forces the rest - the server stores a character
// that left mid combat fifteen seconds after the combat ends. The
// read loop of Run notices the closed socket and unwinds the
// session.
func (gc *GameClient) RequestLogout() error {
    if err := gc.sendPacket(&togameserver.Logout{}); err != nil {
        gc.logger.Printf("Failed to announce the logout: %v", err)
    }
    if err := gc.conn.Close(); err != nil {
        return fmt.Errorf(
            "failed to close the game connection: %w", err)
    }

    return nil
}

// Close drops the game connection without the logout announcement.
// The character selection stage holds no world session, so a plain
// socket close is the clean exit of a client that never entered the
// world (the character creation probe of the acceptance runner); the
// server frees the account the same way it does for a client closing
// at the char screen.
func (gc *GameClient) Close() error {
    if err := gc.conn.Close(); err != nil {
        return fmt.Errorf(
            "failed to close the game connection: %w", err)
    }

    return nil
}

// disconnect closes the connection, announcing the logout to the server
// first when the connection is still usable.
func (gc *GameClient) disconnect(announce bool) {
    if announce {
        if err := gc.sendPacket(&togameserver.Logout{}); err != nil {
            gc.logger.Printf("Failed to send logout: %v", err)
        }
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
