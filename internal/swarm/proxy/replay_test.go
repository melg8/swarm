// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"math"
	"testing"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// liveCharacter builds a character snapshot of a bot that walked away
// from the recorded login place, leveled up and took damage: the values
// the reconnect patch must push into every replayed self packet.
func liveCharacter() state.CharacterSnapshot {
	return state.CharacterSnapshot{
		ObjectID: 1055,
		Name:     "PatchChar",
		X:        60000,
		Y:        61000,
		Z:        -2500,
		Level:    8,
		Exp:      2000,
		Sp:       9,
		STR:      40,
		DEX:      30,
		CON:      36,
		INT:      21,
		WIT:      11,
		MEN:      20,
		CurHP:    75,
		MaxHP:    130,
		CurMP:    21,
		MaxMP:    55,
	}
}

// TestPatchCharSelectedLiveState pins the reconnect fix of the char
// selection answer: the recorded packet carries the login-time place,
// the proxy must rewrite it with the live tracker state, and the
// recorded bytes must stay untouched for the other replays.
func TestPatchCharSelectedLiveState(t *testing.T) {
	recorded := buildTestCharSelected("PatchChar")
	patched := patchCharSelectedLive(recorded, liveCharacter())

	selected := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(selected, patched))
	require.EqualValues(t, 60000, selected.X, "live x")
	require.EqualValues(t, 61000, selected.Y, "live y")
	require.EqualValues(t, -2500, selected.Z, "live z")
	require.InDelta(t, 75, selected.CurrentHP, 0.001, "live hp")
	require.InDelta(t, 21, selected.CurrentMP, 0.001, "live mp")

	original := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(original, recorded))
	require.EqualValues(t, 45000, original.X, "the recorded packet stays untouched")
	require.InDelta(t, 90, original.CurrentHP, 0.001)
}

// TestPatchCharSelectedLiveMalformed pins the degradation: an
// unscannable recorded packet replays unchanged instead of failing the
// client connection.
func TestPatchCharSelectedLiveMalformed(t *testing.T) {
	truncated := buildTestCharSelected("PatchChar")[:20]
	require.Equal(t, truncated, patchCharSelectedLive(truncated, liveCharacter()))

	unterminated := []byte{0x21, 'P', 0}
	require.Equal(t, unterminated,
		patchCharSelectedLive(unterminated, liveCharacter()))
}

// buildTestUserInfoPacket builds a wire-faithful UserInfo packet (see
// UserInfo.writeImpl) with the full tail the parser walks through.
func buildTestUserInfoPacket() []byte {
	data := []byte{0x04}
	data = binary.LittleEndian.AppendUint32(data, 45000) // x
	data = binary.LittleEndian.AppendUint32(data, 50000) // y
	data = appendTestInt32(data, -3500)                  // z
	data = binary.LittleEndian.AppendUint32(data, 0)     // vehicle id
	data = binary.LittleEndian.AppendUint32(data, 1055)  // object id
	data = appendUTF16(data, "PatchChar")
	data = binary.LittleEndian.AppendUint32(data, 1)  // race
	data = binary.LittleEndian.AppendUint32(data, 0)  // female
	data = binary.LittleEndian.AppendUint32(data, 18) // base class
	data = binary.LittleEndian.AppendUint32(data, 7)  // level
	data = binary.LittleEndian.AppendUint32(data, 1000)
	for range 6 {
		data = binary.LittleEndian.AppendUint32(data, 10) // stats
	}
	data = binary.LittleEndian.AppendUint32(data, 120) // max hp
	data = binary.LittleEndian.AppendUint32(data, 90)  // cur hp
	data = binary.LittleEndian.AppendUint32(data, 50)  // max mp
	data = binary.LittleEndian.AppendUint32(data, 40)  // cur mp
	data = binary.LittleEndian.AppendUint32(data, 5)   // sp
	data = binary.LittleEndian.AppendUint32(data, 0)   // current load
	data = binary.LittleEndian.AppendUint32(data, 0)   // max load
	data = binary.LittleEndian.AppendUint32(data, 20)  // weapon flag
	for range 15 {
		data = binary.LittleEndian.AppendUint32(data, 0) // paperdoll ids
	}
	for range 15 {
		data = binary.LittleEndian.AppendUint32(data, 0) // display ids
	}
	for range 12 {
		data = binary.LittleEndian.AppendUint32(data, 0) // combat stats
	}
	data = binary.LittleEndian.AppendUint32(data, 120) // run speed
	data = binary.LittleEndian.AppendUint32(data, 60)  // walk speed
	for range 6 {
		data = binary.LittleEndian.AppendUint32(data, 0) // swim and fly
	}
	data = binary.LittleEndian.AppendUint64(data, math.Float64bits(1))

	return data
}

// TestPatchUserInfoSelfLiveState pins the replay fix: a replayed
// UserInfo of the played character carries the live position, vitals
// and progression, while a UserInfo of any other object and a short
// packet replay unchanged.
func TestPatchUserInfoSelfLiveState(t *testing.T) {
	recorded := buildTestUserInfoPacket()
	live := liveCharacter()
	patched := patchUserInfoSelfLive(recorded, 1055, live)

	parsed := fromgameserver.NewUserInfoPacket()
	require.NoError(t, fromgameserver.ParseUserInfoPacket(parsed, patched))
	require.EqualValues(t, 60000, parsed.X, "live x")
	require.EqualValues(t, 61000, parsed.Y, "live y")
	require.EqualValues(t, -2500, parsed.Z, "live z")
	require.EqualValues(t, 8, parsed.Level, "live level")
	require.EqualValues(t, 2000, parsed.Exp, "live exp")
	require.EqualValues(t, 9, parsed.Sp, "live sp")
	require.EqualValues(t, 75, parsed.CurHP, "live hp")
	require.EqualValues(t, 130, parsed.MaxHP, "live max hp")
	require.EqualValues(t, 21, parsed.CurMP, "live mp")
	require.EqualValues(t, 55, parsed.MaxMP, "live max mp")

	original := fromgameserver.NewUserInfoPacket()
	require.NoError(t, fromgameserver.ParseUserInfoPacket(original, recorded))
	require.EqualValues(t, 45000, original.X, "the recorded packet stays untouched")

	// A UserInfo of another object (or a packet without the fixed header)
	// is not patched.
	require.Equal(t, recorded, patchUserInfoSelfLive(recorded, 268, live))
	require.Equal(t, recorded[:5], patchUserInfoSelfLive(recorded[:5], 1055, live))
}

// TestPatchUserInfoSelfPositionOnly pins the fallback: when the name
// string cannot be scanned, the patcher still rewrites the fixed header
// position of the self UserInfo.
func TestPatchUserInfoSelfPositionOnly(t *testing.T) {
	recorded := buildTestUserInfoPacket()
	// Cut the payload inside the name string: no null terminator.
	truncated := recorded[:30]
	live := liveCharacter()

	patched := patchUserInfoSelfLive(truncated, 1055, live)
	require.NotEqual(t, truncated, patched, "the position must still be patched")
	require.EqualValues(t, 60000, int32(binary.LittleEndian.Uint32(patched[1:5])))
	require.EqualValues(t, 61000, int32(binary.LittleEndian.Uint32(patched[5:9])))
	require.EqualValues(t, -2500, int32(binary.LittleEndian.Uint32(patched[9:13])))
}

// TestSelfMovementFiltering covers the movement family filter of the
// replay: every family opcode is recognized through the object id, and
// the newest self movement entry wins.
func TestSelfMovementFiltering(t *testing.T) {
	selfMove := buildTestWorldPacket(0x01, 1055)
	npcMove := buildTestWorldPacket(0x01, 42)
	selfStop := buildTestWorldPacket(0x59, 1055)
	selfValidate := buildTestWorldPacket(0x76, 1055)
	selfTeleport := buildTestWorldPacket(0x38, 1055)

	require.True(t, isSelfMovementPayload(selfMove, 1055))
	require.True(t, isSelfMovementPayload(selfStop, 1055))
	require.True(t, isSelfMovementPayload(selfValidate, 1055))
	require.True(t, isSelfMovementPayload(selfTeleport, 1055))
	require.False(t, isSelfMovementPayload(npcMove, 1055))
	require.False(t, isSelfMovementPayload(selfMove, 42))
	require.False(t, isSelfMovementPayload(
		buildTestWorldPacket(0x22, 1055), 1055), "npc info is not movement")
	require.False(t, isSelfMovementPayload(nil, 1055))
	require.False(t, isSelfMovementPayload(selfMove[:3], 1055))

	entries := []RecorderEntry{
		{seq: 1, payload: selfMove},
		{seq: 2, payload: npcMove},
		{seq: 3, payload: selfStop},
		{seq: 4, payload: buildTestWorldPacket(0x22, 42)},
		{seq: 5, payload: selfValidate},
	}
	require.EqualValues(t, 5, lastSelfMovementSeq(entries, 1055))
	require.Zero(t, lastSelfMovementSeq(
		[]RecorderEntry{{seq: 2, payload: npcMove}}, 1055))
}

// appendTestInt32 appends a little endian int32 to the test payload.
func appendTestInt32(dst []byte, value int32) []byte {
	return binary.LittleEndian.AppendUint32(dst, uint32(value))
}
