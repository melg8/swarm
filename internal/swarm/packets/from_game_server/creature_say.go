// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const CreatureSayPacketID = 0x5D

// CreatureSayPacket is one chat line of a creature: the sender object
// id, the ChatType client id of the channel, the sender name and the
// text. Wire (CreatureSay.writeImpl): [opcode 0x5D][senderObjectId:
// 4][channel: 4][senderName: UTF-16LE][text: UTF-16LE], the strings
// null-terminated. The system variant that writes an int charId and an
// int messageId instead of the strings is not a chat line and is not
// parsed; the client renders those through NpcSay and SystemMessage.
type CreatureSayPacket struct {
    ObjectID int32
    Channel  int32
    From     string
    Text     string
}

func NewCreatureSayPacket() *CreatureSayPacket {
    return &CreatureSayPacket{
        ObjectID: 0,
        Channel:  0,
        From:     "",
        Text:     "",
    }
}

func ParseCreatureSayPacket(
    p *CreatureSayPacket, data []byte,
) error {
    reader := packet.NewReader(data)
    if err := expectPacketID(reader, CreatureSayPacketID); err != nil {
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
