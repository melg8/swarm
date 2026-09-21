// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "encoding/json"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestApplySkillCastOpensWindows pins the snapshot section: a self
// cast opens the cast window (the hit time) and the reuse window (the
// cooldown), the snapshot reads both left over milliseconds against
// the totals.
func TestApplySkillCastOpensWindows(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 1077, SkillLevel: 3,
        HitTimeMs: 1500, ReuseDelayMs: 6000,
    })

    states := bot.Snapshot().SkillStates
    require.Len(t, states, 1)
    state := states[0]
    require.Equal(t, int32(1077), state.SkillID)
    require.Equal(t, int64(1500), state.CastTotalMs)
    require.Equal(t, int64(6000), state.ReuseTotalMs)
    require.Positive(t, state.CastLeftMs)
    require.LessOrEqual(t, state.CastLeftMs, int64(1500))
    require.Positive(t, state.ReuseLeftMs)
    require.LessOrEqual(t, state.ReuseLeftMs, int64(6000))

    // A second skill keeps its own windows, the section stays sorted
    // by skill id.
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 16, SkillLevel: 1,
        HitTimeMs: 0, ReuseDelayMs: 3000,
    })
    states = bot.Snapshot().SkillStates
    require.Len(t, states, 2)
    require.Equal(t, int32(16), states[0].SkillID)
    require.Equal(t, int64(0), states[0].CastLeftMs,
        "an instant cast opens no cast window")
    require.Equal(t, int32(1077), states[1].SkillID)
}

// TestApplySkillCastIgnoresOtherCasters pins the self filter: the
// cast broadcasts of the other creatures open no windows (the cast
// icon and the cooldowns show the played character only).
func TestApplySkillCastIgnoresOtherCasters(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplySkillCast(SkillCast{
        CasterID: 7, TargetID: 100,
        SkillID: 4122, SkillLevel: 1,
        HitTimeMs: 1500, ReuseDelayMs: 6000,
    })

    require.Empty(t, bot.Snapshot().SkillStates)
}

// TestApplySkillCastExpires pins the window lifecycle: the windows
// read zero and the skill drops out of the section once both runs
// elapsed.
func TestApplySkillCastExpires(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 1077, SkillLevel: 3,
        HitTimeMs: 20, ReuseDelayMs: 40,
    })
    require.Len(t, bot.Snapshot().SkillStates, 1)
    time.Sleep(70 * time.Millisecond)
    require.Empty(t, bot.Snapshot().SkillStates)
}

// TestApplySkillCastRecastReplaces pins the recast: a fresh cast of
// the same skill replaces the windows (the countdown restarts, it
// never stacks).
func TestApplySkillCastRecastReplaces(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 1077, SkillLevel: 3,
        HitTimeMs: 1500, ReuseDelayMs: 6000,
    })
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 1077, SkillLevel: 3,
        HitTimeMs: 800, ReuseDelayMs: 2000,
    })

    states := bot.Snapshot().SkillStates
    require.Len(t, states, 1)
    require.Equal(t, int64(800), states[0].CastTotalMs)
    require.Equal(t, int64(2000), states[0].ReuseTotalMs)
}

// TestSkillStatesJSONRoundTrip pins the wire encoding of the new
// section: the hand written snapshot encoder emits the same JSON the
// struct tags describe.
func TestSkillStatesJSONRoundTrip(t *testing.T) {
    bot := NewBot("acc1")
    bot.SetCharacter("unittest1", 100, 18, 45000, 50000, -3500, 50, 30)
    bot.ApplySkillCast(SkillCast{
        CasterID: 100, TargetID: 7,
        SkillID: 1077, SkillLevel: 3,
        HitTimeMs: 1500, ReuseDelayMs: 6000,
    })
    snap := bot.Snapshot()
    data, err := json.Marshal(snap)
    require.NoError(t, err)

    var parsed struct {
        SkillStates []SkillStateView `json:"skillStates"`
    }
    require.NoError(t, json.Unmarshal(data, &parsed))
    require.Len(t, parsed.SkillStates, 1)
    require.Equal(t, snap.SkillStates[0], parsed.SkillStates[0])
}
