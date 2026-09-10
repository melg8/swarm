// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// BuffEntry is one active effect of the server AbnormalStatusUpdate
// packet: the buff skill, its level and the remaining seconds (a
// huge value marks the effects the server never times out).
type BuffEntry struct {
	SkillID int32
	Level   int32
	Time    int32
}

// buffRecord is the stored form of one active effect: the learned
// level of the buff skill and the seconds it still had when the
// server last refreshed the list.
type buffRecord struct {
	level int32
	left  int32
}

// BuffSnapshot is one active effect of the snapshot: the skill the
// effect comes from with its level and the remaining seconds, plus
// the resolved display data (name, icon). The web UI buffs widget
// renders the list.
type BuffSnapshot struct {
	SkillID int32  `json:"skillId"`
	Level   int32  `json:"level"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Left    int32  `json:"left"`
}

// SetBuffs applies the full active effect list of the server packet:
// the server sends the complete list whenever an effect changes (a
// buff lands, one expires), so the map is replaced, not merged. The
// arrival time anchors the remaining seconds the views compute.
func (b *Bot) SetBuffs(buffs []BuffEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := make(map[int32]buffRecord, len(buffs))
	for _, buff := range buffs {
		entries[buff.SkillID] = buffRecord{
			level: buff.Level,
			left:  buff.Time,
		}
	}
	b.buffs = entries
	b.buffsAt = time.Now()
	b.touch()
}

// buffLeftCapped bounds the remaining seconds of an effect: the
// server marks the effects it never times out with a huge value
// (2147483647), the views clamp those to a day so the countdown
// never renders as nonsense.
func buffLeftCapped(left int32) int32 {
	if left < 0 {
		return 0
	}
	if left > 86400 {
		return 86400
	}

	return left
}

// SelfHasBuff reports whether the effect of a skill currently runs
// on the character.
func (b *Bot) SelfHasBuff(skillID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.buffs[skillID]

	return ok
}

// SelfManaPercent returns the mana fill of the character in percent
// (0 when the maximum is not known yet).
func (b *Bot) SelfManaPercent() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.char.MaxMP <= 0 {
		return 0
	}

	return b.char.CurMP / b.char.MaxMP * 100
}

// SelfWeaponKind returns the weapon family of the equipped weapon
// (SWORD, BLUNT, DAGGER, BOW, POLE): the weapon of the right hand
// paperdoll slot resolved through the item stats of the generated
// dictionary. The second answer is false when no weapon is worn or
// its family is unknown - the weapon priority of the skill queue
// then falls back to the plain category order.
func (b *Bot) SelfWeaponKind() (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.selfWeaponKindLocked()
}

// selfWeaponKindLocked is the locked form of SelfWeaponKind. The
// caller must hold a lock.
func (b *Bot) selfWeaponKindLocked() (string, bool) {
	objectID := b.paperdoll[PaperdollRHand]
	if objectID == 0 {
		return "", false
	}
	item, ok := b.inventory.lookupLocked(objectID)
	if !ok {
		return "", false
	}
	stats, ok := npcdata.ItemGearStats(item.ItemID)
	if !ok || stats.WeaponType == "" {
		return "", false
	}

	return stats.WeaponType, true
}

// buffSnapshotsLocked builds the active effect list of the snapshot
// sorted by skill id, with the remaining seconds counted down from
// the arrival of the last server list. The caller must hold a lock.
func (b *Bot) buffSnapshotsLocked(now time.Time) []BuffSnapshot {
	if len(b.buffs) == 0 {
		return nil
	}
	elapsed := int32(now.Sub(b.buffsAt).Seconds())
	snapshots := make([]BuffSnapshot, 0, len(b.buffs))
	for id, buff := range b.buffs {
		left := buffLeftCapped(buff.left - elapsed)
		snapshot := BuffSnapshot{
			SkillID: id,
			Level:   buff.level,
			Name:    fmt.Sprintf("skill #%d", id),
			Icon:    "",
			Left:    left,
		}
		if info, ok := npcdata.SkillInfoOf(id); ok {
			snapshot.Name = info.Name
			snapshot.Icon = info.Icon
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].SkillID < snapshots[j].SkillID
	})

	return snapshots
}

// SetSkillWeaponPriority overrides the weapon kinds the learning
// queue prefers: the skills usable with the given weapon families
// (the equipped weapon and the next weapon of the purchase plan)
// sort before the rest of their category. An empty slice restores
// the plain category order.
func (b *Bot) SetSkillWeaponPriority(kinds []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(kinds) == len(b.skillWeapons) {
		match := true
		for i, kind := range kinds {
			if kind != b.skillWeapons[i] {
				match = false

				break
			}
		}
		if match {
			return
		}
	}
	b.skillWeapons = append([]string(nil), kinds...)
	b.skillsRevision++
	b.touch()
}
