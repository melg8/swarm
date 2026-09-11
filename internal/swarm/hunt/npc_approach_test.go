// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"math"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The roof teleport regression of 2026-09-11: the user report (Russian)
// said the bots run somewhere behind the building trying to talk to
// the village teachers (Cobendell et al.), and when the character
// approaches the npc the server teleports it onto the roof of the
// building instead of letting it enter inside. The geodata probe
// (scripts/probe_cobendell) confirmed the mechanism: clicking on the
// npc's exact spawn cell from the south or west makes the server's
// getValidLocation Bresenham line cross the building wall, the
// height-step fallback resolves the target onto the roof layer (z
// -2456..-2576 instead of the ground floor z -2792), and the bot ends
// up on the roof. The fix: the approach walk clicks the ground at the
// npc approach point - a point npcApproachOffset units (150) from the
// npc toward the bot - so the click line stays outside the building
// walls and the server validates it on the ground floor.

// TestNpcApproachPointOffsetsTowardTheBot pins the offset computation:
// the click target lies on the line from the npc to the bot, at the
// configured offset distance, and never past the bot. When the bot
// already stands within the offset, the target collapses onto the
// bot's own cell (the click is a no-op the caller skips).
func TestNpcApproachPointOffsetsTowardTheBot(t *testing.T) {
	const npcX, npcY, npcZ = int32(44823), int32(52414), int32(-2792)
	cases := []struct {
		name  string
		selfX int32
		selfY int32
	}{
		{"north 500", npcX, npcY - 500},
		{"east 500", npcX + 500, npcY},
		{"south-east 424", npcX + 300, npcY + 300},
		{"north-west 424", npcX - 300, npcY - 300},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ax, ay, az := npcApproachPoint(npcX, npcY, npcZ, tc.selfX, tc.selfY)
			// The z is always the npc's z.
			require.Equal(t, npcZ, az, "the offset target keeps the npc z")
			// The offset target lies on the line from the npc to the bot.
			dx := float64(ax - npcX)
			dy := float64(ay - npcY)
			offsetDist := math.Hypot(dx, dy)
			require.InDelta(t, npcApproachOffset, offsetDist, 1.0,
				"the offset target is %.0f units from the npc", npcApproachOffset)
			// The offset target is between the npc and the bot, never past it.
			toBotX := float64(tc.selfX - npcX)
			toBotY := float64(tc.selfY - npcY)
			require.Positive(t, dx*toBotX+dy*toBotY,
				"the offset target is on the bot side of the npc")
		})
	}
}

// TestNpcApproachPointCollapsesOntoTheBotWhenClose pins the clamp: a
// bot already within the offset distance gets its own x and y back so
// the caller skips the click (the server would collapse it anyway).
func TestNpcApproachPointCollapsesOntoTheBotWhenClose(t *testing.T) {
	const npcX, npcY, npcZ = int32(44823), int32(52414), int32(-2792)
	// The dump scenario: the bot stands 26 units from Cobendell.
	selfX, selfY := int32(44831), int32(52389)
	ax, ay, az := npcApproachPoint(npcX, npcY, npcZ, selfX, selfY)
	require.Equal(t, selfX, ax, "the offset target is the bot's own x")
	require.Equal(t, selfY, ay, "the offset target is the bot's own y")
	require.Equal(t, npcZ, az, "the z stays the npc's z")
}

// TestApproachTeacherClicksTheOffsetNotTheExactCell pins the fix: a
// bot far from the teacher clicks the ground at the npc approach
// point, never at the teacher's exact spawn cell. The 2026-09-11 roof
// teleport happened because the bot clicked Cobendell's exact spawn
// (44823 52414 -2792) and the server's Bresenham line stepped over
// onto the roof layer. The offset keeps the click on the surrounding
// deck.
func TestApproachTeacherClicksTheOffsetNotTheExactCell(t *testing.T) {
	loop, game, bot := newLearnLoop(500)
	// The teach stop is current, the teacher Cobendell is in the
	// known list at its spawn point.
	loop.phase = phaseTownSell
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7156, Name: "Cobendell",
			X: 44823, Y: 52414, Z: -2792,
		},
		teach: true,
	}}
	const teacherID = int32(77)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: teacherID, TemplateID: 7156 + 1000000,
		X: 44823, Y: 52414, Z: -2792, Name: "Cobendell",
	})
	loop.teacherID = teacherID
	// The bot stands 500 units north of Cobendell (a safe approach
	// direction the geodata probe confirmed).
	moveSelfTo(bot, 44823, 51914, -2796)

	loop.tick()

	require.NotEmpty(t, game.walks,
		"the approach walk must send a ground click")
	click := game.walks[len(game.walks)-1]
	// The click must NOT be the teacher's exact spawn cell.
	require.False(t, click[0] == 44823 && click[1] == 52414,
		"the approach click must not target the teacher's exact cell "+
			"(the roof teleport root cause), got %d %d %d",
		click[0], click[1], click[2])
	// The click must target the offset point: 150 units from the
	// teacher toward the bot.
	ax, ay, az := npcApproachPoint(44823, 52414, -2792, 44823, 51914)
	require.Equal(t, [3]int32{ax, ay, az}, click,
		"the approach click targets the npc approach point")
	// The click z is the teacher's z (the ground floor).
	require.Equal(t, int32(-2792), click[2],
		"the click z is the teacher's ground floor z")
}

// TestApproachMerchantClicksTheOffsetNotTheExactCell pins the same
// fix for the merchant approach: the bot far from the merchant clicks
// the offset point, not the merchant's exact cell.
func TestApproachMerchantClicksTheOffsetNotTheExactCell(t *testing.T) {
	loop, game, bot, _ := newTripLoop()
	fillInventory(bot)
	// The sell stop is current, the merchant Herbiel is in the known
	// list at its spawn point.
	loop.phase = phaseTownSell
	loop.tripStart = time.Now()
	loop.sellPhaseAt = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7150, Name: "Herbiel",
			X: 42766, Y: 50037, Z: -2984,
		},
		sell: true,
	}}
	const merchantID = int32(88)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: merchantID, TemplateID: 7150 + 1000000,
		X: 42766, Y: 50037, Z: -2984, Name: "Herbiel",
	})
	loop.merchantID = merchantID
	// The bot stands 500 units east of Herbiel - far enough that the
	// 3D distance exceeds the approach gate and the approach walk
	// fires before the sale.
	moveSelfTo(bot, 43266, 50037, -2984)

	game.walks = nil
	loop.tick()

	require.NotEmpty(t, game.walks,
		"the approach walk must send a ground click")
	click := game.walks[len(game.walks)-1]
	// The click must NOT be the merchant's exact spawn cell.
	require.False(t, click[0] == 42766 && click[1] == 50037,
		"the approach click must not target the merchant's exact cell "+
			"(the roof teleport root cause), got %d %d %d",
		click[0], click[1], click[2])
	// The click must target the offset point.
	ax, ay, az := npcApproachPoint(42766, 50037, -2984, 43266, 50037)
	require.Equal(t, [3]int32{ax, ay, az}, click,
		"the approach click targets the npc approach point")
}

// TestApproachTeacherDeckHopClicksFromTheOffsetRing pins the 2026-09-11
// 05:45 dump fix: a bot within the 2D approach distance but on a
// different z (the deck hop case) clicks the teacher directly through
// the talk click (ClickObject) - the server INTERACTION_DISTANCE of
// 250 in 3D is met even with the z gap. The earlier roof teleport fix
// (the offset approach point) keeps the ground click safe; this fix
// keeps the talk click firing from the offset ring instead of looping
// forever waiting for dist3D <= 200.
func TestApproachTeacherDeckHopClicksFromTheOffsetRing(t *testing.T) {
	loop, game, bot := newLearnLoop(500)
	loop.phase = phaseTownSell
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7156, Name: "Cobendell",
			X: 44823, Y: 52414, Z: -2792,
		},
		teach: true,
	}}
	const teacherID = int32(77)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: teacherID, TemplateID: 7156 + 1000000,
		X: 44823, Y: 52414, Z: -2792, Name: "Cobendell",
	})
	loop.teacherID = teacherID
	// The bot stands 100 units north of Cobendell (within the 200 unit
	// 2D approach distance) but 200 units above it (the deck hop case:
	// the z mismatch pushes the 3D distance to ~224, above the 200
	// approach gate but within the 250 interaction gate).
	moveSelfTo(bot, 44823, 52314, -2592)

	game.walks = nil
	game.clicks = nil
	loop.tick()

	// The deck hop case with dist3D in (200, 250]: the talk click
	// fires (ClickObject on the teacher), no ground walk is sent.
	require.Empty(t, game.walks,
		"the deck hop case must not send a ground walk (the offset "+
			"collapses onto the bot's own cell)")
	require.Contains(t, game.clicks, teacherID,
		"the talk click fires from the offset ring - the server "+
			"INTERACTION_DISTANCE of 250 is met even with the z gap")
	require.NotEqual(t, int32(-1), loop.teacherID,
		"the teacher is not given up (the talk click landed)")
}

// TestApproachTeacherTalkClickFiresFromTheOffsetRing pins the exact
// dump scenario: the bot at 44616 52536 -2832 (dist 244 from Cobendell
// at z -2792, dz 40) clicks the teacher directly - dist3D 244 is above
// the 200 approach gate but within the 250 interaction gate. Without
// the fix the bot looped forever on the offset ring.
func TestApproachTeacherTalkClickFiresFromTheOffsetRing(t *testing.T) {
	loop, game, bot := newLearnLoop(500)
	loop.phase = phaseTownSell
	loop.tripStart = time.Now()
	loop.tripStops = []tripStop{{
		merchant: townNpc{
			TemplateID: 7156, Name: "Cobendell",
			X: 44823, Y: 52414, Z: -2792,
		},
		teach: true,
	}}
	const teacherID = int32(77)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: teacherID, TemplateID: 7156 + 1000000,
		X: 44823, Y: 52414, Z: -2792, Name: "Cobendell",
	})
	loop.teacherID = teacherID
	loop.teacherPick = time.Now().Add(-2 * selectPeriod)
	// The exact dump position: dist2D 240, dz 40, dist3D 244.
	// dist3D 244 is in (200, 250]: the talk click fires at once,
	// no ground walk. Without the fix the bot looped forever on the
	// offset ring because the talk click waited for dist3D <= 200.
	moveSelfTo(bot, 44616, 52536, -2832)

	game.walks = nil
	game.clicks = nil
	loop.tick()

	// The talk click fires from the dump position (dist3D 244 is
	// within the 250 interaction gate). No ground walk is sent.
	require.Empty(t, game.walks,
		"no ground walk - dist3D 244 is within the 250 interaction "+
			"gate, the talk click fires directly")
	require.Contains(t, game.clicks, teacherID,
		"the talk click fires from the dump position - the server "+
			"INTERACTION_DISTANCE of 250 is met even with the z gap "+
			"(the 2026-09-11 05:45 dump fix)")
	require.NotEqual(t, int32(-1), loop.teacherID,
		"the teacher is not given up (the talk click landed)")
}
