// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// newFarmBot builds a tracker with the injected level 15 start state.
func newFarmBot() *state.Bot {
	bot := state.NewBot(farmAccount)
	bot.SetCharacter(farmAccount, 100, 18, elvenSpawnX, elvenSpawnY,
		elvenSpawnZ, level15HP, level15MP)
	bot.ApplyUserInfo(state.UserInfo{
		Name:    farmAccount,
		Level:   15,
		ClassID: 18,
		X:       elvenSpawnX,
		Y:       elvenSpawnY,
		Z:       elvenSpawnZ,
		MaxHP:   level15HP,
		CurHP:   level15HP,
		MaxMP:   level15MP,
		CurMP:   level15MP,
	})

	return bot
}

// TestEvaluateEquipmentEmpty pins the start state: the reset
// character wears nothing, the check must not pass.
func TestEvaluateEquipmentEmpty(t *testing.T) {
	bot := newFarmBot()
	counts, proper := evaluateEquipment(bot)
	require.False(t, proper)
	require.Equal(t, 0, counts.weapon)
	require.Equal(t, 0, counts.armor())
	require.Equal(t, 0, counts.jewels)
}

// TestEvaluateEquipmentFull pins the pass state: the weapon, the
// chest, four armor pieces and a jewel read as the proper gear set.
func TestEvaluateEquipmentFull(t *testing.T) {
	bot := newFarmBot()
	doll := [state.PaperdollSlots]int32{}
	doll[state.PaperdollRHand] = 101
	doll[state.PaperdollChest] = 102
	doll[state.PaperdollLegs] = 103
	doll[state.PaperdollHead] = 104
	doll[state.PaperdollGloves] = 105
	doll[state.PaperdollFeet] = 106
	doll[state.PaperdollRFinger] = 107
	bot.ApplyPaperdoll(doll)

	counts, proper := evaluateEquipment(bot)
	require.True(t, proper)
	require.Equal(t, 1, counts.weapon)
	require.Equal(t, 5, counts.armor())
	require.Equal(t, 1, counts.jewels)
}

// TestEvaluateEquipmentWeaponOnly pins the gate: a lone weapon (the
// naked shopping start) is not the proper gear set.
func TestEvaluateEquipmentWeaponOnly(t *testing.T) {
	bot := newFarmBot()
	doll := [state.PaperdollSlots]int32{}
	doll[state.PaperdollRHand] = 101
	bot.ApplyPaperdoll(doll)

	_, proper := evaluateEquipment(bot)
	require.False(t, proper)
}

// TestEvaluateSkillsAuras pins the aura gate: the skill check only
// passes with both auras learned and no affordable lesson left.
func TestEvaluateSkillsAuras(t *testing.T) {
	bot := newFarmBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: attackAuraSkillID, Level: 1, Passive: false},
		{SkillID: defenseAuraSkillID, Level: 1, Passive: false},
	})
	_, done := evaluateSkills(bot)
	require.True(t, done)

	bot.SetSkills([]state.LearnedSkill{
		{SkillID: attackAuraSkillID, Level: 1, Passive: false},
	})
	detail, done := evaluateSkills(bot)
	require.False(t, done)
	require.Contains(t, detail, "auras")
}

// TestEvaluateSkillsAffordableLeft pins the affordability gate: a
// learnable lesson below the level keeps the check open.
func TestEvaluateSkillsAffordableLeft(t *testing.T) {
	bot := newFarmBot()
	bot.SetSkills([]state.LearnedSkill{
		{SkillID: attackAuraSkillID, Level: 1, Passive: false},
		{SkillID: defenseAuraSkillID, Level: 1, Passive: false},
	})
	// The cheapest unlocked lessons of the elven fighter cost 60 sp:
	// a wallet below it keeps the queue affordable empty, above it one
	// lesson is due.
	bot.ApplyUserInfo(state.UserInfo{
		Name: farmAccount, Level: 15, ClassID: 18, Sp: 10,
	})
	_, done := evaluateSkills(bot)
	require.True(t, done)
	bot.ApplyUserInfo(state.UserInfo{
		Name: farmAccount, Level: 15, ClassID: 18, Sp: 5000,
	})
	detail, done := evaluateSkills(bot)
	require.False(t, done)
	require.Contains(t, detail, "affordable")
}

// TestEvaluateZoneAndKill pins the zone membership and the kill
// placement checks.
func TestEvaluateZoneAndKill(t *testing.T) {
	bot := newFarmBot()
	bot.SetHuntingZone(45000, 41000, 500)
	bot.ApplyUserInfo(state.UserInfo{
		Name: farmAccount, Level: 15, ClassID: 18,
		X: 45100, Y: 41100, Z: elvenSpawnZ,
	})
	_, inside, zone := evaluateZone(bot)
	require.True(t, inside)
	require.NotNil(t, zone)

	runStart := int64(1000)
	bot.SetKillMarks([]state.KillMarkView{
		{BotID: farmAccount, X: 45150, Y: 41150, AtMs: 900},
		{BotID: farmAccount, X: 46000, Y: 41500, AtMs: 1100},
		{BotID: farmAccount, X: 45100, Y: 41200, AtMs: 1200},
	})
	detail, killed := evaluateKill(bot, zone, runStart)
	require.True(t, killed)
	require.Contains(t, detail, "killed at")

	// A kill of the previous run never counts: the AtMs filter.
	bot.SetKillMarks([]state.KillMarkView{
		{BotID: farmAccount, X: 45150, Y: 41150, AtMs: 900},
	})
	_, killed = evaluateKill(bot, zone, runStart)
	require.False(t, killed)

	// A kill outside the zone never counts.
	bot.SetKillMarks([]state.KillMarkView{
		{BotID: farmAccount, X: 46000, Y: 41500, AtMs: 1100},
	})
	_, killed = evaluateKill(bot, zone, runStart)
	require.False(t, killed)
}

// TestEvaluateZoneWithoutPosition pins the tolerant path: a zone
// without a position (the early session) reads as not inside.
func TestEvaluateZoneWithoutPosition(t *testing.T) {
	bot := state.NewBot(farmAccount)
	bot.SetHuntingZone(45000, 41000, 500)
	detail, inside, zone := evaluateZone(bot)
	require.False(t, inside)
	require.NotNil(t, zone)
	require.Contains(t, detail, "position")

	bot2 := state.NewBot(farmAccount)
	detail2, inside2, zone2 := evaluateZone(bot2)
	require.False(t, inside2)
	require.Nil(t, zone2)
	require.Contains(t, detail2, "hunting zone")
}

// TestFarmReadinessReset pins the injected start state of the user
// scenario: level 15, the 20k sp, the 100k adena wallet, the elven
// creation spawn.
func TestFarmReadinessReset(t *testing.T) {
	reset := farmReadinessReset(farmAccount)
	require.Equal(t, int32(15), reset.Level)
	require.Equal(t, int64(20000), reset.SP)
	require.Equal(t, int64(100000), reset.Adena)
	require.Equal(t, int32(elvenSpawnX), reset.X)
	require.Equal(t, int32(elvenSpawnY), reset.Y)
	require.Equal(t, int32(elvenSpawnZ), reset.Z)
	require.Equal(t, reset.Exp, int64(level15Exp))
}

// TestFarmChecksCoverTheStory pins the check list of the scenario:
// the narrative order of the user story, every condition of the pass
// state represented.
func TestFarmChecksCoverTheStory(t *testing.T) {
	checks := farmChecks()
	require.Len(t, checks, 6)
	ids := map[string]bool{}
	for _, check := range checks {
		ids[check.ID] = true
		require.NotEmpty(t, check.Label)
	}
	for _, id := range []string{
		checkOnline, checkEquipped, checkSkills,
		checkZone, checkBuffs, checkKill,
	} {
		require.True(t, ids[id], "missing check "+id)
	}
}
