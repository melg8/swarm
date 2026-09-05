// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const serverLoginPacketID = 0x02

// RequestServerLogin selects the game server to play on.
// Wire format: [opcode 0x02][loginOkID1: 4][loginOkID2: 4][serverID: 1].
type RequestServerLogin struct {
	LoginOkID1 int32
	LoginOkID2 int32
	ServerID   int8
}

func (p *RequestServerLogin) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(serverLoginPacketID); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.LoginOkID1); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.LoginOkID2); err != nil {
		return err
	}

	return writer.WriteInt8(p.ServerID)
}
