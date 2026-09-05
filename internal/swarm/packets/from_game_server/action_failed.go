// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const actionFailedPacketID = 0x35

// ActionFailedPacket is the one byte server answer to a refused
// request: a flood protector rejection, a click on a non interactable
// object, an action while casting or overloaded and so on. It carries
// no fields and no feedback about which request failed; the bot logs
// the occurrence and keeps driving its own retry logic.
// Wire format (see ActionFailed.writeImpl): [opcode 0x35].
type ActionFailedPacket struct{}

// NewActionFailedPacket creates a zero valued packet ready for parsing.
func NewActionFailedPacket() *ActionFailedPacket {
	return &ActionFailedPacket{}
}

// ParseActionFailedPacket validates the packet id.
func ParseActionFailedPacket(_ *ActionFailedPacket, data []byte) error {
	reader := packet.NewReader(data)

	return expectPacketID(reader, actionFailedPacketID)
}
