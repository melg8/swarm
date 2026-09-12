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
// quest and the proof item lands. The class change leg (the Rains
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

// questWalkTimeout bounds one walk to a station or a kill ground.
const questWalkTimeout = 5 * time.Minute

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
		time.Sleep(questWalkPoll)
	}
}

// walkToQuestPoint walks to a station and waits for the arrival
// poll to close inside the arrival radius. The z of a kill ground
// is unknown to the chain data, so the caller passes the current
// self z there (the server walk corrects the height along the way).
func (l *Loop) walkToQuestPoint(
	x int32, y int32, z int32, timeout time.Duration,
) error {
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
		time.Sleep(questWalkPoll)
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
// change leg of the caller consumes it). A cond outside the ladder,
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

// driveQuestStage runs one stage of the ladder: the talk half walks
// to the station and drives the dialog route, the kill half farms
// the item counters on the ground.
func (l *Loop) driveQuestStage(
	ctx context.Context, chain QuestChain, stage QuestStage,
) error {
	if stage.TalkNpc.TemplateID == 0 {
		return l.farmQuestStage(ctx, stage)
	}
	route := append(QuestEntryLinks(chain), stage.Links...)
	l.logf("quest %d: the talk stage at %s",
		chain.QuestID, stage.TalkNpc.Name)

	return l.driveQuestTalk(chain, stage.TalkNpc, route)
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

// farmQuestStage runs a kill stage: the walk to the kill ground and
// the engage loop over the quest mobs until the journal counters
// fill the target. The attacks ride the double click semantics (the
// repeated request starts the fight); a ground with no living quest
// mob in the scan radius holds for the respawn instead of walking
// away (the stage deadline guards a dead ground).
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
		if total := questItemCount(l.tracker, kill.ItemIDs); total >= kill.Target {
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
		mob, ok := l.tracker.NearestNpcByTemplates(
			templates, questKillScanRadius)
		if !ok {
			time.Sleep(questWalkPoll)

			continue
		}
		if err := l.game.AttackTarget(mob.ObjectID); err != nil {
			return fmt.Errorf("the attack on %s: %w", mob.Name, err)
		}
		time.Sleep(questKillAttackPeriod)
	}
}
