// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const requestJoinPartyPacketID = 0x29

// PartyDistributionType ids of the C1 client packet: the finders keepers
// default is what a fresh party answers and what the bot asks for.
const (
    PartyDistributionFindersKeepers = 0
    PartyDistributionRandom         = 1
)

// RequestJoinParty invites a visible player into the party of the requestor:
// the server requires the requestor to be the party leader (or partyless, in
// which case the party is created on the acceptance) and the target to sit in
// the known list. The server answers with an AskJoinParty to the target and a
// JoinParty response to the requestor once the answer lands. Wire format (see
// RequestJoinParty.readImpl): [opcode 0x29][objectId: 4]
// [distributionType: 4]. Reference:
// org/l2jmobius/gameserver/network/clientpackets/RequestJoinParty.java.
type RequestJoinParty struct {
    ObjectID         int32
    DistributionType int32
}

// NewRequestJoinParty creates an invitation packet.
func NewRequestJoinParty() *RequestJoinParty {
    return &RequestJoinParty{
        ObjectID:         0,
        DistributionType: PartyDistributionFindersKeepers,
    }
}

// ToBytes serializes the packet.
func (p *RequestJoinParty) ToBytes(writer *packet.Writer) error {
    if err := writer.WriteInt8(requestJoinPartyPacketID); err != nil {
        return fmt.Errorf("failed to write join party id: %w", err)
    }
    if err := writer.WriteInt32(p.ObjectID); err != nil {
        return fmt.Errorf("failed to write join party target: %w", err)
    }
    if err := writer.WriteInt32(p.DistributionType); err != nil {
        return fmt.Errorf("failed to write party distribution type: %w", err)
    }

    return nil
}
