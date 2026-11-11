// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestCharacterProjectionFollowsTheSegment pins the server side
// estimate of the played character place: the Mobius move loop steps a
// mover toward the destination at the effective speed every game tick
// (Creature.updatePosition), so the projection advances along the
// straight segment by the elapsed time, z included, and clamps at the
// destination when the arrival broadcast is lost - never past it.
func TestCharacterProjectionFollowsTheSegment(t *testing.T) {
    start := time.Unix(1_700_000_000, 0)
    walking := CharacterState{
        X: 1000, Y: 2000, Z: -100,
        Moving: true, DestX: 1300, DestY: 2000, DestZ: 300,
        RunSpeed: 100, MoveAt: start,
    }

    // The segment is 500 units long in 3D (300 x and 400 z), so at 100
    // units per second a quarter of it is reached in 1.25 seconds.
    x, y, z := walking.projectedPosition(start)
    require.InDelta(t, 1000, x, 0.001)
    require.InDelta(t, 2000, y, 0.001)
    require.InDelta(t, -100, z, 0.001)

    x, y, z = walking.projectedPosition(start.Add(1250 * time.Millisecond))
    require.InDelta(t, 1075, x, 0.001)
    require.InDelta(t, 2000, y, 0.001)
    require.InDelta(t, 0, z, 0.001)

    x, y, z = walking.projectedPosition(start.Add(time.Minute))
    require.InDelta(t, 1300, x, 0.001)
    require.InDelta(t, 2000, y, 0.001)
    require.InDelta(t, 300, z, 0.001)

    // A standing character keeps the broadcast place, and so does a
    // mover without a usable speed or movement stamp.
    standing := walking
    standing.Moving = false
    x, _, z = standing.projectedPosition(start.Add(time.Minute))
    require.InDelta(t, 1000, x, 0.001)
    require.InDelta(t, -100, z, 0.001)

    speedless := walking
    speedless.RunSpeed = 0
    x, _, z = speedless.projectedPosition(start.Add(time.Minute))
    require.InDelta(t, 1000, x, 0.001)
    require.InDelta(t, -100, z, 0.001)

    unstamped := walking
    unstamped.MoveAt = time.Time{}
    x, _, z = unstamped.projectedPosition(start.Add(time.Minute))
    require.InDelta(t, 1000, x, 0.001)
    require.InDelta(t, -100, z, 0.001)
}

// TestSelfProjectedPositionNeedsTheCharacter pins the accessor of the
// proxy seam: no character means no estimate, a selected character
// projects from its broadcast place, and a movement broadcast keeps
// the estimate on the segment.
func TestSelfProjectedPositionNeedsTheCharacter(t *testing.T) {
    bot := NewBot("proj0")
    _, _, _, ok := bot.SelfProjectedPosition()
    require.False(t, ok)

    bot.SetCharacter("proj1", 100, 18, 45000, 50000, -3500, 90, 40)
    x, y, z, ok := bot.SelfProjectedPosition()
    require.True(t, ok)
    require.InDelta(t, 45000, x, 0.001)
    require.InDelta(t, 50000, y, 0.001)
    require.InDelta(t, -3500, z, 0.001)

    bot.ApplyMovement(Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 47000, DestY: 50000, DestZ: -3500,
    })
    x, y, z, ok = bot.SelfProjectedPosition()
    require.True(t, ok)
    require.GreaterOrEqual(t, x, 45000.0)
    require.LessOrEqual(t, x, 47000.0)
    require.InDelta(t, 50000, y, 0.001)
    require.InDelta(t, -3500, z, 0.001)
}
