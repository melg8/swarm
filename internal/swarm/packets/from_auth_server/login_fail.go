// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const loginFailPacketID = 0x01

// LoginFailPacket reports a failed account login attempt.
// Wire format: [opcode 0x01][reason: 4].
type LoginFailPacket struct {
	Reason int32
}

// NewLoginFailPacket creates a zero valued login fail packet ready
// for parsing.
func NewLoginFailPacket() *LoginFailPacket {
	return &LoginFailPacket{Reason: 0}
}

// ParseLoginFailPacket reads the packet from decrypted content bytes.
func ParseLoginFailPacket(p *LoginFailPacket, data []byte) error {
	reader := packet.NewReader(data)

	id, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if id != loginFailPacketID {
		return errors.New("invalid login fail packet id")
	}

	if p.Reason, err = reader.ReadInt32(); err != nil {
		return err
	}

	return nil
}

// loginFailReasons maps known fail reason codes to descriptions.
var loginFailReasons = map[int32]string{
	0x00: "no message",
	0x01: "system error, login later",
	0x02: "user or password wrong",
	0x04: "access failed, try again later",
	0x05: "account info incorrect, contact support",
	0x06: "not authed",
	0x07: "account in use",
	0x0F: "server overloaded",
	0x10: "server maintenance",
}

// ReasonText returns a human readable representation of the fail reason.
func (p *LoginFailPacket) ReasonText() string {
	if text, ok := loginFailReasons[p.Reason]; ok {
		return text
	}

	return fmt.Sprintf("unknown reason %d", p.Reason)
}
