// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package connection

import (
    "log"
    "testing"
    "time"

    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// buildSetupGaugePayload builds the RED bow gauge of the live Short
// Bow dump: 85 [color: 4][current: 4][max: 4].
func buildSetupGaugePayload(color, current, maxMS int32) []byte {
    data := []byte{0x85}
    data = appendInt32(data, color)
    data = appendInt32(data, current)
    data = appendInt32(data, maxMS)

    return data
}

// TestSetupGaugeDispatchRecordsTheBowCycle pins the 0x85 dispatch:
// the RED gauge of the played character reaches the tracker as the
// exact bow disable window the kite retreat plans against, while
// every other gauge color (a skill cast, the water breath) leaves
// the last shot cycle untouched.
func TestSetupGaugeDispatchRecordsTheBowCycle(t *testing.T) {
    bot := state.NewBot("acc1")
    client := &GameClient{
        logger:     log.New(&testLogBuffer{}, "", 0),
        tracker:    bot,
        setupGauge: *fromgameserver.NewSetupGaugePacket(),
    }

    client.handleServerPacket(buildSetupGaugePayload(1, 2965, 2965))
    at, cycle := bot.SelfBowCycle()
    require.False(t, at.IsZero(), "the shot gauge must land")
    require.Equal(t, 2965*time.Millisecond, cycle)
    require.WithinDuration(t, time.Now(), at, time.Second)

    client.handleServerPacket(buildSetupGaugePayload(2, 60000, 60000))
    at2, cycle2 := bot.SelfBowCycle()
    require.True(t, at2.Equal(at),
        "a non bow gauge must not restamp the shot")
    require.Equal(t, cycle, cycle2)
}
