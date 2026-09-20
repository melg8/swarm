// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The client-to-server packet decoder of the run log: one readable
// line (plus continuation lines for the batch packets) per packet
// the session sends. The decode reads the serialized plaintext with
// a bounds-checked cursor - a malformed or short payload falls back
// to the size and the hex tail instead of panicking on the session
// goroutine.

package botlog

import (
    "strconv"
    "strings"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// cursor is a bounds-checked reader over one serialized packet.
type cursor struct {
    payload []byte
    pos     int
}

// int32 reads one little endian int32 field.
func (c *cursor) int32() (int32, bool) {
    if c.pos+4 > len(c.payload) {
        return 0, false
    }
    value := int32(uint32(c.payload[c.pos]) |
        uint32(c.payload[c.pos+1])<<8 |
        uint32(c.payload[c.pos+2])<<16 |
        uint32(c.payload[c.pos+3])<<24)
    c.pos += 4

    return value, true
}

// byteVal reads one byte field (zero past the payload end).
func (c *cursor) byteVal() byte {
    if c.pos >= len(c.payload) {
        return 0
    }
    value := c.payload[c.pos]
    c.pos++

    return value
}

// utf16 reads one null-terminated UTF-16LE string.
func (c *cursor) utf16() (string, bool) {
    var out strings.Builder
    for c.pos+1 < len(c.payload) {
        unit := uint16(c.payload[c.pos]) |
            uint16(c.payload[c.pos+1])<<8
        c.pos += 2
        if unit == 0 {
            return out.String(), true
        }
        out.WriteRune(rune(unit))
    }

    return out.String(), false
}

// shiftFlagSuffix is the shift marker of the request lines.
const shiftFlagSuffix = " shift"

// decodeSend renders one client packet into its log lines. The
// first line carries the fields, the batch packets (the buy and
// sell requests) append one indented line per entry. The session
// family answers directly, the world and inventory families ride
// their dispatchers.
func decodeSend(payload []byte) []string {
    if len(payload) == 0 {
        return nil
    }
    c := &cursor{payload: payload, pos: 1}
    switch payload[0] {
    case 0x00:
        return []string{"protocol version " + strconv.Itoa(
            int(firstInt32(payload)))}
    case 0x03:
        return []string{"enter world"}
    case 0x09:
        return []string{"logout"}
    case 0x0F:
        return []string{"request item list"}
    case 0x30:
        return []string{"appearing"}
    case 0xA8:
        return []string{"net ping"}
    }
    if lines, ok := decodeSendWorld(c, payload); ok {
        return lines
    }
    if lines, ok := decodeSendInventory(c, payload); ok {
        return lines
    }

    return []string{unknownLine("send", payload)}
}

// decodeSendWorld dispatches the movement, target and skill packets.
func decodeSendWorld(c *cursor, payload []byte) ([]string, bool) {
    switch payload[0] {
    case 0x01:
        return decodeSendMove(c), true
    case 0x04:
        return decodeSendAction(c), true
    case 0x2F:
        return decodeSendMagicSkillUse(c), true
    case 0x45:
        return decodeSendActionUse(c), true
    case 0x48:
        return decodeSendValidatePosition(c), true
    case 0x6D:
        return decodeSendRestartPoint(c), true
    default:
        return nil, false
    }
}

// decodeSendInventory dispatches the account and item packets.
func decodeSendInventory(c *cursor, payload []byte) ([]string, bool) {
    switch payload[0] {
    case 0x08:
        return decodeSendAuthLogin(c), true
    case 0x0B:
        return decodeSendCharCreate(c), true
    case 0x0D:
        return decodeSendCharSelect(c), true
    case 0x12:
        return decodeSendDropItem(c), true
    case 0x14:
        return decodeSendUseItem(c), true
    case 0x1C:
        return decodeSendChangeMoveType(c), true
    case 0x1E:
        return decodeSendSellItem(c), true
    case 0x1F:
        return decodeSendBuyItem(c), true
    case 0x21:
        return decodeSendBypass(c), true
    case 0x59:
        return decodeSendDestroyItem(c), true
    case 0x6C:
        return decodeSendAcquireSkill(c), true
    default:
        return nil, false
    }
}

// firstInt32 reads the int32 after the opcode (the version fields).
func firstInt32(payload []byte) int32 {
    c := &cursor{payload: payload, pos: 1}
    value, _ := c.int32()

    return value
}

// decodeSendMove renders MoveToLocation: the walk click with its
// destination, origin and mode.
func decodeSendMove(c *cursor) []string {
    tx, ok1 := c.int32()
    ty, ok2 := c.int32()
    tz, ok3 := c.int32()
    ox, ok4 := c.int32()
    oy, ok5 := c.int32()
    oz, ok6 := c.int32()
    mode, _ := c.int32()
    if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
        return []string{shortLine("move to location")}
    }
    modeWord := "mouse"
    if mode == 0 {
        modeWord = "cursor-keys"
    }

    return []string{"move to location dest=" + xyz(tx, ty, tz) +
        " origin=" + xyz(ox, oy, oz) + " mode=" + modeWord}
}

// decodeSendAction renders the world object click (a target select,
// a pickup or an npc talk).
func decodeSendAction(c *cursor) []string {
    objectID, ok1 := c.int32()
    x, ok2 := c.int32()
    y, ok3 := c.int32()
    z, ok4 := c.int32()
    shift := c.byteVal()
    if !ok1 || !ok2 || !ok3 || !ok4 {
        return []string{shortLine("action")}
    }
    line := "click object=" + strconv.Itoa(int(objectID)) +
        " at=" + xyz(x, y, z)
    if shift != 0 {
        line += shiftFlagSuffix
    }

    return []string{line}
}

// decodeSendAuthLogin renders the game server auth login.
func decodeSendAuthLogin(c *cursor) []string {
    account, ok := c.utf16()
    if !ok {
        return []string{shortLine("auth login")}
    }

    return []string{"auth login account=" + account}
}

// decodeSendCharCreate renders the character creation request.
func decodeSendCharCreate(c *cursor) []string {
    name, ok := c.utf16()
    if !ok {
        return []string{shortLine("character create")}
    }

    return []string{"character create name=" + name}
}

// decodeSendCharSelect renders the character slot selection.
func decodeSendCharSelect(c *cursor) []string {
    slot, ok := c.int32()
    if !ok {
        return []string{shortLine("character select")}
    }

    return []string{"character select slot=" + strconv.Itoa(int(slot))}
}

// decodeSendDropItem renders the ground drop request.
func decodeSendDropItem(c *cursor) []string {
    objectID, ok1 := c.int32()
    count, ok2 := c.int32()
    x, ok3 := c.int32()
    y, ok4 := c.int32()
    z, ok5 := c.int32()
    if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
        return []string{shortLine("drop item")}
    }

    return []string{"drop item object=" + strconv.Itoa(int(objectID)) +
        " count=" + strconv.Itoa(int(count)) + " at=" + xyz(x, y, z)}
}

// decodeSendUseItem renders the use/equip request.
func decodeSendUseItem(c *cursor) []string {
    objectID, ok := c.int32()
    if !ok {
        return []string{shortLine("use item")}
    }

    return []string{"use item object=" + strconv.Itoa(int(objectID))}
}

// decodeSendChangeMoveType renders the run/walk stance switch.
func decodeSendChangeMoveType(c *cursor) []string {
    run, ok := c.int32()
    if !ok {
        return []string{shortLine("change move type")}
    }
    stance := "walk"
    if run != 0 {
        stance = "run"
    }

    return []string{"change move type " + stance}
}

// decodeSendSellItem renders the sell batch with one continuation
// line per sold stack.
func decodeSendSellItem(c *cursor) []string {
    listID, ok1 := c.int32()
    count, ok2 := c.int32()
    if !ok1 || !ok2 || count < 0 || count > 512 {
        return []string{shortLine("sell item")}
    }
    lines := []string{"sell item list=" + strconv.Itoa(int(listID)) +
        " entries=" + strconv.Itoa(int(count))}
    for range count {
        objectID, okA := c.int32()
        itemID, okB := c.int32()
        cnt, okC := c.int32()
        if !okA || !okB || !okC {
            lines = append(lines, "  entry cut short")

            break
        }
        lines = append(lines, "  sell "+npcdata.ItemName(itemID)+
            " (item "+strconv.Itoa(int(itemID))+", object "+
            strconv.Itoa(int(objectID))+") x"+
            strconv.Itoa(int(cnt)))
    }

    return lines
}

// decodeSendBuyItem renders the buy batch with one continuation
// line per bought stack.
func decodeSendBuyItem(c *cursor) []string {
    listID, ok1 := c.int32()
    count, ok2 := c.int32()
    if !ok1 || !ok2 || count < 0 || count > 512 {
        return []string{shortLine("buy item")}
    }
    lines := []string{"buy item list=" + strconv.Itoa(int(listID)) +
        " entries=" + strconv.Itoa(int(count))}
    for range count {
        itemID, okA := c.int32()
        cnt, okB := c.int32()
        if !okA || !okB {
            lines = append(lines, "  entry cut short")

            break
        }
        lines = append(lines, "  buy "+npcdata.ItemName(itemID)+
            " (item "+strconv.Itoa(int(itemID))+") x"+
            strconv.Itoa(int(cnt)))
    }

    return lines
}

// decodeSendBypass renders the npc dialog bypass command (the
// button click of a html page).
func decodeSendBypass(c *cursor) []string {
    command, ok := c.utf16()
    if !ok {
        return []string{shortLine("bypass")}
    }

    return []string{"bypass \"" + command + "\""}
}

// decodeSendMagicSkillUse renders the skill cast request.
func decodeSendMagicSkillUse(c *cursor) []string {
    skillID, ok1 := c.int32()
    ctrl, ok2 := c.int32()
    shift := c.byteVal()
    if !ok1 || !ok2 {
        return []string{shortLine("magic skill use")}
    }
    line := "cast " + skillName(skillID) + " (skill " +
        strconv.Itoa(int(skillID)) + ")"
    if ctrl != 0 {
        line += " ctrl"
    }
    if shift != 0 {
        line += shiftFlagSuffix
    }

    return []string{line}
}

// decodeSendActionUse renders the social action request (the sit
// and stand of the rest cycle ride here).
func decodeSendActionUse(c *cursor) []string {
    actionID, ok1 := c.int32()
    ctrl, ok2 := c.int32()
    shift := c.byteVal()
    if !ok1 || !ok2 {
        return []string{shortLine("action use")}
    }
    line := "action use id=" + strconv.Itoa(int(actionID))
    if ctrl != 0 {
        line += " ctrl"
    }
    if shift != 0 {
        line += shiftFlagSuffix
    }

    return []string{line}
}

// decodeSendValidatePosition renders the client position claim the
// session streams while the character moves.
func decodeSendValidatePosition(c *cursor) []string {
    x, ok1 := c.int32()
    y, ok2 := c.int32()
    z, ok3 := c.int32()
    heading, ok4 := c.int32()
    if !ok1 || !ok2 || !ok3 || !ok4 {
        return []string{shortLine("validate position")}
    }

    return []string{"validate position at=" + xyz(x, y, z) +
        " heading=" + strconv.Itoa(int(heading))}
}

// decodeSendDestroyItem renders the destroy request.
func decodeSendDestroyItem(c *cursor) []string {
    objectID, ok1 := c.int32()
    count, ok2 := c.int32()
    if !ok1 || !ok2 {
        return []string{shortLine("destroy item")}
    }

    return []string{"destroy item object=" +
        strconv.Itoa(int(objectID)) + " count=" +
        strconv.Itoa(int(count))}
}

// decodeSendAcquireSkill renders the lesson request.
func decodeSendAcquireSkill(c *cursor) []string {
    skillID, ok1 := c.int32()
    level, ok2 := c.int32()
    if !ok1 || !ok2 {
        return []string{shortLine("acquire skill")}
    }

    return []string{"learn " + skillName(skillID) + " (skill " +
        strconv.Itoa(int(skillID)) + " level " +
        strconv.Itoa(int(level)) + ")"}
}

// decodeSendRestartPoint renders the restart point request (the
// village revive of the death flow).
func decodeSendRestartPoint(c *cursor) []string {
    pointType, ok := c.int32()
    if !ok {
        return []string{shortLine("restart point")}
    }
    word := "village"
    if pointType != 0 {
        word = "agathion"
    }

    return []string{"restart point " + word}
}
