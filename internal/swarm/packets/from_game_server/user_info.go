// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "fmt"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

const UserInfoPacketID = 0x04

// userInfoPaperdollSlots is the paperdoll object id block slot count (15, a C1
// duplicate of the right hand).
const userInfoPaperdollSlots = 15

// UserInfo skipped field groups of the wire format, counted in bytes: the
// weapon flag, the paperdoll display ids with the combat stats, and the swim
// and fly speeds.
const (
    userInfoWeaponFlagSkip = 4
    userInfoStatsSkip      = 15*4 + 12*4
    userInfoSpeedTrail     = 24
)

// UserInfoPacket carries the state of the played character; the wire layout and
// the base speed rule: docs/protocol_description.md.
type UserInfoPacket struct {
    ObjectID           int32
    Name               string
    Race               int32
    ClassID            int32
    Level              int32
    Exp                int32
    Sp                 int32
    STR                int32
    DEX                int32
    CON                int32
    INT                int32
    WIT                int32
    MEN                int32
    MaxHP              int32
    CurHP              int32
    MaxMP              int32
    CurMP              int32
    CurrentLoad        int32
    MaxLoad            int32
    PaperdollObjectIDs [userInfoPaperdollSlots]int32
    RunSpeed           int32
    WalkSpeed          int32
    MoveSpeedMult      float64
    X                  int32
    Y                  int32
    Z                  int32
}

func NewUserInfoPacket() *UserInfoPacket {
    return &UserInfoPacket{
        ObjectID:           0,
        Name:               "",
        Race:               0,
        ClassID:            0,
        Level:              0,
        Exp:                0,
        Sp:                 0,
        STR:                0,
        DEX:                0,
        CON:                0,
        INT:                0,
        WIT:                0,
        MEN:                0,
        MaxHP:              0,
        CurHP:              0,
        MaxMP:              0,
        CurMP:              0,
        CurrentLoad:        0,
        MaxLoad:            0,
        PaperdollObjectIDs: [userInfoPaperdollSlots]int32{},
        RunSpeed:           0,
        WalkSpeed:          0,
        MoveSpeedMult:      0,
        X:                  0,
        Y:                  0,
        Z:                  0,
    }
}

// ParseUserInfoPacket reads the packet from payload bytes (the linear field
// dispatch keeps the protocol readable).
func ParseUserInfoPacket(p *UserInfoPacket, data []byte) error { //nolint:cyclop
    reader := packet.NewReader(data)

    if err := expectPacketID(reader, UserInfoPacketID); err != nil {
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
    // Skip the weapon flag before the paperdoll block.
    if err := reader.Skip(userInfoWeaponFlagSkip); err != nil {
        return fmt.Errorf("not enough bytes for user stats: %w", err)
    }
    // The 15 paperdoll object ids tell the equipped item of every slot (the
    // inventory mask cannot tell the ear or finger slots apart).
    for index := range p.PaperdollObjectIDs {
        if p.PaperdollObjectIDs[index], err = reader.ReadInt32(); err != nil {
            return fmt.Errorf("failed to read paperdoll slot: %w", err)
        }
    }
    // Skip the 15 paperdoll display ids and the 12 combat stat ints.
    if err := reader.Skip(userInfoStatsSkip); err != nil {
        return fmt.Errorf("not enough bytes for user stats: %w", err)
    }
    if err := readInt32Fields(reader, &p.RunSpeed, &p.WalkSpeed); err != nil {
        return fmt.Errorf("failed to read user speeds: %w", err)
    }
    // Skip the two swim and the four fly speed ints (the fly pair twice).
    if err := reader.Skip(userInfoSpeedTrail); err != nil {
        return fmt.Errorf("not enough bytes for user speed trail: %w", err)
    }
    p.MoveSpeedMult, err = reader.ReadFloat64()
    if err != nil {
        return fmt.Errorf("failed to read user move multiplier: %w", err)
    }

    return nil
}
