// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const NetPingPacketID byte = 0xEC

// NetPingPacket is the server response to a client ping request.
// Wire format: [opcode 0xEC][gameTime: 4].
type NetPingPacket struct {
    GameTime int32
}

func NewNetPingPacket() *NetPingPacket {
    return &NetPingPacket{GameTime: 0}
}

func ParseNetPingPacket(p *NetPingPacket, data []byte) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, NetPingPacketID); err != nil {
        return err
    }

    var err error
    if p.GameTime, err = reader.ReadInt32(); err != nil {
        return err
    }

    return nil
}
