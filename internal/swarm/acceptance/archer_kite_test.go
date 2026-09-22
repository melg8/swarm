// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The archer kite scenario tests (issue #28): the registration, the
// injected start state, the check story and the pure evaluation of
// the kiting contract - the fold and the verdicts run against
// synthetic samples so the CI verifies the check logic without the
// live stack.

// TestArcherKiteDefinition pins the scenario registration: the
// archer type owns temp15, runs under the window budget and carries
// the contract story.
func TestArcherKiteDefinition(t *testing.T) {
    var found bool
    for _, def := range Definitions() {
        if def.ID != archerKiteScenarioID {
            continue
        }
        found = true
        require.Equal(t, archerKiteAccount, def.Account)
        require.Equal(t, archerKiteTimeout(), def.Timeout)
        require.NotNil(t, def.Scenario)
        for _, needle := range []string{
            "36000 50229 -3456", "level 7", "archer type",
            "bow radii", "arrow stock never empties", "#28",
        } {
            require.Contains(t, def.Description, needle)
        }
    }
    require.True(t, found, "the archer-kite scenario is registered")
}

// TestArcherKiteDurationReadsTheEnv pins the window knob: the
// default smoke, the env override and the broken value fallback.
func TestArcherKiteDurationReadsTheEnv(t *testing.T) {
    require.Equal(t, 10*time.Minute, archerKiteDuration())
    t.Setenv("SWARM_ARCHER_KITE_MINUTES", "25")
    require.Equal(t, 25*time.Minute, archerKiteDuration())
    t.Setenv("SWARM_ARCHER_KITE_MINUTES", "zero")
    require.Equal(t, 10*time.Minute, archerKiteDuration())
}

// TestArcherKiteResetMatchesTheStartState pins the injected start
// state: the level 7 vitals of the gain table, the level bottom
// experience, the empty wallet and skill points, the cell focus
// position with the geodata height and the bow kit.
func TestArcherKiteResetMatchesTheStartState(t *testing.T) {
    reset := archerKiteReset(archerKiteAccount)
    require.Equal(t, archerKiteAccount, reset.Account)
    require.Equal(t, int32(archerKiteStartLevel), reset.Level)
    require.Equal(t, reset.Exp,
        soakExperienceTable[archerKiteStartLevel-1])
    require.Equal(t, int64(0), reset.SP)
    require.Equal(t, int64(0), reset.Adena)
    require.Equal(t, int32(archerKiteSpawnX), reset.X)
    require.Equal(t, int32(archerKiteSpawnY), reset.Y)
    require.Equal(t, int32(archerKiteSpawnZ), reset.Z)
    require.Equal(t, int32(archerKiteStartHP), reset.MaxHP)
    require.Equal(t, int32(archerKiteStartMP), reset.MaxMP)
    require.Equal(t, int32(archerKiteStartCP), reset.MaxCP)
    require.NotEmpty(t, reset.Items, "the bow kit rides the reset")
}

// TestArcherKiteItemsMatchTheKit pins the injected kit: the short
// bow, the full quiver, the wooden outfit of the zone return dump
// and the healing potions - and no second left hand item beside the
// arrows.
func TestArcherKiteItemsMatchTheKit(t *testing.T) {
    counts := map[int32]int32{}
    for _, item := range archerKiteItems {
        counts[item.ItemID] += item.Count
        require.Positive(t, item.Count)
    }
    require.Equal(t, int32(1), counts[13], "the Short Bow")
    require.Equal(t, int32(archerKiteQuiver), counts[woodenArrowItemID],
        "the full quiver")
    require.Equal(t, int32(1), counts[23], "Wooden Breastplate")
    require.Equal(t, int32(1), counts[31], "Bone Gaiters")
    require.Equal(t, int32(1), counts[44], "Leather Helmet")
    require.Equal(t, int32(1), counts[50], "Leather Gloves")
    require.Equal(t, int32(1), counts[1121], "Apprentice's Shoes")
    require.Equal(t, int32(1), counts[114], "Earring of Strength")
    require.Equal(t, int32(1), counts[115], "Earring of Wisdom")
    require.Equal(t, int32(2), counts[876], "the Ring of Anguish pair")
    require.Equal(t, int32(1), counts[907], "Necklace of Anguish")
    require.Equal(t, int32(3), counts[1060], "healing potions")
    // No shield: the left hand belongs to the quiver (the archer
    // profile lets no other left hand item stay).
    require.NotContains(t, counts, int32(20), "no Buckler")
}

// TestArcherKiteChecksCoverTheStory pins the check list: the world
// entry plus the five contract checks in the narrative order.
func TestArcherKiteChecksCoverTheStory(t *testing.T) {
    checks := archerKiteChecks()
    ids := make([]string, 0, len(checks))
    for _, check := range checks {
        ids = append(ids, check.ID)
        require.NotEmpty(t, check.Label)
        require.False(t, check.Done)
    }
    require.Equal(t, []string{
        checkOnline, checkArcherKiteArmed, checkArcherKiteRange,
        checkArcherKiteRetreats, checkArcherKiteSurvives,
        checkArcherKiteQuiver,
    }, ids)
}

// TestArcherKiteFoldLatchesTheTelemetry pins the pure fold: the
// ranged samples count only the fights beyond the melee radius, the
// minimums latch down (never up), the arrow floor forms on the first
// answer and the offline and death flags latch once.
func TestArcherKiteFoldLatchesTheTelemetry(t *testing.T) {
    watch := newArcherKiteWatch()
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 87, arrows: 540,
        fighting: true, fightDist: 420,
    })
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 64, arrows: 470,
        fighting: true, fightDist: 260,
    })
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 71, arrows: 430,
        fighting: true, fightDist: 140,
    })
    watch = watch.fold(archerKiteSample{online: true, arrows: 430,
        hpPercent: -1})

    require.Equal(t, 3, watch.fightSamples)
    require.Equal(t, 2, watch.rangedSamples)
    require.InDelta(t, 420.0, watch.maxFightDist, 0.01)
    require.InDelta(t, 64.0, watch.minHP, 0.01)
    require.Equal(t, int32(430), watch.minArrows)
    require.False(t, watch.sawOffline)
    require.False(t, watch.died)

    watch = watch.fold(archerKiteSample{
        online: false, dead: true, hpPercent: -1, arrows: -1,
    })
    require.True(t, watch.sawOffline)
    require.True(t, watch.died)
    // The minimums never move up on a later sample.
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 99, arrows: 600,
    })
    require.InDelta(t, 64.0, watch.minHP, 0.01)
    require.Equal(t, int32(430), watch.minArrows)
}

// TestArcherKiteVerdictsPinTheContract pins the verdict table of the
// pure evaluation: the honest kite window passes every check, a
// window of melee samples fails the range verdict, a dry quiver
// fails the stock verdict and a death fails the survival verdict.
func TestArcherKiteVerdictsPinTheContract(t *testing.T) {
    // The honest kite window: the bow and the quiver worn, six
    // fight samples of which four ranged, all inside the bow band,
    // three kite steps, the HP floor never breached, the stock
    // never dry.
    honest := archerKiteWatch{
        bowSeen: true, quiverSeen: true,
        fightSamples: 6, rangedSamples: 4, maxFightDist: 480,
        kiteSteps: 3, kiteHolds: 1, minHP: 55, minArrows: 380,
    }
    verdicts := honest.verdicts()
    require.True(t, verdicts.armed)
    require.True(t, verdicts.ranged)
    require.True(t, verdicts.retreats)
    require.True(t, verdicts.survives)
    require.True(t, verdicts.quiver)

    // The melee tank: the archer took the melee trades instead of
    // kiting (every sample below the melee radius) - the range
    // verdict fails, the rest of the contract still reads.
    melee := honest
    melee.rangedSamples = 1
    verdicts = melee.verdicts()
    require.False(t, verdicts.ranged)
    require.True(t, verdicts.retreats)

    // The chase beyond the band: the fight ran past the stall
    // radius - the range verdict fails.
    chase := honest
    chase.maxFightDist = archerKiteBowBand + archerKiteBandSlack + 1
    verdicts = chase.verdicts()
    require.False(t, verdicts.ranged)

    // The evidence floor: a window of two fights holds the range
    // verdict open (not failed, undecided).
    early := honest
    early.fightSamples = 2
    early.rangedSamples = 2
    verdicts = early.verdicts()
    require.False(t, verdicts.ranged)
    require.Contains(t, verdicts.rangedD, "2 fight samples")

    // The static archer: no kite steps - the retreat verdict fails.
    standing := honest
    standing.kiteSteps = 1
    verdicts = standing.verdicts()
    require.False(t, verdicts.retreats)

    // The dry quiver: the stock hit zero.
    dry := honest
    dry.minArrows = 0
    verdicts = dry.verdicts()
    require.False(t, verdicts.quiver)
    require.Contains(t, verdicts.quiverD, "min 0 arrows")

    // The death and the offline drop fail the survival verdict.
    dead := honest
    dead.died = true
    verdicts = dead.verdicts()
    require.False(t, verdicts.survives)
    require.Contains(t, verdicts.surviveD, "died")

    offline := honest
    offline.sawOffline = true
    verdicts = offline.verdicts()
    require.False(t, verdicts.survives)
    require.Contains(t, verdicts.surviveD, "status dropped")

    // The HP bottom: the health fell under the floor.
    bottomed := honest
    bottomed.minHP = archerKiteHPFloor - 1
    verdicts = bottomed.verdicts()
    require.False(t, verdicts.survives)
}

// TestArcherKiteLogCountsTheKiteLines pins the log wrapper: the kite
// step and hold lines of the hunt loop count, the ordinary hunt
// lines and the mirrored session lines do not, and every line still
// reaches the wrapped callback.
func TestArcherKiteLogCountsTheKiteLines(t *testing.T) {
    var seen []string
    kiteLog := newArcherKiteLog(func(line string) {
        seen = append(seen, line)
    })
    kiteLog.line("Hunt: hostile 7 closed to 200 units of the fight " +
        "on 7, kiting clear (step 1 of 8)")
    kiteLog.line("Hunt: hostile 7 closed to 240 units of the fight " +
        "on 7, kiting clear (step 2 of 8)")
    kiteLog.line("Hunt: no walkable retreat lane on target 7 " +
        "(cornered), holding ground and shooting the bow")
    kiteLog.line("Hunt: the chasers surround the bow fight on " +
        "target 7, holding ground and shooting through the train")
    kiteLog.line("Hunt: target 7 selected, engaging")
    kiteLog.line("acceptance: entered the world")

    steps, holds := kiteLog.counts()
    require.Equal(t, 2, steps)
    require.Equal(t, 2, holds)
    require.Len(t, seen, 6, "every line flows to the wrapped log")
}

// TestArcherKiteSampleOfReadsTheTracker pins the live extraction:
// the worn bow and quiver answer the armed check, the fight distance
// reads the self position against the fight target, the health
// percentage and the arrow stock sum the worn and the bagged piles.
func TestArcherKiteSampleOfReadsTheTracker(t *testing.T) {
    bot := state.NewBot(archerKiteAccount)
    bot.SetCharacter(archerKiteAccount, 100, 18,
        archerKiteSpawnX, archerKiteSpawnY, archerKiteSpawnZ,
        float64(archerKiteStartHP/2), float64(archerKiteStartMP))
    bot.SetOnline(archerKiteAccount)
    bot.ApplyUserInfo(state.UserInfo{
        Name:  archerKiteAccount,
        Level: archerKiteStartLevel, ClassID: 18,
        X: archerKiteSpawnX, Y: archerKiteSpawnY, Z: archerKiteSpawnZ,
        MaxHP: archerKiteStartHP,
        CurHP: archerKiteStartHP / 2,
        MaxMP: archerKiteStartMP,
        CurMP: archerKiteStartMP,
    })
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 201, ItemID: 13, Count: 1, Equipped: true},
        {ObjectID: 202, ItemID: woodenArrowItemID, Count: 470,
            Equipped: true},
        {ObjectID: 203, ItemID: woodenArrowItemID, Count: 60},
        {ObjectID: 204, ItemID: 1060, Count: 3},
    })
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = 201
    paperdoll[state.PaperdollLHand] = 202
    bot.ApplyPaperdoll(paperdoll)

    // The fight: the self swings at mob 7 standing 300 units east.
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000470, Attackable: true,
        X: archerKiteSpawnX + 300, Y: archerKiteSpawnY,
        Z: archerKiteSpawnZ, Name: "Kaboo Orc Grunt",
    })
    bot.ApplyAttack(state.Attack{
        AttackerID: 100, X: archerKiteSpawnX, Y: archerKiteSpawnY,
        Z:       archerKiteSpawnZ,
        TargetX: archerKiteSpawnX + 300, TargetY: archerKiteSpawnY,
        TargetZ:     archerKiteSpawnZ,
        TargetIDs:   [state.AttackTargets]int32{7},
        TargetCount: 1,
    })
    bot.ApplyAutoAttackStart(100)

    sample := archerKiteSampleOf(bot)
    require.True(t, sample.online)
    require.False(t, sample.dead)
    require.True(t, sample.bowWorn)
    require.True(t, sample.quiverWorn)
    require.True(t, sample.fighting)
    require.InDelta(t, 300.0, sample.fightDist, 1.0)
    require.InDelta(t, 50.0, sample.hpPercent, 1.0)
    require.Equal(t, int32(530), sample.arrows,
        "the worn quiver and the bagged pile sum")
}
