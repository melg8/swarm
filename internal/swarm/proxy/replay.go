// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"math"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The replayed history describes the world as the bot saw it, and the
// kept packets of the world objects stay valid for a reconnecting
// client (the objects converge through the prologue and the tail). The
// self-state packets do not: the recorded CharSelected and UserInfo
// carry the login-time place, so a client that reconnects after the bot
// walked away spawns at the stale spot, runs against the server-side
// geometry and crashes. The patchers below rewrite the recorded
// self-state with the live tracker snapshot instead, and the replay
// filter drops the stale self movement packets (keeping only the newest
// one, which matches the live tracker position by construction).

// Server packet opcodes the replay patching understands. Every movement
// family packet carries the moved object id in its first int32.
const (
	replayOpMoveToLocation = 0x01
	replayOpUserInfo       = 0x04
	replayOpTeleport       = 0x38
	replayOpMoveToPawn     = 0x75
	replayOpValidateLoc    = 0x76
	replayOpStopMove       = 0x59
)

// charSelectedPatchLen is the byte length of the patched tail of the
// CharSelected packet: x, y, z, curHp and curMp doubles, sp, exp and
// level ints.
const charSelectedPatchLen = 3*4 + 2*8 + 3*4

// patchCharSelectedLive returns a copy of the recorded CharSelected
// packet with the position, vitals and progression fields rewritten
// from the live character snapshot. A packet that cannot be scanned
// (truncated or malformed) is returned unchanged: the replay must never
// fail a client connection over a cosmetic patch.
func patchCharSelectedLive(
	payload []byte, char state.CharacterSnapshot,
) []byte {
	offset, ok := charSelectedPositionOffset(payload)
	if !ok {
		return payload
	}

	patched := make([]byte, len(payload))
	copy(patched, payload)
	putInt32(patched, offset, char.X)
	putInt32(patched, offset+4, char.Y)
	putInt32(patched, offset+8, char.Z)
	putFloat64(patched, offset+12, char.CurHP)
	putFloat64(patched, offset+20, char.CurMP)
	putInt32(patched, offset+28, char.Sp)
	putInt32(patched, offset+32, char.Exp)
	putInt32(patched, offset+36, char.Level)

	return patched
}

// charSelectedHeaderInts counts the int32 fields between the title
// string and the x coordinate: session, clan, unknown, sex, race,
// class and active.
const charSelectedHeaderInts = 7

// charSelectedPositionOffset scans the header of the recorded
// CharSelected packet (two null terminated utf16 strings, the object id
// and the header ints, see CharSelected.writeImpl) and returns the byte
// offset of the x field with the patch tail fitting the payload.
func charSelectedPositionOffset(payload []byte) (int, bool) {
	offset := skipUtf16String(payload, 1)     // name
	offset += 4                               // object id
	offset = skipUtf16String(payload, offset) // title
	offset += charSelectedHeaderInts * 4

	return offset, offset >= 0 &&
		offset+charSelectedPatchLen <= len(payload)
}

// skipUtf16String returns the offset right after the null terminator of
// the utf16 string starting at offset, or a negative value when the
// payload ends before the terminator.
func skipUtf16String(payload []byte, offset int) int {
	for offset+1 < len(payload) {
		if payload[offset] == 0 && payload[offset+1] == 0 {
			return offset + 2
		}
		offset += 2
	}

	return -1
}

// userInfoFixedHeaderLen is the byte length of the fixed UserInfo
// header before the name string: opcode, x, y, z, vehicle id and
// object id.
const userInfoFixedHeaderLen = 21

// userInfoPatchedTailLen is the byte length of the UserInfo block after
// the name string the patcher needs: race, female, base class, level,
// exp, six stats, the four vitals and sp.
const userInfoPatchedTailLen = 64

// patchUserInfoSelfLive returns a copy of a replayed UserInfo packet of
// the played character with the position, vitals, level and progression
// fields rewritten from the live snapshot. A UserInfo of any other
// object (or a packet without the fixed header) is returned unchanged;
// when the name string cannot be scanned the patch degrades to the
// fixed header position fields.
func patchUserInfoSelfLive(
	payload []byte, selfID int32, char state.CharacterSnapshot,
) []byte {
	if len(payload) < userInfoFixedHeaderLen ||
		payload[0] != replayOpUserInfo {
		return payload
	}
	if int32(binary.LittleEndian.Uint32(
		payload[17:userInfoFixedHeaderLen])) != selfID {
		return payload
	}

	patched := make([]byte, len(payload))
	copy(patched, payload)
	putInt32(patched, 1, char.X)
	putInt32(patched, 5, char.Y)
	putInt32(patched, 9, char.Z)

	// The vitals follow the variable length name string: they are
	// patched only when the full block can be scanned.
	nameEnd := skipUtf16String(payload, userInfoFixedHeaderLen)
	if nameEnd < 0 || nameEnd+userInfoPatchedTailLen > len(payload) {
		return patched
	}
	putInt32(patched, nameEnd+12, char.Level)
	putInt32(patched, nameEnd+16, char.Exp)
	putInt32(patched, nameEnd+20, char.STR)
	putInt32(patched, nameEnd+24, char.DEX)
	putInt32(patched, nameEnd+28, char.CON)
	putInt32(patched, nameEnd+32, char.INT)
	putInt32(patched, nameEnd+36, char.WIT)
	putInt32(patched, nameEnd+40, char.MEN)
	putInt32(patched, nameEnd+44, int32(math.Round(char.MaxHP)))
	putInt32(patched, nameEnd+48, int32(math.Round(char.CurHP)))
	putInt32(patched, nameEnd+52, int32(math.Round(char.MaxMP)))
	putInt32(patched, nameEnd+56, int32(math.Round(char.CurMP)))
	putInt32(patched, nameEnd+60, char.Sp)

	return patched
}

// isSelfMovementPayload reports whether the payload is a movement
// family packet of the played character (the family members all carry
// the moved object id in the first int32 after the opcode).
func isSelfMovementPayload(payload []byte, selfID int32) bool {
	if len(payload) < 5 {
		return false
	}
	switch payload[0] {
	case replayOpMoveToLocation, replayOpMoveToPawn, replayOpStopMove,
		replayOpValidateLoc, replayOpTeleport:
		return int32(binary.LittleEndian.Uint32(payload[1:5])) == selfID
	default:
		return false
	}
}

// lastSelfMovementSeq returns the sequence number of the newest self
// movement entry of the replay window: the one recorded packet whose
// coordinates still match the live tracker position (the tracker takes
// its position from exactly these packets), so the replay keeps it and
// drops the older ones.
func lastSelfMovementSeq(entries []RecorderEntry, selfID int32) int64 {
	var last int64
	for i := range entries {
		if isSelfMovementPayload(entries[i].payload, selfID) {
			last = entries[i].seq
		}
	}

	return last
}

// putInt32 writes a little endian int32 into the payload at offset.
func putInt32(payload []byte, offset int, value int32) {
	binary.LittleEndian.PutUint32(payload[offset:], uint32(value))
}

// putFloat64 writes a little endian float64 into the payload at offset.
func putFloat64(payload []byte, offset int, value float64) {
	binary.LittleEndian.PutUint64(payload[offset:], math.Float64bits(value))
}
