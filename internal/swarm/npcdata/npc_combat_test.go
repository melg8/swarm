// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestNPCCombatProfileKnownTemplate pins the generated row of one elven
// ground mob: the values come from the Mobius npc stats (20400-20499.xml,
// Kaboo Orc Fighter Leader) and the adena from its drop group.
func TestNPCCombatProfileKnownTemplate(t *testing.T) {
    profile, ok := NPCCombatProfile(20472)
    require.True(t, ok)
    require.Equal(t, int32(20472), profile.TemplateID)
    require.Equal(t, int32(12), profile.Level)
    require.InDelta(t, 199.6232, profile.HP, 1e-4)
    require.InDelta(t, 22.0, profile.PAtk, 1e-9)
    require.InDelta(t, 72.0, profile.PDef, 1e-9)
    require.Equal(t, int32(353), profile.Exp)
    require.InDelta(t, 76.65, profile.Adena, 1e-9)
}

// TestNPCCombatProfileUnknownTemplate covers the not-found answers: an
// absent id and the wire id of a known template (the display offset must
// never resolve, the hunt registries hold the stats ids).
func TestNPCCombatProfileUnknownTemplate(t *testing.T) {
    _, ok := NPCCombatProfile(unknownTemplateID)
    require.False(t, ok)

    _, ok = NPCCombatProfile(20472 + NPCDisplayOffset)
    require.False(t, ok)
}

// TestNPCCombatTableSorted guards the binary search contract of the
// generated table.
func TestNPCCombatTableSorted(t *testing.T) {
    require.NotEmpty(t, npcCombatTable)
    for index := 1; index < len(npcCombatTable); index++ {
        require.Less(t, npcCombatTable[index-1].TemplateID,
            npcCombatTable[index].TemplateID, "the row %d breaks the order",
            index)
    }
}

// TestNPCCombatAdenaFollowsTheDropGroup pins the adena formula on two
// grounds: the group chance times the amount range of the adena item.
func TestNPCCombatAdenaFollowsTheDropGroup(t *testing.T) {
    cases := []struct {
        templateID int32
        adena      float64
    }{
        {20472, 76.65},
        {20036, 94.50},
        {20534, 6.30},
    }
    for _, test := range cases {
        profile, ok := NPCCombatProfile(test.templateID)
        require.True(t, ok, "template %d", test.templateID)
        require.InDelta(t, test.adena, profile.Adena, 1e-9,
            "template %d", test.templateID)
    }
}

func BenchmarkNPCCombatProfile(b *testing.B) {
    for range b.N {
        if _, ok := NPCCombatProfile(20472); !ok {
            b.Fatal("the Kaboo Orc Fighter Leader row is missing")
        }
    }
}
