// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestSelfAttackerCountsMobsOnUs pins the aggro load reading of the
// two-attacker emergency logout: every living attackable npc holding
// the character as its target counts, everything else does not.
func TestSelfAttackerCountsMobsOnUs(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    require.Equal(t, 0, bot.SelfAttackerCount(), "no mobs, no aggro")

    // Two gremlins swing at the character, a third chases someone
    // else, a passive villager targets us harmlessly.
    spawnNpcInfo(bot, 7, 1000001, 45300)
    spawnNpcInfo(bot, 8, 1000001, 45100)
    spawnNpcInfo(bot, 9, 1000001, 45200)
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 10, TemplateID: 1000001, Attackable: false,
        X: 45400, Y: 50000, Z: -3500,
    })
    for _, id := range []int32{7, 8} {
        bot.ApplyAttack(Attack{
            AttackerID: id, X: 45000 + id*100, Y: 50000, Z: -3500,
            TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
            TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        })
    }
    bot.ApplyAttack(Attack{
        AttackerID: 9, X: 45200, Y: 50000, Z: -3500,
        TargetCount: 1, TargetIDs: [AttackTargets]int32{55},
        TargetX: 45100, TargetY: 50000, TargetZ: -3500,
    })
    bot.ApplyAttack(Attack{
        AttackerID: 10, X: 45400, Y: 50000, Z: -3500,
        TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
    })

    require.Equal(t, 2, bot.SelfAttackerCount(),
        "two attackable mobs on us, the rest does not count")

    // A corpse stops counting the moment it drops.
    bot.ApplyStatusUpdate(7, []Attribute{
        {ID: AttrCurHP, Value: 0},
        {ID: AttrMaxHP, Value: 30},
    })
    require.Equal(t, 1, bot.SelfAttackerCount())
}

// TestCombatEventsFeedSwings pins the attack animation feed: every
// Attack broadcast lands as one swing event with the attacker and the
// hit target placement and a monotonic sequence.
func TestCombatEventsFeedSwings(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)

    // The mob swings at the character.
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 45300, Y: 50000, Z: -3500,
        TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
    })
    // The character swings back.
    bot.ApplyAttack(Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetCount: 1, TargetIDs: [AttackTargets]int32{7},
        TargetX: 45300, TargetY: 50000, TargetZ: -3500,
    })

    events := bot.Snapshot().CombatEvents
    require.Len(t, events, 2, "one swing event per attack broadcast")
    require.Equal(t, CombatEventAttack, events[0].Kind)
    require.Equal(t, int32(7), events[0].AttackerID)
    require.Equal(t, int32(100), events[0].TargetID)
    require.Equal(t, int32(45300), events[0].X)
    require.Equal(t, int32(45000), events[0].TargetX)
    require.Equal(t, CombatEventAttack, events[1].Kind)
    require.Equal(t, int32(100), events[1].AttackerID)
    require.Greater(t, events[1].Seq, events[0].Seq,
        "the sequence grows monotonically")
    require.NotZero(t, events[0].AtMs)

    // A swing without a target (TargetCount 0) feeds nothing.
    bot.ApplyAttack(Attack{AttackerID: 7, X: 45300, Y: 50000, Z: -3500})
    require.Len(t, bot.Snapshot().CombatEvents, 2)
}

// TestCombatEventsFeedDamage pins the damage animation feed: an HP
// drop records the exact lost amount on the hurt unit, a heal or a
// refresh records nothing.
func TestCombatEventsFeedDamage(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    spawnNpcInfo(bot, 7, 1000001, 45300)

    // The first vitals establish the baseline: no damage yet.
    bot.ApplyStatusUpdate(7, []Attribute{
        {ID: AttrMaxHP, Value: 100},
        {ID: AttrCurHP, Value: 100},
    })
    bot.ApplyStatusUpdate(100, []Attribute{
        {ID: AttrMaxHP, Value: 200},
        {ID: AttrCurHP, Value: 200},
    })
    require.Empty(t, bot.Snapshot().CombatEvents)

    // The character grinds the mob down, the mob bites back.
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 62}})
    bot.ApplyStatusUpdate(100, []Attribute{{ID: AttrCurHP, Value: 180}})

    events := bot.Snapshot().CombatEvents
    require.Len(t, events, 2)
    require.Equal(t, CombatEventDamage, events[0].Kind)
    require.Equal(t, int32(7), events[0].TargetID)
    require.InDelta(t, 38.0, events[0].Amount, 0.001)
    require.Equal(t, int32(45300), events[0].X)
    require.Equal(t, CombatEventDamage, events[1].Kind)
    require.Equal(t, int32(100), events[1].TargetID)
    require.InDelta(t, 20.0, events[1].Amount, 0.001)
    require.Equal(t, int32(45000), events[1].X)

    // A heal records nothing, an equal value neither.
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 80}})
    bot.ApplyStatusUpdate(100, []Attribute{{ID: AttrCurHP, Value: 180}})
    require.Len(t, bot.Snapshot().CombatEvents, 2)
}

// TestCombatEventsFeedBounded pins the feed cap: a burst of swings
// never grows the snapshot payload without end.
func TestCombatEventsFeedBounded(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    for i := range combatEventMax + 10 {
        bot.ApplyAttack(Attack{
            AttackerID: 7, X: 45300, Y: 50000, Z: -3500,
            TargetCount: 1, TargetIDs: [AttackTargets]int32{100},
            TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        })
        _ = i
    }

    require.Len(t, bot.Snapshot().CombatEvents, combatEventMax)
}

// TestApplyAttackRecordsMisses pins the miss float feed: the Attack
// packet marks every hit with the Mobius miss flag, and a dodged
// attack records a miss event instead of a swing - the viewer draws
// the miss float where the landed blow would draw the streak, so the
// fight stays legible when blows start missing. A dual hit draws one
// event per hit, each with its own kind.
func TestApplyAttackRecordsMisses(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetOnline("unittest1")
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 46000, Y: 50000, Name: "Gremlin",
    })

    // The mob swings at the character and misses: one miss event.
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{100},
        HitFlags:    [AttackTargets]int8{attackHitMissFlag},
        TargetCount: 1,
    })
    views := bot.Snapshot().CombatEvents
    require.Len(t, views, 1, "the missed swing records one event")
    require.Equal(t, CombatEventMiss, views[0].Kind)
    require.Equal(t, int32(7), views[0].AttackerID)
    require.Equal(t, int32(100), views[0].TargetID)
    require.Empty(t, countSwings(views),
        "the missed swing records no attack event")

    // The mob lands a blow: one swing event on the attacker.
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{100},
        TargetCount: 1,
    })
    views = bot.Snapshot().CombatEvents
    require.Equal(t, 1, countSwings(views))

    // A dual weapon lands both hits: two swing events, one per hit.
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{100, 100},
        TargetCount: 2,
    })
    views = bot.Snapshot().CombatEvents
    require.Equal(t, 3, countSwings(views))

    // A dual weapon lands one and misses one: one swing event and
    // one miss event, in the feed order. The miss of the opening
    // swing is still inside the feed window, so the miss count reads
    // incrementally.
    missesBefore := len(countMisses(bot.Snapshot().CombatEvents))
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{100, 100},
        HitFlags:    [AttackTargets]int8{0, attackHitMissFlag},
        TargetCount: 2,
    })
    views = bot.Snapshot().CombatEvents
    require.Equal(t, 4, countSwings(views))
    misses := countMisses(views)
    require.Len(t, misses, missesBefore+1,
        "the evaded hit of the pair records one miss event")
    newest := misses[len(misses)-1]
    require.Equal(t, CombatEventMiss, newest.Kind)
    require.Equal(t, int32(7), newest.AttackerID)
    require.Equal(t, int32(100), newest.TargetID)
}

// countSwings counts the attack events of the snapshot feed.
func countSwings(views []CombatEventView) int {
    count := 0
    for _, view := range views {
        if view.Kind == CombatEventAttack {
            count++
        }
    }

    return count
}

// countMisses collects the miss events of the snapshot feed.
func countMisses(views []CombatEventView) []CombatEventView {
    misses := make([]CombatEventView, 0, 2)
    for _, view := range views {
        if view.Kind == CombatEventMiss {
            misses = append(misses, view)
        }
    }

    return misses
}

// TestCritHintLabelsDamage pins the critical correlation: the Attack
// broadcast carries the Mobius HITFLAG_CRIT per hit, the StatusUpdate
// HP drops carry no crit flag, so the tracker remembers the victim of
// a landed critical blow and labels the next HP drop of that victim
// inside the window as a critical damage float.
func TestCritHintLabelsDamage(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    spawnNpcInfo(bot, 7, 1000001, 45300)
    bot.ApplyStatusUpdate(7, []Attribute{
        {ID: AttrMaxHP, Value: 100},
        {ID: AttrCurHP, Value: 100},
    })

    // The character lands a critical blow on the mob.
    bot.ApplyAttack(Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 45300, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{7},
        HitFlags:    [AttackTargets]int8{attackHitCritFlag},
        CritFlags:   [AttackTargets]bool{true},
        TargetCount: 1,
    })
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 62}})
    events := bot.Snapshot().CombatEvents
    require.Len(t, events, 2)
    require.Equal(t, CombatEventDamage, events[1].Kind)
    require.True(t, events[1].Crit,
        "the HP drop right after the critical swing labels crit")

    // A plain blow does not label: no crit hint was set.
    bot.ApplyAttack(Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 45300, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{7},
        TargetCount: 1,
    })
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 40}})
    events = bot.Snapshot().CombatEvents
    require.Len(t, events, 4)
    require.False(t, events[3].Crit,
        "the blow without the crit flag labels plain")

    // A missed blow sets no hint even with the crit bit set (the
    // server never sends that combination, the guard costs nothing).
    bot.ApplyAttack(Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 45300, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{7},
        HitFlags:    [AttackTargets]int8{attackHitMissFlag},
        CritFlags:   [AttackTargets]bool{true},
        TargetCount: 1,
    })
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 30}})
    events = bot.Snapshot().CombatEvents
    require.Len(t, events, 6)
    require.False(t, events[5].Crit,
        "the evaded blow labels no critical damage")

    // A stale hint (past the window) labels nothing.
    bot.ApplyAttack(Attack{
        AttackerID: 100, X: 45000, Y: 50000, Z: -3500,
        TargetX: 45300, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{7},
        HitFlags:    [AttackTargets]int8{attackHitCritFlag},
        CritFlags:   [AttackTargets]bool{true},
        TargetCount: 1,
    })
    time.Sleep(critHintWindow + 50*time.Millisecond)
    bot.ApplyStatusUpdate(7, []Attribute{{ID: AttrCurHP, Value: 20}})
    events = bot.Snapshot().CombatEvents
    require.Len(t, events, 8)
    require.False(t, events[7].Crit,
        "the HP drop past the window labels plain")
}

// TestCritHintLabelsSelfDamage pins the character side: the mob's
// critical swing labels the HP drop the character takes.
func TestCritHintLabelsSelfDamage(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplyStatusUpdate(100, []Attribute{
        {ID: AttrMaxHP, Value: 200},
        {ID: AttrCurHP, Value: 200},
    })
    bot.ApplyAttack(Attack{
        AttackerID: 7, X: 46000, Y: 50000, Z: -3500,
        TargetX: 45000, TargetY: 50000, TargetZ: -3500,
        TargetIDs:   [AttackTargets]int32{100},
        HitFlags:    [AttackTargets]int8{attackHitCritFlag},
        CritFlags:   [AttackTargets]bool{true},
        TargetCount: 1,
    })
    bot.ApplyStatusUpdate(100, []Attribute{{ID: AttrCurHP, Value: 160}})
    events := bot.Snapshot().CombatEvents
    require.Len(t, events, 2)
    require.Equal(t, CombatEventDamage, events[1].Kind)
    require.True(t, events[1].Crit,
        "the damage the character takes from the critical swing labels crit")
}
