// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The hand verified skill flag sets (skill_flags.go): the ids are
// transcribed from the C1 skill stats, so the tests lock the set
// membership and the table shape against transcription drift.

// TestOverhitSkillSetMembership pins the load-bearing membership:
// the deployment strikes answer true, the nearest unflagged strikes
// answer false, and every flagged id that the C1 stats declare
// castable carries cast data the combat gate can read.
func TestOverhitSkillSetMembership(t *testing.T) {
    require.True(t, OverhitSkill(3), "Power Strike is overhit flagged")
    require.True(t, OverhitSkill(56), "Power Shot is overhit flagged")
    require.False(t, OverhitSkill(16),
        "Mortal Blow carries no overHit flag")
    require.False(t, OverhitSkill(1177),
        "Wind Strike carries no overHit flag")
    for id := range overhitSkills {
        _, ok := SkillCastOf(id)
        require.True(t, ok,
            "overhit skill %d lost its generated cast data", id)
    }
}

// TestSelfHealSkillSetShape pins the self heal set: every member is
// an active SELF target cast (the recovery lookup keys on that
// shape) and the heal descriptions exist for the log lines.
func TestSelfHealSkillSetShape(t *testing.T) {
    for id := range selfHealSkills {
        cast, ok := SkillCastOf(id)
        require.True(t, ok, "self heal %d lost its cast data", id)
        require.Equal(t, "A1", cast.Operate,
            "self heal %d is not an active skill", id)
        require.Equal(t, "SELF", cast.Target,
            "self heal %d does not target the caster", id)
        _, named := SkillInfoOf(id)
        require.True(t, named, "self heal %d lost its name", id)
    }
}

// TestSelfHealInitialConsumeTables pins the hand copied initial
// consume tables: the spot values of both ends of every table, the
// length against the declared level count of the C1 stats and the
// clamp behavior the gate relies on.
func TestSelfHealInitialConsumeTables(t *testing.T) {
    require.Equal(t, int32(15),
        SelfHealInitialConsumeOf(45, 1), "Divine Heal level 1")
    require.Equal(t, int32(21),
        SelfHealInitialConsumeOf(45, 9), "Divine Heal level 9")
    require.Equal(t, int32(8),
        SelfHealInitialConsumeOf(58, 1), "Elemental Heal level 1")
    require.Equal(t, int32(48),
        SelfHealInitialConsumeOf(58, 55), "Elemental Heal level 55")
    require.Equal(t, int32(2),
        SelfHealInitialConsumeOf(1216, 1), "Self Heal level 1")
    require.Equal(t, int32(48),
        SelfHealInitialConsumeOf(58, 99),
        "the table clamps past its last level")
    require.Zero(t, SelfHealInitialConsumeOf(1177, 1),
        "an unknown id answers 0")
    require.Len(t, selfHealInitialConsume[45], 9,
        "Divine Heal declares 9 levels")
    require.Len(t, selfHealInitialConsume[58], 55,
        "Elemental Heal declares 55 levels")
    require.Len(t, selfHealInitialConsume[1216], 1,
        "Self Heal declares 1 level")
}
