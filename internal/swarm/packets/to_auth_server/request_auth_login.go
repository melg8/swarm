// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"errors"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	authLoginPacketID = 0x00
	authFieldSize     = 14
)

// RequestAuthLogin is the first client packet of the Mobius login protocol.
// Wire format: [opcode 0x00][account: 14 bytes zero padded]
// [password: 14 bytes zero padded] followed by protocol filler and checksum.
type RequestAuthLogin struct {
	Account  string
	Password string
}

// validateField checks that a login or password fits the 14 byte field.
func validateField(value string) error {
	if len(value) == 0 {
		return errors.New("field is empty")
	}
	if len(value) > authFieldSize {
		return errors.New("field is longer than 14 bytes")
	}

	return nil
}

// writeField writes a fixed size zero padded ASCII field.
func writeField(writer *packet.Writer, value string) error {
	field := [authFieldSize]byte{}
	copy(field[:], value)
	if err := writer.WriteBytes(field[:]); err != nil {
		return err
	}

	return nil
}

func (p *RequestAuthLogin) ToBytes(writer *packet.Writer) error {
	if err := validateField(p.Account); err != nil {
		return errors.New("invalid account: " + err.Error())
	}
	if err := validateField(p.Password); err != nil {
		return errors.New("invalid password: " + err.Error())
	}

	if err := writer.WriteInt8(authLoginPacketID); err != nil {
		return err
	}
	if err := writeField(writer, p.Account); err != nil {
		return err
	}

	return writeField(writer, p.Password)
}
