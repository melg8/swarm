// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// learnTestBot builds the elven fighter test bot with a learning
// budget: level 5, the given SP and the learned set that leaves the
// level 5 strikes queued.
func learnTestBot(sp int32) *state.Bot {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: sp,
	})
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})

	return bot
}

// newLearnLoop builds a hunt loop with a working navigator and the
// learning test bot.
func newLearnLoop(sp int32) (*Loop, *fakeGame, *state.Bot) {
	bot := learnTestBot(sp)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true})
	loop.lastHit = time.Now().Add(-time.Minute)

	return loop, game, bot
}

// arriveAtStop walks the trip into the current stop: the character
// snaps to the stop npc position, the arrival tick switches into the
// sell phase and the npc spawns in the known list.
func arriveAtStop(t *testing.T, loop *Loop, bot *state.Bot) {
	t.Helper()
	stop := loop.tripStops[0]
	moveSelfTo(bot, stop.merchant.X, stop.merchant.Y, stop.merchant.Z)
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 55, TemplateID: stop.merchant.TemplateID + 1000000,
		X: stop.merchant.X, Y: stop.merchant.Y, Z: stop.merchant.Z,
		Name: stop.merchant.Name,
	})
}

// TestLearnTripTriggersOnTheSkillBudget pins the learning trigger:
// enough SP worth of unlocked lessons arms the town trip exactly like
// the full inventory does, and the reason names the lessons. The learn
// stops plan at the sell stop (one town visit: the gear stops, the
// books, the teacher), so the assertions walk the trip into its first
// stop first.
func TestLearnTripTriggersOnTheSkillBudget(t *testing.T) {
	loop, _, bot := newLearnLoop(500)

	require.False(t, loop.tripActive())
	loop.tick()

	require.Equal(t, phaseTownWalk, loop.phase,
		"the learning budget starts the town trip")
	arriveAtStop(t, loop, bot)
	loop.tick()
	require.True(t, loop.teachStop(),
		"the teach stop follows the sell stop of the trip")
	require.True(t, loop.tripStops[len(loop.tripStops)-1].teach,
		"the last learning stop is the teacher")
}

// TestLearnTripStaysHomeBelowTheBudget pins the trigger threshold:
// a wallet that cannot pay the minimum lesson budget never walks.
func TestLearnTripStaysHomeBelowTheBudget(t *testing.T) {
	loop, _, _ := newLearnLoop(100)

	loop.tick()
	require.False(t, loop.tripActive(),
		"100 sp worth of lessons is below the trip minimum")
}

// TestLearnTripWalksToTheClassTeacher pins the teacher walk: the
// teach stop walks to the class teacher of the character (Ellenia
// for the elven fighter), the arrival switches into the teaching
// and the teacher gets the talk click that the lesson requests
// resolve their trainer through.
func TestLearnTripWalksToTheClassTeacher(t *testing.T) {
	loop, game, bot := newLearnLoop(500)

	loop.tick()
	// The sell stop runs first: no junk, no purchases, the trip
	// advances to the teach stop.
	arriveAtStop(t, loop, bot)
	loop.tick()
	require.True(t, loop.teachStop(), "the teach stop is current")
	require.Equal(t, phaseTownWalk, loop.phase)

	// The teacher arrival: the loop finds Ellenia and walks to her.
	arriveAtStop(t, loop, bot)
	require.True(t, loop.teachStop(),
		"the arrival keeps the teach stop current")
	loop.tick()
	require.Equal(t, int32(55), loop.teacherID,
		"the teacher npc is picked")
	loop.teacherPick = time.Now().Add(-2 * time.Second)
	loop.tick()
	require.Equal(t, []int32{55}, game.clicks,
		"the teacher gets the talk click")
	// The click confirms the selection: the first lesson request
	// goes out (Power Strike level 1 heads the queue).
	bot.ApplySelfTarget(55)
	loop.learnAt = time.Time{}
	loop.learnConfirmAt = time.Now().Add(-2 * learnConfirmWait)
	loop.tick()
	require.NotEmpty(t, game.lessons)
	require.Equal(t, [2]int32{3, 1}, game.lessons[0],
		"Power Strike level 1 is the first lesson")
}

// TestLearnLessonConfirmsBySkillList pins the learn confirmation:
// the SkillList bump after a learned lesson advances the queue to
// the next lesson, one request at a time.
func TestLearnLessonConfirmsBySkillList(t *testing.T) {
	loop, game, bot := newLearnLoop(500)

	loop.tick()
	arriveAtStop(t, loop, bot)
	loop.tick()
	arriveAtStop(t, loop, bot)
	loop.tick()
	require.Equal(t, int32(55), loop.teacherID)
	bot.ApplySelfTarget(55)
	loop.teacherPick = time.Now().Add(-2 * time.Second)
	loop.learnAt = time.Time{}
	loop.tick()
	require.Len(t, game.lessons, 1)

	// The server confirms the learn with a fresh SkillList: the next
	// lesson request fires on the following tick (the confirm tick
	// only consumes the bump).
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 3, Level: 1, Passive: false},
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	stopPos := loop.tripStops[0].merchant
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 440,
		X: stopPos.X, Y: stopPos.Y, Z: stopPos.Z,
	})
	loop.learnAt = time.Time{}
	loop.tick()
	loop.learnAt = time.Time{}
	loop.tick()
	require.Len(t, game.lessons, 2)
	require.Equal(t, [2]int32{3, 2}, game.lessons[1],
		"Power Strike level 2 follows the confirmed level 1")
}

// TestLearnLessonSkipsAfterRetries pins the retry budget: a lesson
// that never confirms (the server refuses silently) is re-requested
// up to the budget and then skipped - the trip does not stall on it.
func TestLearnLessonSkipsAfterRetries(t *testing.T) {
	loop, game, bot := newLearnLoop(500)

	loop.tick()
	arriveAtStop(t, loop, bot)
	loop.tick()
	arriveAtStop(t, loop, bot)
	loop.tick()
	require.Equal(t, int32(55), loop.teacherID)
	bot.ApplySelfTarget(55)
	loop.teacherPick = time.Now().Add(-2 * time.Second)
	loop.learnAt = time.Time{}
	loop.tick()
	require.Len(t, game.lessons, 1)

	// No SkillList bump ever: the retries pile up and the lesson is
	// skipped, the next lesson takes over.
	for i := 0; i <= learnRetries+1; i++ {
		loop.learnConfirmAt = time.Now().Add(-2 * learnConfirmWait)
		loop.learnAt = time.Time{}
		loop.tick()
	}
	require.GreaterOrEqual(t, len(game.lessons), learnRetries+1,
		"the lesson was re-requested up to its budget")
}

// TestLearnTripBuysTheSpellbooks pins the book stop: the lessons that
// demand a spellbook plan its purchase from the town merchant that
// sells spellbooks, the buy stop runs before the teacher stop.
func TestLearnTripBuysTheSpellbooks(t *testing.T) {
	bot := newTestBot()
	// Level 15 with the strikes and the masteries learned: the queue
	// head is the Attack Aura lesson with its spellbook 1095, the
	// Defence Aura spellbook 1294 follows it.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 15, ClassID: 18, Race: 1, Sp: 2000,
	})
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 3, Level: 9, Passive: false},
		{SkillID: 16, Level: 9, Passive: false},
		{SkillID: 56, Level: 9, Passive: false},
		{SkillID: 141, Level: 3, Passive: true},
		{SkillID: 142, Level: 5, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true})
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()
	require.True(t, loop.tripActive())
	// The learn stops plan at the sell stop: walk the trip into its
	// first stop, the arrival tick plans the book stop (merged into
	// the sell stop when the book merchant matches it) and the
	// teacher behind it.
	arriveAtStop(t, loop, bot)
	loop.tick()
	bookStop, teachStop := -1, -1
	for i, stop := range loop.tripStops {
		if stop.teach {
			teachStop = i

			continue
		}
		if len(stop.buys) > 0 && stop.buys[0].Reason == bookReason {
			bookStop = i
		}
	}
	require.NotEqual(t, -1, bookStop, "the trip plans a book stop")
	require.NotEqual(t, -1, teachStop, "the trip plans a teach stop")
	require.Less(t, bookStop, teachStop,
		"the books are bought before the teaching")
	// The spellbooks of the queued lessons: the Attack Aura book 1095
	// and the Defence Aura book 1294 from the village merchant
	// buylist, priced at the reference with the town tax.
	stop := loop.tripStops[bookStop]
	items := make([]int32, 0, len(stop.buys))
	for _, purchase := range stop.buys {
		items = append(items, purchase.ItemID)
		require.Equal(t, int32(7149), purchase.MerchantTemplateID)
		require.Equal(t, int32(3014901), purchase.ListID)
	}
	require.ElementsMatch(t, []int32{1095, 1294}, items)
	for _, purchase := range stop.buys {
		require.Equal(t, int64(86), purchase.Price,
			"the reference price 75 with the 15 percent town tax")
	}
}

// TestTeacherTimeoutSkipsTheLessons pins the teacher wait budget: a
// teacher that never shows up (no NpcInfo in the known list) skips
// the teaching after the wait, the trip continues home.
func TestTeacherTimeoutSkipsTheLessons(t *testing.T) {
	loop, game, bot := newLearnLoop(500)

	loop.tick()
	arriveAtStop(t, loop, bot)
	loop.tick()
	// The teach stop arrival without the teacher npc in the known
	// list: the wait times out and the trip heads home.
	stop := loop.tripStops[0]
	moveSelfTo(bot, stop.merchant.X, stop.merchant.Y, stop.merchant.Z)
	loop.tick()
	require.Equal(t, phaseTownSell, loop.phase)
	require.True(t, loop.teachStop())
	loop.sellPhaseAt = time.Now().Add(-teacherWaitTimeout - time.Second)
	for i := range 60 {
		loop.teacherPick = time.Now().Add(-selectPeriod)
		loop.tick()
		if loop.phase != phaseTownSell || !loop.teachStop() {
			break
		}
		require.Less(t, i, 59, "the teacher wait never timed out")
	}
	require.NotEqual(t, phaseTownSell, loop.phase,
		"the teacher wait gave up and the trip moved on")
	require.Empty(t, game.lessons,
		"no lesson request without the teacher")
}

// TestTripWaitsForTheSkillList pins the enter world race the learning
// stops died on: the packet burst of the login (UserInfo, ItemList,
// SkillList) races the first hunt ticks, and a trip that starts between
// the ItemList and the SkillList would plan without the learning - the
// sessions shopped on their first walk and the teach stop never came.
// The trip start now holds until the skill list arrived (bounded, so a
// server that never lists skills keeps the trips working).
func TestTripWaitsForTheSkillList(t *testing.T) {
	// The fresh session state: the character entered the world, the
	// vitals arrived, the skills have NOT been listed yet.
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 100},
		{ID: state.AttrCurHP, Value: 90},
	})
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true})
	loop.lastHit = time.Now().Add(-time.Minute)
	fillInventory(bot)

	// The inventory is full, but the skill list has not arrived: the
	// trip start holds.
	loop.tick()
	require.False(t, loop.tripActive(),
		"the first trip waits for the server skill list")

	// The list lands (the enter world burst finishes): the trip
	// starts and carries the learning stops of the fresh queue.
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 500,
	})
	loop.tick()
	require.True(t, loop.tripActive(),
		"the trip starts once the skill list arrived")
	require.True(t, loop.learnTripWanted(),
		"the fresh skill queue arms the learning")
}

// TestTripWaitsForTheSkillListAfterAReconnect pins the reconnect half
// of the race: ResetSession drops the skill list and the relogin
// packet burst re-delivers it a second later - a trip that starts in
// that window plans without the learning (the reconnect loops of the
// emergency logout made every trip a shopping trip). The gate keys on
// the SESSION clock, so every reconnect re-arms the wait.
func TestTripWaitsForTheSkillListAfterAReconnect(t *testing.T) {
	bot := learnTestBot(500)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.SetNavigator(&fakeNavigator{found: true})
	loop.lastHit = time.Now().Add(-time.Minute)
	fillInventory(bot)

	// The skills were listed long ago. The session drops and
	// reconnects: the skill list is gone until the enter world burst
	// re-delivers it.
	bot.ResetSession()
	bot.SetCharacter("test1", 100, 18, 45000, 50000, -3500, 50, 30)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 100},
		{ID: state.AttrCurHP, Value: 90},
	})
	fillInventory(bot)
	loop.tick()
	require.False(t, loop.tripActive(),
		"the reconnect re-arms the skill list wait")

	// The list lands: the trip starts with the learning stops.
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, ClassID: 18, Race: 1, Sp: 500,
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.True(t, loop.tripActive())
	require.True(t, loop.learnTripWanted(),
		"the fresh skill queue arms the reconnected learning")
}
