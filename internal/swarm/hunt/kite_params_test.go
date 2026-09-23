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
// (the step length rides its own race formula and the train
// geometry). The chase-fresh scene keeps the shot-paced rhythm
// out of the answer: the widened radius itself must arm the step.
func TestKiteParamsWidenTheRetreatRadius(t *testing.T) {
    // 300 units out: beyond the shipped trigger, inside the widened
    // one.
    _, game, loop := kiteBowBotChaseFresh(t, 45300)
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
    require.Equal(t, int32(43650), game.walks[0][0],
        "the step length still rides the race formula: the deficit "+
            "180 bought back at the parity rate caps at the fresh "+
            "anchor's 1350 leash budget")
}

// TestKiteParamsBrokenNumbersFallBackToTheShippedTuning pins the
// defensive normalization: a params set with zero (or negative)
// numbers degrades to the shipped tuning, never to a degenerate
// fight - a broken config line cannot shrink the step to zero or
// stop the pacing.
func TestKiteParamsBrokenNumbersFallBackToTheShippedTuning(t *testing.T) {
    // The mob closed to 200 units: the shipped geometry answers with
    // the shipped race leg (the deficit 280 at the parity rate,
    // capped at the fresh anchor's 1350 leash budget - straight away
    // on the x axis).
    _, game, loop := kiteBowBot(t, 45200)
    loop.SetKiteParams(KiteParams{Enabled: true})
    tickPastTheWindup(loop)
    require.Len(t, game.walks, 1,
        "the zero numbers must not disable the fight")
    require.Equal(t, int32(43650), game.walks[0][0],
        "the zero step length falls back to the shipped race formula")
    require.InDelta(t, kiteRetreatRadius, loop.kite.RetreatRadius, 0.001,
        "the zero radius falls back to the shipped trigger")
    require.Equal(t, kiteStepPeriod, loop.kite.StepPeriod,
        "the zero period falls back to the shipped pacing")
}

// TestKiteParamsReengageDelayHoldsTheReRequest pins the re-engage
// knob against the round-17 composed window: the movement hold is
// the max of the caller's window (the plain window plus the delay)
// and the scaled window of the race leg (leg/kiteStep * the bow
// window - the 1350 race leg of the closed mob scales the shipped 2s
// fallback to 6.75s). A delay inside the scaled window changes
// nothing (the leg owns the hold), a delay past it extends the hold
// one for one - the hold the tuning round widens when the aim needs
// to settle.
func TestKiteParamsReengageDelayHoldsTheReRequest(t *testing.T) {
    // The delay past the scaled window: the plain window (2s) plus
    // the delay (6s) = 8s from the issue, past the scaled 6.75s of
    // the race leg - the knob owns the hold.
    _, game, loop := kiteBowBotChaseFresh(t, 45200)
    loop.SetKiteParams(KiteParams{
        Enabled:       true,
        RetreatRadius: kiteRetreatRadius,
        Step:          kiteStep,
        StepPeriod:    kiteStepPeriod,
        ReengageDelay: 6 * time.Second,
    })
    before := time.Now()
    loop.tick()
    require.Len(t, game.walks, 1)
    scaled := time.Duration(
        kiteRoamRadius / kiteStep * float64(kiteStepWindow))
    require.WithinDuration(t,
        before.Add(kiteStepWindow+6*time.Second),
        loop.combatAvoidUntil, 250*time.Millisecond,
        "a delay past the scaled leg window must extend the hold")
    require.True(t, loop.combatAvoidUntil.After(before.Add(scaled)),
        "the delayed hold outranks the scaled race window")

    // The shipped profile: the scaled window of the race leg wins -
    // the delay of zero leaves the leg the only term past the
    // plain window.
    _, _, loop2 := kiteBowBotChaseFresh(t, 45200)
    before2 := time.Now()
    loop2.tick()
    require.WithinDuration(t, before2.Add(scaled),
        loop2.combatAvoidUntil, 250*time.Millisecond,
        "the shipped delay of zero holds exactly the scaled leg window")
}
