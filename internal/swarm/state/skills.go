// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "fmt"
    "sort"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// skillQueueCap bounds the learning queue of the snapshot: 256 covers the
// few hundred remaining lessons of every C1 class.
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

// SkillSnapshot is one learned skill: the level the server confirmed
// plus the display data resolved from the generated skill dictionary
// (name, icon, passive flag, tooltip of the learned level). The web UI
// renders the learned list in active / passive tabs.
type SkillSnapshot struct {
    SkillID int32  `json:"skillId"`
    Level   int32  `json:"level"`
    Passive bool   `json:"passive"`
    Name    string `json:"name"`
    Icon    string `json:"icon"`
    Desc    string `json:"desc"`
}

// SkillPlanEntry is one queued lesson: the next level of the skill the bot
// has not learned yet, with the SP cost, the unlocking character level, the
// consumed book (zero - no book) and name, the warrior priority category
// (npcdata.SkillCategory*), the tooltip and the affordability against the SP.
type SkillPlanEntry struct {
    SkillID    int32  `json:"skillId"`
    Name       string `json:"name"`
    Icon       string `json:"icon"`
    Desc       string `json:"desc"`
    Level      int32  `json:"level"`
    Passive    bool   `json:"passive"`
    SpCost     int32  `json:"spCost"`
    ReqLevel   int32  `json:"reqLevel"`
    BookItemID int32  `json:"bookItemId"`
    BookName   string `json:"bookName"`
    Category   int    `json:"category"`
    Affordable bool   `json:"affordable"`
}

// SkillPlanView is the published learning queue: the remaining lessons in
// plan order (warrior priorities first, within a category by unlock level),
// the SP the plan was computed against, the queue total and the SP still
// missing. The stored queue carries no affordability flag; each view computes
// it against the SP it read under the same lock.
type SkillPlanView struct {
    Sp      int64            `json:"sp"`
    Total   int64            `json:"total"`
    Missing int64            `json:"missing"`
    Entries []SkillPlanEntry `json:"entries"`
}

// SetSkills applies the full skill list of the server packet: the server
// sends every known skill on entering the world and after every learn, so
// the map is replaced, not merged. The learning queue rebuilds lazily on
// the next snapshot.
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

// skillTotalsLocked returns the SP total of the stored queue and the SP
// still missing against the given wallet. The caller must hold a lock.
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

// skillQueueCache is the immutable cached build of the learning
// queue: the key fields answer "is this still current", the queue
// slice is never mutated after the store.
type skillQueueCache struct {
    class    int32
    revision uint64
    queue    []SkillPlanEntry
}

// bookKeepCache is the immutable cached set of the demanded spellbook
// item ids (see demandedBooksLocked).
type bookKeepCache struct {
    class    int32
    revision uint64
    level    int32
    books    map[int32]bool
}

// currentSkillQueueLocked returns the cached queue with a freshness flag:
// ok is true when the class and skills revision still match (a class with
// nothing left to learn caches as empty). The peek never rebuilds, so the
// hint paths stay allocation free. The caller must hold a lock.
func (b *Bot) currentSkillQueueLocked() (queue []SkillPlanEntry, ok bool) {
    if c := b.skillQueueCache.Load(); c != nil &&
        c.class == b.char.ClassID && c.revision == b.skillsRevision {
        return c.queue, true
    }

    return nil, false
}

// skillQueueLocked returns the ordered learning queue of the current class,
// learned set and weapon priority, rebuilding the cache when either moved.
// The cache sits behind an atomic pointer: the readers run under the store
// read lock, so a lazily written field would make two concurrent snapshot
// encoders writers of the single source of truth, while two rebuilds store
// two consistent snapshots and one wins. The steady state read allocates
// nothing and the slice is immutable. The caller must hold a lock.
func (b *Bot) skillQueueLocked() []SkillPlanEntry {
    if queue, ok := b.currentSkillQueueLocked(); ok {
        return queue
    }
    var queue []SkillPlanEntry
    if b.skills != nil {
        queue = buildSkillQueue(b.char.ClassID, b.skills, b.skillWeapons)
    }
    b.skillQueueCache.Store(&skillQueueCache{
        class:    b.char.ClassID,
        revision: b.skillsRevision,
        queue:    queue,
    })

    return queue
}

// skillPlanViewLocked builds the learning queue view of the snapshot: a
// copy of the queue with the affordability flags set against the current SP,
// plus the queue total and the missing SP, safe against the next rebuild.
// The caller must hold a lock.
func (b *Bot) skillPlanViewLocked() *SkillPlanView {
    queue := b.skillQueueLocked()
    if len(queue) == 0 {
        return nil
    }
    sp := int64(b.char.Sp)
    entries := make([]SkillPlanEntry, len(queue))
    copy(entries, queue)
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

// demandedBooksLocked collects the spellbook item ids of the near term
// lessons (every queued lesson whose unlock level the character reached); the
// learning trips buy them and the junk flows keep them, since a book sold for
// referencePrice/2 comes back as a full priced buy and the lesson it feeds
// waits forever. The set caches per class, skills revision and level; the
// caller must hold a lock.
func (b *Bot) demandedBooksLocked() map[int32]bool {
    if c := b.bookKeepCache.Load(); c != nil &&
        c.class == b.char.ClassID && c.revision == b.skillsRevision &&
        c.level == b.char.Level {
        return c.books
    }
    var books map[int32]bool
    queue := b.skillQueueLocked()
    for i := range queue {
        entry := &queue[i]
        if entry.BookItemID == 0 || entry.ReqLevel > b.char.Level {
            continue
        }
        if books == nil {
            books = make(map[int32]bool, 4)
        }
        books[entry.BookItemID] = true
    }
    b.bookKeepCache.Store(&bookKeepCache{
        class:    b.char.ClassID,
        revision: b.skillsRevision,
        level:    b.char.Level,
        books:    books,
    })

    return books
}

// buildSkillQueue sorts the remaining lessons (not learned, not auto granted)
// by the warrior priority: the attack power skills whose weapon condition
// accepts a preferred family first (the weapon in hand and the next one of the
// purchase plan), the other attack power skills second, the defense skills
// third, the rest last, with the stable tree order (unlock level, skill id,
// level) inside a group. The views fill the affordability flag, never here.
func buildSkillQueue(
    classID int32, learned map[int32]learnedSkill, weapons []string,
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
            SkillID: lesson.SkillID,
            Name:    "",
            Icon:    "",
            Desc: npcdata.SkillDescription(lesson.SkillID,
                lesson.Level),
            Level:      lesson.Level,
            Passive:    false,
            SpCost:     lesson.SpCost,
            ReqLevel:   lesson.GetLevel,
            BookItemID: lesson.BookItem,
            BookName:   npcdata.ItemName(lesson.BookItem),
            Category:   npcdata.SkillCategoryOther,
            Affordable: false,
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
    // resorts by the warrior priority groups. The sort is stable so
    // the tree order survives inside a group.
    sort.SliceStable(queue, func(i, j int) bool {
        return skillPlanPriority(queue[i], weapons) <
            skillPlanPriority(queue[j], weapons)
    })

    return queue
}

// skillPlanPriority maps one queued lesson onto the warrior priority
// group: 0 the attack power skills usable with a preferred weapon (the
// masteries carry no weapon condition and match every weapon), 1 the
// attack power skills of other weapons, 2 the defense skills, 3 the rest.
func skillPlanPriority(entry SkillPlanEntry, weapons []string) int {
    if entry.Category == npcdata.SkillCategoryAttack {
        if len(weapons) == 0 {
            return 0
        }
        cast, ok := npcdata.SkillCastOf(entry.SkillID)
        if ok {
            for _, weapon := range weapons {
                if cast.UsableWithWeapon(weapon) {
                    return 0
                }
            }
        }

        return 1
    }
    if entry.Category == npcdata.SkillCategoryDefense {
        return 2
    }

    return 3
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
            Icon:    "",
            Desc:    "",
        }
        if info, ok := npcdata.SkillInfoOf(id); ok {
            snapshot.Name = info.Name
            snapshot.Icon = info.Icon
            snapshot.Passive = info.Passive
        }
        if desc := npcdata.SkillDescription(id, skill.level); desc != "" {
            snapshot.Desc = desc
        }
        snapshots = append(snapshots, snapshot)
    }
    sort.Slice(snapshots, func(i, j int) bool {
        return snapshots[i].SkillID < snapshots[j].SkillID
    })

    return snapshots
}
