// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package state tracks the observed world state of every bot session:
// character vitals and position, surrounding world objects and a rolling
// event log. The state is fed by the connection layer from parsed game
// server packets and read by the web interface.
package state

import (
	"math"
	"time"
)

// ObjectKind classifies a world object observed around the character.
type ObjectKind string

const (
	// KindNPC is a monster or friendly npc.
	KindNPC ObjectKind = "npc"
	// KindPlayer is another player character.
	KindPlayer ObjectKind = "player"
	// KindItem is an item lying on the ground.
	KindItem ObjectKind = "item"
)

// objectKindCode values are the compact int8 codes of the object kinds
// stored in the hot records: the scan filters compare the kind every
// tick and a one byte load beats the sixteen byte string header walk
// (plus the string data cache miss on a real comparison).
const (
	kindNPC int8 = iota
	kindPlayer
	kindItem
)

// kindCode maps the public kind to the compact code.
func kindCode(kind ObjectKind) int8 {
	switch kind {
	case KindNPC:
		return kindNPC
	case KindPlayer:
		return kindPlayer
	case KindItem:
		return kindItem
	default:
		return kindNPC
	}
}

// kindString maps the compact code back to the public kind: the
// snapshot encode writes it as the wire string.
func kindString(code int8) ObjectKind {
	switch code {
	case kindNPC:
		return KindNPC
	case kindPlayer:
		return KindPlayer
	case kindItem:
		return KindItem
	default:
		return KindNPC
	}
}

// zeroTimeUnixMilli is the UnixMilli of the zero time.Time (year 1):
// the 0 sentinel of the stored nanoseconds encodes the same value, so
// the JSON view of an untouched field stays byte identical to the
// time.Time based encoding it replaces.
const zeroTimeUnixMilli = int64(-62135596800000)

// unixMilliFromNano converts stored unix nanoseconds to the
// milliseconds of the JSON view; the 0 sentinel is the zero time,
// exactly like time.Time.UnixMilli of a zero value.
func unixMilliFromNano(n int64) int64 {
	if n == 0 {
		return zeroTimeUnixMilli
	}

	return n / int64(time.Millisecond)
}

// objectHot is the hot half of a world object record (see
// objectStore): the fields the per tick scan paths, the movement
// projection, the target tracking and the combat marks touch. The
// layout is one dense 88 byte block per object, so the scans stream
// the first cache lines of the records instead of chasing the 240
// byte full records (names, titles and vitals included) line by line
// - a 200 npc world drops from 48 KB of scan traffic to 17.6 KB.
type objectHot struct {
	ObjectID      int32
	TargetID      int32
	Level         int32
	X             int32
	Y             int32
	Z             int32
	DestX         int32
	DestY         int32
	DestZ         int32
	ClanHelpRange int32
	RunSpeed      int32
	WalkSpeed     int32
	ClanMask      uint64
	// MoveAt is the movement start as unix nanoseconds, 0 = never
	// (the zero time).
	MoveAt int64
	// CombatUntil is the end of the observed combat window as unix
	// nanoseconds, 0 = none.
	CombatUntil int64
	// MoveSpeedMult is the multiplier of the spawn packet.
	MoveSpeedMult float64
	// Kind is the objectKindCode of the record.
	Kind          int8
	Attackable    bool
	Aggressive    bool
	Dead          bool
	Moving        bool
	Running       bool
	AutoAttacking bool
}

// objectCold is the cold half of a world object record: the display
// fields of the snapshot view and the vitals of the StatusUpdate
// writes. The scan paths never touch it, so the names and HP floats
// cost bandwidth only on the 300 ms snapshot polls of a watched bot,
// not on the hunt loop ticks.
type objectCold struct {
	Name            string
	Title           string
	TemplateID      int32
	Heading         int32
	Count           int32
	AggroRange      int32
	CollisionRadius float64
	// Sitting marks the rest state of the creature (the zZ icon of
	// the map): a player object sits through its own ChangeWaitType
	// broadcast or the standing byte of its CharInfo.
	Sitting bool
	// SocialUntil is the end of the social animation marker as unix
	// nanoseconds, 0 = none.
	SocialUntil int64
	// UpdatedAt is the arrival time of the last packet that touched
	// the record, as unix nanoseconds.
	UpdatedAt int64
	CurHP     float64
	MaxHP     float64
	CurMP     float64
	MaxMP     float64
}

// inCombat reports whether the object was seen fighting recently. The
// auto attack flag holds until the stop packet, single attacks and the
// NpcInfo combat flag hold for the combat window.
func (h *objectHot) inCombat(nowNano int64) bool {
	return h.AutoAttacking || h.CombatUntil > nowNano
}

// effectiveSpeed returns the movement speed of the object in world
// units per second, applying the move multiplier of the spawn packet.
// It falls back to a common monster run speed when the packet carried
// nothing.
func (h *objectHot) effectiveSpeed() float64 {
	speed := float64(h.WalkSpeed)
	if h.Running || h.WalkSpeed <= 0 {
		speed = float64(h.RunSpeed)
	}
	if h.MoveSpeedMult > 0 {
		speed *= h.MoveSpeedMult
	}
	if speed <= 1 {
		return defaultRunSpeed
	}

	return speed
}

// defaultRunSpeed is the fallback movement speed for objects without
// transmitted speeds.
const defaultRunSpeed = 120

// defaultWalkSpeed is the fallback walking speed of the character.
const defaultWalkSpeed = 60

// effectiveSpeed combines a transmitted base speed with the move
// multiplier of the spawn packets. The server divides the real speeds by
// the multiplier before writing them (see UserInfo, CharInfo and
// AbstractNpcInfo writeImpl of the Mobius server), so the observed
// movement is speed * multiplier. It falls back to the common monster
// run speed when the packet carried nothing usable.
func effectiveSpeed(base int32, walk int32, mult float64) float64 {
	speed := float64(base)
	if mult > 0 {
		speed *= mult
	}
	if speed <= 1 {
		speed = float64(walk)
		if mult > 0 {
			speed *= mult
		}
	}
	if speed <= 1 {
		return defaultRunSpeed
	}

	return speed
}

// mathPi is pi as its own constant to keep the hot path allocation free.
const mathPi = math.Pi

// HeadingFromDelta returns the game heading value for a movement delta,
// mirroring LocationUtil.calculateHeadingFrom of the Mobius server:
// atan2(dy, dx) scaled to the 0..65535 range where 0 faces east and the
// angle grows clockwise on the map (positive y points south).
func HeadingFromDelta(dx, dy int32) int32 {
	const fullCircle = 65536.0
	angle := math.Atan2(float64(dy), float64(dx)) *
		fullCircle / (2 * mathPi)
	if angle < 0 {
		angle += fullCircle
	}

	return int32(angle)
}
