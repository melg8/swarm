// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The shared render helpers and the opcode name tables of the run
// log decoders.

package botlog

import (
    "strconv"
    "strings"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// xyz renders a world position tuple.
func xyz(x int32, y int32, z int32) string {
    return "(" + strconv.Itoa(int(x)) + "," +
        strconv.Itoa(int(y)) + "," + strconv.Itoa(int(z)) + ")"
}

// shortLine renders the fallback of a packet whose body is shorter
// than its wire format demands.
func shortLine(name string) string {
    return name + " (short payload)"
}

// unknownLine renders the fallback of an unrecognized opcode: the
// size and up to 48 bytes of the hex tail (enough to eyeball the
// structure without flooding the file).
func unknownLine(side string, payload []byte) string {
    tail := payload
    if len(tail) > 48 {
        tail = tail[:48]
    }
    hex := make([]byte, 0, len(tail)*3)
    const digits = "0123456789abcdef"
    for i, b := range tail {
        if i > 0 {
            hex = append(hex, ' ')
        }
        hex = append(hex, digits[b>>4], digits[b&0x0f])
    }

    return "unknown " + side + " packet, " +
        strconv.Itoa(len(payload)) + " bytes: " + string(hex)
}

// skillName resolves a skill id into its catalog name.
func skillName(skillID int32) string {
    info, ok := npcdata.SkillInfoOf(skillID)
    if !ok || info.Name == "" {
        return "skill"
    }

    return info.Name
}

// recvOpcodeNames maps the server-to-client opcodes the session
// observes.
var recvOpcodeNames = map[byte]string{
    0x01: "MoveToLocation", 0x03: "CharInfo", 0x04: "UserInfo",
    0x06: "Attack", 0x0B: "Die", 0x15: "SpawnItem",
    0x16: "DropItem", 0x17: "GetItem", 0x1A: "StatusUpdate",
    0x1B: "NpcHtmlMessage", 0x1E: "DeleteObject",
    0x1F: "CharSelectInfo", 0x21: "CharSelected", 0x22: "NpcInfo",
    0x25: "CharCreateOk", 0x26: "CharCreateFail", 0x27: "ItemList",
    0x2E: "KeyPacket", 0x35: "ActionFailed", 0x36: "ServerClose",
    0x37: "InventoryUpdate", 0x38: "TeleportToLocation",
    0x39: "TargetSelected", 0x3A: "TargetUnselected",
    0x3B: "AutoAttackStart", 0x3C: "AutoAttackStop",
    0x3D: "SocialAction", 0x3E: "ChangeMoveType",
    0x3F: "ChangeWaitType", 0x59: "StopMove", 0x6D: "SkillList",
    0x75: "MoveToPawn", 0x76: "ValidateLocation",
    0x77: "BeginRotation", 0x78: "StopRotation",
    0x7A: "SystemMessage", 0x96: "LeaveWorld",
    0x97: "AbnormalStatusUpdate", 0x98: "QuestList",
    0xEC: "NetPing", 0xBF: "MyTargetSelected",
}

// sendOpcodeNames maps the client-to-server opcodes the session
// sends.
var sendOpcodeNames = map[byte]string{
    0x00: "ProtocolVersion", 0x01: "MoveToLocation",
    0x03: "EnterWorld", 0x04: "Action", 0x08: "AuthLogin",
    0x09: "Logout", 0x0B: "CharacterCreate",
    0x0D: "CharacterSelect", 0x0F: "RequestItemList",
    0x12: "RequestDropItem", 0x14: "RequestUseItem",
    0x1C: "ChangeMoveType2", 0x1E: "RequestSellItem",
    0x1F: "RequestBuyItem", 0x21: "RequestBypassToServer",
    0x2F: "RequestMagicSkillUse", 0x30: "Appearing",
    0x45: "RequestActionUse", 0x48: "ValidatePosition",
    0x59: "RequestDestroyItem", 0x6C: "RequestAcquireSkill",
    0x6D: "RequestRestartPoint", 0xA8: "RequestNetPing",
}

// trimText clamps a decoded string field for the log line.
func trimText(value string, limit int) string {
    if len(value) <= limit {
        return value
    }

    return value[:limit] + "..."
}

// quote renders a quoted log field.
func quote(value string) string {
    return "\"" + strings.ReplaceAll(value, "\"", "'") + "\""
}
