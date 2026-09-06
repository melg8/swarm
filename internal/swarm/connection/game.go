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
func NewGameClient(conn net.Conn) (*GameClient, error) {
	client := &GameClient{
		conn:           conn,
		crypt:          nil,
		writeMu:        sync.Mutex{},
		logger:         log.Default(),
		trace:          os.Getenv(packetTraceEnv) != "",
		packetCount:    atomic.Int64{},
		readBuf:        nil,
		tracker:        nil,
		npcInfo:        *fromgameserver.NewNpcInfoPacket(),
		userInfo:       *fromgameserver.NewUserInfoPacket(),
		charInfo:       *fromgameserver.NewCharInfoPacket(),
		moveTo:         *fromgameserver.NewMoveToLocationPacket(),
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

// handleServerPacket logs and dispatches known in game packets.
func (gc *GameClient) handleServerPacket(payload []byte) {
	if gc.tracker != nil {
		gc.tracker.CountPacket()
	}
	if gc.trace {
		gc.logger.Printf("Trace packet id 0x%02x with %d bytes",
			payload[0], len(payload))
	}

	switch payload[0] {
	case netPingResponseID:
		gc.handleNetPing(payload)
	case leaveWorldID:
		gc.logger.Println("Server confirmed leave world")
	case serverCloseID:
		gc.logger.Println("Server is closing the connection")
	case systemMessageID:
		gc.applySystemMessage(payload)
	case socialActionID:
		gc.applySocialAction(payload)
	case actionFailedID:
		gc.applyActionFailed(payload)
	default:
		gc.handleWorldPacket(payload)
	}
}

// applyActionFailed validates and logs the refusal answer of the
// server. The packet carries no details, so the hunt loop keeps
// driving its own retry logic without reacting to it.
func (gc *GameClient) applyActionFailed(payload []byte) {
	if err := fromgameserver.ParseActionFailedPacket(
		&gc.actionFailed, payload); err != nil {
		gc.logger.Printf("Failed to parse action failed: %v", err)

		return
	}
	gc.logger.Println("Action failed")
}

// applySystemMessage parses SystemMessage and forwards the formatted
// chat line to the tracker.
func (gc *GameClient) applySystemMessage(payload []byte) {
	if err := fromgameserver.ParseSystemMessagePacket(
		&gc.systemMessage, payload); err != nil {
		gc.logger.Printf("Failed to parse system message: %v", err)

		return
	}
	if gc.tracker != nil {
		message := state.SystemMessage{ID: gc.systemMessage.MessageID}
		for _, param := range gc.systemMessage.Params {
			message.Params = append(message.Params, state.ChatMessageParam{
				Type: param.Type,
				Int:  param.Int,
				Text: param.Text,
			})
		}
		gc.tracker.ApplySystemMessage(message)
	}
}

// applySocialAction parses SocialAction and forwards it to the tracker.
func (gc *GameClient) applySocialAction(payload []byte) {
	if err := fromgameserver.ParseSocialActionPacket(
		&gc.socialAction, payload); err != nil {
		gc.logger.Printf("Failed to parse social action: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplySocialAction(state.SocialAction{
			ObjectID: gc.socialAction.ObjectID,
			ActionID: gc.socialAction.ActionID,
		})
	}
}

// handleWorldPacket dispatches the packets that carry the observed world
// state: characters, npcs, items, movement and combat.
func (gc *GameClient) handleWorldPacket(payload []byte) {
	if gc.handleObjectPacket(payload) {
		return
	}
	if gc.handleCombatPacket(payload) {
		return
	}
	if gc.handlePlacementPacket(payload) {
		return
	}
	if gc.handleRotationPacket(payload) {
		return
	}
	gc.handleInventoryPacket(payload)
}

// handleObjectPacket dispatches the spawn and remove packets. It reports
// whether the packet was consumed.
func (gc *GameClient) handleObjectPacket(payload []byte) bool {
	switch payload[0] {
	case userInfoID:
		gc.applyUserInfo(payload)
	case charInfoID:
		gc.applyCharInfo(payload)
	case npcInfoID:
		gc.applyNpcInfo(payload)
	case spawnItemID:
		gc.applySpawnItem(payload)
	case dropItemID:
		gc.applyDropItem(payload)
	case getItemID:
		gc.applyGetItem(payload)
	case deleteObjectID:
		gc.applyDeleteObject(payload)
	default:
		return false
	}

	return true
}

// handlePlacementPacket dispatches movement and vitals packets. It
// reports whether the packet was consumed.
func (gc *GameClient) handlePlacementPacket(payload []byte) bool {
	switch payload[0] {
	case moveToLocationID:
		gc.applyMoveToLocation(payload)
	case moveToPawnID:
		gc.applyMoveToPawn(payload)
	case stopMoveID:
		gc.applyStopMove(payload)
	case validateLocationID:
		gc.applyValidateLocation(payload)
	case statusUpdateID:
		gc.applyStatusUpdate(payload)
	default:
		return false
	}

	return true
}

// handleRotationPacket dispatches the turn, movement mode and wait type
// packets. It reports whether the packet was consumed.
func (gc *GameClient) handleRotationPacket(payload []byte) bool {
	switch payload[0] {
	case beginRotationID:
		gc.applyBeginRotation(payload)
	case stopRotationID:
		gc.applyStopRotation(payload)
	case changeMoveTypeID:
		gc.applyChangeMoveType(payload)
	case changeWaitTypeID:
		gc.applyChangeWaitType(payload)
	case teleportID:
		gc.applyTeleport(payload)
	default:
		return false
	}

	return true
}

// handleCombatPacket dispatches the combat and target packets. It
// reports whether the packet was consumed.
func (gc *GameClient) handleCombatPacket(payload []byte) bool {
	switch payload[0] {
	case attackID:
		gc.applyAttack(payload)
	case autoAttackStartID:
		gc.applyAutoAttackStart(payload)
	case autoAttackStopID:
		gc.applyAutoAttackStop(payload)
	case myTargetSelectedID:
		gc.applyMyTargetSelected(payload)
	case targetSelectedID:
		gc.applyTargetSelected(payload)
	case targetUnselectedID:
		gc.applyTargetUnselected(payload)
	default:
		return false
	}

	return true
}

// handleInventoryPacket dispatches the inventory packets.
func (gc *GameClient) handleInventoryPacket(payload []byte) {
	switch payload[0] {
	case itemListID:
		gc.applyItemList(payload)
	case inventoryUpdateID:
		gc.applyInventoryUpdate(payload)
	default:
		gc.logUnknownPacket(payload)
	}
}

// logUnknownPacket reports an unobserved packet to the console and the
// tracker event log.
func (gc *GameClient) logUnknownPacket(payload []byte) {
	gc.logger.Printf("Received packet id 0x%02x with %d bytes",
		payload[0], len(payload))
	if gc.tracker != nil {
		gc.tracker.RecordEvent(fmt.Sprintf(
			"packet 0x%02x with %d bytes", payload[0], len(payload)))
	}
}

// handleNetPing parses and logs the server net ping response.
func (gc *GameClient) handleNetPing(payload []byte) {
	ping := fromgameserver.NewNetPingPacket()
	if err := fromgameserver.ParseNetPingPacket(ping, payload); err != nil {
		gc.logger.Printf("Failed to parse net ping: %v", err)

		return
	}
	gc.logger.Printf("Net ping with game time %d", ping.GameTime)
}

// applyUserInfo parses UserInfo and updates the character state.
func (gc *GameClient) applyUserInfo(payload []byte) {
	err := fromgameserver.ParseUserInfoPacket(&gc.userInfo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse user info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.userInfo
		gc.tracker.ApplyUserInfo(state.UserInfo{
			Name:          info.Name,
			Level:         info.Level,
			Race:          info.Race,
			ClassID:       info.ClassID,
			X:             info.X,
			Y:             info.Y,
			Z:             info.Z,
			STR:           info.STR,
			DEX:           info.DEX,
			CON:           info.CON,
			INT:           info.INT,
			WIT:           info.WIT,
			MEN:           info.MEN,
			Exp:           info.Exp,
			Sp:            info.Sp,
			MaxHP:         info.MaxHP,
			CurHP:         info.CurHP,
			MaxMP:         info.MaxMP,
			CurMP:         info.CurMP,
			CurrentLoad:   info.CurrentLoad,
			MaxLoad:       info.MaxLoad,
			RunSpeed:      info.RunSpeed,
			WalkSpeed:     info.WalkSpeed,
			MoveSpeedMult: info.MoveSpeedMult,
		})
	}
	gc.logger.Printf("User info: %s level %d hp %d/%d mp %d/%d speed %d x %d",
		gc.userInfo.Name, gc.userInfo.Level,
		gc.userInfo.CurHP, gc.userInfo.MaxHP,
		gc.userInfo.CurMP, gc.userInfo.MaxMP, gc.userInfo.RunSpeed,
		gc.userInfo.CurrentLoad)
}

// applyNpcInfo parses NpcInfo and upserts the npc object.
func (gc *GameClient) applyNpcInfo(payload []byte) {
	if err := fromgameserver.ParseNpcInfoPacket(&gc.npcInfo, payload); err != nil {
		gc.logger.Printf("Failed to parse npc info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.npcInfo
		gc.tracker.ApplyNpcInfo(state.NpcInfo{
			ObjectID:        info.ObjectID,
			TemplateID:      info.TemplateID,
			Attackable:      info.Attackable,
			X:               info.X,
			Y:               info.Y,
			Z:               info.Z,
			Heading:         info.Heading,
			RunSpeed:        info.RunSpeed,
			WalkSpeed:       info.WalkSpeed,
			MoveSpeedMult:   info.MoveSpeedMult,
			CollisionRadius: info.CollisionRadius,
			Running:         info.Running,
			InCombat:        info.InCombat,
			Dead:            info.Dead,
			Name:            info.Name,
			Title:           info.Title,
		})
	}
	gc.logger.Printf("NPC %s spawned at %d %d %d heading %d",
		gc.npcInfo.Name, gc.npcInfo.X, gc.npcInfo.Y, gc.npcInfo.Z,
		gc.npcInfo.Heading)
}

// applyCharInfo parses CharInfo and upserts the player object.
func (gc *GameClient) applyCharInfo(payload []byte) {
	err := fromgameserver.ParseCharInfoPacket(&gc.charInfo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse char info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.charInfo
		gc.tracker.ApplyPlayerInfo(state.PlayerInfo{
			ObjectID:        info.ObjectID,
			Name:            info.Name,
			Title:           info.Title,
			Race:            info.Race,
			ClassID:         info.ClassID,
			RunSpeed:        info.RunSpeed,
			WalkSpeed:       info.WalkSpeed,
			MoveSpeedMult:   info.MoveSpeedMult,
			CollisionRadius: info.CollisionRadius,
			Running:         info.Running,
			InCombat:        info.InCombat,
			Dead:            info.Dead,
			X:               info.X,
			Y:               info.Y,
			Z:               info.Z,
		})
	}
	gc.logger.Printf("Player %s appeared at %d %d %d speed %d",
		gc.charInfo.Name, gc.charInfo.X, gc.charInfo.Y, gc.charInfo.Z,
		gc.charInfo.RunSpeed)
}

// applyDropItem parses DropItem and upserts the ground item object.
func (gc *GameClient) applyDropItem(payload []byte) {
	err := fromgameserver.ParseDropItemPacket(&gc.dropItem, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse drop item: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.dropItem
		gc.tracker.ApplyItemInfo(state.ItemInfo{
			ObjectID:   info.ObjectID,
			TemplateID: info.TemplateID,
			Stackable:  info.Stackable,
			Count:      info.Count,
			X:          info.X,
			Y:          info.Y,
			Z:          info.Z,
		})
	}
}

// applyDeleteObject parses DeleteObject and removes the object.
func (gc *GameClient) applyDeleteObject(payload []byte) {
	err := fromgameserver.ParseDeleteObjectPacket(&gc.deleted, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse delete object: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.RemoveObject(gc.deleted.ObjectID)
	}
}

// applyMoveToLocation parses MoveToLocation and updates the movement.
func (gc *GameClient) applyMoveToLocation(payload []byte) {
	err := fromgameserver.ParseMoveToLocationPacket(&gc.moveTo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse movement: %v", err)

		return
	}
	if gc.tracker != nil {
		move := gc.moveTo
		gc.tracker.ApplyMovement(state.Movement{
			ObjectID: move.ObjectID,
			X:        move.X,
			Y:        move.Y,
			Z:        move.Z,
			DestX:    move.DestX,
			DestY:    move.DestY,
			DestZ:    move.DestZ,
		})
	}
}

// applyStopMove parses StopMove and updates the object placement.
func (gc *GameClient) applyStopMove(payload []byte) {
	err := fromgameserver.ParseStopMovePacket(&gc.stopMove, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse stop move: %v", err)

		return
	}
	if gc.tracker != nil {
		stop := gc.stopMove
		gc.tracker.ApplyPlacement(state.Placement{
			ObjectID: stop.ObjectID,
			X:        stop.X,
			Y:        stop.Y,
			Z:        stop.Z,
			Heading:  stop.Heading,
			Moving:   false,
		})
	}
}

// applyValidateLocation parses ValidateLocation and updates the placement.
func (gc *GameClient) applyValidateLocation(payload []byte) {
	if err := fromgameserver.ParseValidateLocationPacket(
		&gc.validateLoc, payload); err != nil {
		gc.logger.Printf("Failed to parse validate location: %v", err)

		return
	}
	if gc.tracker != nil {
		place := gc.validateLoc
		gc.tracker.ApplyPlacement(state.Placement{
			ObjectID: place.ObjectID,
			X:        place.X,
			Y:        place.Y,
			Z:        place.Z,
			Heading:  place.Heading,
			Moving:   false,
		})
	}
}

// applyStatusUpdate parses StatusUpdate and applies the vitals changes.
func (gc *GameClient) applyStatusUpdate(payload []byte) {
	if err := fromgameserver.ParseStatusUpdatePacket(
		&gc.statusUpd, payload); err != nil {
		gc.logger.Printf("Failed to parse status update: %v", err)

		return
	}
	if gc.tracker != nil {
		attrs := gc.statusAttrs[:0]
		gc.statusUpd.ForEach(func(id int32, value int32) {
			attrs = append(attrs, state.Attribute{ID: id, Value: value})
		})
		gc.tracker.ApplyStatusUpdate(gc.statusUpd.ObjectID, attrs)
	}
}

// applyAttack parses Attack and updates the combat state.
func (gc *GameClient) applyAttack(payload []byte) {
	if err := fromgameserver.ParseAttackPacket(&gc.attack, payload); err != nil {
		gc.logger.Printf("Failed to parse attack: %v", err)

		return
	}
	if gc.tracker != nil {
		attack := state.Attack{
			AttackerID:  gc.attack.AttackerID,
			X:           gc.attack.X,
			Y:           gc.attack.Y,
			Z:           gc.attack.Z,
			TargetX:     gc.attack.TargetX,
			TargetY:     gc.attack.TargetY,
			TargetZ:     gc.attack.TargetZ,
			TargetIDs:   [4]int32{},
			TargetCount: gc.attack.HitCount,
		}
		for i := range min(gc.attack.HitCount, state.AttackTargets) {
			attack.TargetIDs[i] = gc.attack.Hits[i].TargetID
		}
		gc.tracker.ApplyAttack(attack)
	}
}

// applyAutoAttackStart parses AutoAttackStart and marks combat.
func (gc *GameClient) applyAutoAttackStart(payload []byte) {
	if err := fromgameserver.ParseAutoAttackStartPacket(
		&gc.attackStart, payload); err != nil {
		gc.logger.Printf("Failed to parse auto attack start: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyAutoAttackStart(gc.attackStart.ObjectID)
	}
}

// applyAutoAttackStop parses AutoAttackStop and clears combat.
func (gc *GameClient) applyAutoAttackStop(payload []byte) {
	if err := fromgameserver.ParseAutoAttackStopPacket(
		&gc.attackStop, payload); err != nil {
		gc.logger.Printf("Failed to parse auto attack stop: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyAutoAttackStop(gc.attackStop.ObjectID)
	}
}

// applyMoveToPawn parses MoveToPawn and updates the chasing object.
func (gc *GameClient) applyMoveToPawn(payload []byte) {
	if err := fromgameserver.ParseMoveToPawnPacket(
		&gc.moveToPawn, payload); err != nil {
		gc.logger.Printf("Failed to parse move to pawn: %v", err)

		return
	}
	if gc.tracker != nil {
		move := gc.moveToPawn
		gc.tracker.ApplyPawnMovement(state.PawnMovement{
			ObjectID: move.ObjectID,
			TargetID: move.TargetID,
			Distance: move.Distance,
			X:        move.X,
			Y:        move.Y,
			Z:        move.Z,
			TargetX:  move.TargetX,
			TargetY:  move.TargetY,
			TargetZ:  move.TargetZ,
		})
	}
}

// applySpawnItem parses SpawnItem and upserts a ground item that already
// existed around the character.
func (gc *GameClient) applySpawnItem(payload []byte) {
	if err := fromgameserver.ParseSpawnItemPacket(
		&gc.spawnItem, payload); err != nil {
		gc.logger.Printf("Failed to parse spawn item: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.spawnItem
		gc.tracker.ApplySpawnItem(state.ItemInfo{
			ObjectID:   info.ObjectID,
			TemplateID: info.TemplateID,
			Stackable:  info.Stackable,
			Count:      info.Count,
			X:          info.X,
			Y:          info.Y,
			Z:          info.Z,
		})
	}
}

// applyGetItem parses GetItem and removes the picked up ground item.
func (gc *GameClient) applyGetItem(payload []byte) {
	if err := fromgameserver.ParseGetItemPacket(&gc.getItem, payload); err != nil {
		gc.logger.Printf("Failed to parse get item: %v", err)

		return
	}
	if gc.tracker != nil {
		item := gc.getItem
		gc.tracker.ApplyItemPickup(state.ItemPickup{
			PlayerID: item.PlayerID,
			ObjectID: item.ObjectID,
			X:        item.X,
			Y:        item.Y,
			Z:        item.Z,
		})
	}
}

// applyBeginRotation parses BeginRotation and turns the object.
func (gc *GameClient) applyBeginRotation(payload []byte) {
	if err := fromgameserver.ParseBeginRotationPacket(
		&gc.beginRotation, payload); err != nil {
		gc.logger.Printf("Failed to parse begin rotation: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyRotationStart(state.Rotation{
			ObjectID: gc.beginRotation.ObjectID,
			Heading:  gc.beginRotation.Heading,
		})
	}
}

// applyStopRotation parses StopRotation and turns the object.
func (gc *GameClient) applyStopRotation(payload []byte) {
	if err := fromgameserver.ParseStopRotationPacket(
		&gc.stopRotation, payload); err != nil {
		gc.logger.Printf("Failed to parse stop rotation: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyRotationStop(state.Rotation{
			ObjectID: gc.stopRotation.ObjectID,
			Heading:  gc.stopRotation.Heading,
		})
	}
}

// applyChangeMoveType parses ChangeMoveType and updates the run state.
func (gc *GameClient) applyChangeMoveType(payload []byte) {
	if err := fromgameserver.ParseChangeMoveTypePacket(
		&gc.changeMoveType, payload); err != nil {
		gc.logger.Printf("Failed to parse change move type: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyMoveType(state.MoveType{
			ObjectID: gc.changeMoveType.ObjectID,
			Running:  gc.changeMoveType.Running,
		})
	}
}

// applyChangeWaitType parses ChangeWaitType and tracks the sit state.
func (gc *GameClient) applyChangeWaitType(payload []byte) {
	if err := fromgameserver.ParseChangeWaitTypePacket(
		&gc.changeWait, payload); err != nil {
		gc.logger.Printf("Failed to parse change wait type: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyWaitType(state.WaitType{
			ObjectID: gc.changeWait.ObjectID,
			Sitting:  gc.changeWait.Sitting,
		})
	}
}

// applyTeleport parses TeleportToLocation and snaps the object.
func (gc *GameClient) applyTeleport(payload []byte) {
	if err := fromgameserver.ParseTeleportToLocationPacket(
		&gc.teleport, payload); err != nil {
		gc.logger.Printf("Failed to parse teleport: %v", err)

		return
	}
	if gc.tracker != nil {
		tele := gc.teleport
		gc.tracker.ApplyTeleport(state.Teleport{
			ObjectID: tele.ObjectID,
			X:        tele.X,
			Y:        tele.Y,
			Z:        tele.Z,
			Heading:  tele.Heading,
		})
	}
}

// applyMyTargetSelected parses MyTargetSelected and records the own
// target of the bot.
func (gc *GameClient) applyMyTargetSelected(payload []byte) {
	if err := fromgameserver.ParseMyTargetSelectedPacket(
		&gc.myTarget, payload); err != nil {
		gc.logger.Printf("Failed to parse my target selected: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplySelfTarget(gc.myTarget.ObjectID)
	}
}

// applyTargetSelected parses TargetSelected and records the target of
// another player.
func (gc *GameClient) applyTargetSelected(payload []byte) {
	if err := fromgameserver.ParseTargetSelectedPacket(
		&gc.targetSelected, payload); err != nil {
		gc.logger.Printf("Failed to parse target selected: %v", err)

		return
	}
	if gc.tracker != nil {
		selected := gc.targetSelected
		gc.tracker.ApplyObjectTarget(selected.ObjectID, selected.TargetID)
	}
}

// applyTargetUnselected parses TargetUnselected and clears the target
// reference of another player.
func (gc *GameClient) applyTargetUnselected(payload []byte) {
	if err := fromgameserver.ParseTargetUnselectedPacket(
		&gc.targetDropped, payload); err != nil {
		gc.logger.Printf("Failed to parse target unselected: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyTargetClear(gc.targetDropped.ObjectID)
	}
}

// applyItemList parses ItemList and replaces the tracked inventory.
func (gc *GameClient) applyItemList(payload []byte) {
	err := fromgameserver.ParseItemListPacket(
		&gc.itemList, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse item list: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	items := gc.convertInventoryItems(gc.itemList.Items)
	gc.tracker.ApplyItemList(items)
	gc.logger.Printf("Inventory listed with %d items", len(items))
}

// applyInventoryUpdate parses InventoryUpdate and applies the changes.
func (gc *GameClient) applyInventoryUpdate(payload []byte) {
	if err := fromgameserver.ParseInventoryUpdatePacket(
		&gc.invUpdate, payload); err != nil {
		gc.logger.Printf("Failed to parse inventory update: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	items := gc.convertInventoryItems(gc.invUpdate.Items)
	gc.tracker.ApplyInventoryUpdate(items)
}

// convertInventoryItems copies the parsed items into state items through
// the reused conversion buffer. The state layer copies the item values
// into its own inventory map, so the buffer is not retained.
func (gc *GameClient) convertInventoryItems(
	source []fromgameserver.InventoryItem,
) []state.InventoryItem {
	gc.invItems = gc.invItems[:0]
	for _, item := range source {
		gc.invItems = append(gc.invItems, state.InventoryItem{
			ObjectID: item.ObjectID,
			ItemID:   item.ItemID,
			Count:    item.Count,
			Type1:    item.Type1,
			Type2:    item.Type2,
			Equipped: item.Equipped,
			Change:   item.Change,
		})
	}

	return gc.invItems
}
