// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const userInfoPacketID = 0x04

// UserInfo skipped field groups of the wire format, counted in bytes: the
// weapon flag with both paperdoll blocks and the combat stats between the
// load fields and the run speed, and the swim and fly speeds between the
// walk speed and the move multiplier.
const (
	userInfoStatsSkip  = 4 + 15*4 + 15*4 + 12*4
	userInfoSpeedTrail = 24
)

// UserInfoPacket carries the state of the played character.
// Wire format (see UserInfo.writeImpl): [opcode 0x04][x: 4][y: 4][z: 4]
// [vehicleId: 4][objectId: 4][name: str][race: 4][female: 4][baseClass: 4]
// [level: 4][exp: 4][STR: 4][DEX: 4][CON: 4][INT: 4][WIT: 4][MEN: 4]
// [maxHp: 4][curHp: 4][maxMp: 4][curMp: 4][sp: 4][curLoad: 4][maxLoad: 4]
// [weaponFlag: 4][15 paperdoll object ids][15 paperdoll display ids]
// [12 combat stat ints][runSpd: 4][walkSpd: 4][swim and fly speeds: 24]
// [moveMultiplier: 8] followed by appearance fields.
//
// The transmitted runSpd and walkSpd are base values: the server divides
// the real speeds by the move multiplier before writing them (see
// UserInfo.writeImpl), so the effective speed is runSpd * moveMultiplier.
type UserInfoPacket struct {
	ObjectID      int32
	Name          string
	Race          int32
	ClassID       int32
	Level         int32
	Exp           int32
	Sp            int32
	STR           int32
	DEX           int32
	CON           int32
	INT           int32
	WIT           int32
	MEN           int32
	MaxHP         int32
	CurHP         int32
	MaxMP         int32
	CurMP         int32
	CurrentLoad   int32
	MaxLoad       int32
	RunSpeed      int32
	WalkSpeed     int32
	MoveSpeedMult float64
	X             int32
	Y             int32
	Z             int32
}

// NewUserInfoPacket creates a zero valued packet ready for parsing.
func NewUserInfoPacket() *UserInfoPacket {
	return &UserInfoPacket{
		ObjectID:      0,
		Name:          "",
		Race:          0,
		ClassID:       0,
		Level:         0,
		Exp:           0,
		Sp:            0,
		STR:           0,
		DEX:           0,
		CON:           0,
		INT:           0,
		WIT:           0,
		MEN:           0,
		MaxHP:         0,
		CurHP:         0,
		MaxMP:         0,
		CurMP:         0,
		CurrentLoad:   0,
		MaxLoad:       0,
		RunSpeed:      0,
		WalkSpeed:     0,
		MoveSpeedMult: 0,
		X:             0,
		Y:             0,
		Z:             0,
	}
}

// ParseUserInfoPacket reads the packet from payload bytes.
func ParseUserInfoPacket(p *UserInfoPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, userInfoPacketID); err != nil {
		return err
	}

	if err := readInt32Fields(reader, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read user position: %w", err)
	}
	if err := reader.Skip(4); err != nil {
		return fmt.Errorf("not enough bytes for vehicle id: %w", err)
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read user object id: %w", err)
	}

	var err error
	if p.Name, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read user name: %w", err)
	}
	if err := readInt32Fields(reader, &p.Race); err != nil {
		return fmt.Errorf("failed to read user race: %w", err)
	}
	// Skip the female field.
	if err := reader.Skip(4); err != nil {
		return fmt.Errorf("not enough bytes for user fields: %w", err)
	}
	if err := readInt32Fields(reader,
		&p.ClassID, &p.Level, &p.Exp,
		&p.STR, &p.DEX, &p.CON, &p.INT, &p.WIT, &p.MEN,
		&p.MaxHP, &p.CurHP, &p.MaxMP, &p.CurMP); err != nil {
		return fmt.Errorf("failed to read user vitals: %w", err)
	}
	if err := readInt32Fields(reader,
		&p.Sp, &p.CurrentLoad, &p.MaxLoad); err != nil {
		return fmt.Errorf("failed to read user load: %w", err)
	}
	// Skip the weapon flag, the 15 paperdoll object ids, the 15 paperdoll
	// display ids and the 12 combat stat ints before the speeds.
	if err := reader.Skip(userInfoStatsSkip); err != nil {
		return fmt.Errorf("not enough bytes for user stats: %w", err)
	}
	if err := readInt32Fields(reader, &p.RunSpeed, &p.WalkSpeed); err != nil {
		return fmt.Errorf("failed to read user speeds: %w", err)
	}
	// Skip the two swim and the four fly speed ints (the fly pair is
	// written twice by the server).
	if err := reader.Skip(userInfoSpeedTrail); err != nil {
		return fmt.Errorf("not enough bytes for user speed trail: %w", err)
	}
	p.MoveSpeedMult, err = reader.ReadFloat64()
	if err != nil {
		return fmt.Errorf("failed to read user move multiplier: %w", err)
	}

	return nil
}
