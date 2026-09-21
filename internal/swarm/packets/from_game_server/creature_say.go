// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const creatureSayPacketID = 0x5D

// CreatureSayPacket is one chat line of a creature in the world: the
// sender object id, the ChatType client id of the channel, the sender
// name and the text.
// Wire format (see CreatureSay.writeImpl): [opcode 0x5D]
// [senderObjectId: 4][channel: 4][senderName: null-terminated UTF-16LE]
// [text: null-terminated UTF-16LE]. The system variant that writes an
// int charId and an int messageId instead of the two strings is not a
// chat line and is not parsed (the name read would fail or degrade;
// the C1 client renders those through NpcSay and SystemMessage
// instead).
// Reference: org/l2jmobius/gameserver/network/serverpackets/
// CreatureSay.java.
type CreatureSayPacket struct {
    ObjectID int32
    Channel  int32
    From     string
    Text     string
}

// NewCreatureSayPacket creates a packet ready for parsing.
func NewCreatureSayPacket() *CreatureSayPacket {
    return &CreatureSayPacket{
        ObjectID: 0,
        Channel:  0,
        From:     "",
        Text:     "",
    }
}

// ParseCreatureSayPacket reads the packet from payload bytes.
func ParseCreatureSayPacket(
    p *CreatureSayPacket, data []byte,
) error {
    reader := packet.NewReader(data)
    if err := expectPacketID(reader, creatureSayPacketID); err != nil {
        return err
    }
    if err := readInt32Fields(reader, &p.ObjectID, &p.Channel); err != nil {
        return fmt.Errorf("failed to read say header: %w", err)
    }
    from, err := reader.ReadStringFromUtf16Format()
    if err != nil {
        return fmt.Errorf("failed to read say sender: %w", err)
    }
    text, err := reader.ReadStringFromUtf16Format()
    if err != nil {
        return fmt.Errorf("failed to read say text: %w", err)
    }
    p.From = from
    p.Text = text

    return nil
}
