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
	ObjectID   int32
	Kind       ObjectKind
	Name       string
	Title      string
	TemplateID int32
	Attackable bool
	InCombat   bool
	Moving     bool
	Count      int32
	X          int32
	Y          int32
	Z          int32
	Heading    int32
	DestX      int32
	DestY      int32
	DestZ      int32
	CurHP      float64
	MaxHP      float64
	UpdatedAt  time.Time
}

// newWorldObject creates a zero valued object with the given identity.
func newWorldObject(objectID int32, kind ObjectKind) WorldObject {
	return WorldObject{
		ObjectID:   objectID,
		Kind:       kind,
		Name:       "",
		Title:      "",
		TemplateID: 0,
		Attackable: false,
		InCombat:   false,
		Moving:     false,
		Count:      1,
		X:          0,
		Y:          0,
		Z:          0,
		Heading:    0,
		DestX:      0,
		DestY:      0,
		DestZ:      0,
		CurHP:      0,
		MaxHP:      0,
		UpdatedAt:  time.Time{},
	}
}

// mathPi is pi as its own constant to keep the hot path allocation free.
const mathPi = math.Pi

// HeadingFromDelta returns the game heading value for a movement delta,
// mirroring LocationUtil.calculateHeadingFrom of the Mobius server:
// atan2(dy, dx) scaled to the 0..65535 range where 0 faces east and the
// angle grows clockwise on the map (positive y points south).
func HeadingFromDelta(dx, dy int32) int32 {
	const fullCircle = 65536.0
	angle := math.Atan2(float64(dy), float64(dx)) * fullCircle / (2 * mathPi)
	if angle < 0 {
		angle += fullCircle
	}

	return int32(angle)
}
