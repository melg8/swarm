// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The movement abuse scenario tests: the desync validate position
// round and the cursor movement round - the two packet level movement
// channels a bot can ride faster than the run speed the server would
// move the character at.

// TestDesyncScenarioOwnsTheListHead pins the slots of the short
// movement abuse rounds behind the three route rounds: the desync
// drift owns the fourth slot, the cursor movement round the fifth.
func TestDesyncScenarioOwnsTheListHead(t *testing.T) {
    defs := Definitions()
    require.NotEmpty(t, defs)
    require.Equal(t, desyncRouteID, defs[0].ID,
        "the desync route scenario owns the list head")
    require.Equal(t, cursorRouteID, defs[1].ID,
        "the cursor route scenario owns the second slot")
    require.Equal(t, fastRouteID, defs[2].ID,
        "the fast route scenario owns the third slot")
    require.Equal(t, desyncScenarioID, defs[3].ID,
        "the desync position scenario owns the fourth slot")
    require.Equal(t, desyncAccount, defs[3].Account)
    require.Equal(t, desyncTimeout, defs[3].Timeout)
    require.NotNil(t, defs[3].Scenario)
    for _, needle := range []string{
        "46045 41251 -3440", "ValidatePosition", "setXYZ",
        "700 units west", "twice", "3000+ units",
    } {
        require.Contains(t, defs[3].Description, needle)
    }

    require.Equal(t, cursorScenarioID, defs[4].ID,
        "the cursor movement scenario owns the fifth slot")
    require.Equal(t, cursorAccount, defs[4].Account)
    require.Equal(t, cursorTimeout, defs[4].Timeout)
    require.NotNil(t, defs[4].Scenario)
    for _, needle := range []string{
        "cursor key", "94 unit hops", "twice", "3000+ units",
    } {
        require.Contains(t, defs[4].Description, needle)
    }
}

// TestDesyncHopLadder pins the claim ladder geometry: the hops walk
// the creation spawn line west in move-speed-clearing strides and
// the final claim lands a long distance away.
func TestDesyncHopLadder(t *testing.T) {
    require.Equal(t, int32(46045-700), desyncHopX(1))
    require.Equal(t, int32(46045-5600), desyncFinalX(),
        "eight hops of 700 units cover 5600 units")
    for hop := 1; hop <= desyncHopCount; hop++ {
        delta := desyncHopX(hop) - desyncHopX(hop-1)
        require.Negative(t, delta)
        require.Equal(t, int32(desyncHopDistance), -delta)
        require.Greater(t, -delta, int32(600),
            "every hop clears the correction band of the handler")
    }
    require.Greater(t, float64(elvenSpawnX-desyncFinalX()),
        desyncMinDistance)
}

// TestDesyncResetIsTheBareSpawn pins the start state of the desync
// round: the bare level 15 character at the creation spawn point.
func TestDesyncResetIsTheBareSpawn(t *testing.T) {
    reset := desyncReset(desyncAccount)
    require.Equal(t, desyncAccount, reset.Account)
    require.Equal(t, int32(15), reset.Level)
    require.Equal(t, int32(elvenSpawnX), reset.X)
    require.Equal(t, int32(elvenSpawnY), reset.Y)
    require.Equal(t, int32(elvenSpawnZ), reset.Z)
    require.Empty(t, reset.Items)
    require.Equal(t, int32(level15HP), reset.MaxHP)
}

// TestDesyncEffectiveSpeed pins the speed math: the displacement over
// the claim window, and zero for a degenerate window.
func TestDesyncEffectiveSpeed(t *testing.T) {
    started := time.Now()
    lastAt := started.Add(10 * time.Second)
    speed := desyncEffectiveSpeed(desyncFinalX(), started, lastAt)
    require.InDelta(t, 560.0, speed, 0.001,
        "5600 units over ten seconds drift at 560 units per second")

    require.Zero(t, desyncEffectiveSpeed(
        desyncFinalX(), time.Time{}, lastAt))
    require.Zero(t, desyncEffectiveSpeed(
        desyncFinalX(), started, time.Time{}))
    require.Zero(t, desyncEffectiveSpeed(
        desyncFinalX(), lastAt, started),
        "a negative window reports no speed")
}

// TestEvaluateDesyncChecks pins the pass gates of the desync round:
// the confirmed hops, the speed factor and the distance thresholds
// decide the checks, the run speed reference scales with the server
// report.
func TestEvaluateDesyncChecks(t *testing.T) {
    test := &Test{def: TestDef{ID: desyncScenarioID}}
    test.setChecks(desyncChecks())

    evaluateDesyncChecks(test, desyncMinHops-1, 100, 1000, 125)
    for _, id := range []string{
        checkDesyncHops, checkDesyncSpeed, checkDesyncDistance,
    } {
        require.False(t, checkDone(test, id), "%s must fail short", id)
    }

    evaluateDesyncChecks(test, desyncMinHops, 560, 5600, 125)
    require.True(t, checkDone(test, checkDesyncHops))
    require.True(t, checkDone(test, checkDesyncSpeed),
        "560 clears twice the 125 run speed")
    require.True(t, checkDone(test, checkDesyncDistance))

    evaluateDesyncChecks(test, desyncMinHops, 250, desyncMinDistance,
        125)
    require.True(t, checkDone(test, checkDesyncHops))
    require.True(t, checkDone(test, checkDesyncSpeed),
        "250 equals twice the 125 run speed")
    require.True(t, checkDone(test, checkDesyncDistance))
}

// TestEvaluateDesyncStored pins the store check: the character row
// keeps the drifted placement within the walk-past tolerance and the
// z band.
func TestEvaluateDesyncStored(t *testing.T) {
    test := &Test{def: TestDef{ID: desyncScenarioID}}
    test.setChecks(desyncChecks())

    evaluateDesyncStored(test, desyncFinalX()-60, elvenSpawnY,
        elvenSpawnZ)
    require.True(t, checkDone(test, checkDesyncStored),
        "the probe walk past the final claim stays within tolerance")

    evaluateDesyncStored(test, elvenSpawnX, elvenSpawnY, elvenSpawnZ)
    require.False(t, checkDone(test, checkDesyncStored),
        "the spawn placement fails the store check")
}

// TestCursorHopStream pins the claim stream geometry: the hops stay
// under the move speed band (only the cursor key branch can adopt
// them), the stream covers a long distance and the cycles bundle the
// claims without queue pileup.
func TestCursorHopStream(t *testing.T) {
    require.Equal(t, int32(46045-94), cursorHopX(1))
    require.Equal(t, int32(46045-5640), cursorFinalX(),
        "sixty hops of 94 units cover 5640 units")
    for hop := 1; hop <= cursorHopCount; hop++ {
        delta := cursorHopX(hop) - cursorHopX(hop-1)
        require.Equal(t, int32(cursorHopDistance), -delta)
        require.Less(t, -delta, int32(125),
            "every hop stays under the move speed band")
    }
    require.Greater(t, float64(elvenSpawnX-cursorFinalX()),
        cursorMinDistance)
    require.Equal(t, 20, cursorCycleCount())
    require.Equal(t, cursorHopCount,
        cursorCycleCount()*cursorCycleClaims,
        "the cycles cover the whole stream")
}

// TestCursorResetIsTheBareSpawn pins the start state of the cursor
// movement round: the bare level 15 character at the creation spawn.
func TestCursorResetIsTheBareSpawn(t *testing.T) {
    reset := cursorReset(cursorAccount)
    require.Equal(t, cursorAccount, reset.Account)
    require.Equal(t, int32(15), reset.Level)
    require.Equal(t, int32(elvenSpawnX), reset.X)
    require.Empty(t, reset.Items)
}

// TestEvaluateCursorChecks pins the pass gates of the cursor round.
func TestEvaluateCursorChecks(t *testing.T) {
    test := &Test{def: TestDef{ID: cursorScenarioID}}
    test.setChecks(cursorChecks())

    evaluateCursorChecks(test, cursorMinCycles-1, 100, 1000, 125)
    for _, id := range []string{
        checkCursorStream, checkCursorSpeed, checkCursorDist,
    } {
        require.False(t, checkDone(test, id), "%s must fail short", id)
    }

    evaluateCursorChecks(test, cursorMinCycles, 537, 5640, 125)
    require.True(t, checkDone(test, checkCursorStream))
    require.True(t, checkDone(test, checkCursorSpeed),
        "537 clears twice the 125 run speed")
    require.True(t, checkDone(test, checkCursorDist))

    evaluateCursorChecks(test, cursorMinCycles, 250, cursorMinDistance,
        125)
    require.True(t, checkDone(test, checkCursorStream))
    require.True(t, checkDone(test, checkCursorSpeed))
    require.True(t, checkDone(test, checkCursorDist))
}

// TestEvaluateCursorStored pins the store check of the cursor round.
func TestEvaluateCursorStored(t *testing.T) {
    test := &Test{def: TestDef{ID: cursorScenarioID}}
    test.setChecks(cursorChecks())

    evaluateCursorStored(test, cursorFinalX()-60, elvenSpawnY,
        elvenSpawnZ)
    require.True(t, checkDone(test, checkCursorStored))

    evaluateCursorStored(test, elvenSpawnX, elvenSpawnY, elvenSpawnZ)
    require.False(t, checkDone(test, checkCursorStored))
}

// TestCursorScenarioCommandsPushClean pins the command payloads of
// the cursor ride: the arm, one claim cycle and the disarm queue the
// documented kinds with the line coordinates.
func TestCursorScenarioCommandsPushClean(t *testing.T) {
    bot := state.NewBot(cursorAccount)

    pushCursorArm(bot)
    pushCursorClaims(bot, 1, cursorCycleClaims)
    pushCursorDisarm(bot)

    commands := drainAllCommands(bot)
    require.Len(t, commands, 5)
    require.Equal(t, state.CommandCursorWalk, commands[0].Kind)
    require.Equal(t, cursorFinalX(), commands[0].X)
    for i, claim := range commands[1 : len(commands)-1] {
        require.Equal(t, state.CommandClaimPosition, claim.Kind)
        require.Equal(t, cursorHopX(i+1), claim.X)
    }
    disarm := commands[len(commands)-1]
    require.Equal(t, state.CommandClickWalk, disarm.Kind)
    require.Equal(t, cursorFinalX()-desyncProbeOffset, disarm.X)
}

// checkDone reads the done flag of one check by id.
func checkDone(test *Test, id string) bool {
    test.mu.Lock()
    defer test.mu.Unlock()
    for i := range test.checks {
        if test.checks[i].ID == id {
            return test.checks[i].Done
        }
    }

    return false
}

// drainAllCommands empties the command queue of a bot and returns the
// commands in queue order.
func drainAllCommands(bot *state.Bot) []state.Command {
    commands := make([]state.Command, 0, 8)
    for {
        select {
        case cmd := <-bot.Commands():
            commands = append(commands, cmd)
        default:
            return commands
        }
    }
}
