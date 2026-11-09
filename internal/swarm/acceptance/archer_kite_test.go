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
            "bow cooldown", "one kill per two minutes",
            "arrow stock never empties", "#28",
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
// entry plus the seven contract checks in the narrative order.
func TestArcherKiteChecksCoverTheStory(t *testing.T) {
    checks := archerKiteChecks()
    ids := make([]string, 0, len(checks))
    for _, check := range checks {
        ids = append(ids, check.ID)
        require.NotEmpty(t, check.Label)
        require.False(t, check.Done)
    }
    require.Equal(t, []string{
        checkOnline, checkArcherKiteArmed, checkArcherKiteCadence,
        checkArcherKiteRange, checkArcherKiteRetreats,
        checkArcherKiteKills, checkArcherKiteSurvives,
        checkArcherKiteQuiver,
    }, ids)
}

// TestArcherKiteFoldLatchesTheTelemetry pins the pure fold: the
// minimums latch down (never up), the arrow floor forms on the first
// answer and the offline and death flags latch once. A walking
// sample still counts as a fight but never latches the bow band: the
// retreat legs open the gap past the band by design. The shot cadence
// chains the own Attack broadcasts of the SAME target and breaks on
// a target switch or a shot whose target already vanished.
func TestArcherKiteFoldLatchesTheTelemetry(t *testing.T) {
    base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
    watch := newArcherKiteWatch()
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 87, arrows: 540,
        fighting: true, fightDist: 420,
        shotAt: base, fightTarget: 7,
    })
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 64, arrows: 470,
        fighting: true, fightDist: 260,
        shotAt: base.Add(3 * time.Second), fightTarget: 7,
    })
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 71, arrows: 430,
        fighting: true, fightDist: 140,
        shotAt: base.Add(6 * time.Second), fightTarget: 7,
    })
    // A retreat leg: the character walks, the lagging chaser leaves
    // the band. The sample counts as a fight, but the band ceiling
    // stays at the standing 420.
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: 70, arrows: 430,
        fighting: true, walking: true, fightDist: 796,
        shotAt: base.Add(9 * time.Second), fightTarget: 7,
    })
    watch = watch.fold(archerKiteSample{online: true, arrows: 430,
        hpPercent: -1})

    require.Equal(t, 4, watch.fightSamples)
    require.InDelta(t, 420.0, watch.maxFightDist, 0.01)
    require.Equal(t, 4, watch.shotCount)
    require.Equal(t, 3, watch.shotGaps)
    require.Equal(t, 3, watch.fastShotGaps)
    require.InDelta(t, 64.0, watch.minHP, 0.01)
    require.Equal(t, int32(430), watch.minArrows)
    require.False(t, watch.sawOffline)
    require.False(t, watch.died)

    // A target switch starts a fresh chain: the first shot at the
    // new mob is not a slow shot of the old fight.
    watch = watch.fold(archerKiteSample{
        online: true, fighting: true, fightDist: 400,
        hpPercent: -1, arrows: -1,
        shotAt: base.Add(12 * time.Second), fightTarget: 8,
    })
    require.Equal(t, 5, watch.shotCount)
    require.Equal(t, 3, watch.shotGaps)
    // A shot whose target already vanished breaks the chain too: the
    // fresh pick must not inherit the dead fight's gap.
    watch = watch.fold(archerKiteSample{
        online: true, hpPercent: -1, arrows: -1,
        shotAt: base.Add(15 * time.Second),
    })
    require.Equal(t, 6, watch.shotCount)
    require.Equal(t, 3, watch.shotGaps)

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
// pure evaluation: the honest kite window passes every check, the
// treadmill cadence and the chase beyond the band fail their own
// verdicts, a dry quiver fails the stock verdict, no kill fails the
// resolution verdict and a death fails the survival verdict.
func TestArcherKiteVerdictsPinTheContract(t *testing.T) {
    // The honest kite window: the bow and the quiver worn, six
    // standing fight samples inside the bow band, eight shots whose
    // seven same-fight gaps all sit inside the cooldown, six
    // shot-cycle retreats, two kills of the floor, the HP floor
    // never breached, the stock never dry.
    honest := archerKiteWatch{
        bowSeen: true, quiverSeen: true,
        fightSamples: 6, maxFightDist: 480,
        shotCount: 8, shotGaps: 7, fastShotGaps: 7, reloadTails: 6,
        kills: 2, minKills: 2,
        kiteSteps: 8, kiteHolds: 1, minHP: 55, minArrows: 380,
    }
    verdicts := honest.verdicts()
    require.True(t, verdicts.armed)
    require.True(t, verdicts.cadence)
    require.True(t, verdicts.band)
    require.True(t, verdicts.retreats)
    require.True(t, verdicts.kills)
    require.True(t, verdicts.survives)
    require.True(t, verdicts.quiver)
    require.Contains(t, verdicts.cadenceD, "100%")

    // The treadmill: most same-fight gaps run 20-30 seconds (the
    // old safe-distance gate) - the cadence verdict fails while the
    // band verdict still reads.
    treadmill := honest
    treadmill.fastShotGaps = 5
    verdicts = treadmill.verdicts()
    require.False(t, verdicts.cadence)
    require.True(t, verdicts.band)
    require.Contains(t, verdicts.cadenceD, "71%")

    // The cadence evidence floor: a window of three gaps holds the
    // verdict open (not failed, undecided).
    earlyCadence := honest
    earlyCadence.shotGaps = 3
    earlyCadence.fastShotGaps = 3
    verdicts = earlyCadence.verdicts()
    require.False(t, verdicts.cadence)
    require.Contains(t, verdicts.cadenceD, "3 shot gaps")

    // The chase beyond the band: the standing fight ran past the
    // stall radius - the band verdict fails.
    chase := honest
    chase.maxFightDist = archerKiteBowBand + archerKiteBandSlack + 1
    verdicts = chase.verdicts()
    require.False(t, verdicts.band)
    require.True(t, verdicts.cadence)

    // The band evidence floor: a window of two standing samples
    // holds the verdict open.
    earlyBand := honest
    earlyBand.fightSamples = 2
    verdicts = earlyBand.verdicts()
    require.False(t, verdicts.band)
    require.Contains(t, verdicts.bandD, "2 fight samples")

    // The static archer: no kite steps - the retreat verdict fails.
    standing := honest
    standing.kiteSteps = 1
    verdicts = standing.verdicts()
    require.False(t, verdicts.retreats)

    // The unresolved window: the shots flew but no fight ended.
    unresolved := honest
    unresolved.kills = 1
    verdicts = unresolved.verdicts()
    require.False(t, verdicts.kills)
    require.Contains(t, verdicts.killsD, "1 kills of 2 wanted")

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
// step and hold lines of the hunt loop count, the shot-cycle retreat
// line counts as both a retreat and a reload tail, the kill line
// counts, the ordinary hunt lines and the mirrored session lines do
// not, and every line still reaches the wrapped callback.
func TestArcherKiteLogCountsTheKiteLines(t *testing.T) {
    var seen []string
    kiteLog := newArcherKiteLog(func(line string) {
        seen = append(seen, line)
    })
    kiteLog.line("Hunt: hostile 7 closed to 200 units of the fight " +
        "on 7, kiting clear (step 1 of 8)")
    kiteLog.line("Hunt: hostile 7 closed to 240 units of the fight " +
        "on 7, kiting clear (step 2 of 8)")
    kiteLog.line("Hunt: the windup on 7 ended (hostile 7 at 320 " +
        "units), kiting the reload tail")
    kiteLog.line("Hunt: shot released on 7, hostile 7 holds 300 " +
        "units, kiting the reload tail")
    kiteLog.line("Hunt: no walkable retreat lane on target 7 " +
        "(cornered), holding ground and shooting the bow")
    kiteLog.line("Hunt: the chasers surround the bow fight on " +
        "target 7, holding ground and shooting through the train")
    kiteLog.line("Hunt: target 7 died, looting")
    kiteLog.line("Hunt: target 8 selected, engaging")
    kiteLog.line("acceptance: entered the world")

    steps, holds, reloads, kills := kiteLog.counts()
    require.Equal(t, 4, steps, "two steps and two shot-cycle lines")
    require.Equal(t, 2, holds)
    require.Equal(t, 2, reloads)
    require.Equal(t, 1, kills)
    require.Len(t, seen, 9, "every line flows to the wrapped log")
}

// TestArcherKiteSampleOfReadsTheTracker pins the live extraction:
// the worn bow and quiver answer the armed check, the fight distance
// reads the self position against the fight target, the shot stamp
// and the fight target feed the cadence chain, the health percentage
// and the arrow stock sum the worn and the bagged piles.
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
    // The character walks a retreat leg: the position stays at the
    // spawn (the packet start point), the destination pulls away.
    bot.ApplyMovement(state.Movement{
        ObjectID: 100,
        X:        archerKiteSpawnX, Y: archerKiteSpawnY,
        Z:     archerKiteSpawnZ,
        DestX: archerKiteSpawnX - 500, DestY: archerKiteSpawnY,
        DestZ: archerKiteSpawnZ,
    })

    sample := archerKiteSampleOf(bot)
    require.True(t, sample.online)
    require.False(t, sample.dead)
    require.True(t, sample.walking, "the walk broadcast reads through")
    require.True(t, sample.bowWorn)
    require.True(t, sample.quiverWorn)
    require.True(t, sample.fighting)
    require.InDelta(t, 300.0, sample.fightDist, 1.0)
    require.False(t, sample.shotAt.IsZero(),
        "the self Attack broadcast stamps the shot")
    require.Equal(t, int32(7), sample.fightTarget)
    require.InDelta(t, 50.0, sample.hpPercent, 1.0)
    require.Equal(t, int32(530), sample.arrows,
        "the worn quiver and the bagged pile sum")
}
