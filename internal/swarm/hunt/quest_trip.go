// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "context"
    "errors"
    "fmt"
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The quest trip engine: the imperative chain runner the acceptance
// scenarios drive on a manual loop (the loop never ticks, the trip owns
// every move). It consumes a QuestChain - the accept conversation, then
// the stage ladder by the journal cond - through DriveDialog with the
// static entry prefix. The server side: docs/quest_protocol.md.

// questNpcScanRadius bounds the scan that finds a quest npc station.
const questNpcScanRadius = 6000.0

// questNpcFindWait bounds the wait for a quest npc to appear.
const questNpcFindWait = 30 * time.Second

// questWalkTimeout bounds one walk to a station or a kill ground: the far
// class transfer segments run tens of thousands of units.
const questWalkTimeout = 10 * time.Minute

// questSegmentLen bounds one planned segment: the 37,500 unit Gludio to
// the Ruins of Agony line would burn the 1M expansion cap of one search.
const questSegmentLen = 8000.0

// questStuckWait is the position silence that marks a stuck segment; the
// var (not a const) is a test seam.
var questStuckWait = 5 * time.Second

// questMoveStartWait bounds the move start watchdog: past the broadcast
// gate and under the stuck window (the var is a test seam).
var questMoveStartWait = 3 * time.Second

// questTransferArriveRadius is the arrival gate of a gatekeeper teleport:
// the arrival square plus the server scatter.
const questTransferArriveRadius = 2000.0

// questTransferArriveWait bounds the wait for the teleport arrival; the
// server answers the burst within a second (the var is a test seam).
var questTransferArriveWait = 20 * time.Second

// questRestSitHP is the health share the kill stage retreats at (the
// Ruins of Agony pair measured 70-80 percent of the bar).
const questRestSitHP = 55.0

// questRestStandHP is the health share the rest stands up at.
const questRestStandHP = 85.0

// questRestTimeout bounds one rest; a timeout stands up and continues
// anyway (the var is a test seam).
var questRestTimeout = 3 * time.Minute

// questPotionItemID is the Lesser Healing Potion (1060).
const questPotionItemID = 1060

// questTransitFightTimeout bounds one transit fight of the route walk: a
// chasing mob must die inside it or the walk gives up on the segment.
const questTransitFightTimeout = 2 * time.Minute

// questEquipConfirmWait bounds the inventory mutation of one equip.
const questEquipConfirmWait = 5 * time.Second

// questEquipMaxActions bounds the equip loop of one dress-up call.
const questEquipMaxActions = 20

// questArriveRadius is the arrival distance of a quest walk: inside the
// interaction distance (250) the dialog talks work.
const questArriveRadius = 200.0

// questWalkPoll is the position poll period of a quest walk.
const questWalkPoll = 400 * time.Millisecond

// questKillScanRadius bounds the engage scan of the kill ground.
const questKillScanRadius = 1600.0

// questKillStageTimeout bounds the whole kill stage: a slow respawn or a
// worse drop rate reports the stage.
const questKillStageTimeout = 20 * time.Minute

// questKillAttackPeriod paces the attack requests: the Mobius double click
// semantics need the repeated request to start the fight.
const questKillAttackPeriod = 700 * time.Millisecond

// questHealthFloor aborts the trip: a dying character is the caller's
// decision, not the engine's.
const questHealthFloor = 20.0

// FindQuestNpc waits for the quest npc station and returns its live
// object, resolving the display id onto the wire template id.
func (l *Loop) FindQuestNpc(
    npc QuestNpc, wait time.Duration,
) (state.AttackTarget, error) {
    templates := []int32{npc.TemplateID + npcdata.NPCDisplayOffset}
    deadline := l.now().Add(wait)
    for {
        found, ok := l.tracker.NearestNpcByTemplates(
            templates, questNpcScanRadius)
        if ok {
            return found, nil
        }
        if l.now().After(deadline) {
            return state.AttackTarget{}, fmt.Errorf(
                "the npc %s (template %d) never appeared",
                npc.Name, npc.TemplateID)
        }
        pace(questWalkPoll)
    }
}

// WalkQuestStation walks to a quest station (an npc cell or a kill
// ground reference) for the acceptance scenarios.
func (l *Loop) WalkQuestStation(x int32, y int32, z int32) error {
    return l.walkToQuestPoint(x, y, z, questWalkTimeout)
}

// walkToQuestPoint walks to a station and waits for the arrival poll. The
// z of a kill ground is unknown to the chain data, so the caller passes
// the current self z (the server walk corrects the height).
func (l *Loop) walkToQuestPoint(
    x int32, y int32, z int32, timeout time.Duration,
) error {
    if l.navigator != nil {
        return l.walkQuestRoute(x, y, timeout)
    }
    if err := l.game.WalkTo(x, y, z); err != nil {
        return fmt.Errorf("the walk request failed: %w", err)
    }
    deadline := l.now().Add(timeout)
    for {
        selfX, selfY, _, ok := l.tracker.SelfPosition()
        if ok && math.Hypot(
            float64(selfX-x), float64(selfY-y)) <= questArriveRadius {
            return nil
        }
        if l.now().After(deadline) {
            return fmt.Errorf(
                "the walk to (%d, %d) did not arrive", x, y)
        }
        pace(questWalkPoll)
    }
}

// fightTransitAttackers clears the mobs that chase the walking character
// until no living mob targets us, then the walk re-plans its segment.
func (l *Loop) fightTransitAttackers() error {
    attacker, ok := l.tracker.NearestAttacker()
    if !ok {
        return nil
    }
    l.logf("quest: the transit fight - %s (object %d) chases",
        attacker.Name, attacker.ObjectID)
    deadline := l.now().Add(questTransitFightTimeout)
    for {
        if l.tracker.SelfHealthPercent() <= 0 {
            return errors.New("the character died in the transit fight")
        }
        attacker, ok := l.tracker.NearestAttacker()
        if !ok {
            return nil
        }
        if l.now().After(deadline) {
            return fmt.Errorf(
                "the transit fight with %s timed out", attacker.Name)
        }
        if err := l.drinkHealingPotion(); err != nil {
            return err
        }
        if err := l.closeAndAttack(
            attacker.X, attacker.Y, attacker.Z, attacker.ObjectID); err != nil {
            return err
        }
        if l.now().Sub(l.questFightLogAt) >= 3*time.Second {
            l.questFightLogAt = l.now()
            l.logf("quest: the transit fight - %s at %.0f%% hp, "+
                "the hunter at %.0f%%", attacker.Name,
                l.tracker.ObjectHealthPercent(attacker.ObjectID),
                l.tracker.SelfHealthPercent())
        }
        pace(questKillAttackPeriod)
    }
}

// closeAndAttack swings at a mob: inside the engage radius the attack runs
// the fight, outside it the character walks up first (the server does not
// move the character on a distant attack).
func (l *Loop) closeAndAttack(x, y, z, objectID int32) error {
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the fight")
    }
    // The weapon in hand picks the engage distance: a bow shoots from the
    // weapon range (see engageRadiusFor).
    if math.Hypot(float64(x-selfX), float64(y-selfY)) > l.engageRadiusFor() {
        if l.now().Sub(l.questWalkAt) < walkRequestPeriod {
            return nil
        }
        l.questWalkAt = l.now()
        if err := l.game.WalkTo(x, y, z); err != nil {
            return fmt.Errorf("the approach walk: %w", err)
        }

        return nil
    }
    if err := l.game.AttackTarget(objectID); err != nil {
        return fmt.Errorf("the attack on object %d: %w", objectID, err)
    }

    return nil
}

// walkQuestRoute walks to the destination through planned geodata
// segments: the route splits into questSegmentLen hops along the straight
// line and every hop plans its own waypoints; a failed search falls back to
// the raw direct click. The walk re-plans a stuck segment.
//
//nolint:gocognit // the quest segment branches read best side by side
func (l *Loop) walkQuestRoute(x int32, y int32, timeout time.Duration) error {
    deadline := l.now().Add(timeout)
    for {
        // A seated character answers every click with ActionFailed: the
        // stand runs first.
        if l.tracker.SelfSitting() {
            l.logf("quest: the walk waits for the stand up")
            if err := l.ensureStanding(); err != nil {
                return err
            }
        }
        // The transit defense: the fight clears the chasers, then the walk
        // replans.
        if err := l.fightTransitAttackers(); err != nil {
            return err
        }
        // The transit mobs grind the walking character down: keep the
        // health buffer full on the way.
        if l.tracker.SelfHealthPercent() < questWalkPotionHP {
            if err := l.drinkHealingPotionAt(questWalkPotionHP); err != nil {
                return err
            }
        }
        selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
        if !ok {
            return errors.New("no self position for the route walk")
        }
        if math.Hypot(float64(selfX-x), float64(selfY-y)) <=
            questArriveRadius {
            return nil
        }
        if l.now().After(deadline) {
            return fmt.Errorf(
                "the route walk to (%d, %d) did not arrive", x, y)
        }
        segX, segY := questSegmentTarget(selfX, selfY, x, y)
        if l.followPlannedSegment(selfX, selfY, selfZ, segX, segY, deadline) {
            continue
        }
        // No geodata path for the segment: the direct click, which the
        // server stops at an obstacle.
        now := l.now()
        if now.Sub(l.questWalkAt) < walkRequestPeriod {
            pace(questWalkPoll)

            continue
        }
        segZ := selfZ
        if height, err := l.navigator.ClosestHeight(
            float64(segX), float64(segY), int16(selfZ)); err == nil {
            segZ = int32(height)
        }
        if err := l.game.WalkTo(segX, segY, segZ); err != nil {
            return fmt.Errorf("the walk request failed: %w", err)
        }
        l.questWalkAt = now
        if !l.awaitSegmentProgress(segX, segY, deadline) {
            return fmt.Errorf(
                "the walk to (%d, %d) stalled at (%d, %d)",
                x, y, selfX, selfY)
        }
    }
}

// questSegmentTarget picks the segment destination: the goal inside the
// last stretch, the point questSegmentLen along the straight line else.
func questSegmentTarget(selfX, selfY, x, y int32) (int32, int32) {
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    if dist <= questSegmentLen {
        return x, y
    }
    frac := questSegmentLen / dist

    return int32(float64(selfX) + float64(x-selfX)*frac),
        int32(float64(selfY) + float64(y-selfY)*frac)
}

// waypointOutcome reports how the follower ended one waypoint of a
// planned segment.
type waypointOutcome int

const (
    // waypointReached means the follower arrived; a lapsed segment budget
    // ends the plan the same way.
    waypointReached waypointOutcome = iota
    // waypointStalled means the character stopped short: the segment is
    // dead and the route re-plans.
    waypointStalled
    // waypointBlocked means the click never started the movement, so the
    // follower skips the waypoint within this plan.
    waypointBlocked
    // waypointFailed means a request error ended the waypoint and the
    // caller falls back to the direct click.
    waypointFailed
)

// followPlannedSegment plans the geodata path of one segment and walks its
// waypoints; blocked ones skip within the plan, since a re-plan from the
// same position answers the identical waypoint.
func (l *Loop) followPlannedSegment(
    selfX, selfY, selfZ, segX, segY int32, deadline time.Time,
) bool {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    dest := pathfind.Vec3{
        X: float64(segX), Y: float64(segY), Z: float64(selfZ),
    }
    result, err := l.navigator.FindPathApproach(
        from, dest, questArriveRadius)
    if err != nil || result == nil || len(result.Waypoints) == 0 {
        return false
    }
    // The partial round (docs/navmesh.md): a segment the search cannot
    // complete still walks its closest reachable waypoints.
    partial := !result.Found
    waypoints := result.Waypoints
    // The plan starts at the own cell resolved on the pack: this measures
    // the frame offset the waypoint clicks ride (click_frame.go).
    frameOffset := measureFrameOffset(selfZ, waypoints[0].Z)
    if partial {
        l.logf("quest: walking a partial segment toward (%d, %d), "+
            "the closest reachable point is %d waypoints ahead",
            segX, segY, len(waypoints))
    } else {
        l.logf("quest: walking a planned segment to (%d, %d) through "+
            "%d waypoints from (%d, %d)", segX, segY, len(waypoints),
            selfX, selfY)
    }
    blocked := 0
    for i := range waypoints {
        switch l.followWaypoint(waypoints, i, frameOffset, deadline) {
        case waypointReached:
        case waypointStalled:
            return true
        case waypointBlocked:
            blocked++
        case waypointFailed:
            return false
        }
    }
    if blocked == len(waypoints) {
        // No waypoint could start a walk: the direct click keeps the
        // failure bounded instead of looping the plan.
        return false
    }
    // A waypoint inside the approach radius would "complete" without
    // moving a cell and spin the planner: no movement reports false.
    afterX, afterY, _, ok := l.tracker.SelfPosition()
    if ok && afterX == selfX && afterY == selfY {
        return false
    }

    return true
}

// followWaypoint walks toward the waypoint until it is reached or the walk
// stalls; the verdict separates a dead segment, a dead click and an error.
//
//nolint:cyclop,funlen,gocognit // the refusal branches read side by side
func (l *Loop) followWaypoint(
    waypoints []pathfind.Vec3, index int, frameOffset float64,
    deadline time.Time,
) waypointOutcome {
    wp := waypoints[index]
    sent := false
    clickAt := time.Time{}
    clickX, clickY := int32(0), int32(0)
    lastX, lastY := int32(0), int32(0)
    stuckSince := time.Time{}
    for {
        if l.now().After(deadline) {
            return waypointReached
        }
        selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
        if !ok {
            pace(questWalkPoll)

            continue
        }
        radius := waypointPassDist
        if index == len(waypoints)-1 {
            radius = questArriveRadius
        }
        // The anchored distance keeps the vintage shift out of the arrival
        // test (waypointDistanceAnchored).
        if waypointDistanceAnchored(wp, selfX, selfY, selfZ,
            frameOffset) <= radius {
            return waypointReached
        }
        moving := selfX != lastX || selfY != lastY
        if moving {
            lastX, lastY = selfX, selfY
            stuckSince = time.Time{}
        } else if stuckSince.IsZero() {
            stuckSince = l.now()
        }
        // The potions ride the follower poll: one planned segment runs over
        // a minute, and drinking between the segments arrived at 39 percent.
        if err := l.drinkHealingPotionAt(questWalkPotionHP); err != nil {
            l.logf("quest: the potion drink failed: %v", err)

            return waypointFailed
        }
        if !stuckSince.IsZero() && l.now().Sub(stuckSince) >= questStuckWait {
            return waypointStalled
        }
        // The move start watchdog: a click that started no movement is
        // dead, so the wait runs to the verdict.
        now := l.now()
        if sent && selfX == clickX && selfY == clickY {
            if now.Sub(clickAt) >= questMoveStartWait &&
                !l.tracker.SelfWalking() {
                l.logf("quest: the click to (%d, %d) never started the "+
                    "walk, leaving the waypoint", int32(wp.X), int32(wp.Y))

                return waypointBlocked
            }
            pace(questWalkPoll)

            continue
        }
        // The pacing: no request while the character walks; a repeat click
        // only for the standing one (the flood protector mutes above one
        // action per second).
        if sent && (moving || now.Sub(l.questWalkAt) < walkRequestPeriod) {
            pace(questWalkPoll)

            continue
        }
        if err := l.clickQuestWaypoint(wp, frameOffset, selfZ); err != nil {
            l.logf("quest: the waypoint walk failed: %v", err)

            return waypointFailed
        }
        sent = true
        clickAt = now
        clickX, clickY = selfX, selfY
        l.questWalkAt = now
        pace(questWalkPoll)
    }
}

// clickQuestWaypoint clicks one waypoint: the click carries the geodata
// height, transported into the server frame of the walked surface.
func (l *Loop) clickQuestWaypoint(
    wp pathfind.Vec3, frameOffset float64, selfZ int32,
) error {
    clickZ := selfZ
    if wp.Z != 0 {
        clickZ = int32(anchorZToServerFrame(wp.Z, frameOffset))
    }

    return l.game.WalkTo(int32(wp.X), int32(wp.Y), clickZ)
}

// awaitSegmentProgress waits until the character moves a meaningful
// distance or the deadline lapses; false marks a stalled click.
func (l *Loop) awaitSegmentProgress(segX, segY int32, deadline time.Time) bool {
    startX, startY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    clickAt := l.now()
    for {
        if l.now().After(deadline) {
            return false
        }
        selfX, selfY, _, ok := l.tracker.SelfPosition()
        if !ok {
            pace(questWalkPoll)

            continue
        }
        if math.Hypot(float64(selfX-startX), float64(selfY-startY)) >
            waypointPassDist {
            return true
        }
        if math.Hypot(float64(selfX-segX), float64(selfY-segY)) <=
            questArriveRadius {
            return true
        }
        // The move start watchdog: a click that started no movement is
        // dead, and the route gives up instead of standing out the budget.
        if l.now().Sub(clickAt) >= questMoveStartWait &&
            !l.tracker.SelfWalking() {
            return false
        }
        pace(questWalkPoll)
    }
}

// questItemCount sums the journal counters of a kill stage.
func questItemCount(tracker *state.Bot, itemIDs []int32) int32 {
    total := int32(0)
    for _, itemID := range itemIDs {
        count, ok := tracker.QuestItemCount(itemID)
        if ok {
            total += count
        }
    }

    return total
}

// DriveQuestChain runs one quest chain end to end: the accept conversation
// when the journal lacks the quest, then the stage ladder until the exit
// talk drops it. Every failure names its stage.
func (l *Loop) DriveQuestChain(
    ctx context.Context, chain QuestChain,
) error {
    if err := ctx.Err(); err != nil {
        return fmt.Errorf("quest %d: %w", chain.QuestID, err)
    }
    if _, ok := l.tracker.QuestCond(chain.QuestID); !ok {
        l.logf("quest %d: driving the accept route at %s",
            chain.QuestID, chain.Start.Name)
        route := append(QuestEntryLinks(chain), chain.Accept...)
        if err := l.driveQuestTalk(chain, chain.Start, route); err != nil {
            return fmt.Errorf(
                "quest %d accept: %w", chain.QuestID, err)
        }
    }
    for {
        if err := ctx.Err(); err != nil {
            return fmt.Errorf("quest %d: %w", chain.QuestID, err)
        }
        cond, ok := l.tracker.QuestCond(chain.QuestID)
        if !ok {
            l.logf("quest %d: the journal dropped the quest, "+
                "the chain is complete", chain.QuestID)

            return nil
        }
        stage, ok := QuestStageByCond(chain, cond)
        if !ok {
            return fmt.Errorf(
                "quest %d: the journal cond %d sits outside "+
                    "the ladder", chain.QuestID, cond)
        }
        if err := l.driveQuestStage(ctx, chain, stage); err != nil {
            return fmt.Errorf("quest %d stage %d: %w",
                chain.QuestID, stage.Cond, err)
        }
    }
}

// driveQuestStage runs one stage: the optional gatekeeper segment first,
// then the talk half drives the dialog route, the kill half farms.
func (l *Loop) driveQuestStage(
    ctx context.Context, chain QuestChain, stage QuestStage,
) error {
    if stage.Transfer != nil {
        if err := l.driveQuestTransfer(ctx, *stage.Transfer); err != nil {
            return fmt.Errorf("the transfer to %s: %w",
                stage.Transfer.DestLabel, err)
        }
    }
    if stage.TalkNpc.TemplateID == 0 {
        return l.farmQuestStage(ctx, stage)
    }
    route := append(QuestEntryLinks(chain), stage.Links...)
    l.logf("quest %d: the talk stage at %s",
        chain.QuestID, stage.TalkNpc.Name)

    return l.driveQuestTalk(chain, stage.TalkNpc, route)
}

// driveQuestTransfer rides one gatekeeper hop: the walk, the talk that
// opens the first page (the html action cache validates only the open
// page), the showTeleports list, the destination button and the arrival.
func (l *Loop) driveQuestTransfer(
    ctx context.Context, transfer QuestTransfer,
) error {
    if err := ctx.Err(); err != nil {
        return fmt.Errorf("cancelled: %w", err)
    }
    target, err := l.FindQuestNpc(
        transfer.Gatekeeper, questNpcFindWait)
    if err != nil {
        return err
    }
    if err := l.walkToQuestPoint(
        target.X, target.Y, target.Z, questWalkTimeout); err != nil {
        return fmt.Errorf("the walk to %s: %w",
            transfer.Gatekeeper.Name, err)
    }
    // The arrival generation the conversation starts from: the server
    // validates every bypass against the links of the last page, so a page
    // left over from an earlier talk never satisfies a wait.
    _, _, talkGen := l.game.LastHTMLDialogArrival()
    l.logf("quest: talking to the gatekeeper %s (object %d), "+
        "riding the teleport to %s",
        transfer.Gatekeeper.Name, target.ObjectID, transfer.DestLabel)

    pacer := bypassPacer{last: time.Time{}}
    teleport, err := l.openTransferList(target, transfer, talkGen, &pacer)
    if err != nil {
        return err
    }
    // The evidence of the hop: the command the button carries (sent
    // verbatim, the server matches the cached link byte for byte) and the
    // geometry its interaction distance gate reads.
    l.logf("quest: teleporting to %s via %q (npc %d, %s)",
        teleport.Label, teleport.Command, target.ObjectID,
        l.transferGeometry(target.ObjectID))
    // The list page arrives one round trip after the showTeleports
    // bypass: a teleport riding it sits inside the flood protector window
    // and is dropped (dialogBypassPace).
    pacer.wait()
    if err := l.game.SendBypass(teleport.Command); err != nil {
        return fmt.Errorf("the teleport send: %w", err)
    }

    return l.awaitTransferArrival(ctx, transfer)
}

// openTransferList opens the teleport list and returns the destination
// button: the talk, the showTeleports bypass and the list page wait.
func (l *Loop) openTransferList(
    target state.AttackTarget, transfer QuestTransfer, talkGen uint64,
    pacer *bypassPacer,
) (*BypassButton, error) {
    // The talk opens the first page (the walk-in gate of the html action
    // cache); the pull follows one pace later.
    if err := l.game.ClickObject(target.ObjectID); err != nil {
        return nil, fmt.Errorf("the gatekeeper click: %w", err)
    }
    pace(dialogClickPause)
    if err := l.game.InteractPull(target.ObjectID); err != nil {
        return nil, fmt.Errorf("the gatekeeper pull: %w", err)
    }
    first, firstGen, err := l.awaitTransferPage(
        target.ObjectID, "", talkGen, questTransferArriveWait)
    if err != nil {
        return nil, err
    }
    if FindShowTeleportsButton(first) == nil {
        return nil, fmt.Errorf(
            "the first page of %s carries no teleport entry",
            transfer.Gatekeeper.Name)
    }
    pacer.wait()
    if err := l.game.SendBypass(fmt.Sprintf(
        "npc_%d_showTeleports", target.ObjectID)); err != nil {
        return nil, fmt.Errorf("the showTeleports send: %w", err)
    }
    buttons, _, err := l.awaitTransferPage(
        target.ObjectID, transfer.DestLabel, firstGen,
        questTransferArriveWait)
    if err != nil {
        return nil, err
    }
    teleport := FindTeleportButton(
        buttons, "NORMAL", transfer.DestLabel)
    if teleport == nil {
        return nil, fmt.Errorf(
            "the teleport list of %s carries no %s button",
            transfer.Gatekeeper.Name, transfer.DestLabel)
    }

    return teleport, nil
}

// transferGeometry renders the server range check of a teleport bypass:
// RequestBypassToServer drops it beyond Npc.INTERACTION_DISTANCE (250
// units, 3D) from the page origin.
func (l *Loop) transferGeometry(npcObjID int32) string {
    npcX, npcY, npcZ, npcOK := l.tracker.ObjectPosition(npcObjID)
    selfX, selfY, selfZ, selfOK := l.tracker.SelfPosition()
    if !npcOK || !selfOK {
        return "geometry unknown"
    }
    dx := float64(selfX - npcX)
    dy := float64(selfY - npcY)
    dz := float64(selfZ - npcZ)

    return fmt.Sprintf(
        "self %d,%d,%d npc %d,%d,%d 2d %.0f 3d %.0f",
        selfX, selfY, selfZ, npcX, npcY, npcZ,
        math.Hypot(dx, dy), math.Sqrt(dx*dx+dy*dy+dz*dz))
}

// awaitTransferPage waits for a dialog page past minGen that carries the wanted
// button; the generation gate keeps an older page from satisfying the wait.
func (l *Loop) awaitTransferPage(
    npcObjID int32, destLabel string, minGen uint64, wait time.Duration,
) ([]BypassButton, uint64, error) {
    deadline := l.now().Add(wait)
    for {
        id, html, gen := l.game.LastHTMLDialogArrival()
        if id == npcObjID && html != "" && gen > minGen {
            buttons := ParseGatekeeperHTML(html)
            if destLabel == "" {
                if FindShowTeleportsButton(buttons) != nil {
                    return buttons, gen, nil
                }
            } else if FindTeleportButton(
                buttons, "NORMAL", destLabel) != nil {
                return buttons, gen, nil
            }
        }
        if l.now().After(deadline) {
            return nil, 0, fmt.Errorf(
                "the gatekeeper dialog with %q never arrived",
                destLabel)
        }
        pace(gatekeeperPollPeriod)
    }
}

// awaitTransferArrival waits until the tracker places the character at the
// arrival square (the connection layer sends the Appearing answer).
func (l *Loop) awaitTransferArrival(
    ctx context.Context, transfer QuestTransfer,
) error {
    deadline := l.now().Add(questTransferArriveWait)
    for {
        if err := ctx.Err(); err != nil {
            return fmt.Errorf("cancelled: %w", err)
        }
        selfX, selfY, _, ok := l.tracker.SelfPosition()
        if ok && math.Hypot(
            float64(selfX-transfer.ArriveX),
            float64(selfY-transfer.ArriveY)) <=
            questTransferArriveRadius {
            l.logf("quest: the teleport to %s landed at (%d, %d)",
                transfer.DestLabel, selfX, selfY)

            return nil
        }
        if l.now().After(deadline) {
            if !ok {
                return fmt.Errorf(
                    "the teleport to %s never landed and the self "+
                        "position is unknown", transfer.DestLabel)
            }

            return fmt.Errorf(
                "the teleport to %s never landed: the character "+
                    "stands at %d,%d, the square is %d,%d",
                transfer.DestLabel, selfX, selfY,
                transfer.ArriveX, transfer.ArriveY)
        }
        pace(questWalkPoll)
    }
}

// driveQuestTalk finds the station npc, walks into the interaction
// distance and drives the dialog route.
func (l *Loop) driveQuestTalk(
    chain QuestChain, npc QuestNpc, route []DialogStep,
) error {
    if len(route) == 0 {
        return errors.New("the talk route is empty")
    }
    target, err := l.FindQuestNpc(npc, questNpcFindWait)
    if err != nil {
        return err
    }
    if err := l.walkToQuestPoint(
        target.X, target.Y, target.Z, questWalkTimeout); err != nil {
        return fmt.Errorf("the walk to %s: %w", npc.Name, err)
    }
    l.logf("quest %d: talking to %s (object %d) through %d links",
        chain.QuestID, npc.Name, target.ObjectID, len(route))

    return l.DriveDialog(target.ObjectID, route)
}

// questRetreatDistance walks the retreat out of the mob radius.
const questRetreatDistance = 2600.0

// retreatAndRest walks the tired hunter out of the mob radius, sits the
// health back and returns to the kill ground, away from the nearest mob.
func (l *Loop) retreatAndRest(ctx context.Context, stage QuestStage) error {
    _, _, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the retreat")
    }
    selfX, selfY, _, _ := l.tracker.SelfPosition()
    templates := make([]int32, 0, len(stage.Kill.Mobs))
    for _, mob := range stage.Kill.Mobs {
        templates = append(templates, mob+npcdata.NPCDisplayOffset)
    }
    awayX, awayY := selfX, selfY
    if mob, found := l.tracker.NearestNpcByTemplates(
        templates, questKillScanRadius); found {
        dx := float64(selfX - mob.X)
        dy := float64(selfY - mob.Y)
        if dist := math.Hypot(dx, dy); dist > 1 {
            awayX = int32(float64(selfX) + dx/dist*questRetreatDistance)
            awayY = int32(float64(selfY) + dy/dist*questRetreatDistance)
        }
    }
    l.logf("quest: the retreat from the mob crowd to (%d, %d)",
        awayX, awayY)
    if err := l.walkToQuestPoint(
        awayX, awayY, selfZ, questWalkTimeout); err != nil {
        return fmt.Errorf("the retreat walk: %w", err)
    }
    // The retreat point sits outside the aggro radius, but a locked chaser
    // covers it faster than the sit heals: nothing may target the sitting
    // character.
    if err := l.fightTransitAttackers(); err != nil {
        return fmt.Errorf("the retreat fight: %w", err)
    }
    if err := l.restBetweenFights(ctx); err != nil {
        return err
    }
    l.logf("quest: the rested hunter returns to the kill ground")

    return l.walkToQuestPoint(
        stage.Kill.GroundX, stage.Kill.GroundY, selfZ, questWalkTimeout)
}

// farmQuestStage runs a kill stage: the walk to the ground and the engage loop
// until the journal counters fill; a dead ground rests for the respawn.
//
//nolint:cyclop,gocognit // the hunt and quest checks read side by side
func (l *Loop) farmQuestStage(
    ctx context.Context, stage QuestStage,
) error {
    kill := stage.Kill
    _, _, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the ground walk")
    }
    if err := l.walkToQuestPoint(
        kill.GroundX, kill.GroundY, selfZ, questWalkTimeout); err != nil {
        return fmt.Errorf("the walk to the kill ground: %w", err)
    }
    templates := make([]int32, 0, len(kill.Mobs))
    for _, mob := range kill.Mobs {
        templates = append(templates, mob+npcdata.NPCDisplayOffset)
    }
    deadline := l.now().Add(questKillStageTimeout)
    for {
        if err := ctx.Err(); err != nil {
            return err
        }
        if total := questItemCount(l.tracker,
            kill.ItemIDs); total >= kill.Target {
            l.logf("quest: the kill stage counters filled (%d)", total)

            return nil
        }
        if l.now().After(deadline) {
            return fmt.Errorf(
                "the kill stage timed out at %d of %d items",
                questItemCount(l.tracker, kill.ItemIDs), kill.Target)
        }
        if l.tracker.SelfHealthPercent() < questHealthFloor {
            return errors.New("the health floor breached")
        }
        if l.tracker.SelfHealthPercent() <= 0 {
            return errors.New("the character died on the kill ground")
        }
        // The retreat policy: a tired hunter walks out, sits the health
        // back and returns.
        if l.tracker.SelfHealthPercent() < questRestSitHP {
            if err := l.retreatAndRest(ctx, stage); err != nil {
                return err
            }

            continue
        }
        mob, ok := l.tracker.NearestNpcByTemplates(
            templates, questKillScanRadius)
        if !ok {
            // The ground is momentarily dead: resting and pacing keeps
            // the loop alive for the rescan.
            if err := l.restBetweenFights(ctx); err != nil {
                return err
            }
            pace(questKillAttackPeriod)

            continue
        }
        // A quest mob stands in range: the tired character drinks a potion
        // instead of sitting down in the camp.
        if err := l.drinkHealingPotion(); err != nil {
            return err
        }
        if err := l.closeAndAttack(mob.X, mob.Y, mob.Z,
            mob.ObjectID); err != nil {
            return fmt.Errorf("the attack on %s: %w", mob.Name, err)
        }
        pace(questKillAttackPeriod)
    }
}

// ensureStanding stands the character up and waits for the server
// confirmation: every walk click of a seated character answers ActionFailed.
func (l *Loop) ensureStanding() error {
    if !l.tracker.SelfSitting() {
        return nil
    }
    for range 3 {
        if err := l.game.ActionSitStand(); err != nil {
            return fmt.Errorf("the stand request: %w", err)
        }
        deadline := l.now().Add(3 * time.Second)
        for l.now().Before(deadline) {
            if !l.tracker.SelfSitting() {
                return nil
            }
            pace(gatekeeperPollPeriod)
        }
    }

    return errors.New("the character stays seated")
}

// restBetweenFights parks a tired character until the sitting regeneration
// reaches the stand threshold; a healthy one returns at once.
func (l *Loop) restBetweenFights(ctx context.Context) error {
    if l.tracker.SelfHealthPercent() >= questRestSitHP {
        return nil
    }
    l.logf("quest: resting at %.0f%% health",
        l.tracker.SelfHealthPercent())
    if err := l.game.ActionSitStand(); err != nil {
        return fmt.Errorf("the sit request: %w", err)
    }
    deadline := l.now().Add(questRestTimeout)
    for {
        if err := ctx.Err(); err != nil {
            _ = l.game.ActionSitStand()

            return fmt.Errorf("cancelled: %w", err)
        }
        // A fresh attacker aborts the rest: the sitting regeneration loses
        // to any chaser still swinging (the caller's fight clears it).
        if attacker, ok := l.tracker.NearestAttacker(); ok {
            l.logf("quest: the rest aborted - %s attacks",
                attacker.Name)
            if err := l.ensureStanding(); err != nil {
                return err
            }

            return nil
        }
        hp := l.tracker.SelfHealthPercent()
        if hp >= questRestStandHP {
            break
        }
        if l.now().After(deadline) {
            l.logf("quest: the rest timed out at %.0f%% health", hp)

            break
        }
        pace(questWalkPoll)
    }
    if err := l.ensureStanding(); err != nil {
        return err
    }
    l.logf("quest: the rest ended at %.0f%% health",
        l.tracker.SelfHealthPercent())

    return nil
}

// questWalkPotionHP is the health share the route walk drinks at: the
// assisting mobs grind a passing character down.
const questWalkPotionHP = 80.0

// questPotionReuse is the reuse delay of the healing potions (10 s on the
// Lesser Healing Potion): the server drops a use request inside the window,
// so the engine paces its own sends.
const questPotionReuse = 10 * time.Second

// drinkHealingPotion restores the health mid fight: the Lesser Healing
// Potion of the bag (when one is left), a no-op for a healthy character.
func (l *Loop) drinkHealingPotion() error {
    return l.drinkHealingPotionAt(questRestSitHP)
}

// drinkHealingPotionAt drinks the first healing potion when the health
// sits below the threshold and the reuse window has lapsed.
func (l *Loop) drinkHealingPotionAt(threshold float64) error {
    if l.tracker.SelfHealthPercent() >= threshold {
        return nil
    }
    if !l.questPotionAt.IsZero() &&
        l.now().Sub(l.questPotionAt) < questPotionReuse {
        return nil
    }
    for _, item := range l.tracker.InventoryItems() {
        if item.ItemID == questPotionItemID && item.Count > 0 &&
            !item.Equipped {
            l.logf("quest: drinking a healing potion at %.0f%% health",
                l.tracker.SelfHealthPercent())
            before := l.tracker.InventoryVersion()
            if err := l.game.UseItem(item.ObjectID); err != nil {
                return fmt.Errorf("the potion use: %w", err)
            }
            l.questPotionAt = l.now()
            l.awaitInventoryMutation(before, questEquipConfirmWait)

            return nil
        }
    }

    return nil
}

// EquipBaggedGear dresses the character from the bag: the blocking auto
// equipment for the manual loop, one planned upgrade per cycle until the
// paperdoll holds no further upgrade or the budget lapses.
func (l *Loop) EquipBaggedGear() error {
    if l.equip == nil || l.game == nil {
        return errors.New("the equip manager is not wired")
    }
    lastObject := int32(0)
    retries := 0
    for range questEquipMaxActions {
        action, ok := gear.NextUpgrade(l.equip.profile, l.equipment())
        if !ok {
            return nil
        }
        if action.ObjectID == lastObject {
            retries++
            if retries > 3 {
                return fmt.Errorf(
                    "the equip of %s never confirmed", action.Reason)
            }
        } else {
            retries = 0
            lastObject = action.ObjectID
        }
        before := l.tracker.InventoryVersion()
        if err := l.game.UseItem(action.ObjectID); err != nil {
            return fmt.Errorf("the equip of %s: %w", action.Reason, err)
        }
        if !l.awaitInventoryMutation(before, questEquipConfirmWait) {
            return fmt.Errorf(
                "the equip of %s never confirmed", action.Reason)
        }
        l.logf("quest: gear: %s", action.Reason)
    }

    return errors.New("the equip loop reached the action budget")
}

// awaitInventoryMutation waits for a new inventory version.
func (l *Loop) awaitInventoryMutation(
    before uint64, wait time.Duration,
) bool {
    deadline := l.now().Add(wait)
    for {
        if l.tracker.InventoryVersion() != before {
            return true
        }
        if l.now().After(deadline) {
            return false
        }
        pace(gatekeeperPollPeriod)
    }
}
