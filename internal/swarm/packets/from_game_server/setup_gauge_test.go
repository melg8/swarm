// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func buildSetupGauge(color, current, maxMS int32) []byte {
    data := []byte{setupGaugePacketID}
    data = putInt32(data, color)
    data = putInt32(data, current)
    data = putInt32(data, maxMS)

    return data
}

func TestParseSetupGaugePacket(t *testing.T) {
    t.Run("constructor defaults", func(t *testing.T) {
        p := NewSetupGaugePacket()
        require.Zero(t, p.Color)
        require.Zero(t, p.Current)
        require.Zero(t, p.Max)
        require.False(t, p.IsBowCycle())
    })

    t.Run("bow cycle", func(t *testing.T) {
        // The live acceptance dump of the Short Bow at pAtkSpd 337:
        // 85 01 00 00 00 95 0b 00 00 95 0b 00 00 (2965 ms).
        p := NewSetupGaugePacket()
        require.NoError(t, ParseSetupGaugePacket(p,
            buildSetupGauge(gaugeColorRed, 2965, 2965)))
        require.Equal(t, int32(gaugeColorRed), p.Color)
        require.Equal(t, int32(2965), p.Current)
        require.Equal(t, int32(2965), p.Max)
        require.True(t, p.IsBowCycle())
    })

    t.Run("other gauges are not the bow cycle", func(t *testing.T) {
        p := NewSetupGaugePacket()
        require.NoError(t, ParseSetupGaugePacket(p,
            buildSetupGauge(2, 60000, 60000)))
        require.False(t, p.IsBowCycle(), "the water breath is no shot")
        p = NewSetupGaugePacket()
        require.NoError(t, ParseSetupGaugePacket(p,
            buildSetupGauge(gaugeColorRed, 0, 0)))
        require.False(t, p.IsBowCycle(), "an empty red gauge is no cycle")
    })

    t.Run("wrong packet id", func(t *testing.T) {
        p := NewSetupGaugePacket()
        require.Error(t, ParseSetupGaugePacket(p, []byte{0x86}))
    })

    t.Run("truncated payload", func(t *testing.T) {
        p := NewSetupGaugePacket()
        require.Error(t, ParseSetupGaugePacket(p, []byte{setupGaugePacketID, 1}))
    })
}
