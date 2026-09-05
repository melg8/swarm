// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const systemMessagePacketID = 0x7A

// Parameter types of the SystemMessage packet (SystemMessage.java of
// the Mobius server).
const (
	sysParamText        = 0
	sysParamIntNumber   = 1
	sysParamNpcName     = 2
	sysParamItemName    = 3
	sysParamSkillName   = 4
	sysParamZoneName    = 7
	sysParamPlayerName  = 12
	systemParamsCap     = 8
	sysParamUnknownTail = 8
)

// SystemMessageParam is one parameter of a system message: the type
// decides which value is valid (text or int).
type SystemMessageParam struct {
	Type int32
	Int  int32
	Text string
}

// SystemMessagePacket is a client chat notification: the message id
// maps to the client side text (npcdata.SystemMessageText) and the
// parameters substitute its $sN placeholders.
// Wire format (see SystemMessage.writeImpl): [opcode 0x7A]
// [messageId: 4][paramCount: 4] then per parameter [type: 4] followed
// by the typed value: text and player name parameters carry a UTF-16
// string, skill names two ints, zone names three ints, everything else
// one int.
type SystemMessagePacket struct {
	MessageID int32
	Params    []SystemMessageParam
}

// NewSystemMessagePacket creates a packet ready for parsing with a
// reusable parameter buffer.
func NewSystemMessagePacket() *SystemMessagePacket {
	return &SystemMessagePacket{
		MessageID: 0,
		Params:    make([]SystemMessageParam, 0, systemParamsCap),
	}
}

// readSystemMessageParam reads one typed parameter block.
func readSystemMessageParam(
	reader *packet.Reader, param *SystemMessageParam,
) error {
	param.Text = ""
	param.Int = 0
	paramType, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read param type: %w", err)
	}
	param.Type = paramType
	switch paramType {
	case sysParamText, sysParamPlayerName:
		text, err := reader.ReadStringFromUtf16Format()
		if err != nil {
			return fmt.Errorf("failed to read text param: %w", err)
		}
		param.Text = text
	case sysParamSkillName:
		if err := readInt32Fields(reader, &param.Int); err != nil {
			return fmt.Errorf("failed to read skill param: %w", err)
		}
		// Skip the skill level int.
		if err := reader.Skip(4); err != nil {
			return fmt.Errorf("failed to read skill param: %w", err)
		}
	case sysParamZoneName:
		if err := readInt32Fields(reader, &param.Int); err != nil {
			return fmt.Errorf("failed to read zone param: %w", err)
		}
		// Skip the y and z ints.
		if err := reader.Skip(sysParamUnknownTail); err != nil {
			return fmt.Errorf("failed to read zone param: %w", err)
		}
	default:
		if err := readInt32Fields(reader, &param.Int); err != nil {
			return fmt.Errorf("failed to read int param: %w", err)
		}
	}

	return nil
}

// ParseSystemMessagePacket reads the packet from payload bytes.
func ParseSystemMessagePacket(p *SystemMessagePacket, data []byte) error {
	reader := packet.NewReader(data)

	if err := expectPacketID(reader, systemMessagePacketID); err != nil {
		return err
	}
	if err := readInt32Fields(reader, &p.MessageID); err != nil {
		return fmt.Errorf("failed to read message id: %w", err)
	}
	count, err := reader.ReadInt32()
	if err != nil {
		return fmt.Errorf("failed to read param count: %w", err)
	}
	if count < 0 || count > systemParamsCap {
		return fmt.Errorf("implausible system message param count %d", count)
	}
	p.Params = p.Params[:0]
	for i := int32(0); i < count; i++ {
		p.Params = append(p.Params, SystemMessageParam{})
		if err := readSystemMessageParam(reader, &p.Params[i]); err != nil {
			return err
		}
	}

	return nil
}
