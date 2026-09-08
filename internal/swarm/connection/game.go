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
	invItems       []state.InventoryItem
	statusAttrs    [statusAttrsCapacity]state.Attribute
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
		conn:           conn,
		crypt:          nil,
		writeMu:        sync.Mutex{},
		logger:         log.Default(),
		trace:          os.Getenv(packetTraceEnv) != "",
		packetCount:    atomic.Int64{},
		readBuf:        nil,
		tracker:        nil,
		tap:            nil,
		rawWriteBuf:    nil,
		npcInfo:        *fromgameserver.NewNpcInfoPacket(),
		userInfo:       *fromgameserver.NewUserInfoPacket(),
		charInfo:       *fromgameserver.NewCharInfoPacket(),
		moveTo:         *fromgameserver.NewMoveToLocationPacket(),
		moveToPawn:     *fromgameserver.NewMoveToPawnPacket(),
		stopMove:       *fromgameserver.NewStopMovePacket(),
		validateLoc:    *fromgameserver.NewValidateLocationPacket(),
		deleted:        *fromgameserver.NewDeleteObjectPacket(),
		dropItem:       *fromgameserver.NewDropItemPacket(),
		spawnItem:      *fromgameserver.NewSpawnItemPacket(),
		getItem:        *fromgameserver.NewGetItemPacket(),
		statusUpd:      *fromgameserver.NewStatusUpdatePacket(),
		attack:         *fromgameserver.NewAttackPacket(),
		attackStart:    *fromgameserver.NewAutoAttackStartPacket(),
		attackStop:     *fromgameserver.NewAutoAttackStopPacket(),
		beginRotation:  *fromgameserver.NewBeginRotationPacket(),
		stopRotation:   *fromgameserver.NewStopRotationPacket(),
		changeMoveType: *fromgameserver.NewChangeMoveTypePacket(),
		changeWait:     *fromgameserver.NewChangeWaitTypePacket(),
		teleport:       *fromgameserver.NewTeleportToLocationPacket(),
		myTarget:       *fromgameserver.NewMyTargetSelectedPacket(),
		targetSelected: *fromgameserver.NewTargetSelectedPacket(),
		targetDropped:  *fromgameserver.NewTargetUnselectedPacket(),
		systemMessage:  *fromgameserver.NewSystemMessagePacket(),
		socialAction:   *fromgameserver.NewSocialActionPacket(),
		actionFailed:   *fromgameserver.NewActionFailedPacket(),
		itemList:       *fromgameserver.NewItemListPacket(),
		invUpdate:      *fromgameserver.NewInventoryUpdatePacket(),
		invItems:       nil,
		statusAttrs:    [statusAttrsCapacity]state.Attribute{},
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

	gc.writeMu.Lock()
	defer gc.writeMu.Unlock()

	// The encryption transforms the buffer in place and the payload may
	// be backed by a shared proxy buffer, so it is copied into the
	// reusable outbound scratch first.
	wire := append(gc.rawWriteBuf[:0], payload...)
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
		return fmt.Errorf("failed to attack target %d: object unknown", objectID)
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
			gc.trackerApplySelection(&selected)

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

	err := gc.runLoop(ctx, packets, pingTicker, characterName)
	// The logout is only announced while the connection is still usable:
	// after a transport error the packet would fail and only spam the log.
	gc.disconnect(err == nil)

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
