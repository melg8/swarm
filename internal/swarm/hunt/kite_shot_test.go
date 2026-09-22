// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The shot-paced layer of the archer kite (issue #60): the
// character's own Attack broadcast is the server commit of the bow
// shot, so the retreat arms the moment the shot released - inside
// the bow cooldown - instead of after the mob crossed the retreat
// radius. The tests pin the trigger, the pursue band, the streak
// exemption and the shared hold answers of the rhythm.

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// kiteBowBotChaseFresh builds the kite scene with the fight
// freshness carried by the character's own chase step instead of a
// swing: the server broadcasts MoveToPawn while the character chases
// its target, and the chase refreshes the fight view (CombatActiveAt)
// without committing a shot - the shot-paced trigger stays quiet and
// the proximity path runs alone. The scenes that pin the proximity
// semantics (the streak bound, the radius knob, the deck gap) build
// on this one.
func kiteBowBotChaseFresh(
    t *testing.T, mobX int32,
) (*state.Bot, *fakeGame, *Loop) {
    t.Helper()
    bot := newTestBot()
    bot.ApplyInventoryUpdate([]state.InventoryItem{
        {ObjectID: 99, ItemID: 13, Equipped: true, Change: 1},
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: mobX, Y: 50000, Name: "Keltir",
    })
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.engageAt = time.Now()
    // The fight runs: the character chases the mob (the fresh fight
    // view of SelfFighting, no shot committed).
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: mobX, TargetY: 50000,
        TargetZ: -3500,
    })
    loop.lastHit = time.Now().Add(-time.Minute)

    return bot, game, loop
}

// TestKiteShotPacedRetreatArmsAfterTheShot pins the trigger of the
// rhythm: a bow fight with the hostile 440 units out - inside the
// pursue band (450), beyond the proximity trigger (250) - retreats
// the moment the character's own shot released. Before the layer the
// same scene was the standing fight the issue reports: the mob
// closed unopposed while the archer stood through the reload.
func TestKiteShotPacedRetreatArmsAfterTheShot(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45440)
    loop.tick()
    require.Len(t, game.walks, 1,
        "a fresh shot with the hostile inside the band must arm the "+
            "retreat")
    step := game.walks[0]
    _, selfY, selfZ, ok := bot.SelfPosition()
    require.True(t, ok)
    require.Equal(t, selfZ, step[2],
        "the step keeps the character's deck")
    require.Equal(t, int32(44600), step[0],
        "the step runs straight away from the target")
    require.Equal(t, selfY, step[1])
    require.Empty(t, game.forces,
        "the kite tick must not re-request the attack")
}

// TestKiteShotPacedStaysQuietWithoutAFreshShot pins the trigger
// boundary: the fight view refreshed by a chase step (no shot
// committed) keeps the rhythm quiet - the retreat rides the shot
// cycle, not the mere running of the fight.
func TestKiteShotPacedStaysQuietWithoutAFreshShot(t *testing.T) {
    _, game, loop := kiteBowBotChaseFresh(t, 45440)
    loop.tick()
    require.Empty(t, game.walks,
        "no fresh shot must not arm the shot-paced retreat")
    require.Empty(t, game.forces,
        "a running fight inside the band keeps the re-request quiet")
}

// TestKiteShotPacedStaysQuietBeyondTheBand pins the pursue band: a
// hostile beyond the bow engage radius is the stall watchdog's
// re-approach, not a retreat - the rhythm never runs the fight away
// from a mob it should walk back to.
func TestKiteShotPacedStaysQuietBeyondTheBand(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45500)
    loop.tick()
    require.Empty(t, game.walks,
        "a hostile beyond the band must not arm the shot-paced retreat")
    require.Empty(t, game.forces,
        "a running bow fight must not re-request the attack")
}

// TestKiteShotPacedIgnoresTheStreakLimit pins the streak exemption:
// the shuffle bound of the proximity path exists because an endless
// step race starves the fight of every swing - the shot-paced cycle
// keeps swinging once per cooldown, so the exhausted streak must not
// stop the retreat.
func TestKiteShotPacedIgnoresTheStreakLimit(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.kiteFor = 7
    loop.kiteStreak = kiteStreakLimit
    loop.tick()
    require.Len(t, game.walks, 1,
        "the shot-paced retreat is the fight itself, not the shuffle")
}

// TestKiteShotPacedDoesNotCountTheStreak pins the bookkeeping side
// of the same exemption: the rhythm step spends none of the shuffle
// budget - the proximity path keeps its own count untouched.
func TestKiteShotPacedDoesNotCountTheStreak(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)
    require.Zero(t, loop.kiteStreak,
        "the shot-paced step must not spend the shuffle budget")
}

// TestKiteShotPacedWindowGuardsTheDoubleStep pins the trigger window:
// the broadcast stays catchable for two ticks, and the second tick
// inside the window must not issue a second step - the movement
// window owns the walk until it finished.
func TestKiteShotPacedWindowGuardsTheDoubleStep(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.tick()
    require.Len(t, game.walks, 1)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the movement window guards the double trigger")
}

// TestKiteShotPacedEncircledTrainHoldsGround pins the shared hold
// answer: a surrounding train cancels the away vectors, the rhythm
// holds ground and shoots through it exactly like the proximity
// path - a fresh shot never walks the character INTO a chaser.
func TestKiteShotPacedEncircledTrainHoldsGround(t *testing.T) {
    bot, game, loop := kiteBowBot(t, 45200)
    trainMember(bot, 44800, 50000)
    mobHitsCharacterAt(bot, 7, 45200)
    loop.tick()
    require.Empty(t, game.walks,
        "a surrounding train has no away direction - no step")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the hold belongs to the current target")
    require.False(t, loop.kiteHeldAt.IsZero(),
        "the hold must be armed")
}

// TestKiteShotPacedCorneredHoldsGround pins the corner answer of the
// rhythm: a wall behind the retreat blocks every lane of the away
// hemisphere, the fresh shot arms the probe, the probe resolves to
// the cornered hold - the archer stands and shoots the way out.
func TestKiteShotPacedCorneredHoldsGround(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    nav := &fakeNavigator{}
    nav.sightFunc = func(_, to pathfind.Vec3) (bool, error) {
        return to.X > 45000, nil
    }
    loop.SetNavigator(nav)
    loop.tick()
    require.Empty(t, game.walks,
        "a cornered archer stops retreating against the wall")
    require.Equal(t, int32(7), loop.kiteHeldFor,
        "the cornered hold is armed for the target")
}

// TestKiteShotPacedRespectsTheProfileGate pins the profile gate of
// the rhythm: a kite-disabled profile (the standing-archer baseline
// of issue #29) never arms the shot-paced retreat either - the layer
// rides the same enable switch as the proximity path.
func TestKiteShotPacedRespectsTheProfileGate(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45440)
    loop.SetKiteParams(KiteParams{Enabled: false})
    loop.tick()
    require.Empty(t, game.walks,
        "a kite-disabled bow bot must not step on the shot trigger")
}
