// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Combat packets: Attack, AutoAttackStart and AutoAttackStop.

const (
	attackPacketID          = 0x06
	autoAttackStartPacketID = 0x3B
	autoAttackStopPacketID  = 0x3C
)

// attackHitsCapacity bounds the stored hits of one Attack packet. Extra
// hits are still parsed but only the first targets are kept.
const attackHitsCapacity = 4

// AttackHit is one hit of an Attack packet.
type AttackHit struct {
	TargetID int32
	Damage   int32
	Flags    int8
}

// AttackPacket announces one creature attacking another. The attacker
// position and the target position make chasing mobs trackable without
// MoveToLocation broadcasts.
// Wire format (see Attack.writeImpl): [opcode 0x06][attackerId: 4]
// [first hit: targetId: 4][damage: 4][flags: 1]
// [attackerX: 4][attackerY: 4][attackerZ: 4][hitsLeft: 2]
// [remaining hits: targetId: 4][damage: 4][flags: 1 each]
// [targetX: 4][targetY: 4][targetZ: 4].
type AttackPacket struct {
	AttackerID int32
	X          int32
	Y          int32
	Z          int32
	TargetX    int32
	TargetY    int32
	TargetZ    int32
	Hits       [attackHitsCapacity]AttackHit
	HitCount   int
}

// NewAttackPacket creates a zero valued packet ready for parsing.
func NewAttackPacket() *AttackPacket {
	return &AttackPacket{
		AttackerID: 0,
		X:          0,
		Y:          0,
		Z:          0,
		TargetX:    0,
		TargetY:    0,
		TargetZ:    0,
		Hits:       [attackHitsCapacity]AttackHit{},
		HitCount:   0,
	}
}

// ParseAttackPacket reads the packet from payload bytes. It resets the
// hit list first because the packet structs are reused between packets.
func ParseAttackPacket(p *AttackPacket, data []byte) error {
	reader := packet.NewReader(data)
	p.HitCount = 0

	if err := expectPacketID(reader, attackPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.AttackerID); err != nil {
		return fmt.Errorf("failed to read attacker id: %w", err)
	}
	if err := readAttackHit(reader, p); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.X, &p.Y, &p.Z); err != nil {
		return fmt.Errorf("failed to read attacker location: %w", err)
	}
	hitsLeft, err := reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read hit count: %w", err)
	}
	if hitsLeft < 0 || hitsLeft > 40 {
		return fmt.Errorf("implausible hit count %d", hitsLeft)
	}
	if err := readRemainingHits(reader, p, hitsLeft); err != nil {
		return err
	}
	if err := readInt32Fields(
		reader, &p.TargetX, &p.TargetY, &p.TargetZ); err != nil {
		return fmt.Errorf("failed to read target location: %w", err)
	}

	return nil
}

// readRemainingHits reads the hits behind the first one.
func readRemainingHits(
	reader *packet.Reader, p *AttackPacket, hits int16,
) error {
	for range hits {
		if err := readAttackHit(reader, p); err != nil {
			return err
		}
	}

	return nil
}

// readAttackHit reads one hit triple and stores it in the packet.
func readAttackHit(reader *packet.Reader, p *AttackPacket) error {
	var hit AttackHit
	if err := readInt32Fields(
		reader, &hit.TargetID, &hit.Damage); err != nil {
		return fmt.Errorf("failed to read hit: %w", err)
	}
	flags, err := reader.ReadInt8()
	if err != nil {
		return fmt.Errorf("failed to read hit flags: %w", err)
	}
	hit.Flags = flags
	if p.HitCount < attackHitsCapacity {
		p.Hits[p.HitCount] = hit
	}
	p.HitCount++

	return nil
}

// AutoAttackStartPacket announces that a creature started auto attacking.
// Wire format (see AutoAttackStart.writeImpl): [opcode 0x3B]
// [objectId: 4].
type AutoAttackStartPacket struct {
	ObjectID int32
}

// NewAutoAttackStartPacket creates a zero valued packet ready for parsing.
func NewAutoAttackStartPacket() *AutoAttackStartPacket {
	return &AutoAttackStartPacket{ObjectID: 0}
}

// ParseAutoAttackStartPacket reads the packet from payload bytes.
func ParseAutoAttackStartPacket(p *AutoAttackStartPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, autoAttackStartPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read object id: %w", err)
	}

	return nil
}

// AutoAttackStopPacket announces that a creature stopped auto attacking.
// Wire format (see AutoAttackStop.writeImpl): [opcode 0x3C]
// [objectId: 4].
type AutoAttackStopPacket struct {
	ObjectID int32
}

// NewAutoAttackStopPacket creates a zero valued packet ready for parsing.
func NewAutoAttackStopPacket() *AutoAttackStopPacket {
	return &AutoAttackStopPacket{ObjectID: 0}
}

// ParseAutoAttackStopPacket reads the packet from payload bytes.
func ParseAutoAttackStopPacket(p *AutoAttackStopPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, autoAttackStopPacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.ObjectID); err != nil {
		return fmt.Errorf("failed to read object id: %w", err)
	}

	return nil
}
