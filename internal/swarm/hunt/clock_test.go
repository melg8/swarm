// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestClockSeam pins the two sources of the loop: the zero value reads
// the system clock, an injected clock answers every read, and nil
// restores the system clock.
func TestClockSeam(t *testing.T) {
    loop := NewLoop(&fakeGame{}, state.NewBot("acc1"))

    require.WithinDuration(t, time.Now(), loop.now(), time.Second)

    fixed := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
    loop.SetClock(func() time.Time { return fixed })
    require.True(t, loop.now().Equal(fixed))
    require.True(t, loop.now().Equal(fixed), "the clock must be stable")

    loop.SetClock(nil)
    require.WithinDuration(t, time.Now(), loop.now(), time.Second)
}
