// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const charSelectInfoPacketID = 0x1F

const (
	// charInfoIntsAfterAccount skips sessionId, clan and builder fields.
	charInfoIntsAfterAccount = 3
	// charInfoGameServerName skips the game server name field.
	charInfoGameServerName = 1
	// charInfoIntsSpExp skips the sp and exp fields.
	charInfoIntsSpExp = 2
	// charInfoSkippedInts skips karma, deprecated zero and paperdoll fields.
	charInfoSkippedInts = 1 + 9 + 15*2
)

// CharacterInfo describes a single character of the account.
type CharacterInfo struct {
	Name        string
	ObjectID    int32
	Account     string
	SessionID   int32
	ClanID      int32
	Sex         int32
	Race        int32
	BaseClassID int32
	X           int32
	Y           int32
	Z           int32
	CurrentHP   float64
	CurrentMP   float64
	Level       int32
	HairStyle   int32
	HairColor   int32
	Face        int32
	MaxHP       float64
	MaxMP       float64
	DeleteTimer int32
}

// CharSelectInfoPacket lists the characters available for selection.
// Wire format: [opcode 0x1F][count: 4] then per character a variable layout
// of strings, ints, doubles and paperdoll tables.
type CharSelectInfoPacket struct {
	Count      int32
	Characters []CharacterInfo
}

// NewCharSelectInfoPacket creates an empty character list packet.
func NewCharSelectInfoPacket() *CharSelectInfoPacket {
	return &CharSelectInfoPacket{Count: 0, Characters: nil}
}

// skipInts consumes n 4 byte integers from the reader.
func skipInts(reader *packet.Reader, n int) error {
	if err := reader.Skip(n * 4); err != nil {
		return fmt.Errorf("not enough bytes for character fields: %w", err)
	}

	return nil
}

// readInt32Fields reads consecutive int32 values into the given fields.
func readInt32Fields(reader *packet.Reader, fields ...*int32) error {
	for _, field := range fields {
		value, err := reader.ReadInt32()
		if err != nil {
			return err
		}
		*field = value
	}

	return nil
}

// parseCharacter reads a single character entry from the reader.
func parseCharacter(reader *packet.Reader, info *CharacterInfo) error {
	if err := parseCharacterBase(reader, info); err != nil {
		return err
	}
	if err := parseCharacterProgression(reader, info); err != nil {
		return err
	}

	return parseCharacterResources(reader, info)
}

// parseCharacterBase reads the identity, class and location fields.
func parseCharacterBase(reader *packet.Reader, info *CharacterInfo) error {
	var err error
	if info.Name, err = reader.ReadStringFromUtf16Format(); err != nil {
		return err
	}
	if info.ObjectID, err = reader.ReadInt32(); err != nil {
		return err
	}
	if info.Account, err = reader.ReadStringFromUtf16Format(); err != nil {
		return err
	}

	return parseCharacterPosition(reader, info)
}

// parseCharacterPosition reads the class and location fields of a character.
func parseCharacterPosition(reader *packet.Reader, info *CharacterInfo) error {
	// Skip sessionId, clanId and builder level fields.
	if err := skipInts(reader, charInfoIntsAfterAccount); err != nil {
		return err
	}
	if err := readInt32Fields(reader,
		&info.Sex, &info.Race, &info.BaseClassID); err != nil {
		return err
	}
	if err := skipInts(reader, charInfoGameServerName); err != nil {
		return err
	}

	return readInt32Fields(reader, &info.X, &info.Y, &info.Z)
}

// parseCharacterCondition reads the current hp and mp fields of a character.
func parseCharacterCondition(reader *packet.Reader, info *CharacterInfo) error {
	var err error
	if info.CurrentHP, err = reader.ReadFloat64(); err != nil {
		return err
	}
	if info.CurrentMP, err = reader.ReadFloat64(); err != nil {
		return err
	}

	return nil
}

// parseCharacterProgression reads the vitals, level and appearance fields.
// The karma, deprecated zero and paperdoll fields are skipped.
func parseCharacterProgression(
	reader *packet.Reader, info *CharacterInfo,
) error {
	if err := parseCharacterCondition(reader, info); err != nil {
		return err
	}
	if err := parseCharacterLevel(reader, info); err != nil {
		return err
	}
	// Skip karma, deprecated zero and paperdoll fields.
	if err := skipInts(reader, charInfoSkippedInts); err != nil {
		return err
	}

	return readInt32Fields(reader, &info.HairStyle, &info.HairColor, &info.Face)
}

// parseCharacterLevel reads the level field of a character.
func parseCharacterLevel(reader *packet.Reader, info *CharacterInfo) error {
	// Skip sp and exp fields.
	if err := skipInts(reader, charInfoIntsSpExp); err != nil {
		return err
	}

	return readInt32Fields(reader, &info.Level)
}

// parseCharacterResources reads the max hp, max mp and delete timer fields.
func parseCharacterResources(reader *packet.Reader, info *CharacterInfo) error {
	var err error
	if info.MaxHP, err = reader.ReadFloat64(); err != nil {
		return err
	}
	if info.MaxMP, err = reader.ReadFloat64(); err != nil {
		return err
	}
	if info.DeleteTimer, err = reader.ReadInt32(); err != nil {
		return err
	}

	return nil
}

// ParseCharSelectInfoPacket reads the packet from payload bytes.
func ParseCharSelectInfoPacket(p *CharSelectInfoPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charSelectInfoPacketID); err != nil {
		return err
	}

	var err error
	if p.Count, err = reader.ReadInt32(); err != nil {
		return err
	}
	count := int(p.Count)
	if count < 0 || count > 128 {
		return errors.New("invalid character count")
	}
	if cap(p.Characters) < count {
		p.Characters = make([]CharacterInfo, 0, count)
	}
	p.Characters = p.Characters[:0]

	for range count {
		var info CharacterInfo
		if err := parseCharacter(reader, &info); err != nil {
			return err
		}
		p.Characters = append(p.Characters, info)
	}

	return nil
}

// FindCharacterByName returns the index and info of the character with the
// given name or nil when there is no such character.
func (p *CharSelectInfoPacket) FindCharacterByName(
	name string,
) (int, *CharacterInfo, bool) {
	for i := range p.Characters {
		if p.Characters[i].Name == name {
			return i, &p.Characters[i], true
		}
	}

	return 0, nil, false
}
