// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The skill learning of the town trips: when the learning queue holds
// enough affordable lessons, the trip walks to the village teacher of
// the class (Ellenia teaches the elven fighters, Greenis the elven
// mystics - the teachers are class specific, a warrior never learns
// at the mage master), buys the spellbooks the lessons demand from
// the town merchant first and then learns one lesson per request at
// the teacher. The teacher interaction is the plain client click on
// the npc (Action 0x04): the NpcClick handler of the server selects
// the teacher AND remembers it as the last folk the character talked
// to - exactly what RequestAcquireSkill resolves its trainer through.

import (
	"math"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// Timing and threshold constants of the skill learning.
const (
	// learnTripMinSp is the minimum total SP of the affordable and
	// unlocked lessons that justifies a walk to the teacher: below it
	// the trip would spend minutes of hunting on a handful of lessons.
	learnTripMinSp = 300
	// learnPlanPeriod bounds the learning queue re-reads of the trip
	// trigger: the queue view allocates, the trigger runs every tick.
	learnPlanPeriod = 2 * time.Second
	// learnPause paces the lesson requests: the server has no flood
	// protector on RequestAcquireSkill, but every learn answers with
	// a SkillList and a UserInfo, and a human learner clicks one
	// lesson at a time.
	learnPause = 1 * time.Second
	// learnConfirmWait bounds the wait for the SkillList bump that
	// confirms a learned lesson.
	learnConfirmWait = 5 * time.Second
	// learnRetries bounds the re-requests of one lesson before it is
	// skipped: a refusal (the level not reached, the SP short, the
	// book missing) answers silently.
	learnRetries = 3
	// teacherWaitTimeout bounds the wait for the teacher NpcInfo
	// before the stop gives the lessons up.
	teacherWaitTimeout = 45 * time.Second
)

// lessonTarget is one lesson the teacher stop plans to learn: the
// skill, the level being learned, its SP cost and the spellbook it
// consumes (0 - none).
type lessonTarget struct {
	skillID int32
	level   int32
	sp      int32
	book    int32
}

// townTeachers are the skill teachers of the deployment village per
// class: the class specific masters the learning trip walks to (the
// generated teacher dictionary joined with the village spawns). The
// display ids carry the 1000000 wire offset like the merchants.
var townTeachers = teacherCatalog()

// teacherCatalog resolves the village teachers of every known class
// into town npcs.
func teacherCatalog() map[int32][]townNpc {
	catalog := make(map[int32][]townNpc)
	for classID, teachers := range npcdata.AllClassTeachers() {
		entries := make([]townNpc, 0, len(teachers))
		for _, teacher := range teachers {
			entries = append(entries, townNpc{
				TemplateID: teacher.TemplateID,
				Name:       teacher.Name,
				X:          teacher.X,
				Y:          teacher.Y,
				Z:          teacher.Z,
			})
		}
		catalog[classID] = entries
	}

	return catalog
}

// nearestTeacher returns the village teacher of the character class
// closest to the character.
func (l *Loop) nearestTeacher() (townNpc, bool) {
	teachers := townTeachers[l.tracker.SelfClassID()]
	if len(teachers) == 0 {
		return zeroTownNpc, false
	}
	selfX, selfY, _, ok := l.tracker.SelfPosition()
	if !ok {
		return teachers[0], true
	}
	best := teachers[0]
	bestDist := math.MaxFloat64
	for _, teacher := range teachers {
		dist := math.Hypot(
			float64(teacher.X-selfX), float64(teacher.Y-selfY))
		if dist < bestDist {
			bestDist = dist
			best = teacher
		}
	}

	return best, true
}

// teacherTemplates lists the packet template ids of the teacher the
// teacher stop walks to (the display id offset of the wire).
func (l *Loop) teacherTemplates() []int32 {
	if len(l.tripStops) == 0 {
		return nil
	}

	return []int32{
		l.tripStops[0].merchant.TemplateID + npcDisplayOffset,
	}
}

// refreshLearnPlan re-reads the learning queue view of the tracker
// when the plan period elapsed OR the skills revision moved (every
// learned lesson rebuilds the queue - a stale cache would re-request
// the learned lesson). The cached queue drives the trip trigger (the
// affordable prefix) and the teacher stop (the lesson walk) - both
// read it every tick, the view allocates.
func (l *Loop) refreshLearnPlan() {
	now := time.Now()
	revision := l.tracker.SkillsRevision()
	if !l.learnPlanAt.IsZero() &&
		now.Sub(l.learnPlanAt) < learnPlanPeriod &&
		revision == l.learnPlanRevision {
		return
	}
	l.learnPlanCache = nil
	if plan := l.tracker.SkillPlan(); plan != nil {
		l.learnPlanCache = plan.Entries
	}
	l.learnPlanAt = now
	l.learnPlanRevision = revision
}

// learnableLessons walks the cached learning queue and collects the
// lessons the character may learn right now: the unlock level
// reached, the SP affordable and the demanded spellbook owned (or
// not needed). The walk stops at the first lesson the wallet cannot
// pay - the queue is priority ordered, the lessons behind it wait
// for the SP whatever their own budget says. The SP spent subtracts
// lesson by lesson, so a full prefix answers.
func (l *Loop) learnableLessons() []lessonTarget {
	return l.walkLearnPrefix(true)
}

// walkLearnPrefix walks the affordable and unlocked lessons of the
// cached learning queue in its priority order. A lesson above the
// character level is skipped - it cannot be learned this trip, so it
// reserves no SP and the unlocked lessons behind it stay reachable
// (the attack lessons of the next level window never fence out the
// defense lesson of the current one); the walk breaks at the first
// unlocked lesson the wallet cannot pay - the priority order spends
// the SP on the head lessons first and the tail waits for it. The
// book lessons join when their spellbook is owned (ownBook true -
// the teacher walk; the trigger and the book planning walk the
// prefix without the gate so a lesson that waits for its spellbook
// counts its SP and demands its purchase).
func (l *Loop) walkLearnPrefix(ownBook bool) []lessonTarget {
	if len(l.learnPlanCache) == 0 {
		return nil
	}
	level := l.tracker.SelfLevel()
	sp := int64(l.tracker.SelfSp())
	lessons := make([]lessonTarget, 0, len(l.learnPlanCache))
	for _, entry := range l.learnPlanCache {
		if entry.ReqLevel > level {
			continue
		}
		if int64(entry.SpCost) > sp {
			break
		}
		if entry.BookItemID != 0 && ownBook &&
			!l.tracker.InventoryHasItem(entry.BookItemID) {
			// The book lesson waits for its spellbook: the book stop
			// buys it first, the lesson stays queued.
			continue
		}
		lessons = append(lessons, lessonTarget{
			skillID: entry.SkillID,
			level:   entry.Level,
			sp:      entry.SpCost,
			book:    entry.BookItemID,
		})
		sp -= int64(entry.SpCost)
	}

	return lessons
}

// learnTripWanted reports whether the learning queue justifies a
// town trip on its own: enough SP worth of lessons unlocked and
// affordable (a lesson that waits for its spellbook counts - the
// trip buys the book). The trigger runs on the cached queue,
// refreshed per period.
func (l *Loop) learnTripWanted() bool {
	l.refreshLearnPlan()
	if len(l.learnPlanCache) == 0 {
		return false
	}

	return spTotal(l.walkLearnPrefix(false)) >= learnTripMinSp
}

// planLearnStops appends the learning stops of the running trip: the
// spellbook purchases of the learnable lessons from the town merchant
// that sells them and the teacher stop of the class master. The stops
// close the trip BEHIND the gear stops (the sell phase calls this
// after planShoppingStops): the books merge into the gear stop of
// their merchant when they match (the jewel trader Creamees sells both
// the basic jewels and the spellbooks - one visit buys them all), the
// teacher walk follows, so one town visit buys the weapon, the armor,
// the jewels, the books and teaches the lessons. The fresh adena of
// the junk sales funds the books.
func (l *Loop) planLearnStops() {
	l.refreshLearnPlan()
	lessons := l.walkLearnPrefix(false)
	if len(lessons) == 0 {
		return
	}
	teacher, ok := l.nearestTeacher()
	if !ok {
		l.logger.Printf("Hunt: learn: no village teacher for class %d,"+
			" skipping the lessons", l.tracker.SelfClassID())

		return
	}
	books := bookPurchases(lessons, l.tracker.InventoryHasItem)
	if len(books) > 0 {
		merchant, found := merchantByTemplate(books[0].MerchantTemplateID)
		if found {
			if stop := l.merchantStop(merchant.TemplateID); stop != nil {
				// The gear stop of the book merchant absorbs the books:
				// the jewels and the spellbooks of the same trader leave
				// in one visit.
				stop.buys = append(stop.buys, books...)
				l.logger.Printf("Hunt: learn: buying %d spellbooks from %s"+
					" with the gear stop", len(books), merchant.Name)
			} else {
				l.tripStops = append(l.tripStops, tripStop{
					merchant: merchant,
					buys:     books,
					sell:     false,
					teach:    false,
				})
				l.logger.Printf("Hunt: learn: buying %d spellbooks from %s",
					len(books), merchant.Name)
			}
		} else {
			l.logger.Printf("Hunt: learn: no known merchant for the %d"+
				" spellbooks, learning without them", len(books))
		}
	}
	l.tripStops = append(l.tripStops, tripStop{
		merchant: teacher,
		buys:     nil,
		sell:     false,
		teach:    true,
	})
	l.logger.Printf("Hunt: learn: walking to the teacher %s for %d"+
		" lessons worth %d sp", teacher.Name, len(lessons),
		spTotal(lessons))
}

// merchantStop resolves the trip stop of the merchant when the trip
// carries one, so a later stop planning merges its buys into it
// instead of walking to the same npc twice.
func (l *Loop) merchantStop(templateID int32) *tripStop {
	for index := range l.tripStops {
		if l.tripStops[index].merchant.TemplateID == templateID {
			return &l.tripStops[index]
		}
	}

	return nil
}

// spTotal sums the SP cost of the lessons.
func spTotal(lessons []lessonTarget) int64 {
	var total int64
	for _, lesson := range lessons {
		total += int64(lesson.sp)
	}

	return total
}

// bookPurchases plans the spellbook purchases of the lessons: every
// distinct demanded book the character does not carry yet (the
// inventory check runs through the has callback - the state accessor
// keeps the function pure for the tests), priced through the buylist
// product the town merchants sell. The returned purchases are plain
// gear purchases - the buy stop machinery executes them unchanged.
func bookPurchases(
	lessons []lessonTarget, has func(itemID int32) bool,
) []gear.Purchase {
	missing := make(map[int32]bool)
	for _, lesson := range lessons {
		if lesson.book != 0 && !has(lesson.book) {
			missing[lesson.book] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	purchases := make([]gear.Purchase, 0, len(missing))
	for itemID := range missing {
		purchase, ok := bookPurchase(itemID)
		if !ok {
			continue
		}
		purchases = append(purchases, purchase)
	}

	return purchases
}

// bookPurchase resolves one spellbook purchase through the town shop
// catalog: the merchant buylist that sells the item and its price
// (the reference price with the town tax of 15 percent).
func bookPurchase(itemID int32) (gear.Purchase, bool) {
	for _, shop := range townShopCatalog.Shops {
		for _, listID := range shop.Lists {
			for _, product := range npcdata.ItemsOfBuyList(listID) {
				if product != itemID {
					continue
				}
				price := npcdata.ItemPrice(itemID)
				price += price * 15 / 100

				return gear.Purchase{ //nolint:exhaustruct_v5 // gear fields stay zero
					ItemID:             itemID,
					ListID:             listID,
					MerchantTemplateID: shop.MerchantTemplateID,
					Count:              1,
					Price:              price,
					Reason:             bookReason,
				}, true
			}
		}
	}

	return gear.Purchase{}, false //nolint:exhaustruct_v5 // not-found
}

// bookReason marks the spellbook purchases in the trip stop buys.
const bookReason = "spellbook"

// teachStop reports whether the current trip stop teaches the
// lessons.
func (l *Loop) teachStop() bool {
	return len(l.tripStops) > 0 && l.tripStops[0].teach
}

// handleTeacher approaches the teacher npc and clicks it like the
// official client talks to an npc: the click selects the teacher and
// the server remembers it as the last folk - the RequestAcquireSkill
// lesson requests resolve their trainer through it. It reports false
// while the character still walks toward the teacher or waits for
// one to appear, true when the teaching may start (the click went
// out or the teacher never showed up and the stop skips).
func (l *Loop) handleTeacher(now time.Time) bool {
	if l.teacherID < 0 {
		return true
	}
	if l.teacherID > 0 {
		return l.approachTeacher(now)
	}
	if now.Sub(l.teacherPick) < selectPeriod {
		return false
	}
	l.teacherPick = now
	teacher, ok := l.tracker.NearestNpcByTemplates(
		l.teacherTemplates(), merchantFindRadius)
	if ok {
		l.teacherID = teacher.ObjectID
		l.logger.Printf("Hunt: learn: teacher %s found, walking to it",
			teacher.Name)

		return false
	}
	if now.Sub(l.sellPhaseAt) < teacherWaitTimeout {
		return false
	}
	l.teacherID = -1
	l.logger.Printf("Hunt: learn: the teacher never showed up, " +
		"skipping the lessons")

	return true
}

// approachTeacher walks to the teacher, clicks it within the
// interaction distance and reports when the lesson requests may
// fire. The distance gate is 3D like the merchant approach (the
// server INTERACTION_DISTANCE of 250 checks x, y and z together); a
// teacher standing on another deck of the geodata is walked to by
// server routing (the ground clicks route through the server
// pathfinder that knows the ramps), bounded by the deck window.
//
// The approach walk clicks the ground at the npc approach point, not
// at the teacher's exact cell: the server's getValidLocation walks a
// Bresenham line that can "step over" onto a roof layer when the
// click targets an interior cell (the 2026-09-11 roof teleport
// report: the bot clicked Cobendell's spawn point inside the trainer
// hall, the line crossed the south wall and the height-step fallback
// resolved the target onto the roof). The offset keeps the click
// line on the surrounding deck, within the interaction distance but
// outside the walled interior.
//
// The talk click fires as soon as the bot is within the server
// interaction distance (npcInteractionDist = 250 in 3D), even when
// the z gap keeps the dist3D above the approach gate (200). The
// 2026-09-11 05:45 dump showed test1 stuck at 44616 52536 -2832
// (dist 244 from Cobendell at z -2792, dz 40): the far-walk branch
// clicked the offset point, the bot walked there, but the z gap
// kept dist3D above 200 forever and the talk click never fired. The
// 250 gate matches the server rule and lets the talk click land from
// the offset ring.
func (l *Loop) approachTeacher(now time.Time) bool {
	x, y, z, ok := l.tracker.ObjectPosition(l.teacherID)
	if !ok {
		l.teacherID = -1

		return true
	}
	selfX, selfY, selfZ, _ := l.tracker.SelfPosition()
	dist2D := math.Hypot(
		float64(x-selfX), float64(y-selfY))
	dz := float64(z - selfZ)
	dist3D := math.Sqrt(dist2D*dist2D + dz*dz)
	// The talk click fires within the server interaction distance
	// (250 in 3D) even when the approach gate (200) is not met: the
	// offset ring lands the bot at ~150 units 2D from the npc, and a
	// small z gap (the trainer hall floor is 40 units above the
	// approach deck) keeps dist3D at ~155 - well within the server
	// 250 gate, but above the 200 approach gate. The 2026-09-11 05:45
	// dump looped forever because the talk click waited for dist3D <=
	// 200 while the bot stood on the offset ring at dist3D 244.
	if dist3D <= npcInteractionDist {
		l.clickTeacher(now)

		return true
	}
	if dist3D > merchantApproachDist {
		ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
		approachDist2D := math.Hypot(
			float64(ax-selfX), float64(ay-selfY))
		if dist2D <= merchantApproachDist {
			// The geodata pack misses the trainer platform ramps: the
			// 2D distance is met, the z is not. The approach point
			// collapses onto the bot's own cell - the click is a no-op
			// the server collapses, the deck window bounds the wait
			// before the teacher is given up. Clicking the teacher's
			// exact cell here teleported the bot onto the roof (the
			// 2026-09-11 report), so the offset keeps the click safe
			// even when it cannot help.
			if l.teacherDeckUntil.IsZero() {
				l.teacherDeckUntil = now.Add(merchantDeckWindow)
				l.logger.Printf("Hunt: learn: the teacher stands on "+
					"another deck (z %d vs %d), re-walking by server "+
					"routing", selfZ, z)
			}
			if now.Before(l.teacherDeckUntil) {
				if approachDist2D > hopCoincideDist {
					l.walkToward(ax, ay, az, now)
				}

				return false
			}
			l.teacherID = -1
			l.logger.Printf("Hunt: learn: the teacher stays out of " +
				"reach, skipping the lessons")

			return true
		}
		// The far walk: the bot is beyond the 2D approach gate. Click
		// the offset point until the bot arrives at the offset ring,
		// then the dist3D <= npcInteractionDist early return above
		// takes over (the talk click lands from the ring even with a
		// small z gap). Without that early return the bot looped on
		// the offset ring forever (the 2026-09-11 05:45 dump).
		if approachDist2D > hopCoincideDist {
			l.walkToward(ax, ay, az, now)
		}

		return false
	}
	// dist3D in (merchantApproachDist, npcInteractionDist]: the bot is
	// on the offset ring, the talk click lands.
	l.clickTeacher(now)

	return true
}

// clickTeacher sends the paced talk click that selects the teacher and
// refreshes the server's last-folk memory (the RequestAcquireSkill
// lesson requests resolve their trainer through it).
func (l *Loop) clickTeacher(now time.Time) {
	if now.Sub(l.teacherPick) >= selectPeriod {
		l.teacherPick = now
		if err := l.game.ClickObject(l.teacherID); err != nil {
			l.logger.Printf("Hunt: learn: teacher click failed: %v", err)
		}
	}
}

// tickTeacherLessons learns the queued lessons one request at a
// time: every request waits for the SkillList bump of its answer
// (the server lists the whole learned set after every learn), a
// lesson that never confirms is re-requested up to the retry budget
// and then skipped (the refusals answer silently). The lesson list
// re-reads the live queue every call - the SP drop of each learned
// lesson drops the tail behind it out of the budget. It reports
// true when nothing learnable is left and the stop may advance.
func (l *Loop) tickTeacherLessons(now time.Time) bool {
	l.refreshLearnPlan()
	lessons := l.learnableLessons()
	if l.learnRequested == nil {
		if len(lessons) == 0 {
			return true
		}
		head := lessons[0]
		l.learnRequested = &head
		l.learnConfirmAt = now
		l.learnRevision = l.tracker.SkillsRevision()
		l.sendLearnRequest(head)

		return false
	}
	if l.tracker.SkillsRevision() != l.learnRevision {
		// The SkillList answer of the learned lesson: the queue head
		// moved, the next lesson waits out the pacing pause.
		l.logger.Printf("Hunt: learn: learned %s level %d for %d sp",
			skillDisplayName(l.learnRequested.skillID),
			l.learnRequested.level, l.learnRequested.sp)
		l.learnRequested = nil
		l.learnConfirmAt = now
		l.learnRetries = 0

		return false
	}
	if now.Sub(l.learnConfirmAt) < learnConfirmWait {
		return false
	}
	l.learnRetries++
	if l.learnRetries > learnRetries {
		l.logger.Printf("Hunt: learn: %s level %d never confirmed,"+
			" skipping it",
			skillDisplayName(l.learnRequested.skillID),
			l.learnRequested.level)
		l.learnRequested = nil
		l.learnConfirmAt = now
		l.learnRetries = 0

		return false
	}
	l.logger.Printf("Hunt: learn: re-requesting %s level %d (try %d"+
		" of %d)", skillDisplayName(l.learnRequested.skillID),
		l.learnRequested.level, l.learnRetries, learnRetries)
	l.sendLearnRequest(*l.learnRequested)
	l.learnConfirmAt = now

	return false
}

// sendLearnRequest sends one paced lesson request.
func (l *Loop) sendLearnRequest(lesson lessonTarget) {
	if !l.learnAt.IsZero() && time.Since(l.learnAt) < learnPause {
		return
	}
	l.learnAt = time.Now()
	if err := l.game.AcquireSkill(lesson.skillID, lesson.level); err != nil {
		l.logger.Printf("Hunt: learn: acquire request failed: %v", err)
	}
}

// skillDisplayName resolves the display name of a skill id for the
// logs (the generated dictionary).
func skillDisplayName(skillID int32) string {
	if info, ok := npcdata.SkillInfoOf(skillID); ok {
		return info.Name
	}

	return "skill"
}

// resetLearnState drops the teacher stop state: a fresh trip
// re-finds its teacher, an aborted trip leaves no half-learned
// lesson behind.
func (l *Loop) resetLearnState() {
	l.teacherID = 0
	l.teacherPick = time.Time{}
	l.teacherDeckUntil = time.Time{}
	l.learnRequested = nil
	l.learnConfirmAt = time.Time{}
	l.learnRetries = 0
	l.learnAt = time.Time{}
}
