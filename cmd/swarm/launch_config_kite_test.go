// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/stretchr/testify/require"
)

// The kite section of the bot spec (owner issue #29): the per-spec
// overrides of the kite fight so the standing-archer baseline and the
// tuned profile switch by config, not by rebuild. The tests pin the
// load, the pointer semantics (unset fields keep the shipped tuning),
// the expansion carry and the validation contract.

// TestLaunchConfigLoadsTheKiteSection pins the happy path: the kite
// section parses into the spec, rides the expansion (every plan slot
// of the spec carries the same override) and reads into the hunt
// params with the set fields overriding and the unset fields keeping
// the shipped defaults.
func TestLaunchConfigLoadsTheKiteSection(t *testing.T) {
    path := writeConfigFile(t, `{
        "account": "tune1",
        "bots": [
            {"type": "archer", "count": 2, "kite": {
                "enabled": true,
                "retreatRadius": 300,
                "step": 450,
                "stepPeriod": "4s",
                "reengageDelay": "500ms"
            }}
        ]
    }`)
    lc, err := loadLaunchConfig(path)
    require.NoError(t, err)

    plan := lc.expand()
    require.Len(t, plan, 2)
    require.NotNil(t, plan[0].Kite,
        "every slot of the spec carries the kite override")
    require.NotNil(t, plan[1].Kite)

    params := plan[0].Kite.toParams()
    require.True(t, params.Enabled)
    require.Equal(t, 300.0, params.RetreatRadius)
    require.Equal(t, 450.0, params.Step)
    require.Equal(t, 4*time.Second, params.StepPeriod)
    require.Equal(t, 500*time.Millisecond, params.ReengageDelay)
}

// TestLaunchConfigKiteSectionUnsetFieldsKeepTheDefaults pins the
// pointer semantics: a partial section overrides exactly the named
// knobs - the omitted fields keep the shipped tuning of
// DefaultKiteParams (the acceptance scenario behavior).
func TestLaunchConfigKiteSectionUnsetFieldsKeepTheDefaults(t *testing.T) {
    path := writeConfigFile(t, `{
        "bots": [
            {"type": "archer", "count": 1, "kite": {"retreatRadius": 350}}
        ]
    }`)
    lc, err := loadLaunchConfig(path)
    require.NoError(t, err)
    params := lc.expand()[0].Kite.toParams()
    require.Equal(t, 350.0, params.RetreatRadius,
        "the named knob overrides")
    require.Equal(t, hunt.DefaultKiteParams().Step, params.Step,
        "the omitted step keeps the shipped value")
    require.Equal(t, hunt.DefaultKiteParams().StepPeriod, params.StepPeriod,
        "the omitted period keeps the shipped value")
    require.Equal(t, hunt.DefaultKiteParams().ReengageDelay,
        params.ReengageDelay,
        "the omitted delay keeps the shipped value")
    require.True(t, params.Enabled,
        "the omitted gate keeps the kite enabled")
}

// TestLaunchConfigWithoutKiteSectionKeepsTheShippedTuning pins the
// absence case: a spec (and the plain flag ladder) carries no kite
// section, the loop keeps its defaults - the existing deployments
// change nothing.
func TestLaunchConfigWithoutKiteSectionKeepsTheShippedTuning(t *testing.T) {
    path := writeConfigFile(t, `{
        "bots": [
            {"type": "fighter", "count": 1},
            {"type": "archer", "count": 1}
        ]
    }`)
    lc, err := loadLaunchConfig(path)
    require.NoError(t, err)
    for i, plan := range lc.expand() {
        require.Nil(t, plan.Kite,
            "spec slot %d carries no kite override", i)
    }
    require.Nil(t, classicFleetPlan("test1", 2)[0].Kite,
        "the flag ladder carries no kite override")
}

// TestLaunchConfigBaselineProfileDisablesTheKite pins the baseline
// profile of the tuning round: {"enabled": false} and nothing else -
// the reading keeps the shipped numbers (they do not matter with the
// gate closed) and flips the gate only.
func TestLaunchConfigBaselineProfileDisablesTheKite(t *testing.T) {
    path := writeConfigFile(t, `{
        "bots": [
            {"type": "archer", "count": 1, "kite": {"enabled": false}}
        ]
    }`)
    lc, err := loadLaunchConfig(path)
    require.NoError(t, err)
    params := lc.expand()[0].Kite.toParams()
    require.False(t, params.Enabled,
        "the baseline profile runs the standing archer")
    require.Equal(t, hunt.DefaultKiteParams().RetreatRadius,
        params.RetreatRadius,
        "the numbers stay at the shipped values")
}

// TestLaunchConfigRejectsBrokenKiteSections pins the validation
// contract of the section: a fighter spec with a kite section, a
// non-positive distance and an unparsable duration all refuse the
// launch with an error naming the spec and the field.
func TestLaunchConfigRejectsBrokenKiteSections(t *testing.T) {
    t.Run("the kite section on a fighter refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "fighter", "count": 1, "kite": {
                "enabled": false
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err,
            "the kite section tunes the archer fight")
    })

    t.Run("the zero retreat radius refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "archer", "count": 1, "kite": {
                "retreatRadius": 0
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err,
            "kite.retreatRadius must be positive")
    })

    t.Run("the negative step refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "archer", "count": 1, "kite": {
                "step": -400
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "kite.step must be positive")
    })

    t.Run("the unparsable step period refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "archer", "count": 1, "kite": {
                "stepPeriod": "three seconds"
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "kite.stepPeriod")
        require.ErrorContains(t, err, "is not a duration")
    })

    t.Run("the unparsable reengage delay refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "archer", "count": 1, "kite": {
                "reengageDelay": "1"
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "kite.reengageDelay")
    })

    t.Run("the unknown kite field refuses", func(t *testing.T) {
        path := writeConfigFile(t, `{
            "bots": [{"type": "archer", "count": 1, "kite": {
                "retreatRadus": 300
            }}]
        }`)
        _, err := loadLaunchConfig(path)
        require.ErrorContains(t, err, "unknown field")
    })
}
