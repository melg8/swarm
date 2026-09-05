// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"github.com/melg8/swarm/internal/swarm/helpers"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

type InitPacket struct {
	SessionID       int32
	ProtocolVersion int32
	RsaPublicKey    []byte
	GameGuard1      int32
	GameGuard2      int32
	GameGuard3      int32
	GameGuard4      int32
	BlowfishKey     []byte
}

// NewInitPacket creates a zero valued init packet ready for parsing.
func NewInitPacket() *InitPacket {
	return &InitPacket{
		SessionID:       0,
		ProtocolVersion: 0,
		RsaPublicKey:    nil,
		GameGuard1:      0,
		GameGuard2:      0,
		GameGuard3:      0,
		GameGuard4:      0,
		BlowfishKey:     nil,
	}
}

// global sentinel error to avoid allocation.
var errEOF = errors.New("EOF")

//go:nosplit
func ParseInitPacket(p *InitPacket, data []byte) error {
	const rsaSize = 128
	const minPacketSize = 4 + 4 + rsaSize + 4*4

	if len(data) < minPacketSize {
		return errEOF
	}

	p.SessionID = *(*int32)(unsafe.Pointer(&data[0]))
	p.ProtocolVersion = *(*int32)(unsafe.Pointer(&data[4]))
	p.RsaPublicKey = data[8 : 8+rsaSize]
	p.GameGuard1 = *(*int32)(unsafe.Pointer(&data[8+rsaSize]))
	p.GameGuard2 = *(*int32)(unsafe.Pointer(&data[12+rsaSize]))
	p.GameGuard3 = *(*int32)(unsafe.Pointer(&data[16+rsaSize]))
	p.GameGuard4 = *(*int32)(unsafe.Pointer(&data[20+rsaSize]))

	if len(data) > 24+rsaSize {
		p.BlowfishKey = data[24+rsaSize:]
	} else {
		p.BlowfishKey = nil
	}

	return nil
}

func (p *InitPacket) ToBytes(writer *packet.Writer) error { //nolint:cyclop
	if len(p.RsaPublicKey) != 128 {
		return fmt.Errorf("invalid RSA public key len: %d bytes, expected 128",
			len(p.RsaPublicKey))
	}
	if err := writer.WriteInt32(p.SessionID); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.ProtocolVersion); err != nil {
		return err
	}
	if err := writer.WriteBytes(p.RsaPublicKey); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.GameGuard1); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.GameGuard2); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.GameGuard3); err != nil {
		return err
	}
	if err := writer.WriteInt32(p.GameGuard4); err != nil {
		return err
	}
	if p.BlowfishKey != nil {
		if err := writer.WriteBytes(p.BlowfishKey); err != nil {
			return err
		}
	}

	return nil
}

func (p *InitPacket) WriteTo(dest []byte) (int, error) {
	requiredSize := 4 + 4 + 128 + 4*4 // Размер без ключа Blowfish
	if p.BlowfishKey != nil {
		requiredSize += len(p.BlowfishKey)
	}

	if len(dest) < requiredSize {
		return 0, fmt.Errorf("destination buffer is too small: got %d, want %d", len(dest), requiredSize)
	}

	offset := 0

	// Записываем int32 напрямую в слайс
	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.SessionID))
	offset += 4

	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.ProtocolVersion))
	offset += 4

	// Копируем данные ключа. Это не вызывает аллокаций.
	copy(dest[offset:], p.RsaPublicKey)
	offset += len(p.RsaPublicKey)

	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.GameGuard1))
	offset += 4
	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.GameGuard2))
	offset += 4
	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.GameGuard3))
	offset += 4
	binary.LittleEndian.PutUint32(dest[offset:], uint32(p.GameGuard4))
	offset += 4

	if p.BlowfishKey != nil {
		copy(dest[offset:], p.BlowfishKey)
		offset += len(p.BlowfishKey)
	}

	return offset, nil
}

func (p *InitPacket) ToString() string {
	result := "\nInitPacket:" +
		"\n  SessionID: " + helpers.HexStringFromInt32(p.SessionID) +
		"\n  ProtocolVersion: " + helpers.HexStringFromInt32(p.ProtocolVersion) +
		"\n  RsaPublicKey: \n" + helpers.HexViewFromWithLineSplit(p.RsaPublicKey, 16, "    ") +
		"\n  GameGuard1: " + helpers.HexStringFromInt32(p.GameGuard1) +
		"\n  GameGuard2: " + helpers.HexStringFromInt32(p.GameGuard2) +
		"\n  GameGuard3: " + helpers.HexStringFromInt32(p.GameGuard3) +
		"\n  GameGuard4: " + helpers.HexStringFromInt32(p.GameGuard4)

	if p.BlowfishKey != nil {
		result += "\n  BlowfishKey: \n" + helpers.HexViewFromWithLineSplit(p.BlowfishKey, 16, "    ")
	} else {
		result += "\n  BlowfishKey: " + "nil"
	}

	return result
}
