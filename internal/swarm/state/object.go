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

// WorldObject is one object in the vicinity of the bot character.
type WorldObject struct {
	ObjectID        int32
	Kind            ObjectKind
	Name            string
	Title           string
	TemplateID      int32
	Attackable      bool
	Aggressive      bool
	AggroRange      int32
	Level           int32
	AutoAttacking   bool
	CombatUntil     time.Time
	Dead            bool
	Moving          bool
	Running         bool
	RunSpeed        int32
	WalkSpeed       int32
	MoveSpeedMult   float64
	CollisionRadius float64
	TargetID        int32
	Count           int32
	X               int32
	Y               int32
	Z               int32
	Heading         int32
	DestX           int32
	DestY           int32
	DestZ           int32
	MoveAt          time.Time
	CurHP           float64
	MaxHP           float64
	CurMP           float64
	MaxMP           float64
	UpdatedAt       time.Time
}

// newWorldObject creates a zero valued object with the given identity.
func newWorldObject(objectID int32, kind ObjectKind) WorldObject {
	return WorldObject{
		ObjectID:        objectID,
		Kind:            kind,
		Name:            "",
		Title:           "",
		TemplateID:      0,
		Attackable:      false,
		Aggressive:      false,
		AggroRange:      0,
		Level:           0,
		AutoAttacking:   false,
		CombatUntil:     time.Time{},
		Dead:            false,
		Moving:          false,
		Running:         true,
		RunSpeed:        0,
		WalkSpeed:       0,
		MoveSpeedMult:   1,
		CollisionRadius: 0,
		TargetID:        0,
		Count:           1,
		X:               0,
		Y:               0,
		Z:               0,
		Heading:         0,
		DestX:           0,
		DestY:           0,
		DestZ:           0,
		MoveAt:          time.Time{},
		CurHP:           0,
		MaxHP:           0,
		CurMP:           0,
		MaxMP:           0,
		UpdatedAt:       time.Time{},
	}
}

// InCombat reports whether the object was seen fighting recently. The
// auto attack flag holds until the stop packet, single attacks and the
// NpcInfo combat flag hold for the combat window.
func (o WorldObject) InCombat(now time.Time) bool {
	return o.AutoAttacking || o.CombatUntil.After(now)
}

// EffectiveSpeed returns the movement speed of the object in world units
// per second, applying the move multiplier of the spawn packet. It falls
// back to a common monster run speed when the packet carried nothing.
func (o WorldObject) EffectiveSpeed() float64 {
	speed := float64(o.WalkSpeed)
	if o.Running || o.WalkSpeed <= 0 {
		speed = float64(o.RunSpeed)
	}
	if o.MoveSpeedMult > 0 {
		speed *= o.MoveSpeedMult
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
