// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package fromgameserver contains parsers for game server to client packets
// of the Mobius C1 protocol.
package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	keyPacketID      = 0x00
	keyPacketKeySize = crypt.GameCryptKeySize
)

// KeyPacket is the first packet sent by the game server. It is transferred
// unencrypted and enables the XOR cipher for the rest of the session.
// Wire format: [opcode 0x00][result: 1][key: 8][serverID: 4][1: 4].
type KeyPacket struct {
	Result   int8
	Key      [keyPacketKeySize]byte
	ServerID int32
}

// NewKeyPacket creates a zero valued key packet ready for parsing.
func NewKeyPacket() *KeyPacket {
	return &KeyPacket{Result: 0, Key: [keyPacketKeySize]byte{}, ServerID: 0}
}

// ParseKeyPacket reads the packet from payload bytes.
func ParseKeyPacket(p *KeyPacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, keyPacketID); err != nil {
		return err
	}

	var err error
	if p.Result, err = reader.ReadInt8(); err != nil {
		return err
	}
	if err := readKey(reader, &p.Key); err != nil {
		return err
	}
	if p.ServerID, err = reader.ReadInt32(); err != nil {
		return err
	}

	return expectKeyPacketTail(reader)
}

// readKey reads the session key bytes into the destination.
func readKey(reader *packet.Reader, dest *[keyPacketKeySize]byte) error {
	key, err := reader.ReadBytes(keyPacketKeySize)
	if err != nil {
		return err
	}
	copy(dest[:], key)

	return nil
}

// expectKeyPacketTail validates the constant tail of the key packet.
func expectKeyPacketTail(reader *packet.Reader) error {
	tail, err := reader.ReadBytes(4)
	if err != nil {
		return err
	}
	if tail[0] != 1 || tail[1] != 0 || tail[2] != 0 || tail[3] != 0 {
		return fmt.Errorf("unexpected key packet tail %v", tail)
	}

	return nil
}

// expectPacketID reads the packet id and verifies it matches the expected one.
func expectPacketID(reader *packet.Reader, id byte) error {
	actual, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if byte(actual) != id {
		return fmt.Errorf("invalid packet id 0x%02x, want 0x%02x",
			byte(actual), id)
	}

	return nil
}

// Ok reports whether the server accepted the protocol version.
func (p *KeyPacket) Ok() bool {
	return p.Result == 1
}
