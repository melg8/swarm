// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The combat casting of the hunt loop. A warrior treats the skills
// as the optional extra damage on top of the auto attack: the strike
// of the weapon in hand fires whenever its reuse window allows and
// the mana pays (the learned skill whose <using> condition accepts
// the equipped weapon family - Power Strike with a sword, Mortal Blow
// with a dagger, Power Shot with a bow). A mystic attacks with magic
// first: the learned magic attack spells fire per their reuse
// windows, the auto attack of the staff only adds between the casts.
// The self buffs (the auras, the timed A2 skills on the caster) cast
// between the fights whenever the effect is missing.
//
// The out of mana mystic stops attacking and sits down: the mana
// regeneration of a sitting caster is several times the standing
// one, and a dry mage swinging the staff prolongs every fight into
// a mana death spiral. The character stands up once the mana
// recovered and the next fight opens with a full bar.

import (
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// Timing and threshold constants of the combat casting.
const (
	// castRetryPeriod paces the cast requests: the server answers a
	// refused cast (the range, the mana, the reuse) with a bare
	// ActionFailed, so the loop re-tries its picks at a human rate.
	castRetryPeriod = 2 * time.Second
	// buffCastPeriod paces the self buff casts: a missing buff is
	// retried until its AbnormalStatusUpdate arrival flips the
	// tracker list.
	buffCastPeriod = 6 * time.Second
	// castReuseMargin widens the locally tracked reuse windows of
	// the casts: the server reuse clock starts at the cast start,
	// the local one at the request, and the network jitter between
	// them must not produce a refused cast on every tick.
	castReuseMargin = 2 * time.Second
	// manaSitPercent is the mana fill below which the out of mana
	// mystic drops the fight and sits down to regenerate.
	manaSitPercent = 15.0
	// manaStandPercent is the mana fill the resting mystic waits for
	// before it stands up and re-engages.
	manaStandPercent = 60.0
)

// maybeCastCombatSkill fires the combat skill of the running fight:
// the warrior strike that accepts the weapon in hand, the mystic
// attack spell. The warrior casting is the optional extra damage -
// the auto attack keeps swinging whatever the cast does; the mystic
// casting is the primary damage. One cast request per retry period,
// the locally tracked reuse windows gate the picks.
func (l *Loop) maybeCastCombatSkill(now time.Time) {
	if !l.castAt.IsZero() && now.Sub(l.castAt) < castRetryPeriod {
		return
	}
	skills := l.tracker.ActiveSkills()
	weapon, hasWeapon := l.tracker.SelfWeaponKind()
	caster := l.isCaster()
	for _, skill := range skills {
		cast, ok := npcdata.SkillCastOf(skill.SkillID)
		if !ok || cast.Operate != "A1" || cast.Target != "ONE" {
			continue
		}
		if caster != cast.Magic {
			// The warrior swings physical strikes, the mystic fires
			// spells - never the other way around.
			continue
		}
		if !cast.Magic && hasWeapon &&
			!cast.UsableWithWeapon(weapon) {
			continue
		}
		if float64(cast.MPCostOf(skill.Level)) >
			l.tracker.SelfCurMP() {
			continue
		}
		if l.skillOnReuse(skill.SkillID, now) {
			continue
		}
		if err := l.game.UseMagicSkill(skill.SkillID); err != nil {
			l.logger.Printf("Hunt: cast of %d failed: %v",
				skill.SkillID, err)

			return
		}
		l.castAt = now
		l.skillReuse[skill.SkillID] = now.Add(
			time.Duration(cast.ReuseDelay)*time.Millisecond +
				castReuseMargin)
		if info, ok := npcdata.SkillInfoOf(skill.SkillID); ok {
			l.logger.Printf("Hunt: casting %s level %d at the target",
				info.Name, skill.Level)
		}

		return
	}
}

// maybeSelfBuff casts the missing self buffs between the fights: the
// learned timed buff skills (the auras) whose effect does not run
// yet. It reports whether a cast went out - the caller yields the
// tick to it.
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

// isCaster reports whether the character class attacks with magic
// (the class tree answer - the mystic families know their attack
// spells from level 1, the fighter families never learn one).
func (l *Loop) isCaster() bool {
	return npcdata.ClassCastsMagic(l.tracker.SelfClassID())
}

// maybePickGearProfile swaps the gear scoring profile once the class
// of the character is known: the mystic families rank the caster
// gear (the staves by their magical damage, the robes by the cast
// speed) instead of the melee weapons of the default fighter
// profile. The pick runs once per session - the class of a character
// never changes mid run (the first class transfer of a leveled bot
// relogs anyway) and the swap resets the cached equip scans so the
// very first equip decision already scores through the caster lens.
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
		// The scans cached against the fighter profile must
		// re-run: the upgrade scan version gates them on the
		// inventory mutations, the profile swap is invisible
		// to it.
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

// mageManaLow reports whether a casting character should hold the
// fights: the mana fill sits below the re-engage threshold but above
// the dry one - the standing regeneration tops it up while the loop
// waits. Non casters never wait.
func (l *Loop) mageManaLow() bool {
	if !l.isCaster() {
		return false
	}

	return l.tracker.SelfManaPercent() < manaStandPercent
}

// dropFightForMana drops the running fight of the out of mana
// mystic: the attack requests stop, the sit down of the rest logic
// breaks the auto attack server side (the breakAttack of the sit
// path) and the mana regenerates at the sitting rate. The safety
// nets of the loop stay armed - a hurt dry mage under attack still
// flees, a critical one still logs out.
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

// publishSkillWeaponPriority feeds the weapon families of the
// learning queue priority: the weapon in hand plus the weapon of
// the next planned purchase (the queue puts the lessons their
// <using> condition accepts first). The tracker no-ops on an
// unchanged list.
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

// nextWeaponKind resolves the weapon family of the next weapon
// purchase of the cached shopping plan: the first affordable weapon
// of the queue. The second answer is false when no weapon is
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
