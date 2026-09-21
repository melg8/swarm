// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "strings"
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

func TestSayPacketLayoutGeneral(t *testing.T) {
    request := NewSay("Hello world", SayChannelGeneral)
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))

    // [0x38]["Hello world" UTF-16LE + null][0][0][0][0]
    want := []byte{0x38}
    for _, ch := range "Hello world" {
        want = append(want, byte(ch), 0x00)
    }
    want = append(want, 0x00, 0x00)
    want = append(want, 0x00, 0x00, 0x00, 0x00)
    require.Equal(t, want, writer.Bytes())
}

func TestSayPacketLayoutWhisper(t *testing.T) {
    request := NewSay("psst", SayChannelWhisper)
    request.Target = "Melg"
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))

    // [0x38]["psst"+null][2]["Melg"+null].
    want := []byte{0x38}
    for _, ch := range "psst" {
        want = append(want, byte(ch), 0x00)
    }
    want = append(want, 0x00, 0x00)
    want = append(want, 0x02, 0x00, 0x00, 0x00)
    for _, ch := range "Melg" {
        want = append(want, byte(ch), 0x00)
    }
    want = append(want, 0x00, 0x00)
    require.Equal(t, want, writer.Bytes())
}

func TestSayPacketValidateRefusesEmpty(t *testing.T) {
    request := NewSay("", SayChannelGeneral)
    require.Error(t, request.Validate())
    writer := packet.NewWriter()
    require.Error(t, request.ToBytes(writer))
}

func TestSayPacketValidateRefusesLong(t *testing.T) {
    request := NewSay(strings.Repeat("a", SayMaxTextRunes+1),
        SayChannelGeneral)
    require.Error(t, request.Validate())
}

func TestSayPacketValidateAcceptsLimit(t *testing.T) {
    request := NewSay(strings.Repeat("a", SayMaxTextRunes),
        SayChannelGeneral)
    require.NoError(t, request.Validate())
}

func TestSayPacketValidateRefusesUnknownChannel(t *testing.T) {
    request := NewSay("hi", 99)
    require.Error(t, request.Validate())
}

func TestSayPacketValidateRefusesWhisperWithoutTarget(t *testing.T) {
    request := NewSay("hi", SayChannelWhisper)
    require.Error(t, request.Validate())
}
