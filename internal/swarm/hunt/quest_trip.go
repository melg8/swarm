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
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The quest trip engine: the imperative chain runner the acceptance
// scenarios drive on a manual loop (SetAutonomy(false), the loop
// never ticks - the trip owns every move). The engine consumes a
// QuestChain of quest_chains.go: the accept conversation at the
// start npc, then the stage ladder by the journal cond the tracker
// reports - a talk stage walks to its npc and drives the dialog
// route of the walker (the QuestEntryLinks prefix plus the stage
// links, the STATIC page entry every conversation starts from), a
// kill stage walks to the ground and engages the quest mobs until
// the journal item counters fill - until the exit talk drops the
// quest and the proof item lands. The class change segment (the Rains
// route of quest_chains.go) stays with the caller: it rides the
// same DriveDialog on the manual loop.
//
// The two live-pinned server facts of the T-016 hand-off hold by
// construction: every conversation goes through DriveDialog (the
// bypass pace lives inside one call, the next conversation starts
// with the two-click entry after the previous one returned) and the
// entry prefix handles the STATIC trainer page every quest npc talk
// opens with.

// questNpcScanRadius bounds the world scan that finds a quest npc
// station (the knownlist push of the approach walk).
const questNpcScanRadius = 6000.0

// questNpcFindWait bounds the wait for a quest npc to appear in the
// tracker world before the trip gives up on the station.
const questNpcFindWait = 30 * time.Second

// questWalkTimeout bounds one walk to a station or a kill ground:
// the far segments of the class transfer chain (Gludio to the Ruins of
// Agony, the Ol Mahum camps north of Gludin) run over 30 000 units
// of planned segments, so the bound holds the whole walk plus the
// re-plans.
const questWalkTimeout = 10 * time.Minute

// questSegmentLen bounds one planned segment of the route walk: the
// geodata search of a long segment (Gludio to the Ruins of Agony is
// 37 500 units) would burn the shipped 1M expansion cap, so the
// route splits into segments of this length and every segment plans
// its own path (~200k expansions, far under the cap).
const questSegmentLen = 8000.0

// questStuckWait is the position silence that marks a stuck segment
// walk: no cell of movement for this window re-plans the segment.
// The var (not a const) is a test seam: the route tests shorten it.
var questStuckWait = 5 * time.Second

// questTransferArriveRadius is the arrival gate of a gatekeeper
// teleport: the arrival square of the teleporter xml plus the
// scatter the server drops the character at.
const questTransferArriveRadius = 2000.0

// questTransferArriveWait bounds the wait for the teleport arrival:
// the server answers the bypass with the TeleportToLocation burst
// within a second, the bound covers a slow tick. The var (not a
// const) is a test seam: the transfer tests shorten it.
var questTransferArriveWait = 20 * time.Second

// questRestSitHP is the health share the kill stage retreats at:
// the manual trip has no rest phase of the hunt loop, so the farm
// loop itself parks the character until the sitting regeneration
// covers the next fights. The live runs pinned the honest fight
// cost of the Ruins of Agony pair at 70-80 percent of the health
// bar (two assisting skeletons of the 17-22 band hit through the
// full D-grade dress at ~60 health per second), so the retreat
// must fire well above the health floor - waiting for 40 percent
// sent the hunter back into the crowd at 13 and it never came out.
const questRestSitHP = 55.0

// questRestStandHP is the health share the rest stands up at.
const questRestStandHP = 85.0

// questRestTimeout bounds one rest: the sitting regeneration of a
// level 20 fighter covers the window with a wide margin; a timeout
// stands up and continues anyway (the health floor still guards).
// The var (not a const) is a test seam: the rest tests shorten it.
var questRestTimeout = 3 * time.Minute

// questPotionItemID is the healing potion of the quest trips (the
// Lesser Healing Potion of the classic item table).
const questPotionItemID = 1060

// questTransitFightTimeout bounds one transit fight of the route
// walk: an aggressive mob that chases the walking character must die
// inside it or the walk gives up on the segment.
const questTransitFightTimeout = 2 * time.Minute

// questEquipConfirmWait bounds the wait for the inventory mutation
// after one equip request of EquipBaggedGear.
const questEquipConfirmWait = 5 * time.Second

// questEquipMaxActions bounds the equip loop of one dress-up call.
const questEquipMaxActions = 20

// questArriveRadius is the arrival distance of a quest walk: inside
// the interaction distance (250) the dialog talks work, so the walk
// stops just short of the station.
const questArriveRadius = 200.0

// questWalkPoll is the position poll period of a quest walk.
const questWalkPoll = 400 * time.Millisecond

// questKillScanRadius bounds the engage scan for the quest mobs
// around the character standing on the kill ground.
const questKillScanRadius = 1600.0

// questKillStageTimeout bounds the whole kill stage: the counters
// must fill inside it or the trip reports the stage (a respawn too
// slow or a drop rate worse than the data claims).
const questKillStageTimeout = 20 * time.Minute

// questKillAttackPeriod paces the attack requests of the kill
// engage: the Mobius double click semantics need the repeated
// request to start the fight, and the running fight needs no
// tighter loop than this.
const questKillAttackPeriod = 700 * time.Millisecond

// questHealthFloor aborts the trip when the character falls below
// the health share: the manual trip has no rest phase of the hunt
// loop, a dying character is the caller's decision, not the
// engine's.
const questHealthFloor = 20.0

// FindQuestNpc waits for the quest npc station to appear in the
// tracker world (the knownlist push of the world entry or the
// approach walk) and returns its live object. The scan walks the
// npcdata display id onto the wire template id (the
// TemplateID+npcDisplayOffset convention of the chain data).
func (l *Loop) FindQuestNpc(
    npc QuestNpc, wait time.Duration,
) (state.AttackTarget, error) {
    templates := []int32{npc.TemplateID + npcDisplayOffset}
    deadline := time.Now().Add(wait)
    for {
        found, ok := l.tracker.NearestNpcByTemplates(
            templates, questNpcScanRadius)
        if ok {
            return found, nil
        }
        if time.Now().After(deadline) {
            return state.AttackTarget{}, fmt.Errorf(
                "the npc %s (template %d) never appeared",
                npc.Name, npc.TemplateID)
        }
        pace(questWalkPoll)
    }
}

// WalkQuestStation walks to a quest station (an npc cell or a kill
// ground reference): the public form of the quest walk the
// acceptance scenarios drive on a manual loop.
func (l *Loop) WalkQuestStation(x int32, y int32, z int32) error {
    return l.walkToQuestPoint(x, y, z, questWalkTimeout)
}

// walkToQuestPoint walks to a station and waits for the arrival
// poll to close inside the arrival radius. The z of a kill ground
// is unknown to the chain data, so the caller passes the current
// self z there (the server walk corrects the height along the way).
func (l *Loop) walkToQuestPoint(
    x int32, y int32, z int32, timeout time.Duration,
) error {
    if l.navigator != nil {
        return l.walkQuestRoute(x, y, timeout)
    }
    if err := l.game.WalkTo(x, y, z); err != nil {
        return fmt.Errorf("the walk request failed: %w", err)
    }
    deadline := time.Now().Add(timeout)
    for {
        selfX, selfY, _, ok := l.tracker.SelfPosition()
        if ok && math.Hypot(
            float64(selfX-x), float64(selfY-y)) <= questArriveRadius {
            return nil
        }
        if time.Now().After(deadline) {
            return fmt.Errorf(
                "the walk to (%d, %d) did not arrive", x, y)
        }
        pace(questWalkPoll)
    }
}

// fightTransitAttackers clears the mobs that chase the walking
// character: the fight engages the nearest attacker until the
// tracker holds no living mob that targets us, then returns (the
// walk re-plans its segment from the standing cell). A no-op when
// nothing attacks.
func (l *Loop) fightTransitAttackers() error {
    attacker, ok := l.tracker.NearestAttacker()
    if !ok {
        return nil
    }
    l.logf("quest: the transit fight - %s (object %d) chases",
        attacker.Name, attacker.ObjectID)
    deadline := time.Now().Add(questTransitFightTimeout)
    for {
        if l.tracker.SelfHealthPercent() <= 0 {
            return errors.New("the character died in the transit fight")
        }
        attacker, ok := l.tracker.NearestAttacker()
        if !ok {
            return nil
        }
        if time.Now().After(deadline) {
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
        if time.Since(l.questFightLogAt) >= 3*time.Second {
            l.questFightLogAt = time.Now()
            l.logf("quest: the transit fight - %s at %.0f%% hp, "+
                "the hunter at %.0f%%", attacker.Name,
                l.tracker.ObjectHealthPercent(attacker.ObjectID),
                l.tracker.SelfHealthPercent())
        }
        pace(questKillAttackPeriod)
    }
}

// closeAndAttack swings at a mob: inside the engage radius the attack
// request runs the fight; outside it the character walks up first
// (the Mobius server does not move the character on a distant
// attack - the official client approaches on its own; the first
// live quest runs attacked from a thousand units away, hit nothing
// and died standing).
func (l *Loop) closeAndAttack(x, y, z, objectID int32) error {
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the fight")
    }
    if math.Hypot(float64(x-selfX), float64(y-selfY)) > userEngageRadius {
        if time.Since(l.questWalkAt) < walkRequestPeriod {
            return nil
        }
        l.questWalkAt = time.Now()
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
// segments: one long segment (tens of thousands of units) exceeds the
// shipped expansion cap of one search, so the route splits into
// questSegmentLen hops along the straight line to the goal and every
// hop plans its own waypoint path. A hop whose search fails falls
// back to the raw direct click of the legacy walk (the server stops
// a walled click, the stuck detector re-plans). The walk stands on
// itself: it re-plans on a stuck segment until the timeout.
//
//nolint:gocognit // the quest segment branches read best side by side
func (l *Loop) walkQuestRoute(x int32, y int32, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for {
        // A seated character cannot walk: every click of the seated
        // answers ActionFailed. The stand runs first (a lost toggle
        // of the previous rest, a server side surprise).
        if l.tracker.SelfSitting() {
            l.logf("quest: the walk waits for the stand up")
            if err := l.ensureStanding(); err != nil {
                return err
            }
        }
        // The transit defense: an aggressive mob that targets the
        // character interrupts the walk - the passive transit of the
        // first live runs dragged a growing chaser tail through the
        // Ruins of Agony and died at the kill ground door (a stack
        // of five assisting skeletons hits harder than the potions
        // heal). The fight clears the chasers, then the walk replans.
        if err := l.fightTransitAttackers(); err != nil {
            return err
        }
        // The aggressive transit mobs grind the walking character
        // down: keep the health buffer full on the way.
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
        if time.Now().After(deadline) {
            return fmt.Errorf(
                "the route walk to (%d, %d) did not arrive", x, y)
        }
        segX, segY := questSegmentTarget(selfX, selfY, x, y)
        if l.followPlannedSegment(selfX, selfY, selfZ, segX, segY, deadline) {
            continue
        }
        // No geodata path for the segment (or the follower
        // ran dry): the direct click, the server stops it at
        // an obstacle and the loop re-plans above. The click
        // rides the same flood protector pacing as the planned
        // waypoints.
        now := time.Now()
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

// questSegmentTarget picks the segment destination: the goal itself
// inside the last stretch, the point questSegmentLen along the
// straight line otherwise.
func questSegmentTarget(selfX, selfY, x, y int32) (int32, int32) {
    dist := math.Hypot(float64(x-selfX), float64(y-selfY))
    if dist <= questSegmentLen {
        return x, y
    }
    frac := questSegmentLen / dist

    return int32(float64(selfX) + float64(x-selfX)*frac),
        int32(float64(selfY) + float64(y-selfY)*frac)
}

// followPlannedSegment plans the geodata path of one segment and
// walks its waypoints. It reports whether the plan existed and was
// followed (the caller falls back to the direct click otherwise).
func (l *Loop) followPlannedSegment(
    selfX, selfY, selfZ, segX, segY int32, deadline time.Time,
) bool {
    from := pathfind.Vec3{
        X: float64(selfX), Y: float64(selfY), Z: float64(selfZ),
    }
    dest := pathfind.Vec3{
        X: float64(segX), Y: float64(segY), Z: float64(selfZ),
    }
    result, err := l.navigator.FindPathApproachAvoiding(
        from, dest, questArriveRadius, l.frozenAreas)
    if err != nil || result == nil || len(result.Waypoints) == 0 {
        return false
    }
    // The partial round (docs/navmesh.md): a segment the search
    // cannot complete still walks its closest reachable waypoints -
    // the loop re-plans from wherever the corridor ends, so the
    // quest route keeps making ground instead of dropping to the
    // direct click at the first sealed segment.
    partial := !result.Found
    waypoints := result.Waypoints
    // The segment plan starts at the character's own cell resolved on
    // the pack: the standing z against the first waypoint's mesh z
    // measures the frame offset the segment's waypoint clicks ride
    // (see click_frame.go) - the quest route segments name the surface
    // the character stands on in the server frame, wherever the
    // pack vintages disagree.
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
    for i := range waypoints {
        if !l.followWaypoint(waypoints, i, frameOffset, deadline) {
            return true
        }
    }
    // The degenerate plan guard: the approach search may answer with
    // a waypoint inside its own approach radius of the character (a
    // segment short enough that the arrival check of the follower
    // closes at once) - the segment then "completes" without moving a
    // cell and the route loop above would spin the planner in a busy
    // loop (the 2026-09-13 retreat run burned nineteen thousand
    // plans a second for four minutes straight). A plan that moved
    // nothing reports false, the caller falls through to the direct
    // click.
    afterX, afterY, _, ok := l.tracker.SelfPosition()
    if ok && afterX == selfX && afterY == selfY {
        return false
    }

    return true
}

// followWaypoint walks toward the waypoint at the index until the
// tracker counts it reached (the tight pass radius of the town
// trips) or the walk stalls. False when the waypoint needs a re-plan
// (the follower returns, the caller plans a fresh segment); true
// when the waypoint was reached or the segment ran out of budget.
//
//nolint:cyclop,gocognit // the refusal and stuck branches read side by side
func (l *Loop) followWaypoint(
    waypoints []pathfind.Vec3, index int, frameOffset float64,
    deadline time.Time,
) bool {
    wp := waypoints[index]
    sent := false
    lastX, lastY := int32(0), int32(0)
    stuckSince := time.Time{}
    for {
        if time.Now().After(deadline) {
            return true
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
        if waypointDistance(wp, selfX, selfY, selfZ) <= radius {
            return true
        }
        moving := selfX != lastX || selfY != lastY
        if moving {
            lastX, lastY = selfX, selfY
            stuckSince = time.Time{}
        } else if stuckSince.IsZero() {
            stuckSince = time.Now()
        }
        // The potions ride the follower poll: one planned segment
        // runs over a minute and the chaser damage of that minute
        // undid the whole buffer of the first live runs (the walk
        // only drank between the segments, arriving at 39 percent).
        if err := l.drinkHealingPotionAt(questWalkPotionHP); err != nil {
            l.logf("quest: the potion drink failed: %v", err)

            return false
        }
        if !stuckSince.IsZero() && time.Since(stuckSince) >= questStuckWait {
            return false
        }
        // The pacing: no request while the character walks; a repeat
        // click only for the standing one, gated by the town walk
        // period (the Mobius PlayerActionFloodProtector mutes the
        // click stream above one action per second - the 2026-09-12
        // run stalled ten minutes on the mute).
        now := time.Now()
        if sent && (moving || now.Sub(l.questWalkAt) < walkRequestPeriod) {
            pace(questWalkPoll)

            continue
        }
        // The click carries the geodata height of the waypoint, not
        // the stale self height: the server validates the click z
        // against its own geodata (the same run stalled on the
        // segment west of Gludio - an 864 unit rise with every click
        // riding the self z, every request ActionFailed). The height
        // rides the server frame transport (see click_frame.go): the
        // mesh height plus the measured vintage shift of the
        // segment's standing surface, so the click names the walked
        // surface's layer in the server frame wherever the packs
        // disagree about absolute heights.
        clickZ := selfZ
        if wp.Z != 0 {
            clickZ = int32(anchorZToServerFrame(wp.Z, frameOffset))
        }
        if err := l.game.WalkTo(
            int32(wp.X), int32(wp.Y), clickZ); err != nil {
            l.logf("quest: the waypoint walk failed: %v", err)

            return false
        }
        sent = true
        l.questWalkAt = now
        pace(questWalkPoll)
    }
}

// awaitSegmentProgress waits until the character moves a meaningful
// distance from the position the direct click started at (the click
// took, the walk runs) or the deadline lapses. False marks a stalled
// click: the character stands where it stood.
func (l *Loop) awaitSegmentProgress(segX, segY int32, deadline time.Time) bool {
    startX, startY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    for {
        if time.Now().After(deadline) {
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
        pace(questWalkPoll)
    }
}

// questItemCount sums the journal counters of the quest item ids of
// a kill stage.
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

// DriveQuestChain runs one quest chain end to end: the accept
// conversation when the journal does not carry the quest yet, then
// the stage ladder by the journal cond until the exit talk drops
// the quest (the proof item survives in the inventory, the class
// change segment of the caller consumes it). A cond outside the ladder,
// a stage that misses its deadline or a health floor breach returns
// an error naming the stage; the journal drop ends the run clean.
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

// driveQuestStage runs one stage of the ladder: the optional
// gatekeeper segment first (the station sits in another town), then the
// talk half walks to the station and drives the dialog route, the
// kill half farms the item counters on the ground.
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

// driveQuestTransfer rides one gatekeeper hop: the walk to the
// teleporter, the talk that opens the first page (the html action
// cache only validates the links of the open page, so the showTeleports
// bypass needs the talk first), the showTeleports list, the
// destination button and the arrival wait at the teleport square.
// Live verified against Bella (Gludio -> Gludin) and Richlin
// (Gludin -> Gludio) on the deployed stack.
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
    l.logf("quest: talking to the gatekeeper %s (object %d), "+
        "riding the teleport to %s",
        transfer.Gatekeeper.Name, target.ObjectID, transfer.DestLabel)

    // The talk opens the first page (the walk-in gate of the
    // html action cache).
    if err := l.game.ClickObject(target.ObjectID); err != nil {
        return fmt.Errorf("the gatekeeper click: %w", err)
    }
    if _, html, err := l.awaitTransferPage(
        target.ObjectID, "", questTransferArriveWait); err != nil {
        return err
    } else if FindShowTeleportsButton(ParseGatekeeperHTML(html)) == nil {
        return fmt.Errorf(
            "the first page of %s carries no teleport entry",
            transfer.Gatekeeper.Name)
    }

    // The showTeleports bypass (the button of the open first
    // page) opens the list.
    if err := l.game.SendBypass(fmt.Sprintf(
        "npc_%d_showTeleports", target.ObjectID)); err != nil {
        return fmt.Errorf("the showTeleports send: %w", err)
    }
    buttons, _, err := l.awaitTransferPage(
        target.ObjectID, transfer.DestLabel, questTransferArriveWait)
    if err != nil {
        return err
    }
    teleport := FindTeleportButton(
        buttons, "NORMAL", transfer.DestLabel)
    if teleport == nil {
        return fmt.Errorf(
            "the teleport list of %s carries no %s button",
            transfer.Gatekeeper.Name, transfer.DestLabel)
    }
    command := fmt.Sprintf("npc_%d_teleport %s %d",
        target.ObjectID, teleport.ListName, teleport.LocID)
    l.logf("quest: teleporting to %s", teleport.Label)
    if err := l.game.SendBypass(command); err != nil {
        return fmt.Errorf("the teleport send: %w", err)
    }

    return l.awaitTransferArrival(ctx, transfer)
}

// awaitTransferPage waits for the dialog of the gatekeeper to carry
// the wanted button (a teleport button whose label contains
// destLabel; any page when destLabel is empty - the first page
// wait). It returns the parsed buttons of the matching page and the
// raw html.
func (l *Loop) awaitTransferPage(
    npcObjID int32, destLabel string, wait time.Duration,
) ([]BypassButton, string, error) {
    deadline := time.Now().Add(wait)
    for {
        id, html := l.game.LastHTMLDialog()
        if id == npcObjID && html != "" {
            buttons := ParseGatekeeperHTML(html)
            if destLabel == "" {
                if len(buttons) > 0 {
                    return buttons, html, nil
                }
            } else if FindTeleportButton(
                buttons, "NORMAL", destLabel) != nil {
                return buttons, html, nil
            }
        }
        if time.Now().After(deadline) {
            return nil, "", fmt.Errorf(
                "the gatekeeper dialog with %q never arrived",
                destLabel)
        }
        pace(gatekeeperPollPeriod)
    }
}

// awaitTransferArrival waits until the tracker places the character
// at the arrival square of the teleport (the TeleportToLocation
// answer of the server; the connection layer sends the Appearing
// confirmation).
func (l *Loop) awaitTransferArrival(
    ctx context.Context, transfer QuestTransfer,
) error {
    deadline := time.Now().Add(questTransferArriveWait)
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
        if time.Now().After(deadline) {
            return fmt.Errorf(
                "the teleport to %s never landed", transfer.DestLabel)
        }
        pace(questWalkPoll)
    }
}

// driveQuestTalk finds the station npc, walks into the interaction
// distance and drives the dialog route through the walker.
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

// questRetreatDistance is how far the retreat walks out of the mob
// radius before the sitting rest starts.
const questRetreatDistance = 2600.0

// retreatAndRest walks the tired hunter out of the quest mob radius,
// sits the health back to the stand threshold and returns to the kill
// ground. The retreat direction runs through the standing cell away
// from the nearest quest mob; the return ride is the ordinary segment
// walk of the ground.
func (l *Loop) retreatAndRest(ctx context.Context, stage QuestStage) error {
    _, _, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return errors.New("no self position for the retreat")
    }
    selfX, selfY, _, _ := l.tracker.SelfPosition()
    templates := make([]int32, 0, len(stage.Kill.Mobs))
    for _, mob := range stage.Kill.Mobs {
        templates = append(templates, mob+npcDisplayOffset)
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
    // The retreat point sits outside the aggro radius of the mob it
    // fled, but a chaser that already locked on walks those 2600
    // units in twenty seconds and beats the sitting hunter to death
    // (the 2026-09-13 run: a Tracker Skeleton Leader ground the rest
    // from 20 to 0 percent for three minutes). Nothing may target the
    // character when the sit lands.
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

// farmQuestStage runs a kill stage: the walk to the kill ground and
// the engage loop over the quest mobs until the journal counters
// fill the target. The attacks ride the double click semantics (the
// repeated request starts the fight); a ground with no living quest
// mob in the scan radius holds for the respawn instead of walking
// away (the stage deadline guards a dead ground). The rest between
// the fights parks the character sitting until the regeneration
// covers the next fight (the manual trip has no rest phase of the
// hunt loop).
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
        templates = append(templates, mob+npcDisplayOffset)
    }
    deadline := time.Now().Add(questKillStageTimeout)
    for {
        if err := ctx.Err(); err != nil {
            return err
        }
        if total := questItemCount(l.tracker,
            kill.ItemIDs); total >= kill.Target {
            l.logf("quest: the kill stage counters filled (%d)", total)

            return nil
        }
        if time.Now().After(deadline) {
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
        // The retreat policy: a tired hunter on the ground walks out
        // of the mob radius, sits the health back and returns - the
        // standing fight to the last drop loses to the assisting
        // clans (the live runs held the ground at 30 percent health
        // until the floor took them).
        if l.tracker.SelfHealthPercent() < questRestSitHP {
            if err := l.retreatAndRest(ctx, stage); err != nil {
                return err
            }

            continue
        }
        mob, ok := l.tracker.NearestNpcByTemplates(
            templates, questKillScanRadius)
        if !ok {
            // The ground is momentarily dead (a respawn window or a
            // cleared camp): resting and pacing keeps the loop alive
            // for the rescan - the old fallthrough attacked the
            // zero-value target and ground refused WalkTo(0, 0)
            // clicks against the flood protector until the respawn.
            if err := l.restBetweenFights(ctx); err != nil {
                return err
            }
            pace(questKillAttackPeriod)

            continue
        }
        // A quest mob stands in the scan radius: the tired
        // character drinks a healing potion instead of sitting
        // down (the Ruins of Agony skeletons are aggressive -
        // the first live run sat the character down inside the
        // camp and it died seated).
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
// confirmation of the transition: a sit/stand toggle the flood
// protector dropped leaves the character seated, and every later
// walk click of a seated character answers ActionFailed (the
// 2026-09-13 run: the rested hunter returned to the ground and
// ground the whole walk budget against the seated state).
func (l *Loop) ensureStanding() error {
    if !l.tracker.SelfSitting() {
        return nil
    }
    for range 3 {
        if err := l.game.ActionSitStand(); err != nil {
            return fmt.Errorf("the stand request: %w", err)
        }
        deadline := time.Now().Add(3 * time.Second)
        for time.Now().Before(deadline) {
            if !l.tracker.SelfSitting() {
                return nil
            }
            pace(gatekeeperPollPeriod)
        }
    }

    return errors.New("the character stays seated")
}

// restBetweenFights parks a tired character: below the sit threshold
// the character sits down and waits for the sitting regeneration to
// reach the stand threshold (the rest timeout stands up anyway - the
// health floor of the farm loop still guards the retreat). A healthy
// character returns at once.
func (l *Loop) restBetweenFights(ctx context.Context) error {
    if l.tracker.SelfHealthPercent() >= questRestSitHP {
        return nil
    }
    l.logf("quest: resting at %.0f%% health",
        l.tracker.SelfHealthPercent())
    if err := l.game.ActionSitStand(); err != nil {
        return fmt.Errorf("the sit request: %w", err)
    }
    deadline := time.Now().Add(questRestTimeout)
    for {
        if err := ctx.Err(); err != nil {
            _ = l.game.ActionSitStand()

            return fmt.Errorf("cancelled: %w", err)
        }
        // A fresh attacker aborts the rest: the sitting regeneration
        // loses to any chaser still swinging (stand up, the caller's
        // fight clears it, the next rest tries again).
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
        if time.Now().After(deadline) {
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

// questWalkPotionHP is the health share the route walk drinks at:
// the aggressive mobs along the approach segments (the Ruins of Agony
// clans assist each other) grind a passing character down, so the
// walk keeps the health buffer full instead of arriving half dead
// (the first live runs reached the kill ground at 26 percent and
// died in seconds).
const questWalkPotionHP = 80.0

// questPotionReuse is the reuse delay of the healing potions (the
// C1 item table pins 10 s on the Lesser Healing Potion): a use
// request inside the window is silently dropped by the server, so
// the engine paces its own sends.
const questPotionReuse = 10 * time.Second

// drinkHealingPotion restores the health mid fight: the Lesser
// Healing Potion of the inventory (when one is left) or the no-op
// for a healthy character.
func (l *Loop) drinkHealingPotion() error {
    return l.drinkHealingPotionAt(questRestSitHP)
}

// drinkHealingPotionAt drinks the first healing potion of the bag
// when the health sits below the threshold and the reuse window of
// the previous round has lapsed (a no-op above either gate or with
// the bag empty of potions).
func (l *Loop) drinkHealingPotionAt(threshold float64) error {
    if l.tracker.SelfHealthPercent() >= threshold {
        return nil
    }
    if !l.questPotionAt.IsZero() &&
        time.Since(l.questPotionAt) < questPotionReuse {
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
            l.questPotionAt = time.Now()
            l.awaitInventoryMutation(before, questEquipConfirmWait)

            return nil
        }
    }

    return nil
}

// EquipBaggedGear dresses the character from the bag: the blocking
// variant of the auto equipment for the manual quest trip loop. The
// loop plans the next paperdoll upgrade (the gear simulation of the
// MeleeFighter profile), sends the use request and waits for the
// inventory mutation to confirm; the cycle ends when the paperdoll
// holds no further upgrade or the action budget lapses. The scenario
// calls it right after the world entry of an injected character
// (the reset lands every stack in the bag).
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

// awaitInventoryMutation waits until the tracker reports a new
// inventory version (an InventoryUpdate the use request triggered).
func (l *Loop) awaitInventoryMutation(
    before uint64, wait time.Duration,
) bool {
    deadline := time.Now().Add(wait)
    for {
        if l.tracker.InventoryVersion() != before {
            return true
        }
        if time.Now().After(deadline) {
            return false
        }
        pace(gatekeeperPollPeriod)
    }
}
