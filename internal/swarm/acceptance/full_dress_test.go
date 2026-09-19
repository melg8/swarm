// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The full dress scenario tests: the world entry burst round of the
// auto equipment - the character wakes with the complete outfit in
// the bag and nothing worn, and the burst planner must dress the
// whole set within the world entry window.

// newFullDressBot builds an online tracker with an empty paperdoll.
func newFullDressBot() *state.Bot {
    bot := state.NewBot(dressAccount)
    bot.SetCharacter(dressAccount, 100, 18, elvenSpawnX,
        elvenSpawnY, elvenSpawnZ, level15HP, level15MP)
    bot.SetOnline(dressAccount)
    bot.ApplyUserInfo(state.UserInfo{
        Name:    dressAccount,
        Level:   15,
        ClassID: 18,
        X:       elvenSpawnX,
        Y:       elvenSpawnY,
        Z:       elvenSpawnZ,
        MaxHP:   level15HP,
        CurHP:   level15HP,
        MaxMP:   level15MP,
        CurMP:   level15MP,
    })

    return bot
}

// TestFullDressDefinition pins the scenario registration: the burst
// round owns temp11, runs under the five minute bound and carries the
// burst story.
func TestFullDressDefinition(t *testing.T) {
    var found bool
    for _, def := range Definitions() {
        if def.ID != "full-dress" {
            continue
        }
        found = true
        require.Equal(t, dressAccount, def.Account)
        require.Equal(t, fullDressTimeout, def.Timeout)
        require.NotNil(t, def.Scenario)
        for _, needle := range []string{
            "temp11", "empty paperdoll", "eleven wearable pieces",
            "one tick", "ten second",
        } {
            require.Contains(t, def.Description, needle)
        }
    }
    require.True(t, found, "the full-dress scenario is registered")
    // The newest round owns the head of the list the web UI serves
    // (the delevel round does now, the church entry round follows,
    // the burst round after it).
    require.Equal(t, "delevel", Definitions()[0].ID)
    require.Equal(t, "church-entry", Definitions()[1].ID)
    require.Equal(t, "full-dress", Definitions()[2].ID)
}

// TestFullDressResetNakedWithTheWholeBag pins the injected start
// state: the creation spawn, the level 15 vitals, the bare wallet and
// the complete outfit in the bag with nothing worn.
func TestFullDressResetNakedWithTheWholeBag(t *testing.T) {
    reset := fullDressReset(dressAccount)
    require.Equal(t, int32(15), reset.Level)
    require.Equal(t, int64(0), reset.SP,
        "no lessons wait on the dress run")
    require.Equal(t, int64(20000), reset.Adena)
    require.Equal(t, int32(elvenSpawnX), reset.X)
    require.Equal(t, int32(elvenSpawnY), reset.Y)
    require.Equal(t, int32(elvenSpawnZ), reset.Z)
    require.Equal(t, int32(level15HP), reset.MaxHP)
    require.Equal(t, int32(level15MP), reset.MaxMP)
    require.Len(t, reset.Items, len(fullDressSlots),
        "every covered slot carries its piece")
}

// TestFullDressItemsAreWearable pins the injected inventory: every
// stack is a wearable piece with gear stats, the body parts cover the
// eleven slots exactly (the two rings ride as separate rows - the C1
// jewelry is not stackable) and the adena never rides the item list.
func TestFullDressItemsAreWearable(t *testing.T) {
    require.Len(t, fullDressItems, 11)
    bodyParts := make(map[string]int, 11)
    for _, item := range fullDressItems {
        require.Positive(t, item.Count)
        stats, ok := npcdata.ItemGearStats(item.ItemID)
        require.True(t, ok,
            "the item must carry gear stats to be wearable")
        require.NotEmpty(t, stats.BodyPart)
        bodyParts[stats.BodyPart]++
    }
    require.Equal(t, 1, bodyParts["lrhand"], "the Brandish two hander")
    require.Equal(t, 1, bodyParts["chest"], "the wooden breastplate")
    require.Equal(t, 1, bodyParts["legs"], "the bone gaiters")
    require.Equal(t, 1, bodyParts["head"], "the leather helmet")
    require.Equal(t, 1, bodyParts["gloves"], "the leather gloves")
    require.Equal(t, 1, bodyParts["feet"], "the apprentice's shoes")
    require.Equal(t, 2, bodyParts["rear;lear"], "the earring pair")
    require.Equal(t, 2, bodyParts["rfinger;lfinger"],
        "the ring pair, one row per finger")
    require.Equal(t, 1, bodyParts["neck"], "the necklace")
    for _, item := range fullDressItems {
        require.NotEqual(t, adenaItemID, item.ItemID,
            "the adena stack never rides the item list")
    }
}

// TestFullDressChecksCoverTheStory pins the check list of the burst
// scenario: the world entry, the whole outfit worn and the burst
// window held.
func TestFullDressChecksCoverTheStory(t *testing.T) {
    checks := fullDressChecks()
    require.Len(t, checks, 3)
    require.Equal(t, checkOnline, checks[0].ID)
    require.Equal(t, checkDress, checks[1].ID)
    require.Equal(t, checkFast, checks[2].ID)
    for _, check := range checks {
        require.False(t, check.Done)
        require.NotEmpty(t, check.Label)
    }
}

// TestFullDressWatchLatchesTheChecks pins the pass conditions: an
// empty paperdoll keeps every check open, the fully dressed paperdoll
// latches the dress and the fast checks together, and a dress that
// lands past the world entry window fails the fast check - the
// retired fixed pause of over twenty seconds for the same bag.
func TestFullDressWatchLatchesTheChecks(t *testing.T) {
    watch := &fullDressWatch{}
    test := &Test{}
    test.setChecks(fullDressChecks())

    // The world entry with the empty paperdoll: the dress check stays
    // open (0 of 11).
    naked := newFullDressBot()
    watch.evaluate(naked, test)
    require.False(t, test.checks[1].Done,
        "the empty paperdoll keeps the dress check open")
    require.False(t, test.checks[2].Done)

    // The burst lands: the whole outfit on the paperdoll, the dress
    // and the fast checks latch together (the elapsed time since the
    // online moment stays inside the window).
    dressed := newFullDressBot()
    var doll [state.PaperdollSlots]int32
    for i, index := range fullDressSlots {
        doll[index] = int32(1000 + i)
    }
    dressed.ApplyPaperdoll(doll)
    watch.evaluate(dressed, test)
    require.True(t, test.checks[1].Done,
        "the full paperdoll latches the dress check")
    require.True(t, test.checks[2].Done,
        "the burst inside the window latches the fast check")

    // A dress that lands past the window (the retired fixed pause
    // pacing) never passes the fast check.
    slow := &fullDressWatch{onlineAt: time.Now().Add(-fullDressWindow - time.Minute)}
    slowTest := &Test{}
    slowTest.setChecks(fullDressChecks())
    slow.evaluate(dressed, slowTest)
    require.True(t, slowTest.checks[1].Done,
        "the slow run still wears the whole outfit")
    require.False(t, slowTest.checks[2].Done,
        "the dress past the window fails the fast check")
}

// TestFullDressWatchIgnoresOfflineTracker pins the precondition: the
// offline tracker (the early session and the reconnect windows) never
// evaluates the checks.
func TestFullDressWatchIgnoresOfflineTracker(t *testing.T) {
    watch := &fullDressWatch{}
    test := &Test{}
    test.setChecks(fullDressChecks())
    offline := state.NewBot(dressAccount)
    var doll [state.PaperdollSlots]int32
    for i, index := range fullDressSlots {
        doll[index] = int32(1000 + i)
    }
    offline.ApplyPaperdoll(doll)
    watch.evaluate(offline, test)
    require.False(t, test.checks[1].Done,
        "the offline tracker never dresses")
    require.False(t, test.checks[2].Done)
    require.True(t, watch.onlineAt.IsZero(),
        "the offline poll never anchors the window")
}

// TestFullDressTimeoutBoundsTheRun pins the timeout constant: the run
// is the injection, the login, one burst and the shutdown - the five
// minute budget keeps the session prologue comfortable.
func TestFullDressTimeoutBoundsTheRun(t *testing.T) {
    require.Equal(t, 5*time.Minute, fullDressTimeout)
    require.Less(t, fullDressTimeout, farmTimeout,
        "the burst round stays inside the farm readiness bound")
}
