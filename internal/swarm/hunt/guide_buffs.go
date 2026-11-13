// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The Newbie Guide support magic of the town trips: a character in the 8-24
// level band receives the beneficial magic of the village guide for free - one
// walk to the spawn (the elven village guide, spawn id 30599, wire template
// 7599 with the 1000000 display offset) and two link clicks (the entry page
// into SupportMagic.htm, then the apply link). Each landed buff triggers an
// AbnormalStatusUpdate the tracker parses into SetBuffs. The gate refuses
// below level 8, above 24 and the non-newbie accounts, so a refusal arms a
// cooldown instead of retrying.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// The support magic constants: the wire template of the guide npc, the level
// band the gate accepts and the refusal cooldown. guideBuffWait is a var so the
// tests shorten the buff landing wait like the dialog seams.
const (
    guideTemplateID     = int32(7599)
    guideMinLevel       = int32(8)
    guideMaxLevel       = int32(24)
    guideRefuseCooldown = 10 * time.Minute
    // guideCubicLevelMin is the lower bound of the Life Cubic (skill 67)
    // window (16..19, the SupportMagic.java band): the cubic rides a summon,
    // never an AbnormalStatusUpdate, so the delevel floor reads this bound.
    guideCubicLevelMin = int32(16)
)

var guideBuffWait = 3 * time.Second

// guideBranch names the class branch a skill rides: both classes or one
// branch of the gate.
type guideBranch uint8

const (
    guideBothClasses guideBranch = iota
    guideFighterBranch
    guideMageBranch
)

// guideBuff is one support magic skill: its grant window and class branch.
type guideBuff struct {
    SkillID  int32
    MinLevel int32
    MaxLevel int32
    Branch   guideBranch
}

// guideBuffSkills are the granted abnormal buffs (the Life Cubic is a summon,
// see guideCubicLevelMin). Durations are 1200 seconds, far above the learned
// auras.
var guideBuffSkills = []guideBuff{
    // The auras of both class branches.
    {SkillID: 1204, MinLevel: 8, MaxLevel: 24, Branch: guideBothClasses},
    {SkillID: 1040, MinLevel: 11, MaxLevel: 24, Branch: guideBothClasses},
    // The fighter branch.
    {SkillID: 1045, MinLevel: 12, MaxLevel: 23, Branch: guideFighterBranch},
    {SkillID: 1068, MinLevel: 13, MaxLevel: 22, Branch: guideFighterBranch},
    {SkillID: 1044, MinLevel: 14, MaxLevel: 21, Branch: guideFighterBranch},
    {SkillID: 1086, MinLevel: 15, MaxLevel: 20, Branch: guideFighterBranch},
    // The mage branch.
    {SkillID: 1048, MinLevel: 12, MaxLevel: 23, Branch: guideMageBranch},
    {SkillID: 1085, MinLevel: 13, MaxLevel: 22, Branch: guideMageBranch},
    {SkillID: 1078, MinLevel: 14, MaxLevel: 21, Branch: guideMageBranch},
    {SkillID: 1059, MinLevel: 15, MaxLevel: 20, Branch: guideMageBranch},
}

// selfBuffSlots maps the self castable auras to the abnormal slot they fill
// (the verified abnormalTypes of the deployed skill data).
var selfBuffSlots = map[int32]string{
    72: "MD_UP", 77: "PA_UP", 86: "DMG_SHIELD", 91: "PD_UP",
    99: "ATTACK_SPEED_UP_BOW",
}

// guideBuffSlots maps the support magic to the abnormal slot it fills. The
// collision guard reads it against selfBuffSlots: the bot never casts its
// weaker aura into a fresh 1200 second guide buff slot, nor seeks a guide
// buff into a slot its own aura holds.
var guideBuffSlots = map[int32]string{
    1204: "SPEED_UP", 1040: "PD_UP", 1045: "MAX_HP_UP",
    1048: "MAX_MP_UP", 1068: "PA_UP", 1085: "CASTING_TIME_DOWN",
    1044: "HP_REGEN_UP", 1078: "CANCEL_PROB_DOWN",
    1086: "ATTACK_TIME_DOWN", 1059: "MA_UP",
}

// guideWireTemplates are the packet template ids of the guide.
var guideWireTemplates = []int32{guideTemplateID + npcdata.NPCDisplayOffset}

// guideStopNpc is the elven village guide (the Mobius spawn position).
var guideStopNpc = townNpc{
    TemplateID: guideTemplateID, Name: "Newbie Guide",
    X: 45475, Y: 48359, Z: -3056,
}

// guideForRegion returns the guide of the hunting region: the elven village
// guide serves the elven region (and the empty one), the rest answer none,
// because a stop planned for a foreign region would walk the Dion trips back
// across the map. A new region stays guide-less until its spawn is mapped.
func guideForRegion(region string) (townNpc, bool) {
    switch region {
    case regionElven, "":
        return guideStopNpc, true
    default:
        return zeroTownNpc, false
    }
}

// guideStop reports whether the current trip stop is the Newbie
// Guide support magic stop.
func (l *Loop) guideStop() bool {
    return len(l.tripStops) > 0 && l.tripStops[0].guide
}

// expectedGuideBuffs walks the support magic table and collects the
// skills the server grants the level and class right now (the table
// order is the grant order of the gate).
func expectedGuideBuffs(level int32, mage bool) []int32 {
    expected := make([]int32, 0, len(guideBuffSkills))
    for _, buff := range guideBuffSkills {
        if level < buff.MinLevel || level > buff.MaxLevel {
            continue
        }
        if buff.Branch == guideFighterBranch && mage {
            continue
        }
        if buff.Branch == guideMageBranch && !mage {
            continue
        }
        expected = append(expected, buff.SkillID)
    }

    return expected
}

// guideWanted reports whether the stop is worth planning: the level band
// holds, no refusal cooldown runs and at least one eligible skill is missing.
// A slot an active self buff fills is not worth seeking either.
func (l *Loop) guideWanted() bool {
    level := l.tracker.SelfLevel()
    if level < guideMinLevel || level > guideMaxLevel {
        return false
    }
    if !l.guideRefusedAt.IsZero() && l.now().Before(l.guideRefusedAt) {
        return false
    }
    for _, skillID := range expectedGuideBuffs(level, l.isCaster()) {
        if l.tracker.SelfHasBuff(skillID) || l.guideSlotCovered(skillID) {
            continue
        }

        return true
    }

    return false
}

// guideSlotCovered reports whether an active self buff fills the slot: the
// guide aura would overwrite the fresh own buff.
func (l *Loop) guideSlotCovered(skillID int32) bool {
    slot, ok := guideBuffSlots[skillID]
    if !ok {
        return false
    }
    for selfSkill, selfSlot := range selfBuffSlots {
        if selfSlot == slot && l.tracker.SelfHasBuff(selfSkill) {
            return true
        }
    }

    return false
}

// selfBuffSuperseded reports whether an active guide buff fills the slot the
// self skill would cast into: the 1200 second aura outlasts the own one.
func (l *Loop) selfBuffSuperseded(skillID int32) bool {
    slot, ok := selfBuffSlots[skillID]
    if !ok {
        return false
    }
    for guideSkillID, guideSlot := range guideBuffSlots {
        if guideSlot == slot && l.tracker.SelfHasBuff(guideSkillID) {
            return true
        }
    }

    return false
}

// guideRunWanted reports whether the support magic is worth a dedicated town
// trip: the region carries a guide and the stop is wanted. It is the second
// trip reason that may start outside the hunting zone - the village revive of
// a death lands next to the guide, and walking home through the aggressive
// packs unbuffed wastes the death, so the buffs come first.
func (l *Loop) guideRunWanted() bool {
    _, ok := guideForRegion(l.zoneRegion)

    return ok && l.guideWanted()
}

// planGuideStop appends the guide stop behind the learning stops (the sell
// phase calls it after planLearnStops), so one town visit sells, buys, learns
// and picks up the buffs. The stop plans once per trip and only when the
// region carries a guide.
func (l *Loop) planGuideStop() {
    guide, ok := guideForRegion(l.zoneRegion)
    if !ok || !l.guideWanted() {
        return
    }
    for index := range l.tripStops {
        if l.tripStops[index].guide {
            return
        }
    }
    l.tripStops = append(l.tripStops, tripStop{
        merchant: guide,
        buys:     nil,
        sell:     false,
        teach:    false,
        guide:    true,
    })
    l.logger.Printf("Hunt: guide: walking to the Newbie Guide for " +
        "the support magic")
}

// handleGuideStop approaches the guide and receives the support magic through
// the dialog links. Reports false while the character walks or waits for a
// guide, true when the stop is done (the buffs landed, the refusal armed the
// cooldown or the guide never showed up).
func (l *Loop) handleGuideStop(now time.Time) bool {
    if l.guideID < 0 {
        return true
    }
    if l.guideID > 0 {
        return l.approachGuide(now)
    }
    if now.Sub(l.guidePick) < selectPeriod {
        return false
    }
    l.guidePick = now
    guide, ok := l.tracker.NearestNpcByTemplates(
        guideWireTemplates, merchantFindRadius)
    if ok {
        l.guideID = guide.ObjectID
        l.logger.Printf("Hunt: guide: the Newbie Guide found, " +
            "walking to it")

        return false
    }
    if now.Sub(l.sellPhaseAt) < teacherWaitTimeout {
        return false
    }
    l.guideID = -1
    // The no-show arms the refusal cooldown too: otherwise the trip would end
    // normally and the next run would walk the same empty square.
    l.armGuideRefusal("the Newbie Guide never showed up")
    l.logger.Printf("Hunt: guide: the Newbie Guide never showed up, " +
        "skipping it")

    return true
}

// approachGuide walks the character up to the guide and receives the support
// magic at the interaction distance. The walk targets the npc approach point,
// so the chained clicks close the last stretch; the walk window bounds the
// approach. The open square needs no deck pull machinery.
func (l *Loop) approachGuide(now time.Time) bool {
    x, y, z, ok := l.tracker.ObjectPosition(l.guideID)
    if !ok {
        l.guideID = -1

        return true
    }
    selfX, selfY, selfZ, _ := l.tracker.SelfPosition()
    dist2D := math.Hypot(float64(x-selfX), float64(y-selfY))
    dz := float64(z - selfZ)
    dist3D := math.Sqrt(dist2D*dist2D + dz*dz)
    if dist3D <= npcInteractionDist {
        return l.receiveGuideMagic()
    }
    if l.guideWalkUntil.IsZero() {
        l.guideWalkUntil = now.Add(teacherApproachWindow)
    }
    if now.After(l.guideWalkUntil) {
        l.guideID = -1
        l.logger.Printf("Hunt: guide: the Newbie Guide stays out of " +
            "reach, skipping it")

        return true
    }
    ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
    approachDist2D := math.Hypot(
        float64(ax-selfX), float64(ay-selfY))
    if approachDist2D > hopCoincideDist {
        l.walkToward(ax, ay, az, now)
    }

    return false
}

// receiveGuideMagic drives the support magic dialog: the entry page links into
// SupportMagic.htm, which links the apply command. The server answers the apply
// bypass with every level-eligible buff and no page of its own (the Mobius
// handler casts and returns, sending html only on the refusal paths), so the
// last step carries AnswerIsEffect and this stop owns the outcome.
func (l *Loop) receiveGuideMagic() bool {
    if err := l.DriveDialog(l.guideID, []DialogStep{
        {LinkText: "Receive help from beneficial magic."},
        {LinkText: "Receive supplemental magic.", AnswerIsEffect: true},
    }); err != nil {
        l.armGuideRefusal("the dialog failed: " + err.Error())

        return true
    }
    l.guideAt = l.now()
    deadline := l.guideAt.Add(guideBuffWait)
    for !l.guideBuffLanded() {
        if l.now().After(deadline) {
            l.armGuideRefusal("the expected buffs never landed")

            return true
        }
        pace(questDialogPoll)
    }
    l.logger.Printf("Hunt: guide: the support magic landed")

    return true
}

// armGuideRefusal arms the refusal cooldown: the gate refused the character or
// the dialog died, so the later trips stay away until it lapses.
func (l *Loop) armGuideRefusal(reason string) {
    l.guideRefusedAt = l.now().Add(guideRefuseCooldown)
    l.logger.Printf("Hunt: guide: %s, cooldown for %s", reason,
        guideRefuseCooldown)
}

// guideBuffLanded reports whether at least one expected support
// magic skill showed up in the tracker buffs.
func (l *Loop) guideBuffLanded() bool {
    level := l.tracker.SelfLevel()
    for _, skillID := range expectedGuideBuffs(level, l.isCaster()) {
        if l.tracker.SelfHasBuff(skillID) {
            return true
        }
    }

    return false
}

// resetGuideState drops the per stop guide state, so an aborted trip leaves no
// half-driven dialog. The refusal cooldown survives across the trips.
func (l *Loop) resetGuideState() {
    l.guideID = 0
    l.guidePick = time.Time{}
    l.guideAt = time.Time{}
    l.guideWalkUntil = time.Time{}
}
