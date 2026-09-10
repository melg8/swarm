// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

// SkillInfo is the static display data of one skill: the resolved
// name, the icon file of the pack (empty when the pack carries no
// icon for the skill - the web UI falls back to a glyph), the
// passive flag (the C1 operateType P of the skill stats) and the
// warrior priority category. Generated into skill_trees.go.
type SkillInfo struct {
	Name     string
	Icon     string
	Passive  bool
	Category int
}

// SkillDesc is one level description run of a skill: the text covers
// every level from Level up to the next run of the skill (the
// generator collapses the consecutive identical level comments into
// one run). The text is the classic client tooltip the Mobius C1
// skill stats carry as XML comments. Generated into skill_trees.go.
type SkillDesc struct {
	Level int32
	Text  string
}

// The warrior priority categories of the skill learning queue: a
// fighter learns the physical weapon attack power skills first, then
// the defense skills, then everything else. The categories come from
// the effect stats of the Mobius C1 skill definitions (a pAtk stat or
// a physical strike is attack power, pDef/sDef/rShld is defense).
const (
	// SkillCategoryAttack marks skills that raise the physical weapon
	// attack power: the weapon masteries, the attack auras and the
	// physical strikes (Power Strike, Power Shot, Mortal Blow and the
	// later class attacks).
	SkillCategoryAttack = 0
	// SkillCategoryDefense marks skills that raise the defense: the
	// armor masteries, the defense auras, the shield skills.
	SkillCategoryDefense = 1
	// SkillCategoryOther marks the rest of the tree: heals, debuffs,
	// magic resistance and the utility skills.
	SkillCategoryOther = 2
)

// SkillLearn is one learnable (skillId, level) pair of a class skill
// tree: the character level the pair unlocks at, the SP cost of the
// lesson, the autoGet flag (the pairs the server grants on its own
// - Lucky, Expertise - never enter the learning queue) and the
// skill book item id the lesson consumes (0 - no book needed; only
// specific levels demand one, the Defence Aura level 1 wants its
// spellbook while levels 2+ do not). Generated into skill_trees.go.
type SkillLearn struct {
	SkillID  int32
	Level    int32
	GetLevel int32
	SpCost   int32
	AutoGet  bool
	BookItem int32
}

// SkillCast is the cast data of one active skill: what the combat
// casting of the hunt loop needs to fire the skill at the right
// moment. Generated into skill_trees.go.
type SkillCast struct {
	// Operate is the operate type of the Mobius skill stats: A1 an
	// active skill (a strike, a spell), A2 a timed buff, P a passive
	// skill (never enters the cast map).
	Operate string
	// Target is the target type: ONE casts at the selected target,
	// SELF lands on the caster (the auras, the heals).
	Target string
	// MPCost is the mana cost per skill level (the list indexes
	// level minus one; shorter lists clamp to the last entry).
	MPCost []int32
	// CastRange is the cast range in world units (40 - melee).
	CastRange int32
	// ReuseDelay is the reuse cooldown in milliseconds.
	ReuseDelay int32
	// HitTime is the cast animation time in milliseconds.
	HitTime int32
	// Magic marks the spells of the mystics (the mana regen and the
	// out of mana rest key on them).
	Magic bool
	// BuffTime is the abnormal time of a timed buff in seconds (0 -
	// not a timed buff).
	BuffTime int32
	// Weapons lists the weapon kinds the using condition demands
	// (SWORD, BLUNT, DAGGER, BOW, POLE); empty - any weapon.
	Weapons []string
}

// TeacherNPC is one skill teacher of the deployment village: the
// packet display id of the NpcInfo packet, the resolved name and the
// spawn point it stands at. Generated into skill_teachers.go.
type TeacherNPC struct {
	TemplateID int32
	Name       string
	X          int32
	Y          int32
	Z          int32
}

// TeachersOfClass returns the village teachers of a class id. The
// list is empty when the class has no teacher in the deployment
// village (an unknown class, a class of another village).
func TeachersOfClass(classID int32) []TeacherNPC {
	return skillTeachers[classID]
}

// AllClassTeachers returns the complete class teacher dictionary of
// the deployment village: the class ids that have at least one
// teacher in the village mapped to their teachers.
func AllClassTeachers() map[int32][]TeacherNPC {
	return skillTeachers
}

// SkillInfoOf returns the static display data of a skill id. The
// second answer is false when the id is unknown to the generated
// dictionary (a skill the server granted but the C1 stats do not
// carry).
func SkillInfoOf(id int32) (SkillInfo, bool) {
	info, ok := skillInfos[id]

	return info, ok
}

// SkillDescription returns the tooltip text of one level of a skill:
// the run that covers the level (the runs walk the level comments of
// the Mobius C1 skill stats, so the exact level answers - the power
// numbers of a strike match the level being learned). A level below
// the first run or beyond the last one clamps to the nearest run, an
// unknown skill answers empty (the web UI drops the description
// block instead of guessing).
func SkillDescription(id, level int32) string {
	runs := skillDescs[id]
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

// SkillTree returns the complete skill tree of a class id (the class
// entries merged with the parent chain, sorted by the unlock level).
// The second answer is false when the class is unknown to the
// generated dictionary.
func SkillTree(classID int32) ([]SkillLearn, bool) {
	tree, ok := skillTrees[classID]

	return tree, ok
}

// SkillCastOf returns the cast data of an active skill id. The
// second answer is false for passive skills and unknown ids - the
// combat casting never fires them.
func SkillCastOf(id int32) (SkillCast, bool) {
	cast, ok := skillCasts[id]

	return cast, ok
}

// MPCostOf returns the mana cost of one level of the skill: the
// per level list clamps to its last entry (shorter tables cover the
// declared levels of the skill), an empty list answers 0.
func (c SkillCast) MPCostOf(level int32) int32 {
	if len(c.MPCost) == 0 {
		return 0
	}
	index := int(level) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(c.MPCost) {
		index = len(c.MPCost) - 1
	}

	return c.MPCost[index]
}

// UsableWithWeapon reports whether the skill accepts the given
// weapon kind: an empty weapon list accepts every weapon, otherwise
// the kind must be one of the demanded ones.
func (c SkillCast) UsableWithWeapon(kind string) bool {
	if len(c.Weapons) == 0 {
		return true
	}
	for _, weapon := range c.Weapons {
		if weapon == kind {
			return true
		}
	}

	return false
}
