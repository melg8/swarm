// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The Newbie Guide support magic of the town trips: a character in
// the 8-24 level band may receive the beneficial magic of the village
// guide for free - one walk to the spawn (the guide of the elven
// village, spawn id 30599, wire display template 7599 - the display
// ids carry the 1000000 wire offset like the merchants) and the two
// link clicks of the dialog: the entry page offers the link into the
// SupportMagic.htm page and that page offers the link that applies
// every level-eligible buff at once. Each landing buff triggers an
// AbnormalStatusUpdate the tracker already parses into SetBuffs.
// The server gate refuses below level 8 and above level 24 and the
// non-newbie accounts (the refusal answers as html, never as buffs)
// - when the expected buffs do not land, the stop arms a cooldown
// and moves on, the trip never retries the refusal in a loop.

import (
    "math"
    "time"
)

// The support magic constants: the wire template of the guide npc,
// the level band the server gate accepts, the cooldown the refusal
// arms and the wait for the buffs to trail in after the bypass.
// guideBuffWait is a var (not a const) so the unit tests shorten
// the buff landing wait like the dialog seams (quest_walker_test).
const (
    guideTemplateID     = int32(7599)
    guideMinLevel       = int32(8)
    guideMaxLevel       = int32(24)
    guideRefuseCooldown = 10 * time.Minute
)

var guideBuffWait = 3 * time.Second

// guideBranch names the class branch a support magic skill rides:
// the Wind Walk and Shield auras answer both classes, the rest of
// the table answers one branch of the server gate.
type guideBranch uint8

const (
    guideBothClasses guideBranch = iota
    guideFighterBranch
    guideMageBranch
)

// guideBuff is one support magic skill of the Newbie Guide: the
// level window the server grants it in and the class branch it
// rides (the windows and the branches of the deployed gate).
type guideBuff struct {
    SkillID  int32
    MinLevel int32
    MaxLevel int32
    Branch   guideBranch
}

// guideBuffSkills are the support magic skills the guide grants
// (the skill ids of the server gate, the Life Cubic of the older
// server versions stays excluded - the deployed gate never grants
// it). Durations are 1200 seconds, far above the learned auras.
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

// selfBuffSlots maps the self castable aura skills of the bot to the
// abnormal type slot their effect fills (the verified abnormalTypes
// of the deployed skill data).
var selfBuffSlots = map[int32]string{
    72: "MD_UP", 77: "PA_UP", 86: "DMG_SHIELD", 91: "PD_UP",
    99: "ATTACK_SPEED_UP_BOW",
}

// guideBuffSlots maps the support magic skills to the abnormal type
// slot their effect fills: the collision guard reads it against
// selfBuffSlots - the bot never casts its weaker aura into a slot a
// fresh 1200 second guide buff holds, and never seeks a guide buff
// into a slot its own aura holds.
var guideBuffSlots = map[int32]string{
    1204: "SPEED_UP", 1040: "PD_UP", 1045: "MAX_HP_UP",
    1048: "MAX_MP_UP", 1068: "PA_UP", 1085: "CASTING_TIME_DOWN",
    1044: "HP_REGEN_UP", 1078: "CANCEL_PROB_DOWN",
    1086: "ATTACK_TIME_DOWN", 1059: "MA_UP",
}

// guideWireTemplates are the packet template ids of the Newbie Guide
// (the display id offset of the wire, see npcDisplayOffset).
var guideWireTemplates = []int32{guideTemplateID + npcDisplayOffset}

// guideStopNpc is the Newbie Guide of the elven village the guide
// stop walks to (the spawn position of the Mobius spawn data).
var guideStopNpc = townNpc{
    TemplateID: guideTemplateID, Name: "Newbie Guide",
    X: 45475, Y: 48359, Z: -3056,
}

// guideForRegion returns the Newbie Guide npc of the hunting region:
// the elven village guide serves the elven region (the default when
// the loop carries no region yet), the regions without a mapped
// guide answer none - a stop planned for a foreign region would walk
// the Dion trips back to the elven village across the whole map. The
// allowlist shape keeps the future regions honest: a new region
// stays guide-less until its guide spawn is mapped here.
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

// guideWanted reports whether the Newbie Guide stop is worth
// planning: the level band of the support magic, no refusal cooldown
// running and at least one eligible skill missing from the active
// buffs. A slot an active self buff fills is not worth seeking
// either - the guide cast would replace the fresh own aura, the bot
// keeps what it already holds.
func (l *Loop) guideWanted() bool {
    level := l.tracker.SelfLevel()
    if level < guideMinLevel || level > guideMaxLevel {
        return false
    }
    if !l.guideRefusedAt.IsZero() && time.Now().Before(l.guideRefusedAt) {
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

// guideSlotCovered reports whether the abnormal slot of the support
// magic skill holds an active self buff already: the guide aura
// would overwrite the fresh own buff, so the bot does not seek it.
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

// selfBuffSuperseded reports whether an active guide support magic
// buff fills the abnormal slot the self skill would cast into: the
// 1200 second guide aura outlasts the own one by far, the cast would
// drop the stronger effect for the weaker duplicate.
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

// guideRunWanted reports whether the support magic of the Newbie
// Guide is worth a dedicated town trip right now: the hunting region
// carries a guide and the stop is wanted (the level band holds, no
// refusal cooldown runs, at least one eligible buff is missing). It
// mirrors weaponlessRunWanted as the second trip reason that may
// start outside the hunting zone: the village revive of a death
// lands next to the guide, and walking home through the aggressive
// packs unbuffed wastes the death the character just paid for - the
// buffs come first, the return segment of the trip walks home armed
// with them.
func (l *Loop) guideRunWanted() bool {
    _, ok := guideForRegion(l.zoneRegion)

    return ok && l.guideWanted()
}

// planGuideStop appends the Newbie Guide stop of the running trip:
// the support magic rides BEHIND the learning stops (the sell phase
// calls this after planLearnStops), so one town visit sells, buys,
// learns and picks up the guide buffs on the way home. The stop
// plans once per trip - the appended stop never duplicates - and
// only when the hunting region carries a guide.
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

// handleGuideStop approaches the Newbie Guide npc and receives the
// support magic through the dialog links. It reports false while the
// character still walks toward the guide or waits for one to appear,
// true when the stop is done (the buffs landed, the refusal armed
// the cooldown or the guide never showed up).
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
    // The no-show arms the refusal cooldown too: a spawn absent from
    // the tracker would otherwise end the trip normally (the abort
    // escalation never sees it) and the next guide run would walk
    // the same empty square every short cooldown - the loop bound of
    // the refused gate applies to the invisible one as well.
    l.armGuideRefusal("the Newbie Guide never showed up")
    l.logger.Printf("Hunt: guide: the Newbie Guide never showed up, " +
        "skipping it")

    return true
}

// approachGuide walks the character right up to the guide and
// receives the support magic once the interaction distance is met.
// The walk targets the npc approach point (npcApproachPoint offsets
// toward the walker, so the chained clicks close the last stretch);
// the window bounds the whole approach - the guide stands in the
// open village square, so the plain ring walk of the teacher stop
// needs no deck pull machinery.
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

// receiveGuideMagic drives the support magic dialog of the guide:
// the entry page links into the SupportMagic.htm page and that page
// links the apply command - the server answers the apply bypass
// with every level-eligible buff (each one an AbnormalStatusUpdate
// the tracker applies) and with NO dialog page of its own (the
// Mobius SupportMagic handler casts and returns, it sends html only
// on the refusal paths), so the route's last step carries
// AnswerIsEffect and this stop owns the outcome: the buff watch
// waits for the expected skills, and when nothing lands the refusal
// cooldown arms and the trip moves on - the refusal pages of the
// gate never answer with buffs, the stop never retries them.
func (l *Loop) receiveGuideMagic() bool {
    if err := l.DriveDialog(l.guideID, []DialogStep{
        {LinkText: "Receive help from beneficial magic."},
        {LinkText: "Receive supplemental magic.", AnswerIsEffect: true},
    }); err != nil {
        l.armGuideRefusal("the dialog failed: " + err.Error())

        return true
    }
    l.guideAt = time.Now()
    deadline := l.guideAt.Add(guideBuffWait)
    for !l.guideBuffLanded() {
        if time.Now().After(deadline) {
            l.armGuideRefusal("the expected buffs never landed")

            return true
        }
        pace(questDialogPoll)
    }
    l.logger.Printf("Hunt: guide: the support magic landed")

    return true
}

// armGuideRefusal arms the refusal cooldown of the failed support
// magic: the gate refused the character (the level or the account
// html) or the dialog died - the trip moves on and the later trips
// stay away until the cooldown lapses.
func (l *Loop) armGuideRefusal(reason string) {
    l.guideRefusedAt = time.Now().Add(guideRefuseCooldown)
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

// resetGuideState drops the per stop guide state: a fresh trip
// re-finds its guide, an aborted trip leaves no half-driven dialog
// behind. The refusal cooldown survives - it gates the wanted check
// across the trips.
func (l *Loop) resetGuideState() {
    l.guideID = 0
    l.guidePick = time.Time{}
    l.guideAt = time.Time{}
    l.guideWalkUntil = time.Time{}
}
