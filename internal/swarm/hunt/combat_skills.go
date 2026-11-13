// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The combat casting of the hunt loop. A warrior treats the skills as the
// optional extra damage: the strike whose <using> condition accepts the weapon
// in hand (Power Strike with a sword, Power Shot with a bow) fires when its
// reuse window allows and the mana pays. A mystic attacks with magic first and
// the staff only adds between the casts; the self buffs (the timed A2 skills)
// cast between the fights whenever their effect is missing. The out of mana
// mystic drops the fight and sits down - the sitting regeneration is several
// times the standing one - and stands up once the mana recovered.

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Timing and threshold constants of the combat casting.
const (
    // castRetryPeriod paces the cast requests: the server refuses a cast
    // (range, mana, reuse) with a bare ActionFailed, so the loop retries at a
    // human rate.
    castRetryPeriod = 2 * time.Second
    // buffCastPeriod paces the self buff casts: a missing buff is retried until
    // its AbnormalStatusUpdate arrival flips the tracker list.
    buffCastPeriod = 6 * time.Second
    // castReuseMargin widens the local reuse windows: the server clock starts
    // at the cast start, the local one at the request, so the jitter between
    // them must not refuse a cast on every tick.
    castReuseMargin = 2 * time.Second
    // manaSitPercent is the mana fill below which the out of mana
    // mystic drops the fight and sits down to regenerate.
    manaSitPercent = 15.0
    // manaStandPercent is the mana fill the resting mystic waits for
    // before it stands up and re-engages.
    manaStandPercent = 60.0
    // overhitFinishPercent is the target health below which the overhit-capable
    // strike fires as the finishing blow. The server (Mobius C1,
    // Attackable.calculateOverhitExp L1480) pays exp * min(overkill/maxHp,
    // 0.25) on a killing blow of an overhit-flagged skill, and any non lethal
    // hit clears the armed flag (AttackableStatus.reduceHp L40), so an early
    // cast only burns the reuse window. One melee swing takes 25-45 percent of
    // the mob bar in the low level band, so a bar inside the window usually
    // holds about a strike's worth of damage.
    overhitFinishPercent = 40.0
)

// maybeCastCombatSkill fires the combat skill of the running fight: the warrior
// strike that accepts the weapon in hand or the mystic attack spell. The
// warrior cast is optional extra damage (the auto attack keeps swinging), the
// mystic cast is the primary damage. One request per retry period, gated by the
// local reuse windows.
func (l *Loop) maybeCastCombatSkill(now time.Time) {
    if !l.castAt.IsZero() && now.Sub(l.castAt) < castRetryPeriod {
        return
    }
    weapon, hasWeapon := l.tracker.SelfWeaponKind()
    caster := l.isCaster()
    for _, skill := range l.tracker.ActiveSkills() {
        if !l.combatSkillCandidate(
            skill, weapon, hasWeapon, caster, now) {
            continue
        }
        cast, _ := npcdata.SkillCastOf(skill.SkillID)
        if err := l.game.UseMagicSkill(skill.SkillID); err != nil {
            l.logger.Printf("Hunt: cast of %d failed: %v",
                skill.SkillID, err)

            return
        }
        l.castAt = now
        l.skillReuse[skill.SkillID] = now.Add(
            time.Duration(cast.ReuseDelay)*time.Millisecond +
                castReuseMargin)
        aim := "at the target"
        if npcdata.OverhitSkill(skill.SkillID) {
            aim = "as the finishing blow"
        }
        if info, ok := npcdata.SkillInfoOf(skill.SkillID); ok {
            l.logger.Printf("Hunt: casting %s level %d %s",
                info.Name, skill.Level, aim)
        }

        return
    }
}

// combatSkillCandidate reports whether one learned skill is a combat cast
// candidate: an active ONE target skill of the class role that accepts the
// weapon in hand, paid by the mana, outside the local reuse window and, for an
// overhit flagged strike, inside the finishing window of the target bar.
func (l *Loop) combatSkillCandidate(
    skill state.LearnedSkill,
    weapon string, hasWeapon, caster bool, now time.Time,
) bool {
    cast, ok := npcdata.SkillCastOf(skill.SkillID)
    if !ok || cast.Operate != "A1" || cast.Target != "ONE" {
        return false
    }
    if caster != cast.Magic {
        // The warrior swings physical strikes, the mystic fires
        // spells - never the other way around.
        return false
    }
    if !cast.Magic && hasWeapon && !cast.UsableWithWeapon(weapon) {
        return false
    }
    if float64(cast.MPCostOf(skill.Level)) >
        l.tracker.SelfCurMP() {
        return false
    }
    if l.skillOnReuse(skill.SkillID, now) {
        return false
    }
    if npcdata.OverhitSkill(skill.SkillID) &&
        l.targetAboveOverhitWindow() {
        return false
    }

    return true
}

// targetAboveOverhitWindow reports whether the target still stands above the
// finishing window. An unknown health means the fight just opened (the server
// refreshes the vitals of the attacked mob), so the strike holds.
func (l *Loop) targetAboveOverhitWindow() bool {
    hp := l.tracker.ObjectHealthPercent(l.target)
    if hp < 0 {
        return true
    }

    return hp > overhitFinishPercent
}

// maybeSelfBuff casts the missing self buffs between the fights: the learned
// timed buff skills whose effect does not run yet. It reports whether a cast
// went out, so the caller yields the tick.
func (l *Loop) maybeSelfBuff(now time.Time) bool {
    if l.tracker.SelfSitting() {
        // The rest owns a sitting character; a cast would stand it
        // up against the regeneration.
        return false
    }
    if !l.buffAt.IsZero() && now.Sub(l.buffAt) < buffCastPeriod {
        return false
    }
    skills := l.tracker.ActiveSkills()
    for _, skill := range skills {
        cast, ok := npcdata.SkillCastOf(skill.SkillID)
        if !ok || cast.Operate != "A2" || cast.Target != "SELF" ||
            cast.BuffTime <= 0 {
            continue
        }
        if l.tracker.SelfHasBuff(skill.SkillID) {
            continue
        }
        // The collision guard: a fresh guide support magic buff in
        // the same abnormal slot outlasts the own aura by far, the
        // cast would replace the stronger effect (see guide_buffs.go).
        if l.selfBuffSuperseded(skill.SkillID) {
            continue
        }
        if float64(cast.MPCostOf(skill.Level)) >
            l.tracker.SelfCurMP() {
            continue
        }
        if err := l.game.UseMagicSkill(skill.SkillID); err != nil {
            l.logger.Printf("Hunt: buff cast of %d failed: %v",
                skill.SkillID, err)

            return false
        }
        l.buffAt = now
        if info, ok := npcdata.SkillInfoOf(skill.SkillID); ok {
            l.logger.Printf("Hunt: casting the self buff %s level %d",
                info.Name, skill.Level)
        }

        return true
    }

    return false
}

// skillOnReuse reports whether the local reuse window of a skill
// still blocks its cast.
func (l *Loop) skillOnReuse(skillID int32, now time.Time) bool {
    until, ok := l.skillReuse[skillID]

    return ok && now.Before(until)
}

// isCaster reports whether the class attacks with magic: the mystic families
// know their attack spells from level one, the fighter families never learn
// one.
func (l *Loop) isCaster() bool {
    return npcdata.ClassCastsMagic(l.tracker.SelfClassID())
}

// maybePickGearProfile swaps the gear scoring profile once the class is known:
// the mystic families rank the caster gear (the staves by magical damage, the
// robes by cast speed) instead of the melee weapons of the fighter profile. The
// pick runs once per session (a class transfer relogs anyway) and resets the
// cached equip scans, so the first equip decision already scores through the
// caster lens.
func (l *Loop) maybePickGearProfile() {
    if l.profilePicked {
        return
    }
    classID := l.tracker.SelfClassID()
    if classID == 0 {
        // The class is not known yet: the character packet of
        // the login sequence answers it.
        return
    }
    l.profilePicked = true
    if !npcdata.ClassCastsMagic(classID) {
        return
    }
    l.SetGearProfile(gear.MysticFighter{})
    if l.equip != nil {
        // The scans cached against the fighter profile must re-run: the upgrade
        // scan version gates them on the inventory mutations, the profile swap
        // is invisible to it.
        l.equip.equipScanVersion = 0
        l.equip.equipScanNone = false
    }
    l.logger.Printf("Hunt: mystic class %d, scoring the caster gear",
        classID)
}

// mageManaDry reports whether a casting character ran out of mana:
// the fill dropped under the sit threshold. Non casters never dry.
func (l *Loop) mageManaDry() bool {
    if !l.isCaster() {
        return false
    }

    return l.tracker.SelfManaPercent() < manaSitPercent
}

// mageManaLow reports whether a caster should hold the fights: the mana sits
// below the re-engage threshold but above the dry one, and the standing
// regeneration tops it up while the loop waits. Non casters never wait.
func (l *Loop) mageManaLow() bool {
    if !l.isCaster() {
        return false
    }

    return l.tracker.SelfManaPercent() < manaStandPercent
}

// dropFightForMana drops the running fight of the out of mana mystic: the
// attack requests stop, the sit down breaks the auto attack server side (the
// breakAttack of the sit path) and the mana regenerates at the sitting rate.
// The safety nets stay armed: a hurt dry mage still flees, a critical one still
// logs out.
func (l *Loop) dropFightForMana() {
    if l.target == 0 {
        return
    }
    l.logger.Printf("Hunt: mana at %.0f%%, dropping the fight and "+
        "resting", l.tracker.SelfManaPercent())
    l.target = 0
    l.clearBlindRecovery()
    l.engageAt = time.Time{}
}

// publishSkillWeaponPriority feeds the weapon families of the learning queue:
// the weapon in hand plus the next planned purchase, so the lessons their
// <using> condition accepts come first. The tracker no-ops on an unchanged
// list.
func (l *Loop) publishSkillWeaponPriority() {
    kinds := make([]string, 0, 2)
    if weapon, ok := l.tracker.SelfWeaponKind(); ok && weapon != "" {
        kinds = append(kinds, weapon)
    }
    if next, ok := l.nextWeaponKind(); ok && next != "" {
        kinds = append(kinds, next)
    }
    l.tracker.SetSkillWeaponPriority(kinds)
}

// nextWeaponKind resolves the weapon family of the next planned purchase: the
// first affordable weapon of the cached shopping queue, with false when none is
// planned.
func (l *Loop) nextWeaponKind() (string, bool) {
    for _, purchase := range l.shoppingPlanCache {
        if !purchase.Affordable {
            break
        }
        stats, ok := npcdata.ItemGearStats(purchase.ItemID)
        if ok && stats.WeaponType != "" {
            return stats.WeaponType, true
        }
    }

    return "", false
}
