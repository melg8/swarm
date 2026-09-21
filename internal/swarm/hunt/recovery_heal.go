// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The recovery self heal of the rest logic: a character that knows
// an instant self heal (npcdata.SelfHealSkill - the C1 SELF target
// Heal skills) casts it instead of sitting down while the mana pays
// the cost and no blow is landing. The cast recovers a chunk of the
// bar in one hit time where the sitting regeneration would grind
// for half a minute, and a standing character keeps the flee option
// the sit gives up.
//
// The verified server facts the flow builds on (Mobius C1): a cast
// request of a sitting character is refused with
// YOU_CANNOT_MOVE_WHILE_SITTING (Player.useMagic L7537), so a
// sitting character stands up first and casts on the next window;
// a sit request of a casting character is refused with "Cannot sit
// while casting" (Player.sitDown L2383) without breaking the cast,
// so the sit transition waits the heal flight out instead of
// bouncing off the server. The spawn protection settle needs no
// heal branch of its own: a cast is one of the five packets that
// burn the protection (see settle.go), so while aggressive mobs
// stand in reach the settle keeps sitting, and once the ground is
// clear the settle ends and this flow owns the recovery.

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// healFlightMargin widens the locally tracked in-flight window of
// the recovery heal past its cast animation: the heal lands at the
// hit time, the sit request of the same window would be refused
// while the cast runs, and the margin keeps the wait honest under
// the network jitter.
const healFlightMargin = 1 * time.Second

// selfHealSkill returns the learned instant self heal that is ready
// to cast: the mana covers the cost of the known level and the
// local reuse window has closed. The skill list the server sent is
// the only source - a fighter granted a heal uses it exactly like a
// mystic does. The second answer is false when no heal is ready.
func (l *Loop) selfHealSkill(now time.Time) (int32, bool) {
    for _, skill := range l.tracker.ActiveSkills() {
        if !npcdata.SelfHealSkill(skill.SkillID) {
            continue
        }
        cast, ok := npcdata.SkillCastOf(skill.SkillID)
        if !ok || cast.Operate != "A1" || cast.Target != "SELF" {
            continue
        }
        if float64(cast.MPCostOf(skill.Level)) >
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

// healInFlight reports whether the recovery heal cast is still
// running: the sit transition waits it out (the server refuses the
// sit of a casting character).
func (l *Loop) healInFlight(now time.Time) bool {
    return now.Before(l.healFlightUntil)
}

// restSelfHeal runs the standing self heal branch of the rest
// logic. It reports whether the heal cast went out - the caller
// yields the tick to it. The gates: the health sits under the sit
// threshold (the branch replaces exactly the sit decision), no blow
// is landing (a heal under the swings is a wasted cast, the flee
// flow owns the fight), the mana pays and the reuse window is
// clear. The mana rest of the mystic is out of scope: a dry caster
// needs the sitting mana regeneration, a heal only spends it.
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

// restStandToHeal runs the sitting self heal branch of the rest
// logic: a sitting character deep in the rest (below the sit
// threshold) whose heal is ready stands up to cast on the next
// window (the server refuses the casts of a sitting character). It
// reports whether the stand transition should go out. A bar past
// the sit threshold finishes on the sitting regeneration without
// the toggle - the stand only pays where the cast replaces the
// long grind. The mana rest of the mystic owns its sit - the stand
// only happens when the mana is above the caster stand threshold.
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

// castSelfHeal fires the ready self heal and arms its local reuse
// window and the in-flight window the sit transition waits on.
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

// learnedLevelOf returns the learned level of a skill id from the
// active skill list.
func (l *Loop) learnedLevelOf(skillID int32) (int32, bool) {
    for _, skill := range l.tracker.ActiveSkills() {
        if skill.SkillID == skillID {
            return skill.Level, true
        }
    }

    return 0, false
}
