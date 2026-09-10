// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The reproduction of the reported relogin loop (2026-09-10): the bot
// ran to a hunting ground through aggressive mobs, the pile up run of
// the emergency logout reset the train, and the fresh session after
// the relogin walked straight to the NEXT ground, ignoring the one it
// had just arrived at. Two causes worked together: the fleet claim of
// the spot leaked on every session death (nothing called the leave
// function of the dead loop - runBot builds a fresh spotHunter per
// session), so each relogin left one more ghost hunter in the process
// wide hub and halved the score of the standing ground in the
// occupancy division; and the fresh session re-contested the whole
// spot economy instead of resuming the ground the character stood on.
// The tests below pin both halves of the fix.

// reloginBot builds the fresh session after a relogin: a level 6
// character entering the world at the given position (the place the
// panic run of the previous session ended on).
func reloginBot(x int32, y int32) *state.Bot {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 6, X: x, Y: y, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
	})

	return bot
}

// reloginLoop arms a fresh hunt loop in the spot mode for the
// relogin bot. The claim of the picked spot releases when the test
// ends.
func reloginLoop(t *testing.T, bot *state.Bot) (*Loop, *bytes.Buffer) {
	t.Helper()
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingSpots(testSpots())
	logBuf := &bytes.Buffer{}
	loop.SetLogger(log.New(logBuf, "", 0))
	hunter := loop.spot
	t.Cleanup(func() {
		hunter.releaseClaim()
	})

	return loop, logBuf
}

// TestSpotClaimReleasedOnSessionEnd pins the leak fix itself: the
// claim of the hunted spot lives exactly as long as the loop
// goroutine. The 24/7 supervisor reconnects through runBotForever -
// the session end cancels the loop context, and the deferred release
// hands the claim back to the hub instead of leaving a ghost hunter
// that halves the ground score of every future pick.
func TestSpotClaimReleasedOnSessionEnd(t *testing.T) {
	bot := spotTestBot(t)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingSpots(testSpots())
	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)
	require.Eventually(t, func() bool {
		return globalSpotHub.occupancy("test-home") == 1
	}, 2*time.Second, 50*time.Millisecond,
		"the running loop holds the claim of its spot")
	cancel()
	require.Eventually(t, func() bool {
		return globalSpotHub.occupancy("test-home") == 0
	}, 2*time.Second, 50*time.Millisecond,
		"the session end releases the claim of the dead loop")
}

// TestReloginResumesStandingGroundDespiteGhostClaim pins the reported
// scene end to end: the fresh session lands on the ground of the spot
// it had just walked to (600 units off the anchor, where the panic
// run ended - still inside the spot circle) and resumes THAT ground,
// even though the hub still carries a ghost claim of a session that
// died before the release fix (the occupancy division alone would
// have walked it to the neighbor ground).
func TestReloginResumesStandingGroundDespiteGhostClaim(t *testing.T) {
	ghostLeave := globalSpotHub.enter("test-home")
	t.Cleanup(ghostLeave)

	bot := reloginBot(46712, 41500)
	loop, logBuf := reloginLoop(t, bot)
	loop.tick()

	require.Equal(t, "test-home", loop.zonePickedID)
	require.Contains(t, logBuf.String(),
		"resuming the spot Home Keltirs - the login landed on its ground")
}

// TestReloginOffGroundFallsToScoredPick covers the relogin that lands
// BETWEEN the grounds (the session died mid-walk): no standing ground
// exists, the scored pick of the economy owns the choice - the value
// of the richer ground beats the proximity edge of the nearer one.
func TestReloginOffGroundFallsToScoredPick(t *testing.T) {
	bot := reloginBot(46112, 48000)
	loop, _ := reloginLoop(t, bot)
	loop.tick()

	require.Equal(t, "test-rich", loop.zonePickedID)
}

// TestStandingGroundNeverResumesOutgrownSpot guards the eligibility
// gate of the relogin handoff: a character that outgrew the window of
// the ground it stands on (the level 13 character on the level 1-5
// keltir field) never resumes it - the economy picks the ground the
// level window actually serves.
func TestStandingGroundNeverResumesOutgrownSpot(t *testing.T) {
	bot := reloginBot(46112, 41500)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 13, X: 46112, Y: 41500, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 40, CurMP: 30,
	})
	loop, _ := reloginLoop(t, bot)
	loop.tick()

	require.Equal(t, "test-rich", loop.zonePickedID)
}

// TestStandingGroundPrefersNearestAnchorOfOverlap covers the tie of
// overlapping spot circles: the fresh session standing in the overlap
// of two grounds takes the NEAREST anchor, deterministically.
func TestStandingGroundPrefersNearestAnchorOfOverlap(t *testing.T) {
	// The rich circle (anchor 49000 54000, radius 1800) overlaps a
	// home circle shifted onto it for the test: build the registry
	// with the home anchor moved next to the rich anchor.
	spots := testSpots()
	spots[0].AnchorX, spots[0].AnchorY = 50600, 54000
	bot := reloginBot(50000, 54000)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetHuntingSpots(spots)
	hunter := loop.spot
	t.Cleanup(func() {
		hunter.releaseClaim()
	})
	loop.tick()

	// 600 units from the home anchor, 1000 from the rich one.
	require.Equal(t, "test-home", loop.zonePickedID)
}
