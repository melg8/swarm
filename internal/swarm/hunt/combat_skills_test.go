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

// The combat casting of the hunt loop: the warrior strikes of the
// equipped weapon family, the mystic spells, the self buffs between
// the fights and the mana rest of the caster (see combat_skills.go).

// newCasterBot returns a test bot of a mystic class with the given
// mana fill (the maximum of 100 makes the fill read as a percent):
// the class decides the caster behavior, the mana drives its rest
// gates.
func newCasterBot(curMP int32) *state.Bot {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 10, ClassID: 25, Race: 1,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 100, CurMP: curMP,
	})

	return bot
}

// startFight selects the spawned mob and starts the running fight:
// the second tick runs inside the engage phase with the fresh combat
// window, exactly where the combat casting lives.
func startFight(loop *Loop, bot *state.Bot) {
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	bot.ApplySelfTarget(7)
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000,
		TargetZ: -3500,
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
}

// equipSword wears the short sword (item 1, SWORD) in the right hand.
func equipSword(bot *state.Bot) {
	bot.ApplyItemList([]state.InventoryItem{{
		ObjectID: 10, ItemID: 1, Count: 1,
	}})
	var slots [state.PaperdollSlots]int32
	slots[state.PaperdollRHand] = 10
	bot.ApplyPaperdoll(slots)
}

// TestLoopCastsStrikeOfEquippedWeapon pins the warrior path: the
// learned Power Strike (a SWORD and BLUNT strike) fires at the
// selected target while the fight runs and a sword is worn.
func TestLoopCastsStrikeOfEquippedWeapon(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 3, Level: 1, Passive: false},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)

	require.Equal(t, []int32{3}, game.casts,
		"the sword strike fires in the fight")
}

// TestLoopSkipsStrikeOfOtherWeaponFamily pins the weapon gate: the
// learned Mortal Blow (a DAGGER only strike) never fires while a
// sword is worn, whatever its reuse and mana windows say.
func TestLoopSkipsStrikeOfOtherWeaponFamily(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 16, Level: 1, Passive: false},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)
	// A few more paced ticks: the retry period passes, the strike of
	// another weapon family still never goes out.
	for range 10 {
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.castAt = time.Time{}
		loop.tick()
	}

	require.Empty(t, game.casts,
		"the dagger strike never fires with a sword worn")
}

// TestLoopCastsMagicSpellsOfCaster pins the mystic path: the learned
// Wind Strike (a magic spell) fires at the selected target of a
// caster class without any weapon in hand.
func TestLoopCastsMagicSpellsOfCaster(t *testing.T) {
	bot := newCasterBot(80)
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 1177, Level: 1, Passive: false},
	})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)

	require.Equal(t, []int32{1177}, game.casts,
		"the magic attack spell fires in the fight")
}

// TestLoopWarriorNeverCastsSpells pins the role split: a fighter
// class never casts the magic attack spells it happens to know - the
// auto attack stays its only damage.
func TestLoopWarriorNeverCastsSpells(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 1177, Level: 1, Passive: false},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)

	require.Empty(t, game.casts,
		"the warrior never casts the mystic spells")
}

// TestLoopPassiveSkillsNeverCast pins the passive gate: a passive
// skill of the learned list never reaches the cast path.
func TestLoopPassiveSkillsNeverCast(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 3, Level: 1, Passive: true},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)

	require.Empty(t, game.casts)
}

// TestSkillReuseBlocksTheSecondCast pins the local reuse window: the
// strike with its 13 second reuse fires once and the next paced
// ticks hold it until the window closes.
func TestSkillReuseBlocksTheSecondCast(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 3, Level: 1, Passive: false},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	startFight(loop, bot)
	// The cast retry period is 2s, the reuse of Power Strike is 13s
	// plus the margin: the ticks inside the window cast nothing.
	for range 5 {
		bot.ApplyPawnMovement(state.PawnMovement{
			ObjectID: 100, TargetID: 7, Distance: 40,
			X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000,
			TargetZ: -3500,
		})
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.tick()
	}

	require.Equal(t, []int32{3}, game.casts,
		"the reuse window blocks the second cast")
}

// TestSelfBuffCastsBetweenFights pins the buff path: the learned
// Attack Aura (a timed self buff) casts while the effect is missing.
func TestSelfBuffCastsBetweenFights(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 77, Level: 1, Passive: false},
	})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Equal(t, []int32{77}, game.casts,
		"the missing self buff casts between the fights")
}

// TestSelfBuffSkippedWhileEffectRuns pins the tracker gate: the self
// buff skill whose effect already runs on the character never
// re-casts.
func TestSelfBuffSkippedWhileEffectRuns(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 77, Level: 1, Passive: false},
	})
	bot.SetBuffs([]state.BuffEntry{{SkillID: 77, Level: 1, Time: 600}})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Empty(t, game.casts,
		"the running effect holds the buff cast back")
}

// TestSelfBuffSkippedWhileSitting pins the rest gate: a sitting
// character never casts - the cast would stand it up against the
// regeneration.
func TestSelfBuffSkippedWhileSitting(t *testing.T) {
	bot := newTestBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 77, Level: 1, Passive: false},
	})
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Empty(t, game.casts,
		"the sitting character casts no buffs")
}

// TestOutOfManaMageDropsTheFight pins the mana drop: the running
// fight of a dry caster is dropped, no further attack requests go
// out and the character sits down to regenerate.
func TestOutOfManaMageDropsTheFight(t *testing.T) {
	bot := newCasterBot(80)
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 1177, Level: 1, Passive: false},
	})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	// The fight starts while the mana still pays, then the bar drops
	// under the sit threshold.
	loop.tick()
	bot.ApplySelfTarget(7)
	bot.ApplyPawnMovement(state.PawnMovement{
		ObjectID: 100, TargetID: 7, Distance: 40,
		X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000,
		TargetZ: -3500,
	})
	require.Equal(t, int32(7), loop.target)

	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurMP, Value: 10},
	})
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Zero(t, loop.target,
		"the dry mage drops the fight")
	require.Equal(t, []int32{7}, game.forces,
		"no further attack requests of the dropped fight")

	// The next ticks sit the character down.
	loop.lastHit = time.Now().Add(-time.Minute)
	loop.tick()
	require.Equal(t, 1, game.sits,
		"the dry mage sits down to regenerate")
}

// TestManaHeldMageHoldsNewFights pins the mana hold: a caster with
// the mana between the sit and the stand threshold never opens a new
// fight, the standing regeneration tops the bar up first.
func TestManaHeldMageHoldsNewFights(t *testing.T) {
	bot := newCasterBot(40)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	for range 3 {
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.tick()
	}

	require.Empty(t, game.forces,
		"the mana held caster picks no target")
	require.Zero(t, game.sits,
		"the mana above the sit threshold never sits")
}

// TestManaHeldMageAnswersWhenAttacked pins the safety exception: a
// mana held caster under attack still answers instead of standing in
// the blows - the hold only fences the new fights.
func TestManaHeldMageAnswersWhenAttacked(t *testing.T) {
	bot := newCasterBot(40)
	spawnMob(bot)
	mobHitsCharacter(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Equal(t, []int32{7}, game.forces,
		"the attacked caster answers the mob")
	require.Zero(t, game.sits,
		"a mage under attack never sits into the blows")
}

// TestSittingMageStandsWhenManaRecovered pins the stand gate: a
// sitting caster stands up once its mana recovered past the stand
// threshold.
func TestSittingMageStandsWhenManaRecovered(t *testing.T) {
	bot := newCasterBot(70)
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Equal(t, 1, game.sits,
		"the recovered mana stands the caster up")
}

// TestSittingMageKeepsSittingBelowStandThreshold pins the hysteresis:
// a sitting caster with the mana still below the stand threshold
// keeps sitting - no sit/stand toggle churn.
func TestSittingMageKeepsSittingBelowStandThreshold(t *testing.T) {
	bot := newCasterBot(30)
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	for range 3 {
		loop.lastHit = time.Now().Add(-time.Minute)
		loop.tick()
	}

	require.Zero(t, game.sits,
		"the caster keeps sitting until the stand threshold")
}

// TestFighterNeverRestsOnMana pins the role split: a fighter class
// with a deep mana hole neither sits nor holds its fights - the mana
// gates key on the caster answer of the class tree.
func TestFighterNeverRestsOnMana(t *testing.T) {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 10, ClassID: 18, Race: 1,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90, MaxMP: 100, CurMP: 5,
	})
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)
	loop.lastHit = time.Now().Add(-time.Minute)

	loop.tick()

	require.Equal(t, []int32{7}, game.forces,
		"the fighter hunts with an empty mana bar")
	require.Zero(t, game.sits,
		"the fighter never sits on mana")
}

// TestMysticClassPicksCasterGearProfile pins the profile pick: the
// loop of a mystic class swaps its gear scoring to the caster profile
// once the class is known, a fighter class keeps the melee one.
func TestMysticClassPicksCasterGearProfile(t *testing.T) {
	bot := newCasterBot(80)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	loop.tick()

	require.NotNil(t, loop.equip)
	require.Equal(t, "mystic fighter", loop.equip.profile.Name())
	require.True(t, loop.profilePicked,
		"the pick runs once per session")
}

// TestFighterClassKeepsMeleeGearProfile pins the default: the fighter
// loop scores its gear through the melee profile.
func TestFighterClassKeepsMeleeGearProfile(t *testing.T) {
	bot := newTestBot()
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	loop.tick()

	require.NotNil(t, loop.equip)
	require.Equal(t, "melee fighter", loop.equip.profile.Name())
	require.True(t, loop.profilePicked)
}

// TestShoppingPlanFeedsTheWeaponPriority pins the wiring: the weapon
// of the purchase plan reaches the learning queue order - the
// equipped sword puts the sword strikes before the strikes of the
// other weapon families.
func TestShoppingPlanFeedsTheWeaponPriority(t *testing.T) {
	bot := newTestBot()
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 40, ClassID: 18, Race: 1, Sp: 50000,
		X: 45000, Y: 50000, Z: -3500,
		MaxHP: 100, CurHP: 90,
	})
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: 142, Level: 1, Passive: true},
		{SkillID: 194, Level: 1, Passive: true},
	})
	equipSword(bot)
	spawnMob(bot)
	game := &fakeGame{}
	loop := NewLoop(game, bot)

	loop.tick()
	loop.refreshShoppingCache()

	plan := bot.SkillPlan()
	require.NotNil(t, plan)
	powerStrike, mortalBlow := -1, -1
	for i, entry := range plan.Entries {
		switch entry.SkillID {
		case 3:
			powerStrike = i
		case 16:
			mortalBlow = i
		}
	}
	require.GreaterOrEqual(t, powerStrike, 0,
		"the Power Strike lesson is queued")
	require.GreaterOrEqual(t, mortalBlow, 0,
		"the Mortal Blow lesson is queued")
	require.Less(t, powerStrike, mortalBlow,
		"the sword strike sorts before the dagger strike")
}
