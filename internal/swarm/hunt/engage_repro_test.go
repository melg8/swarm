// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The reproduction of the standing bot state dump (2026-09-10
// 02:11:07): the bot test1 (level 13, 306 of 321 hp, hunting phase
// engage) stood exactly at the center of the square elven-2019_23-b1
// (Kaboo Orc Fighter Lieutenant SW-b1, center 30502 62755, half
// 1400) for 49 seconds without a target, without a walk and without
// a zone rotation - while the only two living attackable npcs inside
// the square (a Kaboo Orc Fighter at 31126 61892 and a Kaboo Orc
// Fighter Lieutenant at 31137 61598, 294 units apart, both of the
// ORC clan) fenced each other out of the target search, the plain
// emptiness reading of the rotation kept seeing them, the aggressive
// fighter sat 1065 units out - just past its 1000 unit aggro range -
// and the center patrol had no leg to walk. The tests below rebuild
// the scene with the dump's exact positions and pin the fix: the
// diagnostic log that names the opponents with their positions, and
// the zone rotation that reads the fenced square as empty and moves
// the hunt on.
const (
	reproDumpBotX = 30502
	reproDumpBotY = 62755
	reproDumpBotZ = -3576
	// The wire template ids of the dump mobs (the Mobius wire sends
	// the C4 display id + 1000000; the xml ids 20471/20473/20472/20469/
	// 20509 map onto the display ids 471/473/472/469/509).
	reproDumpFighterID     = 1000471
	reproDumpLieutenantID  = 1000473
	reproDumpLeaderID      = 1000472
	reproDumpArcherID      = 1000469
	reproDumpSporeFungusID = 1000509
)

// reproDumpNpc is one entry of the dumped object list.
type reproDumpNpc struct {
	objectID   int32
	templateID int32
	x          int32
	y          int32
	z          int32
}

// reproDumpObjects carries the 22 npcs of the dump verbatim: every
// object the bot saw at 02:11:07 with its exact position.
var reproDumpObjects = []reproDumpNpc{
	{268439688, reproDumpFighterID, 31126, 61892, -3560},
	{268439687, reproDumpLieutenantID, 31137, 61598, -3523},
	{268439680, reproDumpSporeFungusID, 31060, 61175, -3488},
	{268439685, reproDumpFighterID, 30979, 64672, -3675},
	{268439674, reproDumpLieutenantID, 30636, 64762, -3640},
	{268439686, reproDumpFighterID, 31970, 64237, -3688},
	{268439679, reproDumpSporeFungusID, 29984, 60608, -3472},
	{268439676, reproDumpLieutenantID, 29837, 60557, -3512},
	{268439614, reproDumpLeaderID, 28194, 62802, -3624},
	{268439611, reproDumpLieutenantID, 28190, 62933, -3624},
	{268439675, reproDumpLieutenantID, 30572, 65118, -3680},
	{268439677, reproDumpLieutenantID, 31499, 64949, -3704},
	{268439683, reproDumpFighterID, 31367, 65009, -3704},
	{268439681, reproDumpSporeFungusID, 29322, 60465, -3592},
	{268439613, reproDumpLeaderID, 28060, 63815, -3616},
	{268439684, reproDumpFighterID, 29171, 60412, -3600},
	{268439612, reproDumpLieutenantID, 27629, 61692, -3608},
	{268439618, reproDumpSporeFungusID, 27143, 62777, -3576},
	{268439608, reproDumpLieutenantID, 27287, 61619, -3592},
	{268439616, reproDumpLeaderID, 27185, 61448, -3592},
	{268439617, reproDumpLeaderID, 26924, 62419, -3568},
	{268439615, reproDumpLeaderID, 26755, 63155, -3560},
}

// reproDumpScene builds the hunt loop over the dumped tracker state:
// the character test1 at the zone center with the full dump object
// list, the elven zone registry and a captured log.
func reproDumpScene(
	t *testing.T,
) (*Loop, *fakeGame, *bytes.Buffer) {
	t.Helper()
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 268450864, 18,
		reproDumpBotX, reproDumpBotY, reproDumpBotZ, 306, 129)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 13, Race: 1, ClassID: 18,
		X: reproDumpBotX, Y: reproDumpBotY, Z: reproDumpBotZ,
		MaxHP: 321, CurHP: 306, MaxMP: 129, CurMP: 129,
	})
	// The dump ran with gear 227 points: the full shop dress of the
	// test helper (212) opens the 9-12 band the same way.
	equipZoneWithGear(bot, 212)
	for i := range reproDumpObjects {
		entry := &reproDumpObjects[i]
		bot.ApplyNpcInfo(state.NpcInfo{
			ObjectID: entry.objectID, TemplateID: entry.templateID,
			Attackable: true, X: entry.x, Y: entry.y, Z: entry.z,
		})
	}
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingZones(ElvenHuntingZones())
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))

	return loop, game, logBuf
}

// TestReproDumpStandingBotExplainsItselfInLog pins the diagnostic of
// the dump state: a targetless hunter standing in a socially fenced
// square names the opponents it sees with their positions and the
// rejection reasons in the log (the dump's object list answers lived
// only in the dump before), the line repeats on the pacing period
// and never sends an attack or a walk request.
func TestReproDumpStandingBotExplainsItselfInLog(t *testing.T) {
	loop, game, logBuf := reproDumpScene(t)
	// The automatic picker takes the square of the dump: the 9-12
	// band is the highest band a level 13 hunter enters (every higher
	// band opens only at its MaxLevel plus the lead), SW-b1 is the
	// nearest ground of the band from the dump position.
	loop.tick()
	require.Equal(t, "elven-2019_23-b1", loop.zonePickedID)

	// The first targetless tick arms the patience, nothing fires yet.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Empty(t, game.forces, "nothing is attackable in the square")
	require.Empty(t, game.walks, "no far target and no patrol leg exist")
	require.Equal(t, "elven-2019_23-b1", loop.zonePickedID)

	// The patience expires: the far search confirms the whole square
	// holds nothing pickable, the diagnostic line fires with the
	// dump's own data.
	loop.noTargetSince = time.Now().Add(-noTargetPatience - time.Second)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Empty(t, game.forces)
	require.Empty(t, game.walks)
	logged := logBuf.String()
	require.Contains(t, logged, "no pickable target in the zone")
	require.Contains(t, logged, "Kaboo Orc Fighter (268439688) at 31126 61892 -3560")
	require.Contains(t, logged, "clan pack with Kaboo Orc Fighter Lieutenant (268439687)")
	require.Contains(t, logged, "Kaboo Orc Fighter Lieutenant (268439687) at 31137 61598 -3523")
	require.Contains(t, logged, "clan pack with Kaboo Orc Fighter (268439688)")

	// The pacing holds the line back within the period, so the
	// standing wait never floods the log.
	lines := strings.Count(logged, "no pickable target")
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, lines, strings.Count(logBuf.String(), "no pickable target"))
}

// TestReproDumpStandingBotRotatesOutOfTheFencedSquare pins the fix of
// the dump state: the zone rotation reads the square through the
// pick's own filters, a square whose only survivors fence each other
// out rotates away after the rotate window instead of holding the
// hunter standing forever, and the walk into the next ground begins.
func TestReproDumpStandingBotRotatesOutOfTheFencedSquare(t *testing.T) {
	loop, game, logBuf := reproDumpScene(t)
	loop.tick()
	require.Equal(t, "elven-2019_23-b1", loop.zonePickedID)

	// The standing wait: the emptiness window arms while the pick
	// stays empty, the diagnostic explains the opponents.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	loop.noTargetSince = time.Now().Add(-noTargetPatience - time.Second)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.False(t, loop.zoneEmptySince.IsZero(),
		"the pick-empty square arms the emptiness window")
	require.Empty(t, game.forces)
	require.Empty(t, game.walks)

	// The rotate window is up: the fenced square rotates away like a
	// cleared-out one, its cooldown keeps it out of the next contests
	// and the hunter starts walking into the next ground.
	loop.zoneEmptySince = time.Now().Add(-zoneRotateAfter - time.Second)
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.NotEqual(t, "elven-2019_23-b1", loop.zonePickedID,
		"the fenced square rotates away")
	require.True(t, loop.zoneCoolingDown("elven-2019_23-b1", time.Now()),
		"the rotated square keeps its cooldown")
	require.Contains(t, logBuf.String(), "is cleared out, rotating to")
	require.NotEmpty(t, game.walks,
		"the hunter walks into the next ground of the band")
	require.Empty(t, game.forces,
		"no attack ever fires against the fenced pack")
}
