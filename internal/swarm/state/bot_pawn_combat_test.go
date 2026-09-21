// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// A pawn movement toward a known non attackable npc is the interact
// walk of the Mobius player AI (the approach to the ~36 unit talk
// distance): it aims the character at the npc but marks no combat, so
// selecting a buffer or a shop on the map never lights the combat
// banner for an interaction.
func TestSelfPawnMovementTowardFolkNpcMarksNoCombat(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 55, TemplateID: 1000001, Attackable: false,
        X: 45200, Y: 50000, Name: "Buffer",
    })

    bot.ApplyPawnMovement(PawnMovement{
        ObjectID: 100, TargetID: 55, Distance: 36,
        X: 45150, Y: 50000, Z: -3500,
        TargetX: 45200, TargetY: 50000, TargetZ: -3500,
    })

    require.False(t, bot.SelfFighting(55),
        "an interact walk toward a folk npc must not mark the fight")
    require.False(t, bot.Snapshot().Character.InCombat,
        "an interact walk toward a folk npc must not arm the combat window")
    require.Equal(t, int32(55), bot.Snapshot().Character.TargetID,
        "the interact walk still aims the character at the npc")
}

// The same pawn movement toward an attackable npc stays an attack
// chase: it marks the fresh fight and arms the combat window exactly
// like before the interact exception existed.
func TestSelfPawnMovementTowardMobMarksCombat(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 46000, Y: 50000, Name: "Gremlin",
    })

    bot.ApplyPawnMovement(PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 60,
        X: 45900, Y: 50000, Z: -3500,
        TargetX: 46000, TargetY: 50000, TargetZ: -3500,
    })

    require.True(t, bot.SelfFighting(7),
        "a chase toward a mob must mark the fresh fight")
    require.True(t, bot.Snapshot().Character.InCombat,
        "a chase toward a mob must arm the combat window")
}

// A pawn movement toward an unknown object keeps the fight marking:
// the chased mob may be unknown to the tracker yet (the npc info can
// lag the chase packet), so the safe default stays combat.
func TestSelfPawnMovementTowardUnknownMarksCombat(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    bot.ApplyPawnMovement(PawnMovement{
        ObjectID: 100, TargetID: 77, Distance: 60,
        X: 45900, Y: 50000, Z: -3500,
        TargetX: 46000, TargetY: 50000, TargetZ: -3500,
    })

    require.True(t, bot.SelfFighting(77),
        "a chase toward an unknown object must keep the combat default")
}
