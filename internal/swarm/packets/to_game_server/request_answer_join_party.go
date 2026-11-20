// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestAnswerJoinPartyPacketID = 0x2A

// Answers of the RequestAnswerJoinParty packet.
const (
    // PartyAnswerDecline refuses the invitation.
    PartyAnswerDecline = 0
    // PartyAnswerAccept joins the party of the requestor.
    PartyAnswerAccept = 1
)

// RequestAnswerJoinParty answers a pending AskJoinParty invitation. The server
// answers the requestor with a JoinParty carrying the same response and, on an
// accept, adds the player to the party and broadcasts the small window. Wire
// format (see RequestAnswerJoinParty.readImpl): [opcode 0x2A][response: 4].
// Reference:
// org/l2jmobius/gameserver/network/clientpackets/RequestAnswerJoinParty.java.
type RequestAnswerJoinParty struct {
    Response int32
}

// NewRequestAnswerJoinParty creates an answer packet.
func NewRequestAnswerJoinParty() *RequestAnswerJoinParty {
    return &RequestAnswerJoinParty{Response: PartyAnswerDecline}
}

// ToBytes serializes the packet.
func (p *RequestAnswerJoinParty) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(requestAnswerJoinPartyPacketID); err != nil {
        return fmt.Errorf("failed to write answer join party id: %w", err)
    }
    if err := writer.WriteInt32(p.Response); err != nil {
        return fmt.Errorf("failed to write party answer: %w", err)
    }

    return nil
}
