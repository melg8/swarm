// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "sort"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// SkillCast is one skill cast broadcast of the server (the Mobius C1
// MagicSkillUse packet): the caster opens a cast window of HitTimeMs
// (the skill resolves when it elapses) and a reuse window of
// ReuseDelayMs (the cooldown). The web view fills the skill icon
// while the cast window runs and shows the cooldown countdown while
// the reuse window runs.
type SkillCast struct {
    CasterID   int32
    TargetID   int32
    SkillID    int32
    SkillLevel int32
    // HitTimeMs is the cast time in milliseconds, ReuseDelayMs the
    // cooldown in milliseconds.
    HitTimeMs    int32
    ReuseDelayMs int32
}

// skillCastWindow is the stored cast and reuse state of one skill of
// the played character: the absolute end times of the two windows and
// the totals the progress bars divide the remainders by.
type skillCastWindow struct {
    castUntil  time.Time
    castTotal  time.Duration
    reuseUntil time.Time
    reuseTotal time.Duration
}

// SkillStateView is the live cast and reuse state of one skill of the
// played character in the snapshot: the milliseconds left of each
// window at the snapshot moment plus the window totals. A window that
// already elapsed reads zero; a skill with both windows elapsed drops
// out of the section entirely. Icon is the display art of the skill,
// so the map cast icon draws the casting skill whatever its source -
// a learned skill or an item skill of the bag like the Scroll of
// Escape (never a learned skill).
type SkillStateView struct {
    SkillID      int32  `json:"skillId"`
    Icon         string `json:"icon"`
    CastLeftMs   int64  `json:"castLeftMs"`
    CastTotalMs  int64  `json:"castTotalMs"`
    ReuseLeftMs  int64  `json:"reuseLeftMs"`
    ReuseTotalMs int64  `json:"reuseTotalMs"`
}

// ApplySkillCast records the skill cast broadcast of the played
// character: the hit time opens the cast window and the reuse delay
// the cooldown window of the skill. The broadcasts of the other
// creatures open nothing - the cast icon of the map and the skills
// widget show the played character's own casting.
func (b *Bot) ApplySkillCast(cast SkillCast) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if cast.CasterID != b.selfID {
        return
    }
    now := b.now()
    if b.skillCasts == nil {
        b.skillCasts = make(map[int32]skillCastWindow, 8)
    }
    window := b.skillCasts[cast.SkillID]
    if cast.HitTimeMs > 0 {
        window.castUntil = now.Add(
            time.Duration(cast.HitTimeMs) * time.Millisecond)
        window.castTotal = time.Duration(cast.HitTimeMs) *
            time.Millisecond
    }
    if cast.ReuseDelayMs > 0 {
        window.reuseUntil = now.Add(
            time.Duration(cast.ReuseDelayMs) * time.Millisecond)
        window.reuseTotal = time.Duration(cast.ReuseDelayMs) *
            time.Millisecond
    }
    b.skillCasts[cast.SkillID] = window
    b.pruneSkillCastsLocked(now)
    b.touch()
}

// pruneSkillCastsLocked drops the windows both of whose runs elapsed
// (the map stays at the size of the freshly used skills). The caller
// must hold the write lock.
func (b *Bot) pruneSkillCastsLocked(now time.Time) {
    for id, window := range b.skillCasts {
        if window.castUntil.Before(now) && window.reuseUntil.Before(now) {
            delete(b.skillCasts, id)
        }
    }
}

// SkillStates returns the live cast and reuse windows of the played
// character: the same rows the snapshot carries, sorted by skill id.
// The acceptance scenario watches the Scroll of Escape cast here.
func (b *Bot) SkillStates() []SkillStateView {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skillStateViewsLocked(b.now())
}

// skillStateViewsLocked builds the skillStates section of the
// snapshot: the live windows sorted by skill id (the deterministic
// wire order), the elapsed windows read zero and the skills with both
// windows elapsed drop out. The caller must hold a lock.
func (b *Bot) skillStateViewsLocked(now time.Time) []SkillStateView {
    if len(b.skillCasts) == 0 {
        return nil
    }
    views := make([]SkillStateView, 0, len(b.skillCasts))
    for id, window := range b.skillCasts {
        castLeft := max(0, window.castUntil.Sub(now))
        reuseLeft := max(0, window.reuseUntil.Sub(now))
        if castLeft <= 0 && reuseLeft <= 0 {
            continue
        }
        view := SkillStateView{
            SkillID:      id,
            Icon:         "",
            CastLeftMs:   castLeft.Milliseconds(),
            CastTotalMs:  window.castTotal.Milliseconds(),
            ReuseLeftMs:  reuseLeft.Milliseconds(),
            ReuseTotalMs: window.reuseTotal.Milliseconds(),
        }
        if info, ok := npcdata.SkillInfoOf(id); ok {
            view.Icon = info.Icon
        }
        views = append(views, view)
    }
    sort.Slice(views, func(i, j int) bool {
        return views[i].SkillID < views[j].SkillID
    })

    return views
}
