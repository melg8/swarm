// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The server-to-client packet decoder of the run log: one readable line (plus
// continuation lines for the list packets) per packet the session receives. The
// decode reuses the tested parsers of the from_game_server package; a parse
// failure or an unknown opcode falls back to the size and the hex tail instead
// of panicking on the session reader goroutine.

package botlog

import (
    "strconv"
    "strings"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
)

// decodeRecv renders one server packet into its log lines: the session family
// answers directly, while the world, combat and list families ride the
// dispatchers that mirror the connection routing.
func decodeRecv(payload []byte) []string {
    if len(payload) == 0 {
        return nil
    }
    switch payload[0] {
    case 0x35:
        return []string{"action failed (the server refused the " +
            "last request)"}
    case 0x36:
        return []string{"server is closing the connection"}
    case 0x96:
        return []string{"leave world confirmed"}
    case 0x98:
        return []string{"quest list"}
    case 0xEC:
        return []string{"net ping answer"}
    }
    if lines, ok := decodeRecvWorld(payload); ok {
        return lines
    }
    if lines, ok := decodeRecvCombat(payload); ok {
        return lines
    }
    if lines, ok := decodeRecvList(payload); ok {
        return lines
    }

    return []string{unknownLine("recv", payload)}
}

func decodeRecvWorld(payload []byte) ([]string, bool) {
    if lines, ok := decodeRecvObject(payload); ok {
        return lines, true
    }
    switch payload[0] {
    case 0x01:
        return decodeRecvMoveToLocation(payload), true
    case 0x38:
        return decodeRecvTeleport(payload), true
    case 0x59:
        return decodeRecvStopMove(payload), true
    case 0x75:
        return decodeRecvMoveToPawn(payload), true
    case 0x76:
        return decodeRecvValidateLocation(payload), true
    case 0x77:
        return decodeRecvRotation(payload, "begin rotation"), true
    case 0x78:
        return decodeRecvRotation(payload, "stop rotation"), true
    case 0x3E:
        return decodeRecvChangeMoveType(payload), true
    case 0x3F:
        return decodeRecvChangeWaitType(payload), true
    default:
        return nil, false
    }
}

func decodeRecvObject(payload []byte) ([]string, bool) {
    switch payload[0] {
    case 0x03:
        return decodeRecvCharInfo(payload), true
    case 0x04:
        return decodeRecvUserInfo(payload), true
    case 0x15:
        return decodeRecvSpawnItem(payload), true
    case 0x16:
        return decodeRecvDropItem(payload), true
    case 0x17:
        return decodeRecvGetItem(payload), true
    case 0x1A:
        return decodeRecvStatusUpdate(payload), true
    case 0x1E:
        return decodeRecvDeleteObject(payload), true
    case 0x22:
        return decodeRecvNpcInfo(payload), true
    default:
        return nil, false
    }
}

func decodeRecvCombat(payload []byte) ([]string, bool) {
    switch payload[0] {
    case 0x06:
        return decodeRecvAttack(payload), true
    case 0x39:
        return decodeRecvTargetSelected(payload), true
    case 0x3A:
        return decodeRecvTargetUnselected(payload), true
    case 0x3B:
        return decodeRecvAutoAttackStart(payload), true
    case 0x3C:
        return decodeRecvAutoAttackStop(payload), true
    case 0x3D:
        return decodeRecvSocialAction(payload), true
    case 0xBF:
        return decodeRecvMyTargetSelected(payload), true
    default:
        return nil, false
    }
}

// decodeRecvList dispatches the account, character and inventory list packets.
func decodeRecvList(payload []byte) ([]string, bool) {
    switch payload[0] {
    case 0x1B:
        return decodeRecvNpcHTML(payload), true
    case 0x1F:
        return decodeRecvCharSelectInfo(payload), true
    case 0x21:
        return decodeRecvCharSelected(payload), true
    case 0x27:
        return decodeRecvItemList(payload), true
    case 0x37:
        return decodeRecvInventoryUpdate(payload), true
    case 0x6D:
        return decodeRecvSkillList(payload), true
    case 0x97:
        return decodeRecvAbnormalStatus(payload), true
    case 0x7A:
        return decodeRecvSystemMessage(payload), true
    default:
        return nil, false
    }
}

func decodeRecvMoveToLocation(payload []byte) []string {
    packet := fromgameserver.NewMoveToLocationPacket()
    if err := fromgameserver.ParseMoveToLocationPacket(
        packet, payload); err != nil {
        return []string{shortLine("move to location")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " moves to " + xyz(packet.DestX, packet.DestY, packet.DestZ) +
        " from " + xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvCharInfo(payload []byte) []string {
    packet := fromgameserver.NewCharInfoPacket()
    if err := fromgameserver.ParseCharInfoPacket(packet, payload); err != nil {
        return []string{shortLine("char info")}
    }
    var flags []string
    if packet.Running {
        flags = append(flags, "running")
    }
    if packet.InCombat {
        flags = append(flags, "combat")
    }
    if packet.Dead {
        flags = append(flags, "dead")
    }
    if packet.Standing {
        flags = append(flags, "standing")
    }
    flagSuffix := flagWords(flags)

    return []string{"player " + quote(packet.Name) + " (object " +
        strconv.Itoa(int(packet.ObjectID)) + ") at " +
        xyz(packet.X, packet.Y, packet.Z) + flagSuffix}
}

func decodeRecvUserInfo(payload []byte) []string {
    packet := fromgameserver.NewUserInfoPacket()
    if err := fromgameserver.ParseUserInfoPacket(packet, payload); err != nil {
        return []string{shortLine("user info")}
    }

    return []string{"self " + quote(packet.Name) + " (object " +
        strconv.Itoa(int(packet.ObjectID)) + ") level " +
        strconv.Itoa(int(packet.Level)) + " hp " +
        strconv.Itoa(int(packet.CurHP)) + "/" +
        strconv.Itoa(int(packet.MaxHP)) + " mp " +
        strconv.Itoa(int(packet.CurMP)) + "/" +
        strconv.Itoa(int(packet.MaxMP)) + " exp " +
        strconv.Itoa(int(packet.Exp)) + " sp " +
        strconv.Itoa(int(packet.Sp)) + " load " +
        strconv.Itoa(int(packet.CurrentLoad)) + "/" +
        strconv.Itoa(int(packet.MaxLoad))}
}

func decodeRecvAttack(payload []byte) []string {
    packet := fromgameserver.NewAttackPacket()
    if err := fromgameserver.ParseAttackPacket(packet, payload); err != nil {
        return []string{shortLine("attack")}
    }
    var line strings.Builder
    line.WriteString("object ")
    line.WriteString(strconv.Itoa(int(packet.AttackerID)))
    line.WriteString(" attacks from ")
    line.WriteString(xyz(packet.X, packet.Y, packet.Z))
    count := min(packet.HitCount, 4)
    for i := range count {
        hit := packet.Hits[i]
        line.WriteString(" hit object ")
        line.WriteString(strconv.Itoa(int(hit.TargetID)))
        line.WriteString(" for ")
        line.WriteString(strconv.Itoa(int(hit.Damage)))
    }

    return []string{line.String()}
}

func decodeRecvSpawnItem(payload []byte) []string {
    packet := fromgameserver.NewSpawnItemPacket()
    if err := fromgameserver.ParseSpawnItemPacket(packet, payload); err != nil {
        return []string{shortLine("spawn item")}
    }

    return []string{"item " + quote(npcdata.ItemName(packet.TemplateID)) +
        " (object " + strconv.Itoa(int(packet.ObjectID)) + ") x" +
        strconv.Itoa(int(packet.Count)) + " appeared at " +
        xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvDropItem(payload []byte) []string {
    packet := fromgameserver.NewDropItemPacket()
    if err := fromgameserver.ParseDropItemPacket(packet, payload); err != nil {
        return []string{shortLine("drop item")}
    }

    return []string{"item " + quote(npcdata.ItemName(packet.TemplateID)) +
        " (object " + strconv.Itoa(int(packet.ObjectID)) + ") x" +
        strconv.Itoa(int(packet.Count)) + " dropped at " +
        xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvGetItem(payload []byte) []string {
    packet := fromgameserver.NewGetItemPacket()
    if err := fromgameserver.ParseGetItemPacket(packet, payload); err != nil {
        return []string{shortLine("get item")}
    }

    return []string{"object " + strconv.Itoa(int(packet.PlayerID)) +
        " picked up object " + strconv.Itoa(int(packet.ObjectID)) +
        " at " + xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvStatusUpdate(payload []byte) []string {
    packet := fromgameserver.NewStatusUpdatePacket()
    if err := fromgameserver.ParseStatusUpdatePacket(
        packet, payload); err != nil {
        return []string{shortLine("status update")}
    }
    var line strings.Builder
    line.WriteString("object ")
    line.WriteString(strconv.Itoa(int(packet.ObjectID)))
    line.WriteString(" status:")
    packet.ForEach(func(id int32, value int32) {
        line.WriteString(" ")
        line.WriteString(attrName(id))
        line.WriteString("=")
        line.WriteString(strconv.Itoa(int(value)))
    })

    return []string{line.String()}
}

// attrNames names the StatusUpdate attribute ids (an unmapped one renders as
// attrN).
var attrNames = map[int32]string{
    0x01: "level", 0x02: "exp", 0x03: "str", 0x04: "dex",
    0x05: "con", 0x06: "int", 0x07: "wit", 0x08: "men",
    0x09: "curHp", 0x0A: "maxHp", 0x0B: "curMp", 0x0C: "maxMp",
    0x0D: "curCp", 0x0E: "curLoad", 0x0F: "maxLoad",
    0x1A: "adena", 0x21: "sp",
}

func attrName(id int32) string {
    if name, ok := attrNames[id]; ok {
        return name
    }

    return "attr" + strconv.Itoa(int(id))
}

// decodeRecvNpcHTML renders the dialog page with its clickable bypass links.
func decodeRecvNpcHTML(payload []byte) []string {
    packet := fromgameserver.NewNpcHTMLMessage()
    if err := fromgameserver.ParseNpcHTMLMessage(packet, payload); err != nil {
        return []string{shortLine("npc html")}
    }
    links := fromgameserver.ParseHTMLLinks(packet.HTML)
    lines := []string{"npc " + strconv.Itoa(int(packet.NpcObjID)) +
        " dialog " + strconv.Itoa(len(packet.HTML)) + " bytes, " +
        strconv.Itoa(len(links)) + " links"}
    for i, link := range links {
        if i >= 16 {
            lines = append(lines, "  ... "+
                strconv.Itoa(len(links)-16)+" more links")

            break
        }
        lines = append(lines, "  link "+quote(trimText(link.Command, 80))+
            " text "+quote(trimText(link.Text, 48)))
    }

    return lines
}

func decodeRecvDeleteObject(payload []byte) []string {
    packet := fromgameserver.NewDeleteObjectPacket()
    if err := fromgameserver.ParseDeleteObjectPacket(
        packet, payload); err != nil {
        return []string{shortLine("delete object")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " removed"}
}

// decodeRecvCharSelectInfo renders the account character list, one line per
// slot.
func decodeRecvCharSelectInfo(payload []byte) []string {
    packet := fromgameserver.NewCharSelectInfoPacket()
    if err := fromgameserver.ParseCharSelectInfoPacket(
        packet, payload); err != nil {
        return []string{shortLine("char select info")}
    }
    lines := []string{"character list: " +
        strconv.Itoa(len(packet.Characters)) + " characters"}
    for i := range packet.Characters {
        character := &packet.Characters[i]
        lines = append(lines, "  slot "+strconv.Itoa(i)+": "+
            quote(character.Name)+" level "+
            strconv.Itoa(int(character.Level))+" at "+
            xyz(character.X, character.Y, character.Z))
    }

    return lines
}

func decodeRecvCharSelected(payload []byte) []string {
    packet := fromgameserver.NewCharSelectedPacket()
    if err := fromgameserver.ParseCharSelectedPacket(
        packet, payload); err != nil {
        return []string{shortLine("char selected")}
    }

    return []string{"playing " + quote(packet.Name) + " (object " +
        strconv.Itoa(int(packet.ObjectID)) + ", class " +
        strconv.Itoa(int(packet.ClassID)) + ") at " +
        xyz(packet.X, packet.Y, packet.Z) + " hp " +
        strconv.Itoa(int(packet.CurrentHP)) + " mp " +
        strconv.Itoa(int(packet.CurrentMP))}
}

// decodeRecvNpcInfo renders one npc spawn with the catalog level and
// aggression.
func decodeRecvNpcInfo(payload []byte) []string {
    packet := fromgameserver.NewNpcInfoPacket()
    if err := fromgameserver.ParseNpcInfoPacket(packet, payload); err != nil {
        return []string{shortLine("npc info")}
    }
    line := "npc " + quote(packet.Name)
    if packet.Title != "" {
        line += " " + quote(packet.Title)
    }
    line += " (object " + strconv.Itoa(int(packet.ObjectID)) +
        ", template " + strconv.Itoa(int(packet.TemplateID)) +
        ", level " + strconv.Itoa(int(npcdata.NPCLevel(
        packet.TemplateID))) + ") at " +
        xyz(packet.X, packet.Y, packet.Z)
    if packet.Attackable {
        line += " attackable"
    }
    if npcdata.NPCIsAggressive(packet.TemplateID) {
        line += " aggressive"
    }
    if packet.InCombat {
        line += " combat"
    }
    if packet.Dead {
        line += " dead"
    }

    return []string{line}
}

// decodeRecvItemList renders the full inventory, a continuation line per stack.
func decodeRecvItemList(payload []byte) []string {
    packet := fromgameserver.NewItemListPacket()
    if err := fromgameserver.ParseItemListPacket(packet, payload); err != nil {
        return []string{shortLine("item list")}
    }
    lines := []string{"inventory list: " +
        strconv.Itoa(len(packet.Items)) + " items"}
    for i := range packet.Items {
        lines = append(lines, inventoryLine(&packet.Items[i], "item"))
    }

    return lines
}

func decodeRecvInventoryUpdate(payload []byte) []string {
    packet := fromgameserver.NewInventoryUpdatePacket()
    if err := fromgameserver.ParseInventoryUpdatePacket(
        packet, payload); err != nil {
        return []string{shortLine("inventory update")}
    }
    lines := []string{"inventory update: " +
        strconv.Itoa(len(packet.Items)) + " changes"}
    for i := range packet.Items {
        lines = append(lines, inventoryLine(&packet.Items[i],
            changeWord(packet.Items[i].Change)))
    }

    return lines
}

func inventoryLine(
    item *fromgameserver.InventoryItem, verb string,
) string {
    line := "  " + verb + " " + quote(npcdata.ItemName(item.ItemID)) +
        " (item " + strconv.Itoa(int(item.ItemID)) + ", object " +
        strconv.Itoa(int(item.ObjectID)) + ") x" +
        strconv.Itoa(int(item.Count))
    if item.Equipped {
        line += " equipped"
    }
    if item.Enchant > 0 {
        line += " +" + strconv.Itoa(int(item.Enchant))
    }

    return line
}

func changeWord(change int16) string {
    switch change {
    case 1:
        return "add"
    case 2:
        return "modify"
    case 3:
        return "remove"
    default:
        return "change" + strconv.Itoa(int(change))
    }
}

func decodeRecvTeleport(payload []byte) []string {
    packet := fromgameserver.NewTeleportToLocationPacket()
    if err := fromgameserver.ParseTeleportToLocationPacket(
        packet, payload); err != nil {
        return []string{shortLine("teleport")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " teleported to " + xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvTargetSelected(payload []byte) []string {
    packet := fromgameserver.NewTargetSelectedPacket()
    if err := fromgameserver.ParseTargetSelectedPacket(
        packet, payload); err != nil {
        return []string{shortLine("target selected")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " targeted object " + strconv.Itoa(int(packet.TargetID))}
}

func decodeRecvTargetUnselected(payload []byte) []string {
    packet := fromgameserver.NewTargetUnselectedPacket()
    if err := fromgameserver.ParseTargetUnselectedPacket(
        packet, payload); err != nil {
        return []string{shortLine("target unselected")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " dropped its target"}
}

func decodeRecvSocialAction(payload []byte) []string {
    packet := fromgameserver.NewSocialActionPacket()
    if err := fromgameserver.ParseSocialActionPacket(
        packet, payload); err != nil {
        return []string{shortLine("social action")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " social action " + strconv.Itoa(int(packet.ActionID))}
}

func decodeRecvChangeMoveType(payload []byte) []string {
    packet := fromgameserver.NewChangeMoveTypePacket()
    if err := fromgameserver.ParseChangeMoveTypePacket(
        packet, payload); err != nil {
        return []string{shortLine("change move type")}
    }
    stance := "walk"
    if packet.Running {
        stance = "run"
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " switches to " + stance}
}

func decodeRecvChangeWaitType(payload []byte) []string {
    packet := fromgameserver.NewChangeWaitTypePacket()
    if err := fromgameserver.ParseChangeWaitTypePacket(
        packet, payload); err != nil {
        return []string{shortLine("change wait type")}
    }
    stance := "standing"
    if packet.Sitting {
        stance = "sitting"
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " is " + stance}
}

// decodeRecvStopMove renders the movement stop with the facing.
func decodeRecvStopMove(payload []byte) []string {
    packet := fromgameserver.NewStopMovePacket()
    if err := fromgameserver.ParseStopMovePacket(packet, payload); err != nil {
        return []string{shortLine("stop move")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " stopped at " + xyz(packet.X, packet.Y, packet.Z) +
        " heading " + strconv.Itoa(int(packet.Heading))}
}

// decodeRecvSkillList renders the learned skill list, one line per skill.
func decodeRecvSkillList(payload []byte) []string {
    packet := fromgameserver.NewSkillListPacket()
    if err := fromgameserver.ParseSkillListPacket(packet, payload); err != nil {
        return []string{shortLine("skill list")}
    }
    lines := []string{"skill list: " +
        strconv.Itoa(len(packet.Skills)) + " skills"}
    for i := range packet.Skills {
        skill := &packet.Skills[i]
        line := "  skill " + skillName(skill.SkillID) +
            " (id " + strconv.Itoa(int(skill.SkillID)) + ") level " +
            strconv.Itoa(int(skill.Level))
        if skill.Passive {
            line += " passive"
        }
        lines = append(lines, line)
    }

    return lines
}

func decodeRecvMoveToPawn(payload []byte) []string {
    packet := fromgameserver.NewMoveToPawnPacket()
    if err := fromgameserver.ParseMoveToPawnPacket(
        packet, payload); err != nil {
        return []string{shortLine("move to pawn")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " chases object " + strconv.Itoa(int(packet.TargetID)) +
        " at distance " + strconv.Itoa(int(packet.Distance)) +
        " from " + xyz(packet.X, packet.Y, packet.Z)}
}

func decodeRecvValidateLocation(payload []byte) []string {
    packet := fromgameserver.NewValidateLocationPacket()
    if err := fromgameserver.ParseValidateLocationPacket(
        packet, payload); err != nil {
        return []string{shortLine("validate location")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " placed at " + xyz(packet.X, packet.Y, packet.Z) +
        " heading " + strconv.Itoa(int(packet.Heading))}
}

// decodeRecvSystemMessage renders the chat notification with its text
// substituted.
func decodeRecvSystemMessage(payload []byte) []string {
    packet := fromgameserver.NewSystemMessagePacket()
    if err := fromgameserver.ParseSystemMessagePacket(
        packet, payload); err != nil {
        return []string{shortLine("system message")}
    }

    return []string{"system message " +
        quote(substituteMessageText(packet)) + " (id " +
        strconv.Itoa(int(packet.MessageID)) + ")"}
}

// sysParamSkillName is the skill name parameter type of SystemMessage.java
// writeImpl: the int pair carries the skill id then the skill level.
const sysParamSkillName = 4

// substituteMessageText renders the message text with its $sN placeholders
// replaced by the packet parameters; a skill name parameter resolves through
// the generated dictionary, so the raw id never surfaces in the line.
func substituteMessageText(
    packet *fromgameserver.SystemMessagePacket,
) string {
    text := npcdata.SystemMessageText(packet.MessageID)
    for i := range packet.Params {
        param := &packet.Params[i]
        placeholder := "$s" + strconv.Itoa(i+1)
        value := param.Text
        if value == "" {
            value = strconv.Itoa(int(param.Int))
            if param.Type == sysParamSkillName {
                if name := skillParamName(param); name != "" {
                    value = name
                }
            }
        }
        text = strings.ReplaceAll(text, placeholder, value)
    }

    return text
}

// skillParamName renders one skill name parameter as the dictionary name with
// its level ("Power Strike lvl 3", the bare name without a level).
func skillParamName(
    param *fromgameserver.SystemMessageParam,
) string {
    info, ok := npcdata.SkillInfoOf(param.Int)
    if !ok || info.Name == "" {
        return ""
    }
    if param.Level > 0 {
        return info.Name + " lvl " + strconv.Itoa(int(param.Level))
    }

    return info.Name
}

func decodeRecvAbnormalStatus(payload []byte) []string {
    packet := fromgameserver.NewAbnormalStatusUpdatePacket()
    if err := fromgameserver.ParseAbnormalStatusUpdatePacket(
        packet, payload); err != nil {
        return []string{shortLine("abnormal status")}
    }
    var line strings.Builder
    line.WriteString("effects:")
    for i := range packet.Buffs {
        buff := &packet.Buffs[i]
        line.WriteString(" ")
        line.WriteString(skillName(buff.SkillID))
        line.WriteString(" (id ")
        line.WriteString(strconv.Itoa(int(buff.SkillID)))
        line.WriteString(") level ")
        line.WriteString(strconv.Itoa(int(buff.Level)))
    }

    return []string{line.String()}
}

// decodeRecvRotation renders the turn broadcasts: the begin and stop packets
// share the object id and heading fields but differ behind them (begin carries
// the side and the speed ints, stop only the speed and an unknown byte), so
// each parses through its own wire shape.
func decodeRecvRotation(payload []byte, name string) []string {
    objectID, heading, err := parseRotation(payload)
    if err != nil {
        return []string{shortLine(name)}
    }

    return []string{name + ": object " +
        strconv.Itoa(int(objectID)) + " heading " +
        strconv.Itoa(int(heading))}
}

// stopRotationOpcode picks the stop rotation wire shape inside parseRotation.
const stopRotationOpcode = 0x78

// parseRotation reads the object id and the heading of either rotation packet:
// the stop wire is two ints shorter, so the begin parser reads a real stop
// packet as a short payload.
func parseRotation(payload []byte) (int32, int32, error) {
    if payload[0] == stopRotationOpcode {
        packet := fromgameserver.NewStopRotationPacket()
        err := fromgameserver.ParseStopRotationPacket(packet, payload)

        return packet.ObjectID, packet.Heading, err
    }
    packet := fromgameserver.NewBeginRotationPacket()
    err := fromgameserver.ParseBeginRotationPacket(packet, payload)

    return packet.ObjectID, packet.Heading, err
}

func decodeRecvAutoAttackStart(payload []byte) []string {
    packet := fromgameserver.NewAutoAttackStartPacket()
    if err := fromgameserver.ParseAutoAttackStartPacket(
        packet, payload); err != nil {
        return []string{shortLine("auto attack start")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " starts the auto attack"}
}

func decodeRecvAutoAttackStop(payload []byte) []string {
    packet := fromgameserver.NewAutoAttackStopPacket()
    if err := fromgameserver.ParseAutoAttackStopPacket(
        packet, payload); err != nil {
        return []string{shortLine("auto attack stop")}
    }

    return []string{"object " + strconv.Itoa(int(packet.ObjectID)) +
        " stops the auto attack"}
}

func decodeRecvMyTargetSelected(payload []byte) []string {
    packet := fromgameserver.NewMyTargetSelectedPacket()
    if err := fromgameserver.ParseMyTargetSelectedPacket(
        packet, payload); err != nil {
        return []string{shortLine("my target selected")}
    }

    return []string{"own target is object " +
        strconv.Itoa(int(packet.ObjectID))}
}

func flagWords(flags []string) string {
    if len(flags) == 0 {
        return ""
    }

    return " " + strings.Join(flags, " ")
}
