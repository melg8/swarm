// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestClockSeam pins the two sources of the tracker: the zero value
// reads the system clock, an injected clock answers every read, and nil
// restores the system clock.
func TestClockSeam(t *testing.T) {
    bot := NewBot("acc1")

    require.WithinDuration(t, time.Now(), bot.now(), time.Second)

    fixed := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
    bot.SetClock(func() time.Time { return fixed })
    require.True(t, bot.now().Equal(fixed))
    require.True(t, bot.now().Equal(fixed), "the clock must be stable")

    bot.SetClock(nil)
    require.WithinDuration(t, time.Now(), bot.now(), time.Second)
}
