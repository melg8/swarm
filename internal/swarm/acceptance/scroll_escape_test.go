// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/state"
)

// The scroll escape scenario tests: the level 15 fighter on the
// Keltir glade uses one Scroll of Escape and lands on the elven town
// respawn ring.

// TestScrollEscapeDefinition pins the scenario registration: the
// scroll round owns the head of the web UI list, temp26 and the five
// minute bound.
func TestScrollEscapeDefinition(t *testing.T) {
    defs := Definitions()
    require.NotEmpty(t, defs)
    require.Equal(t, scrollEscapeScenarioID, defs[0].ID,
        "the scroll escape scenario owns the head of the list")
    require.Equal(t, scrollEscapeAccount, defs[0].Account)
    require.Equal(t, scrollEscapeTimeout, defs[0].Timeout)
    require.NotNil(t, defs[0].Scenario)
    for _, needle := range []string{
        "45496 44544 -3704", "level 15", "Scroll of Escape",
        "skill 2013", "skill0000", "town respawn ring",
    } {
        require.Contains(t, defs[0].Description, needle)
    }
}

// TestScrollEscapeResetMatchesTheStartState pins the injected state:
// the level 15 vitals on the Keltir glade with exactly one Scroll of
// Escape in the bag and nothing else.
func TestScrollEscapeResetMatchesTheStartState(t *testing.T) {
    reset := scrollEscapeReset(scrollEscapeAccount)
    require.Equal(t, scrollEscapeAccount, reset.Account)
    require.Equal(t, int32(scrollEscapeLevel), reset.Level)
    require.Equal(t, reset.Exp, int64(level15Exp))
    require.Equal(t, int32(scrollEscapeSpawnX), reset.X)
    require.Equal(t, int32(scrollEscapeSpawnY), reset.Y)
    require.Equal(t, int32(scrollEscapeSpawnZ), reset.Z)
    require.Equal(t, int32(level15HP), reset.MaxHP)
    require.Equal(t, int32(level15MP), reset.MaxMP)
    require.Len(t, reset.Items, 1)
    require.Equal(t, hunt.SOEItemID(), reset.Items[0].ItemID)
    require.Equal(t, int32(1), reset.Items[0].Count)
}

// TestScrollEscapeChecksCoverTheStory pins the check list: the world
// entry, the cast, the icon, the consumption and the landing.
func TestScrollEscapeChecksCoverTheStory(t *testing.T) {
    checks := scrollEscapeChecks()
    require.Len(t, checks, 5)
    require.Equal(t, checkOnline, checks[0].ID)
    require.Equal(t, checkScrollCast, checks[1].ID)
    require.Equal(t, checkScrollIcon, checks[2].ID)
    require.Equal(t, checkScrollConsumed, checks[3].ID)
    require.Equal(t, checkScrollLanded, checks[4].ID)
    for _, check := range checks {
        require.NotEmpty(t, check.Label)
        require.False(t, check.Done)
    }
}

// newScrollEscapeBot builds an online tracker on the start ground with
// the scroll in the bag.
func newScrollEscapeBot() *state.Bot {
    bot := state.NewBot(scrollEscapeAccount)
    bot.SetCharacter(scrollEscapeAccount, 100, 18,
        scrollEscapeSpawnX, scrollEscapeSpawnY, scrollEscapeSpawnZ,
        level15HP, level15MP)
    bot.SetOnline(scrollEscapeAccount)
    bot.ApplyItemList([]state.InventoryItem{{
        ObjectID: 900, ItemID: hunt.SOEItemID(), Count: 1,
    }})

    return bot
}

// TestScrollEscapeWatchLatchesTheStory drives the watch through the
// whole scroll: the cast window with the vortex icon, the consumption
// and the town ring landing.
func TestScrollEscapeWatchLatchesTheStory(t *testing.T) {
    bot := newScrollEscapeBot()
    test := &Test{}
    test.setChecks(scrollEscapeChecks())
    test.updateCheck(checkOnline, true, "online")
    watch := &scrollEscapeWatch{}

    watch.evaluate(bot, test)
    require.False(t, test.checks[1].Done, "no cast before the scroll use")
    require.False(t, test.checks[2].Done, "no icon before the scroll use")
    require.False(t, test.checks[3].Done, "the scroll is still in the bag")
    require.False(t, test.checks[4].Done,
        "the start ground is not the town ring")

    // The server answers the use with the 20 second self cast.
    bot.ApplySkillCast(state.SkillCast{
        CasterID: 100, TargetID: 100,
        SkillID: scrollEscapeSkillID, SkillLevel: 1,
        HitTimeMs: 20000,
    })
    watch.evaluate(bot, test)
    require.True(t, test.checks[1].Done, "the cast window reached the list")
    require.Contains(t, test.checks[1].Detail, "20000 ms")
    require.True(t, test.checks[2].Done, "the cast carries skill0000")
    require.Contains(t, test.checks[2].Detail, "skill0000")

    // The consume removes the item from the bag.
    bot.ApplyInventoryUpdate([]state.InventoryItem{{
        ObjectID: 900, Change: 3,
    }})
    watch.evaluate(bot, test)
    require.True(t, test.checks[3].Done, "the scroll left the bag")

    // The teleport lands the character on the first respawn point of
    // the elf_town ring.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45487, Y: 49767, Z: -2950,
    })
    watch.evaluate(bot, test)
    require.True(t, test.checks[4].Done, "the ring landing latched")
    require.Contains(t, test.checks[4].Detail, "45487,49767")
    require.True(t, allChecksDone(test))
}

// TestScrollEscapeWatchKeepsTheStartGroundOpen pins the landing
// predicate: the start ground and the village plaza outside the ring
// radius keep the check open, the chaotic points of the region never
// count as the town landing.
func TestScrollEscapeWatchKeepsTheStartGroundOpen(t *testing.T) {
    bot := newScrollEscapeBot()
    test := &Test{}
    test.setChecks(scrollEscapeChecks())
    test.updateCheck(checkOnline, true, "online")
    watch := &scrollEscapeWatch{}

    watch.evaluate(bot, test)
    require.False(t, test.checks[4].Done)

    // A chaotic respawn point of the elf_town region (40720 56064)
    // sits outside the town ring box.
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 40720, Y: 56064, Z: -3580,
    })
    watch.evaluate(bot, test)
    require.False(t, test.checks[4].Done,
        "the chaotic point is not the town respawn")

    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: scrollEscapeTownX, Y: scrollEscapeTownY,
        Z: -2950,
    })
    watch.evaluate(bot, test)
    require.True(t, test.checks[4].Done)
}
