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
// lesson and the autoGet flag (the pairs the server grants on its own
// - Lucky, Expertise - never enter the learning queue). Generated
// into skill_trees.go.
type SkillLearn struct {
	SkillID  int32
	Level    int32
	GetLevel int32
	SpCost   int32
	AutoGet  bool
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
