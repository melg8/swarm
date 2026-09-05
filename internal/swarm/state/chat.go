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

// Parameter types of the SystemMessage packet (SystemMessage.java of
// the Mobius server) that need special rendering.
const (
	chatParamText = 0
	chatParamNpc  = 2
	chatParamItem = 3
)

// ChatEvent is one line of the web chat window: a parsed system
// message or a social animation of a creature around the bot.
type ChatEvent struct {
	Time time.Time `json:"time"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// ChatMessageParam mirrors one SystemMessage packet parameter: the type
// decides whether the int or the text value is meaningful.
type ChatMessageParam struct {
	Type int32
	Int  int32
	Text string
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
// appends it to the chat window log.
func (b *Bot) ApplySystemMessage(m SystemMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recordChatLocked("system",
		formatChatText(npcdata.SystemMessageText(m.ID), m.Params))
}

// ApplySocialAction appends a social animation line to the chat window
// log: level ups of anyone and the idle animations of the creatures
// around the bot.
func (b *Bot) ApplySocialAction(a SocialAction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	name := "unknown"
	if a.ObjectID == b.selfID {
		name = "You"
	} else if obj, ok := b.objects[a.ObjectID]; ok && obj.Name != "" {
		name = obj.Name
	}
	var text string
	if a.ActionID == socialActionLevelUp {
		text = name + " reached a new level"
	} else {
		text = name + " plays social animation " +
			strconv.Itoa(int(a.ActionID))
	}
	b.recordChatLocked("social", text)
}

// recordChatLocked appends one line to the chat ring buffer. The caller
// must hold the state write lock.
func (b *Bot) recordChatLocked(kind string, text string) {
	b.chatLog[b.chatPos] = ChatEvent{
		Time: time.Now(), Kind: kind, Text: text,
	}
	b.chatPos = (b.chatPos + 1) % chatCapacity
	if b.chatLen < chatCapacity {
		b.chatLen++
	}
	b.touch()
}

// formatChatText substitutes the $sN and $cN placeholders of a system
// message text with the packet parameters in their order.
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

	return out.String()
}

// renderChatParam renders one parameter for the chat text: item and npc
// name parameters resolve through the generated dictionaries, everything
// else falls back to its raw value.
func renderChatParam(params []ChatMessageParam, index int) string {
	if index >= len(params) {
		return "?"
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
	}

	return strconv.Itoa(int(param.Int))
}
