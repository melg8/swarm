// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const serverListPacketID = 0x05

// RequestServerList asks the login server for the game server list.
// Wire format: [opcode 0x05][loginOkID1: 4][loginOkID2: 4][flag: 1].
type RequestServerList struct {
	LoginOkID1 int32
	LoginOkID2 int32
	Flag       int8
}

// NewRequestServerList creates the packet with the default client flag.
func NewRequestServerList(loginOkID1, loginOkID2 int32) *RequestServerList {
	return &RequestServerList{
		LoginOkID1: loginOkID1,
		LoginOkID2: loginOkID2,
		Flag:       1,
	}
}

func (p *RequestServerList) ToBytes(writer *packet.Writer) error {
	if err := writer.WriteInt8(serverListPacketID); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.LoginOkID1); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.LoginOkID2); err != nil {
		return err
	}

	return writer.WriteInt8(p.Flag)
}
