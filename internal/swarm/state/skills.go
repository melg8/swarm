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
// skill dictionary (name, icon, passive flag, the tooltip text of the
// learned level). The web UI renders the learned list in active /
// passive tabs.
type SkillSnapshot struct {
    SkillID int32  `json:"skillId"`
    Level   int32  `json:"level"`
    Passive bool   `json:"passive"`
    Name    string `json:"name"`
    Icon    string `json:"icon"`
    Desc    string `json:"desc"`
}

// SkillPlanEntry is one queued lesson of the learning plan: the next
// level of the skill the bot has not learned yet, with the SP cost,
// the character level that unlocks the lesson, the skill book the
// lesson consumes (zero - no book), the book display name, the
// warrior priority category (npcdata.SkillCategory*), the tooltip
// text of the level being learned and the affordability against the
// SP the plan was computed with.
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

// currentSkillQueueLocked returns the cached queue with a freshness
// flag: ok is true when the cache key still matches the current class
// and skills revision (the queue itself may be nil - a class with
// nothing left to learn caches as empty). The peek never rebuilds, so
// the size estimate and the other hint paths stay allocation free.
// The caller must hold a lock.
func (b *Bot) currentSkillQueueLocked() (queue []SkillPlanEntry, ok bool) {
    if c := b.skillQueueCache.Load(); c != nil &&
        c.class == b.char.ClassID && c.revision == b.skillsRevision {
        return c.queue, true
    }

    return nil, false
}

// skillQueueLocked returns the ordered learning queue of the current
// class, learned set and weapon priority, rebuilding the cache when
// the class or the skills revision moved. The cache lives behind an
// atomic pointer instead of plain bot fields: the readers here run
// under the store read lock, and a lazily written field turned two
// concurrent snapshot encoders into writers of the single source of
// truth - a real data race the review of 2026-09-20 caught before a
// test did. Two concurrent rebuilds store two consistent snapshots
// and one wins; the steady state read stays allocation free. The
// returned slice is immutable (every view copies it or reads it value
// by value). The caller must hold a lock.
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

// skillPlanViewLocked builds the learning queue view of the snapshot:
// a copy of the queue with the affordability flags set against the
// current SP, plus the queue total and the missing SP. The copy keeps
// the view safe against the next queue rebuild. The caller must hold
// a lock.
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

// demandedBooksLocked collects the spellbook item ids the character
// still needs for its near term lessons: every queued lesson whose
// unlock level the character already reached demands its spellbook
// (the learning trips buy exactly these books; a looted book of the
// same lessons joins them). The sell and destroy junk flows keep the
// collected items - a book sold for referencePrice/2 comes back as a
// full priced buy of the next learning trip, and the lesson it feeds
// waits forever without it. The set is cached per class, skills
// revision and level behind an atomic pointer for the same reason as
// the queue (see skillQueueLocked): a learn bumps the revision, a
// level up shifts the unlock window, and the next read rebuilds. The
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

// buildSkillQueue computes the ordered learning queue of a class: the
// remaining lessons (not learned yet, not auto granted) sorted by the
// warrior priority - the attack power skills whose weapon condition
// accepts one of the preferred weapon families first (the weapon in
// hand and the next weapon of the purchase plan), the other attack
// power skills second, the defense skills third, everything else
// last; within a priority group the stable tree order (the unlock
// level, then the skill id, then the level) survives. The returned
// slice is the stored queue; the affordability flag is filled by the
// views, never here.
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
// group: 0 the attack power skills usable with a preferred weapon
// (the masteries carry no weapon condition and match every weapon),
// 1 the attack power skills of other weapons, 2 the defense skills,
// 3 the rest.
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
