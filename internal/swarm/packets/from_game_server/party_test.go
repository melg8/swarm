// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// writePartyWindowMember writes one small window entry.
func writePartyWindowMember(
    t *testing.T, writer *packet.Writer, objectID int32, name string,
) {
    t.Helper()
    require.NoError(t, writer.WriteInt32(objectID))
    require.NoError(t, writer.WriteStringAsUtf16(name))
    require.NoError(t, writer.WriteInt32(100))
    require.NoError(t, writer.WriteInt32(200))
    require.NoError(t, writer.WriteInt32(30))
    require.NoError(t, writer.WriteInt32(60))
    require.NoError(t, writer.WriteInt32(18))
}

// TestParsePartySmallWindowAll pins the whole window: the count excludes the
// self member and every entry carries the vitals and the class.
func TestParsePartySmallWindowAll(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowAllPacketID))
    require.NoError(t, writer.WriteInt32(2))
    writePartyWindowMember(t, writer, 2, "test2")
    require.NoError(t, writer.WriteInt32(1))
    require.NoError(t, writer.WriteInt32(1))
    writePartyWindowMember(t, writer, 3, "test3")
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(1))

    p := NewPartySmallWindowAllPacket()
    require.NoError(t, ParsePartySmallWindowAllPacket(p, writer.Bytes()))
    require.Len(t, p.Members, 2)
    require.Equal(t, int32(2), p.Members[0].ObjectID)
    require.Equal(t, "test2", p.Members[0].Name)
    require.Equal(t, int32(200), p.Members[0].MaxHP)
    require.Equal(t, int32(18), p.Members[0].ClassID)
    require.Equal(t, "test3", p.Members[1].Name)
}

// TestParsePartySmallWindowAllRejectsAnImplausibleCount pins the cap.
func TestParsePartySmallWindowAllRejectsAnImplausibleCount(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowAllPacketID))
    require.NoError(t, writer.WriteInt32(12))

    p := NewPartySmallWindowAllPacket()
    require.Error(t, ParsePartySmallWindowAllPacket(p, writer.Bytes()))
}

// TestParsePartySmallWindowAdd pins the joined member entry.
func TestParsePartySmallWindowAdd(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowAddPacketID))
    writePartyWindowMember(t, writer, 4, "test4")
    require.NoError(t, writer.WriteInt32(9))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(1))

    p := NewPartySmallWindowAddPacket()
    require.NoError(t, ParsePartySmallWindowAddPacket(p, writer.Bytes()))
    require.Equal(t, int32(4), p.Member.ObjectID)
    require.Equal(t, "test4", p.Member.Name)
    require.Equal(t, int32(9), p.Member.Level)
}

// TestParsePartySmallWindowUpdate pins the vitals refresh entry.
func TestParsePartySmallWindowUpdate(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowUpdatePacketID))
    writePartyWindowMember(t, writer, 2, "test2")
    require.NoError(t, writer.WriteInt32(10))

    p := NewPartySmallWindowUpdatePacket()
    require.NoError(t, ParsePartySmallWindowUpdatePacket(p, writer.Bytes()))
    require.Equal(t, int32(2), p.Member.ObjectID)
    require.Equal(t, int32(100), p.Member.HP)
    require.Equal(t, int32(10), p.Member.Level)
}

// TestParsePartySmallWindowDelete pins the removed member entry.
func TestParsePartySmallWindowDelete(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowDeletePacketID))
    require.NoError(t, writer.WriteInt32(3))
    require.NoError(t, writer.WriteStringAsUtf16("test3"))

    p := NewPartySmallWindowDeletePacket()
    require.NoError(t, ParsePartySmallWindowDeletePacket(p, writer.Bytes()))
    require.Equal(t, int32(3), p.ObjectID)
    require.Equal(t, "test3", p.Name)
}

// TestParsePartySmallWindowDeleteAll pins the empty end packet.
func TestParsePartySmallWindowDeleteAll(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(PartySmallWindowDeleteAllPacketID))

    p := NewPartySmallWindowDeleteAllPacket()
    require.NoError(t, ParsePartySmallWindowDeleteAllPacket(p, writer.Bytes()))
}

// TestParsePartyMemberPosition pins the placements: every member entry carries
// the level, the class and the world point.
func TestParsePartyMemberPosition(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteBytes([]byte{PartyMemberPositionPacketID}))
    require.NoError(t, writer.WriteInt32(2))
    require.NoError(t, writer.WriteInt32(100))
    require.NoError(t, writer.WriteStringAsUtf16("test1"))
    require.NoError(t, writer.WriteInt32(12))
    require.NoError(t, writer.WriteInt32(18))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(45000))
    require.NoError(t, writer.WriteInt32(50000))
    require.NoError(t, writer.WriteInt32(-3500))
    require.NoError(t, writer.WriteInt32(200))
    require.NoError(t, writer.WriteStringAsUtf16("test2"))
    require.NoError(t, writer.WriteInt32(11))
    require.NoError(t, writer.WriteInt32(18))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(0))
    require.NoError(t, writer.WriteInt32(44900))
    require.NoError(t, writer.WriteInt32(50010))
    require.NoError(t, writer.WriteInt32(-3500))

    p := NewPartyMemberPositionPacket()
    require.NoError(t, ParsePartyMemberPositionPacket(p, writer.Bytes()))
    require.Len(t, p.Members, 2)
    require.Equal(t, int32(100), p.Members[0].ObjectID)
    require.Equal(t, int32(45000), p.Members[0].X)
    require.Equal(t, int32(-3500), p.Members[0].Z)
    require.Equal(t, int32(11), p.Members[1].Level)
}

// TestParseAskJoinParty pins the invitation ask.
func TestParseAskJoinParty(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(AskJoinPartyPacketID))
    require.NoError(t, writer.WriteInt32(100))
    require.NoError(t, writer.WriteInt32(0))

    p := NewAskJoinPartyPacket()
    require.NoError(t, ParseAskJoinPartyPacket(p, writer.Bytes()))
    require.Equal(t, int32(100), p.RequestorObjectID)
}

// TestParseJoinParty pins the answer echo.
func TestParseJoinParty(t *testing.T) {
    writer := packet.NewWriter()
    require.NoError(t, writer.WriteInt8(JoinPartyPacketID))
    require.NoError(t, writer.WriteInt32(1))

    p := NewJoinPartyPacket()
    require.NoError(t, ParseJoinPartyPacket(p, writer.Bytes()))
    require.Equal(t, int32(1), p.Response)
}
