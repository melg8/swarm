// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "errors"
    "unicode/utf8"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const sayPacketID = 0x38

// ChatType client ids of the Say2 packet (ChatType.java of the Mobius
// server). The send whitelist carries only the channels a player chat
// handler exists for (ChatHandler.java); the receive side maps the
// rest of the ids in the state tracker.
const (
    SayChannelGeneral = 0
    SayChannelShout   = 1
    SayChannelWhisper = 2
    SayChannelParty   = 3
    SayChannelClan    = 4
    SayChannelTrade   = 8
    SayMaxTextRunes   = 105
)

// Say is the Say2 client packet: one chat message of the character.
// The server broadcasts it through the channel handler and answers an
// unknown channel with a disconnect (Say2.runImpl), an empty text with
// a disconnect too, so the bot validates both before sending.
// Wire format (see Say2.readImpl): [opcode 0x38][text: null-terminated
// UTF-16LE][channel: 4] plus [target: null-terminated UTF-16LE] for
// the whisper channel only.
// Reference: org/l2jmobius/gameserver/network/clientpackets/Say2.java.
type Say struct {
    Text    string
    Channel int32
    Target  string
}

// NewSay builds a chat message packet for the channel. The whisper
// target stays empty for every other channel.
func NewSay(text string, channel int32) *Say {
    return &Say{
        Text:    text,
        Channel: channel,
        Target:  "",
    }
}

// Validate reports whether the server accepts the message: Say2
// disconnects on an unknown channel and on an empty text, and refuses
// more than 105 characters from a non GM (the keyboard input limit).
func (p *Say) Validate() error {
    if p.Text == "" {
        return errors.New("chat text is empty")
    }
    if utf8.RuneCountInString(p.Text) > SayMaxTextRunes {
        return errors.New("chat text exceeds 105 characters")
    }
    switch p.Channel {
    case SayChannelGeneral, SayChannelShout, SayChannelWhisper,
        SayChannelParty, SayChannelClan, SayChannelTrade:
    default:
        return errors.New("unsupported chat channel")
    }
    if p.Channel == SayChannelWhisper && p.Target == "" {
        return errors.New("whisper needs a target name")
    }

    return nil
}

// ToBytes serializes the packet. The message is validated first: the
// server answers a malformed chat with a disconnect, never a refusal.
func (p *Say) ToBytes(writer *packet.Writer) error {
    if err := p.Validate(); err != nil {
        return err
    }
    if err := writer.WriteInt8(sayPacketID); err != nil {
        return err
    }
    if err := writer.WriteStringAsUtf16(p.Text); err != nil {
        return err
    }
    if err := writer.WriteInt32(p.Channel); err != nil {
        return err
    }
    if p.Channel == SayChannelWhisper {
        return writer.WriteStringAsUtf16(p.Target)
    }

    return nil
}
