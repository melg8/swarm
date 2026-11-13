// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The recovery self heal of the rest logic: a character whose skill list holds
// an instant self heal (npcdata.SelfHealSkill) casts it instead of sitting down
// while the mana pays and no blow lands. A sitting character cannot cast and a
// casting character cannot sit, so the deep rest stands first and the sit
// transition waits the flight out (docs/hunting.md).

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// healFlightMargin widens the tracked in-flight window past the cast animation,
// so the sit request never races the server refusal.
const healFlightMargin = 1 * time.Second

// selfHealSkill returns the learned instant self heal that is ready: the mana
// covers the cost and the reuse window closed. The server skill list is the
// only source, so a fighter granted a heal uses it like a mystic.
func (l *Loop) selfHealSkill(now time.Time) (int32, bool) {
    for _, skill := range l.tracker.ActiveSkills() {
        if !npcdata.SelfHealSkill(skill.SkillID) {
            continue
        }
        cast, ok := npcdata.SkillCastOf(skill.SkillID)
        if !ok || cast.Operate != "A1" || cast.Target != "SELF" {
            continue
        }
        if float64(cast.MPCostOf(skill.Level)+
            npcdata.SelfHealInitialConsumeOf(skill.SkillID, skill.Level)) >
            l.tracker.SelfCurMP() {
            continue
        }
        if l.skillOnReuse(skill.SkillID, now) {
            continue
        }

        return skill.SkillID, true
    }

    return 0, false
}

// healInFlight reports whether the heal cast is still running; the sit
// transition waits it out.
func (l *Loop) healInFlight(now time.Time) bool {
    return now.Before(l.healFlightUntil)
}

// restSelfHeal runs the standing self heal branch and reports whether the cast
// went out: under the sit threshold, no blow landing, mana paid and reuse
// clear. The mana rest of the mystic stays with the sitting regeneration.
func (l *Loop) restSelfHeal(now time.Time, hp float64) bool {
    if l.tracker.SelfUnderAttack() || l.mageManaDry() {
        return false
    }
    if hp >= sitDownHealthPercent {
        return false
    }
    healID, ready := l.selfHealSkill(now)
    if !ready {
        return false
    }
    if l.tracker.SelfSitting() {
        // The server refuses the casts of a sitting character:
        // the stand to heal branch owns this case.
        return false
    }
    l.castSelfHeal(healID, now)

    return true
}

// restStandToHeal runs the sitting branch: a character below the sit threshold
// whose heal is ready stands to cast on the next window. The mystic with low
// mana keeps the sit.
func (l *Loop) restStandToHeal(now time.Time, hp float64) bool {
    if !l.tracker.SelfSitting() || l.tracker.SelfUnderAttack() {
        return false
    }
    if hp >= sitDownHealthPercent || l.mageManaLow() {
        return false
    }
    _, ready := l.selfHealSkill(now)

    return ready
}

// castSelfHeal fires the ready heal and arms its reuse and in-flight windows.
func (l *Loop) castSelfHeal(skillID int32, now time.Time) {
    cast, ok := npcdata.SkillCastOf(skillID)
    if !ok {
        return
    }
    if err := l.game.UseMagicSkill(skillID); err != nil {
        l.logger.Printf("Hunt: heal cast of %d failed: %v", skillID, err)

        return
    }
    l.skillReuse[skillID] = now.Add(
        time.Duration(cast.ReuseDelay)*time.Millisecond +
            castReuseMargin)
    l.healFlightUntil = now.Add(
        time.Duration(cast.HitTime)*time.Millisecond + healFlightMargin)
    if info, ok := npcdata.SkillInfoOf(skillID); ok {
        if level, levelOK := l.learnedLevelOf(skillID); levelOK {
            l.logf("Hunt: casting %s level %d instead of resting",
                info.Name, level)
        }
    }
}

func (l *Loop) learnedLevelOf(skillID int32) (int32, bool) {
    for _, skill := range l.tracker.ActiveSkills() {
        if skill.SkillID == skillID {
            return skill.Level, true
        }
    }

    return 0, false
}
