// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "math"
    "sort"
    "time"
)

// SelfObjectID returns the object id of the played character.
func (b *Bot) SelfObjectID() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.selfID
}

// SkillsListed reports whether the server skill list arrived: the learning
// queue builds on it and the first town trip of a session waits for it, since
// the enter world burst (UserInfo, ItemList, SkillList) races the first hunt
// ticks.
func (b *Bot) SkillsListed() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skills != nil
}

// SelfTargetID returns the object id of the current target of the
// character, zero when nothing is targeted.
func (b *Bot) SelfTargetID() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.TargetID
}

// SelfEngaged reports whether the character fights the given target inside the
// combat window: the signal that a forced attack request started the fight, so
// the hunt loop stops re-requesting it.
func (b *Bot) SelfEngaged(targetID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.FightingTargetID == targetID && b.char.inCombat(b.now())
}

// SelfFighting reports whether the character swings at or chases the given
// target right now: the engagement must be fresh. The auto attack flag and the
// combat window linger after an interrupted fight, so a stale engagement
// re-requests the forced attack instead of trusting the flags.
func (b *Bot) SelfFighting(targetID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.FightingTargetID == targetID &&
        b.char.fightingFresh(b.now())
}

// SelfEngagedBy reports whether the target is still a live engagement,
// counting both sides: the own fresh swing or chase step, or the target
// holding the character. The one-sided SelfFighting view reads a point-blank
// bow fight as a stuck selection while the C1 bow reuse window refuses every
// attack (Creature.canUseRangeWeapon answers ActionFailed and reschedules).
// The object side needs no freshness window: a switched target replaces
// TargetID and a dead one fails the attackable test.
func (b *Bot) SelfEngagedBy(targetID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.char.FightingTargetID == targetID &&
        b.char.fightingFresh(b.now()) {
        return true
    }
    obj, _ := b.objectLocked(targetID)
    if obj == nil || obj.Dead || obj.Kind != kindNPC || !obj.Attackable {
        return false
    }

    return b.selfID != 0 && obj.TargetID == b.selfID
}

// SelfCannotSeeTargetAt returns when the server last answered "Cannot see
// target." (zero when never): an obstacle blocks the geodata line of sight, so
// the hunt walks around it instead of re-requesting the refused attack.
func (b *Bot) SelfCannotSeeTargetAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.CannotSeeTargetAt
}

// SelfCombatActiveAt returns when the last fight activity landed (a swing
// or a chase step, zero when none ever), regardless of freshness. The hunt
// orders it against SelfCannotSeeTargetAt: a refusal newer than the activity
// marks the phantom chase, because the server keeps broadcasting the chase
// steps of an attack it refuses behind an obstacle, so fresh activity alone is
// no running fight (a dump showed the character standing 80 units off its
// target for over a minute with only "Cannot see target" answers).
func (b *Bot) SelfCombatActiveAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.CombatActiveAt
}

// SelfLastShotAt returns when the last Attack broadcast arrived (zero when
// never): the moment the Mobius server finalizes a bow shot (see
// CharacterState.LastSelfShotAt). The hunt reads it as the release trigger of
// the shot-paced kite retreat.
func (b *Bot) SelfLastShotAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.LastSelfShotAt
}

// SelfPAtkSpd returns the live attack speed (the StatusUpdate ATK_SPD
// attribute) for the bow cooldown math (see CharacterState.PAtkSpd). Zero when
// the server never broadcast it.
func (b *Bot) SelfPAtkSpd() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.PAtkSpd
}

// SelfBowCycle returns when the last RED SetupGauge arrived and the bow disable
// window it announced (zero values before the first): the kite retreat reads
// the server's own windup and reuse timing instead of the pAtkSpd formula (see
// CharacterState.BowCycleAt).
func (b *Bot) SelfBowCycle() (time.Time, time.Duration) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.BowCycleAt, b.char.BowCycle
}

// SelfWalking reports whether the character moves right now: the fresh window
// guards against a lost stop packet, where the flag claims motion without an
// update for seconds.
func (b *Bot) SelfWalking() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Moving && b.now().Sub(b.char.MoveAt) <= walkingFreshWindow
}

// SelfPosition returns the last observed placement of the played character.
func (b *Bot) SelfPosition() (int32, int32, int32, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0, 0, 0, false
    }

    return b.char.X, b.char.Y, b.char.Z, true
}

// SelfProjectedPosition estimates where the server holds the character: a
// standing one keeps its broadcast place, a moving one advances along the
// segment at the live run speed. The estimate is 3D like the server check
// (the object scans use the 2D projectedPosition); the proxy compares the
// client reports against it, and the server snaps a report a move speed away.
func (b *Bot) SelfProjectedPosition() (float64, float64, float64, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0, 0, 0, false
    }
    x, y, z := b.char.projectedPosition(b.now())

    return x, y, z, true
}

// projectedPosition estimates the place from the movement broadcast: the
// Mobius Creature.updatePosition loop steps a mover toward the destination at
// the effective speed every game tick, so the segment interpolates by the
// traveled 3D distance. The clamps end a lost arrival at the destination.
func (c CharacterState) projectedPosition(now time.Time) (
    float64, float64, float64,
) {
    x := float64(c.X)
    y := float64(c.Y)
    z := float64(c.Z)
    if !c.Moving || c.MoveAt.IsZero() || c.RunSpeed <= 0 {
        return x, y, z
    }
    dx := float64(c.DestX) - x
    dy := float64(c.DestY) - y
    dz := float64(c.DestZ) - z
    dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
    elapsed := now.Sub(c.MoveAt).Seconds()
    if dist < 1 || elapsed <= 0 {
        return x, y, z
    }
    frac := math.Min(c.RunSpeed*elapsed, dist) / dist

    return x + dx*frac, y + dy*frac, z + dz*frac
}

// SelfHeading returns the last observed heading: the client position validation
// reports it alongside the placement and the server stores it as the client
// heading.
func (b *Bot) SelfHeading() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0
    }

    return b.char.Heading
}

// ApplySelfFacing records the facing the client itself drives: the cursor
// key escape claims the placement and the facing of the official movement
// simulation (the arrow walk renders it locally), while the server echo of
// the claim carries the server side move heading (ValidateLocation reads the
// location heading). The claim owns the shown facing until a mouse click.
func (b *Bot) ApplySelfFacing(heading int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if b.selfID == 0 {
        return
    }
    b.char.Heading = heading
    b.touch()
}

// SelfSitting reports whether the character is sitting. The hunt loop gates the
// rest transitions on it because the sit/stand action is a server side toggle.
func (b *Bot) SelfSitting() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Sitting
}

// SelfLevel returns the observed level of the played character, zero
// before the first UserInfo or level StatusUpdate.
func (b *Bot) SelfLevel() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Level
}

// SelfClassID returns the class id of the played character: the skill teachers
// and the learning queue resolve through it. Zero before the first UserInfo.
func (b *Bot) SelfClassID() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.ClassID
}

// SelfSp returns the current skill points of the played character: the learning
// trigger and the per lesson budget of the teacher stop read it.
func (b *Bot) SelfSp() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Sp
}

// SkillsRevision returns the count of the SetSkills calls and weapon priority
// changes: the teacher stop waits on its bump, because the server answers every
// learned lesson with a fresh SkillList.
func (b *Bot) SkillsRevision() uint64 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skillsRevision
}

// SkillPlan returns a copy of the learning queue view (nil when the class is
// unknown or nothing is left); the hunt plans its teacher trips on it.
func (b *Bot) SkillPlan() *SkillPlanView {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skillPlanViewLocked()
}

// SelfExp returns the last observed experience. The server refreshes it in the
// UserInfo packet after every experience change (the PlayerStat add and remove
// paths both call updateUserInfo), so a death penalty shows at once while a
// death that removed nothing leaves the value untouched.
func (b *Bot) SelfExp() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Exp
}

// SelfUnderAttack reports whether the character was hit within the last
// seconds, so the rest logic never sits down in the middle of a fight the bot
// did not start.
func (b *Bot) SelfUnderAttack() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    last := b.char.LastHitAt

    return !last.IsZero() && b.now().Sub(last) < underAttackWindow
}

// SelfLandedHit returns the target of the last blow that actually landed with
// its time. The miss flagged swings never update the pair, so a zero target
// means no landed blow yet (or since the last tracker reset).
func (b *Bot) SelfLandedHit() (int32, time.Time) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.LastLandedHitTarget, b.char.LastLandedHitAt
}

// SelfAttackerCount returns how many living attackable npcs hold the character
// as their target: the aggro load. The emergency logout fires on a social pile
// up, where two swinging mobs grind a lone farmer down faster than any escape
// could answer.
func (b *Bot) SelfAttackerCount() int {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0
    }
    count := 0
    for i := range b.world.hot {
        obj := &b.world.hot[i]
        if obj.Kind == kindNPC && obj.Attackable && !obj.Dead &&
            obj.TargetID == b.selfID {
            count++
        }
    }

    return count
}

// SelfDead reports whether the character died: a known maximum with a
// zero current HP only happens on death (the server broadcasts
// StatusUpdate CUR_HP 0 there).
func (b *Bot) SelfDead() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.MaxHP > 0 && b.char.CurHP <= 0
}

// SelfHealthPercent returns the current HP as a percentage of the maximum
// (0..100). An unknown maximum (no UserInfo yet) counts as healthy: resting
// forever on missing vitals is worse than engaging.
func (b *Bot) SelfHealthPercent() float64 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.char.MaxHP <= 0 {
        return 100
    }
    pct := b.char.CurHP / b.char.MaxHP * 100

    return math.Min(100, math.Max(0, pct))
}

// ActiveSkills returns the learned non passive skills in ascending id order,
// where the combat casting picks its strikes, spells and buffs. The answer is a
// fresh slice the caller owns.
func (b *Bot) ActiveSkills() []LearnedSkill {
    b.mu.RLock()
    defer b.mu.RUnlock()
    skills := make([]LearnedSkill, 0, len(b.skills))
    for id, skill := range b.skills {
        if skill.passive {
            continue
        }
        skills = append(skills, LearnedSkill{
            SkillID: id,
            Level:   skill.level,
            Passive: false,
        })
    }
    sort.Slice(skills, func(i, j int) bool {
        return skills[i].SkillID < skills[j].SkillID
    })

    return skills
}
