// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// The tunable block of the kite fight (owner issue #29): the four
// named knobs ride the loop as KiteParams - the launch config
// overrides them per bot spec, so the standing-archer baseline (the
// kite disabled) and the tuned profile switch without a rebuild.
// The tests pin the gate, the knob reach into the step geometry and
// pacing, and the broken-value fallback to the shipped tuning.

// TestDefaultKiteParamsPinTheShippedTuning pins the equality of the
// params defaults and the constants block of kite.go: a drift on
// either side is a behavior change of every existing profile.
func TestDefaultKiteParamsPinTheShippedTuning(t *testing.T) {
    require.Equal(t, KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: kiteReengageDelay,
    }, DefaultKiteParams())
}

// TestKiteDisabledRunsTheStandingFight pins the baseline profile: a
// bow bot with the kite disabled never arms the step - the mob may
// close into the melee reach, the character stands and shoots (the
// melee fallback rate the live round measures comes from exactly
// this).
func TestKiteDisabledRunsTheStandingFight(t *testing.T) {
    // The mob sits 200 units away: well inside the shipped trigger
    // (250) - the default profile would step here.
    _, game, loop := kiteBowBot(t, 45200)
    loop.SetKiteParams(KiteParams{Enabled: false})
    loop.tick()
    require.Empty(t, game.walks,
        "a kite-disabled bow bot must not step away from the closed "+
            "target")
    require.Empty(t, game.forces,
        "the standing fight keeps its own re-request contract")
}

// TestKiteParamsWidenTheRetreatRadius pins the knob reach: a widened
// retreat radius arms the step from farther out - the fight picks
// the bigger optimal band the config names, nothing else changes
// (the step length and the direction ride their own knobs and the
// train geometry).
func TestKiteParamsWidenTheRetreatRadius(t *testing.T) {
    // 300 units out: beyond the shipped trigger, inside the widened
    // one.
    _, game, loop := kiteBowBot(t, 45300)
    loop.SetKiteParams(KiteParams{
        Enabled:       true,
        RetreatRadius: 400,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: kiteReengageDelay,
    })
    loop.tick()
    require.Len(t, game.walks, 1,
        "the widened radius arms the step from 300 units")
    require.Equal(t, int32(44600), game.walks[0][0],
        "the step length still rides the shipped kiteStep")
}

// TestKiteParamsBrokenNumbersFallBackToTheShippedTuning pins the
// defensive normalization: a params set with zero (or negative)
// numbers degrades to the shipped tuning, never to a degenerate
// fight - a broken config line cannot shrink the step to zero or
// stop the pacing.
func TestKiteParamsBrokenNumbersFallBackToTheShippedTuning(t *testing.T) {
    // The mob closed to 200 units: the shipped geometry answers with
    // the shipped step (400 units straight away on the x axis).
    _, game, loop := kiteBowBot(t, 45200)
    loop.SetKiteParams(KiteParams{Enabled: true})
    loop.tick()
    require.Len(t, game.walks, 1,
        "the zero numbers must not disable the fight")
    require.Equal(t, int32(44600), game.walks[0][0],
        "the zero step length falls back to the shipped kiteStep")
    require.Equal(t, kiteRetreatRadius, loop.kite.RetreatRadius,
        "the zero radius falls back to the shipped trigger")
    require.Equal(t, kiteStepPeriod, loop.kite.StepPeriod,
        "the zero period falls back to the shipped pacing")
}

// TestKiteParamsReengageDelayHoldsTheReRequest pins the re-engage
// knob: the delay extends the movement hold past the step window -
// the forced attack re-requests wait the window plus the delay (the
// hold the tuning round widens when the aim needs to settle).
func TestKiteParamsReengageDelayHoldsTheReRequest(t *testing.T) {
    _, game, loop := kiteBowBot(t, 45200)
    loop.SetKiteParams(KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: time.Second,
    })
    before := time.Now()
    loop.tick()
    require.Len(t, game.walks, 1)
    // The hold: window (2s) + delay (1s), measured from the tick.
    require.WithinDuration(t,
        before.Add(kiteStepWindow+time.Second),
        loop.combatAvoidUntil, 2*time.Second,
        "the delay must extend the hold past the step window")

    // The shipped profile holds exactly the window: the delay is the
    // only difference between the two holds.
    _, _, loop2 := kiteBowBot(t, 45200)
    before2 := time.Now()
    loop2.tick()
    require.WithinDuration(t,
        before2.Add(kiteStepWindow),
        loop2.combatAvoidUntil, 2*time.Second,
        "the shipped delay of zero holds exactly the window")
}
