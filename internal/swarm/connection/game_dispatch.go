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
    "time"

    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
)

// handleServerPacket accounts and dispatches one inbound game packet.
// An empty payload is the zero length frame the read loop reports as
// no packet: skip it instead of indexing the opcode. An unregistered
// id keeps the unknown packet log.
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
    handler := packetHandlers[payload[0]]
    if handler == nil {
        gc.logUnknownPacket(payload)

        return
    }
    handler(gc, payload)
}

// packetHandler handles one inbound packet payload of the session.
type packetHandler func(gc *GameClient, payload []byte)

// packetRoutes is the declarative dispatch table of the session: one
// line per handled opcode, grouped by the packet family. The table is
// the single place a new inbound packet registers; buildPacketHandlers
// rejects a duplicate id at startup.
var packetRoutes = []struct {
    id     byte
    handle packetHandler
}{
    // The session flow and the keepalive.
    {fromgameserver.NetPingPacketID, (*GameClient).handleNetPing},
    {leaveWorldID, (*GameClient).handleLeaveWorld},
    {serverCloseID, (*GameClient).handleServerClose},
    {fromgameserver.SkillListPacketID, (*GameClient).applySkillList},
    {fromgameserver.QuestListPacketID, (*GameClient).applyQuestList},
    {fromgameserver.AbnormalStatusUpdatePacketID,
        (*GameClient).applyAbnormalStatusUpdate},
    {fromgameserver.NpcHTMLMessagePacketID, (*GameClient).applyNpcHTMLMessage},
    // The chat and the action feedback.
    {fromgameserver.SystemMessagePacketID, (*GameClient).applySystemMessage},
    {fromgameserver.CreatureSayPacketID, (*GameClient).applyCreatureSay},
    {fromgameserver.SocialActionPacketID, (*GameClient).applySocialAction},
    {fromgameserver.ActionFailedPacketID, (*GameClient).applyActionFailed},
    // The observed world objects.
    {fromgameserver.UserInfoPacketID, (*GameClient).applyUserInfo},
    {fromgameserver.CharInfoPacketID, (*GameClient).applyCharInfo},
    {fromgameserver.NpcInfoPacketID, (*GameClient).applyNpcInfo},
    {fromgameserver.SpawnItemPacketID, (*GameClient).applySpawnItem},
    {fromgameserver.DropItemPacketID, (*GameClient).applyDropItem},
    {fromgameserver.GetItemPacketID, (*GameClient).applyGetItem},
    {fromgameserver.DeleteObjectPacketID, (*GameClient).applyDeleteObject},
    // The movement and the vitals.
    {fromgameserver.MoveToLocationPacketID, (*GameClient).applyMoveToLocation},
    {fromgameserver.MoveToPawnPacketID, (*GameClient).applyMoveToPawn},
    {fromgameserver.StopMovePacketID, (*GameClient).applyStopMove},
    {fromgameserver.ValidateLocationPacketID,
        (*GameClient).applyValidateLocation},
    {fromgameserver.StatusUpdatePacketID, (*GameClient).applyStatusUpdate},
    {fromgameserver.SetupGaugePacketID, (*GameClient).applySetupGauge},
    // The turn, the movement mode and the wait type.
    {fromgameserver.BeginRotationPacketID, (*GameClient).applyBeginRotation},
    {fromgameserver.StopRotationPacketID, (*GameClient).applyStopRotation},
    {fromgameserver.ChangeMoveTypePacketID, (*GameClient).applyChangeMoveType},
    {fromgameserver.ChangeWaitTypePacketID, (*GameClient).applyChangeWaitType},
    {fromgameserver.TeleportToLocationPacketID, (*GameClient).applyTeleport},
    // The combat and the target selection.
    {fromgameserver.AttackPacketID, (*GameClient).applyAttack},
    {fromgameserver.AutoAttackStartPacketID,
        (*GameClient).applyAutoAttackStart},
    {fromgameserver.AutoAttackStopPacketID, (*GameClient).applyAutoAttackStop},
    {fromgameserver.MyTargetSelectedPacketID,
        (*GameClient).applyMyTargetSelected},
    {fromgameserver.MagicSkillUsePacketID, (*GameClient).applyMagicSkillUse},
    {fromgameserver.TargetSelectedPacketID, (*GameClient).applyTargetSelected},
    {fromgameserver.TargetUnselectedPacketID,
        (*GameClient).applyTargetUnselected},
    // The inventory.
    {fromgameserver.ItemListPacketID, (*GameClient).applyItemList},
    {fromgameserver.InventoryUpdatePacketID,
        (*GameClient).applyInventoryUpdate},
}

// packetHandlers is the indexed form of packetRoutes: one array read
// per packet.
var packetHandlers = buildPacketHandlers()

// buildPacketHandlers indexes packetRoutes and rejects a duplicate
// opcode: the second registration would be unreachable and the
// compiler cannot see it.
func buildPacketHandlers() [256]packetHandler {
    var table [256]packetHandler
    for _, route := range packetRoutes {
        if table[route.id] != nil {
            panic(fmt.Sprintf(
                "connection: duplicate packet handler for 0x%02x", route.id))
        }
        table[route.id] = route.handle
    }

    return table
}

// handleLeaveWorld logs the server confirmation that the character
// left the world.
func (gc *GameClient) handleLeaveWorld([]byte) {
    gc.logger.Println("Server confirmed leave world")
}

// handleServerClose logs the server shutdown notification.
func (gc *GameClient) handleServerClose([]byte) {
    gc.logger.Println("Server is closing the connection")
}

// applyActionFailed validates the refusal answer of the server and
// hands its arrival time to the state tracker: the hunt walk
// machinery reads it as the online refusal evidence - a walk click
// that got the answer while the character stood still is a server
// side refusal the offline click validation cannot see (see
// hunt.Loop.refusalEvidence). The packet itself carries no details
// about which request failed, so the loop keeps driving its own
// retry logic around the timestamp.
func (gc *GameClient) applyActionFailed(payload []byte) {
    if err := fromgameserver.ParseActionFailedPacket(
        &gc.actionFailed, payload); err != nil {
        gc.logger.Printf("Failed to parse action failed: %v", err)

        return
    }
    gc.logger.Println("Action failed")
    if gc.tracker != nil {
        gc.tracker.ApplyActionFailed(time.Now())
    }
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
                Type:  param.Type,
                Int:   param.Int,
                Level: param.Level,
                Text:  param.Text,
            })
        }
        gc.tracker.ApplySystemMessage(message)
    }
}

// applyCreatureSay parses CreatureSay and forwards the world chat
// line to the tracker chat window.
func (gc *GameClient) applyCreatureSay(payload []byte) {
    if err := fromgameserver.ParseCreatureSayPacket(
        &gc.creatureSay, payload); err != nil {
        gc.logger.Printf("Failed to parse creature say: %v", err)

        return
    }
    if gc.tracker != nil {
        gc.tracker.ApplySay(state.Say{
            ObjectID: gc.creatureSay.ObjectID,
            Channel:  gc.creatureSay.Channel,
            From:     gc.creatureSay.From,
            Text:     gc.creatureSay.Text,
        })
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

// applyMagicSkillUse parses MagicSkillUse and forwards the cast to
// the tracker: the played character's casts open the cast and reuse
// windows the web view renders.
func (gc *GameClient) applyMagicSkillUse(payload []byte) {
    if err := fromgameserver.ParseMagicSkillUsePacket(
        &gc.magicSkillUse, payload); err != nil {
        gc.logger.Printf("Failed to parse magic skill use: %v", err)

        return
    }
    if gc.tracker != nil {
        gc.tracker.ApplySkillCast(state.SkillCast{
            CasterID:     gc.magicSkillUse.CasterID,
            TargetID:     gc.magicSkillUse.TargetID,
            SkillID:      gc.magicSkillUse.SkillID,
            SkillLevel:   gc.magicSkillUse.SkillLevel,
            HitTimeMs:    gc.magicSkillUse.HitTime,
            ReuseDelayMs: gc.magicSkillUse.ReuseDelay,
        })
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

// handleNetPing validates the server net ping response and stays
// silent on the happy path. The reply itself is only the proof the
// connection is alive (the game time it carries is used nowhere), and
// a C1 client behind the proxy pings continuously - the client's own
// RequestNetPing (0xA8) packets are relayed through this bot session,
// so the server answers at the client's ping rate, several replies
// per second observed live. One log line per answer flooded the
// process log. A malformed packet still logs: it flags a cipher or
// protocol desync worth noticing.
func (gc *GameClient) handleNetPing(payload []byte) {
    ping := fromgameserver.NewNetPingPacket()
    if err := fromgameserver.ParseNetPingPacket(ping, payload); err != nil {
        gc.logger.Printf("Failed to parse net ping: %v", err)

        return
    }
}
