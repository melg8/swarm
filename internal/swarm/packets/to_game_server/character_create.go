// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const characterCreatePacketID = 0x0B

// Character stats field indexes for the creation packet.
const (
	charCreateMaxNameLength = 16
	charCreateFieldsCount   = 12
)

// CharacterCreate requests creation of a new character.
// Wire format: [opcode 0x0B][name: str][race: 4][female: 4][classID: 4]
// [int, str, con, men, dex, wit: 4 each][hairStyle: 4][hairColor: 4][face: 4].
type CharacterCreate struct {
	Name      string
	Race      int32
	Female    int32
	ClassID   int32
	INT       int32
	STR       int32
	CON       int32
	MEN       int32
	DEX       int32
	WIT       int32
	HairStyle int32
	HairColor int32
	Face      int32
}

func (p *CharacterCreate) ToBytes(writer *packet.Writer) error {
	if p.Name == "" {
		return errors.New("character name is empty")
	}
	if len(p.Name) > charCreateMaxNameLength {
		return fmt.Errorf("character name %q exceeds %d characters",
			p.Name, charCreateMaxNameLength)
	}

	if err := writer.WriteInt8(characterCreatePacketID); err != nil {
		return err
	}
	if err := writer.WriteStringAsUtf16(p.Name); err != nil {
		return err
	}

	fields := [charCreateFieldsCount]int32{
		p.Race,
		p.Female,
		p.ClassID,
		p.INT,
		p.STR,
		p.CON,
		p.MEN,
		p.DEX,
		p.WIT,
		p.HairStyle,
		p.HairColor,
		p.Face,
	}
	for _, field := range fields {
		if err := writer.WriteInt32(field); err != nil {
			return err
		}
	}

	return nil
}
