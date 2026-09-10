// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"fmt"
	"sort"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// skillQueueCap bounds the learning queue of the snapshot: the C1
// class trees carry at most a few hundred remaining lessons, so 256
// covers every class while a pathological tree cannot grow the view
// without end.
const skillQueueCap = 256

// LearnedSkill is one entry of the server skill list packet: the
// learned level of the skill and the passive flag the server sent.
type LearnedSkill struct {
	SkillID int32
	Level   int32
	Passive bool
}

// learnedSkill is the stored form of one learned skill.
type learnedSkill struct {
	level   int32
	passive bool
}

// SkillSnapshot is one learned skill of the snapshot: the level the
// server confirmed plus the display data resolved from the generated
// skill dictionary (name, icon, passive flag). The web UI renders the
// learned list in active/passive tabs.
type SkillSnapshot struct {
	SkillID int32  `json:"skillId"`
	Level   int32  `json:"level"`
	Passive bool   `json:"passive"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
}

// SkillPlanEntry is one queued lesson of the learning plan: the next
// level of the skill the bot has not learned yet, with the SP cost,
// the character level that unlocks the lesson, the warrior priority
// category (npcdata.SkillCategory*) and the affordability against the
// SP the plan was computed with. The learning function itself is not
// implemented - the queue only shows the planned order.
type SkillPlanEntry struct {
	SkillID    int32  `json:"skillId"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Level      int32  `json:"level"`
	Passive    bool   `json:"passive"`
	SpCost     int32  `json:"spCost"`
	ReqLevel   int32  `json:"reqLevel"`
	Category   int    `json:"category"`
	Affordable bool   `json:"affordable"`
}

// SkillPlanView is the published learning queue of the web UI: the
// remaining lessons in the order the bot plans to learn them (the
// warrior priorities first - physical weapon attack power, then
// defense, then the rest; within a category by the unlock level), the
// SP the plan was computed against, the total SP the whole queue
// costs and the SP still missing for it. The entries never carry the
// affordability flag from the stored queue - every view (the snapshot
// copy, the live encode) computes it against the SP it read under the
// same lock.
type SkillPlanView struct {
	Sp      int64            `json:"sp"`
	Total   int64            `json:"total"`
	Missing int64            `json:"missing"`
	Entries []SkillPlanEntry `json:"entries"`
}

// SetSkills applies the full skill list of the server packet: the
// learned level and the passive flag of every skill the character
// knows. The server sends the whole list on entering the world and
// after every learn, so the map is replaced, not merged. The learning
// queue rebuilds lazily on the next snapshot (the learned set
// changed).
func (b *Bot) SetSkills(skills []LearnedSkill) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := make(map[int32]learnedSkill, len(skills))
	for _, skill := range skills {
		entries[skill.SkillID] = learnedSkill{
			level:   skill.Level,
			passive: skill.Passive,
		}
	}
	b.skills = entries
	b.skillsRevision++
	b.touch()
}

// skillTotalsLocked walks the stored queue and returns the SP total
// of the whole queue and the SP still missing for it against the
// given wallet. The caller must hold a lock.
func skillTotals(entries []SkillPlanEntry, sp int64) (total, missing int64) {
	for i := range entries {
		total += int64(entries[i].SpCost)
	}
	missing = total - sp
	if missing < 0 {
		missing = 0
	}

	return total, missing
}

// ensureSkillQueueLocked rebuilds the stored learning queue when the
// class or the learned set changed since the last build. A nil skills
// map (never listed, or cleared by a session reset) keeps the queue
// empty: the server lists the learned skills on entering the world,
// so a queue without them would plan lessons the character may
// already know. The stored queue carries the order only - the
// affordability flag stays meaningless on it and is computed by every
// view against the SP it reads under the same lock. The caller must
// hold a lock.
func (b *Bot) ensureSkillQueueLocked() {
	if b.skillQueueClass == b.char.ClassID &&
		b.skillQueueRevision == b.skillsRevision {
		return
	}
	b.skillQueue = nil
	if b.skills != nil {
		b.skillQueue = buildSkillQueue(b.char.ClassID, b.skills)
	}
	b.skillQueueClass = b.char.ClassID
	b.skillQueueRevision = b.skillsRevision
}

// skillPlanViewLocked builds the learning queue view of the snapshot:
// a defensive copy of the stored queue with the affordability flags
// set against the current SP, plus the queue total and the missing
// SP. The copy keeps the view safe against the next queue rebuild.
// The caller must hold a lock.
func (b *Bot) skillPlanViewLocked() *SkillPlanView {
	b.ensureSkillQueueLocked()
	if len(b.skillQueue) == 0 {
		return nil
	}
	sp := int64(b.char.Sp)
	entries := make([]SkillPlanEntry, len(b.skillQueue))
	copy(entries, b.skillQueue)
	for i := range entries {
		entries[i].Affordable = sp >= int64(entries[i].SpCost)
	}
	total, missing := skillTotals(entries, sp)

	return &SkillPlanView{
		Sp:      sp,
		Total:   total,
		Missing: missing,
		Entries: entries,
	}
}

// buildSkillQueue computes the ordered learning queue of a class: the
// remaining lessons (not learned yet, not auto granted) sorted by the
// warrior priority - the physical weapon attack power skills first,
// the defense skills second, everything else last; within a category
// by the unlock level, then the skill id, then the level (the order
// of the generated tree). The returned slice is the stored queue; the
// affordability flag is filled by the views, never here.
func buildSkillQueue(
	classID int32, learned map[int32]learnedSkill,
) []SkillPlanEntry {
	tree, ok := npcdata.SkillTree(classID)
	if !ok {
		return nil
	}
	queue := make([]SkillPlanEntry, 0, min(len(tree), skillQueueCap))
	for _, lesson := range tree {
		if lesson.AutoGet || len(queue) >= skillQueueCap {
			continue
		}
		known := learned[lesson.SkillID].level
		if known >= lesson.Level {
			continue
		}
		entry := SkillPlanEntry{
			SkillID:  lesson.SkillID,
			Level:    lesson.Level,
			SpCost:   lesson.SpCost,
			ReqLevel: lesson.GetLevel,
			Passive:  false,
			Category: npcdata.SkillCategoryOther,
		}
		if info, hasInfo := npcdata.SkillInfoOf(lesson.SkillID); hasInfo {
			entry.Name = info.Name
			entry.Icon = info.Icon
			entry.Passive = info.Passive
			entry.Category = info.Category
		} else {
			entry.Name = fmt.Sprintf("skill #%d", lesson.SkillID)
		}
		queue = append(queue, entry)
	}
	// The tree is sorted by (getLevel, skillId, level); the queue
	// resorts by the warrior priority category first. The sort is
	// stable so the tree order survives inside a category.
	sort.SliceStable(queue, func(i, j int) bool {
		return queue[i].Category < queue[j].Category
	})

	return queue
}

// skillSnapshotsLocked builds the learned skill list of the snapshot
// sorted by skill id (the web UI splits the list into the active and
// passive tabs and sorts each by id). The caller must hold a lock.
func (b *Bot) skillSnapshotsLocked() []SkillSnapshot {
	if len(b.skills) == 0 {
		return nil
	}
	snapshots := make([]SkillSnapshot, 0, len(b.skills))
	for id, skill := range b.skills {
		snapshot := SkillSnapshot{
			SkillID: id,
			Level:   skill.level,
			Passive: skill.passive,
			Name:    fmt.Sprintf("skill #%d", id),
		}
		if info, ok := npcdata.SkillInfoOf(id); ok {
			snapshot.Name = info.Name
			snapshot.Icon = info.Icon
			snapshot.Passive = info.Passive
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].SkillID < snapshots[j].SkillID
	})

	return snapshots
}
