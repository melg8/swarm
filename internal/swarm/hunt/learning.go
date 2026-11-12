// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The skill learning of the town trips: when the queue holds enough affordable
// lessons, the trip walks to the village teacher of the class (Ellenia the
// elven fighters, Greenis the elven mystics - a warrior never learns at the
// mage master), buys the demanded spellbooks from the town merchant and learns
// one lesson per request. The teacher interaction is the plain client click on
// the npc (Action 0x04): the NpcClick handler selects the teacher and remembers
// it as the last folk, which RequestAcquireSkill resolves its trainer through.

import (
    "math"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// Timing and threshold constants of the skill learning.
const (
    // learnTripMinSp is the minimum total SP of the affordable and unlocked
    // lessons that justifies a walk to the teacher: below it the trip would
    // spend minutes of hunting on a handful of lessons.
    learnTripMinSp = 300
    // learnPlanPeriod bounds the learning queue re-reads of the trip trigger:
    // the queue view allocates and the trigger runs every tick.
    learnPlanPeriod = 2 * time.Second
    // learnPause is the minimum spacing of two RequestAcquireSkill packets. The
    // server knows no flood protector on the packet (the handler checks only
    // the trainer distance, the level, the SP and the spellbook), so the
    // confirm round trip is the real pacing; the floor only keeps two sends
    // out of one decision tick.
    learnPause = 250 * time.Millisecond
    // learnConfirmWait bounds the SkillList bump wait that confirms a lesson.
    learnConfirmWait = 5 * time.Second
    // learnRetries bounds the re-requests of one lesson before it is skipped:
    // a refusal (the level, the SP, the book) answers silently.
    learnRetries = 3
    // teacherWaitTimeout bounds the wait for the teacher NpcInfo
    // before the stop gives the lessons up.
    teacherWaitTimeout = 45 * time.Second
    // teacherApproachWindow bounds the whole close approach of the teach stop:
    // the ring walk clicks the approach point until the character stands by the
    // teacher; past it the talk fires from the current spot or the stop skips.
    teacherApproachWindow = 45 * time.Second
)

// lessonTarget is one lesson the teacher stop plans to learn: the skill, the
// level being learned, its SP cost and the spellbook it consumes (0 - none).
type lessonTarget struct {
    skillID int32
    level   int32
    sp      int32
    book    int32
}

// townTeachers are the skill teachers of the deployment village per class: the
// class specific masters the learning trip walks to (the generated teacher
// dictionary joined with the village spawns); the ids carry the wire offset.
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
        l.tripStops[0].merchant.TemplateID + npcdata.NPCDisplayOffset,
    }
}

// refreshLearnPlan re-reads the learning queue view of the tracker when the
// plan period elapsed or the skills revision moved (every learned lesson
// rebuilds the queue). The cached queue drives the trip trigger (the affordable
// prefix) and the teacher stop (the lesson walk), both read every tick.
func (l *Loop) refreshLearnPlan() {
    now := l.now()
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

// learnableLessons collects the lessons the character may learn right now: the
// unlock level reached, the SP affordable and the demanded spellbook owned. The
// SP spent subtracts lesson by lesson, so a full affordable prefix answers.
func (l *Loop) learnableLessons() []lessonTarget {
    return l.walkLearnPrefix(true)
}

// walkLearnPrefix walks the affordable and unlocked lessons of the queue in its
// priority order. A lesson above the character level is skipped - it reserves
// no SP, so the unlocked lessons behind it stay reachable; the walk breaks at
// the first unlocked lesson the wallet cannot pay. The book lessons join when
// their spellbook is owned (ownBook - the teacher walk; the trigger and the
// book planning count them without the gate so their SP and purchase demand).
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

// learnTripWanted reports whether the learning queue justifies a town trip on
// its own: enough SP worth of unlocked and affordable lessons (a lesson that
// waits for its spellbook counts - the trip buys the book). The trigger runs on
// the cached queue, refreshed per period.
func (l *Loop) learnTripWanted() bool {
    l.refreshLearnPlan()
    if len(l.learnPlanCache) == 0 {
        return false
    }

    return spTotal(l.walkLearnPrefix(false)) >= learnTripMinSp
}

// planLearnStops appends the learning stops behind the gear stops (the sell
// phase calls it after planShoppingStops): the spellbook purchases of the
// learnable lessons from the merchant that sells them (merged into an existing
// gear stop of the same merchant, Creamees sells both jewels and books) and the
// teacher stop of the class master. The junk sales fund the books.
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
                // The gear stop of the book merchant absorbs the books: the
                // jewels and the spellbooks of the same trader leave together.
                stop.buys = append(stop.buys, books...)
                l.logger.Printf("Hunt: learn: buying %d spellbooks from %s"+
                    " with the gear stop", len(books), merchant.Name)
            } else {
                l.tripStops = append(l.tripStops, tripStop{
                    merchant: merchant,
                    buys:     books,
                    sell:     false,
                    teach:    false,
                    guide:    false,
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
        guide:    false,
    })
    l.logger.Printf("Hunt: learn: walking to the teacher %s for %d"+
        " lessons worth %d sp", teacher.Name, len(lessons),
        spTotal(lessons))
}

// merchantStop resolves the trip stop of the merchant when the trip carries
// one, so a later stop planning merges its buys instead of walking twice.
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

// bookPurchases plans the spellbook purchases of the lessons: every distinct
// demanded book the character does not carry yet, priced through the buylist
// product the town merchants sell (the has callback keeps the function pure).
// The returned purchases are plain gear purchases the buy stop executes.
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

// bookPurchase resolves one spellbook purchase through the town shop catalogs:
// the elven village catalog first (the near stop of the 1-19 band lessons), the
// Dion catalog as the fallback (Sonia sells the whole first profession band,
// the village list only Charm and Poison Recovery). The price carries the tax.
func bookPurchase(itemID int32) (gear.Purchase, bool) {
    for _, catalog := range []gear.Catalog{townShopCatalog, dionShopCatalog} {
        for _, shop := range catalog.Shops {
            for _, listID := range shop.Lists {
                for _, product := range npcdata.ItemsOfBuyList(listID) {
                    if product != itemID {
                        continue
                    }
                    price := npcdata.ItemPrice(itemID)
                    price += price * int64(shopTaxPercent(shop)) / 100

                    //nolint:exhaustruct_v5 // gear fields stay zero
                    return gear.Purchase{
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
    }

    return gear.Purchase{}, false //nolint:exhaustruct_v5 // not-found
}

// shopTaxPercent reads the buy tax of a catalog shop back as whole percent for
// the integer price math (the catalog builders carry the exact rates).
func shopTaxPercent(shop gear.Shop) int32 {
    return int32(math.Round(float64(shop.TaxRate) * 100))
}

// bookReason marks the spellbook purchases in the trip stop buys.
const bookReason = "spellbook"

// teachStop reports whether the current trip stop teaches the
// lessons.
func (l *Loop) teachStop() bool {
    return len(l.tripStops) > 0 && l.tripStops[0].teach
}

// handleTeacher approaches the teacher npc and clicks it like the official
// client: the click selects the teacher and the server remembers it as the last
// folk, which RequestAcquireSkill resolves its trainer through. It reports
// false while the character still walks, true when the teaching may start.
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

// approachTeacher walks the character right up to the teacher npc and reports
// when the lesson requests may fire: the npc approach point ring before the
// talk click, never the teacher's exact cell (the Bresenham line of the server
// can step over onto a roof layer there). The teacherWalkUntil window bounds
// the walk and answers its two dead ends: a ring the server routing refuses
// (the talk fires from wherever the character stands inside the 250 unit
// INTERACTION_DISTANCE) and a teacher on another deck (the stop skips).
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
    // The window arms once per teach stop and bounds the whole approach.
    if l.teacherWalkUntil.IsZero() {
        l.teacherWalkUntil = now.Add(teacherApproachWindow)
    }
    // The close ring: the character stands right by the teacher, so the
    // talk click lands from the ring.
    if dist2D <= npcApproachOffset+hopCoincideDist {
        if dist3D <= npcInteractionDist {
            l.clickTeacher(now)

            return true
        }
        // The teacher stands on another deck and no ring walk closes the z
        // gap: the paced talk clicks keep firing while the window runs, and
        // the attack analog pull of the already selected teacher hands the
        // close walk to the server. The expiry path talks or skips.
        if l.teacherDeckUntil.IsZero() {
            l.teacherDeckUntil = now.Add(merchantDeckWindow)
            l.logger.Printf("Hunt: learn: the teacher stands on "+
                "another deck (z %d vs %d), the pull walks the "+
                "straight line", z, selfZ)
        }
        l.clickTeacher(now)

        return false
    }
    if now.After(l.teacherWalkUntil) {
        // The window burned without reaching the ring: the talk fires from
        // wherever the character stands when the 3D distance fits the server
        // interaction gate (a z gap must not hold the gate forever), else the
        // lessons are skipped and the trip continues.
        if dist3D <= npcInteractionDist {
            l.clickTeacher(now)

            return true
        }
        l.teacherID = -1
        l.logger.Printf("Hunt: learn: the teacher stays out of " +
            "reach, skipping the lessons")

        return true
    }
    // The ring walk: click the npc approach point until the character stands
    // on the ring. The point converges with the character, so the chained
    // clicks close the last stretch through the entrance to the npc.
    ax, ay, az := npcApproachPoint(x, y, z, selfX, selfY)
    approachDist2D := math.Hypot(
        float64(ax-selfX), float64(ay-selfY))
    if approachDist2D > hopCoincideDist {
        l.walkToward(ax, ay, az, now)
    }

    return false
}

// clickTeacher sends the paced talk click that selects the teacher and
// refreshes the server last-folk memory. When the teacher is already the
// selected target the click is the attack analog pull: the second plain click
// takes the interact branch and the server walks the character straight in.
func (l *Loop) clickTeacher(now time.Time) {
    if now.Sub(l.teacherPick) >= selectPeriod {
        l.teacherPick = now
        var err error
        if l.tracker.SelfTargetID() == l.teacherID {
            err = l.game.InteractPull(l.teacherID)
        } else {
            err = l.game.ClickObject(l.teacherID)
        }
        if err != nil {
            l.logger.Printf("Hunt: learn: teacher click failed: %v", err)
        }
    }
}

// tickTeacherLessons learns the queued lessons one request at a time: the next
// request fires the moment the previous lesson's SkillList answer lands, the
// confirm round trip being the only real rate limit. A lesson that never
// confirms is re-requested up to the retry budget and then skipped. A send held
// back by the pacing arms nothing, so the confirm window never burns on a
// packet that never went out. The lesson list re-reads the live queue every
// call and reports true when nothing learnable is left and the stop advances.
func (l *Loop) tickTeacherLessons(now time.Time) bool {
    l.refreshLearnPlan()
    lessons := l.learnableLessons()
    if l.learnRequested == nil {
        if len(lessons) == 0 {
            return true
        }
        head := lessons[0]
        if !l.sendLearnRequest(head) {
            return false
        }
        l.learnRequested = &head
        l.learnConfirmAt = now
        l.learnRevision = l.tracker.SkillsRevision()

        return false
    }
    if l.tracker.SkillsRevision() != l.learnRevision {
        // The SkillList answer of the learned lesson: the queue head moved,
        // so the next request fires within this same tick.
        l.logger.Printf("Hunt: learn: learned %s level %d for %d sp",
            skillDisplayName(l.learnRequested.skillID),
            l.learnRequested.level, l.learnRequested.sp)
        l.learnRequested = nil
        l.learnRetries = 0

        return l.tickTeacherLessons(now)
    }
    if now.Sub(l.learnConfirmAt) < learnConfirmWait {
        return false
    }
    if l.learnRetries >= learnRetries {
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
        l.learnRequested.level, l.learnRetries+1, learnRetries)
    if !l.sendLearnRequest(*l.learnRequested) {
        // The pacing pause holds the re-request: only real sends count.
        return false
    }
    l.learnRetries++
    l.learnConfirmAt = now

    return false
}

// sendLearnRequest sends one paced lesson request and reports whether it went
// out: the pacing pause and a transport error both report false, so the caller
// retries on a later tick and the confirm machinery stays unarmed.
func (l *Loop) sendLearnRequest(lesson lessonTarget) bool {
    if !l.learnAt.IsZero() && l.now().Sub(l.learnAt) < learnPause {
        return false
    }
    l.learnAt = l.now()
    if err := l.game.AcquireSkill(lesson.skillID, lesson.level); err != nil {
        l.logger.Printf("Hunt: learn: acquire request failed: %v", err)

        return false
    }

    return true
}

// skillDisplayName resolves the display name of a skill id for the logs.
func skillDisplayName(skillID int32) string {
    if info, ok := npcdata.SkillInfoOf(skillID); ok {
        return info.Name
    }

    return "skill"
}

// resetLearnState drops the teacher stop state: a fresh trip re-finds its
// teacher, an aborted trip leaves no half-learned lesson behind.
func (l *Loop) resetLearnState() {
    l.teacherID = 0
    l.teacherPick = time.Time{}
    l.teacherDeckUntil = time.Time{}
    l.teacherWalkUntil = time.Time{}
    l.learnRequested = nil
    l.learnConfirmAt = time.Time{}
    l.learnRetries = 0
    l.learnAt = time.Time{}
}
