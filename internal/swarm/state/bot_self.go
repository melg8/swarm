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

// SkillsListed reports whether the server skill list of the character
// arrived: the learning queue builds on it, and the first town trip
// of a session waits for it - the packet burst of the enter world
// (UserInfo, ItemList, SkillList) races the first hunt ticks, and a
// trip started between the ItemList and the SkillList would silently
// plan without the learning stops.
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

// SelfEngaged reports whether the character is currently fighting the
// given target: chasing it or attacking it inside the combat window. It
// is the signal that a forced attack request actually started the fight,
// so the hunt loop can stop re-requesting it.
func (b *Bot) SelfEngaged(targetID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.FightingTargetID == targetID && b.char.inCombat(b.now())
}

// SelfFighting reports whether the character is actually swinging at or
// chasing the given target right now: the engagement must be fresh (an
// attack or a chase step within the last seconds). The auto attack flag
// and the combat window linger after an interrupted fight, so a manual
// attack command relies on this stricter view: a stale engagement
// re-requests the forced attack instead of trusting the stale flags.
func (b *Bot) SelfFighting(targetID int32) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.FightingTargetID == targetID &&
        b.char.fightingFresh(b.now())
}

// SelfEngagedBy reports whether the given target is still a live
// engagement for the character, counting BOTH sides: the own fresh
// swing or chase step, or the target holding the character as its own
// target. The one-sided SelfFighting view reads a point-blank bow
// fight as a stuck selection while the C1 bow reuse window refuses
// every attack request (Creature.canUseRangeWeapon answers
// ActionFailed and reschedules the retry) and the mob keeps swinging -
// the engage stuck gate then dropped the target that was killing the
// character. The object side needs no freshness window: a target that
// switched away replaces TargetID with the new victim, and a dead
// target fails the attackable test.
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

// SelfCannotSeeTargetAt returns when the server last answered the attack
// with "Cannot see target." (zero when it never happened): the geodata
// line of sight to the attack target is blocked, an obstacle stands
// between the character and the target. The hunt loop reads it to
// recognize the obstructed engage and walk around the obstacle instead
// of re-requesting the refused attack forever.
func (b *Bot) SelfCannotSeeTargetAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.CannotSeeTargetAt
}

// SelfCombatActiveAt returns when the last fight activity of the played
// character landed (a swing attempt or a chase step, zero when none ever
// did), regardless of freshness. The hunt loop orders it against
// SelfCannotSeeTargetAt: a refusal that is newer than the activity
// marks the phantom chase - the server keeps broadcasting the chase
// steps of an attack it refuses to execute behind an obstacle, so a
// fresh activity view alone must not read as a running fight (the
// 2026-09-12 03:25 dump livelock: the character stood 80 units from
// the target for over a minute, "Cannot see target" every few seconds,
// no swing ever landing).
func (b *Bot) SelfCombatActiveAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.CombatActiveAt
}

// SelfLastShotAt returns when the last Attack broadcast of the played
// character arrived (zero when none ever did): the shot commit moment
// the Mobius server finalizes the bow shot at (see the
// CharacterState.LastSelfShotAt comment). The hunt loop reads it as
// the release trigger of the shot-paced kite retreat (issue #60).
func (b *Bot) SelfLastShotAt() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.LastSelfShotAt
}

// SelfPAtkSpd returns the live physical attack speed of the played
// character (the StatusUpdate ATK_SPD attribute): the bow cooldown
// window math reads it (see CharacterState.PAtkSpd - the C1 disable
// window formulas). Zero when the server never broadcast it yet.
func (b *Bot) SelfPAtkSpd() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.PAtkSpd
}

// SelfBowCycle returns when the last RED SetupGauge of the played
// character arrived and the bow disable window it announced (zero
// values when no bow cycle was seen yet): the kite retreat reads it
// as the server's own windup and reuse timing instead of the
// pAtkSpd formula (see CharacterState.BowCycleAt).
func (b *Bot) SelfBowCycle() (time.Time, time.Duration) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.BowCycleAt, b.char.BowCycle
}

// SelfWalking reports whether the character is moving right now: the
// moving flag is set by the movement broadcasts and the fresh window
// guards against a lost stop packet (no update for seconds means the
// walk stalled even though the flag claims motion).
func (b *Bot) SelfWalking() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Moving && b.now().Sub(b.char.MoveAt) <= walkingFreshWindow
}

// SelfPosition returns the last observed placement of the played
// character.
func (b *Bot) SelfPosition() (int32, int32, int32, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0, 0, 0, false
    }

    return b.char.X, b.char.Y, b.char.Z, true
}

// SelfProjectedPosition estimates where the server holds the played
// character right now: a standing character keeps its broadcast place,
// a moving one advances along its segment at the live run speed. The
// proxy compares the attached client reports against it, because the
// server snaps a character whose report is more than one move speed
// away. The estimate is 3D like the server check; the object scans use
// the 2D projectedPosition.
func (b *Bot) SelfProjectedPosition() (float64, float64, float64, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0, 0, 0, false
    }
    x, y, z := b.char.projectedPosition(b.now())

    return x, y, z, true
}

// projectedPosition estimates the current place of the character from
// its movement broadcast: the Mobius Creature.updatePosition loop
// steps a mover toward the destination at the effective speed every
// game tick, so the straight segment interpolates by the traveled 3D
// distance. The clamped fraction makes a lost arrival broadcast end at
// the destination instead of running past it.
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

// SelfHeading returns the last observed heading of the played
// character (the client position validation reports it alongside the
// placement, the server stores it as the client heading).
func (b *Bot) SelfHeading() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.selfID == 0 {
        return 0
    }

    return b.char.Heading
}

// ApplySelfFacing records the facing the client itself drives: the
// cursor key escape claims the placement AND the facing of the
// official client's own movement simulation (the arrow walk renders
// its facing locally), while the server echo of the claimed placement
// carries the server side move heading (ValidateLocation reads the
// location's heading - the arm direction of the running move - the
// claimed facing lands in the client heading field the server does
// not broadcast back). The claim facing is the truth the web UI
// shows while the keyboard movement owns the stream; the normal
// placement broadcasts own it again once the mouse clicks resume.
func (b *Bot) ApplySelfFacing(heading int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    if b.selfID == 0 {
        return
    }
    b.char.Heading = heading
    b.touch()
}

// SelfSitting reports whether the character is sitting. The hunt loop
// gates the rest transitions on it because the sit/stand action is a
// server side toggle.
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

// SelfClassID returns the class id of the played character: the skill
// teachers and the learning queue resolve through it. Zero before the
// first UserInfo.
func (b *Bot) SelfClassID() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.ClassID
}

// SelfSp returns the current skill points of the played character:
// the learning trigger and the per lesson budget of the teacher stop
// read it.
func (b *Bot) SelfSp() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Sp
}

// SkillsRevision returns the count of the SetSkills calls (and the
// weapon priority changes): the teacher stop of the hunt loop waits
// on its bump as the learn confirmation - the server answers every
// learned lesson with a fresh SkillList.
func (b *Bot) SkillsRevision() uint64 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skillsRevision
}

// SkillPlan returns a copy of the current learning queue view (nil
// when the class is unknown or nothing is left to learn). The hunt
// loop plans its teacher trips on it.
func (b *Bot) SkillPlan() *SkillPlanView {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.skillPlanViewLocked()
}

// SelfExp returns the last observed experience of the played character.
// The server refreshes the value in the UserInfo packet after every
// experience change (the PlayerStat add and remove paths both call
// updateUserInfo), so a death experience penalty shows up here at once;
// a death that removed nothing leaves the value untouched.
func (b *Bot) SelfExp() int32 {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.Exp
}

// SelfUnderAttack reports whether the character was hit within the last
// seconds. The hunt loop uses it to keep the rest logic from sitting
// down in the middle of a fight it did not start itself.
func (b *Bot) SelfUnderAttack() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    last := b.char.LastHitAt

    return !last.IsZero() && b.now().Sub(last) < underAttackWindow
}

// SelfLandedHit returns the target of the last blow of the played
// character that actually landed, with the time it landed. The miss
// flagged swings never update the pair, so a zero target means no
// landed blow yet (or since the last tracker reset).
func (b *Bot) SelfLandedHit() (int32, time.Time) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.char.LastLandedHitTarget, b.char.LastLandedHitAt
}

// SelfAttackerCount returns how many living attackable npcs hold
// the played character as their target right now: the aggro load
// of the moment. The emergency logout of the hunt loop fires on a
// social pile up - two swinging mobs grind a lone farmer down
// faster than any escape could answer.
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

// SelfHealthPercent returns the current HP of the character as a
// percentage of the maximum (0..100). An unknown maximum (no UserInfo
// yet) counts as healthy: resting forever on missing vitals is worse
// than engaging. The hunt loop uses it to decide whether the character
// is healthy enough to instantly engage the next target.
func (b *Bot) SelfHealthPercent() float64 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    if b.char.MaxHP <= 0 {
        return 100
    }
    pct := b.char.CurHP / b.char.MaxHP * 100

    return math.Min(100, math.Max(0, pct))
}

// ActiveSkills returns the learned active skills (the non passive
// ones) in ascending skill id order: the combat casting of the hunt
// loop picks its strikes, spells and buffs from the list. The answer
// is a fresh slice - the caller owns it.
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
