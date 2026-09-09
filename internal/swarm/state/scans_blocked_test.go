// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The reproduction scene of the standing bot state dump (2026-09-10
// 02:11:07, bot test1 in the hunting square elven-2019_23-b1): the
// character stands exactly at the zone center, and the ONLY living
// attackable npcs inside the square are a Kaboo Orc Fighter and a
// Kaboo Orc Fighter Lieutenant 294 units apart - both of the ORC
// clan with the 300 unit help range, so the social fence of the
// target search (300 + the 200 margin) holds each of them out
// through the other. The plain emptiness reading of the zone rotation
// still saw them, the pick did not, and the hunter stood still
// forever. The constants below come from the dump verbatim.
const (
	reproDumpSelfX = 30502
	reproDumpSelfY = 62755
	reproDumpSelfZ = -3576
	// Wire template ids: the Mobius NpcIdConverter maps the xml ids
	// 20471/20473/20472/20509 onto the display ids 471/473/472/509
	// and the wire adds the 1000000 offset.
	reproFighterWireID     = 1000471
	reproLieutenantWireID  = 1000473
	reproLeaderWireID      = 1000472
	reproSporeFungusWireID = 1000509
)

// reproDumpBot builds the tracker state of the dumped character: the
// elven fighter test1 (object 268450864, level 13) standing at the
// center of the hunting square with a healthy 306 of 321 hp.
func reproDumpBot(t *testing.T) *Bot {
	t.Helper()
	bot := NewBot("acc1")
	bot.SetCharacter("test1", 268450864, 18,
		reproDumpSelfX, reproDumpSelfY, reproDumpSelfZ, 306, 129)
	bot.ApplyUserInfo(UserInfo{
		Name: "test1", Level: 13, Race: 1, ClassID: 18,
		X: reproDumpSelfX, Y: reproDumpSelfY, Z: reproDumpSelfZ,
		MaxHP: 321, CurHP: 306, MaxMP: 129, CurMP: 129,
	})

	return bot
}

// reproDumpZone is the hunting square of the dump: center 30502
// 62755, half 1400 (a 2800x2800 square).
func reproDumpZone() *Zone {
	return &Zone{CX: reproDumpSelfX, CY: reproDumpSelfY, Half: 1400}
}

// spawnReproDumpPair spawns the two npcs the dump holds inside the
// square, at their exact dump positions: the Kaboo Orc Fighter
// (aggressive, level 10) 1065 units from the character and the
// Kaboo Orc Fighter Lieutenant (level 11) 1320 units out, 294 units
// from each other.
func spawnReproDumpPair(bot *Bot) {
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439688, TemplateID: reproFighterWireID,
		Attackable: true, X: 31126, Y: 61892, Z: -3560,
	})
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439687, TemplateID: reproLieutenantWireID,
		Attackable: true, X: 31137, Y: 61598, Z: -3523,
	})
}

// TestZoneHasPickableTreatsTheFencedClanPackAsEmpty pins the reading
// gap that stalled the dumped bot: the plain emptiness reading of the
// zone rotation (ZoneHasAttackableBelow) still counts the socially
// fenced clan pair, while the pick-shaped reading (ZoneHasPickable)
// correctly reports the square as empty for the hunter - no pick, no
// far target walk, no rotation, a standing character.
func TestZoneHasPickableTreatsTheFencedClanPackAsEmpty(t *testing.T) {
	bot := reproDumpBot(t)
	spawnReproDumpPair(bot)
	zone := reproDumpZone()

	require.True(t, bot.ZoneHasAttackableBelow(zone, 15),
		"the plain reading sees the living pair - this is what kept "+
			"the rotation away before the fix")
	require.False(t, bot.ZoneHasPickable(zone, 15, nil),
		"the pick-shaped reading must treat the fenced pair as empty")

	// The pair breaks up: the fighter walks 600 units south, 596
	// units from the lieutenant now - past the 500 unit fence - and
	// the pick may take it again.
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439688, TemplateID: reproFighterWireID,
		Attackable: true, X: 31126, Y: 62192, Z: -3560,
	})
	require.True(t, bot.ZoneHasPickable(zone, 15, nil),
		"a lone clan member is pickable again")
}

// TestZoneHasPickableAppliesLevelCeilingAndSkips pins the remaining
// filters of the pick-shaped emptiness reading: the level ceiling and
// the skip list of the engage hold mobs out of the reading the same
// way they hold them out of the pick.
func TestZoneHasPickableAppliesLevelCeilingAndSkips(t *testing.T) {
	bot := reproDumpBot(t)
	zone := reproDumpZone()
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439614, TemplateID: reproLeaderWireID,
		Attackable: true, X: 30900, Y: 62900, Z: -3600,
	})

	require.True(t, bot.ZoneHasPickable(zone, 15, nil),
		"the level 12 leader passes the 15 ceiling of the level 13 hunter")
	require.False(t, bot.ZoneHasPickable(zone, 10, nil),
		"the leader above the ceiling reads empty")
	require.False(t, bot.ZoneHasPickable(zone, 15, []int32{268439614}),
		"a skipped mob reads empty")
}

// TestNearestBlockedTargetsReportsTheClanPack pins the targetless
// diagnostic data: the nearest rejected npcs with their dump
// positions and the human readable rejection reasons - the clan pack
// with the blocking pack mate for the fenced pair, the zone square
// for the mobs outside it, and nothing for a pickable loner.
func TestNearestBlockedTargetsReportsTheClanPack(t *testing.T) {
	bot := reproDumpBot(t)
	spawnReproDumpPair(bot)
	// The nearest outside mob of the dump: a Spore Fungus 1676 units
	// out, north of the square.
	bot.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439680, TemplateID: reproSporeFungusWireID,
		Attackable: true, X: 31060, Y: 61175, Z: -3488,
	})

	blocked := bot.NearestBlockedTargets(reproDumpZone(), 15, nil, 5)
	require.Len(t, blocked, 3)
	require.Equal(t, int32(268439688), blocked[0].ObjectID)
	require.Equal(t, "Kaboo Orc Fighter", blocked[0].Name)
	require.Equal(t, int32(31126), blocked[0].X)
	require.Equal(t, int32(61892), blocked[0].Y)
	require.Equal(t, int32(-3560), blocked[0].Z)
	require.Equal(t, "clan pack with Kaboo Orc Fighter Lieutenant "+
		"(268439687)", blocked[0].Reason)
	require.Equal(t, int32(268439687), blocked[1].ObjectID)
	require.Equal(t, "clan pack with Kaboo Orc Fighter (268439688)",
		blocked[1].Reason)
	require.Equal(t, int32(268439680), blocked[2].ObjectID)
	require.Equal(t, "outside the hunting zone", blocked[2].Reason)

	// The limit truncates the list to the nearest rejected mobs.
	pair := bot.NearestBlockedTargets(reproDumpZone(), 15, nil, 2)
	require.Len(t, pair, 2)
	require.Equal(t, int32(268439688), pair[0].ObjectID)
	require.Equal(t, int32(268439687), pair[1].ObjectID)

	// A pickable loner never appears in the list: the far target walk
	// reaches it, it is simply not engaged yet.
	loner := NewBot("acc1")
	loner.SetCharacter("test1", 268450864, 18,
		reproDumpSelfX, reproDumpSelfY, reproDumpSelfZ, 306, 129)
	loner.ApplyNpcInfo(NpcInfo{
		ObjectID: 268439688, TemplateID: reproFighterWireID,
		Attackable: true, X: 31126, Y: 61892, Z: -3560,
	})
	require.Empty(t,
		loner.NearestBlockedTargets(reproDumpZone(), 15, nil, 5),
		"a lone fighter is pickable, not blocked")
}
