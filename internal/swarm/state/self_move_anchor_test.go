// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// anchorDestX is the destination of the staged walk.
const anchorDestX int32 = 3000

// movingSelf stages a character walking from x=1000 to x=3000 with the
// movement stamp aged by two seconds, the shape of a walk that has been
// running for a while. It returns the aged stamp the fresh sample must
// advance past.
func movingSelf(t *testing.T) (*Bot, time.Time) {
    t.Helper()
    bot := NewBot("anchor0")
    bot.SetCharacter("anchor", 100, 18, 1000, 2000, -100, 165, 82)
    bot.ApplyMovement(Movement{
        ObjectID: 100, X: 1000, Y: 2000, Z: -100,
        DestX: anchorDestX, DestY: 2000, DestZ: -100,
    })
    aged := time.Now().Add(-2 * time.Second)
    bot.mu.Lock()
    bot.char.MoveAt = aged
    bot.mu.Unlock()

    return bot, aged
}

// requireReanchored checks the sample moved the character, kept the
// movement state and restarted the projection clock at the sample, so
// the estimated server position sits at the sample instead of two
// seconds of travel ahead of it (the rollback source).
func requireReanchored(
    t *testing.T, bot *Bot, aged time.Time, x int32,
) {
    t.Helper()
    char := bot.Snapshot().Character
    require.True(t, char.Moving, "the sample must keep the move state")
    require.Equal(t, anchorDestX, char.DestX,
        "the sample must keep the destination")
    require.Equal(t, x, char.X, "the sample must land at its position")
    require.Greater(t, char.MoveAtMs, aged.UnixMilli(),
        "the sample must restart the projection clock")

    projectedX, _, _, ok := bot.SelfProjectedPosition()
    require.True(t, ok)
    require.InDelta(t, float64(x), projectedX, 5,
        "the projection must not overrun the fresh sample")
}

// TestSelfUserInfoReanchorsTheMove pins the UserInfo refresh of a
// moving character: the server builds the packet at its own position,
// so the tracker adopts the sample and restarts the projection there.
// Keeping the old movement stamp let the client projection run ahead by
// the already covered distance and snap back on the next broadcast.
func TestSelfUserInfoReanchorsTheMove(t *testing.T) {
    bot, aged := movingSelf(t)

    bot.ApplyUserInfo(UserInfo{
        Name: "anchor", X: 1320, Y: 2000, Z: -100,
        RunSpeed: 165, WalkSpeed: 82, MoveSpeedMult: 1,
    })

    requireReanchored(t, bot, aged, 1320)
}

// TestSelfPawnTargetSampleReanchorsTheMove pins the chase sample of a
// mob that targets the played character: the MoveToPawn target fields
// carry the character position at packet build time and no movement
// state, so the tracker re-anchors the running move instead of leaving
// it on the old stamp.
func TestSelfPawnTargetSampleReanchorsTheMove(t *testing.T) {
    bot, aged := movingSelf(t)
    bot.ApplyNpcInfo(NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 1400, Y: 2000, Name: "Gremlin",
    })

    bot.ApplyPawnMovement(PawnMovement{
        ObjectID: 7, TargetID: 100, Distance: 40,
        X: 1400, Y: 2000, Z: -100,
        TargetX: 1100, TargetY: 2000, TargetZ: -100,
    })

    requireReanchored(t, bot, aged, 1100)
}

// TestSelfAttackSampleReanchorsTheMove pins both self positions of an
// Attack packet: the attacker block of the own swing and the target
// block of a hit on the character. Both carry a fresh sample while the
// move keeps running, so neither may stop the move or leave the stale
// stamp behind.
func TestSelfAttackSampleReanchorsTheMove(t *testing.T) {
    t.Run("self attacker", func(t *testing.T) {
        bot, aged := movingSelf(t)
        before := time.Now()

        bot.ApplyAttackAt(Attack{
            AttackerID: 100, X: 1150, Y: 2000, Z: -100,
            TargetCount: 1,
            TargetIDs:   [AttackTargets]int32{7},
            TargetX:     1400, TargetY: 2000, TargetZ: -100,
        }, before)

        requireReanchored(t, bot, aged, 1150)
    })

    t.Run("self target", func(t *testing.T) {
        bot, aged := movingSelf(t)
        before := time.Now()

        bot.ApplyAttackAt(Attack{
            AttackerID: 7, X: 1400, Y: 2000, Z: -100,
            TargetCount: 1,
            TargetIDs:   [AttackTargets]int32{100},
            TargetX:     1200, TargetY: 2000, TargetZ: -100,
        }, before)

        requireReanchored(t, bot, aged, 1200)
    })
}

// TestStandingSelfSampleKeepsTheStamp pins the other side of the
// re-anchor gate: a standing character adopts the sample position but
// never gains a movement stamp or a destination from it.
func TestStandingSelfSampleKeepsTheStamp(t *testing.T) {
    bot := NewBot("anchor0")
    bot.SetCharacter("anchor", 100, 18, 1000, 2000, -100, 165, 82)
    stamp := bot.Snapshot().Character.MoveAtMs

    bot.ApplyUserInfo(UserInfo{
        Name: "anchor", X: 1100, Y: 2000, Z: -100,
        RunSpeed: 165, WalkSpeed: 82, MoveSpeedMult: 1,
    })

    char := bot.Snapshot().Character
    require.False(t, char.Moving)
    require.Equal(t, int32(1100), char.X)
    require.Equal(t, stamp, char.MoveAtMs,
        "a standing sample must not restart the projection clock")
}
