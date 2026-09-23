// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestKiteProbeRegistered pins the registration of the probe: the id,
// the account and the timeout budget under the six minute cap.
func TestKiteProbeRegistered(t *testing.T) {
    defs := Definitions()
    found := false
    for _, def := range defs {
        if def.ID != kiteProbeScenarioID {
            continue
        }
        found = true
        require.Equal(t, kiteProbeAccount, def.Account)
        require.Less(t, def.Timeout, 6*time.Minute,
            "the probe stays under the six minute process cap")
    }
    require.True(t, found, "the probe must be registered")
}

// TestKiteProbeReset pins the start state: the archer kit plus the
// potion belt on the proven spawn focus.
func TestKiteProbeReset(t *testing.T) {
    reset := kiteProbeReset(kiteProbeAccount)
    require.Equal(t, kiteProbeAccount, reset.Account)
    require.EqualValues(t, archerKiteSpawnX, reset.X)
    require.EqualValues(t, archerKiteSpawnY, reset.Y)
    require.EqualValues(t, archerKiteSpawnZ, reset.Z)
    var potions int32
    bow := false
    for _, item := range reset.Items {
        if item.ItemID == 1060 {
            potions += item.Count
        }
        if item.ItemID == 13 {
            bow = true
        }
    }
    require.True(t, bow, "the kit carries the short bow")
    // The base archer kit already carries 3 potions; the probe belt
    // adds its own stack on top.
    require.EqualValues(t, 3+kiteProbePotions, potions)
}

// TestClassifyProbeRound pins the round classification: the movement
// broadcast inside the immediate cap is immediate, a later one is
// deferred and no broadcast at all never moved.
func TestClassifyProbeRound(t *testing.T) {
    anchor := time.Now()
    click := anchor.Add(600 * time.Millisecond)
    cases := []struct {
        name   string
        round  probeRound
        verdict string
    }{
        {
            name: "immediate",
            round: probeRound{
                shotAt: anchor, walkSent: click,
                movedAt: click.Add(80 * time.Millisecond),
            },
            verdict: "immediate",
        },
        {
            name: "deferred",
            round: probeRound{
                shotAt: anchor, walkSent: click,
                movedAt: click.Add(2400 * time.Millisecond),
            },
            verdict: "deferred",
        },
        {
            name: "no-move",
            round: probeRound{
                shotAt: anchor, walkSent: click,
            },
            verdict: "no-move",
        },
    }
    for _, testCase := range cases {
        t.Run(testCase.name, func(t *testing.T) {
            round := testCase.round
            round.verdict = classifyProbeRound(round)
            require.Equal(t, testCase.verdict, round.verdict)
        })
    }
}

// TestSummarizeLadder pins the boundary fold: the smallest immediate
// delay wins and a ladder without an immediate round has no boundary.
func TestSummarizeLadder(t *testing.T) {
    base := time.Now()
    rounds := []probeRound{
        {delay: 0, verdict: "deferred", shotAt: base},
        {delay: 300 * time.Millisecond, verdict: "deferred", shotAt: base},
        {delay: 1200 * time.Millisecond, verdict: "immediate",
            shotAt: base},
        {delay: 1500 * time.Millisecond, verdict: "immediate",
            shotAt: base},
    }
    summary := summarizeLadder(rounds)
    require.True(t, summary.HasBoundary)
    require.Equal(t, 1200*time.Millisecond, summary.ImmediateAt)
    require.Equal(t, []string{
        "deferred", "deferred", "immediate", "immediate",
    }, summary.Verdicts)

    empty := summarizeLadder([]probeRound{
        {delay: 0, verdict: "deferred", shotAt: base},
    })
    require.False(t, empty.HasBoundary)
}

// TestProbeRecorderSelfEvents pins the wire event recorder: the own
// attack and movement broadcasts latch, the foreign ones do not.
func TestProbeRecorderSelfEvents(t *testing.T) {
    selfID := int32(77)
    recorder := newProbeRecorder(func() int32 { return selfID })
    attack := []byte{probeOpAttack, 0x4d, 0x00, 0x00, 0x00}
    foreign := []byte{probeOpAttack, 0x99, 0x00, 0x00, 0x00}
    move := []byte{probeOpMove, 0x4d, 0x00, 0x00, 0x00}
    failed := []byte{probeOpFailed, 0x00, 0x00, 0x00, 0x00}
    recorder.recv(attack)
    recorder.recv(foreign)
    recorder.recv(failed)
    recorder.recv(move)
    mark := time.Now().Add(-time.Second)
    require.False(t, recorder.lastAttackAfter(mark).IsZero(),
        "the own attack broadcast latched")
    require.False(t, recorder.firstMoveAfterMarkIsZero(mark))
    _, fails := recorder.firstMoveAfter(mark)
    require.Equal(t, 1, fails)
}

// firstMoveAfterMarkIsZero answers whether no own movement broadcast
// arrived after the mark (the test helper of the recorder check).
func (r *probeRecorder) firstMoveAfterMarkIsZero(
    mark time.Time,
) bool {
    at, _ := r.firstMoveAfter(mark)

    return at.IsZero()
}

// TestRetreatEndpoint pins the retreat geometry: the endpoint runs
// the kite step length along the away ray and a zero ray falls back
// to the east.
func TestRetreatEndpoint(t *testing.T) {
    endX, endY := retreatEndpoint(36000, 50229, 35000, 50229)
    require.InDelta(t, 36400, endX, 1.0)
    require.InDelta(t, 50229, endY, 1.0)
    sameX, sameY := retreatEndpoint(36000, 50229, 36000, 50229)
    require.InDelta(t, 36400, sameX, 1.0)
    require.InDelta(t, 50229, sameY, 1.0)
}
