// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The coverage top-up round of issue #9: the exported accessors the
// trip machinery and the probe tools read (the merchant spawn table,
// the self heal flags, the class teacher dictionary) and the clamp
// contracts of the generated-table readers.

func TestMerchantSpawnOf(t *testing.T) {
    t.Run("known merchant spawn resolves", func(t *testing.T) {
        // Lector (7001) is the first entry of the generated table.
        spawn, ok := MerchantSpawnOf(7001)
        require.True(t, ok)
        require.Equal(t, "Lector", spawn.Name)
        require.Equal(t, int32(-86385), spawn.X)
        require.Equal(t, int32(243267), spawn.Y)
        require.Equal(t, int32(-3717), spawn.Z)
    })

    t.Run("unknown template id reports false", func(t *testing.T) {
        spawn, ok := MerchantSpawnOf(unknownTemplateID)
        require.False(t, ok)
        require.Equal(t, MerchantSpawn{}, spawn)
    })
}

func TestMerchantTemplateIDs(t *testing.T) {
    t.Run("count matches the spawn table size", func(t *testing.T) {
        ids := MerchantTemplateIDs()
        require.NotEmpty(t, ids)
        require.Len(t, ids, MerchantSpawnCount())
    })

    t.Run("ids come back sorted ascending", func(t *testing.T) {
        ids := MerchantTemplateIDs()
        for i := 1; i < len(ids); i++ {
            require.Less(t, ids[i-1], ids[i],
                "template ids must sort ascending")
        }
    })

    t.Run("every id resolves through MerchantSpawnOf", func(t *testing.T) {
        for _, id := range MerchantTemplateIDs() {
            _, ok := MerchantSpawnOf(id)
            require.True(t, ok,
                "template id %d of the enumeration must resolve", id)
        }
    })
}

func TestSelfHealSkill(t *testing.T) {
    t.Run("the three known self heals answer true", func(t *testing.T) {
        require.True(t, SelfHealSkill(45))   // Self Heal
        require.True(t, SelfHealSkill(58))   // Divine Heal
        require.True(t, SelfHealSkill(1216)) // Elemental Heal
    })

    t.Run("unknown and zero ids answer false", func(t *testing.T) {
        require.False(t, SelfHealSkill(0))
        require.False(t, SelfHealSkill(unknownTemplateID))
        require.False(t, SelfHealSkill(-1))
    })
}

func TestSelfHealInitialConsumeOf(t *testing.T) {
    t.Run("unknown id answers zero", func(t *testing.T) {
        require.Zero(t, SelfHealInitialConsumeOf(unknownTemplateID, 1))
        require.Zero(t, SelfHealInitialConsumeOf(0, 1))
    })

    t.Run("the per level table answers by level index", func(t *testing.T) {
        // Self Heal (45) carries nine levels, 15 mana up front at
        // the first.
        require.Equal(t, int32(15), SelfHealInitialConsumeOf(45, 1))
        require.Equal(t, int32(16), SelfHealInitialConsumeOf(45, 2))
    })

    t.Run("level zero clamps to the first entry", func(t *testing.T) {
        require.Equal(t, int32(15), SelfHealInitialConsumeOf(45, 0))
        require.Equal(t, int32(15), SelfHealInitialConsumeOf(45, -3))
    })

    t.Run("levels beyond the table clamp to the last entry", func(t *testing.T) {
        require.Equal(t, int32(21), SelfHealInitialConsumeOf(45, 9))
        require.Equal(t, int32(21), SelfHealInitialConsumeOf(45, 99))
    })
}

func TestAllClassTeachers(t *testing.T) {
    t.Run("the dictionary carries the village classes", func(t *testing.T) {
        teachers := AllClassTeachers()
        require.NotEmpty(t, teachers)

        // The elven classes learn from Ellenia and Cobendell.
        elven, ok := teachers[18]
        require.True(t, ok)
        require.NotEmpty(t, elven)
        require.Equal(t, "Ellenia", elven[0].Name)
        require.Equal(t, int32(7155), elven[0].TemplateID)
    })

    t.Run("the dictionary agrees with TeachersOfClass", func(t *testing.T) {
        for classID, teachers := range AllClassTeachers() {
            require.Equal(t, teachers, TeachersOfClass(classID),
                "class %d must read the same list both ways", classID)
        }
    })
}

func TestClassCastsMagic(t *testing.T) {
    t.Run("a mystic class auto casts Wind Strike", func(t *testing.T) {
        // Class 10 (elven mystic) knows Wind Strike from level 1.
        require.True(t, ClassCastsMagic(10))
    })

    t.Run("a fighter class never casts magic", func(t *testing.T) {
        // Class 0 (human fighter) auto gets only the mortal skills.
        require.False(t, ClassCastsMagic(0))
    })

    t.Run("an unknown class answers false", func(t *testing.T) {
        require.False(t, ClassCastsMagic(unknownTemplateID))
    })
}

func TestSkillCastMPCostOf(t *testing.T) {
    t.Run("an empty table answers zero at every level", func(t *testing.T) {
        cast := SkillCast{}
        require.Zero(t, cast.MPCostOf(1))
        require.Zero(t, cast.MPCostOf(0))
        require.Zero(t, cast.MPCostOf(99))
    })

    t.Run("levels below the table clamp to the first entry", func(t *testing.T) {
        cast := SkillCast{MPCost: []int32{10, 20, 30}}
        require.Equal(t, int32(10), cast.MPCostOf(0))
        require.Equal(t, int32(10), cast.MPCostOf(-2))
        require.Equal(t, int32(10), cast.MPCostOf(1))
    })

    t.Run("levels beyond the table clamp to the last entry", func(t *testing.T) {
        cast := SkillCast{MPCost: []int32{10, 20, 30}}
        require.Equal(t, int32(30), cast.MPCostOf(3))
        require.Equal(t, int32(30), cast.MPCostOf(30))
    })
}

func TestItoa(t *testing.T) {
    t.Run("zero renders", func(t *testing.T) {
        require.Equal(t, "0", itoa(0))
    })

    t.Run("positive values render in order", func(t *testing.T) {
        require.Equal(t, "7", itoa(7))
        require.Equal(t, "123456", itoa(123456))
    })

    t.Run("negative values carry the sign", func(t *testing.T) {
        require.Equal(t, "-42", itoa(-42))
    })

    t.Run("the system message fallback routes through it", func(t *testing.T) {
        // An unmapped id falls back to the numbered text: the
        // fallback is the itoa consumer the params never reach.
        require.Equal(t, "system message 999999",
            SystemMessageText(999999))
    })
}
