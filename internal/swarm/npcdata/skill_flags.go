// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

// The hand-verified skill flag sets of the Mobius C1 server stats.
// The generated dictionaries (skill_trees.go) carry the cast data
// and the descriptions, but not every XML attribute the bot behavior
// keys on - these two sets cover the attributes that do not fit the
// SkillCast shape. Every id below is read off the official C1 skill
// stats (dist/game/data/stats/skills/00000-00299.xml), the file and
// the line of each entry are the verification trail.

// overhitSkills holds every C1 skill whose stats carry
// <overHit>true</overHit>. The server mechanic (Attackable.java
// setOverhitValues L1302, calculateOverhitExp L1480): when the cast
// of such a skill resolves on an attackable, the target arms its
// overhit flag; the flag survives only until the next damage event,
// and if that event kills the mob, the killer gains
// exp * min(overkill / maxHp, 0.25) on top of the base share - the
// bonus caps at +25 percent. Source: the <overHit> entries of
// 00000-00099.xml (L120, 182, 1479, 1664, 1996, 2451, 2844, 3265,
// 4129), 00100-00199.xml (L42, 181, 1266, 2334) and 00200-00299.xml
// (L502, 886, 1210, 1471, 1595, 3424, 3566, 3815). The deployment
// classes meet the flag on Power Strike (3) and Power Shot (56).
var overhitSkills = map[int32]bool{
    1:   true, // Triple Slash
    3:   true, // Power Strike
    19:  true, // Double Shot
    24:  true, // Burst Shot
    29:  true, // Iron Punch
    36:  true, // Whirlwind
    48:  true, // Thunder Storm
    56:  true, // Power Shot
    81:  true, // Punch of Doom
    100: true, // Stun Attack
    101: true, // Stun Shot
    120: true, // Stunning Fist
    190: true, // Fatal Strike
    223: true, // Sting
    245: true, // Wild Sweep
    255: true, // Power Smash
    260: true, // Hammer Crush
    261: true, // Triple Sonic Slash
    280: true, // Burning Fist
    281: true, // Soul Breaker
    284: true, // Hurricane Assault
}

// OverhitSkill reports whether the skill id carries the C1 overhit
// flag: casting it as the killing blow pays the overhit experience
// bonus. The hunt loop holds these strikes back until the target
// stands in the finishing window, so the cast lands the kill.
func OverhitSkill(id int32) bool {
    return overhitSkills[id]
}

// selfHealSkills holds the instant self heals of the C1 stats: the
// active skills that target SELF and carry a Heal effect. Source:
// the TARGET_SELF blocks with <effect name="Heal"> of the skill
// stats - Divine Heal (00000-00099.xml), Elemental Heal (same file),
// Self Heal (01200-01299.xml L533, power 42, mp 7 plus 2 initial,
// reuse 10 s). The mystic starting classes auto learn Self Heal; a
// server may grant any of the three to any class, so the recovery
// keys on the skill list the server sent, not on the class tree.
var selfHealSkills = map[int32]bool{
    45:   true, // Divine Heal
    58:   true, // Elemental Heal
    1216: true, // Self Heal
}

// SelfHealSkill reports whether the skill id is an instant self
// heal: casting it recovers the caster's own HP. The rest logic of
// the hunt loop casts one instead of sitting down while the mana
// pays and no blow is landing.
func SelfHealSkill(id int32) bool {
    return selfHealSkills[id]
}

// selfHealInitialConsume holds the per level mpInitialConsume of
// the self heal skills (the mana the server burns before the cast
// resolves, on top of mpConsume - the server gate sums both,
// Creature.java L2049: getMpConsume + getMpInitialConsume, so the
// bot gate must too). The tables are hand copied from
// the C1 stats: 01200-01299.xml (Self Heal, one level), and the
// #mpInitialConsume tables of Divine Heal and Elemental Heal in
// 00000-00099.xml. An id outside the map answers 0.
var selfHealInitialConsume = map[int32][]int32{
    45: {15, 16, 17, 18, 18, 18, 20, 21, 21},
    58: {8, 9, 9, 11, 12, 12, 13, 13, 14, 15, 16, 17, 18, 18,
        19, 20, 21, 22, 23, 24, 25, 25, 26, 26, 27, 28, 29, 30,
        31, 32, 32, 32, 33, 34, 35, 36, 36, 37, 38, 39, 39, 39,
        40, 41, 42, 42, 43, 44, 45, 45, 46, 46, 47, 48, 48},
    1216: {2},
}

// SelfHealInitialConsumeOf returns the initial mana consume of one
// level of a self heal skill (the tables clamp to their last entry
// like MPCostOf does, an unknown id answers 0). The generated cast
// tables carry mpConsume only - folding the initial consume in is
// the generator follow up, this hand table covers the three skills
// the recovery gates until then.
func SelfHealInitialConsumeOf(id, level int32) int32 {
    table, ok := selfHealInitialConsume[id]
    if !ok || len(table) == 0 {
        return 0
    }
    index := int(level) - 1
    if index < 0 {
        index = 0
    }
    if index >= len(table) {
        index = len(table) - 1
    }

    return table[index]
}
