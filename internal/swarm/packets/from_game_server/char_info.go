// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const charInfoPacketID = 0x03

// CharInfoPacket describes another player around the character.
// Wire format (see CharInfo.writeImpl): [opcode 0x03][x: 4][y: 4][z: 4]
// [vehicleId: 4][objectId: 4][name: str][race: 4][female: 4]
// [baseClass: 4][paperdoll and combat ints: 68][runSpd: 4][walkSpd: 4]
// [swim and fly speeds: 24][moveMultiplier: 8][attack speed multiplier,
// collision and hair fields: 36][title: str][clan and ally ids: 16]
// [relation: 4][7 state bytes: standing, running, inCombat, alikeDead,
// invisible, mountType, privateStore].
//
// The transmitted runSpd and walkSpd are base values: the server divides
// the real speeds by the move multiplier before writing them (see
// CharInfo.writeImpl), so the effective speed is runSpd * moveMultiplier.
// CharInfo carries no heading; the server announces the facing of a
// standing player through the StartRotation and StopRotation packets (see
// Player.sendInfo).
type CharInfoPacket struct {
	ObjectID      int32
	Name          string
	Title         string
	Race          int32
	ClassID       int32
	RunSpeed      int32
	WalkSpeed     int32
	MoveSpeedMult float64
	Running       bool
	InCombat      bool
	Dead          bool
	X             int32
	Y             int32
	Z             int32
}

// NewCharInfoPacket creates a zero valued packet ready for parsing.
func NewCharInfoPacket() *CharInfoPacket {
	return &CharInfoPacket{
		ObjectID:      0,
		Name:          "",
		Title:         "",
		Race:          0,
		ClassID:       0,
		RunSpeed:      0,
		WalkSpeed:     0,
		MoveSpeedMult: 0,
		Running:       false,
		InCombat:      false,
		Dead:          false,
		X:             0,
		Y:             0,
		Z:             0,
	}
}

// ParseCharInfoPacket reads the packet from payload bytes.
func ParseCharInfoPacket(p *CharInfoPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, charInfoPacketID); err != nil {
		return err
	}

	return parseCharIdentity(reader, p)
}

// parseCharIdentity reads the location and identity fields up to the
// base class, then the speeds, title and state flags.
func parseCharIdentity(reader *packet.Reader, p *CharInfoPacket) error {
	var err error
	if err = readInt32Fields(reader, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read char position: %w", err)
	}
	if p.Name, err = readCharName(reader, p); err != nil {
		return err
	}
	if err := readCharSpeeds(reader, p); err != nil {
		return err
	}
	if p.Title, err = reader.ReadStringFromUtf16Format(); err != nil {
		return fmt.Errorf("failed to read char title: %w", err)
	}
	readCharStateFlags(reader, p)

	return nil
}

// readCharName reads the object id, name and class fields.
func readCharName(reader *packet.Reader, p *CharInfoPacket) (string, error) {
	// Skip the vehicle id before the object id.
	if err := reader.Skip(4); err != nil {
		return "", fmt.Errorf("not enough bytes for vehicle id: %w", err)
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return "", fmt.Errorf("failed to read char object id: %w", err)
	}
	name, err := reader.ReadStringFromUtf16Format()
	if err != nil {
		return "", fmt.Errorf("failed to read char name: %w", err)
	}
	if err := readInt32Fields(reader, &p.Race); err != nil {
		return "", fmt.Errorf("failed to read char race: %w", err)
	}
	// Skip the female field before the base class.
	if err := reader.Skip(4); err != nil {
		return "", fmt.Errorf("not enough bytes for char fields: %w", err)
	}
	if err := readInt32Fields(reader, &p.ClassID); err != nil {
		return "", fmt.Errorf("failed to read char class: %w", err)
	}

	return name, nil
}

// readCharSpeeds reads the run and walk base speeds with the move
// multiplier behind the paperdoll and combat fields.
func readCharSpeeds(reader *packet.Reader, p *CharInfoPacket) error {
	// Skip 12 paperdoll ints, the attack speed pair, pvp flag and karma.
	if err := reader.Skip(charInfoSpeedLead); err != nil {
		return fmt.Errorf("not enough bytes for char speeds: %w", err)
	}
	if err := readInt32Fields(reader, &p.RunSpeed, &p.WalkSpeed); err != nil {
		return fmt.Errorf("failed to read char speeds: %w", err)
	}
	// Skip the two swim and the four fly speed ints (the fly pair is
	// written twice by the server).
	if err := reader.Skip(charInfoSpeedTrail); err != nil {
		return fmt.Errorf("not enough bytes for char speed trail: %w", err)
	}
	mult, err := reader.ReadFloat64()
	if err != nil {
		return fmt.Errorf("failed to read char move multiplier: %w", err)
	}
	p.MoveSpeedMult = mult
	// Skip the attack speed multiplier, the collision values and the hair
	// and face ints between the multiplier and the title string.
	if err := reader.Skip(charInfoAppearanceTail); err != nil {
		return fmt.Errorf("not enough bytes for char fields: %w", err)
	}

	return nil
}

// readCharStateFlags reads the standing, combat and dead flags behind the
// clan and ally ids. A truncated packet keeps the defaults.
func readCharStateFlags(reader *packet.Reader, p *CharInfoPacket) {
	if err := reader.Skip(charInfoClanTail); err != nil {
		return
	}
	var flags [7]int8
	for i := range flags {
		flag, err := reader.ReadInt8()
		if err != nil {
			return
		}
		flags[i] = flag
	}
	p.Running = flags[1] != 0
	p.InCombat = flags[2] != 0
	p.Dead = flags[3] != 0
}

// CharInfo skipped field groups of the wire format, counted in bytes.
const (
	// charInfoSpeedLead skips 12 paperdoll ints plus the attack speed
	// pair, pvp flag and karma between the base class and the run speed.
	charInfoSpeedLead = (12 + 2 + 2) * 4
	// charInfoSpeedTrail skips the two swim and the four fly speed ints
	// between the walk speed and the move multiplier (the fly pair is
	// written twice by the server).
	charInfoSpeedTrail = 24
	// charInfoAppearanceTail skips the attack speed multiplier, the
	// collision values and the hair and face ints between the move
	// multiplier and the title string.
	charInfoAppearanceTail = 8 + 8 + 8 + 3*4
	// charInfoClanTail skips the clan and ally ids plus the relation int
	// between the title string and the state flags.
	charInfoClanTail = 4 * 5
)
