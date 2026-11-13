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
    // validatePositionPeriod paces the client position validation: the ticker
    // sends only when the placement changed (or the standing heartbeat
    // elapsed), so a calm session stays quiet.
    validatePositionPeriod = time.Second
    // validatePositionIdleGap bounds the standing heartbeat: a still character
    // keeps the server view fresh at a whisper rate, under every flood
    // protector threshold.
    validatePositionIdleGap = 15 * time.Second
)

// silentSessionOpcodes lists the outbound maintenance opcodes that never own an
// ActionFailed answer, so the refusal attribution of sendPacket skips them: the
// handshake family (ProtocolVersion 0x00, AuthLogin 0x08, CharacterCreate 0x0B,
// CharacterSelect 0x0D), ChangeMoveType2 0x1C, Appearing 0x30, RequestNetPing
// 0xA8 and ValidatePosition 0x48 (which the server answers with nothing).
// Counting them would shadow the genuine walk refusals of a moving session.
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

// gameSilenceTimeout bounds the absolute packet silence of a live session. The
// server answers every RequestNetPing with a NetPing broadcast
// (RequestNetPing.runImpl sends it unconditionally, ~gamePingPeriod apart), so
// a healthy connection never goes quiet for minutes. A socket delivering
// nothing while the ping writes succeed is half-open (the host slept, the
// network black-holed, the JVM froze): the read side blocks forever and the bot
// stands frozen, which the 24/7 runs must not tolerate. The var is a test seam.
var gameSilenceTimeout = 3 * time.Minute

// The session end notifications of the server carry no body, so no
// parser owns their ids.
const (
    leaveWorldID  = 0x96
    serverCloseID = 0x36
)

type GameSessionParams struct {
    Account    string
    LoginOkID1 int32
    LoginOkID2 int32
    PlayOkID1  int32
    PlayOkID2  int32
}

type CharacterParams struct {
    Name      string
    Race      int32
    Female    int32
    ClassID   int32
    HairStyle int32
    HairColor int32
    Face      int32
}

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
    // htmlGen counts the NpcHTMLMessage arrivals: the dialog walker gates a
    // page wait on the generation, so a page left over from a previous
    // conversation never satisfies a fresh talk.
    htmlGen uint64
    // claimsOwnStream gates the echo ticker of the position validation while
    // the cursor key escape drives the session: the claimed placements own the
    // stream like the official client's own movement, and the broadcast echo
    // would fight them one broadcast behind (see ClaimValidatePosition).
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

// NewGameClient wraps a game server connection and performs the handshake: the
// unencrypted ProtocolVersion and KeyPacket exchange that enables the XOR
// cipher. The constructor seeds every reusable packet struct.
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

func (gc *GameClient) SetLogger(logger *log.Logger) {
    gc.logger = logger
}

// SetTracker attaches the state tracker; without it the client only logs.
func (gc *GameClient) SetTracker(tracker *state.Bot) {
    gc.tracker = tracker
}

// SetTap installs a callback that observes every decrypted server packet from
// the first CharSelectionInfo onward (the KeyPacket is unencrypted). It runs on
// the reader goroutine and must copy the payload: the buffer is reused by the
// next read. The proxy installs its history recorder here.
func (gc *GameClient) SetTap(tap func(payload []byte)) {
    gc.tap = tap
}

// SetSendTap installs a callback that observes every client packet right
// before the encryption (the acceptance run log installs its outbound decoder).
// It runs on the sending goroutine and must copy the payload: the encryption
// transforms the buffer in place.
func (gc *GameClient) SetSendTap(tap func(payload []byte)) {
    gc.sendTap = tap
}

// notifySendTap hands the plaintext of one outbound packet to the observer; the
// hot path checks nil because the fleet sessions run without a tap.
func (gc *GameClient) notifySendTap(payload []byte) {
    if gc.sendTap != nil {
        gc.sendTap(payload)
    }
}

// SendRaw sends a raw payload (opcode and body, no framing) through the session
// cipher like a typed sendPacket call, under the same critical section, so the
// outbound cipher chain stays consistent.
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

    // The encryption transforms in place and the payload may be a shared proxy
    // buffer, so it is copied into the outbound scratch first.
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

func (gc *GameClient) PacketCount() int {
    return int(gc.packetCount.Load())
}

// sendPacket serializes, encrypts and sends a packet. Encryption and the wire
// write share one critical section: the rolling XOR chain is stateful, so the
// encryption order must match the wire order across the caller goroutines.
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
        // The refusal bookkeeping: the walk clicks (0x01) and the action
        // requests answer ActionFailed the same way, so their send times decide
        // which arrival belongs to a click (see state.Bot.OtherRequestBetween).
        // The maintenance stream never answers and stays out.
        opcode := writer.Bytes()[0]
        now := time.Now()
        if opcode == togameserver.MoveToLocationPacketID {
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

type gamePacket struct {
    buf     *gameBuffer
    payload []byte
    err     error
}

// Run blocks while the character stays in the world: a reader goroutine, the
// periodic net pings, and the logout packet on context cancellation.
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

// runLoop is the receive loop of the session. Every packet re-arms the silence
// watchdog, and a silence past gameSilenceTimeout unwinds with an error, so the
// supervisor reconnects instead of blocking on a dead socket. The validate
// ticker drives the position validation (see validatePosition).
func (gc *GameClient) runLoop(
    ctx context.Context,
    packets <-chan gamePacket,
    pingTicker *time.Ticker,
    validateTicker *time.Ticker,
    characterName string,
) error {
    silence := time.NewTimer(gameSilenceTimeout)
    defer silence.Stop()
    // The validation state: the last placement and report moment, so the ticker
    // sends on a change or on the idle heartbeat.
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

// RequestLogout ends the session from another goroutine (the emergency logout):
// the packet announces the leave while the server accepts it (out of combat),
// and the socket close forces the rest - the server stores a character that
// left mid combat fifteen seconds after the combat ends.
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

// Close drops the connection without the logout announcement: the character
// selection stage holds no world session, so a plain close is the clean exit of
// a client that never entered the world (the creation probe of the acceptance
// runner).
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
