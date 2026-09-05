// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const authLoginPacketID = 0x08

// AuthLogin authenticates the game connection with the session keys
// received from the login server.
// Wire format: [opcode 0x08][login: str][playOkID2: 4][playOkID1: 4]
// [loginOkID1: 4][loginOkID2: 4].
type AuthLogin struct {
	Login      string
	PlayOkID1  int32
	PlayOkID2  int32
	LoginOkID1 int32
	LoginOkID2 int32
}

func (p *AuthLogin) ToBytes(writer *packet.Writer) error {
	if p.Login == "" {
		return errors.New("login is empty")
	}

	if err := writer.WriteInt8(authLoginPacketID); err != nil {
		return err
	}
	if err := writer.WriteStringAsUtf16(p.Login); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.PlayOkID2); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.PlayOkID1); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.LoginOkID1); err != nil {
		return err
	}

	return writer.WriteInt32(p.LoginOkID2)
}
