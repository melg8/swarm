// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	loginOkPacketID  = 0x03
	loginOkExtraInts = 5
)

// LoginOkPacket confirms successful account authentication.
// Wire format: [opcode 0x03][loginOkID1: 4][loginOkID2: 4][5 unused ints].
type LoginOkPacket struct {
	LoginOkID1 int32
	LoginOkID2 int32
}

// NewLoginOkPacket creates a zero valued login ok packet ready for
// parsing.
func NewLoginOkPacket() *LoginOkPacket {
	return &LoginOkPacket{LoginOkID1: 0, LoginOkID2: 0}
}

// ParseLoginOkPacket reads the packet from decrypted content bytes.
func ParseLoginOkPacket(p *LoginOkPacket, data []byte) error {
	reader := packet.NewReader(data)

	id, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if id != loginOkPacketID {
		return errors.New("invalid login ok packet id")
	}

	if p.LoginOkID1, err = reader.ReadInt32(); err != nil {
		return err
	}
	if p.LoginOkID2, err = reader.ReadInt32(); err != nil {
		return err
	}

	// Skip unused tail of the packet.
	if err := reader.Skip(loginOkExtraInts * 4); err != nil {
		return err
	}

	return nil
}
