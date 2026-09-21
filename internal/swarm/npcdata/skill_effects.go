// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

// SkillEffect is one level effect summary run of a skill: the text
// carries the real per-level value the deployed Mobius C1 skill
// stats bite with (the stat table of the effect element, cross
// checked against the level comment), e.g. Wind Walk 2 answers
// "+33 Speed". Hand curated from the server sources - the set only
// grows entry by entry, every entry cites its evidence in the
// comment above it.
type SkillEffect struct {
    Level int32
    Text  string
}

// skillEffects maps the skill id to the level effect summary runs
// (the run semantics of skillDescs: a run covers its level up to the
// next one). Sources: the skill stat XMLs of
// L2J_Mobius_C1_HarbingersOfWar, files 00000-00099, 01000-01099,
// 01200-01299 and 04000-04099 of dist/game/data/stats/skills; the
// mob provenance of the debuffs cites the spawn file. The set covers
// the effects the bot actually runs (the self auras, the Newbie
// Guide support magic and the mob debuffs of the hunting grounds) -
// an absent skill answers empty and the web UI drops the line
// instead of guessing.
var skillEffects = map[int32][]SkillEffect{
    // Self-cast auras, 00000-00099.xml: the mul tables of the buff
    // effect elements.
    // 72 Iron Will: <mul stat="mDef">#mDef</mul>,
    // #mDef = 1.15 1.23 1.3.
    72: {
        {Level: 1, Text: "+15% M. Def"},
        {Level: 2, Text: "+23% M. Def"},
        {Level: 3, Text: "+30% M. Def"},
    },
    // 77 Attack Aura: <mul stat="pAtk">#rate</mul>,
    // #rate = 1.08 1.12.
    77: {
        {Level: 1, Text: "+8% P. Atk"},
        {Level: 2, Text: "+12% P. Atk"},
    },
    // 86 Reflect Damage: <add stat="reflectDam">#reflectDam</add>,
    // #reflectDam = 10 15 20.
    86: {
        {Level: 1, Text: "Reflect 10 dmg"},
        {Level: 2, Text: "Reflect 15 dmg"},
        {Level: 3, Text: "Reflect 20 dmg"},
    },
    // 91 Defense Aura: <mul stat="pDef">#rate</mul>,
    // #rate = 1.08 1.12.
    91: {
        {Level: 1, Text: "+8% P. Def"},
        {Level: 2, Text: "+12% P. Def"},
    },
    // 99 Rapid Shot: <mul stat="pAtkSpd"><value>#rate</value>
    // <using kind="BOW"/></mul>, #rate = 1.08 1.12.
    99: {
        {Level: 1, Text: "+8% Bow Atk. Spd"},
        {Level: 2, Text: "+12% Bow Atk. Spd"},
    },
    // Newbie Guide support magic, 01000-01099.xml and
    // 01200-01299.xml.
    // 1040 Shield: <mul stat="pDef">#rate</mul>,
    // #rate = 1.08 1.12 1.15.
    1040: {
        {Level: 1, Text: "+8% P. Def"},
        {Level: 2, Text: "+12% P. Def"},
        {Level: 3, Text: "+15% P. Def"},
    },
    // 1044 Regeneration: <mul stat="regHp">#rate</mul>,
    // #rate = 1.1 1.15 1.2.
    1044: {
        {Level: 1, Text: "+10% HP Regen"},
        {Level: 2, Text: "+15% HP Regen"},
        {Level: 3, Text: "+20% HP Regen"},
    },
    // 1045 Bless the Body: <mul stat="maxHp">#maxHp</mul>,
    // #maxHp = 1.1 1.15 1.2 1.25 1.3 1.35.
    1045: {
        {Level: 1, Text: "+10% Max HP"},
        {Level: 2, Text: "+15% Max HP"},
        {Level: 3, Text: "+20% Max HP"},
        {Level: 4, Text: "+25% Max HP"},
        {Level: 5, Text: "+30% Max HP"},
        {Level: 6, Text: "+35% Max HP"},
    },
    // 1048 Bless the Soul: <mul stat="maxMp">#maxMp</mul>,
    // #maxMp = 1.1 1.15 1.2 1.25 1.3 1.35.
    1048: {
        {Level: 1, Text: "+10% Max MP"},
        {Level: 2, Text: "+15% Max MP"},
        {Level: 3, Text: "+20% Max MP"},
        {Level: 4, Text: "+25% Max MP"},
        {Level: 5, Text: "+30% Max MP"},
        {Level: 6, Text: "+35% Max MP"},
    },
    // 1059 Empower: <mul stat="mAtk">#mAtk</mul>,
    // #mAtk = 1.55 1.65 1.75.
    1059: {
        {Level: 1, Text: "+55% M. Atk"},
        {Level: 2, Text: "+65% M. Atk"},
        {Level: 3, Text: "+75% M. Atk"},
    },
    // 1068 Might: <mul stat="pAtk">#rate</mul>,
    // #rate = 1.08 1.12 1.15.
    1068: {
        {Level: 1, Text: "+8% P. Atk"},
        {Level: 2, Text: "+12% P. Atk"},
        {Level: 3, Text: "+15% P. Atk"},
    },
    // 1078 Concentration: <sub stat="cancel">#cancel</sub>,
    // #cancel = 18 25 36 42 48 53.
    1078: {
        {Level: 1, Text: "Magic cancel -18"},
        {Level: 2, Text: "Magic cancel -25"},
        {Level: 3, Text: "Magic cancel -36"},
        {Level: 4, Text: "Magic cancel -42"},
        {Level: 5, Text: "Magic cancel -48"},
        {Level: 6, Text: "Magic cancel -53"},
    },
    // 1085 Acumen: <mul stat="mAtkSpd">#mAtkSpd</mul>,
    // #mAtkSpd = 1.15 1.23 1.3.
    1085: {
        {Level: 1, Text: "+15% Cast. Spd"},
        {Level: 2, Text: "+23% Cast. Spd"},
        {Level: 3, Text: "+30% Cast. Spd"},
    },
    // 1086 Haste: <mul stat="pAtkSpd">#pAtkSpd</mul>,
    // #pAtkSpd = 1.15 1.33.
    1086: {
        {Level: 1, Text: "+15% Atk. Spd"},
        {Level: 2, Text: "+33% Atk. Spd"},
    },
    // 1204 Wind Walk: <add stat="runSpd">#runSpd</add>,
    // #runSpd = 20 33.
    1204: {
        {Level: 1, Text: "+20 Speed"},
        {Level: 2, Text: "+33 Speed"},
    },
    // 4076 the Lirein speed debuff (the only debuff a mob of the
    // elven starting lands casts, npc 20036 of
    // dist/game/data/stats/npcs/20000-20099.xml, spawned by
    // dist/game/data/spawns/ElvenTerritory/ElvenStarting.xml):
    // <mul stat="runSpd">#runSpd</mul> in the Debuff effect,
    // #runSpd = 0.9 0.8 0.7.
    4076: {
        {Level: 1, Text: "Speed -10%"},
        {Level: 2, Text: "Speed -20%"},
        {Level: 3, Text: "Speed -30%"},
    },
}

// SkillEffectOf returns the numeric effect summary of one level of a
// skill: the run that covers the level (the run semantics of
// SkillDescription - each run covers its level up to the next one, a
// level beyond the last run clamps to it, a level below the first
// one answers the first run). The texts carry the real per-level
// values of the deployed Mobius C1 skill stats, so the web UI
// tooltip answers how the buff really bites, e.g. Wind Walk level 2
// answers "+33 Speed". A skill outside the table answers empty (the
// tooltip drops the line instead of guessing).
func SkillEffectOf(id, level int32) string {
    runs := skillEffects[id]
    if len(runs) == 0 {
        return ""
    }
    text := runs[0].Text
    for i := range runs {
        if runs[i].Level > level {
            break
        }
        text = runs[i].Text
    }

    return text
}
