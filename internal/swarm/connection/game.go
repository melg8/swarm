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
    "strconv"
    "sync"
    "sync/atomic"
    "time"

    "github.com/melg8/swarm/internal/swarm/crypt"
    "github.com/melg8/swarm/internal/swarm/gear"
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

// Character selection constants. The server drops a CharacterSelect
// silently when it races the creation flow: the updated char list is
// written before the server side char selection cache is updated, so a
// select that arrives in that window finds no character and is ignored
// without any answer. The wait for the CharSelected answer is therefore
// bounded and the selection retransmitted; the server flood protector
// allows one select per 3 seconds (30 game ticks of 100 ms), which the
// 5 second wait between attempts respects.
const (
    charSelectWait     = 5 * time.Second
    charSelectAttempts = 3
    charCreateOkWait   = 2 * time.Second
    // charListWait bounds one pre-world read outside the bounded
    // selection dance: the character list answer of the AuthLogin
    // exchange and each packet of the creation exchange (see
    // EnsureCharacter). These reads sit before the in-world silence
    // watchdog exists, so an unbounded read here parks the bot
    // goroutine forever on a stalled or half-open server while the
    // supervisor waits for a session that never returns.
    charListWait = 30 * time.Second
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

// AttackTarget repeats the attack request for a target that is already
// selected. This is the second click of the Mobius semantics: the server
// resolves it to onForcedAttack and notifies the player AI with the
// ATTACK intention, which starts the chase and the auto attack.
func (gc *GameClient) AttackTarget(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf("failed to attack target %d: object unknown",
            objectID)
    }
    if err := gc.sendAttackRequest(objectID, x, y, z); err != nil {
        return fmt.Errorf("failed to attack: %w", err)
    }

    return nil
}

// sendAttackRequest serializes and sends one AttackRequest packet.
func (gc *GameClient) sendAttackRequest(
    targetID int32, x int32, y int32, z int32,
) error {
    request := togameserver.NewAttackRequestPacket()
    request.TargetID = targetID
    request.X = x
    request.Y = y
    request.Z = z

    return gc.sendPacket(request)
}

// PickupItem clicks a ground item: the server walks the character to it
// and adds it to the inventory (the movement is broadcast as
// MoveToLocation and the pickup as GetItem with a StopMove to self).
func (gc *GameClient) PickupItem(item state.LootItem) error {
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = item.ObjectID
    request.X = item.X
    request.Y = item.Y
    request.Z = item.Z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to pick up item: %w", err)
    }
    gc.tracker.RecordEvent("picking up " + item.Name)

    return nil
}

// DestroyItem destroys inventory items to free slots or weight.
func (gc *GameClient) DestroyItem(objectID int32, count int32) error {
    request := togameserver.NewRequestDestroyItem()
    request.ObjectID = objectID
    request.Count = count
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to destroy item: %w", err)
    }

    return nil
}

// UseItem uses an inventory item. Equippable items toggle their
// equipped state (the same packet equips and unequips, see
// UseItem.runImpl -> useEquippableItem), other items run their item
// handler. The web UI drives it from the equipment widget: double
// click and drag-and-drop of the cells.
func (gc *GameClient) UseItem(objectID int32) error {
    request := togameserver.NewRequestUseItem()
    request.ObjectID = objectID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use item: %w", err)
    }
    gc.tracker.RecordEvent("using item " + strconv.Itoa(int(objectID)))

    return nil
}

// DropItem drops an inventory item on the ground at the given world
// position: the server only accepts drops within 150 units of the
// player (see RequestDropItem.runImpl), so the caller passes the
// character position. Stackable items drop a partial stack through the
// count, the server splits the stack itself.
func (gc *GameClient) DropItem(
    objectID int32, count int32, x int32, y int32, z int32,
) error {
    request := togameserver.NewRequestDropItem()
    request.ObjectID = objectID
    request.Count = count
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to drop item: %w", err)
    }
    gc.tracker.RecordEvent("dropping " + strconv.Itoa(int(count)) +
        " of item " + strconv.Itoa(int(objectID)))

    return nil
}

// SellItems sells inventory items to the targeted merchant. The packet
// uses the standard inventory sell list of the official client (list id
// 0): the server prices every item itself at referencePrice/2, answers
// with InventoryUpdate removals and adds the adena.
func (gc *GameClient) SellItems(items []state.InventoryItem) error {
    request := togameserver.NewRequestSellItemPacket()
    entries := make([]togameserver.SellItemEntry, 0, len(items))
    for _, item := range items {
        entries = append(entries, togameserver.SellItemEntry{
            ObjectID: item.ObjectID,
            ItemID:   item.ItemID,
            Count:    item.Count,
        })
    }
    request.Items = entries
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to sell items: %w", err)
    }

    return nil
}

// BuyItems buys items from the buylist of the targeted merchant. The
// server requires the selected merchant within the interaction
// distance and prices every entry itself (the reference price with
// the town tax), so the request carries item ids and counts only.
// Buying shares the transaction flood protector with selling, so the
// caller paces it like the sell batches.
func (gc *GameClient) BuyItems(listID int32, items []gear.Purchase) error {
    request := togameserver.NewRequestBuyItemPacket()
    request.ListID = listID
    entries := make([]togameserver.BuyItemEntry, 0, len(items))
    for _, item := range items {
        entries = append(entries, togameserver.BuyItemEntry{
            ItemID: item.ItemID,
            Count:  item.Count,
        })
    }
    request.Items = entries
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to buy items: %w", err)
    }

    return nil
}

// RequestInventory asks the server for the full inventory list.
func (gc *GameClient) RequestInventory() error {
    if err := gc.sendPacket(&togameserver.RequestItemList{}); err != nil {
        return fmt.Errorf("failed to request item list: %w", err)
    }

    return nil
}

// ActionSitStand toggles between sitting and standing (RequestActionUse
// action 0). The hunt loop uses it to rest at low HP: the sitting
// regeneration is faster. The server refuses the transition while
// moving, casting or attacking, so the caller sends it only while idle.
func (gc *GameClient) ActionSitStand() error {
    request := togameserver.NewRequestActionUsePacket()
    request.ActionID = togameserver.ActionSitStand
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send action use: %w", err)
    }

    return nil
}

// ClickObject selects a world object through the plain client click
// (Action 0x04): the NpcClick handler of the server selects the npc
// AND remembers it as the last folk the character talked to - the
// RequestAcquireSkill learning resolves its trainer through exactly
// that. The hunt loop clicks the skill teacher with it before the
// lesson requests.
func (gc *GameClient) ClickObject(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf(
            "failed to click object %d: object unknown", objectID)
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to click object: %w", err)
    }
    gc.tracker.RecordEvent("talking to the teacher")

    return nil
}

// InteractPull fires the attack analog of the npc interaction: the
// second plain client click on an already selected npc (the client
// double click). The Mobius NpcClick handler resolves it to the
// interact branch - for a folk npc out of the interaction distance
// the player AI takes the INTERACT intention and walks the character
// to the npc along the straight line (thinkInteract moves the pawn
// to 36 units), opening the dialog on arrival; within the distance
// it just opens the dialog. The interaction distance is therefore
// met whatever the geometry between the character and the npc does.
// It is deliberately NOT the real attack request (AttackTarget, the
// 0x0A packet the guard engage drives): no forced attack intention,
// no swings, the folk npc never takes damage - and a plain click on
// an npc the character does not hold yet only selects it, so the
// pull is safe at any range.
func (gc *GameClient) InteractPull(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf(
            "failed to interact pull %d: object unknown", objectID)
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to interact pull: %w", err)
    }
    gc.tracker.RecordEvent("attack analog pull to the npc")

    return nil
}

// ClearTarget drops the selection a conversation with an npc left
// behind (the teacher or the merchant the trip talked to): the client
// cannot unselect directly - the C1 protocol has no deselect request
// and the server never clears a selection on its own (only the next
// selection replaces it) - so the self click is the official client
// way out: Action 0x04 on the own object id runs through the
// PlayerClick handler, selects the character itself (MyTargetSelected
// of the own id, the friendly npc selection is gone) and the follow
// intention on self moves nothing. The hunt loop calls it when a trip
// stop finishes talking, so the leftover villager selection never
// reaches the hunting engage.
func (gc *GameClient) ClearTarget() error {
    objectID := gc.tracker.SelfObjectID()
    if objectID == 0 || gc.tracker.SelfTargetID() == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.SelfPosition()
    if !ok {
        x, y, z = 0, 0, 0
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to clear the target: %w", err)
    }
    gc.tracker.RecordEvent("clearing the target after the talk")

    return nil
}

// AcquireSkill learns one lesson of the class skill tree at the
// teacher the character last talked to (see ClickObject): the server
// charges the SP, consumes the required skill book and answers with
// a fresh SkillList.
func (gc *GameClient) AcquireSkill(skillID int32, level int32) error {
    request := togameserver.NewRequestAcquireSkillPacket()
    request.SkillID = skillID
    request.Level = level
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to acquire skill: %w", err)
    }
    gc.tracker.RecordEvent(fmt.Sprintf(
        "learning skill %d level %d", skillID, level))

    return nil
}

// UseMagicSkill casts a learned active skill: a strike at the
// selected target, a self buff on the caster. The server runs the
// cast flow (the range check, the mana cost, the reuse delay) and
// answers the refusals with ActionFailed.
func (gc *GameClient) UseMagicSkill(skillID int32) error {
    request := togameserver.NewRequestMagicSkillUsePacket()
    request.SkillID = skillID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use magic skill: %w", err)
    }

    return nil
}

// RestartAtVillage revives a dead character at the nearest village
// restart point, exactly like the death dialog of the official client
// (RequestRestartPoint 0x6D type 0). The server refuses the request
// while the character is alive, so the caller sends it only after death.
func (gc *GameClient) RestartAtVillage() error {
    request := togameserver.NewRequestRestartPointPacket()
    request.PointType = togameserver.RestartTypeVillage
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send restart point: %w", err)
    }

    return nil
}

// positionValidation is the send state of the client position
// validation loop: the placement of the last report and its moment,
// so the ticker sends only what the official client sends - a
// validation whenever the observed placement changed (the movement
// rate) and a sparse heartbeat while the character stands still (see
// validatePositionIdleGap).
type positionValidation struct {
    x, y, z  int32
    heading  int32
    reported bool
    at       time.Time
}

// validatePosition sends the client position validation of the
// official client (ValidatePosition 0x48) when the observed placement
// of the character changed since the last report, or when the idle
// heartbeat elapsed while the character stood still. The official
// client streams this packet from its own movement simulation and the
// server builds half of its session view from it: the clientZ the z
// adoption gate reads, the last server position the door logout
// exploit check compares, the client heading. A bot that never
// validates leaves that view frozen at its defaults - the C1
// clientZ adoption branch (Math.abs(_z - player.getClientZ()) < 800)
// can never run for a session whose client z the server still reads
// as zero - and every deployment whose movement validation expects
// the client liveness signal answers the silence its own way. The
// report carries exactly the tracker placement the server itself
// broadcast: nothing is claimed the server did not say.
func (gc *GameClient) validatePosition(validation *positionValidation) error {
    if gc.tracker == nil {
        return nil
    }
    // The cursor key claims own the stream while the escape runs:
    // the echo of the broadcast position would lag the claims by one
    // broadcast and snap the character back a step each tick while
    // the server's cursor key flag holds (the claimed placements
    // move the character unconditionally, see
    // ClaimValidatePosition).
    if gc.claimsOwnStream.Load() {
        return nil
    }
    x, y, z, ok := gc.tracker.SelfPosition()
    if !ok {
        return nil
    }
    heading := gc.tracker.SelfHeading()
    now := time.Now()
    changed := !validation.reported || x != validation.x || y != validation.y ||
        z != validation.z || heading != validation.heading
    idle := now.Sub(validation.at) >= validatePositionIdleGap
    if !changed && !idle {
        return nil
    }
    request := togameserver.NewValidatePositionPacket()
    request.X = x
    request.Y = y
    request.Z = z
    request.Heading = heading
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send validate position: %w", err)
    }
    validation.x, validation.y, validation.z = x, y, z
    validation.heading = heading
    validation.reported = true
    validation.at = now

    return nil
}

// WalkTo makes the character walk to a world point, exactly like a
// ground click of the official client (client MoveToLocation 0x01 in
// mouse mode). The hunt loop uses it to run toward a drop: this build of
// the Mobius C1 server has no click handler for ground items, so the
// character would otherwise never move toward the loot by itself.
func (gc *GameClient) WalkTo(x int32, y int32, z int32) error {
    selfX, selfY, selfZ, ok := gc.tracker.SelfPosition()
    if !ok {
        return errors.New("failed to walk: own position is unknown")
    }
    // The mouse click returns the session to the mouse movement
    // mode: the server clears the cursor key flag the cursor key
    // escape armed (MoveToLocation.runImpl), so the echo ticker of
    // the client position validation owns the stream again (see
    // claimsOwnStream).
    gc.claimsOwnStream.Store(false)
    request := togameserver.NewMoveToLocationRequestPacket()
    request.TargetX = x
    request.TargetY = y
    request.TargetZ = z
    request.OriginX = selfX
    request.OriginY = selfY
    request.OriginZ = selfZ
    request.Mode = togameserver.MoveModeMouse
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send move to location: %w", err)
    }

    return nil
}

// CursorKeyWalkTo sends the keyboard-mode move request (MoveToLocation
// 0x01 in cursor key mode, the arrow keys of the official client): the
// server arms the cursor key movement of the session - the packet's
// origin is adopted within the thousand unit window and the cursor key
// flag latches - and every ValidatePosition the session streams
// afterwards moves the character server-side without any click
// validation (ValidatePosition.runImpl syncs the claimed placement
// into the world and broadcasts it). The ground a click-refusing cell
// owns answers no mouse click at all; the cursor key stream is the
// movement that still walks it off (the 2026-09-14 15:10 report: even
// the official client stood frozen on the village plaza cell until
// the player walked the arrows). The hunt loop arms it only for the
// cursor key escape of a click-refusing stuck (see
// hunt.Loop.beginCursorKeyEscape).
func (gc *GameClient) CursorKeyWalkTo(
    x int32, y int32, z int32,
) error {
    selfX, selfY, selfZ, ok := gc.tracker.SelfPosition()
    if !ok {
        return errors.New(
            "failed to walk with the cursor keys: own position is unknown")
    }
    request := togameserver.NewMoveToLocationRequestPacket()
    request.TargetX = x
    request.TargetY = y
    request.TargetZ = z
    request.OriginX = selfX
    request.OriginY = selfY
    request.OriginZ = selfZ
    request.Mode = togameserver.MoveModeCursorKeys
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf(
            "failed to send the cursor key move: %w", err)
    }

    return nil
}

// ClaimValidatePosition reports a CLAIMED client position of the
// character (ValidatePosition 0x48): while the server's cursor key
// movement is armed, the claimed placement is synced straight into
// the world and broadcast (ValidatePosition.runImpl - the cursor key
// branch), so the claims walk the character without any click
// validation. This is the packet stream the official client drives
// from its own movement simulation while the player walks with the
// arrows - the arrow walk being the only movement a click-refusing
// cell answers (the 2026-09-14 15:10 report). The claims gate the
// echo ticker (see claimsOwnStream): the echo of the broadcast
// position would otherwise lag the claims by one broadcast and snap
// the character back a step each tick while the cursor key flag
// holds. The first mouse-mode walk clears the gate (see WalkTo).
func (gc *GameClient) ClaimValidatePosition(
    x int32, y int32, z int32, heading int32,
) error {
    request := togameserver.NewValidatePositionPacket()
    request.X = x
    request.Y = y
    request.Z = z
    request.Heading = heading
    gc.claimsOwnStream.Store(true)
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf(
            "failed to send the claimed position: %w", err)
    }

    return nil
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

    // The char list answer bounds like every pre-world read (see
    // charListWait): the silence watchdog of the live session does
    // not exist yet.
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charListWait)); err != nil {
        return nil, fmt.Errorf(
            "failed to set the character list deadline: %w", err)
    }
    payload, err := gc.readPacket(gc.readBuf)
    if err != nil {
        return nil, fmt.Errorf("failed to read character list: %w", err)
    }
    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        return nil, fmt.Errorf(
            "failed to reset the character list deadline: %w", err)
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
            // The updated list precedes the trailing CharCreateOk on the
            // wire (the server writes the list from inside the creation
            // handler and the ok after it returns). Draining the ok with a
            // bounded wait guarantees the server side char selection cache
            // is populated before the next packet selection runs, which
            // closes the silent drop race of a fast client.
            gc.drainCharCreateOk()

            return updated, nil
        }
    }
}

// drainCharCreateOk consumes the CharCreateOk packet that follows the
// updated character list of a successful creation. The packet always
// follows (the server writes it last), so a bounded wait drains it; a
// lost packet only costs the selection retry of EnterWorld.
func (gc *GameClient) drainCharCreateOk() {
    deadline := time.Now().Add(charCreateOkWait)
    if err := gc.conn.SetReadDeadline(deadline); err != nil {
        gc.logger.Printf("Failed to set the char create ok deadline: %v", err)

        return
    }

    for {
        payload, err := gc.readPacket(gc.readBuf)
        if err != nil {
            gc.readBuf = nil
            if !errors.Is(err, os.ErrDeadlineExceeded) {
                gc.logger.Printf("Failed to drain the char create ok: %v", err)

                return
            }
            gc.logger.Println("Char create ok not drained in time, " +
                "the selection retry will cover it")

            return
        }
        gc.readBuf = payload
        if len(payload) == 0 {
            continue
        }
        if payload[0] == charCreateOkID {
            gc.logger.Println("Character create confirmed")

            break
        }
    }

    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        gc.logger.Printf("Failed to reset the char create ok deadline: %v", err)
    }
}

// awaitCharacterCreation reads packets until the creation result resolves.
// The first return value reports completion, the second carries the updated
// character list when the requested character appeared in it. Every read
// bounds by charListWait: the creation exchange sits before the in-world
// silence watchdog exists.
func (gc *GameClient) awaitCharacterCreation(
    name string,
) (bool, *fromgameserver.CharSelectInfoPacket, error) {
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charListWait)); err != nil {
        return false, nil, fmt.Errorf(
            "failed to set the creation wait deadline: %w", err)
    }
    payload, err := gc.readPacket(gc.readBuf)
    if err != nil {
        return false, nil, fmt.Errorf("failed to read creation result: %w", err)
    }
    if err := gc.conn.SetReadDeadline(time.Time{}); err != nil {
        return false, nil, fmt.Errorf(
            "failed to reset the creation wait deadline: %w", err)
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
    if err := fromgameserver.ParseCharCreateFailPacket(fail,
        payload); err != nil {
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

    return false, nil, errors.New(
        "created character missing in the updated list")
}

// EnterWorld selects the character slot and requests world entry.
func (gc *GameClient) EnterWorld(slot int32) error {
    // The selection is retransmitted when the server drops it
    // silently (see the charSelectWait constants): the wait is
    // bounded, the select is resent and only repeated failures give
    // up (the reconnect supervisor then rebuilds the session).
    var selected fromgameserver.CharSelectedPacket
    for attempt := 1; attempt <= charSelectAttempts; attempt++ {
        if err := gc.sendPacket(
            &togameserver.CharacterSelect{CharSlot: slot}); err != nil {
            return fmt.Errorf("failed to send character select: %w", err)
        }

        answered, err := gc.awaitCharSelected(&selected)
        if err != nil {
            return err
        }
        if answered {
            break
        }
        if attempt == charSelectAttempts {
            return errors.New("the server did not answer the character " +
                "selection after " + strconv.Itoa(charSelectAttempts) +
                " attempts")
        }
        gc.logger.Printf("CharSelected did not arrive (attempt %d), "+
            "reselecting in %s", attempt, charSelectWait)
    }
    gc.logger.Println("Selected character " + selected.Name)

    if err := gc.sendPacket(&togameserver.EnterWorld{}); err != nil {
        return fmt.Errorf("failed to send enter world: %w", err)
    }
    gc.logger.Println("Sent enter world request")

    // The movement toggle of the official client entry: the server
    // starts every session walking (Creature._isRunning defaults to
    // false), and a bot that never flips it crosses the world at the
    // walk speed - every hunt run of the project moved at 97 instead
    // of ~170 units per second until the 2026-09-12 class transfer
    // rounds pinned it.
    run := togameserver.NewChangeMoveTypePacket()
    run.TypeRun = 1
    if err := gc.sendPacket(run); err != nil {
        return fmt.Errorf("failed to send run toggle: %w", err)
    }

    return nil
}

// awaitCharSelected waits for the CharSelected packet that allows
// entering the world, bounded by charSelectWait. It reports whether the
// answer arrived; unrelated packets are ignored like before.
func (gc *GameClient) awaitCharSelected(
    selected *fromgameserver.CharSelectedPacket,
) (bool, error) {
    if err := gc.conn.SetReadDeadline(
        time.Now().Add(charSelectWait)); err != nil {
        return false, fmt.Errorf(
            "failed to set the char select deadline: %w", err)
    }

    for {
        payload, err := gc.readPacket(gc.readBuf)
        if err != nil {
            gc.readBuf = nil
            if errors.Is(err, os.ErrDeadlineExceeded) {
                return false, nil
            }

            return false, fmt.Errorf(
                "failed to read character selected: %w", err)
        }
        gc.readBuf = payload
        if len(payload) == 0 {
            continue
        }
        if payload[0] == charSelectedID {
            if err := fromgameserver.ParseCharSelectedPacket(
                selected, payload); err != nil {
                return false, fmt.Errorf(
                    "failed to parse char selected: %w", err)
            }
            gc.trackerApplySelection(selected)

            if resetErr := gc.conn.SetReadDeadline(
                time.Time{}); resetErr != nil {
                gc.logger.Printf(
                    "Failed to reset the char select deadline: %v",
                    resetErr)
            }

            return true, nil
        }
        gc.logger.Printf("Ignoring packet id 0x%02x while entering world",
            payload[0])
    }
}

// trackerApplySelection feeds the selected character state to the tracker.
func (gc *GameClient) trackerApplySelection(
    selected *fromgameserver.CharSelectedPacket,
) {
    if gc.tracker == nil {
        return
    }
    gc.tracker.SetCharacter(selected.Name, selected.ObjectID, selected.ClassID,
        selected.X, selected.Y, selected.Z,
        selected.CurrentHP, selected.CurrentMP)
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
