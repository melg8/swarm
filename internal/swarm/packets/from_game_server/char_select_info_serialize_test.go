// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/stretchr/testify/require"
)

// TestCharSelectInfoToBytesRoundTrip serializes a one character list,
// parses it back and compares the stored fields (the parser skips the
// sp and exp fields, so both sides write zeros for them).
func TestCharSelectInfoToBytesRoundTrip(t *testing.T) {
	t.Parallel()
	original := &CharSelectInfoPacket{
		Count: 1,
		Characters: []CharacterInfo{{
			Name:     "proxybot",
			ObjectID: 1055,
			Account:  "test1",
			// SessionID and ClanID are skipped by the parser (never
			// stored), so the round trip compares them as zeros.
			SessionID:   0,
			ClanID:      0,
			Sex:         0,
			Race:        1,
			BaseClassID: 18,
			X:           45000,
			Y:           50000,
			Z:           -3500,
			CurrentHP:   85.5,
			CurrentMP:   30.25,
			Level:       9,
			HairStyle:   0,
			HairColor:   1,
			Face:        2,
			MaxHP:       100.5,
			MaxMP:       40.5,
			DeleteTimer: 0,
		}},
	}

	writer := packet.NewWriter()
	require.NoError(t, original.ToBytes(writer))

	parsed := NewCharSelectInfoPacket()
	require.NoError(t, ParseCharSelectInfoPacket(parsed, writer.Bytes()))
	require.Equal(t, original.Count, parsed.Count)
	require.Len(t, parsed.Characters, 1)
	require.Equal(t, original.Characters[0].Name, parsed.Characters[0].Name)
	require.Equal(t, original.Characters[0].ObjectID, parsed.Characters[0].ObjectID)
	require.Equal(t, original.Characters[0].Account, parsed.Characters[0].Account)
	require.Equal(t, original.Characters[0].SessionID, parsed.Characters[0].SessionID)
	require.Equal(t, original.Characters[0].Sex, parsed.Characters[0].Sex)
	require.Equal(t, original.Characters[0].Race, parsed.Characters[0].Race)
	require.Equal(t, original.Characters[0].BaseClassID,
		parsed.Characters[0].BaseClassID)
	require.Equal(t, original.Characters[0].X, parsed.Characters[0].X)
	require.Equal(t, original.Characters[0].Y, parsed.Characters[0].Y)
	require.Equal(t, original.Characters[0].Z, parsed.Characters[0].Z)
	require.InDelta(t, original.Characters[0].CurrentHP,
		parsed.Characters[0].CurrentHP, 0.0001)
	require.InDelta(t, original.Characters[0].CurrentMP,
		parsed.Characters[0].CurrentMP, 0.0001)
	require.Equal(t, original.Characters[0].Level, parsed.Characters[0].Level)
	require.Equal(t, original.Characters[0].HairStyle, parsed.Characters[0].HairStyle)
	require.Equal(t, original.Characters[0].HairColor, parsed.Characters[0].HairColor)
	require.Equal(t, original.Characters[0].Face, parsed.Characters[0].Face)
	require.InDelta(t, original.Characters[0].MaxHP,
		parsed.Characters[0].MaxHP, 0.0001)
	require.InDelta(t, original.Characters[0].MaxMP,
		parsed.Characters[0].MaxMP, 0.0001)
	require.Equal(t, original.Characters[0].DeleteTimer, parsed.Characters[0].DeleteTimer)
}

// TestCharCreateFailToBytes pins the failure answer of the emulated
// server for character creation attempts.
func TestCharCreateFailToBytes(t *testing.T) {
	t.Parallel()
	writer := packet.NewWriter()
	require.NoError(t, writer.WriteInt8(0x26))
	require.NoError(t, writer.WriteInt32(0x01))

	parsed := NewCharCreateFailPacket()
	require.NoError(t, ParseCharCreateFailPacket(parsed, writer.Bytes()))
	require.Equal(t, int32(0x01), parsed.Reason)
}
