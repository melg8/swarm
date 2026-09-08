// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package connection

// The packet dispatch layer of the game session, split out of the
// game.go god file: the opcode routing, the unknown packet log and
// the net ping keepalive. Every handler receives the raw payload
// and parses it into the reusable scratch structs of the client
// (see GameClient) before applying it to the tracker.

import (
	"fmt"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
)

// handleServerPacket logs and dispatches known in game packets. An
// empty payload is the zero length frame the read loop reports as
// no packet: skip it instead of indexing the opcode.
func (gc *GameClient) handleServerPacket(payload []byte) {
	if len(payload) == 0 {
		return
	}
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
		message := state.SystemMessage{
			ID:     gc.systemMessage.MessageID,
			Params: nil,
		}
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
