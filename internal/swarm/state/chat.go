// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// chatCapacity is the size of the rolling chat window log per bot.
const chatCapacity = 64

// socialActionLevelUp is the social action id the server broadcasts
// when a character reaches a new level (SocialAction.LEVEL_UP).
const socialActionLevelUp = 15

// systemMessageCannotSeeTarget is the SystemMessage id the Mobius C1
// server answers an attack with when the geodata line of sight to the
// target is blocked (SystemMessageId.CANNOT_SEE_TARGET -
// Creature.doAttack sends it and keeps the attack intention armed, so
// the refusal repeats on every AI think while the obstruction lasts).
const systemMessageCannotSeeTarget = 181

// socialWindow is how long a creature keeps the social animation marker
// on the map after its SocialAction broadcast.
const socialWindow = 3 * time.Second

// Parameter types of the SystemMessage packet (SystemMessage.java of
// the Mobius server) that need special rendering.
const (
    chatParamText  = 0
    chatParamNpc   = 2
    chatParamItem  = 3
    chatParamSkill = 4
)

// ChatEvent is one line of the web chat window: a parsed system
// message, a social animation of a creature around the bot or a
// world chat line of the CreatureSay packet. The From field carries
// the sender name of the chat lines and stays empty for the system
// and social kinds.
type ChatEvent struct {
    Time time.Time `json:"time"`
    Kind string    `json:"kind"`
    Text string    `json:"text"`
    From string    `json:"from"`
}

// ChatType client ids of the CreatureSay packet (ChatType.java of the
// Mobius server).
const (
    chatChannelGeneral      = 0
    chatChannelShout        = 1
    chatChannelWhisper      = 2
    chatChannelParty        = 3
    chatChannelClan         = 4
    chatChannelTrade        = 8
    chatChannelAnnouncement = 10
)

// chatKindForChannel maps a CreatureSay channel to the chat line kind
// the web UI styles by. Unknown channels degrade to the plain say
// kind instead of being dropped: the text still matters.
func chatKindForChannel(channel int32) string {
    switch channel {
    case chatChannelShout:
        return "shout"
    case chatChannelWhisper:
        return "whisper"
    case chatChannelParty:
        return "party"
    case chatChannelClan:
        return "clan"
    case chatChannelTrade:
        return "trade"
    case chatChannelAnnouncement:
        return "announcement"
    default:
        return "say"
    }
}

// Say carries the parsed CreatureSay packet: one world chat line of
// a creature (the own sends of the character echo back through the
// same broadcast).
type Say struct {
    ObjectID int32
    Channel  int32
    From     string
    Text     string
}

// ApplySay appends a world chat line to the chat window log. The
// sender name rides the From field of the chat event: the web UI
// renders it as its own column.
func (b *Bot) ApplySay(s Say) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.chat.record(chatKindForChannel(s.Channel), s.Text, s.From,
        time.Now())
    b.touch()
}

// ChatMessageParam mirrors one SystemMessage packet parameter: the type
// decides whether the int or the text value is meaningful. Level
// carries the second int of the multi int parameters (the skill level
// of a skill name parameter).
type ChatMessageParam struct {
    Type  int32
    Int   int32
    Level int32
    Text  string
}

// SystemMessage carries the parsed SystemMessage packet: the id maps to
// the client side text and the parameters substitute its $sN and $cN
// placeholders.
type SystemMessage struct {
    ID     int32
    Params []ChatMessageParam
}

// SocialAction carries the parsed SocialAction packet.
type SocialAction struct {
    ObjectID int32
    ActionID int32
}

// ApplySystemMessage formats a system message with its parameters and
// appends it to the chat window log. The "Cannot see target." answer
// additionally records its arrival time: the hunt loop reads it to
// recognize an engage the terrain obstructs and to walk around the
// obstacle instead of re-requesting the refused attack forever.
func (b *Bot) ApplySystemMessage(m SystemMessage) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if m.ID == systemMessageCannotSeeTarget {
        b.char.CannotSeeTargetAt = time.Now()
    }
    b.recordChatLocked("system",
        formatChatText(npcdata.SystemMessageText(m.ID), m.Params))
}

// ApplySocialAction marks the animating creature with a short lived
// map marker. The idle gestures of the surrounding npcs would spam the
// chat window, so only level ups - rare and meaningful - become chat
// lines; every other social interaction is the marker alone.
func (b *Bot) ApplySocialAction(a SocialAction) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := time.Now()
    if a.ObjectID == b.selfID {
        b.char.SocialUntil = now.Add(socialWindow)
    } else if _, cold := b.objectLocked(a.ObjectID); cold != nil {
        cold.SocialUntil = now.Add(socialWindow).UnixNano()
    }
    if a.ActionID != socialActionLevelUp {
        return
    }
    name := "someone"
    if a.ObjectID == b.selfID {
        name = "You"
    } else if _, cold := b.objectLocked(a.ObjectID); cold != nil &&
        cold.Name != "" {
        name = cold.Name
    }
    b.recordChatLocked("social", name+" reached a new level")
}

// recordChatLocked appends one line to the chat ring buffer. The caller
// must hold the state write lock.
func (b *Bot) recordChatLocked(kind string, text string) {
    b.chat.record(kind, text, "", time.Now())
    b.touch()
}

// chatLog is the rolling chat window of one bot session, split out of
// the Bot god object: a fixed capacity ring written by the system
// message and social action paths and read whole by the snapshot. The
// ring allocates lazily on the first line so idle sessions pay no
// per bot chat memory.
type chatLog struct {
    ring   []ChatEvent
    length int
    head   int
}

// newChatLog creates the empty log.
func newChatLog() chatLog {
    return chatLog{ring: nil, length: 0, head: 0}
}

// record appends one chat line. The caller must hold the bot write
// lock.
func (l *chatLog) record(
    kind string, text string, from string, at time.Time,
) {
    if l.ring == nil {
        l.ring = make([]ChatEvent, chatCapacity)
    }
    l.ring[l.head] = ChatEvent{Time: at, Kind: kind, Text: text, From: from}
    l.head = (l.head + 1) % chatCapacity
    if l.length < chatCapacity {
        l.length++
    }
}

// appendAll copies every line in chronological order onto dst and
// returns the grown slice.
func (l *chatLog) appendAll(dst []ChatEvent) []ChatEvent {
    for i := range l.length {
        dst = append(dst, l.ring[(l.head-l.length+i+chatCapacity)%chatCapacity])
    }

    return dst
}

// formatChatText substitutes the $sN and $cN placeholders of a system
// message text with the packet parameters in their order. A missing
// parameter renders as nothing (the server sendMessage texts ride the
// generic "$s1 $s2" template with one parameter) and the trailing gap
// it leaves is trimmed.
func formatChatText(text string, params []ChatMessageParam) string {
    var out strings.Builder
    for i := 0; i < len(text); {
        if text[i] == '$' && i+2 < len(text) &&
            (text[i+1] == 's' || text[i+1] == 'c') {
            n := 0
            j := i + 2
            for j < len(text) && text[j] >= '0' && text[j] <= '9' {
                n = n*10 + int(text[j]-'0')
                j++
            }
            if n >= 1 && j > i+2 {
                out.WriteString(renderChatParam(params, n-1))
                i = j

                continue
            }
        }
        out.WriteByte(text[i])
        i++
    }

    return strings.TrimRight(out.String(), " ")
}

// renderChatParam renders one parameter for the chat text: item, npc
// and skill name parameters resolve through the generated
// dictionaries, a missing parameter renders as nothing so the unused
// tail placeholders of the server templates never surface, everything
// else falls back to its raw value.
func renderChatParam(params []ChatMessageParam, index int) string {
    if index >= len(params) {
        return ""
    }
    param := params[index]
    switch param.Type {
    case chatParamText:
        return param.Text
    case chatParamItem:
        if name := npcdata.ItemName(param.Int); name != "" {
            return name
        }
    case chatParamNpc:
        if name := npcdata.NPCName(param.Int + 1000000); name != "" {
            return name
        }
    case chatParamSkill:
        if name := skillChatName(param); name != "" {
            return name
        }
    }

    return strconv.Itoa(int(param.Int))
}

// skillChatName renders a skill name parameter as the generated
// dictionary name with its level: "Power Strike lvl 3". The level
// rides the second int of the packet parameter and a skill without
// one renders the bare name.
func skillChatName(param ChatMessageParam) string {
    info, ok := npcdata.SkillInfoOf(param.Int)
    if !ok || info.Name == "" {
        return ""
    }
    if param.Level > 0 {
        return info.Name + " lvl " + strconv.Itoa(int(param.Level))
    }

    return info.Name
}
