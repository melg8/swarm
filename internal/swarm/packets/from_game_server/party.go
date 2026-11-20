// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

// Party packets: the invitation ask, the answer echo and the small window the
// party members and their positions arrive through. The layouts follow the
// Mobius C1 server packets (AskJoinParty, JoinParty, PartySmallWindowAll,
// PartySmallWindowAdd, PartySmallWindowDeleteAll, PartySmallWindowDelete,
// PartySmallWindowUpdate, PartyMemberPosition).

const (
    AskJoinPartyPacketID              = 0x4B
    JoinPartyPacketID                 = 0x3C
    PartySmallWindowAllPacketID       = 0x63
    PartySmallWindowAddPacketID       = 0x64
    PartySmallWindowDeleteAllPacketID = 0x65
    PartySmallWindowDeletePacketID    = 0x66
    PartySmallWindowUpdatePacketID    = 0x67
    PartyMemberPositionPacketID       = 0xC0
)

// partyMembersCapacity bounds the stored party members: the server caps a
// party at nine players.
const partyMembersCapacity = 9

// PartyMember is one member of the party small window. The fields valid
// depend on the packet: the window packets carry the vitals and the class,
// the position packet the level and the world placement.
type PartyMember struct {
    ObjectID int32
    Name     string
    HP       int32
    MaxHP    int32
    MP       int32
    MaxMP    int32
    ClassID  int32
    Level    int32
    X        int32
    Y        int32
    Z        int32
}

// AskJoinPartyPacket tells the client that a player invited it into a party.
// Wire format (see AskJoinParty.writeImpl): [opcode 0x4B]
// [requestorObjectId: 4][distributionType: 4].
type AskJoinPartyPacket struct {
    RequestorObjectID int32
    DistributionType  int32
}

// NewAskJoinPartyPacket creates the reusable ask struct.
func NewAskJoinPartyPacket() *AskJoinPartyPacket {
    return &AskJoinPartyPacket{
        RequestorObjectID: 0,
        DistributionType:  0,
    }
}

// ParseAskJoinPartyPacket reads the invitation ask.
func ParseAskJoinPartyPacket(p *AskJoinPartyPacket, data []byte) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, AskJoinPartyPacketID); err != nil {
        return err
    }
    if err := readInt32Fields(reader, &p.RequestorObjectID,
        &p.DistributionType); err != nil {
        return fmt.Errorf("failed to read join party ask: %w", err)
    }

    return nil
}

// JoinPartyPacket echoes the answer of an invitation to the requestor: the
// response repeats the invited player's answer (-1 refuses every invitation,
// 0 declines, 1 accepts). Wire format (see JoinParty.writeImpl): [opcode
// 0x3C][response: 4].
type JoinPartyPacket struct {
    Response int32
}

// NewJoinPartyPacket creates the reusable answer struct.
func NewJoinPartyPacket() *JoinPartyPacket {
    return &JoinPartyPacket{Response: 0}
}

// ParseJoinPartyPacket reads the invitation answer.
func ParseJoinPartyPacket(p *JoinPartyPacket, data []byte) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, JoinPartyPacketID); err != nil {
        return err
    }
    if err := readInt32Fields(reader, &p.Response); err != nil {
        return fmt.Errorf("failed to read join party answer: %w", err)
    }

    return nil
}

// PartySmallWindowAllPacket carries every other member of the party; the
// server writes the member count as members-1 (the self member is excluded).
// Wire format (see PartySmallWindowAll.writeImpl): [opcode 0x63][count: 4]
// then per member [objectId: 4][name][curHP: 4][maxHP: 4][curMP: 4][maxMP: 4]
// [classId: 4][0: 4][race: 4].
type PartySmallWindowAllPacket struct {
    Members []PartyMember
}

// NewPartySmallWindowAllPacket creates the packet with a reusable slice.
func NewPartySmallWindowAllPacket() *PartySmallWindowAllPacket {
    return &PartySmallWindowAllPacket{
        Members: make([]PartyMember, 0, partyMembersCapacity),
    }
}

// ParsePartySmallWindowAllPacket reads the whole window.
func ParsePartySmallWindowAllPacket(
    p *PartySmallWindowAllPacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, PartySmallWindowAllPacketID); err != nil {
        return err
    }
    count, err := reader.ReadInt32()
    if err != nil {
        return fmt.Errorf("failed to read party member count: %w", err)
    }
    if count < 0 || count >= partyMembersCapacity {
        return fmt.Errorf("implausible party member count %d", count)
    }
    p.Members = p.Members[:0]
    for range count {
        member, readErr := readPartyWindowMember(reader)
        if readErr != nil {
            return readErr
        }
        // The window entry trails the class with the unread flag and the race
        // (PartySmallWindowAll.writeImpl).
        if err := reader.Skip(8); err != nil {
            return fmt.Errorf("not enough bytes for party window tail: %w", err)
        }
        p.Members = append(p.Members, member)
    }

    return nil
}

// PartySmallWindowAddPacket carries a member that just joined. Wire format
// (see PartySmallWindowAdd.writeImpl): [opcode 0x64][objectId: 4][name]
// [curHP: 4][maxHP: 4][curMP: 4][maxMP: 4][classId: 4][level: 4][0: 4][0: 4].
type PartySmallWindowAddPacket struct {
    Member PartyMember
}

// NewPartySmallWindowAddPacket creates the reusable add struct.
func NewPartySmallWindowAddPacket() *PartySmallWindowAddPacket {
    return &PartySmallWindowAddPacket{Member: PartyMember{}}
}

// ParsePartySmallWindowAddPacket reads the joined member.
func ParsePartySmallWindowAddPacket(
    p *PartySmallWindowAddPacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, PartySmallWindowAddPacketID); err != nil {
        return err
    }
    member, err := readPartyWindowMember(reader)
    if err != nil {
        return err
    }
    level, err := reader.ReadInt32()
    if err != nil {
        return fmt.Errorf("failed to read party member level: %w", err)
    }
    member.Level = level
    if err := reader.Skip(8); err != nil {
        return fmt.Errorf("not enough bytes for party member tail: %w", err)
    }
    p.Member = member

    return nil
}

// PartySmallWindowUpdatePacket carries a member whose vitals changed. Wire
// format (see PartySmallWindowUpdate.writeImpl): [opcode 0x67][objectId: 4]
// [name][curHP: 4][maxHP: 4][curMP: 4][maxMP: 4][classId: 4][level: 4].
type PartySmallWindowUpdatePacket struct {
    Member PartyMember
}

// NewPartySmallWindowUpdatePacket creates the reusable update struct.
func NewPartySmallWindowUpdatePacket() *PartySmallWindowUpdatePacket {
    return &PartySmallWindowUpdatePacket{Member: PartyMember{}}
}

// ParsePartySmallWindowUpdatePacket reads the updated member.
func ParsePartySmallWindowUpdatePacket(
    p *PartySmallWindowUpdatePacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader,
        PartySmallWindowUpdatePacketID); err != nil {
        return err
    }
    member, err := readPartyWindowMember(reader)
    if err != nil {
        return err
    }
    level, err := reader.ReadInt32()
    if err != nil {
        return fmt.Errorf("failed to read party member level: %w", err)
    }
    member.Level = level
    p.Member = member

    return nil
}

// PartySmallWindowDeleteAllPacket ends the party for the client: it carries no
// body. Wire format (see PartySmallWindowDeleteAll.writeImpl): [opcode 0x65].
type PartySmallWindowDeleteAllPacket struct{}

// NewPartySmallWindowDeleteAllPacket creates the packet.
func NewPartySmallWindowDeleteAllPacket() *PartySmallWindowDeleteAllPacket {
    return &PartySmallWindowDeleteAllPacket{}
}

// ParsePartySmallWindowDeleteAllPacket validates the packet id.
func ParsePartySmallWindowDeleteAllPacket(
    _ *PartySmallWindowDeleteAllPacket, data []byte,
) error {
    reader := packet.NewReader(data)

    return expectPacketID(reader, PartySmallWindowDeleteAllPacketID)
}

// PartySmallWindowDeletePacket removes one member of the window. Wire format
// (see PartySmallWindowDelete.writeImpl): [opcode 0x66][objectId: 4][name].
type PartySmallWindowDeletePacket struct {
    ObjectID int32
    Name     string
}

// NewPartySmallWindowDeletePacket creates the reusable delete struct.
func NewPartySmallWindowDeletePacket() *PartySmallWindowDeletePacket {
    return &PartySmallWindowDeletePacket{ObjectID: 0, Name: ""}
}

// ParsePartySmallWindowDeletePacket reads the removed member.
func ParsePartySmallWindowDeletePacket(
    p *PartySmallWindowDeletePacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader,
        PartySmallWindowDeletePacketID); err != nil {
        return err
    }
    if err := readInt32Fields(reader, &p.ObjectID); err != nil {
        return fmt.Errorf("failed to read party delete member: %w", err)
    }
    name, err := reader.ReadStringFromUtf16Format()
    if err != nil {
        return fmt.Errorf("failed to read party delete name: %w", err)
    }
    p.Name = name

    return nil
}

// PartyMemberPositionPacket carries the party members and their placements,
// including the self member. Wire format (see PartyMemberPosition.writeImpl):
// [opcode 0xC0][count: 4] then per member [objectId: 4][name][level: 4]
// [classId: 4][0: 4][clanId: 4][0: 4][x: 4][y: 4][z: 4].
type PartyMemberPositionPacket struct {
    Members []PartyMember
}

// NewPartyMemberPositionPacket creates the packet with a reusable slice.
func NewPartyMemberPositionPacket() *PartyMemberPositionPacket {
    return &PartyMemberPositionPacket{
        Members: make([]PartyMember, 0, partyMembersCapacity),
    }
}

// ParsePartyMemberPositionPacket reads the member placements.
func ParsePartyMemberPositionPacket(
    p *PartyMemberPositionPacket, data []byte,
) error {
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, PartyMemberPositionPacketID); err != nil {
        return err
    }
    count, err := reader.ReadInt32()
    if err != nil {
        return fmt.Errorf("failed to read party position count: %w", err)
    }
    if count < 0 || count > partyMembersCapacity {
        return fmt.Errorf("implausible party position count %d", count)
    }
    p.Members = p.Members[:0]
    for range count {
        member, readErr := readPartyPositionMember(reader)
        if readErr != nil {
            return readErr
        }
        p.Members = append(p.Members, member)
    }

    return nil
}

// readPartyWindowMember reads the common small window head: the object id, the
// name and the four vitals plus the class. It leaves the level trailing.
func readPartyWindowMember(reader *packet.Reader) (PartyMember, error) {
    var member PartyMember
    if err := readInt32Fields(reader, &member.ObjectID); err != nil {
        return member, fmt.Errorf("failed to read party member id: %w", err)
    }
    name, err := reader.ReadStringFromUtf16Format()
    if err != nil {
        return member, fmt.Errorf("failed to read party member name: %w", err)
    }
    member.Name = name
    if err := readInt32Fields(reader, &member.HP, &member.MaxHP,
        &member.MP, &member.MaxMP, &member.ClassID); err != nil {
        return member, fmt.Errorf("failed to read party member vitals: %w", err)
    }

    return member, nil
}

// readPartyPositionMember reads one PartyMemberPosition entry.
func readPartyPositionMember(reader *packet.Reader) (PartyMember, error) {
    var member PartyMember
    if err := readInt32Fields(reader, &member.ObjectID); err != nil {
        return member, fmt.Errorf("failed to read party position id: %w", err)
    }
    name, err := reader.ReadStringFromUtf16Format()
    if err != nil {
        return member, fmt.Errorf("failed to read party position name: %w", err)
    }
    member.Name = name
    if err := readInt32Fields(reader, &member.Level,
        &member.ClassID); err != nil {
        return member, fmt.Errorf(
            "failed to read party position class: %w", err)
    }
    // The placement trails the class with the unread flag, the clan id and
    // the unread clan flag (PartyMemberPosition.writeImpl).
    if err := reader.Skip(12); err != nil {
        return member, fmt.Errorf(
            "not enough bytes for party position: %w", err)
    }
    if err := readInt32Fields(reader, &member.X, &member.Y,
        &member.Z); err != nil {
        return member, fmt.Errorf("failed to read party position: %w", err)
    }

    return member, nil
}
